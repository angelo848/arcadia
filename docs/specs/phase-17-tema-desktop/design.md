# Fase 17 — Tema Desktop do Big Box: design

Requisitos em [requirements.md](requirements.md) (aprovados em 2026-11-27, com as respostas padrão e TDK-037/TDK-038). Contrato em
[contracts/bigbox-tema-desktop.md](contracts/bigbox-tema-desktop.md). Cada seção cita os IDs que atende. ADRs: 0028, 0029, 0036, 0040;
proposto 0042 (layout em colunas no contrato de tema).

## 1. Mapa da mudança

| Lugar | O que muda | Quem escreve |
|---|---|---|
| `internal/*`, `cmd/arcadia`, IPC, banco | **nada** (sem método, evento, migração nem binding novo) | — |
| `internal/tracker`, `cmd/fake-game` | **nada** | — |
| `frontend/src/bigbox/` | contrato (campos novos), reducer (zonas, layout `columns`, `tabSet`), shell (colunas e fileiras dinâmicas, Detalhes em Navegar, `onTab`, ordem e visualização herdadas, rolagem pelas bordas, limite de nós), modelo (ícone e contagem nas abas, campos do cartão e da linha), `hintsFor` por zona, `desktopLayout.ts` (capacidade e reexportações), `themes/desktop/*` | implementer |
| `frontend/src/lib/` | adaptações mínimas, sem mudar comportamento: `platformIcons.ts` (leitura), `gameLabels.ts` (rótulos de status), `LIST_ROW_HEIGHT` em `gridNav.ts`, `sortLabel` exportado | implementer |
| `frontend/src/components/` | só passam a importar os helpers movidos (`PlatformNav`, `GameDetailsPanel`, `LibraryList`, `SortSelect`) | implementer |
| `frontend/src/App.tsx` | passa `librarySort` e `libraryView` ao `BigBox` | implementer |
| `frontend/src/index.css` | bloco `[data-bigbox-theme="desktop"]` e altura do rodapé no Desktop | implementer |

## 2. Contrato e registro [BBX-060, BBX-061, BBX-069]

Tipos em [contrato §1](contracts/bigbox-tema-desktop.md). Todos os campos novos são opcionais, com padrão igual ao comportamento atual,
para que Faixa, Mosaico e Estante não mudem (os testes atuais desses temas são a rede de segurança). `BigBoxThemeId` ganha `desktop` em
`viewModel.ts` e em `lib/uiPrefs.ts` (os dois tipos existem hoje; o planner pode unificá-los). O registro ganha a entrada:

```ts
{
  id: 'desktop', name: 'Desktop', available: true, axis: 'grid', columns: 1, anchor: 0,
  layout: 'columns', gridColumns: (vp, view) => desktopGrid(vp, view).columns, gridRows: (vp, view) => desktopGrid(vp, view).rows,
  listCapacity: (vp) => { const g = desktopGrid(vp, 'grid'); return g.columns * g.rows },
  maxNodes: DESKTOP_MAX_NODES, gridScroll: 'edges',
  backdrop: 'plain', videoSlot: 'panel', tabs: 'platforms', sort: 'inherit', timeToBeatInBrowse: true,
  Component: Desktop, Details: DesktopDetails,
}
```

inserida depois de `estante` e antes de `roda` (BBX-060). A área segura já é por bloco de tokens (`--arc-bb-safe-*` existe em cada bloco);
o Desktop só define `0` (BBX-069).

## 3. Reducer [TDK-012..TDK-018, BBX-065]

Tabela completa em [contrato §2](contracts/bigbox-tema-desktop.md). Implementação em `nav.ts`:

```ts
export type NavZone = 'games' | 'platforms'
export type NavState = { /* atual */ zone: NavZone }
export type NavCtx = { /* atual */ axis: 'horizontal' | 'vertical' | 'grid'; layout: 'screens' | 'columns' }
export type NavAction = /* atual */ | { type: 'tabSet'; index: number }
```

- O ramo de `up`/`down` em Navegar troca `if (cols <= 1) return none()` por `if (c.axis !== 'grid') return none()`; com `columns`,
  `zone = platforms` desvia para a troca de aba.
- `prev`/`next` em Navegar com `layout = columns` seguem a tabela (sem passar de fileira; zonas nas pontas). Com `screens`, o código
  atual.
- Em Detalhes, com `columns`, `up`/`down` seguem a ordem empilhada e `prev` num botão volta a Navegar; com `screens`, o código atual.
- `back`, `accept` e `next` em `platforms` voltam a `games` sem efeito; `listLoaded` e `gameClosed` forçam `zone = games`.
- `tabSet` é a única ação nova; não é ação de entrada (não entra em `inputTypes`) mas é ignorada durante a suspensão como as de
  entrada (o clique não pode trocar de aba com o jogo rodando).

O reducer continua puro e sem conhecer o tema; os testes atuais passam o `NavCtx` com `axis` e `layout = screens`.

## 4. Shell (`BigBox.tsx`) [BBX-062..BBX-068, TDK-010, TDK-011, TDK-019, TDK-020, TDK-033]

```ts
export type BigBoxProps = { /* atual */ librarySort: SortKey; libraryView: LibraryView } // BBX-067, BBX-068
```

1. **Colunas e fileiras:** `C = theme.gridColumns?.(viewport, libraryView) ?? theme.columns`, `R = theme.gridRows?.(viewport,
   libraryView) ?? 1`, `N = theme.maxNodes ?? GRID_MAX_NODES`, `R' = effectiveRows(R, C, N)` ([contrato §4](contracts/bigbox-tema-desktop.md)).
   `C` vai para `NavCtx.columns`, para `buildViewModel` e para `useTabGames({kind: 'grid', columns: C, rows: R'})`. Um resize que muda
   `C` mantém o índice `focus` (o jogo em foco não muda; só a fileira dele).
2. **Janela:** `gridScroll = edges` guarda `startRow` num `useRef` (por aba) e o atualiza a cada render pelo `edgeScrollRow`/`centeredRow`
   do contrato §5; `gridWindowAt(T, startRow, C, R')` substitui `gridWindow` nesse caso. O modelo recebe `stripStart` como hoje; o tema não
   recalcula nada. As funções puras ficam em `viewModel.ts`.
3. **Detalhes em Navegar:** com `layout = columns`, o nó de Detalhes é montado sempre que há `data.detail`, com `active = nav.screen ===
   'details'`. A rolagem (`scrollSignal`) só é emitida em Detalhes (o hook já faz isso).
4. **Vídeo `panel`:** o `BackgroundStage` não recebe vídeo (como `cover`); o `DesktopDetails` em modo inativo monta `FocusVideo` dentro
   do visor com `active = nav.screen === 'browse' && !nav.suspended`. Em modo ativo, o `Video` da galeria (como `BigBoxDetails`). Nunca
   os dois ao mesmo tempo (TDK-010).
5. **Ordem:** `sort = theme.sort === 'inherit' ? p.librarySort : theme.sort` em `useTabGames` e no modelo (`sortLabel`). A chave da consulta
   já inclui `sort`.
6. **`onTab`:** `onTab={(i) => dispatchNav({ type: 'tabSet', index: i })}`.
7. **`App.tsx`:** passa `librarySort={query.sort}` e `libraryView={view}`; nada mais muda na entrada e na saída (BBX-001, BBX-002).

Nenhuma leitura nova: `useFocusedData` já lê detalhe, mídia e sessão do jogo em foco em Navegar; o tempo para zerar em Navegar usa o
`fetchAfterMs` que o Mosaico já usa (TDK-011, BBX-059).

## 5. Modelo de exibição [BBX-066, TDK-004..TDK-006, TDK-024]

```ts
type BigBoxTab = { /* atual */ iconUrl: string | null; count: number | null }      // null nos temas sem layout columns
type BigBoxGame = { /* atual */
  coverFit: 'contain' | 'cover'                // do CardImage, como o CoverImage da janela normal
  platform: string                             // lista
  playTimeText: string                         // formatPlayTime (UI-005)
  lastPlayedText: string                       // formatLocalDateTime ("Nunca" sem data)
}
type BigBoxViewModel = { /* atual */ layout: 'screens' | 'columns'; zone: NavZone; view: LibraryView; columns: number }
type BigBoxDetailsProps = { /* atual */
  active?: boolean                             // padrão true; false = painel em Navegar (BBX-064)
  platformIconUrl?: string | null
  coverUrl?: string | null                     // visor sem galeria e em Navegar (TDK-009)
  preview?: { gameId: number; videoUrl: string | null; failed: boolean; onVideoError: (id: number) => void } // TDK-010
  labels?: DesktopLabels                       // rótulos já formatados (status, datas, modos), ver §6
}
```

`buildTabs` recebe o modo (`withIcons`) e preenche `iconUrl` por `platformIconSrc` (de `lib/platformIcons`) e `count` por `gameCount`
(soma em "Todos"). `hintsFor` ganha `{ layout, zone, detailsFocus, empty }` e devolve a tabela do [contrato §8](contracts/bigbox-tema-desktop.md);
com `layout = screens` devolve exatamente o de hoje.

## 6. Componentes do tema e adaptações mínimas [TDK-001..TDK-009, TDK-021..TDK-026, TDK-034, BBX-070]

**Por que não reaproveitar os componentes do desktop:** `LibraryGrid`/`LibraryList` têm virtualizador, foco do DOM, teclado e pedido de
página próprios (concorreriam com o reducer e com BBX-007); `GameDetailsPanel` tem botões de escrita, `SessionsList` com edição (fase 16),
`useGame`/`useMedia` próprios e cores `--arc-*`; `GameCard`, `CoverImage` e `PlatformNav` usam tokens do desktop e `components/`, que a
varredura de BBX-003 proíbe em `bigbox/`. Abrir uma exceção na varredura traria esse risco para todos os temas. A escolha é **redesenhar
a marcação** em `themes/desktop/` com as mesmas classes de tamanho e espaçamento e os tokens `--arc-bb-*` de valor igual (decisão f/g).

Arquivos novos (todos só desenham o modelo e repassam cliques):

| Arquivo | Conteúdo | IDs |
|---|---|---|
| `bigbox/desktopLayout.ts` | `desktopGrid`, constantes do cromo, reexporta `columnsFor`, `usableWidth`, `GRID_GAP`, `GRID_INSET`, `LIST_ROW_HEIGHT` de `lib/gridNav`, `coverPlaceholderUrl` do asset | TDK-007, TDK-034 |
| `themes/desktop/Desktop.tsx` | raiz: cabeçalho (só o título, sem nenhum controle), faixa de erro (sem botão de fechar), três colunas | TDK-001, TDK-002, TDK-026, TDK-038 |
| `themes/desktop/DesktopSidebar.tsx` | "Plataformas", itens com ícone e contagem, item ativo, `scrollIntoView` | TDK-003, TDK-021, TDK-025 |
| `themes/desktop/DesktopToolbar.tsx` | "`T` jogos", "Ordenar por", "Grade"/"Lista" | TDK-004 |
| `themes/desktop/DesktopGrid.tsx` | grade da janela de cartões (sem virtualizador), `role="listbox"` | TDK-005, TDK-019, TDK-023 |
| `themes/desktop/DesktopList.tsx` | linhas de 64 px | TDK-006, TDK-019 |
| `themes/desktop/DesktopDetails.tsx` | painel (ativo e inativo), visor (miniaturas sem papel de botão), só os botões "Jogar" e "Jogar sem contar tempo", campos, descrição, rolagem | TDK-008..TDK-011, TDK-018, TDK-022, TDK-037 |

Os componentes do tema importam só de `bigbox/` (`Panel`, `FocusVideo`, `TimeToBeatList`, `desktopLayout`, `viewModel`) e do asset de
placeholder; nunca de `lib/` (BBX-047). A formatação (datas, status, modos de jogo) fica no shell ou em `viewModel.ts`, que podem usar
`lib/`, e chega pronta (`labels`), como o Estante já faz com `relativeDays`.

Adaptações em `lib/` (sem mudança de comportamento; os testes atuais do desktop são a garantia):

1. `lib/platformIcons.ts` recebe `platformIconSrc`, `seedIconUrls` e `genericIconUrl`; `lib/platforms.ts` os reexporta. O Big Box só
   importa o módulo novo, sem funções de escrita (BBX-066).
2. `lib/gameLabels.ts` recebe `statusLabels` (hoje local a `GameDetailsPanel`) e reexporta `playModeLabels`.
3. `lib/gridNav.ts` ganha `LIST_ROW_HEIGHT = 64`; `LibraryList` passa a usá-lo.
4. `SortSelect` exporta os rótulos (`sortLabel(k)`) por um módulo de `lib/` (ex.: `lib/filters.ts`) para o modelo (BBX-067).

## 7. Tokens [TDK-027, TDK-028, BBX-058]

Bloco novo em `index.css` com os valores do [contrato §7](contracts/bigbox-tema-desktop.md). Os valores são **copiados** (o teste de
contraste lê hexadecimais literais) e o teste de TDK-027 compara cada par com o bloco `[data-theme="dark"]`. A regra do rodapé
(`[data-bigbox-theme="desktop"] [data-bb-footer] { height: 48px }`) exige `data-bb-footer` no `Footer` comum. Nenhum token novo no
contrato (decisão: as diferenças de cor estão listadas nos requisitos).

## 8. Riscos

- **Regressão nos temas atuais** pela mudança do reducer e do shell: mitigado pelos padrões dos campos novos (BBX-061) e pelos testes
  atuais de Faixa, Mosaico e Estante, que não podem mudar.
- **Resize durante a navegação:** `C` muda e a fileira do foco também; `startRow` é recalculado por `edgeScrollRow` e limitado a
  `maxStart`. A página já lida não muda (o `limit` é o do momento do pedido, BBX-038).
- **Nós no DOM sem virtualizador:** o limite de 200 cobre 4K a 100 % (119 nós, contrato §3); acima disso o `R'` cai e a parte de baixo
  fica vazia (Limitações 3).
- **Um único `<video>`:** o `FocusVideo` do painel inativo e o `Video` da galeria nunca coexistem (montagem condicionada a `active`).
- **Foco do DOM:** o Big Box não move o foco do DOM; `scrollIntoView` é chamado só para rolar (com `?.` por causa do jsdom).
- **Temporizadores e efeitos:** frontend só; `useMinuteClock`, `useVideoDelay` e o atraso de 1 s do tempo para zerar já limpam no
  desmonte. Não há goroutine, handle nem processo novo.
- **Troca rápida de plataforma com ↑/↓ segurado** (repetição a 120 ms): cada troca cancela a leitura da aba anterior (BBX-038); as
  páginas já lidas ficam no cache.

## 9. Testes

| Arquivo | Cobre |
|---|---|
| `bigbox/nav.test.ts` | contrato §2 inteiro (vetores), `tabSet`, `axis`, e os casos atuais com `layout = screens` |
| `bigbox/viewModel.test.ts` | `effectiveRows`, `gridWindowAt`, `edgeScrollRow`, `centeredRow`, `buildTabs` com ícones, campos novos |
| `bigbox/useBigBoxData.test.tsx` | `gridPageSize`/`shouldPrefetchGrid` com `R'`; `sort` herdado |
| `bigbox/hints.test.ts` | contrato §8 e o comportamento atual |
| `bigbox/desktopLayout.test.ts` | `desktopGrid` (vetores), reexportações iguais às de `lib/gridNav` (TDK-034) |
| `bigbox/themes/registry.test.ts` | entrada `desktop`, ordem, padrões dos campos novos |
| `bigbox/themes/desktop/*.test.tsx` | estrutura, colunas, cartão, lista, painel ativo/inativo, visor, vídeo, marcas de foco, estados, fora do tema, acessibilidade, mouse; painel com exatamente os dois botões na matriz de fixtures (TDK-037); cabeçalho sem controles (TDK-002) |
| `bigbox/BigBox.test.tsx` | shell com o tema Desktop e controle falso (zonas, pré-busca, rolagem, suspensão, `gameClosed`, espiões de bindings); varredura dos elementos acionáveis do DOM contra a lista permitida em cada zona e estado (TDK-038) |
| `bigbox/bigbox-scan.test.ts` | sanidade incluindo `themes/desktop/` |
| `theme.test.ts` | bloco Desktop (tokens iguais, contraste, área segura por tema) |
| `App.test.tsx`, `App.invariants.test.tsx` | seletor com "Desktop", `librarySort`/`libraryView` repassados, invariantes |

Verificação manual: TDK-035 e TDK-036.

## 10. ADR proposto

**0042 — Layout em colunas no contrato de tema do Big Box** (texto resumido no fim de [requirements.md](requirements.md)). Estende o
0040; não o substitui.
