package ipc

// Import methods (phase 6, contract v0.7.0 [IMP-001]). Scanning only reads; applying
// writes one game per transaction (IMP-008).

// Import sources accepted by import.scan (IMP-002..IMP-005).
const (
	ImportSourceSteam     = "steam"
	ImportSourceEpic      = "epic"
	ImportSourceFile      = "file"
	ImportSourceFolder    = "folder"
	ImportSourceLaunchBox = "launchbox"
	ImportSourceURI       = "uri" // Path carries the URI typed by the user
	ImportSourceXbox      = "xbox"
)

// Values of ImportScanParams.TitleFrom (contract v0.13.0 [IMV-017, IMV-070]).
const (
	ImportTitleFromFile   = "file"
	ImportTitleFromFolder = "folder"
)

// ImportScanParams is the params of import.scan. Path is used by the file, folder
// and launchbox sources only. Platform, TitleFrom and Recursive are used by the
// file and folder sources only [IMV-070, IMV-071]; Recursive nil means true.
type ImportScanParams struct {
	Source    string `json:"source"`
	Path      string `json:"path,omitempty"`
	Platform  string `json:"platform,omitempty"`
	TitleFrom string `json:"titleFrom,omitempty"`
	Recursive *bool  `json:"recursive,omitempty"`
}

// ImportCandidate is one game an import source offers. Exists is set by
// import.scan only (IMP-006); the apply side ignores it. File, Extension, Discs
// and DiscFiles are set by the file and folder sources only [IMV-024].
type ImportCandidate struct {
	Source        string   `json:"source"`
	Title         string   `json:"title"`
	Platform      string   `json:"platform"`
	LaunchType    string   `json:"launchType"`
	LaunchTarget  string   `json:"launchTarget"`
	InstallDir    string   `json:"installDir"`
	Developers    []string `json:"developers"`
	Publishers    []string `json:"publishers"`
	Genres        []string `json:"genres"`
	Year          *int     `json:"year"`
	Description   string   `json:"description"`
	PlayModes     []string `json:"playModes"`
	PlayedSeconds *int64   `json:"playedSeconds"`
	LastPlayed    *string  `json:"lastPlayed"`
	Exists        bool     `json:"exists"`
	File          string   `json:"file,omitempty"`
	Extension     string   `json:"extension,omitempty"`
	Discs         int      `json:"discs,omitempty"`
	DiscFiles     []string `json:"discFiles,omitempty"`
	// SearchName is the name proposed for the metadata search; file source only,
	// omitted otherwise [MET-040] (contract v0.14.0). import.apply ignores it.
	SearchName string `json:"searchName,omitempty"`
}

// ImportScanResult is the result of import.scan: candidates and warnings (IMP-008).
type ImportScanResult struct {
	Candidates []ImportCandidate `json:"candidates"`
	Warnings   []string          `json:"warnings"`
}

// ImportApplyParams is the params of import.apply (1..500 candidates).
type ImportApplyParams struct {
	Candidates []ImportCandidate `json:"candidates"`
	// LaunchBoxPath is the Data\Platforms folder the LaunchBox scan read; when set, the images of the
	// LaunchBox candidates are copied (MID-010). Omitted when no LaunchBox folder was used.
	LaunchBoxPath string `json:"launchboxPath,omitempty"`
}

// ImportApplied is one game written by import.apply.
type ImportApplied struct {
	GameID int64  `json:"gameId"`
	Title  string `json:"title"`
}

// ImportSkipped is one candidate that import.apply did not write: "duplicate" (IMP-006)
// or "failed" (IMP-008).
type ImportSkipped struct {
	Title  string `json:"title"`
	Reason string `json:"reason"`
}

// ImportApplyResult is the result of import.apply.
type ImportApplyResult struct {
	Applied []ImportApplied `json:"applied"`
	Skipped []ImportSkipped `json:"skipped"`
}
