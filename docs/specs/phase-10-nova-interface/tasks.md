# Fase 10 — Nova interface: tarefas

- **Status:** rascunho para aprovação do usuário (2026-11-14).
- **Requisitos:** [requirements.md](requirements.md) (aprovado; lista mantida; painel sem foco com resumo "Jogados recentemente") · **Design:** [design.md](design.md) · **Contrato:** [contracts/selecao-do-jogo.md](contracts/selecao-do-jogo.md). O contrato IPC continua o da fase 9 (v0.11.0).
- **ADRs:** 0014, 0015, 0019, 0028, 0029.

## Decisões do plano (registradas)

1. **Transições que faltavam no contrato** (acrescentadas): `readOk(G)` em `error(G)` vai para `shown(G)`; `readError(G)` em `shown(G)` não muda o estado (o TanStack Query mantém os dados antigos).
2. **Callback único do painel:** `GameDetailsPanel.onSelectionEvent` no lugar do `onMissing` do design §8; o comportamento não muda.
3. **`isNotFound` pelo texto:** o binding só devolve o texto do DSK-011 (`"Jogo não encontrado. Atualize a lista."`), não o código `-32001`.
4. **UIX-005:** o teste usa as larguras do requisito (484 px e 1132 px), não o 1140 px do design §4.
5. **Salvar/Cancelar da edição** vão ao rodapé do `Modal` por `createPortal` (`GameEditForm.actionsTarget`).
6. Chave da preferência dos filtros: `arcadia.filtersOpen`. `mediaKindLabels` vai para `lib/metadata.ts`. `libraryView.ts` vira reexportação de `uiPrefs.ts`.

## Premissas

- Uma tarefa por commit. Nas tarefas com testes: **test-writer** primeiro (testes novos vermelhos), **implementer** depois. Ao fim de cada tarefa: `go build ./...`, `go test ./...`, `npm test` e `tsc --noEmit` verdes.
- Quando uma tarefa muda o que testes antigos supõem, o test-writer os ajusta na mesma tarefa.
- Todo `.tsx` tocado já sai só com tokens do tema. As varreduras de UIX-006/040/041 entram na T21.
- `App.tsx` é tocado só pelas T17, T18, T19 e T20, nessa ordem. Nada toca `internal/` nem `cmd/fake-game/`.

## T01 — Janela maximizada com tamanho restaurado e mínimo
- Requisitos: UIX-001, UIX-002
- **implementer:** `cmd/arcadia/window.go` (novo, **sem build tag**); `main_windows.go`: `runUI` monta `opts := options.App{...}` sem `Width`/`Height`, chama `applyWindowOptions(&opts)` e passa `&opts` ao `wails.Run`. `showWindow` não muda (UIX-003).
```go
const (
    windowWidth     = 1280 // restored size [UIX-002]
    windowHeight    = 800
    windowMinWidth  = 1024
    windowMinHeight = 640
)
func applyWindowOptions(o *options.App) // [UIX-001, UIX-002]
```
- **test-writer:** `cmd/arcadia/window_test.go` (`TestApplyWindowOptions`):
  - `UIX-001/WindowStartState é Maximised e Fullscreen é falso`
  - `UIX-002/Width 1280, Height 800, MinWidth 1024, MinHeight 640`
- Dependência: nenhuma.

## T02 — Tokens do tema e CSS base
- Requisitos: UIX-006, UIX-007, UIX-005, UIX-037, UIX-040
- **implementer:** `frontend/src/index.css` — os 20 tokens do design §3 sob `[data-theme="dark"]`; `@theme inline` com um utilitário por token (`--color-app`, `surface`, `raised`, `hover`, `line`, `fg`, `muted`, `accent`, `accent-hover`, `on-accent`, `play`, `play-hover`, `running`, `paused`, `warning`, `danger`, `danger-strong`, `focus`, `overlay`, `placeholder`); `--col-left: clamp(220px, 16vw, 300px)`; `--col-right: clamp(320px, 26vw, 480px)`; `:focus-visible { outline: 2px solid var(--arc-focus); outline-offset: 2px; }`; `.modal-large/.modal-medium/.modal-compact` (medidas de UIX-037); `.big-box` mantido. `frontend/index.html`: `<html lang="pt-BR" data-theme="dark">`.
- **test-writer:** `frontend/src/theme.test.ts` (lê `index.css?raw` e `index.html?raw`; luminância/contraste WCAG como helper do teste):
  - `UIX-006/cada token da tabela tem valor sob [data-theme="dark"] e utilitário em @theme inline`
  - `UIX-006/index.html tem data-theme="dark" no elemento raiz`
  - `UIX-007/fg e muted ≥ 4,5:1 sobre app, surface, raised e hover`
  - `UIX-007/on-accent ≥ 4,5:1 sobre accent, play e danger-strong`
  - `UIX-007/running, paused, warning e danger ≥ 4,5:1 sobre surface e raised`
  - `UIX-007/focus ≥ 3:1 sobre app, surface e raised`
  - `UIX-040/regra global :focus-visible usa var(--arc-focus)`
  - `UIX-005/--col-left e --col-right com os clamp da spec`
  - `UIX-037/.modal-large, .modal-medium e .modal-compact com largura e altura da tabela`
- Dependência: nenhuma. Se um par falhar no contraste, o valor é ajustado no design.md (spec-writer) antes do código.

## T03 — Reducer do jogo em foco
- Requisitos: UIX-017, UIX-021, UIX-022, UIX-023, UIX-024, UIX-034, UIX-035
- **Arquivos:** `frontend/src/lib/selection.ts` (implementer), `selection.test.ts` (test-writer).
```ts
export type Selection =
  | { kind: 'empty' } | { kind: 'loading'; gameId: number }
  | { kind: 'shown'; gameId: number } | { kind: 'error'; gameId: number; message: string }
export type SelectionEvent =
  | { type: 'select'; gameId: number } | { type: 'readOk'; gameId: number }
  | { type: 'readMissing'; gameId: number } | { type: 'readError'; gameId: number; message: string }
  | { type: 'retry' } | { type: 'removed'; gameId: number }
export const initialSelection: Selection // { kind: 'empty' } [UIX-024]
export function selectionReducer(s: Selection, e: SelectionEvent): Selection
export function focusedGameId(s: Selection): number | null
```
- **Testes:**
  - `UIX-024/estado inicial é empty`
  - `UIX-017/select(H) de empty vai para loading(H)`
  - `UIX-017/select do mesmo jogo em loading, shown e error não muda o estado (mesma referência)`
  - `UIX-017/select(H≠G) em loading, shown e error vai para loading(H)`
  - `UIX-035/readOk(G) em loading(G) vai para shown(G)`
  - `UIX-035/readError(G) em loading(G) vai para error(G, msg); retry volta a loading(G)`
  - `UIX-035/retry fora de error não muda nada`
  - `UIX-022/readOk(G) em error(G) vai para shown(G)`
  - `UIX-022/readError(G) em shown(G) mantém shown(G)`
  - `UIX-023/readMissing(G) em loading(G) e em shown(G) vai para empty`
  - `UIX-021/readOk, readMissing e readError de J≠G são ignorados`
  - `UIX-034/removed(G) em shown(G) vai para empty; removed(J≠G) não muda nada`
  - `UIX-022/focusedGameId devolve G em loading, shown e error e null em empty`
- Dependência: nenhuma.

## T04 — Navegação e medidas da grade
- Requisitos: UIX-005, UIX-014, UIX-019, UIX-020
- **Arquivos:** `frontend/src/lib/gridNav.ts` (implementer), `gridNav.test.ts` (test-writer).
```ts
export const CARD_MIN_WIDTH = 160
export const GRID_GAP = 16
export const CARD_TITLE_HEIGHT = 44 // two title lines under the 2:3 cover
export type NavKey = 'ArrowLeft' | 'ArrowRight' | 'ArrowUp' | 'ArrowDown' | 'Home' | 'End'
export function isNavKey(key: string): key is NavKey
export function nextIndex(index: number, key: NavKey, cols: number, loaded: number): number
export function columnsFor(width: number): { cols: number; cardWidth: number; rowHeight: number }
```
- **Testes:**
  - `UIX-019/direita e esquerda vão ao vizinho; na primeira e na última coluna da linha ficam`
  - `UIX-019/baixo vai à mesma coluna; para uma linha mais curta vai ao último cartão dela; na última linha fica`
  - `UIX-019/cima vai à mesma coluna; na primeira linha fica`
  - `UIX-019/Home vai a 0 e End a loaded - 1`
  - `UIX-019/isNavKey aceita só as seis teclas`
  - `UIX-020/com cols = 1 cima e baixo movem uma linha e esquerda e direita ficam`
  - `UIX-005/columnsFor(484) dá 2 colunas e columnsFor(1132) dá pelo menos 5`
  - `UIX-005/columnsFor de largura menor que um cartão dá 1 coluna`
  - `UIX-014/rowHeight = cardWidth × 3/2 + CARD_TITLE_HEIGHT + GRID_GAP`
- Dependência: nenhuma.

## T05 — Preferências locais da UI
- Requisitos: UIX-012, UIX-020
- **implementer:** `frontend/src/lib/uiPrefs.ts` (novo); `lib/libraryView.ts` vira só reexportação (chave `arcadia.libraryView` não muda).
```ts
export type LibraryView = 'grid' | 'list'
export function readLibraryView(storage?: Pick<Storage, 'getItem'>): LibraryView
export function writeLibraryView(v: LibraryView, storage?: Pick<Storage, 'setItem'>): void
export function readFiltersOpen(storage?: Pick<Storage, 'getItem'>): boolean   // 'arcadia.filtersOpen', default false
export function writeFiltersOpen(open: boolean, storage?: Pick<Storage, 'setItem'>): void
export function activeFacetCount(f: FilterSpec): number                         // ignora version, text, platformIds
```
- **test-writer:** `frontend/src/lib/uiPrefs.test.ts`:
  - `UIX-020/readLibraryView e writeLibraryView usam a chave arcadia.libraryView (preferência antiga continua valendo)`
  - `UIX-012/readFiltersOpen é false sem valor gravado e com o storage indisponível`
  - `UIX-012/writeFiltersOpen(true) é lido de volta como true`
  - `UIX-012/activeFacetCount ignora version, text e platformIds`
  - `UIX-012/activeFacetCount conta cada faceta presente (gênero, modo, empresas, ano de/até, status, favorito false, jogado, tempo mín/máx)`
- Dependência: nenhuma.

## T06 — Leituras novas: "Jogados recentemente" e jogo inexistente
- Requisitos: UIX-023, UIX-024
- **implementer:** `frontend/src/lib/library.ts`:
```ts
// libraryKeys ganha: recent: ['library','games','recent'] as const  (sob libraryGamesKey)
export const gameNotFoundMessage = 'Jogo não encontrado. Atualize a lista.'
export function isNotFound(err: unknown): boolean
export function useRecentGames(): UseQueryResult<GameView[], Error>
// ListGames({ filter: { version: 1, played: 'played' }, sort: 'lastPlayed', offset: 0, limit: 5 })
```
- **test-writer:** `frontend/src/lib/library.test.tsx`:
  - `UIX-024/useRecentGames chama ListGames com played=played, sort lastPlayed, offset 0 e limit 5 e devolve os jogos`
  - `UIX-024/invalidar libraryGamesKey relê a lista recente`
  - `UIX-023/isNotFound reconhece o texto de -32001 e recusa outras mensagens e não-erros`
- Dependência: nenhuma.

## T07 — Recarga do detalhe no fim da sessão e na reconexão
- Requisitos: UIX-031, UIX-022
- **implementer:** `frontend/src/lib/games.ts` — `useSessionEvents`: no `ended`/`discarded` de J, também invalida `libraryKeys.game(J)` e `libraryKeys.sessions(J)`; `useReloadOnReconnect`: também invalida `['library','game']` e `['library','sessions']`, com o mesmo `predicate` de "já tem dados".
- **test-writer:** `frontend/src/lib/games.test.tsx`:
  - `UIX-031/ended de J relê GetGame(J) e GameSessions(J) uma vez`
  - `UIX-031/discarded de J relê GetGame(J) e GameSessions(J) uma vez`
  - `UIX-031/running e paused não releem o detalhe nem as sessões`
  - `UIX-031/fim de sessão de J não relê o detalhe de outro jogo G em cache`
  - `UIX-022/reconectar relê o detalhe e as sessões já carregados, sem ler os que nunca foram carregados`
- Dependência: nenhuma.

## T08 — Capa ou placeholder
- Requisitos: UIX-015, UIX-042
- **implementer:** `frontend/src/assets/cover-placeholder.svg` (cor fixa no próprio SVG, fora da regra de UIX-006; silhueta de controle, sem `<text>`); `frontend/src/components/CoverImage.tsx`.
```ts
export const coverPlaceholderUrl: string
export function CoverImage(props: { src?: string; ratio?: '2/3' | '16/9'; className?: string }): JSX.Element
// alt="" sempre; aspect-ratio por ratio (padrão 2/3); object-fit: contain
```
- **test-writer:** `frontend/src/components/CoverImage.test.tsx`:
  - `UIX-015/com src mostra a imagem com object-fit contain na área 2:3`
  - `UIX-015/src vazio ou ausente mostra o placeholder do mesmo tamanho`
  - `UIX-015/evento error da imagem troca pelo placeholder`
  - `UIX-015/trocar src depois de um erro volta a tentar a imagem nova`
  - `UIX-042/imagem e placeholder têm alt vazio`
- Dependência: nenhuma.

## T09 — Contêiner comum dos modais
- Requisitos: UIX-037, UIX-038, UIX-039
- **implementer:** `frontend/src/components/Modal.tsx`. O trap de foco de `DeleteDialog` e `ImportDialog` vira o do `Modal`.
```ts
export type ModalSize = 'large' | 'medium' | 'compact'
export function Modal(props: {
  size: ModalSize; title: string; onClose: () => void
  closeDisabled?: boolean              // Esc não faz nada enquanto true
  initialFocus?: RefObject<HTMLElement | null>
  footer?: ReactNode
  children: ReactNode                  // corpo com rolagem
}): JSX.Element
```
- **test-writer:** `frontend/src/components/Modal.test.tsx`:
  - `UIX-037/data-size e a classe modal-<size> para large, medium e compact`
  - `UIX-038/cabeçalho e rodapé com shrink-0 e corpo com overflow-y auto e flex-1`
  - `UIX-039/role dialog, aria-modal true e nome acessível pelo título`
  - `UIX-039/ao abrir, o foco vai ao primeiro controle; com initialFocus, ao elemento indicado`
  - `UIX-039/Tab no último volta ao primeiro e Shift+Tab no primeiro vai ao último`
  - `UIX-039/Esc chama onClose uma vez; com closeDisabled não chama`
  - `UIX-039/clicar no overlay não chama onClose`
  - `UIX-039/ao desmontar, o foco volta ao elemento que abriu; se ele saiu do documento, vai ao listbox "Jogos"`
- Dependência: T02.

## T10 — Diálogos existentes sobre o `Modal`
- Requisitos: UIX-037, UIX-038, UIX-039
- **implementer:** props públicas e `App.tsx` não mudam. `DeleteDialog` e `ProviderKeysDialog`: `compact` (`DeleteDialog` com `initialFocus` = "Cancelar", `closeDisabled` = false). `EmulatorsDialog`, `PlatformsDialog`, `MetadataDialog`: `medium`. `ImportDialog`: `large`; a lista perde `max-h-72` e ocupa o corpo (`flex-1 min-h-0`), trap próprio sai; Esc segue o botão de fechar e fica desativado durante a importação. Os seis arquivos saem só com tokens.
- **test-writer:** `frontend/src/components/modals.test.tsx` (tabela com os seis diálogos); ajustar só seletores dos testes existentes que dependiam de classes antigas:
  - `UIX-037/Importar é large; Emuladores, Plataformas e Buscar metadados são medium; Provedores e Remover jogo são compact`
  - `UIX-038/Importar: a lista de candidatos fica no corpo, sem max-h fixo`
  - `UIX-039/cada diálogo tem role dialog, aria-modal e nome pelo título`
  - `UIX-039/Esc em cada diálogo faz o mesmo que fechar ou Cancelar`
  - `UIX-039/Remover jogo abre com o foco em Cancelar`
  - `UIX-039/Importar em andamento: Esc não fecha`
- Dependência: T09.

## T11 — Lista de plataformas da coluna esquerda
- Requisitos: UIX-009, UIX-010, UIX-042, UIX-043
- **Arquivos:** `components/PlatformNav.tsx` (implementer), `PlatformNav.test.tsx` (test-writer).
```ts
export function PlatformNav(props: {
  platforms: PlatformView[] | undefined
  selectedIds: number[] | undefined      // query.filter.platformIds
  onChange: (platformIds: number[] | undefined) => void
}): JSX.Element
```
Título "Plataformas". Cada item é `<button aria-label="<nome>, N jogos">`, `aria-current="true"` no atual, `PlatformIcon` (`alt=""`), nome e número. Com mais de um id, nenhum item é o atual.
- **Testes:**
  - `UIX-009/"Todos" primeiro, com a soma de gameCount das plataformas listadas`
  - `UIX-009/plataforma com 0 jogos fica fora; a selecionada com 0 jogos fica`
  - `UIX-009/ordem do catálogo, cada item com ícone, nome e número`
  - `UIX-009/sem catálogo mostra só "Todos", sem número`
  - `UIX-010/clicar em P chama onChange([P]); clicar em Todos chama onChange(undefined)`
  - `UIX-010/clicar no item atual não chama onChange`
  - `UIX-010/sem filtro "Todos" tem aria-current; com [P], P tem aria-current; com dois ids, nenhum`
  - `UIX-042/nome acessível "<plataforma>, N jogos" e ícone com alt vazio`
  - `UIX-043/Enter e Espaço no item escolhem a plataforma`
- Dependência: nenhuma.

## T12 — Carrossel de imagens
- Requisitos: UIX-026, UIX-042, UIX-043
- **implementer:** `components/MediaCarousel.tsx`; `lib/metadata.ts` ganha `mediaKindLabels` e `orderMedia`.
```ts
export const mediaKindLabels: Record<MediaKind, string>
export function orderMedia(items: MediaItemView[]): MediaItemView[]   // capas primeiro, resto na ordem recebida (estável)
export function MediaCarousel(props: { items: MediaItemView[] | undefined; resetKey: number }): JSX.Element
```
Região "Imagens" com `aria-roledescription="carrossel"`; usa `CoverImage ratio="16/9"`, rótulo do tipo e "k de n" em `aria-live="polite"`.
- **test-writer:** `MediaCarousel.test.tsx`; casos de `orderMedia` em `lib/metadata.test.ts`:
  - `UIX-026/n = 0 ou items undefined mostra o placeholder, sem botões`
  - `UIX-026/n = 1 mostra a imagem, o rótulo e "1 de 1", sem botões`
  - `UIX-026/n = 3: a capa vem primeiro e Próxima imagem/Imagem anterior dão a volta nas pontas`
  - `UIX-026/setas esquerda e direita com o foco no carrossel trocam a imagem`
  - `UIX-026/mudar resetKey volta à primeira imagem`
  - `UIX-026/imagem que falha mostra o placeholder no lugar dela`
  - `UIX-026/orderMedia põe as capas primeiro e mantém a ordem das demais`
  - `UIX-042/botões com os nomes "Imagem anterior" e "Próxima imagem"`
  - `UIX-043/o carrossel é navegável só pelo teclado`
- Dependência: T08.

## T13 — Painel sem jogo em foco
- Requisitos: UIX-024
- **Arquivos:** `components/PanelEmpty.tsx` (implementer), `PanelEmpty.test.tsx` (test-writer).
```ts
export function PanelEmpty(props: { onSelect: (id: number) => void }): JSX.Element
// useRecentGames + useCovers; cada item: CoverImage pequena, título, formatLocalDateTime(lastPlayedAt)
```
- **Testes:**
  - `UIX-024/mostra "Nenhum jogo selecionado" e o texto de instrução`
  - `UIX-024/com jogos jogados lista até 5 em "Jogados recentemente" com capa ou placeholder, título e última vez`
  - `UIX-024/ListGames chamado com played=played, sort lastPlayed e limit 5`
  - `UIX-024/clicar num item chama onSelect(id) e nenhum binding de lançamento`
  - `UIX-024/resposta vazia (biblioteca vazia) não mostra a lista`
- Dependências: T06, T08.

## T14 — Edição em modal
- Requisitos: UIX-032, UIX-037
- **implementer:** `components/GameEditDialog.tsx` (novo); `GameEditForm.tsx`: prop opcional `actionsTarget` (Salvar/Cancelar por `createPortal` no rodapé do `Modal`; sem ela, como hoje); troca para tokens inclui tirar `focus:outline-none`; atualizar o comentário que cita `GamePage`.
```ts
// GameEditFormProps ganha: actionsTarget?: Element | null
export function GameEditDialog(props: {
  game: GameDetailView; genreSuggestions: string[]; emulators: EmulatorView[]
  pending: boolean; error: string | null
  onSave: (patch: Record<string, unknown>) => void; onCancel: () => void
}): JSX.Element   // Modal size="medium" title="Editar jogo"; Esc = onCancel
```
- **test-writer:** `GameEditDialog.test.tsx` (`GameEditForm.test.tsx` e `GameEditFormPick.test.tsx` continuam):
  - `UIX-032/abre como modal medium com o título "Editar jogo" e os campos de UI-039`
  - `UIX-032/Salvar e Cancelar ficam no rodapé do modal`
  - `UIX-032/Salvar chama onSave só com o patch alterado`
  - `UIX-032/com error, a mensagem aparece e os valores digitados ficam`
  - `UIX-032/Cancelar e Esc chamam onCancel sem onSave`
  - `UIX-032/título vazio desativa Salvar (UI-040)`
- Dependência: T09.

## T15 — Painel de detalhes do jogo
- Requisitos: UIX-023, UIX-025, UIX-026, UIX-027, UIX-028, UIX-029, UIX-030, UIX-035, UIX-036
- **implementer:** `components/GameDetailsPanel.tsx`. `statusLabels`, `Field`, `ListValue` vêm de `GamePage` (apagado só na T18). Raiz = elemento que rola (`h-full overflow-y-auto`); `useLayoutEffect` em `gameId` zera `scrollTop`.
```ts
export function GameDetailsPanel(props: {
  gameId: number; current: SessionView | null; pending: PendingLaunch
  onPlay: (id: number) => void; onPlayUntracked: (id: number) => void
  onEdit: () => void; onSearchMetadata: () => void; onRemove: () => void
  onSelectionEvent: (e: SelectionEvent) => void  // readOk / readMissing / readError / retry, sempre para props.gameId
}): JSX.Element
```
Ordem: cabeçalho (`h2`, `PlatformIcon` e nome, marcação, aviso de rastreio por processo, aviso de emulador) → `MediaCarousel resetKey={gameId}` → "Jogar" (`data-bigbox-play`) / "Jogar sem contar tempo" → tempo → metadados → "Sessões" (`SessionsList current={null}`) → Editar / Buscar metadados / Remover.
- **test-writer:** `GameDetailsPanel.test.tsx` (casos de `GamePage.test.tsx` migrados):
  - `UIX-025/cabeçalho com título, ícone e nome da plataforma`
  - `UIX-025/sessão running ou grace do jogo mostra "Em execução"; paused mostra "Pausado"; sessão de outro jogo, nada`
  - `UIX-025/sessão por processo adotado mostra o aviso de rastreio por processo`
  - `UIX-025/jogo de emulador sem emulador próprio nem padrão mostra o aviso de EMU-003; com padrão, não`
  - `UIX-026/carrossel com as imagens de MediaList do jogo`
  - `UIX-027/Jogar chama onPlay(G) uma vez e Jogar sem contar tempo chama onPlayUntracked(G) uma vez`
  - `UIX-027/pendência de G desativa os dois e o acionado mostra "Iniciando…"`
  - `UIX-027/pendência de outro jogo deixa os botões de G ativos`
  - `UIX-028/tempo total, primeira e última vez (ou "Nunca") e "Concluído em" em UTC, como vêm de GetGame`
  - `UIX-029/status, favorito, gêneros, modos, desenvolvedora, editora, ano e descrição inteira; "—" para vazios`
  - `UIX-030/seção Sessões com as sessões do jogo e "Carregar mais"`
  - `UIX-035/sem resposta mostra "Carregando jogo…" e nada do jogo anterior`
  - `UIX-035/erro diferente de -32001 mostra role=alert e "Tentar de novo", que relê GetGame e emite retry`
  - `UIX-023/-32001 emite readMissing(G)`
  - `UIX-035/leitura ok emite readOk(G)`
  - `UIX-036/trocar gameId volta o scrollTop do painel a 0`
  - `UIX-025/Editar, Buscar metadados e Remover chamam os callbacks uma vez`
- Dependências: T03, T06, T12.

## T16 — Barra do centro
- Requisitos: UIX-013
- **implementer:** `components/LibraryToolbar.tsx`; `SortSelect.tsx` e `ViewToggle.tsx` passam para tokens.
```ts
export function LibraryToolbar(props: {
  total: number | undefined   // "N jogos" só com > 0 [UI-044]
  sort: SortKey; view: LibraryView
  onSortChange: (s: SortKey) => void; onViewChange: (v: LibraryView) => void
}): JSX.Element
```
- **test-writer:** `LibraryToolbar.test.tsx`:
  - `UIX-013/mostra "N jogos" com total > 0 e nenhum contador com total 0 ou undefined`
  - `UIX-013/ordenação chama onSortChange e a alternância chama onViewChange`
- Dependência: nenhuma.

## T17 — Coluna esquerda: busca, plataformas e "Filtros" recolhível
- Requisitos: UIX-008, UIX-010, UIX-011, UIX-012, UIX-043
- **implementer:** `lib/filters.ts`: `withPlatforms`; `components/Sidebar.tsx` (novo: `SearchBox` + `PlatformNav` + `FilterPanel`); `FilterPanel.tsx`: sai o select "Plataforma" (PLT-032 passa a valer em `PlatformNav`), ganha cabeçalho `<button aria-expanded>` "Filtros"/"Filtros (N)", recolhido por padrão, estado por `readFiltersOpen`/`writeFiltersOpen`; "Limpar filtros" continua `onChange(resetQuery())`; `SearchBox.tsx` só tokens; `App.tsx`: busca e `FilterPanel` em linha saem; `<nav aria-label="Filtros da biblioteca">` com `Sidebar` à esquerda do conteúdo atual.
```ts
export function withPlatforms(q: LibraryQuery, ids: number[] | undefined): LibraryQuery
export function Sidebar(props: {
  search: string; onSearchChange: (text: string) => void
  query: LibraryQuery; catalog: CatalogView | undefined
  onChange: (q: LibraryQuery) => void; timers?: Timers
}): JSX.Element
```
- **test-writer:** `Sidebar.test.tsx`; casos novos em `lib/filters.test.ts`. Ajustes: `FilterPanel.test.tsx` (casos da plataforma/PLT-032 saem, já cobertos em `PlatformNav.test.tsx`; os demais abrem a seção antes); `App.test.tsx` (UI-032, UI-035, PLT-032 pelo `PlatformNav`; testes de outras facetas abrem "Filtros" antes):
  - `UIX-008/o primeiro controle da coluna é a busca; espera de 300 ms com o relógio injetável; apagar remove text`
  - `UIX-010/withPlatforms troca platformIds e mantém text, as outras facetas e sort; undefined remove o membro`
  - `UIX-010/clicar numa plataforma envia ListGames com platformIds=[P] e mantém as outras facetas` (App)
  - `UIX-011/cada controle gera o mesmo FilterSpec de antes; não há controle "Plataforma"`
  - `UIX-011/"Limpar filtros" volta a {version: 1}, à ordenação padrão e a "Todos"` (App)
  - `UIX-012/a seção começa recolhida e aria-expanded alterna`
  - `UIX-012/recolhida com 2 facetas ativas mostra "Filtros (2)"; com só texto e plataforma mostra "Filtros"`
  - `UIX-012/recolher não chama onChange; a escolha é gravada e lida de volta`
  - `UIX-043/Enter e Espaço no cabeçalho abrem e fecham a seção`
- Dependências: T05, T11.

## T18 — Três colunas, jogo em foco no painel e fim da página do jogo
- Requisitos: UIX-004, UIX-013, UIX-014, UIX-015, UIX-016, UIX-017, UIX-018, UIX-020, UIX-021, UIX-022, UIX-023, UIX-024, UIX-027, UIX-031, UIX-032, UIX-033, UIX-034, UIX-042, UIX-045, UIX-048
- **implementer:**
  - `App.tsx`: layout do design §4 (`grid` com `var(--col-left) 1fr var(--col-right)`, `<section aria-label="Jogos">`, `<aside aria-label="Detalhes do jogo" class="overflow-hidden">`); `useReducer(selectionReducer, initialSelection)`; `GameDetailsPanel` ou `PanelEmpty`; `onSelect` → `select`, `onLaunch` → `select` + `play`; componente interno que abre `GameEditDialog`, `MetadataDialog` e `DeleteDialog` para o jogo em foco; `readMissing` → invalida `libraryGamesKey` uma vez; `confirmRemove` com sucesso → `removed` e foco no listbox; `StatsPage.onOpenGame` → fecha estatísticas e despacha `select(id)`; sai `GameScreen`, `openGame`/`closeGame` e o import de `GamePage`; tudo com tokens.
  - `GameCard.tsx` (design §6): `<div role="option" aria-selected data-game-id tabIndex>` com `CoverImage` 2:3, título em até duas linhas (`line-clamp-2`, `title`) e selo sobre a capa; `onClick` → `onSelect`, `onDoubleClick` → `onLaunch`; sem botões, plataforma nem tempo; `BusyMode` sai.
  - `LibraryGrid.tsx`: `role="listbox" aria-label="Jogos"`; `columnsFor` de `gridNav` (`estimateSize` usa `rowHeight`, remedido no `ResizeObserver`); `selectedId`; ao montar, `scrollToIndex(row, {align:'center'})` uma vez se o jogo em foco está carregado; prefetch de UI-036 como está.
  - `LibraryList.tsx`: linhas `role="option"` com título, plataforma, tempo, última vez e selo, sem botões; clique e duplo clique como na grade.
  - `LibraryStates.tsx`: o contador vai para `LibraryToolbar`.
  - apagar `components/GamePage.tsx`.
```ts
export function GameCard(props: {
  game: GameView; coverUrl?: string; live: LiveMark; selected: boolean; tabbable: boolean
  onSelect: (id: number) => void; onLaunch: (id: number) => void
}): JSX.Element
export function LibraryGrid(props: {
  games: GameView[]; covers?: Record<number, string>; total: number; current: SessionView | null
  selectedId: number | null; onNeedMore: () => void
  onSelect: (id: number) => void; onLaunch: (id: number) => void
}): JSX.Element
export function LibraryList(props: Parameters<typeof LibraryGrid>[0]): JSX.Element
```
- **test-writer:** apagar `GamePage.test.tsx`; reescrever `GameCard.test.tsx`, `GameCardCover.test.tsx`, `LibraryGrid.test.tsx`, `LibraryList.test.tsx`; ajustar `LibraryStates.test.tsx`; `App.test.tsx` (describe "página do jogo, edição e remoção" vira "painel"; testes de Jogar/Sem contar tempo — UI-008, UI-012, UI-020, UI-021, UI-027 — põem o jogo em foco e usam os botões do painel; UI-030 "mesmos jogos e botões" vira "mesmos jogos"; UI-044 lê o contador da barra); `library-removed.test.ts` (describe UIX-048):
  - `UIX-004/três marcos: nav "Filtros da biblioteca", região "Jogos" e aside "Detalhes do jogo"; main com h-screen e overflow-hidden` (App)
  - `UIX-013/"Conectando ao agente…", "Nenhum jogo ainda" e "Nenhum jogo com estes filtros." aparecem no centro` (App)
  - `UIX-014/cartão com capa 2:3 e título (title com o texto completo); sem plataforma, tempo nem botões`
  - `UIX-014/selo "Em execução" em running e grace, "Pausado" em paused, nenhum sem sessão do jogo`
  - `UIX-015/cartão sem capa mostra o placeholder; com capa, a miniatura`
  - `UIX-016/no máximo 100 cartões no DOM com 10 000 jogos; a próxima página é pedida uma vez`
  - `UIX-017/um clique marca o cartão (aria-selected) e desmarca o anterior; o painel mostra G; só leituras chamadas`
  - `UIX-018/duplo clique chama LaunchGame(G) exatamente uma vez e nunca LaunchGameUntracked`
  - `UIX-018/com lançamento pendente, o duplo clique só põe em foco`
  - `UIX-018/erro do lançamento por duplo clique vai para o aviso do topo (UI-008)`
  - `UIX-020/lista: linha com título, plataforma, tempo, última vez e selo, sem botões; clique põe em foco e duplo clique lança`
  - `UIX-021/filtro que mantém G: painel com G sem passar pelo vazio e cartão marcado`
  - `UIX-021/filtro que tira G: painel com G e nenhum cartão marcado; filtro que o traz de volta: marcado de novo`
  - `UIX-022/alternar grade e lista, abrir e fechar estatísticas e abrir e fechar um modal mantêm G no painel`
  - `UIX-022/reconectar mantém G e relê GetGame(G)`
  - `UIX-023/GetGame(G) com -32001: painel vazio e ListGames chamado de novo uma vez`
  - `UIX-024/ao abrir, nenhum jogo em foco e o painel mostra o resumo`
  - `UIX-027/pendência de outro jogo: clicar em Jogar de G não chama nada` (App)
  - `UIX-031/ended de G relê GetGame(G) e GameSessions(G) uma vez e o painel mostra o total novo; sessão de outro jogo não relê G`
  - `UIX-032/Editar abre "Editar jogo"; Salvar com sucesso fecha e o painel mostra o detalhe; erro mantém o modal; Cancelar não chama UpdateGame`
  - `UIX-033/Buscar metadados abre o diálogo de G; depois de aplicar, o painel mostra os dados novos e MediaList(G) é lido de novo`
  - `UIX-034/Remover abre a confirmação de UI-041; sucesso esvazia o painel e a grade é recarregada sem G`
  - `UIX-034/-32002: aviso de DSK-018 no topo, a confirmação fecha e G continua em foco`
  - `UIX-042/grade com role listbox "Jogos" e cartões role option com o título como nome`
  - `UIX-045/escolher G nas estatísticas volta à biblioteca com os filtros e G no painel`
  - `UIX-048/nenhum arquivo de frontend/src referencia GamePage nem GameScreen`
  - `UIX-048/GamePage.tsx e GamePage.test.tsx não existem`
  - `UIX-048/não há botão "Voltar" fora de StatsPage nem botão "Detalhes"`
- Dependências: T03, T04, T07, T08, T10, T13, T14, T15, T16, T17.

## T19 — Teclado na grade e na lista
- Requisitos: UIX-019, UIX-020, UIX-041, UIX-043
- **implementer:** `LibraryGrid.tsx` e `LibraryList.tsx`: prop `keyboard`; tabindex móvel (cartão do jogo em foco carregado, senão o primeiro); contêiner com `tabIndex` 0 só quando o cartão focável não está no DOM (ao receber foco, passa ao cartão depois do `scrollToIndex`); `onKeyDown` no listbox com `nextIndex` → `onSelect`, `scrollToIndex` e foco no cartão por `useLayoutEffect`; Enter → `onLaunch`. `App.tsx` passa `keyboard={!bigBox}` (`keyboard: boolean` — false no Big Box, onde `useBigBoxNav` manda nas setas [UIX-044]).
- **test-writer:** `LibraryGrid.test.tsx` e `LibraryList.test.tsx` (grade fixa, 3 colunas); `App.test.tsx` (ordem do Tab):
  - `UIX-019/a grade é uma parada de Tab: o foco vai ao cartão do jogo em foco ou ao primeiro`
  - `UIX-019/setas, Home e End movem o foco e o jogo em foco (onSelect) pelas regras de nextIndex`
  - `UIX-019/seta na borda não chama onSelect`
  - `UIX-019/o cartão focado rola para a área visível; perto do fim do carregado, onNeedMore uma vez`
  - `UIX-019/Enter chama onLaunch do cartão focado; com pendência, App não chama LaunchGame`
  - `UIX-019/com keyboard=false as setas não fazem nada na grade`
  - `UIX-020/lista: setas para cima e para baixo movem o foco e o jogo em foco; Enter lança`
  - `UIX-041/Tab percorre cabeçalho → coluna esquerda (busca, plataformas, filtros) → barra do centro → grade (uma parada) → painel` (App)
  - `UIX-043/pôr em foco e lançar só pelo teclado, sem mouse` (App)
- Dependência: T18.

## T20 — Big Box sobre o layout novo
- Requisitos: UIX-044, UIX-045, UIX-022
- **implementer:**
  - `lib/bigbox.ts`: `cardGrid` agrupa por `[data-bigbox-card]` (um item por cartão); `gridNext` devolve `null` na borda e `nextFocus` cai em `pickNext` com todos os itens (direita → painel, esquerda → coluna esquerda); Y acha o `[data-bigbox-card]` do foco e chama `onPlay(Number(dataset.gameId))`; Enter/A num cartão chama `click()`; `pageKey` passa a `'library' | 'stats'`.
  - `data-bigbox-item` em `GameCard` (também `data-bigbox-card`), `SearchBox`, `PlatformNav`, `FilterPanel` (cabeçalho e controles), `GameDetailsPanel` (botões; "Jogar" com `data-bigbox-play`), `MediaCarousel` (botões), `PanelEmpty` (itens).
  - `App.tsx`: `bigBoxBack` — foco no `<aside>` → foca o cartão `[data-game-id=G]` (ou o listbox); estatísticas abertas → fecha; senão sai do Big Box; "focar Jogar" pendente (A num cartão e escolha nas estatísticas), cumprida quando `selection.kind === 'shown'`; `onPlay` → `play`.
```ts
export type Card = { items: HTMLElement[] }
export function cardGrid(items: HTMLElement[]): Card[][] | null
export function gridNext(grid: Card[][], current: HTMLElement, dir: Direction): HTMLElement | null
export function nextFocus(items: HTMLElement[], focused: HTMLElement | null, dir: Direction): HTMLElement
export function useBigBoxNav(opts: {
  enabled: boolean; pageKey: 'library' | 'stats'; onBack: () => void; onPlay: (gameId: number) => void
}): void
```
- **test-writer:** `lib/bigbox.test.tsx` e `components/GameCardBigBox.test.tsx` (reescritos, um item por cartão); casos novos em `App.test.tsx`:
  - `UIX-044/cada cartão é um item; direções movem cartão a cartão`
  - `UIX-044/na borda direita da grade o foco vai ao painel e na borda esquerda à coluna esquerda`
  - `UIX-044/busca, Todos, plataformas e filtros são itens navegáveis`
  - `UIX-044/A ou Enter num cartão põe o jogo em foco e leva o foco a "Jogar" do painel`
  - `UIX-044/Y ou Shift+Enter num cartão chama LaunchGame do jogo do cartão uma vez`
  - `UIX-044/B com foco no painel volta ao cartão do jogo em foco; nas estatísticas volta à biblioteca; na grade sai do Big Box`
  - `UIX-044/no Big Box uma seta move o foco uma vez só (sem o handler da grade)`
  - `UIX-045/no Big Box, escolher G nas estatísticas põe G em foco e o foco em "Jogar"`
  - `UIX-022/entrar e sair do Big Box mantém G no painel`
- Dependência: T19.

## T21 — Cores restantes por tokens e varreduras
- Requisitos: UIX-006, UIX-040, UIX-041
- **implementer:** trocar classes de paleta e cores literais por utilitários do tema, sem mudar comportamento, em `AgentConflict`, `AgentIndicator`, `EmptyResults`, `ErrorBanner`, `PlatformIcon`, `SessionPanel`, `SessionsList`, `StatsChart`, `StatsPage`, `TagInput` e qualquer `.tsx` que a varredura apontar (inclusive `main.tsx`). Tirar todo `outline-none` sem `focus-visible:` no mesmo `className`.
- **test-writer:** `frontend/src/theme-scan.test.ts` (varre `./**/*.tsx` sem `*.test.tsx` por `import.meta.glob(..., { query: '?raw' })`):
  - `UIX-006/nenhum .tsx usa classe de paleta do Tailwind (padrão de design §3)`
  - `UIX-006/nenhum .tsx usa cor literal (#hex, rgb(, hsl() em className ou style`
  - `UIX-040/nenhum className com outline-none sem focus-visible: correspondente`
  - `UIX-041/nenhum tabIndex maior que 0`
- Dependência: T20.

## T22 — Invariante: a UI não influencia o tempo de jogo
- Requisitos: UIX-046, UIX-047
- **test-writer** (só teste; o implementer só entra se o teste achar chamada indevida): `frontend/src/App.invariants.test.tsx` (todos os bindings de `window.go.main.App` como espiões):
  - `UIX-046/pôr em foco, setas, carrossel, plataformas, filtros, seção recolhível, abrir e fechar cada modal, entrar e sair do Big Box e estatísticas chamam só leituras`
  - `UIX-046/duplo clique, Enter, Jogar e Y chamam LaunchGame exatamente uma vez cada; Jogar sem contar tempo chama LaunchGameUntracked uma vez`
  - `UIX-046/ToggleSession só é chamado pelo painel de sessão`
  - `UIX-047/nenhum binding novo: as chamadas feitas pela UI estão na lista de bindings da fase 9`
- **Revisão (sessão principal):** `git diff --stat` da fase sem arquivos em `internal/` e `cmd/fake-game/`; em `cmd/arcadia`, só `window.go`, `window_test.go` e `main_windows.go`; `internal/ipc/contract_test.go` sem alteração e verde.
- Dependência: T21.

## T24 — Lacunas da revisão: modais no Big Box, foco dos modais, edição e tecla mantida
- Requisitos: UIX-032, UIX-037, UIX-039, UIX-042, UIX-044, UIX-045, UIX-046 (critérios "acrescentados após a revisão", 2026-11-14)
- **implementer:**
  - `lib/bigbox.ts` (`useBigBoxNav`): não age enquanto houver `[role="dialog"][aria-modal="true"]` no documento (teclado e laço do controle: setas, A/Enter, Y/Shift+Enter, B); ignora `KeyboardEvent` com `defaultPrevented`; ignora `e.repeat` em Enter/Shift+Enter; Y do controle lança só na borda de descida do botão (já é, conferir).
  - `components/MediaCarousel.tsx`: setas tratadas chamam `e.preventDefault()` (o Big Box então ignora).
  - `components/Modal.tsx`: `mousedown` no overlay e em área não focável não tira o foco do diálogo (`preventDefault` no overlay; `focusout`/`focusin` para fora do diálogo devolve o foco ao `role="dialog"` com `tabIndex={-1}`); o trap de Tab e o Esc tratam também o foco no próprio `dialog`.
  - `components/GameEditDialog.tsx`: `closeDisabled={pending}` (Esc e Cancelar sem efeito durante o salvamento); props `game` passa a `game: GameDetailView | undefined` mais `loading: boolean` e `loadError: string | null` (corpo com "Carregando jogo…" ou `role="alert"` no lugar do formulário).
  - `App.tsx`: `GameDialogs` montado com `key={gameId}`; abre o modal de edição sempre que `dialog === 'edit'` (passa `loading`/`loadError` de `useGame`); o fechamento depois do sucesso e a mensagem de erro não dependem do modal ter sido desmontado; o "focar Jogar" pendente guarda o id do jogo e é limpo quando `selection` passa a outro id ou a leitura dá `readError`/`readMissing`; `LibraryGrid`/`LibraryList`: Enter com `e.repeat` não chama `onLaunch`.
```ts
export function GameEditDialog(props: {
  game: GameDetailView | undefined; loading: boolean; loadError: string | null
  genreSuggestions: string[]; emulators: EmulatorView[]
  pending: boolean; error: string | null
  onSave: (patch: Record<string, unknown>) => void; onCancel: () => void
}): JSX.Element   // Modal size="medium" title="Editar jogo" closeDisabled={pending}
```
- **test-writer:**
  - `lib/bigbox.test.tsx`:
    - `UIX-037/com um modal aberto, setas, Enter, Shift+Enter, Y e B do teclado não agem no fundo e o foco fica no modal`
    - `UIX-037/com um modal aberto, direções, A, Y e B do controle (gamepad simulado) não agem no fundo`
    - `UIX-037/ao fechar o modal, a navegação do Big Box volta a valer`
    - `UIX-044/evento de teclado com defaultPrevented é ignorado pelo Big Box`
    - `UIX-046/Shift+Enter mantido (repeat) num cartão chama onPlay uma vez`
  - `components/MediaCarousel.test.tsx`:
    - `UIX-042/no Big Box, seta num botão do carrossel troca a imagem uma vez e não move o foco do Big Box`
  - `components/Modal.test.tsx`:
    - `UIX-039/mousedown no overlay não tira o foco do diálogo; depois, Tab continua preso e Esc fecha`
    - `UIX-039/clique em texto não focável do modal mantém o foco no diálogo; Tab vai ao primeiro controle e Esc fecha`
    - `UIX-039/foco no próprio dialog: Shift+Tab vai ao último controle`
  - `components/GameEditDialog.test.tsx`:
    - `UIX-032/com pending, Esc não chama onCancel e Cancelar fica desativado`
    - `UIX-032/com loading mostra "Carregando jogo…" e nenhum campo`
    - `UIX-032/com loadError mostra role=alert e nenhum campo`
  - `App.test.tsx`:
    - `UIX-032/Esc durante o salvamento não fecha; sucesso fecha e o painel mostra o detalhe devolvido`
    - `UIX-032/Esc durante o salvamento não fecha; erro mantém o modal com os valores e a mensagem`
    - `UIX-032/Editar com GetGame pendente abre o modal com "Carregando jogo…" e depois o formulário`
    - `UIX-032/Editar com GetGame em erro abre o modal com a mensagem de erro`
    - `UIX-032/erro de edição de A não aparece no modal de edição de B (GameDialogs reinicia por jogo)`
    - `UIX-037/Big Box com Editar aberto: setas, Enter, Shift+Enter, Y e B não mudam o jogo em foco, não lançam e não saem do Big Box`
    - `UIX-044/A num cartão de G e depois clique em H antes de G carregar: o foco não vai a "Jogar" de H`
    - `UIX-044/A num cartão de G com GetGame em erro: o foco não vai a "Jogar" (nem depois de "Tentar de novo")`
    - `UIX-045/no Big Box, escolher G nas estatísticas com GetGame(G) em -32001: o foco não vai a "Jogar"`
  - `components/LibraryGrid.test.tsx` e `components/LibraryList.test.tsx`:
    - `UIX-046/Enter mantido (repeat) chama onLaunch uma vez`
  - `App.invariants.test.tsx`:
    - `UIX-046/Enter mantido na grade e Shift+Enter mantido no Big Box chamam LaunchGame exatamente uma vez cada`
- Dependência: T22.

## T23 — Conferência manual da fase 10
- Requisitos: UIX-049, UIX-001, UIX-002, UIX-003, UIX-005, UIX-007, UIX-037, UIX-038, UIX-040, UIX-043, UIX-044, UIX-047
- **Responsável:** sessão principal com o usuário.
- **Builds e testes:** `go build ./...`, `go vet ./...` (Windows e `GOOS=linux`, pois `window.go` não tem build tag), `CGO_ENABLED=0 go build ./...`, `go test ./...`; `npm test`, `tsc --noEmit`, `npm run build`, `wails build -ldflags "-X main.version=<v>"`.
- **Rastreio:** `powershell -NoProfile -ExecutionPolicy Bypass -File scripts/spec-trace.ps1 -Spec docs/specs/phase-10-nova-interface` (antes do test-writer: `-SkipTests`).
- **Roteiro (UIX-049)**, com biblioteca real de alguns milhares de jogos e backup de `data\`, em 1366×768 e 1920×1080:
  1. abrir a UI (maximizada, sem tela cheia; restaurar dá 1280×800 e não encolhe abaixo de 1024×640);
  2. filtrar por plataforma, buscar um título, pôr um jogo em foco e mudar o filtro (o jogo continua no painel);
  3. passar as imagens, jogar com duplo clique, pausar pelo atalho e fechar o jogo (tempo e sessões do painel atualizados e iguais ao do agente);
  4. abrir Importar, Emuladores e Editar (tamanho da classe, rolagem interna; conferir em 1024×640);
  5. entrar e sair do Big Box: a janela volta maximizada (UIX-003) e o controle funciona na TV (UIX-044);
  6. segunda instância com a janela restaurada: vem para a frente sem maximizar (UIX-003);
  7. foco visível e todas as ações pelo teclado (UIX-040, UIX-043);
  8. comparação visual com o LaunchBox (UIX-007) e colunas cabendo em 1024, 1366 e 1920 (UIX-005).
- **Se a saída do Big Box não restaurar a janela maximizada:** tarefa extra com a alternativa do design §2 (`exitBigBox` guarda `WindowIsMaximised()` ao entrar e chama `WindowMaximise()` ao sair); muda só `lib/bigbox.ts` e o teste.
- **Documentação:** `docs/ARCHITECTURE.md` §11 (três colunas, jogo em foco como estado da UI, tokens `--arc-*`, classes de modal); `docs/ROADMAP.md`: fase 10 concluída.
- Dependência: T01..T22 e T24.

## Ordem

- **Bloco A (paralelo):** T01, T02, T03, T04, T05, T06, T07, T08, T11, T16.
- **Bloco B:** T09 (após T02) → T10 e T14 em paralelo; T12 após T08; T13 após T06 e T08; T15 após T03, T06 e T12; T17 após T05 e T11.
- **Depois:** T18 → T19 → T20 → T21 → T22 → T24 → T23.

## Riscos de concorrência e recursos

- **Go:** só `options.App` declarativo; nenhuma goroutine ou handle novo. Risco de build: `window.go` sem build tag importa `wails/v2/pkg/options`; T23 confere `GOOS=linux go vet ./cmd/arcadia`; se falhar, vira `window_windows.go` e o teste ganha a mesma tag.
- **UI:** `ResizeObserver` da grade/lista desconectado no cleanup; laço `requestAnimationFrame` do Big Box com `cancelAnimationFrame` no cleanup; listeners de `window` removidos no cleanup; o carrossel não tem timer; a espera da busca segue no `createDebouncer`, cancelada ao desmontar.
- **Respostas atrasadas:** leituras por chave do jogo; o painel renderiza só a chave do jogo em foco e o reducer ignora eventos de outro id.
- **Lançamento único:** a guarda `pending !== null` em `play` e `playUntracked` continua a única porta; no duplo clique, os dois `select` do mesmo id não mudam o estado e só o `dblclick` lança.
- **Teclado em dobro:** fora do Big Box só o listbox trata setas; no Big Box só o `window` (`keyboard={!bigBox}`); a T20 testa.
- **Foco dos modais:** o `Modal` guarda o `activeElement` e o devolve ao fechar; se saiu do documento (remoção), vai ao listbox.
- **Edição durante fim de sessão:** a invalidação de `game(G)` não reinicia o formulário (cópia inicial por `useState`), que envia só o patch.

## Rastreabilidade

| Requisito | Tarefas |
|---|---|
| UIX-001, UIX-002 | T01, T23 |
| UIX-003 | T23 (manual; tarefa extra se falhar) |
| UIX-004 | T18 |
| UIX-005 | T02, T04, T23 |
| UIX-006 | T02, T21 |
| UIX-007 | T02, T23 (visual) |
| UIX-008 | T17 |
| UIX-009 | T11 |
| UIX-010 | T11, T17 |
| UIX-011 | T17 |
| UIX-012 | T05, T17 |
| UIX-013 | T16, T18 |
| UIX-014 | T04, T18 |
| UIX-015 | T08, T18 |
| UIX-016 | T18 |
| UIX-017 | T03, T18 |
| UIX-018 | T18 |
| UIX-019 | T04, T19 |
| UIX-020 | T04, T05, T18, T19 |
| UIX-021 | T03, T18 |
| UIX-022 | T03, T07, T18, T20 |
| UIX-023 | T03, T06, T15, T18 |
| UIX-024 | T03, T06, T13, T18 |
| UIX-025 | T15 |
| UIX-026 | T12, T15 |
| UIX-027 | T15, T18 |
| UIX-028, UIX-029, UIX-030 | T15 |
| UIX-031 | T07, T18 |
| UIX-032 | T14, T18, T24 |
| UIX-033 | T18 |
| UIX-034 | T03, T18 |
| UIX-035 | T03, T15 |
| UIX-036 | T15 |
| UIX-037 | T02, T09, T10, T14, T24, T23 |
| UIX-038 | T09, T10, T23 |
| UIX-039 | T09, T10, T24 |
| UIX-040 | T02, T21, T23 |
| UIX-041 | T19, T21 |
| UIX-042 | T08, T11, T12, T18, T24 |
| UIX-043 | T11, T12, T17, T19, T23 |
| UIX-044 | T20, T24, T23 (TV) |
| UIX-045 | T18, T20, T24 |
| UIX-046 | T22, T24 |
| UIX-047 | T22, T23 (revisão do diff) |
| UIX-048 | T18 |
| UIX-049 | T23 |

Todos os 49 requisitos têm tarefa.
