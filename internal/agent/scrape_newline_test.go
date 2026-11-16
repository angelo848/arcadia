package agent

import (
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

// T49 (SSC-006): provider.setKey recusa \n e \r em key ou secret de screenscraper*.

func TestScreenScraperKeyLineBreaks(t *testing.T) {
	t.Parallel()
	vals := map[string]string{"nl-comeco": "\nabc", "nl-meio": "a\nbc", "nl-fim": "abc\n", "cr-comeco": "\rabc", "cr-meio": "a\rbc", "cr-fim": "abc\r"}

	t.Run("SSC-006/provider.setKey screenscraper e screenscraperdev com \\n ou \\r em key ou secret: -32602, nada gravado, credencial anterior continua e gancho nao chamado", func(t *testing.T) {
		t.Parallel()
		for _, prov := range []string{"screenscraper", "screenscraperdev"} {
			for vname, v := range vals {
				for _, field := range []string{"key", "secret"} {
					f := newScrapeFixture(t, nil)
					var calls atomic.Int32
					f.svc.videoCredsChanged = func() { calls.Add(1) }
					if _, err := f.call(t, ipc.MethodProviderSetKey, fmt.Sprintf(`{"provider":%q,"key":"antigo","secret":"antiga"}`, prov)); err != nil {
						t.Fatalf("setKey inicial: %v", err)
					}
					calls.Store(0)
					key, secret := "novo", "nova"
					if field == "key" {
						key = v
					} else {
						secret = v
					}
					_, err := f.call(t, ipc.MethodProviderSetKey, fmt.Sprintf(`{"provider":%q,"key":%q,"secret":%q}`, prov, key, secret))
					if codeOf(err) != ipc.CodeInvalidParams {
						t.Errorf("%s/%s/%s: code = %d (%v), want %d", prov, vname, field, codeOf(err), err, ipc.CodeInvalidParams)
					}
					if id, sec, perr := f.keys.Pair(prov); perr != nil || id != "antigo" || sec != "antiga" {
						t.Errorf("%s/%s/%s: Pair = %q, %q, %v; want a credencial anterior", prov, vname, field, id, sec, perr)
					}
					if calls.Load() != 0 {
						t.Errorf("%s/%s/%s: gancho chamado %d vezes", prov, vname, field, calls.Load())
					}
				}
			}
		}
	})

	t.Run("SSC-006/secret \"senha\\n\" (so quebra no fim) e recusado, nao aparado", func(t *testing.T) {
		t.Parallel()
		f := newScrapeFixture(t, nil)
		_, err := f.call(t, ipc.MethodProviderSetKey, `{"provider":"screenscraper","key":"u","secret":"senha\n"}`)
		if codeOf(err) != ipc.CodeInvalidParams {
			t.Errorf("code = %d, want %d", codeOf(err), ipc.CodeInvalidParams)
		}
		if f.keys.Configured("screenscraper") {
			t.Error("algo foi gravado")
		}
	})

	t.Run("SSC-006/igdb e steamgriddb nao mudam (SSC-001)", func(t *testing.T) {
		t.Parallel()
		f := newScrapeFixture(t, nil)
		if _, err := f.call(t, ipc.MethodProviderSetKey, `{"provider":"igdb","key":"cid","secret":"sec"}`); err != nil {
			t.Errorf("igdb: %v", err)
		}
		if _, err := f.call(t, ipc.MethodProviderSetKey, `{"provider":"steamgriddb","key":"k"}`); err != nil {
			t.Errorf("steamgriddb: %v", err)
		}
	})
}
