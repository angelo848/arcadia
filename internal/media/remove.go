package media

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
)

// ErrNoRoot is returned when the store has no root folder: nothing is removed relative to the
// working directory (DEL-009).
var ErrNoRoot = errors.New("media: raiz de mídia vazia")

// gameFolder returns <root>\<id> for a positive id. The name is built only from the integer, so a
// folder of another name can never be the target (DEL-009).
func (s *Store) gameFolder(id int64) (string, error) {
	if id <= 0 {
		return "", fmt.Errorf("media: id de jogo %d inválido", id)
	}
	if s.root == "" {
		return "", ErrNoRoot
	}
	return filepath.Join(s.root, strconv.FormatInt(id, 10)), nil
}

// RemoveGame deletes <root>\<id> with everything in it (DEL-007). A missing folder is not an error.
// A link (symbolic link or junction) in place of the folder is refused and nothing is deleted: the
// link is neither removed nor followed (DEL-009). Links inside the folder are removed as links and
// their targets are left alone.
func (s *Store) RemoveGame(id int64) error {
	dir, err := s.gameFolder(id)
	if err != nil {
		return err
	}
	info, err := os.Lstat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("media: ler pasta do jogo %d: %w", id, err)
	}
	if info.Mode().Type() != fs.ModeDir {
		return fmt.Errorf("media: pasta do jogo %d não é um diretório comum", id)
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("media: apagar pasta do jogo %d: %w", id, err)
	}
	return nil
}

// SweepOrphans removes the game folders of the root that belong to no game (DEL-010). The folders
// are listed first and the ids are read after (listIDs), so a game added during the sweep keeps its
// folder. Only directories named as the canonical form of a positive id ("7", not "007", "0", "1a"
// or "platforms") are candidates; files and links are never removed. A failure in one folder is
// collected and does not stop the others; a failure of listIDs or a cancelled ctx removes nothing.
func (s *Store) SweepOrphans(ctx context.Context, listIDs func(context.Context) ([]int64, error)) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("media: varredura: %w", err)
	}
	if s.root == "" {
		return ErrNoRoot
	}
	entries, err := os.ReadDir(s.root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("media: listar raiz: %w", err)
	}
	ids, err := listIDs(ctx)
	if err != nil {
		return fmt.Errorf("media: listar jogos: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("media: varredura: %w", err)
	}
	live := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		live[id] = struct{}{}
	}

	var errs []error
	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return errors.Join(append(errs, fmt.Errorf("media: varredura: %w", err))...)
		}
		id, ok := canonicalID(e.Name())
		if !ok {
			continue
		}
		if _, alive := live[id]; alive {
			continue
		}
		path := filepath.Join(s.root, e.Name())
		info, err := os.Lstat(path)
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				errs = append(errs, fmt.Errorf("media: ler pasta órfã %d: %w", id, err))
			}
			continue
		}
		if info.Mode().Type() != fs.ModeDir { // files, symbolic links and other reparse points stay
			continue
		}
		if err := os.RemoveAll(path); err != nil {
			errs = append(errs, fmt.Errorf("media: apagar pasta órfã %d: %w", id, err))
		}
	}
	return errors.Join(errs...)
}

// canonicalID returns the id named by name when name is exactly the canonical decimal form of a
// positive int64 ("7"); anything else ("007", "+5", "0", "-1", "1a", overflow) is not an id.
func canonicalID(name string) (int64, bool) {
	id, err := strconv.ParseInt(name, 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	if strconv.FormatInt(id, 10) != name {
		return 0, false
	}
	return id, true
}
