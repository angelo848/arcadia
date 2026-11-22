package ipc_test

// Tests for T08 of docs/specs/phase-15-big-box-v2/tasks.md: contract IPC v0.16.0 [TTB-029, TTB-008,
// TTB-020..TTB-023]. They stay red until T09 (Go types) and T10 (the full contract file) exist.

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

// timeToBeatRequests are the six timeToBeat.* requests of v0.16.0.
var timeToBeatRequests = []string{
	"timeToBeat.get", "timeToBeat.refresh", "timeToBeat.search", "timeToBeat.choose",
	"timeToBeat.getSettings", "timeToBeat.setSettings",
}

// phase14Methods are the methods of phase 14 that the v0.16.0 file also adds (tasks.md, Dúvidas 1).
var phase14Methods = []string{"media.add", "media.remove", "game.setPlayTime"}

// phase15NewSchemas are the components.schemas added in v0.16.0 (delta §3 plus the phase 14 results).
var phase15NewSchemas = []string{
	"MediaAddResult", "MediaRemoveResult", "TimeToBeat", "TimeToBeatCandidate",
	"TimeToBeatSearchResult", "TimeToBeatSeconds", "TimeToBeatSettings",
}

func TestTimeToBeatContract(t *testing.T) {
	t.Parallel()
	c := loadContract(t)
	doc := loadGeneric(t, contractPath)

	t.Run("TTB-029/versão 0.16.0", func(t *testing.T) {
		t.Parallel()
		if c.Info.Version != "0.16.0" {
			t.Fatalf("contract %s info.version = %q, want \"0.16.0\"", contractPath, c.Info.Version)
		}
	})

	t.Run("TTB-029/os seis métodos timeToBeat.* existem com paramStructure by-name e os nomes das constantes Go", func(t *testing.T) {
		t.Parallel()
		for got, want := range map[string]string{
			ipc.MethodTimeToBeatGet:         "timeToBeat.get",
			ipc.MethodTimeToBeatRefresh:     "timeToBeat.refresh",
			ipc.MethodTimeToBeatSearch:      "timeToBeat.search",
			ipc.MethodTimeToBeatChoose:      "timeToBeat.choose",
			ipc.MethodTimeToBeatGetSettings: "timeToBeat.getSettings",
			ipc.MethodTimeToBeatSetSettings: "timeToBeat.setSettings",
			ipc.MethodTimeToBeatChanged:     "timeToBeat.changed",
		} {
			if got != want {
				t.Errorf("Go constant = %q, want %q", got, want)
			}
		}
		for _, name := range timeToBeatRequests {
			m := methodByName(t, doc, name)
			if m["paramStructure"] != "by-name" {
				t.Errorf("%s paramStructure = %v, want by-name", name, m["paramStructure"])
			}
			if m["x-notification"] == true {
				t.Errorf("%s is a notification, want a request", name)
			}
		}
		for _, tc := range []struct {
			method       string
			wantAll      []string
			wantRequired []string
			resultRef    string
			errs         []string
		}{
			{"timeToBeat.get", []string{"fetch", "gameId"}, []string{"gameId"}, "TimeToBeat", []string{"InvalidParams", "GameNotFound", "InternalError"}},
			{"timeToBeat.refresh", []string{"gameId"}, []string{"gameId"}, "TimeToBeat", []string{"InvalidParams", "GameNotFound", "ProviderUnavailable", "InternalError"}},
			{"timeToBeat.search", []string{"gameId", "term"}, []string{"gameId"}, "TimeToBeatSearchResult", []string{"InvalidParams", "GameNotFound", "ProviderUnavailable", "InternalError"}},
			{"timeToBeat.choose", []string{"externalId", "gameId"}, []string{"externalId", "gameId"}, "TimeToBeat", []string{"InvalidParams", "GameNotFound", "InternalError"}},
			{"timeToBeat.getSettings", nil, nil, "TimeToBeatSettings", []string{"InternalError"}},
			{"timeToBeat.setSettings", []string{"afterMetadata", "enabled"}, nil, "TimeToBeatSettings", []string{"InvalidParams", "InternalError"}},
		} {
			all, required := c.paramNames(t, tc.method)
			if !slices.Equal(all, tc.wantAll) || !slices.Equal(required, tc.wantRequired) {
				t.Errorf("%s params = %v (required %v), want %v (required %v)", tc.method, all, required, tc.wantAll, tc.wantRequired)
			}
			m := methodByName(t, doc, tc.method)
			res := mustObject(t, mustObject(t, m["result"], "result")["schema"], "result schema")
			if res["$ref"] != "#/components/schemas/"+tc.resultRef {
				t.Errorf("%s result = %v, want $ref %s", tc.method, res, tc.resultRef)
			}
			if got := methodErrorRefs(t, m); !slices.Equal(got, tc.errs) {
				t.Errorf("%s errors = %v, want %v", tc.method, got, tc.errs)
			}
		}
		// The Go params match the wire names; omitempty members stay out when unset.
		term := "x"
		yes := true
		for _, tc := range []struct {
			what string
			v    any
			want []string
		}{
			{"get{fetch}", ipc.TimeToBeatGetParams{GameID: 1, Fetch: true}, []string{"fetch", "gameId"}},
			{"get{}", ipc.TimeToBeatGetParams{GameID: 1}, []string{"gameId"}},
			{"game", ipc.TimeToBeatGameParams{GameID: 1}, []string{"gameId"}},
			{"search{term}", ipc.TimeToBeatSearchParams{GameID: 1, Term: &term}, []string{"gameId", "term"}},
			{"search{}", ipc.TimeToBeatSearchParams{GameID: 1}, []string{"gameId"}},
			{"choose empty", ipc.TimeToBeatChooseParams{GameID: 1}, []string{"externalId", "gameId"}},
			{"setSettings both", ipc.TimeToBeatSetSettingsParams{Enabled: &yes, AfterMetadata: &yes}, []string{"afterMetadata", "enabled"}},
			{"setSettings none", ipc.TimeToBeatSetSettingsParams{}, nil},
			{"changed", ipc.TimeToBeatChangedParams{GameID: 7}, []string{"gameId"}},
		} {
			if got := jsonFieldNames(t, tc.v); !slices.Equal(got, tc.want) {
				t.Errorf("%s JSON fields = %v, want %v", tc.what, got, tc.want)
			}
		}
		assertExactFields(t, "timeToBeat.search result", c.resultSchema(t, "timeToBeat.search"), ipc.TimeToBeatSearchResult{})
	})

	t.Run("TTB-029/timeToBeat.changed é notificação com o único parâmetro gameId inteiro >= 1", func(t *testing.T) {
		t.Parallel()
		m := methodByName(t, doc, "timeToBeat.changed")
		if m["x-notification"] != true || m["paramStructure"] != "by-name" {
			t.Errorf("timeToBeat.changed = %v, want x-notification true and by-name", m)
		}
		all, required := c.paramNames(t, ipc.MethodTimeToBeatChanged)
		if !slices.Equal(all, []string{"gameId"}) || !slices.Equal(required, []string{"gameId"}) {
			t.Errorf("params = %v (required %v), want [gameId] all required", all, required)
		}
		if s := paramSchema(t, m, "gameId"); s["type"] != "integer" || s["minimum"] != 1.0 {
			t.Errorf("gameId schema = %v, want integer minimum 1", s)
		}
		raw, err := json.Marshal(ipc.TimeToBeatChangedParams{GameID: 42})
		if err != nil || string(raw) != `{"gameId":42}` {
			t.Errorf("params JSON = %s (err %v), want {\"gameId\":42}", raw, err)
		}
	})

	t.Run("TTB-029/campos dos tipos Go TimeToBeat, TimeToBeatCandidate e TimeToBeatSettings batem com os esquemas", func(t *testing.T) {
		t.Parallel()
		for _, name := range phase15NewSchemas {
			c.schema(t, name)
		}
		assertExactFields(t, "TimeToBeat", c.schema(t, "TimeToBeat"), ipc.TimeToBeat{})
		assertExactFields(t, "TimeToBeatCandidate", c.schema(t, "TimeToBeatCandidate"), ipc.TimeToBeatCandidate{})
		assertExactFields(t, "TimeToBeatSettings", c.schema(t, "TimeToBeatSettings"), ipc.TimeToBeatSettings{})
		// The candidate list is never null on the wire.
		raw, err := json.Marshal(ipc.TimeToBeatSearchResult{Candidates: []ipc.TimeToBeatCandidate{}})
		if err != nil || string(raw) != `{"candidates":[]}` {
			t.Errorf("TimeToBeatSearchResult JSON = %s (err %v), want {\"candidates\":[]}", raw, err)
		}
	})

	t.Run("TTB-029/enum source tem hltb e igdb (e vazio com none) e state tem none, found, doubt e not-found", func(t *testing.T) {
		t.Parallel()
		var source, state struct {
			Enum []string `json:"enum"`
		}
		props := c.schema(t, "TimeToBeat").Properties
		if err := json.Unmarshal(props["source"], &source); err != nil {
			t.Fatalf("parse TimeToBeat.source: %v", err)
		}
		if err := json.Unmarshal(props["state"], &state); err != nil {
			t.Fatalf("parse TimeToBeat.state: %v", err)
		}
		if want := []string{"hltb", "igdb", ""}; !slices.Equal(slices.Sorted(slices.Values(source.Enum)), slices.Sorted(slices.Values(want))) {
			t.Errorf("TimeToBeat.source enum = %v, want %v", source.Enum, want)
		}
		if want := []string{"none", "found", "doubt", "not-found"}; !slices.Equal(slices.Sorted(slices.Values(state.Enum)), slices.Sorted(slices.Values(want))) {
			t.Errorf("TimeToBeat.state enum = %v, want %v", state.Enum, want)
		}
	})

	t.Run("TTB-029/x-timetobeat repete as constantes do delta §5", func(t *testing.T) {
		t.Parallel()
		xt := mustObject(t, genericSection(t, doc, "info")["x-timetobeat"], "info.x-timetobeat")
		delete(xt, "description")
		want := map[string]any{
			"source":         "hltb",
			"fallbackSource": "igdb",
			"fallbackMapping": map[string]any{
				"mainSeconds": "hastily", "mainExtraSeconds": "normally", "completionistSeconds": "completely",
			},
			"requestIntervalMs":    2000.0,
			"requestTimeoutMs":     10000.0,
			"syncTimeoutMs":        25000.0,
			"foundTtlDays":         90.0,
			"fallbackFoundTtlDays": 30.0,
			"doubtTtlDays":         30.0,
			"notFoundTtlDays":      30.0,
			"backoffMinutes":       []any{1.0, 5.0, 30.0, 60.0},
			"batchQueueMax":        1000.0,
			"maxCandidates":        10.0,
			"termMaxLength":        200.0,
			"settingKeys":          map[string]any{"enabled": "timeToBeat.enabled", "afterMetadata": "timeToBeat.afterMetadata"},
		}
		if !reflect.DeepEqual(xt, want) {
			t.Errorf("info.x-timetobeat =\n%v\nwant\n%v", xt, want)
		}
		// The bounds repeated in the schemas agree with the block.
		if s := paramSchema(t, methodByName(t, doc, "timeToBeat.search"), "term"); s["maxLength"] != want["termMaxLength"] {
			t.Errorf("term maxLength = %v, x-timetobeat.termMaxLength = %v", s["maxLength"], want["termMaxLength"])
		}
		if s := mustObject(t, genericSchemaProps(t, doc, "TimeToBeat")["candidates"], "candidates"); s["maxItems"] != want["maxCandidates"] {
			t.Errorf("candidates maxItems = %v, x-timetobeat.maxCandidates = %v", s["maxItems"], want["maxCandidates"])
		}
	})

	t.Run("TTB-020/-32011 do refresh e do search traz data.provider hltb e reason unavailable ou disabled; -32001 e -32602 documentados", func(t *testing.T) {
		t.Parallel()
		if ipc.CodeProviderUnavailable != -32011 {
			t.Errorf("ipc.CodeProviderUnavailable = %d, want -32011", ipc.CodeProviderUnavailable)
		}
		if e := c.Components.Errors["ProviderUnavailable"]; e.Code != ipc.CodeProviderUnavailable {
			t.Errorf("ProviderUnavailable code = %d, want %d", e.Code, ipc.CodeProviderUnavailable)
		}
		data := mustObject(t, mustObject(t, genericSection(t, doc, "components")["errors"], "errors")["ProviderUnavailable"], "ProviderUnavailable")
		d := mustObject(t, data["data"], "ProviderUnavailable.data")
		if req := mustArray(t, d["required"], "data.required"); !reflect.DeepEqual(req, []any{"provider"}) {
			t.Errorf("ProviderUnavailable data.required = %v, want [provider] (reason is optional)", req)
		}
		props := mustObject(t, d["properties"], "data.properties")
		prov := mustObject(t, props["provider"], "data.provider")
		enum := mustArray(t, prov["enum"], "data.provider enum")
		for _, v := range []any{"igdb", "steamgriddb", "steamstore", "hltb"} {
			if !slices.Contains(enum, v) {
				t.Errorf("ProviderUnavailable data.provider enum = %v, want it to contain %v", enum, v)
			}
		}
		reason := mustObject(t, props["reason"], "data.reason")
		if got := mustArray(t, reason["enum"], "data.reason enum"); !reflect.DeepEqual(got, []any{"unavailable", "disabled"}) {
			t.Errorf("ProviderUnavailable data.reason enum = %v, want [unavailable disabled]", got)
		}
		for _, name := range []string{"timeToBeat.refresh", "timeToBeat.search"} {
			if refs := methodErrorRefs(t, methodByName(t, doc, name)); !slices.Contains(refs, "ProviderUnavailable") {
				t.Errorf("%s errors = %v, want ProviderUnavailable (-32011)", name, refs)
			}
		}
		if c.Components.Errors["GameNotFound"].Code != -32001 || c.Components.Errors["InvalidParams"].Code != -32602 {
			t.Error("GameNotFound must be -32001 and InvalidParams -32602")
		}
		for _, name := range []string{"timeToBeat.get", "timeToBeat.refresh", "timeToBeat.search", "timeToBeat.choose"} {
			refs := methodErrorRefs(t, methodByName(t, doc, name))
			if !slices.Contains(refs, "InvalidParams") || !slices.Contains(refs, "GameNotFound") {
				t.Errorf("%s errors = %v, want InvalidParams and GameNotFound", name, refs)
			}
		}
	})

	t.Run("TTB-021/timeToBeat.search: term opcional de 1 a 200 caracteres", func(t *testing.T) {
		t.Parallel()
		s := paramSchema(t, methodByName(t, doc, "timeToBeat.search"), "term")
		if s["type"] != "string" || s["minLength"] != 1.0 || s["maxLength"] != 200.0 {
			t.Errorf("term schema = %v, want string minLength 1 maxLength 200", s)
		}
		if _, required := c.paramNames(t, "timeToBeat.search"); slices.Contains(required, "term") {
			t.Error("term must be optional")
		}
	})

	t.Run("TTB-022/timeToBeat.choose: externalId aceita string vazia", func(t *testing.T) {
		t.Parallel()
		s := paramSchema(t, methodByName(t, doc, "timeToBeat.choose"), "externalId")
		if s["type"] != "string" {
			t.Errorf("externalId schema = %v, want string", s)
		}
		if ml, ok := s["minLength"]; ok && ml != 0.0 {
			t.Errorf("externalId minLength = %v, want none (empty string = \"nenhum destes\")", ml)
		}
		if _, required := c.paramNames(t, "timeToBeat.choose"); !slices.Contains(required, "externalId") {
			t.Error("externalId must be required")
		}
	})

	t.Run("TTB-023/timeToBeat.setSettings: enabled e afterMetadata opcionais, pelo menos um", func(t *testing.T) {
		t.Parallel()
		m := methodByName(t, doc, "timeToBeat.setSettings")
		for _, p := range []string{"enabled", "afterMetadata"} {
			if s := paramSchema(t, m, p); s["type"] != "boolean" {
				t.Errorf("%s schema = %v, want boolean", p, s)
			}
		}
		if _, required := c.paramNames(t, "timeToBeat.setSettings"); len(required) != 0 {
			t.Errorf("required = %v, want none (at least one is a runtime rule)", required)
		}
		if s, _ := m["summary"].(string); !containsFold(s, "at least one") {
			t.Errorf("summary = %q, want it to say \"at least one\"", s)
		}
		if refs := methodErrorRefs(t, m); !slices.Contains(refs, "InvalidParams") {
			t.Errorf("errors = %v, want InvalidParams for the empty call", refs)
		}
	})

	t.Run("TTB-008/timeToBeat.get: fetch tem padrão false e nunca devolve -32011", func(t *testing.T) {
		t.Parallel()
		m := methodByName(t, doc, "timeToBeat.get")
		s := paramSchema(t, m, "fetch")
		if s["type"] != "boolean" || s["default"] != false {
			t.Errorf("fetch schema = %v, want boolean default false", s)
		}
		if refs := methodErrorRefs(t, m); slices.Contains(refs, "ProviderUnavailable") {
			t.Errorf("timeToBeat.get errors = %v, want no ProviderUnavailable", refs)
		}
	})

	t.Run("TTB-029/métodos da fase 14 no arquivo: media.add, media.remove e game.setPlayTime", func(t *testing.T) {
		t.Parallel()
		for got, want := range map[string]string{
			ipc.MethodMediaAdd: "media.add", ipc.MethodMediaRemove: "media.remove", ipc.MethodGameSetPlayTime: "game.setPlayTime",
		} {
			if got != want {
				t.Errorf("Go constant = %q, want %q", got, want)
			}
		}
		for _, tc := range []struct {
			method       string
			wantAll      []string
			wantRequired []string
			errs         []string // must be contained
		}{
			{"media.add", []string{"gameId", "kind", "path"}, []string{"gameId", "kind", "path"}, []string{"InvalidParams", "GameNotFound", "InternalError"}},
			{"media.remove", []string{"mediaId"}, []string{"mediaId"}, []string{"InvalidParams", "InternalError"}},
			{"game.setPlayTime", []string{"gameId", "totalPlaySeconds"}, []string{"gameId", "totalPlaySeconds"}, []string{"InvalidParams", "GameNotFound", "InternalError"}},
		} {
			m := methodByName(t, doc, tc.method)
			if m["paramStructure"] != "by-name" || m["x-notification"] == true {
				t.Errorf("%s = by-name request expected, got %v", tc.method, m)
			}
			all, required := c.paramNames(t, tc.method)
			if !slices.Equal(all, tc.wantAll) || !slices.Equal(required, tc.wantRequired) {
				t.Errorf("%s params = %v (required %v), want %v (required %v)", tc.method, all, required, tc.wantAll, tc.wantRequired)
			}
			refs := methodErrorRefs(t, m)
			for _, e := range tc.errs {
				if !slices.Contains(refs, e) {
					t.Errorf("%s errors = %v, want %s", tc.method, refs, e)
				}
			}
		}
		if got := c.paramEnum(t, "media.add", "kind"); !slices.Equal(got, []string{"cover", "screenshot", "background"}) {
			t.Errorf("media.add kind enum = %v, want [cover screenshot background]", got)
		}
		if s := paramSchema(t, methodByName(t, doc, "media.add"), "path"); s["type"] != "string" {
			t.Errorf("media.add path schema = %v, want string", s)
		}
		if s := paramSchema(t, methodByName(t, doc, "media.remove"), "mediaId"); s["type"] != "integer" || s["minimum"] != 1.0 {
			t.Errorf("media.remove mediaId schema = %v, want integer minimum 1", s)
		}
		s := paramSchema(t, methodByName(t, doc, "game.setPlayTime"), "totalPlaySeconds")
		if s["type"] != "integer" || s["minimum"] != 0.0 || s["maximum"] != float64(ipc.MaxPlaySeconds) {
			t.Errorf("totalPlaySeconds schema = %v, want integer 0..%d", s, ipc.MaxPlaySeconds)
		}
		if got := jsonFieldNames(t, ipc.MediaAddParams{}); !slices.Equal(got, []string{"gameId", "kind", "path"}) {
			t.Errorf("MediaAddParams JSON fields = %v", got)
		}
		if got := jsonFieldNames(t, ipc.MediaRemoveParams{}); !slices.Equal(got, []string{"mediaId"}) {
			t.Errorf("MediaRemoveParams JSON fields = %v", got)
		}
		if got := jsonFieldNames(t, ipc.GameSetPlayTimeParams{}); !slices.Equal(got, []string{"gameId", "totalPlaySeconds"}) {
			t.Errorf("GameSetPlayTimeParams JSON fields = %v", got)
		}
		assertExactFields(t, "media.add result", c.resultSchema(t, "media.add"), ipc.MediaAddResult{})
		assertExactFields(t, "media.remove result", c.resultSchema(t, "media.remove"), ipc.MediaRemoveResult{})
		assertExactFields(t, "game.setPlayTime result", c.resultSchema(t, "game.setPlayTime"), ipc.GameDetailResult{})
		res := mustObject(t, mustObject(t, methodByName(t, doc, "game.setPlayTime")["result"], "result")["schema"], "result schema")
		if res["$ref"] != "#/components/schemas/GameDetailResult" {
			t.Errorf("game.setPlayTime result = %v, want $ref GameDetailResult", res)
		}
	})

	t.Run("TTB-029/a v0.16.0 é a v0.15.0 mais acréscimos", func(t *testing.T) {
		t.Parallel()
		prev := loadGeneric(t, contractV015Path)
		next := loadGeneric(t, contractPath)

		pinfo := genericSection(t, prev, "info")
		ninfo := genericSection(t, next, "info")
		if pinfo["version"] != "0.15.0" || ninfo["version"] != "0.16.0" {
			t.Errorf("info.version v0.15.0 = %v, v0.16.0 = %v", pinfo["version"], ninfo["version"])
		}
		if _, ok := ninfo["x-timetobeat"]; !ok {
			t.Error("v0.16.0 info has no x-timetobeat")
		}
		delete(ninfo, "x-timetobeat")
		for _, k := range []string{"version", "description"} {
			delete(pinfo, k)
			delete(ninfo, k)
		}

		// Methods: the new ones are dropped, the rest must be identical and in the same order.
		newMethods := append(slices.Clone(timeToBeatRequests), "timeToBeat.changed")
		newMethods = append(newMethods, phase14Methods...)
		nMethods := mustArray(t, next["methods"], "v0.16.0 methods")
		for _, name := range newMethods {
			methodIndex(t, nMethods, name)
		}
		var kept []any
		for _, m := range nMethods {
			if !slices.Contains(newMethods, methodName(m)) {
				kept = append(kept, m)
			}
		}
		if len(nMethods)-len(kept) != len(newMethods) {
			t.Errorf("v0.16.0 has %d methods more than the kept ones, want exactly %d new", len(nMethods)-len(kept), len(newMethods))
		}
		next["methods"] = kept

		// Schemas: exactly the expected additions.
		pComps := genericSection(t, prev, "components")
		nComps := genericSection(t, next, "components")
		pSchemas := mustObject(t, pComps["schemas"], "v0.15.0 components.schemas")
		nSchemas := mustObject(t, nComps["schemas"], "v0.16.0 components.schemas")
		if got, want := takeAddedKeys(t, "components.schemas", pSchemas, nSchemas), slices.Sorted(slices.Values(phase15NewSchemas)); !slices.Equal(got, want) {
			t.Errorf("new components.schemas = %v, want %v", got, want)
		}

		// Errors: no new component; ProviderUnavailable only gains hltb and the optional reason.
		pErrs := mustObject(t, pComps["errors"], "v0.15.0 components.errors")
		nErrs := mustObject(t, nComps["errors"], "v0.16.0 components.errors")
		if got := takeAddedKeys(t, "components.errors", pErrs, nErrs); len(got) != 0 {
			t.Errorf("new components.errors = %v, want none (the existing ProviderUnavailable is reused)", got)
		}
		pu := mustObject(t, nErrs["ProviderUnavailable"], "v0.16.0 ProviderUnavailable")
		ppu := mustObject(t, pErrs["ProviderUnavailable"], "v0.15.0 ProviderUnavailable")
		nd, pd := mustObject(t, pu["data"], "data"), mustObject(t, ppu["data"], "prev data")
		nProps, pProps := mustObject(t, nd["properties"], "props"), mustObject(t, pd["properties"], "prev props")
		nProvider, pProvider := mustObject(t, nProps["provider"], "provider"), mustObject(t, pProps["provider"], "prev provider")
		if !reflect.DeepEqual(nProvider["enum"], append(slices.Clone(mustArray(t, pProvider["enum"], "prev enum")), "hltb")) {
			t.Errorf("v0.16.0 ProviderUnavailable provider enum = %v, want the v0.15.0 values plus hltb at the end", nProvider["enum"])
		}
		nProvider["enum"] = pProvider["enum"]
		if _, ok := nProps["reason"]; !ok {
			t.Error("v0.16.0 ProviderUnavailable data has no reason")
		}
		delete(nProps, "reason")
		if !reflect.DeepEqual(pu, ppu) {
			t.Errorf("ProviderUnavailable differs beyond the hltb value and the reason member:\n%v\n%v", pu, ppu)
		}

		// Free-form notes the delta carries in components are tolerated.
		for k := range nComps {
			if _, ok := pComps[k]; !ok && len(k) > 6 && k[:6] == "x-note" {
				delete(nComps, k)
			}
		}

		if !reflect.DeepEqual(prev, next) {
			t.Error("v0.16.0 differs from v0.15.0 beyond info.version and description, x-timetobeat, the six timeToBeat.* methods, timeToBeat.changed, media.add, media.remove, game.setPlayTime, the new schemas and the ProviderUnavailable additions")
		}
	})
}

func containsFold(s, sub string) bool {
	ls, lsub := []rune(s), []rune(sub)
	for i := 0; i+len(lsub) <= len(ls); i++ {
		if equalFoldRunes(ls[i:i+len(lsub)], lsub) {
			return true
		}
	}
	return false
}

func equalFoldRunes(a, b []rune) bool {
	for i := range a {
		x, y := a[i], b[i]
		if x >= 'A' && x <= 'Z' {
			x += 'a' - 'A'
		}
		if y >= 'A' && y <= 'Z' {
			y += 'a' - 'A'
		}
		if x != y {
			return false
		}
	}
	return true
}
