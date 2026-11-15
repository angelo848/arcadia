package importers

import (
	"io/fs"
	"strings"
)

// Rule tables of the folder source (phase 11, import-rules.md §1, §1.1, §3, §4.1). Extensions are lower case
// without the dot. They are package-level variables, never spread over if chains [IMV-013, IMV-015, IMV-020].

// executableExts are the files launched directly (launchType exe) [IMV-013, IMV-016].
var executableExts = map[string]bool{"exe": true, "bat": true, "cmd": true}

// consoleExts are the ROM and disc files of every platform that is not Windows (launchType emulator) [IMV-013].
var consoleExts = map[string]bool{
	// Nintendo
	"nes": true, "unf": true, "unif": true, "fds": true, "sfc": true, "smc": true, "fig": true, "swc": true,
	"gb": true, "gbc": true, "gba": true, "n64": true, "z64": true, "v64": true, "ndd": true, "nds": true,
	"dsi": true, "3ds": true, "cia": true, "cci": true, "cxi": true, "3dsx": true, "gcm": true, "gcz": true,
	"rvz": true, "wbfs": true, "wad": true, "wia": true, "nsp": true, "xci": true, "nsz": true, "xcz": true,
	"vb": true, "min": true,
	// Sega
	"sms": true, "gg": true, "md": true, "gen": true, "smd": true, "32x": true, "sg": true, "cdi": true, "gdi": true,
	// Disco (Sony e outros)
	"iso": true, "cue": true, "bin": true, "img": true, "chd": true, "pbp": true, "cso": true, "mdf": true,
	"mds": true, "ccd": true, "m3u": true,
	// Atari
	"a26": true, "a52": true, "a78": true, "lnx": true, "j64": true, "jag": true, "st": true, "stx": true, "xex": true,
	// NEC, SNK, Bandai e outros
	"pce": true, "sgx": true, "tg16": true, "ngp": true, "ngc": true, "ws": true, "wsc": true, "col": true,
	"int": true, "vec": true, "rom": true,
	// Computadores
	"adf": true, "dsk": true, "d64": true, "t64": true, "tap": true, "crt": true, "tzx": true,
	// Compactados
	"zip": true, "7z": true,
}

// notAGameNames are the names (without extension, lower case) of executables that are not games [IMV-015].
// A trailing * in the spec is a prefix match; a leading * is a suffix match; a middle * is a substring match.
var (
	notAGamePrefixes  = []string{"unins", "uninstall", "setup", "vcredist", "vc_redist", "dotnet", "unitycrashhandler", "directx"}
	notAGameSuffixes  = []string{"setup"}
	notAGameContains  = []string{"crashreport", "redist"}
	notAGameExactName = []string{"dxsetup", "oalinst", "crashpad_handler"}
)

// ignoredFolderNames are entries never read, compared without case [IMV-012].
var ignoredFolderNames = map[string]bool{
	"system volume information": true,
	"@eadir":                    true,
	"__macosx":                  true,
	"node_modules":              true,
}

// companionExts maps a container to the track files that sit next to it and are not candidates [IMV-020].
var companionExts = map[string][]string{
	"cue": {"bin", "img", "sub", "wav"},
	"ccd": {"img", "sub"},
	"mds": {"mdf"},
	"gdi": {"bin", "raw"},
}

// isGameExt reports whether ext (lower case, no dot) is read by a folder scan of the given kind [IMV-013].
func isGameExt(ext string, kind FolderKind) bool {
	switch kind {
	case KindExecutables:
		return executableExts[ext]
	case KindConsole:
		return consoleExts[ext]
	default:
		return executableExts[ext] || consoleExts[ext]
	}
}

// isExecutableExt reports whether ext (lower case, no dot) is an executable file [IMV-013, IMV-016].
func isExecutableExt(ext string) bool { return executableExts[ext] }

// isNotAGame reports whether an executable named stem (no extension) is an installer, a redistributable or a
// crash reporter, which is ignored and counted as .exe [IMV-015].
func isNotAGame(stem string) bool {
	s := strings.ToLower(stem)
	for _, v := range notAGameExactName {
		if s == v {
			return true
		}
	}
	for _, p := range notAGamePrefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	for _, x := range notAGameSuffixes {
		if strings.HasSuffix(s, x) {
			return true
		}
	}
	for _, x := range notAGameContains {
		if strings.Contains(s, x) {
			return true
		}
	}
	return false
}

// skipEntry reports whether a directory entry is never read or counted [IMV-012]: names starting with "." or "$",
// the named system and tool folders, and anything that is neither a regular file nor a folder (symbolic links,
// junctions, devices).
func skipEntry(name string, typ fs.FileMode) bool {
	if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "$") {
		return true
	}
	if ignoredFolderNames[strings.ToLower(name)] {
		return true
	}
	if t := typ.Type(); t != 0 && t != fs.ModeDir {
		return true
	}
	return false
}
