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
		{
			started: *atomic.NewBool(true),
			public:  selfKey,
		},
		// from
		{
			started: *atomic.NewBool(true),
			public:  parentKey,
		},
		// assorted peers
		{
			started: *atomic.NewBool(true),
			public:  destUpKey,
		},
		{
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
			Root: root,
			Signatures: []types.SignatureWithHop{
				{PublicKey: destUpKey},
			},
		},
	}
	knowsHigherAnn := rootAnnouncementWithTime{
		receiveTime:  time.Now(),
		receiveOrder: 1,
		SwitchAnnouncement: types.SwitchAnnouncement{
			Root: root,
			Signatures: []types.SignatureWithHop{
				{PublicKey: higherKey},
			},
		},
	}

	cases := []struct {
		desc     string
		input    virtualSnakeNextHopParams
		expected *peer // index into peer list
	}{
		{"TestNotBootstrapNoValidNextHop", virtualSnakeNextHopParams{
			false,
			false,
			destUpKey,
			selfKey,
			peers[1],
			peers[0],
			&selfAnn,
			announcementTable{
				peers[1]: &parentAnn,
			},
			virtualSnakeTable{},
		}, peers[1]}, // default peer with no next hop is parent
		{"TestBootstrapNoValidNextHop", virtualSnakeNextHopParams{
			true,
			false,
			destUpKey,
			selfKey,
			peers[1],
			peers[0],
			&selfAnn,
			announcementTable{
				peers[1]: &parentAnn,
			},
			virtualSnakeTable{},
		}, peers[1]}, // default bootstrap peer with no next hop is parent
		{"TestNotBootstrapDestIsSelf", virtualSnakeNextHopParams{
			false,
			false,
			destUpKey,
			destUpKey,
			peers[1],
			peers[0],
			&selfAnn,
			announcementTable{
				peers[1]: &parentAnn,
				peers[2]: &knowsDestUpAnn,
			},
			virtualSnakeTable{},
		}, peers[0]},
		{"TestBootstrapDestIsSelf", virtualSnakeNextHopParams{
			true,
			false,
			destUpKey,
			destUpKey,
			peers[1],
			peers[0],
			&selfAnn,
			announcementTable{
				peers[1]: &parentAnn,
			},
			virtualSnakeTable{},
		}, peers[1]}, // bootstraps always start working towards root via parent
		{"TestNotBootstrapPeerIsDestination", virtualSnakeNextHopParams{
			false,
			false,
			destUpKey,
			selfKey,
			peers[1],
			peers[0],
			&selfAnn,
			announcementTable{
				peers[1]: &parentAnn,
				peers[2]: &knowsDestUpAnn,
			},
			virtualSnakeTable{},
		}, peers[2]},
		{"TestBootstrapPeerIsDestination", virtualSnakeNextHopParams{
			true,
			false,
			destUpKey,
			selfKey,
			peers[1],
			peers[0],
			&selfAnn,
			announcementTable{
				peers[1]: &parentAnn,
				peers[2]: &knowsDestUpAnn,
			},
			virtualSnakeTable{},
		}, peers[1]}, // bootstraps work their way toward the root
		{"TestNotBootstrapParentKnowsDestination", virtualSnakeNextHopParams{
			false,
			false,
			destUpKey,
			selfKey,
			peers[1],
			peers[0],
			&selfAnn,
			announcementTable{
				peers[1]: &knowsDestUpAnn,
			},
			virtualSnakeTable{},
		}, peers[1]},
		{"TestNotBootstrapPeerKnowsDestination", virtualSnakeNextHopParams{
			false,
			false,
			destUpKey,
			selfKey,
			peers[1],
			peers[0],
			&selfAnn,
			announcementTable{
				peers[1]: &parentAnn,
				peers[2]: &knowsDestUpAnn,
			},
			virtualSnakeTable{},
		}, peers[2]},
		{"TestBootstrapPeerKnowsDestination", virtualSnakeNextHopParams{
			true,
			false,
			destUpKey,
			selfKey,
			peers[1],
			peers[0],
			&selfAnn,
			announcementTable{
				peers[1]: &parentAnn,
				peers[2]: &knowsDestUpAnn,
			},
			virtualSnakeTable{},
		}, peers[1]}, // bootstraps work their way toward the root
		{"TestNotBootstrapParentKnowsCloser", virtualSnakeNextHopParams{
			false,
			false,
			destUpKey,
			selfKey,
			peers[1],
			peers[0],
			&selfAnn,
			announcementTable{
				peers[1]: &knowsHigherAnn,
			},
			virtualSnakeTable{},
		}, peers[1]},
		{"TestBootstrapParentKnowsCloser", virtualSnakeNextHopParams{
			true,
			false,
			destUpKey,
			selfKey,
			peers[1],
			peers[0],
			&selfAnn,
			announcementTable{
				peers[1]: &knowsHigherAnn,
			},
			virtualSnakeTable{},
		}, peers[1]},
		{"TestNotBootstrapSnakeEntryIsDest", virtualSnakeNextHopParams{
			false,
			false,
			destDownKey,
			selfKey,
			peers[1],
			peers[0],
			&selfAnn,
			announcementTable{
				peers[1]: &parentAnn,
			},
			virtualSnakeTable{
				virtualSnakeIndex{}: &virtualSnakeEntry{
					Source:   peers[3],
					LastSeen: time.Now(),
					//	Active:            true,
					virtualSnakeIndex: &virtualSnakeIndex{PublicKey: destDownKey},
				}},
		}, peers[3]},
		{"TestBootstrapSnakeEntryIsDest", virtualSnakeNextHopParams{
			true,
			false,
			destDownKey,
			selfKey,
			peers[1],
			peers[0],
			&selfAnn,
			announcementTable{
				peers[1]: &parentAnn,
			},
			virtualSnakeTable{
				virtualSnakeIndex{}: &virtualSnakeEntry{
					Source:   peers[3],
					LastSeen: time.Now(),
					//	Active:            true,
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
