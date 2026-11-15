package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
	"github.com/arcadia-launcher/arcadia/internal/scrapers"
)

// Casos da T22 (phase-13-modo-console): chaves do ScreenScraper e provider.*; SSC-005 (fora dos metadados).
//
// Assinaturas assumidas:
//   - scrapeService.videoCredsChanged func() (nil = nenhum), chamado depois de setKey/deleteKey bem-sucedidos
//     de screenscraper e screenscraperdev, nunca de igdb/steamgriddb nem apos falha;
//   - keyProviderNames = igdb, steamgriddb, screenscraper, screenscraperdev; providerNames nao muda;
//   - setKey de screenscraper*: key = usuario/devid, secret = senha/devpassword (1..512 runas apos trim).
// Lacuna: o lote (metadata.batchStart) nao e exercitado aqui porque o formato dos parametros nao esta no tasks.md;
// a recusa de screenscraper nos refs e coberta em metadata.apply.

func TestScreenScraperKeys(t *testing.T) {
	t.Parallel()

	const (
		ssRawUser   = "meu-usuario"
		ssRawSecret = "SENHA-SECRETA-77"
	)

	t.Run("SSC-001/provider.setKey screenscraper (usuario e senha) e screenscraperdev (devid e devpassword) gravam em arcadia/screenscraper e arcadia/screenscraperdev com valor id\\nsenha", func(t *testing.T) {
		t.Parallel()
		f := newScrapeFixture(t, nil)
		for _, prov := range []string{"screenscraper", "screenscraperdev"} {
			out, err := f.call(t, ipc.MethodProviderSetKey, fmt.Sprintf(`{"provider":%q,"key":%q,"secret":%q}`, prov, ssRawUser, ssRawSecret))
			if err != nil {
				t.Fatalf("%s setKey: %v", prov, err)
			}
			res, ok := out.(ipc.ProviderKeyStatusResult)
			if !ok || res.Status.Provider != prov || !res.Status.Configured {
				t.Errorf("%s resultado = %#v", prov, out)
			}
			v, err := f.keys.Value(prov) // reads arcadia/<prov>
			if err != nil || v != ssRawUser+"\n"+ssRawSecret {
				t.Errorf("%s valor gravado = %q, %v", prov, v, err)
			}
			id, secret, err := f.keys.Pair(prov)
			if err != nil || id != ssRawUser || secret != ssRawSecret {
				t.Errorf("%s Pair = %q, %q, %v", prov, id, secret, err)
			}
		}
	})

	t.Run("SSC-001/sem secret, secret so de espacos ou com 513 runas: -32602 e nada gravado", func(t *testing.T) {
		t.Parallel()
		long := strings.Repeat("é", 513)
		for _, prov := range []string{"screenscraper", "screenscraperdev"} {
			for name, params := range map[string]string{
				"sem secret":       fmt.Sprintf(`{"provider":%q,"key":"u"}`, prov),
				"secret em branco": fmt.Sprintf(`{"provider":%q,"key":"u","secret":"   "}`, prov),
				"513 runas":        fmt.Sprintf(`{"provider":%q,"key":"u","secret":%q}`, prov, long),
				"key em branco":    fmt.Sprintf(`{"provider":%q,"key":" ","secret":"s"}`, prov),
			} {
				f := newScrapeFixture(t, nil)
				_, err := f.call(t, ipc.MethodProviderSetKey, params)
				if codeOf(err) != ipc.CodeInvalidParams {
					t.Errorf("%s/%s: code = %d (%v), want %d", prov, name, codeOf(err), err, ipc.CodeInvalidParams)
				}
				if f.keys.Configured(prov) {
					t.Errorf("%s/%s: algo foi gravado", prov, name)
				}
			}
		}
	})

	t.Run("SSC-001/secret de 512 runas e aceito", func(t *testing.T) {
		t.Parallel()
		f := newScrapeFixture(t, nil)
		_, err := f.call(t, ipc.MethodProviderSetKey, fmt.Sprintf(`{"provider":"screenscraper","key":"u","secret":%q}`, strings.Repeat("é", 512)))
		if err != nil {
			t.Errorf("512 runas: %v", err)
		}
	})

	t.Run("SSC-001/keyStatus e deleteKey aceitam os dois e nunca devolvem o valor", func(t *testing.T) {
		t.Parallel()
		f := newScrapeFixture(t, nil)
		for _, prov := range []string{"screenscraper", "screenscraperdev"} {
			st, err := f.call(t, ipc.MethodProviderKeyStatus, fmt.Sprintf(`{"provider":%q}`, prov))
			if err != nil || !strings.Contains(scrapeJSON(t, st), `"configured":false`) {
				t.Errorf("%s keyStatus antes = %v, %v", prov, st, err)
			}
			if _, err := f.call(t, ipc.MethodProviderSetKey, fmt.Sprintf(`{"provider":%q,"key":%q,"secret":%q}`, prov, ssRawUser, ssRawSecret)); err != nil {
				t.Fatalf("%s setKey: %v", prov, err)
			}
			st, err = f.call(t, ipc.MethodProviderKeyStatus, fmt.Sprintf(`{"provider":%q}`, prov))
			raw := scrapeJSON(t, st)
			if err != nil || !strings.Contains(raw, `"configured":true`) || strings.Contains(raw, ssRawUser) || strings.Contains(raw, ssRawSecret) {
				t.Errorf("%s keyStatus depois = %s, %v", prov, raw, err)
			}
			del, err := f.call(t, ipc.MethodProviderDeleteKey, fmt.Sprintf(`{"provider":%q}`, prov))
			raw = scrapeJSON(t, del)
			if err != nil || !strings.Contains(raw, `"configured":false`) || strings.Contains(raw, ssRawSecret) {
				t.Errorf("%s deleteKey = %s, %v", prov, raw, err)
			}
			if f.keys.Configured(prov) {
				t.Errorf("%s ainda configurado apos deleteKey", prov)
			}
		}
	})

	t.Run("SSC-001/o segredo nunca aparece em log (nem no de erro)", func(t *testing.T) {
		t.Parallel()
		f := newScrapeFixture(t, nil)
		var buf bytes.Buffer
		f.svc.log = slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
		for _, prov := range []string{"screenscraper", "screenscraperdev"} {
			_, _ = f.call(t, ipc.MethodProviderSetKey, fmt.Sprintf(`{"provider":%q,"key":%q,"secret":%q}`, prov, ssRawUser, ssRawSecret))
			_, _ = f.call(t, ipc.MethodProviderSetKey, fmt.Sprintf(`{"provider":%q,"key":%q,"secret":"   "}`, prov, ssRawUser))
			_, _ = f.call(t, ipc.MethodProviderKeyStatus, fmt.Sprintf(`{"provider":%q}`, prov))
			_, _ = f.call(t, ipc.MethodProviderDeleteKey, fmt.Sprintf(`{"provider":%q}`, prov))
		}
		if strings.Contains(buf.String(), ssRawSecret) {
			t.Errorf("log contem o segredo: %s", buf.String())
		}
	})

	t.Run("SSC-001/igdb e steamgriddb continuam como antes", func(t *testing.T) {
		t.Parallel()
		f := newScrapeFixture(t, nil)
		// igdb still needs its secret; steamgriddb still takes only the key.
		if _, err := f.call(t, ipc.MethodProviderSetKey, `{"provider":"igdb","key":"cid"}`); codeOf(err) != ipc.CodeInvalidParams {
			t.Errorf("igdb sem secret: code = %d, want %d", codeOf(err), ipc.CodeInvalidParams)
		}
		if _, err := f.call(t, ipc.MethodProviderSetKey, `{"provider":"igdb","key":"cid","secret":"sec"}`); err != nil {
			t.Errorf("igdb: %v", err)
		}
		if _, err := f.call(t, ipc.MethodProviderSetKey, `{"provider":"steamgriddb","key":"k"}`); err != nil {
			t.Errorf("steamgriddb: %v", err)
		}
		if _, err := f.call(t, ipc.MethodProviderKeyStatus, `{"provider":"steamstore"}`); codeOf(err) != ipc.CodeInvalidParams {
			t.Errorf("steamstore keyStatus: code = %d, want %d", codeOf(err), ipc.CodeInvalidParams)
		}
		if _, err := f.call(t, ipc.MethodProviderKeyStatus, `{"provider":"outro"}`); codeOf(err) != ipc.CodeInvalidParams {
			t.Errorf("provedor desconhecido: code = %d, want %d", codeOf(err), ipc.CodeInvalidParams)
		}
	})

	t.Run("VID-017/setKey e deleteKey de screenscraper e screenscraperdev chamam o gancho de credenciais; os de igdb nao", func(t *testing.T) {
		t.Parallel()
		f := newScrapeFixture(t, nil)
		var calls atomic.Int32
		f.svc.videoCredsChanged = func() { calls.Add(1) }

		for i, prov := range []string{"screenscraper", "screenscraperdev"} {
			before := calls.Load()
			if _, err := f.call(t, ipc.MethodProviderSetKey, fmt.Sprintf(`{"provider":%q,"key":"u","secret":"s"}`, prov)); err != nil {
				t.Fatalf("%s setKey: %v", prov, err)
			}
			if calls.Load() != before+1 {
				t.Errorf("%s (#%d) setKey: gancho chamado %d vezes, want 1", prov, i, calls.Load()-before)
			}
			before = calls.Load()
			if _, err := f.call(t, ipc.MethodProviderDeleteKey, fmt.Sprintf(`{"provider":%q}`, prov)); err != nil {
				t.Fatalf("%s deleteKey: %v", prov, err)
			}
			if calls.Load() != before+1 {
				t.Errorf("%s deleteKey: gancho chamado %d vezes, want 1", prov, calls.Load()-before)
			}
		}

		before := calls.Load()
		_, _ = f.call(t, ipc.MethodProviderSetKey, `{"provider":"igdb","key":"c","secret":"s"}`)
		_, _ = f.call(t, ipc.MethodProviderDeleteKey, `{"provider":"igdb"}`)
		_, _ = f.call(t, ipc.MethodProviderSetKey, `{"provider":"steamgriddb","key":"k"}`)
		_, _ = f.call(t, ipc.MethodProviderDeleteKey, `{"provider":"steamgriddb"}`)
		if calls.Load() != before {
			t.Errorf("gancho chamado %d vezes por igdb/steamgriddb, want 0", calls.Load()-before)
		}
	})

	t.Run("VID-017/setKey recusado nao chama o gancho; gancho nil e aceito", func(t *testing.T) {
		t.Parallel()
		f := newScrapeFixture(t, nil)
		var calls atomic.Int32
		f.svc.videoCredsChanged = func() { calls.Add(1) }
		_, _ = f.call(t, ipc.MethodProviderSetKey, `{"provider":"screenscraper","key":"u","secret":" "}`)
		if calls.Load() != 0 {
			t.Errorf("gancho chamado %d vezes apos setKey recusado", calls.Load())
		}
		f.svc.videoCredsChanged = nil
		if _, err := f.call(t, ipc.MethodProviderSetKey, `{"provider":"screenscraper","key":"u","secret":"s"}`); err != nil {
			t.Errorf("gancho nil: %v", err)
		}
	})
}

func TestScreenScraperOutOfMetadata(t *testing.T) {
	t.Parallel()

	t.Run("SSC-005/validProvider(\"screenscraper\") e falso: scrape.search, scrape.apply e media.queue recusam com -32602", func(t *testing.T) {
		t.Parallel()
		for _, name := range []string{"screenscraper", "screenscraperdev"} {
			if validProvider(name) {
				t.Errorf("validProvider(%q) = true", name)
			}
			if !validKeyProvider(name) {
				t.Errorf("validKeyProvider(%q) = false", name)
			}
		}
		spy := &metaProv{id: "screenscraper"}
		f := newScrapeFixtureWith(t, map[string]scrapers.MetadataProvider{"screenscraper": spy})
		for method, params := range map[string]string{
			ipc.MethodScrapeSearch: `{"gameId":1,"provider":"screenscraper"}`,
			ipc.MethodScrapeApply:  `{"gameId":1,"provider":"screenscraper","providerId":"1"}`,
			ipc.MethodMediaQueue:   `{"gameId":1,"provider":"screenscraper","providerId":"1","kinds":["cover"]}`,
		} {
			_, err := f.call(t, method, params)
			if codeOf(err) != ipc.CodeInvalidParams {
				t.Errorf("%s: code = %d (%v), want %d", method, codeOf(err), err, ipc.CodeInvalidParams)
			}
		}
		if spy.totalRequests() != 0 {
			t.Errorf("fake do ScreenScraper recebeu %d chamadas, want 0", spy.totalRequests())
		}
	})

	t.Run("SSC-005/a fabrica de provedores de metadados nao oferece screenscraper", func(t *testing.T) {
		t.Parallel()
		keys := scrapers.NewKeys(&memKeyStore{values: map[string]string{}})
		if err := keys.SetPair("screenscraper", "u", "s"); err != nil {
			t.Fatalf("SetPair: %v", err)
		}
		if err := keys.SetPair("screenscraperdev", "d", "p"); err != nil {
			t.Fatalf("SetPair: %v", err)
		}
		f := newProviderFactory(keys, nil, domain.NewFakeClock(provNow))
		for _, name := range []string{"screenscraper", "screenscraperdev"} {
			if p, err := f(name); err == nil || p != nil {
				t.Errorf("fabrica(%s) = %v, %v; want errUnknownProvider", name, p, err)
			}
		}
	})

	t.Run("SSC-005/metadata.search, metadata.apply e o lote nunca chamam o fake do ScreenScraper (contador zero)", func(t *testing.T) {
		t.Parallel()
		ss := &metaProv{id: "screenscraper", cands: []scrapers.Candidate{metaCand("screenscraper", "1", "Celeste", 2018, "Windows")}}
		igdb := &metaProv{id: "igdb", cands: []scrapers.Candidate{metaCand("igdb", "7", "Celeste", 2018, "Windows")}}
		f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"screenscraper": ss, "igdb": igdb})

		out := searchOK(t, f, ipc.MetadataSearchParams{Title: "Celeste"})
		for _, c := range out.Candidates {
			for _, r := range c.Refs {
				if r.Provider == "screenscraper" {
					t.Errorf("candidato com ref do screenscraper: %+v", c)
				}
			}
		}
		raw, _ := json.Marshal(ipc.MetadataApplyParams{GameID: f.gameID, Refs: []ipc.MetadataRef{{Provider: "screenscraper", ProviderID: "1"}}})
		_, err := f.call(t, ipc.MethodMetadataApply, string(raw))
		if codeOf(err) != ipc.CodeInvalidParams {
			t.Errorf("metadata.apply com ref screenscraper: code = %d (%v), want %d", codeOf(err), err, ipc.CodeInvalidParams)
		}
		if ss.totalRequests() != 0 {
			t.Errorf("fake do ScreenScraper recebeu %d chamadas, want 0", ss.totalRequests())
		}
	})
}
