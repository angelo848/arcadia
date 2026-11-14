package agent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/importers"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

// fakeImportStore keeps the library in memory and records every write.
type fakeImportStore struct {
	platforms map[string]int64
	targets   map[string]bool // "type|target" already in the library
	failTitle string          // ApplyImport fails for this title
	applied   []store.ImportGame
	ensured   []string

	windowsID      int64   // platform of key windows; 0 = it does not exist
	ensuredWindows int     // EnsureWindowsPlatformID calls
	dupPlatformIDs []int64 // platform ids ImportDuplicate was asked about
	byNameCalls    []string
}

func newFakeImportStore() *fakeImportStore {
	return &fakeImportStore{platforms: map[string]int64{}, targets: map[string]bool{}}
}

func (f *fakeImportStore) EnsureWindowsPlatformID(context.Context) (int64, error) {
	f.ensuredWindows++
	if f.windowsID == 0 {
		f.windowsID = 1000
	}
	return f.windowsID, nil
}

func (f *fakeImportStore) WindowsPlatformID(context.Context) (int64, bool, error) {
	return f.windowsID, f.windowsID != 0, nil
}

func (f *fakeImportStore) PlatformIDByName(_ context.Context, name string) (int64, bool, error) {
	f.byNameCalls = append(f.byNameCalls, name)
	id, ok := f.platforms[name]
	return id, ok, nil
}

func (f *fakeImportStore) ImportDuplicate(_ context.Context, launchType, target, _ string, platformID int64) (bool, error) {
	f.dupPlatformIDs = append(f.dupPlatformIDs, platformID)
	return f.targets[launchType+"|"+target], nil
}

func (f *fakeImportStore) EnsurePlatformID(_ context.Context, name string, _ domain.PlatformCategory) (int64, error) {
	f.ensured = append(f.ensured, name)
	if id, ok := f.platforms[name]; ok {
		return id, nil
	}
	id := int64(len(f.platforms) + 1)
	f.platforms[name] = id
	return id, nil
}

func (f *fakeImportStore) ApplyImport(_ context.Context, g store.ImportGame, _ time.Time) (int64, error) {
	if g.Title == f.failTitle {
		return 0, errors.New("disk full")
	}
	key := g.LaunchType + "|" + g.LaunchTarget
	if f.targets[key] {
		return 0, store.ErrDuplicate
	}
	f.targets[key] = true
	f.applied = append(f.applied, g)
	return int64(len(f.applied)), nil
}

// fakeImportSource returns a fixed result or an error.
type fakeImportSource struct {
	id  string
	res importers.Result
	err error
}

func (f fakeImportSource) ID() string { return f.id }

func (f fakeImportSource) Scan(context.Context, importers.ScanInput) (importers.Result, error) {
	return f.res, f.err
}

// fakeLocations reports no launcher folders.
type fakeLocations struct{}

func (fakeLocations) SteamPath() (string, error)         { return "", errors.New("not installed") }
func (fakeLocations) EpicManifestsPath() (string, error) { return "", errors.New("not installed") }

func quietLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func rawParams(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	return b
}

// invalidParams reports whether err is the -32602 JSON-RPC error.
func invalidParams(err error) bool {
	var rpcErr *ipc.Error
	return errors.As(err, &rpcErr) && rpcErr.Code == ipc.CodeInvalidParams
}

func TestImportHandlers(t *testing.T) {
	t.Parallel()

	steamCandidate := importers.Candidate{
		Source: importers.SourceSteam, Title: "Counter-Strike 2", Platform: importers.PlatformWindows,
		LaunchType: importers.LaunchURI, LaunchTarget: "steam://rungameid/730",
	}
	folderCandidate := importers.Candidate{
		Source: importers.SourceFile, Title: "ABZU", LaunchType: importers.LaunchExe,
		LaunchTarget: `D:\ABZU\ABZU.exe`, InstallDir: `D:\ABZU`,
	}

	t.Run("IMP-001/scan não grava nada", func(t *testing.T) {
		t.Parallel()
		st := newFakeImportStore()
		sources := map[string]importers.Source{
			ipc.ImportSourceSteam: fakeImportSource{id: "steam", res: importers.Result{Candidates: []importers.Candidate{steamCandidate}}},
		}
		h := importHandlers(st, fakeLocations{}, testClock(), sources, nil, quietLog())[ipc.MethodImportScan]
		if _, err := h(context.Background(), rawParams(t, map[string]string{"source": "steam"})); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if len(st.applied) != 0 || len(st.ensured) != 0 {
			t.Errorf("scan wrote: applied=%d ensured=%v", len(st.applied), st.ensured)
		}
	})

	t.Run("IMP-001/apply grava só os selecionados", func(t *testing.T) {
		t.Parallel()
		st := newFakeImportStore()
		h := importHandlers(st, fakeLocations{}, testClock(), nil, nil, quietLog())[ipc.MethodImportApply]
		params := ipc.ImportApplyParams{Candidates: []ipc.ImportCandidate{
			toIPCCandidate(steamCandidate, false),
		}}
		out, err := h(context.Background(), rawParams(t, params))
		if err != nil {
			t.Fatalf("apply: %v", err)
		}
		res := out.(ipc.ImportApplyResult)
		if len(res.Applied) != 1 || res.Applied[0].Title != "Counter-Strike 2" || len(st.applied) != 1 {
			t.Errorf("result = %+v, applied = %d", res, len(st.applied))
		}
	})

	t.Run("IMP-006/scan marca exists para candidatos já na biblioteca", func(t *testing.T) {
		t.Parallel()
		st := newFakeImportStore()
		st.targets[importers.LaunchURI+"|steam://rungameid/730"] = true
		sources := map[string]importers.Source{
			ipc.ImportSourceSteam: fakeImportSource{id: "steam", res: importers.Result{Candidates: []importers.Candidate{steamCandidate}}},
		}
		h := importHandlers(st, fakeLocations{}, testClock(), sources, nil, quietLog())[ipc.MethodImportScan]
		out, err := h(context.Background(), rawParams(t, map[string]string{"source": "steam"}))
		if err != nil {
			t.Fatalf("scan: %v", err)
		}
		res := out.(ipc.ImportScanResult)
		if len(res.Candidates) != 1 || !res.Candidates[0].Exists {
			t.Errorf("candidates = %+v, want exists=true", res.Candidates)
		}
	})

	t.Run("IMP-008/uma fonte com erro não derruba as outras", func(t *testing.T) {
		t.Parallel()
		st := newFakeImportStore()
		sources := map[string]importers.Source{
			ipc.ImportSourceSteam: fakeImportSource{id: "steam", err: errors.New("boom")},
			ipc.ImportSourceFile:  fakeImportSource{id: "file", res: importers.Result{Candidates: []importers.Candidate{folderCandidate}}},
		}
		h := importHandlers(st, fakeLocations{}, testClock(), sources, nil, quietLog())[ipc.MethodImportScan]
		out, err := h(context.Background(), rawParams(t, map[string]string{"source": "steam"}))
		if err != nil {
			t.Fatalf("scan of a failing source must not be an error: %v", err)
		}
		if res := out.(ipc.ImportScanResult); len(res.Warnings) == 0 {
			t.Errorf("warnings empty, want the source failure")
		}
		out, err = h(context.Background(), rawParams(t, map[string]string{"source": "file", "path": `D:\ABZU`}))
		if err != nil {
			t.Fatalf("folder scan: %v", err)
		}
		if res := out.(ipc.ImportScanResult); len(res.Candidates) != 1 {
			t.Errorf("file candidates = %d, want 1", len(res.Candidates))
		}
	})

	t.Run("IMP-008/um candidato que falha não impede os demais", func(t *testing.T) {
		t.Parallel()
		st := newFakeImportStore()
		st.failTitle = "Quebra"
		h := importHandlers(st, fakeLocations{}, testClock(), nil, nil, quietLog())[ipc.MethodImportApply]
		bad := importers.Candidate{Source: importers.SourceFile, Title: "Quebra", Platform: "PC", LaunchType: importers.LaunchExe, LaunchTarget: `C:\q.exe`}
		good := importers.Candidate{Source: importers.SourceFile, Title: "Bom", Platform: "PC", LaunchType: importers.LaunchExe, LaunchTarget: `C:\b.exe`}
		out, err := h(context.Background(), rawParams(t, ipc.ImportApplyParams{Candidates: []ipc.ImportCandidate{
			toIPCCandidate(bad, false), toIPCCandidate(good, false),
		}}))
		if err != nil {
			t.Fatalf("apply: %v", err)
		}
		res := out.(ipc.ImportApplyResult)
		if len(res.Applied) != 1 || res.Applied[0].Title != "Bom" {
			t.Errorf("applied = %+v, want only Bom", res.Applied)
		}
		if len(res.Skipped) != 1 || res.Skipped[0].Reason != "failed" {
			t.Errorf("skipped = %+v, want Quebra failed", res.Skipped)
		}
	})

	t.Run("IMP-006/apply devolve duplicate e não grava de novo", func(t *testing.T) {
		t.Parallel()
		st := newFakeImportStore()
		st.targets[importers.LaunchURI+"|steam://rungameid/730"] = true
		h := importHandlers(st, fakeLocations{}, testClock(), nil, nil, quietLog())[ipc.MethodImportApply]
		out, err := h(context.Background(), rawParams(t, ipc.ImportApplyParams{Candidates: []ipc.ImportCandidate{toIPCCandidate(steamCandidate, false)}}))
		if err != nil {
			t.Fatalf("apply: %v", err)
		}
		res := out.(ipc.ImportApplyResult)
		if len(res.Applied) != 0 || len(res.Skipped) != 1 || res.Skipped[0].Reason != "duplicate" {
			t.Errorf("result = %+v, want one duplicate", res)
		}
	})

	t.Run("PLT-026/scan de Steam marca já existe pela plataforma de chave windows", func(t *testing.T) {
		t.Parallel()
		st := newFakeImportStore()
		st.windowsID = 7
		st.platforms["Windows"] = 5 // another platform with the name; must not be used
		sources := map[string]importers.Source{
			ipc.ImportSourceSteam: fakeImportSource{id: "steam", res: importers.Result{Candidates: []importers.Candidate{steamCandidate}}},
		}
		h := importHandlers(st, fakeLocations{}, testClock(), sources, nil, quietLog())[ipc.MethodImportScan]
		if _, err := h(context.Background(), rawParams(t, map[string]string{"source": "steam"})); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if len(st.dupPlatformIDs) != 1 || st.dupPlatformIDs[0] != 7 {
			t.Errorf("ImportDuplicate platform ids = %v, want [7] (key windows)", st.dupPlatformIDs)
		}
		if len(st.byNameCalls) != 0 {
			t.Errorf("platform looked up by name: %v", st.byNameCalls)
		}
	})

	t.Run("PLT-026/scan de Steam e Epic sem plataforma windows não cria nada", func(t *testing.T) {
		t.Parallel()
		st := newFakeImportStore() // windowsID 0: the platform was deleted
		epic := importers.Candidate{Source: importers.SourceEpic, Title: "GTA", Platform: importers.PlatformWindows,
			LaunchType: importers.LaunchURI, LaunchTarget: "com.epicgames.launcher://apps/x?action=launch&silent=true"}
		sources := map[string]importers.Source{
			ipc.ImportSourceSteam: fakeImportSource{id: "steam", res: importers.Result{Candidates: []importers.Candidate{steamCandidate}}},
			ipc.ImportSourceEpic:  fakeImportSource{id: "epic", res: importers.Result{Candidates: []importers.Candidate{epic}}},
		}
		h := importHandlers(st, fakeLocations{}, testClock(), sources, nil, quietLog())[ipc.MethodImportScan]
		for _, src := range []string{"steam", "epic"} {
			if _, err := h(context.Background(), rawParams(t, map[string]string{"source": src})); err != nil {
				t.Fatalf("scan %s: %v", src, err)
			}
		}
		if st.ensuredWindows != 0 || len(st.ensured) != 0 {
			t.Errorf("scan created platforms: windows=%d ensured=%v", st.ensuredWindows, st.ensured)
		}
		for _, id := range st.dupPlatformIDs {
			if id != 0 {
				t.Errorf("ImportDuplicate platform id = %d, want 0 (no windows platform)", id)
			}
		}
	})

	t.Run("PLT-026/apply de Steam e Epic usa EnsureWindowsPlatformID, não o nome", func(t *testing.T) {
		t.Parallel()
		st := newFakeImportStore()
		st.platforms["Windows"] = 5
		h := importHandlers(st, fakeLocations{}, testClock(), nil, nil, quietLog())[ipc.MethodImportApply]
		epic := importers.Candidate{Source: importers.SourceEpic, Title: "GTA", Platform: importers.PlatformWindows,
			LaunchType: importers.LaunchURI, LaunchTarget: "com.epicgames.launcher://apps/x?action=launch&silent=true"}
		noPlatform := importers.Candidate{Source: importers.SourceSteam, Title: "Sem plataforma",
			LaunchType: importers.LaunchURI, LaunchTarget: "steam://rungameid/1"}
		out, err := h(context.Background(), rawParams(t, ipc.ImportApplyParams{Candidates: []ipc.ImportCandidate{
			toIPCCandidate(steamCandidate, false), toIPCCandidate(epic, false), toIPCCandidate(noPlatform, false),
		}}))
		if err != nil {
			t.Fatalf("apply: %v", err)
		}
		if res := out.(ipc.ImportApplyResult); len(res.Applied) != 3 {
			t.Fatalf("result = %+v, want 3 applied", res)
		}
		if st.ensuredWindows == 0 {
			t.Errorf("EnsureWindowsPlatformID not called")
		}
		for _, g := range st.applied {
			if g.PlatformID != st.windowsID {
				t.Errorf("%s: platform id = %d, want %d (key windows)", g.Title, g.PlatformID, st.windowsID)
			}
		}
		if len(st.ensured) != 0 {
			t.Errorf("EnsurePlatformID (by name) called: %v", st.ensured)
		}
	})

	t.Run("PLT-026/candidato de arquivo sem plataforma continua sendo erro", func(t *testing.T) {
		t.Parallel()
		st := newFakeImportStore()
		h := importHandlers(st, fakeLocations{}, testClock(), nil, nil, quietLog())[ipc.MethodImportApply]
		out, err := h(context.Background(), rawParams(t, ipc.ImportApplyParams{Candidates: []ipc.ImportCandidate{toIPCCandidate(folderCandidate, false)}}))
		if err != nil {
			t.Fatalf("apply: %v", err)
		}
		res := out.(ipc.ImportApplyResult)
		if len(res.Applied) != 0 || len(res.Skipped) != 1 || res.Skipped[0].Reason != "failed" || st.ensuredWindows != 0 {
			t.Errorf("result = %+v, windows ensures = %d; want one failed and no platform created", res, st.ensuredWindows)
		}
	})

	t.Run("PLT-028/apply do LaunchBox com plataforma Windows resolve pelo nome", func(t *testing.T) {
		t.Parallel()
		st := newFakeImportStore()
		h := importHandlers(st, fakeLocations{}, testClock(), nil, nil, quietLog())[ipc.MethodImportApply]
		lb := importers.Candidate{Source: importers.SourceLaunchBox, Title: "Starfield", Platform: importers.PlatformWindows,
			LaunchType: importers.LaunchExe, LaunchTarget: `C:\s.exe`}
		if _, err := h(context.Background(), rawParams(t, ipc.ImportApplyParams{Candidates: []ipc.ImportCandidate{toIPCCandidate(lb, false)}})); err != nil {
			t.Fatalf("apply: %v", err)
		}
		if len(st.ensured) != 1 || st.ensured[0] != "Windows" || st.ensuredWindows != 0 {
			t.Errorf("ensured by name = %v, windows ensures = %d; want [Windows] and 0", st.ensured, st.ensuredWindows)
		}
	})

	t.Run("IMP-001/entradas inválidas são -32602", func(t *testing.T) {
		t.Parallel()
		st := newFakeImportStore()
		scan := importHandlers(st, fakeLocations{}, testClock(), importSourceSet(), nil, quietLog())[ipc.MethodImportScan]
		apply := importHandlers(st, fakeLocations{}, testClock(), nil, nil, quietLog())[ipc.MethodImportApply]
		cases := []struct {
			name string
			err  error
		}{
			{"fonte desconhecida", func() error {
				_, err := scan(context.Background(), rawParams(t, map[string]string{"source": "gog"}))
				return err
			}()},
			{"pasta relativa", func() error {
				_, err := scan(context.Background(), rawParams(t, map[string]string{"source": "file", "path": "relativa"}))
				return err
			}()},
			{"apply sem candidatos", func() error {
				_, err := apply(context.Background(), rawParams(t, ipc.ImportApplyParams{Candidates: []ipc.ImportCandidate{}}))
				return err
			}()},
		}
		for _, c := range cases {
			if !invalidParams(c.err) {
				t.Errorf("%s: err = %v, want -32602", c.name, c.err)
			}
		}
	})
}

// openImportRealStore opens a migrated store (with the seed platforms) in a temp dir.
func openImportRealStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "library.db"), store.OpenOptions{Migrate: true})
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func importPlatformByKey(t *testing.T, st *store.Store, key string) (store.Platform, bool) {
	t.Helper()
	ps, err := st.Platforms(t.Context())
	if err != nil {
		t.Fatalf("Platforms: %v", err)
	}
	for _, p := range ps {
		if p.SeedKey == key {
			return p, true
		}
	}
	return store.Platform{}, false
}

func importPlatformByName(t *testing.T, st *store.Store, name string) (store.Platform, bool) {
	t.Helper()
	ps, err := st.Platforms(t.Context())
	if err != nil {
		t.Fatalf("Platforms: %v", err)
	}
	for _, p := range ps {
		if p.Name == name {
			return p, true
		}
	}
	return store.Platform{}, false
}

// renameWindowsAndAddImpostor renames the platform of key windows to "PC" and creates
// another platform called "Windows" without a key. It returns both ids.
func renameWindowsAndAddImpostor(t *testing.T, st *store.Store) (win, impostor int64) {
	t.Helper()
	p, ok := importPlatformByKey(t, st, "windows")
	if !ok {
		t.Fatal("seed platform windows missing")
	}
	if _, err := st.UpdatePlatform(t.Context(), p.ID, "PC", domain.CategoryPC); err != nil {
		t.Fatalf("UpdatePlatform: %v", err)
	}
	res, err := st.DB().ExecContext(t.Context(), `INSERT INTO platform (name, category) VALUES ('Windows', 'pc')`)
	if err != nil {
		t.Fatalf("insert Windows: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	return p.ID, id
}

func importedGame(t *testing.T, st *store.Store, title string) store.Game {
	t.Helper()
	page, err := st.ListGames(t.Context(), store.Query{Filter: domain.FilterSpec{Version: domain.FilterVersion}, Sort: domain.SortTitle, Limit: 100})
	if err != nil {
		t.Fatalf("ListGames: %v", err)
	}
	for _, g := range page.Games {
		if g.Title == title {
			return g
		}
	}
	t.Fatalf("game %q not in the library: %+v", title, page.Games)
	return store.Game{}
}

func TestImportPlatformsRealStore(t *testing.T) {
	t.Parallel()
	steam := importers.Candidate{Source: importers.SourceSteam, Title: "Counter-Strike 2", Platform: importers.PlatformWindows,
		LaunchType: importers.LaunchURI, LaunchTarget: "steam://rungameid/730"}
	epic := importers.Candidate{Source: importers.SourceEpic, Title: "GTA V", Platform: importers.PlatformWindows,
		LaunchType: importers.LaunchURI, LaunchTarget: "com.epicgames.launcher://apps/x?action=launch&silent=true"}
	apply := func(t *testing.T, st *store.Store, cs ...importers.Candidate) ipc.ImportApplyResult {
		t.Helper()
		in := make([]ipc.ImportCandidate, 0, len(cs))
		for _, c := range cs {
			in = append(in, toIPCCandidate(c, false))
		}
		h := importHandlers(st, fakeLocations{}, testClock(), nil, nil, quietLog())[ipc.MethodImportApply]
		out, err := h(t.Context(), rawParams(t, ipc.ImportApplyParams{Candidates: in}))
		if err != nil {
			t.Fatalf("apply: %v", err)
		}
		return out.(ipc.ImportApplyResult)
	}

	t.Run("PLT-026/apply de Steam vai para a plataforma de chave windows mesmo existindo outra chamada Windows", func(t *testing.T) {
		t.Parallel()
		st := openImportRealStore(t)
		win, impostor := renameWindowsAndAddImpostor(t, st)
		if res := apply(t, st, steam); len(res.Applied) != 1 {
			t.Fatalf("result = %+v, want 1 applied", res)
		}
		g := importedGame(t, st, "Counter-Strike 2")
		if g.PlatformID != win {
			t.Errorf("platform id = %d, want %d (key windows, renamed PC), not %d", g.PlatformID, win, impostor)
		}
	})

	t.Run("PLT-026/apply de Epic recria Windows (pc, windows) quando a de chave windows foi excluída", func(t *testing.T) {
		t.Parallel()
		st := openImportRealStore(t)
		p, ok := importPlatformByKey(t, st, "windows")
		if !ok {
			t.Fatal("seed platform windows missing")
		}
		if _, err := st.DeletePlatform(t.Context(), p.ID); err != nil {
			t.Fatalf("DeletePlatform: %v", err)
		}
		if res := apply(t, st, epic); len(res.Applied) != 1 {
			t.Fatalf("result = %+v, want 1 applied", res)
		}
		np, ok := importPlatformByKey(t, st, "windows")
		if !ok || np.Name != "Windows" || np.Category != domain.CategoryPC {
			t.Fatalf("platform of key windows = %+v, %v; want Windows / pc", np, ok)
		}
		if g := importedGame(t, st, "GTA V"); g.PlatformID != np.ID {
			t.Errorf("platform id = %d, want %d", g.PlatformID, np.ID)
		}
	})

	t.Run("PLT-026/scan de Steam marca já existe pela plataforma de chave windows", func(t *testing.T) {
		t.Parallel()
		st := openImportRealStore(t)
		renameWindowsAndAddImpostor(t, st)
		// Same title on the platform of key windows, different launch target.
		if _, err := st.AddGame(t.Context(), store.NewGame{Title: "Counter-Strike 2", Exe: `C:\cs2.exe`, At: testClock().Now()}); err != nil {
			t.Fatalf("AddGame: %v", err)
		}
		before, _ := st.Platforms(t.Context())
		sources := map[string]importers.Source{
			ipc.ImportSourceSteam: fakeImportSource{id: "steam", res: importers.Result{Candidates: []importers.Candidate{steam}}},
		}
		h := importHandlers(st, fakeLocations{}, testClock(), sources, nil, quietLog())[ipc.MethodImportScan]
		out, err := h(t.Context(), rawParams(t, map[string]string{"source": "steam"}))
		if err != nil {
			t.Fatalf("scan: %v", err)
		}
		if res := out.(ipc.ImportScanResult); len(res.Candidates) != 1 || !res.Candidates[0].Exists {
			t.Errorf("candidates = %+v, want exists=true", res.Candidates)
		}
		if after, _ := st.Platforms(t.Context()); len(after) != len(before) {
			t.Errorf("scan changed the platforms: %d -> %d", len(before), len(after))
		}
	})

	t.Run("PLT-026/scan de Steam com a plataforma windows excluída não cria nada", func(t *testing.T) {
		t.Parallel()
		st := openImportRealStore(t)
		p, _ := importPlatformByKey(t, st, "windows")
		if _, err := st.DeletePlatform(t.Context(), p.ID); err != nil {
			t.Fatalf("DeletePlatform: %v", err)
		}
		sources := map[string]importers.Source{
			ipc.ImportSourceSteam: fakeImportSource{id: "steam", res: importers.Result{Candidates: []importers.Candidate{steam}}},
		}
		h := importHandlers(st, fakeLocations{}, testClock(), sources, nil, quietLog())[ipc.MethodImportScan]
		out, err := h(t.Context(), rawParams(t, map[string]string{"source": "steam"}))
		if err != nil {
			t.Fatalf("scan: %v", err)
		}
		if res := out.(ipc.ImportScanResult); len(res.Candidates) != 1 || res.Candidates[0].Exists {
			t.Errorf("candidates = %+v, want exists=false", res.Candidates)
		}
		if _, ok := importPlatformByKey(t, st, "windows"); ok {
			t.Errorf("scan recreated the platform of key windows")
		}
	})

	t.Run("PLT-028/apply do LaunchBox com plataforma Windows resolve pelo nome", func(t *testing.T) {
		t.Parallel()
		st := openImportRealStore(t)
		win, impostor := renameWindowsAndAddImpostor(t, st)
		lb := importers.Candidate{Source: importers.SourceLaunchBox, Title: "Starfield", Platform: importers.PlatformWindows,
			LaunchType: importers.LaunchExe, LaunchTarget: `C:\s.exe`}
		if res := apply(t, st, lb); len(res.Applied) != 1 {
			t.Fatalf("result = %+v, want 1 applied", res)
		}
		g := importedGame(t, st, "Starfield")
		if g.PlatformID != impostor {
			t.Errorf("platform id = %d, want %d (the one named Windows), not %d (key windows)", g.PlatformID, impostor, win)
		}
	})

	t.Run("PLT-026/PLT-028/um apply com Steam e LaunchBox Windows não mistura o cache, em qualquer ordem", func(t *testing.T) {
		t.Parallel()
		lb := importers.Candidate{Source: importers.SourceLaunchBox, Title: "Starfield", Platform: importers.PlatformWindows,
			LaunchType: importers.LaunchExe, LaunchTarget: `C:\s.exe`}
		orders := map[string][]importers.Candidate{
			"steam primeiro":     {steam, lb},
			"launchbox primeiro": {lb, steam},
		}
		for name, cs := range orders {
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				st := openImportRealStore(t)
				p, ok := importPlatformByKey(t, st, "windows")
				if !ok {
					t.Fatal("seed platform windows missing")
				}
				if _, err := st.UpdatePlatform(t.Context(), p.ID, "Meu PC", domain.CategoryPC); err != nil {
					t.Fatalf("UpdatePlatform: %v", err)
				}
				res, err := st.DB().ExecContext(t.Context(), `INSERT INTO platform (name, category) VALUES ('Windows', 'pc')`)
				if err != nil {
					t.Fatalf("insert Windows: %v", err)
				}
				impostor, _ := res.LastInsertId()
				if r := apply(t, st, cs...); len(r.Applied) != 2 {
					t.Fatalf("result = %+v, want 2 applied", r)
				}
				if g := importedGame(t, st, "Counter-Strike 2"); g.PlatformID != p.ID {
					t.Errorf("Steam platform id = %d, want %d (key windows)", g.PlatformID, p.ID)
				}
				if g := importedGame(t, st, "Starfield"); g.PlatformID != impostor {
					t.Errorf("LaunchBox platform id = %d, want %d (named Windows)", g.PlatformID, impostor)
				}
			})
		}
	})

	t.Run("PLT-029/apply do LaunchBox cria Nintendo 3DS excluída como handheld com a chave", func(t *testing.T) {
		t.Parallel()
		st := openImportRealStore(t)
		p, ok := importPlatformByKey(t, st, "3ds")
		if !ok {
			t.Fatal("seed platform 3ds missing")
		}
		if _, err := st.DeletePlatform(t.Context(), p.ID); err != nil {
			t.Fatalf("DeletePlatform: %v", err)
		}
		lb := importers.Candidate{Source: importers.SourceLaunchBox, Title: "Zelda", Platform: "Nintendo 3DS",
			LaunchType: importers.LaunchEmulator, LaunchTarget: `C:\roms\z.3ds`}
		if res := apply(t, st, lb); len(res.Applied) != 1 {
			t.Fatalf("result = %+v, want 1 applied", res)
		}
		np, ok := importPlatformByName(t, st, "Nintendo 3DS")
		if !ok || np.Category != domain.CategoryHandheld || np.SeedKey != "3ds" {
			t.Errorf("platform = %+v, %v; want Nintendo 3DS / handheld / key 3ds", np, ok)
		}
	})
}
