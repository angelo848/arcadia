# Big Box — contrato de tema v2, grade 2D, Mosaico e Estante

Requisitos: BBX-046..BBX-059, MOS-001..MOS-016, ETN-001..ETN-015, TTB-016. Complementa (não substitui)
[bigbox-faixa](../../phase-13-modo-console/contracts/bigbox-faixa.md): onde este documento não fala, vale o de lá.

## 1. Contrato de tema v2 [BBX-047]

```ts
// frontend/src/bigbox/themes/types.ts
export type BigBoxThemeId = 'faixa' | 'mosaico' | 'estante' | 'roda'
export type BigBoxThemeDef = {
  id: BigBoxThemeId
  name: string                                  // "Faixa", "Mosaico", "Estante", "Roda"
  available: boolean
  axis: 'horizontal' | 'vertical' | 'grid'
  columns: number                               // grid: colunas (Estante 5); faixa: 1
  anchor: number                                // faixa: posição do foco na janela (BBX-051); grid: ignorado (0)
  listCapacity: (vp: { width: number; height: number }) => number // faixa: V capas inteiras; grid: C × R
  gridRows?: (vp: { width: number; height: number }) => number    // só grid: R fileiras inteiras (ETN-008)
  backdrop: 'contain' | 'cover' | 'plain'       // BBX-052
  videoSlot: 'background' | 'cover'             // BBX-053
  tabs: 'platforms' | 'collections'             // BBX-054
  sort: 'title' | 'lastPlayed'                  // BBX-055
  timeToBeatInBrowse: boolean                   // BBX-059 (Mosaico true)
  Component?: ComponentType<BigBoxThemeProps>
  Details?: ComponentType<BigBoxDetailsProps>   // BBX-056; ausente → BigBoxDetails
}
export type BigBoxThemeProps = {
  model: BigBoxViewModel
  details: ReactNode
  onPick: (index: number) => void
  onPlay: (index: number) => void
  onDetails: () => void
  onVideoError: (gameId: number) => void        // novo: vídeo na capa (BBX-053)
}
```

| Tema | `available` | `axis` | `columns` | `anchor` | `backdrop` | `videoSlot` | `tabs` | `sort` | `timeToBeatInBrowse` | `Details` |
|---|---|---|---|---|---|---|---|---|---|---|
| `faixa` | sim | horizontal | 1 | 2 | contain | background | platforms | title | não | — |
| `mosaico` | sim | horizontal | 1 | 1 | plain | cover | platforms | title | sim | — |
| `estante` | sim | grid | 5 | 0 | cover | background | collections | lastPlayed | não | `EstanteDetails` |
| `roda` | não | vertical | 1 | 2 | contain | background | platforms | title | não | — |

O tema só desenha o modelo e repassa cliques. Pode importar de `bigbox/`: `Panel`, `FocusVideo`, `CoverArt` (capa ou
substituta, BBX-008), `TimeToBeatList` (os três rótulos e valores). Não importa bindings, `lib/` de escrita nem eventos (varredura
de BBX-003/BBX-059).

## 2. Modelo de exibição — acréscimos [BBX-047..BBX-056, TTB-016]

```ts
type BigBoxTab = {
  key: string                        // 'all' | 'favorites' | 'completed' | `p:${platformId}`
  label: string
  filter: { platformId: number | null; favorite?: true; status?: 'completed' }
}
type BigBoxGame = { /* fase 13 */ playTimeLabel: string }          // "Jogou por 12 h 05 min" | "Nunca jogado" (ETN-005)
type TimeToBeatRow = { label: 'História principal' | 'História + extras' | '100%'; value: string } // TTB-014
type BigBoxFocused = { /* fase 13 */
  favorite: boolean
  lastPlayedRelative: string          // "hoje" | "ontem" | "há N dias" | "há N meses" | "há N anos" | "Nunca jogado" (ETN-002)
  screenshots: string[]               // até 3 URLs de miniatura (MOS-011)
  timeToBeat: TimeToBeatRow[]         // sempre 3 linhas; "—" sem dado (TTB-013)
  timeToBeatSource: string | null     // "HowLongToBeat" ou "IGDB" (reserva) quando há valor; null sem valor (TTB-039)
}
type BigBoxViewModel = { /* fase 13 */
  axis: 'horizontal' | 'vertical' | 'grid'
  columns: number
  tabTotal: number | null             // T da aba ativa depois da 1ª página (BBX-054, MOS-003)
  sortLabel: string                   // "Título" | "Jogado recentemente" (ETN-004)
  videoFailed: boolean                // vídeo do jogo em foco já falhou nesta visita (BBX-053)
}
```

`games` continua sendo a janela já recortada (faixa: §4; grade: §3.2) e `stripStart` o índice do primeiro item dela. O rótulo da aba
com total ("Todos (23)") é montado pelo tema a partir de `tabTotal`. `hints` sai de `hintsFor` com o rótulo de LB/RB do tema
("Plataforma" ou "Aba").

`BigBoxDetailsProps` ganha `timeToBeat: TimeToBeatRow[]` e `timeToBeatSource: string | null`; o resto igual à fase 13.

## 3. Grade 2D [BBX-048..BBX-050]

### 3.1 Reducer

`NavCtx` ganha `columns` (1 nos temas de faixa). Na tela Navegar, com `M = min(total, loaded) − 1` e `C = columns`:

| Estado | Ação | Próximo | Requisito |
|---|---|---|---|
| browse, `C = 1` | `up` / `down` | (mesmo) | BBX-006 (sem mudança) |
| browse, `C > 1` | `prev` / `next` | `focus ∓/± 1` limitado a `[0, M]` | BBX-048 |
| browse, `C > 1` | `down` | `f + C ≤ M` → `f + C`; senão `⌊f/C⌋ < ⌊M/C⌋` → `M`; senão (mesmo) | BBX-048 |
| browse, `C > 1` | `up` | `f − C ≥ 0` → `f − C`; senão (mesmo) | BBX-048 |
| details, qualquer `C` | direções | tabela da fase 13 §3 | BBX-032..BBX-034 |

Vetores (`C = 5`):

| `T` | `L` | `f` | ação | novo `f` | nota |
|---|---|---|---|---|---|
| 23 | 23 | 2 | down | 7 | |
| 23 | 23 | 17 | down | 22 | fileira parcial: vai ao último |
| 23 | 23 | 20 | down | 20 | já na última fileira |
| 23 | 23 | 3 | up | 3 | primeira fileira |
| 23 | 23 | 9 | next | 10 | passa de fileira |
| 23 | 23 | 22 | next | 22 | ponta |
| 40 | 25 | 18 | down | 23 | |
| 40 | 25 | 22 | down | 22 | a fileira de baixo ainda não foi lida (página atrasada): não anda |

`useBigBoxInput`: com `axis = 'grid'`, em Navegar, ←/→ emitem `prev`/`next` e ↑/↓ emitem `up`/`down`; com `horizontal`/`vertical`,
como hoje.

### 3.2 Janela da grade

```
gridWindow(T, f, C, R) → {start, end}   // end exclusivo
R' = clamp(R, 1, 5);  rows = ⌈T / C⌉
startRow = clamp(⌊f / C⌋, 0, max(rows − R', 0))
start = startRow × C
end = min(start + (R' + 1) × C, T)        // no máximo 6 × 5 = 30 cartões (GRID_MAX_NODES)
T = 0 → {0, 0}
```

| T | f | C | R | start | end |
|---|---|---|---|---|---|
| 23 | 0 | 5 | 1 | 0 | 10 |
| 23 | 7 | 5 | 1 | 5 | 15 |
| 23 | 22 | 5 | 1 | 20 | 23 |
| 23 | 22 | 5 | 2 | 15 | 23 |
| 4 | 3 | 5 | 1 | 0 | 4 |
| 0 | 0 | 5 | 1 | 0 | 0 |

O modelo recorta em `L`: `games = lidos[start, min(end, L))`.

### 3.3 Páginas e pré-busca da grade

```
gridPageSize(C, R)  = min((clamp(R, 1, 5) + 1 + PAGE_EXTRA) × C, PAGE_MAX)            // PAGE_EXTRA = 3
shouldPrefetchGrid(L, T, f, C, R, exhausted) =
    !exhausted && L < T && L < min(T, (⌊f/C⌋ + clamp(R, 1, 5) + 1 + PAGE_EXTRA) × C)
```

Com `C = 5`, `R = 1` (`P = 25`) e `T = 60`: entrada lê `[0, 25)`; foco na fileira 1 (`f` 5..9) pede `offset 25` (precisa de 30);
foco na fileira 6 (precisa de 55) pede `offset 50`. Ao pedir, sobram três fileiras lidas além da janela: a janela só passaria de
`L` no quarto ↓ seguinte (≥ 760 ms com o direcional segurado: 400 ms e depois 120 ms por passo). O shell escolhe `shouldPrefetchGrid`/`gridPageSize` ou `shouldPrefetch`/
`pageSize` (fase 13) pelo `axis` do tema; o resto de BBX-007/BBX-038 (uma leitura em voo, sem cancelar ao mover o foco, troca de
aba cancela) é o mesmo código.

## 4. Âncora da faixa [BBX-051]

`stripWindow(T, f, V, A = STRIP_ANCHOR)`: `start = clamp(f − A, 0, max(T − V', 0))`; o resto como a fase 13 §3.1.

| T | f | V | A | start | end |
|---|---|---|---|---|---|
| 40 | 3 | 12 | 2 | 1 | 14 |
| 40 | 3 | 4 | 1 | 2 | 7 |
| 40 | 0 | 4 | 1 | 0 | 5 |
| 40 | 39 | 4 | 1 | 36 | 40 |

## 5. Mosaico [MOS-001..MOS-016]

### 5.1 Layout (Navegar, 16:9)

```
┌ janela (fundo liso --arc-bb-bg) ───────────────────────────────────────────────┐
│ ┌ área segura (5vh / 5vw) ─────────────────────────────────────────────────┐ │
│ │ (Todos) (Windows) (PlayStation) [Nintendo 64] (Super Nintendo)  120 jogos│ │  pílulas 6,6vh, raio total
│ │ The Legend of Zelda: Majora's Mask                (--arc-bb-heading)     │ │  título 3,6vh, 1 linha
│ │ ▢ [▣ vídeo após 2 s] ▢ ▢ ▢|                                               │ │  quadrados 30vh; foco 1,09×
│ │ ┌ painel (--arc-bb-panel, raio 10px) ────────────────────────────────────┐│ │
│ │ │ N64 · 2000 · Ação       │ Tempo jogado   12 h 05 min │ [tela][tela][tela]││ │  colunas 1,3fr / 1fr / 1,2fr
│ │ │ Descrição (4 linhas)    │ Última vez     12/10/2026  │                   ││ │
│ │ │                         │ Sessões        14          │                   ││ │
│ │ │                         │ História principal  20 h   │                   ││ │
│ │ │                         │ História + extras   31½ h  │                   ││ │
│ │ │                         │ 100%                41 h   │                   ││ │
│ │ └─────────────────────────────────────────────────────────────────────────┘│ │
│ │ ───────────────────── [Select+B] Fechar o jogo [LB RB] Plataforma [X] Detalhes [A] Jogar │ │
│ └──────────────────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────────────────┘
```

- Pílulas [MOS-002]: `--arc-bb-surface` / `--arc-bb-text-muted`; ativa `--arc-bb-accent` / `--arc-bb-on-accent`, peso 600.
- Contagem [MOS-003]: à direita, `--arc-bb-text-muted`, "`T` jogos".
- Título [MOS-006]: fonte do título do tema, `--arc-bb-heading`, `text-overflow: ellipsis`.
- Cartões [MOS-004, MOS-005]: quadrados, raio 6px, imagem `contain` sobre `--arc-bb-surface`; sem imagem, `--arc-bb-placeholder` com o
  título; foco: escala 1,09, `outline: 5px solid var(--arc-bb-focus)`, `outline-offset: 4px`; vídeo [MOS-007] dentro do cartão em foco
  (`FocusVideo`).
- Painel [MOS-008..MOS-011]: `Panel` (`data-bb-panel`), `--arc-bb-panel`, três colunas (`1.3fr 1fr 1.2fr`; sem screenshots,
  `1.3fr 1fr`); descrição com `-webkit-line-clamp: 4`; pares rótulo (`text-muted`) / valor (`text`, peso 600) alinhados nas pontas;
  miniaturas `contain` sobre `--arc-bb-surface`, raio 6px.
- Rodapé: `Footer` comum, alinhado à direita, com uma linha `--arc-bb-placeholder` em cima; o botão de cada dica é um selo
  `--arc-bb-chip` com texto `--arc-bb-on-chip`.
- Detalhes [MOS-014]: `BigBoxDetails` comum com estes tokens.

### 5.2 Capacidade [MOS-012]

```
mosaicoCapacity({width, height}):
  width ≤ 0 ou height ≤ 0 → 12 (STRIP_FALLBACK_CAPACITY)
  W = 0,9 × width                      // área segura
  s = 0,30 × height                    // lado do cartão
  g = 16                               // vão
  extra = 0,09 × s + 18                // escala 1,09 do focado + contorno (5 + 4 px dos dois lados)
  V = floor((W − extra + g) / (s + g)), limitado a 1..14
```

| janela | V |
|---|---|
| 1280 × 720 | 4 |
| 1920 × 1080 | 4 |
| 3840 × 2160 | 5 |
| 2560 × 1080 (21:9) | 6 |
| 0 × 0 | 12 |

## 6. Estante [ETN-001..ETN-015]

### 6.1 Layout (Navegar, foco na fileira 0)

```
┌ janela: arte do jogo em foco em cover, degradê --arc-bb-scrim → --arc-bb-bg embaixo ─────────────────┐
│ ┌ área segura ─────────────────────────────────────────────────────────────────────────────────────┐ │
│ │ TÍTULO DO JOGO (texto, até 2 linhas)                              ▒ Tempo jogado ▒ ▒ Sessões ▒   │ │  destaque ≈ 45 % da altura
│ │ Última sessão há 7 dias                                            ▒  28 h 10 min ▒ ▒   14    ▒   │ │
│ │ [▶ Iniciar jogo]  [Detalhes]  ♥                                                                  │ │
│ │ ▒ Sua biblioteca                                 Ordem: Jogado recentemente ▒                    │ │  cabeçalho ≈ 16 %
│ │ ▒ [LB] [Todos (23)] Favoritos  Concluídos  Nintendo 64  SNES ... [RB] ▒                          │ │
│ │ [capa] [capa] [capa] [capa] [capa]                                                               │ │  5 colunas, gap 24px
│ │  Título  Título  Título  Título  Título                                                          │ │
│ │  Jogou por 28 h …                                                                                │ │
│ │ ▒ rodapé ▒                                                                                       │ │
│ └──────────────────────────────────────────────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────────────────────────────────────────────┘
```

- Com o foco na fileira `r ≥ 1` [ETN-007]: o destaque não é desenhado; o cabeçalho fica no topo da área segura; a grade começa na
  fileira do foco (§3.2). Transição de altura/opacidade até 300 ms.
- Botões do destaque [ETN-002]: "Iniciar jogo" com ícone ▶ (SVG `currentColor`), `--arc-bb-accent` / `--arc-bb-on-accent`;
  "Detalhes" `--arc-bb-surface`; coração `currentColor` cheio/vazado, sem `onClick`, `aria-label` "Favorito"/"Não favorito".
- Selos LB/RB [ETN-004]: `--arc-bb-chip` / `--arc-bb-on-chip`; aba ativa `--arc-bb-accent` / `--arc-bb-on-accent`.
- Cartão [ETN-005]: capa 2:3 `contain` sobre `--arc-bb-surface`, raio 8px; título 1 linha; "Jogou por …" `--arc-bb-text-muted`; foco
  `outline 3px --arc-bb-focus`, escala 1,05. Textos do cartão sobre `Panel` (BBX-037).

### 6.2 Capacidade [ETN-008]

```
estanteRows({width, height}):
  width ≤ 0 ou height ≤ 0 → 2
  C = 5;  g = 24
  w = (0,9 × width − (C − 1) × g) / C       // largura do cartão
  h = 1,5 × w + 56                          // capa 2:3 + título + tempo
  A = 0,9 × height − 0,16 × height − 0,07 × height   // área da grade com o destaque fora
  R = floor((A + g) / (h + g)), limitado a 1..5
listCapacity = 5 × estanteRows
```

| janela | R | capacidade |
|---|---|---|
| 1280 × 720 | 1 | 5 |
| 1920 × 1080 | 1 | 5 |
| 3840 × 2160 | 1 | 5 |
| 1920 × 1200 (16:10) | 1 | 5 |
| 0 × 0 | 2 | 10 |

### 6.3 Detalhes (`EstanteDetails`) [ETN-010..ETN-012]

```
┌ área segura (fundo cover do jogo) ─────────────────────────────────────────────────────────────────┐
│ ┌ coluna esquerda (62 %) ────────────────────────────┐  ┌ coluna direita (38 %) ───────────────────┐ │
│ │ TÍTULO                                             │  │ ▒ Estatísticas do jogo                 ▒ │ │
│ │ ▒ Descrição (rola, BBX-036)                      ▒ │  │ ▒ Tempo jogado · Sessões · Última vez  ▒ │ │
│ │ [▶ Iniciar jogo] [Jogar sem contar tempo]          │  │ ▒ Tempo para zerar (HowLongToBeat)     ▒ │ │
│ │ ▒ Jogado por 2 min · Jogado recentemente há 8 dias▒│  │ ▒ História principal      20 h         ▒ │ │
│ │ ┌ visor (contain; vídeo quando selecionado) ─────┐ │  │ ▒ História + extras       31½ h        ▒ │ │
│ │ │                                                │ │  │ ▒ 100%                    41 h         ▒ │ │
│ │ └────────────────────────────────────────────────┘ │  │ ▒ Sobre: Plataforma, Gêneros, Modos,   ▒ │ │
│ │            ●  ○  ○  ○  ○   (uma bolinha por item)   │  │ ▒ Desenvolvedor, Publicado por, Ano    ▒ │ │
│ └────────────────────────────────────────────────────┘  └──────────────────────────────────────────┘ │
│ ▒ rodapé ▒                                                                                          │
└─────────────────────────────────────────────────────────────────────────────────────────────────────┘
```

- Estados de foco: os da fase 13 (`play`, `playUntracked`, `gallery`); marca única (BBX-035): botão focado, ou o **visor** quando o
  foco está no carrossel; bolinhas: selecionada `--arc-bb-text`, outras com borda `--arc-bb-text-muted` e sem preenchimento,
  `aria-current` na selecionada.
- Contenção como BBX-043 (grid de duas colunas, `overflow: hidden`, só a descrição com `overflow-y: auto`).

## 7. Tokens por tema [BBX-058, MOS-015, ETN-014]

Todos os blocos `[data-bigbox-theme="faixa"]`, `["mosaico"]` e `["estante"]` definem **todos** os tokens abaixo (os quatro novos
também no Faixa, que os usa no rodapé e no placeholder). Valores sugeridos; o valor final é o que passar nos pares de contraste.

| Token | Uso | Faixa | Mosaico | Estante |
|---|---|---|---|---|
| `--arc-bb-bg` | fundo | `#0b0d12` | `#edeef0` | `#07080b` |
| `--arc-bb-bg-2` | 2ª cor do gradiente sem arte | `#161a22` | `#e2e4e8` | `#12151c` |
| `--arc-bb-scrim` | preenchimento (contain) / degradê (cover) | `#000000d9` | `#00000000` | `#07080bcc` |
| `--arc-bb-panel` | painel atrás de texto | `#0b0d12e6` | `#ffffff` | `#0b0d12d9` |
| `--arc-bb-text` | texto principal | `#f2f4f8` | `#1b1d21` | `#f2f4f8` |
| `--arc-bb-text-muted` | texto secundário | `#c9ced8` | `#4a4f57` | `#c3c9d4` |
| `--arc-bb-heading` | título do jogo em foco | `#f2f4f8` | `#a3171d` | `#ffffff` |
| `--arc-bb-accent` | botão Jogar, aba ativa | `#1f6feb` | `#d8232a` | `#2f6fed` |
| `--arc-bb-on-accent` | texto sobre `accent` | `#ffffff` | `#ffffff` | `#ffffff` |
| `--arc-bb-surface` | cartões, pílulas, botões secundários | `#1e2128` | `#ffffff` | `#1c2029` |
| `--arc-bb-placeholder` | cartão sem imagem, linha do rodapé | `#2a2e37` | `#c9ccd2` | `#262b35` |
| `--arc-bb-chip` | selo de botão no rodapé, LB/RB | `#2a2e37` | `#2a2d33` | `#2a2e37` |
| `--arc-bb-on-chip` | texto do selo | `#f2f4f8` | `#ffffff` | `#f2f4f8` |
| `--arc-bb-focus` | contorno de foco | `#79b8ff` | `#d8232a` | `#79b8ff` |
| `--arc-bb-danger` | aviso de erro | `#ff8a84` | `#b3261e` | `#ff8a84` |

Pares conferidos (≥ 4,5:1 para texto, ≥ 3:1 para foco):

| Tema | Texto sobre | Pares |
|---|---|---|
| Faixa | `panel` composto sobre `#fff` e `#000` | `text`, `text-muted`, `danger` (como BBX-026); `on-accent`/`accent`; `text`/`surface`; `on-chip`/`chip`; `focus` ≥ 3:1 |
| Mosaico | `panel` e `bg` (opacos) | `text`, `text-muted`, `heading`, `danger`; `on-accent`/`accent`; `text-muted`/`surface`; `on-chip`/`chip`; `focus` contra `bg` e `panel` |
| Estante | `panel` composto sobre `#fff` e `#000`, e `bg` | `text`, `text-muted`, `danger`, `heading`; `on-accent`/`accent`; `text`/`surface`; `on-chip`/`chip`; `focus` contra `panel` composto e `bg` |

Tailwind: `@theme inline` ganha `--color-bb-heading`, `--color-bb-placeholder`, `--color-bb-chip`, `--color-bb-on-chip`.

## 8. Mapa de entrada — mudanças [BBX-057, ETN-006]

| Entrada | Navegar (faixa) | Navegar (grade) | Com Select segurado |
|---|---|---|---|
| ←/→ | jogo anterior/seguinte | jogo anterior/seguinte (passa de fileira) | vale |
| ↑/↓ (direcional) | nada | fileira de cima/baixo | **ignorado** (volume do agente) |
| ↑/↓ (analógico esquerdo) | nada | fileira de cima/baixo | vale |
| A | Jogar | Jogar | **ignorado** (Big Box do agente) |
| B, Y | como a fase 13 | como a fase 13 | ignorados (fase 13) |
| X, LB/RB | como a fase 13 | como a fase 13 | valem |

## 9. Tempos e limites novos

| Constante | Valor | Requisito |
|---|---|---|
| `GRID_MAX_ROWS` | 5 (fileiras inteiras; + 1 cortada) | BBX-049, ETN-008 |
| `GRID_MAX_NODES` | 30 | BBX-049 |
| `ESTANTE_COLUMNS` | 5 | ETN-005 |
| `MOSAICO_FOCUS_SCALE` | 1,09 | MOS-004 |
| `ESTANTE_FOCUS_SCALE` | 1,05 | ETN-005 |
| `HERO_FADE_MS` | 300 | ETN-007 |
| `TTB_FETCH_DELAY_MS` | 1000 | BBX-059 |
