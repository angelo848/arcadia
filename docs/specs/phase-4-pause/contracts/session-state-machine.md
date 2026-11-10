# Máquina de estados da sessão (Camada 1, com pausa) — contrato

Fonte da verdade para `internal/tracker` (máquina pura + coordenador). **Estende e substitui** o contrato da fase 3
([session-state-machine.md](../../phase-3-tracker/contracts/session-state-machine.md)): todas as linhas daquele contrato
continuam valendo e estão repetidas aqui; entram o estado `Paused`, os eventos de pausa, de energia e de lançamento sem
rastrear, e os efeitos de pausa e de feedback. Requisitos: SES-001..SES-037, FBK-001, AGT-011.

## Estados

| Estado | Em memória | Gravado em `play_session.state` | Sessão atual (`session.current`) |
|---|---|---|---|
| Idle | sim | — (nenhuma linha aberta) | `null` |
| Launching | sim (transitório, dura o `game.launch`) | — antes do insert; `running` depois do insert e antes do resume | `null` |
| Running | sim | `running` | sessão com `state = running` |
| **Paused** | sim | **`paused`** (e uma linha aberta em `pause_interval`) | sessão com `state = paused`, tempo congelado |
| Grace | sim | `grace` | sessão com `state = grace` |
| Ended | transitório → Idle | `ended` ou `discarded` (DOM-008) | `null` |
| Failed | transitório → Idle | — (linha nunca existe ou é apagada) | `null` |

Campos em memória: `sessionID`, `gameID`, `title`, `startedAt`, `runningSince` (início do intervalo Running atual),
`accumulated` (soma dos intervalos Running já fechados), `graceSince` (instante em que o job ficou vazio), **`pausedAt`**
(instante em que a pausa começou) e **`pauseReason`** (`hotkey` | `gamepad` | `ui` | `sleep`).

Tempo ativo em `t`: `accumulated + (t − runningSince)` em Running; `accumulated` em Paused e em Grace (a pausa e a
carência não contam — decisão P4 da fase 3 e SES-016). Durações com `time.Time.Sub` sobre instantes do `domain.Clock`
(o `SystemClock` carrega leitura monotônica); um intervalo negativo conta 0 e nunca desconta o acumulado. O descarte
(DOM-008) olha só o tempo ativo, nunca a duração no relógio (SES-023).

**Instante dos eventos externos.** `TogglePause`, `Suspend` e `Wake` carregam o instante `t` em que o evento aconteceu
(lido pela fonte no momento do atalho/da mensagem, não quando a goroutine dona os atende). O coordenador aplica
`max(t, último instante aplicado)` para o tempo nunca andar para trás. Assim uma suspensão atendida só depois de acordar
ainda pausa no instante em que o PC adormeceu (PWR-005).

**Launching × Stop/Pausa não ocorre:** o lançamento inteiro roda na goroutine dona, então pedidos de pausa, energia e
parada só são lidos em Idle, Running, Paused ou Grace. Um gatilho com `t < startedAt` é descartado (SES-018).

**Pausar não toca no jogo:** nenhuma transição de pausa chama `Job.Terminate`, `Job.Close` nem suspende processos.

## Eventos

| Evento | Origem |
|---|---|
| `Launch(spec)` | `game.launch` (via `Tracker.Launch`) |
| **`LaunchUntracked(spec)`** | `game.launchUntracked` (via `Tracker.LaunchUntracked`) |
| `Prepared(t)` | `Launcher.Prepare` criou o processo suspenso e o associou ao job |
| `Inserted(id)` / `InsertFailed(err)` | `SessionStore.StartSession` |
| `Resumed` / `ResumeFailed(err)` | `Job.Resume` |
| `PrepareFailed(err)` | `Launcher.Prepare` |
| `ProcessAdded(t)` | `JOB_OBJECT_MSG_NEW_PROCESS`, ou verificação periódica com processos ativos > 0 (SES-013) |
| `TrackedEmpty(t)` | `JOB_OBJECT_MSG_ACTIVE_PROCESS_ZERO`, ou verificação periódica com 0 processos ativos (SES-013, SES-022) |
| `RootExited(pid, code)` | `JOB_OBJECT_MSG_EXIT_PROCESS`/`ABNORMAL_EXIT_PROCESS` com o PID do processo inicial |
| `HeartbeatTick(t)` | temporizador de 30 s (`clock.After`) |
| `GraceTimeout` | temporizador de 15 s (`clock.After`) |
| `PollTick(t)` | temporizador de 5 s (`clock.After`) |
| **`TogglePause(t, reason)`** | gatilho do atalho (`hotkey`) ou do controle (`gamepad`), via `Tracker.TogglePause` |
| **`Pause(t)`** | `session.pause` (motivo `ui`), via `Tracker.Pause`; explícito e idempotente |
| **`Resume(t)`** | `session.resume`, via `Tracker.Resume`; explícito e idempotente |
| **`Suspend(t)`** | `PBT_APMSUSPEND` (via `Tracker.Suspend`) |
| **`Wake(t, userPresent)`** | `PBT_APMRESUMESUSPEND` (`userPresent = true`) ou `PBT_APMRESUMEAUTOMATIC` (`false`), via `Tracker.Wake` |
| `Stop(t)` | `Tracker.Close` (shutdown do agente) |

## Transições

Estado × evento → próximo estado + efeitos. "Notifica" = `session.changed` com a sessão no estado novo (IPC-027,
IPC-037). "Feedback" = `Config.Feedback(kind)` (FBK-001), chamado depois da gravação e sem bloquear.
Qualquer combinação ausente da tabela é ignorada (log `Debug`), nunca é erro.

### Lançamento (inalterado, mais o lançamento sem rastrear)

| Estado | Evento | Próximo | Efeitos |
|---|---|---|---|
| Idle | `Launch(spec)` | Launching | chama `Launcher.Prepare(spec)` |
| Launching | `PrepareFailed(err)` | Failed → Idle | o `Prepare` já terminou/fechou o que criou; log `Error` "falha ao iniciar o jogo"; responde `LaunchFailed` [SES-012] |
| Launching | `Prepared(t)` | Launching | `StartSession(gameID, t)` (linha `running`, STO-012) [SES-001] |
| Launching | `InsertFailed(err)` | Failed → Idle | `Job.Terminate` (processo nunca retomado) + `Job.Close`; log `Error`; responde `LaunchFailed` [SES-012] |
| Launching | `Inserted(id)` | Launching | `Job.Resume()` |
| Launching | `ResumeFailed(err)` | Failed → Idle | `Job.Terminate` + `Job.Close`; `DeleteSession(id)` (o `ON DELETE CASCADE` leva `pause_interval`); log `Error`; responde `LaunchFailed` [SES-012] |
| Launching | `Resumed` | Running | `startedAt = runningSince = t` (o `t` de `Prepared`); `accumulated = 0`; arma heartbeat (30 s) e verificação (5 s); notifica `running`; responde a sessão [SES-001] |
| Launching / Running / **Paused** / Grace | `Launch(spec)` | (mesmo) | nada é criado nem gravado; responde `SessionActive{sessionId, gameId, title}` (em Launching, `sessionId = 0` até o insert) [SES-002, SES-031] |
| **Idle** | **`LaunchUntracked(spec)`** | Idle | `Launcher.LaunchDetached(spec)` na goroutine dona: sucesso → responde `{pid}`, **sem** `StartSession`, **sem** job, **sem** notificação; falha → log `Error` "falha ao iniciar o jogo", responde `LaunchFailed` [SES-032, SES-034] |
| **Launching / Running / Paused / Grace** | **`LaunchUntracked(spec)`** | (mesmo) | nada é criado; responde `SessionActive` [SES-033] |
| **qualquer** | **`LaunchUntracked(spec)` depois de `BeginShutdown`** | (mesmo) | nada é criado; responde `ErrShuttingDown` [SES-033] |

### Running

| Estado | Evento | Próximo | Efeitos |
|---|---|---|---|
| Running | `ProcessAdded(t)` | Running | nenhum (log `Debug` com `pid`) |
| Running | `RootExited(pid, code)` | Running | log `Info` "processo inicial do jogo encerrou" (`session_id`, `pid`, `exit_code`) [SES-015] |
| Running | `HeartbeatTick(t)` | Running | `UpdateSession(running, last_seen = t, ativo em t)`; rearma heartbeat; falha → log `Error`, segue [SES-009, SES-014] |
| Running | `PollTick(t)`, ativos = 0 | Grace | igual a `TrackedEmpty(t)` e rearma a verificação [SES-013] |
| Running | `PollTick(t)`, ativos > 0 ou erro | Running | rearma verificação; erro → log `Warn` [SES-013] |
| Running | `TrackedEmpty(t)` | Grace | `accumulated += t − runningSince`; `graceSince = t`; `UpdateSession(grace, last_seen = t, accumulated)`; para de rearmar o heartbeat; arma carência (15 s); notifica `grace`; falha de escrita → log `Error`, segue [SES-003, SES-014] |
| Running | **`TogglePause(t, reason)`**, `t ≥ startedAt` | **Paused** | `accumulated += t − runningSince`; `pausedAt = t`; `pauseReason = reason`; `PauseSession(open interval, reason, state paused, last_seen = t, accumulated)`; **não** rearma nem cancela heartbeat e verificação; notifica `paused`; **Feedback(`paused`)** (`reason` é `hotkey`/`gamepad`); falha de escrita → log `Error`, segue [SES-016, SES-014, FBK-001] |
| Running | **`TogglePause(t, reason)`**, `t < startedAt` | Running | ignorado (log `Debug` "pausa ignorada") [SES-018] |
| Running | **`Pause(t)`**, `t ≥ startedAt` | **Paused** | igual a `TogglePause(t, ui)` (motivo `ui`, Feedback `paused`); responde a sessão pausada [SES-035] |
| Running | **`Resume(t)`** | Running | idempotente: nada é gravado, notificado nem sinalizado; responde a sessão atual [SES-036] |
| Running | **`Suspend(t)`** | **Paused** | igual a `TogglePause(t, sleep)`, mas **sem** Feedback [SES-026] |
| Running | **`Wake(t, _)`** | Running | ignorado [SES-029] |
| Running | `Stop(t)` | Ended → Idle | `accumulated += t − runningSince`; `EndSession(ended_at = t, accumulated, agent-shutdown)`; `Job.Close` (nunca termina processos); sem notificação [SES-011, TRK-004] |

### Paused (novo)

| Estado | Evento | Próximo | Efeitos |
|---|---|---|---|
| Paused | **`TogglePause(t, reason)`** | **Running** | `runningSince = t`; `ResumePause(close interval at t, state running, last_seen = t, accumulated)`; notifica `running`; **Feedback(`resumed`)** (qualquer `reason`, inclusive quando a pausa era `sleep`); `accumulated` não muda [SES-017] |
| Paused | **`Resume(t)`** | **Running** | igual a `TogglePause(t, _)` em Paused, para qualquer `pauseReason` (inclusive `sleep`); Feedback `resumed`; responde a sessão em Running [SES-036] |
| Paused | **`Pause(t)`** | Paused | idempotente: nada é gravado, notificado nem sinalizado; o intervalo aberto mantém o motivo; responde a sessão pausada [SES-035] |
| Paused | **`Wake(t, userPresent = true)`**, `pauseReason = sleep` | **Running** | igual à linha acima (retomada por acordar); **Feedback(`resumed`)** [SES-028] |
| Paused | **`Wake(t, userPresent = true)`**, `pauseReason ≠ sleep` | Paused | ignorado: a pausa era do usuário e continua [SES-030] |
| Paused | **`Wake(t, userPresent = false)`** | Paused | ignorado: retomada automática não retoma [SES-029] |
| Paused | **`Suspend(t)`** | Paused | ignorado; o intervalo aberto mantém o motivo [SES-027] |
| Paused | `HeartbeatTick(t)` | Paused | `UpdateSession(paused, last_seen = t, accumulated)`; rearma heartbeat; falha → log `Error`, segue [SES-020, SES-014] |
| Paused | `PollTick(t)`, ativos = 0 | Grace | igual a `TrackedEmpty(t)` e rearma a verificação [SES-022] |
| Paused | `PollTick(t)`, ativos > 0 ou erro | Paused | rearma verificação; erro → log `Warn` [SES-022] |
| Paused | `TrackedEmpty(t)` | Grace | `graceSince = t`; `ResumePause(close interval at t, state grace, last_seen = t, accumulated)`; para de rearmar o heartbeat; arma carência (15 s); notifica `grace`; **sem** Feedback; falha de escrita → log `Error`, segue [SES-021, SES-014] |
| Paused | `ProcessAdded(t)` | Paused | nenhum (log `Debug`) |
| Paused | `RootExited(pid, code)` | Paused | log `Info` de SES-015 |
| Paused | `Stop(t)` | Ended → Idle | `EndSession(ended_at = t, accumulated, agent-shutdown)` (o store fecha a pausa aberta em `t`, STO-021); `Job.Close`; sem notificação [SES-024] |

### Grace

| Estado | Evento | Próximo | Efeitos |
|---|---|---|---|
| Grace | `ProcessAdded(t)` | Running | `runningSince = t`; `UpdateSession(running, last_seen = t, accumulated)`; descarta a carência pendente (geração); arma heartbeat; notifica `running`; a pausa **não** é reaberta [SES-005, SES-021] |
| Grace | `PollTick(t)`, ativos > 0 | Running | igual a `ProcessAdded(t)` e rearma a verificação [SES-013] |
| Grace | `PollTick(t)`, ativos = 0 ou erro | Grace | rearma verificação; erro → log `Warn` |
| Grace | `TrackedEmpty(t)` | Grace | nenhum (já vazio) |
| Grace | `RootExited(pid, code)` | Grace | log `Info` de SES-015 |
| Grace | **`TogglePause`**, **`Suspend`**, **`Wake`** | Grace | ignorados (log `Debug` "pausa ignorada"): não há tempo correndo para pausar [SES-018, SES-027] |
| Grace | **`Pause`**, **`Resume`** | Grace | recusados: responde `NotPausable{grace}` (`-32005`); nada é gravado nem notificado [SES-037] |
| Grace | `GraceTimeout` | Ended → Idle | `EndSession(ended_at = graceSince, accumulated, exited)` — estado final por DOM-008; `Job.Close`; notifica `ended`/`discarded`; falha de escrita → log `Error`, segue para Idle (linha aberta, SES-010 na próxima subida) [SES-004, SES-006, SES-007, SES-014] |
| Grace | `Stop(t)` | Ended → Idle | `EndSession(ended_at = graceSince, accumulated, exited)`; `Job.Close`; sem notificação [SES-011] |

### Idle

| Estado | Evento | Próximo | Efeitos |
|---|---|---|---|
| Idle | **`TogglePause`**, **`Suspend`**, **`Wake`** | Idle | ignorados (log `Debug`); sem gravação, notificação nem feedback [SES-018, PAU-003] |
| Idle | **`Pause`**, **`Resume`** | Idle | recusados: responde `NotPausable{no-session}` (`-32005`) [SES-037] |
| Idle | `Stop(t)` | Idle | nenhum |

## Efeitos novos

| Efeito | Quem executa | O que faz |
|---|---|---|
| `PauseSession` | coordenador → `SessionStore.PauseSession` | uma transação: abre o intervalo e grava `state = 'paused'`, `last_seen_at`, `active_seconds` (STO-019) |
| `ResumePause` | coordenador → `SessionStore.ResumePause` | uma transação: fecha o intervalo em `t` (remove se tamanho zero) e grava `state` (`running` ou `grace`), `last_seen_at`, `active_seconds` (STO-020) |
| `Feedback(kind)` | coordenador → `Config.Feedback` | `FeedbackPaused` ou `FeedbackResumed`; chamado depois da gravação e da notificação, sem bloquear; fila cheia descarta (FBK-003) |
| `LaunchDetached` | coordenador → `Launcher.LaunchDetached` | `CreateProcess` sem job e sem `CREATE_SUSPENDED`; devolve o PID; fecha os handles do processo e da thread antes de retornar (TRK-010, TRK-011) |

## Notas

- **Temporizadores com geração.** `clock.After` não é cancelável: cada temporizador armado leva o número de geração da
  sessão/estado; um disparo de geração antiga é ignorado. Pausar e retomar **não** rearmam nem invalidam o heartbeat e
  a verificação (continuam os mesmos); só Running → Grace zera a geração do heartbeat, e Paused → Grace também.
- **Waiters esperados** (para `BlockUntilWaiters`): Running = heartbeat + verificação (2); **Paused = heartbeat +
  verificação (2)**; Grace = carência + verificação (2) + o heartbeat antigo ainda pendente (1) até ele disparar e ser
  ignorado. Depois de Grace → Running somam-se os temporizadores antigos ainda não disparados: até 4.
- **Efeitos de fim sem id:** `EffDeleteSession` e `EffEndSession` não levam o id; a máquina já voltou a Idle quando os
  devolve, então o coordenador guarda o `sessionID` **antes** de aplicar o evento. O mesmo vale para `PauseSession` e
  `ResumePause`, que usam o `sessionID` da sessão corrente.
- **Eventos do job depois de `Job.Close`** não chegam (o canal de eventos é fechado pelo leitor ao parar).
- **Estado final** de `EndSession`: `ended` se `accumulated ≥ 30 s`, senão `discarded` (DOM-008); só `ended` atualiza o
  jogo (STO-014, STO-015). A pausa aberta é fechada pelo `EndSession`, em qualquer caminho de fim (STO-021).
- **Instante de referência das sessões notificadas** [IPC-031]: a sessão de "Notifica" leva `asOf = t` do evento da
  transição e `activeMs` = tempo ativo em `t` (em Paused, Grace e no fim, o `accumulated`); para `GraceTimeout`, `asOf`
  é o instante em que a carência venceu (`ended_at` continua `graceSince`). `session.current` usa `asOf = clock.Now()`
  da chamada e o tempo congelado em Paused.
- **Ordem notificação × resposta:** a notificação `running` é enfileirada antes de `game.launch` responder; o cliente
  não deve depender dessa ordem.
- **Feedback depois da notificação:** o coordenador chama `Config.Feedback` na própria goroutine dona, depois da
  gravação e do `Notify`; ele só enfileira (não toca em janela nem em áudio).
- **Fim com a pausa por `sleep` e job vazio ao acordar:** `Wake` mantém Paused; o `TrackedEmpty`/`PollTick` seguinte leva
  a Grace e ao fim normal (sem perda de tempo ativo).

## Recuperação na subida (fora da máquina) [SES-010, SES-025]
Para cada linha com `ended_at IS NULL` (inclusive `state = 'paused'`): `EndSession(ended_at = last_seen_at,
active = active_seconds gravado, recovered)`, com a mesma regra de DOM-008. O `EndSession` fecha a pausa aberta em
`last_seen_at`, ou a remove se `last_seen_at ≤ paused_at` (STO-021). Roda antes de o tracker aceitar lançamentos e de o
IPC aceitar conexões (AGT-009, AGT-012); não gera notificação nem feedback. Um `state` desconhecido na linha é logado e
tratado como sessão aberta (ADR 0012).
