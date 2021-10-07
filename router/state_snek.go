package router

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"time"

	"github.com/matrix-org/pinecone/types"
	"github.com/matrix-org/pinecone/util"
)

const virtualSnakeMaintainInterval = time.Second
const virtualSnakeNeighExpiryPeriod = time.Hour

type virtualSnakeTable map[virtualSnakeIndex]*virtualSnakeEntry

type virtualSnakeIndex struct {
	PublicKey types.PublicKey
	PathID    types.VirtualSnakePathID
}

type virtualSnakeEntry struct {
	virtualSnakeIndex
	Source        *peer
	Destination   *peer
	LastSeen      time.Time
	RootPublicKey types.PublicKey
	RootSequence  types.Varu64
}

func (e *virtualSnakeEntry) valid() bool {
	return time.Since(e.LastSeen) < virtualSnakeNeighExpiryPeriod
}

func (s *state) _maintainSnake() {
	select {
	case <-s.r.context.Done():
		return
	default:
		defer s._maintainSnakeIn(virtualSnakeMaintainInterval)
	}

	rootAnn := s._rootAnnouncement()
	canBootstrap := s._parent != nil && rootAnn.RootPublicKey != s.r.public
	willBootstrap := false

	if asc := s._ascending; asc != nil {
		switch {
		case !asc.valid():
			s._sendTeardownForExistingPath(s.r.local, asc.PublicKey, asc.PathID, true)
			fallthrough
		case asc.RootPublicKey != rootAnn.RootPublicKey || asc.RootSequence != rootAnn.Sequence:
			willBootstrap = canBootstrap
		}
	} else {
		willBootstrap = canBootstrap
	}

	if desc := s._descending; desc != nil && !desc.valid() {
		s._sendTeardownForExistingPath(s.r.local, desc.PublicKey, desc.PathID, false)
	}

	// Send bootstrap messages into the network. Ordinarily we
	// would only want to do this when starting up or after a
	// predefined interval, but for now we'll continue to send
	// them on a regular interval until we can derive some better
	// connection state.
	if willBootstrap {
		s._bootstrapNow()
	}
}

func (s *state) _bootstrapNow() {
	if s._parent == nil {
		return
	}
	ann := s._rootAnnouncement()
	payload := make([]byte, 8+ed25519.PublicKeySize+ann.Sequence.Length())
	bootstrap := types.VirtualSnakeBootstrap{
		RootPublicKey: ann.RootPublicKey,
		RootSequence:  ann.Sequence,
	}
	if _, err := rand.Read(bootstrap.PathID[:]); err != nil {
		return
	}
	if _, err := bootstrap.MarshalBinary(payload[:]); err != nil {
		return
	}
	send := &types.Frame{
		Type:           types.TypeVirtualSnakeBootstrap,
		DestinationKey: s.r.public,
		Source:         s._coords(),
		Payload:        payload[:],
	}
	if p := s._nextHopsSNEK(s.r.local, send, true); p != nil && p.proto != nil {
		p.proto.push(send)
	}
}

func (s *state) _nextHopsSNEK(from *peer, rx *types.Frame, bootstrap bool) *peer {
	destKey := rx.DestinationKey
	if !bootstrap && s.r.public.EqualTo(destKey) {
		return s.r.local
	}
	rootAnn := s._rootAnnouncement()
	ancestors, parentPort := s._ancestors()
	bestKey := s.r.public
	var bestPeer *peer
	if !bootstrap {
		bestPeer = s.r.local
	}
	newCandidate := func(key types.PublicKey, p *peer) {
		bestKey, bestPeer = key, p
	}
	newCheckedCandidate := func(candidate types.PublicKey, p *peer) {
		switch {
		case !bootstrap && candidate.EqualTo(destKey) && !bestKey.EqualTo(destKey):
			newCandidate(candidate, p)
		case util.DHTOrdered(destKey, candidate, bestKey):
			newCandidate(candidate, p)
		}
	}

	// Check if we can use the path to the root via our parent as a starting point
	if parentPort != nil && parentPort.started.Load() {
		switch {
		case bootstrap && bestKey.EqualTo(destKey):
			// Bootstraps always start working towards the root so that
			// they go somewhere rather than getting stuck
			fallthrough
		case util.DHTOrdered(bestKey, destKey, rootAnn.RootPublicKey):
			// The destination key is higher than our own key, so
			// start using the path to the root as the first candidate
			newCandidate(rootAnn.RootPublicKey, parentPort)
		}

		// Check our direct ancestors
		// bestKey <= destKey < rootKey
		for _, ancestor := range ancestors {
			newCheckedCandidate(ancestor, parentPort)
		}
	}

	// Check our direct peers ancestors
	for p, ann := range s._announcements {
		if !p.started.Load() {
			continue
		}
		for _, hop := range ann.Signatures {
			newCheckedCandidate(hop.PublicKey, p)
		}
	}

	// Check our direct peers
	for p := range s._announcements {
		if !p.started.Load() {
			continue
		}
		if peerKey := p.public; bestKey.EqualTo(peerKey) {
			// We've seen this key already, either as one of our ancestors
			// or as an ancestor of one of our peers, but it turns out we
			// are directly peered with that node, so use the more direct
			// path instead
			newCandidate(peerKey, p)
		}
	}

	// Check our DHT entries
	for _, entry := range s._table {
		if !entry.Source.started.Load() || !entry.valid() {
			continue
		}
		newCheckedCandidate(entry.PublicKey, entry.Source)
	}

	return bestPeer
}

func (s *state) _handleBootstrap(from *peer, rx *types.Frame) error {
	if rx.DestinationKey.EqualTo(s.r.public) {
		return nil
	}
	// Unmarshal the bootstrap.
	var bootstrap types.VirtualSnakeBootstrap
	_, err := bootstrap.UnmarshalBinary(rx.Payload)
	if err != nil {
		return fmt.Errorf("bootstrap.UnmarshalBinary: %w", err)
	}
	root := s._rootAnnouncement()
	bootstrapACK := types.VirtualSnakeBootstrapACK{
		PathID:        bootstrap.PathID,
		RootPublicKey: root.RootPublicKey,
		RootSequence:  root.Sequence,
	}
	buf := make([]byte, 8+ed25519.PublicKeySize+root.Sequence.Length())
	if _, err := bootstrapACK.MarshalBinary(buf[:]); err != nil {
		return fmt.Errorf("bootstrapACK.MarshalBinary: %w", err)
	}
	acknowledge := false
	desc := s._descending
	switch {
	case rx.SourceKey.EqualTo(s.r.public):
		// We received a bootstrap from ourselves. This shouldn't happen,
		// so either another node has forwarded it to us incorrectly, or
		// a routing loop has occurred somewhere. Don't act on the bootstrap
		// in that case.
	case !bootstrap.RootPublicKey.EqualTo(root.RootPublicKey) || bootstrap.RootSequence != root.Sequence:
		// The root or sequence don't match so we won't act on the bootstrap.
	case desc != nil && desc.PublicKey.EqualTo(rx.DestinationKey):
		// We've received another bootstrap from our direct descending node.
		// Send back an acknowledgement as this is OK.
		acknowledge = true
	case desc != nil && !desc.valid():
		// We already have a direct descending node, but we haven't seen it
		// recently, so it's quite possible that it has disappeared. We'll
		// therefore handle this bootstrap instead. If the original node comes
		// back later and is closer to us then we'll end up using it again.
		acknowledge = true
	case desc == nil && util.LessThan(rx.DestinationKey, s.r.public):
		// We don't know about a descending node and at the moment we don't know
		// any better candidates, so we'll accept a bootstrap from a node with a
		// key lower than ours (so that it matches descending order).
		acknowledge = true
	case desc != nil && util.DHTOrdered(desc.PublicKey, rx.DestinationKey, s.r.public):
		// We know about a descending node already but it turns out that this
		// new node that we've received a bootstrap from is actually closer to
		// us than the previous node. We'll update our record to use the new
		// node instead and then send back a bootstrap ACK.
		acknowledge = true
	default:
		// The bootstrap conditions weren't met. This might just be because
		// there's a node out there that hasn't converged to a closer node
		// yet, so we'll just ignore the bootstrap.
	}
	if acknowledge {
		send := &types.Frame{
			Destination:    rx.Source,
			DestinationKey: rx.DestinationKey,
			Source:         s._coords(),
			SourceKey:      s.r.public,
			Type:           types.TypeVirtualSnakeBootstrapACK,
			Payload:        buf,
		}
		if p := s._nextHopsTree(s.r.local, send); p != nil && p.proto != nil {
			p.proto.push(send)
		}
		return nil
	}
	return nil
}

func (s *state) _handleBootstrapACK(from *peer, rx *types.Frame) error {
	// Unmarshal the bootstrap ACK.
	var bootstrapACK types.VirtualSnakeBootstrapACK
	_, err := bootstrapACK.UnmarshalBinary(rx.Payload)
	if err != nil {
		return fmt.Errorf("bootstrapACK.UnmarshalBinary: %w", err)
	}
	index := virtualSnakeIndex{
		PublicKey: rx.SourceKey,
		PathID:    bootstrapACK.PathID,
	}
	root := s._rootAnnouncement()
	update := false
	asc := s._ascending
	switch {
	case rx.SourceKey.EqualTo(s.r.public):
		// We received a bootstrap ACK from ourselves. This shouldn't happen,
		// so either another node has forwarded it to us incorrectly, or
		// a routing loop has occurred somewhere. Don't act on the bootstrap
		// in that case.
	case !bootstrapACK.RootPublicKey.EqualTo(root.RootPublicKey) || bootstrapACK.RootSequence != root.Sequence:
		// The root or sequence don't match so we won't act on the bootstrap.
	case asc != nil && asc.PublicKey.EqualTo(rx.SourceKey) && asc.PathID != bootstrapACK.PathID:
		// We've received another bootstrap ACK from our direct ascending node.
		// Just refresh the record and then send a new path setup message to
		// that node.
		update = true
	case asc != nil && !asc.valid():
		// We already have a direct ascending node, but we haven't seen it
		// recently, so it's quite possible that it has disappeared. We'll
		// therefore handle this bootstrap ACK instead. If the original node comes
		// back later and is closer to us then we'll end up using it again.
		update = true
	case asc == nil && util.LessThan(s.r.public, rx.SourceKey):
		// We don't know about an ascending node and at the moment we don't know
		// any better candidates, so we'll accept a bootstrap ACK from a node with a
		// key higher than ours (so that it matches descending order).
		update = true
	case asc != nil && util.DHTOrdered(s.r.public, rx.SourceKey, asc.PublicKey):
		// We know about an ascending node already but it turns out that this
		// new node that we've received a bootstrap from is actually closer to
		// us than the previous node. We'll update our record to use the new
		// node instead and then send a new path setup message to it.
		update = true
	default:
		// The bootstrap ACK conditions weren't met. This might just be because
		// there's a node out there that hasn't converged to a closer node
		// yet, so we'll just ignore the acknowledgement.
	}
	if !update {
		return nil
	}
	if asc != nil {
		// Remote side is responsible for clearing up the replaced path, but
		// we do want to make sure we don't have any old paths to other nodes
		// that *aren't* the new ascending node lying around.
		s._sendTeardownForExistingPath(s.r.local, asc.PublicKey, asc.PathID, true)
		if s._ascending != nil {
			panic("should have cleaned up ascending node")
		}
	}
	setup := types.VirtualSnakeSetup{ // nolint:gosimple
		PathID:        bootstrapACK.PathID,
		RootPublicKey: root.RootPublicKey,
		RootSequence:  root.Sequence,
	}
	buf := make([]byte, 8+ed25519.PublicKeySize+root.Sequence.Length())
	if _, err := setup.MarshalBinary(buf[:]); err != nil {
		return fmt.Errorf("setup.MarshalBinary: %w", err)
	}
	send := &types.Frame{
		Destination:    rx.Source,
		DestinationKey: rx.SourceKey, // the other end of the path
		SourceKey:      s.r.public,   // our source key
		Type:           types.TypeVirtualSnakeSetup,
		Payload:        buf,
	}
	nexthop := s.r.state._nextHopsTree(s.r.local, send)
	if nexthop == nil || nexthop == s.r.local || nexthop.proto == nil {
		return fmt.Errorf("no next-hop")
	}
	if !nexthop.proto.push(send) {
		return fmt.Errorf("failed to send setup")
	}
	entry := &virtualSnakeEntry{
		virtualSnakeIndex: index,
		Source:            nexthop,
		Destination:       s.r.local,
		LastSeen:          time.Now(),
		RootPublicKey:     bootstrapACK.RootPublicKey,
		RootSequence:      bootstrapACK.RootSequence,
	}
	s._ascending = entry
	return nil
}

func (s *state) _handleSetup(from *peer, rx *types.Frame, nexthop *peer) error {
	root := s._rootAnnouncement()

	// Unmarshal the setup.
	var setup types.VirtualSnakeSetup
	if _, err := setup.UnmarshalBinary(rx.Payload); err != nil {
		return fmt.Errorf("setup.UnmarshalBinary: %w", err)
	}
	index := virtualSnakeIndex{
		PublicKey: rx.SourceKey,
		PathID:    setup.PathID,
	}

	if _, ok := s._table[virtualSnakeIndex{rx.SourceKey, setup.PathID}]; ok {
		s._sendTeardownForExistingPath(s.r.local, rx.SourceKey, setup.PathID, false) // first call fixes routing table
		if _, ok := s._table[virtualSnakeIndex{rx.SourceKey, setup.PathID}]; ok {
			panic("should have cleaned up duplicate path in routing table")
		}
		s._sendTeardownForRejectedPath(rx.SourceKey, setup.PathID, from) // second call sends back to origin
		return fmt.Errorf("setup is a duplicate")
	}

	// If we're at the destination of the setup then update our predecessor
	// with information from the bootstrap.
	if nexthop == s.r.local && rx.DestinationKey.EqualTo(s.r.public) {
		update := false
		desc := s._descending
		switch {
		case rx.SourceKey.EqualTo(s.r.public):
			// We received a bootstrap from ourselves. This shouldn't happen,
			// so either another node has forwarded it to us incorrectly, or
			// a routing loop has occurred somewhere. Don't act on the bootstrap
			// in that case.
		case !setup.RootPublicKey.EqualTo(root.RootPublicKey) || setup.RootSequence != root.Sequence:
			// The root or sequence don't match so we won't act on the setup
			// and send a teardown back to the sender.
		case desc != nil && desc.PublicKey.EqualTo(rx.SourceKey):
			// We've received another bootstrap from our direct descending node.
			// Just refresh the record and then send back an acknowledgement.
			update = true
		case desc != nil && time.Since(desc.LastSeen) >= virtualSnakeNeighExpiryPeriod:
			// We already have a direct descending node, but we haven't seen it
			// recently, so it's quite possible that it has disappeared. We'll
			// therefore handle this bootstrap instead. If the original node comes
			// back later and is closer to us then we'll end up using it again.
			update = true
		case desc == nil && util.LessThan(rx.SourceKey, s.r.public):
			// We don't know about a descending node and at the moment we don't know
			// any better candidates, so we'll accept a bootstrap from a node with a
			// key lower than ours (so that it matches descending order).
			update = true
		case desc != nil && util.DHTOrdered(desc.PublicKey, rx.SourceKey, s.r.public):
			// We know about a descending node already but it turns out that this
			// new node that we've received a bootstrap from is actually closer to
			// us than the previous node. We'll update our record to use the new
			// node instead and then send back a bootstrap ACK.
			update = true
		default:
			// The bootstrap conditions weren't met. This might just be because
			// there's a node out there that hasn't converged to a closer node
			// yet, so we'll just ignore the bootstrap.
		}
		if !update {
			s._sendTeardownForRejectedPath(rx.SourceKey, setup.PathID, from)
			return nil
		}
		if desc != nil {
			// Tear down the previous path, if there was one.
			s._sendTeardownForExistingPath(s.r.local, desc.PublicKey, desc.PathID, false)
			if s._descending != nil {
				panic("should have cleaned up descending node")
			}
			if _, ok := s._table[virtualSnakeIndex{desc.PublicKey, desc.PathID}]; ok {
				panic("should have cleaned up descending entry in routing table")
			}
		}
		entry := &virtualSnakeEntry{
			virtualSnakeIndex: index,
			Source:            from,
			Destination:       s.r.local,
			LastSeen:          time.Now(),
			RootPublicKey:     setup.RootPublicKey,
			RootSequence:      setup.RootSequence,
		}
		s._table[index] = entry
		s._descending = entry
		return nil
	}
	// Try to forward the setup onto the next node first. If we
	// can't do that then there's no point in keeping the path.
	if nexthop == nil || nexthop == s.r.local || nexthop.proto == nil || !nexthop.proto.push(rx) {
		s._sendTeardownForRejectedPath(rx.SourceKey, setup.PathID, from)
		return fmt.Errorf("unable to forward setup packet (next-hop %s)", nexthop)
	}
	// Add a new routing table entry as we are intermediate to
	// the path.
	s._table[index] = &virtualSnakeEntry{
		virtualSnakeIndex: index,
		LastSeen:          time.Now(),
		RootPublicKey:     setup.RootPublicKey,
		RootSequence:      setup.RootSequence,
		Source:            from,    // node with lower of the two keys
		Destination:       nexthop, // node with higher of the two keys
	}
	return nil
}

func (s *state) _handleTeardown(from *peer, rx *types.Frame) ([]*peer, error) {
	if len(rx.Payload) < 8 {
		return nil, fmt.Errorf("payload too short")
	}
	var teardown types.VirtualSnakeTeardown
	if _, err := teardown.UnmarshalBinary(rx.Payload); err != nil {
		return nil, fmt.Errorf("teardown.UnmarshalBinary: %w", err)
	}
	return s._teardownPath(from, rx.DestinationKey, teardown.PathID), nil
}

func (s *state) _sendTeardownForRejectedPath(pathKey types.PublicKey, pathID types.VirtualSnakePathID, via *peer) {
	if _, ok := s._table[virtualSnakeIndex{pathKey, pathID}]; ok {
		panic("rejected path should not be in routing table")
	}
	if via != nil {
		via.proto.push(s._getTeardown(pathKey, pathID, false))
	}
}

func (s *state) _sendTeardownForExistingPath(from *peer, pathKey types.PublicKey, pathID types.VirtualSnakePathID, ascending bool) {
	frame := s._getTeardown(pathKey, pathID, ascending)
	for _, nexthop := range s._teardownPath(from, pathKey, pathID) {
		if nexthop != nil && nexthop.proto != nil {
			nexthop.proto.push(frame)
		}
	}
}

func (s *state) _getTeardown(pathKey types.PublicKey, pathID types.VirtualSnakePathID, ascending bool) *types.Frame {
	var payload [8]byte
	teardown := types.VirtualSnakeTeardown{
		PathID: pathID,
	}
	if _, err := teardown.MarshalBinary(payload[:]); err != nil {
		return nil
	}
	if ascending {
		// We're sending a teardown to our ascending node, so the teardown
		// needs to contain *our* key and not theirs, as we're the lower key.
		return &types.Frame{
			Type:           types.TypeVirtualSnakeTeardown,
			DestinationKey: s.r.public,
			Payload:        payload[:],
		}
	}
	return &types.Frame{
		Type:           types.TypeVirtualSnakeTeardown,
		DestinationKey: pathKey,
		Payload:        payload[:],
	}
}

func (s *state) _teardownPath(from *peer, pathKey types.PublicKey, pathID types.VirtualSnakePathID) []*peer {
	if asc := s._ascending; asc != nil && asc.PathID == pathID {
		switch {
		case from == s.r.local && asc.PublicKey.EqualTo(pathKey): // originated locally
			defer s._bootstrapNow()
			fallthrough
		case from == asc.Source && s.r.public.EqualTo(pathKey): // from network
			s._ascending = nil
			return []*peer{asc.Source}
		}
	}
	if desc := s._descending; desc != nil && desc.PublicKey.EqualTo(pathKey) && desc.PathID == pathID {
		switch {
		case from == desc.Source: // from network
			fallthrough
		case from == s.r.local: // originated locally
			s._descending = nil
			delete(s._table, virtualSnakeIndex{desc.PublicKey, desc.PathID})
			return []*peer{desc.Source}
		}
	}
	for k, v := range s._table {
		if k.PublicKey == pathKey && k.PathID == pathID {
			switch {
			case from == s.r.local: // happens when we're tearing down an existing duplicate path
				delete(s._table, k)
				return []*peer{v.Destination, v.Source}
			case from == v.Source: // from network, return the opposite direction
				delete(s._table, k)
				return []*peer{v.Destination}
			case from == v.Destination: // from network, return the opposite direction
				delete(s._table, k)
				return []*peer{v.Source}
			}
		}
	}
	return nil
}
