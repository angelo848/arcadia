package winapi

// DescendantsOf returns the processes that descend from a child of parent, children included, given the
// parent of every process (pid -> ppid). parent itself is never in the result.
func DescendantsOf(parents map[uint32]uint32, parent uint32) map[uint32]struct{} {
	out := map[uint32]struct{}{}
	for changed := true; changed; {
		changed = false
		for pid, ppid := range parents {
			if pid == parent {
				continue
			}
			if _, done := out[pid]; done {
				continue
			}
			if _, in := out[ppid]; in || ppid == parent {
				out[pid] = struct{}{}
				changed = true
			}
		}
	}
	return out
}
