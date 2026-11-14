# Fase 10.1 — Ajustes da nova interface: tarefas

- **Status:** rascunho para aprovação do usuário (2026-11-14).
- **Requisitos:** [requirements.md](requirements.md) (aprovado; IDs 001 a 029, com os números 23 e 24 removidos) · **Design:** [design.md](design.md) · **Contrato:** nenhum novo; o IPC continua v0.11.0.
- **ADRs:** 0015, 0028, 0029, 0030.

## Dúvidas da spec (resolvidas na T01 ou aceitas)

1. **Nomes com GUID podem misturar dois jogos de mesmo título (AJU-021).** O candidato não traz o ID do LaunchBox e o IPC não muda. A T01 conta quantos títulos têm as duas formas (`T-NN` e `T.<GUID>-NN`) na pasta real; se for relevante, o spec-writer decide antes da T03 entre aceitar a mistura (limitação conhecida) ou outra solução.
2. A T01 lista as subpastas reais de `Images\<plataforma>` e compara com a tabela de AJU-018; divergência volta ao spec-writer antes da T03.
3. A anotação de UIX-026 já foi ajustada para "substituído por AJU-009..AJU-014" (o `spec-trace` só reconhece `substituído por`).
4. AJU-028 é verificado por revisão do diff e por um caso em `App.invariants.test.tsx` (T16).
5. Mensagem de erro dos lotes (AJU-025): `"<mensagem do erro>. <N> jogos não foram enviados."`; progresso "Importando k de N…".
6. AJU-022 conta, por plataforma, todo candidato do LaunchBox da chamada com `launchBoxPath` válido (gravado ou "já existe").

## Decisões do plano

1. `ListMedia` com erro no início de `CopyLaunchBox`: devolve o erro e não copia nada daquele jogo.
2. Arquivos de cada lugar (raiz ou região) em ordem por nome sem maiúsculas; empate pelo nome exato (AJU-019).
3. A faixa de miniaturas é remontada com `key={resetKey}` dentro de `MediaCarousel`.
4. `scrollbar-gutter: stable` como classe arbitrária `[scrollbar-gutter:stable]` (testável no jsdom).
5. `PanelEmpty` mantém seu `Thumb` e ganha `image?: CardImage` com `data-fit`.
6. `useCovers` só sai na T11.

## Premissas

- Uma tarefa por commit; test-writer primeiro, implementer depois. Ao fim de cada tarefa: `go build ./...`, `go vet ./...`, `go test ./...`, `npm test` e `tsc --noEmit` verdes.
- Nada em `internal/tracker`, `cmd/fake-game`, `internal/ipc`, `internal/store` (queries, migrações, `sqlc`) nem nos bindings de `cmd/arcadia` (AJU-028).
- Todo `.tsx` tocado sai só com tokens (UIX-006/040/041).

## T01 — Conferir nomes com GUID e pastas na instalação real do LaunchBox
- Requisitos: AJU-021, AJU-018. **Responsável:** sessão principal com o usuário (só leitura, sem código).
- Pedir o caminho da pasta do LaunchBox (a que contém `Data` e `Images`; vira `ARCADIA_LAUNCHBOX_ROOT`). Em PowerShell, em `<root>\Images\<plataforma>\**`: (1) listar até 20 nomes com `\.[0-9A-Fa-f]{8}-…-[0-9A-Fa-f]{12}`; (2) anotar separador, maiúsculas do GUID, posição do `-NN`, extensões; (3) contar títulos com as duas formas na mesma plataforma; (4) listar subpastas das plataformas da biblioteca e comparar com a tabela de AJU-018.
- Saída: formato confirmado ou pedido de correção ao spec-writer; 5 a 10 nomes reais viram fixtures da T03. Alternativa: o usuário cola 5 a 10 nomes e a lista de pastas de uma plataforma; a conferência real passa para a T17.
- Dependência: nenhuma. **Bloqueia T03.**

## T02 — Tabela de limites no domínio
- Requisitos: AJU-020. **implementer:** `internal/domain/media_limits.go`.
```go
// MediaKindLimit is the most images a game keeps of one kind from an automatic source [AJU-020, ADR 0030].
func MediaKindLimit(kind string) int // cover 2, screenshot 10, background 2, title 2, select 2, banner 2; unknown 0
```
- **test-writer:** `internal/domain/media_limits_test.go`: `AJU-020/cover 2, screenshot 10, background 2, title 2, select 2, banner 2`; `AJU-020/tipo desconhecido ("logo", "") dá 0`.
- Dependência: nenhuma.

## T03 — Pastas, ordem, nome com GUID e pastas não mapeadas (importador do LaunchBox)
- Requisitos: AJU-018, AJU-019, AJU-021, AJU-022, AJU-020
- **implementer:** `internal/importers/launchbox_images.go`: `launchBoxKinds` dá lugar a `launchBoxFolders` (tabela de AJU-018, ordem cover → screenshot → title → select → background → banner, com prioridade dentro do tipo); sai `maxImagesPerKind` (quem limita é o agente); pasta casada com `strings.EqualFold` sobre `os.ReadDir(Images\<plataforma>)`; em cada pasta, raiz primeiro e regiões em ordem alfabética; regex do nome compilada uma vez por chamada: `(?i)^<título escapado>(\.<GUID>)?-\d+\.(jpg|jpeg|png)$` (formato confirmado na T01).
```go
type ImageRef struct { Kind string; Path string }
func LaunchBoxImages(root, platform, title string) []ImageRef          // prioridade, sem limite [AJU-018, 019, 021]
func UnmappedLaunchBoxFolders(root, platform string) []string          // subpastas fora da tabela, ordenadas; nil se ausente [AJU-022]
```
- **test-writer:** `internal/importers/launchbox_images_test.go` (reescrever `MID-010` com a ordem nova); `launchbox_images_real_test.go` registra contagem por pasta, pastas não mapeadas e arquivos com GUID:
  - `AJU-018/cada pasta da tabela vira o tipo indicado`; `AJU-018/Box - Front e Steam Poster viram cover; Steam Banner vira banner; Epic Games Background vira background`; `AJU-018/pasta não mapeada (Clear Logo, Box - 3D) não é lida`; `AJU-018/nome de pasta com outras maiúsculas é lido`; `AJU-018/pasta ausente ou sem arquivo do jogo devolve nada, sem erro`
  - `AJU-019/Box - Front\Jogo-01.jpg antes de Box - Front\North America\Jogo-01.jpg, antes de Steam Poster\Jogo-01.jpg`; `AJU-019/regiões em ordem alfabética sem maiúsculas; -01 antes de -02`; `AJU-019/ordem entre tipos segue a tabela`
  - `AJU-020/LaunchBoxImages não limita: 12 screenshots dão 12 referências`
  - `AJU-021/aceita T-01.jpg e T.<GUID>-01.jpg com .jpg, .jpeg e .png`; `AJU-021/recusa T 2-01.jpg, T.naoguid-01.jpg, T.<GUID>.jpg e .gif`; `AJU-021/título com ':' e apóstrofo trocados por '_' também com GUID`
  - `AJU-022/UnmappedLaunchBoxFolders lista só as subpastas fora da tabela, em ordem; arquivos soltos não entram`; `AJU-022/pasta da plataforma ausente ou plataforma com separador dá nil`
- Dependência: T01.

## T04 — Cópia com limite que conta o que o jogo já tem
- Requisitos: AJU-020, AJU-018, AJU-026. **implementer:** `internal/agent/import_images.go`: a interface `st` ganha `ListMedia`; `CopyLaunchBox` conta `have[kind]` em memória; para cada ref, se `have[kind] >= domain.MediaKindLimit(kind)` pula sem ler o arquivo, senão `files.Import` + `AddMedia` e, se gravou, `have[kind]++`; falha de arquivo vai ao log e segue; `ListMedia` com erro devolve o erro sem copiar nada. Assinatura de `CopyLaunchBox` não muda.
- **test-writer:** `internal/agent/import_images_test.go` (SQLite em `t.TempDir()`):
  - `AJU-020/12 screenshots distintos gravam 10`; `AJU-020/3 capas iguais pelo conteúdo gravam 1 e não ocupam vagas; uma 4ª distinta grava (total 2)`; `AJU-020/jogo com 2 capas já gravadas não recebe capa nova; screenshots continuam`; `AJU-020/jogo com 3 capas antes continua com 3 (nada é apagado)`; `AJU-020/arquivo acima do limite não é lido`; `AJU-020/ListMedia com erro devolve o erro e não grava nada`
  - `AJU-018/import.apply: candidato "já existe" não recebe imagens`; `AJU-026/import.apply com imagens do LaunchBox não cria sessão`
- Dependências: T02, T03.

## T05 — Registro das pastas de imagem não lidas
- Requisitos: AJU-022. **implementer:** `internal/agent/import_images.go` (helper) e `imports.go` (`handleImportApply` cria `seen := map[string]bool{}` e chama o helper por candidato do LaunchBox com `launchBoxRoot(p.LaunchBoxPath) != ""`; nada interrompe a importação).
```go
// logUnmappedFolders logs once per platform the LaunchBox image folders not read [AJU-022].
// Message "pastas de imagem do LaunchBox não lidas", attrs "platform" and "folders"; nothing when none.
func logUnmappedFolders(log *slog.Logger, seen map[string]bool, root, platform string)
```
- **test-writer:** `import_images_test.go` (handler de captura): `AJU-022/dois candidatos da mesma plataforma com Clear Logo e Box - 3D: uma entrada Info com a plataforma e as duas pastas`; `AJU-022/duas plataformas com pastas extras: uma entrada por plataforma`; `AJU-022/sem pasta não mapeada: nenhuma entrada`; `AJU-022/candidatos de outra fonte ou sem launchBoxPath: nenhuma entrada, e os jogos são gravados`.
- Dependências: T03, T04.

## T06 — Barra de rolagem e cursor no CSS global
- Requisitos: AJU-004, AJU-005, AJU-008. **implementer:** `frontend/src/index.css`: tokens `--arc-scrollbar-track` (`transparent`), `--arc-scrollbar-thumb` (`#3a404d`), `--arc-scrollbar-thumb-hover` (`#6b7587`) em `[data-theme="dark"]`; regras `::-webkit-scrollbar*` do design §3 (8 px, sem botões, polegar com `border-radius: 4px`, hover/active), sem `scrollbar-width`/`scrollbar-color`; regras de cursor do design §4 em `@layer base`.
- **test-writer:** `frontend/src/theme.test.ts` (casos novos): `AJU-004/::-webkit-scrollbar com width e height 8px e ::-webkit-scrollbar-button com display none`; `AJU-004/track e corner usam var(--arc-scrollbar-track); thumb usa var(--arc-scrollbar-thumb) com border-radius; :hover e :active usam var(--arc-scrollbar-thumb-hover)`; `AJU-004/index.css não usa scrollbar-width nem scrollbar-color`; `AJU-005/os três tokens existem no bloco do tema`; `AJU-005/as regras de barra usam só var(--arc-*)`; `AJU-005/fora do bloco do tema não há #hex, rgb( nem hsl(`; `AJU-005/thumb-hover tem contraste ≥ 3:1 com --arc-surface`; `AJU-008/button:not(:disabled), [role="button"], [role="option"], [role="tab"], a[href], summary, select, checkbox, radio e label[for] com cursor: pointer`; `AJU-008/button:disabled, select:disabled, input:disabled e [aria-disabled="true"] com cursor: not-allowed`.
- Dependência: nenhuma.

## T07 — Largura útil da grade
- Requisitos: AJU-003, AJU-001. **implementer:** `frontend/src/lib/gridNav.ts`:
```ts
export const GRID_INSET = 6       // ≥ ring (2) + outline (2) + outline-offset (2) [AJU-001]
export const SCROLLBAR_SIZE = 8   // AJU-004
export function usableWidth(boxWidth: number): number // max(0, boxWidth - 2 * GRID_INSET - SCROLLBAR_SIZE)
```
- **test-writer:** `gridNav.test.ts`: `AJU-003/usableWidth(484) = 464 e usableWidth(1132) = 1112`; `AJU-003/caixa menor que as margens dá 0`; `AJU-003/columnsFor(usableWidth(484)) dá 2 colunas e columnsFor(usableWidth(1132)) dá pelo menos 5`; `AJU-001/GRID_INSET ≥ 6`.
- Dependência: nenhuma.

## T08 — Grade e lista com margem interna e espaço da barra
- Requisitos: AJU-001, AJU-002, AJU-003, AJU-004. **implementer:** `LibraryGrid.tsx`: `columnsFor(usableWidth(w))`; `useVirtualizer` com `paddingStart`/`paddingEnd` = `GRID_INSET`; linhas com `left: GRID_INSET` e `width: calc(100% - 2 * GRID_INSET px)`; rolagem e centralização somam `GRID_INSET`; contêiner com `[scrollbar-gutter:stable]`. `LibraryList.tsx`: o mesmo, com `ROW_HEIGHT`. Props públicas não mudam.
- **test-writer:** `LibraryGrid.test.tsx` e `LibraryList.test.tsx` (ajustar só números de pixel dos testes de UI-029, UI-036 e UIX-016/019): `AJU-001/primeira linha em translateY(GRID_INSET), cada linha com left GRID_INSET e largura descontando 2 × GRID_INSET`; `AJU-001/altura total inclui GRID_INSET em cima e embaixo`; `AJU-001/seta para baixo até linha fora da janela rola com GRID_INSET de folga`; `AJU-001/ao montar com o jogo em foco, a centralização soma GRID_INSET`; `AJU-003/caixa de 484 px dá 2 colunas e de 1132 px dá pelo menos 5`; `AJU-004/grade e lista têm a classe [scrollbar-gutter:stable]`; `AJU-002/lista: primeira linha em translateY(GRID_INSET), linhas com left GRID_INSET; Home e End rolam com a margem`.
- Dependência: T07.

## T09 — Ordem do carrossel e escolha da imagem do cartão
- Requisitos: AJU-009, AJU-015, AJU-016. **implementer:** `frontend/src/lib/metadata.ts` (`useCovers` fica até a T11):
```ts
export const carouselKindOrder: readonly MediaKind[] // ['cover','screenshot','title','select','background','banner']
export function orderMedia(items: MediaItemView[]): MediaItemView[] // estável por carouselKindOrder
export const cardFallbackOrder: readonly MediaKind[] // ['title','select','screenshot','banner','background'] [AJU-015]
export type CardImage = { url: string; fit: 'contain' | 'cover' }
export function cardImageOf(items: MediaItemView[]): CardImage | undefined // 1ª capa (contain), senão 1ª de cardFallbackOrder (cover); url = thumbUrl || url
export function useCardImages(gameIds: number[]): Record<number, CardImage> // mediaKey(id), staleTime 60_000, retry false
```
- **test-writer:** `lib/metadata.test.ts` (reescrever `UIX-026/orderMedia…` como AJU-009): `AJU-009/orderMedia com tipos misturados dá capa, screenshots, título, seleção, fundo, banner, mantendo a ordem dentro do tipo, sem alterar a entrada`; `AJU-015/com capa: cardImageOf devolve a primeira capa com fit contain`; `AJU-015/sem capa: título vence seleção, screenshot, banner e fundo; seleção vence screenshot; screenshot vence banner`; `AJU-015/só banner e fundo: vence o banner (fit cover)`; `AJU-015/lista vazia dá undefined; thumbUrl vazio usa url`. Novo `lib/useCardImages.test.tsx`: `AJU-015/useCardImages lê MediaList uma vez por jogo, compartilha mediaKey com o painel e omite jogos com erro ou sem imagem`.
- Dependência: nenhuma.

## T10 — Cartão com a imagem substituta
- Requisitos: AJU-015, AJU-016. **implementer:** `CoverImage.tsx` (`fit`, `object-contain` ou `object-cover object-center`, `data-fit` só na imagem real); `GameCard.tsx` (`coverUrl` vira `image`); `LibraryGrid.tsx` (`covers` vira `images`); `App.tsx` (`useCovers` vira `useCardImages`).
```ts
export function CoverImage(props: { src?: string; fit?: 'contain' | 'cover' }): JSX.Element // default 'contain'
export function GameCard(props: { game: GameView; image?: CardImage; live: LiveMark; selected: boolean; tabbable: boolean; onSelect: (id: number) => void; onLaunch: (id: number) => void }): JSX.Element
// LibraryGrid props: covers?: Record<number, string> → images?: Record<number, CardImage>
```
- **test-writer:** `CoverImage.test.tsx`, `GameCardCover.test.tsx`, `GameCard.test.tsx`, `LibraryGrid.test.tsx`, `App.test.tsx` (se citar capas): `AJU-016/fit contain: data-fit="contain" e object-contain; fit cover: data-fit="cover" e object-cover object-center`; `AJU-016/substituta que falha mostra o placeholder, sem data-fit (não tenta a próxima)`; `AJU-015/cartão com image de capa mostra a miniatura com contain; com substituta, cover; sem image, placeholder`; `AJU-015/a área continua aspect-[2/3] nos três casos`; `AJU-015/App: jogo sem capa e com screenshot mostra o screenshot no cartão; MediaList com erro mostra o placeholder`.
- Dependências: T08, T09.

## T11 — "Jogados recentemente" com a mesma regra; fim de `useCovers`
- Requisitos: AJU-017. **implementer:** `PanelEmpty.tsx` (`useCardImages`; `Thumb({ image?, title })` com contain/cover, `data-fit` e placeholder em erro); `lib/metadata.ts`: apagar `useCovers`.
- **test-writer:** `PanelEmpty.test.tsx`: `AJU-017/jogo com capa mostra a capa com contain; sem capa e com tela de título mostra a tela de título com cover; sem imagens, placeholder`; `AJU-017/substituta que falha vira placeholder`.
- Dependência: T10.

## T12 — Carrossel com destaque e faixa de miniaturas
- Requisitos: AJU-009..AJU-013. **implementer:** novo `components/MediaThumbStrip.tsx`: `div role="group" aria-label="Miniaturas"`, `overflow-x-auto`, padding `GRID_INSET`; um `button` por imagem (96×54, `object-cover`, `aria-label="<rótulo>, k de n"`, `aria-current` no destaque com `ring-2 ring-accent`, `data-bigbox-item`, `tabIndex` 0 só no destaque); `onKeyDown` pela tabela do design §5 (`preventDefault` só quando trata a tecla; nas pontas não trata); `focus()` e `scrollIntoView({ inline: 'nearest', block: 'nearest' })` na nova miniatura; `Set` próprio de miniaturas com falha. `MediaCarousel.tsx`: saem os botões ‹ › e o `onKeyDown` da `section`; destaque 16:9 `object-contain`, rótulo e "k de n" (`aria-live="polite"`); `index` volta a 0 com `resetKey` ou `items` novos; faixa só com n ≥ 2, montada com `key={resetKey}`; clique no destaque não faz nada.
```ts
export function MediaCarousel(props: { items: MediaItemView[] | undefined; resetKey: number }): JSX.Element
export function MediaThumbStrip(props: { items: MediaItemView[]; index: number; onPick: (k: number) => void }): JSX.Element
```
- **test-writer:** `MediaCarousel.test.tsx` reescrito (saem os casos de ‹ ›); ajustar `App.invariants.test.tsx` (cliques em "Próxima imagem" viram cliques em miniatura): `AJU-009/n = 0, items undefined ou erro: placeholder, sem rótulo, sem indicador e sem faixa`; `AJU-009/n = 1: imagem, rótulo e "1 de 1", sem faixa`; `AJU-009/n = 3 com tipos misturados: a primeira em destaque é a capa e a ordem segue carouselKindOrder`; `AJU-009/clicar na imagem em destaque não muda nada`; `AJU-010/n ≥ 2: group "Miniaturas" com um botão por imagem, nome "<tipo>, k de n", thumbUrl (url se vazia)`; `AJU-010/só a miniatura em destaque tem aria-current="true" e tabIndex 0`; `AJU-010/não há botões "Imagem anterior" nem "Próxima imagem"`; `AJU-010/a faixa tem a margem interna GRID_INSET`; `AJU-011/clicar na miniatura k a põe em destaque, muda rótulo e indicador, dá o foco a k e só chama leituras`; `AJU-011/clicar na já em destaque não muda nada`; `AJU-012/seta direita e esquerda na faixa movem destaque e foco; Home e End vão às pontas`; `AJU-012/seta direita na última e esquerda na primeira não fazem nada e não chamam preventDefault`; `AJU-012/a nova miniatura chama scrollIntoView com inline nearest`; `AJU-012/setas com o foco fora da faixa não mudam a imagem; cima e baixo nunca são tratadas`; `AJU-012/Tab para uma vez na faixa, na miniatura em destaque`; `AJU-013/trocar resetKey ou items volta à primeira imagem e a faixa ao início (scrollLeft 0)`; `AJU-013/erro na imagem em destaque mostra o placeholder; rótulo, indicador e faixa continuam`; `AJU-013/erro numa miniatura mostra o placeholder só nela`; `UIX-043/o carrossel é navegável só pelo teclado`.
- Dependência: T09.

## T13 — Faixa de miniaturas no Big Box
- Requisitos: AJU-014. **test-writer** (implementer só se o teste falhar; a T12 já marca `data-bigbox-item` e o `preventDefault`): `lib/bigbox.test.tsx` com `useBigBoxNav` e o `MediaCarousel`: `AJU-014/cada miniatura é um item data-bigbox-item`; `AJU-014/A (gamepad simulado) e Enter numa miniatura a põem em destaque`; `AJU-014/seta direita numa miniatura do meio troca a imagem uma vez e o Big Box não move o foco (defaultPrevented)`; `AJU-014/seta direita na última miniatura não troca a imagem e o Big Box move o foco ao vizinho`; `AJU-014/cima e baixo numa miniatura são do Big Box`.
- Dependência: T12.

## T14 — "Jogar" em destaque e "Jogar sem contar tempo" visível
- Requisitos: AJU-006, AJU-007. **implementer:** `GameDetailsPanel.tsx`: bloco abaixo do carrossel vira coluna (classes do design §4); "Jogar" mantém `data-bigbox-play` e `data-bigbox-item`; Editar, Buscar metadados e Remover numa linha abaixo; lógica de pendência não muda.
- **test-writer:** `GameDetailsPanel.test.tsx`: `AJU-006/"Jogar" é o primeiro botão depois do carrossel, com w-full, min-h-12, text-lg, font-bold, bg-play, hover:bg-play-hover e text-on-accent`; `AJU-006/com pendência continua "Iniciando…" e desativado`; `AJU-007/"Jogar sem contar tempo" vem logo depois, com w-full, min-h-10, text-base, bg-raised, hover:bg-hover, border border-play e text-fg`; `AJU-007/Editar, Buscar metadados e Remover vêm depois, numa linha, sem w-full`.
- Dependência: nenhuma.

## T15 — Importação gravada em lotes
- Requisitos: AJU-025. **implementer:** `lib/imports.ts` (`IMPORT_LOT_SIZE`, `applyImportInLots`: lotes sequenciais, para no primeiro que falha, soma `applied` e `skipped`, `onProgress(0, N)` antes e `onProgress(sent, N)` depois de cada lote); `ImportDialog.tsx` (`apply()` usa `applyImportInLots`, mostra "Importando k de N…" em `role="status"`, "Fechar" desativado durante a importação, resumo do que foi gravado, `onImported()` se houve algum `applied` mesmo com erro, mensagem de erro da dúvida 5; guarda de "montado" contra `setState` após desmontar).
```ts
export const IMPORT_LOT_SIZE = 25
export async function applyImportInLots(candidates: ImportCandidate[], launchboxPath: string, onProgress: (sent: number, total: number) => void): Promise<{ result: ImportApplyResult; notSent: number; error?: Error }>
```
- **test-writer:** `lib/imports.lots.test.ts` (novo) e `ImportDialog.test.tsx`: `AJU-025/60 candidatos geram 3 chamadas ImportApply de 25, 25 e 10, na ordem, com o mesmo launchboxPath`; `AJU-025/o resultado soma applied e skipped dos lotes`; `AJU-025/falha no 2º lote: o 3º não é enviado, result tem só o 1º e notSent = 35`; `AJU-025/onProgress recebe 0, 25, 50 e 60`; `AJU-025/600 candidatos são aceitos (24 chamadas, nenhuma com mais de 500)`; `AJU-025/diálogo: "Importando k de N…" durante os lotes; Fechar desativado e Esc sem efeito`; `AJU-025/diálogo: falha no 2º lote mostra o resumo do 1º e role=alert com quantos não foram enviados; onImported é chamado`.
- Dependência: nenhuma.

## T16 — Invariantes, varreduras e revisão do diff
- Requisitos: AJU-026, AJU-027, AJU-028. **test-writer:** `App.invariants.test.tsx`: `AJU-026/clicar e navegar nas miniaturas (mouse, setas, Home/End), rolar grade, lista, painel e faixa e passar o mouse nos botões chamam só leituras`; `AJU-026/importar 60 candidatos em lotes chama só ImportScan e ImportApply (3 vezes); nenhum LaunchGame, LaunchGameUntracked ou ToggleSession`; `AJU-028/as chamadas da UI desta fase estão todas na lista de bindings da fase 9`. `theme-scan.test.ts`: `AJU-027/MediaThumbStrip.tsx e os .tsx alterados nesta fase estão no conjunto varrido`. **implementer** só se um teste achar chamada indevida ou cor fora do tema.
- **Revisão (sessão principal):** `git diff --stat` da fase sem `internal/tracker`, `cmd/fake-game`, `internal/ipc`, `internal/store` (queries, `dbgen`, migrações) e `cmd/arcadia`. Em Go, só `internal/domain/media_limits*.go`, `internal/importers/launchbox_images*.go`, `internal/agent/import_images*.go` e `internal/agent/imports.go`. `internal/ipc/contract_test.go` sem alteração e verde.
- Dependência: T01..T15.

## T17 — Conferência manual com a base limpa e a biblioteca reimportada
- Requisitos: AJU-029, AJU-001, AJU-002, AJU-004, AJU-006, AJU-007, AJU-008, AJU-018, AJU-021, AJU-022. **Responsável:** sessão principal com o usuário.
- **Builds e testes:** `go build ./...`, `go vet ./...`, `CGO_ENABLED=0 go build ./...`, `go test ./...`; com `ARCADIA_LAUNCHBOX_ROOT`: `go test ./internal/importers -run TestLaunchBoxImagesRealFolder -v`; `npm test`, `tsc --noEmit`, `npm run build`, `wails build -ldflags "-X main.version=<v>"`. **Rastreio:** `spec-trace.ps1 -Spec docs/specs/phase-10-1-ajustes` e também na fase 10 (UIX-026 como substituído).
- **Preparação:** backup de `data\`; o usuário limpa a base (as sessões do Arcadia são apagadas; o tempo do LaunchBox volta pela sessão importada, IMP-007) e reimporta tudo do LaunchBox.
- **Roteiro (AJU-029)**, em 1366×768 e 1920×1080: (1) importação em lotes termina com "Importando k de N…"; (2) o log mostra "pastas de imagem do LaunchBox não lidas", uma linha por plataforma, sem nenhuma pasta da tabela; (3) jogos antes sem capa mostram capa ou substituta; Steam/Epic com o pôster da loja; (4) cartões nos quatro cantos da grade e na primeira e última linha da lista com contorno inteiro; colunas não mudam quando a barra aparece; (5) barra fina no tema na coluna esquerda, grade, lista, painel, faixa, modal de importação e estatísticas; (6) carrossel por mouse, teclado e Big Box com controle; (7) cursor de mão nos controles e "não permitido" em "Jogar" durante "Iniciando…"; "Jogar" é o botão mais visível.
- **Documentação:** `docs/ARCHITECTURE.md` (tabela de limites, mapa de pastas do LaunchBox, importação em lotes); `docs/ROADMAP.md` (10.1 concluída; tabela de limites antecipada da fase 12); `CLAUDE.md` (spec atual).
- Dependência: T01..T16.

## Ordem

- **Bloco A (paralelo):** T01 (usuário), T02, T06, T07, T09, T14, T15.
- **Bloco B:** Go: T03 (após T01) → T04 (após T02 e T03) → T05. Frontend: T08 (após T07); T10 (após T08 e T09) → T11; T12 (após T09) → T13. As duas cadeias rodam em paralelo.
- **Depois:** T16 → T17.
- **Arquivos compartilhados (não paralelos):** `LibraryGrid.tsx` e seu teste: T08 → T10; `lib/metadata.ts`: T09 → T11; `App.invariants.test.tsx`: T12 → T16; `internal/agent/import_images.go` e seu teste: T04 → T05; `internal/importers/launchbox_images.go`: só T03; `index.css`: só T06; `App.tsx`: só T10; `GameDetailsPanel.tsx`: só T14.

## Riscos de concorrência e recursos

- **Go:** nenhuma goroutine nova; `CopyLaunchBox` continua síncrona dentro de `import.apply` (uma transação por jogo); `os.ReadDir` fecha o diretório sozinho, sem handle do Windows; o `ctx` da chamada já é conferido por candidato; o mapa `seen` de AJU-022 é local à chamada.
- **Tempo de `import.apply`:** cada lote de 25 jogos precisa caber nos 60 s de `importTimeout`; arquivos acima do limite são pulados sem leitura; se a T17 mostrar lotes lentos, ajusta-se só `IMPORT_LOT_SIZE`.
- **UI:** lotes sequenciais num único `async`, sem timer, e o diálogo não fecha durante a importação; guarda de "montado" contra `setState` após desmontar; `ResizeObserver` da grade e da lista seguem desconectados no cleanup; o carrossel e a faixa não têm timer.
- **Teclado em dobro (AJU-014):** a faixa trata a seta no handler React e chama `preventDefault`; o `keydown` do Big Box (no `window`) ignora `defaultPrevented`; na ponta não há `preventDefault` e o Big Box age. A T13 testa.
- **Virtualização:** `paddingStart/End` muda `getTotalSize()`; o prefetch (UI-036) usa índices e não muda.
- **Cache:** `useCardImages`, o painel e `PanelEmpty` dividem `mediaKey(id)`; o compartilhamento estrutural do TanStack Query mantém a referência de `items` quando os dados não mudam, então uma releitura não zera o carrossel (AJU-013).

## Rastreabilidade

| Requisito | Tarefas |
|---|---|
| AJU-001 | T07, T08, T17 |
| AJU-002 | T08, T17 |
| AJU-003 | T07, T08 |
| AJU-004 | T06, T08, T17 |
| AJU-005 | T06 |
| AJU-006, AJU-007 | T14, T17 |
| AJU-008 | T06, T17 |
| AJU-009 | T09, T12 |
| AJU-010..AJU-013 | T12 |
| AJU-014 | T13 |
| AJU-015, AJU-016 | T09, T10 |
| AJU-017 | T11 |
| AJU-018 | T01, T03, T04, T17 |
| AJU-019 | T03 |
| AJU-020 | T02, T03, T04 |
| AJU-021 | T01, T03, T17 |
| AJU-022 | T03, T05, T17 |
| (números 23 e 24) | removidos, sem tarefa |
| AJU-025 | T15 |
| AJU-026 | T04, T16 |
| AJU-027 | T16 |
| AJU-028 | T16 |
| AJU-029 | T17 |

Todos os 27 requisitos ativos têm tarefa. Nenhum ADR novo além do 0030.
