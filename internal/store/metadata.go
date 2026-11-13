package store

import (
	"context"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/scrapers"
)

// playModeOf maps the mode names of the providers to the play modes of DOM-014; unknown names
// are left out.
var playModeOf = map[string]domain.PlayMode{
	"Single player": domain.PlayModeSingle,
	"Multiplayer":   domain.PlayModeMultiOnline,
	"Co-operative":  domain.PlayModeCoopOnline,
	"Split screen":  domain.PlayModeMultiLocal,
}

// ApplyMetadata writes the merged fields of a provider onto the game of id (MID-003). A field the
// user filled stays as it is, unless replace is set; an empty field of the provider is not written.
// It returns the game after the write.
func (s *Store) ApplyMetadata(ctx context.Context, id int64, f scrapers.Fields, replace bool, at time.Time) (GameDetail, error) {
	cur, err := s.GameDetail(ctx, id)
	if err != nil {
		return GameDetail{}, err
	}
	var p GamePatch
	if f.Description != "" && (replace || cur.Description == "") {
		d := f.Description
		p.Description = &d
	}
	if len(f.Genres) > 0 && (replace || len(cur.Genres) == 0) {
		g := f.Genres
		p.Genres = &g
	}
	if len(f.Developers) > 0 && (replace || len(cur.Developers) == 0) {
		d := f.Developers
		p.Developers = &d
	}
	if len(f.Publishers) > 0 && (replace || len(cur.Publishers) == 0) {
		pub := f.Publishers
		p.Publishers = &pub
	}
	if modes := playModesOf(f.Modes); len(modes) > 0 && (replace || len(cur.PlayModes) == 0) {
		p.PlayModes = &modes
	}
	if f.Year != 0 && (replace || cur.ReleaseDate == "") {
		year := f.Year
		yp := &year
		p.Year = &yp
	}
	if p == (GamePatch{}) {
		return cur, nil
	}
	return s.UpdateGame(ctx, id, p, at)
}

// playModesOf maps provider mode names to play modes, dropping duplicates and unknown names.
func playModesOf(names []string) []domain.PlayMode {
	var out []domain.PlayMode
	seen := map[domain.PlayMode]bool{}
	for _, n := range names {
		m, ok := playModeOf[n]
		if ok && !seen[m] {
			seen[m] = true
			out = append(out, m)
		}
	}
	return out
}
