package domain

import "fmt"

// PlayMode is one of the play modes a game can declare (DOM-014).
type PlayMode string

const (
	PlayModeSingle      PlayMode = "single"
	PlayModeMultiLocal  PlayMode = "multi_local"
	PlayModeMultiOnline PlayMode = "multi_online"
	PlayModeCoopLocal   PlayMode = "coop_local"
	PlayModeCoopOnline  PlayMode = "coop_online"
)

// ParsePlayMode accepts exactly one of the five mode names of DOM-014.
func ParsePlayMode(s string) (PlayMode, error) {
	switch m := PlayMode(s); m {
	case PlayModeSingle, PlayModeMultiLocal, PlayModeMultiOnline, PlayModeCoopLocal, PlayModeCoopOnline:
		return m, nil
	}
	return "", fmt.Errorf("domain: play mode %q: %w", s, ErrInvalidField)
}

// PlayModeGroup is a filter group over play modes (FLT-003, DOM-014).
type PlayModeGroup string

const (
	PlayGroupAll    PlayModeGroup = "all"
	PlayGroupSingle PlayModeGroup = "single"
	PlayGroupMulti  PlayModeGroup = "multi"
	PlayGroupCoop   PlayModeGroup = "coop"
)

// ParsePlayModeGroup accepts exactly all, single, multi or coop. The empty
// string is an error; the default is applied by FilterSpec.Normalize.
func ParsePlayModeGroup(s string) (PlayModeGroup, error) {
	switch g := PlayModeGroup(s); g {
	case PlayGroupAll, PlayGroupSingle, PlayGroupMulti, PlayGroupCoop:
		return g, nil
	}
	return "", fmt.Errorf("domain: play mode group %q: %w", s, ErrInvalidField)
}

// Modes returns the modes covered by the group, or nil for PlayGroupAll
// (any mode, including a game with no mode).
func (g PlayModeGroup) Modes() []PlayMode {
	switch g {
	case PlayGroupSingle:
		return []PlayMode{PlayModeSingle}
	case PlayGroupMulti:
		return []PlayMode{PlayModeMultiLocal, PlayModeMultiOnline}
	case PlayGroupCoop:
		return []PlayMode{PlayModeCoopLocal, PlayModeCoopOnline}
	}
	return nil
}
