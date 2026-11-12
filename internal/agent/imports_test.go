package agent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
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
}

func newFakeImportStore() *fakeImportStore {
	return &fakeImportStore{platforms: map[string]int64{}, targets: map[string]bool{}}
}

func (f *fakeImportStore) PlatformIDByName(_ context.Context, name string) (int64, bool, error) {
	id, ok := f.platforms[name]
	return id, ok, nil
}

func (f *fakeImportStore) ImportDuplicate(_ context.Context, launchType, target, _ string, _ int64) (bool, error) {
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
		Source: importers.SourceSteam, Title: "Counter-Strike 2", Platform: importers.PlatformPC,
		LaunchType: importers.LaunchURI, LaunchTarget: "steam://rungameid/730",
	}
	folderCandidate := importers.Candidate{
		Source: importers.SourceFolder, Title: "ABZU", LaunchType: importers.LaunchExe,
		LaunchTarget: `D:\ABZU\ABZU.exe`, InstallDir: `D:\ABZU`,
	}

	t.Run("IMP-001/scan não grava nada", func(t *testing.T) {
		t.Parallel()
		st := newFakeImportStore()
		sources := map[string]importers.Source{
			ipc.ImportSourceSteam: fakeImportSource{id: "steam", res: importers.Result{Candidates: []importers.Candidate{steamCandidate}}},
		}
		h := importHandlers(st, fakeLocations{}, testClock(), sources, quietLog())[ipc.MethodImportScan]
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
		h := importHandlers(st, fakeLocations{}, testClock(), nil, quietLog())[ipc.MethodImportApply]
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
		h := importHandlers(st, fakeLocations{}, testClock(), sources, quietLog())[ipc.MethodImportScan]
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
			ipc.ImportSourceSteam:  fakeImportSource{id: "steam", err: errors.New("boom")},
			ipc.ImportSourceFolder: fakeImportSource{id: "folder", res: importers.Result{Candidates: []importers.Candidate{folderCandidate}}},
		}
		h := importHandlers(st, fakeLocations{}, testClock(), sources, quietLog())[ipc.MethodImportScan]
		out, err := h(context.Background(), rawParams(t, map[string]string{"source": "steam"}))
		if err != nil {
			t.Fatalf("scan of a failing source must not be an error: %v", err)
		}
		if res := out.(ipc.ImportScanResult); len(res.Warnings) == 0 {
			t.Errorf("warnings empty, want the source failure")
		}
		out, err = h(context.Background(), rawParams(t, map[string]string{"source": "folder", "path": `D:\ABZU`}))
		if err != nil {
			t.Fatalf("folder scan: %v", err)
		}
		if res := out.(ipc.ImportScanResult); len(res.Candidates) != 1 {
			t.Errorf("folder candidates = %d, want 1", len(res.Candidates))
		}
	})

	t.Run("IMP-008/um candidato que falha não impede os demais", func(t *testing.T) {
		t.Parallel()
		st := newFakeImportStore()
		st.failTitle = "Quebra"
		h := importHandlers(st, fakeLocations{}, testClock(), nil, quietLog())[ipc.MethodImportApply]
		bad := importers.Candidate{Source: importers.SourceFolder, Title: "Quebra", Platform: "PC", LaunchType: importers.LaunchExe, LaunchTarget: `C:\q.exe`}
		good := importers.Candidate{Source: importers.SourceFolder, Title: "Bom", Platform: "PC", LaunchType: importers.LaunchExe, LaunchTarget: `C:\b.exe`}
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
		h := importHandlers(st, fakeLocations{}, testClock(), nil, quietLog())[ipc.MethodImportApply]
		out, err := h(context.Background(), rawParams(t, ipc.ImportApplyParams{Candidates: []ipc.ImportCandidate{toIPCCandidate(steamCandidate, false)}}))
		if err != nil {
			t.Fatalf("apply: %v", err)
		}
		res := out.(ipc.ImportApplyResult)
		if len(res.Applied) != 0 || len(res.Skipped) != 1 || res.Skipped[0].Reason != "duplicate" {
			t.Errorf("result = %+v, want one duplicate", res)
		}
	})

	t.Run("IMP-001/entradas inválidas são -32602", func(t *testing.T) {
		t.Parallel()
		st := newFakeImportStore()
		scan := importHandlers(st, fakeLocations{}, testClock(), importSourceSet(), quietLog())[ipc.MethodImportScan]
		apply := importHandlers(st, fakeLocations{}, testClock(), nil, quietLog())[ipc.MethodImportApply]
		cases := []struct {
			name string
			err  error
		}{
			{"fonte desconhecida", func() error {
				_, err := scan(context.Background(), rawParams(t, map[string]string{"source": "gog"}))
				return err
			}()},
			{"pasta relativa", func() error {
				_, err := scan(context.Background(), rawParams(t, map[string]string{"source": "folder", "path": "relativa"}))
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
