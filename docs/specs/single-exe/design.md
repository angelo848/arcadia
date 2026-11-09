# Executável único (`arcadia.exe`, dois processos) — design

Implementa [requirements.md](requirements.md). Base: design da [fase 2](../phase-2-skeleton/design.md) (tudo o que não
é citado aqui continua como lá), ADRs 0003 (substituído em parte pelo **ADR novo de executável único**, a escrever
pela sessão principal), 0006, 0010, 0011, 0013. Contrato IPC: [contracts/ipc.openrpc.json](contracts/ipc.openrpc.json)
(v0.3.0, substitui a v0.2.0 da fase 2 como fonte da verdade). Os IDs entre colchetes indicam os requisitos atendidos.

## Layout de pacotes [EXE-001, EXE-005, EXE-006]

```
cmd/arcadia/            # ÚNICO main: escolhe o modo; contém o App Wails (package main, ver abaixo)
  main_windows.go       # run(args): agente → agent.Run; UI → runUI (wails.Run)
  main_other.go         # !windows: sai com 1 (como hoje)
  mode.go               # isAgentMode(args []string) bool — puro [EXE-001]
  app.go probe.go library.go datadir.go supervisor.go agentproc_windows.go …  # vindos de cmd/desktop
  wails.json  build/{appicon.png, windows/{icon.ico, info.json, wails.exe.manifest}}
internal/agent/         # NOVO: o que era cmd/agent (startAgent, Shutdown, trayLoop, bandeja, openUI, autostart)
  run_windows.go        # func Run(opts Options) int — o antigo run() de cmd/agent/main_windows.go
  run_other.go          # !windows: Run retorna 1
  app.go tray_loop.go tray_windows.go openui.go autostart.go assets/icon.ico
internal/winapi/        # + autostart_windows.go (registro), + AllowSetForegroundWindow em user32_windows.go
internal/ipc/           # + Server.Drain, + MethodShutdown/ShutdownParams, + Client.Connected/StateChanged
internal/store/         # + migrations/00002_app_setting.sql, queries/settings.sql, Setting/SetSetting
```

- `cmd/desktop` vira `cmd/arcadia` (`git mv`), com `wails.json` e `build/`. O App **continua em `package main`**: o
  Wails gera os bindings por pacote (`window.go.main.App`), e o frontend (`frontend/src/lib/agent.ts`) depende desse
  caminho — mover o App mudaria o binding sem ganho.
- `cmd/agent` é removido; seu código vai para `internal/agent` (package `agent`). Os tipos internos (`agent`,
  `config`) podem ser renomeados pelo planner para não colidir com o nome do pacote (ex.: `instance`).
- `wails.json` em `cmd/arcadia`: `outputfilename: "arcadia"`, `frontend:dir: "../../frontend"`,
  `wailsjsdir: "../../frontend"` (inalterados). `wails build` gera `cmd/arcadia/build/bin/arcadia.exe` [EXE-005].
  `.gitignore` já cobre `build/bin/` em qualquer nível.
- Versão: `var version = "dev"` em `cmd/arcadia` (`-X main.version=<v>`, repassado por
  `wails build -ldflags "-X main.version=<v>"`); `agent.Run` recebe `Options.Version` [AGT-004 inalterado].
- BLD-001 / EXE-006: `go build ./...` continua sem Node (`frontend/dist/.gitkeep` + `//go:embed all:dist`). Sem as
  build tags do Wails (`desktop,production` ou `dev`), `wails.Run` falha em tempo de execução — só o modo UI desse
  binário é afetado; o modo agente não chama nada do Wails. Desenvolvimento do agente:
  `go run ./cmd/arcadia --agent --data-dir ./.devdata` (substitui `go run ./cmd/agent` no CLAUDE.md — sessão principal).
- `-H windowsgui`: o `wails build` já o aplica; o agente (que já era `windowsgui`) herda. O manifest do Wails
  (DPI aware, common controls v6) passa a valer também para o processo do agente — sem efeito funcional esperado.
- Nenhum caminho de código monta o nome `arcadia.exe` à mão: todo comando usa `os.Executable()` (em `wails dev` o exe
  se chama `arcadia-dev.exe` em `build/bin`) [EXE-007, EXE-009, AST-002].

## Seleção de modo [EXE-001, EXE-003, EXE-004]

```go
// cmd/arcadia/mode.go
// isAgentMode reports whether args (os.Args[1:]) select agent mode: some arg is exactly "--agent" or "-agent".
func isAgentMode(args []string) bool
```

- `main`: `os.Exit(run(os.Args[1:]))`; `run` decide **antes de qualquer outra coisa** (nenhum mutex, log, `paths`
  ou Wails antes da escolha) [EXE-004].
- Modo agente → `agent.Run(agent.Options{Args: args, Version: version})`. O `FlagSet` do agente define `agent`
  (bool, só para ser aceito), `data-dir` e `log-level`, com `flag.ContinueOnError` + saída 2 em erro de parse
  [EXE-003, AGT-005]. A ordem de subida segue a fase 2 (ParseLevel → mutex → exe → SID → `startAgent` …) com o passo
  novo de autostart (abaixo).
- Modo UI → `runUI(args)`: o código atual de `cmd/desktop/main.go` (FlagSet tolerante a flags do Wails;
  `--data-dir`), mais `SingleInstanceLock` e `AllowSetForegroundWindow` (abaixo). O modo UI nunca chama
  `winapi.AcquireSingleInstance(AgentMutexName)` nem `ipc.ListenPipe` [EXE-004].
- `--agent=false` não é reconhecido por `isAgentMode` (→ UI); no `FlagSet` do agente ele nunca chega lá.

## Modo UI

### Início do agente [EXE-007, EXE-008, DSK-005]
- `agentCommand(exe, root string) (string, []string, uint32)`: `exe` = `os.Executable()` (resolvido no `main` e
  guardado em `desktopConfig.Exe`, ao lado de `ExeDir`), `args = ["--agent", "--data-dir", root]`, flags iguais às de
  DSK-004. `startAgentProcess` (fallback de DSK-005) não muda; a constante `agentExeName` some.
- `ensureAgent`/`probePinger` não mudam [EXE-008].

### Instância única da UI [EXE-011, EXE-012]
- Decisão: usar o `options.App.SingleInstanceLock` do Wails v2 (`UniqueId` fixo `"arcadia-ui-5b0f7c1e"`,
  `OnSecondInstanceLaunch`). Justificativa: já existe no framework, roda **dentro** de `wails.Run` antes do
  `OnStartup` — logo a segunda instância sai sem abrir log nem sondar/iniciar o agente — e entrega o aviso à primeira.
  Alternativa descartada: mutex próprio + `FindWindow`/`WM_COPYDATA`, que duplicaria o que o Wails já faz.
- Antes de `wails.Run`, o modo UI chama `winapi.AllowSetForegroundWindow(ASFW_ANY)` (best effort, erro ignorado com log
  `Debug` no bootstrap): o processo novo foi iniciado pelo usuário (ou pelo agente logo após o clique no menu) e tem
  direito de foco; isso permite que a instância existente se traga à frente.
- `OnSecondInstanceLaunch(data)` → `App.onSecondInstance()`: `wailsruntime.WindowUnminimise`, `WindowShow`, log `Info`
  "segunda instância da UI; trazendo a janela à frente" e `supervisor.Kick()` [EXE-012]. Os `Args` da segunda
  instância são ignorados (um `--data-dir` diferente não troca a biblioteca da janela aberta).

### Supervisor do agente [EXE-017, EXE-019..EXE-025, DSK-003]
Substitui o laço `connectAndForward` + `lateConnect`: uma goroutine dona (`App.wg`), com `ctx` cancelado no
`OnShutdown`, roda o supervisor. Ele recebe tudo injetado (`domain.Clock`, `ensure func(ctx) (ipc.AgentStatus, error)`,
`ipcClient`, `quit func()`, `log`) para ser testado com `FakeClock`.

```go
const (
    relaunchMaxFailures = 3
    stableConnection    = 60 * time.Second
)
var relaunchWaits = [relaunchMaxFailures]time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second}

type supervisor struct { /* clock, ensure, client, quit, gate, root, log; failures int; state */ }
func (s *supervisor) run(ctx context.Context)  // dono do laço; sai com ctx ou com o fechamento de Notifications()
func (s *supervisor) Kick()                     // não bloqueia; canal com buffer 1 [EXE-012]
```

Sequência de subida: `ensure` (= EXE-008) → sucesso: `gate.MarkConnected`, `warnIfDataDirDiffers` (DSK-006), estado
`Connected`; falha: `failures = 1` [EXE-024]. O `ipc.Client` é criado em seguida nos dois casos (como hoje). O primeiro
`agent.heartbeat` ou `Connected()` verdadeiro conta como "primeira conexão" do DSK-007 se o `ensure` falhou.

Laço principal: `select` em `client.Notifications()`, `client.StateChanged()`, temporizador do estado, `kick` e
`ctx.Done()`. **Antes de tratar uma queda**, o supervisor drena sem bloquear `Notifications()` — o cliente entrega a
notificação antes de sinalizar a queda (EXE-019), então um `agent.shutdown` que chegou antes do EOF é visto antes de
decidir relançar.

Tabela de transições (estado × evento → estado + efeito):

| Estado | Evento | Próximo | Efeito |
|---|---|---|---|
| Starting | `ensure` ok | Connected | `upSince = now`; gate; DSK-006 |
| Starting | `ensure` falha | Waiting | `failures = 1`; log `Error` "falha ao relançar o agente" (`attempt` 1); espera `relaunchWaits[1]` |
| Connected | heartbeat | Connected | repassa `agent:heartbeat` à UI |
| Connected | `agent.shutdown` | Quitting | log `Info` "agente encerrado; fechando a UI" (`reason`); `quit()` uma vez [EXE-017] |
| Connected | queda (sem shutdown) | Waiting / GaveUp | se `now − upSince < 60 s`: `failures++`, log `Warn` "agente caiu logo após subir" [EXE-023]; senão `failures = 0`. Log `Warn` "agente desconectado sem aviso de encerramento" (`retry_in`). `failures ≥ 3` → GaveUp; senão espera `relaunchWaits[failures]` [EXE-020, EXE-022] |
| Waiting | conexão volta | Connected | log `Info` "agente reconectado"; `upSince = now` [EXE-021] |
| Waiting | temporizador | Attempting | log `Info` "relançando o agente" (`attempt` = failures+1, `max` 3); roda `ensure` |
| Attempting | `ensure` ok | Connected | `upSince = now`; log `Info` "agente reconectado" (se a sonda respondeu sem `start`, nenhum agente foi criado) [EXE-021] |
| Attempting | `ensure` falha | Waiting / GaveUp | `failures++`; log `Error` "falha ao relançar o agente"; `failures ≥ 3` → GaveUp + log `Error` "desistindo de relançar o agente"; senão espera `relaunchWaits[failures]` [EXE-022] |
| GaveUp | conexão volta | Connected | log `Info` "agente reconectado"; `upSince = now` (failures fica; zera após 60 s estáveis pela regra da queda) |
| GaveUp | `Kick` | Attempting | `failures = 0`; tentativa imediata [EXE-012] |
| outro ≠ GaveUp | `Kick` | (mesmo) | só o log de EXE-012 (feito por `onSecondInstance`) |
| Quitting | qualquer | Quitting | ignora quedas e temporizadores; nunca chama `ensure` [EXE-017] |
| qualquer | `ctx.Done()` (`OnShutdown`) | — | sai do laço; `ensure` em andamento recebe o `ctx` cancelado e não chama `start` depois disso [EXE-025] |

- `ensure` roda **na própria goroutine do supervisor** (bloqueante, já limitado a 5 s e ao `ctx`): não há goroutine
  extra para relançar. Enquanto ele roda, notificações acumulam no buffer de 16 do cliente (IPC-017); um
  `agent.shutdown` que chegue nesse intervalo é visto ao voltar ao laço.
- `quit()` real: `wailsruntime.Quit(wailsCtx)` (assíncrono). O `OnShutdown` que ele dispara fecha o `Client`; a queda
  da conexão é o que libera o drain do agente (EXE-014).
- "Conectado" na tela continua vindo de `AgentStatus()` (UI-001); o supervisor não muda o frontend.
- O `OnShutdown` passa a ser: `cancel` → `Client.Close` → `wg.Wait` (supervisor) → `libraryGate.Close` → log
  "UI encerrada" → `Close` do log (ADR 0013).

## Modo agente

### Shutdown com `agent.shutdown` [EXE-013..EXE-016, EXE-018, EXE-026, AGT-003, AGT-006]
- `trayLoop` passa a **devolver o motivo**: `func trayLoop(...) ShutdownReason` com
  `ShutdownReason` ∈ {`ReasonQuit`, `ReasonSessionEnd`, `ReasonSignal`, `ReasonOther`} (strings do contrato). "Sair" →
  `quit`; canal do watcher → `session-end`; `ctx.Done()` (Ctrl+C/SIGTERM) → `signal`; `stop` (loop do systray terminou
  sozinho) → `other`; `ipcFailed` (abaixo) → `other` [EXE-016, EXE-026]. Os caminhos antes do `onReady` devolvem o
  mesmo mapeamento (`ipcFailed` antes do `onReady` usa `quitBeforeReady`, como `ctx.Done()`).

### Agente sem IPC [EXE-026, EXE-027]
- A goroutine do `Serve` (dona: `wg` do agente, como hoje) classifica o retorno: `nil` (por `Close`) ou
  `errors.Is(err, context.Canceled)` (fim do `ctx`) → só sai; qualquer outro erro → log `Error` "IPC indisponível;
  encerrando o agente" (`error`) e `close(ipcFailed)` (canal `chan struct{}` criado pelo agente, fechado só por essa
  goroutine, no máximo uma vez). Exposto como `func (a *instance) IPCFailed() <-chan struct{}` e passado ao `trayLoop`
  ao lado de `sessionEnd`; o log antigo "ipc: serve ended" é substituído por esse.
- `trayLoop` recebe `ipcFailed <-chan struct{}`: depois do `onReady` chama `quit()` (o `systray.Quit` remove o ícone) e
  devolve `ReasonOther`; antes, `trayQuitEarly(..., "IPC indisponível", ...)` e devolve `ReasonOther`.
- `Run` guarda se o motivo veio de `ipcFailed` (consulta não bloqueante ao canal depois do `trayLoop`) e, nesse caso,
  devolve código **1** ao final, mesmo com `Shutdown` sem erro. A ordem do shutdown não muda.
- Drain nesse caso [EXE-027]: quando `Serve` retorna (por qualquer motivo) ele já fechou todas as conexões e o listener
  (`s.shutdown()` + `wg.Wait()`), então não há a quem notificar. `Drain` verifica, sob `s.mu`, se o `Serve` já terminou
  (`serveDone` fechado); nesse caso devolve `nil` na hora, sem marcar `draining`, sem enfileirar nada e sem esperar o
  `ctx` (com o `Serve` ativo e zero conexões, a regra geral já devolve `nil` sem esperar). O shutdown segue com `Server.Close` (idempotente após o fim do `Serve`) → `Store.Close`.
  Não se tenta reabrir o pipe nem continuar na bandeja sem IPC: sem pipe a UI não teria como falar com o agente, e
  um agente "mudo" segurando o mutex impediria o relançamento.
- Na UI nada muda: a conexão caiu sem `agent.shutdown` (o `Serve` fechou as conexões antes do `Drain`), então o
  supervisor segue a linha "Connected × queda (sem shutdown)" da tabela [EXE-020..EXE-023]. A primeira tentativa pode
  esbarrar no mutex do agente que ainda está encerrando (o novo sai com 0 por AGT-002, a tentativa falha em 5 s e conta
  para EXE-022); a seguinte sobe normalmente.
- Testes: `trayLoop` com `ipcFailed` fechado antes e depois do `ready` → `ReasonOther` e a função de quit certa;
  `Drain` depois de `Serve` ter retornado com erro de `Accept` (listener falso) → `nil` sem esperar um `ctx` sem prazo;
  a classificação do erro do `Serve` é uma função pura testável (`serveFailed(err error) bool`).
- `Shutdown(ctx, reason)` do agente:
  1. `server.Drain(drainCtx, ipc.MethodShutdown, ipc.ShutdownParams{Reason: reason})`, com
     `drainCtx = WithTimeout(ctx, 500 ms)`; erro → `Warn` "clientes não fecharam a conexão a tempo" (`pending`,
     `error`) e segue [EXE-014].
  2. `server.Close` → `wg.Wait` → `store.Close` (como hoje).
- Ordem completa: `systray.Quit` → **Drain (≤ 500 ms)** → `Server.Close` → `Store.Close` → log "shutdown concluído" →
  `watcher.ShutdownDone` → `watcher.Close` → `mutex.Close` → `Close` do log. O orçamento de 2 s do AGT-003 cobre tudo;
  o drain consome no máximo 500 ms. No `WM_ENDSESSION` o handler espera até 2 s (fase 2), o que comporta o drain.

### `ipc.Server.Drain` [EXE-013, EXE-014, EXE-015, EXE-027]
```go
const MethodShutdown = "agent.shutdown"
type ShutdownParams struct { Reason string `json:"reason"` }
// Drain sends the notification method/params to every open connection (and to any connection accepted
// while draining, as its first message), then waits until all of them were closed by the peer or ctx is done.
// It does not stop accepting and does not close connections; call Close afterwards. It returns nil when no
// connection is left (immediately, sending nothing, if Serve has already returned), or an error wrapping
// ctx.Err() reporting how many are still open.
func (s *Server) Drain(ctx context.Context, method string, params any) error
```
- Implementação: marca `draining` + guarda a mensagem (sob `s.mu`); enfileira a notificação em `c.out` de cada conexão
  (espera por espaço no máximo até `ctx`, como as respostas — não é descartada como o heartbeat); `startConn` com
  `draining` enfileira a mensagem antes de iniciar o leitor. A espera usa um canal sinalizado quando `len(s.conns)`
  chega a 0 (o `readLoop` já remove a conexão ao terminar; o EOF do cliente encerra o `readLoop`).
- Por que esperar o cliente fechar em vez de só escrever e fechar: ao fechar o handle do servidor do pipe, dados ainda
  não lidos pelo cliente podem ser descartados; esperar o EOF do cliente garante que a notificação foi lida quando a UI
  está responsiva. A UI fecha a conexão como consequência do `quit()` (EXE-017).
- O heartbeat continua durante o drain (inofensivo). Testes: `net.Pipe`/TCP local + `FakeClock` não é necessário para o
  limite (é `ctx`), mas o teste de EXE-014 usa `context.WithTimeout` curto ou um `ctx` cancelado manualmente.
- Dispatcher/servidor não precisam de mudança para notificações do cliente; `agent.shutdown` é só servidor → cliente.

### `ipc.Client`: estado da conexão [EXE-019]
```go
// Connected reports whether the client currently has a live connection.
func (c *Client) Connected() bool
// StateChanged returns a channel (buffer 1) signalled, without blocking, after every connect and every drop.
// Signals coalesce; read Connected() after receiving. Closed by Close.
func (c *Client) StateChanged() <-chan struct{}
```
- Sinal de "subiu" em `serve` depois de publicar `c.conn`; sinal de "caiu" depois de `readLoop` retornar e `c.conn`
  virar `nil`. Como `readLoop` entrega as notificações (envio não bloqueante) antes de retornar, toda notificação lida
  antes do EOF está no canal antes do sinal de queda.
- `StateChanged()` é fechado junto com `Notifications()` pela goroutine `run` (única remetente).

### "Abrir Arcadia" [EXE-009, EXE-010]
- `openUI(exe, root string, spawn spawnFunc, log)`: `spawn(exe, ["--data-dir", root], DETACHED_PROCESS |
  CREATE_NEW_PROCESS_GROUP)`; erro → `Error` "falha ao abrir o Arcadia" (`path`, `error`); sucesso → `Info`. O `stat`
  injetado some (o exe é o próprio processo). `root` = `Paths.Root` do agente, para a UI abrir a mesma biblioteca mesmo
  quando o agente subiu com `--data-dir` (ex.: pela entrada Run).
- Com a UI já aberta, o processo novo cai na instância única (EXE-011) e sai com 0.

### Início com o Windows [AST-002..AST-011]
- Subida: depois de `startAgent` (store aberto e migrado) e antes da bandeja, `autostart.Reconcile(ctx)` decide o
  estado inicial do item [AST-003..AST-005, AST-010]. Falhas não interrompem a subida [AST-009].
- Código portável em `internal/agent/autostart.go`:
  ```go
  const settingAutostart = "autostart" // "true" | "false"
  type runEntry interface {           // real: *winapi.RunValue
      Get() (value string, ok bool, err error)
      Set(value string) error
      Delete() error                  // absent is not an error
  }
  type settingStore interface {      // real: *store.Store
      Setting(ctx context.Context, key string) (string, error)
      SetSetting(ctx context.Context, key, value string, at time.Time) error
  }
  type autostart struct {
      enabled bool                   // false and inert when dev
      dev     bool                   // version == "dev" [AST-010]
      command string                 // winapi.AutostartCommand(exe, root, exeDir)
      exe     string
      exeOf   func(cmdline string) (string, error) // winapi.CommandExe
      run     runEntry; settings settingStore; clock domain.Clock; log *slog.Logger
  }
  func (a *autostart) Reconcile(ctx context.Context) (checked bool)
  func (a *autostart) Toggle(ctx context.Context) (checked bool)
  ```
  - `Reconcile`: chave ausente → grava `"true"` e trata como ligado [AST-003]; ligado → `Get`; ausente ou ≠ `command`
    → `Set` [AST-004]; desligado → `Get`; se `exeOf(valor)` igual a `exe` (`strings.EqualFold` após
    `filepath.Clean`) → `Delete` [AST-005]. Valor ilegível por `exeOf` em modo desligado → não remove (log `Warn`).
  - `Toggle`: grava a preferência nova **primeiro**; falha → `Error` e devolve o estado antigo (registro intocado);
    sucesso → aplica no registro (ligar: `Set(command)`; desligar: regra de AST-005); falha de registro → `Error` e
    devolve o estado novo [AST-007..AST-009].
  - Log de erro: `Error` "início com o Windows: falha" com `op` ∈ {`read-setting`, `write-setting`, `read-run`,
    `write-run`, `delete-run`} e `error` [AST-009].
- Windows em `internal/winapi/autostart_windows.go` (`golang.org/x/sys/windows/registry`):
  ```go
  const RunKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`
  const AutostartValueName = "Arcadia"
  type RunValue struct { keyPath, name string }       // sem handle guardado
  func NewRunValue(keyPath, name string) *RunValue    // HKCU\keyPath, valor name (REG_SZ)
  func (r *RunValue) Get() (string, bool, error)
  func (r *RunValue) Set(value string) error           // CreateKey + SetStringValue
  func (r *RunValue) Delete() error                    // ERROR_FILE_NOT_FOUND → nil
  func AutostartCommand(exe, root, exeDir string) string // AST-002; windows.EscapeArg + quoteExe
  func CommandExe(cmdline string) (string, error)        // windows.DecomposeCommandLine, 1º token
  ```
  Cada operação abre a chave, opera e fecha (`defer k.Close()` logo após o erro checado) — nenhum handle de registro
  sobrevive à chamada. Testes de `RunValue` usam uma subchave descartável `Software\ArcadiaTest-<aleatório>` em HKCU
  (removida no `t.Cleanup`), nunca a `Run` real.
- Bandeja: `systray.AddMenuItemCheckbox("Iniciar com o Windows", "Inicia o agente do Arcadia no logon", checked)`
  entre "Abrir Arcadia" e o separador. `trayItems` ganha `autostart <-chan struct{}`; o `trayLoop` recebe
  `onToggle func() bool` e `setChecked func(bool)` (o lado Windows chama `Check`/`Uncheck`). Em dev, o item é criado
  desabilitado (`Disable()`) e desmarcado [AST-006, AST-010]. O clique roda síncrono no laço da bandeja (banco +
  registro, rápido), como "Abrir Arcadia".
- No logon, o Windows executa o comando gravado: `arcadia.exe --agent [...]` → só o agente [AST-011].

### Preferências no SQLite [AST-001]
- Decisão: tabela de preferências no `library.db`, dono = agente (que já migra o schema — ADR 0010). Motivo: é o único
  armazenamento persistente que existe; vai com a biblioteca portátil (ADR 0006); a fase 4 (atalhos de pausa) vai
  precisar de configuração no mesmo lugar. Um arquivo novo na pasta de dados exigiria formato, escrita atômica e
  concorrência próprios.
- `internal/store/migrations/00002_app_setting.sql`:
  ```sql
  -- +goose Up
  CREATE TABLE app_setting (
      key        TEXT    PRIMARY KEY,
      value      TEXT    NOT NULL,
      updated_at INTEGER NOT NULL   -- Unix ms UTC (ADR 0009)
  );
  -- +goose Down
  DROP TABLE app_setting;
  ```
  `store.SchemaVersion = 2`. Queries sqlc em `queries/settings.sql`: `GetSetting :one`,
  `UpsertSetting :exec` (`INSERT … ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at =
  excluded.updated_at`).
- API:
  ```go
  var ErrSettingNotFound = errors.New("store: setting not found")
  func (s *Store) Setting(ctx context.Context, key string) (string, error)                 // sql.ErrNoRows → ErrSettingNotFound
  func (s *Store) SetSetting(ctx context.Context, key, value string, at time.Time) error   // at via TimeToMillis
  ```
- A UI é o mesmo binário (mesma `SchemaVersion`), então ADR 0010/STO-009 seguem sem descompasso entre UI e agente do
  mesmo exe. Um agente antigo (v1) rodando com UI nova: a UI recebe `ErrSchemaMismatch` → "agente ainda não pronto"
  (DSK-008), como já previsto.

## Contrato IPC [EXE-013]
[contracts/ipc.openrpc.json](contracts/ipc.openrpc.json) v0.3.0 = v0.2.0 + notificação `agent.shutdown`
(`reason` ∈ `quit | session-end | signal | other`) + `x-limits.shutdownDrainMs = 500`. `internal/ipc/contract_test.go`
passa a ler o arquivo desta pasta (tarefa do test-writer) e o comentário de `protocol.go` passa a apontar para ele. O
arquivo da fase 2 fica como histórico.

## Riscos
- **Wails no processo do agente:** o binário único linka o Wails; o modo agente não chama `wails.Run`, mas
  `init()` de pacotes do Wails/go-webview2 rodam. Verificar (EXE-002) que nenhuma janela nem `msedgewebview2.exe` surge
  e anotar o working set do agente antes/depois (meta: não passar de ~2× o da fase 2). Se algum `init` criar recurso
  visível, isolar o import do Wails atrás do modo UI não é possível em Go (imports são estáticos) — exigiria ADR.
- **`fyne.io/systray` no processo da UI:** o `init()` do systray trava a goroutine principal na thread do SO, o que
  também é o que o Wails precisa; o modo UI não chama `systray.Run`. Conferir na verificação manual que a UI abre normal.
- **Notificação perdida:** se a UI não ler `agent.shutdown` em 500 ms (travada, ou ainda no `ensure` inicial sem
  `Client`), o agente fecha mesmo assim; a UI trata como queda e **relança o agente** depois de 1 s — o "Sair" não
  "pega". Aceito: janela pequena; o usuário pode "Sair" de novo. As conexões de sonda (`probePinger`) ignoram
  notificações.
- **Foco ao trazer a janela à frente:** regras de foreground do Windows podem fazer só o botão piscar (EXE-011
  aceita). `AllowSetForegroundWindow(ASFW_ANY)` na segunda instância reduz o problema.
- **Relançamento em loop:** limitado por EXE-022/EXE-023 (no máximo 3 tentativas seguidas; conexão < 60 s conta como
  falha). Um agente que sobe e cai a cada ~61 s seria relançado indefinidamente (1 por minuto) — aceito.
- **Usuário mata o agente no Gerenciador de Tarefas:** os dois processos se chamam `arcadia.exe`; matar o agente com a
  UI aberta faz a UI relançá-lo (decisão do usuário). Matar a UI não afeta o agente.
- **Entrada Run disputada:** duas cópias ligadas reescrevem o valor `Arcadia` a cada subida (última vence); a regra de
  AST-005 impede uma cópia desligada de apagar a entrada da outra. Limitação aceita (requirements.md, Limitações
  conhecidas), assim como a instância única da UI compartilhada entre cópias (`UniqueId` fixo).
- **Falha persistente do IPC:** com EXE-026, um agente cujo `Accept` falha sempre sobe, encerra e é relançado pela UI
  até o limite de EXE-022 (3 falhas seguidas) — sem loop infinito.
- **StartupApproved:** se o usuário desativar o Arcadia em Configurações → Aplicativos → Inicialização, o Windows marca
  `Explorer\StartupApproved\Run` e não executa a entrada; o item da bandeja continuará marcado (fora de escopo).
- **Handles:** registro (abre/fecha por operação), processos criados (`CreateDetached` fecha `hProcess`/`hThread`),
  mutex do agente, pipe e janela do watcher — inalterados. Goroutines novas: só o supervisor (dono `App.wg`, sai com
  `ctx`/fechamento de `Notifications()`); `Drain` não cria goroutine (espera na goroutine chamadora).
- **Bindings do Wails:** `wails build` compila e executa o app com a tag `bindings` para gerar `frontend/wailsjs`;
  nesse modo o `OnStartup` não roda, então nenhum agente é iniciado. Sem `--agent` nos argumentos, o modo é UI, como
  precisa ser.
