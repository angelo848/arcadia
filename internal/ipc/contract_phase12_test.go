package ipc_test

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

// metadataMethods are the eight metadata.* methods added in v0.14.0, in contract order [MET-070, MET-052..MET-058].
var metadataMethods = []string{
	"metadata.search",
	"metadata.apply",
	"metadata.batchStart",
	"metadata.batchStatus",
	"metadata.batchResults",
	"metadata.batchApply",
	"metadata.batchCancel",
	"metadata.batchDiscard",
}

// metadataNewSchemas are the components.schemas added in v0.14.0 [MET-070].
var metadataNewSchemas = []string{
	"MetadataRef", "MetadataCandidate", "MetadataConfidence", "MetadataSearchResult", "MetadataApplyResult",
	"BatchState", "BatchResult", "BatchStatus", "BatchRow", "BatchChoice",
}

// metadataNewErrors are the components.errors added in v0.14.0 [MET-070].
var metadataNewErrors = []string{"BatchNotFound", "BatchBusy", "BatchWrongState"}

// obsoleteReplacement maps the methods that v0.14.0 marks obsolete to the method that replaces them [MET-070].
var obsoleteReplacement = map[string]string{
	"scrape.search": "metadata.search",
	"scrape.apply":  "metadata.apply",
	"media.queue":   "metadata.apply",
}

// loadGeneric parses a contract file without the typed subset, for the "only additions" comparison.
func loadGeneric(t *testing.T, path string) map[string]any {
	t.Helper()
	return mustObject(t, parseFile(t, path), path)
}

func parseFile(t *testing.T, path string) any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return v
}

func mustObject(t *testing.T, v any, what string) map[string]any {
	t.Helper()
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("%s is not a JSON object", what)
	}
	return m
}

func mustArray(t *testing.T, v any, what string) []any {
	t.Helper()
	a, ok := v.([]any)
	if !ok {
		t.Fatalf("%s is not a JSON array", what)
	}
	return a
}

// genericSection returns the object doc[key] of a generic contract ("info", "components", ...).
func genericSection(t *testing.T, doc map[string]any, key string) map[string]any {
	t.Helper()
	return mustObject(t, doc[key], key)
}

// methodName returns the "name" member of one entry of the methods array.
func methodName(m any) string {
	mm, ok := m.(map[string]any)
	if !ok {
		return ""
	}
	s, _ := mm["name"].(string)
	return s
}

// methodIndex returns the position of method name in the methods array, failing the test when absent.
func methodIndex(t *testing.T, methods []any, name string) int {
	t.Helper()
	for i, m := range methods {
		if methodName(m) == name {
			return i
		}
	}
	t.Fatalf("method %q not in contract", name)
	return -1
}

// takeAddedKeys removes from next the keys that next has and prev does not, and returns them sorted.
// Keys that prev has and next lost are reported as errors.
func takeAddedKeys(t *testing.T, what string, prev, next map[string]any) []string {
	t.Helper()
	for k := range prev {
		if _, ok := next[k]; !ok {
			t.Errorf("%s: v0.13.0 entry %q is missing in v0.14.0", what, k)
		}
	}
	var added []string
	for k := range next {
		if _, ok := prev[k]; !ok {
			added = append(added, k)
		}
	}
	slices.Sort(added)
	for _, k := range added {
		delete(next, k)
	}
	return added
}

// assertFieldsWithOptional checks that the schema's required members are its properties minus optional,
// and that the JSON of full (every optional member set) has exactly the properties.
func assertFieldsWithOptional(t *testing.T, what string, schema jsonSchema, optional []string, full any) {
	t.Helper()
	want := sortedKeys(schema.Properties)
	if len(want) == 0 {
		t.Fatalf("contract %s has no properties", what)
	}
	var wantRequired []string
	for _, p := range want {
		if !slices.Contains(optional, p) {
			wantRequired = append(wantRequired, p)
		}
	}
	if got := slices.Sorted(slices.Values(schema.Required)); !slices.Equal(got, wantRequired) {
		t.Errorf("contract %s: required %v, want %v (properties minus optional %v)", what, got, wantRequired, optional)
	}
	if got := jsonFieldNames(t, full); !slices.Equal(got, want) {
		t.Errorf("%T JSON fields = %v, want exactly %v (contract %s)", full, got, want, what)
	}
}

func TestMetadataContract(t *testing.T) {
	t.Parallel()
	c := loadContract(t)

	t.Run("MET-070/versão 0.14.0", func(t *testing.T) {
		t.Parallel()
		if c.Info.Version != "0.14.0" {
			t.Fatalf("contract %s info.version = %q, want \"0.14.0\"", contractPath, c.Info.Version)
		}
	})

	t.Run("MET-070/os 8 métodos metadata.* existem com paramStructure by-name e os nomes das constantes Go", func(t *testing.T) {
		t.Parallel()
		goNames := map[string]string{
			ipc.MethodMetadataSearch:       "metadata.search",
			ipc.MethodMetadataApply:        "metadata.apply",
			ipc.MethodMetadataBatchStart:   "metadata.batchStart",
			ipc.MethodMetadataBatchStatus:  "metadata.batchStatus",
			ipc.MethodMetadataBatchResults: "metadata.batchResults",
			ipc.MethodMetadataBatchApply:   "metadata.batchApply",
			ipc.MethodMetadataBatchCancel:  "metadata.batchCancel",
			ipc.MethodMetadataBatchDiscard: "metadata.batchDiscard",
		}
		for got, want := range goNames {
			if got != want {
				t.Errorf("Go constant = %q, want %q", got, want)
			}
		}
		if len(goNames) != len(metadataMethods) {
			t.Fatalf("test table has %d methods, want %d", len(goNames), len(metadataMethods))
		}

		doc := loadGeneric(t, contractPath)
		methods := mustArray(t, doc["methods"], "methods")
		for _, name := range metadataMethods {
			m := findMethod(t, methods, name)
			if m["paramStructure"] != "by-name" {
				t.Errorf("contract %s paramStructure = %v, want by-name", name, m["paramStructure"])
			}
			if m["x-notification"] == true {
				t.Errorf("contract %s is a notification, want a request", name)
			}
		}

		// Parameters by name and the required ones, from the delta §3 table.
		for _, tc := range []struct {
			method       string
			wantAll      []string
			wantRequired []string
		}{
			{"metadata.search", []string{"platform", "title", "year"}, []string{"title"}},
			{"metadata.apply", []string{"gameId", "refs", "replace"}, []string{"gameId", "refs"}},
			{"metadata.batchStart", []string{"gameIds"}, []string{"gameIds"}},
			{"metadata.batchStatus", nil, nil},
			{"metadata.batchResults", []string{"batchId", "limit", "offset"}, []string{"batchId"}},
			{"metadata.batchApply", []string{"batchId", "choices"}, []string{"batchId", "choices"}},
			{"metadata.batchCancel", []string{"batchId"}, []string{"batchId"}},
			{"metadata.batchDiscard", []string{"batchId"}, []string{"batchId"}},
		} {
			all, required := c.paramNames(t, tc.method)
			if !slices.Equal(all, tc.wantAll) || !slices.Equal(required, tc.wantRequired) {
				t.Errorf("contract %s params = %v (required %v), want %v (required %v)",
					tc.method, all, required, tc.wantAll, tc.wantRequired)
			}
		}

		// The Go param types carry exactly those JSON members.
		for _, tc := range []struct {
			name string
			v    any
			want []string
		}{
			{"MetadataSearchParams", ipc.MetadataSearchParams{Title: "x", Platform: "PC", Year: 1998}, []string{"platform", "title", "year"}},
			{"MetadataApplyParams", ipc.MetadataApplyParams{GameID: 1, Refs: []ipc.MetadataRef{{Provider: "igdb", ProviderID: "1"}}, Replace: true}, []string{"gameId", "refs", "replace"}},
			{"BatchStartParams", ipc.BatchStartParams{GameIDs: []int64{1}}, []string{"gameIds"}},
			{"BatchResultsParams", ipc.BatchResultsParams{BatchID: 1, Offset: 5, Limit: 100}, []string{"batchId", "limit", "offset"}},
			{"BatchIDParams", ipc.BatchIDParams{BatchID: 1}, []string{"batchId"}},
		} {
			if got := jsonFieldNames(t, tc.v); !slices.Equal(got, tc.want) {
				t.Errorf("ipc.%s JSON fields = %v, want exactly %v", tc.name, got, tc.want)
			}
		}
		if got := jsonFieldNames(t, ipc.BatchApplyParams{}); !slices.Equal(got, []string{"batchId", "choices"}) {
			t.Errorf("ipc.BatchApplyParams JSON fields = %v, want [batchId choices]", got)
		}
		// Methods without params serialize no members of their own (metadata.batchStatus has none).
		if all, _ := c.paramNames(t, "metadata.batchStatus"); len(all) != 0 {
			t.Errorf("contract metadata.batchStatus params = %v, want none", all)
		}
	})

	t.Run("MET-070/metadata.batchChanged é notificação com batchId, state, done, total, failed", func(t *testing.T) {
		t.Parallel()
		if ipc.MethodMetadataBatchChanged != "metadata.batchChanged" {
			t.Fatalf("ipc.MethodMetadataBatchChanged = %q", ipc.MethodMetadataBatchChanged)
		}
		var found bool
		for _, m := range c.Methods {
			if m.Name != ipc.MethodMetadataBatchChanged {
				continue
			}
			found = true
			if !m.Notification {
				t.Errorf("contract %s x-notification = false, want true", m.Name)
			}
		}
		if !found {
			t.Fatalf("method %q not in contract", ipc.MethodMetadataBatchChanged)
		}
		all, required := c.paramNames(t, ipc.MethodMetadataBatchChanged)
		want := []string{"batchId", "done", "failed", "state", "total"}
		if !slices.Equal(all, want) || !slices.Equal(required, want) {
			t.Errorf("contract metadata.batchChanged params = %v (required %v), want %v all required", all, required, want)
		}
		if got := jsonFieldNames(t, ipc.MetadataBatchChangedParams{BatchID: 1, State: "searching", Done: 1, Total: 2, Failed: 0}); !slices.Equal(got, want) {
			t.Errorf("ipc.MetadataBatchChangedParams JSON fields = %v, want exactly %v", got, want)
		}
		raw, err := json.Marshal(ipc.MetadataBatchChangedParams{BatchID: 1763217600000, State: "searching", Done: 12, Total: 80, Failed: 1})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var got, exp map[string]any
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatalf("parse %s: %v", raw, err)
		}
		if err := json.Unmarshal([]byte(`{"batchId":1763217600000,"state":"searching","done":12,"total":80,"failed":1}`), &exp); err != nil {
			t.Fatalf("parse example: %v", err)
		}
		if !reflect.DeepEqual(got, exp) {
			t.Errorf("metadata.batchChanged params JSON = %s, want the delta §4 example", raw)
		}
	})

	t.Run("MET-070/campos de cada tipo batem com as propriedades do esquema", func(t *testing.T) {
		t.Parallel()
		// Types whose members are all required: exact match with the zero value (every member present).
		for _, tc := range []struct {
			schema string
			v      any
		}{
			{"MetadataRef", ipc.MetadataRef{}},
			{"MetadataSearchResult", ipc.MetadataSearchResult{}},
			{"MetadataApplyResult", ipc.MetadataApplyResult{}},
			{"BatchResult", ipc.BatchResult{}},
			{"BatchRow", ipc.BatchRow{}},
			{"BatchChoice", ipc.BatchChoice{}},
		} {
			assertExactFields(t, tc.schema, c.schema(t, tc.schema), tc.v)
		}
		// Types with one optional member: every member set.
		assertFieldsWithOptional(t, "MetadataCandidate", c.schema(t, "MetadataCandidate"), []string{"thumbUrl"},
			ipc.MetadataCandidate{
				Refs:     []ipc.MetadataRef{{Provider: "igdb", ProviderID: "1"}},
				Title:    "x",
				Year:     1998,
				Platform: "PC",
				Score:    0.9,
				ThumbURL: "https://images.igdb.com/igdb/image/upload/t_cover_small/x.jpg",
			})
		assertFieldsWithOptional(t, "BatchStatus", c.schema(t, "BatchStatus"), []string{"result"},
			ipc.BatchStatus{Result: &ipc.BatchResult{}})
	})

	t.Run("MET-070/enums: BatchState tem os 5 estados; MetadataConfidence tem match, doubt, none; MetadataRef.provider tem os 3 provedores", func(t *testing.T) {
		t.Parallel()
		if got, want := c.schema(t, "BatchState").Enum, []string{"searching", "ready", "cancelled", "applying", "done"}; !slices.Equal(got, want) {
			t.Errorf("BatchState enum = %v, want %v", got, want)
		}
		assertEnumMatchesDomain(t, "MetadataConfidence", c.schema(t, "MetadataConfidence").Enum,
			[]string{string(domain.ConfidenceMatch), string(domain.ConfidenceDoubt), string(domain.ConfidenceNone)},
			func(s string) error {
				switch domain.Confidence(s) {
				case domain.ConfidenceMatch, domain.ConfidenceDoubt, domain.ConfidenceNone:
					return nil
				}
				return fmt.Errorf("unknown confidence %q", s)
			})
		if got := c.schema(t, "MetadataConfidence").Enum; !slices.Equal(got, []string{"match", "doubt", "none"}) {
			t.Errorf("MetadataConfidence enum = %v, want [match doubt none]", got)
		}

		var provider struct {
			Enum []string `json:"enum"`
		}
		raw := c.schema(t, "MetadataRef").Properties["provider"]
		if err := json.Unmarshal(raw, &provider); err != nil {
			t.Fatalf("parse MetadataRef.provider %s: %v", raw, err)
		}
		if want := []string{"igdb", "steamgriddb", "steamstore"}; !slices.Equal(provider.Enum, want) {
			t.Errorf("MetadataRef.provider enum = %v, want %v", provider.Enum, want)
		}
	})

	t.Run("MET-070/erros -32014, -32015, -32016 com mensagens e data.state", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			name    string
			code    int
			codeGo  int
			message string
			msgGo   string
			hasData bool
		}{
			{"BatchNotFound", -32014, ipc.CodeBatchNotFound, "Metadata batch not found", ipc.MsgBatchNotFound, false},
			{"BatchBusy", -32015, ipc.CodeBatchBusy, "Metadata batch in progress", ipc.MsgBatchBusy, true},
			{"BatchWrongState", -32016, ipc.CodeBatchWrongState, "Wrong batch state", ipc.MsgBatchWrongState, true},
		} {
			e, ok := c.Components.Errors[tc.name]
			if !ok {
				t.Errorf("contract has no error %s", tc.name)
				continue
			}
			if e.Code != tc.code || tc.codeGo != tc.code {
				t.Errorf("%s: contract code %d, Go code %d, want %d", tc.name, e.Code, tc.codeGo, tc.code)
			}
			if e.Message != tc.message || tc.msgGo != tc.message {
				t.Errorf("%s: contract message %q, Go message %q, want %q", tc.name, e.Message, tc.msgGo, tc.message)
			}
			if !tc.hasData {
				if e.Data != nil {
					t.Errorf("%s has a data schema, want none", tc.name)
				}
				continue
			}
			data := c.errorData(t, tc.name)
			if got := slices.Sorted(slices.Values(data.Required)); !slices.Equal(got, []string{"state"}) {
				t.Errorf("%s data required = %v, want [state]", tc.name, got)
			}
			if got := sortedKeys(data.Properties); !slices.Equal(got, []string{"state"}) {
				t.Errorf("%s data properties = %v, want [state]", tc.name, got)
			}
			var st jsonSchema
			if err := json.Unmarshal(data.Properties["state"], &st); err != nil {
				t.Fatalf("%s data.state: %v", tc.name, err)
			}
			if got := c.resolve(t, st).Enum; !slices.Equal(got, c.schema(t, "BatchState").Enum) {
				t.Errorf("%s data.state enum = %v, want BatchState", tc.name, got)
			}
		}
		if err := ipc.NewError(ipc.CodeBatchBusy, ipc.MsgBatchBusy); err.Code != -32015 {
			t.Errorf("NewError(CodeBatchBusy).Code = %d, want -32015", err.Code)
		}
		// The wire data of -32015 and -32016 is exactly {state}: the shape is checked with a local
		// struct, so the test does not depend on a Go name the plan does not fix.
		type stateData struct {
			State string `json:"state"`
		}
		assertExactFields(t, "BatchBusy data", c.errorData(t, "BatchBusy"), stateData{State: "searching"})
		assertExactFields(t, "BatchWrongState data", c.errorData(t, "BatchWrongState"), stateData{State: "ready"})
	})

	t.Run("MET-070/ProviderNoKey e ProviderUnavailable continuam com data.provider", func(t *testing.T) {
		t.Parallel()
		prev := genericSection(t, loadGeneric(t, contractV013Path), "components")
		next := genericSection(t, loadGeneric(t, contractPath), "components")
		prevErrs := mustObject(t, prev["errors"], "v0.13.0 components.errors")
		nextErrs := mustObject(t, next["errors"], "v0.14.0 components.errors")
		for _, name := range []string{"ProviderNoKey", "ProviderUnavailable"} {
			pe := mustObject(t, prevErrs[name], "v0.13.0 "+name)
			ne := mustObject(t, nextErrs[name], "v0.14.0 "+name)
			if !reflect.DeepEqual(pe["data"], ne["data"]) {
				t.Errorf("%s data changed in v0.14.0: %v -> %v", name, pe["data"], ne["data"])
			}
			data := c.errorData(t, name)
			if !slices.Contains(sortedKeys(data.Properties), "provider") {
				t.Errorf("%s data has no provider member", name)
			}
		}
	})

	t.Run("MET-070/x-metadata: valores literais do delta §1 e, para as constantes de domain, igualdade com o código", func(t *testing.T) {
		t.Parallel()
		info := genericSection(t, loadGeneric(t, contractPath), "info")
		xm := mustObject(t, info["x-metadata"], "info.x-metadata")
		if desc, _ := xm["description"].(string); desc == "" {
			t.Error("x-metadata has no description")
		}
		delete(xm, "description")
		want := map[string]any{
			"maxSearchCandidates": 10.0,
			"maxRowCandidates":    5.0,
			"matchAuto":           0.85,
			"matchMinSimilarity":  0.80,
			"matchMargin":         0.05,
			"matchDoubt":          0.60,
			"matchAlternative":    0.40,
			"matchCap":            0.79,
			"searchTimeoutMs":     20000.0,
			"gameSearchTimeoutMs": 15000.0,
			"requestIntervalMs":   map[string]any{"igdb": 334.0, "steamgriddb": 500.0, "steamstore": 2000.0},
			"batchWorkers":        3.0,
			"maxBatchGames":       1000.0,
			"maxBatchChoices":     1000.0,
			"defaultResultsLimit": 100.0,
			"maxResultsLimit":     200.0,
			"maxResultBytes":      921600.0,
			"progressWindowMs":    500.0,
			"providerCooldownMs":  []any{30000.0, 60000.0, 120000.0},
			"providerMaxFailures": 3.0,
			"maxStatusWarnings":   10.0,
			"maxFailedTitles":     10.0,
		}
		if !reflect.DeepEqual(xm, want) {
			t.Errorf("info.x-metadata =\n%v\nwant\n%v", xm, want)
		}

		m := c.Info.Metadata
		if m == nil {
			t.Fatal("contract info has no x-metadata")
		}
		// domain keeps the thresholds in thousandths (integers); the contract has decimals (0.85 = 850/1000).
		for _, tc := range []struct {
			name string
			got  *float64
			want int // thousandths, as in domain
		}{
			{"matchAuto", m.MatchAuto, domain.MatchAuto},
			{"matchMinSimilarity", m.MatchMinSimilarity, domain.MatchMinSimilarity},
			{"matchMargin", m.MatchMargin, domain.MatchMargin},
			{"matchDoubt", m.MatchDoubt, domain.MatchDoubt},
			{"matchAlternative", m.MatchAlternative, domain.MatchAlternative},
			{"matchCap", m.MatchCap, domain.MatchCap},
		} {
			if tc.got == nil {
				t.Errorf("x-metadata has no %s", tc.name)
				continue
			}
			if want := float64(tc.want) / 1000; *tc.got != want {
				t.Errorf("x-metadata.%s = %v, domain constant = %d (%v)", tc.name, *tc.got, tc.want, want)
			}
		}
	})

	t.Run("MET-070/ponteiro nulo de BatchStatusResult serializa batch:null e Next nulo serializa next:null", func(t *testing.T) {
		t.Parallel()
		raw, err := json.Marshal(ipc.BatchStatusResult{})
		if err != nil {
			t.Fatalf("marshal BatchStatusResult: %v", err)
		}
		var st map[string]any
		if err := json.Unmarshal(raw, &st); err != nil {
			t.Fatalf("parse %s: %v", raw, err)
		}
		if v, ok := st["batch"]; !ok || v != nil {
			t.Errorf("BatchStatusResult{} JSON = %s, want batch:null", raw)
		}

		raw, err = json.Marshal(ipc.BatchResultsResult{Rows: []ipc.BatchRow{}})
		if err != nil {
			t.Fatalf("marshal BatchResultsResult: %v", err)
		}
		var rs map[string]any
		if err := json.Unmarshal(raw, &rs); err != nil {
			t.Fatalf("parse %s: %v", raw, err)
		}
		if v, ok := rs["next"]; !ok || v != nil {
			t.Errorf("BatchResultsResult with nil Next JSON = %s, want next:null", raw)
		}
		if _, ok := rs["rows"].([]any); !ok {
			t.Errorf("BatchResultsResult with empty rows JSON = %s, want rows as an array", raw)
		}

		// The optional result of BatchStatus is omitted while nil [MET-065, MET-066].
		raw, err = json.Marshal(ipc.BatchStatus{BatchID: 1, State: "ready"})
		if err != nil {
			t.Fatalf("marshal BatchStatus: %v", err)
		}
		var bs map[string]any
		if err := json.Unmarshal(raw, &bs); err != nil {
			t.Fatalf("parse %s: %v", raw, err)
		}
		if _, has := bs["result"]; has {
			t.Errorf("BatchStatus with nil Result JSON = %s, want no result member", raw)
		}
	})

	t.Run("MET-054/params de metadata.batchChanged só têm inteiros e o estado (sem dados de jogo)", func(t *testing.T) {
		t.Parallel()
		methods := mustArray(t, loadGeneric(t, contractPath)["methods"], "methods")
		m := findMethod(t, methods, ipc.MethodMetadataBatchChanged)
		params := mustArray(t, m["params"], "metadata.batchChanged params")
		if len(params) == 0 {
			t.Fatal("metadata.batchChanged has no params")
		}
		for _, p := range params {
			pm := mustObject(t, p, "param")
			name, _ := pm["name"].(string)
			schema := mustObject(t, pm["schema"], "param "+name+" schema")
			switch name {
			case "state":
				if schema["$ref"] != "#/components/schemas/BatchState" {
					t.Errorf("state schema = %v, want $ref BatchState", schema)
				}
			case "batchId", "done", "total", "failed":
				if schema["type"] != "integer" {
					t.Errorf("%s type = %v, want integer", name, schema["type"])
				}
			default:
				t.Errorf("metadata.batchChanged has param %q; only batchId, state, done, total and failed are allowed (no game data)", name)
			}
		}
	})

	t.Run("MET-040/searchName é opcional e omitido: candidato de Steam, Epic, LaunchBox e de pasta serializa byte a byte como na v0.13.0", func(t *testing.T) {
		t.Parallel()
		// Same bytes as the v0.13.0 candidates (the IMV-024 case), per source.
		const tmpl = `{"source":"%s","title":"x","platform":"","launchType":"%s","launchTarget":"%s","installDir":"","developers":null,"publishers":null,"genres":null,"year":null,"description":"","playModes":null,"playedSeconds":null,"lastPlayed":null,"exists":false}`
		for _, tc := range []struct {
			source, launchType, target string // target is the Go value (a Windows path has single backslashes)
		}{
			{ipc.ImportSourceSteam, "uri", "steam://rungameid/1"},
			{ipc.ImportSourceEpic, "uri", "com.epicgames.launcher://apps/x"},
			{ipc.ImportSourceLaunchBox, "exe", `C:\Jogos\x.exe`},
			{ipc.ImportSourceFolder, "exe", `C:\Jogos\x\x.exe`},
		} {
			raw, err := json.Marshal(ipc.ImportCandidate{
				Source:       tc.source,
				Title:        "x",
				LaunchType:   tc.launchType,
				LaunchTarget: tc.target,
			})
			if err != nil {
				t.Fatalf("marshal %s candidate: %v", tc.source, err)
			}
			// JSON escapes each backslash of the path.
			want := fmt.Sprintf(tmpl, tc.source, tc.launchType, strings.ReplaceAll(tc.target, `\`, `\\`))
			if string(raw) != want {
				t.Errorf("%s candidate JSON =\n%s\nwant (v0.13.0 bytes)\n%s", tc.source, raw, want)
			}
		}
	})

	t.Run("MET-040/searchName aparece quando preenchido", func(t *testing.T) {
		t.Parallel()
		raw, err := json.Marshal(ipc.ImportCandidate{
			Source:       ipc.ImportSourceFile,
			Title:        "Final Fantasy VIII",
			LaunchType:   "exe",
			LaunchTarget: `E:\Jogos\ff8.exe`,
			SearchName:   "Final Fantasy VIII",
		})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("parse %s: %v", raw, err)
		}
		if m["searchName"] != "Final Fantasy VIII" {
			t.Errorf("searchName in %s = %v, want the filled value", raw, m["searchName"])
		}

		p := importProp(t, c, "ImportCandidate", "searchName")
		if p.Type != "string" || p.MaxLength == nil || *p.MaxLength != 200 {
			t.Errorf("ImportCandidate.searchName type %q maxLength %v, want string with maxLength 200", p.Type, p.MaxLength)
		}
		if slices.Contains(c.schema(t, "ImportCandidate").Required, "searchName") {
			t.Error("ImportCandidate.searchName is required, want optional [MET-040]")
		}
	})

	t.Run("MET-070/v0.14.0 é a v0.13.0 mais acréscimos", func(t *testing.T) {
		t.Parallel()
		prev := loadGeneric(t, contractV013Path)
		next := loadGeneric(t, contractPath)

		// info: version and description change; x-metadata is new.
		pinfo := genericSection(t, prev, "info")
		ninfo := genericSection(t, next, "info")
		if pinfo["version"] != "0.13.0" || ninfo["version"] != "0.14.0" {
			t.Errorf("info.version v0.13.0 = %v, v0.14.0 = %v", pinfo["version"], ninfo["version"])
		}
		if _, ok := ninfo["x-metadata"]; !ok {
			t.Error("v0.14.0 info has no x-metadata")
		}
		delete(pinfo, "version")
		delete(pinfo, "description")
		delete(ninfo, "version")
		delete(ninfo, "description")
		delete(ninfo, "x-metadata")

		// methods: the eight new ones follow scrape.apply in order, batchChanged follows media.changed.
		pMethods := mustArray(t, prev["methods"], "v0.13.0 methods")
		nMethods := mustArray(t, next["methods"], "v0.14.0 methods")
		iApply := methodIndex(t, nMethods, "scrape.apply")
		for i, name := range metadataMethods {
			pos := iApply + 1 + i
			if pos >= len(nMethods) || methodName(nMethods[pos]) != name {
				t.Errorf("v0.14.0 method after scrape.apply #%d = %q, want %q", i, methodName(nMethods[pos%len(nMethods)]), name)
			}
		}
		if got, want := methodIndex(t, nMethods, ipc.MethodMetadataBatchChanged), methodIndex(t, nMethods, "media.changed")+1; got != want {
			t.Errorf("metadata.batchChanged at %d, want right after media.changed (%d)", got, want)
		}

		// scrape.search, scrape.apply and media.queue: "Obsolete (use …)" prefix on the summary, deprecated:true; nothing else changes.
		for _, name := range []string{"scrape.search", "scrape.apply", "media.queue"} {
			pm := findMethod(t, pMethods, name)
			nm := findMethod(t, nMethods, name)
			if _, has := pm["deprecated"]; has {
				t.Errorf("v0.13.0 %s has a deprecated member", name)
			}
			if strings.Contains(strings.ToLower(pm["summary"].(string)), "obsolete") {
				t.Errorf("v0.13.0 %s summary mentions obsolete", name)
			}
			if nm["deprecated"] != true {
				t.Errorf("v0.14.0 %s deprecated = %v, want true", name, nm["deprecated"])
			}
			wantSummary := "Obsolete (use " + obsoleteReplacement[name] + "; still served, same behaviour; the image limit of MET-022 applies). " + pm["summary"].(string)
			if nm["summary"] != wantSummary {
				t.Errorf("v0.14.0 %s summary = %q, want %q", name, nm["summary"], wantSummary)
			}
			delete(nm, "deprecated")
			nm["summary"] = pm["summary"]
		}

		// Drop the new methods (and the notification) and compare the rest in order.
		newMethods := map[string]bool{ipc.MethodMetadataBatchChanged: true}
		for _, name := range metadataMethods {
			newMethods[name] = true
		}
		var kept []any
		for _, m := range nMethods {
			if newMethods[methodName(m)] {
				continue
			}
			if mm := mustObject(t, m, "method"); mm["deprecated"] != nil {
				t.Errorf("v0.14.0 method %s has a deprecated member outside scrape.search/scrape.apply/media.queue", methodName(m))
			}
			kept = append(kept, m)
		}
		next["methods"] = kept

		// components: the new schemas and errors are the only added keys; ImportCandidate gains searchName only.
		pComps := genericSection(t, prev, "components")
		nComps := genericSection(t, next, "components")
		pSchemas := mustObject(t, pComps["schemas"], "v0.13.0 components.schemas")
		nSchemas := mustObject(t, nComps["schemas"], "v0.14.0 components.schemas")
		if got, want := takeAddedKeys(t, "components.schemas", pSchemas, nSchemas), slices.Sorted(slices.Values(metadataNewSchemas)); !slices.Equal(got, want) {
			t.Errorf("new components.schemas = %v, want %v", got, want)
		}
		nCandProps := genericSchemaProps(t, next, "ImportCandidate")
		if _, ok := nCandProps["searchName"]; !ok {
			t.Error("v0.14.0 ImportCandidate has no searchName")
		}
		delete(nCandProps, "searchName")

		pErrs := mustObject(t, pComps["errors"], "v0.13.0 components.errors")
		nErrs := mustObject(t, nComps["errors"], "v0.14.0 components.errors")
		if got, want := takeAddedKeys(t, "components.errors", pErrs, nErrs), slices.Sorted(slices.Values(metadataNewErrors)); !slices.Equal(got, want) {
			t.Errorf("new components.errors = %v, want %v", got, want)
		}

		if !reflect.DeepEqual(prev, next) {
			t.Error("v0.14.0 differs from v0.13.0 beyond info.version and description, x-metadata, the eight metadata.* methods, metadata.batchChanged, the obsolete summaries and deprecated flags, the new schemas and errors and ImportCandidate.searchName")
		}
	})
}
