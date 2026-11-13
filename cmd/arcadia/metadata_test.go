package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

// TestMetadataBindings checks the metadata and media bindings: the params they send, the views they
// return and the messages of the contract's errors (phase 7).
func TestMetadataBindings(t *testing.T) {
	t.Parallel()

	t.Run("MID-001/setKey manda a chave ao agente e a resposta não traz a chave", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{results: map[string]any{ipc.MethodProviderSetKey: ipc.ProviderKeyStatusResult{
			Status: ipc.ProviderKeyStatus{Provider: "igdb", Configured: true},
		}}}
		a := &App{client: p, log: discardLog()}

		got, err := a.ProviderSetKey("igdb", "cid-secreto", "sec-secreto")
		if err != nil || got != (ProviderKeyView{Provider: "igdb", Configured: true}) {
			t.Fatalf("ProviderSetKey = %+v, %v", got, err)
		}
		raw, _ := json.Marshal(got)
		if strings.Contains(string(raw), "secreto") {
			t.Errorf("view %s repeats a key", raw)
		}
		c := singleCall(t, p, ipc.MethodProviderSetKey, libraryTimeout)
		var sent ipc.ProviderSetKeyParams
		if err := json.Unmarshal(c.params, &sent); err != nil || sent.Provider != "igdb" || sent.Secret != "sec-secreto" {
			t.Errorf("params = %s, %v", c.params, err)
		}
	})

	t.Run("MID-002/search devolve os candidatos com a pontuação", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{results: map[string]any{ipc.MethodScrapeSearch: ipc.ScrapeSearchResult{
			Candidates: []ipc.ScrapeCandidate{{ProviderID: "7", Title: "Celeste", Platform: "PC", Year: 2018, Score: 1, Accepted: true}},
		}}}
		a := &App{client: p, log: discardLog()}

		got, err := a.ScrapeSearch(3, "igdb")
		if err != nil || len(got.Candidates) != 1 || got.Candidates[0].ProviderID != "7" || !got.Candidates[0].Accepted {
			t.Errorf("ScrapeSearch = %+v, %v", got, err)
		}
	})

	t.Run("MID-005/cancelar um job que não existe mostra a mensagem do contrato", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{errs: map[string]error{ipc.MethodMediaCancel: ipc.NewError(ipc.CodeMediaJobNotFound, ipc.MsgMediaJobNotFound)}}
		a := &App{client: p, log: discardLog()}

		if err := a.MediaCancel(99); err == nil || err.Error() != msgMediaJobNotFound {
			t.Errorf("MediaCancel err = %v, want %q", err, msgMediaJobNotFound)
		}
	})

	t.Run("MID-001/chave ausente mostra a mensagem de cadastro", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{errs: map[string]error{ipc.MethodScrapeSearch: ipc.NewError(ipc.CodeProviderNoKey, ipc.MsgProviderNoKey)}}
		a := &App{client: p, log: discardLog()}

		if _, err := a.ScrapeSearch(3, "igdb"); err == nil || err.Error() != msgProviderNoKey {
			t.Errorf("ScrapeSearch err = %v, want %q", err, msgProviderNoKey)
		}
	})
}

// TestMediaListURLs checks that the images of a game reach the UI with the URLs the media
// handler serves (MID-006).
func TestMediaListURLs(t *testing.T) {
	t.Parallel()
	data := t.TempDir()
	abs := filepath.Join(data, "media", "7", "h.png")
	p := &gamePinger{results: map[string]any{ipc.MethodMediaList: ipc.MediaListResult{
		Media: []ipc.MediaItem{{ID: 1, Kind: "cover", Provider: "steamgriddb", Path: abs, ThumbPath: ""}},
	}}}
	a := &App{client: p, log: discardLog(), dataRoot: data}

	got, err := a.MediaList(7)
	if err != nil || len(got.Media) != 1 || got.Media[0].URL != "/media/7/h.png" || got.Media[0].ThumbURL != "" {
		t.Errorf("MediaList = %+v, %v; want url /media/7/h.png and no thumb url", got, err)
	}
}
