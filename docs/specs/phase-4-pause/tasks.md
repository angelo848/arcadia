# Fase 4 — Pausa, retomada e "iniciar sem contar tempo": tarefas

- **Status:** aprovado pelo usuário (2026-11-10)
- **Spec:** [requirements.md](requirements.md) · [design.md](design.md) · [contracts/ipc.openrpc.json](contracts/ipc.openrpc.json) (v0.5.0) · [contracts/session-state-machine.md](contracts/session-state-machine.md)
- **ADRs:** 0005, 0009, 0010, 0011, 0012, 0013, 0014, 0015, 0016 e os novos 0017 (modelo de pausa), 0018 (entrada global e feedback nativo), 0019 (lançamento sem rastrear)

## Decisões do planner (a confirmar na aprovação)
1. **Canal de controle com buffer.** `ctrlCh` tem buffer 16 e o chamador só abandona o envio se o tracker fechar (`done`), nunca por `ctx`. Com canal sem buffer e `select` em `ctx.Done()`, um `Suspend` com a goroutine dona ocupada seria perdido quando o `ctx` vencesse, e PWR-005 exige que ele seja aplicado depois, com o instante original. Depois de enviado, o chamador espera a resposta ou o `ctx`.
2. **Componentes da plataforma chegam por fábrica, não por valor.** Os componentes precisam do `log`, que só existe dentro de `startAgent`. `config` ganha `Platform func(domain.Clock, *slog.Logger) platformParts`; `nil` significa nenhum componente. Só `Run` (em `run_windows.go`) preenche `Platform`.
   - Consequência: `go test ./internal/agent` nunca registra o atalho do desenvolvedor, nem abre janela, nem toca som.
   ```go
   type platformParts struct {
       Sources    []input.Source
       Power      powerStarter                          // nil => no power watcher
       NewOverlay func() (feedback.Overlay, error)      // nil => NopOverlay; error => Warn OVL-006 + NopOverlay
       NewSound   func() (feedback.Sound, error)        // nil => NopSound; error => Warn + NopSound
   }
   type powerStarter func(onSuspend func(at time.Time), onWake func(at time.Time, userPresent bool)) (io.Closer, error)
   ```
   - `winapi.PowerCallbacks` só existe no Windows, então o `agent` portável usa `powerStarter` e `run_windows.go` adapta.
3. **`Source` ganha `Name() string`** (`"hotkey"`, `"gamepad"`), para o agente logar qual fonte falhou (PAU-006). O `Hotkey.Start` loga ele mesmo o `Error` "atalho de pausa indisponível" com `hotkey` e `winError` (HOT-003); a fábrica de `run_windows.go` loga o `Warn` "controle indisponível (XInput)" (PAD-007). O agente loga só um `Warn` genérico, "fonte de gatilho de pausa indisponível" (`source`, `error`).
4. **Registro de janelas compartilhado.** O `wndProc` da `winapi` passa a despachar para `map[HWND]windowHandler`, com `type windowHandler func(hwnd, msg, wParam, lParam uintptr) (ret uintptr, handled bool)`. O design omite o `hwnd`, mas o aviso precisa dele no `WM_PAINT`.
5. **Tipos de erro no tracker.** `NotPausableError{Reason string}` e as constantes `ReasonNoSession = "no-session"`, `ReasonGrace = "grace"` ficam no pacote `tracker`. O agente converte para `ipc.SessionNotPausableData`.
6. **`feedback.Event` espelha `tracker.FeedbackKind` pelo valor** (`EventPaused = 1`, `EventResumed = 2`, e `FeedbackPaused = 1`, `FeedbackResumed = 2`). O agente converte com `feedback.Event(k)`, e um teste do agente trava a igualdade.
7. **"Pausa ignorada" é logada pelo coordenador do tracker** (Debug com o estado), porque a máquina é pura e só devolve zero efeitos.
8. **`Machine` mantém `NotPausable() (reason string, ok bool)`** para o coordenador decidir `*NotPausableError` sem duplicar a regra de estados.
9. **`RegisterSuspendResumeNotification` não entra.** O watcher de energia usa só `WM_POWERBROADCAST`. Se PWR-007 (T25) mostrar que faz falta, vira tarefa nova; não mexe nos callbacks.
10. **Testes de interface do usuário.** Qualquer teste que precisa de área de trabalho interativa chama `requireInteractiveDesktop(t)`, que dá `t.Skip` quando `OpenInputDesktop(0, false, DESKTOP_SWITCHDESKTOP)` falha. Testes de janela e de atalho que funcionam sem desktop rodam sempre.
    - A combinação de teste do atalho é `Ctrl+Alt+Shift+F24` (`VK_F24 = 0x87`). A combinação real `Ctrl+Alt+P` nunca é registrada em testes.

## Resumo
São vinte e cinco tarefas, em ordem de dependência:
- **T01–T05 (portáveis):** domain (T01); migração 4 (T02); store de pausas (T03); fechamento de pausa no fim da sessão, STO-023 e SES-025 (T04); protocolo IPC v0.5.0 (T05).
- **T06–T08 (tracker-specialist):** máquina pura com `Paused` (T06); coordenador com pausa, suspensão e retomada (T07); lançamento sem rastrear e integração Windows (T08).
- **T09–T11 (`internal/input`):** combo e poller portáveis (T09); atalho `RegisterHotKey` (T10); XInput real (T11).
- **T12–T13 (`winapi`):** `wndProc` compartilhado (T12); `PowerWatcher` (T13).
- **T14–T17 (feedback):** coordenador portátil (T14); geometria do aviso (T15); janela do aviso (T16); som (T17).
- **T18–T20 (agente):** handlers `game.launchUntracked`, `session.pause`, `session.resume` (T18); roteador e ponte de energia (T19); subida e shutdown com as fontes (T20).
- **T21–T23 (UI):** bindings Go (T21); lib do frontend (T22); componentes e `App` (T23).
- **T24–T25:** documentação (T24); verificação final com os requisitos manuais (T25).

Cada tarefa passa por test-writer → implementer/tracker-specialist → reviewer e vira um commit próprio.

**Premissas:**
- Ao fim de cada tarefa passam `CGO_ENABLED=0 go build ./...`, `go test ./...`, `go vet ./...` e `GOOS=linux go vet ./...`.
- **Nenhuma dependência nova de terceiros.** `goleak`, `x/sys/windows`, TanStack Query e Testing Library já estão no projeto. XInput (`xinput1_4.dll`), `Beep` (`kernel32`), `user32`, `gdi32` e `shcore` entram por `windows.NewLazySystemDLL` e declarações locais.
- `sqlc generate` é necessário em T02 (só conferência, diff esperado vazio) e T03.
- `cd cmd/arcadia && wails generate module` é necessário em T21; atualiza `frontend/wailsjs/`, que é versionado.
- `store.SchemaVersion` passa a 4 em T02. Os testes que fixavam 3 como literal (`migrate3_test.go`) são ajustados na mesma tarefa; os que já usam `store.SchemaVersion` (`store_test.go`, `settings_test.go`, `agent/app_test.go`) passam sem mudança.
- Mocks e falsos novos que os testes pedem (`fakeStore.PauseSession/ResumePause`, `fakeLauncher.LaunchDetached`, `fakeSessionTracker.Pause/Resume/LaunchUntracked`) são escritos pelo test-writer junto com os testes da tarefa.

---

## T01 — domain: estado `paused`, motivos de pausa e antirrepique
- Requisitos: DOM-010, DOM-011
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `internal/domain/session.go` (alterar)
  - testes: `internal/domain/session_test.go` (alterar `TestParseSessionState`: o caso "DOM-007/paused é erro" passa a ser aceito) e `internal/domain/pause_test.go` (novo)
- Assinaturas públicas:
  ```go
  const SessionPaused SessionState = "paused" // ParseSessionState passa a aceitá-lo

  type PauseReason string
  const (
      PauseHotkey  PauseReason = "hotkey"
      PauseGamepad PauseReason = "gamepad"
      PauseUI      PauseReason = "ui"
      PauseSleep   PauseReason = "sleep"
  )
  func ParsePauseReason(s string) (PauseReason, error) // exato; "", "Hotkey", "idle", "minimized" são erro
  func (r PauseReason) UserInitiated() bool            // hotkey, gamepad, ui

  const PauseToggleDebounce = 300 * time.Millisecond
  ```
- Testes:
  - `TestParseSessionState`:
    - "DOM-010/running, paused, grace, ended e discarded são aceitos"
    - "DOM-010/Paused com maiúscula é erro"
    - "DOM-010/vazio é erro"
    - "DOM-007/valor desconhecido continua erro"
  - `TestParsePauseReason`:
    - "DOM-011/hotkey, gamepad, ui e sleep são aceitos"
    - "DOM-011/vazio, Hotkey, idle e minimized são erro"
  - `TestPauseReasonUserInitiated`: "DOM-011/hotkey, gamepad e ui são do usuário e sleep não"
- Depende de: —

## T02 — store: migração 4 e `SchemaVersion = 4`
- Requisitos: STO-018, STO-001, AST-001
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `internal/store/migrations/00004_pause_interval_open_uidx.sql` (DDL do design; o `Down` remove só o índice)
  - `internal/store/store.go` (`SchemaVersion = 4`)
  - **`sqlc generate`**: conferir que o sqlc aceita o índice parcial; o diff de `internal/store/dbgen` deve ser vazio.
  - testes: `internal/store/migrate4_test.go` (novo) e `internal/store/migrate3_test.go` (ajustar)
- Assinaturas públicas:
  ```go
  const SchemaVersion int64 = 4
  ```
- Testes:
  - `TestMigration4` (`t.TempDir()`; banco v3 sai de goose `UpTo(3)`):
    - "STO-018/banco novo tem pause_interval_open_uidx e goose na versão 4"
    - "STO-018/banco v3 com jogo, sessão e pausa fechada migra para 4 mantendo os dados"
    - "STO-018/segunda pausa aberta da mesma sessão é recusada pelo banco"
    - "STO-018/pausa aberta em outra sessão e nova pausa depois de fechar a anterior são aceitas"
    - "STO-018/Down remove só o índice e mantém a tabela e os dados"
    - "STO-018/SchemaVersion é 4"
  - ajuste em `migrate3_test.go`:
    - os casos "STO-011/…goose na versão 3" e "…migra para 3" passam a comparar com `store.SchemaVersion`
    - o caso "STO-011/SchemaVersion é 3" é substituído por "STO-011/a migração 3 continua embutida" (versão 3 existe no provider)
  - `store_test.go` e `settings_test.go` já usam `store.SchemaVersion` e continuam cobrindo STO-001 e AST-001.
- Depende de: —

## T03 — store: abrir, fechar e listar pausas
- Requisitos: STO-019, STO-020, STO-022
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `internal/store/queries/pauses.sql` (novo): `InsertPauseOpen :exec`, `GetOpenPause :one`, `ClosePause :exec`, `DeletePause :exec`, `ClosePauseAt :execrows` (`paused_at < :ended_at`), `DeleteOpenPause :exec`, `ListPauses :many` (`ORDER BY paused_at, id`)
  - `internal/store/dbgen/*`: regerado com **`sqlc generate`**
  - `internal/store/pauses.go` (novo)
  - testes: `internal/store/pauses_test.go` (novo)
- Assinaturas públicas:
  ```go
  var ErrPauseOpen = errors.New("store: pause already open")
  type PauseStart struct {
      SessionID int64
      At        time.Time
      Reason    domain.PauseReason
      Active    time.Duration
  }
  type PauseEnd struct {
      SessionID int64
      At        time.Time
      State     domain.SessionState // running | grace
      Active    time.Duration
  }
  type PauseInterval struct {
      ID, SessionID       int64
      PausedAt, ResumedAt time.Time // ResumedAt zero = open
      Reason              domain.PauseReason
  }
  func (s *Store) PauseSession(ctx context.Context, p PauseStart) error
  func (s *Store) ResumePause(ctx context.Context, p PauseEnd) error
  func (s *Store) Pauses(ctx context.Context, sessionID int64) ([]PauseInterval, error) // never nil
  ```
- Notas de implementação:
  - `PauseSession` roda em `Store.Tx` e faz `UpdateOpenSession` (0 linhas → `ErrSessionNotOpen`), depois `GetOpenPause` (existe → `ErrPauseOpen`) e `InsertPauseOpen`.
  - A transação usa `_txlock=immediate`, então a pré-checagem é segura e não depende do código de erro do driver.
  - `ResumePause` lê a pausa aberta: se `resumed_at == paused_at` em ms, apaga; senão `ClosePause`. Depois `UpdateOpenSession` com o estado dado.
  - `UpdateSession` já aceita `paused` (é só texto).
- Testes (`t.TempDir()`, jogo e sessão criados por `AddGame`/`StartSession`):
  - `TestPauseSession`:
    - "STO-019/grava intervalo aberto com reason e a sessão com state paused, last_seen_at, active_seconds=floor(a) e ended_at nulo"
    - "STO-019/segunda pausa aberta devolve ErrPauseOpen e nada muda"
    - "STO-019/sessão inexistente devolve ErrSessionNotOpen sem criar linha"
    - "STO-019/sessão já encerrada devolve ErrSessionNotOpen sem criar linha"
    - "STO-019/falha no INSERT desfaz o UPDATE da sessão" (trigger de teste `BEFORE INSERT ON pause_interval … RAISE(ABORT)`)
  - `TestResumePause`:
    - "STO-020/fecha o intervalo em R e grava state running, last_seen_at e active_seconds"
    - "STO-020/state grace é gravado"
    - "STO-020/R igual a P em ms remove o intervalo"
    - "STO-020/sem pausa aberta só atualiza a sessão e não dá erro"
    - "STO-020/sessão encerrada devolve ErrSessionNotOpen"
  - `TestPauses`:
    - "STO-022/ordenadas por paused_at com ResumedAt zero quando aberto"
    - "STO-022/sem pausas devolve lista vazia não nula"
    - "STO-022/reason desconhecido gravado por fora é devolvido como está"
- Depende de: T01, T02

## T04 — store: fim de sessão fecha a pausa aberta; STO-023; recuperação
- Requisitos: STO-021, STO-023, SES-025
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `internal/store/sessions.go` (alterar `EndSession`: depois de `GetOpenSession` e antes de `CloseSession`, na mesma transação, `ClosePauseAt` e `DeleteOpenPause`)
  - testes: `internal/store/sessions_test.go` (alterar), `internal/store/pause_active_test.go` (novo), `internal/tracker/recover_test.go` (alterar; usa `store.Open` real em `t.TempDir()`, pois fakes não enxergam `pause_interval`)
- Assinaturas públicas: nenhuma nova.
- Testes:
  - `TestEndSessionClosesPause`:
    - "STO-021/E > P: o intervalo fica com resumed_at = E"
    - "STO-021/E <= P: o intervalo é removido"
    - "STO-021/descartada: intervalos continuam gravados" (SES-023)
    - "STO-021/falha ao fechar o intervalo desfaz o fim da sessão e do jogo" (trigger `BEFORE UPDATE ON pause_interval … RAISE(ABORT)`; a sessão continua aberta)
    - "STO-021/sem pausa aberta o comportamento de STO-014 e STO-015 não muda"
  - `TestActiveDurationFromPauses`: "STO-023/sessão de 300 s com pausas [100,160] e [200,210] vale 230 e bate com active_seconds" (converte `[]PauseInterval` em `[]domain.Interval`)
  - `TestRecoverPaused` (em `tracker`, store real):
    - "SES-025/pausa aberta em P=T+100s e last_seen L=T+120s vira [P, L], 100 s e motivo recovered"
    - "SES-025/P igual a L: nenhum intervalo"
    - "SES-025/sem notificação nem feedback"
- Depende de: T03

## T05 — ipc: protocolo v0.5.0 e teste de contrato
- Requisitos: IPC-032
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `internal/ipc/protocol.go` (alterar; o comentário do pacote aponta para `docs/specs/phase-4-pause/contracts/ipc.openrpc.json`)
  - testes: `internal/ipc/contract_test.go` (alterar: `contractPath` e `contractRelPath` apontam para a fase 4; os casos "IPC-030/…" são trocados por "IPC-032/…")
- Assinaturas públicas:
  ```go
  const (
      MethodGameLaunchUntracked = "game.launchUntracked"
      MethodSessionPause        = "session.pause"
      MethodSessionResume       = "session.resume"
  )
  const (
      CodeSessionNotPausable = -32005
      MsgSessionNotPausable  = "Session not pausable"
      ReasonNoSession        = "no-session"
      ReasonGrace            = "grace"
  )
  type GameLaunchUntrackedParams struct{ GameID int64 `json:"gameId"` }
  type LaunchedProcess struct{ PID uint32 `json:"pid"` }
  type SessionResult struct{ Session Session `json:"session"` }
  type SessionNotPausableData struct{ Reason string `json:"reason"` }
  ```
- Testes:
  - `TestContract`:
    - "IPC-032/versão 0.5.0"
    - "IPC-032/todos os métodos e notificações do contrato existem com os mesmos nomes, inclusive game.launchUntracked, session.pause e session.resume"
    - "IPC-032/enum Session.state contém running, paused, grace, ended e discarded"
    - "IPC-032/campos obrigatórios de LaunchedProcess, SessionResult e SessionNotPausableData batem com os tipos Go"
    - "IPC-032/códigos e mensagens de erro batem com as constantes, inclusive -32005"
    - "IPC-032/comentário do pacote aponta para phase-4-pause"
- Depende de: —

## T06 — tracker: máquina pura com `Paused` (tracker-specialist)
- Requisitos: SES-016, SES-017, SES-018, SES-019, SES-020, SES-021, SES-022, SES-023, SES-024, SES-026, SES-027, SES-028, SES-029, SES-030, SES-035, SES-036, SES-037, FBK-001
- Responsável: tracker-specialist (código) · Testes: test-writer
- Arquivos:
  - `internal/tracker/machine.go` (alterar)
  - testes: `internal/tracker/machine_test.go` (alterar)
- Assinaturas públicas (o tracker-specialist pode ajustar assinaturas, não o comportamento do contrato):
  ```go
  const StatePaused State // ordem numérica livre
  type FeedbackKind int
  const (
      FeedbackPaused  FeedbackKind = iota + 1 // = 1
      FeedbackResumed                         // = 2
  )
  // novos EffectKind: EffPauseSession, EffResumePause, EffFeedback.
  // Effect ganha Pause domain.PauseReason e Feedback FeedbackKind.
  func (m *Machine) TogglePause(now time.Time, reason domain.PauseReason) []Effect // Running<->Paused; ignora Idle/Launching/Grace e now < startedAt
  func (m *Machine) Pause(now time.Time) []Effect                                   // Running->Paused(ui); Paused idempotente
  func (m *Machine) Resume(now time.Time) []Effect                                  // Paused->Running; Running idempotente
  func (m *Machine) Suspend(now time.Time) []Effect                                 // Running->Paused(sleep), sem feedback
  func (m *Machine) Wake(now time.Time, userPresent bool) []Effect                  // Paused(sleep)+userPresent -> Running
  func (m *Machine) NotPausable() (reason string, ok bool)                          // "no-session" (Idle/Launching) | "grace"; ok=false se pausável
  func (m *Machine) PausedReason() (domain.PauseReason, bool)
  ```
- Alterações de comportamento existentes:
  - `Poll` trata Paused como Running para `ativos == 0`.
  - `Heartbeat` aceita Paused e grava `paused`.
  - `TrackedEmpty` em Paused emite `EffResumePause{State: grace}` + carência + notificação `grace`, sem feedback.
  - `ProcessAdded` em Paused é ignorado.
  - `Stop` em Paused emite `EffEndSession{At: now, Reason: EndAgentShutdown}`.
  - `activeAt` devolve `accumulated` em Paused; `Snapshot` devolve `SessionPaused`.
  - `Machine` guarda `pausedAt` e `pauseReason`.
- Testes (tabela de transições, `time.Time` fixos, sem goroutines):
  - `TestMachinePause`:
    - "SES-016/Running + hotkey em T+100s: Paused, PauseSession(hotkey, active 100 s), notificação paused com activeMs 100000 e asOf T+100s, sem arme de timer"
    - "SES-016/Snapshot em T+130s: paused com activeMs 100000"
    - "SES-017/Paused + gatilho em T+160s: Running, ResumePause(running, active 100 s), notificação running; Snapshot em T+170s dá 110000"
    - "SES-018/Idle, Launching e Grace ignoram TogglePause, Suspend e Wake sem efeitos"
    - "SES-018/gatilho com instante anterior a startedAt é ignorado"
    - "SES-019/cadeia pausa em 100, retoma em 160, pausa em 200, retoma em 210, job vazio em 300, carência vence em 315: End com 230 s, ended_at T+300s"
    - "SES-020/Heartbeat em Paused grava paused com active 100 s e rearma; geração obsoleta é ignorada"
    - "SES-021/TrackedEmpty em Paused: ResumePause(grace, active 100 s), carência armada, notificação grace, sem feedback"
    - "SES-021/carência vence em T+415s: End exited em ended_at T+400s com 100 s"
    - "SES-021/ProcessAdded na carência volta a Running e não reabre a pausa"
    - "SES-022/Poll em Paused com 0 ativos vai a Grace e rearma"
    - "SES-022/Poll em Paused com ativos > 0 ou com erro continua Paused e rearma"
    - "SES-023/pausada em T+20s e carência em T+615s: End discarded com 20 s"
    - "SES-023/29,999 s descartada e 30 s encerrada, com pausas no meio"
    - "SES-024/Stop em Paused: End agent-shutdown com ended_at = now e 100 s, EffCloseJob e sem notificação"
    - "SES-026/Suspend em Running: Paused(sleep), PauseSession, notificação paused e nenhum EffFeedback"
    - "SES-027/Suspend em Paused(hotkey), Paused(sleep) e Grace é ignorado e o motivo é mantido"
    - "SES-028/Wake(userPresent) em Paused(sleep): Running, ResumePause, notificação running e EffFeedback(resumed); active continua 100 s"
    - "SES-029/Wake sem usuário presente em Paused(sleep) é ignorado; depois gatilho retoma"
    - "SES-029/Wake em Running é ignorado"
    - "SES-030/Paused(hotkey) e Paused(gamepad): Suspend e depois Wake(userPresent) mantêm a pausa sem efeitos"
    - "SES-035/Pause em Running: Paused(ui), efeitos de SES-016 e EffFeedback(paused)"
    - "SES-035/Pause em Paused (qualquer motivo) não emite efeitos e mantém o motivo"
    - "SES-036/Resume em Paused (hotkey, gamepad, ui e sleep) volta a Running com EffFeedback(resumed)"
    - "SES-036/Resume em Running é idempotente e não emite efeitos"
    - "SES-037/NotPausable devolve no-session em Idle, grace em Grace e ok=false em Running e Paused; Pause e Resume não emitem efeitos"
  - `TestMachineFeedback` (matriz de FBK-001):
    - "FBK-001/pausa por hotkey, gamepad e ui emite FeedbackPaused"
    - "FBK-001/retomada por gatilho, Resume e Wake com usuário emite FeedbackResumed"
    - "FBK-001/pausa por sleep não emite feedback"
    - "FBK-001/Paused para Grace não emite feedback"
    - "FBK-001/fim de sessão não emite feedback"
    - "FBK-001/gatilho, evento ou pedido ignorado ou idempotente não emite feedback"
- Depende de: T01

## T07 — tracker: coordenador com pausa, suspensão e retomada (tracker-specialist)
- Requisitos: SES-016, SES-017, SES-020, SES-021, SES-022, SES-024, SES-026, SES-028, SES-031, SES-035, SES-036, SES-037, PWR-005, FBK-003
- Responsável: tracker-specialist (código) · Testes: test-writer
- Arquivos:
  - `internal/tracker/tracker.go` (alterar)
  - `internal/tracker/launcher.go` (alterar `SessionStore` com `PauseSession` e `ResumePause`)
  - testes: `internal/tracker/tracker_test.go` e `fakes_test.go` (alterar: `fakeStore` com `opPause` e `opResumePause`; gravação das chamadas de `Feedback`)
- Assinaturas públicas:
  ```go
  type SessionStore interface { // acréscimos
      PauseSession(ctx context.Context, p store.PauseStart) error
      ResumePause(ctx context.Context, p store.PauseEnd) error
  }
  type Config struct { // acréscimo
      Feedback func(FeedbackKind) // must not block; nil = no-op
  }
  type PauseResult int
  const ( PauseIgnored PauseResult = iota; PausePaused; PauseResumed )
  var ErrInvalidPauseReason = errors.New("tracker: invalid pause reason")
  const (
      ReasonNoSession = "no-session"
      ReasonGrace     = "grace"
  )
  type NotPausableError struct{ Reason string }
  func (e *NotPausableError) Error() string
  func (t *Tracker) TogglePause(ctx context.Context, reason domain.PauseReason, at time.Time) (PauseResult, error) // só hotkey/gamepad
  func (t *Tracker) Pause(ctx context.Context, at time.Time) (domain.SessionSnapshot, error)                       // motivo ui
  func (t *Tracker) Resume(ctx context.Context, at time.Time) (domain.SessionSnapshot, error)
  func (t *Tracker) Suspend(ctx context.Context, at time.Time) error
  func (t *Tracker) Wake(ctx context.Context, at time.Time, userPresent bool) error
  ```
- Notas de implementação:
  - `ctrlCh chan ctrlReq` com buffer 16, lido no `select` da goroutine dona ao lado de `launchCh`. Os métodos acima só abandonam o envio se `done` fechar; depois esperam a resposta ou o `ctx` (decisão 1).
  - O instante aplicado é `monotonic(at)`.
  - Depois de `BeginShutdown` ou `Close` devolvem `ErrShuttingDown`.
  - `exec` ganha `EffPauseSession` → `store.PauseSession`, `EffResumePause` → `store.ResumePause` (ambos com `writeTimeout`; falha → `writeFailed`) e `EffFeedback` → `cfg.Feedback(kind)` depois do `Notify`.
  - Pausa e retomada não rearmam heartbeat nem verificação.
  - `Pause` e `Resume` devolvem o snapshot medido no instante aplicado.
  - Gatilho ignorado loga Debug "pausa ignorada" com o estado.
- Testes (`FakeClock`, `BlockUntilWaiters(2)` em Running e Paused, `goleak` no `Close`):
  - `TestTrackerPause`:
    - "SES-016/hotkey em T+100s grava PauseSession(hotkey, 100 s), notifica paused e Current em T+130s mostra paused com activeMs 100000 e asOf T+130s"
    - "SES-016/nenhum processo é terminado: Job.Terminate e Job.Close não são chamados e só ActiveProcesses é consultado"
    - "SES-017/segundo gatilho em T+160s grava ResumePause(running) e notifica running; Current em T+170s dá 110000"
    - "SES-018/TogglePause sem sessão ou em Grace devolve PauseIgnored, sem gravação, sem notificação e sem feedback"
    - "SES-018/instante anterior ao startedAt devolve PauseIgnored"
    - "SES-020/heartbeats em paused gravam last_seen_at T+120s e T+150s com active 100 s; avançar 30 s − 1 ns não grava"
    - "SES-021/job vazio em Paused: ResumePause(grace), carência armada, notificação grace; carência vence e a sessão termina exited com 100 s"
    - "SES-022/verificação periódica em Paused com 0 processos leva a Grace; com erro registra Warn e continua Paused"
    - "SES-024/Close em Paused encerra com agent-shutdown em T+500s, 100 s, sem notificação, sem Job.Terminate; Close é idempotente e espera a goroutine dona"
    - "SES-026/Suspend em Running grava sleep, notifica paused e não chama Feedback"
    - "SES-028/Wake com usuário presente em T+28900s volta a Running, notifica com asOf T+28900s, chama Feedback(resumed); Current em T+28910s dá 110000"
    - "SES-031/Launch em Paused devolve SessionActiveError com sessionId, gameId e title, sem Prepare"
    - "SES-035/Pause em Running grava ui e chama Feedback(paused); em Paused (hotkey ou sleep) devolve a sessão atual sem gravar, notificar nem dar feedback"
    - "SES-036/Resume em Paused (qualquer motivo) volta a Running com Feedback(resumed); em Running é idempotente"
    - "SES-037/Pause e Resume sem sessão devolvem *NotPausableError{no-session}; em Grace devolvem {grace}; nada é gravado"
    - "SES-016/TogglePause com motivo sleep ou ui devolve ErrInvalidPauseReason"
    - "SES-016/falha em PauseSession loga Error, o estado em memória avança e a notificação sai (SES-014)"
    - "SES-016/depois de BeginShutdown os métodos devolvem ErrShuttingDown"
  - `TestTrackerControlOrdering`:
    - "PWR-005/Suspend com ctx já expirado e a goroutine dona ocupada em uma gravação lenta ainda é aplicado com o instante original"
    - "PWR-005/a retomada que chega depois não toma o instante da suspensão"
  - `TestTrackerFeedbackNonBlocking`: "FBK-003/Feedback bloqueante não atrasa as notificações nem as gravações" (o callback bloqueia num canal liberado no `t.Cleanup`)
- Depende de: T03, T06

## T08 — tracker: lançamento sem rastrear e integração Windows (tracker-specialist)
- Requisitos: SES-031, SES-032, SES-033, SES-034, TRK-009, TRK-010, TRK-011
- Responsável: tracker-specialist (código) · Testes: test-writer
- Arquivos:
  - `internal/tracker/launcher.go` (`Launcher.LaunchDetached`)
  - `internal/tracker/tracker.go` (`LaunchUntracked`, tipo de pedido no `ctrlCh`)
  - `internal/tracker/job_windows.go` (`winLauncher.LaunchDetached`)
  - `internal/tracker/job_other.go` (`unsupportedLauncher.LaunchDetached` → `*LaunchError` com `errors.ErrUnsupported`)
  - `cmd/fake-game/`: sem mudança (o contrato da fase 3 serve)
  - testes: `internal/tracker/tracker_test.go`, `fakes_test.go` (`fakeLauncher.LaunchDetached`), `integration_windows_test.go`, `job_windows_test.go`
- Assinaturas públicas:
  ```go
  type Launcher interface {
      Prepare(spec LaunchSpec) (Job, error)
      // CreateProcess sem Job Object e sem CREATE_SUSPENDED; fecha hProcess e hThread antes de retornar.
      LaunchDetached(spec LaunchSpec) (pid uint32, err error)
  }
  func (t *Tracker) LaunchUntracked(ctx context.Context, spec LaunchSpec) (pid uint32, err error)
  ```
- Notas de implementação:
  - `LaunchUntracked` tem refusal rápida pelo `view` publicado (como `Launch`) e checagem final na goroutine dona.
  - Não toca na máquina, no job nem no store.
  - Falha loga `Error` "falha ao iniciar o jogo" (`game_id`, `path`, `error`) e devolve `*LaunchError`.
  - `LaunchDetached` no Windows reutiliza as regras de TRK-005 (`quoteExe`, `launch_args`, `cwd`, `CREATE_DEFAULT_ERROR_MODE`, `inherit=false`) e faz `defer windows.CloseHandle` em `pi.Process` e `pi.Thread` logo depois do erro checado.
- Testes:
  - `TestTrackerLaunchUntracked` (qualquer SO, falsos):
    - "SES-032/cria o processo com o mesmo LaunchSpec, devolve o PID, sem StartSession, sem Notify e sem job"
    - "SES-032/Current continua sem sessão e um Launch rastreado logo depois é aceito; total do jogo não muda"
    - "SES-033/com Running, Grace, Paused ou lançamento em andamento devolve SessionActiveError e não cria processo"
    - "SES-033/depois de BeginShutdown devolve ErrShuttingDown sem criar processo"
    - "SES-034/falha do CreateProcess loga Error com game_id, path e error, devolve *LaunchError com WinError e não cria sessão nem notificação"
    - "SES-031/LaunchUntracked em Paused devolve SessionActiveError"
  - `TestIntegrationPause` (Windows, `fake-game --depth 1 --parent-run forever --run forever --until-file F`, `FakeClock`, espera real limitada):
    - "TRK-009/pausa e retomada mantêm os dois processos vivos e dentro do job (IsProcessInJob)"
    - "TRK-009/F criado durante a pausa e processos saindo: a sessão vai a Grace e termina sem perder evento do job"
  - `TestIntegrationLaunchUntracked` (Windows, `fake-game --run forever --until-file F --report R`):
    - "TRK-010/processo roda com cwd = pasta de trabalho e os argumentos intactos"
    - "TRK-010/nenhum job é criado: IsProcessInJob(filho) é igual ao do processo de teste, e se o teste já estiver num job, o filho não está num job diferente do pai"
    - "TRK-010/nenhuma linha de play_session e o processo continua vivo depois de Tracker.Close; termina quando F existe"
  - `TestLaunchDetachedHandles`: "TRK-011/20 lançamentos de fake-game --run 0 e um Close deixam GetProcessHandleCount <= h0 + 2"
- Depende de: T07

## T09 — input: gatilho, detector de combo e poller (portável)
- Requisitos: PAD-001, PAD-002, PAD-003, PAD-004, PAD-005, PAD-006, PAD-008
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `internal/input/input.go` (novo)
  - `internal/input/combo.go` (novo)
  - `internal/input/pad.go` (novo)
  - testes: `internal/input/combo_test.go` e `internal/input/pad_test.go` (novos)
- Assinaturas públicas:
  ```go
  package input
  type Trigger struct {
      Source domain.PauseReason // PauseHotkey | PauseGamepad
      At     time.Time
  }
  type Source interface {
      Name() string
      Start(out chan<- Trigger) error // erro = indisponível; nada fica rodando
      Close() error                   // idempotente; espera a goroutine
  }
  const ( ModAlt = 0x1; ModControl = 0x2; ModShift = 0x4; ModNoRepeat = 0x4000 )
  type Hotkey struct{ Modifiers uint32; VK uint32 }
  var DefaultHotkey = Hotkey{Modifiers: ModControl | ModAlt, VK: 'P'}
  func (h Hotkey) String() string // "Ctrl+Alt+P"

  var ErrDisconnected = errors.New("input: controller disconnected")
  const ( ButtonBack uint16 = 0x0020; ButtonY uint16 = 0x8000 )
  type PadState struct{ Buttons uint16 }
  type PadAPI interface{ State(slot int) (PadState, error) } // erro que embrulha ErrDisconnected
  type PadConfig struct{ Hold, Poll, Rescan time.Duration; Mask uint16 } // zero => 1 s, 16 ms, 1 s, Back|Y
  func NewPad(api PadAPI, cfg PadConfig, clock domain.Clock, log *slog.Logger) Source

  type comboDetector struct{ since [4]time.Time; fired [4]bool } // interno
  func (d *comboDetector) Observe(slot int, buttons uint16, connected bool, now time.Time) (fire bool)
  ```
- Notas de implementação:
  - `Pad`: uma goroutine dona (`wg`); laço `select { <-stop; now := <-clock.After(Poll) }`.
  - Ao iniciar lê os 4 slots; depois lê os conectados a cada `Poll` e os desconectados a cada `Rescan`, medido por `lastProbe`.
  - `ERROR_DEVICE_NOT_CONNECTED` marca o slot desconectado e zera a contagem.
  - Envio ao `out` é não bloqueante; cheio → `Warn`.
  - Sem `time.Sleep` nem `Ticker`.
- Testes (`FakeClock` + `BlockUntilWaiters(1)` + `Advance(16ms)`, `PadAPI` falso, `goleak`):
  - `TestComboDetector`:
    - "PAD-001/Back e Y juntos por 1 s disparam uma vez (T+992 ms não dispara, T+1008 ms dispara)"
    - "PAD-001/outros botões pressionados junto não impedem nem atrasam"
    - "PAD-002/soltar Y aos 900 ms cancela e a nova pressão recomeça a contagem"
    - "PAD-003/10 s a mais pressionado não dispara de novo; soltar e pressionar de novo por 1 s dispara"
    - "PAD-004/Back só no slot 0 e Y só no slot 1 não disparam"
    - "PAD-004/dois slots que completam juntos disparam um gatilho cada"
    - "PAD-005/desconexão aos 600 ms zera a contagem e a reconexão pede mais 1 s inteiro"
  - `TestPad`:
    - "PAD-001/62 avanços de 16 ms não entregam Trigger e o 63º entrega um com Source gamepad e At = instante lido"
    - "PAD-006/ao iniciar cada slot é lido uma vez; após 62 avanços o conectado tem 63 leituras e os desconectados 1; no 63º avanço cada desconectado é sondado de novo (2)"
    - "PAD-005/ERROR_DEVICE_NOT_CONNECTED no meio da pressão zera a contagem"
    - "PAD-008/Close sem avançar o relógio retorna, a goroutine termina (goleak) e a segunda chamada não tem efeito"
    - "PAD-001/fila de saída cheia descarta o gatilho com Warn sem bloquear"
- Depende de: T01

## T10 — input: atalho global `RegisterHotKey` (Windows)
- Requisitos: HOT-001, HOT-002, HOT-003, HOT-004
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `internal/input/hotkey_windows.go` (novo)
  - `internal/input/hotkey_other.go` (novo; `NewHotkey` devolve uma `Source` cujo `Start` falha com `errors.ErrUnsupported`)
  - testes: `internal/input/hotkey_windows_test.go` (novo, pacote `input` para alcançar `loopThreadID` e `registerModifiers`)
- Assinaturas públicas:
  ```go
  func NewHotkey(h Hotkey, clock domain.Clock, log *slog.Logger) Source
  // internos
  func registerModifiers(h Hotkey) uint32 // h.Modifiers | ModNoRepeat
  func (s *hotkey) threadID() uint32
  ```
- Notas de implementação:
  - `Start` cria a goroutine `loop`: `runtime.LockOSThread`, `PeekMessageW(PM_NOREMOVE)`, `RegisterHotKey(0, id=1, registerModifiers(h), vk)` e responde em `ready` com `windows.GetCurrentThreadId()` ou com o erro que embrulha o `Errno`.
  - Laço `GetMessageW`: `WM_HOTKEY` com `wParam == id` lê `clock.Now()` e envia não bloqueante em `out`; cheio → `Warn` "gatilho de pausa descartado: fila cheia".
  - `Close`: `PostThreadMessageW(tid, WM_QUIT)` + `wg.Wait`; a própria goroutine chama `UnregisterHotKey` antes de sair; `sync.Once`.
  - O `Start` com erro loga `Error` "atalho de pausa indisponível" (`hotkey`, `winError`).
  - Info "atalho de pausa registrado" no sucesso.
- Testes (combinação `testHotkey = {ModControl|ModAlt|ModShift, 0x87}`; `WM_HOTKEY` injetado com `PostThreadMessageW(tid, WM_HOTKEY, 1, lParam)`; `FakeClock`; `goleak`). Nenhum precisa de desktop, exceto o caso de `SendInput`.
  - `TestHotkey`:
    - "HOT-001/DefaultHotkey é Ctrl+Alt+P com VK 0x50 e registerModifiers inclui MOD_NOREPEAT"
    - "HOT-001/Start registra a combinação no processo: um segundo listener com a mesma combinação falha"
    - "HOT-001/Info registrado com o atalho em texto"
    - "HOT-001/SendInput de Ctrl+Alt+Shift+F24 entrega um gatilho" (pulado sem `requireInteractiveDesktop`)
    - "HOT-002/WM_HOTKEY com o id entrega exatamente um Trigger{hotkey, clock.Now() do recebimento}"
    - "HOT-002/WM_HOTKEY com outro id não gera gatilho"
    - "HOT-002/fila cheia descarta com Warn e o loop continua respondendo"
    - "HOT-003/combinação já registrada devolve erro que embrulha ERROR_HOTKEY_ALREADY_REGISTERED (1409), nenhuma goroutine fica viva e o Error é logado com hotkey e winError"
    - "HOT-004/Close desregistra, a goroutine termina antes de Close retornar (goleak) e outro listener registra a mesma combinação"
    - "HOT-004/segunda chamada a Close não tem efeito"
- Depende de: T09

## T11 — input: XInput real (Windows)
- Requisitos: PAD-007
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `internal/input/xinput_windows.go` (novo)
  - testes: `internal/input/xinput_windows_test.go` (novo, pacote `input`)
- Assinaturas públicas:
  ```go
  func NewXInput() (PadAPI, error)               // carrega xinput1_4.dll
  func loadXInput(dll string) (PadAPI, error)    // interno: testável com um nome inexistente
  ```
- Notas de implementação:
  - `windows.NewLazySystemDLL` + `Load()`; erro ao carregar → erro.
  - `XInputGetState(slot, *xinputState)`; `ERROR_DEVICE_NOT_CONNECTED` (1167) → erro que embrulha `ErrDisconnected`.
  - Sem handle aberto.
- Testes:
  - `TestXInput`:
    - "PAD-007/DLL inexistente devolve erro e nenhum PadAPI"
    - "PAD-007/NewXInput real carrega e State(0) devolve estado ou ErrDisconnected, sem pânico"
    - "PAD-007/State com slot fora de 0..3 devolve erro"
- Depende de: T09

## T12 — winapi: `wndProc` compartilhado por mapa de handlers
- Requisitos: PWR-006 (infraestrutura; a regressão do `SessionEndWatcher` é coberta pelos testes EXE-* existentes de `sessionend_windows_test.go`)
- Responsável: implementer · Testes: test-writer (somente se faltar caso)
- Arquivos:
  - `internal/winapi/wndproc_windows.go` (novo)
  - `internal/winapi/sessionend_windows.go` (alterar para registrar um `windowHandler`)
  - `internal/winapi/user32_windows.go` (declarações novas usadas por T13 e T16: `SendMessageW`, `GetGuiResources`, `PeekMessageW`, `RegisterHotKey` não, `GetForegroundWindow`, `MonitorFromWindow`, `GetMonitorInfoW`, `SetWindowPos`, `SetLayeredWindowAttributes`, `ShowWindow`, `InvalidateRect`, `BeginPaint`, `EndPaint`, `GetWindowLongPtrW`, `SetThreadDpiAwarenessContext`, `GetDpiForWindow`)
  - `internal/winapi/gdi_windows.go` (novo: `CreateFontW`, `CreateSolidBrush`, `FillRect`, `DrawTextW`, `SetBkMode`, `SetTextColor`, `SelectObject`, `DeleteObject`; `shcore.GetDpiForMonitor`)
  - testes: os de `sessionend_windows_test.go` continuam passando
- Assinaturas internas:
  ```go
  type windowHandler func(hwnd, msg, wParam, lParam uintptr) (ret uintptr, handled bool)
  func registerWindowHandler(hwnd windows.HWND, h windowHandler)
  func unregisterWindowHandler(hwnd windows.HWND)
  func sharedWndProc() uintptr // um único windows.NewCallback
  ```
- Testes: nenhum novo. O `go test ./internal/winapi` existente (EXE-016, AGT-006: WM_QUERYENDSESSION, WM_ENDSESSION, WM_CLOSE externo ignorado, 20 ciclos sem vazar) deve continuar verde sem alteração.
- Depende de: —

## T13 — winapi: `PowerWatcher` (suspensão e hibernação)
- Requisitos: PWR-001, PWR-002, PWR-003, PWR-004, PWR-006
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `internal/winapi/power_windows.go` (novo)
  - testes: `internal/winapi/power_windows_test.go` (novo, pacote `winapi`)
- Assinaturas públicas:
  ```go
  type PowerCallbacks struct {
      OnSuspend func(at time.Time)
      OnWake    func(at time.Time, userPresent bool)
  }
  func NewPowerWatcher(clock domain.Clock, log *slog.Logger, cb PowerCallbacks) *PowerWatcher
  func (w *PowerWatcher) Start() error // cria a janela e o laço; erro não deixa nada rodando
  func (w *PowerWatcher) Close() error // idempotente; espera o laço
  func (w *PowerWatcher) windowHandle() windows.HWND
  ```
- Notas de implementação:
  - Mesmo padrão do `SessionEndWatcher`: janela oculta de topo (estilo 0, não `HWND_MESSAGE`), `LockOSThread`, classe e janela criadas e destruídas na thread do laço, parada por `WM_APP+1`, `postMu` e `exited`.
  - O handler lê `clock.Now()` dentro do procedimento. Devolve `1` (TRUE) para `WM_POWERBROADCAST` em todos os casos; os demais eventos de energia são ignorados.
- Testes (`SendMessageW(hwnd, WM_POWERBROADCAST, wParam, 0)`; `FakeClock`; `goleak`; sem desktop interativo):
  - `TestPowerWatcher`:
    - "PWR-001/PBT_APMSUSPEND chama OnSuspend uma vez com clock.Now() do recebimento e o procedimento devolve TRUE"
    - "PWR-002/PBT_APMRESUMESUSPEND chama OnWake(at, true) uma vez"
    - "PWR-003/PBT_APMRESUMEAUTOMATIC chama OnWake(at, false) uma vez"
    - "PWR-004/POWERSTATUSCHANGE, BATTERYLOW, RESUMECRITICAL e POWERSETTINGCHANGE não chamam callback e devolvem TRUE"
    - "PWR-006/Close termina o laço, destrói janela e classe antes de retornar (goleak) e a segunda chamada não tem efeito"
    - "PWR-006/20 ciclos de Start/Close: GetProcessHandleCount <= h0 + 2 e GetGuiResources não cresce mais que 2"
    - "PWR-006/Start com erro injetado (classe duplicada) não deixa goroutine viva"
- Depende de: T12

## T14 — feedback: coordenador portátil
- Requisitos: OVL-001, OVL-003, FBK-002, FBK-004, FBK-005, FBK-003
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `internal/feedback/feedback.go` (novo)
  - testes: `internal/feedback/feedback_test.go` (novo)
- Assinaturas públicas:
  ```go
  package feedback
  type Notice int
  const ( NoticePaused Notice = iota + 1; NoticeResumed )
  func (n Notice) Text() string // "Tempo pausado" | "Tempo retomado"
  type Cue int
  const ( CuePaused Cue = iota + 1; CueResumed )
  type Overlay interface { Show(Notice); Hide(); Close() error }
  type Sound interface { Play(Cue); Close() error }
  type NopOverlay struct{}
  type NopSound struct{}
  type Event int
  const ( EventPaused Event = iota + 1; EventResumed ) // mesmos valores de tracker.FeedbackKind
  const OverlayDuration = 2500 * time.Millisecond
  const queueSize = 4
  type Coordinator struct{ /* ... */ }
  func New(o Overlay, s Sound, clock domain.Clock, log *slog.Logger) *Coordinator // nil => Nop
  func (c *Coordinator) Start()
  func (c *Coordinator) Send(e Event) // não bloqueia; fila cheia descarta o mais antigo + Warn
  func (c *Coordinator) Close() error // esconde, fecha Overlay e Sound, espera a goroutine; idempotente
  ```
- Notas de implementação:
  - Uma goroutine dona lê `events` (cap 4) e o temporizador `hide` (`clock.After(OverlayDuration)` com geração; evento novo reinicia e invalida o anterior).
  - Por evento chama `overlay.Show` e `sound.Play`; um não impede o outro.
- Testes (`FakeClock`, falsos que gravam chamadas, `goleak`):
  - `TestNotice`: "OVL-001/NoticePaused é Tempo pausado e NoticeResumed é Tempo retomado"
  - `TestCoordinator`:
    - "FBK-002/EventPaused chama Show(NoticePaused) e Play(CuePaused) uma vez cada; EventResumed os de retomada"
    - "FBK-002/Overlay ausente (nil) ainda toca o som e vice-versa; nada é propagado"
    - "FBK-002/um dos dois com Close que devolve erro é logado em Warn e o outro é fechado mesmo assim"
    - "OVL-003/Hide chega em t + 2,5 s e não em t + 2,5 s − 1 ns"
    - "OVL-003/segundo evento em t + 1 s troca o texto, não esconde em t + 2,5 s e esconde em t + 3,5 s uma única vez"
    - "FBK-004/Paused seguido de Resumed 100 ms depois são processados em ordem e o último texto é Tempo retomado"
    - "FBK-003/Show que bloqueia não bloqueia Send: a fila cap 4 cheia descarta o mais antigo com um Warn"
    - "FBK-005/Close esconde o aviso, fecha Overlay e Sound e espera a goroutine (goleak); segunda chamada sem efeito"
    - "FBK-005/Send depois do Close é descartado sem pânico"
- Depende de: —

## T15 — winapi: geometria do aviso
- Requisitos: OVL-004
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `internal/winapi/overlay_geometry.go` (novo, sem build tag)
  - testes: `internal/winapi/overlay_geometry_test.go` (novo, sem build tag)
- Assinaturas internas:
  ```go
  type Rect struct{ Left, Top, Right, Bottom int32 }
  func (r Rect) Width() int32
  func overlaySize(dpi int) (w, h int)             // 240x56 a 96 dpi, escalado por dpi/96
  func overlayRect(work Rect, dpi, w, h int) Rect  // x = work.Left + (work.Width-w)/2 ; y = work.Top + 48*dpi/96
  ```
- Testes:
  - `TestOverlayRect`:
    - "OVL-004/área (0,0,1920,1040) 240x56 a 96 dpi: canto (840,48)"
    - "OVL-004/área (1920,0,3840,1040): canto (2760,48)"
    - "OVL-004/144 dpi 360x84 na primeira área: canto (780,72)"
    - "OVL-004/overlaySize escala por dpi/96"
- Depende de: —

## T16 — winapi: janela do aviso (`Overlay`)
- Requisitos: OVL-002, OVL-005, OVL-006
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `internal/winapi/overlay_windows.go` (novo)
  - testes: `internal/winapi/overlay_windows_test.go` (novo, pacote `winapi`)
- Assinaturas públicas:
  ```go
  type Overlay struct{ /* ... */ } // implementa feedback.Overlay
  func NewOverlay(log *slog.Logger) *Overlay
  func (o *Overlay) Start() error
  func (o *Overlay) Show(n feedback.Notice) // posta WM_APP+2 com o texto guardado sob mu; nunca bloqueia
  func (o *Overlay) Hide()                  // posta WM_APP+3
  func (o *Overlay) Close() error           // posta WM_APP+1; espera a goroutine; idempotente
  func (o *Overlay) windowHandle() windows.HWND
  ```
- Notas de implementação:
  - Goroutine `loop` com `LockOSThread` cria classe `ArcadiaNotice-N`, janela `WS_POPUP` com `WS_EX_TOPMOST|WS_EX_NOACTIVATE|WS_EX_TOOLWINDOW|WS_EX_LAYERED|WS_EX_TRANSPARENT`, alfa 230, fonte "Segoe UI" 20 px × dpi/96 e pincel. Handler registrado em `registerWindowHandler`.
  - `WM_PAINT`: `FillRect` RGB(24,24,27) + `DrawTextW` branco centralizado.
  - Mostrar: `SetWindowPos(HWND_TOPMOST, x, y, w, h, SWP_NOACTIVATE|SWP_SHOWWINDOW)`, posição via `overlayRect` e `MonitorFromWindow(GetForegroundWindow(), MONITOR_DEFAULTTOPRIMARY)` + `GetMonitorInfoW(rcWork)` + `GetDpiForMonitor`. **Nunca** `SetForegroundWindow` nem `ShowWindow(SW_SHOW)`.
  - O DPI awareness per-monitor V2 vale só para a thread (`SetThreadDpiAwarenessContext`).
  - Ao sair, o `loop` destrói a janela, desregistra a classe e libera fonte e pincel (`DeleteObject`).
  - O tempo de exibição é do `feedback.Coordinator` (a janela é burra).
- Testes:
  - `TestOverlayWindow`:
    - "OVL-002/estilos WS_POPUP e estendidos TOPMOST, NOACTIVATE, TOOLWINDOW, LAYERED e TRANSPARENT; invisível antes do primeiro Show" (`GetWindowLongPtr`)
    - "OVL-002/Show deixa a janela visível e a janela em primeiro plano e a do foco continuam as mesmas" (pulado sem `requireInteractiveDesktop`)
    - "OVL-002/Hide esconde a janela"
    - "OVL-005/Close esconde e destrói a janela, desregistra a classe e espera a goroutine (goleak)"
    - "OVL-005/Show depois do Close não tem efeito nem erro"
    - "OVL-005/segunda chamada a Close não tem efeito"
    - "OVL-005/100 ciclos de Show/Hide: GetGuiResources(USEROBJECTS e GDIOBJECTS) cresce <= 2 e handles <= h0 + 2"
    - "OVL-006/Start com falha injetada (classe já registrada) devolve erro, nada fica rodando e Show/Hide/Close seguintes são no-op sem erro"
- Depende de: T12, T14, T15

## T17 — winapi: som (`Beeper`)
- Requisitos: SND-001, SND-002
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `internal/winapi/beep_windows.go` (novo)
  - testes: `internal/winapi/beep_windows_test.go` (novo, pacote `winapi`)
- Assinaturas públicas:
  ```go
  type Beeper struct{ /* ... */ } // implementa feedback.Sound
  func NewBeeper(log *slog.Logger) *Beeper                                    // beep = kernel32.Beep
  func newBeeper(log *slog.Logger, beep func(freq, ms uint32) error) *Beeper // testes injetam
  func (b *Beeper) Play(c feedback.Cue) // não bloqueia; substitui o pendente
  func (b *Beeper) Close() error        // fecha stop e espera; idempotente
  ```
- Notas de implementação: uma goroutine dona recebe de um canal de capacidade 1 e chama `beep` em sequência. Pausa: 660 Hz/90 ms → 440 Hz/120 ms. Retomada: 440 Hz/90 ms → 660 Hz/120 ms. Erro do `beep` loga `Warn`.
- Testes (`beep` falso que grava e pode bloquear num canal; `goleak`):
  - `TestBeeper`:
    - "SND-001/CuePaused toca 660 Hz por 90 ms e depois 440 Hz por 120 ms"
    - "SND-001/CueResumed toca 440 Hz por 90 ms e depois 660 Hz por 120 ms"
    - "SND-001/Play volta imediatamente com beep bloqueado"
    - "SND-002/Close termina a goroutine antes de retornar (goleak) e a segunda chamada não tem efeito"
    - "SND-002/Play depois do Close não tem efeito"
    - "SND-002/Play durante um sinal em curso deixa no máximo um pendente: o mais novo substitui o pendente"
    - "SND-002/Close com beep bloqueado ainda retorna depois de liberá-lo no cleanup"
- Depende de: T14

## T18 — agente: handlers `game.launchUntracked`, `session.pause` e `session.resume`
- Requisitos: IPC-033, IPC-034, IPC-035, IPC-036, IPC-038, IPC-039, IPC-040, SES-035, SES-036, SES-037
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `internal/agent/games.go` (alterar)
  - testes: `internal/agent/games_test.go` (alterar; o falso de `sessionTracker` ganha `Pause`, `Resume` e `LaunchUntracked`)
- Assinaturas públicas (internas ao pacote):
  ```go
  type sessionTracker interface { // acréscimos
      LaunchUntracked(ctx context.Context, spec tracker.LaunchSpec) (uint32, error)
      Pause(ctx context.Context, at time.Time) (domain.SessionSnapshot, error)
      Resume(ctx context.Context, at time.Time) (domain.SessionSnapshot, error)
  }
  const pauseHandlerTimeout = 5 * time.Second
  func prepareLaunch(ctx context.Context, st gameStore, p gameLaunchParams, log *slog.Logger) (store.Game, tracker.LaunchSpec, error) // comum a game.launch e game.launchUntracked
  // gameHandlers passa a registrar também ipc.MethodGameLaunchUntracked, ipc.MethodSessionPause e ipc.MethodSessionResume
  ```
- Notas de implementação:
  - `game.launchUntracked`: mesma validação de `game.launch` (params nulos → `-32602`, `parseGameID`, jogo inexistente → `-32001`, `launch_type != exe` → `-32003`), prazo de 10 s, mapeamento de erros por `launchError`. Resultado `ipc.LaunchedProcess{PID}`.
  - `session.pause` e `session.resume`: `ipc.NoParams`, `at = clock.Now()` lido ao receber, prazo de 5 s, resultado `ipc.SessionResult{Session: SessionFromSnapshot(snap)}`.
  - `*tracker.NotPausableError` → `-32005` com `SessionNotPausableData`; `ErrShuttingDown` loga Debug e vira `-32603`; fim do `ctx` loga Error e vira `-32603`.
  - Esses dois não passam pelo roteador de gatilhos (PAU-008).
- Testes (`ipc.Dispatch` com falsos):
  - `TestHandleGameLaunchUntracked`:
    - "IPC-033/jogo existente e sem sessão devolve {pid: N >= 1} e session.current continua {session: null}"
    - "IPC-033/nenhuma notificação session.changed é enviada"
    - "IPC-034/gameId inexistente devolve -32001 antes de consultar a sessão"
    - "IPC-034/sessão Running, Grace, Paused ou lançamento em andamento devolve -32002 com sessionId, gameId e title"
    - "IPC-034/falha do lançamento devolve -32003 com data.error e winError"
    - "IPC-034/agente em shutdown devolve -32003 com error agente encerrando"
    - "IPC-034/launch_type diferente de exe devolve -32003 tipo de lançamento não suportado"
    - "IPC-034/prazo de 10 s esgotado devolve -32603"
    - "IPC-035/gameId texto, fracionário, < 1, array posicional, ausente ou membro desconhecido devolve -32602"
    - "IPC-035/params vazios ({}, [], null, ausente) devolvem -32602"
  - `TestHandleSessionCurrentPaused`: "IPC-036/sessão pausada devolve state paused, activeMs 100000, activeSeconds 100 e asOf do momento da chamada; sem sessão continua {session: null}"
  - `TestHandleSessionPause`:
    - "IPC-038/session.pause em Running devolve {session} paused com activeMs congelado e asOf do pedido"
    - "IPC-038/em Paused devolve o estado atual como sucesso"
    - "IPC-038/params não vazios devolvem -32602 e params vazios ({}, [], null, ausente) são aceitos"
    - "SES-035/o handler usa clock.Now() lido ao receber como at"
  - `TestHandleSessionResume`:
    - "IPC-039/session.resume em Paused devolve {session} running"
    - "IPC-039/em Running devolve o estado atual como sucesso"
    - "IPC-039/mesma regra de params de IPC-038"
    - "SES-036/o handler usa clock.Now() lido ao receber como at"
  - `TestHandleSessionNotPausable`:
    - "IPC-040/sem sessão devolve -32005 com data.reason no-session (pause e resume)"
    - "IPC-040/em Grace devolve -32005 com data.reason grace"
    - "SES-037/o erro não grava nem notifica"
    - "IPC-040/ErrShuttingDown devolve -32603 e prazo de 5 s esgotado devolve -32603"
- Depende de: T05, T07, T08

## T19 — agente: roteador de gatilhos e ponte de energia
- Requisitos: PAU-001, PAU-002, PAU-003, PAU-004, PAU-005, PAU-008, PWR-005
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `internal/agent/pause.go` (novo)
  - testes: `internal/agent/pause_test.go` (novo)
- Assinaturas públicas (internas ao pacote):
  ```go
  type pauseTracker interface {
      TogglePause(ctx context.Context, reason domain.PauseReason, at time.Time) (tracker.PauseResult, error)
      Suspend(ctx context.Context, at time.Time) error
      Wake(ctx context.Context, at time.Time, userPresent bool) error
  }
  const (
      triggerQueueSize   = 8
      toggleTimeout      = time.Second
      PowerHandlerTimeout = 1500 * time.Millisecond
  )
  type pauseRouter struct{ /* ... */ }
  func newPauseRouter(tr pauseTracker, in <-chan input.Trigger, log *slog.Logger) *pauseRouter
  func (r *pauseRouter) Start()
  func (r *pauseRouter) Close() // fecha stop e espera a goroutine; idempotente
  type powerBridge struct {
      tr      pauseTracker
      timeout time.Duration // PowerHandlerTimeout; testes encurtam
      log     *slog.Logger
  }
  func (b *powerBridge) OnSuspend(at time.Time)
  func (b *powerBridge) OnWake(at time.Time, userPresent bool)
  ```
- Notas de implementação: o antirrepique mede só `Trigger.At` (nunca `time.Now()`), a partir do último gatilho encaminhado. `PauseIgnored` loga Debug "gatilho de pausa ignorado"; `ErrShuttingDown` loga Debug; outro erro loga Warn. `powerBridge.OnSuspend` usa `context.WithTimeout` do relógio real (limite de espera de E/S, como os `writeTimeout`) e loga Warn "tracker não respondeu à suspensão a tempo".
- Testes (`pauseTracker` falso, `goleak`):
  - `TestPauseRouter`:
    - "PAU-001/Trigger hotkey chama TogglePause com motivo hotkey e At do gatilho"
    - "PAU-002/Trigger gamepad chama TogglePause com motivo gamepad"
    - "PAU-003/PauseIgnored loga Debug gatilho de pausa ignorado e nada mais acontece"
    - "PAU-004/gatilhos com At separados por 299 ms: só o primeiro é encaminhado (mesma fonte e fontes diferentes)"
    - "PAU-004/separados por exatamente 300 ms o segundo também é encaminhado"
    - "PAU-004/a janela é medida a partir do último encaminhado: At 0, 200 (descartado) e 300 são encaminhados 0 e 300"
    - "PAU-005/ErrShuttingDown é descartado com Debug, sem pânico"
    - "PAU-005/Close espera a goroutine (goleak) e é idempotente"
  - `TestPauseRouterUIRequests`: "PAU-008/pedido da UI (session.pause) 100 ms depois de um gatilho é aplicado, não atualiza o último encaminhado, e um gatilho 100 ms depois do pedido da UI é encaminhado" (o fake serve de `sessionTracker` e de `pauseTracker`; os pedidos da UI vão pelo handler de T18)
  - `TestPowerBridge`:
    - "PWR-005/OnSuspend repassa Suspend com o At do evento"
    - "PWR-005/tracker lento: OnSuspend volta em no máximo o timeout e loga Warn tracker não respondeu à suspensão a tempo"
    - "PWR-005/OnWake repassa userPresent e At; erro loga Debug"
- Depende de: T07, T09, T18

## T20 — agente: subida e shutdown com as fontes, o feedback e a energia
- Requisitos: AGT-012, AGT-013, PAU-006, IPC-037, FBK-003, OVL-006
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `internal/agent/app.go` (alterar)
  - `internal/agent/components_windows.go` (novo: `realPlatform`)
  - `internal/agent/components_other.go` (novo: `realPlatform` devolve `platformParts{}`)
  - `internal/agent/run_windows.go` (alterar: `cfg.Platform = realPlatform`)
  - testes: `internal/agent/app_test.go`, `internal/agent/shutdown_test.go`, `internal/agent/tracker_wiring_test.go`, `internal/agent/components_test.go` (novos casos)
- Assinaturas públicas (internas ao pacote):
  ```go
  // config ganha (decisão 2):
  Platform func(domain.Clock, *slog.Logger) platformParts
  // instance ganha: coord *feedback.Coordinator; router *pauseRouter; sources []input.Source (as iniciadas); power io.Closer
  func realPlatform(clock domain.Clock, log *slog.Logger) platformParts // components_windows.go
  ```
- Notas de implementação:
  - **Ordem de subida:** store → recuperação → `NewOverlay`/`NewSound` (falha → Warn e `Nop`) → `feedback.New` + `Start` → `tracker.New(Feedback: func(k) { coord.Send(feedback.Event(k)) })` + `Start` → servidor IPC → `pauseRouter.Start` → `Source.Start(triggers)` de cada fonte e `Power`.
    - Falha de componente: Warn e segue.
    - Caminhos de erro anteriores ao `Serve` também fecham o coordenador (depois do tracker).
  - **Ordem de shutdown:** `BeginShutdown` → `power.Close` → `Source.Close` (cada) → `router.Close` → `Drain` → `Server.Close` → `wg.Wait` → `tracker.Close` → `coord.Close` → `Store.Close`.
  - `realPlatform` (Windows): `NewHotkey(DefaultHotkey, ...)`; se `NewXInput` falhar, loga Warn "controle indisponível (XInput)" e omite o `Pad`; `Power` adapta `winapi.NewPowerWatcher(...).Start`; `NewOverlay` e `NewSound` criam `winapi.NewOverlay(...)` + `Start()` e `winapi.NewBeeper`.
  - `Platform == nil` não cria nada.
- Testes (falsos, `FakeClock`, store real em `t.TempDir()`; **nenhum teste usa `realPlatform`**):
  - `TestStartupOrderWithPause`:
    - "AGT-012/ordem: recuperação, feedback, tracker, servidor, roteador e depois fontes e energia"
    - "AGT-012/nenhuma fonte recebe Start antes de o tracker iniciar"
    - "AGT-012/a recuperação fecha a pausa aberta antes de qualquer fonte" (sessão `paused` pendente no banco)
  - `TestComponentFailures`:
    - "PAU-006/fonte com Start que falha: Warn com source e error, o agente sobe e as outras fontes funcionam"
    - "PAU-006/energia que falha ao iniciar não impede o agente"
    - "PAU-006/NewOverlay com erro: Warn aviso na tela indisponível, o som e a pausa continuam" (OVL-006)
    - "PAU-006/NewSound com erro: o aviso e a pausa continuam"
    - "PAU-006/o rastreamento de tempo não muda: lançar, pausar e encerrar funciona com tudo indisponível"
  - `TestShutdownWithPausedSession`:
    - "AGT-013/ordem: BeginShutdown, energia, fontes, roteador, Drain, Server.Close, Tracker.Close, feedback, Store.Close"
    - "AGT-013/sessão Paused encerra com agent-shutdown e a pausa fechada em ended_at"
    - "AGT-013/nenhum feedback no encerramento"
    - "AGT-013/Shutdown termina em < 2 s com a sessão em Paused"
    - "AGT-013/gatilho depois do BeginShutdown é ignorado (PAU-005)"
  - `TestSessionChangedPause` (dois clientes por `ln.listen`):
    - "IPC-037/Running → Paused → Running → Paused → Grace: uma notificação por transição, na ordem, com asOf do instante do evento"
    - "IPC-037/gatilho, evento de energia ou lançamento sem contar tempo ignorados não notificam"
    - "IPC-037/pedidos idempotentes (session.pause em Paused, session.resume em Running) não notificam"
    - "IPC-037/retomada por acordar notifica running com asOf do RESUMESUSPEND"
  - `TestFeedbackWiring`:
    - "FBK-003/Overlay.Show que bloqueia 5 s reais não atrasa as transições nem o Notify (tracker real com FakeClock)"
    - "FBK-002/feedback.Event e tracker.FeedbackKind têm os mesmos valores"
- Depende de: T14, T16, T17, T18, T19 (e T10/T11/T13 só para `components_windows.go`)

## T21 — UI Go: bindings, mensagens de erro e repasse de sessão pausada
- Requisitos: DSK-013, DSK-014, DSK-015
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `cmd/arcadia/games.go` (alterar)
  - `cmd/arcadia/errors.go` (alterar)
  - `cmd/arcadia/supervisor.go`: sem mudança de código esperada (`sessionView` valida por `domain.ParseSessionState`, que aceita `paused` desde T01).
  - testes: `cmd/arcadia/games_test.go`, `errors_test.go`, `supervisor_test.go`
  - **`cd cmd/arcadia && wails generate module`**: atualiza `frontend/wailsjs/go/main/App.{d.ts,js}` e `models.ts`.
- Assinaturas públicas:
  ```go
  const pauseSessionTimeout = 5 * time.Second
  func (a *App) LaunchGameUntracked(id int64) error   // game.launchUntracked, prazo 12 s, sem evento e sem valor
  func (a *App) PauseSession() (SessionView, error)   // session.pause, 5 s
  func (a *App) ResumeSession() (SessionView, error)  // session.resume, 5 s
  // errors.go
  const (
      msgNothingToPause   = "Não há jogo em andamento para pausar."
      msgGraceNoPause     = "O jogo está encerrando. Não dá para pausar agora."
      msgPauseUnavailable = "Não foi possível pausar agora."
  )
  ```
  `userError` ganha o caso `ipc.CodeSessionNotPausable`.
- Testes (`pinger` falso):
  - `TestLaunchGameUntracked`:
    - "DSK-013/chama game.launchUntracked com {gameId: id} e prazo de 12 s, devolve sucesso sem valor e não emite session:changed"
    - "DSK-013/-32002 vira Já há um jogo em andamento: «title». Feche-o antes de iniciar outro."
    - "DSK-013/-32003 vira Não foi possível iniciar o jogo: «error»"
  - `TestPauseResumeSession`:
    - "DSK-015/PauseSession chama session.pause (5 s) e devolve a SessionView; ResumeSession chama session.resume"
    - "DSK-015/os bindings não emitem evento por conta própria"
  - `TestUserErrorNotPausable`:
    - "DSK-015/-32005 reason no-session → Não há jogo em andamento para pausar."
    - "DSK-015/-32005 reason grace → O jogo está encerrando. Não dá para pausar agora."
    - "DSK-015/-32005 sem data ou motivo desconhecido → Não foi possível pausar agora."
    - "DSK-015/as demais mensagens de DSK-011 continuam"
  - `TestSupervisorPaused`:
    - "DSK-014/session.changed com state paused emite session:changed com state paused, activeSeconds, activeMs, asOf e endedAt vazio"
    - "DSK-014/CurrentSession devolve a sessão pausada no mesmo formato"
    - "DSK-014/estado desconhecido é registrado em Warn e não é repassado"
- Depende de: T01, T05

## T22 — frontend: `lib/games.ts` e `lib/sessionClock.ts`
- Requisitos: UI-022, UI-023, UI-025, UI-028
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `frontend/src/lib/games.ts` (alterar)
  - `frontend/src/lib/sessionClock.ts` (alterar)
  - testes: `frontend/src/lib/games.test.tsx` e `frontend/src/lib/sessionClock.test.ts` (alterar)
- Assinaturas públicas:
  ```ts
  // games.ts
  export type SessionState = 'running' | 'paused' | 'grace' | 'ended' | 'discarded'
  export async function launchGameUntracked(id: number): Promise<void>
  export async function pauseSession(): Promise<SessionView>
  export async function resumeSession(): Promise<SessionView>
  export function useLaunchGameUntracked(): UseMutationResult<void, Error, number> // não mexe no cache de sessão nem na lista
  export function useToggleSession(): UseMutationResult<SessionView, Error, void>   // pauseSession() em running, resumeSession() em paused, conforme o cache
  // WailsApp ganha LaunchGameUntracked?, PauseSession?, ResumeSession?
  // sessionClock.ts
  export type SessionAnchor = { activeMs: number; state: 'running' | 'paused' | 'grace'; receivedAt: number }
  ```
- Notas de implementação:
  - `isLive` e `isSessionView` tratam `running`, `paused` e `grace` como sessão viva.
  - `anchorFrom` mapeia `paused` → `'paused'`.
  - `elapsedMs` e `useElapsedSeconds` só avançam e só armam temporizador em `running`.
  - `useToggleSession` aplica a resposta só se nenhum `session:changed` foi aplicado desde o pedido (mesma regra de UI-018) e nunca altera o cache antes da resposta.
- Testes (Vitest, relógio falso):
  - `sessionClock.test.ts`:
    - "UI-022/anchorFrom de paused congela: A = 100000 mostra 0:01:40 e depois de 10 min falsos continua 0:01:40"
    - "UI-023/de paused para running o cronômetro volta a avançar de A: 0:01:40 → 0:01:41 após 1000 ms"
    - "UI-023/de paused para grace congela"
    - "UI-025/em paused nenhum setTimeout fica armado (vi.getTimerCount() é 0)"
  - `games.test.tsx`:
    - "UI-022/session:changed com paused entra no cache como sessão viva"
    - "UI-016/reconexão com sessão paused zera o cache e recarrega" (caso `paused` acrescentado ao teste existente)
    - "UI-018/resposta de pauseSession atrasada, depois de um session:changed, não sobrescreve o cache" (caso `paused`)
    - "UI-028/useToggleSession chama PauseSession em running e ResumeSession em paused, uma vez cada"
    - "UI-028/o cache não muda antes da resposta"
    - "UI-020/useLaunchGameUntracked chama LaunchGameUntracked uma vez e não toca no cache de sessão nem recarrega a lista"
- Depende de: T21

## T23 — frontend: componentes e `App`
- Requisitos: UI-019, UI-020, UI-021, UI-022, UI-024, UI-025, UI-027, UI-028
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `frontend/src/components/GameList.tsx` (alterar)
  - `frontend/src/components/SessionPanel.tsx` (alterar)
  - `frontend/src/App.tsx` (alterar)
  - testes: `GameList.test.tsx`, `SessionPanel.test.tsx` e `App.test.tsx` (alterar)
- Assinaturas públicas:
  ```ts
  type PendingLaunch = { id: number; mode: 'tracked' | 'untracked' } | null
  GameList({ games, current, pending, onPlay, onPlayUntracked }: {
    games: GameView[]; current: SessionView | null; pending: PendingLaunch
    onPlay: (id: number) => void; onPlayUntracked: (id: number) => void
  })
  SessionPanel({ session, onToggle, pending, now }: {
    session: SessionView; onToggle: () => void; pending: boolean; now?: () => number
  })
  ```
- Notas de implementação:
  - `GameList` mostra "Em execução" em `running`/`grace` e "Pausado" em `paused`. Cada linha tem "Jogar" e "Sem contar tempo". No jogo pendente os dois botões ficam `disabled` e só o acionado mostra "Iniciando…".
  - `SessionPanel` mostra "Pausado" em `paused` (cor distinta do âmbar de "Encerrando…") e "Encerrando…" em `grace`. O botão mostra "Pausar" em `running` e `grace` (desabilitado em `grace` ou com `pending`) e "Retomar" em `paused`. O rótulo vem só de `session.state`.
  - `App` guarda `pausePending` e chama `useToggleSession`; `onPlayUntracked` usa `useLaunchGameUntracked`; erro → `ErrorBanner`; sucesso → limpa o aviso e não faz mais nada.
- Testes:
  - `GameList.test.tsx`:
    - "UI-019/cada linha mostra Jogar e Sem contar tempo"
    - "UI-021/lançamento pendente do jogo G desabilita os dois botões de G, só o acionado mostra Iniciando… e os outros jogos continuam habilitados"
    - "UI-024/sessão paused mostra Pausado no lugar de Em execução; running e grace mostram Em execução"
    - "UI-024/ended ou discarded: a marcação some"
  - `SessionPanel.test.tsx`:
    - "UI-022/paused mostra título, 0:01:40 parado após 10 min falsos, Pausado e não mostra Encerrando…"
    - "UI-023/de paused para running Pausado some e o cronômetro volta a andar; para grace mostra Encerrando… congelado"
    - "UI-025/em paused nenhum binding é chamado e nenhum temporizador fica armado"
    - "UI-027/rótulo Pausar em running, Retomar em paused, Pausar desabilitado em grace"
    - "UI-027/botão desabilitado enquanto pending e rótulo muda quando o estado novo chega"
    - "UI-028/clicar chama onToggle uma vez e não altera estado, cronômetro nem rótulo por conta própria"
  - `App.test.tsx`:
    - "UI-020/Sem contar tempo chama LaunchGameUntracked(G) uma vez e não LaunchGame; sem painel, sem Em execução, sem recarregar a lista"
    - "UI-020/erro -32002 mostra o aviso do topo com Fechar e a próxima ação bem-sucedida o remove"
    - "UI-028/clicar Pausar chama PauseSession uma vez e Retomar chama ResumeSession uma vez"
    - "UI-028/erro (-32005, conexão perdida) mostra o aviso, o painel não muda e a próxima ação bem-sucedida o remove"
    - "UI-027/botão desabilitado durante a chamada pendente (sem duplo clique)"
    - "UI-022/painel carregado por CurrentSession em paused mostra Pausado"
- Depende de: T22

## T24 — Documentação
- Requisitos: STO-018, IPC-032, DOM-010, AGT-013 (anotações em docs de fases anteriores; nenhum teste automatizado)
- Responsável:
  - sessão principal: `CLAUDE.md` e `docs/ARCHITECTURE.md`
  - spec-writer: as anotações em `docs/specs/phase-2-skeleton`, `docs/specs/single-exe` e `docs/specs/phase-3-tracker`
- Arquivos:
  - `CLAUDE.md`: "Spec atual" aponta para `docs/specs/phase-4-pause/`; layout ganha `internal/{input,feedback}`; nota de que o atalho `Ctrl+Alt+P` e o combo `Select+Y` são fixos nesta fase.
  - `docs/ARCHITECTURE.md`: §4.3 (diagrama com `Paused`), §4.4 (atalhos fixos e combo Select+Y), §10 item 4 (decidido), ADRs 0017–0019 referenciados.
  - `docs/specs/phase-3-tracker/requirements.md`:
    - marcar IPC-030 como "substituído por IPC-032" (o `spec-trace.ps1` ignora requisitos substituídos; os casos "IPC-030/…" saem do `contract_test.go` em T05)
    - anotar "alterado em 2026-11-10" em DOM-007, STO-011 (versão 3 → 4), STO-013..015, SES-002, SES-009, SES-010, SES-011, SES-013, IPC-026, IPC-027, DSK-010, UI-004, UI-006, UI-008, UI-012
  - `docs/specs/phase-2-skeleton/requirements.md`: "alterado em 2026-11-10" em STO-001, AGT-003, AGT-006, AGT-009, AGT-010.
  - `docs/specs/single-exe/requirements.md`: "alterado em 2026-11-10" em AST-001 (versão 4).
  - Limitação 7 da fase 3 (suspensão conta como tempo jogado) anotada como superada pela Limitação 12 da fase 4.
- Testes: nenhum automatizado; `spec-trace.ps1` confere a anotação "substituído por".
- Depende de: T20, T23

## T25 — Verificação final e checagens manuais
- Requisitos: HOT-005, PAD-009, PWR-007, OVL-007, SND-003, PAU-007, UI-026
- Responsável: reviewer (os builds ficam com o implementer)
- Roteiro:
  - **Build e testes:**
    - `CGO_ENABLED=0 go build ./...`, `go test ./...` (com `-race` em `internal/tracker`, `internal/input`, `internal/feedback` e `internal/agent`), `go vet ./...` e `GOOS=linux go vet ./...` passam.
    - `cd frontend && npm test && npm run build` passa.
    - `wails build -ldflags "-X main.version=1.2.3"` gera `arcadia.exe`.
  - **Migração:** um banco v3 real abre na versão 4 sem perder dados.
  - **HOT-005:** jogo em janela, tela cheia sem borda e tela cheia exclusiva: `Ctrl+Alt+P` alterna Running ↔ Paused, o jogo não perde o foco e segurar as teclas por 2 s alterna uma única vez.
  - **PAD-009:** controle com fio e sem fio: `Select+Y` por 1 s alterna; toque curto não; cada slot 1..4 funciona; desligar o controle sem fio no meio da pressão não alterna.
  - **OVL-007:** o aviso aparece sobre o jogo em janela e em tela cheia sem borda, sem roubar o foco, no topo e centralizado, por ~2,5 s; em tela cheia exclusiva anotar se aparece.
  - **SND-003:** o sinal descendente na pausa e o ascendente na retomada, inclusive em tela cheia exclusiva.
  - **PWR-007:** suspender e acordar por ação do usuário e hibernar e acordar: sessão `Paused` durante o sono, intervalo `reason = sleep`, `active_seconds` sem o sono (±2 s), UI volta a "Em execução". Acordar sem usuário (tarefa agendada) mantém `Paused`. Anotar o comportamento em PC com Modern Standby (Limitações 1 e 12).
  - **PAU-007:** "Jogar" → pausa/retoma pelo atalho → pausa/retoma pelo controle → fechar o jogo: UI "Pausado" no painel e na linha, cronômetro parado, aviso e som a cada troca, total sem o tempo pausado (±2 s), `pause_interval` com `hotkey` e `gamepad`.
  - **UI-026:**
    - "Sem contar tempo" → jogar → fechar: nenhuma sessão e o total não muda.
    - "Jogar" → pausar pelo atalho: "Pausado" no painel e na linha; fechar e reabrir a UI durante a pausa e o painel volta em "Pausado" com o valor certo (±1 s).
    - Clicar "Retomar" e depois "Pausar" no painel: aviso e som aparecem e `pause_interval` tem `reason = ui`.
  - **Rastreabilidade:** `spec-trace.ps1` sai com 0 para `phase-4-pause`, `phase-3-tracker`, `single-exe` e `phase-2-skeleton`.
- Depende de: T01–T24

---

## Riscos de concorrência e recursos

| Goroutine / recurso | Dono | Como termina / quem fecha |
|---|---|---|
| Coordenador do tracker (`run`), agora com `ctrlCh` (cap 16) | `Tracker.wg` | `stop` fechado por `Close`; antes de sair aplica `Stop` (inclusive em Paused) e fecha o job. Pedidos pendentes em `ctrlCh` ficam sem resposta, mas os chamadores têm `done`/`ctx` |
| Atalho: goroutine `loop` com `LockOSThread` | `input.hotkey` (`wg`) | `Close` posta `WM_QUIT` na thread; a própria goroutine chama `UnregisterHotKey` e sai; `Close` espera |
| Poller do controle | `input.Pad` (`wg`) | `stop` fechado por `Close`; sai no próximo `select`, sem depender do relógio |
| `PowerWatcher`: janela oculta + laço em thread travada | `winapi.PowerWatcher` | `WM_APP+1`; o laço destrói janela e classe na própria thread; `Close` espera |
| Janela do aviso: laço, classe, fonte e pincel GDI | `winapi.Overlay` | `WM_APP+1`; o laço libera tudo na própria thread; `Close` espera |
| Som: goroutine + `Beep` síncrono | `winapi.Beeper` | `stop` fechado por `Close`; `Close` espera (no máximo o sinal em curso, ~210 ms) |
| Coordenador de feedback | `feedback.Coordinator` | `stop` fechado por `Close`; esconde o aviso, fecha `Overlay` e `Sound` e espera a goroutine |
| Roteador de gatilhos | `agent.pauseRouter` | `stop` fechado por `Close`; fila `triggers` (cap 8) nunca é fechada pelas fontes |
| `powerBridge.OnSuspend` | thread da janela do `PowerWatcher` | não cria goroutine; espera no máximo 1,5 s e volta |
| `LaunchDetached` (`hProcess`, `hThread`) | `winLauncher` | fechados com `defer` antes de retornar; TRK-011 mede |
| Processos de `fake-game` nos testes | o teste | `--until-file` criado no `t.Cleanup` e `--max-life 30s` |
| Cronômetro da UI (`setTimeout`) | `useEffect` de `useElapsedSeconds` | limpo no cleanup; nenhum em `paused`, `grace`, sem âncora ou desmontado |

- **Ordem de subida:** store → recuperação → feedback → tracker → servidor IPC → roteador → fontes e energia.
- **Ordem de shutdown:**
  1. `tracker.BeginShutdown`
  2. `power.Close`, `Source.Close` (cada uma) e `router.Close`
  3. `Drain` (≤ 500 ms)
  4. `Server.Close`
  5. `wg.Wait`
  6. `tracker.Close(ctx)` (uma escrita de fim, ≤ 2 s; fecha a pausa aberta)
  7. `coord.Close`
  8. `Store.Close`
  9. log "shutdown concluído", watcher de fim de sessão, mutex, `Close` do log
- **Locks:** o `mu` do snapshot do tracker continua tomado só pela goroutine dona (escrita) e por `Current`/`Launch`/`LaunchUntracked` (leitura); nenhuma chamada externa com ele tomado. `Config.Feedback` e `Notify` rodam sem lock. `windowHandlers` (mapa de `hwnd`) tem seu próprio mutex, solto antes de chamar o handler.
- **Suspensão:** o Windows espera `PBT_APMSUSPEND` ~2 s. A ponte espera ≤ 1,5 s; a goroutine dona do tracker ainda aplica o evento com o instante original (decisão 1). A gravação pode terminar depois de o PC dormir; o heartbeat ao acordar corrige, e uma queda durante o sono é recuperada em `last_seen_at`.
- **Threads de UI:** nenhum callback de janela chama o tracker diretamente (o atalho passa pelo roteador; a energia, pela ponte com timeout), então nenhuma thread de mensagens bloqueia.
- **`FakeClock`:** temporizadores não canceláveis; o coordenador de feedback usa geração para o `hide`; o poller usa um `After` por ciclo. Contagens de waiters: Running 2, Paused 2, Grace 3.
- **Colisão com o desenvolvedor:** testes só registram `Ctrl+Alt+Shift+F24`; `startAgent` sem `Platform` não cria componentes de sistema.

## Rastreabilidade

| Requisito | Tarefa(s) |
|---|---|
| DOM-010, DOM-011 | T01 |
| STO-018 | T02 |
| STO-019, STO-020, STO-022 | T03 |
| STO-021, STO-023 | T04 |
| SES-016, SES-017 | T06, T07 |
| SES-018 | T06, T07 |
| SES-019 | T06 |
| SES-020 | T06, T07 |
| SES-021, SES-022 | T06, T07 |
| SES-023 | T04, T06 |
| SES-024 | T06, T07 |
| SES-025 | T04 |
| SES-026 | T06, T07 |
| SES-027, SES-029, SES-030 | T06 |
| SES-028 | T06, T07 |
| SES-031 | T07, T08 |
| SES-032, SES-033, SES-034 | T08 |
| SES-035, SES-036, SES-037 | T06, T07, T18 |
| TRK-009, TRK-010, TRK-011 | T08 |
| HOT-001..HOT-004 | T10 |
| HOT-005 (manual) | T25 |
| PAD-001..PAD-006, PAD-008 | T09 |
| PAD-007 | T11 |
| PAD-009 (manual) | T25 |
| PWR-001..PWR-004 | T13 |
| PWR-005 | T07, T19 |
| PWR-006 | T12, T13 |
| PWR-007 (manual) | T25 |
| OVL-001, OVL-003 | T14 |
| OVL-002, OVL-005 | T16 |
| OVL-004 | T15 |
| OVL-006 | T16, T20 |
| OVL-007 (manual) | T25 |
| SND-001, SND-002 | T17 |
| SND-003 (manual) | T25 |
| FBK-001 | T06, T07 |
| FBK-002, FBK-004, FBK-005 | T14 |
| FBK-003 | T07, T14, T20 |
| PAU-001..PAU-005 | T19 |
| PAU-006 | T20 |
| PAU-007 (manual) | T25 |
| PAU-008 | T18, T19 |
| AGT-012, AGT-013 | T20 |
| IPC-032 | T05, T24 |
| IPC-033, IPC-034, IPC-035 | T18 |
| IPC-036 | T18 |
| IPC-037 | T20 |
| IPC-038, IPC-039, IPC-040 | T18 |
| DSK-013, DSK-014, DSK-015 | T21 |
| UI-019, UI-021 | T23 |
| UI-020 | T22, T23 |
| UI-022, UI-023, UI-025 | T22, T23 |
| UI-024 | T23 |
| UI-027 | T23 |
| UI-028 | T22, T23 |
| UI-026 (manual) | T25 |
| Afetados de outras fases: STO-001, AST-001 (T02); DOM-007 (T01); STO-011 (T02, T24); IPC-030 (T05, T24); AGT-003, AGT-006, AGT-009, AGT-010 (T20, T24, T25); UI-016, UI-018 (T22) | — |

## Decisões novas que merecem ADR
Nenhuma além dos ADRs 0017, 0018 e 0019 já escritos. As decisões do planner 1–10 são de implementação; a 2 (fábrica `Platform`) e a 4 (registro de janelas compartilhado) podem virar nota no `design.md`.
