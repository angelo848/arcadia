package main

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"testing"
)

// manifestDoc is the part of the Wails manifest the requirement is about.
type manifestDoc struct {
	TrustInfo struct {
		Security struct {
			RequestedPrivileges struct {
				Level []struct {
					Level string `xml:"level,attr"`
				} `xml:"requestedExecutionLevel"`
			} `xml:"requestedPrivileges"`
		} `xml:"security"`
	} `xml:"trustInfo"`
}

// TestManifest checks the Windows manifest embedded by `wails build` (ADM-001): the
// executable asks for administrator rights when it is opened.
func TestManifest(t *testing.T) {
	t.Run("ADM-001/manifesto pede requireAdministrator", func(t *testing.T) {
		raw, err := os.ReadFile(filepath.Join("build", "windows", "wails.exe.manifest"))
		if err != nil {
			t.Fatalf("read manifest: %v", err)
		}
		var doc manifestDoc
		// The file is a Go template, but its placeholders are valid XML attribute text.
		if err := xml.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("parse manifest: %v", err)
		}
		levels := doc.TrustInfo.Security.RequestedPrivileges.Level
		if len(levels) != 1 {
			t.Fatalf("requestedExecutionLevel count = %d, want 1", len(levels))
		}
		if got := levels[0].Level; got != "requireAdministrator" {
			t.Errorf("requestedExecutionLevel = %q, want requireAdministrator", got)
		}
	})
}
