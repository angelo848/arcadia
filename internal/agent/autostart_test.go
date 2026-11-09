package agent

// T07 of docs/specs/single-exe/tasks.md: start with Windows (AST-002..AST-011).
// Portable: the registry (runEntry) and the preference store (settingStore)
// are fakes that count calls; nothing touches the real registry.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

const (
	autostartTestExe     = `C:\Jogos\Arcadia\arcadia.exe`
	autostartTestCommand = `"C:\Jogos\Arcadia\arcadia.exe" --agent`
	autostartErrMsg      = "início com o Windows: falha"
	autostartDevMsg      = "início com o Windows desativado em build de desenvolvimento"
)

var errFake = errors.New("fake failure")

// callTrace records the order of calls across the fakes (shared by one case).
type callTrace struct {
	mu    sync.Mutex
	calls []string
}

func (c *callTrace) add(s string) {
	c.mu.Lock()
	c.calls = append(c.calls, s)
	c.mu.Unlock()
}

func (c *callTrace) list() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.calls...)
}

// fakeRunEntry is a runEntry with a single value and injectable failures.
type fakeRunEntry struct {
	trace *callTrace

	mu                     sync.Mutex
	value                  string
	present                bool
	getErr, setErr, delErr error
	gets, sets, deletes    int
	setValues              []string
}

func (f *fakeRunEntry) Get() (string, bool, error) {
	f.trace.add("run.Get")
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gets++
	if f.getErr != nil {
		return "", false, f.getErr
	}
	return f.value, f.present, nil
}

func (f *fakeRunEntry) Set(value string) error {
	f.trace.add("run.Set")
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sets++
	f.setValues = append(f.setValues, value)
	if f.setErr != nil {
		return f.setErr
	}
	f.value, f.present = value, true
	return nil
}

func (f *fakeRunEntry) Delete() error {
	f.trace.add("run.Delete")
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deletes++
	if f.delErr != nil {
		return f.delErr
	}
	f.value, f.present = "", false
	return nil
}

func (f *fakeRunEntry) counts() (gets, sets, deletes int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.gets, f.sets, f.deletes
}

type settingWrite struct {
	key, value string
	at         time.Time
}

// fakeSettings is a settingStore backed by a map.
type fakeSettings struct {
	trace *callTrace

	mu             sync.Mutex
	values         map[string]string
	getErr, setErr error
	gets           int
	writes         []settingWrite
}

func (f *fakeSettings) Setting(_ context.Context, key string) (string, error) {
	f.trace.add("settings.Setting")
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gets++
	if f.getErr != nil {
		return "", f.getErr
	}
	v, ok := f.values[key]
	if !ok {
		// Same shape as *store.Store: wrapped sentinel.
		return "", fmt.Errorf("store: setting %q: %w", key, store.ErrSettingNotFound)
	}
	return v, nil
}

func (f *fakeSettings) SetSetting(_ context.Context, key, value string, at time.Time) error {
	f.trace.add("settings.SetSetting")
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writes = append(f.writes, settingWrite{key: key, value: value, at: at})
	if f.setErr != nil {
		return f.setErr
	}
	if f.values == nil {
		f.values = map[string]string{}
	}
	f.values[key] = value
	return nil
}

func (f *fakeSettings) snapshot() (gets int, writes []settingWrite) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.gets, append([]settingWrite(nil), f.writes...)
}

// fakeExeOf mimics winapi.CommandExe: first token, quoted or not; an
// unterminated quote or an empty line is an error. It counts calls.
type fakeExeOf struct {
	mu    sync.Mutex
	calls int
}

func (f *fakeExeOf) exeOf(cmdline string) (string, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	s := strings.TrimSpace(cmdline)
	if s == "" {
		return "", errors.New("empty command line")
	}
	if strings.HasPrefix(s, `"`) {
		end := strings.Index(s[1:], `"`)
		if end < 0 {
			return "", errors.New("unterminated quote")
		}
		return s[1 : 1+end], nil
	}
	if i := strings.IndexByte(s, ' '); i >= 0 {
		return s[:i], nil
	}
	return s, nil
}

func (f *fakeExeOf) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// autostartFixture holds an autostart under test and its fakes.
type autostartFixture struct {
	a        *autostart
	run      *fakeRunEntry
	settings *fakeSettings
	exe      *fakeExeOf
	clock    *domain.FakeClock
	logs     *recordHandler
	trace    *callTrace
}

type autostartSetup struct {
	dev      bool
	enabled  bool   // initial autostart.enabled (Toggle cases; matches pref)
	pref     string // "" => key absent; else "true"/"false"
	run      string // Run value; "" => absent
	getErr   error
	setErr   error
	delErr   error
	readErr  error // settings.Setting
	writeErr error // settings.SetSetting
}

func newAutostartFixture(s autostartSetup) *autostartFixture {
	trace := &callTrace{}
	clock := testClock()
	// Move the clock away from its start so "the clock's instant" is distinct.
	clock.Advance(90 * time.Minute)
	f := &autostartFixture{
		run: &fakeRunEntry{
			trace: trace, value: s.run, present: s.run != "",
			getErr: s.getErr, setErr: s.setErr, delErr: s.delErr,
		},
		settings: &fakeSettings{trace: trace, values: map[string]string{}, getErr: s.readErr, setErr: s.writeErr},
		exe:      &fakeExeOf{},
		clock:    clock,
		logs:     newRecordHandler(),
		trace:    trace,
	}
	if s.pref != "" {
		f.settings.values[settingAutostart] = s.pref
	}
	f.a = &autostart{
		enabled:  s.enabled,
		dev:      s.dev,
		command:  autostartTestCommand,
		exe:      autostartTestExe,
		exeOf:    f.exe.exeOf,
		run:      f.run,
		settings: f.settings,
		clock:    clock,
		log:      slog.New(f.logs),
	}
	return f
}

// recordsWith returns the records at level whose message is msg.
func (h *recordHandler) recordsWith(level slog.Level, msg string) []slog.Record {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []slog.Record
	for _, r := range h.records {
		if r.Level == level && r.Message == msg {
			out = append(out, r)
		}
	}
	return out
}

func recordAttr(r slog.Record, key string) (slog.Value, bool) {
	var v slog.Value
	found := false
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == key {
			v, found = a.Value, true
			return false
		}
		return true
	})
	return v, found
}

// assertRun checks how many Get/Set/Delete calls the Run entry received.
func (f *autostartFixture) assertRun(t *testing.T, gets, sets, deletes int) {
	t.Helper()
	g, s, d := f.run.counts()
	if g != gets || s != sets || d != deletes {
		t.Errorf("run entry calls Get/Set/Delete = %d/%d/%d, want %d/%d/%d (trace %v)",
			g, s, d, gets, sets, deletes, f.trace.list())
	}
}

// assertNoRegistry checks the Run entry received no call at all.
func (f *autostartFixture) assertNoRegistry(t *testing.T) {
	t.Helper()
	f.assertRun(t, 0, 0, 0)
}

// assertSetValues checks the values passed to Set.
func (f *autostartFixture) assertSetValues(t *testing.T, want ...string) {
	t.Helper()
	f.run.mu.Lock()
	got := append([]string(nil), f.run.setValues...)
	f.run.mu.Unlock()
	if len(got) != len(want) {
		t.Fatalf("Set values = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Set value #%d = %q, want %q", i, got[i], want[i])
		}
	}
}

// assertWrites checks the preference writes (key autostart, value, clock instant).
func (f *autostartFixture) assertWrites(t *testing.T, values ...string) {
	t.Helper()
	_, writes := f.settings.snapshot()
	if len(writes) != len(values) {
		t.Fatalf("SetSetting calls = %+v, want values %q", writes, values)
	}
	for i, w := range writes {
		if w.key != settingAutostart {
			t.Errorf("SetSetting #%d key = %q, want %q", i, w.key, settingAutostart)
		}
		if w.value != values[i] {
			t.Errorf("SetSetting #%d value = %q, want %q", i, w.value, values[i])
		}
		if !w.at.Equal(f.clock.Now()) {
			t.Errorf("SetSetting #%d at = %v, want clock instant %v", i, w.at, f.clock.Now())
		}
	}
}

// assertErrorOp checks exactly one Error "início com o Windows: falha" with op
// and a non-empty error attribute, and no other Error record.
func (f *autostartFixture) assertErrorOp(t *testing.T, op string) {
	t.Helper()
	recs := f.logs.recordsWith(slog.LevelError, autostartErrMsg)
	if len(recs) != 1 {
		t.Fatalf("%d Error %q records, want 1", len(recs), autostartErrMsg)
	}
	if n := f.logs.count(slog.LevelError); n != 1 {
		t.Errorf("%d Error records in total, want 1", n)
	}
	v, ok := recordAttr(recs[0], "op")
	if !ok {
		t.Fatalf("Error record has no op attribute")
	}
	if v.String() != op {
		t.Errorf("Error op = %q, want %q", v.String(), op)
	}
	ev, ok := recordAttr(recs[0], "error")
	if !ok {
		t.Fatalf("Error record has no error attribute")
	}
	if !strings.Contains(fmt.Sprint(ev.Any()), errFake.Error()) {
		t.Errorf("Error error = %v, want it to contain %q", ev.Any(), errFake.Error())
	}
}

func (f *autostartFixture) assertNoErrors(t *testing.T) {
	t.Helper()
	if n := f.logs.count(slog.LevelError); n != 0 {
		t.Errorf("%d Error records, want 0", n)
	}
}

func TestReconcile(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("AST-003/chave ausente grava true com o instante do relógio, faz Set(command) e devolve true", func(t *testing.T) {
		t.Parallel()
		f := newAutostartFixture(autostartSetup{})
		if got := f.a.Reconcile(ctx); !got {
			t.Errorf("Reconcile = false, want true")
		}
		f.assertWrites(t, "true")
		f.assertSetValues(t, autostartTestCommand)
		_, sets, deletes := f.run.counts()
		if sets != 1 || deletes != 0 {
			t.Errorf("Set/Delete = %d/%d, want 1/0", sets, deletes)
		}
		// The preference is written before the registry is touched.
		tr := f.trace.list()
		iw, is := indexOf(tr, "settings.SetSetting"), indexOf(tr, "run.Set")
		if iw < 0 || is < 0 || iw > is {
			t.Errorf("call order %v, want settings.SetSetting before run.Set", tr)
		}
		f.assertNoErrors(t)
	})

	cases := []struct {
		name            string
		setup           autostartSetup
		want            bool
		sets, deletes   int
		setValues       []string
		exeOfCalled     bool
		warn            bool
		errOp           string // "" => no Error
		readsRegistry   bool
		noRegistryAtAll bool
		writesPref      []string
	}{
		{
			name:          "AST-004/ligado e valor ausente faz Set",
			setup:         autostartSetup{pref: "true"},
			want:          true,
			sets:          1,
			setValues:     []string{autostartTestCommand},
			readsRegistry: true,
		},
		{
			name:          "AST-004/ligado e valor diferente faz Set",
			setup:         autostartSetup{pref: "true", run: `"D:\Velho\arcadia.exe" --agent`},
			want:          true,
			sets:          1,
			setValues:     []string{autostartTestCommand},
			readsRegistry: true,
		},
		{
			name:          "AST-004/ligado e valor igual não escreve",
			setup:         autostartSetup{pref: "true", run: autostartTestCommand},
			want:          true,
			readsRegistry: true,
		},
		{
			name:          "AST-005/desligado e mesmo exe (caixa diferente) faz Delete",
			setup:         autostartSetup{pref: "false", run: `"c:\jogos\ARCADIA\Arcadia.EXE" --agent --data-dir D:\`},
			want:          false,
			deletes:       1,
			exeOfCalled:   true,
			readsRegistry: true,
		},
		{
			name:          "AST-005/desligado e outro exe não altera",
			setup:         autostartSetup{pref: "false", run: `"C:\Outro\arcadia.exe" --agent`},
			want:          false,
			exeOfCalled:   true,
			readsRegistry: true,
		},
		{
			name:          "AST-005/desligado e valor ausente não altera",
			setup:         autostartSetup{pref: "false"},
			want:          false,
			readsRegistry: true,
		},
		{
			name:          "AST-005/desligado e valor ilegível não remove e loga Warn",
			setup:         autostartSetup{pref: "false", run: `"C:\Jogos\Arcadia\arcadia.exe --agent`},
			want:          false,
			exeOfCalled:   true,
			warn:          true,
			readsRegistry: true,
		},
		{
			name:            "AST-009/falha de read-setting loga Error e não toca o registro",
			setup:           autostartSetup{pref: "true", readErr: errFake},
			want:            false,
			errOp:           "read-setting",
			noRegistryAtAll: true,
		},
		{
			name:            "AST-009/falha de write-setting na primeira execução loga Error e não toca o registro",
			setup:           autostartSetup{writeErr: errFake},
			want:            false,
			errOp:           "write-setting",
			noRegistryAtAll: true,
			writesPref:      []string{"true"},
		},
		{
			name:          "AST-009/falha de Get loga op=read-run",
			setup:         autostartSetup{pref: "true", getErr: errFake},
			want:          true, // AST-006: the item reflects the stored preference
			errOp:         "read-run",
			readsRegistry: true,
		},
		{
			name:          "AST-009/falha de Set loga op=write-run",
			setup:         autostartSetup{pref: "true", setErr: errFake},
			want:          true,
			sets:          1,
			setValues:     []string{autostartTestCommand},
			errOp:         "write-run",
			readsRegistry: true,
		},
		{
			name:          "AST-009/falha de Delete loga op=delete-run",
			setup:         autostartSetup{pref: "false", run: autostartTestCommand, delErr: errFake},
			want:          false,
			deletes:       1,
			exeOfCalled:   true,
			errOp:         "delete-run",
			readsRegistry: true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			f := newAutostartFixture(c.setup)
			if got := f.a.Reconcile(ctx); got != c.want {
				t.Errorf("Reconcile = %v, want %v", got, c.want)
			}

			if c.noRegistryAtAll {
				f.assertNoRegistry(t)
			} else {
				gets, sets, deletes := f.run.counts()
				if c.readsRegistry && gets == 0 {
					t.Errorf("run.Get not called, want it read before deciding (trace %v)", f.trace.list())
				}
				if sets != c.sets || deletes != c.deletes {
					t.Errorf("Set/Delete = %d/%d, want %d/%d (trace %v)", sets, deletes, c.sets, c.deletes, f.trace.list())
				}
				if c.sets > 0 {
					f.assertSetValues(t, c.setValues...)
				}
			}

			// Existing preferences are never rewritten by Reconcile.
			f.assertWrites(t, c.writesPref...)

			if c.exeOfCalled && f.exe.count() == 0 {
				t.Errorf("exeOf not called, want the first token compared with exe")
			}

			warns := f.logs.count(slog.LevelWarn)
			if c.warn && warns == 0 {
				t.Errorf("no Warn record, want one for the unreadable Run value")
			}
			if !c.warn && warns != 0 {
				t.Errorf("%d Warn records, want 0", warns)
			}

			if c.errOp != "" {
				f.assertErrorOp(t, c.errOp)
			} else {
				f.assertNoErrors(t)
			}
		})
	}
}

func TestToggle(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("AST-007/ligado: grava false, remove a entrada própria e devolve false", func(t *testing.T) {
		t.Parallel()
		f := newAutostartFixture(autostartSetup{enabled: true, pref: "true", run: autostartTestCommand})
		if got := f.a.Toggle(ctx); got {
			t.Errorf("Toggle = true, want false")
		}
		f.assertWrites(t, "false")
		_, sets, deletes := f.run.counts()
		if sets != 0 || deletes != 1 {
			t.Errorf("Set/Delete = %d/%d, want 0/1", sets, deletes)
		}
		assertBefore(t, f.trace.list(), "settings.SetSetting", "run.Delete")
		f.assertNoErrors(t)
	})

	t.Run("AST-007/ligado com entrada de outro exe: grava false e não remove", func(t *testing.T) {
		t.Parallel()
		f := newAutostartFixture(autostartSetup{enabled: true, pref: "true", run: `"C:\Outro\arcadia.exe" --agent`})
		if got := f.a.Toggle(ctx); got {
			t.Errorf("Toggle = true, want false")
		}
		f.assertWrites(t, "false")
		_, sets, deletes := f.run.counts()
		if sets != 0 || deletes != 0 {
			t.Errorf("Set/Delete = %d/%d, want 0/0", sets, deletes)
		}
		f.assertNoErrors(t)
	})

	t.Run("AST-008/desligado: grava true, faz Set(command) e devolve true", func(t *testing.T) {
		t.Parallel()
		f := newAutostartFixture(autostartSetup{enabled: false, pref: "false"})
		if got := f.a.Toggle(ctx); !got {
			t.Errorf("Toggle = false, want true")
		}
		f.assertWrites(t, "true")
		f.assertSetValues(t, autostartTestCommand)
		_, _, deletes := f.run.counts()
		if deletes != 0 {
			t.Errorf("Delete called %d times, want 0", deletes)
		}
		assertBefore(t, f.trace.list(), "settings.SetSetting", "run.Set")
		f.assertNoErrors(t)
	})

	t.Run("AST-009/falha de write-setting devolve o estado antigo sem tocar o registro", func(t *testing.T) {
		t.Parallel()
		for _, old := range []bool{true, false} {
			pref := "false"
			if old {
				pref = "true"
			}
			f := newAutostartFixture(autostartSetup{enabled: old, pref: pref, run: autostartTestCommand, writeErr: errFake})
			if got := f.a.Toggle(ctx); got != old {
				t.Errorf("old=%v: Toggle = %v, want the old state %v", old, got, old)
			}
			f.assertNoRegistry(t)
			f.assertErrorOp(t, "write-setting")
		}
	})

	t.Run("AST-009/falha de registro devolve o estado novo e loga Error", func(t *testing.T) {
		t.Parallel()
		t.Run("ligar com Set falhando", func(t *testing.T) {
			t.Parallel()
			f := newAutostartFixture(autostartSetup{enabled: false, pref: "false", setErr: errFake})
			if got := f.a.Toggle(ctx); !got {
				t.Errorf("Toggle = false, want true (new state)")
			}
			f.assertWrites(t, "true")
			f.assertErrorOp(t, "write-run")

			// The new state holds: the next click turns it off again.
			if got := f.a.Toggle(ctx); got {
				t.Errorf("second Toggle = true, want false")
			}
			f.assertWrites(t, "true", "false")
		})
		t.Run("desligar com Delete falhando", func(t *testing.T) {
			t.Parallel()
			f := newAutostartFixture(autostartSetup{enabled: true, pref: "true", run: autostartTestCommand, delErr: errFake})
			if got := f.a.Toggle(ctx); got {
				t.Errorf("Toggle = true, want false (new state)")
			}
			f.assertWrites(t, "false")
			f.assertErrorOp(t, "delete-run")
		})
		t.Run("desligar com Get falhando", func(t *testing.T) {
			t.Parallel()
			f := newAutostartFixture(autostartSetup{enabled: true, pref: "true", getErr: errFake})
			if got := f.a.Toggle(ctx); got {
				t.Errorf("Toggle = true, want false (new state)")
			}
			f.assertWrites(t, "false")
			_, _, deletes := f.run.counts()
			if deletes != 0 {
				t.Errorf("Delete called %d times after a failed Get, want 0", deletes)
			}
			f.assertErrorOp(t, "read-run")
		})
	})
}

func TestAutostartDev(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("AST-010/Reconcile em dev não chama registro nem preferência, devolve false e loga Info uma vez", func(t *testing.T) {
		t.Parallel()
		f := newAutostartFixture(autostartSetup{dev: true, pref: "true", run: autostartTestCommand})
		if got := f.a.Reconcile(ctx); got {
			t.Errorf("Reconcile = true, want false")
		}
		f.assertNoRegistry(t)
		if gets, writes := f.settings.snapshot(); gets != 0 || len(writes) != 0 {
			t.Errorf("settings Setting/SetSetting = %d/%d calls, want 0/0", gets, len(writes))
		}
		if n := f.exe.count(); n != 0 {
			t.Errorf("exeOf called %d times, want 0", n)
		}
		if n := len(f.logs.recordsWith(slog.LevelInfo, autostartDevMsg)); n != 1 {
			t.Errorf("%d Info %q records, want 1", n, autostartDevMsg)
		}
		f.assertNoErrors(t)
	})

	t.Run("AST-010/Toggle em dev não chama nada e devolve false", func(t *testing.T) {
		t.Parallel()
		f := newAutostartFixture(autostartSetup{dev: true, pref: "false"})
		for i := 0; i < 2; i++ {
			if got := f.a.Toggle(ctx); got {
				t.Errorf("Toggle #%d = true, want false", i+1)
			}
		}
		f.assertNoRegistry(t)
		if gets, writes := f.settings.snapshot(); gets != 0 || len(writes) != 0 {
			t.Errorf("settings Setting/SetSetting = %d/%d calls, want 0/0", gets, len(writes))
		}
		if n := f.exe.count(); n != 0 {
			t.Errorf("exeOf called %d times, want 0", n)
		}
		f.assertNoErrors(t)
	})
}

func indexOf(list []string, s string) int {
	for i, v := range list {
		if v == s {
			return i
		}
	}
	return -1
}

func assertBefore(t *testing.T, trace []string, first, second string) {
	t.Helper()
	i, j := indexOf(trace, first), indexOf(trace, second)
	if i < 0 || j < 0 || i > j {
		t.Errorf("call order %v, want %s before %s", trace, first, second)
	}
}
