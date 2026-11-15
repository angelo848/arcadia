package tracker

import (
	"os"
	"regexp"
	"strconv"
	"testing"
	"time"
)

// White-box: closeWait and detachedPollEvery are unexported; they must equal
// x-console of the contract.
func TestCloseConstantsMatchContract(t *testing.T) {
	t.Parallel()
	t.Run("CON-030/closeWait e detachedPollEvery iguais a x-console.closeWaitMs e closeDetachedPollMs do delta", func(t *testing.T) {
		raw, err := os.ReadFile("../../docs/specs/phase-13-modo-console/contracts/ipc-v0.15.0-delta.md")
		if err != nil {
			t.Fatalf("read delta: %v", err)
		}
		get := func(key string) time.Duration {
			m := regexp.MustCompile(`"` + key + `":\s*(\d+)`).FindSubmatch(raw)
			if m == nil {
				t.Fatalf("key %s not found in the delta", key)
			}
			n, _ := strconv.Atoi(string(m[1]))
			return time.Duration(n) * time.Millisecond
		}
		wait, poll := get("closeWaitMs"), get("closeDetachedPollMs")
		if wait != 3*time.Second {
			t.Errorf("closeWaitMs = %v, want 3s", wait)
		}
		if poll != 250*time.Millisecond {
			t.Errorf("closeDetachedPollMs = %v, want 250ms", poll)
		}
		if closeWait != wait {
			t.Errorf("closeWait = %v, want %v", closeWait, wait)
		}
		if detachedPollEvery != poll {
			t.Errorf("detachedPollEvery = %v, want %v", detachedPollEvery, poll)
		}
	})
}
