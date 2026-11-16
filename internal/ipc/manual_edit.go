package ipc

// Manual edits of a game (phase 14) [EDM-001..EDM-005]: images chosen from the disk and the total
// play time typed by the user.
const (
	MethodMediaAdd        = "media.add"
	MethodMediaRemove     = "media.remove"
	MethodGameSetPlayTime = "game.setPlayTime"
)

// ProviderUser is the provider of an image the user added from a file.
const ProviderUser = "user"

// MaxPlaySeconds is the largest total play time accepted (999,999 hours).
const MaxPlaySeconds = 999_999 * 3600

// MediaAddParams are the params of media.add. Kind is cover, screenshot or background; Path is the
// absolute path of a PNG or JPEG file.
type MediaAddParams struct {
	GameID int64  `json:"gameId"`
	Kind   string `json:"kind"`
	Path   string `json:"path"`
}

// MediaAddResult is the result of media.add.
type MediaAddResult struct {
	Added bool `json:"added"`
}

// MediaRemoveParams are the params of media.remove.
type MediaRemoveParams struct {
	MediaID int64 `json:"mediaId"`
}

// MediaRemoveResult is the result of media.remove.
type MediaRemoveResult struct {
	Removed bool `json:"removed"`
}

// GameSetPlayTimeParams are the params of game.setPlayTime; the result is GameDetailResult.
type GameSetPlayTimeParams struct {
	GameID           int64 `json:"gameId"`
	TotalPlaySeconds int64 `json:"totalPlaySeconds"`
}
