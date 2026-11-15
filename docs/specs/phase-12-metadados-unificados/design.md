# Fase 12 — Metadados unificados: design

- **Requisitos:** [requirements.md](requirements.md) (rascunho, 2026-11-15) · **Tarefas:** `tasks.md` (planner)
- **Contratos:** [contracts/ipc-v0.14.0-delta.md](contracts/ipc-v0.14.0-delta.md) (acréscimo sobre a v0.13.0; o JSON completo é
  uma tarefa), [contracts/match-rules.md](contracts/match-rules.md) (pontuação, limiares, aplicabilidade, prioridades, vetores) e
  [contracts/batch-state-machine.md](contracts/batch-state-machine.md) (estados da busca em lote).
- **ADRs:** 0015, 0026, 0027, 0030, 0031, 0032; propostos 0033 (busca unificada e confirmação em lote) e 0034 (limite de
  imagens contando as atuais e as enfileiradas).

## 0. Estado atual (conferido no código)

| Ponto | Onde | O que acontece hoje |
|---|---|---|
| Provedores | `internal/scrapers/{igdb,steamgriddb,steamstore}.go`, `provider.go` | `MetadataProvider{ID, Search, Details, Media}`. IGDB: `search "<título>"` sem filtro de plataforma (só devolve `Platform = "Windows"` se achar), `Media` = só screenshots (2ª requisição). SteamGridDB: `search/autocomplete` e `grids/game/<id>` (todas as capas, sem filtro de forma). Steam Store: `storesearch` e `appdetails`; `Details` e `Media` repetem a mesma requisição. Nenhum limita requisições. |
| Plataforma por provedor | `internal/scrapers/platforms.go` | `PlatformID(seedKey, "igdb")`; só o IGDB tem id. |
| Pontuação | `internal/domain/match.go` | `MatchScore` = Dice sobre bigramas + 0,05 plataforma + 0,05 ano; `MatchThreshold` 0,80. Sem trava de número (`Resident Evil` × `Resident Evil 2` dá 0,92). |
| Merge de texto | `internal/scrapers/merge.go` | `Merge(map[provider]Metadata)`: IGDB e depois Steam Store, primeiro valor vence. |
| Busca e aplicação | `internal/agent/scrape.go` | `scrape.search(gameId, provider)` e `scrape.apply(gameId, provider, providerId, replace)`: **um** provedor por vez; `queueImages` enfileira **todas** as imagens, sem limite por tipo. |
| Fila de mídia | `internal/agent/media_queue.go`, `media_changes.go`, `notifier.go` | 2 downloads em paralelo, 3 tentativas, `media.changed` agrupado em 500 ms por jogo; `mediaNotifier` leva as notificações ao `ipc.Server`. |
| Limite de imagens | `internal/domain/media_limits.go`, `internal/agent/import_images.go` | `MediaKindLimit`: screenshot 10, demais 2; só a cópia do LaunchBox o usa e conta as imagens atuais (ADR 0030). |
| Importação | `internal/agent/imports.go`, `internal/ipc/import.go` | `import.apply` devolve `applied[{gameId, title}]` em lotes de 25 (AJU-025); a janela tem a tabela de revisão (fase 11). Sem busca de metadados. |
| Diálogo | `frontend/src/components/MetadataDialog.tsx`, `lib/metadata.ts` | `select` de provedor, "Buscar", lista de candidatos com "Aplicar"; usa `ScrapeSearch` e `ScrapeApply`. |
| Eventos | `cmd/arcadia/supervisor.go`, `media_events.go` | `media.changed` vira o evento Wails `media:changed` (IMG-005). |
| Contrato | `…/phase-11…/ipc.openrpc.json` v0.13.0 | Códigos até -32013; sem métodos `metadata.*`. |

Achados que a spec trata: (1) `scrape.apply` pode enfileirar dezenas de imagens (sem teto); (2) IGDB não traz capa; (3) a Steam
Store faz a mesma requisição duas vezes por jogo; (4) o IGDB cria um objeto novo (e pede um token novo) a cada chamada, o que num
lote de 1.000 jogos seria 1.000 tokens; (5) a pontuação atual aceitaria sequela como "provável".

## 1. Decisões que o roadmap deixou em aberto (resumo; a justificativa está em requirements.md)

| Pergunta | Decisão |
|---|---|
| ScreenScraper | Fora; o agregador deixa a porta aberta (§4.6). |
| Limiar de "em dúvida" | provável ≥ 0,85 (`sim` ≥ 0,80, vantagem ≥ 0,05, sem trava); dúvida ≥ 0,60; alternativas ≥ 0,40. |
| Requisições por provedor | IGDB 334 ms, SteamGridDB 500 ms, Steam Store 2 s entre requisições; Steam só quando preciso no lote. |
| Limite de imagens conta as atuais? | Sim, mais os jobs ativos; em todo caminho da fila. |
| Screenshots do SteamGridDB | Não existem na API; capa (retrato) e herói (fundo) no lugar. |
| Confirmação | Janela própria depois da importação; "em dúvida" desmarcada. |

## 2. Arquivos e responsabilidades

| Arquivo | Mudança | IDs |
|---|---|---|
| `internal/domain/match_unified.go` (novo) | `SearchTerm`, `MatchKey`, `Marks`, `SameGame`, `ScoreGroup`, `ClassifyMatch`, constantes `Match*` | MET-004..MET-009 |
| `internal/scrapers/provider.go` | `Query` ganha `SeedKey`, `Category`; `Candidate` ganha `PlatformKeys`, `PlatformConfirmed`, `ThumbURL`, `Meta`, `Images`; interface opcional `Fetcher` | MET-003, MET-012..MET-014 |
| `internal/scrapers/igdb.go` | consulta com filtro de plataforma e 2ª sem filtro; campos de texto + `cover`/`screenshots`/`artworks` numa requisição; `Fetch` | MET-003, MET-012 |
| `internal/scrapers/steamgriddb.go` | `grids` em retrato (estáticas) + `heroes`; sem screenshot | MET-013 |
| `internal/scrapers/steamstore.go` | uma requisição `appdetails` para texto e mídia (`Fetch`); `ThumbURL` pelo `appid` | MET-014 |
| `internal/scrapers/limiter.go` (novo) | `Limiter` por provedor, `Wait(ctx)`, relógio injetado | MET-010 |
| `internal/scrapers/applicability.go` (novo) | `Applicable(provider, PlatformInfo)` | MET-003 |
| `internal/scrapers/platforms.go` | inverso do mapa do IGDB (`SeedKeysForIGDB`) | MET-003 |
| `internal/scrapers/aggregate.go` (novo) | `Aggregator.Search`, junção em grupos, escalonamento da Steam Store, avisos | MET-001, MET-005, MET-015 |
| `internal/scrapers/images.go` (novo) | `SelectImages` (prioridade, URL repetida, limite) | MET-020, MET-021 |
| `internal/importers/search_name.go` (novo) | `SearchName(path)` e as listas de genéricos | MET-040 |
| `internal/importers/file.go`, `internal/agent/imports.go` | preenche `searchName` na fonte "Arquivo" | MET-040 |
| `internal/store/queries/media.sql`, `internal/store/media.go` | consulta `ListActiveMediaJobsForGame` e `ActiveMediaJobCounts(ctx, gameID)`; **sem migração** | MET-021, MET-081 |
| `internal/agent/metadata.go` (novo) | `metadata.search`, `metadata.apply`, validação, erros | MET-001, MET-002, MET-030..MET-033, MET-071 |
| `internal/agent/metadata_batch.go` (novo) | job de lote (busca, resultados, aplicação), máquina de estados | MET-052..MET-059, MET-064..MET-066 |
| `internal/agent/metadata_notify.go` (novo) | notificador do lote e agrupador de contadores | MET-054 |
| `internal/agent/scrape.go` | `queueImages` com limite, trava por jogo e contadores; `scrape.apply` e `media.queue` passam por ela | MET-021..MET-024 |
| `internal/agent/scrape_providers.go` | fábrica com limitadores compartilhados e IGDB reaproveitável no lote | MET-010 |
| `internal/agent/app.go` | liga o serviço de metadados, o notificador e o `Wait` no desligamento | MET-059 |
| `internal/ipc/metadata.go` (novo), `internal/ipc/import.go` | tipos, métodos, erros, `searchName` | MET-070 |
| `cmd/arcadia/metadata.go`, `metadata_events.go`, `supervisor.go`, `imports.go` | bindings, evento `metadata:batch`, `SearchName` na view | MET-072 |
| `frontend/src/lib/metadata.ts`, `lib/metadataBatch.ts` (novo) | tipos, chamadas, hook do lote | MET-043..MET-067 |
| `frontend/src/components/MetadataDialog.tsx`, `MetadataOptions.tsx` (novo) | sem provedor; "Nome para busca" + "Obter dados"; lista única | MET-043, MET-044 |
| `frontend/src/components/ImportDialog.tsx` | caixa do lote, painel de busca do arquivo, chamadas depois dos lotes | MET-041, MET-042, MET-050, MET-051 |
| `frontend/src/components/MetadataReviewDialog.tsx`, `MetadataBatchChip.tsx` (novos), `App.tsx` | confirmação, indicador e abertura automática | MET-060..MET-069 |
| `docs/specs/phase-12-metadados-unificados/contracts/ipc.openrpc.json` (criado na T01) | contrato completo v0.14.0 | MET-070 |

Nada em `internal/tracker`, `cmd/fake-game`, migrações ou tabelas de sessão (MET-081). A única consulta nova do store é de
leitura.

## 3. Domínio: chave, pontuação e confiança [MET-004..MET-009]

```go
// internal/domain/match_unified.go  (puro: sem relógio, disco ou rede)

const (
	MatchAuto          = 0.85 // minimum score of a "match" [MET-009]
	MatchMinSimilarity = 0.80 // minimum title similarity of a "match" (bonuses cannot make up for a weak title)
	MatchMargin        = 0.05 // minimum lead over the second item
	MatchDoubt         = 0.60 // minimum score of a "doubt"
	MatchAlternative   = 0.40 // floor of any listed item
	MatchCap           = 0.79 // ceiling of a trapped score [MET-007]
)

type Confidence string

const (
	ConfidenceMatch Confidence = "match"
	ConfidenceDoubt Confidence = "doubt"
	ConfidenceNone  Confidence = "none"
)

// SearchTerm is the text sent to every provider [MET-004]. It returns "" when nothing is left.
func SearchTerm(title string) string

// MatchKey is the comparison key of a title [MET-006]: NormalizeTitle, minus the, a, an, and, with the Roman numerals ii..xx as numbers.
func MatchKey(title string) string

// Marks lists the terms of a key that tell games of one series apart: numbers and loose letters, except s, t, d, m [MET-007].
func Marks(key string) []string

// Member is one provider's candidate inside a group.
type Member struct {
	Provider          string   // "igdb", "steamstore", "steamgriddb"
	Title             string
	Year              int      // 0 = unknown
	PlatformKeys      []string // seed keys the provider lists; empty = no information
	PlatformConfirmed bool     // false for the IGDB candidates of the unfiltered second query [MET-003]
}

// SameGame says whether two members of different providers are the same game [MET-005].
func SameGame(a, b Member) bool

// Query is what the user searched for. LibraryKey is the seed key of the library platform, "" when it has none.
type Query struct {
	Title      string
	Year       int
	LibraryKey string
}

// Scored is the result of scoring a group; Score and Sim are in thousandths, so thresholds compare as integers.
type Scored struct {
	Score, Sim int
	Capped     bool
}

// ScoreGroup scores a group of members [MET-006, MET-007].
func ScoreGroup(q Query, members []Member) Scored

// ClassifyMatch applies the table of match-rules.md §3 to the best and the second best group [MET-009].
func ClassifyMatch(first, second *Scored) Confidence
```

- `MatchThreshold` e `MatchScore` de `match.go` ficam como estão (o `scrape.search` obsoleto os usa).
- `Dice` sobre runas já existe em `TitleSimilarity`; `ScoreGroup` reaproveita o núcleo sobre as **chaves** (não chama
  `TitleSimilarity`, que normaliza de novo sem remover artigos). O núcleo vira função interna `diceKeys(a, b string) float64`.
- Os valores internos são milésimos inteiros (`int(math.Round(x*1000))`) para a comparação de limiares ficar exata.
- Dois `NormalizeTitle` convivem: `domain.NormalizeTitle` (base da chave) e `importers.NormalizeTitle` (título proposto de um
  arquivo, fase 11). O lote busca pelo título **gravado**, que já passou pelo segundo e pela revisão do usuário.

## 4. Scrapers [MET-001, MET-003, MET-005, MET-010..MET-015, MET-020, MET-021]

### 4.1 Tipos

```go
// internal/scrapers/provider.go
type Query struct {
	Title    string // already the search term (domain.SearchTerm)
	Platform string // library platform name, "" when unknown
	Year     int
	SeedKey  string // seed key of the platform, "" when it has none
	Category string // "pc" | "console" | ..., "" when unknown
}

type Candidate struct {
	// ... campos atuais ...
	PlatformKeys      []string      // seed keys the provider lists for the game; nil = no information
	PlatformConfirmed bool          // false only for IGDB candidates of the unfiltered second query
	ThumbURL          string        // small cover on an allowed host, "" when none [MET-068]
	Meta              *Metadata     // text the search already returned (IGDB), nil when it did not
	Images            []RemoteMedia // images the search already returned (IGDB), nil when it did not
}

// Fetcher is implemented by providers that return text and images in one request [MET-012, MET-014].
// The aggregator uses it when present and falls back to Details + Media otherwise.
type Fetcher interface {
	Fetch(ctx context.Context, providerID string) (Metadata, []RemoteMedia, error)
}
```

`KindBackground` já existe; `Candidate.Platform` (texto) continua para o `scrape.search` obsoleto.

### 4.2 Limitador [MET-010]

```go
// internal/scrapers/limiter.go
// Limiter spaces the requests of one provider: each Wait returns at least interval after the previous slot [MET-010].
// The next free slot is a single time guarded by a mutex (no goroutine, no queue of waiters): Wait reserves the slot and
// then waits on clock.After until it, or returns ctx.Err() when the context ends first (the reservation stays spent).
type Limiter struct{ /* mu, next time.Time, interval, clock */ }
func NewLimiter(clock domain.Clock, interval time.Duration) *Limiter
func (l *Limiter) Wait(ctx context.Context) error
```

Cada configuração de provedor ganha `Limiter *Limiter` (nulo = sem espera, como nos testes de provedor); o provedor chama `Wait`
logo antes de cada requisição de API (não antes do token OAuth do IGDB, que é a Twitch). O agente cria os três limitadores
uma vez (na montagem do serviço) e os passa à fábrica, para que busca única, lote e aplicação compartilhem o mesmo ritmo.
Uma busca única durante um lote espera no máximo os 3 trabalhadores à frente dela (≈ 1 s no IGDB, ≈ 6 s na Steam Store).

### 4.3 Aplicabilidade e mapa de plataformas [MET-003]

```go
// internal/scrapers/applicability.go
type PlatformInfo struct{ SeedKey, Category string; Known bool } // Known = the platform was found
func Applicable(provider string, p PlatformInfo) bool            // table of match-rules.md §1
```

`platforms.go` ganha `SeedKeysForIGDB(ids []int64) []string`, o inverso de `igdbPlatformIDs` (ids fora da tabela são
ignorados). Os ids por provedor continuam sendo do seed (`contracts/seed-platforms.md` da fase 9); SteamGridDB e Steam Store não
têm id de plataforma.

### 4.4 Provedores

- **IGDB** [MET-003, MET-012]: `Search` monta
  `search "<termo>"; fields name,first_release_date,platforms.id,summary,genres.name,game_modes.name,involved_companies.developer,involved_companies.publisher,involved_companies.company.name,cover.image_id,screenshots.image_id,artworks.image_id; where <filtro>; limit 10;`.
  O filtro é `platforms = (<id>)` quando `SeedKey` tem id; se a resposta vem vazia, repete sem filtro e marca
  `PlatformConfirmed = false`. O filtro de jogo principal (`game_type`/`category`, conforme a API atual) entra no `where`; o
  nome exato do campo se **confere na API real na tarefa**, como os ids de plataforma foram conferidos na T02 da fase 9.
  `Meta` e `Images` saem da própria resposta: capa → `cover`, `t_cover_big`; screenshots → `t_screenshot_big`; artes →
  `background`, `t_1080p`. `ThumbURL` = `t_cover_small`. `Details`/`Media` continuam (uso do `scrape.*` obsoleto) e `Fetch`
  devolve `Meta` e `Images` do candidato quando o chamador os tem, senão faz uma requisição por id com os mesmos campos.
- **SteamGridDB** [MET-013]: `Media` faz `grids/game/<id>` (parâmetros de dimensões de retrato e `types=static`, a conferir na
  API real) → `cover`, e `heroes/game/<id>` → `background`; cada uma passa pelo limitador. `Search` não muda. `Details` vazio.
- **Steam Store** [MET-014]: `Fetch` faz **um** `appdetails` e devolve texto e screenshots; `Details` e `Media` o chamam e
  descartam a outra metade (compatibilidade). `Search` preenche `PlatformKeys = ["windows"]` e `ThumbURL` =
  `https://cdn.cloudflare.steamstatic.com/steam/apps/<appid>/capsule_231x87.jpg`.

### 4.5 Agregador [MET-001, MET-005, MET-015]

```go
// internal/scrapers/aggregate.go
type Aggregator struct {
	Providers func(name string) (MetadataProvider, error) // the agent's factory; ErrNoKey = skip
	Clock     domain.Clock
}

type SearchOptions struct {
	Staged  bool          // batch mode: ask the Steam Store only when the others did not find a "match" [MET-015]
	Timeout time.Duration // 20 s (single) or 15 s (one game of the batch)
}

type Group struct {
	Members []domain.Member
	Refs    []Ref // one per provider
	Score   domain.Scored
	// Title, Year, Platform and ThumbURL of the first member; Meta/Images kept per provider for apply
}

type Outcome struct {
	Groups     []Group         // best first
	Confidence domain.Confidence
	Warnings   []string        // providers that failed temporarily
	Missing    []string        // providers skipped for lack of a key (not an error by themselves)
	Failed     []string        // providers that failed temporarily, in order
	Asked      []string        // providers actually asked (an empty list + Missing = ErrNoKey; Asked all failed = ErrUnavailable)
}

func (a *Aggregator) Search(ctx context.Context, q Query, opt SearchOptions) (Outcome, error)
```

Passos: (1) lista os provedores aplicáveis (`Applicable`) e obtém cada instância; `ErrNoKey` → `Missing`; (2) uma goroutine por
provedor, todas juntas em um `sync.WaitGroup` dentro da chamada (nenhuma sobrevive a ela), com `ctx` limitado pelo prazo
(`withClockTimeout`: um `context.WithCancel` cancelado por `Clock.After`); erro `ErrUnavailable` → `Failed`; outro erro →
propaga como erro interno; (3) no modo escalonado, a Steam Store só roda depois das outras e só se não houver `match`; (4) agrupa
(`SameGame`, ordem igdb → steamstore → steamgriddb), pontua (`ScoreGroup`), ordena, classifica e corta em 10 itens ≥ 0,40.
Sem provedor aplicável com chave: `ErrNoKey` (o agente devolve -32010); todos os perguntados falharam: `ErrUnavailable` (-32011).

### 4.6 Um provedor novo (ScreenScraper e outros)

Para acrescentar um provedor basta: implementar `MetadataProvider` (e `Fetcher` se couber), acrescentar o nome ao enum de
`MetadataRef.provider` (contrato), uma linha em `Applicable`, uma em `providerPriority` de `images.go` e na ordem de
junção de `aggregate.go`, e o id de plataforma dele em `platforms.go`. Nada de agente, IPC ou UI muda.

### 4.7 Seleção de imagens [MET-020, MET-021]

```go
// internal/scrapers/images.go
type Offer struct {
	Provider string
	Media    []RemoteMedia // kinds cover, screenshot, background; others are ignored [MET-025]
}

// SelectImages picks the images to queue for one game: per kind, the vacancies are
// MediaKindLimit(kind) - have[kind] - active[kind]; providers are visited in the order of match-rules.md §4.2 for the
// platform class, each in its own order, repeated URLs dropped. It also returns how many offered images were left out.
func SelectImages(offers []Offer, have, active map[string]int, windows bool) (picked []Pick, skipped int)
type Pick struct{ Provider, Kind, URL string }
```

Pura (sem I/O); a tabela de prioridade é `var providerPriority`.

## 5. Agente [MET-001, MET-002, MET-021..MET-024, MET-030..MET-033, MET-050..MET-059, MET-064..MET-066]

### 5.1 `queueImages` com limite e trava por jogo

```go
// internal/agent/scrape.go
type queueResult struct{ queued, skipped int }

// queueImages queues, within the per-kind limits, the images the providers offered for a game. The count of images and
// active jobs, SelectImages and the EnqueueMedia calls run under the game's lock, so two requests for the same game cannot
// both see the same vacancy [MET-023].
func (s *scrapeService) queueImages(ctx context.Context, gameID int64, offers []scrapers.Offer, kinds map[string]bool, at time.Time) (queueResult, error)
```

- `gameLocks` é um mapa `map[int64]*refMutex` protegido por um mutex (entrada criada e removida por contagem de referências;
  nenhuma goroutine). Jogos diferentes não se bloqueiam.
- Lê `ListMedia` (contagem por tipo) e `ActiveMediaJobCounts` (jobs `pending`/`running` por tipo) do store; o resto é
  `SelectImages`. `scrape.apply` e `media.queue` chamam a mesma função (um `Offer` com o provedor); `kinds` (de `media.queue`)
  restringe os tipos. `scrapeStore` ganha `ActiveMediaJobCounts`.
- `ipc.ScrapeApplyResult.Queued` continua o número enfileirado (sem campo novo: o `scrape.*` é obsoleto).

### 5.2 `metadata.search` e `metadata.apply`

```go
// internal/agent/metadata.go
type metadataService struct {
	scrape *scrapeService      // game(), providerError(), queueImages(), st
	agg    *scrapers.Aggregator
	st     metadataStore       // ResolvePlatform, GameDetail, ApplyMetadata, ListMedia, ActiveMediaJobCounts, EnqueueMedia
	clock  domain.Clock
	log    *slog.Logger
	batch  *batchJob           // nil = no batch; guarded by mu
	mu     sync.Mutex
	wg     sync.WaitGroup      // every goroutine of every job; Wait() at shutdown
	root   context.Context     // the agent's context: jobs are children of it, never of a request
}
```

- `search`: valida (MET-071), `domain.SearchTerm`, resolve a plataforma com `ResolvePlatform` (sem criar), monta
  `scrapers.Query`, chama o agregador com prazo de 20 s, converte `Outcome` em `ipc.MetadataSearchResult` (corte em 10, nome da
  plataforma, `thumbUrl` só dos hosts permitidos) e mapeia erros como `providerError` já faz (`ErrNoKey` → -32010,
  `ErrUnavailable` → -32011, com `data.provider`).
- `apply`: valida `refs`; `GameDetail`; para cada ref obtém texto e imagens com `Fetcher.Fetch` (ou `Details` + `Media`): o IGDB
  em 1 requisição por id, a Steam Store em 1, o SteamGridDB em 2. A UI manda só `refs`, e o agente não guarda candidatos entre
  chamadas, então o IGDB é consultado de novo por id. **Todas** as obtenções antes de qualquer escrita (MET-032); depois `Merge`,
  `ApplyMetadata` e `queueImages`. O resultado leva o `GameDetail` novo, `queued` e `skippedByLimit`.
  - Na busca **em lote** o IGDB já devolveu tudo na própria busca e o job guarda isso em `batchRow.groups`, então a aplicação do
    lote não faz requisição ao IGDB (§5.3, MET-012).

### 5.3 Busca em lote

```go
// internal/agent/metadata_batch.go
type batchJob struct {
	mu       sync.Mutex
	id       int64
	state    string
	total    int
	done     int
	failed   int
	warnings []string
	games    []int64              // the ids, in order
	rows     []*batchRow          // same index as games; nil until that game was searched
	choices  []batchChoice
	result   *ipc.BatchResult
	cancel   context.CancelFunc   // cancels the job's context (child of root)
	events   chan batchEvent      // workers -> coordinator, buffered
}

type batchRow struct {
	gameID     int64
	title      string
	platform   string
	confidence domain.Confidence
	selected   int
	groups     []scrapers.Group   // up to 5, with Refs and the Meta/Images each provider returned
}
```

- **Início** (`batchStart`): valida, lê `GameDetail` de cada id (≤ 1.000 leituras locais) para a plataforma e o `-32010`, cria o
  job com `id` = `Clock.Now().UnixMilli()` (se igual ou menor que o último, último + 1), instancia **uma vez** os provedores com
  chave (o IGDB guarda o token) e inicia o **coordenador** (`wg.Add(1); go coordinator()`); responde na hora.
- **Coordenador:** alimenta um canal de índices, inicia 3 trabalhadores (`wg.Add(3)`), recebe `batchEvent` (jogo terminado,
  falha), atualiza os contadores sob `job.mu` e é o **único** que fala com o agrupador de notificações (§5.4); quando todos os
  índices voltaram e o job não foi cancelado, passa a `ready` e notifica na hora.
- **Trabalhador (busca):** pega o índice, lê título/plataforma/ano do jogo, monta a `Query` (`SearchTerm`, chave da plataforma
  por `ResolvePlatform`), chama `Aggregator.Search` com `Staged: true` e prazo de 15 s, monta a `batchRow` (até 5 grupos, o
  primeiro selecionado salvo `none`) e a grava sob `job.mu` **só se** o estado ainda é `searching`. Jogo apagado
  (`ErrGameNotFound`) → linha `none` sem alternativas.
- **Falhas de provedor** (MET-011): um `providerHealth` por job (mapa por provedor: falhas seguidas, suspenso até) consultado
  antes de cada pergunta; a suspensão é um instante do `domain.Clock`, sem goroutine. Provedor desativado entra em
  `warnings` uma vez.
- **Aplicação** (`batchApply`): valida (índices, ids), guarda as escolhas, passa a `applying` e inicia um coordenador com 1
  trabalhador. Para cada escolha: pega o grupo da linha, obtém o que falta de cada provedor (`Meta`/`Images` do grupo quando o
  IGDB já trouxe; senão `Fetch`), `Merge` + `ApplyMetadata` (`replace=false`), `queueImages`; erro → `failed`+1 e título em
  `failedTitles`. Ao fim monta o `result` e passa a `done`.
- **Cancelamento:** `cancel()` do contexto do job; os trabalhadores conferem `ctx.Err()` antes de cada jogo e a escrita da
  linha confere o estado sob `job.mu`. A espera do limitador respeita `ctx`.
- **Memória:** 1.000 linhas × até 5 grupos com `Meta` e até ~25 URLs: da ordem de 20 MB; o teto de 1.000 jogos é o limite.

### 5.4 Notificações do lote [MET-054]

```go
// internal/agent/metadata_notify.go
type batchSink interface{ BatchChanged(p ipc.MetadataBatchChangedParams) } // implemented by *batchNotifier (holds the *ipc.Server)

// progressCoalescer decides when a counter-only change may be sent: the first goes out now and opens a window of
// progressWindow; changes inside the window fold into one sent when it ends. A state change is always sent now.
// Not safe for concurrent use: only the job's coordinator calls it. It owns no goroutine or clock.
type progressCoalescer struct { /* window, until, pending */ }
func (c *progressCoalescer) counters(now time.Time) (sendNow bool)
func (c *progressCoalescer) state(now time.Time)
func (c *progressCoalescer) due(now time.Time) bool
func (c *progressCoalescer) next() (time.Time, bool)
```

É a mesma ideia de `mediaChangeCoalescer` (IMG-003), para uma só chave (o job). O coordenador usa `select` em
`events`, `ctx.Done()` e um canal de `Clock.After` armado só quando há pendência (sem pendência, nenhum temporizador). O
`batchNotifier` é irmão de `mediaNotifier` (um `atomic.Pointer[ipc.Server]`), ligado em `app.go` ao lado dele e zerado no
desligamento.

### 5.5 Goroutines, handles e desligamento [MET-059]

| Goroutine | Dono | Termina quando |
|---|---|---|
| coordenador do job | `metadataService` (`wg`) | todos os índices voltaram, ou o contexto do job (filho do contexto do agente) acaba |
| 3 trabalhadores da busca / 1 da aplicação | o coordenador (`sync.WaitGroup` próprio) | o canal de índices fecha ou o contexto acaba |
| uma por provedor, dentro de `Aggregator.Search` | a própria chamada (`WaitGroup`) | a chamada devolve; nenhuma sobrevive a ela |

O desligamento do agente cancela o contexto raiz e chama `metadataService.Wait()` depois da fila de mídia e antes de fechar o
banco (a ordem exata de `app.go` é conferida na tarefa; os testes de ordem existentes, `order_test.go`, ganham o caso). Nenhum
`windows.Handle` é aberto; nada de Win32. O `http.Client` é o já injetado.

## 6. IPC e binding [MET-070..MET-072]

Ver [contracts/ipc-v0.14.0-delta.md](contracts/ipc-v0.14.0-delta.md). `internal/ipc/metadata.go` traz tipos e constantes; o
`contract_test.go` aponta para o JSON v0.14.0 e confere nomes de campo (`assertExactFields`), enums, os três códigos de erro,
a versão, `x-metadata` contra as constantes de `domain` e do agente (helper exportado ou valor duplicado com comentário, como
na fase 11), e que a v0.14.0 é a v0.13.0 mais acréscimos. `cmd/arcadia/metadata.go` expõe os bindings;
`metadata_events.go` valida o payload do evento (`decodeBatchChanged`, no molde de `decodeMediaChanged`) e `supervisor.go`
repassa. Os arquivos `frontend/wailsjs/go/**` são regerados pelo `wails build`.

## 7. Frontend [MET-041..MET-069]

### 7.1 `lib/metadata.ts` e `lib/metadataBatch.ts`

```ts
// lib/metadata.ts
export type MetadataRefView = { provider: ProviderId; providerId: string }
export type MetadataCandidateView = { refs: MetadataRefView[]; title: string; year: number; platform: string; score: number; thumbUrl?: string }
export type Confidence = 'match' | 'doubt' | 'none'
export function metadataSearch(title: string, platform: string, year: number): Promise<{ candidates: MetadataCandidateView[]; confidence: Confidence; warnings: string[] }>
export function metadataApply(gameId: number, refs: MetadataRefView[], replace: boolean): Promise<MetadataApplyView>

// lib/metadataBatch.ts
export type BatchState = 'searching' | 'ready' | 'cancelled' | 'applying' | 'done'
export type BatchStatusView = { batchId: number; state: BatchState; total: number; done: number; failed: number; warnings: string[]; result?: BatchResultView }
export function useMetadataBatch(): { status: BatchStatusView | null; interrupted: boolean; start(ids: number[]): Promise<void>; cancel(): Promise<void>; discard(): Promise<void> }
export async function loadBatchRows(batchId: number): Promise<BatchRowView[]>   // pages of 100 until next == null
```

`useMetadataBatch` tem **uma** assinatura de `metadata:batch` (cancelada ao desmontar, como IMG-007), lê
`MetadataBatchStatus` no início, ao reconectar e a cada mudança de `state`, e atualiza só os contadores com o payload do evento.
`interrupted` fica verdadeiro quando o hook tinha um estado e `MetadataBatchStatus` volta `batch: null` (MET-067). Ao ir para
`done`, invalida `['library','games']` e o catálogo (leituras).

`acceptedScore` (0,8) e `scrapeSearch`/`scrapeApply` deixam de ser usados pela UI; a tarefa os remove com os testes que só os
cobriam (o `scrape.*` continua no contrato, mas sem consumidor).

### 7.2 Componentes

- **`MetadataOptions`** (apresentacional): lista de rádios para a lista única. Props: `candidates`, `confidence`, `warnings`,
  `selected`, `onSelect`, `renderAction?` (o botão "Aplicar" do diálogo do jogo). Cada item: `<img>` pequena com fallback
  (`CoverImage` existente), título, ano, plataforma, provedores, `%` (`scoreLabel`) e o selo "provável" (MET-044, MET-068).
- **`MetadataDialog`**: `Modal size="medium"`; campo "Nome para busca", botão "Obter dados", caixa "Substituir os campos já
  preenchidos", `MetadataOptions` com "Aplicar"; o resultado mostra `queued` e `skippedByLimit` (MET-024).
- **`ImportDialog`**: (a) fonte `folder`: caixa "Buscar metadados e imagens" (estado `searchMeta`, padrão `true`; desativada pelas
  regras de MET-050 com `ProviderKeyStatus` e o estado do lote vindo de `useMetadataBatch`); depois de `applyImportInLots`,
  se `searchMeta` e há gravados, `onImported()` e em seguida `batch.start(ids)`; (b) fonte `file`: abaixo da tabela, "Nome para
  busca" (`candidate.searchName`), "Obter dados" e `MetadataOptions` com a opção "Nenhuma"; a escolha vira `refs`, guardada
  junto da linha; depois do `ImportApply`, `metadataApply(gameId, refs, false)` (MET-042). Todo `resetList()` (IMV-049) descarta
  também a busca e a escolha.
- **`MetadataReviewDialog`**: `Modal size="large"`; carrega `loadBatchRows`, monta `ReviewRow = { row, choice, checked }` com a
  marcação padrão de MET-061, tabela com `React.memo` por linha e callbacks estáveis (como `ImportReviewTable`); "Aplicar
  selecionados" → `MetadataBatchApply` com `{gameId, candidate: choice}` das marcadas, e fecha; "Descartar tudo" →
  `MetadataBatchDiscard`; Esc fecha sem descartar.
- **`MetadataBatchChip`**: no cabeçalho; mostra o estado, os contadores e os botões da tabela de MET-067; `onOpenReview` abre a
  janela. `App.tsx` guarda `reviewOpen` e aplica a regra de abertura automática (MET-067) comparando o estado anterior e o novo
  e consultando se algum outro modal está aberto (o `App` já controla os modais: `importOpen`, `metadataGameId` etc.).

Cores só por tokens `--arc-*` (UIX-006); a varredura de `theme-scan.test.ts` ganha os `.tsx` novos. Nenhuma interação chama
lançamento, pausa ou sessão (MET-080).

## 8. Alternativas consideradas

| Decisão | Escolhida | Alternativa | Por que não |
|---|---|---|---|
| Onde roda a busca em lote | Job do agente em segundo plano, um por vez, em memória | A UI chama `metadata.search` uma vez por jogo | A UI teria de ficar aberta e controlar 1.000 chamadas; o limite de requisições e o cancelamento ficariam espalhados. |
| Como a UI acompanha | Notificação só com contadores + `batchStatus` | Polling de `batchResults` | Polling gasta chamadas e não diz quando mudou de estado (ADR 0031, mesmo argumento). |
| Resultados no fio | Páginas de até 900 KiB sob demanda | Levar tudo no `batchStatus` ou numa notificação | 1.000 linhas × 5 alternativas passam de 1 MiB; a notificação não leva dados (ADR 0031). |
| Aplicar a confirmação | Assíncrono (`accepted` na hora) | `batchApply` síncrono | Cada jogo custa até 3 requisições com intervalo; 1.000 jogos passariam do prazo de 60 s do desktop. |
| Escolha na confirmação | Índice na lista da linha | A UI manda as `refs` | O agente já guarda os grupos (com o texto do IGDB); reenviar `refs` obrigaria a nova consulta e abre porta a ids arbitrários. |
| Persistência do lote | Memória | Tabela nova | Migração e limpeza por um resultado de minutos; os jogos já estão gravados (decisão 7). |
| Limitador | Um por provedor, por reserva de horário | Fila de espera (canal) por provedor | O horário reservado não tem goroutine nem fila para vazar, e cancela sem estado. |
| Busca em lote com todos os provedores | Steam Store escalonada | Todos sempre | A Steam Store é o provedor mais lento (2 s) e o IGDB já acha a maioria dos jogos de PC. |
| Identificar o mesmo jogo | Chave + Dice ≥ 0,90 + marcas + ano | Só Dice | Remake e sequência juntariam por engano (MET-005, MET-007). |
| Provedor novo (ScreenScraper) | Fora; porta aberta (§4.6) | Incluir agora | Credenciais de desenvolvedor, limite por conta e busca por hash (abrir ROM). |
| Remover `scrape.*` do contrato | Manter, obsoleto | Remover na v0.14.0 | Fase só de acréscimos; a remoção é uma versão maior à parte. |

## 9. Testes e riscos

- **Go (`internal/domain`):** `match_unified_test.go` (vetores de `match-rules.md` §2.3, §3 e §2.4, `SearchTerm` §5.1,
  `Marks`, comparação em milésimos).
- **Go (`internal/scrapers`):** `igdb_test.go` (corpo da consulta, filtro e 2ª consulta, mídia embutida), `steamgriddb_test.go`
  (`grids` e `heroes`), `steamstore_test.go` (uma requisição), `limiter_test.go` (relógio fake), `aggregate_test.go` (junção,
  pontuação, escalonamento, falhas parciais, prazo), `images_test.go` (tabela §4.2), `applicability_test.go`,
  `platforms_test.go` (inverso). `httptest` e relógio fake; sem rede.
- **Go (`internal/importers`):** `search_name_test.go` (tabela §5.2).
- **Go (`internal/store`):** consulta de jobs ativos por tipo (pendente, em andamento, concluído, falho).
- **Go (`internal/agent`):** `metadata_test.go` (busca e aplicação com store real), `metadata_batch_test.go` (cada linha da
  máquina de estados, cancelamento, falhas de provedor, limites, páginas, jogo apagado, sessão ativa intacta, `-race` e
  contagem de goroutines), `metadata_notify_test.go` (agrupador e dois clientes IPC reais), `scrape_limits_test.go`
  (`queueImages` por caminho e concorrência), contrato em `internal/ipc` (MET-070).
- **`cmd/arcadia`:** bindings, evento `metadata:batch` válido e inválidos.
- **Frontend (Vitest):** `metadata.test.ts`, `metadataBatch.test.ts` (uma assinatura, reconexão, interrupção),
  `MetadataOptions.test.tsx`, `MetadataDialog.test.tsx` (reescrito), `ImportDialog.test.tsx` (caixa do lote, painel do arquivo,
  ordem das chamadas), `MetadataReviewDialog.test.tsx`, `MetadataBatchChip.test.tsx`, `App.test.tsx` (abertura automática),
  `App.invariants.test.tsx` (MET-080), `theme-scan.test.ts`.
- **Riscos:**
  1. *Detalhes da API do IGDB e do SteamGridDB* (nome do filtro de jogo principal, parâmetros de dimensão e tipo, `artworks`):
     escritos de memória da documentação; **conferir na API real** na tarefa de cada provedor, antes dos testes (como foi feito
     com os ids de plataforma, fase 9 T02). Se algum não existir, a spec vale pelo comportamento, não pelo parâmetro.
  2. *Intervalos de requisição* são estimativas conservadoras; são constantes em um lugar só (`x-metadata`) e o 429 já vira
     suspensão (MET-011).
  3. *Falso "provável":* títulos curtos e sequências. Mitigação: `sim` mínima, vantagem, trava de marcas, janela de confirmação
     e linhas "em dúvida" desmarcadas. A conferência manual (MET-083) mede; os limiares são constantes do domínio.
  4. *Memória do lote* (~20 MB para 1.000 jogos): aceitável; o teto de 1.000 jogos e o descarte limitam.
  5. *Imagem pequena de host de terceiro* no navegador embutido: privacidade e falta de rede; cai no placeholder; hosts
     permitidos fixos. Alternativa futura: o agente baixar a miniatura para um cache temporário.
  6. *Jogo apagado ou plataforma renomeada no meio do lote:* linha `none` ou falha contada; nada corrompe (cada escrita é uma
     transação do store).
  7. *Notificação perdida* (fila de 16 cheia): `batchStatus` na reconexão e a cada mudança de estado; o pior caso é um contador
     atrasado.
  8. *Abertura automática da janela com outro modal em foco:* só abre sem outro modal; o indicador cobre o resto (MET-067).
  9. *Reinício do agente com busca em curso:* o lote se perde e a UI avisa; os jogos já estão na biblioteca.
  10. *Ordem de desligamento:* o job escreve no banco (aplicação) e usa a fila de mídia; precisa parar antes de fechar ambos
      (§5.5).
  11. *Dois `NormalizeTitle`* com papéis diferentes podem confundir quem lê; os comentários de `match_unified.go` e de
      `importers/title.go` apontam um para o outro.

## 10. ADRs propostos

- **0033 — Busca unificada de metadados com agregador por grupos, pontuação com travas e confirmação em lote** (ver
  requirements.md, "ADRs propostos").
- **0034 — Limite de imagens por tipo contando as atuais e as enfileiradas, aplicado em todo caminho da fila.**
