package domain_test

import (
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
)

func TestRankTimeToBeat(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		title     string
		year      int
		results   []domain.TTBResult
		want      domain.TTBState
		wantFirst int // expected Index of the first ranked, -1 = not checked
	}{
		{"TTB-004/Hollow Knight (2017) vs Silksong é found", "Hollow Knight", 2017,
			[]domain.TTBResult{{ID: 26286, Name: "Hollow Knight", Year: 2017}, {ID: 2, Name: "Hollow Knight: Silksong", Year: 2025}},
			domain.TTBFound, 0},
		{"TTB-004/Final Fantasy VII contra Remake é found", "Final Fantasy VII", 1997,
			[]domain.TTBResult{{ID: 1, Name: "Final Fantasy VII", Year: 1997}, {ID: 2, Name: "Final Fantasy VII Remake", Year: 2020}},
			domain.TTBFound, 0},
		{"TTB-004/Resident Evil 2 (1998) contra 2019 e 1998 é doubt", "Resident Evil 2", 1998,
			[]domain.TTBResult{{ID: 10, Name: "Resident Evil 2", Year: 2019}, {ID: 11, Name: "Resident Evil 2", Year: 1998}},
			domain.TTBDoubt, -1},
		{"TTB-004/Zelda sem ano é doubt", "Zelda", 0,
			[]domain.TTBResult{{ID: 1, Name: "The Legend of Zelda", Year: 1986}, {ID: 2, Name: "Zelda II", Year: 1987}},
			domain.TTBDoubt, -1},
		{"TTB-004/sem resultados é not-found", "Jogo Que Não Existe", 0, nil, domain.TTBNotFound, -1},
		{"TTB-004/Celeste com apelido é found", "Celeste", 2018,
			[]domain.TTBResult{{ID: 5, Name: "Celeste", Aliases: []string{"Celeste Classic"}, Year: 2018}},
			domain.TTBFound, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			state, ranked := domain.RankTimeToBeat(tt.title, tt.year, tt.results)
			if state != tt.want {
				t.Fatalf("state = %q, want %q (ranked %+v)", state, tt.want, ranked)
			}
			if tt.wantFirst >= 0 && (len(ranked) == 0 || ranked[0].Index != tt.wantFirst) {
				t.Fatalf("first ranked = %+v, want index %d", ranked, tt.wantFirst)
			}
		})
	}

	t.Run("TTB-004/Super Mario World (USA) é found e o segundo tem a marca 2", func(t *testing.T) {
		t.Parallel()
		state, ranked := domain.RankTimeToBeat("Super Mario World (USA)", 1990, []domain.TTBResult{
			{ID: 1, Name: "Super Mario World", Year: 1990},
			{ID: 2, Name: "Super Mario World 2: Yoshi's Island", Year: 1995},
		})
		if state != domain.TTBFound {
			t.Fatalf("state = %q, want found", state)
		}
		if len(ranked) < 2 || ranked[0].Index != 0 || ranked[1].Index != 1 {
			t.Fatalf("ranked = %+v", ranked)
		}
		if !ranked[1].Scored.Capped {
			t.Errorf("second should be capped by the marks trap: %+v", ranked[1].Scored)
		}
	})

	t.Run("TTB-004/Mega Man X contra Mega Man X2 não é match", func(t *testing.T) {
		t.Parallel()
		state, _ := domain.RankTimeToBeat("Mega Man X", 1993, []domain.TTBResult{{ID: 1, Name: "Mega Man X2", Year: 1994}})
		if state == domain.TTBFound {
			t.Fatalf("state = found, want doubt or not-found")
		}
	})

	t.Run("TTB-004/ordem por pontuação e empate pelo menor ID", func(t *testing.T) {
		t.Parallel()
		// Worse match first in the input; the best one must come first.
		_, ranked := domain.RankTimeToBeat("Celeste", 2018, []domain.TTBResult{
			{ID: 1, Name: "Celeste Something Else Entirely", Year: 2018},
			{ID: 2, Name: "Celeste", Year: 2018},
		})
		if len(ranked) == 0 || ranked[0].Index != 1 {
			t.Fatalf("ranked = %+v, want index 1 first", ranked)
		}
		for i := 1; i < len(ranked); i++ {
			if ranked[i-1].Scored.Score < ranked[i].Scored.Score {
				t.Errorf("not sorted by descending score: %+v", ranked)
			}
		}
		// Tie: identical scores, the smaller ID wins even when listed later.
		_, tie := domain.RankTimeToBeat("Resident Evil 2", 1998, []domain.TTBResult{
			{ID: 20, Name: "Resident Evil 2", Year: 1998},
			{ID: 10, Name: "Resident Evil 2", Year: 1998},
		})
		if len(tie) != 2 || tie[0].Index != 1 {
			t.Fatalf("tie ranked = %+v, want index 1 (ID 10) first", tie)
		}
	})

	t.Run("TTB-004/a plataforma não entra na pontuação", func(t *testing.T) {
		t.Parallel()
		_, ranked := domain.RankTimeToBeat("Hollow Knight", 2017, []domain.TTBResult{{ID: 1, Name: "Hollow Knight", Year: 2017}})
		want := domain.ScoreGroup(domain.Query{Title: "Hollow Knight", Year: 2017, LibraryKey: ""},
			[]domain.Member{{Provider: "hltb", Title: "Hollow Knight", Year: 2017}})
		if len(ranked) != 1 || ranked[0].Scored != want {
			t.Fatalf("ranked = %+v, want score %+v", ranked, want)
		}
	})
}

func TestTTBExpiresAt(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		source string
		state  domain.TTBState
		ttl    time.Duration
	}{
		{"TTB-006/found do hltb vence em 90 dias", domain.TTBSourceHLTB, domain.TTBFound, 90 * 24 * time.Hour},
		{"TTB-006/found do igdb vence em 30 dias", domain.TTBSourceIGDB, domain.TTBFound, 30 * 24 * time.Hour},
		{"TTB-006/doubt e not-found vencem em 30 dias/doubt", domain.TTBSourceHLTB, domain.TTBDoubt, 30 * 24 * time.Hour},
		{"TTB-006/doubt e not-found vencem em 30 dias/not-found", domain.TTBSourceHLTB, domain.TTBNotFound, 30 * 24 * time.Hour},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := domain.TTBExpiresAt(tt.source, tt.state, at); !got.Equal(at.Add(tt.ttl)) {
				t.Errorf("got %v, want %v", got, at.Add(tt.ttl))
			}
		})
	}
}

func TestTTBStale(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	exp := now.Add(time.Hour)

	t.Run("TTB-006/vence quando agora ≥ expires_at", func(t *testing.T) {
		t.Parallel()
		for _, src := range []string{domain.TTBSourceHLTB, domain.TTBSourceIGDB} {
			if domain.TTBStale(src, false, "A", "A", exp, exp.Add(-time.Nanosecond)) {
				t.Errorf("%s: stale before expiry", src)
			}
			if !domain.TTBStale(src, false, "A", "A", exp, exp) {
				t.Errorf("%s: not stale at expiry", src)
			}
			if !domain.TTBStale(src, false, "A", "A", exp, exp.Add(time.Second)) {
				t.Errorf("%s: not stale after expiry", src)
			}
		}
	})
	t.Run("TTB-006/título atual diferente do gravado vence na hora se não confirmada", func(t *testing.T) {
		t.Parallel()
		if !domain.TTBStale(domain.TTBSourceHLTB, false, "Old", "New", exp, now) {
			t.Error("title change should make an unconfirmed row stale")
		}
		if domain.TTBStale(domain.TTBSourceHLTB, false, "Same", "Same", exp, now) {
			t.Error("same title must not be stale")
		}
		if !domain.TTBStale(domain.TTBSourceHLTB, false, "Game", "game", exp, now) {
			t.Error("comparison is exact (case matters)")
		}
	})
	t.Run("TTB-006/linha confirmada ignora a troca de título", func(t *testing.T) {
		t.Parallel()
		if domain.TTBStale(domain.TTBSourceHLTB, true, "Old", "New", exp, now) {
			t.Error("confirmed row must ignore title change")
		}
		if !domain.TTBStale(domain.TTBSourceHLTB, true, "Old", "New", exp, exp) {
			t.Error("confirmed row still expires by time")
		}
	})
}

func TestTTBBackoff(t *testing.T) {
	t.Parallel()
	want := map[int]time.Duration{
		1: time.Minute, 2: 5 * time.Minute, 3: 30 * time.Minute,
		4: 60 * time.Minute, 5: 60 * time.Minute, 100: 60 * time.Minute,
	}
	t.Run("TTB-011/escada 1, 5, 30 e 60 minutos; a quarta em diante fica em 60", func(t *testing.T) {
		t.Parallel()
		for n, d := range want {
			if got := domain.TTBBackoff(n); got != d {
				t.Errorf("TTBBackoff(%d) = %v, want %v", n, got, d)
			}
		}
	})
	t.Run("TTB-038/a mesma escada serve ao IGDB", func(t *testing.T) {
		t.Parallel()
		// The ladder is a pure function of the failure count; both sources share it.
		for n, d := range want {
			if got := domain.TTBBackoff(n); got != d {
				t.Errorf("failures=%d: %v, want %v", n, got, d)
			}
		}
	})
}

func TestTTBResolve(t *testing.T) {
	t.Parallel()
	igdbAll := []domain.IGDBOutcome{domain.IGDBNotAsked, domain.IGDBValues, domain.IGDBNoMatch, domain.IGDBUnavailable, domain.IGDBNoKey}
	type want struct {
		write  bool
		source string
		state  domain.TTBState
	}
	check := func(t *testing.T, h domain.HLTBOutcome, i domain.IGDBOutcome, w want) {
		t.Helper()
		write, source, state := domain.TTBResolve(h, i)
		if write != w.write {
			t.Fatalf("write = %v, want %v (h=%d i=%d)", write, w.write, h, i)
		}
		if w.write && (source != w.source || state != w.state) {
			t.Fatalf("got (%q,%q), want (%q,%q) (h=%d i=%d)", source, state, w.source, w.state, h, i)
		}
	}

	t.Run("TTB-033/found do hltb: grava found hltb e não consulta o IGDB", func(t *testing.T) {
		t.Parallel()
		for _, i := range igdbAll {
			check(t, domain.HLTBFound, i, want{true, domain.TTBSourceHLTB, domain.TTBFound})
		}
	})
	t.Run("TTB-033/doubt do hltb: grava doubt hltb", func(t *testing.T) {
		t.Parallel()
		for _, i := range igdbAll {
			check(t, domain.HLTBDoubt, i, want{true, domain.TTBSourceHLTB, domain.TTBDoubt})
		}
	})
	t.Run("TTB-033/not-found e IGDB com valores: grava found igdb", func(t *testing.T) {
		t.Parallel()
		check(t, domain.HLTBNotFound, domain.IGDBValues, want{true, domain.TTBSourceIGDB, domain.TTBFound})
	})
	t.Run("TTB-033/not-found e IGDB sem match ou sem valores: not-found hltb", func(t *testing.T) {
		t.Parallel()
		check(t, domain.HLTBNotFound, domain.IGDBNoMatch, want{true, domain.TTBSourceHLTB, domain.TTBNotFound})
	})
	t.Run("TTB-033/not-found e IGDB com falha, em recuo ou sem chave: not-found hltb", func(t *testing.T) {
		t.Parallel()
		for _, i := range []domain.IGDBOutcome{domain.IGDBUnavailable, domain.IGDBNoKey, domain.IGDBNotAsked} {
			check(t, domain.HLTBNotFound, i, want{true, domain.TTBSourceHLTB, domain.TTBNotFound})
		}
	})
	t.Run("TTB-034/falha do hltb e IGDB com valores: found igdb", func(t *testing.T) {
		t.Parallel()
		check(t, domain.HLTBFailed, domain.IGDBValues, want{true, domain.TTBSourceIGDB, domain.TTBFound})
	})
	t.Run("TTB-034/falha ou recuo do hltb e IGDB sem match ou sem valores: nada gravado", func(t *testing.T) {
		t.Parallel()
		check(t, domain.HLTBFailed, domain.IGDBNoMatch, want{write: false})
	})
	t.Run("TTB-034/falha do hltb e IGDB com falha, recuo ou sem chave: nada gravado", func(t *testing.T) {
		t.Parallel()
		for _, i := range []domain.IGDBOutcome{domain.IGDBUnavailable, domain.IGDBNoKey, domain.IGDBNotAsked} {
			check(t, domain.HLTBFailed, i, want{write: false})
		}
	})
	t.Run("TTB-033/termo vazio: not-found hltb sem consulta", func(t *testing.T) {
		t.Parallel()
		for _, i := range igdbAll {
			check(t, domain.HLTBEmptyTerm, i, want{true, domain.TTBSourceHLTB, domain.TTBNotFound})
		}
	})
}
