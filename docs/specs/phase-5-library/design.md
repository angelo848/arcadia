# Fase 5 — Biblioteca: design

- **Status:** aprovado junto com [requirements.md](requirements.md) (2026-11-12)
- Implementa [requirements.md](requirements.md). Base: designs da [fase 4](../phase-4-pause/design.md), da
  [fase 3](../phase-3-tracker/design.md), da [fase 2](../phase-2-skeleton/design.md) e do
  [executável único](../single-exe/design.md); o que não é citado aqui continua como lá.
- Contratos: [ipc.openrpc.json](contracts/ipc.openrpc.json) (v0.6.0, substitui a v0.5.0 como fonte da verdade).
- ADRs: 0004 (SQLite, goose, sqlc), 0009 (instantes em ms UTC), 0010 (só o agente migra), 0012 (enums no `domain`,
  sem `CHECK`), 0015 (agente único escritor), 0019 (lançamento sem rastrear). Novos, a escrever pela sessão principal:
  0020 (índice de busca e `FilterSpec`), 0021 (remoção de jogo), 0022 (listagem paginada no IPC).
- Os IDs entre colchetes indicam os requisitos atendidos por cada seção.
- **Quem escreve o quê (ADR 0007):** `internal/domain`, `internal/store`, `internal/ipc`, `internal/agent`,
  `cmd/arcadia` e `frontend/` são do implementer. `internal/tracker/` é do tracker-specialist, e só ganha uma função de
  serialização (seção [Remoção](#remoção-do-jogo)).

## Visão geral

```
 UI (Wails, cmd/arcadia)                         agente (arcadia.exe --agent)
 ┌────────────────────────────────┐   IPC JSON-RPC   ┌──────────────────────────────────────────┐
 │ LibraryGrid / LibraryList      │ ───────────────► │ ipc.Server ─► agent/library.go handlers  │
 │ FilterPanel · SearchBox · Sort │  game.list       │   · prepareEdit / validate (domain)      │
 │ GamePage · GameEditForm        │  game.get        │   · store.ListGames / GameDetail /       │
 │ DeleteDialog                   │  game.update     │     UpdateGame / DeleteGame / Sessions  │
 │ lib/library.ts (TanStack Query)│  game.delete     │   · tracker.WithoutActiveGame (remoção)  │
 │ sem caminho de escrita no banco│  session.list    │                                          │
 └────────────────────────────────┘  library.catalog └───────────────┬──────────────────────────┘
                                     platform.*                      │ única conexão de escrita
                                                                     ▼
                                                     SQLite library.db (WAL, foreign_keys = 1)
                                                     game · game_fts (FTS5, rowid = game.id) · vínculos
```

Princípios: (1) toda escrita entra por IPC e é uma transação no agente; (2) a grade, o contador e a busca são consultas
do agente com o mesmo `FilterSpec`; a UI só guarda o estado dos controles; (3) a UI não calcula tempo, nem soma, nem
filtra localmente (o cronômetro da sessão aberta continua sendo o da fase 4, UI-014); (4) nenhuma consulta monta SQL com
texto do usuário.

## Layout de pacotes

```
internal/domain/
  game_status.go       # GameStatus, ParseGameStatus                                          [DOM-012]
  release_date.go      # ReleaseYear, WithYear                                                [DOM-013]
  play_mode.go         # PlayMode, ParsePlayMode, PlayModeGroup, ParsePlayModeGroup, Matches  [DOM-014]
  platform.go          # PlatformCategory, ParsePlatformCategory, NormalizeName              [DOM-015, DOM-016]
  filter.go            # FilterSpec, Validate, Normalize, constantes de limite                 [FLT-001..FLT-008]
  library_sort.go      # Sort, ParseSort                                                      [LIB-001]
internal/store/
  migrations/00005_fts_rebuild.sql                                                            [STO-024]
  fts.go               # reindexGame(tx, id), deleteGameIndex(tx, id), ftsQuery(text)          [FTS-001..FTS-005]
  library.go           # ListGames, GameDetail, UpdateGame, DeleteGame, Catalog, AddPlatform,
                       # GameSessions, buildGameFilter, sortClause                              [STO-025..STO-030, LIB-*, FLT-*]
  games.go             # AddGame passa a chamar reindexGame na mesma transação                 [FTS-002]
  store.go             # SchemaVersion = 5; remove LibrarySummary                              [STO-024, DSK-019]
  queries/library.sql  # consultas de sqlc das partes fixas (detalhe, vínculos, catálogo)
internal/tracker/      # só o tracker-specialist
  tracker.go           # + WithoutActiveGame(ctx, gameID, fn) (goroutine dona)                 [DEL-002, DEL-003]
internal/ipc/
  protocol.go          # + métodos e tipos da v0.6.0                                           [IPC-041..IPC-049]
  contract_test.go     # passa a ler docs/specs/phase-5-library/contracts/ipc.openrpc.json      [IPC-041]
internal/agent/
  library.go           # handlers game.get, game.update, game.delete, session.list,
                       # platform.list, platform.add, library.catalog; game.list com params   [IPC-042..IPC-049]
  games.go             # game.list vira paginado; prepareLaunch/toIPCGame reaproveitados       [IPC-042]
cmd/arcadia/
  library.go           # ListGames, GetGame, GameSessions, UpdateGame, DeleteGame, Catalog,
                       # AddPlatform; remove LibrarySummary                                     [DSK-016..DSK-019]
  errors.go            # mensagem de -32002 para DeleteGame                                     [DSK-018]
frontend/src/
  lib/library.ts       # tipos, filtros padrão, consultas e mutações (TanStack Query)           [UI-032..UI-036, UI-042]
  lib/filters.ts       # FilterSpec da UI: valores padrão, reset, texto → FilterSpec            [UI-032..UI-035]
  lib/debounce.ts      # espera de 300 ms com temporizador injetável (fake timers nos testes)     [UI-033]
  components/LibraryGrid.tsx  LibraryList.tsx  GameCard.tsx  ViewToggle.tsx  Sort.tsx           [UI-029..UI-031, UI-034]
  components/FilterPanel.tsx  SearchBox.tsx  EmptyResults.tsx                                   [UI-032, UI-033, UI-035]
  components/GamePage.tsx  SessionsList.tsx  GameEditForm.tsx  DeleteDialog.tsx                 [UI-037..UI-041]
  components/LibraryStates.tsx   # "Conectando…", "Nenhum jogo ainda", contagem                 [UI-044]
  App.tsx              # roteia entre grade e página do jogo; remove LibrarySummary/LibraryEmpty [UI-045]
```

Regra do CLAUDE.md respeitada: `domain` e `store` não importam Windows; nada desta fase toca em Win32.

## Constantes (domain)

```go
const (
    MaxTitleRunes       = 200   // GAM-003, existente
    MaxNameRunes        = 100   // DOM-015, DOM-016
    MaxDescriptionRunes = 10000 // EDT-001
    MaxFilterTextRunes  = 200   // FLT-001
    MaxFilterIDs        = 50    // FLT-001
    MaxNameList         = 50    // IPC: NameList maxItems
    YearMin             = 1900  // DOM-013
    YearMax             = 2100  // DOM-013
    DefaultPageLimit    = 200   // LIB-002
    MaxPageLimit        = 500   // LIB-002
    DefaultSessionLimit = 50    // STO-027
    MaxSessionLimit     = 200   // STO-027
)
```
Os mesmos valores estão em `x-limits` do contrato; o teste de contrato (IPC-041) confere.

## Modelo de dados

Nenhuma tabela nova. Nenhuma coluna nova. Os enums continuam `TEXT` sem `CHECK` (ADR 0012): `game.status`,
`platform.category`, `play_mode.name`. O ano usa `game.release_date` (já existe). `completed_at` (`INTEGER` ms, ADR 0009)
passa a ser editável; `first_played_at`, `last_played_at`, `total_play_seconds` e `play_count` continuam derivados das
sessões (STO-014, STO-015).

### Migração 5 — índice reconstruído [STO-024]
```sql
-- +goose Up
DELETE FROM game_fts;
INSERT INTO game_fts (rowid, title, description, developer, publisher)
SELECT g.id, g.title, g.description,
       COALESCE((SELECT group_concat(c.name, ' ') FROM game_developer gd
                 JOIN company c ON c.id = gd.company_id WHERE gd.game_id = g.id), ''),
       COALESCE((SELECT group_concat(c.name, ' ') FROM game_publisher gp
                 JOIN company c ON c.id = gp.company_id WHERE gp.game_id = g.id), '')
FROM game g;

-- +goose Down
DELETE FROM game_fts;
```
- `store.SchemaVersion = 5`; `TestMigrate5` cria um banco na versão 4 com 3 jogos (com desenvolvedora e editora), migra
  e confere `MATCH` por título (STO-024). O `Down` só apaga o índice (o código da versão 4 não o lê).
- Migração é de dados, não de schema: roda dentro da transação do goose como as anteriores (sem `NO TRANSACTION`).

### Vínculos e catálogo
- `company` e `genre` têm `name UNIQUE` (case-sensitive no SQLite). Para casar sem diferenciar maiúsculas
  (EDT-004), a busca é `SELECT id, name FROM genre WHERE name = ? COLLATE NOCASE` antes do `INSERT`; a inserção usa o
  nome já cortado (DOM-016). Não há migração para mudar o índice.
- `game_genre`, `game_developer`, `game_publisher` e `game_play_mode` são substituídos por `DELETE ... WHERE game_id = ?`
  seguido de `INSERT` na mesma transação (STO-028). `play_mode` vem da semente da migração 1; `playModes` usa o nome.

## FilterSpec

Tipo em `domain` (`filter.go`), serializado com os nomes JSON do contrato:

| Membro JSON | Tipo Go | Regra | SQL (parâmetros) | IDs |
|---|---|---|---|---|
| `version` | `int` | obrigatório, `1` | — | FLT-001 |
| `platformIds` | `[]int64` | any-of, ≤ 50, únicos | `g.platform_id IN (…)` | FLT-002 |
| `genreIds` | `[]int64` | any-of | `EXISTS (SELECT 1 FROM game_genre x WHERE x.game_id = g.id AND x.genre_id IN (…))` | FLT-002 |
| `developerIds` | `[]int64` | any-of | `EXISTS` em `game_developer` | FLT-002 |
| `publisherIds` | `[]int64` | any-of | `EXISTS` em `game_publisher` | FLT-002 |
| `playMode` | `PlayModeGroup` | `all` padrão | `all`: sem cláusula; `single`/`multi`/`coop`: `EXISTS` em `game_play_mode` JOIN `play_mode` com os nomes de DOM-014 | FLT-003 |
| `yearFrom`, `yearTo` | `*int` | 1900..2100; `from ≤ to` | com algum limite: `g.release_date GLOB '[0-9][0-9][0-9][0-9]*' AND CAST(substr(g.release_date,1,4) AS INTEGER) BETWEEN ? AND ?` (com um só limite, o outro lado é aberto); sem limite: sem cláusula | FLT-004 |
| `statuses` | `[]GameStatus` | any-of; vazio = sem filtro | `g.status IN (…)` | FLT-005 |
| `favorite` | `*bool` | `null` = sem filtro | `g.favorite = ?` | FLT-005 |
| `played` | `string` | `any`/`played`/`notPlayed` | `g.play_count > 0` / `= 0` | FLT-006 |
| `playSecondsMin`, `playSecondsMax` | `*int64` | ≥ 0; `min ≤ max` | `g.total_play_seconds BETWEEN ? AND ?` (um lado) | FLT-006 |
| `text` | `string` | ≤ 200 runas, cortado | `g.id IN (SELECT rowid FROM game_fts WHERE game_fts MATCH ?)` com `ftsQuery(text)`; vazio → sem cláusula | FLT-007, FTS-005 |

- `Validate` recusa membro desconhecido, `version` diferente de 1, id ≤ 0, lista com duplicado ou > 50, faixa invertida,
  tipo errado (o decodificador do IPC já cuida dos tipos, via `ipc.DecodeParams` com ponteiros). Erro de domínio
  `ErrInvalidFilter` → `-32602` (IPC-043).
- `Normalize` tira espaços de `text`, ordena e remove duplicados das listas. Dois `FilterSpec` que filtram o mesmo
  conjunto podem ter formas diferentes; a igualdade do teste de round-trip (FLT-001) é sobre o JSON serializado a partir
  do objeto normalizado.
- A montagem SQL fica em `store/library.go` (`buildGameFilter(spec) (where string, args []any)`). Os fragmentos são
  constantes; só os valores vão como `?`. Os `IN (…)` são gerados com um `?` por item.
- O mesmo `buildGameFilter` monta o `COUNT(*)` do `total` (LIB-002) — uma só fonte de verdade para página e total.

## Busca FTS5

### Índice (FTS-001..FTS-004)
`game_fts` (migração 1) é `fts5(title, description, developer, publisher)` com `rowid = game.id`. O índice é mantido
pelo agente, na mesma transação de cada escrita de jogo (ADR 0020), por duas funções em `store/fts.go`:
- `reindexGame(ctx, tx, id)`: `DELETE FROM game_fts WHERE rowid = ?` e depois `INSERT INTO game_fts (rowid, title, …)
  SELECT g.id, g.title, g.description, <developers>, <publishers> FROM game g WHERE g.id = ?` (a mesma expressão da
  migração 5). Chamada por `AddGame` (FTS-002), `UpdateGame` (FTS-003) e pela migração.
- `deleteGameIndex(ctx, tx, id)`: `DELETE FROM game_fts WHERE rowid = ?` (FTS-004), chamada por `DeleteGame` antes de
  apagar a linha de `game`.
- Por que SQL direto e não sqlc: o sqlc não conhece a tabela virtual FTS5; as três consultas ficam em `fts.go` com um
  teste de contrato do formato. (ADR 0020.)
- Rowid reaproveitado: como `game.id` é `INTEGER PRIMARY KEY` sem `AUTOINCREMENT`, o SQLite pode reusar o maior id.
  Por isso a linha antiga é sempre apagada antes (FTS-004 tem teste com o id reaproveitado).

### Consulta (FTS-005)
`ftsQuery(text string) (string, bool)`:
1. Separa `text` em palavras por qualquer caractere que não seja letra ou dígito (Unicode), descarta vazias.
2. Cada palavra vira `"palavra"*` (aspas duplicadas por escape, `"` → `""`), e as palavras são juntadas por espaço (AND
   implícito do FTS5).
3. Se não sobrar palavra, `ok = false` e o filtro não entra (FLT-007).
Como toda palavra vai entre aspas, operadores (`AND`, `OR`, `NOT`), `:`, `*`, `(`, `)`, `^` e `-` perdem o significado de
sintaxe e viram texto. O tokenizador é o padrão `unicode61` (sem diferenciar caixa e, por padrão, sem acentos).

## Listagem (LIB)

- `Store.ListGames(ctx, Query) (Page, error)` com `Query{Filter FilterSpec; Sort Sort; Offset, Limit int}` e
  `Page{Games []Game; Total int64}`. `Sort` é um tipo fechado em `domain` (`title`, `lastPlayed`, `totalPlayed`, `year`).
- `sortClause(sort) string` devolve um fragmento fixo; todos terminam com `, g.id`, e os de `title` usam
  `g.title COLLATE NOCASE, g.id`:
  - `title`: `g.title COLLATE NOCASE ASC, g.id ASC` (LIB-001, GAM-005).
  - `lastPlayed`: `g.last_played_at IS NULL, g.last_played_at DESC, g.title COLLATE NOCASE, g.id`.
  - `totalPlayed`: `g.total_play_seconds DESC, g.title COLLATE NOCASE, g.id`.
  - `year`: `CASE WHEN g.release_date GLOB '[0-9][0-9][0-9][0-9]*' THEN 0 ELSE 1 END, substr(g.release_date,1,4), g.title COLLATE NOCASE, g.id`.
- Página: `ORDER BY … LIMIT ? OFFSET ?` (LIB-002). Desempate por `id` faz as páginas não repetirem nem pularem.
- A consulta de página faz `JOIN platform p` para `platform` (nome) e lê `status`, `favorite`, `release_date` (para o ano
  de DOM-013, calculado em Go com `domain.ReleaseYear`) — a conversão do ano fica em Go, não em SQL.
- `total` é `SELECT COUNT(*) FROM game g WHERE <mesmo where>` na mesma transação de leitura (`Store.Tx` com nível de
  leitura; o SQLite em WAL dá instantâneo consistente dentro de uma transação de leitura).
- Limite de tamanho: cada `Game` do IPC tem `path` (≤ 260 caracteres normalmente) e `platform` (≤ 100 runas) e título
  (≤ 200 runas): 500 × ~1 KB ≈ 500 KB, abaixo de 1 MiB (IPC-051). O teste de IPC-051 mede o caso de pior tamanho.

## Detalhe, sessões e catálogo

- `Store.GameDetail(ctx, id) (GameDetail, error)`: uma consulta a `game` + `platform`; depois as listas de
  `game_genre`/`genre`, `game_developer`/`company`, `game_publisher`/`company` e `game_play_mode`/`play_mode`, cada uma
  ordenada por nome sem diferenciar caixa; listas vazias como `[]` (STO-026).
- `Store.GameSessions(ctx, gameID, offset, limit) (SessionPage, error)`: `ErrGameNotFound` se o jogo não existe; depois
  `SELECT … FROM play_session WHERE game_id = ? ORDER BY started_at DESC, id DESC LIMIT ? OFFSET ?` e o `COUNT(*)`. Lê
  `end_reason` sem validar (leitor tolerante, ADR 0012). Pausas não entram (STO-027).
- `Store.Catalog(ctx) (Catalog, error)`: `platform` (id, name, category), `genre.name` e `company.name` ordenados por
  `COLLATE NOCASE`, e os cinco nomes de `play_mode` (STO-030). Nada é nulo.
- `Store.AddPlatform(ctx, name, category)`: `SELECT … WHERE name = ? COLLATE NOCASE`; se achou, devolve; senão `INSERT`.
  Validação antes (DOM-015/016) (EDT-008).

## Edição do jogo

### Patch
```go
// store: campos ausentes = nil (não mexe); campos anuláveis têm ponteiro duplo ou flag Clear (EDT-001, EDT-003).
type GamePatch struct {
    Title, Description *string
    PlatformID         *int64
    Status             *domain.GameStatus
    Favorite           *bool
    CompletedAt        **time.Time // nil = ausente; *nil = null (apaga)
    Year               **int       // nil = ausente; *nil = null (apaga release_date)
    Developers, Publishers, Genres *[]string
    PlayModes          *[]domain.PlayMode
}
```
O decodificador do IPC (`ipc.DecodeParams`) distingue ausente de `null` com um tipo próprio `Optional[T]`
(`Present bool`, `Null bool`, `Value T`) para evitar ponteiros duplos no código de negócio; o `store` recebe o patch já
resolvido.

### `Store.UpdateGame(ctx, id, patch, at)` [STO-028, EDT-005, EDT-007]
Uma transação, nesta ordem:
1. Lê o jogo (`ErrGameNotFound`); lê `platform` se `PlatformID` veio (`-32602` se inexistente, EDT-002).
2. Calcula os valores novos em memória (título, status, favorito, descrição, `completed_at` como meia-noite UTC,
   `release_date = domain.WithYear(atual, year)` ou `NULL` com `year = null`, `platform_id`).
3. Resolve o catálogo: para cada nome de desenvolvedora/editora/gênero, `get-or-create` com `COLLATE NOCASE` (EDT-004);
   para `playModes`, confere o nome em `play_mode`.
4. Compara com a linha atual: se nenhum campo mudou e os conjuntos são iguais, não grava nada (sem `updated_at`,
   sem reindexar) e devolve o detalhe (EDT-005).
5. Senão: `UPDATE game SET … , updated_at = at WHERE id = ?`; substitui os vínculos; `reindexGame`.
6. Commit. Qualquer erro → rollback: nenhum nome novo de catálogo, nenhum vínculo, nenhum índice muda (EDT-007).
O `Store` não valida texto de negócio: a validação (tamanhos, enums, faixa de ano) é do `domain` antes da transação, na
camada do agente. A transação só recusa o que depende do banco (plataforma inexistente).

## Remoção do jogo

### `Store.DeleteGame(ctx, id)` [STO-029, DEL-001, DEL-006]
Uma transação: (1) `SELECT 1 FROM play_session WHERE game_id = ? AND ended_at IS NULL` → `ErrGameInUse` (DEL-002, como
defesa em profundidade); (2) `deleteGameIndex`; (3) `DELETE FROM game WHERE id = ?` (0 linhas → `ErrGameNotFound`); as
demais linhas caem por `ON DELETE CASCADE` (`foreign_keys(1)` já está no DSN, `store.go`). Teste de atomicidade com
trigger `BEFORE DELETE ON game` que aborta, como em STO-021: confere que o índice e as sessões continuam (DEL-006).

### Serialização com o tracker [DEL-002, DEL-003, DEL-005]
Verificar "sem sessão" e apagar em duas chamadas deixaria uma janela para um `game.launch` entrar entre elas. Por isso a
checagem e a remoção rodam **na goroutine dona do tracker**, como os lançamentos:
```go
// internal/tracker (tracker-specialist)
// WithoutActiveGame runs fn in the tracker goroutine when neither a session nor a tracked launch of gameID is active.
// Otherwise it returns *SessionActiveError (the same type as Launch) without calling fn.
func (t *Tracker) WithoutActiveGame(ctx context.Context, gameID int64, fn func() error) error
```
- Mesmo canal de controle da fase 4 (`ctrlCh`, buffer 16). Enquanto `fn` roda, nenhum `Launch` ou `LaunchUntracked` é
  aceito (a goroutine dona está ocupada); o `fn` é curto (uma transação).
- O agente chama `tracker.WithoutActiveGame(ctx, id, func() error { return store.DeleteGame(ctx, id) })`. `ErrGameNotFound`
  de dentro de `fn` vai para `-32001`. O `*SessionActiveError` vai para `-32002` com `SessionRef` (DEL-002).
- Pedido com o agente em shutdown: `ErrShuttingDown` → `-32603` (remoção não é aplicada). Mesma regra de IPC-040.
- Lançamento sem contar tempo (`LaunchUntracked`) de um jogo que está sendo removido: a remoção não o impede (DEL-005);
  o processo fica vivo sem linha de jogo. Como `LaunchUntracked` lê o jogo antes, o pior caso é um lançamento de um
  jogo que sumiu logo depois: aceito e documentado (Limitações da fase 4, item 11).

## Sessões e tracker — sem mudança de comportamento

Nenhuma regra de SES-*, PAU-*, FBK-* muda. `game.update` não chama o tracker (EDT-006): a sessão é uma leitura de
`Current()` que não muda; o teste confere que `session.current` e `session.changed` ficam iguais.

## `internal/ipc` [IPC-041..IPC-052]

- `protocol.go`: novos métodos `MethodGameGet = "game.get"`, `MethodGameUpdate = "game.update"`,
  `MethodGameDelete = "game.delete"`, `MethodSessionList = "session.list"`, `MethodPlatformList = "platform.list"`,
  `MethodPlatformAdd = "platform.add"`, `MethodLibraryCatalog = "library.catalog"`. Tipos: `GameListParams{Filter *FilterSpec;
  Sort string; Offset, Limit *int}`, `GameListResult{Games []Game; Total int64; Offset int}`, `Game` com `platformId`,
  `platform`, `status`, `favorite`, `year *int`; `GameDetail`, `GameDetailResult`, `GameUpdateParams` (com `Optional[T]`
  para os campos anuláveis), `GameDeletedResult{GameID}`, `SessionItem`, `SessionListResult`, `Platform`, `Catalog`.
- `FilterSpec` é `domain.FilterSpec` com as tags JSON (um tipo só, decodificado com `ipc.DecodeParams`, que já recusa
  membro desconhecido — IPC-043). `Optional[T]` é usado só para o patch.
- `contract_test.go` lê `docs/specs/phase-5-library/contracts/ipc.openrpc.json`: versão `0.6.0`; cada método do contrato
  existe em `MethodXxx`; `x-limits` batem com as constantes de `domain`; os enums (`sort`, `playMode`, `status`,
  `played`, `category`) batem com o `domain`; os códigos de erro batem. O comentário do pacote aponta para o novo arquivo.
- `Session` e `SessionResult` não mudam (fase 4).

## `internal/agent` [IPC-042..IPC-052, DEL-*, EDT-*]

`library.go` registra os handlers no mesmo `gameHandlers`:
- `game.list`: `ipc.DecodeParams` (params vazios → padrão; membros inválidos → `-32602`); `Filter` vazio se ausente;
  `domain.Validate`; `store.ListGames`; converte para `ipc.Game` (`toIPCGame` ganha `platform`, `status`, `favorite`,
  `year`). `context.WithTimeout(ctx, 2 s)`.
- `game.get`: `store.GameDetail`; `ErrGameNotFound` → `-32001`.
- `game.update`: decodifica o patch; `domain.ValidatePatch` (título, status, descrição, datas, ano, nomes, modos) → `-32602`;
  `store.UpdateGame(ctx, id, patch, clock.Now())`; devolve `GameDetail`. Timeout 5 s. Sem notificação (IPC-050).
- `game.delete`: `tracker.WithoutActiveGame` (acima); mapeamento de erro: `ErrGameNotFound` → `-32001`,
  `*tracker.SessionActiveError` → `-32002` com `SessionRef`, `ErrShuttingDown`/fim do prazo → `-32603`. Timeout 5 s.
- `session.list`: `store.GameSessions`; paginação fora da faixa → `-32602`; `ErrGameNotFound` → `-32001`.
- `platform.list`, `platform.add`, `library.catalog`: leitura/escrita simples no `store`; validação no `domain`.
- Todos os handlers de escrita usam `clock.Now()` lido ao receber o pedido (ADR 0011); nenhum `time.Now()`.

## `cmd/arcadia` (UI em Go) [DSK-016..DSK-019]

- `library.go`: `ListGames(query)`, `GetGame(id)`, `GameSessions(id, offset, limit)`, `UpdateGame(id, patch)`,
  `DeleteGame(id)`, `Catalog()`, `AddPlatform(name, category)`. Cada um chama o método de IPC com o prazo de DSK-016/017 e
  converte o resultado em tipos de view (`GameView`, `GameDetailView`, `SessionItemView`, `CatalogView`), sem calcular
  nada além de formatação de instantes (RFC 3339, como hoje).
- `userError`: `-32002` em `DeleteGame` usa a mensagem de DSK-018 (o erro carrega a operação); demais códigos seguem
  DSK-011.
- `LibrarySummary` e `LibrarySummaryView` saem (DSK-019). `AddGame`/`ListGames` antigos são substituídos: `AddGame` fica,
  `ListGames()` sem parâmetros vira `ListGames(query)`.
- `wails generate module` atualiza `frontend/wailsjs/` (lembrete da fase 4).

## `frontend/` [UI-029..UI-046]

### Estado e dados (`lib/library.ts`)
- Chaves de consulta: `['library', 'games', query]` (paginada com `useInfiniteQuery`, `getNextPageParam` por
  `loaded < total`, UI-036), `['library', 'game', id]`, `['library', 'sessions', id]` (infinita, 50 por página),
  `['library', 'catalog']`.
- `query` = `{filter: FilterSpec; sort: Sort}` (`lib/filters.ts`, `version: 1`). Mudar `query` descarta as páginas
  (UI-036).
- Mutações (`useUpdateGame`, `useDeleteGame`, `useAddPlatform`, `useAddGame` existente): ao concluir, `invalidateQueries`
  de `games`, `game`, `catalog` (UI-042). Sem polling; `UI-009` recarrega na reconexão.
- A UI não escreve no banco: todos os caminhos passam pelos bindings de DSK-016/017.

### Grade virtualizada (UI-029, UI-030, UI-036)
- `@tanstack/react-virtual` (já previsto na [ARCHITECTURE §1](../../ARCHITECTURE.md)); se a dependência ainda não estiver
  instalada, sua inclusão é uma tarefa do planner. A grade calcula linhas (colunas conforme a largura) e renderiza só a
  janela visível. A lista usa o mesmo virtualizador com uma coluna.
- Pré-busca: quando o último índice visível passa de `loaded − 40`, dispara `fetchNextPage` uma vez (flag em ref).
- Testes com elemento de tamanho fixo (mock de `getBoundingClientRect`/`ResizeObserver`) e `Array` de 10 000 itens
  sintéticos; o número de cards no DOM é contado (UI-029). Nada de tempo real: nenhum `setTimeout` na grade.
- Preferência grade/lista: `localStorage['arcadia.libraryView']` (UI-030). Não vai para o agente.

### Busca e filtros (UI-032..UI-035)
- `SearchBox`: estado local; a consulta é atualizada por `lib/debounce.ts` com um temporizador do relógio injetado
  (padrão de `setTimeout` da UI, substituível em teste por `vi.useFakeTimers`); 300 ms (UI-033).
- `FilterPanel`: um controle por faceta; listas de opções vêm de `useCatalog()`; mudança → nova `query` (UI-032).
- `EmptyResults`: aparece quando `total = 0` e algum filtro está ativo; "Limpar filtros" → `{version: 1}` e `sort = 'title'`
  (UI-035). `LibraryStates` cuida de "Conectando…" e de "Nenhum jogo ainda" (UI-044).

### Página do jogo e sessões (UI-037..UI-038)
- `App` guarda `selectedGameId` (estado da tela, não do agente). `GamePage` chama `useGame(id)` e `useGameSessions(id)`.
- `SessionsList`: linha por sessão; sessão aberta (`running`/`paused`/`grace`) usa o `SessionPanel` da fase 4 para o
  cronômetro (UI-014/015/022), recebendo `activeMs` do agente; o total não é recalculado.
- Não há `<img>` nem `<video>` na página (fase 7).

### Edição e remoção (UI-039..UI-041)
- `GameEditForm` mantém o rascunho; `patch` = só os campos diferentes do `GameDetail` original (`diffPatch`, função pura
  testada). Validação de cliente (`lib/validation.ts`: título, ano, data `AAAA-MM-DD`, descrição) antes de habilitar
  "Salvar" (UI-040); o `domain` do agente valida de novo.
- `DeleteDialog` é um diálogo modal com foco preso e o texto de UI-041; a ação é enviada uma única vez (estado
  `pending`).

## Testes

- **Domínio** (`internal/domain/*_test.go`, qualquer SO): DOM-012..DOM-016 em tabela; FLT-001 (round-trip de JSON,
  `Validate` com cada erro), FLT-004 (ano aberto/fechado), LIB-001 (comparação de ordem como função pura de `sortKey`).
- **Store** (`internal/store/*_test.go`, SQLite em `t.TempDir()`): `TestMigrate5` (STO-024, com dados da versão 4);
  `fts_test.go` (FTS-001..FTS-005, incluindo rowid reaproveitado e trigger de falha para FTS-003/FTS-004);
  `library_test.go` (FLT-002..FLT-008 com uma tabela de jogos fixa, LIB-001..LIB-003, o total igual à soma das páginas
  de LIB-002 com 450 jogos); `game_edit_test.go` (EDT-001..EDT-007, comparação de `updated_at`, catálogo sem duplicata
  com grafia diferente); `game_delete_test.go` (DEL-001, DEL-006 com trigger); `sessions_list_test.go` (STO-027).
- **Tracker** (`internal/tracker`, tracker-specialist): `WithoutActiveGame` com sessão ativa, com lançamento pendente e
  sem nada (DEL-002, DEL-003); goroutine dona, `goleak`.
- **IPC** (`internal/ipc`): `contract_test.go` (IPC-041); `Dispatch` dos métodos novos com params inválidos (IPC-043,
  IPC-052); teste de tamanho de página (IPC-051) com 500 jogos de pior caso.
- **Agent** (`internal/agent/library_test.go`): handlers via `ipc.Dispatch` com `Store` real em temporário e tracker
  falso (IPC-042..IPC-050); nenhum `session.changed` em escrita (IPC-050); `game.update` com sessão em Running não
  muda `session.current` (EDT-006).
- **UI Go** (`cmd/arcadia/library_test.go`): mensagens de DSK-018 e conversões; `LibrarySummary` ausente (DSK-019).
- **Frontend** (Vitest): `LibraryGrid` com 10 000 itens e contagem de cards (UI-029); `ViewToggle` e persistência (UI-030);
  `useInfiniteGames` pré-busca uma vez (UI-036); `SearchBox` com debounce de fake timers (UI-033); `FilterPanel`
  monta `FilterSpec` (UI-032); `EmptyResults` e `Limpar` (UI-035); `GamePage` com sessões e marcação (UI-037, UI-038);
  `GameEditForm` com `diffPatch` e validação (UI-039, UI-040); `DeleteDialog` (UI-041); `UI-042` invalidação; `UI-043`;
  `UI-044` e `UI-045` (leitura de arquivos).
- **Manual:** UI-046 (biblioteca real com milhares de jogos).

## Riscos

- **Limite de 1 MiB e páginas grandes** (IPC-051): o teste de pior caso é o guarda; se a biblioteca tiver títulos maiores
  que o limite de 200 runas, o `Validate` já os recusa.
- **Consistência do índice:** toda escrita de jogo passa por `reindexGame`/`deleteGameIndex` na mesma transação; o único
  caminho fora é a migração 5 (rebuild completo). Um `INSERT` direto no banco (fora do agente) deixa o índice
  desatualizado; é o mesmo risco já aceito pelo ADR 0012 para os enums.
- **Remoção × lançamento:** serializada no tracker (acima). O único buraco aceito é o lançamento sem contar tempo de
  jogo que some logo depois (DEL-005).
- **Virtualização em teste:** o layout real do WebView não existe no Vitest; os testes usam tamanhos fixos e a manual
  UI-046 cobre o comportamento real.
- **Patch e `null`:** `completedAt` e `year` aceitam `null`; `Optional[T]` evita que ausente vire `null`. Teste de
  IPC-052 cobre os dois casos.
- **Tempo de consulta com milhares de jogos:** `ORDER BY` por `COLLATE NOCASE` e `COUNT(*)` com filtros de `EXISTS` são
  lineares; a meta é ≤ 100 ms para uma página de 200 com 10 000 jogos (teste de desempenho opcional, não bloqueante).
- **Compatibilidade:** UI e agente são o mesmo binário (`SchemaVersion = 5`), então não há combinação de versões a
  tratar. Um agente v4 que ainda estiver rodando recebe `game.list` com params e responde `-32602` (IPC-002); o
  `agent.status` e o `Version` do agente já permitem ao indicador de DSK/UI-001 mostrar a divergência, como hoje.
