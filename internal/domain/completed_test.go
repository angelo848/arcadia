package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
)

func TestParseCompletedDate(t *testing.T) {
	t.Parallel()

	t.Run("EDT-001/2026-11-12 vira 2026-11-12T00:00:00Z", func(t *testing.T) {
		t.Parallel()

		got, err := domain.ParseCompletedDate("2026-11-12")
		if err != nil {
			t.Fatalf("ParseCompletedDate(2026-11-12) error: %v", err)
		}
		want := time.Date(2026, time.November, 12, 0, 0, 0, 0, time.UTC)
		if !got.Equal(want) {
			t.Errorf("ParseCompletedDate(2026-11-12) = %s, want %s", got.Format(time.RFC3339), want.Format(time.RFC3339))
		}
		if got.Format(time.RFC3339) != "2026-11-12T00:00:00Z" {
			t.Errorf("ParseCompletedDate(2026-11-12) = %s, want 2026-11-12T00:00:00Z", got.Format(time.RFC3339))
		}
	})

	t.Run("EDT-001/formato diferente e data inexistente como 2026-13-45 são erro", func(t *testing.T) {
		t.Parallel()

		inputs := []string{
			"",
			"2026-11",
			"2026-1-5",
			"12/11/2026",
			"2026-11-12T00:00:00Z",
			"2026-13-45",
			"2026-02-30",
		}
		for _, in := range inputs {
			got, err := domain.ParseCompletedDate(in)
			if !errors.Is(err, domain.ErrInvalidField) {
				t.Errorf("ParseCompletedDate(%q) = %s, %v; want ErrInvalidField", in, got, err)
			}
		}
	})

	t.Run("EDT-001/FormatCompletedDate devolve YYYY-MM-DD em UTC", func(t *testing.T) {
		t.Parallel()

		// 2026-11-12 22:00 em UTC-03:00 é 2026-11-13 01:00 em UTC: o dia de saída é o UTC.
		local := time.FixedZone("UTC-3", -3*60*60)
		if got := domain.FormatCompletedDate(time.Date(2026, time.November, 12, 22, 0, 0, 0, local)); got != "2026-11-13" {
			t.Errorf("FormatCompletedDate(2026-11-12 22:00 UTC-3) = %q, want %q", got, "2026-11-13")
		}
		if got := domain.FormatCompletedDate(time.Date(2026, time.November, 12, 0, 0, 0, 0, time.UTC)); got != "2026-11-12" {
			t.Errorf("FormatCompletedDate(2026-11-12 00:00 UTC) = %q, want %q", got, "2026-11-12")
		}
	})

	t.Run("EDT-001/ida e volta: FormatCompletedDate(ParseCompletedDate(s)) é s", func(t *testing.T) {
		t.Parallel()

		const s = "2024-02-29"
		got, err := domain.ParseCompletedDate(s)
		if err != nil {
			t.Fatalf("ParseCompletedDate(%q) error: %v", s, err)
		}
		if back := domain.FormatCompletedDate(got); back != s {
			t.Errorf("round trip = %q, want %q", back, s)
		}
	})
}
