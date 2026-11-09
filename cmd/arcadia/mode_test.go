package main

// Tests for the command-line mode selection (EXE-001).

import "testing"

func TestIsAgentMode(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		args []string
		want bool
	}{
		{"EXE-001/[] é UI", []string{}, false},
		{"EXE-001/--data-dir X é UI", []string{"--data-dir", "X"}, false},
		{"EXE-001/--agent é agente", []string{"--agent"}, true},
		{"EXE-001/-agent é agente", []string{"-agent"}, true},
		{"EXE-001/--data-dir X --agent é agente", []string{"--data-dir", "X", "--agent"}, true},
		{"EXE-001/--agent=false é UI", []string{"--agent=false"}, false},
		{"EXE-001/--agents é UI", []string{"--agents"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := isAgentMode(tc.args); got != tc.want {
				t.Errorf("isAgentMode(%q) = %v, want %v", tc.args, got, tc.want)
			}
		})
	}
	t.Run("EXE-001/nil é UI", func(t *testing.T) {
		t.Parallel()
		if isAgentMode(nil) {
			t.Error("isAgentMode(nil) = true, want false")
		}
	})
}
