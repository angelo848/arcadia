# Fase 4 — Pausa, retomada e "iniciar sem contar tempo": design

- **Status:** aprovado junto com [requirements.md](requirements.md) (2026-11-10)
- Implementa [requirements.md](requirements.md). Base: designs da [fase 3](../phase-3-tracker/design.md), da
  [fase 2](../phase-2-skeleton/design.md) e do [executável único](../single-exe/design.md); tudo o que não é citado aqui
  continua como lá. ADRs 0005, 0009, 0010, 0011, 0012, 0013, 0014, 0015, 0016 (e os propostos 0017..0019).
  Contratos: [ipc.openrpc.json](contracts/ipc.openrpc.json) (v0.5.0, substitui a v0.4.0),
  [session-state-machine.md](contracts/session-state-machine.md) (estende a da fase 3 com `Paused`).
- Os IDs entre colchetes indicam os requisitos atendidos. As escolhas marcadas "(Q1..Q4)" seguem as
  [Decisões do usuário (2026-11-09)](requirements.md#decisões-do-usuário-2026-11-09).
- **Quem escreve o quê (ADR 0007):** `internal/tracker/` e `cmd/fake-game/` são do tracker-specialist; o restante
  (`domain`, `store`, `input`, `feedback`, `winapi`, `ipc`, `agent`, `cmd/arcadia`, `frontend`) é do implementer.

## Visão geral

```
 atalho Ctrl+Alt+P ──► input.Hotkey ──┐                                     ┌─► feedback.Coordinator ─► Overlay (janela Win32)
 Select+Y (XInput) ──► input.Pad ─────┼─► chan Trigger ─► agent.pauseRouter │                         └► Sound (Beep)
                                      │      (debounce 300 ms)    │         │ Config.Feedback
 PBT_APM* ──► winapi.PowerWatcher ────┴──────────────────────────►│         │ (enfileira, não bloqueia)
                      OnSuspend/OnWake(at)                         ▼         │
                                                      tracker.Tracker (goroutine dona) ──► Notify ──► ipc.Server ──► UI
                                                             │  Machine (pura): Running ⇄ Paused → Grace
                                                             ▼
                                                  store: PauseSession / ResumePause / EndSession (pause_interval)

 UI: game.launchUntracked ──► Tracker.LaunchUntracked ──► Launcher.LaunchDetached (sem job, sem sessão)
```

Princípios: (1) a goroutine dona do tracker continua sendo a única que toca na máquina, no job e no store — pausa,
suspensão e retomada entram por canais, como `Launch`; (2) as fontes só produzem eventos com o instante já lido
(`At`), nunca decidem o estado; (3) o feedback é uma consequência de uma transição, enfileirada e sem retorno;
(4) a UI continua sem caminho de escrita de tempo — o botão do painel (Q4) só pede `session.pause`/`session.resume` ao
agente, que decide, grava e notifica; a UI só reage a `session.changed`.

## Layout de pacotes

```
internal/domain/
  session.go          # + SessionPaused, PauseReason, ParsePauseReason, UserInitiated, PauseToggleDebounce  [DOM-010, DOM-011]
internal/store/
  migrations/00004_pause_interval_open_uidx.sql                                                          [STO-018]
  queries/pauses.sql  (sqlc)  pauses.go                                                                  [STO-019..STO-023]
  sessions.go         # EndSession fecha a pausa aberta                                                  [STO-021]
internal/tracker/      # só o tracker-specialist
  machine.go          # + Paused, TogglePause, Suspend, Wake, efeitos PauseSession/ResumePause/Feedback   [SES-016..SES-031]
  tracker.go          # + TogglePause/Suspend/Wake/LaunchUntracked, Config.Feedback, canal de controle     [SES-016..SES-034]
  launcher.go         # + Launcher.LaunchDetached; SessionStore + PauseSession/ResumePause
  job_windows.go      # + LaunchDetached (CreateProcess sem job)                                           [TRK-010, TRK-011]
  job_other.go        # LaunchDetached → errors.ErrUnsupported
internal/input/       # NOVO — fontes de gatilho de pausa
  input.go            # Trigger, Source, constantes e padrões (portável)
  combo.go            # detector do combo do controle, puro                                                [PAD-001..PAD-005]
  pad.go              # Pad: poller sobre PadAPI + Clock (portável)                                        [PAD-006, PAD-008]
  xinput_windows.go   # PadAPI real (xinput1_4.dll)                                                        [PAD-007]
  hotkey_windows.go   # Hotkey: RegisterHotKey + loop de mensagens                                         [HOT-001..HOT-004]
  hotkey_other.go     # Source que falha com errors.ErrUnsupported
internal/feedback/    # NOVO — coordenação do aviso e do som (portável)
  feedback.go         # Coordinator, Overlay, Sound, Notice, Cue                                           [FBK-001..FBK-005, OVL-001, OVL-003]
internal/winapi/
  overlay_windows.go  # janela de aviso Win32 (implementa feedback.Overlay)                                [OVL-002, OVL-004..OVL-006]
  overlay_geometry.go # overlayRect puro                                                                   [OVL-004]
  beep_windows.go     # Beeper: goroutine + Beep (implementa feedback.Sound)                              [SND-001, SND-002]
  power_windows.go    # PowerWatcher: janela oculta + WM_POWERBROADCAST                                    [PWR-001..PWR-006]
internal/ipc/
  protocol.go         # + MethodGameLaunchUntracked, LaunchedProcess, GameLaunchUntrackedParams            [IPC-032..IPC-037]
internal/agent/
  pause.go            # pauseRouter, powerBridge                                                           [PAU-001..PAU-006]
  games.go            # + handler game.launchUntracked; sessionTracker.LaunchUntracked                      [IPC-033..IPC-035]
  app.go run_windows.go  # subida/shutdown com as fontes e o feedback                                      [AGT-012, AGT-013]
cmd/arcadia/
  games.go            # + LaunchGameUntracked, PauseSession, ResumeSession; SessionView aceita paused       [DSK-013..DSK-015]
frontend/src/
  components/GameList.tsx  SessionPanel.tsx  App.tsx
  lib/games.ts  lib/sessionClock.ts                                                                       [UI-019..UI-025]
```

Regra do CLAUDE.md respeitada: `domain`, `store`, `input/{input,combo,pad}.go`, `feedback`, `machine.go`, `tracker.go`,
`launcher.go` e `recover.go` não importam nada de Windows; Win32 só em `*_windows.go`. Os `*_other.go` mantêm
`CGO_ENABLED=0 GOOS=linux go vet ./...` compilando.

## `internal/domain` [DOM-010, DOM-011]

```go
const SessionPaused SessionState = "paused"                    // ParseSessionState passa a aceitá-lo

type PauseReason string
const (
    PauseHotkey  PauseReason = "hotkey"
    PauseGamepad PauseReason = "gamepad"
    PauseUI      PauseReason = "ui"       // session.pause from the panel button; no migration (reason is free TEXT)
    PauseSleep   PauseReason = "sleep"
    // "idle" and "minimized" are reserved (ARCHITECTURE §5); adding them is a domain-only change (ADR 0012).
)
func ParsePauseReason(s string) (PauseReason, error)           // exact match, like ParseSessionState
func (r PauseReason) UserInitiated() bool                      // hotkey, gamepad, ui

// PauseToggleDebounce is the minimum spacing between two forwarded pause triggers [PAU-004].
const PauseToggleDebounce = 300 * time.Millisecond
```

- `SessionSnapshot` não muda: o estado `paused` basta; o motivo da pausa não vai para a UI.
- `ActiveDuration` (DOM-001..004, fase 2) deixa de ser "reservado para a fase 4": o tracker acumula em memória (não o
  usa), mas STO-023 o usa como verificação cruzada com o que está gravado.

## `internal/store` [STO-018..STO-023]

### Migração 4 [STO-018]
```sql
-- +goose Up
CREATE UNIQUE INDEX pause_interval_open_uidx ON pause_interval (session_id) WHERE resumed_at IS NULL;
-- +goose Down
DROP INDEX pause_interval_open_uidx;
```
`store.SchemaVersion = 4`. A tabela `pause_interval` e o índice `pause_interval_session_id_idx` vêm da migração 1 e não
mudam; ninguém escrevia nela, então criar o índice único é seguro. O índice faz o banco garantir "no máximo uma pausa
aberta por sessão" (invariante usada por `ResumePause` e pelo `EndSession`, que fecham "a" pausa aberta sem precisar de
id). Nenhuma coluna nova: `reason` continua `TEXT` sem `CHECK` (ADR 0012).

### API
```go
var (
    ErrPauseOpen = errors.New("store: pause already open")
)
type PauseStart struct {
    SessionID int64; At time.Time; Reason domain.PauseReason; Active time.Duration
}
type PauseEnd struct {
    SessionID int64; At time.Time; State domain.SessionState // running | grace
    Active time.Duration
}
type PauseInterval struct {
    ID, SessionID      int64
    PausedAt, ResumedAt time.Time // ResumedAt zero = open
    Reason              domain.PauseReason
}

func (s *Store) PauseSession(ctx context.Context, p PauseStart) error                    // [STO-019]
func (s *Store) ResumePause(ctx context.Context, p PauseEnd) error                       // [STO-020]
func (s *Store) Pauses(ctx context.Context, sessionID int64) ([]PauseInterval, error)    // [STO-022]
```
- `PauseSession` (`Store.Tx`): `UPDATE play_session SET state = 'paused', last_seen_at = ?, active_seconds = ? WHERE id =
  ? AND ended_at IS NULL` (0 linhas → `ErrSessionNotOpen`) → `INSERT INTO pause_interval(session_id, paused_at,
  resumed_at, reason) VALUES (?, ?, NULL, ?)`. Violação do índice único (`SQLITE_CONSTRAINT_UNIQUE`) → `ErrPauseOpen`.
  A transação inteira desfaz em qualquer erro.
- `ResumePause` (`Store.Tx`): lê o intervalo aberto; se existir e `resumed_at == paused_at` em ms → `DELETE`, senão
  `UPDATE pause_interval SET resumed_at = ?`; depois `UPDATE play_session SET state, last_seen_at, active_seconds WHERE
  id = ? AND ended_at IS NULL` (0 linhas → `ErrSessionNotOpen`). Sem intervalo aberto: só a sessão é atualizada,
  sem erro (idempotente; cobre a falha de escrita de SES-014).
- `EndSession` ganha, antes do `CloseSession`, duas queries na mesma transação: `ClosePauseAt` (`UPDATE pause_interval
  SET resumed_at = :ended_at WHERE session_id = ? AND resumed_at IS NULL AND paused_at < :ended_at`) e `DeleteOpenPause`
  (`DELETE FROM pause_interval WHERE session_id = ? AND resumed_at IS NULL`, que só sobra quando `paused_at ≥
  ended_at`) [STO-021]. O resto de `EndSession` (STO-014/STO-015) não muda.
- `Pauses`: `ORDER BY paused_at, id`; slice sempre não nulo. `reason` lido sem validar (leitor tolerante) [STO-022].
- `OpenSessions` não muda: a recuperação (SES-025) não precisa saber da pausa; o `EndSession` cuida dela.
- `UpdateSession` aceita `domain.SessionPaused` (é só texto, validado no `domain`) [SES-020].
- `sqlc generate` depois de criar `queries/pauses.sql`; `dbgen` é regenerado.
- Testes de atomicidade (STO-021): trigger de teste `BEFORE UPDATE ON pause_interval … RAISE(ABORT)` criado pelo
  teste; confere que a sessão continua aberta, como em STO-014.

## `internal/tracker` [SES-016..SES-034, TRK-009..TRK-011]

### Interfaces (acréscimos em `launcher.go`)
```go
type Launcher interface {
    Prepare(spec LaunchSpec) (Job, error)
    // LaunchDetached creates the process with no Job Object and without CREATE_SUSPENDED, closes its process and
    // thread handles before returning, and reports the PID. On error nothing was left running or open.
    LaunchDetached(spec LaunchSpec) (pid uint32, err error)
}
type SessionStore interface {        // real: *store.Store
    StartSession(...); UpdateSession(...); EndSession(...); DeleteSession(...)   // como na fase 3
    PauseSession(ctx context.Context, p store.PauseStart) error                    // [STO-019]
    ResumePause(ctx context.Context, p store.PauseEnd) error                       // [STO-020]
}
type FeedbackKind int
const ( FeedbackPaused FeedbackKind = iota + 1; FeedbackResumed )
```

### Máquina pura (`machine.go`) [SES-016..SES-031, FBK-001]
Implementa exatamente a tabela de [session-state-machine.md](contracts/session-state-machine.md). Acréscimos (o
tracker-specialist pode ajustar assinaturas, não o comportamento):
```go
const StatePaused State = ... // entre StateRunning e StateGrace na enumeração; Machine ganha pausedAt, pauseReason
func (m *Machine) TogglePause(now time.Time, reason domain.PauseReason) []Effect   // Running↔Paused; ignora Idle/Launching/Grace e now < startedAt
func (m *Machine) Pause(now time.Time) []Effect                                     // Running→Paused(ui); Paused no-op; Idle/Grace → caller maps to NotPausable
func (m *Machine) Resume(now time.Time) []Effect                                    // Paused→Running; Running no-op
func (m *Machine) Suspend(now time.Time) []Effect                                   // Running → Paused(sleep); resto ignorado
func (m *Machine) Wake(now time.Time, userPresent bool) []Effect                    // Paused(sleep) + userPresent → Running
// novos EffectKind: EffPauseSession, EffResumePause, EffFeedback (State/At/Active/Reason como nos demais)
```
- `Poll` passa a tratar `StatePaused` como `StateRunning` para o caso `ativos == 0` (→ `TrackedEmpty`) e a rearmar
  a verificação [SES-022]; `Heartbeat` aceita `StatePaused` e grava `paused` [SES-020]; `TrackedEmpty` aceita
  `StatePaused` e emite `EffResumePause{State: grace}` em vez do `EffUpdateSession` [SES-021]; `ProcessAdded` em
  Paused é ignorado; `Stop` em Paused emite `EffEndSession{At: now, Reason: EndAgentShutdown}` [SES-024].
- `activeAt` devolve `accumulated` em Paused (como em Grace); `Snapshot` devolve `domain.SessionPaused` em Paused
  [IPC-036].
- `EffFeedback` é emitido pela própria máquina conforme a tabela de FBK-001 (pausa `UserInitiated`; toda retomada), de
  modo que a regra é testável sem goroutines.
- Testes table-driven só com `time.Time` fixos, uma linha da tabela por caso, IDs SES-*.

### Coordenador (`tracker.go`) [SES-016..SES-034, FBK-003, PWR-005]
```go
type Config struct {
    // ... campos da fase 3 ...
    Feedback func(FeedbackKind) // must not block (real: feedback.Coordinator.Send); nil = no-op
}
type PauseResult int
const ( PauseIgnored PauseResult = iota; PausePaused; PauseResumed )

var ErrInvalidPauseReason = errors.New("tracker: invalid pause reason") // TogglePause only takes hotkey/gamepad

func (t *Tracker) TogglePause(ctx context.Context, reason domain.PauseReason, at time.Time) (PauseResult, error)
// Explicit, idempotent requests from session.pause / session.resume [SES-035..SES-037, IPC-038..IPC-040].
// They return the session measured at the handling instant, or *NotPausableError{Reason: "no-session"|"grace"}.
func (t *Tracker) Pause(ctx context.Context, at time.Time) (domain.SessionSnapshot, error)   // reason ui
func (t *Tracker) Resume(ctx context.Context, at time.Time) (domain.SessionSnapshot, error)
func (t *Tracker) Suspend(ctx context.Context, at time.Time) error
func (t *Tracker) Wake(ctx context.Context, at time.Time, userPresent bool) error
func (t *Tracker) LaunchUntracked(ctx context.Context, spec LaunchSpec) (pid uint32, err error)
```
- **Canal de controle.** Um segundo canal `ctrlCh chan ctrlReq` (sem buffer, resposta em canal de 1), lido pelo
  `select` da goroutine dona ao lado de `launchCh`. `TogglePause`, `Suspend`, `Wake` e `LaunchUntracked` enviam o
  pedido e esperam a resposta ou o `ctx` (como `Launch`); quando o `ctx` do chamador acaba, a goroutine dona ainda
  aplica o evento com o instante `at` original [PWR-005]. Depois de `BeginShutdown`/`Close` devolvem `ErrShuttingDown`.
- **Instante.** O instante aplicado é `monotonic(at)` (máximo com o último aplicado), para `last_seen_at` e `pausedAt`
  nunca andarem para trás se a goroutine estava ocupada [SES-016..SES-028]. `TogglePause` com motivo diferente de
  `hotkey`/`gamepad` devolve `ErrInvalidPauseReason` (o motivo `sleep` só nasce de `Suspend`).
- **Efeitos** (`exec`): `EffPauseSession` → `store.PauseSession` com `writeTimeout` (falha → `writeFailed`, o estado em
  memória avança, SES-014); `EffResumePause` → `store.ResumePause`; `EffNotify` como hoje; `EffFeedback` →
  `cfg.Feedback(kind)` depois do `Notify` [FBK-003]. Heartbeat e verificação não são rearmados na pausa nem na retomada
  (os mesmos temporizadores seguem; ver a nota de waiters do contrato).
- **`LaunchUntracked`**: executado na goroutine dona (para compartilhar a recusa com `Launch`): se
  `isShuttingDown()` → `ErrShuttingDown`; se o estado não é Idle → `*SessionActiveError` com o snapshot da sessão ou o
  `Pending()`; senão `launcher.LaunchDetached(spec)` → `(pid, nil)`, ou log `Error` "falha ao iniciar o jogo" (`game_id`,
  `path`, `error`) e `*LaunchError` [SES-032..SES-034]. Não toca na máquina, no job nem no store. Enquanto roda (alguns ms)
  o estado continua Idle; um `Launch` rastreado que chegue em seguida só é lido depois (mesma goroutine) [SES-032].
- **`Current`/`view`**: o snapshot publicado já cobre Paused (a cópia da `Machine` inclui `pausedAt`) [IPC-036].
- **`Close` em Paused**: a goroutine dona aplica `Machine.Stop`, o store fecha a pausa na mesma transação do fim
  [SES-024, STO-021]; `Close` continua esperando a dona (TRK-008).

### Job Object no Windows (`job_windows.go`) [TRK-009..TRK-011]
- O `Job` e a leitora de IOCP **não mudam**: pausar não consulta nem altera o job além do `ActiveProcesses` periódico
  [TRK-009].
- `LaunchDetached(spec)`: `CreateProcess(appName = Exe, cmdLine = quoteExe(Exe) + args, inherit = false, flags =
  CREATE_DEFAULT_ERROR_MODE, cwd = WorkDir ou dir(Exe))` — as mesmas regras de TRK-005; sem `CREATE_SUSPENDED`, sem
  `AssignProcessToJobObject`. Fecha `pi.Process` e `pi.Thread` com `defer windows.CloseHandle` logo após o erro
  checado, e devolve `pi.ProcessId`. O processo é filho do agente, mas sem job: fechar o agente não o mata [TRK-010,
  TRK-011].
- Teste de "fora de job" (TRK-010): `IsProcessInJob(child, 0)` comparado com o mesmo valor para o processo de teste (se
  o próprio teste estiver num job de CI, só se confere que o resultado do filho é igual ao do pai).

## `internal/input` — atalho e controle [HOT-001..HOT-005, PAD-001..PAD-009]

```go
type Trigger struct {
    Source domain.PauseReason // PauseHotkey or PauseGamepad
    At     time.Time          // clock reading taken when the event was seen
}
// Source is started with the shared output queue and is closed exactly once by its owner.
type Source interface {
    Start(out chan<- Trigger) error // error = unavailable; nothing is left running
    Close() error                   // idempotent; waits for the goroutine
}
```

### Atalho de teclado [HOT-001..HOT-004]
- `Hotkey{Modifiers, VK}`; `DefaultHotkey = Hotkey{ModControl|ModAlt, 'P'}` (`input.go`, portável). `NewHotkey(h, clock,
  log) Source` em `hotkey_windows.go`.
- **Uma goroutine, uma thread.** `Start` cria a goroutine `loop`, que faz `runtime.LockOSThread()`, chama
  `PeekMessageW(PM_NOREMOVE)` (garante a fila da thread), `RegisterHotKey(0, id = 1, mods|MOD_NOREPEAT, vk)` e responde
  em `ready` com o `windows.GetCurrentThreadId()` ou o erro (que embrulha o `Errno`, p. ex. 1409) [HOT-001, HOT-003].
  Registrar com `hWnd = 0` associa o atalho à fila da thread: não precisa de janela.
- **Laço:** `GetMessageW`; em `WM_HOTKEY (0x0312)` com `wParam == id` lê `clock.Now()` e faz um envio não bloqueante
  (`select`/`default`) em `out`; cheio → `Warn` "gatilho de pausa descartado: fila cheia" [HOT-002]. `WM_QUIT` sai do laço.
- **Parada:** `Close` faz `PostThreadMessageW(tid, WM_QUIT, 0, 0)` e espera `wg`; antes de sair, a própria goroutine
  chama `UnregisterHotKey(0, id)` (a mesma thread que registrou) [HOT-004]. Se o `Post` falhar, devolve o erro sem
  esperar (como o `SessionEndWatcher.stopLoop`). Idempotente por `sync.Once`.
- Teste de integração: opções com combinação de teste (p. ex. `Ctrl+Alt+Shift+F24`) para não colidir com o agente real
  do desenvolvedor; o `WM_HOTKEY` é injetado com `PostThreadMessageW(tid, WM_HOTKEY, id, lParam)`; um segundo listener
  com a mesma combinação cobre `ERROR_HOTKEY_ALREADY_REGISTERED` [HOT-003].
- Por que não `SetWindowsHookEx`/`GetAsyncKeyState`: hook de teclado global é intrusivo (antivírus, anti-cheat) e
  `RegisterHotKey` não pede privilégio nem DLL injetada.

### Controle XInput [PAD-001..PAD-008]
```go
type PadState struct{ Buttons uint16 }
type PadAPI interface { State(slot int) (PadState, error) } // error wrapping ErrDisconnected for ERROR_DEVICE_NOT_CONNECTED
type PadConfig struct{ Hold, Poll, Rescan time.Duration; Mask uint16 } // zero => 1s, 16ms, 1s, Back|Y
func NewPad(api PadAPI, cfg PadConfig, clock domain.Clock, log *slog.Logger) Source
func NewXInput() (PadAPI, error) // xinput_windows.go: xinput1_4.dll via windows.NewLazySystemDLL; error if it does not load [PAD-007]
```
- **Detector puro** (`combo.go`): `type comboDetector struct{ since [4]time.Time; fired [4]bool }` com
  `Observe(slot int, buttons uint16, connected bool, now time.Time) (fire bool)`. Combo presente = `buttons & mask ==
  mask`. Primeira observação com o combo → `since[slot] = now`; ausente ou desconectado → zera `since` e `fired`;
  `now − since ≥ Hold` e `!fired` → `fired = true` e dispara. Slots são independentes [PAD-001..PAD-005].
- **Poller** (`pad.go`): uma goroutine dona (`wg`) com laço `select { case <-stop: return; case now := <-clock.After(Poll): }`.
  Ao iniciar, lê os 4 slots; depois lê a cada `Poll` os slots conectados e, a cada `Rescan` (1 s), os desconectados
  (guarda `lastProbe[slot]` e `connected[slot]`) [PAD-006]. `ERROR_DEVICE_NOT_CONNECTED` marca o slot desconectado e
  chama `Observe(..., connected=false)`. Gatilho: envio não bloqueante em `out` com `At = now` [PAD-001].
- Tempo exclusivamente por `clock.After`/`clock.Now()` (nada de `time.Sleep`/`time.Ticker`); nos testes, `FakeClock`
  com `BlockUntilWaiters(1)` + `Advance(16ms)` [PAD-006]. `Close` fecha `stop` e espera; não depende de o relógio
  avançar [PAD-008].
- Observação: leitura de slot vazio no XInput 1.4 é cara (alguns ms); por isso os desconectados são re-sondados a
  cada 1 s e não a 60 Hz.

## `internal/winapi` — energia, aviso e som

### Energia [PWR-001..PWR-006]
```go
type PowerCallbacks struct {
    OnSuspend func(at time.Time)
    OnWake    func(at time.Time, userPresent bool)
}
func NewPowerWatcher(clock domain.Clock, log *slog.Logger, cb PowerCallbacks) *PowerWatcher
func (w *PowerWatcher) Start() error   // creates the window and loop; error leaves nothing running
func (w *PowerWatcher) Close() error   // idempotent; waits for the loop
```
- Mesmo padrão do `SessionEndWatcher`: janela oculta de topo (estilo 0, sem `WS_VISIBLE`, **não** `HWND_MESSAGE`), laço
  `GetMessage` em goroutine com `LockOSThread`, classe e janela criadas e destruídas na própria thread, parada por
  mensagem privada (`WM_APP+1`), `exited`/`postMu` para nunca postar em `hwnd` reciclado. O `wndProc` compartilhado
  da `winapi` é generalizado para um mapa `hwnd → func(msg, wParam, lParam uintptr) (uintptr, bool)`, de modo que o
  `SessionEndWatcher` e o `PowerWatcher` usem a mesma função de janela sem um conhecer o outro (refatoração mínima; os
  testes do `SessionEndWatcher` continuam passando).
- `WM_POWERBROADCAST (0x0218)`: `wParam` ∈ {`PBT_APMSUSPEND 0x0004` → `OnSuspend(clock.Now())`;
  `PBT_APMRESUMESUSPEND 0x0007` → `OnWake(now, true)`; `PBT_APMRESUMEAUTOMATIC 0x0012` → `OnWake(now, false)`; qualquer
  outro → nada}; o procedimento sempre devolve `TRUE` (1). `at` é lido **dentro** do procedimento, antes de qualquer
  espera [PWR-001..PWR-004].
- O callback roda na thread da janela e **deve voltar rápido**: o Windows espera `PBT_APMSUSPEND` por ~2 s antes de
  dormir. Por isso o `powerBridge` do agente (abaixo) limita a espera a 1,5 s [PWR-005].
- Se `RegisterSuspendResumeNotification(hwnd, DEVICE_NOTIFY_WINDOW_HANDLE)` ajudar a receber os eventos em máquinas
  com Modern Standby, o implementer pode chamá-la no `Start` (e `UnregisterSuspendResumeNotification` no fim da
  thread); a decisão fica para o teste PWR-007 (Limitações 1), sem mudar o contrato dos callbacks.
- Testes: `PostMessageW`/`SendMessageW(hwnd, WM_POWERBROADCAST, PBT_*, 0)` na própria janela; contagem de handles/`GetGuiResources`
  em 20 ciclos [PWR-006].

### Aviso na tela [OVL-002, OVL-004, OVL-005, OVL-006]
```go
type Overlay struct{ ... } // implements feedback.Overlay
func NewOverlay(log *slog.Logger) *Overlay
func (o *Overlay) Start() error                  // thread/loop; error => caller logs OVL-006 and uses a no-op overlay
func (o *Overlay) Show(n feedback.Notice)        // sets the text, shows without activating; never blocks the caller
func (o *Overlay) Hide()
func (o *Overlay) Close() error
```
- **Thread dona e loop próprio**: `loop` (goroutine, `LockOSThread`) cria classe (`ArcadiaNotice-N`), janela e recursos
  GDI, sinaliza `ready`, e faz `GetMessage`. `Show`/`Hide`/`Close` só postam mensagens privadas (`WM_APP+2` mostrar com
  o texto guardado sob `mu`, `WM_APP+3` esconder, `WM_APP+1` parar) — nunca tocam na janela de outra thread.
- **Janela:** `CreateWindowExW(WS_EX_TOPMOST | WS_EX_NOACTIVATE | WS_EX_TOOLWINDOW | WS_EX_LAYERED | WS_EX_TRANSPARENT,
  class, "", WS_POPUP, ...)`; `SetLayeredWindowAttributes(hwnd, 0, 230, LWA_ALPHA)`. Mostrar:
  `SetWindowPos(HWND_TOPMOST, x, y, w, h, SWP_NOACTIVATE | SWP_SHOWWINDOW)` — **nunca** `SetForegroundWindow` nem
  `ShowWindow(SW_SHOW)` simples [OVL-002].
- **Pintura (`WM_PAINT`):** `FillRect` com fundo escuro (RGB 24,24,27) e `DrawTextW` branco centralizado, fonte "Segoe UI"
  20 px × dpi/96 criada no `loop` e liberada ao sair (`DeleteObject`), pincel idem [OVL-005]. `InvalidateRect` quando o
  texto muda. A thread define `SetThreadDpiAwarenessContext(DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2)` para o tamanho
  e a posição valerem em pixels reais do monitor.
- **Posição (`overlay_geometry.go`, puro):** `overlayRect(work Rect, dpi int, w, h int) Rect` com `x = work.Left +
  (work.Width − w)/2` e `y = work.Top + 48·dpi/96` (divisão inteira, arredondando para baixo). O monitor vem de
  `MonitorFromWindow(GetForegroundWindow(), MONITOR_DEFAULTTOPRIMARY)` + `GetMonitorInfoW(rcWork)` + `GetDpiForMonitor`
  [OVL-004].
- **Duração não está na janela:** quem decide quando esconder é o `feedback.Coordinator` (com `Clock`), que chama
  `Hide()` em `t + 2,5 s` [OVL-003]. A janela é burra.
- **Falha** (`Start` com erro): `NewOverlay` devolve o erro ao agente, que registra `Warn` "aviso na tela indisponível" e
  usa `feedback.NopOverlay` [OVL-006]. Em estação sem área de trabalho interativa os testes de janela são pulados.

### Som [SND-001, SND-002]
```go
type Beeper struct{ ... } // implements feedback.Sound
func NewBeeper(log *slog.Logger) *Beeper                       // beep = kernel32.Beep
func newBeeper(log *slog.Logger, beep func(freq, ms uint32) error) *Beeper // tests inject
func (b *Beeper) Play(c feedback.Cue)                          // non-blocking: replaces the pending cue
func (b *Beeper) Close() error
```
- Uma goroutine dona recebe de um canal de capacidade 1 (envio não bloqueante que troca o pendente) e chama `beep`
  em sequência; `Close` fecha `stop` e espera. Pausa: `660 Hz/90 ms → 440 Hz/120 ms`; retomada: `440 Hz/90 ms → 660 Hz/120 ms`
  [SND-001].
- `Beep` é síncrono (bloqueia ~210 ms) e usa o dispositivo de áudio padrão pelo mixer do Windows; por isso vive na
  própria goroutine. Não depende do esquema de sons do Windows (diferente de `MessageBeep`), que é o que dá a garantia
  de feedback pedida na decisão 4. Sem dependência nova.

## `internal/feedback` — coordenação [FBK-001..FBK-005, OVL-001, OVL-003]

```go
type Notice int
const ( NoticePaused Notice = iota + 1; NoticeResumed )
func (n Notice) Text() string // "Tempo pausado" | "Tempo retomado" [OVL-001]

type Cue int
const ( CuePaused Cue = iota + 1; CueResumed )

type Overlay interface { Show(Notice); Hide(); Close() error }
type Sound   interface { Play(Cue); Close() error }

type Event int // mirrors tracker.FeedbackKind without importing tracker
type Coordinator struct{ ... }
func New(o Overlay, s Sound, clock domain.Clock, log *slog.Logger) *Coordinator // nil o/s => no-op
func (c *Coordinator) Start()
func (c *Coordinator) Send(e Event)  // non-blocking; full queue drops the oldest + Warn [FBK-003]
func (c *Coordinator) Close() error  // hides, closes sinks, waits the goroutine; idempotent [FBK-005]
```
- **Uma goroutine dona** lê `events` (canal de capacidade 4) e `hide` (`clock.After(OverlayDuration)` com geração):
  por evento chama `overlay.Show(notice)` e `sound.Play(cue)` (as duas interfaces não devolvem erro; falhas internas são
  logadas por elas em `Warn`, e uma não impede a outra) e rearma o temporizador de esconder; ao vencer (e a geração casar) chama
  `overlay.Hide()` [OVL-003, FBK-002, FBK-004].
- `Send` faz envio não bloqueante; se cheio, descarta o mais antigo (lê um e reenvia) e loga `Warn` uma vez
  [FBK-003]. O `tracker.Config.Feedback` é `func(k tracker.FeedbackKind){ coord.Send(feedback.Event(k)) }` montado no
  `agent`.
- Duração `OverlayDuration = 2500 * time.Millisecond` (constante do pacote; `x-input.overlayDurationMs`).
- `FBK-001` (quando emitir) é da máquina; o coordenador só reage. `FBK-003` é testado com um `Overlay` falso cujo
  `Show` bloqueia 5 s reais e um tracker real com `FakeClock`: as transições e o `Notify` não esperam.

## `internal/agent` [PAU-001..PAU-006, AGT-012, AGT-013, IPC-033..IPC-035]

### Roteador e ponte de energia (`pause.go`)
```go
type pauseTracker interface {
    TogglePause(ctx context.Context, reason domain.PauseReason, at time.Time) (tracker.PauseResult, error)
    Suspend(ctx context.Context, at time.Time) error
    Wake(ctx context.Context, at time.Time, userPresent bool) error
}
type pauseRouter struct{ ... }
func newPauseRouter(tr pauseTracker, in <-chan input.Trigger, log *slog.Logger) *pauseRouter
func (r *pauseRouter) Start()
func (r *pauseRouter) Close()  // stops the goroutine and waits [PAU-005]
type powerBridge struct{ tr pauseTracker; timeout time.Duration; log *slog.Logger } // timeout 1.5s; tests shorten it
func (b *powerBridge) OnSuspend(at time.Time)
func (b *powerBridge) OnWake(at time.Time, userPresent bool)
```
- **`pauseRouter`**: uma goroutine dona lê `in` (fila única de capacidade 8 compartilhada por `Hotkey` e `Pad`),
  aplica o antirrepique — descarta o gatilho se `gatilho.At − últimoEncaminhado.At < domain.PauseToggleDebounce` e
  atualiza `últimoEncaminhado` ao encaminhar — e chama `TogglePause(ctx, t.Source, t.At)` com `context.WithTimeout(1 s)`.
  Resultado `PauseIgnored` → `Debug` "gatilho de pausa ignorado" (PAU-003); `ErrShuttingDown` → `Debug` (PAU-005);
  outro erro → `Warn`. Para sair: canal `stop`. Mede a janela só pelo `At` do gatilho, nunca por `time.Now()` [PAU-004].
- **`powerBridge`**: `OnSuspend` chama `tr.Suspend(ctx com timeout, at)`; ao estourar, `Warn` "tracker não respondeu à
  suspensão a tempo" e volta (a goroutine dona aplica com o `at` original). `OnWake` idem, `Debug` em erro.
  `context.WithTimeout` usa o relógio real — é um limite de espera de E/S, como os `writeTimeout` do tracker; o
  `FakeClock` não participa [PWR-005].
- Por que um roteador em vez de as fontes chamarem o tracker direto: o loop do atalho e o poller não devem bloquear
  numa chamada ao tracker (a thread do atalho tem de continuar bombeando mensagens); o roteador centraliza o
  antirrepique e o tratamento de erros [PAU-001..PAU-005].

### Handler `game.launchUntracked` (`games.go`) [IPC-033..IPC-035]
- `sessionTracker` ganha `LaunchUntracked(ctx, spec) (uint32, error)`. O handler reaproveita o decodificador de
  `game.launch` (`gameLaunchParams`, `parseGameID`, `ipc.DecodeParams`): params nulos/ausentes → `-32602`;
  `store.Game(id)` (`ErrGameNotFound` → `-32001`, conferido antes da sessão); `launch_type != exe` → `-32003`; monta
  o mesmo `LaunchSpec`; `context.WithTimeout(ctx, 10 s)`; `tr.LaunchUntracked` → `ipc.LaunchedProcess{PID}`. O mapeamento
  de erros é `launchError` (o mesmo de `game.launch`): `SessionActiveError` → `-32002`, `ErrShuttingDown`/`LaunchError`
  → `-32003`, resto (inclusive o fim do `ctx`) → log `Error` + `-32603` [IPC-034].
- **Handlers `session.pause` / `session.resume`** (`sessionHandlers` em `games.go`): `sessionTracker` ganha
  `Pause(ctx, at)` e `Resume(ctx, at)`; o handler lê `at = clock.Now()` ao receber o pedido, usa
  `context.WithTimeout(ctx, 5 s)` e converte o snapshot com `ipc.SessionFromSnapshot`. `*tracker.NotPausableError` →
  `-32005` com `SessionNotPausableData`; `ErrShuttingDown` e fim do `ctx` → log `Debug`/`Error` e `-32603` [IPC-038..IPC-040].
  Esses pedidos **não passam pelo `pauseRouter`**: são síncronos (a UI espera a resposta), idempotentes e não usam o
  antirrepique nem atualizam seu "último gatilho" [PAU-008]. O feedback sai do tracker, igual ao do atalho.
- Extrai-se um `prepareLaunch(ctx, st, params) (store.Game, tracker.LaunchSpec, error)` comum aos dois handlers para
  não duplicar a validação.

### Subida e shutdown [AGT-012, AGT-013]
```go
// config gains optional injections for tests (nil => real components on Windows, none elsewhere).
Sources  []input.Source           // default: input.NewHotkey(DefaultHotkey), input.NewPad(xinput, ...)
Power    func(winapi.PowerCallbacks) (io.Closer, error) // default: winapi.NewPowerWatcher(...).Start
Overlay  feedback.Overlay          // default: winapi.NewOverlay
Sound    feedback.Sound            // default: winapi.NewBeeper
```
- `startAgent`: `store.Open(Migrate)` → recuperação → `feedback.New(overlay, sound, clock, log)` + `Start()` →
  `tracker.New(Config{..., Feedback: coord.Send})` + `Start()` → `ipc.NewServer` + `Serve` → `pauseRouter.Start()` →
  `Source.Start(triggers)` para cada fonte e `PowerWatcher.Start()`. Cada falha de componente é `Error`/`Warn` do
  componente e segue (PAU-006); os já iniciados são mantidos. Nenhuma fonte inicia antes do tracker [AGT-012].
- `instance.Shutdown(ctx, reason)`: `tracker.BeginShutdown()` → **`powerWatcher.Close()`, `Source.Close()` (cada uma) e
  `pauseRouter.Close()`** → `Drain` → `Server.Close` → `wg.Wait` → `tracker.Close(ctx)` → **`feedback.Close()`** →
  `store.Close` [AGT-013]. As fontes fecham primeiro para nenhum gatilho novo chegar; o feedback fecha depois do
  tracker, que pode ainda chamar `Feedback` em nenhum caso do `Stop`, e antes do store só por simetria (não usa o banco).
- Orçamento: fechar as fontes é imediato (`PostThreadMessage` + `wg.Wait`, o poller sai no próximo `select`, o watcher de
  energia posta a parada); `Drain` ≤ 500 ms + escrita de fim ≤ 2 s de timeout (normalmente < 50 ms) continuam dentro dos
  2 s de AGT-003, também com a sessão em Paused [AGT-013].
- Em `run_other.go` (não Windows) as fontes, o watcher e o aviso são nulos; o agente compila e os testes portáveis usam
  falsos.

## `internal/ipc` [IPC-032..IPC-037]

- `protocol.go`: `MethodGameLaunchUntracked = "game.launchUntracked"`; `LaunchedProcess{PID uint32 `json:"pid"`}`;
  `GameLaunchUntrackedParams{GameID int64 `json:"gameId"`}` (codificação do cliente). Nenhum código de erro novo.
  `Session.State` passa a poder ser `"paused"` (a conversão `SessionFromSnapshot` já copia `State`).
- `contract_test.go` passa a ler `docs/specs/phase-4-pause/contracts/ipc.openrpc.json`: versão `0.5.0`; todos os
  métodos/notificações existem; o enum de `Session.state`; os campos obrigatórios de `LaunchedProcess`; o comentário do
  pacote aponta para o novo arquivo [IPC-032]. O contrato da fase 3 fica como histórico.
- `session.current` e `session.changed` não mudam de forma: só aparece o valor `paused` [IPC-036, IPC-037].
- Métodos `MethodSessionPause = "session.pause"` e `MethodSessionResume = "session.resume"` (sem params; `ipc.NoParams`),
  resultado `SessionResult{Session Session}` (nunca nulo); código `CodeSessionNotPausable = -32005`, mensagem
  `MsgSessionNotPausable = "Session not pausable"`, `SessionNotPausableData{Reason string}` com as constantes
  `ReasonNoSession = "no-session"` e `ReasonGrace = "grace"` [IPC-038..IPC-040].

## `cmd/arcadia` (UI, Go) [DSK-013, DSK-014, DSK-015]

```go
func (a *App) LaunchGameUntracked(id int64) error   // timeout 12 s; no event, no return value
func (a *App) PauseSession() (SessionView, error)    // session.pause, timeout 5 s [DSK-015]
func (a *App) ResumeSession() (SessionView, error)   // session.resume, timeout 5 s [DSK-015]
```
- `userError` ganha o caso `-32005` (`data.reason` → "Não há jogo em andamento para pausar." / "O jogo está encerrando. Não
  dá para pausar agora."; sem `data` ou motivo desconhecido → "Não foi possível pausar agora.") [DSK-015].
- `sessionView` converte o resultado; os bindings não emitem evento (o `session:changed` vem do supervisor).
- Chama `game.launchUntracked` com `GameLaunchUntrackedParams`; erros passam por `userError` (a tabela de DSK-011 já
  cobre `-32001`, `-32002`, `-32003`, `-32603` e conexão) [DSK-013].
- `sessionView` já valida o estado com `domain.ParseSessionState`; com DOM-010 `paused` passa a ser aceito e repassado
  pelo supervisor como `session:changed` [DSK-014]. `wails generate module` atualiza `frontend/wailsjs/`.

## `frontend/` [UI-019..UI-025]

- `lib/games.ts`: `SessionState` ganha `'paused'`; `isLive` (e `isSessionView`) tratam `running`, `paused` e `grace`
  como sessão viva; `launchGameUntracked(id)` e `useLaunchGameUntracked()` (mutation que **não** mexe no cache de
  sessão nem na lista) [UI-020, UI-024].
- `lib/sessionClock.ts`: `SessionAnchor.state` vira `'running' | 'paused' | 'grace'`; `anchorFrom` mapeia `paused` →
  `'paused'`; `elapsedMs` só soma o tempo decorrido em `running`; `useElapsedSeconds` só arma temporizador em `running`
  — em `paused` não há `setTimeout` [UI-022, UI-023, UI-025].
- `SessionPanel`: `grace` → "Encerrando…"; `paused` → "Pausado" (cor neutra/azul, distinta do âmbar da carência); o resto
  igual. O cronômetro continua sem chamar binding [UI-022, UI-025].
- **Botão do painel** [UI-027, UI-028]: `SessionPanel` recebe `onToggle: () => void` e `pending: boolean`; o rótulo sai só
  do `session.state` ("Pausar" em `running` e `grace`, "Retomar" em `paused`), `disabled` em `grace` ou `pending`. `App`
  guarda `pausePending` (`useState`) e chama `useToggleSession()`: mutation que, conforme o estado atual em cache,
  chama `pauseSession()` (`running`) ou `resumeSession()` (`paused`), com a mesma regra de UI-018 (a resposta só vai
  ao cache se nenhum `session:changed` chegou desde o pedido); erro → `ErrorBanner`. O botão nunca altera o cache de sessão
  antes da resposta e não calcula tempo. `lib/games.ts` ganha `pauseSession()`/`resumeSession()` sobre
  `window.go.main.App`.
- `GameList`: indicador por estado — `running`/`grace` "Em execução", `paused` "Pausado" [UI-024]; dois botões por linha,
  "Jogar" e "Sem contar tempo" (`onPlay(id)` e `onPlayUntracked(id)`); `pending` passa de `number | null` para
  `{ id: number; mode: 'tracked' | 'untracked' } | null`: ambos os botões do jogo pendente ficam `disabled`, o acionado
  mostra "Iniciando…" [UI-019, UI-021].
- `App.tsx`: `onPlayUntracked` chama `launchGameUntracked`; erro → `ErrorBanner` com a mensagem de DSK-011; sucesso →
  limpa o aviso e não faz mais nada [UI-020].
- O resto de UI-009/UI-016/UI-018 não muda: os estados novos passam pelos mesmos caminhos (`isLive`, `resetQueries`).

## Testes

- **Pura** (`machine_test.go`, qualquer SO): uma linha da tabela de transições por caso, com IDs SES-*; inclui
  Paused × cada evento, `TogglePause` com `t < startedAt`, `Wake` por motivo, efeitos de feedback (FBK-001) e a cadeia
  SES-019/SES-021/SES-023 com instantes fixos.
- **Coordenador** (`tracker_test.go`, qualquer SO): `FakeClock`, `fakeLauncher` (com `LaunchDetached`), `fakeJob`,
  `fakeStore` (com `PauseSession`/`ResumePause`), `Notify` e `Feedback` gravando chamadas. Sempre
  `BlockUntilWaiters(2)` antes de `Advance` em Running/Paused. `goleak` no `Close` com sessão em Paused [SES-024].
- **Store** (`pauses_test.go`, `sessions_test.go`): SQLite em `t.TempDir()`, table-driven; migração 3 → 4 com dados
  [STO-018]; atomicidade por trigger de teste [STO-021]; STO-023 com `domain.ActiveDuration`.
- **Input**: `combo_test.go` e `pad_test.go` com `PadAPI` falso e `FakeClock` (PAD-001..PAD-006, PAD-008);
  `hotkey_windows_test.go` com combinação de teste e `WM_HOTKEY` injetado, `goleak` e segundo listener (HOT-001..HOT-004).
- **Feedback**: `feedback_test.go` com `Overlay`/`Sound` falsos e `FakeClock` (OVL-001, OVL-003, FBK-002..FBK-005);
  `overlay_geometry_test.go` (OVL-004); `overlay_windows_test.go` (OVL-002, OVL-005, OVL-006; pulado sem desktop);
  `beep_windows_test.go` com `beep` injetado (SND-001, SND-002).
- **Energia**: `power_windows_test.go` injetando `WM_POWERBROADCAST` (PWR-001..PWR-006); `powerBridge` com tracker
  falso lento e timeout curto (PWR-005).
- **Agent**: `pause_test.go` (roteador, antirrepique, PAU-008 com pedido da UI no meio, shutdown), handlers
  `session.pause`/`session.resume` via `ipc.Dispatch` (IPC-038..IPC-040), `app_test.go`/`shutdown_test.go` com fontes e feedback
  falsos (AGT-012, AGT-013, PAU-006), `games_test.go` via `ipc.Dispatch` para `game.launchUntracked`.
- **Integração Windows** (`integration_windows_test.go`): `fake-game` real com `Launcher` real e `FakeClock`;
  TRK-009 (pausa/retoma, `F` durante a pausa → Grace), TRK-010 e TRK-011 (lançamento sem rastrear, handles).
- **UI Go**: `LaunchGameUntracked` com `pinger` falso e `userError`; supervisor com `state = paused`.
- **Frontend**: Vitest para o botão do painel (UI-027, UI-028) e `SessionPanel` paused/resume (UI-022, UI-023, UI-025), `GameList` (UI-019, UI-021, UI-024),
  `App` (UI-020), `sessionClock` com `paused` e UI-016/UI-018 com um caso `paused`.
- **Manuais**: HOT-005, PAD-009, PWR-007, OVL-007, SND-003, PAU-007, UI-026 (checklist na T final do planner).

## Riscos

- **Goroutines novas e donos.** Atalho (dono: `Hotkey`, sai por `WM_QUIT` postado no `Close`), poller (dono: `Pad`, sai
  por `stop`), energia (dono: `PowerWatcher`, mensagem privada), aviso (dono: `Overlay`, mensagem privada), som (dono:
  `Beeper`, `stop`), coordenador de feedback (dono: `Coordinator`, `stop`), roteador (dono: `pauseRouter`, `stop`).
  Todas esperadas no shutdown na ordem de AGT-013; nenhum `go func()` solto.
- **Handles/GDI.** Janela, classe, fonte e pincel do aviso e a janela/classe da energia são criados e destruídos na
  thread que os possui; `GetGuiResources` e `GetProcessHandleCount` medem nos testes (OVL-005, PWR-006). XInput não abre
  handle. `LaunchDetached` fecha `hProcess`/`hThread` ao retornar.
- **Suspensão e tempo de resposta.** O Windows dá ~2 s ao `PBT_APMSUSPEND`; o handler espera ≤ 1,5 s e a gravação
  (`writeTimeout` 2 s) pode terminar depois de adormecer. O estado em memória já tem o instante certo; se o PC dormir
  antes da gravação, o heartbeat seguinte (ao acordar) grava `paused` e, se o agente cair durante o sono, a recuperação
  fecha a sessão em `last_seen_at` (≤ 30 s antes) sem contar o sono.
- **Relógio e sono.** O tempo ativo é acumulado por `Sub` de instantes; com a pausa aberta de `at` (suspensão) até a
  retomada, o período dormindo nunca entra no acumulado, qualquer que seja a leitura monotônica do Go durante o sono.
- **`RegisterHotKey` pega o atalho no sistema todo** e conflita com outro programa (Limitações 2); falha só no log.
  A combinação padrão é de baixo risco, mas não zero (editores, ferramentas de captura).
- **Anti-cheat.** Nenhum hook de teclado nem injeção: só `RegisterHotKey`, XInput de leitura e janela própria; não
  deve acionar anti-cheat, mas o aviso é uma janela topmost sobre o jogo e jogos com proteção agressiva podem reclamar
  (a verificar em OVL-007).
- **XInput e Steam Input.** Com Steam Input ativo o jogo pode ver um controle virtual; o agente lê os dispositivos
  XInput do sistema e pode ver só o virtual ou o físico (Limitações 9). PAD-009 verifica.
- **Modern Standby.** `PBT_APMSUSPEND` pode não chegar; ver Limitações 1 e 12. A mitigação por lacuna de heartbeat
  (tratar um intervalo >> 30 s entre ticks como sono) está fora de escopo; é candidata a fase futura.
- **Antirrepique × múltiplas fontes.** A janela de 300 ms é medida em `At`; um controle e o teclado apertados juntos
  contam como um só gatilho. Um usuário que pausa e retoma em < 300 ms de propósito terá a retomada ignorada (aceito).
- **`FakeClock` e temporizadores não canceláveis:** o coordenador de feedback usa geração para o `hide`; o poller usa um
  `After` por ciclo; os testes contam waiters como na fase 3.
- **Despacho sequencial do IPC (fase 2):** `game.launchUntracked` é rápido (um `CreateProcess`), mas com antivírus lento
  atrasa o `agent.status` da mesma conexão, como `game.launch` (Limitações 10 da fase 3).
- **Compatibilidade de schema:** UI e agente são o mesmo binário (`SchemaVersion = 4`). Um agente v3 antigo com UI nova:
  o método `game.launchUntracked` dá `-32601` → "Erro inesperado" até o agente reiniciar; sessões `paused` só existem
  com agente v4.
