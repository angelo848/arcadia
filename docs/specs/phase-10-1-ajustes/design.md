# Fase 10.1 — Ajustes da nova interface: design

- **Requisitos:** [requirements.md](requirements.md) (rascunho, 2026-11-14) · **Tarefas:** `tasks.md` (planner)
- **Contrato:** v0.11.0 (fase 9) sem mudança. Não há contrato novo nesta fase (decisão 1 do usuário: sem "Completar
  imagens"; AJU-023 e AJU-024 removidos).
- **ADRs:** 0015, 0028, 0029; proposto 0030 (limites por tipo).
- **Contexto:** o usuário vai limpar a base e reimportar tudo do LaunchBox depois desta fase; a correção das imagens só
  precisa valer para jogos gravados pela importação.

## 0. Estado atual (conferido no código)

| Ponto | Onde | O que acontece hoje |
|---|---|---|
| Contorno cortado | `GameCard.tsx`, `LibraryGrid.tsx`, `LibraryList.tsx` | O cartão marcado usa `ring-2 ring-accent` (sombra de 2 px **fora** da caixa) e o foco usa `outline: 2px` com `outline-offset: 2px` (até 4 px fora). As linhas virtualizadas são `absolute left-0 top-0 w-full` dentro do contêiner com `overflow-y: auto`, que recorta tudo o que passa da caixa: o anel some em cima na 1ª linha (sob a barra do centro) e à esquerda na 1ª coluna (junto da coluna esquerda). O mesmo vale para a direita e para o fim da lista. |
| Largura das colunas | `LibraryGrid.tsx` (`columnsFor(getBoundingClientRect().width)`) | A largura da caixa inclui a barra de rolagem; a última coluna fica parcialmente sob a barra. |
| Cursor | `index.css` + Tailwind 4 | O preflight do Tailwind 4 põe `cursor: default` em `button`; só os cartões e as linhas têm `cursor-pointer`. |
| Carrossel | `MediaCarousel.tsx` | Uma imagem por vez em `h-64`, botões ‹ ›, setas com o foco em qualquer ponto da `section`, volta nas pontas. Ordem: capa e depois a ordem de `media.list` (`ORDER BY kind, id`: background, banner, cover, screenshot, select, title). |
| Capa no cartão | `lib/metadata.ts` (`useCovers`) | Um `media.list` por cartão (cache de 60 s, chave `['media', id]`, a mesma do painel). Procura só `kind === 'cover'`; sem capa → placeholder. **A resposta já tem todas as imagens do jogo**, com `thumbUrl`: a substituta não precisa de IPC nem de agente. |
| Imagens do LaunchBox | `internal/importers/launchbox_images.go` | Lê só 5 pastas: `Banner`, `Fanart - Background`, `Screenshot - Game Title`, `Screenshot - Gameplay`, `Screenshot - Game Select`. **Nenhuma pasta vira `cover`** (`Box - Front` e os pôsteres de loja são ignorados). Nome aceito só `^<título>-\d+\.(jpg\|jpeg\|png)$`. Até 5 por tipo, ordem pelo caminho completo (raiz e regiões misturadas). |
| Cópia | `internal/agent/import_images.go` | `copyLaunchBoxImages` só roda para candidatos **gravados**; um candidato "já existe" (`store.ErrDuplicate`) não recebe nada. A cópia é síncrona dentro de `import.apply` (decodifica e gera miniatura de cada arquivo). |
| Lote | `ImportDialog.tsx`, `cmd/arcadia/imports.go` | Uma única chamada `import.apply` com tudo o que foi marcado; o agente recusa mais de 500 (`maxImportCandidates`) e a chamada tem 60 s (`importTimeout`). |
| Tipos de mídia | migração `00007_media.sql` | `CHECK (kind IN ('cover','screenshot','title','banner','background','select'))`: tipo novo exige migração (fica para a fase 12). |

## 1. Arquivos e responsabilidades

| Arquivo | Mudança | IDs |
|---|---|---|
| `frontend/src/index.css` | tokens de barra; regras globais de barra e cursor | AJU-004, AJU-005, AJU-008 |
| `frontend/src/lib/gridNav.ts` | `GRID_INSET`, `SCROLLBAR_SIZE`, `usableWidth` | AJU-001, AJU-003 |
| `frontend/src/components/LibraryGrid.tsx`, `LibraryList.tsx` | margem interna, `paddingStart/End`, rolagem com a margem | AJU-001..AJU-003 |
| `frontend/src/components/GameDetailsPanel.tsx` | bloco de lançamento | AJU-006, AJU-007 |
| `frontend/src/components/MediaCarousel.tsx` (+ `MediaThumbStrip.tsx`) | destaque e faixa | AJU-009..AJU-014 |
| `frontend/src/lib/metadata.ts` | `orderMedia` com ordem fixa; `cardImageOf`; `useCardImages` (substitui `useCovers`) | AJU-009, AJU-015 |
| `frontend/src/components/CoverImage.tsx`, `GameCard.tsx`, `PanelEmpty.tsx`, `App.tsx` | prop `image: CardImage` com `fit` | AJU-015..AJU-017 |
| `frontend/src/components/ImportDialog.tsx`, `lib/imports.ts` | lotes, progresso | AJU-025 |
| `internal/domain/media_limits.go` (novo) | tabela de limites | AJU-020 |
| `internal/importers/launchbox_images.go` | mapa de pastas, ordem, nome com GUID, pastas não mapeadas | AJU-018, AJU-019, AJU-021, AJU-022 |
| `internal/agent/import_images.go`, `imports.go` | limite contando o que o jogo já tem (via `ListMedia`, que já existe); log | AJU-020, AJU-022 |

Nada em `internal/tracker`, `cmd/fake-game`, `internal/ipc`, nos bindings de `cmd/arcadia`, em `internal/store` (nem
query nova, nem `sqlc generate`) nem em migrações (AJU-028).

## 2. Margem interna da grade e da lista [AJU-001..AJU-003]

```ts
// lib/gridNav.ts
export const GRID_INSET = 6       // ≥ ring (2) + outline (2) + outline-offset (2)
export const SCROLLBAR_SIZE = 8   // AJU-004; reserved by scrollbar-gutter: stable
// usableWidth is the width the cards share inside the scroll box [AJU-003].
export function usableWidth(boxWidth: number): number // max(0, boxWidth - 2 * GRID_INSET - SCROLLBAR_SIZE)
```

- `LibraryGrid`: mede a caixa como hoje e chama `columnsFor(usableWidth(w))`. O virtualizador recebe
  `paddingStart: GRID_INSET` e `paddingEnd: GRID_INSET` (o `row.start` já inclui o início); cada linha absoluta fica em
  `left: GRID_INSET` com `width: calc(100% - 2 * GRID_INSET)`. O contêiner de rolagem tem `scrollbar-gutter: stable`.
- `scrollToIndex` e a centralização inicial somam `GRID_INSET` ao topo da linha (`top = GRID_INSET + row * rowHeight`) e
  usam `GRID_INSET` de folga ao rolar para baixo, para o anel do cartão aparecer inteiro.
- `LibraryList`: o mesmo `paddingStart/End` e `left/width` com a margem; linhas continuam com `ROW_HEIGHT`.
- O teste de AJU-001 confere a constante (`GRID_INSET >= 6`) e o estilo das linhas; o recorte em si é visual (manual).
- UIX-005: `usableWidth(484) = 464 → 2 colunas`; `usableWidth(1132) = 1112 → 6 colunas`.

Risco: o virtualizador soma `paddingStart` em `getTotalSize()`; o pedido da próxima página (UI-036) usa índices, não
pixels, e não muda.

## 3. Barra de rolagem [AJU-004, AJU-005]

Tokens novos no bloco `[data-theme="dark"]` (valores de partida; o teste de contraste decide):

| Token | Valor | Uso |
|---|---|---|
| `--arc-scrollbar-track` | `transparent` | trilho |
| `--arc-scrollbar-thumb` | `#3a404d` | polegar parado |
| `--arc-scrollbar-thumb-hover` | `#6b7587` | polegar com mouse ou arrastando (≥ 3:1 com `--arc-surface` `#1e2128`: ≈ 3,5:1) |

Regras globais (WebView2 é Chromium):

```css
::-webkit-scrollbar { width: 8px; height: 8px; }
::-webkit-scrollbar-track, ::-webkit-scrollbar-corner { background: var(--arc-scrollbar-track); }
::-webkit-scrollbar-thumb { background: var(--arc-scrollbar-thumb); border-radius: 4px; }
::-webkit-scrollbar-thumb:hover, ::-webkit-scrollbar-thumb:active { background: var(--arc-scrollbar-thumb-hover); }
::-webkit-scrollbar-button { display: none; }
```

Não usar `scrollbar-width`/`scrollbar-color` nos mesmos elementos: no Chromium 121+, a presença delas faz o navegador
ignorar as pseudo-classes `::-webkit-scrollbar*`. O teste de AJU-005 também confere que fora do bloco do tema não há cor
literal em `index.css`.

## 4. Botões de lançamento e cursor [AJU-006..AJU-008]

Bloco de lançamento no painel (abaixo do carrossel), em coluna:

| Botão | Classes (Tailwind sobre tokens) |
|---|---|
| Jogar | `w-full min-h-12 rounded px-4 text-lg font-bold bg-play hover:bg-play-hover text-on-accent disabled:opacity-50` + `data-bigbox-play` |
| Jogar sem contar tempo | `w-full min-h-10 rounded px-4 text-base font-medium bg-raised hover:bg-hover border border-play text-fg disabled:opacity-50` |
| Editar · Buscar metadados · Remover | linha abaixo, classes atuais (`secondary`) |

Os testes conferem a ordem no DOM e a presença de `w-full`, `min-h-12`/`min-h-10`, `text-lg`/`text-base`.

Cursor, em `@layer base` de `index.css`:

```css
button:not(:disabled), [role="button"], [role="option"], [role="tab"], a[href], summary, select:not(:disabled),
input[type="checkbox"]:not(:disabled), input[type="radio"]:not(:disabled), label[for] { cursor: pointer; }
button:disabled, select:disabled, input:disabled, [aria-disabled="true"] { cursor: not-allowed; }
```

## 5. Carrossel [AJU-009..AJU-014]

```ts
// lib/metadata.ts
export const carouselKindOrder: readonly MediaKind[] = ['cover', 'screenshot', 'title', 'select', 'background', 'banner']
// orderMedia sorts by carouselKindOrder, keeping the received order inside a kind (stable) [AJU-009].
export function orderMedia(items: MediaItemView[]): MediaItemView[]
```

`MediaCarousel({ items, resetKey })` mantém a assinatura. Estado: `index` (destaque) e `failed: Set<number>` (ids de
mídia cuja imagem grande falhou); a faixa guarda seu próprio `Set` para miniaturas. `resetKey` (id do jogo) ou `items`
novos → `index = 0` e `scrollLeft = 0` da faixa.

`MediaThumbStrip({ items, index, onPick })`: `div role="group" aria-label="Miniaturas"` com rolagem horizontal
(`overflow-x: auto`, margem interna de `GRID_INSET` para o anel, AJU-010) e um `button` por imagem
(`aria-label="<rótulo>, k de n"`, `aria-current` no destaque, `data-bigbox-item`, `tabIndex` 0 só no destaque — uma
parada de Tab). Miniatura 96 × 54 px, `object-fit: cover`.

Teclado (só no `onKeyDown` da faixa; nada no destaque):

| Foco | Tecla | Condição | Efeito | `preventDefault` |
|---|---|---|---|---|
| miniatura k | → | k < n | destaque k+1, foco k+1, `scrollIntoView({ inline: 'nearest' })` | sim |
| miniatura k | → | k = n | nada (decisão a) | **não** (o Big Box age) |
| miniatura k | ← | k > 1 / k = 1 | simétrico | sim / **não** |
| miniatura k | Home / End | — | destaque 1 / n | sim |
| miniatura k | ↑ ↓ | — | nada | não |
| clique em k | — | — | destaque k, `focus()` em k | — |

*Acrescentado após a revisão [AJU-012]:* nas linhas acima, **k é a miniatura que tem o foco** (o índice vem do botão
alvo do evento, por exemplo `data-index` em `event.currentTarget`/`target`), não `index` (destaque). No Big Box o foco
pode estar numa miniatura não destacada (vindo de cima ou de baixo); com destaque 0 e foco em 4, → leva destaque e foco
a 5. As pontas também são as da focada.

O Big Box (`lib/bigbox.ts`) já ignora eventos com `defaultPrevented` (linha do `keydown`); A/Enter numa miniatura é um
clique. Os botões ‹ › saem; o critério de UIX-042 ("uma tecla, uma ação") passa a ser coberto pela tabela acima.

## 6. Imagem do cartão [AJU-015..AJU-017]

```ts
// lib/metadata.ts
// Decisão 2 do usuário: tela de título, tela de seleção, screenshot (gameplay), banner, fundo [AJU-015].
export const cardFallbackOrder: readonly MediaKind[] = ['title', 'select', 'screenshot', 'banner', 'background']
export type CardImage = { url: string; fit: 'contain' | 'cover' }
// cardImageOf picks the first cover (fit contain), else the first image of cardFallbackOrder (fit cover) [AJU-015, AJU-016].
export function cardImageOf(items: MediaItemView[]): CardImage | undefined
// useCardImages maps each game to its card image; it reuses mediaKey(id), so the panel and the grid share the cache.
export function useCardImages(gameIds: number[]): Record<number, CardImage>
```

- `useCovers` é substituído por `useCardImages` (`App.tsx`, `PanelEmpty.tsx`); `url` é `thumbUrl || url`.
- `CoverImage({ src, fit = 'contain' })` aplica `object-contain` ou `object-cover object-center` e expõe `data-fit`;
  erro → placeholder (`data-fit` some).
- `GameCard` troca `coverUrl?: string` por `image?: CardImage`; `LibraryGrid` troca `covers` por `images`.
- **Sem IPC, sem agente:** `media.list` já devolve todas as imagens. Custo igual ao de hoje (uma leitura por cartão).

## 7. Imagens do LaunchBox [AJU-018..AJU-022]

### Achados (pedido do item 4)

| Pasta do LaunchBox | Hoje | Depois |
|---|---|---|
| `Box - Front` (+ regiões) | **ignorada** | `cover`, prioridade 1 |
| `Box - Front - Reconstructed`, `Fanart - Box - Front` | ignoradas | `cover` 2, 3 |
| `Steam Poster`, `Epic Games Poster`, `GOG Poster`, `Origin Poster`, `Uplay Thumbnail`, `Amazon Poster` | ignoradas | `cover` 4..9 |
| `Screenshot - Gameplay` | `screenshot` | `screenshot` 1 |
| `Steam/Epic Games/GOG/Origin/Amazon Screenshot` | ignoradas | `screenshot` 2..6 |
| `Screenshot - High Scores`, `Screenshot - Game Over` | ignoradas | `screenshot` 7, 8 |
| `Screenshot - Game Title` | `title` | `title` |
| `Screenshot - Game Select` | `select` | `select` |
| `Fanart - Background` | `background` | `background` 1 |
| `Epic Games/Origin/Uplay/Amazon Background` | ignoradas | `background` 2..5 |
| `Banner` | `banner` | `banner` 1 |
| `Steam Banner` | ignorada | `banner` 2 |
| `Clear Logo`, `Box - 3D`, `Box - Back*`, `Box - Full`, `Box - Spine`, `Fanart - Box - Back`, `Cart - *`, `Fanart - Cart - *`, `Disc`, `Fanart - Disc`, `Advertisement Flyer - *`, `Arcade - *` | ignoradas | ignoradas (fase 12), listadas no log (AJU-022) |

Outras causas encontradas: arquivos `Título.<GUID>-NN` não casam com a expressão atual (AJU-021; o usuário confirmou que
existem `.jpg` assim; o formato exato é conferido na pasta real antes de implementar); limite fixo de 5 sem contar o que
o jogo já tem. Jogos que já existiam nunca recebem imagens e isso **continua assim** (decisão 1: o usuário reimporta com a
base limpa).
A hipótese mais forte para "jogos do LaunchBox sem capa" é a ausência de `Box - Front` e dos pôsteres de loja (para jogos
de Steam/Epic, o LaunchBox costuma baixar `Steam Poster`/`Epic Games Poster` em vez de `Box - Front`).

### Assinaturas

```go
// internal/domain/media_limits.go
// MediaKindLimit is the most images a game keeps of one kind from an automatic source [AJU-020, ADR 0030].
// Unknown kinds give 0.
func MediaKindLimit(kind string) int // cover 2, screenshot 10, background 2, title 2, select 2, banner 2

// internal/importers/launchbox_images.go
// launchBoxFolders lists folder -> kind in priority order inside each kind [AJU-018].
var launchBoxFolders = []struct{ folder, kind string }{ /* tabela acima */ }

// LaunchBoxImages lists every image of one game in priority order [AJU-018, AJU-019, AJU-021]: folders by
// launchBoxFolders, then root files before region subfolders (alphabetical, case-insensitive), then by name.
// It applies no limit (the agent does, counting what the game has).
func LaunchBoxImages(root, platform, title string) []ImageRef

// UnmappedLaunchBoxFolders returns the subfolders of root\Images\<platform> that are not in launchBoxFolders,
// sorted; a missing folder gives nil [AJU-022].
func UnmappedLaunchBoxFolders(root, platform string) []string
```

Nome (AJU-021): `(?i)^<título escapado>(\.[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})?-\d+\.(jpg|jpeg|png)$`,
compilada uma vez por jogo. A comparação de pasta usa `strings.EqualFold` sobre `os.ReadDir(Images\<plataforma>)`.
**Primeira tarefa da área:** listar alguns nomes com GUID da pasta real (`ARCADIA_LAUNCHBOX_ROOT`) e confirmar a
expressão; divergência volta à spec (AJU-021) antes do código.

### Cópia com limite (agente)

`imageImporter.CopyLaunchBox(ctx, gameID, root, platform, title) (int, error)` mantém a assinatura:

1. `have` (`map[string]int`) contado em memória a partir de `st.ListMedia(ctx, gameID)` (método que já existe); sem
   query nova. Para um jogo recém-gravado pela importação, normalmente é vazio; a contagem garante a regra de AJU-020
   caso a fila de mídia já tenha gravado algo.
2. Para cada `ref` de `LaunchBoxImages` na ordem: se `have[ref.Kind] >= domain.MediaKindLimit(ref.Kind)`, pula **sem
   ler o arquivo**; senão importa (`media.Store.Import`) e grava (`AddMedia`); se `added`, `have[kind]++`.
3. Arquivo que falha: log e segue (como hoje).

`handleImportApply` guarda um `map[string]bool` de plataformas já registradas e chama `UnmappedLaunchBoxFolders` uma vez
por plataforma por chamada (`log.Info("pastas de imagem do LaunchBox não lidas", "platform", p, "folders", list)`).
*Acrescentado após a revisão [AJU-022]:* o helper é chamado para todo candidato do LaunchBox com `launchBoxPath` válido,
gravado ou "já existe"; o mapa é local à chamada, então com os lotes de AJU-025 a mesma plataforma pode ser registrada
uma vez por lote (aceito; nada de estado entre chamadas no agente).

## 8. Importação em lotes [AJU-025]

(A seção "completar existentes" da primeira versão saiu com AJU-023 e AJU-024, decisão 1 do usuário. Não há
`FindImportDuplicate`, `CountMediaByKind`, `completeImages` nem `imagesAdded`; `import.apply`, os tipos de
`internal/ipc/import.go` e o binding `ImportApply(candidates, launchboxPath)` ficam como na v0.11.0.)

```ts
// lib/imports.ts
export const IMPORT_LOT_SIZE = 25
// applyImportInLots sends the candidates in lots, one at a time, and sums the results [AJU-025].
// It stops at the first lot that fails and reports how many were not sent.
export async function applyImportInLots(
  candidates: ImportCandidate[], launchboxPath: string,
  onProgress: (sent: number, total: number) => void,
): Promise<{ result: ImportApplyResult; notSent: number; error?: Error }>
```

`ImportDialog`: os candidatos enviados são os marcados (como hoje), agora por `applyImportInLots`; progresso
"Importando k de N…"; botão de fechar desativado durante os lotes (UIX-039).

Concorrência: lotes sequenciais dentro de um único `async` do diálogo; nenhum timer. O agente processa um lote por
chamada como hoje (uma transação por jogo). Um lote de 25 jogos com até 20 imagens cada (limites somados) fica bem dentro
dos 60 s em disco local; se não ficar, o tamanho do lote é a constante a ajustar.

## 9. Invariantes, testes e riscos [AJU-026..AJU-029]

- **Testes Go:** `internal/importers` (árvore temporária para AJU-018, AJU-019, AJU-021; `UnmappedLaunchBoxFolders`),
  `internal/domain` (tabela), `internal/agent` (limite contando o que o jogo já tem, com SQLite em `t.TempDir()`; log
  capturado; candidato "já existe" não recebe imagens; a cópia não cria sessão, AJU-026). Sem teste novo em
  `internal/store` nem em `internal/ipc` (o teste de contrato continua passando sem alteração, AJU-028). O teste real
  `TestLaunchBoxImagesRealFolder` passa a registrar a contagem por pasta, as pastas não mapeadas e quantos arquivos têm
  GUID no nome (AJU-021).
- **Testes frontend:** `gridNav.test.ts` (AJU-003), `LibraryGrid/LibraryList.test.tsx` (AJU-001, AJU-002),
  `theme.test.ts` (AJU-004, AJU-005, AJU-008), `GameDetailsPanel.test.tsx` (AJU-006, AJU-007),
  `MediaCarousel.test.tsx` (AJU-009..AJU-013), `bigbox.test.tsx` (AJU-014), `metadata.test.ts` e
  `GameCardCover.test.tsx` (AJU-015, AJU-016), `PanelEmpty.test.tsx` (AJU-017), `ImportDialog.test.tsx`
  (AJU-025), `App.invariants.test.tsx` (AJU-026), `theme-scan.test.ts` inalterado (AJU-027). Os testes de UIX-026 que
  citam os botões ‹ › são reescritos para a faixa.
- **Riscos:**
  - *Nomes de pasta de loja:* a tabela segue o padrão do LaunchBox; AJU-022 e o teste real mostram divergências antes
    do fim (limitação 1).
  - *Tempo de `import.apply`:* mais imagens por jogo; mitigado pelos lotes (AJU-025) e por pular arquivos acima do limite
    sem lê-los.
  - *`::-webkit-scrollbar` vs. propriedades padrão:* ver §3; um componente que defina `scrollbar-width` quebra a regra
    global naquele elemento (revisão).
  - *Virtualização com margem:* `paddingStart/End` muda `getTotalSize`; os testes de UI-029/UI-036 precisam continuar
    passando com o novo cálculo.
  - *Substituta recortada:* aceito (limitação 3).
  - *Formato do nome com GUID:* conferido na pasta real antes do código (AJU-021).
  - *Reimportação completa:* com a base limpa, a importação do LaunchBox inteira passa por `import.apply` de uma vez;
    os lotes (AJU-025) são o que a torna possível (limite de 500 por chamada e 60 s por chamada).

## 10. ADR proposto

0030 — Limites de imagem por tipo como tabela única do domínio (ver requirements.md, "ADR proposto").
