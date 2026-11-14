# Fase 4 — Pausa, retomada e "iniciar sem contar tempo": requisitos

- **Status:** aprovado pelo usuário (2026-11-10)
- **Alterado em 2026-11-12 (docs/specs/phase-5-library):** o contrato v0.5.0 deste arquivo é substituído pela v0.6.0
  (IPC-041); os itens de UI-019 e UI-024 ganham o card da grade (UI-029, UI-031).
- **Design:** [design.md](design.md) · **Contratos:** [contracts/ipc.openrpc.json](contracts/ipc.openrpc.json) (v0.5.0,
  substitui a v0.4.0 de [phase-3-tracker](../phase-3-tracker/contracts/ipc.openrpc.json) como fonte da verdade) ·
  [contracts/session-state-machine.md](contracts/session-state-machine.md) (estende a da fase 3 com `Paused`) ·
  **Tarefas:** [tasks.md](tasks.md)
- **Base:** fase 3 ([phase-3-tracker](../phase-3-tracker/requirements.md)), fase 2 e executável único implementados.
- **Objetivo:** pausar e retomar o tempo da sessão ativa por atalho de teclado e de controle, com aviso na tela e som;
  pausar sozinho quando o PC suspende/hiberna (o tempo dormindo nunca conta); gravar as pausas em `pause_interval`;
  mostrar "Pausado" na UI, com um botão "Pausar"/"Retomar" no painel da sessão; e lançar um jogo **sem contar tempo** (botão "Sem contar tempo" na lista).
- **ADRs relacionados:** 0005 (rastreamento em camadas; `Paused` já previsto), 0009 (instantes em ms UTC), 0010 (só o
  agente migra), 0011 (Clock com temporizadores), 0012 (enums validados no `domain`: `paused` e os motivos de pausa
  não pedem `CHECK`), 0013 (log fecha por último), 0014 (executável único), 0015 (agente único escritor), 0016
  (breakaway). **ADRs a escrever pela sessão principal:** ver [ADRs propostos](#adrs-propostos).

Formato: cada requisito tem ID estável (`ÁREA-NNN`), nunca reutilizado. Critérios em Dado/Quando/Então.
Todo ID precisa de pelo menos um teste que o cite no nome (`t.Run("SES-016/...")`, `it("UI-022 ...")`) — exceto os
marcados `verificação: manual`, conferidos pelo reviewer. Instantes nos exemplos: `T` é o `Now()` de um `FakeClock`.
Áreas novas: **PAU** (roteamento do gatilho de pausa no agente), **HOT** (atalho global de teclado), **PAD** (controle
por XInput), **PWR** (suspensão/hibernação), **OVL** (aviso na tela), **SND** (som), **FBK** (coordenação do feedback).

---

## Decisões do usuário (2026-11-09)

Respostas às perguntas de comportamento de produto (não reabrir):

- **Q1 — Combo do controle:** `Select (Back) + Y` segurado por **1 s**, disparando **uma vez por pressão** (precisa
  soltar Back ou Y para disparar de novo). *Afeta:* PAD-001..PAD-003, `x-input.gamepadHoldMs`.
- **Q2 — Ao acordar:** a sessão pausada por suspensão volta a contar **só com usuário presente**
  (`PBT_APMRESUMESUSPEND`). Retomada automática (`PBT_APMRESUMEAUTOMATIC`) não retoma; uma pausa do usuário
  (atalho, controle ou botão) feita antes de dormir continua pausada. Em PCs com Modern Standby o `RESUMESUSPEND`
  pode não chegar (Limitações 1). *Afeta:* SES-028..SES-030, PWR-002, PWR-003, FBK-001.
- **Q3 — "Sem contar tempo" com sessão ativa:** **recusado** com `-32002` quando há qualquer sessão rastreada
  ativa (Running, Grace ou Paused) ou lançamento em andamento. *Afeta:* SES-033, IPC-034, UI-020.
- **Q4 — Botão de pausa na UI (alterado em relação à base):** **sim**, há um botão "Pausar"/"Retomar" no painel da
  sessão. Ele só chama o agente por IPC (`session.pause` / `session.resume`); a UI continua sem calcular nem alterar
  tempo. Promovido a requisitos: SES-035..SES-037, PAU-008, IPC-038..IPC-040, DSK-015, UI-027, UI-028; o motivo `ui`
  entra em DOM-011.

---

## Decisões do usuário (2026-11-10, não reabrir)

1. **Atalhos fixos nesta fase** (sem tela de configuração): teclado `Ctrl+Alt+P`, global, por `RegisterHotKey` numa
   goroutine com `runtime.LockOSThread` rodando o loop de mensagens (dona e esperada, CLAUDE.md); controle
   `Select (Back) + Y`, por polling XInput (~60 Hz, até 4 controles) no agente, segurado por 1 s (Q1).
2. **O atalho alterna** pausa/retomada da sessão rastreada ativa (Running ↔ Paused). Sem sessão ativa, o atalho não faz
   nada (detalhado em PAU-003).
3. **Pausa automática só na suspensão/hibernação do PC** (`WM_POWERBROADCAST`): `PBT_APMSUSPEND` pausa; o tempo dormindo
   nunca conta. Minimizar e inatividade ficam fora (a retomada ao acordar é a Q2).
4. **Feedback:** aviso sempre-no-topo e sem ativar a janela ("Tempo pausado" / "Tempo retomado", alguns segundos) mais
   som curto. Em tela cheia exclusiva o aviso pode não aparecer; o som garante o feedback. O aviso é uma janela Win32
   nativa no agente (sem Wails), dona de uma goroutine com loop de mensagens próprio; o som usa uma API Win32, sem
   dependência nova de terceiros.
5. **"Iniciar sem contar tempo":** segundo botão por jogo na lista, ao lado de "Jogar", rotulado "Sem contar tempo". O
   jogo abre normalmente, mas **não** há sessão (nenhuma linha em `play_session`, nenhum `session.changed`, não aparece
   "Em execução", sem cronômetro). O agente lança o jogo desacoplado do rastreamento.
6. **Modelo de dados:** `pause_interval(id, session_id, paused_at, resumed_at, reason)`; tempo efetivo = fim − início −
   Σ pausas. Esta spec define a migração (schema v4), a recuperação de pausa aberta, a regra de 30 s, o heartbeat em
   pausa, o job esvaziando em pausa e o shutdown em pausa.
7. **UI:** o painel mostra "Pausado" (cronômetro congelado como em Grace) e a linha da lista mostra "Pausado" no lugar
   de "Em execução". O painel tem um botão "Pausar"/"Retomar" (Q4, decidido em 2026-11-09), que só chama o agente por
   IPC. A UI nunca calcula nem influencia o tempo por conta própria.
8. **Testabilidade:** máquina de estados pura (agora com `Paused`); todo tempo por `domain.Clock`/`FakeClock`;
   atalho, XInput, energia, aviso e som atrás de interfaces pequenas com falsos; o que só existe no Windows real tem
   teste de integração ou `verificação: manual`.

## Decisões do spec-writer (a confirmar na aprovação)

Detalhes que não são produto puro; ficam como estão se ninguém objetar.

1. **Heartbeat continua em `Paused`** (grava `last_seen_at`, `state = 'paused'` e o `active_seconds` congelado a cada
   30 s). Assim a recuperação sempre tem um instante de vida do agente dentro da pausa e fecha o intervalo aberto nele
   (SES-020, SES-025). Em Grace o heartbeat continua parado (SES-009).
2. **Job vazio durante a pausa → Grace** (a pausa é fechada no instante do vazio e não é reaberta; se o jogo reaparecer
   na carência, volta a **Running**). Reaproveita a carência em vez de criar um segundo caminho de fim (SES-021).
3. **Pausa aberta é fechada em qualquer fim de sessão**, dentro da mesma transação do `EndSession`, em `ended_at`; se
   `ended_at ≤ paused_at` o intervalo é removido (STO-021). Um intervalo de tamanho zero nunca é gravado.
4. **Pausar não suspende o jogo:** o agente só para de contar. Nenhuma chamada altera os processos (`SuspendThread`,
   `NtSuspendProcess` ficam fora, SES-016).
5. **Feedback só para ação do usuário:** pausa por atalho/controle/botão (`ui`) e **qualquer** retomada (atalho,
   controle, botão, ou acordar com usuário presente) mostram aviso e tocam som; a pausa por suspensão é silenciosa (o PC vai dormir e tocar
   um som atrasaria a suspensão); pausa que vira Grace e fim de sessão não têm feedback (FBK-001).
6. **Antirrepique de 300 ms** entre gatilhos de qualquer fonte (teclado e controle apertados juntos não devem pausar e
   retomar no mesmo instante) (PAU-004).
7. **`game.launchUntracked` é um método à parte, não uma flag de `game.launch`:** o resultado é diferente (`{pid}` em
   vez de `Session`), e uma flag faria o resultado depender de um parâmetro. Erros e validação são os mesmos
   (IPC-033..IPC-035).
8. **Som:** `Beep` (kernel32) com dois pares de tons distintos, numa goroutine própria; não depende do esquema de sons
   do Windows e não pede asset nem dependência (SND-001). **Aviso:** 2,5 s, no topo e centralizado da área de trabalho
   do monitor da janela em primeiro plano (OVL-003, OVL-004).
9. **Migração 4 só adiciona um índice único parcial:** a tabela `pause_interval` já existe desde a migração 1 e fica
   como está (STO-018). Os motivos `idle` e `minimized` da ARCHITECTURE ficam reservados: o `domain` só aceita os que
   já têm origem (`hotkey`, `gamepad`, `sleep`); acrescentar um valor depois não pede migração (ADR 0012).
10. **Gatilho com instante anterior ao início da sessão é ignorado** (um atalho apertado durante o `CreateProcess` não
    pausa uma sessão que ainda nem existia, SES-018).

---

## Requisitos de fases anteriores afetados

| ID | Situação | Por |
|---|---|---|
| STO-001 — Migração em banco novo (fase 2) | alterado (schema na versão 4) | STO-018 |
| STO-011 — Migração 3: motivo de fim da sessão (fase 3) | alterado (`store.SchemaVersion` passa de 3 para 4) | STO-018 |
| AST-001 — Tabela de preferências (single-exe) | alterado (`store.SchemaVersion` passa de 3 para 4; resto mantido) | STO-018 |
| DOM-007 — Valores de estado validados (fase 3) | alterado (`paused` também é aceito) | DOM-010 |
| STO-013 — Atualizar sessão aberta (fase 3) | alterado (o estado gravado pode ser `paused`) | SES-020 |
| STO-014 / STO-015 — Encerrar sessão (fase 3) | alterado (a mesma transação fecha a pausa aberta) | STO-021 |
| SES-002 — Uma sessão por vez (fase 3) | alterado (sessão ativa = Running, Grace **ou Paused**) | SES-031 |
| SES-009 — Heartbeat (fase 3) | alterado (também em Paused; em Grace segue parado) | SES-020 |
| SES-010 — Recuperação na subida (fase 3) | alterado (fecha a pausa aberta) | SES-025 |
| SES-011 — Shutdown com sessão ativa (fase 3) | alterado (caso Paused) | SES-024 |
| SES-013 — Verificação periódica do job (fase 3) | alterado (também em Paused) | SES-022 |
| TRK-008 — Goroutines do tracker (fase 3) | inalterado; o teste cobre também `Close` em Paused | SES-024 |
| AGT-003 / AGT-006 / AGT-010 — Saída ordenada (fases 2 e 3) | alterado (fontes de pausa fecham antes do drain e o feedback fecha depois do tracker; < 2 s mantido) | AGT-013 |
| AGT-009 — Ordem de subida (fase 3) | alterado (fontes de pausa sobem depois do tracker) | AGT-012 |
| IPC-026 — `session.current` (fase 3) | alterado (Running, Grace **ou Paused**) | IPC-036 |
| IPC-027 — `session.changed` (fase 3) | alterado (transições de pausa e retomada) | IPC-037 |
| IPC-030 — Contrato v0.4.0 é a fonte da verdade (fase 3) | substituído | IPC-032 |
| DSK-010 — Repasse de `session.changed` (fase 3) | alterado (`state = paused` repassado) | DSK-014 |
| UI-004 — Lista de jogos (fase 3) | alterado (segundo botão, "Sem contar tempo") | UI-019 |
| UI-006 — Jogo em execução (fase 3) | alterado (`paused` mostra "Pausado", não "Em execução") | UI-024 |
| UI-008 — Erros visíveis (fase 3) | alterado (pendência vale para os dois botões do jogo) | UI-021 |
| UI-012 — Painel da sessão (fase 3) | alterado (existe também em `paused`; ganha o botão "Pausar"/"Retomar") | UI-022, UI-027 |
| UI-017 — Cronômetro só exibe (fase 3) | inalterado: o cronômetro não chama binding; o clique do usuário no botão chama (UI-028) | UI-028 |
| DSK-011 — Mensagens de erro (fase 3) | alterado (código novo `-32005`) | DSK-015 |
| IPC-002 / IPC-008 — Params vazios (fase 2) | valem para `session.pause` e `session.resume` | IPC-038, IPC-039 |
| ARCHITECTURE §4.3 / §4.4 / §10 item 4 | atualizar (diagrama com Paused, atalhos fixos e combo Select+Y, item 4 decidido) | sessão principal |
| Fora de escopo da fase 3: pausa e "iniciar sem contar tempo" | entram nesta fase | — |
| Limitação 7 da fase 3 — suspensão conta como tempo jogado | deixa de valer (exceto se o Windows não avisar, Limitações 12) | SES-026 |

Inalterados e ainda válidos: DOM-001..006, DOM-008, DOM-009, GAM-*, STO-002..STO-012, STO-016, STO-017, SES-001,
SES-003..SES-008, SES-012, SES-014, SES-015, TRK-001..TRK-007, AGT-011, IPC-001..IPC-025, IPC-028..IPC-029,
IPC-031, DSK-012, UI-001, UI-005, UI-007, UI-009..UI-011, UI-013..UI-018, FKG-*. (Os testes de UI-016 e
UI-018 ganham um caso com `paused`; o ID não muda.)

---

## Domínio (DOM)

### DOM-010 — `paused` é estado de sessão válido
Dado o texto de uma coluna, quando chamo `domain.ParseSessionState`, então `running`, `paused`, `grace`, `ended` e
`discarded` são aceitos e qualquer outro valor (inclusive `""` e `Paused`) retorna erro. (ADR 0012)

### DOM-011 — Motivos de pausa validados
Dado o texto de uma coluna, quando chamo `domain.ParsePauseReason`, então `hotkey`, `gamepad`, `ui` e `sleep` são aceitos
e qualquer outro valor retorna erro — inclusive `""`, `Hotkey` e `idle`/`minimized` (reservados, ver decisão 9 do
spec-writer). `PauseReason.UserInitiated()` é verdadeiro para `hotkey`, `gamepad` e `ui` e falso para `sleep`.
`ui` é o motivo da pausa pedida pelo botão do painel (`session.pause`); `pause_interval.reason` é `TEXT` sem `CHECK`
(ADR 0012), então acrescentá-lo não pede migração. (ADR 0012)
· alterado em 2026-11-09: entra `ui` (decisão Q4).

## Store (STO)

### STO-018 — Migração 4: uma pausa aberta por sessão
Dado um banco novo ou na versão 3 com dados, quando o agente abre o store com migração, então existe o índice único
parcial `pause_interval_open_uidx` sobre `pause_interval (session_id) WHERE resumed_at IS NULL`,
`store.SchemaVersion = 4`, a versão do goose é 4 e os dados existentes permanecem; o `Down` remove só o índice. Dado
que se tenta inserir uma segunda linha de `pause_interval` com `resumed_at` nulo para a mesma sessão, então o banco
recusa a inserção.

### STO-019 — Abrir pausa
Dada uma sessão aberta `S`, quando chamo `PauseSession` com instante `P`, motivo `r` e tempo ativo `a`, então numa
única transação existe uma linha em `pause_interval` com `session_id = S`, `paused_at = P`, `resumed_at` nulo e
`reason = r`, e a sessão fica com `state = 'paused'`, `last_seen_at = P`, `active_seconds = floor(a / 1 s)` e
`ended_at` nulo. Dado que já existe pausa aberta de `S`, então o erro embrulha `ErrPauseOpen` e nada muda. Dada uma
sessão inexistente ou já encerrada, então o erro embrulha `ErrSessionNotOpen` e nenhuma linha é criada.

### STO-020 — Fechar pausa
Dada uma pausa aberta desde `P`, quando chamo `ResumePause` com instante `R ≥ P`, estado `s` (`running` ou `grace`) e
tempo ativo `a`, então numa única transação o intervalo fica com `resumed_at = R` e a sessão com `state = s`,
`last_seen_at = R` e `active_seconds = floor(a / 1 s)`. Dado `R` igual a `P` (mesmo milissegundo), então o intervalo
é removido em vez de gravado com tamanho zero. Dado que não há pausa aberta, então só a sessão é atualizada e não há
erro (idempotente); dada uma sessão já encerrada, então o erro embrulha `ErrSessionNotOpen`.

### STO-021 — Encerrar sessão fecha a pausa aberta
Dada uma sessão com pausa aberta desde `P`, quando a encerro com `ended_at = E` (qualquer motivo), então na mesma
transação de STO-014/STO-015 o intervalo fica com `resumed_at = E` se `E > P`, e é removido se `E ≤ P`. Dado que o
fechamento do intervalo falha, então a sessão e o jogo também não mudam.

### STO-022 — Intervalos de uma sessão
Dada uma sessão com pausas, quando chamo `Pauses(sessionID)`, então recebo os intervalos por `paused_at` crescente, com
`PausedAt`, `ResumedAt` (instante zero quando aberto) e `Reason`; sem pausas o resultado é uma lista vazia, nunca nula.
Um `reason` desconhecido gravado fora do Arcadia é devolvido como está (ADR 0012, leitores toleram).

### STO-023 — Tempo ativo recalculável com pausas
Dada uma sessão iniciada em `T`, encerrada com `ended_at = T+300s` e `active_seconds = 230`, com pausas
`[T+100s, T+160s]` e `[T+200s, T+210s]`, então `floor(domain.ActiveDuration(started_at, ended_at, Pauses))` é 230,
igual a `active_seconds`. (Vale para sessões sem carência no meio; `active_seconds` continua sendo o valor
autoritativo, Limitações 8.)

## Sessão (SES)

Os critérios desta seção valem para a máquina de estados pura e para o coordenador do tracker com job, store e relógio
falsos, salvo indicação. Tabela de transições: [contracts/session-state-machine.md](contracts/session-state-machine.md).
"Gatilho" é um pedido de alternar a pausa vindo do atalho (`hotkey`) ou do controle (`gamepad`).

### SES-016 — Pausar uma sessão em Running
Dada uma sessão em Running lançada em `T`, quando chega um gatilho `hotkey` em `T+100s`, então o estado vai para
Paused; a sessão é gravada por `PauseSession` com `paused_at = T+100s`, `reason = hotkey`, `state = 'paused'`,
`last_seen_at = T+100s` e `active_seconds = 100`; é enviada uma notificação com `state = paused`, `activeMs = 100000`
e `asOf = T+100s`; `session.current` em `T+130s` devolve `state = paused`, `activeMs = 100000` e `asOf = T+130s`
(tempo congelado). Nenhum processo do jogo é suspenso nem terminado: o job só tem `ActiveProcesses` consultado.

### SES-017 — Retomar uma sessão pausada
Dada uma sessão em Paused desde `T+100s` com 100 s acumulados, quando chega um gatilho em `T+160s`, então o estado
volta a Running; `ResumePause` grava `resumed_at = T+160s`, `state = 'running'`, `last_seen_at = T+160s` e
`active_seconds = 100`; é enviada uma notificação `running` com `activeMs = 100000` e `asOf = T+160s`; e
`session.current` em `T+170s` devolve `activeMs = 110000`.

### SES-018 — Gatilho fora de Running/Paused é ignorado
Dado o tracker sem sessão, ou uma sessão em Grace, quando chega um gatilho, então nada é gravado nem notificado, não há
feedback (FBK), o estado não muda e é registrado `Debug` "pausa ignorada" com o estado. Dado um gatilho com instante
anterior ao `startedAt` da sessão (apertado durante o lançamento), então ele também é ignorado.

### SES-019 — O tempo ativo exclui as pausas
Dada uma sessão lançada em `T`, pausada em `T+100s`, retomada em `T+160s`, pausada em `T+200s`, retomada em `T+210s`,
com o job vazio em `T+300s` e a carência vencendo em `T+315s`, então a sessão termina com `active_seconds = 230`
(100 + 40 + 90), `ended_at = T+300s` e `state = 'ended'`, o total do jogo aumenta 230 e `Pauses` lista
`[T+100s, T+160s]` e `[T+200s, T+210s]`, ambos `reason = hotkey`.

### SES-020 — Heartbeat continua em Paused
Dada uma sessão lançada em `T` (heartbeats em `T+30s`, `T+60s`, `T+90s`) e pausada em `T+100s`, quando o relógio chega a
`T+120s` e depois a `T+150s`, então a sessão é gravada com `state = 'paused'`, `last_seen_at = T+120s` e depois
`T+150s`, sempre com `active_seconds = 100`; avançar `30 s − 1 ns` não grava. A pausa não abre nem fecha intervalo
nesses heartbeats.

### SES-021 — Job vazio durante a pausa inicia a carência
Dada uma sessão em Paused desde `T+100s` (100 s acumulados), quando o job informa que não há processos ativos em
`T+400s`, então o estado vai para Grace; `ResumePause` grava `resumed_at = T+400s`, `state = 'grace'`,
`last_seen_at = T+400s` e `active_seconds = 100`; a carência de 15 s é armada; é enviada uma notificação `grace` com
`activeMs = 100000` e `asOf = T+400s`; e não há feedback. Dado que a carência vence em `T+415s`, então a sessão termina
com `ended_at = T+400s`, `active_seconds = 100`, motivo `exited` e `state = 'ended'`. Dado que um processo novo aparece
na carência, então o estado volta a **Running** (SES-005) e a pausa não é reaberta.

### SES-022 — Verificação periódica em Paused
Dada uma sessão em Paused cujo job ficou vazio sem a mensagem `ACTIVE_PROCESS_ZERO` chegar, quando a verificação
periódica (5 s) lê 0 processos ativos em `t`, então a sessão vai para Grace em `t`, como em SES-021. Dado que a leitura
traz processos ativos > 0, então a sessão continua em Paused. Erro na leitura é registrado em `Warn` e não muda o estado.

### SES-023 — O descarte usa o tempo ativo
Dada uma sessão lançada em `T`, pausada em `T+20s`, com o job vazio em `T+600s` e a carência vencendo em `T+615s`
(10 min de relógio), então ela termina com `state = 'discarded'` e `active_seconds = 20`, e o jogo não muda
(STO-015). Dadas sessões com 29,999 s e 30 s de tempo ativo (com pausas no meio), então a primeira é `discarded` e a
segunda é `ended` (DOM-008 se aplica ao tempo ativo, nunca à duração no relógio). Os intervalos de pausa de uma sessão
descartada continuam gravados.

### SES-024 — Agente encerrando com sessão pausada
Dada uma sessão em Paused desde `T+100s` (100 s acumulados), quando o agente faz o shutdown ordenado em `T+500s`, então
a sessão é encerrada com `ended_at = T+500s`, `active_seconds = 100` e motivo `agent-shutdown`, a pausa é fechada em
`T+500s` (STO-021), os processos do jogo não são encerrados e não há notificação. `Tracker.Close` retorna só depois de a
goroutine dona terminar (TRK-008) e uma segunda chamada não tem efeito.

### SES-025 — Recuperação com pausa aberta
Dada, no banco, uma sessão aberta com `state = 'paused'`, `last_seen_at = L`, `active_seconds = 100` e pausa aberta
desde `P`, quando o agente sobe, então a sessão é encerrada como em SES-010 (`ended_at = L`, 100 s, motivo `recovered`)
e a pausa é fechada com `resumed_at = L` se `L > P`, ou removida se `L ≤ P`. Exemplos: `P = T+100s` e `L = T+120s` →
intervalo `[T+100s, T+120s]`; `P = L = T+100s` (o agente caiu antes do primeiro heartbeat em pausa) → nenhum intervalo.

### SES-026 — Suspensão pausa a sessão
Dada uma sessão em Running lançada em `T`, quando o Windows avisa a suspensão em `T+100s`, então o estado vai para
Paused com `reason = sleep`; a sessão é gravada como em SES-016 (`paused_at = T+100s`, `active_seconds = 100`); é enviada
uma notificação `paused`; e não há feedback de aviso nem de som (FBK-001).

### SES-027 — Suspensão sem efeito se já pausada ou em carência
Dada uma sessão em Paused por `hotkey`, em Paused por `sleep`, em Grace ou sem sessão, quando chega a suspensão, então
nada é gravado, notificado nem sinalizado e o estado não muda; o intervalo aberto mantém o motivo que tinha.

### SES-028 — Retomada com usuário presente após suspensão
Dada uma sessão em Paused por `sleep` desde `T+100s` (100 s acumulados), quando chega a retomada com usuário presente em
`T+28900s` (8 h depois), então o estado volta a Running; a pausa é fechada em `T+28900s`; `active_seconds` continua 100
(o período dormindo não conta); é enviada uma notificação `running` com `asOf = T+28900s`; há feedback de retomada
(FBK-001); e `session.current` em `T+28910s` devolve `activeMs = 110000`. (Q2.)

### SES-029 — Retomada automática não retoma
Dada uma sessão em Paused por `sleep`, quando chega a retomada **sem** usuário presente (automática), então a sessão
continua Paused: nada é gravado, notificado nem sinalizado. Dado um gatilho `hotkey` depois disso, então a sessão volta
a Running como em SES-017. (Q2.)

### SES-030 — A pausa do usuário sobrevive à suspensão
Dada uma sessão em Paused por `hotkey` (ou `gamepad`), quando chegam a suspensão e depois a retomada com usuário
presente, então a sessão continua Paused com o mesmo intervalo aberto e o motivo original: nada é gravado nem
notificado. (Q2.)

### SES-031 — Sessão pausada bloqueia novo lançamento
Dada uma sessão em Paused, quando peço para lançar um jogo (rastreado ou sem contar tempo), então nenhum processo é
criado, nada é gravado, a sessão não muda e o pedido é recusado com o erro "sessão ativa", que identifica a sessão
(`sessionId`, `gameId`, `title`), como em SES-002.

### SES-032 — Lançamento sem rastrear não cria sessão
Dado o tracker sem sessão e o jogo `G`, quando peço para lançá-lo sem contar tempo, então o processo é criado com o mesmo
`LaunchSpec` de SES-001 (executável entre aspas como primeiro token da linha de comando, `launch_args`, pasta de
trabalho), o PID é devolvido, nenhuma linha de `play_session` é criada, nenhuma notificação é enviada, `Current()`
continua sem sessão, o estado continua sem sessão (um lançamento rastreado logo depois é aceito) e a linha do jogo
(`total_play_seconds`, `play_count`, `last_played_at`, `updated_at`) não muda.

### SES-033 — Lançamento sem rastrear recusado com sessão ativa ou durante o shutdown
Dada uma sessão em Running, Grace ou Paused, ou um lançamento rastreado ainda em andamento, quando peço para lançar sem
contar tempo, então nenhum processo é criado e o pedido é recusado com o erro "sessão ativa" (como em SES-002). Dado o
agente em shutdown (depois de `BeginShutdown`), então nenhum processo é criado e o pedido é recusado com
`ErrShuttingDown`. (Q3.)

### SES-034 — Falha no lançamento sem rastrear
Dado que criar o processo falha, quando peço para lançar sem contar tempo, então nenhuma sessão nem notificação é
criada, é registrado `Error` "falha ao iniciar o jogo" com `game_id`, `path` e `error`, e o pedido é recusado com o erro
"falha no lançamento" contendo o texto do erro e, se houver, o código do Windows (como em SES-012). Nenhum processo fica
vivo nem handle aberto.

### SES-035 — Pausar explicitamente pelo botão da UI
Dada uma sessão em Running lançada em `T`, quando chega o pedido explícito de pausa da UI (`session.pause`) em
`T+100s`, então o estado vai para Paused com `reason = ui`, gravado e notificado como em SES-016 (`paused_at = T+100s`,
`active_seconds = 100`, `activeMs = 100000`, `asOf = T+100s`), e há feedback de pausa (FBK-001). O pedido é idempotente:
dada uma sessão já em Paused (por qualquer motivo, inclusive `sleep`), o pedido não grava, não notifica, não dá
feedback, não altera o intervalo aberto e devolve a sessão atual como sucesso.

### SES-036 — Retomar explicitamente pelo botão da UI
Dada uma sessão em Paused desde `T+100s` (100 s acumulados), por qualquer motivo (`hotkey`, `gamepad`, `ui` ou
`sleep`), quando chega o pedido explícito de retomada da UI (`session.resume`) em `T+160s`, então o estado volta a
Running, gravado e notificado como em SES-017, com feedback de retomada (FBK-001). O pedido é idempotente: dada uma
sessão em Running, o pedido não grava, não notifica, não dá feedback e devolve a sessão atual como sucesso. Diferente
do gatilho de `TogglePause`, o pedido explícito nunca inverte o estado que o usuário já vê.

### SES-037 — Pausa/retomada explícita sem sessão pausável
Dado o tracker sem sessão, ou uma sessão em Grace, quando chega o pedido explícito de pausa ou de retomada, então nada é
gravado, notificado nem sinalizado, o estado não muda e o pedido é recusado com o erro "sessão não pausável", cujo
motivo é `no-session` (sem sessão) ou `grace` (em carência, quando não há tempo correndo nem pausa para retomar).

## Tracker com Job Object (TRK)

Testes de integração Windows com `cmd/fake-game` ([contrato](../phase-3-tracker/contracts/fake-game.md), sem mudança),
relógio falso e espera real limitada para os eventos do job, como na fase 3.

### TRK-009 — Pausa não toca no jogo
Dado o `fake-game` com `--depth 1 --parent-run forever --run forever --until-file F` e a sessão em Running, quando
pauso e retomo a sessão, então os dois processos continuam vivos e dentro do job durante e depois da pausa
(`IsProcessInJob`), e a sessão não perde nenhum evento do job: dado que `F` é criado durante a pausa e os processos
saem, então a sessão vai para Grace (SES-021) e termina.

### TRK-010 — Lançamento sem rastrear fica fora do rastreamento
Dado o `fake-game` com `--run forever --until-file F --report R`, quando o tracker o lança sem contar tempo, então o
processo roda com `cwd` igual à pasta de trabalho e com os argumentos de `launch_args` sem alteração, nenhum job é
criado para ele, nenhuma linha de `play_session` existe, e o processo continua vivo depois de `Tracker.Close`; quando
`F` passa a existir, ele termina. (Se o próprio processo de teste já estiver dentro de um job, o teste confere apenas
que o filho não está num job **diferente** do do pai.)

### TRK-011 — Handles do lançamento sem rastrear fechados
Dado o processo de teste com `GetProcessHandleCount = h0`, quando faço 20 lançamentos sem rastrear de
`fake-game --run 0` e um `Close` do tracker, então a contagem final é `≤ h0 + 2`.

## Atalho de teclado (HOT)

Pacote `internal/input`. Os testes de integração usam uma combinação de teste que não colide com a do usuário (a
combinação padrão fica numa constante e as opções permitem trocá-la).

### HOT-001 — Registro do atalho padrão
Dado o listener criado com as opções padrão, então o atalho é `Ctrl+Alt+P` (`MOD_CONTROL | MOD_ALT | MOD_NOREPEAT`,
tecla virtual `P` = `0x50`). Dado que o listener inicia, então `RegisterHotKey` é chamado na mesma thread do sistema
que roda o loop de mensagens (goroutine com `runtime.LockOSThread`), a combinação fica registrada para o processo e é
registrado `Info` "atalho de pausa registrado" com o atalho em texto.

### HOT-002 — Um gatilho por pressionamento
Dado o listener registrado, quando chega `WM_HOTKEY` com o identificador registrado, então exatamente um
`Trigger{Source: hotkey, At: clock.Now() do recebimento}` é entregue; `WM_HOTKEY` com outro identificador não gera
gatilho. Dada a fila de saída cheia, então o gatilho novo é descartado com `Warn` e o loop de mensagens não bloqueia.

### HOT-003 — Falha no registro
Dado que a combinação já está registrada por outro processo (`ERROR_HOTKEY_ALREADY_REGISTERED`), quando o listener
inicia, então `Start` devolve um erro que embrulha o código do Windows, nenhuma goroutine fica viva e o próprio
`Start` do atalho registra `Error` "atalho de pausa indisponível" com `hotkey` e `winError`; o agente segue (PAU-006).
(Alterado em 2026-11-10, planejamento: quem registra o log é a fonte, não o agente; o teste fica em `internal/input`.)

### HOT-004 — Encerramento do atalho
Dado o listener ativo, quando chamo `Close`, então `UnregisterHotKey` roda na thread dona, a goroutine termina e `Close`
só retorna depois disso (sem goroutines do listener, `goleak`); depois do `Close` outro listener consegue registrar a
mesma combinação; uma segunda chamada a `Close` retorna sem efeito.

### HOT-005 — Atalho com o jogo em primeiro plano
Dado um jogo rastreado em primeiro plano (em janela, em tela cheia sem borda e em tela cheia exclusiva), quando aperto
`Ctrl+Alt+P`, então a sessão alterna entre Running e Paused, o jogo não perde o foco e segurar as teclas por 2 s
alterna uma única vez.
· verificação: manual

## Controle (PAD)

Pacote `internal/input`, sobre uma interface `PadAPI` com falso (a leitura real é `XInputGetState` de
`xinput1_4.dll`). Padrões: leitura a cada 16 ms (~60 Hz), 4 controles, combo `Back` (`0x0020`) + `Y` (`0x8000`)
segurado por 1 s (Q1).

### PAD-001 — Combo segurado dispara
Dado o controle 0 sem botões, quando `Back` e `Y` passam a estar pressionados juntos (primeira leitura que vê os dois,
em `T`) e continuam pressionados, então exatamente um `Trigger{Source: gamepad}` é entregue na primeira leitura com
`agora − T ≥ 1 s`. Exemplo com leituras a cada 16 ms: 62 avanços (`T+992ms`) não disparam; o 63º (`T+1008ms`) dispara.
Outros botões pressionados junto não impedem nem atrasam o disparo.

### PAD-002 — Soltar antes do tempo cancela
Dado `Back` e `Y` pressionados por 900 ms, quando `Y` é solto, e depois os dois são pressionados de novo por 900 ms,
então nenhum gatilho é entregue (a contagem recomeça a cada pressão; só 1 s ininterrupto dispara).

### PAD-003 — Um gatilho por pressão
Dado que o combo disparou, quando `Back` e `Y` continuam pressionados por mais 10 s, então nenhum outro gatilho é
entregue; quando `Back` ou `Y` é solto e os dois são pressionados de novo por 1 s, então um novo gatilho é entregue.

### PAD-004 — Controles independentes
Dados os controles 0 e 1, quando `Back` está pressionado só no 0 e `Y` só no 1, então nenhum gatilho é entregue (os
botões não se combinam entre controles). Dado que qualquer um dos 4 controles completa o combo por 1 s, então ele
entrega um gatilho; dois controles que completam juntos entregam um gatilho cada (o antirrepique de PAU-004 os junta).

### PAD-005 — Desconexão zera a contagem
Dado o combo pressionado há 600 ms, quando a leitura do controle falha com `ERROR_DEVICE_NOT_CONNECTED`, então a
contagem é zerada; quando ele reconecta com o combo ainda pressionado, então são necessários mais 1 s inteiro para
disparar.

### PAD-006 — Cadência da leitura
Dado um controle conectado e três slots desconectados, quando o poller inicia, então cada slot é lido uma vez; depois
de 62 avanços de 16 ms (`992 ms`) o slot conectado foi lido 63 vezes e os desconectados continuam com 1 leitura cada;
no 63º avanço (`1008 ms`) cada slot desconectado é sondado de novo (2 leituras), porque slots desconectados só são
sondados a cada 1 s (sondar slot vazio a 60 Hz é caro no XInput). Um controle ligado é percebido em até 1 s.

### PAD-007 — XInput indisponível
Dado que `xinput1_4.dll` não carrega, então criar o `PadAPI` real devolve erro; a fábrica de plataforma do Windows
registra `Warn` "controle indisponível (XInput)" e o agente segue (PAU-006), sem tocar no atalho de teclado.
(Alterado em 2026-11-10, planejamento: quem registra o log é a fábrica de plataforma, não o agente.)

### PAD-008 — Encerramento do poller
Dado o poller ativo, quando chamo `Close`, então a goroutine termina (sem goroutines do poller, `goleak`), `Close` só
retorna depois disso, sem depender de o relógio avançar, e uma segunda chamada não tem efeito.

### PAD-009 — Controle real
Dado um controle XInput real (com fio e sem fio) e um jogo rastreado em tela cheia, quando seguro `Select + Y` por 1 s,
então a sessão alterna entre Running e Paused; um toque curto não alterna; cada um dos controles 1..4 funciona;
desligar o controle sem fio no meio da pressão não alterna.
· verificação: manual

## Suspensão e hibernação (PWR)

Pacote `internal/winapi` (`PowerWatcher`): janela oculta de topo própria (não `HWND_MESSAGE`, que não recebe
`WM_POWERBROADCAST`) com loop de mensagens em thread travada, como o `SessionEndWatcher`. Os testes de integração
injetam `WM_POWERBROADCAST` na própria janela.

### PWR-001 — Suspensão notificada
Dado o watcher iniciado, quando a janela recebe `WM_POWERBROADCAST` com `wParam = PBT_APMSUSPEND` (`0x0004`), então
`OnSuspend(at)` é chamado uma vez com `at = clock.Now()` do recebimento e o procedimento da janela devolve `TRUE`.

### PWR-002 — Retomada com usuário presente
Dado o watcher iniciado, quando a janela recebe `PBT_APMRESUMESUSPEND` (`0x0007`), então `OnWake(at, userPresent=true)`
é chamado uma vez com `at = clock.Now()` do recebimento.

### PWR-003 — Retomada automática
Dado o watcher iniciado, quando a janela recebe `PBT_APMRESUMEAUTOMATIC` (`0x0012`), então
`OnWake(at, userPresent=false)` é chamado uma vez. (O Windows envia este evento em toda retomada, antes de
`RESUMESUSPEND`; a distinção é a da Q2.)

### PWR-004 — Outros eventos de energia são ignorados
Dado o watcher iniciado, quando a janela recebe qualquer outro `wParam` de `WM_POWERBROADCAST` (por exemplo
`PBT_APMPOWERSTATUSCHANGE` `0x000A`, `PBT_APMBATTERYLOW` `0x0009`, `PBT_APMRESUMECRITICAL` `0x0006`,
`PBT_POWERSETTINGCHANGE` `0x8013`), então nenhum callback é chamado e o procedimento da janela devolve `TRUE`.

### PWR-005 — A suspensão não espera o tracker
Dado um tracker que demora a responder, quando chega `PBT_APMSUSPEND`, então o tratamento do agente espera no máximo
1,5 s (`PowerHandlerTimeout`), registra `Warn` "tracker não respondeu à suspensão a tempo" e devolve; a pausa continua
valendo com o instante `at` do evento (a goroutine dona do tracker a aplica quando puder, sem tomar o instante da
retomada).
(Alterado em 2026-11-10, planejamento: o canal de controle do tracker tem buffer e o chamador só abandona o envio se o
tracker fechar, nunca pelo fim do `ctx`; assim o `Suspend` é mesmo aplicado depois, com o instante original.)

### PWR-006 — Ciclo de vida do watcher de energia
Dado o watcher, quando chamo `Close`, então o loop termina, a janela e a classe são destruídas na própria thread, `Close`
só retorna depois disso (sem goroutines do watcher, `goleak`) e uma segunda chamada não tem efeito; depois de 20 ciclos
de `Start`/`Close` a contagem de handles do processo é `≤ h0 + 2` e a de objetos de USER/GDI (`GetGuiResources`) não
cresce mais que 2.

### PWR-007 — Suspensão e hibernação reais
Dado um jogo rastreado, quando o PC suspende e depois acorda por ação do usuário, e quando hiberna e acorda, então a
sessão fica Paused durante o sono, existe um intervalo `reason = sleep` de `[suspensão, acordar]`, o `active_seconds`
não inclui o sono (±2 s) e a UI mostra "Pausado" e volta a "Em execução" (Q2); dado um acordar sem usuário (tarefa
agendada que acorda o PC), então a sessão continua Paused; em um PC com Modern Standby o resultado é anotado
(Limitações 1).
· verificação: manual

## Aviso na tela (OVL)

Pacote `internal/feedback` (interface `Overlay`, coordenação) e `internal/winapi` (janela Win32). O aviso é uma janela
nativa do agente, sem Wails.

### OVL-001 — Textos do aviso
Dado `Show(NoticePaused)`, então o texto exibido é exatamente "Tempo pausado"; dado `Show(NoticeResumed)`, então é
exatamente "Tempo retomado".

### OVL-002 — Janela no topo, sem ativar e sem clique
Dada a janela de aviso criada, então ela é `WS_POPUP` com os estilos estendidos `WS_EX_TOPMOST`, `WS_EX_NOACTIVATE`,
`WS_EX_TOOLWINDOW` (fora do Alt+Tab e da barra de tarefas), `WS_EX_LAYERED` e `WS_EX_TRANSPARENT` (o clique atravessa) e
não é visível até o primeiro `Show`. Dado que `Show` roda com outra janela em primeiro plano, então a janela de aviso
fica visível, a janela em primeiro plano e a que tem o foco do teclado continuam as mesmas. (Em máquina sem área de
trabalho interativa o teste é pulado e o requisito é conferido no OVL-007.)

### OVL-003 — Duração e substituição
Dado `Show` em `t`, então o aviso é escondido quando `t + 2,5 s` chega (em `t + 2,5 s − 1 ns` continua visível). Dado um
segundo `Show` em `t + 1 s` com outro texto, então o texto passa a ser o novo, o aviso não é escondido em `t + 2,5 s` e
é escondido em `t + 3,5 s`, uma única vez.

### OVL-004 — Posição
Dada a área de trabalho `[left, top, right, bottom]` do monitor da janela em primeiro plano (ou do monitor principal se
não houver janela em primeiro plano) e a janela de aviso `w × h`, então ela fica centralizada na horizontal e a 48 px
do topo (48 × dpi/96 px): para a área `(0, 0, 1920, 1040)` e `240 × 56` a 96 dpi, o canto superior esquerdo é
`(840, 48)`; para a área `(1920, 0, 3840, 1040)` é `(2760, 48)`; a 144 dpi (`360 × 84`) é `(780, 72)` na primeira
área.

### OVL-005 — Ciclo de vida do aviso
Dado o aviso, quando chamo `Close`, então a janela é escondida e destruída, a classe é desregistrada e a fonte e os
pincéis são liberados, tudo na thread dona; `Close` só retorna depois de a goroutine terminar (sem goroutines do aviso,
`goleak`); `Show` depois do `Close` não tem efeito nem erro; depois de 100 ciclos de `Show`/esconder a contagem de
objetos de USER/GDI não cresce mais que 2 e a de handles do processo, mais que 2.

### OVL-006 — Falha do aviso não impede a pausa nem o som
Dado que criar a janela de aviso falha, então é registrado `Warn` "aviso na tela indisponível", `Show` passa a não ter
efeito e o som (SND) e a pausa continuam funcionando (PAU-006).

### OVL-007 — Aviso com jogo real
Dado um jogo rastreado em janela e em tela cheia sem borda, quando pauso e retomo, então o aviso aparece sobre o jogo
sem roubar o foco, no topo e centralizado, por ~2,5 s; em tela cheia exclusiva o aviso pode não aparecer e o som toca
(SND-003).
· verificação: manual

## Som (SND)

### SND-001 — Dois sinais distintos
Dado `Play(CuePaused)`, então são tocados dois tons seguidos: 660 Hz por 90 ms e 440 Hz por 120 ms (descendente); dado
`Play(CueResumed)`, então 440 Hz por 90 ms e 660 Hz por 120 ms (ascendente). Os tons saem por uma função injetável (no
Windows, `Beep` de `kernel32`). `Play` volta imediatamente, mesmo com a função de tom demorando 500 ms.

### SND-002 — Ciclo de vida do som
Dado o som, quando chamo `Close`, então a goroutine termina e `Close` só retorna depois disso (sem goroutines do som,
`goleak`); `Play` depois do `Close` não tem efeito; dado `Play` enquanto um sinal anterior ainda toca, então no máximo um
sinal fica pendente (o mais novo substitui o pendente).

### SND-003 — Som audível
Dado um jogo rastreado em primeiro plano (inclusive em tela cheia exclusiva), quando pauso e retomo, então ouço o sinal
descendente na pausa e o ascendente na retomada.
· verificação: manual

## Feedback (FBK)

Pacote `internal/feedback`: coordenador portátil (uma goroutine dona) sobre `Overlay` e `Sound`, com falsos nos testes.

### FBK-001 — Quando há feedback
Dada a máquina de estados, então o tracker emite `FeedbackPaused` na pausa por `hotkey`, `gamepad` ou `ui`, e
`FeedbackResumed` na retomada por gatilho, por `session.resume` e na retomada com usuário presente após suspensão; não
emite nada na pausa por `sleep` (SES-026), na entrada em Grace vinda de Paused (SES-021), no encerramento, nem em
gatilho, evento ou pedido ignorado ou idempotente (SES-018, SES-027, SES-029, SES-035, SES-036, SES-037). Decisão: a
pausa e a retomada pelo botão da UI dão o mesmo aviso e o mesmo som do atalho (a ação é do usuário, e com a UI em
foco o som e o aviso confirmam que o agente aplicou o pedido).
· alterado em 2026-11-09: inclui o motivo `ui`.

### FBK-002 — O coordenador mostra o aviso e toca o som
Dado `FeedbackPaused`, então o coordenador chama `Overlay.Show(NoticePaused)` e `Sound.Play(CuePaused)` uma vez cada;
dado `FeedbackResumed`, `Show(NoticeResumed)` e `Play(CueResumed)`. Dado que um dos dois falha ou está ausente, então o
outro ainda é chamado, o erro é registrado em `Warn` e nada é propagado ao tracker.

### FBK-003 — O feedback nunca atrasa o tracker
Dado um `Overlay.Show` que demora 5 s (relógio real), quando o tracker pausa e retoma, então as notificações e as
gravações da sessão continuam dentro do prazo normal do teste. O callback `Config.Feedback` não bloqueia: a fila do
coordenador tem capacidade 4 e, cheia, descarta o evento mais antigo com `Warn`.

### FBK-004 — Eventos em sequência
Dado `FeedbackPaused` seguido de `FeedbackResumed` com 100 ms de diferença, então os dois são processados na ordem e o
aviso final mostra "Tempo retomado".

### FBK-005 — Encerramento do feedback
Dado o coordenador, quando chamo `Close`, então ele esconde o aviso, fecha o `Overlay` e o `Sound`, espera a goroutine
terminar (sem goroutines do coordenador, `goleak`) e uma segunda chamada não tem efeito; eventos enviados depois do
`Close` são descartados sem pânico.

## Roteamento do gatilho de pausa (PAU)

Pacote `internal/agent` (`pauseRouter`): lê os gatilhos de uma fila única alimentada pelo atalho e pelo controle.

### PAU-001 — O atalho alterna a sessão
Dado `Trigger{Source: hotkey, At: t}` com a sessão em Running, então o roteador chama `Tracker.TogglePause` com motivo
`hotkey` e instante `t`, e a sessão vai para Paused com intervalo `reason = hotkey` (SES-016); com a sessão em Paused,
a chamada a retoma (SES-017).

### PAU-002 — O controle alterna a sessão
Dado `Trigger{Source: gamepad, At: t}`, então o roteador chama `Tracker.TogglePause` com motivo `gamepad`, e o intervalo
gravado tem `reason = gamepad`.

### PAU-003 — Sem sessão ativa, nada acontece
Dado o tracker sem sessão (ou em Grace), quando chega um gatilho, então nada é gravado, notificado nem sinalizado
(sem aviso e sem som) e é registrado `Debug` "gatilho de pausa ignorado". O atalho continua registrado no sistema
(Limitações 2).

### PAU-004 — Antirrepique entre fontes
Dados dois gatilhos (da mesma fonte ou de fontes diferentes) com instantes `At` separados por menos de 300 ms, então só
o primeiro é encaminhado ao tracker; separados por exatamente 300 ms, o segundo também é encaminhado. A janela é medida
a partir do último gatilho encaminhado.

### PAU-005 — Gatilho durante o shutdown é ignorado
Dado o agente depois de `BeginShutdown`, quando chega um gatilho, então ele é descartado (o tracker devolve
`ErrShuttingDown`, registrado em `Debug`), sem pânico e sem gravação; `Close` do roteador só retorna depois de a
goroutine terminar (sem goroutines do roteador, `goleak`).

### PAU-006 — Falha de um componente não derruba o agente
Dado que o atalho, o controle, o watcher de energia, o aviso ou o som falha ao iniciar (erro injetado), então o agente
registra o erro do componente (as fontes de gatilho registram o próprio erro detalhado, HOT-003/PAD-007; o agente
registra `Warn` genérico "fonte de gatilho de pausa indisponível" com `source` e `error`), sobe normalmente, os outros componentes funcionam e o rastreamento de tempo não muda.

### PAU-007 — Pausa de ponta a ponta com jogo real
Dado um jogo real aberto por "Jogar", quando pauso pelo atalho e retomo, pauso pelo controle e retomo, e por fim fecho o
jogo, então: a UI mostra "Pausado" no painel e na linha e o cronômetro para; o aviso e o som aparecem em cada
troca; ao retomar o cronômetro volta a andar do valor em que parou; o tempo total do jogo ao fim não inclui o tempo
pausado (±2 s); `pause_interval` tem um intervalo por pausa, com `hotkey` e `gamepad`.
· verificação: manual

### PAU-008 — Pedidos da UI não passam pelo antirrepique
Dados `session.pause`/`session.resume` chegando em qualquer intervalo de um gatilho de atalho ou de controle (por
exemplo, 100 ms depois), então o pedido da UI é sempre aplicado (não é descartado pela janela de 300 ms) e não atualiza
o "último gatilho encaminhado" de PAU-004; ele também não é descartado por ser repetido, porque é idempotente
(SES-035, SES-036). Dado um gatilho de atalho 100 ms depois de um pedido da UI, então o gatilho é encaminhado
normalmente (a janela de PAU-004 só conta gatilhos de atalho e controle).

## Agente (AGT)

### AGT-012 — Ordem de subida com a pausa
Dado o agente subindo, então a ordem é: store abre e migra → recuperação de sessões (SES-010/SES-025) → coordenador de
feedback → tracker (com `Config.Feedback`) → servidor IPC → roteador de gatilhos → fontes (atalho, controle, energia);
nenhuma fonte entrega gatilho a um tracker que ainda não iniciou. Dado um componente de feedback ou de fonte
indisponível, então vale PAU-006.

### AGT-013 — Ordem de shutdown com a pausa
Dado o agente com a sessão em Paused, quando o shutdown ordenado começa (por qualquer motivo de EXE-016), então a ordem
é: `Tracker.BeginShutdown` → fontes (atalho, controle, energia) fechadas e roteador esperado → drain (`agent.shutdown`)
→ `Server.Close` → `Tracker.Close` (encerra a sessão por SES-024) → coordenador de feedback → `Store.Close`; e o agente
termina em < 2 s (AGT-003), sem nenhum feedback no encerramento.

## IPC (IPC)

Contrato: [contracts/ipc.openrpc.json](contracts/ipc.openrpc.json) v0.5.0.

### IPC-032 — Contrato v0.5.0 é a fonte da verdade
· substituído por IPC-041 (docs/specs/phase-5-library): a fonte da verdade passa a ser o contrato v0.6.0.
Dado `docs/specs/phase-4-pause/contracts/ipc.openrpc.json`, quando o teste de contrato roda, então a versão é `0.5.0`,
todos os métodos e notificações dele existem no servidor/`internal/ipc` com os mesmos nomes (inclusive
`game.launchUntracked`), os campos obrigatórios dos resultados batem com os tipos Go, o enum `Session.state` contém
`running`, `paused`, `grace`, `ended` e `discarded`, e os códigos de erro batem com as constantes; o comentário do
pacote `ipc` aponta para este arquivo.

### IPC-033 — `game.launchUntracked`
Dado um jogo existente e nenhuma sessão ativa, quando chamo `game.launchUntracked` com `{"gameId": G}`, então recebo
`{"pid": N}` com `N ≥ 1`, o processo do jogo está criado, `session.current` continua `{"session": null}` e nenhuma
notificação `session.changed` é enviada.

### IPC-034 — Erros de `game.launchUntracked`
Dado um `gameId` inexistente, quando chamo `game.launchUntracked`, então recebo `-32001` "Game not found" (a existência é
conferida antes da sessão ativa); dada uma sessão em Running, Grace ou Paused, ou um lançamento em andamento, então
recebo `-32002` "Session already active" com `data.sessionId`, `data.gameId` e `data.title`; dada uma falha do
lançamento (SES-034), o agente em shutdown (`error = "agente encerrando"`) ou um `launch_type` diferente de `exe`
(`error = "tipo de lançamento não suportado"`), então recebo `-32003` "Launch failed" com `data.error` e, se houver,
`data.winError`. Dado que o prazo do handler (10 s) se esgota antes da resposta, então recebo `-32603` "Internal error"
(o processo pode já ter sido criado).

### IPC-035 — Params inválidos em `game.launchUntracked`
Quando chamo `game.launchUntracked` com `gameId` de tipo errado (texto, fracionário, `< 1`), com array posicional, sem
`gameId` ou com membro desconhecido, então recebo `-32602`, como em IPC-028. Params vazios (`{}`, `[]`, `null`,
ausente) também são `-32602`, porque `gameId` é obrigatório.

### IPC-036 — `session.current` com sessão pausada
Dada uma sessão em Paused desde `T+100s` (100 s acumulados), quando chamo `session.current` em `T+130s`, então recebo
`{"session": Session}` com `state = "paused"`, `activeMs = 100000`, `activeSeconds = 100` e `asOf = T+130s` (IPC-031; o
tempo ativo não anda em Paused); dada nenhuma sessão, continua `{"session": null}`.

### IPC-037 — `session.changed` em pausa e retomada
Dados dois clientes conectados, quando a sessão vai de Running para Paused, de Paused para Running, ou de Paused para
Grace, então cada cliente recebe uma notificação `session.changed` com `params.session` no estado novo (`paused`,
`running`, `grace`), uma por transição, na ordem, com `asOf` igual ao instante do evento (a pausa: o instante do
gatilho; a retomada por acordar: o instante de `RESUMESUSPEND`). Gatilho ou evento de energia ignorado não gera
notificação, nem o lançamento sem contar tempo. Pedidos explícitos idempotentes (IPC-038, IPC-039) também não geram.

### IPC-038 — `session.pause`
Dada uma sessão em Running, quando chamo `session.pause` (sem params), então recebo `{"session": Session}` com
`state = "paused"`, `activeMs` congelado e `asOf` do instante do pedido, e todos os clientes recebem `session.changed`
`paused` (SES-035). Dada uma sessão já em Paused, então recebo `{"session": Session}` com o estado atual (sucesso, sem
notificação). Params não vazios → `-32602` (IPC-002); params vazios (`{}`, `[]`, `null`, ausente) são aceitos (IPC-008).

### IPC-039 — `session.resume`
Dada uma sessão em Paused, quando chamo `session.resume` (sem params), então recebo `{"session": Session}` com
`state = "running"` e todos os clientes recebem `session.changed` `running` (SES-036). Dada uma sessão já em Running,
então recebo `{"session": Session}` com o estado atual (sucesso, sem notificação). Mesma regra de params de IPC-038.

### IPC-040 — Erro "sessão não pausável"
Dado o agente sem sessão ou com a sessão em Grace, quando chamo `session.pause` ou `session.resume`, então recebo
`-32005` "Session not pausable" com `data.reason` ∈ {`no-session`, `grace`} (SES-037). Dado o agente em shutdown (depois
do início do drain), então recebo `-32603` "Internal error" e nada é aplicado (o tracker devolve `ErrShuttingDown`,
registrado em `Debug`). Dado que o prazo do handler (5 s) se esgota, então recebo
`-32603`.

## Desktop (DSK)

### DSK-013 — Binding `LaunchGameUntracked`
Quando chamo `LaunchGameUntracked(id)`, então a UI chama `game.launchUntracked` com `{"gameId": id}` (prazo de 12 s),
devolve sucesso sem valor e não emite `session:changed`; os erros usam as mesmas mensagens de DSK-011 (inclusive
`-32002` → "Já há um jogo em andamento: «title». Feche-o antes de iniciar outro." e `-32003` → "Não foi possível iniciar
o jogo: «error»").

### DSK-015 — Bindings `PauseSession` e `ResumeSession`
Quando chamo `PauseSession()` ou `ResumeSession()`, então a UI chama `session.pause` / `session.resume` (prazo de 5 s)
e devolve a sessão recebida no formato de DSK-010 (`SessionView`), sem emitir evento por conta própria (o evento
`session:changed` vem da notificação do agente); os erros usam DSK-011, que ganha: `-32005` com `reason = no-session` →
"Não há jogo em andamento para pausar."; `-32005` com `reason = grace` → "O jogo está encerrando. Não dá para pausar
agora."; `-32005` com `data` ausente ou motivo desconhecido → "Não foi possível pausar agora.". As demais mensagens de
DSK-011 continuam.

### DSK-014 — Repasse de sessão pausada
Dada a UI conectada, quando recebe `session.changed` com `state = "paused"`, então emite o evento Wails
`session:changed` com a sessão convertida (`state = "paused"`, `activeSeconds`, `activeMs`, `asOf`, `endedAt = ""`);
`CurrentSession` devolve a sessão pausada no mesmo formato; um estado desconhecido continua sendo registrado em `Warn`
e não é repassado (DSK-010).

## Frontend (UI)

### UI-019 — Botão "Sem contar tempo"
· alterado em 2026-11-12 (docs/specs/phase-5-library, UI-031): os dois botões também ficam no card da grade e na linha da lista.
· alterado em 2026-11-14 (docs/specs/phase-10-nova-interface, UIX-027): os dois botões ficam só no painel de detalhes do jogo em foco; este passa a se chamar "Jogar sem contar tempo".
Dada a lista de jogos, então cada linha mostra, ao lado do botão "Jogar", um botão "Sem contar tempo".

### UI-020 — Lançar sem contar tempo
Quando clico "Sem contar tempo" no jogo `G`, então a UI chama `LaunchGameUntracked(G)` uma vez (e não `LaunchGame`);
dado sucesso, então nenhum painel de sessão aparece, nenhuma linha mostra "Em execução" ou "Pausado", o cronômetro não
existe e a lista não é recarregada; dado erro (por exemplo `-32002` com uma sessão ativa), então o aviso do topo mostra
a mensagem de DSK-011 com "Fechar", e a próxima ação bem-sucedida o remove, como em UI-008. (Q3.)

### UI-021 — Lançamento pendente bloqueia os dois botões do jogo
· alterado em 2026-11-14 (docs/specs/phase-10-nova-interface, UIX-027): vale para os dois botões do painel de detalhes.
Dado um lançamento pendente do jogo `G` (por qualquer um dos botões), então os dois botões de `G` ficam desabilitados e o
botão acionado mostra "Iniciando…" (o outro mantém o rótulo); os botões dos outros jogos continuam habilitados.

### UI-022 — Painel com sessão pausada
Dada uma sessão em `paused` (lida de `CurrentSession` ao carregar ou recebida por `session:changed`) com
`activeMs = A`, então o painel de sessão existe (UI-012) com o título e o cronômetro mostrando `floor(A / 1000)` parado
(avançar o tempo falso não o muda), o painel mostra "Pausado" e não mostra "Encerrando…". Exemplo: `A = 100000` →
"0:01:40", ainda "0:01:40" depois de 10 min falsos.

### UI-023 — Retomada volta a contar
Dada uma sessão em `paused` mostrada com "Pausado", quando chega `session:changed` com `running` e `activeMs = A`,
então "Pausado" some e o cronômetro volta a avançar a partir de `A` (UI-014). Exemplo: `A = 100000` → "0:01:40";
avançar 1000 ms → "0:01:41". Quando chega `grace` (job esvaziou em pausa), então o painel mostra "Encerrando…"
congelado (UI-015) e não "Pausado".

### UI-024 — Linha do jogo pausado
· alterado em 2026-11-12 (docs/specs/phase-5-library, UI-029): a marcação vale também para o card da grade.
Dada uma sessão em `paused`, então a linha do jogo dela mostra "Pausado" no lugar de "Em execução"; em `running` ou
`grace` continua "Em execução"; quando chega `ended` ou `discarded` a marcação some e a lista é recarregada (UI-006).

### UI-025 — Cronômetro de pausa só exibe
Dado o painel em `paused`, quando 10 min passam no relógio falso, então nenhum binding é chamado por causa do painel e
nenhum temporizador do cronômetro fica armado (UI-017).

### UI-027 — Botão "Pausar"/"Retomar" no painel
Dado o painel da sessão (UI-012), então ele mostra um botão cujo rótulo depende só do estado recebido do agente:
"Pausar" em `running`, "Retomar" em `paused`; em `grace` o botão existe, mostra "Pausar" e fica desabilitado; enquanto
uma chamada de pausa/retomada está pendente o botão fica desabilitado (sem duplo clique). Dado `session:changed` com o
estado novo, então o rótulo muda para o do estado novo, e o botão volta a habilitar quando a chamada termina.

### UI-028 — O botão só chama o agente
Quando clico "Pausar", então a UI chama `PauseSession` uma vez; quando clico "Retomar", `ResumeSession` uma vez; a UI
não altera o estado, o cronômetro nem o rótulo por conta própria: tudo muda quando chega `session:changed` (ou a
resposta da chamada, aplicada só se nenhum evento chegou desde o pedido, como em UI-018). Dado um erro (por exemplo
`-32005`, conexão perdida), então o aviso do topo mostra a mensagem de DSK-015/DSK-011 com "Fechar", o painel não muda
e a próxima ação bem-sucedida remove o aviso (UI-008). A UI nunca soma nem subtrai tempo: o cronômetro continua sendo
calculado só de `activeMs` (UI-014, UI-022).

### UI-026 — Ponta a ponta de pausa e sem contar tempo
Dado o Arcadia aberto, quando abro um jogo por "Sem contar tempo", jogo e fecho, então nenhuma sessão aparece e o total
do jogo não muda; quando abro outro por "Jogar" e pauso pelo atalho, então o painel e a linha mostram "Pausado" com o
cronômetro parado, o aviso e o som aparecem, e ao retomar o cronômetro volta a andar do mesmo valor; dado que fecho e
reabro a UI durante a pausa, então o painel reaparece em "Pausado" com o valor certo (±1 s); dado que clico "Retomar" e
depois "Pausar" no painel, então o aviso e o som também aparecem e o `pause_interval` tem `reason = ui`.
· verificação: manual

---

## Fora de escopo
Configuração de atalhos, combo ou duração do combo (tela de configurações; valores ficam em constantes) · botão
de pausa fora do painel da sessão (linha da lista, bandeja) · outros botões do controle, botão Guide, DirectInput/HID
e controles que não sejam XInput ·
pausa automática por janela minimizada ou por inatividade (os motivos `minimized` e `idle` ficam reservados) ·
**suspender os processos do jogo** (a pausa só para de contar) · detecção de suspensão por lacuna de tempo quando o
Windows não avisa (Limitações 12) · persistir a pausa entre quedas do agente · indicação de pausa na bandeja ·
cronômetro ou nome do jogo no aviso · aviso em tela cheia exclusiva · edição manual de pausas e estatísticas de
pausa na UI · "iniciar sem contar tempo" em apps adicionais (`additional_app.track_time`), em menu de contexto ou por
atalho · registro (histórico) de lançamentos sem contar tempo · remover/editar jogo (fase 5).

## Limitações conhecidas
A confirmar na aprovação; não geram requisito nem teste.

1. **Modern Standby (S0 low power idle):** em PCs que o usam, `PBT_APMSUSPEND` e `RESUMESUSPEND` podem não chegar ou
   chegar em momentos diferentes do esperado. Sem `SUSPEND` a sessão **não** é pausada e o sono conta (ver 12); sem
   `RESUMESUSPEND` ela fica pausada até o atalho. PWR-007 registra o comportamento de cada máquina testada.
2. **O atalho é reservado para o Arcadia em todo o sistema enquanto o agente roda:** `RegisterHotKey` consome
   `Ctrl+Alt+P` mesmo sem sessão ativa, e outros programas deixam de recebê-lo. Se outro programa já o usa, o registro
   falha e só o log (HOT-003) avisa: o controle e o botão do painel (UI-027) continuam funcionando e o atalho de teclado, não.
3. **Sem sessão ativa o atalho é silencioso** (PAU-003): quem aperta com o jogo aberto por "Sem contar tempo" ou por
   fora do Arcadia não vê nada.
4. **Pausar não suspende o jogo:** ele continua rodando, com som e rede; só o relógio da sessão para.
5. **Som e aviso são melhor esforço:** o aviso pode não aparecer em tela cheia exclusiva e o `Beep` pode não tocar se
   o jogo mantiver o dispositivo de áudio em modo exclusivo; um dos dois costuma bastar.
6. **Queda do agente durante a pausa:** a sessão é recuperada em `last_seen_at` (até 30 s antes) e a pausa não é
   restaurada (SES-025).
7. **Sessão pausada indefinidamente fica aberta:** não há tempo-limite; o tempo não conta, e o jogo fechar encerra a
   sessão pela carência (SES-021).
8. **`pause_interval` é registro auxiliar de melhor esforço:** `active_seconds` é o valor autoritativo. Uma falha de
   escrita da pausa (SES-014) pode deixar um intervalo faltando ou mais longo que o real; o total do jogo não é afetado.
9. **Steam Input e remapeadores** podem mudar quais controles o XInput enxerga; PAD-009 verifica em máquina real.
10. **Controles que não são XInput** (DualShock/DualSense sem Steam Input ou DS4Windows) não disparam o combo.
11. **Lançamentos sem contar tempo não deixam estado:** "Jogar" no mesmo jogo (ou em outro) depois de abrir um jogo por
    "Sem contar tempo" é aceito e abre uma segunda instância sem aviso (Q3).
12. **Suspensão não avisada conta como tempo jogado:** se o Windows não entregar `PBT_APMSUSPEND` (queda de energia,
    hibernação crítica, Modern Standby), o sono entra no tempo ativo até a próxima retomada.
13. **Gatilho durante o lançamento:** apertado nos ~100 ms do `CreateProcess` é descartado (SES-018).

## ADRs propostos
A sessão principal escreve (a spec só os referencia). Numeração sugerida a partir de 0017.

1. **0017 — Modelo de pausa:** pausa não suspende o jogo; tempo ativo acumulado em memória e gravado em
   `active_seconds`; `pause_interval` como registro auxiliar com no máximo uma pausa aberta por sessão (índice único
   parcial); heartbeat segue em Paused; pausa aberta fechada pelo `EndSession` na mesma transação; job vazio em pausa
   vira Grace; política de suspensão e retomada (Q2); motivo `ui` e pedidos explícitos idempotentes.
2. **0018 — Entrada global e feedback nativo no agente:** atalho fixo por `RegisterHotKey` e combo XInput (Q1) como
   constantes; threads/goroutines donas (hotkey, poller, energia, aviso, som); aviso Win32 nativo + `Beep`, sem Wails e
   sem dependência nova.
3. **0019 — Lançamento sem rastrear como método à parte:** `game.launchUntracked` (resultado `{pid}`), processo
   desacoplado (sem Job Object, sem sessão), recusado com sessão ativa (Q3).
