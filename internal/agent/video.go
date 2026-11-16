package agent

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
	"github.com/arcadia-launcher/arcadia/internal/scrapers"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

// maxVideoQueueGames is the most games one video.queue accepts (x-video.maxQueueGames).
const maxVideoQueueGames = 1000

// videoStore is what the video service reads and writes.
type videoStore interface {
	VideoCandidates(ctx context.Context, gameIDs []int64) ([]store.VideoCandidate, error)
	VideoCandidatesIn(ctx context.Context, platformID int64) ([]store.VideoCandidate, error)
	EnqueueVideoJobs(ctx context.Context, gameIDs []int64, at time.Time) (int, error)
	VideoJobCounts(ctx context.Context) (store.VideoCounts, error)
	VideoGameInfo(ctx context.Context, gameID int64) (store.VideoGameInfo, error)
	PlatformByID(ctx context.Context, id int64) (store.Platform, error)
}

// videoQueue is the media queue as the video service sees it.
type videoQueue interface {
	Notify()
	CancelVideo(ctx context.Context) (int, error)
	VideoLaneStatus() (string, time.Time)
}

// videoService runs the video.* methods and the automatic search after metadata.apply [VID-004..VID-008].
type videoService struct {
	st    videoStore
	keys  *scrapers.Keys
	q     videoQueue
	quota func() scrapers.SSQuota
	clock domain.Clock
	log   *slog.Logger
}

// videoHandlers returns the handlers of the video methods.
func videoHandlers(v *videoService) map[string]ipc.Handler {
	return map[string]ipc.Handler{
		ipc.MethodVideoQueue:         v.queue,
		ipc.MethodVideoQueueMissing:  v.queueMissing,
		ipc.MethodVideoCancelPending: v.cancelPending,
		ipc.MethodVideoStatus:        v.status,
	}
}

// missingCredential names the first credential that is absent: the developer pair, then the user account.
func (v *videoService) missingCredential() (string, bool) {
	if _, _, err := v.keys.Pair(ipc.ProviderScreenScraperDev); err != nil {
		return ipc.ProviderScreenScraperDev, true
	}
	if _, _, err := v.keys.Pair(ipc.ProviderScreenScraper); err != nil {
		return ipc.ProviderScreenScraper, true
	}
	return "", false
}

func (v *videoService) noKeyError(provider string) error {
	return ipc.NewErrorData(ipc.CodeProviderNoKey, ipc.MsgProviderNoKey, map[string]string{"provider": provider})
}

func (v *videoService) internal(what string, err error) error {
	v.log.Error("vídeo: "+what, "error", err.Error())
	return ipc.NewError(ipc.CodeInternalError, ipc.MsgInternalError)
}

// mapped reports whether the platform seed key has a ScreenScraper system.
func videoMapped(seedKey string) bool {
	if seedKey == "" {
		return false
	}
	_, ok := scrapers.SystemID(seedKey)
	return ok
}

func (v *videoService) queue(ctx context.Context, params json.RawMessage) (any, error) {
	var p ipc.VideoQueueParams
	if rpcErr := decodeOrInvalid(params, &p); rpcErr != nil {
		return nil, rpcErr
	}
	if len(p.GameIDs) == 0 || len(p.GameIDs) > maxVideoQueueGames {
		return nil, ipc.NewError(ipc.CodeInvalidParams, ipc.MsgInvalidParams)
	}
	seen := make(map[int64]bool, len(p.GameIDs))
	for _, id := range p.GameIDs {
		if id < 1 || seen[id] {
			return nil, ipc.NewError(ipc.CodeInvalidParams, ipc.MsgInvalidParams)
		}
		seen[id] = true
	}
	cands, err := v.st.VideoCandidates(ctx, p.GameIDs)
	if err != nil {
		return nil, v.internal("ler jogos", err)
	}
	found := make(map[int64]store.VideoCandidate, len(cands))
	for _, c := range cands {
		found[c.GameID] = c
	}
	for _, id := range p.GameIDs {
		if _, ok := found[id]; !ok {
			return nil, ipc.NewErrorData(ipc.CodeGameNotFound, ipc.MsgGameNotFound, ipc.GameNotFoundData{GameID: id})
		}
	}
	if provider, missing := v.missingCredential(); missing {
		return nil, v.noKeyError(provider)
	}
	res := ipc.VideoQueueResult{Skipped: []ipc.VideoSkip{}}
	var eligible []int64
	for _, id := range p.GameIDs {
		c := found[id]
		switch {
		case !videoMapped(c.SeedKey):
			res.Skipped = append(res.Skipped, ipc.VideoSkip{GameID: id, Reason: ipc.VideoSkipPlatformUnmapped})
		case c.HasVideo:
			res.Skipped = append(res.Skipped, ipc.VideoSkip{GameID: id, Reason: ipc.VideoSkipHasVideo})
		default:
			eligible = append(eligible, id)
		}
	}
	if len(eligible) > 0 {
		n, err := v.st.EnqueueVideoJobs(ctx, eligible, v.clock.Now())
		if err != nil {
			return nil, v.internal("enfileirar", err)
		}
		res.Queued = n
		if n > 0 {
			v.q.Notify()
		}
	}
	return res, nil
}

func (v *videoService) queueMissing(ctx context.Context, params json.RawMessage) (any, error) {
	var p ipc.VideoQueueMissingParams
	if params != nil {
		if rpcErr := decodeOrInvalid(params, &p); rpcErr != nil {
			return nil, rpcErr
		}
	}
	if p.PlatformID < 0 {
		return nil, ipc.NewError(ipc.CodeInvalidParams, ipc.MsgInvalidParams)
	}
	if p.PlatformID > 0 {
		if _, err := v.st.PlatformByID(ctx, p.PlatformID); err != nil {
			if errors.Is(err, store.ErrPlatformNotFound) {
				return nil, ipc.NewError(ipc.CodeGameNotFound, ipc.MsgGameNotFound)
			}
			return nil, v.internal("ler plataforma", err)
		}
	}
	if provider, missing := v.missingCredential(); missing {
		return nil, v.noKeyError(provider)
	}
	cands, err := v.st.VideoCandidatesIn(ctx, p.PlatformID)
	if err != nil {
		return nil, v.internal("ler jogos", err)
	}
	var res ipc.VideoQueueMissingResult
	var eligible []int64
	for _, c := range cands {
		if !videoMapped(c.SeedKey) || c.HasVideo {
			res.Skipped++
			continue
		}
		eligible = append(eligible, c.GameID)
	}
	if len(eligible) > 0 {
		n, err := v.st.EnqueueVideoJobs(ctx, eligible, v.clock.Now())
		if err != nil {
			return nil, v.internal("enfileirar", err)
		}
		res.Queued = n
		res.Skipped += len(eligible) - n
		if n > 0 {
			v.q.Notify()
		}
	}
	return res, nil
}

func (v *videoService) cancelPending(ctx context.Context, _ json.RawMessage) (any, error) {
	n, err := v.q.CancelVideo(ctx)
	if err != nil {
		return nil, v.internal("cancelar", err)
	}
	return ipc.VideoCancelResult{Cancelled: n}, nil
}

func (v *videoService) status(ctx context.Context, _ json.RawMessage) (any, error) {
	counts, err := v.st.VideoJobCounts(ctx)
	if err != nil {
		return nil, v.internal("contar", err)
	}
	st := ipc.VideoStatus{Pending: counts.Pending, Running: counts.Running, Failed: counts.Failed}
	lane, until := v.q.VideoLaneStatus()
	if _, missing := v.missingCredential(); missing {
		lane = ipc.VideoLaneNoCredentials
	}
	st.Lane = lane
	if lane == ipc.VideoLanePausedQuota && !until.IsZero() {
		u := until
		st.PausedUntil = &u
	}
	if v.quota != nil {
		if q := v.quota(); q.Known {
			today, perDay := q.RequestsToday, q.MaxPerDay
			st.RequestsToday, st.MaxRequestsPerDay = &today, &perDay
		}
	}
	return ipc.VideoStatusResult{Status: st}, nil
}

// maybeQueue queues the video of a game just applied when every condition holds [VID-004]. It is silent:
// a missing condition is only a Debug record, and nothing is notified unless a job was created.
func (v *videoService) maybeQueue(ctx context.Context, gameID int64) {
	if provider, missing := v.missingCredential(); missing {
		v.log.Debug("vídeo automático: credencial ausente", "provider", provider, "gameId", gameID)
		return
	}
	cands, err := v.st.VideoCandidates(ctx, []int64{gameID})
	if err != nil || len(cands) == 0 {
		v.log.Debug("vídeo automático: jogo não lido", "gameId", gameID)
		return
	}
	c := cands[0]
	if !videoMapped(c.SeedKey) {
		v.log.Debug("vídeo automático: plataforma sem sistema no ScreenScraper", "gameId", gameID)
		return
	}
	if c.HasVideo {
		v.log.Debug("vídeo automático: o jogo já tem vídeo ou job ativo", "gameId", gameID)
		return
	}
	n, err := v.st.EnqueueVideoJobs(ctx, []int64{gameID}, v.clock.Now())
	if err != nil {
		v.log.Debug("vídeo automático: falha ao enfileirar", "gameId", gameID, "error", err.Error())
		return
	}
	if n > 0 {
		v.q.Notify()
	}
}
