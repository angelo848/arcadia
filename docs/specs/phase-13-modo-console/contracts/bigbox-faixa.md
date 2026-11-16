# Big Box — contrato do shell, da navegação e do tema Faixa

Requisitos: BBX-001..BBX-044 (BBX-007 e BBX-031 removidos), CON-022. Fonte da verdade para `frontend/src/bigbox/`.
· alterado em 2026-11-16: área segura, painéis atrás do texto, fundo e capas em `contain`, faixa sem buracos, lista lida inteira e
layout/foco novos dos detalhes (BBX-032..BBX-044).

## 1. Layout do Faixa (tela Navegar, 16:9; medidas relativas à janela)

```
┌ janela (fundo BBX-040 ocupa tudo) ─────────────────────────────────────────────┐
│ ┌ área segura: 5vh em cima e embaixo, 5vw nas laterais (data-bb-safe) ───────┐ │
│ │ ▒[Todos]  Nintendo 64  Super Nintendo ...▒   ← abas num painel (6vh)        │ │
│ │ [▣] ▢ ▢ ▢ ▢ ▢ ▢ ▢ ▢ ▢ ▢ ▢ ▢|                  ← faixa cheia até a borda (18vh)│ │
│ │ ▒ TÍTULO GRANDE (clamp 2.4rem..4rem)       ▒                                │ │
│ │ ▒ Plataforma · 1996 · Ação                 ▒                                │ │
│ │ ▒ Descrição curta (até 280, 3 linhas)      ▒        ARTE / VÍDEO DE FUNDO   │ │
│ │ ▒ Tempo jogado · Última vez · Sessões      ▒        (contain + preenchimento)│ │
│ │ ▒ [ Jogar ]  [ Detalhes ]                  ▒                                │ │
│ │ (aviso de erro, role=alert, surface)                                         │ │
│ │ ▒ A Jogar  X Detalhes  LB/RB Plataforma  Select+B Fechar o jogo  B Sair [Y Som]▒│ │
│ └──────────────────────────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────────────────────────┘
▒ = painel `--arc-bb-panel` (data-bb-panel), cantos arredondados, recuo 0,75em..1.25em
```

- Fundo [BBX-040]: duas camadas por arte. Embaixo, o **preenchimento** (`data-testid="bb-art-fill"`): a arte em
  `object-fit: cover`, `filter: blur(≥ 24px)`, coberta por `--arc-bb-scrim`. Em cima, a **arte** (`data-testid="bb-art"`) em
  `object-fit: contain`, centrada. O vídeo (`bb-video`) também em `contain`, sobre o mesmo preenchimento. Sem arte, o gradiente.
  O véu em gradiente sobre a arte deixa de existir: o contraste do texto vem dos painéis [BBX-026, BBX-037].
- Área segura [BBX-042]: `data-bb-safe` com `padding: 5vh 5vw`; contém abas, faixa, bloco, aviso e rodapé (o rodapé sai do
  shell para dentro dela). Só `BackgroundStage` fica fora.
- Faixa [BBX-006, BBX-039, BBX-041]: capas em 2:3 (`h-[18vh]`), imagem em `object-fit: contain` sobre `--arc-bb-surface`; a em
  foco escala 1,25 com contorno `--arc-bb-focus`; janela de §3.1, ancorada na posição 2; com mais jogos que a capacidade, a última
  capa desenhada fica cortada na borda direita.
- Bloco de informações: coluna esquerda, até 40 % da largura, dentro de um painel. Marcação "Em execução"/"Pausado" ao lado do
  título e como selo na capa (BBX-017).
- Painéis [BBX-037]: um por bloco de texto (abas; bloco de informações; "Carregando jogos…"/lista vazia; rodapé). Componente
  único `Panel` (`data-bb-panel`, fundo `--arc-bb-panel`).
- Texto sempre: nomes de plataforma e de jogo nunca são logos no Faixa.

### 1.1 Tela Detalhes [BBX-016, BBX-032..BBX-037, BBX-041..BBX-044]

```
┌ área segura ────────────────────────────────────────────────────────────────────┐
│ ┌ painel: coluna de texto (40%) ─────┐  ┌ visor (resto da largura) ───────────────┐ │
│ │ TÍTULO (clamp 2rem..3.2rem)        │  │                                          │ │
│ │ ┌ data-bb-facts (gap ≤ 0,25em) ──┐ │  │   item selecionado inteiro (contain),    │ │
│ │ │ Plataforma · 1996 · Ação, RPG  │ │  │   vídeo tocando quando é o selecionado   │ │
│ │ │ Desenvolvedores: …             │ │  │                                ▒3 / 7▒   │ │
│ │ │ Editoras: …                    │ │  └──────────────────────────────────────────┘ │
│ │ │ Um jogador, Cooperativo        │ │  ┌ miniaturas (h 10vh, 16:9, contain) ──────┐ │
│ │ │ 12 h 5 min · Última vez: … · S │ │  │ [▶] ▢ (▣) ▢ ▢ ▢ ▢ ...  rola p/ selecionado│ │
│ │ └────────────────────────────────┘ │  └──────────────────────────────────────────┘ │
│ │ [ Jogar ] [ Jogar sem contar tempo]│                                                │
│ │ Descrição (flex 1, overflow-y auto)│                                                │
│ └────────────────────────────────────┘                                                │
│ ▒ rodapé ▒                                                                            │
└───────────────────────────────────────────────────────────────────────────────────────┘
```

- Raiz da tela: `display: grid` (duas colunas 40 % / 1fr), `height: 100%`, `min-height: 0`, `overflow: hidden`; a coluna de texto é
  flex em coluna, com só a descrição em `overflow-y: auto` e `min-height: 0` [BBX-043]. A coluna do visor é flex em coluna: visor
  `flex: 1; min-height: 0`, miniaturas com altura fixa de 10vh [BBX-043].
- Espaçamento [BBX-044]: `data-bb-facts` com `display: flex; flex-direction: column; row-gap ≤ 0.25em; line-height ≤ 1.4`,
  parágrafos com `margin: 0`; entre título, fatos, botões e descrição, `gap ≤ 0.75em`.
- Sem galeria (N = 0): a coluna do visor não é desenhada; a coluna de texto vai até 60 %.
- Miniatura do vídeo: um cartão com o texto "Vídeo" (sem `<video>`); o único `<video>` da tela é o do visor [BBX-034].
- Marca de foco [BBX-035]: foco num botão → só ele tem `data-focused="true"` e contorno; foco na galeria → só a fileira de
  miniaturas tem `data-focused="true"` e contorno, e dentro dela a selecionada tem `aria-current="true"` e borda
  `--arc-bb-text-muted`.

## 2. Mapa de entrada

Mapeamento padrão da Gamepad API (`navigator.getGamepads()`, primeiro controle conectado): 0 A, 1 B, 2 X, 3 Y, 4 LB, 5 RB,
8 Select/Back, 9 Start, 12..15 direcional; eixos 0/1 analógico esquerdo, 2/3 analógico direito. Borda de subida; zona morta 0,6.

| Entrada | Teclado | Navegar | Detalhes |
|---|---|---|---|
| ←/→ (direcional, analógico esq.) | setas | jogo anterior/seguinte (sem volta nas pontas) | foco num botão: Jogar ↔ Jogar sem contar tempo; foco na galeria: item anterior/seguinte no visor (sem volta) [BBX-033, BBX-034] |
| ↑/↓ | setas | nada | ↓ dos botões para a galeria (se houver); ↑ da galeria para o último botão [BBX-032] |
| analógico direito ↕ | PageUp / PageDown | nada (PageUp/PageDown: ver LB/RB) | rola a descrição, com o foco onde estiver [BBX-036] |
| A | Enter | `LaunchGame` do jogo em foco | aciona o botão em foco; na galeria, nada |
| X | D | abre Detalhes | nada |
| LB / RB | PageUp / PageDown (só em Navegar) | aba anterior/seguinte (circular) | nada |
| Y | M | liga/desliga o som (com vídeo visível) | idem |
| B | Esc | sai do Big Box | volta a Navegar (foco no mesmo jogo) |
| Select segurado + B ou Y | — | **ignorado na UI** (combos do agente) | idem |
| Start | — | nada nesta fase | nada |
| clique / duplo clique na capa | — | foca / lança | — |

Tecla mantida (`repeat`) nunca relança nem reabre (UIX-046). Setas e analógico repetem ao segurar (400 ms, depois 120 ms).

## 3. Estados da navegação (reducer puro `bigBoxNav`)

Estado: `screen ∈ {browse, details}`, `tab` (índice; 0 = Todos), `focus` (índice na lista da aba), `detailsFocus ∈ {play,
playUntracked, gallery}`, `detailsButton ∈ {play, playUntracked}` (último botão focado), `gallery` (índice), `suspended` (bool),
`awaitingFocusAfterBlur` (bool), `sound` (bool, começa `false`), `videoFailed` (id do jogo ou `null`), `launchedGameId`.
Contexto (`NavCtx`, lido do shell a cada ação): `tabs`, `total` (jogos carregados da aba, BBX-038), `galleryCount`,
`focusedGameId`, `videoVisible`. · alterado em 2026-11-16: `description` sai de `detailsFocus`, entra `detailsButton`; sem
`loaded` nem `fetchNextPage` (BBX-007 removido).

| Estado | Ação | Próximo | Efeito pedido ao shell |
|---|---|---|---|
| qualquer, `suspended` | qualquer ação de entrada | (mesmo) | nada [BBX-018] |
| browse | `prev` / `next` | `focus ± 1` limitado a `[0, total − 1]`; `videoFailed = null` | — [BBX-006] |
| browse | `tabPrev` / `tabNext` | `tab ∓/± 1` circular; `focus = 0` | trocar a consulta da lista [BBX-005, BBX-038] |
| browse | `accept` (com jogo) | browse | `LaunchGame(id)` [BBX-014] |
| browse | `details` (com jogo) | details; `detailsFocus = detailsButton = play`; `gallery = 0` | — [BBX-016] |
| browse | `back` | — | `exitBigBox()` e desmontar [BBX-002] |
| details, foco em botão | `prev` / `next` | `detailsFocus = detailsButton =` play / playUntracked (sem volta) | — [BBX-033] |
| details, foco na galeria | `prev` / `next` | `gallery ± 1` limitado a `[0, galleryCount − 1]` | — [BBX-034] |
| details, foco em botão | `down` | `galleryCount > 0` → `detailsFocus = gallery`; senão (mesmo) | — [BBX-032] |
| details, foco em botão | `up` | (mesmo) | — [BBX-032] |
| details, foco na galeria | `up` | `detailsFocus = detailsButton` | — [BBX-032] |
| details, foco na galeria | `down` | (mesmo) | — [BBX-032] |
| details | `accept` em play / playUntracked | details | `LaunchGame` / `LaunchGameUntracked` |
| details | `accept` na galeria | (mesmo) | — [BBX-016] |
| details | `galleryChanged(count)` | `gallery = min(gallery, max(count − 1, 0))`; `count = 0` com foco na galeria → `detailsFocus = detailsButton` | — [BBX-032, BBX-034] |
| browse | `galleryChanged(count)` | (mesmo) | — |
| details | `back` | browse (mesmo `focus`) | — |
| qualquer | `toggleSound` com vídeo visível | `sound = !sound` | — [BBX-013] |
| qualquer | `launched(id)` (sucesso) | `suspended = true` | — [BBX-018] |
| qualquer, `suspended` | `windowBlur` | `awaitingFocusAfterBlur = true` | — |
| qualquer, `suspended`, `awaitingFocusAfterBlur` | `windowFocus` | `suspended = false`, `awaitingFocusAfterBlur = false` | — |
| qualquer, `suspended` | `sessionEnded(id)` do jogo lançado | `suspended = false` | — |
| qualquer | `gameClosed(id)` | browse; `suspended = false`; `focus` = índice de `id` se carregado na aba, senão o mesmo | `BringToFront()`, `enterBigBox()` [CON-022] |
| browse | `listLoaded(items)` na entrada | `focus` = índice do jogo em foco da janela normal, se presente; senão 0 | — [BBX-027] |

`galleryChanged` não é ação de entrada: vale também com `suspended`. O shell a despacha quando `galleryOf(media).length` muda.

### 3.1 Janela da faixa [BBX-039]

Função pura `stripWindow(total T, focus f, capacity V) → {start, end}` (`end` exclusivo), com `A = STRIP_ANCHOR = 2`:

```
V' = clamp(V, 1, STRIP_MAX − 1)          // STRIP_MAX = 15 nós no DOM
start = clamp(f − A, 0, max(T − V', 0))
end   = min(start + V' + 1, T)            // + 1: a capa cortada na borda direita
T = 0 → {0, 0}
```

| T | f | V | start | end | nota |
|---|---|---|---|---|---|
| 40 | 0 | 12 | 0 | 13 | entrada na aba: faixa cheia desde a primeira capa |
| 40 | 2 | 12 | 0 | 13 | foco anda até a âncora |
| 40 | 3 | 12 | 1 | 14 | a faixa rola, foco fica na posição 2 |
| 40 | 39 | 12 | 28 | 40 | fim: 12 capas, foco na posição 11 (inteira) |
| 8 | 5 | 12 | 0 | 8 | lista menor que a capacidade |
| 0 | 0 | 12 | 0 | 0 | aba vazia |
| 100 | 50 | 30 | 48 | 63 | V limitado a 14 → no máximo 15 nós |

Capacidade do Faixa (`faixa.listCapacity({width, height})`, pura): largura útil `W = width × 0,9` (área segura) menos os recuos
laterais da faixa; capa `c = 0,18 × height × 2/3`; vão `g` = 16 px; `V = max(1, floor((W − 0,25 × c + g) / (c + g)))` (a folga
de `0,25 × c` é a escala 1,25 da capa em foco). Sem tamanho conhecido (0 × 0, jsdom): `STRIP_FALLBACK_CAPACITY = 12`. O shell lê
`window.innerWidth`/`innerHeight` e assina `resize` (removido ao desmontar).

## 4. Modelo de exibição

```ts
type BigBoxThemeId = 'faixa' | 'mosaico' | 'roda'
type BigBoxTab = { platformId: number | null; label: string } // null = Todos
type BigBoxGame = { id: number; title: string; coverUrl: string | null; session: 'running' | 'paused' | null }
type BigBoxFocused = {
  game: BigBoxGame
  platform: string; year: number | null; genre: string | null
  shortDescription: string; totalPlaySeconds: number; lastPlayedAt: string | null; playCount: number
  backgroundUrl: string | null; videoUrl: string | null
}
type BigBoxViewModel = {
  screen: 'browse' | 'details'
  tabs: BigBoxTab[]; tab: number
  loading: boolean                                    // lendo as páginas da aba (BBX-038): "Carregando jogos…"
  games: BigBoxGame[]; total: number; focus: number   // games = janela de stripWindow (§3.1), até 15
  stripStart: number                                  // índice na aba de games[0]; o tema não recalcula a janela
  focused: BigBoxFocused | null                       // null enquanto loading
  videoVisible: boolean; sound: boolean
  hints: Array<{ button: string; label: string }>
  alert: string | null
}
```

· alterado em 2026-11-16: `loading` e `stripStart` novos; `games` segue §3.1 (antes, foco − 7 .. foco + 7).

## 5. Contrato de tema

```ts
type BigBoxThemeProps = {
  model: BigBoxViewModel
  details: JSX.Element            // BigBoxDetails comum, mostrado quando screen = 'details'
  onPick: (index: number) => void  // clique numa capa
  onPlay: (index: number) => void  // duplo clique / botão Jogar
  onDetails: () => void
}
type BigBoxTheme = {
  id: BigBoxThemeId
  name: string                     // "Faixa", "Mosaico", "Roda"
  available: boolean               // só faixa nesta fase
  axis: 'horizontal' | 'vertical'
  listCapacity: (viewport: { width: number; height: number }) => number // itens inteiros na lista (§3.1); pura [BBX-024, BBX-039]
  Component?: (p: BigBoxThemeProps) => JSX.Element
}
```

O tema não lê bindings, não assina eventos e não guarda estado de navegação; só desenha o modelo e repassa cliques. Temas
indisponíveis (Mosaico, Roda) declaram `listCapacity: () => STRIP_FALLBACK_CAPACITY`. · alterado em 2026-11-16: `listCapacity`.

## 6. Tokens do Faixa (`index.css`, `[data-bigbox-theme="faixa"]`)

| Token | Uso |
|---|---|
| `--arc-bb-bg` | fundo sem arte (base do gradiente) |
| `--arc-bb-bg-2` | segunda cor do gradiente sem arte |
| `--arc-bb-scrim` | escurece o preenchimento desfocado do fundo (BBX-040); cor com alfa |
| `--arc-bb-panel` | painel atrás de todo texto (BBX-037); escuro com alfa (sugestão: alfa ≥ 0,8, valor final o que passar em BBX-026) |
| `--arc-bb-text` | título, linhas de informação, abas ativas |
| `--arc-bb-text-muted` | descrição curta, abas inativas, rodapé, borda da miniatura selecionada sem foco |
| `--arc-bb-accent` | botão Jogar (fundo) |
| `--arc-bb-on-accent` | texto do botão Jogar |
| `--arc-bb-surface` | botão Detalhes, selo de sessão, faixa de aviso, fundo das capas e miniaturas em `contain` |
| `--arc-bb-focus` | contorno de foco (capa, botões, fileira de miniaturas) |
| `--arc-bb-danger` | texto do aviso de erro |

Pares conferidos por BBX-026 (texto sobre `panel` composto sobre `#fff` e sobre `#000`, ≥ 4,5:1): `text`, `text-muted`,
`danger`; `on-accent` sobre `accent` (≥ 4,5:1); `text` sobre `surface` (≥ 4,5:1); `focus` contra `panel` composto (≥ 3:1).
· alterado em 2026-11-16: `panel` novo e base dos pares de texto (antes `scrim`, que não era desenhado atrás do texto); o alfa
mínimo do `scrim` deixa de ser exigido.

Área segura [BBX-042] (não é cor): `--arc-bb-safe-y: 5vh` e `--arc-bb-safe-x: 5vw` no mesmo bloco, usados por `data-bb-safe`.

## 7. Tempos e limites

| Constante | Valor | Requisito |
|---|---|---|
| `VIDEO_DELAY_MS` | 2000 | BBX-011 |
| `VIDEO_FADE_MS` | 400 | BBX-011 |
| `ART_FADE_MS` | 300 | BBX-010 |
| `STRIP_ANCHOR` | 2 (posição do foco na janela, longe das pontas) | BBX-039 |
| `STRIP_MAX` | 15 nós no DOM (capacidade até 14 + 1 cortada) | BBX-006, BBX-039 |
| `STRIP_FALLBACK_CAPACITY` | 12 (janela de tamanho desconhecido) | BBX-039 |
| ~~`STRIP_WINDOW`~~ | removido em 2026-11-16 (substituído por §3.1) | — |
| ~~`PREFETCH_BEFORE_END`~~ | removido em 2026-11-16 (BBX-007 removido) | — |
| `PAGE_SIZE` | 500 (= `x-limits.maxGameListLimit`, `domain.MaxPageLimit`); alterado em 2026-11-16, antes 200 | BBX-038 |
| `SAFE_AREA` | 5vh em cima e embaixo, 5vw nas laterais | BBX-042 |
| `THUMB_HEIGHT` | 10vh (miniaturas da galeria, 16:9) | BBX-043 |
| `FILL_BLUR_PX` | ≥ 24 | BBX-040 |
| `SHORT_DESCRIPTION_MAX` | 280 | BBX-009 |
| `REPEAT_DELAY_MS` / `REPEAT_EVERY_MS` | 400 / 120 | §2 |
| `STICK_DEAD_ZONE` | 0,6 | §2 |
