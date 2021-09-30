package router

import (
	"encoding/hex"
	"fmt"
	"math"
	"time"

	"github.com/Arceliar/phony"
	"github.com/matrix-org/pinecone/types"
)

// announcementInterval is the frequency at which this
// node will send root announcements to other peers.
const announcementInterval = time.Second * 5

// announcementTimeout is the amount of time that must
// pass without receiving a root announcement before we
// will assume that the peer is dead.
const announcementTimeout = announcementInterval * 2

func (s *state) _maintainTree() {
	select {
	case <-s.r.context.Done():
		return
	default:
	}

	if s._parent == nil {
		s._sendTreeAnnouncements()
	}

	s._maintainTreeIn(announcementInterval)
}

type rootAnnouncementWithTime struct {
	types.SwitchAnnouncement
	receiveTime  time.Time // when did we receive the update?
	receiveOrder uint64    // the relative order that the update was received
}

func (a *rootAnnouncementWithTime) forPeer(p *peer) *types.Frame {
	if p == nil || p.port == 0 {
		return nil
	}
	announcement := a.SwitchAnnouncement
	announcement.Signatures = append([]types.SignatureWithHop{}, a.Signatures...)
	for _, sig := range announcement.Signatures {
		if p.router.public.EqualTo(sig.PublicKey) {
			// For some reason the announcement that we want to send already
			// includes our signature. This shouldn't really happen but if we
			// did send it, other nodes would end up ignoring the announcement
			// anyway since it would appear to be a routing loop.
			return nil
		}
	}
	// Sign the announcement.
	if err := announcement.Sign(p.router.private[:], p.port); err != nil {
		p.router.log.Println("Failed to sign switch announcement:", err)
		return nil
	}
	frame := &types.Frame{
		Type:    types.TypeSTP,
		Payload: make([]byte, types.MaxPayloadSize),
	}
	n, err := announcement.MarshalBinary(frame.Payload[:])
	if err != nil {
		p.router.log.Println("Failed to marshal switch announcement:", err)
		return nil
	}
	frame.Payload = frame.Payload[:n]
	return frame
}

func (s *state) _rootAnnouncement() *rootAnnouncementWithTime {
	if s._parent == nil || s._announcements[s._parent] == nil {
		return &rootAnnouncementWithTime{
			SwitchAnnouncement: types.SwitchAnnouncement{
				RootPublicKey: s.r.public,
				Sequence:      types.Varu64(s._sequence),
			},
		}
	}
	return s._announcements[s._parent]
}

func (s *state) coords() types.SwitchPorts {
	var coords types.SwitchPorts
	phony.Block(s, func() {
		coords = s._coords()
	})
	return coords
}

func (s *state) _coords() types.SwitchPorts {
	if ann := s._rootAnnouncement(); ann != nil {
		return ann.Coords()
	}
	return types.SwitchPorts{}
}

func (s *state) _becomeRoot() {
	if s._parent == nil {
		return
	}
	s._parent = nil
	s.r.log.Println("New coords:", types.SwitchPorts{})
	s._maintainTreeIn(0)
}

func (s *state) _sendTreeAnnouncementToPeer(ann *rootAnnouncementWithTime, p *peer) {
	if peerAnn := ann.forPeer(p); peerAnn != nil {
		p.proto.push(peerAnn)
	}
}

func (s *state) _sendTreeAnnouncements() {
	ann := s._rootAnnouncement()
	s.Act(s, func() {
		var peers []*peer
		phony.Block(s.r, func() {
			peers = s.r._peers
		})
		for _, p := range peers {
			if p == nil || !p.started.Load() {
				continue
			}
			s._sendTreeAnnouncementToPeer(ann, p)
		}
	})
}

func (s *state) _nextHopsTree(from *peer, f *types.Frame) []*peer {
	// We'll collect all possible candidates. We start at PortCount-1
	// because that guarantees the last candidate port is always 0, so
	// that if we don't know what else to do with a packet, we hand it
	// up to the local router.
	candidates := make([]*peer, PortCount)
	canlength := PortCount
	newCandidate := func(peer *peer) {
		canlength--
		candidates[canlength] = peer
	}

	// If it's loopback then don't bother doing anything else.
	ourCoords := s._coords()
	if f.Destination.EqualTo(ourCoords) {
		return []*peer{s.r.local}
	}

	// Work out how close our own coordinates are to the destination
	// message. This is important because we'll only forward a frame
	// to a peer that takes the message closer to the destination than
	// we are.
	ourDist := int64(ourCoords.DistanceTo(f.Destination))
	if ourDist == 0 {
		// It's impossible to get closer so there's a pretty good
		// chance at this point that the traffic is destined for us.
		// Pass it up to the router.
		return []*peer{s.r.local}
	}

	var peers []*peer
	phony.Block(s.r, func() {
		peers = s.r._peers
	})

	// Now work out which of our peers takes the message closer.
	bestDist := ourDist
	for _, p := range peers {
		if p == nil || p == s.r.local {
			continue
		}
		ann := s._announcements[p]
		if ann == nil {
			continue // panic(fmt.Sprintf("found nil ann for peer %+v", p)) // continue
		}

		// Don't deliberately create routing loops by forwarding
		// to a node that doesn't share our root - the coordinate
		// system will be different.
		if p == from /* || !peer.SeenCommonRootRecently() */ {
			continue
		}

		// Look up the coordinates of the peer, and the distance
		// across the tree to those coordinates.
		peerCoords := ann.PeerCoords()
		peerDist := int64(peerCoords.DistanceTo(f.Destination))
		switch {
		case peerDist == 0 || f.Destination.EqualTo(peerCoords):
			// The peer is the actual destination.
			return []*peer{p}

		case peerDist < bestDist:
			// The peer is closer to the destination.
			bestDist = peerDist
			newCandidate(p)

		default:
		}
	}

	// If we've got an eligible next peer, and it doesn't create a
	// routing loop by sending the frame back where it came from,
	// then return it.
	return candidates[canlength:]
}

func (s *state) _handleTreeAnnouncement(p *peer, f *types.Frame) error {
	var newUpdate types.SwitchAnnouncement
	if _, err := newUpdate.UnmarshalBinary(f.Payload); err != nil {
		return fmt.Errorf("update unmarshal failed: %w", err)
	}

	if len(newUpdate.Signatures) == 0 {
		// The update must have signatures.
		return fmt.Errorf("update has no signatures")
	}

	sigs := make(map[string]struct{})
	for index, sig := range newUpdate.Signatures {
		if index == 0 && sig.PublicKey != newUpdate.RootPublicKey {
			// The first signature in the announcement must be from the
			// key that claims to be the root.
			return fmt.Errorf("update first signature doesn't match root key")
		}
		if sig.Hop == 0 {
			// None of the hops in the update should have a port number of 0
			// as this would imply that another node has sent their router
			// port, which is impossible. We'll therefore reject any update
			// that tries to do that.
			return fmt.Errorf("update contains invalid 0 hop")
		}
		if index == len(newUpdate.Signatures)-1 && p.public != sig.PublicKey {
			// The last signature in the announcement must be from the
			// direct peer. If it isn't then it sounds like someone is
			// trying to replay someone else's announcement to us.
			return fmt.Errorf("update last signature is not from direct peer")
		}
		pk := hex.EncodeToString(sig.PublicKey[:])
		if _, ok := sigs[pk]; ok {
			// One of the signatures has appeared in the update more than
			// once, which would suggest that there's a loop somewhere.
			return fmt.Errorf("update contains routing loop")
		}
		sigs[pk] = struct{}{}
	}

	lastParent := s._parent
	lastParentUpdate := s._rootAnnouncement()
	var lastCoords types.SwitchPorts
	if lastParentUpdate != nil {
		lastCoords = lastParentUpdate.Coords()
	}

	s._ordering++
	s._announcements[p] = &rootAnnouncementWithTime{
		SwitchAnnouncement: newUpdate,
		receiveTime:        time.Now(),
		receiveOrder:       s._ordering,
	}

	rootKey := s.r.public
	rootTime := time.Now()
	if lastParentUpdate != nil {
		rootKey = lastParentUpdate.RootPublicKey
		rootTime = lastParentUpdate.receiveTime
	}

	// We don't think we're the root, we have chosen a parent already and have
	// a previous update from them. Let's see how it compares.
	rootPublicKeyDelta := newUpdate.RootPublicKey.CompareTo(rootKey)

	var foundNewParent bool
	switch {
	case time.Since(rootTime) >= announcementTimeout:
		// We haven't seen our previous parent recently, something is wrong.
		foundNewParent = s._selectNewParent()

	case rootPublicKeyDelta > 0:
		// The update has come from a better root key, so this is a better parent!
		s._parent = p

	case p != s._parent && rootPublicKeyDelta < 0:
		// The update has come from a worse root key, let's send our announcement back
		// to this peer in the hope that they choose us as their parent instead.
		s._sendTreeAnnouncementToPeer(s._rootAnnouncement(), p)

	case p == s._parent && rootPublicKeyDelta < 0:
		// Our parent's key has suddenly got weaker, which is suspicious. Try selecting
		// a new parent.
		foundNewParent = s._selectNewParent()

	case p == s._parent && newUpdate.Sequence > lastParentUpdate.Sequence:
		// Our parent sent us an update from the same root key but with a better sequence
		// number, so repeat it to our peers.
		s._sendTreeAnnouncements()
		return nil
	}

	if s._parent == nil {
		// We are the root apparently.
		if lastParent != nil {
			s._becomeRoot()
		}
		return nil
	}

	latestParentUpdate := s._announcements[s._parent]
	latestCoords := latestParentUpdate.Coords()

	switch {
	case !lastCoords.EqualTo(latestCoords): // the parent changed
		s.r.log.Println("New coords:", latestCoords)
		fallthrough
	case foundNewParent && s._parent != lastParent:
		s._sendTreeAnnouncements()
	}

	if rootKey != latestParentUpdate.RootPublicKey {
		s._rootChanged(latestParentUpdate.RootPublicKey)
	}

	return nil
}

func (s *state) _selectNewParent() bool {
	bestKey := s.r.public
	bestSeq := types.Varu64(0)
	bestOrder := uint64(math.MaxUint64)
	var bestPeer *peer

	for peer, ann := range s._announcements {
		if ann == nil || time.Since(ann.receiveTime) >= announcementTimeout {
			continue
		}
		accept := func() {
			bestKey = ann.RootPublicKey
			bestPeer = peer
			bestOrder = ann.receiveOrder
			bestSeq = ann.Sequence
		}
		keyDelta := ann.RootPublicKey.CompareTo(bestKey)
		switch {
		case ann.IsLoopOrChildOf(s.r.public):
			// ignore our children or loopy announcements
		case keyDelta > 0:
			accept()
		case keyDelta < 0:
			// ignore weaker root keys
		case ann.Sequence > bestSeq:
			accept()
		case ann.Sequence < bestSeq:
			// ignore lower sequence numbers
		case ann.receiveOrder < bestOrder:
			accept()
		}
	}

	if bestPeer != nil {
		s._parent = bestPeer
		return true
	}

	// No suitable other peer was found, so we'll just become the root
	// and hope that one of our peers corrects us if it matters.
	s._becomeRoot()
	return false
}

func (s *state) _ancestors() ([]types.PublicKey, *peer) {
	root, parent := s._rootAnnouncement(), s._parent
	if parent == nil {
		return nil, nil
	}
	ancestors := make([]types.PublicKey, 0, 1+len(root.Signatures))
	if len(root.Signatures) == 0 {
		return ancestors, parent
	}
	for _, sig := range root.Signatures {
		ancestors = append(ancestors, sig.PublicKey)
	}
	return ancestors, parent
}
