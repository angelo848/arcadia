package agent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
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

	resolvable   map[string]store.Platform // ResolvePlatform answers, by lower-case name (IMV-030)
	resolveCalls []string                  // names ResolvePlatform was asked about
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

// ResolvePlatform answers from the resolvable table, by lower-case name (PLT-025 steps 1 and 2).
func (f *fakeImportStore) ResolvePlatform(_ context.Context, name string) (store.Platform, bool, error) {
	f.resolveCalls = append(f.resolveCalls, name)
	p, ok := f.resolvable[strings.ToLower(name)]
	return p, ok, nil
}

// addPlatform makes p findable by ResolvePlatform under its name.
func (f *fakeImportStore) addPlatform(p store.Platform) {
	if f.resolvable == nil {
		f.resolvable = map[string]store.Platform{}
	}
	f.resolvable[strings.ToLower(p.Name)] = p
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

// spyImportSource records every ScanInput it receives and reads nothing (IMV-080).
type spyImportSource struct {
	id  string
	res importers.Result
	in  []importers.ScanInput
}

func (s *spyImportSource) ID() string { return s.id }

func (s *spyImportSource) Scan(_ context.Context, in importers.ScanInput) (importers.Result, error) {
	s.in = append(s.in, in)
	return s.res, nil
}

// lastInput returns the ScanInput of the last scan; the test fails when there was none.
func (s *spyImportSource) lastInput(t *testing.T) importers.ScanInput {
	t.Helper()
	if len(s.in) == 0 {
		t.Fatal("source was not scanned")
	}
	return s.in[len(s.in)-1]
}

func importBoolPtr(b bool) *bool { return &b }

// scanV2 runs import.scan over the given sources and returns the typed result or the error.
func scanV2(t *testing.T, st importStore, sources map[string]importers.Source, params any) (ipc.ImportScanResult, error) {
	t.Helper()
	h := importHandlers(st, fakeLocations{}, testClock(), sources, nil, quietLog())[ipc.MethodImportScan]
	out, err := h(t.Context(), rawParams(t, params))
	if err != nil {
		return ipc.ImportScanResult{}, err
	}
	return out.(ipc.ImportScanResult), nil
}

// importTableCounts counts the rows of the tables a scan must never touch (IMV-080).
func importTableCounts(t *testing.T, st *store.Store) map[string]int64 {
	t.Helper()
	counts := map[string]int64{}
	for _, table := range []string{"game", "play_session", "tracking_rule", "pause_interval"} {
		var n int64
		if err := st.DB().QueryRowContext(t.Context(), "SELECT COUNT(*) FROM "+table).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		counts[table] = n
	}
	return counts
}

// applyOneFileCandidate writes one candidate through import.apply with the real store.
func applyOneFileCandidate(t *testing.T, st *store.Store, c importers.Candidate, platform string) {
	t.Helper()
	c.Platform = platform // file candidates need the platform chosen at apply (IMP-004, PLT-026)
	h := importHandlers(st, fakeLocations{}, testClock(), nil, nil, quietLog())[ipc.MethodImportApply]
	out, err := h(t.Context(), rawParams(t, ipc.ImportApplyParams{Candidates: []ipc.ImportCandidate{toIPCCandidate(c, false)}}))
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if res := out.(ipc.ImportApplyResult); len(res.Applied) != 1 {
		t.Fatalf("apply result = %+v, want 1 applied", res)
	}
}

// launcherScanCase is one candidate of a launcher source (Steam, Epic, LaunchBox).
type launcherScanCase struct {
	source string
	cand   importers.Candidate
}

func launcherScanCases() []launcherScanCase {
	return []launcherScanCase{
		{ipc.ImportSourceSteam, importers.Candidate{Source: importers.SourceSteam, Title: "Counter-Strike 2",
			Platform: importers.PlatformWindows, LaunchType: importers.LaunchURI, LaunchTarget: "steam://rungameid/730"}},
		{ipc.ImportSourceEpic, importers.Candidate{Source: importers.SourceEpic, Title: "GTA V",
			Platform: importers.PlatformWindows, LaunchType: importers.LaunchURI,
			LaunchTarget: "com.epicgames.launcher://apps/x?action=launch&silent=true"}},
		{ipc.ImportSourceLaunchBox, importers.Candidate{Source: importers.SourceLaunchBox, Title: "Starfield",
			Platform: importers.PlatformWindows, LaunchType: importers.LaunchExe, LaunchTarget: `C:\s.exe`}},
	}
}

// TestImportScanV2Params covers the new import.scan parameters at the handler: validation
// before any read (IMV-071), the options that reach the source (IMV-011, IMV-017), and the
// candidate fields (IMV-024, IMV-007).
func TestImportScanV2Params(t *testing.T) {
	t.Parallel()

	t.Run("IMV-071/titleFrom diferente de file e folder: -32602 sem ler nada", func(t *testing.T) {
		t.Parallel()
		for _, tf := range []string{"pasta", "Folder", "nome"} {
			spy := &spyImportSource{id: "folder"}
			_, err := scanV2(t, newFakeImportStore(), map[string]importers.Source{ipc.ImportSourceFolder: spy},
				ipc.ImportScanParams{Source: ipc.ImportSourceFolder, Path: t.TempDir(), TitleFrom: tf})
			if !invalidParams(err) {
				t.Errorf("titleFrom %q: err = %v, want -32602", tf, err)
			}
			if len(spy.in) != 0 {
				t.Errorf("titleFrom %q: source was scanned %d times, want 0", tf, len(spy.in))
			}
		}
	})

	t.Run("IMV-071/platform com 101 caracteres: -32602 sem ler nada", func(t *testing.T) {
		t.Parallel()
		spy := &spyImportSource{id: "file"}
		_, err := scanV2(t, newFakeImportStore(), map[string]importers.Source{ipc.ImportSourceFile: spy},
			ipc.ImportScanParams{Source: ipc.ImportSourceFile, Path: filepath.Join(t.TempDir(), "jogo.bin"),
				Platform: strings.Repeat("a", 101)})
		if !invalidParams(err) {
			t.Errorf("err = %v, want -32602", err)
		}
		if len(spy.in) != 0 {
			t.Errorf("source was scanned %d times, want 0", len(spy.in))
		}
	})

	t.Run("IMV-071/platform com 100 runas acentuadas é aceito (conta runas, não bytes)", func(t *testing.T) {
		t.Parallel()
		spy := &spyImportSource{id: "file"}
		_, err := scanV2(t, newFakeImportStore(), map[string]importers.Source{ipc.ImportSourceFile: spy},
			ipc.ImportScanParams{Source: ipc.ImportSourceFile, Path: filepath.Join(t.TempDir(), "jogo.bin"),
				Platform: strings.Repeat("á", 100)})
		if err != nil {
			t.Fatalf("err = %v, want accepted", err)
		}
		if len(spy.in) != 1 {
			t.Errorf("source scanned %d times, want 1", len(spy.in))
		}
	})

	t.Run("IMV-071/titleFrom e recursive com Steam, Epic e LaunchBox são aceitos e ignorados", func(t *testing.T) {
		t.Parallel()
		for _, c := range launcherScanCases() {
			spy := &spyImportSource{id: c.source, res: importers.Result{Candidates: []importers.Candidate{c.cand}}}
			st := newFakeImportStore()
			res, err := scanV2(t, st, map[string]importers.Source{c.source: spy}, ipc.ImportScanParams{
				Source: c.source, Path: t.TempDir(), TitleFrom: "folder", Recursive: importBoolPtr(false)})
			if err != nil {
				t.Fatalf("%s: scan: %v", c.source, err)
			}
			if len(res.Candidates) != 1 || res.Candidates[0].Title != c.cand.Title {
				t.Errorf("%s: candidates = %+v, want %q", c.source, res.Candidates, c.cand.Title)
			}
		}
	})

	t.Run("IMV-071/platform com Steam, Epic e LaunchBox é aceito e ignorado", func(t *testing.T) {
		t.Parallel()
		for _, c := range launcherScanCases() {
			spy := &spyImportSource{id: c.source, res: importers.Result{Candidates: []importers.Candidate{c.cand}}}
			st := newFakeImportStore()
			res, err := scanV2(t, st, map[string]importers.Source{c.source: spy}, ipc.ImportScanParams{
				Source: c.source, Path: t.TempDir(), Platform: "Nome que não existe"})
			if err != nil {
				t.Fatalf("%s: scan: %v", c.source, err)
			}
			if len(res.Candidates) != 1 {
				t.Errorf("%s: candidates = %d, want 1", c.source, len(res.Candidates))
			}
			if len(st.resolveCalls) != 0 {
				t.Errorf("%s: platform resolved %v, want never (launcher sources ignore it)", c.source, st.resolveCalls)
			}
		}
	})

	t.Run("IMV-071/parâmetros ausentes: título pelo arquivo, recursivo, sem plataforma", func(t *testing.T) {
		t.Parallel()
		spy := &spyImportSource{id: "folder"}
		_, err := scanV2(t, newFakeImportStore(), map[string]importers.Source{ipc.ImportSourceFolder: spy},
			ipc.ImportScanParams{Source: ipc.ImportSourceFolder, Path: t.TempDir()})
		if err != nil {
			t.Fatalf("scan: %v", err)
		}
		opts := spy.lastInput(t).Folder
		if opts.TitleFrom != importers.TitleFromFile || opts.FirstLevelOnly || opts.Kind != importers.KindAny {
			t.Errorf("folder options = %+v, want title from file, recursive, KindAny", opts)
		}
	})

	t.Run("IMV-011/recursive=false chega à fonte como FirstLevelOnly", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name string
			rec  *bool
			want bool
		}{
			{"recursive false", importBoolPtr(false), true},
			{"recursive true", importBoolPtr(true), false},
			{"recursive ausente", nil, false},
		}
		for _, c := range cases {
			spy := &spyImportSource{id: "folder"}
			_, err := scanV2(t, newFakeImportStore(), map[string]importers.Source{ipc.ImportSourceFolder: spy},
				ipc.ImportScanParams{Source: ipc.ImportSourceFolder, Path: t.TempDir(), Recursive: c.rec})
			if err != nil {
				t.Fatalf("%s: scan: %v", c.name, err)
			}
			if got := spy.lastInput(t).Folder.FirstLevelOnly; got != c.want {
				t.Errorf("%s: FirstLevelOnly = %v, want %v", c.name, got, c.want)
			}
		}
	})

	t.Run("IMV-017/titleFrom=folder chega à fonte", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name string
			tf   string
			want importers.TitleFrom
		}{
			{"folder", "folder", importers.TitleFromFolder},
			{"file", "file", importers.TitleFromFile},
			{"vazio", "", importers.TitleFromFile},
		}
		for _, c := range cases {
			spy := &spyImportSource{id: "folder"}
			_, err := scanV2(t, newFakeImportStore(), map[string]importers.Source{ipc.ImportSourceFolder: spy},
				ipc.ImportScanParams{Source: ipc.ImportSourceFolder, Path: t.TempDir(), TitleFrom: c.tf})
			if err != nil {
				t.Fatalf("%s: scan: %v", c.name, err)
			}
			if got := spy.lastInput(t).Folder.TitleFrom; got != c.want {
				t.Errorf("%s: TitleFrom = %q, want %q", c.name, got, c.want)
			}
		}
	})

	t.Run("IMV-024/campos novos aparecem no resultado do scan de pasta", func(t *testing.T) {
		t.Parallel()
		ff := importers.Candidate{Source: importers.SourceFolder, Title: "Final Fantasy VIII", LaunchType: importers.LaunchEmulator,
			LaunchTarget: `E:\ps1\04. Final Fantasy VIII\FF8 (Disc 1).cue`, InstallDir: `E:\ps1\04. Final Fantasy VIII`,
			File: `04. Final Fantasy VIII\FF8 (Disc 1).cue`, Extension: "cue", Discs: 2,
			DiscFiles: []string{`04. Final Fantasy VIII\FF8 (Disc 1).cue`, `04. Final Fantasy VIII\FF8 (Disc 2).cue`}}
		spy := &spyImportSource{id: "folder", res: importers.Result{Candidates: []importers.Candidate{ff}}}
		res, err := scanV2(t, newFakeImportStore(), map[string]importers.Source{ipc.ImportSourceFolder: spy},
			ipc.ImportScanParams{Source: ipc.ImportSourceFolder, Path: t.TempDir()})
		if err != nil {
			t.Fatalf("scan: %v", err)
		}
		if len(res.Candidates) != 1 {
			t.Fatalf("candidates = %d, want 1", len(res.Candidates))
		}
		c := res.Candidates[0]
		if c.File != ff.File || c.Extension != "cue" || c.Discs != 2 || len(c.DiscFiles) != 2 ||
			c.DiscFiles[0] != ff.DiscFiles[0] || c.DiscFiles[1] != ff.DiscFiles[1] {
			t.Errorf("candidate = %+v, want file, extension cue, discs 2 and discFiles", c)
		}
	})

	t.Run("IMV-024/campos novos não aparecem no JSON de Steam, Epic e LaunchBox", func(t *testing.T) {
		t.Parallel()
		for _, lc := range launcherScanCases() {
			spy := &spyImportSource{id: lc.source, res: importers.Result{Candidates: []importers.Candidate{lc.cand}}}
			res, err := scanV2(t, newFakeImportStore(), map[string]importers.Source{lc.source: spy},
				ipc.ImportScanParams{Source: lc.source, Path: t.TempDir()})
			if err != nil {
				t.Fatalf("%s: scan: %v", lc.source, err)
			}
			raw, err := json.Marshal(res.Candidates[0])
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(raw, &fields); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			for _, k := range []string{"file", "extension", "discs", "discFiles"} {
				if _, ok := fields[k]; ok {
					t.Errorf("%s: candidate JSON has %q, want it omitted", lc.source, k)
				}
			}
		}
	})

	t.Run("IMV-007/título de Steam, Epic e LaunchBox sai sem normalização", func(t *testing.T) {
		t.Parallel()
		const raw = "Jogo (USA) [!] 2. Edição_Especial"
		for _, lc := range launcherScanCases() {
			cand := lc.cand
			cand.Title = raw
			spy := &spyImportSource{id: lc.source, res: importers.Result{Candidates: []importers.Candidate{cand}}}
			res, err := scanV2(t, newFakeImportStore(), map[string]importers.Source{lc.source: spy},
				ipc.ImportScanParams{Source: lc.source, Path: t.TempDir(), TitleFrom: "folder"})
			if err != nil {
				t.Fatalf("%s: scan: %v", lc.source, err)
			}
			if len(res.Candidates) != 1 || res.Candidates[0].Title != raw {
				t.Errorf("%s: title = %+v, want %q unchanged", lc.source, res.Candidates, raw)
			}
		}
	})
}

// TestImportScanOptions covers scanOptions: the platform lookup without writes (IMV-030, IMV-013),
// the folder options derived from the parameters (IMV-011, IMV-017), and the platform rename (IMV-063).
func TestImportScanOptions(t *testing.T) {
	t.Parallel()

	platformCases := []struct {
		name     string
		seed     []store.Platform
		platform string
		kind     importers.FolderKind
		wantID   int64
	}{
		{"IMV-030/Windows renomeada (chave windows) escolhe a classe de executáveis",
			[]store.Platform{{ID: 7, Name: "PC", SeedKey: domain.SeedKeyWindows, Category: domain.CategoryPC}},
			"PC", importers.KindExecutables, 7},
		{"IMV-030/plataforma de console do seed escolhe a classe de console",
			[]store.Platform{{ID: 8, Name: "Sony Playstation 2", SeedKey: "ps2", Category: domain.CategoryConsole}},
			"Sony Playstation 2", importers.KindConsole, 8},
		{"IMV-030/plataforma criada pelo usuário (sem chave) lê as duas listas",
			[]store.Platform{{ID: 9, Name: "Arcade Meu", Category: domain.CategoryConsole}},
			"Arcade Meu", importers.KindAny, 9},
		{"IMV-030/nome inexistente: classe qualquer e id 0",
			nil, "Nao existe", importers.KindAny, 0},
		{"IMV-030/nome vazio: classe qualquer e id 0",
			nil, "", importers.KindAny, 0},
	}
	for _, c := range platformCases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			st := newFakeImportStore()
			for _, p := range c.seed {
				st.addPlatform(p)
			}
			opts, id, err := scanOptions(t.Context(), st, ipc.ImportScanParams{Source: ipc.ImportSourceFolder, Platform: c.platform})
			if err != nil {
				t.Fatalf("scanOptions: %v", err)
			}
			if id != c.wantID || opts.Kind != c.kind {
				t.Errorf("id = %d, kind = %v; want id %d, kind %v", id, opts.Kind, c.wantID, c.kind)
			}
		})
	}

	t.Run("IMV-011/recursive nil ou true lê subpastas; false vira FirstLevelOnly", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name string
			rec  *bool
			want bool
		}{
			{"ausente", nil, false},
			{"true", importBoolPtr(true), false},
			{"false", importBoolPtr(false), true},
		}
		for _, c := range cases {
			opts, _, err := scanOptions(t.Context(), newFakeImportStore(), ipc.ImportScanParams{Source: ipc.ImportSourceFolder, Recursive: c.rec})
			if err != nil {
				t.Fatalf("%s: %v", c.name, err)
			}
			if opts.FirstLevelOnly != c.want {
				t.Errorf("%s: FirstLevelOnly = %v, want %v", c.name, opts.FirstLevelOnly, c.want)
			}
		}
	})

	t.Run("IMV-017/titleFrom folder vira TitleFromFolder; ausente ou file vira TitleFromFile", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name string
			tf   string
			want importers.TitleFrom
		}{
			{"folder", "folder", importers.TitleFromFolder},
			{"file", "file", importers.TitleFromFile},
			{"ausente", "", importers.TitleFromFile},
		}
		for _, c := range cases {
			opts, _, err := scanOptions(t.Context(), newFakeImportStore(), ipc.ImportScanParams{Source: ipc.ImportSourceFolder, TitleFrom: c.tf})
			if err != nil {
				t.Fatalf("%s: %v", c.name, err)
			}
			if opts.TitleFrom != c.want {
				t.Errorf("%s: TitleFrom = %q, want %q", c.name, opts.TitleFrom, c.want)
			}
		}
	})
}

// TestImportCandidateExistsScope covers the platform used by the "already exists" check (IMV-030, IMV-063).
func TestImportCandidateExistsScope(t *testing.T) {
	t.Parallel()

	t.Run("IMV-030/candidato de pasta sem plataforma confere na plataforma do scan", func(t *testing.T) {
		t.Parallel()
		st := newFakeImportStore()
		c := importers.Candidate{Source: importers.SourceFolder, Title: "Jogo", LaunchType: importers.LaunchExe, LaunchTarget: `C:\jogo.exe`}
		if _, err := candidateExists(t.Context(), st, c, 42); err != nil {
			t.Fatalf("candidateExists: %v", err)
		}
		if len(st.dupPlatformIDs) != 1 || st.dupPlatformIDs[0] != 42 {
			t.Errorf("ImportDuplicate platform ids = %v, want [42] (the platform of the scan)", st.dupPlatformIDs)
		}
	})

	t.Run("IMV-030/Steam continua na plataforma de chave windows, não na do scan", func(t *testing.T) {
		t.Parallel()
		st := newFakeImportStore()
		st.windowsID = 7
		steam := importers.Candidate{Source: importers.SourceSteam, Title: "Counter-Strike 2", Platform: importers.PlatformWindows,
			LaunchType: importers.LaunchURI, LaunchTarget: "steam://rungameid/730"}
		if _, err := candidateExists(t.Context(), st, steam, 42); err != nil {
			t.Fatalf("candidateExists: %v", err)
		}
		if len(st.dupPlatformIDs) != 1 || st.dupPlatformIDs[0] != 7 {
			t.Errorf("ImportDuplicate platform ids = %v, want [7] (key windows)", st.dupPlatformIDs)
		}
	})
}

// TestImportScanRealStore runs the scan against the real store (IMV-030, IMV-063, IMV-080).
func TestImportScanRealStore(t *testing.T) {
	t.Parallel()

	jogo := importers.Candidate{Source: importers.SourceFile, Title: "Jogo X", LaunchType: importers.LaunchEmulator,
		LaunchTarget: `E:\roms\jogo-x.bin`, InstallDir: `E:\roms`}
	copia := importers.Candidate{Source: importers.SourceFile, Title: "Jogo X", LaunchType: importers.LaunchEmulator,
		LaunchTarget: `E:\roms\outra-copia.bin`, InstallDir: `E:\roms`}

	t.Run("IMV-030/título duplicado na plataforma certa é exists", func(t *testing.T) {
		t.Parallel()
		st := openImportRealStore(t)
		applyOneFileCandidate(t, st, jogo, "Sony Playstation") // Jogo X on Sony Playstation, target jogo-x.bin
		spy := &spyImportSource{id: "file", res: importers.Result{Candidates: []importers.Candidate{copia}}}
		res, err := scanV2(t, st, map[string]importers.Source{ipc.ImportSourceFile: spy},
			ipc.ImportScanParams{Source: ipc.ImportSourceFile, Path: filepath.Join(t.TempDir(), "outra-copia.bin"),
				Platform: "Sony Playstation"})
		if err != nil {
			t.Fatalf("scan: %v", err)
		}
		if len(res.Candidates) != 1 || !res.Candidates[0].Exists {
			t.Errorf("candidates = %+v, want exists=true (same title on the same platform)", res.Candidates)
		}
	})

	t.Run("IMV-030/título duplicado em outra plataforma não é exists", func(t *testing.T) {
		t.Parallel()
		st := openImportRealStore(t)
		applyOneFileCandidate(t, st, jogo, "Sony Playstation")
		spy := &spyImportSource{id: "file", res: importers.Result{Candidates: []importers.Candidate{copia}}}
		res, err := scanV2(t, st, map[string]importers.Source{ipc.ImportSourceFile: spy},
			ipc.ImportScanParams{Source: ipc.ImportSourceFile, Path: filepath.Join(t.TempDir(), "outra-copia.bin"),
				Platform: "Nintendo 64"})
		if err != nil {
			t.Fatalf("scan: %v", err)
		}
		if len(res.Candidates) != 1 || res.Candidates[0].Exists {
			t.Errorf("candidates = %+v, want exists=false (same title on another platform)", res.Candidates)
		}
	})

	t.Run("IMV-030/o scan não cria plataforma, nem para nome inexistente nem para seed excluído", func(t *testing.T) {
		t.Parallel()
		st := openImportRealStore(t)
		n64, ok := importPlatformByKey(t, st, "n64")
		if !ok {
			t.Fatal("seed platform n64 missing")
		}
		if _, err := st.DeletePlatform(t.Context(), n64.ID); err != nil {
			t.Fatalf("DeletePlatform: %v", err)
		}
		before, err := st.Platforms(t.Context())
		if err != nil {
			t.Fatalf("Platforms: %v", err)
		}
		spy := &spyImportSource{id: "file", res: importers.Result{Candidates: []importers.Candidate{jogo}}}
		for _, name := range []string{"Plataforma Inventada", "Nintendo 64"} {
			if _, err := scanV2(t, st, map[string]importers.Source{ipc.ImportSourceFile: spy},
				ipc.ImportScanParams{Source: ipc.ImportSourceFile, Path: filepath.Join(t.TempDir(), "jogo.bin"), Platform: name}); err != nil {
				t.Fatalf("scan with %q: %v", name, err)
			}
		}
		after, err := st.Platforms(t.Context())
		if err != nil {
			t.Fatalf("Platforms: %v", err)
		}
		if len(after) != len(before) {
			t.Errorf("platforms %d -> %d, want no change", len(before), len(after))
		}
		if _, ok := importPlatformByKey(t, st, "n64"); ok {
			t.Errorf("scan recreated the platform of seed key n64")
		}
	})

	t.Run("IMV-063/plataforma renomeada é resolvida sem diferenciar maiúsculas", func(t *testing.T) {
		t.Parallel()
		st := openImportRealStore(t)
		win, _ := renameWindowsAndAddImpostor(t, st) // key windows is now named "PC"
		opts, id, err := scanOptions(t.Context(), st, ipc.ImportScanParams{Source: ipc.ImportSourceFolder, Platform: "pc"})
		if err != nil {
			t.Fatalf("scanOptions: %v", err)
		}
		if id != win || opts.Kind != importers.KindExecutables {
			t.Errorf("id = %d, kind = %v; want %d and KindExecutables (key windows)", id, opts.Kind, win)
		}
	})

	t.Run("IMV-080/scan com store real não cria jogo, sessão nem regra", func(t *testing.T) {
		t.Parallel()
		st := openImportRealStore(t)
		before := importTableCounts(t, st)
		spy := &spyImportSource{id: "folder", res: importers.Result{Candidates: []importers.Candidate{jogo, copia}}}
		if _, err := scanV2(t, st, map[string]importers.Source{ipc.ImportSourceFolder: spy},
			ipc.ImportScanParams{Source: ipc.ImportSourceFolder, Path: t.TempDir(), Platform: "Sony Playstation"}); err != nil {
			t.Fatalf("scan: %v", err)
		}
		after := importTableCounts(t, st)
		for table, n := range before {
			if after[table] != n {
				t.Errorf("%s rows %d -> %d, want no change", table, n, after[table])
			}
		}
	})
}
