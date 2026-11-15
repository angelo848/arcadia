package importers

import (
	"regexp"
	"strings"
	"unicode"
)

// maxSearchFolders is how many folders above the file are read for a search name [match-rules.md §5.2].
const maxSearchFolders = 3

// reEngineSuffix matches one engine suffix at the end of a stem: a separator followed by win32, win64, x86, x64,
// 32bit, 64bit or shipping, in any case [match-rules.md §5.2 step 1].
var reEngineSuffix = regexp.MustCompile(`(?i)[-_ .](win32|win64|x86|x64|32bit|64bit|shipping)$`)

// genericFileNames are file stems that say nothing about the game [match-rules.md §5.2].
var genericFileNames = keySet(
	"game", "games", "launcher", "launch", "start", "startgame", "play", "run", "main",
	"app", "application", "client", "bin", "program", "exe",
)

// genericFolderNames are folder names that say nothing about the game [match-rules.md §5.2].
var genericFolderNames = keySet(
	"bin", "bin32", "bin64", "binaries", "win32", "win64", "x86", "x64", "windows", "release", "debug",
	"retail", "shipping", "build", "game", "games", "jogos", "app", "program", "program files",
	"program files (x86)", "common", "steamapps", "data", "roms", "downloads", "desktop",
)

// SearchName proposes the metadata search name of a file path [MET-040, match-rules.md §5.2]. It is pure: no disk, no
// clock. The path uses Windows separators; '/' is accepted too. The result is at most domain.MaxTitleRunes runes.
func SearchName(path string) string {
	parts := strings.FieldsFunc(path, func(r rune) bool { return r == '\\' || r == '/' })
	if len(parts) == 0 {
		return ""
	}
	file := parts[len(parts)-1]
	stem, _ := SplitExt(file)

	// Engine suffixes are removed from the end, repeatedly (e.g. "-Win64-Shipping").
	for {
		next := reEngineSuffix.ReplaceAllString(stem, "")
		if next == stem {
			break
		}
		stem = next
	}

	name := searchTitle(stem, false)
	if name != "" && !genericFileNames[genericKey(name)] {
		return name
	}

	// Folders above the file, nearest first, never the drive root, at most maxSearchFolders of them.
	folders := parts[:len(parts)-1]
	if len(folders) > 0 && strings.HasSuffix(folders[0], ":") {
		folders = folders[1:]
	}
	examined := 0
	for i := len(folders) - 1; i >= 0 && examined < maxSearchFolders; i-- {
		examined++
		folder := folders[i]
		if genericFolderNames[genericKey(folder)] {
			continue
		}
		if t := searchTitle(folder, true); t != "" {
			return t
		}
	}

	return name
}

// searchTitle is NormalizeTitle for a search name. NormalizeTitle keeps the raw text when only markings are
// left ("[]" stays "[]"); a search name never does, so it returns "" then. The result is at most
// domain.MaxTitleRunes runes [MET-040].
func searchTitle(s string, fromFolder bool) string {
	clean := s
	if fromFolder {
		clean = reOrderPrefix.ReplaceAllString(clean, "$1")
	}
	if cleanSpaces(strings.ReplaceAll(removeGroups(clean), "_", " ")) == "" {
		return ""
	}
	return NormalizeTitle(s, fromFolder)
}

// genericKey compares names without case, spaces, separators or punctuation.
func genericKey(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

// keySet builds a set of genericKey values.
func keySet(names ...string) map[string]bool {
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[genericKey(n)] = true
	}
	return m
}
