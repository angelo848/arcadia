//go:build !windows

package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
)

// errUnsupported is returned by every platform hook outside Windows.
var errUnsupported = errors.New("desktop: supported only on Windows")

// The UI depends on Win32 (named pipe, detached process creation, WebView2); on
// other systems the binary only reports that it is unsupported, so the package
// still builds, vets and runs its portable tests.
func main() {
	slog.Error("arcadia: suportado apenas no Windows")
	os.Exit(1)
}

// defaultPlatform returns hooks that fail: there is no agent pipe here
// (startAgent receives the executable path and the data root, like on Windows).
func defaultPlatform() platform {
	return platform{
		newProbe:   func() (pinger, error) { return nil, errUnsupported },
		newClient:  func(*slog.Logger) (ipcClient, error) { return nil, errUnsupported },
		startAgent: func(string, string, *slog.Logger) error { return errUnsupported },
		emit:       func(context.Context, string, any) {},
		quit:       func(context.Context) {},
		showWindow: func(context.Context) {},
		openFile: func(context.Context, string, []fileFilter) (string, error) {
			return "", errUnsupported
		},
	}
}
