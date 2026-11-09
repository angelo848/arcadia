package agent

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

// settingAutostart is the app_setting key of the start-with-Windows
// preference; its value is "true" or "false".
const settingAutostart = "autostart"

const (
	autostartErrorMsg   = "início com o Windows: falha"
	autostartDevInfoMsg = "início com o Windows desativado em build de desenvolvimento"
)

// runEntry is the HKCU Run value of the agent (real: *winapi.RunValue).
type runEntry interface {
	Get() (value string, ok bool, err error)
	Set(value string) error
	Delete() error // absent is not an error
}

// settingStore reads and writes preferences (real: *store.Store).
type settingStore interface {
	Setting(ctx context.Context, key string) (string, error)
	SetSetting(ctx context.Context, key, value string, at time.Time) error
}

// autostart keeps the Run entry in line with the autostart preference
// (AST-003..AST-005, AST-007..AST-010). It is not safe for concurrent use; the
// agent calls Reconcile once at startup and Toggle from the tray loop.
type autostart struct {
	enabled, dev bool // enabled is false and inert when dev (AST-010)
	command, exe string
	exeOf        func(cmdline string) (string, error)
	run          runEntry
	settings     settingStore
	clock        domain.Clock
	log          *slog.Logger
}

// Reconcile reads the preference (writing "true" on the first run) and fixes
// the Run entry; it returns the initial state of the tray item. A failure to
// read or write the preference leaves the registry untouched and returns
// false; a registry failure is logged and the preference still decides the
// returned state (AST-009). In dev it touches nothing and returns false.
func (a *autostart) Reconcile(ctx context.Context) (checked bool) {
	a.enabled = false
	if a.dev {
		a.log.Info(autostartDevInfoMsg)
		return false
	}
	value, err := a.settings.Setting(ctx, settingAutostart)
	switch {
	case errors.Is(err, store.ErrSettingNotFound):
		// AST-003: first run turns it on.
		if err := a.settings.SetSetting(ctx, settingAutostart, "true", a.clock.Now()); err != nil {
			a.fail("write-setting", err)
			return false
		}
		value = "true"
	case err != nil:
		a.fail("read-setting", err)
		return false
	}
	a.enabled = value == "true"
	if a.enabled {
		a.ensureEntry()
	} else {
		a.removeOwnEntry()
	}
	return a.enabled
}

// Toggle flips the preference, writing it first, and then applies it to the
// Run entry (AST-007, AST-008). If the preference write fails it returns the
// old state without touching the registry; a registry failure is logged and
// the new state is returned (AST-009). In dev it does nothing and returns false.
func (a *autostart) Toggle(ctx context.Context) (checked bool) {
	if a.dev {
		return false
	}
	next := !a.enabled
	if err := a.settings.SetSetting(ctx, settingAutostart, boolSetting(next), a.clock.Now()); err != nil {
		a.fail("write-setting", err)
		return a.enabled
	}
	a.enabled = next
	if next {
		if err := a.run.Set(a.command); err != nil {
			a.fail("write-run", err)
		}
	} else {
		a.removeOwnEntry()
	}
	return a.enabled
}

// ensureEntry writes the command when the Run value is absent or differs (AST-004).
func (a *autostart) ensureEntry() {
	value, ok, err := a.run.Get()
	if err != nil {
		a.fail("read-run", err)
		return
	}
	if ok && value == a.command {
		return
	}
	if err := a.run.Set(a.command); err != nil {
		a.fail("write-run", err)
	}
}

// removeOwnEntry deletes the Run value only when it points to this executable
// (AST-005); an unreadable value is kept and logged as Warn.
func (a *autostart) removeOwnEntry() {
	value, ok, err := a.run.Get()
	if err != nil {
		a.fail("read-run", err)
		return
	}
	if !ok {
		return
	}
	exe, err := a.exeOf(value)
	if err != nil {
		a.log.Warn("início com o Windows: entrada Run ilegível; mantida", "value", value, "error", err.Error())
		return
	}
	if !strings.EqualFold(filepath.Clean(exe), filepath.Clean(a.exe)) {
		return
	}
	if err := a.run.Delete(); err != nil {
		a.fail("delete-run", err)
	}
}

func (a *autostart) fail(op string, err error) {
	a.log.Error(autostartErrorMsg, "op", op, "error", err.Error())
}

func boolSetting(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
