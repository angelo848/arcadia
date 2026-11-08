package domain_test

import (
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
)

// base is a fixed reference instant; at(m) is base + m minutes.
var base = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

func at(minutes int) time.Time {
	return base.Add(time.Duration(minutes) * time.Minute)
}

func pause(startMin, endMin int) domain.Interval {
	return domain.Interval{Start: at(startMin), End: at(endMin)}
}

func openPause(startMin int) domain.Interval {
	return domain.Interval{Start: at(startMin)}
}

func TestActiveDuration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		start  time.Time
		end    time.Time
		pauses []domain.Interval
		want   time.Duration
	}{
		{
			name:  "DOM-001/sem pausas",
			start: at(0), end: at(60),
			pauses: nil,
			want:   60 * time.Minute,
		},
		{
			name:  "DOM-001/sem pausas com slice vazio",
			start: at(0), end: at(60),
			pauses: []domain.Interval{},
			want:   60 * time.Minute,
		},
		{
			name:  "DOM-004/sessão de duração zero (fim == início) = 0",
			start: at(0), end: at(0),
			pauses: nil,
			want:   0,
		},
		{
			name:  "DOM-001/precisão de nanossegundos",
			start: base, end: base.Add(90*time.Minute + 7*time.Nanosecond),
			pauses: nil,
			want:   90*time.Minute + 7*time.Nanosecond,
		},
		{
			name:  "DOM-002/pausa interna",
			start: at(0), end: at(60),
			pauses: []domain.Interval{pause(10, 20)},
			want:   50 * time.Minute,
		},
		{
			name:  "DOM-002/duas pausas internas disjuntas",
			start: at(0), end: at(60),
			pauses: []domain.Interval{pause(10, 20), pause(30, 35)},
			want:   45 * time.Minute,
		},
		{
			name:  "DOM-002/pausas sobrepostas contam uma vez",
			start: at(0), end: at(60),
			pauses: []domain.Interval{pause(10, 30), pause(20, 40)},
			want:   30 * time.Minute,
		},
		{
			name:  "DOM-002/pausa contida em outra conta uma vez",
			start: at(0), end: at(60),
			pauses: []domain.Interval{pause(10, 40), pause(15, 20)},
			want:   30 * time.Minute,
		},
		{
			name:  "DOM-002/pausas idênticas contam uma vez",
			start: at(0), end: at(60),
			pauses: []domain.Interval{pause(10, 20), pause(10, 20)},
			want:   50 * time.Minute,
		},
		{
			name:  "DOM-002/pausas adjacentes",
			start: at(0), end: at(60),
			pauses: []domain.Interval{pause(10, 20), pause(20, 30)},
			want:   40 * time.Minute,
		},
		{
			name:  "DOM-002/pausas fora de ordem",
			start: at(0), end: at(60),
			pauses: []domain.Interval{pause(40, 50), pause(10, 20)},
			want:   40 * time.Minute,
		},
		{
			name:  "DOM-002/pausas fora de ordem e sobrepostas",
			start: at(0), end: at(60),
			pauses: []domain.Interval{pause(25, 45), pause(5, 15), pause(10, 30)},
			want:   20 * time.Minute,
		},
		{
			name:  "DOM-002/pausa começa antes do início",
			start: at(0), end: at(60),
			pauses: []domain.Interval{pause(-10, 10)},
			want:   50 * time.Minute,
		},
		{
			name:  "DOM-002/pausa termina depois do fim",
			start: at(0), end: at(60),
			pauses: []domain.Interval{pause(50, 70)},
			want:   50 * time.Minute,
		},
		{
			name:  "DOM-002/pausa totalmente fora",
			start: at(0), end: at(60),
			pauses: []domain.Interval{pause(-30, -10), pause(70, 80)},
			want:   60 * time.Minute,
		},
		{
			name:  "DOM-002/pausa encostando nas bordas por fora",
			start: at(0), end: at(60),
			pauses: []domain.Interval{pause(-10, 0), pause(60, 70)},
			want:   60 * time.Minute,
		},
		{
			name:  "DOM-002/pausa cobre a sessão inteira = 0",
			start: at(0), end: at(60),
			pauses: []domain.Interval{pause(-10, 70)},
			want:   0,
		},
		{
			name:  "DOM-002/pausa exatamente igual à sessão = 0",
			start: at(0), end: at(60),
			pauses: []domain.Interval{pause(0, 60)},
			want:   0,
		},
		{
			name:  "DOM-002/pausa invertida (End < Start) é ignorada",
			start: at(0), end: at(60),
			pauses: []domain.Interval{pause(20, 10)},
			want:   60 * time.Minute,
		},
		{
			name:  "DOM-002/pausa invertida ignorada junto com pausa válida",
			start: at(0), end: at(60),
			pauses: []domain.Interval{pause(50, 5), pause(30, 40)},
			want:   50 * time.Minute,
		},
		{
			name:  "DOM-002/pausa de duração zero não desconta",
			start: at(0), end: at(60),
			pauses: []domain.Interval{pause(30, 30)},
			want:   60 * time.Minute,
		},
		{
			name:  "DOM-003/pausa aberta até o fim",
			start: at(0), end: at(60),
			pauses: []domain.Interval{openPause(40)},
			want:   40 * time.Minute,
		},
		{
			name:  "DOM-003/pausa aberta antes do início = 0",
			start: at(0), end: at(60),
			pauses: []domain.Interval{openPause(-10)},
			want:   0,
		},
		{
			name:  "DOM-003/pausa aberta sobreposta a pausa fechada",
			start: at(0), end: at(60),
			pauses: []domain.Interval{pause(30, 50), openPause(40)},
			want:   30 * time.Minute,
		},
		{
			name:  "DOM-003/pausa aberta começando depois do fim não desconta",
			start: at(0), end: at(60),
			pauses: []domain.Interval{openPause(70)},
			want:   60 * time.Minute,
		},
		{
			name:  "DOM-004/fim antes do início = 0",
			start: at(60), end: at(0),
			pauses: nil,
			want:   0,
		},
		{
			name:  "DOM-004/fim antes do início com pausas = 0",
			start: at(60), end: at(0),
			pauses: []domain.Interval{pause(10, 20), openPause(30)},
			want:   0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := domain.ActiveDuration(tc.start, tc.end, tc.pauses)
			if got != tc.want {
				t.Fatalf("ActiveDuration(%v, %v, %v) = %v, want %v", tc.start, tc.end, tc.pauses, got, tc.want)
			}
		})
	}
}

func TestActiveDurationInputUnchanged(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		pauses []domain.Interval
		want   time.Duration
	}{
		{
			name:   "DOM-002/não altera o slice de pausas do chamador",
			pauses: []domain.Interval{pause(40, 50), pause(10, 20), pause(15, 25), pause(-10, 5)},
			want:   30 * time.Minute,
		},
		{
			name:   "DOM-002/não altera o slice com pausa aberta e invertida",
			pauses: []domain.Interval{openPause(55), pause(30, 20), pause(70, 80), pause(10, 20)},
			want:   45 * time.Minute,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			before := append([]domain.Interval(nil), tc.pauses...)
			got := domain.ActiveDuration(at(0), at(60), tc.pauses)
			if got != tc.want {
				t.Fatalf("ActiveDuration = %v, want %v", got, tc.want)
			}
			if len(tc.pauses) != len(before) {
				t.Fatalf("len(pauses) = %d, want %d", len(tc.pauses), len(before))
			}
			for i := range before {
				// == (not Equal) so a changed zero End or location is also caught.
				if tc.pauses[i] != before[i] {
					t.Fatalf("pauses[%d] = %+v after call, want %+v (input mutated)", i, tc.pauses[i], before[i])
				}
			}
		})
	}
}
