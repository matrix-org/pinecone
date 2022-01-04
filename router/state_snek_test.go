package router

import (
	"testing"
	"time"

	"github.com/matrix-org/pinecone/types"
	"go.uber.org/atomic"
)

func TestSNEKNextHopSelection(t *testing.T) {
	destUpKey := types.PublicKey{6}
	destDownKey := types.PublicKey{2}
	selfKey := types.PublicKey{4}
	rootKey := types.PublicKey{9}
	parentKey := types.PublicKey{3}
	higherKey := types.PublicKey{5}

	peers := []*peer{
		// self
		&peer{
			started: *atomic.NewBool(true),
			public:  selfKey,
		},
		// from
		&peer{
			started: *atomic.NewBool(true),
			public:  parentKey,
		},
		// assorted peers
		&peer{
			started: *atomic.NewBool(true),
			public:  destUpKey,
		},
		&peer{
			started: *atomic.NewBool(true),
			public:  destDownKey,
		},
	}

	root := types.Root{
		RootPublicKey: rootKey, RootSequence: 1,
	}

	selfAnn := rootAnnouncementWithTime{
		receiveTime:  time.Now(),
		receiveOrder: 1,
		SwitchAnnouncement: types.SwitchAnnouncement{
			Root:       root,
			Signatures: []types.SignatureWithHop{},
		},
	}
	parentAnn := rootAnnouncementWithTime{
		receiveTime:  time.Now(),
		receiveOrder: 1,
		SwitchAnnouncement: types.SwitchAnnouncement{
			Root:       root,
			Signatures: []types.SignatureWithHop{},
		},
	}
	knowsDestUpAnn := rootAnnouncementWithTime{
		receiveTime:  time.Now(),
		receiveOrder: 1,
		SwitchAnnouncement: types.SwitchAnnouncement{
			Root:       root,
			Signatures: []types.SignatureWithHop{types.SignatureWithHop{PublicKey: destUpKey}},
		},
	}
	knowsHigherAnn := rootAnnouncementWithTime{
		receiveTime:  time.Now(),
		receiveOrder: 1,
		SwitchAnnouncement: types.SwitchAnnouncement{
			Root:       root,
			Signatures: []types.SignatureWithHop{types.SignatureWithHop{PublicKey: higherKey}},
		},
	}

	cases := []struct {
		desc     string
		input    snekNextHopParams
		expected *peer // index into peer list
	}{
		{"TestNotBootstrapNoValidNextHop", snekNextHopParams{
			false,
			&destUpKey,
			&selfKey,
			peers[1],
			peers[0],
			&selfAnn,
			announcementTable{
				peers[1]: &parentAnn,
			},
			virtualSnakeTable{},
		}, peers[1]}, // default peer with no next hop is parent
		{"TestBootstrapNoValidNextHop", snekNextHopParams{
			true,
			&destUpKey,
			&selfKey,
			peers[1],
			peers[0],
			&selfAnn,
			announcementTable{
				peers[1]: &parentAnn,
			},
			virtualSnakeTable{},
		}, peers[1]}, // default bootstrap peer with no next hop is parent
		{"TestNotBootstrapDestIsSelf", snekNextHopParams{
			false,
			&destUpKey,
			&destUpKey,
			peers[1],
			peers[0],
			&selfAnn,
			announcementTable{
				peers[1]: &parentAnn,
				peers[2]: &knowsDestUpAnn,
			},
			virtualSnakeTable{},
		}, peers[0]},
		{"TestBootstrapDestIsSelf", snekNextHopParams{
			true,
			&destUpKey,
			&destUpKey,
			peers[1],
			peers[0],
			&selfAnn,
			announcementTable{
				peers[1]: &parentAnn,
			},
			virtualSnakeTable{},
		}, peers[1]}, // bootstraps always start working towards root via parent
		{"TestNotBootstrapPeerIsDestination", snekNextHopParams{
			false,
			&destUpKey,
			&selfKey,
			peers[1],
			peers[0],
			&selfAnn,
			announcementTable{
				peers[1]: &parentAnn,
				peers[2]: &knowsDestUpAnn,
			},
			virtualSnakeTable{},
		}, peers[2]},
		{"TestBootstrapPeerIsDestination", snekNextHopParams{
			true,
			&destUpKey,
			&selfKey,
			peers[1],
			peers[0],
			&selfAnn,
			announcementTable{
				peers[1]: &parentAnn,
				peers[2]: &knowsDestUpAnn,
			},
			virtualSnakeTable{},
		}, peers[1]}, // bootstraps work their way toward the root
		{"TestNotBootstrapParentKnowsDestination", snekNextHopParams{
			false,
			&destUpKey,
			&selfKey,
			peers[1],
			peers[0],
			&selfAnn,
			announcementTable{
				peers[1]: &knowsDestUpAnn,
			},
			virtualSnakeTable{},
		}, peers[1]},
		{"TestNotBootstrapPeerKnowsDestination", snekNextHopParams{
			false,
			&destUpKey,
			&selfKey,
			peers[1],
			peers[0],
			&selfAnn,
			announcementTable{
				peers[1]: &parentAnn,
				peers[2]: &knowsDestUpAnn,
			},
			virtualSnakeTable{},
		}, peers[2]},
		{"TestBootstrapPeerKnowsDestination", snekNextHopParams{
			true,
			&destUpKey,
			&selfKey,
			peers[1],
			peers[0],
			&selfAnn,
			announcementTable{
				peers[1]: &parentAnn,
				peers[2]: &knowsDestUpAnn,
			},
			virtualSnakeTable{},
		}, peers[1]}, // bootstraps work their way toward the root
		{"TestNotBootstrapParentKnowsCloser", snekNextHopParams{
			false,
			&destUpKey,
			&selfKey,
			peers[1],
			peers[0],
			&selfAnn,
			announcementTable{
				peers[1]: &knowsHigherAnn,
			},
			virtualSnakeTable{},
		}, peers[1]},
		{"TestBootstrapParentKnowsCloser", snekNextHopParams{
			true,
			&destUpKey,
			&selfKey,
			peers[1],
			peers[0],
			&selfAnn,
			announcementTable{
				peers[1]: &knowsHigherAnn,
			},
			virtualSnakeTable{},
		}, peers[1]},
		{"TestNotBootstrapSnakeEntryIsDest", snekNextHopParams{
			false,
			&destDownKey,
			&selfKey,
			peers[1],
			peers[0],
			&selfAnn,
			announcementTable{
				peers[1]: &parentAnn,
			},
			virtualSnakeTable{
				virtualSnakeIndex{}: &virtualSnakeEntry{
					Source:            peers[3],
					LastSeen:          time.Now(),
					Active:            true,
					virtualSnakeIndex: &virtualSnakeIndex{PublicKey: destDownKey},
				}},
		}, peers[3]},
		{"TestBootstrapSnakeEntryIsDest", snekNextHopParams{
			true,
			&destDownKey,
			&selfKey,
			peers[1],
			peers[0],
			&selfAnn,
			announcementTable{
				peers[1]: &parentAnn,
			},
			virtualSnakeTable{
				virtualSnakeIndex{}: &virtualSnakeEntry{
					Source:            peers[3],
					LastSeen:          time.Now(),
					Active:            true,
					virtualSnakeIndex: &virtualSnakeIndex{PublicKey: destDownKey},
				}},
		}, peers[0]}, // handle a bootstrap received from a lower key node
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			actual := getNextHopSNEK(tc.input)
			actualString, expectedString := convertToString(actual, tc.expected, peers)

			if actual != tc.expected {
				t.Fatalf("expected: %s got: %s", expectedString, actualString)
			}
		})
	}
}
