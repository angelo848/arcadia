package domain_test

import (
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
)

var editNow = time.Date(2026, 11, 25, 20, 0, 0, 0, time.UTC)

func TestParseSessionSource(t *testing.T) {
	t.Parallel()

	t.Run("SED-002/tracked, import e manual são aceitos", func(t *testing.T) {
		t.Parallel()
		cases := map[string]domain.SessionSource{
			"tracked": domain.SourceTracked,
			"import":  domain.SourceImport,
			"manual":  domain.SourceManual,
		}
		for in, want := range cases {
			got, err := domain.ParseSessionSource(in)
			if err != nil {
				t.Fatalf("%q: erro inesperado: %v", in, err)
			}
			if got != want {
				t.Errorf("%q: got %q, want %q", in, got, want)
			}
		}
	})

	t.Run(`SED-002/vazio, "Manual", "MANUAL" e "edited" retornam erro`, func(t *testing.T) {
		t.Parallel()
		for _, in := range []string{"", "Manual", "MANUAL", "edited"} {
			if _, err := domain.ParseSessionSource(in); err == nil {
				t.Errorf("%q: esperava erro", in)
			}
		}
	})
}

func TestValidateSessionTimes(t *testing.T) {
	t.Parallel()
	h := time.Hour
	T := editNow

	cases := []struct {
		name    string
		times   domain.SessionTimes
		checked bool
		want    domain.EditReason
	}{
		{"SED-015/linha 3: início = fim é end-not-after-start (e fim antes do início)",
			domain.SessionTimes{Start: T.Add(-h), End: T.Add(-h), Active: 30 * time.Second}, true, domain.ReasonEndNotAfterStart},
		{"SED-015/linha 3b: fim antes do início é end-not-after-start",
			domain.SessionTimes{Start: T.Add(-h), End: T.Add(-2 * h), Active: 30 * time.Second}, true, domain.ReasonEndNotAfterStart},
		{"SED-016/linha 4: fim em T+1min é in-future",
			domain.SessionTimes{Start: T.Add(-h), End: T.Add(time.Minute), Active: 30 * time.Minute}, true, domain.ReasonInFuture},
		{"SED-016/fim igual a T é aceito (linha 2)",
			domain.SessionTimes{Start: T.Add(-2 * h), End: T, Active: 2 * h}, true, ""},
		{"SED-017/linha 5: início em 1979-12-31 é too-early",
			domain.SessionTimes{Start: time.Date(1979, 12, 31, 23, 59, 0, 0, time.UTC), End: time.Date(1980, 1, 1, 1, 0, 0, 0, time.UTC), Active: 30 * time.Minute}, true, domain.ReasonTooEarly},
		{"SED-017/1980-01-01T00:00:00Z é aceito",
			domain.SessionTimes{Start: domain.EarliestSessionStart, End: domain.EarliestSessionStart.Add(h), Active: h}, true, ""},
		{"SED-018/linha 6: 25 h é span-too-long",
			domain.SessionTimes{Start: T.Add(-25 * h), End: T, Active: 2 * h}, true, domain.ReasonSpanTooLong},
		{"SED-018/linha 10: exatamente 24 h é válido",
			domain.SessionTimes{Start: T.Add(-24 * h), End: T, Active: 24 * h}, true, ""},
		{"SED-018/24 h + 1 ms é span-too-long",
			domain.SessionTimes{Start: T.Add(-24*h - time.Millisecond), End: T, Active: 2 * h}, true, domain.ReasonSpanTooLong},
		{"SED-021/linha 7: spanChecked=false aceita 25 h",
			domain.SessionTimes{Start: T.Add(-25 * h), End: T, Active: 2 * h}, false, ""},
		{"SED-019/linha 8: 29 s é active-too-short",
			domain.SessionTimes{Start: T.Add(-h), End: T, Active: 29 * time.Second}, true, domain.ReasonActiveTooShort},
		{"SED-019/30 s exatos é aceito",
			domain.SessionTimes{Start: T.Add(-h), End: T, Active: 30 * time.Second}, true, ""},
		{"SED-019/linha 9: tempo 1h01 num intervalo de 1h é active-exceeds-span",
			domain.SessionTimes{Start: T.Add(-h), End: T, Active: h + time.Minute}, true, domain.ReasonActiveExceedsSpan},
		{"SED-019/tempo igual ao intervalo é aceito",
			domain.SessionTimes{Start: T.Add(-h), End: T, Active: h}, true, ""},
		{"SED-019/sessão com 2 h e fim 1 h depois do início é active-exceeds-span",
			domain.SessionTimes{Start: T.Add(-3 * h), End: T.Add(-2 * h), Active: 2 * h}, true, domain.ReasonActiveExceedsSpan},
		{"SED-019/linha 1: caminho feliz",
			domain.SessionTimes{Start: T.Add(-2 * h), End: T.Add(-h), Active: h}, true, ""},

		// Ordem das checagens.
		{"SED-015/ordem: too-early vence end-not-after-start",
			domain.SessionTimes{Start: time.Date(1979, 1, 1, 0, 0, 0, 0, time.UTC), End: time.Date(1978, 1, 1, 0, 0, 0, 0, time.UTC), Active: time.Second}, true, domain.ReasonTooEarly},
		{"SED-015/ordem: end-not-after-start vence in-future",
			domain.SessionTimes{Start: T.Add(2 * h), End: T.Add(h), Active: time.Second}, true, domain.ReasonEndNotAfterStart},
		{"SED-015/ordem: in-future vence span-too-long",
			domain.SessionTimes{Start: T.Add(-25 * h), End: T.Add(time.Minute), Active: time.Second}, true, domain.ReasonInFuture},
		{"SED-015/ordem: span-too-long vence active-too-short",
			domain.SessionTimes{Start: T.Add(-25 * h), End: T, Active: time.Second}, true, domain.ReasonSpanTooLong},
		{"SED-015/ordem: active-too-short vence active-exceeds-span",
			domain.SessionTimes{Start: T.Add(-10 * time.Second), End: T, Active: 20 * time.Second}, true, domain.ReasonActiveTooShort},

		// spanChecked=false não desliga as outras regras.
		{"SED-021/spanChecked=false não desliga end-not-after-start",
			domain.SessionTimes{Start: T.Add(-h), End: T.Add(-h), Active: time.Minute}, false, domain.ReasonEndNotAfterStart},
		{"SED-021/spanChecked=false não desliga in-future",
			domain.SessionTimes{Start: T.Add(-h), End: T.Add(time.Minute), Active: time.Minute}, false, domain.ReasonInFuture},
		{"SED-021/spanChecked=false não desliga too-early",
			domain.SessionTimes{Start: time.Date(1979, 1, 1, 0, 0, 0, 0, time.UTC), End: T, Active: time.Minute}, false, domain.ReasonTooEarly},
		{"SED-021/spanChecked=false não desliga active-too-short",
			domain.SessionTimes{Start: T.Add(-300 * h), End: T, Active: 29 * time.Second}, false, domain.ReasonActiveTooShort},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := domain.ValidateSessionTimes(tc.times, T, tc.checked); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestOverlaps(t *testing.T) {
	t.Parallel()
	at := func(m int) time.Time { return editNow.Add(time.Duration(m) * time.Minute) }

	cases := []struct {
		name           string
		as, ae, bs, be int
		want           bool
	}{
		{"SED-020/cruzar de fato", 0, 60, 30, 90, true},
		{"SED-020/cruzar de fato, ordem inversa", 30, 90, 0, 60, true},
		{"SED-020/encostar (fim = início) não cruza: a antes de b", 0, 60, 60, 120, false},
		{"SED-020/encostar (fim = início) não cruza: b antes de a", 60, 120, 0, 60, false},
		{"SED-020/contida", 10, 20, 0, 60, true},
		{"SED-020/contendo", 0, 60, 10, 20, true},
		{"SED-020/intervalos disjuntos", 0, 10, 50, 60, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := domain.Overlaps(at(tc.as), at(tc.ae), at(tc.bs), at(tc.be)); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestClampTotal(t *testing.T) {
	t.Parallel()
	const max int64 = 3599996400

	cases := []struct {
		name string
		in   int64
		want int64
	}{
		{"SED-013/negativo vira 0", -5, 0},
		{"SED-013/acima de 3599996400 vira o teto", max + 1, max},
		{"SED-013/valor dentro da faixa passa igual", 12345, 12345},
		{"SED-013/zero e o teto passam iguais", max, max},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := domain.ClampTotal(tc.in, max); got != tc.want {
				t.Errorf("got %d, want %d", got, tc.want)
			}
		})
	}
}

func TestClipPause(t *testing.T) {
	t.Parallel()
	at := func(m int) time.Time { return editNow.Add(time.Duration(m) * time.Minute) }
	ptr := func(m int) *time.Time { v := at(m); return &v }
	start, end := at(0), at(60)

	cases := []struct {
		name    string
		paused  time.Time
		resumed *time.Time
		wantP   time.Time
		wantR   time.Time
		wantOK  bool
	}{
		{"SED-009/pausa dentro do intervalo não muda", at(10), ptr(20), at(10), at(20), true},
		{"SED-009/pausa que sai pela esquerda é cortada no início", at(-10), ptr(20), start, at(20), true},
		{"SED-009/pausa que sai pela direita é cortada no fim", at(40), ptr(90), at(40), end, true},
		{"SED-009/pausa que cobre tudo vira o intervalo todo", at(-10), ptr(90), start, end, true},
		{"SED-009/pausa totalmente fora devolve ok=false (antes)", at(-30), ptr(-10), time.Time{}, time.Time{}, false},
		{"SED-009/pausa totalmente fora devolve ok=false (depois)", at(70), ptr(90), time.Time{}, time.Time{}, false},
		{"SED-009/pausa com duração zero depois do corte devolve ok=false", at(-10), ptr(0), time.Time{}, time.Time{}, false},
		{"SED-009/retomada nula conta como o fim da sessão", at(40), nil, at(40), end, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p, r, ok := domain.ClipPause(tc.paused, tc.resumed, start, end)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if !p.Equal(tc.wantP) || !r.Equal(tc.wantR) {
				t.Errorf("got [%v, %v], want [%v, %v]", p, r, tc.wantP, tc.wantR)
			}
		})
	}
}
