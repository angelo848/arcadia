package ipc_test

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

// TestPlatformContract checks the phase 9 platform types against the contract v0.11.0
// (PLT-011..PLT-021).
func TestPlatformContract(t *testing.T) {
	t.Parallel()
	c := loadContract(t)

	t.Run("PLT-014/Platform tem exatamente os campos do schema (seedKey, iconPath, gameCount)", func(t *testing.T) {
		t.Parallel()
		schema := c.schema(t, "Platform")
		for _, f := range []string{"seedKey", "iconPath", "gameCount"} {
			if _, ok := schema.Properties[f]; !ok {
				t.Errorf("contract Platform has no %s", f)
			}
		}
		assertExactFields(t, "Platform", schema, ipc.Platform{})
		assertExactFields(t, "PlatformResult", c.schema(t, "PlatformResult"), ipc.PlatformResult{})
	})

	t.Run("PLT-014/seedKey e iconPath nulos codificam null", func(t *testing.T) {
		t.Parallel()
		var m map[string]json.RawMessage
		if err := json.Unmarshal(mustMarshal(t, ipc.Platform{ID: 1, Name: "X", Category: "pc"}), &m); err != nil {
			t.Fatalf("decode: %v", err)
		}
		for name, want := range map[string]string{"seedKey": "null", "iconPath": "null", "emulatorId": "null", "gameCount": "0"} {
			if got := compactJSON(t, m[name]); got != want {
				t.Errorf("Platform.%s = %s, want %s", name, got, want)
			}
		}
		key, icon := "windows", `C:\data\media\platforms\a.png`
		m = nil
		if err := json.Unmarshal(mustMarshal(t, ipc.Platform{ID: 1, SeedKey: &key, IconPath: &icon, GameCount: 4}), &m); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if got := compactJSON(t, m["seedKey"]); got != `"windows"` {
			t.Errorf("Platform.seedKey = %s", got)
		}
		if got := compactJSON(t, m["gameCount"]); got != "4" {
			t.Errorf("Platform.gameCount = %s", got)
		}
	})

	t.Run("PLT-016/platform.add result tem platform e created", func(t *testing.T) {
		t.Parallel()
		schema := c.resultSchema(t, ipc.MethodPlatformAdd)
		if _, ok := schema.Properties["created"]; !ok {
			t.Error("contract platform.add result has no created")
		}
		assertExactFields(t, "platform.add result", schema, ipc.PlatformAddResult{})
		if got := jsonFieldNames(t, ipc.PlatformAddResult{}); !slices.Equal(got, []string{"created", "platform"}) {
			t.Errorf("PlatformAddResult JSON fields = %v, want [created platform]", got)
		}
	})

	t.Run("PLT-018/platform.update params id, name, category", func(t *testing.T) {
		t.Parallel()
		if ipc.MethodPlatformUpdate != "platform.update" {
			t.Fatalf("MethodPlatformUpdate = %q", ipc.MethodPlatformUpdate)
		}
		all, required := c.paramNames(t, ipc.MethodPlatformUpdate)
		want := []string{"category", "id", "name"}
		if !slices.Equal(all, want) || !slices.Equal(required, want) {
			t.Errorf("contract platform.update params = %v (required %v), want %v all required", all, required, want)
		}
		if got := jsonFieldNames(t, ipc.PlatformUpdateParams{}); !slices.Equal(got, want) {
			t.Errorf("PlatformUpdateParams JSON fields = %v, want %v", got, want)
		}
		assertExactFields(t, "platform.update result", c.resultSchema(t, ipc.MethodPlatformUpdate), ipc.PlatformResult{})
	})

	t.Run("PLT-011/platform.setIcon params platformId e path; reasons do contrato iguais às constantes", func(t *testing.T) {
		t.Parallel()
		if ipc.MethodPlatformSetIcon != "platform.setIcon" {
			t.Fatalf("MethodPlatformSetIcon = %q", ipc.MethodPlatformSetIcon)
		}
		all, required := c.paramNames(t, ipc.MethodPlatformSetIcon)
		want := []string{"path", "platformId"}
		if !slices.Equal(all, want) || !slices.Equal(required, want) {
			t.Errorf("contract platform.setIcon params = %v (required %v), want %v all required", all, required, want)
		}
		if got := jsonFieldNames(t, ipc.PlatformSetIconParams{}); !slices.Equal(got, want) {
			t.Errorf("PlatformSetIconParams JSON fields = %v, want %v", got, want)
		}
		assertExactFields(t, "platform.setIcon result", c.resultSchema(t, ipc.MethodPlatformSetIcon), ipc.PlatformResult{})

		// PLT-012: path may be null (back to the default icon): Optional distinguishes it from absent.
		var p ipc.PlatformSetIconParams
		if err := ipc.DecodeParams(json.RawMessage(`{"platformId":2,"path":null}`), &p); err != nil {
			t.Fatalf("decode null path: %v", err)
		}
		if p.PlatformID != 2 || !p.Path.Present || !p.Path.Null {
			t.Errorf("params with path null = %+v, want PlatformID 2 and Path present and null", p)
		}
		p = ipc.PlatformSetIconParams{}
		if err := ipc.DecodeParams(json.RawMessage(`{"platformId":2,"path":"C:\\i.png"}`), &p); err != nil {
			t.Fatalf("decode path: %v", err)
		}
		if !p.Path.Present || p.Path.Null || p.Path.Value != `C:\i.png` {
			t.Errorf("params with a path = %+v", p)
		}
		p = ipc.PlatformSetIconParams{}
		if err := ipc.DecodeParams(json.RawMessage(`{"platformId":2}`), &p); err != nil || p.Path.Present {
			t.Errorf("params without path = %+v (err %v), want Path absent", p, err)
		}

		// The reason enum lives in the description of InvalidParams (contract v0.11.0): the
		// five names, exactly the constants.
		wantReasons := []string{"not-absolute", "not-found", "not-a-file", "too-large", "unsupported"}
		gotConsts := []string{ipc.IconReasonNotAbsolute, ipc.IconReasonNotFound, ipc.IconReasonNotAFile, ipc.IconReasonTooLarge, ipc.IconReasonUnsupported}
		if !slices.Equal(gotConsts, wantReasons) {
			t.Errorf("IconReason* = %v, want %v", gotConsts, wantReasons)
		}
		desc := c.Components.Errors["InvalidParams"].Description
		for _, r := range wantReasons {
			if !containsWord(desc, r) {
				t.Errorf("contract InvalidParams description does not name reason %q: %s", r, desc)
			}
		}
		if got := jsonFieldNames(t, ipc.PlatformIconErrorData{}); !slices.Equal(got, []string{"reason"}) {
			t.Errorf("PlatformIconErrorData JSON fields = %v, want [reason]", got)
		}
	})

	t.Run("PLT-020/platform.delete params e result {id}", func(t *testing.T) {
		t.Parallel()
		if ipc.MethodPlatformDelete != "platform.delete" {
			t.Fatalf("MethodPlatformDelete = %q", ipc.MethodPlatformDelete)
		}
		all, required := c.paramNames(t, ipc.MethodPlatformDelete)
		if !slices.Equal(all, []string{"id"}) || !slices.Equal(required, []string{"id"}) {
			t.Errorf("contract platform.delete params = %v (required %v), want [id] required", all, required)
		}
		if got := jsonFieldNames(t, ipc.PlatformDeleteParams{}); !slices.Equal(got, []string{"id"}) {
			t.Errorf("PlatformDeleteParams JSON fields = %v, want [id]", got)
		}
		assertExactFields(t, "platform.delete result", c.resultSchema(t, ipc.MethodPlatformDelete), ipc.PlatformDeletedResult{})
		if got := string(mustMarshal(t, ipc.PlatformDeletedResult{ID: 7})); got != `{"id":7}` {
			t.Errorf("PlatformDeletedResult JSON = %s, want {\"id\":7}", got)
		}
	})

	t.Run("PLT-021/PlatformInUse tem código -32013 e data {platformId, gameCount}", func(t *testing.T) {
		t.Parallel()
		if ipc.CodePlatformInUse != -32013 {
			t.Errorf("CodePlatformInUse = %d, want -32013", ipc.CodePlatformInUse)
		}
		e, ok := c.Components.Errors["PlatformInUse"]
		if !ok {
			t.Fatal("contract has no PlatformInUse error")
		}
		if e.Code != ipc.CodePlatformInUse {
			t.Errorf("contract code = %d, constant = %d", e.Code, ipc.CodePlatformInUse)
		}
		if e.Message != ipc.MsgPlatformInUse || ipc.MsgPlatformInUse != "Platform has games" {
			t.Errorf("contract message %q, MsgPlatformInUse %q, want \"Platform has games\"", e.Message, ipc.MsgPlatformInUse)
		}
		assertExactFields(t, "PlatformInUse data", c.errorData(t, "PlatformInUse"), ipc.PlatformInUseData{PlatformID: 1, GameCount: 2})

		// The wire error built with the constants carries the data.
		err := ipc.NewErrorData(ipc.CodePlatformInUse, ipc.MsgPlatformInUse, ipc.PlatformInUseData{PlatformID: 3, GameCount: 5})
		if err.Code != -32013 || err.Message != "Platform has games" {
			t.Errorf("error = %+v", err)
		}
		if got := compactJSON(t, err.Data); got != `{"platformId":3,"gameCount":5}` {
			t.Errorf("error data = %s, want {\"platformId\":3,\"gameCount\":5}", got)
		}

		// platform.delete lists the error; platform.update and platform.setIcon do not.
		for _, m := range c.Methods {
			if m.Name == ipc.MethodPlatformDelete {
				return
			}
		}
		t.Error("platform.delete not in contract")
	})
}

// containsWord reports whether s contains word as a substring.
func containsWord(s, word string) bool {
	for i := 0; i+len(word) <= len(s); i++ {
		if s[i:i+len(word)] == word {
			return true
		}
	}
	return false
}
