package main

// Tests for T25 (docs/specs/phase-15-big-box-v2): --bigbox, second instance and TakeBigBoxRequest
// [ATB-008, ATB-009, ATB-012, ATB-014, ATB-015].
//
// Assumed symbols (T26): desktopConfig.BigBox bool, wantsBigBox([]string) bool,
// (*App).TakeBigBoxRequest() bool, (*App).secondInstance(bigBox bool).

import (
	"context"
	"flag"
	"io"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

const bigBoxEvent = "ui:bigBoxRequested"

// stepLog records the order of platform calls.
type stepLog struct {
	mu    sync.Mutex
	steps []string
	data  []any
}

func (s *stepLog) add(step string, data any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.steps = append(s.steps, step)
	s.data = append(s.data, data)
}

func (s *stepLog) recorded() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.steps...)
}

// newBigBoxApp returns a started-looking App (wailsCtx set) whose platform records every call in
// order, with an agent spy that must never be called.
func newBigBoxApp(fg bool, fgErr error) (*App, *stepLog, *gamePinger) {
	p := &gamePinger{}
	a := newBindingApp(p, &gameEmitLog{})
	steps := &stepLog{}
	a.platform.showWindow = func(context.Context) { steps.add("restore", nil) }
	a.platform.bringToFront = func() (bool, error) {
		steps.add("front", nil)
		return fg, fgErr
	}
	a.platform.emit = func(_ context.Context, event string, data any) {
		steps.add("emit:"+event, data)
	}
	return a, steps, p
}

func TestWantsBigBox(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		args []string
		want bool
	}{
		{"ATB-009/--bigbox", []string{"--bigbox"}, true},
		{"ATB-009/-bigbox", []string{"-bigbox"}, true},
		{"ATB-009/--bigbox=true", []string{"--bigbox=true"}, true},
		{"ATB-009/--bigbox=false não pede", []string{"--bigbox=false"}, false},
		{"ATB-009/-bigbox=false não pede", []string{"-bigbox=false"}, false},
		{"ATB-009/sem argumentos", []string{}, false},
		{"ATB-009/nil", nil, false},
		{"ATB-009/nome do exe na frente", []string{`C:\Arcadia\arcadia.exe`, "--bigbox"}, true},
		{"ATB-009/nome do exe na frente sem flag", []string{`C:\Arcadia\arcadia.exe`}, false},
		{"ATB-009/flag desconhecido antes", []string{"--wails-foo", "--bigbox"}, true},
		{"ATB-009/flag desconhecido depois", []string{"--bigbox", "--wails-foo=1"}, true},
		{"ATB-009/--data-dir diferente é ignorado", []string{"--data-dir", `D:\outra`, "--bigbox"}, true},
		{"ATB-009/--data-dir diferente sem bigbox", []string{"--data-dir", `D:\outra`}, false},
		{"ATB-009/--data-dir=X --bigbox", []string{`--data-dir=D:\outra`, "--bigbox"}, true},
		{"ATB-009/look-alike --bigboxes não conta", []string{"--bigboxes"}, false},
		{"ATB-009/look-alike --nobigbox não conta", []string{"--nobigbox"}, false},
		{"ATB-009/flag desconhecido não derruba o que vem depois", []string{"--zzz", "valor", "-bigbox"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := wantsBigBox(tc.args); got != tc.want {
				t.Errorf("wantsBigBox(%q) = %v, want %v", tc.args, got, tc.want)
			}
		})
	}
}

func TestBigBoxUIArgs(t *testing.T) {
	t.Parallel()

	// The UI parse in runUI uses a flag.FlagSet with fset.BoolVar(&cfg.BigBox, "bigbox", ...) (T26).
	// This mirrors that declaration to pin the boolean syntax the spec promises (ATB-008).
	parse := func(args []string) desktopConfig {
		var cfg desktopConfig
		fset := flag.NewFlagSet("arcadia", flag.ContinueOnError)
		fset.SetOutput(io.Discard)
		fset.StringVar(&cfg.DataDir, "data-dir", "", "")
		fset.BoolVar(&cfg.BigBox, "bigbox", false, "")
		_ = fset.Parse(args)
		return cfg
	}
	cases := []struct {
		name string
		args []string
		want bool
	}{
		{"ATB-008/--bigbox pede", []string{"--bigbox"}, true},
		{"ATB-008/-bigbox pede", []string{"-bigbox"}, true},
		{"ATB-008/--bigbox=true pede", []string{"--bigbox=true"}, true},
		{"ATB-008/--bigbox=false não pede", []string{"--bigbox=false"}, false},
		{"ATB-008/sem o flag nada é pedido", []string{"--data-dir", "X"}, false},
		{"ATB-008/com --data-dir e --bigbox", []string{"--data-dir", "X", "--bigbox"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := parse(tc.args).BigBox; got != tc.want {
				t.Errorf("cfg.BigBox = %v, want %v", got, tc.want)
			}
		})
	}

	t.Run("ATB-008/com --agent o modo é o do agente e --bigbox é ignorado", func(t *testing.T) {
		t.Parallel()
		if !isAgentMode([]string{"--agent", "--bigbox"}) {
			t.Error("isAgentMode(--agent --bigbox) = false, want true")
		}
		if isAgentMode([]string{"--bigbox"}) {
			t.Error("isAgentMode(--bigbox) = true, want false")
		}
	})

	t.Run("ATB-008/NewApp com cfg.BigBox guarda o pedido; TakeBigBoxRequest o devolve", func(t *testing.T) {
		t.Parallel()
		a := NewApp(desktopConfig{BigBox: true})
		if !a.TakeBigBoxRequest() {
			t.Error("first TakeBigBoxRequest = false, want true")
		}
		if a.TakeBigBoxRequest() {
			t.Error("second TakeBigBoxRequest = true, want false")
		}
	})

	t.Run("ATB-008/NewApp sem cfg.BigBox não guarda pedido", func(t *testing.T) {
		t.Parallel()
		if NewApp(desktopConfig{}).TakeBigBoxRequest() {
			t.Error("TakeBigBoxRequest = true, want false")
		}
	})
}

func TestSecondInstanceBigBox(t *testing.T) {
	t.Parallel()

	t.Run("ATB-009/secondInstance(true): restaurar, bringToFront, guardar o pedido, emit sem dados", func(t *testing.T) {
		t.Parallel()
		a, steps, p := newBigBoxApp(true, nil)
		// The request must already be stored when the event is emitted: check it inside emit.
		var takenAtEmit atomic.Bool
		inner := a.platform.emit
		a.platform.emit = func(ctx context.Context, event string, data any) {
			if event == bigBoxEvent {
				takenAtEmit.Store(a.TakeBigBoxRequest())
			}
			inner(ctx, event, data)
		}
		a.secondInstance(true)

		want := []string{"restore", "front", "emit:" + bigBoxEvent}
		if got := steps.recorded(); !reflect.DeepEqual(got, want) {
			t.Fatalf("platform steps = %v, want %v", got, want)
		}
		if !takenAtEmit.Load() {
			t.Error("pedido não estava guardado quando o evento foi emitido")
		}
		if d := steps.data[2]; d != nil {
			t.Errorf("event data = %v, want nil", d)
		}
		if calls := p.recorded(); len(calls) != 0 {
			t.Errorf("agent calls = %+v, want none", calls)
		}
	})

	t.Run("ATB-009/secondInstance(true) deixa o pedido para um único consumo", func(t *testing.T) {
		t.Parallel()
		a, _, _ := newBigBoxApp(true, nil)
		a.secondInstance(true)
		if !a.TakeBigBoxRequest() {
			t.Error("first TakeBigBoxRequest = false, want true")
		}
		if a.TakeBigBoxRequest() {
			t.Error("second TakeBigBoxRequest = true, want false")
		}
	})

	t.Run("ATB-009/secondInstance(false) segue EXE-011: janela à frente, sem pedido e sem evento", func(t *testing.T) {
		t.Parallel()
		a, steps, p := newBigBoxApp(true, nil)
		a.secondInstance(false)
		for _, s := range steps.recorded() {
			if strings.HasPrefix(s, "emit:") {
				t.Errorf("unexpected event %q", s)
			}
		}
		if got := steps.recorded(); len(got) == 0 || got[0] != "restore" {
			t.Errorf("steps = %v, want the window restored (EXE-011)", got)
		}
		if a.TakeBigBoxRequest() {
			t.Error("TakeBigBoxRequest = true, want false")
		}
		if calls := p.recorded(); len(calls) != 0 {
			t.Errorf("agent calls = %+v, want none", calls)
		}
	})

	t.Run("ATB-009/antes do startup (sem wailsCtx) só o pedido é guardado e nenhum emit acontece", func(t *testing.T) {
		t.Parallel()
		a, steps, _ := newBigBoxApp(true, nil)
		a.wailsCtx = nil
		a.secondInstance(true)
		for _, s := range steps.recorded() {
			if strings.HasPrefix(s, "emit:") {
				t.Errorf("emit %q before startup", s)
			}
		}
		if !a.TakeBigBoxRequest() {
			t.Error("pedido não foi guardado antes do startup")
		}
	})

	t.Run("ATB-009/recusa do primeiro plano (false) não é erro: pedido e evento seguem", func(t *testing.T) {
		t.Parallel()
		a, steps, _ := newBigBoxApp(false, nil)
		a.secondInstance(true)
		want := []string{"restore", "front", "emit:" + bigBoxEvent}
		if got := steps.recorded(); !reflect.DeepEqual(got, want) {
			t.Errorf("platform steps = %v, want %v", got, want)
		}
		if !a.TakeBigBoxRequest() {
			t.Error("pedido não guardado")
		}
	})

	t.Run("ATB-009/depois do OnShutdown nada acontece", func(t *testing.T) {
		t.Parallel()
		a, steps, _ := newBigBoxApp(true, nil)
		a.stopped = true
		a.secondInstance(true)
		if got := steps.recorded(); len(got) != 0 {
			t.Errorf("platform steps after shutdown = %v, want none", got)
		}
	})
}

func TestTakeBigBoxRequest(t *testing.T) {
	t.Parallel()

	t.Run("ATB-012/devolve true uma única vez e depois false até um pedido novo", func(t *testing.T) {
		t.Parallel()
		a, _, _ := newBigBoxApp(true, nil)
		if a.TakeBigBoxRequest() {
			t.Error("sem pedido: true, want false")
		}
		a.secondInstance(true)
		if !a.TakeBigBoxRequest() || a.TakeBigBoxRequest() {
			t.Error("após um pedido: want true e depois false")
		}
		a.secondInstance(true)
		if !a.TakeBigBoxRequest() {
			t.Error("pedido novo: want true")
		}
	})

	t.Run("ATB-012/100 chamadas concorrentes entregam exatamente um true", func(t *testing.T) {
		t.Parallel()
		a := NewApp(desktopConfig{BigBox: true})
		var trues atomic.Int32
		var wg sync.WaitGroup
		start := make(chan struct{})
		for range 100 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				if a.TakeBigBoxRequest() {
					trues.Add(1)
				}
			}()
		}
		close(start)
		wg.Wait()
		if n := trues.Load(); n != 1 {
			t.Errorf("true count = %d, want 1", n)
		}
	})
}

func TestBigBoxContract(t *testing.T) {
	t.Parallel()

	t.Run("ATB-014/TakeBigBoxRequest(): bool existe como binding", func(t *testing.T) {
		t.Parallel()
		m, ok := reflect.TypeOf(&App{}).MethodByName("TakeBigBoxRequest")
		if !ok {
			t.Fatal("*App has no method TakeBigBoxRequest")
		}
		if m.Type.NumIn() != 1 || m.Type.NumOut() != 1 || m.Type.Out(0).Kind() != reflect.Bool {
			t.Errorf("TakeBigBoxRequest signature = %v, want func() bool", m.Type)
		}
	})

	t.Run("ATB-014/TakeBigBoxRequest nunca chama o agente", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{}
		a := NewApp(desktopConfig{BigBox: true})
		a.client = p
		_ = a.TakeBigBoxRequest()
		_ = a.TakeBigBoxRequest()
		if calls := p.recorded(); len(calls) != 0 {
			t.Errorf("agent calls = %+v, want none", calls)
		}
	})

	t.Run("ATB-014/o evento ui:bigBoxRequested não leva dados e não chama o agente", func(t *testing.T) {
		t.Parallel()
		a, steps, p := newBigBoxApp(true, nil)
		a.secondInstance(true)
		found := false
		for i, s := range steps.recorded() {
			if s == "emit:"+bigBoxEvent {
				found = true
				if steps.data[i] != nil {
					t.Errorf("event data = %v, want nil", steps.data[i])
				}
			}
		}
		if !found {
			t.Errorf("event %q not emitted", bigBoxEvent)
		}
		if calls := p.recorded(); len(calls) != 0 {
			t.Errorf("agent calls = %+v, want none", calls)
		}
	})

	t.Run("ATB-015/os caminhos desta área não chamam nenhum método de sessão no agente", func(t *testing.T) {
		t.Parallel()
		a, _, p := newBigBoxApp(true, nil)
		a.secondInstance(true)
		a.secondInstance(false)
		_ = a.TakeBigBoxRequest()
		_ = wantsBigBox([]string{"--bigbox"})
		for _, c := range p.recorded() {
			t.Errorf("agent call %q, want none", c.method)
		}
	})
}
