package agent

// Tests for T28 of docs/specs/phase-12-metadados-unificados/tasks.md, invariants of the phase (MET-081,
// MET-082): the batch and the apply never write tracking_rule, play_session or pause_interval; no file of
// the phase imports internal/tracker; internal/tracker and the migrations get no new file; and the tests of
// the phase do not sleep nor make external requests. The static checks read the sources of the package
// (relative paths, the test runs in internal/agent); the dynamic ones use the real store and the real agent.

import (
	"context"
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
	"github.com/arcadia-launcher/arcadia/internal/scrapers"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

// trackingTables are the tables of the tracking, which the metadata features must not write [MET-081].
var trackingTables = []string{"tracking_rule", "play_session", "pause_interval"}

// trackingSnapshot returns every row of the tracking tables, in rowid order, as text (MET-081).
func trackingSnapshot(t *testing.T, st *store.Store) map[string][]string {
	t.Helper()
	out := map[string][]string{}
	for _, table := range trackingTables {
		rows, err := st.DB().QueryContext(context.Background(), "SELECT * FROM "+table+" ORDER BY rowid")
		if err != nil {
			t.Fatalf("query %s: %v", table, err)
		}
		cols, err := rows.Columns()
		if err != nil {
			rows.Close()
			t.Fatalf("columns of %s: %v", table, err)
		}
		var lines []string
		for rows.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				rows.Close()
				t.Fatalf("scan %s: %v", table, err)
			}
			lines = append(lines, fmt.Sprint(vals...))
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			t.Fatalf("rows %s: %v", table, err)
		}
		rows.Close()
		out[table] = lines
	}
	return out
}

// assertTrackingUnchanged fails the test for every tracking table whose rows differ between before and after.
func assertTrackingUnchanged(t *testing.T, when string, before, after map[string][]string) {
	t.Helper()
	for _, table := range trackingTables {
		if !slices.Equal(before[table], after[table]) {
			t.Errorf("%s: %s changed\n before: %v\n after:  %v", when, table, before[table], after[table])
		}
	}
}

func TestMetadataDoesNotWriteTracking(t *testing.T) {
	t.Run("MET-081/aplicação com sessão pausada não escreve em tracking_rule, play_session nem pause_interval", func(t *testing.T) {
		fakes := map[string]scrapers.MetadataProvider{
			ipc.MetadataProviderIGDB: &metaProv{id: ipc.MetadataProviderIGDB, meta: scrapers.Metadata{Title: "Celeste"}},
		}
		f := newMetaFixture(t, fakes)
		ctx := t.Context()

		t0 := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)
		sessID, err := f.st.StartSession(ctx, f.gameID, t0)
		if err != nil {
			t.Fatalf("StartSession: %v", err)
		}
		if err := f.st.PauseSession(ctx, store.PauseStart{
			SessionID: sessID, At: t0.Add(10 * time.Second), Reason: domain.PauseHotkey, Active: 10 * time.Second,
		}); err != nil {
			t.Fatalf("PauseSession: %v", err)
		}

		before := trackingSnapshot(t, f.st)
		applyOK(t, f, ipc.MetadataApplyParams{GameID: f.gameID, Refs: refsOf("7", ipc.MetadataProviderIGDB)})
		assertTrackingUnchanged(t, "metadata.apply", before, trackingSnapshot(t, f.st))
	})

	t.Run("MET-081/lote em andamento com sessão ativa não escreve em tracking_rule, play_session nem pause_interval", func(t *testing.T) {
		started := make(chan struct{}, 8)
		w := newWiring(t, wiringOpts{
			mutate: func(cfg *config) {
				cfg.Providers = blockingBatchProviders(started, func(string) {})
			},
		})
		c := w.client(t)
		gameID, _ := w.addGame(t, c, "Tracking Batch Game")
		w.launch(t, c, gameID)
		c.rpcOK(t, ipc.MethodSessionPause, nil)
		_ = c.waitChanges(t, 2)

		st := openDB(t, w.cfg)
		defer st.Close()
		before := trackingSnapshot(t, st)

		c.rpcOK(t, ipc.MethodMetadataBatchStart, ipc.BatchStartParams{GameIDs: []int64{gameID}})
		waitSearchStarted(t, started)
		assertTrackingUnchanged(t, "metadata.batchStart with a search in progress", before, trackingSnapshot(t, st))
	})
}

// importsOf returns the import paths of a Go file, parsed without its declarations.
func importsOf(t *testing.T, path string) []string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse imports of %s: %v", path, err)
	}
	var out []string
	for _, imp := range f.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			t.Fatalf("import path %s in %s: %v", imp.Path.Value, path, err)
		}
		out = append(out, p)
	}
	return out
}

// globOrFail returns the files matching pattern and fails the test when there are none (so an invariant
// cannot pass by matching nothing).
func globOrFail(t *testing.T, pattern string) []string {
	t.Helper()
	files, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("glob %s: %v", pattern, err)
	}
	if len(files) == 0 {
		t.Fatalf("glob %s matched no file", pattern)
	}
	return files
}

func TestMetadataPhaseInvariants(t *testing.T) {
	t.Run("MET-081/nenhum arquivo da fase importa internal/tracker", func(t *testing.T) {
		var files []string
		files = append(files, globOrFail(t, "metadata*.go")...)
		files = append(files, globOrFail(t, "scrape*.go")...)
		files = append(files, globOrFail(t, filepath.Join("..", "scrapers", "*.go"))...)
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			for _, imp := range importsOf(t, f) {
				if strings.Contains(imp, "/internal/tracker") {
					t.Errorf("%s imports %s; the batch and the apply must not depend on the tracker [MET-080, MET-081]", f, imp)
				}
			}
		}
	})

	t.Run("MET-081/internal/tracker e as migrações não ganham arquivo nesta fase", func(t *testing.T) {
		// Reference list of the files after phase 13 (alterado em 2026-11-15 (fase 13)): phase 12 itself
		// added no migration and no tracker file; phase 13 added the files below (close.go, closer_*,
		// detached_windows.go, the close/kill tests and 00009_video.sql). A file beyond this list
		// is a new change that must be justified by its own phase. MET-081 intent (the batch and the
		// apply do not write the tracking tables nor import internal/tracker) is checked by the other cases.
		wantTracker := []string{
			"adopt_steam_test.go", "adopt_test.go", "adopted_close_test.go", "adopted_job.go", "adopted_job_test.go",
			"close.go", "close_const_test.go", "closer_other.go", "closer_windows.go", "closer_windows_test.go",
			"detached_windows.go", "fakes_test.go", "integration_windows_test.go", "job_kill_windows_test.go",
			"job_other.go", "job_windows.go", "job_windows_test.go", "launcher.go", "machine.go", "machine_test.go",
			"main_windows_test.go", "match.go", "process_killer_windows_test.go", "recover.go", "recover_test.go",
			"tracker.go", "tracker_close_adopted_test.go", "tracker_close_test.go", "tracker_detached_test.go",
			"tracker_test.go", "uri_test.go",
		}
		wantMigrations := []string{
			"00001_initial.sql", "00002_app_setting.sql", "00003_session_end_reason.sql",
			"00004_pause_interval_open_uidx.sql", "00005_fts_rebuild.sql", "00006_emulators.sql",
			"00007_media.sql", "00008_platforms.sql", "00009_video.sql",
		}
		if len(wantMigrations) != 9 {
			t.Fatalf("reference has %d migrations, want 9 (SchemaVersion 9)", len(wantMigrations))
		}
		slices.Sort(wantTracker)
		slices.Sort(wantMigrations)
		for _, c := range []struct {
			name, dir string
			want      []string
		}{
			{"internal/tracker", filepath.Join("..", "tracker"), wantTracker},
			{"internal/store/migrations", filepath.Join("..", "store", "migrations"), wantMigrations},
		} {
			entries, err := os.ReadDir(c.dir)
			if err != nil {
				t.Fatalf("read %s: %v", c.name, err)
			}
			var got []string
			for _, e := range entries {
				// alterado em 2026-11-22 (fase 15): 00010 belongs to phase 15, not to this reference list.
				// alterado em 2026-11-25 (fase 16): 00011 also belongs to a later phase.
				if !e.IsDir() && e.Name() != "00010_time_to_beat.sql" && e.Name() != "00011_session_edit.sql" {
					got = append(got, e.Name())
				}
			}
			slices.Sort(got)
			if !slices.Equal(got, c.want) {
				t.Errorf("%s files changed in phase 12:\n got:  %v\n want: %v", c.name, got, c.want)
			}
		}
	})

	t.Run("MET-082/os testes da fase não dormem nem fazem requisição externa", func(t *testing.T) {
		var files []string
		files = append(files, globOrFail(t, "metadata*_test.go")...)
		files = append(files, globOrFail(t, filepath.Join("..", "scrapers", "*_test.go"))...)
		// Built by concatenation so this file does not match its own check.
		forbidden := []string{"time." + "Sleep(", "http." + "Get(", "http." + "Post(", "http." + "DefaultClient", "http." + "DefaultTransport"}
		for _, f := range files {
			data, err := os.ReadFile(f)
			if err != nil {
				t.Fatalf("read %s: %v", f, err)
			}
			for _, bad := range forbidden {
				if strings.Contains(string(data), bad) {
					t.Errorf("%s uses %s; use domain.FakeClock and httptest/fakes [MET-082]", f, bad)
				}
			}
		}
	})
}

// TestMetadataLeavesSessionsAlone covers MET-080 on the live agent: the batch runs while a session is
// active and paused, and neither the session rows, nor the session.changed notifications, nor the launches
// of the tracker change. The tracker is concrete, so its "no call" is observed through the fake launcher
// (every launch goes through Prepare) and through the session rows.
func TestMetadataLeavesSessionsAlone(t *testing.T) {
	t.Run("MET-080/busca em lote com sessão ativa: a sessão, play_session e session.changed ficam intactos", func(t *testing.T) {
		started := make(chan struct{}, 8)
		w := newWiring(t, wiringOpts{
			mutate: func(cfg *config) {
				cfg.Providers = blockingBatchProviders(started, func(string) {})
			},
		})
		c := w.client(t)
		gameID, _ := w.addGame(t, c, "Session Batch Game")
		w.launch(t, c, gameID)
		c.rpcOK(t, ipc.MethodSessionPause, nil)
		_ = c.waitChanges(t, 2)

		st := openDB(t, w.cfg)
		defer st.Close()
		sessionsBefore := readSessions(t, st)
		changesBefore := len(c.flushedChanges(t))
		launchesBefore := len(w.fl.prepareCalls())

		c.rpcOK(t, ipc.MethodMetadataBatchStart, ipc.BatchStartParams{GameIDs: []int64{gameID}})
		waitSearchStarted(t, started)

		if got := len(c.flushedChanges(t)); got != changesBefore {
			t.Errorf("session.changed count = %d after metadata.batchStart, want %d (no new notification)", got, changesBefore)
		}
		if got := readSessions(t, st); !reflect.DeepEqual(got, sessionsBefore) {
			t.Errorf("play_session changed during the batch:\n before: %v\n after:  %v", sessionsBefore, got)
		}
		if got := len(w.fl.prepareCalls()); got != launchesBefore {
			t.Errorf("launcher prepared %d times during the batch, want %d", got, launchesBefore)
		}
	})

	t.Run("MET-080/nenhum método de lote toca no tracker", func(t *testing.T) {
		started := make(chan struct{}, 8)
		w := newWiring(t, wiringOpts{
			mutate: func(cfg *config) {
				cfg.Providers = blockingBatchProviders(started, func(string) {})
			},
		})
		c := w.client(t)
		gameID, _ := w.addGame(t, c, "Tracker Spy Game")
		w.launch(t, c, gameID)
		c.rpcOK(t, ipc.MethodSessionPause, nil)
		_ = c.waitChanges(t, 2)

		st := openDB(t, w.cfg)
		defer st.Close()
		sessionsBefore := readSessions(t, st)
		launchesBefore := len(w.fl.prepareCalls())

		var start ipc.BatchStartResult
		resultInto(t, c.rpcOK(t, ipc.MethodMetadataBatchStart, ipc.BatchStartParams{GameIDs: []int64{gameID}}), &start)
		waitSearchStarted(t, started)

		// Each batch method is called once, and the tracker state is checked after each one.
		calls := []struct {
			method string
			params any
		}{
			{ipc.MethodMetadataBatchStatus, nil},
			{ipc.MethodMetadataBatchResults, map[string]any{"batchId": start.BatchID, "offset": 0, "limit": 10}},
			{ipc.MethodMetadataBatchCancel, map[string]any{"batchId": start.BatchID}},
		}
		for _, call := range calls {
			resp := c.rpc(t, call.method, call.params)
			// A refused call (for example batchResults before ready) is fine; only the tracker matters here.
			_ = resp
			if got := len(w.fl.prepareCalls()); got != launchesBefore {
				t.Errorf("%s: launcher prepared %d times, want %d", call.method, got, launchesBefore)
			}
			if got := readSessions(t, st); !reflect.DeepEqual(got, sessionsBefore) {
				t.Errorf("%s changed play_session:\n before: %v\n after:  %v", call.method, sessionsBefore, got)
			}
		}
	})
}

// TestMetadataConstantsMatchContract covers MET-070: the agent constants of metadata_consts.go equal the
// x-metadata block of the phase 12 contract (v0.14.0). Milliseconds in the JSON are compared with the
// durations of the agent; the JSON numbers are read as float64 like the other contract tests.
func TestMetadataConstantsMatchContract(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "specs", "phase-12-metadados-unificados", "contracts", "ipc.openrpc.json"))
	if err != nil {
		t.Fatalf("read contract: %v", err)
	}
	var doc struct {
		Info struct {
			XMetadata map[string]json.RawMessage `json:"x-metadata"`
		} `json:"info"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decode contract: %v", err)
	}
	x := doc.Info.XMetadata
	if x == nil {
		t.Fatal("contract has no info.x-metadata block")
	}
	num := func(key string) float64 {
		t.Helper()
		var v float64
		if err := json.Unmarshal(x[key], &v); err != nil {
			t.Fatalf("x-metadata.%s: %v", key, err)
		}
		return v
	}
	ms := func(d time.Duration) float64 { return float64(d / time.Millisecond) }

	t.Run("MET-070/contadores e limites iguais ao x-metadata", func(t *testing.T) {
		cases := []struct {
			key string
			got float64
		}{
			{"maxSearchCandidates", maxSearchCandidates},
			{"maxRowCandidates", maxRowCandidates},
			{"batchWorkers", batchWorkers},
			{"maxBatchGames", maxBatchGames},
			{"maxBatchChoices", maxBatchChoices},
			{"defaultResultsLimit", defaultResultsLimit},
			{"maxResultsLimit", maxResultsLimit},
			{"maxResultBytes", maxResultBytes},
			{"providerMaxFailures", providerMaxFailures},
			{"maxStatusWarnings", maxStatusWarnings},
			{"maxFailedTitles", maxFailedTitles},
		}
		for _, c := range cases {
			t.Run("MET-070/"+c.key, func(t *testing.T) {
				if want := num(c.key); c.got != want {
					t.Errorf("%s = %v, contract %v", c.key, c.got, want)
				}
			})
		}
	})

	t.Run("MET-070/intervalos e prazos em milissegundos iguais ao x-metadata", func(t *testing.T) {
		cases := []struct {
			key string
			got float64
		}{
			{"searchTimeoutMs", ms(searchTimeout)},
			{"gameSearchTimeoutMs", ms(gameSearchTimeout)},
			{"progressWindowMs", ms(progressWindow)},
		}
		for _, c := range cases {
			t.Run("MET-070/"+c.key, func(t *testing.T) {
				if want := num(c.key); c.got != want {
					t.Errorf("%s = %v ms, contract %v ms", c.key, c.got, want)
				}
			})
		}
	})

	t.Run("MET-070/providerCooldownMs iguais a providerCooldown", func(t *testing.T) {
		var want []float64
		if err := json.Unmarshal(x["providerCooldownMs"], &want); err != nil {
			t.Fatalf("x-metadata.providerCooldownMs: %v", err)
		}
		got := make([]float64, len(providerCooldown))
		for i, d := range providerCooldown {
			got[i] = ms(d)
		}
		if !slices.Equal(got, want) {
			t.Errorf("providerCooldown = %v ms, contract %v ms", got, want)
		}
	})
}
