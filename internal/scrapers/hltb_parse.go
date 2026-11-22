package scrapers

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
)

// HLTBGame is one game of a HowLongToBeat search answer [TTB-005]. Times are seconds, 0 = no data.
type HLTBGame struct {
	ID                             int64
	Name                           string
	Aliases                        []string
	Year                           int // 0 = unknown
	Platforms                      []string
	Main, MainExtra, Completionist int64
}

var errHLTBFormat = errors.New("formato inválido")

// positiveInt reads a JSON integer literal greater than zero; anything else (null, text, fraction,
// zero, negative) is 0.
func positiveInt(raw json.RawMessage) int64 {
	n, err := strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

// splitList splits a comma separated text, trimming and dropping empty parts.
func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

type hltbItem struct {
	ID       json.RawMessage `json:"game_id"`
	Name     json.RawMessage `json:"game_name"`
	Alias    json.RawMessage `json:"game_alias"`
	Type     json.RawMessage `json:"game_type"`
	Year     json.RawMessage `json:"release_world"`
	Platform json.RawMessage `json:"profile_platform"`
	CompMain json.RawMessage `json:"comp_main"`
	CompPlus json.RawMessage `json:"comp_plus"`
	CompFull json.RawMessage `json:"comp_100"`
}

func rawString(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return s
}

// parseHLTB reads the search answer: only "data" is used [TTB-005].
func parseHLTB(body []byte) ([]HLTBGame, error) {
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, errHLTBFormat
	}
	var items []hltbItem
	if len(env.Data) == 0 || json.Unmarshal(env.Data, &items) != nil || items == nil {
		return nil, errHLTBFormat
	}
	games := make([]HLTBGame, 0, len(items))
	for _, it := range items {
		id := positiveInt(it.ID)
		name := rawString(it.Name)
		if id == 0 || name == "" {
			continue
		}
		if len(it.Type) > 0 && string(it.Type) != "null" && rawString(it.Type) != "game" {
			continue
		}
		g := HLTBGame{
			ID:            id,
			Name:          name,
			Aliases:       splitList(rawString(it.Alias)),
			Platforms:     splitList(rawString(it.Platform)),
			Main:          positiveInt(it.CompMain),
			MainExtra:     positiveInt(it.CompPlus),
			Completionist: positiveInt(it.CompFull),
		}
		if y, err := strconv.ParseInt(strings.TrimSpace(string(it.Year)), 10, 64); err == nil && y >= 1950 && y <= 2100 {
			g.Year = int(y)
		}
		games = append(games, g)
	}
	return games, nil
}
