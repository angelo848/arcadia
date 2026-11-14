# Fase 10 — Nova interface: design

- **Requisitos:** [requirements.md](requirements.md) (rascunho, 2026-11-14) · **Tarefas:** `tasks.md` (o planner gera)
- **Contratos:** [contracts/selecao-do-jogo.md](contracts/selecao-do-jogo.md) (jogo em foco e painel). O contrato IPC
  continua o da fase 9 ([v0.11.0](../phase-9-plataformas/contracts/ipc.openrpc.json)), sem mudança (UIX-047).
- **ADRs:** 0014 (dois processos), 0015 (agente único escritor), 0019 (lançamento sem rastrear). Nenhum é contrariado.
  Propostos: 0028 (três colunas, fim da página do jogo) e 0029 (tema por tokens), ver §12.

## 0. Estado atual (conferido no código)

- `cmd/arcadia/main_windows.go`: `options.App{Width: 1024, Height: 768}`, sem `WindowStartState`, sem `MinWidth`/
  `MinHeight`. As opções são montadas dentro de `runUI`, sem função testável. `showWindow` (segunda instância) faz
  `WindowUnminimise` + `WindowShow`.
- `App.tsx`: cabeçalho (Estatísticas, Provedores, Emuladores, Plataformas, Importar, indicador), `SessionPanel` global,
  e um corpo que alterna entre `StatsPage`, `GameScreen` (`GamePage` ou `GameEditForm` + `DeleteDialog`) e a biblioteca
  (`ViewToggle`, `SearchBox`, `SortSelect`, `FilterPanel` em linha, `LibraryStates`, `LibraryGrid`/`LibraryList`).
  `selectedId` é o jogo da página aberta.
- `GameCard`: capa opcional (sem placeholder), título como `role="link"` (abre a página), plataforma, tempo, marcação e
  os botões "Jogar"/"Sem contar tempo". A grade usa `CARD_MIN_WIDTH = 220`, `CARD_HEIGHT = 180`.
- `FilterPanel`: selects em linha; plataforma é um `<select>` de uma plataforma (com PLT-032).
- `GamePage`: dados do jogo, avisos de rastreio por processo e de emulador, "Buscar mídia", "Editar", "Remover", imagens
  em grade e `SessionsList`.
- Modais: cada diálogo tem o próprio contêiner (`max-w-2xl`/`max-w-lg`/`max-w-md`, `max-h-full overflow-y-auto`); só
  `DeleteDialog` e `ImportDialog` prendem o foco; a lista de candidatos da importação tem `max-h-72`.
- `index.css`: só `@import "tailwindcss"` e `.big-box { font-size: 1.25rem }`. As cores são classes de paleta
  (`neutral-*`, `emerald-*`, `sky-*`, `amber-*`, `red-*`) espalhadas pelos componentes.
- `useSessionEvents`: no fim de uma sessão, invalida as listas de jogos e as estatísticas, mas não `libraryKeys.game(G)`
  nem `libraryKeys.sessions(G)` (lacuna corrigida por UIX-031).
- Big Box (`lib/bigbox.ts`): os itens navegáveis são `[data-bigbox-item]`; o cartão é o `<article>` e seus itens (título,
  Jogar, Sem contar tempo); Y procura `[data-bigbox-play]` dentro do `<article>`. `SearchBox` e `FilterPanel` não têm
  `data-bigbox-item` (lacuna de BIG-003).

## 1. Arquivos e responsabilidades

| Arquivo | Muda | Requisitos |
|---|---|---|
| `cmd/arcadia/window.go` (novo, sem build tag) | `applyWindowOptions(*options.App)` | UIX-001, UIX-002 |
| `cmd/arcadia/main_windows.go` | chama `applyWindowOptions` | UIX-001, UIX-002 |
| `frontend/src/index.css` | tokens do tema, `@theme inline`, foco visível, classes de tamanho de modal | UIX-006, UIX-007, UIX-037, UIX-040 |
| `frontend/index.html` | `<html data-theme="dark">` | UIX-006 |
| `App.tsx` | layout em três colunas; estado do jogo em foco pelo reducer; sem `GameScreen` | UIX-004, UIX-017..UIX-023, UIX-045, UIX-048 |
| `components/Sidebar.tsx` (novo) | busca + `PlatformNav` + `FilterPanel` | UIX-008..UIX-012 |
| `components/PlatformNav.tsx` (novo) | "Todos" e plataformas com ícone e contagem | UIX-009, UIX-010, UIX-042 |
| `components/FilterPanel.tsx` | sem o select de plataforma; seção recolhível "Filtros (N)" | UIX-011, UIX-012 |
| `components/LibraryToolbar.tsx` (novo) | contador, ordenação, alternância | UIX-013 |
| `components/GameCard.tsx` | capa/placeholder + título + selo; `role="option"` | UIX-014, UIX-015, UIX-042 |
| `components/CoverImage.tsx` (novo) | imagem com troca para o placeholder em `error` | UIX-015, UIX-024, UIX-026 |
| `components/LibraryGrid.tsx` | listbox com tabindex móvel, teclado, seleção, rolagem até o jogo em foco | UIX-016..UIX-019, UIX-021 |
| `components/LibraryList.tsx` | linhas sem botões, mesmas regras da grade | UIX-020 |
| `components/GameDetailsPanel.tsx` (novo; substitui `GamePage`) | cabeçalho, carrossel, botões, tempo, metadados, sessões, ações | UIX-025..UIX-036 |
| `components/MediaCarousel.tsx` (novo) | carrossel | UIX-026 |
| `components/PanelEmpty.tsx` (novo) | resumo sem jogo em foco | UIX-024 |
| `components/Modal.tsx` (novo) | contêiner comum dos modais | UIX-037..UIX-039 |
| `components/GameEditDialog.tsx` (novo) | `Modal` média + `GameEditForm` | UIX-032 |
| `components/{Import,Emulators,Platforms,Metadata,ProviderKeys,Delete}Dialog.tsx` | passam a usar `Modal` | UIX-037..UIX-039 |
| `components/GamePage.tsx`, `GamePage.test.tsx` | removidos (testes migram para o painel) | UIX-048 |
| `lib/selection.ts` (novo) | reducer do contrato | UIX-017, UIX-021..UIX-024, UIX-034, contrato |
| `lib/gridNav.ts` (novo) | vizinho por tecla, colunas por largura | UIX-005, UIX-019 |
| `lib/uiPrefs.ts` (novo; absorve `libraryView.ts`) | preferências locais: visualização, seção "Filtros" | UIX-012, UIX-020 |
| `lib/library.ts` | `useRecentGames()` | UIX-024 |
| `lib/games.ts` | `useSessionEvents` invalida detalhe e sessões do jogo | UIX-031 |
| `lib/bigbox.ts` | cartão como item único, saída pela borda da grade, Y pelo id do cartão | UIX-044 |
| `assets/cover-placeholder.svg` (novo) | placeholder padrão | UIX-015 |

`internal/` não muda. Nenhum binding novo (UIX-047).

## 2. Janela [UIX-001..UIX-003]

```go
// cmd/arcadia/window.go — no build tag: options is portable, so the test runs anywhere.
const (
    windowWidth     = 1280 // restored size [UIX-002]
    windowHeight    = 800
    windowMinWidth  = 1024
    windowMinHeight = 640
)

// applyWindowOptions sets the size and the start state of the main window [UIX-001, UIX-002].
func applyWindowOptions(o *options.App) {
    o.Width, o.Height = windowWidth, windowHeight
    o.MinWidth, o.MinHeight = windowMinWidth, windowMinHeight
    o.WindowStartState = options.Maximised
    o.Fullscreen = false
}
```

`runUI` monta `options.App` como hoje e chama `applyWindowOptions(&opts)` antes de `wails.Run`. Teste:
`window_test.go` confere os cinco campos.

- **Segunda instância (UIX-003):** `showWindow` continua `WindowUnminimise` + `WindowShow`; não chama
  `WindowMaximise`, para respeitar o tamanho escolhido pelo usuário.
- **Saída do Big Box (UIX-003):** `WindowUnfullscreen` do Wails no Windows restaura o posicionamento anterior da janela
  (inclusive maximizada). Conferência manual; se não restaurar, `exitBigBox` guarda `WindowIsMaximised()` antes de entrar e
  chama `WindowMaximise()` ao sair (só leitura e estado de janela; nada de tempo).
- **Risco:** com `WindowStartState: Maximised` o WebView2 pode pintar um quadro no tamanho restaurado antes de maximizar.
  Aceitável; conferido na verificação manual.

## 3. Tema [UIX-006, UIX-007]

Os valores ficam sob o atributo do tema; o Tailwind 4 os expõe como utilitários por `@theme inline` (que referencia a
variável em vez de copiar o valor, permitindo trocar o tema em tempo de execução):

```css
@import "tailwindcss";

[data-theme="dark"] {
  --arc-bg: #16181d;
  /* ... um valor por token da tabela ... */
}

@theme inline {
  --color-app: var(--arc-bg);
  --color-surface: var(--arc-surface);
  /* ... um utilitário por token ... */
}
```

| Token | Utilitário | Uso | Escuro |
|---|---|---|---|
| `--arc-bg` | `app` | fundo do app e da coluna do centro | `#16181d` |
| `--arc-surface` | `surface` | colunas laterais, cabeçalho, modais | `#1e2128` |
| `--arc-surface-2` | `raised` | campos, botões secundários, cartão em repouso | `#272b34` |
| `--arc-surface-3` | `hover` | hover de itens e botões secundários | `#323744` |
| `--arc-border` | `line` | bordas e divisórias | `#343a46` |
| `--arc-text` | `fg` | texto principal | `#e6e8ec` |
| `--arc-text-muted` | `muted` | texto secundário, rótulos | `#9aa3b2` |
| `--arc-accent` | `accent` | jogo em foco, item atual, botão primário | `#1f6feb` |
| `--arc-accent-hover` | `accent-hover` | hover do primário | `#388bfd` |
| `--arc-on-accent` | `on-accent` | texto sobre `accent`, `play` e `danger-strong` | `#ffffff` |
| `--arc-play` | `play` | botão "Jogar" | `#238636` |
| `--arc-play-hover` | `play-hover` | hover de "Jogar" | `#2ea043` |
| `--arc-running` | `running` | "Em execução" | `#3fb950` |
| `--arc-paused` | `paused` | "Pausado" | `#58a6ff` |
| `--arc-warning` | `warning` | avisos (emulador, carência) | `#d29922` |
| `--arc-danger` | `danger` | texto de erro e de "Remover" | `#ff6b64` (ajustado na T02: `#f85149` dava 4,23:1 sobre `raised`, abaixo dos 4,5:1 de UIX-007) |
| `--arc-danger-strong` | `danger-strong` | botão de confirmação destrutiva | `#da3633` |
| `--arc-focus` | `focus` | contorno de foco | `#79b8ff` |
| `--arc-overlay` | `overlay` | fundo atrás dos modais | `rgb(0 0 0 / 0.6)` |
| `--arc-placeholder` | `placeholder` | fundo do placeholder de capa | `#2a2f3a` |

Pares conferidos por UIX-007 (texto ≥ 4,5:1; foco ≥ 3:1): `fg` e `muted` sobre `app`, `surface`, `raised` e `hover`;
`on-accent` sobre `accent`, `play` e `danger-strong`; `running`, `paused`, `warning` e `danger` sobre `surface` e
`raised`; `focus` sobre `app`, `surface` e `raised`. Os valores acima foram escolhidos para passar (ex.: branco sobre
`#1f6feb` ≈ 4,6:1); o teste é a fonte da verdade, e um valor que falhar é ajustado no design antes do código.

- O teste de UIX-006 (`theme.test.ts`) varre `frontend/src/**/*.tsx` (sem `*.test.tsx`) com o padrão
  `\b(bg|text|border|ring|outline|from|via|to|fill|stroke|divide|placeholder|accent|caret|decoration|shadow)-(slate|gray|zinc|neutral|stone|red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose|black|white)\b`
  e por cores literais (`#[0-9a-f]{3,8}\b`, `rgb\(`, `hsl\(`); lê `index.css` e confere que cada token da tabela tem valor
  sob `[data-theme="dark"]` e utilitário em `@theme inline`.
- O mesmo teste calcula o contraste (luminância relativa WCAG 2.x) dos pares a partir dos valores de `index.css`.
- `.big-box` continua só aumentando a fonte; usa os mesmos tokens.
- Foco visível (UIX-040): regra global `:focus-visible { outline: 2px solid var(--arc-focus); outline-offset: 2px; }`;
  componentes não usam `outline-none` (o teste de varredura proíbe `outline-none` sem `focus-visible:` no mesmo
  `className`).

## 4. Layout [UIX-004, UIX-005, UIX-013]

```
<main class="flex h-screen flex-col overflow-hidden bg-app text-fg" data-theme raiz no <html>>
  <header> (como hoje) </header>
  {AgentConflict} {ErrorBanner} {SessionPanel}         ← inalterados (UI-008, UI-012)
  {estatísticas abertas ? <StatsPage/> : (
  <div class="grid min-h-0 flex-1" style="grid-template-columns: var(--col-left) 1fr var(--col-right)">
    <nav aria-label="Filtros da biblioteca" class="overflow-y-auto">  <Sidebar/> </nav>
    <section aria-label="Jogos" class="flex min-h-0 flex-col">
      <LibraryToolbar/>  <LibraryStates/>  <LibraryGrid|LibraryList/>
    </section>
    <aside aria-label="Detalhes do jogo" class="overflow-y-auto">  <GameDetailsPanel|PanelEmpty/> </aside>
  </div>)}
</main>
```

- `--col-left: clamp(220px, 16vw, 300px)` e `--col-right: clamp(320px, 26vw, 480px)` em `index.css`.
- Grade: `CARD_MIN_WIDTH = 160`, `GAP = 16`; altura da linha = largura do cartão × 3/2 (capa 2:3) + 2 linhas de título
  (≈ 44 px). Como a altura depende da largura, `columnsFor(width)` passa a devolver `{cols, cardWidth, rowHeight}`
  (`lib/gridNav.ts`) e o virtualizador usa `rowHeight` como `estimateSize` (refeito no `ResizeObserver`). Em 1024 de
  janela o centro tem 1024 − 220 − 320 = 484 px → 2 colunas; em 1920, 1920 − 300 − 480 = 1140 px → 6 colunas
  (UIX-005 pede ≥ 5).
- `StatsPage` continua substituindo as três colunas enquanto aberta (como hoje substitui a biblioteca); o jogo em foco é
  mantido (UIX-022).

## 5. Jogo em foco [UIX-017..UIX-024, UIX-034, UIX-045]

```ts
// lib/selection.ts — pure reducer of contracts/selecao-do-jogo.md.
export type Selection =
  | { kind: 'empty' }
  | { kind: 'loading'; gameId: number }
  | { kind: 'shown'; gameId: number }
  | { kind: 'error'; gameId: number; message: string }

export type SelectionEvent =
  | { type: 'select'; gameId: number }
  | { type: 'readOk'; gameId: number }
  | { type: 'readMissing'; gameId: number }
  | { type: 'readError'; gameId: number; message: string }
  | { type: 'retry' }
  | { type: 'removed'; gameId: number }

export function selectionReducer(s: Selection, e: SelectionEvent): Selection
export function focusedGameId(s: Selection): number | null
```

- O reducer só decide o estado; os efeitos (leituras, `LaunchGame`, recarga da grade) ficam em `App` e nos hooks, para
  o reducer continuar puro e testável pela tabela do contrato. `filtroMudou`, `trocaDeTela` e `sessaoTerminou` não são
  eventos do reducer (não mudam o estado; o contrato documenta a ausência de efeito no painel).
- `loading`/`shown`/`error` vêm de `useGame(gameId)` (`isPending`, `data`, `error`). O código de erro `-32001` chega como
  mensagem de `DSK-011`; `lib/library.ts` ganha `isNotFound(err)` que reconhece o erro de jogo inexistente pelo código
  que o binding já devolve (o planner confere no binding como o código chega; se só chegar o texto, a comparação é pelo
  texto fixo de DSK-011 para `-32001`).
- Respostas atrasadas: `useGame` é por chave `['library','game',id]`, então a resposta de outro jogo nunca entra no painel
  do jogo em foco (invariante 4 do contrato).
- **Clique e duplo clique (UIX-017, UIX-018):** `onClick` → `select`; `onDoubleClick` → `select` + `play(id)`. O
  navegador dispara `click`, `click`, `dblclick`: os dois `select` do mesmo id não têm efeito (mesma linha do
  contrato) e o lançamento acontece só no `dblclick`. `play` mantém a guarda de pendência de hoje (`pending !== null`).
- **Preservado ao filtrar (UIX-021):** `selection` fica em `App`, fora da chave da grade. A grade continua remontada por
  `key={queryKey}`; ao montar, se `selectedId` está em `games`, chama `virtualizer.scrollToIndex(row, {align: 'center'})`
  uma vez; quando uma página nova traz o jogo em foco, não rola de novo (só marca), para não puxar a rolagem do usuário.
- **Fim de sessão (UIX-031):** em `useSessionEvents`, no `ended`/`discarded` do jogo J, além do que já faz,
  `invalidateQueries(libraryKeys.game(J))` e `invalidateQueries(libraryKeys.sessions(J))`. Só leituras; a regra vale
  para qualquer J (o cache de um jogo fora do painel só é relido quando for usado).
- **Remoção (UIX-034):** `confirmRemove` no sucesso despacha `removed`; o foco do teclado vai para a grade (o botão que
  abriu a confirmação deixou de existir).
- **Estatísticas (UIX-045):** `onOpenGame(id)` fecha a página e despacha `select(id)`; no Big Box, depois do render, foca
  `[data-bigbox-play]` do painel.

### Resumo sem jogo em foco [UIX-024]

```ts
// lib/library.ts
export function useRecentGames(): UseQueryResult<GameView[], Error>
// ListGames({ filter: { version: 1, played: 'played' }, sort: 'lastPlayed', offset: 0, limit: 5 })
```

Chave própria sob `libraryGamesKey` (`['library','games','recent']`), para ser recarregada junto com a grade nas
escritas e no fim de sessão (UI-042, UI-006).

## 6. Grade, lista e teclado [UIX-014..UIX-020, UIX-041..UIX-043]

```ts
// components/GameCard.tsx
export function GameCard(props: {
  game: GameView
  coverUrl?: string
  live: LiveMark
  selected: boolean
  tabbable: boolean          // roving tabindex: only one card has tabIndex 0
  onSelect: (id: number) => void
  onLaunch: (id: number) => void
}): JSX.Element

// components/LibraryGrid.tsx
export function LibraryGrid(props: {
  games: GameView[]
  covers?: Record<number, string>
  total: number
  current: SessionView | null
  selectedId: number | null
  keyboard: boolean          // false in the Big Box: useBigBoxNav owns the arrows [UIX-044]
  onNeedMore: () => void
  onSelect: (id: number) => void
  onLaunch: (id: number) => void
}): JSX.Element

// lib/gridNav.ts
export type NavKey = 'ArrowLeft' | 'ArrowRight' | 'ArrowUp' | 'ArrowDown' | 'Home' | 'End'
export function nextIndex(index: number, key: NavKey, cols: number, loaded: number): number
export function columnsFor(width: number): { cols: number; cardWidth: number; rowHeight: number }
```

- O cartão é o próprio elemento focável: `<div role="option" aria-selected data-game-id tabIndex={tabbable ? 0 : -1}>`
  dentro de `<div role="listbox" aria-label="Jogos">`. `pending` sai do cartão (não há mais botões nele).
- **Tabindex móvel:** o cartão com `tabIndex = 0` é o do jogo em foco, se carregado, senão o primeiro. Como a grade é
  virtualizada, o cartão focável pode não estar no DOM; ao receber o foco pelo Tab, o contêiner (`tabIndex = -1` quando há
  cartão focável no DOM, `0` quando não há) passa o foco para o cartão certo depois de `scrollToIndex`.
- **Teclado (UIX-019):** `onKeyDown` no listbox (só com `keyboard = true`): `nextIndex` → `onSelect(games[i].id)`,
  `scrollToIndex`, foco no cartão depois do render (`requestAnimationFrame` não; usar `useLayoutEffect` ao mudar o índice
  focado, para não depender de tempo). Enter → `onLaunch`. A prefetch de UI-036 continua pelo efeito da janela visível.
- `nextIndex` (tabela de casos no teste): direita na última coluna da linha fica; baixo na última linha fica; baixo para
  uma linha mais curta vai ao último cartão dela; Home → 0; End → `loaded - 1`.
- **Selo de sessão (UIX-014):** `liveOf` não muda; o selo fica no canto da capa com fundo `surface` e texto `running` ou
  `paused`.
- **Lista (UIX-020):** linha `role="option"` com título, plataforma, `formatPlayTime`, última vez (`formatLocalDateTime`) e
  selo; mesmas props da grade (`nextIndex` com `cols = 1`).
- **Placeholder (UIX-015):** `CoverImage({src, alt: ''})` mostra `cover-placeholder.svg` (fundo `--arc-placeholder`,
  silhueta de controle) quando `src` é vazio ou no evento `error`; a área tem `aspect-ratio: 2 / 3` e `object-fit:
  contain`, então a capa chega sem mudar a altura. O SVG é importado como URL pelo Vite (embutido no executável, sem rede).

## 7. Coluna esquerda [UIX-008..UIX-012]

```ts
// components/PlatformNav.tsx
export function PlatformNav(props: {
  platforms: PlatformView[] | undefined
  selectedIds: number[] | undefined     // query.filter.platformIds
  onChange: (platformIds: number[] | undefined) => void
}): JSX.Element

// lib/uiPrefs.ts — local preferences, never sent to the agent (UI-030).
export function readLibraryView(storage?): LibraryView
export function writeLibraryView(v: LibraryView, storage?): void
export function readFiltersOpen(storage?): boolean          // default false [UIX-012]
export function writeFiltersOpen(open: boolean, storage?): void
export function activeFacetCount(f: FilterSpec): number     // excludes version, text, platformIds
```

- Itens: `<button aria-current={atual ? 'true' : undefined} aria-label="Nome, N jogos">` com `PlatformIcon` (`alt=""`),
  nome e o número. "Todos" soma `gameCount` das plataformas listadas. A regra de PLT-032 sai do `FilterPanel` e vai para
  `PlatformNav`.
- Com `platformIds` de mais de um id (vindo de fora desta tela), nenhum item fica atual; clicar num item troca por um só.
- `FilterPanel` perde o select de plataforma e ganha o cabeçalho recolhível; `withFacet` e os demais controles não
  mudam (mesmo `FilterSpec`, UIX-011). "Limpar filtros" continua `onChange(resetQuery())`, o que já zera plataforma,
  texto e ordenação.
- `lib/libraryView.ts` vira reexportação de `uiPrefs.ts` (ou é removido e os testes de UI-030 passam a importar de
  `uiPrefs`); a chave `arcadia.libraryView` não muda, para a preferência gravada continuar valendo.

## 8. Painel [UIX-025..UIX-036]

```ts
// components/GameDetailsPanel.tsx
export function GameDetailsPanel(props: {
  gameId: number
  current: SessionView | null
  pending: PendingLaunch
  onPlay: (id: number) => void
  onPlayUntracked: (id: number) => void
  onEdit: () => void
  onSearchMetadata: () => void
  onRemove: () => void
  onMissing: (id: number) => void       // -32001 → selection 'readMissing' [UIX-023]
}): JSX.Element

// components/MediaCarousel.tsx
export function MediaCarousel(props: { items: MediaItemView[] | undefined; resetKey: number }): JSX.Element
```

- Ordem das seções: cabeçalho (título `h2`, plataforma com ícone, marcação, avisos) → carrossel → "Jogar" / "Jogar sem
  contar tempo" → tempo jogado → metadados → sessões (`SessionsList` com `current={null}`, como hoje; o cronômetro é o
  `SessionPanel` global) → ações (Editar, Buscar metadados, Remover).
- Os rótulos (`statusLabels`, `mediaKindLabels`, `Field`, `ListValue`) saem de `GamePage.tsx` para o painel sem mudar o
  texto.
- Carrossel: índice local, `resetKey = gameId` reinicia em 0; a capa é posta primeiro (filtro estável: capas, depois o
  resto na ordem recebida); usa `url` (imagem cheia), não `thumbUrl`; `aspect-ratio: 16 / 9`, `object-fit: contain`;
  `aria-roledescription="carrossel"`, região com nome "Imagens", indicador "k de n" em `aria-live="polite"`.
- Rolagem (UIX-036): o `<aside>` é o elemento que rola; `useLayoutEffect` em `gameId` zera `scrollTop`.
- `MetadataDialog` já invalida `game(G)`, as listas e `media(G)` ao aplicar (UIX-033 já se cumpre); só muda o contêiner
  para `Modal` média.
- Editar (UIX-032): `GameEditDialog` = `Modal size="medium" title="Editar jogo"` com o `GameEditForm` atual (o form
  continua dono dos botões Salvar/Cancelar; o rodapé do `Modal` recebe esses botões por prop ou o form os renderiza no
  slot de rodapé). Esc = Cancelar (UIX-039).

## 9. Modais [UIX-037..UIX-039]

```ts
// components/Modal.tsx
export type ModalSize = 'large' | 'medium' | 'compact'
export function Modal(props: {
  size: ModalSize
  title: string
  onClose: () => void
  closeDisabled?: boolean           // Esc does nothing while true
  initialFocus?: RefObject<HTMLElement>
  footer?: ReactNode
  children: ReactNode               // the scrolling body
}): JSX.Element
```

- Estrutura: `div.fixed.inset-0.bg-overlay` > `div[role=dialog][aria-modal][aria-labelledby][data-size]` com
  `flex flex-col` e altura da classe; `header` (título) e `footer` com `shrink-0`; corpo `min-h-0 flex-1 overflow-y-auto`.
- Classes em `index.css`: `.modal-large { width: min(92vw, 1600px); height: 88vh }`, `.modal-medium { width: min(75vw,
  1100px); height: 80vh }`, `.modal-compact { width: clamp(420px, 40vw, 560px); max-height: 80vh }`. Na janela mínima
  (1024 × 640): grande 942 × 563, média 768 × 512, compacta 420 × até 512.
- Foco: o código de trap de `DeleteDialog`/`ImportDialog` vira o do `Modal` (um só). Guarda o `document.activeElement` ao
  abrir e devolve no fechamento; se o elemento não está mais no documento, foca o listbox da grade.
- Clique no overlay não fecha (evita perder o que foi digitado na importação ou na edição).
- `ImportDialog`: a lista de candidatos perde `max-h-72` e ocupa o corpo (`flex-1`); o resto não muda (IMP-010).

## 10. Big Box [UIX-044]

Mudanças em `lib/bigbox.ts`, sem visual novo:

- O cartão é um item único: `[data-bigbox-item][data-bigbox-card][data-game-id]` no `role="option"`. `cardGrid` passa a
  agrupar por `[data-bigbox-card]` em vez de `article`, e cada cartão tem um item.
- `gridNext` na borda (sem vizinho naquela direção) devolve `null`; `nextFocus` então usa `pickNext` (geometria) com
  todos os itens, o que leva ao painel (direita) ou à coluna esquerda (esquerda). Itens novos com `data-bigbox-item`:
  busca, itens de `PlatformNav`, cabeçalho e controles de `FilterPanel`, botões do painel e do carrossel.
- `useBigBoxNav({enabled, pageKey, onBack, onPlay})`: Y procura o `[data-bigbox-card]` mais próximo do foco e chama
  `onPlay(Number(dataset.gameId))` (no lugar de clicar `[data-bigbox-play]` do `article`). `[data-bigbox-play]` passa a
  marcar o "Jogar" do painel, usado para levar o foco depois de A num cartão.
- A num cartão: o `click()` atual dispara `onSelect`; `App`, com `bigBox` ativo, foca `[data-bigbox-play]` do painel
  depois do render do detalhe (efeito em `selection.kind === 'shown'` com uma marca "focar Jogar" pendente).
- B: `App.bigBoxBack` decide pelo foco atual: dentro do `<aside>` → foca o cartão `[data-game-id=G]` (rolando até ele;
  se não está carregado, foca o listbox); estatísticas abertas → fecha; senão → sai do Big Box.
- `pageKey` deixa de incluir o jogo (não há página do jogo): `library` ou `stats`.
- **Risco (teclado em dobro):** fora do Big Box o listbox trata as setas; no Big Box o `window` trata. `LibraryGrid`
  recebe `keyboard={!bigBox}` e o handler de `useBigBoxNav` continua no `window` com `preventDefault`. Teste cobre que
  no Big Box uma seta move o foco uma vez só.

## 11. Invariantes e riscos [UIX-046..UIX-048]

- **Tempo de jogo (UIX-046):** os únicos pontos que chamam `LaunchGame`/`LaunchGameUntracked` são `play` e
  `playUntracked` em `App` (como hoje), chamados por: botões do painel, `onLaunch` do cartão/linha (duplo clique,
  Enter) e `onPlay` do Big Box (Y). `ToggleSession` continua só no `SessionPanel`. O teste de UIX-046 monta `App` com
  todos os bindings como espiões, percorre seleção, setas, carrossel, filtros, seção recolhível, abrir/fechar modais e
  Big Box, e confere que só leituras foram chamadas; depois um duplo clique → exatamente um `LaunchGame`.
- **Sem IPC novo (UIX-047):** "Jogados recentemente" usa `ListGames` (existente) com `limit = 5`.
- **Página removida (UIX-048):** estender `library-removed.test.ts` com `GamePage`, `GameScreen`, o texto "Voltar" fora
  de `StatsPage` e "Detalhes".
- **Handles e goroutines:** a única mudança em Go é declarativa (`options.App`); nenhuma goroutine nem handle novo.
- **Concorrência na UI:** respostas atrasadas são descartadas pela chave do jogo (§5); a pendência de lançamento continua
  única (`pending`). O fim de uma sessão durante a edição invalida `game(G)`, mas o formulário trabalha sobre a cópia
  inicial (como hoje) e envia só o patch, então não perde o que foi digitado.
- **Desempenho:** `useCovers` continua com uma leitura por jogo carregado (limitação 3 dos requisitos); o painel adiciona
  três leituras por troca de jogo em foco, todas em cache depois da primeira.

## 12. ADRs propostos

- **0028 — Biblioteca em três colunas e fim da página do jogo** (jogo em foco como estado da UI, clique × duplo clique,
  painel no lugar da página, preservação do foco ao filtrar). Substitui a navegação de UI-037.
- **0029 — Tema por tokens em variáveis CSS** (`--arc-*` por `[data-theme]`, `@theme inline`, paleta proibida nos
  componentes com teste).
