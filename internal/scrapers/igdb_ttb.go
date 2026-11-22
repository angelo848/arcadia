package scrapers

import (
	"context"
	"encoding/json"
	"fmt"
)

// IGDBTimeToBeat is the game_time_to_beats record of IGDB, in seconds; 0 = no data [TTB-036].
type IGDBTimeToBeat struct{ Hastily, Normally, Completely int64 }

// TimeToBeat reads game_time_to_beats of one IGDB game. The bool is false when IGDB has no record.
// The call goes through the IGDB limiter, like the metadata search [TTB-038].
func (c *IGDB) TimeToBeat(ctx context.Context, igdbID string) (IGDBTimeToBeat, bool, error) {
	if err := ctx.Err(); err != nil {
		return IGDBTimeToBeat{}, false, err
	}
	var rows []struct {
		Hastily    json.RawMessage `json:"hastily"`
		Normally   json.RawMessage `json:"normally"`
		Completely json.RawMessage `json:"completely"`
	}
	body := fmt.Sprintf("fields game_id,hastily,normally,completely; where game_id = %s; limit 1;", igdbID)
	if err := c.query(ctx, "/game_time_to_beats", body, &rows); err != nil {
		if cerr := ctx.Err(); cerr != nil {
			return IGDBTimeToBeat{}, false, cerr
		}
		return IGDBTimeToBeat{}, false, err
	}
	if len(rows) == 0 {
		return IGDBTimeToBeat{}, false, nil
	}
	r := rows[0]
	return IGDBTimeToBeat{
		Hastily:    positiveInt(r.Hastily),
		Normally:   positiveInt(r.Normally),
		Completely: positiveInt(r.Completely),
	}, true, nil
}
