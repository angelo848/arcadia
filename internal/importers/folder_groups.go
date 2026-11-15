package importers

import (
	"slices"
)

// groupEntry is one candidate of a folder scan after companions are dropped and disc sets are grouped [IMV-020..IMV-023].
// Main is the file the candidate is launched from; Discs is the number of disc files it stands for; DiscFiles lists
// the relative paths of those discs in disc order, and is nil when Discs is 1.
type groupEntry struct {
	Main      fileEntry
	Discs     int
	DiscFiles []string
}

// containerKey identifies a container or a disc group: the folder (or scope) plus the normalized title key.
type containerKey struct {
	dir string
	key string
}

// dropCompanions removes the track files that sit next to their container in the same folder with the same key
// [IMV-020]. The input order is kept. It reads no disk: only names are used [IMV-080].
func dropCompanions(files []fileEntry) []fileEntry {
	containers := make(map[containerKey]map[string]bool)
	for _, f := range files {
		ext := f.Ext()
		if _, ok := companionExts[ext]; !ok {
			continue
		}
		k := containerKey{dir: f.Dir, key: titleKey(f.Stem())}
		if containers[k] == nil {
			containers[k] = make(map[string]bool)
		}
		containers[k][ext] = true
	}

	out := make([]fileEntry, 0, len(files))
	for _, f := range files {
		k := containerKey{dir: f.Dir, key: titleKey(f.Stem())}
		if isCompanion(containers[k], f.Ext()) {
			continue
		}
		out = append(out, f)
	}
	return out
}

// isCompanion reports whether ext is a track of one of the containers present in the same folder and key [IMV-020].
func isCompanion(present map[string]bool, ext string) bool {
	for container := range present {
		if slices.Contains(companionExts[container], ext) {
			return true
		}
	}
	return false
}

// discMember is a file of a disc group with its disc number.
type discMember struct {
	file fileEntry
	disc int
}

// discGroup collects the files of one (scope, title key) with distinct disc numbers [IMV-021, IMV-022].
type discGroup struct {
	members []discMember
	discs   map[int]bool
	main    *fileEntry // a .m3u of the same key, if any [IMV-023]
}

// groupDiscs turns the principal files into candidates [IMV-021..IMV-023]. Files with a disc number are grouped by
// (scope, title key) in natural order; a repeated disc number stays out as a common candidate [IMV-022]. A group of
// two or more discs becomes one candidate whose main file is the lowest disc; a .m3u with the same key becomes the
// main file of that candidate. The result is sorted by the natural order of Main.Rel(). It reads no disk [IMV-080].
func groupDiscs(files []fileEntry) []groupEntry {
	sorted := slices.Clone(files)
	slices.SortFunc(sorted, func(a, b fileEntry) int { return naturalCompare(a.Rel(), b.Rel()) })

	groups := make(map[containerKey]*discGroup)
	var groupOrder []containerKey
	var m3us, singles []fileEntry

	for _, e := range sorted {
		if e.Ext() == "m3u" {
			m3us = append(m3us, e)
			continue
		}
		n := DiscNumber(e.Stem())
		if n < 1 {
			singles = append(singles, e)
			continue
		}
		k := containerKey{dir: e.Scope(), key: titleKey(e.Stem())}
		g := groups[k]
		if g == nil {
			g = &discGroup{discs: make(map[int]bool)}
			groups[k] = g
			groupOrder = append(groupOrder, k)
		}
		if g.discs[n] {
			singles = append(singles, e)
			continue
		}
		g.discs[n] = true
		g.members = append(g.members, discMember{file: e, disc: n})
	}

	for _, m := range m3us {
		k := containerKey{dir: m.Scope(), key: titleKey(m.Stem())}
		g := groups[k]
		if g == nil || len(g.members) < 2 || g.main != nil {
			singles = append(singles, m)
			continue
		}
		main := m
		g.main = &main
	}

	out := make([]groupEntry, 0, len(groupOrder)+len(singles))
	for _, k := range groupOrder {
		g := groups[k]
		if len(g.members) < 2 {
			singles = append(singles, g.members[0].file)
			continue
		}
		slices.SortFunc(g.members, func(a, b discMember) int { return a.disc - b.disc })
		ge := groupEntry{Main: g.members[0].file, Discs: len(g.members)}
		if g.main != nil {
			ge.Main = *g.main
		}
		ge.DiscFiles = make([]string, 0, len(g.members))
		for _, dm := range g.members {
			ge.DiscFiles = append(ge.DiscFiles, dm.file.Rel())
		}
		out = append(out, ge)
	}
	for _, e := range singles {
		out = append(out, groupEntry{Main: e, Discs: 1})
	}

	slices.SortFunc(out, func(a, b groupEntry) int { return naturalCompare(a.Main.Rel(), b.Main.Rel()) })
	return out
}
