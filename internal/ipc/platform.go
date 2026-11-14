package ipc

// Platform methods of phase 9 (contract v0.11.0 [PLT-011..PLT-021]).
const (
	MethodPlatformUpdate  = "platform.update"
	MethodPlatformSetIcon = "platform.setIcon"
	MethodPlatformDelete  = "platform.delete"
)

// CodePlatformInUse is the error of platform.delete on a platform that has games (PLT-021).
const CodePlatformInUse = -32013

// MsgPlatformInUse is the message of CodePlatformInUse.
const MsgPlatformInUse = "Platform has games"

// Reasons of an invalid platform icon, carried in PlatformIconErrorData (PLT-012).
const (
	IconReasonNotAbsolute = "not-absolute"
	IconReasonNotFound    = "not-found"
	IconReasonNotAFile    = "not-a-file"
	IconReasonTooLarge    = "too-large"
	IconReasonUnsupported = "unsupported"
)

// PlatformUpdateParams are the params of platform.update.
type PlatformUpdateParams struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"`
}

// PlatformSetIconParams are the params of platform.setIcon. A null Path goes back to the
// default icon.
type PlatformSetIconParams struct {
	PlatformID int64            `json:"platformId"`
	Path       Optional[string] `json:"path"`
}

// PlatformDeleteParams are the params of platform.delete.
type PlatformDeleteParams struct {
	ID int64 `json:"id"`
}

// PlatformDeletedResult is the result of platform.delete.
type PlatformDeletedResult struct {
	ID int64 `json:"id"`
}

// PlatformInUseData is the data of the PlatformInUse error.
type PlatformInUseData struct {
	PlatformID int64 `json:"platformId"`
	GameCount  int64 `json:"gameCount"`
}

// PlatformIconErrorData is the data of the InvalidParams error of platform.setIcon.
type PlatformIconErrorData struct {
	Reason string `json:"reason"`
}
