package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strings"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/media"
	"github.com/arcadia-launcher/arcadia/internal/scrapers"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

// Permanent messages of the video lookup (screenscraper-rules §3..§5).
const (
	msgVideoPlatform = "plataforma sem vídeo no ScreenScraper"
	msgVideoNoMatch  = "sem correspondência no ScreenScraper"
	msgVideoNone     = "sem vídeo no ScreenScraper"
	msgVideoTooLarge = "vídeo maior que 20 MB"
	msgVideoGone     = "jogo não existe mais"
)

const ssProvider = "screenscraper"

// videoGames reads what the lookup needs about a game.
type videoGames interface {
	VideoGameInfo(ctx context.Context, gameID int64) (store.VideoGameInfo, error)
}

// ssAPI is the ScreenScraper client (scrapers.ScreenScraper).
type ssAPI interface {
	SearchGames(ctx context.Context, systemID int, term string) ([]scrapers.SSGame, scrapers.SSQuota, error)
	GameInfo(ctx context.Context, systemID int, gameID string) (scrapers.SSGame, scrapers.SSQuota, error)
	OpenVideo(ctx context.Context, mediaURL string) (io.ReadCloser, int64, error)
	Quota() scrapers.SSQuota
}

// videoFiles stores a downloaded video (media.Store).
type videoFiles interface {
	FetchVideo(ctx context.Context, gameID int64, r io.Reader, size int64) (media.Stored, bool, error)
}

// videoResolver turns a video job (a game id) into a stored video: search, match, pick, download.
// It opens no file of the game and sends no hash (VID-010).
type videoResolver struct {
	games videoGames
	ss    ssAPI
	files videoFiles
	log   *slog.Logger
}

func newVideoResolver(g videoGames, ss ssAPI, files videoFiles, log *slog.Logger) *videoResolver {
	return &videoResolver{games: g, ss: ss, files: files, log: log}
}

// Quota is the last quota reading of the ScreenScraper client.
func (r *videoResolver) Quota() scrapers.SSQuota { return r.ss.Quota() }

func permanent(msg string) error { return &media.PermanentError{Msg: msg} }

// Fetch resolves and downloads the video of job.GameID. Stored.SourceURL is the media URL without credentials.
func (r *videoResolver) Fetch(ctx context.Context, job store.MediaJob) (media.Stored, bool, error) {
	info, err := r.games.VideoGameInfo(ctx, job.GameID)
	if errors.Is(err, store.ErrGameNotFound) {
		return media.Stored{}, false, permanent(msgVideoGone)
	}
	if err != nil {
		return media.Stored{}, false, fmt.Errorf("vídeo: ler jogo %d: %w", job.GameID, err)
	}
	sysID, ok := scrapers.SystemID(info.SeedKey)
	if info.SeedKey == "" || !ok {
		return media.Stored{}, false, permanent(msgVideoPlatform)
	}
	term := domain.SearchTerm(info.Title)
	if term == "" {
		return media.Stored{}, false, permanent(msgVideoNoMatch)
	}

	games, _, err := r.ss.SearchGames(ctx, sysID, term)
	if err != nil {
		return media.Stored{}, false, r.apiError("buscar", err, msgVideoNoMatch)
	}
	best, conf := pickSSGame(term, info.Year, info.SeedKey, sysID, games)
	if conf != domain.ConfidenceMatch || best == nil {
		return media.Stored{}, false, permanent(msgVideoNoMatch)
	}

	medias := best.Medias
	if len(medias) == 0 {
		full, _, err := r.ss.GameInfo(ctx, sysID, best.ID)
		if err != nil {
			return media.Stored{}, false, r.apiError("detalhar", err, msgVideoNone)
		}
		medias = full.Medias
	}
	m, ok := pickVideoMedia(scrapers.SSGame{Medias: medias})
	if !ok {
		return media.Stored{}, false, permanent(msgVideoNone)
	}
	if m.Size > media.MaxVideoBytes {
		return media.Stored{}, false, permanent(msgVideoTooLarge)
	}

	body, size, err := r.ss.OpenVideo(ctx, m.URL)
	if err != nil {
		return media.Stored{}, false, r.apiError("baixar", err, msgVideoNone)
	}
	defer body.Close()
	if size > media.MaxVideoBytes {
		return media.Stored{}, false, permanent(msgVideoTooLarge)
	}
	stored, added, err := r.files.FetchVideo(ctx, job.GameID, body, size)
	if err != nil {
		var pe *media.PermanentError
		if errors.As(err, &pe) {
			return media.Stored{}, false, err
		}
		return media.Stored{}, false, fmt.Errorf("vídeo: gravar: %w", scrapers.RedactError(err))
	}
	stored.SourceURL = scrapers.RedactURL(m.URL)
	r.log.Debug("vídeo baixado", "game", job.GameID, "bytes", size)
	return stored, added, nil
}

// apiError classifies an error of the client: not-found and bad-request are permanent, the rest
// (quota, credentials, network) goes up with its identity and with the URLs redacted.
func (r *videoResolver) apiError(step string, err error, notFound string) error {
	err = scrapers.RedactError(err)
	switch {
	case errors.Is(err, scrapers.ErrNotFound):
		return permanent(notFound)
	case errors.Is(err, scrapers.ErrBadRequest):
		r.log.Warn("vídeo: pedido recusado pelo ScreenScraper", "step", step, "error", err.Error())
		return permanent(err.Error())
	}
	return fmt.Errorf("vídeo: %s: %w", step, err)
}

// pickSSGame scores each game of the answer as a group of its names and applies the match table
// (rules §3). Only ConfidenceMatch means best is the game.
func pickSSGame(term string, year int, seedKey string, systemID int, games []scrapers.SSGame) (*scrapers.SSGame, domain.Confidence) {
	type scored struct {
		idx int
		s   domain.Scored
	}
	var all []scored
	q := domain.Query{Title: term, Year: year, LibraryKey: seedKey}
	for i, g := range games {
		y := 0
		for _, gy := range g.Years {
			if gy > 0 && (y == 0 || gy < y) {
				y = gy
			}
		}
		var keys []string
		if g.SystemID == systemID {
			keys = []string{seedKey}
		}
		members := make([]domain.Member, 0, len(g.Names))
		for _, n := range g.Names {
			members = append(members, domain.Member{Provider: ssProvider, Title: n.Text, Year: y, PlatformKeys: keys, PlatformConfirmed: true})
		}
		if len(members) == 0 {
			continue
		}
		all = append(all, scored{i, domain.ScoreGroup(q, members)})
	}
	if len(all) == 0 {
		return nil, domain.ConfidenceNone
	}
	slices.SortStableFunc(all, func(a, b scored) int { return b.s.Score - a.s.Score })
	var second *domain.Scored
	if len(all) > 1 {
		second = &all[1].s
	}
	conf := domain.ClassifyMatch(&all[0].s, second)
	best := games[all[0].idx]
	return &best, conf
}

// pickVideoMedia chooses the video-normalized mp4 by region (rules §4).
func pickVideoMedia(g scrapers.SSGame) (scrapers.SSMedia, bool) {
	rank := func(region string) int {
		switch strings.ToLower(region) {
		case "wor":
			return 0
		case "us":
			return 1
		case "eu":
			return 2
		case "ss":
			return 3
		case "jp":
			return 4
		case "":
			return 5
		}
		return 6
	}
	var best scrapers.SSMedia
	bestRank, found := 0, false
	for _, m := range g.Medias {
		if m.Type != scrapers.SSVideoMediaType {
			continue
		}
		if m.Format != "" && !strings.EqualFold(m.Format, "mp4") {
			continue
		}
		if rk := rank(m.Region); !found || rk < bestRank {
			best, bestRank, found = m, rk, true
		}
	}
	return best, found
}
