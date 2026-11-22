# Fase 15 — Big Box v2: requisitos

- **Status:** **aprovada em 2026-11-22**, com as respostas registradas em "Respostas do usuário (aprovação)" no fim (IGDB como
  reserva do HowLongToBeat; as duas opções do HowLongToBeat ligadas por padrão; nome "Estante" e abas de coleção como propostos).
- **Design:** [design.md](design.md) · **Contratos:** [contracts/ipc-v0.16.0-delta.md](contracts/ipc-v0.16.0-delta.md) e
  [contracts/ipc-v0.16.0-delta.openrpc.json](contracts/ipc-v0.16.0-delta.openrpc.json) (só o acréscimo sobre a v0.15.0 de
  [phase-13-modo-console](../phase-13-modo-console/contracts/ipc.openrpc.json) e da fase 14; o JSON completo v0.16.0 é uma tarefa,
  como nas fases 11 a 13), [contracts/hltb-rules.md](contracts/hltb-rules.md) (descoberta do endpoint, pedido, leitura,
  correspondência, validade do cache, recuo após falha, formato das horas) e
  [contracts/bigbox-temas-v2.md](contracts/bigbox-temas-v2.md) (contrato de tema v2, grade 2D no reducer, janelas e páginas, layouts
  e tokens do Mosaico e da Estante) · **Tarefas:** `tasks.md` (o planner gera depois da aprovação).
- **Base:** [phase-13-modo-console](../phase-13-modo-console/requirements.md) (BBX-001..BBX-045, CON-001..CON-033,
  [bigbox-faixa](../phase-13-modo-console/contracts/bigbox-faixa.md)), [single-exe](../single-exe/requirements.md) (EXE-009..EXE-012,
  instância única), [phase-4-pause](../phase-4-pause/requirements.md) (combo do controle, PAD-*),
  [phase-12-metadados-unificados](../phase-12-metadados-unificados/requirements.md) (`SearchTerm`, `ScoreGroup`, `ClassifyMatch`,
  lote, `scrapers.Limiter`), [phase-10-nova-interface](../phase-10-nova-interface/requirements.md) (painel, UIX-028, UIX-029) e
  [phase-14-edicao-manual](../phase-14-edicao-manual/requirements.md) (janela "Editar jogo").
- **ADRs:** 0014 (executável único), 0018 (entrada global), 0023 e 0024 (UI e agente sempre elevados, sem início com o Windows),
  0029 (tema por tokens), 0031 (avisos de invalidação), 0033 (busca unificada), 0035 (Select + B), 0036 (Big Box com temas). ADRs
  propostos no fim.
- **Objetivo:** (1) **Select + A** segurado por 1 s abre o Big Box mesmo com a janela fechada ou sem foco (ex.: pelo
  Moonlight/Sunshine), e `arcadia.exe --bigbox` abre a UI já no Big Box; (2) **tempo para zerar do HowLongToBeat** (história
  principal, história + extras, 100%), com o **IGDB como reserva** quando o HowLongToBeat falha ou não acha o jogo, cache no banco e
  falha silenciosa; (3) tema **Mosaico** (estilo Nintendo Switch);
  (4) tema novo **Estante** (estilo "Big Picture" de launchers de PC, com grade 2D). **A UI nunca influencia o tempo de jogo.**

Formato: IDs `ÁREA-NNN`, estáveis e nunca reutilizados; critérios Dado/Quando/Então. Áreas novas (conferidas com `grep` em
`docs/`, sem colisão): **ATB** (atalho e parâmetro do Big Box), **TTB** (tempo para zerar), **MOS** (tema Mosaico) e **ETN**
(tema Estante; `EST` já é das estatísticas). As mudanças comuns aos temas continuam na área **BBX** (a partir de BBX-046). Todo ID
ativo precisa de um teste que o cite no nome (`t.Run("ATB-004/...")`, `it("MOS-004 ...")`), exceto os marcados
`· verificação: manual`. Nenhum teste usa rede (`httptest`, fakes) nem `time.Sleep` (relógio fake `domain.Clock` no Go,
`vi.useFakeTimers` no frontend). Instantes: `T` é o `Now()` de um `FakeClock`.

---

## Decisões do spec-writer (confirmadas na aprovação, 2026-11-22)

a. **Select + A com um jogo em execução é recusado** (som de recusa, nada abre). Justificativa: tirar o foco de um jogo em tela
   cheia exclusiva pode minimizá-lo ou travá-lo, e com a sessão aberta o Big Box só serviria para lançar outro jogo, que seria
   recusado (`-32002`); o caminho seguro é fechar o jogo com Select + B, que já devolve o Big Box quando ele estava ativo (CON-022).
   Vale para toda sessão aberta (Launching, Running, Paused, Grace e fechando); o jogo **sem contar tempo** não é visto pelo
   roteador (o tracker não o expõe) e fica em "Limitações" (ATB-005).
b. **O agente abre o Big Box executando o próprio exe com `--bigbox --data-dir <Root>`**, como "Abrir Arcadia" (EXE-009). Com a UI
   aberta, o processo novo cai na instância única do Wails e a primeira instância passa a **ler os argumentos** da segunda (antes
   ignorados, design de single-exe): só `--bigbox` tem efeito; `--data-dir` diferente continua sem trocar a biblioteca (ATB-009).
c. **O pedido de Big Box é guardado no desktop e consumido uma vez** (`TakeBigBoxRequest()`), para não se perder quando chega
   antes de o frontend montar e para nunca entrar duas vezes (ATB-012).
d. **Com uma janela modal aberta no modo normal, o pedido só traz a janela à frente** e não entra no Big Box: entrar desmontaria a
   modal (o Big Box substitui a árvore da janela normal) e perderia uma edição não salva (ATB-011).
e. **Sem dica nova no rodapé:** o atalho leva ao Big Box, então dentro dele não faz nada; fora dele a janela normal não tem
   rodapé de controle. O atalho aparece na dica (`title`) do botão "Big Box" e na documentação (ATB-013).
f. **Falha ao abrir o Big Box toca o som de recusa** além do `Warn`: quem está na TV não vê o log (ATB-006).
g. **Nome do serviço no contrato: `timeToBeat.*`** (neutro: serve às duas fontes). A fonte gravada é `hltb` ou `igdb` (resposta 1).
q. **Reserva do IGDB (resposta 1):** o IGDB (`game_time_to_beats`) só é consultado quando o HowLongToBeat **não acha** (`not-found`)
   ou **falha** (falha temporária ou recuo); "em dúvida" no HowLongToBeat não aciona a reserva (há candidatos para o usuário
   confirmar). O jogo do IGDB é escolhido pela busca da fase 12 (com plataforma e ano) e só um `match` vale. Mapeamento: apressado
   (`hastily`) → História principal, normal (`normally`) → História + extras, completo (`completely`) → 100%. Um achado do IGDB vale
   **30 dias** (menos que os 90 do HowLongToBeat), para a fonte principal ser tentada de novo antes. Sem chave do IGDB, a reserva é
   ignorada em silêncio (`Debug`, sem recuo, sem aviso). Um IGDB "não achou" depois de uma **falha** do HowLongToBeat não grava nada
   (a fonte principal ainda não respondeu); depois de um "não achou" do HowLongToBeat, grava `not-found` (TTB-033..TTB-040).
h. **Correspondência automática só com `match` das regras da fase 12** (`domain.ScoreGroup` sem plataforma e `domain.ClassifyMatch`):
   pontuação ≥ 0,85, similaridade do título ≥ 0,80, mesmas marcas (números e letras soltas) e vantagem ≥ 0,05 sobre o segundo.
   "Em dúvida" (≥ 0,60) fica gravado sem valores, com os candidatos, para o usuário confirmar; abaixo disso, "não encontrado".
   A plataforma não entra: os nomes de plataforma do HowLongToBeat não têm tabela de correspondência com as chaves de seed
   (TTB-004).
i. **Busca sob demanda sem bloquear a tela:** `timeToBeat.get` responde na hora com o cache e, se preciso, enfileira a busca; o
   fim chega por `timeToBeat.changed {gameId}` (ADR 0031). Para não gastar uma busca por capa olhada, a UI só pede a busca depois de
   o jogo ficar **1 s** em foco no painel do modo normal e no painel do Mosaico, e na hora ao abrir os Detalhes do Big Box ou a
   janela "Editar jogo" (TTB-008, TTB-015..TTB-018).
j. **Exceção à regra "o Big Box só lê" (BBX-029):** ler o tempo para zerar pode fazer o agente consultar o HowLongToBeat e gravar o
   cache. É a única exceção; download de imagem ou vídeo continua proibido no Big Box (BBX-059).
k. **Configuração no agente** (`app_setting`), na janela dos provedores: "Consultar o HowLongToBeat" e "Buscar depois de aplicar
   metadados", as duas **ligadas** por padrão (o usuário pediu o recurso). Desligar a primeira corta toda requisição; o cache
   continua aparecendo (TTB-023).
l. **Tema novo: `estante` ("Estante")**, nome neutro sem marca. "Galeria" foi descartado porque já nomeia a galeria de imagens dos
   Detalhes (BBX-016, BBX-032).
m. **Estante com abas de coleção:** "Todos", "Favoritos" e "Concluídos" (o que o Arcadia tem de `favorite` e `status`), seguidas
   das plataformas; "Modernos" e "Clássicos" saem (o Arcadia não classifica jogos assim). Ordem fixa "Jogado recentemente"
   (`lastPlayed`), mostrada como rótulo (BBX-054, BBX-055).
n. **Estante com fundo em `cover`** (arte em tela cheia escurecida em degradê para o fundo embaixo, como na referência), diferente
   do Faixa, que segue BBX-040 (`contain`). O Mosaico não tem arte de fundo (fundo claro liso) e mostra o vídeo **na capa em foco**
   (BBX-052, BBX-053).
o. **Grade 2D no reducer compartilhado:** o contrato ganha o eixo `grid` com colunas; ↑/↓ andam uma fileira; ←/→ andam um jogo
   (passando de fileira). A janela e a pré-busca da grade têm regra própria; as da faixa não mudam (BBX-048..BBX-050).
p. **Select segurado passa a bloquear também A e ↑/↓ do direcional na UI:** sem isso, Select + A lançaria o jogo em foco no Big
   Box e Select + ↑/↓ (volume, commit 4d7e8c7) andaria na grade da Estante (BBX-057). Lacuna registrada: o volume pelo controle
   não tinha requisito sobre a UI.

---

## Requisitos de fases anteriores afetados

Cada um recebeu, na spec de origem, a anotação `· alterado em 2026-11-22 (docs/specs/phase-15-big-box-v2, ID)` (ou
`· substituído em ...`), feita na aprovação.

| Requisito | Fase | Situação | Por |
|---|---|---|---|
| CON-002 — Os dois combos são independentes | 13 | alterado: são três combos (pausa, fechar, Big Box), todos independentes | ATB-002 |
| PAD-001 — Combo do controle | 4 | inalterado para Select + Y; o poller ganha o terceiro detector | ATB-001 |
| EXE-009 — "Abrir Arcadia" lança o próprio executável | single-exe | inalterado; o mesmo processo com `--bigbox` é ATB-004 | ATB-004 |
| EXE-011 — Instância única da UI | single-exe | alterado: a segunda instância com `--bigbox` também faz a janela entrar no Big Box; sem `--bigbox`, igual | ATB-009 |
| (design de single-exe) "Os `Args` da segunda instância são ignorados" | single-exe | alterado: `--bigbox` é lido; o resto continua ignorado | ATB-009 |
| BBX-001 — Entrar no Big Box | 13 | alterado: também por `--bigbox` e por Select + A (pedido externo) | ATB-008..ATB-012 |
| BBX-003 — O Big Box só navega | 13 | alterado: além de `LaunchGame`, `LaunchGameUntracked` e `BringToFront`, o Big Box chama `TimeToBeatGet` | BBX-059 |
| BBX-004 — Abas de plataforma | 13 | alterado: o tema pode declarar abas de coleção antes das plataformas | BBX-054 |
| BBX-006 — Faixa de capas | 13 | inalterado para o Faixa; a âncora passa a ser do tema | BBX-051 |
| BBX-007, BBX-038 — Pré-busca e páginas | 13 | alterados: a grade tem regra própria de página e de pré-busca; a faixa não muda | BBX-050 |
| BBX-010, BBX-040 — Arte de fundo | 13 | alterados: valem para fundo `contain` (Faixa); `cover` (Estante) e liso (Mosaico) são novos | BBX-052 |
| BBX-011 — Vídeo depois de 2 s parado | 13 | alterado: o vídeo vai para o fundo ou para a capa em foco, conforme o tema | BBX-053 |
| BBX-015 — Rodapé com dicas | 13 | alterado: o rótulo de LB/RB é "Aba" quando o tema usa abas de coleção | BBX-054 |
| BBX-016 — Tela de detalhes | 13 | alterado: mostra o tempo para zerar; o tema pode ter tela de detalhes própria com os mesmos estados | TTB-016, BBX-056 |
| BBX-019 — Select segurado bloqueia B e Y na UI | 13 | alterado: bloqueia também A e ↑/↓ do direcional | BBX-057 |
| BBX-023 — Temas selecionáveis | 13 | alterado: `faixa`, `mosaico` e `estante` disponíveis; `roda` indisponível | BBX-046 |
| BBX-024 — Contrato de tema | 13 | alterado: contrato v2 (eixo `grid`, âncora, fundo, lugar do vídeo, abas, ordem, detalhes próprios) | BBX-047 |
| BBX-025, BBX-026 — Tokens e contraste | 13 | alterados: cada tema tem o bloco completo de tokens e o seu teste de contraste | BBX-058 |
| BBX-029 — Só leituras ao navegar | 13 | alterado: a leitura do tempo para zerar pode buscar no HowLongToBeat (única exceção) | BBX-059 |
| CON-024 — A UI nunca influencia o tempo de jogo | 13 | alterado: o teste de invariantes ganha as interações desta fase | ATB-015, TTB-030 |
| MET-030, MET-065 — Aplicar metadados | 12 | alterados: depois de aplicar, o agente enfileira a busca do tempo para zerar quando configurado | TTB-025 |
| MET-010 — Intervalo mínimo por provedor | 12 | inalterado; as requisições do IGDB da reserva passam pelo mesmo limitador do IGDB | TTB-038 |
| UIX-028, UIX-029 — Painel do jogo | 10 | inalterados; o painel ganha o bloco "Tempo para zerar" | TTB-015 |
| EDM-004 — Imagens na janela de edição | 14 | inalterado; a janela ganha a seção do tempo para zerar | TTB-018, TTB-019 |
| SSC-002 — Janela dos provedores | 13 | inalterado; a janela ganha a seção do HowLongToBeat | TTB-024 |
| ~~(design 4.6 da fase 13) Mosaico "sem vídeo de fundo, vídeo só em Detalhes"~~ | 13 | substituído: o vídeo toca na capa em foco | MOS-007 |

Inalterados e ainda válidos: BBX-002, BBX-005, BBX-008, BBX-009, BBX-012..BBX-014, BBX-017, BBX-018, BBX-020..BBX-022,
BBX-027, BBX-028, BBX-030, BBX-032..BBX-037, BBX-039 (com a âncora do tema), BBX-041..BBX-045; CON-001, CON-003..CON-033 (Select +
B não muda); SES-*, TRK-*, PAU-*, PAD-002..PAD-009; VID-*, SSC-001, SSC-003..SSC-006; MET-001..MET-029, MET-031..MET-064,
MET-066..MET-083; EDM-001..EDM-005.

**Lacunas registradas:** (1) o acorde Select + ↑/↓ do volume (commit 4d7e8c7) não tinha requisito sobre a UI: no Big Box, o
direcional apertado com Select também move o foco; BBX-057 fecha. (2) O `CLAUDE.md` ainda diz que o agente "inicia com o Windows",
mas o ADR 0024 removeu isso: Select + A só funciona com o agente na bandeja (aberto pelo usuário desde o login); registrado em
"Limitações" (a correção do `CLAUDE.md` é da sessão principal).

---

## ATB — Atalho Select + A e parâmetro `--bigbox`

### ATB-001 — Combo Select + A segurado dispara o gatilho do Big Box
Dado o controle 0 sem botões, quando `Back` (`0x0020`) e `A` (`0x1000`) passam a estar pressionados juntos em `T` e continuam, então
exatamente um `Trigger{Action: bigbox, Source: gamepad, At}` é entregue na primeira leitura com `agora − T ≥ 1 s` (leituras a cada
16 ms: `T+992ms` não dispara, `T+1008ms` dispara). Valem as regras de PAD-002..PAD-005: soltar antes cancela, um gatilho por
pressão, controles independentes, desconexão zera a contagem. O atalho de teclado (`Ctrl+Alt+P`) continua só de pausa (CON-003).
· verificação: teste do poller com `PadAPI` falsa e relógio fake.

### ATB-002 — Os três combos são independentes
Dado o poller, então `Back + Y` só entrega `pause`, `Back + B` só entrega `close` e `Back + A` só entrega `bigbox`. Dado
`Back + A + B` segurados juntos por 1 s, então são entregues um `close` e um `bigbox`, cada um uma vez por pressão. Soltar `A` não
zera a contagem de `Back + B` nem de `Back + Y`, e vice-versa. Os acordes de volume (`Back + ↑/↓`) continuam como estão.
· verificação: teste do poller (sequências de botões; contagem por ação).

### ATB-003 — Roteamento do gatilho do Big Box
Dado `Trigger{Action: bigbox, At: t}`, então o roteador de gatilhos trata-o com uma janela de antirrepique própria de 300 ms (dois
gatilhos do Big Box com menos de 300 ms entre os `At` viram um só, o segundo descartado sem som); as janelas de pausa e de fechar
não mudam e não contam este gatilho. O roteador só **lê** a sessão atual (`Tracker.Current()`); nunca chama `TogglePause`,
`CloseByHotkey` nem lança jogo.
· verificação: teste do roteador com tracker falso (nenhuma chamada além de `Current`; janelas por ação).

### ATB-004 — Sem jogo em execução, o agente abre o Big Box
Dado o agente rodando a partir do executável `E` com `Root` `R` e `Tracker.Current()` sem sessão, quando chega o gatilho do Big Box,
então o agente cria um processo com executável `E`, argumentos `--bigbox --data-dir R` (sem `--agent`) e flags
`DETACHED_PROCESS | CREATE_NEW_PROCESS_GROUP`, registra `Info` "Big Box pedido pelo controle" e não toca som. O processo novo herda a
elevação do agente (ADR 0023, 0024).
· verificação: teste do roteador com `spawn` espião (exe, argumentos, flags; sem som).

### ATB-005 — Com um jogo em execução, o pedido é recusado
Dada uma sessão aberta em qualquer estado (Launching, Running, Paused, Grace ou em fechamento, CON-031), quando chega o gatilho do
Big Box, então nenhum processo é criado, a janela não muda, é registrado `Debug` "Big Box ignorado: jogo em execução" e o roteador
envia `feedback.EventRefused` (som de recusa de CON-015, sem aviso na tela). Justificativa: decisão a. Dado um jogo **sem contar
tempo** vivo e nenhuma sessão, então o pedido segue ATB-004 (Limitações 2).
· verificação: teste do roteador com tracker falso (um caso por estado; `spawn` nunca chamado; feedback espião).

### ATB-006 — Falha ao criar o processo
Dado que criar o processo de ATB-004 falha, então o agente registra `Warn` "falha ao abrir o Big Box" com `path` e `error`, envia
`feedback.EventRefused`, continua rodando e o próximo gatilho tenta de novo.
· verificação: teste do roteador com `spawn` que falha (log capturado, feedback espião, segunda tentativa chama `spawn`).

### ATB-007 — Gatilho durante o encerramento
Dado o agente depois de `BeginShutdown` (ou o roteador encerrando), quando chega o gatilho do Big Box, então ele é descartado com
`Debug`, sem processo criado e sem som.
· verificação: teste do roteador.

### ATB-008 — `arcadia.exe --bigbox` abre a UI no Big Box
Dado nenhuma UI aberta, quando inicio `arcadia.exe --bigbox` (com ou sem `--data-dir`), então a UI sobe como em EXE-008 e, assim que
a janela normal monta, entra no Big Box (tela cheia, tema escolhido, BBX-001) e chama `BringToFront()` uma vez. `--bigbox` segue a
sintaxe booleana do pacote `flag` (`--bigbox`, `-bigbox`, `--bigbox=true`; `--bigbox=false` não pede nada). Com `--agent` na linha,
o modo é o do agente (EXE-001) e `--bigbox` é ignorado. Sem `--bigbox`, a UI abre na janela normal como hoje.
· verificação: teste de `cmd/arcadia` (leitura dos argumentos → pedido guardado) e de componente do `App` com `TakeBigBoxRequest`
falso (entra uma vez; `BringToFront` uma vez).

### ATB-009 — Segunda instância com `--bigbox`
Dada a UI aberta (janela normal, visível, minimizada ou atrás de outras), quando um segundo `arcadia.exe` é iniciado com
`--bigbox`, então o segundo processo sai como em EXE-011 e a primeira instância restaura a janela, força o primeiro plano (o
`bringToFront` de CON-023, já que quem pediu foi o agente, sem foco), guarda o pedido e emite o evento Wails `ui:bigBoxRequested`;
o frontend consome o pedido (ATB-012) e entra no Big Box. Os outros argumentos da segunda instância continuam ignorados (um
`--data-dir` diferente não troca a biblioteca). Dada a segunda instância **sem** `--bigbox`, então vale EXE-011 sem mudança (a
janela vem à frente e não entra no Big Box).
· verificação: teste de `cmd/arcadia` com `SecondInstanceData` falso e plataforma espiã (ordem: restaurar, primeiro plano, pedido,
evento) e de componente do `App` com o evento simulado.

### ATB-010 — Já no Big Box, nada muda
Dado o Big Box ativo (Navegar ou Detalhes, suspenso ou não), quando chega um pedido de Big Box (`--bigbox` de segunda instância),
então a janela só é trazida à frente: tela, aba, foco, tela de Detalhes e suspensão (BBX-018) não mudam, e o pedido é consumido sem
efeito.
· verificação: teste de componente do `App` com o Big Box ativo (estado do reducer igual antes e depois).

### ATB-011 — Pedido com uma janela modal aberta
Dado o modo normal com uma janela modal aberta (edição, metadados, importação, emuladores, plataformas, provedores, confirmação do
lote ou diálogo de remoção), quando chega um pedido de Big Box, então a janela vem à frente, o pedido é consumido e o Big Box **não**
abre (a modal e o que nela foi digitado continuam). Com a página de estatísticas aberta e nenhuma modal, o Big Box abre e, ao sair,
a página de estatísticas reaparece.
· verificação: teste de componente do `App` (modal aberta: sem Big Box, campo digitado preservado; estatísticas: entra e volta).

### ATB-012 — O pedido é atendido uma vez
Dado o pedido guardado no desktop, então `TakeBigBoxRequest()` devolve `true` uma única vez e depois `false` até um pedido novo. O
frontend chama `TakeBigBoxRequest()` ao montar e a cada `ui:bigBoxRequested`. Dado um pedido que chega antes de o frontend montar,
então ele é atendido na montagem; dado um evento e a montagem atendendo o mesmo pedido, então o Big Box entra uma vez só.
· verificação: teste de `cmd/arcadia` (consumo único, concorrência com `-race`) e de componente (montagem + evento → uma entrada).

### ATB-013 — Onde o atalho aparece
Dado o cabeçalho da janela normal, então o botão "Big Box" tem a dica (`title`) "Big Box (F11 ou Select + A no controle)". Dado o
rodapé do Big Box em qualquer tela e tema, então nenhuma dica de Select + A aparece (decisão e).
· verificação: teste de componente (atributo `title`; `hintsFor` sem "Select+A").

### ATB-014 — Contrato do desktop
Dado o desktop, então existem o binding `TakeBigBoxRequest(): bool` e o evento `ui:bigBoxRequested` (sem dados). Nenhum dos dois chama
o agente.
· verificação: teste de `cmd/arcadia`.

### ATB-015 — Select + A não toca no tempo de jogo
Dado qualquer caminho desta área (combo, roteador, `--bigbox`, segunda instância, entrada no Big Box), então nenhuma sessão é criada,
pausada, retomada ou encerrada e nenhum jogo é lançado ou fechado; o teste de invariantes da UI (`App.invariants.test.tsx`) ganha a
entrada por pedido externo.
· verificação: teste do roteador (tracker falso só com `Current`) e `App.invariants.test.tsx`.

### ATB-016 — Ponta a ponta do atalho
Dado o Arcadia real com um controle XInput e o agente na bandeja, quando seguro Select + A por 1 s: com a janela fechada; com a
janela aberta atrás de outro programa; com a janela minimizada; já no Big Box; com um jogo rodando; e pelo Moonlight conectado ao
Sunshine (controle virtual), então: sem jogo, o Big Box abre em tela cheia à frente (o processo novo herda a elevação, sem aviso do
UAC); já no Big Box, nada muda; com jogo, ouço o som de recusa e o jogo continua em primeiro plano; um toque curto não faz nada.
· verificação: manual.

---

## TTB — Tempo para zerar (HowLongToBeat)

Regras do serviço em [contracts/hltb-rules.md](contracts/hltb-rules.md); contrato IPC em
[contracts/ipc-v0.16.0-delta.md](contracts/ipc-v0.16.0-delta.md). Os três valores são: **História principal** (`main`),
**História + extras** (`mainExtra`) e **100%** (`completionist`).

### TTB-001 — Migração 10: tabela do cache
· alterado em 2026-11-22 (aprovação, resposta 1): a coluna da fonte aceita `hltb` e `igdb`.
Dado um banco novo ou na versão 9 com dados, quando o agente abre o store com migração, então existe a tabela `time_to_beat`
(uma linha por jogo, `game_id` com `ON DELETE CASCADE`) com as colunas de [design §3.1](design.md), `store.SchemaVersion = 10` e a
versão do goose é 10; nenhuma outra tabela muda. O `Down` apaga a tabela.
· verificação: teste de migração do store (banco novo; v9 com jogos; `Down`).

### TTB-002 — O que é gravado
· alterado em 2026-11-22 (aprovação, resposta 1): a fonte pode ser `igdb` (só com `found`, TTB-033).
Dada uma busca concluída do jogo `G`, então a linha de `G` guarda: a fonte (`hltb` ou `igdb`), o estado (`found`, `doubt` ou
`not-found`; com `igdb`, só `found`), o título de `G` no momento da busca, o termo usado, o instante da busca e o de validade (ms UTC,
ADR 0009), se foi confirmada pelo usuário e, conforme o estado: em `found`, o id e o título do jogo na fonte e os três valores em
segundos (cada um ou nulo);
em `doubt`, os candidatos (até 10, [rules §5](contracts/hltb-rules.md)); em `not-found`, nada além disso. Falha temporária não grava
nada (TTB-011).
· verificação: teste do store e do serviço com cliente falso.

### TTB-003 — Termo de busca
Dado o jogo `G`, então o termo enviado é `domain.SearchTerm(título de G)` (fase 12: sem grupos entre parênteses e colchetes, sem
sinais de marca, primeiro " - " vira ": "). Dado um termo vazio, então nenhuma requisição é feita e `G` fica `not-found`. Dada uma
busca com termo contendo ": " que não devolve nenhum resultado, então uma segunda busca é feita com o texto antes do primeiro ": "
e a classificação continua contra o título inteiro.
· verificação: teste do serviço com cliente falso (termos recebidos).

### TTB-004 — Correspondência automática só com confiança alta
Dados os resultados da busca, então cada um vira um grupo de membros (o nome e cada apelido do HowLongToBeat) pontuado por
`domain.ScoreGroup` com o título e o ano de `G` e **sem** plataforma, ordenado pela pontuação, e o par (primeiro, segundo) é
classificado por `domain.ClassifyMatch` (limiares da fase 12, decisão h): `match` grava `found` com os valores do primeiro; `doubt`
grava `doubt` sem valores; `none` (ou nenhum resultado) grava `not-found`. Os vetores de [rules §5](contracts/hltb-rules.md) valem
(ex.: "Resident Evil 2" de 1998 contra os jogos de 1998 e de 2019 com o mesmo nome dá `doubt`; "Hollow Knight" dá `found`;
"Mega Man X" contra "Mega Man X2" não é `match`).
· verificação: teste de unidade com os vetores.

### TTB-005 — Valores ausentes
Dado um resultado escolhido, então cada valor vem do campo do HowLongToBeat em segundos ([rules §4](contracts/hltb-rules.md)); valor
0, ausente, negativo ou não inteiro é gravado como nulo, sem afetar os outros dois.
· verificação: teste de leitura com respostas gravadas.

### TTB-006 — Validade do cache
· alterado em 2026-11-22 (aprovação, resposta 1): `found` do IGDB vale 30 dias.
Dada a linha de `G` buscada em `t`, então ela vale até `t + 90 dias` em `found` do HowLongToBeat, `t + 30 dias` em `found` do IGDB,
em `doubt` e em `not-found`. Vencida uma linha do IGDB, a nova busca começa de novo pelo HowLongToBeat (TTB-033). Dado
que o título atual de `G` difere do título gravado e a linha não foi confirmada pelo usuário, então ela está vencida na hora. Uma
linha confirmada (TTB-019) nunca troca de jogo sozinha: vencida, só os valores são relidos pelo id confirmado
([rules §6](contracts/hltb-rules.md)); se o id não volta na busca, os valores antigos ficam e a validade é renovada.
· verificação: teste do serviço com relógio fake (cada estado; título trocado; confirmada).

### TTB-007 — Ler sem buscar
Dado `timeToBeat.get {gameId: G, fetch: false}`, então a resposta vem só do banco e da memória do serviço, sem requisição e sem
enfileirar nada: o estado gravado (`none` se não há linha), os valores, `fetchedAt`, `confirmed`, `pending` (há busca de `G` na fila
ou em curso) e `unavailable` (o serviço está em recuo, TTB-011). Linha vencida é devolvida como está, com `stale: true`.
· verificação: teste do agente com store real e cliente falso que conta chamadas.

### TTB-008 — Ler e buscar quando preciso
Dado `timeToBeat.get {gameId: G, fetch: true}` com o serviço ligado (TTB-023), quando não há linha de `G` ou ela está vencida, e o
serviço não está em recuo, então a busca de `G` é enfileirada com prioridade interativa e a resposta (imediata) traz `pending: true`
e o que houver no cache; com a linha válida, a resposta é a de TTB-007 e nada é enfileirado. Quando a busca termina e grava, então
todo cliente conectado recebe `timeToBeat.changed {gameId: G}` (sem `id`, ADR 0031). Em recuo, nada é enfileirado e a resposta traz
`unavailable: true`.
· verificação: teste do agente com dois clientes IPC, cliente HLTB falso e relógio fake.

### TTB-009 — Fila de buscas
Dado o serviço, então há no máximo **uma** requisição ao HowLongToBeat por vez (busca da fila, `refresh` e `search` somados); um jogo
aparece no máximo uma vez na fila (pedir de novo não duplica; um pedido interativo promove o jogo que estava na fila do lote); os
pedidos interativos são atendidos antes dos do lote, e cada fila segue a ordem de chegada. A fila do lote guarda até 1.000 jogos; o
excedente é descartado com `Debug`. Uma busca de um jogo removido no meio do caminho não grava nem notifica.
· verificação: teste do serviço com cliente falso controlado por canais (ordem, deduplicação, promoção, limite, jogo removido).

### TTB-010 — Intervalo e prazo das requisições
Dado qualquer requisição ao HowLongToBeat (descoberta, busca), então ela passa por um `scrapers.Limiter` próprio com **2.000 ms**
entre requisições e tem prazo de **10 s**; com o `ctx` do agente cancelado, ela é abandonada sem gravar.
· verificação: teste do limitador e do cliente com relógio fake e `httptest` lento.

### TTB-011 — Falha temporária e recuo
· alterado em 2026-11-22 (aprovação, resposta 1): o recuo é por fonte; com o HowLongToBeat em recuo, a fila segue pelo IGDB.
Dada uma falha do HowLongToBeat que não é resposta válida (rede, prazo, 429, 5xx, 403, corpo que não é o JSON esperado, endpoint não
descoberto, [rules §7](contracts/hltb-rules.md)), então o HowLongToBeat entra em **recuo** de 1 min na primeira falha seguida, 5 min
na segunda, 30 min na terceira e 60 min da quarta em diante, e durante o recuo nenhuma requisição é feita a ele; a busca do jogo
segue para a reserva (TTB-034) e, sem resultado gravado, sai da fila sem gravar nada. Com as duas fontes indisponíveis (HowLongToBeat
em recuo e IGDB em recuo ou sem chave), a fila espera o fim do primeiro recuo e `get` responde `unavailable: true`. É registrado
`Warn` "HowLongToBeat indisponível" com a causa (sem corpo da resposta) **uma vez** por sequência de falhas; uma resposta válida zera
a sequência.
· verificação: teste do serviço com cliente falso e relógio fake (escada de recuo; um `Warn`; sucesso zera).

### TTB-012 — Descoberta do endpoint de busca
Dado que o caminho da busca do HowLongToBeat muda com frequência, então o cliente descobre o caminho atual pela página inicial e pelo
script do site, guarda-o em memória e o reusa; dado um 404 na busca, então descobre de novo uma única vez antes de contar a falha. As
regras e os pontos a conferir estão em [rules §2, §3, §9](contracts/hltb-rules.md); divergência que mude regra visível volta à spec.
· verificação: teste do cliente com `httptest` servindo página e script de exemplo (descoberta, reuso, 404 → nova descoberta,
script sem o padrão → falha temporária).

### TTB-013 — A UI nunca quebra
Dado qualquer falha ou ausência de dado (serviço desligado, recuo, erro do IPC, agente desconectado, `not-found`, `doubt`, valor
nulo), então os três rótulos continuam na tela com "—" no valor, sem `role="alert"`, sem modal e sem mensagem de erro; nada da tela
em volta muda. Só a janela "Editar jogo" mostra o estado em texto (TTB-018), e só o painel do modo normal esconde o bloco quando o
serviço está desligado e não há cache (TTB-015).
· verificação: teste de componente do painel, dos Detalhes do Big Box e do Mosaico com bindings que falham.

### TTB-014 — Formato das horas
Dado um valor em segundos `s`, então o texto é: "—" para nulo ou `s ≤ 0`; senão `m = round(s / 300) × 5` minutos e, com `m < 60`,
"`m` min" (no mínimo "5 min"); com `m ≥ 60`, `n = round(s / 1800)` meias horas e o texto "`⌊n/2⌋` h", com "½" logo após o número
quando `n` é ímpar. Vetores: 1800 → "30 min"; 3599 → "1 h"; 5400 → "1½ h"; 97200 → "27 h"; 99000 → "27½ h"; 99900 → "28 h";
100 → "5 min". Rótulos: "História principal", "História + extras", "100%".
· verificação: teste de unidade de `formatTimeToBeat` (os vetores).

### TTB-015 — Painel do modo normal
· alterado em 2026-11-22 (aprovação, resposta 1): a linha da fonte segue TTB-039.
Dado o jogo `G` em foco no painel da janela normal, então há o bloco "Tempo para zerar" com os três rótulos e valores (TTB-014) e a
linha da fonte de TTB-039 quando há valor. Ao focar `G`, o painel lê o cache (`fetch: false`) na hora e, se `G` continuar em foco
por 1.000 ms, pede `fetch: true` uma vez; trocar de jogo antes disso cancela o pedido. Em `doubt`, o bloco mostra "Correspondência
incerta" e o botão "Confirmar…", que abre "Editar jogo" na seção do tempo para zerar. Com o serviço desligado e sem cache, o bloco
não aparece.
· verificação: teste de componente com temporizadores falsos e bindings espiões.

### TTB-016 — Detalhes do Big Box
Dada a tela Detalhes do Big Box (comum, BBX-016, e a da Estante, ETN-012) de `G`, então ela mostra "Tempo para zerar" com os três
rótulos e valores e a linha da fonte (TTB-039), e ao abrir pede `fetch: true` uma vez (sem espera). Os rótulos ficam no painel
(BBX-037). Nenhum botão de
atualizar ou confirmar aparece no Big Box.
· verificação: teste de componente.

### TTB-017 — Atualização ao vivo
Dado o painel, os Detalhes do Big Box, o painel do Mosaico ou "Editar jogo" mostrando `G`, quando chega `timeToBeat:changed` de
`G`, então o tempo para zerar de `G` é lido de novo uma vez (`fetch: false`); um aviso de outro jogo não relê `G`.
· verificação: teste de componente com o evento simulado.

### TTB-018 — Seção na janela "Editar jogo"
Dado "Editar jogo" de `G`, então há a seção "Tempo para zerar (HowLongToBeat)" com os três valores e uma linha de estado:
"Encontrado: <título no HowLongToBeat> · buscado em DD/MM/AAAA" (com "· confirmado" quando for o caso), "Encontrado no IGDB
(reserva): <título no IGDB> · buscado em DD/MM/AAAA", "Correspondência incerta",
"Não encontrado", "Ainda não buscado" ou "HowLongToBeat indisponível no momento". O botão "Atualizar" chama `TimeToBeatRefresh(G)`,
fica desativado com "Buscando…" até a resposta e mostra o resultado; falha mostra "Não foi possível consultar o HowLongToBeat" sem
mudar os valores. As mudanças valem na hora, sem esperar "Salvar" (como EDM-004). Com o serviço desligado, a seção mostra só o
cache e "Consulta ao HowLongToBeat desligada", sem botões.
· verificação: teste de componente com bindings falsos.

### TTB-019 — Escolher a correspondência à mão
Dado "Editar jogo" de `G`, quando o usuário toca "Escolher outro…", então aparece um campo com o termo de TTB-003 (editável) e
"Buscar", que chama `TimeToBeatSearch(G, termo)` e lista até 10 candidatos (título, ano, plataformas e os três valores); escolher um
chama `TimeToBeatChoose(G, id)` e grava `found` **confirmado** com os valores dele; "Nenhum destes" chama
`TimeToBeatChoose(G, "")` e grava `not-found` confirmado. Em `doubt`, a lista abre já com os candidatos gravados, sem nova busca.
· verificação: teste de componente com bindings falsos.

### TTB-020 — `timeToBeat.refresh`
· alterado em 2026-11-22 (aprovação, resposta 1): segue a tabela de fontes de TTB-033/TTB-034.
Dado `timeToBeat.refresh {gameId: G}`, então o agente faz a busca de `G` na hora (ignorando validade e o recuo das duas fontes, mas
passando pelos limitadores e pela vez única de TTB-009), com prazo total de 25 s, seguindo a ordem HowLongToBeat → IGDB de
TTB-033/TTB-034; grava, envia `timeToBeat.changed` e responde o mesmo de `get`. Numa linha confirmada, relê os valores do id
confirmado (TTB-006), sem reserva. Nada gravado por falha (TTB-034): `-32011` com `data.provider = "hltb"`, e cada falha conta para
o recuo da sua fonte. Jogo inexistente: `-32001`. Serviço desligado: `-32011` com `data.reason = "disabled"`, sem requisição.
· verificação: teste do agente (um caso por regra).

### TTB-021 — `timeToBeat.search`
Dado `timeToBeat.search {gameId: G, term?}`, então o agente busca com `term` (sem espaços nas pontas, 1 a 200 caracteres; ausente,
o termo de TTB-003) e responde `{candidates}` com até 10 itens pontuados contra o título de `G`, em ordem decrescente; a linha de `G`
não muda e os candidatos ficam guardados em memória para `choose`. Termo vazio depois do corte ou maior que 200: `-32602`. Falha,
jogo inexistente e serviço desligado: como TTB-020.
· verificação: teste do agente.

### TTB-022 — `timeToBeat.choose`
Dado `timeToBeat.choose {gameId: G, externalId}`, quando `externalId` está entre os candidatos guardados de `G` (da última
`search` ou da linha em `doubt`), então a linha de `G` passa a `found` confirmada com os valores desse candidato, sem requisição, e
`timeToBeat.changed` é enviado. Dado `externalId = ""`, então a linha passa a `not-found` confirmada. Dado um id que não está entre
os candidatos (inclusive depois de o agente reiniciar sem `doubt` gravado), então `-32602` e nada muda. Jogo inexistente: `-32001`.
· verificação: teste do agente.

### TTB-023 — Configuração
Dado `timeToBeat.getSettings {}`, então a resposta é `{enabled, afterMetadata}`, lidos de `app_setting` (`timeToBeat.enabled`,
`timeToBeat.afterMetadata`, valores `"1"`/`"0"`; ausente vale ligado). `timeToBeat.setSettings {enabled?, afterMetadata?}` grava os
campos presentes e responde o estado final; nenhum campo: `-32602`. Com `enabled = false`, nenhuma requisição é feita por nenhum
caminho (nem ao IGDB da reserva), a fila é esvaziada e `get` continua devolvendo o cache (`pending` sempre falso).
· alterado em 2026-11-22 (aprovação, resposta 1): o desligamento vale também para a reserva.
· verificação: teste do agente com store real.

### TTB-024 — HowLongToBeat na janela dos provedores
Dada a janela "Chaves dos provedores", então há a seção "HowLongToBeat (tempo para zerar)" com as caixas "Consultar o
HowLongToBeat" e "Buscar depois de aplicar metadados (também no lote após importar)" (a segunda desativada com a primeira
desligada), que gravam na hora por `TimeToBeatSetSettings`, e os textos "Serviço não oficial: pode parar de funcionar sem aviso" e
"Se o HowLongToBeat falhar ou não achar o jogo, o IGDB é usado como reserva (precisa da chave do IGDB acima)". O HowLongToBeat não
tem chave nem credencial.
· alterado em 2026-11-22 (aprovação, resposta 1): texto da reserva.
· verificação: teste de componente.

### TTB-025 — Busca depois de aplicar metadados
Dado `metadata.apply` ou a aplicação do lote (MET-065) do jogo `G` com sucesso, quando `enabled` e `afterMetadata` estão ligados e a
linha de `G` não é confirmada, então a busca de `G` é enfileirada com prioridade de lote, ignorando a validade (o título e o ano podem
ter mudado). O resultado de `metadata.apply` e o do lote não mudam e nenhuma notificação nova sai na aplicação; faltando uma
condição, nada é enfileirado (`Debug`).
· verificação: teste do agente (aplicação única e em lote; cada condição).

### TTB-026 — Jogo removido
Dada a remoção de `G`, então a linha de `G` some com ele (`ON DELETE CASCADE`), a busca de `G` sai da fila e uma busca em curso de `G`
termina sem gravar nem notificar.
· verificação: teste do agente com store real e cliente falso bloqueado.

### TTB-027 — Só o termo sai da máquina
Dado qualquer requisição ao HowLongToBeat, então ela leva só o termo de busca e os campos fixos de [rules §3](contracts/hltb-rules.md)
(cabeçalhos genéricos de navegador, sem nome de usuário, caminho de arquivo, id do jogo no Arcadia nem cookie guardado); o log nunca
grava o corpo das respostas. Ao IGDB da reserva vão só o que a busca de metadados da fase 12 já envia (termo, plataforma, ano) e o id
do jogo no IGDB, com as credenciais do Credential Manager (MID-001).
· verificação: teste do cliente com `httptest` (cabeçalhos e corpo recebidos) e revisão.

### TTB-028 — Encerramento
Dado o agente encerrando, então o trabalhador do serviço para (`ctx` cancelado e espera pelo `WaitGroup` no shutdown), a fila é
descartada, a requisição em curso é abandonada sem gravar e nenhuma notificação sai depois do `BeginShutdown`.
· verificação: teste do serviço (sem goroutine viva depois de `Close`, verificador de vazamento dos testes do agente).

### TTB-029 — Contrato IPC v0.16.0
Dado o contrato `docs/specs/phase-15-big-box-v2/contracts/ipc.openrpc.json` v0.16.0 (gerado pela tarefa a partir da v0.15.0, dos
acréscimos da fase 14 e do [delta](contracts/ipc-v0.16.0-delta.openrpc.json)), então ele é a fonte da verdade, o teste de contrato
de `internal/ipc` aponta para ele e um teste confere que, em relação à v0.15.0, só há acréscimos: os métodos `timeToBeat.*`, a
notificação `timeToBeat.changed`, os esquemas novos e o bloco `x-timetobeat`.
· verificação: teste de contrato do IPC.

### TTB-030 — Bindings e evento do desktop
Dado o desktop, então existem os bindings `TimeToBeatGet(gameId, fetch)`, `TimeToBeatRefresh(gameId)`, `TimeToBeatSearch(gameId,
term)`, `TimeToBeatChoose(gameId, externalId)`, `TimeToBeatSettings()` e `TimeToBeatSetSettings(enabled, afterMetadata)` com as
assinaturas de [delta §5](contracts/ipc-v0.16.0-delta.md), e o supervisor repassa `timeToBeat.changed` como o evento Wails
`timeToBeat:changed {gameId}`; parâmetros ausentes, JSON inválido ou `gameId` não inteiro positivo registram `Warn`
("timeToBeat.changed com parâmetros inválidos") e nada é emitido. Erros ao usuário seguem UI-008. Nenhum deles mexe em sessão; o
teste de invariantes ganha as interações desta área.
· verificação: teste de `cmd/arcadia` com fake do agente; `App.invariants.test.tsx`.

### TTB-031 — Sem rede nos testes
Dado a suíte da fase, então nenhum teste faz requisição externa: o serviço depende de interfaces de cliente para as duas fontes
(fakes nos testes do agente) e os clientes HTTP (HowLongToBeat e `game_time_to_beats` do IGDB) são testados com `httptest` e
respostas gravadas em `testdata`; o relógio é `domain.Clock` fake.
· verificação: revisão do diff; `go test ./... -race` sem internet.

### TTB-032 — Tempo para zerar de ponta a ponta
Dado o Arcadia com internet, quando abro o painel de cinco jogos conhecidos e de um jogo obscuro, abro os Detalhes no Big Box, aplico
metadados em lote após importar e uso "Atualizar" e "Escolher outro…" na edição, então os valores aparecem em poucos segundos sem
travar a tela, coincidem com o site, o jogo obscuro mostra "—", e sem internet nada quebra (só "—").
· verificação: manual.

### Reserva do IGDB (aprovação, 2026-11-22, resposta 1)

Regras em [rules §10](contracts/hltb-rules.md). A tabela de fontes vale para a fila, para TTB-025 e para `refresh`.

### TTB-033 — Reserva quando o HowLongToBeat não acha
Dada a busca de `G` com o HowLongToBeat respondendo `not-found` (nenhum resultado, ou `none` em TTB-004), quando a chave do IGDB está
cadastrada e o IGDB não está em recuo, então o agente consulta o IGDB (TTB-035, TTB-036): com valores, grava `found` com fonte `igdb`
(validade de 30 dias, TTB-006); sem `match`, sem valores, com falha ou em recuo do IGDB, grava `not-found` com fonte `hltb`. Dado o
HowLongToBeat respondendo `found` ou `doubt`, então o IGDB não é consultado.
· verificação: teste do serviço com os dois clientes falsos (um caso por linha da tabela de rules §10; IGDB sem chamada em `found` e
`doubt`).

### TTB-034 — Reserva quando o HowLongToBeat falha
Dada a busca de `G` com o HowLongToBeat em falha temporária (TTB-011) ou em recuo, quando o IGDB está disponível, então o agente
consulta o IGDB: com valores, grava `found` com fonte `igdb`; sem `match` ou sem valores, **não grava nada** (a fonte principal ainda não
respondeu) e a busca sai da fila; com falha do IGDB, também não grava. Em `refresh`, "nada gravado" é `-32011` (TTB-020).
· verificação: teste do serviço (HowLongToBeat que falha e em recuo; IGDB com valores, sem match e com falha).

### TTB-035 — Jogo do IGDB só com confiança alta
Dada a consulta à reserva para `G`, então o jogo do IGDB é procurado pela busca do IGDB da fase 12 (consulta com a plataforma e, se
preciso, sem ela, MET-003), cada candidato pontuado por `domain.ScoreGroup` com título, ano e chave de seed da plataforma de `G` e
classificado por `domain.ClassifyMatch`; só `match` segue para os valores. `doubt` e `none` contam como "sem match" (não guardam
candidatos do IGDB: a escolha manual, TTB-019, é só entre jogos do HowLongToBeat).
· verificação: teste do serviço com o cliente IGDB falso (vetores de match, dúvida e nenhum).

### TTB-036 — Valores do IGDB
Dado o jogo do IGDB escolhido, então os valores vêm de `game_time_to_beats` desse jogo, em segundos: `hastily` → História principal
(`mainSeconds`), `normally` → História + extras (`mainExtraSeconds`), `completely` → 100% (`completionistSeconds`); cada valor 0,
ausente, negativo ou não inteiro é nulo. Sem registro de `game_time_to_beats` ou com os três nulos, conta como "sem valores". A linha
gravada guarda o id e o nome do jogo no IGDB.
· verificação: teste do cliente com `httptest` (corpo da consulta, mapeamento, campos ausentes) e do serviço.

### TTB-037 — Sem chave do IGDB, a reserva é ignorada em silêncio
Dado que a credencial do IGDB não está cadastrada (`scrapers.ErrNoKey`), quando a reserva seria consultada, então nenhuma requisição ao
IGDB é feita, nada é registrado além de `Debug`, o IGDB não entra em recuo, nenhum aviso aparece na UI e o resultado é o do
HowLongToBeat sozinho (TTB-033: `not-found`; TTB-034: nada gravado). Cadastrar a chave depois faz a reserva valer na próxima busca.
· verificação: teste do serviço com `KeyStore` falso sem a chave (cliente IGDB nunca chamado; log sem `Warn`).

### TTB-038 — Limite e recuo do IGDB
Dado qualquer requisição da reserva, então ela passa pelo mesmo `scrapers.Limiter` do IGDB usado pela busca de metadados (MET-010),
tem prazo de 10 s e conta na vez única de TTB-009. Uma falha temporária do IGDB (`scrapers.ErrUnavailable`) põe o **IGDB** em recuo
com a mesma escada de TTB-011 (1, 5, 30, 60 min), separada da do HowLongToBeat, com `Warn` "IGDB indisponível para o tempo para zerar"
uma vez por sequência; credencial recusada conta como falha temporária. O recuo do IGDB nunca impede consultas ao HowLongToBeat.
· verificação: teste do serviço com relógio fake (duas escadas independentes).

### TTB-039 — A fonte aparece junto dos valores
Dado um valor mostrado (painel, Detalhes do Big Box, painel do Mosaico, Detalhes da Estante, "Editar jogo"), então a linha da fonte diz
"Fonte: HowLongToBeat" para `source = hltb` e "Fonte: IGDB" para `source = igdb`; sem valor, a linha não aparece. Os rótulos dos três
valores são os mesmos nas duas fontes (TTB-014).
· verificação: teste de componente (uma fonte por caso).

### TTB-040 — Linha do IGDB e escolha manual
Dada a linha de `G` com fonte `igdb`, então ela nunca é `confirmed` (a reserva é automática), "Escolher outro…" (TTB-019) busca e lista
jogos do HowLongToBeat, e escolher um grava `found` confirmado com fonte `hltb`, que depois nunca cai na reserva sozinho (TTB-006).
`timeToBeat.choose` com `externalId = ""` grava `not-found` confirmado com fonte `hltb`.
· verificação: teste do agente.

---

## BBX — Big Box: mudanças comuns aos temas

Contrato completo em [contracts/bigbox-temas-v2.md](contracts/bigbox-temas-v2.md).

### BBX-046 — Temas disponíveis
Dado o registro de temas, então existem, nesta ordem, `faixa` ("Faixa"), `mosaico` ("Mosaico") e `estante` ("Estante"),
disponíveis, e `roda` ("Roda"), indisponível. O seletor "Tema do Big Box" do cabeçalho da janela normal lista os quatro pelo nome do
registro, com "Roda (em breve)" desativado; a escolha fica em `localStorage` (`arcadia.bigBoxTheme`, `lib/uiPrefs.ts`); valor
ausente, desconhecido ou indisponível vira `faixa` (BBX-023).
· verificação: teste do registro, de `uiPrefs` e de componente do cabeçalho.

### BBX-047 — Contrato de tema v2
Dado um tema, então ele declara, além de `id`, `name`, `available` e `listCapacity`: o eixo (`horizontal`, `vertical` ou `grid`,
com o número de colunas na grade), a âncora da faixa, o fundo (`contain`, `cover` ou `plain`), o lugar do vídeo (`background` ou
`cover`), o conjunto de abas (`platforms` ou `collections`), a ordem da lista (`title` ou `lastPlayed`), se mostra o tempo para
zerar em Navegar e, opcionalmente, um componente de Detalhes próprio. Ele continua recebendo só o modelo de exibição e as ações e
só desenha o modelo e repassa cliques; pode usar os componentes de apresentação compartilhados de `bigbox/` (`Panel`,
`FocusVideo`, `CoverArt`, `TimeToBeatList`), nunca bindings nem eventos. Um tema de teste para cada combinação navega pelo mesmo
reducer.
· verificação: teste do registro (campos obrigatórios de cada tema) e do shell com temas falsos.

### BBX-048 — Grade 2D no reducer
Dado um tema com eixo `grid` de `C` colunas, a aba com `T` jogos, `L` lidos e o último índice navegável `M = min(T, L) − 1`, na tela
Navegar: ← / → movem o foco um jogo (passando de fileira), limitado a `[0, M]`; ↓ leva o foco a `f + C` quando `f + C ≤ M`, ou a `M`
quando há fileira abaixo (`⌊f/C⌋ < ⌊M/C⌋`), e senão não faz nada; ↑ leva a `f − C` quando `f − C ≥ 0` e senão não faz nada. Nos
eixos `horizontal` e `vertical` nada muda (BBX-006, BBX-024). Na tela Detalhes as direções seguem BBX-032..BBX-034 em qualquer eixo.
· verificação: teste do reducer (tabela de [temas-v2 §3](contracts/bigbox-temas-v2.md): pontas, fileira parcial, `M` limitado por
`L`) e do hook de entrada (eixo `grid` emite `up`/`down` em Navegar).

### BBX-049 — Janela da grade
Dado a grade com `C` colunas, `R` fileiras inteiras visíveis (capacidade do tema) e o foco na fileira `r = ⌊f/C⌋`, então são
desenhados os jogos `[s, min(s + (R + 1)·C, T))` com `s = clamp(r, 0, max(⌈T/C⌉ − R, 0))·C`: a fileira do foco é a primeira
desenhada (a última fileira pode aparecer cortada embaixo) e nunca há mais de 30 cartões no DOM. A janela é recortada em `L`.
· verificação: teste da função pura `gridWindow` (começo, meio, fim, fileira parcial, `T = 0`) e de componente (contagem de nós).

### BBX-050 — Páginas e pré-busca da grade
Dado um tema `grid`, então cada página de `ListGames` tem `P = min((R + 1 + 3)·C, 500)` jogos, e a próxima página é pedida quando
`L < T` e `L < min(T, (r + R + 1 + 3)·C)` (a janela de três fileiras abaixo do foco ainda não foi lida), com as demais regras de
BBX-007 e BBX-038 (uma leitura em voo por aba, sem repetir nem cancelar ao mover o foco, troca de aba cancela). Para temas de faixa,
`pageSize` e `shouldPrefetch` não mudam.
· verificação: teste das funções puras `gridPageSize` e `shouldPrefetchGrid` e do shell com `ListGames` controlado por promessas
(andar para baixo pede cada página uma vez; a janela nunca passa de `L`).

### BBX-051 — Âncora da faixa por tema
Dado um tema de faixa com âncora `A`, então a janela de BBX-039 usa `s = clamp(f − A, 0, max(T − V, 0))`; o Faixa declara `A = 2`
(sem mudança) e o Mosaico `A = 1`.
· verificação: teste de `stripWindow` com âncora (vetores de [temas-v2 §4](contracts/bigbox-temas-v2.md)).

### BBX-052 — Fundo por tema
Dado o tema ativo, então o fundo segue o que ele declara: `contain` desenha a arte de BBX-010 inteira sobre o preenchimento
desfocado (BBX-040; Faixa); `cover` desenha a arte de BBX-010 em `object-fit: cover`, sem desfoque, coberta por um degradê vertical
que vai de transparente no topo, passa por `--arc-bb-scrim` no meio e chega a `--arc-bb-bg` opaco embaixo (Estante); `plain` desenha
só `--arc-bb-bg`, sem arte e sem vídeo (Mosaico). A transição de 300 ms vale para `contain` e `cover`.
· verificação: teste de componente de `BackgroundStage` (um caso por fundo).

### BBX-053 — Vídeo na capa em foco
Dado um tema com vídeo `cover`, então o fundo nunca toca vídeo, e o vídeo do jogo em foco toca **dentro da capa em foco**
(`FocusVideo`, `object-fit: contain` sobre `--arc-bb-surface`) com as regras de BBX-011..BBX-013: depois de 2.000 ms parado, só na
tela Navegar e fora da suspensão, um único `<video>` na tela, `src` removido ao trocar de foco, sem nova tentativa depois de
`error` enquanto o foco não sair, Y liga e desliga o som. Com vídeo `background`, vale BBX-011 como está.
· verificação: teste de componente com temporizadores falsos (um `<video>` no máximo, dentro do cartão em foco).

### BBX-054 — Abas de coleção
Dado um tema com abas `collections`, então a barra mostra "Todos", "Favoritos" e "Concluídos" e depois as plataformas de BBX-004;
"Favoritos" lê só jogos com `favorite = true` e "Concluídos" só os de `status = completed`, com a ordem do tema; uma aba de coleção
sem jogos mostra "Nenhum jogo nesta aba". A aba ativa mostra o total entre parênteses depois da primeira página ("Todos (23)"). A aba
inicial é a plataforma da janela normal, se houver, senão "Todos". O rodapé diz "LB/RB Aba" em vez de "LB/RB Plataforma". Com abas
`platforms`, vale BBX-004 como está.
· verificação: teste de `buildTabs` (filtros de cada aba) e de componente (rótulo com total; dica).

### BBX-055 — Ordem da lista por tema
Dado o tema ativo, então `ListGames` da aba usa a ordem que ele declara: `title` (Faixa, Mosaico) ou `lastPlayed` (Estante, jogados
mais recentes primeiro, nunca jogados por último). Trocar de tema só vale na próxima entrada no Big Box.
· verificação: teste do hook de dados (parâmetro `sort` enviado).

### BBX-056 — Detalhes próprios do tema
Dado um tema com componente de Detalhes, então a tela Detalhes usa esse componente com as mesmas propriedades de `BigBoxDetails`
(jogo, mídia, sessão, foco, item da galeria, som, sinal de rolagem, tempo para zerar, ações), os mesmos estados de foco
(BBX-032..BBX-036), uma única marca de foco (BBX-035) e as mesmas dicas (BBX-015); sem componente, vale `BigBoxDetails`.
· verificação: teste do shell com um tema falso de Detalhes próprios (mesmas transições e dicas).

### BBX-057 — Select segurado bloqueia A e ↑/↓ na UI
Dado o botão Select/Back (botão 8) pressionado, então a UI ignora A, B, Y e as direções ↑/↓ **do direcional** (botões 12 e 13),
inclusive a borda de subida que acontecer com Select apertado: Select + A (Big Box), Select + B, Select + Y e Select + ↑/↓ (volume)
são do agente. X, LB/RB, ←/→ e o analógico continuam valendo.
· verificação: teste do hook de entrada com `navigator.getGamepads` falso (cada botão com e sem Select).

### BBX-058 — Tokens completos por tema e contraste
Dado `index.css`, então cada tema disponível tem um bloco `[data-bigbox-theme="<id>"]` com **todos** os tokens de
[temas-v2 §7](contracts/bigbox-temas-v2.md) (os de BBX-025 e os novos `--arc-bb-heading`, `--arc-bb-chip`, `--arc-bb-on-chip` e
`--arc-bb-placeholder`), expostos ao Tailwind por `@theme inline`; a varredura de UIX-006 continua valendo para todo `bigbox/`. Os
pares de contraste de cada tema estão na mesma seção e são conferidos como BBX-026 (texto ≥ 4,5:1, foco ≥ 3:1).
· verificação: teste que lê os tokens de cada bloco (todos presentes) e calcula o contraste por tema.

### BBX-059 — Tempo para zerar no Big Box só por leitura
Dado o código de `frontend/src/bigbox/`, então o único acesso ao tempo para zerar é `TimeToBeatGet` (pelo módulo de `lib/`): nenhum
`TimeToBeatRefresh`, `TimeToBeatSearch`, `TimeToBeatChoose` nem `TimeToBeatSetSettings`. O Big Box pede `fetch: true` só ao abrir os
Detalhes e, num tema que mostra o tempo em Navegar, depois de 1.000 ms com o mesmo jogo em foco; durante a suspensão (BBX-018) não
pede nada. Download de imagem e vídeo continua proibido (BBX-029).
· verificação: teste de varredura de `bigbox/` e de componente com temporizadores falsos e espiões.

---

## MOS — Tema Mosaico (estilo Nintendo Switch)

Layout e tokens em [contracts/bigbox-temas-v2.md §5, §7](contracts/bigbox-temas-v2.md). Desenho de referência: fundo claro, abas em
pílula, fileira de capas quadradas, painel branco e rodapé de dicas. **Sem menu lateral.**

### MOS-001 — Estrutura da tela Navegar
Dado o Mosaico na tela Navegar, então, de cima para baixo, dentro da área segura (BBX-042): a barra de abas com a contagem à
direita; o título do jogo em foco; a fileira de capas; o painel de informações; e o rodapé de dicas (BBX-015). O fundo é liso
(`plain`, BBX-052) e não há arte de fundo nem menu lateral.
· verificação: teste de componente (ordem dos blocos; sem `bb-art`).

### MOS-002 — Abas em pílula
Dado o Mosaico, então cada aba é uma pílula de texto sobre `--arc-bb-surface` com `--arc-bb-text-muted`; a ativa tem fundo
`--arc-bb-accent` e texto `--arc-bb-on-accent`. LB/RB trocam de aba (BBX-005). Abas de plataforma (`platforms`).
· verificação: teste de componente.

### MOS-003 — Contagem de jogos
Dado a aba com `T ≥ 1` jogos, então à direita das abas aparece "`T` jogos" ("1 jogo" com `T = 1`); com "Carregando jogos…" ou aba
vazia, nada aparece.
· verificação: teste de componente.

### MOS-004 — Fileira de capas quadradas
Dado a aba carregada, então as capas aparecem numa fileira horizontal de cartões quadrados com a imagem inteira (`contain`, BBX-041);
a capa em foco é maior (escala 1,09) com contorno `--arc-bb-focus` de 5 px afastado 4 px; ←/→ movem o foco um jogo sem dar a volta;
a janela é a de BBX-039 com âncora 1 (BBX-051) e no máximo 15 cartões no DOM; com mais jogos que a capacidade, o último cartão
aparece cortado na borda direita. Clique foca, duplo clique lança (BBX-014).
· verificação: teste de componente (escala e contorno do focado; contagem de nós; índices da janela).

### MOS-005 — Capa substituta
Dado um jogo sem capa, então o cartão usa a substituta de BBX-008; sem nenhuma imagem, o cartão `--arc-bb-placeholder` com o título em
texto.
· verificação: teste de componente.

### MOS-006 — Título acima da fileira
Dado o jogo em foco, então o título dele aparece acima da fileira, em uma linha (cortada com "…"), com `--arc-bb-heading`, e muda a
cada movimento do foco; a marcação "Em execução"/"Pausado" (BBX-017) aparece ao lado.
· verificação: teste de componente.

### MOS-007 — Vídeo na capa em foco
Dado o jogo em foco com vídeo, então o vídeo toca dentro da capa em foco pelas regras de BBX-053.
· verificação: teste de componente com temporizadores falsos.

### MOS-008 — Painel: dados e descrição
Dado o jogo em foco, então o painel (`--arc-bb-panel`, cantos arredondados) tem três colunas; a primeira mostra a linha
"plataforma · ano · gênero" (BBX-009) e a descrição curta (BBX-009, até 280 caracteres) em no máximo 4 linhas.
· verificação: teste de componente.

### MOS-009 — Painel: tempo jogado
Dado o jogo em foco, então a segunda coluna mostra, em pares rótulo/valor: "Tempo jogado" (formato de UIX-028), "Última vez" (data,
ou "Nunca jogado") e "Sessões" (`playCount`).
· verificação: teste de componente.

### MOS-010 — Painel: tempo para zerar
Dado o jogo em foco, então a segunda coluna mostra, abaixo dos dados de MOS-009, "História principal", "História + extras" e "100%"
com os valores de TTB-014 ("—" sem dado, TTB-013), lidos por BBX-059 (cache na hora; busca depois de 1 s parado).
· verificação: teste de componente com temporizadores falsos.

### MOS-011 — Painel: miniaturas de telas
Dado o jogo em foco com screenshots, então a terceira coluna mostra as três primeiras (miniatura, `contain`, ordem de `media.list`);
com menos, só as que existem; sem nenhuma, a coluna não aparece e as outras duas ocupam o painel.
· verificação: teste de componente.

### MOS-012 — Capacidade da fileira
Dado o tamanho da janela, então `mosaicoCapacity` (pura) devolve quantos cartões cabem inteiros na largura útil pela fórmula de
[temas-v2 §5.2](contracts/bigbox-temas-v2.md) (1280×720 e 1920×1080 → 4), limitada a 1..14; tamanho desconhecido → 12.
· verificação: teste da função pura (vetores).

### MOS-013 — Estados da tela
Dado o Mosaico, então "Carregando jogos…", a aba vazia ("Nenhum jogo nesta plataforma" / "Nenhum jogo na biblioteca"), o aviso de
erro (BBX-020) e a marcação de sessão no cartão (BBX-017) aparecem como no Faixa, dentro de painéis (BBX-037).
· verificação: teste de componente.

### MOS-014 — Detalhes no Mosaico
Dado X no Mosaico, então a tela Detalhes é a comum (`BigBoxDetails`, BBX-016, com o tempo para zerar de TTB-016), desenhada com os
tokens do Mosaico (fundo claro, painel branco).
· verificação: teste de componente (`data-bigbox-theme="mosaico"` na raiz; `BigBoxDetails` montado).

### MOS-015 — Contraste do Mosaico
Dado o bloco de tokens do Mosaico, então `text`, `text-muted`, `heading` e `danger` têm contraste ≥ 4,5:1 com `panel` e com `bg`;
`on-accent` sobre `accent` e `on-chip` sobre `chip` ≥ 4,5:1; `focus` ≥ 3:1 contra `bg` e contra `panel`.
· verificação: teste de tokens (BBX-058).

### MOS-016 — Mosaico na TV
Dado o Arcadia numa TV com controle, quando uso o Mosaico, então troco de plataforma com LB/RB, ando pela fileira, o vídeo entra na
capa depois de ~2 s parado, o tempo para zerar aparece no painel depois de alguns segundos, X abre os detalhes claros e A lança; o
texto é legível a 3 m.
· verificação: manual.

---

## ETN — Tema Estante (estilo "Big Picture" de launchers de PC)

Layout e tokens em [contracts/bigbox-temas-v2.md §6, §7](contracts/bigbox-temas-v2.md). **Sem menu lateral.** O que a referência
tem e o Arcadia não tem fica de fora (ETN-013).

### ETN-001 — Estrutura da tela Navegar
Dado a Estante na tela Navegar com o foco na primeira fileira, então, dentro da área segura: o destaque do jogo em foco (ETN-002,
ETN-003) na parte de cima; o cabeçalho "Sua biblioteca" (ETN-004); a grade (ETN-005), com a fileira de baixo cortada; e o rodapé
(BBX-015). O fundo é a arte do jogo em foco em `cover` com degradê para o fundo embaixo (BBX-052), com o vídeo de fundo de BBX-011.
· verificação: teste de componente (ordem dos blocos; fundo `cover`).

### ETN-002 — Destaque do jogo em foco
Dado o jogo em foco, então o destaque mostra: o título grande em texto (`clamp(2.4rem, 4vw, 4.5rem)`, até duas linhas); a linha
"Última sessão `<relativo>`" ("hoje", "ontem", "há N dias" até 29, "há N meses" até 11, "há N anos"), ou "Nunca jogado"; o botão
**Iniciar jogo** com ícone de play (`--arc-bb-accent`), que age como A; o botão **Detalhes**, que age como X; e o indicador de
favorito (coração cheio com `favorite`, vazio sem), **só de leitura** (BBX-003). A marcação de sessão (BBX-017) fica ao lado do
título.
· verificação: teste de componente (relativo com relógio falso; clique nos botões; indicador sem `onClick`).

### ETN-003 — Blocos de estatística do destaque
Dado o jogo em foco, então à direita do destaque há dois blocos em painel: "Tempo jogado" (valor de UIX-028) e "Sessões"
(`playCount`).
· verificação: teste de componente.

### ETN-004 — Cabeçalho da biblioteca
Dado a Estante, então o cabeçalho mostra "Sua biblioteca", o rótulo da ordem ("Jogado recentemente", BBX-055) e a barra de abas de
coleção (BBX-054) com o selo "LB" antes da primeira aba e "RB" depois da última; a aba ativa fica destacada com `--arc-bb-accent` e o
total entre parênteses.
· verificação: teste de componente.

### ETN-005 — Cartões da grade
Dado a grade, então ela tem 5 colunas; cada cartão mostra a capa vertical (2:3, `contain`, BBX-041; substituta de BBX-008), o
título em uma linha (cortado com "…") e "Jogou por `<tempo>`" (formato de UIX-028) ou "Nunca jogado"; o cartão em foco tem o contorno
`--arc-bb-focus` e escala 1,05; a marcação de sessão é um selo na capa (BBX-017). Clique foca, duplo clique lança.
· verificação: teste de componente.

### ETN-006 — Navegação na grade
Dado a Estante, então a navegação é a de BBX-048 com `C = 5`; A lança o jogo em foco, X abre Detalhes, LB/RB trocam de aba, Y liga e
desliga o som do vídeo, B sai (BBX-002, BBX-005, BBX-013, BBX-014).
· verificação: teste do shell com controle falso (↓ da fileira 0 para a 1; → no fim da fileira passa à seguinte).

### ETN-007 — Cabeçalho fixo ao descer
Dado o foco na fileira `r ≥ 1`, então o destaque não aparece, o cabeçalho "Sua biblioteca" fica no topo da área segura e a grade
ocupa o resto da altura, começando pela fileira do foco (BBX-049); com o foco de volta na fileira 0, o destaque volta. A troca é
animada em até 300 ms.
· verificação: teste de componente (destaque presente com `r = 0` e ausente com `r = 1`; cabeçalho sempre presente).

### ETN-008 — Capacidade da grade
Dado o tamanho da janela, então `estanteRows` (pura) devolve as fileiras inteiras visíveis com o destaque fora, pela fórmula de
[temas-v2 §6.2](contracts/bigbox-temas-v2.md) (1280×720 e 1920×1080 → 1), limitada a 1..5; tamanho desconhecido → 2. A capacidade do
tema é `5 × R`.
· verificação: teste da função pura (vetores).

### ETN-009 — Estados da tela
Dado a Estante, então "Carregando jogos…", a aba vazia (BBX-021, BBX-054), o aviso de erro (BBX-020) e a marcação de sessão aparecem
dentro de painéis (BBX-037), com o cabeçalho e as abas sempre visíveis.
· verificação: teste de componente.

### ETN-010 — Detalhes da Estante: estrutura
Dado X na Estante, então a tela Detalhes própria (BBX-056) mostra, sobre o fundo `cover` do jogo: à esquerda (≈ 62 %), o título
grande, a descrição (rolagem de BBX-036), os botões **Iniciar jogo** e **Jogar sem contar tempo**, a faixa "Jogado por `<tempo>` ·
Jogado recentemente `<relativo>`" (ou "Nunca jogado") e o carrossel (ETN-011); à direita (≈ 38 %), os painéis de ETN-012. Tudo cabe na
área segura sem rolar a página (BBX-043); só a descrição rola.
· verificação: teste de componente (blocos e contenção, como BBX-043).

### ETN-011 — Detalhes da Estante: carrossel com bolinhas
Dado o jogo com vídeo ou screenshots, então o carrossel mostra o item selecionado inteiro (`contain`) num visor e, embaixo, uma
bolinha por item, com a selecionada cheia e as outras vazadas; o foco anda entre os botões e o carrossel e ←/→ trocam o item pelas
regras de BBX-032..BBX-034 (o vídeo toca no visor quando selecionado); com o foco no carrossel, só o visor tem a marca de foco
(BBX-035). Sem galeria, o carrossel não aparece e ↓ não faz nada.
· verificação: teste de componente (bolinhas = N; selecionada; uma marca de foco; vídeo no visor).

### ETN-012 — Detalhes da Estante: coluna da direita
Dado o jogo `G`, então a coluna da direita tem três painéis: "Estatísticas do jogo" (Tempo jogado, Sessões, Última vez); "Tempo para
zerar" (os três valores de TTB-016, "—" sem dado); e "Sobre" (Plataforma, Gêneros, Modos de jogo, Desenvolvedor, Publicado por, Ano
de lançamento), cada linha omitida quando vazia.
· verificação: teste de componente (linhas omitidas; valores do tempo para zerar).

### ETN-013 — O que a referência tem e a Estante não tem
Dado a Estante (Navegar e Detalhes), então **não** aparecem: logotipo do jogo (o Arcadia não tem a mídia `clear_logo`; o título é
texto), botão "Gerenciar" (o Big Box não edita, BBX-003; no lugar, "Detalhes"), favoritar pela tela (o indicador é só leitura),
campo de busca, seletor de ordem, filtro "Todos os jogos" (as abas fazem o papel), alternância lista/grade (só grade), abas
"Modernos" e "Clássicos", conquistas e barra de conquistas, avaliação, downloads, "jogando agora", requisitos do sistema, "Ver
similares", tempos "Co-Op" e "Vs" (no lugar, os três do HowLongToBeat) e menu lateral.
· verificação: teste de componente (nenhum desses textos ou controles no DOM).

### ETN-014 — Contraste da Estante
Dado o bloco de tokens da Estante, então `text`, `text-muted` e `danger` têm contraste ≥ 4,5:1 com `panel` composto sobre branco e
sobre preto e com `bg`; `on-accent` sobre `accent` e `on-chip` sobre `chip` ≥ 4,5:1; `focus` ≥ 3:1 contra `panel` composto e contra
`bg`.
· verificação: teste de tokens (BBX-058).

### ETN-015 — Estante na TV
Dado o Arcadia numa TV com controle, quando uso a Estante, então ando na grade com o direcional nas quatro direções, o destaque some
ao descer e volta na primeira fileira, LB/RB trocam entre Todos, Favoritos, Concluídos e as plataformas, X abre os detalhes com o
carrossel e o tempo para zerar, A lança e Y liga o som; o texto é legível a 3 m.
· verificação: manual.

---

## Fora desta spec

- **IGDB como fonte principal** e **escolha manual de jogos do IGDB** (a reserva é automática; a escolha manual é só do
  HowLongToBeat, TTB-040).
- **Reserva do IGDB quando o HowLongToBeat fica "em dúvida"** (os candidatos vão para o usuário, decisão q).
- **Tempo para zerar no bloco de informações do Faixa e no destaque da Estante** (só nos Detalhes nesses temas).
- **Barra de progresso "tempo jogado / história principal"** nos cartões (exigiria o tempo para zerar de cada cartão).
- **Busca por texto, ordem alternável e filtros dentro do Big Box** (sem teclado na tela para o controle).
- **Logotipos (`clear_logo`)** e o tema **Roda** (continua preparado, indisponível).
- **Configurar o combo Select + A** ou o tempo de segurar.
- **Detectar o jogo sem contar tempo no Select + A** (exige mudança no tracker; Limitações 2).
- **Movimento reduzido (`prefers-reduced-motion`)**, como na fase 13.

## Limitações conhecidas

1. **Select + A precisa do agente na bandeja.** Desde o ADR 0024 o agente não sobe com o Windows: depois do login, o Arcadia precisa
   ter sido aberto uma vez (fechar a janela deixa o agente na bandeja, DSK-002). Com o agente parado, nada lê o controle.
2. **Jogo sem contar tempo não bloqueia Select + A** (decisão a): o roteador só vê sessões; o Big Box pode subir por cima dele.
3. **O Windows pode negar o primeiro plano** ao Big Box aberto pelo agente (trava de primeiro plano; o agente não está em primeiro
   plano). O desktop força com `ForceForeground` (CON-023); se negar, a janela fica no topo e o botão pisca na barra de tarefas.
4. **O HowLongToBeat não tem API oficial.** O endpoint e o formato mudam sem aviso; o Arcadia trata como falha temporária, recua e
   mostra "—" até uma versão nova corrigir as regras. O uso automatizado pode contrariar os termos do site.
5. **A correspondência ignora a plataforma:** remakes com o mesmo nome e ano diferente ficam "em dúvida" até o usuário confirmar.
6. **A grade da Estante mostra uma fileira inteira em 16:9** (cartões grandes, 5 por fileira, como na referência); a de baixo aparece
   cortada.
7. **Os tempos do IGDB têm outra base** (apressado/normal/completo, com menos envios de usuários que o HowLongToBeat); por isso a fonte
   aparece junto dos valores (TTB-039) e o achado da reserva vale só 30 dias.

## ADRs propostos

A sessão principal escreve (a spec só os referencia). Próximo número livre: 0038.

1. **0038 — Big Box por pedido externo (Select + A e `--bigbox`):** o agente lê o terceiro combo do controle e, sem sessão aberta,
   executa o próprio exe com `--bigbox`; a UI lê `--bigbox` na primeira e na segunda instância (muda a regra de single-exe de ignorar
   os argumentos da segunda instância), guarda o pedido e o entrega uma vez ao frontend. Recusa com som durante um jogo.
2. **0039 — Tempo para zerar do HowLongToBeat com cache e falha silenciosa:** serviço não oficial consultado pelo agente, com
   endpoint descoberto em tempo de execução, uma requisição por vez a cada 2 s, recuo exponencial até 1 h, cache por jogo no banco
   (migração 10) com validade longa e "não encontrado" também guardado, correspondência automática só com `match` da fase 12,
   confirmação manual na edição e aviso `timeToBeat.changed` (ADR 0031). O IGDB (`game_time_to_beats`) é a reserva quando o
   HowLongToBeat não acha ou falha (resposta 1), com recuo próprio, achado válido por 30 dias e fonte mostrada; sem chave do IGDB, a
   reserva é ignorada em silêncio.
3. **0040 — Contrato de tema v2 do Big Box:** estende o ADR 0036 com grade 2D no reducer compartilhado, fundo e lugar do vídeo
   declarados pelo tema, abas de coleção, ordem por tema e tela de Detalhes própria opcional; tokens completos por tema.

## Respostas do usuário (aprovação, 2026-11-22)

Não há decisões em aberto. Os requisitos foram **aprovados** com estas respostas:

1. **Fonte do tempo para zerar:** opção (b). O HowLongToBeat é a fonte principal e o IGDB (`game_time_to_beats`) é a reserva quando o
   HowLongToBeat falha ou não acha. A fonte aparece ("Fonte: IGDB") e os valores são mapeados assim: apressado → História principal,
   normal → História + extras, completo → 100%. `igdb` entra no enum `source` (contrato e migração 10). Sem chave do IGDB, a reserva é
   ignorada em silêncio. Requisitos novos: TTB-033..TTB-040. Alterados: TTB-001, TTB-002, TTB-006, TTB-011, TTB-015, TTB-016,
   TTB-018, TTB-020, TTB-023, TTB-024, TTB-027 e TTB-031. Decisão q.
2. **As duas opções do HowLongToBeat vêm ligadas por padrão:** sim, como proposto (decisão k, TTB-023).
3. **Nome "Estante" e abas de coleção (Todos, Favoritos, Concluídos e depois as plataformas):** aprovados como propostos (decisões l
   e m, BBX-054, ETN-004).
