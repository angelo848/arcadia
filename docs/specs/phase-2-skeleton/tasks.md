# Fase 2 — Esqueleto: tarefas

- **Status:** aprovado (2026-11-08) — lacunas 1–4 abaixo confirmadas pelo usuário em 2026-11-08 (refletidas no design.md)
- **Spec:** [requirements.md](requirements.md) · [design.md](design.md) · [contracts/ipc.openrpc.json](contracts/ipc.openrpc.json) (v0.2.0)
- **ADRs:** 0003, 0004, 0006, 0009 (instantes em ms Unix UTC), 0010 (só o agente migra), 0011 (Clock com temporizadores)

## Resumo
Treze tarefas em ordem de dependência: módulo/build sem Node (T01) → pacotes portáveis `domain`, `paths`, `store`,
`logging` (T02–T05) → `ipc` em três camadas: dispatch, servidor/cliente, pipe Windows (T06–T08) → `winapi`: mutex,
fim de sessão, `CreateDetached` (T09) → `cmd/agent`, `frontend`, `cmd/desktop` (T10–T12) → verificação manual (T13).
`internal/tracker/` não entra nesta fase. **Cada tarefa roda o ciclo test-writer → implementer → reviewer antes da
próxima**: o Stop hook do implementer roda `go test ./...` inteiro, e testes de tarefas futuras o bloqueariam.

**Premissas:** módulo `github.com/arcadia-launcher/arcadia` (provisório) · Windows 11, Go 1.27.3, Wails CLI v2.17.1 ·
`CGO_ENABLED=0`, logo sem `go test -race` · testes de `cmd/*` e `internal/winapi` rodam no Windows (`//go:build windows`
onde houver Win32) · o test-writer só começa depois de T01 criar o `go.mod`.

## Detalhes confirmados (antes lacunas)
1. **`startedAt` do heartbeat:** `NewServer` ganha a opção variádica `WithStartedAt(t)` (padrão `clock.Now()` em `NewServer`);
   `agent.status` e heartbeat usam o mesmo instante.
2. **Fim de shutdown no `SessionEndWatcher`:** método `ShutdownDone()`; timeout de 2 s via `domain.Clock`. O watcher é
   fechado **depois** de IPC e store (não logo após a bandeja), senão o Windows mata o processo no meio do shutdown.
   · alterado em 2026-11-08: o `Close` do log passou para o último passo, depois de `mutex.Close` (revisão da T05; ver
   design.md, `cmd/agent` e `internal/logging`).
3. **UI e `--data-dir`:** a UI resolve com o mesmo `paths.Resolve` (flag `--data-dir`, env, pasta do exe) e loga em `info` fixo.
4. **`ParseLevel`:** fica em `internal/logging` (compartilhado) em vez de `parseLogLevel` em `cmd/agent`.

---

## T01 — Módulo, layout e build sem Node/CGO
- Requisitos: BLD-001
- Responsável: implementer · Testes: — (verificação manual em T13)
- Arquivos: `go.mod` (novo, `go 1.27`), `.gitignore` (novo: `build/bin/`, `frontend/node_modules/`, `frontend/dist/*` +
  `!frontend/dist/.gitkeep`, `.devdata/`, `*.log`), `frontend/dist/.gitkeep`, `frontend/embed.go`
- Assinaturas públicas:
  ```go
  package frontend
  //go:embed all:dist
  var Assets embed.FS
  ```
- Testes: nenhum automatizado. Verificação: `$env:CGO_ENABLED='0'; go build ./...` e `go test ./...` passam.
- Dependências: —
- Depende de: —

## T02 — Domain: relógio com temporizadores e tempo ativo
- Requisitos: DOM-001, DOM-002, DOM-003, DOM-004, DOM-005, DOM-006
- Responsável: implementer · Testes: test-writer
- Arquivos: `internal/domain/clock.go`, `internal/domain/session_time.go`; testes `clock_test.go`, `session_time_test.go`
- Assinaturas públicas:
  ```go
  package domain
  type Clock interface {
      Now() time.Time
      After(d time.Duration) <-chan time.Time
  }
  type SystemClock struct{} // the only place allowed to call time.Now/time.After
  func (SystemClock) Now() time.Time
  func (SystemClock) After(d time.Duration) <-chan time.Time

  type FakeClock struct{ /* mu, cond/signal, now, waiters */ }
  func NewFakeClock(start time.Time) *FakeClock
  func (c *FakeClock) Now() time.Time
  func (c *FakeClock) After(d time.Duration) <-chan time.Time // buffer 1; d<=0 sends Now() immediately, no waiter
  func (c *FakeClock) Advance(d time.Duration)                 // fires (non-blocking) and removes waiters with deadline <= now
  func (c *FakeClock) BlockUntilWaiters(ctx context.Context, n int) error // wakes on registration; returns ctx.Err()

  type Interval struct{ Start, End time.Time } // End.IsZero() == open
  func ActiveDuration(start, end time.Time, pauses []Interval) time.Duration
  ```
- Testes:
  - `TestActiveDuration`: "DOM-001/sem pausas"; "DOM-002/pausa interna", "DOM-002/pausas sobrepostas contam uma vez",
    "DOM-002/pausas fora de ordem", "DOM-002/pausa começa antes do início", "DOM-002/pausa termina depois do fim",
    "DOM-002/pausa totalmente fora", "DOM-002/pausa cobre a sessão inteira = 0", "DOM-002/pausa invertida (End < Start) é ignorada";
    "DOM-003/pausa aberta até o fim", "DOM-003/pausa aberta antes do início = 0"; "DOM-004/fim antes do início = 0"
  - `TestFakeClockAdvance`: "DOM-005/Advance avança exatamente d", "DOM-005/Advance acumulado", "DOM-005/Now e Advance concorrentes"
  - `TestFakeClockTimers`: "DOM-006/After não dispara em d-1ns", "DOM-006/After dispara uma vez em d com Now resultante",
    "DOM-006/After com d<=0 já tem valor", "DOM-006/Advance(2s) dispara 1s e 2s e mantém 5s", "DOM-006/Advance não bloqueia sem leitor",
    "DOM-006/BlockUntilWaiters retorna após registro", "DOM-006/waiters disparados ou d<=0 não contam",
    "DOM-006/BlockUntilWaiters retorna ctx.Err() ao cancelar"
- Dependências: —
- Depende de: T01

## T03 — Paths: resolução e criação de pastas
- Requisitos: PTH-001, PTH-002, PTH-003
- Responsável: implementer · Testes: test-writer
- Arquivos: `internal/paths/paths.go`; teste `internal/paths/paths_test.go`
- Assinaturas públicas:
  ```go
  package paths
  const EnvDataDir = "ARCADIA_DATA_DIR"
  type Options struct {
      Flag   string
      ExeDir string
      Getenv func(string) string // nil => os.Getenv
  }
  type Paths struct{ Root, DB, Media, Logs string } // DB=Root/library.db, Media=Root/media, Logs=Root/logs
  func Resolve(opts Options) (Paths, error)          // error if Flag, env and ExeDir are all empty
  func (p Paths) EnsureDirs() error                  // creates Root, Media, Logs
  ```
- Testes:
  - `TestResolve`: "PTH-001/flag vence env", "PTH-001/env vence exe", "PTH-001/sem flag nem env usa exe/data",
    "PTH-001/env vazio é ignorado", "PTH-001/tudo vazio retorna erro", "PTH-002/flag relativa vira absoluta",
    "PTH-002/env relativo vira absoluto", "PTH-002/DB Media Logs absolutos dentro de Root"
  - `TestEnsureDirs`: "PTH-003/cria pastas inexistentes", "PTH-003/segunda chamada não falha", "PTH-003/Root é arquivo retorna erro"
- Dependências: —
- Depende de: T01

## T04 — Store: abertura com/sem migração, schema, tempo e sqlc
- Requisitos: STO-001, STO-002, STO-003, STO-004, STO-005, STO-006, STO-007, STO-008, STO-009, STO-010
- Responsável: implementer · Testes: test-writer
- Arquivos: `internal/store/store.go`, `migrate.go`, `timeconv.go`; `internal/store/migrations/00001_initial.sql` (§5, todo
  `*_at` INTEGER, `release_date` TEXT, FTS5, seeds de `play_mode`, índices `game(platform_id)` e `play_session(game_id, started_at)`);
  `internal/store/queries/library.sql` (`CountGames :one`, `ListPlatforms :many`, `InsertPlatform :one ... RETURNING id`);
  `sqlc.yaml` (raiz); `internal/store/dbgen/*` (gerado, versionado); testes `store_test.go`, `timeconv_test.go`
- Assinaturas públicas:
  ```go
  package store
  // DSN: file:<path>?mode=rw[c]&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)
  //      &_pragma=synchronous(NORMAL)&_txlock=immediate   (mode=rw, no create, when Migrate=false)
  const SchemaVersion int64 = 1
  var ErrSchemaMismatch = errors.New("store: schema version mismatch")
  type OpenOptions struct{ Migrate bool } // true only in the agent (ADR 0010)
  type Store struct{ /* db *sql.DB, q *dbgen.Queries */ }
  type LibrarySummary struct{ Games, Platforms int64 }
  func Open(ctx context.Context, path string, opts OpenOptions) (*Store, error)
  func (s *Store) Close() error
  func (s *Store) DB() *sql.DB
  func (s *Store) Queries() *dbgen.Queries
  func (s *Store) Tx(ctx context.Context, fn func(q *dbgen.Queries) error) error
  func (s *Store) CurrentVersion(ctx context.Context) (int64, error) // goose version, read-only
  func (s *Store) LibrarySummary(ctx context.Context) (LibrarySummary, error)
  func TimeToMillis(t time.Time) int64  // t.UnixMilli()
  func MillisToTime(ms int64) time.Time // time.UnixMilli(ms).UTC()

  package dbgen // generated by sqlc (expected shape)
  func (q *Queries) CountGames(ctx context.Context) (int64, error)
  func (q *Queries) ListPlatforms(ctx context.Context) ([]Platform, error)
  type InsertPlatformParams struct{ Name string; Category string }
  func (q *Queries) InsertPlatform(ctx context.Context, arg InsertPlatformParams) (int64, error)
  ```
  Caminho `Migrate: false`: `os.Stat` antes de abrir (inexistente → `ErrSchemaMismatch` sem criar arquivo); versão lida sem
  escrita (nada de `goose.EnsureDBVersion`); versão ≠ `SchemaVersion` → fecha o `*sql.DB` e retorna `%w` de `ErrSchemaMismatch`
  com as duas versões na mensagem.
- Testes (`t.TempDir()`):
  - "STO-001/tabelas do §5 existem", "STO-001/colunas do §5 existem", "STO-001/game_fts é FTS5 com title, description, developer, publisher",
    "STO-001/índices criados", "STO-001/journal_mode é wal"
  - "STO-002/reabrir com migração mantém a versão", "STO-002/goose_db_version não ganha linhas", "STO-002/dados permanecem"
  - "STO-003/seeds presentes" (conjunto exato dos 5)
  - "STO-004/game com platform_id inexistente falha", "STO-004/FK ativa em todas as conexões do pool"
  - "STO-005/dois Store, escrita e leitura paralelas sem SQLITE_BUSY"
  - "STO-006/toda coluna _at é INTEGER", "STO-006/release_date é TEXT"
  - "STO-007/grava UnixMilli de instante não-UTC", "STO-007/lê truncado em ms e em UTC", "STO-007/release_date YYYY-MM-DD idêntico"
  - "STO-008/abre sem migrar banco atual e consulta"
  - "STO-009/arquivo inexistente", "STO-009/banco sem goose_db_version", "STO-009/versão menor", "STO-009/versão maior"
    (todos com `errors.Is(err, ErrSchemaMismatch)`)
  - "STO-010/arquivo inexistente não é criado", "STO-010/versão e tabelas inalteradas após mismatch"
- Dependências: `modernc.org/sqlite` (última estável v1.x) · `github.com/pressly/goose/v3` (última estável) ·
  CLI `sqlc` pelo binário pré-compilado de Windows (o `go install` exige CGO)
- Depende de: T01

## T05 — Logging compartilhado
- Requisitos: AGT-005, DSK-009
- Responsável: implementer · Testes: test-writer
- Arquivos: `internal/logging/logging.go`; teste `internal/logging/logging_test.go` (caixa-branca permitida para o rotator)
- Assinaturas públicas:
  ```go
  package logging
  const (
      MaxSizeMB  = 10
      MaxBackups = 3
  )
  type Options struct {
      Dir   string // Paths.Logs
      File  string // "agent.log" | "desktop.log"
      Level slog.Level
  }
  func New(opts Options) (*slog.Logger, io.Closer, error) // slog JSON handler over lumberjack
  func ParseLevel(s string) (slog.Level, error)          // ""=>Info; debug|info|warn|error; else error
  ```
- Testes:
  - `TestParseLevel`: "AGT-005/debug info warn error mapeiam para slog", "AGT-005/vazio é info", "AGT-005/valor inválido retorna erro"
  - `TestNew`: "DSK-009/grava JSON em Dir/File", "DSK-009/rotação 10 MB × 3", "DSK-009/nível filtra mensagens abaixo"
- Dependências: `gopkg.in/natefinch/lumberjack.v2` v2.2.1
- Depende de: T01

## T06 — IPC: tipos do contrato, codec NDJSON e dispatch
- Requisitos: IPC-001, IPC-002, IPC-008, IPC-009, IPC-010, IPC-011, IPC-012, AGT-004
- Responsável: implementer · Testes: test-writer
- Arquivos: `internal/ipc/protocol.go`, `codec.go`, `dispatch.go`, `agent_methods.go`; testes `codec_test.go`, `dispatch_test.go`, `contract_test.go`
- Assinaturas públicas:
  ```go
  package ipc
  const (
      Version            = "2.0"
      MaxMessageSize     = 1 << 20 // excluding '\n'
      HeartbeatInterval  = 5 * time.Second
      NotificationBuffer = 16
      MethodPing, MethodStatus, MethodHeartbeat = "agent.ping", "agent.status", "agent.heartbeat"
      CodeParseError, CodeInvalidRequest, CodeMethodNotFound, CodeInvalidParams, CodeInternalError = -32700, -32600, -32601, -32602, -32603
  )
  type Request struct {
      JSONRPC string          `json:"jsonrpc"`
      ID      json.RawMessage `json:"id,omitempty"`
      Method  string          `json:"method"`
      Params  json.RawMessage `json:"params,omitempty"`
  }
  type Response struct {
      JSONRPC string          `json:"jsonrpc"`
      ID      json.RawMessage `json:"id"`
      Result  json.RawMessage `json:"result,omitempty"`
      Error   *Error          `json:"error,omitempty"`
  }
  type Error struct {
      Code    int             `json:"code"`
      Message string          `json:"message"`
      Data    json.RawMessage `json:"data,omitempty"`
  }
  func (e *Error) Error() string
  func NewError(code int, message string) *Error
  type Notification struct {
      Method string          `json:"method"`
      Params json.RawMessage `json:"params,omitempty"`
  }
  type PingResult struct{ Pong bool `json:"pong"` }
  type AgentStatus struct {
      Version       string    `json:"version"`
      PID           int       `json:"pid"`
      StartedAt     time.Time `json:"startedAt"` // RFC 3339, UTC
      DataDir       string    `json:"dataDir"`
      UptimeSeconds int64     `json:"uptimeSeconds"`
  }
  type HeartbeatParams struct{ UptimeSeconds int64 `json:"uptimeSeconds"` }

  var ErrMessageTooLarge = errors.New("ipc: message exceeds 1 MiB")
  type Decoder struct{ /* bufio.Reader */ }
  func NewDecoder(r io.Reader) *Decoder
  func (d *Decoder) Next() ([]byte, error) // line without '\n'; ErrMessageTooLarge if > MaxMessageSize
  type Encoder struct{ /* mu, w */ }
  func NewEncoder(w io.Writer) *Encoder
  func (e *Encoder) Encode(v any) error // json + '\n', concurrency-safe

  type Handler func(ctx context.Context, params json.RawMessage) (any, error) // *Error propagates its code
  // Dispatch validates in design order (parse -> batch -> request -> notification -> method -> params).
  func Dispatch(ctx context.Context, handlers map[string]Handler, msg []byte) (resp []byte, ok bool) // ok=false: no response

  type AgentInfo struct {
      Version   string
      PID       int
      StartedAt time.Time
      DataDir   string
  }
  func AgentHandlers(info AgentInfo, clock domain.Clock) map[string]Handler // agent.ping, agent.status
  ```
- Testes:
  - `TestDispatch`: "IPC-001/ping retorna exatamente {pong:true}", "IPC-001/status calcula uptime pelo relógio",
    "IPC-002/linha não-JSON retorna -32700 id null", "IPC-002/método inexistente retorna -32601", "IPC-002/params [1] retorna -32602",
    "IPC-002/params {a:1} retorna -32602", "IPC-008/params ausente", "IPC-008/params null", "IPC-008/params []", "IPC-008/params {}",
    "IPC-009/params string", "IPC-009/params número", "IPC-009/params bool", "IPC-009/jsonrpc ausente", "IPC-009/jsonrpc 1.0",
    "IPC-010/batch vazio retorna um -32600 id null", "IPC-010/batch com itens retorna um -32600 id null",
    "IPC-011/notificação para método existente sem resposta", "IPC-011/notificação para método inexistente sem resposta",
    "AGT-004/status devolve AgentInfo.Version"
  - `TestContract` (lê `contracts/ipc.openrpc.json`): "IPC-001/status tem exatamente os campos do schema",
    "IPC-001/startedAt RFC 3339 em UTC", "IPC-002/códigos batem com components.errors", "IPC-012/limite bate com x-limits.maxMessageBytes"
  - Acrescentados em 2026-11-08 (lacunas e revisão da T06): `TestDispatchHandlerErrors`, `TestDispatchErrorMessages`,
    `TestEncoder` e casos extras de `TestDispatch`/`TestDecoder` (`id: null` explícito, `id` de tipo inválido, CRLF,
    linhas vazias, EOF sem `\n`, memória limitada, erro preso, uptime nunca negativo).
  - `TestDecoder`: "IPC-012/exatamente 1.048.576 bytes aceito", "IPC-003/1.048.577 bytes retorna ErrMessageTooLarge",
    "IPC-012/várias mensagens no stream"
- Dependências: —
- Depende de: T02

## T07 — IPC: servidor, cliente, heartbeat e reconexão
- Requisitos: IPC-001, IPC-003, IPC-004, IPC-005, IPC-006, IPC-010, IPC-011, IPC-012, IPC-013, IPC-014, IPC-015, IPC-016, IPC-017
- Responsável: implementer · Testes: test-writer
- Arquivos: `internal/ipc/server.go`, `internal/ipc/client.go`; testes `server_test.go`, `client_test.go`,
  `helpers_test.go` (listener em memória/`net.Pipe`, servidor bruto fake)
- Assinaturas públicas:
  ```go
  package ipc
  type ServerOption func(*serverConfig) // · alterado em 2026-11-08: alinhado ao design.md
  func WithStartedAt(t time.Time) ServerOption // default: clock.Now() at NewServer (lacuna 1)
  type Server struct{ /* ... */ }
  func NewServer(clock domain.Clock, handlers map[string]Handler, log *slog.Logger, opts ...ServerOption) *Server
  func (s *Server) StartedAt() time.Time
  func (s *Server) Serve(ctx context.Context, ln net.Listener) error // accept + one heartbeat loop broadcasting to all conns
  func (s *Server) Close() error                                     // closes ln and conns; waits accept, conns, heartbeat

  var (
      ErrConnectionLost = errors.New("ipc: connection lost")
      ErrClientClosed   = errors.New("ipc: client closed")
  )
  type Dialer func(ctx context.Context) (net.Conn, error)
  type Client struct{ /* ... */ }
  func NewClient(dial Dialer, clock domain.Clock, log *slog.Logger) *Client // starts dial/read/backoff goroutine
  func (c *Client) Call(ctx context.Context, method string, params, result any) error
  func (c *Client) Notifications() <-chan Notification // buffer 16, non-blocking send, closed by Close
  func (c *Client) Close() error                       // idempotent; returns after goroutine exits
  ```
- Testes (`FakeClock` com `BlockUntilWaiters` antes de cada `Advance`; relógios separados para servidor e cliente):
  - `TestServerClient`: "IPC-001/ping e status ponta a ponta", "IPC-010/conexão segue utilizável após batch",
    "IPC-011/chamada com id após notificação é respondida", "IPC-012/mensagem de 1 MiB exato respondida ponta a ponta"
  - `TestServerLimits`: "IPC-003/mensagem acima do limite fecha só aquela conexão", "IPC-003/outro cliente continua respondendo",
    "IPC-003/nova conexão é aceita"
  - `TestHeartbeat`: "IPC-004/nada antes de 5 s", "IPC-004/heartbeat em 5 s com uptimeSeconds", "IPC-004/repete a cada 5 s para todas as conexões"
  - `TestReconnect`: "IPC-005/servidor reinicia e cliente volta a responder"
  - `TestBackoff`: "IPC-013/esperas 250ms 500ms 1s 2s 4s 5s 5s", "IPC-013/sem jitter (nada dispara 1ns antes)",
    "IPC-013/volta a 250ms após conexão e nova queda"
  - `TestCallWaits`: "IPC-014/Call enviada quando o servidor sobe antes do prazo", "IPC-014/ctx expira antes de conectar retorna errors.Is ctx.Err()"
  - `TestInFlight`: "IPC-015/queda com chamada pendente retorna ErrConnectionLost"
  - `TestAfterClose`: "IPC-016/Call após Close retorna ErrClientClosed"
  - `TestNotificationBuffer`: "IPC-017/canal contém as 16 primeiras", "IPC-017/Call segue respondida na mesma conexão"
  - `TestShutdown` (sem `t.Parallel`, `goleak.VerifyNone`): "IPC-006/Server.Close aguarda goroutines", "IPC-006/Client.Close aguarda goroutines",
    "IPC-006/nenhuma goroutine sobra com conexões abertas", "IPC-006/Close idempotente"
  - · alterado em 2026-11-08 (revisão da T07), testes extras:
    `TestServerHandlerError` "IPC-002/erro genérico de handler é logado e vira -32603 Internal error" (confere nível `Error`);
    `TestHeartbeat` "IPC-004/conexão que não lê não atrasa as outras nem trava o Close";
    `TestServerLimits` confere log `Warn` na mensagem grande;
    `TestCallWaits` "IPC-014/servidor que não lê: Call retorna errors.Is ctx.Err() no prazo";
    `TestServerClose` "IPC-006/Close antes de Serve faz Serve retornar nil";
    `TestBackoff` "IPC-013/após queda espera 250ms antes do redial";
    `TestDialer` "IPC-013/Dialer (nil, nil) é tratado como falha".
- Dependências: `go.uber.org/goleak` v1.3.0 (só teste)
- Depende de: T06

## T08 — IPC: named pipe do usuário (Windows)
- Requisitos: IPC-007; IPC-018, IPC-019 (· alterado em 2026-11-08, revisão da T08)
- Responsável: implementer · Testes: test-writer
- Arquivos: `internal/ipc/pipe.go` (portável), `internal/ipc/pipe_windows.go`; testes `pipe_test.go` (portável:
  `PipeName`), `pipe_windows_test.go` · alterado em 2026-11-08: incluído `pipe_test.go` (revisão da T08)
- Assinaturas públicas:
  ```go
  package ipc
  func PipeName(sid string) string // `\\.\pipe\arcadia-agent-<sid>`
  // windows only:
  func CurrentUserSID() (string, error)
  func ListenPipe(name string) (net.Listener, error) // go-winio, SDDL D:P(A;;GA;;;<current SID>), first instance
  func DialPipe(ctx context.Context, name string) (net.Conn, error)
  func PipeDialer(name string) Dialer
  ```
- Testes (nome único `PipeName(sid)+"-test-<rand>"`): "IPC-007/nome segue arcadia-agent-<SID>",
  "IPC-007/usuário atual conecta e faz ping via Server", "IPC-007/segunda escuta no mesmo nome falha",
  "IPC-007/DACL protegida só com o SID atual" (com `Accept` pendente; · alterado em 2026-11-08);
  extra em `server_test.go`: `TestServerAcceptError` "IPC-006/erro transitório de Accept é logado e o accept continua"
  e "IPC-006/Close durante a espera após erro de Accept encerra" (listener fake)
  · alterado em 2026-11-08 (revisão da T08): os dois casos de `TestServerAcceptError` passam a IPC-018 (ver abaixo);
  "IPC-007/segunda escuta no mesmo nome falha" fica mais forte: além de `err != nil`, confere
  `errors.As(err, &pathErr)` com `*os.PathError` (`Op == "open"`) e `errors.As(err, &errno)` com `windows.Errno`
  igual ao código observado (esperado `windows.ERROR_ALREADY_EXISTS`; se for outro, o test-writer fixa o observado e
  registra no relatório), e que a primeira escuta continua aceitando (um `ping` depois ainda funciona).
- Dependências: `github.com/Microsoft/go-winio` v0.6.4 (efetiva; · alterado em 2026-11-08, era "v0.6.2 ou última
  estável") · `golang.org/x/sys` (última estável)
- Depende de: T07
- · alterado em 2026-11-08 (revisão da T08) — erro de Accept com backoff e anti-squatting:
  - Requisitos: IPC-018, IPC-019
  - Arquivos: `internal/ipc/server.go`, `internal/ipc/pipe_windows.go` (e `pipe.go` se o sentinela ficar portável);
    testes `server_test.go`, `pipe_windows_test.go`
  - Assinaturas:
    ```go
    package ipc
    var ErrUntrustedServer = errors.New("ipc: untrusted pipe server") // public

    // internal (server.go), replace acceptRetryDelay:
    const (
        acceptBackoffMin  = 100 * time.Millisecond
        acceptBackoffMax  = 5 * time.Second
        maxAcceptFailures = 10
    )
    // internal (pipe_windows.go), test seam; tests swap it without t.Parallel and restore in t.Cleanup:
    var pipeServerSID = realPipeServerSID
    func realPipeServerSID(h windows.Handle) (string, error) // GetNamedPipeServerProcessId -> OpenProcess(QUERY_LIMITED) -> OpenProcessToken(TOKEN_QUERY) -> token user SID
    ```
    `DialPipe` mantém a assinatura; em falha de verificação fecha a `conn` e retorna erro com
    `errors.Is(err, ErrUntrustedServer)`. `ListenPipe` mantém a assinatura e a cadeia `%w` até `*os.PathError`/`windows.Errno`.
  - Testes (`server_test.go`, listener fake com fila programável, `FakeClock` + `BlockUntilWaiters`, handler de log de teste):
    - `TestServerAcceptError`: "IPC-018/erro transitório de Accept é logado em Warn e o accept continua" (renomeado de
      IPC-006), "IPC-018/Close durante a espera após erro de Accept faz Serve retornar nil" (renomeado de IPC-006),
      "IPC-018/fim do ctx durante a espera faz Serve retornar ctx.Err()",
      "IPC-018/esperas 100 200 400 800 1600 3200 5000 5000 5000 ms", "IPC-018/sem jitter (nada dispara 1ns antes)",
      "IPC-018/Accept bem-sucedido zera o backoff para 100 ms",
      "IPC-018/10ª falha seguida: Serve retorna errors.Is último erro e loga Error",
      "IPC-018/net.ErrClosed sem Close continua encerrando Serve com erro"
  - Testes (`pipe_windows_test.go`, sem `t.Parallel` nos casos que trocam `pipeServerSID`):
    - `TestDialPipeOwner`: "IPC-019/servidor do mesmo usuário é aceito e responde ping",
      "IPC-019/realPipeServerSID do próprio pipe devolve CurrentUserSID",
      "IPC-019/SID diferente retorna ErrUntrustedServer e fecha a conexão",
      "IPC-019/falha ao ler o SID do servidor retorna ErrUntrustedServer e fecha a conexão",
      "IPC-019/Client com PipeDialer trata ErrUntrustedServer como falha de dial e não envia Call"
    - `TestListenPipeExists`: "IPC-019/ListenPipe em nome existente preserva os.PathError e windows.Errno"
      (pode ser o mesmo teste de "IPC-007/segunda escuta no mesmo nome falha", com os dois nomes de caso)
  - Handles: processo (`defer windows.CloseHandle`) e token (`defer tok.Close()`) em `realPipeServerSID`; o handle da
    `conn` é da `conn` e não é fechado ali; em falha, `conn.Close()` antes de retornar.
  - O log "pipe já existe: outra instância ou outro processo" é do agente e entra na T10 (usa a cadeia de erro de `ListenPipe`).

## T09 — winapi: instância única, fim de sessão e criação desacoplada
- Requisitos: AGT-002, AGT-006, AGT-007, DSK-004, DSK-005
- Responsável: implementer · Testes: test-writer
- Arquivos: `internal/winapi/mutex_windows.go`, `sessionend_windows.go`, `process_windows.go`; testes `mutex_windows_test.go`,
  `sessionend_windows_test.go` (caixa-branca: `SendMessage` no hwnd), `process_windows_test.go`
- Assinaturas públicas:
  ```go
  //go:build windows
  package winapi
  const AgentMutexName = `Local\arcadia-agent`
  type NamedMutex struct{ /* h windows.Handle */ }
  func AcquireSingleInstance(name string) (m *NamedMutex, already bool, err error) // closes its own handle when already
  func (m *NamedMutex) Close() error

  // Hidden top-level window (not HWND_MESSAGE) on a locked OS thread.
  type SessionEndWatcher struct{ /* clock, hwnd, done, ack */ }
  func NewSessionEndWatcher(clock domain.Clock, log *slog.Logger) *SessionEndWatcher
  func (w *SessionEndWatcher) Start(ctx context.Context) (<-chan struct{}, error) // signals on WM_ENDSESSION wParam=TRUE
  func (w *SessionEndWatcher) ShutdownDone() // lets the WM_ENDSESSION handler return (else clock timeout of 2 s) — lacuna 2
  func (w *SessionEndWatcher) Close() error  // idempotent; WM_CLOSE, waits loop, DestroyWindow + UnregisterClass

  // CreateProcessW wrapper; args quoted with windows.EscapeArg; closes hProcess/hThread right after creation.
  // Returned error wraps the windows.Errno (errors.Is works).
  func CreateDetached(exe string, args []string, flags uint32) error
  ```
- Testes:
  - `TestSingleInstance`: "AGT-002/primeira aquisição não é duplicada", "AGT-002/segunda aquisição detecta instância", "AGT-002/após Close readquire"
  - `TestSessionEndWatcher`: "AGT-006/WM_ENDSESSION TRUE sinaliza o canal", "AGT-006/WM_ENDSESSION FALSE não sinaliza",
    "AGT-006/handler espera ShutdownDone", "AGT-006/handler retorna após 2 s do relógio sem ShutdownDone",
    "AGT-006/Close encerra a goroutine (goleak)", "AGT-006/Close idempotente"
  - `TestCreateDetached`: "AGT-007/inicia processo sem aumentar a contagem de handles" (`cmd.exe /c exit 0`, `GetProcessHandleCount`),
    "DSK-005/exe inexistente preserva o errno via errors.Is", "DSK-004/argumento com espaço chega como um só" (opcional)
- Dependências: `golang.org/x/sys`
- Depende de: T02

## T10 — cmd/agent: subida, versão, log, bandeja, "Abrir Arcadia" e shutdown
- Requisitos: AGT-001, AGT-002, AGT-003, AGT-004, AGT-005, AGT-006, AGT-007, AGT-008, IPC-019 (log do nome existente;
  · alterado em 2026-11-08)
- Responsável: implementer · Testes: test-writer
- Arquivos: `cmd/agent/app.go` (subida/shutdown injetáveis), `cmd/agent/openui.go`, `cmd/agent/main_windows.go`,
  `cmd/agent/tray_windows.go`, `cmd/agent/assets/icon.ico` (placeholder); testes `app_test.go`, `openui_test.go`
- Assinaturas (pacote `main`, não exportadas):
  ```go
  var version = "dev" // -ldflags "-H windowsgui -X main.version=<v>"
  type config struct {
      DataDir  string // --data-dir
      LogLevel string // --log-level
      ExeDir   string
  }
  type spawnFunc func(exe string, args []string, flags uint32) error // real: winapi.CreateDetached
  type agent struct{ /* paths, store, server, log closer, wg */ }
  // order: logging.ParseLevel (invalid => error before any file) -> paths.Resolve/EnsureDirs
  //        -> logging.New(agent.log) -> store.Open(Migrate:true) -> listen() -> go server.Serve
  func startAgent(ctx context.Context, cfg config, clock domain.Clock, getenv func(string) string,
      listen func() (net.Listener, error)) (*agent, error)
  func (a *agent) Info() ipc.AgentInfo                // Version: version
  func (a *agent) Shutdown(ctx context.Context) error // IPC -> store; idempotent; does NOT close the log
  func (a *agent) CloseLog() error                    // closes the logging io.Closer; idempotent; called last by main
  func openUI(exeDir string, stat func(string) error, spawn spawnFunc, log *slog.Logger) // DETACHED_PROCESS|CREATE_NEW_PROCESS_GROUP
  ```
  Fluxo do `main`: flags → `AcquireSingleInstance` (já existe → `os.Exit(0)` sem tocar em logs) → `startAgent` (nível de log
  inválido → `os.Exit(2)`) → `watcher.Start` → `systray.Run` ("Sair", Ctrl+C/SIGTERM e o canal do watcher chamam `systray.Quit()`)
  → `agent.Shutdown` (2 s) → log "shutdown concluído" → `watcher.ShutdownDone()` → `watcher.Close()` → `mutex.Close()`
  → `agent.CloseLog()`. · alterado em 2026-11-08: fechamento do log movido para o fim, pois o watcher loga e um `Write`
  após `Close` reabre o arquivo (revisão da T05).
- Testes: "AGT-001/cria library.db migrado e logs/agent.log", "AGT-004/sem -X version é dev",
  "AGT-004/agent.status reflete a variável version", "AGT-005/nível inválido falha sem criar library.db", "AGT-005/sem flag usa info",
  "AGT-003/Shutdown sem goroutines sobrando (goleak)", "AGT-003/Shutdown fecha o store", "AGT-003/Shutdown idempotente",
  "AGT-003/CloseLog idempotente",
  "AGT-007/arcadia.exe existe: spawn com flags desacopladas", "AGT-008/arcadia.exe ausente: loga erro e não chama spawn"
  · nota 2026-11-08: o caso "dev" sem `-ldflags` de AGT-004 é testado só aqui (T10); a T06 cobre apenas que
  `agent.status` devolve o `AgentInfo.Version` recebido (lacunas da T06).
- goleak · alterado em 2026-11-08: exceção da goroutine do lumberjack (revisão da T05): o teste "AGT-003/Shutdown sem
  goroutines sobrando (goleak)" (e qualquer `goleak.VerifyNone`/`VerifyTestMain` deste pacote) usa
  `goleak.VerifyNone(t, goleak.IgnoreTopFunction("gopkg.in/natefinch/lumberjack%2ev2.(*Logger).millRun"))` — a goroutine
  `millRun` não é encerrada por `Close` (ver design.md, `internal/logging`). O teste chama `Shutdown` e `CloseLog` antes
  do `VerifyNone`.
- · alterado em 2026-11-08 (revisão da T08): IPC-019 (parte do agente). Quando `listen()` falha com nome já existente
  (`errors.Is(err, windows.ERROR_ALREADY_EXISTS)` ou `windows.ERROR_ACCESS_DENIED`, via cadeia de `ListenPipe`),
  `startAgent` loga em `Error` "pipe já existe: outra instância ou outro processo" (atributos `pipe`, `error`) e
  retorna o erro. Teste: "IPC-019/listen com nome existente loga Error explícito" (`listen` fake devolvendo
  `&os.PathError{Op: "open", Err: windows.ERROR_ALREADY_EXISTS}`).
- Dependências: `fyne.io/systray` v1.11.0 (ou última estável) · `go.uber.org/goleak` v1.3.0 (só teste)
- Depende de: T03, T04, T05, T08, T09

## T11 — Frontend: indicador, estado vazio e uptime
- Requisitos: UI-001, UI-002, UI-003
- Responsável: implementer · Testes: test-writer
- Arquivos: `frontend/package.json`, `vite.config.ts`, `tsconfig.json`, `index.html`, `src/main.tsx`, `src/App.tsx`,
  `src/components/AgentIndicator.tsx`, `src/components/LibraryEmpty.tsx`, `src/lib/format.ts`,
  `src/lib/agent.ts` (TanStack Query `refetchInterval` 2 s + evento `agent:heartbeat`), `src/test/setup.ts`;
  testes `AgentIndicator.test.tsx`, `LibraryEmpty.test.tsx`, `format.test.ts`
- Assinaturas públicas (TS, componentes puros):
  ```ts
  export type AgentStatusView = { connected: boolean; version: string; pid: number; startedAt: string; dataDir: string; uptimeSeconds: number };
  export type LibrarySummaryView = { games: number; platforms: number };
  export function formatUptime(seconds: number): string;
  export function AgentIndicator(props: { status: AgentStatusView }): JSX.Element; // role="status", data-state="connected"|"disconnected"
  export function LibraryEmpty(props: { summary: LibrarySummaryView }): JSX.Element;
  ```
- Testes (Vitest + Testing Library + jsdom): "UI-001/conectado mostra Conectado em verde com versão e uptime",
  "UI-001/desconectado mostra Desconectado em vermelho sem versão nem uptime", "UI-002/mostra Nenhum jogo ainda",
  "UI-002/mostra 0 jogos · M plataformas", "UI-003/0 → 0s", "UI-003/59 → 59s", "UI-003/60 → 1m 00s", "UI-003/3599 → 59m 59s",
  "UI-003/3600 → 1h 00m 00s", "UI-003/3723 → 1h 02m 03s"
- Dependências: template `react-ts` do Wails atualizado: React, Vite, Tailwind CSS v4 (`@tailwindcss/vite`),
  `@tanstack/react-query` v5, `zustand` v5, `vitest`, `@testing-library/react`, `@testing-library/jest-dom`, `jsdom`
  (últimas estáveis); Node LTS
- Depende de: T01

## T12 — cmd/desktop: app Wails, início do agente, banco tardio e bindings
- Requisitos: DSK-001, DSK-002, DSK-003, DSK-004, DSK-005, DSK-006, DSK-007, DSK-008, DSK-009
- Responsável: implementer · Testes: test-writer
- Arquivos: `cmd/desktop/wails.json` (`frontend:dir` = `../../frontend`, `outputfilename` = `arcadia`), `main.go`, `app.go`,
  `agentproc_windows.go`, `library.go`, `datadir.go`, `frontend/wailsjs/**` (gerado, versionado);
  testes `agentproc_windows_test.go`, `library_test.go`, `datadir_test.go`, `app_test.go`
- Assinaturas:
  ```go
  package main
  var ErrAgentNotReady = errors.New("agente ainda não pronto")
  type StatusView struct {
      Connected     bool   `json:"connected"`
      Version       string `json:"version"`
      PID           int    `json:"pid"`
      StartedAt     string `json:"startedAt"`
      DataDir       string `json:"dataDir"`
      UptimeSeconds int64  `json:"uptimeSeconds"`
  }
  type LibrarySummaryView struct {
      Games     int64 `json:"games"`
      Platforms int64 `json:"platforms"`
  }
  type App struct{ /* client, gate, log, wg */ }
  func NewApp(cfg desktopConfig) *App
  func (a *App) AgentStatus() StatusView                     // agent.status, 1 s; error => Connected=false
  func (a *App) LibrarySummary() (LibrarySummaryView, error) // via libraryGate
  // unexported:
  type desktopConfig struct{ DataDir, ExeDir string } // --data-dir resolved via paths.Resolve (lacuna 3)
  type spawnFunc func(exe string, args []string, flags uint32) error
  func agentCommand(exeDir, root string) (exe string, args []string, flags uint32)
  func startAgentProcess(spawn spawnFunc, exeDir, root string, log *slog.Logger) error // ERROR_ACCESS_DENIED => warn + retry without BREAKAWAY
  type pinger interface{ Call(ctx context.Context, method string, params, result any) error }
  func ensureAgent(ctx context.Context, p pinger, start func() error, clock domain.Clock, timeout time.Duration) (ipc.AgentStatus, error)
  func warnIfDataDirDiffers(log *slog.Logger, root, agentDataDir string) bool // filepath.Clean + case-insensitive
  type libraryStore interface {
      LibrarySummary(ctx context.Context) (store.LibrarySummary, error)
      Close() error
  }
  type libraryGate struct{ /* mu, st, connected, open */ }
  func newLibraryGate(open func(ctx context.Context) (libraryStore, error)) *libraryGate // real: store.Open(Migrate:false)
  func (g *libraryGate) MarkConnected()
  func (g *libraryGate) Summary(ctx context.Context) (LibrarySummaryView, error) // not connected or ErrSchemaMismatch => ErrAgentNotReady, no cached failure
  func (g *libraryGate) Close() error
  func setupLogging(p paths.Paths) (*slog.Logger, io.Closer, error) // logging.New(File:"desktop.log", Level:Info)
  ```
  OnStartup: `paths.Resolve` → `setupLogging` → `ipc.NewClient(PipeDialer(PipeName(sid)), SystemClock, log)` →
  `ensureAgent` (5 s, `start` = `startAgentProcess`) → `gate.MarkConnected()` → `warnIfDataDirDiffers` → goroutine que repassa
  `Notifications()` como evento `agent:heartbeat`. OnShutdown: `client.Close()` → `wg.Wait()` → `gate.Close()` → log `Close`.
  **Nunca** encerra o agente.
- Testes:
  - `TestAgentCommand`: "DSK-004/exe é P\arcadia-agent.exe", "DSK-004/args são --data-dir R", "DSK-004/flags DETACHED|NEW_PROCESS_GROUP|BREAKAWAY"
  - `TestStartAgentProcess`: "DSK-005/ACCESS_DENIED tenta de novo sem BREAKAWAY e loga aviso", "DSK-005/outro erro não tenta de novo",
    "DSK-005/sucesso na primeira não tenta de novo"
  - `TestEnsureAgent` (`FakeClock`): "DSK-001/agente rodando não chama start", "DSK-001/agente parado chama start uma vez e conecta",
    "DSK-001/sem resposta em 5 s retorna erro"
  - `TestDataDir`: "DSK-006/dataDir diferente loga aviso", "DSK-006/mesmo dir com caixa/barra diferente não avisa"
  - `TestLibraryGate`: "DSK-007/antes de conectar não chama open", "DSK-007/após MarkConnected abre e responde",
    "DSK-008/ErrSchemaMismatch retorna ErrAgentNotReady", "DSK-008/próxima chamada tenta abrir de novo"
  - `TestSetupLogging`: "DSK-009/registra em R/logs/desktop.log"
  - `TestAgentStatusView`: "DSK-003/erro de Call vira Connected=false"
- Dependências: `github.com/wailsapp/wails/v2` v2.17.1 (igual ao CLI); o binário funcional sai de `wails build`
- Depende de: T04, T05, T08, T09, T11

## T13 — Verificação final e checagens manuais
- Requisitos: BLD-001, AGT-001, AGT-002, AGT-003, AGT-004, AGT-006, AGT-007, DSK-001, DSK-002, DSK-003
- Responsável: reviewer (builds pelo implementer) · Testes: —
- Arquivos: — (nenhum código novo)
- Roteiro:
  - **BLD-001:** clone limpo sem Node no PATH → `$env:CGO_ENABLED='0'; go build ./...`, `go test ./...`, `go vet ./...` passam.
  - **AGT-001:** `go run ./cmd/agent --data-dir .devdata` → `library.db` na versão 1, `logs/agent.log` em JSON, ícone na bandeja.
  - **AGT-004:** `go build -ldflags "-H windowsgui -X main.version=1.2.3" ./cmd/agent` → `agent.status` mostra `1.2.3`.
  - **AGT-002:** segundo `arcadia-agent.exe` sai com 0, sem novo ícone nem linha de log.
  - **AGT-003:** "Sair" → processo some em < 2 s (timestamps no log); sem crescimento de handles após 10 conexões/desconexões.
  - **AGT-006:** logoff com o agente rodando → log mostra o shutdown ordenado completo.
  - **AGT-007:** `arcadia.exe` ao lado do agente → "Abrir Arcadia" abre a UI como processo independente.
  - **DSK-001:** agente parado → abrir `arcadia.exe` → verde em ≤ 5 s; agente sem pai vivo.
  - **DSK-002:** fechar a UI → agente segue na bandeja e responde na reabertura.
  - **DSK-003:** `Stop-Process` no agente → "Desconectado" em ≤ 2 s; reiniciar → volta a "Conectado" sozinho.
  - **Rastreabilidade:** `scripts/spec-trace.ps1 -Spec docs/specs/phase-2-skeleton` sai com 0.
- Depende de: T01–T12

---

## Riscos de concorrência e recursos

| Goroutine | Dono | Como termina |
|---|---|---|
| Accept do `Server.Serve` | `Server` (`wg`) | `Close`/ctx → `ln.Close()` |
| Uma por conexão (lê e responde em sequência) | `Server` (`wg`) | `conn.Close()`, EOF/erro ou mensagem acima do limite |
| Loop de heartbeat (broadcast) | `Server` (`wg`) | `select` em `clock.After(5s)` / `ctx.Done()` |
| Dial, leitura e backoff do cliente | `Client` | `Close()` cancela e fecha a conn; espera e então fecha `Notifications()` |
| Repasse `Notifications()` → evento Wails | `App` (`wg`) | `range` termina quando o canal fecha |
| Sinais → `systray.Quit` | `main` do agente | `stop()` após `systray.Run` |
| Laço de mensagens do `SessionEndWatcher` (`LockOSThread`) | `SessionEndWatcher` | `WM_CLOSE` → `DestroyWindow` → `WM_QUIT` |
| Ponte canal do watcher → `systray.Quit` | `main` do agente | ctx do agente cancelado |

- WndProc de `WM_ENDSESSION` bloqueia até `ShutdownDone()` ou 2 s: a ordem `Shutdown` → `ShutdownDone` → `Close` é obrigatória.
- `FakeClock.Advance` envia sem bloquear (buffer 1) e nunca espera leitor com o mutex segurado.
- `Encoder` com mutex: resposta e heartbeat não se intercalam. Notificações com buffer 16 e descarte (IPC-017).

| Recurso | Quem fecha |
|---|---|
| Mutex `Local\arcadia-agent` | `NamedMutex.Close()`, por último; no caso "já existe", dentro de `AcquireSingleInstance` |
| Pipe e conexões | `listener.Close()` em `Server.Close`, `conn.Close()` |
| Token do processo (SID) | `defer token.Close()` em `CurrentUserSID` |
| Processo e token do servidor do pipe (IPC-019) | `defer windows.CloseHandle(hp)` e `defer tok.Close()` em `realPipeServerSID`; `conn.Close()` em `DialPipe` se a verificação falha |
| Janela oculta e classe | goroutine do watcher (`DestroyWindow`, `UnregisterClass`) |
| `hProcess`/`hThread` de UI e agente criados | `CreateDetached`, logo após `CreateProcessW` |
| Logs | `io.Closer` do `logging.New` — agente em `agent.CloseLog` (último passo do `main`); UI por último no `OnShutdown` |
| SQLite | `Store.Close()` — agente em `Shutdown`; UI em `libraryGate.Close` |

**Shutdown do agente:** `systray.Quit` → `Server.Close` → `Store.Close` → log "shutdown concluído" → `watcher.ShutdownDone` → `watcher.Close` → `mutex.Close` → log `Close`.
· alterado em 2026-11-08: log `Close` movido para o fim (revisão da T05).
**Shutdown do desktop:** `Client.Close` → `wg.Wait` → `libraryGate.Close` → log `Close`; o agente não é tocado.
O log `Close` é sempre o último passo: um `Write` depois dele reabre o arquivo (lumberjack) e vaza o handle.

Exceção aceita à tabela de goroutines: `millRun` do `lumberjack` v2.2.1 (uma por processo, sem dono, não encerrada por
`Close`, não segura handles); testes `goleak` a ignoram com `IgnoreTopFunction("gopkg.in/natefinch/lumberjack%2ev2.(*Logger).millRun")`.

Outros riscos: sqlc pode não entender `CREATE VIRTUAL TABLE ... USING fts5` (schema separado só para o sqlc, registrado no
relatório); fallback sem breakaway (DSK-005) pode fazer o agente morrer com o job do terminal/IDE (aceito, com aviso no log);
`frontend/node_modules` com arquivos `.go` pode quebrar `go build ./...`.

## Rastreabilidade

| Requisito | Tarefa(s) | | Requisito | Tarefa(s) |
|---|---|---|---|---|
| BLD-001 | T01, T13 | | IPC-010 | T06, T07 |
| PTH-001 | T03 | | IPC-011 | T06, T07 |
| PTH-002 | T03 | | IPC-012 | T06, T07 |
| PTH-003 | T03 | | IPC-013 | T07 |
| DOM-001 | T02 | | IPC-014 | T07 |
| DOM-002 | T02 | | IPC-015 | T07 |
| DOM-003 | T02 | | IPC-016 | T07 |
| DOM-004 | T02 | | IPC-017 | T07 |
| DOM-005 | T02 | | AGT-001 | T10, T13 |
| DOM-006 | T02 | | AGT-002 | T09, T10, T13 |
| STO-001 | T04 | | AGT-003 | T10, T13 |
| STO-002 | T04 | | AGT-004 | T06, T10, T13 |
| STO-003 | T04 | | AGT-005 | T05, T10 |
| STO-004 | T04 | | AGT-006 | T09, T10, T13 |
| STO-005 | T04 | | AGT-007 | T09, T10, T13 |
| STO-006 | T04 | | AGT-008 | T10 |
| STO-007 | T04 | | DSK-001 | T12, T13 |
| STO-008 | T04 | | DSK-002 | T12, T13 |
| STO-009 | T04 | | DSK-003 | T12, T13 |
| STO-010 | T04 | | DSK-004 | T09, T12 |
| IPC-001 | T06, T07 | | DSK-005 | T09, T12 |
| IPC-002 | T06 | | DSK-006 | T12 |
| IPC-003 | T06, T07 | | DSK-007 | T12 |
| IPC-004 | T07 | | DSK-008 | T12 |
| IPC-005 | T07 | | DSK-009 | T05, T12 |
| IPC-006 | T07 | | UI-001 | T11 |
| IPC-007 | T08 | | UI-002 | T11 |
| IPC-008 | T06 | | UI-003 | T11 |
| IPC-009 | T06 | | IPC-018 | T08 |
| | | | IPC-019 | T08, T10 |
