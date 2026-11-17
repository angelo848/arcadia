package importers

import (
	"context"
	"time"
)

// Source identifiers, platform names and launch types used by every importer.
const (
	SourceSteam     = "steam"
	SourceEpic      = "epic"
	SourceFile      = "file"
	SourceFolder    = "folder"
	SourceLaunchBox = "launchbox"
	SourceURI       = "uri"

	// PlatformWindows is the platform of Steam and Epic games (seed key "windows").
	PlatformWindows = "Windows"

	LaunchURI      = "uri"
	LaunchEmulator = "emulator" // the ROM runs in an emulator of its platform or game (EMU-003)
	LaunchExe      = "exe"
)

// ScanInput carries the folder a source reads from. Steam and Epic find their
// folders themselves when Path is empty. Folder is read by the folder source only.
type ScanInput struct {
	Path   string
	Folder FolderOptions
}

// Source is one import origin. Scan only reads (IMP-001, IMP-009).
type Source interface {
	ID() string
	Scan(ctx context.Context, in ScanInput) (Result, error)
}

// Candidate is one game a source offers. Platform is empty for file candidates
// (chosen at apply, IMP-004). Year and PlayedSeconds are nil when unknown.
type Candidate struct {
	Source       string
	Title        string
	Platform     string
	LaunchType   string
	LaunchTarget string
	InstallDir   string
	Developers   []string
	Publishers   []string
	Genres       []string
	Year         *int
	Description  string
	PlayModes    []string
	Played       *Played

	// File is the main file relative to the chosen folder (the file name for the file source); Extension is
	// its lower-case extension without the dot; Discs is the number of discs (>= 1 for file and folder
	// candidates, 0 for the other sources); DiscFiles lists the disc files in disc order when Discs >= 2 [IMV-024].
	File      string
	Extension string
	Discs     int
	DiscFiles []string

	// SearchName is the name proposed for the metadata search; only the file source fills it
	// (SearchName(path), MET-040). The other sources leave it empty.
	SearchName string
}

// Played is the play time LaunchBox records (IMP-007). LastPlay is in UTC.
type Played struct {
	Seconds  int64
	LastPlay time.Time
}

// Result is what a scan found. Warnings name sources or entries that could not be
// read (IMP-002, IMP-003, IMP-008); they never fail the scan.
type Result struct {
	Candidates []Candidate
	Warnings   []string
}
