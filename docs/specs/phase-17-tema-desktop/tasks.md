# Fase 17 — Tema Desktop do Big Box: tarefas

- **Status:** gerado pelo planner em 2026-11-27, sobre os requisitos aprovados na mesma data (oito respostas padrão, mais TDK-002, TDK-037 e TDK-038 do pedido do usuário; versão 1.3.0).
- **Requisitos:** [requirements.md](requirements.md) (BBX-060..BBX-070, TDK-001..TDK-038) · **Design:** [design.md](design.md) · **Contrato:** [contracts/bigbox-tema-desktop.md](contracts/bigbox-tema-desktop.md).
- **ADRs:** 0028, 0029, 0036, 0040 (existentes); 0042 (aceito, já escrito; falta só a linha no `docs/adr/README.md`, T17).

## Resumo

Só frontend. **Nenhuma tarefa de Go**: nada muda em `internal/*`, `cmd/arcadia`, IPC, banco, `internal/tracker/` nem `cmd/fake-game/` (nenhuma tarefa do tracker-specialist). O tema `desktop` entra no registro com o layout `columns`: o reducer ganha zonas, o shell ganha colunas/fileiras por janela, rolagem pelas bordas, limite de nós por tema e Detalhes montado em Navegar, e o tema redesenha a janela normal com componentes próprios, constantes compartilhadas e tokens copiados.

Etapas (cada uma com os testes antes do código; testes vermelhos entre o par teste/código são esperados):

- **A (T01..T02):** movimentos mínimos para `lib/` (ícones de plataforma, rótulos de status, `LIST_ROW_HEIGHT`, `sortLabel`).
- **B (T03..T04):** reducer: zonas, layout `columns`, `tabSet`.
- **C (T05..T06):** contrato de tema, capacidade `desktopGrid`, funções puras da janela, modelo, páginas, dicas.
- **D (T07..T08):** shell (`BigBox.tsx`): colunas e fileiras dinâmicas, rolagem pelas bordas, Detalhes em Navegar, `onTab`, ordem e visualização herdadas; `App.tsx` só repassa as duas props.
- **E (T09..T10):** tokens e CSS.
- **F (T11..T15):** tema Desktop. Testes de componente (T11), **testes de DOM TDK-037/TDK-038 (T12)**, integração do shell com o Desktop (T13), varreduras (T14); código (T15).
- **G (T16):** App e seletor (verificação; esperado verde).
- **H (T17..T18):** documentação e versão 1.3.0.
- **I (T19):** verificação final e roteiro manual (TDK-035, TDK-036).

Ordem diferente da pedida, de propósito: os testes de DOM (TDK-037/038) e as varreduras vêm **antes** do código do tema (T12 e T14 antes de T15) para respeitar "teste antes de código"; as tarefas de código só terminam quando eles passam.

## Dúvidas e lacunas da spec (nenhuma bloqueia; as marcadas "spec-writer" pedem ajuste de texto)

1. **Clique num cartão não deve recentrar (spec-writer, BBX-063, TDK-025).** O shell usa `listLoaded {initialIndex}` também para `onPick` (clique), e o contrato §5 manda aplicar `centeredRow` em `listLoaded`. Recentrar a cada clique mexeria na grade. O plano **não** liga o centramento ao tipo da ação: `useGridStart` expõe `recenter()`, chamado só na entrada (efeito de `listLoaded` da entrada), na troca de aba (chave da aba mudou) e em `gameClosed`. O clique usa `edgeScrollRow`. Sugestão: dizer isso no contrato §5.
2. **`preview` sem `active` e `sound` (design §5).** O `FocusVideo` do painel precisa de `active` (`browse && !suspended`). O plano usa `preview: { gameId, videoUrl, active, failed, sound, onVideoError }`.
3. **`LibraryView` no contrato.** O contrato põe o tipo em `themes/types.ts`, mas temas não importam `lib/` (BBX-047) e o tipo já existe em `lib/libraryView.ts` e `lib/uiPrefs.ts`. O plano o define em `bigbox/viewModel.ts` (estruturalmente igual) e `themes/types.ts` o reexporta.
4. **`BigBoxThemeId` duplicado** (`viewModel.ts` e `lib/uiPrefs.ts`). Para compilar com `desktop`, a T06 unifica: `uiPrefs.ts` passa a reexportar o de `viewModel.ts`.
5. **Zona obsoleta em Detalhes (menor).** X em Plataformas abre Detalhes sem mudar `zone`; `back` e `prev` já a levam a `games`. Só `tabSet` vindo de Detalhes herda `platforms` ("zona igual", contrato §2). Comportamento mantido e fixado por teste; sugestão ao spec-writer: confirmar.
6. **Anotações nas specs de origem ausentes.** `requirements.md` diz que BBX-002, 004, 015, 016, 032, 033, 042 (fase 13) e BBX-046..050, 053, 055, 056 (fase 15) "receberam a anotação", mas `grep phase-17` nessas specs e no `docs/adr/README.md` não acha nada. A T17 faz isso.
7. **Campos novos do modelo e de `BigBoxTab`.** Para não quebrar os `toEqual` atuais, `BigBoxTab.iconUrl/count` e `BigBoxGame.coverFit/platform/playTimeText/lastPlayedText` ficam **opcionais** e só são preenchidos com `layout = columns` (o contrato dizia "null nos outros temas"; ausente é equivalente para a UI). `layout`, `zone` e `view` do `BigBoxViewModel` são **obrigatórios**: o test-writer atualiza os `model()` dos testes de tema (T05).
8. **`gridWindow`, `gridPageSize` e `shouldPrefetchGrid` com limite de nós.** Os testes atuais chamam `gridPageSize(5, 99)` esperando 45 (corte em `GRID_MAX_ROWS`). O plano mantém as assinaturas e acrescenta um último parâmetro opcional `maxNodes = GRID_MAX_NODES`; internamente usam `effectiveRows(rows, columns, maxNodes)`. Com 30 nós o resultado é idêntico ao de hoje.
9. **Árvore suja.** `git status` mostra `.claude/agents/*` modificados e `frontend/tsconfig.tsbuildinfo` e `scripts/icons/*` não rastreados; nenhum é tocado por estas tarefas, mas convém commitar ou descartar antes da T17/T18 (que mexem em `CLAUDE.md`).

## Premissas

- Uma tarefa por commit; a de testes vem antes da de código. Ao fim de cada tarefa: `npm test` e `npx tsc --noEmit` em `frontend/` (o hook `gotest.ps1` só roda `go test ./...`, então a verificação de frontend é manual por tarefa).
- Todo caso começa pelo ID (`it('TDK-013/...')`). Sem rede e sem espera real: `vi.useFakeTimers`, `navigator.getGamepads` falso, bindings falsos em `window.go.main.App`, `scrollIntoView` ausente tolerado (jsdom).
- **Dependências externas novas: nenhuma** (npm e Go).
- **Testes atuais que mudam de propósito** (test-writer anota `alterado em 2026-11-27 (fase 17)`): `bigbox/nav.test.ts` (helper `ctx` ganha `axis` e `layout: 'screens'`, T03); `bigbox/themes/{faixa/Faixa,mosaico/Mosaico,estante/Estante}.test.tsx` (helper `model()` ganha `layout`, `zone`, `view`, T05); `bigbox/BigBox.test.tsx` (`setup` passa `librarySort` e `libraryView`, T07); `bigbox/themes/registry.test.ts` (lista de temas, `videoSlot` com `panel`, `sort` com `inherit`, T11); `App.test.tsx` (opções do seletor, T16). Os demais testes de Faixa, Mosaico, Estante, `useBigBoxData`, `hints`, `LibraryList`, `SortSelect` e `GameDetailsPanel` passam sem mudança e são a rede de segurança.

## Paralelismo e dependências

```
A: T01 -> T02
B: T03 -> T04                       (independe de A)
C: T05 -> T06                       (T06 depende de T02 e T04: tipos NavZone e módulos de lib)
D: T07 -> T08                       (T08 depende de T04 e T06)
E: T09 -> T10                       (independe de B..D; só index.css)
F: T11, T12, T13, T14 (testes)  ->  T15 (código; depende de T06, T08, T10)
G: T16                              (depois de T15)
H: T17 (depois de T15), T18 (depois de T17)
I: T19                              (depende de tudo)
```

- **Em paralelo:** E com B..D (arquivos disjuntos); T11..T14 entre si (arquivos disjuntos).
- **Não paralelizar:** T06 e T08 (ambos mexem em `viewModel.ts`/`BigBox.tsx`); T15 (cria todos os arquivos de `themes/desktop/` e a entrada do registro).

---

## T01 — Testes: movimentos mínimos para `lib/`
- Requisitos: BBX-066, BBX-067, TDK-008, TDK-034. **Responsável:** test-writer. Dependência: nenhuma.
- Arquivos (testes):
  - `frontend/src/lib/platformIcons.test.ts` (novo)
  - `frontend/src/lib/gameLabels.test.ts` (novo)
  - `frontend/src/lib/gridNav.test.ts` (acrescentar)
  - `frontend/src/lib/filters.test.ts` (acrescentar)
- Testes:
  - `platformIcons`: `BBX-066/ícone personalizado (iconUrl) vence`; `BBX-066/seedKey conhecido devolve o SVG embutido`; `BBX-066/sem seedKey ou chave desconhecida devolve o genérico`; `BBX-066/seedIconUrls tem as 13 chaves da semente`; `BBX-066/lib/platforms reexporta as mesmas referências (platformIconSrc, seedIconUrls, genericIconUrl)`.
  - `gameLabels`: `TDK-008/statusLabels cobre not_played, playing, completed, abandoned e backlog com "Não jogado", "Jogando", "Concluído", "Abandonado", "Na fila"`; `TDK-008/playModeLabels reexportado é o mesmo objeto de lib/library`.
  - `gridNav`: `TDK-034/LIST_ROW_HEIGHT = 64`.
  - `filters`: `BBX-067/sortOptions na ordem Título, Última vez jogado, Tempo jogado, Ano com as chaves title, lastPlayed, totalPlayed, year`; `BBX-067/sortLabel(k) devolve o rótulo de cada chave`; `BBX-067/isSortKey aceita as quatro chaves e recusa "" e "desktop"`.

## T02 — Frontend: movimentos mínimos para `lib/`
- Requisitos: BBX-066, BBX-067, TDK-008, TDK-034. **Responsável:** implementer. Dependência: T01.
- Arquivos:
  - `frontend/src/lib/platformIcons.ts` (novo): move `iconModules`, `seedIconUrls`, `genericIconUrl` e `platformIconSrc` de `lib/platforms.ts`, sem funções de escrita (a varredura de BBX-003 proíbe `lib/platforms` em `bigbox/`; `platformIcons` não casa com o padrão).
  - `frontend/src/lib/platforms.ts`: passa a importar e reexportar de `./platformIcons`; as funções de escrita ficam.
  - `frontend/src/lib/gameLabels.ts` (novo).
  - `frontend/src/lib/gridNav.ts`: `LIST_ROW_HEIGHT`.
  - `frontend/src/components/LibraryList.tsx`: usa `LIST_ROW_HEIGHT` (remove o `ROW_HEIGHT` local).
  - `frontend/src/lib/filters.ts`: `sortOptions`, `sortLabel`, `isSortKey` movidos de `SortSelect`.
  - `frontend/src/components/SortSelect.tsx` e `frontend/src/components/GameDetailsPanel.tsx`: passam a importar `sortOptions`/`isSortKey` e `statusLabels`.
- Assinaturas:
```ts
// lib/platformIcons.ts
export const seedIconUrls: Record<string, string>
export const genericIconUrl: string
export function platformIconSrc(p: Pick<PlatformView, 'seedKey' | 'iconUrl'>): string
// lib/gameLabels.ts
export const statusLabels: Record<GameStatus, string>
export { playModeLabels } from './library'
// lib/gridNav.ts
export const LIST_ROW_HEIGHT = 64
// lib/filters.ts
export const sortOptions: ReadonlyArray<{ key: SortKey; label: string }>
export function sortLabel(k: SortKey): string
export function isSortKey(v: string): v is SortKey
```
- Sem mudança de comportamento: os testes atuais de plataformas, lista, ordenação e painel passam como estão.

## T03 — Testes: reducer com zonas, layout `columns` e `tabSet`
- Requisitos: BBX-065, TDK-012, TDK-013, TDK-014, TDK-015, TDK-016, TDK-017, TDK-018, TDK-033. **Responsável:** test-writer. Dependência: nenhuma.
- Arquivo: `frontend/src/bigbox/nav.test.ts` (alterar o helper `ctx` e acrescentar). `ctx({ columns: 6, total: 20, loaded: 20, tabs: 4, galleryCount: 3, layout: 'columns', axis: 'grid' })` para os vetores do contrato §2.
- Testes (um caso por linha das tabelas do contrato §2):
  - `TDK-012/estado inicial tem zone games`; `TDK-012/listLoaded e gameClosed levam a zone games (a partir de platforms)`.
  - `TDK-013/vetores de games com C=6`: prev em f 0 → platforms; prev em f 3 → 2; next em f 4 → 5; next em f 5 → details/play; next em f 19 → details/play; down f 13 → 19; down f 14 → 19; down f 18 → 18; up f 8 → 2; up f 2 → 2.
  - `TDK-013/T=0: prev leva a platforms e next não faz nada`; `TDK-013/nunca entra em jogo não lido (loaded < total: down e next param em M = loaded-1)`; `TDK-013/next sem jogo em foco não vai a details`.
  - `TDK-014/C=1, T=5: up/down/prev/next em 0/0/2/2 dão 0/1/platforms/details`.
  - `TDK-015/platforms: up em tab 0 e down em tab 3 (tabs 4) ficam iguais`; `TDK-015/up e down trocam a aba com focus 0 e videoFailed null`; `TDK-015/next, accept e back levam a games sem effects (nem launch nem exit)`; `TDK-015/prev não faz nada`; `TDK-015/accept em platforms com aba vazia também volta a games`.
  - `TDK-016/tabPrev e tabNext são circulares, focus 0 e mantêm a zona (games e platforms)`; `TDK-016/details (X) a partir de platforms com jogo em foco abre details com foco em play; sem jogo não faz nada`.
  - `TDK-017/games: accept com jogo → launch; accept sem jogo nada; back → exit`.
  - `TDK-018/details, play: up → gallery; down → playUntracked; prev → browse games com o mesmo focus; next igual; back → browse games`; `TDK-018/play sem galeria: up igual`; `TDK-018/gallery: down → play; prev/next limitados a [0, galleryCount-1]; up igual`; `TDK-018/playUntracked: up → play e down igual`; `TDK-018/galleryChanged(0) com foco no visor → play`; `TDK-018/accept em play e playUntracked dá launch e launchUntracked; no visor nada`.
  - `BBX-065/tabSet: índice diferente e válido → browse, tab i, focus 0`; `BBX-065/índice igual, -1 e tabs ficam iguais`; `BBX-065/a partir de details volta a browse`; `BBX-065/ignorado com suspended` (e não é ação de entrada: `windowFocus` ainda libera).
  - `TDK-033/gameClosed volta a browse, zone games, focus = indexInTab, com os effects bringToFront e enterBigBox`.
  - Regressão `layout: 'screens'` (casos atuais, com o `ctx` novo): `BBX-048/layout screens: ←/→ passam de fileira e zone fica games`; `BBX-048/axis horizontal com columns 1: up/down não fazem nada`.

## T04 — Frontend: reducer com zonas e layout `columns`
- Requisitos: BBX-065, TDK-012, TDK-013, TDK-014, TDK-015, TDK-016, TDK-017, TDK-018, TDK-033. **Responsável:** implementer. Dependência: T03.
- Arquivo: `frontend/src/bigbox/nav.ts`.
- Assinaturas (acréscimos):
```ts
export type NavZone = 'games' | 'platforms'
export type NavState = { /* atual */ zone: NavZone }            // initialNav.zone = 'games'
export type NavCtx = { /* atual */ axis: 'horizontal' | 'vertical' | 'grid'; layout: 'screens' | 'columns' }
export type NavAction = /* atual */ | { type: 'tabSet'; index: number }
export function bigBoxNav(s: NavState, a: NavAction, c: NavCtx): { state: NavState; effects: NavEffect[] }  // assinatura igual
```
- Regras (contrato §2): o ramo `up`/`down` de Navegar passa de `cols <= 1` para `c.axis !== 'grid'`; com `columns`, `zone = platforms` desvia para troca de aba; `prev`/`next` seguem a tabela (sem passar de fileira, zonas nas pontas); em Detalhes com `columns`, `up`/`down` na ordem empilhada visor ↔ Jogar ↔ Jogar sem contar tempo (atualizando `detailsButton`) e `prev` num botão volta a `browse`/`games`; `back`, `accept`, `next` em `platforms` voltam a `games` **antes** da checagem de `focusedGameId`; `listLoaded` e `gameClosed` forçam `zone = games`; `tabSet` fora de `inputTypes`, mas ignorado com `suspended` por checagem própria. Com `layout = screens` o código atual fica intacto.

## T05 — Testes: contrato de tema, capacidade, janela, modelo, dados, dicas
- Requisitos: BBX-061, BBX-062, BBX-063, BBX-066, BBX-067, BBX-068, TDK-004, TDK-005, TDK-006, TDK-007, TDK-008, TDK-019, TDK-020, TDK-024, TDK-034. **Responsável:** test-writer. Dependência: T01.
- Arquivos (testes):
  - `frontend/src/bigbox/viewModel.test.ts` (acrescentar)
  - `frontend/src/bigbox/useBigBoxData.test.tsx` (acrescentar)
  - `frontend/src/bigbox/hints.test.ts` (acrescentar)
  - `frontend/src/bigbox/desktopLayout.test.ts` (novo)
  - `frontend/src/bigbox/desktopLabels.test.ts` (novo)
  - fixtures: `Faixa.test.tsx`, `Mosaico.test.tsx`, `Estante.test.tsx` (helper `model()` ganha `layout: 'screens'`, `zone: 'games'`, `view: 'grid'`, anotado).
- Testes:
  - `viewModel.test.ts`:
    - `BBX-062/effectiveRows(R, C, N)`: (1,5,30)=1; (9,5,30)=5; (0,5,30)=1; (3,40,30)=1; (2,6,200)=2; (6,17,200)=6; (13,1,200)=13; sem `N` vale 30.
    - `BBX-062/gridWindowAt` com os 7 vetores do contrato §4 e `T=0 → {0,0}`; `BBX-062/nunca desenha mais de (R'+1)·C ≤ N cartões`.
    - `BBX-049/gridWindow` com os vetores atuais continua igual (rede de segurança).
    - `BBX-063/edgeScrollRow`: os 8 vetores do contrato §5 mais `limitado a [0, max(rows-R',0)]`.
    - `BBX-063/centeredRow`: os 4 vetores do §5.
    - `BBX-066/buildTabs(..., withIcons)`: ícone por `platformIconSrc` (personalizado, embutido, genérico) e `count = gameCount`; "Todos" soma as plataformas com jogos.
    - `BBX-066/sem withIcons as abas são iguais às de hoje (toEqual)`.
    - `BBX-067/sortLabel do modelo com sortInherited`: "Título", "Última vez jogado", "Tempo jogado", "Ano"; sem herança continua "Título" e "Jogado recentemente".
    - `BBX-068/o modelo repassa layout, zone, view e columns`.
    - `TDK-005/TDK-006/com layout columns os jogos trazem coverFit (do CardImage), platform, playTimeText (formatPlayTime) e lastPlayedText ("Nunca" sem data)`; `TDK-005/sem layout columns esses campos não existem (toEqual dos atuais)`.
    - `TDK-019/buildViewModel com startRow usa gridWindowAt e stripStart = startRow·C`.
    - `TDK-004/total e rótulos do modelo`.
  - `useBigBoxData.test.tsx`:
    - `BBX-062/gridPageSize(6, 2, 200) = 36`, `(17, 6, 200) = 170`, `(1, 13, 200) = 17`; sem o 3º argumento os testes atuais seguem.
    - `BBX-062/shouldPrefetchGrid com maxNodes 200 pede quando L < min(T, (⌊f/C⌋ + R' + 1 + 3)·C)` (vetores C=6/R=2 e C=1/R=13) e não com `exhausted` ou `L = T`.
    - `TDK-020/useTabGames com paging { kind: 'grid', columns, rows, maxNodes } envia limit = gridPageSize(columns, rows, maxNodes)`.
    - `BBX-067/useTabGames envia sort 'totalPlayed' e a chave da consulta o inclui`.
  - `hints.test.ts` (opções novas `layout`, `zone`; um caso por linha do contrato §8):
    - `TDK-024/Jogos com jogos`: A Jogar, X Detalhes, LB/RB Plataforma, Select+B Fechar o jogo, B Sair, e Y Som só com vídeo visível.
    - `TDK-024/Jogos aba vazia`: LB/RB Plataforma, ← Plataformas, B Sair.
    - `TDK-024/Plataformas`: ↑/↓ Plataforma, A Ver jogos, LB/RB Plataforma, B Voltar, Select+B Fechar o jogo.
    - `TDK-024/Painel com foco em Jogar` e `em Jogar sem contar tempo`: A <ação>, ↑/↓ Mover, B Voltar, Select+B.
    - `TDK-024/Painel no visor`: ←/→ Imagem, ↓ Botões, B Voltar, Select+B.
    - `TDK-024/nenhuma dica de Select+A`.
    - `TDK-024/layout screens devolve exatamente o de hoje (↑ Botões no visor)`.
  - `desktopLayout.test.ts`:
    - `TDK-007/desktopGrid` com os 7 vetores do contrato §3: 1920×1080 grade 6×2; 1366×768 4×1; 1280×720 3×1; 3840×2160 17×6; 1920×1080 lista 1×13; 1280×720 lista 1×7; 0×0 grade 4×2 e lista 1×8.
    - `TDK-007/largura ou altura ≤ 0 usa o padrão`.
    - `TDK-007/rows nunca é menor que 1`.
    - `TDK-034/columnsFor, usableWidth, GRID_GAP, GRID_INSET, CARD_MIN_WIDTH, CARD_TITLE_HEIGHT, SCROLLBAR_SIZE e LIST_ROW_HEIGHT reexportados têm a mesma referência de lib/gridNav`.
    - `TDK-007/constantes do cromo (65, 24, 16, 32, 12, 48, 12) e DESKTOP_MAX_NODES = 200`.
    - `TDK-005/coverPlaceholderUrl é uma string não vazia`.
  - `desktopLabels.test.ts` (fixture com os mesmos textos de `GameDetailsPanel` linhas ~205-245): `TDK-008/status, tempo total, primeira e última vez, concluído em (só se houver), modos, gêneros, desenvolvedora, editora e ano`; `TDK-008/vazio vira "—"`; `TDK-008/favorito`.

## T06 — Frontend: contrato de tema, capacidade, janela, modelo, dados, dicas
- Requisitos: BBX-061, BBX-062, BBX-063, BBX-066, BBX-067, BBX-068, TDK-004, TDK-005, TDK-006, TDK-007, TDK-008, TDK-019, TDK-020, TDK-024, TDK-034. **Responsável:** implementer. Dependência: T05, T02, T04.
- Arquivos:
  - `frontend/src/bigbox/themes/types.ts`
  - `frontend/src/bigbox/viewModel.ts`
  - `frontend/src/bigbox/useBigBoxData.ts`
  - `frontend/src/bigbox/hints.ts`
  - `frontend/src/bigbox/desktopLayout.ts` (novo)
  - `frontend/src/bigbox/desktopLabels.ts` (novo)
  - `frontend/src/bigbox/BigBoxDetails.tsx` (só o tipo `BigBoxDetailsProps`)
  - `frontend/src/lib/uiPrefs.ts` (unificar `BigBoxThemeId`, Dúvida 4)
- Assinaturas:
```ts
// themes/types.ts
export type { LibraryView } from '../viewModel'
export type BigBoxThemeProps = { /* atual */ onTab?: (index: number) => void }
export type BigBoxThemeDef = { /* atual */
  videoSlot: 'background' | 'cover' | 'panel'
  sort: 'title' | 'lastPlayed' | 'inherit'
  layout?: 'screens' | 'columns'
  gridColumns?: (vp: Viewport, view: LibraryView) => number
  gridRows?: (vp: Viewport, view: LibraryView) => number
  maxNodes?: number
  gridScroll?: 'focusTop' | 'edges'
}
// viewModel.ts
export type BigBoxThemeId = 'faixa' | 'mosaico' | 'estante' | 'desktop' | 'roda'
export type LibraryView = 'grid' | 'list'
export type BigBoxTab = { /* atual */ iconUrl?: string | null; count?: number | null }
export type BigBoxGame = { /* atual */ coverFit?: 'contain' | 'cover'; platform?: string; playTimeText?: string; lastPlayedText?: string }
export type BigBoxViewModel = { /* atual */ layout: 'screens' | 'columns'; zone: NavZone; view: LibraryView }
export type ViewModelInput = { /* atual */
  sort: SortKey; sortInherited?: boolean
  layout?: 'screens' | 'columns'; zone?: NavZone; view?: LibraryView; startRow?: number
}
export function effectiveRows(rows: number, columns: number, maxNodes?: number): number
export function gridWindowAt(total: number, startRow: number, columns: number, rows: number): { start: number; end: number }
export function edgeScrollRow(prev: number, row: number, visibleRows: number, totalRows: number): number
export function centeredRow(row: number, visibleRows: number, totalRows: number): number
export function gridWindow(total: number, focus: number, columns: number, rows: number, maxNodes?: number): { start: number; end: number } // 5º parâmetro novo
export function buildTabs(catalog: CatalogView | undefined, initialPlatformId: number | null, kind: 'platforms' | 'collections', withIcons?: boolean): { tabs: BigBoxTab[]; initial: number }
// useBigBoxData.ts
export function gridPageSize(columns: number, rows: number, maxNodes?: number): number
export function shouldPrefetchGrid(loaded: number, total: number, focus: number, columns: number, rows: number, exhausted: boolean, maxNodes?: number): boolean
export type TabPaging = { kind: 'strip'; capacity: number } | { kind: 'grid'; columns: number; rows: number; maxNodes?: number }
// hints.ts
export function hintsFor(screen: 'browse' | 'details',
  o: { empty: boolean; videoVisible: boolean; detailsFocus?: DetailsFocus; hasGallery?: boolean; layout?: 'screens' | 'columns'; zone?: NavZone },
  tabsLabel?: 'Plataforma' | 'Aba'): Hint[]
// desktopLayout.ts
export { columnsFor, usableWidth, GRID_GAP, GRID_INSET, CARD_MIN_WIDTH, CARD_TITLE_HEIGHT, SCROLLBAR_SIZE, LIST_ROW_HEIGHT } from '../lib/gridNav'
export const DESKTOP_MAX_NODES = 200
export const HEADER_PX = 65, CENTER_PAD_X = 24, CENTER_PAD_Y = 16, TOOLBAR_PX = 32, TOOLBAR_GAP = 12, FOOTER_PX = 48, FOOTER_GAP = 12
export function desktopGrid(vp: { width: number; height: number }, view: LibraryView): { columns: number; rows: number }
export const coverPlaceholderUrl: string        // import '../assets/cover-placeholder.svg'
// desktopLabels.ts (pode importar lib/)
export type DesktopLabels = {
  platform: string; status: string; totalTime: string; firstPlayed: string; lastPlayed: string; completedAt: string | null
  playModes: string; genres: string; developers: string; publishers: string; year: string; favorite: boolean
}
export function desktopLabels(g: GameDetailView): DesktopLabels
// BigBoxDetails.tsx
export type BigBoxDetailsProps = { /* atual */
  active?: boolean                                  // padrão true (BBX-064)
  platformIconUrl?: string | null
  coverUrl?: string | null
  preview?: { gameId: number; videoUrl: string | null; active: boolean; failed: boolean; sound: boolean; onVideoError: (id: number) => void }
  labels?: DesktopLabels
}
```
- `buildViewModel`: com `layout = columns` preenche os campos novos de jogo e `withIcons` fica por conta do chamador; `sortLabel` usa `sortLabel(k)` de `lib/filters` quando `sortInherited`; com `startRow` a janela é `gridWindowAt(total, startRow, columns, ⌊capacity/columns⌋)`, senão o `gridWindow` atual. `hintsFor` segue o contrato §8 com `layout = columns` e devolve o de hoje com `screens`. Fórmulas de `desktopGrid` no contrato §3.

## T07 — Testes: shell com layout `columns` (tema falso), `useGridStart`, `App` repassa ordem e visualização
- Requisitos: BBX-061, BBX-062, BBX-063, BBX-064, BBX-065, BBX-067, BBX-068, TDK-010, TDK-011, TDK-019, TDK-020, TDK-033. **Responsável:** test-writer. Dependência: T04 (tipos), T05.
- Arquivos (testes):
  - `frontend/src/bigbox/useGridStart.test.tsx` (novo, `renderHook`)
  - `frontend/src/bigbox/BigBox.test.tsx` (acrescentar; `setup` passa `librarySort: 'title'` e `libraryView: 'grid'`; registra um tema falso `columns` em `BIGBOX_THEMES` e o remove em `afterEach`)
  - `frontend/src/App.test.tsx` (acrescentar, com tema falso `sort: 'inherit'` pré-selecionado via `arcadia.bigBoxTheme`)
- Testes:
  - `useGridStart`:
    - `BBX-063/edges: andar dentro das fileiras inteiras visíveis não rola; passar da última rola uma fileira; passar da primeira também`.
    - `BBX-063/troca de tabKey centraliza (centeredRow)`.
    - `BBX-063/recenter() centraliza no próximo cálculo e só nele`.
    - `BBX-063/resize que muda R' reaplica edgeScrollRow com a fileira guardada`.
    - `BBX-049/focusTop devolve clamp(⌊f/C⌋, 0, max(⌈T/C⌉-R', 0))`.
  - `BigBox.test.tsx`:
    - `BBX-061/tema sem os campos novos usa colunas, fileiras e 30 nós como hoje` (os testes atuais de Faixa, Mosaico e Estante seguem verdes).
    - `BBX-062/o limit do ListGames é gridPageSize(C, R', N) do tema falso (N = 200)`.
    - `BBX-062/nunca mais de (R'+1)·C cartões no DOM`.
    - `BBX-063/andar para baixo só rola ao passar da última fileira inteira`.
    - `BBX-063/na entrada, na troca de aba e depois de game:closedByHotkey a fileira do foco fica no meio`.
    - `BBX-063/clicar num cartão (onPick) não recentra`.
    - `BBX-064/layout columns monta o Details também em Navegar, com active=false`.
    - `BBX-064/mesmos bindings chamados que num tema screens (nenhuma leitura nova)`.
    - `BBX-064/em Detalhes active=true`.
    - `BBX-065/onTab(i) ativa a aba, foco no primeiro jogo e lê a lista da aba nova`.
    - `BBX-065/onTab da aba ativa não faz nada`.
    - `BBX-065/onTab durante a suspensão é ignorado`.
    - `BBX-065/onTab a partir de Detalhes volta a Navegar`.
    - `BBX-067/sort inherit envia o librarySort ao ListGames (totalPlayed) e o rótulo do modelo é o do seletor`.
    - `BBX-068/libraryView list dá C=1 em gridColumns e gridRows`.
    - `BBX-068/a visualização não é gravada pelo Big Box (localStorage intacto)`.
    - `TDK-010/videoSlot panel: o fundo nunca toca vídeo; preview.active é true em Navegar e false em Detalhes`.
    - `TDK-011/tempo para zerar: cache lido na hora, busca depois de 1000 ms em Navegar (timeToBeatInBrowse) e na hora ao entrar em Detalhes` (espião de `TimeToBeatGet`).
    - `TDK-020/uma leitura em voo por aba, sem repetir nem cancelar ao mover o foco; troca de aba cancela; ↓ com a página seguinte pendente não passa de M` (promessas controladas).
    - `TDK-033/entrada suspensa com o jogo lançado; Select segurado bloqueia A, B, Y e ↑/↓; depois de Select+B o foco volta ao jogo que rodava com zona games`.
  - `App.test.tsx`: `BBX-067/BBX-068/o App passa query.sort e a visualização da janela normal ao BigBox` (tema falso `sort: 'inherit'`; ListGames recebe a ordenação da janela; com visualização em lista o tema falso recebe `view = 'list'`).

## T08 — Frontend: shell com colunas, rolagem pelas bordas, Detalhes em Navegar e `onTab`
- Requisitos: BBX-061, BBX-062, BBX-063, BBX-064, BBX-065, BBX-067, BBX-068, TDK-010, TDK-011, TDK-017, TDK-019, TDK-020, TDK-025, TDK-031, TDK-033. **Responsável:** implementer. Dependência: T07, T06.
- Arquivos:
  - `frontend/src/bigbox/useGridStart.ts` (novo)
  - `frontend/src/bigbox/BigBox.tsx`
  - `frontend/src/bigbox/Footer.tsx` (`data-bb-footer=""` no `Panel`)
  - `frontend/src/App.tsx` (só `librarySort={query.sort}` e `libraryView={view}`)
- Assinaturas:
```ts
// useGridStart.ts — só para temas de grade
export type GridStartInput = { mode: 'focusTop' | 'edges'; focus: number; total: number; columns: number; rows: number; tabKey: string }
export function useGridStart(i: GridStartInput): { startRow: number; recenter: () => void }
// BigBox.tsx
export type BigBoxProps = { /* atual */ librarySort: SortKey; libraryView: LibraryView }
```
- Mudanças no shell: `C = theme.gridColumns?.(viewport, libraryView) ?? theme.columns`, `R = theme.gridRows?.(viewport, libraryView) ?? 1`, `N = theme.maxNodes ?? GRID_MAX_NODES`, `R' = effectiveRows(R, C, N)`; `C` vai para `NavCtx.columns`, `NavCtx.axis`/`layout`, `buildViewModel` e `useTabGames({ kind: 'grid', columns: C, rows: R', maxNodes: N })`; `shouldPrefetchGrid(..., N)`; `sort = theme.sort === 'inherit' ? p.librarySort : theme.sort` em `useTabGames` e no modelo (`sortInherited`); `startRow` de `useGridStart` (`recenter()` na entrada, em `gameClosed` e nunca em `onPick`) vai ao modelo e à chave de capas; `buildTabs(..., theme.layout === 'columns')`; com `layout = columns` o nó de Detalhes é montado sempre que há `data.detail`, com `active = nav.screen === 'details'`, `platformIconUrl` (aba `p:${platformId}`), `coverUrl`, `preview` e `labels = desktopLabels(detail)`; `BackgroundStage` só recebe vídeo com `videoSlot = 'background'`; `onTab={(i) => { setAlert(null); dispatchNav({ type: 'tabSet', index: i }) }}`.
- Nada de binding novo: `useFocusedData` já lê detalhe, mídia e sessão do jogo em foco.

## T09 — Testes: tokens e CSS do Desktop
- Requisitos: BBX-069, TDK-001, TDK-027, TDK-028, TDK-034. **Responsável:** test-writer. Dependência: nenhuma.
- Arquivo: `frontend/src/theme.test.ts` (acrescentar; reusa `block`, `luminance`, `contrast`).
- Testes:
  - `TDK-027/o bloco [data-bigbox-theme="desktop"] está dentro de [data-theme="dark"] e define os 15 tokens --arc-bb-* mais --arc-bb-safe-y e --arc-bb-safe-x`.
  - `TDK-027/cada token de cor tem o valor do token do tema escuro da tabela do contrato §7` (um caso por par: bg=bg, bg-2=surface, panel=surface, text=text, text-muted=text-muted, heading=text, accent=play, on-accent=on-accent, surface=surface-2, placeholder=border, chip=surface-3, on-chip=text, focus=focus, danger=danger).
  - `TDK-028/text, text-muted, heading e danger têm contraste ≥ 4,5 com panel, bg e surface`.
  - `TDK-028/on-accent sobre accent e on-chip sobre chip ≥ 4,5`.
  - `TDK-028/focus ≥ 3 contra panel, bg e surface`.
  - `BBX-069/safe-y e safe-x: faixa, mosaico e estante 5vh/5vw; desktop 0/0`.
  - `TDK-001/o rodapé do Desktop tem height 48px pela regra [data-bigbox-theme="desktop"] [data-bb-footer]`.
  - `TDK-034/--col-left e --col-right continuam únicos e com os clamp de UIX-005` (o Desktop não os redefine).

## T10 — Frontend: bloco de tokens do Desktop
- Requisitos: BBX-069, TDK-027, TDK-028. **Responsável:** implementer. Dependência: T09.
- Arquivo: `frontend/src/index.css`: bloco `[data-bigbox-theme="desktop"]` aninhado em `[data-theme="dark"]` (depois do `estante`), com os valores do contrato §7 (`--arc-bb-bg: #16181d`, `--arc-bb-bg-2: #1e2128`, `--arc-bb-scrim: #00000000`, `--arc-bb-panel: #1e2128`, `--arc-bb-text: #e6e8ec`, `--arc-bb-text-muted: #9aa3b2`, `--arc-bb-heading: #e6e8ec`, `--arc-bb-accent: #238636`, `--arc-bb-on-accent: #ffffff`, `--arc-bb-surface: #272b34`, `--arc-bb-placeholder: #343a46`, `--arc-bb-chip: #323744`, `--arc-bb-on-chip: #e6e8ec`, `--arc-bb-focus: #79b8ff`, `--arc-bb-danger: #ff6b64`, `--arc-bb-safe-y: 0`, `--arc-bb-safe-x: 0`) e a regra `[data-bigbox-theme="desktop"] [data-bb-footer] { height: 48px }`. Valores copiados como literais (o teste de contraste lê hexadecimais); literais só dentro do bloco do tema (`theme-scan`).

## T11 — Testes: componentes do tema Desktop e registro
- Requisitos: BBX-060, BBX-061, BBX-065, TDK-001, TDK-002, TDK-003, TDK-004, TDK-005, TDK-006, TDK-008, TDK-009, TDK-010, TDK-011, TDK-018, TDK-019, TDK-021, TDK-022, TDK-023, TDK-025, TDK-026, TDK-029, TDK-030. **Responsável:** test-writer. Dependência: T05 (fixtures do modelo), T06.
- Arquivos (testes):
  - `frontend/src/bigbox/themes/desktop/Desktop.test.tsx` (novo)
  - `frontend/src/bigbox/themes/desktop/DesktopSidebar.test.tsx` (novo)
  - `frontend/src/bigbox/themes/desktop/DesktopGrid.test.tsx` (novo; cobre a lista também)
  - `frontend/src/bigbox/themes/desktop/DesktopDetails.test.tsx` (novo)
  - `frontend/src/bigbox/themes/registry.test.ts` (alterar e acrescentar)
- Testes:
  - `registry.test.ts`:
    - `BBX-060/o registro lista faixa, mosaico, estante, desktop e roda nesta ordem; desktop se chama "Desktop" e está disponível; roda indisponível`.
    - `BBX-061/valores do Desktop: axis grid, layout columns, gridScroll edges, videoSlot panel, sort inherit, maxNodes 200, backdrop plain, tabs platforms, timeToBeatInBrowse true, gridColumns e gridRows funções, Details função`.
    - `BBX-061/Faixa, Mosaico e Estante não declaram layout, gridColumns, maxNodes nem gridScroll`.
    - `BBX-060/resolveTheme('desktop') devolve o Desktop; "foo" e "roda" caem em faixa`.
    - O teste de forma de BBX-047 passa a aceitar `panel` e `inherit` (anotado).
  - `Desktop.test.tsx`:
    - `TDK-001/marcos nesta ordem: header, nav "Plataformas", região "Jogos" e aside "Detalhes do jogo"`.
    - `TDK-001/a grade usa as variáveis --col-left e --col-right`.
    - `TDK-001/sem bb-art e sem arte de fundo`.
    - `TDK-002/o header mostra "Arcadia" e nenhum elemento acionável (button, select, input, a[href], [role=button], [role=combobox])`.
    - `TDK-002/nenhum botão nem combobox com os rótulos Big Box, Tema do Big Box, Importar, Provedores, Emuladores, Plataformas (botão), Estatísticas em todo o DOM`; "Plataformas" aparece como texto do título da coluna.
    - `TDK-004/barra: "120 jogos", "1 jogo" com T=1, "Ordenar por: <rótulo>" e "Grade"/"Lista" só como texto (sem select nem button)`.
    - `TDK-026/carregando: "Carregando jogos…" no centro e "Nenhum jogo selecionado" no painel`.
    - `TDK-026/aba vazia: "Nenhum jogo nesta plataforma" (ou "Nenhum jogo na biblioteca" em Todos)`.
    - `TDK-026/jogo sem detalhe: "Carregando jogo…"`.
    - `TDK-026/erro em role="alert" logo abaixo do cabeçalho, sem botão de fechar`.
    - `TDK-029/o centro é listbox "Jogos" com opções (aria-label com o título, aria-selected no foco)`.
    - `TDK-023/exatamente um elemento com data-focused="true" por zona (games, platforms, painel)`.
    - `TDK-023/fora da zona games o jogo em foco mantém o destaque de seleção sem data-focused`.
  - `DesktopSidebar.test.tsx`:
    - `TDK-003/título "Plataformas", Todos com a soma e cada plataforma com ícone, nome e contagem; sem busca nem Filtros`.
    - `TDK-003/aba ativa com aria-current="true"`.
    - `TDK-029/<nav aria-label="Plataformas"> com nome "<plataforma>, N jogos"`.
    - `BBX-065/TDK-025/clicar numa aba chama onTab(i)`.
    - `TDK-021/scrollIntoView({ block: 'nearest' }) no item ativo quando a aba muda; ausente no jsdom é tolerado`.
  - `DesktopGrid.test.tsx`:
    - `TDK-005/cartão com capa 3:4, título em duas linhas com title completo, "Jogado DD/MM/AAAA HH:MM" e selo "Em execução"/"Pausado"; placeholder sem imagem`.
    - `TDK-006/linha de 64 px com título, plataforma, tempo, última vez ("Nunca") e marcação, sem botões`.
    - `TDK-025/clique foca (onPick com o índice absoluto stripStart + i), duplo clique lança (onPlay)`.
    - `TDK-019/desenha só os cartões da janela e a última fileira pode ficar cortada; sem barra de rolagem`.
    - `TDK-023/zona games: contorno de foco no jogo em foco`.
  - `DesktopDetails.test.tsx`:
    - `TDK-008/título, ícone e nome da plataforma, marcação de sessão, Jogar largo e Jogar sem contar tempo abaixo, campos na ordem (Tempo total, Primeira vez jogado, Última vez jogado, Tempo para zerar, Concluído em, Status, Marcação, Gêneros, Modos de jogo, Desenvolvedora, Editora, Ano) com "—" para vazio e a Descrição inteira`.
    - `TDK-008/ao trocar de jogo o painel volta ao topo`.
    - `TDK-009/visor com n = 0 (capa ou placeholder, sem foco), n = 1 (sem miniaturas) e n = 3 (rótulo "Vídeo"/"Screenshot", "k de n" em aria-live polite e miniaturas com a selecionada marcada)`; miniaturas sem papel de botão.
    - `TDK-010/Navegar: capa com FocusVideo depois de 2.000 ms; no máximo um <video>, dentro do visor`.
    - `TDK-010/Detalhes: o <video> de Navegar não existe e a galeria toca só o item selecionado`.
    - `TDK-010/failed não tenta de novo`.
    - `TDK-011/o painel mostra o tempo para zerar sem botões de atualizar ou confirmar`.
    - `TDK-018/active=true marca o foco em Jogar, Jogar sem contar tempo ou visor (uma só marca)`.
    - `TDK-018/o analógico direito rola o painel (scrollSignal) sem mover o foco`.
    - `TDK-022/o elemento focado é levado à vista (scrollIntoView block nearest)`.
    - `TDK-025/Jogar e Jogar sem contar tempo chamam onPlay e onPlayUntracked`.
    - `TDK-030/nenhum Editar, Buscar metadados, Buscar vídeo, Abrir local do arquivo, Remover, Adicionar sessão, Carregar mais, Ampliar, lista de sessões, painel de sessão, "Sem emulador configurado" nem "Rastreio por processo" no DOM`.

## T12 — Testes de DOM: painel só com os dois botões (TDK-037) e nenhum controle fora da lista (TDK-038)
- Requisitos: TDK-002, TDK-030, TDK-037, TDK-038. **Responsável:** test-writer. Dependência: T06 (usa o registro real do Desktop, vermelho até a T15).
- Arquivos (testes):
  - `frontend/src/bigbox/themes/desktop/DesktopDetails.buttons.test.tsx` (novo; TDK-037)
  - `frontend/src/bigbox/BigBox.desktop.dom.test.tsx` (novo; TDK-038)
- Seletor de acionáveis (constante compartilhada nos dois arquivos): `button, a[href], input, select, textarea, [role="button"], [role="link"], [role="menuitem"], [contenteditable]`.
- Testes:
  - TDK-037, `describe.each` sobre a matriz de fixtures (com e sem vídeo; com e sem screenshots; sessão aberta ou não; favorito ou não; concluído ou não; tempo para zerar em cache, pendente e ausente; emulador sem emulador configurado; credenciais do ScreenScraper prontas ou não), com `active = false` e com `active = true` (foco em botão e no visor):
    - `TDK-037/o aside tem exatamente dois botões, "Jogar" e "Jogar sem contar tempo", nesta ordem`.
    - `TDK-037/a busca pelos seletores acionáveis devolve só esses dois (visor e miniaturas não são botões)`.
    - `TDK-037/Jogar chama só LaunchGame e Jogar sem contar tempo só LaunchGameUntracked`.
    - `TDK-037/"Nenhum jogo selecionado" e "Carregando jogo…" não têm nenhum botão`.
  - TDK-038, shell com o tema `desktop` registrado e bindings falsos, a cada estado (Jogos em grade, Jogos em lista, Plataformas, Painel no botão e no visor, aba vazia, primeira página carregando, detalhe carregando, aviso de erro, sessão aberta, vídeo tocando):
    - `TDK-038/os únicos acionáveis de todo o DOM do tema são os itens de plataforma (um por aba) e os dois botões do painel`; falha se aparecer qualquer controle novo.
    - `TDK-038/cartões e linhas são role="option" e o <video> não tem controls`.
    - `TDK-038/nenhum elemento tem nome acessível ou texto igual a um dos rótulos proibidos` (Big Box, Tema do Big Box, Importar, Provedores, Emuladores, Estatísticas, Editar, Buscar metadados, Buscar vídeo, Abrir local do arquivo, Remover, Tentar de novo, Confirmar…, Escolher outro…, Buscar, Nenhum destes, Adicionar sessão, Carregar mais, Ampliar, Limpar filtros, Pausar, Retomar) por `queryByRole` e `queryByText` exatos; "Plataformas" só como texto do título da coluna.
    - `TDK-002/o cabeçalho do tema não tem acionável em nenhum desses estados`.

## T13 — Testes: shell com o tema Desktop real (zonas, controle, mouse, só leituras)
- Requisitos: TDK-011, TDK-012, TDK-016, TDK-017, TDK-018, TDK-019, TDK-020, TDK-024, TDK-025, TDK-031, TDK-033. **Responsável:** test-writer. Dependência: T08 (e vermelho até a T15).
- Arquivo: `frontend/src/bigbox/BigBox.desktop.test.tsx` (novo). `navigator.getGamepads` falso para controle e `KeyboardEvent` para teclado; bindings falsos com espiões em todos.
- Testes:
  - `TDK-012/entra em Jogos com o foco no jogo em foco da janela normal (BBX-027) e o painel mostra o jogo em foco em qualquer zona`.
  - `TDK-016/X leva o foco ao Painel em Jogar; LB/RB trocam de aba sem mudar a zona`.
  - `TDK-017/A (Enter) chama LaunchGame uma vez; tecla mantida não relança; B sai do Big Box e a janela normal reaparece; Y liga e desliga o som do vídeo visível`.
  - `TDK-018/← na primeira coluna vai a Plataformas, → na última vai ao Painel, ← num botão volta a Jogos, B volta a Jogos com o mesmo jogo, A no botão lança com e sem contar tempo e A no visor não faz nada`.
  - `TDK-019/andar pela grade só rola ao passar da última fileira inteira e o foco nunca fica fora da área visível`.
  - `TDK-020/pré-busca por páginas com R' (uma leitura em voo por aba)`.
  - `TDK-011/tempo para zerar em Navegar: busca depois de 1.000 ms com o mesmo jogo; no Painel, na hora; nenhum botão de atualizar`.
  - `TDK-024/o rodapé muda a cada zona (Jogos, Plataformas, botão, visor, aba vazia) e nunca mostra Select+A`.
  - `TDK-025/clicar num cartão foca e volta a zona a Jogos; duplo clique lança; clicar numa plataforma a ativa; Jogar do painel lança; um lançamento pendente bloqueia o seguinte (UI-021)`.
  - `TDK-031/em toda a navegação só são chamados ListGames, GetGame, MediaList, Catalog, CurrentSession, TimeToBeatGet, LaunchGame, LaunchGameUntracked e BringToFront; nenhum download de imagem ou vídeo`.
  - `TDK-033/suspensão enquanto o jogo roda; Select segurado bloqueia A, B, Y e ↑/↓; depois de game:closedByHotkey o Big Box volta com zona Jogos, foco no jogo que rodava e a fileira dele no meio`.

## T14 — Testes: varreduras do Big Box com o tema Desktop
- Requisitos: BBX-066, BBX-070, TDK-030, TDK-034. **Responsável:** test-writer. Dependência: nenhuma (fica vermelho até a T15).
- Arquivos (testes): `frontend/src/bigbox/bigbox-scan.test.ts` (acrescentar). `theme-scan.test.ts` não muda (varre todo `.tsx`, inclusive o Desktop).
- Testes:
  - `BBX-070/sanidade: os arquivos Desktop.tsx, DesktopSidebar.tsx, DesktopToolbar.tsx, DesktopGrid.tsx, DesktopList.tsx e DesktopDetails.tsx de themes/desktop/ entram na varredura`.
  - `BBX-070/nenhum import de components/ em bigbox/ (BBX-003), sem exceção nova`.
  - `BBX-070/os arquivos de themes/desktop/ não importam lib/ nem bindings (BBX-047); só bigbox/ e o asset`.
  - `BBX-070/nenhuma cor literal nem classe de paleta em themes/desktop/ (BBX-025)`.
  - `BBX-070/nenhum binding de escrita nem window.go em themes/desktop/ (BBX-003, BBX-029)`.
  - `BBX-070/o tempo para zerar só pelo modelo: nenhum useTimeToBeat nem fetchAfterMs em themes/desktop/ (BBX-059)`.
  - `BBX-066/o import de lib/platformIcons é aceito e o de lib/platforms continua proibido em bigbox/`.
  - `TDK-030/nenhum arquivo de themes/desktop/ importa PlatformNav, GameDetailsPanel, SessionsList, GameCard, CoverImage, LibraryGrid nem LibraryList`.
  - `TDK-034/o tema usa var(--col-left) e var(--col-right) e não redefine constantes de layout (nenhum literal 160, 16, 60, 6 ou 64 fora de desktopLayout.ts)`.
  - `BBX-037/nenhum --arc-bb-scrim fora do BackgroundStage` (regra atual, vale também para o Desktop).

## T15 — Frontend: tema Desktop (componentes) e entrada no registro
- Requisitos: BBX-060, BBX-061, BBX-065, BBX-070, TDK-001, TDK-002, TDK-003, TDK-004, TDK-005, TDK-006, TDK-008, TDK-009, TDK-010, TDK-011, TDK-018, TDK-019, TDK-021, TDK-022, TDK-023, TDK-025, TDK-026, TDK-029, TDK-030, TDK-031, TDK-034, TDK-037, TDK-038. **Responsável:** implementer. Dependência: T06, T08, T10, T11, T12, T13, T14.
- Arquivos (todos novos, exceto o registro): `frontend/src/bigbox/themes/desktop/{Desktop,DesktopSidebar,DesktopToolbar,DesktopGrid,DesktopList,DesktopDetails}.tsx` e `frontend/src/bigbox/themes/registry.ts` (entrada `desktop` depois de `estante` e antes de `roda`, como no design §2).
- Assinaturas:
```ts
// Desktop.tsx: raiz do tema (cabeçalho só com "Arcadia", faixa de erro, 3 colunas); details vem pronto do shell
export function Desktop(p: BigBoxThemeProps): JSX.Element
// DesktopSidebar.tsx
export type DesktopSidebarProps = { tabs: BigBoxTab[]; tab: number; focused: boolean; onTab: (index: number) => void }
export function DesktopSidebar(p: DesktopSidebarProps): JSX.Element
// DesktopToolbar.tsx — só leitura
export type DesktopToolbarProps = { total: number; sortLabel: string; view: LibraryView }
export function DesktopToolbar(p: DesktopToolbarProps): JSX.Element
// DesktopGrid.tsx e DesktopList.tsx (sem virtualizador; role="listbox" fica no pai)
export type DesktopItemsProps = {
  games: BigBoxGame[]; start: number; focus: number; columns: number; zoneFocused: boolean
  onPick: (index: number) => void; onPlay: (index: number) => void
}
export function DesktopGrid(p: DesktopItemsProps): JSX.Element
export function DesktopList(p: DesktopItemsProps): JSX.Element
// DesktopDetails.tsx — aside completo, ativo e inativo; só os botões Jogar e Jogar sem contar tempo
export function DesktopDetails(p: BigBoxDetailsProps): JSX.Element
// registry.ts
{ id: 'desktop', name: 'Desktop', available: true, axis: 'grid', columns: 1, anchor: 0, layout: 'columns',
  gridColumns: (vp, view) => desktopGrid(vp, view).columns, gridRows: (vp, view) => desktopGrid(vp, view).rows,
  listCapacity: (vp) => { const g = desktopGrid(vp, 'grid'); return g.columns * g.rows },
  maxNodes: DESKTOP_MAX_NODES, gridScroll: 'edges', backdrop: 'plain', videoSlot: 'panel', tabs: 'platforms',
  sort: 'inherit', timeToBeatInBrowse: true, Component: Desktop, Details: DesktopDetails }
```
- Regras de implementação: sem `components/` nem `lib/` (formatação vem do shell em `labels`); tokens só por utilitários `bb-*` e `var(--arc-bb-*)`; um `<video>` por vez (o `FocusVideo` do visor só com `!active`, o `Video` da galeria só com `active`); `scrollIntoView?.()` com `?.`; nenhum `button` fora dos itens de plataforma e dos dois botões do painel; cartões e linhas `role="option"` sem foco do DOM; nenhuma classe/estilo de cor literal.

## T16 — Testes: seletor, preferência e invariantes da UI com o Desktop
- Requisitos: BBX-060, TDK-032. **Responsável:** test-writer. Dependência: T15. Esperado verde; se algum caso falhar, o implementer corrige em `frontend/src/App.tsx` (nenhuma mudança esperada além do repasse da T08).
- Arquivos (testes): `frontend/src/App.test.tsx` (seção do Big Box: opções do seletor, anotado), `frontend/src/lib/uiPrefs.test.ts` (acrescentar), `frontend/src/App.invariants.test.tsx` (acrescentar).
- Testes:
  - `BBX-060/o seletor "Tema do Big Box" lista Faixa, Mosaico, Estante, Desktop e "Roda (em breve)" desabilitada`.
  - `BBX-060/escolher "Desktop" grava desktop em arcadia.bigBoxTheme e a próxima entrada (botão, F11) monta data-bigbox-theme="desktop"`.
  - `BBX-060/uiPrefs: readBigBoxTheme lê "desktop" e writeBigBoxTheme o grava; ausente, desconhecido e "roda" viram faixa`.
  - `TDK-032/a navegação do tema Desktop (zonas, abas, painel, som, lançar) não chama PauseSession, ResumeSession, ToggleSession nem fecha jogo, e a UI não encerra sessão`.

## T17 — Documentação
- Requisitos: documentação das mudanças de BBX-060..BBX-070 e TDK-001..TDK-038 (sem requisito próprio). **Responsável:** sessão principal (docs). Dependência: T15.
- Arquivos:
  - `docs/ARCHITECTURE.md`: seção do Big Box (temas, contrato com `layout`, zonas, tema Desktop, ADR 0042).
  - `docs/adr/README.md`: linha do 0042 (hoje ausente).
  - Specs de origem, anotação `· alterado em 2026-11-27 (docs/specs/phase-17-tema-desktop, ID)` (Dúvida 6): `docs/specs/phase-13-modo-console/requirements.md` (BBX-002, BBX-004, BBX-015, BBX-016, BBX-032, BBX-033, BBX-042) e `docs/specs/phase-15-big-box-v2/requirements.md` (BBX-046..BBX-050, BBX-053, BBX-055, BBX-056).
  - `CLAUDE.md`: linha "Spec atual" passa a `phase-17-tema-desktop` ("Tema Desktop do Big Box", sobre a fase 16); linha da versão 1.3.0.
  - `README.md` e `docs/ROADMAP.md` (seção "Fase 17" e "Versão 1.3.0"); `docs/GUIA-DO-PROJETO.md` (tema Desktop, navegação por zonas, comandos).
  - `docs/specs/phase-17-tema-desktop/tasks.md` (este arquivo): marcar tarefas `[concluída]`.

## T18 — Versão 1.3.0
- Requisitos: nenhum requisito próprio (decisão de release). **Responsável:** sessão principal (mecânico). Dependência: T17.
- Arquivos: `cmd/arcadia/wails.json` (`productVersion`), `frontend/package.json` e `frontend/package-lock.json` (os dois campos `version` do topo; não tocar nas dependências), `CLAUDE.md` (comando de build `-X main.version=1.3.0` e "versão atual: 1.3.0"), `README.md` ("Current version" e comando de build), `docs/GUIA-DO-PROJETO.md` e `docs/ROADMAP.md` (comando de build). `cmd/arcadia/version.go` e `version_test.go` não mudam (a versão vem do `-ldflags`).

## T19 — Verificação final e roteiro manual
- Requisitos: TDK-035, TDK-036. **Responsável:** sessão principal, com o usuário (parte manual). Dependência: todas.
- Automático: `npm test` e `npx tsc --noEmit` em `frontend/`; `go build ./...`, `go vet ./...`, `CGO_ENABLED=0 go build ./...` e `go test ./...` (devem passar sem diff em Go); `scripts/spec-trace.ps1 -Spec docs/specs/phase-17-tema-desktop`; `wails build -ldflags "-X main.version=1.3.0"`. Conferir com `git diff --stat` que `internal/` e `cmd/fake-game/` não mudaram.
- **Manual (TDK-035), no monitor e na TV (inclusive Moonlight/Sunshine), com um controle XInput:**
  1. escolher o tema Desktop e entrar no Big Box pelo botão, F11 e Select + A;
  2. andar na grade e na lista com o direcional e o analógico; ← na primeira coluna vai às plataformas; → na última vai ao painel;
  3. ↑/↓ e LB/RB trocam de plataforma; X e A no painel lançam com e sem contar tempo;
  4. o vídeo entra no visor depois de cerca de 2 s;
  5. B volta um nível e só sai do Big Box em Jogos;
  6. Select + B fecha o jogo e devolve ao mesmo jogo, com a fileira dele no meio;
  7. nenhum botão de edição aparece, o painel só tem "Jogar" e "Jogar sem contar tempo" e o cabeçalho não tem botões.
- **Manual (TDK-036):** com o mesmo jogo, plataforma, ordenação e visualização, comparar capturas da janela normal e do Big Box Desktop em 1920×1080 e 1280×720 (cores, fontes, colunas da grade, painel), salvo as diferenças aceitas da spec.

---

## Riscos de concorrência e recursos

- **Goroutines e handles do Windows:** nenhum (sem Go). Frontend: nenhum temporizador novo fora dos que já limpam no desmonte (`useVideoDelay`, atraso de 1 s do tempo para zerar, `useMinuteClock`, `requestAnimationFrame` e `keydown` de `useBigBoxInput`, `resize` de `useViewport`).
- **Um único `<video>`:** o `FocusVideo` do visor (Navegar) e o `Video` da galeria (Detalhes) nunca coexistem (condicionados a `active`); o fundo do Desktop nunca toca vídeo; `src` removido ao desmontar.
- **`useGridStart`:** guarda a fileira inicial em `useRef` (sem efeito colateral fora do render e sem temporizador); `recenter()` só liga uma bandeira.
- **Regressão nos temas atuais:** mitigada pelos padrões dos campos novos (BBX-061) e pelos testes atuais de Faixa, Mosaico e Estante, que só mudam nos helpers de fixture (T03, T05, T07).
- **Resize durante a navegação:** `C` muda e a fileira do foco também; `edgeScrollRow` reaplica o `startRow` guardado, limitado a `maxStart`; a página já lida não muda.
- **Troca rápida de plataforma com ↑/↓ segurado:** cada troca cancela a leitura da aba anterior (BBX-038); as páginas lidas ficam em cache (T07, `TDK-020`).
- **Foco do DOM:** o Big Box não move o foco do DOM; `scrollIntoView` só para rolar, com `?.` por causa do jsdom.
- **A UI nunca influencia o tempo de jogo (TDK-032):** nenhuma tarefa chama binding de sessão; só `LaunchGame`, `LaunchGameUntracked` e `BringToFront`.

## Rastreabilidade

Colunas: tarefa(s) de código, tarefa(s) de testes. Conferência automática: `scripts/spec-trace.ps1 -Spec docs/specs/phase-17-tema-desktop` (`-SkipTests` antes das tarefas de teste). Verificação manual: TDK-035, TDK-036 (T19).

| Requisito | Código | Testes |
|---|---|---|
| BBX-060 | T15 | T11, T16 |
| BBX-061 | T06, T15 | T05, T07, T11 |
| BBX-062 | T06, T08 | T05, T07 |
| BBX-063 | T06, T08 | T05, T07 |
| BBX-064 | T08 | T07 |
| BBX-065 | T04, T08, T15 | T03, T07, T11 |
| BBX-066 | T02, T06 | T01, T05, T14 |
| BBX-067 | T02, T06, T08 | T01, T05, T07 |
| BBX-068 | T06, T08 | T07 |
| BBX-069 | T10 | T09 |
| BBX-070 | T15 | T14 |
| TDK-001 | T10, T15 | T09, T11 |
| TDK-002 | T15 | T11, T12 |
| TDK-003 | T15 | T11 |
| TDK-004 | T06, T15 | T05, T11 |
| TDK-005 | T06, T15 | T05, T11 |
| TDK-006 | T06, T15 | T05, T11 |
| TDK-007 | T06 | T05 |
| TDK-008 | T02, T06, T15 | T01, T05, T11 |
| TDK-009 | T15 | T11 |
| TDK-010 | T08, T15 | T07, T11 |
| TDK-011 | T08, T15 | T07, T11, T13 |
| TDK-012 | T04 | T03, T13 |
| TDK-013 | T04 | T03 |
| TDK-014 | T04 | T03 |
| TDK-015 | T04 | T03 |
| TDK-016 | T04 | T03, T13 |
| TDK-017 | T04, T08 | T03, T13 |
| TDK-018 | T04, T15 | T03, T11, T13 |
| TDK-019 | T06, T08, T15 | T05, T07, T11, T13 |
| TDK-020 | T06, T08 | T05, T07, T13 |
| TDK-021 | T15 | T11 |
| TDK-022 | T15 | T11 |
| TDK-023 | T15 | T11 |
| TDK-024 | T06 | T05, T13 |
| TDK-025 | T08, T15 | T11, T13 |
| TDK-026 | T15 | T11 |
| TDK-027 | T10 | T09 |
| TDK-028 | T10 | T09 |
| TDK-029 | T15 | T11 |
| TDK-030 | T15 | T11, T12, T14 |
| TDK-031 | T08, T15 | T13 |
| TDK-032 | (nenhum código novo) | T16 |
| TDK-033 | T04, T08 | T03, T07, T13 |
| TDK-034 | T02, T06, T15 | T01, T05, T09, T14 |
| TDK-035 | manual | T19 |
| TDK-036 | manual | T19 |
| TDK-037 | T15 | T12 |
| TDK-038 | T15 | T12 |

Lacunas explícitas: nenhum requisito sem tarefa. As dúvidas 1, 2, 3, 5 e 7 da lista do topo pedem ajuste de texto ao spec-writer.

## Decisões novas que merecem ADR

Nenhuma além do **0042**, já escrito e aceito (falta só o índice, T17). As escolhas deste plano (`useGridStart` com `recenter()`, `maxNodes` como último parâmetro opcional das funções puras, `LibraryView` em `viewModel.ts`, campos novos do modelo opcionais) são detalhes de implementação do 0042, não decisões de arquitetura.