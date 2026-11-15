package ipc_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

// contractV012Path is the phase-10-2 contract (v0.12.0), the base of the v0.13.0 delta [IMV-082].
var contractV012Path = filepath.Join("..", "..", "docs", "specs", "phase-10-2-imagens-ao-vivo", "contracts", "ipc.openrpc.json")

// importScanNewProps are the optional params that v0.13.0 adds to ImportScanParams [IMV-070].
var importScanNewProps = []string{"platform", "recursive", "titleFrom"}

// importCandidateNewProps are the optional fields that v0.13.0 adds to ImportCandidate [IMV-024, IMV-070].
var importCandidateNewProps = []string{"discFiles", "discs", "extension", "file"}

// importRawProp is the part of a property schema the import tests read.
type importRawProp struct {
	Type      string          `json:"type"`
	Enum      []string        `json:"enum"`
	Default   json.RawMessage `json:"default"`
	MaxLength *int            `json:"maxLength"`
	OneOf     []struct {
		Type   string `json:"type"`
		Format string `json:"format"`
	} `json:"oneOf"`
}

// importProp parses components.schemas[schema].properties[prop] of the contract.
func importProp(t *testing.T, c openRPC, schema, prop string) importRawProp {
	t.Helper()
	raw, ok := c.schema(t, schema).Properties[prop]
	if !ok {
		t.Fatalf("contract %s has no property %s", schema, prop)
	}
	var p importRawProp
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatalf("parse %s.%s %s: %v", schema, prop, raw, err)
	}
	return p
}

// v13FullCandidate is an ImportCandidate with every field set (the 19 JSON members of v0.13.0).
func v13FullCandidate() ipc.ImportCandidate {
	year := 1998
	seconds := int64(3600)
	last := "2026-11-01T10:00:00Z"
	return ipc.ImportCandidate{
		Source:        ipc.ImportSourceFolder,
		Title:         "Final Fantasy VIII",
		Platform:      "Sony Playstation",
		LaunchType:    "emulator",
		LaunchTarget:  `E:\Games\ps1\04. Final Fantasy VIII (USA)\FF8 (Disc 1).cue`,
		InstallDir:    `E:\Games\ps1\04. Final Fantasy VIII (USA)\FF8 (Disc 1)`,
		Developers:    []string{"Square"},
		Publishers:    []string{"Square"},
		Genres:        []string{"RPG"},
		Year:          &year,
		Description:   "desc",
		PlayModes:     []string{"single"},
		PlayedSeconds: &seconds,
		LastPlayed:    &last,
		Exists:        true,
		File:          `04. Final Fantasy VIII (USA)\FF8 (Disc 1)\FF8 (Disc 1).cue`,
		Extension:     "cue",
		Discs:         2,
		DiscFiles:     []string{`04. Final Fantasy VIII (USA)\FF8 (Disc 1)\FF8 (Disc 1).cue`, `04. Final Fantasy VIII (USA)\FF8 (Disc 2)\FF8 (Disc 2).cue`},
	}
}

// findMethod returns the object of the method named name inside the "methods" array of a generic contract.
func findMethod(t *testing.T, methods any, name string) map[string]any {
	t.Helper()
	list, ok := methods.([]any)
	if !ok {
		t.Fatal("contract has no methods array")
	}
	for _, m := range list {
		if mm, ok := m.(map[string]any); ok && mm["name"] == name {
			return mm
		}
	}
	t.Fatalf("method %q not in contract", name)
	return nil
}

// genericSchemaProps returns components.schemas[name].properties of a generic contract.
func genericSchemaProps(t *testing.T, doc map[string]any, name string) map[string]any {
	t.Helper()
	comps, ok := doc["components"].(map[string]any)
	if !ok {
		t.Fatal("contract has no components object")
	}
	schemas, ok := comps["schemas"].(map[string]any)
	if !ok {
		t.Fatal("contract has no components.schemas object")
	}
	s, ok := schemas[name].(map[string]any)
	if !ok {
		t.Fatalf("contract has no components.schemas.%s", name)
	}
	props, ok := s["properties"].(map[string]any)
	if !ok {
		t.Fatalf("components.schemas.%s has no properties", name)
	}
	return props
}

func TestImportV2Contract(t *testing.T) {
	t.Parallel()
	c := loadContract(t)

	t.Run("IMV-070/ImportScanParams tem source, path, platform, titleFrom e recursive; os três novos opcionais", func(t *testing.T) {
		t.Parallel()
		wantProps := []string{"path", "platform", "recursive", "source", "titleFrom"}
		if got := sortedKeys(c.schema(t, "ImportScanParams").Properties); !slices.Equal(got, wantProps) {
			t.Errorf("ImportScanParams properties = %v, want %v", got, wantProps)
		}
		if got := slices.Sorted(slices.Values(c.schema(t, "ImportScanParams").Required)); !slices.Equal(got, []string{"source"}) {
			t.Errorf("ImportScanParams required = %v, want [source] (the new params are optional)", got)
		}
		recursive := false
		got := jsonFieldNames(t, ipc.ImportScanParams{
			Source:    ipc.ImportSourceFolder,
			Path:      `C:\Jogos`,
			Platform:  "Windows",
			TitleFrom: ipc.ImportTitleFromFolder,
			Recursive: &recursive,
		})
		if !slices.Equal(got, wantProps) {
			t.Errorf("ipc.ImportScanParams JSON fields = %v, want exactly %v", got, wantProps)
		}
	})

	t.Run("IMV-070/titleFrom é enum file/folder com default file; recursive é boolean com default true; platform tem maxLength 100", func(t *testing.T) {
		t.Parallel()
		tf := importProp(t, c, "ImportScanParams", "titleFrom")
		if tf.Type != "string" || !slices.Equal(tf.Enum, []string{"file", "folder"}) {
			t.Errorf("titleFrom type %q enum %v, want string enum [file folder]", tf.Type, tf.Enum)
		}
		if string(tf.Default) != `"file"` {
			t.Errorf("titleFrom default = %s, want \"file\"", tf.Default)
		}

		rec := importProp(t, c, "ImportScanParams", "recursive")
		if rec.Type != "boolean" {
			t.Errorf("recursive type = %q, want boolean", rec.Type)
		}
		if string(rec.Default) != `true` {
			t.Errorf("recursive default = %s, want true", rec.Default)
		}

		pl := importProp(t, c, "ImportScanParams", "platform")
		if pl.Type != "string" || pl.MaxLength == nil || *pl.MaxLength != 100 {
			t.Errorf("platform type %q maxLength %v, want string with maxLength 100", pl.Type, pl.MaxLength)
		}
	})

	t.Run("IMV-070/ImportCandidate tem file, extension, discs e discFiles; nenhum em required", func(t *testing.T) {
		t.Parallel()
		props := sortedKeys(c.schema(t, "ImportCandidate").Properties)
		for _, p := range importCandidateNewProps {
			if !slices.Contains(props, p) {
				t.Errorf("ImportCandidate has no property %s", p)
			}
		}
		wantRequired := []string{"launchTarget", "launchType", "source", "title"}
		if got := slices.Sorted(slices.Values(c.schema(t, "ImportCandidate").Required)); !slices.Equal(got, wantRequired) {
			t.Errorf("ImportCandidate required = %v, want %v", got, wantRequired)
		}
		if got := jsonFieldNames(t, v13FullCandidate()); !slices.Equal(got, props) {
			t.Errorf("ipc.ImportCandidate JSON fields = %v, want exactly %v (contract properties)", got, props)
		}
		if got := importProp(t, c, "ImportCandidate", "discs"); got.Type != "integer" {
			t.Errorf("discs type = %q, want integer", got.Type)
		}
		if got := importProp(t, c, "ImportCandidate", "discFiles"); got.Type != "array" {
			t.Errorf("discFiles type = %q, want array", got.Type)
		}
	})

	t.Run("IMV-024/campos novos codificam com omitempty: candidato de Steam serializa byte a byte como na v0.12.0", func(t *testing.T) {
		t.Parallel()
		raw, err := json.Marshal(ipc.ImportCandidate{
			Source:       ipc.ImportSourceSteam,
			Title:        "x",
			LaunchType:   "uri",
			LaunchTarget: "steam://rungameid/1",
		})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		want := `{"source":"steam","title":"x","platform":"","launchType":"uri","launchTarget":"steam://rungameid/1","installDir":"","developers":null,"publishers":null,"genres":null,"year":null,"description":"","playModes":null,"playedSeconds":null,"lastPlayed":null,"exists":false}`
		if string(raw) != want {
			t.Errorf("Steam candidate JSON =\n%s\nwant (v0.12.0 bytes)\n%s", raw, want)
		}
	})

	t.Run("IMV-070/Recursive nil não aparece; ponteiro para false aparece como false", func(t *testing.T) {
		t.Parallel()
		if got := jsonFieldNames(t, ipc.ImportScanParams{Source: ipc.ImportSourceFolder, Path: `C:\Jogos`}); slices.Contains(got, "recursive") {
			t.Errorf("ImportScanParams with Recursive nil has key recursive: %v", got)
		}

		f := false
		raw, err := json.Marshal(ipc.ImportScanParams{Source: ipc.ImportSourceFolder, Path: `C:\Jogos`, Recursive: &f})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("parse %s: %v", raw, err)
		}
		if b, ok := m["recursive"].(bool); !ok || b {
			t.Errorf("recursive in %s = %v, want false", raw, m["recursive"])
		}
	})

	t.Run("IMV-070/x-import: maxScanDepth 8, maxScanItems 20000, maxScanResultBytes 921600", func(t *testing.T) {
		t.Parallel()
		if c.Info.Import == nil {
			t.Fatal("contract info has no x-import")
		}
		for _, tc := range []struct {
			name string
			got  *int
			want int
		}{
			{"maxScanDepth", c.Info.Import.MaxScanDepth, 8},
			{"maxScanItems", c.Info.Import.MaxScanItems, 20000},
			{"maxScanResultBytes", c.Info.Import.MaxScanResultBytes, 921600},
		} {
			if tc.got == nil {
				t.Errorf("contract x-import has no %s", tc.name)
				continue
			}
			if *tc.got != tc.want {
				t.Errorf("x-import.%s = %d, want %d", tc.name, *tc.got, tc.want)
			}
		}
	})

	t.Run("IMV-070/ImportCandidate.launchType lista exe, uri e emulator; lastPlayed é string date-time ou nulo", func(t *testing.T) {
		t.Parallel()
		lt := importProp(t, c, "ImportCandidate", "launchType")
		if want := []string{"exe", "uri", "emulator"}; !slices.Equal(lt.Enum, want) {
			t.Errorf("ImportCandidate.launchType enum = %v, want %v", lt.Enum, want)
		}

		lp := importProp(t, c, "ImportCandidate", "lastPlayed")
		if len(lp.OneOf) != 2 {
			t.Fatalf("ImportCandidate.lastPlayed oneOf has %d members, want 2 (string date-time, null)", len(lp.OneOf))
		}
		if lp.OneOf[0].Type != "string" || lp.OneOf[0].Format != "date-time" {
			t.Errorf("lastPlayed oneOf[0] = %+v, want string with format date-time", lp.OneOf[0])
		}
		if lp.OneOf[1].Type != "null" {
			t.Errorf("lastPlayed oneOf[1] = %+v, want null", lp.OneOf[1])
		}
	})

	t.Run("IMV-082/v0.13.0 é a v0.12.0 mais os campos novos e x-import, nada mais", func(t *testing.T) {
		t.Parallel()
		load := func(path string) map[string]any {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			var m map[string]any
			if err := json.Unmarshal(raw, &m); err != nil {
				t.Fatalf("parse %s: %v", path, err)
			}
			return m
		}
		prev := load(contractV012Path)
		next := load(contractPath)

		// info: only the version, the description and x-import are new.
		pinfo, ok := prev["info"].(map[string]any)
		if !ok {
			t.Fatal("v0.12.0 has no info object")
		}
		ninfo, ok := next["info"].(map[string]any)
		if !ok {
			t.Fatal("v0.13.0 has no info object")
		}
		for _, k := range []string{"version", "description"} {
			delete(pinfo, k)
			delete(ninfo, k)
		}
		delete(ninfo, "x-import")

		// import.scan: the summary text is the only change of the method.
		pm := findMethod(t, prev["methods"], "import.scan")
		nm := findMethod(t, next["methods"], "import.scan")
		nm["summary"] = pm["summary"]

		// ImportScanParams: the three new optional params go away.
		nScanProps := genericSchemaProps(t, next, "ImportScanParams")
		for _, p := range importScanNewProps {
			delete(nScanProps, p)
		}

		// ImportCandidate: the four new fields go away, and the two corrections of v0.12.0 divergences
		// (launchType lists emulator, lastPlayed is a date-time string) are normalized to the old value.
		pCandProps := genericSchemaProps(t, prev, "ImportCandidate")
		nCandProps := genericSchemaProps(t, next, "ImportCandidate")
		for _, p := range importCandidateNewProps {
			delete(nCandProps, p)
		}
		nCandProps["launchType"] = pCandProps["launchType"]
		nCandProps["lastPlayed"] = pCandProps["lastPlayed"]

		if !reflect.DeepEqual(prev, next) {
			t.Error("v0.13.0 differs from v0.12.0 beyond x-import, the version and description, the summary of import.scan, the three ImportScanParams params, the four ImportCandidate fields and the two corrections")
		}
	})
}
