package store_test

import (
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/store"
)

// instants in non-UTC zones with sub-millisecond precision (ADR 0009).
var conversionCases = []struct {
	name   string
	in     time.Time
	millis int64     // expected t.UnixMilli(), written out independently
	want   time.Time // in truncated to ms, in UTC
}{
	{
		name:   "UTC-3 com nanossegundos",
		in:     time.Date(2026, 3, 14, 15, 9, 26, 535_897_932, time.FixedZone("BRT", -3*60*60)),
		millis: 1773511766535,
		want:   time.Date(2026, 3, 14, 18, 9, 26, 535_000_000, time.UTC),
	},
	{
		name:   "UTC+5:30 com microssegundos",
		in:     time.Date(2024, 2, 29, 23, 59, 59, 999_999_000, time.FixedZone("IST", 5*60*60+30*60)),
		millis: 1709231399999,
		want:   time.Date(2024, 2, 29, 18, 29, 59, 999_000_000, time.UTC),
	},
}

func TestTimeConversion(t *testing.T) {
	t.Parallel()

	t.Run("STO-007/grava UnixMilli de instante não-UTC", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		db := s.DB()
		ctx := t.Context()
		pid := insertPlatform(t, s, "PC")

		for _, c := range conversionCases {
			if c.in.UnixMilli() != c.millis {
				t.Fatalf("%s: fixture inconsistent: UnixMilli = %d, table says %d", c.name, c.in.UnixMilli(), c.millis)
			}
			if got := store.TimeToMillis(c.in); got != c.millis {
				t.Errorf("%s: TimeToMillis = %d, want %d", c.name, got, c.millis)
			}

			gameID := mustInsertGame(t, db, pid, "Game "+c.name, "")
			if _, err := db.ExecContext(ctx, `UPDATE game SET first_played_at = ? WHERE id = ?`,
				store.TimeToMillis(c.in), gameID); err != nil {
				t.Fatalf("%s: update first_played_at: %v", c.name, err)
			}
			var (
				stored  int64
				sqlType string
			)
			if err := db.QueryRowContext(ctx,
				`SELECT first_played_at, typeof(first_played_at) FROM game WHERE id = ?`, gameID).
				Scan(&stored, &sqlType); err != nil {
				t.Fatalf("%s: read first_played_at: %v", c.name, err)
			}
			if sqlType != "integer" {
				t.Errorf("%s: typeof(first_played_at) = %q, want integer", c.name, sqlType)
			}
			if stored != c.millis {
				t.Errorf("%s: stored = %d, want %d", c.name, stored, c.millis)
			}
		}
	})

	t.Run("STO-007/lê truncado em ms e em UTC", func(t *testing.T) {
		t.Parallel()
		for _, c := range conversionCases {
			got := store.MillisToTime(store.TimeToMillis(c.in))
			if !got.Equal(c.want) {
				t.Errorf("%s: MillisToTime(TimeToMillis(t)) = %v, want %v", c.name, got, c.want)
			}
			if got.Location() != time.UTC {
				t.Errorf("%s: Location = %v, want UTC", c.name, got.Location())
			}
			if got.Nanosecond()%int(time.Millisecond) != 0 {
				t.Errorf("%s: Nanosecond = %d, want a whole number of ms", c.name, got.Nanosecond())
			}
			if direct := store.MillisToTime(c.millis); !direct.Equal(c.want) || direct.Location() != time.UTC {
				t.Errorf("%s: MillisToTime(%d) = %v, want %v in UTC", c.name, c.millis, direct, c.want)
			}
		}
	})

	t.Run("STO-007/release_date YYYY-MM-DD idêntico", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		db := s.DB()
		pid := insertPlatform(t, s, "Arcade")

		for _, date := range []string{"1998-11-21", "2000-02-29", "1987-01-01"} {
			gameID := mustInsertGame(t, db, pid, "Game "+date, date)
			var (
				got     string
				sqlType string
			)
			if err := db.QueryRowContext(t.Context(),
				`SELECT release_date, typeof(release_date) FROM game WHERE id = ?`, gameID).
				Scan(&got, &sqlType); err != nil {
				t.Fatalf("read release_date: %v", err)
			}
			if got != date {
				t.Errorf("release_date = %q, want %q", got, date)
			}
			if sqlType != "text" {
				t.Errorf("typeof(release_date) = %q, want text", sqlType)
			}
		}
	})
}
