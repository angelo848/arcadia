package store_test

// Tests of store.GetTimeToBeat / UpsertTimeToBeat (phase 15, TTB-002).

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/store"
)

func i64p(v int64) *int64 { return &v }

func ttbRow(gameID int64) store.TimeToBeatRow {
	return store.TimeToBeatRow{
		GameID:         gameID,
		Source:         "hltb",
		State:          "found",
		ExternalID:     "7231",
		ExternalTitle:  "Hollow Knight",
		MainS:          i64p(97200),
		MainExtraS:     i64p(147600),
		CompletionistS: i64p(244800),
		TitleAtFetch:   "Hollow Knight (GOG)",
		Term:           "Hollow Knight",
		CandidatesJSON: "[]",
		Confirmed:      true,
		FetchedAt:      time.UnixMilli(1789257600123).UTC(),
		ExpiresAt:      time.UnixMilli(1791849600456).UTC(),
	}
}

func TestTimeToBeat(t *testing.T) {
	t.Parallel()

	t.Run("TTB-002/found do HowLongToBeat grava fonte, estado, id, título externo, três valores, título e termo da busca, instantes em ms UTC e confirmed", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		g := m10Game(t, s.DB(), "Hollow Knight (GOG)")
		want := ttbRow(g)
		if err := s.UpsertTimeToBeat(t.Context(), want); err != nil {
			t.Fatalf("UpsertTimeToBeat: %v", err)
		}
		got, ok, err := s.GetTimeToBeat(t.Context(), g)
		if err != nil || !ok {
			t.Fatalf("GetTimeToBeat ok=%v err=%v", ok, err)
		}
		if got.GameID != g || got.Source != "hltb" || got.State != "found" || got.ExternalID != "7231" ||
			got.ExternalTitle != "Hollow Knight" || got.TitleAtFetch != "Hollow Knight (GOG)" ||
			got.Term != "Hollow Knight" || !got.Confirmed {
			t.Errorf("row = %+v, want %+v", got, want)
		}
		if got.MainS == nil || *got.MainS != 97200 || got.MainExtraS == nil || *got.MainExtraS != 147600 ||
			got.CompletionistS == nil || *got.CompletionistS != 244800 {
			t.Errorf("values = %v %v %v", got.MainS, got.MainExtraS, got.CompletionistS)
		}
		if !got.FetchedAt.Equal(want.FetchedAt) || !got.ExpiresAt.Equal(want.ExpiresAt) {
			t.Errorf("instants = %v %v, want %v %v", got.FetchedAt, got.ExpiresAt, want.FetchedAt, want.ExpiresAt)
		}
		var rawFetched, rawExpires int64
		if err := s.DB().QueryRowContext(t.Context(),
			`SELECT fetched_at, expires_at FROM time_to_beat WHERE game_id = ?`, g).Scan(&rawFetched, &rawExpires); err != nil {
			t.Fatalf("raw read: %v", err)
		}
		if rawFetched != 1789257600123 || rawExpires != 1791849600456 {
			t.Errorf("stored instants = %d %d, want Unix ms 1789257600123 1791849600456", rawFetched, rawExpires)
		}
	})

	t.Run("TTB-002/found do IGDB grava source igdb com id e nome do IGDB", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		g := m10Game(t, s.DB(), "Jogo")
		row := ttbRow(g)
		row.Source, row.ExternalID, row.ExternalTitle, row.Confirmed = "igdb", "1942", "The Witcher 3", false
		if err := s.UpsertTimeToBeat(t.Context(), row); err != nil {
			t.Fatalf("UpsertTimeToBeat: %v", err)
		}
		got, ok, err := s.GetTimeToBeat(t.Context(), g)
		if err != nil || !ok {
			t.Fatalf("GetTimeToBeat ok=%v err=%v", ok, err)
		}
		if got.Source != "igdb" || got.ExternalID != "1942" || got.ExternalTitle != "The Witcher 3" || got.Confirmed {
			t.Errorf("row = %+v", got)
		}
		// igdb with doubt violates the CHECK.
		bad := ttbRow(g)
		bad.Source, bad.State, bad.Confirmed = "igdb", "doubt", false
		if err := s.UpsertTimeToBeat(t.Context(), bad); err == nil {
			t.Errorf("igdb + doubt accepted")
		}
	})

	t.Run("TTB-002/doubt grava os candidatos (JSON, até 10) e nenhum valor", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		g := m10Game(t, s.DB(), "Jogo")
		type cand struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		}
		var cands []cand
		for i := 0; i < 10; i++ {
			cands = append(cands, cand{ID: string(rune('a' + i)), Title: "Titulo"})
		}
		raw, err := json.Marshal(cands)
		if err != nil {
			t.Fatal(err)
		}
		row := ttbRow(g)
		row.State, row.ExternalID, row.ExternalTitle = "doubt", "", ""
		row.MainS, row.MainExtraS, row.CompletionistS = nil, nil, nil
		row.CandidatesJSON, row.Confirmed = string(raw), false
		if err := s.UpsertTimeToBeat(t.Context(), row); err != nil {
			t.Fatalf("UpsertTimeToBeat: %v", err)
		}
		got, ok, err := s.GetTimeToBeat(t.Context(), g)
		if err != nil || !ok {
			t.Fatalf("GetTimeToBeat ok=%v err=%v", ok, err)
		}
		if got.State != "doubt" || got.MainS != nil || got.MainExtraS != nil || got.CompletionistS != nil {
			t.Errorf("row = %+v", got)
		}
		var back []cand
		if err := json.Unmarshal([]byte(got.CandidatesJSON), &back); err != nil || len(back) != 10 || back[9].ID != "j" {
			t.Errorf("candidates = %q (err=%v), want the 10 stored", got.CandidatesJSON, err)
		}
	})

	t.Run("TTB-002/not-found grava só o estado, o título, o termo e os instantes", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		g := m10Game(t, s.DB(), "Jogo")
		row := ttbRow(g)
		row.State, row.ExternalID, row.ExternalTitle = "not-found", "", ""
		row.MainS, row.MainExtraS, row.CompletionistS = nil, nil, nil
		row.CandidatesJSON, row.Confirmed = "[]", false
		if err := s.UpsertTimeToBeat(t.Context(), row); err != nil {
			t.Fatalf("UpsertTimeToBeat: %v", err)
		}
		got, ok, err := s.GetTimeToBeat(t.Context(), g)
		if err != nil || !ok {
			t.Fatalf("GetTimeToBeat ok=%v err=%v", ok, err)
		}
		if got.State != "not-found" || got.ExternalID != "" || got.ExternalTitle != "" ||
			got.MainS != nil || got.MainExtraS != nil || got.CompletionistS != nil ||
			got.CandidatesJSON != "[]" || got.TitleAtFetch != row.TitleAtFetch || got.Term != row.Term ||
			!got.FetchedAt.Equal(row.FetchedAt) || !got.ExpiresAt.Equal(row.ExpiresAt) {
			t.Errorf("row = %+v", got)
		}
	})

	t.Run("TTB-002/valor ausente volta como nulo e o outro valor não é afetado", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		g := m10Game(t, s.DB(), "Jogo")
		row := ttbRow(g)
		row.MainS, row.MainExtraS, row.CompletionistS = nil, i64p(5400), nil
		if err := s.UpsertTimeToBeat(t.Context(), row); err != nil {
			t.Fatalf("UpsertTimeToBeat: %v", err)
		}
		got, _, err := s.GetTimeToBeat(t.Context(), g)
		if err != nil {
			t.Fatal(err)
		}
		if got.MainS != nil || got.CompletionistS != nil || got.MainExtraS == nil || *got.MainExtraS != 5400 {
			t.Errorf("values = %v %v %v", got.MainS, got.MainExtraS, got.CompletionistS)
		}
	})

	t.Run("TTB-002/upsert do mesmo jogo substitui a linha (ON CONFLICT)", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		g := m10Game(t, s.DB(), "Jogo")
		first := ttbRow(g)
		if err := s.UpsertTimeToBeat(t.Context(), first); err != nil {
			t.Fatalf("first upsert: %v", err)
		}
		second := ttbRow(g)
		second.State, second.ExternalID, second.ExternalTitle = "not-found", "", ""
		second.MainS, second.MainExtraS, second.CompletionistS = nil, nil, nil
		second.Confirmed = false
		second.Term = "outro termo"
		if err := s.UpsertTimeToBeat(t.Context(), second); err != nil {
			t.Fatalf("second upsert: %v", err)
		}
		if n := countRows(t, s.DB(), `SELECT COUNT(*) FROM time_to_beat WHERE game_id = ?`, g); n != 1 {
			t.Errorf("rows = %d, want 1", n)
		}
		got, ok, err := s.GetTimeToBeat(t.Context(), g)
		if err != nil || !ok {
			t.Fatalf("GetTimeToBeat ok=%v err=%v", ok, err)
		}
		if got.State != "not-found" || got.Term != "outro termo" || got.MainS != nil || got.Confirmed {
			t.Errorf("row not replaced: %+v", got)
		}
	})

	t.Run("TTB-002/jogo sem linha devolve ok = false", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		g := m10Game(t, s.DB(), "Jogo")
		_, ok, err := s.GetTimeToBeat(t.Context(), g)
		if err != nil || ok {
			t.Errorf("GetTimeToBeat ok=%v err=%v, want ok=false and no error", ok, err)
		}
		_, ok, err = s.GetTimeToBeat(t.Context(), 99999)
		if err != nil || ok {
			t.Errorf("unknown game: ok=%v err=%v, want ok=false and no error", ok, err)
		}
	})
}
