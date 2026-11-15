package agent

// Tests of metadata.apply (phase 12, T21): validation, the fetch of every ref before any write, the
// merge by field, the image queue through queueImages, and the notifications. Provedores são fakes
// (metaProv, metaFetchProv de metadata_test.go); a loja é real, em t.TempDir().
// Alterado em 2026-11-15: arquivo novo (fase 12, T21).

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
	"github.com/arcadia-launcher/arcadia/internal/scrapers"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

// applyJSON is the params of metadata.apply as JSON.
func applyJSON(t *testing.T, p ipc.MetadataApplyParams) string {
	t.Helper()
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(raw)
}

// applyOK runs metadata.apply and fails the test when it is not a success.
func applyOK(t *testing.T, f metaFixture, p ipc.MetadataApplyParams) ipc.MetadataApplyResult {
	t.Helper()
	out, err := f.call(t, ipc.MethodMetadataApply, applyJSON(t, p))
	if err != nil {
		t.Fatalf("metadata.apply: %v", err)
	}
	res, ok := out.(ipc.MetadataApplyResult)
	if !ok {
		t.Fatalf("result type %T, want ipc.MetadataApplyResult", out)
	}
	return res
}

// pendingJobs are the jobs waiting in the queue, all games.
func pendingJobs(t *testing.T, st *store.Store) []store.MediaJob {
	t.Helper()
	jobs, err := st.PendingMediaJobs(t.Context())
	if err != nil {
		t.Fatalf("PendingMediaJobs: %v", err)
	}
	return jobs
}

// gameText reads the game's description, release date and title from the store.
func gameText(t *testing.T, st *store.Store, id int64) store.GameDetail {
	t.Helper()
	d, err := st.GameDetail(t.Context(), id)
	if err != nil {
		t.Fatalf("GameDetail: %v", err)
	}
	return d
}

// refs builds the refs of the given providers, with the same id for each.
func refsOf(id string, providers ...string) []ipc.MetadataRef {
	out := make([]ipc.MetadataRef, 0, len(providers))
	for _, p := range providers {
		out = append(out, ipc.MetadataRef{Provider: p, ProviderID: id})
	}
	return out
}

func TestMetadataApply(t *testing.T) {
	t.Parallel()

	t.Run("MET-031/refs inválidos dão -32602 sem nenhuma chamada", func(t *testing.T) {
		t.Parallel()
		bad := []struct {
			name string
			refs []ipc.MetadataRef
		}{
			{"MET-031/refs vazio", []ipc.MetadataRef{}},
			{"MET-031/quatro itens", []ipc.MetadataRef{
				{Provider: ipc.MetadataProviderIGDB, ProviderID: "7"},
				{Provider: ipc.MetadataProviderSteamStore, ProviderID: "1"},
				{Provider: ipc.MetadataProviderSteamGridDB, ProviderID: "2"},
				{Provider: ipc.MetadataProviderIGDB, ProviderID: "8"},
			}},
			{"MET-031/provedor repetido", []ipc.MetadataRef{
				{Provider: ipc.MetadataProviderIGDB, ProviderID: "7"},
				{Provider: ipc.MetadataProviderIGDB, ProviderID: "8"},
			}},
			{"MET-031/provedor desconhecido", []ipc.MetadataRef{{Provider: "epic", ProviderID: "7"}}},
			{"MET-031/providerId vazio", []ipc.MetadataRef{{Provider: ipc.MetadataProviderIGDB, ProviderID: ""}}},
			{"MET-031/providerId com letras", []ipc.MetadataRef{{Provider: ipc.MetadataProviderIGDB, ProviderID: "12a"}}},
			{"MET-031/providerId com 21 dígitos", []ipc.MetadataRef{{Provider: ipc.MetadataProviderIGDB, ProviderID: "123456789012345678901"}}},
		}
		for _, tc := range bad {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				igdb := &metaFetchProv{metaProv: metaProv{id: "igdb"}}
				steam := &metaFetchProv{metaProv: metaProv{id: "steamstore"}}
				sgdb := &metaProv{id: "steamgriddb"}
				f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": igdb, "steamstore": steam, "steamgriddb": sgdb})
				_, err := f.call(t, ipc.MethodMetadataApply, applyJSON(t, ipc.MetadataApplyParams{GameID: f.gameID, Refs: tc.refs}))
				if codeOf(err) != ipc.CodeInvalidParams {
					t.Errorf("code = %d (%v), want %d", codeOf(err), err, ipc.CodeInvalidParams)
				}
				if n := igdb.totalRequests() + steam.totalRequests() + sgdb.totalRequests(); n != 0 {
					t.Errorf("provider requests = %d, want 0", n)
				}
			})
		}
	})

	t.Run("MET-031/validação vem antes da leitura do jogo", func(t *testing.T) {
		t.Parallel()
		igdb := &metaFetchProv{metaProv: metaProv{id: "igdb"}}
		f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": igdb})
		_, err := f.call(t, ipc.MethodMetadataApply, applyJSON(t, ipc.MetadataApplyParams{
			GameID: 999, Refs: refsOf("7", ipc.MetadataProviderIGDB, ipc.MetadataProviderIGDB),
		}))
		if codeOf(err) != ipc.CodeInvalidParams {
			t.Errorf("code = %d (%v), want %d (not game not found)", codeOf(err), err, ipc.CodeInvalidParams)
		}
	})

	t.Run("MET-031/providerId com 20 dígitos é aceito", func(t *testing.T) {
		t.Parallel()
		igdb := &metaFetchProv{metaProv: metaProv{id: "igdb", meta: scrapers.Metadata{Description: "Texto"}}}
		f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": igdb})
		applyOK(t, f, ipc.MetadataApplyParams{GameID: f.gameID, Refs: refsOf("12345678901234567890", ipc.MetadataProviderIGDB)})
		if ids := igdb.askedIDs(); len(ids) != 1 || ids[0] != "12345678901234567890" {
			t.Errorf("IGDB asked for %v, want the 20-digit id", ids)
		}
	})

	t.Run("MET-031/provedor com chave ausente: -32010 com data.provider", func(t *testing.T) {
		t.Parallel()
		f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": &metaFetchProv{metaProv: metaProv{id: "igdb"}}}, "igdb")
		_, err := f.call(t, ipc.MethodMetadataApply, applyJSON(t, ipc.MetadataApplyParams{GameID: f.gameID, Refs: refsOf("7", ipc.MetadataProviderIGDB)}))
		if codeOf(err) != ipc.CodeProviderNoKey {
			t.Fatalf("code = %d (%v), want %d", codeOf(err), err, ipc.CodeProviderNoKey)
		}
		if d := errData(t, err); d["provider"] != "igdb" {
			t.Errorf("data = %v, want provider igdb", d)
		}
	})

	t.Run("MET-031/falha temporária de um provedor: -32011", func(t *testing.T) {
		t.Parallel()
		igdb := &metaFetchProv{metaProv: metaProv{id: "igdb", fetchErr: scrapers.ErrUnavailable}}
		f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": igdb})
		_, err := f.call(t, ipc.MethodMetadataApply, applyJSON(t, ipc.MetadataApplyParams{GameID: f.gameID, Refs: refsOf("7", ipc.MetadataProviderIGDB)}))
		if codeOf(err) != ipc.CodeProviderUnavailable {
			t.Fatalf("code = %d (%v), want %d", codeOf(err), err, ipc.CodeProviderUnavailable)
		}
		if d := errData(t, err); d["provider"] != "igdb" {
			t.Errorf("data = %v, want provider igdb", d)
		}
	})

	t.Run("MET-030/campos vazios preenchidos por IGDB e depois Steam Store", func(t *testing.T) {
		t.Parallel()
		igdb := &metaFetchProv{metaProv: metaProv{id: "igdb", meta: scrapers.Metadata{
			Description: "Texto do IGDB", Genres: []string{"Platform"}, Year: 2018, Modes: []string{"Single player"},
		}}}
		steam := &metaFetchProv{metaProv: metaProv{id: "steamstore", meta: scrapers.Metadata{
			Description: "Texto da Steam", Genres: []string{"Action"}, Developers: []string{"Maddy Makes Games"},
			Publishers: []string{"Matt Makes Games"}, Modes: []string{"Multiplayer"},
		}}}
		f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": igdb, "steamstore": steam})
		applyOK(t, f, ipc.MetadataApplyParams{GameID: f.gameID, Refs: refsOf("7", ipc.MetadataProviderIGDB, ipc.MetadataProviderSteamStore)})
		d := gameText(t, f.st, f.gameID)
		if d.Description != "Texto do IGDB" {
			t.Errorf("description = %q, want the IGDB text (first in priority)", d.Description)
		}
		if strings.Join(d.Genres, ",") != "Platform" {
			t.Errorf("genres = %v, want [Platform] from IGDB", d.Genres)
		}
		if strings.Join(d.Developers, ",") != "Maddy Makes Games" || strings.Join(d.Publishers, ",") != "Matt Makes Games" {
			t.Errorf("developers %v publishers %v: the empty IGDB fields must come from Steam Store", d.Developers, d.Publishers)
		}
		if strings.Join(d.PlayModes, ",") != string(domain.PlayModeSingle) {
			t.Errorf("play modes = %v, want only the IGDB mode (single)", d.PlayModes)
		}
		if !strings.HasPrefix(d.ReleaseDate, "2018") {
			t.Errorf("release date = %q, want the IGDB year 2018", d.ReleaseDate)
		}
	})

	t.Run("MET-030/campo preenchido pelo usuário é preservado", func(t *testing.T) {
		t.Parallel()
		igdb := &metaFetchProv{metaProv: metaProv{id: "igdb", meta: scrapers.Metadata{Description: "Texto do IGDB", Genres: []string{"Platform"}}}}
		f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": igdb})
		mine := "Meu texto"
		if _, err := f.st.UpdateGame(t.Context(), f.gameID, store.GamePatch{Description: &mine}, f.clock.Now()); err != nil {
			t.Fatalf("UpdateGame: %v", err)
		}
		applyOK(t, f, ipc.MetadataApplyParams{GameID: f.gameID, Refs: refsOf("7", ipc.MetadataProviderIGDB), Replace: false})
		d := gameText(t, f.st, f.gameID)
		if d.Description != mine {
			t.Errorf("description = %q, want the user's %q", d.Description, mine)
		}
		if strings.Join(d.Genres, ",") != "Platform" {
			t.Errorf("genres = %v, want the empty field filled", d.Genres)
		}
	})

	t.Run("MET-030/replace sobrescreve o campo do usuário", func(t *testing.T) {
		t.Parallel()
		igdb := &metaFetchProv{metaProv: metaProv{id: "igdb", meta: scrapers.Metadata{Description: "Texto do IGDB"}}}
		f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": igdb})
		mine := "Meu texto"
		if _, err := f.st.UpdateGame(t.Context(), f.gameID, store.GamePatch{Description: &mine}, f.clock.Now()); err != nil {
			t.Fatalf("UpdateGame: %v", err)
		}
		applyOK(t, f, ipc.MetadataApplyParams{GameID: f.gameID, Refs: refsOf("7", ipc.MetadataProviderIGDB), Replace: true})
		if d := gameText(t, f.st, f.gameID); d.Description != "Texto do IGDB" {
			t.Errorf("description = %q, want the IGDB text with replace", d.Description)
		}
	})

	t.Run("MET-030/o título do jogo nunca muda", func(t *testing.T) {
		t.Parallel()
		igdb := &metaFetchProv{metaProv: metaProv{id: "igdb", meta: scrapers.Metadata{Title: "Outro Nome", Description: "Texto"}}}
		f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": igdb})
		applyOK(t, f, ipc.MetadataApplyParams{GameID: f.gameID, Refs: refsOf("7", ipc.MetadataProviderIGDB), Replace: true})
		if d := gameText(t, f.st, f.gameID); d.Title != "Celeste" {
			t.Errorf("title = %q, want Celeste", d.Title)
		}
	})

	t.Run("MET-030/IGDB 1 requisição, Steam Store 1, SteamGridDB 2 (contagem nos fakes)", func(t *testing.T) {
		t.Parallel()
		igdb := &metaFetchProv{metaProv: metaProv{id: "igdb", meta: scrapers.Metadata{Description: "Texto"}}}
		steam := &metaFetchProv{metaProv: metaProv{id: "steamstore", meta: scrapers.Metadata{Developers: []string{"Dev"}}}}
		sgdb := &metaProv{id: "steamgriddb", media: []scrapers.RemoteMedia{{Kind: scrapers.KindCover, URL: "https://cdn.example.test/g.png"}},
			cost: map[string]int{"Details": 0, "Media": 2}}
		f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": igdb, "steamstore": steam, "steamgriddb": sgdb})
		applyOK(t, f, ipc.MetadataApplyParams{GameID: f.gameID, Refs: refsOf("7", ipc.MetadataProviderIGDB, ipc.MetadataProviderSteamStore, ipc.MetadataProviderSteamGridDB)})
		if n := igdb.totalRequests(); n != 1 {
			t.Errorf("IGDB requests = %d, want 1", n)
		}
		if n := steam.totalRequests(); n != 1 {
			t.Errorf("Steam Store requests = %d, want 1", n)
		}
		if n := sgdb.totalRequests(); n != 2 {
			t.Errorf("SteamGridDB requests = %d, want 2 (grids and heroes)", n)
		}
	})

	t.Run("MET-030/imagens enfileiradas pela prioridade de MET-020 (Windows: SteamGridDB primeiro)", func(t *testing.T) {
		t.Parallel()
		igdb := &metaFetchProv{metaProv: metaProv{id: "igdb", media: []scrapers.RemoteMedia{{Kind: scrapers.KindCover, URL: "https://cdn.example.test/igdb-cover.png"}}}}
		sgdb := &metaProv{id: "steamgriddb", media: []scrapers.RemoteMedia{{Kind: scrapers.KindCover, URL: "https://cdn.example.test/sgdb-cover.png"}}}
		f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": igdb, "steamgriddb": sgdb})
		// No cover is stored yet: the game has the one cover vacancy allowed (MET-021, capa = 1, alterado em 2026-11-15).
		res := applyOK(t, f, ipc.MetadataApplyParams{GameID: f.gameID, Refs: refsOf("7", ipc.MetadataProviderIGDB, ipc.MetadataProviderSteamGridDB)})
		if res.Queued != 1 {
			t.Errorf("queued = %d, want 1", res.Queued)
		}
		jobs := pendingJobs(t, f.st)
		if len(jobs) != 1 || jobs[0].Kind != scrapers.KindCover || jobs[0].Provider != "steamgriddb" || jobs[0].SourceURL != "https://cdn.example.test/sgdb-cover.png" {
			t.Errorf("jobs = %+v, want the SteamGridDB cover (Windows priority)", jobs)
		}
	})

	t.Run("MET-012/IGDB no apply avulso: uma requisição por id e nenhuma de mídia à parte", func(t *testing.T) {
		t.Parallel()
		igdb := &metaFetchProv{metaProv: metaProv{id: "igdb", meta: scrapers.Metadata{Description: "Texto"}}}
		f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": igdb})
		applyOK(t, f, ipc.MetadataApplyParams{GameID: f.gameID, Refs: refsOf("7", ipc.MetadataProviderIGDB)})
		if n := igdb.count("Fetch"); n != 1 {
			t.Errorf("Fetch calls = %d, want 1", n)
		}
		if n := igdb.count("Details") + igdb.count("Media"); n != 0 {
			t.Errorf("Details and Media calls = %d, want 0 (one request per id)", n)
		}
		if ids := igdb.askedIDs(); len(ids) != 1 || ids[0] != "7" {
			t.Errorf("IGDB asked for %v, want [7]", ids)
		}
	})

	t.Run("MET-013/ref do SteamGridDB devolve só cover e background", func(t *testing.T) {
		t.Parallel()
		sgdb := &metaProv{id: "steamgriddb", media: []scrapers.RemoteMedia{
			{Kind: scrapers.KindCover, URL: "https://cdn.example.test/g-cover.png"},
			{Kind: scrapers.KindBackground, URL: "https://cdn.example.test/h-hero.png"},
		}, cost: map[string]int{"Details": 0, "Media": 2}}
		f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"steamgriddb": sgdb})
		applyOK(t, f, ipc.MetadataApplyParams{GameID: f.gameID, Refs: refsOf("2", ipc.MetadataProviderSteamGridDB)})
		if n := sgdb.totalRequests(); n != 2 {
			t.Errorf("SteamGridDB requests = %d, want 2", n)
		}
		kinds := map[string]bool{}
		for _, j := range pendingJobs(t, f.st) {
			kinds[j.Kind] = true
		}
		if !kinds[scrapers.KindCover] || !kinds[scrapers.KindBackground] || len(kinds) != 2 {
			t.Errorf("queued kinds = %v, want cover and background only", kinds)
		}
	})

	t.Run("MET-014/ref da Steam Store: uma requisição", func(t *testing.T) {
		t.Parallel()
		steam := &metaFetchProv{metaProv: metaProv{id: "steamstore", meta: scrapers.Metadata{Description: "Texto"}}}
		f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"steamstore": steam})
		applyOK(t, f, ipc.MetadataApplyParams{GameID: f.gameID, Refs: refsOf("504230", ipc.MetadataProviderSteamStore)})
		if n := steam.totalRequests(); n != 1 {
			t.Errorf("Steam Store requests = %d, want 1", n)
		}
		if n := steam.count("Details") + steam.count("Media"); n != 0 {
			t.Errorf("Details and Media calls = %d, want 0 (Fetch does both)", n)
		}
	})

	t.Run("MET-032/segundo provedor falha: jogo e fila intactos", func(t *testing.T) {
		t.Parallel()
		igdb := &metaFetchProv{metaProv: metaProv{id: "igdb", meta: scrapers.Metadata{Description: "Texto do IGDB"},
			media: []scrapers.RemoteMedia{{Kind: scrapers.KindCover, URL: "https://cdn.example.test/c.png"}}}}
		steam := &metaFetchProv{metaProv: metaProv{id: "steamstore", fetchErr: scrapers.ErrUnavailable}}
		f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": igdb, "steamstore": steam})
		_, err := f.call(t, ipc.MethodMetadataApply, applyJSON(t, ipc.MetadataApplyParams{
			GameID: f.gameID, Refs: refsOf("7", ipc.MetadataProviderIGDB, ipc.MetadataProviderSteamStore), Replace: true,
		}))
		if codeOf(err) != ipc.CodeProviderUnavailable {
			t.Fatalf("code = %d (%v), want %d", codeOf(err), err, ipc.CodeProviderUnavailable)
		}
		if d := errData(t, err); d["provider"] != "steamstore" {
			t.Errorf("data = %v, want provider steamstore", d)
		}
		if d := gameText(t, f.st, f.gameID); d.Description != "" {
			t.Errorf("description = %q, want the game unchanged", d.Description)
		}
		if jobs := pendingJobs(t, f.st); len(jobs) != 0 {
			t.Errorf("queued jobs = %d, want 0", len(jobs))
		}
	})

	t.Run("MET-032/gameId inexistente: -32001 sem chamada", func(t *testing.T) {
		t.Parallel()
		igdb := &metaFetchProv{metaProv: metaProv{id: "igdb"}}
		f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": igdb})
		_, err := f.call(t, ipc.MethodMetadataApply, applyJSON(t, ipc.MetadataApplyParams{GameID: 999, Refs: refsOf("7", ipc.MetadataProviderIGDB)}))
		if codeOf(err) != ipc.CodeGameNotFound {
			t.Errorf("code = %d (%v), want %d", codeOf(err), err, ipc.CodeGameNotFound)
		}
		if n := igdb.totalRequests(); n != 0 {
			t.Errorf("IGDB requests = %d, want 0", n)
		}
	})

	t.Run("MET-033/nenhuma notificação antes de um job gravar", func(t *testing.T) {
		t.Parallel()
		igdb := &metaFetchProv{metaProv: metaProv{id: "igdb", media: []scrapers.RemoteMedia{{Kind: scrapers.KindCover, URL: "https://cdn.example.test/c.png"}}}}
		f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": igdb})
		applyOK(t, f, ipc.MetadataApplyParams{GameID: f.gameID, Refs: refsOf("7", ipc.MetadataProviderIGDB)})
		if n := f.sink.count(); n != 0 {
			t.Errorf("media.changed notifications = %d, want 0 before any job writes", n)
		}
		if jobs := pendingJobs(t, f.st); len(jobs) != 1 {
			t.Errorf("queued jobs = %d, want 1 (the queue wakes, the notice comes from the job)", len(jobs))
		}
	})

	t.Run("MET-022/jogo com 1 vaga de screenshot enfileira 1 e skippedByLimit conta o resto", func(t *testing.T) {
		t.Parallel()
		igdb := &metaFetchProv{metaProv: metaProv{id: "igdb", media: imageOffer("igdb", scrapers.KindScreenshot, "shot", 3).Media}}
		f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": igdb})
		storeImages(t, f.st, f.gameID, "igdb", scrapers.KindScreenshot, "old", 9)
		res := applyOK(t, f, ipc.MetadataApplyParams{GameID: f.gameID, Refs: refsOf("7", ipc.MetadataProviderIGDB)})
		if res.Queued != 1 || res.SkippedByLimit != 2 {
			t.Errorf("queued %d skipped %d, want 1 and 2", res.Queued, res.SkippedByLimit)
		}
		if n := len(pendingJobs(t, f.st)); n != 1 {
			t.Errorf("queued jobs = %d, want 1", n)
		}
	})

	t.Run("MET-024/queued e skippedByLimit no resultado", func(t *testing.T) {
		t.Parallel()
		igdb := &metaFetchProv{metaProv: metaProv{id: "igdb", media: imageOffer("igdb", scrapers.KindCover, "cov", 3).Media}}
		f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": igdb})
		res := applyOK(t, f, ipc.MetadataApplyParams{GameID: f.gameID, Refs: refsOf("7", ipc.MetadataProviderIGDB)})
		// One cover fits (MediaKindLimit, capa = 1, alterado em 2026-11-15), the other two are left out.
		if res.Queued != 1 || res.SkippedByLimit != 2 {
			t.Errorf("queued %d skipped %d, want 1 and 2", res.Queued, res.SkippedByLimit)
		}
		if res.Detail.ID != f.gameID {
			t.Errorf("detail id = %d, want %d", res.Detail.ID, f.gameID)
		}
	})

	t.Run("MET-071/gameId menor que 1 é -32602 sem chamada", func(t *testing.T) {
		t.Parallel()
		for _, id := range []int64{0, -3} {
			igdb := &metaFetchProv{metaProv: metaProv{id: "igdb"}}
			f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": igdb})
			_, err := f.call(t, ipc.MethodMetadataApply, applyJSON(t, ipc.MetadataApplyParams{GameID: id, Refs: refsOf("7", ipc.MetadataProviderIGDB)}))
			if codeOf(err) != ipc.CodeInvalidParams {
				t.Errorf("gameId %d: code = %d (%v), want %d", id, codeOf(err), err, ipc.CodeInvalidParams)
			}
			if n := igdb.totalRequests(); n != 0 {
				t.Errorf("gameId %d: IGDB requests = %d, want 0", id, n)
			}
		}
	})
}
