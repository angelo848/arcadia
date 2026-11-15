# Máquina de estados da sessão — fechar pelo controle (Select + B)

**Estende** o contrato da fase 4 ([session-state-machine.md](../../phase-4-pause/contracts/session-state-machine.md)), que
continua valendo inteiro (inclusive as linhas de adoção da fase 6). Aqui entram o evento `CloseByHotkey(t)`, o estado
intermediário `Closing`, os efeitos `RequestClose` e `KillJob` e o job guardado do lançamento sem rastrear. Requisitos:
CON-005..CON-013, CON-019, CON-030..CON-033.

**Regra de tempo (resposta 2 da aprovação):** o tempo ativo é cortado no instante do gatilho `t`; a espera do fechamento (até
3 s) e a morte não contam. Em Grace, o corte é `graceSince`, como em todo fim por carência.

## Evento novo

| Evento | Origem |
|---|---|
| `CloseByHotkey(t)` | gatilho `Back + B` segurado 1 s (CON-001), via roteador → `Tracker.CloseByHotkey`; `t` = instante em que o combo completou |
| `CloseTimeout` | temporizador de 3 s (`clock.After`, com geração) armado ao entrar em `Closing` ou no fechamento do jogo sem contar tempo |
| `DetachedPoll` | temporizador de 250 ms (`clock.After`, com geração), só no fechamento do jogo sem contar tempo |

O coordenador aplica `max(t, último instante aplicado)`, como nos outros eventos externos.

## Estado novo

| Estado | Em memória | Gravado em `play_session.state` | Sessão atual (`session.current`) |
|---|---|---|---|
| **Closing** | sim (transitório, até 3 s + morte) | **não muda** (continua `running` ou `paused`; nada é gravado durante a espera) | sessão no estado de origem (`running`/`paused`), `activeMs` = `accumulated` congelado em `closeAt` |

Campos novos em memória: `closeAt` (o `t` do gatilho), `closeFrom` (`Running` ou `Paused`), `closeGen` (geração do prazo).
Em `Closing`, o tempo ativo é `accumulated` (congelado em `closeAt`).

## Estado em memória novo (coordenador, fora da máquina)

| Campo | Dono | Ciclo de vida |
|---|---|---|
| `detached DetachedJob` (+ `gameId`, `title`) | goroutine dona | criado por `LaunchUntracked` bem-sucedido; o anterior é fechado (sem matar) antes; fechado e zerado ao fim de um fechamento bem-sucedido em Idle, quando está vazio, e por `shutdown()` |
| `closing{at, reply, gen}` | goroutine dona | criado ao começar a espera (sessão em `Closing` ou detached); zerado ao responder (sucesso, falha ou shutdown); `reply` tem buffer 1 |

## Transições novas

"Pedido" = efeito `RequestClose` (`WM_CLOSE` às janelas de topo visíveis dos PIDs vivos do jogo, CON-030), que devolve `n`
(janelas que receberam). "Fim do fechamento" = `EndSession(ended_at = closeAt, accumulated, closed-by-hotkey)` (estado por
DOM-008; em Paused o store fecha a pausa em `closeAt`, STO-021); `Job.Close`; notifica `ended`/`discarded` com
`endedAt = closeAt`, `asOf` = instante do fim; **sem** Feedback; responde `closed{gameId}` [CON-005, CON-006, CON-014].

| Estado | Evento / condição | Próximo | Efeitos | Resultado |
|---|---|---|---|---|
| Running | `CloseByHotkey(t)`, `t ≥ startedAt` | **Closing** | `accumulated += t − runningSince`; `closeAt = t`; `closeFrom = Running`; nada gravado nem notificado; Pedido; `n = 0` ou erro → `KillJob` na hora (linhas de `KillJob` abaixo); `n > 0` → arma `CloseTimeout` [CON-005, CON-008, CON-030] | (na saída de Closing) |
| Paused | `CloseByHotkey(t)` | **Closing** | `accumulated` não muda; `closeAt = t`; `closeFrom = Paused`; o intervalo de pausa continua aberto; Pedido; como acima [CON-006, CON-030] | (na saída de Closing) |
| Grace | `CloseByHotkey(t)`, `KillJob` ok | Ended → Idle | sem Pedido nem espera (job vazio); `graceGen = 0`; `EndSession(ended_at = graceSince, accumulated, closed-by-hotkey)`; `Job.Close`; notifica com `asOf = t`, `endedAt = graceSince` [CON-007] | `closed{gameId}` |
| Grace | `CloseByHotkey(t)`, `KillJob` falhou | Grace | nada gravado nem notificado; `Warn` [CON-013] | `failed` |
| Running / Paused | `CloseByHotkey(t)`, `t < startedAt` | (mesmo) | ignorado; `Debug` "fechar ignorado" [CON-012] | `refused{no-game}` |
| **Closing** | `TrackedEmpty(t')` (job: `ACTIVE_PROCESS_ZERO`; adotado: nenhum adotado vivo) ou `PollTick(t')` com ativos = 0 | Ended → Idle | Fim do fechamento, sem `KillJob` (`asOf = t'`) [CON-030] | `closed{gameId}` |
| **Closing** | `CloseTimeout` (geração atual), `KillJob` ok | Ended → Idle | Fim do fechamento (`asOf` = instante do prazo) [CON-030] | `closed{gameId}` |
| **Closing** | `KillJob` (no prazo ou imediato) falhou | `closeFrom` | desfaz: em Running, `runningSince = closeAt` (o tempo da espera conta, o jogo não fechou); em Paused, nada muda (o intervalo segue aberto); `closeGen = 0`; nada gravado nem notificado; `Warn` "falha ao fechar o jogo" (`session_id`, `error`) [CON-013] | `failed` |
| **Closing** | `CloseTimeout` de geração antiga | Closing | ignorado | — |
| **Closing** | `ProcessAdded`, `RootExited` | Closing | `Debug`; o prazo continua | — |
| **Closing** | `TogglePause`, `Suspend`, `Wake` | Closing | ignorados (`Debug`), sem gravação, notificação nem Feedback [CON-032] | — |
| **Closing** | `Pause(t)`, `Resume(t)` | Closing | nada gravado; responde a sessão atual (estado de origem, tempo de `closeAt`) [CON-032] | — |
| **Closing** | `HeartbeatTick(t)` | Closing | nada gravado; não rearma (a geração da sessão segue para o fim) [CON-032] | — |
| **Closing** | `CloseByHotkey(t)` | Closing | nada [CON-032] | `refused{closing}` |
| **Closing** | `Launch(spec)`, `LaunchUntracked(spec)` | Closing | nada criado; responde `SessionActive{sessionId, gameId, title}` [CON-031] | — |
| **Closing** | `Stop(t)` | Ended → Idle | **sem** `KillJob`; `EndSession(ended_at = closeAt, accumulated, agent-shutdown)` (Paused: fecha a pausa em `closeAt`); `Job.Close`; sem notificação; `closing.reply` ← `ErrShuttingDown` [CON-033] | `ErrShuttingDown` |
| Launching (URI, sem adoção) | `CloseByHotkey(t)` | Launching | nada; o prazo de adoção (60 s) continua [CON-009] | `refused{not-adopted}` |
| Idle, `detached` com processos ativos (ou leitura com erro → trata como vivo) | `CloseByHotkey(t)` | Idle (detached em espera) | Pedido aos PIDs do job; `n = 0` → `KillJob` na hora; `n > 0` → guarda `closing`, arma `CloseTimeout` e `DetachedPoll` [CON-010, CON-030] | (no fim da espera) |
| Idle, detached em espera | `DetachedPoll`, `ActiveProcesses() = 0` | Idle | `detached.Close()`; `detached = nil`; sem gravação, sem `session.changed` [CON-010] | `closed{gameId, untracked}` |
| Idle, detached em espera | `DetachedPoll`, ativos > 0 ou erro | Idle (em espera) | rearma `DetachedPoll` | — |
| Idle, detached em espera | `CloseTimeout` (geração atual), `KillJob` ok | Idle | `detached.Close()`; `detached = nil` [CON-010] | `closed{gameId, untracked}` |
| Idle, detached em espera | `KillJob` falhou | Idle | `detached` continua guardado; `Warn` [CON-013] | `failed` |
| Idle, detached em espera | `CloseByHotkey(t)` | Idle (em espera) | nada [CON-032] | `refused{closing}` |
| Idle, detached em espera | `Launch(spec)`, `LaunchUntracked(spec)` | Idle (em espera) | nada criado; responde `SessionActive{sessionId: 0, gameId, title}` do jogo sendo fechado [CON-031] | — |
| Idle, detached em espera | `Stop(t)` | Idle | sem `KillJob`; `detached.Close()` (sem matar); `closing.reply` ← `ErrShuttingDown` [CON-033] | `ErrShuttingDown` |
| Idle, `detached` sem processos | `CloseByHotkey(t)` | Idle | `detached.Close()`; `detached = nil` [CON-012] | `refused{no-game}` |
| Idle, sem `detached` | `CloseByHotkey(t)` | Idle | `Debug` "fechar ignorado" [CON-012] | `refused{no-game}` |
| qualquer | `CloseByHotkey(t)` depois de `BeginShutdown` | (mesmo) | nada [CON-021] | `ErrShuttingDown` |
| **Idle** (sem espera) | `LaunchUntracked(spec)` ok | Idle | fecha `detached` anterior (sem matar); guarda o novo; responde `{pid}` [CON-011] | — |
| qualquer | `Stop(t)` | (como na fase 4) | além do da fase 4: `detached.Close()` (sem matar) [CON-011] | — |

Com sessão aberta **e** `detached` vivo, a linha da sessão vale primeiro; `detached` fica para o próximo gatilho (CON-010).

## Efeitos novos

| Efeito | Quem executa | O que faz |
|---|---|---|
| `RequestClose` | coordenador → `Job.RequestClose` / `DetachedJob.RequestClose` | PIDs vivos (`winJob`/`detachedJob`: `JobObjectBasicProcessIdList`; `adoptedJob`: snapshot novo, só os que ainda casam pelo caminho, CON-008); `PostMessageW(WM_CLOSE)` a cada janela de topo visível, sem dono, desses PIDs; nunca `SendMessage`; devolve o número de janelas |
| `KillJob` (executado pelo coordenador **antes** de aplicar o fim, não devolvido pela máquina) | coordenador → `Job.Kill` | `winJob`: `TerminateJobObject(job, 1)`; `adoptedJob`: snapshot novo, `TerminateProcess` dos PIDs vivos que ainda casam pelo caminho, conferindo a imagem; `detachedJob`: `TerminateJobObject` |

## Notas

- `EffEndSession` leva `Reason = closed-by-hotkey` e `EndedAt = closeAt` (Grace: `graceSince`); o estado final segue DOM-008
  (`ended` com ≥ 30 s ativos, senão `discarded`), então um fechamento rápido não soma ao jogo, como qualquer outra sessão curta.
- Depois de `Kill` + `Job.Close`, os eventos do job (`EXIT_PROCESS`, `ACTIVE_PROCESS_ZERO`) não chegam (o leitor para).
- O processo que pediu breakaway (ADR 0016) não está no job: não recebe `WM_CLOSE` nem morre.
- Waiters (`BlockUntilWaiters`): `Closing` soma o `CloseTimeout` (1) aos da sessão; o detached em espera soma `CloseTimeout` e
  `DetachedPoll` (2). Os disparos que chegarem depois do fim são ignorados pela geração.
- Ordem: `session.changed` (efeito `Notify` no dono) é enfileirado antes de o tracker responder; o roteador só envia
  `game.closedByHotkey` depois da resposta (CON-019).
- Recuperação na subida: um agente que caiu em `Closing` deixa a linha `running`/`paused` com o último `last_seen_at`; a regra da
  fase 4 (`recovered`) vale sem mudança.
