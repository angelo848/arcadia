package ipc

// MethodGameLocation asks where the files of a game are, to open them in the Explorer (phase 1.0.3).
const MethodGameLocation = "game.location"

// Kinds of a GameLocationResult.
const (
	LocationFile = "file" // Path is a file: the Explorer opens its folder with the file selected
	LocationDir  = "dir"  // Path is a folder: the Explorer opens it
)

// GameLocationParams are the params of game.location.
type GameLocationParams struct {
	GameID int64 `json:"gameId"`
}

// GameLocationResult is the result of game.location. Path is empty when the game has no known
// location (a URI game without an install folder).
type GameLocationResult struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
}
