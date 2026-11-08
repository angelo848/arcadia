# Fase 2 — Esqueleto: design

Implementa [requirements.md](requirements.md). Base: [ARCHITECTURE.md](../../ARCHITECTURE.md), ADRs 0002–0006,
0009 (timestamps em milissegundos UTC) e 0010 (só o agente migra o schema).
Os IDs entre colchetes indicam os requisitos atendidos.

## Módulo e estrutura [BLD-001]
- `go.mod`: `github.com/arcadia-launcher/arcadia` (provisório), `go 1.27`.
- Só são criados os pacotes usados nesta fase (sem `doc.go` vazio).
- `.gitignore`: `build/bin/`, `frontend/node_modules/`, `frontend/dist/*` (exceto `.gitkeep`), `.devdata/`, `*.log`.
- `frontend/dist/.gitkeep` versionado, para `go build ./...` funcionar sem Node.

## `internal/paths` [PTH-001..003]
- `Resolve(opts Options) (Paths, error)`; `Options{Flag string; ExeDir string; Getenv func(string) string}` —
  tudo injetado para testar sem tocar no ambiente real.
- `Paths{Root, DB, Media, Logs string}`; `(Paths) EnsureDirs() error`.

## `internal/domain` [DOM-001..006]
- `Clock` interface:
  ```go
  type Clock interface {
      Now() time.Time
      After(d time.Duration) <-chan time.Time
  }
  ```
- `SystemClock`: `time.Now()` / `time.After(d)` (único lugar autorizado a chamá-los).
- `FakeClock` (seguro para uso concorrente; `sync.Mutex` + `sync.Cond` ou canal de sinal):
  - `NewFakeClock(start time.Time) *FakeClock`.
  - `After(d)`: devolve canal com buffer 1. `d <= 0` → envia `Now()` na hora e não registra waiter. `d > 0` →
    registra waiter com prazo `now + d` [DOM-006].
  - `Advance(d)`: soma `d` a `now`, depois dispara (envio não bloqueante, o buffer garante) todo waiter com
    prazo `<= now` e o remove; envia o `now` resultante [DOM-005, DOM-006].
  - `BlockUntilWaiters(ctx context.Context, n int) error`: espera até haver pelo menos `n` waiters pendentes ou o
    `ctx` terminar (retorna `ctx.Err()`). Implementação deve acordar a cada registro de waiter, sem polling por tempo
    real [DOM-006].
- `Interval{Start, End time.Time}` (End zero = aberta).
- `ActiveDuration(start, end time.Time, pauses []Interval) time.Duration`: descarta pausas fechadas com
  `End < Start`, trata `End` zero como `end`, recorta em `[start,end]`, ordena e mescla as sobrepostas, soma e
  subtrai. `end < start` → 0 [DOM-001..004].

## `internal/store` [STO-001..010]
- Assinatura:
  ```go
  type OpenOptions struct {
      Migrate bool // true só no agente (ADR 0010)
  }
  func Open(ctx context.Context, path string, opts OpenOptions) (*Store, error)
  func (s *Store) Close() error
  var ErrSchemaMismatch = errors.New("store: schema version mismatch")
  const SchemaVersion int64 = 1 // última migração embutida no binário
  ```
- `modernc.org/sqlite`; DSN com pragmas `journal_mode(WAL)`, `busy_timeout(5000)`, `foreign_keys(1)`,
  `synchronous(NORMAL)` e `_txlock=immediate` (transações de escrita pegam o lock na abertura, evitando
  `SQLITE_BUSY` em upgrade de lock) [STO-004, STO-005].
  - `_txlock=immediate` vale para **todo** `BeginTx`, inclusive transações só de leitura: qualquer `Tx` pega o
    lock de escrita. Use `Tx` só para escrita; leituras vão direto no `*sql.DB` (fora de transação).
    · alterado em 2026-11-08: escopo do `_txlock` explicitado (revisão da T04, aprovado pelo usuário).
- `Migrate: true` → goose (`embed.FS`) `Up` até `SchemaVersion` [STO-001, STO-002].
  - Banco com versão **maior** que `SchemaVersion` (criado por binário mais novo): `Up` não faz nada; em seguida
    `Open` confere a versão, fecha o `*sql.DB` e retorna `ErrSchemaMismatch` — o agente se recusa a rodar.
    · alterado em 2026-11-08: caso de banco mais novo definido (revisão da T04, aprovado pelo usuário).
- `Migrate: false` [STO-008..010]:
  - arquivo inexistente → `ErrSchemaMismatch` sem criar o arquivo (checa com `os.Stat` antes e abre com `mode=rw`).
  - lê a versão atual do goose (`goose.GetDBVersion` ou consulta direta a `goose_db_version`), sem escrever; se a
    tabela não existir ou a versão ≠ `SchemaVersion` → fecha o `*sql.DB` e retorna
    `fmt.Errorf("...: %w", ErrSchemaMismatch)` com as duas versões na mensagem.
  - Nenhuma chamada que crie `goose_db_version` (ex.: `goose.EnsureDBVersion`) pode ser usada nesse caminho.
  - "Não alterar o banco" refere-se ao **schema** e a `goose_db_version`. O pragma `journal_mode(WAL)` do DSN pode
    converter o journal de um arquivo não criado pelo Arcadia para WAL e criar `-wal`/`-shm` ao lado dele; isso é
    aceito (todo banco do Arcadia já nasce em WAL, então no caso normal nada muda).
    · alterado em 2026-11-08: alcance de "não alterar" explicitado (revisão da T04, aprovado pelo usuário).
- `00001_initial.sql` = schema completo do §5, FTS5, seeds de `play_mode`, índices `game(platform_id)` e
  `play_session(game_id, started_at)` [STO-001, STO-003].
  · alterado em 2026-11-08: decisões abaixo registradas após a revisão da T04 (aprovadas pelo usuário).
  - **Enums sem CHECK no banco**: `game.status`, `game.launch_type`, `media_asset.kind`, `platform.category`,
    `pause_interval.reason`, `tracking_rule.kind` e `play_session.source` são `TEXT` livre no schema; a validação
    dos valores fica no `domain`. Motivo: no SQLite mudar um CHECK exige reconstruir a tabela; `game` tem várias
    filhas com `ON DELETE CASCADE` e o goose roda cada migração numa transação, onde `PRAGMA foreign_keys=OFF` não
    tem efeito — a reconstrução dispararia os CASCADE. `CHECK (col IN (0,1))` em colunas booleanas é permitido.
  - **UNIQUE em `name`** de `platform`, `company`, `genre` e `play_mode` (chaves naturais para dedup dos
    importadores; colação `BINARY`, ou seja, sensível a maiúsculas).
  - **Índices adicionais** (além dos dois exigidos): `media_asset(game_id, kind)`, `tracking_rule(game_id)`,
    `pause_interval(session_id)`.
  - **FKs**: `ON DELETE CASCADE` nas tabelas filhas de `game` e de `play_session`; `ON DELETE RESTRICT` em
    `game.platform_id`; `ON DELETE SET NULL` em `platform.emulator_id`.
- **Tempo (ADR 0009)** [STO-006, STO-007]: toda coluna `*_at` é `INTEGER` com milissegundos Unix UTC;
  `release_date` é `TEXT` `YYYY-MM-DD`. Helpers no pacote:
  `func TimeToMillis(t time.Time) int64` (`t.UnixMilli()`), `func MillisToTime(ms int64) time.Time`
  (`time.UnixMilli(ms).UTC()`); nulos via `sql.NullInt64`. Nenhuma coluna de instante como `TEXT`/`DATETIME`.
- sqlc (`sqlc.yaml`, engine sqlite, código gerado versionado): `CountGames`, `ListPlatforms`, `InsertPlatform`.
  Colunas `INTEGER` de tempo saem como `int64`/`sql.NullInt64`; a conversão fica nos helpers acima.
- STO-005 é testado com dois `Store` no mesmo processo sobre o mesmo arquivo em `t.TempDir()`.

## `internal/ipc` [IPC-001..017]
Contrato: [contracts/ipc.openrpc.json](contracts/ipc.openrpc.json) — fonte da verdade dos métodos, parâmetros,
resultados e notificações. Os tipos Go em `internal/ipc` devem bater com ele.

### Framing e validação (servidor)
- JSON-RPC 2.0, NDJSON (uma mensagem por linha). `MaxMessageSize = 1 << 20` (1.048.576 bytes, sem o `\n`).
  Leitor com `bufio.Reader` limitado: linha de até `MaxMessageSize` é processada [IPC-012]; acima disso a conexão
  é fechada sem resposta obrigatória, só aquela goroutine de conexão termina [IPC-003].
- Ordem de validação de cada linha:
  1. Não é JSON → `-32700`, `id: null` [IPC-002].
  2. É array → uma resposta `-32600`, `id: null`; conexão segue [IPC-010].
  3. Não é objeto, `jsonrpc` ≠ `"2.0"`, `method` não string, ou `params` presente e não for `null`/array/objeto →
     `-32600` (com o `id` se legível, senão `null`) [IPC-009].
  4. Sem `id` → notificação: processa (ou ignora se o método não existe) e **não responde** [IPC-011].
  5. Método desconhecido → `-32601` [IPC-002].
  6. Método sem parâmetros: `params` ausente, `null`, `[]` ou `{}` → ok [IPC-008]; array/objeto não vazio →
     `-32602` [IPC-002].
- Handlers: `type Handler func(ctx context.Context, params json.RawMessage) (any, error)`; o dispatcher normaliza
  "params vazios" antes de chamar.

### Servidor
- `Server`, codec e dispatcher sobre `net.Listener`/`net.Conn` (testáveis com `net.Pipe()`/`net.Listen("tcp", "127.0.0.1:0")`).
  Só `Listen`/`Dial` do pipe ficam em `*_windows.go` (`go-winio`, SDDL `D:P(A;;GA;;;<SID>)`) [IPC-007].
- Construção com opções funcionais:
  ```go
  type ServerOption func(*serverConfig)
  func WithStartedAt(t time.Time) ServerOption
  func NewServer(clock domain.Clock, handlers map[string]Handler, log *slog.Logger, opts ...ServerOption) *Server
  func (s *Server) StartedAt() time.Time
  ```
  Sem `WithStartedAt`, `NewServer` usa `clock.Now()` no momento da construção. O agente captura o instante uma
  vez e o passa tanto para `WithStartedAt` quanto para o handler de `agent.status`, de modo que `startedAt`/
  `uptimeSeconds` de `agent.status` e o `uptimeSeconds` do heartbeat derivam do **mesmo** instante
  (`uptimeSeconds = floor(clock.Now() − startedAt)` em segundos) [IPC-001, IPC-004].
- `Serve(ctx, net.Listener) error`; `Close() error` aguarda accept + conexões + heartbeat [IPC-006].
- Heartbeat: loop `select { case <-clock.After(5*time.Second): ...; case <-ctx.Done(): }` envia
  `agent.heartbeat` (`uptimeSeconds` calculado a partir de `startedAt`) a todas as conexões [IPC-004]. Testes usam
  `FakeClock.BlockUntilWaiters` + `Advance`.
- Escrita por conexão serializada (mutex por conexão ou goroutine escritora única).

### Cliente
- ```go
  var ErrConnectionLost = errors.New("ipc: connection lost")
  var ErrClientClosed  = errors.New("ipc: client closed")
  type Dialer func(ctx context.Context) (net.Conn, error)
  func NewClient(dial Dialer, clock domain.Clock, log *slog.Logger) *Client
  func (c *Client) Call(ctx context.Context, method string, params, result any) error
  func (c *Client) Notifications() <-chan Notification
  func (c *Client) Close() error
  ```
- `Call` sem conexão espera a próxima conexão até o `ctx` terminar; retorna erro que satisfaz
  `errors.Is(err, ctx.Err())` [IPC-014]. Chamadas pendentes quando a conexão cai recebem `ErrConnectionLost`
  [IPC-015]. Após `Close`, toda `Call` retorna `ErrClientClosed` [IPC-016].
- Reconexão: goroutine dona do ciclo dial → leitura → queda; espera via `clock.After(backoff)`; backoff começa em
  250 ms, dobra a cada falha, teto 5 s, sem jitter, volta a 250 ms após conexão bem-sucedida [IPC-005, IPC-013].
- `Notifications()`: canal com buffer 16; envio não bloqueante — cheio, a notificação é descartada (log em debug)
  e a leitura de respostas não para [IPC-017].
- `Close()` cancela o ctx interno, fecha a conexão e aguarda a goroutine de reconexão/leitura [IPC-006].

## `internal/winapi` [AGT-006]
- `SessionEndWatcher`: cria uma janela top-level **oculta** (classe própria, sem `WS_VISIBLE`; não message-only,
  porque `HWND_MESSAGE` não recebe broadcasts de `WM_QUERYENDSESSION`/`WM_ENDSESSION`) numa goroutine com
  `runtime.LockOSThread`, rodando o loop de mensagens.
  ```go
  func NewSessionEndWatcher(clock domain.Clock, log *slog.Logger) *SessionEndWatcher
  func (w *SessionEndWatcher) Start(ctx context.Context) (<-chan struct{}, error)
  func (w *SessionEndWatcher) ShutdownDone() // idempotente; seguro de chamar de qualquer goroutine
  func (w *SessionEndWatcher) Close() error
  ```
  `Start` sinaliza no canal ao receber `WM_ENDSESSION` com `wParam = TRUE`;
  `Close()` posta `WM_CLOSE`, espera o loop sair, `DestroyWindow` + `UnregisterClass`.
- O handler de `WM_ENDSESSION` só retorna depois que o agente chama `ShutdownDone()` ou após 2 s medidos por
  `domain.Clock` (`select { case <-done: case <-clock.After(2*time.Second): }`), porque o Windows pode encerrar o
  processo assim que a mensagem retorna. `ShutdownDone()` fecha um canal interno (protegido por `sync.Once`);
  chamá-lo sem `WM_ENDSESSION` pendente é no-op [AGT-006].
- `CreateDetached(exe string, args []string, flags uint32) error` (wrapper de `CreateProcess`): fecha `hProcess` e
  `hThread` logo após a criação. Usado pelo agente e pela UI.

## `internal/logging` [AGT-005, DSK-009]
- Pacote compartilhado por agente e UI (sem código específico de Windows):
  ```go
  const (
      MaxSizeMB  = 10
      MaxBackups = 3
  )
  type Options struct {
      Dir   string // Paths.Logs
      File  string // "agent.log" | "desktop.log"
      Level slog.Level
  }
  func New(opts Options) (*slog.Logger, io.Closer, error) // slog JSON + lumberjack
  func ParseLevel(s string) (slog.Level, error)          // debug|info|warn|error; "" → Info; outro → erro
  ```
- O agente usa `ParseLevel` com o valor de `--log-level` (substitui o antigo `parseLogLevel` em `cmd/agent`); a UI
  não tem `--log-level` e chama `New` com `Level: slog.LevelInfo` fixo.
- Escrita síncrona, sem buffer: o handler JSON do `slog` chama `Write` do `lumberjack`, que escreve direto no
  `*os.File`. Uma linha cujo `Write` retornou já está no sistema operacional; `Close` só fecha o arquivo (não há
  flush a fazer). Isso é o que permite fechar o log por último no shutdown (ver `cmd/agent`).
- **Exceção à regra "toda goroutine tem dono e condição de saída"** · alterado em 2026-11-08: registrada a goroutine
  do lumberjack (revisão da T05). O `lumberjack` v2.2.1 inicia uma goroutine `millRun` (no primeiro `Write` ou na
  primeira rotação; uma por `*lumberjack.Logger`) que o `Close` não encerra. Ela vive até o fim do processo e não
  segura handles (só espera num canal interno). Aceita como dependência de terceiros: uma por processo no agente
  (`agent.log`) e uma na UI (`desktop.log`). Testes com `goleak` a ignoram com
  `goleak.IgnoreTopFunction("gopkg.in/natefinch/lumberjack%2ev2.(*Logger).millRun")` (o `.` de `lumberjack.v2`
  aparece escapado como `%2e` no nome da função).
- **`Close` não é terminal** · alterado em 2026-11-08: comportamento do lumberjack (revisão da T05). Um `Write`
  depois de `Close` reabre o arquivo silenciosamente, e esse novo handle não tem quem o feche. Portanto **nada pode
  logar depois que o `io.Closer` do log é chamado**: o fechamento do log é o último passo de shutdown que pode
  registrar mensagens, e todo componente que recebeu o `*slog.Logger` (servidor, cliente IPC, watcher, store) tem
  de estar encerrado antes dele.

## `cmd/agent` [AGT-001..008]
- `-ldflags "-H windowsgui -X main.version=<v>"`; `var version = "dev"` em `main` [AGT-004], repassado ao handler
  de `agent.status`.
- Flags `--data-dir`, `--log-level` (`debug|info|warn|error`, padrão `info`); valor inválido → mensagem em stderr/log
  de bootstrap e `os.Exit(2)` antes de abrir o banco [AGT-005]. Parsing na função pura
  `logging.ParseLevel(string) (slog.Level, error)` do pacote compartilhado `internal/logging` (ver abaixo).
- Mutex nomeado `Local\arcadia-agent` (instância única) [AGT-002].
- Ordem de subida: `logging.ParseLevel` → mutex → paths → logs (`slog` JSON + `lumberjack`, 10 MB × 3, `logs/agent.log`) → store
  (`OpenOptions{Migrate: true}`) → servidor IPC (`WithStartedAt`) → `SessionEndWatcher` → bandeja
  (`fyne.io/systray`: **Abrir Arcadia**, **Sair**) [AGT-001].
- **Ordem de shutdown** (disparado por "Sair", `WM_ENDSESSION` ou Ctrl+C) — não é o inverso exato da subida
  [AGT-003, AGT-006]:
  · alterado em 2026-11-08: fechamento do log movido para o último passo (revisão da T05)
  1. `systray.Quit`
  2. `Server.Close`
  3. `Store.Close`
  4. log `info` "shutdown concluído" — última mensagem com entrega garantida
  5. `watcher.ShutdownDone` — libera o handler de `WM_ENDSESSION`
  6. `watcher.Close`
  7. `mutex.Close`
  8. `Close` do log (lumberjack) — último passo; nada loga depois dele

  O watcher é liberado e fechado **depois** de IPC e store: se o handler de `WM_ENDSESSION` retornasse antes, o
  Windows poderia matar o processo no meio do shutdown (banco sem fechar). O log não precisa estar fechado antes do
  `ShutdownDone`, porque não tem buffer (ver `internal/logging`): o que foi escrito até o passo 4 já está no sistema
  operacional, e o `Close` não faz flush. Por isso o fechamento do log vai para o fim — o watcher recebeu o
  `*slog.Logger` e pode logar em `ShutdownDone`/`Close` (ex.: timeout de 2 s do handler), e um `Write` depois do
  `Close` reabriria o arquivo e vazaria o handle.
  Consequência aceita: mensagens dos passos 5–7 são *best effort* — num logoff, o Windows pode encerrar o processo
  logo após o passo 5, e elas (e o passo 8) não acontecem; o SO fecha o handle do arquivo ao encerrar o processo.
  `agent.Shutdown` passa a cobrir só IPC → store (passos 2–3); o fechamento do log fica com o `main`, após
  `mutex.Close`.
- **Abrir Arcadia** [AGT-007, AGT-008]: função `openUI(exeDir string, stat func(string) error, spawn SpawnFunc, log)`
  procura `exeDir\arcadia.exe`; inexistente → `log.Error` e retorna sem chamar `spawn`; existente →
  `spawn(path, nil, DETACHED_PROCESS|CREATE_NEW_PROCESS_GROUP)`. Dependências injetadas para teste.

## `cmd/desktop` [DSK-001..009]
- `wails.json` em `cmd/desktop/` com `frontend:dir` = `../../frontend`; os assets ficam embutidos via
  `frontend/embed.go` (`//go:embed all:dist`), porque `go:embed` não acessa pastas pai.
- **Pasta de dados** [PTH-001, DSK-004]: a UI aceita a flag `--data-dir` e resolve com o mesmo `paths.Resolve`
  do agente (flag > env `ARCADIA_DATA_DIR` > `<pasta do exe>\data`). O `Root` resultante é o que vai em
  `--data-dir` ao iniciar o agente.
- Log: `slog` JSON + `lumberjack` em `<Root>/logs/desktop.log`, 10 MB × 3, via `internal/logging`, em nível
  `info` fixo (a UI não tem `--log-level` nesta fase) [DSK-009].
- **Ordem de shutdown da UI** (`OnShutdown`) · alterado em 2026-11-08: explicitado que o log fecha por último
  (revisão da T05): `Client.Close` (aguarda a goroutine de reconexão, que loga) → `wg.Wait` (repasse de
  notificações) → `libraryGate.Close` → `Close` do log. O log já é o último passo; nenhum código da UI pode logar
  com o `*slog.Logger` depois dele. O agente não é tocado [DSK-002].
- **Início do agente** [DSK-001, DSK-004, DSK-005]:
  - `agentCommand(exeDir, root string) (exe string, args []string, flags uint32)` pura: `exeDir\arcadia-agent.exe`,
    `["--data-dir", root]`, `DETACHED_PROCESS|CREATE_NEW_PROCESS_GROUP|CREATE_BREAKAWAY_FROM_JOB`.
  - `startAgent(spawn SpawnFunc, ...)`: se `spawn` falhar com `errors.Is(err, windows.ERROR_ACCESS_DENIED)`, loga
    `warn` e tenta uma vez sem `CREATE_BREAKAWAY_FROM_JOB`; outros erros retornam direto. `SpawnFunc` injetável
    (a real é `winapi.CreateDetached`).
  - Se o pipe não existir, inicia o agente e o `Client` (reconexão própria) conecta em até 5 s.
- **dataDir divergente** [DSK-006]: após conectar, chama `agent.status`; se `filepath.Clean` de `dataDir` ≠ `Root`
  (comparação case-insensitive no Windows), `log.Warn` e segue.
- **Banco** (ADR 0010) [DSK-007, DSK-008]: o `Store` é aberto preguiçosamente com `OpenOptions{Migrate: false}`,
  só depois da primeira conexão bem-sucedida ao agente. `LibrarySummary()` sem store aberto tenta abrir; se o erro
  for `ErrSchemaMismatch`, retorna `ErrAgentNotReady` (mensagem "agente ainda não pronto") e não guarda estado de
  falha — a próxima chamada tenta de novo. Acesso ao store protegido por mutex.
- Bindings: `AgentStatus()`, `LibrarySummary()`; repassa `agent.heartbeat` como evento Wails `agent:heartbeat`.

## `frontend/` [UI-001..003]
- Template Wails React-TS + Vite, Tailwind, TanStack Query, Zustand; Vitest + Testing Library.
- Uma tela: cabeçalho com o indicador do agente ("Conectado" verde + versão + uptime / "Desconectado" vermelho) e
  o estado vazio da biblioteca ("Nenhum jogo ainda", "N jogos · M plataformas", sempre no plural, sem tratar
  singular) [UI-001, UI-002].
- `formatUptime(seconds: number): string` pura: `< 60` → `Ns`; `< 3600` → `Mm SSs`; senão `Hh MMm SSs`
  (minutos e segundos com 2 dígitos quando há unidade maior) [UI-003].

## Riscos
- Goroutines do servidor IPC (accept, uma por conexão, heartbeat) e do cliente (reconexão/leitura): todas com
  `context` e `WaitGroup`; `Close()` espera.
- `FakeClock.Advance` não pode bloquear com o mutex segurado (envio em canal com buffer 1, não bloqueante).
- Corrida em testes de heartbeat/backoff: sempre `BlockUntilWaiters` antes de `Advance`, nunca `time.Sleep`.
- Handles: mutex nomeado, pipe, janela oculta/classe do `SessionEndWatcher` e processos criados (agente pela UI,
  UI pelo agente): `hProcess`/`hThread` fechados logo após o spawn.
- `CREATE_BREAKAWAY_FROM_JOB` falha com `ERROR_ACCESS_DENIED` quando a UI roda dentro de um job sem
  `JOB_OBJECT_LIMIT_BREAKAWAY_OK` (ex.: terminais/IDEs); o fallback mantém a UI funcional, mas o agente pode morrer
  junto com esse job — registrado como aviso.
- Abrir sem migrar não pode escrever no banco (nem criar `goose_db_version`), senão a UI interferiria na migração
  do agente (ADR 0010).
