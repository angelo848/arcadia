# Executável único — tarefas

- **Status:** aprovado pelo usuário (2026-11-08)
- **Spec:** [requirements.md](requirements.md) · [design.md](design.md) · [contracts/ipc.openrpc.json](contracts/ipc.openrpc.json) (v0.3.0)
- **ADRs:** 0003 (substituído em parte), 0006, 0010 (só o agente migra), 0011 (Clock), 0013 (log fecha por último), 0014 (executável único)

## Decisões sobre as lacunas do plano (sessão principal, 2026-11-08)
1. **Limite do drain (EXE-014):** segue o design. `Drain` é limitado só pelo `ctx`; o agente usa
   `context.WithTimeout(ctx, instance.drainTimeout)` (padrão `shutdownDrainTimeout = 500 ms`), como o
   `shutdownTimeout` de 2 s da fase 2. Os testes reduzem o campo ou cancelam o `ctx` à mão.
2. **AST-009 na subida:** se `Reconcile` falhar ao ler a preferência (`read-setting`), ou ao gravar `"true"` na
   primeira execução (`write-setting`), loga `Error`, não toca o registro e devolve `false` (item desmarcado). A
   próxima subida tenta de novo.
3. **Rastreabilidade da fase 2:** o `spec-trace.ps1` deixa de exigir teste de um requisito cuja seção contenha
   "substituído por" (AGT-008 perde o teste em EXE-009).
4. **Agente que morre entre o `Ensure` e o primeiro dial do `Client`:** se o `Client` não conectar em 5 s depois de
   um `Ensure` bem-sucedido, o supervisor trata isso como queda (EXE-020). Teste em T09.
5. **`.gitignore`:** `cmd/desktop/build/bin/` → `cmd/arcadia/build/bin/`.
6. **Nome da constante:** `ASFW_ANY` vira `winapi.ASFWAny` (`0xFFFFFFFF`).

## Resumo
São onze tarefas, em ordem de dependência:
- **T01–T04 (partes portáveis ou isoladas):** store com `app_setting` (T01); IPC com `agent.shutdown` e `Drain`
  (T02); estado do `Client` (T03); winapi com registro e foco (T04).
- **T05–T07 (agente):** pacote `internal/agent` com `Run(Options) int` (T05); shutdown com motivo, drain e EXE-026
  (T06); início com o Windows (T07).
- **T08–T09 (UI):** `cmd/arcadia` com seleção de modo, removendo `cmd/agent` e `cmd/desktop` (T08); supervisor e
  instância única da UI (T09).
- **T10–T11:** documentação e scripts (T10); verificação manual (T11).

Até T08, `cmd/agent/main` continua existindo como casca que chama `agent.Run`, para o build ficar verde em cada
tarefa. Cada tarefa passa pelo ciclo test-writer → implementer → reviewer antes da próxima.

**Premissas:**
- Go 1.27, Wails v2.17.1 (`options.SingleInstanceLock` existe) e `golang.org/x/sys` v0.49.0.
- `golang.org/x/sys/windows/registry` já faz parte desse módulo, então não há dependência nova.
- Ao fim de cada tarefa passam `CGO_ENABLED=0 go build ./...` (sem Node) e `GOOS=linux go vet ./...`.
- Win32 fica só em `*_windows.go`.

---

## T01 — Store: tabela `app_setting`, versão 2 e preferências
- Requisitos: AST-001, STO-001
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `internal/store/migrations/00002_app_setting.sql` (DDL do design)
  - `internal/store/queries/settings.sql` (`GetSetting :one`, `UpsertSetting :exec` com `ON CONFLICT(key) DO UPDATE`)
  - `internal/store/dbgen/*` (regerado com `sqlc generate`)
  - `internal/store/store.go` (`SchemaVersion = 2`)
  - `internal/store/settings.go` (novo)
  - testes: `internal/store/settings_test.go` e ajustes em `store_test.go` (casos que fixavam a versão 1)
- Assinaturas públicas:
  ```go
  package store
  const SchemaVersion int64 = 2
  var ErrSettingNotFound = errors.New("store: setting not found")
  func (s *Store) Setting(ctx context.Context, key string) (string, error)               // sql.ErrNoRows => ErrSettingNotFound (%w)
  func (s *Store) SetSetting(ctx context.Context, key, value string, at time.Time) error // updated_at = TimeToMillis(at)

  package dbgen // generated (expected shape)
  type AppSetting struct{ Key string; Value string; UpdatedAt int64 }
  func (q *Queries) GetSetting(ctx context.Context, key string) (AppSetting, error)
  type UpsertSettingParams struct{ Key string; Value string; UpdatedAt int64 }
  func (q *Queries) UpsertSetting(ctx context.Context, arg UpsertSettingParams) error
  ```
- Testes (`t.TempDir()`):
  - `TestAppSettingSchema`:
    - "AST-001/tabela app_setting com key TEXT PK, value TEXT NOT NULL, updated_at INTEGER NOT NULL"
    - "AST-001/banco novo fica na versão 2"
    - "AST-001/banco na versão 1 migra para 2 mantendo os dados" (o banco v1 sai de goose `UpTo(1)`)
  - `TestSettings`:
    - "AST-001/chave inexistente retorna ErrSettingNotFound"
    - "AST-001/gravar duas vezes devolve o último valor"
    - "AST-001/updated_at é UnixMilli do instante recebido"
  - Os casos STO-001, STO-002 e STO-009 que comparam com a versão 1 passam a usar `SchemaVersion`.
- Dependências: —
- Depende de: —

## T02 — IPC: notificação `agent.shutdown` e `Server.Drain`
- Requisitos: EXE-013, EXE-014, EXE-015, EXE-027
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `internal/ipc/protocol.go`: `MethodShutdown`, `ShutdownParams` e o comentário do pacote apontando para
    `docs/specs/single-exe/contracts/ipc.openrpc.json`
  - `internal/ipc/server.go`: campos `draining` e `drainMsg`, um sinal quando `len(conns)` chega a 0, e `startConn`
    enfileirando a mensagem quando está em drain
  - testes: `internal/ipc/drain_test.go`; o `contract_test.go` passa a ler o contrato de `single-exe`
- Assinaturas públicas:
  ```go
  package ipc
  const MethodShutdown = "agent.shutdown"
  type ShutdownParams struct{ Reason string `json:"reason"` }
  // DrainError is returned by Drain when ctx ends with connections still open.
  type DrainError struct {
      Pending int   // connections still open
      Err     error // ctx.Err()
  }
  func (e *DrainError) Error() string
  func (e *DrainError) Unwrap() error
  // Drain returns nil immediately when Serve already returned (EXE-027) or no connection is open;
  // it never closes connections nor stops accepting.
  func (s *Server) Drain(ctx context.Context, method string, params any) error
  ```
- Testes:
  - `TestDrain`:
    - "EXE-013/cada conexão recebe agent.shutdown com params.reason"
    - "EXE-013/a notificação não tem id"
    - "EXE-013/a notificação chega antes de o servidor fechar a conexão"
    - "EXE-014/todos os clientes fecham: Drain retorna nil antes do prazo"
    - "EXE-014/cliente que não fecha: Drain retorna errors.Is DeadlineExceeded e DrainError.Pending = 1"
    - "EXE-014/Drain não fecha conexões nem para o accept"
    - "EXE-014/sem conexões abertas retorna nil na hora"
    - "EXE-014/Close depois do Drain fecha as conexões restantes"
  - `TestDrainNewConn`:
    - "EXE-015/conexão aceita durante o drain recebe agent.shutdown como primeira mensagem, com o mesmo reason"
  - `TestDrainAfterServe` (`ctx` sem prazo; o teste falha se bloquear):
    - "EXE-027/Serve encerrado por erro de Accept: Drain retorna nil sem esperar"
    - "EXE-027/após Close: Drain retorna nil"
    - "EXE-027/após fim do ctx do Serve: Drain retorna nil"
    - "EXE-027/nada é enviado"
  - `TestContract`:
    - "EXE-013/agent.shutdown é notificação com reason enum quit|session-end|signal|other"
    - "EXE-014/x-limits.shutdownDrainMs = 500"
    - os casos IPC-* existentes passam a usar o contrato v0.3.0
  - `TestShutdown` (goleak): "IPC-006/Drain não deixa goroutines"
- Depende de: —

## T03 — IPC: estado de conexão do `Client`
- Requisitos: EXE-019
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `internal/ipc/client.go`: canal `state` com buffer 1; o sinal é não bloqueante e sai depois de publicar
    `c.conn` e depois de `c.conn = nil`; o canal é fechado em `run`, junto com `notifications`
  - teste: `internal/ipc/client_state_test.go`
- Assinaturas públicas:
  ```go
  func (c *Client) Connected() bool
  func (c *Client) StateChanged() <-chan struct{} // buffer 1, coalescing, closed by Close
  ```
- Testes (`FakeClock` + `BlockUntilWaiters`):
  - `TestClientState`:
    - "EXE-019/Connected falso antes do primeiro dial"
    - "EXE-019/conectar sinaliza StateChanged e Connected fica verdadeiro"
    - "EXE-019/queda sinaliza StateChanged e Connected fica falso"
    - "EXE-019/sinais seguidos se acumulam em um sem bloquear o cliente"
    - "EXE-019/notificação lida antes do EOF já está em Notifications ao observar a queda"
    - "EXE-019/StateChanged é fechado por Close"
  - `TestShutdown` (goleak): "IPC-006/Close com StateChanged sem leitor não deixa goroutines"
- Depende de: —

## T04 — winapi: entrada Run do registro e permissão de foco
- Requisitos: AST-002, AST-003, AST-004, AST-005, AST-009, EXE-011
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `internal/winapi/autostart_windows.go` (novo, usa `golang.org/x/sys/windows/registry`)
  - `internal/winapi/user32_windows.go` (ganha `procAllowSetForegroundWindow`)
  - `internal/winapi/foreground_windows.go` (novo)
  - `internal/winapi/doc.go`
  - teste: `internal/winapi/autostart_windows_test.go`
- Assinaturas públicas:
  ```go
  //go:build windows
  package winapi
  const (
      RunKeyPath         = `Software\Microsoft\Windows\CurrentVersion\Run`
      AutostartValueName = "Arcadia"
      ASFWAny     uint32 = 0xFFFFFFFF // ASFW_ANY
  )
  type RunValue struct{ /* keyPath, name; no handle kept */ }
  func NewRunValue(keyPath, name string) *RunValue            // HKCU\keyPath, value name (REG_SZ)
  func (r *RunValue) Get() (value string, ok bool, err error) // absent key/value => ok=false, nil
  func (r *RunValue) Set(value string) error                  // CreateKey + SetStringValue
  func (r *RunValue) Delete() error                           // absent => nil
  func AutostartCommand(exe, root, exeDir string) string      // AST-002
  func CommandExe(cmdline string) (string, error)             // windows.DecomposeCommandLine, first token; empty => error
  func AllowSetForegroundWindow(pid uint32) error             // wraps windows.Errno
  ```
- Testes (só na subchave `Software\ArcadiaTest-<rand>` de HKCU, removida em `t.Cleanup`; nunca a `Run` real):
  - `TestAutostartCommand`:
    - "AST-002/root igual a D\data omite --data-dir"
    - "AST-002/root igual a D\data com caixa e barra final diferentes omite --data-dir"
    - "AST-002/root com espaço vem entre aspas"
    - "AST-002/root D:\ sem aspas"
    - "AST-002/exe sempre entre aspas"
  - `TestCommandExe`:
    - "AST-005/primeiro token entre aspas"
    - "AST-005/primeiro token sem aspas"
    - "AST-005/linha vazia retorna erro"
    - "AST-005/CommandExe(AutostartCommand(E,...)) == E"
  - `TestRunValue`:
    - "AST-003/Set grava REG_SZ e Get lê o mesmo valor"
    - "AST-004/Get de valor ausente retorna ok=false sem erro"
    - "AST-004/Get com subchave ausente retorna ok=false sem erro"
    - "AST-005/Delete remove o valor"
    - "AST-005/Delete de valor ausente retorna nil"
    - "AST-009/valor que não é REG_SZ retorna erro em Get"
    - "AST-009/nenhum handle de registro sobra" (`GetProcessHandleCount` antes e depois de 20 operações)
- Depende de: —

## T05 — internal/agent: `cmd/agent` vira o pacote `internal/agent` com `Run(Options) int`, flags e "Abrir Arcadia"
- Requisitos: EXE-003, EXE-009, EXE-010, AGT-002, AGT-004, AGT-005, AGT-006, IPC-019
- Responsável: implementer (código), test-writer (move e adapta os testes)
- Arquivos:
  - `git mv`:
    - `cmd/agent/{app.go,tray_loop.go,tray_windows.go,openui.go,assets/icon.ico}` → `internal/agent/`
    - `cmd/agent/main_windows.go` → `internal/agent/run_windows.go`
    - `cmd/agent/main_other.go` → `internal/agent/run_other.go`
  - renomeações: `package agent`; o tipo `agent` vira `instance`; `config` ganha `Version`
  - `cmd/agent/main_*.go` viram cascas temporárias que chamam `agent.Run` (removidas em T08)
  - testes: `git mv` de `cmd/agent/*_test.go` para `internal/agent/`, mais `run_windows_test.go` (novo)
- Assinaturas:
  ```go
  package agent
  type Options struct {
      Args    []string // os.Args[1:], may contain "--agent"
      Version string   // main.version; "dev" when not stamped
  }
  func Run(opts Options) int // Windows only; elsewhere returns 1

  type config struct{ DataDir, LogLevel, ExeDir, Exe, PipeName, Version string }
  func parseFlags(args []string) (config, error) // FlagSet "agent", ContinueOnError; agent, data-dir, log-level
  type instance struct{ /* ... */ }
  func startAgent(ctx context.Context, cfg config, clock domain.Clock, getenv func(string) string,
      listen func() (net.Listener, error)) (*instance, error)
  func openUI(exe, root string, spawn spawnFunc, log *slog.Logger) // spawn(exe, ["--data-dir", root], DETACHED|NEW_PROCESS_GROUP)
  ```
  Ordem do `Run`:
  1. `parseFlags` (erro → 2, antes de qualquer outra coisa) e `ParseLevel` (erro → 2)
  2. mutex (`already` → 0)
  3. `os.Executable`
  4. SID
  5. `startAgent`
  6. watcher
  7. bandeja
  8. shutdown
- Testes:
  - `TestParseFlags`:
    - "EXE-003/--agent --foo retorna erro"
    - "EXE-003/--agent --data-dir X é aceito"
    - "EXE-003/-agent é aceito"
    - "AGT-005/--log-level debug é lido"
  - `TestRunFlags`:
    - "EXE-003/Run com --agent --foo --data-dir <tmp>/x retorna 2 e não cria <tmp>/x"
    - "AGT-005/Run com --log-level inválido retorna 2 e não cria a pasta"
  - `TestOpenUI`:
    - "EXE-009/spawn com o exe recebido, --data-dir R e flags DETACHED|NEW_PROCESS_GROUP"
    - "EXE-009/sem --agent nos argumentos"
    - "EXE-010/falha do spawn loga Error falha ao abrir o Arcadia com path e error"
    - "EXE-010/falha do spawn não interrompe"
  - migrados com o mesmo nome: AGT-001, AGT-003, AGT-005, AGT-006, AGT-007 (trayLoop) e IPC-019
  - novo: "AGT-004/Info().Version vem de config.Version"
- Depende de: —

## T06 — internal/agent: shutdown com motivo, drain e agente sem IPC
- Requisitos: EXE-013, EXE-014, EXE-016, EXE-018, EXE-026, EXE-027, AGT-003, AGT-006
- Responsável: implementer · Testes: test-writer
- Arquivos: `internal/agent/{app.go,tray_loop.go,tray_windows.go,run_windows.go}`; testes `app_test.go`,
  `tray_loop_test.go` e `shutdown_test.go` (novo)
- Assinaturas:
  ```go
  type ShutdownReason string
  const (
      ReasonQuit       ShutdownReason = "quit"
      ReasonSessionEnd ShutdownReason = "session-end"
      ReasonSignal     ShutdownReason = "signal"
      ReasonOther      ShutdownReason = "other"
  )
  const shutdownDrainTimeout = 500 * time.Millisecond // instance.drainTimeout default
  type trayLoopDeps struct {
      Ready                 <-chan trayItems
      SessionEnd, IPCFailed <-chan struct{} // nil never fires
      Stop                  <-chan struct{}
      Quit, QuitBeforeReady func()
      OnOpen                func()
      OnToggle              func() bool // T07
      SetChecked            func(bool)  // T07
  }
  func trayLoop(ctx context.Context, clock domain.Clock, log *slog.Logger, d trayLoopDeps) ShutdownReason
  func runTray(ctx context.Context, clock domain.Clock, log *slog.Logger, sessionEnd, ipcFailed <-chan struct{}, onOpen func()) ShutdownReason
  func serveFailed(err error) bool                 // false for nil and errors.Is(err, context.Canceled)
  func (a *instance) IPCFailed() <-chan struct{}   // closed once by the Serve goroutine
  func (a *instance) Shutdown(ctx context.Context, reason ShutdownReason) error
      // Drain(≤ drainTimeout; DrainError => Warn) -> Server.Close -> wg.Wait -> Store.Close; idempotent
  func exitCode(shutdownErr error, ipcFailed bool) int // 1 if either; else 0
  ```
  Quando `serveFailed(err)` é verdadeiro: log `Error` "IPC indisponível; encerrando o agente" e `close(ipcFailed)`.
- Testes:
  - `TestTrayLoopReason`:
    - "EXE-016/Sair devolve quit"
    - "EXE-016/fim de sessão antes e depois da bandeja devolve session-end"
    - "EXE-016/Ctrl+C antes e depois da bandeja devolve signal"
    - "EXE-016/stop devolve other sem chamar nada"
    - "EXE-026/ipcFailed depois do ready chama Quit e devolve other"
    - "EXE-026/ipcFailed antes do ready chama QuitBeforeReady e devolve other"
    - "EXE-026/IPCFailed nil nunca dispara"
    - os casos AGT-003, AGT-006 e AGT-007 existentes, adaptados a `trayLoopDeps`
  - `TestServeFailed`:
    - "EXE-026/nil não é falha"
    - "EXE-026/context.Canceled não é falha"
    - "EXE-026/erro de Accept é falha"
    - "EXE-026/net.ErrClosed sem Close é falha"
  - `TestShutdownDrain`:
    - "EXE-013/Shutdown envia agent.shutdown com o reason ao cliente conectado"
    - "EXE-014/cliente que fecha libera o Shutdown antes do limite"
    - "EXE-014/cliente que não fecha: loga Warn com pending=1 e conclui fechando o store"
    - "AGT-003/ordem Drain → Server.Close → Store.Close"
    - "AGT-003/Shutdown idempotente com reason"
  - `TestIPCFailed`:
    - "EXE-026/Serve com 10 falhas de Accept fecha IPCFailed e loga Error IPC indisponível"
    - "EXE-026/Shutdown normal não fecha IPCFailed"
    - "EXE-027/Shutdown depois de IPCFailed não espera o drain"
  - `TestExitCode`:
    - "EXE-026/ipcFailed sem erro de shutdown sai com 1"
    - "AGT-003/sem erro sai com 0"
    - "AGT-003/erro de shutdown sai com 1"
- Depende de: T02, T05

## T07 — internal/agent: início com o Windows
- Requisitos: AST-002..AST-011
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `internal/agent/autostart.go` (novo, portável)
  - `internal/agent/tray_windows.go`: `AddMenuItemCheckbox` entre "Abrir Arcadia" e o separador; `Disable()` em dev
  - `internal/agent/tray_loop.go`: `trayItems.autostart`; o clique chama `OnToggle` e depois `SetChecked`
  - `internal/agent/run_windows.go`: monta o `autostart` e chama `Reconcile` depois do `startAgent` e antes da bandeja
  - testes: `autostart_test.go`, `tray_loop_test.go`
- Assinaturas:
  ```go
  const settingAutostart = "autostart"
  type runEntry interface{ Get() (string, bool, error); Set(value string) error; Delete() error }
  type settingStore interface {
      Setting(ctx context.Context, key string) (string, error)
      SetSetting(ctx context.Context, key, value string, at time.Time) error
  }
  type autostart struct {
      enabled, dev bool
      command, exe string
      exeOf        func(cmdline string) (string, error)
      run          runEntry
      settings     settingStore
      clock        domain.Clock
      log          *slog.Logger
  }
  func (a *autostart) Reconcile(ctx context.Context) (checked bool)
  func (a *autostart) Toggle(ctx context.Context) (checked bool)
  ```
  Logs:
  - `Error` "início com o Windows: falha", com `op` ∈ {read-setting, write-setting, read-run, write-run, delete-run}
  - `Info` "início com o Windows desativado em build de desenvolvimento", uma vez, em dev
- Testes:
  - `TestReconcile`:
    - "AST-003/chave ausente grava true com o instante do relógio, faz Set(command) e devolve true"
    - "AST-004/ligado e valor ausente faz Set"
    - "AST-004/ligado e valor diferente faz Set"
    - "AST-004/ligado e valor igual não escreve"
    - "AST-005/desligado e mesmo exe (caixa diferente) faz Delete"
    - "AST-005/desligado e outro exe não altera"
    - "AST-005/desligado e valor ausente não altera"
    - "AST-005/desligado e valor ilegível não remove e loga Warn"
    - "AST-009/falha de read-setting loga Error e não toca o registro"
    - "AST-009/falha de write-setting na primeira execução loga Error e não toca o registro"
    - "AST-009/falha de Get loga op=read-run"
    - "AST-009/falha de Set loga op=write-run"
    - "AST-009/falha de Delete loga op=delete-run"
  - `TestToggle`:
    - "AST-007/ligado: grava false, remove a entrada própria e devolve false"
    - "AST-007/ligado com entrada de outro exe: grava false e não remove"
    - "AST-008/desligado: grava true, faz Set(command) e devolve true"
    - "AST-009/falha de write-setting devolve o estado antigo sem tocar o registro"
    - "AST-009/falha de registro devolve o estado novo e loga Error"
  - `TestAutostartDev`:
    - "AST-010/Reconcile em dev não chama registro nem preferência, devolve false e loga Info uma vez"
    - "AST-010/Toggle em dev não chama nada e devolve false"
  - `TestTrayLoopToggle`:
    - "AST-007/clique chama OnToggle e SetChecked(false)"
    - "AST-008/clique chama OnToggle e SetChecked(true)"
  - manuais (T11): AST-006, AST-010 e AST-011
- Depende de: T01, T04, T06

## T08 — cmd/arcadia: executável único, seleção de modo e início do agente; remove `cmd/agent` e `cmd/desktop`
- Requisitos: EXE-001, EXE-002, EXE-004, EXE-005, EXE-006, EXE-007, EXE-008, AGT-004, DSK-005, DSK-002
- Responsável: implementer (código), test-writer (move e adapta os testes)
- Arquivos:
  - `git mv cmd/desktop cmd/arcadia` (inclui `wails.json` e `build/`)
  - `git rm -r cmd/agent`
  - `cmd/arcadia/main.go` vira `main_windows.go`: `main` faz `os.Exit(run(os.Args[1:]))`; `run` decide o modo antes de qualquer outra coisa
  - `cmd/arcadia/main_other.go`
  - `cmd/arcadia/mode.go` (novo)
  - `cmd/arcadia/version.go` (novo, portável)
  - `cmd/arcadia/agentproc_windows.go`
  - `cmd/arcadia/app.go`
  - `.gitignore`
  - testes: `git mv` dos `*_test.go`, mais `mode_test.go` e `version_test.go`
- Assinaturas:
  ```go
  var version = "dev" // wails build -ldflags "-X main.version=<v>"
  func isAgentMode(args []string) bool // some arg exactly "--agent" or "-agent"
  func run(args []string) int          // agent => agent.Run(agent.Options{Args: args, Version: version}); else runUI(args)
  func runUI(args []string) int
  type desktopConfig struct{ DataDir, Exe, ExeDir string }
  func agentCommand(exe, root string) (string, []string, uint32) // exe, ["--agent","--data-dir",root], DETACHED|NEW_PROCESS_GROUP|BREAKAWAY
  func startAgentProcess(spawn spawnFunc, exe, root string, log *slog.Logger) error
  ```
  O modo UI nunca adquire o mutex do agente nem escuta o pipe (EXE-004).
- Testes:
  - `TestIsAgentMode`:
    - "EXE-001/[] é UI"
    - "EXE-001/--data-dir X é UI"
    - "EXE-001/--agent é agente"
    - "EXE-001/-agent é agente"
    - "EXE-001/--data-dir X --agent é agente"
    - "EXE-001/--agent=false é UI"
    - "EXE-001/--agents é UI"
  - `TestVersion`: "AGT-004/sem -X version é dev"
  - `TestAgentCommand`:
    - "EXE-007/exe é o caminho recebido, com qualquer nome de arquivo"
    - "EXE-007/args são --agent --data-dir R"
    - "EXE-007/flags DETACHED|NEW_PROCESS_GROUP|BREAKAWAY"
    - "EXE-007/root com espaços é um único argumento"
  - `TestStartAgentProcess`:
    - os casos DSK-005 existentes
    - "EXE-007/fallback sem BREAKAWAY usa o mesmo exe e args"
  - os demais casos migrados (DSK-001, DSK-003, DSK-006..DSK-009) mantêm o nome
  - manuais (T11): EXE-002, EXE-004, EXE-005, EXE-006 e EXE-008
- Depende de: T05, T06, T07

## T09 — cmd/arcadia: supervisor do agente, fechar ao receber `agent.shutdown` e instância única da UI
- Requisitos: EXE-011, EXE-012, EXE-017, EXE-018, EXE-020..EXE-025, DSK-003, DSK-007
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `cmd/arcadia/supervisor.go` (novo, portável)
  - `cmd/arcadia/app.go`:
    - o supervisor substitui `connectAndForward`, `waitAgent` e `lateConnect`
    - `ipcClient` ganha `StateChanged()` e `Connected()`
    - `platform` ganha `quit` e `showWindow`
    - novo `onSecondInstance`
  - `cmd/arcadia/main_windows.go`:
    - `SingleInstanceLock{UniqueId: "arcadia-ui-5b0f7c1e", OnSecondInstanceLaunch}`
    - `winapi.AllowSetForegroundWindow(winapi.ASFWAny)` antes do `wails.Run`
    - `quit` = `wailsruntime.Quit`; `showWindow` = `WindowUnminimise` + `WindowShow`
  - `cmd/arcadia/main_other.go`
  - testes: `supervisor_test.go` (novo), `app_test.go`
- Assinaturas:
  ```go
  const (
      relaunchMaxFailures = 3
      stableConnection    = 60 * time.Second
      firstDialTimeout    = 5 * time.Second // decisão 4
  )
  var relaunchWaits = [relaunchMaxFailures]time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second}
  type ipcClient interface {
      pinger
      Notifications() <-chan ipc.Notification
      StateChanged() <-chan struct{}
      Connected() bool
      Close() error
  }
  type connectGate interface{ MarkConnected() }
  type supervisorConfig struct {
      Clock     domain.Clock
      Ensure    func(ctx context.Context) (ipc.AgentStatus, error)
      NewClient func() (ipcClient, error)
      Publish   func(c ipcClient) bool
      Gate      connectGate
      Root      string
      Emit      func(event string, data any)
      Quit      func()
      Log       *slog.Logger
  }
  func newSupervisor(cfg supervisorConfig) *supervisor
  func (s *supervisor) run(ctx context.Context) // owner: App.wg
  func (s *supervisor) Kick()                   // non-blocking
  func (a *App) secondInstance()                // showWindow + Info + Kick (nil-safe)
  ```
  Antes de tratar uma queda, o supervisor drena `Notifications()` sem bloquear.
- Testes (`FakeClock` + `BlockUntilWaiters`):
  - `TestSupervisorStartup`:
    - "EXE-024/ensure inicial falha: conta 1 falha, loga falha ao relançar o agente e espera 2 s"
    - "EXE-024/nada dispara 1ns antes de 2 s"
    - "DSK-007/ensure ok marca o gate e checa dataDir"
    - "DSK-007/após ensure falho, primeiro heartbeat ou Connected marca o gate e checa dataDir"
    - "EXE-020/ensure ok mas Client não conecta em 5 s é tratado como queda" (decisão 4)
  - `TestSupervisorDrop`:
    - "EXE-020/queda após ≥ 60 s loga Warn, espera 1 s e loga relançando o agente com attempt=1 max=3"
    - "EXE-020/tentativa chama Ensure"
    - "EXE-021/conexão volta durante a espera: Ensure não é chamado e loga agente reconectado"
    - "EXE-021/Ensure ok na tentativa loga agente reconectado"
    - "EXE-023/conexão < 60 s conta falha e loga agente caiu logo após subir"
    - "EXE-023/conexão ≥ 60 s zera o contador"
  - `TestSupervisorLimit`:
    - "EXE-022/esperas 1 s 2 s 4 s indexadas pelas falhas"
    - "EXE-022/sem disparo 1ns antes de cada espera"
    - "EXE-022/cada falha loga Error com attempt e error"
    - "EXE-022/3ª falha loga desistindo de relançar o agente e não tenta mais"
    - "EXE-022/em GaveUp, conexão que volta é detectada"
  - `TestSupervisorShutdownNotice`:
    - "EXE-017/agent.shutdown loga Info com reason e chama Quit uma vez"
    - "EXE-017/queda depois de agent.shutdown não chama Ensure"
    - "EXE-017/agent.shutdown entregue antes do sinal de queda é visto primeiro"
    - "EXE-017/segundo agent.shutdown não chama Quit de novo"
  - `TestSupervisorKick`:
    - "EXE-012/Kick em GaveUp zera falhas e tenta sem espera"
    - "EXE-012/Kick em outro estado não tenta"
    - "EXE-012/Kick não bloqueia"
  - `TestSupervisorStop`:
    - "EXE-025/ctx cancelado durante a espera: Ensure não é chamado e run retorna"
    - "EXE-025/ctx cancelado durante Ensure: start não ocorre depois e run retorna"
    - "EXE-025/Publish falso fecha o client e sai"
  - `TestSupervisorHeartbeat`:
    - "DSK-003/heartbeat repassado como agent:heartbeat"
    - "DSK-003/heartbeat com params inválidos não é repassado"
  - `TestAppShutdown`:
    - "EXE-025/OnShutdown só retorna depois de o supervisor terminar"
    - "EXE-025/nenhum start depois do início do OnShutdown"
    - "DSK-002/OnShutdown não encerra o agente"
  - `TestSecondInstance`:
    - "EXE-012/secondInstance chama showWindow, loga Info e Kick"
    - "EXE-012/secondInstance antes do startup não falha"
  - manuais (T11): EXE-011 e EXE-018
- Depende de: T03, T08

## T10 — Documentação, scripts e limpeza
- Requisitos: EXE-005, EXE-006, AST-011
- Responsável:
  - sessão principal: `CLAUDE.md` e `docs/ARCHITECTURE.md`
  - implementer: `scripts/gen-icon.go`, `scripts/spec-trace.ps1` (decisão 3) e os comentários de `frontend/src/lib/agent.ts`
- Arquivos:
  - `CLAUDE.md`:
    - um executável com dois processos
    - layout com `cmd/arcadia` e `internal/agent`
    - os comandos `go run ./cmd/arcadia --agent --data-dir ./.devdata` e `cd cmd/arcadia && wails dev|build`
    - link para a spec atual
  - `docs/ARCHITECTURE.md`: diagrama da §2, layout de `cmd/` e a pergunta 5 respondida (ADR 0014)
- Testes: nenhum automatizado
- Depende de: T08, T09

## T11 — Verificação final e checagens manuais
- Requisitos: EXE-002, EXE-004, EXE-005, EXE-006, EXE-008, EXE-011, EXE-018, AST-006, AST-010, AST-011, BLD-001,
  AGT-002, AGT-003, DSK-002
- Responsável: reviewer (os builds ficam com o implementer)
- Roteiro:
  - **BLD-001 / EXE-006:** clone limpo sem Node.
    - `CGO_ENABLED=0 go build ./...`, `go test ./...`, `go vet ./...` e `GOOS=linux go vet ./...` passam.
    - `go build -o arcadia.exe ./cmd/arcadia` seguido de `arcadia.exe --agent --data-dir X` sobe o agente.
  - **EXE-005:** `wails build -ldflags "-X main.version=1.2.3"` gera só `arcadia.exe`; não existem mais
    `cmd/agent` nem `cmd/desktop`.
  - **EXE-002:** `arcadia.exe --agent --data-dir X`.
    - Cria o banco na versão 2, o `agent.log` e o ícone na bandeja.
    - Não abre janela nem `msedgewebview2.exe` e não cria `desktop.log`.
    - Anotar o working set.
  - **AGT-002:** um segundo `--agent` sai com 0.
  - **EXE-004:** com a UI aberta e o agente parado, `--agent` à mão sobe normalmente.
  - **EXE-008:** agente parado; abrir `arcadia.exe` cria `arcadia.exe --agent --data-dir R` desacoplado e mostra
    "Conectado" em até 5 s.
  - **EXE-011:** com a janela visível, minimizada ou atrás de outras, abrir de novo pelo Explorer, por atalho e
    por "Abrir Arcadia".
    - A janela existente vem para a frente.
    - O segundo processo sai com 0.
    - Nunca há duas UIs.
  - **EXE-018 / AGT-003:** "Sair" com a janela visível, minimizada e em segundo plano.
    - A UI fecha sem confirmação.
    - Os dois processos terminam, o agente em menos de 2 s.
    - Nenhum agente novo é criado.
  - **EXE-020..EXE-023 (fumaça):** `Stop-Process` no agente faz a UI relançá-lo depois de ~1 s.
  - **DSK-002:** fechar a UI deixa o agente na bandeja.
  - **AST-006 / AST-010:**
    - Release: o item vem marcado na primeira execução, a entrada `HKCU\...\Run\Arcadia` existe, e desmarcar
      remove a entrada.
    - Dev: o item fica desabilitado e o registro não é tocado.
  - **AST-011:** logoff e logon com o item ligado mostram só o ícone na bandeja.
  - **Rastreabilidade:** `spec-trace.ps1` sai com 0 para `single-exe` e para `phase-2-skeleton`.
- Depende de: T01–T10

---

## Riscos de concorrência e recursos

| Goroutine | Dono | Como termina |
|---|---|---|
| Supervisor do agente (UI) | `App.wg` | `ctx` cancelado no `OnShutdown` ou `Notifications()` fechado por `Client.Close` |
| Sondas de `probeStatus` | `probeStatus` | sempre esperadas antes de retornar |
| `Serve` do agente (também fecha `ipcFailed`) | `instance.wg` | `Close`, `ctx` ou erro; `ipcFailed` é fechado no máximo uma vez |
| `trayLoop` | `runTray` | devolve o `ShutdownReason`; depois `close(stop)` e `wg.Wait` |
| `Client.run` | `Client` | `Close`; fecha `StateChanged()` e `Notifications()` |
| `Drain` | — | não cria goroutine |

- **Shutdown do agente:**
  1. `systray.Quit`
  2. `Drain` (até 500 ms)
  3. `Server.Close`
  4. `wg.Wait`
  5. `Store.Close`
  6. log "shutdown concluído"
  7. `watcher.ShutdownDone`
  8. `watcher.Close`
  9. `mutex.Close`
  10. `Close` do log
- **Shutdown da UI:** `cancel` → `Client.Close` → `wg.Wait` → `libraryGate.Close` → log "UI encerrada" → `Close`
  do log.
- **Locks:**
  - `Drain` marca `draining` sob `s.mu` e só depois enfileira.
  - `startConn` lê `draining` sob o mesmo lock.
  - `serveDone` também é conferido sob `s.mu`.
- **Registro:** cada operação abre a chave, opera e fecha. Nenhum handle sobrevive; o teste confere com
  `GetProcessHandleCount`.
- **Riscos para conferir em T11:**
  - Wails linkado no processo do agente
  - `init()` do systray no processo da UI
  - `agent.shutdown` perdido com a UI travada (limitação 3)
  - relançamento logo após EXE-026 (limitação 4)
