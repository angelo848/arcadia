//go:build windows

package tracker_test

import (
	"encoding/json"
	"os"
	"testing"

	"golang.org/x/sys/windows"
)

// treeDepth2 is the initial process, a child and a grandchild, all alive until
// the until-file (3 PIDs).
var treeDepth2 = []string{"--depth", "2", "--parent-run", "forever", "--run", "forever"}

// markKilled appends an exit line per start line to the report: a killed
// fake-game cannot write its own, and the scenario cleanup waits for them.
func markKilled(t *testing.T, sc *scenario) {
	t.Helper()
	lines, _ := readReport(sc.report)
	f, err := os.OpenFile(sc.report, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open report: %v", err)
	}
	defer f.Close()
	for _, l := range linesOf(lines, "start", -1) {
		b, _ := json.Marshal(reportLine{Event: "exit", PID: l.PID, Depth: l.Depth})
		if _, err := f.Write(append(b, '\n')); err != nil {
			t.Fatalf("write report: %v", err)
		}
	}
}

func TestJobRequestCloseAndKill(t *testing.T) {
	t.Parallel()

	t.Run("CON-030/winJob.RequestClose lista só os PIDs do job (fake-game --depth 2: 3 PIDs) e devolve 0 sem janela", func(t *testing.T) {
		t.Parallel()
		sc := newScenario(t)
		job := prepare(t, sc, treeDepth2...)
		resume(t, job)
		sc.waitCounts(t, 3, 0)
		n, err := job.RequestClose()
		if err != nil || n != 0 {
			t.Errorf("RequestClose = (%d, %v), want 0, nil (fake-game has no window)", n, err)
		}
		if active, err := job.ActiveProcesses(); err != nil || active != 3 {
			t.Errorf("ActiveProcesses = (%d, %v), want 3: RequestClose must not end anything", active, err)
		}
	})

	t.Run("CON-030/a lista de PIDs cresce o buffer (fake-game com muitos filhos)", func(t *testing.T) {
		t.Parallel()
		sc := newScenario(t)
		job := prepare(t, sc, "--depth", "1", "--fanout", "60", "--parent-run", "forever", "--run", "forever")
		resume(t, job)
		sc.waitCounts(t, 61, 0)
		n, err := job.RequestClose()
		if err != nil || n != 0 {
			t.Errorf("RequestClose with 61 processes = (%d, %v), want 0, nil", n, err)
		}
	})

	t.Run("CON-030/winJob.Kill mata os 3 processos (espera real limitada a 10 s)", func(t *testing.T) {
		t.Parallel()
		sc := newScenario(t)
		job := prepare(t, sc, treeDepth2...)
		resume(t, job)
		lines := sc.waitCounts(t, 3, 0)
		var hs []windows.Handle
		for _, l := range linesOf(lines, "start", -1) {
			hs = append(hs, openProcess(t, l.PID))
		}
		if len(hs) != 3 {
			t.Fatalf("start lines = %d, want 3", len(hs))
		}
		if err := job.Kill(); err != nil {
			t.Fatalf("Kill: %v", err)
		}
		for _, h := range hs {
			waitProcessExit(t, h, "killed process")
		}
		markKilled(t, sc)
	})

	t.Run("CON-030/Kill depois de Resume é permitido e Terminate continua recusado", func(t *testing.T) {
		t.Parallel()
		sc := newScenario(t)
		job := prepare(t, sc, treeDepth2...)
		resume(t, job)
		sc.waitCounts(t, 3, 0)
		root := openProcess(t, int(job.RootPID()))
		if err := job.Terminate(); err == nil {
			t.Error("Terminate after Resume = nil, want a refusal")
		}
		if err := job.Kill(); err != nil {
			t.Errorf("Kill after Resume = %v, want nil", err)
		}
		waitProcessExit(t, root, "killed root")
		markKilled(t, sc)
	})

	t.Run("CON-030/Kill depois de Close devolve erro", func(t *testing.T) {
		t.Parallel()
		sc := newScenario(t)
		job := prepare(t, sc, "--run", "forever")
		if err := job.Terminate(); err != nil {
			t.Fatalf("Terminate: %v", err)
		}
		if err := job.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		if err := job.Kill(); err == nil {
			t.Error("Kill on a closed job = nil, want an error")
		}
		if _, err := job.RequestClose(); err == nil {
			t.Error("RequestClose on a closed job = nil error, want an error")
		}
	})

	t.Run("CON-013/Kill de job vazio é nil", func(t *testing.T) {
		t.Parallel()
		sc := newScenario(t)
		job := prepare(t, sc, "--run", "forever")
		h := openProcess(t, int(job.RootPID()))
		if err := job.Terminate(); err != nil {
			t.Fatalf("Terminate: %v", err)
		}
		waitProcessExit(t, h, "terminated initial process")
		if err := job.Kill(); err != nil {
			t.Errorf("Kill of an empty job = %v, want nil", err)
		}
	})
}
