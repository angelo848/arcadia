# Máquina de estados da sessão (Camada 1) — contrato

Fonte da verdade para `internal/tracker` (máquina pura + coordenador). Deriva da ARCHITECTURE §4.3 sem `Paused`
(fase 4) e sem o timeout de Launching (decisão 4 do usuário: com Job Object o primeiro processo existe assim que
`CreateProcess` retorna). Requisitos: SES-001..SES-015, AGT-011.

## Estados

| Estado | Em memória | Gravado em `play_session.state` | Sessão atual (`session.current`) |
|---|---|---|---|
| Idle | sim | — (nenhuma linha aberta) | `null` |
| Launching | sim (transitório, dura o `game.launch`) | — antes do insert; `running` depois do insert e antes do resume | `null` |
| Running | sim | `running` | sessão com `state = running` |
| Grace | sim | `grace` | sessão com `state = grace` |
| Ended | transitório → Idle | `ended` ou `discarded` (DOM-008) | `null` |
| Failed | transitório → Idle | — (linha nunca existe ou é apagada) | `null` |

Campos em memória: `sessionID`, `gameID`, `title`, `startedAt`, `runningSince` (início do intervalo Running atual),
`accumulated` (soma dos intervalos Running já fechados), `graceSince` (instante em que o job ficou vazio).
Tempo ativo em `t`: `accumulated + (t − runningSince)` em Running; `accumulated` em Grace (decisão P4: a carência não
conta). Durações calculadas com `time.Time.Sub` sobre instantes do `domain.Clock` (o `SystemClock` carrega leitura
monotônica; mudar o relógio do Windows não altera a duração medida em memória).

## Eventos

| Evento | Origem |
|---|---|
| `Launch(spec)` | `game.launch` (via `Tracker.Launch`) |
| `Prepared(t)` | `Launcher.Prepare` criou o processo suspenso e o associou ao job |
| `Inserted(id)` / `InsertFailed(err)` | `SessionStore.StartSession` |
| `Resumed` / `ResumeFailed(err)` | `Job.Resume` |
| `PrepareFailed(err)` | `Launcher.Prepare` |
| `ProcessAdded(t)` | `JOB_OBJECT_MSG_NEW_PROCESS`, ou verificação periódica com processos ativos > 0 (SES-013) |
| `TrackedEmpty(t)` | `JOB_OBJECT_MSG_ACTIVE_PROCESS_ZERO`, ou verificação periódica com 0 processos ativos (SES-013) |
| `RootExited(pid, code)` | `JOB_OBJECT_MSG_EXIT_PROCESS`/`ABNORMAL_EXIT_PROCESS` com o PID do processo inicial |
| `HeartbeatTick(t)` | temporizador de 30 s (`clock.After`) |
| `GraceTimeout` | temporizador de 15 s (`clock.After`) |
| `PollTick(t)` | temporizador de 5 s (`clock.After`) |
| `Stop(t)` | `Tracker.Close` (shutdown do agente) |

## Transições

Estado × evento → próximo estado + efeitos. "Notifica" = `session.changed` com a sessão no estado novo (IPC-027).
Qualquer combinação ausente da tabela é ignorada (log `Debug`), nunca é erro.

| Estado | Evento | Próximo | Efeitos |
|---|---|---|---|
| Idle | `Launch(spec)` | Launching | chama `Launcher.Prepare(spec)` |
| Launching | `PrepareFailed(err)` | Failed → Idle | o `Prepare` já terminou/fechou o que criou; log `Error` "falha ao iniciar o jogo"; responde `LaunchFailed` [SES-012] |
| Launching | `Prepared(t)` | Launching | `StartSession(gameID, t)` (linha `running`, STO-012) [SES-001] |
| Launching | `InsertFailed(err)` | Failed → Idle | `Job.Terminate` (processo nunca retomado) + `Job.Close`; log `Error`; responde `LaunchFailed` [SES-012] |
| Launching | `Inserted(id)` | Launching | `Job.Resume()` |
| Launching | `ResumeFailed(err)` | Failed → Idle | `Job.Terminate` + `Job.Close`; `DeleteSession(id)`; log `Error`; responde `LaunchFailed` [SES-012] |
| Launching | `Resumed` | Running | `startedAt = runningSince = t` (o `t` de `Prepared`); `accumulated = 0`; arma heartbeat (30 s) e verificação (5 s); notifica `running`; responde a sessão [SES-001] |
| Launching / Running / Grace | `Launch(spec)` | (mesmo) | nada é criado nem gravado; responde `SessionActive{sessionId, gameId, title}` (em Launching, `sessionId = 0` até o insert) [SES-002] |
| Running | `ProcessAdded(t)` | Running | nenhum (log `Debug` com `pid`) |
| Running | `RootExited(pid, code)` | Running | log `Info` "processo inicial do jogo encerrou" (`session_id`, `pid`, `exit_code`) [SES-015] |
| Running | `HeartbeatTick(t)` | Running | `UpdateSession(running, last_seen = t, ativo em t)`; rearma heartbeat; falha → log `Error`, segue [SES-009, SES-014] |
| Running | `PollTick(t)`, ativos = 0 | Grace | igual a `TrackedEmpty(t)` [SES-013] |
| Running | `PollTick(t)`, ativos > 0 ou erro | Running | rearma verificação; erro → log `Warn` [SES-013] |
| Running | `TrackedEmpty(t)` | Grace | `accumulated += t − runningSince`; `graceSince = t`; `UpdateSession(grace, last_seen = t, accumulated)`; para de rearmar o heartbeat; arma carência (15 s); notifica `grace`; falha de escrita → log `Error`, segue [SES-003, SES-014] |
| Running | `Stop(t)` | Ended → Idle | `accumulated += t − runningSince`; `EndSession(ended_at = t, accumulated, agent-shutdown)`; `Job.Close` (nunca termina processos); sem notificação (o servidor já fechou) [SES-011, TRK-004] |
| Grace | `ProcessAdded(t)` | Running | `runningSince = t`; `UpdateSession(running, last_seen = t, accumulated)`; descarta a carência pendente (geração); arma heartbeat; notifica `running` [SES-005] |
| Grace | `PollTick(t)`, ativos > 0 | Running | igual a `ProcessAdded(t)` [SES-013] |
| Grace | `PollTick(t)`, ativos = 0 ou erro | Grace | rearma verificação; erro → log `Warn` |
| Grace | `TrackedEmpty(t)` | Grace | nenhum (já vazio) |
| Grace | `RootExited(pid, code)` | Grace | log `Info` de SES-015 |
| Grace | `GraceTimeout` | Ended → Idle | `EndSession(ended_at = graceSince, accumulated, exited)` — estado final por DOM-008; `Job.Close`; notifica `ended`/`discarded`; falha de escrita → log `Error`, segue para Idle (linha aberta, SES-010 na próxima subida) [SES-004, SES-006, SES-007, SES-014] |
| Grace | `Stop(t)` | Ended → Idle | `EndSession(ended_at = graceSince, accumulated, exited)`; `Job.Close`; sem notificação [SES-011] |
| Idle | `Stop(t)` | Idle | nenhum |

Notas:
- **Temporizadores com geração.** `clock.After` não é cancelável: cada temporizador armado leva o número de geração
  da sessão/estado; um disparo de geração antiga é ignorado. Testes com `FakeClock` contam os waiters pendentes
  sabendo disso (ver design.md, Testes).
- **Waiters esperados** (para `BlockUntilWaiters`): Running = heartbeat + verificação (2); Grace = carência +
  verificação (2) + o heartbeat antigo ainda pendente (1) até ele disparar e ser ignorado.
- **Eventos do job depois de `Job.Close`** não chegam (o canal de eventos é fechado pelo leitor ao parar).
- **Estado final** de `EndSession`: `ended` se `accumulated ≥ 30 s`, senão `discarded` (DOM-008); só `ended`
  atualiza o jogo (STO-014, STO-015).
- **Instante de referência das sessões notificadas** [IPC-031]: a sessão de "Notifica" leva `asOf = t` do evento
  da transição e `activeMs` = tempo ativo em `t` (em Grace e no fim, o `accumulated`); para `GraceTimeout`, `asOf` é
  o instante em que a carência venceu (`ended_at` continua `graceSince`). `session.current` usa `asOf = clock.Now()`
  da chamada.
- **Ordem notificação × resposta:** a notificação `running` é enfileirada antes de `game.launch` responder; o
  cliente não deve depender dessa ordem.

## Recuperação na subida (fora da máquina) [SES-010]
Para cada linha com `ended_at IS NULL`: `EndSession(ended_at = last_seen_at, active = active_seconds gravado,
recovered)`, com a mesma regra de DOM-008. Roda antes de o tracker aceitar lançamentos e de o IPC aceitar conexões
(AGT-009); não gera notificação.
