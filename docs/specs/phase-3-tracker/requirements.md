# Fase 3 — Tracker MVP: requisitos

- **Status:** aprovado pelo usuário (2026-11-09)
- **Design:** [design.md](design.md) · **Contratos:** [contracts/ipc.openrpc.json](contracts/ipc.openrpc.json) (v0.4.0,
  substitui a v0.3.0 de [single-exe](../single-exe/contracts/ipc.openrpc.json) como fonte da verdade) ·
  [contracts/session-state-machine.md](contracts/session-state-machine.md) ·
  [contracts/fake-game.md](contracts/fake-game.md) · **Tarefas:** [tasks.md](tasks.md)
- **Base:** fase 2 ([phase-2-skeleton](../phase-2-skeleton/requirements.md)) e executável único
  ([single-exe](../single-exe/requirements.md)) implementados.
- **Objetivo:** lançar um `.exe` pelo agente, rastrear a árvore de processos com Job Object (Camada 1), gravar sessões
  com heartbeat, carência e descarte, atualizar o tempo total do jogo e mostrar isso numa UI mínima (adicionar jogo,
  lista, "Jogar", jogo em execução com cronômetro ao vivo). `cmd/fake-game` e testes de integração.
- **ADRs relacionados:** 0005 (rastreamento em camadas — aqui só a Camada 1), 0009 (instantes em ms UTC), 0010 (só o
  agente migra), 0011 (Clock com temporizadores), 0012 (enums validados no `domain`), 0013 (log fecha por último),
  0014 (executável único), 0015 (o agente é o único escritor do `library.db`; a UI escreve via IPC — ver
  [design.md](design.md#quem-escreve-no-banco)), 0016 (o job do jogo permite breakaway, TRK-006).

Formato: cada requisito tem ID estável (`ÁREA-NNN`), nunca reutilizado. Critérios em Dado/Quando/Então.
Todo ID precisa de pelo menos um teste que o cite no nome (`t.Run("SES-004/...")`, `it("UI-005 ...")`) — exceto os
marcados `verificação: manual`, conferidos pelo reviewer. Instantes nos exemplos: `T` é o `Now()` de um `FakeClock`.

### Decisões do usuário (não reabrir)
1. Escopo: só a Camada 1 (Job Object com IOCP; processo criado suspenso, associado ao job e só então retomado; sem
   `KILL_ON_JOB_CLOSE`). Watcher (Camada 2), sinais de loja (Camada 3), pausa (fase 4) e "iniciar sem contar tempo"
   ficam fora.
2. UI mínima: "Adicionar jogo" com seletor de `.exe` (título padrão = nome do exe, editável), lista simples com
   "Jogar" e tempo total, indicando o jogo em execução. A fase 5 a substitui. O lançamento acontece no agente.
3. Um jogo rastreado por vez: lançar outro com uma sessão ativa (Launching/Running/Grace) é recusado com erro claro.
4. Padrões (constantes agora): carência 15 s; sessão < 30 s é descartada; heartbeat de `last_seen_at` a cada 30 s;
   na subida, sessões abertas são fechadas em `last_seen_at`. O timeout de 90 s de Launching não se aplica à Camada 1
   (o primeiro processo existe assim que `CreateProcess` retorna): Launching → Failed só por falha do lançamento
   (ver [máquina de estados](contracts/session-state-machine.md)).
5. Máquina de estados da ARCHITECTURE §4.3 sem Paused; tempo efetivo = segundos ativos; ao fim de uma sessão não
   descartada atualizar `total_play_seconds`, `last_played_at`, `play_count` e `first_played_at`.
6. `cmd/fake-game` com flags especificadas como contrato.
7. IPC para adicionar/listar jogos, lançar, consultar a sessão atual; notificação de mudança de sessão; códigos de erro.
8. Tempo via `domain.Clock`; máquina de estados pura e testável sem Windows; integração com `fake-game`.

---

## Decisões do usuário (2026-11-09)

- **P1 — Shutdown com jogo rodando:** a sessão é encerrada no instante do shutdown (`end_reason = agent-shutdown`);
  o jogo continua e o tempo depois disso não conta (SES-011).
- **P2 — Mesmo `.exe` mais de uma vez:** permitido; cada adição cria um jogo distinto (GAM-001). Não há erro de
  executável repetido.
- **P3 — Formato do total:** "Nunca jogado", "< 1 min", "N min", "H h MM min" (UI-005).
- **P4 — Carência não conta** como tempo jogado; `ended_at` é o início da carência (SES-006).
- **P5 — Remover/editar jogo:** fora desta fase (fase 5).
  · alterado em 2026-11-12 (docs/specs/phase-5-library): entra na fase 5 (EDT-*, DEL-*).
- **P6 — Breakaway:** o job usa `JOB_OBJECT_LIMIT_BREAKAWAY_OK`, sem `SILENT_BREAKAWAY_OK` (ADR 0016, TRK-006).
- **P7 — Cronômetro ao vivo:** a UI mostra o jogo em sessão e o tempo jogado, avançando a cada segundo, inclusive
  quando aberta no meio da sessão; é só exibição, calculado na UI a partir de `activeMs`/`asOf` do agente, e nunca
  altera o tempo gravado (IPC-031, UI-012..UI-018).

---

## Requisitos de fases anteriores afetados

| ID | Situação | Por |
|---|---|---|
| STO-001 — Migração em banco novo (fase 2) | alterado (schema na versão 3: `play_session.end_reason`) | STO-011 |
| AST-001 — Tabela de preferências (single-exe) | alterado (`store.SchemaVersion` passa de 2 para 3; resto mantido) | STO-011 |
| AGT-003 — Saída ordenada (fase 2) | alterado (o shutdown encerra a sessão ativa antes de fechar o store; < 2 s mantido) | AGT-010, SES-011 |
| AGT-006 — Encerramento com a sessão do Windows (fase 2) | alterado (mesmo shutdown de AGT-010) | AGT-010 |
| UI-002 — Estado vazio da biblioteca (fase 2) | substituído (estado vazio vem de `game.list`, sem contadores) | UI-010 |
| Contrato IPC v0.3.0 (single-exe) | substituído como fonte da verdade | IPC-030 |
| ARCHITECTURE §2 — "a UI é dona da biblioteca" | alterado (o agente é o único escritor nesta fase; ADR 0015) | GAM-001, design.md |

Inalterados e ainda válidos: BLD-001, PTH-*, DOM-001..006, STO-002..010, IPC-001..019, AGT-001..002 (substituídos
como antes), AGT-004..005, DSK-*, UI-001, EXE-*, AST-002..011. DSK-007/DSK-008 continuam valendo para o binding
`LibrarySummary`, que a tela deixa de usar nesta fase (design.md). · alterado em 2026-11-12 (docs/specs/phase-5-library, DSK-019): o binding `LibrarySummary` sai de vez.

---

## Domínio (DOM)

### DOM-007 — Valores de estado e de motivo de fim validados · alterado em 2026-11-10: `paused` também é aceito (docs/specs/phase-4-pause, DOM-010)
Dado o texto de uma coluna, quando chamo `domain.ParseSessionState`, então `running`, `grace`, `ended` e `discarded`
são aceitos e qualquer outro valor (inclusive `""` e `Running`) retorna erro; quando chamo
`domain.ParseSessionEndReason`, então `exited`, `agent-shutdown` e `recovered` são aceitos e qualquer outro valor
retorna erro. (ADR 0012)

### DOM-008 — Regra de descarte
Dado o tempo ativo `a` de uma sessão encerrada, quando aplico a regra de descarte, então o estado final é `discarded`
se `a < 30 s` e `ended` se `a ≥ 30 s`. Exemplos: `0` → `discarded`; `29,999 s` → `discarded`; `30 s` → `ended`;
`2 h` → `ended`.

### DOM-009 — Título padrão do jogo
Dado o caminho de um executável, quando calculo o título padrão, então ele é o nome do arquivo sem a extensão `.exe`
(sem diferenciar maiúsculas), aceitando `\` e `/` como separadores em qualquer sistema. Exemplos:
`C:\Jogos\Hollow Knight\hollow_knight.exe` → `hollow_knight`; `D:\x\Game.EXE` → `Game`;
`C:\a\my.game.exe` → `my.game`; `C:\a\b.exe.exe` → `b.exe`.

## Biblioteca de jogos (GAM)

### GAM-001 — Adicionar jogo grava a linha
· alterado em 2026-11-12 (docs/specs/phase-5-library, FTS-002): a linha de `game_fts` é gravada na mesma transação.
· alterado em 2026-11-13 (docs/specs/phase-9-plataformas, PLT-027): a plataforma deixa de ser `PC` e passa a ser a de
chave `windows` (criada como "Windows" se não existir); vale PLT-027.
Dado um executável válido `P` (absoluto, existente, arquivo regular, extensão `.exe`) e o título `X`, quando o agente
adiciona o jogo, então existe uma linha em `game` com `title = X`, `launch_type = 'exe'`,
`launch_target = filepath.Clean(P)`, `working_dir` = pasta de `P`, `launch_args = ''`, `total_play_seconds = 0`,
`play_count = 0`, `first_played_at` e `last_played_at` nulos e `created_at = updated_at = T`, ligada à plataforma
`PC` (categoria `pc`), que é criada na primeira adição e reutilizada nas seguintes. Dado um jogo já cadastrado com
o mesmo `P` (ou o mesmo caminho com outras maiúsculas), quando adiciono `P` de novo, então nada é recusado: é criada
outra linha com `id` diferente, as duas aparecem em `game.list` e cada uma tem seu próprio total e suas sessões
(decisão P2).

### GAM-002 — Executável inválido
Dado um caminho que não é absoluto, não existe, não é um arquivo regular ou não termina em `.exe` (sem diferenciar
maiúsculas), quando peço para adicioná-lo, então nada é gravado e o erro indica o motivo, respectivamente
`not-absolute`, `not-found`, `not-a-file` ou `not-exe`.

### GAM-003 — Título
Dado o título informado, quando adiciono o jogo, então o título gravado é o informado sem espaços nas pontas; se ele
for ausente ou ficar vazio, então é o título padrão de DOM-009; se tiver mais de 200 caracteres (runas) depois do
corte, então nada é gravado e o pedido é recusado como parâmetro inválido.

### GAM-005 — Ordem da lista
· alterado em 2026-11-12 (docs/specs/phase-5-library, LIB-001): é a ordem `sort = title` de `game.list`; a regra não muda.
Dados jogos com títulos `b`, `A`, `a` e `C` (ids 1..4), quando listo os jogos, então a ordem é por título sem
diferenciar maiúsculas e, em empate, por id: `A`(2), `a`(3), `b`(1), `C`(4).

## Store (STO)

### STO-011 — Migração 3: motivo de fim da sessão · alterado em 2026-11-10: SchemaVersion 3 → 4 (docs/specs/phase-4-pause, STO-018)
Dado um banco novo ou na versão 2 com dados, quando o agente abre o store com migração, então `play_session` tem a
coluna `end_reason TEXT` (nula), `store.SchemaVersion = 3`, a versão do goose é 3 e os dados existentes permanecem
(sessões antigas ficam com `end_reason` nulo).

### STO-012 — Abrir sessão
Dado um jogo `G`, quando abro uma sessão em `T`, então existe uma linha em `play_session` com `game_id = G`,
`started_at = last_seen_at = T`, `active_seconds = 0`, `state = 'running'`, `source = 'tracked'`, `ended_at` e
`end_reason` nulos, e o id devolvido é o dela.

### STO-013 — Atualizar sessão aberta · alterado em 2026-11-10: o estado gravado também pode ser `paused` (docs/specs/phase-4-pause, SES-020)
Dada uma sessão aberta, quando gravo estado `s`, `last_seen_at = L` e tempo ativo `a`, então a linha passa a ter
esses valores, com `active_seconds = floor(a / 1 s)`, e `ended_at` continua nulo.

### STO-014 — Encerrar sessão contada atualiza o jogo na mesma transação · alterado em 2026-11-10: a mesma transação fecha a pausa aberta (docs/specs/phase-4-pause, STO-021)
Dada uma sessão aberta do jogo `G` com `started_at = S`, quando a encerro com `ended_at = E`, tempo ativo `a ≥ 30 s`,
motivo `r` e instante de escrita `W`, então numa única transação: a sessão fica com `state = 'ended'`, `ended_at = E`,
`last_seen_at = E`, `active_seconds = floor(a / 1 s)` e `end_reason = r`; e `G` fica com `total_play_seconds`
somado de `floor(a / 1 s)`, `play_count + 1`, `last_played_at = E`, `first_played_at` mantido se já existia ou `S`
se era nulo, e `updated_at = W`. Dado que a atualização do jogo falha, então a sessão também não muda.

### STO-015 — Encerrar sessão descartada não altera o jogo · alterado em 2026-11-10: a mesma transação fecha a pausa aberta (docs/specs/phase-4-pause, STO-021)
Dada uma sessão aberta, quando a encerro com tempo ativo `a < 30 s`, então ela fica com `state = 'discarded'`,
`ended_at`, `last_seen_at`, `active_seconds` e `end_reason` gravados como em STO-014, e a linha do jogo não muda
(nem `updated_at`).

### STO-016 — Total recalculável
Dado um jogo novo, quando encerro em sequência sessões com tempos ativos 10 s, 45 s, 30 s e 3600 s, então
`total_play_seconds = 3675`, `play_count = 3` e `total_play_seconds` é igual à soma de `active_seconds` das sessões
do jogo com `state = 'ended'`.

### STO-017 — Sessões abertas
Dado um banco com sessões encerradas e abertas (`ended_at` nulo), quando listo as sessões abertas, então recebo só as
abertas, com `id`, `game_id`, `started_at`, `last_seen_at` e `active_seconds`.

## Sessão (SES)

Os critérios desta seção valem para a máquina de estados pura e para o coordenador do tracker com job, store e
relógio falsos, salvo indicação. Tabela de transições: [contracts/session-state-machine.md](contracts/session-state-machine.md).

### SES-001 — Lançamento abre sessão em Running
Dado o tracker sem sessão e o jogo `G`, quando peço para lançá-lo em `T` e o processo é criado e associado ao job,
então a sessão é gravada como em STO-012 **antes** de o processo ser retomado, o processo é retomado, o estado é
Running e o lançamento devolve a sessão (`state = running`, `startedAt = T`, `activeSeconds = 0`).

### SES-002 — Uma sessão por vez · alterado em 2026-11-10: sessão ativa = Running, Grace ou Paused (docs/specs/phase-4-pause, SES-031)
Dada uma sessão em Running ou Grace (do mesmo jogo ou de outro), ou um lançamento ainda em andamento, quando peço
para lançar um jogo, então nenhum processo é criado, nada é gravado, a sessão existente não muda e o pedido é
recusado com o erro "sessão ativa", que identifica a sessão existente (`sessionId`, `gameId`, `title`).

### SES-003 — Conjunto vazio inicia a carência
Dada uma sessão em Running desde `T`, quando o job informa que não há mais processos ativos em `T+100s`, então o
estado vai para Grace, a sessão é gravada com `state = 'grace'`, `last_seen_at = T+100s` e `active_seconds = 100`,
e um temporizador de 15 s é armado.

### SES-004 — Fim da carência encerra a sessão
Dada uma sessão em Grace desde `E`, quando 15 s passam sem processo novo no job, então a sessão é encerrada com
`ended_at = E` e motivo `exited` (STO-014/STO-015 conforme DOM-008), o job é fechado, não há mais sessão atual e um
novo lançamento é aceito. Avançar `15 s − 1 ns` não encerra.

### SES-005 — Processo novo na carência volta a Running
Dada uma sessão em Grace desde `E`, quando o job informa um processo novo em `E+5s`, então o estado volta a Running,
a sessão é gravada com `state = 'running'` e `last_seen_at = E+5s`, e avançar mais 15 s não encerra a sessão.

### SES-006 — Tempo ativo é a soma dos intervalos em Running
Dada uma sessão lançada em `T`, quando o job fica vazio em `T+100s`, recebe um processo novo em `T+105s`, fica vazio
de novo em `T+200s` e a carência vence em `T+215s`, então a sessão termina com `active_seconds = 195`,
`ended_at = T+200s` e `state = 'ended'`, e o total do jogo aumenta 195.

### SES-007 — Sessão curta é descartada
Dada uma sessão lançada em `T`, quando o job fica vazio em `T+29s` e a carência vence, então a sessão termina com
`state = 'discarded'` e `active_seconds = 29`, e o jogo não muda (STO-015).

### SES-008 — Sessão contada aparece no total do jogo
Dado um jogo com total 0, quando uma sessão dele termina com 120 s ativos, então `game.list` devolve o jogo com
`totalPlaySeconds = 120`, `playCount = 1` e `lastPlayedAt` igual ao `ended_at` da sessão.

### SES-009 — Heartbeat a cada 30 s em Running · alterado em 2026-11-10: também em Paused; em Grace segue parado (docs/specs/phase-4-pause, SES-020)
Dada uma sessão em Running desde `T`, quando o relógio avança 30 s e depois mais 30 s, então a sessão é gravada com
`last_seen_at = T+30s`/`active_seconds = 30` e depois `T+60s`/`60`; avançar `30 s − 1 ns` não grava. Dada uma sessão
em Grace, então nenhum heartbeat é gravado até ela voltar a Running ou terminar.

### SES-010 — Sessões abertas são fechadas na subida · alterado em 2026-11-10: fecha também a pausa aberta (docs/specs/phase-4-pause, SES-025)
Dadas, no banco, sessões abertas (deixadas por queda do agente ou falta de energia), quando o agente sobe, então,
antes de aceitar conexões IPC, cada uma é encerrada com `ended_at = last_seen_at`, o `active_seconds` gravado,
motivo `recovered` e a regra de DOM-008 (STO-014/STO-015), e é registrado `Warn` "sessões abertas recuperadas" com
`count`. Exemplo: `active_seconds = 590` → `ended`, total do jogo +590; `active_seconds = 20` → `discarded`.

### SES-011 — Agente encerrando com sessão ativa · alterado em 2026-11-10: caso Paused (docs/specs/phase-4-pause, SES-024)
Dada uma sessão em Running, quando o agente faz o shutdown ordenado em `T`, então a sessão é encerrada com
`ended_at = T`, o tempo ativo até `T` e motivo `agent-shutdown`; dada uma sessão em Grace desde `E`, então ela é
encerrada com `ended_at = E` e motivo `exited`. Em ambos os casos os processos do jogo não são encerrados.

### SES-012 — Falha no lançamento
Dado que criar o processo, associá-lo ao job ou gravar a sessão falha, quando peço para lançar, então o processo (se
chegou a ser criado) é terminado sem nunca ter sido retomado, nenhuma sessão fica gravada, nenhuma notificação é
enviada, o estado volta a sem sessão, é registrado `Error` "falha ao iniciar o jogo" com `game_id`, `path` e `error`,
e o pedido é recusado com o erro "falha no lançamento" contendo o texto do erro e, se houver, o código do Windows.
Dado que só a retomada do processo falha, então o processo é terminado e a sessão gravada é apagada, com o mesmo
resultado.

### SES-013 — Verificação periódica do job · alterado em 2026-11-10: também em Paused (docs/specs/phase-4-pause, SES-022)
Dada uma sessão em Running cujo job ficou vazio sem a mensagem `ACTIVE_PROCESS_ZERO` chegar, quando a verificação
periódica (a cada 5 s) lê 0 processos ativos em `t`, então a sessão vai para Grace em `t` (como em SES-003). Dada uma
sessão em Grace, quando a verificação lê processos ativos > 0 em `t`, então ela volta a Running em `t` (como em
SES-005). Erro na leitura é registrado em `Warn` e não muda o estado.

### SES-014 — Falha de escrita durante a sessão
Dado que gravar um heartbeat ou a entrada em Grace falha, quando ocorre, então é registrado `Error` "falha ao gravar a
sessão" com `session_id` e `error` e a sessão continua no estado novo (a próxima escrita tenta de novo). Dado que
gravar o encerramento falha, então é registrado o mesmo `Error`, o job é fechado e o tracker volta a sem sessão; a
linha continua aberta e é recuperada na próxima subida (SES-010).

### SES-015 — Fim do processo inicial no log
Dada uma sessão cujo processo inicial termina com código `N` (outros processos podem continuar), quando o job informa
a saída dele, então é registrado `Info` "processo inicial do jogo encerrou" com `session_id`, `pid` e `exit_code = N`,
e o estado da sessão não muda por isso.

## Tracker com Job Object (TRK)

Testes de integração Windows com `cmd/fake-game` ([contrato](contracts/fake-game.md)), relógio falso para a carência
e espera real limitada para os eventos do job.

### TRK-001 — Processo entra no job antes de executar
Dado o `fake-game` com `--depth 1 --parent-run forever --run forever --until-file F`, quando o tracker o lança, então
o processo inicial e o filho que ele cria logo ao começar estão no job (`IsProcessInJob`), e o job conta 2
processos novos.

### TRK-002 — Lançador que sai mantém a sessão
Dado o `fake-game` no padrão lançador (`--depth 1 --parent-run 0 --run forever --until-file F`), quando o processo
inicial sai e o filho continua, então a sessão continua em Running; quando crio `F` e o filho sai, então a sessão vai
para Grace.

### TRK-003 — Árvore de três níveis
Dado o `fake-game` com `--depth 2 --fanout 2 --parent-run 0 --run forever --until-file F` (7 processos), quando os
intermediários saem e as 4 folhas continuam, então a sessão continua em Running até a última folha sair.

### TRK-004 — Fechar o tracker não mata o jogo
Dada uma sessão em Running com o `fake-game` vivo, quando o tracker é fechado (shutdown do agente), então todos os
processos do jogo continuam vivos depois do `Close` (sem `KILL_ON_JOB_CLOSE`); o teste os encerra criando `F`.

### TRK-005 — Linha de comando e pasta de trabalho
Dado um jogo com `launch_target = E` (caminho com espaços) e `working_dir = D`, quando o tracker o lança, então o
`fake-game` relata `cwd = D` e recebe os argumentos de `launch_args` sem alteração; `E` vai entre aspas como primeiro
token da linha de comando.

### TRK-006 — Filho que pede breakaway escapa do job (ADR 0016)
Dado o `fake-game` com `--depth 1 --breakaway --parent-run 2s --run forever --until-file F`, quando o processo inicial
cria o filho com `CREATE_BREAKAWAY_FROM_JOB`, então a criação funciona, o filho fica fora do job, e a sessão vai para
Grace quando o processo inicial sai, mesmo com o filho vivo.

### TRK-007 — Handles fechados
Dado o processo de teste com `GetProcessHandleCount = h0`, quando faço 20 lançamentos de `fake-game --run 0` com
sessões que terminam (carência vencida) e um `Close` do tracker, então a contagem final é `≤ h0 + 2`.

### TRK-008 — Goroutines do tracker
Dado um tracker iniciado, com ou sem sessão ativa, quando chamo `Close`, então ele só retorna depois que as goroutines
do coordenador e do leitor da porta de conclusão terminaram (`goleak` sem goroutines do tracker), e uma segunda
chamada a `Close` retorna sem efeito.

## Agente (AGT)

### AGT-009 — Ordem de subida · alterado em 2026-11-10: fontes de pausa sobem depois do tracker (docs/specs/phase-4-pause, AGT-012)
Dado o agente subindo, quando o store abre e migra, então a recuperação de SES-010 roda e o tracker é iniciado antes
de o servidor IPC aceitar conexões; dada a recuperação falhando, então é registrado `Error` e a subida continua.

### AGT-010 — Ordem de shutdown com sessão · alterado em 2026-11-10: fontes de pausa fecham antes do drain e o feedback depois do tracker (docs/specs/phase-4-pause, AGT-013)
Dado o agente com uma sessão ativa, quando o shutdown ordenado começa (por qualquer motivo de EXE-016), então a ordem
é: drain (`agent.shutdown`) → `Server.Close` → `Tracker.Close` (encerra a sessão por SES-011) → `Store.Close`, e o
agente termina em < 2 s (AGT-003).

### AGT-011 — Lançamento recusado durante o shutdown
Dado o agente em shutdown (depois do início do drain), quando chega `game.launch`, então nenhum processo é criado e o
pedido é recusado com o erro "falha no lançamento" e `error = "agente encerrando"`.

## IPC (IPC)

Contrato: [contracts/ipc.openrpc.json](contracts/ipc.openrpc.json) v0.4.0.

### IPC-020 — `game.add`
Dado um executável válido, quando chamo `game.add` com `{"path": P, "title": X}`, então recebo o `Game` gravado
(`id`, `title`, `path`, `totalPlaySeconds = 0`, `playCount = 0`, `firstPlayedAt = null`, `lastPlayedAt = null`).

### IPC-021 — `game.add` com executável inválido
Dado um caminho inválido (GAM-002), quando chamo `game.add`, então recebo o erro `-32004` "Invalid executable" com
`data.reason` ∈ {`not-absolute`, `not-found`, `not-a-file`, `not-exe`}.

### IPC-023 — `game.list`
· substituído por IPC-042 (docs/specs/phase-5-library): `game.list` passa a ser paginado e filtrado, com `{games, total, offset}`.
Dados jogos cadastrados, quando chamo `game.list`, então recebo `{"games": [...]}` na ordem de GAM-005, com os campos
de `Game`; dado nenhum jogo, então `games` é `[]` (nunca `null`).

### IPC-024 — `game.launch` com sucesso
Dado um jogo existente e nenhuma sessão ativa, quando chamo `game.launch` com `{"gameId": G}`, então recebo a
`Session` aberta (`state = "running"`, `endedAt = null`, `activeSeconds = 0`).

### IPC-025 — Erros de `game.launch`
Dado um `gameId` inexistente, quando chamo `game.launch`, então recebo `-32001` "Game not found" (a existência do
jogo é conferida antes da sessão ativa); dada uma sessão ativa, então recebo `-32002` "Session already active" com
`data.sessionId`, `data.gameId` e `data.title`; dada uma falha de lançamento (SES-012, AGT-011), então recebo `-32003`
"Launch failed" com `data.error` (texto) e `data.winError` (inteiro, só quando houver código do Windows).
Dado que o prazo do handler (10 s) se esgota antes de o lançamento terminar, então recebo `-32603` "Internal error";
o lançamento pode ainda concluir na goroutine dona do tracker e o resultado real chega só por `session.changed`.
· alterado em 2026-11-09 (planejamento): explicita o `-32603` por prazo do handler, já previsto no contrato; sem
mudança de comportamento.

### IPC-026 — `session.current` · alterado em 2026-11-10: Running, Grace ou Paused (docs/specs/phase-4-pause, IPC-036)
Dada uma sessão em Running ou Grace, quando chamo `session.current`, então recebo `{"session": Session}` com o estado
atual e o tempo ativo medido no instante da chamada (`asOf`, IPC-031); dada nenhuma sessão, então recebo
`{"session": null}`.

### IPC-027 — Notificação `session.changed` · alterado em 2026-11-10: também nas transições de pausa e retomada (docs/specs/phase-4-pause, IPC-037)
Dados dois clientes conectados, quando a sessão muda de estado (abre em Running, entra em Grace, volta a Running,
termina como `ended` ou `discarded`), então cada cliente recebe uma notificação `session.changed` (sem `id`) com
`params.session` no estado novo, uma por transição, na ordem das transições. Não há notificação para lançamento que
falhou, para sessões recuperadas na subida nem para o encerramento feito depois de `Server.Close` (SES-011).

### IPC-028 — Params inválidos nos métodos novos
· alterado em 2026-11-12 (docs/specs/phase-5-library, IPC-043): `game.list` passa a aceitar params; as mesmas regras de tipo valem para ele.
Quando chamo `game.add` ou `game.launch` com params de tipo errado (ex.: `gameId` texto,
fracionário ou `< 1`; `path` ausente ou não texto), com array posicional ou com membro desconhecido, então recebo
`-32602`. `game.list` e `session.current` seguem IPC-008 (params vazios aceitos) e IPC-002 (não vazios → `-32602`).

### IPC-029 — Envio de notificação a todos os clientes
Dado um `ipc.Server` com clientes conectados, quando chamo `Notify(method, params)`, então cada conexão recebe a
notificação; dada uma conexão com a fila de saída cheia, então só ela perde a notificação, com log `Warn`, e as
outras recebem.

### IPC-030 — Contrato v0.4.0 é a fonte da verdade

· alterado em 2026-11-10: substituído por IPC-032 (docs/specs/phase-4-pause).
Dado `docs/specs/phase-3-tracker/contracts/ipc.openrpc.json`, quando o teste de contrato roda, então a versão é
`0.4.0`, todos os métodos e notificações dele existem no servidor/`internal/ipc` com os mesmos nomes, os campos
obrigatórios dos resultados batem com os tipos Go e os códigos de erro batem com as constantes; o comentário do
pacote `ipc` aponta para este arquivo.

### IPC-031 — Tempo ativo com instante de referência
Dada uma `Session` devolvida por `game.launch`, `session.current` ou `session.changed`, então ela traz `activeMs`
(tempo ativo em milissegundos, truncado) e `asOf` (instante do relógio do agente em que `activeMs` foi medido, RFC 3339
UTC com milissegundos), lidos na mesma chamada a `Clock.Now()`, e `activeSeconds = floor(activeMs / 1000)`. Exemplos
com `FakeClock`: sessão em Running desde `T`, `session.current` em `T+252,4s` → `activeMs = 252400`,
`activeSeconds = 252`, `asOf = T+252,4s`; a mesma sessão em Grace desde `T+300s`, `session.current` em `T+310s` →
`activeMs = 300000`, `asOf = T+310s`; a notificação de uma transição em `t` tem `asOf = t`.

## Desktop (DSK)

### DSK-010 — Repasse de `session.changed` · alterado em 2026-11-10: `state = paused` repassado (docs/specs/phase-4-pause, DSK-014)
Dada a UI conectada, quando recebe `session.changed`, então emite o evento Wails `session:changed` com a sessão
convertida (`sessionId`, `gameId`, `title`, `state`, `startedAt`, `endedAt` ou `""`, `activeSeconds`, `activeMs`,
`asOf`); params inválidos são registrados em `Warn` e não são repassados. `CurrentSession` e `LaunchGame` devolvem a
sessão no mesmo formato.

### DSK-011 — Mensagens de erro em português · alterado em 2026-11-10: código novo `-32005` (docs/specs/phase-4-pause, DSK-015)
Dado um erro de binding, quando o converto para o texto mostrado ao usuário, então:
`-32001` → "Jogo não encontrado. Atualize a lista."; `-32002` → "Já há um jogo em andamento: «title». Feche-o antes de
iniciar outro."; `-32003` → "Não foi possível iniciar o jogo: «error»"; `-32004` com `not-absolute`/`not-found` →
"Arquivo não encontrado.", `not-a-file` → "O caminho escolhido não é um arquivo.", `not-exe` → "Escolha um arquivo
.exe."; `-32602` → "Dados inválidos."; sem conexão, conexão perdida, cliente fechado ou prazo esgotado → "O agente
não está respondendo. Tente de novo."; qualquer outro → "Erro
inesperado. Veja o log.". Erros que não são de `-32002`..`-32004` nem de conexão são registrados em `Error`.
Isso inclui o `-32603` de `game.launch` por prazo do handler (IPC-025) e o de `game.add` quando o `stat` do
executável falha com erro diferente de "não existe" (ex.: acesso negado): ambos mostram "Erro inesperado. Veja o
log."; no caso do lançamento, o estado real chega depois por `session:changed`.
· alterado em 2026-11-09 (planejamento): explicita dois casos de `-32603` já cobertos por "qualquer outro"; sem
mudança de comportamento.
· alterado em 2026-11-09 (testes da T12): «title» e «error» marcam só o valor inserido; as aspas angulares não
fazem parte do texto (ex.: "Já há um jogo em andamento: Hollow Knight. Feche-o antes de iniciar outro."). Com `data`
ausente ou inválido: `-32002` → "Já há um jogo em andamento. Feche-o antes de iniciar outro."; `-32003` → "Não foi
possível iniciar o jogo."; `-32004` com motivo desconhecido → "Dados inválidos.". `context.Canceled` é tratado
como conexão ("O agente não está respondendo. Tente de novo.").

### DSK-012 — Seletor de executável
> **Removido na fase 6:** `PickGameExe` não é mais usado pela UI; o seletor de arquivos do Importar é `PickFile` (IMP-004).

Quando chamo `PickGameExe`, então abre o diálogo de arquivo com título "Escolha o executável do jogo" e filtro
"Executáveis (*.exe)" / `*.exe`; dado que o usuário cancela, então o resultado é `{path: "", title: ""}` e nada é
enviado ao agente; dado um arquivo escolhido `P`, então o resultado é `{path: P, title: DOM-009(P)}`; dado que o
diálogo falha, então o erro é registrado em `Error` e o binding devolve "Erro inesperado. Veja o log.".
· alterado em 2026-11-09 (testes da T12): falha do diálogo definida.

## Frontend (UI)

### UI-004 — Lista de jogos · alterado em 2026-11-10: segundo botão "Sem contar tempo" (docs/specs/phase-4-pause, UI-019)
· substituído por UI-029 e UI-030 (docs/specs/phase-5-library): a grade de cards e a lista alternativa.
Dada a lista de `game.list` não vazia, então a tela mostra uma linha por jogo, na ordem recebida, com o título, o
tempo total (UI-005) e o botão "Jogar".

### UI-005 — Formato do tempo total
Dado `totalPlaySeconds`, então o texto é: `0` → "Nunca jogado"; `1` e `59` → "< 1 min"; `60` → "1 min"; `3599` →
"59 min"; `3600` → "1 h 00 min"; `3900` → "1 h 05 min"; `451800` → "125 h 30 min".

### UI-006 — Jogo em execução · alterado em 2026-11-10: `paused` mostra "Pausado", não "Em execução" (docs/specs/phase-4-pause, UI-024)
· alterado em 2026-11-12 (docs/specs/phase-5-library): a marcação também aparece no card da grade (UI-029) e na página do jogo (UI-038).
Dada uma sessão em `running` ou `grace` (lida de `CurrentSession` ao carregar e atualizada pelos eventos
`session:changed`, sem polling), então a linha do jogo dela mostra "Em execução"; quando chega `session:changed` com
`ended` ou `discarded`, então a marcação some e a lista é recarregada (total atualizado).

### UI-007 — Adicionar jogo
> **Removido na fase 6 (sem botão "Adicionar jogo"):** a única forma de adicionar jogos avulsos é a fonte "Arquivo" do diálogo Importar (IMP-004, fase 6). Este requisito não vale mais.

Quando clico "Adicionar jogo" e escolho um `.exe`, então aparece o formulário com o caminho escolhido e o campo
"Título" preenchido com o título padrão (editável, no máximo 200 caracteres), e os botões "Salvar" e "Cancelar";
"Salvar" fica desabilitado com título vazio (após cortar espaços) ou durante o envio; quando salvo com sucesso, então
o formulário fecha e o jogo aparece na lista; quando cancelo no diálogo ou no formulário, então nada muda.

### UI-008 — Erros visíveis · alterado em 2026-11-10: a pendência vale para os dois botões do jogo (docs/specs/phase-4-pause, UI-021)
Dado que "Jogar" ou "Salvar" falha, então a mensagem de DSK-011 aparece num aviso no topo da lista, com o botão
"Fechar"; um erro novo substitui o anterior e a próxima ação bem-sucedida o remove. Durante um "Jogar" pendente, o
botão daquele jogo mostra "Iniciando…" e fica desabilitado.

### UI-009 — Recarga ao reconectar
Dado que o indicador passa de "Desconectado" para "Conectado", então a UI recarrega a lista de jogos e a sessão atual.

### UI-010 — Estado vazio e carregamento
· substituído por UI-044 (docs/specs/phase-5-library): mesmos textos, agora dentro da grade e com contagem de `total`.
Dado `game.list` com `[]`, então a tela mostra "Nenhum jogo ainda" (sem contadores) e o botão "Adicionar jogo"
continua disponível; enquanto a lista não carregou ou a chamada falha, então a tela mostra "Conectando ao agente…" e
tenta de novo a cada 2 s.

### UI-011 — Ponta a ponta com um jogo real
Dado o Arcadia aberto, quando adiciono um jogo real cujo `.exe` é um lançador que abre o jogo e fecha, jogo por mais
de 1 min e fecho o jogo, então durante o jogo a linha mostra "Em execução" e o painel da sessão mostra o cronômetro
avançando, ~15 s depois do fechamento a marcação e o painel somem e o tempo total mostra o tempo jogado; fechar e
reabrir a UI durante o jogo não altera o tempo gravado e o cronômetro reaparece no valor certo (±1 s).
· verificação: manual

### UI-012 — Painel da sessão em andamento · alterado em 2026-11-10: existe também em `paused` e ganha o botão "Pausar"/"Retomar" (docs/specs/phase-4-pause, UI-022, UI-027)
Dada uma sessão em `running` ou `grace` (UI-006), então acima da lista aparece um painel (`role="status"`, nome
acessível "Sessão em andamento") com o título do jogo da sessão e o cronômetro (UI-013); dada nenhuma sessão, ou
quando chega `session:changed` com `ended` ou `discarded`, então o painel não existe na tela.

### UI-013 — Formato do cronômetro
Dado um tempo em segundos inteiros `s`, então o cronômetro mostra `H:MM:SS`, com horas sem zero à esquerda e sem
limite: `0` → "0:00:00"; `59` → "0:00:59"; `252` → "0:04:12"; `3600` → "1:00:00"; `45005` → "12:30:05";
`450000` → "125:00:00".

### UI-014 — Cronômetro avança a cada segundo
Dada uma sessão em `running` recebida com `activeMs = A` (de `CurrentSession` ao abrir a UI ou ao reconectar, de
`LaunchGame` ou de `session:changed`), quando passam `d` ms no relógio monotônico da UI desde o recebimento, então o
cronômetro mostra UI-013 de `floor((A + d) / 1000)`, e o texto muda no instante em que esse valor muda. Exemplo com
temporizadores falsos do Vitest: `A = 252400` → "0:04:12"; avançar 599 ms → "0:04:12"; avançar mais 1 ms →
"0:04:13"; avançar mais 1000 ms → "0:04:14". Dada uma sessão nova recebida, então o cronômetro recomeça dela
(`A` e o instante de recebimento novos), sem somar o valor anterior. A UI nunca calcula o tempo a partir de
`startedAt` nem de `asOf` comparados com o relógio dela.

### UI-015 — Carência congela o cronômetro
Dada uma sessão recebida em `grace` com `activeMs = A`, então o cronômetro mostra `floor(A / 1000)` parado (avançar o
tempo falso não o muda) e o painel mostra "Encerrando…"; quando chega `session:changed` com `running`, então
"Encerrando…" some e o cronômetro volta a avançar a partir do `activeMs` recebido (UI-014).

### UI-016 — Desconexão esconde o painel
Dado o painel visível, quando o indicador passa a "Desconectado", então o painel deixa de existir na tela; quando o
indicador volta a "Conectado", então o painel reaparece só depois da recarga de UI-009, com o valor da sessão lida
(sem retomar o valor antigo).

### UI-017 — Cronômetro só exibe
Dado o painel visível em `running`, quando 10 min passam no relógio falso, então nenhum binding é chamado por causa do
cronômetro (nenhuma chamada a `CurrentSession`, `LaunchGame`, `AddGame` ou `ListGames` além das que ocorreriam sem
ele), e o tempo total da lista não muda até chegar `session:changed` com `ended` e a lista ser recarregada (o total
mostrado é sempre o de `game.list`, calculado pelo agente).

### UI-018 — Resposta atrasada não sobrescreve evento
Dado um `CurrentSession` pendente, quando chega um `session:changed` antes da resposta, então a resposta é ignorada e
a tela segue o evento (ex.: evento `ended` seguido da resposta `running` atrasada → painel ausente).

## fake-game (FKG)

Contrato completo: [contracts/fake-game.md](contracts/fake-game.md).

### FKG-001 — Processo folha
Quando rodo `fake-game --run 300ms --exit-code 3`, então o processo termina depois de pelo menos 300 ms com código 3,
sem criar processos.

### FKG-002 — Árvore por profundidade e fan-out
Quando rodo `fake-game --depth 2 --fanout 2 --run 0 --report R`, então `R` tem 7 linhas `start` (1 com `depth = 2`,
2 com `depth = 1`, 4 com `depth = 0`), cada filho com `ppid` igual ao `pid` de quem o criou.

### FKG-003 — Padrão lançador
Quando rodo `fake-game --depth 1 --parent-run 0 --run forever --until-file F --report R`, então o processo inicial
sai com o código de `--exit-code` logo depois de criar o filho, e o filho continua até `F` existir.

### FKG-004 — Arquivo de término
Dado `--until-file F`, quando `F` passa a existir, então todo processo da árvore ainda vivo termina em até 500 ms,
mesmo com `--run forever`.

### FKG-005 — Vida máxima
Dado `--max-life 1s` e `--run forever`, quando 1 s passa, então o processo termina (proteção contra processos órfãos
em testes); sem a flag, o limite é 2 min.

### FKG-006 — Relatório
Dado `--report R`, então cada processo acrescenta a `R` uma linha JSON `start` ao começar e uma `exit` antes de
sair, no formato do contrato (`pid`, `ppid`, `depth`, `cwd`, `args` e, no `exit`, `code`).

### FKG-007 — Flags inválidas
Quando rodo `fake-game` com uma flag desconhecida ou valor inválido (ex.: `--depth -1`, `--run abc`), então ele sai
com código 2 sem criar processos nem escrever no relatório.

### FKG-008 — Filho com breakaway
Dado `--breakaway` e `--depth ≥ 1`, então os filhos são criados com `CREATE_BREAKAWAY_FROM_JOB`; se a criação falhar,
o processo sai com código 4 e registra a linha `exit` com `error` (o mesmo vale para qualquer falha ao criar filho).

---

## Fora de escopo
Watcher de processos (Camada 2) e regras de rastreamento · sinais de loja (Camada 3) · lançamento por URI, emulador
e argumentos de lançamento na UI · pausa por atalho/controle, pausa em suspensão/hibernação (fase 4) · "iniciar sem
contar tempo" e apps adicionais · remover/editar jogo (P5) · cronômetro fora da janela da UI (bandeja,
sobreposição no jogo) · readotar o jogo depois de o agente reiniciar · mudar `game.status` automaticamente · edição
manual de sessões · várias sessões simultâneas · constantes configuráveis pelo usuário · indicação de sessão na
bandeja · grade, filtros e mídia (fase 5). · alterado em 2026-11-12 (docs/specs/phase-5-library): grade, filtros, busca e edição/remoção de jogo entram na fase 5; mídia fica na fase 7.

## Limitações conhecidas
A confirmar na aprovação; não geram requisito nem teste.

1. **A carência quase nunca volta a Running na Camada 1.** Um job sem processos não ganha processos novos (ninguém
   dentro dele pode criá-los); o retorno de SES-005 só ocorre com mensagens atrasadas/fora de ordem ou com a verificação
   de SES-013. A carência, por ora, só atrasa o fim em 15 s; ela passa a ter efeito real com a Camada 2 (fase 6).
2. **Jogos que entregam o lançamento a um cliente externo** (Steam/Epic por URI ou lançador que pede ao cliente já
   aberto) saem em segundos: a sessão fica `discarded`. Resolvido nas Camadas 2 e 3.
3. **Filhos com breakaway** escapam do rastreamento (ADR 0016, TRK-006).
4. **Jogos que exigem administrador** (`requireAdministrator` no manifesto) falham com `ERROR_ELEVATION_REQUIRED`
   (740) e aparecem como "Não foi possível iniciar o jogo"; elevar quebraria a árvore do job.
5. **Queda do agente durante o jogo:** perde-se até 30 s (último heartbeat); o jogo que continua aberto depois da queda
   não é readotado, e o tempo dali em diante não conta.
6. **Mensagens do job não são garantidas pelo Windows:** a verificação de 5 s (SES-013) cobre a perda; nesse caso o
   fim é registrado até 5 s depois do real.
7. **Suspensão/hibernação conta como tempo jogado** até a fase 4 (superada pela Limitação 12 da fase 4).
8. **Cronômetro × tempo gravado:** o cronômetro é uma estimativa da UI. Ele pode estar até ~1 s à frente ou atrás do
   valor do agente (truncamento, latência do IPC e do evento Wails) e, ao entrar em Grace, é substituído pelo
   `activeMs` do agente, podendo recuar um pouco. O tempo gravado é sempre o do agente.
9. **Notificação perdida:** com a fila de uma conexão cheia, `session.changed` é descartada para ela (IPC-029); a UI
   se corrige ao reconectar (UI-009) ou no próximo evento. Até lá, se o evento perdido era o fim, o painel continua
   mostrando o cronômetro avançando.
10. **Despacho sequencial por conexão:** um `game.launch` lento (antivírus no `CreateProcess`) atrasa o
    `agent.status` da mesma conexão e pode fazer o indicador piscar "Desconectado" (design.md, Riscos).
