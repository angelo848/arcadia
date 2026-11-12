package ipc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
)

// Optional distinguishes an absent member from null and from a value [IPC-052].
// Present is true when the member exists; Null is true when it is the null
// literal; Value only means something when Present && !Null.
type Optional[T any] struct {
	Present bool
	Null    bool
	Value   T
}

// UnmarshalJSON is called only for a member that exists. The null literal sets
// Present and Null; any other value is decoded into Value and sets Present. A
// value of the wrong type is an error (DecodeParams turns it into -32602).
func (o *Optional[T]) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		*o = Optional[T]{Present: true, Null: true}
		return nil
	}
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		return fmt.Errorf("ipc: optional member: %w", err)
	}
	*o = Optional[T]{Present: true, Value: v}
	return nil
}

// DecodeParams decodes named params into v. A nil raw yields nil and leaves v
// untouched; a non-object (including positional arrays), an unknown member, a
// member whose name differs in case from the json tag, a duplicate member, a
// wrong type or trailing data yield -32602 [IPC-028]. Required members are
// checked by the method handler.
func DecodeParams(raw json.RawMessage, v any) *Error {
	if raw == nil {
		return nil
	}
	trimmed := bytes.TrimLeft(raw, " \t\r\n")
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return NewError(CodeInvalidParams, msgInvalidParams)
	}
	if !checkMemberNames(trimmed, jsonFieldNames(v)) {
		return NewError(CodeInvalidParams, msgInvalidParams)
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return NewError(CodeInvalidParams, msgInvalidParams)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return NewError(CodeInvalidParams, msgInvalidParams)
	}
	return nil
}

// checkMemberNames scans the top-level members of the object in data and
// reports whether every member name is an exact entry of names and appears
// only once. A nil names set skips the check (v is not a struct).
func checkMemberNames(data []byte, names map[string]struct{}) bool {
	if names == nil {
		return true
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return false
	}
	seen := make(map[string]struct{})
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return false
		}
		key, ok := tok.(string)
		if !ok {
			return false
		}
		if _, ok := names[key]; !ok {
			return false
		}
		if _, dup := seen[key]; dup {
			return false
		}
		seen[key] = struct{}{}
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			return false
		}
	}
	return true
}

// jsonFieldNames returns the set of member names encoding/json maps to the
// struct pointed to by v, or nil when v is not a (pointer to a) struct.
func jsonFieldNames(v any) map[string]struct{} {
	t := reflect.TypeOf(v)
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == nil || t.Kind() != reflect.Struct {
		return nil
	}
	names := make(map[string]struct{})
	collectFieldNames(t, names, map[reflect.Type]bool{})
	return names
}

// collectFieldNames adds the json member names of t's fields to names,
// honoring "-", tag options and untagged embedded structs.
func collectFieldNames(t reflect.Type, names map[string]struct{}, visited map[reflect.Type]bool) {
	if visited[t] {
		return
	}
	visited[t] = true
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name, _, _ := strings.Cut(tag, ",")
		if f.Anonymous && name == "" {
			ft := f.Type
			if ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}
			if ft.Kind() == reflect.Struct {
				collectFieldNames(ft, names, visited)
				continue
			}
		}
		if !f.IsExported() {
			continue
		}
		if name == "" {
			name = f.Name
		}
		names[name] = struct{}{}
	}
}

// NoParams wraps a handler for a method without params: any non-empty params
// (the dispatcher already normalized empty ones to nil) yield -32602.
func NoParams(fn func(ctx context.Context) (any, error)) Handler {
	return func(ctx context.Context, params json.RawMessage) (any, error) {
		if len(params) != 0 {
			return nil, NewError(CodeInvalidParams, msgInvalidParams)
		}
		return fn(ctx)
	}
}
