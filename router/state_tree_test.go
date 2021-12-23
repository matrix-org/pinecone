package router

import (
	"testing"
	"time"

	"github.com/matrix-org/pinecone/types"
)

func TestHandleTreeAnnouncement(t *testing.T) {
	cases := []struct {
		desc               string
		senderIsParent     bool
		updateContainsLoop bool
		rootDelta          int
		newRootSequence    types.Varu64
		lastRootSequence   types.Varu64
		expected           TreeAnnouncementAction
	}{
		{"TestParentLoop1", true, true, -1, 1, 1, SelectNewParentWithWait},
		{"TestParentLoop2", true, true, -1, 1, 2, SelectNewParentWithWait},
		{"TestParentLoop3", true, true, -1, 2, 1, SelectNewParentWithWait},
		{"TestParentLoop4", true, true, 1, 1, 1, SelectNewParentWithWait},
		{"TestParentLoop5", true, true, 1, 1, 2, SelectNewParentWithWait},
		{"TestParentLoop6", true, true, 1, 2, 1, SelectNewParentWithWait},
		{"TestParentLoop7", true, true, 0, 1, 1, SelectNewParentWithWait},
		{"TestParentLoop8", true, true, 0, 1, 2, SelectNewParentWithWait},
		{"TestParentLoop9", true, true, 0, 2, 1, SelectNewParentWithWait},
		{"TestParentLowerRoot1", true, false, -1, 1, 1, SelectNewParentWithWait},
		{"TestParentLowerRoot2", true, false, -1, 1, 2, SelectNewParentWithWait},
		{"TestParentLowerRoot3", true, false, -1, 2, 1, SelectNewParentWithWait},
		{"TestParentHigherRoot1", true, false, 1, 1, 1, AcceptUpdate},
		{"TestParentHigherRoot2", true, false, 1, 1, 2, AcceptUpdate},
		{"TestParentHigherRoot3", true, false, 1, 2, 1, AcceptUpdate},
		{"TestParentSameRootSameSeq", true, false, 0, 1, 1, SelectNewParentWithWait},
		{"TestParentSameRootLowerSeq", true, false, 0, 1, 2, DropFrame},
		{"TestParentSameRootHigherSeq", true, false, 0, 2, 1, AcceptUpdate},

		{"TestNonParentLoop1", false, true, -1, 1, 1, DropFrame},
		{"TestNonParentLoop2", false, true, -1, 1, 2, DropFrame},
		{"TestNonParentLoop3", false, true, -1, 2, 1, DropFrame},
		{"TestNonParentLoop4", false, true, 1, 1, 1, DropFrame},
		{"TestNonParentLoop5", false, true, 1, 1, 2, DropFrame},
		{"TestNonParentLoop6", false, true, 1, 2, 1, DropFrame},
		{"TestNonParentLoop7", false, true, 0, 1, 1, DropFrame},
		{"TestNonParentLoop8", false, true, 0, 1, 2, DropFrame},
		{"TestNonParentLoop9", false, true, 0, 2, 1, DropFrame},
		{"TestNonParentLowerRoot1", false, false, -1, 1, 1, InformPeerOfStrongerRoot},
		{"TestNonParentLowerRoot2", false, false, -1, 1, 2, InformPeerOfStrongerRoot},
		{"TestNonParentLowerRoot3", false, false, -1, 2, 1, InformPeerOfStrongerRoot},
		{"TestNonParentHigherRoot1", false, false, 1, 1, 1, AcceptNewParent},
		{"TestNonParentHigherRoot2", false, false, 1, 1, 2, AcceptNewParent},
		{"TestNonParentHigherRoot3", false, false, 1, 2, 1, AcceptNewParent},
		{"TestNonParentSameRoot1", false, false, 0, 1, 1, SelectNewParent},
		{"TestNonParentSameRoot2", false, false, 0, 1, 2, SelectNewParent},
		{"TestNonParentSameRoot3", false, false, 0, 2, 1, SelectNewParent},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			actual := determineAnnouncementAction(tc.senderIsParent, tc.updateContainsLoop,
				tc.rootDelta, tc.newRootSequence, tc.lastRootSequence)
			if actual != tc.expected {
				t.Fatalf("expected: %d got: %d", tc.expected, actual)
			}
		})
	}
}

func TestTreeParentSelection(t *testing.T) {
	cases := []struct {
		desc         string
		announcement rootAnnouncementWithTime
		bestRoot     types.Root
		bestOrder    uint64
		containsLoop bool
		expected     bool
	}{
		{desc: "TestAnnouncementTooOld",
			announcement: rootAnnouncementWithTime{
				receiveTime:  time.Now().Add(-announcementTimeout * 2),
				receiveOrder: 0,
				SwitchAnnouncement: types.SwitchAnnouncement{
					Root: types.Root{
						RootPublicKey: types.PublicKey{6}, RootSequence: 1,
					}}},
			bestRoot: types.Root{
				RootPublicKey: types.PublicKey{5}, RootSequence: 1,
			},
			bestOrder:    0,
			containsLoop: false,
			expected:     false,
		},
		{desc: "TestContainsLoop",
			announcement: rootAnnouncementWithTime{
				receiveTime:  time.Now(),
				receiveOrder: 0,
				SwitchAnnouncement: types.SwitchAnnouncement{
					Root: types.Root{
						RootPublicKey: types.PublicKey{6},
					}}},
			bestRoot: types.Root{
				RootPublicKey: types.PublicKey{5}, RootSequence: 1,
			},
			bestOrder:    0,
			containsLoop: true,
			expected:     false,
		},
		{desc: "TestLowerRoot1",
			announcement: rootAnnouncementWithTime{
				receiveTime:  time.Now(),
				receiveOrder: 0,
				SwitchAnnouncement: types.SwitchAnnouncement{
					Root: types.Root{
						RootPublicKey: types.PublicKey{4},
					}}},
			bestRoot: types.Root{
				RootPublicKey: types.PublicKey{5}, RootSequence: 1,
			},
			bestOrder:    0,
			containsLoop: false,
			expected:     false,
		},
		{desc: "TestLowerRoot2",
			announcement: rootAnnouncementWithTime{
				receiveTime:  time.Now(),
				receiveOrder: 0,
				SwitchAnnouncement: types.SwitchAnnouncement{
					Root: types.Root{
						RootPublicKey: types.PublicKey{4}, RootSequence: 2,
					}}},
			bestRoot: types.Root{
				RootPublicKey: types.PublicKey{5}, RootSequence: 1,
			},
			bestOrder:    0,
			containsLoop: false,
			expected:     false,
		},
		{desc: "TestLowerRoot3",
			announcement: rootAnnouncementWithTime{
				receiveTime:  time.Now(),
				receiveOrder: 0,
				SwitchAnnouncement: types.SwitchAnnouncement{
					Root: types.Root{
						RootPublicKey: types.PublicKey{4}, RootSequence: 1,
					}}},
			bestRoot: types.Root{
				RootPublicKey: types.PublicKey{5}, RootSequence: 2,
			},
			bestOrder:    0,
			containsLoop: false,
			expected:     false,
		},
		{desc: "TestLowerRoot4",
			announcement: rootAnnouncementWithTime{
				receiveTime:  time.Now(),
				receiveOrder: 1,
				SwitchAnnouncement: types.SwitchAnnouncement{
					Root: types.Root{
						RootPublicKey: types.PublicKey{4}, RootSequence: 1,
					}}},
			bestRoot: types.Root{
				RootPublicKey: types.PublicKey{5}, RootSequence: 1,
			},
			bestOrder:    0,
			containsLoop: false,
			expected:     false,
		},
		{desc: "TestLowerRoot5",
			announcement: rootAnnouncementWithTime{
				receiveTime:  time.Now(),
				receiveOrder: 1,
				SwitchAnnouncement: types.SwitchAnnouncement{
					Root: types.Root{
						RootPublicKey: types.PublicKey{4}, RootSequence: 2,
					}}},
			bestRoot: types.Root{
				RootPublicKey: types.PublicKey{5}, RootSequence: 1,
			},
			bestOrder:    0,
			containsLoop: false,
			expected:     false,
		},
		{desc: "TestLowerRoot6",
			announcement: rootAnnouncementWithTime{
				receiveTime:  time.Now(),
				receiveOrder: 1,
				SwitchAnnouncement: types.SwitchAnnouncement{
					Root: types.Root{
						RootPublicKey: types.PublicKey{4}, RootSequence: 1,
					}}},
			bestRoot: types.Root{
				RootPublicKey: types.PublicKey{5}, RootSequence: 2,
			},
			bestOrder:    0,
			containsLoop: false,
			expected:     false,
		},
		{desc: "TestLowerRoot7",
			announcement: rootAnnouncementWithTime{
				receiveTime:  time.Now(),
				receiveOrder: 0,
				SwitchAnnouncement: types.SwitchAnnouncement{
					Root: types.Root{
						RootPublicKey: types.PublicKey{4}, RootSequence: 1,
					}}},
			bestRoot: types.Root{
				RootPublicKey: types.PublicKey{5}, RootSequence: 1,
			},
			bestOrder:    1,
			containsLoop: false,
			expected:     false,
		},
		{desc: "TestLowerRoot8",
			announcement: rootAnnouncementWithTime{
				receiveTime:  time.Now(),
				receiveOrder: 0,
				SwitchAnnouncement: types.SwitchAnnouncement{
					Root: types.Root{
						RootPublicKey: types.PublicKey{4}, RootSequence: 2,
					}}},
			bestRoot: types.Root{
				RootPublicKey: types.PublicKey{5}, RootSequence: 1,
			},
			bestOrder:    1,
			containsLoop: false,
			expected:     false,
		},
		{desc: "TestLowerRoot9",
			announcement: rootAnnouncementWithTime{
				receiveTime:  time.Now(),
				receiveOrder: 0,
				SwitchAnnouncement: types.SwitchAnnouncement{
					Root: types.Root{
						RootPublicKey: types.PublicKey{4}, RootSequence: 1,
					}}},
			bestRoot: types.Root{
				RootPublicKey: types.PublicKey{5}, RootSequence: 2,
			},
			bestOrder:    1,
			containsLoop: false,
			expected:     false,
		},
		{desc: "TestHigherRoot1",
			announcement: rootAnnouncementWithTime{
				receiveTime:  time.Now(),
				receiveOrder: 0,
				SwitchAnnouncement: types.SwitchAnnouncement{
					Root: types.Root{
						RootPublicKey: types.PublicKey{6},
					}}},
			bestRoot: types.Root{
				RootPublicKey: types.PublicKey{5}, RootSequence: 1,
			},
			bestOrder:    0,
			containsLoop: false,
			expected:     true,
		},
		{desc: "TestHigherRoot2",
			announcement: rootAnnouncementWithTime{
				receiveTime:  time.Now(),
				receiveOrder: 0,
				SwitchAnnouncement: types.SwitchAnnouncement{
					Root: types.Root{
						RootPublicKey: types.PublicKey{6}, RootSequence: 2,
					}}},
			bestRoot: types.Root{
				RootPublicKey: types.PublicKey{5}, RootSequence: 1,
			},
			bestOrder:    0,
			containsLoop: false,
			expected:     true,
		},
		{desc: "TestHigherRoot3",
			announcement: rootAnnouncementWithTime{
				receiveTime:  time.Now(),
				receiveOrder: 0,
				SwitchAnnouncement: types.SwitchAnnouncement{
					Root: types.Root{
						RootPublicKey: types.PublicKey{6}, RootSequence: 1,
					}}},
			bestRoot: types.Root{
				RootPublicKey: types.PublicKey{5}, RootSequence: 2,
			},
			bestOrder:    0,
			containsLoop: false,
			expected:     true,
		},
		{desc: "TestHigherRoot4",
			announcement: rootAnnouncementWithTime{
				receiveTime:  time.Now(),
				receiveOrder: 1,
				SwitchAnnouncement: types.SwitchAnnouncement{
					Root: types.Root{
						RootPublicKey: types.PublicKey{6}, RootSequence: 1,
					}}},
			bestRoot: types.Root{
				RootPublicKey: types.PublicKey{5}, RootSequence: 1,
			},
			bestOrder:    0,
			containsLoop: false,
			expected:     true,
		},
		{desc: "TestHigherRoot5",
			announcement: rootAnnouncementWithTime{
				receiveTime:  time.Now(),
				receiveOrder: 1,
				SwitchAnnouncement: types.SwitchAnnouncement{
					Root: types.Root{
						RootPublicKey: types.PublicKey{6}, RootSequence: 2,
					}}},
			bestRoot: types.Root{
				RootPublicKey: types.PublicKey{5}, RootSequence: 1,
			},
			bestOrder:    0,
			containsLoop: false,
			expected:     true,
		},
		{desc: "TestHigherRoot6",
			announcement: rootAnnouncementWithTime{
				receiveTime:  time.Now(),
				receiveOrder: 1,
				SwitchAnnouncement: types.SwitchAnnouncement{
					Root: types.Root{
						RootPublicKey: types.PublicKey{6}, RootSequence: 1,
					}}},
			bestRoot: types.Root{
				RootPublicKey: types.PublicKey{5}, RootSequence: 2,
			},
			bestOrder:    0,
			containsLoop: false,
			expected:     true,
		},
		{desc: "TestHigherRoot7",
			announcement: rootAnnouncementWithTime{
				receiveTime:  time.Now(),
				receiveOrder: 0,
				SwitchAnnouncement: types.SwitchAnnouncement{
					Root: types.Root{
						RootPublicKey: types.PublicKey{6}, RootSequence: 1,
					}}},
			bestRoot: types.Root{
				RootPublicKey: types.PublicKey{5}, RootSequence: 1,
			},
			bestOrder:    1,
			containsLoop: false,
			expected:     true,
		},
		{desc: "TestHigherRoot8",
			announcement: rootAnnouncementWithTime{
				receiveTime:  time.Now(),
				receiveOrder: 0,
				SwitchAnnouncement: types.SwitchAnnouncement{
					Root: types.Root{
						RootPublicKey: types.PublicKey{6}, RootSequence: 2,
					}}},
			bestRoot: types.Root{
				RootPublicKey: types.PublicKey{5}, RootSequence: 1,
			},
			bestOrder:    1,
			containsLoop: false,
			expected:     true,
		},
		{desc: "TestHigherRoot9",
			announcement: rootAnnouncementWithTime{
				receiveTime:  time.Now(),
				receiveOrder: 0,
				SwitchAnnouncement: types.SwitchAnnouncement{
					Root: types.Root{
						RootPublicKey: types.PublicKey{6}, RootSequence: 1,
					}}},
			bestRoot: types.Root{
				RootPublicKey: types.PublicKey{5}, RootSequence: 2,
			},
			bestOrder:    1,
			containsLoop: false,
			expected:     true,
		},
		{desc: "TestSameRootHigherSequenceHigherOrder",
			announcement: rootAnnouncementWithTime{
				receiveTime:  time.Now(),
				receiveOrder: 1,
				SwitchAnnouncement: types.SwitchAnnouncement{
					Root: types.Root{
						RootPublicKey: types.PublicKey{5}, RootSequence: 2,
					}}},
			bestRoot: types.Root{
				RootPublicKey: types.PublicKey{5}, RootSequence: 1,
			},
			bestOrder:    0,
			containsLoop: false,
			expected:     true,
		},
		{desc: "TestSameRootHigherSequenceLowerOrder",
			announcement: rootAnnouncementWithTime{
				receiveTime:  time.Now(),
				receiveOrder: 0,
				SwitchAnnouncement: types.SwitchAnnouncement{
					Root: types.Root{
						RootPublicKey: types.PublicKey{5}, RootSequence: 2,
					}}},
			bestRoot: types.Root{
				RootPublicKey: types.PublicKey{5}, RootSequence: 1,
			},
			bestOrder:    1,
			containsLoop: false,
			expected:     true,
		},
		{desc: "TestSameRootHigherSequenceSameOrder",
			announcement: rootAnnouncementWithTime{
				receiveTime:  time.Now(),
				receiveOrder: 0,
				SwitchAnnouncement: types.SwitchAnnouncement{
					Root: types.Root{
						RootPublicKey: types.PublicKey{5}, RootSequence: 2,
					}}},
			bestRoot: types.Root{
				RootPublicKey: types.PublicKey{5}, RootSequence: 1,
			},
			bestOrder:    0,
			containsLoop: false,
			expected:     true,
		},
		{desc: "TestSameRootLowerSequenceHigherOrder",
			announcement: rootAnnouncementWithTime{
				receiveTime:  time.Now(),
				receiveOrder: 1,
				SwitchAnnouncement: types.SwitchAnnouncement{
					Root: types.Root{
						RootPublicKey: types.PublicKey{5}, RootSequence: 1,
					}}},
			bestRoot: types.Root{
				RootPublicKey: types.PublicKey{5}, RootSequence: 2,
			},
			bestOrder:    0,
			containsLoop: false,
			expected:     false,
		},
		{desc: "TestSameRootLowerSequenceLowerOrder",
			announcement: rootAnnouncementWithTime{
				receiveTime:  time.Now(),
				receiveOrder: 0,
				SwitchAnnouncement: types.SwitchAnnouncement{
					Root: types.Root{
						RootPublicKey: types.PublicKey{5}, RootSequence: 1,
					}}},
			bestRoot: types.Root{
				RootPublicKey: types.PublicKey{5}, RootSequence: 2,
			},
			bestOrder:    1,
			containsLoop: false,
			expected:     false,
		},
		{desc: "TestSameRootLowerSequenceSameOrder",
			announcement: rootAnnouncementWithTime{
				receiveTime:  time.Now(),
				receiveOrder: 0,
				SwitchAnnouncement: types.SwitchAnnouncement{
					Root: types.Root{
						RootPublicKey: types.PublicKey{5}, RootSequence: 1,
					}}},
			bestRoot: types.Root{
				RootPublicKey: types.PublicKey{5}, RootSequence: 2,
			},
			bestOrder:    0,
			containsLoop: false,
			expected:     false,
		},
		{desc: "TestSameRootSameSequenceHigherOrder",
			announcement: rootAnnouncementWithTime{
				receiveTime:  time.Now(),
				receiveOrder: 1,
				SwitchAnnouncement: types.SwitchAnnouncement{
					Root: types.Root{
						RootPublicKey: types.PublicKey{5}, RootSequence: 1,
					}}},
			bestRoot: types.Root{
				RootPublicKey: types.PublicKey{5}, RootSequence: 1,
			},
			bestOrder:    0,
			containsLoop: false,
			expected:     false,
		},
		{desc: "TestSameRootSameSequenceLowerOrder",
			announcement: rootAnnouncementWithTime{
				receiveTime:  time.Now(),
				receiveOrder: 0,
				SwitchAnnouncement: types.SwitchAnnouncement{
					Root: types.Root{
						RootPublicKey: types.PublicKey{5}, RootSequence: 1,
					}}},
			bestRoot: types.Root{
				RootPublicKey: types.PublicKey{5}, RootSequence: 1,
			},
			bestOrder:    1,
			containsLoop: false,
			expected:     true,
		},
		{desc: "TestSameRootSameSequenceSameOrder",
			announcement: rootAnnouncementWithTime{
				receiveTime:  time.Now(),
				receiveOrder: 0,
				SwitchAnnouncement: types.SwitchAnnouncement{
					Root: types.Root{
						RootPublicKey: types.PublicKey{5}, RootSequence: 1,
					}}},
			bestRoot: types.Root{
				RootPublicKey: types.PublicKey{5}, RootSequence: 1,
			},
			bestOrder:    0,
			containsLoop: false,
			expected:     false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			actual := isBetterParentCandidate(tc.announcement, tc.bestRoot, tc.bestOrder, tc.containsLoop)
			if actual != tc.expected {
				t.Fatalf("expected: %t got: %t", tc.expected, actual)
			}
		})
	}
}

func TestTreeNextHopCandidate(t *testing.T) {
	cases := []struct {
		desc            string
		peerDist        int64
		bestDist        int64
		peerOrder       uint64
		bestOrder       uint64
		candidateExists bool
		expected        bool
	}{
		{"TestCloserHigherOrderCandidate", 1, 2, 2, 1, true, true},
		{"TestCloserHigherOrderNoCandidate", 1, 2, 2, 1, false, true},
		{"TestCloserLowerOrderCandidate", 1, 2, 1, 2, true, true},
		{"TestCloserLowerOrderNoCandidate", 1, 2, 1, 2, false, true},
		{"TestCloserSameOrderCandidate", 1, 2, 1, 1, true, true},
		{"TestCloserSameOrderNoCandidate", 1, 2, 1, 1, false, true},
		{"TestFurtherHigherOrderCandidate", 2, 1, 2, 1, true, false},
		{"TestFurtherHigherOrderNoCandidate", 2, 1, 2, 1, false, false},
		{"TestFurtherLowerOrderCandidate", 2, 1, 1, 2, true, false},
		{"TestFurtherLowerOrderNoCandidate", 2, 1, 1, 2, false, false},
		{"TestFurtherSameOrderCandidate", 2, 1, 1, 1, true, false},
		{"TestFurtherSameOrderNoCandidate", 2, 1, 1, 1, false, false},
		{"TestEquidistantHigherOrderCandidate", 1, 1, 2, 1, true, false},
		{"TestEquidistantHigherOrderNoCandidate", 1, 1, 2, 1, false, false},
		{"TestEquidistantLowerOrderCandidate", 1, 1, 1, 2, true, true},
		{"TestEquidistantLowerOrderNoCandidate", 1, 1, 1, 2, false, false},
		{"TestEquidistantSameOrderCandidate", 1, 1, 1, 1, true, false},
		{"TestEquidistantSameOrderNoCandidate", 1, 1, 1, 1, false, false},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			actual := isBetterNextHopCandidate(tc.peerDist, tc.bestDist, tc.peerOrder, tc.bestOrder, tc.candidateExists)
			if actual != tc.expected {
				t.Fatalf("expected: %t got: %t", tc.expected, actual)
			}
		})
	}
}
