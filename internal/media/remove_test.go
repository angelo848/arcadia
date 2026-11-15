package media_test

// Tests of Store.RemoveGame (DEL-007, DEL-009) and Store.SweepOrphans (DEL-010, DEL-011). Every
// folder lives under t.TempDir(). A link is made with os.Symlink and, on Windows, a junction as a
// fallback; the test is skipped when neither can be created (no privilege). Ids are passed as
// integers only, as the handler does.

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/media"
)

// rmFile writes a file at p, creating its folders.
func rmFile(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(p), err)
	}
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
}

// rmExists reports whether p exists; a link is looked at, not followed.
func rmExists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// rmLink makes link point to the folder target. It skips the test when no link can be created.
func rmLink(t *testing.T, target, link string) {
	t.Helper()
	symErr := os.Symlink(target, link)
	if symErr == nil {
		return
	}
	if runtime.GOOS == "windows" {
		out, jErr := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput()
		if jErr == nil {
			return
		}
		t.Skipf("cannot create a symlink (%v) nor a junction (%v: %s): needs privilege", symErr, jErr, out)
	}
	t.Skipf("cannot create a symlink: %v", symErr)
}

// rmFileLink makes link point to the file target. It skips the test when no link can be created.
func rmFileLink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("cannot create a file symlink: %v", err)
	}
}

// rmIDs returns a listIDs function that answers ids.
func rmIDs(ids ...int64) func(context.Context) ([]int64, error) {
	return func(context.Context) ([]int64, error) { return ids, nil }
}

func TestRemoveGame(t *testing.T) {
	t.Parallel()

	t.Run("DEL-007/apaga a pasta do jogo com imagens, miniaturas e subpastas", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		rmFile(t, filepath.Join(root, "7", "a.jpg"))
		rmFile(t, filepath.Join(root, "7", "thumb_a.jpg"))
		rmFile(t, filepath.Join(root, "7", "b.png"))
		rmFile(t, filepath.Join(root, "7", "sub", "c.png"))
		rmFile(t, filepath.Join(root, "8", "keep.jpg"))

		if err := media.New(root, nil).RemoveGame(7); err != nil {
			t.Fatalf("RemoveGame(7): %v", err)
		}
		if rmExists(filepath.Join(root, "7")) {
			t.Error("folder of game 7 still exists after RemoveGame")
		}
		if !rmExists(filepath.Join(root, "8", "keep.jpg")) {
			t.Error("folder of game 8 was removed by RemoveGame(7)")
		}
	})

	t.Run("DEL-007/pasta ausente é no-op: nil e nenhuma pasta criada", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		rmFile(t, filepath.Join(root, "1", "a.jpg"))

		if err := media.New(root, nil).RemoveGame(42); err != nil {
			t.Fatalf("RemoveGame(42) with no folder: %v, want nil", err)
		}
		if rmExists(filepath.Join(root, "42")) {
			t.Error("RemoveGame created the folder of game 42")
		}
		if !rmExists(filepath.Join(root, "1", "a.jpg")) {
			t.Error("unrelated folder removed")
		}
	})

	t.Run("DEL-009/id 0 e negativos são recusados e nada é apagado", func(t *testing.T) {
		t.Parallel()
		for _, id := range []int64{0, -1, -7} {
			root := t.TempDir()
			name := strconv.FormatInt(id, 10)
			rmFile(t, filepath.Join(root, name, "x.jpg"))
			rmFile(t, filepath.Join(root, "1", "keep.jpg"))

			if err := media.New(root, nil).RemoveGame(id); err == nil {
				t.Errorf("RemoveGame(%d) err = nil, want an error", id)
			}
			if !rmExists(filepath.Join(root, name, "x.jpg")) {
				t.Errorf("RemoveGame(%d) removed folder %q", id, name)
			}
			if !rmExists(filepath.Join(root, "1", "keep.jpg")) {
				t.Errorf("RemoveGame(%d) removed folder 1", id)
			}
		}
	})

	t.Run("DEL-009/pastas-irmãs de nome parecido não são apagadas (12 não apaga 120 nem 012)", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		rmFile(t, filepath.Join(root, "12", "a.jpg"))
		rmFile(t, filepath.Join(root, "120", "b.jpg"))
		rmFile(t, filepath.Join(root, "012", "c.jpg"))

		if err := media.New(root, nil).RemoveGame(12); err != nil {
			t.Fatalf("RemoveGame(12): %v", err)
		}
		if rmExists(filepath.Join(root, "12")) {
			t.Error("folder 12 still exists")
		}
		for _, keep := range []string{"120/b.jpg", "012/c.jpg"} {
			if !rmExists(filepath.Join(root, filepath.FromSlash(keep))) {
				t.Errorf("%s was removed by RemoveGame(12)", keep)
			}
		}
	})

	t.Run("DEL-009/<root>/<id> é link para pasta de fora: erro, o link e o destino ficam", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		outside := t.TempDir()
		rmFile(t, filepath.Join(outside, "keep.txt"))
		rmLink(t, outside, filepath.Join(root, "5"))

		if err := media.New(root, nil).RemoveGame(5); err == nil {
			t.Error("RemoveGame over a link: err = nil, want an error")
		}
		if !rmExists(filepath.Join(outside, "keep.txt")) {
			t.Error("the content of the link target was removed")
		}
		if !rmExists(filepath.Join(root, "5")) {
			t.Error("the link itself was removed; nothing must be deleted")
		}
	})

	t.Run("DEL-009/link de pasta dentro da pasta do jogo é removido sem seguir o destino", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		outside := t.TempDir()
		rmFile(t, filepath.Join(outside, "keep.txt"))
		rmFile(t, filepath.Join(root, "3", "a.jpg"))
		rmLink(t, outside, filepath.Join(root, "3", "linkdir"))

		if err := media.New(root, nil).RemoveGame(3); err != nil {
			t.Fatalf("RemoveGame(3): %v", err)
		}
		if rmExists(filepath.Join(root, "3")) {
			t.Error("folder 3 still exists")
		}
		if !rmExists(filepath.Join(outside, "keep.txt")) {
			t.Error("RemoveGame followed a link inside the game folder and removed the target content")
		}
	})

	t.Run("DEL-009/link de arquivo dentro da pasta do jogo: o arquivo de fora fica", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		outside := t.TempDir()
		target := filepath.Join(outside, "target.jpg")
		rmFile(t, target)
		rmFile(t, filepath.Join(root, "4", "a.jpg"))
		rmFileLink(t, target, filepath.Join(root, "4", "link.jpg"))

		if err := media.New(root, nil).RemoveGame(4); err != nil {
			t.Fatalf("RemoveGame(4): %v", err)
		}
		if rmExists(filepath.Join(root, "4")) {
			t.Error("folder 4 still exists")
		}
		if !rmExists(target) {
			t.Error("the file behind a link inside the game folder was removed")
		}
	})
}

// TestRemoveGameRelativeRoot changes the working directory, so it cannot run in parallel.
func TestRemoveGameRelativeRoot(t *testing.T) {
	t.Run("DEL-009/mediaRoot vazio é recusado e nada é apagado no diretório atual", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		rmFile(t, filepath.Join(dir, "5", "x.jpg"))

		if err := media.New("", nil).RemoveGame(5); err == nil {
			t.Error("RemoveGame with an empty root: err = nil, want an error")
		}
		if !rmExists(filepath.Join(dir, "5", "x.jpg")) {
			t.Error("a folder of the working directory was removed by RemoveGame with an empty root")
		}
	})

	t.Run("DEL-009/mediaRoot relativo: só <root>/5 pode ser apagada, nada fora da raiz", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		rmFile(t, filepath.Join(dir, "rel", "5", "x.jpg"))
		rmFile(t, filepath.Join(dir, "rel", "6", "y.jpg"))
		rmFile(t, filepath.Join(dir, "5", "outside.jpg"))

		// The spec leaves open whether a relative root is refused; either is accepted, as long as
		// the result is coherent: the error means nothing was removed, nil means only rel/5 went.
		err := media.New("rel", nil).RemoveGame(5)
		if !rmExists(filepath.Join(dir, "5", "outside.jpg")) {
			t.Error("a folder outside the media root was removed")
		}
		if !rmExists(filepath.Join(dir, "rel", "6", "y.jpg")) {
			t.Error("a sibling game folder inside the media root was removed")
		}
		gone := !rmExists(filepath.Join(dir, "rel", "5"))
		switch {
		case err == nil && !gone:
			t.Error("RemoveGame returned nil but rel/5 still exists")
		case err != nil && gone:
			t.Error("RemoveGame returned an error but rel/5 was removed")
		}
	})
}

func TestSweepOrphans(t *testing.T) {
	t.Parallel()

	t.Run("DEL-010/apaga as pastas numéricas órfãs e mantém as dos jogos existentes e platforms", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		for _, n := range []string{"1", "2", "3", "4"} {
			rmFile(t, filepath.Join(root, n, "a.jpg"))
		}
		rmFile(t, filepath.Join(root, "platforms", "icon.png"))

		if err := media.New(root, nil).SweepOrphans(t.Context(), rmIDs(1, 3)); err != nil {
			t.Fatalf("SweepOrphans: %v", err)
		}
		for _, gone := range []string{"2", "4"} {
			if rmExists(filepath.Join(root, gone)) {
				t.Errorf("orphan folder %s still exists", gone)
			}
		}
		for _, keep := range []string{"1", "3", filepath.Join("platforms", "icon.png")} {
			if !rmExists(filepath.Join(root, keep)) {
				t.Errorf("%s was removed by the sweep", keep)
			}
		}
	})

	t.Run("DEL-010/nomes que não são a forma canônica de um id positivo, e arquivos, nunca são apagados", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		keep := []string{"007", "0", "-1", "1a", "platforms", "99999999999999999999", "+5"}
		for _, n := range keep {
			rmFile(t, filepath.Join(root, n, "a.jpg"))
		}
		rmFile(t, filepath.Join(root, "5")) // a file named 5, not a folder
		rmFile(t, filepath.Join(root, "6.jpg"))
		rmFile(t, filepath.Join(root, "2", "a.jpg")) // the one canonical orphan

		if err := media.New(root, nil).SweepOrphans(t.Context(), rmIDs()); err != nil {
			t.Fatalf("SweepOrphans: %v", err)
		}
		if rmExists(filepath.Join(root, "2")) {
			t.Error("canonical orphan folder 2 still exists")
		}
		for _, n := range keep {
			if !rmExists(filepath.Join(root, n, "a.jpg")) {
				t.Errorf("folder %q was removed; it is not the canonical form of an id", n)
			}
		}
		for _, f := range []string{"5", "6.jpg"} {
			if !rmExists(filepath.Join(root, f)) {
				t.Errorf("file %q was removed by the sweep", f)
			}
		}
	})

	t.Run("DEL-010/link com nome numérico não é seguido nem apagado: o destino de fora fica", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		outside := t.TempDir()
		rmFile(t, filepath.Join(outside, "keep.txt"))
		rmLink(t, outside, filepath.Join(root, "8"))

		if err := media.New(root, nil).SweepOrphans(t.Context(), rmIDs()); err != nil {
			t.Fatalf("SweepOrphans: %v", err)
		}
		if !rmExists(filepath.Join(outside, "keep.txt")) {
			t.Error("the sweep followed a link and removed the content outside the media root")
		}
	})

	t.Run("DEL-010/a lista de pastas é lida antes da lista de ids: jogo criado durante a varredura fica", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		rmFile(t, filepath.Join(root, "2", "a.jpg"))
		listIDs := func(context.Context) ([]int64, error) {
			// A game added while the sweep runs: its folder appears after the folders were listed.
			rmFile(t, filepath.Join(root, "9", "late.jpg"))
			return []int64{1}, nil
		}

		if err := media.New(root, nil).SweepOrphans(t.Context(), listIDs); err != nil {
			t.Fatalf("SweepOrphans: %v", err)
		}
		if !rmExists(filepath.Join(root, "9", "late.jpg")) {
			t.Error("the folder of a game created during the sweep was removed")
		}
		if rmExists(filepath.Join(root, "2")) {
			t.Error("orphan folder 2 still exists")
		}
	})

	t.Run("DEL-010/se listIDs falha nada é apagado e o erro é devolvido", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		rmFile(t, filepath.Join(root, "2", "a.jpg"))
		rmFile(t, filepath.Join(root, "4", "a.jpg"))
		errDB := errors.New("banco fechado")

		err := media.New(root, nil).SweepOrphans(t.Context(), func(context.Context) ([]int64, error) {
			return nil, errDB
		})
		if !errors.Is(err, errDB) {
			t.Errorf("SweepOrphans err = %v, want the listIDs error", err)
		}
		if !rmExists(filepath.Join(root, "2", "a.jpg")) || !rmExists(filepath.Join(root, "4", "a.jpg")) {
			t.Error("folders removed although the ids could not be read")
		}
	})

	t.Run("DEL-010/ctx cancelado antes da varredura: nada é apagado", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		rmFile(t, filepath.Join(root, "2", "a.jpg"))
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		err := media.New(root, nil).SweepOrphans(ctx, rmIDs())
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Errorf("SweepOrphans with a cancelled ctx err = %v, want nil or context.Canceled", err)
		}
		if !rmExists(filepath.Join(root, "2", "a.jpg")) {
			t.Error("folder removed by a sweep whose ctx was already cancelled")
		}
	})

	t.Run("DEL-010/ctx cancelado durante a leitura dos ids: nenhuma pasta é apagada", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		rmFile(t, filepath.Join(root, "2", "a.jpg"))
		rmFile(t, filepath.Join(root, "4", "a.jpg"))
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		listIDs := func(context.Context) ([]int64, error) {
			cancel()
			return []int64{1}, nil
		}

		err := media.New(root, nil).SweepOrphans(ctx, listIDs)
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Errorf("SweepOrphans err = %v, want nil or context.Canceled", err)
		}
		if !rmExists(filepath.Join(root, "2", "a.jpg")) || !rmExists(filepath.Join(root, "4", "a.jpg")) {
			t.Error("a folder was removed after the ctx was cancelled")
		}
	})

	t.Run("DEL-010/falha em uma pasta não interrompe as outras", func(t *testing.T) {
		t.Parallel()
		if runtime.GOOS != "windows" {
			t.Skip("a folder that cannot be removed is simulated with a file held open, on Windows only")
		}
		root := t.TempDir()
		held := filepath.Join(root, "2", "held.jpg")
		rmFile(t, held)
		rmFile(t, filepath.Join(root, "4", "a.jpg"))
		f, err := os.Open(held)
		if err != nil {
			t.Fatalf("open held file: %v", err)
		}
		t.Cleanup(func() { _ = f.Close() }) // registered after TempDir: closed before the folder is removed

		if err := media.New(root, nil).SweepOrphans(t.Context(), rmIDs()); err != nil {
			t.Logf("SweepOrphans err = %v (the failure may be logged instead)", err)
		}
		if !rmExists(filepath.Join(root, "2")) {
			t.Skip("the OS removed the folder with a file held open; the failure cannot be simulated here")
		}
		if rmExists(filepath.Join(root, "4")) {
			t.Error("folder 4 kept after folder 2 failed: the sweep stopped at the first failure")
		}
	})
}
