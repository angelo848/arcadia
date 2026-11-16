package scrapers_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/scrapers"
)

// Casos da T21 (phase-13-modo-console): cliente ScreenScraper. httptest + FakeClock, sem rede real.
//
// Premissas (ver o relatorio):
//   - SSConfig.BaseURL e a base da API com barra final tolerada; os caminhos sao "jeuRecherche.php" e "jeuInfos.php"
//     anexados a ela (o teste serve em <srv>/api2/ e casa por sufixo do caminho).
//   - ssuser numerico vem como string na API real; o decodificador aceita string ou numero.
//   - Limiter.Wait e chamado antes de cada requisicao (busca, informacoes e download) e SetInterval(SSInterval(MaxPerMinute))
//     depois da resposta que traz ssuser.
//   - Contexto ja cancelado devolve um erro que satisfaz errors.Is(err, context.Canceled).

var ssT0 = time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)

const (
	ssDevID  = "DEVID-MARK-1"
	ssDevPw  = "DEVPW-MARK-2"
	ssUser   = "SSID-MARK-3"
	ssUserPw = "SSPW-MARK-4"
)

var ssMarks = []string{ssDevID, ssDevPw, ssUser, ssUserPw}

func ssGoodCreds() (scrapers.SSCreds, error) {
	return scrapers.SSCreds{DevID: ssDevID, DevPassword: ssDevPw, User: ssUser, Password: ssUserPw}, nil
}

const ssSearchJSON = `{"header":{"success":"true"},"response":{"ssuser":{"maxthreads":"1","maxrequestspermin":"20","requeststoday":"10","maxrequestsperday":"20000","requestskotoday":"1","maxrequestskoperday":"2000"},"jeux":[` +
	`{"id":"1234","noms":[{"region":"us","text":"Super Mario World"},{"region":"jp","text":"Super Mario Warudo"}],"systeme":{"id":"4","text":"SNES"},"dates":[{"region":"us","text":"1991-08-23"},{"region":"jp","text":"1990-11-21"}],` +
	`"medias":[{"type":"video","region":"us","format":"mp4","url":"http://x.test/old.mp4"},{"type":"video-normalized","region":"eu","format":"mp4","url":"http://x.test/eu.mp4"},{"type":"box-2D","region":"us","format":"png","url":"http://x.test/box.png"}]}]}}`

const ssInfoJSON = `{"header":{"success":"true"},"response":{"ssuser":{"maxthreads":"2","maxrequestspermin":"20","requeststoday":"11","maxrequestsperday":"20000"},"jeu":` +
	`{"id":"1234","noms":[{"region":"us","text":"Super Mario World"}],"systeme":{"id":4,"text":"SNES"},"dates":[{"region":"us","text":"1991"}],` +
	`"medias":[{"type":"video-normalized","region":"us","format":"mp4","url":"http://x.test/us.mp4"}]}}}`

// ssRig is a ScreenScraper over an httptest server with a real Limiter on a fake clock.
type ssRig struct {
	srv   *httptest.Server
	clock *domain.FakeClock
	lim   *scrapers.Limiter
	log   *syncBuf
	ss    *scrapers.ScreenScraper

	mu      sync.Mutex
	queries []url.Values
	paths   []string
	times   []time.Time
	hits    atomic.Int32
}

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// newSSRig serves handler after recording the request. creds nil = ssGoodCreds.
func newSSRig(t *testing.T, handler http.HandlerFunc, creds func() (scrapers.SSCreds, error)) *ssRig {
	t.Helper()
	r := &ssRig{clock: domain.NewFakeClock(ssT0), log: &syncBuf{}}
	r.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.hits.Add(1)
		r.mu.Lock()
		r.queries = append(r.queries, req.URL.Query())
		r.paths = append(r.paths, req.URL.Path)
		r.times = append(r.times, r.clock.Now())
		r.mu.Unlock()
		handler(w, req)
	}))
	t.Cleanup(r.srv.Close)
	r.lim = scrapers.NewLimiter(r.clock, scrapers.IntervalScreenScraper)
	if creds == nil {
		creds = ssGoodCreds
	}
	r.ss = scrapers.NewScreenScraper(scrapers.SSConfig{
		BaseURL: r.srv.URL + "/api2/",
		HTTP:    r.srv.Client(),
		Limiter: r.lim,
		Log:     slog.New(slog.NewTextHandler(r.log, &slog.HandlerOptions{Level: slog.LevelDebug})),
		Creds:   creds,
	})
	return r
}

func (r *ssRig) query(i int) url.Values {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.queries[i]
}

func ssJSON(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}
}

func ssStatus(code int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(code)
		_, _ = io.WriteString(w, body)
	}
}

// ssRun runs fn in a goroutine and advances the fake clock by step whenever a Limiter wait is pending,
// until fn returns. Real time is only a failure guard.
func ssRun(t *testing.T, clock *domain.FakeClock, step time.Duration, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() { defer close(done); fn() }()
	for range 400 {
		ctx, cancel := context.WithCancel(context.Background())
		blocked := make(chan error, 1)
		go func() { blocked <- clock.BlockUntilWaiters(ctx, 1) }()
		select {
		case <-done:
			cancel()
			<-blocked
			return
		case <-blocked:
			cancel()
			clock.Advance(step)
		case <-time.After(5 * time.Second):
			cancel()
			<-blocked
			t.Fatal("chamada nao terminou nem esperou o relogio")
		}
	}
	t.Fatal("limite de passos excedido")
}

func TestScreenScraperRequests(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("SSC-004/toda requisicao leva softname=arcadia e output=json", func(t *testing.T) {
		t.Parallel()
		r := newSSRig(t, func(w http.ResponseWriter, req *http.Request) {
			if strings.HasSuffix(req.URL.Path, "jeuInfos.php") {
				ssJSON(ssInfoJSON)(w, req)
				return
			}
			ssJSON(ssSearchJSON)(w, req)
		}, nil)
		ssRun(t, r.clock, time.Second, func() {
			_, _, _ = r.ss.SearchGames(ctx, 4, "Super Mario World")
			_, _, _ = r.ss.GameInfo(ctx, 4, "1234")
			rc, _, _ := r.ss.OpenVideo(ctx, r.srv.URL+"/media.mp4")
			if rc != nil {
				_ = rc.Close()
			}
		})
		if r.hits.Load() != 3 {
			t.Fatalf("requisicoes = %d, want 3", r.hits.Load())
		}
		for i := 0; i < 2; i++ { // the download URL is the media's own
			q := r.query(i)
			if q.Get("softname") != "arcadia" || q.Get("output") != "json" {
				t.Errorf("requisicao %d: softname=%q output=%q", i, q.Get("softname"), q.Get("output"))
			}
			if q.Get("devid") != ssDevID || q.Get("devpassword") != ssDevPw || q.Get("ssid") != ssUser || q.Get("sspassword") != ssUserPw {
				t.Errorf("requisicao %d sem as quatro credenciais: %v", i, q)
			}
		}
	})

	t.Run("VID-010/a busca envia systemeid e recherche e nenhum parametro de ROM (crc, md5, sha1, romnom, romtaille)", func(t *testing.T) {
		t.Parallel()
		r := newSSRig(t, ssJSON(ssSearchJSON), nil)
		if _, _, err := r.ss.SearchGames(ctx, 4, "Super Mario World"); err != nil {
			t.Fatalf("SearchGames: %v", err)
		}
		q := r.query(0)
		if !strings.HasSuffix(r.paths[0], "jeuRecherche.php") {
			t.Errorf("caminho = %q, want .../jeuRecherche.php", r.paths[0])
		}
		if q.Get("systemeid") != "4" || q.Get("recherche") != "Super Mario World" {
			t.Errorf("systemeid=%q recherche=%q", q.Get("systemeid"), q.Get("recherche"))
		}
		for _, p := range []string{"crc", "md5", "sha1", "romnom", "romtaille"} {
			if _, ok := q[p]; ok {
				t.Errorf("parametro de ROM %q enviado", p)
			}
		}
	})

	t.Run("VID-010/GameInfo envia systemeid e gameid", func(t *testing.T) {
		t.Parallel()
		r := newSSRig(t, ssJSON(ssInfoJSON), nil)
		g, _, err := r.ss.GameInfo(ctx, 4, "1234")
		if err != nil {
			t.Fatalf("GameInfo: %v", err)
		}
		q := r.query(0)
		if !strings.HasSuffix(r.paths[0], "jeuInfos.php") || q.Get("systemeid") != "4" || q.Get("gameid") != "1234" {
			t.Errorf("caminho=%q systemeid=%q gameid=%q", r.paths[0], q.Get("systemeid"), q.Get("gameid"))
		}
		for _, p := range []string{"crc", "md5", "sha1", "romnom", "romtaille"} {
			if _, ok := q[p]; ok {
				t.Errorf("parametro de ROM %q enviado", p)
			}
		}
		// jeuInfos answers response.jeu (singular object), not jeux[].
		if g.ID != "1234" || len(g.Medias) != 1 {
			t.Errorf("GameInfo = %+v, want id 1234 with 1 media", g)
		}
	})

	t.Run("VID-010/decodifica noms, systeme, dates e medias das respostas gravadas", func(t *testing.T) {
		t.Parallel()
		r := newSSRig(t, ssJSON(ssSearchJSON), nil)
		games, quota, err := r.ss.SearchGames(ctx, 4, "Super Mario World")
		if err != nil || len(games) != 1 {
			t.Fatalf("SearchGames = %v, %v", games, err)
		}
		g := games[0]
		if g.ID != "1234" || g.SystemID != 4 {
			t.Errorf("id=%q systemID=%d", g.ID, g.SystemID)
		}
		if len(g.Names) != 2 || g.Names[0] != (scrapers.SSName{Region: "us", Text: "Super Mario World"}) {
			t.Errorf("Names = %+v", g.Names)
		}
		if len(g.Years) != 2 || g.Years[0] != 1991 || g.Years[1] != 1990 {
			t.Errorf("Years = %v, want [1991 1990] (ordem da resposta)", g.Years)
		}
		var vid *scrapers.SSMedia
		for i := range g.Medias {
			if g.Medias[i].Type == "video-normalized" {
				vid = &g.Medias[i]
			}
		}
		if vid == nil || vid.URL != "http://x.test/eu.mp4" || vid.Region != "eu" || vid.Format != "mp4" || vid.Size != 0 {
			t.Errorf("video-normalized = %+v (Size 0 = desconhecido)", vid)
		}
		if !quota.Known || quota.MaxThreads != 1 || quota.MaxPerMinute != 20 || quota.RequestsToday != 10 || quota.MaxPerDay != 20000 || quota.KoToday != 1 || quota.MaxKoPerDay != 2000 {
			t.Errorf("quota = %+v", quota)
		}
	})

	t.Run("VID-010/o limite por minuto tambem e lido com a grafia maxrequestsperdmin", func(t *testing.T) {
		t.Parallel()
		body := `{"response":{"ssuser":{"maxthreads":"1","maxrequestsperdmin":"30","requeststoday":"1","maxrequestsperday":"100"},"jeux":[]}}`
		r := newSSRig(t, ssJSON(body), nil)
		_, q, err := r.ss.SearchGames(ctx, 4, "x")
		if err != nil || q.MaxPerMinute != 30 {
			t.Errorf("MaxPerMinute = %d, err %v; want 30", q.MaxPerMinute, err)
		}
	})

	t.Run("VID-010/jogo sem medias na busca: GameInfo preenche", func(t *testing.T) {
		t.Parallel()
		search := `{"response":{"jeux":[{"id":"1234","noms":[{"region":"us","text":"Super Mario World"}],"systeme":{"id":"4"},"dates":[]}]}}`
		r := newSSRig(t, func(w http.ResponseWriter, req *http.Request) {
			if strings.HasSuffix(req.URL.Path, "jeuInfos.php") {
				ssJSON(ssInfoJSON)(w, req)
				return
			}
			ssJSON(search)(w, req)
		}, nil)
		var games []scrapers.SSGame
		var info scrapers.SSGame
		ssRun(t, r.clock, time.Second, func() {
			games, _, _ = r.ss.SearchGames(ctx, 4, "Super Mario World")
			info, _, _ = r.ss.GameInfo(ctx, 4, games[0].ID)
		})
		if len(games[0].Medias) != 0 {
			t.Errorf("a busca trouxe medias: %+v", games[0].Medias)
		}
		if len(info.Medias) != 1 || info.Medias[0].Type != "video-normalized" || info.Medias[0].URL != "http://x.test/us.mp4" {
			t.Errorf("GameInfo.Medias = %+v", info.Medias)
		}
	})

	t.Run("SSC-004/credenciais lidas a cada requisicao (trocar vale na seguinte)", func(t *testing.T) {
		t.Parallel()
		var n atomic.Int32
		creds := func() (scrapers.SSCreds, error) {
			c := ssGoodCreds
			cr, _ := c()
			if n.Add(1) > 1 {
				cr.User = "SSID-NOVO"
			}
			return cr, nil
		}
		r := newSSRig(t, ssJSON(ssSearchJSON), creds)
		ssRun(t, r.clock, time.Second, func() {
			_, _, _ = r.ss.SearchGames(ctx, 4, "a")
			_, _, _ = r.ss.SearchGames(ctx, 4, "b")
		})
		if r.query(0).Get("ssid") != ssUser || r.query(1).Get("ssid") != "SSID-NOVO" {
			t.Errorf("ssid = %q then %q", r.query(0).Get("ssid"), r.query(1).Get("ssid"))
		}
	})
}

func TestScreenScraperLimiter(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("VID-015/busca, informacoes e download passam pelo Limiter (instantes crescentes, relogio fake)", func(t *testing.T) {
		t.Parallel()
		r := newSSRig(t, func(w http.ResponseWriter, req *http.Request) {
			if strings.HasSuffix(req.URL.Path, "jeuInfos.php") {
				ssJSON(ssInfoJSON)(w, req)
				return
			}
			if strings.HasSuffix(req.URL.Path, "media.mp4") {
				_, _ = io.WriteString(w, "xxxxftypmp42")
				return
			}
			ssJSON(ssSearchJSON)(w, req)
		}, nil)
		ssRun(t, r.clock, 500*time.Millisecond, func() {
			_, _, _ = r.ss.SearchGames(ctx, 4, "a")
			_, _, _ = r.ss.GameInfo(ctx, 4, "1234")
			if rc, _, err := r.ss.OpenVideo(ctx, r.srv.URL+"/media.mp4"); err == nil {
				_ = rc.Close()
			}
		})
		if len(r.times) != 3 {
			t.Fatalf("requisicoes = %d, want 3", len(r.times))
		}
		for i := 1; i < 3; i++ {
			if gap := r.times[i].Sub(r.times[i-1]); gap < scrapers.IntervalScreenScraper {
				t.Errorf("intervalo %d = %v, want >= %v", i, gap, scrapers.IntervalScreenScraper)
			}
		}
	})

	t.Run("VID-015/maxrequestspermin menor ajusta o intervalo para a proxima requisicao", func(t *testing.T) {
		t.Parallel()
		// 10 requests per minute -> SSInterval(10) = 6 s.
		body := `{"response":{"ssuser":{"maxthreads":"1","maxrequestspermin":"10","requeststoday":"1","maxrequestsperday":"20000"},"jeux":[]}}`
		r := newSSRig(t, ssJSON(body), nil)
		ssRun(t, r.clock, 500*time.Millisecond, func() {
			for range 3 {
				_, _, _ = r.ss.SearchGames(ctx, 4, "a")
			}
		})
		if len(r.times) != 3 {
			t.Fatalf("requisicoes = %d, want 3", len(r.times))
		}
		if gap := r.times[2].Sub(r.times[1]); gap < scrapers.SSInterval(10) {
			t.Errorf("intervalo apos o ajuste = %v, want >= %v", gap, scrapers.SSInterval(10))
		}
	})

	t.Run("VID-015/duas chamadas simultaneas nunca estao em voo ao mesmo tempo", func(t *testing.T) {
		t.Parallel()
		var inflight, maxIn atomic.Int32
		entered := make(chan struct{}, 4)
		release := make(chan struct{})
		r := newSSRig(t, func(w http.ResponseWriter, req *http.Request) {
			n := inflight.Add(1)
			for {
				m := maxIn.Load()
				if n <= m || maxIn.CompareAndSwap(m, n) {
					break
				}
			}
			entered <- struct{}{}
			<-release
			inflight.Add(-1)
			ssJSON(`{"response":{"jeux":[]}}`)(w, req)
		}, nil)

		var wg sync.WaitGroup
		for range 2 {
			wg.Add(1)
			go func() { defer wg.Done(); _, _, _ = r.ss.SearchGames(ctx, 4, "a") }()
		}
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			t.Fatal("nenhuma requisicao chegou")
		}
		select { // the second call must not reach the server while the first is in flight
		case <-entered:
			t.Error("a segunda requisicao chegou com a primeira em voo")
		case <-time.After(100 * time.Millisecond):
		}
		close(release)
		// The second call may wait for the Limiter: drive the clock until both finish.
		done := make(chan struct{})
		go func() { wg.Wait(); close(done) }()
		ssRun(t, r.clock, 500*time.Millisecond, func() { <-done })
		if maxIn.Load() != 1 {
			t.Errorf("maximo em voo = %d, want 1", maxIn.Load())
		}
	})
}

func TestScreenScraperErrors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	cases := []struct {
		id     string
		status int
		body   string
		want   error
	}{
		{"VID-016/430 vira ErrQuota", 430, "Votre quota de scrape est atteint", scrapers.ErrQuota},
		{"VID-016/431 vira ErrQuota", 431, "trop de ROMs non reconnues", scrapers.ErrQuota},
		{"VID-016/corpo 'Votre quota de scrape est' com status 200 vira ErrQuota", 200, "Votre quota de scrape est depasse", scrapers.ErrQuota},
		{"VID-017/403 vira ErrCredentials", 403, "Erreur de login", scrapers.ErrCredentials},
		{"VID-017/426 vira ErrCredentials (lista negra)", 426, "Le logiciel de scrape utilisé a été blacklisté", scrapers.ErrCredentials},
		{"VID-017/corpo 'blacklisté' com status 200 vira ErrCredentials", 200, "Le logiciel de scrape utilisé a été blacklisté", scrapers.ErrCredentials},
		{"VID-015/401 vira ErrUnavailable", 401, "API closed for non-registered members", scrapers.ErrUnavailable},
		{"VID-015/423 vira ErrUnavailable", 423, "API totalement fermé", scrapers.ErrUnavailable},
		{"VID-015/429 vira ErrUnavailable", 429, "limite de threads", scrapers.ErrUnavailable},
		{"VID-015/500 vira ErrUnavailable", 500, "oops", scrapers.ErrUnavailable},
		{"VID-015/503 vira ErrUnavailable", 503, "oops", scrapers.ErrUnavailable},
		{"VID-015/corpo 'API closed' com status 200 vira ErrUnavailable", 200, "API closed for non-registered members", scrapers.ErrUnavailable},
		{"VID-015/404 vira ErrNotFound", 404, "Erreur : Jeu non trouvée", scrapers.ErrNotFound},
		{"VID-015/200 com corpo nao JSON 'non trouvée' vira ErrNotFound", 200, "Erreur : Rom/Iso/Dossier non trouvée dans la base de données !", scrapers.ErrNotFound},
		{"VID-015/200 com corpo nao JSON 'Erreur : Rom' vira ErrNotFound", 200, "Erreur : Rom/Iso/Dossier introuvable", scrapers.ErrNotFound},
		{"VID-015/400 vira ErrBadRequest", 400, "Problème dans l'URL", scrapers.ErrBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			t.Parallel()
			r := newSSRig(t, ssStatus(tc.status, tc.body), nil)
			for _, call := range []string{"SearchGames", "GameInfo"} {
				var err error
				if call == "SearchGames" {
					_, _, err = r.ss.SearchGames(ctx, 4, "a")
				} else {
					ssRun(t, r.clock, time.Second, func() { _, _, err = r.ss.GameInfo(ctx, 4, "1") })
				}
				if !errors.Is(err, tc.want) {
					t.Errorf("%s err = %v, want %v", call, err, tc.want)
				}
			}
		})
	}

	t.Run("VID-015/400 registra Warn com a URL limpa", func(t *testing.T) {
		t.Parallel()
		r := newSSRig(t, ssStatus(400, "bad"), nil)
		_, _, _ = r.ss.SearchGames(ctx, 4, "a")
		out := r.log.String()
		if !strings.Contains(out, "level=WARN") {
			t.Errorf("sem Warn no log: %q", out)
		}
		for _, m := range ssMarks {
			if strings.Contains(out, m) {
				t.Errorf("log contem a credencial %q: %s", m, out)
			}
		}
	})

	t.Run("VID-016/requeststoday >= maxrequestsperday na ultima resposta: a proxima chamada devolve ErrQuota sem requisicao", func(t *testing.T) {
		t.Parallel()
		body := `{"response":{"ssuser":{"maxthreads":"1","requeststoday":"100","maxrequestsperday":"100"},"jeux":[]}}`
		r := newSSRig(t, ssJSON(body), nil)
		if _, _, err := r.ss.SearchGames(ctx, 4, "a"); err != nil {
			t.Fatalf("primeira chamada: %v", err)
		}
		before := r.hits.Load()
		_, _, err := r.ss.SearchGames(ctx, 4, "b")
		if !errors.Is(err, scrapers.ErrQuota) {
			t.Errorf("err = %v, want ErrQuota", err)
		}
		if r.hits.Load() != before {
			t.Error("houve requisicao com a cota esgotada")
		}
	})

	t.Run("VID-016/Quota devolve a ultima leitura", func(t *testing.T) {
		t.Parallel()
		r := newSSRig(t, ssJSON(ssSearchJSON), nil)
		if r.ss.Quota().Known {
			t.Error("Quota().Known antes de qualquer resposta")
		}
		_, got, _ := r.ss.SearchGames(ctx, 4, "a")
		if q := r.ss.Quota(); q != got || !q.Known || q.RequestsToday != 10 || q.MaxPerDay != 20000 {
			t.Errorf("Quota() = %+v, retorno da busca = %+v", q, got)
		}
	})

	t.Run("VID-017/credencial ausente (Creds devolve ErrNoCredentials): ErrNoCredentials sem requisicao", func(t *testing.T) {
		t.Parallel()
		r := newSSRig(t, ssJSON(ssSearchJSON), func() (scrapers.SSCreds, error) {
			return scrapers.SSCreds{}, scrapers.ErrNoCredentials
		})
		if _, _, err := r.ss.SearchGames(ctx, 4, "a"); !errors.Is(err, scrapers.ErrNoCredentials) {
			t.Errorf("SearchGames err = %v", err)
		}
		if _, _, err := r.ss.GameInfo(ctx, 4, "1"); !errors.Is(err, scrapers.ErrNoCredentials) {
			t.Errorf("GameInfo err = %v", err)
		}
		if _, _, err := r.ss.OpenVideo(ctx, r.srv.URL+"/m.mp4"); !errors.Is(err, scrapers.ErrNoCredentials) {
			t.Errorf("OpenVideo err = %v", err)
		}
		if r.hits.Load() != 0 {
			t.Errorf("requisicoes = %d, want 0", r.hits.Load())
		}
	})

	t.Run("VID-015/rede e prazo viram ErrUnavailable", func(t *testing.T) {
		t.Parallel()
		// Network: server closed before the call.
		r := newSSRig(t, ssJSON(`{}`), nil)
		r.srv.Close()
		if _, _, err := r.ss.SearchGames(ctx, 4, "a"); !errors.Is(err, scrapers.ErrUnavailable) {
			t.Errorf("rede: err = %v, want ErrUnavailable", err)
		}
		// Deadline: handler hangs until the client gives up.
		r2 := newSSRig(t, func(_ http.ResponseWriter, req *http.Request) { <-req.Context().Done() }, nil)
		dctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		defer cancel()
		if _, _, err := r2.ss.SearchGames(dctx, 4, "a"); !errors.Is(err, scrapers.ErrUnavailable) {
			t.Errorf("prazo: err = %v, want ErrUnavailable", err)
		}
	})

	t.Run("VID-015/contexto cancelado antes da chamada nao faz requisicao", func(t *testing.T) {
		t.Parallel()
		r := newSSRig(t, ssJSON(ssSearchJSON), nil)
		cctx, cancel := context.WithCancel(ctx)
		cancel()
		_, _, err := r.ss.SearchGames(cctx, 4, "a")
		if !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v, want context.Canceled", err)
		}
		if r.hits.Load() != 0 {
			t.Error("houve requisicao com o contexto cancelado")
		}
	})
}

func TestScreenScraperRedaction(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	noMarks := func(t *testing.T, what, s string) {
		t.Helper()
		for _, m := range ssMarks {
			if strings.Contains(s, m) {
				t.Errorf("%s contem a credencial %q: %s", what, m, s)
			}
		}
	}

	t.Run("VID-020/valores marcados de devid, devpassword, ssid e sspassword nao aparecem no log capturado, no erro devolvido nem no texto de OpenVideo", func(t *testing.T) {
		t.Parallel()
		var failing atomic.Bool
		r := newSSRig(t, func(w http.ResponseWriter, req *http.Request) {
			if failing.Load() {
				ssStatus(500, "boom "+req.URL.RawQuery)(w, req) // even if the server echoes the URL
				return
			}
			ssJSON(ssSearchJSON)(w, req)
		}, nil)
		var errs []string
		ssRun(t, r.clock, time.Second, func() {
			_, _, err := r.ss.SearchGames(ctx, 4, "a")
			if err != nil {
				errs = append(errs, err.Error())
			}
			failing.Store(true)
			for _, e := range []error{
				func() error { _, _, e := r.ss.SearchGames(ctx, 4, "a"); return e }(),
				func() error { _, _, e := r.ss.GameInfo(ctx, 4, "1"); return e }(),
				func() error { _, _, e := r.ss.OpenVideo(ctx, r.srv.URL+"/m.mp4"); return e }(),
			} {
				if e != nil {
					errs = append(errs, e.Error())
				}
			}
		})
		for _, e := range errs {
			noMarks(t, "erro", e)
		}
		noMarks(t, "log", r.log.String())
	})

	t.Run("VID-020/o erro de rede traz a URL sem as credenciais", func(t *testing.T) {
		t.Parallel()
		r := newSSRig(t, ssJSON(`{}`), nil)
		host := strings.TrimPrefix(r.srv.URL, "http://")
		r.srv.Close()
		_, _, err := r.ss.SearchGames(ctx, 4, "a")
		if err == nil {
			t.Fatal("err = nil")
		}
		noMarks(t, "erro de rede", err.Error())
		noMarks(t, "log", r.log.String())
		if !strings.Contains(err.Error(), host) {
			t.Logf("aviso: o erro nao cita o host %q: %v", host, err)
		}
		var ue *url.Error
		if errors.As(err, &ue) {
			noMarks(t, "url.Error.URL", ue.URL)
		}
	})

	t.Run("VID-020/OpenVideo acrescenta as credenciais em memoria e devolve Content-Length", func(t *testing.T) {
		t.Parallel()
		payload := "0000ftypmp42" + strings.Repeat("v", 100)
		r := newSSRig(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "video/mp4")
			w.Header().Set("Content-Length", fmt.Sprint(len(payload)))
			_, _ = io.WriteString(w, payload)
		}, nil)
		var rc io.ReadCloser
		var size int64
		var err error
		ssRun(t, r.clock, time.Second, func() { rc, size, err = r.ss.OpenVideo(ctx, r.srv.URL+"/media.mp4?systemeid=4&jeuid=1") })
		if err != nil {
			t.Fatalf("OpenVideo: %v", err)
		}
		defer rc.Close()
		got, _ := io.ReadAll(rc)
		if string(got) != payload || size != int64(len(payload)) {
			t.Errorf("corpo %d bytes, size %d; want %d", len(got), size, len(payload))
		}
		q := r.query(0)
		if q.Get("devid") != ssDevID || q.Get("devpassword") != ssDevPw || q.Get("ssid") != ssUser || q.Get("sspassword") != ssUserPw {
			t.Errorf("credenciais ausentes na requisicao de download: %v", q)
		}
		if q.Get("systemeid") != "4" || q.Get("jeuid") != "1" {
			t.Errorf("parametros originais perdidos: %v", q)
		}
		noMarks(t, "log", r.log.String())
	})

	t.Run("VID-020/OpenVideo nao duplica credenciais que a URL ja traz", func(t *testing.T) {
		t.Parallel()
		r := newSSRig(t, ssVideo, nil)
		u := r.srv.URL + "/m.mp4?devid=" + ssDevID + "&devpassword=" + ssDevPw + "&ssid=" + ssUser + "&sspassword=" + ssUserPw
		rc, _, err := r.ss.OpenVideo(ctx, u)
		if err != nil {
			t.Fatalf("OpenVideo: %v", err)
		}
		_ = rc.Close()
		q := r.query(0)
		for _, k := range []string{"devid", "devpassword", "ssid", "sspassword"} {
			if len(q[k]) != 1 {
				t.Errorf("%s aparece %d vezes", k, len(q[k]))
			}
		}
		noMarks(t, "log", r.log.String())
	})
}
