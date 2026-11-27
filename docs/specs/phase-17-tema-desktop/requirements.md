# Fase 17 — Tema Desktop do Big Box: requisitos

- **Status:** **Requisitos aprovados em 2026-11-27.** As oito perguntas foram resolvidas com as respostas padrão (seção "Decisões do
  usuário" no fim); na aprovação o usuário pediu também que o painel tenha só "Jogar" e "Jogar sem contar tempo" e que nenhum botão da
  barra superior apareça (TDK-002, TDK-037, TDK-038).
- **Pedido (literal):** "ative a navegação no modo desktop pelo controle também, ou adicione um tema do big box igual ao do desktop".
  Decisão da sessão principal: seguir pela **segunda opção**, um tema novo do Big Box chamado **Desktop**.
- **Design:** [design.md](design.md) · **Contratos:** [contracts/bigbox-tema-desktop.md](contracts/bigbox-tema-desktop.md) (layout,
  zonas de foco, tabela de transições do reducer, capacidade, rolagem, tokens, dicas) · **Tarefas:** `tasks.md` (o planner gera depois
  da aprovação).
- **Base:** [phase-15-big-box-v2](../phase-15-big-box-v2/requirements.md) (BBX-046..BBX-059, contrato de tema v2,
  [bigbox-temas-v2](../phase-15-big-box-v2/contracts/bigbox-temas-v2.md)), [phase-13-modo-console](../phase-13-modo-console/requirements.md)
  (BBX-001..BBX-045, [bigbox-faixa](../phase-13-modo-console/contracts/bigbox-faixa.md)), [phase-10-nova-interface](../phase-10-nova-interface/requirements.md)
  (UIX-004..UIX-030, UIX-040..UIX-042: o layout de três colunas que o tema reproduz) e [phase-10-1-ajustes](../phase-10-1-ajustes/requirements.md)
  (AJU-001..AJU-017: cartão, lista, carrossel).
- **ADRs:** 0028 (biblioteca em três colunas), 0029 (tema por tokens), 0036 (Big Box com temas), 0040 (contrato de tema v2). ADR proposto
  no fim.
- **Objetivo:** um tema do Big Box cujo visual reproduz a janela normal (cabeçalho, coluna de plataformas à esquerda, grade ou lista
  de jogos no centro, painel de detalhes do jogo em foco à direita, mesmas cores, tipografia e densidade), navegável **só pelo controle
  e pelo teclado do Big Box**, sem foco do mouse nem Tab. Reaproveita todo o pipeline do Big Box: entrada pela Gamepad API na UI
  (`useBigBoxInput`), combos do agente (Select + Y/B/A), reducer `bigBoxNav`, modelo de exibição, leitura por páginas, dicas e rodapé.
  **Nada muda no agente, no IPC, no banco nem no tracker. A UI nunca influencia o tempo de jogo.**

Formato: IDs `ÁREA-NNN`, estáveis e nunca reutilizados; critérios Dado/Quando/Então. Áreas: **BBX** continua com as mudanças comuns
aos temas (a partir de BBX-060) e **TDK** é nova (tema Desktop; conferida com `grep` em `docs/` e no código, sem colisão; `DSK` já é do
desktop). Todo ID ativo precisa de um teste que o cite no nome (`it("TDK-013 ...")`), exceto os marcados `· verificação: manual`. Nenhum
teste usa rede nem espera real (`vi.useFakeTimers`, `navigator.getGamepads` falso, bindings falsos).

Vocabulário desta spec:
- **Janela normal / desktop:** a biblioteca de três colunas fora do Big Box.
- **Zona:** a coluna que tem a marca de foco do controle: **Plataformas** (esquerda), **Jogos** (centro) ou **Painel** (direita). Na
  linguagem do reducer, Plataformas e Jogos são a tela Navegar (`browse`) e o Painel é a tela Detalhes (`details`).
- **Jogo em foco:** o jogo da posição `focus` da aba ativa; o painel mostra sempre ele.
- **`C`** colunas da grade (1 na lista), **`R`** fileiras inteiras visíveis, **`T`** total da aba, **`L`** jogos lidos,
  **`M = min(T, L) − 1`**, **`f`** o foco, **`r = ⌊f/C⌋`** a fileira do foco.

Correção de premissa registrada: a navegação do Big Box **não** vem do XInput do agente; o agente lê o XInput só para os combos com
Select (pausa, fechar o jogo, abrir o Big Box). A navegação é lida pela própria UI com a Gamepad API (`navigator.getGamepads`, BBX-019,
[bigbox-faixa §2](../phase-13-modo-console/contracts/bigbox-faixa.md)). Também: **Select + B fecha o jogo** (CON-001), não sai do Big
Box; quem sai é **B** (BBX-002).

---

## Decisões do spec-writer (aprovadas em 2026-11-27)

a. **Tema em colunas, não tela a tela.** O painel da direita fica sempre visível (como na janela normal) e "abrir Detalhes" vira
   "levar o foco para o painel"; a tela Detalhes do reducer continua existindo, só que desenhada dentro da coluna direita. Para isso o
   contrato de tema ganha o layout `columns` (BBX-061) e o shell monta os Detalhes também em Navegar, sem marca de foco (BBX-064).
b. **Três zonas com navegação espacial:** ← na primeira coluna da grade vai para Plataformas; → na última coluna (ou no último jogo)
   vai para o Painel; ← num botão do Painel volta para Jogos. Na grade do Desktop, ←/→ **não passam de fileira** (como o teclado da
   janela normal, UIX-019), diferente da Estante (BBX-048).
c. **↑/↓ em Plataformas trocam a plataforma na hora**, como o clique na janela normal (UIX-010), sem volta nas pontas; LB/RB continuam
   trocando de plataforma (circular) em qualquer zona de Navegar.
d. **B volta um nível:** no Painel e em Plataformas, B leva a Jogos; em Jogos, B sai do Big Box (BBX-002 continua valendo ali).
e. **Painel empilhado:** o foco do Painel anda com ↑/↓ na ordem visual do desktop (visor de imagens em cima, depois "Jogar", depois
   "Jogar sem contar tempo"); ←/→ trocam a imagem só com o foco no visor. Os outros temas mantêm o foco em linha (BBX-032, BBX-033).
f. **Mesmos tokens do desktop por cópia conferida:** o bloco `[data-bigbox-theme="desktop"]` define os 15 tokens `--arc-bb-*` com os
   mesmos valores dos tokens `--arc-*` do tema escuro (tabela em [contrato §7](contracts/bigbox-tema-desktop.md)), e um teste confere a
   igualdade. Assim valem BBX-025 e BBX-058 (o Big Box só usa `--arc-bb-*`) e o visual não diverge do desktop.
g. **Componentes próprios, medidas compartilhadas:** o tema não importa `components/` (a varredura de BBX-003 continua proibindo, e os
   componentes do desktop trazem botões de escrita, foco do DOM e virtualização próprios). Ele redesenha a marcação do desktop com as
   **mesmas constantes** (largura das colunas, cartão, vão, altura da linha da lista), que passam a ser lidas de um só lugar (TDK-034).
   As adaptações mínimas fora de `bigbox/` são só mover helpers puros para módulos de leitura (design §6).
h. **Rolagem pelas bordas:** a grade e a lista só rolam quando o foco sairia da área visível (como `scrollIntoView` do desktop), por
   fileiras inteiras, em vez de pôr a fileira do foco no topo (Estante). Na entrada, a fileira do foco fica no meio (como UIX-021).
i. **Grade e lista e ordem herdadas da janela normal:** a visualização (grade/lista, UI-030) e a ordenação atual (UI-034) da janela
   normal valem no tema, sem as divisórias por ano. Os outros filtros e a busca da janela normal não se aplicam (como nos outros temas).
j. **Sem área segura:** o Desktop ocupa a janela inteira como a janela normal (`--arc-bb-safe-*` = 0 no bloco dele). Quem usa TV com
   overscan escolhe outro tema ou o "ajustar à tela" da TV (Limitações 1).
k. **Escala de texto 1:1 com o desktop** (mesmos tamanhos de fonte e densidade). A legibilidade a 3 m não é exigida deste tema (os
   outros três continuam para isso); decisão 1 do usuário.
l. **O que é escrita fica fora:** busca, filtros, ordenar, alternar grade/lista, botões do cabeçalho, "Editar", "Buscar metadados",
   "Buscar vídeo", "Abrir local do arquivo", "Remover", a lista de sessões (e a edição da fase 16), o painel de sessão (pausar/retomar),
   modais, importação e estatísticas. Continuam só na janela normal (BBX-003; TDK-030). Pedido do usuário na aprovação: no painel, os
   **únicos** botões são "Jogar" e "Jogar sem contar tempo" (TDK-037), o cabeçalho não tem nenhum controle (TDK-002) e um teste varre o
   DOM do tema inteiro atrás de qualquer controle fora da lista permitida (TDK-038).

---

## Requisitos de fases anteriores afetados

Cada um recebeu, na spec de origem, a anotação `· alterado em 2026-11-27 (docs/specs/phase-17-tema-desktop, ID)`.

| Requisito | Fase | Situação | Por |
|---|---|---|---|
| BBX-002 — Sair do Big Box | 13 | alterado: no tema Desktop, B só sai a partir da zona Jogos; em Plataformas, B volta a Jogos | TDK-015 |
| BBX-004 — Abas de plataforma | 13 | alterado: no layout `columns` a aba de plataforma mostra ícone e contagem; nos outros temas, só texto como hoje | BBX-066 |
| BBX-015 — Rodapé com dicas | 13 | alterado: no tema Desktop as dicas seguem a zona | TDK-024 |
| BBX-016, BBX-032, BBX-033 — Detalhes e foco | 13 | alterados: no layout `columns` a tela Detalhes é o Painel da direita, com foco empilhado (↑/↓) | TDK-018 |
| BBX-042 — Área segura | 13 | alterado: a área segura vem dos tokens do tema; o Desktop usa 0 | BBX-069 |
| BBX-046 — Temas disponíveis | 15 | alterado: `desktop` ("Desktop") entra disponível, depois de `estante` e antes de `roda` | BBX-060 |
| BBX-047 — Contrato de tema v2 | 15 | alterado: campos novos opcionais (layout, colunas e fileiras por janela e visualização, limite de nós, rolagem, vídeo no painel, ordem herdada) | BBX-061 |
| BBX-048 — Grade 2D no reducer | 15 | alterado: no layout `columns`, ←/→ não passam de fileira e levam às zonas vizinhas nas pontas | TDK-013 |
| BBX-049, BBX-050 — Janela e páginas da grade | 15 | alterados: o limite de nós é do tema (30 por padrão; Desktop 200) | BBX-062 |
| BBX-053 — Vídeo na capa em foco | 15 | alterado: o lugar `panel` toca o vídeo no visor do painel | TDK-010 |
| BBX-055 — Ordem por tema | 15 | alterado: a ordem `inherit` usa a ordenação da janela normal | BBX-067 |
| BBX-056 — Detalhes próprios | 15 | alterado: no layout `columns` o componente de Detalhes é montado também em Navegar, sem marca de foco | BBX-064 |

Inalterados e ainda válidos para todos os temas, inclusive o Desktop: BBX-001, BBX-003, BBX-005, BBX-007 (faixas), BBX-008, BBX-011..BBX-014,
BBX-017..BBX-022, BBX-025..BBX-029, BBX-034..BBX-041, BBX-043..BBX-045 (faixas), BBX-051, BBX-052, BBX-054, BBX-057..BBX-059; ATB-*,
CON-*, TTB-*. Faixa, Mosaico e Estante não mudam de comportamento (os campos novos têm padrão igual ao de hoje, BBX-061).

---

## BBX — Big Box: mudanças comuns aos temas

### BBX-060 — Tema Desktop no registro e no seletor
Dado o registro de temas, então existem, nesta ordem, `faixa`, `mosaico`, `estante` e `desktop` ("Desktop"), disponíveis, e `roda`,
indisponível. O seletor "Tema do Big Box" do cabeçalho lista os cinco pelo nome do registro; escolher "Desktop" grava `desktop` em
`arcadia.bigBoxTheme` e a próxima entrada no Big Box (botão, F11, `--bigbox` ou Select + A) usa o tema Desktop. Valor ausente,
desconhecido ou indisponível continua virando `faixa` (BBX-023).
· verificação: teste do registro, de `uiPrefs` (`desktop` lido e gravado) e de componente do cabeçalho (opção "Desktop" ativa).

### BBX-061 — Campos novos do contrato de tema
Dado um tema, então ele pode declarar, além dos campos de BBX-047: `layout` (`screens` ou `columns`), `gridColumns(janela,
visualização)` e `gridRows(janela, visualização)` (colunas e fileiras da grade calculadas na hora), `maxNodes` (limite de cartões no
DOM), `gridScroll` (`focusTop` ou `edges`), o lugar de vídeo `panel` e a ordem `inherit`. Ausentes, valem `screens`, `columns` e
`gridRows` fixos do tema, 30, `focusTop`, e o que o tema já declara; Faixa, Mosaico e Estante não declaram nenhum deles e navegam e
desenham como hoje.
· verificação: teste do registro (padrões; Desktop com `columns`, `edges`, `panel`, `inherit`, `maxNodes = 200`) e os testes atuais
dos três temas passando sem mudança.

### BBX-062 — Limite de nós da grade por tema
Dado um tema de grade com `C` colunas, `R` fileiras inteiras e limite `N` (BBX-061), então o número de fileiras inteiras usado na
janela e nas páginas é `R' = clamp(R, 1, max(⌊N/C⌋ − 1, 1))`; a janela desenha no máximo `(R' + 1) · C ≤ N` cartões e a página e a
pré-busca de BBX-050 usam `R'`. Com `N = 30` e `C = 5` (Estante), `R' = clamp(R, 1, 5)`, igual a hoje.
· verificação: teste das funções puras (`gridWindow`, `gridPageSize`, `shouldPrefetchGrid`) com os vetores de
[contrato §4](contracts/bigbox-tema-desktop.md) e os vetores de BBX-049/BBX-050 inalterados.

### BBX-063 — Rolagem pelas bordas
Dado um tema com `gridScroll = edges`, a fileira inicial desenhada `s`, `R'` fileiras inteiras e `rows = ⌈T/C⌉`, quando o foco muda para
a fileira `r`, então `s` passa a `r` se `r < s`, a `r − R' + 1` se `r > s + R' − 1`, e fica igual nos outros casos, sempre limitada a
`[0, max(rows − R', 0)]`. Na entrada no Big Box, na troca de aba e depois de `gameClosed`, `s = clamp(r − ⌊(R' − 1)/2⌋, 0,
max(rows − R', 0))` (a fileira do foco no meio). Com `focusTop`, vale BBX-049 como está.
· verificação: teste da função pura (`edgeScrollRow`, `centeredRow`) com os vetores do contrato §5 e teste do shell (andar para baixo
só rola ao passar da última fileira inteira).

### BBX-064 — Detalhes montados também em Navegar no layout `columns`
Dado um tema com `layout = columns` e um jogo em foco com detalhe lido (`GetGame`), então o shell monta o componente de Detalhes do
tema também na tela Navegar, com `active = false` (nenhuma marca de foco, nenhum vídeo da galeria tocando, nenhuma reação à rolagem);
na tela Detalhes, com `active = true` e as regras de BBX-032..BBX-036 ajustadas por TDK-018. Nenhuma leitura nova é feita: o detalhe, a
mídia e a sessão já são lidos para o jogo em foco (`useFocusedData`). Com `layout = screens`, vale BBX-056 como está.
· verificação: teste do shell com um tema falso `columns` (montado em Navegar com `active = false`; mesmos bindings chamados que num
tema `screens`).

### BBX-065 — Clique numa aba
Dado um tema que desenha as abas como itens clicáveis, quando o usuário clica na aba de índice `i` diferente da ativa, então a aba `i`
fica ativa, o foco vai ao primeiro jogo dela e a tela volta a Navegar (se estava em Detalhes); clicar na aba ativa não faz nada. O
reducer recebe a ação `tabSet {index}`; o tema só a repassa por `onTab(i)`.
· verificação: teste do reducer (`tabSet`: índice igual, diferente, fora da faixa, a partir de Detalhes) e de componente do Desktop.

### BBX-066 — Ícone e contagem nas abas de plataforma
Dado o catálogo, então cada aba de plataforma do modelo traz o ícone da plataforma (mesma regra de PLT-009..PLT-011: personalizado,
embutido pela chave ou genérico) e o número de jogos (`gameCount`), e "Todos" traz a soma; só temas com `layout = columns` os
desenham. Faixa, Mosaico e Estante continuam só com texto (BBX-004). O ícone vem de um módulo só de leitura (sem as funções de escrita
de plataformas, BBX-003).
· verificação: teste de `buildTabs` (ícone e contagem por aba; soma em "Todos") e a varredura de BBX-003 sem exceção nova.

### BBX-067 — Ordem herdada da janela normal
Dado um tema com `sort = inherit`, quando o Big Box é aberto com a janela normal ordenada por `k` (`title`, `lastPlayed`,
`totalPlayed` ou `year`, UI-034), então toda leitura `ListGames` das abas usa `sort = k` e o rótulo da ordem no modelo é o do seletor
da janela normal ("Título", "Última vez jogado", "Tempo jogado", "Ano"). Mudar a ordem na janela normal só vale na próxima entrada.
Temas com `title` ou `lastPlayed` seguem BBX-055.
· verificação: teste do hook de dados (parâmetro `sort` enviado) e do modelo (rótulo).

### BBX-068 — Visualização herdada da janela normal
Dado o Big Box aberto com a janela normal em grade ou em lista (UI-030), então o tema recebe essa visualização e a usa em
`gridColumns`/`gridRows` (na lista, `C = 1`); a preferência não é gravada nem mudada pelo Big Box. Temas que não declaram essas funções
ignoram a visualização.
· verificação: teste de componente do `App` (a visualização chega ao Big Box) e do shell (colunas 1 na lista).

### BBX-069 — Área segura por tema
Dado o tema ativo, então o recuo da área segura (`data-bb-safe`) vem dos tokens `--arc-bb-safe-y` e `--arc-bb-safe-x` do bloco do tema:
Faixa, Mosaico e Estante `5vh`/`5vw` (sem mudança), Desktop `0`/`0`.
· verificação: teste que lê os blocos de `index.css` (valores por tema).

### BBX-070 — Varreduras continuam valendo
Dado o código novo em `frontend/src/bigbox/`, então as varreduras de BBX-003, BBX-025, BBX-029, BBX-037, BBX-047 e BBX-059 passam sem
nenhuma exceção nova: nenhum import de `components/`; temas sem import de `lib/`; nenhuma cor literal ou classe de paleta; nenhum
binding de escrita; o tempo para zerar só pelo modelo.
· verificação: `bigbox-scan.test.ts` (sanidade: os arquivos de `themes/desktop/` entram na varredura).

---

## TDK — Tema Desktop

Layout, zonas, transições, capacidade, rolagem, tokens e dicas em [contracts/bigbox-tema-desktop.md](contracts/bigbox-tema-desktop.md).

### Estrutura e visual

### TDK-001 — Três colunas como a janela normal
Dado o Big Box no tema Desktop, então a tela tem, de cima para baixo: o cabeçalho (TDK-002); as três colunas lado a lado, com a
esquerda (`<nav>` "Plataformas") de largura `clamp(220px, 16vw, 300px)`, a direita (`<aside>` "Detalhes do jogo") de
`clamp(320px, 26vw, 480px)` e o centro (região "Jogos") com o resto (UIX-004, UIX-005); e o rodapé de dicas (BBX-015, TDK-024) na
borda de baixo. O fundo é liso (`plain`, BBX-052), sem arte de fundo nem véu, e a área segura é 0 (BBX-069). Nada da tela rola como
documento.
· verificação: teste de componente (ordem e nomes dos marcos; larguras pelas variáveis `--col-left`/`--col-right`; sem `bb-art`).

### TDK-002 — Cabeçalho só com o nome
· alterado em 2026-11-27 (na aprovação, pedido do usuário): explicitado que o cabeçalho não tem nenhum controle e que os rótulos da
barra superior não aparecem como botão em nenhum lugar do tema.
Dado o tema Desktop, então o cabeçalho (`<header>` do tema) mostra só o título "Arcadia", com a mesma tipografia e espaçamento da
janela normal, e não contém nenhum elemento acionável (`button`, `select`, `input`, `a[href]`, `[role="button"]`, `[role="combobox"]`).
Nenhum dos controles da barra superior da janela normal existe no tema, nem no cabeçalho nem fora dele: o botão "Big Box", o seletor
"Tema do Big Box", os botões de adicionar jogos ("Importar"), "Provedores", "Emuladores", "Plataformas" e "Estatísticas", o indicador
do lote de metadados (e as ações dele: revisar, cancelar, descartar) e o indicador do agente. O título "Plataformas" da coluna
esquerda (TDK-003) é texto, não botão.
· verificação: teste de componente (texto "Arcadia" presente; zero elementos acionáveis dentro do `<header>`; `queryByRole('button',
{ name })` e `queryByRole('combobox', { name })` nulos para cada rótulo listado, em todo o DOM do tema).

### TDK-003 — Coluna de plataformas
Dado o catálogo, então a coluna esquerda mostra o título "Plataformas" e a lista "Todos" (com a soma dos jogos) seguida de cada
plataforma com jogos, na ordem do catálogo, cada item com ícone, nome e número de jogos (BBX-066, como UIX-009). O item da aba ativa tem
`aria-current="true"` e o destaque do item atual da janela normal (fundo `--arc-bb-surface`, texto `--arc-bb-text`); os outros, texto
`--arc-bb-text-muted`. Não há campo de busca nem seção "Filtros".
· verificação: teste de componente com catálogo falso (itens, contagens, item atual, ausência de busca e filtros).

### TDK-004 — Barra do centro
Dado a coluna do centro, então no topo há uma barra com "`T` jogos" (com `T ≥ 1`; "1 jogo" com `T = 1`) à esquerda e, à direita, os
textos "Ordenar por: `<rótulo>`" (BBX-067) e "Grade" ou "Lista" (BBX-068), só de leitura (sem `<select>` nem botão).
· verificação: teste de componente (textos; nenhum controle de formulário na barra).

### TDK-005 — Cartões da grade como os do desktop
Dado a visualização em grade, então cada cartão mostra a capa numa área 3:4 (imagem inteira, ou a substituta de BBX-008, ou o
placeholder padrão da janela normal), o título em até duas linhas (cortado com "…", completo no `title`) e, se já foi jogado, "Jogado
DD/MM/AAAA HH:MM" (UIX-014, AJU-015); com sessão aberta do jogo, o selo "Em execução" ou "Pausado" sobre a capa (BBX-017). O número de
colunas segue a largura do centro pela regra da janela normal (TDK-007). O cartão do jogo em foco tem o destaque de seleção (TDK-023).
· verificação: teste de componente (partes do cartão; selo; placeholder).

### TDK-006 — Linhas da lista como as do desktop
Dado a visualização em lista, então cada linha (64 px de altura) mostra título, plataforma, tempo jogado (UI-005), última vez jogado
(data e hora locais, ou "Nunca") e a marcação de sessão, sem botões (UIX-020); a linha do jogo em foco tem o destaque de seleção.
· verificação: teste de componente.

### TDK-007 — Colunas e fileiras pela janela
Dado o tamanho da janela `W × H` e a visualização, então `desktopGrid` (pura) devolve `{columns, rows}`: o centro útil é
`W − clamp(220, 0,16·W, 300) − clamp(320, 0,26·W, 480)` menos os recuos da janela normal, as colunas e a altura da fileira são as de
`columnsFor` da janela normal (lista: 1 coluna de 64 px), e `rows` são as fileiras inteiras que cabem na altura menos o cabeçalho, a
barra, o rodapé e os recuos (fórmula e constantes no [contrato §3](contracts/bigbox-tema-desktop.md)). Vetores: 1920×1080 grade → 6 × 2;
1280×720 grade → 3 × 1; 1920×1080 lista → 1 × 13; tamanho desconhecido → 4 × 2 (lista 1 × 8).
· verificação: teste da função pura (vetores do contrato).

### TDK-008 — Painel da direita
· alterado em 2026-11-27 (na aprovação, pedido do usuário): remete a TDK-037 (só "Jogar" e "Jogar sem contar tempo").
Dado o jogo em foco `G` com detalhe lido, então a coluna direita mostra, de cima para baixo, como a janela normal (UIX-025..UIX-029):
o título, o ícone e o nome da plataforma e a marcação de sessão; o visor de imagens (TDK-009); o botão **Jogar** (largo, `--arc-bb-accent`)
e, logo abaixo, **Jogar sem contar tempo**; os campos "Tempo total", "Primeira vez jogado", "Última vez jogado", "Tempo para zerar"
(três valores e a fonte, TTB-014, TTB-039), "Concluído em" (quando houver), "Status", "Marcação: Favorito" (quando favorito), "Gêneros",
"Modos de jogo", "Desenvolvedora", "Editora" e "Ano" ("—" para vazio); e a "Descrição" inteira. Ao trocar de jogo, o painel volta ao
topo (UIX-036). Os únicos botões do painel são os dois de lançar (TDK-037); os de escrita ficam fora (TDK-030).
· verificação: teste de componente com detalhe de fixture (campos, rótulos, "—", volta ao topo).

### TDK-009 — Visor de imagens do painel
Dado `G` com galeria (vídeo e screenshots, na ordem de BBX-016), então o visor 16:9 mostra o item selecionado inteiro, com o rótulo do
tipo ("Vídeo" ou "Screenshot") e "`k` de `n`", e, com `n ≥ 2`, a fileira de miniaturas embaixo com a selecionada marcada. Em Navegar o
item mostrado é a capa de `G` (ou a substituta) com o vídeo de TDK-010 por cima; sem galeria, o visor mostra a capa (ou o placeholder)
e não recebe foco.
· verificação: teste de componente (n = 0, 1, 3; rótulo e contagem; miniaturas).

### TDK-010 — Vídeo no visor depois de 2 s parado
Dado o tema Desktop (vídeo `panel`) na tela Navegar com o jogo em foco com vídeo, quando o foco fica 2.000 ms sem mudar, então o vídeo
toca dentro do visor do painel com as regras de BBX-011..BBX-013 (um único `<video>` na tela, `src` removido ao trocar de foco, sem
nova tentativa depois de `error`, Y liga e desliga o som); o fundo nunca toca vídeo. Na tela Detalhes (foco no Painel), o vídeo toca só
quando é o item selecionado do visor, como na galeria de BBX-034, e o `<video>` de Navegar não existe.
· verificação: teste de componente com temporizadores falsos (no máximo um `<video>`, dentro do visor).

### TDK-011 — Tempo para zerar no painel em Navegar
Dado o tema Desktop, então o painel mostra o tempo para zerar também em Navegar (`timeToBeatInBrowse`): o cache é lido na hora e a
busca é pedida depois de 1.000 ms com o mesmo jogo em foco; ao entrar no Painel (tela Detalhes), na hora (BBX-059). Nenhum botão de
atualizar ou confirmar aparece (TTB-016).
· verificação: teste de componente com temporizadores falsos e espião de `TimeToBeatGet`.

### Navegação

### TDK-012 — Zonas de foco
Dado o tema Desktop, então a marca de foco do controle está sempre em exatamente uma zona: **Jogos** (o jogo em foco), **Plataformas**
(o item da aba ativa) ou **Painel** (um botão ou o visor). A entrada no Big Box começa em Jogos, com o foco no jogo em foco da janela
normal se ele estiver na aba inicial (BBX-027), e o Painel mostra sempre o jogo em foco, em qualquer zona.
· verificação: teste do reducer (zona inicial; `listLoaded` e `gameClosed` voltam a Jogos) e de componente (uma marca só).

### TDK-013 — Zona Jogos na grade
Dado a zona Jogos na grade com `C` colunas, então: ↑/↓ andam uma fileira pela regra de BBX-048 (fileira parcial vai ao último;
nunca entra em jogo não lido); ← move o foco ao jogo anterior **na mesma fileira** e, na primeira coluna, leva o foco para
Plataformas; → move ao jogo seguinte na mesma fileira e, na última coluna ou no último jogo lido, leva o foco para o Painel (tela
Detalhes, foco em "Jogar") se houver jogo em foco. Com a aba vazia, ← leva a Plataformas e → não faz nada.
· verificação: teste do reducer com a tabela e os vetores do [contrato §2](contracts/bigbox-tema-desktop.md).

### TDK-014 — Zona Jogos na lista
Dado a zona Jogos na lista (`C = 1`), então ↑/↓ movem o foco uma linha (sem volta nas pontas, nunca para jogo não lido), ← leva a
Plataformas e → leva ao Painel (com jogo em foco).
· verificação: teste do reducer (`columns = 1`).

### TDK-015 — Zona Plataformas
Dado a zona Plataformas com a aba `t` ativa, então ↑ ativa a aba `t − 1` e ↓ a aba `t + 1` (sem volta nas pontas), cada troca lendo a
lista da aba nova com o foco no primeiro jogo (BBX-005, BBX-038); →, A e B levam o foco a Jogos sem trocar de aba; ← não faz nada.
Nenhuma dessas ações lança jogo nem sai do Big Box.
· verificação: teste do reducer (pontas, trocas, A/B/→ voltam a Jogos sem efeito).

### TDK-016 — LB/RB e X em qualquer zona de Navegar
Dado a tela Navegar em Jogos ou Plataformas, então LB/RB trocam de aba como BBX-005 (circular, foco no primeiro jogo) sem mudar a zona,
e X (D) leva o foco ao Painel com o foco em "Jogar" quando há jogo em foco.
· verificação: teste do reducer.

### TDK-017 — A, B e Y na zona Jogos
Dado a zona Jogos com um jogo em foco, então A (Enter) chama `LaunchGame` uma vez (BBX-014; tecla mantida não relança); B (Esc) sai do
Big Box e a janela normal reaparece como estava (BBX-002); Y (M) liga e desliga o som do vídeo visível (BBX-013).
· verificação: teste do shell com controle falso e espiões.

### TDK-018 — Zona Painel (Detalhes empilhados)
Dado o foco no Painel, então ele começa em **Jogar**; ↑/↓ andam na ordem visual: visor (só com galeria) → Jogar → Jogar sem contar
tempo, sem volta; ←/→ com o foco no visor trocam o item (sem volta, BBX-034); ← com o foco num botão leva a Jogos (mesmo jogo em foco) e →
num botão não faz nada; A aciona o botão focado (`LaunchGame` ou `LaunchGameUntracked`) e, no visor, nada; B (Esc) volta a Jogos com o foco
no mesmo jogo; o analógico direito e PageUp/PageDown rolam o painel sem mover o foco (BBX-036). Se a galeria some com o foco no visor,
o foco vai para Jogar.
· verificação: teste do reducer com a tabela do contrato §2 (layout `columns`) e de componente (uma marca de foco).

### TDK-019 — Rolagem da grade e da lista
Dado a zona Jogos, então a grade e a lista desenham a partir da fileira `s` de BBX-063: andar dentro das fileiras inteiras visíveis não
rola; passar da última (ou da primeira) rola uma fileira; o jogo em foco nunca fica fora da área visível nem cortado. Na entrada, a
fileira do foco aparece no meio. No máximo `(R' + 1) · C` cartões ficam no DOM (BBX-062, `N = 200`), a última fileira desenhada pode
aparecer cortada embaixo e não há barra de rolagem.
· verificação: teste de componente (índices desenhados ao andar; contagem de nós) e das funções puras de BBX-063.

### TDK-020 — Pré-busca na grade do Desktop
Dado a grade ou a lista do Desktop, então a leitura por páginas e a pré-busca são as de BBX-050 com `R'` de BBX-062: uma leitura em voo
por aba, sem repetir nem cancelar ao mover o foco, troca de aba cancela; ↓ com a página seguinte pendente não passa de `M`.
· verificação: teste do shell com `ListGames` controlado por promessas.

### TDK-021 — Plataforma ativa sempre visível
Dado mais plataformas do que cabem na coluna esquerda, quando a aba ativa muda (↑/↓, LB/RB ou clique), então a coluna rola até o item
ativo ficar inteiro à vista.
· verificação: teste de componente (`scrollIntoView` chamado com `block: 'nearest'` no item ativo; ausente no jsdom tolerado).

### TDK-022 — Item focado do painel sempre visível
Dado o foco no Painel, quando ele muda de elemento, então o painel rola até o elemento focado ficar à vista; a rolagem pelo analógico
direito não move o foco.
· verificação: teste de componente.

### TDK-023 — Marca de foco e destaque de seleção
Dado o tema Desktop, então exatamente um elemento da tela tem `data-focused="true"` e o contorno de foco da janela normal
(`outline: 2px solid var(--arc-bb-focus)`, afastado 2 px, UIX-040): o cartão ou a linha do jogo em foco (zona Jogos), o item da aba ativa
(zona Plataformas) ou o botão ou o visor (zona Painel). Fora da zona Jogos, o cartão ou a linha do jogo em foco mantém o destaque de
seleção (fundo `--arc-bb-surface` e anel `--arc-bb-text-muted`), sem o contorno.
· verificação: teste de componente (uma marca por zona; destaque de seleção fora da zona Jogos).

### TDK-024 — Dicas do rodapé por zona
Dado o tema Desktop, então o rodapé mostra: em Jogos, as dicas de BBX-015 ("A Jogar", "X Detalhes", "LB/RB Plataforma", "Select+B
Fechar o jogo", "B Sair" e "Y Som" com vídeo visível); em Plataformas, "↑/↓ Plataforma", "A Ver jogos", "LB/RB Plataforma", "B Voltar" e
"Select+B Fechar o jogo"; no Painel com o foco num botão, "A Jogar" ou "A Jogar sem contar tempo", "↑/↓ Mover", "B Voltar" e "Select+B
Fechar o jogo"; no visor, "←/→ Imagem", "↓ Botões", "B Voltar" e "Select+B Fechar o jogo"; com a aba vazia, "LB/RB Plataforma",
"← Plataformas" e "B Sair". Nenhuma dica de Select + A (ATB-013).
· verificação: teste de `hintsFor` (um caso por zona e foco).

### TDK-025 — Mouse no tema Desktop
Dado o tema Desktop, quando clico num cartão ou numa linha, então o jogo fica em foco e a zona passa a Jogos; duplo clique lança
(BBX-014); clicar numa plataforma a ativa (BBX-065); "Jogar" e "Jogar sem contar tempo" do painel lançam como A. Um lançamento pendente
bloqueia novo lançamento (UI-021).
· verificação: teste de componente com espiões.

### TDK-026 — Estados da tela
Dado o tema Desktop, então: com a primeira página da aba ainda não lida, o centro mostra "Carregando jogos…" e o painel "Nenhum jogo
selecionado"; com o jogo em foco sem detalhe lido, o painel mostra "Carregando jogo…"; com a aba vazia, "Nenhum jogo nesta plataforma"
(ou "Nenhum jogo na biblioteca") e o painel "Nenhum jogo selecionado"; um erro (BBX-020) aparece numa faixa `role="alert"` logo abaixo do
cabeçalho, como o aviso de erro da janela normal, até a próxima ação.
· verificação: teste de componente.

### Tokens, acessibilidade e limites

### TDK-027 — Tokens iguais aos do desktop
Dado `index.css`, então o bloco `[data-bigbox-theme="desktop"]` define os 15 tokens de BBX-058 e os dois de área segura (BBX-069), e cada
token de cor tem exatamente o valor do token do tema escuro da tabela do [contrato §7](contracts/bigbox-tema-desktop.md) (por exemplo
`--arc-bb-panel` = `--arc-surface`, `--arc-bb-accent` = `--arc-play`).
· verificação: teste que lê os dois blocos e compara os pares da tabela.

### TDK-028 — Contraste do Desktop
Dado o bloco do Desktop, então `text`, `text-muted`, `heading` e `danger` têm contraste ≥ 4,5:1 com `panel`, `bg` e `surface`;
`on-accent` sobre `accent` e `on-chip` sobre `chip` ≥ 4,5:1; `focus` ≥ 3:1 contra `panel`, `bg` e `surface`.
· verificação: teste de tokens (como BBX-058).

### TDK-029 — Nomes e estados acessíveis
Dado o tema Desktop, então: a coluna esquerda é `<nav aria-label="Plataformas">` com itens `aria-current` na aba ativa e nome
"`<plataforma>`, `N` jogos"; o centro é `role="listbox" aria-label="Jogos"` com cada cartão ou linha `role="option"`, `aria-label` com o
título e `aria-selected` no jogo em foco; a direita é `<aside aria-label="Detalhes do jogo">`; o visor é uma região "Imagens" com
"`k` de `n`" em `aria-live="polite"`; os botões têm nome em texto; o selo de sessão é texto. O Big Box continua sem mover o foco do DOM
(a entrada é global, BBX-019) e o rodapé diz em texto o que cada botão faz.
· verificação: teste de componente (papéis, nomes e estados).

### TDK-030 — O que fica fora do tema
· alterado em 2026-11-27 (na aprovação, pedido do usuário): a lista ganha "Tentar de novo", os botões do tempo para zerar, "Adicionar
sessão" e o fechar do aviso de erro; os botões do cabeçalho remetem a TDK-002 e a lista permitida a TDK-037 e TDK-038.
Dado o tema Desktop (Navegar e Painel), então **não** aparecem nem são alcançáveis: campo de busca, seção "Filtros", "Limpar filtros",
seletor de ordenação, alternância grade/lista, botões do cabeçalho da janela normal (TDK-002), "Editar", "Buscar metadados", "Buscar
vídeo", "Abrir local do arquivo", "Remover", "Tentar de novo", os botões do tempo para zerar ("Confirmar…", "Escolher outro…",
"Buscar", "Nenhum destes", TTB-016), a seção "Sessões" (lista, "Adicionar sessão", "Carregar mais", editar e excluir sessões da fase
16), o painel de sessão (pausar/retomar), "Ampliar" e a janela de imagem ampliada, o resumo "Jogados recentemente" do painel vazio,
avisos "Sem emulador configurado" e "Rastreio por processo", divisórias por ano, botão de fechar no aviso de erro (TDK-026), nenhuma
modal, importação nem estatísticas.
· verificação: teste de componente (nenhum desses textos ou controles no DOM) e a varredura de BBX-003.

### TDK-031 — Só leituras e lançamento
Dado qualquer navegação no tema Desktop (zonas, abas, foco, painel, visor, som, rolagem), então só são chamados `ListGames`, `GetGame`,
`MediaList`, `Catalog`, `CurrentSession`, `TimeToBeatGet`, `LaunchGame`, `LaunchGameUntracked` e `BringToFront` (BBX-003, BBX-029,
BBX-059); nenhum download de imagem ou vídeo é pedido.
· verificação: teste de componente com espiões em todos os bindings.

### TDK-032 — A UI nunca influencia o tempo de jogo
Dado qualquer caminho do tema Desktop, então nenhuma sessão é pausada, retomada ou encerrada pela UI e nenhum jogo é fechado; o teste de
invariantes da UI (`App.invariants.test.tsx`) ganha a navegação do tema Desktop.
· verificação: `App.invariants.test.tsx`.

### TDK-033 — Suspensão, Select segurado e jogo fechado pelo controle
Dado o tema Desktop, então valem como nos outros temas: a entrada suspensa enquanto o jogo lançado roda (BBX-018); Select segurado
bloqueia A, B, Y e ↑/↓ do direcional na UI (BBX-057); e, depois de Select + B fechar o jogo, o Big Box volta ao primeiro plano com a zona
Jogos e o foco no jogo que rodava, com a fileira dele no meio (CON-022, BBX-063).
· verificação: teste do shell com o tema Desktop (eventos de janela e `game:closedByHotkey` simulados; `getGamepads` falso).

### TDK-034 — Mesmas medidas da janela normal
Dado o tema Desktop e a janela normal, então as medidas de layout vêm das mesmas constantes: larguras das colunas (`--col-left`,
`--col-right`), largura mínima do cartão, vão, altura do título do cartão, recuo do contorno, largura da barra de rolagem descontada e
altura da linha da lista; o tema não redefine nenhuma delas. Mudar uma constante na janela normal muda o tema junto.
· verificação: teste que confere que o módulo de layout do Big Box reexporta os valores de `lib/gridNav` (mesma referência) e que o
tema usa as variáveis CSS de UIX-005.

### TDK-035 — Desktop com controle no monitor e na TV
· alterado em 2026-11-27 (na aprovação, pedido do usuário): a checagem manual inclui o painel só com os dois botões e o cabeçalho sem
botões.
Dado o Arcadia com um controle XInput, no monitor e na TV (inclusive pelo Moonlight/Sunshine), quando escolho o tema Desktop e entro no
Big Box (botão, F11 e Select + A), então vejo a mesma biblioteca da janela normal; ando na grade e na lista com o direcional e o
analógico, vou para as plataformas com ← e para o painel com →, troco de plataforma com ↑/↓ e LB/RB, X e A no painel lançam com e sem
contar tempo, o vídeo entra no visor depois de ~2 s, B volta um nível e sai em Jogos, e Select + B fecha o jogo e me devolve ao mesmo
jogo; nenhum botão de edição aparece, o painel só tem "Jogar" e "Jogar sem contar tempo" e o cabeçalho não tem nenhum botão.
· verificação: manual.

### TDK-036 — Comparação lado a lado
Dado a janela normal e o Big Box no tema Desktop, com o mesmo jogo em foco, a mesma plataforma, a mesma ordenação e a mesma
visualização, em 1920×1080 e 1280×720, quando comparo capturas, então cores, fontes, tamanhos, colunas da grade e painel coincidem,
salvo as diferenças listadas em "Diferenças em relação à janela normal".
· verificação: manual.

### Só lançar (pedido do usuário na aprovação, 2026-11-27)

### TDK-037 — Painel só com "Jogar" e "Jogar sem contar tempo"
Dado o painel da direita (`<aside aria-label="Detalhes do jogo">`) de um jogo com detalhe lido, em Navegar (`active = false`) e no
Painel (`active = true`, foco num botão ou no visor), para qualquer jogo (com e sem vídeo, com e sem screenshots, com e sem sessão
aberta, favorito ou não, concluído ou não, com tempo para zerar em cache, pendente ou ausente, de emulador sem emulador configurado,
com as credenciais do ScreenScraper prontas ou não), então o painel contém **exatamente dois** botões, nesta ordem: "Jogar" (aciona
`LaunchGame`) e "Jogar sem contar tempo" (aciona `LaunchGameUntracked`), e nenhum outro elemento acionável (`button`, `a[href]`,
`input`, `select`, `textarea`, `[role="button"]`, `[role="link"]`, `[role="menuitem"]`, `[contenteditable]`); o visor e as miniaturas
não são botões. Sem jogo em foco ou sem detalhe lido ("Nenhum jogo selecionado", "Carregando jogo…"), o painel não tem nenhum botão.
· verificação: teste de componente table-driven sobre essa matriz de fixtures (`within(aside).getAllByRole('button')` tem tamanho 2
com os nomes "Jogar" e "Jogar sem contar tempo"; a busca pelos seletores acionáveis listados devolve só esses dois; zero nos estados
sem detalhe) e espiões confirmando que cada um chama só o binding de lançamento dele.

### TDK-038 — Nenhum controle fora da lista permitida no DOM do tema
Dado o Big Box no tema Desktop, em cada zona e estado (Jogos em grade e em lista, Plataformas, Painel com foco num botão e no visor,
aba vazia, primeira página carregando, detalhe carregando, aviso de erro, sessão aberta, vídeo tocando), então os únicos elementos
acionáveis de todo o DOM do tema (mesmos seletores de TDK-037) são: os itens de plataforma da coluna esquerda (um por aba, BBX-065) e
os dois botões do painel (TDK-037); os cartões e as linhas são `role="option"` (TDK-029) e o `<video>` não tem `controls`. Em
particular, nenhum elemento (de qualquer papel) tem como nome acessível ou texto um dos rótulos proibidos de TDK-002 e TDK-030
("Big Box", "Tema do Big Box", "Importar", "Provedores", "Emuladores", "Estatísticas", "Editar", "Buscar metadados", "Buscar vídeo",
"Abrir local do arquivo", "Remover", "Tentar de novo", "Confirmar…", "Escolher outro…", "Buscar", "Nenhum destes", "Adicionar sessão",
"Carregar mais", "Ampliar", "Limpar filtros", "Pausar", "Retomar"), salvo "Plataformas" como título de texto da coluna esquerda.
· verificação: teste de componente do shell com o tema Desktop que, a cada estado da lista, coleta os elementos acionáveis do DOM
(`querySelectorAll` com os seletores de TDK-037) e compara com a lista permitida, e confere os rótulos proibidos por `queryByRole`
e `queryByText` (exatos); o teste falha se aparecer qualquer controle novo.

---

## Diferenças em relação à janela normal (aceitas por esta spec)

1. Sem busca, filtros, ordenação e alternância editáveis, sem botões de escrita e sem a lista de sessões (TDK-030); o painel só tem
   "Jogar" e "Jogar sem contar tempo" (TDK-037) e o cabeçalho não tem botões (TDK-002, TDK-038).
2. Sem barra de rolagem; a grade rola por fileiras inteiras (TDK-019); sem divisórias por ano na ordem "Última vez jogado".
3. Rodapé de dicas embaixo (a janela normal não tem).
4. O jogo em foco é marcado pelo contorno de foco (`--arc-bb-focus`) na zona Jogos e por um anel `--arc-bb-text-muted` fora dela; o
   anel azul `--arc-accent` da seleção da janela normal não existe no Big Box (o `--arc-bb-accent` do Desktop é o verde de "Jogar").
5. O selo "Em execução"/"Pausado" usa `--arc-bb-chip`/`--arc-bb-on-chip` (os tokens `--arc-running`/`--arc-paused` não existem no Big
   Box).
6. O visor do painel mostra só capa, vídeo e screenshots (a galeria do Big Box), não banners, fundos nem telas de título.
7. Nenhum hover de mouse diferente do clique (o tema é para controle).

## Fora desta spec

- **Navegação por controle na janela normal** (a primeira opção do pedido): exigiria foco espacial em todas as telas e modais.
- **Alternar grade/lista, ordenar, buscar e filtrar dentro do Big Box** (sem teclado na tela; decisão 7 do usuário).
- **Sessões no painel** (decisão 6 do usuário) e qualquer edição.
- **Qualquer botão do painel além de "Jogar" e "Jogar sem contar tempo"** e qualquer botão da barra superior (pedido do usuário).
- **Manter na janela normal o jogo em foco escolhido no Big Box** ao sair (BBX-001 continua: a janela volta como estava).
- **Escala de texto para TV** (decisão 1 do usuário) e **movimento reduzido** (`prefers-reduced-motion`), como nas fases 13 e 15.
- **Tokens novos de cor** (seleção azul, `running`/`paused`) no contrato de tema.

## Limitações conhecidas

1. **Sem área segura:** numa TV com overscan as bordas (cabeçalho, rodapé, colunas laterais) podem ficar cortadas; use outro tema ou o
   modo "ajustar à tela" da TV.
2. **Texto no tamanho do desktop:** a 3 m numa TV o texto `text-sm` fica pequeno; o tema foi feito para monitor ou TV próxima.
3. **Janelas muito largas** (4K a 100% de escala) mostram até 17 colunas; o limite de 200 nós cobre até 10 fileiras desenhadas nessa
   largura, mais do que cabe na altura.

## ADR proposto

A sessão principal escreve (a spec só o referencia). Próximo número livre: **0042**.

1. **0042 — Layout em colunas no contrato de tema do Big Box (tema Desktop):** estende o ADR 0040. O reducer compartilhado ganha zonas
   (`games`/`platforms`) e o layout `columns` (←/→ dentro da fileira e entre zonas; Detalhes empilhados; B volta um nível); o shell
   monta os Detalhes também em Navegar; o tema declara colunas e fileiras por janela e visualização, limite de nós, rolagem pelas bordas,
   vídeo no painel e ordem herdada. O tema Desktop reproduz a janela normal com componentes próprios, constantes compartilhadas e tokens
   `--arc-bb-*` copiados dos `--arc-*` e conferidos por teste, em vez de reaproveitar os componentes do desktop ou dar navegação por
   controle à janela normal.

## Decisões do usuário (2026-11-27)

As oito perguntas da proposta foram resolvidas pelo usuário na aprovação, todas com a resposta padrão. Nenhum requisito precisou
mudar por elas (os critérios já seguiam o padrão); ficam registradas aqui.

1. **Tamanho do texto:** igual ao desktop, 1:1 (decisão k). A escala para TV fica fora desta spec (Limitações 2).
2. **Ordem dos jogos:** herdada da ordenação atual da janela normal (decisão i; BBX-067).
3. **Área segura:** nenhuma, tela inteira como o desktop (decisão j; BBX-069, TDK-001; Limitações 1).
4. **B em Plataformas:** volta para Jogos, não sai do Big Box (decisão d; TDK-015). B só sai a partir de Jogos (TDK-017).
5. **← num botão do painel:** volta para Jogos (decisão b; TDK-018).
6. **Sessões no painel:** ficam fora; `session.list` continua proibido no Big Box (TDK-030, TDK-031, TDK-037).
7. **Alternar grade/lista no Big Box:** fica fora; vale a escolha da janela normal, sem botão Start (BBX-068, TDK-004, TDK-030).
8. **↑/↓ em Plataformas:** trocam a lista na hora, sem confirmação por A (decisão c; TDK-015).

Pedidos adicionais do usuário na mesma aprovação: (a) no painel de detalhes, só "Jogar" e "Jogar sem contar tempo", nenhum botão de
buscar metadados, editar, buscar vídeo, abrir local, remover etc. (TDK-037, TDK-030, TDK-008); (b) nenhum botão da barra superior
(adicionar jogos, provedores, emuladores e semelhantes) (TDK-002, TDK-038).
