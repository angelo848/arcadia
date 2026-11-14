package importers

import (
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// launchBoxSource is the LaunchBox importer (IMP-005). It reads every platform
// file (*.xml) of Data\Platforms. Games that run through an emulator are left out
// with a warning per platform: the emulator launch is out of this phase (fase 6b).
type launchBoxSource struct{}

// LaunchBox returns the LaunchBox source. ScanInput.Path is the LaunchBox
// Data\Platforms folder.
func LaunchBox() Source { return launchBoxSource{} }

func (launchBoxSource) ID() string { return SourceLaunchBox }

// lbLibrary is the root of a LaunchBox platform file.
type lbLibrary struct {
	Games []lbGame `xml:"Game"`
}

// lbGame holds the fields confirmed on the real platform files. Unknown fields are
// ignored by encoding/xml.
type lbGame struct {
	Title           string `xml:"Title"`
	Platform        string `xml:"Platform"`
	Developer       string `xml:"Developer"`
	Publisher       string `xml:"Publisher"`
	Genre           string `xml:"Genre"`
	ReleaseDate     string `xml:"ReleaseDate"`
	Notes           string `xml:"Notes"`
	ApplicationPath string `xml:"ApplicationPath"`
	Emulator        string `xml:"Emulator"`
	PlayMode        string `xml:"PlayMode"`
	PlayCount       int    `xml:"PlayCount"`
	PlayTime        int64  `xml:"PlayTime"`
	LastPlayedDate  string `xml:"LastPlayedDate"`
}

// lbPlayModes maps the LaunchBox play mode names (Portuguese) to DOM-014 names
// (design §9). Unknown names are dropped.
var lbPlayModes = map[string]string{
	"Único jogador": "single",
	"Cooperativo":   "coop_online",
	"Multiplayer":   "multi_online",
}

// Scan reads every *.xml platform file of the folder, in name order. A file that
// cannot be read or parsed is a warning and the others are still read (IMP-008).
func (launchBoxSource) Scan(ctx context.Context, in ScanInput) (Result, error) {
	var res Result
	if err := ctx.Err(); err != nil {
		return res, err
	}
	if in.Path == "" {
		res.Warnings = append(res.Warnings, `LaunchBox: pasta Data\Platforms não escolhida`)
		return res, nil
	}
	entries, err := os.ReadDir(in.Path)
	if err != nil {
		res.Warnings = append(res.Warnings, fmt.Sprintf("LaunchBox: pasta %s ilegível", in.Path))
		return res, nil
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".xml") {
			files = append(files, e.Name())
		}
	}
	if len(files) == 0 {
		res.Warnings = append(res.Warnings, fmt.Sprintf("LaunchBox: nenhum arquivo de plataforma (.xml) em %s", in.Path))
		return res, nil
	}

	skipped := 0
	for _, name := range files {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		data, err := os.ReadFile(filepath.Join(in.Path, name))
		if err != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("LaunchBox: %s ilegível", name))
			continue
		}
		var lib lbLibrary
		if err := xml.Unmarshal(data, &lib); err != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("LaunchBox: %s inválido (%v)", name, err))
			continue
		}
		fallback := strings.TrimSuffix(name, filepath.Ext(name))
		for _, g := range lib.Games {
			platform := platformOf(g, fallback)
			c, ok := launchBoxCandidate(g, platform)
			if !ok {
				skipped++
				continue
			}
			res.Candidates = append(res.Candidates, c)
		}
	}
	if skipped > 0 {
		res.Warnings = append(res.Warnings, fmt.Sprintf("LaunchBox: %d jogo(s) sem título ou caminho ignorado(s)", skipped))
	}
	return res, nil
}

// platformOf returns the platform of a game: its own Platform field, or the name of
// its file when the field is empty.
func platformOf(g lbGame, fallback string) string {
	name := strings.TrimSpace(g.Platform)
	if name == "" {
		name = fallback
	}
	return name
}

// launchBoxCandidate converts one <Game> of the given platform. It reports false
// when the game has no title or no launch target.
func launchBoxCandidate(g lbGame, platform string) (Candidate, bool) {
	title := strings.TrimSpace(g.Title)
	target := strings.TrimSpace(g.ApplicationPath)
	if title == "" || target == "" {
		return Candidate{}, false
	}
	launch := LaunchExe
	switch {
	case strings.TrimSpace(g.Emulator) != "":
		launch = LaunchEmulator // the ROM is the target; the emulator is chosen per platform or game (EMU-003)
	case strings.Contains(target, "://"):
		launch = LaunchURI
	}
	c := Candidate{
		Source:       SourceLaunchBox,
		Title:        title,
		Platform:     platform,
		LaunchType:   launch,
		LaunchTarget: target,
		Developers:   splitList(g.Developer),
		Publishers:   splitList(g.Publisher),
		Genres:       splitList(g.Genre),
		Description:  g.Notes,
		PlayModes:    playModes(g.PlayMode),
		Year:         releaseYear(g.ReleaseDate),
	}
	if g.PlayTime > 0 {
		if last, ok := parseLaunchBoxTime(g.LastPlayedDate); ok {
			c.Played = &Played{Seconds: g.PlayTime, LastPlay: last}
		}
	}
	return c, true
}

// splitList splits a LaunchBox list ("A; B") into trimmed, non-empty names.
func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ";") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// playModes maps the play mode list to DOM-014 names, without duplicates.
func playModes(s string) []string {
	var out []string
	seen := map[string]bool{}
	for _, name := range splitList(s) {
		mode, ok := lbPlayModes[name]
		if !ok || seen[mode] {
			continue
		}
		seen[mode] = true
		out = append(out, mode)
	}
	return out
}

// releaseYear returns the year of an RFC 3339 date, or nil when it is absent.
func releaseYear(s string) *int {
	t, ok := parseLaunchBoxTime(s)
	if !ok {
		return nil
	}
	y := t.Year()
	return &y
}

// parseLaunchBoxTime reads the RFC 3339 dates of LaunchBox (with offset) and
// returns them in UTC.
func parseLaunchBoxTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, false
	}
	return t.UTC(), true
}
