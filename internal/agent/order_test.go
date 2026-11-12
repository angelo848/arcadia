package agent

// indexOf returns the position of s in list, or -1 (used by the call-order assertions of app and shutdown tests).
func indexOf(list []string, s string) int {
	for i, v := range list {
		if v == s {
			return i
		}
	}
	return -1
}
