# Fase 12 — Metadados unificados: tarefas

- **Status:** rascunho para aprovação do usuário (2026-11-15).
- **Requisitos:** [requirements.md](requirements.md) (aprovado com as 10 recomendações; MET-001..MET-083) · **Design:** [design.md](design.md) · **Contratos:** [contracts/ipc-v0.14.0-delta.md](contracts/ipc-v0.14.0-delta.md), [contracts/match-rules.md](contracts/match-rules.md), [contracts/batch-state-machine.md](contracts/batch-state-machine.md) e `contracts/ipc.openrpc.json` (v0.14.0, criado na T01).
- **ADRs:** 0015, 0026, 0027, 0030, 0031, 0032, 0033, 0034 (0033 e 0034 já escritos e commitados).

## Dúvidas e achados do planejamento (nenhuma bloqueia; as marcadas "spec-writer" pedem ajuste de texto)

1. **Nome da função de pontuação (spec-writer).** MET-006 e `match-rules.md` §2.2 citam `domain.UnifiedScore`; o design §3 define `domain.ScoreGroup`. O plano usa **`ScoreGroup`** (a assinatura do design). Os testes citam MET-006 pelo ID, então nada quebra; alinhar o texto depois.
2. **Como a saúde do provedor (MET-011) entra no agregador.** O design diz que o job consulta `providerHealth` "antes de cada pergunta", mas `Aggregator.Search` não tem ponto de entrada para isso. O plano acrescenta a `SearchOptions` dois ganchos opcionais: `Skip func(provider string) bool` e `Report func(provider string, err error)`. Não muda comportamento da busca única (nil = sem gancho). Registrar no design §4.5 (spec-writer).
3. **Limitadores dentro da fábrica.** `newProviderFactory(keys, client, clock)` mantém a assinatura e cria os três `Limiter` e o IGDB (com o token) uma vez, dentro do fechamento. Assim `app.go` não muda na fase de provedores, e busca única, lote e aplicação compartilham ritmo e token (achado 4 do design). O lote não precisa "instanciar provedores uma vez" por job: a fábrica já o faz.
4. **Testes existentes que quebram de propósito** (o test-writer os reescreve, com a anotação "alterado em 2026-11-15"): `igdb_test.go` (corpo da consulta), `steamstore_test.go` (uma requisição só), `steamgriddb_test.go` (`grids` mais `heroes`), `scrape_test.go` e `media_changed_test.go` (constroem `scrapeService` e esperam **todas** as imagens enfileiradas; o `scrapeStore` fake ganha `ActiveMediaJobCounts`), `MetadataDialog.test.tsx`, `metadata.test.ts` (`acceptedScore`), `ImportDialog.test.tsx`, `contract_test.go`/`contract_phase11_test.go` (versão e `contractPath`).
5. **`media.queue` aceita `title`** (`scrape.go:219`), mas nenhum provedor oferece `title` e `SelectImages` ignora tipos fora de cover, screenshot e background (MET-025). Sem efeito prático; o teste de `media.queue` (MET-022) cobre `cover` e `screenshot`.
6. **MET-054 e cancelamento.** `batchCancel` (chamada da UI) não gera notificação, nem na transição `applying → done` causada por ela; a UI relê `batchStatus` com a resposta da chamada. Só as transições feitas em segundo plano e o progresso notificam. O plano segue `batch-state-machine.md` §3.
7. **Timeout do desktop.** `scrapeTimeout` (30 s) cobre `metadata.search` (prazo de 20 s) e `metadata.apply` (a espera do limitador numa busca em lote soma no máximo uns 6 s da Steam Store). Sem mudança; só `MetadataBatchResults` usa `libraryTimeout`.
8. **Wails.** `frontend/wailsjs/go/**` é regerado pelo `wails build` (T40); se versionado, entra no commit da T40.
9. **API real (risco 1 do design).** Os parâmetros exatos de IGDB (campo de jogo principal, `artworks`, filtro `platforms`) e de SteamGridDB (`dimensions`, `types`, `heroes`) não foram verificados. A T02 os confere **antes** dos testes dos provedores (T13, T14) e grava as notas em `contracts/provider-api-notes.md`.

## Premissas

- Uma tarefa por commit; test-writer primeiro, implementer depois (como na fase 11). Ao fim de cada tarefa Go: `go build ./...`, `go vet ./...`, `CGO_ENABLED=0 go build ./...`, `go test ./...`; ao fim de cada tarefa de frontend: `npm test` e `npx tsc --noEmit`.
- **Nada** em `internal/tracker`, `cmd/fake-game`, `internal/store/migrations` ou tabelas de sessão (MET-081). Único código novo no store: uma consulta de leitura (T09). Nenhum `windows.Handle`, nenhum código Win32.
- Sem rede e sem `time.Sleep` nos testes (MET-082): provedores fake ou `httptest`, relógio fake (`domain.Clock`). Todo caso de teste começa pelo ID (`t.Run("MET-006/...")`, `it("MET-061 ...")`).
- Constantes do `x-metadata`: as de `domain` já vêm da T03/T04; as do agente ficam num só arquivo, `internal/agent/metadata_consts.go` (T20), e os testes de contrato as comparam com o JSON.

---

## T01 — Contrato completo `ipc.openrpc.json` v0.14.0
- Requisitos: MET-070. **Responsável:** sessão principal (cópia mecânica de JSON grande, como a T01 da fase 11).
- Criar `docs/specs/phase-12-metadados-unificados/contracts/ipc.openrpc.json` como cópia de `docs/specs/phase-11-importacao-v2/contracts/ipc.openrpc.json` (v0.13.0) mais **só** o que o `ipc-v0.14.0-delta.md` descreve:
  - `info.version = "0.14.0"`, `info.description` (§1), `info.x-metadata` (§1);
  - esquemas novos `MetadataRef`, `MetadataCandidate`, `MetadataConfidence`, `MetadataSearchResult`, `MetadataApplyResult`, `BatchState`, `BatchResult`, `BatchStatus`, `BatchRow`, `BatchChoice` (§2); `ImportCandidate.searchName` (opcional, fora de `required`);
  - os 8 métodos `metadata.*` depois de `scrape.apply` (§3, com os `summary`), a notificação `metadata.batchChanged` depois de `media.changed` (§4) e os erros `BatchNotFound` (-32014), `BatchBusy` (-32015), `BatchWrongState` (-32016) (§5);
  - **a descrição de `scrape.search`, `scrape.apply` e `media.queue` ganha a palavra "obsolete"** (`Obsolete: use metadata.search/metadata.apply`), sem mudar parâmetro, resultado ou erro.
- Em `ipc-v0.14.0-delta.md` e `design.md` §2, trocar "(tarefa)" pelo arquivo criado. Dependência: nenhuma. Bloqueia T11.

## T02 — Conferência dos parâmetros reais de IGDB e SteamGridDB
- Requisitos: MET-012, MET-013, MET-003 (risco 1 do design). **Responsável:** sessão principal com o usuário (chaves dele; nada de chave no repositório nem em log). Antes de T13 e T14.
- Conferir na documentação e, com as chaves do usuário, por chamadas de teste manuais (`curl`/PowerShell), e gravar o resultado em `contracts/provider-api-notes.md`:
  1. **IGDB:** (a) o campo/valor que exclui DLC e pacote no `where` (`game_type = (0,8,9,...)` na API atual, ou `category`), e se aceita junto com `search`; (b) a sintaxe `where platforms = (<id>)` com `search`; (c) `artworks.image_id`, `cover.image_id`, `screenshots.image_id` expandidos na mesma consulta; (d) os tamanhos `t_cover_big`, `t_screenshot_big`, `t_1080p`, `t_cover_small`; (e) os ids de plataforma de `igdbPlatformIDs` continuam válidos.
  2. **SteamGridDB:** (a) `GET /grids/game/<id>` com `dimensions=` de retrato (ex.: `600x900,342x482,660x930`) e `types=static`; (b) `GET /heroes/game/<id>` (tipo/forma do resultado); (c) confirmar que **não** existe screenshot de gameplay (decisão 5); (d) formato da resposta (`data[].url`).
  3. **Steam Store:** o host real do `capsule_231x87.jpg` bate com `cdn.cloudflare.steamstatic.com` (MET-068); `screenshots[].path_full` continua `https`.
- Saída: quando um parâmetro não existir, a spec vale pelo comportamento (design §9, risco 1) e as notas dizem o substituto; se alterar uma regra visível, a sessão principal abre ajuste na spec antes de T13/T14. Dependência: nenhuma.

## T03 — Domínio: termo de busca, chave de comparação e marcas
- Requisitos: MET-004, MET-006, MET-007. **implementer:** `internal/domain/match_unified.go` (novo; só esta parte, o resto na T04). Puro: sem relógio, disco ou rede. Reaproveita `NormalizeTitle` e o núcleo do Dice de `match.go`, que vira `diceKeys(a, b string) float64` interna (sem mudar `TitleSimilarity`, `MatchScore`, `MatchThreshold`).
```go
const (
	MatchAuto          = 0.85
	MatchMinSimilarity = 0.80
	MatchMargin        = 0.05
	MatchDoubt         = 0.60
	MatchAlternative   = 0.40
	MatchCap           = 0.79
)
type Confidence string
const ( ConfidenceMatch Confidence = "match"; ConfidenceDoubt Confidence = "doubt"; ConfidenceNone Confidence = "none" )
func SearchTerm(title string) string // "" when nothing is left [MET-004]
func MatchKey(title string) string   // NormalizeTitle minus the/a/an/and, roman ii..xx as numbers [MET-006]
func Marks(key string) []string      // numbers and loose letters except s, t, d, m [MET-007]
```
- **test-writer:** `internal/domain/match_unified_test.go`:
  - `TestSearchTerm`: `MET-004/Legend of Zelda - Majora's Mask (USA)`, `MET-004/Final Fantasy VII™`, `MET-004/Spider-Man mantém hífen`, `MET-004/Game [!] (Disc 1)`, `MET-004/Foo  -  Bar`, `MET-004/A - B - C troca só a primeira`, `MET-004/(Demo) vira vazio`, `MET-004/o título original não é alterado` (a função é pura).
  - `TestMatchKey`: `MET-006/Super Mario World igual`, `MET-006/Zelda - Majora's Mask e Zelda: Majora's Mask têm a mesma chave`, `MET-006/Mario & Luigi e Mario and Luigi`, `MET-006/Final Fantasy VII e Final Fantasy 7`, `MET-006/romanos ii a xx viram número; i, v e x sozinhos não`, `MET-006/the, a, an somem`, `MET-006/acentos e palavras de edição`.
  - `TestMarks`: `MET-007/mega man x → [x]`, `MET-007/final fantasy 7 → [7]`, `MET-007/link s awakening → []`, `MET-007/don t → []`, `MET-007/ordem de aparição preservada`, `MET-007/final fantasy v → [v]`.
- Dependência: nenhuma.

## T04 — Domínio: mesmo jogo, pontuação e confiança
- Requisitos: MET-005, MET-006, MET-007, MET-009. **implementer:** `internal/domain/match_unified.go` (continuação).
```go
type Member struct {
	Provider          string   // "igdb", "steamstore", "steamgriddb"
	Title             string
	Year              int      // 0 = unknown
	PlatformKeys      []string // seed keys the provider lists; empty = no information
	PlatformConfirmed bool     // false for IGDB candidates of the unfiltered second query [MET-003]
}
func SameGame(a, b Member) bool // different providers; (key equal, or Dice >= 0.90 and equal Marks); years unknown or within 1 [MET-005]
type Query struct{ Title string; Year int; LibraryKey string }
type Scored struct{ Score, Sim int; Capped bool } // thousandths
func ScoreGroup(q Query, members []Member) Scored // [MET-006, MET-007]
func ClassifyMatch(first, second *Scored) Confidence // [MET-009]
```
Valores internos em milésimos (`int(math.Round(x*1000))`); `ScoreGroup` usa as chaves (não `TitleSimilarity`).
- **test-writer:** `match_unified_test.go` (continuação):
  - `TestSameGame`: `MET-005/mesma chave, provedores distintos`, `MET-005/Dice 0,90 com marcas iguais junta`, `MET-005/Sonic e Sonic Mania não juntam`, `MET-005/mesmo provedor nunca junta`, `MET-005/diferença de 6 anos não junta; de 1 ano junta; ano 0 junta`, `MET-005/marcas diferentes não juntam`.
  - `TestScoreGroup` (vetores de `match-rules.md` §2.3, uma linha por caso): `MET-006/Super Mario World 1,00`, `MET-006/Zelda Majora 1,00`, `MET-006/Mario & Luigi 1,00`, `MET-006/FF VII e FF 7 1,00`, `MET-006/Link's Awakening ≈ 0,90 (tolerância 0,005)`, `MET-007/Resident Evil × Resident Evil 2 = 0,79`, `MET-007/Final Fantasy VII × VIII = 0,79`, `MET-007/Mega Man X × Mega Man = 0,79`, `MET-007/Final Fantasy V × 5 = 0,79`, `MET-006/Pokemon Red Version ≈ 0,71`, `MET-006/Pokemon Red com plataforma, ano e 2 provedores = 0,83`, `MET-006/Sonic ≈ 0,47`, `MET-006/Doom em dois provedores satura em 1,00`, `MET-006/bônus de provedor extra limitado a 0,04`, `MET-007/plataforma não confirmada limita a 0,79`, `MET-007/membros com plataforma que não inclui a da biblioteca limitam`, `MET-007/membro só do SteamGridDB (sem plataforma) não trava`, `MET-007/biblioteca sem chave de seed nunca trava nem confere`, `MET-006/comparação em milésimos: 0,95 − 0,90 ≥ 0,05`.
  - `TestClassifyMatch`: os 9 vetores de §3 (`MET-009/850 sim 850 sem segundo = match`, `849 = doubt`, `900 com sim 780 = doubt`, `vantagem exata 50 = match`, `vantagem 49 = doubt`, `trava 790 = doubt`, `600 = doubt`, `599 = none`, `lista vazia = none`).
- Dependência: T03.

## T05 — Limitador de requisições por provedor
- Requisitos: MET-010. **implementer:** `internal/scrapers/limiter.go` (novo).
```go
// Limiter spaces the requests of one provider [MET-010]. The next free slot is one time guarded by a mutex: no goroutine, no queue.
type Limiter struct{ /* mu, next time.Time, interval, clock */ }
func NewLimiter(clock domain.Clock, interval time.Duration) *Limiter
func (l *Limiter) Wait(ctx context.Context) error // reserves the slot, waits on clock.After; ctx.Err() when canceled (the reservation stays spent)
const ( IntervalIGDB = 334 * time.Millisecond; IntervalSteamGridDB = 500 * time.Millisecond; IntervalSteamStore = 2 * time.Second )
```
(`Limiter` nulo = sem espera; `(*Limiter)(nil).Wait` devolve `nil`, para os testes de provedor.)
- **test-writer:** `limiter_test.go` (relógio fake do pacote, sem `Sleep`):
  - `MET-010/primeira chamada sai na hora`; `MET-010/10 chamadas simultâneas saem espaçadas pelo intervalo (instantes distintos e crescentes)`; `MET-010/espera conta do horário reservado, não da chegada`; `MET-010/contexto cancelado libera quem espera e devolve ctx.Err()`; `MET-010/a reserva de quem cancelou continua gasta`; `MET-010/provedores independentes não se atrasam`; `MET-010/Limiter nulo não espera`; `MET-010/constantes: 334 ms, 500 ms, 2000 ms`; `MET-010/sem goroutine restante depois de Wait` (contagem de goroutines).
- Dependência: nenhuma.

## T06 — Aplicabilidade e mapa inverso de plataformas
- Requisitos: MET-003. **implementer:** `internal/scrapers/applicability.go` (novo), `internal/scrapers/platforms.go` (acrescenta).
```go
type PlatformInfo struct{ SeedKey, Category string; Known bool }
func Applicable(provider string, p PlatformInfo) bool // table of match-rules.md §1
func SeedKeysForIGDB(ids []int64) []string            // inverse of igdbPlatformIDs; unknown ids ignored
```
- **test-writer:** `applicability_test.go`, `platforms_test.go`:
  - `MET-003/igdb e steamgriddb aplicam a qualquer plataforma`; `MET-003/steamstore: chave windows aplica`; `MET-003/steamstore: sem chave de seed e categoria pc aplica`; `MET-003/steamstore: categoria console não aplica`; `MET-003/steamstore: plataforma não informada (Known=false) aplica`; `MET-003/chave de console do seed não aplica à Steam Store`; `MET-003/provedor desconhecido não aplica`.
  - `MET-003/SeedKeysForIGDB devolve as chaves dos ids conhecidos, ignora desconhecidos e mantém a ordem`; `MET-003/SeedKeysForIGDB é o inverso de PlatformID(chave, "igdb") para todas as chaves do mapa`.
- Dependência: nenhuma.

## T07 — Seleção de imagens por tipo e prioridade
- Requisitos: MET-020, MET-021, MET-025. **implementer:** `internal/scrapers/images.go` (novo). Pura (sem I/O).
```go
type Offer struct{ Provider string; Media []RemoteMedia }
type Pick struct{ Provider, Kind, URL string }
// SelectImages: per kind (cover, screenshot, background) vacancies = max(0, MediaKindLimit(kind) - have[kind] - active[kind]);
// providers visited in the order of match-rules.md §4.2; repeated URL dropped; other kinds ignored.
func SelectImages(offers []Offer, have, active map[string]int, windows bool) (picked []Pick, skipped int)
var providerPriority = /* table by kind and platform class */
```
- **test-writer:** `images_test.go` (tabela de `match-rules.md` §4.2):
  - `MET-020/jogo vazio Windows: 2 capas do SteamGridDB, skipped 2`; `MET-020/jogo vazio console: IGDB primeiro, depois a 1ª do SteamGridDB`; `MET-020/screenshot: IGDB antes da Steam Store nos dois casos`; `MET-020/background: SteamGridDB antes do IGDB`; `MET-020/mesma URL em IGDB e Steam entra uma vez`; `MET-020/ordem de cada provedor preservada`;
  - `MET-021/9 de 10 screenshots: 1 vaga, skipped 4`; `MET-021/2 capas: nenhuma vaga`; `MET-021/8 screenshots e 2 jobs ativos: 0, skipped 5`; `MET-021/active conta e concluído já está em have`; `MET-021/have e active negativos ou ausentes tratam como 0`;
  - `MET-025/tipo title, banner e select oferecidos não entram e não contam em skipped`.
- Dependência: nenhuma.

## T08 — Nome para busca do arquivo avulso (função pura)
- Requisitos: MET-040. **implementer:** `internal/importers/search_name.go` (novo). Listas de nomes genéricos como `var` do pacote.
```go
// SearchName proposes the metadata search name of a file path [MET-040, match-rules.md §5.2].
func SearchName(path string) string
```
Usa `SplitExt`, `NormalizeTitle(stem, false)` e, para a pasta, `NormalizeTitle(dir, true)`; sobe até 3 pastas, nunca a raiz do disco.
- **test-writer:** `search_name_test.go`:
  - `MET-040/Hades.exe → Hades`; `MET-040/hollow_knight.exe → hollow knight`; `MET-040/MyGame-Win64-Shipping.exe sem sufixo de motor`; `MET-040/sufixos empilhados -Win64-Shipping`, `-x64` e `_32bit`; `MET-040/game.exe usa a pasta pai`; `MET-040/Launcher.exe em Binaries\Win64 sobe duas pastas`; `MET-040/Cuphead (v1.3.4).exe`; `MET-040/04. Final Fantasy VIII (USA)\launcher.exe tira prefixo e marcação`; `MET-040/F:\game.exe sem pasta útil devolve game`; `MET-040/Super Metroid (U).smc`; `MET-040/pasta cujo título normalizado fica vazio passa para a seguinte`; `MET-040/nunca sobe além de 3 pastas nem da raiz`; `MET-040/genérico sem diferenciar maiúsculas nem separadores (Start_Game)`; `MET-040/título longo cortado em 200 runas`.
- Dependência: nenhuma.

## T09 — Store: jobs de mídia ativos por tipo
- Requisitos: MET-021, MET-081, MET-025. **implementer:** `internal/store/queries/media.sql`, depois `sqlc generate` (arquivos de `internal/store/dbgen/`), `internal/store/media.go`. **Sem migração.**
```sql
-- name: CountActiveMediaJobsByKind :many
SELECT kind, COUNT(*) AS n FROM media_job WHERE game_id = ? AND state IN ('pending','running') GROUP BY kind;
```
```go
// ActiveMediaJobCounts counts the pending and running download jobs of a game by kind [MET-021].
func (s *Store) ActiveMediaJobCounts(ctx context.Context, gameID int64) (map[string]int, error)
```
- **test-writer:** `internal/store/media_test.go` (store real em `t.TempDir()`): `MET-021/pendente e em andamento contam por tipo`; `MET-021/concluído, falho e cancelado não contam`; `MET-021/outro jogo não conta`; `MET-021/jogo sem job devolve mapa vazio`; `MET-025/nenhum arquivo novo em migrations` (compara a lista de migrações com a esperada: mesma contagem de antes da fase).
- Dependência: nenhuma.

## T10 — `internal/ipc`: tipos, métodos, erros e `searchName`
- Requisitos: MET-070, MET-040, MET-054, MET-071 (nomes dos campos). **implementer:** `internal/ipc/metadata.go` (novo), `internal/ipc/import.go` (campo), `internal/ipc/protocol.go` (códigos, mensagens, comentário do pacote aponta para o contrato da fase 12).
```go
const (
	MethodMetadataSearch       = "metadata.search"
	MethodMetadataApply        = "metadata.apply"
	MethodMetadataBatchStart   = "metadata.batchStart"
	MethodMetadataBatchStatus  = "metadata.batchStatus"
	MethodMetadataBatchResults = "metadata.batchResults"
	MethodMetadataBatchApply   = "metadata.batchApply"
	MethodMetadataBatchCancel  = "metadata.batchCancel"
	MethodMetadataBatchDiscard = "metadata.batchDiscard"
	MethodMetadataBatchChanged = "metadata.batchChanged"
	CodeBatchNotFound = -32014; CodeBatchBusy = -32015; CodeBatchWrongState = -32016
	MsgBatchNotFound = "Metadata batch not found"; MsgBatchBusy = "Metadata batch in progress"; MsgBatchWrongState = "Wrong batch state"
)
type MetadataRef struct{ Provider string `json:"provider"`; ProviderID string `json:"providerId"` }
type MetadataCandidate struct{ Refs []MetadataRef `json:"refs"`; Title string `json:"title"`; Year int `json:"year"`; Platform string `json:"platform"`; Score float64 `json:"score"`; ThumbURL string `json:"thumbUrl,omitempty"` }
type MetadataSearchParams struct{ Title string `json:"title"`; Platform string `json:"platform,omitempty"`; Year int `json:"year,omitempty"` }
type MetadataSearchResult struct{ Candidates []MetadataCandidate `json:"candidates"`; Confidence string `json:"confidence"`; Warnings []string `json:"warnings"` }
type MetadataApplyParams struct{ GameID int64 `json:"gameId"`; Refs []MetadataRef `json:"refs"`; Replace bool `json:"replace,omitempty"` }
type MetadataApplyResult struct{ Detail GameDetail `json:"detail"`; Queued int `json:"queued"`; SkippedByLimit int `json:"skippedByLimit"` }
type BatchStartParams struct{ GameIDs []int64 `json:"gameIds"` }
type BatchStartResult struct{ BatchID int64 `json:"batchId"`; Total int `json:"total"` }
type BatchResult struct{ Applied, ImagesQueued, ImagesSkippedByLimit int; FailedTitles []string } // json: applied, imagesQueued, imagesSkippedByLimit, failedTitles
type BatchStatus struct{ BatchID int64; State string; Total, Done, Failed int; Warnings []string; Result *BatchResult } // result omitempty
type BatchStatusResult struct{ Batch *BatchStatus `json:"batch"` } // nil serializes null
type BatchResultsParams struct{ BatchID int64; Offset, Limit int }
type BatchRow struct{ GameID int64; Title, Platform, Confidence string; Selected int; Candidates []MetadataCandidate }
type BatchResultsResult struct{ Rows []BatchRow `json:"rows"`; Total int `json:"total"`; Next *int `json:"next"` }
type BatchChoice struct{ GameID int64 `json:"gameId"`; Candidate int `json:"candidate"` }
type BatchApplyParams struct{ BatchID int64; Choices []BatchChoice }
type BatchApplyResult struct{ Accepted int `json:"accepted"` }
type BatchIDParams struct{ BatchID int64 `json:"batchId"` }
type BatchCancelResult struct{ State string `json:"state"` }
type BatchDiscardResult struct{ Discarded bool `json:"discarded"` }
type MetadataBatchChangedParams struct{ BatchID int64 `json:"batchId"`; State string `json:"state"`; Done, Total, Failed int } // json: done, total, failed
// ImportCandidate gains: SearchName string `json:"searchName,omitempty"`
```
(Todos com as tags JSON do delta §6; os `BatchStatus`/`BatchRow`/etc. com campos em camelCase como o esquema.) `NewErrorData` já aceita `data.state`; sem helper novo.
- **test-writer:** junto com a T11 (os testes de contrato cobrem os tipos).
- Dependência: nenhuma (T11 depende desta e da T01).

## T11 — Testes de contrato da v0.14.0
- Requisitos: MET-070, MET-040, MET-054. **test-writer:** `internal/ipc/contract_test.go` (`contractPath`/`contractRelPath` para a fase 12; `IPC-041/versão` vira `0.14.0`; `openRPC.Info` ganha `x-metadata`), `contract_phase11_test.go` (ajusta o que aponta para a v0.13.0 e para o `ImportCandidate` agora com `searchName`), novo `contract_phase12_test.go` (`TestMetadataContract`):
  - `MET-070/versão 0.14.0`; `MET-070/os 8 métodos metadata.* existem com paramStructure by-name e os nomes das constantes Go`; `MET-070/metadata.batchChanged é notificação com batchId, state, done, total, failed`; `MET-070/campos de cada tipo Go batem com as propriedades do esquema (assertExactFields): MetadataRef, MetadataCandidate, MetadataSearchResult, MetadataApplyResult, BatchStatus, BatchResult, BatchRow, BatchChoice`; `MET-070/enums: BatchState tem os 5 estados; MetadataConfidence tem match, doubt, none; MetadataRef.provider tem os 3 provedores`; `MET-070/erros -32014, -32015, -32016 com mensagens e data.state`; `MET-070/ProviderNoKey e ProviderUnavailable continuam com data.provider`; `MET-070/x-metadata: valores literais do delta §1 e, para as constantes de domain (MatchAuto, MatchMinSimilarity, MatchMargin, MatchDoubt, MatchAlternative, MatchCap), igualdade com o código`; `MET-070/ponteiro nulo de BatchStatusResult serializa batch:null e Next nulo serializa next:null`;
  - `MET-040/searchName é opcional e omitido: candidato de Steam, Epic, LaunchBox e de pasta serializa byte a byte como na v0.13.0`; `MET-040/searchName aparece quando preenchido`;
  - `MET-054/params de metadata.batchChanged só têm inteiros e o estado (sem dados de jogo)`;
  - **teste "só acréscimo"** (`MET-070/v0.14.0 é a v0.13.0 mais acréscimos`): os dois JSON como `map[string]any`; do v0.14.0 saem `info.version`, `info.description`, `info.x-metadata`, os 10 esquemas novos, `ImportCandidate.properties.searchName`, os 8 métodos novos, a notificação nova e os 3 erros novos; as **descrições** de `scrape.search`, `scrape.apply` e `media.queue` são normalizadas (conferindo que contêm "obsolete" na v0.14.0 e que a v0.13.0 não); depois `reflect.DeepEqual` com a v0.13.0; também confere que nenhum método, parâmetro, resultado ou erro da v0.13.0 mudou ou sumiu, e que `methods` da v0.14.0 mantém a ordem da v0.13.0 com os novos inseridos depois de `scrape.apply`.
- Dependência: T01, T10.

## T12 — Tipos comuns dos provedores
- Requisitos: MET-003, MET-012, MET-014 (tipos). **implementer:** `internal/scrapers/provider.go` (só tipos; o comportamento fica nas T13..T15). `Query` e `Candidate` ganham campos, os existentes não mudam (o `scrape.search` obsoleto continua compilando).
```go
type Query struct {
	Title, Platform string
	Year            int
	SeedKey         string // seed key of the library platform, "" when none
	Category        string // "pc", "console", ... "" when unknown
}
// Candidate gains:
//   PlatformKeys []string; PlatformConfirmed bool; ThumbURL string; Meta *Metadata; Images []RemoteMedia
type Fetcher interface {
	Fetch(ctx context.Context, providerID string) (Metadata, []RemoteMedia, error) // text and images in one request [MET-012, MET-014]
}
```
`PlatformConfirmed` vale `true` por padrão **apenas** onde o provedor o define (T13); campos novos não têm teste próprio (cobertos nas T13..T15).
- Dependência: nenhuma. Arquivo compartilhado: só esta tarefa mexe em `provider.go` nesta fase.

## T13 — IGDB: consulta com filtro, mídia embutida, `Fetch`, limitador
- Requisitos: MET-003, MET-012, MET-010, MET-068. **implementer:** `internal/scrapers/igdb.go`. `IGDBConfig` ganha `Limiter *Limiter`; `Wait` antes de cada requisição de API (não antes do token da Twitch). `Search` monta a consulta de design §4.4 (filtro de jogo principal e `platforms = (<id>)`, conforme as notas da T02); resposta vazia com filtro → repete sem filtro e marca `PlatformConfirmed = false`; `PlatformKeys` via `SeedKeysForIGDB`; `Meta` e `Images` saem da resposta (capa `t_cover_big` → `cover`, screenshots `t_screenshot_big`, artes `t_1080p` → `background`), `ThumbURL` = `t_cover_small`. `Details`/`Media` continuam (`scrape.*` obsoleto). Novo:
```go
func (p *IGDB) Fetch(ctx context.Context, providerID string) (Metadata, []RemoteMedia, error) // one request with the same fields
```
- **test-writer:** `igdb_test.go` (`httptest`, relógio fake, `Limiter` fake):
  - `MET-003/consulta com plataforma de seed traz where platforms = (id) no corpo`; `MET-003/plataforma sem id no IGDB: sem filtro de plataforma`; `MET-003/filtro vazio: segunda consulta sem filtro e candidatos com PlatformConfirmed false`; `MET-003/filtro com resultado: uma só requisição e PlatformConfirmed true`; `MET-003/PlatformKeys vem de platforms.id`;
  - `MET-012/o corpo pede summary, genres, game_modes, involved_companies, cover, screenshots e artworks`; `MET-012/o filtro de jogo principal está no where` (texto conforme T02); `MET-012/candidato traz Meta e Images embutidos (cover, screenshot, background com os tamanhos certos)`; `MET-012/Fetch faz uma requisição por id e devolve o mesmo que a mídia embutida`; `MET-012/uma só requisição por busca, sem requisição de mídia`; `MET-012/aspas no título são removidas do termo`;
  - `MET-068/ThumbURL é https em images.igdb.com com t_cover_small; sem capa fica vazio`;
  - `MET-010/cada requisição de API passa pelo Limiter e a do token não`;
  - `MET-008/falha 429 vira ErrUnavailable`; `MET-001/sem chave: ErrNoKey sem requisição`.
- Dependência: T02, T05, T06, T12.

## T14 — SteamGridDB: capa em retrato e herói
- Requisitos: MET-013, MET-010. **implementer:** `internal/scrapers/steamgriddb.go`. `SGDBConfig` ganha `Limiter`. `Media`: duas requisições, `grids/game/<id>` (parâmetros de retrato e `types=static`, conforme T02 → `cover`) e `heroes/game/<id>` (→ `background`), cada uma depois de `Wait`. `Search` não muda. `Details` vazio.
- **test-writer:** `steamgriddb_test.go`:
  - `MET-013/Media faz duas requisições: grids e heroes`; `MET-013/grids com dimensões de retrato e types=static na URL` (valores das notas da T02); `MET-013/capas viram cover e heróis viram background`; `MET-013/nenhum item screenshot`; `MET-013/sem chave: ErrNoKey e nenhuma requisição`; `MET-013/falha em heroes após grids devolve erro (nada parcial)`; `MET-010/as duas requisições passam pelo Limiter`; `MET-013/Search e Details continuam como antes`.
- Dependência: T02, T05, T12.

## T15 — Steam Store: uma requisição, plataforma e miniatura
- Requisitos: MET-014, MET-010, MET-003, MET-068. **implementer:** `internal/scrapers/steamstore.go`. `SteamConfig` ganha `Limiter`. `Fetch` faz **um** `appdetails` (texto e screenshots, só `https`); `Details` e `Media` o chamam e descartam a outra metade. `Search` preenche `PlatformKeys = ["windows"]`, `PlatformConfirmed = true` e `ThumbURL` = `https://cdn.cloudflare.steamstatic.com/steam/apps/<appid>/capsule_231x87.jpg`.
- **test-writer:** `steamstore_test.go` (atualiza os casos antigos que contavam duas requisições):
  - `MET-014/Fetch faz uma só requisição appdetails e devolve texto e screenshots`; `MET-014/Details e Media continuam funcionando (uma requisição cada)`; `MET-014/screenshots http são descartados`; `MET-014/ThumbURL pelo appid sem requisição`; `MET-003/PlatformKeys é windows`; `MET-010/cada requisição passa pelo Limiter`; `MET-014/app sem dados devolve vazio sem erro`; `MET-068/ThumbURL no host cdn.cloudflare.steamstatic.com`.
- Dependência: T05, T12.

## T16 — Agrupamento e ordenação (função pura do agregador)
- Requisitos: MET-005, MET-006, MET-008. **implementer:** `internal/scrapers/aggregate.go` (novo; só os tipos e `buildGroups`, `Aggregator.Search` na T17).
```go
type Ref struct{ Provider, ProviderID string }
type Group struct {
	Members []domain.Member
	Refs    []Ref                 // one per provider, igdb, steamstore, steamgriddb order
	Score   domain.Scored
	Title   string; Year int; Platform string; ThumbURL string // of the first member in provider order; year = first non-zero
	Meta    map[string]Metadata    // per provider, what the search already returned
	Images  map[string][]RemoteMedia
}
// buildGroups joins candidates by domain.SameGame in provider order, scores and sorts (score desc, more providers, title),
// keeps up to max groups with score >= MatchAlternative [MET-005, MET-006, MET-008].
func buildGroups(q domain.Query, byProvider map[string][]Candidate, libraryName string, max int) []Group
```
- **test-writer:** `aggregate_group_test.go`:
  - `MET-005/Hades em IGDB e SteamGridDB: um grupo com 2 provedores`; `MET-005/Resident Evil 1996 e 2002 do mesmo provedor: 2 grupos`; `MET-005/SteamGridDB sem ano entra no grupo mais antigo (empate)`; `MET-005/Resident Evil 2002 e SteamGridDB 1996: 2 grupos`; `MET-005/Sonic e Sonic Mania: 2 grupos`; `MET-005/título, ano e plataforma exibidos são os do primeiro membro na ordem igdb, steamstore, steamgriddb`; `MET-005/ano é o primeiro não zero`; `MET-005/três provedores no mesmo grupo`;
  - `MET-006/pontuação do grupo vem de ScoreGroup com a chave da biblioteca`; `MET-008/ordem: pontuação decrescente, empate por mais provedores e depois título`; `MET-008/corta em 10 grupos`; `MET-008/descarta pontuação abaixo de 0,40`; `MET-008/nome da plataforma da biblioteca quando confere; senão o nome canônico da primeira chave; senão vazio`; `MET-008/lista vazia é válida`.
- Dependência: T04, T12.

## T17 — Agregador: busca paralela, escalonamento e falhas parciais
- Requisitos: MET-001, MET-002, MET-015, MET-008, MET-009, MET-004. **implementer:** `internal/scrapers/aggregate.go` (continuação).
```go
type Aggregator struct {
	Providers func(name string) (MetadataProvider, error) // ErrNoKey = skip
	Clock     domain.Clock
}
type SearchOptions struct {
	Staged  bool          // batch: Steam Store only when the others did not find a "match" [MET-015]
	Timeout time.Duration // 20 s single, 15 s per game of the batch
	Skip    func(provider string) bool   // batch: provider suspended or disabled [MET-011]; nil = never
	Report  func(provider string, err error) // batch: result of each asked provider (nil err = ok); nil = ignored
}
type Outcome struct {
	Groups     []Group
	Confidence domain.Confidence
	Warnings   []string // "IGDB indisponível no momento" for each temporary failure
	Missing    []string // skipped for lack of key
	Failed     []string // failed temporarily, in order
	Asked      []string // actually asked
}
var ErrAllUnavailable = ErrUnavailable // returned (wrapped, with the provider) when every asked provider failed
func (a *Aggregator) Search(ctx context.Context, q Query, opt SearchOptions) (Outcome, error)
```
Uma goroutine por provedor dentro da chamada (`sync.WaitGroup`), prazo por `Clock.After` com `context.WithCancel`; nenhuma goroutine sobrevive à chamada. O termo enviado é `domain.SearchTerm(q.Title)` já aplicado pelo chamador; o agregador envia `Query.Title` sem alterar. Sem provedor aplicável com chave: `ErrNoKey` com o provedor que falta (`igdb`, depois `steamgriddb`); todos os perguntados falharam: `ErrUnavailable` com o primeiro. Pergunta aos provedores aplicáveis por `Applicable` e usa `Candidate.Meta/Images` para o grupo.
- **test-writer:** `aggregate_test.go` (provedores fake, relógio fake):
  - `MET-001/todos aplicáveis com chave são consultados em paralelo (um barrier prova a simultaneidade)`; `MET-001/provedor sem chave não é consultado e não conta como falha`; `MET-001/provedor não aplicável (Steam Store em console) não é consultado`; `MET-001/falha temporária de um: lista dos outros e warnings "IGDB indisponível no momento"`; `MET-001/prazo de 20 s: provedor lento conta como falha temporária (relógio fake avança)`; `MET-001/Timeout da opção vale 15 s no lote`; `MET-001/nenhuma goroutine fica depois de Search`;
  - `MET-002/nenhum provedor aplicável com chave: ErrNoKey com o provedor igdb e zero chamadas`; `MET-002/só a chave do SteamGridDB falta: ErrNoKey não ocorre se o IGDB tem chave`; `MET-002/todos falham: ErrUnavailable com o primeiro que falhou`; `MET-002/um respondeu com lista vazia: sucesso`;
  - `MET-015/Staged: IGDB acha match, Steam Store não é consultada`; `MET-015/Staged: IGDB acha doubt, Steam Store é consultada uma vez`; `MET-015/Staged sem chave do IGDB: Steam Store consultada`; `MET-015/busca única sempre consulta a Steam Store quando aplicável`; `MET-015/Staged depois: a Steam Store entra no agrupamento`;
  - `MET-004/o fake recebe o termo exatamente como veio em Query.Title`;
  - `MET-009/confiança calculada pela lista ordenada (match, doubt, none)`; `MET-008/lista limitada a 10`;
  - `MET-011/Skip evita a pergunta ao provedor suspenso`; `MET-011/Report recebe nil para sucesso e o erro para falha`.
- Dependência: T04, T06, T12, T16.

## T18 — Fábrica de provedores com limitadores e IGDB compartilhado
- Requisitos: MET-010, MET-012. **implementer:** `internal/agent/scrape_providers.go`. A assinatura de `newProviderFactory(keys, client, clock)` **não muda**; dentro do fechamento: três `Limiter` (`IntervalIGDB`, `IntervalSteamGridDB`, `IntervalSteamStore`) criados uma vez; uma instância de IGDB reaproveitada enquanto o par (Client ID, Secret) for o mesmo (o token Twitch é reusado), recriada quando a chave muda; SteamGridDB e Steam Store novos a cada chamada (baratos, sem estado) mas com o limitador compartilhado. `app.go` não muda aqui.
```go
// providerLimiters are the three limiters of the agent, one per provider [MET-010].
type providerLimiters struct{ igdb, sgdb, steam *scrapers.Limiter }
```
- **test-writer:** `scrape_providers_test.go`:
  - `MET-010/duas chamadas à fábrica compartilham o limitador do IGDB (as requisições saem espaçadas, relógio fake)`; `MET-010/SteamGridDB e Steam Store compartilham o seu`; `MET-010/os três limitadores são independentes`; `MET-012/duas chamadas à fábrica pedem um só token (httptest conta o /token)`; `MET-012/a chave mudou: novo token`; `MET-012/sem chave: ErrNoKey sem requisição`.
- Dependência: T05, T13, T14, T15.

## T19 — `queueImages` com limite, trava por jogo e contadores
- Requisitos: MET-021, MET-022, MET-023, MET-024. **implementer:** `internal/agent/scrape.go` (`scrapeStore` ganha `ActiveMediaJobCounts`; `scrapeService` ganha `locks gameLocks`; `apply` e `mediaQueue` passam por `queueImages`). `gameLocks` é um mapa `map[int64]*refMutex` com contagem de referências, protegido por um mutex (sem goroutine).
```go
type queueResult struct{ queued, skipped int }
// queueImages counts have (ListMedia) and active (ActiveMediaJobCounts), runs scrapers.SelectImages and EnqueueMedia, all under the game's lock [MET-021, MET-023].
func (s *scrapeService) queueImages(ctx context.Context, gameID int64, offers []scrapers.Offer, kinds map[string]bool, at time.Time) (queueResult, error)
type gameLocks struct{ /* mu sync.Mutex; m map[int64]*refMutex */ }
func (g *gameLocks) lock(gameID int64) (unlock func())
```
`windows` de `SelectImages` = a plataforma do jogo tem chave `windows` (`GameDetail`/`ResolvePlatform`); sem plataforma = `false`. `scrape.apply` e `media.queue` montam um `Offer` com o provedor; `ScrapeApplyResult.Queued` continua o enfileirado; `MediaQueueResult.Queued` idem.
- **test-writer:** `scrape_limits_test.go` (store real) e ajuste de `scrape_test.go`/`media_changed_test.go` (o fake de `scrapeStore` ganha o método):
  - `MET-021/jogo vazio recebe até o limite por tipo`; `MET-021/jogo com 9 de 10 screenshots recebe 1`; `MET-021/jogo com 2 capas não recebe capa`; `MET-021/jobs pendentes e em andamento contam, falhos e cancelados não`; `MET-021/imagens de qualquer origem (LaunchBox) contam`; `MET-021/nada é apagado`;
  - `MET-022/metadata.apply-via-queueImages, scrape.apply, media.queue e a aplicação do lote usam a mesma função: mesmo jogo com 1 vaga de screenshot enfileira 1 em cada caminho` (um caso por caminho; os de metadata e lote entram nas T21 e T27 reaproveitando este helper); `MET-022/media.queue com tipos explícitos limita cada tipo pedido`; `MET-022/scrape.apply passa pelo limite`;
  - `MET-023/dois pedidos concorrentes do mesmo jogo não passam do limite (-race)`; `MET-023/jogos diferentes não se bloqueiam (um lock preso não atrasa o outro)`; `MET-023/o lock é liberado em erro do store`; `MET-023/mapa de locks não vaza entradas depois de liberar`;
  - `MET-024/queued e skippedByLimit contados`; `MET-024/queue.Notify só é chamado quando enfileirou`.
- Dependência: T07, T09.

## T20 — `metadata.search` e constantes do agente
- Requisitos: MET-001, MET-002, MET-003, MET-004, MET-008, MET-009, MET-068, MET-071. **implementer:** `internal/agent/metadata.go` (novo), `internal/agent/metadata_consts.go` (novo).
```go
const (
	searchTimeout = 20 * time.Second; gameSearchTimeout = 15 * time.Second
	maxSearchCandidates = 10; maxRowCandidates = 5
	batchWorkers = 3; maxBatchGames = 1000; maxBatchChoices = 1000
	defaultResultsLimit = 100; maxResultsLimit = 200; maxResultBytes = 900 << 10
	progressWindow = 500 * time.Millisecond; providerMaxFailures = 3
	maxStatusWarnings = 10; maxFailedTitles = 10
)
var providerCooldown = []time.Duration{30 * time.Second, 60 * time.Second, 120 * time.Second}

type metadataStore interface { // what the unified service needs from the store
	scrapeStore
	ResolvePlatform(ctx context.Context, name string) (store.Platform, bool, error)
}
type metadataService struct { /* scrape *scrapeService; agg *scrapers.Aggregator; st metadataStore; clock; log; root context.Context; mu; batch *batchJob; wg sync.WaitGroup */ }
func newMetadataService(s *scrapeService, st metadataStore, root context.Context) *metadataService
func metadataHandlers(m *metadataService) map[string]ipc.Handler
func (m *metadataService) search(ctx context.Context, params json.RawMessage) (any, error)
func (m *metadataService) Wait() // waits for every goroutine of every job [MET-059]
func allowedThumb(raw string) string // https and images.igdb.com | cdn.cloudflare.steamstatic.com | cdn2.steamgriddb.com, else "" [MET-068]
func validateRefs(refs []ipc.MetadataRef) error // 1..3, unique provider, providerId ^[0-9]{1,20}$ [MET-031, MET-071]
```
`search`: valida (MET-071) antes de qualquer leitura; `domain.SearchTerm` (vazio = `-32602`); `ResolvePlatform` sem criar; `scrapers.Query` com `SeedKey` e `Category`; `Aggregator.Search` com 20 s; converte para `ipc.MetadataSearchResult`; `ErrNoKey` → `-32010`, `ErrUnavailable` → `-32011` com `data.provider` (reaproveita `providerError`).
- **test-writer:** `metadata_test.go` (store real, provedores fake via `scrapeService.providers`):
  - `MET-071/título vazio, só espaços, com 201 runas`, `plataforma com 101 runas`, `ano 1899, 2101 (0 e ausente valem)`: `-32602` sem nenhuma chamada ao fake; `MET-004/título só de marcações (Demo): -32602`;
  - `MET-001/todos respondem: chamadas a cada fake conferidas e uma lista única`; `MET-001/um sem chave não é chamado`; `MET-001/um com falha temporária: lista dos outros e warnings`; `MET-001/provedor lento com relógio fake: falha temporária depois de 20 s`;
  - `MET-002/nenhum aplicável com chave: -32010 com data.provider igdb, zero requisições`; `MET-002/só IGDB sem chave e SteamGridDB com chave: sucesso`; `MET-002/todos falham: -32011 com o primeiro`; `MET-002/um respondeu vazio: sucesso com lista vazia`;
  - `MET-003/Windows do seed renomeada: Steam Store aplicável; plataforma de console do seed: não`; `MET-003/plataforma criada pelo usuário com categoria pc: Steam Store; categoria console: não`; `MET-003/plataforma inexistente ou não informada: Steam Store aplicável`; `MET-003/a busca não cria plataforma`;
  - `MET-004/o fake recebe o termo de SearchTerm e o título original não muda`;
  - `MET-008/até 10 itens, ordem e piso 0,40`; `MET-009/confidence match, doubt e none para listas fabricadas`; `MET-008/ano 0 e plataforma vazia quando desconhecidos`;
  - `MET-068/thumbUrl só nos três hosts https; http e outro host viram vazio`; `MET-068/URL malformada vira vazio`.
- Dependência: T10, T17, T18, T19 (para o `scrapeService`; usa só `game()` e `providerError`, então pode começar assim que T17 compilar).

## T21 — `metadata.apply`
- Requisitos: MET-030, MET-031, MET-032, MET-033, MET-012, MET-013, MET-014, MET-022, MET-024, MET-071. **implementer:** `internal/agent/metadata.go` (continuação).
```go
func (m *metadataService) apply(ctx context.Context, params json.RawMessage) (any, error)
// fetchRefs obtains text and images of every ref before any write; Fetcher when available, Details + Media otherwise [MET-032].
func (m *metadataService) fetchRefs(ctx context.Context, refs []ipc.MetadataRef) (map[string]scrapers.Metadata, []scrapers.Offer, error)
```
Validação (MET-031, MET-071) antes de qualquer leitura de jogo ou chamada; `GameDetail` (`-32001`); todas as obtenções antes de `ApplyMetadata`; `scrapers.Merge` (IGDB, depois Steam Store); `queueImages` (T19); o resultado leva `Detail`, `Queued`, `SkippedByLimit`; nenhuma notificação (a fila avisa por `media.changed`). O título do jogo nunca muda.
- **test-writer:** `metadata_apply_test.go` (store real, fila fake):
  - `MET-031/refs vazio, com 4 itens, provedor repetido, provedor desconhecido, providerId vazio, com letras, com 21 dígitos: -32602 sem chamada`; `MET-031/provedor com chave ausente: -32010`; `MET-031/falha temporária: -32011`;
  - `MET-030/campos vazios preenchidos por IGDB e depois Steam Store`; `MET-030/campo preenchido pelo usuário é preservado`; `MET-030/replace sobrescreve`; `MET-030/título do jogo nunca muda`; `MET-030/IGDB 1 requisição, Steam Store 1, SteamGridDB 2 (contagem nos fakes)`; `MET-030/imagens enfileiradas pela prioridade de MET-020`;
  - `MET-012/IGDB sem Fetcher cache: uma requisição por id`; `MET-013/ref do SteamGridDB devolve só cover e background`; `MET-014/ref da Steam Store: uma requisição`;
  - `MET-032/segundo provedor falha: jogo e fila intactos`; `MET-032/gameId inexistente: -32001 sem chamada`;
  - `MET-033/nenhuma notificação antes de um job gravar` (servidor IPC espião: nenhuma `notify`);
  - `MET-022/jogo com 1 vaga de screenshot enfileira 1 e skippedByLimit conta o resto`; `MET-024/queued e skippedByLimit no resultado`; `MET-071/gameId < 1 é -32602`.
- Dependência: T19, T20.

## T22 — `searchName` na fonte "Arquivo" e no IPC
- Requisitos: MET-040. **implementer:** `internal/importers/types.go` (`Candidate` ganha `SearchName string`), `internal/importers/file.go` (preenche com `SearchName(path)`; outras fontes não), `internal/agent/imports.go` (`toIPCCandidate` copia `SearchName`; `import.apply` aceita e ignora o campo).
- **test-writer:** `file_test.go` e `internal/agent/imports_test.go`:
  - `MET-040/Arquivo traz searchName normalizado do nome (Hades.exe, MyGame-Win64-Shipping.exe)`; `MET-040/nome genérico usa a pasta pai`; `MET-040/título (Title) não muda: só searchName é novo`; `MET-040/Pasta, Steam, Epic e LaunchBox não trazem searchName (campo vazio e omitido no JSON)`; `MET-040/import.apply aceita searchName e o ignora`; `MET-040/o scan de arquivo devolve searchName no resultado do IPC`; `MET-040/launchTarget e installDir intactos`.
- Dependência: T08, T10.

## T23 — Notificador do lote e agrupador de contadores
- Requisitos: MET-054. **implementer:** `internal/agent/metadata_notify.go` (novo), `internal/agent/notifier.go` (irmão do `mediaNotifier`).
```go
type batchSink interface{ BatchChanged(p ipc.MetadataBatchChangedParams) }
type batchNotifier struct{ srv atomic.Pointer[ipc.Server] }
func (n *batchNotifier) set(s *ipc.Server)
func (n *batchNotifier) BatchChanged(p ipc.MetadataBatchChangedParams) // no-op when srv is nil; never blocks
// progressCoalescer: first counter change goes out now and opens a window; the next fold into one at the end; a state change always goes now.
// Not safe for concurrent use (only the coordinator calls it); no goroutine, no clock of its own.
type progressCoalescer struct{ /* window, until, pending */ }
func newProgressCoalescer(window time.Duration) *progressCoalescer
func (c *progressCoalescer) counters(now time.Time) (sendNow bool)
func (c *progressCoalescer) state(now time.Time)
func (c *progressCoalescer) due(now time.Time) bool
func (c *progressCoalescer) next() (time.Time, bool)
```
- **test-writer:** `metadata_notify_test.go` (mesma estrutura de `media_changes_test.go`/`notifier_test.go`):
  - `MET-054/primeira mudança de contador sai na hora`; `MET-054/mudanças dentro da janela de 500 ms viram uma no fim`; `MET-054/depois da janela, a próxima sai na hora de novo`; `MET-054/mudança de estado sai na hora e reabre a janela`; `MET-054/sem pendência, next() é false (nenhum temporizador)`; `MET-054/estado reabre a janela e descarta pendente de contador`; `MET-054/cenários em tabela (como IMG-003)`;
  - `MET-054/batchNotifier sem servidor e depois de set(nil) não faz nada nem bloqueia`; `MET-054/dois clientes IPC reais recebem a notificação com batchId, state, done, total, failed`; `MET-054/fila cheia (16) descarta sem bloquear` (IPC-017).
- Dependência: T10.

## T24 — Saúde do provedor e busca de um jogo do lote
- Requisitos: MET-011, MET-015, MET-053. **implementer:** `internal/agent/metadata_batch.go` (novo; só estas partes).
```go
// providerHealth keeps, per provider, the failures in a row and the instant until which it is suspended [MET-011]. Safe for concurrent use.
type providerHealth struct{ /* mu; per provider: fails int, until time.Time, disabled bool */ }
func newProviderHealth(clock domain.Clock) *providerHealth
func (h *providerHealth) allowed(provider string) bool          // not suspended and not disabled
func (h *providerHealth) report(provider string, err error) (nowDisabled bool) // nil resets; ErrUnavailable suspends 30/60/120 s; the 3rd in a row disables
type batchRow struct{ gameID int64; title, platform string; confidence domain.Confidence; selected int; groups []scrapers.Group }
// searchOne reads title, platform and year of the stored game, builds the Query (SearchTerm, ResolvePlatform) and asks the aggregator
// with Staged and the 15 s deadline; deleted game = row none without alternatives [MET-053].
func (m *metadataService) searchOne(ctx context.Context, gameID int64, h *providerHealth) (batchRow, failedAll bool, err error)
```
- **test-writer:** `metadata_batch_search_test.go` (provedor fake que falha N vezes, relógio fake):
  - `MET-011/falha temporária suspende 30 s, depois 60 s, depois 120 s`; `MET-011/durante a suspensão, os jogos seguem com os demais provedores`; `MET-011/3 falhas seguidas desativam até o fim do lote e geram o aviso "IGDB indisponível: a busca continuou sem ele" uma só vez`; `MET-011/sucesso zera a contagem`; `MET-011/jogo cuja busca falhou em todos os provedores fica none`; `MET-011/provedores têm contagens independentes`;
  - `MET-015/IGDB match: zero chamadas à Steam Store`; `MET-015/IGDB doubt: uma chamada`; `MET-015/sem chave do IGDB: uma chamada`;
  - `MET-053/a consulta usa título, plataforma e ano gravados do jogo`; `MET-053/a linha guarda confiança, item selecionado (o primeiro) e até 5 alternativas ≥ 0,40`; `MET-053/none: selected -1`; `MET-053/jogo apagado: linha none sem alternativas`; `MET-053/prazo de 15 s por jogo`.
- Dependência: T17, T20.

## T25 — Job do lote: início, coordenador, estado, cancelamento e descarte
- Requisitos: MET-052, MET-053, MET-054, MET-055, MET-056, MET-058, MET-059, MET-071. **implementer:** `internal/agent/metadata_batch.go` (continuação).
```go
type batchJob struct { /* mu; id; state; total, done, failed; warnings; games []int64; rows []*batchRow; choices; result *ipc.BatchResult; cancel context.CancelFunc; events chan batchEvent */ }
func (m *metadataService) batchStart(ctx context.Context, params json.RawMessage) (any, error)
func (m *metadataService) batchStatus(ctx context.Context, params json.RawMessage) (any, error)
func (m *metadataService) batchCancel(ctx context.Context, params json.RawMessage) (any, error)
func (m *metadataService) batchDiscard(ctx context.Context, params json.RawMessage) (any, error)
func (m *metadataService) runSearch(job *batchJob, ctx context.Context) // coordinator: feeds indices, starts batchWorkers workers, owns the coalescer
func nextBatchID(clock domain.Clock, last int64) int64 // UnixMilli, strictly increasing [MET-058]
```
Regras: `batchStart` valida (1..1000 distintos, ≥ 1), lê `GameDetail` de cada id (`-32001`), resolve se algum provedor aplicável tem chave (`-32010`), cria o job com `id`, responde na hora; `-32015` em `searching`/`ready`/`cancelled`/`applying`; `done` ou sem busca substitui. O job é filho de `m.root` (nunca da requisição). Coordenador: `wg.Add(1)` no serviço, 3 trabalhadores com `WaitGroup` próprio, único que fala com o `progressCoalescer` e o `batchSink`; `Clock.After` armado só com pendência. A linha só é gravada sob `job.mu` se o estado ainda for `searching`. `searching → ready` notifica na hora; a chamada da UI não notifica. `batchCancel` em `searching` vai a `cancelled` na hora; em `ready`/`cancelled` é no-op; `batchDiscard` só em `ready`/`cancelled`/`done` (outro: `-32016`). Todo método com `batchId` diferente do atual: `-32014`.
- **test-writer:** `metadata_batch_test.go` (relógio fake, provedores fake, store real; uma tabela com a máquina de `batch-state-machine.md` §2):
  - `MET-052/batchStart devolve batchId e total antes de qualquer requisição (provedor fake bloqueado)`; `MET-052/gameId inexistente: -32001 e nada começa`; `MET-052/em searching, ready, cancelled e applying: -32015 com data.state`; `MET-052/em done a nova substitui a antiga`; `MET-052/nenhum provedor aplicável com chave para nenhum jogo: -32010`; `MET-052/mais de 1000 ids e ids repetidos: -32602`; `MET-071/gameIds vazio, id 0, batchId 0: -32602`;
  - `MET-053/3 trabalhadores pegam os jogos na ordem recebida (no máximo 3 buscas ao mesmo tempo)`; `MET-053/todas as chamadas passam pelos limitadores (contagem de requisições por provedor com intervalo, relógio fake)`; `MET-053/estado vai a ready quando todos têm linha`; `MET-053/a plataforma de cada jogo vai na sua consulta`;
  - `MET-054/mudanças de contador agrupadas em 500 ms e estado final sempre avisado`; `MET-054/batchStart, batchCancel e batchDiscard não geram notificação`; `MET-054/searching → ready notifica na hora`; `MET-054/dois clientes IPC reais recebem as mesmas notificações`;
  - `MET-055/batchStatus em cada estado (null sem busca; contadores; warnings até 10; result em done)`; `MET-055/batchStatus nunca falha por estado`;
  - `MET-056/cancelar no meio: cancelled na hora, nenhuma requisição nova depois (contagem), em andamento não grava linha`; `MET-056/linhas já buscadas continuam disponíveis`; `MET-056/em ready e cancelled é no-op que devolve o estado`; `MET-056/os trabalhadores terminam sem goroutine restante`;
  - `MET-058/batchDiscard em ready, cancelled e done libera; em searching e applying: -32016`; `MET-058/batchId desconhecido ou já descartado: -32014 em qualquer estado`; `MET-058/batchId cresce estritamente mesmo com relógio parado`; `MET-058/nada é gravado no banco durante a busca (contagem de linhas antes e depois)`; `MET-058/reiniciar o agente perde a busca (novo serviço: batchStatus null)`;
  - `MET-059/desligamento cancela o contexto raiz, Wait espera os trabalhadores e o coordenador; nenhuma requisição nova depois` (-race e contagem de goroutines antes e depois);
  - tabela da máquina de estados: um caso por linha de `batch-state-machine.md` §2 com ID MET-052/MET-055/MET-056/MET-058.
- Dependência: T23, T24.

## T26 — `metadata.batchResults` em páginas dentro do teto
- Requisitos: MET-057, MET-071. **implementer:** `internal/agent/metadata_batch.go` (continuação).
```go
func (m *metadataService) batchResults(ctx context.Context, params json.RawMessage) (any, error)
// pageRows returns the rows from offset, at most limit and at most maxResultBytes of JSON (len(Marshal)+1 per row); never a cut row; next = index of the first row that did not fit or nil.
func pageRows(rows []ipc.BatchRow, offset, limit int) (page []ipc.BatchRow, next *int)
```
Só em `ready` e `cancelled` (outro: `-32016`); em `cancelled`, só as linhas que existem; linhas na ordem dos jogos; `limit` padrão 100 (ausente/0 usa o padrão), 1..200; `offset` ≥ 0.
- **test-writer:** `metadata_batch_results_test.go`:
  - `MET-057/500 linhas com título de 200 caracteres e 5 alternativas: cada página cabe em 921600 bytes e a união é todas as linhas, sem repetir nem pular`; `MET-057/menos linhas que limit quando preciso e next aponta para a primeira que não coube`; `MET-057/nenhuma linha cortada ao meio`; `MET-057/uma linha sozinha maior que o teto ainda é devolvida` (se impossível pela validação, caso que prova o limite de 200 runas por campo); `MET-057/limit padrão 100 e máximo 200`; `MET-057/offset além do fim: rows vazio, next nulo`; `MET-057/estado errado: -32016 com data.state`; `MET-057/cancelled devolve só as linhas existentes`; `MET-071/offset negativo, limit 0 explícito negativo ou 201: -32602`; `MET-057/maxResultBytes é igual a x-metadata.maxResultBytes do contrato v0.14.0` (lê o JSON).
- Dependência: T25.

## T27 — `metadata.batchApply`: execução, cancelamento e resumo
- Requisitos: MET-064, MET-065, MET-066, MET-012, MET-022, MET-071. **implementer:** `internal/agent/metadata_batch.go` (continuação).
```go
func (m *metadataService) batchApply(ctx context.Context, params json.RawMessage) (any, error)
func (m *metadataService) runApply(job *batchJob, ctx context.Context) // coordinator with 1 worker
```
Valida (`gameId` presente na busca, `candidate` dentro da lista da linha, 1..1.000, sem `gameId` repetido: `-32602`; estado fora de `ready`/`cancelled`: `-32016`); guarda as escolhas, vai a `applying` (`total` = nº de escolhas, `done`/`failed` zerados), responde `accepted`. O trabalhador aplica em ordem, com `replace = false`: reaproveita `Meta`/`Images` do grupo (zero requisição ao IGDB), pede à Steam Store e ao SteamGridDB o que falta, faz `Merge` + `ApplyMetadata` e `queueImages` (T19, mesmo caminho); erro de um jogo incrementa `failed` e guarda o título (até 10). Ao fim: `done` com `result {applied, imagesQueued, imagesSkippedByLimit, failedTitles}` e notificação imediata. `batchCancel` em `applying`: nenhum jogo novo, o em curso termina, `done` com resultado parcial, sem notificação própria (chamada da UI). A aplicação não espera pelas imagens.
- **test-writer:** `metadata_batch_apply_test.go` (store real, fila fake, provedores fake):
  - `MET-064/gameId fora da busca, candidate fora da lista e mais de 1000 escolhas: -32602`; `MET-064/gameId repetido: -32602`; `MET-064/fora de ready ou cancelled: -32016`; `MET-064/em ready vai a applying e devolve accepted na hora`; `MET-064/em cancelled só jogos com linha`;
  - `MET-065/aplica em ordem com um só trabalhador (nunca duas aplicações ao mesmo tempo)`; `MET-065/replace sempre false: campo do usuário preservado`; `MET-065/o IGDB não recebe nenhuma requisição na aplicação`; `MET-065/Steam Store e SteamGridDB são pedidos só se o grupo os tem`; `MET-065/um jogo falha: failed+1, título em failedTitles, segue para o próximo`; `MET-065/jogo apagado conta como falha`; `MET-065/failedTitles guarda no máximo 10`; `MET-065/contadores finais do result (applied, imagesQueued, imagesSkippedByLimit)`; `MET-065/não espera pela fila de mídia (fila fake bloqueada não atrasa o done)`; `MET-065/estado final done e notificação imediata`;
  - `MET-066/cancelar no meio: nenhum jogo novo, o em curso termina, o que foi aplicado fica, done com resultado parcial`; `MET-066/batchCancel em done é no-op (devolve o estado)`; `MET-066/batchDiscard em done libera`;
  - `MET-022/mesmo jogo com 1 vaga de screenshot recebe 1 pelo lote`; `MET-023/lote e metadata.apply concorrentes do mesmo jogo não passam do limite (-race)`;
  - `MET-059/desligamento durante applying: o trabalhador termina, o que foi gravado fica, nenhuma goroutine restante`.
- Dependência: T21, T25.

## T28 — Ligação no agente, ordem de desligamento e invariantes
- Requisitos: MET-059, MET-070, MET-080, MET-082. **implementer:** `internal/agent/app.go`:
  - cria `metadataService` (com o `scrapeService` já construído, o `batchNotifier`, o `Aggregator` com a fábrica e o `rootCtx` filho do `ctx` do agente), `maps.Copy(handlers, metadataHandlers(...))`; `batchNotes.set(server)` ao lado de `mediaNotes.set(server)`;
  - `instance` ganha `metadata *metadataService`, `stopMetadata context.CancelFunc` e `batchNotifier`;
  - **Shutdown:** depois de `server.Close()` e **antes** de `stopQueue()`: `a.stopMetadata()` (cancela o contexto raiz dos jobs), `a.metadata.Wait()`; depois `stopQueue()`/`a.wg.Wait()`; depois `batchNotifier.set(nil)` junto de `mediaNotifier.set(nil)`; o store é fechado por último, como hoje. (O job escreve no banco e chama `queue.Notify`; precisa parar antes de ambos.)
- **test-writer:** `internal/agent/order_test.go`, `app_test.go`, novo `metadata_wire_test.go`:
  - `MET-059/a ordem do Shutdown tem metadata.Wait depois de server.Close e antes de stopQueue e do fechamento do store` (caso novo no `order_test.go`);
  - `MET-059/Shutdown com busca em curso termina sem goroutine restante (-race)`;
  - `MET-070/as constantes do agente (batchWorkers, maxBatchGames, maxBatchChoices, defaultResultsLimit, maxResultsLimit, maxResultBytes, progressWindow, providerMaxFailures, providerCooldown, searchTimeout, gameSearchTimeout, maxSearchCandidates, maxRowCandidates, maxStatusWarnings, maxFailedTitles e os intervalos dos limitadores) são iguais ao x-metadata do contrato v0.14.0`;
  - `MET-070/o agente registra os 8 handlers metadata.* e continua com scrape.search, scrape.apply e media.queue`;
  - `MET-080/busca e aplicação com sessão ativa: a sessão, play_session e session.changed ficam intactos` (store real, tracker de teste existente da fase 3);
  - `MET-080/nenhum método de lote toca no tracker` (tracker espião, nenhuma chamada);
  - `MET-082/a suíte do pacote não faz rede nem Sleep` (revisão do diff; teste de `grep` de `time.Sleep` em `metadata*_test.go`, como o teste existente de `Sleep` do repositório se houver).
- Dependência: T21, T25, T26, T27.

## T29 — Binding do desktop: `Metadata*` e `SearchName`
- Requisitos: MET-072. **implementer:** `cmd/arcadia/metadata_unified.go` (novo), `cmd/arcadia/imports.go` (`ImportCandidateView` ganha `SearchName`, nos dois sentidos).
```go
type MetadataRefView struct{ Provider string `json:"provider"`; ProviderID string `json:"providerId"` }
type MetadataCandidateView struct{ Refs []MetadataRefView `json:"refs"`; Title string `json:"title"`; Year int `json:"year"`; Platform string `json:"platform"`; Score float64 `json:"score"`; ThumbURL string `json:"thumbUrl"` }
type MetadataSearchView struct{ Candidates []MetadataCandidateView `json:"candidates"`; Confidence string `json:"confidence"`; Warnings []string `json:"warnings"` }
type MetadataApplyView struct{ Game GameDetailView `json:"game"`; Queued int `json:"queued"`; SkippedByLimit int `json:"skippedByLimit"` }
type MetadataBatchStartView struct{ BatchID int64 `json:"batchId"`; Total int `json:"total"` }
type MetadataBatchResultView struct{ Applied, ImagesQueued, ImagesSkippedByLimit int; FailedTitles []string } // camelCase json
type MetadataBatchView struct{ BatchID int64; State string; Total, Done, Failed int; Warnings []string; Result *MetadataBatchResultView }
type MetadataBatchStatusView struct{ Batch *MetadataBatchView `json:"batch"` }
type MetadataBatchRowView struct{ GameID int64; Title, Platform, Confidence string; Selected int; Candidates []MetadataCandidateView }
type MetadataBatchResultsView struct{ Rows []MetadataBatchRowView; Total int; Next *int }
type MetadataChoiceView struct{ GameID int64 `json:"gameId"`; Candidate int `json:"candidate"` }
func (a *App) MetadataSearch(title, platform string, year int) (MetadataSearchView, error)
func (a *App) MetadataApply(gameID int64, refs []MetadataRefView, replace bool) (MetadataApplyView, error)
func (a *App) MetadataBatchStart(gameIDs []int64) (MetadataBatchStartView, error)
func (a *App) MetadataBatchStatus() (MetadataBatchStatusView, error)
func (a *App) MetadataBatchResults(batchID int64, offset, limit int) (MetadataBatchResultsView, error)
func (a *App) MetadataBatchApply(batchID int64, choices []MetadataChoiceView) (int, error)
func (a *App) MetadataBatchCancel(batchID int64) (string, error)
func (a *App) MetadataBatchDiscard(batchID int64) error
```
Timeouts: `scrapeTimeout` para `Search`, `Apply` e `BatchStart`; `libraryTimeout` para os demais. Erros pelo `callAgent` (UI-008).
- **test-writer:** `cmd/arcadia/metadata_unified_test.go` (usa `gamePinger`, `singleCall`, `newBindingApp` de `games_test.go`) e `imports_test.go`:
  - `MET-072/cada binding chama o método IPC certo com os parâmetros certos` (um caso por binding); `MET-072/MetadataSearch converte candidatos, confidence e warnings`; `MET-072/MetadataBatchStatus devolve Batch nulo quando o agente devolve null`; `MET-072/MetadataBatchResults converte linhas e next nulo`; `MET-072/ImportCandidateView leva searchName nos dois sentidos (scan e apply)`; `MET-072/erro do agente sai com a mensagem de UI-008`; `MET-072/-32014, -32015 e -32016 viram mensagem ao usuário`; `MET-072/nenhum binding chama lançamento, sessão ou pausa` (o fake registra só `metadata.*`).
- Dependência: T10.

## T30 — Evento Wails `metadata:batch` e repasse no supervisor
- Requisitos: MET-072. **implementer:** `cmd/arcadia/metadata_events.go` (novo), `cmd/arcadia/supervisor.go` (um `case ipc.MethodMetadataBatchChanged`).
```go
const metadataBatchEvent = "metadata:batch"
type MetadataBatchChangedView struct{ BatchID int64 `json:"batchId"`; State string `json:"state"`; Done, Total, Failed int } // camelCase json
func decodeBatchChanged(raw json.RawMessage) (MetadataBatchChangedView, error) // [MET-072]
```
- **test-writer:** `cmd/arcadia/metadata_events_test.go`, `supervisor_test.go` (no molde de `media_events_test.go`):
  - `MET-072/evento válido é emitido com os cinco campos`; `MET-072/params ausentes, JSON inválido, campo ausente, não inteiro, batchId 0, inteiro negativo, state desconhecido: Warn "metadata.batchChanged com parâmetros inválidos" e nada emitido`; `MET-072/o repasse não chama o agente e não muda o estado do supervisor`; `MET-072/repasse nos dois sentidos: notificação do agente vira evento e a reconexão não duplica`; `MET-072/media.changed continua repassado`.
- Dependência: T10. Arquivo compartilhado: `supervisor.go` só nesta tarefa.

## T31 — `lib/metadata.ts`: tipos e chamadas unificadas
- Requisitos: MET-043, MET-044, MET-024. **implementer:** `frontend/src/lib/metadata.ts` (a UI deixa de usar `scrapeSearch`/`scrapeApply`/`acceptedScore`; o código morto sai na T34).
```ts
export type Confidence = 'match' | 'doubt' | 'none'
export type MetadataRefView = { provider: ProviderId; providerId: string }
export type MetadataCandidateView = { refs: MetadataRefView[]; title: string; year: number; platform: string; score: number; thumbUrl?: string }
export type MetadataSearchView = { candidates: MetadataCandidateView[]; confidence: Confidence; warnings: string[] }
export type MetadataApplyView = { game: GameDetail; queued: number; skippedByLimit: number }
export function metadataSearch(title: string, platform: string, year: number): Promise<MetadataSearchView>
export function metadataApply(gameId: number, refs: MetadataRefView[], replace: boolean): Promise<MetadataApplyView>
export function providersLabel(refs: MetadataRefView[]): string // "IGDB · SteamGridDB"
export function scoreLabel(score: number): string                // percentage
export function applyResultLabel(queued: number, skipped: number): string // "N imagens na fila; M não entraram porque o jogo já tem o máximo" [MET-024]
```
- **test-writer:** `metadata.test.ts`:
  - `MET-043/metadataSearch repassa título, plataforma e ano ao binding`; `MET-043/binding ausente falha como chamada com erro`; `MET-043/metadataApply repassa gameId, refs e replace`; `MET-044/providersLabel junta os nomes na ordem igdb, steamgriddb, steamstore com " · "`; `MET-044/scoreLabel em percentual arredondado`; `MET-024/applyResultLabel: sem pulados mostra só a fila; com pulados mostra a segunda frase; singular e plural`.
- Dependência: nenhuma (usa o binding por tipo local). Paralelo ao Go.

## T32 — `lib/metadataBatch.ts`: hook, páginas e auxiliares puros
- Requisitos: MET-055, MET-061, MET-063, MET-066, MET-067, MET-057. **implementer:** `frontend/src/lib/metadataBatch.ts` (novo).
```ts
export type BatchState = 'searching' | 'ready' | 'cancelled' | 'applying' | 'done'
export type BatchResultView = { applied: number; imagesQueued: number; imagesSkippedByLimit: number; failedTitles: string[] }
export type BatchStatusView = { batchId: number; state: BatchState; total: number; done: number; failed: number; warnings: string[]; result?: BatchResultView }
export type BatchRowView = { gameId: number; title: string; platform: string; confidence: Confidence; selected: number; candidates: MetadataCandidateView[] }
export function useMetadataBatch(): { status: BatchStatusView | null; interrupted: boolean; start(ids: number[]): Promise<void>; cancel(): Promise<void>; discard(): Promise<void>; apply(choices: {gameId:number; candidate:number}[]): Promise<void> }
export async function loadBatchRows(batchId: number, onProgress?: (loaded: number, total: number) => void): Promise<BatchRowView[]> // pages of 100 until next == null
export type ReviewRow = { row: BatchRowView; choice: number; checked: boolean; byUser: boolean }
export function buildReviewRows(rows: BatchRowView[]): ReviewRow[]       // match: checked; doubt: first item, unchecked; none: choice -1, unchecked [MET-061]
export function chooseCandidate(r: ReviewRow, choice: number): ReviewRow // choice >= 0 marks; -1 unmarks and blocks [MET-061, MET-062]
export function reviewCounts(rows: ReviewRow[]): { selectable: number; checked: number; none: number } // [MET-063]
export function toChoices(rows: ReviewRow[]): { gameId: number; candidate: number }[]
```
O hook tem **uma** assinatura de `metadata:batch` (cancelada ao desmontar, como IMG-007); lê `MetadataBatchStatus` no início, ao reconectar (UI-009) e quando o `state` do evento muda; só os contadores vêm do payload; `interrupted` verdadeiro quando tinha estado e a leitura volta `batch: null`; ao ir a `done` invalida as chaves de leitura de jogos e catálogo (sem tocar em sessão).
- **test-writer:** `metadataBatch.test.ts` (fake de eventos e de binding):
  - `MET-055/lê o estado no início e na reconexão`; `MET-055/evento com o mesmo state só atualiza contadores (sem nova leitura)`; `MET-055/evento com state novo relê o estado`; `MET-055/uma só assinatura de evento e cancelada ao desmontar`; `MET-055/estado depois do descarte volta a null`;
  - `MET-067/status null depois de ter tido estado marca interrupted uma vez`; `MET-067/start com falha mostra o erro e não muda o estado`;
  - `MET-066/ao ir a done invalida jogos e catálogo e nada de sessão`;
  - `MET-057/loadBatchRows junta páginas de 100 até next nulo, com progresso`; `MET-057/página única`;
  - `MET-061/buildReviewRows: 15 linhas (9 match, 4 doubt, 2 none) → 9 marcadas; doubt com 1º item e desmarcada; none sem escolha, desmarcada`; `MET-061/chooseCandidate marca a linha; -1 desmarca e bloqueia`; `MET-062/chooseCandidate guarda byUser`; `MET-063/reviewCounts: selecionáveis, marcadas e sem correspondência`; `MET-064/toChoices só as marcadas, na ordem`.
- Dependência: T31 (tipos). Paralelo ao Go.

## T33 — `MetadataOptions` (lista única de opções)
- Requisitos: MET-044, MET-068, MET-069. **implementer:** `frontend/src/components/MetadataOptions.tsx` (novo). Apresentacional.
```ts
export type MetadataOptionsProps = {
  candidates: MetadataCandidateView[]; confidence: Confidence; warnings: string[]
  selected: number | null; onSelect: (i: number | null) => void
  renderAction?: (c: MetadataCandidateView, i: number) => React.ReactNode // "Aplicar" in the game dialog
  allowNone?: boolean // adds the "Nenhuma" option (file import) [MET-041]
}
export function MetadataOptions(props: MetadataOptionsProps): JSX.Element
```
Grupo de rádios (`role="radiogroup"`), nomes "<título> (<ano>), <provedores>"; `<img alt="" loading="lazy" referrerPolicy="no-referrer">` com queda para o placeholder de capa (`CoverImage`) em erro ou sem `thumbUrl`; selo "provável" só no primeiro item com `confidence = 'match'`; "Nenhum resultado para este nome." na lista vazia; avisos acima da lista; "Plataforma desconhecida"; só tokens `--arc-*`.
- **test-writer:** `MetadataOptions.test.tsx`:
  - `MET-044/item mostra capa, título, ano entre parênteses, plataforma, provedores e percentual`; `MET-044/ano 0 omite os parênteses`; `MET-044/plataforma vazia mostra "Plataforma desconhecida"`; `MET-044/selo provável só no primeiro item e só com match`; `MET-044/lista vazia mostra "Nenhum resultado para este nome."`; `MET-044/avisos de provedor acima da lista`; `MET-044/renderAction aparece por item`; `MET-044/allowNone adiciona "Nenhuma"`;
  - `MET-068/img com alt vazio, loading lazy e referrerPolicy no-referrer`; `MET-068/erro da imagem ou thumbUrl ausente: placeholder`;
  - `MET-069/grupo de rádios com nomes "<título> (<ano>), <provedores>"`; `MET-069/setas e Espaço escolhem` (comportamento do navegador, sem Tab por item).
- Dependência: T31.

## T34 — `MetadataDialog` reescrito e limpeza do código obsoleto
- Requisitos: MET-043, MET-044, MET-024, MET-080. **implementer:** `frontend/src/components/MetadataDialog.tsx` (reescrito), `frontend/src/lib/metadata.ts` (remove `acceptedScore`, `scrapeSearch`, `scrapeApply` e tipos sem consumidor), `frontend/src/lib/metadata.test.ts` (o test-writer remove os casos que só os cobriam).
`Modal size="medium"`; campo "Nome para busca" (título do jogo), botão "Obter dados", caixa "Substituir os campos já preenchidos", `MetadataOptions` com "Aplicar" por item; **sem** `select` de provedor; "Obter dados" chama `metadataSearch(nome, plataforma, ano)` (plataforma e ano do jogo se conhecidos); "Aplicar" chama `metadataApply(gameId, refs, replace)` e mostra `applyResultLabel`; invalidações de IMG-011 (jogo e lista) continuam.
- **test-writer:** `MetadataDialog.test.tsx` (reescrito):
  - `MET-043/campo "Nome para busca" vem com o título do jogo e é editável`; `MET-043/não há select de provedor`; `MET-043/Obter dados chama metadataSearch com o nome digitado, a plataforma e o ano do jogo`; `MET-043/jogo sem ano envia 0`; `MET-043/botão desativado durante a busca`; `MET-043/Aplicar chama metadataApply com as refs da opção e replace da caixa`; `MET-043/erro de provedor mostra a mensagem do binding`;
  - `MET-044/lista de opções aparece com selo e avisos`;
  - `MET-024/resultado "N imagens na fila; M não entraram porque o jogo já tem o máximo"`; `MET-024/sem pulados mostra só a primeira parte`;
  - `MET-043/aplicar invalida jogo e lista (IMG-011)`;
  - `MET-080/nenhuma chamada de lançamento, sessão ou pausa`.
- Dependência: T31, T33. Arquivo compartilhado: `metadata.ts` e `metadata.test.ts` passam da T31 para a T34, nesta ordem.

## T35 — `ImportDialog`: caixa "Buscar metadados e imagens" (fonte Pasta)
- Requisitos: MET-050, MET-051. **implementer:** `frontend/src/components/ImportDialog.tsx` (acrescenta; a tabela de revisão da fase 11 não muda). Estado `searchMeta` (padrão `true`, só `folder`); usa `ProviderKeyStatus` (igdb e steamgriddb) e `useMetadataBatch` (T32); desativada com "Cadastre a chave do IGDB ou do SteamGridDB em Configurações." quando não há chave aplicável à plataforma escolhida (regra de `Applicable`: Steam Store só vale se a plataforma for Windows ou sem chave de seed com categoria `pc`, mas sem chave da Steam Store a caixa **só** depende de IGDB/SteamGridDB, MET-050) e com "Há metadados para revisar. Revise ou descarte antes de buscar de novo." em `ready`/`cancelled`. Depois de `applyImportInLots` terminar com ao menos um jogo gravado: `onImported()` e em seguida `batch.start(ids)` com `applied[].gameId` em ordem; falha do start vira aviso e nada é desfeito; lote interrompido por falha: só os gravados.
- **test-writer:** `ImportDialog.test.tsx` (continuação; a fase 11 já o reescreveu):
  - `MET-050/fonte Pasta tem a caixa marcada por padrão`; `MET-050/Steam, Epic, LaunchBox e Arquivo não têm a caixa`; `MET-050/sem as duas chaves: desativada com a mensagem de Configurações`; `MET-050/só uma chave cadastrada: ativa`; `MET-050/busca em ready ou cancelled: desativada com "Há metadados para revisar..."`; `MET-050/desmarcar importa sem chamar batchStart`;
  - `MET-051/ordem: lotes de import.apply, onImported, depois metadataBatchStart com os gameId do resultado na ordem`; `MET-051/nenhum jogo gravado: batchStart não é chamado`; `MET-051/"Importados: N" aparece antes de batchStart responder`; `MET-051/falha do batchStart aparece como aviso e a importação não é desfeita`; `MET-051/falha no 2º lote: batchStart só com os gravados dos lotes que passaram`; `MET-051/o título enviado ao lote é o gravado (editado na revisão)`.
- Dependência: T32, T29 (tipos do binding; o frontend usa tipos locais, sem bloqueio de build).

## T36 — `ImportDialog`: "Nome para busca" e "Obter dados" do arquivo avulso
- Requisitos: MET-041, MET-042, MET-040. **implementer:** `frontend/src/lib/imports.ts` (`ImportCandidate` ganha `searchName?: string`), `frontend/src/components/ImportDialog.tsx`. Fonte `file`, abaixo da tabela: campo "Nome para busca" (preenchido com `candidate.searchName`, editável) e botão "Obter dados" (desativado durante a busca e sem plataforma) → `metadataSearch(nome, plataformaEscolhida, 0)` → `MetadataOptions` com `allowNone` ("Nenhuma" marcada por padrão); confiança `match` pré-seleciona a primeira. Todo `resetList()` descarta busca e escolha. Ao importar: `ImportApply` primeiro e, com o `gameId` do resultado, `metadataApply(gameId, refs, false)`; recusa do import ou falha do apply mostram "Jogo importado; os dados não foram aplicados: use Buscar metadados no painel" sem desfazer; sem opção, nada é chamado.
- **test-writer:** `ImportDialog.test.tsx` (continuação) e `imports.test.ts` (campo `searchName` nos tipos):
  - `MET-041/campo Nome para busca vem de searchName e é editável`; `MET-041/Obter dados chama metadataSearch com o nome, a plataforma escolhida e ano 0`; `MET-041/opção Nenhuma vem marcada; match pré-seleciona a primeira`; `MET-041/trocar o nome, o arquivo ou a plataforma descarta a lista e a escolha`; `MET-041/botão desativado durante a busca e sem plataforma`; `MET-041/fontes Pasta, Steam, Epic e LaunchBox não têm o painel`;
  - `MET-042/importar chama ImportApply e só depois metadataApply com o gameId e as refs, replace false`; `MET-042/ImportApply recusa o jogo: aviso e metadataApply não é chamado`; `MET-042/metadataApply falha: aviso e o jogo importado fica`; `MET-042/sem opção escolhida (Nenhuma): metadataApply não é chamado`; `MET-042/metadataApply chamado uma vez só`;
  - `MET-040/searchName ausente em outras fontes não quebra o tipo`.
- Dependência: T31, T33, T35 (mesmo arquivo `ImportDialog.tsx`, nesta ordem).

## T37 — `MetadataReviewDialog` (janela "Confirmar metadados")
- Requisitos: MET-060, MET-061, MET-062, MET-063, MET-064, MET-066 (Descartar), MET-068, MET-069. **implementer:** `frontend/src/components/MetadataReviewDialog.tsx` (novo).
```ts
export type MetadataReviewDialogProps = {
  batch: BatchStatusView
  onClose: () => void                       // closes without discarding
  onApply: (choices: { gameId: number; candidate: number }[]) => Promise<void>
  onDiscard: () => Promise<void>
}
export function MetadataReviewDialog(props: MetadataReviewDialogProps): JSX.Element
```
`Modal size="large"`; carrega `loadBatchRows` mostrando "k de n jogos"; `<table>` com `<th scope="col">`: caixa, Capa, Jogo (título e plataforma gravados), Encontrado (título, ano e plataforma), Situação ("provável", "em dúvida", "nenhum", "escolhido por você" com a pontuação em %), Alternativas (`select` com as alternativas na ordem do agente mais "Nenhum"); linha em `React.memo` com callbacks estáveis; cabeçalho com "Selecionar todos" (três estados, desativado sem linha marcável e durante a aplicação) e o texto "k de n jogos selecionados" mais " (j sem correspondência)"; "Aplicar selecionados" (ativo com k ≥ 1) → `onApply(toChoices(...))` e fecha; "Descartar tudo" → `onDiscard`; título e botões fixos, corpo rolável (UIX-038, UIX-039); Esc fecha sem descartar, salvo durante chamada em curso; cores só `--arc-*`.
- **test-writer:** `MetadataReviewDialog.test.tsx`:
  - `MET-060/colunas e cabeçalho fixo; corpo rola; título e botões fixos`; `MET-060/carrega em páginas e mostra "k de n jogos" enquanto carrega`; `MET-060/estrutura modal grande (data-size large)`;
  - `MET-061/15 linhas (9 provável, 4 dúvida, 2 nenhum): 9 marcadas, dúvida desmarcada com 1º item, nenhum desmarcada e sem caixa ativa`; `MET-061/escolher alternativa marca a linha; Nenhum desmarca e bloqueia`;
  - `MET-062/trocar a alternativa muda Encontrado e capa, situação vira "escolhido por você" e a pontuação continua em %`; `MET-062/select lista título, ano e plataforma na ordem do agente e "Nenhum"`;
  - `MET-063/Selecionar todos: vazio, indeterminado e tudo`; `MET-063/desativado sem linha marcável e durante a aplicação`; `MET-063/contador com e sem "(j sem correspondência)"`;
  - `MET-064/Aplicar selecionados envia só as linhas marcadas com o índice da alternativa e fecha`; `MET-064/desativado com zero marcadas`; `MET-064/Descartar tudo chama onDiscard`;
  - `MET-068/capa com falha cai no placeholder`;
  - `MET-069/th scope col; nomes "Selecionar <título>" e "Alternativas de <título>"; Tab percorre caixa e select; Espaço alterna a caixa; Esc fecha sem descartar; Esc ignorado durante chamada em curso`; `MET-060/digitar ou trocar uma linha não re-renderiza as outras (React.Profiler)`.
- Dependência: T32, T33.

## T38 — `MetadataBatchChip` e abertura automática no `App`
- Requisitos: MET-066, MET-067. **implementer:** `frontend/src/components/MetadataBatchChip.tsx` (novo), `frontend/src/App.tsx` (único arquivo compartilhado desta tarefa).
```ts
export type MetadataBatchChipProps = {
  status: BatchStatusView | null; interrupted: boolean
  onOpenReview: () => void; onCancel: () => void; onDiscard: () => void; onCloseSummary: () => void
}
export function MetadataBatchChip(p: MetadataBatchChipProps): JSX.Element | null
```
Textos: "Buscando metadados k/n" (searching, com "Cancelar"), "Revisar metadados (n)" (ready e cancelled, com "Descartar"), "Aplicando metadados k/n" (applying, com "Cancelar"), o resumo em `done` ("Metadados aplicados em N jogos; M imagens na fila; K falharam"; "Fechar" chama `batchDiscard`); aviso único "A busca de metadados foi interrompida. Use Buscar metadados no jogo." quando `interrupted`. `App.tsx` guarda `reviewOpen`; ao ir a `ready` com nenhum modal aberto, ou só a janela de importação aberta e sem operação em curso, abre a confirmação sozinha (a de importação fecha antes); com outro modal, só o indicador muda e pisca uma vez.
- **test-writer:** `MetadataBatchChip.test.tsx` e `App.test.tsx`:
  - `MET-067/searching mostra "Buscando metadados k/n" e Cancelar`; `MET-067/ready e cancelled mostram "Revisar metadados (n)" e Descartar`; `MET-067/applying mostra "Aplicando metadados k/n" e Cancelar`; `MET-067/sem busca não mostra nada`; `MET-067/interrupted mostra o aviso uma vez`;
  - `MET-067/transição para ready sem modal abre a confirmação sozinha`; `MET-067/só com a janela de importação aberta e sem operação: ela fecha e a confirmação abre`; `MET-067/janela de importação em operação: não abre`; `MET-067/outro modal aberto: não abre, só o indicador muda e pisca`; `MET-067/ready já visto ao reabrir o app (primeira leitura) não reabre sozinho` (só transição de estado abre);
  - `MET-066/done mostra o resumo com os três números e Fechar chama batchDiscard`; `MET-066/ao entrar em done a lista de jogos e o catálogo são invalidados`; `MET-066/Cancelar em applying chama batchCancel`;
  - `MET-067/reconexão relê o estado (MET-055)`.
- Dependência: T32, T37.

## T39 — Invariantes de UI e varredura de tema
- Requisitos: MET-080, MET-060 (UIX-006). **test-writer:** `frontend/src/App.invariants.test.tsx` ganha `MET-080/obter dados, escolher opção, marcar a caixa, importar, acompanhar o progresso, confirmar, cancelar e descartar chamam só MetadataSearch, MetadataApply, MetadataBatch*, ImportScan, ImportApply, PickFile, PickFolder, ProviderKeyStatus e leituras; nenhum LaunchGame, LaunchGameUntracked, ToggleSession, Pause nem Resume`; `theme-scan.test.ts` ganha `MET-060/MetadataOptions.tsx, MetadataReviewDialog.tsx e MetadataBatchChip.tsx e MetadataDialog.tsx sem cores literais` (se a varredura já usa glob, só o caso nomeado). **implementer:** só se os testes revelarem cor literal ou chamada indevida.
- Dependência: T34, T36, T38.

## T40 — Verificação, rastreio, documentação e revisão do diff
- Requisitos: MET-070, MET-081, MET-082, MET-025. **Responsável:** sessão principal (e o reviewer).
  1. `go build ./...`, `go vet ./...`, `CGO_ENABLED=0 go build ./...`, `go test ./...` e `go test ./... -race` em `internal/scrapers`, `internal/agent`, `internal/domain`, `internal/importers` e `cmd/arcadia` sem internet.
  2. `go test ./internal/ipc/... ./internal/scrapers/... ./internal/agent/... ./cmd/arcadia/... -v` conferindo os casos MET-070 (contrato e "só acréscimo"), MET-054, MET-057 e MET-059.
  3. Frontend em `frontend/`: `npm test`, `npx tsc --noEmit`, `npm run build`; depois `cd cmd/arcadia && wails build -ldflags "-X main.version=<v>"` para regerar `frontend/wailsjs/go/**` (achado 8); se versionado, entra no commit.
  4. `powershell -NoProfile -ExecutionPolicy Bypass -File scripts/spec-trace.ps1 -Spec docs/specs/phase-12-metadados-unificados`, e também nas fases 7, 10, 11 (requisitos anotados "alterado/substituído em 2026-11-15").
  5. `git diff --stat` da fase: só `internal/domain/{match.go,match_unified.go}`, `internal/scrapers/*`, `internal/importers/{search_name.go,file.go,types.go}`, `internal/store/{queries/media.sql,dbgen/*,media.go}`, `internal/agent/{scrape.go,scrape_providers.go,metadata*.go,notifier.go,imports.go,app.go}`, `internal/ipc/{metadata.go,import.go,protocol.go}`, `cmd/arcadia/{metadata_unified.go,metadata_events.go,supervisor.go,imports.go}`, `frontend/src/**`, testes e docs; **nenhum** arquivo em `internal/tracker`, `cmd/fake-game`, `internal/store/migrations` nem em tabelas de sessão (MET-025, MET-081); contratos anteriores (fases 9, 10.2, 11) intactos.
  6. Revisão de concorrência e recursos (ver seção abaixo): nenhum `go func` fora de `metadataService`/`Aggregator`/`Limiter` (que não tem); nenhum `time.Now()`/`time.Sleep()` em lógica testável (`rg "time\.(Now|Sleep)" internal/scrapers internal/agent/metadata*`); nenhum teste com rede.
  7. Documentação: `docs/ARCHITECTURE.md` (seção de scrapers e agente), `docs/ROADMAP.md` (fase 12 concluída no código) e `CLAUDE.md` (spec atual) já referenciam a fase; ajustar o que a implementação mudou. Os ADRs 0033 e 0034 já existem.
- Dependência: T01..T39.

## T41 — Conferência manual ponta a ponta (MET-083)
- Requisitos: MET-083 (`· verificação: manual`). **Responsável:** sessão principal com o usuário. Pré-requisito: T40 verde, `wails build` gerado, chaves do IGDB e do SteamGridDB cadastradas.
- **Roteiro:**
  1. Importar jogos, fonte "Pasta", plataforma Sony Playstation, `E:\Games\emulators roms\ps1` com subpastas, caixa "Buscar metadados e imagens" marcada: os jogos entram e aparecem na biblioteca de imediato; o indicador mostra "Buscando metadados k/n".
  2. Ao terminar, a janela "Confirmar metadados" abre sozinha com os "prováveis" marcados e os demais desmarcados; trocar uma alternativa (a linha marca e vira "escolhido por você"), desmarcar um jogo, "Aplicar selecionados": capas e screenshots chegam aos cartões e ao painel conforme baixam, sem passar do limite por tipo (conferir nenhum jogo com mais de 2 capas ou 10 screenshots).
  3. Repetir com `n64` e com uma pasta de PC na plataforma Windows (Steam Store e SteamGridDB entram; conferir `agent.log` sem `Error`, sem rajada de 429).
  4. Importar um `.exe` avulso: ver o nome sugerido, "Obter dados" lista opções, escolher uma, importar: o jogo vem com os dados. Em outro `.exe`, escolher "Nenhuma": importa sem dados.
  5. Cancelar uma busca no meio e revisar o que veio (indicador "Revisar metadados (n)"); descartar.
  6. Fechar o app e reabrir durante uma busca: o agente continua e o indicador reaparece; reiniciar o agente durante a busca mostra "A busca de metadados foi interrompida...".
  7. Abrir "Buscar metadados" no painel de um jogo: sem escolha de provedor, "Obter dados", aplicar uma opção; resultado "N imagens na fila; M não entraram...".
  8. Janela em 1024 x 640 e 1920 x 1080: confirmação com cabeçalho fixo e rolagem só no corpo; Tab, Espaço e Esc; nenhum jogo lançado nem sessão criada durante todo o roteiro (MET-080).
  9. Anotar a cobertura de retro do IGDB + SteamGridDB para decidir a fase 12.1 (ScreenScraper).
- Dependência: T40.

---

## Ordem

- **Bloco A (paralelo, sem dependência):** T01 (sessão principal), T02 (sessão principal + usuário), T03, T05, T06, T07, T08, T09, T10, T12, T31.
- **Bloco B (depois do A):** T04 (T03) · T11 (T01, T10) · T13 (T02, T05, T06, T12) · T14 (T02, T05, T12) · T15 (T05, T12) · T16 (T04, T12) · T19 (T07, T09) · T22 (T08, T10) · T23 (T10) · T29 (T10) · T30 (T10) · T32 (T31) · T33 (T31).
- **Bloco C:** T17 (T04, T06, T12, T16) · T18 (T05, T13, T14, T15) · T34 (T31, T33) · T37 (T32, T33).
- **Bloco D (agente, sequencial no mesmo arquivo):** T20 (T10, T17, T18, T19) → T21 (T19, T20) → T24 (T17, T20) → T25 (T23, T24) → T26 (T25) → T27 (T21, T25) → T28 (T21, T25, T26, T27).
- **Bloco E (frontend, sequencial no mesmo arquivo):** T35 (T32, T29) → T36 (T31, T33, T35) · T38 (T32, T37) → T39 (T34, T36, T38).
- **Depois:** T40 → T41.
- **Paralelismo seguro:** os Go de provedores (T13, T14, T15) rodam juntos depois do Bloco A; o Bloco D só começa depois de T17 e T18; o frontend (T31..T39) corre em paralelo ao Go, pois usa tipos locais; T29 e T30 (`cmd/arcadia`) são independentes entre si (arquivos diferentes, exceto `supervisor.go` só na T30 e `imports.go` só na T29).
- **Arquivos compartilhados (não paralelos):**
  - `internal/scrapers/provider.go`: só T12. `igdb.go`: só T13. `steamgriddb.go`: só T14. `steamstore.go`: só T15. `aggregate.go`: T16 e depois T17.
  - `internal/domain/match_unified.go`: T03 e depois T04 (mesmo arquivo de teste `match_unified_test.go`).
  - `internal/agent/scrape.go` e `scrape_test.go`/`media_changed_test.go`: só T19. `scrape_providers.go`: só T18.
  - `internal/agent/metadata.go`: T20 e depois T21. `metadata_batch.go`: T24, T25, T26, T27 nessa ordem (os arquivos de teste são distintos, cada um da sua tarefa).
  - `internal/agent/app.go`, `order_test.go`: só T28. `notifier.go`: só T23. `imports.go` e `imports_test.go` do agente: só T22.
  - `internal/ipc/metadata.go`, `import.go`, `protocol.go`: só T10; `contract_test.go`, `contract_phase11_test.go`: só T11.
  - `cmd/arcadia/imports.go`: só T29. `supervisor.go`: só T30.
  - `frontend/src/lib/metadata.ts` e `.test.ts`: T31 e depois T34. `lib/imports.ts`: só T36. `ImportDialog.tsx` e `.test.tsx`: T35 e depois T36. `App.tsx` e `App.test.tsx`: só T38. `App.invariants.test.tsx` e `theme-scan.test.ts`: só T39.

## Riscos de concorrência e recursos

- **Goroutines (dono e saída):**
  - *Coordenador do job* (busca e aplicação): dono `metadataService` (`wg`), criado em `batchStart` e `batchApply`; termina quando todos os índices voltam ou quando o contexto do job (filho de `m.root`, nunca da requisição) é cancelado.
  - *3 trabalhadores da busca / 1 da aplicação*: dono o coordenador (`sync.WaitGroup` próprio); terminam quando o canal de índices fecha ou o contexto acaba; o coordenador só termina depois deles.
  - *Uma por provedor dentro de `Aggregator.Search`*: dono a própria chamada (`WaitGroup`); nenhuma sobrevive a ela. O prazo é `Clock.After` com `context.WithCancel` (sem `time.AfterFunc`, sem `time.Now`).
  - `Limiter` e `providerHealth`: **sem goroutine** (horário reservado sob mutex; suspensão é um instante do relógio).
  - `progressCoalescer`: sem goroutine nem relógio próprio; só o coordenador o usa; `Clock.After` armado apenas com pendência.
- **Rate limit:** um `Limiter` por provedor, criado na fábrica (T18) e compartilhado por busca única, lote e aplicação; `Wait` reserva o horário sob mutex e espera por `clock.After`, sai com `ctx.Err()` ao cancelar (a reserva fica gasta, pior caso um intervalo perdido). Uma busca única durante um lote espera no máximo os 3 trabalhadores à frente (≈ 1 s no IGDB, ≈ 6 s na Steam Store); cabe no `scrapeTimeout` de 30 s do desktop.
- **Locks e ordem:** `job.mu` nunca é segurado durante E/S de rede, de banco ou enquanto se toma um lock de jogo; `gameLocks` (por jogo, T19) só segura o contador, a seleção e `EnqueueMedia` (store), nunca chama provedor; `m.mu` (ponteiro do job) é segurado só para ler/trocar `m.batch`. Ordem permitida: `m.mu` → `job.mu`; `gameLock` não se combina com os dois. `providerHealth.mu` é folha.
- **Cancelamento:** `batchCancel` em `searching` cancela o contexto e muda o estado na hora; um trabalhador com requisição em curso termina sozinho e **não grava** a linha (a escrita confere o estado sob `job.mu`); `batchApply` e `batchDiscard` não esperam por ele. Em `applying`, o jogo em curso termina (o trabalhador confere `ctx` entre jogos, não dentro de `ApplyMetadata`).
- **Shutdown (T28):** `server.Close()` → `stopMetadata()` + `metadataService.Wait()` → `stopQueue()` + `a.wg.Wait()` → `notifier.set(nil)`, `mediaNotifier.set(nil)`, `batchNotifier.set(nil)` → `tracker.Close` → `coord.Close` → `store.Close`. O job escreve no banco e chama `queue.Notify`, então para antes de ambos. Teste de ordem no `order_test.go`.
- **Handles do Windows:** nenhum aberto por esta fase; nenhum arquivo Win32. O `http.Client` é o já injetado (`mediaHTTPTimeout`).
- **Memória:** 1.000 linhas × até 5 grupos com `Meta` e até ~25 URLs, da ordem de 20 MB, só em memória (decisão 7); `batchDiscard` libera. A página de resultados tem teto de 900 KiB (ADR 0032).
- **Notificação perdida:** a fila de 16 por conexão descarta (IPC-017); `batchStatus` na reconexão e a cada mudança de estado cobre.
- **Tracker e sessão:** nada muda; testes da T28 provam que busca e aplicação com sessão ativa não alteram `play_session` nem `session.changed`.
- **Rede em testes:** nenhuma; `httptest` e fakes; relógio fake em limitador, prazos, janela de avisos e suspensão (MET-082).

## Rastreabilidade

| Requisito | Tarefas |
|---|---|
| MET-001 | T13, T17, T20 |
| MET-002 | T17, T20 |
| MET-003 | T02, T06, T12, T13, T15, T20 |
| MET-004 | T03, T17, T20 |
| MET-005 | T04, T16 |
| MET-006 | T03, T04, T16 |
| MET-007 | T03, T04 |
| MET-008 | T16, T17, T20 |
| MET-009 | T04, T17, T20 |
| MET-010 | T05, T13, T14, T15, T18 |
| MET-011 | T24 |
| MET-012 | T02, T12, T13, T18, T21, T27 |
| MET-013 | T02, T14, T21 |
| MET-014 | T12, T15, T21 |
| MET-015 | T17, T24 |
| MET-020 | T07 |
| MET-021 | T07, T09, T19 |
| MET-022 | T19, T21, T27 |
| MET-023 | T19, T27 |
| MET-024 | T19, T21, T31, T34 |
| MET-025 | T07, T09, T40 |
| MET-030 | T21 |
| MET-031 | T20, T21 |
| MET-032 | T21 |
| MET-033 | T21 |
| MET-040 | T08, T10, T11, T22, T36 |
| MET-041 | T36 |
| MET-042 | T36 |
| MET-043 | T31, T34 |
| MET-044 | T31, T33, T34 |
| MET-050 | T35 |
| MET-051 | T35 |
| MET-052 | T25 |
| MET-053 | T24, T25 |
| MET-054 | T10, T11, T23, T25 |
| MET-055 | T25, T32 |
| MET-056 | T25 |
| MET-057 | T26, T32 |
| MET-058 | T25 |
| MET-059 | T25, T27, T28 |
| MET-060 | T37, T39 |
| MET-061 | T32, T37 |
| MET-062 | T32, T37 |
| MET-063 | T32, T37 |
| MET-064 | T27, T32, T37 |
| MET-065 | T27 |
| MET-066 | T27, T32, T38 |
| MET-067 | T32, T38 |
| MET-068 | T13, T15, T20, T33, T37 |
| MET-069 | T33, T37 |
| MET-070 | T01, T10, T11, T28, T40 |
| MET-071 | T10, T20, T21, T25, T26, T27 |
| MET-072 | T29, T30 |
| MET-080 | T20 (sem sessão), T28, T34, T39 |
| MET-081 | T09, T40 |
| MET-082 | T28, T40 (e todos os testes) |
| MET-083 | T41 (manual) |

Os 57 requisitos têm tarefa. Lacunas explícitas para o spec-writer (nenhuma bloqueia): (1) nome `UnifiedScore` x `ScoreGroup` (achado 1); (2) ganchos `Skip`/`Report` em `SearchOptions` (achado 2); (3) os parâmetros exatos de IGDB e SteamGridDB dependem da T02.

## Decisões novas que merecem ADR

Nenhuma além dos **0033** e **0034** (já escritos e commitados). Se a T02 revelar que um parâmetro da API não existe e isso mudar uma regra visível (por exemplo, sem filtro de jogo principal no IGDB), vale registrar no 0033 ou num ADR curto.