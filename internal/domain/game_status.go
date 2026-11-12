package domain

import "fmt"

// GameStatus is the play status of a game in the library (DOM-012, ADR 0012).
type GameStatus string

const (
	StatusNotPlayed GameStatus = "not_played" // default for a new game (DOM-012)
	StatusPlaying   GameStatus = "playing"
	StatusCompleted GameStatus = "completed"
	StatusAbandoned GameStatus = "abandoned"
	StatusBacklog   GameStatus = "backlog"
)

// ParseGameStatus accepts exactly one of the five statuses of DOM-012.
// The match is exact: empty and differently-cased values are errors.
func ParseGameStatus(s string) (GameStatus, error) {
	switch st := GameStatus(s); st {
	case StatusNotPlayed, StatusPlaying, StatusCompleted, StatusAbandoned, StatusBacklog:
		return st, nil
	}
	return "", fmt.Errorf("domain: game status %q: %w", s, ErrInvalidField)
}
