package router

import (
	"net"

	"github.com/Arceliar/phony"
	"github.com/matrix-org/pinecone/types"
)

type Simulator interface {
	ReportDistance(a, b string, l int64)
	LookupCoords(string) (types.SwitchPorts, error)
	LookupNodeID(types.SwitchPorts) (string, error)
	LookupPublicKey(types.PublicKey) (string, error)
	ReportNewLink(net.Conn, types.PublicKey, types.PublicKey)
	ReportDeadLink(types.PublicKey, types.PublicKey)
}

func (r *Router) Coords() types.SwitchPorts {
	return r.state.coords()
}

func (r *Router) RootPublicKey() types.PublicKey {
	var ann *rootAnnouncementWithTime
	phony.Block(r.state, func() {
		ann = r.state._rootAnnouncement()
	})
	if ann != nil {
		return ann.RootPublicKey
	}
	return r.public
}

func (r *Router) ParentPublicKey() types.PublicKey {
	var parent *peer
	phony.Block(r.state, func() {
		parent = r.state._parent
	})
	if parent == nil {
		return r.public
	}
	return parent.public
}

func (r *Router) IsRoot() bool {
	return r.RootPublicKey().EqualTo(r.public)
}

func (r *Router) DHTInfo() (asc, desc *virtualSnakeEntry, table map[virtualSnakeIndex]virtualSnakeEntry, stale int) {
	table = map[virtualSnakeIndex]virtualSnakeEntry{}
	phony.Block(r.state, func() {
		ann := r.state._rootAnnouncement()
		asc = r.state._ascending
		desc = r.state._descending
		for k, v := range r.state._table {
			table[k] = *v
			switch {
			case v.RootPublicKey != ann.RootPublicKey:
				fallthrough
			case v.RootSequence != ann.Sequence:
				stale++
			}
		}
	})
	return
}

func (r *Router) Descending() (*types.PublicKey, *types.VirtualSnakePathID) {
	_, desc, _, _ := r.DHTInfo()
	if desc == nil {
		return nil, nil
	}
	return &desc.PublicKey, &desc.PathID
}

func (r *Router) Ascending() (*types.PublicKey, *types.VirtualSnakePathID) {
	asc, _, _, _ := r.DHTInfo()
	if asc == nil {
		return nil, nil
	}
	return &asc.PublicKey, &asc.PathID
}

type PeerInfo struct {
	Port          int
	PublicKey     string
	RootPublicKey string
	PeerType      int
	Zone          string
}

func (r *Router) Peers() []PeerInfo {
	peers := make([]PeerInfo, 0, PortCount)
	phony.Block(r.state, func() {
		for _, p := range r.state._peers {
			if p == nil || !p.started.Load() {
				continue
			}
			info := PeerInfo{
				Port:      int(p.port),
				PeerType:  p.peertype,
				PublicKey: p.public.String(),
				Zone:      p.zone,
			}
			if r.state._announcements[p] != nil {
				info.RootPublicKey = r.state._announcements[p].RootPublicKey.String()
			}
			if info.RootPublicKey == "" {
				info.RootPublicKey = r.public.String()
			}
			peers = append(peers, info)
		}
	})
	return peers
}
