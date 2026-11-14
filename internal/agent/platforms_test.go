package agent

// Tests of T11 (docs/specs/phase-9-plataformas/tasks.md): platform.list, platform.add,
// library.catalog, platform.update, platform.setIcon, platform.delete and
// platform.setEmulator [PLT-011..PLT-024], called through ipc.Dispatch with a real
// store in t.TempDir() and a temporary media folder.

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
	"github.com/arcadia-launcher/arcadia/internal/media"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

// platformNotFound is the -32001 of the contract for an unknown platform.
const platformNotFound = -32001

type platHarness struct {
	dir       string // scratch folder for source files
	mediaRoot string
	st        *store.Store
	handlers  map[string]ipc.Handler
}

func newPlatHarness(t *testing.T) *platHarness {
	t.Helper()
	root := t.TempDir()
	st, err := store.Open(t.Context(), filepath.Join(root, "library.db"), store.OpenOptions{Migrate: true})
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	h := &platHarness{dir: filepath.Join(root, "src"), mediaRoot: filepath.Join(root, "media"), st: st}
	if err := os.MkdirAll(h.dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	h.handlers = platformHandlers(newPlatformService(st, media.New(h.mediaRoot, nil), h.mediaRoot, os.Stat, quietLog()))
	for _, m := range []string{ipc.MethodPlatformList, ipc.MethodPlatformAdd, ipc.MethodLibraryCatalog,
		ipc.MethodPlatformUpdate, ipc.MethodPlatformSetIcon, ipc.MethodPlatformDelete, ipc.MethodPlatformSetEmulator} {
		if h.handlers[m] == nil {
			t.Fatalf("platformHandlers has no handler for %q", m)
		}
	}
	return h
}

func (h *platHarness) call(t *testing.T, method string, params any) ipc.Response {
	t.Helper()
	p := ""
	if params != nil {
		p = mustJSON(t, params)
	}
	return callHandlers(t, t.Context(), h.handlers, method, p)
}

func (h *platHarness) add(t *testing.T, name, category string) ipc.PlatformAddResult {
	t.Helper()
	var out ipc.PlatformAddResult
	resultInto(t, h.call(t, ipc.MethodPlatformAdd, map[string]string{"name": name, "category": category}), &out)
	return out
}

func (h *platHarness) list(t *testing.T) []ipc.Platform {
	t.Helper()
	var out ipc.PlatformListResult
	resultInto(t, h.call(t, ipc.MethodPlatformList, nil), &out)
	return out.Platforms
}

// find returns the platform with id from platform.list; ok is false when absent.
func (h *platHarness) find(t *testing.T, id int64) (ipc.Platform, bool) {
	t.Helper()
	for _, p := range h.list(t) {
		if p.ID == id {
			return p, true
		}
	}
	return ipc.Platform{}, false
}

func (h *platHarness) must(t *testing.T, id int64) ipc.Platform {
	t.Helper()
	p, ok := h.find(t, id)
	if !ok {
		t.Fatalf("platform %d not in platform.list", id)
	}
	return p
}

// setIcon calls platform.setIcon with path (nil = JSON null) and returns the response.
func (h *platHarness) setIcon(t *testing.T, id int64, path *string) ipc.Response {
	t.Helper()
	var p any
	if path != nil {
		p = *path
	}
	return h.call(t, ipc.MethodPlatformSetIcon, map[string]any{"platformId": id, "path": p})
}

func (h *platHarness) setIconOK(t *testing.T, id int64, path *string) ipc.Platform {
	t.Helper()
	var out ipc.PlatformResult
	resultInto(t, h.setIcon(t, id, path), &out)
	return out.Platform
}

// iconFiles lists the files under <media>\platforms (empty when the folder does not exist).
func (h *platHarness) iconFiles(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(h.mediaRoot, "platforms"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

// pngFile writes a valid PNG whose content depends on seed (so its hash does) and returns its path.
func (h *platHarness) pngFile(t *testing.T, name string, seed uint8) *string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	img.Set(0, 0, color.RGBA{seed, 40, 200, 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png: %v", err)
	}
	return h.raw(t, name, buf.Bytes())
}

func (h *platHarness) jpegFile(t *testing.T, name string) *string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("jpeg: %v", err)
	}
	return h.raw(t, name, buf.Bytes())
}

func (h *platHarness) raw(t *testing.T, name string, data []byte) *string {
	t.Helper()
	p := filepath.Join(h.dir, name)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
	return &p
}

// wantIconInside checks that p.IconPath is an absolute path of an existing file
// directly inside <media>\platforms.
func (h *platHarness) wantIconInside(t *testing.T, p ipc.Platform) string {
	t.Helper()
	if p.IconPath == nil {
		t.Fatalf("iconPath = null, want a file inside %s", filepath.Join(h.mediaRoot, "platforms"))
	}
	ip := *p.IconPath
	if !filepath.IsAbs(ip) {
		t.Errorf("iconPath %q is not absolute", ip)
	}
	if filepath.Dir(ip) != filepath.Join(h.mediaRoot, "platforms") {
		t.Errorf("iconPath %q is not directly inside %s", ip, filepath.Join(h.mediaRoot, "platforms"))
	}
	if _, err := os.Stat(ip); err != nil {
		t.Errorf("icon file: %v", err)
	}
	return ip
}

// gameOn adds a game and moves it to platformID; hidden marks it hidden straight in the table.
func (h *platHarness) gameOn(t *testing.T, platformID int64, title string, hidden bool) int64 {
	t.Helper()
	at := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	g, err := h.st.AddGame(t.Context(), store.NewGame{Title: title, Exe: filepath.Join(h.dir, title+".exe"), At: at})
	if err != nil {
		t.Fatalf("AddGame: %v", err)
	}
	if _, err := h.st.UpdateGame(t.Context(), g.ID, store.GamePatch{PlatformID: &platformID}, at); err != nil {
		t.Fatalf("UpdateGame: %v", err)
	}
	if hidden {
		if _, err := h.st.DB().ExecContext(t.Context(), `UPDATE game SET hidden = 1 WHERE id = ?`, g.ID); err != nil {
			t.Fatalf("hide game: %v", err)
		}
	}
	return g.ID
}

func TestPlatformHandlers(t *testing.T) {
	t.Parallel()

	t.Run("PLT-014/platform.list e library.catalog trazem seedKey, iconPath absoluto e gameCount", func(t *testing.T) {
		t.Parallel()
		h := newPlatHarness(t)
		user := h.add(t, "Amiga", "console").Platform
		h.gameOn(t, user.ID, "A", false)
		h.gameOn(t, user.ID, "B", true) // hidden games count
		withIcon := h.setIconOK(t, user.ID, h.pngFile(t, "a.png", 1))

		resp := h.call(t, ipc.MethodPlatformList, nil)
		if resp.Error != nil {
			t.Fatalf("platform.list: %d %q", resp.Error.Code, resp.Error.Message)
		}
		items := objectOf(t, resp.Result)
		var raws []map[string]json.RawMessage
		if err := json.Unmarshal(items["platforms"], &raws); err != nil {
			t.Fatalf("platforms: %v", err)
		}
		for _, r := range raws {
			wantKeys(t, r, "id", "name", "category", "emulatorId", "seedKey", "iconPath", "gameCount")
		}

		got := h.must(t, user.ID)
		if got.SeedKey != nil || got.GameCount != 2 {
			t.Errorf("user platform = %+v, want seedKey null and gameCount 2", got)
		}
		h.wantIconInside(t, got)
		if got.IconPath == nil || withIcon.IconPath == nil || *got.IconPath != *withIcon.IconPath {
			t.Errorf("list iconPath = %v, want the one setIcon returned (%v)", got.IconPath, withIcon.IconPath)
		}
		win := domain.SeedKeyWindows
		for _, p := range h.list(t) {
			if p.Name == "Windows" && (p.SeedKey == nil || *p.SeedKey != win || p.IconPath != nil) {
				t.Errorf("Windows = %+v, want seedKey windows and iconPath null", p)
			}
		}

		cat := h.call(t, ipc.MethodLibraryCatalog, nil)
		if cat.Error != nil {
			t.Fatalf("library.catalog: %d %q", cat.Error.Code, cat.Error.Message)
		}
		wantRaw(t, objectOf(t, cat.Result), "platforms", string(items["platforms"]))
	})

	t.Run("PLT-014/toIPCPlatform junta a pasta de mídia ao ícone; seedKey e ícone vazios viram null", func(t *testing.T) {
		t.Parallel()
		root := filepath.Join(t.TempDir(), "media")
		got := toIPCPlatform(store.Platform{ID: 3, Name: "X", Category: domain.CategoryArcade, SeedKey: "n64",
			Icon: filepath.Join("platforms", "abc.png"), GameCount: 4}, root)
		if got.IconPath == nil || *got.IconPath != filepath.Join(root, "platforms", "abc.png") {
			t.Errorf("iconPath = %v, want %s", got.IconPath, filepath.Join(root, "platforms", "abc.png"))
		}
		if got.SeedKey == nil || *got.SeedKey != "n64" || got.GameCount != 4 || got.Category != "arcade" {
			t.Errorf("platform = %+v", got)
		}
		bare := toIPCPlatform(store.Platform{ID: 4, Name: "Y", Category: domain.CategoryPC}, root)
		if bare.SeedKey != nil || bare.IconPath != nil {
			t.Errorf("platform = %+v, want seedKey and iconPath null", bare)
		}
	})

	t.Run("PLT-016/platform.add de nome existente devolve created false com a plataforma existente", func(t *testing.T) {
		t.Parallel()
		h := newPlatHarness(t)
		first := h.add(t, "Amiga", "console")
		if !first.Created {
			t.Fatalf("first add created = false, want true")
		}
		before := len(h.list(t))
		again := h.add(t, "  amiga ", "arcade")
		if again.Created || again.Platform.ID != first.Platform.ID || again.Platform.Name != "Amiga" || again.Platform.Category != "console" {
			t.Errorf("second add = %+v, want the existing Amiga console with created false", again)
		}
		if after := len(h.list(t)); after != before {
			t.Errorf("platforms = %d, want %d: nothing is created", after, before)
		}
		// A seed name that already exists in the database resolves to it too.
		win := h.add(t, "windows", "console")
		if win.Created || win.Platform.Name != "Windows" {
			t.Errorf("add(windows) = %+v, want the existing Windows with created false", win)
		}
	})

	t.Run("PLT-017/platform.add com nome canônico de chave livre devolve seedKey e categoria do seed", func(t *testing.T) {
		t.Parallel()
		h := newPlatHarness(t)
		var wii ipc.Platform
		for _, p := range h.list(t) {
			if p.SeedKey != nil && *p.SeedKey == "wii" {
				wii = p
			}
		}
		if wii.ID == 0 {
			t.Fatal("seed platform wii not found")
		}
		resultInto(t, h.call(t, ipc.MethodPlatformDelete, map[string]int64{"id": wii.ID}), &ipc.PlatformDeletedResult{})
		got := h.add(t, "nintendo wii", "arcade")
		if !got.Created || got.Platform.SeedKey == nil || *got.Platform.SeedKey != "wii" ||
			got.Platform.Category != "console" || got.Platform.Name != "Nintendo Wii" {
			t.Errorf("add = %+v, want created, seedKey wii, category console (the seed's) and the canonical name", got)
		}
	})

	t.Run("PLT-018/platform.update renomeia; nome de outra plataforma é -32602 e nada muda", func(t *testing.T) {
		t.Parallel()
		h := newPlatHarness(t)
		p := h.add(t, "Amiga", "console").Platform
		other := h.add(t, "Atari", "console").Platform
		emu, err := h.st.AddEmulator(t.Context(), "Emu", filepath.Join(h.dir, "e.exe"), "{rom}")
		if err != nil {
			t.Fatalf("AddEmulator: %v", err)
		}
		if err := h.st.SetPlatformEmulator(t.Context(), p.ID, &emu.ID); err != nil {
			t.Fatalf("SetPlatformEmulator: %v", err)
		}
		h.setIconOK(t, p.ID, h.pngFile(t, "i.png", 2))
		gid := h.gameOn(t, p.ID, "G", false)
		before := h.must(t, p.ID)

		var out ipc.PlatformResult
		resultInto(t, h.call(t, ipc.MethodPlatformUpdate, map[string]any{"id": p.ID, "name": "  Amiga 500 ", "category": "handheld"}), &out)
		if out.Platform.Name != "Amiga 500" || out.Platform.Category != "handheld" || out.Platform.ID != p.ID {
			t.Errorf("update = %+v, want Amiga 500 handheld", out.Platform)
		}
		if out.Platform.EmulatorID == nil || *out.Platform.EmulatorID != emu.ID ||
			out.Platform.IconPath == nil || before.IconPath == nil || *out.Platform.IconPath != *before.IconPath ||
			out.Platform.GameCount != 1 {
			t.Errorf("update = %+v, want the emulator, icon and game kept (before %+v)", out.Platform, before)
		}
		if g, err := h.st.Game(t.Context(), gid); err != nil || g.PlatformID != p.ID {
			t.Errorf("game = %+v, %v; want it still on platform %d", g, err, p.ID)
		}

		// A change of case only is accepted.
		resultInto(t, h.call(t, ipc.MethodPlatformUpdate, map[string]any{"id": p.ID, "name": "AMIGA 500", "category": "handheld"}), &out)
		if out.Platform.Name != "AMIGA 500" {
			t.Errorf("name = %q, want AMIGA 500", out.Platform.Name)
		}

		// The name of another platform, in any case, is refused and nothing changes.
		snapshot := h.call(t, ipc.MethodPlatformList, nil).Result
		for _, name := range []string{other.Name, strings.ToLower(other.Name), "  " + other.Name + "  "} {
			resp := h.call(t, ipc.MethodPlatformUpdate, map[string]any{"id": p.ID, "name": name, "category": "console"})
			wantError(t, resp, ipc.CodeInvalidParams, "")
		}
		if after := h.call(t, ipc.MethodPlatformList, nil).Result; string(after) != string(snapshot) {
			t.Errorf("platforms after the refused updates = %s, want %s", after, snapshot)
		}
	})

	t.Run("PLT-018/platform.update de id inexistente é -32001", func(t *testing.T) {
		t.Parallel()
		h := newPlatHarness(t)
		wantError(t, h.call(t, ipc.MethodPlatformUpdate, map[string]any{"id": 999999, "name": "X", "category": "pc"}), platformNotFound, "")
	})

	t.Run("PLT-018/platform.update recusa nome vazio ou longo demais (DOM-015) com -32602", func(t *testing.T) {
		t.Parallel()
		h := newPlatHarness(t)
		p := h.add(t, "Amiga", "console").Platform
		before := h.call(t, ipc.MethodPlatformList, nil).Result
		for _, name := range []string{"", "   ", strings.Repeat("x", domain.MaxNameRunes+1)} {
			wantError(t, h.call(t, ipc.MethodPlatformUpdate, map[string]any{"id": p.ID, "name": name, "category": "console"}), ipc.CodeInvalidParams, "")
		}
		if after := h.call(t, ipc.MethodPlatformList, nil).Result; string(after) != string(before) {
			t.Errorf("platforms changed after refused updates: %s", after)
		}
	})

	t.Run("PLT-018/params de platform.update: membro desconhecido ou de tipo errado é -32602", func(t *testing.T) {
		t.Parallel()
		h := newPlatHarness(t)
		for _, p := range []string{`{"id":1,"name":"X","category":"pc","extra":1}`, `{"id":"1","name":"X","category":"pc"}`,
			`{"name":"X","category":"pc"}`, `[1,"X","pc"]`} {
			wantError(t, callHandlers(t, t.Context(), h.handlers, ipc.MethodPlatformUpdate, p), ipc.CodeInvalidParams, "")
		}
	})

	t.Run("PLT-019/categoria fora da lista é -32602 e nada muda", func(t *testing.T) {
		t.Parallel()
		h := newPlatHarness(t)
		p := h.add(t, "Amiga", "console").Platform
		for _, c := range []string{"toaster", "", "PC", "Console"} {
			wantError(t, h.call(t, ipc.MethodPlatformUpdate, map[string]any{"id": p.ID, "name": "Amiga", "category": c}), ipc.CodeInvalidParams, "")
		}
		if got := h.must(t, p.ID); got.Category != "console" {
			t.Errorf("category = %q, want console unchanged", got.Category)
		}
		for _, c := range []string{"pc", "console", "arcade", "handheld"} {
			var out ipc.PlatformResult
			resultInto(t, h.call(t, ipc.MethodPlatformUpdate, map[string]any{"id": p.ID, "name": "Amiga", "category": c}), &out)
			if out.Platform.Category != c {
				t.Errorf("category = %q, want %q", out.Platform.Category, c)
			}
		}
	})

	t.Run("PLT-011/setIcon copia para platforms\\ e iconPath aponta para a cópia", func(t *testing.T) {
		t.Parallel()
		h := newPlatHarness(t)
		p := h.add(t, "Amiga", "console").Platform
		for _, src := range []*string{h.pngFile(t, "a.png", 3), h.jpegFile(t, "b.jpg")} {
			got := h.setIconOK(t, p.ID, src)
			ip := h.wantIconInside(t, got)
			if ip == *src {
				t.Errorf("iconPath = the source file; want a copy")
			}
			want, _ := os.ReadFile(*src)
			have, _ := os.ReadFile(ip)
			if !bytes.Equal(want, have) {
				t.Errorf("copy differs from the source")
			}
			// Deleting the original does not change the icon.
			if err := os.Remove(*src); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(ip); err != nil {
				t.Errorf("icon after removing the source: %v", err)
			}
			if again := h.must(t, p.ID); again.IconPath == nil || *again.IconPath != ip {
				t.Errorf("list iconPath = %v, want %s", again.IconPath, ip)
			}
		}
	})

	t.Run("PLT-011/setIcon de plataforma inexistente é -32001 e não deixa arquivo", func(t *testing.T) {
		t.Parallel()
		h := newPlatHarness(t)
		wantError(t, h.setIcon(t, 999999, h.pngFile(t, "a.png", 4)), platformNotFound, "")
		if files := h.iconFiles(t); len(files) != 0 {
			t.Errorf("files in platforms\\ = %v, want none", files)
		}
	})

	t.Run("PLT-012/setIcon recusa não absoluto, inexistente, pasta, acima de 2 MB e não imagem com data.reason; nada criado, ícone igual", func(t *testing.T) {
		t.Parallel()
		h := newPlatHarness(t)
		p := h.add(t, "Amiga", "console").Platform
		good := h.setIconOK(t, p.ID, h.pngFile(t, "good.png", 5))
		filesBefore := h.iconFiles(t)

		big := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, media.MaxPlatformIconBytes)...)
		folder := filepath.Join(h.dir, "folder")
		if err := os.Mkdir(folder, 0o755); err != nil {
			t.Fatal(err)
		}
		cases := []struct {
			name, reason string
			path         string
		}{
			{"relativo", ipc.IconReasonNotAbsolute, filepath.Join("rel", "icon.png")},
			{"inexistente", ipc.IconReasonNotFound, filepath.Join(h.dir, "nope.png")},
			{"pasta", ipc.IconReasonNotAFile, folder},
			{"grande", ipc.IconReasonTooLarge, *h.raw(t, "big.png", big)},
			{"texto com extensão png", ipc.IconReasonUnsupported, *h.raw(t, "text.png", []byte("not an image at all"))},
			{"gif", ipc.IconReasonUnsupported, *h.raw(t, "a.gif", []byte("GIF89a\x01\x00\x01\x00"))},
		}
		for _, tc := range cases {
			data := wantError(t, h.setIcon(t, p.ID, ptr(tc.path)), ipc.CodeInvalidParams, "")
			var reason string
			if err := json.Unmarshal(data["reason"], &reason); err != nil || reason != tc.reason {
				t.Errorf("%s: data.reason = %s, want %q", tc.name, data["reason"], tc.reason)
			}
		}
		if got := h.must(t, p.ID); got.IconPath == nil || *got.IconPath != *good.IconPath {
			t.Errorf("iconPath = %v, want %s unchanged", got.IconPath, *good.IconPath)
		}
		if after := h.iconFiles(t); strings.Join(after, ",") != strings.Join(filesBefore, ",") {
			t.Errorf("files in platforms\\ = %v, want %v", after, filesBefore)
		}
	})

	t.Run("PLT-012/setIcon recusado numa plataforma sem ícone não cria a pasta platforms", func(t *testing.T) {
		t.Parallel()
		h := newPlatHarness(t)
		p := h.add(t, "Amiga", "console").Platform
		wantError(t, h.setIcon(t, p.ID, h.raw(t, "text.png", []byte("hello"))), ipc.CodeInvalidParams, "")
		if files := h.iconFiles(t); len(files) != 0 {
			t.Errorf("files in platforms\\ = %v, want none", files)
		}
		if got := h.must(t, p.ID); got.IconPath != nil {
			t.Errorf("iconPath = %v, want null", *got.IconPath)
		}
	})

	t.Run("PLT-013/setIcon null restaura o padrão e apaga o arquivo", func(t *testing.T) {
		t.Parallel()
		h := newPlatHarness(t)
		p := h.add(t, "Amiga", "console").Platform
		ip := h.wantIconInside(t, h.setIconOK(t, p.ID, h.pngFile(t, "a.png", 6)))
		got := h.setIconOK(t, p.ID, nil)
		if got.IconPath != nil {
			t.Errorf("iconPath = %v, want null", *got.IconPath)
		}
		if _, err := os.Stat(ip); !os.IsNotExist(err) {
			t.Errorf("icon file after restoring: err = %v, want it deleted", err)
		}
		// Restoring a platform that has no custom icon is a no-op, not an error.
		if again := h.setIconOK(t, p.ID, nil); again.IconPath != nil {
			t.Errorf("second restore iconPath = %v", *again.IconPath)
		}
	})

	t.Run("PLT-013/arquivo usado por duas plataformas não é apagado ao restaurar uma", func(t *testing.T) {
		t.Parallel()
		h := newPlatHarness(t)
		a := h.add(t, "Amiga", "console").Platform
		b := h.add(t, "Atari", "console").Platform
		src := h.pngFile(t, "shared.png", 7)
		ipA := h.wantIconInside(t, h.setIconOK(t, a.ID, src))
		ipB := h.wantIconInside(t, h.setIconOK(t, b.ID, src))
		if ipA != ipB {
			t.Fatalf("same content gave two files: %s and %s", ipA, ipB)
		}
		h.setIconOK(t, a.ID, nil)
		if _, err := os.Stat(ipA); err != nil {
			t.Errorf("shared file after restoring one platform: %v, want it kept", err)
		}
		if got := h.must(t, b.ID); got.IconPath == nil || *got.IconPath != ipB {
			t.Errorf("other platform iconPath = %v, want %s", got.IconPath, ipB)
		}
		h.setIconOK(t, b.ID, nil)
		if _, err := os.Stat(ipA); !os.IsNotExist(err) {
			t.Errorf("file after the last user restored: err = %v, want it deleted", err)
		}
	})

	t.Run("PLT-013/trocar um ícone por outro apaga o antigo", func(t *testing.T) {
		t.Parallel()
		h := newPlatHarness(t)
		p := h.add(t, "Amiga", "console").Platform
		oldIP := h.wantIconInside(t, h.setIconOK(t, p.ID, h.pngFile(t, "old.png", 8)))
		newIP := h.wantIconInside(t, h.setIconOK(t, p.ID, h.pngFile(t, "new.png", 9)))
		if oldIP == newIP {
			t.Fatalf("different contents gave the same file %s", oldIP)
		}
		if _, err := os.Stat(oldIP); !os.IsNotExist(err) {
			t.Errorf("old icon: err = %v, want it deleted", err)
		}
		if _, err := os.Stat(newIP); err != nil {
			t.Errorf("new icon: %v", err)
		}
		if files := h.iconFiles(t); len(files) != 1 {
			t.Errorf("files in platforms\\ = %v, want only the new icon", files)
		}
	})

	t.Run("PLT-013/trocar por um ícone usado por outra plataforma não apaga o antigo compartilhado", func(t *testing.T) {
		t.Parallel()
		h := newPlatHarness(t)
		a := h.add(t, "Amiga", "console").Platform
		b := h.add(t, "Atari", "console").Platform
		shared := h.wantIconInside(t, h.setIconOK(t, a.ID, h.pngFile(t, "s.png", 10)))
		h.setIconOK(t, b.ID, h.pngFile(t, "s2.png", 10)) // same content
		h.setIconOK(t, a.ID, h.pngFile(t, "other.png", 11))
		if _, err := os.Stat(shared); err != nil {
			t.Errorf("file still used by the other platform: %v, want it kept", err)
		}
	})

	t.Run("PLT-013/escolher o mesmo arquivo de novo não apaga o ícone", func(t *testing.T) {
		t.Parallel()
		h := newPlatHarness(t)
		p := h.add(t, "Amiga", "console").Platform
		src := h.pngFile(t, "same.png", 12)
		first := h.wantIconInside(t, h.setIconOK(t, p.ID, src))
		second := h.wantIconInside(t, h.setIconOK(t, p.ID, src))
		if first != second {
			t.Errorf("icon changed from %s to %s", first, second)
		}
	})

	t.Run("PLT-020/delete sem jogos apaga a plataforma e o ícone; emuladores ficam", func(t *testing.T) {
		t.Parallel()
		h := newPlatHarness(t)
		p := h.add(t, "Amiga", "console").Platform
		ip := h.wantIconInside(t, h.setIconOK(t, p.ID, h.pngFile(t, "a.png", 13)))
		emu, err := h.st.AddEmulator(t.Context(), "Emu", filepath.Join(h.dir, "e.exe"), "{rom}")
		if err != nil {
			t.Fatalf("AddEmulator: %v", err)
		}
		if err := h.st.SetPlatformEmulator(t.Context(), p.ID, &emu.ID); err != nil {
			t.Fatalf("SetPlatformEmulator: %v", err)
		}

		resp := h.call(t, ipc.MethodPlatformDelete, map[string]int64{"id": p.ID})
		var out ipc.PlatformDeletedResult
		resultInto(t, resp, &out)
		wantKeys(t, objectOf(t, resp.Result), "id")
		if out.ID != p.ID {
			t.Errorf("result id = %d, want %d", out.ID, p.ID)
		}
		if _, ok := h.find(t, p.ID); ok {
			t.Errorf("platform %d still listed", p.ID)
		}
		if _, err := os.Stat(ip); !os.IsNotExist(err) {
			t.Errorf("icon file after delete: err = %v, want it deleted", err)
		}
		emus, err := h.st.Emulators(t.Context())
		if err != nil || len(emus) != 1 || emus[0].ID != emu.ID {
			t.Errorf("emulators = %+v, %v; want the emulator kept", emus, err)
		}
	})

	t.Run("PLT-020/delete de plataforma com o ícone compartilhado não apaga o arquivo", func(t *testing.T) {
		t.Parallel()
		h := newPlatHarness(t)
		a := h.add(t, "Amiga", "console").Platform
		b := h.add(t, "Atari", "console").Platform
		src := h.pngFile(t, "s.png", 14)
		ip := h.wantIconInside(t, h.setIconOK(t, a.ID, src))
		h.setIconOK(t, b.ID, src)
		var out ipc.PlatformDeletedResult
		resultInto(t, h.call(t, ipc.MethodPlatformDelete, map[string]int64{"id": a.ID}), &out)
		if _, err := os.Stat(ip); err != nil {
			t.Errorf("shared icon after delete: %v, want it kept", err)
		}
	})

	t.Run("PLT-020/delete de id inexistente é -32001", func(t *testing.T) {
		t.Parallel()
		h := newPlatHarness(t)
		wantError(t, h.call(t, ipc.MethodPlatformDelete, map[string]int64{"id": 999999}), platformNotFound, "")
	})

	t.Run("PLT-021/delete com jogo oculto é -32013 com data {platformId, gameCount}", func(t *testing.T) {
		t.Parallel()
		h := newPlatHarness(t)
		p := h.add(t, "Amiga", "console").Platform
		ip := h.wantIconInside(t, h.setIconOK(t, p.ID, h.pngFile(t, "a.png", 15)))
		h.gameOn(t, p.ID, "Hidden", true)

		data := wantError(t, h.call(t, ipc.MethodPlatformDelete, map[string]int64{"id": p.ID}), ipc.CodePlatformInUse, ipc.MsgPlatformInUse)
		wantKeys(t, data, "platformId", "gameCount")
		wantRaw(t, data, "platformId", strconv.FormatInt(p.ID, 10))
		wantRaw(t, data, "gameCount", "1")
		if _, ok := h.find(t, p.ID); !ok {
			t.Errorf("platform %d was deleted", p.ID)
		}
		if _, err := os.Stat(ip); err != nil {
			t.Errorf("icon after the refused delete: %v, want it kept", err)
		}
	})

	t.Run("PLT-022/platform.setEmulator devolve o Platform completo com o novo emulatorId", func(t *testing.T) {
		t.Parallel()
		h := newPlatHarness(t)
		p := h.add(t, "Amiga", "console").Platform
		h.gameOn(t, p.ID, "G", false)
		emu, err := h.st.AddEmulator(t.Context(), "Emu", filepath.Join(h.dir, "e.exe"), "{rom}")
		if err != nil {
			t.Fatalf("AddEmulator: %v", err)
		}
		resp := h.call(t, ipc.MethodPlatformSetEmulator, map[string]any{"platformId": p.ID, "emulatorId": emu.ID})
		wantKeys(t, objectOf(t, resp.Result), "platform")
		wantKeys(t, objectOf(t, objectOf(t, resp.Result)["platform"]),
			"id", "name", "category", "emulatorId", "seedKey", "iconPath", "gameCount")
		var out ipc.PlatformResult
		resultInto(t, resp, &out)
		if out.Platform.ID != p.ID || out.Platform.Name != "Amiga" || out.Platform.Category != "console" ||
			out.Platform.EmulatorID == nil || *out.Platform.EmulatorID != emu.ID || out.Platform.GameCount != 1 {
			t.Errorf("platform = %+v, want Amiga console with emulatorId %d and gameCount 1", out.Platform, emu.ID)
		}
		resultInto(t, h.call(t, ipc.MethodPlatformSetEmulator, map[string]any{"platformId": p.ID, "emulatorId": nil}), &out)
		if out.Platform.EmulatorID != nil {
			t.Errorf("emulatorId after null = %d, want null", *out.Platform.EmulatorID)
		}
	})
}

// TestPlatformNotifications runs every platform write in a full agent with two
// clients [PLT-024]. It is not parallel: newWiring checks for leaked goroutines.
func TestPlatformNotifications(t *testing.T) {
	t.Run("PLT-024/nenhum método de plataforma envia notificação", func(t *testing.T) {
		w := newWiring(t, wiringOpts{})
		c1, c2 := w.client(t), w.client(t)
		before1, before2 := countNotifications(c1.snapshotAll()), countNotifications(c2.snapshotAll())

		src := filepath.Join(t.TempDir(), "i.png")
		img := image.NewRGBA(image.Rect(0, 0, 8, 8))
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(src, buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}

		var added ipc.PlatformAddResult
		if err := json.Unmarshal(c1.rpcOK(t, ipc.MethodPlatformAdd, map[string]string{"name": "Amiga", "category": "console"}).Result, &added); err != nil {
			t.Fatal(err)
		}
		id := added.Platform.ID
		c1.rpcOK(t, ipc.MethodPlatformUpdate, map[string]any{"id": id, "name": "Amiga 500", "category": "arcade"})
		c1.rpcOK(t, ipc.MethodPlatformSetIcon, map[string]any{"platformId": id, "path": src})
		c1.rpcOK(t, ipc.MethodPlatformSetIcon, map[string]any{"platformId": id, "path": nil})
		c1.rpcOK(t, ipc.MethodPlatformSetEmulator, map[string]any{"platformId": id, "emulatorId": nil})
		c1.rpcOK(t, ipc.MethodPlatformList, nil)
		c1.rpcOK(t, ipc.MethodLibraryCatalog, nil)
		c1.rpcOK(t, ipc.MethodPlatformDelete, map[string]any{"id": id})
		// A round trip on each connection: anything queued before the response has arrived.
		c1.rpcOK(t, ipc.MethodStatus, nil)
		c2.rpcOK(t, ipc.MethodStatus, nil)
		if n := countNotifications(c1.snapshotAll()); n != before1 {
			t.Errorf("client 1 got %d notification(s), want none", n-before1)
		}
		if n := countNotifications(c2.snapshotAll()); n != before2 {
			t.Errorf("client 2 got %d notification(s), want none", n-before2)
		}
	})
}
