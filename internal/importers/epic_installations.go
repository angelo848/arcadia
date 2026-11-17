package importers

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// installationsFile is the journal of the new Epic launcher: one JSON record per line, the last one of
// an installation being its current state.
const installationsFile = "installations.jsonl"

// epicInstallation is the part of an installations.jsonl record the importer needs.
type epicInstallation struct {
	InstallationID string `json:"installationId"`
	Complete       bool   `json:"complete"`
	Dir            string `json:"dir"`
	Namespace      string `json:"namespace"`
	ItemID         string `json:"itemId"`
	ArtifactID     string `json:"artifactId"`
}

// epicCatalog is the part of cache/prod/catalog-cache.json that maps an item to its title.
type epicCatalog struct {
	Items map[string]struct {
		Item struct {
			Title string `json:"title"`
		} `json:"item"`
	} `json:"items"`
}

// scanEpicInstallations reads the games installed through the new Epic launcher. An installation counts
// when its last record is complete and its folder still exists.
func scanEpicInstallations(ctx context.Context, dir string) (Result, error) {
	var res Result
	f, err := os.Open(filepath.Join(dir, installationsFile))
	if err != nil {
		res.Warnings = append(res.Warnings, "Epic: lista de instalações ilegível")
		return res, nil
	}
	defer f.Close()

	last := map[string]epicInstallation{}
	var order []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		var rec epicInstallation
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil || rec.InstallationID == "" {
			continue // the header line and anything unknown
		}
		if _, seen := last[rec.InstallationID]; !seen {
			order = append(order, rec.InstallationID)
		}
		last[rec.InstallationID] = rec
	}
	if err := sc.Err(); err != nil {
		res.Warnings = append(res.Warnings, "Epic: lista de instalações ilegível")
	}

	titles := epicTitles(dir)
	for _, id := range order {
		rec := last[id]
		if !rec.Complete || rec.Namespace == "" || rec.ItemID == "" || rec.ArtifactID == "" {
			continue
		}
		if _, err := os.Stat(rec.Dir); err != nil {
			continue
		}
		title := strings.TrimSpace(titles[rec.ItemID])
		if title == "" {
			title = filepath.Base(rec.Dir)
		}
		res.Candidates = append(res.Candidates, Candidate{
			Source:     SourceEpic,
			Title:      title,
			Platform:   PlatformWindows,
			LaunchType: LaunchURI,
			LaunchTarget: fmt.Sprintf("com.epicgames.launcher://apps/%s:%s:%s?action=launch&silent=true",
				rec.Namespace, rec.ItemID, rec.ArtifactID),
			InstallDir: rec.Dir,
		})
	}
	return res, nil
}

// epicTitles maps an item id to its title using the launcher's catalog cache; a missing or broken cache
// gives an empty map (the folder name is the title then).
func epicTitles(dir string) map[string]string {
	out := map[string]string{}
	data, err := os.ReadFile(filepath.Join(dir, "cache", "prod", "catalog-cache.json"))
	if err != nil {
		return out
	}
	var cat epicCatalog
	if err := json.Unmarshal(data, &cat); err != nil {
		return out
	}
	for id, it := range cat.Items {
		out[id] = it.Item.Title
	}
	return out
}
