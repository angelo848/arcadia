package importers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// epicSource is the Epic Games Store importer (IMP-003).
type epicSource struct{}

// Epic returns the Epic source. ScanInput.Path is the Manifests folder
// (ProgramData\Epic\EpicGamesLauncher\Data\Manifests); the agent resolves it.
func Epic() Source { return epicSource{} }

func (epicSource) ID() string { return SourceEpic }

// epicItem is the part of a *.item manifest the importer needs.
type epicItem struct {
	AppName         string `json:"AppName"`
	DisplayName     string `json:"DisplayName"`
	InstallLocation string `json:"InstallLocation"`
}

func (epicSource) Scan(ctx context.Context, in ScanInput) (Result, error) {
	var res Result
	if err := ctx.Err(); err != nil {
		return res, err
	}
	if in.Path == "" {
		res.Warnings = append(res.Warnings, "Epic: pasta de manifestos não encontrada")
		return res, nil
	}
	entries, err := os.ReadDir(in.Path)
	if err != nil {
		res.Warnings = append(res.Warnings, "Epic: pasta de manifestos não encontrada")
		return res, nil
	}
	unnamed := 0
	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".item") {
			continue
		}
		path := filepath.Join(in.Path, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("Epic: %s ilegível", e.Name()))
			continue
		}
		var it epicItem
		if err := json.Unmarshal(data, &it); err != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("Epic: %s inválido", e.Name()))
			continue
		}
		if strings.TrimSpace(it.DisplayName) == "" || it.AppName == "" {
			unnamed++
			continue
		}
		c := Candidate{
			Source:       SourceEpic,
			Title:        strings.TrimSpace(it.DisplayName),
			Platform:     PlatformPC,
			LaunchType:   LaunchURI,
			LaunchTarget: "com.epicgames.launcher://apps/" + it.AppName + "?action=launch&silent=true",
			InstallDir:   it.InstallLocation,
		}
		res.Candidates = append(res.Candidates, c)
	}
	if unnamed > 0 {
		res.Warnings = append(res.Warnings, fmt.Sprintf("Epic: %d item(ns) sem nome ignorado(s)", unnamed))
	}
	return res, nil
}
