package ipc_test

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

// videoMethods are the four video.* methods added in v0.15.0, in contract order [VID-005..VID-008].
var videoMethods = []string{"video.queue", "video.queueMissing", "video.cancelPending", "video.status"}

// consoleNewSchemas are the components.schemas added in v0.15.0 [CON-026].
var consoleNewSchemas = []string{"VideoSkip", "VideoQueueResult", "VideoQueueMissingResult", "VideoLane", "VideoStatus"}

const launchRefusedSentence = "Also refused with SessionActive while a close by the gamepad combo Select+B is in progress"

// methodByName returns the generic method object.
func methodByName(t *testing.T, doc map[string]any, name string) map[string]any {
	t.Helper()
	return findMethod(t, mustArray(t, doc["methods"], "methods"), name)
}

// paramSchema returns the schema object of the named param of a generic method.
func paramSchema(t *testing.T, m map[string]any, name string) map[string]any {
	t.Helper()
	for _, p := range mustArray(t, m["params"], "params") {
		pm := mustObject(t, p, "param")
		if pm["name"] == name {
			return mustObject(t, pm["schema"], "param "+name+" schema")
		}
	}
	t.Fatalf("method %v has no param %q", m["name"], name)
	return nil
}

// methodErrorRefs returns the component error names listed in a generic method.
func methodErrorRefs(t *testing.T, m map[string]any) []string {
	t.Helper()
	var out []string
	for _, e := range mustArray(t, m["errors"], "errors") {
		ref, _ := mustObject(t, e, "error")["$ref"].(string)
		out = append(out, strings.TrimPrefix(ref, "#/components/errors/"))
	}
	return out
}

func TestConsoleVideoContract(t *testing.T) {
	t.Parallel()
	c := loadContract(t)
	doc := loadGeneric(t, contractPath)

	t.Run("CON-026/versão 0.15.0", func(t *testing.T) {
		t.Parallel()
		if c.Info.Version != "0.15.0" {
			t.Fatalf("contract %s info.version = %q, want \"0.15.0\"", contractPath, c.Info.Version)
		}
	})

	t.Run("VID-005/os quatro métodos video.* existem por nome, com os nomes das constantes Go e params do delta §4", func(t *testing.T) {
		t.Parallel()
		for got, want := range map[string]string{
			ipc.MethodVideoQueue:         "video.queue",
			ipc.MethodVideoQueueMissing:  "video.queueMissing",
			ipc.MethodVideoCancelPending: "video.cancelPending",
			ipc.MethodVideoStatus:        "video.status",
		} {
			if got != want {
				t.Errorf("Go constant = %q, want %q", got, want)
			}
		}
		for _, name := range videoMethods {
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
		}{
			{"video.queue", []string{"gameIds"}, []string{"gameIds"}},
			{"video.queueMissing", []string{"platformId"}, nil},
			{"video.cancelPending", nil, nil},
			{"video.status", nil, nil},
		} {
			all, required := c.paramNames(t, tc.method)
			if !slices.Equal(all, tc.wantAll) || !slices.Equal(required, tc.wantRequired) {
				t.Errorf("%s params = %v (required %v), want %v (required %v)", tc.method, all, required, tc.wantAll, tc.wantRequired)
			}
		}
		if got := jsonFieldNames(t, ipc.VideoQueueParams{GameIDs: []int64{1}}); !slices.Equal(got, []string{"gameIds"}) {
			t.Errorf("VideoQueueParams JSON fields = %v, want [gameIds]", got)
		}
		if got := jsonFieldNames(t, ipc.VideoQueueMissingParams{PlatformID: 3}); !slices.Equal(got, []string{"platformId"}) {
			t.Errorf("VideoQueueMissingParams JSON fields = %v, want [platformId]", got)
		}
		if got := jsonFieldNames(t, ipc.VideoQueueMissingParams{}); len(got) != 0 {
			t.Errorf("VideoQueueMissingParams{} JSON fields = %v, want none (platformId omitted)", got)
		}
	})

	t.Run("VID-005/video.queue valida gameIds: 1..1000, inteiros >= 1, distintos", func(t *testing.T) {
		t.Parallel()
		s := paramSchema(t, methodByName(t, doc, "video.queue"), "gameIds")
		if s["type"] != "array" || s["minItems"] != 1.0 || s["maxItems"] != 1000.0 || s["uniqueItems"] != true {
			t.Errorf("gameIds schema = %v, want array minItems 1 maxItems 1000 uniqueItems true", s)
		}
		items := mustObject(t, s["items"], "gameIds items")
		if items["type"] != "integer" || items["minimum"] != 1.0 {
			t.Errorf("gameIds items = %v, want integer minimum 1", items)
		}
		p := paramSchema(t, methodByName(t, doc, "video.queueMissing"), "platformId")
		if p["type"] != "integer" || p["minimum"] != 1.0 {
			t.Errorf("platformId schema = %v, want integer minimum 1", p)
		}
	})

	t.Run("VID-005/resultados e erros de cada método video.*", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			method string
			ref    string // result $ref, empty for an inline schema
			errs   []string
		}{
			{"video.queue", "VideoQueueResult", []string{"InvalidParams", "GameNotFound", "ProviderNoKey", "InternalError"}},
			{"video.queueMissing", "VideoQueueMissingResult", []string{"InvalidParams", "PlatformNotFound", "ProviderNoKey", "InternalError"}},
			{"video.cancelPending", "", []string{"InternalError"}},
			{"video.status", "", []string{"InternalError"}},
		} {
			m := methodByName(t, doc, tc.method)
			res := mustObject(t, mustObject(t, m["result"], "result")["schema"], "result schema")
			if tc.ref != "" {
				if res["$ref"] != "#/components/schemas/"+tc.ref {
					t.Errorf("%s result = %v, want $ref %s", tc.method, res, tc.ref)
				}
			} else if res["type"] != "object" {
				t.Errorf("%s result = %v, want an inline object", tc.method, res)
			}
			if got := methodErrorRefs(t, m); !slices.Equal(got, tc.errs) {
				t.Errorf("%s errors = %v, want %v", tc.method, got, tc.errs)
			}
			for _, e := range tc.errs {
				if _, ok := c.Components.Errors[e]; !ok {
					t.Errorf("%s lists error %s that is not in components.errors", tc.method, e)
				}
			}
		}
		// Inline results: cancelPending {cancelled}, status {status: VideoStatus}.
		cancel := c.resultSchema(t, "video.cancelPending")
		assertExactFields(t, "video.cancelPending result", cancel, ipc.VideoCancelResult{})
		status := c.resultSchema(t, "video.status")
		assertExactFields(t, "video.status result", status, ipc.VideoStatusResult{})
		var st jsonSchema
		if err := json.Unmarshal(status.Properties["status"], &st); err != nil || st.Ref != "#/components/schemas/VideoStatus" {
			t.Errorf("video.status result.status = %s, want $ref VideoStatus", status.Properties["status"])
		}
		// ProviderNoKey keeps data.provider (the delta says screenscraperdev or screenscraper).
		if !slices.Contains(sortedKeys(c.errorData(t, "ProviderNoKey").Properties), "provider") {
			t.Error("ProviderNoKey has no data.provider")
		}
	})

	t.Run("VID-005/campos dos esquemas novos batem com os tipos Go", func(t *testing.T) {
		t.Parallel()
		for _, name := range consoleNewSchemas {
			c.schema(t, name)
		}
		assertExactFields(t, "VideoSkip", c.schema(t, "VideoSkip"), ipc.VideoSkip{})
		assertExactFields(t, "VideoQueueResult", c.schema(t, "VideoQueueResult"), ipc.VideoQueueResult{})
		assertExactFields(t, "VideoQueueMissingResult", c.schema(t, "VideoQueueMissingResult"), ipc.VideoQueueMissingResult{})
		pausedUntil := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
		n := 1
		assertFieldsWithOptional(t, "VideoStatus", c.schema(t, "VideoStatus"),
			[]string{"pausedUntil", "requestsToday", "maxRequestsPerDay"},
			ipc.VideoStatus{Lane: ipc.VideoLanePausedQuota, PausedUntil: &pausedUntil, RequestsToday: &n, MaxRequestsPerDay: &n})
		// Optional members are omitted while nil.
		if got := jsonFieldNames(t, ipc.VideoStatus{Lane: ipc.VideoLaneActive}); !slices.Equal(got, []string{"failed", "lane", "pending", "running"}) {
			t.Errorf("VideoStatus{} JSON fields = %v, want [failed lane pending running]", got)
		}
		// The skipped list is never null on the wire.
		raw, err := json.Marshal(ipc.VideoQueueResult{Skipped: []ipc.VideoSkip{}})
		if err != nil || string(raw) != `{"queued":0,"skipped":[]}` {
			t.Errorf("VideoQueueResult JSON = %s (err %v), want {\"queued\":0,\"skipped\":[]}", raw, err)
		}
	})

	t.Run("VID-005/enums VideoSkip.reason e VideoLane batem com as constantes Go", func(t *testing.T) {
		t.Parallel()
		var reason struct {
			Enum []string `json:"enum"`
		}
		if err := json.Unmarshal(c.schema(t, "VideoSkip").Properties["reason"], &reason); err != nil {
			t.Fatalf("parse VideoSkip.reason: %v", err)
		}
		if want := []string{ipc.VideoSkipHasVideo, ipc.VideoSkipPlatformUnmapped}; !slices.Equal(reason.Enum, want) {
			t.Errorf("VideoSkip.reason enum = %v, want %v", reason.Enum, want)
		}
		wantLane := []string{ipc.VideoLaneActive, ipc.VideoLanePausedQuota, ipc.VideoLanePausedCredentials, ipc.VideoLaneNoCredentials}
		if got := c.schema(t, "VideoLane").Enum; !slices.Equal(got, wantLane) {
			t.Errorf("VideoLane enum = %v, want %v", got, wantLane)
		}
		if want := []string{"active", "paused-quota", "paused-credentials", "no-credentials"}; !slices.Equal(wantLane, want) {
			t.Errorf("Go lane constants = %v, want %v", wantLane, want)
		}
	})

	t.Run("CON-019/game.closedByHotkey é notificação com gameId (inteiro >= 1) e o nome da constante Go", func(t *testing.T) {
		t.Parallel()
		if ipc.MethodGameClosedByHotkey != "game.closedByHotkey" {
			t.Fatalf("ipc.MethodGameClosedByHotkey = %q", ipc.MethodGameClosedByHotkey)
		}
		m := methodByName(t, doc, "game.closedByHotkey")
		if m["x-notification"] != true || m["paramStructure"] != "by-name" {
			t.Errorf("game.closedByHotkey = %v, want x-notification true and by-name", m)
		}
		all, required := c.paramNames(t, ipc.MethodGameClosedByHotkey)
		if !slices.Equal(all, []string{"gameId"}) || !slices.Equal(required, []string{"gameId"}) {
			t.Errorf("params = %v (required %v), want [gameId] all required", all, required)
		}
		s := paramSchema(t, m, "gameId")
		if s["type"] != "integer" || s["minimum"] != 1.0 {
			t.Errorf("gameId schema = %v, want integer minimum 1", s)
		}
		if got := jsonFieldNames(t, ipc.GameClosedByHotkeyParams{GameID: 42}); !slices.Equal(got, []string{"gameId"}) {
			t.Errorf("GameClosedByHotkeyParams JSON fields = %v, want [gameId]", got)
		}
		raw, err := json.Marshal(ipc.GameClosedByHotkeyParams{GameID: 42})
		if err != nil || string(raw) != `{"gameId":42}` {
			t.Errorf("params JSON = %s (err %v), want the delta §5 example {\"gameId\":42}", raw, err)
		}
	})

	t.Run("SSC-001/provedores screenscraper e screenscraperdev nos enums de provider.* e ProviderKeyStatus", func(t *testing.T) {
		t.Parallel()
		if ipc.ProviderScreenScraper != "screenscraper" || ipc.ProviderScreenScraperDev != "screenscraperdev" {
			t.Fatalf("Go constants = %q, %q", ipc.ProviderScreenScraper, ipc.ProviderScreenScraperDev)
		}
		want := []string{"igdb", "steamgriddb", "screenscraper", "screenscraperdev"}
		for _, method := range []string{ipc.MethodProviderKeyStatus, ipc.MethodProviderSetKey, ipc.MethodProviderDeleteKey} {
			if got := c.paramEnum(t, method, "provider"); !slices.Equal(got, want) {
				t.Errorf("%s provider enum = %v, want %v", method, got, want)
			}
		}
		var p struct {
			Enum []string `json:"enum"`
		}
		if err := json.Unmarshal(c.schema(t, "ProviderKeyStatus").Properties["provider"], &p); err != nil {
			t.Fatalf("parse ProviderKeyStatus.provider: %v", err)
		}
		if !slices.Equal(p.Enum, want) {
			t.Errorf("ProviderKeyStatus.provider enum = %v, want %v", p.Enum, want)
		}
		secret := paramSchema(t, methodByName(t, doc, "provider.setKey"), "secret")
		if d, _ := secret["description"].(string); !strings.Contains(d, "required (1..512) for igdb, screenscraper and screenscraperdev") {
			t.Errorf("provider.setKey secret description = %q", d)
		}
		for _, r := range []string{"MetadataRef", "MetadataCandidate"} {
			_ = r
		}
		var mref struct {
			Enum []string `json:"enum"`
		}
		if err := json.Unmarshal(c.schema(t, "MetadataRef").Properties["provider"], &mref); err != nil {
			t.Fatalf("parse MetadataRef.provider: %v", err)
		}
		if w := []string{"igdb", "steamgriddb", "steamstore"}; !slices.Equal(mref.Enum, w) {
			t.Errorf("MetadataRef.provider enum = %v, want unchanged %v", mref.Enum, w)
		}
		if got := c.paramEnum(t, ipc.MethodMediaQueue, "provider"); slices.Contains(got, "screenscraper") {
			t.Errorf("media.queue provider enum = %v, want no screenscraper", got)
		}
	})

	t.Run("VID-005/MediaItem.kind ganha video (media.queue.kinds não)", func(t *testing.T) {
		t.Parallel()
		var k struct {
			Enum []string `json:"enum"`
		}
		if err := json.Unmarshal(c.schema(t, "MediaItem").Properties["kind"], &k); err != nil {
			t.Fatalf("parse MediaItem.kind: %v", err)
		}
		if want := []string{"cover", "screenshot", "title", "banner", "background", "select", "video"}; !slices.Equal(k.Enum, want) {
			t.Errorf("MediaItem.kind enum = %v, want %v", k.Enum, want)
		}
		var thumb struct {
			Description string `json:"description"`
		}
		if err := json.Unmarshal(c.schema(t, "MediaItem").Properties["thumbPath"], &thumb); err != nil {
			t.Fatalf("parse MediaItem.thumbPath: %v", err)
		}
		if !strings.Contains(strings.ToLower(thumb.Description), "empty for video") {
			t.Errorf("MediaItem.thumbPath description = %q, want it to say \"empty for video\"", thumb.Description)
		}
		kinds := mustObject(t, mustObject(t, paramSchema(t, methodByName(t, doc, "media.queue"), "kinds")["items"], "kinds items"), "kinds items")
		if enum, _ := kinds["enum"].([]any); slices.Contains(enum, any("video")) {
			t.Errorf("media.queue kinds enum = %v, want no video", enum)
		}
	})

	t.Run("CON-017/SessionItem.endReason lista client-no-launch e closed-by-hotkey (ou null)", func(t *testing.T) {
		t.Parallel()
		var er struct {
			OneOf []struct {
				Type string   `json:"type"`
				Enum []string `json:"enum"`
			} `json:"oneOf"`
		}
		if err := json.Unmarshal(c.schema(t, "SessionItem").Properties["endReason"], &er); err != nil {
			t.Fatalf("parse SessionItem.endReason: %v", err)
		}
		if len(er.OneOf) != 2 || er.OneOf[1].Type != "null" {
			t.Fatalf("endReason oneOf = %+v, want [string enum, null]", er.OneOf)
		}
		if want := []string{"exited", "agent-shutdown", "recovered", "client-no-launch", "closed-by-hotkey"}; !slices.Equal(er.OneOf[0].Enum, want) {
			t.Errorf("endReason enum = %v, want %v", er.OneOf[0].Enum, want)
		}
	})

	t.Run("CON-031/game.launch e game.launchUntracked: SessionActive nos erros e summary com a recusa durante o fechamento pelo combo", func(t *testing.T) {
		t.Parallel()
		for _, name := range []string{"game.launch", "game.launchUntracked"} {
			m := methodByName(t, doc, name)
			if s, _ := m["summary"].(string); !strings.Contains(s, launchRefusedSentence) || !strings.Contains(s, "[CON-031]") {
				t.Errorf("%s summary = %q, want the CON-031 sentence", name, s)
			}
			if !slices.Contains(methodErrorRefs(t, m), "SessionActive") {
				t.Errorf("%s errors = %v, want SessionActive", name, methodErrorRefs(t, m))
			}
		}
		if s, _ := methodByName(t, doc, "game.launchUntracked")["summary"].(string); strings.Contains(s, "no Job Object") ||
			!strings.Contains(s, "closing the agent never kills it [CON-011]") {
			t.Errorf("game.launchUntracked summary = %q, want the Job Object kept by the agent only for the combo", s)
		}
	})

	t.Run("CON-026/x-console: valores literais do delta §1", func(t *testing.T) {
		t.Parallel()
		xc := mustObject(t, genericSection(t, doc, "info")["x-console"], "info.x-console")
		if d, _ := xc["description"].(string); d == "" {
			t.Error("x-console has no description")
		}
		delete(xc, "description")
		want := map[string]any{
			"closeCombo":            "Back+B",
			"closeHoldMs":           1000.0,
			"closeDebounceMs":       300.0,
			"closeWaitMs":           3000.0,
			"closeDetachedPollMs":   250.0,
			"closeHandlerTimeoutMs": 6000.0,
			"refusedCue":            []any{[]any{300.0, 100.0}, []any{200.0, 200.0}},
		}
		if !reflect.DeepEqual(xc, want) {
			t.Errorf("info.x-console =\n%v\nwant\n%v", xc, want)
		}
		// No Go constant exists yet for these agent values (they live in agent/domain); when they do,
		// this case is extended to compare them with the JSON.
	})

	t.Run("VID-005/x-video: valores literais do delta §1", func(t *testing.T) {
		t.Parallel()
		xv := mustObject(t, genericSection(t, doc, "info")["x-video"], "info.x-video")
		if d, _ := xv["description"].(string); d == "" {
			t.Error("x-video has no description")
		}
		delete(xv, "description")
		want := map[string]any{
			"kindLimit":         1.0,
			"mediaType":         "video-normalized",
			"maxVideoBytes":     20971520.0,
			"laneWorkers":       1.0,
			"requestIntervalMs": 1500.0,
			"quotaPauseMs":      3600000.0,
			"maxQueueGames":     1000.0,
			"softname":          "arcadia",
		}
		if !reflect.DeepEqual(xv, want) {
			t.Errorf("info.x-video =\n%v\nwant\n%v", xv, want)
		}
		// maxQueueGames is the same bound as the gameIds maxItems and the skipped maxItems.
		s := paramSchema(t, methodByName(t, doc, "video.queue"), "gameIds")
		if s["maxItems"] != want["maxQueueGames"] {
			t.Errorf("gameIds maxItems = %v, x-video.maxQueueGames = %v", s["maxItems"], want["maxQueueGames"])
		}
		// x-metadata does not gain screenscraper (SSC-005).
		xm := mustObject(t, genericSection(t, doc, "info")["x-metadata"], "info.x-metadata")
		if iv := mustObject(t, xm["requestIntervalMs"], "x-metadata.requestIntervalMs"); iv["screenscraper"] != nil {
			t.Errorf("x-metadata.requestIntervalMs = %v, want no screenscraper", iv)
		}
	})

	t.Run("CON-026/v0.15.0 é a v0.14.0 mais acréscimos", func(t *testing.T) {
		t.Parallel()
		prev := loadGeneric(t, contractV014Path)
		next := loadGeneric(t, contractPath)

		// info: version and description change; x-console and x-video are new; the rest is equal.
		pinfo := genericSection(t, prev, "info")
		ninfo := genericSection(t, next, "info")
		if pinfo["version"] != "0.14.0" || ninfo["version"] != "0.15.0" {
			t.Errorf("info.version v0.14.0 = %v, v0.15.0 = %v", pinfo["version"], ninfo["version"])
		}
		for _, k := range []string{"x-console", "x-video"} {
			if _, ok := ninfo[k]; !ok {
				t.Errorf("v0.15.0 info has no %s", k)
			}
			delete(ninfo, k)
		}
		for _, k := range []string{"version", "description"} {
			delete(pinfo, k)
			delete(ninfo, k)
		}

		pMethods := mustArray(t, prev["methods"], "v0.14.0 methods")
		nMethods := mustArray(t, next["methods"], "v0.15.0 methods")

		// Order: the four video.* follow metadata.batchDiscard, game.closedByHotkey follows metadata.batchChanged.
		iDiscard := methodIndex(t, nMethods, "metadata.batchDiscard")
		for i, name := range videoMethods {
			if pos := iDiscard + 1 + i; pos >= len(nMethods) || methodName(nMethods[pos]) != name {
				t.Errorf("v0.15.0 method after metadata.batchDiscard #%d, want %q", i, name)
			}
		}
		if got, want := methodIndex(t, nMethods, "game.closedByHotkey"), methodIndex(t, nMethods, ipc.MethodMetadataBatchChanged)+1; got != want {
			t.Errorf("game.closedByHotkey at %d, want right after metadata.batchChanged (%d)", got, want)
		}

		// Summaries of game.launch/game.launchUntracked: the sentence is appended (or substituted for launchUntracked).
		for _, name := range []string{"game.launch", "game.launchUntracked"} {
			pm := findMethod(t, pMethods, name)
			nm := findMethod(t, nMethods, name)
			ps, _ := pm["summary"].(string)
			ns, _ := nm["summary"].(string)
			if name == "game.launch" && !strings.HasPrefix(ns, ps) {
				t.Errorf("v0.15.0 %s summary = %q, want it to start with the v0.14.0 one", name, ns)
			}
			if !strings.Contains(ns, launchRefusedSentence) {
				t.Errorf("v0.15.0 %s summary has no CON-031 sentence", name)
			}
			nm["summary"] = ps
		}

		// Provider enums (three params), the secret description, MediaItem.kind/thumbPath and SessionItem.endReason.
		wantProv := []any{"igdb", "steamgriddb", "screenscraper", "screenscraperdev"}
		for _, name := range []string{"provider.keyStatus", "provider.setKey", "provider.deleteKey"} {
			ps := paramSchema(t, findMethod(t, pMethods, name), "provider")
			ns := paramSchema(t, findMethod(t, nMethods, name), "provider")
			if !reflect.DeepEqual(ns["enum"], wantProv) {
				t.Errorf("v0.15.0 %s provider enum = %v, want %v", name, ns["enum"], wantProv)
			}
			ns["enum"] = ps["enum"]
		}
		ps := paramSchema(t, findMethod(t, pMethods, "provider.setKey"), "secret")
		ns := paramSchema(t, findMethod(t, nMethods, "provider.setKey"), "secret")
		ns["description"] = ps["description"]
		if ps["description"] == nil {
			delete(ns, "description")
		}

		pComps := genericSection(t, prev, "components")
		nComps := genericSection(t, next, "components")
		pSchemas := mustObject(t, pComps["schemas"], "v0.14.0 components.schemas")
		nSchemas := mustObject(t, nComps["schemas"], "v0.15.0 components.schemas")
		if got, want := takeAddedKeys(t, "components.schemas", pSchemas, nSchemas), slices.Sorted(slices.Values(consoleNewSchemas)); !slices.Equal(got, want) {
			t.Errorf("new components.schemas = %v, want %v", got, want)
		}

		kProps := genericSchemaProps(t, next, "ProviderKeyStatus")
		pkProps := genericSchemaProps(t, prev, "ProviderKeyStatus")
		mustObject(t, kProps["provider"], "ProviderKeyStatus.provider")["enum"] = mustObject(t, pkProps["provider"], "prev ProviderKeyStatus.provider")["enum"]

		miProps := genericSchemaProps(t, next, "MediaItem")
		pmiProps := genericSchemaProps(t, prev, "MediaItem")
		nKind := mustObject(t, miProps["kind"], "MediaItem.kind")
		pKind := mustObject(t, pmiProps["kind"], "prev MediaItem.kind")
		if enum := mustArray(t, nKind["enum"], "MediaItem.kind enum"); len(enum) != len(mustArray(t, pKind["enum"], "prev kind enum"))+1 || enum[len(enum)-1] != "video" {
			t.Errorf("v0.15.0 MediaItem.kind enum = %v, want the v0.14.0 values plus video", enum)
		}
		nKind["enum"] = pKind["enum"]
		nThumb := mustObject(t, miProps["thumbPath"], "MediaItem.thumbPath")
		pThumb := mustObject(t, pmiProps["thumbPath"], "prev MediaItem.thumbPath")
		if d, _ := nThumb["description"].(string); !strings.Contains(strings.ToLower(d), "empty for video") {
			t.Errorf("v0.15.0 MediaItem.thumbPath description = %q", d)
		}
		nThumb["description"] = pThumb["description"]
		if pThumb["description"] == nil {
			delete(nThumb, "description")
		}

		seProps := genericSchemaProps(t, next, "SessionItem")
		pseProps := genericSchemaProps(t, prev, "SessionItem")
		nEnd := mustObject(t, seProps["endReason"], "SessionItem.endReason")
		pEnd := mustObject(t, pseProps["endReason"], "prev SessionItem.endReason")
		nStr := mustObject(t, mustArray(t, nEnd["oneOf"], "endReason oneOf")[0], "endReason string")
		pStr := mustObject(t, mustArray(t, pEnd["oneOf"], "prev endReason oneOf")[0], "prev endReason string")
		wantEnd := []any{"exited", "agent-shutdown", "recovered", "client-no-launch", "closed-by-hotkey"}
		if !reflect.DeepEqual(nStr["enum"], wantEnd) {
			t.Errorf("v0.15.0 endReason enum = %v, want %v", nStr["enum"], wantEnd)
		}
		nStr["enum"] = pStr["enum"]

		// Drop the new methods and compare the rest in order.
		newMethods := map[string]bool{"game.closedByHotkey": true}
		for _, name := range videoMethods {
			newMethods[name] = true
		}
		var kept []any
		for _, m := range nMethods {
			if !newMethods[methodName(m)] {
				kept = append(kept, m)
			}
		}
		next["methods"] = kept

		// No error code is added or changed.
		if !reflect.DeepEqual(pComps["errors"], nComps["errors"]) {
			t.Error("v0.15.0 components.errors differs from v0.14.0; the delta adds no error")
		}

		if !reflect.DeepEqual(prev, next) {
			t.Error("v0.15.0 differs from v0.14.0 beyond info.version and description, x-console, x-video, the four video.* methods, game.closedByHotkey, the launch summaries, the provider/kind/endReason enums, the new schemas and the two descriptions")
		}
	})
}
