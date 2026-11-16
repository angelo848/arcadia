package agent

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/feedback"
	"github.com/arcadia-launcher/arcadia/internal/input"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
	"github.com/arcadia-launcher/arcadia/internal/logging"
	"github.com/arcadia-launcher/arcadia/internal/media"
	"github.com/arcadia-launcher/arcadia/internal/paths"
	"github.com/arcadia-launcher/arcadia/internal/scrapers"
	"github.com/arcadia-launcher/arcadia/internal/store"
	"github.com/arcadia-launcher/arcadia/internal/tracker"
)

// platformParts contains the platform components created for the agent (T20).
type platformParts struct {
	Sources    []input.Source
	Power      powerStarter
	NewOverlay func() (feedback.Overlay, error)
	NewSound   func() (feedback.Sound, error)
}

// powerStarter registers the power callbacks and returns an io.Closer to stop watching.
type powerStarter func(onSuspend func(at time.Time), onWake func(at time.Time, userPresent bool)) (io.Closer, error)

// Options are the inputs of Run, provided by main.
type Options struct {
	Args    []string // os.Args[1:], may contain "--agent"
	Version string   // main.version; "dev" when not stamped
}

// config holds the agent inputs resolved by Run.
type config struct {
	DataDir  string // --data-dir
	LogLevel string // --log-level
	ExeDir   string // directory of the running executable
	Exe      string // path of the running executable (launched by "Abrir Arcadia")
	PipeName string // ipc.PipeName(sid); reported in the IPC-019 log record
	Version  string // reported by agent.status (AGT-004)

	// Launcher starts the games; nil => tracker.NewLauncher() (tests inject a fake).
	Launcher tracker.Launcher
	// Recover closes the sessions left open by a crash [SES-010];
	// nil => tracker.RecoverOpenSessions.
	Recover func(context.Context, tracker.RecoveryStore, domain.Clock, *slog.Logger) (int, error)

	// Platform creates platform-specific sources, power watcher and feedback [T20].
	Platform func(domain.Clock, *slog.Logger) platformParts

	// Providers builds a metadata provider by name; nil => newProviderFactory with the agent keys
	// and the HTTP client (tests inject blocking fakes).
	Providers func(name string) (scrapers.MetadataProvider, error)

	// ScreenScraperURL is the ScreenScraper API base URL (tests point it at httptest); empty = the real one.
	ScreenScraperURL string

	// KeyStore holds the provider credentials; nil => credentialStore{} (Windows Credential Manager).
	KeyStore scrapers.KeyStore
}

// parseFlags parses the agent command line (EXE-003). It accepts --agent (the
// mode selector), --data-dir and --log-level; any other flag is an error. Flag
// package output is discarded: Run reports the error itself.
func parseFlags(args []string) (config, error) {
	fs := flag.NewFlagSet("agent", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var cfg config
	_ = fs.Bool("agent", false, "run as the tray agent")
	fs.StringVar(&cfg.DataDir, "data-dir", "", "data directory (default: $ARCADIA_DATA_DIR or <exe dir>\\data)")
	fs.StringVar(&cfg.LogLevel, "log-level", "info", "log level: debug|info|warn|error")
	if err := fs.Parse(args); err != nil {
		return config{}, fmt.Errorf("parse flags: %w", err)
	}
	return cfg, nil
}

// ShutdownReason says what triggered the agent shutdown; it is sent to the
// clients as agent.shutdown params.reason (EXE-016).
type ShutdownReason string

// Shutdown reasons (strings of the IPC contract).
const (
	ReasonQuit       ShutdownReason = "quit"
	ReasonSessionEnd ShutdownReason = "session-end"
	ReasonSignal     ShutdownReason = "signal"
	ReasonOther      ShutdownReason = "other"
)

// shutdownDrainTimeout bounds the wait for the clients to close after
// agent.shutdown (EXE-014); it is the default of instance.drainTimeout.
const shutdownDrainTimeout = 500 * time.Millisecond

// spawnFunc starts a process without waiting for it (real: winapi.CreateDetached).
type spawnFunc func(exe string, args []string, flags uint32) error

// instance owns everything startAgent opened: store, IPC server and the log file.
type instance struct {
	paths  paths.Paths
	log    *slog.Logger
	info   ipc.AgentInfo
	store  *store.Store
	server *ipc.Server

	tracker  *tracker.Tracker
	notifier *sessionNotifier

	mediaNotifier *mediaNotifier

	// metadata runs metadata.search/apply and the batch [MET-059]; stopMetadata cancels its root
	// context at shutdown, before Wait. batchNotifier forwards the batch progress to the IPC server.
	metadata      *metadataService
	stopMetadata  context.CancelFunc
	batchNotifier *batchNotifier

	// onShutdownStep, when set (tests only), is called with each shutdown step in order.
	onShutdownStep func(step string)

	coord  *feedback.Coordinator
	router *triggerRouter

	closeNotifier *gameClosedNotifier
	sources       []input.Source
	power         io.Closer

	logCloser io.Closer

	drainTimeout time.Duration // limit of the drain in Shutdown (tests override it)

	// ipcFailed is closed (at most once) by the Serve goroutine when Serve
	// ended with an error not caused by Close or by ctx (EXE-026).
	ipcFailed chan struct{}

	wg sync.WaitGroup // the server.Serve and the media queue goroutines
	// stopQueue ends the media queue; Shutdown calls it after the IPC server closes (phase 7).
	stopQueue context.CancelFunc

	shutdownOnce sync.Once
	shutdownErr  error
	closeLogOnce sync.Once
	closeLogErr  error
}

// startAgent validates the log level, resolves and creates the data
// directory, opens agent.log, opens and migrates the store, recovers the open
// sessions, starts the tracker and then serves IPC on the listener returned by
// listen [AGT-009]. On failure everything it opened is
// closed before returning (no handle leaks).
func startAgent(ctx context.Context, cfg config, clock domain.Clock, getenv func(string) string,
	listen func() (net.Listener, error)) (*instance, error) {
	level, err := logging.ParseLevel(cfg.LogLevel)
	if err != nil {
		return nil, fmt.Errorf("start agent: %w", err)
	}

	p, err := paths.Resolve(paths.Options{Flag: cfg.DataDir, ExeDir: cfg.ExeDir, Getenv: getenv})
	if err != nil {
		return nil, fmt.Errorf("start agent: %w", err)
	}
	if err := p.EnsureDirs(); err != nil {
		return nil, fmt.Errorf("start agent: %w", err)
	}

	log, logCloser, err := logging.New(logging.Options{Dir: p.Logs, File: "agent.log", Level: level})
	if err != nil {
		return nil, fmt.Errorf("start agent: %w", err)
	}

	st, err := store.Open(ctx, p.DB, store.OpenOptions{Migrate: true})
	if err != nil {
		log.Error("falha ao abrir o banco", "path", p.DB, "error", err.Error())
		return nil, errors.Join(fmt.Errorf("start agent: %w", err), closeWrap("log", logCloser))
	}

	// Recovery runs before the tracker and the IPC server [AGT-009, SES-010];
	// its failure is logged and the startup goes on.
	recoverFn := cfg.Recover
	if recoverFn == nil {
		recoverFn = tracker.RecoverOpenSessions
	}
	if _, err := recoverFn(ctx, st, clock, log); err != nil {
		log.Error("falha ao recuperar as sessões abertas", "error", err.Error())
	}

	launcher := cfg.Launcher
	if launcher == nil {
		launcher = tracker.NewLauncher()
	}

	var parts platformParts
	if cfg.Platform != nil {
		parts = cfg.Platform(clock, log)
	}

	var ov feedback.Overlay = feedback.NopOverlay{}
	if parts.NewOverlay != nil {
		o, err := parts.NewOverlay()
		if err != nil {
			log.Warn("aviso na tela indisponível", "error", err.Error())
		} else if o != nil {
			ov = o
		}
	}

	var snd feedback.Sound = feedback.NopSound{}
	if parts.NewSound != nil {
		s, err := parts.NewSound()
		if err != nil {
			log.Warn("som de feedback indisponível", "error", err.Error())
		} else if s != nil {
			snd = s
		}
	}

	coord := feedback.New(ov, snd, clock, log)
	coord.Start()

	notifier := &sessionNotifier{}
	tr := tracker.New(tracker.Config{
		Clock:    clock,
		Store:    st,
		Launcher: launcher,
		Notify:   notifier.Notify,
		Feedback: func(k tracker.FeedbackKind) {
			coord.Send(feedback.Event(k))
		},
		Log:          log,
		Processes:    defaultProcessLister(),
		SteamRunning: defaultSteamRunning(),
	})
	tr.Start()
	// closeTracker stops the tracker and feedback on a failed startup (no session is open
	// yet, so it returns at once); it ignores the cancellation of ctx so the
	// owner goroutine is always waited for.
	closeTracker := func() error {
		var errs []error
		if err := tr.Close(context.WithoutCancel(ctx)); err != nil {
			errs = append(errs, fmt.Errorf("close tracker: %w", err))
		}
		if err := coord.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close feedback: %w", err))
		}
		return errors.Join(errs...)
	}

	ln, err := listen()
	if err != nil {
		if isPipeNameTaken(err) {
			log.Error("pipe já existe: outra instância ou outro processo", "pipe", cfg.PipeName, "error", err.Error())
		} else {
			log.Error("falha ao abrir o pipe", "pipe", cfg.PipeName, "error", err.Error())
		}
		return nil, errors.Join(fmt.Errorf("start agent: listen: %w", err), closeTracker(),
			closeWrap("store", st), closeWrap("log", logCloser))
	}

	startedAt := clock.Now()
	info := ipc.AgentInfo{
		Version:   cfg.Version,
		PID:       os.Getpid(),
		StartedAt: startedAt,
		DataDir:   p.Root,
	}
	handlers := ipc.AgentHandlers(info, clock)
	maps.Copy(handlers, emulatorHandlers(st, log))
	// Metadata and media (phase 7): the keys live in the Credential Manager and the images in
	// data\media. The queue runs in its own goroutine, below, and is waited for at shutdown.
	httpClient := &http.Client{Timeout: mediaHTTPTimeout}
	mediaRoot := filepath.Join(p.Root, "media")
	mediaFiles := media.New(mediaRoot, httpClient)
	maps.Copy(handlers, platformHandlers(newPlatformService(st, media.New(mediaRoot, httpClient), mediaRoot, os.Stat, log)))
	var keyStore scrapers.KeyStore = credentialStore{}
	if cfg.KeyStore != nil {
		keyStore = cfg.KeyStore
	}
	keys := scrapers.NewKeys(keyStore)
	mediaNotes := &mediaNotifier{}
	// Video (phase 13): the ScreenScraper client reads the credentials on every request (VID-017).
	ssClient := scrapers.NewScreenScraper(scrapers.SSConfig{
		BaseURL: cfg.ScreenScraperURL,
		HTTP:    httpClient,
		Limiter: scrapers.NewLimiter(clock, scrapers.IntervalScreenScraper),
		Log:     log,
		Creds: func() (scrapers.SSCreds, error) {
			devID, devPass, err := keys.Pair("screenscraperdev")
			if err != nil {
				return scrapers.SSCreds{}, scrapers.ErrNoCredentials
			}
			user, pass, err := keys.Pair("screenscraper")
			if err != nil {
				return scrapers.SSCreds{}, scrapers.ErrNoCredentials
			}
			return scrapers.SSCreds{DevID: devID, DevPassword: devPass, User: user, Password: pass}, nil
		},
	})
	videoResolver := newVideoResolver(st, ssClient, media.New(mediaRoot, httpClient), log)
	queue := newMediaQueue(st, fetchRouter{images: imageFetcher{files: media.New(mediaRoot, httpClient)}, video: videoResolver}, clock, log, mediaNotes)
	queue.ConfigureVideo(func() bool {
		_, _, errDev := keys.Pair("screenscraperdev")
		_, _, errUser := keys.Pair("screenscraper")
		return errDev == nil && errUser == nil
	}, videoResolver.Quota)
	// game.delete cleans the media of the game it removes (DEL-007, DEL-008).
	maps.Copy(handlers, gameHandlersWithMedia(st, tr, clock, os.Stat, log, gameMediaOf{q: queue, files: mediaFiles}))
	images := imageImporter{files: media.New(mediaRoot, httpClient), st: st, clock: clock, log: log}
	maps.Copy(handlers, importHandlers(st, defaultImportLocations(), clock, importSourceSet(), images, log))
	// Statistics (phase 8): read only; the local zone gives the weeks (EST-002, EST-006).
	maps.Copy(handlers, statsHandlers(st, clock, time.Local, log))
	providers := cfg.Providers
	if providers == nil {
		providers = newProviderFactory(keys, httpClient, clock)
	}
	scrapeSvc := &scrapeService{
		st:        st,
		keys:      keys,
		providers: providers,
		queue:     queue,
		mediaRoot: mediaRoot,
		clock:     clock,
		log:       log,
	}
	scrapeSvc.videoCredsChanged = queue.VideoCredentialsChanged
	maps.Copy(handlers, scrapeHandlers(scrapeSvc))
	maps.Copy(handlers, manualEditHandlers(manualEdit{st: st, files: mediaFiles, notes: mediaNotes, clock: clock, log: log}))
	// Unified metadata (phase 12): jobs are children of metaCtx, cancelled at shutdown (MET-059).
	metaCtx, stopMetadata := context.WithCancel(ctx)
	metadata := newMetadataService(scrapeSvc, st, metaCtx)
	batchNotes := &batchNotifier{}
	metadata.notify = batchNotes
	videoSvc := &videoService{st: st, keys: keys, q: queue, quota: videoResolver.Quota, clock: clock, log: log}
	metadata.video = videoSvc
	maps.Copy(handlers, videoHandlers(videoSvc))
	maps.Copy(handlers, metadataHandlers(metadata))
	server := ipc.NewServer(clock, handlers, log, ipc.WithStartedAt(startedAt))
	notifier.set(server)
	mediaNotes.set(server)
	batchNotes.set(server)
	closeNotes := &gameClosedNotifier{}
	closeNotes.set(server)

	triggers := make(chan input.Trigger, triggerQueueSize)
	router := newTriggerRouter(tr, triggers, log, coord, closeNotes)
	router.Start()

	var startedSources []input.Source
	for _, src := range parts.Sources {
		if err := src.Start(triggers); err != nil {
			log.Warn("fonte de gatilho de pausa indisponível", "source", src.Name(), "error", err.Error())
		} else {
			startedSources = append(startedSources, src)
		}
	}

	var powerCloser io.Closer
	if parts.Power != nil {
		bridge := &powerBridge{tr: tr, timeout: PowerHandlerTimeout, log: log}
		pCloser, err := parts.Power(bridge.OnSuspend, bridge.OnWake)
		if err != nil {
			log.Warn("monitor de energia indisponível", "error", err.Error())
		} else {
			powerCloser = pCloser
		}
	}

	a := &instance{
		paths:         p,
		log:           log,
		info:          info,
		store:         st,
		server:        server,
		tracker:       tr,
		notifier:      notifier,
		mediaNotifier: mediaNotes,
		metadata:      metadata,
		stopMetadata:  stopMetadata,
		batchNotifier: batchNotes,
		coord:         coord,
		router:        router,
		closeNotifier: closeNotes,
		sources:       startedSources,
		power:         powerCloser,
		logCloser:     logCloser,
		drainTimeout:  shutdownDrainTimeout,
		ipcFailed:     make(chan struct{}),
	}

	queueCtx, stopQueue := context.WithCancel(ctx)
	a.stopQueue = stopQueue
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		if err := queue.Run(queueCtx); err != nil {
			log.Error("fila de mídia encerrada com erro", "error", err.Error())
		}
	}()

	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		// This goroutine is the only one closing ipcFailed, and Serve returns once.
		if err := a.server.Serve(ctx, ln); serveFailed(err) {
			log.Error("IPC indisponível; encerrando o agente", "error", err.Error())
			close(a.ipcFailed)
		}
	}()

	// Orphan sweep [DEL-010]: runs in the background under the agent ctx (not the queue's, which
	// Shutdown cancels first: a sweep started just before it must still finish). Shutdown waits for it
	// through a.wg, before the store closes.
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		if err := mediaFiles.SweepOrphans(ctx, st.GameIDs); err != nil && !errors.Is(err, context.Canceled) {
			log.Error("varredura das pastas de mídia órfãs", "error", err.Error())
		}
	}()

	log.Info("agente iniciado", "version", info.Version, "pid", info.PID, "data_dir", p.Root)
	return a, nil
}

// serveFailed reports whether the error returned by Server.Serve means the IPC
// is unavailable (EXE-026): nil (Close) and context.Canceled (end of ctx) are
// a requested stop; anything else is a failure.
func serveFailed(err error) bool {
	return err != nil && !errors.Is(err, context.Canceled)
}

// exitCode is the process exit code after the shutdown: 1 if the shutdown
// failed or the agent ended because the IPC failed (EXE-026), else 0.
func exitCode(shutdownErr error, ipcFailed bool) int {
	if shutdownErr != nil || ipcFailed {
		return 1
	}
	return 0
}

// gameMediaOf is the gameMedia of the agent: the queue stops the downloads of a game and the media
// store removes its folder [DEL-008].
type gameMediaOf struct {
	q     *mediaQueue
	files *media.Store
}

// CancelGame stops and waits for the downloads of gameID (see mediaQueue.CancelGame).
func (m gameMediaOf) CancelGame(ctx context.Context, gameID int64) error {
	return m.q.CancelGame(ctx, gameID)
}

// RemoveGame deletes the folder of gameID (see media.Store.RemoveGame).
func (m gameMediaOf) RemoveGame(gameID int64) error {
	return m.files.RemoveGame(gameID)
}

// closeWrap closes c and wraps its error with what.
func closeWrap(what string, c io.Closer) error {
	if err := c.Close(); err != nil {
		return fmt.Errorf("close %s: %w", what, err)
	}
	return nil
}

// Info returns the identity reported by agent.status.
func (a *instance) Info() ipc.AgentInfo { return a.info }

// Logger returns the agent logger (valid until CloseLog).
func (a *instance) Logger() *slog.Logger { return a.log }

// IPCFailed returns a channel closed when Serve ended with an error not caused
// by Close or by ctx (EXE-026). It is the same channel for the whole life of
// the instance and a normal Shutdown never closes it.
func (a *instance) IPCFailed() <-chan struct{} { return a.ipcFailed }

// Shutdown notifies the clients with agent.shutdown{reason} and waits up to
// drainTimeout for them to close the connection (EXE-013, EXE-014), then
// closes the IPC server (and waits for its goroutines), the tracker (which
// ends the open session, SES-011) and the store [AGT-010]. A drain
// that times out only logs a Warn. It is idempotent (later calls, whatever the
// reason, return the first result) and does not close the log (see CloseLog).
// If ctx ends first, Shutdown still finishes closing but reports ctx's error.
func (a *instance) Shutdown(ctx context.Context, reason ShutdownReason) error {
	a.shutdownOnce.Do(func() {
		var errs []error

		// No game.launch starts a session from here on [AGT-011].
		a.tracker.BeginShutdown()

		// Close power and sources before drain [AGT-013].
		if a.power != nil {
			if err := a.power.Close(); err != nil {
				errs = append(errs, fmt.Errorf("shutdown: close power: %w", err))
			}
		}
		for _, src := range a.sources {
			if err := src.Close(); err != nil {
				errs = append(errs, fmt.Errorf("shutdown: close source: %w", err))
			}
		}
		if a.router != nil {
			a.router.Close()
		}
		a.step("router.close")

		dctx, cancel := context.WithTimeout(ctx, a.drainTimeout)
		err := a.server.Drain(dctx, ipc.MethodShutdown, ipc.ShutdownParams{Reason: string(reason)})
		cancel()
		if err != nil {
			attrs := []any{"error", err.Error()}
			var de *ipc.DrainError
			if errors.As(err, &de) {
				attrs = append(attrs, "pending", de.Pending)
			}
			a.log.Warn("clientes não fecharam a conexão a tempo", attrs...)
		}

		if err := a.server.Close(); err != nil {
			errs = append(errs, fmt.Errorf("shutdown: close ipc server: %w", err))
		}
		a.step("server.close")
		// The batch is cancelled and waited for before the queue and the store: its jobs
		// write the database and call queue.Notify [MET-059].
		a.stopMetadata()
		a.metadata.Wait()
		a.step("metadata.wait")
		a.step("queue.stop")
		a.stopQueue()
		a.wg.Wait()
		a.notifier.set(nil)
		a.mediaNotifier.set(nil)
		a.batchNotifier.set(nil)
		a.closeNotifier.set(nil)
		// After the server (no request can reach the tracker) and before the
		// store (the tracker writes the end of the session) [AGT-010, SES-011].
		a.step("tracker.close")
		if err := a.tracker.Close(ctx); err != nil {
			errs = append(errs, fmt.Errorf("shutdown: %w", err))
		}
		if a.coord != nil {
			if err := a.coord.Close(); err != nil {
				errs = append(errs, fmt.Errorf("shutdown: close feedback: %w", err))
			}
		}
		a.step("store.close")
		if err := a.store.Close(); err != nil {
			errs = append(errs, fmt.Errorf("shutdown: close store: %w", err))
		}
		if err := ctx.Err(); err != nil {
			errs = append(errs, fmt.Errorf("shutdown: %w", err))
		}
		a.shutdownErr = errors.Join(errs...)
	})
	return a.shutdownErr
}

// step reports a shutdown step to the test hook onShutdownStep, when it is set.
func (a *instance) step(name string) {
	if a.onShutdownStep != nil {
		a.onShutdownStep(name)
	}
}

// CloseLog closes the log file. It is idempotent and must be the last step of
// the shutdown: nothing may log after it (a write would reopen the file).
func (a *instance) CloseLog() error {
	a.closeLogOnce.Do(func() {
		a.closeLogErr = closeWrap("log", a.logCloser)
	})
	return a.closeLogErr
}
