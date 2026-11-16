package agent

// Testes do resolvedor de vídeo (fase 13, T23: VID-010..VID-014, VID-020).
// Assinaturas assumidas (tasks.md T23):
//
//	newVideoResolver(g videoGames, ss ssAPI, files videoFiles, log *slog.Logger) *videoResolver
//	(*videoResolver).Fetch(ctx, store.MediaJob) (media.Stored, bool, error)
//	pickSSGame(term string, year int, seedKey string, systemID int, games []scrapers.SSGame) (*scrapers.SSGame, domain.Confidence)
//	pickVideoMedia(g scrapers.SSGame) (scrapers.SSMedia, bool)
//	media.Stored.SourceURL (T19): URL da mídia sem credenciais.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/media"
	"github.com/arcadia-launcher/arcadia/internal/scrapers"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

const (
	secDevID   = "DEVID-SEGREDO-1"
	secDevPass = "DEVPASS-SEGREDO-2"
	secUser    = "USER-SEGREDO-3"
	secPass    = "SSPASS-SEGREDO-4"
)

var secrets = []string{secDevID, secDevPass, secUser, secPass}

// vidGames is a videoGames over a map.
type vidGames struct {
	info map[int64]store.VideoGameInfo
	err  error
}

func (g vidGames) VideoGameInfo(_ context.Context, id int64) (store.VideoGameInfo, error) {
	if g.err != nil {
		return store.VideoGameInfo{}, g.err
	}
	i, ok := g.info[id]
	if !ok {
		return store.VideoGameInfo{}, store.ErrGameNotFound
	}
	return i, nil
}

// trackedBody is a ReadCloser that records whether it was read and closed.
type trackedBody struct {
	r      io.Reader
	mu     sync.Mutex
	read   bool
	closed bool
}

func (b *trackedBody) Read(p []byte) (int, error) {
	b.mu.Lock()
	b.read = true
	b.mu.Unlock()
	return b.r.Read(p)
}

func (b *trackedBody) Close() error {
	b.mu.Lock()
	b.closed = true
	b.mu.Unlock()
	return nil
}

type searchCall struct {
	system int
	term   string
}

// fakeSS is an ssAPI.
type fakeSS struct {
	mu          sync.Mutex
	games       []scrapers.SSGame
	searchErr   error
	info        scrapers.SSGame
	infoErr     error
	body        []byte
	openSize    int64
	openErr     error
	quota       scrapers.SSQuota
	searches    []searchCall
	infos       []string
	opened      []string
	lastBody    *trackedBody
	requestsCnt int
}

func (f *fakeSS) SearchGames(_ context.Context, sys int, term string) ([]scrapers.SSGame, scrapers.SSQuota, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.searches = append(f.searches, searchCall{sys, term})
	f.requestsCnt++
	return f.games, f.quota, f.searchErr
}

func (f *fakeSS) GameInfo(_ context.Context, _ int, gameID string) (scrapers.SSGame, scrapers.SSQuota, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.infos = append(f.infos, gameID)
	f.requestsCnt++
	return f.info, f.quota, f.infoErr
}

func (f *fakeSS) OpenVideo(_ context.Context, u string) (io.ReadCloser, int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.opened = append(f.opened, u)
	f.requestsCnt++
	if f.openErr != nil {
		return nil, 0, f.openErr
	}
	f.lastBody = &trackedBody{r: bytes.NewReader(f.body)}
	return f.lastBody, f.openSize, nil
}

func (f *fakeSS) Quota() scrapers.SSQuota {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.quota
}

// vidFiles is a videoFiles that drains the body.
type vidFiles struct {
	mu      sync.Mutex
	calls   int
	gameID  int64
	size    int64
	body    []byte
	err     error
	stored  media.Stored
	written bool
}

func (f *vidFiles) FetchVideo(_ context.Context, gameID int64, r io.Reader, size int64) (media.Stored, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.gameID, f.size = gameID, size
	f.body, _ = io.ReadAll(r)
	if f.err != nil {
		return media.Stored{}, false, f.err
	}
	st := f.stored
	if st.Hash == "" {
		st = media.Stored{Hash: "abc123", Path: fmt.Sprintf("%d/abc123.mp4", gameID)}
	}
	return st, true, nil
}

func ssNames(texts ...string) []scrapers.SSName {
	out := make([]scrapers.SSName, 0, len(texts))
	for _, t := range texts {
		out = append(out, scrapers.SSName{Region: "wor", Text: t})
	}
	return out
}

func ssGame(id string, sys int, years []int, texts ...string) scrapers.SSGame {
	return scrapers.SSGame{
		ID: id, SystemID: sys, Names: ssNames(texts...), Years: years,
		Medias: []scrapers.SSMedia{{Type: "video-normalized", Region: "wor", Format: "mp4", URL: "https://media.example.test/" + id + ".mp4"}},
	}
}

type resolverRig struct {
	r     *videoResolver
	ss    *fakeSS
	files *vidFiles
	logs  *lockedBuf
}

func newResolverRig(t *testing.T, info store.VideoGameInfo, ss *fakeSS) resolverRig {
	t.Helper()
	files := &vidFiles{}
	logs := &lockedBuf{}
	log := slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	r := newVideoResolver(vidGames{info: map[int64]store.VideoGameInfo{7: info}}, ss, files, log)
	return resolverRig{r: r, ss: ss, files: files, logs: logs}
}

func videoJobOf(game int64) store.MediaJob {
	return store.MediaJob{ID: 1, GameID: game, Kind: "video", Provider: store.VideoProvider, SourceURL: store.VideoLookupURL, State: store.JobRunning}
}

func isPermanent(err error, msg string) bool {
	var pe *media.PermanentError
	return errors.As(err, &pe) && pe.Msg == msg
}

func mp4Bytes() []byte {
	return append([]byte{0, 0, 0, 0x18, 'f', 't', 'y', 'p', 'm', 'p', '4', '2'}, bytes.Repeat([]byte{1}, 32)...)
}

func TestVideoResolverVectors(t *testing.T) {
	t.Parallel()

	const sys = 4 // snes
	cases := []struct {
		name  string
		title string
		games []scrapers.SSGame
		want  int // index of the game that must match; -1 = no match
	}{
		{"VID-011/Super Mario World (USA) × Super Mario World e Yoshi's Island: match no primeiro", "Super Mario World (USA)",
			[]scrapers.SSGame{ssGame("1", sys, nil, "Super Mario World"), ssGame("2", sys, nil, "Super Mario World 2: Yoshi's Island")}, 0},
		{"VID-011/Resident Evil × Resident Evil 2 e Resident Evil: match no certo", "Resident Evil",
			[]scrapers.SSGame{ssGame("1", sys, nil, "Resident Evil 2"), ssGame("2", sys, nil, "Resident Evil")}, 1},
		{"VID-011/Mega Man X × Mega Man: sem correspondência", "Mega Man X",
			[]scrapers.SSGame{ssGame("1", sys, nil, "Mega Man")}, -1},
		{"VID-011/Legend of Zelda, The - Ocarina of Time × The Legend of Zelda: Ocarina of Time: match", "Legend of Zelda, The - Ocarina of Time",
			[]scrapers.SSGame{ssGame("1", sys, nil, "The Legend of Zelda: Ocarina of Time")}, 0},
		{"VID-011/Final Fantasy VII com dois nomes regionais no mesmo jogo: match", "Final Fantasy VII",
			[]scrapers.SSGame{ssGame("1", sys, nil, "Final Fantasy VII", "Final Fantasy VII International")}, 0},
		{"VID-011/Tetris × dois Tetris homônimos: sem correspondência (margem < 0,05)", "Tetris",
			[]scrapers.SSGame{ssGame("1", sys, nil, "Tetris"), ssGame("2", sys, nil, "Tetris")}, -1},
		{"VID-011/lista vazia: sem correspondência", "Chrono Trigger", nil, -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			term := domain.SearchTerm(tc.title)
			best, conf := pickSSGame(term, 0, "snes", sys, tc.games)
			if tc.want < 0 {
				if conf == domain.ConfidenceMatch {
					t.Fatalf("pickSSGame = match (%+v), want doubt or none", best)
				}
			} else {
				if conf != domain.ConfidenceMatch || best == nil || best.ID != tc.games[tc.want].ID {
					t.Fatalf("pickSSGame = %v %+v, want match on game %s", conf, best, tc.games[tc.want].ID)
				}
			}

			// Through Fetch: only a match downloads; the rest is permanent without opening the video.
			ss := &fakeSS{games: tc.games, body: mp4Bytes(), openSize: 44}
			rig := newResolverRig(t, store.VideoGameInfo{Title: tc.title, SeedKey: "snes"}, ss)
			_, _, err := rig.r.Fetch(t.Context(), videoJobOf(7))
			if tc.want < 0 {
				if !isPermanent(err, "sem correspondência no ScreenScraper") {
					t.Fatalf("Fetch err = %v, want *media.PermanentError \"sem correspondência no ScreenScraper\"", err)
				}
				if len(ss.opened) != 0 || rig.files.calls != 0 {
					t.Errorf("opened %v, FetchVideo calls %d; want none", ss.opened, rig.files.calls)
				}
				if ss.requestsCnt != 1 {
					t.Errorf("requests = %d, want only the search (no quota beyond it)", ss.requestsCnt)
				}
				return
			}
			if err != nil {
				t.Fatalf("Fetch: %v", err)
			}
			if len(ss.opened) != 1 || ss.opened[0] != tc.games[tc.want].Medias[0].URL {
				t.Errorf("opened = %v, want the video of game %s", ss.opened, tc.games[tc.want].ID)
			}
		})
	}

	t.Run("VID-011/doubt e none são *media.PermanentError e nunca baixam", func(t *testing.T) {
		t.Parallel()
		for name, games := range map[string][]scrapers.SSGame{
			"none":  nil,
			"doubt": {ssGame("1", sys, nil, "Super Mario World 2")},
		} {
			ss := &fakeSS{games: games, body: mp4Bytes(), openSize: 44}
			rig := newResolverRig(t, store.VideoGameInfo{Title: "Super Mario World", SeedKey: "snes"}, ss)
			_, _, err := rig.r.Fetch(t.Context(), videoJobOf(7))
			if !isPermanent(err, "sem correspondência no ScreenScraper") {
				t.Errorf("%s: err = %v, want permanent \"sem correspondência no ScreenScraper\"", name, err)
			}
			if len(ss.opened) != 0 || len(ss.infos) != 0 || rig.files.calls != 0 {
				t.Errorf("%s: opened %v, infos %v, files %d; want no spend beyond the search", name, ss.opened, ss.infos, rig.files.calls)
			}
		}
	})
}

func TestVideoResolverLookup(t *testing.T) {
	t.Parallel()

	good := func() *fakeSS {
		return &fakeSS{games: []scrapers.SSGame{ssGame("10", 4, nil, "Super Metroid")}, body: mp4Bytes(), openSize: int64(len(mp4Bytes()))}
	}
	info := store.VideoGameInfo{Title: "Super Metroid (USA) [!]", SeedKey: "snes"}

	t.Run("VID-010/o termo é domain.SearchTerm do título e o sistema é o da chave de seed", func(t *testing.T) {
		t.Parallel()
		ss := good()
		rig := newResolverRig(t, info, ss)
		if _, _, err := rig.r.Fetch(t.Context(), videoJobOf(7)); err != nil {
			t.Fatalf("Fetch: %v", err)
		}
		wantSys, _ := scrapers.SystemID("snes")
		want := searchCall{system: wantSys, term: domain.SearchTerm(info.Title)}
		if len(ss.searches) != 1 || ss.searches[0] != want {
			t.Errorf("searches = %+v, want [%+v]", ss.searches, want)
		}
	})

	for _, tc := range []struct{ name, seed string }{
		{"sem chave de seed", ""},
		{"chave sem sistema (switch2)", "switch2"},
		{"chave fora da tabela", "dreamcast-inexistente"},
	} {
		t.Run("VID-010/plataforma sem chave de seed ou sem sistema: permanente \"plataforma sem vídeo no ScreenScraper\", zero requisição ("+tc.name+")", func(t *testing.T) {
			t.Parallel()
			ss := good()
			rig := newResolverRig(t, store.VideoGameInfo{Title: "Super Metroid", SeedKey: tc.seed}, ss)
			_, _, err := rig.r.Fetch(t.Context(), videoJobOf(7))
			if !isPermanent(err, "plataforma sem vídeo no ScreenScraper") {
				t.Fatalf("err = %v, want permanent \"plataforma sem vídeo no ScreenScraper\"", err)
			}
			if ss.requestsCnt != 0 {
				t.Errorf("requests = %d, want 0", ss.requestsCnt)
			}
		})
	}

	t.Run("VID-010/jogo apagado: permanente", func(t *testing.T) {
		t.Parallel()
		ss := good()
		rig := newResolverRig(t, info, ss)
		_, _, err := rig.r.Fetch(t.Context(), videoJobOf(999))
		var pe *media.PermanentError
		if !errors.As(err, &pe) {
			t.Fatalf("err = %v, want *media.PermanentError", err)
		}
		if ss.requestsCnt != 0 {
			t.Errorf("requests = %d, want 0", ss.requestsCnt)
		}
	})

	t.Run("VID-010/erro de leitura do jogo que não é ErrGameNotFound não é permanente", func(t *testing.T) {
		t.Parallel()
		boom := errors.New("banco fora")
		r := newVideoResolver(vidGames{err: boom}, good(), &vidFiles{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
		_, _, err := r.Fetch(t.Context(), videoJobOf(7))
		var pe *media.PermanentError
		if err == nil || errors.As(err, &pe) {
			t.Fatalf("err = %v, want a transient error", err)
		}
	})

	t.Run("VID-010/nenhum arquivo do jogo é aberto e nenhum hash é enviado", func(t *testing.T) {
		t.Parallel()
		// The resolver has no file system dependency at all: the only inputs are the title, year and seed key
		// from videoGames. What is sent to the API is what the fake saw: a term and a system, nothing else.
		ss := good()
		rig := newResolverRig(t, info, ss)
		if _, _, err := rig.r.Fetch(t.Context(), videoJobOf(7)); err != nil {
			t.Fatalf("Fetch: %v", err)
		}
		if len(ss.searches) != 1 {
			t.Errorf("searches = %d, want 1", len(ss.searches))
		}
		for _, s := range ss.searches {
			for _, bad := range []string{"crc", "md5", "sha1", "romnom", ".exe", ".sfc"} {
				if strings.Contains(strings.ToLower(s.term), bad) {
					t.Errorf("term %q carries %q", s.term, bad)
				}
			}
		}
	})

	t.Run("VID-012/só video-normalized; o video original nunca é baixado", func(t *testing.T) {
		t.Parallel()
		g := ssGame("10", 4, nil, "Super Metroid")
		g.Medias = []scrapers.SSMedia{
			{Type: "video", Region: "wor", Format: "mp4", URL: "https://media.example.test/original.mp4"},
			{Type: "video-normalized", Region: "wor", Format: "mp4", URL: "https://media.example.test/norm.mp4"},
		}
		ss := &fakeSS{games: []scrapers.SSGame{g}, body: mp4Bytes(), openSize: 44}
		rig := newResolverRig(t, info, ss)
		if _, _, err := rig.r.Fetch(t.Context(), videoJobOf(7)); err != nil {
			t.Fatalf("Fetch: %v", err)
		}
		if len(ss.opened) != 1 || ss.opened[0] != "https://media.example.test/norm.mp4" {
			t.Errorf("opened = %v, want only video-normalized", ss.opened)
		}
	})

	t.Run("VID-012/ordem de região wor, us, eu, ss, jp, sem região, ordem da resposta", func(t *testing.T) {
		t.Parallel()
		mk := func(region, u string) scrapers.SSMedia {
			return scrapers.SSMedia{Type: "video-normalized", Region: region, Format: "mp4", URL: u}
		}
		all := []scrapers.SSMedia{mk("fr", "fr1"), mk("", "none"), mk("jp", "jp"), mk("ss", "ss"), mk("eu", "eu"), mk("us", "us"), mk("wor", "wor"), mk("fr", "fr2")}
		// Drop the head each round: the first element in priority order must win.
		for _, want := range []string{"wor", "us", "eu", "ss", "jp", "none", "fr1", "fr2"} {
			got, ok := pickVideoMedia(scrapers.SSGame{Medias: all})
			if !ok || got.URL != want {
				t.Fatalf("pickVideoMedia = %q (ok=%v), want %q", got.URL, ok, want)
			}
			next := all[:0:0]
			for _, m := range all {
				if m.URL != want {
					next = append(next, m)
				}
			}
			all = next
		}
		if _, ok := pickVideoMedia(scrapers.SSGame{Medias: all}); ok {
			t.Error("pickVideoMedia on an empty list = ok")
		}
	})

	t.Run("VID-012/format diferente de mp4 é pulado", func(t *testing.T) {
		t.Parallel()
		g := scrapers.SSGame{Medias: []scrapers.SSMedia{
			{Type: "video-normalized", Region: "wor", Format: "avi", URL: "avi"},
			{Type: "video-normalized", Region: "jp", Format: "mp4", URL: "mp4"},
			{Type: "video-normalized", Region: "us", Format: "", URL: "semformato"},
		}}
		got, ok := pickVideoMedia(g)
		if !ok || got.URL != "semformato" {
			t.Fatalf("pickVideoMedia = %q ok=%v, want the us one without format (format absent is accepted, avi skipped)", got.URL, ok)
		}
		g.Medias = g.Medias[:1]
		if _, ok := pickVideoMedia(g); ok {
			t.Error("only an avi: want no media")
		}
	})

	t.Run("VID-012/sem video-normalized: permanente \"sem vídeo no ScreenScraper\"", func(t *testing.T) {
		t.Parallel()
		g := ssGame("10", 4, nil, "Super Metroid")
		g.Medias = []scrapers.SSMedia{{Type: "video", Region: "wor", Format: "mp4", URL: "https://media.example.test/o.mp4"}}
		ss := &fakeSS{games: []scrapers.SSGame{g}, info: g}
		rig := newResolverRig(t, info, ss)
		_, _, err := rig.r.Fetch(t.Context(), videoJobOf(7))
		if !isPermanent(err, "sem vídeo no ScreenScraper") {
			t.Fatalf("err = %v, want permanent \"sem vídeo no ScreenScraper\"", err)
		}
		if len(ss.opened) != 0 {
			t.Errorf("opened = %v, want none", ss.opened)
		}
	})

	t.Run("VID-012/GameInfo só é chamado quando a busca não trouxe medias", func(t *testing.T) {
		t.Parallel()
		// Search with medias: no GameInfo.
		ss := good()
		rig := newResolverRig(t, info, ss)
		if _, _, err := rig.r.Fetch(t.Context(), videoJobOf(7)); err != nil {
			t.Fatalf("Fetch: %v", err)
		}
		if len(ss.infos) != 0 {
			t.Errorf("GameInfo called %v with medias in the search", ss.infos)
		}
		// Search without medias: GameInfo of the chosen game, and its medias are used.
		bare := ssGame("10", 4, nil, "Super Metroid")
		bare.Medias = nil
		full := ssGame("10", 4, nil, "Super Metroid")
		full.Medias = []scrapers.SSMedia{{Type: "video-normalized", Region: "us", Format: "mp4", URL: "https://media.example.test/full.mp4"}}
		ss2 := &fakeSS{games: []scrapers.SSGame{bare}, info: full, body: mp4Bytes(), openSize: 44}
		rig2 := newResolverRig(t, info, ss2)
		if _, _, err := rig2.r.Fetch(t.Context(), videoJobOf(7)); err != nil {
			t.Fatalf("Fetch: %v", err)
		}
		if len(ss2.infos) != 1 || ss2.infos[0] != "10" {
			t.Errorf("GameInfo calls = %v, want [10]", ss2.infos)
		}
		if len(ss2.opened) != 1 || ss2.opened[0] != "https://media.example.test/full.mp4" {
			t.Errorf("opened = %v, want the media of GameInfo", ss2.opened)
		}
	})

	t.Run("VID-013/size informado acima do teto: nada é aberto, permanente \"vídeo maior que 20 MB\"", func(t *testing.T) {
		t.Parallel()
		g := ssGame("10", 4, nil, "Super Metroid")
		g.Medias[0].Size = media.MaxVideoBytes + 1
		ss := &fakeSS{games: []scrapers.SSGame{g}, body: mp4Bytes()}
		rig := newResolverRig(t, info, ss)
		_, _, err := rig.r.Fetch(t.Context(), videoJobOf(7))
		if !isPermanent(err, "vídeo maior que 20 MB") {
			t.Fatalf("err = %v, want permanent \"vídeo maior que 20 MB\"", err)
		}
		if len(ss.opened) != 0 || rig.files.calls != 0 {
			t.Errorf("opened %v, FetchVideo %d; want nothing", ss.opened, rig.files.calls)
		}
	})

	t.Run("VID-013/Content-Length acima do teto: corpo não lido", func(t *testing.T) {
		t.Parallel()
		ss := &fakeSS{games: []scrapers.SSGame{ssGame("10", 4, nil, "Super Metroid")}, body: mp4Bytes(), openSize: media.MaxVideoBytes + 1}
		rig := newResolverRig(t, info, ss)
		_, _, err := rig.r.Fetch(t.Context(), videoJobOf(7))
		if !isPermanent(err, "vídeo maior que 20 MB") {
			t.Fatalf("err = %v, want permanent \"vídeo maior que 20 MB\"", err)
		}
		if ss.lastBody == nil || ss.lastBody.read {
			t.Errorf("body read = %v, want not read", ss.lastBody != nil && ss.lastBody.read)
		}
		if ss.lastBody != nil && !ss.lastBody.closed {
			t.Error("body not closed")
		}
		if rig.files.calls != 0 {
			t.Errorf("FetchVideo calls = %d, want 0", rig.files.calls)
		}
	})

	t.Run("VID-014/o corpo vai a FetchVideo e Stored.SourceURL é a URL da mídia sem credenciais", func(t *testing.T) {
		t.Parallel()
		g := ssGame("10", 4, nil, "Super Metroid")
		raw := "https://media.example.test/v/10.mp4?devid=" + secDevID + "&ssid=" + secUser + "&sspassword=" + secPass + "&x=1"
		g.Medias[0].URL = raw
		body := mp4Bytes()
		ss := &fakeSS{games: []scrapers.SSGame{g}, body: body, openSize: int64(len(body))}
		rig := newResolverRig(t, info, ss)
		stored, added, err := rig.r.Fetch(t.Context(), videoJobOf(7))
		if err != nil || !added {
			t.Fatalf("Fetch = %+v, %v, %v", stored, added, err)
		}
		if rig.files.calls != 1 || rig.files.gameID != 7 || !bytes.Equal(rig.files.body, body) || rig.files.size != int64(len(body)) {
			t.Errorf("FetchVideo got game %d size %d, %d bytes; want game 7 size %d and the body", rig.files.gameID, rig.files.size, len(rig.files.body), len(body))
		}
		if want := scrapers.RedactURL(raw); stored.SourceURL != want {
			t.Errorf("Stored.SourceURL = %q, want %q", stored.SourceURL, want)
		}
		if ss.lastBody == nil || !ss.lastBody.closed {
			t.Error("the video body was not closed")
		}
	})

	t.Run("VID-018/erro de FetchVideo permanente passa sem ser embrulhado em transitório", func(t *testing.T) {
		t.Parallel()
		ss := good()
		rig := newResolverRig(t, info, ss)
		rig.files.err = &media.PermanentError{Msg: "formato de vídeo não suportado"}
		_, _, err := rig.r.Fetch(t.Context(), videoJobOf(7))
		if !isPermanent(err, "formato de vídeo não suportado") {
			t.Fatalf("err = %v, want the permanent error of FetchVideo", err)
		}
	})

	t.Run("VID-016/erros de cota e credencial do cliente chegam intactos (errors.Is) e a busca de cancelamento do contexto também", func(t *testing.T) {
		t.Parallel()
		for _, want := range []error{scrapers.ErrQuota, scrapers.ErrCredentials, scrapers.ErrNoCredentials, scrapers.ErrUnavailable} {
			ss := good()
			ss.searchErr = fmt.Errorf("busca: %w", want)
			rig := newResolverRig(t, info, ss)
			_, _, err := rig.r.Fetch(t.Context(), videoJobOf(7))
			if !errors.Is(err, want) {
				t.Errorf("err = %v, want errors.Is %v", err, want)
			}
		}
		ss := good()
		ss.searchErr = context.Canceled
		rig := newResolverRig(t, info, ss)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if _, _, err := rig.r.Fetch(ctx, videoJobOf(7)); err == nil {
			t.Error("Fetch with a cancelled context: err = nil")
		}
	})

	t.Run("VID-016/Quota devolve a última leitura do cliente", func(t *testing.T) {
		t.Parallel()
		ss := good()
		ss.quota = scrapers.SSQuota{Known: true, RequestsToday: 3, MaxPerDay: 5}
		rig := newResolverRig(t, info, ss)
		if got := rig.r.Quota(); got != ss.quota {
			t.Errorf("Quota = %+v, want %+v", got, ss.quota)
		}
	})
}

// lockedBuf is a goroutine-safe bytes.Buffer for log capture.
type lockedBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuf) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

func (l *lockedBuf) count(sub string) int {
	return strings.Count(l.String(), sub)
}

func assertNoSecrets(t *testing.T, where, text string) {
	t.Helper()
	for _, s := range secrets {
		if strings.Contains(text, s) {
			t.Errorf("%s contains the secret %q: %s", where, s, text)
		}
	}
}

// TestVideoResolverNoSecrets uses the real client against httptest (no network) with marked credentials.
func TestVideoResolverNoSecrets(t *testing.T) {
	t.Parallel()

	type mode struct {
		name   string
		search func(w http.ResponseWriter, r *http.Request, base string)
		ok     bool
	}
	jsonGame := func(base string) string {
		media := base + "/media/v.mp4?devid=" + secDevID + "&devpassword=" + secDevPass + "&ssid=" + secUser + "&sspassword=" + secPass + "&mediaformat=mp4"
		return `{"response":{"ssuser":{"requeststoday":"1","maxrequestsperday":"20000"},"jeux":[{"id":"10","noms":[{"region":"wor","text":"Super Metroid"}],` +
			`"systeme":{"id":"4"},"medias":[{"type":"video-normalized","region":"wor","format":"mp4","url":"` + media + `"}]}]}}`
	}
	modes := []mode{
		{"sucesso", func(w http.ResponseWriter, r *http.Request, base string) {
			switch {
			case strings.HasPrefix(r.URL.Path, "/media/"):
				w.Header().Set("Content-Length", fmt.Sprint(len(mp4Bytes())))
				_, _ = w.Write(mp4Bytes())
			default:
				_, _ = io.WriteString(w, jsonGame(base))
			}
		}, true},
		{"404", func(w http.ResponseWriter, r *http.Request, _ string) { http.NotFound(w, r) }, false},
		{"403", func(w http.ResponseWriter, _ *http.Request, _ string) { w.WriteHeader(http.StatusForbidden) }, false},
		{"429", func(w http.ResponseWriter, _ *http.Request, _ string) { w.WriteHeader(http.StatusTooManyRequests) }, false},
		{"rede", nil, false},
	}
	for _, m := range modes {
		t.Run("VID-020/valores marcados de credencial não aparecem em erros, em Stored.SourceURL nem no log ("+m.name+")", func(t *testing.T) {
			t.Parallel()
			var srv *httptest.Server
			if m.search != nil {
				srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { m.search(w, r, "http://"+r.Host) }))
				defer srv.Close()
			} else {
				srv = httptest.NewServer(http.NotFoundHandler())
				srv.Close() // connection refused: a *url.Error carrying the full URL
			}
			logs := &lockedBuf{}
			log := slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
			clock := domain.NewFakeClock(time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC))
			client := scrapers.NewScreenScraper(scrapers.SSConfig{
				BaseURL: srv.URL, HTTP: srv.Client(), Limiter: scrapers.NewLimiter(clock, 0), Log: log,
				Creds: func() (scrapers.SSCreds, error) {
					return scrapers.SSCreds{DevID: secDevID, DevPassword: secDevPass, User: secUser, Password: secPass}, nil
				},
			})
			files := &vidFiles{}
			r := newVideoResolver(vidGames{info: map[int64]store.VideoGameInfo{7: {Title: "Super Metroid", SeedKey: "snes"}}}, client, files, log)
			stored, _, err := r.Fetch(t.Context(), videoJobOf(7))
			if m.ok {
				if err != nil {
					t.Fatalf("Fetch: %v", err)
				}
				if stored.SourceURL == "" {
					t.Error("Stored.SourceURL empty on success")
				}
			} else if err == nil {
				t.Fatal("Fetch: err = nil, want a failure")
			}
			if err != nil {
				assertNoSecrets(t, "error", err.Error())
			}
			assertNoSecrets(t, "Stored.SourceURL", stored.SourceURL)
			assertNoSecrets(t, "log", logs.String())
		})
	}
}
