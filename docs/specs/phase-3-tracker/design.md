# Fase 3 — Tracker MVP: design

- **Status:** aprovado junto com [requirements.md](requirements.md) (2026-11-09)
- Implementa [requirements.md](requirements.md). Base: designs da [fase 2](../phase-2-skeleton/design.md) e do
  [executável único](../single-exe/design.md) (tudo o que não é citado aqui continua como lá). ADRs 0005, 0009, 0010,
  0011, 0012, 0013, 0014, 0015, 0016. Contratos: [ipc.openrpc.json](contracts/ipc.openrpc.json) (v0.4.0, substitui a
  v0.3.0), [session-state-machine.md](contracts/session-state-machine.md), [fake-game.md](contracts/fake-game.md).
- Os IDs entre colchetes indicam os requisitos atendidos. Segue as decisões do usuário de 2026-11-09 (P1..P7,
  [requirements.md](requirements.md#decisões-do-usuário-2026-11-09)).

## Quem escreve no banco

**Decisão ([ADR 0015](../../adr/0015-agente-unico-escritor-do-banco.md)): nesta fase o agente é o único escritor do `library.db`. A UI pede tudo
por IPC (`game.add`, `game.launch`) e lê a lista por IPC (`game.list`); ela não abre mais o banco para a tela.**
[GAM-001, IPC-020, IPC-023]

Motivos:
1. O fim de uma sessão atualiza `game.total_play_seconds`, `play_count`, `last_played_at` e `first_played_at` na
   **mesma transação** que fecha `play_session` (STO-014). Se a UI fosse dona de `game`, o agente escreveria colunas
   de uma tabela alheia ou o total seria atualizado em dois passos por dois processos.
2. O agente já é o dono das migrações (ADR 0010) e o processo que sempre está de pé; um único escritor elimina
   `SQLITE_BUSY` entre processos e a corrida "UI grava jogo enquanto o agente fecha sessão".
3. A lista precisa juntar dados do jogo com o estado vivo da sessão, que só o agente conhece.
4. "A UI nunca influencia o tempo de jogo" fica mais fácil de garantir: a UI não tem nenhum caminho de escrita.

Isso **altera a divisão da ARCHITECTURE §2** ("a UI é dona da biblioteca"), registrado no ADR 0015 (a sessão
principal ajusta a §2). A fase 5
(metadata, edição, scraping) pode reavaliar: por exemplo, manter o agente como escritor único e mover só as leituras
pesadas (grade, filtros, FTS) para leitura direta da UI sem migrar (ADR 0010 continua valendo para leitura).

Consequências nesta fase: o binding `LibrarySummary` e o `libraryGate` continuam como estão (DSK-007/DSK-008 seguem
válidos e testados), mas o frontend deixa de chamá-los (UI-010). A remoção fica para a fase 5.

## Layout de pacotes

```
internal/domain/
  session.go          # SessionState, SessionEndReason, Parse*, Outcome, constantes, SessionSnapshot [DOM-007, DOM-008]
  game.go             # DefaultGameTitle [DOM-009]
internal/store/
  migrations/00003_session_end_reason.sql            [STO-011]
  queries/games.sql  queries/sessions.sql            (sqlc)
  games.go  sessions.go                              [GAM-001..003, GAM-005, STO-012..017]
internal/tracker/      # só o tracker-specialist (ADR 0005/0007)
  machine.go          # máquina pura, portável [SES-*]
  tracker.go          # coordenador: dono da goroutine da sessão, portável [SES-*, TRK-008]
  recover.go          # recuperação de sessões abertas, portável [SES-010]
  launcher.go         # interfaces Launcher/Job/JobEvent, portável
  job_windows.go      # Job Object + IOCP + CreateProcess [TRK-001..007]
  job_other.go        # !windows: Launcher que sempre falha (errors.ErrUnsupported)
internal/ipc/
  protocol.go         # + métodos, códigos, tipos Game/Session/params/data [IPC-020, IPC-021, IPC-023..031]
  server.go           # + Server.Notify [IPC-029]
internal/agent/
  games.go            # handlers game.* e session.current; validação do exe [GAM-002, GAM-003, IPC-020..028]
  app.go run_windows.go  # subida/shutdown com recuperação e tracker [AGT-009..011]
cmd/arcadia/
  games.go            # bindings PickGameExe/AddGame/ListGames/LaunchGame/CurrentSession [DSK-011, DSK-012]
  errors.go           # userError: código JSON-RPC → texto em português [DSK-011]
  supervisor.go       # + repasse de session.changed [DSK-010]
cmd/fake-game/        # só o tracker-specialist [FKG-*]
frontend/src/
  components/GameList.tsx  AddGameForm.tsx  ErrorBanner.tsx  SessionPanel.tsx
  lib/games.ts        # hooks e formatPlayTime [UI-004..010, UI-018]
  lib/sessionClock.ts # formatElapsed, useElapsedMs [UI-012..017]
```

Regra do CLAUDE.md respeitada: `domain`, `store`, `machine.go`, `tracker.go`, `recover.go` e `launcher.go` não
importam nada de Windows; Win32 só em `job_windows.go`.

## `internal/domain` [DOM-007, DOM-008, DOM-009]

```go
type SessionState string
const (
    SessionRunning   SessionState = "running"
    SessionGrace     SessionState = "grace"
    SessionEnded     SessionState = "ended"
    SessionDiscarded SessionState = "discarded"
)
func ParseSessionState(s string) (SessionState, error)

type SessionEndReason string
const (
    EndExited        SessionEndReason = "exited"
    EndAgentShutdown SessionEndReason = "agent-shutdown"
    EndRecovered     SessionEndReason = "recovered"
)
func ParseSessionEndReason(s string) (SessionEndReason, error)

const (
    SessionGracePeriod       = 15 * time.Second
    MinCountedSession        = 30 * time.Second
    SessionHeartbeatInterval = 30 * time.Second
)

// Outcome returns SessionEnded when active >= MinCountedSession, else SessionDiscarded.
func Outcome(active time.Duration) SessionState

// SessionSnapshot is the live view of a session (IPC Session, notifications).
type SessionSnapshot struct {
    ID, GameID int64
    Title      string
    State      SessionState
    StartedAt  time.Time
    EndedAt    time.Time     // zero while running/grace
    Active     time.Duration // active time measured at AsOf
    AsOf       time.Time     // agent clock reading at which Active was measured [IPC-031]
}

// DefaultGameTitle returns the file name of path without a trailing ".exe" (case-insensitive),
// splitting on both '\\' and '/' so it behaves the same on every OS.
func DefaultGameTitle(path string) string
```

- Valores lidos do banco passam por `Parse*`; valor desconhecido (escrita fora do Arcadia) é logado e tratado como
  sessão aberta na recuperação (ADR 0012, "leitores devem tolerar").
- `ActiveDuration` (DOM-001..004) não é usado nesta fase (sem pausas); fica para a fase 4.

## `internal/store` [STO-011..017, GAM-001..003, GAM-005]

### Migração 3 [STO-011]
```sql
-- +goose Up
ALTER TABLE play_session ADD COLUMN end_reason TEXT;      -- exited | agent-shutdown | recovered (ADR 0012)
CREATE INDEX play_session_open_idx ON play_session (ended_at) WHERE ended_at IS NULL;
-- +goose Down
DROP INDEX play_session_open_idx;
ALTER TABLE play_session DROP COLUMN end_reason;
```
`store.SchemaVersion = 3`. `ALTER TABLE ... DROP COLUMN` exige SQLite ≥ 3.35 (o `modernc.org/sqlite` atual embute
3.4x). Nenhuma reconstrução de tabela (ADR 0012).

### API
```go
var ErrGameNotFound = errors.New("store: game not found")
type Game struct {
    ID int64; Title, LaunchType, LaunchTarget, LaunchArgs, WorkingDir string
    TotalPlaySeconds, PlayCount int64
    FirstPlayedAt, LastPlayedAt time.Time // zero = NULL
}
type NewGame struct{ Title, Exe, WorkingDir string; At time.Time }

func (s *Store) AddGame(ctx context.Context, g NewGame) (Game, error)        // [GAM-001]
func (s *Store) Games(ctx context.Context) ([]Game, error)                   // [GAM-005]
func (s *Store) Game(ctx context.Context, id int64) (Game, error)            // ErrGameNotFound

type OpenSession struct{ ID, GameID int64; StartedAt, LastSeenAt time.Time; ActiveSeconds int64 }
type SessionEnd struct {
    ID int64; EndedAt time.Time; Active time.Duration
    Reason domain.SessionEndReason; WrittenAt time.Time
}
func (s *Store) StartSession(ctx context.Context, gameID int64, at time.Time) (int64, error)           // [STO-012]
func (s *Store) UpdateSession(ctx context.Context, id int64, st domain.SessionState,
    lastSeen time.Time, active time.Duration) error                                                     // [STO-013]
func (s *Store) EndSession(ctx context.Context, e SessionEnd) (domain.SessionState, error)             // [STO-014, STO-015]
func (s *Store) DeleteSession(ctx context.Context, id int64) error                                     // [SES-012]
func (s *Store) OpenSessions(ctx context.Context) ([]OpenSession, error)                               // [STO-017]
```

- `AddGame` numa transação (`Store.Tx`): `INSERT INTO platform(name, category) VALUES('PC','pc') ON CONFLICT(name)
  DO NOTHING` → `SELECT id FROM platform WHERE name = 'PC'` → `INSERT INTO game (...)`. `sort_title` fica `''`,
  `status` `'not_played'` (default). Não há checagem de executável repetido: o mesmo `launch_target` pode aparecer em
  várias linhas, cada uma um jogo distinto (decisão P2) [GAM-001].
- `Games`: `ORDER BY title COLLATE NOCASE, id` [GAM-005].
- `EndSession` numa transação: lê `started_at`/`game_id` da sessão; `state = domain.Outcome(active)`; `UPDATE
  play_session SET ended_at, last_seen_at = ended_at, active_seconds = floor(active/1s), state, end_reason WHERE id
  = ? AND ended_at IS NULL`; se `ended` → `UPDATE game SET total_play_seconds = total_play_seconds + ?, play_count =
  play_count + 1, last_played_at = ?, first_played_at = COALESCE(first_played_at, ?), updated_at = ? WHERE id = ?`.
  Devolve o estado final. Sessão já encerrada (0 linhas) → erro (não soma duas vezes) [STO-014, STO-015, STO-016].
- `active_seconds = int64(active / time.Second)` em todo lugar (floor) [STO-013].
- Instantes via `TimeToMillis`/`MillisToTime` (ADR 0009); nulos por `sql.NullInt64`.
- Testes de atomicidade (STO-014): um `Store` sobre banco com trigger de teste `BEFORE UPDATE ON game … RAISE(ABORT)`
  criado pelo próprio teste; confere que a sessão continua aberta.

## `internal/tracker` [SES-*, TRK-*]

### Interfaces (portáveis, `launcher.go`)
```go
type LaunchSpec struct {
    GameID  int64
    Title   string
    Exe     string // game.launch_target
    Args    string // game.launch_args, raw command-line tail (phase 3: always "")
    WorkDir string // game.working_dir; "" → directory of Exe
}
type Launcher interface {
    // Prepare creates the process suspended and assigns it to a new job. On error everything it created
    // was terminated and closed.
    Prepare(spec LaunchSpec) (Job, error)
}
type JobEventKind int
const (EventNewProcess JobEventKind = iota + 1; EventExitProcess; EventActiveZero)
type JobEvent struct { Kind JobEventKind; PID uint32; Root bool; ExitCode uint32 } // ExitCode only for Root exits
type Job interface {
    RootPID() uint32
    Resume() error                   // resumes the initial thread; once
    Terminate() error                // only before Resume (failed launch); never after
    Events() <-chan JobEvent         // closed after Close
    ActiveProcesses() (int, error)   // JobObjectBasicAccountingInformation.ActiveProcesses
    Close() error                    // stops the reader, closes job/port/process handles; never kills processes
}
type SessionStore interface {        // real: *store.Store
    StartSession(ctx context.Context, gameID int64, at time.Time) (int64, error)
    UpdateSession(ctx context.Context, id int64, st domain.SessionState, lastSeen time.Time, active time.Duration) error
    EndSession(ctx context.Context, e store.SessionEnd) (domain.SessionState, error)
    DeleteSession(ctx context.Context, id int64) error
}
```
(`SessionEnd` pode ser movido para `domain` pelo planner se o tracker não deve importar `store`; o tracker já não
importa nada de Windows nos arquivos portáveis.)

`LaunchError{Err error; WinError uint32}` (com `Unwrap`) é o erro de `Prepare`/`Resume`; `WinError` vem de
`errors.As(err, &windows.Errno)` no lado Windows [IPC-025].

### Máquina pura (`machine.go`) [SES-001..007, SES-011, SES-013, SES-015]
Implementa exatamente a tabela de [session-state-machine.md](contracts/session-state-machine.md). Forma sugerida
(o tracker-specialist pode ajustar a assinatura, não o comportamento):
```go
type State int // Idle, Launching, Running, Grace
type Machine struct { /* state, session fields, accumulated, runningSince, graceSince, gen */ }
type Effect struct { Kind EffectKind; /* params: state, lastSeen, active, endedAt, reason, gen, delay */ }
// Each method takes the event instant and returns the effects to run, in order. No I/O, no clock.
func (m *Machine) Prepared(now time.Time) []Effect
func (m *Machine) Resumed() []Effect
func (m *Machine) LaunchFailed() []Effect
func (m *Machine) ProcessAdded(now time.Time) []Effect
func (m *Machine) TrackedEmpty(now time.Time) []Effect
func (m *Machine) Heartbeat(now time.Time, gen uint64) []Effect
func (m *Machine) GraceTimeout(gen uint64) []Effect
func (m *Machine) Stop(now time.Time) []Effect
func (m *Machine) Snapshot(now time.Time) (domain.SessionSnapshot, bool) // Active and AsOf from the same now [IPC-031]
```
Efeitos: `StartSession`, `UpdateSession`, `EndSession`, `DeleteSession`, `ArmHeartbeat`, `ArmGrace`, `Notify`,
`CloseJob`, `TerminateJob`. Testes table-driven só com `time.Time` fixos, sem goroutines.

### Coordenador (`tracker.go`) [SES-001..015, AGT-011, TRK-008]
```go
type Config struct {
    Clock    domain.Clock
    Store    SessionStore
    Launcher Launcher
    Notify   func(domain.SessionSnapshot) // must not block (real: ipc.Server.Notify)
    Log      *slog.Logger
    // zero → domain defaults; tests may shorten
    Grace, Heartbeat, Poll time.Duration
}
var ErrSessionActive = errors.New("tracker: session already active")  // *SessionActiveError{Session domain.SessionSnapshot}
var ErrShuttingDown  = errors.New("tracker: shutting down")           // [AGT-011]
func New(cfg Config) *Tracker
func (t *Tracker) Start()                                              // starts the owner goroutine
func (t *Tracker) Launch(ctx context.Context, spec LaunchSpec) (domain.SessionSnapshot, error)
func (t *Tracker) Current() (domain.SessionSnapshot, bool)             // activeSeconds at clock.Now()
func (t *Tracker) BeginShutdown()                                      // from now on Launch → ErrShuttingDown
func (t *Tracker) Close(ctx context.Context) error                     // Stop, wait goroutines; idempotent
```
- **Uma goroutine dona** (`run`), iniciada por `Start` e esperada por `Close` (`sync.WaitGroup`). Ela é a única que
  toca na `Machine`, no `Job` e no store. `select` em: pedidos de lançamento (canal com resposta), `job.Events()`
  (nil sem sessão), temporizadores do estado (`clock.After`, com geração), `stop`.
- `Launch` envia o pedido e espera a resposta ou o `ctx`. Se o `ctx` do chamador acaba no meio, o lançamento segue
  até o fim na goroutine dona (o processo pode já ter sido criado) e a resposta é descartada; a UI vê o resultado por
  `session.changed`.
- Lançamento em andamento é feito **dentro** da goroutine dona (`Prepare` → `StartSession` → `Resume`), então um
  segundo `Launch` só é lido depois e encontra a sessão ativa [SES-002]. Os pedidos que chegam enquanto ela está
  ocupada ficam no canal (sem buffer: o chamador espera); por isso `SessionActive` em "Launching" na prática vira
  "Running" quando lido — o contrato aceita os dois.
- `Current` lê um snapshot protegido por `sync.Mutex`, atualizado pela goroutine dona a cada transição; com uma única
  leitura `now := clock.Now()`, `AsOf = now` e `Active` é recalculado com esse `now` quando o estado é Running (em
  Grace, `Active` é o acumulado) [IPC-026, IPC-031].
- `Notify` é chamado pela goroutine dona depois da escrita no banco (com ou sem erro de escrita), com `AsOf` = instante
  do evento da transição (para `ended`/`discarded`, o instante em que a carência venceu) [IPC-027, IPC-031].
- `Stop` (via `Close`): a goroutine dona aplica `Machine.Stop(clock.Now())`, faz o `EndSession` com o `ctx` do
  `Close`, chama `Job.Close` e sai [SES-011, TRK-004]. Sem `Notify` (o servidor já fechou).
- Falhas de escrita seguem SES-014: log `Error` "falha ao gravar a sessão" (`session_id`, `error`); o estado em
  memória avança.
- Timeout de cada escrita: `context.WithTimeout(base, 2*time.Second)` (o `busy_timeout` do SQLite é 5 s; 2 s cabe no
  orçamento do shutdown).

### Job Object no Windows (`job_windows.go`) [TRK-001..007]
`Prepare(spec)`:
1. `job := CreateJobObject(nil, nil)` (sem nome).
2. `SetInformationJobObject(job, JobObjectExtendedLimitInformation, {BasicLimitInformation.LimitFlags =
   JOB_OBJECT_LIMIT_BREAKAWAY_OK})` — **sem** `KILL_ON_JOB_CLOSE` (ADR 0005) e sem `SILENT_BREAKAWAY_OK`
   (ADR 0016) [TRK-004, TRK-006].
3. `port := CreateIoCompletionPort(INVALID_HANDLE_VALUE, 0, 0, 1)`;
   `SetInformationJobObject(job, JobObjectAssociateCompletionPortInformation, {CompletionKey: 1, CompletionPort: port})`.
4. `CreateProcess(appName = Exe, cmdLine = quoteExe(Exe) + (" " + Args se não vazio), inherit = false,
   flags = CREATE_SUSPENDED | CREATE_DEFAULT_ERROR_MODE, env = nil, cwd = WorkDir ou dir(Exe))` [TRK-005].
   Reaproveitar `quoteExe` de `internal/winapi` (exportar como `winapi.QuoteExe` ou duplicar no tracker).
5. `AssignProcessToJobObject(job, pi.Process)`. Falha → `TerminateProcess(pi.Process, 1)`, fecha tudo, erro.
6. Inicia a goroutine leitora (dona: o `job`, esperada no `Close`) e devolve o `job`, guardando `pi.Process` (para o
   código de saída do processo inicial) e `pi.Thread` (para `Resume`).

- Como o processo só é retomado depois do `AssignProcessToJobObject`, todo descendente nasce dentro do job, sem janela
  de corrida [TRK-001, TRK-002, TRK-003].
- `Resume()`: `ResumeThread(pi.Thread)` (retorno `0xFFFFFFFF` = erro) e fecha `pi.Thread` logo depois.
- `Terminate()`: `TerminateProcess(pi.Process, 1)` — só permitido antes do `Resume` (processo que nunca executou
  código do jogo). Nunca chamado com o jogo rodando.
- Leitora: laço `GetQueuedCompletionStatus(port, &msg, &key, &ov, INFINITE)`; `msg` é o código
  (`JOB_OBJECT_MSG_NEW_PROCESS = 6`, `EXIT_PROCESS = 7`, `ABNORMAL_EXIT_PROCESS = 8`, `ACTIVE_PROCESS_ZERO = 4`;
  outros ignorados), `ov` carrega o PID. Saída do PID do processo inicial → `GetExitCodeProcess(pi.Process)` e
  `JobEvent{Root: true, ExitCode}` [SES-015]. Envio no canal `events` (buffer 64) com `select` no canal de parada —
  nunca descarta `EventActiveZero`. Parada: `PostQueuedCompletionStatus(port, 0, stopKey = 2, nil)`; a leitora sai
  ao ver `stopKey` e fecha `events`.
- `Close()`: posta a parada, espera a leitora, fecha `job`, `port`, `pi.Process` (e `pi.Thread` se ainda aberto).
  Fechar o handle do job **não** mata os processos (sem `KILL_ON_JOB_CLOSE`); o job continua existindo enquanto
  tiver processos [TRK-004, TRK-007]. Idempotente (`sync.Once`).
- `ActiveProcesses()`: `QueryInformationJobObject(job, JobObjectBasicAccountingInformation)` → `ActiveProcesses`
  [SES-013]. As mensagens IOCP de job não têm entrega garantida (documentação do Windows); a verificação de 5 s é a
  rede de segurança.
- Constantes/estruturas que faltarem em `x/sys/windows` são declaradas localmente no arquivo (`windows.NewLazySystemDLL`
  só se preciso; `QueryInformationJobObject`, `SetInformationJobObject`, `CreateIoCompletionPort`,
  `GetQueuedCompletionStatus`, `PostQueuedCompletionStatus`, `AssignProcessToJobObject`, `IsProcessInJob`,
  `GetProcessHandleCount` — conferir quais já existem).
- Processo console (ex.: `fake-game`) lançado pelo agente `windowsgui` ganha console próprio — comportamento normal de
  jogos de console. Nos testes, o processo de teste já tem console e o filho herda.

### Recuperação (`recover.go`) [SES-010]
```go
type RecoveryStore interface {
    OpenSessions(ctx context.Context) ([]store.OpenSession, error)
    EndSession(ctx context.Context, e store.SessionEnd) (domain.SessionState, error)
}
// RecoverOpenSessions ends every open session at its last_seen_at with its stored active seconds and reason
// recovered. It returns how many were closed; per-session failures are logged and do not stop the others.
func RecoverOpenSessions(ctx context.Context, st RecoveryStore, clock domain.Clock, log *slog.Logger) (int, error)
```
Log `Warn` "sessões abertas recuperadas" (`count`) quando `count > 0`.

## `internal/ipc` [IPC-020, IPC-021, IPC-023..031]

- `protocol.go`: constantes `MethodGameAdd = "game.add"`, `MethodGameList = "game.list"`,
  `MethodGameLaunch = "game.launch"`, `MethodSessionCurrent = "session.current"`,
  `MethodSessionChanged = "session.changed"`; códigos `CodeGameNotFound = -32001`, `CodeSessionActive = -32002`,
  `CodeLaunchFailed = -32003`, `CodeInvalidExecutable = -32004` (mensagens exatamente as do contrato); tipos `Game`,
  `Session` (`EndedAt *time.Time`, `FirstPlayedAt`/`LastPlayedAt *time.Time` → `null`; `ActiveSeconds`, `ActiveMs
  int64`, `AsOf time.Time` serializado em RFC 3339 UTC com milissegundos), `GameAddParams`, `GameLaunchParams`,
  `GameListResult` (`Games []Game` sempre não nulo), `SessionCurrentResult` (`Session *Session`),
  `SessionChangedParams`, `SessionRef`, `LaunchFailedData`, `InvalidExecutableData`, `GameNotFoundData`. Helper
  `NewErrorData(code, message string, data any) *Error`.
- Conversão `domain.SessionSnapshot` → `ipc.Session` num único helper: `ActiveMs = Active.Milliseconds()`,
  `ActiveSeconds = ActiveMs / 1000`, `AsOf = snapshot.AsOf.UTC()` [IPC-031].
- Decodificação de params com membros: `json.Decoder` com `DisallowUnknownFields`, rejeita array e exige os
  obrigatórios (helper `decodeParams(raw, &v) *Error` → `-32602`). `gameId` como `json.Number` para recusar
  fracionário [IPC-028].
- `Server.Notify(method string, params any)`: serializa uma vez e enfileira sem bloquear em cada conexão aberta
  (`c.out`), como `broadcastHeartbeat` (que passa a usá-lo); fila cheia → `Warn` "notificação descartada: fila da
  conexão cheia" (`method`, `remote`) [IPC-029]. Seguro de chamar de qualquer goroutine, antes do `Serve` (sem
  conexões, no-op) e depois do `Close` (no-op).
- Comentário do pacote passa a apontar para `docs/specs/phase-3-tracker/contracts/ipc.openrpc.json`;
  `contract_test.go` passa a ler esse arquivo e cobre também os campos de `Game`/`Session` e os códigos novos
  [IPC-030]. O arquivo da single-exe fica como histórico.

## `internal/agent` [AGT-009..011, GAM-*, IPC-020..028]

### Handlers (`games.go`)
```go
type gameStore interface {
    AddGame(ctx context.Context, g store.NewGame) (store.Game, error)
    Games(ctx context.Context) ([]store.Game, error)
    Game(ctx context.Context, id int64) (store.Game, error)
}
type sessionTracker interface {
    Launch(ctx context.Context, spec tracker.LaunchSpec) (domain.SessionSnapshot, error)
    Current() (domain.SessionSnapshot, bool)
}
func gameHandlers(st gameStore, tr sessionTracker, clock domain.Clock, stat func(string) (fs.FileInfo, error),
    log *slog.Logger) map[string]ipc.Handler
```
- `game.add`: `validateExe(path, stat)` → motivo de GAM-002 (`filepath.IsAbs`; `stat` → `os.ErrNotExist` =
  `not-found`; `!Mode().IsRegular()` = `not-a-file`; `strings.EqualFold(filepath.Ext(p), ".exe")`), título de GAM-003
  (`strings.TrimSpace`, `utf8.RuneCountInString > 200` → `-32602`, vazio → `domain.DefaultGameTitle`) →
  `store.AddGame{At: clock.Now()}` → `ipc.Game` [IPC-020, IPC-021].
- `game.list` → `ipc.GameListResult` [IPC-023].
- `game.launch`: `store.Game(id)` (`ErrGameNotFound` → `-32001`) → `tracker.Launch` com `LaunchSpec` montado do jogo;
  `ErrSessionActive` → `-32002` com `SessionRef`; `LaunchError`/`ErrShuttingDown` → `-32003` (`error`, `winError`
  quando houver; `ErrShuttingDown` → `"agente encerrando"`). `launch_type ≠ 'exe'` → `-32003` "tipo de lançamento não
  suportado" [IPC-024, IPC-025, AGT-011]. Timeout do handler: 10 s (`context.WithTimeout`).
- `session.current` → `tracker.Current()` [IPC-026].
- Os handlers são registrados ao lado de `ipc.AgentHandlers` no `NewServer`.

### Subida e shutdown [AGT-009, AGT-010, AGT-011]
- `startAgent`: `store.Open(Migrate)` → **`tracker.RecoverOpenSessions`** (erro → `Error`, segue) →
  `tracker.New(Config{Notify: notifier.Notify, ...})` + `Start()` → `ipc.NewServer(..., handlers)` → `notifier` passa
  a apontar para o servidor → `Serve`. O `notifier` é um adaptador (`atomic.Pointer[ipc.Server]`) que resolve a
  dependência circular tracker ↔ servidor; antes de apontar, `Notify` é no-op (não há conexões ainda).
- `instance.Shutdown(ctx, reason)`: `tracker.BeginShutdown()` → `Drain` (≤ 500 ms) → `Server.Close` → `wg.Wait` →
  **`tracker.Close(ctx)`** → `store.Close`. O tracker fecha depois do servidor para que nenhum `game.launch` chegue
  durante o encerramento da sessão, e antes do store porque grava o fim [AGT-010, SES-011].
- Orçamento: `Drain` ≤ 500 ms + uma escrita de fim (≤ 2 s de timeout, normalmente < 50 ms) dentro dos 2 s do AGT-003.
  No `WM_ENDSESSION` o handler espera até 2 s (fase 2), o mesmo orçamento.

## `cmd/arcadia` (UI, Go) [DSK-010..012]

```go
type GameView struct {
    ID int64 `json:"id"`; Title string `json:"title"`; Path string `json:"path"`
    TotalPlaySeconds int64 `json:"totalPlaySeconds"`; PlayCount int64 `json:"playCount"`
    LastPlayedAt string `json:"lastPlayedAt"` // RFC 3339 or ""
}
type SessionView struct {
    SessionID int64 `json:"sessionId"`; GameID int64 `json:"gameId"`; Title string `json:"title"`
    State string `json:"state"`; StartedAt string `json:"startedAt"`; EndedAt string `json:"endedAt"`
    ActiveSeconds int64 `json:"activeSeconds"`; ActiveMs int64 `json:"activeMs"`
    AsOf string `json:"asOf"` // RFC 3339 UTC with ms, agent clock [DSK-010, IPC-031]
}
type PickedExe struct{ Path string `json:"path"`; Title string `json:"title"` }

func (a *App) PickGameExe() (PickedExe, error)                // [DSK-012]
func (a *App) AddGame(path, title string) (GameView, error)   // timeout 5 s
func (a *App) ListGames() ([]GameView, error)                 // timeout 2 s; [] nunca nil
func (a *App) LaunchGame(id int64) (SessionView, error)       // timeout 12 s (> 10 s do handler)
func (a *App) CurrentSession() (*SessionView, error)          // timeout 2 s; nil → null
```
- `PickGameExe` usa `platform.openFile` injetável (real: `wailsruntime.OpenFileDialog` com
  `Title: "Escolha o executável do jogo"`, `Filters: [{DisplayName: "Executáveis (*.exe)", Pattern: "*.exe"}]`);
  `""` = cancelado [DSK-012].
- `userError(err error, log) error` (`errors.go`, puro): `errors.As(err, *ipc.Error)` → texto da tabela de DSK-011
  usando `Data` (`title`, `error`, `reason`); `ipc.ErrConnectionLost`, `ipc.ErrClientClosed`,
  `context.DeadlineExceeded`, sem cliente → "O agente não está respondendo. Tente de novo."; resto → "Erro inesperado.
  Veja o log." + log `Error`. Os bindings devolvem `errors.New(texto)` — o Wails rejeita a Promise com a string, que o
  frontend mostra como está [DSK-011, UI-008].
- Supervisor: `handleNote` ganha `case ipc.MethodSessionChanged` → valida e converte → `Emit("session:changed",
  SessionView)`; params inválidos → `Warn` "session.changed com parâmetros inválidos" [DSK-010]. Nada mais muda na
  tabela de transições do supervisor.
- `LaunchGame` com timeout no cliente: o agente pode ter lançado mesmo assim; a UI se acerta pelo `session:changed`.

## `frontend/` [UI-004..018]

- `lib/games.ts`: tipos `GameView`/`SessionView`; `formatPlayTime(seconds)` puro [UI-005]; hooks TanStack Query:
  `useGames()` (`ListGames`; `retry: false`; `refetchInterval` 2 s só em erro) e `useCurrentSession()`
  (`CurrentSession`, sem polling); `useSessionEvents()` assina `session:changed`: `running`/`grace` →
  `setQueryData(current, s)`; `ended`/`discarded` → `setQueryData(current, null)` + `invalidateQueries(games)`
  [UI-006]. `useReloadOnReconnect(status)` invalida games e current quando `connected` passa de `false` para `true`
  [UI-009].
- **Evento vence resposta atrasada** [UI-018]: `useSessionEvents` incrementa um contador `eventSeq` a cada
  `session:changed` aplicado; o `queryFn` de `useCurrentSession` guarda o `eventSeq` antes de chamar
  `CurrentSession` e, se ele mudou quando a resposta chega, devolve o valor já em cache em vez da resposta. Os eventos
  chegam em ordem (uma conexão, IPC-027), então não é preciso comparar `asOf` entre fontes — e comparar instantes de
  parede quebraria se o relógio do Windows fosse ajustado para trás durante a sessão.

### Cronômetro ao vivo [UI-012..UI-017]
- **Âncora.** Cada `SessionView` aplicado (de `CurrentSession`, `LaunchGame` ou `session:changed`) vira uma âncora
  `{activeMs, state, receivedAt}`, com `receivedAt = now()` lido do relógio monotônico da UI (`performance.now()`,
  injetável para testes). O valor exibido é `floor((activeMs + (now() − receivedAt)) / 1000)` em `running` e
  `floor(activeMs / 1000)` em `grace` [UI-014, UI-015].
- **Por que não `startedAt` nem `asOf` contra o relógio da UI.** `startedAt` é instante de parede: `agora − startedAt`
  incluiria a carência (P4) e, na fase 4, as pausas, e mudaria com ajustes do relógio do Windows. `asOf` também é de
  parede (o relógio do agente); `uiNow − asOf` depende de os dois processos lerem o mesmo relógio de parede de forma
  coerente (fuso, ajuste de NTP ou manual entre a medição e a leitura, `Date` falso em teste). A âncora usa só a
  **diferença** no relógio monotônico da própria UI, que não sofre esses efeitos; o erro fica limitado à latência
  agente → UI (ms). `asOf` vai no contrato para tornar a âncora explícita e verificável (testes IPC-031, logs e
  diagnóstico), não para o cálculo.
- `lib/sessionClock.ts`: `formatElapsed(seconds): string` puro (`H:MM:SS`) [UI-013]; `useElapsedSeconds(anchor,
  now = () => performance.now())` agenda um `setTimeout` para o próximo limite de segundo
  (`1000 − ((activeMs + elapsed) mod 1000)` ms) e rearma a cada disparo, só em `running`; em `grace`, sem anchor ou ao
  desmontar, não há temporizador (limpo no cleanup do `useEffect`). Nenhum binding é chamado pelo cronômetro [UI-017].
- `SessionPanel` (`role="status"`, `aria-label="Sessão em andamento"`): título, `formatElapsed(...)` e, em `grace`,
  "Encerrando…". Renderizado por `App` só quando `connected && current != null` (status do `AgentIndicator`):
  desconectado → painel some; ao reconectar, a recarga de UI-009 cria uma âncora nova [UI-012, UI-016]. Escolha de
  esconder (e não congelar) na desconexão: sem agente a UI não sabe se a sessão continua (o agente pode ter caído, e
  o tempo depois do último heartbeat não conta, Limitações 5); mostrar um valor parado ou avançando sugeriria um tempo
  que talvez não seja gravado. O indicador "Desconectado" já explica a ausência.
- O total da lista continua vindo só de `game.list` (recarregado em `ended`/`discarded`); o cronômetro não o altera
  [UI-017].
- Testes: `vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout', 'setInterval', 'clearInterval', 'Date',
  'performance'] })` ou o `now` injetado; `vi.advanceTimersByTime` para os exemplos de UI-014/UI-015; contagem de
  chamadas dos bindings falsos para UI-017.
- `App.tsx`: cabeçalho (título, botão "Adicionar jogo", `AgentIndicator`), `ErrorBanner`, `AddGameForm` (quando há
  exe escolhido) e `GameList`. Estado de erro e de "Iniciando…" em `useState` local (Zustand não é necessário).
- `GameList`: linhas com título, "Em execução" quando `current?.gameId === game.id`, `formatPlayTime`, botão "Jogar"
  (desabilitado com "Iniciando…" enquanto o `LaunchGame` daquele jogo está pendente) [UI-004, UI-006, UI-008]. A
  sessão devolvida por `LaunchGame` só é aplicada ao cache se nenhum `session:changed` chegou desde o pedido (mesma
  regra de UI-018).
- `AddGameForm`: caminho (texto somente leitura), input "Título" (`maxLength={200}`), "Salvar" desabilitado com
  `title.trim() === ''` ou envio pendente, "Cancelar" [UI-007].
- Estado vazio: `games.length === 0` → "Nenhum jogo ainda"; sem dados (carregando/erro) → "Conectando ao agente…"
  [UI-010]. O `LibraryEmpty` e o `useLibrarySummary` saem da tela (o componente e seu teste podem ser removidos ou
  ficar sem uso — decisão do planner; UI-002 está substituído).
- Testes Vitest + Testing Library com `window.go.main.App` e `window.runtime.EventsOn` falsos (como hoje).

## `cmd/fake-game` [FKG-001..008]
Implementa [contracts/fake-game.md](contracts/fake-game.md). `main.go` (`//go:build windows`) + `main_other.go`.
Filhos criados com `windows.CreateProcess` (não `os/exec`, para poder passar `CREATE_BREAKAWAY_FROM_JOB`) com handles
fechados logo após a criação; o pai não espera os filhos. Parse com `flag.NewFlagSet(ContinueOnError)` + validação
própria; `forever` tratado antes de `time.ParseDuration`. Testes do próprio fake-game (FKG-*) rodam o binário compilado
e leem o relatório.

## Testes
- **Pura** (`machine_test.go`, qualquer SO): uma linha da tabela de transições por caso, com IDs SES-*. Exemplo
  SES-006 com instantes fixos.
- **Coordenador** (`tracker_test.go`, qualquer SO): `FakeClock`, `fakeLauncher`/`fakeJob` (canal de eventos
  controlado pelo teste, `ActiveProcesses` programável), `fakeStore` (registra chamadas, injeta erros), `Notify`
  gravando snapshots. Sempre `BlockUntilWaiters` antes de `Advance` (waiters por estado: ver
  [máquina](contracts/session-state-machine.md#transições)). `goleak` no `Close` [TRK-008].
- **Integração Windows** (`job_windows_test.go`, `integration_windows_test.go`): `TestMain` compila `cmd/fake-game`;
  `Launcher` real + `FakeClock` para carência/heartbeat + `store` real em `t.TempDir()`; espera de eventos com prazo
  real ≤ 10 s; `t.Cleanup` cria o `--until-file`. TRK-007 com `windows.GetProcessHandleCount(CurrentProcess)`.
- **Store** (`games_test.go`, `sessions_test.go`): SQLite em `t.TempDir()`, table-driven.
- **Agent** (`games_test.go`): handlers com `gameStore`/`sessionTracker`/`stat` falsos, via `ipc.Dispatch`.
- **IPC** (`notify_test.go`, `contract_test.go`): `net.Pipe`/TCP local como hoje.
- **UI Go**: `userError` table-driven; bindings com `pinger` falso; supervisor com `session.changed`.
- **Frontend**: Vitest para `formatPlayTime`, `formatElapsed`, `GameList`, `AddGameForm`, `ErrorBanner`,
  `SessionPanel` (temporizadores falsos), eventos, recarga e resposta atrasada.

## Riscos
- **Goroutines novas e donos:** coordenador do tracker (dono: `Tracker.wg`, sai com `stop` no `Close`); leitora da
  IOCP por job (dona: o `job`, sai com o pacote `stopKey` postado no `Close`, que espera por ela). Nenhum `go func()`
  solto; `Launch` não cria goroutine.
- **Handles:** job, porta, `hProcess` e `hThread` do processo inicial, todos donos do `job` e fechados no `Close`
  (`hThread` logo após o `Resume`); caminhos de erro do `Prepare` fecham tudo que criaram antes de retornar. Nenhum
  handle sai do pacote fora de uma struct com `Close()`. TRK-007 mede.
- **Bloqueio da leitora:** se a goroutine dona parar de ler `Events()` (ex.: escrita lenta no banco), a leitora espera
  no envio; a IOCP acumula pacotes no kernel sem perda. O `Close` destrava pelo canal de parada.
- **Despacho sequencial do IPC** (decisão da fase 2): `game.launch` ocupa a conexão da UI enquanto o `CreateProcess`
  roda (normalmente < 100 ms; com antivírus, segundos). O `AgentStatus` da UI tem timeout de 500 ms e pode mostrar
  "Desconectado" por um instante. Aceito nesta fase (Limitações 10); se for incômodo, a evolução é despachar cada
  requisição em goroutine própria com limite por conexão (muda IPC, exige requisito).
- **`ActiveProcesses` e a mensagem `ACTIVE_PROCESS_ZERO` chegando fora de ordem com `NEW_PROCESS`:** a máquina só
  entra em Grace por `TrackedEmpty` e sai por `ProcessAdded`; uma mensagem tardia de processo novo depois do vazio
  (impossível no kernel, mas possível na entrega) devolve a Running — exatamente o que a carência existe para cobrir.
- **Jobs aninhados:** se o agente estiver dentro de um job (iniciado sem breakaway, DSK-005), o job do jogo fica
  aninhado (Windows 8+). Funciona; o job externo pode ter `KILL_ON_JOB_CLOSE` (terminal/IDE) e matar o jogo junto com
  o terminal — só em desenvolvimento.
- **`JOB_OBJECT_LIMIT_BREAKAWAY_OK`** (ADR 0016): troca rastreamento por compatibilidade; a Camada 2 (fase 6)
  recupera os filhos que escapam.
- **Cronômetro da UI:** temporizador por componente montado (um `setTimeout` por vez, limpo no cleanup); com a janela
  minimizada o WebView pode atrasar temporizadores, mas o valor é recalculado da âncora a cada disparo, sem acumular
  erro.
- **Relógio:** durações em memória usam a leitura monotônica do `time.Time` (via `SystemClock`); instantes gravados são
  de parede (ADR 0009). Ajuste do relógio durante a sessão pode deixar `ended_at − started_at ≠ active_seconds`;
  `active_seconds` é o valor confiável.
- **`FakeClock` e temporizadores não canceláveis:** waiters de geração antiga continuam pendentes até dispararem; os
  testes contam com isso (tabela de waiters na máquina de estados).
- **Compatibilidade de schema:** UI e agente são o mesmo binário (`SchemaVersion = 3` nos dois). Um agente v2 antigo
  rodando com UI nova: a UI não abre mais o banco para a tela, e os métodos novos dão `-32601` → DSK-011 "Erro
  inesperado" até o agente ser reiniciado.
