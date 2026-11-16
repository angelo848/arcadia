package main

// Tests for T15: the App.BringToFront binding [CON-023, CON-027]. The ordering of the Win32 calls
// is tested in internal/winapi (ForceForegroundWith); here we only prove that the binding uses the
// platform seam, never the agent, and treats a Windows refusal as success.
//
// Assumption: platform gains `bringToFront func() (foreground bool, err error)`.

import (
	"errors"
	"reflect"
	"sync/atomic"
	"testing"
)

func TestBringToFront(t *testing.T) {
	t.Parallel()

	t.Run("CON-027/BringToFront existe como binding", func(t *testing.T) {
		t.Parallel()
		m, ok := reflect.TypeOf(&App{}).MethodByName("BringToFront")
		if !ok {
			t.Fatal("*App has no method BringToFront")
		}
		if m.Type.NumIn() != 1 || m.Type.NumOut() != 1 {
			t.Errorf("BringToFront signature = %v, want func() error", m.Type)
		}
	})

	newApp := func(fn func() (bool, error), calls *atomic.Int32) (*App, *gamePinger) {
		p := &gamePinger{}
		a := newBindingApp(p, &gameEmitLog{})
		a.platform.bringToFront = func() (bool, error) {
			calls.Add(1)
			return fn()
		}
		return a, p
	}

	t.Run("CON-023/chama a plataforma uma vez e nunca o agente", func(t *testing.T) {
		t.Parallel()
		var n atomic.Int32
		a, p := newApp(func() (bool, error) { return true, nil }, &n)
		if err := a.BringToFront(); err != nil {
			t.Fatalf("BringToFront err = %v", err)
		}
		if n.Load() != 1 {
			t.Errorf("platform called %d times, want 1", n.Load())
		}
		if calls := p.recorded(); len(calls) != 0 {
			t.Errorf("agent calls = %+v, want none", calls)
		}
	})

	t.Run("CON-023/recusa do Windows (foreground false) devolve nil", func(t *testing.T) {
		t.Parallel()
		var n atomic.Int32
		a, p := newApp(func() (bool, error) { return false, nil }, &n)
		if err := a.BringToFront(); err != nil {
			t.Errorf("BringToFront err = %v, want nil", err)
		}
		if n.Load() != 1 || len(p.recorded()) != 0 {
			t.Errorf("platform calls %d, agent calls %v", n.Load(), p.recorded())
		}
	})

	t.Run("CON-023/erro de programação (sem janela) vira erro", func(t *testing.T) {
		t.Parallel()
		var n atomic.Int32
		a, _ := newApp(func() (bool, error) { return false, errors.New("no window") }, &n)
		if err := a.BringToFront(); err == nil {
			t.Error("BringToFront err = nil, want an error")
		}
	})
}
