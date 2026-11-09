package main

// Tests for the build version stamped with -ldflags "-X main.version=<v>" (AGT-004).

import "testing"

func TestVersion(t *testing.T) {
	t.Parallel()
	t.Run("AGT-004/sem -X version é dev", func(t *testing.T) {
		t.Parallel()
		if version != "dev" {
			t.Errorf("version = %q, want %q (test binaries are built without -X main.version)", version, "dev")
		}
	})
}
