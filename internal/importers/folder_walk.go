package importers

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
)

// fileEntry is one regular file found by the recursive folder walk [IMV-010]. Dir is the folder relative to the
// scanned root with "\" separators ("" for the root itself); Name is the file name.
type fileEntry struct {
	Dir  string
	Name string
}

// Rel returns the path of the file relative to the scanned root, with "\" separators [IMV-010].
func (e fileEntry) Rel() string {
	if e.Dir == "" {
		return e.Name
	}
	return e.Dir + `\` + e.Name
}

// Scope returns the first segment of Dir, the folder that groups the file under the root [IMV-021].
// A file of the root has an empty scope.
func (e fileEntry) Scope() string {
	return strings.SplitN(e.Dir, `\`, 2)[0]
}

// Stem returns the file name without its extension [IMV-001].
func (e fileEntry) Stem() string {
	stem, _ := SplitExt(e.Name)
	return stem
}

// Ext returns the extension of the file in lower case, without the dot [IMV-001].
func (e fileEntry) Ext() string {
	_, ext := SplitExt(e.Name)
	return ext
}

// walkResult is what the recursive walk found, plus the warnings the scan reports [IMV-010, IMV-011, IMV-012, IMV-018].
type walkResult struct {
	Files      []fileEntry // regular files in visit order (depth first, natural order)
	DepthCut   bool        // a folder below MaxDepth exists and was not read
	ItemsCut   bool        // there is an item beyond MaxItems; the walk stopped there
	Unreadable int         // subfolders whose ReadDir failed (they are skipped)
}

// walker holds the state of one walk. It is not safe for concurrent use.
type walker struct {
	ctx      context.Context
	dirs     dirReader
	root     string
	opts     FolderOptions
	maxDepth int
	maxItems int
	items    int
	res      walkResult
}

// walk lists the folder root recursively. It reads only names and types through dirs [IMV-080]: entries that
// skipEntry rejects are not read nor counted [IMV-012]; every other regular file is returned, and the extension
// filter is applied by the caller [IMV-013]. Folders are read depth first, in natural order, files and folders mixed
// [IMV-010]. FirstLevelOnly does not read subfolders [IMV-011]. An error of the root folder is returned; an error of a
// subfolder counts as Unreadable. A cancelled ctx is returned as an error, wrapping ctx.Err() [IMV-018].
func walk(ctx context.Context, dirs dirReader, root string, opts FolderOptions) (walkResult, error) {
	w := &walker{ctx: ctx, dirs: dirs, root: root, opts: opts, maxDepth: opts.MaxDepth, maxItems: opts.MaxItems}
	if w.maxDepth == 0 {
		w.maxDepth = MaxScanDepth
	}
	if w.maxItems == 0 {
		w.maxItems = MaxScanItems
	}
	if err := w.visit("", 0, true); err != nil {
		return w.res, err
	}
	return w.res, nil
}

// visit reads the folder rel (relative to the root) at the given depth and walks its entries in natural order.
func (w *walker) visit(rel string, depth int, isRoot bool) error {
	if err := w.ctx.Err(); err != nil {
		return fmt.Errorf("leitura da pasta %q: %w", rel, err)
	}
	entries, err := w.dirs.ReadDir(filepath.Join(w.root, rel))
	if err != nil {
		if isRoot {
			return fmt.Errorf("ler pasta %q: %w", w.root, err)
		}
		w.res.Unreadable++
		return nil
	}
	sorted := slices.Clone(entries)
	slices.SortFunc(sorted, func(a, b fs.DirEntry) int {
		switch {
		case naturalLess(a.Name(), b.Name()):
			return -1
		case naturalLess(b.Name(), a.Name()):
			return 1
		}
		return 0
	})

	for _, e := range sorted {
		if skipEntry(e.Name(), e.Type()) {
			continue
		}
		w.items++
		if w.items > w.maxItems {
			w.res.ItemsCut = true
			return nil
		}
		if !e.IsDir() {
			w.res.Files = append(w.res.Files, fileEntry{Dir: rel, Name: e.Name()})
			continue
		}
		if w.opts.FirstLevelOnly {
			continue
		}
		if depth+1 > w.maxDepth {
			w.res.DepthCut = true
			continue
		}
		child := e.Name()
		if rel != "" {
			child = rel + `\` + e.Name()
		}
		if err := w.visit(child, depth+1, false); err != nil {
			return err
		}
		if w.res.ItemsCut {
			return nil
		}
	}
	return nil
}
