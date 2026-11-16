# Fase 10 — Nova interface: requisitos

- **Status:** rascunho para aprovação do usuário (2026-11-14). Há duas perguntas no fim do documento
  ("Perguntas ao usuário"). A spec já usa a recomendação de cada uma; se o usuário escolher outra coisa, os requisitos
  marcados mudam antes da aprovação.
- **Design:** [design.md](design.md) · **Contratos:** [contracts/selecao-do-jogo.md](contracts/selecao-do-jogo.md)
  (máquina de estados do jogo em foco e do painel) · **Tarefas:** `tasks.md` (o planner gera depois da aprovação).
- **Base:** [phase-5-library](../phase-5-library/requirements.md) (grade, filtros, página do jogo, edição, remoção),
  [phase-7-midia](../phase-7-midia/requirements.md) (capas e imagens, MID-006),
  [phase-8-estatisticas](../phase-8-estatisticas/requirements.md) (estatísticas e Big Box),
  [phase-9-plataformas](../phase-9-plataformas/requirements.md) (ícones e contagem de jogos por plataforma),
  [phase-4-pause](../phase-4-pause/requirements.md) e [phase-3-tracker](../phase-3-tracker/requirements.md) (botões de
  lançamento, painel de sessão, marcação de sessão).
- **ROADMAP:** fase 10 (pontos 2, 3 e 4). **ARCHITECTURE:** §2 (a UI nunca influencia o tempo de jogo), §11 (seletor do
  Explorer em todo campo de arquivo).
- **ADRs:** 0014 (dois processos), 0015 (o agente é o único escritor do banco), 0019 (lançamento sem rastrear). Nenhum é
  contrariado. ADRs propostos no fim.
- **Objetivo:** trocar a tela da biblioteca por um layout em três colunas, como o do LaunchBox (filtros à esquerda,
  grade de capas no centro, detalhes do jogo em foco à direita), acabar com a página do jogo, dar às janelas modais um
  tamanho proporcional à tela e abrir a janela principal maximizada. **Sem mudança no agente, no tracker nem no IPC.**

Formato: IDs `ÁREA-NNN`, critérios Dado/Quando/Então. Área nova: **UIX** (interface da fase 10). Todo ID precisa de um
teste que o cite no nome (`it("UIX-017 ...")`), exceto os marcados `· verificação: manual`.

Termos usados abaixo:
- **Jogo em foco:** o jogo escolhido com um clique (ou com o teclado) na grade; é o que o painel da direita mostra. No
  máximo um por vez. Não confundir com o **foco do teclado** (o elemento que recebe as teclas).
- **Painel:** a coluna da direita.
- **Leitura:** chamada de binding que não grava nada (`ListGames`, `GetGame`, `GameSessions`, `MediaList`, `Catalog`,
  `CurrentSession`, estatísticas). **Lançamento:** `LaunchGame` ou `LaunchGameUntracked`.

---

## Decisões já tomadas (não reabrir)

1. **Tema:** escuro, próximo do LaunchBox (fundo cinza-azulado muito escuro, colunas laterais um pouco mais claras,
   destaque azul), com **todas as cores em variáveis CSS** (tokens), para que um segundo tema seja só um novo conjunto
   de valores. (UIX-006, UIX-007)
2. **Big Box fica como está nesta fase:** não ganha visual novo. Ele continua sendo a mesma tela em tela cheia, com
   fonte maior e navegação por controle; só se adapta ao que deixou de existir (a página do jogo). (UIX-044)
3. **Sem mudança no agente, no tracker e no IPC** (contrato continua o da fase 9, v0.11.0). A única mudança fora do
   `frontend` é a janela em `cmd/arcadia/main_windows.go`. (UIX-001, UIX-047)
4. **A página do jogo deixa de existir.** Tudo o que ela mostrava vai para o painel; duplo clique no cartão lança o
   jogo. (UIX-018, UIX-048)
5. **Janela principal maximizada** ao abrir (não é tela cheia nem Big Box), com tamanho mínimo. (UIX-001, UIX-002)

## Decisões do spec-writer (a confirmar na aprovação)

- **a. Editar abre num modal.** O formulário de edição tem 14 campos (tags, caminho com "Procurar...", emulador) e não
  cabe bem na largura do painel; "Editar" abre o formulário num modal de tamanho médio. As regras de UI-039 e UI-040 não
  mudam. (UIX-032)
- **b. Uma plataforma por vez na coluna esquerda.** Clicar numa plataforma troca o filtro de plataforma por ela, como no
  LaunchBox; não há seleção de várias plataformas (o `FilterSpec` continua aceitando várias). (UIX-010)
- **c. A contagem ao lado de cada plataforma é o `gameCount` do catálogo** (PLT-014): o total de jogos da plataforma,
  sem depender dos outros filtros. Assim a coluna não faz uma consulta por plataforma. (UIX-009)
- **d. A seleção segue o foco do teclado na grade:** mover com as setas troca o jogo em foco (como no LaunchBox). Enter
  lança, como o duplo clique. (UIX-019)
- **e. O jogo em foco não é desmarcado por filtro:** se ele sair do resultado, o painel continua mostrando o jogo e
  nenhum cartão fica marcado. Só a remoção (ou o jogo deixar de existir) limpa o jogo em foco. (UIX-021, UIX-023)
- **f. Os rótulos "Remover" e "Buscar metadados".** O botão de remoção continua "Remover" (mantém os textos de UI-041 e
  DSK-018). O antigo "Buscar mídia" passa a "Buscar metadados", que é o que o diálogo faz (metadados e imagens).
  (UIX-032..UIX-034)
- **g. O cartão perde plataforma, tempo jogado e botões.** Fica só a capa (ou o placeholder), o título e a marcação de
  sessão; o resto está no painel. (UIX-014)
- **h. A área do jogo em foco não é salva entre execuções.** Ao abrir a UI de novo, nenhum jogo está em foco. Só a
  visualização (grade ou lista) continua salva, como em UI-030. (UIX-024)

---

## Requisitos de fases anteriores afetados

Cada um recebeu, na spec de origem, a anotação `· alterado em 2026-11-14 (docs/specs/phase-10-nova-interface, UIX-NNN)`
(ou `· substituído por ...`). Se esta spec não for aprovada, as anotações são desfeitas.

| Requisito | Fase | Situação | Por |
|---|---|---|---|
| UI-006 — Jogo em execução | 3 | alterado: a marcação fica no cartão (selo sobre a capa) e no painel, não mais na página | UIX-014, UIX-025 |
| UI-019 — Botão "Sem contar tempo" | 4 | alterado: o botão fica no painel, com o rótulo "Jogar sem contar tempo" | UIX-027 |
| UI-021 — Pendência bloqueia os dois botões | 4 | alterado: vale para os dois botões do painel | UIX-027 |
| UI-029 — Grade virtualizada de cards | 5 | alterado: o cartão mostra capa (ou placeholder), título e marcação; sem plataforma, tempo e botões | UIX-014..UIX-016 |
| UI-030 — Alternar grade e lista | 5 | alterado (pergunta 1): a lista fica, sem botões, com as regras de seleção da grade | UIX-020 |
| UI-031 — Botões no card e na linha | 5 | substituído: os botões vão para o painel; o duplo clique lança | UIX-018, UIX-027 |
| UI-032 — Painel de filtros | 5 | alterado: plataforma vira a lista da coluna esquerda; as demais facetas ficam abaixo dela | UIX-009..UIX-011 |
| UI-033 — Busca com espera | 5 | inalterado (só muda de lugar: topo da coluna esquerda) | UIX-008 |
| UI-034 — Ordenação | 5 | inalterado (barra do centro) | UIX-013 |
| UI-035 — Limpar filtros | 5 | alterado: o botão fica na coluna esquerda e também volta a plataforma para "Todos" | UIX-011 |
| UI-037 — Página do jogo | 5 | substituído pelo painel | UIX-025, UIX-028, UIX-029, UIX-048 |
| UI-038 — Sessões na página do jogo | 5 | alterado: a lista fica no painel; regras iguais | UIX-030 |
| UI-039 — Editar jogo | 5 | alterado: o formulário abre num modal a partir do painel (decisão a) | UIX-032 |
| UI-041 — Remover jogo | 5 | alterado: sucesso limpa o jogo em foco e o painel volta ao estado vazio | UIX-034 |
| UI-043 — Cards de jogo em sessão | 5 | alterado: selo sobre a capa | UIX-014 |
| MID-006 — Exibição na grade e na página | 7 | alterado: cartão sempre com capa ou placeholder; imagens no carrossel do painel | UIX-015, UIX-026 |
| EMU-003 — Aviso "sem emulador configurado" | 6b | alterado: o aviso aparece no painel | UIX-025 |
| EST-001 — Tempo por jogo | 8 | alterado: escolher um jogo volta à biblioteca com ele em foco no painel | UIX-045 |
| BIG-001, BIG-002 — Big Box | 8 | alterados: "página do jogo" vira o painel; B no painel volta ao cartão | UIX-044 |
| BIG-003 — Sem depender do mouse | 8 | lacuna registrada: hoje a busca e os filtros não são navegáveis pelo controle | UIX-044 |
| PLT-032 — Filtro sem plataformas vazias | 9 | alterado: a regra passa a valer para a lista de plataformas da coluna esquerda | UIX-009 |

Inalterados e ainda válidos (só mudam de lugar na tela): UI-005, UI-008, UI-009, UI-012..UI-018, UI-020, UI-022..UI-028,
UI-033, UI-034, UI-036, UI-040, UI-042, UI-044, TAG-001..TAG-005, IMP-010, EMU-006, PLT-015..PLT-023, EST-002..EST-006,
BIG-004 e todos os requisitos de agente, store, IPC, tracker e desktop.

**Lacuna encontrada (registrada aqui):** quando uma sessão termina, a UI recarrega a grade e as estatísticas, mas não o
detalhe nem as sessões do jogo aberto; a página atual mostrava o total antigo até reabrir. UIX-031 corrige.

---

## UIX — Janela principal

### UIX-001 — Janela abre maximizada
Dado o `arcadia.exe` iniciado sem `--agent`, quando a janela principal aparece, então ela está maximizada (ocupa a área
de trabalho do monitor, com a barra de título e a barra de tarefas do Windows visíveis), não está em tela cheia e não
está no Big Box (o Big Box continua só pelo F11, BIG-001).
· verificação: teste em `cmd/arcadia` da função que monta as opções da janela (`WindowStartState` é `Maximised`,
`Fullscreen` é falso); conferência manual.

### UIX-002 — Tamanho mínimo e tamanho restaurado
Dado a janela maximizada, quando o usuário a restaura, então ela fica com 1280 × 800 pixels lógicos (limitada à área de
trabalho, se for menor) e não pode ser redimensionada abaixo de 1024 × 640.
· verificação: teste das opções da janela (`Width` 1280, `Height` 800, `MinWidth` 1024, `MinHeight` 640); manual.

### UIX-003 — Estado da janela respeitado depois de aberta
Dado a janela restaurada pelo usuário, quando uma segunda instância da UI é iniciada (EXE-011), então a janela existente
vem para a frente no tamanho em que estava (não é maximizada à força). Dado o Big Box ativo, quando o usuário sai dele
(BIG-001), então a janela volta ao estado de antes de entrar (maximizada, se estava maximizada).
· verificação: manual.

---

## UIX — Layout e tema

### UIX-004 — Três colunas
Dado a biblioteca, quando a tela é mostrada, então abaixo do cabeçalho (e do painel de sessão, UI-012, quando existe) há
três colunas lado a lado: a esquerda (`<nav>` com o nome acessível "Filtros da biblioteca"), a do centro (região com o
nome "Jogos") e a direita (`<aside>` com o nome "Detalhes do jogo"). Cada coluna rola sozinha; o documento em si não rola.
· verificação: teste de componente (os três marcos existem; o contêiner principal tem altura da janela e
`overflow: hidden`).

### UIX-005 — Larguras das colunas
Dado uma janela de largura L, então a coluna esquerda tem `clamp(220px, 16vw, 300px)`, a direita `clamp(320px, 26vw,
480px)` e o centro fica com o resto. Na largura mínima (1024), a grade mostra pelo menos 2 cartões por linha; em
1920, pelo menos 5.
· verificação: teste de unidade da função que calcula as colunas da grade a partir da largura do centro (larguras de
centro de 484 px e 1132 px); conferência manual em 1024 × 640, 1366 × 768 e 1920 × 1080.

### UIX-006 — Cores só por variáveis do tema
· alterado em 2026-11-15 (docs/specs/phase-13-modo-console, BBX-025): a varredura inclui `frontend/src/bigbox/` e os tokens
`--arc-bb-*` sob `[data-bigbox-theme="faixa"]`.
Dado o código do frontend, quando a suíte roda, então nenhum arquivo `.tsx` em `frontend/src` usa classe de cor da
paleta do Tailwind (ex.: `bg-neutral-900`, `text-emerald-400`, `border-red-500`, `bg-black/60`, `text-white`) nem cor
literal (`#rgb`, `rgb(...)`, `hsl(...)`) em `className` ou `style`; as cores vêm só dos tokens do tema (`--arc-*`, lista
em [design.md §3](design.md#3-tema-uix-006-uix-007)), com os valores definidos uma vez em `index.css` sob
`[data-theme="dark"]` e expostos ao Tailwind como utilitários próprios (`bg-surface`, `text-muted`...), e o elemento
raiz tem `data-theme="dark"`. Os SVGs de ícone de plataforma (`assets/platforms/`) e o placeholder de capa ficam fora
da regra.
· verificação: teste que varre os arquivos (lista de padrões proibidos) e confere que cada token da lista existe em
`index.css`.

### UIX-007 — Contraste mínimo do tema
Dado o tema escuro, quando os pares de cores da lista do design ([design.md §3](design.md#3-tema-uix-006-uix-007)) são
comparados, então todo texto (inclusive o secundário, `--arc-text-muted`, e o texto sobre botões coloridos) tem contraste
de pelo menos 4,5:1 com o fundo onde aparece, e o contorno de foco (`--arc-focus`) tem pelo menos 3:1 com os fundos.
· verificação: teste que lê os valores dos tokens em `index.css` e calcula o contraste (fórmula WCAG 2.x);
conferência visual manual da proximidade com o LaunchBox.

---

## UIX — Coluna esquerda

### UIX-008 — Busca no topo da coluna esquerda
Dado a coluna esquerda, então o primeiro controle é o campo de busca, com as regras de UI-033 (espera de 300 ms; apagar o
texto remove o filtro `text`).
· verificação: teste de componente (posição e regra de UI-033 com o relógio injetável).

### UIX-009 — "Todos" e plataformas com ícone e contagem
Dado o catálogo carregado, então abaixo da busca há a lista "Plataformas": primeiro o item "Todos", depois cada plataforma
com pelo menos um jogo (e a que está selecionada, mesmo sem jogos, PLT-032), na ordem do catálogo (nome), cada uma com o
ícone (PLT-009..PLT-011), o nome e o número de jogos (`gameCount`, PLT-014). "Todos" mostra a soma de `gameCount` das
plataformas. Enquanto o catálogo não carregou, a lista mostra só "Todos", sem número.
· verificação: teste de componente com catálogo de fixture (plataforma com 0 jogos fica fora; selecionada fica).

### UIX-010 — Escolher a plataforma
Dado a lista de plataformas, quando o usuário clica numa plataforma P, então a biblioteca é recarregada com
`platformIds = [P]` (substituindo qualquer plataforma anterior), as outras facetas, o texto e a ordenação ficam como
estavam, e P fica marcada como atual (`aria-current="true"` e destaque visual). Quando clica em "Todos", então
`platformIds` sai do filtro e "Todos" fica marcado. Clicar no item que já é o atual não recarrega nada. Sem filtro de
plataforma, "Todos" é o atual.
· verificação: teste de componente (o `FilterSpec` enviado em cada caso; nenhuma chamada no clique repetido).

### UIX-011 — Demais filtros abaixo das plataformas
Dado a coluna esquerda, então abaixo da lista de plataformas fica a seção "Filtros" com os controles de UI-032 para
gênero, modo, desenvolvedora, editora, ano de/até, status, favorito, jogado e tempo mínimo/máximo (em horas), com os
mesmos valores e o mesmo `FilterSpec`, e o botão "Limpar filtros", que segue UI-035 e também volta a plataforma para
"Todos". O seletor "Plataforma" de UI-032 deixa de existir.
· verificação: teste de componente (cada controle gera o mesmo `FilterSpec` de antes; "Limpar filtros" volta a
`{"version": 1}`, à ordenação padrão e a "Todos").

### UIX-012 — Seção "Filtros" recolhível
Dado a seção "Filtros", quando o usuário toca no cabeçalho dela (botão com `aria-expanded`), então os controles aparecem ou
somem; o padrão é recolhida. Recolhida com N facetas ativas além da plataforma e do texto (N ≥ 1), o cabeçalho mostra
"Filtros (N)". Recolher não muda o filtro. A escolha (aberta ou recolhida) fica guardada na própria UI, como a
visualização de UI-030.
· verificação: teste de componente (contagem de facetas ativas; preferência local lida e gravada).

---

## UIX — Centro: barra e grade

### UIX-013 — Barra do centro
Dado a coluna do centro, então no topo há uma barra com o contador "N jogos" (UI-044), o seletor de ordenação (UI-034) e
a alternância grade/lista (UIX-020). Os estados de UI-044 e UI-035 ("Conectando ao agente…", "Nenhum jogo ainda", "Nenhum
jogo com estes filtros." com "Limpar filtros") aparecem no lugar da grade.
· verificação: teste de componente.

### UIX-014 — Cartão com capa e título
· alterado em 2026-11-14 (docs/specs/phase-10-1-ajustes, AJU-001, AJU-015): a área da capa pode mostrar a imagem
substituta; o contorno de seleção aparece inteiro em todas as bordas da grade.
Dado um jogo na grade, então o cartão mostra a área da capa (proporção 3:4, ajustada na 1.0.1 para as capas não ficarem com tarjas) e, embaixo, o título em até duas linhas
(cortado com reticências; o título completo no atributo `title`). O cartão não mostra plataforma, tempo jogado nem
botões. Dado uma sessão do jogo em `running` ou `grace`, então um selo "Em execução" aparece sobre a capa; em `paused`,
"Pausado" (UI-006, UI-043); sem sessão do jogo, nenhum selo.
· verificação: teste de componente.

### UIX-015 — Capa ou placeholder, sempre
· alterado em 2026-11-14 (docs/specs/phase-10-1-ajustes, AJU-015, AJU-016): sem capa, o cartão usa outra imagem do jogo
(tela de título, tela de seleção, screenshot, banner, fundo) antes do placeholder.
Dado um jogo com capa (MID-006), quando o cartão é mostrado, então aparece a miniatura da capa, inteira dentro da área
(sem cortar). Dado um jogo sem capa, ou com a lista de mídia ainda sem resposta ou com erro, ou uma imagem que falha ao
carregar, então aparece o placeholder padrão (imagem embutida, sem rede), do mesmo tamanho, de modo que a grade não muda
de altura quando a capa chega.
· verificação: teste de componente (com capa; sem capa; evento `error` da imagem troca pelo placeholder).

### UIX-016 — Virtualização e paginação mantidas
Dado uma biblioteca grande, então a grade nova continua obedecendo UI-029 (no máximo 100 cartões no DOM, em teste com
tamanho fixo) e UI-036 (a próxima página é pedida uma única vez perto do fim do que foi carregado).
· verificação: os testes de UI-029 e UI-036 continuam passando com o cartão novo.

### UIX-017 — Um clique põe o jogo em foco
Dado a grade, quando o usuário clica num cartão do jogo G, então G passa a ser o jogo em foco: o cartão dele fica marcado
(`aria-selected="true"` e borda de destaque), o de qualquer outro jogo deixa de ficar, e o painel mostra G (UIX-025).
Nenhum binding além de leituras é chamado.
· verificação: teste de componente com espiões nos bindings.

### UIX-018 — Duplo clique lança
Dado um cartão do jogo G, quando o usuário dá um duplo clique nele, então G fica em foco e a UI chama `LaunchGame(G)`
exatamente uma vez (lançamento rastreado, como "Jogar"). Dado um lançamento pendente (UI-021), então o duplo clique só
põe G em foco e não chama nada. O duplo clique nunca chama `LaunchGameUntracked`. Erros seguem UI-008.
· verificação: teste de componente (uma chamada por duplo clique; nenhuma com pendência).

### UIX-019 — Teclado na grade
Dado a grade fora do Big Box, então ela é uma única parada de Tab: ao receber o foco do teclado, o foco vai para o cartão
do jogo em foco (se estiver carregado) ou para o primeiro cartão. As setas movem o foco para o cartão vizinho (esquerda,
direita, linha de cima, linha de baixo, na mesma coluna ou na última da linha, se for mais curta), Home e End para o
primeiro e o último cartão carregado, e o jogo em foco acompanha o foco do teclado (decisão d). O cartão focado rola para
a área visível; chegar perto do fim do que foi carregado pede a próxima página (UI-036). Enter lança o jogo do cartão
focado, como o duplo clique (UIX-018). Nas bordas, a seta não faz nada.
· verificação: teste de componente com eventos de teclado (grade de tamanho fixo, 3 colunas).

### UIX-020 — Visualização em lista (pergunta 1: manter)
· alterado em 2026-11-14 (docs/specs/phase-10-1-ajustes, AJU-002): o contorno de foco da linha aparece inteiro.
Dado a alternância grade/lista (UI-030, preferência local mantida), quando a lista está ativa, então cada linha mostra
título, plataforma, tempo jogado (UI-005), última vez jogado e a marcação de sessão, sem botões; um clique põe o jogo em
foco, o duplo clique lança, as setas para cima e para baixo movem o foco (e o jogo em foco), Enter lança, e a lista
continua virtualizada (UI-030, UI-036). As regras de UIX-017, UIX-018, UIX-019 e UIX-021 valem iguais.
· verificação: teste de componente.

---

## UIX — Jogo em foco

### UIX-021 — Jogo em foco preservado ao filtrar
Dado o jogo G em foco, quando o usuário muda a busca, uma faceta, a plataforma ou a ordenação, então G continua em foco e
o painel continua mostrando G, sem piscar o estado vazio. Se G está nas páginas carregadas do resultado novo, então o
cartão dele aparece marcado e a grade rola até ele; senão, nenhum cartão fica marcado e a grade começa do topo. Dado que G
volta a aparecer depois (outro filtro), então o cartão dele volta a ficar marcado.
· verificação: teste de componente (filtro que mantém G; filtro que tira G; filtro que o traz de volta).

### UIX-022 — Jogo em foco preservado ao trocar de tela
Dado o jogo G em foco, quando o usuário alterna grade e lista, abre e fecha as estatísticas, abre e fecha qualquer modal,
entra e sai do Big Box, ou a UI reconecta ao agente (UI-009), então G continua em foco e o painel volta a mostrar G (com
os dados recarregados, no caso da reconexão).
· verificação: teste de componente para cada transição.

### UIX-023 — Jogo em foco que deixa de existir
Dado o jogo G em foco, quando a leitura de G responde `-32001` (jogo inexistente, por exemplo removido por outra janela),
então G deixa de estar em foco, o painel mostra o estado vazio (UIX-024) e a grade é recarregada uma vez.
· verificação: teste de componente com o binding respondendo o erro.

---

## UIX — Painel da direita

### UIX-024 — Painel sem jogo em foco (pergunta 2: resumo)
· alterado em 2026-11-14 (docs/specs/phase-10-1-ajustes, AJU-017): a capa pequena segue a regra da imagem substituta.
Dado nenhum jogo em foco (ao abrir a UI, decisão h, ou depois de UIX-023 e UIX-034), então o painel mantém a mesma largura
e mostra "Nenhum jogo selecionado", o texto "Clique em um jogo para ver os detalhes. Clique duas vezes para jogar." e,
quando há jogos já jogados, a lista "Jogados recentemente" com até 5 jogos (`ListGames` com `played = "played"`, ordem
`lastPlayed`, `limit = 5`), cada um com a capa pequena (ou o placeholder), o título e a última vez jogado. Clicar num deles
o põe em foco (UIX-017); a lista não lança nada. Com a biblioteca vazia, a lista não aparece.
· verificação: teste de componente (os params da leitura; clique põe em foco; nenhuma chamada de lançamento).

### UIX-025 — Cabeçalho do painel
Dado o jogo G em foco, então o painel mostra, no topo: o título de G, o ícone e o nome da plataforma, a marcação de
sessão (UI-006: "Em execução" ou "Pausado"), o aviso "Rastreio por processo: pausa e processos filhos não entram no tempo"
quando a sessão de G é por processo adotado, e o aviso "Sem emulador configurado: escolha um padrão em Emuladores ou neste
jogo." quando G é de emulador sem emulador próprio nem padrão da plataforma (EMU-003). Os textos são os da página atual.
· verificação: teste de componente.

### UIX-026 — Carrossel de imagens
· alterado em 2026-11-14 (docs/specs/phase-10-1-ajustes, AJU-009..AJU-014): substituído por AJU-009..AJU-014 (carrossel com imagem em
destaque e faixa de miniaturas (sem botões anterior/próximo, sem volta nas pontas, ordem fixa por tipo)).
Dado G em foco com n imagens (`MediaList`, MID-006), então abaixo do cabeçalho há um carrossel que mostra uma imagem por
vez, inteira dentro de uma área 16:9 da largura do painel, a capa primeiro e as demais na ordem recebida, com o rótulo do
tipo ("Capa", "Screenshot", "Tela de título", "Banner", "Fundo", "Tela de seleção") e o indicador "k de n". Com n ≥ 2,
os botões "Imagem anterior" e "Próxima imagem" (e as setas esquerda e direita com o foco no carrossel) passam para a
vizinha, dando a volta nas pontas; com n = 1, não há botões; com n = 0 (ou a leitura com erro), a área mostra o
placeholder padrão, sem botões. O carrossel não avança sozinho. Trocar o jogo em foco volta à primeira imagem. Uma
imagem que falha ao carregar mostra o placeholder no lugar dela.
· verificação: teste de componente (n = 0, 1, 3; volta nas pontas; troca de jogo).

### UIX-027 — "Jogar" e "Jogar sem contar tempo"
· alterado em 2026-11-14 (docs/specs/phase-10-1-ajustes, AJU-006, AJU-007, AJU-008): "Jogar" em destaque grande,
"Jogar sem contar tempo" logo abaixo com destaque menor, cursor de mão; as regras de chamada e pendência não mudam.
Dado G em foco, então abaixo do carrossel ficam os botões "Jogar" (chama `LaunchGame(G)` uma vez) e "Jogar sem contar
tempo" (chama `LaunchGameUntracked(G)` uma vez, com as regras de UI-020). Dado um lançamento pendente de G, então os dois
ficam desativados e o acionado mostra "Iniciando…" (UI-021). Dado um lançamento pendente de outro jogo, então os botões de
G ficam ativos e o clique não chama nada até o pendente terminar (um lançamento por vez, como hoje). Erros seguem UI-008.
· verificação: teste de componente com espiões nos bindings.

### UIX-028 — Tempo jogado
Dado G em foco, então o painel mostra o tempo total (UI-005), a primeira e a última vez jogado (data e hora locais, ou
"Nunca") e, quando houver, "Concluído em" (data em UTC, decisão h da fase 5), todos como vêm de `GetGame`; a UI não
calcula nem soma tempo.
· verificação: teste de componente com detalhe de fixture.

### UIX-029 — Metadados
Dado G em foco, então o painel mostra status, favorito, gêneros, modos de jogo, desenvolvedora, editora, ano e descrição,
com os rótulos de UI-037 e "—" para listas vazias e ano ausente; a descrição aparece inteira (o painel rola).
· verificação: teste de componente.

### UIX-030 — Sessões no painel
Dado G em foco, então o painel mostra a seção "Sessões" com as regras de UI-038 (50 por página, "Carregar mais", início,
fim ou "Em andamento", tempo ativo, estado, "Descartada" sem somar no total).
· verificação: os testes de `SessionsList` continuam passando; teste de componente do painel com a seção.

### UIX-031 — Recarga quando a sessão de um jogo termina
Dado o jogo G em foco, quando chega `session:changed` com `ended` ou `discarded` de uma sessão de G, então o detalhe de G
(tempo total, última vez) e a lista de sessões de G são lidos de novo uma vez, além da recarga da grade e das
estatísticas que já existe (UI-006). Uma sessão de outro jogo não recarrega o detalhe de G.
· verificação: teste de componente com evento simulado (lacuna registrada acima).

### UIX-032 — Editar num modal
Dado G em foco, quando o usuário toca "Editar" no painel, então abre o modal "Editar jogo" (tamanho médio, UIX-037) com o
formulário de UI-039 e UI-040 (mesmos campos, validações, "Procurar..." e patch só com o que mudou). "Salvar" com sucesso
fecha o modal e o painel mostra o detalhe devolvido; com erro, o modal continua aberto com os valores digitados e a
mensagem de DSK-011; "Cancelar" (ou Esc) fecha sem chamar o agente.
· verificação: teste de componente.

Critérios acrescentados após a revisão (2026-11-14):
- **Salvamento pendente.** Dado o modal "Editar jogo" com "Salvar" acionado e a resposta do agente ainda pendente, quando
  o usuário aperta Esc, então o modal não fecha (o fechamento fica desativado, UIX-039) e, quando a resposta chega, o
  resultado aparece sempre: sucesso fecha o modal e o painel mostra o detalhe devolvido; erro mantém o modal com os valores
  digitados e a mensagem. Nenhum resultado de salvamento é descartado em silêncio.
- **Leitura do jogo ainda carregando ou com falha.** Dado G em foco com a leitura de G (`GetGame`) ainda sem resposta,
  quando o usuário aciona "Editar", então o modal "Editar jogo" abre mostrando "Carregando jogo…" (sem formulário) e, quando a
  leitura chega, mostra o formulário. Dado a leitura de G com erro, quando o usuário aciona "Editar", então o modal abre
  com a mensagem de erro (`role="alert"`) e sem formulário. Em nenhum dos dois casos o clique em "Editar" fica sem efeito
  visível. (O conteúdo do painel continua regido por UIX-035, sem mudança.)
- **Diálogos por jogo.** Dado o jogo A em foco com um erro de edição exibido ou um estado de diálogo qualquer, quando o
  jogo em foco passa a ser B, então os diálogos de B começam do zero: nenhuma mensagem de erro, pendência ou valor de A
  aparece no modal de B.

### UIX-033 — Buscar metadados
· alterado em 2026-11-14 (docs/specs/phase-10-2-imagens-ao-vivo, IMG-011): as imagens baixadas pela fila depois de
aplicar aparecem no carrossel e no cartão conforme chegam (`media.changed`), sem trocar o jogo em foco.
· alterado em 2026-11-15 (docs/specs/phase-12-metadados-unificados, MET-043): o diálogo não tem mais a escolha de provedor;
tem o campo "Nome para busca" (com o título do jogo) e o botão "Obter dados", e a lista de opções é única.
Dado G em foco, quando o usuário toca "Buscar metadados", então abre o diálogo de busca de metadados de G (MID-002..MID-004,
tamanho médio). Depois de aplicar um candidato, o painel mostra os dados novos e o carrossel lê a lista de mídia de novo.
· verificação: teste de componente.

### UIX-034 — Remover a partir do painel
Dado G em foco, quando o usuário toca "Remover", então abre a confirmação de UI-041 (mesmo texto e botões). Sucesso: G
deixa de estar em foco, o painel mostra o estado vazio (UIX-024) e a grade é recarregada sem G (UI-042). `-32002`: a
mensagem de DSK-018 aparece no aviso do topo, a confirmação fecha e G continua em foco.
· verificação: teste de componente.

### UIX-035 — Carregando e erro no painel
Dado G em foco e a leitura de G ainda sem resposta, então o painel mostra "Carregando jogo…" (sem mostrar os dados do jogo
anterior). Dado um erro de leitura diferente de `-32001`, então o painel mostra a mensagem (`role="alert"`) e o botão
"Tentar de novo", que repete a leitura.
· verificação: teste de componente.

### UIX-036 — Rolagem do painel
Dado o painel com conteúdo maior que a altura, então só o painel rola (a grade não se mexe). Quando o jogo em foco muda,
então o painel volta ao topo.
· verificação: teste de componente (`scrollTop` volta a 0 na troca).

---

## UIX — Janelas modais

### UIX-037 — Tamanho proporcional à tela
Dado um modal aberto numa janela de largura L e altura A (em pixels lógicos), então ele tem o tamanho da sua classe:

| Classe | Modais | Largura | Altura |
|---|---|---|---|
| grande | Importar | `min(92vw, 1600px)` | `88vh` |
| média | Emuladores, Plataformas, Buscar metadados, Editar jogo | `min(75vw, 1100px)` | `80vh` |
| compacta | Provedores, Remover jogo (e as demais confirmações) | `clamp(420px, 40vw, 560px)` | conforme o conteúdo, até `80vh` |

Importar é sempre o maior. Na janela mínima (1024 × 640), todos cabem inteiros, sem cortar conteúdo.
· verificação: teste de componente (cada modal declara a classe certa, `data-size`); conferência manual em 1024 × 640 e
1920 × 1080.

Critério acrescentado após a revisão (2026-11-14):
- **Modal suspende o Big Box.** Dado o Big Box ativo e um modal aberto (qualquer classe), quando o usuário aperta setas,
  Enter, Shift+Enter, Y ou B, pelo teclado ou pelo controle, então nada acontece no fundo (o foco não se move na grade, no
  painel ou na coluna esquerda, nenhum jogo é posto em foco nem lançado, o Big Box não é encerrado) e o foco do teclado
  permanece dentro do modal. Ao fechar o modal, a navegação do Big Box volta a valer. (Ver também UIX-044.)

### UIX-038 — Rolagem interna
Dado um modal grande ou médio, então o título fica fixo no topo, os botões de ação ficam fixos embaixo e só o corpo rola;
a tela de trás não rola enquanto o modal está aberto. Na importação, a lista de candidatos usa toda a altura do corpo (não
fica limitada a uma altura fixa pequena).
· verificação: teste de componente (estrutura cabeçalho/corpo/rodapé; corpo com `overflow-y: auto`); manual.

### UIX-039 — Comportamento comum dos modais
Dado qualquer modal (UIX-037), então ele tem `role="dialog"`, `aria-modal="true"` e nome pelo título; ao abrir, o foco do
teclado vai para o primeiro controle (ou para "Cancelar", nas confirmações destrutivas, UI-041); Tab e Shift+Tab ficam
presos dentro dele; Esc faz o mesmo que o botão de fechar (ou "Cancelar") e não faz nada quando esse botão está
desativado (operação em andamento); clicar fora do modal não o fecha; ao fechar, o foco volta ao elemento que o abriu.
· verificação: teste de componente comum a todos os modais.

Critério acrescentado após a revisão (2026-11-14):
- **Clique que não pega foco.** Dado um modal aberto, quando o usuário clica no overlay (fora do diálogo) ou em texto não
  focável dentro do modal, então o foco do teclado não sai do diálogo (se o clique o tirou do controle atual, ele volta ao
  elemento `role="dialog"`); em seguida, Tab e Shift+Tab continuam presos no modal e Esc continua fechando-o (com a mesma
  regra de botão desativado acima).

---

## UIX — Acessibilidade básica

### UIX-040 — Foco visível
Dado qualquer elemento interativo (botões, campos, itens de plataforma, cartões, linhas da lista, controles do carrossel,
modais), quando ele recebe o foco pelo teclado, então mostra um contorno de foco visível com a cor `--arc-focus`
(UIX-007). Nenhum elemento remove o contorno sem pôr outro no lugar.
· verificação: teste que varre os `.tsx` (nenhum `outline-none` sem `focus-visible:` correspondente); conferência manual.

### UIX-041 — Ordem do teclado
Dado a tela da biblioteca, quando o usuário percorre com Tab, então a ordem é: cabeçalho, coluna esquerda (busca,
plataformas, filtros), barra do centro, grade (uma parada, UIX-019), painel (botões e seções, de cima para baixo). Nenhum
elemento usa `tabIndex` maior que 0.
· verificação: teste de componente da ordem; teste que varre os `.tsx` (sem `tabIndex` positivo).

### UIX-042 — Nomes e estados acessíveis
· alterado em 2026-11-14 (docs/specs/phase-10-1-ajustes, AJU-010, AJU-014): os botões "Imagem anterior" e "Próxima
imagem" dão lugar às miniaturas ("<tipo>, k de n"); o critério "uma tecla, uma ação" vale para a faixa de miniaturas.
Dado a tela, então: a grade tem `role="listbox"` com o nome "Jogos" e cada cartão `role="option"` com o título como nome
e `aria-selected` conforme o jogo em foco; cada item de plataforma é um botão com o nome "<plataforma>, N jogos" e
`aria-current` no atual; os botões do carrossel têm nome ("Imagem anterior", "Próxima imagem"); os ícones de plataforma,
as capas e o placeholder são decorativos (`alt=""`), porque o nome aparece ao lado.
· verificação: teste de componente por papel e nome (Testing Library `getByRole`).

Critério acrescentado após a revisão (2026-11-14):
- **Uma tecla, uma ação no carrossel.** Dado o Big Box ativo e o foco num botão do carrossel ("Imagem anterior" ou
  "Próxima imagem"), quando o usuário aperta uma seta esquerda ou direita, então acontece exatamente uma ação: a imagem
  troca uma vez e o foco do Big Box não se move por causa da mesma tecla (o carrossel marca o evento como tratado,
  `defaultPrevented`, e o Big Box ignora eventos já tratados). (Ver também UIX-044.)

### UIX-043 — Tudo alcançável pelo teclado
· alterado em 2026-11-14 (docs/specs/phase-10-1-ajustes, AJU-012): navegar no carrossel é pela faixa de miniaturas.
Dado a tela fora do Big Box, então toda ação de mouse desta fase tem um equivalente de teclado: pôr em foco (setas, UIX-019),
lançar (Enter, ou os botões do painel), escolher plataforma (Enter ou Espaço no item), abrir e fechar a seção "Filtros",
navegar no carrossel e acionar Editar, Buscar metadados e Remover. Nenhum ponto prende o foco fora dos modais.
· verificação: teste de componente por teclado para cada ação; conferência manual.

---

## UIX — Big Box e estatísticas

### UIX-044 — Big Box sobre o layout novo
· substituído por BBX-001..BBX-030 (docs/specs/phase-13-modo-console, 2026-11-15): o Big Box deixa de usar as três colunas;
`useBigBoxNav`, `cardGrid`, `gridNext` e `nextFocus` saem.
Dado o Big Box (BIG-001..BIG-004, sem visual novo), então ele usa as três colunas em tela cheia com a fonte maior, e:
- cada cartão é um item navegável (o próprio cartão); as direções movem cartão a cartão dentro da grade e, na borda da
  grade, a direita leva ao painel e a esquerda à coluna esquerda (busca, "Todos", plataformas e filtros também são itens
  navegáveis, o que fecha a lacuna de BIG-003);
- A (ou Enter) num cartão põe o jogo em foco e leva o foco do controle ao botão "Jogar" do painel; A nos outros itens os
  aciona;
- Y (ou Shift+Enter) num cartão lança o jogo dele (rastreado), como hoje;
- B (ou Esc) com o foco no painel volta o foco ao cartão do jogo em foco (no lugar de "voltar da página do jogo"); nas
  estatísticas, volta à biblioteca; na grade ou na coluna esquerda, sai do Big Box (BIG-001).
· verificação: teste de componente com eventos de teclado simulados (`useBigBoxNav`); manual com controle na TV.

Critérios acrescentados após a revisão (2026-11-14):
- **Suspenso com modal aberto.** Dado um modal aberto, então o Big Box não trata setas, A/Enter, Y/Shift+Enter nem B (teclado
  e controle): o critério de UIX-037 vale aqui.
- **Eventos já tratados.** Dado um evento de teclado que um controle já tratou (`defaultPrevented`, como as setas no
  carrossel, UIX-042), então o Big Box não age sobre ele.
- **"Focar Jogar" pendente é descartado.** Dado A/Enter num cartão de G (ou a escolha de G nas estatísticas, UIX-045) com a
  leitura de G ainda pendente, quando outro jogo H passa a ser o jogo em foco antes de G aparecer no painel, ou quando a
  leitura de G falha (erro ou `-32001`), então o pedido de levar o foco a "Jogar" é descartado: o foco não salta para
  "Jogar" de H nem para "Jogar" de G numa leitura posterior.

### UIX-045 — Estatísticas abrem o jogo no painel
· alterado em 2026-11-15 (docs/specs/phase-13-modo-console, BBX-003): as estatísticas saem do Big Box; a parte "No Big Box..."
deixa de existir.
Dado a página de estatísticas, quando o usuário escolhe um jogo G (EST-001), então a página fecha, a biblioteca volta com
os filtros que tinha e G fica em foco no painel (com a regra de UIX-021 se G estiver fora dos filtros). No Big Box, o
foco do controle vai para "Jogar" do painel.
· verificação: teste de componente.

Critério acrescentado após a revisão (2026-11-14):
- No Big Box, se outro jogo é posto em foco antes de G aparecer no painel, ou a leitura de G falha, o foco não vai para
  "Jogar" (regra do pedido pendente em UIX-044).

---

## UIX — Invariantes

### UIX-046 — A UI nunca influencia o tempo de jogo
Dado qualquer interação desta fase (pôr em foco, navegar pelo teclado ou pelo controle, carrossel, filtros, seção
recolhível, modais, maximizar ou restaurar a janela, entrar e sair do Big Box), então a UI só chama leituras e as escritas
já existentes acionadas pelo usuário (editar, remover, metadados, plataformas, emuladores, importar). Lançamentos só
acontecem por ação explícita (Jogar, Jogar sem contar tempo, duplo clique, Enter na grade, Y no Big Box), um por ação;
pausa e retomada só pelo painel de sessão (UI-027); a UI não calcula, soma nem altera tempo (UI-017, UI-025).
· verificação: teste de componente que percorre as interações com espiões em todos os bindings (nenhum lançamento,
`ToggleSession` ou escrita fora dos casos permitidos); revisão.

Critério acrescentado após a revisão (2026-11-14):
- **Tecla mantida não relança.** Dado o foco num cartão ou numa linha da grade/lista (fora do Big Box), quando o usuário
  mantém Enter apertado (eventos com `repeat = true` depois do primeiro), então `LaunchGame` é chamado uma única vez. Dado
  o Big Box com o foco num cartão, quando o usuário mantém Shift+Enter (ou Y) apertado, então `LaunchGame` é chamado uma
  única vez. Repetições automáticas da tecla nunca são uma nova ação de lançamento.

### UIX-047 — Sem mudança no agente, no tracker e no IPC
· alterado em 2026-11-14 (docs/specs/phase-10-1-ajustes, AJU-028): vale para a fase 10; na fase 10.1, agente, domínio e
importadores mudam; o tracker e o IPC (v0.11.0) continuam sem mudança.
· alterado em 2026-11-14 (docs/specs/phase-10-2-imagens-ao-vivo, IMG-013): na fase 10.2, o IPC ganha a notificação
`media.changed` (v0.12.0, só acréscimo) e o agente e o repasse do desktop mudam; o tracker continua sem mudança.
Dado o diff da fase, então não há mudança em `internal/` nem no contrato IPC (continua a v0.11.0 da fase 9), e nenhum
binding novo é criado em `cmd/arcadia`; a única mudança em Go é a das opções da janela (UIX-001, UIX-002).
· verificação: revisão do diff; o teste de contrato do IPC continua passando sem alteração.

### UIX-048 — Página do jogo removida
Dado o código do frontend, quando a suíte roda, então nenhum arquivo de `frontend/src` referencia `GamePage` nem
`GameScreen`, não existe botão "Voltar" na biblioteca nem o botão "Detalhes", e os testes da página foram removidos ou
movidos para o painel (como UI-045 fez na fase 5).
· verificação: teste que varre os arquivos.

### UIX-049 — Ponta a ponta da nova interface
Dado o Arcadia com uma biblioteca real de alguns milhares de jogos, em 1366 × 768 e em 1920 × 1080, quando abro a UI,
filtro por plataforma pela coluna esquerda, busco um título, ponho um jogo em foco, mudo o filtro, passo pelas imagens,
jogo com duplo clique, pauso pelo atalho, fecho o jogo, abro Importar, Emuladores e Editar, e entro e saio do Big Box,
então a janela abre maximizada, as três colunas cabem, o jogo em foco continua no painel ao filtrar, o tempo e as sessões
do painel se atualizam ao fechar o jogo, os modais têm o tamanho da classe e rolam por dentro, e o tempo da sessão é o
mesmo que o agente registrou.
· verificação: manual.

---

## Fora de escopo

- Visual novo para o Big Box (decisão 2), tema claro ou segundo tema (só fica possível pelos tokens), escolha de tema
  pelo usuário.
- Tamanho dos cartões ajustável, colunas redimensionáveis ou recolhíveis pelo usuário, lembrar posição e tamanho da
  janela entre execuções.
- Seleção de vários jogos (ações em lote), seleção de várias plataformas na coluna esquerda, agrupamento por outra coisa
  além de plataforma (gênero, ano, playlist).
- Ampliar a imagem do carrossel em tela cheia, vídeos no carrossel (MID-007 adiado), troca da capa pelo painel.
- Janela de revisão da importação (fase 11) e busca unificada de metadados (fase 12): esta fase só muda o tamanho dos
  diálogos atuais.
- Lembrar o jogo em foco entre execuções (decisão h).

## Limitações conhecidas

A confirmar na aprovação; não geram requisito nem teste.

1. **Jogos do LaunchBox sem capa:** a importação do LaunchBox (MID-010) não lê a pasta `Box - Front`, então esses jogos
   aparecem com o placeholder até uma busca de metadados trazer a capa. Ler `Box - Front` é um item para a fase 11 ou 12.
   · alterado em 2026-11-14: tratado na fase 10.1 ([phase-10-1-ajustes](../phase-10-1-ajustes/requirements.md), AJU-018)
   para jogos importados a partir dela; os já importados ganham as imagens na reimportação com a base limpa.
2. **Imagens que chegam depois:** a fila de mídia (MID-005) não avisa a UI quando termina; o carrossel e a capa mostram as
   imagens novas na próxima leitura (trocar de jogo em foco, reconexão ou nova busca).
   · alterado em 2026-11-14: resolvida na fase 10.2 ([phase-10-2-imagens-ao-vivo](../phase-10-2-imagens-ao-vivo/requirements.md),
   IMG-001..IMG-011). A reconexão, citada aqui, não relia a lista de mídia; passa a reler (IMG-010).
3. **Uma leitura por cartão:** a capa de cada cartão carregado continua vindo de um `MediaList` por jogo (como hoje). Em
   bibliotecas grandes, rolar rápido dispara muitas leituras; um método de capas em lote precisaria de mudança no IPC.
4. **Contagem da plataforma sem filtros:** o número ao lado de cada plataforma não muda com os outros filtros (decisão c).
5. **Teclado e leituras:** segurar uma seta na grade troca o jogo em foco a cada cartão e dispara as leituras do painel
   para cada um (o cache evita repetir as já feitas).

## ADRs propostos

A sessão principal escreve (a spec só os referencia). Numeração a partir de 0028.

1. **0028 — Biblioteca em três colunas e fim da página do jogo:** o jogo em foco é estado da UI (não do agente); um
   clique põe em foco, duplo clique e Enter lançam; o painel concentra o que era a página; o jogo em foco não é limpo por
   filtro. Substitui a navegação por página da fase 5 (UI-037).
2. **0029 — Tema por tokens em variáveis CSS:** todas as cores dos componentes vêm de tokens (`--color-*`) definidos por
   tema em `index.css` e expostos ao Tailwind; classes de paleta e cores literais são proibidas nos componentes (UIX-006),
   com teste que garante a regra.

---

## Perguntas ao usuário

A spec já segue a recomendação de cada pergunta. Se a resposta for outra, os requisitos citados mudam antes da aprovação.

1. **Visualização em lista: manter ou remover? (recomendação: manter, sem botões.)** A lista é útil em bibliotecas grandes
   para ver plataforma, tempo jogado e última vez jogado de muitos jogos de uma vez, e já existe (com preferência
   guardada). Com o painel, ela perde os botões e ganha as mesmas regras de seleção da grade; o custo é pequeno.
   *Se remover:* UIX-020 vira "a alternância e `LibraryList` deixam de existir e uma preferência `list` já gravada é
   ignorada (abre na grade)"; UI-030 passa a "substituído"; a barra do centro (UIX-013) perde a alternância.
   *Afeta:* UIX-013, UIX-020, UI-030.
2. **Painel da direita sem jogo em foco: recolhido ou com resumo da biblioteca? (recomendação: resumo.)** Com o painel
   sempre na mesma largura, a grade não muda de número de colunas nem pula de posição quando o primeiro jogo é posto em
   foco (recolher e abrir o painel refaz a virtualização e desloca os cartões debaixo do mouse, o que atrapalha o duplo
   clique). O resumo traz a instrução de uso e os 5 jogados recentemente, que servem de atalho.
   *Se recolhido:* UIX-024 vira "sem jogo em foco o painel não existe e o centro ocupa a largura dele; ao pôr um jogo em
   foco o painel aparece"; UIX-005 ganha o caso de duas colunas; UIX-034 e UIX-023 passam a recolher o painel.
   *Afeta:* UIX-005, UIX-023, UIX-024, UIX-034.
