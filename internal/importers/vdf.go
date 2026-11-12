// Package importers reads the game libraries of other launchers (phase 6). Every
// source only reads: it returns candidates and warnings and never writes (IMP-001,
// IMP-009). The agent decides what to store.
package importers

import (
	"fmt"
	"strings"
)

// vdfEntry is one key of a Valve KeyValues object: a string value or a nested object.
type vdfEntry struct {
	Key    string
	Value  string
	Object vdfObject
	IsObj  bool
}

// vdfObject is an ordered list of entries (keys may repeat).
type vdfObject []vdfEntry

// parseVDF parses the text KeyValues format used by libraryfolders.vdf and
// appmanifest_*.acf: quoted keys and values, nested objects in braces, // comments.
func parseVDF(data []byte) (vdfObject, error) {
	p := &vdfParser{src: strings.TrimPrefix(string(data), "\xef\xbb\xbf")}
	return p.object(false)
}

type vdfParser struct {
	src string
	pos int
}

func (p *vdfParser) skip() {
	for p.pos < len(p.src) {
		c := p.src[p.pos]
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
			p.pos++
		case c == '/' && p.pos+1 < len(p.src) && p.src[p.pos+1] == '/':
			for p.pos < len(p.src) && p.src[p.pos] != '\n' {
				p.pos++
			}
		default:
			return
		}
	}
}

// quoted reads one quoted string and returns it unescaped.
func (p *vdfParser) quoted() (string, error) {
	p.pos++ // opening quote
	var b strings.Builder
	for p.pos < len(p.src) {
		c := p.src[p.pos]
		p.pos++
		if c == '"' {
			return b.String(), nil
		}
		if c == '\\' && p.pos < len(p.src) {
			esc := p.src[p.pos]
			p.pos++
			switch esc {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			default: // \\ and \" and any other: the character itself
				b.WriteByte(esc)
			}
			continue
		}
		b.WriteByte(c)
	}
	return "", fmt.Errorf("vdf: unterminated string at byte %d", p.pos)
}

// object reads entries until the end of the input (top level) or a closing brace.
func (p *vdfParser) object(nested bool) (vdfObject, error) {
	var out vdfObject
	for {
		p.skip()
		if p.pos >= len(p.src) {
			if nested {
				return nil, fmt.Errorf("vdf: missing closing brace")
			}
			return out, nil
		}
		if p.src[p.pos] == '}' {
			if !nested {
				return nil, fmt.Errorf("vdf: unexpected closing brace at byte %d", p.pos)
			}
			p.pos++
			return out, nil
		}
		if p.src[p.pos] != '"' {
			return nil, fmt.Errorf("vdf: expected a quoted key at byte %d", p.pos)
		}
		key, err := p.quoted()
		if err != nil {
			return nil, err
		}
		p.skip()
		if p.pos < len(p.src) && p.src[p.pos] == '{' {
			p.pos++
			child, err := p.object(true)
			if err != nil {
				return nil, err
			}
			out = append(out, vdfEntry{Key: key, Object: child, IsObj: true})
			continue
		}
		if p.pos >= len(p.src) || p.src[p.pos] != '"' {
			return nil, fmt.Errorf("vdf: missing value for %q at byte %d", key, p.pos)
		}
		val, err := p.quoted()
		if err != nil {
			return nil, err
		}
		out = append(out, vdfEntry{Key: key, Value: val})
	}
}

// get returns the first string value of key (keys compare without regard to case).
func (o vdfObject) get(key string) (string, bool) {
	for _, e := range o {
		if !e.IsObj && strings.EqualFold(e.Key, key) {
			return e.Value, true
		}
	}
	return "", false
}

// child returns the first nested object of key.
func (o vdfObject) child(key string) (vdfObject, bool) {
	for _, e := range o {
		if e.IsObj && strings.EqualFold(e.Key, key) {
			return e.Object, true
		}
	}
	return nil, false
}
