# Big Box — tema Desktop: layout, zonas, transições, capacidade, rolagem e tokens

Requisitos: BBX-060..BBX-070, TDK-001..TDK-036. Complementa (não substitui) [bigbox-temas-v2](../../phase-15-big-box-v2/contracts/bigbox-temas-v2.md)
e [bigbox-faixa](../../phase-13-modo-console/contracts/bigbox-faixa.md): onde este documento não fala, vale o de lá.

## 1. Contrato de tema — campos novos [BBX-061]

```ts
// frontend/src/bigbox/themes/types.ts (acréscimos; todos opcionais)
export type BigBoxThemeId = 'faixa' | 'mosaico' | 'estante' | 'desktop' | 'roda'
export type LibraryView = 'grid' | 'list'
export type BigBoxThemeDef = {
  /* campos de temas-v2 §1 */
  videoSlot: 'background' | 'cover' | 'panel'                 // 'panel': vídeo no visor do painel (TDK-010)
  sort: 'title' | 'lastPlayed' | 'inherit'                     // 'inherit': ordenação da janela normal (BBX-067)
  layout?: 'screens' | 'columns'                               // padrão 'screens'
  gridColumns?: (vp: Viewport, view: LibraryView) => number    // padrão: `columns`
  gridRows?: (vp: Viewport, view: LibraryView) => number       // já existia com (vp); o 2º argumento é ignorado pela Estante
  maxNodes?: number                                            // padrão 30 (GRID_MAX_NODES)
  gridScroll?: 'focusTop' | 'edges'                            // padrão 'focusTop'
}
export type BigBoxThemeProps = { /* temas-v2 */ onTab?: (index: number) => void } // BBX-065
```

| Tema | `axis` | `columns` | `layout` | `gridColumns`/`gridRows` | `maxNodes` | `gridScroll` | `backdrop` | `videoSlot` | `tabs` | `sort` | `timeToBeatInBrowse` | `Details` |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| `faixa`, `mosaico`, `estante`, `roda` | como hoje | como hoje | (screens) | como hoje | (30) | (focusTop) | como hoje | como hoje | como hoje | como hoje | como hoje | como hoje |
| `desktop` | grid | 1 (ignorado) | columns | `desktopGrid(vp, view)` | 200 | edges | plain | panel | platforms | inherit | sim | `DesktopDetails` |

O shell usa `C = gridColumns?.(vp, view) ?? columns` e `R = gridRows?.(vp, view) ?? 1` em todos os lugares onde hoje usa
`theme.columns` e `theme.gridRows(vp)` (contexto do reducer, modelo, janela, páginas, pré-busca).

## 2. Reducer — layout `columns` [TDK-012..TDK-018, BBX-065]

`NavState` ganha `zone: 'games' | 'platforms'` (inicial `games`; só tem sentido em `browse`). `NavCtx` ganha `layout` e `axis` (o
comportamento de grade em ↑/↓ passa a depender de `axis = 'grid'`, não de `columns > 1`; para os temas atuais o resultado é o mesmo).
`NavAction` ganha `tabSet {index}`. Com `layout = screens`, todas as transições de [temas-v2 §3](../../phase-15-big-box-v2/contracts/bigbox-temas-v2.md)
e [bigbox-faixa §3](../../phase-13-modo-console/contracts/bigbox-faixa.md) ficam iguais (`zone` fica sempre `games`).

Com `layout = columns`, `C` colunas, `M = min(T, L) − 1`, `col = f mod C`:

| Estado | Ação | Próximo | Efeito | Req. |
|---|---|---|---|---|
| browse, games | `prev` | `col > 0` → `f − 1`; `col = 0` (ou `T = 0`) → `zone = platforms` | — | TDK-013, TDK-014 |
| browse, games | `next` | `T > 0`, `col < C − 1` e `f < M` → `f + 1`; senão, com jogo em foco → `details`, `detailsFocus = detailsButton = play`, `gallery = 0`; sem jogo → (mesmo) | — | TDK-013 |
| browse, games | `down` | `f + C ≤ M` → `f + C`; senão `⌊f/C⌋ < ⌊M/C⌋` → `M`; senão (mesmo) | — | TDK-013 (BBX-048) |
| browse, games | `up` | `f − C ≥ 0` → `f − C`; senão (mesmo) | — | TDK-013 |
| browse, games | `accept` (com jogo) | (mesmo) | `launch` | TDK-017 |
| browse, games | `back` | (mesmo) | `exit` | TDK-017 |
| browse, platforms | `up` | `tab > 0` → `tab − 1`, `focus = 0`; senão (mesmo) | trocar a consulta | TDK-015 |
| browse, platforms | `down` | `tab < tabs − 1` → `tab + 1`, `focus = 0`; senão (mesmo) | trocar a consulta | TDK-015 |
| browse, platforms | `next`, `accept`, `back` | `zone = games` | — | TDK-015 |
| browse, platforms | `prev` | (mesmo) | — | TDK-015 |
| browse, qualquer zona | `tabPrev` / `tabNext` | circular, `focus = 0`, zona igual | trocar a consulta | TDK-016 |
| browse, qualquer zona | `details` (com jogo) | `details`, foco em `play`, `gallery = 0` | — | TDK-016 |
| details | `up` | `playUntracked` → `play`; `play` → `gallery` se `galleryCount > 0`, senão (mesmo); `gallery` → (mesmo) | — | TDK-018 |
| details | `down` | `gallery` → `play`; `play` → `playUntracked`; `playUntracked` → (mesmo) | — | TDK-018 |
| details, foco no visor | `prev` / `next` | `gallery ∓/± 1` limitado a `[0, galleryCount − 1]` | — | TDK-018 |
| details, foco num botão | `prev` | `browse`, `zone = games` (mesmo `focus`) | — | TDK-018 |
| details, foco num botão | `next` | (mesmo) | — | TDK-018 |
| details | `back` | `browse`, `zone = games` | — | TDK-018 |
| details | `galleryChanged(0)` com foco no visor | `detailsFocus = play` | — | TDK-018 |
| qualquer | `tabSet {i}` com `0 ≤ i < tabs` e `i ≠ tab` | `browse`, `tab = i`, `focus = 0` (zona igual) | trocar a consulta | BBX-065 |
| qualquer | `tabSet {i}` com `i = tab` ou fora da faixa | (mesmo) | — | BBX-065 |
| qualquer | `listLoaded`, `gameClosed` | como hoje e `zone = games` | como hoje | TDK-012, TDK-033 |

`detailsButton` acompanha o último botão focado, como hoje. `accept` em Detalhes, `toggleSound`, `launched`, `windowBlur`,
`windowFocus`, `sessionEnded`, `videoFailed` e a suspensão não mudam.

Vetores (`C = 6`, `T = L = 20`, `M = 19`, `tabs = 4`, `galleryCount = 3` salvo indicação):

| Estado | `f` | Ação | Resultado |
|---|---|---|---|
| games | 0 | prev | zone platforms, `f = 0` |
| games | 3 | prev | `f = 2` |
| games | 4 | next | `f = 5` |
| games | 5 | next | details, `play` |
| games | 19 | next | details, `play` (último jogo, coluna 1) |
| games | 13 | down | `f = 19` |
| games | 14 | down | `f = 19` (fileira parcial) |
| games | 18 | down | `f = 18` |
| games | 8 | up | `f = 2` |
| games | 2 | up | `f = 2` |
| games, `C = 1`, `T = 5` | 0 / 0 / 2 / 2 | up / down / prev / next | 0 / 1 / platforms / details |
| games, `T = 0` | 0 | prev / next | platforms / (mesmo) |
| platforms, `tab = 0` | — | up / down | (mesmo) / `tab = 1`, `f = 0` |
| platforms, `tab = 3` | — | down | (mesmo) |
| platforms | — | next / accept / back / prev | games / games / games / (mesmo) |
| details, `play` | — | up / down / prev / next / back | gallery / playUntracked / browse games / (mesmo) / browse games |
| details, `play`, `galleryCount = 0` | — | up | (mesmo) |
| details, `gallery = 1` | — | down / prev / up | play / `gallery = 0` / (mesmo) |

## 3. Capacidade `desktopGrid` [TDK-007, TDK-034]

Constantes da janela normal (lidas de `lib/gridNav` e das variáveis de UIX-005, nunca redefinidas): `CARD_MIN_WIDTH = 160`,
`GRID_GAP = 16`, `CARD_TITLE_HEIGHT = 60`, `GRID_INSET = 6`, `SCROLLBAR_SIZE = 8`, `LIST_ROW_HEIGHT = 64` (hoje `ROW_HEIGHT` local de
`LibraryList`; passa a `lib/gridNav`). Constantes do tema (alturas fixas do cromo): `HEADER_PX = 65`, `CENTER_PAD_X = 24`,
`CENTER_PAD_Y = 16`, `TOOLBAR_PX = 32`, `TOOLBAR_GAP = 12`, `FOOTER_PX = 48`, `FOOTER_GAP = 12`.

```
desktopGrid({width: W, height: H}, view):
  W ≤ 0 ou H ≤ 0 → grade {columns: 4, rows: 2}; lista {columns: 1, rows: 8}
  left  = clamp(220, 0,16·W, 300);  right = clamp(320, 0,26·W, 480)
  content = W − left − right − 2·CENTER_PAD_X
  A = H − HEADER_PX − 2·CENTER_PAD_Y − TOOLBAR_PX − TOOLBAR_GAP − FOOTER_PX − FOOTER_GAP   // H − 201
  grade: {cols, rowHeight} = columnsFor(usableWidth(content));  rows = max(1, ⌊(A − 2·GRID_INSET) / rowHeight⌋)
  lista: cols = 1;  rows = max(1, ⌊(A − 2·GRID_INSET) / LIST_ROW_HEIGHT⌋)
```

| Janela | Visualização | `columns` × `rows` | `R'` (N = 200) | Nós desenhados |
|---|---|---|---|---|
| 1920 × 1080 | grade | 6 × 2 | 2 | 18 |
| 1366 × 768 | grade | 4 × 1 | 1 | 8 |
| 1280 × 720 | grade | 3 × 1 | 1 | 6 |
| 3840 × 2160 | grade | 17 × 6 | 6 | 119 |
| 1920 × 1080 | lista | 1 × 13 | 13 | 14 |
| 1280 × 720 | lista | 1 × 7 | 7 | 8 |
| 0 × 0 | grade / lista | 4 × 2 / 1 × 8 | 2 / 8 | 12 / 9 |

No tema Desktop, o rodapé tem altura fixa de `FOOTER_PX` (regra `[data-bigbox-theme="desktop"] [data-bb-footer]` em `index.css`).

## 4. Janela e páginas com limite por tema [BBX-062]

```
R' = clamp(R, 1, max(⌊N / C⌋ − 1, 1))                    // N = maxNodes (30 por padrão)
gridWindowAt(T, s, C, R') = {start: s·C, end: min((s + R' + 1)·C, T)}   // T = 0 → {0, 0}
focusTop: s = clamp(⌊f/C⌋, 0, max(⌈T/C⌉ − R', 0))         // BBX-049 como hoje
edges:    s de §5
gridPageSize(C, R')            = min((R' + 1 + PAGE_EXTRA)·C, PAGE_MAX)
shouldPrefetchGrid(L,T,f,C,R') = !exhausted && L < T && L < min(T, (⌊f/C⌋ + R' + 1 + PAGE_EXTRA)·C)
```

| `T` | `s` | `C` | `R` | `N` | `R'` | janela | página |
|---|---|---|---|---|---|---|---|
| 23 | 0 | 5 | 1 | 30 | 1 | [0, 10) | 25 (igual a hoje) |
| 23 | 4 | 5 | 2 | 30 | 2 | [20, 23) | 30 |
| 60 | 0 | 6 | 2 | 200 | 2 | [0, 18) | 36 |
| 60 | 3 | 6 | 2 | 200 | 2 | [18, 36) | 36 |
| 20 | 1 | 6 | 2 | 200 | 2 | [6, 20) | 36 |
| 500 | 0 | 17 | 6 | 200 | 6 | [0, 119) | 170 |
| 200 | 0 | 1 | 13 | 200 | 13 | [0, 14) | 17 |

Com `edges`, `s + R' − 1 ≥ ⌊f/C⌋`, logo a janela termina antes de `(⌊f/C⌋ + R' + 1)·C` e a regra de pré-busca continua cobrindo três
fileiras além dela. A janela é recortada em `L` como hoje.

## 5. Rolagem pelas bordas [BBX-063, TDK-019]

```
maxStart(R', rows) = max(rows − R', 0)                    // rows = ⌈T/C⌉
edgeScrollRow(prev, r, R', rows) = clamp(r < prev ? r : r > prev + R' − 1 ? r − R' + 1 : prev, 0, maxStart)
centeredRow(r, R', rows)         = clamp(r − ⌊(R' − 1)/2⌋, 0, maxStart)
```

O shell guarda `s` (um `useRef` por aba, zerado na troca de aba) e aplica `centeredRow` em `listLoaded` (entrada), na troca de aba e em
`gameClosed`; nas outras mudanças de foco, `edgeScrollRow`. Um resize recalcula `R'` e aplica `edgeScrollRow` com o `s` guardado.

| Função | Entrada | `s` |
|---|---|---|
| edge | prev 0, r 0, R' 2, rows 10 | 0 |
| edge | prev 0, r 1, R' 2, rows 10 | 0 |
| edge | prev 0, r 2, R' 2, rows 10 | 1 |
| edge | prev 3, r 2, R' 2, rows 10 | 2 |
| edge | prev 0, r 9, R' 2, rows 10 | 8 |
| edge | prev 8, r 9, R' 2, rows 10 | 8 |
| edge | prev 0, r 1, R' 1, rows 5 | 1 |
| edge | prev 0, r 1, R' 3, rows 2 | 0 |
| centered | r 5, R' 3, rows 10 | 4 |
| centered | r 0, R' 3, rows 10 | 0 |
| centered | r 9, R' 3, rows 10 | 7 |
| centered | r 4, R' 2, rows 10 | 4 |

## 6. Layout e mapa de entrada [TDK-001..TDK-026]

```
┌ janela (fundo --arc-bb-bg, área segura 0) ───────────────────────────────────────────────────────────┐
│ Arcadia                                                               (cabeçalho, 65 px, borda embaixo)│
│ ▒ faixa de erro role=alert (só com erro) ▒                                                            │
│┌ nav "Plataformas" ──┐┌ região "Jogos" ─────────────────────────────┐┌ aside "Detalhes do jogo" ─────┐│
││ PLATAFORMAS         ││ 120 jogos         Ordenar por: Título  Grade ││ ▣ Título                       ││
││[Todos          120] ││ [▣][ ][ ][ ][ ][ ]                           ││ ⌂ Nintendo 64  · Em execução   ││
││ ⌂ Nintendo 64   40  ││ [ ][ ][ ][ ][ ][ ]                           ││ ┌ visor 16:9 ┐  Vídeo · 1 de 4 ││
││ ⌂ SNES          80  ││ [ ][ ][ ][ ][ ][ ]  (fileira cortada)        ││ [ Jogar                     ]  ││
││                     ││                                              ││ [ Jogar sem contar tempo    ]  ││
││                     ││                                              ││ Tempo total · Primeira vez ... ││
││                     ││                                              ││ Descrição …                    ││
│└─────────────────────┘└──────────────────────────────────────────────┘└────────────────────────────────┘│
│ ▒ rodapé 48 px: A Jogar  X Detalhes  LB/RB Plataforma  Select+B Fechar o jogo  B Sair ▒               │
└────────────────────────────────────────────────────────────────────────────────────────────────────────┘
```

- Colunas laterais e rodapé sobre `--arc-bb-panel`; centro e cabeçalho sobre `--arc-bb-bg`; divisórias de 1 px em `--arc-bb-placeholder`.
- Item ativo da coluna esquerda: fundo `--arc-bb-surface`, texto `--arc-bb-text`; demais `--arc-bb-text-muted`; ícone 24 px.
- Cartão: capa 3:4 sobre `--arc-bb-surface`, título `text-sm` em 2 linhas, "Jogado …" `text-xs` `--arc-bb-text-muted`.
- Marca de foco (TDK-023): `outline: 2px solid var(--arc-bb-focus); outline-offset: 2px` e `data-focused="true"` num só elemento.
  Seleção fora da zona Jogos: fundo `--arc-bb-surface` e anel 2 px `--arc-bb-text-muted`.
- Botões do painel: "Jogar" `--arc-bb-accent`/`--arc-bb-on-accent`, `min-h-12`, `text-lg` negrito; "Jogar sem contar tempo"
  `--arc-bb-surface` com borda `--arc-bb-accent`, `min-h-10`.

| Entrada | Teclado | Jogos | Plataformas | Painel |
|---|---|---|---|---|
| ←/→ (direcional, analógico esq.) | setas | §2 | → volta a Jogos; ← nada | visor: item; botão: ← volta a Jogos, → nada |
| ↑/↓ | setas | fileira / linha | plataforma anterior/seguinte | visor ↔ Jogar ↔ Jogar sem contar tempo |
| analógico direito ↕ | PageUp/PageDown | — (PageUp/PageDown: LB/RB) | — (LB/RB) | rola o painel |
| A | Enter | `LaunchGame` | volta a Jogos | aciona o botão; visor: nada |
| X | D | Painel | Painel | nada |
| LB/RB | PageUp/PageDown | aba circular | aba circular | nada |
| Y | M | som do vídeo visível | idem | (como BBX-015) |
| B | Esc | sai do Big Box | volta a Jogos | volta a Jogos |
| Select + A/B/Y, Select + ↑/↓ | — | ignorados na UI (BBX-057) | idem | idem |
| clique / duplo clique | — | foca / lança | ativa a plataforma | botões lançam |

## 7. Tokens do Desktop [TDK-027, TDK-028, BBX-069]

| Token | Valor | Igual a (tema escuro) | Uso no Desktop |
|---|---|---|---|
| `--arc-bb-bg` | `#16181d` | `--arc-bg` | centro, cabeçalho |
| `--arc-bb-bg-2` | `#1e2128` | `--arc-surface` | (gradiente sem arte; não desenhado com `plain`) |
| `--arc-bb-scrim` | `#00000000` | — | não usado |
| `--arc-bb-panel` | `#1e2128` | `--arc-surface` | colunas laterais, rodapé |
| `--arc-bb-text` | `#e6e8ec` | `--arc-text` | texto |
| `--arc-bb-text-muted` | `#9aa3b2` | `--arc-text-muted` | texto secundário, anel de seleção |
| `--arc-bb-heading` | `#e6e8ec` | `--arc-text` | títulos |
| `--arc-bb-accent` | `#238636` | `--arc-play` | "Jogar" |
| `--arc-bb-on-accent` | `#ffffff` | `--arc-on-accent` | texto de "Jogar" |
| `--arc-bb-surface` | `#272b34` | `--arc-surface-2` | item ativo, jogo selecionado, botões, fundo da capa |
| `--arc-bb-placeholder` | `#343a46` | `--arc-border` | divisórias, bordas, cartão sem imagem |
| `--arc-bb-chip` | `#323744` | `--arc-surface-3` | selo do botão no rodapé, selo de sessão |
| `--arc-bb-on-chip` | `#e6e8ec` | `--arc-text` | texto do selo |
| `--arc-bb-focus` | `#79b8ff` | `--arc-focus` | marca de foco |
| `--arc-bb-danger` | `#ff6b64` | `--arc-danger` | faixa de erro |
| `--arc-bb-safe-y` / `--arc-bb-safe-x` | `0` / `0` | — | área segura (BBX-069) |

Pares de contraste (TDK-028): `text`, `text-muted`, `heading`, `danger` sobre `panel`, `bg` e `surface` (≥ 4,5:1; o pior,
`text-muted`/`surface`, ≈ 5,6:1); `on-accent`/`accent` (≈ 4,6:1); `on-chip`/`chip`; `focus` contra `panel`, `bg` e `surface` (≥ 3:1).
Os valores são os da tabela do tema escuro de hoje; se a janela normal mudar um token, TDK-027 obriga o Desktop a acompanhar.

## 8. Dicas do rodapé [TDK-024]

| Situação | Dicas, nesta ordem |
|---|---|
| Jogos, com jogos | A Jogar · X Detalhes · LB/RB Plataforma · Select+B Fechar o jogo · B Sair · (Y Som com vídeo visível) |
| Jogos, aba vazia | LB/RB Plataforma · ← Plataformas · B Sair |
| Plataformas | ↑/↓ Plataforma · A Ver jogos · LB/RB Plataforma · B Voltar · Select+B Fechar o jogo |
| Painel, botão | A Jogar (ou A Jogar sem contar tempo) · ↑/↓ Mover · B Voltar · Select+B Fechar o jogo |
| Painel, visor | ←/→ Imagem · ↓ Botões · B Voltar · Select+B Fechar o jogo |

## 9. Constantes novas

| Constante | Valor | Requisito |
|---|---|---|
| `DESKTOP_MAX_NODES` | 200 | BBX-062, TDK-019 |
| `LIST_ROW_HEIGHT` | 64 (movida de `LibraryList` para `lib/gridNav`) | TDK-006, TDK-034 |
| `HEADER_PX`, `TOOLBAR_PX`, `TOOLBAR_GAP`, `FOOTER_PX`, `FOOTER_GAP` | 65, 32, 12, 48, 12 | TDK-007 |
| `CENTER_PAD_X`, `CENTER_PAD_Y` | 24, 16 (`px-6 py-4` do centro da janela normal) | TDK-007 |
| fallback de `desktopGrid` | grade 4 × 2, lista 1 × 8 | TDK-007 |
