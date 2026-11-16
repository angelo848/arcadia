package winapi

import "testing"

func TestDescendantsOf(t *testing.T) {
	parents := map[uint32]uint32{
		1:  0,  // system
		10: 1,  // the agent
		20: 10, // game started by the agent
		21: 20, // child of the game
		22: 21, // grandchild
		30: 1,  // unrelated
		31: 30,
	}
	got := DescendantsOf(parents, 10)
	for _, pid := range []uint32{20, 21, 22} {
		if _, ok := got[pid]; !ok {
			t.Errorf("pid %d missing from %v", pid, got)
		}
	}
	for _, pid := range []uint32{1, 10, 30, 31} {
		if _, ok := got[pid]; ok {
			t.Errorf("pid %d should not be in %v", pid, got)
		}
	}
	if got := DescendantsOf(parents, 99); len(got) != 0 {
		t.Errorf("no children: got %v", got)
	}
}
