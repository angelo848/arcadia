package domain

// MediaKindLimit returns the maximum number of media items kept per game for
// the given kind (AJU-020, ADR 0030). Unknown kinds return 0.
func MediaKindLimit(kind string) int {
	switch kind {
	case "screenshot":
		return 10
	case "cover", "video":
		return 1
	case "background", "title", "select", "banner":
		return 2
	default:
		return 0
	}
}
