package tracker

import (
	"errors"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
)

// internalLister is a scripted snapshot source for the white-box tests.
type internalLister struct {
	procs []ProcInfo
}

func (l *internalLister) List() ([]ProcInfo, error) { return l.procs, nil }

func TestMatchesAny(t *testing.T) {
	t.Parallel()

	rules := []Rule{
		{Kind: RuleInstallDir, Value: `C:\Games\CS2`},
		{Kind: RuleExePath, Value: `D:\Jogos\ABZU.exe`},
		{Kind: RuleExeName, Value: "launcher.exe"},
	}
	cases := []struct {
		name string
		proc ProcInfo
		want bool
	}{
		{"RUL-002/pasta de instalação casa qualquer exe dentro dela", ProcInfo{PID: 1, Path: `C:\Games\CS2\bin\cs2.exe`}, true},
		{"RUL-002/casa sem diferença de maiúsculas", ProcInfo{PID: 1, Path: `c:\GAMES\cs2\CS2.EXE`}, true},
		{"RUL-002/pasta vizinha com prefixo igual não casa", ProcInfo{PID: 1, Path: `C:\Games\CS2-old\cs2.exe`}, false},
		{"RUL-002/caminho exato casa", ProcInfo{PID: 2, Path: `D:\Jogos\ABZU.exe`}, true},
		{"RUL-002/caminho exato de outra pasta não casa", ProcInfo{PID: 2, Path: `D:\Outra\ABZU.exe`}, false},
		{"RUL-002/nome do executável casa em qualquer pasta", ProcInfo{PID: 3, Path: `E:\x\Launcher.exe`}, true},
		{"RUL-002/processo sem caminho legível não casa", ProcInfo{PID: 4}, false},
		{"RUL-002/processo de sistema não casa", ProcInfo{PID: 5, Path: `C:\Windows\explorer.exe`}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := matchesAny(c.proc, rules); got != c.want {
				t.Errorf("matchesAny(%+v) = %v, want %v", c.proc, got, c.want)
			}
		})
	}
}

func TestAdoptedJob(t *testing.T) {
	t.Parallel()

	t.Run("RUL-005/Terminate não mata processo que o agente não iniciou", func(t *testing.T) {
		t.Parallel()
		clock := domain.NewFakeClock(time.Date(2026, time.November, 12, 18, 0, 0, 0, time.UTC))
		j, err := newAdoptedJob(&internalLister{}, []Rule{{Kind: RuleExeName, Value: "g.exe"}}, clock, time.Second, nil)
		if err != nil {
			t.Fatalf("newAdoptedJob: %v", err)
		}
		defer func() { _ = j.Close() }()
		if err := j.Terminate(); !errors.Is(err, ErrNotOwned) {
			t.Errorf("Terminate = %v, want ErrNotOwned", err)
		}
		if err := j.Resume(); err != nil {
			t.Errorf("Resume = %v, want nil", err)
		}
	})

	t.Run("RUL-002/Close para o watcher, fecha o canal de eventos e é idempotente", func(t *testing.T) {
		t.Parallel()
		clock := domain.NewFakeClock(time.Date(2026, time.November, 12, 18, 0, 0, 0, time.UTC))
		j, err := newAdoptedJob(&internalLister{}, nil, clock, time.Second, nil)
		if err != nil {
			t.Fatalf("newAdoptedJob: %v", err)
		}
		if err := j.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		if err := j.Close(); err != nil {
			t.Fatalf("second Close: %v", err)
		}
		if _, ok := <-j.Events(); ok {
			t.Error("Events still open after Close")
		}
	})

	t.Run("RUL-002/baseline com erro não cria o job", func(t *testing.T) {
		t.Parallel()
		clock := domain.NewFakeClock(time.Date(2026, time.November, 12, 18, 0, 0, 0, time.UTC))
		_, err := newAdoptedJob(&failingLister{}, nil, clock, time.Second, nil)
		if err == nil {
			t.Error("newAdoptedJob with a failing baseline returned nil error")
		}
	})
}

type failingLister struct{}

func (failingLister) List() ([]ProcInfo, error) { return nil, errors.New("snapshot failed") }
