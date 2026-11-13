package agent

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

// statsFixture is a store with two games and sessions, and the handlers over it.
func statsFixture(t *testing.T) (map[string]ipc.Handler, *store.Store, time.Time) {
	t.Helper()
	st, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "library.db"), store.OpenOptions{Migrate: true})
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	now := time.Date(2026, 11, 13, 15, 0, 0, 0, time.FixedZone("UTC-3", -3*3600))
	celeste, err := st.AddGame(t.Context(), store.NewGame{Title: "Celeste", Exe: `C:\c.exe`, At: now})
	if err != nil {
		t.Fatalf("AddGame: %v", err)
	}
	hades, err := st.AddGame(t.Context(), store.NewGame{Title: "Hades", Exe: `C:\h.exe`, At: now})
	if err != nil {
		t.Fatalf("AddGame: %v", err)
	}
	add := func(game int64, when time.Time, seconds int64, source string) {
		ms := when.UnixMilli()
		if _, err := st.DB().ExecContext(t.Context(),
			`INSERT INTO play_session (game_id, started_at, ended_at, last_seen_at, active_seconds, state, source)
			 VALUES (?, ?, ?, ?, ?, 'ended', ?)`, game, ms, ms+seconds*1000, ms+seconds*1000, seconds, source); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	add(celeste.ID, now.AddDate(0, 0, -1), 600, "tracked")
	add(hades.ID, now.AddDate(0, 0, -2), 1200, "tracked")
	add(celeste.ID, now.AddDate(0, 0, -200), 3600, "import")
	clock := domain.NewFakeClock(now)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return statsHandlers(st, clock, now.Location(), log), st, now
}

func callStats(t *testing.T, h map[string]ipc.Handler, method, params string) (any, error) {
	t.Helper()
	return h[method](context.Background(), json.RawMessage(params))
}

func TestStatsHandlers(t *testing.T) {
	t.Parallel()

	t.Run("EST-004/resumo dos últimos 7 dias soma jogos e plataformas sem a importação antiga", func(t *testing.T) {
		t.Parallel()
		h, _, _ := statsFixture(t)
		out, err := callStats(t, h, ipc.MethodStatsSummary, `{"period":"7d"}`)
		if err != nil {
			t.Fatalf("summary: %v", err)
		}
		res := out.(ipc.StatsSummaryResult)
		if res.TotalSeconds != 1800 || res.ImportedSeconds != 0 || len(res.Games) != 2 || res.Games[0].Title != "Hades" {
			t.Errorf("summary = %+v, want 1800 s, Hades first", res)
		}
		if len(res.Platforms) != 1 || res.Platforms[0].ActiveSeconds != 1800 {
			t.Errorf("platforms = %+v", res.Platforms)
		}
	})

	t.Run("EST-005/tudo inclui a importação e a marca à parte", func(t *testing.T) {
		t.Parallel()
		h, _, _ := statsFixture(t)
		out, err := callStats(t, h, ipc.MethodStatsSummary, `{"period":"all"}`)
		if err != nil {
			t.Fatalf("summary: %v", err)
		}
		res := out.(ipc.StatsSummaryResult)
		if res.TotalSeconds != 5400 || res.ImportedSeconds != 3600 {
			t.Errorf("total %d imported %d, want 5400 and 3600", res.TotalSeconds, res.ImportedSeconds)
		}
	})

	t.Run("EST-004/período desconhecido é -32602", func(t *testing.T) {
		t.Parallel()
		h, _, _ := statsFixture(t)
		_, err := callStats(t, h, ipc.MethodStatsSummary, `{"period":"2d"}`)
		if codeOf(err) != ipc.CodeInvalidParams {
			t.Errorf("code = %d (%v), want -32602", codeOf(err), err)
		}
	})

	t.Run("EST-002/semanal devolve as semanas pedidas com a atual marcada", func(t *testing.T) {
		t.Parallel()
		h, _, _ := statsFixture(t)
		out, err := callStats(t, h, ipc.MethodStatsWeekly, `{"weeks":12}`)
		if err != nil {
			t.Fatalf("weekly: %v", err)
		}
		weeks := out.(ipc.StatsWeeklyResult).Weeks
		if len(weeks) != 12 || !weeks[11].Current || weeks[11].Start != "2026-11-09" {
			t.Fatalf("weeks = %+v, want 12 with the current 2026-11-09", weeks)
		}
		var total int64
		for _, w := range weeks {
			total += w.Seconds
		}
		if total != 1800 {
			t.Errorf("total in 12 weeks = %d, want 1800 (the import is older)", total)
		}
	})

	t.Run("EST-006/a estatística só lê: a interface da loja tem só métodos de leitura", func(t *testing.T) {
		t.Parallel()
		typ := reflect.TypeOf((*statsStore)(nil)).Elem()
		for i := 0; i < typ.NumMethod(); i++ {
			name := typ.Method(i).Name
			if !strings.HasPrefix(name, "Stats") && name != "SessionTimes" {
				t.Errorf("statsStore has method %s, which may write", name)
			}
		}
	})
}
