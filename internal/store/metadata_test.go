package store

import (
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/scrapers"
)

func metadataGame(t *testing.T, s *Store, description string) int64 {
	t.Helper()
	at := time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)
	g, err := s.AddGame(t.Context(), NewGame{Title: "Celeste", Exe: `C:\c.exe`, At: at})
	if err != nil {
		t.Fatalf("AddGame: %v", err)
	}
	if description != "" {
		if _, err := s.UpdateGame(t.Context(), g.ID, GamePatch{Description: &description}, at); err != nil {
			t.Fatalf("UpdateGame: %v", err)
		}
	}
	return g.ID
}

func TestApplyMetadata(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
	fields := scrapers.Fields{Description: "Escalada.", Genres: []string{"Platform"}, Year: 2018}

	t.Run("MID-003/campo vazio é preenchido pelo provedor", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		id := metadataGame(t, s, "")
		d, err := s.ApplyMetadata(t.Context(), id, fields, false, at)
		if err != nil {
			t.Fatalf("ApplyMetadata: %v", err)
		}
		if d.Description != "Escalada." || len(d.Genres) != 1 || d.Genres[0] != "Platform" {
			t.Errorf("detail = %+v, want description and genre filled", d)
		}
		if d.ReleaseDate == "" || d.ReleaseDate[:4] != "2018" {
			t.Errorf("release date = %q, want the year 2018", d.ReleaseDate)
		}
	})

	t.Run("MID-003/campo que o usuário preencheu não muda", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		id := metadataGame(t, s, "Meu texto")
		d, err := s.ApplyMetadata(t.Context(), id, fields, false, at)
		if err != nil {
			t.Fatalf("ApplyMetadata: %v", err)
		}
		if d.Description != "Meu texto" {
			t.Errorf("description = %q, want the user's text kept", d.Description)
		}
		if len(d.Genres) != 1 {
			t.Errorf("genres = %v, want the empty field filled", d.Genres)
		}
	})

	t.Run("MID-003/substituir troca os campos que o provedor trouxe", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		id := metadataGame(t, s, "Meu texto")
		d, err := s.ApplyMetadata(t.Context(), id, fields, true, at)
		if err != nil {
			t.Fatalf("ApplyMetadata: %v", err)
		}
		if d.Description != "Escalada." {
			t.Errorf("description = %q, want the provider's text", d.Description)
		}
	})
}
