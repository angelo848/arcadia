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
     `-32600` (com o `id` se legível, senão `null`) [IPC-009]. `id` presente que não seja string, número ou `null`
     (objeto, array, bool) → `-32600` com `id: null`.
     · alterado em 2026-11-08: tipo inválido de `id` (lacunas da T06).
  4. Sem `id` → notificação: processa (ou ignora se o método não existe) e **não responde** [IPC-011].
     Só a **ausência do membro** `id` caracteriza notificação; `"id": null` explícito é requisição, é respondida e a
     resposta leva `id: null` [IPC-011]. · alterado em 2026-11-08: distingue `id` ausente de `id: null` (lacunas da T06).
  5. Método desconhecido → `-32601` [IPC-002].
  6. Método sem parâmetros: `params` ausente, `null`, `[]` ou `{}` → ok [IPC-008]; array/objeto não vazio →
     `-32602` [IPC-002].
- Decoder de linhas (· alterado em 2026-11-08: comportamento de borda definido (lacunas da T06)):
  - A última linha sem `\n` antes do EOF é devolvida normalmente (se ≤ `MaxMessageSize`); o `io.EOF` só vem na
    chamada seguinte [IPC-012].
  - Um `\r` final imediatamente antes do `\n` é removido (CRLF tolerado); o limite de `MaxMessageSize` vale para a
    linha já sem `\r`/`\n`. O mesmo vale para a última linha sem `\n`: um `\r` final antes do EOF também é removido.
  - Depois de `ErrMessageTooLarge`, `io.EOF` ou erro de leitura, o decoder fica preso nesse erro (o servidor fecha a
    conexão). · alterado em 2026-11-08: `\r` no EOF e erro preso (revisão da T06).
- Handler que devolve `*Error` nil dentro de um `error` não nulo é tratado como erro genérico (`-32603`).
  · alterado em 2026-11-08 (revisão da T06).
- Notas para a T07 (revisão da T06): `Dispatch` devolve `[]byte` — o servidor deve escrever com
  `Encoder.Encode(json.RawMessage(resp))` (com `[]byte` o `json.Marshal` geraria base64); o heartbeat usa
  `Request{JSONRPC: Version, Method: MethodHeartbeat, Params: ...}` sem `ID`; para logar erros genéricos de handler, o
  `Server` embrulha cada `Handler` num decorador que loga erros que não são `*Error` (a API de `Dispatch` não muda).
  - Linhas vazias (zero bytes, ou só `\r`) são ignoradas: não geram resposta nem erro; o decoder segue para a próxima.
- Handlers: `type Handler func(ctx context.Context, params json.RawMessage) (any, error)`; o dispatcher normaliza
  "params vazios" antes de chamar.
- Erro de handler (· alterado em 2026-11-08: mapeamento definido (lacunas da T06)): se o handler devolve `*Error`,
  ele vai ao cliente como está; qualquer outro `error` vira `-32603` com message `"Internal error"` — o texto do erro
  original **não** vai ao cliente (é logado pelo servidor, T07).
- Textos das `message` (· alterado em 2026-11-08: textos fixados (lacunas da T06)): as respostas de erro geradas pelo
  dispatcher usam exatamente os textos de `components.errors` em [contracts/ipc.openrpc.json](contracts/ipc.openrpc.json):
  `-32700` `"Parse error"`, `-32600` `"Invalid Request"`, `-32601` `"Method not found"`, `-32602` `"Invalid params"`,
  `-32603` `"Internal error"`.

### Servidor
- `Server`, codec e dispatcher sobre `net.Listener`/`net.Conn` (testáveis com `net.Pipe()`/`net.Listen("tcp", "127.0.0.1:0")`).
  Só `Listen`/`Dial` do pipe ficam em `*_windows.go` (`go-winio`, SDDL `D:P(A;;GA;;;<SID>)`) [IPC-007].
  · alterado em 2026-11-08 (revisão da T08): o `Dial` também confere o dono do servidor [IPC-019]; ver
  "Dono do servidor do pipe".
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
- · alterado em 2026-11-08 (lacunas da T07): uma conexão lenta não impede as outras de receber o heartbeat nem
  trava o `Close` (que fecha as conexões e destrava escritas pendentes). Erro de handler que não é `*Error` é logado
  em `Error` com o texto original. Mensagem acima do limite fecha a conexão com log em `Warn`. O relógio do servidor
  só é usado pelo heartbeat (além de `Now`).
- · alterado em 2026-11-08 (revisão da T07): o despacho é **sequencial por conexão** (a leitora chama o handler e só
  então lê a próxima linha); reavaliar quando houver handlers lentos (fase 3+). `Close` antes de `Serve` faz o
  `Serve` posterior retornar `nil` (encerramento pedido, não erro); só um segundo `Serve` retorna erro. Na T08,
  conferir se o `Accept` do go-winio pode devolver erro transitório; se puder, logar e continuar em vez de encerrar.
- · alterado em 2026-11-08 (T08): no go-winio v0.6.4 `ERROR_NO_DATA` e `ERROR_PIPE_CONNECTED` já são absorvidos, mas
  falhas ao criar a próxima instância chegam ao `Accept` com o listener ainda utilizável. Por isso: erro de `Accept`
  que não seja `net.ErrClosed` (nem `Close`/fim do `ctx`) é logado em `Warn` e o accept é tentado de novo após
  `100 ms` via `clock.After` (único outro uso do relógio do servidor, só nesse caminho de erro); `Close`/`ctx` durante
  essa espera encerram normalmente [IPC-006, IPC-007].
  · **substituído por IPC-018** (2026-11-08, revisão da T08): o retry fixo de 100 ms deu lugar ao backoff com
  desistência descrito em "Erro de Accept" abaixo.
- · alterado em 2026-11-08 (revisão da T08) — **Erro de Accept** [IPC-018, IPC-006]:
  - Constantes não exportadas em `server.go`: `acceptBackoffMin = 100 * time.Millisecond`,
    `acceptBackoffMax = 5 * time.Second`, `maxAcceptFailures = 10` (substituem `acceptRetryDelay`).
  - `acceptLoop` mantém `failures int` e `backoff time.Duration` locais (a goroutine de accept é a única dona; sem
    mutex). Em erro de `Accept`:
    1. `Close` pedido ou `ctx` encerrado → sai sem erro (o `Serve` decide `nil`/`ctx.Err()`, como hoje).
    2. `errors.Is(err, net.ErrClosed)` sem `Close`/`ctx` → retorna `fmt.Errorf("ipc: accept: %w", err)` (inalterado).
    3. Senão `failures++`. Se `failures == maxAcceptFailures` → log `Error` (`"ipc: accept failed repeatedly, giving up"`,
       `error`, `failures`) e retorna `fmt.Errorf("ipc: accept failed %d times in a row: %w", failures, err)` sem esperar.
    4. Caso contrário log `Warn` (`"ipc: accept failed, retrying"`, `error` = `err.Error()`, `failures`, `retry_in`) e
       `select` em `s.closed`/`ctx.Done()`/`s.clock.After(backoff)`; depois `backoff = min(2*backoff, acceptBackoffMax)`.
  - `Accept` bem-sucedido → `failures = 0`, `backoff = acceptBackoffMin`.
  - Sequência resultante (9 esperas antes da 10ª falha): 100, 200, 400, 800, 1600, 3200, 5000, 5000, 5000 ms.
  - Ao desistir, `Serve` segue o caminho de erro já existente: fecha conexões, para o heartbeat, aguarda as goroutines
    e devolve o erro (o `errors.Is` com o último erro de Accept vale pela cadeia `%w`). O servidor não é reutilizável
    depois disso (vale a regra "Server serve uma vez").
  - O relógio do servidor passa a ter três usos: `Now`, heartbeat e esta espera (só no caminho de erro).
  - Testes: listener fake com fila programável de resultados (`err` ou `net.Conn`) e `Accept` bloqueante quando a
    fila esvazia; `FakeClock.BlockUntilWaiters` antes de cada `Advance`. Os waiters do heartbeat (5 s) e do backoff
    coexistem: o teste conta 2 waiters (ou usa `WithStartedAt` e só avança o necessário) e confere "nada dispara 1 ns
    antes" avançando `backoff − 1ns` e verificando que nenhum novo `Accept` ocorreu. Log capturado num
    `slog.Handler` de teste para conferir nível e texto.
- · alterado em 2026-11-08 (T08): a primeira instância do go-winio é criada sem aceitar clientes; só há instância
  conectável enquanto existe um `Accept` pendente. Um `ListenPipe` sem `Serve` recusa clientes com `ERROR_PIPE_BUSY`
  (testes que inspecionam o pipe mantêm um `Accept` pendente). A exclusividade do nome vem de `FILE_CREATE` na
  primeira instância (equivalente a `FILE_FLAG_FIRST_PIPE_INSTANCE`).

### Dono do servidor do pipe (anti-squatting) [IPC-019, IPC-007]
· alterado em 2026-11-08 (revisão da T08): seção nova, decidida pelo usuário. Complementa o ADR 0003 (pipe por
usuário com ACL só do usuário atual): a DACL protege o pipe que **nós** criamos, mas não impede que outro usuário local
crie antes o mesmo nome `\\.\pipe\arcadia-agent-<SID da vítima>` com uma DACL que nos permita conectar. Sem ADR novo:
é uma verificação adicional dentro da decisão do ADR 0003, não uma mudança dela.

- API pública nova (`pipe_windows.go`; o sentinela pode ficar em `pipe.go` para ser portável):
  ```go
  var ErrUntrustedServer = errors.New("ipc: untrusted pipe server")
  ```
- `DialPipe(ctx, name)`, depois de `winio.DialPipeContext` ter sucesso:
  1. Obtém o handle da conexão via `conn.(interface{ Fd() uintptr })` (o `win32Pipe` do go-winio v0.6.4 expõe `Fd`);
     o handle continua sendo da `conn` — **não** é fechado aqui. Sem `Fd` → não verificável.
  2. `windows.GetNamedPipeServerProcessId(h, &pid)`.
  3. `windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)` → `defer windows.CloseHandle(hp)`.
  4. `windows.OpenProcessToken(hp, windows.TOKEN_QUERY, &tok)` → `defer tok.Close()`.
  5. `tok.GetTokenUser()` → `User.Sid.String()`; compara com `CurrentUserSID()` (comparação de string SID exata).
  - Qualquer falha nos passos 1–5 ou SID diferente → `conn.Close()` e retorna
    `fmt.Errorf("ipc: dial pipe %s: %w: %v", name, ErrUntrustedServer, cause)` (o motivo vai no texto; `errors.Is` com
    `ErrUntrustedServer` vale; a causa não precisa ser alcançável por `errors.Is`). Nenhum handle fica aberto.
- Ponto de injeção para teste (não exportado, em `pipe_windows.go`):
  ```go
  // pipeServerSID returns the string SID of the user owning the server process of the pipe handle h.
  var pipeServerSID = realPipeServerSID // func(h windows.Handle) (string, error): steps 2–5
  ```
  `DialPipe` chama `pipeServerSID(h)` e compara com `CurrentUserSID()`. Testes do pacote (`pipe_windows_test.go`,
  **sem `t.Parallel`**, restaurando com `t.Cleanup`) substituem a variável por uma função que devolve um SID diferente
  (ex.: `"S-1-5-18"`) ou um erro, e conferem: `errors.Is(err, ErrUntrustedServer)`, `conn == nil`, e que a conexão
  aceita pelo lado servidor (listener real com `Accept` pendente) lê EOF/erro em prazo curto. Não é preciso outro
  usuário real. Um teste sem substituição confere o caminho real: `realPipeServerSID` sobre uma conexão a um pipe
  do próprio processo devolve `CurrentUserSID()` e o `ping` via `Server` funciona.
- Cliente: nenhuma mudança em `client.go` — `PipeDialer` devolve o erro, que o `Client` trata como falha de dial
  (log `Debug` existente + backoff IPC-013). Nada é escrito na conexão antes da verificação.
  · alterado em 2026-11-08: exceção — quando `errors.Is(err, ErrUntrustedServer)`, o `Client` loga em `Warn`
  (sinal de possível ataque) em vez de `Debug`; única mudança em `client.go`.
- Agente (T10): `ListenPipe` mantém a cadeia `%w` até o `*os.PathError` do go-winio (`Op: "open"`) e o
  `windows.Errno`. Com `FILE_CREATE` num nome existente o esperado é `ERROR_ALREADY_EXISTS` (de
  `STATUS_OBJECT_NAME_COLLISION`); um pipe de outro usuário com DACL restritiva pode render `ERROR_ACCESS_DENIED`.
  O teste da T08 fixa o código observado; o agente trata ambos como "nome já existe" e loga em `Error`
  `"pipe já existe: outra instância ou outro processo"` com `pipe` e `error`.
  · alterado em 2026-11-08 (testes da T08): o código observado (Windows 11 26200, go-winio v0.6.4) é
  `ERROR_ACCESS_DENIED` (5), mesmo com o pipe do próprio usuário. Como o agente só chama `ListenPipe` depois de obter
  o mutex de instância única (T09), uma falha aqui com 5 ou 183 significa outro processo segurando o nome.
- Riscos: (a) corrida de PID — o processo servidor pode sair e o PID ser reutilizado entre os passos 2 e 3; o pior
  caso é recusar (falha segura) ou aceitar um processo do mesmo usuário, aceitável. (b) agente do mesmo usuário
  rodando elevado e UI não elevada: `OpenProcessToken` pode ser negado → recusado (falha segura); a fase 2 não prevê
  agente elevado (questão em aberto se isso mudar).
  (c) Serviço/SYSTEM servindo o nome → SID diferente → recusado.

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
- · alterado em 2026-11-08 (lacunas da T07): a primeira tentativa de dial é imediata no `NewClient`; o backoff só
  começa após uma falha ou queda. O relógio do cliente só é usado pelo backoff. Resposta de erro JSON-RPC faz `Call`
  retornar o `*Error` recebido (`errors.As` funciona). `Close` é idempotente, cancela o `ctx` passado ao `Dialer`,
  fecha `Notifications()` e só retorna depois que a goroutine do cliente saiu.
- · alterado em 2026-11-08 (revisão da T07):
  - `Call` observa o `ctx` também durante o **envio** da requisição: se o servidor não lê, `Call` retorna
    `errors.Is(err, ctx.Err())` no prazo, sem derrubar a conexão nem as outras chamadas (goroutine escritora por
    conexão, dona da escrita; uma requisição cujo chamador desistiu antes de ser escrita não é enviada) [IPC-014].
  - Após uma queda, o cliente espera `250 ms` antes do novo dial (não redial imediato) [IPC-013].
  - Mensagens do servidor fora do esperado não derrubam a conexão: linha não JSON → ignorada com `Warn`; requisição
    do servidor com `id` → ignorada; resposta com `id` desconhecido, não numérico ou `null` → ignorada com `Warn`.
    Linha acima de `MaxMessageSize` derruba a conexão (chamadas pendentes recebem `ErrConnectionLost`).
  - `Dialer` que devolve `(nil, nil)` é tratado como falha de dial (entra no backoff).
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
