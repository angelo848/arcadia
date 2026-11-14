package main

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

func strPtr(s string) *string { return &s }

// TestPlatformIconURL covers the iconUrl of PlatformView (PLT-011).
func TestPlatformIconURL(t *testing.T) {
	t.Parallel()

	t.Run("PLT-011/iconUrl de iconPath dentro de data\\media é /media/platforms/...", func(t *testing.T) {
		t.Parallel()
		data := t.TempDir()
		icon := filepath.Join(data, "media", "platforms", "abc123.png")
		p := &gamePinger{results: map[string]any{ipc.MethodPlatformUpdate: ipc.PlatformResult{Platform: ipc.Platform{
			ID: 4, Name: "Switch", Category: "console", SeedKey: strPtr("switch"), IconPath: &icon, GameCount: 2,
		}}}}
		a := &App{client: p, log: discardLog(), dataRoot: data}

		got, err := a.PlatformUpdate(4, "Switch", "console")
		if err != nil {
			t.Fatalf("PlatformUpdate err = %v", err)
		}
		if want := mediaURLPrefix + "platforms/abc123.png"; got.IconURL != want {
			t.Errorf("IconURL = %q, want %q", got.IconURL, want)
		}
		if got.GameCount != 2 || got.SeedKey == nil || *got.SeedKey != "switch" {
			t.Errorf("view = %+v, want seedKey switch and gameCount 2", got)
		}
	})

	t.Run("PLT-011/iconPath fora de data\\media vira iconUrl vazio", func(t *testing.T) {
		t.Parallel()
		data := t.TempDir()
		outside := filepath.Join(t.TempDir(), "icon.png")
		p := &gamePinger{results: map[string]any{ipc.MethodPlatformUpdate: ipc.PlatformResult{Platform: ipc.Platform{
			ID: 4, Name: "Switch", Category: "console", IconPath: &outside,
		}}}}
		a := &App{client: p, log: discardLog(), dataRoot: data}

		got, err := a.PlatformUpdate(4, "Switch", "console")
		if err != nil {
			t.Fatalf("PlatformUpdate err = %v", err)
		}
		if got.IconURL != "" {
			t.Errorf("IconURL = %q, want empty", got.IconURL)
		}
	})

	t.Run("PLT-011/sem iconPath o iconUrl é vazio e seedKey é null no JSON", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{results: map[string]any{ipc.MethodPlatformUpdate: ipc.PlatformResult{Platform: ipc.Platform{
			ID: 9, Name: "Minha", Category: "other",
		}}}}
		a := &App{client: p, log: discardLog(), dataRoot: t.TempDir()}

		got, err := a.PlatformUpdate(9, "Minha", "other")
		if err != nil {
			t.Fatalf("PlatformUpdate err = %v", err)
		}
		wantJSON(t, got, `{"id":9,"name":"Minha","category":"other","emulatorId":null,"seedKey":null,"iconUrl":"","gameCount":0}`)
	})
}

// TestPlatformSetIcon covers platform.setIcon params and the reason texts (PLT-012, PLT-013).
func TestPlatformSetIcon(t *testing.T) {
	t.Parallel()

	t.Run("PLT-013/PlatformSetIcon(nil) envia path null", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{results: map[string]any{ipc.MethodPlatformSetIcon: ipc.PlatformResult{Platform: ipc.Platform{ID: 4}}}}
		a := &App{client: p, log: discardLog()}

		if _, err := a.PlatformSetIcon(4, nil); err != nil {
			t.Fatalf("PlatformSetIcon err = %v", err)
		}
		c := singleCall(t, p, ipc.MethodPlatformSetIcon, libraryTimeout)
		m := rawObject(t, c)
		if len(m) != 2 || !sameJSON(t, m["platformId"], `4`) || !sameJSON(t, m["path"], `null`) {
			t.Errorf("params = %s, want platformId 4 and path null", c.params)
		}
	})

	t.Run("PLT-013/PlatformSetIcon com caminho envia o caminho", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{results: map[string]any{ipc.MethodPlatformSetIcon: ipc.PlatformResult{Platform: ipc.Platform{ID: 4}}}}
		a := &App{client: p, log: discardLog()}
		path := `C:\Icons\switch.png`

		if _, err := a.PlatformSetIcon(4, &path); err != nil {
			t.Fatalf("PlatformSetIcon err = %v", err)
		}
		m := rawObject(t, singleCall(t, p, ipc.MethodPlatformSetIcon, libraryTimeout))
		if !sameJSON(t, m["platformId"], `4`) || !sameJSON(t, m["path"], `"C:\\Icons\\switch.png"`) {
			t.Errorf("params = %v, want platformId 4 and the path", m)
		}
	})

	t.Run("PLT-012/PlatformSetIcon traduz cada reason para o texto", func(t *testing.T) {
		t.Parallel()
		cases := []struct{ reason, want string }{
			{ipc.IconReasonNotAbsolute, "Informe o caminho completo do arquivo."},
			{ipc.IconReasonNotFound, "Arquivo não encontrado."},
			{ipc.IconReasonNotAFile, "O caminho não é um arquivo."},
			{ipc.IconReasonTooLarge, "O arquivo passa de 2 MB."},
			{ipc.IconReasonUnsupported, "O arquivo não é uma imagem PNG ou JPEG."},
		}
		for _, tc := range cases {
			t.Run(tc.reason, func(t *testing.T) {
				t.Parallel()
				p := &gamePinger{errs: map[string]error{ipc.MethodPlatformSetIcon: ipc.NewErrorData(
					ipc.CodeInvalidParams, "Invalid params", ipc.PlatformIconErrorData{Reason: tc.reason})}}
				a := &App{client: p, log: discardLog()}
				path := `C:\x.png`

				_, err := a.PlatformSetIcon(4, &path)
				if err == nil || err.Error() != tc.want {
					t.Errorf("err = %v, want %q", err, tc.want)
				}
			})
		}
	})

	t.Run("PLT-012/reason desconhecido cai na tradução genérica", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{errs: map[string]error{ipc.MethodPlatformSetIcon: ipc.NewErrorData(
			ipc.CodeInvalidParams, "Invalid params", ipc.PlatformIconErrorData{Reason: "weird"})}}
		a := &App{client: p, log: discardLog()}
		path := `C:\x.png`

		_, err := a.PlatformSetIcon(4, &path)
		if err == nil || err.Error() == "" {
			t.Fatalf("err = %v, want a non-empty generic message", err)
		}
		for _, text := range []string{
			"Informe o caminho completo do arquivo.", "Arquivo não encontrado.", "O caminho não é um arquivo.",
			"O arquivo passa de 2 MB.", "O arquivo não é uma imagem PNG ou JPEG.",
		} {
			if err.Error() == text {
				t.Errorf("err = %q, an unknown reason must not map to a known text", err)
			}
		}
	})
}

// TestAddPlatformCreated covers PlatformAddView (PLT-016).
func TestAddPlatformCreated(t *testing.T) {
	t.Parallel()

	for _, created := range []bool{true, false} {
		name := "PLT-016/AddPlatform devolve created=false quando já existia"
		if created {
			name = "PLT-016/AddPlatform devolve created=true quando criou"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			p := &gamePinger{results: map[string]any{ipc.MethodPlatformAdd: ipc.PlatformAddResult{
				Platform: ipc.Platform{ID: 9, Name: "Arcade X", Category: "arcade"}, Created: created,
			}}}
			a := &App{client: p, log: discardLog(), dataRoot: t.TempDir()}

			got, err := a.AddPlatform("Arcade X", "arcade")
			if err != nil {
				t.Fatalf("AddPlatform err = %v", err)
			}
			singleCall(t, p, ipc.MethodPlatformAdd, libraryTimeout)
			if got.Created != created || got.Platform.ID != 9 {
				t.Errorf("AddPlatform() = %+v, want created %v and platform 9", got, created)
			}
			if m := jsonObject(t, got); len(m) != 2 || m["created"] != created {
				t.Errorf("JSON = %v, want exactly platform and created", m)
			}
		})
	}
}

// TestPlatformUpdate covers platform.update params (PLT-018, PLT-019).
func TestPlatformUpdate(t *testing.T) {
	t.Parallel()

	t.Run("PLT-018/PlatformUpdate chama platform.update com id, name, category", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{results: map[string]any{ipc.MethodPlatformUpdate: ipc.PlatformResult{Platform: ipc.Platform{
			ID: 4, Name: "Nintendo Switch", Category: "handheld"}}}}
		a := &App{client: p, log: discardLog()}

		got, err := a.PlatformUpdate(4, "Nintendo Switch", "handheld")
		if err != nil {
			t.Fatalf("PlatformUpdate err = %v", err)
		}
		c := singleCall(t, p, ipc.MethodPlatformUpdate, libraryTimeout)
		m := rawObject(t, c)
		if len(m) != 3 || !sameJSON(t, m["id"], `4`) || !sameJSON(t, m["name"], `"Nintendo Switch"`) ||
			!sameJSON(t, m["category"], `"handheld"`) {
			t.Errorf("params = %s, want id 4, name and category", c.params)
		}
		if got.Name != "Nintendo Switch" || got.Category != "handheld" {
			t.Errorf("PlatformUpdate() = %+v, want the platform returned by the agent", got)
		}
	})

	t.Run("PLT-019/erro do agente em platform.update volta como erro", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{errs: map[string]error{ipc.MethodPlatformUpdate: errors.New("boom")}}
		a := &App{client: p, log: discardLog()}
		if _, err := a.PlatformUpdate(4, "x", "pc"); err == nil {
			t.Error("PlatformUpdate err = nil, want an error")
		}
	})
}

// TestPlatformDelete covers platform.delete (PLT-020, PLT-021).
func TestPlatformDelete(t *testing.T) {
	t.Parallel()

	t.Run("PLT-020/PlatformDelete envia platform.delete com o id", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{results: map[string]any{ipc.MethodPlatformDelete: ipc.PlatformDeletedResult{ID: 4}}}
		a := &App{client: p, log: discardLog()}

		if err := a.PlatformDelete(4); err != nil {
			t.Fatalf("PlatformDelete err = %v", err)
		}
		c := singleCall(t, p, ipc.MethodPlatformDelete, libraryTimeout)
		if m := rawObject(t, c); len(m) != 1 || !sameJSON(t, m["id"], `4`) {
			t.Errorf("params = %s, want {\"id\":4}", c.params)
		}
	})

	t.Run("PLT-021/PlatformDelete com -32013 devolve a mensagem com N", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{errs: map[string]error{ipc.MethodPlatformDelete: ipc.NewErrorData(
			ipc.CodePlatformInUse, ipc.MsgPlatformInUse, ipc.PlatformInUseData{PlatformID: 4, GameCount: 3})}}
		a := &App{client: p, log: discardLog()}

		err := a.PlatformDelete(4)
		want := "Esta plataforma tem 3 jogo(s). Mova-os para outra plataforma ou exclua-os antes."
		if err == nil || err.Error() != want {
			t.Errorf("err = %v, want %q", err, want)
		}
	})
}

// TestPickPlatformIcon covers the icon dialog (PLT-023).
func TestPickPlatformIcon(t *testing.T) {
	t.Parallel()

	t.Run("PLT-023/PickPlatformIcon abre o seletor com o filtro de imagens", func(t *testing.T) {
		t.Parallel()
		calls := make(chan openFileCall, 1)
		a := &App{
			log:      discardLog(),
			wailsCtx: context.Background(),
			platform: platform{openFile: func(ctx context.Context, title string, filters []fileFilter) (string, error) {
				calls <- openFileCall{ctx, title, filters}
				return `C:\Icons\a.png`, nil
			}},
		}
		got, err := a.PickPlatformIcon()
		if err != nil || got != `C:\Icons\a.png` {
			t.Fatalf("PickPlatformIcon() = %q, %v", got, err)
		}
		select {
		case c := <-calls:
			if len(c.filters) != 1 || c.filters[0].DisplayName != "Imagens (*.png;*.jpg;*.jpeg)" ||
				c.filters[0].Pattern != "*.png;*.jpg;*.jpeg" {
				t.Errorf("filters = %+v, want Imagens (*.png;*.jpg;*.jpeg)", c.filters)
			}
		case <-time.After(time.Second):
			t.Fatal("the dialog was not opened")
		}
	})

	t.Run("PLT-023/cancelar devolve vazio", func(t *testing.T) {
		t.Parallel()
		a := &App{
			log:      discardLog(),
			wailsCtx: context.Background(),
			platform: platform{openFile: func(context.Context, string, []fileFilter) (string, error) { return "", nil }},
		}
		if got, err := a.PickPlatformIcon(); err != nil || got != "" {
			t.Errorf("PickPlatformIcon() = %q, %v; want empty and nil", got, err)
		}
	})

	t.Run("PLT-023/falha do diálogo devolve erro", func(t *testing.T) {
		t.Parallel()
		a := &App{
			log:      discardLog(),
			wailsCtx: context.Background(),
			platform: platform{openFile: func(context.Context, string, []fileFilter) (string, error) {
				return "", errors.New("boom")
			}},
		}
		if _, err := a.PickPlatformIcon(); err == nil {
			t.Error("PickPlatformIcon() err = nil, want an error")
		}
	})
}
