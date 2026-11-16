package agent

// Tests of the manual edits (phase 14): media.add, media.remove and game.setPlayTime, over a real store and a
// real media.Store under t.TempDir().

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
	"github.com/arcadia-launcher/arcadia/internal/media"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

type editNotes struct{ games []int64 }

func (n *editNotes) MediaChanged(id int64) { n.games = append(n.games, id) }

type editFixture struct {
	h     map[string]ipc.Handler
	st    *store.Store
	root  string
	notes *editNotes
	game  int64
}

func newEditFixture(t *testing.T) editFixture {
	t.Helper()
	st := openImgStore(t)
	root := filepath.Join(t.TempDir(), "media")
	notes := &editNotes{}
	h := manualEditHandlers(manualEdit{
		st: st, files: media.New(root, nil), notes: notes,
		clock: domain.NewFakeClock(time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC)), log: quietLog(),
	})
	return editFixture{h: h, st: st, root: root, notes: notes, game: addGame(t, st, "Jogo")}
}

// editPNG writes a small PNG whose content depends on shade, so each shade is another image.
func editPNG(t *testing.T, shade uint8) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for i := range img.Pix {
		img.Pix[i] = shade
	}
	img.Set(0, 0, color.RGBA{R: shade, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "img.png")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func (f editFixture) call(t *testing.T, method string, params any) (json.RawMessage, error) {
	t.Helper()
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	res, err := f.h[method](context.Background(), raw)
	if err != nil {
		return nil, err
	}
	out, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	return out, nil
}

func (f editFixture) add(t *testing.T, kind, path string) error {
	t.Helper()
	_, err := f.call(t, ipc.MethodMediaAdd, ipc.MediaAddParams{GameID: f.game, Kind: kind, Path: path})
	return err
}

func (f editFixture) media(t *testing.T) []store.MediaRow {
	t.Helper()
	rows, err := f.st.ListMedia(t.Context(), f.game)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func wantCode(t *testing.T, err error, code int) {
	t.Helper()
	var rpcErr *ipc.Error
	if !errors.As(err, &rpcErr) || rpcErr.Code != code {
		t.Fatalf("error = %v, want code %d", err, code)
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestManualMedia(t *testing.T) {
	t.Parallel()

	t.Run("EDM-001/adicionar uma screenshot grava a imagem com provedor user e avisa media.changed", func(t *testing.T) {
		t.Parallel()
		f := newEditFixture(t)
		if err := f.add(t, "screenshot", editPNG(t, 10)); err != nil {
			t.Fatalf("media.add: %v", err)
		}
		rows := f.media(t)
		if len(rows) != 1 || rows[0].Kind != "screenshot" || rows[0].Provider != ipc.ProviderUser {
			t.Fatalf("media = %+v, want one user screenshot", rows)
		}
		if !exists(filepath.Join(f.root, rows[0].Path)) || !exists(filepath.Join(f.root, rows[0].ThumbPath)) {
			t.Fatal("image or thumbnail file missing")
		}
		if len(f.notes.games) != 1 || f.notes.games[0] != f.game {
			t.Fatalf("media.changed = %v, want [%d]", f.notes.games, f.game)
		}
	})

	t.Run("EDM-002/capa nova substitui a antiga e apaga os arquivos dela", func(t *testing.T) {
		t.Parallel()
		f := newEditFixture(t)
		if err := f.add(t, "cover", editPNG(t, 10)); err != nil {
			t.Fatal(err)
		}
		old := f.media(t)[0]
		if err := f.add(t, "cover", editPNG(t, 200)); err != nil {
			t.Fatal(err)
		}
		rows := f.media(t)
		if len(rows) != 1 || rows[0].Hash == old.Hash {
			t.Fatalf("media = %+v, want only the new cover", rows)
		}
		if exists(filepath.Join(f.root, old.Path)) || exists(filepath.Join(f.root, old.ThumbPath)) {
			t.Fatal("files of the old cover were kept")
		}
	})

	t.Run("EDM-002/fundo além do limite, tipo desconhecido e arquivo que não é imagem dão -32602 sem gravar", func(t *testing.T) {
		t.Parallel()
		f := newEditFixture(t)
		for shade := uint8(1); shade <= 2; shade++ {
			if err := f.add(t, "background", editPNG(t, shade*50)); err != nil {
				t.Fatal(err)
			}
		}
		wantCode(t, f.add(t, "background", editPNG(t, 250)), ipc.CodeInvalidParams)
		wantCode(t, f.add(t, "video", editPNG(t, 251)), ipc.CodeInvalidParams)
		text := filepath.Join(t.TempDir(), "nota.png")
		if err := os.WriteFile(text, []byte("isto não é uma imagem"), 0o644); err != nil {
			t.Fatal(err)
		}
		wantCode(t, f.add(t, "screenshot", text), ipc.CodeInvalidParams)
		if n := len(f.media(t)); n != 2 {
			t.Fatalf("media rows = %d, want 2", n)
		}
	})

	t.Run("EDM-003/remover apaga o registro e os arquivos", func(t *testing.T) {
		t.Parallel()
		f := newEditFixture(t)
		if err := f.add(t, "screenshot", editPNG(t, 10)); err != nil {
			t.Fatal(err)
		}
		row := f.media(t)[0]
		if _, err := f.call(t, ipc.MethodMediaRemove, ipc.MediaRemoveParams{MediaID: row.ID}); err != nil {
			t.Fatalf("media.remove: %v", err)
		}
		if n := len(f.media(t)); n != 0 {
			t.Fatalf("media rows = %d, want 0", n)
		}
		if exists(filepath.Join(f.root, row.Path)) || exists(filepath.Join(f.root, row.ThumbPath)) {
			t.Fatal("files of the removed image were kept")
		}
	})
}

func TestManualPlayTime(t *testing.T) {
	t.Parallel()

	t.Run("EDM-005/game.setPlayTime grava o total e devolve o jogo", func(t *testing.T) {
		t.Parallel()
		f := newEditFixture(t)
		raw, err := f.call(t, ipc.MethodGameSetPlayTime, ipc.GameSetPlayTimeParams{GameID: f.game, TotalPlaySeconds: 5400})
		if err != nil {
			t.Fatalf("game.setPlayTime: %v", err)
		}
		var res ipc.GameDetailResult
		if err := json.Unmarshal(raw, &res); err != nil {
			t.Fatal(err)
		}
		if res.Game.TotalPlaySeconds != 5400 {
			t.Fatalf("result total = %d, want 5400", res.Game.TotalPlaySeconds)
		}
		d, err := f.st.GameDetail(t.Context(), f.game)
		if err != nil {
			t.Fatal(err)
		}
		if d.TotalPlaySeconds != 5400 {
			t.Fatalf("stored total = %d, want 5400", d.TotalPlaySeconds)
		}
	})

	t.Run("EDM-005/valor negativo dá -32602 e jogo inexistente dá -32001", func(t *testing.T) {
		t.Parallel()
		f := newEditFixture(t)
		_, err := f.call(t, ipc.MethodGameSetPlayTime, ipc.GameSetPlayTimeParams{GameID: f.game, TotalPlaySeconds: -1})
		wantCode(t, err, ipc.CodeInvalidParams)
		_, err = f.call(t, ipc.MethodGameSetPlayTime, ipc.GameSetPlayTimeParams{GameID: 9999, TotalPlaySeconds: 60})
		wantCode(t, err, ipc.CodeGameNotFound)
	})
}
