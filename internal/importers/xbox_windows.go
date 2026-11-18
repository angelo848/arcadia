//go:build windows

package importers

import (
	"os"
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const xboxRepositoryRoot = `SOFTWARE\Microsoft\GamingServices\PackageRepository\Root`

// xboxInstallRoots lists the folders where Gaming Services installed packages: every
// Root\{volume}\{package} key of the package repository has a "Root" value with the folder.
func xboxInstallRoots() ([]string, error) {
	base, err := registry.OpenKey(registry.LOCAL_MACHINE, xboxRepositoryRoot, registry.READ)
	if err != nil {
		return nil, err
	}
	defer base.Close()
	volumes, err := base.ReadSubKeyNames(-1)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var roots []string
	for _, vol := range volumes {
		if !strings.HasPrefix(vol, "{") {
			continue // the other keys are per-package save data
		}
		for _, root := range xboxVolumeRoots(base, vol) {
			if !seen[strings.ToLower(root)] {
				seen[strings.ToLower(root)] = true
				roots = append(roots, root)
			}
		}
	}
	return roots, nil
}

// xboxVolumeRoots reads the install folders of one volume key. A key that cannot be read is skipped.
func xboxVolumeRoots(base registry.Key, volume string) []string {
	vk, err := registry.OpenKey(base, volume, registry.READ)
	if err != nil {
		return nil
	}
	defer vk.Close()
	packages, err := vk.ReadSubKeyNames(-1)
	if err != nil {
		return nil
	}
	var roots []string
	for _, pkg := range packages {
		pk, err := registry.OpenKey(vk, pkg, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		root, _, err := pk.GetStringValue("Root")
		pk.Close()
		if err != nil {
			continue
		}
		root = strings.TrimPrefix(root, `\\?\`)
		if st, err := os.Stat(root); err == nil && st.IsDir() {
			roots = append(roots, root)
		}
	}
	return roots
}

// realDir resolves junctions and mount points, because a running game reports its real path: the
// package folder of Gaming Services is a junction to the folder the user chose. It returns dir itself
// when the path cannot be resolved.
func realDir(dir string) string {
	p, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return dir
	}
	h, err := windows.CreateFile(p, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return dir
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n, err := windows.GetFinalPathNameByHandle(h, &buf[0], uint32(len(buf)), 0)
	if err != nil || n == 0 || int(n) > len(buf) {
		return dir
	}
	return strings.TrimPrefix(windows.UTF16ToString(buf[:n]), `\\?\`)
}
