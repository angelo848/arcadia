package agent

// Tests of T26 (docs/specs/phase-13-modo-console/tasks.md): the video track wired in app.go. The agent is
// the real one (startAgent over TCP, two raw IPC clients), the ScreenScraper is an httptest server reached
// through config.ScreenScraperURL, the credentials live in an in-memory key store (config.KeyStore) so the
// Credential Manager is never touched, the clock is a FakeClock and nothing sleeps [CON-029].
// Alterado em 2026-11-15: arquivo novo (fase 13, T26).
//
// Assumed production surface (T26): config.KeyStore scrapers.KeyStore (nil => the Credential Manager).

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite" // registers the "sqlite" driver used to read the tables

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
	"github.com/arcadia-launcher/arcadia/internal/scrapers"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

// lockedKeys is a concurrency-safe in-memory scrapers.KeyStore (the agent reads it from its goroutines).
type lockedKeys struct {
	mu sync.Mutex
	m  map[string]string
}

func (k *lockedKeys) Set(name, value string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.m[name] = value
	return nil
}

func (k *lockedKeys) Get(name string) (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	v, ok := k.m[name]
	if !ok {
		return "", scrapers.ErrKeyNotFound
	}
	return v, nil
}

func (k *lockedKeys) Delete(name string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	delete(k.m, name)
	return nil
}

// ssServer is the fake ScreenScraper: jeuRecherche.php, jeuInfos.php and one video file.
type ssServer struct {
	srv *httptest.Server

	mu         sync.Mutex
	mode       string // ok | 403 | 431 | nomatch | hold
	searches   int
	infos      int
	downloads  int
	hold       chan struct{} // closed to release a held download
	holdOnce   sync.Once
	inDownload chan struct{} // receives one value per download that starts
	cancelled  chan struct{} // receives one value per download whose request context ended
}

func newSSServer(t *testing.T, sysID int) *ssServer {
	t.Helper()
	s := &ssServer{
		mode: "ok", hold: make(chan struct{}),
		inDownload: make(chan struct{}, 8), cancelled: make(chan struct{}, 8),
	}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Connection", "close")
		s.mu.Lock()
		mode := s.mode
		s.mu.Unlock()
		switch {
		case strings.HasSuffix(r.URL.Path, "jeuRecherche.php"):
			s.mu.Lock()
			s.searches++
			s.mu.Unlock()
			games := fmt.Sprintf(`[{"id":"1","noms":[{"region":"wor","text":"Super Mario 64"}],"systeme":{"id":"%d"},`+
				`"dates":[{"region":"wor","text":"1996-06-23"}],`+
				`"medias":[{"type":"video-normalized","region":"wor","format":"mp4","url":"%s/media/v.mp4","size":"60"}]}]`, sysID, s.srv.URL)
			if mode == "nomatch" {
				games = `[]`
			}
			_, _ = fmt.Fprintf(w, `{"response":{"ssuser":{"maxthreads":"1","maxrequestspermin":"40","requeststoday":"3","maxrequestsperday":"20000"},"jeux":%s}}`, games)
		case strings.HasSuffix(r.URL.Path, "jeuInfos.php"):
			s.mu.Lock()
			s.infos++
			s.mu.Unlock()
			http.Error(w, "nope", http.StatusNotFound)
		case strings.HasSuffix(r.URL.Path, "/media/v.mp4"):
			s.mu.Lock()
			s.downloads++
			s.mu.Unlock()
			select {
			case s.inDownload <- struct{}{}:
			default:
			}
			switch mode {
			case "403":
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			case "431":
				http.Error(w, "quota", 431)
				return
			case "hold":
				select {
				case <-s.hold:
				case <-r.Context().Done():
					select {
					case s.cancelled <- struct{}{}:
					default:
					}
					return
				}
			}
			body := mp4Bytes()
			w.Header().Set("Content-Type", "video/mp4")
			w.Header().Set("Content-Length", fmt.Sprint(len(body)))
			_, _ = w.Write(body)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(s.srv.Close)
	return s
}

func (s *ssServer) setMode(m string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mode = m
}

func (s *ssServer) releaseHold() { s.holdOnce.Do(func() { close(s.hold) }) }

func (s *ssServer) counts() (searches, infos, downloads int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.searches, s.infos, s.downloads
}

// videoWire is a real agent with the video track, two clients and games on a mapped platform.
type videoWire struct {
	*wiring
	ss     *ssServer
	keys   *lockedKeys
	c1, c2 *rawClient
	games  []int64
}

type videoWireOpts struct {
	games       int
	credentials bool // devid, devpassword, user and password registered (marked values)
	providers   func(name string) (scrapers.MetadataProvider, error)
	seedDirs    bool // creates <mediaRoot>/<G>/old.mp4 for every game before the start
}

func newVideoWire(t *testing.T, ss *ssServer, o videoWireOpts) *videoWire {
	t.Helper()
	ks := &lockedKeys{m: map[string]string{}}
	if o.credentials {
		keys := scrapers.NewKeys(ks)
		if err := keys.SetPair("screenscraperdev", secDevID, secDevPass); err != nil {
			t.Fatalf("SetPair dev: %v", err)
		}
		if err := keys.SetPair("screenscraper", secUser, secPass); err != nil {
			t.Fatalf("SetPair user: %v", err)
		}
	}
	prov := &metaFetchProv{metaProv: metaProv{id: "igdb", meta: scrapers.Metadata{Description: "Texto"}}}
	providers := o.providers
	if providers == nil {
		providers = func(name string) (scrapers.MetadataProvider, error) {
			if name != "igdb" {
				return nil, errUnknownProvider
			}
			return prov, nil
		}
	}
	vw := &videoWire{ss: ss, keys: ks}
	vw.wiring = newWiring(t, wiringOpts{
		mutate: func(cfg *config) {
			cfg.ScreenScraperURL = ss.srv.URL
			cfg.KeyStore = ks
			cfg.Providers = providers
		},
		before: func(t *testing.T, cfg config) {
			if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
				t.Fatalf("mkdir data dir: %v", err)
			}
			st, err := store.Open(t.Context(), dbPath(cfg), store.OpenOptions{Migrate: true})
			if err != nil {
				t.Fatalf("store.Open: %v", err)
			}
			defer func() { _ = st.Close() }()
			for i := 0; i < o.games; i++ {
				g, err := st.AddGame(t.Context(), store.NewGame{Title: "Super Mario 64", Exe: fmt.Sprintf(`C:\mario%d.exe`, i), At: videoT0})
				if err != nil {
					t.Fatalf("AddGame: %v", err)
				}
				id := g.ID
				setGamePlatform(t, st, id, platMapped)
				vw.games = append(vw.games, id)
				if o.seedDirs {
					dir := filepath.Join(cfg.DataDir, "media", fmt.Sprint(id))
					if err := os.MkdirAll(dir, 0o755); err != nil {
						t.Fatalf("mkdir media dir: %v", err)
					}
					if err := os.WriteFile(filepath.Join(dir, "old.mp4"), mp4Bytes(), 0o644); err != nil {
						t.Fatalf("write old.mp4: %v", err)
					}
				}
			}
		},
	})
	// Registered after newWiring: runs before the agent shutdown, so a held download can end.
	t.Cleanup(ss.releaseHold)
	vw.c1, vw.c2 = vw.client(t), vw.client(t)
	return vw
}

// pumpUntil polls cond and advances the fake clock between polls (the ScreenScraper limiter waits on it).
func pumpUntil(t *testing.T, clk *domain.FakeClock, what string, cond func() bool) {
	t.Helper()
	deadline := time.After(testTimeout)
	for {
		if cond() {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("timeout waiting for %s", what)
		case <-time.After(10 * time.Millisecond):
			clk.Advance(2 * time.Second)
		}
	}
}

func (vw *videoWire) status(t *testing.T) ipc.VideoStatus {
	t.Helper()
	var res ipc.VideoStatusResult
	resultInto(t, vw.c1.rpcOK(t, ipc.MethodVideoStatus, nil), &res)
	return res.Status
}

func (vw *videoWire) waitLane(t *testing.T, lane string) {
	t.Helper()
	pumpUntil(t, vw.clock, "video lane "+lane, func() bool { return vw.status(t).Lane == lane })
}

func (vw *videoWire) apply(t *testing.T, gameID int64) {
	t.Helper()
	vw.c1.rpcOK(t, ipc.MethodMetadataApply, ipc.MetadataApplyParams{GameID: gameID, Refs: refsOf("7", ipc.MetadataProviderIGDB)})
}

func (vw *videoWire) mediaChanged(c *rawClient) int {
	return countMethod(c.snapshotAll(), ipc.MethodMediaChanged)
}

func (vw *videoWire) waitMediaChanged(t *testing.T, c *rawClient, label string, gameID int64) {
	t.Helper()
	pumpUntil(t, vw.clock, label+" media.changed", func() bool {
		for _, m := range c.snapshotAll() {
			if m.method() != ipc.MethodMediaChanged {
				continue
			}
			var p ipc.MediaChangedParams
			if err := json.Unmarshal(m["params"], &p); err == nil && p.GameID == gameID {
				return true
			}
		}
		return false
	})
}

func mappedSystem(t *testing.T) int {
	t.Helper()
	id, ok := scrapers.SystemID("n64")
	if !ok {
		t.Fatal("scrapers.SystemID(n64) is not mapped")
	}
	return id
}

func TestVideoWireMediaChanged(t *testing.T) {
	t.Run("VID-021/dois clientes IPC reais recebem media.changed quando o vídeo é gravado (ScreenScraper em httptest com MP4 pequeno)", func(t *testing.T) {
		ss := newSSServer(t, mappedSystem(t))
		vw := newVideoWire(t, ss, videoWireOpts{games: 1, credentials: true})
		g := vw.games[0]
		if n := vw.mediaChanged(vw.c1) + vw.mediaChanged(vw.c2); n != 0 {
			t.Fatalf("media.changed before the video is stored = %d, want 0", n)
		}
		vw.apply(t, g)
		vw.waitMediaChanged(t, vw.c1, "client 1", g)
		vw.waitMediaChanged(t, vw.c2, "client 2", g)
		var res ipc.MediaListResult
		resultInto(t, vw.c2.rpcOK(t, ipc.MethodMediaList, ipc.MediaListParams{GameID: g}), &res)
		if len(res.Media) != 1 || res.Media[0].Kind != "video" {
			t.Fatalf("media.list = %+v, want one video", res.Media)
		}
		if _, err := os.Stat(res.Media[0].Path); err != nil {
			t.Errorf("stored video %s: %v", res.Media[0].Path, err)
		}
	})

	t.Run("VID-021/falha, cancelamento e pausa não avisam", func(t *testing.T) {
		ss := newSSServer(t, mappedSystem(t))
		vw := newVideoWire(t, ss, videoWireOpts{games: 2, credentials: true})

		// Failure: no match is a permanent failure.
		ss.setMode("nomatch")
		vw.apply(t, vw.games[0])
		pumpUntil(t, vw.clock, "failed job", func() bool { return vw.status(t).Failed == 1 })

		// Pause: the credentials are refused.
		ss.setMode("403")
		vw.apply(t, vw.games[1])
		vw.waitLane(t, ipc.VideoLanePausedCredentials)

		// Cancellation.
		var cancelled ipc.VideoCancelResult
		resultInto(t, vw.c1.rpcOK(t, ipc.MethodVideoCancelPending, nil), &cancelled)
		if cancelled.Cancelled != 1 {
			t.Fatalf("cancelled = %d, want 1", cancelled.Cancelled)
		}

		// A sentinel proves the notices of the others would have arrived by now: a good download for a
		// third game ends with exactly one media.changed.
		ss.setMode("ok")
		// Only saving the credentials again takes the lane out of paused-credentials (VID-017).
		vw.c1.rpcOK(t, ipc.MethodProviderSetKey, ipc.ProviderSetKeyParams{Provider: "screenscraper", Key: secUser, Secret: secPass})
		third := vw.addGame(t, vw.c1)
		vw.apply(t, third)
		vw.waitMediaChanged(t, vw.c1, "sentinel client 1", third)
		vw.waitMediaChanged(t, vw.c2, "sentinel client 2", third)
		for i, c := range []*rawClient{vw.c1, vw.c2} {
			for _, m := range c.snapshotAll() {
				if m.method() != ipc.MethodMediaChanged {
					continue
				}
				var p ipc.MediaChangedParams
				_ = json.Unmarshal(m["params"], &p)
				if p.GameID != third {
					t.Errorf("client %d: unexpected media.changed for game %d (failure, cancel and pause must not notify)", i+1, p.GameID)
				}
			}
		}
	})
}

// addGame registers a game on the mapped platform through the store of the running agent... the agent is the
// only writer of its database (ADR 0015), so the game goes in through game.add and game.update.
func (vw *videoWire) addGame(t *testing.T, c *rawClient) int64 {
	t.Helper()
	id, _ := vw.wiring.addGame(t, c, "Super Mario 64")
	var plats ipc.PlatformListResult
	resultInto(t, c.rpcOK(t, ipc.MethodPlatformList, nil), &plats)
	var pid int64
	for _, p := range plats.Platforms {
		if strings.EqualFold(p.Name, platMapped) {
			pid = p.ID
		}
	}
	if pid == 0 {
		t.Fatalf("platform %q not found in %+v", platMapped, plats.Platforms)
	}
	c.rpcOK(t, ipc.MethodGameUpdate, map[string]any{"gameId": id, "platformId": pid})
	return id
}

func TestVideoWireNoSecrets(t *testing.T) {
	t.Run("VID-020/fluxo completo (aplicar metadados, busca, download, erro 403, cota 431, cancelamento, status) com devid, devpassword, usuário e senha de valor marcado: o valor não aparece no log capturado, nas linhas de media_job, em media.source_url, nas respostas IPC nem em video.status", func(t *testing.T) {
		ss := newSSServer(t, mappedSystem(t))
		vw := newVideoWire(t, ss, videoWireOpts{games: 2, credentials: true})
		g1, g2 := vw.games[0], vw.games[1]

		// Download that works (media.source_url is written).
		vw.apply(t, g1)
		vw.waitMediaChanged(t, vw.c1, "first video", g1)

		// 403: credentials refused, lane paused.
		ss.setMode("403")
		vw.apply(t, g2)
		vw.waitLane(t, ipc.VideoLanePausedCredentials)
		statuses := []ipc.VideoStatus{vw.status(t)}

		// Saving the credentials again resumes the lane; now the daily quota is spent (431).
		ss.setMode("431")
		vw.c1.rpcOK(t, ipc.MethodProviderSetKey, ipc.ProviderSetKeyParams{Provider: "screenscraper", Key: secUser, Secret: secPass})
		vw.waitLane(t, ipc.VideoLanePausedQuota)
		statuses = append(statuses, vw.status(t))

		var cancelled ipc.VideoCancelResult
		resultInto(t, vw.c1.rpcOK(t, ipc.MethodVideoCancelPending, nil), &cancelled)
		if cancelled.Cancelled != 1 {
			t.Errorf("cancelled = %d, want 1", cancelled.Cancelled)
		}
		statuses = append(statuses, vw.status(t))

		for i, s := range statuses {
			raw, _ := json.Marshal(s)
			assertNoSecrets(t, fmt.Sprintf("video.status #%d", i), string(raw))
		}
		for i, c := range []*rawClient{vw.c1, vw.c2} {
			raw, _ := json.Marshal(c.snapshotAll())
			assertNoSecrets(t, fmt.Sprintf("IPC messages of client %d", i+1), string(raw))
		}

		vw.shutdown(t)
		logRaw, err := os.ReadFile(agentLogPath(vw.cfg))
		if err != nil {
			t.Fatalf("read log: %v", err)
		}
		assertNoSecrets(t, "agent.log", string(logRaw))

		db, err := sql.Open("sqlite", dbPath(vw.cfg))
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		defer db.Close()
		var dump strings.Builder
		sawSource := false
		for _, table := range []string{"media_job", "media"} {
			rows, err := db.Query("SELECT * FROM " + table)
			if err != nil {
				t.Fatalf("select %s: %v", table, err)
			}
			cols, _ := rows.Columns()
			for rows.Next() {
				vals := make([]any, len(cols))
				ptrs := make([]any, len(cols))
				for i := range vals {
					ptrs[i] = &vals[i]
				}
				if err := rows.Scan(ptrs...); err != nil {
					t.Fatalf("scan %s: %v", table, err)
				}
				for i, v := range vals {
					if b, ok := v.([]byte); ok {
						v = string(b)
					}
					fmt.Fprintf(&dump, "%s.%s=%v\n", table, cols[i], v)
					if table == "media" && cols[i] == "source_url" && v != "" && v != nil {
						sawSource = true
					}
				}
			}
			if err := rows.Err(); err != nil {
				t.Fatalf("rows %s: %v", table, err)
			}
			_ = rows.Close()
		}
		if !sawSource {
			t.Error("no media.source_url written: the first download did not run")
		}
		assertNoSecrets(t, "media_job and media rows", dump.String())
	})
}

func TestVideoWireGameDelete(t *testing.T) {
	t.Run("VID-025/game.delete com job de vídeo em curso: o download é interrompido antes de apagar a pasta e o .mp4 some", func(t *testing.T) {
		ss := newSSServer(t, mappedSystem(t))
		ss.setMode("hold")
		vw := newVideoWire(t, ss, videoWireOpts{games: 1, credentials: true, seedDirs: true})
		g := vw.games[0]
		dir := filepath.Join(vw.cfg.DataDir, "media", fmt.Sprint(g))
		if _, err := os.Stat(filepath.Join(dir, "old.mp4")); err != nil {
			t.Fatalf("seeded video: %v", err)
		}
		vw.apply(t, g)
		pumpUntil(t, vw.clock, "download started", func() bool {
			select {
			case <-ss.inDownload:
				return true
			default:
				return false
			}
		})

		vw.c1.rpcOK(t, ipc.MethodGameDelete, ipc.GameDeleteParams{GameID: g})

		select {
		case <-ss.cancelled:
		case <-time.After(testTimeout):
			t.Fatal("the in-flight download was not interrupted by game.delete")
		}
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("media folder %s still exists after game.delete (err=%v)", dir, err)
		}
		if n := vw.mediaChanged(vw.c1); n != 0 {
			t.Errorf("media.changed = %d after an interrupted download, want 0", n)
		}
	})
}

func TestVideoWireNoScreenScraperInMetadata(t *testing.T) {
	t.Run("SSC-005/o agente não consulta o ScreenScraper em metadata.search, metadata.apply e no lote (servidor httptest com contador zero)", func(t *testing.T) {
		ss := newSSServer(t, mappedSystem(t))
		prov := func(name string) (scrapers.MetadataProvider, error) {
			if name == "igdb" {
				return &metaFetchProv{metaProv: metaProv{id: name, meta: scrapers.Metadata{Description: "Texto"}}}, nil
			}
			for _, b := range batchProviders {
				if b == name {
					return &metaProv{id: name}, nil
				}
			}
			return nil, errUnknownProvider
		}
		// No credentials: the video hook of VID-004 stays silent, so any request to the server would come
		// from the metadata path itself.
		vw := newVideoWire(t, ss, videoWireOpts{games: 1, credentials: false, providers: prov})
		g := vw.games[0]
		vw.c1.rpcOK(t, ipc.MethodMetadataSearch, ipc.MetadataSearchParams{Title: "Super Mario 64", Platform: platMapped})
		vw.apply(t, g)
		vw.c1.rpcOK(t, ipc.MethodMetadataBatchStart, ipc.BatchStartParams{GameIDs: []int64{g}})
		vw.shutdown(t) // waits for the batch
		if s, i, d := ss.counts(); s+i+d != 0 {
			t.Errorf("ScreenScraper requests = search %d, info %d, download %d; want zero", s, i, d)
		}
	})
}

func TestVideoWireSuiteIsOffline(t *testing.T) {
	t.Run("CON-029/a suíte da trilha não faz rede nem Sleep", func(t *testing.T) {
		sleep := "time." + "Sleep("
		files := []string{
			"video_test.go", "video_wire_test.go", "video_lane_test.go", "video_resolver_test.go",
			"metadata_apply_test.go", "metadata_batch_apply_test.go",
		}
		for _, f := range files {
			raw, err := os.ReadFile(f)
			if err != nil {
				t.Fatalf("read %s: %v", f, err)
			}
			if bytes.Contains(raw, []byte(sleep)) {
				t.Errorf("%s calls %s: use channels and the fake clock", f, sleep)
			}
			if bytes.Contains(raw, []byte("http."+"DefaultClient")) || bytes.Contains(raw, []byte("http."+"Get(")) {
				t.Errorf("%s uses the default HTTP client: tests must only reach httptest servers", f)
			}
		}
	})
}
