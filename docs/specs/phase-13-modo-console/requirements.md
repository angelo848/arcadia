# Fase 13 — Modo console: requisitos

- **Status:** aprovada pelo usuário em 2026-11-15, com as respostas registradas em "Respostas do usuário" no fim (motivo
  `closed-by-hotkey`, `WM_CLOSE` por até 3 s antes de matar, sem número de discos; as demais recomendações aceitas).
  Ajustes do teste com controle (BBX-032..BBX-045, 2026-11-16): **aprovados pelo usuário em 2026-11-16**, com as mudanças
  registradas em "Ajustes do teste com controle" no fim (aba lida por páginas com pré-busca à frente, contador na faixa, Y fora
  dos detalhes).
- **Design:** [design.md](design.md) · **Contratos:** [contracts/ipc-v0.15.0-delta.md](contracts/ipc-v0.15.0-delta.md) (só o
  acréscimo sobre a v0.14.0 de [phase-12-metadados-unificados](../phase-12-metadados-unificados/contracts/ipc.openrpc.json); o
  JSON completo v0.15.0 é uma tarefa, como nas fases 11 e 12),
  [contracts/session-close-state-machine.md](contracts/session-close-state-machine.md) (linhas novas da máquina de estados da
  sessão para Select + B), [contracts/bigbox-faixa.md](contracts/bigbox-faixa.md) (layout, mapa de entrada, estados da
  navegação, contrato de tema e tokens), [contracts/screenscraper-rules.md](contracts/screenscraper-rules.md) (sistemas,
  identificação do jogo, escolha da mídia, limites, erros e credenciais) e
  [contracts/screenscraper-api-notes.md](contracts/screenscraper-api-notes.md) (pontos da API a conferir, preenchido pela tarefa
  de conferência) · **Tarefas:** `tasks.md` (o planner gera depois da aprovação).
- **Base:** [phase-4-pause](../phase-4-pause/requirements.md) (combo do controle, roteador, feedback, lançamento sem rastrear),
  [phase-3-tracker](../phase-3-tracker/requirements.md) (Job Object, máquina de estados, motivos de fim),
  [phase-6-importers](../phase-6-importers/requirements.md) (processo adotado, RUL-002..RUL-005),
  [phase-8-estatisticas](../phase-8-estatisticas/requirements.md) e [phase-10-nova-interface](../phase-10-nova-interface/requirements.md)
  (Big Box atual, UIX-044), [phase-10-2-imagens-ao-vivo](../phase-10-2-imagens-ao-vivo/requirements.md) (`media.changed`),
  [phase-12-metadados-unificados](../phase-12-metadados-unificados/requirements.md) (busca, `scrapers.Limiter`, limite por tipo).
- **ROADMAP:** fase 13 (ponto 9), com as decisões do usuário de 2026-11-15 (antes e depois dos mockups).
- **ADRs:** 0005 e 0016 (Job Object, breakaway), 0017 (pausa), 0018 (entrada global e feedback nativo), 0019 (lançamento sem
  rastrear: **substituído em parte**, ver ADR proposto 1), 0025 (processo adotado), 0029 (tema por tokens), 0030 e 0034 (limite
  por tipo), 0031 (avisos de invalidação), 0033 (busca unificada). ADRs propostos no fim.
- **Objetivo:** (1) o controle fecha o jogo em curso com **Select + B** segurado por 1 s, contando o tempo até ali, e o Big Box
  volta ao primeiro plano no jogo que rodava; (2) o Big Box é refeito no estilo de launcher de console, com temas selecionáveis e
  o tema **Faixa** implementado; (3) o Big Box mostra **vídeo** dos jogos, baixado do ScreenScraper (`video-normalized`, até
  20 MB, um por jogo). **A UI nunca influencia o tempo de jogo:** quem fecha o jogo e encerra a sessão é o agente, pelo controle.

Formato: IDs `ÁREA-NNN`, estáveis e nunca reutilizados; critérios Dado/Quando/Então. Áreas novas (conferidas com `grep` em
`docs/`, sem colisão): **CON** (fechar o jogo pelo controle e invariantes da fase), **BBX** (Big Box novo e temas), **VID**
(vídeo: modelo, fila, provedor, exibição) e **SSC** (credenciais do ScreenScraper e janela dos provedores). Todo ID ativo
precisa de um teste que o cite no nome (`t.Run("CON-005/...")`, `it("BBX-011 ...")`), exceto os marcados
`· verificação: manual`. Nenhum teste usa rede (`httptest`, fakes) nem `time.Sleep` (relógio fake `domain.Clock` no Go,
`vi.useFakeTimers` no frontend). Instantes: `T` é o `Now()` de um `FakeClock`.

---

## Decisões do usuário (2026-11-15, não reabrir)

1. **Select + B**, só controle, segurado **1 s** como o Select + Y, **sem** som de confirmação e **sem** confirmação extra.
2. Encerra a sessão em curso fechando a árvore de processos: primeiro **pede para fechar** (`WM_CLOSE` às janelas dos
   processos vivos do jogo) e espera até 3 s; se ainda houver processo vivo, **mata** (Job Object; adotado por PID). A sessão
   fecha com o tempo contado até o gatilho e o motivo de fim novo **`closed-by-hotkey`** (com hífen, como os outros motivos;
   decidido na aprovação).
3. Jogo lançado por URI (Steam/Epic): fecha o processo que o tracker adotou; sem processo adotado, **ignora com o aviso sonoro
   de recusa**.
4. Funciona **fora do Big Box** também. Com a sessão **pausada**, encerra normalmente e o tempo pausado não conta. Uma sessão por
   vez. Vale para sessão **sem contar tempo** (ADR 0019) e para **processo adotado** (ADR 0025).
5. Depois de fechar, se o Big Box estava ativo, a janela volta ao primeiro plano, em tela cheia, com o foco no jogo que rodava.
6. **Big Box novo**, tema inicial **Faixa** (estilo PS5); nomes de plataforma e de jogo em **texto**. O Big Box fica só com
   navegar por plataformas e jogos, imagens e vídeos, detalhes, tempo total e gamepad; estatísticas, emuladores, plataformas,
   provedores e importação saem dele. **Temas selecionáveis**: a estrutura nasce pronta; Mosaico e Roda ficam descritos e
   preparados, não implementados; Vitrine fica fora.
7. **Vídeo do ScreenScraper**, sempre o `video-normalized`; **teto de 20 MB** por vídeo (acima não baixa); sem limite total;
   um vídeo por jogo. Credenciais (desenvolvedor do Arcadia e conta do usuário) **entram pelo app**, na janela dos provedores,
   nunca em repositório nem em log. O jogo é identificado por **nome + sistema**, sem hash (IMV-080).

## Decisões do spec-writer (confirmadas na aprovação, 2026-11-15)

a. **Motivo gravado `closed-by-hotkey`**, com hífen como `agent-shutdown` e `client-no-launch` (resposta 1). `end_reason` é
   `TEXT` sem `CHECK` (ADR 0012): **sem migração** para o motivo (CON-016).
b. **O tempo conta até o instante do gatilho; a espera do fechamento não conta.** O tempo ativo vai até `t` (o instante em que
   o combo completou 1 s); em Grace, até `graceSince` (a carência nunca conta). A sessão fica num estado intermediário de
   fechamento enquanto o jogo atende ao `WM_CLOSE` (até 3 s) ou é morto, e só então é gravada, com `ended_at = t`
   (CON-005..CON-007, CON-030..CON-033). Justificativa: o usuário pediu o fim em `t`; o que vem depois é o tempo que cada jogo
   leva para sair (0 a 3 s, variável por jogo e por diálogo de "salvar?"), não jogo; contá-lo faria o mesmo aperto dar tempos
   diferentes e o registro ficar maior que o pedido. Se a morte falhar, o fechamento é desfeito e a sessão segue como se o
   gatilho não tivesse acontecido, então nada se perde (CON-013).
c. **Jogo sem contar tempo passa a nascer dentro de um Job Object só para poder ser fechado**, sem sessão, sem porta de
   conclusão e sem goroutine; o tracker guarda só o mais recente e nunca o mata ao fechar o agente. Sem isso não há como fechar
   a árvore de um jogo lançado desacoplado (substitui em parte o ADR 0019; CON-010, CON-011).
d. **Prioridade:** com sessão aberta, Select + B fecha a sessão; sem sessão, fecha o jogo sem contar tempo vivo; sem nenhum
   dos dois, recusa com som (CON-012).
e. **Aviso sonoro de recusa novo** (não existe hoje): um sinal grave de dois tons, só som, sem aviso na tela (CON-015).
f. **A UI sabe que o jogo foi fechado pela notificação nova `game.closedByHotkey {gameId}`** (ADR 0031: só o id). Ela cobre
   também o jogo sem contar tempo, que não tem sessão nem `session.changed` (CON-019).
g. **O vídeo é um tipo de mídia novo, `video`**, na mesma tabela e na mesma fila de MID-005, com limite 1 (`MediaKindLimit`,
   ADR 0030/0034) e migração 9 para o `CHECK` de `media.kind` (VID-001, VID-002). A busca do vídeo é um job da fila com
   pseudo-URL `screenscraper:lookup`, resolvido na hora do download, numa **raia própria de 1 trabalhador** que nunca atrasa as
   imagens (VID-009).
h. **Quando o vídeo é buscado:** sozinho depois de aplicar metadados (busca única ou lote da fase 12) e, para a biblioteca já
   existente, por pedido explícito na janela normal ("Buscar vídeo" no painel e "Buscar vídeos dos jogos sem vídeo" na janela
   dos provedores). **O Big Box nunca dispara download** (navegar não gasta cota nem escreve nada). (VID-004..VID-007)
i. **Só `match` baixa vídeo.** O jogo do ScreenScraper é escolhido com a pontuação da fase 12 (`domain.ScoreGroup`,
   `ClassifyMatch`); "em dúvida" e "nenhum" não baixam (vídeo errado no fundo é pior que nenhum). (VID-011)
j. **ScreenScraper só para vídeo nesta fase:** não entra em `metadata.search` nem em texto e imagens (preserva a cota e não mexe
   no agregador da fase 12). (SSC-005)
k. **Vídeo por plataforma fica fora:** o Faixa não tem lugar para ele (a barra de plataformas é texto e o fundo é sempre o
   jogo em foco). A conferência da API registra se ele existe, para o tema Roda. (Fora desta spec)
l. ~~Número de discos passa a ser gravado~~ · removida na aprovação (resposta 7): sem `game.disc_count`, sem
   `GameDetail.discs` e sem mudança na importação; o Faixa não mostra discos (BBX-031 removido).
m. **Tecla de controle no Big Box:** A = Jogar, X = Detalhes, LB/RB = plataforma, B = Voltar/Sair, Y = som do vídeo; com
   Select segurado, a UI ignora B e Y (são os combos do agente). Teclado: Enter, D, PageUp/PageDown, Esc, M.
   ([bigbox-faixa §2](contracts/bigbox-faixa.md))

---

## Requisitos de fases anteriores afetados

Cada um recebeu, na spec de origem, a anotação `· alterado em 2026-11-15 (docs/specs/phase-13-modo-console, ID)` ou
`· substituído em ...`. Se esta spec não for aprovada, as anotações são desfeitas.

| Requisito | Fase | Situação | Por |
|---|---|---|---|
| TRK-010 — Lançamento sem rastrear fica fora do rastreamento | 4 | alterado: o processo nasce suspenso dentro de um Job Object guardado pelo tracker (sem sessão, sem porta, sem `KILL_ON_JOB_CLOSE`); continua vivo depois de `Tracker.Close` | CON-011 |
| SES-032 — Lançamento sem rastrear não cria sessão | 4 | alterado (a nota do job; sem sessão, sem notificação, como antes; durante o fechamento pelo controle, lançamentos são recusados) | CON-011, CON-031 |
| PAU-004 — Antirrepique entre fontes | 4 | alterado: a janela de 300 ms conta só gatilhos de pausa; os de fechar têm janela própria | CON-004 |
| SND-001 — Dois sinais distintos | 4 | alterado: entra um terceiro sinal, o de recusa | CON-015 |
| FBK-002 — O coordenador mostra o aviso e toca o som | 4 | alterado: `EventRefused` toca `CueRefused` sem aviso na tela | CON-015 |
| RUL-005 — Pausa e demais regras (sessão adotada) | 6 | alterado: Select + B pede para fechar e depois mata os processos adotados vivos | CON-008 |
| STO-027 — Sessões de um jogo | 5 | alterado: `endReason` pode ser `client-no-launch` (correção do contrato) e `closed-by-hotkey` | CON-017 |
| BIG-001, BIG-002, BIG-003 — Big Box | 8 | **substituídos** pelo Big Box novo | BBX-001..BBX-030 |
| BIG-004 — Pausa durante o Big Box | 8 | alterado: a marcação "Pausado" fica no bloco de informações e na capa do Faixa | BBX-017 |
| UIX-044 — Big Box sobre o layout novo | 10 | **substituído**: o Big Box não usa mais as três colunas | BBX-001..BBX-030 |
| UIX-045 — Estatísticas abrem o jogo no painel | 10 | alterado: a parte "no Big Box" deixa de existir (estatísticas saem do Big Box) | BBX-003 |
| UIX-006 — Cores só por variáveis do tema | 10 | alterado: a varredura inclui `frontend/src/bigbox/` e os tokens `--arc-bb-*` | BBX-025 |
| MID-001 — Chaves de API | 7 | alterado: dois provedores novos de credencial, `screenscraper` e `screenscraperdev` | SSC-001 |
| MID-005 — Fila em background | 7 | alterado: jobs de vídeo numa raia própria, com pausa por cota e por credencial | VID-009, VID-016, VID-017 |
| AJU-020 — Limite de imagens por tipo | 10.1 | alterado: a tabela de `MediaKindLimit` ganha `video` = 1 | VID-002 |
| MET-010 — Intervalo mínimo por provedor | 12 | alterado: ScreenScraper com 1.500 ms (ou mais, pelo limite por minuto da conta) | VID-015 |
| MET-025 — Só os tipos que existem | 12 | alterado: vale para a fase 12; a 13 cria o tipo `video` (migração 9) | VID-001 |
| MET-030, MET-065 — Aplicar metadados | 12 | alterado: depois de aplicar, o agente enfileira a busca de vídeo do jogo quando cabe | VID-004 |
| MET-067 — Indicador e abertura automática | 12 | alterado: com o Big Box ativo a janela de confirmação não abre sozinha | BBX-022 |
| Decisão 1 da fase 12 (ScreenScraper fora) | 12 | superada para vídeo; continua fora da busca de metadados | SSC-005 |

Inalterados e ainda válidos: SES-001..SES-031, SES-033..SES-037 (a máquina de pausa não muda; Select + B entra como evento novo),
SES-011 e TRK-004 (fechar o agente **nunca** mata o jogo), PAD-001..PAD-009 (o combo de pausa é o mesmo), HOT-*, PWR-*, OVL-*,
FBK-001 (o fechamento continua sem feedback), FBK-003..FBK-005, PAU-001..PAU-003, PAU-005..PAU-008, RUL-001..RUL-004,
STM-001, DEL-001..DEL-011 (a pasta do jogo inclui o `.mp4`), IMG-001..IMG-011, MET-001..MET-024, MET-026..MET-083, UIX-001..UIX-043,
UIX-046..UIX-049 e EST-*.

**Lacuna encontrada (registrada aqui):** o contrato v0.14.0 lista em `SessionItem.endReason` só `exited`,
`agent-shutdown` e `recovered`, mas o agente grava `client-no-launch` desde a fase 6 (RUL-003); CON-017 corrige. (O número de
discos lido na importação continua sem ser gravado; por decisão da aprovação, fica fora desta fase.)

---

## CON — Fechar o jogo pelo controle

### CON-001 — Combo Select + B segurado dispara o gatilho de fechar
Dado o controle 0 sem botões, quando `Back` (`0x0020`) e `B` (`0x2000`) passam a estar pressionados juntos em `T` e continuam,
então exatamente um `Trigger{Action: close, Source: gamepad, At}` é entregue na primeira leitura com `agora − T ≥ 1 s` (com
leituras a cada 16 ms: `T+992ms` não dispara, `T+1008ms` dispara). As regras de PAD-002..PAD-005 valem iguais para este combo:
soltar antes cancela, um gatilho por pressão, controles independentes, desconexão zera a contagem.
· verificação: teste do poller com `PadAPI` falsa e relógio fake.

### CON-002 — Os dois combos são independentes
· alterado em 2026-11-22 (docs/specs/phase-15-big-box-v2, ATB-002): são três combos independentes (pausa, fechar e Big Box com
`Back + A`).
Dado o poller, então `Back + Y` só entrega `Action: pause` e `Back + B` só entrega `Action: close`. Dado `Back + Y + B`
segurados juntos por 1 s, então os dois gatilhos são entregues (um de cada), cada um uma vez por pressão. Soltar `B` não zera a
contagem de `Back + Y`, e vice-versa.
· verificação: teste do poller (sequências de botões; contagem por ação).

### CON-003 — Sem atalho de teclado novo
Dado o agente, então o único atalho de teclado registrado continua `Ctrl+Alt+P` (pausa); fechar o jogo não tem atalho de
teclado nem botão na UI.
· verificação: teste do listener (uma chamada a `RegisterHotKey`); teste de invariantes da UI (CON-024).

### CON-004 — Roteamento do gatilho de fechar
· alterado em 2026-11-15: prazo de 3 s para 6 s, para caber a espera do fechamento (CON-030) e a gravação (resposta 2).
Dado `Trigger{Action: close, At: t}`, então o roteador chama `Tracker.CloseByHotkey(ctx, t)` com prazo de 6 s. A janela de
antirrepique de PAU-004 conta só gatilhos de pausa; gatilhos de fechar têm a sua: dois deles com menos de 300 ms entre os `At`
viram um só (o segundo é descartado sem som); um gatilho de pausa e um de fechar no mesmo instante são ambos encaminhados, o de
pausa primeiro se chegou primeiro.
· verificação: teste do roteador com tracker falso (ordem das chamadas; janelas por ação).

### CON-005 — Fechar uma sessão em Running
· alterado em 2026-11-15: pede para fechar e espera antes de matar (CON-030); o fim é gravado ao terminar o fechamento, com o
tempo cortado no gatilho; motivo com hífen (respostas 1 e 2).
Dada uma sessão em Running do jogo `G`, lançada em `T` (`tracking = job`), com 0 s acumulados, quando chega `CloseByHotkey` em
`T+600s`, então a sessão entra em fechamento (CON-031, CON-032) com o tempo ativo congelado em 600 s e a árvore é fechada por
CON-030 (pedido `WM_CLOSE`, espera de até 3 s, `TerminateJobObject` no prazo). Quando a árvore termina (sozinha ou morta), então
o tracker grava `EndSession` com `ended_at = T+600s`, `active_seconds = 600`, motivo `closed-by-hotkey` e estado por DOM-008
(`ended`); fecha o job; notifica `session.changed` com `state = ended`, `endedAt = T+600s`, `activeMs = 600000` e `asOf` = instante
em que o fechamento terminou; o total do jogo aumenta 600 (STO-014); e o resultado é `closed{gameId: G}`. A espera nunca conta:
com a árvore saindo sozinha em `T+602s` ou morta em `T+603s`, os valores gravados são os mesmos (só o `asOf` muda). Dada uma
sessão com 20 s de tempo ativo, então ela termina `discarded` e o jogo não muda (DOM-008). Um gatilho com `At` anterior ao
`startedAt` é recusado (CON-012).
· verificação: teste da máquina e do coordenador com job, store e relógio falsos (árvore sai durante a espera; árvore morta no
prazo); integração Windows (TRK) com `fake-game --depth 2 --run forever` (sem janela, morte imediata por CON-030): os três
processos terminam (`WaitForSingleObject` com prazo real limitado).

### CON-006 — Fechar uma sessão em Paused
· alterado em 2026-11-15: fechamento por CON-030 antes do fim; motivo com hífen (respostas 1 e 2).
Dada uma sessão em Paused desde `T+100s` com 100 s acumulados, quando chega `CloseByHotkey` em `T+500s`, então a sessão entra em
fechamento com o tempo continuando congelado e a árvore é fechada por CON-030 (pausar não suspende o jogo, ADR 0017, então ele
recebe o pedido). Quando a árvore termina, então a sessão termina com `ended_at = T+500s`, `active_seconds = 100` e motivo
`closed-by-hotkey`, a pausa aberta é fechada em `T+500s` na mesma transação (STO-021), a notificação traz `activeMs = 100000`, e
não há feedback de retomada (nem nenhum outro).
· verificação: teste da máquina e do coordenador.

### CON-007 — Fechar uma sessão em Grace
· alterado em 2026-11-15: sem pedido nem espera (job vazio); motivo com hífen (respostas 1 e 2).
Dada uma sessão em Grace desde `T+300s` (job vazio, 300 s acumulados), quando chega `CloseByHotkey` em `T+305s`, então não há
processo a quem pedir nem espera (CON-030), o job é morto (sem efeito se vazio), a carência pendente é descartada (geração), a
sessão termina na hora com `ended_at = T+300s`, `active_seconds = 300` e motivo `closed-by-hotkey`, e a notificação é enviada com
`asOf = T+305s`. A carência nunca conta.
· verificação: teste da máquina e do coordenador (a carência que vence depois não grava nada).

### CON-008 — Fechar um processo adotado
· alterado em 2026-11-15: os processos conferidos recebem primeiro o pedido de fechar (CON-030) e só são mortos no prazo
(resposta 2).
Dada uma sessão em Running com `tracking = process` (RUL-002), quando chega `CloseByHotkey`, então o tracker tira um snapshot
novo e o conjunto a fechar é o dos processos vivos do conjunto adotado que **ainda casam com uma regra do jogo pelo caminho**;
só as janelas desses PIDs recebem o pedido de CON-030. A árvore terminou quando o `adoptedJob` avisa que nenhum processo adotado
está vivo (RUL-004). Vencido o prazo, um snapshot novo decide quem ainda morre: para cada um, o tracker abre o processo com
`PROCESS_TERMINATE | PROCESS_QUERY_LIMITED_INFORMATION`, confere que o caminho da imagem é o mesmo do snapshot (PID reaproveitado
não é morto), chama `TerminateProcess` e fecha o handle; a sessão termina como em CON-005. Processos fora das regras (inclusive
o cliente da loja) nunca recebem `WM_CLOSE` nem são mortos. Dado que o conjunto adotado está vazio no snapshot, então a sessão
termina na hora (nada a pedir nem a matar).
· verificação: teste do `adoptedJob` com `ProcessLister`, enumerador de janelas e terminador falsos (PID reaproveitado, processo
fora da regra, conjunto vazio, saída durante a espera); manual com Steam (CON-025).

### CON-009 — Jogo por URI ainda sem processo adotado
Dada uma sessão em Launching de um jogo `uri` (RUL-003, sem processo adotado), quando chega `CloseByHotkey`, então nada é morto
nem gravado, a sessão continua esperando a adoção (o prazo de 60 s não muda), o resultado é `refused{reason: not-adopted}` e o
roteador pede o som de recusa (CON-015).
· verificação: teste do coordenador.

### CON-010 — Fechar um jogo sem contar tempo
· alterado em 2026-11-15: a árvore é fechada por CON-030 (pedido, espera, morte no prazo) (resposta 2).
Dado o tracker sem sessão e com um jogo `G` lançado sem contar tempo cujo job (CON-011) tem processos vivos, quando chega
`CloseByHotkey`, então a árvore é fechada por CON-030 (durante a espera, CON-031 vale; o fim da árvore é conferido por
`ActiveProcesses()` a cada 250 ms), o job é fechado e esquecido, nenhuma linha de `play_session` é
criada, nenhum `session.changed` é enviado e o resultado é `closed{gameId: G, untracked: true}`. Dado que o job não tem mais
processos (`ActiveProcesses = 0`), então ele é fechado e esquecido e o caso é CON-012. Dada uma sessão aberta **e** um jogo sem
contar tempo vivo (ADR 0019 permite os dois), então o primeiro Select + B fecha só a sessão; o segundo fecha o jogo sem contar
tempo.
· verificação: teste do coordenador com launcher falso; integração Windows com `fake-game --depth 1 --run forever`.

### CON-011 — O jogo sem contar tempo nasce dentro de um job guardado
Dado `LaunchUntracked`, então o processo é criado suspenso com o mesmo `LaunchSpec` de SES-032, associado a um Job Object novo
com `JOB_OBJECT_LIMIT_BREAKAWAY_OK` (sem `KILL_ON_JOB_CLOSE` e sem `SILENT_BREAKAWAY_OK`, ADR 0016), retomado, e os handles do
processo e da thread são fechados antes de responder `{pid}`. O job não tem porta de conclusão nem goroutine. O tracker guarda só
o job do **último** lançamento sem rastrear: um novo lançamento fecha o anterior **sem matar** seus processos. `Tracker.Close`
fecha o job guardado sem matar nada (o processo continua vivo, como em TRK-010). Falha ao criar ou associar o job é
`LaunchFailed` (SES-034) e não deixa processo vivo nem handle aberto. Depois de 20 lançamentos sem rastrear de
`fake-game --run 0` e um `Close`, a contagem de handles do processo de teste é `≤ h0 + 2` (TRK-011).
· verificação: integração Windows (`IsProcessInJob`; processo vivo depois de `Close`; contagem de handles).

### CON-012 — Nada para fechar
Dado o tracker sem sessão e sem jogo sem contar tempo vivo, ou um gatilho com `At` anterior ao `startedAt` da sessão, quando chega
`CloseByHotkey`, então nada é morto, gravado nem notificado, é registrado `Debug` "fechar ignorado" com o motivo, o resultado é
`refused{reason: no-game}` e o roteador pede o som de recusa.
· verificação: teste do coordenador e do roteador.

### CON-013 — Falha ao matar
· alterado em 2026-11-15: a falha é a da morte no fim da espera, e o fechamento é desfeito (resposta 2).
Dado que a morte no fim da espera de CON-030 falha (`Job.Kill()` com acesso negado, processo protegido), então o fechamento é
desfeito: a sessão **volta ao estado de origem** como se o gatilho não tivesse acontecido (Running continua contando a partir de
`at`, sem lacuna nem desconto; Paused continua pausada com o mesmo intervalo aberto), nada é gravado nem notificado, é
registrado `Warn` "falha ao fechar o jogo" com `session_id` e `error`, o resultado é `failed` e o roteador pede o som de recusa.
Dado que só parte dos processos adotados morreu, então vale o mesmo: a sessão segue e o watcher encerra quando o resto sair
(RUL-004). No jogo sem contar tempo, o job continua guardado.
· verificação: teste da máquina e do coordenador com job falso que falha (tempo ativo depois do desfazer).

### CON-014 — Sucesso sem som nem aviso
Dado um fechamento com resultado `closed` (sessão ou jogo sem contar tempo), então nenhum feedback é emitido: nem `Overlay.Show`
nem `Sound.Play` (decisão 1). O fim da sessão continua sem feedback (FBK-001).
· verificação: teste do roteador com feedback falso.

### CON-015 — Som de recusa
· alterado em 2026-11-15: `refused{closing}` (CON-032) não toca nada.
Dado o resultado `refused` (motivo `no-game` ou `not-adopted`) ou `failed`, então o roteador envia `feedback.EventRefused`; o
coordenador chama só `Sound.Play(CueRefused)` (nenhum `Overlay.Show`), e o som são dois tons graves descendentes: 300 Hz por
100 ms e 200 Hz por 200 ms, distintos dos sinais de pausa (SND-001). Gatilho descartado pelo antirrepique (CON-004), recusado
com `refused{closing}` (CON-032) ou durante o shutdown (CON-021) não toca nada.
· verificação: teste do coordenador e do `Beeper` com função de tom injetada.

### CON-016 — Motivo `closed-by-hotkey` validado no domínio, sem migração
· alterado em 2026-11-15: valor com hífen (resposta 1).
Dado o texto de uma coluna, quando chamo `domain.ParseSessionEndReason`, então `exited`, `agent-shutdown`, `recovered`,
`client-no-launch` e `closed-by-hotkey` são aceitos e qualquer outro valor (inclusive `""`, `closed_by_hotkey` e
`Closed-By-Hotkey`) é erro. Nenhuma
migração é criada para o motivo (`end_reason` é `TEXT` sem `CHECK`, ADR 0012).
· verificação: teste de unidade do domínio; teste do store gravando e lendo o motivo.

### CON-017 — `session.list` devolve o motivo novo
· alterado em 2026-11-15: valor com hífen (resposta 1).
Dada uma sessão encerrada por Select + B, quando chamo `session.list` do jogo, então o item traz `endReason = "closed-by-hotkey"`;
dada uma sessão de URI descartada por RUL-003, `endReason = "client-no-launch"`. O esquema `SessionItem.endReason` da v0.15.0
lista os cinco valores (correção da lacuna 1).
· verificação: teste do agente e teste de contrato do IPC.

### CON-018 — Sessões no painel mostram o fechamento pelo controle
· alterado em 2026-11-15: valor com hífen (resposta 1).
Dada uma sessão com `endReason = closed-by-hotkey` na lista de sessões do painel (UIX-030), então a linha mostra, além da duração,
o texto "fechado pelo controle"; os outros motivos não ganham texto novo.
· verificação: teste de componente de `SessionsList`.

### CON-019 — Notificação `game.closedByHotkey`
Dado um fechamento com resultado `closed` do jogo `G`, então, depois de gravado o fim e enviado o `session.changed` (quando há
sessão), todo cliente conectado recebe a notificação `game.closedByHotkey` com `params = {"gameId": G}` (sem `id`). Ela não é
enviada para `refused` nem `failed`, nem em resposta a chamada da UI (IPC-050). Fila de conexão cheia descarta (IPC-017).
· verificação: teste do agente com dois clientes IPC reais e tracker falso.

### CON-020 — Repasse do desktop
Dado `game.closedByHotkey` vindo do agente, então o supervisor do desktop emite o evento Wails `game:closedByHotkey` com
`{gameId}`; params ausentes, JSON inválido ou `gameId` não inteiro positivo registram `Warn` ("game.closedByHotkey com parâmetros
inválidos") e nada é emitido. O repasse não chama o agente.
· verificação: teste de `cmd/arcadia` com fake do agente.

### CON-021 — Gatilho durante o shutdown
Dado o agente depois de `BeginShutdown`, quando chega um gatilho de fechar, então ele é descartado (o tracker devolve
`ErrShuttingDown`, registrado em `Debug`), sem morte, sem gravação e sem som.
· verificação: teste do roteador.

### CON-022 — O Big Box volta ao primeiro plano no jogo que rodava
Dado o Big Box ativo com o foco no jogo `G` quando `G` foi lançado, quando chega `game:closedByHotkey {gameId: G}`, então a UI
chama `BringToFront()`, põe a janela em tela cheia (`enterBigBox`), sai da tela de detalhes se estava nela, libera a entrada
suspensa (BBX-018) e deixa o foco do Faixa em `G` (se `G` está na aba atual carregada; senão o foco não muda). Dado o Big Box
**inativo**, então nada acontece na janela (ela não sobe nem muda de estado). Dado `gameId` de outro jogo, então vale o mesmo
retorno, com o foco em `gameId` se estiver na aba.
· verificação: teste de componente com evento simulado e bindings espiões.

### CON-023 — `BringToFront` no desktop
Dado o binding `BringToFront()`, quando chamado, então o desktop restaura a janela principal se estiver minimizada, traz para o
topo (`SetWindowPos` topmost e depois não-topmost) e pede `SetForegroundWindow`; se o Windows recusar o primeiro plano, tenta uma
vez com `AttachThreadInput` à thread em primeiro plano e registra `Debug` com o resultado. Nunca chama o agente nem devolve erro
ao usuário por recusa do Windows.
· verificação: teste de `cmd/arcadia` com `winapi` falsa (ordem das chamadas); manual (CON-025).

### CON-024 — A UI nunca influencia o tempo de jogo
· alterado em 2026-11-22 (docs/specs/phase-15-big-box-v2, ATB-015, TTB-030): o teste de invariantes ganha a entrada no Big Box por
pedido externo (`TakeBigBoxRequest`) e os bindings `TimeToBeat*`.
Dado qualquer interação desta fase (Big Box, temas, vídeo, detalhes, "Buscar vídeo", janela dos provedores, evento
`game:closedByHotkey`), então a UI não tem ação de fechar jogo nem de encerrar sessão; só chama leituras, `LaunchGame` e
`LaunchGameUntracked` por ação explícita (um por ação, sem repetição por tecla mantida), `BringToFront`, `Video*` e
`ProviderSetKey`/`ProviderDeleteKey`. O teste de invariantes (`App.invariants.test.tsx`) ganha as interações desta fase.
· verificação: `App.invariants.test.tsx` com espiões em todos os bindings; revisão.

### CON-025 — Ponta a ponta do fechamento
Dado o Arcadia real com um controle XInput, quando seguro Select + B por 1 s: num jogo `.exe` rastreado; num emulador com ROM;
num jogo da Steam já adotado; num jogo da Steam antes da adoção; num jogo sem contar tempo; com a sessão pausada; fora do Big Box
e com a UI fechada; e com o Big Box ativo, então: o emulador (RetroArch) fecha sozinho pelo pedido e mantém o que salva ao
sair; um jogo que ignora o pedido fecha em até 5 s; a sessão aparece com o tempo até o aperto (±2 s, sem a espera) e
"fechado pelo controle"; o tempo pausado não conta; antes da adoção e sem jogo ouço o som de recusa e nada fecha; com o Big Box
ativo a janela volta em tela cheia com o foco no jogo; nenhum som toca no sucesso; um toque curto não fecha.
· verificação: manual.

### CON-026 — Contrato IPC v0.15.0
· alterado em 2026-11-15: sem `GameDetail.discs` (resposta 7); `x-console` ganha a espera do fechamento (resposta 2).
Dado o contrato `docs/specs/phase-13-modo-console/contracts/ipc.openrpc.json` v0.15.0 (gerado pela tarefa a partir da v0.14.0 e do
[delta](contracts/ipc-v0.15.0-delta.md)), então ele é a fonte da verdade, o teste de contrato de `internal/ipc` aponta para ele e
um teste confere que, em relação à v0.14.0, só há acréscimos: métodos e notificação novos, valores novos de enum
(`provider`, `MediaItem.kind`, `SessionItem.endReason`) e os blocos `x-console` e `x-video`.
· verificação: teste de contrato do IPC.

### CON-027 — Bindings e eventos do desktop
Dado o desktop, então existem os bindings `BringToFront`, `VideoQueue`, `VideoQueueMissing`, `VideoStatus` e
`VideoCancelPending` e o evento `game:closedByHotkey`, com as assinaturas do [delta §7](contracts/ipc-v0.15.0-delta.md);
`ProviderKeyStatus`, `ProviderSetKey` e `ProviderDeleteKey` aceitam `screenscraper` e `screenscraperdev`. Erros ao usuário seguem
UI-008.
· verificação: teste de `cmd/arcadia` com fake do agente.

### CON-028 — Onde a fase mexe
· alterado em 2026-11-15: entram o pedido de fechar e o estado de fechamento; a migração 9 só traz o tipo `video` (respostas 2 e 7).
Dado o diff da fase, então `internal/tracker` (e só ele) muda no rastreamento, pelo tracker-specialist: `Job.Kill`, o pedido de
fechar (`WM_CLOSE`), o estado de fechamento, o job do lançamento sem rastrear e o evento de fechar; `cmd/fake-game` não muda. A
única migração é a 9 (VID-001), sem mudança na tabela `game`. Nenhuma mudança em `play_session`, `pause_interval` ou
`tracking_rule`.
· verificação: revisão do diff.

### CON-029 — Sem rede e sem espera real nos testes
Dado a suíte da fase, então nenhum teste faz requisição de rede externa nem usa `time.Sleep`: o ScreenScraper é `httptest`, o
relógio é `domain.Clock` fake e o frontend usa temporizadores falsos. Os testes de integração Windows esperam processos com prazo
real limitado, como os da fase 3.
· verificação: revisão do diff; `go test ./... -race` sem internet.

### CON-030 — Pedir para fechar antes de matar
Dado um fechamento aceito (CON-005, CON-006, CON-008, CON-010) em `at`, com processos vivos, então o tracker posta
(`PostMessageW`, nunca `SendMessage`) `WM_CLOSE` a cada janela de topo visível cujo PID é de um processo vivo do jogo: no Job
Object (rastreado ou sem contar tempo), os PIDs de `JobObjectBasicProcessIdList`; no adotado, os de CON-008. Em seguida espera,
pelo `domain.Clock`, até 3.000 ms depois de `at`: se a árvore termina antes, nada é morto; vencido o prazo com processo vivo, a
árvore é morta (`TerminateJobObject`; adotado, `TerminateProcess` conferido de CON-008). Dado que nenhuma janela recebeu o pedido
(processos sem janela visível, ou erro ao enumerar, registrado em `Debug`), então a morte é imediata, sem espera. Janelas de
processos fora do jogo (a do Arcadia, o cliente da loja) nunca recebem o pedido. A espera não bloqueia a goroutine dona (eventos
do job e temporizadores continuam sendo atendidos).
· verificação: teste do coordenador com job, enumerador de janelas e relógio falsos (árvore sai em 1 s: sem `Kill`; não sai:
`Kill` em `at+3s`; sem janelas: `Kill` na hora); teste do enumerador com API falsa (só PIDs do jogo, só janelas visíveis de topo,
só `PostMessageW`); manual (CON-025).

### CON-031 — Lançamento recusado durante o fechamento
Dado um fechamento em espera (CON-030), de sessão ou de jogo sem contar tempo, quando peço `game.launch` ou
`game.launchUntracked`, então nenhum processo é criado, nada é gravado e o pedido é recusado com `SessionActive` (`-32002`): no
fechamento de sessão, com a sessão dela (como SES-002); no do jogo sem contar tempo, com `sessionId = 0`, `gameId` e `title` do
jogo sendo fechado. Terminado o fechamento (sucesso ou falha), os lançamentos voltam a seguir as regras de sempre.
· verificação: teste do coordenador com relógio falso (lançamento no meio da espera e depois dela).

### CON-032 — O que fica parado durante o fechamento
Dado um fechamento de sessão em espera (CON-030), então: gatilhos de pausa, `Suspend` e `Wake` são ignorados (`Debug`, sem
gravação, notificação nem feedback); `session.pause` e `session.resume` não gravam e devolvem a sessão atual; o heartbeat não
grava; `session.current` devolve a sessão no estado de origem (`running` ou `paused`) com `activeMs` congelado no tempo de `at`;
nenhum `session.changed` é enviado antes do fim. Um novo `CloseByHotkey` durante a espera (de sessão ou de jogo sem contar tempo)
não muda nada e tem resultado `refused{reason: closing}`, sem som (CON-015).
· verificação: teste da máquina e do coordenador.

### CON-033 — Shutdown durante o fechamento
Dado um fechamento em espera (CON-030), quando o agente para (`Stop(t)`), então nada é morto (SES-011, TRK-004), a sessão termina
com `ended_at = at` (a espera não conta), o tempo ativo congelado e o motivo `agent-shutdown`, sem notificação, e o
`CloseByHotkey` pendente responde `ErrShuttingDown` (o roteador não toca nada, CON-021). No jogo sem contar tempo, o job é fechado
sem matar (CON-011).
· verificação: teste do coordenador.

---

## BBX — Big Box novo

Layout, mapa de entrada, estados e tokens em [contracts/bigbox-faixa.md](contracts/bigbox-faixa.md). "Navegar" é a tela de
capas; "Detalhes" é a tela do jogo. Teclas entre parênteses são o equivalente no teclado.

### BBX-001 — Entrar no Big Box
· alterado em 2026-11-22 (docs/specs/phase-15-big-box-v2, ATB-008..ATB-012): também se entra por `arcadia.exe --bigbox` e por
Select + A (pedido externo).
Dado a janela normal, quando o usuário toca "Big Box" ou aperta F11, então a janela vai para tela cheia e mostra o tema escolhido
(BBX-023; padrão Faixa) na tela Navegar, no lugar da biblioteca de três colunas. Filtros, busca, jogo em foco e visualização da
janela normal não mudam (voltam iguais ao sair).
· verificação: teste de componente com runtime do Wails falso.

### BBX-002 — Sair do Big Box
Dado a tela Navegar, quando aperto B (Esc), então a janela sai da tela cheia (volta ao estado anterior, UIX-003) e a biblioteca de
três colunas reaparece como estava. Na tela Detalhes, B (Esc) volta a Navegar (BBX-016).
· verificação: teste de componente.

### BBX-003 — O Big Box só navega
· alterado em 2026-11-22 (docs/specs/phase-15-big-box-v2, BBX-059): além de `LaunchGame`, `LaunchGameUntracked` e `BringToFront`,
o Big Box chama `TimeToBeatGet` (leitura que pode buscar no HowLongToBeat); `TimeToBeatRefresh/Search/Choose/SetSettings` ficam
proibidos.
Dado o Big Box, então ele não mostra nem alcança estatísticas, emuladores, plataformas, provedores, importação, edição, remoção,
busca de metadados nem "Buscar vídeo"; não abre modal; e não chama nenhum binding de escrita além de `LaunchGame`,
`LaunchGameUntracked` e `BringToFront`. Tudo isso continua na janela normal.
· verificação: teste que varre `frontend/src/bigbox/` (sem importar esses componentes e bindings) e teste de componente com
espiões.

### BBX-004 — Abas de plataforma
· alterado em 2026-11-22 (docs/specs/phase-15-big-box-v2, BBX-054): um tema pode declarar abas de coleção (Todos, Favoritos,
Concluídos) antes das plataformas; com abas de plataforma, vale este critério.
Dado o catálogo, então a barra de plataformas mostra "Todos" e cada plataforma com pelo menos um jogo (`gameCount > 0`, como
PLT-032), na ordem do catálogo, **só em texto** (sem ícone). A aba inicial é a plataforma escolhida na coluna esquerda da janela
normal, ou "Todos" se nenhuma.
· verificação: teste de componente com catálogo falso.

### BBX-005 — LB/RB trocam de aba
Dado a tela Navegar, quando aperto RB (PageDown), então a aba seguinte fica ativa; LB (PageUp), a anterior; da última, RB volta
a "Todos" e de "Todos", LB vai à última (circular). A lista passa a ser a da aba (`game.list` com o filtro só de plataforma,
ordem por título) e o foco vai para o primeiro jogo dela.
· verificação: teste do reducer de navegação e de componente.

### BBX-006 — Faixa de capas
· alterado em 2026-11-22 (docs/specs/phase-15-big-box-v2, BBX-051): a âncora da janela passa a ser do tema (Faixa: 2, sem mudança).
· alterado em 2026-11-16: a janela de capas no DOM passa a ser a de BBX-039 (a janela centrada deixava a faixa vazia à direita
no começo da lista); as pontas são as da aba inteira (`total`), e o foco não entra em jogo ainda não lido (BBX-007).
Dado a tela Navegar do Faixa, então os jogos da aba aparecem numa faixa horizontal no topo, com a capa em foco maior que as
outras; ←/→ (direcional, analógico esquerdo, setas) movem o foco um jogo por vez, sem dar a volta nas pontas; a faixa rola para
manter o jogo em foco à vista. No máximo 15 capas ficam no DOM, na janela de BBX-039.
· verificação: teste do reducer (pontas) e de componente (contagem de nós).

### BBX-007 — Pré-busca à frente do foco
· alterado em 2026-11-22 (docs/specs/phase-15-big-box-v2, BBX-050): vale para os temas de faixa; a grade tem regra própria de
pré-busca.
· alterado em 2026-11-16: volta como pré-busca à frente do foco, com páginas do tamanho de BBX-038 (aprovação do usuário: a aba
não é lida inteira; a remoção proposta no mesmo dia não foi aprovada). Antes: próxima página de 200 com o foco a 10 do fim do
carregado.
Dado a aba com `T` jogos (BBX-039), `L` jogos lidos, o foco `f` e a capacidade `V` (BBX-024), quando `L < T` e há menos de
`V + 3` jogos lidos a partir do foco, contando-o (`L − f < V + 3`), então o Big Box pede a próxima página (`offset = L`, tamanho
de BBX-038), sem nenhuma ação extra do usuário. A condição é reavaliada a cada mudança de `f`, de `L` e de `V` (redimensionar a
janela). Há no máximo uma leitura em voo por aba; enquanto ela não chega, a condição verdadeira não pede outra. Com `L ≥ T`, ou
depois de uma página vazia, não pede mais nada. Se a página atrasar, → no jogo `L − 1` não faz nada (o foco nunca entra em jogo
não lido) e a faixa só desenha jogos lidos.
· verificação: teste da função pura `shouldPrefetch` (limiar, `L ≥ T`, página vazia, `V` maior depois de resize) e de componente
do shell com `ListGames` falso controlado por promessas (andar para a direita pede cada página uma vez, nunca duas em voo; →
parado em `L − 1` com a página pendente).

### BBX-008 — Capa ou substituto
Dado um jogo da faixa, então a capa é a mídia `cover`; sem capa, a mesma substituta da janela normal (`useCardImages`: tela de
título, de seleção, screenshot, banner, fundo); sem nenhuma imagem, um cartão de placeholder com o título em texto.
· verificação: teste de componente.

### BBX-009 — Bloco de informações
· alterado em 2026-11-15: a linha de informações não mostra discos (resposta 7).
Dado o jogo em foco, então o bloco à esquerda mostra: o título grande; a linha "plataforma · ano · gênero" (partes sem valor
são omitidas com o separador; o gênero é o primeiro); a descrição curta (até 280 caracteres,
cortada no último espaço antes do limite, com "…"; sem descrição, nada); "Tempo jogado" (formato da janela normal, UIX-028),
"Última vez" (data, ou "Nunca jogado") e "Sessões: N" (`playCount`); e os botões **Jogar** e **Detalhes**.
· verificação: teste de componente (cada parte ausente; corte da descrição em 280).

### BBX-010 — Arte de fundo
· alterado em 2026-11-22 (docs/specs/phase-15-big-box-v2, BBX-052): a ordem de escolha vale para os fundos `contain` e `cover`; o
fundo `plain` (Mosaico) não desenha arte.
Dado o jogo em foco, então o fundo em tela cheia é a primeira imagem existente na ordem `background`, `screenshot`, `title`,
`select`, `banner`, `cover`; sem nenhuma, um gradiente de tokens. A troca de jogo faz a transição de 300 ms entre as artes.
· verificação: teste de componente (ordem de escolha).

### BBX-011 — Vídeo depois de 2 s parado
· alterado em 2026-11-22 (docs/specs/phase-15-big-box-v2, BBX-053): o vídeo toca no fundo ou dentro da capa em foco, conforme o
tema; as regras de tempo, de um único `<video>` e de `src` removido continuam.
Dado o jogo em foco com uma mídia `video`, quando o foco fica 2.000 ms sem mudar, então um único elemento `<video>` com
`muted`, `loop` e `playsInline` passa a tocar sobre a arte (transição de 400 ms). Dada uma troca de foco antes dos 2.000 ms, então
nada toca e a contagem recomeça no jogo novo. Dada uma troca de foco com vídeo tocando, então o vídeo para e seu `src` é removido
(o arquivo não fica carregado). Na tela Detalhes e fora do Big Box nenhum vídeo do Faixa toca.
· verificação: teste de componente com temporizadores falsos.

### BBX-012 — Vídeo que não toca
Dado um `<video>` que dispara `error` (formato ou arquivo ausente), então a arte de fundo volta e o vídeo desse jogo não é tentado
de novo enquanto o foco não sair dele.
· verificação: teste de componente.

### BBX-013 — Som do vídeo
Dado o Big Box recém-aberto, então os vídeos tocam com som. Se o navegador recusar o som (sem interação do usuário ainda), o
vídeo toca mudo e ganha o som na próxima tecla ou clique. Quando aperto Y (M) com um vídeo visível, então o som desliga; de novo,
liga. A escolha vale até sair do Big Box (não é gravada). *(Alterado na 1.0.3: antes o som começava desligado.)* A dica "Y Som" aparece no rodapé só com um vídeo visível.
· verificação: teste de componente.

### BBX-014 — Jogar e Detalhes
Dado a tela Navegar com um jogo em foco, quando aperto A (Enter), então `LaunchGame(gameId)` é chamado uma vez (mantido apertado,
uma vez só; UIX-046); X (D) abre Detalhes do jogo. Com o mouse, clicar numa capa a põe em foco, duplo clique lança, e os botões
Jogar e Detalhes funcionam como A e X. Um lançamento pendente bloqueia novo lançamento (UI-021).
· verificação: teste de componente com espiões.

### BBX-015 — Rodapé com dicas do gamepad
· alterado em 2026-11-22 (docs/specs/phase-15-big-box-v2, BBX-054): com abas de coleção a dica é "LB/RB Aba"; nenhuma dica de
Select + A (ATB-013).
· alterado em 2026-11-16: na tela Detalhes as dicas seguem a zona em foco (BBX-032), e "Rolar" deixa de existir (a descrição não
recebe mais foco, BBX-036).
Dado a tela Navegar, então o rodapé mostra, em texto, "A Jogar", "X Detalhes", "LB/RB Plataforma", "Select+B Fechar o jogo" e
"B Sair". Na tela Detalhes com o foco num botão, "A Jogar" ou "A Jogar sem contar tempo" (o botão focado) e, se houver galeria,
"↓ Imagens"; com o foco na galeria, "←/→ Imagem" e "↑ Botões"; nos dois casos, "B Voltar" e "Select+B Fechar o jogo". "Y Som"
segue BBX-013 na tela Navegar; na tela Detalhes ela não aparece (Y nos detalhes está fora desta spec, decisão do usuário de
2026-11-16).
· verificação: teste de componente.

### BBX-016 — Tela de detalhes
· alterado em 2026-11-22 (docs/specs/phase-15-big-box-v2, TTB-016, BBX-056): a tela mostra o tempo para zerar (três valores e a
fonte); um tema pode ter tela de Detalhes própria com os mesmos estados de foco.
· alterado em 2026-11-15: sem discos (resposta 7).
· alterado em 2026-11-16: o foco passa a andar entre os botões e a galeria (BBX-032..BBX-035), a descrição rola sem receber foco
(BBX-036) e o layout segue [bigbox-faixa §1.1](contracts/bigbox-faixa.md) (BBX-043, BBX-044). Teste com controle: o foco não
chegava aos botões e a imagem da galeria não trocava.
Dado X sobre o jogo `G`, então a tela Detalhes mostra: título; descrição completa (rolagem de BBX-036); plataforma, ano,
gêneros, desenvolvedores, editoras e modos de jogo; tempo jogado, última vez e sessões; os botões **Jogar** e **Jogar sem contar
tempo**; e, se `G` tiver vídeo ou screenshots, uma galeria com o vídeo primeiro e depois os screenshots, num visor grande com a
fileira de miniaturas embaixo (BBX-034). O foco começa em **Jogar**. A aciona o botão focado (`LaunchGame` / `LaunchGameUntracked`);
com o foco na galeria, A não faz nada. B volta a Navegar com o foco em `G`.
· verificação: teste de componente.

### BBX-017 — Marcação de sessão
Dado um jogo com sessão aberta, então a capa dele na faixa e o bloco de informações mostram "Em execução" (Running, Grace) ou
"Pausado" (Paused), a partir de `session.changed` e `session.current` (BIG-004). A UI não calcula tempo (UI-017).
· verificação: teste de componente com eventos simulados.

### BBX-018 — Entrada suspensa enquanto o jogo roda
Dado um lançamento bem-sucedido feito pelo Big Box (rastreado ou sem contar tempo), então o Big Box ignora controle e teclado até:
(a) a sessão desse jogo terminar (`session.changed` com `ended`/`discarded`), (b) chegar `game:closedByHotkey`, ou (c) a janela
receber `focus` depois de ter recebido `blur` após o lançamento. Assim, A e B apertados dentro do jogo não lançam nem saem do Big
Box.
· verificação: teste do reducer e de componente (eventos de janela e de sessão simulados).

### BBX-019 — Select segurado bloqueia B e Y na UI
· alterado em 2026-11-22 (docs/specs/phase-15-big-box-v2, BBX-057): bloqueia também A (Select + A é o Big Box do agente) e ↑/↓ do
direcional (volume do agente).
Dado o botão Select/Back (botão 8 do mapeamento padrão) pressionado, então a UI ignora B e Y (inclusive a borda de subida que
acontecer enquanto Select está apertado): Select + B e Select + Y são do agente. A, direções, LB/RB e X continuam valendo.
· verificação: teste do hook de entrada com `navigator.getGamepads` falso.

### BBX-020 — Erros no Big Box
Dado um lançamento recusado (`-32002` sessão ativa, `-32003` falha, agente desconectado), então a mensagem do erro (textos de
UI-008) aparece numa faixa de aviso acima do rodapé, com `role="alert"`, até a próxima ação do usuário; o Big Box continua
navegável.
· verificação: teste de componente.

### BBX-021 — Aba ou biblioteca vazia
Dado uma aba sem jogos carregados ou a biblioteca vazia, então a tela Navegar mostra "Nenhum jogo nesta plataforma" (ou "Nenhum
jogo na biblioteca") e o rodapé só com "LB/RB Plataforma" e "B Sair".
· verificação: teste de componente.

### BBX-022 — Confirmação de metadados não abre sobre o Big Box
Dado o Big Box ativo, quando a busca em lote chega a `ready` (MET-067), então a janela de confirmação não abre sozinha; o
indicador do cabeçalho pisca como com outro modal aberto, e a janela abre pelo indicador depois de sair do Big Box.
· verificação: teste de componente (`App`).

### BBX-023 — Temas selecionáveis
· alterado em 2026-11-22 (docs/specs/phase-15-big-box-v2, BBX-046): `faixa`, `mosaico` e `estante` disponíveis; `roda`
indisponível; o seletor usa os nomes do registro.
Dado o registro de temas, então existem `faixa` (disponível), `mosaico` e `roda` (indisponíveis, "em breve"). Na janela normal,
ao lado do botão "Big Box", um seletor "Tema do Big Box" lista os três; os indisponíveis aparecem desativados. A escolha fica em
`localStorage` (`arcadia.bigBoxTheme`, `lib/uiPrefs.ts`); valor ausente, desconhecido ou indisponível vira `faixa`.
· verificação: teste de `uiPrefs` e de componente.

### BBX-024 — Contrato de tema
· alterado em 2026-11-22 (docs/specs/phase-15-big-box-v2, BBX-047, BBX-048): contrato v2 (eixo `grid` com colunas, âncora, fundo,
lugar do vídeo, abas, ordem, tempo para zerar em Navegar, Detalhes próprios); o reducer ganha a grade 2D.
· alterado em 2026-11-16: o tema declara também a capacidade da lista (`listCapacity`, função pura do tamanho da janela, usada
por BBX-039).
Dado um tema, então ele recebe só o modelo de exibição e as ações do Big Box ([bigbox-faixa §5](contracts/bigbox-faixa.md)) e
declara o eixo da lista (`horizontal` ou `vertical`) e quantos itens cabem inteiros nela para um tamanho de janela; a navegação (abas, foco, telas, suspensão) é um reducer puro compartilhado,
que não conhece o tema. Um tema de teste com eixo vertical navega com ↑/↓ pelo mesmo reducer.
· verificação: teste do reducer e de um tema falso.

### BBX-025 — Cores do Big Box só por tokens
· alterado em 2026-11-22 (docs/specs/phase-15-big-box-v2, BBX-058): cada tema disponível tem o bloco completo de tokens, com os
novos `--arc-bb-heading`, `--arc-bb-chip`, `--arc-bb-on-chip` e `--arc-bb-placeholder`.
· alterado em 2026-11-16: a lista de tokens ganha `--arc-bb-panel` (BBX-037).
Dado o código de `frontend/src/bigbox/`, então vale a regra de UIX-006 (nenhuma classe de cor da paleta, nenhuma cor literal); as
cores vêm dos tokens `--arc-bb-*`, definidos em `index.css` sob `[data-bigbox-theme="faixa"]`, e o elemento raiz do Big Box tem
`data-bigbox-theme` com o tema ativo.
· verificação: o teste de varredura de UIX-006 passa a incluir `bigbox/` e a lista de tokens de
[bigbox-faixa §6](contracts/bigbox-faixa.md).

### BBX-026 — Contraste sobre a arte
· alterado em 2026-11-22 (docs/specs/phase-15-big-box-v2, BBX-058, MOS-015, ETN-014): vale para o Faixa; o Mosaico e a Estante
têm os seus pares de contraste.
· alterado em 2026-11-16: o fundo do texto passa a ser o painel `--arc-bb-panel` (BBX-037). O véu `--arc-bb-scrim` não era
desenhado atrás do texto, e o texto ficava ilegível sobre artes claras.
Dado o tema Faixa, então `--arc-bb-text`, `--arc-bb-text-muted` e `--arc-bb-danger` têm contraste ≥ 4,5:1 com a cor do painel
(`--arc-bb-panel`) composta sobre branco puro **e** sobre preto puro (pior caso de qualquer arte), e o contorno de foco
(`--arc-bb-focus`) tem ≥ 3:1 nos dois casos. Os pares `on-accent` sobre `accent` e `text` sobre `surface` continuam ≥ 4,5:1.
· verificação: teste que lê os tokens de `index.css` e calcula o contraste (WCAG 2.x), como UIX-007.

### BBX-027 — Foco inicial
· alterado em 2026-11-16: "carregada" é a primeira página da aba (BBX-038); se o jogo está nela, o aviso de carregamento dura
até a faixa dele estar completa (BBX-038).
Dado a entrada no Big Box, então o foco vai para o jogo em foco da janela normal se ele estiver na aba inicial carregada; senão,
para o primeiro jogo.
· verificação: teste de componente.

### BBX-028 — Atualização ao vivo
Dado o jogo em foco, quando chega `media:changed` dele, então capa, arte e vídeo são relidos (o vídeo segue BBX-011 a partir da
chegada); quando a sessão de um jogo termina, então o bloco de informações relê o jogo (tempo, última vez, sessões), como UIX-031.
· verificação: teste de componente com eventos simulados.

### BBX-029 — Só leituras ao navegar
· alterado em 2026-11-22 (docs/specs/phase-15-big-box-v2, BBX-059): única exceção, a leitura do tempo para zerar pode fazer o agente
consultar o HowLongToBeat (e o IGDB como reserva) e gravar o cache; download de imagem e vídeo continua proibido.
Dado qualquer navegação no Big Box (abas, foco, detalhes, galeria, som), então só há leituras (`ListGames`, `GetGame`,
`MediaList`, `Catalog`, `CurrentSession`); nenhum download de vídeo ou imagem é pedido (decisão h).
· verificação: teste de componente com espiões.

### BBX-030 — Big Box na TV
Dado o Arcadia numa TV com controle, quando uso o Big Box Faixa, então troco de plataforma com LB/RB, ando pela faixa, o vídeo
entra depois de ~2 s parado e sai ao mexer, Y liga o som, X abre detalhes com a galeria, A lança, e Select + B fecha o jogo e me
devolve ao Big Box no mesmo jogo; o texto é legível a 3 m.
· verificação: manual.

### ~~BBX-031 — Número de discos gravado na importação~~
· status: removido em 2026-11-15 (resposta 7): sem `game.disc_count`, sem `GameDetail.discs` e sem mudança em `import.apply`.

### Ajustes do teste com controle (2026-11-16)

· status: **aprovado pelo usuário em 2026-11-16** (BBX-032..BBX-045 novos; BBX-006, BBX-007, BBX-015, BBX-016, BBX-024..BBX-027
alterados). Na aprovação: a aba é lida por páginas de `V + 3` com pré-busca à frente do foco (BBX-007, BBX-038, BBX-039; a
leitura inteira proposta saiu), entrou o contador "posição / total" (BBX-045) e Y nos detalhes ficou fora desta spec. Layout dos
detalhes em [bigbox-faixa §1.1](contracts/bigbox-faixa.md); estados em §3.

**Lacuna registrada:** o reducer já movia `detailsFocus` com ↑/↓ e `gallery` com ←/→, mas a spec não definia **uma** marca de
foco nem um visor: a tela desenhava o contorno de foco no botão **e** na miniatura selecionada ao mesmo tempo, a galeria era uma
fileira de miniaturas cortada embaixo e à direita, e trocar de item só movia um contorno pequeno fora da tela. O usuário via o foco
"preso" na galeria e a imagem sem trocar. BBX-032..BBX-035 fecham a lacuna; um teste do shell com controle falso cobre a fiação.

### BBX-032 — Foco dos detalhes entre os botões e a galeria
Dado a tela Detalhes de um jogo com galeria (vídeo ou screenshots) e o foco num botão, quando aperto ↓ (direcional, analógico
esquerdo, seta), então o foco vai para a galeria; com o foco na galeria, ↑ devolve o foco ao último botão focado (Jogar ou Jogar
sem contar tempo). ↑ nos botões e ↓ na galeria não fazem nada. Sem galeria, ↓ não faz nada. Dada a galeria que fica vazia com o
foco nela (`media:changed`), então o foco volta ao último botão focado.
· verificação: teste do reducer e teste do shell com `navigator.getGamepads` falso (X, ↓, ↑).

### BBX-033 — ←/→ entre os botões
Dado o foco num botão da tela Detalhes, quando aperto →, então o foco vai de **Jogar** para **Jogar sem contar tempo**; ←
volta; sem dar a volta nas pontas. A imagem da galeria não muda.
· verificação: teste do reducer e teste do shell com controle falso.

### BBX-034 — ←/→ na galeria troca a imagem do visor
Dado o foco na galeria com N itens (N ≥ 1) e o item `i` selecionado, quando aperto →, então o item `i + 1` passa a ser mostrado
inteiro no visor (`object-fit: contain`, sem corte) e fica marcado como selecionado na fileira de miniaturas, que rola para
mantê-lo à vista; ← mostra `i − 1`; sem dar a volta nas pontas. O visor mostra o contador "`i + 1` / N". Selecionado o vídeo, ele
toca no visor (`muted` conforme BBX-013, `loop`, `playsInline`); sair do vídeo para outro item o para e remove o `src`; nunca há mais de um
`<video>` na tela Detalhes. Dada a galeria encolhida ao vivo, então o selecionado passa a ser `min(i, N − 1)`.
· verificação: teste do reducer e teste de componente de `BigBoxDetails` (visor, contador, miniatura selecionada, vídeo).

### BBX-035 — Uma única marca de foco
Dado a tela Detalhes, então só o elemento com o foco tem o contorno `--arc-bb-focus` (`data-focused="true"`): o botão focado, ou
a fileira de miniaturas quando o foco está na galeria. A miniatura selecionada sem foco na galeria fica marcada de outro jeito
(`aria-current="true"`, borda `--arc-bb-text-muted`), nunca com o contorno de foco.
· verificação: teste de componente (exatamente um `data-focused="true"` por estado de foco).

### BBX-036 — Rolagem da descrição sem foco
Dado a tela Detalhes, quando inclino o analógico direito para cima ou para baixo (ou aperto PageUp/PageDown no teclado), então a
descrição rola um passo nesse sentido, com o foco onde estiver; a descrição não recebe foco. Na tela Navegar, PageUp/PageDown
continuam trocando de aba (BBX-005).
· verificação: teste do hook de entrada e de componente.

### BBX-037 — Painel atrás de todo texto
Dado o Big Box (Navegar e Detalhes), então todo texto desenhado sobre a arte ou o vídeo de fundo fica dentro de um painel
semitransparente escuro com a cor `--arc-bb-panel` (`data-bb-panel`): a barra de abas; o bloco de informações (título, linha de
dados, descrição curta, tempo jogado, última vez, sessões e botões); o aviso de lista vazia e o de carregamento; o rodapé com as
dicas; o contador da faixa (BBX-045); na tela Detalhes, a coluna de texto (título, linhas de dado, tempo, botões e descrição) e o
contador da galeria. O aviso de
erro (`role="alert"`) e os selos de sessão continuam sobre `--arc-bb-surface` (opaco). O contraste do painel é o de BBX-026.
· verificação: teste de componente (cada texto listado tem um ancestral `data-bb-panel` antes de chegar à raiz) e o teste de
tokens de BBX-026.

### BBX-038 — Leitura da aba por páginas
· alterado em 2026-11-22 (docs/specs/phase-15-big-box-v2, BBX-050, BBX-055): o tamanho de página `V + 3` vale para os temas de
faixa (a grade usa o seu); a ordem é a do tema (`title` ou `lastPlayed`).
· alterado em 2026-11-16 (aprovação do usuário): a aba não é mais lida inteira antes de aparecer; é lida por páginas pequenas,
com pré-busca à frente do foco (BBX-007).
Dado a entrada no Big Box ou a troca de aba, com a capacidade `V` (BBX-024), então o Big Box lê a aba por páginas de `ListGames`
(filtro só de plataforma, ordem por título, `limit = V + 3`, com `V` do momento do pedido; nunca acima de 500), começando pelo
`offset` 0, e mostra a faixa e o bloco de informações assim que a primeira página chega. "Carregando jogos…" aparece só na
primeira leitura da aba (a primeira página; na entrada do Big Box, também a página seguinte quando o jogo inicial, BBX-027, precisa
dela para completar a faixa); as páginas seguintes chegam sem aviso, e LB/RB continuam trocando de aba durante qualquer leitura.
Mover o foco dentro da aba nunca cancela nem repete uma leitura em voo. Trocar de aba cancela a leitura em voo da aba antiga (a
resposta que chegar depois é descartada) e não pede mais páginas dela. Falha na primeira página: o aviso de BBX-020 com a mensagem
do erro e o texto de aba vazia de BBX-021; voltar à aba tenta de novo. Falha numa página seguinte: o aviso de BBX-020, a faixa
mantém o que já foi lido e a nova tentativa vem na próxima mudança de foco que ainda pedir página (BBX-007), nunca em laço. As
páginas de uma aba já lidas voltam sem nova leitura enquanto o Big Box está aberto.
· verificação: teste do hook de dados com `ListGames` falso controlado por promessas (primeira página `offset 0`/`limit V + 3`;
sem leitura repetida ao mover o foco; troca de aba no meio descarta a resposta tardia; falha na primeira e na segunda página;
volta sem releitura) e de componente do shell ("Carregando jogos…" só antes da primeira página).

### BBX-039 — Faixa sem buracos
· alterado em 2026-11-16 (aprovação do usuário): `T` é o total da aba informado pela primeira página, não o número de jogos lidos;
com a pré-busca de BBX-007 a janela cai sempre dentro do que já foi lido.
Dado a aba com `T` jogos (o `total` devolvido pela primeira página de `ListGames`, igual ao `gameCount` da plataforma no catálogo,
PLT-032; em "Todos", a biblioteca inteira), o foco `f` e `V` capas que cabem inteiras na largura da faixa (capacidade do tema,
BBX-024, de 1 a 14), então a faixa desenha os jogos `[s, min(s + V + 1, T))` com `s = clamp(f − 2, 0, max(T − V, 0))`: com
`T > V`, a faixa fica cheia até a borda direita (a capa `V + 1` aparece cortada na borda, como sinal de que há mais) e nunca há
espaço vazio à direita antes do último jogo da aba; a capa em foco fica sempre inteira à vista (posição ≤ `V − 1`). Ao trocar de
aba, a faixa aparece cheia desde a primeira capa (a primeira página tem `V + 3` jogos). Andando para a direita com as páginas
chegando no tempo de BBX-007, todo índice da janela já está lido.
· verificação: teste da função pura `stripWindow` (começo, meio, fim, `T ≤ V`, `T = 0`), varredura de `f` de 0 a `T − 1` com
a regra de BBX-007 (janela sempre dentro de `[0, L)`) e de componente (nós e índices desenhados ao entrar numa aba de 40 jogos
com `V = 12`, com só a primeira página de 15 lida).

### BBX-040 — Fundo inteiro, sem corte
· alterado em 2026-11-22 (docs/specs/phase-15-big-box-v2, BBX-052): vale para o fundo `contain` (Faixa); a Estante usa `cover` com
degradê e o Mosaico, fundo liso.
Dado a arte ou o vídeo de fundo do jogo em foco (Navegar e Detalhes), então ele é desenhado inteiro, centrado, com
`object-fit: contain` (sem zoom nem corte), sobre um preenchimento: a mesma arte em `object-fit: cover`, desfocada (blur ≥ 24 px)
e escurecida por `--arc-bb-scrim`; sem arte, o gradiente de BBX-010. A transição de 300 ms (BBX-010) vale para o par
preenchimento + arte.
· verificação: teste de componente de `BackgroundStage` (camadas, `object-fit`, desfoque; vídeo também em `contain`).

### BBX-041 — Capas e miniaturas inteiras
Dado uma capa da faixa ou uma miniatura da galeria dos detalhes, então a imagem aparece inteira dentro do cartão
(`object-fit: contain`, centrada), sem corte, com o espaço que sobrar em `--arc-bb-surface`.
· verificação: teste de componente.

### BBX-042 — Área segura
Dado o Big Box em qualquer tela, então todo o conteúdo (abas, faixa, bloco de informações, tela Detalhes, aviso e rodapé) fica
dentro da área segura: 5 % da altura da janela no topo e embaixo e 5 % da largura nas laterais; só o fundo (BBX-040) ocupa a
janela inteira.
· verificação: teste de componente (o contêiner `data-bb-safe` tem o recuo declarado e contém tudo menos o fundo) e manual
(BBX-030).

### BBX-043 — Detalhes cabem na tela
Dado a tela Detalhes numa janela 16:9 (1280×720, 1920×1080 ou 3840×2160), então título, linhas de dado, botões, visor e a
fileira de miniaturas inteira cabem na área segura acima do rodapé, sem rolagem da página; só a descrição rola (BBX-036), dentro
da altura que sobrar na coluna de texto.
· verificação: teste de componente (contêiner com altura limitada e `overflow: hidden`; visor e miniaturas dentro dele; só a
descrição com `overflow-y: auto`) e manual nas três resoluções.

### BBX-044 — Linhas de dado compactas
Dado a coluna de texto dos detalhes, então as linhas de dado (plataforma · ano · gêneros; desenvolvedores; editoras; modos de
jogo; tempo jogado · última vez · sessões) formam um bloco único (`data-bb-facts`) sem margem entre parágrafos, com espaçamento de
no máximo 0,25em entre linhas e altura de linha de no máximo 1,4; entre título, bloco de dados, botões e descrição, no máximo
0,75em.
· verificação: teste de componente (espaçamentos declarados no bloco) e manual (BBX-030).

### BBX-045 — Contador de posição na faixa
· novo em 2026-11-16 (aprovação do usuário).
Dado a tela Navegar com a aba de `T ≥ 1` jogos (`T` de BBX-039) e o foco `f`, então a faixa mostra, em texto e dentro de um
painel (BBX-037), o contador "`f + 1` / `T`" (ex.: "7 / 120"), que acompanha cada movimento do foco e usa o total da aba mesmo
antes de todas as páginas serem lidas. Enquanto "Carregando jogos…" aparece ou com a aba vazia, o contador não aparece.
· verificação: teste do modelo (`position` e `total`) e de componente do Faixa (texto "7 / 120" com foco 6 e `T` 120 e só a
primeira página lida; ausente com `loading` e com `T = 0`; ancestral `data-bb-panel`).

---

## VID — Vídeo

Regras do provedor em [contracts/screenscraper-rules.md](contracts/screenscraper-rules.md).

### VID-001 — Migração 9: tipo de mídia `video`
· alterado em 2026-11-15: a migração só acrescenta o tipo `video` (sem `game.disc_count`, resposta 7).
Dado um banco novo ou na versão 8 com dados, quando o agente abre o store com migração, então `media.kind` e `media_job.kind`
aceitam `video` além dos seis tipos atuais (tabelas reconstruídas na mesma transação, com todos os dados e o `UNIQUE (game_id,
hash)` preservados), a tabela `game` não muda, `store.SchemaVersion = 9` e a versão do goose é 9. O `Down` apaga as linhas
`video` e reconstrói as tabelas sem o tipo.
· verificação: teste de migração do store (banco novo; v8 com mídias e jobs; `Down`).

### VID-002 — Um vídeo por jogo
Dado `domain.MediaKindLimit("video")`, então é 1. Dado um jogo com um vídeo ou com um job de vídeo pendente ou em andamento,
então nenhum job de vídeo novo é criado para ele (a contagem de ADR 0034: atuais + enfileirados); nada é apagado.
· verificação: teste de unidade do domínio; teste do agente com store real.

### VID-003 — O que é um job de vídeo
Dado um job de vídeo criado por qualquer caminho, então ele é uma linha de `media_job` com `kind = video`,
`provider = screenscraper` e `source_url = "screenscraper:lookup"` (sem título, sem credencial); o jogo é lido na hora do
download (título e plataforma atuais).
· verificação: teste do agente com store real.

### VID-004 — Busca automática depois de aplicar metadados
Dado `metadata.apply` ou a aplicação do lote (MET-065) de um jogo `G` com sucesso, quando as duas credenciais do ScreenScraper
estão cadastradas (SSC-001), a plataforma de `G` tem sistema no ScreenScraper ([rules §1](contracts/screenscraper-rules.md)) e
VID-002 permite, então um job de vídeo de `G` é criado. O resultado de `metadata.apply` não muda e nenhuma notificação nova é
enviada; faltando uma condição, nada é criado (registro em `Debug`).
· verificação: teste do agente (cada condição; aplicação única e em lote).

### VID-005 — `video.queue`
Dado `video.queue {gameIds}` com 1 a 1.000 ids distintos ≥ 1, então para cada jogo é criado um job de vídeo ou o jogo é listado em
`skipped` com o motivo `has-video` (VID-002) ou `platform-unmapped`; o resultado é `{queued, skipped: [{gameId, reason}]}`. Dado
um id inexistente, então `-32001` e nada é criado. Dado uma credencial ausente, então `-32010` com `data.provider` =
`screenscraperdev` ou `screenscraper` (o primeiro que falta, nessa ordem), sem nada criado. Lista vazia, maior que 1.000, repetida
ou com id < 1: `-32602`.
· verificação: teste do agente (um caso por regra).

### VID-006 — `video.queueMissing`
Dado `video.queueMissing {platformId?}`, então são criados jobs de vídeo para todos os jogos (da plataforma, se informada) cuja
plataforma tem sistema no ScreenScraper e que VID-002 permite, numa transação; o resultado é `{queued, skipped}` (contagens).
Plataforma inexistente: `-32001`. Credencial ausente: `-32010` como em VID-005.
· verificação: teste do agente com store real.

### VID-007 — `video.cancelPending`
Dado `video.cancelPending {}`, então todo job de vídeo pendente é cancelado, o que estiver baixando é interrompido (como
`media.cancel`), e o resultado é `{cancelled: N}`. Jobs de imagem não mudam.
· verificação: teste do agente.

### VID-008 — `video.status`
Dado `video.status {}`, então o resultado traz `pending`, `running` e `failed` (contagens de jobs de vídeo), `lane`
(`active`, `paused-quota`, `paused-credentials` ou `no-credentials`), `pausedUntil` (só em `paused-quota`) e, quando o
ScreenScraper já informou, `requestsToday` e `maxRequestsPerDay`.
· verificação: teste do agente.

### VID-009 — Raia de vídeo separada das imagens
· alterado em 2026-11-15: critério único de raia (revisão do diff: o despacho usava `provider` e o roteador do fetch, `kind`).
Dado jobs de vídeo pendentes, então eles rodam numa raia própria com **um** download por vez, que não ocupa as 2 vagas das
imagens: com 100 vídeos pendentes, um job de imagem novo começa na hora. A raia pausada (VID-016, VID-017) não atrasa imagens.
Um job é de vídeo **se e somente se** `kind = video`: o despacho da fila (qual vaga ocupa) e o roteador do fetch (quem baixa)
usam esse mesmo critério, então um job com `kind = video` ocupa a vaga de vídeo e vai ao resolvedor de vídeo e qualquer outro
ocupa uma vaga de imagem e vai ao `media.Store`, seja qual for o `provider`.
· verificação: teste da fila com relógio fake e fetchers falsos (ordem e concorrência; job com `kind` e `provider`
cruzados).

### VID-010 — Identificação por nome e sistema
Dado um job de vídeo do jogo `G`, então a busca usa `domain.SearchTerm(título de G)` e o `systemeid` da chave de seed da
plataforma de `G` ([rules §1, §2](contracts/screenscraper-rules.md)); nenhum arquivo do jogo é aberto ou lido e nenhum hash é
enviado (IMV-080). Plataforma sem chave de seed ou sem sistema: o job falha de vez com "plataforma sem vídeo no ScreenScraper"
(sem requisição).
· verificação: teste do resolvedor com `httptest` (parâmetros enviados) e `fs` espião.

### VID-011 — Só um match baixa vídeo
Dado os jogos devolvidos pela busca, então cada um vira um grupo de membros (um por nome regional) pontuado por
`domain.ScoreGroup` com a plataforma confirmada pelo sistema e o ano de `G`, e classificado por `domain.ClassifyMatch`
(limiares da fase 12). Só `match` segue; `doubt` e `none` fazem o job falhar de vez com "sem correspondência no ScreenScraper",
sem nova tentativa. Os vetores de [rules §3](contracts/screenscraper-rules.md) valem.
· verificação: teste de unidade do resolvedor com os vetores.

### VID-012 — Só o `video-normalized`
Dado o jogo escolhido, então a mídia baixada é a de tipo `video-normalized` (primeira na ordem de região de
[rules §4](contracts/screenscraper-rules.md)); o `video` original nunca é baixado. Sem `video-normalized`, o job falha de vez com
"sem vídeo no ScreenScraper".
· verificação: teste do resolvedor com respostas gravadas (`httptest`).

### VID-013 — Teto de 20 MB
Dado o vídeo escolhido, quando o tamanho informado pela API ou o `Content-Length` passa de 20.971.520 bytes, então nada é baixado;
quando o corpo passa desse limite durante o download, então o download para e o arquivo temporário é apagado. Nos três casos o job
falha de vez com "vídeo maior que 20 MB" e nenhuma linha de `media` é criada.
· verificação: teste de `media` com `httptest` (cada caso; pasta sem resto).

### VID-014 — Formato e gravação
Dado o corpo baixado, então ele é aceito só se for MP4 (`ftyp` nos bytes 4..8); outro conteúdo falha de vez com "formato de vídeo
não suportado". O download vai para um arquivo temporário com o SHA-256 calculado no caminho (sem carregar tudo na memória) e é
renomeado para `<mediaRoot>/<G>/<sha256>.mp4`; a linha de `media` tem `kind = video` e `thumb_path = ''`. Conteúdo igual já
gravado não cria linha nova (`UNIQUE (game_id, hash)`).
· verificação: teste de `media` com `httptest`.

### VID-015 — Intervalo entre requisições e uma por vez
Dado qualquer requisição ao ScreenScraper (busca, informações do jogo e download do vídeo), então ela passa pelo
`scrapers.Limiter` do ScreenScraper: no mínimo 1.500 ms entre requisições, ou `60.000 / maxrequestspermin` ms se a conta informar
um limite por minuto menor; nunca duas ao mesmo tempo (`maxthreads` 1).
· verificação: teste do limitador e do resolvedor com relógio fake.

### VID-016 — Cota diária esgotada
Dado uma resposta de cota esgotada ([rules §6](contracts/screenscraper-rules.md)), ou `requestsToday ≥ maxRequestsPerDay` na última
resposta, então a raia de vídeo pausa por 1 h (relógio fake), o job em curso volta a pendente **sem** gastar tentativa, é
registrado `Warn` "cota do ScreenScraper esgotada" uma vez por pausa, e `video.status` mostra `paused-quota` com `pausedUntil`.
Vencida a pausa, a raia tenta de novo.
· verificação: teste da fila com `httptest` e relógio fake.

### VID-017 — Credenciais recusadas
Dado uma resposta de credencial recusada ([rules §6](contracts/screenscraper-rules.md)), então a raia pausa até uma das duas
credenciais do ScreenScraper ser gravada ou removida, o job volta a pendente sem gastar tentativa, é registrado `Warn`
"credenciais do ScreenScraper recusadas" uma vez, e `video.status` mostra `paused-credentials`.
· verificação: teste da fila.

### VID-018 — Falha temporária
Dado rede fora, 429 ou 5xx do ScreenScraper, então vale a regra da fila (MID-008: nova tentativa em 30 s, até 3 tentativas).
· verificação: teste da fila.

### VID-019 — Sem credenciais
Dado jobs de vídeo pendentes e uma das credenciais ausente, então nenhuma requisição é feita, os jobs ficam pendentes sem gastar
tentativa e `video.status` mostra `no-credentials`; quando as duas passam a existir (`provider.setKey`), a raia volta sozinha.
· verificação: teste da fila.

### VID-020 — Credenciais nunca em log, banco ou URL gravada
Dado qualquer fluxo da fase (busca, download, erro, cancelamento, status), então `devpassword`, `sspassword`, `devid` e `ssid` não
aparecem em log, em `media_job`, em `media.source_url`, em erro devolvido pelo IPC nem em `video.status`: a URL gravada e a de
log passam por `redactURL`, que remove esses quatro parâmetros, e o erro de rede é reescrito com a URL sem eles.
· verificação: teste que roda os fluxos com credenciais de valor marcado e procura o valor no log capturado, no banco e nas
respostas.

### VID-021 — Aviso quando o vídeo chega
Dado um job de vídeo que grava a mídia (`added = true`), então `media.changed {gameId}` é enviado como em IMG-001..IMG-004; falha,
cancelamento e pausa não avisam.
· verificação: teste do agente com dois clientes IPC.

### VID-022 — Vídeo servido ao WebView
Dado `GET /media/<G>/<hash>.mp4` no servidor de mídia do desktop, então a resposta tem `Content-Type: video/mp4` e aceita
`Range` (206 com `Content-Range`); as regras de caminho de `serveMedia` (fora da pasta, pasta ou ausente → 404) continuam.
· verificação: teste de `cmd/arcadia` com `httptest`; manual na WebView (BBX-030).

### VID-023 — O vídeo no carrossel da janela normal
Dado `media.list` com itens `video`, então o carrossel do painel (UIX-026) mostra o vídeo (com controles, em loop) logo depois
das screenshots e antes de título, seleção, fundo e banner, e o conta; a capa substituta dos cartões (`useCardImages`) continua
ignorando vídeo; o tipo `video` existe no tipo TypeScript de mídia. *(Alterado na 1.0.3: antes a janela normal ignorava vídeos.)*
· verificação: teste de componente.

### VID-024 — "Buscar vídeo" no painel
Dado o painel do jogo em foco, quando as duas credenciais do ScreenScraper estão cadastradas e o jogo não tem vídeo, então há o
botão "Buscar vídeo", que chama `VideoQueue([id])` e mostra "Vídeo na fila", "Este jogo já tem vídeo ou busca em andamento" ou
"A plataforma não tem vídeo no ScreenScraper"; erros seguem UI-008. Sem credenciais ou com vídeo, o botão não aparece.
· verificação: teste de componente.

### VID-025 — Vídeo sai com o jogo
Dado a remoção do jogo `G` com vídeo gravado e um job de vídeo em andamento, então o download é interrompido antes de apagar a
pasta (DEL-008) e `<mediaRoot>/<G>` some com o `.mp4` (DEL-007).
· verificação: teste do agente (remoção com job de vídeo em curso, fetcher falso bloqueado).

### VID-026 — Vídeo de ponta a ponta
Dado o Arcadia com as credenciais do ScreenScraper, quando aplico metadados a um jogo de SNES e peço "Buscar vídeos dos jogos sem
vídeo" para a biblioteca, então os vídeos chegam (≤ 20 MB, `.mp4`), aparecem no Big Box sem reabrir, a pasta `data\media` cresce
cerca de 2 MB por jogo, a cota mostrada sobe, e nenhuma credencial aparece em `data\logs`.
· verificação: manual.

### VID-027 — Credencial só vai ao ScreenScraper
· acrescentado em 2026-11-15 (revisão do diff: a URL de mídia vem da resposta JSON e recebia as quatro credenciais sem
conferência).
Dado uma URL de mídia vinda da resposta da API (`medias[].url`), quando o cliente vai abri-la, então, **antes** de acrescentar
`devid`, `devpassword`, `ssid` e `sspassword` e antes do limitador, ela só é aceita com esquema `https` e host igual ao do
`BaseURL` ou terminado em `.screenscraper.fr`; `http` só é aceito quando o próprio `BaseURL` é `http` (o `httptest` dos
testes) e o host é o do `BaseURL` ([rules §10](contracts/screenscraper-rules.md)). Recusada (inclusive URL inválida, relativa
ou sem host), nenhuma requisição é feita, o erro é `scrapers.ErrBadRequest` e o job falha de vez (`*media.PermanentError`, sem
nova tentativa) com "URL de vídeo fora do ScreenScraper"; nenhuma credencial aparece no erro, no `last_error` nem no log.
· verificação: teste do cliente com `httptest` (vetores de rules §10; servidores que contam requisições) e do resolvedor.

### VID-028 — Redirecionamento só para o mesmo host
· acrescentado em 2026-11-15 (revisão do diff).
Dado qualquer requisição do cliente do ScreenScraper (busca, informações e vídeo), quando a resposta redireciona, então o
redirecionamento só é seguido se o host de destino for igual ao da requisição original (sem diferença de caixa, com a porta) e o
esquema não cair de `https` para `http`; senão, a requisição ao outro host não é feita (nenhuma credencial é reenviada a ele),
o erro é `scrapers.ErrBadRequest` e o job falha de vez com "redirecionamento para fora do ScreenScraper", sem credencial no erro,
no `last_error` nem no log ([rules §11](contracts/screenscraper-rules.md)).
· verificação: teste do cliente com dois `httptest` (o segundo conta requisições e nunca é chamado).

### VID-029 — Aviso em texto no lugar do vídeo
· acrescentado em 2026-11-15 (revisão do diff: um aviso de cota no corpo virava "formato de vídeo não suportado").
Dado o download do vídeo com status 200 e `Content-Type` `text/*` (qualquer subtipo e parâmetros, sem diferença de caixa), então
o corpo (lido até 4.096 bytes e fechado) passa pela classificação de erros em texto de [rules §6](contracts/screenscraper-rules.md)
em vez de ir a `FetchVideo`: aviso de cota → cota esgotada (VID-016), software bloqueado → credencial recusada (VID-017), API
fechada → falha temporária (VID-018), não encontrado → falha de vez com "sem vídeo no ScreenScraper"; texto não reconhecido →
falha de vez com "formato de vídeo não suportado" (VID-014). Em todos os casos nenhum arquivo temporário e nenhuma linha de
`media` são criados.
· verificação: teste do cliente e do resolvedor com `httptest` (um caso por texto; pasta de mídia sem resto).

---

## SSC — Credenciais do ScreenScraper

### SSC-001 — Dois provedores de credencial
Dado `provider.setKey`, então `screenscraper` (conta do usuário: `key` = usuário, `secret` = senha) e `screenscraperdev`
(desenvolvedor do Arcadia: `key` = devid, `secret` = devpassword) são aceitos, ambos com `secret` obrigatório (1..512); sem `secret`
é `-32602`. Ficam no Credential Manager em `arcadia/screenscraper` e `arcadia/screenscraperdev` (valor `id\nsenha`, como o IGDB).
`provider.keyStatus` e `provider.deleteKey` aceitam os dois; nenhum devolve o valor (MID-001).
· verificação: teste do agente com `KeyStore` falso.

### SSC-002 — ScreenScraper na janela dos provedores
Dado a janela "Chaves dos provedores", então há a seção "ScreenScraper (vídeos)" com dois grupos: "Conta do usuário" (Usuário,
Senha) e "Desenvolvedor do Arcadia" (devid, devpassword), cada um com "Cadastrada"/"Não cadastrada", Salvar (ativo com os dois
campos preenchidos) e Remover; os campos são do tipo senha, sem autocompletar, e ficam vazios depois de salvar.
· verificação: teste de componente.

### SSC-003 — Vídeos da biblioteca na janela dos provedores
Dado a seção do ScreenScraper com as duas credenciais cadastradas, então há "Buscar vídeos dos jogos sem vídeo"
(`VideoQueueMissing(0)`, mostra "N vídeos na fila"), "Cancelar buscas pendentes" (`VideoCancelPending`) e uma linha de estado de
`VideoStatus` ("N pendentes · M sem vídeo", mais "cota esgotada até HH:MM", "credenciais recusadas" ou "R de D requisições hoje"
quando houver), relida a cada 5 s com a janela aberta e a cada `media:changed`.
· verificação: teste de componente com temporizadores falsos.

### SSC-004 — `softname` fixo
Dado qualquer requisição ao ScreenScraper, então `softname` é `arcadia` e `output` é `json` ([rules §2](contracts/screenscraper-rules.md)).
· verificação: teste do cliente com `httptest`.

### SSC-005 — ScreenScraper fora da busca de metadados
Dado `metadata.search`, `metadata.apply` e o lote, então o ScreenScraper não é consultado para texto nem imagens; a tabela de
aplicabilidade da fase 12 não muda.
· verificação: teste do agregador (nenhuma chamada ao fake do ScreenScraper).

### SSC-006 — Credencial do ScreenScraper sem quebra de linha
· acrescentado em 2026-11-15 (revisão do diff: o valor é gravado como `id\nsenha`, e uma quebra de linha dentro de uma das
metades troca o par lido por `Pair`).
Dado `provider.setKey` de `screenscraper` ou `screenscraperdev`, quando `key` ou `secret` contém `\n` ou `\r` em qualquer
posição (inclusive no começo ou no fim, sem `trim` antes da conferência), então a resposta é `-32602`, nada é gravado no Credential
Manager, a credencial anterior continua valendo e o gancho de credenciais (VID-017) não é chamado. `scrapers.Keys.SetPair`
recusa o mesmo caso com erro, sem gravar, para qualquer chamador.
· verificação: teste do agente com `KeyStore` falso e teste de `Keys` (um caso por posição e por caractere).

---

## Fora desta spec

- **Temas Mosaico e Roda** (só descritos em [design §4.6](design.md) e preparados no registro, BBX-023, BBX-024) e **Vitrine**.
- **Vídeo por plataforma** (decisão k): a conferência registra se a API o fornece; entra com o Roda.
- **ScreenScraper para texto, imagens e por hash** (decisão j, IMV-080).
- **Fechar o jogo pelo teclado ou por botão da UI.**
- **Gravar e mostrar o número de discos** (resposta 7).
- **Configurar combos, tempo de segurar e teto de vídeo** pelo usuário.
- **Escolher o vídeo manualmente** quando a busca dá "em dúvida".
- **Logos, caixa 3D e ícones** como mídia.
- **Movimento reduzido (`prefers-reduced-motion`)** no Big Box (rolagem da faixa, transições, vídeo de fundo): **não é requisito
  desta fase** (revisão do diff, 2026-11-15); nenhum teste da fase o exige e nenhum ID o cobre.
- **Ligar e desligar o som com Y (M) na tela Detalhes** (decisão do usuário, 2026-11-16): não é exigido. O vídeo do visor segue a
  escolha de som feita em Navegar (BBX-013, BBX-034), a dica "Y Som" não aparece nos detalhes (BBX-015) e nenhum teste exige Y
  nessa tela.
- **Recusar quebra de linha nas credenciais do IGDB** (`SetIGDB`, mesmo formato `id\nsegredo`): fora desta fase; SSC-006 vale
  só para `screenscraper` e `screenscraperdev`.

## Limitações conhecidas

1. **Processo que saiu do Job por breakaway** (ADR 0016) não é morto por Select + B; ele não era rastreado.
2. **O pedido de fechar pode não bastar:** jogo que ignora `WM_CLOSE`, que abre um diálogo ("salvar antes de sair?") ou que não
   tem janela visível é morto ao fim dos 3 s (ou na hora) e não salva o que salvaria ao sair.
3. **O Windows pode negar o primeiro plano** à janela do Big Box (trava de primeiro plano); `BringToFront` tenta duas vezes e, se
   negar, a janela fica no topo e o controle volta a funcionar ao receber o foco.
4. **Jogo protegido (anticheat)** pode recusar `TerminateProcess`: o som de recusa toca e a sessão segue (CON-013).
5. **Só o último jogo sem contar tempo** pode ser fechado pelo controle (decisão c).
6. **Cota e códigos do ScreenScraper não confirmados** até a conferência; os valores do contrato são conservadores.
7. **Sem internet ou sem cota** o Big Box só mostra artes.
8. **O fechamento leva até 3 s a mais para o Big Box voltar** quando o jogo não atende ao pedido de fechar (CON-030).

## Respostas do usuário (aprovação, 2026-11-15)

1. **Valor gravado do motivo:** `closed-by-hotkey`, com hífen (decisão a; CON-005..CON-008, CON-016..CON-018).
2. **Fechar educadamente antes de matar:** sim. `WM_CLOSE` às janelas de topo dos processos vivos do jogo (árvore do job ou PIDs
   adotados conferidos), espera de até 3 s pelo `domain.Clock`, depois `TerminateJobObject`/`TerminateProcess`. **O tempo conta
   até o instante do gatilho** (a espera não conta; justificativa na decisão b). URI sem processo adotado e nada para fechar
   continuam com o som de recusa. Estado intermediário de fechamento sem nova sessão até terminar (CON-005..CON-008, CON-010,
   CON-013, CON-030..CON-033; [máquina de estados](contracts/session-close-state-machine.md)).
3. **Jogo sem contar tempo dentro de um Job Object guardado pelo tracker:** sim (decisão c; CON-010, CON-011).
4. **Vídeo buscado sozinho depois de aplicar metadados e por pedido explícito na janela normal, nunca pelo Big Box:** sim
   (decisão h; VID-004..VID-007, VID-024, SSC-003, BBX-029).
5. **Só "match" baixa vídeo:** sim (decisão i; VID-011).
6. **Mapa de teclas do Big Box** (A Jogar, X Detalhes, Y Som, B Voltar/Sair; "Jogar sem contar tempo" só nos Detalhes): sim
   (decisão m; BBX-002, BBX-013..BBX-016, [bigbox-faixa §2](contracts/bigbox-faixa.md)).
7. **Número de discos:** não gravar. Sem `game.disc_count`, sem `GameDetail.discs`, sem mudança na importação; a migração 9 só
   acrescenta o tipo `video`; o Faixa não mostra discos (BBX-031 removido; VID-001, BBX-009, BBX-016 alterados).

## Correções da revisão do diff (2026-11-15)

Achados do reviewer registrados na spec antes do código (tarefas T46..T51):

| Achado | Requisito | Requisitos ativos afetados (texto inalterado) |
|---|---|---|
| Credenciais anexadas a uma URL de mídia de qualquer host | VID-027 (novo) | VID-012, VID-020, SSC-004 |
| Redirecionamento podia levar as credenciais a outro host | VID-028 (novo) | VID-015, VID-020 |
| 200 `text/*` com aviso de cota virava "formato não suportado" | VID-029 (novo) | VID-014, VID-016, VID-017, VID-018 |
| `\n`/`\r` em `id` ou `secret` corrompia o par gravado | SSC-006 (novo) | SSC-001, VID-017, VID-019 |
| Despacho por `provider`, roteamento por `kind` | VID-009 (alterado) | VID-003, VID-007, VID-021 |
| `AttachThreadInput` sem a thread fixa (`runtime.LockOSThread`) | nenhum novo: detalhe de implementação de CON-023 ([design §2.5](design.md)) | CON-022, CON-023 |

`prefers-reduced-motion` não é requisito desta fase (ver "Fora desta spec").

## Ajustes do teste com controle (2026-11-16)

Pedidos do usuário depois de usar o Faixa com controle, registrados antes do código (tarefas T52..T61). **Aprovados pelo usuário
em 2026-11-16**, com estas mudanças sobre a proposta:

- **Leitura da aba (decisão 2 da proposta, alterada):** a aba **não** é lida inteira. A primeira página tem `V + 3` jogos (a
  capacidade da tela do usuário é 12, então 15) e as seguintes são pedidas à frente do foco, para que a faixa nunca tenha buracos
  nem espere (BBX-007 volta como pré-busca, BBX-038 reescrito, BBX-039 com o total da aba, BBX-027 esclarecido).
- **Contador "posição / total" na faixa (decisão 5):** sim, BBX-045.
- **Y (som) nos detalhes (decisão 6):** não precisa funcionar; registrado em "Fora desta spec" (BBX-015 alterado).
- Itens 1, 3 e 4 da proposta: aprovados como estavam.

| Pedido | Requisitos novos | Alterados / removidos |
|---|---|---|
| Foco nos detalhes não chega aos botões; imagem da galeria não troca | BBX-032, BBX-033, BBX-034, BBX-035, BBX-036 | BBX-015, BBX-016 (alterados) |
| Texto ilegível sobre algumas artes | BBX-037 | BBX-025, BBX-026 (alterados) |
| Lista da aba aparece aos pedaços | BBX-038, BBX-039, BBX-045 | BBX-006, BBX-007, BBX-024, BBX-027 (alterados) |
| Capas e fundos cortados | BBX-040, BBX-041 | — (BBX-010 continua) |
| Galeria cortada, texto colado na borda, linhas espaçadas | BBX-042, BBX-043, BBX-044 | — |

## ADRs propostos

A sessão principal escreve (a spec só os referencia). Próximo número livre: 0035.

1. **0035 — Fechar o jogo pelo controle (Select + B):** o agente pede para fechar (`WM_CLOSE`, até 3 s) e depois mata a árvore
   do jogo (`TerminateJobObject`; processos adotados por PID com o caminho conferido), encerra a sessão com o tempo cortado no
   instante do gatilho e o motivo `closed-by-hotkey`, recusa com som quando não há
   o que fechar, e avisa a UI por `game.closedByHotkey` para o Big Box voltar ao primeiro plano. O jogo sem contar tempo passa a
   nascer num Job Object guardado pelo tracker, sem sessão (**substitui em parte o ADR 0019**, que dizia "sem Job Object";
   mantém o 0005: fechar o agente nunca mata o jogo).
2. **0036 — Big Box com temas selecionáveis:** o Big Box deixa de ser a biblioteca de três colunas em tela cheia (substitui a
   parte de Big Box do ADR 0028/UIX-044) e passa a ser um shell próprio com navegação por reducer puro, modelo de exibição comum e
   temas registrados (Faixa implementado; Mosaico e Roda preparados), com tokens por tema sob `data-bigbox-theme` (estende o
   ADR 0029) e entrada suspensa enquanto o jogo roda.
3. **0037 — Vídeo do ScreenScraper como mídia `video` na fila:** tipo novo com limite 1 (estende os ADRs 0030 e 0034, migração do
   `CHECK`), job com pseudo-URL resolvido no download numa raia própria de 1 trabalhador com pausa por cota e por credencial,
   identificação por nome + sistema com a pontuação da fase 12 (só `match`), só `video-normalized` até 20 MB, credenciais fora de
   URL gravada e de log, servido pelo mesmo `/media/`.
