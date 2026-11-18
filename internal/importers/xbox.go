package importers

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// xboxSource is the Xbox app / Microsoft Store importer: the PC games installed by Gaming Services.
type xboxSource struct {
	roots func() ([]string, error)
}

// Xbox returns the Xbox source. It ignores ScanInput.Path: the install folders of the games come from
// the Gaming Services package repository of this machine (see xbox_windows.go).
func Xbox() Source { return xboxSource{roots: xboxInstallRoots} }

// XboxFrom is Xbox with the list of install folders given, for tests.
func XboxFrom(roots func() ([]string, error)) Source { return xboxSource{roots: roots} }

func (xboxSource) ID() string { return SourceXbox }

// xboxConfig is the part of MicrosoftGame.Config the importer needs.
type xboxConfig struct {
	Visuals struct {
		Name string `xml:"DefaultDisplayName,attr"`
	} `xml:"ShellVisuals"`
	StoreID string `xml:"StoreId"`
}

// xboxManifest is the part of appxmanifest.xml the importer needs: the id of the first application,
// which is the "Exe" of the launch link.
type xboxManifest struct {
	Apps []struct {
		ID string `xml:"Id,attr"`
	} `xml:"Applications>Application"`
}

var xboxTitleCleaner = strings.NewReplacer("®", "", "™", "", "©", "")

func (s xboxSource) Scan(ctx context.Context, in ScanInput) (Result, error) {
	var res Result
	if err := ctx.Err(); err != nil {
		return res, err
	}
	roots, err := s.roots()
	if err != nil {
		res.Warnings = append(res.Warnings, "Xbox: lista de jogos instalados indisponível")
		return res, nil
	}
	unnamed := 0
	for _, root := range roots {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		content := root // the package folder: a junction to the real install folder
		var cfg xboxConfig
		if err := readXML(filepath.Join(content, "MicrosoftGame.Config"), &cfg); err != nil {
			continue // not a game package (DLC, app) or not readable
		}
		var man xboxManifest
		if err := readXML(filepath.Join(content, "appxmanifest.xml"), &man); err != nil {
			continue
		}
		storeID := strings.TrimSpace(cfg.StoreID)
		if storeID == "" || len(man.Apps) == 0 || man.Apps[0].ID == "" {
			continue // a package without a store id or an application cannot be launched
		}
		title := strings.TrimSpace(xboxTitleCleaner.Replace(cfg.Visuals.Name))
		if title == "" || strings.HasPrefix(strings.ToLower(title), "ms-resource:") {
			unnamed++
			continue
		}
		res.Candidates = append(res.Candidates, Candidate{
			Source:       SourceXbox,
			Title:        title,
			Platform:     PlatformWindows,
			LaunchType:   LaunchURI,
			LaunchTarget: "msgamelaunch://shortcutLaunch/?ProductId=" + storeID + "&Exe=" + man.Apps[0].ID,
			InstallDir:   realDir(content),
		})
	}
	sort.SliceStable(res.Candidates, func(i, j int) bool {
		return strings.ToLower(res.Candidates[i].Title) < strings.ToLower(res.Candidates[j].Title)
	})
	if unnamed > 0 {
		res.Warnings = append(res.Warnings, fmt.Sprintf("Xbox: %d jogo(s) sem nome ignorado(s)", unnamed))
	}
	return res, nil
}

// readXML decodes the file into v, ignoring a UTF-8 byte order mark.
func readXML(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	return xml.Unmarshal(data, v)
}
