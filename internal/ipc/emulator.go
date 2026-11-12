package ipc

// Emulator methods (phase 6b, contract v0.8.0 [EMU-001, EMU-002]).
const (
	MethodEmulatorList        = "emulator.list"
	MethodEmulatorAdd         = "emulator.add"
	MethodEmulatorUpdate      = "emulator.update"
	MethodEmulatorDelete      = "emulator.delete"
	MethodPlatformSetEmulator = "platform.setEmulator"
)

// Emulator is a program that runs the games of one or more platforms (EMU-001).
type Emulator struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	ExePath      string `json:"exePath"`
	ArgsTemplate string `json:"argsTemplate"`
}

// EmulatorListResult is the result of emulator.list.
type EmulatorListResult struct {
	Emulators []Emulator `json:"emulators"`
}

// EmulatorResult is the result of emulator.add and emulator.update.
type EmulatorResult struct {
	Emulator Emulator `json:"emulator"`
}

// EmulatorDeletedResult is the result of emulator.delete.
type EmulatorDeletedResult struct {
	ID int64 `json:"id"`
}

// PlatformResult is the result of platform.setEmulator.
type PlatformResult struct {
	Platform Platform `json:"platform"`
}

// EmulatorAddParams are the params of emulator.add.
type EmulatorAddParams struct {
	Name         string `json:"name"`
	ExePath      string `json:"exePath"`
	ArgsTemplate string `json:"argsTemplate"`
}

// EmulatorUpdateParams are the params of emulator.update.
type EmulatorUpdateParams struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	ExePath      string `json:"exePath"`
	ArgsTemplate string `json:"argsTemplate"`
}

// EmulatorDeleteParams are the params of emulator.delete.
type EmulatorDeleteParams struct {
	ID int64 `json:"id"`
}

// PlatformSetEmulatorParams are the params of platform.setEmulator. An absent or null
// EmulatorID clears the default.
type PlatformSetEmulatorParams struct {
	PlatformID int64           `json:"platformId"`
	EmulatorID Optional[int64] `json:"emulatorId"`
}
