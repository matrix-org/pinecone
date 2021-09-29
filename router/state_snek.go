package router

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/matrix-org/pinecone/types"
	"github.com/matrix-org/pinecone/util"
)

const virtualSnakeNeighExpiryPeriod = time.Hour

type virtualSnakeTable map[virtualSnakeIndex]virtualSnakeEntry

type virtualSnakeIndex struct {
	PublicKey types.PublicKey
	PathID    types.VirtualSnakePathID
}

type virtualSnakeEntry struct {
	Source        *peer
	Destination   *peer
	LastSeen      time.Time
	RootPublicKey types.PublicKey
}

type virtualSnakeNeighbour struct {
	PublicKey     types.PublicKey
	Port          *peer
	LastSeen      time.Time
	Coords        types.SwitchPorts
	PathID        types.VirtualSnakePathID
	RootPublicKey types.PublicKey
}

func (s *state) _maintain() {
	select {
	case <-s.r.context.Done():
		return
	default:
	}

	rootKey := s._rootAnnouncement().RootPublicKey
	canBootstrap := rootKey != s.r.public
	willBootstrap := false

	if asc := s._ascending; asc != nil {
		switch {
		case time.Since(asc.LastSeen) > virtualSnakeNeighExpiryPeriod:
			s._teardownPath(s.r.public, asc.PathID, asc.Port, true)
			willBootstrap = canBootstrap
		case asc.RootPublicKey != rootKey:
			s._teardownPath(s.r.public, asc.PathID, asc.Port, true)
			willBootstrap = canBootstrap
		}
	} else {
		willBootstrap = canBootstrap
	}

	if desc := s._descending; desc != nil {
		switch {
		case time.Since(desc.LastSeen) > virtualSnakeNeighExpiryPeriod:
			s._teardownPath(desc.PublicKey, desc.PathID, desc.Port, false)
		case !desc.RootPublicKey.EqualTo(rootKey):
			s._teardownPath(desc.PublicKey, desc.PathID, desc.Port, false)
		}
	}

	// Send bootstrap messages into the network. Ordinarily we
	// would only want to do this when starting up or after a
	// predefined interval, but for now we'll continue to send
	// them on a regular interval until we can derive some better
	// connection state.
	if willBootstrap {
		s._bootstrapNow()
	}

	time.AfterFunc(time.Second, func() {
		s.Act(s, s._maintain)
	})
}

func (s *state) _bootstrapNow() {
	if len(s.r.peers()) <= 1 || s._parent == nil {
		return
	}
	var payload [8 + ed25519.PublicKeySize]byte
	bootstrap := types.VirtualSnakeBootstrap{
		RootPublicKey: s._rootAnnouncement().RootPublicKey,
	} // nolint:gosimple
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
	for _, p := range s._nextHopsSNEK(s.r.local, send, true) {
		if p.proto.push(send) {
			return
		}
	}
}

func (s *state) _teardownPath(pk types.PublicKey, pathID types.VirtualSnakePathID, via *peer, ascending bool) {
	var payload [9]byte
	teardown := types.VirtualSnakeTeardown{ // nolint:gosimple
		PathID:    pathID,
		Ascending: ascending,
	}
	if _, err := teardown.MarshalBinary(payload[:]); err != nil {
		return
	}
	frame := &types.Frame{
		Type:           types.TypeVirtualSnakeTeardown,
		SourceKey:      s.r.public,
		DestinationKey: pk,
		Payload:        payload[:],
	}
	if err := s._handleTeardown(s.r.local, frame); err != nil {
		s.r.log.Println("Failed to handle teardown:", err)
		return
	}
	if via != nil {
		via.proto.push(frame)
	}
}

func (s *state) _nextHopsSNEK(from *peer, rx *types.Frame, bootstrap bool) []*peer {
	destKey := rx.DestinationKey
	if !bootstrap && s.r.public.EqualTo(destKey) {
		return []*peer{s.r.local}
	}
	rootKey := s._rootAnnouncement().RootPublicKey
	ancestors, parentPort := s._ancestors()
	if len(ancestors) > 0 {
		rootKey = ancestors[0]
		ancestors = ancestors[1:]
	}
	bestKey := s.r.public
	bestPeer := s.r.local
	var candidates []*peer
	var canlength int
	if !bootstrap {
		candidates, canlength = make([]*peer, PortCount), PortCount
	}
	newCandidate := func(key types.PublicKey, p *peer) bool {
		if !bootstrap && p != bestPeer {
			canlength--
			candidates[canlength] = p
		}
		bestKey, bestPeer = key, p
		return true
	}
	newCheckedCandidate := func(candidate types.PublicKey, p *peer) bool {
		switch {
		case !bootstrap && candidate.EqualTo(destKey) && !bestKey.EqualTo(destKey):
			return newCandidate(candidate, p)
		case util.DHTOrdered(destKey, candidate, bestKey):
			return newCandidate(candidate, p)
		}
		return false
	}

	// Check if we can use the path to the root as a starting point
	switch {
	case bootstrap && bestKey.EqualTo(destKey):
		// Bootstraps always start working towards the root so that
		// they go somewhere rather than getting stuck
		newCandidate(rootKey, parentPort)
	case destKey.EqualTo(rootKey):
		// The destination is actually the root node itself
		newCandidate(rootKey, parentPort)
	case util.DHTOrdered(bestKey, destKey, rootKey):
		// The destination key is higher than our own key, so
		// start using the path to the root as the first candidate
		newCandidate(rootKey, parentPort)
	}

	// Check our direct ancestors
	// bestKey <= destKey < rootKey
	for _, ancestor := range ancestors {
		newCheckedCandidate(ancestor, parentPort)
	}

	peers := s.r.peers()

	// Check our direct peers ancestors
	for _, p := range peers {
		if p == nil {
			continue
		}
		peerAnn := s._announcements[p]
		if peerAnn == nil {
			continue
		}
		newCheckedCandidate(peerAnn.RootPublicKey, p)
		for _, hop := range peerAnn.Signatures {
			newCheckedCandidate(hop.PublicKey, p)
		}
	}

	// Check our DHT entries
	for dhtKey, entry := range s._table {
		switch {
		//case !entry.Valid():
		//	continue
		default:
			newCheckedCandidate(dhtKey.PublicKey, entry.Source)
		}
	}

	// Check our direct peers
	for _, p := range peers {
		if p == nil {
			continue
		}
		if s._announcements[p] == nil {
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

	// Return the candidate ports
	if bootstrap {
		return []*peer{bestPeer}
	} else {
		return candidates[canlength:]
	}
}

func (s *state) _rootChanged(root types.PublicKey) {
	if asc := s._ascending; asc == nil || (asc != nil && !asc.RootPublicKey.EqualTo(root)) {
		if asc != nil {
			s._teardownPath(s.r.public, asc.PathID, asc.Port, true)
		}
		if root != s.r.public {
			s._bootstrapNow() // TODO: in one second
		}
	}
	if desc := s._descending; desc != nil && !desc.RootPublicKey.EqualTo(root) {
		s._teardownPath(desc.PublicKey, desc.PathID, desc.Port, false)
	}
	teardown := map[virtualSnakeIndex]virtualSnakeEntry{}
	for k, v := range s._table {
		if !v.RootPublicKey.EqualTo(v.RootPublicKey) {
			teardown[k] = v
		}
	}
	for k, v := range teardown {
		_, _ = k, v
		if v.Source != nil {
			s._teardownPath(k.PublicKey, k.PathID, v.Source, false)
		}
		if v.Destination != nil {
			s._teardownPath(k.PublicKey, k.PathID, v.Destination, false)
		}
	}
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
	rootKey := s._rootAnnouncement().RootPublicKey
	if !bootstrap.RootPublicKey.EqualTo(rootKey) {
		return nil
	}
	bootstrapACK := types.VirtualSnakeBootstrapACK{ // nolint:gosimple
		PathID:        bootstrap.PathID,
		RootPublicKey: rootKey,
	}
	var buf [8 + ed25519.PublicKeySize]byte
	if _, err := bootstrapACK.MarshalBinary(buf[:]); err != nil {
		return fmt.Errorf("bootstrapACK.MarshalBinary: %w", err)
	}
	send := &types.Frame{
		Destination:    rx.Source,
		DestinationKey: rx.DestinationKey,
		Source:         s._coords(),
		SourceKey:      s.r.public,
		Type:           types.TypeVirtualSnakeBootstrapACK,
		Payload:        buf[:],
	}
	for _, p := range s._nextHopsTree(s.r.local, send) {
		if p.proto.push(send) {
			return nil
		}
	}
	return fmt.Errorf("failed to send bootstrap ACK to a peer")
}

func (s *state) _handleBootstrapACK(from *peer, rx *types.Frame) error {
	// Unmarshal the bootstrap ACK.
	var bootstrapACK types.VirtualSnakeBootstrapACK
	_, err := bootstrapACK.UnmarshalBinary(rx.Payload)
	if err != nil {
		return fmt.Errorf("bootstrapACK.UnmarshalBinary: %w", err)
	}
	if !bootstrapACK.RootPublicKey.EqualTo(s._rootAnnouncement().RootPublicKey) {
		return nil
	}
	update := false
	asc := s._ascending
	switch {
	case rx.SourceKey.EqualTo(s.r.public):
		// We received a bootstrap ACK from ourselves. This shouldn't happen,
		// so either another node has forwarded it to us incorrectly, or
		// a routing loop has occurred somewhere. Don't act on the bootstrap
		// in that case.
	case asc != nil && asc.PublicKey.EqualTo(rx.SourceKey) && asc.PathID != bootstrapACK.PathID:
		// We've received another bootstrap ACK from our direct ascending node.
		// Just refresh the record and then send a new path setup message to
		// that node.
		update = true
	case asc != nil && time.Since(asc.LastSeen) >= virtualSnakeNeighExpiryPeriod:
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
	if update {
		if asc != nil && !rx.SourceKey.EqualTo(asc.PublicKey) {
			// Remote side is responsible for clearing up the replaced path, but
			// we do want to make sure we don't have any old paths to other nodes
			// that *aren't* the new ascending node lying around.
			teardown := map[virtualSnakeIndex]virtualSnakeEntry{}
			for k, v := range s._table {
				if v.Source == nil && !k.PublicKey.EqualTo(rx.SourceKey) {
					teardown[k] = v
				}
			}
			for k, v := range teardown {
				_, _ = k, v
				// TODO: t.teardownPath(k.PublicKey, k.PathID, v.Destination, true, fmt.Errorf("replacing ascending"))
			}
		}
		s._ascending = &virtualSnakeNeighbour{
			PublicKey:     rx.SourceKey,
			Port:          from,
			LastSeen:      time.Now(),
			Coords:        rx.Source,
			PathID:        bootstrapACK.PathID,
			RootPublicKey: bootstrapACK.RootPublicKey,
		}
		setup := types.VirtualSnakeSetup{ // nolint:gosimple
			PathID:        bootstrapACK.PathID,
			RootPublicKey: bootstrapACK.RootPublicKey,
		}
		var buf [8 + ed25519.PublicKeySize]byte
		if _, err := setup.MarshalBinary(buf[:]); err != nil {
			return fmt.Errorf("setup.MarshalBinary: %w", err)
		}
		ts, err := util.SignedTimestamp(s.r.private)
		if err != nil {
			return fmt.Errorf("util.SignedTimestamp: %w", err)
		}
		send := &types.Frame{
			Destination:    rx.Source,
			DestinationKey: rx.SourceKey,
			SourceKey:      s.r.public,
			Type:           types.TypeVirtualSnakeSetup,
			Payload:        append(buf[:], ts...),
		}
		for _, p := range s._nextHopsTree(s.r.local, send) {
			if p.proto.push(send) {
				return nil
			}
		}
		return fmt.Errorf("failed to send bootstrap ACK to a peer")
	}
	return nil
}

func (s *state) _handleSetup(from *peer, rx *types.Frame) error {
	// Unmarshal the setup.
	var setup types.VirtualSnakeSetup
	if _, err := setup.UnmarshalBinary(rx.Payload); err != nil {
		return fmt.Errorf("setup.UnmarshalBinary: %w", err)
	}

	rootKey := s._rootAnnouncement().RootPublicKey
	if !setup.RootPublicKey.EqualTo(rootKey) {
		s._teardownPath(rx.SourceKey, setup.PathID, from, false)
		return nil
	}

	// Did the setup hit a dead end on the way to the ascending node?
	nextHops := s._nextHopsTree(s.r.local, rx)
	if len(nextHops) == 0 && !rx.DestinationKey.EqualTo(s.r.public) {
		s._teardownPath(rx.SourceKey, setup.PathID, from, false)
		return fmt.Errorf("setup for %q (%s) en route to %q %s hit dead end at %s", rx.SourceKey, hex.EncodeToString(setup.PathID[:]), rx.DestinationKey, rx.Destination, s._coords())
	}

	var addToRoutingTable bool

	// Is the setup a duplicate of one we already have in our table?
	if path, ok := s._table[virtualSnakeIndex{rx.SourceKey, setup.PathID}]; ok {
		s._teardownPath(rx.SourceKey, setup.PathID, from, false)
		if path.Destination != nil {
			s._teardownPath(rx.SourceKey, setup.PathID, path.Destination, false)
		}
		if path.Source != nil {
			s._teardownPath(rx.SourceKey, setup.PathID, path.Source, false)
		}
		return fmt.Errorf("setup is a duplicate")
	}

	// If we're at the destination of the setup then update our predecessor
	// with information from the bootstrap.
	if rx.DestinationKey.EqualTo(s.r.public) {
		update := false
		desc := s._descending
		switch {
		case rx.SourceKey.EqualTo(s.r.public):
			// We received a bootstrap from ourselves. This shouldn't happen,
			// so either another node has forwarded it to us incorrectly, or
			// a routing loop has occurred somewhere. Don't act on the bootstrap
			// in that case.
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
		if update {
			if desc != nil {
				// Tear down the previous path, if there was one.
				s._teardownPath(desc.PublicKey, desc.PathID, desc.Port, false)
			}
			s._descending = &virtualSnakeNeighbour{
				PublicKey:     rx.SourceKey,
				Port:          from,
				LastSeen:      time.Now(),
				Coords:        rx.Source,
				PathID:        setup.PathID,
				RootPublicKey: setup.RootPublicKey,
			}
			addToRoutingTable = true
		}
	} else {
		addToRoutingTable = true
	}

	if addToRoutingTable {
		// Add a new routing table entry.
		// TODO: The routing table needs to be bounded by size, so that we don't
		// exhaust available system memory trying to maintain network paths. To
		// bound the routing table safely, we may want to make sure that we have
		// a reasonable spread of routes across keyspace so that we don't create
		// any obvious routing holes.
		index := virtualSnakeIndex{
			PublicKey: rx.SourceKey,
			PathID:    setup.PathID,
		}
		entry := virtualSnakeEntry{
			LastSeen:      time.Now(),
			Source:        from,
			RootPublicKey: setup.RootPublicKey,
		}
		if len(nextHops) > 0 {
			entry.Destination = nextHops[0]
		}
		s._table[index] = entry

		return nil
	}

	s._teardownPath(rx.SourceKey, setup.PathID, from, false)
	return fmt.Errorf("no conditions met")
}

func (s *state) _handleTeardown(from *peer, rx *types.Frame) error {
	if len(rx.Payload) < 1 {
		return fmt.Errorf("payload too short")
	}
	var teardown types.VirtualSnakeTeardown
	if _, err := teardown.UnmarshalBinary(rx.Payload); err != nil {
		return fmt.Errorf("teardown.UnmarshalBinary: %w", err)
	}
	if desc := s._descending; desc != nil && desc.PublicKey.EqualTo(rx.DestinationKey) && desc.PathID == teardown.PathID {
		s._descending = nil
		s._teardownPath(desc.PublicKey, desc.PathID, desc.Port, false)
	}
	if asc := s._ascending; asc != nil && s.r.public.EqualTo(rx.DestinationKey) && asc.PathID == teardown.PathID {
		s._ascending = nil
		s._teardownPath(asc.PublicKey, asc.PathID, asc.Port, true)
		defer s._bootstrapNow()
	}
	for k, v := range s._table {
		if k.PublicKey == rx.DestinationKey && k.PathID == teardown.PathID {
			delete(s._table, k)
			switch {
			case from == v.Destination:
				s._teardownPath(rx.DestinationKey, teardown.PathID, v.Source, false)
			case from == v.Source:
				s._teardownPath(rx.DestinationKey, teardown.PathID, v.Destination, false)
			case from == s.r.local:
				if teardown.Ascending {
					s._teardownPath(rx.DestinationKey, teardown.PathID, v.Destination, true)
				} else {
					s._teardownPath(rx.DestinationKey, teardown.PathID, v.Source, false)
				}
			default:
				return fmt.Errorf("should be a candidate port but isn't")
			}
		}
	}
	return nil
}
