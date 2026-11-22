# Fase 15 — Big Box v2: design

Requisitos em [requirements.md](requirements.md) (aprovados em 2026-11-22; reserva do IGDB incluída na aprovação, §3.2.1 e §3.4). Cada seção cita os IDs que atende. ADRs: 0014, 0018, 0023, 0024, 0029, 0031,
0033, 0035, 0036; propostos 0038 (Big Box por pedido externo), 0039 (tempo para zerar), 0040 (contrato de tema v2).

## 1. Mapa da mudança

| Pacote | O que muda | Quem escreve |
|---|---|---|
| `internal/input` | `ButtonA`, `ActionBigBox`, terceiro `comboDetector` (`Back + A`) | implementer |
| `internal/agent` | roteador: ação `bigbox` (janela própria, `Current()`, abridor); `openBigBox`; serviço `timeToBeat` (fila, trabalhador, recuo, cache, handlers `timeToBeat.*`, notificação); gancho depois de aplicar metadados; gancho na remoção de jogo | implementer |
| `internal/scrapers` | cliente `HLTB` (token em memória, busca no caminho fixo, leitura), limitador próprio; `IGDB.TimeToBeat` (`game_time_to_beats`, reserva) | implementer |
| `internal/domain` | funções puras do tempo para zerar (estado, validade, recuo) | implementer |
| `internal/store` | migração 10 (`time_to_beat`), consultas, `SchemaVersion = 10` | implementer |
| `internal/ipc` | tipos e constantes v0.16.0 | implementer |
| `internal/tracker` | **nada** (o roteador só usa `Current()`, que já existe) | — |
| `cmd/arcadia` | `--bigbox` na primeira e na segunda instância; `TakeBigBoxRequest`; evento `ui:bigBoxRequested`; bindings `TimeToBeat*`; repasse de `timeToBeat.changed` | implementer |
| `frontend` | `lib/timeToBeat.ts` (leitura), `lib/timeToBeatEdit.ts` (escrita); painel; "Editar jogo"; janela dos provedores; `App.tsx` (pedido de Big Box, dica, seletor pelo registro); `bigbox/` (contrato v2, grade, temas Mosaico e Estante, `FocusVideo`, `TimeToBeatList`); `index.css` (tokens) | implementer |

Nenhuma mudança no rastreamento, em `play_session`, `pause_interval`, `tracking_rule`, `media` ou `media_job`.

## 2. Select + A e `--bigbox` [ATB-001..ATB-016]

### 2.1 Entrada [ATB-001, ATB-002]

```go
// internal/input
const ButtonA uint16 = 0x1000
const (
    ActionPause Action = iota
    ActionClose
    ActionBigBox // new
)
type PadConfig struct {
    // ... existing ...
    BigBoxMask uint16 // default Back+A
}
var defaultBigBoxMask = ButtonBack | ButtonA
```

O `pad` ganha `bdet comboDetector{Hold: cfg.Hold, Mask: cfg.BigBoxMask}`, observado em `read` junto com `det` e `cdet` (inclusive no
caminho de erro, que zera a contagem) e emitindo `Trigger{Source: domain.PauseGamepad, Action: ActionBigBox, At: now}` pela mesma fila
(`triggerQueueSize = 8`, cheia descarta com `Warn`). Os três detectores são independentes.

### 2.2 Roteador [ATB-003..ATB-007, ATB-015]

```go
// internal/agent
type routerTracker interface {
    TogglePause(ctx context.Context, reason domain.PauseReason, at time.Time) (tracker.PauseResult, error)
    CloseByHotkey(ctx context.Context, at time.Time) (tracker.CloseResult, error)
    Current() (domain.SessionSnapshot, bool) // already in *tracker.Tracker
}
// bigBoxOpener starts the UI in the Big Box (exe --bigbox --data-dir root).
type bigBoxOpener interface{ OpenBigBox() error }

func newTriggerRouter(tr routerTracker, in <-chan input.Trigger, log *slog.Logger, fb feedbackSender, sink closeSink,
    opener bigBoxOpener, shuttingDown func() bool) *triggerRouter

// internal/agent/openui.go
// openBigBox spawns exe with --bigbox --data-dir root and DETACHED_PROCESS|CREATE_NEW_PROCESS_GROUP [ATB-004].
func openBigBox(exe, root string, spawn spawnFunc) error
type uiOpener struct{ exe, root string; spawn spawnFunc }
func (o uiOpener) OpenBigBox() error { return openBigBox(o.exe, o.root, o.spawn) }
```

Tabela de decisão (goroutine do roteador; `haveBigBox`, `lastBigBox` são dela):

| Gatilho `bigbox` em `t` | Condição | Efeito |
|---|---|---|
| — | `|t − lastBigBox| < 300 ms` (`domain.BigBoxDebounce`) | descarta, sem som |
| — | `shuttingDown()` ou `ctx` do roteador cancelado | `Debug` "agente encerrando; gatilho do Big Box descartado" |
| — | `Current()` devolve sessão (qualquer estado, inclusive em fechamento) | `Debug` "Big Box ignorado: jogo em execução" (`game_id`, `state`); `fb.Send(EventRefused)` |
| — | sem sessão; `OpenBigBox()` ok | `Info` "Big Box pedido pelo controle" |
| — | sem sessão; `OpenBigBox()` erro | `Warn` "falha ao abrir o Big Box" (`path`, `error`); `fb.Send(EventRefused)` |

`shuttingDown` é um `atomic.Bool` do agente ligado em `BeginShutdown` (o mesmo instante em que o tracker passa a recusar). A chamada
a `CreateProcess` é curta e roda na goroutine do roteador; o processo filho não é esperado (nenhum handle fica aberto: `CreateDetached`
já fecha `hProcess` e `hThread`). Como o agente roda elevado (ADR 0023, 0024), o filho herda o token elevado e não mostra o UAC.

### 2.3 Desktop [ATB-008..ATB-012, ATB-014]

```go
// cmd/arcadia
type desktopConfig struct {
    // ... existing ...
    BigBox bool // --bigbox: open in the Big Box [ATB-008]
}
// runUI: fset.BoolVar(&cfg.BigBox, "bigbox", false, "open the UI in the Big Box")

// wantsBigBox parses a second instance's args with a throwaway FlagSet (ContinueOnError, io.Discard,
// unknown flags tolerated) and reports the value of --bigbox [ATB-009].
func wantsBigBox(args []string) bool

type App struct {
    // ... existing ...
    bigBoxReq bool // guarded by mu; set from cfg.BigBox in NewApp and by secondInstance [ATB-012]
}
// TakeBigBoxRequest returns the pending Big Box request once and clears it. It never calls the agent.
func (a *App) TakeBigBoxRequest() bool

func (a *App) onSecondInstance(d options.SecondInstanceData) { a.secondInstance(wantsBigBox(d.Args)) }
// secondInstance: as today (showWindow, Info, Kick); with bigBox also platform.bringToFront() (Debug with the
// result), bigBoxReq = true and platform.emit(wailsCtx, "ui:bigBoxRequested", nil). Before startup (no
// wailsCtx) only the flag is set; the frontend reads it when it mounts.
func (a *App) secondInstance(bigBox bool)
```

O `--data-dir` da segunda instância continua ignorado. `isAgentMode` não muda: `--agent --bigbox` é o agente, que ignora flags que
não conhece (o parse do agente já tolera desconhecidas; conferir na tarefa).

### 2.4 Frontend [ATB-008..ATB-013]

```ts
// frontend/src/lib/bigbox.ts
export async function takeBigBoxRequest(): Promise<boolean> // binding lido na hora da chamada; falha ou ausência → false

// frontend/src/App.tsx
// requestBigBox(): if (bigBox) return; if (anyModalOpen) return; enterBigBoxShell(); void bringToFront()
// useEffect(() => { void takeBigBoxRequest().then((ok) => ok && requestBigBox()) }, [])           // montagem
// EventsOn('ui:bigBoxRequested', () => void takeBigBoxRequest().then((ok) => ok && requestBigBox())) // removido no retorno
```

O `requestBigBox` lê `bigBox` e `anyModalOpen` por ref (o evento chega fora do ciclo de render). O botão "Big Box" ganha
`title="Big Box (F11 ou Select + A no controle)"`.

### 2.5 Riscos

- **Primeiro plano negado:** o processo que pede é o agente (sem foco). A primeira instância chama `ForceForeground`
  (`AttachThreadInput`, CON-023); a UI nova (sem instância anterior) pede `BringToFront` depois de entrar. Limitações 3.
- **Pedido perdido entre o evento e a montagem:** coberto pelo pedido guardado (ATB-012).
- **`SecondInstanceData.Args` no Wails v2 em uso:** conferir na tarefa que os argumentos chegam sem o nome do exe (ou tratar os dois
  casos em `wantsBigBox`).
- **Select + A dentro do Big Box:** a UI ignora A com Select segurado (BBX-057); o gatilho do agente chega à segunda instância e
  ATB-010 não muda nada.

## 3. Tempo para zerar [TTB-001..TTB-032]

### 3.1 Migração 10 [TTB-001, TTB-002]

`internal/store/migrations/00010_time_to_beat.sql` (uma transação do goose):

```sql
-- +goose Up
CREATE TABLE time_to_beat (
  game_id         INTEGER PRIMARY KEY REFERENCES game (id) ON DELETE CASCADE,
  source          TEXT    NOT NULL CHECK (source IN ('hltb', 'igdb')),          -- igdb: reserva (TTB-033)
  state           TEXT    NOT NULL CHECK (state IN ('found', 'doubt', 'not-found')),
  external_id     TEXT    NOT NULL DEFAULT '',
  external_title  TEXT    NOT NULL DEFAULT '',
  main_s          INTEGER NULL CHECK (main_s IS NULL OR main_s > 0),
  main_extra_s    INTEGER NULL CHECK (main_extra_s IS NULL OR main_extra_s > 0),
  completionist_s INTEGER NULL CHECK (completionist_s IS NULL OR completionist_s > 0),
  title_at_fetch  TEXT    NOT NULL,
  term            TEXT    NOT NULL,
  candidates      TEXT    NOT NULL DEFAULT '[]',   -- JSON (rules §5), only for state = 'doubt'
  confirmed       INTEGER NOT NULL DEFAULT 0 CHECK (confirmed IN (0, 1)),
  CHECK (source = 'hltb' OR (state = 'found' AND confirmed = 0)),                -- igdb só como found automático (TTB-002, TTB-040)
  fetched_at      INTEGER NOT NULL,                -- Unix ms UTC (ADR 0009)
  expires_at      INTEGER NOT NULL
);
-- +goose Down
DROP TABLE time_to_beat;
```

`store.SchemaVersion` 9 → 10. Consultas (`internal/store/queries/time_to_beat.sql`, `sqlc generate`): `GetTimeToBeat(gameID)`,
`UpsertTimeToBeat(...)` (`INSERT ... ON CONFLICT (game_id) DO UPDATE`), `GameTitleYear(gameID)` se ainda não houver leitura
equivalente. As duas chaves de configuração usam `store.Setting`/`SetSetting` existentes (`app_setting`).

### 3.2 Cliente do HowLongToBeat [TTB-003, TTB-005, TTB-010..TTB-012, TTB-027, TTB-031]

```go
// internal/scrapers/hltb.go
type HLTBGame struct {
    ID         int64
    Name       string
    Aliases    []string
    Year       int      // 0 = unknown
    Platforms  []string
    Main, MainExtra, Completionist int64 // seconds; 0 = no data
}
type HLTBConfig struct {
    BaseURL string       // default https://howlongtobeat.com
    HTTP    *http.Client // copied; Jar nil, Timeout unset (per-request ctx instead)
    Limiter *Limiter     // 2 s, own instance (newProviderFactory)
    Clock   domain.Clock // the t=<epoch ms> of the token request
}
type HLTB struct { /* cfg, mu sync.Mutex, token string (memory only) */ }
func NewHLTB(cfg HLTBConfig) *HLTB
// Search runs one search on the fixed path /api/search/site with the x-auth-token header, getting a
// token from /api/search/site/init first when none is kept, and renewing it and retrying once on 403
// [TTB-012]. Every temporary failure (404, 429, 5xx, second 403, no token) wraps ErrUnavailable; a ctx
// cancelled by the caller returns ctx.Err() as is. The token never leaves the struct (no log, no error).
func (h *HLTB) Search(ctx context.Context, term string) ([]HLTBGame, error)
```

· alterado em 2026-11-22 (conferência T05): a descoberta do caminho por script saiu (o site usa Turbopack e caminho fixo); entrou o
token de [rules §2](contracts/hltb-rules.md). Comportamento visível igual.

Cada requisição (`init` e busca): `Limiter.Wait(ctx)` → `context.WithTimeout(ctx, 10s)` → `do` com os cabeçalhos de
[rules §3](contracts/hltb-rules.md) → `io.LimitReader(body, 2 MiB + 1)`. O token é lido e trocado sob `h.mu`, mas a requisição não
segura o mutex (o serviço já garante uma requisição por vez com `reqMu`); duas buscas que recebam 403 ao mesmo tempo renovam no
máximo duas vezes, sem dano. Erros de rede sem URL (`netCause`, como os outros provedores). Nada é logado no cliente além de
`Debug` "token do HowLongToBeat renovado" (sem o valor); quem registra `Warn` é o serviço (uma vez por sequência).

### 3.2.1 Reserva do IGDB [TTB-035, TTB-036, TTB-038]

```go
// internal/scrapers/igdb_ttb.go
type IGDBTimeToBeat struct{ Hastily, Normally, Completely int64 } // seconds; 0 = no data
// TimeToBeat reads game_time_to_beats of one IGDB game (rules §10.3), through the IGDB limiter of phase 12
// and with the IGDB token. ErrNoKey without the key; ErrUnavailable for network, 429 and 5xx.
func (c *IGDB) TimeToBeat(ctx context.Context, igdbID string) (IGDBTimeToBeat, bool, error) // false = no record
```

A busca do jogo no IGDB reaproveita `(*IGDB).Search` (MET-003) e as regras de pontuação de [rules §10.2](contracts/hltb-rules.md).

### 3.3 Funções puras [TTB-004, TTB-006, TTB-011]

```go
// internal/domain/timetobeat.go
type TTBState string
const (TTBFound TTBState = "found"; TTBDoubt TTBState = "doubt"; TTBNotFound TTBState = "not-found")
type TTBResult struct { ID int64; Name string; Aliases []string; Year int }
type TTBRanked struct { Index int; Scored Scored }
// RankTimeToBeat scores every result against the game (ScoreGroup, no platform) and classifies the
// best two (ClassifyMatch) [rules §5]. The ranking is sorted by score, then by ID.
func RankTimeToBeat(title string, year int, results []TTBResult) (TTBState, []TTBRanked)
func TTBExpiresAt(source string, state TTBState, fetchedAt time.Time) time.Time // rules §1, §6 (igdb found: 30 d)
// TTBResolve applies the source table of rules §10.1: what to write (or nothing) from the HowLongToBeat
// outcome and the IGDB outcome (IGDBNotAsked, IGDBValues, IGDBNoMatch, IGDBUnavailable, IGDBNoKey).
func TTBResolve(h HLTBOutcome, i IGDBOutcome) (write bool, source string, state TTBState)
func TTBBackoff(failures int) time.Duration                             // 1, 5, 30, 60 min
```

### 3.4 Serviço do agente [TTB-006..TTB-011, TTB-020..TTB-023, TTB-025, TTB-026, TTB-028]

```go
// internal/agent/timetobeat.go
type ttbSearcher interface {
    Search(ctx context.Context, term string) ([]scrapers.HLTBGame, error)
}
// ttbFallback is the IGDB side of the reserve (fake in tests) [TTB-033..TTB-038].
type ttbFallback interface {
    Search(ctx context.Context, q scrapers.Query) ([]scrapers.Candidate, error)
    TimeToBeat(ctx context.Context, igdbID string) (scrapers.IGDBTimeToBeat, bool, error)
}
type ttbNotifier interface{ TimeToBeatChanged(gameID int64) }

type timeToBeatService struct {
    st     *store.Store
    client ttbSearcher
    igdb   ttbFallback
    clock  domain.Clock
    log    *slog.Logger
    note   ttbNotifier

    ctx    context.Context    // child of the agent's; cancelled by Close
    cancel context.CancelFunc
    wg     sync.WaitGroup     // the worker
    wake   chan struct{}      // buffer 1
    reqMu  sync.Mutex         // one HTTP request at a time (worker, Refresh, Search) [TTB-009]

    mu          sync.Mutex    // everything below
    interactive []int64       // FIFO
    batch       []int64       // FIFO, ≤ 1000
    queued      map[int64]bool
    inflight    int64         // 0 = none
    dropped     map[int64]bool // removed while in flight [TTB-026]
    backoff     map[string]*ttbBackoff // "hltb", "igdb": {failures int; pausedUntil time.Time; warned bool} [TTB-011, TTB-038]
    candidates  map[int64][]ipc.TimeToBeatCandidate // last search per game, ≤ 64 games (oldest out)
    order       []int64
    stopped     bool          // set by BeginShutdown: no more notifications [TTB-028]
}

func newTimeToBeatService(st *store.Store, client ttbSearcher, igdb ttbFallback, clock domain.Clock, log *slog.Logger, note ttbNotifier) *timeToBeatService
func (s *timeToBeatService) Start()            // one worker goroutine, owned by wg
func (s *timeToBeatService) BeginShutdown()    // stopped = true; queues emptied
func (s *timeToBeatService) Close()            // cancel + wg.Wait; idempotent

func (s *timeToBeatService) Get(ctx context.Context, gameID int64, fetch bool) (ipc.TimeToBeat, error)
func (s *timeToBeatService) Refresh(ctx context.Context, gameID int64) (ipc.TimeToBeat, error)
func (s *timeToBeatService) Search(ctx context.Context, gameID int64, term string) ([]ipc.TimeToBeatCandidate, error)
func (s *timeToBeatService) Choose(ctx context.Context, gameID int64, externalID string) (ipc.TimeToBeat, error)
func (s *timeToBeatService) Settings(ctx context.Context) (ipc.TimeToBeatSettings, error)
func (s *timeToBeatService) SetSettings(ctx context.Context, p ipc.TimeToBeatSetSettingsParams) (ipc.TimeToBeatSettings, error)
func (s *timeToBeatService) AfterMetadata(gameID int64) // TTB-025: checks settings and confirmed, queues as batch
func (s *timeToBeatService) GameRemoved(gameID int64)   // TTB-026
```

- **Trabalhador** (uma goroutine): laço `select` em `s.ctx.Done()`, `s.wake` e, em recuo, `s.clock.After(pausedUntil − now)`. A cada
  volta, com `enabled` ligado e fora do recuo, tira o próximo jogo (interativo antes do lote), marca `inflight`, faz `lookup` sob
  `reqMu` com `s.ctx`, grava (`UpsertTimeToBeat`), limpa `inflight` e notifica — a menos que o jogo esteja em `dropped` ou `stopped`.
  Sai quando `s.ctx` acaba. Sem `time.Sleep`.
- **`lookup(ctx, game, ignoreBackoff)`:** termo (`domain.SearchTerm`; vazio → `not-found` sem requisição) → HowLongToBeat (se fora do
  recuo ou `ignoreBackoff`): `client.Search` → (sem resultados e termo com ": " → segunda busca com o prefixo) →
  `domain.RankTimeToBeat`. Com `not-found`, falha ou recuo do HowLongToBeat, e linha não confirmada → reserva: `igdb.Search` →
  pontuação de rules §10.2 → `match` → `igdb.TimeToBeat`. `domain.TTBResolve` decide o que gravar (rules §10.1). Linha confirmada:
  busca pelo `external_title` e procura o mesmo id (rules §6), sem reserva. `ErrUnavailable` de uma fonte → `failures++` e
  `pausedUntil` **dela**, `Warn` na primeira; sucesso → zera a escada dela; `ErrNoKey` do IGDB → `Debug`, sem recuo (TTB-037).
  `reqMu` cobre o lookup inteiro (as duas fontes), então nunca há duas requisições ao mesmo tempo.
- **`Get`:** lê a linha e o jogo (título atual), calcula `stale` (validade ou título trocado sem confirmação), `pending`
  (`queued[g] || inflight == g`), `unavailable` (HowLongToBeat em recuo **e** IGDB em recuo ou sem chave; a falta de chave é lida
  do `scrapers.Keys` sem requisição); com `fetch`, ligado, não `unavailable` e (`sem linha` ou `stale`), enfileira como interativo
  (ou promove do lote) e acorda o trabalhador. Jogo inexistente → `-32001`.
- **Trabalhador e recuo:** atende a fila enquanto houver uma fonte disponível; com as duas indisponíveis, espera o menor
  `pausedUntil` pelo `domain.Clock`.
- **`Refresh`/`Search`:** `context.WithTimeout(ctx, 25s)`, `reqMu`; `Refresh` = `lookup(..., ignoreBackoff = true)`; nada gravado →
  `-32011`. `Search` só no HowLongToBeat (ignora o recuo); falha → `-32011`; guarda os candidatos em `candidates` (mapa limitado).
- **`Choose`:** procura o id em `candidates[g]` e, senão, nos candidatos da linha em `doubt`; grava `found`/`not-found` confirmada
  sem requisição; notifica.
- **`SetSettings(enabled = false)`:** grava, esvazia as filas e `queued` (a requisição em curso termina e grava normalmente).
- **Remoção:** o handler de `game.delete` chama `GameRemoved(g)` antes de apagar: tira `g` das filas e de `candidates`; se
  `inflight == g`, põe em `dropped` (o resultado é descartado; a escrita também falharia pela FK).
- **Notificação:** `ttbNotifier` é o mesmo broadcast de `media.changed` (fila por conexão com descarte, IPC-017).
- **Ciclo de vida:** criado em `Run` depois do store e do servidor IPC; `Start` junto da fila de mídia; no shutdown,
  `BeginShutdown` junto do do tracker e `Close` antes de fechar o store.

### 3.5 IPC e gancho de metadados [TTB-020..TTB-025, TTB-029]

- `internal/ipc/timetobeat.go`: `MethodTimeToBeatGet/Refresh/Search/Choose/GetSettings/SetSettings`,
  `MethodTimeToBeatChanged`, tipos de [delta §3](contracts/ipc-v0.16.0-delta.md), `ProviderUnavailable` com `provider: "hltb"` e
  `reason`.
- `metadataService.apply` e a aplicação do lote chamam `ttb.AfterMetadata(gameID)` depois de gravar com sucesso (o mesmo ponto em que
  VID-004 enfileira o vídeo). Nada no resultado muda.

### 3.6 Desktop [TTB-030]

`cmd/arcadia/timetobeat.go`: os seis bindings de [delta §6](contracts/ipc-v0.16.0-delta.md), cada um com o seu prazo; o supervisor
ganha `case ipc.MethodTimeToBeatChanged` → `decodeMediaChanged`-like → `Emit("timeToBeat:changed", {gameId})` (ou `Warn`).

### 3.7 Frontend [TTB-013..TTB-019, TTB-024]

```ts
// frontend/src/lib/timeToBeat.ts (só leitura; o Big Box pode importar)
export type TimeToBeatView = { /* delta §3 */ }
export const TTB_FETCH_DELAY_MS = 1000
export function formatTimeToBeat(seconds: number | null): string            // rules §8
export function timeToBeatRows(v: TimeToBeatView | undefined): TimeToBeatRow[] // 3 linhas; '—' sem dado
export const timeToBeatKey = (id: number) => ['timeToBeat', id] as const
// useTimeToBeat lê o cache na hora (fetch false) e, com fetchAfterMs definido, pede fetch true uma vez depois
// de o mesmo gameId ficar esse tempo montado (0 = na hora); o temporizador é limpo ao trocar de jogo e ao desmontar.
// Qualquer erro vira data undefined (TTB-013); retry: false.
export function useTimeToBeat(gameId: number | null, o: { fetchAfterMs: number | null }): { data?: TimeToBeatView }
export function useTimeToBeatEvents(): void // EventsOn('timeToBeat:changed') → invalidate timeToBeatKey(id)

// frontend/src/lib/timeToBeatEdit.ts (escrita; proibido em bigbox/)
export function refreshTimeToBeat(id: number): Promise<TimeToBeatView>
export function searchTimeToBeat(id: number, term: string): Promise<TimeToBeatCandidateView[]>
export function chooseTimeToBeat(id: number, externalId: string): Promise<TimeToBeatView>
export function useTimeToBeatSettings(): UseQueryResult<TimeToBeatSettingsView>
export function setTimeToBeatSettings(enabled: boolean, afterMetadata: boolean): Promise<TimeToBeatSettingsView>
```

- `useTimeToBeatEvents()` é montado uma vez no `App` (vale para a janela normal e para o Big Box, que fica dentro do `App`).
- Fonte [TTB-039]: `timeToBeatSource(v)` devolve "HowLongToBeat" (`hltb`), "IGDB" (`igdb`) ou `null` sem valor; usada pelo painel,
  pelo Big Box (`model.focused.timeToBeatSource`) e por "Editar jogo" (que também mostra "Encontrado no IGDB (reserva): …").
- Painel: `components/TimeToBeatBlock.tsx` (`useTimeToBeat(id, { fetchAfterMs: TTB_FETCH_DELAY_MS })`), dentro do `<dl>` de
  `GameDetailsPanel` depois de "Tempo total"; o botão "Confirmar…" chama o `onEdit` existente com a seção pedida
  (`onEdit({ section: 'timeToBeat' })`, rola até a seção).
- "Editar jogo": `components/TimeToBeatEditor.tsx` (estado, "Atualizar", "Escolher outro…", lista de candidatos, "Nenhum destes"),
  fora do formulário (vale na hora, como `GameImagesEditor`).
- Provedores: seção nova em `ProviderKeysDialog` com as duas caixas (`useTimeToBeatSettings` + `setTimeToBeatSettings`).

### 3.8 Riscos

- **Endpoint e formato mudam:** o caminho e o passo do token foram conferidos em 2026-11-22 ([rules §9](contracts/hltb-rules.md));
  uma mudança futura vira falha temporária e recuo até 1 h. Pior caso: "—" em toda parte até uma versão nova. O token expirado se
  resolve sozinho (403 → token novo, uma repetição).
- **Bloqueio por excesso de pedidos:** uma requisição por vez, 2 s entre elas, busca na UI só depois de 1 s parado, cache longo
  (inclusive "não encontrado"), recuo. Lote de 1.000 jogos ≈ 35 min, em segundo plano.
- **Termos de uso do site:** registrado em Limitações 4 e no texto da janela dos provedores; o usuário pode desligar.
- **`reqMu` e prazos:** um `refresh` pode esperar a busca em curso do trabalhador (até ~40 s no pior caso: token, busca 403, token novo, busca, cada uma com até 10 s); o prazo
  de 25 s do agente vence antes e responde `-32011`, sem bloquear a goroutine do IPC além disso.
- **Reserva do IGDB:** o IGDB divide o limitador com a busca de metadados (MET-010); um lote grande de metadados e um lote do tempo
  para zerar se atrasam mutuamente, sem passar do limite. O token do IGDB é o mesmo (renovação pelo cliente existente). Uma linha do
  IGDB vale 30 dias para a fonte principal voltar a ser tentada.
- **Correspondência errada com confiança alta:** o usuário corrige em "Escolher outro…", e a escolha confirmada nunca é desfeita
  sozinha.

## 4. Big Box v2 [BBX-046..BBX-059, MOS-*, ETN-*]

Contrato em [contracts/bigbox-temas-v2.md](contracts/bigbox-temas-v2.md). Nada muda no Go para esta seção.

### 4.1 Estrutura

```
frontend/src/bigbox/
  themes/types.ts            BigBoxThemeDef v2, BigBoxThemeProps (+ onVideoError)
  themes/registry.ts         faixa, mosaico, estante, roda; resolveTheme; capacityOf(theme, vp)
  themes/faixa/*             sem mudança visual (âncora 2, tokens novos definidos)
  themes/mosaico/Mosaico.tsx, MosaicoRow.tsx, MosaicoPanel.tsx, capacity.ts (mosaicoCapacity)
  themes/estante/Estante.tsx, EstanteHero.tsx, EstanteHeader.tsx, EstanteGrid.tsx, EstanteDetails.tsx, capacity.ts (estanteRows)
  nav.ts                     NavCtx.columns; up/down em Navegar na grade
  useBigBoxInput.ts          eixo grid; Select bloqueia A e ↑/↓ do direcional
  useBigBoxData.ts           useTabGames(tab, sort, paging); gridPageSize; shouldPrefetchGrid
  viewModel.ts               buildTabs(catalog, initialPlatformId, kind); stripWindow(..., anchor); gridWindow; relativeDays; acréscimos do modelo
  hints.ts                   rótulo de LB/RB ('Plataforma' | 'Aba')
  useVideoDelay.ts           (novo) o temporizador de 2 s tirado de BackgroundStage
  FocusVideo.tsx             (novo) vídeo dentro da capa em foco
  CoverArt.tsx               (novo) capa/substituta/placeholder, tirado de FaixaStrip
  TimeToBeatList.tsx         (novo) três rótulos e valores
  BackgroundStage.tsx        backdrop: 'contain' | 'cover' | 'plain'; vídeo só com videoSlot 'background'
  BigBoxDetails.tsx          + tempo para zerar
  BigBox.tsx                 escolhe janela/pré-busca pelo eixo, Details do tema, fetch do tempo para zerar
```

### 4.2 Shell [BBX-047, BBX-050, BBX-052, BBX-053, BBX-055, BBX-056, BBX-059]

- `theme = resolveTheme(themeId)`; `tabs = buildTabs(catalog, initialPlatformId, theme.tabs)`; `capacity = theme.listCapacity(vp)`;
  na grade, `rows = theme.gridRows(vp)`.
- `useTabGames(tab, theme.sort, paging)` com `paging = axis === 'grid' ? { kind: 'grid', columns, rows } : { kind: 'strip',
  capacity }`; chave `['bigbox', 'games', tab.key, sort]`. O efeito de pré-busca chama `shouldPrefetchGrid` ou `shouldPrefetch`.
- Janela: `axis === 'grid' ? gridWindow(T, f, C, rows) : stripWindow(T, f, V, theme.anchor)`; `useCardImages` recebe os ids dela.
- `ctxRef.current.columns = theme.columns`.
- Fundo: `<BackgroundStage backdrop={theme.backdrop} videoUrl={theme.videoSlot === 'background' ? url : null} .../>`.
- Detalhes: `const Details = theme.Details ?? BigBoxDetails`; recebe também `timeToBeat` e `timeToBeatSource`.
- Tempo para zerar: `useTimeToBeat(focusedId, { fetchAfterMs: nav.suspended ? null : nav.screen === 'details' ? 0 :
  theme.timeToBeatInBrowse ? TTB_FETCH_DELAY_MS : null })` (`null` = só cache) [BBX-059].
- Tema: `onVideoError={(id) => dispatchNav({ type: 'videoFailed', gameId: id })}`.

### 4.3 Vídeo na capa [BBX-053, MOS-007]

```ts
// bigbox/useVideoDelay.ts
export function useVideoDelay(o: { gameId: number | null; videoUrl: string | null; active: boolean; failed: boolean }): boolean
// bigbox/FocusVideo.tsx: { gameId, url, active, sound, failed, onError } → <video> com contain, loop, playsInline,
// playWithSound, src removido ao desmontar; só desenha depois de useVideoDelay
```

`BackgroundStage` passa a usar `useVideoDelay` (comportamento igual, os testes de BBX-011..BBX-013 continuam). O Mosaico põe
`FocusVideo` dentro do cartão em foco com `active = model.screen === 'browse'` (a suspensão já desliga o shell); como o fundo não tem
vídeo, há no máximo um `<video>`.

### 4.4 Entrada [BBX-048, BBX-057]

`mapDirection(axis, dir)` ganha `grid`: `left/right → prev/next`, `up/down → up/down`. No `tick`, com `select` apertado: `aR` não emite
`accept`, e as direções `up`/`down` do **direcional** (botões 12/13) contam como soltas (o analógico continua); a borda que acontecer
com Select apertado não emite depois que ele for solto (o `prevBtn` é atualizado do mesmo jeito que hoje para B e Y).

### 4.5 Mosaico e Estante [MOS-*, ETN-*]

Layouts, fórmulas e tokens em [temas-v2 §5..§7](contracts/bigbox-temas-v2.md). Pontos de implementação:

- **Mosaico:** `Mosaico` desenha abas (`model.tabs`, `model.tab`), contagem (`model.tabTotal`), título (`model.focused.game.title`),
  `MosaicoRow` (janela de `model.games` a partir de `model.stripStart`, `CoverArt`, `FocusVideo` no focado) e `MosaicoPanel` (dados,
  `TimeToBeatList`, miniaturas de `model.focused.screenshots`). Em Detalhes, `details` (comum).
- **Estante:** `Estante` decide `heroVisible = Math.floor(model.focus / model.columns) === 0`; `EstanteHero` (título,
  `lastPlayedRelative`, botões, coração, dois blocos), `EstanteHeader` (título, `sortLabel`, abas com selos), `EstanteGrid` (cartões
  com `CoverArt`, `playTimeLabel`). `EstanteDetails` implementa `BigBoxDetailsProps` com o layout de §6.3 e as bolinhas no lugar das
  miniaturas; reaproveita o `Video` do visor e a rolagem da descrição de `BigBoxDetails` (exportados).
- **Relativo** (`viewModel.relativeDays(iso, now)`): diferença em dias de calendário locais; 0 "hoje", 1 "ontem", 2..29 "há N dias",
  30..364 "há N meses" (`⌊d/30⌋`, "há 1 mês"; 12 vira "há 1 ano"), ≥ 365 "há N anos" (`⌊d/365⌋`, "há 1 ano"); sem data "Nunca
  jogado". O shell passa `now = new Date()` a cada render (testes com `vi.setSystemTime`).

### 4.6 Seletor e preferências [BBX-046]

`lib/uiPrefs.ts`: `BigBoxThemeId` ganha `estante`. `App.tsx`: as opções vêm de `BIGBOX_THEMES` com `t.name` e "(em breve)" quando
indisponível (some o mapeamento fixo de nomes).

### 4.7 Varreduras [BBX-003, BBX-025, BBX-058, BBX-059]

`bigbox-scan.test.ts` ganha: `lib/timeToBeatEdit` e os nomes `TimeToBeat(Refresh|Search|Choose|SetSettings)`,
`refreshTimeToBeat`, `searchTimeToBeat`, `chooseTimeToBeat`, `setTimeToBeatSettings` proibidos; os arquivos novos de `bigbox/` na
lista de UIX-006. O teste de tokens lê os três blocos e confere a lista completa e os pares de §7.

### 4.8 Riscos

- **Mudança do contrato de tema:** todos os campos novos são obrigatórios no registro (o TypeScript pega o esquecimento); o tema falso
  dos testes ganha os campos.
- **Grade e pré-busca:** regra separada da faixa, então os testes de BBX-007/BBX-038 do Faixa não mudam; a grade tem os seus vetores.
- **Abas de coleção:** `GameListResult.total` da primeira página dá `T` para "Favoritos" e "Concluídos" (sem `gameCount` no
  catálogo); uma aba de coleção vazia aparece mesmo assim (BBX-054).
- **Ordem `lastPlayed`:** o foco inicial de BBX-027 continua procurando o jogo da janela normal só na primeira página.
- **Vídeo na capa:** o cartão em foco muda de tamanho (escala 1,09); o `<video>` fica dentro dele com `contain` para não cortar.
- **Fundo `cover` na Estante:** corta a arte nas bordas por escolha (decisão n); os textos ficam sobre painéis (BBX-037).

## 5. Invariantes [ATB-015, TTB-030, CON-024]

- A UI não pausa, não retoma, não fecha jogo e não encerra sessão: Select + A só **lê** a sessão no agente; o tempo para zerar não
  toca em sessão.
- Goroutines novas: só o trabalhador do serviço do tempo para zerar (dono: o serviço; saída: `ctx`; espera: `wg` no `Close`, chamado
  no shutdown do agente antes do store). O roteador e o poller não ganham goroutine. No frontend, só temporizadores e assinaturas
  limpos ao desmontar.
- Nenhum handle novo do Windows além do processo criado por `CreateDetached` (fechado lá dentro).
