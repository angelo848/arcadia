package domain

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// MaxArgsTemplateRunes is the longest emulator argument template (EMU-001).
const MaxArgsTemplateRunes = 2000

// Argument markers an emulator template may use (EMU-001, EMU-004).
const (
	MarkerROM = "{rom}" // the game's launch target (the ROM path)
	MarkerDir = "{dir}" // the folder of the ROM
)

// ValidateArgsTemplate checks an emulator argument template (EMU-001): at most
// MaxArgsTemplateRunes runes, and braces only around the known markers. Any other
// {...} token, or an unclosed brace, is an error that names the valid markers. The
// error wraps ErrInvalidField.
func ValidateArgsTemplate(t string) error {
	if utf8.RuneCountInString(t) > MaxArgsTemplateRunes {
		return fmt.Errorf("args template longer than %d runes: %w", MaxArgsTemplateRunes, ErrInvalidField)
	}
	rest := t
	for {
		open := strings.IndexByte(rest, '{')
		if open < 0 {
			if strings.IndexByte(rest, '}') >= 0 {
				return fmt.Errorf("args template has a closing brace without marker: %w", ErrInvalidField)
			}
			return nil
		}
		end := strings.IndexByte(rest[open:], '}')
		if end < 0 {
			return fmt.Errorf("args template has an unclosed brace: %w", ErrInvalidField)
		}
		token := rest[open : open+end+1]
		if token != MarkerROM && token != MarkerDir {
			return fmt.Errorf("unknown marker %s; valid markers are %s and %s: %w",
				token, MarkerROM, MarkerDir, ErrInvalidField)
		}
		rest = rest[open+end+1:]
	}
}

// BuildArgs expands an argument template: {rom} becomes the quoted ROM path and {dir}
// the quoted folder. Other text is kept as written. The template must have passed
// ValidateArgsTemplate.
func BuildArgs(template, rom, dir string) string {
	out := strings.ReplaceAll(template, MarkerROM, quoteArg(rom))
	return strings.ReplaceAll(out, MarkerDir, quoteArg(dir))
}

// quoteArg wraps a value in double quotes when it has spaces, tabs or quotes, and
// escapes the embedded quotes with a backslash, as the Windows command line expects.
func quoteArg(s string) string {
	if !strings.ContainsAny(s, " \t\"") {
		return s
	}
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}
