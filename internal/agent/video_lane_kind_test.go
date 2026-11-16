package agent

import (
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/store"
)

// T50 (VID-009 alterado): kind == "video" e o unico criterio de raia, no despacho e no roteador.
// Assinatura assumida: isVideoJob(job store.MediaJob) bool.

func TestVideoLaneKindCriterion(t *testing.T) {
	t.Parallel()

	t.Run("VID-009/job kind video com provider diferente de screenscraper ocupa a vaga de video e vai ao resolvedor", func(t *testing.T) {
		t.Parallel()
		j := vJob(1, 1)
		j.Provider = "steamgriddb"
		j.SourceURL = "https://cdn.example.test/other.mp4"
		r := newLaneRig(t, []store.MediaJob{j})
		r.vid.hold = make(chan struct{})
		r.start()
		if id := waitStarted(t, r.vid.started); id != 1 {
			t.Fatalf("resolvedor recebeu o job %d, want 1", id)
		}
		if n := r.img.callsOf(j.SourceURL); n != 0 {
			t.Errorf("media.Store recebeu o job de video %d vezes", n)
		}
	})

	t.Run("VID-009/job kind cover com provider screenscraper (montado no store) ocupa vaga de imagem e vai ao media.Store", func(t *testing.T) {
		t.Parallel()
		const url = "https://cdn.example.test/cover-ss.png"
		j := gameJob(1, 1, url)
		j.Kind = "cover"
		j.Provider = store.VideoProvider
		r := newLaneRig(t, []store.MediaJob{j})
		r.start()
		waitState(t, r.st, 1, store.JobDone)
		if r.vid.totalCalls() != 0 {
			t.Errorf("resolvedor de video recebeu %d jobs", r.vid.totalCalls())
		}
		if r.img.callsOf(url) == 0 {
			t.Error("media.Store nao recebeu o job de imagem")
		}
	})

	t.Run("VID-009/com a raia de video pausada, imagem de qualquer provider comeca na hora e nenhum job kind video e despachado", func(t *testing.T) {
		t.Parallel()
		v1 := vJob(1, 1)
		v2 := vJob(2, 2)
		v2.Provider = "steamgriddb"
		imgSS := gameJob(3, 3, "https://cdn.example.test/ss.png")
		imgSS.Provider = store.VideoProvider
		imgOther := gameJob(4, 4, "https://cdn.example.test/other.png")
		imgOther.Provider = "steamgriddb"
		r := newLaneRig(t, []store.MediaJob{v1, v2, imgSS, imgOther})
		r.ready.Store(false) // no-credentials
		r.start()
		r.waitLane("no-credentials")
		waitState(t, r.st, 3, store.JobDone)
		waitState(t, r.st, 4, store.JobDone)
		if r.vid.totalCalls() != 0 {
			t.Errorf("job kind video despachado com a raia pausada (%d chamadas)", r.vid.totalCalls())
		}
		if r.img.callsOf(v1.SourceURL) != 0 || r.img.callsOf(v2.SourceURL) != 0 {
			t.Error("job kind video foi parar no media.Store")
		}
	})

	t.Run("VID-009/despacho e roteador concordam para cada combinacao de kind e provider (tabela)", func(t *testing.T) {
		t.Parallel()
		for _, kind := range []string{"cover", "logo", "background", "screenshot", "video", ""} {
			for _, prov := range []string{store.VideoProvider, "steamgriddb", "launchbox", "igdb", ""} {
				job := store.MediaJob{Kind: kind, Provider: prov}
				want := kind == "video"
				if got := isVideoJob(job); got != want {
					t.Errorf("isVideoJob(kind=%q, provider=%q) = %v, want %v", kind, prov, got, want)
				}
				img, vid := &recFetcher{name: "img"}, &recFetcher{name: "vid"}
				got, _, err := fetchRouter{images: img, video: vid}.Fetch(t.Context(), job)
				if err != nil {
					t.Fatalf("Fetch: %v", err)
				}
				if (got.Hash == "vid") != want {
					t.Errorf("roteador: kind=%q provider=%q foi a %q", kind, prov, got.Hash)
				}
			}
		}
	})
}
