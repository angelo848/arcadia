# Fase 3 — Tracker MVP: tarefas

- **Status:** aprovado pelo usuário (2026-11-09); T01–T16 concluídas (2026-11-10)
- **Spec:** [requirements.md](requirements.md) · [design.md](design.md) · [contracts/ipc.openrpc.json](contracts/ipc.openrpc.json) (v0.4.0) · [contracts/session-state-machine.md](contracts/session-state-machine.md) · [contracts/fake-game.md](contracts/fake-game.md)
- **ADRs:** 0005 (só a Camada 1), 0009 (ms UTC), 0010 (só o agente migra), 0011 (Clock), 0012 (enums no `domain`), 0013 (log fecha por último), 0014 (executável único), 0015 (agente é o único escritor), 0016 (breakaway permitido)

## Decisões do planner (a confirmar na aprovação)
1. **`asOf` com milissegundos:** o `time.Time` do Go serializa em RFC3339Nano e corta os zeros finais (`.3Z`, ou nenhuma fração). Por isso entra o tipo `ipc.MilliTime`, que sempre usa o layout `2006-01-02T15:04:05.000Z07:00`. Os outros instantes continuam `time.Time`.
2. **`gameId`:** `json.Number` sozinho aceita `"5"` entre aspas, o que fere IPC-028. O agente decodifica `gameId` como `json.RawMessage` e só aceita um literal inteiro (sem fração nem expoente) ≥ 1.
3. **Ordem da validação do exe (GAM-002):** segue o design: `IsAbs` → `stat` (`not-found`) → `IsRegular` (`not-a-file`) → extensão (`not-exe`). Um `stat` que falha com outro erro (ex.: acesso negado) vira `-32603`, com log.
4. **`ctx` do handler `game.launch` esgotado (10 s):** devolve `-32603`. A UI mostra "Erro inesperado"; o resultado real chega por `session:changed` (ver "Problemas na spec", item 4).
5. **Snapshot de `game.launch` e da notificação `running`:** medido no `t` de `Prepared`: `asOf = t`, `activeMs = 0`, `startedAt = t`.
6. **`Machine.GraceTimeout` recebe `now`:** o design não tem esse parâmetro, mas ele é preciso para o `asOf` do fim (IPC-031).
7. **Heartbeat continua com descarte em `Debug`:** `Notify` e `broadcastHeartbeat` passam a usar um helper interno com o nível de log como parâmetro. `Notify` usa `Warn` (IPC-029); o heartbeat fica como na fase 2.
8. **UI-016:** ao reconectar, a sessão atual é **resetada** (`resetQueries`), não só invalidada. Assim o painel não reaparece com o valor antigo antes da recarga.
9. **`UpdateSession` em sessão já encerrada:** devolve `store.ErrSessionNotOpen` (`WHERE ended_at IS NULL`), igual ao `EndSession`.
10. **`LibraryEmpty`/`useLibrarySummary`:** a tela deixa de usá-los, mas o componente, o hook e o teste ficam no repositório até a fase 5. O spec-trace da fase 2 continua verde.
11. **`quoteExe`:** o tracker-specialist duplica a função em `job_windows.go`, com cerca de 10 linhas. Assim ele não precisa editar `internal/winapi`.
12. **Injeção no agente:** `config` ganha dois campos opcionais para os testes: `Launcher` (nil → `tracker.NewLauncher()`) e `Recover` (nil → `tracker.RecoverOpenSessions`).

## Resumo
São dezesseis tarefas, em ordem de dependência:
- **T01–T05 (portáveis):** domain (T01); store com migração 3 e jogos (T02); store de sessões (T03); protocolo IPC v0.4.0 (T04); `Server.Notify` (T05).
- **T06–T09 (tracker-specialist):** `cmd/fake-game` (T06); máquina pura (T07); coordenador e recuperação (T08); Job Object no Windows com integração (T09).
- **T10–T11 (agente):** handlers (T10); subida e shutdown com o tracker (T11).
- **T12 (UI Go):** bindings, erros e `session.changed`.
- **T13–T14 (frontend):** lib (T13); componentes e `App` (T14).
- **T15–T16:** documentação (T15); verificação final, com UI-011 (T16).

Cada tarefa passa por test-writer → implementer/tracker-specialist → reviewer e vira um commit próprio.

**Premissas:**
- Ao fim de cada tarefa passam `CGO_ENABLED=0 go build ./...`, `go test ./...`, `go vet ./...` e `GOOS=linux go vet ./...`.
- Não há dependência nova: `goleak`, `x/sys`, `go-winio`, TanStack Query e Testing Library já estão no projeto.
- `sqlc generate` é necessário em T02 e T03.
- `cd cmd/arcadia && wails generate module` é necessário em T12: atualiza `frontend/wailsjs/`, que é versionado. O frontend continua chamando `window.go.main.App` com tipos locais, como hoje.

---

## T01 — domain: estados, motivos de fim, descarte e título padrão
- Requisitos: DOM-007, DOM-008, DOM-009
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `internal/domain/session.go` (novo)
  - `internal/domain/game.go` (novo)
  - testes: `internal/domain/session_test.go` e `internal/domain/game_test.go`
- Assinaturas públicas:
  ```go
  package domain
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
      JobPollInterval          = 5 * time.Second
  )
  func Outcome(active time.Duration) SessionState // ended if active >= MinCountedSession, else discarded
  type SessionSnapshot struct {
      ID, GameID int64
      Title      string
      State      SessionState
      StartedAt  time.Time
      EndedAt    time.Time     // zero while running/grace
      Active     time.Duration // measured at AsOf
      AsOf       time.Time
  }
  func DefaultGameTitle(path string) string // splits on '\\' and '/', strips one trailing ".exe" (case-insensitive)
  ```
- Testes:
  - `TestParseSessionState`:
    - "DOM-007/running, grace, ended e discarded são aceitos"
    - "DOM-007/vazio é erro"
    - "DOM-007/Running com maiúscula é erro"
    - "DOM-007/paused é erro"
  - `TestParseSessionEndReason`:
    - "DOM-007/exited, agent-shutdown e recovered são aceitos"
    - "DOM-007/valor desconhecido é erro"
    - "DOM-007/vazio é erro"
  - `TestOutcome`:
    - "DOM-008/0 é discarded"
    - "DOM-008/29,999 s é discarded"
    - "DOM-008/30 s é ended"
    - "DOM-008/2 h é ended"
  - `TestDefaultGameTitle`:
    - "DOM-009/hollow_knight.exe vira hollow_knight"
    - "DOM-009/Game.EXE vira Game"
    - "DOM-009/my.game.exe vira my.game"
    - "DOM-009/b.exe.exe vira b.exe"
    - "DOM-009/barra normal como separador"
  - `TestSessionConstants`: "DOM-008/MinCountedSession é 30 s, carência 15 s, heartbeat 30 s, verificação 5 s"
- Depende de: —

## T02 — store: migração 3, `SchemaVersion = 3` e jogos
- Requisitos: STO-011, GAM-001, GAM-005, STO-001, AST-001
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `internal/store/migrations/00003_session_end_reason.sql` (DDL do design)
  - `internal/store/queries/games.sql`:
    - `EnsurePCPlatform :exec` (`ON CONFLICT(name) DO NOTHING`)
    - `GetPlatformIDByName :one`
    - `InsertGame :one` (`RETURNING *`)
    - `ListGames :many` (`ORDER BY title COLLATE NOCASE, id`)
    - `GetGame :one`
  - `internal/store/dbgen/*`: regerado com **`sqlc generate`**. Conferir que o sqlc aceita o índice parcial e o `ALTER TABLE`; se não aceitar, a sessão principal decide.
  - `internal/store/store.go` (`SchemaVersion = 3`)
  - `internal/store/games.go` (novo)
  - testes: `internal/store/games_test.go` (novo), `migrate3_test.go` (novo) e os ajustes de `store_test.go`/`settings_test.go` (casos que fixavam a versão 2 passam a usar `SchemaVersion`)
- Assinaturas públicas:
  ```go
  package store
  const SchemaVersion int64 = 3
  var ErrGameNotFound = errors.New("store: game not found")
  type Game struct {
      ID                                                     int64
      Title, LaunchType, LaunchTarget, LaunchArgs, WorkingDir string
      TotalPlaySeconds, PlayCount                            int64
      FirstPlayedAt, LastPlayedAt                            time.Time // zero = NULL
  }
  type NewGame struct {
      Title, Exe, WorkingDir string
      At                     time.Time
  }
  func (s *Store) AddGame(ctx context.Context, g NewGame) (Game, error) // one Tx: platform PC/pc + game row
  func (s *Store) Games(ctx context.Context) ([]Game, error)            // never nil
  func (s *Store) Game(ctx context.Context, id int64) (Game, error)     // sql.ErrNoRows => ErrGameNotFound (%w)
  ```
- Testes (`t.TempDir()`):
  - `TestMigration3`:
    - "STO-011/banco novo tem play_session.end_reason TEXT nula e goose na versão 3"
    - "STO-011/banco v2 com jogo, sessão e app_setting migra para 3 mantendo os dados, com end_reason nulo" (o banco v2 sai de goose `UpTo(2)`)
    - "STO-011/índice play_session_open_idx existe"
    - "STO-011/SchemaVersion é 3"
  - `TestAddGame`:
    - "GAM-001/linha com title, launch_type exe, launch_target limpo, working_dir, launch_args vazio, totais 0 e datas nulas"
    - "GAM-001/created_at e updated_at são UnixMilli de At"
    - "GAM-001/plataforma PC (categoria pc) criada na primeira adição"
    - "GAM-001/segunda adição reutiliza a plataforma"
    - "GAM-001/mesmo exe duas vezes cria dois ids"
    - "GAM-001/mesmo caminho com outra caixa cria outro jogo"
  - `TestGames`:
    - "GAM-005/b, A, a e C (ids 1..4) saem na ordem A(2), a(3), b(1), C(4)"
    - "GAM-005/biblioteca vazia devolve slice vazio, não nil"
  - `TestGame`:
    - "GAM-001/devolve o jogo pelo id"
    - "GAM-001/id inexistente: errors.Is ErrGameNotFound"
  - Os casos existentes STO-001, STO-002, STO-009 e AST-001 que fixavam a versão 2 passam a usar `SchemaVersion`.
- Depende de: T01

## T03 — store: sessões
- Requisitos: STO-012, STO-013, STO-014, STO-015, STO-016, STO-017
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `internal/store/queries/sessions.sql`:
    - `InsertSession :one`
    - `UpdateOpenSession :execrows`
    - `GetOpenSession :one`
    - `CloseSession :execrows` (`WHERE id = ? AND ended_at IS NULL`)
    - `AddPlayToGame :execrows`
    - `DeleteSession :exec`
    - `ListOpenSessions :many`
  - `internal/store/dbgen/*` (regerado com **`sqlc generate`**)
  - `internal/store/sessions.go` (novo)
  - teste: `internal/store/sessions_test.go`
- Assinaturas públicas:
  ```go
  var ErrSessionNotOpen = errors.New("store: session not open") // missing or already ended
  type OpenSession struct {
      ID, GameID            int64
      StartedAt, LastSeenAt time.Time
      ActiveSeconds         int64
  }
  type SessionEnd struct {
      ID        int64
      EndedAt   time.Time
      Active    time.Duration
      Reason    domain.SessionEndReason
      WrittenAt time.Time
  }
  func (s *Store) StartSession(ctx context.Context, gameID int64, at time.Time) (int64, error) // running, tracked
  func (s *Store) UpdateSession(ctx context.Context, id int64, st domain.SessionState,
      lastSeen time.Time, active time.Duration) error // active_seconds = int64(active / time.Second)
  func (s *Store) EndSession(ctx context.Context, e SessionEnd) (domain.SessionState, error) // one Tx; state = domain.Outcome(e.Active)
  func (s *Store) DeleteSession(ctx context.Context, id int64) error
  func (s *Store) OpenSessions(ctx context.Context) ([]OpenSession, error) // never nil
  ```
- Testes:
  - `TestStartSession`:
    - "STO-012/linha com game_id, started_at = last_seen_at = T, active 0, running, tracked, ended_at e end_reason nulos"
    - "STO-012/id devolvido é o da linha"
  - `TestUpdateSession`:
    - "STO-013/grava estado, last_seen_at e floor(a/1s)"
    - "STO-013/ended_at continua nulo"
    - "STO-013/29,9 s grava 29"
    - "STO-013/sessão encerrada devolve ErrSessionNotOpen"
  - `TestEndSessionCounted`:
    - "STO-014/sessão ended com ended_at, last_seen_at = E, floor(a), end_reason"
    - "STO-014/jogo soma floor(a), play_count+1, last_played_at = E e updated_at = W"
    - "STO-014/first_played_at nulo vira S"
    - "STO-014/first_played_at existente é mantido"
    - "STO-014/trigger que aborta o UPDATE do jogo deixa a sessão aberta"
    - "STO-014/encerrar duas vezes devolve ErrSessionNotOpen e não soma de novo"
  - `TestEndSessionDiscarded`:
    - "STO-015/a < 30 s grava discarded com os mesmos campos"
    - "STO-015/linha do jogo não muda (nem updated_at)"
  - `TestTotalRecomputable`: "STO-016/10 s, 45 s, 30 s e 3600 s dão total 3675, play_count 3 e soma dos ended"
  - `TestOpenSessions`:
    - "STO-017/só as abertas, com id, game_id, started_at, last_seen_at e active_seconds"
    - "STO-017/sem abertas devolve vazio"
  - `TestDeleteSession`: "STO-012/DeleteSession remove a linha"
- Depende de: T02

## T04 — IPC: protocolo v0.4.0, `MilliTime`, conversão de sessão e params por nome
- Requisitos: IPC-030, IPC-031, IPC-028, IPC-020, IPC-021, IPC-025
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `internal/ipc/protocol.go`: constantes, tipos e o comentário do pacote apontando para `docs/specs/phase-3-tracker/contracts/ipc.openrpc.json`
  - `internal/ipc/session.go` (novo): `MilliTime`, `SessionFromSnapshot`
  - `internal/ipc/params.go` (novo): `DecodeParams`, `NoParams` (exportado; `agent_methods.go` passa a usá-lo)
  - testes:
    - `internal/ipc/contract_test.go` passa a ler o contrato de `phase-3-tracker`
    - `session_test.go` e `params_test.go` (novos)
- Assinaturas públicas:
  ```go
  package ipc
  const (
      MethodGameAdd        = "game.add"
      MethodGameList       = "game.list"
      MethodGameLaunch     = "game.launch"
      MethodSessionCurrent = "session.current"
      MethodSessionChanged = "session.changed"
  )
  const (
      CodeGameNotFound      = -32001
      CodeSessionActive     = -32002
      CodeLaunchFailed      = -32003
      CodeInvalidExecutable = -32004
  )
  const (
      MsgGameNotFound      = "Game not found"
      MsgSessionActive     = "Session already active"
      MsgLaunchFailed      = "Launch failed"
      MsgInvalidExecutable = "Invalid executable"
      MsgInvalidParams     = "Invalid params" // exported alias of msgInvalidParams
  )
  const (
      ReasonNotAbsolute = "not-absolute"
      ReasonNotFound    = "not-found"
      ReasonNotAFile    = "not-a-file"
      ReasonNotExe      = "not-exe"
  )
  const MaxGameTitleRunes = 200
  type MilliTime struct{ time.Time }            // JSON: UTC, layout "2006-01-02T15:04:05.000Z07:00"
  func (t MilliTime) MarshalJSON() ([]byte, error)
  func (t *MilliTime) UnmarshalJSON(b []byte) error
  type Game struct {
      ID               int64      `json:"id"`
      Title            string     `json:"title"`
      Path             string     `json:"path"`
      TotalPlaySeconds int64      `json:"totalPlaySeconds"`
      PlayCount        int64      `json:"playCount"`
      FirstPlayedAt    *time.Time `json:"firstPlayedAt"`
      LastPlayedAt     *time.Time `json:"lastPlayedAt"`
  }
  type Session struct {
      SessionID     int64      `json:"sessionId"`
      GameID        int64      `json:"gameId"`
      Title         string     `json:"title"`
      State         string     `json:"state"`
      StartedAt     time.Time  `json:"startedAt"`
      EndedAt       *time.Time `json:"endedAt"`
      ActiveSeconds int64      `json:"activeSeconds"`
      ActiveMs      int64      `json:"activeMs"`
      AsOf          MilliTime  `json:"asOf"`
  }
  type GameAddParams struct {
      Path  string `json:"path"`
      Title string `json:"title,omitempty"`
  } // client-side encoding
  type GameLaunchParams struct {
      GameID int64 `json:"gameId"`
  } // client-side encoding
  type GameListResult struct {
      Games []Game `json:"games"`
  } // never nil
  type SessionCurrentResult struct {
      Session *Session `json:"session"`
  }
  type SessionChangedParams struct {
      Session Session `json:"session"`
  }
  type SessionRef struct {
      SessionID int64  `json:"sessionId"`
      GameID    int64  `json:"gameId"`
      Title     string `json:"title"`
  }
  type LaunchFailedData struct {
      Error    string  `json:"error"`
      WinError *uint32 `json:"winError,omitempty"`
  }
  type InvalidExecutableData struct {
      Reason string `json:"reason"`
  }
  type GameNotFoundData struct {
      GameID int64 `json:"gameId"`
  }
  func NewErrorData(code int, message string, data any) *Error // Data = json.Marshal(data)
  // SessionFromSnapshot: ActiveMs = Active.Milliseconds(); ActiveSeconds = ActiveMs/1000;
  // AsOf = AsOf.UTC(); EndedAt nil when zero; StartedAt UTC.
  func SessionFromSnapshot(s domain.SessionSnapshot) Session
  // DecodeParams decodes named params into v: nil raw => nil (v untouched); a non-object,
  // an unknown member, a wrong type or trailing data => -32602.
  func DecodeParams(raw json.RawMessage, v any) *Error
  func NoParams(fn func(ctx context.Context) (any, error)) Handler
  ```
- Testes:
  - `TestContract`:
    - "IPC-030/versão 0.4.0"
    - "IPC-030/todo método e notificação do contrato tem constante Method*"
    - "IPC-030/campos obrigatórios de Game, Session e SessionRef batem com as tags JSON"
    - "IPC-030/códigos e mensagens de -32001 a -32004 batem com as constantes"
    - "IPC-030/data de LaunchFailed, InvalidExecutable e GameNotFound batem com os tipos"
    - "IPC-030/maxGameTitleRunes é MaxGameTitleRunes"
    - "IPC-030/x-tracking bate com as constantes do domain"
    - "IPC-030/comentário do pacote aponta para phase-3-tracker" (via `go/parser`)
    - os casos IPC-*/EXE-* existentes passam a usar o contrato v0.4.0
  - `TestSessionFromSnapshot`:
    - "IPC-031/Active 252,4 s dá activeMs 252400 e activeSeconds 252"
    - "IPC-031/activeMs é truncado (252 400,9 ms dá 252400)"
    - "IPC-031/asOf em UTC com milissegundos"
    - "IPC-031/asOf com ms zero sai com .000Z"
    - "IPC-031/endedAt null em running e grace"
    - "IPC-031/endedAt preenchido em ended"
  - `TestMilliTime`: "IPC-031/ida e volta preserva o milissegundo"
  - `TestGameJSON`: "IPC-020/firstPlayedAt e lastPlayedAt nulos saem como null"
  - `TestDecodeParams`:
    - "IPC-028/array posicional é -32602"
    - "IPC-028/membro desconhecido é -32602"
    - "IPC-028/tipo errado é -32602"
    - "IPC-028/nil não é erro"
    - "IPC-028/dados depois do objeto são -32602"
  - `TestNewErrorData`:
    - "IPC-021/InvalidExecutableData serializa reason em data"
    - "IPC-025/LaunchFailedData sem winError omite o campo"
  - Os casos IPC-001/IPC-002/IPC-008 existentes continuam passando com `NoParams` exportado.
- Depende de: T01

## T05 — IPC: `Server.Notify`
- Requisitos: IPC-029
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `internal/ipc/server.go`: `Notify` e o helper interno `broadcast(msg Request, dropLevel slog.Level)`, usado também por `broadcastHeartbeat`, que mantém `Debug`
  - teste: `internal/ipc/notify_test.go`
- Assinaturas públicas:
  ```go
  // Notify encodes params once and queues the notification on every open connection without
  // blocking. A full queue drops it for that connection only, logging Warn
  // "notificação descartada: fila da conexão cheia" (method, remote). No-op before Serve and after
  // Close; safe from any goroutine. An encoding error is logged (Error) and nothing is sent.
  func (s *Server) Notify(method string, params any)
  ```
- Testes:
  - `TestNotify`:
    - "IPC-029/cada conexão recebe a notificação, sem id"
    - "IPC-029/fila cheia: só aquela conexão perde, com Warn contendo method e remote"
    - "IPC-029/antes do Serve não faz nada"
    - "IPC-029/depois do Close não faz nada"
    - "IPC-029/chamadas concorrentes de várias goroutines" (-race)
    - "IPC-004/heartbeat descartado continua em Debug"
  - `TestShutdown` (goleak): "IPC-006/Notify não cria goroutines"
- Depende de: T04

## T06 — `cmd/fake-game`
- Requisitos: FKG-001, FKG-002, FKG-003, FKG-004, FKG-005, FKG-006, FKG-007, FKG-008
- Responsável: tracker-specialist · Testes: test-writer
- Arquivos:
  - `cmd/fake-game/options.go` (portável: parse e validação)
  - `cmd/fake-game/main.go` (`//go:build windows`): `windows.CreateProcess`, que fecha os handles logo depois; `CREATE_BREAKAWAY_FROM_JOB` com `--breakaway`
  - `cmd/fake-game/main_other.go` (sai com 1)
  - testes:
    - `cmd/fake-game/options_test.go` (portável)
    - `cmd/fake-game/fakegame_windows_test.go`: o `TestMain` compila o binário em pasta temporária
- Assinaturas (pacote `main`, para os testes):
  ```go
  type lifetime struct {
      D       time.Duration
      Forever bool
  }
  type options struct {
      Depth, Fanout, ExitCode int
      Run, ParentRun          lifetime
      UntilFile, Report       string
      MaxLife                 time.Duration // default 2m
      Breakaway               bool
      Rest                    []string // positional args after flags
  }
  func parseOptions(args []string) (options, error) // FlagSet ContinueOnError; "forever" before ParseDuration
  func childArgs(args []string, depth int) []string // same flags, --depth replaced by depth
  type reportLine struct {
      Event string   `json:"event"` // start | exit
      PID   int      `json:"pid"`
      PPID  int      `json:"ppid,omitempty"`
      Depth int      `json:"depth"`
      Cwd   string   `json:"cwd,omitempty"`
      Args  []string `json:"args,omitempty"`
      Code  *int     `json:"code,omitempty"`
      Error string   `json:"error,omitempty"`
  }
  ```
- Testes:
  - `TestParseOptions`:
    - "FKG-007/flag desconhecida é erro"
    - "FKG-007/--depth -1 é erro"
    - "FKG-007/--run abc é erro"
    - "FKG-007/--fanout 0 é erro"
    - "FKG-007/--exit-code 256 é erro"
    - "FKG-007/--max-life 0 é erro"
    - "FKG-005/padrão de --max-life é 2m"
    - "FKG-003/forever é aceito em --run e --parent-run"
  - `TestChildArgs`: "FKG-002/mesmas flags com --depth d-1"
  - `TestFakeGame` (Windows, binário real):
    - "FKG-001/--run 300ms --exit-code 3 sai com 3 depois de ≥ 300 ms, sem filhos"
    - "FKG-002/depth 2 fanout 2: 7 starts (1 com depth 2, 2 com depth 1, 4 com depth 0) e ppid = pid do pai"
    - "FKG-003/padrão lançador: o inicial sai com --exit-code e o filho continua até F"
    - "FKG-004/criar F encerra a árvore em até 500 ms"
    - "FKG-005/--max-life 1s encerra --run forever"
    - "FKG-006/linhas start e exit com pid, ppid, depth, cwd, args e code"
    - "FKG-006/argumentos depois de -- aparecem em args"
    - "FKG-007/flag inválida sai com 2 sem criar relatório"
    - "FKG-008/--breakaway dentro de um job sem BREAKAWAY_OK sai com 4 e grava exit com error"
- Depende de: —

## T07 — tracker: interfaces e máquina de estados pura
- Requisitos: SES-001, SES-003, SES-004, SES-005, SES-006, SES-007, SES-009, SES-011, SES-012, SES-013
- Responsável: tracker-specialist · Testes: test-writer
- Arquivos:
  - `internal/tracker/launcher.go` (portável)
  - `internal/tracker/machine.go` (portável, sem I/O, sem relógio)
  - teste: `internal/tracker/machine_test.go`
- Assinaturas públicas:
  ```go
  package tracker
  type LaunchSpec struct {
      GameID           int64
      Title, Exe, Args string
      WorkDir          string // "" => dir(Exe)
  }
  type Launcher interface{ Prepare(spec LaunchSpec) (Job, error) }
  type JobEventKind int
  const (
      EventNewProcess JobEventKind = iota + 1
      EventExitProcess
      EventActiveZero
  )
  type JobEvent struct {
      Kind     JobEventKind
      PID      uint32
      Root     bool
      ExitCode uint32
  }
  type Job interface {
      RootPID() uint32
      Resume() error
      Terminate() error // only before Resume
      Events() <-chan JobEvent
      ActiveProcesses() (int, error)
      Close() error // never kills processes; idempotent
  }
  type LaunchError struct {
      Err      error
      WinError uint32 // 0 = no Windows code
  }
  func (e *LaunchError) Error() string
  func (e *LaunchError) Unwrap() error
  type SessionStore interface {
      StartSession(ctx context.Context, gameID int64, at time.Time) (int64, error)
      UpdateSession(ctx context.Context, id int64, st domain.SessionState, lastSeen time.Time, active time.Duration) error
      EndSession(ctx context.Context, e store.SessionEnd) (domain.SessionState, error)
      DeleteSession(ctx context.Context, id int64) error
  }

  type State int
  const (
      StateIdle State = iota
      StateLaunching
      StateRunning
      StateGrace
  )
  type EffectKind int
  const (
      EffStartSession EffectKind = iota + 1
      EffUpdateSession
      EffEndSession
      EffDeleteSession
      EffResumeJob
      EffTerminateJob
      EffCloseJob
      EffArmHeartbeat
      EffArmGrace
      EffArmPoll
      EffNotify
  )
  type Effect struct {
      Kind    EffectKind
      State   domain.SessionState     // Update: running|grace; End/Notify: resulting state
      At      time.Time               // Start: started_at; Update: last_seen; End: ended_at
      Active  time.Duration
      Reason  domain.SessionEndReason // End
      Gen     uint64                  // Arm*: generation to pass back
      Delay   time.Duration           // Arm*
      Session domain.SessionSnapshot  // Notify (AsOf = event instant)
  }
  func NewMachine(grace, heartbeat, poll time.Duration) *Machine
  func (m *Machine) State() State
  func (m *Machine) Begin(gameID int64, title string) bool // Idle -> Launching; false otherwise
  func (m *Machine) Prepared(now time.Time) []Effect       // [StartSession]
  func (m *Machine) Inserted(id int64) []Effect            // [ResumeJob]
  func (m *Machine) Resumed() []Effect                     // -> Running: [ArmHeartbeat, ArmPoll, Notify running]
  func (m *Machine) PrepareFailed() []Effect               // -> Idle: []
  func (m *Machine) InsertFailed() []Effect                // -> Idle: [TerminateJob, CloseJob]
  func (m *Machine) ResumeFailed() []Effect                // -> Idle: [TerminateJob, CloseJob, DeleteSession]
  func (m *Machine) ProcessAdded(now time.Time) []Effect
  func (m *Machine) TrackedEmpty(now time.Time) []Effect
  func (m *Machine) Heartbeat(now time.Time, gen uint64) []Effect
  func (m *Machine) GraceTimeout(now time.Time, gen uint64) []Effect // now = AsOf of the end notification
  func (m *Machine) Poll(now time.Time, gen uint64, active int, err error) []Effect
  func (m *Machine) Stop(now time.Time) []Effect // no Notify
  func (m *Machine) Snapshot(now time.Time) (domain.SessionSnapshot, bool) // false in Idle/Launching
  func (m *Machine) Pending() (gameID int64, sessionID int64, title string, ok bool) // for SessionActive
  ```
  As gerações devem dar as contagens de waiters do contrato: Running = 2; Grace = 3 (carência, verificação e o heartbeat antigo).
- Testes (instantes fixos, sem goroutines; uma linha da tabela de transições por caso):
  - `TestMachineLaunch`:
    - "SES-001/Prepared emite StartSession com At = t"
    - "SES-001/Inserted emite ResumeJob"
    - "SES-001/Resumed vai a Running com startedAt = t, arma heartbeat e verificação e notifica running com Active 0 e AsOf t"
    - "SES-012/PrepareFailed volta a Idle sem efeitos"
    - "SES-012/InsertFailed emite TerminateJob e CloseJob"
    - "SES-012/ResumeFailed emite TerminateJob, CloseJob e DeleteSession"
    - "SES-012/Begin fora de Idle devolve false"
  - `TestMachineGrace`:
    - "SES-003/TrackedEmpty em T+100s grava grace, last_seen T+100s, Active 100 s, arma carência e notifica"
    - "SES-004/GraceTimeout da geração atual emite EndSession(ended_at = E, exited), CloseJob e Notify ended"
    - "SES-004/GraceTimeout de geração antiga é ignorado"
    - "SES-005/ProcessAdded em Grace volta a Running com last_seen E+5s e notifica"
    - "SES-005/carência antiga depois da volta é ignorada"
    - "SES-006/sequência T+100, T+105, T+200, T+215 dá 195 s, ended_at T+200s e ended"
    - "SES-007/vazio em T+29s dá discarded com 29 s"
  - `TestMachineHeartbeat`:
    - "SES-009/Heartbeat em Running grava running, last_seen t e o ativo, e rearma"
    - "SES-009/Heartbeat em Grace não grava"
    - "SES-009/Heartbeat de geração antiga é ignorado"
  - `TestMachinePoll`:
    - "SES-013/Running com 0 ativos vai a Grace em t"
    - "SES-013/Grace com > 0 ativos volta a Running em t"
    - "SES-013/erro não muda o estado e rearma"
    - "SES-013/Running com > 0 rearma"
  - `TestMachineStop`:
    - "SES-011/Running: EndSession(ended_at = T, ativo até T, agent-shutdown) e CloseJob, sem Notify nem Terminate"
    - "SES-011/Grace: EndSession(ended_at = E, exited)"
    - "SES-011/Idle não emite nada"
  - `TestMachineSnapshot`:
    - "IPC-031/Running: Active = acumulado + (now − runningSince), AsOf = now"
    - "IPC-031/Grace: Active = acumulado"
    - "IPC-026/Idle e Launching: ok = false"
- Depende de: T01, T03 (tipo `store.SessionEnd`)

## T08 — tracker: coordenador, recuperação e Launcher fora do Windows
- Requisitos: SES-001, SES-002, SES-003, SES-004, SES-005, SES-006, SES-007, SES-009, SES-010, SES-011, SES-012, SES-013, SES-014, SES-015, TRK-008, AGT-011, IPC-026, IPC-027, IPC-031
- Responsável: tracker-specialist · Testes: test-writer
- Arquivos:
  - `internal/tracker/tracker.go`
  - `internal/tracker/recover.go`
  - `internal/tracker/job_other.go` (`//go:build !windows`: `NewLauncher` devolve um Launcher cujo `Prepare` falha com `errors.ErrUnsupported`)
  - testes: `internal/tracker/tracker_test.go`, `internal/tracker/recover_test.go` e `internal/tracker/fakes_test.go`
- Assinaturas públicas:
  ```go
  type Config struct {
      Clock    domain.Clock
      Store    SessionStore
      Launcher Launcher
      Notify   func(domain.SessionSnapshot) // must not block; nil = no-op
      Log      *slog.Logger
      Grace, Heartbeat, Poll time.Duration // zero => domain defaults
  }
  const writeTimeout = 2 * time.Second
  var (
      ErrSessionActive = errors.New("tracker: session already active")
      ErrShuttingDown  = errors.New("tracker: shutting down")
  )
  type SessionActiveError struct{ Session domain.SessionSnapshot } // Is(ErrSessionActive)
  func (e *SessionActiveError) Error() string
  func (e *SessionActiveError) Is(target error) bool
  func New(cfg Config) *Tracker
  func (t *Tracker) Start()
  // Launch returns *SessionActiveError, ErrShuttingDown, *LaunchError (every SES-012 failure,
  // store failures with WinError 0) or ctx.Err() if the caller gave up waiting.
  func (t *Tracker) Launch(ctx context.Context, spec LaunchSpec) (domain.SessionSnapshot, error)
  func (t *Tracker) Current() (domain.SessionSnapshot, bool) // one clock.Now(): AsOf = now
  func (t *Tracker) BeginShutdown()
  func (t *Tracker) Close(ctx context.Context) error // Stop + EndSession with ctx + Job.Close; waits; idempotent
  func NewLauncher() Launcher // job_windows.go / job_other.go

  type RecoveryStore interface {
      OpenSessions(ctx context.Context) ([]store.OpenSession, error)
      EndSession(ctx context.Context, e store.SessionEnd) (domain.SessionState, error)
  }
  func RecoverOpenSessions(ctx context.Context, st RecoveryStore, clock domain.Clock, log *slog.Logger) (int, error)
  ```
  Logs:
  - `Error` "falha ao iniciar o jogo" (`game_id`, `path`, `error`)
  - `Error` "falha ao gravar a sessão" (`session_id`, `error`)
  - `Info` "processo inicial do jogo encerrou" (`session_id`, `pid`, `exit_code`)
  - `Warn` na leitura de `ActiveProcesses` que falha
  - `Warn` "sessões abertas recuperadas" (`count`)
- Testes (`FakeClock` + `BlockUntilWaiters`; `fakeLauncher`/`fakeJob` com canal de eventos e `ActiveProcesses` programáveis; `fakeStore` que registra chamadas e injeta erros):
  - `TestTrackerLaunch`:
    - "SES-001/StartSession acontece antes de Resume"
    - "SES-001/devolve running com startedAt T e activeSeconds 0"
    - "IPC-027/notifica running uma vez"
    - "IPC-031/snapshot devolvido tem Active 0 e AsOf T"
  - `TestTrackerSingleSession`:
    - "SES-002/Running: segundo Launch não chama Prepare nem o store e devolve SessionActiveError com id, gameId e title"
    - "SES-002/Grace: mesmo resultado"
    - "SES-002/Launch concorrente durante um Prepare lento recebe SessionActiveError"
    - "SES-002/a sessão existente não muda"
  - `TestTrackerGrace`:
    - "SES-003/job vazio em T+100s grava grace, last_seen T+100s e 100 s, e notifica grace"
    - "SES-003/em Grace há 3 waiters"
    - "SES-004/15 s − 1 ns não encerra"
    - "SES-004/15 s encerra com ended_at E e exited, fecha o job, Current vazio e aceita novo Launch"
    - "IPC-031/notificação de fim tem AsOf no vencimento e EndedAt E"
    - "SES-005/processo novo em E+5s volta a running, grava e notifica"
    - "SES-005/mais 15 s não encerra"
    - "SES-006/exemplo de 195 s"
    - "SES-007/29 s termina discarded e notifica discarded"
    - "IPC-027/notificações saem na ordem das transições, uma por transição"
  - `TestTrackerHeartbeat`:
    - "SES-009/T+30s grava 30 e T+60s grava 60"
    - "SES-009/30 s − 1 ns não grava"
    - "SES-009/em Grace não grava"
  - `TestTrackerPoll`:
    - "SES-013/Running com 0 ativos e sem mensagem vai a Grace no tick"
    - "SES-013/Grace com > 0 volta a Running"
    - "SES-013/erro de leitura loga Warn e não muda o estado"
  - `TestTrackerLaunchFailure`:
    - "SES-012/Prepare falha: nada gravado, sem notificação, Idle, Error com game_id, path e error, LaunchError com WinError"
    - "SES-012/StartSession falha: Terminate e Close do job, LaunchError com WinError 0"
    - "SES-012/Resume falha: Terminate, Close e DeleteSession"
    - "SES-012/depois da falha um novo Launch é aceito"
  - `TestTrackerWriteFailure`:
    - "SES-014/heartbeat que falha loga Error com session_id e continua Running"
    - "SES-014/entrada em Grace que falha continua em Grace"
    - "SES-014/próxima escrita tenta de novo"
    - "SES-014/fim que falha loga Error, fecha o job e volta a Idle"
  - `TestTrackerRootExit`:
    - "SES-015/saída do processo inicial loga Info com session_id, pid e exit_code"
    - "SES-015/o estado não muda"
  - `TestTrackerCurrent`:
    - "IPC-026/sem sessão devolve false"
    - "IPC-031/Running desde T e consulta em T+252,4s: Active 252,4 s e AsOf T+252,4s"
    - "IPC-031/Grace desde T+300s e consulta em T+310s: Active 300 s e AsOf T+310s"
  - `TestTrackerStop`:
    - "SES-011/Running: Close em T encerra com ended_at T, agent-shutdown, sem Notify nem Terminate"
    - "SES-011/Grace: Close encerra com ended_at E e exited"
  - `TestTrackerShuttingDown`:
    - "AGT-011/depois de BeginShutdown, Launch devolve ErrShuttingDown sem Prepare"
    - "AGT-011/depois de Close, Launch devolve ErrShuttingDown"
  - `TestTrackerClose` (goleak):
    - "TRK-008/Close sem sessão espera a goroutine"
    - "TRK-008/Close com sessão espera a goroutine"
    - "TRK-008/segundo Close não faz nada"
    - "TRK-008/Close sem Start retorna"
  - `TestRecoverOpenSessions`:
    - "SES-010/cada aberta encerra com ended_at = last_seen_at, o ativo gravado e recovered"
    - "SES-010/count e Warn sessões abertas recuperadas"
    - "SES-010/sem abertas não loga Warn"
    - "SES-010/falha em uma sessão é logada e as outras seguem"
    - "SES-010/erro de OpenSessions é devolvido"
    - "SES-010/store real: 590 s vira ended com total +590 e 20 s vira discarded"
- Depende de: T03, T07

## T09 — tracker: Job Object, IOCP e CreateProcess no Windows
- Requisitos: TRK-001, TRK-002, TRK-003, TRK-004, TRK-005, TRK-006, TRK-007, TRK-008, SES-012, SES-013, SES-015
- Responsável: tracker-specialist · Testes: test-writer
- Arquivos:
  - `internal/tracker/job_windows.go`: o `NewLauncher` real e `quoteExe` duplicado (decisão 11). Procs que faltarem em `x/sys/windows` são declarados localmente.
  - testes:
    - `internal/tracker/main_windows_test.go`: o `TestMain` compila `cmd/fake-game` em pasta temporária com espaço no nome
    - `job_windows_test.go` e `integration_windows_test.go`
- Assinaturas públicas: `func NewLauncher() Launcher`. Os demais tipos são privados (`winJob` implementa `Job`).
- Testes (`--max-life 30s`; `t.Cleanup` cria o `--until-file`; espera real ≤ 10 s; `FakeClock` no tracker; store real em `t.TempDir()`):
  - `TestJobTree`:
    - "TRK-001/processo inicial e filho estão no job (IsProcessInJob)"
    - "TRK-001/job emite 2 EventNewProcess"
  - `TestTrackerIntegration`:
    - "TRK-002/lançador sai e a sessão continua Running"
    - "TRK-002/criar F faz a sessão ir a Grace"
    - "TRK-003/árvore de 7 processos continua Running até a última folha sair"
    - "TRK-005/cwd = D, args de launch_args sem alteração e exe com espaços entre aspas"
    - "TRK-006/filho com breakaway é criado fora do job e a sessão vai a Grace quando o inicial sai"
    - "SES-015/--exit-code 7 loga exit_code 7"
  - `TestTrackerCloseKeepsGame`: "TRK-004/depois de Close todos os processos continuam vivos (GetExitCodeProcess = STILL_ACTIVE)"
  - `TestLaunchFailureWindows`:
    - "SES-012/exe inexistente devolve LaunchError com WinError 2 e não deixa processo"
    - "SES-012/Terminate antes do Resume encerra o processo suspenso"
  - `TestActiveProcesses`: "SES-013/ActiveProcesses conta os vivos e chega a 0"
  - `TestHandleCount`: "TRK-007/20 lançamentos de --run 0 com carência vencida e Close terminam com ≤ h0 + 2 handles"
  - `TestJobGoroutines` (goleak): "TRK-008/Close do job espera a leitora da IOCP"
- Depende de: T06, T08

## T10 — agente: handlers `game.*` e `session.current`
- Requisitos: GAM-001, GAM-002, GAM-003, GAM-005, IPC-020, IPC-021, IPC-023, IPC-024, IPC-025, IPC-026, IPC-028, IPC-031, AGT-011
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `internal/agent/games.go` (novo)
  - teste: `internal/agent/games_test.go`, com `gameStore`/`sessionTracker`/`stat` falsos e chamadas via `ipc.Dispatch`
- Assinaturas:
  ```go
  const launchHandlerTimeout = 10 * time.Second
  type gameStore interface {
      AddGame(ctx context.Context, g store.NewGame) (store.Game, error)
      Games(ctx context.Context) ([]store.Game, error)
      Game(ctx context.Context, id int64) (store.Game, error)
  }
  type sessionTracker interface {
      Launch(ctx context.Context, spec tracker.LaunchSpec) (domain.SessionSnapshot, error)
      Current() (domain.SessionSnapshot, bool)
  }
  func gameHandlers(st gameStore, tr sessionTracker, clock domain.Clock,
      stat func(string) (fs.FileInfo, error), log *slog.Logger) map[string]ipc.Handler
  func validateExe(p string, stat func(string) (fs.FileInfo, error)) (clean, reason string, err error) // decisão 3
  func gameTitle(title *string, clean string) (string, bool) // false => > MaxGameTitleRunes
  func parseGameID(raw json.RawMessage) (int64, bool)        // decisão 2
  func toIPCGame(g store.Game) ipc.Game
  ```
  O caso `launch_type ≠ 'exe'` devolve `-32003` com error "tipo de lançamento não suportado".
- Testes:
  - `TestGameAdd`:
    - "IPC-020/devolve o Game com totais 0 e datas null"
    - "GAM-001/AddGame recebe exe limpo, pasta do exe e At do relógio"
    - "GAM-001/o mesmo caminho duas vezes é aceito"
    - "IPC-021/relativo devolve -32004 not-absolute"
    - "IPC-021/inexistente devolve -32004 not-found"
    - "IPC-021/pasta devolve -32004 not-a-file"
    - "IPC-021/.txt devolve -32004 not-exe"
    - "GAM-002/.EXE maiúsculo é aceito"
    - "GAM-002/inválido não chama AddGame"
    - "GAM-003/título com espaços é cortado"
    - "GAM-003/ausente ou só espaços usa o título padrão"
    - "GAM-003/200 runas aceito"
    - "GAM-003/201 runas devolve -32602 sem gravar"
  - `TestGameList`:
    - "IPC-023/games na ordem do store"
    - "IPC-023/vazio sai como [] e não null"
    - "GAM-005/a ordem do store é preservada"
  - `TestGameLaunch`:
    - "IPC-024/devolve Session running com endedAt null e activeSeconds 0"
    - "IPC-031/a Session tem activeMs e asOf do snapshot"
    - "IPC-025/gameId inexistente devolve -32001 sem chamar Launch"
    - "IPC-025/sessão ativa devolve -32002 com sessionId, gameId e title"
    - "IPC-025/LaunchError com WinError devolve -32003 com error e winError"
    - "IPC-025/LaunchError sem WinError omite winError"
    - "AGT-011/ErrShuttingDown devolve -32003 com error agente encerrando"
    - "IPC-024/LaunchSpec montado de launch_target, launch_args e working_dir"
  - `TestSessionCurrent`:
    - "IPC-026/sem sessão devolve session null"
    - "IPC-026/com sessão devolve state e o ativo do snapshot"
  - `TestNewMethodParams`:
    - "IPC-028/gameId texto devolve -32602"
    - "IPC-028/gameId fracionário devolve -32602"
    - "IPC-028/gameId 0 devolve -32602"
    - "IPC-028/path ausente devolve -32602"
    - "IPC-028/path não texto devolve -32602"
    - "IPC-028/array posicional devolve -32602"
    - "IPC-028/membro desconhecido devolve -32602"
    - "IPC-028/game.list e session.current com params não vazios devolvem -32602"
    - "IPC-028/vazios são aceitos"
- Depende de: T02, T04, T08

## T11 — agente: subida com recuperação e tracker, shutdown com sessão e `session.changed`
- Requisitos: AGT-009, AGT-010, AGT-011, SES-008, SES-010, SES-011, IPC-024, IPC-027, AGT-003, AGT-006
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `internal/agent/app.go`:
    - `config` ganha `Launcher tracker.Launcher` e `Recover func(context.Context, tracker.RecoveryStore, domain.Clock, *slog.Logger) (int, error)`
    - `instance` ganha `tracker *tracker.Tracker` e `notifier *sessionNotifier`
    - ordem do `startAgent`: store → recuperação → tracker `Start` → listen → servidor (`AgentHandlers` + `gameHandlers`) → `notifier.set` → `Serve`
    - caminhos de erro depois do `Start` chamam `tracker.Close`
    - `Shutdown`: `BeginShutdown` → `Drain` → `Server.Close` → `wg.Wait` → `tracker.Close(ctx)` → `Store.Close`
  - `internal/agent/notifier.go` (novo)
  - `run_windows.go`: sem mudança de comportamento (só confere a ordem)
  - testes: `internal/agent/tracker_wiring_test.go` (novo) e ajustes em `app_test.go`/`shutdown_test.go`, que passam um `fakeLauncher`
- Assinaturas:
  ```go
  type sessionNotifier struct{ srv atomic.Pointer[ipc.Server] }
  func (n *sessionNotifier) set(s *ipc.Server)
  func (n *sessionNotifier) Notify(s domain.SessionSnapshot) // nil srv => no-op; ipc.SessionChangedParams{SessionFromSnapshot(s)}
  ```
- Testes (`FakeClock`, `fakeLauncher`/`fakeJob`, store real em `t.TempDir()`, clientes por `ln.listen`):
  - `TestStartupRecovery`:
    - "AGT-009/sessões abertas já estão fechadas quando listen é chamado"
    - "AGT-009/o tracker é iniciado antes do Serve"
    - "AGT-009/Recover que falha loga Error e a subida continua"
    - "SES-010/recuperação na subida não gera notificação"
  - `TestSessionChangedBroadcast`:
    - "IPC-027/dois clientes recebem running, grace e ended, na ordem e sem id"
    - "IPC-027/discarded é notificado"
    - "IPC-027/lançamento que falhou não notifica"
    - "IPC-024/game.launch fim a fim devolve a Session running"
  - `TestSessionTotals`: "SES-008/sessão de 120 s aparece em game.list com totalPlaySeconds 120, playCount 1 e lastPlayedAt = ended_at"
  - `TestShutdownWithSession`:
    - "AGT-010/ordem Drain, Server.Close, Tracker.Close e Store.Close" (o cliente recebe agent.shutdown e nenhum session.changed depois; o banco reaberto mostra a sessão encerrada)
    - "SES-011/sessão Running encerra com agent-shutdown e o tempo até o shutdown"
    - "SES-011/Job.Terminate não é chamado no shutdown"
    - "IPC-027/encerramento no shutdown não notifica"
    - "AGT-003/Shutdown com sessão termina em < 2 s"
  - `TestLaunchDuringShutdown`: "AGT-011/game.launch durante o drain devolve -32003 agente encerrando sem chamar Prepare"
  - os testes existentes de AGT-003, AGT-006 e EXE-013/EXE-014 continuam passando com o tracker ligado
- Depende de: T05, T08, T10

## T12 — UI Go: bindings, mensagens de erro e repasse de `session.changed`
- Requisitos: DSK-010, DSK-011, DSK-012
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `cmd/arcadia/games.go` (novo)
  - `cmd/arcadia/errors.go` (novo, puro)
  - `cmd/arcadia/supervisor.go`: `case ipc.MethodSessionChanged`
  - `cmd/arcadia/app.go`: `platform.openFile`
  - `cmd/arcadia/main_windows.go`: `openFile` com `wailsruntime.OpenFileDialog`
  - `cmd/arcadia/main_other.go`
  - testes: `cmd/arcadia/games_test.go`, `errors_test.go` e `supervisor_test.go`
  - **`cd cmd/arcadia && wails generate module`**, que atualiza `frontend/wailsjs/go/main/App.{d.ts,js}` e `models.ts`
- Assinaturas:
  ```go
  const (
      addGameTimeout        = 5 * time.Second
      listGamesTimeout      = 2 * time.Second
      launchGameTimeout     = 12 * time.Second
      currentSessionTimeout = 2 * time.Second
      sessionChangedEvent   = "session:changed"
      pickExeTitle          = "Escolha o executável do jogo"
  )
  type GameView struct {
      ID               int64  `json:"id"`
      Title            string `json:"title"`
      Path             string `json:"path"`
      TotalPlaySeconds int64  `json:"totalPlaySeconds"`
      PlayCount        int64  `json:"playCount"`
      LastPlayedAt     string `json:"lastPlayedAt"` // RFC 3339 or ""
  }
  type SessionView struct {
      SessionID     int64  `json:"sessionId"`
      GameID        int64  `json:"gameId"`
      Title         string `json:"title"`
      State         string `json:"state"`
      StartedAt     string `json:"startedAt"`
      EndedAt       string `json:"endedAt"` // RFC 3339 or ""
      ActiveSeconds int64  `json:"activeSeconds"`
      ActiveMs      int64  `json:"activeMs"`
      AsOf          string `json:"asOf"` // RFC 3339 UTC with ms
  }
  type PickedExe struct {
      Path  string `json:"path"`
      Title string `json:"title"`
  }
  type fileFilter struct{ DisplayName, Pattern string }
  // platform.openFile: func(ctx context.Context, title string, filters []fileFilter) (string, error) // "" = cancelled
  func gameView(g ipc.Game) GameView
  func sessionView(s ipc.Session) (SessionView, error) // error when state fails domain.ParseSessionState
  func (a *App) PickGameExe() (PickedExe, error)
  func (a *App) AddGame(path, title string) (GameView, error)
  func (a *App) ListGames() ([]GameView, error) // never nil
  func (a *App) LaunchGame(id int64) (SessionView, error)
  func (a *App) CurrentSession() (*SessionView, error)
  var errNoClient = errors.New("desktop: agent client not ready")
  func userError(err error, log *slog.Logger) error // errors.New(<texto de DSK-011>)
  ```
- Testes:
  - `TestUserError`:
    - "DSK-011/-32001 Jogo não encontrado. Atualize a lista."
    - "DSK-011/-32002 com title"
    - "DSK-011/-32003 com error"
    - "DSK-011/-32004 not-absolute e not-found dão Arquivo não encontrado."
    - "DSK-011/-32004 not-a-file"
    - "DSK-011/-32004 not-exe"
    - "DSK-011/-32602 Dados inválidos."
    - "DSK-011/ErrConnectionLost, ErrClientClosed, DeadlineExceeded e errNoClient dão O agente não está respondendo."
    - "DSK-011/outro erro dá Erro inesperado. Veja o log."
    - "DSK-011/-32001, -32602 e erro inesperado logam Error"
    - "DSK-011/-32002..-32004 e erros de conexão não logam Error"
  - `TestPickGameExe`:
    - "DSK-012/diálogo com título e filtro Executáveis (*.exe) / *.exe"
    - "DSK-012/cancelar devolve vazio sem chamar o agente"
    - "DSK-012/arquivo escolhido devolve path e título padrão"
  - `TestGameBindings`:
    - "DSK-010/LaunchGame e CurrentSession devolvem SessionView com endedAt vazio, activeMs e asOf"
    - "DSK-010/CurrentSession com session null devolve nil"
    - "DSK-011/ListGames sem cliente devolve O agente não está respondendo."
    - "DSK-010/ListGames vazio devolve [] e não nil"
    - "DSK-010/AddGame envia path e title"
  - `TestSupervisorSessionChanged`:
    - "DSK-010/session.changed emite session:changed com a SessionView"
    - "DSK-010/params ausentes ou inválidos logam Warn e não emitem"
    - "DSK-010/state desconhecido loga Warn e não emite"
- Depende de: T04, T11

## T13 — frontend: `lib/games.ts` e `lib/sessionClock.ts`
- Requisitos: UI-005, UI-006, UI-009, UI-013, UI-014, UI-015, UI-016, UI-017, UI-018
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `frontend/src/lib/games.ts` (novo)
  - `frontend/src/lib/sessionClock.ts` (novo)
  - testes: `frontend/src/lib/games.test.tsx` e `frontend/src/lib/sessionClock.test.ts`
- Assinaturas:
  ```ts
  export type GameView = {
    id: number; title: string; path: string
    totalPlaySeconds: number; playCount: number; lastPlayedAt: string
  }
  export type SessionState = 'running' | 'grace' | 'ended' | 'discarded'
  export type SessionView = {
    sessionId: number; gameId: number; title: string; state: SessionState
    startedAt: string; endedAt: string
    activeSeconds: number; activeMs: number; asOf: string
  }
  export type PickedExe = { path: string; title: string }
  export const gamesKey: readonly ['games']
  export const currentSessionKey: readonly ['session', 'current']
  export function formatPlayTime(seconds: number): string
  export function listGames(): Promise<GameView[]>              // rejections normalized to Error(String(e))
  export function currentSession(): Promise<SessionView | null>
  export function addGame(path: string, title: string): Promise<GameView>
  export function launchGame(id: number): Promise<SessionView>
  export function pickGameExe(): Promise<PickedExe>
  export function sessionEventSeq(qc: QueryClient): number      // per-QueryClient counter (WeakMap)
  export function useGames(): UseQueryResult<GameView[], Error>   // retry false; refetchInterval 2000 only on error
  export function useCurrentSession(): UseQueryResult<SessionView | null, Error> // no polling; UI-018 rule
  export function useSessionEvents(): void                        // EventsOn('session:changed')
  export function useReloadOnReconnect(connected: boolean): void  // false->true: invalidate games, reset current
  export function useLaunchGame(): UseMutationResult<SessionView, Error, number> // applies result only if seq unchanged

  export type SessionAnchor = { activeMs: number; state: 'running' | 'grace'; receivedAt: number }
  export function formatElapsed(seconds: number): string // H:MM:SS
  export function anchorFrom(s: SessionView, now?: () => number): SessionAnchor
  export function useElapsedSeconds(anchor: SessionAnchor | null, now?: () => number): number
  ```
- Testes (Vitest; `window.go.main.App` e `window.runtime.EventsOn` falsos; `vi.useFakeTimers` com `performance`):
  - `formatPlayTime`:
    - "UI-005 0 é Nunca jogado"
    - "UI-005 1 e 59 são < 1 min"
    - "UI-005 60 é 1 min e 3599 é 59 min"
    - "UI-005 3600 é 1 h 00 min, 3900 é 1 h 05 min e 451800 é 125 h 30 min"
  - `formatElapsed`:
    - "UI-013 0, 59, 252, 3600, 45005 e 450000"
  - `useElapsedSeconds`:
    - "UI-014 A = 252400 mostra 0:04:12; +599 ms continua 0:04:12; +1 ms vira 0:04:13; +1000 ms vira 0:04:14"
    - "UI-014 âncora nova recomeça sem somar a anterior"
    - "UI-014 não usa startedAt nem asOf" (Date falso adiantado não muda o valor)
    - "UI-015 grace fica parado ao avançar o tempo"
    - "UI-015 grace para running volta a avançar do activeMs novo"
    - "UI-017 10 min avançando não chama nenhum binding"
    - "UI-017 desmontar limpa o temporizador"
  - `useSessionEvents`:
    - "UI-006 running e grace gravam a sessão no cache"
    - "UI-006 ended e discarded limpam a sessão e invalidam games"
  - `useCurrentSession`:
    - "UI-018 evento ended antes da resposta running atrasada mantém null"
    - "UI-018 sem evento a resposta é aplicada"
  - `useLaunchGame`: "UI-018 resultado de LaunchGame ignorado se chegou session:changed depois do pedido"
  - `useReloadOnReconnect`:
    - "UI-009 Desconectado para Conectado recarrega games e a sessão"
    - "UI-016 a sessão antiga é descartada até a recarga"
- Depende de: T12

## T14 — frontend: componentes e `App`
- Requisitos: UI-004, UI-006, UI-007, UI-008, UI-009, UI-010, UI-012, UI-014, UI-015, UI-016, UI-017, UI-018
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `frontend/src/components/GameList.tsx`
  - `frontend/src/components/AddGameForm.tsx`
  - `frontend/src/components/ErrorBanner.tsx`
  - `frontend/src/components/SessionPanel.tsx`
  - `frontend/src/App.tsx`: sai `LibraryView`/`useLibrarySummary` (decisão 10); o painel aparece só com `connected && current`
  - testes: `GameList.test.tsx`, `AddGameForm.test.tsx`, `ErrorBanner.test.tsx`, `SessionPanel.test.tsx` e `frontend/src/App.test.tsx`
- Assinaturas:
  ```ts
  export function GameList(p: { games: GameView[]; current: SessionView | null;
    pendingId: number | null; onPlay: (id: number) => void }): JSX.Element
  export function AddGameForm(p: { picked: PickedExe; pending: boolean;
    onSave: (title: string) => void; onCancel: () => void }): JSX.Element
  export function ErrorBanner(p: { message: string; onClose: () => void }): JSX.Element
  export function SessionPanel(p: { session: SessionView; now?: () => number }): JSX.Element // role="status", aria-label="Sessão em andamento"
  ```
- Testes:
  - `GameList`:
    - "UI-004 uma linha por jogo na ordem recebida, com título, total e Jogar"
    - "UI-006 Em execução só na linha do gameId da sessão"
    - "UI-008 Jogar pendente mostra Iniciando… desabilitado"
  - `AddGameForm`:
    - "UI-007 caminho e Título preenchidos com maxLength 200"
    - "UI-007 Salvar desabilitado com título só de espaços"
    - "UI-007 Salvar desabilitado durante o envio"
    - "UI-007 Cancelar chama onCancel"
  - `ErrorBanner`: "UI-008 mostra a mensagem e Fechar chama onClose"
  - `SessionPanel`:
    - "UI-012 role status com nome Sessão em andamento, título e cronômetro"
    - "UI-014 avança a cada segundo"
    - "UI-015 grace mostra Encerrando… e congela"
  - `App`:
    - "UI-010 [] mostra Nenhum jogo ainda sem contadores, com Adicionar jogo"
    - "UI-010 erro mostra Conectando ao agente… e tenta de novo em 2 s"
    - "UI-007 escolher exe abre o formulário; salvar fecha e o jogo aparece"
    - "UI-007 cancelar no diálogo não muda nada"
    - "UI-008 falha de Jogar mostra a mensagem no aviso"
    - "UI-008 erro novo substitui o anterior"
    - "UI-008 ação bem-sucedida remove o aviso"
    - "UI-006 CurrentSession ao carregar marca Em execução"
    - "UI-006 ended remove a marcação e recarrega a lista"
    - "UI-012 sem sessão o painel não existe"
    - "UI-012 discarded remove o painel"
    - "UI-016 Desconectado esconde o painel"
    - "UI-016 ao reconectar o painel volta só com o valor recarregado"
    - "UI-009 reconectar chama ListGames e CurrentSession"
    - "UI-017 10 min com painel não chamam bindings extras e o total só muda depois de ended"
    - "UI-018 evento antes da resposta atrasada de CurrentSession vence"
- Depende de: T13

## T15 — Documentação
- Requisitos: GAM-001, STO-011, AGT-010, UI-010, IPC-030
- Responsável:
  - sessão principal: `CLAUDE.md` e `docs/ARCHITECTURE.md`
  - spec-writer: as anotações em `docs/specs/phase-2-skeleton/requirements.md` e `docs/specs/single-exe/requirements.md`
- Arquivos:
  - `CLAUDE.md`: link "Spec atual" para `docs/specs/phase-3-tracker/`; nota de que o agente é o único escritor do banco (ADR 0015); `cmd/fake-game` no layout
  - `docs/ARCHITECTURE.md`: §2 (divisão de escrita, ADR 0015), §4.3 (sem Paused e sem timeout de Launching na Camada 1, apontando para o contrato), breakaway (ADR 0016)
  - `docs/specs/phase-2-skeleton/requirements.md`:
    - "alterado em 2026-11-09" em STO-001, AGT-003 e AGT-006
    - UI-002: "substituído por UI-010 (docs/specs/phase-3-tracker)"
  - `docs/specs/single-exe/requirements.md`: "alterado em 2026-11-09" em AST-001 (versão 3)
  - `frontend/src/lib/agent.ts`: comentário obsoleto sobre os bindings
- Testes: nenhum automatizado
- Depende de: T11, T14

## T16 — Verificação final e checagens manuais
- Requisitos: UI-011, TRK-004, TRK-006, AGT-003, AGT-006, AGT-010, SES-010, SES-011, BLD-001
- Responsável: reviewer (os builds ficam com o implementer)
- Roteiro:
  - **BLD-001:**
    - `CGO_ENABLED=0 go build ./...`, `go test ./...` (com `-race` em `internal/tracker` e `internal/ipc`), `go vet ./...` e `GOOS=linux go vet ./...` passam
    - `cd frontend && npm test && npm run build` passa
    - `wails build -ldflags "-X main.version=1.2.3"` gera `arcadia.exe`
  - **Migração:** um banco v2 real abre na versão 3 sem perder dados.
  - **UI-011:** jogo real com lançador que abre o jogo e fecha, jogado por mais de 1 min:
    - "Em execução" e o cronômetro avançam durante o jogo
    - ~15 s depois de fechar, a marcação e o painel somem e o total mostra o tempo jogado
    - fechar e reabrir a UI durante o jogo não altera o tempo gravado e o cronômetro volta no valor certo (±1 s)
  - **TRK-004 / SES-011 / AGT-010:** "Sair" na bandeja com um jogo aberto:
    - o jogo continua aberto
    - a sessão fica `agent-shutdown`
    - o agente termina em < 2 s (AGT-003)
  - **AGT-006:** logoff com um jogo aberto encerra a sessão do mesmo jogeito (conferir no banco).
  - **SES-010:** `Stop-Process` no agente durante o jogo; a próxima subida loga "sessões abertas recuperadas" e o total soma o último heartbeat.
  - **TRK-006 (fumaça):** `fake-game --depth 1 --breakaway ...` pelo agente; a sessão termina quando o inicial sai.
  - **Erros visíveis:** lançar com um jogo ativo mostra a mensagem -32002; um exe com `requireAdministrator` mostra "Não foi possível iniciar o jogo" (Limitação 4).
  - **Rastreabilidade:** `spec-trace.ps1` sai com 0 para `phase-3-tracker`, `single-exe` e `phase-2-skeleton`.
- Depende de: T01–T15

---

## Riscos de concorrência e recursos

| Goroutine / recurso | Dono | Como termina / quem fecha |
|---|---|---|
| Coordenador do tracker (`run`) | `Tracker.wg` (iniciada em `Start`) | canal `stop` fechado por `Close`; antes de sair aplica `Stop`, grava o fim e fecha o job |
| Leitora da IOCP (uma por job) | o `winJob` | pacote `stopKey` postado por `Close`, que espera por ela; fecha `Events()` ao sair |
| `Serve`, heartbeat e conexões IPC | `instance.wg` / `Server.wg` | como na single-exe; `Notify` não cria goroutine |
| `Launch` / `Current` | quem chama | não criam goroutine; `Launch` espera resposta ou `ctx`; `Current` lê o snapshot sob `sync.Mutex` |
| Processos do `fake-game` nos testes | o teste | `--until-file` criado no `t.Cleanup` e `--max-life 30s` |
| Handles do job: job, porta IOCP, `hProcess`, `hThread` | o `winJob` | `hThread` fechado logo depois do `Resume`; o resto em `Close` (`sync.Once`); os caminhos de erro do `Prepare` fecham tudo que criaram; TRK-007 mede |
| Handles dos filhos no `fake-game` | o `fake-game` | fechados logo depois de `CreateProcess` |
| Cronômetro da UI (`setTimeout`) | o `useEffect` de `useElapsedSeconds` | limpo no cleanup; nenhum em `grace`, sem âncora ou desmontado |

- **Ordem de shutdown do agente:**
  1. `systray.Quit`
  2. `tracker.BeginShutdown`
  3. `Drain` (≤ 500 ms)
  4. `Server.Close`
  5. `wg.Wait`
  6. `tracker.Close(ctx)`: uma escrita de fim, ≤ 2 s
  7. `Store.Close`
  8. log "shutdown concluído"
  9. watcher
  10. mutex
  11. `Close` do log
- **Dependência circular tracker ↔ servidor:** `sessionNotifier` com `atomic.Pointer`. `Notify` antes do `set` e depois do `Close` não faz nada.
- **Leitora bloqueada:** se o coordenador para de ler `Events()`, a leitora espera no `select` com o canal de parada; a IOCP guarda os pacotes no kernel. `EventActiveZero` nunca é descartado.
- **Lock:** o mutex do snapshot só é tomado pelo coordenador (escrita) e por `Current` (leitura); nenhuma chamada externa acontece com ele tomado. `Notify` é chamado sem lock.
- **`FakeClock`:** temporizadores não canceláveis, com geração. Os testes usam as contagens de waiters do contrato (Running 2, Grace 3).
- **Despacho sequencial:** um `game.launch` lento atrasa o `agent.status` da mesma conexão (Limitação 10); aceito.

## Rastreabilidade

| Requisito | Tarefa(s) |
|---|---|
| DOM-007, DOM-008, DOM-009 | T01 |
| GAM-001 | T02, T10, T15 |
| GAM-002, GAM-003 | T10 |
| GAM-005 | T02, T10 |
| STO-011 | T02, T15 |
| STO-012..STO-017 | T03 |
| SES-001 | T07, T08 |
| SES-002 | T08 |
| SES-003..SES-007, SES-009 | T07, T08 |
| SES-008 | T11 |
| SES-010 | T08, T11, T16 |
| SES-011 | T07, T08, T11, T16 |
| SES-012 | T07, T08, T09 |
| SES-013 | T07, T08, T09 |
| SES-014 | T08 |
| SES-015 | T08, T09 |
| TRK-001..TRK-003, TRK-005, TRK-007 | T09 |
| TRK-004, TRK-006 | T09, T16 |
| TRK-008 | T08, T09 |
| AGT-009 | T11 |
| AGT-010 | T11, T15, T16 |
| AGT-011 | T08, T10, T11 |
| IPC-020, IPC-021 | T04, T10 |
| IPC-023 | T10 |
| IPC-024 | T10, T11 |
| IPC-025 | T04, T10 |
| IPC-026 | T08, T10 |
| IPC-027 | T08, T11 |
| IPC-028 | T04, T10 |
| IPC-029 | T05 |
| IPC-030 | T04, T15 |
| IPC-031 | T04, T07, T08, T10 |
| DSK-010, DSK-011, DSK-012 | T12 |
| UI-004, UI-007, UI-008, UI-010, UI-012 | T14 |
| UI-005, UI-013 | T13 |
| UI-006, UI-009, UI-014..UI-018 | T13, T14 |
| UI-011 (manual) | T16 |
| FKG-001..FKG-008 | T06 |
| Afetados de outras fases: STO-001, AST-001 (T02); AGT-003, AGT-006 (T11, T16); UI-002 (T15) | — |

## Decisões novas que merecem ADR
Nenhuma. As decisões 1–12 são de implementação e cabem nesta spec. Se a sessão principal preferir, a decisão 7 (níveis de log diferentes no descarte de notificação e de heartbeat) pode virar nota no design.md.

