//go:build windows

package input

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	xinputDLL            = "xinput1_4.dll"
	xinputMaxSlot        = 3
	errorDeviceNotConnec = 1167 // ERROR_DEVICE_NOT_CONNECTED
)

// xinputGamepad mirrors XINPUT_GAMEPAD.
type xinputGamepad struct {
	Buttons      uint16
	LeftTrigger  uint8
	RightTrigger uint8
	ThumbLX      int16
	ThumbLY      int16
	ThumbRX      int16
	ThumbRY      int16
}

// xinputState mirrors XINPUT_STATE.
type xinputState struct {
	PacketNumber uint32
	Gamepad      xinputGamepad
}

type xinputAPI struct {
	getState *windows.LazyProc
}

// NewXInput loads xinput1_4.dll and returns a PadAPI backed by XInputGetState.
func NewXInput() (PadAPI, error) {
	return loadXInput(xinputDLL)
}

func loadXInput(dll string) (PadAPI, error) {
	lib := windows.NewLazySystemDLL(dll)
	if err := lib.Load(); err != nil {
		return nil, fmt.Errorf("load %s: %w", dll, err)
	}
	proc := lib.NewProc("XInputGetState")
	if err := proc.Find(); err != nil {
		return nil, fmt.Errorf("find XInputGetState in %s: %w", dll, err)
	}
	return &xinputAPI{getState: proc}, nil
}

func (x *xinputAPI) State(slot int) (PadState, error) {
	if slot < 0 || slot > xinputMaxSlot {
		return PadState{}, fmt.Errorf("xinput: slot %d out of range 0..%d", slot, xinputMaxSlot)
	}
	var st xinputState
	r, _, _ := x.getState.Call(uintptr(slot), uintptr(unsafe.Pointer(&st)))
	switch r {
	case 0:
		return PadState{Buttons: st.Gamepad.Buttons}, nil
	case errorDeviceNotConnec:
		return PadState{}, fmt.Errorf("xinput slot %d: %w", slot, ErrDisconnected)
	default:
		return PadState{}, fmt.Errorf("xinput slot %d: XInputGetState returned %d", slot, r)
	}
}
