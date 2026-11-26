package ipc_test

// Tests for T16 of docs/specs/phase-16-edicao-sessoes/tasks.md: contract IPC v0.17.0 [SED-030, SED-033, SED-034,
// SED-027]. They stay red until T17 (Go types) exists.

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

// sessionEditMethods are the three methods of v0.17.0, in contract order (right after session.list).
var sessionEditMethods = []string{"session.create", "session.update", "session.delete"}

// phase16NewSchemas are the components.schemas added in v0.17.0 (the *Additions of the delta are merged, not copied).
var phase16NewSchemas = []string{
	"SessionDeleteResult", "SessionEditReason", "SessionOverlapData", "SessionSource", "SessionWriteResult",
}

// jsonTagNames returns the JSON member names declared by the struct tags of v (ignores omitempty and
// the zero-value marshalling, which matters for ipc.Optional).
func jsonTagNames(t *testing.T, v any) []string {
	t.Helper()
	rt := reflect.TypeOf(v)
	var out []string
	for i := range rt.NumField() {
		tag := rt.Field(i).Tag.Get("json")
		name, _, _ := strings.Cut(tag, ",")
		if name == "" || name == "-" {
			t.Fatalf("%T field %s has no json name", v, rt.Field(i).Name)
		}
		out = append(out, name)
	}
	slices.Sort(out)
	return out
}

// removeSchemaProp deletes prop (and its required entry) from components.schemas[schema], failing if absent.
func removeSchemaProp(t *testing.T, doc map[string]any, schema, prop string) {
	t.Helper()
	s := mustObject(t, mustObject(t, genericSection(t, doc, "components")["schemas"], "schemas")[schema], schema)
	props := mustObject(t, s["properties"], schema+".properties")
	if _, ok := props[prop]; !ok {
		t.Errorf("%s has no property %q in v0.17.0", schema, prop)
		return
	}
	delete(props, prop)
	var req []any
	found := false
	for _, r := range mustArray(t, s["required"], schema+".required") {
		if r == prop {
			found = true
			continue
		}
		req = append(req, r)
	}
	if !found {
		t.Errorf("%s.%s is not required in v0.17.0", schema, prop)
	}
	s["required"] = req
}

func TestSessionEditContract(t *testing.T) {
	t.Parallel()
	c := loadContract(t)
	doc := loadGeneric(t, contractPath)

	t.Run("SED-030/versão 0.17.0 e o comentário do pacote aponta para phase-16-edicao-sessoes/contracts/ipc.openrpc.json", func(t *testing.T) {
		t.Parallel()
		if c.Info.Version != "0.17.0" {
			t.Fatalf("contract %s info.version = %q, want \"0.17.0\"", contractPath, c.Info.Version)
		}
		if contractRelPath != "docs/specs/phase-16-edicao-sessoes/contracts/ipc.openrpc.json" {
			t.Fatalf("contractRelPath = %q, want the phase-16 contract", contractRelPath)
		}
		files, err := filepath.Glob("*.go")
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			af, err := parser.ParseFile(token.NewFileSet(), f, nil, parser.ParseComments|parser.PackageClauseOnly)
			if err != nil || af.Doc == nil {
				continue
			}
			if strings.Contains(af.Doc.Text(), "phase-16-edicao-sessoes/contracts/ipc.openrpc.json") {
				found = true
			}
		}
		if !found {
			t.Error("no package comment of internal/ipc names docs/specs/phase-16-edicao-sessoes/contracts/ipc.openrpc.json")
		}
	})

	t.Run("SED-030/os três métodos session.* existem com paramStructure by-name e os nomes das constantes Go", func(t *testing.T) {
		t.Parallel()
		for got, want := range map[string]string{
			ipc.MethodSessionCreate: "session.create",
			ipc.MethodSessionUpdate: "session.update",
			ipc.MethodSessionDelete: "session.delete",
		} {
			if got != want {
				t.Errorf("Go constant = %q, want %q", got, want)
			}
		}
		methods := mustArray(t, doc["methods"], "methods")
		list := methodIndex(t, methods, "session.list")
		for i, name := range sessionEditMethods {
			m := methodByName(t, doc, name)
			if m["paramStructure"] != "by-name" || m["x-notification"] == true {
				t.Errorf("%s = by-name request expected, got %v", name, m)
			}
			if idx := methodIndex(t, methods, name); idx != list+1+i {
				t.Errorf("%s is at index %d, want %d (right after session.list)", name, idx, list+1+i)
			}
		}
	})

	t.Run("SED-030/params de session.create, session.update e session.delete batem com os tipos Go (assertExactFields) e os obrigatórios são os do delta", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			method       string
			wantAll      []string
			wantRequired []string
			v            any
			errs         []string
		}{
			{"session.create", []string{"activeSeconds", "endedAt", "gameId", "includedInTotal", "startedAt"},
				[]string{"activeSeconds", "endedAt", "gameId", "startedAt"}, ipc.SessionCreateParams{},
				[]string{"InvalidParams", "GameNotFound", "SessionOverlap", "InternalError"}},
			{"session.update", []string{"activeSeconds", "endedAt", "sessionId", "startedAt"},
				[]string{"sessionId"}, ipc.SessionUpdateParams{},
				[]string{"InvalidParams", "SessionActive", "SessionNotFound", "SessionOverlap", "InternalError"}},
			{"session.delete", []string{"sessionId"}, []string{"sessionId"}, ipc.SessionDeleteParams{},
				[]string{"InvalidParams", "SessionActive", "SessionNotFound", "InternalError"}},
		} {
			all, required := c.paramNames(t, tc.method)
			if !slices.Equal(all, tc.wantAll) || !slices.Equal(required, tc.wantRequired) {
				t.Errorf("%s params = %v (required %v), want %v (required %v)", tc.method, all, required, tc.wantAll, tc.wantRequired)
			}
			if got := jsonTagNames(t, tc.v); !slices.Equal(got, tc.wantAll) {
				t.Errorf("%T JSON members = %v, want %v", tc.v, got, tc.wantAll)
			}
			if got := methodErrorRefs(t, methodByName(t, doc, tc.method)); !slices.Equal(got, tc.errs) {
				t.Errorf("%s errors = %v, want %v", tc.method, got, tc.errs)
			}
		}
		// Create: the four required members are always on the wire; includedInTotal is omitted when false.
		if got := jsonFieldNames(t, ipc.SessionCreateParams{}); !slices.Equal(got, []string{"activeSeconds", "endedAt", "gameId", "startedAt"}) {
			t.Errorf("SessionCreateParams{} JSON fields = %v, want the four required members", got)
		}
		if got := jsonFieldNames(t, ipc.SessionCreateParams{IncludedInTotal: true}); !slices.Equal(got, []string{"activeSeconds", "endedAt", "gameId", "includedInTotal", "startedAt"}) {
			t.Errorf("SessionCreateParams{IncludedInTotal} JSON fields = %v", got)
		}
		if got := jsonFieldNames(t, ipc.SessionDeleteParams{}); !slices.Equal(got, []string{"sessionId"}) {
			t.Errorf("SessionDeleteParams JSON fields = %v", got)
		}
		if s := paramSchema(t, methodByName(t, doc, "session.create"), "includedInTotal"); s["type"] != "boolean" || s["default"] != false {
			t.Errorf("includedInTotal schema = %v, want boolean default false", s)
		}
		for _, name := range sessionEditMethods {
			if s := paramSchema(t, methodByName(t, doc, name), map[string]string{
				"session.create": "gameId", "session.update": "sessionId", "session.delete": "sessionId",
			}[name]); s["type"] != "integer" || s["minimum"] != 1.0 {
				t.Errorf("%s id schema = %v, want integer minimum 1", name, s)
			}
		}
	})

	t.Run("SED-030/session.update: startedAt, endedAt e activeSeconds opcionais", func(t *testing.T) {
		t.Parallel()
		m := methodByName(t, doc, "session.update")
		_, required := c.paramNames(t, "session.update")
		if !slices.Equal(required, []string{"sessionId"}) {
			t.Errorf("required = %v, want only sessionId", required)
		}
		for _, p := range []string{"startedAt", "endedAt"} {
			if s := paramSchema(t, m, p); s["type"] != "string" || s["format"] != "date-time" {
				t.Errorf("%s schema = %v, want string date-time", p, s)
			}
		}
		s := paramSchema(t, m, "activeSeconds")
		if s["type"] != "integer" || s["minimum"] != 30.0 {
			t.Errorf("activeSeconds schema = %v, want integer minimum 30", s)
		}
		if _, ok := s["maximum"]; ok {
			t.Errorf("activeSeconds has a maximum (%v); an imported session may exceed 86400 (SED-021)", s["maximum"])
		}
		// Server-side Optional members.
		var p ipc.SessionUpdateParams
		if p.StartedAt.Present || p.EndedAt.Present || p.ActiveSeconds.Present {
			t.Error("zero SessionUpdateParams must have no member present")
		}
	})

	t.Run("SED-030/os resultados SessionWriteResult e SessionDeleteResult batem com os tipos Go", func(t *testing.T) {
		t.Parallel()
		for _, name := range []string{"SessionWriteResult", "SessionDeleteResult"} {
			c.schema(t, name)
		}
		assertExactFields(t, "SessionWriteResult", c.schema(t, "SessionWriteResult"), ipc.SessionWriteResult{})
		assertExactFields(t, "SessionDeleteResult", c.schema(t, "SessionDeleteResult"), ipc.SessionDeleteResult{})
		assertExactFields(t, "session.create result", c.resultSchema(t, "session.create"), ipc.SessionWriteResult{})
		assertExactFields(t, "session.update result", c.resultSchema(t, "session.update"), ipc.SessionWriteResult{})
		assertExactFields(t, "session.delete result", c.resultSchema(t, "session.delete"), ipc.SessionDeleteResult{})
	})

	t.Run("SED-030/o enum source é tracked, import, manual e bate com as constantes de domain.SessionSource", func(t *testing.T) {
		t.Parallel()
		enum := c.schema(t, "SessionSource").Enum
		if !slices.Equal(enum, []string{"tracked", "import", "manual"}) {
			t.Errorf("SessionSource enum = %v, want [tracked import manual]", enum)
		}
		assertEnumMatchesDomain(t, "SessionSource", enum,
			[]string{string(domain.SourceTracked), string(domain.SourceImport), string(domain.SourceManual)},
			func(s string) error { _, err := domain.ParseSessionSource(s); return err })
	})

	t.Run("SED-030/o enum reason de SessionEditReason bate com as constantes de domain.EditReason, na mesma ordem", func(t *testing.T) {
		t.Parallel()
		want := []string{
			string(domain.ReasonEndNotAfterStart), string(domain.ReasonInFuture), string(domain.ReasonTooEarly),
			string(domain.ReasonSpanTooLong), string(domain.ReasonActiveTooShort), string(domain.ReasonActiveExceedsSpan),
			string(domain.ReasonDiscarded), string(domain.ReasonExceedsAdjustment), string(domain.ReasonEmptyPatch),
		}
		if got := c.schema(t, "SessionEditReason").Enum; !slices.Equal(got, want) {
			t.Errorf("SessionEditReason enum = %v, want %v", got, want)
		}
		if got := jsonFieldNames(t, ipc.InvalidParamsReasonData{Reason: "x"}); !slices.Equal(got, []string{"reason"}) {
			t.Errorf("InvalidParamsReasonData JSON fields = %v, want [reason]", got)
		}
	})

	t.Run("SED-030/os erros -32017 e -32018 batem com ipc.CodeSessionNotFound, ipc.CodeSessionOverlap e as mensagens; SessionOverlapData bate com o tipo Go", func(t *testing.T) {
		t.Parallel()
		if ipc.CodeSessionNotFound != -32017 || ipc.CodeSessionOverlap != -32018 {
			t.Errorf("codes = %d, %d, want -32017, -32018", ipc.CodeSessionNotFound, ipc.CodeSessionOverlap)
		}
		nf, ov := c.Components.Errors["SessionNotFound"], c.Components.Errors["SessionOverlap"]
		if nf.Code != ipc.CodeSessionNotFound || nf.Message != ipc.MsgSessionNotFound {
			t.Errorf("SessionNotFound = %d %q, want %d %q", nf.Code, nf.Message, ipc.CodeSessionNotFound, ipc.MsgSessionNotFound)
		}
		if ov.Code != ipc.CodeSessionOverlap || ov.Message != ipc.MsgSessionOverlap {
			t.Errorf("SessionOverlap = %d %q, want %d %q", ov.Code, ov.Message, ipc.CodeSessionOverlap, ipc.MsgSessionOverlap)
		}
		if ipc.MsgSessionNotFound != "Session not found" || ipc.MsgSessionOverlap != "Session overlaps another" {
			t.Errorf("messages = %q, %q", ipc.MsgSessionNotFound, ipc.MsgSessionOverlap)
		}
		assertExactFields(t, "SessionOverlapData", c.schema(t, "SessionOverlapData"), ipc.SessionOverlapData{})
		if d := c.errorData(t, "SessionOverlap"); !reflect.DeepEqual(sortedKeys(d.Properties), sortedKeys(c.schema(t, "SessionOverlapData").Properties)) {
			t.Errorf("SessionOverlap data = %v, want SessionOverlapData", d)
		}
		if d := c.errorData(t, "SessionNotFound"); !slices.Equal(d.Required, []string{"sessionId"}) {
			t.Errorf("SessionNotFound data required = %v, want [sessionId]", d.Required)
		}
	})

	t.Run("SED-030/session.update e session.delete citam SessionActive (-32002) nos erros", func(t *testing.T) {
		t.Parallel()
		for _, name := range []string{"session.update", "session.delete"} {
			if refs := methodErrorRefs(t, methodByName(t, doc, name)); !slices.Contains(refs, "SessionActive") {
				t.Errorf("%s errors = %v, want SessionActive", name, refs)
			}
		}
		if c.Components.Errors["SessionActive"].Code != ipc.CodeSessionActive {
			t.Errorf("SessionActive code = %d, want %d", c.Components.Errors["SessionActive"].Code, ipc.CodeSessionActive)
		}
	})

	t.Run("SED-033/SessionItem tem source e editedAt obrigatórios e os tipos Go batem", func(t *testing.T) {
		t.Parallel()
		s := c.schema(t, "SessionItem")
		for _, f := range []string{"source", "editedAt"} {
			if _, ok := s.Properties[f]; !ok {
				t.Errorf("SessionItem has no property %q", f)
			}
			if !slices.Contains(s.Required, f) {
				t.Errorf("SessionItem.%s is not required", f)
			}
		}
		assertExactFields(t, "SessionItem", s, ipc.SessionItem{})
	})

	t.Run("SED-034/GameDetail tem sessionPlaySeconds obrigatório", func(t *testing.T) {
		t.Parallel()
		s := c.schema(t, "GameDetail")
		if _, ok := s.Properties["sessionPlaySeconds"]; !ok {
			t.Error("GameDetail has no property sessionPlaySeconds")
		}
		if !slices.Contains(s.Required, "sessionPlaySeconds") {
			t.Error("GameDetail.sessionPlaySeconds is not required")
		}
		assertExactFields(t, "GameDetail", s, ipc.GameDetail{})
	})

	t.Run("SED-027/StatsGame, StatsPlatform e StatsSummaryResult têm manualSeconds obrigatório", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			schema string
			v      any
		}{
			{"StatsGame", ipc.StatsGame{}},
			{"StatsPlatform", ipc.StatsPlatform{}},
			{"StatsSummaryResult", ipc.StatsSummaryResult{}},
		} {
			s := c.schema(t, tc.schema)
			if _, ok := s.Properties["manualSeconds"]; !ok {
				t.Errorf("%s has no property manualSeconds", tc.schema)
			}
			if !slices.Contains(s.Required, "manualSeconds") {
				t.Errorf("%s.manualSeconds is not required", tc.schema)
			}
			assertExactFields(t, tc.schema, s, tc.v)
		}
		assertExactFields(t, "stats.summary result", c.resultSchema(t, ipc.MethodStatsSummary), ipc.StatsSummaryResult{})
		// stats.weekly does not gain a field.
		if _, ok := c.schema(t, "StatsWeek").Properties["manualSeconds"]; ok {
			t.Error("StatsWeek must not have manualSeconds")
		}
	})

	t.Run("SED-030/a v0.17.0 é a v0.16.0 mais acréscimos", func(t *testing.T) {
		t.Parallel()
		prev := loadGeneric(t, contractV016Path)
		next := loadGeneric(t, contractPath)

		pinfo := genericSection(t, prev, "info")
		ninfo := genericSection(t, next, "info")
		if pinfo["version"] != "0.16.0" || ninfo["version"] != "0.17.0" {
			t.Errorf("info.version v0.16.0 = %v, v0.17.0 = %v", pinfo["version"], ninfo["version"])
		}
		for _, k := range []string{"version", "description"} {
			delete(pinfo, k)
			delete(ninfo, k)
		}

		// Methods: the three new ones are dropped, the rest identical and in the same order.
		nMethods := mustArray(t, next["methods"], "v0.17.0 methods")
		var kept []any
		for _, m := range nMethods {
			if !slices.Contains(sessionEditMethods, methodName(m)) {
				kept = append(kept, m)
			}
		}
		if len(nMethods)-len(kept) != len(sessionEditMethods) {
			t.Errorf("v0.17.0 has %d methods more than the kept ones, want exactly %d new", len(nMethods)-len(kept), len(sessionEditMethods))
		}
		next["methods"] = kept

		// Schemas: exactly the expected additions.
		pComps := genericSection(t, prev, "components")
		nComps := genericSection(t, next, "components")
		pSchemas := mustObject(t, pComps["schemas"], "v0.16.0 components.schemas")
		nSchemas := mustObject(t, nComps["schemas"], "v0.17.0 components.schemas")
		if got, want := takeAddedKeys(t, "components.schemas", pSchemas, nSchemas), phase16NewSchemas; !slices.Equal(got, want) {
			t.Errorf("new components.schemas = %v, want %v", got, want)
		}
		// Added members of existing schemas.
		removeSchemaProp(t, next, "SessionItem", "source")
		removeSchemaProp(t, next, "SessionItem", "editedAt")
		removeSchemaProp(t, next, "GameDetail", "sessionPlaySeconds")
		for _, s := range []string{"StatsGame", "StatsPlatform", "StatsSummaryResult"} {
			removeSchemaProp(t, next, s, "manualSeconds")
		}

		// Errors: SessionNotFound and SessionOverlap are new; InvalidParams and SessionActive only extend the description.
		pErrs := mustObject(t, pComps["errors"], "v0.16.0 components.errors")
		nErrs := mustObject(t, nComps["errors"], "v0.17.0 components.errors")
		if got, want := takeAddedKeys(t, "components.errors", pErrs, nErrs), []string{"SessionNotFound", "SessionOverlap"}; !slices.Equal(got, want) {
			t.Errorf("new components.errors = %v, want %v", got, want)
		}
		for _, name := range []string{"InvalidParams", "SessionActive"} {
			pe := mustObject(t, pErrs[name], "v0.16.0 "+name)
			ne := mustObject(t, nErrs[name], "v0.17.0 "+name)
			pd, _ := pe["description"].(string)
			nd, _ := ne["description"].(string)
			if !strings.HasPrefix(nd, pd) || nd == pd {
				t.Errorf("%s description must extend the v0.16.0 text:\n%q\n%q", name, nd, pd)
			}
			ne["description"] = pd
		}

		// Free-form notes the delta carries in components are tolerated.
		for k := range nComps {
			if _, ok := pComps[k]; !ok && strings.HasPrefix(k, "x-note") {
				delete(nComps, k)
			}
		}

		if !reflect.DeepEqual(prev, next) {
			t.Error("v0.17.0 differs from v0.16.0 beyond info.version and description, the three session.* methods, the new schemas, the added SessionItem/GameDetail/stats members and the InvalidParams and SessionActive descriptions")
		}
	})
}

// TestContractFilesStillExist keeps the earlier contract files readable for the "only additions" tests.
func TestContractFilesStillExist(t *testing.T) {
	t.Parallel()
	t.Run("SED-030/os contratos das fases 15 e 16 existem", func(t *testing.T) {
		t.Parallel()
		for _, p := range []string{contractV016Path, contractPath} {
			if _, err := os.Stat(p); err != nil {
				t.Errorf("contract file: %v", err)
			}
		}
	})
}
