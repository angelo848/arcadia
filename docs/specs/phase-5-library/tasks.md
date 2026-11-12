# Fase 5 — Biblioteca: tarefas

- **Status:** proposto pelo planner, aguardando aprovação do usuário (2026-11-12)
- **Spec:** [requirements.md](requirements.md) · [design.md](design.md) · [contracts/ipc.openrpc.json](contracts/ipc.openrpc.json) (v0.6.0)
- **ADRs:** 0004, 0009, 0010, 0011, 0012, 0015, 0019; propostos e a escrever pela sessão principal: 0020 (índice de busca e `FilterSpec`, antes da T05), 0021 (remoção de jogo, antes da T13), 0022 (listagem paginada, antes da T18).

## Decisões do usuário (2026-11-12)

- **Problema 1 (limite de 1 MiB):** paginação por orçamento de bytes. A página para antes de passar de 1 MiB, qualquer
  que seja o conteúdo; a T18 ganha esse critério no teste de pior caso.
- **Problema 2 (remoção com prazo vencido):** depois que a remoção começa, o agente espera o resultado real e responde
  sucesso se foi aplicada (decisão 7 do planner confirmada).
- **Dependência nova:** `@tanstack/react-virtual` aprovada (T25).
- **Decisão 9 / Problema 6:** `LibrarySummary`, `useLibrarySummary`, `LibraryEmpty` e `libraryGate` saem; DSK-008 vira
  substituído pelas leituras novas.

## Decisões do planner (a confirmar na aprovação)

1. **`Store.Tx` pública fica como está; helper interno `withTx`.** `Tx` só entrega `*dbgen.Queries`, e o FTS (`reindexGame`, `deleteGameIndex`) e o SQL dinâmico do filtro precisam de `*sql.Tx`. A T05 adiciona `func (s *Store) withTx(ctx context.Context, fn func(tx *sql.Tx, q *dbgen.Queries) error) error`, e `Tx` passa a delegar para ele.
2. **`Store.Games` sai na T18**, que é o último uso (o handler `game.list`). `Store.ListGames` o substitui. `Store.LibrarySummary` sai na T23 junto com o binding, porque `cmd/arcadia` compila contra ele e cada commit precisa passar em `go build ./...`.
3. **`Store.UpdateGame` devolve `GameDetail`** (não só `error`), para o `game.update` não precisar de uma segunda leitura. O design não diz o retorno.
4. **Testes de falha do FTS não usam trigger.** SQLite não cria trigger sobre tabela virtual FTS5. Nas falhas de FTS-002 e FTS-003 o teste faz `DROP TABLE game_fts` dentro do banco temporário, e a transação precisa abortar. DEL-006 usa trigger em `game` (como STO-021), que funciona.
5. **Ano em SQL com três padrões exatos**, não só prefixo `GLOB '[0-9]{4}*'`: `release_date GLOB '[0-9][0-9][0-9][0-9]' OR GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]' OR GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]'`. Isso coincide com `domain.ReleaseYear` (DOM-013) e evita que `1998abc` entre como 1998 no filtro e na ordenação.
6. **`ErrGameInUse` vira `-32002` com `sessionId: 0`** e o título lido com `Store.Game`. O store não conhece o id da sessão. O caso normal (sessão ativa ou lançamento pendente) já sai do `WithoutActiveGame` com `*SessionActiveError` e id real ou zero.
7. **Política de `WithoutActiveGame` com `ctx`:** se o `ctx` já venceu quando a goroutine dona pega o pedido, `fn` não roda e devolve `ctx.Err()`. Depois que `fn` começa, o chamador espera a resposta mesmo com `ctx` vencido (o `fn` usa o próprio `ctx` no banco, então ou grava tudo ou nada). Assim o agente não responde erro para uma remoção que foi aplicada. Ver Problema 2.
8. **Nova dependência: `@tanstack/react-virtual`.** Não está em `frontend/package.json` e o ARCHITECTURE só fala em "virtualização", sem nomear biblioteca. A T25 pede aprovação explícita do usuário antes de instalar.
9. **`libraryGate` e `storeOpener` saem na T23** (a UI deixa de abrir o banco). Sem o `LibrarySummary` eles não têm consumidor, e o teste DSK-008 de `cmd/arcadia/library_test.go` vai junto. Ver Problema 6.
10. **Binding `UpdateGame(id, patch)` recebe o patch como `json.RawMessage`.** Só assim o binding preserva ausente ≠ `null` vindo do JavaScript. Ver Problema 12.
11. **`platform.list` e `library.catalog` ordenam sem diferenciar maiúsculas** (`COLLATE NOCASE`), igual aos gêneros e empresas. A consulta atual `ListPlatforms` (`ORDER BY name`) é binária, então ganha uma consulta nova.
12. **`completedAt` valida data real** com `time.Parse("2006-01-02", s)`, então `2026-13-45` é `-32602`. A regex do contrato só checa formato.
13. **LIB-001 não tem `sortKey` no domínio.** A ordem é a `ORDER BY` da store, e os testes de LIB-001 ficam em `store` (o design mencionava `sortKey` nos testes de domínio, mas não no layout).
14. **Fora da lista de requisitos, mas cabe na T16:** `ipc.Game` ganha `platformId`, `platform`, `status`, `favorite` e `year` (o `game.add` do IPC-020 também muda de formato, ver Problema 10).

## Resumo

São 35 tarefas, em ordem de dependência, cada uma com commit próprio e build verde:
- **T01–T03 (domain):** status, modos, categoria e nomes; ano, data de conclusão e textos de edição; `FilterSpec`, `Sort` e limites.
- **T04–T14 (store):** migração 5 e `SchemaVersion = 5` (T04); índice FTS mantido na inserção (T05); consulta FTS5 (T06); detalhe do jogo (T07); `ListGames` com filtros (T08), com ordenação e página (T09); sessões do jogo (T10); `UpdateGame` campos simples (T11) e com vínculos e reindexação (T12); remoção (T13); catálogo e plataforma (T14).
- **T15 (tracker-specialist):** `WithoutActiveGame` (DEL-002, DEL-003).
- **T16–T17 (ipc):** protocolo v0.6.0 e teste de contrato (T16); decodificação de params com `Optional[T]` (T17).
- **T18–T22 (agent):** `game.list` paginado e `game.add` com campos novos (T18); `game.get` e `session.list` (T19); `game.update` (T20); `game.delete` (T21); plataformas e catálogo (T22).
- **T23–T24 (cmd/arcadia):** leituras e remoção de `LibrarySummary` (T23); escritas e mensagem de remoção (T24). Ambas rodam `wails generate module`.
- **T25–T34 (frontend):** dependência nova (T25, aguarda aprovação); lib de filtros e debounce (T26); consultas e mutações (T27); grade virtualizada (T28); lista e alternância (T29); painel, busca e estados (T30); página do jogo (T31); edição (T32); remoção com confirmação (T33); `App` e limpeza (T34).
- **T35:** verificação final, documentação e UI-046 manual.

Premissas:
- Ao fim de cada tarefa passam `CGO_ENABLED=0 go build ./...`, `go test ./...`, `go vet ./...` e `GOOS=linux go vet ./...`; no frontend, `npm run build` e `npx vitest run`.
- `sqlc generate` roda em T07, T10, T11, T12, T13 e T14 (cada uma com o diff de `internal/store/dbgen` revisado). As consultas de `ListGames`, do `COUNT` e do filtro são SQL montado à mão com fragmentos constantes e parâmetros (ver Problema 9).
- `cd cmd/arcadia && wails generate module` roda em T23 e em T24; atualiza `frontend/wailsjs/`, que é versionado.
- Os testes de falha de transação usam trigger em tabela comum (`game`, `game_developer`, `pause_interval`) e `DROP TABLE` para o FTS (decisão 4).
- Ordem de execução da UI com a de backend: T26 e T27 não dependem de binding novo (só tipos locais); a integração real começa em T34.

---

## T01 — domain: status, modos, categoria, nomes e limites
- Requisitos: DOM-012, DOM-014, DOM-015, DOM-016
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `internal/domain/game_status.go` (novo)
  - `internal/domain/play_mode.go` (novo)
  - `internal/domain/platform.go` (novo)
  - `internal/domain/name.go` (novo)
  - `internal/domain/limits.go` (novo: todas as constantes de limite de design.md, para as tarefas seguintes)
  - testes: `internal/domain/game_status_test.go`, `play_mode_test.go`, `platform_test.go`, `name_test.go` (novos)
- Assinaturas públicas:
  ```go
  // game_status.go
  type GameStatus string
  const (
      StatusNotPlayed GameStatus = "not_played" // padrão de jogo novo (DOM-012)
      StatusPlaying   GameStatus = "playing"
      StatusCompleted GameStatus = "completed"
      StatusAbandoned GameStatus = "abandoned"
      StatusBacklog   GameStatus = "backlog"
  )
  func ParseGameStatus(s string) (GameStatus, error) // exato

  // play_mode.go
  type PlayMode string
  const (
      PlayModeSingle      PlayMode = "single"
      PlayModeMultiLocal  PlayMode = "multi_local"
      PlayModeMultiOnline PlayMode = "multi_online"
      PlayModeCoopLocal   PlayMode = "coop_local"
      PlayModeCoopOnline  PlayMode = "coop_online"
  )
  func ParsePlayMode(s string) (PlayMode, error)
  type PlayModeGroup string
  const (
      PlayGroupAll    PlayModeGroup = "all"
      PlayGroupSingle PlayModeGroup = "single"
      PlayGroupMulti  PlayModeGroup = "multi"
      PlayGroupCoop   PlayModeGroup = "coop"
  )
  func ParsePlayModeGroup(s string) (PlayModeGroup, error) // exato; "" é erro (o padrão é aplicado em FilterSpec.Normalize, T03)
  // Modes devolve os modos do grupo; nil para PlayGroupAll (qualquer modo, inclusive nenhum).
  func (g PlayModeGroup) Modes() []PlayMode

  // platform.go
  type PlatformCategory string
  const (
      CategoryPC       PlatformCategory = "pc"
      CategoryConsole  PlatformCategory = "console"
      CategoryArcade   PlatformCategory = "arcade"
      CategoryHandheld PlatformCategory = "handheld"
  )
  func ParsePlatformCategory(s string) (PlatformCategory, error)

  // name.go
  var ErrInvalidField = errors.New("domain: invalid field")
  // NormalizeName corta espaços das pontas; vazio ou mais de MaxNameRunes runas => ErrInvalidField (DOM-015, DOM-016).
  func NormalizeName(s string) (string, error)

  // limits.go
  const (
      MaxNameRunes        = 100
      MaxTitleRunes       = 200
      MaxDescriptionRunes = 10000
      MaxFilterTextRunes  = 200
      MaxFilterIDs        = 50
      MaxNameList         = 50
      MaxFilterStatuses   = 5
      YearMin             = 1900
      YearMax             = 2100
      DefaultPageLimit    = 200
      MaxPageLimit        = 500
      DefaultSessionLimit = 50
      MaxSessionLimit     = 200
  )
  ```
- Testes:
  - `TestParseGameStatus`:
    - "DOM-012/not_played, playing, completed, abandoned e backlog são aceitos"
    - "DOM-012/vazio, Playing e valor desconhecido são erro"
  - `TestParsePlayMode`:
    - "DOM-014/single, multi_local, multi_online, coop_local e coop_online são aceitos"
    - "DOM-014/nome fora da lista e maiúscula são erro"
  - `TestParsePlayModeGroup`:
    - "DOM-014/all, single, multi e coop são aceitos e vazio é erro"
    - "DOM-014/multi tem multi_local e multi_online; coop tem coop_local e coop_online; single só single"
    - "DOM-014/all devolve nil (qualquer modo, inclusive jogo sem modo)"
  - `TestParsePlatformCategory`: "DOM-015/pc, console, arcade e handheld são aceitos" e "DOM-015/outro valor é erro"
  - `TestNormalizeName`:
    - "DOM-016/corta espaços das pontas e mantém a capitalização"
    - "DOM-016/vazio depois de cortar é erro"
    - "DOM-016/100 runas passam e 101 são erro"
    - "DOM-015/nome de plataforma usa a mesma regra de 1 a 100 runas"
- Depende de: —

## T02 — domain: ano, data de conclusão e textos de edição
- Requisitos: DOM-013, EDT-001, EDT-003
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `internal/domain/release_date.go` (novo: `ReleaseYear`, `WithYear`)
  - `internal/domain/completed.go` (novo: `ParseCompletedDate`, `FormatCompletedDate`)
  - `internal/domain/text.go` (novo: `ValidateTitle`, `ValidateDescription`)
  - testes: `release_date_test.go`, `completed_test.go`, `text_test.go` (novos)
- Assinaturas públicas:
  ```go
  // release_date.go
  // ReleaseYear dá o ano de YYYY, YYYY-MM ou YYYY-MM-DD; outro texto não dá ano (ok = false).
  func ReleaseYear(releaseDate string) (year int, ok bool)
  // WithYear: se releaseDate já tem o ano y, devolve releaseDate; senão "y" com quatro dígitos. y fora de YearMin..YearMax => ErrInvalidField.
  func WithYear(releaseDate string, year int) (string, error)

  // completed.go
  // ParseCompletedDate lê YYYY-MM-DD (data real, time.Parse) e devolve meia-noite UTC (ADR 0009, decisão h).
  func ParseCompletedDate(s string) (time.Time, error) // erro => ErrInvalidField
  // FormatCompletedDate devolve YYYY-MM-DD da data em UTC.
  func FormatCompletedDate(t time.Time) string

  // text.go
  // ValidateTitle corta espaços; vazio ou mais de MaxTitleRunes => ErrInvalidField (EDT-001, GAM-003 sem o padrão).
  func ValidateTitle(s string) (string, error)
  // ValidateDescription não corta; pode ser vazia; mais de MaxDescriptionRunes => ErrInvalidField.
  func ValidateDescription(s string) (string, error)
  ```
- Testes:
  - `TestReleaseYear`:
    - "DOM-013/YYYY, YYYY-MM e YYYY-MM-DD dão o ano"
    - "DOM-013/abc, 98 e vazio não dão ano"
  - `TestWithYear`:
    - "DOM-013/1998-06-12 com 1998 fica igual; com 1999 vira 1999"
    - "DOM-013/vazio com 2001 vira 2001"
    - "EDT-003/ano 1899 ou 2101 é erro; 1900 e 2100 passam"
  - `TestParseCompletedDate`:
    - "EDT-001/2026-11-12 vira 2026-11-12T00:00:00Z"
    - "EDT-001/formato diferente e data inexistente como 2026-13-45 são erro"
    - "EDT-001/FormatCompletedDate devolve YYYY-MM-DD em UTC"
  - `TestValidateText`:
    - "EDT-001/title corta espaços e recusa vazio ou 201 runas"
    - "EDT-001/description aceita vazio e 10 000 runas, e recusa 10 001"
- Depende de: T01

## T03 — domain: FilterSpec, validação e ordem de leitura
- Requisitos: FLT-001, LIB-001
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `internal/domain/filter.go` (novo: `FilterSpec`, `Validate`, `Normalize`)
  - `internal/domain/library_sort.go` (novo: `Sort`, `ParseSort`)
  - testes: `internal/domain/filter_test.go`, `library_sort_test.go` (novos)
- Assinaturas públicas:
  ```go
  // filter.go
  const FilterVersion = 1
  var ErrInvalidFilter = errors.New("domain: invalid filter")

  // Nomes JSON do contrato v0.6.0 (FilterSpec). Membros ausentes ou null não filtram.
  type FilterSpec struct {
      Version        int           `json:"version"`
      PlatformIDs    []int64       `json:"platformIds,omitempty"`
      GenreIDs       []int64       `json:"genreIds,omitempty"`
      DeveloperIDs   []int64       `json:"developerIds,omitempty"`
      PublisherIDs   []int64       `json:"publisherIds,omitempty"`
      PlayMode       PlayModeGroup `json:"playMode,omitempty"`
      YearFrom       *int          `json:"yearFrom,omitempty"`
      YearTo         *int          `json:"yearTo,omitempty"`
      Statuses       []GameStatus  `json:"statuses,omitempty"`
      Favorite       *bool         `json:"favorite,omitempty"`
      Played         string        `json:"played,omitempty"` // any | played | notPlayed
      PlaySecondsMin *int64        `json:"playSecondsMin,omitempty"`
      PlaySecondsMax *int64        `json:"playSecondsMax,omitempty"`
      Text           string        `json:"text,omitempty"`
  }
  // Validate devolve erro que embrulha ErrInvalidFilter (version = 1; ids > 0, únicos, até 50; statuses válidos, únicos, até 5; played no conjunto; anos em YearMin..YearMax e from <= to; segundos >= 0 e min <= max; text até 200 runas depois de cortar).
  func (f FilterSpec) Validate() error
  // Normalize corta text, ordena e tira repetidos das listas, e põe PlayMode = PlayGroupAll e Played = "any" quando vazios.
  func (f FilterSpec) Normalize() FilterSpec

  // library_sort.go
  type Sort string
  const (
      SortTitle       Sort = "title"
      SortLastPlayed  Sort = "lastPlayed"
      SortTotalPlayed Sort = "totalPlayed"
      SortYear        Sort = "year"
      DefaultSort          = SortTitle
  )
  func ParseSort(s string) (Sort, error) // exato; "" é erro (o agente aplica DefaultSort quando o membro é ausente)
  ```
- Testes:
  - `TestFilterValidate`:
    - "FLT-001/{version:1} e filtro cheio válido passam"
    - "FLT-001/version ausente, 0 ou 2 é erro"
    - "FLT-001/id zero ou negativo, 51 ids numa faceta e ids repetidos são erro"
    - "FLT-001/yearFrom > yearTo e playSecondsMin > playSecondsMax são erro"
    - "FLT-001/text com 201 runas depois de cortar é erro e 200 passa"
    - "FLT-005/statuses com valor inválido, repetido ou mais de 5 é erro"
    - "FLT-006/played fora de any, played e notPlayed é erro"
  - `TestFilterJSON`: "FLT-001/FilterSpec completo serializado e lido de volta é igual ao original"
  - `TestFilterNormalize`: "FLT-001/text sem espaços nas pontas; listas ordenadas e sem repetição; PlayMode vazio vira all e Played vazio vira any"
  - `TestParseSort`:
    - "LIB-001/title, lastPlayed, totalPlayed e year são aceitos"
    - "LIB-001/vazio, Title e idle são erro"
- Depende de: T01

## T04 — store: migração 5 e SchemaVersion 5
- Requisitos: STO-024, STO-001, AST-001
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `internal/store/migrations/00005_fts_rebuild.sql` (novo; `Up`: `DELETE FROM game_fts` e `INSERT INTO game_fts (rowid, title, description, developer, publisher)` com o `SELECT` de design.md §Migração 5; `Down`: `DELETE FROM game_fts`)
  - `internal/store/store.go` (`SchemaVersion = 5`; o único outro efeito é o comentário)
  - testes: `internal/store/migrate5_test.go` (novo) e `internal/store/migrate4_test.go` (ajustar)
- Assinaturas públicas:
  ```go
  const SchemaVersion int64 = 5
  ```
- Notas de implementação:
  - A migração é de dados, roda dentro da transação do goose (sem `NO TRANSACTION`).
  - `migrate4_test.go` tem o caso "STO-018/SchemaVersion é 4", que quebra com a versão 5: vira "STO-024/a migração 4 continua embutida" (versão 4 existe no provider).
  - `store_test.go`, `settings_test.go` e `agent/app_test.go` já usam `store.SchemaVersion` e passam sem mudança.
- Testes (`t.TempDir()`, banco v4 gerado por goose `UpTo(4)`):
  - `TestMigrate5`:
    - "STO-024/banco novo fica na versão 5 e game_fts existe"
    - "STO-024/banco v4 com 3 jogos migra e MATCH pelo título encontra cada um"
    - "STO-024/desenvolvedora e editora de um jogo entram no índice (MATCH pelo nome da empresa)"
    - "STO-024/jogos, sessões e pausas permanecem iguais após a migração"
    - "STO-024/SchemaVersion é 5 e a versão do goose é 5"
    - "STO-024/Down apaga só as linhas de game_fts e mantém os jogos"
- Depende de: —

## T05 — store: índice FTS mantido na inserção de jogo
- Requisitos: FTS-001, FTS-002, GAM-001 (alterado)
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `internal/store/fts.go` (novo)
  - `internal/store/store.go` (acrescentar `withTx`; `Tx` passa a delegar)
  - `internal/store/games.go` (`AddGame` usa `withTx` e chama `reindexGame` na mesma transação)
  - testes: `internal/store/fts_test.go` (novo); `internal/store/games_test.go` (ajustar se necessário)
- Assinaturas públicas (internas ao pacote):
  ```go
  // store.go
  func (s *Store) withTx(ctx context.Context, fn func(tx *sql.Tx, q *dbgen.Queries) error) error

  // fts.go
  // reindexGame apaga a linha de game_fts de id e a reinsere com título, descrição e nomes das empresas (mesma expressão da migração 5).
  func reindexGame(ctx context.Context, tx *sql.Tx, id int64) error
  // deleteGameIndex apaga a linha de game_fts de id.
  func deleteGameIndex(ctx context.Context, tx *sql.Tx, id int64) error
  ```
- Testes:
  - `TestFTSIndexing`:
    - "FTS-001/linha com rowid = id e colunas título, descrição, desenvolvedoras e editoras (empresas inseridas direto no teste)"
    - "FTS-001/nome de gênero, de plataforma, ano e modo não são encontrados pelo índice"
    - "FTS-002/AddGame deixa a linha de game_fts com o título depois do commit"
    - "FTS-002/falha no índice (game_fts removida no teste com DROP TABLE) faz AddGame devolver erro e não grava o jogo"
    - "GAM-001/o mesmo executável adicionado duas vezes gera dois jogos e duas linhas de índice"
- Depende de: T04
- Pré-requisito: ADR 0020 escrito pela sessão principal.

## T06 — store: consulta livre do FTS5
- Requisitos: FTS-005
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `internal/store/fts.go` (alterar: `ftsQuery`)
  - testes: `internal/store/fts_query_test.go` (novo)
- Assinaturas públicas (internas):
  ```go
  // ftsQuery separa text em palavras (letras e dígitos Unicode), cada uma vira "palavra"* com aspas duplicadas por escape, unidas por espaço (AND). ok = false quando não sobra palavra.
  func ftsQuery(text string) (query string, ok bool)
  ```
- Testes:
  - `TestFTSQuery`:
    - "FTS-005/palavras viram \"palavra\"* e são unidas por espaço"
    - "FTS-005/aspas, dois-pontos, asterisco, parênteses, hífen e circunflexo não geram erro de sintaxe"
    - "FTS-005/AND, OR e NOT digitados viram texto e não mudam o sentido"
    - "FTS-005/só pontuação devolve ok = false"
  - `TestFTSQueryMatch` (banco real): "FTS-005/pokemon encontra o jogo com título Pokémon"
- Depende de: T05

## T07 — store: campos do jogo e página de detalhe
- Requisitos: STO-026
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `internal/store/games.go` (alterar: `Game` ganha campos; `AddGame` preenche `Platform = "PC"`, `Status`, `Favorite`, `ReleaseDate`)
  - `internal/store/library.go` (novo: `GameDetail`)
  - `internal/store/queries/library.sql` (acrescentar: detalhe, vínculos e nomes de gênero, desenvolvedora, editora e modos por jogo)
  - `internal/store/dbgen/*` (regerado com **`sqlc generate`**; o diff deve ser só das consultas novas)
  - testes: `internal/store/library_detail_test.go` (novo)
- Assinaturas públicas:
  ```go
  type Game struct { // acrescentar campos aos existentes
      ID                                                      int64
      Title, LaunchType, LaunchTarget, LaunchArgs, WorkingDir string
      TotalPlaySeconds, PlayCount                             int64
      FirstPlayedAt, LastPlayedAt                             time.Time
      PlatformID                                              int64
      Platform                                                string
      Status                                                  domain.GameStatus // leitor tolerante (ADR 0012)
      Favorite                                                bool
      ReleaseDate                                             string // "" quando NULL
  }
  type GameDetail struct {
      Game
      Description                    string
      CompletedAt                    time.Time // zero = NULL
      Developers, Publishers, Genres []CatalogItem  // nunca nil; ordem NOCASE
      PlayModes                      []string  // nunca nil; nomes de DOM-014
  }
  func (s *Store) GameDetail(ctx context.Context, id int64) (GameDetail, error) // id inexistente => erro que embrulha ErrGameNotFound
  ```
- Testes:
  - `TestGameDetail`:
    - "STO-026/detalhe traz os campos de Game, descrição, data de conclusão, vínculos e modos"
    - "STO-026/listas sem nada são [] e nunca nil"
    - "STO-026/nomes de gênero, desenvolvedora, editora e modo em ordem alfabética sem diferenciar maiúsculas"
    - "STO-026/id inexistente devolve erro que embrulha ErrGameNotFound"
  - `TestAddGameFields`: "GAM-001/AddGame devolve Platform PC, Status not_played, Favorite false e ReleaseDate vazio"
- Depende de: T01, T02

## T08 — store: listagem com filtros
- Requisitos: FLT-002, FLT-003, FLT-004, FLT-005, FLT-006, FLT-007, FLT-008, STO-025 (parcial: filtro e total)
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `internal/store/library.go` (alterar: `Query`, `Page`, `ListGames`, `buildGameFilter`)
  - `internal/store/games.go` (remover `Store.Games`, ver decisão 2; fica na T18)
  - testes: `internal/store/library_filter_test.go` (novo)
- Assinaturas públicas:
  ```go
  type Query struct {
      Filter domain.FilterSpec
      Sort   domain.Sort
      Offset int
      Limit  int
  }
  type Page struct {
      Games []Game // nunca nil
      Total int64  // conta todos que passam pelo filtro, não só a página (LIB-002)
  }
  // ListGames valida Filter (Normalize + Validate) e os limites (Limit 1..MaxPageLimit, Offset >= 0); fora disso, erro sem consultar.
  func (s *Store) ListGames(ctx context.Context, q Query) (Page, error)
  // buildGameFilter devolve a cláusula WHERE com fragmentos constantes e ? por valor; "" quando não há filtro.
  func buildGameFilter(f domain.FilterSpec) (where string, args []any)
  ```
- Notas: página e total usam o mesmo `where` na mesma transação (`withTx`); ano em SQL usa os três padrões da decisão 5; `text` passa por `ftsQuery` (T06).
- Testes (tabela fixa de jogos, com desenvolvedora, gênero e modos inseridos pelo teste):
  - `TestListGamesFilter`:
    - "FLT-002/platformIds [1,2] com genreIds [5] dá [A, B]; platformIds [1] com genreIds [5,6] dá [A, C]"
    - "FLT-002/developerIds e publisherIds seguem a mesma regra"
    - "FLT-003/playMode multi dá [M]; single dá [S]; coop dá []; all ou ausente dá [M, S, N]"
    - "FLT-004/yearFrom 2000 dá [2005]; yearFrom 1998 e yearTo 2005 dá [1998, 2005]"
    - "FLT-004/com limite, jogo sem ano fica fora; sem limite, jogo sem ano entra"
    - "FLT-005/statuses é any-of e lista vazia não filtra"
    - "FLT-005/favorite true só favoritos, false só não favoritos, null não filtra"
    - "FLT-006/played dá só jogos com play_count > 0; notPlayed só com 0; any não filtra"
    - "FLT-006/faixa de segundos é inclusiva; jogo com 0 s fica fora com playSecondsMin 1"
    - "FLT-007/text combina com os outros filtros; text só com espaços não filtra"
    - "FLT-008/filtro vazio devolve a biblioteca inteira e total = número de jogos; com favorite true, total = 1 e página e total batem"
    - "STO-025/Total usa o mesmo WHERE da página"
- Depende de: T03, T06, T07

## T09 — store: ordenação, paginação e lista nunca nula
- Requisitos: LIB-001, LIB-002, LIB-003, STO-025, GAM-005 (alterado)
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `internal/store/library.go` (alterar: `sortClause`, `ORDER BY`, `LIMIT/OFFSET`)
  - testes: `internal/store/library_sort_test.go` (novo); `internal/store/games_test.go` (ajustar: os casos de GAM-005 passam por `ListGames`)
- Assinaturas públicas (internas):
  ```go
  // sortClause devolve um fragmento fixo de ORDER BY; todos terminam com g.id.
  func sortClause(s domain.Sort) (string, error)
  ```
- Notas: `title` usa `g.title COLLATE NOCASE, g.id`; `lastPlayed` usa `g.last_played_at IS NULL, g.last_played_at DESC, …`; `totalPlayed` usa `g.total_play_seconds DESC, …`; `year` usa `CASE` do ano (decisão 5) e depois `substr(g.release_date,1,4)`.
- Testes:
  - `TestListGamesSort`:
    - "LIB-001/title com b(1), A(2), a(3), C(4) dá A, a, b, C"
    - "LIB-001/lastPlayed põe nunca jogados por último, com desempate por título e id"
    - "LIB-001/totalPlayed é decrescente; year é crescente com sem ano por último"
    - "LIB-001/busca por texto usa o mesmo sort, sem ranking"
    - "GAM-005/ordem padrão é a de antes: título sem diferenciar maiúsculas e depois id"
  - `TestListGamesPage`:
    - "LIB-002/450 jogos com limit 200 dão páginas de 200, 200 e 50 sem repetição nem falta"
    - "LIB-002/offset além do fim dá games [] e total correto"
    - "LIB-002/limit 0 ou 501 e offset negativo são erro (no store)"
    - "STO-025/Total é a contagem do filtro, não da página"
  - `TestListGamesEmpty`: "LIB-003/filtro sem resultado e biblioteca vazia dão games [] (não nil) e total 0"
- Depende de: T08

## T10 — store: sessões de um jogo
- Requisitos: STO-027
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `internal/store/sessions_list.go` (novo: `GameSessions`)
  - `internal/store/queries/sessions.sql` (acrescentar `CountSessionsByGame :one`; a página é `ORDER BY started_at DESC, id DESC LIMIT ? OFFSET ?`)
  - `internal/store/dbgen/*` (regerado com **`sqlc generate`**)
  - testes: `internal/store/sessions_list_test.go` (novo)
- Assinaturas públicas:
  ```go
  type SessionItem struct {
      ID, GameID    int64
      State         domain.SessionState // leitor tolerante
      StartedAt     time.Time
      EndedAt       time.Time // zero = NULL (sessão aberta)
      ActiveSeconds int64
      EndReason     string    // "" = NULL
  }
  type SessionPage struct {
      Sessions []SessionItem // nunca nil
      Total    int64         // todas as sessões do jogo
  }
  // GameSessions: id inexistente => erro que embrulha ErrGameNotFound. offset >= 0 e limit 1..MaxSessionLimit, senão erro.
  func (s *Store) GameSessions(ctx context.Context, gameID int64, offset, limit int) (SessionPage, error)
  ```
- Testes:
  - `TestGameSessions`:
    - "STO-027/sessões ordenadas por started_at e id decrescentes, com ended, discarded e a aberta"
    - "STO-027/sessão aberta tem EndedAt zero e discarded aparece na lista"
    - "STO-027/total conta todas e a página respeita offset e limit"
    - "STO-027/jogo sem sessões dá Sessions [] e Total 0"
    - "STO-027/jogo inexistente devolve ErrGameNotFound"
- Depende de: T07

## T11 — store: edição de campos simples
- Requisitos: EDT-001, EDT-002, EDT-003, EDT-005, EDT-007, STO-028 (parcial: campos, sem vínculos)
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `internal/store/game_edit.go` (novo: `GamePatch`, `UpdateGame`)
  - `internal/store/queries/games.sql` (acrescentar: `GetGameForUpdate :one`, `UpdateGameFields :execrows`, `PlatformExists :one`)
  - `internal/store/dbgen/*` (regerado com **`sqlc generate`**)
  - testes: `internal/store/game_edit_test.go` (novo)
- Assinaturas públicas:
  ```go
  var ErrPlatformNotFound = errors.New("store: platform not found")
  // Ausente = nil. Campos anuláveis: *nil = null (apaga). Os vínculos (Developers, Publishers, Genres, PlayModes) são aceitos aqui, mas só são gravados na T12.
  type GamePatch struct {
      Title, Description *string
      PlatformID         *int64
      Status             *domain.GameStatus
      Favorite           *bool
      CompletedAt        **time.Time
      Year               **int
      Developers         *[]string
      Publishers         *[]string
      Genres             *[]string
      PlayModes          *[]domain.PlayMode
  }
  // UpdateGame grava numa transação (withTx). Devolve GameDetail (decisão 3). updated_at = at só se algum campo mudou de fato (EDT-005).
  func (s *Store) UpdateGame(ctx context.Context, id int64, p GamePatch, at time.Time) (GameDetail, error)
  ```
- Notas: `Year` novo = `domain.WithYear(atual, y)`; `Year` nulo zera `release_date`; `CompletedAt` é gravado como UTC; plataforma inexistente → `ErrPlatformNotFound`; `ErrGameNotFound` para id inexistente.
- Testes:
  - `TestUpdateGameFields`:
    - "EDT-001/title, status, favorite, description e completedAt são gravados"
    - "EDT-001/completedAt grava meia-noite UTC do dia; CompletedAt nulo apaga"
    - "EDT-002/platformId existente troca a plataforma e mantém sessões, total e contagem"
    - "EDT-002/platformId inexistente devolve ErrPlatformNotFound e nada muda"
    - "EDT-003/year 1998 com release_date 1998-06-12 preserva a data; year nulo zera release_date"
    - "EDT-005/total, contagem, primeira e última vez, sessões e pausas iguais antes e depois"
    - "EDT-005/patch igual ao atual não muda updated_at; patch com mudança muda"
    - "EDT-007/trigger BEFORE UPDATE ON game que aborta faz UpdateGame devolver erro e nada muda"
    - "STO-028/UpdateGame devolve o GameDetail já atualizado"
    - "STO-028/id inexistente devolve ErrGameNotFound"
- Depende de: T07, T02, T04

## T12 — store: vínculos, catálogo e reindexação na edição
- Requisitos: EDT-004, EDT-007, FTS-003, STO-028
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `internal/store/game_edit.go` (alterar: vínculos e reindexação)
  - `internal/store/queries/library.sql` (acrescentar consultas de catálogo: `GetGenreByName`, `InsertGenre`, `GetCompanyByName`, `InsertCompany`, `DeleteGameGenres`, `DeleteGameDevelopers`, `DeleteGamePublishers`, `DeleteGamePlayModes`, e os `Insert*` dos vínculos)
  - `internal/store/dbgen/*` (regerado com **`sqlc generate`**)
  - testes: `internal/store/game_edit_catalog_test.go` (novo)
- Assinaturas públicas (internas ao pacote):
  ```go
  // resolveName devolve o id do nome já gravado (COLLATE NOCASE) ou cria a linha; o nome novo sai de DOM-016, já cortado.
  func resolveCompany(ctx context.Context, q *dbgen.Queries, name string) (int64, error)
  func resolveGenre(ctx context.Context, q *dbgen.Queries, name string) (int64, error)
  ```
- Notas: vínculos são substituídos (`DELETE ... WHERE game_id = ?` e `INSERT`); a reindexação (`reindexGame`) acontece no fim da transação, só se algo mudou; qualquer erro desfaz tudo (catálogo novo incluído).
- Testes:
  - `TestUpdateGameLinks`:
    - "EDT-004/developers [Team Cherry, team cherry] com Team Cherry existente gera uma só empresa com a grafia já gravada"
    - "EDT-004/genres e playModes substituem o conjunto; array vazio limpa"
    - "EDT-004/nome novo cria a linha de catálogo"
    - "STO-028/os vínculos são substituídos numa transação"
  - `TestUpdateGameIndex`:
    - "FTS-003/editar descrição e trocar desenvolvedora: o texto novo encontra o jogo, o antigo não, e a desenvolvedora nova encontra"
    - "FTS-003/falha na reindexação (game_fts removida no teste com DROP TABLE) desfaz a edição inteira"
  - `TestUpdateGameAtomic`:
    - "EDT-007/patch com developers novos e uma falha depois não cria empresa nova e não muda linhas"
    - "STO-028/patch sem mudança não reindexa e não grava updated_at"
- Depende de: T11, T05

## T13 — store: remoção de jogo
- Requisitos: DEL-001, DEL-006, FTS-004, STO-029
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `internal/store/game_delete.go` (novo: `ErrGameInUse`, `DeleteGame`)
  - `internal/store/queries/games.sql` (acrescentar: `HasOpenSession :one`, `DeleteGameRow :execrows`)
  - `internal/store/dbgen/*` (regerado com **`sqlc generate`**)
  - testes: `internal/store/game_delete_test.go` (novo)
- Assinaturas públicas:
  ```go
  var ErrGameInUse = errors.New("store: game has an open session")
  // DeleteGame, numa transação: checa sessão aberta (ErrGameInUse), apaga a linha de game_fts (deleteGameIndex), apaga a linha de game (0 linhas => ErrGameNotFound). As demais linhas caem por ON DELETE CASCADE (foreign_keys = 1).
  func (s *Store) DeleteGame(ctx context.Context, id int64) error
  ```
- Testes:
  - `TestDeleteGame`:
    - "DEL-001/remove o jogo, as sessões, as pausas, os vínculos, additional_app, tracking_rule e media_asset"
    - "DEL-001/outros jogos, a plataforma, gêneros, empresas e modos continuam como estavam"
    - "STO-029/jogo com sessão aberta devolve ErrGameInUse e nada muda"
    - "STO-029/id inexistente devolve ErrGameNotFound"
  - `TestDeleteGameAtomic`:
    - "DEL-006/trigger BEFORE DELETE ON game que aborta mantém jogo, sessões, pausas e linha de game_fts"
    - "FTS-004/após remover, o título do jogo não aparece no índice"
    - "FTS-004/um jogo novo com o mesmo id reaproveitado não aparece pelo texto do removido"
- Depende de: T05, T11

## T14 — store: catálogo e criação de plataforma
- Requisitos: STO-030, EDT-008 (parte de store)
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `internal/store/catalog.go` (novo: `Platform`, `Catalog`, `Platforms`, `Catalog`, `AddPlatform`)
  - `internal/store/queries/library.sql` (acrescentar: `ListGenreNames`, `ListCompanyNames` ordenados por `COLLATE NOCASE`; `GetPlatformByNameNoCase` com `COLLATE NOCASE`; `ListPlatformsByName` com `COLLATE NOCASE`)
  - `internal/store/dbgen/*` (regerado com **`sqlc generate`**)
  - testes: `internal/store/catalog_test.go` (novo)
- Assinaturas públicas:
  ```go
  type Platform struct {
      ID       int64
      Name     string
      Category domain.PlatformCategory
  }
  type Catalog struct {
      Platforms []Platform
      Genres    []string
      Companies []CatalogItem
      PlayModes []domain.PlayMode // os cinco de DOM-014
  }
  // Platforms: ordem de nome sem diferenciar maiúsculas (IPC-048). Nunca nil.
  func (s *Store) Platforms(ctx context.Context) ([]Platform, error)
  // Catalog: nada nulo; gêneros e empresas em ordem NOCASE.
  func (s *Store) Catalog(ctx context.Context) (Catalog, error)
  // AddPlatform: name e category já validados pelo domain (EDT-008). Se existir plataforma com o mesmo nome sem diferenciar maiúsculas, devolve a existente e ignora category.
  func (s *Store) AddPlatform(ctx context.Context, name string, category domain.PlatformCategory) (Platform, error)
  ```
- Testes:
  - `TestCatalog`:
    - "STO-030/Catalog tem plataformas com id, name e category, gêneros e empresas em ordem NOCASE, e os cinco modos"
    - "STO-030/nenhum campo é nulo em banco sem gênero nem empresa"
  - `TestAddPlatform`:
    - "EDT-008/nome novo cria a plataforma e devolve ela"
    - "EDT-008/mesmo nome em outra grafia devolve a existente e ignora a categoria enviada"
  - `TestPlatforms`: "IPC-048/Platforms em ordem de nome sem diferenciar maiúsculas; lista vazia não é nil"
- Depende de: T01

## T15 — tracker: `WithoutActiveGame` (tracker-specialist)
- Requisitos: DEL-002, DEL-003
- Responsável: tracker-specialist (código em `internal/tracker/`) · Testes: test-writer (`internal/tracker/*_test.go`)
- Arquivos:
  - `internal/tracker/tracker.go` (alterar: `ctrlWithoutGame` em `ctrlKind`, campos `gameID` e `fn` em `ctrlReq`, `WithoutActiveGame`, tratamento no `handleControl`)
  - testes: `internal/tracker/tracker_test.go` (alterar), `internal/tracker/fakes_test.go` (alterar se preciso)
- Assinaturas públicas:
  ```go
  // WithoutActiveGame roda fn na goroutine dona quando não há sessão nem lançamento rastreado pendente de gameID.
  // Se há (qualquer estado fora de Idle com gameID igual), devolve *SessionActiveError sem chamar fn:
  // Session.GameID = gameID; sessão aberta => ID e Title dela; lançamento pendente => ID 0 e o título do lançamento.
  // Não é recusado por sessão ou lançamento de outro jogo (DEL-003).
  // Política de ctx (decisão 7): se ctx já venceu antes de a dona pegar o pedido, fn não roda e devolve ctx.Err();
  // depois que fn começou, espera a resposta sem ctx.
  // Com agente em shutdown: ErrShuttingDown, fn não roda. Erro de fn é devolvido como está.
  func (t *Tracker) WithoutActiveGame(ctx context.Context, gameID int64, fn func() error) error
  ```
- Notas: usa o canal `ctrlCh` com buffer 16 (fase 4), e a verificação é feita com `m.Pending()` na própria goroutine dona. Não cria goroutine nova. Enquanto `fn` roda, `Launch` e `LaunchUntracked` esperam na fila (a dona está ocupada).
- Testes (`tracker_test.go`, `goleak` ativo):
  - `TestWithoutActiveGame`:
    - "DEL-002/sem sessão nem lançamento do jogo executa fn e devolve o erro dela"
    - "DEL-002/sessão Running, Grace ou Paused do mesmo jogo devolve *SessionActiveError com o id da sessão e sem chamar fn"
    - "DEL-002/lançamento pendente do mesmo jogo devolve *SessionActiveError com ID 0 e sem chamar fn"
    - "DEL-003/sessão de outro jogo não impede fn"
    - "DEL-003/lançamento de outro jogo em andamento: fn roda depois que ele termina"
    - "DEL-002/ctx vencido antes de a dona pegar o pedido: fn não roda e devolve ctx.Err()"
    - "DEL-002/fn iniciada e ctx vencido: a espera termina com o resultado de fn"
    - "DEL-002/agente em shutdown devolve ErrShuttingDown sem chamar fn"
    - "DEL-002/depois de Close não sobra goroutine (goleak)"
- Depende de: —

## T16 — ipc: protocolo v0.6.0 e teste de contrato
- Requisitos: IPC-041, IPC-042 (tipos e nomes)
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `internal/ipc/protocol.go` (alterar: métodos novos, `Game` com os campos novos, tipos de resultado; comentário do pacote aponta para `docs/specs/phase-5-library/contracts/ipc.openrpc.json`)
  - `internal/ipc/library.go` (novo: tipos da biblioteca)
  - testes: `internal/ipc/contract_test.go` (alterar: caminho e versão `0.6.0`), e testes de tipos em `internal/ipc/library_types_test.go` (novo)
- Assinaturas públicas:
  ```go
  const (
      MethodGameGet        = "game.get"
      MethodGameUpdate     = "game.update"
      MethodGameDelete     = "game.delete"
      MethodSessionList    = "session.list"
      MethodPlatformList   = "platform.list"
      MethodPlatformAdd    = "platform.add"
      MethodLibraryCatalog = "library.catalog"
  )
  // Game: campos do contrato, com o formato de game.list e de game.add (IPC-020, IPC-042).
  type Game struct {
      ID               int64      `json:"id"`
      Title            string     `json:"title"`
      PlatformID       int64      `json:"platformId"`
      Platform         string     `json:"platform"`
      Path             string     `json:"path"`
      Status           string     `json:"status"`
      Favorite         bool       `json:"favorite"`
      Year             *int       `json:"year"`
      TotalPlaySeconds int64      `json:"totalPlaySeconds"`
      PlayCount        int64      `json:"playCount"`
      FirstPlayedAt    *time.Time `json:"firstPlayedAt"`
      LastPlayedAt     *time.Time `json:"lastPlayedAt"`
  }
  type GameListParams struct { // by-name; sem params = primeira página
      Filter *domain.FilterSpec `json:"filter,omitempty"`
      Sort   *string            `json:"sort,omitempty"`
      Offset *int               `json:"offset,omitempty"`
      Limit  *int               `json:"limit,omitempty"`
  }
  type GameListResult struct {
      Games  []Game `json:"games"` // nunca nil
      Total  int64  `json:"total"`
      Offset int    `json:"offset"`
  }
  type GameDetail struct {
      Game
      Description string   `json:"description"`
      CompletedAt *string  `json:"completedAt"` // YYYY-MM-DD ou null
      Developers  []string `json:"developers"`
      Publishers  []string `json:"publishers"`
      Genres      []string `json:"genres"`
      PlayModes   []string `json:"playModes"`
  }
  type GameDetailResult struct { Game GameDetail `json:"game"` }
  type GameDeletedResult struct { GameID int64 `json:"gameId"` }
  type SessionItem struct {
      SessionID     int64      `json:"sessionId"`
      GameID        int64      `json:"gameId"`
      State         string     `json:"state"`
      StartedAt     time.Time  `json:"startedAt"`
      EndedAt       *time.Time `json:"endedAt"`
      ActiveSeconds int64      `json:"activeSeconds"`
      EndReason     *string    `json:"endReason"`
  }
  type SessionListParams struct { // gameId obrigatório
      GameID int64 `json:"gameId"`
      Offset *int  `json:"offset,omitempty"`
      Limit  *int  `json:"limit,omitempty"`
  }
  type SessionListResult struct {
      Sessions []SessionItem `json:"sessions"`
      Total    int64         `json:"total"`
      Offset   int           `json:"offset"`
  }
  type Platform struct {
      ID       int64  `json:"id"`
      Name     string `json:"name"`
      Category string `json:"category"`
  }
  type PlatformListResult struct { Platforms []Platform `json:"platforms"` }
  type PlatformAddParams struct {
      Name     string `json:"name"`
      Category string `json:"category"`
  }
  type PlatformAddResult struct { Platform Platform `json:"platform"` }
  type Catalog struct {
      Platforms []Platform `json:"platforms"`
      Genres    []string   `json:"genres"`
      Companies []CatalogItem   `json:"companies"`
      PlayModes []string   `json:"playModes"`
  }
  ```
  Limites: `MaxGameTitleRunes` continua em `protocol.go` e o teste confere com `domain.MaxTitleRunes`.
- Testes:
  - `TestContract` (lê `docs/specs/phase-5-library/contracts/ipc.openrpc.json`):
    - "IPC-041/versão 0.6.0"
    - "IPC-041/os sete métodos novos e os antigos existem com os mesmos nomes"
    - "IPC-041/enums sort, playMode, status, played e category batem com o domain"
    - "IPC-041/x-limits batem com as constantes de domain e com MaxGameTitleRunes"
    - "IPC-041/campos obrigatórios de Game, GameDetail, GameListResult, SessionItem, SessionListResult e Catalog batem com as tags Go"
    - "IPC-041/códigos de erro batem com as constantes"
    - "IPC-041/comentário do pacote aponta para phase-5-library"
  - `TestLibraryTypes`:
    - "IPC-042/GameListResult serializa games, total e offset; games nil vira [] (checar no teste de agente, T18)"
    - "IPC-042/Game serializa platformId, platform, status, favorite e year (null quando nil)"
- Depende de: T03

## T17 — ipc: decodificação de params (`Optional[T]`, `game.list`, patch)
- Requisitos: IPC-043, IPC-052
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `internal/ipc/params.go` (alterar: `Optional[T]`)
  - `internal/ipc/library_params.go` (novo: `GameUpdateParams`)
  - testes: `internal/ipc/params_optional_test.go` (novo)
- Assinaturas públicas:
  ```go
  // Optional distingue ausente de null: Present = membro existe; Null = valor null; Value só vale com Present && !Null.
  type Optional[T any] struct {
      Present bool
      Null    bool
      Value   T
  }
  func (o *Optional[T]) UnmarshalJSON(b []byte) error // tipo errado => erro de json (vira -32602 pelo DecodeParams)

  // game.update: by-name; gameId obrigatório; demais membros opcionais (ausente não mexe, null só em CompletedAt e Year).
  type GameUpdateParams struct {
      GameID      int64                `json:"gameId"`
      Title       Optional[string]     `json:"title"`
      PlatformID  Optional[int64]      `json:"platformId"`
      Status      Optional[string]     `json:"status"`
      Favorite    Optional[bool]       `json:"favorite"`
      CompletedAt Optional[string]     `json:"completedAt"`
      Description Optional[string]     `json:"description"`
      Year        Optional[int]        `json:"year"`
      Developers  Optional[[]string]   `json:"developers"`
      Publishers  Optional[[]string]   `json:"publishers"`
      Genres      Optional[[]string]   `json:"genres"`
      PlayModes   Optional[[]string]   `json:"playModes"`
  }
  ```
- Testes:
  - `TestOptional`:
    - "IPC-052/completedAt ausente vira Present = false; completedAt null vira Null = true"
    - "IPC-052/completedAt com string vira Value"
  - `TestDecodeParamsLibrary`:
    - "IPC-043/membro desconhecido dentro de filter é -32602"
    - "IPC-043/array posicional em game.list é -32602"
    - "IPC-043/offset texto, limit 1.5 e version 2 são -32602"
    - "IPC-052/game.update com array posicional é -32602"
    - "IPC-052/membro desconhecido no patch é -32602"
    - "IPC-052/favorite texto é -32602"
    - "IPC-052/genres com item numérico, [1], é -32602"
- Depende de: T16

## T18 — agent: `game.list` paginado e `game.add` com campos novos
- Requisitos: IPC-042, IPC-043, IPC-020 (alterado), IPC-023 (substituído), IPC-008 (alterado), IPC-051 (parte), LIB-002 (via agente)
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `internal/agent/games.go` (alterar: `game.list` com params; `toIPCGame` com os campos novos; remover `Store.Games` do uso)
  - `internal/agent/library.go` (novo: `handleGameList`, conversão de params para `store.Query`)
  - `internal/store/games.go` (remover `Store.Games`, último uso aqui; decisão 2)
  - `internal/ipc/protocol.go` (somente se faltar campo)
  - testes: `internal/agent/library_test.go` (novo), `internal/agent/games_test.go` (ajustar)
- Assinaturas públicas (internas):
  ```go
  // gameStore ganha:
  //   ListGames(ctx context.Context, q store.Query) (store.Page, error)
  // (Games sai da interface.)
  // toIPCGame(g store.Game) ipc.Game: Year = domain.ReleaseYear(g.ReleaseDate); Status, Favorite, PlatformID, Platform.
  // gameListQuery(p ipc.GameListParams) (store.Query, error): Sort padrão title; Offset padrão 0; Limit padrão DefaultPageLimit; Filter Normalize + Validate.
  ```
- Notas: `game.list` com `context.WithTimeout(ctx, 2 s)`; params vazios (`nil`, `{}`) = primeira página. Para o IPC-051, a resposta é medida sem o `\n` final.
- Testes:
  - `TestGameList`:
    - "IPC-042/sem params devolve primeira página de 200 com filtro vazio e ordem title"
    - "IPC-042/{} é igual a sem params"
    - "IPC-042/filter, sort, offset e limit chegam ao store e o resultado traz games, total e offset"
    - "IPC-042/games vazio sai como [] e não null (LIB-003)"
    - "IPC-043/sort desconhecido, limit 0, limit 501, offset -1 e offset texto são -32602 sem consultar o store"
    - "IPC-043/filter com version 2 ou membro desconhecido é -32602"
    - "IPC-008/game.list com array vazio [] é -32602 (params posicionais)"
    - "IPC-051/página de 500 jogos de pior caso (título de 200 runas, caminho de 260, plataforma de 100) cabe em 1 048 576 bytes"
  - `TestGameAddShape`:
    - "IPC-020/game.add devolve Game com platformId, platform, status, favorite e year (null quando não há data)"
- Depende de: T09, T17, T16

## T19 — agent: `game.get` e `session.list`
- Requisitos: IPC-044, IPC-047, IPC-051 (parte)
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `internal/agent/library.go` (alterar: `handleGameGet`, `handleSessionList`)
  - testes: `internal/agent/library_test.go` (alterar)
- Assinaturas públicas (internas):
  ```go
  // gameStore ganha:  GameDetail(ctx, id int64) (store.GameDetail, error)
  //                   GameSessions(ctx, gameID int64, offset, limit int) (store.SessionPage, error)
  // gameDetailToIPC(d store.GameDetail) ipc.GameDetail // listas nunca nil; CompletedAt via domain.FormatCompletedDate
  // sessionItemToIPC(s store.SessionItem) ipc.SessionItem
  ```
- Notas: `game.get` sem timeout extra além do padrão do handler. `session.list`: limit padrão `DefaultSessionLimit`, máximo `MaxSessionLimit`.
- Testes:
  - `TestGameGet`:
    - "IPC-044/gameId existente devolve game com o detalhe completo"
    - "IPC-044/gameId inexistente é -32001"
    - "IPC-044/gameId 0, texto ou params ausentes são -32602"
    - "IPC-051/detalhe com descrição de 10 000 caracteres e 50 nomes de vínculo cabe no limite de 1 MiB"
  - `TestSessionList`:
    - "IPC-047/devolve sessions, total e offset na ordem de STO-027"
    - "IPC-047/limit padrão 50; limit 201 e offset negativo são -32602"
    - "IPC-047/gameId inexistente é -32001"
    - "IPC-047/sessions vazio sai como [] e não null"
- Depende de: T10, T07, T17, T18

## T20 — agent: `game.update`
- Requisitos: IPC-045, IPC-050, IPC-052 (agente), EDT-004, EDT-005, EDT-006, EDT-007, EDT-009
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `internal/agent/library.go` (alterar: `handleGameUpdate`, `patchFromParams`)
  - `internal/domain/patch.go` (novo: `ValidateGameNames`, `ValidatePlayModes` se preciso; regras de DOM-016 e DOM-014 sobre listas)
  - testes: `internal/agent/library_test.go` (alterar), `internal/domain/patch_test.go` (novo)
- Assinaturas públicas (internas):
  ```go
  // gameStore ganha: UpdateGame(ctx context.Context, id int64, p store.GamePatch, at time.Time) (store.GameDetail, error)
  // patchFromParams(p ipc.GameUpdateParams, clock domain.Clock) (store.GamePatch, error): valida (ValidateTitle, ParseGameStatus, ValidateDescription, ParseCompletedDate, WithYear em YearMin..YearMax, NormalizeName por nome, ParsePlayMode, PlatformID > 0); null em campo que não aceita null => ErrInvalidField.
  // Erros de domínio viram -32602; ErrGameNotFound vira -32001; ErrPlatformNotFound vira -32602.
  ```
- Notas: `game.update` usa `clock.Now()` lido ao receber o pedido (ADR 0011); timeout de 5 s; não chama o tracker (EDT-006); nenhuma notificação (IPC-050). Lista duplicada sem diferenciar maiúsculas é removida antes de ir ao store.
- Testes:
  - `TestGameUpdate`:
    - "IPC-045/patch válido devolve game com o detalhe já alterado"
    - "IPC-045/gameId inexistente é -32001 e nada muda"
    - "IPC-045/patch inválido (title vazio) é -32602 e nada muda (EDT-007)"
    - "EDT-007/title vazio junto com developers novos é -32602 e nenhum nome novo é criado"
    - "EDT-009/só gameId devolve o jogo sem alteração e com sucesso"
    - "EDT-009/membro desconhecido é -32602"
    - "EDT-004/playModes com nome fora de DOM-014 é -32602"
    - "EDT-004/developers com nomes repetidos sem diferenciar maiúsculas sai com uma só entrada"
    - "EDT-006/game.update com sessão Running não muda session.current nem envia session.changed"
    - "IPC-050/game.update não envia notificação"
    - "IPC-052/completedAt null apaga e completedAt ausente não mexe"
  - `TestPatchValidation` (domain): "DOM-016/nomes passam por NormalizeName e duplicados saem"
- Depende de: T12, T17, T18

## T21 — agent: `game.delete`
- Requisitos: IPC-046, IPC-050, DEL-001, DEL-002, DEL-003, DEL-004, DEL-005
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `internal/agent/library.go` (alterar: `handleGameDelete`)
  - `internal/agent/games.go` (alterar: `sessionTracker` ganha `WithoutActiveGame`)
  - testes: `internal/agent/library_test.go` (alterar), `internal/agent/fakes_test.go` (alterar: `fakeSessionTracker.WithoutActiveGame`)
- Assinaturas públicas (internas):
  ```go
  // gameStore ganha: DeleteGame(ctx context.Context, id int64) error
  // sessionTracker ganha: WithoutActiveGame(ctx context.Context, gameID int64, fn func() error) error
  // handleGameDelete: st.Game (para o título) => tr.WithoutActiveGame(ctx, id, func() error { return st.DeleteGame(ctx, id) })
  // Mapeamento: ErrGameNotFound => -32001; *tracker.SessionActiveError => -32002 com SessionRef; ErrGameInUse => -32002 com sessionId 0 e título (decisão 6);
  //             ErrShuttingDown ou fim do prazo => -32603. Timeout de 5 s.
  ```
- Testes:
  - `TestGameDelete`:
    - "IPC-046/gameId sem sessão devolve {gameId} e o jogo sai do banco"
    - "IPC-046/gameId inexistente é -32001 e nada muda"
    - "DEL-002/sessão ativa do jogo devolve -32002 com SessionRef (sessionId, gameId, title) e nada muda"
    - "DEL-002/lançamento pendente do jogo devolve -32002 com sessionId 0"
    - "DEL-002/ErrGameInUse do store vira -32002 com sessionId 0"
    - "DEL-003/sessão de outro jogo não impede a remoção e a sessão não muda"
    - "DEL-004/gameId inexistente é -32001"
    - "DEL-005/remoção aceita com processo lançado sem contar tempo ainda aberto; session.current continua null"
    - "IPC-046/agente encerrando devolve -32603 e nada muda"
    - "IPC-050/game.delete não envia notificação"
- Depende de: T13, T15, T18

## T22 — agent: plataformas e catálogo
- Requisitos: IPC-048, IPC-049, IPC-050, EDT-008
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `internal/agent/library.go` (alterar: `handlePlatformList`, `handlePlatformAdd`, `handleLibraryCatalog`)
  - testes: `internal/agent/library_test.go` (alterar)
- Assinaturas públicas (internas):
  ```go
  // gameStore ganha:
  //   Platforms(ctx context.Context) ([]store.Platform, error)
  //   AddPlatform(ctx context.Context, name string, category domain.PlatformCategory) (store.Platform, error)
  //   Catalog(ctx context.Context) (store.Catalog, error)
  // platform.add: NormalizeName (DOM-016) e ParsePlatformCategory (DOM-015); inválidos => -32602.
  ```
- Testes:
  - `TestPlatformAndCatalog`:
    - "IPC-048/platform.list devolve platforms em ordem de nome"
    - "IPC-048/platform.add com nome existente em outra grafia devolve a existente"
    - "IPC-048/platform.add com nome novo devolve a plataforma criada"
    - "EDT-008/nome vazio ou categoria inválida é -32602 e nada muda"
    - "IPC-049/library.catalog devolve listas não nulas e exatamente os cinco playModes"
    - "IPC-050/platform.add não envia notificação"
- Depende de: T14, T17, T18

## T23 — cmd/arcadia: leituras da biblioteca e remoção de `LibrarySummary`
- Requisitos: DSK-016, DSK-019, DSK-008 (alterado)
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `cmd/arcadia/library.go` (alterar: `GameView` com campos novos; `ListGames(q)`, `GetGame`, `GameSessions`; remover `LibrarySummary`, `LibrarySummaryView` e `libraryGate`/`storeOpener`)
  - `cmd/arcadia/app.go` (alterar: sem `gate`; ordem de shutdown sem `libraryGate.Close`)
  - `internal/store/store.go` (remover `LibrarySummary` e o método)
  - `frontend/wailsjs/` (regerado com **`cd cmd/arcadia && wails generate module`**)
  - testes: `cmd/arcadia/library_test.go` (alterar), `cmd/arcadia/app_test.go` (alterar)
- Assinaturas públicas:
  ```go
  type LibraryQuery struct {
      Filter json.RawMessage `json:"filter"` // FilterSpec como JSON; vazio = {"version":1}
      Sort   string          `json:"sort"`
      Offset int             `json:"offset"`
      Limit  int             `json:"limit"`
  }
  type GameView struct {
      ID               int64  `json:"id"`
      PlatformID       int64  `json:"platformId"`
      Title            string `json:"title"`
      Platform         string `json:"platform"`
      Path             string `json:"path"`
      Status           string `json:"status"`
      Favorite         bool   `json:"favorite"`
      Year             *int   `json:"year"`
      TotalPlaySeconds int64  `json:"totalPlaySeconds"`
      PlayCount        int64  `json:"playCount"`
      FirstPlayedAt    string `json:"firstPlayedAt"` // RFC 3339 ou ""
      LastPlayedAt     string `json:"lastPlayedAt"`
  }
  type GameListView struct {
      Games  []GameView `json:"games"`
      Total  int64      `json:"total"`
      Offset int        `json:"offset"`
  }
  type GameDetailView struct { /* campos de GameView + Description, CompletedAt (YYYY-MM-DD ou ""), Developers, Publishers, Genres, PlayModes */ }
  type SessionItemView struct { /* SessionID, GameID, State, StartedAt, EndedAt, ActiveSeconds, EndReason */ }
  type SessionListView struct { Sessions []SessionItemView `json:"sessions"`; Total int64; Offset int }

  func (a *App) ListGames(q LibraryQuery) (GameListView, error)            // game.list, 2 s
  func (a *App) GetGame(id int64) (GameDetailView, error)                   // game.get, 5 s
  func (a *App) GameSessions(id int64, offset, limit int) (SessionListView, error) // session.list, 5 s
  ```
  Parte de DSK-016: `ListGames` tem prazo de 5 s. Erros seguem `userError` (DSK-011).
- Testes:
  - `TestLibraryReads`:
    - "DSK-016/ListGames repassa filter, sort, offset e limit ao agente e converte para GameView sem calcular tempo"
    - "DSK-016/GetGame devolve o detalhe com listas nunca nil"
    - "DSK-016/GameSessions repassa offset e limit e devolve sessions nunca nil"
    - "DSK-016/erro do agente sai como mensagem de DSK-011"
  - `TestLibrarySummaryRemoved` (lê arquivos do repositório):
    - "DSK-019/LibrarySummary e LibrarySummaryView não existem em cmd/arcadia nem em internal/store"
    - "DSK-019/wailsjs não tem LibrarySummary"
  - `TestLibraryGateRemoved`: "DSK-008/libraryGate e storeOpener foram removidos; a UI não abre o banco (ADR 0015)"
- Depende de: T18, T19

## T24 — cmd/arcadia: escritas da biblioteca e mensagem de remoção
- Requisitos: DSK-017, DSK-018
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `cmd/arcadia/library.go` (alterar: `UpdateGame`, `DeleteGame`, `Catalog`, `AddPlatform`)
  - `cmd/arcadia/errors.go` (alterar: mensagem de `-32002` em `DeleteGame`)
  - `frontend/wailsjs/` (regerado com **`cd cmd/arcadia && wails generate module`**)
  - testes: `cmd/arcadia/library_test.go` (alterar), `cmd/arcadia/errors_test.go` (alterar)
- Assinaturas públicas:
  ```go
  // UpdateGame recebe o patch cru (json.RawMessage) para preservar ausente ≠ null (decisão 10); envia game.update (5 s) e devolve GameDetailView.
  func (a *App) UpdateGame(id int64, patch json.RawMessage) (GameDetailView, error)
  // DeleteGame envia game.delete (5 s) e devolve só o erro.
  func (a *App) DeleteGame(id int64) error
  // Catalog envia library.catalog (5 s).
  func (a *App) Catalog() (CatalogView, error)
  // AddPlatform envia platform.add (5 s).
  func (a *App) AddPlatform(name, category string) (PlatformView, error)
  ```
  DSK-018: `DeleteGame` com `-32002` mostra "Não é possível remover: o jogo está em uma sessão ativa. Feche a sessão antes de remover.".
- Testes:
  - `TestLibraryWrites`:
    - "DSK-017/UpdateGame envia game.update com o patch cru e devolve o detalhe"
    - "DSK-017/DeleteGame envia game.delete e não devolve valor"
    - "DSK-017/Catalog e AddPlatform chamam library.catalog e platform.add e devolvem views"
    - "DSK-017/a UI não grava no banco nem emite evento (sem caminho de escrita)"
  - `TestDeleteMessage`:
    - "DSK-018/-32002 em DeleteGame mostra exatamente a mensagem de DSK-018, sem o título"
    - "DSK-018/demais códigos em DeleteGame seguem DSK-011"
- Depende de: T20, T21, T22, T23

## T25 — frontend: dependência `@tanstack/react-virtual` (AGUARDA APROVAÇÃO)
- Requisitos: suporte a UI-029, UI-030 (sem requisito próprio)
- Responsável: implementer · Testes: nenhum (verificação: `npm install` e `npm run build`)
- Arquivos:
  - `frontend/package.json` e `frontend/package-lock.json` (alterar: `"@tanstack/react-virtual"` em `dependencies`)
- Notas: **o usuário precisa aprovar esta dependência antes da instalação.** Motivo: virtualização de 10 000 cards sem reimplementar o cálculo de janela (UI-029); alternativa é escrever a janela à mão. Versão a fixar: a mais nova compatível com React 19 na data da instalação.
- Depende de: — (bloqueado pela aprovação)

## T26 — frontend: filtros, ordenação, debounce
- Requisitos: UI-032 (parte: montagem do FilterSpec), UI-033, UI-034 (estado), UI-035 (reset)
- Responsável: implementer · Testes: test-writer (`*.test.ts`)
- Arquivos:
  - `frontend/src/lib/filters.ts` (novo)
  - `frontend/src/lib/debounce.ts` (novo)
  - testes: `frontend/src/lib/filters.test.ts`, `frontend/src/lib/debounce.test.ts` (novos)
- Assinaturas públicas:
  ```ts
  export type FilterSpec = {
    version: 1
    platformIds?: number[]; genreIds?: number[]; developerIds?: number[]; publisherIds?: number[]
    playMode?: 'all' | 'single' | 'multi' | 'coop'
    yearFrom?: number | null; yearTo?: number | null
    statuses?: GameStatus[]; favorite?: boolean | null
    played?: 'any' | 'played' | 'notPlayed'
    playSecondsMin?: number | null; playSecondsMax?: number | null
    text?: string
  }
  export type GameStatus = 'not_played' | 'playing' | 'completed' | 'abandoned' | 'backlog'
  export type SortKey = 'title' | 'lastPlayed' | 'totalPlayed' | 'year'
  export type LibraryQuery = { filter: FilterSpec; sort: SortKey }
  export const defaultQuery: LibraryQuery // { filter: { version: 1 }, sort: 'title' }
  export function resetQuery(): LibraryQuery // volta ao padrão (UI-035)
  export function withText(q: LibraryQuery, text: string): LibraryQuery // text vazio ou só espaços remove o filtro (UI-033)
  export type Timers = { setTimeout: (f: () => void, ms: number) => unknown; clearTimeout: (h: unknown) => void }
  export function createDebouncer<T>(ms: number, apply: (v: T) => void, timers?: Timers): { call(v: T): void; cancel(): void }
  ```
- Testes:
  - `filters.test.ts`:
    - "UI-032/cada faceta entra no FilterSpec e lista vazia ou nula não aparece"
    - "UI-035/resetQuery volta a {version: 1} e sort title"
    - "UI-033/withText com texto vazio ou só espaços tira o filtro text"
    - "UI-034/withText e troca de sort preservam os outros filtros"
  - `debounce.test.ts` (fake timers):
    - "UI-033/a consulta só é aplicada 300 ms depois da última tecla"
    - "UI-033/uma tecla por vez não dispara consulta intermediária"
    - "UI-033/cancel remove o temporizador pendente (sem aplicar após desmontar)"
- Depende de: —

## T27 — frontend: consultas e mutações da biblioteca
- Requisitos: UI-036 (parte: `getNextPageParam` e troca de query), UI-042, UI-044 (parte: retry de 2 s), UI-009 (reconexão)
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `frontend/src/lib/library.ts` (novo)
  - `frontend/src/lib/games.ts` (alterar: `gamesKey` passa a ser `libraryKeys.games`; os `GameView` e `listGames` antigos saem na T34)
  - testes: `frontend/src/lib/library.test.tsx` (novo)
- Assinaturas públicas:
  ```ts
  export const libraryKeys = {
    games: (q: LibraryQuery) => ['library', 'games', q] as const,
    game: (id: number) => ['library', 'game', id] as const,
    sessions: (id: number) => ['library', 'sessions', id] as const,
    catalog: ['library', 'catalog'] as const,
  }
  export type GameView = { id: number; platformId: number; title: string; platform: string; path: string; status: GameStatus; favorite: boolean; year: number | null; totalPlaySeconds: number; playCount: number; firstPlayedAt: string; lastPlayedAt: string }
  export type GameListView = { games: GameView[]; total: number; offset: number }
  export function useInfiniteGames(q: LibraryQuery): UseInfiniteQueryResult<InfiniteData<GameListView>, Error> // pageParam = offset; getNextPageParam: loaded < total ? loaded : undefined; retry a cada 2 s em erro
  export function useGame(id: number): UseQueryResult<GameDetailView, Error>
  export function useGameSessions(id: number): UseInfiniteQueryResult<InfiniteData<SessionListView>, Error> // 50 por página
  export function useCatalog(): UseQueryResult<CatalogView, Error>
  export function useUpdateGame(): UseMutationResult<GameDetailView, Error, { id: number; patch: Record<string, unknown> }>
  export function useDeleteGame(): UseMutationResult<void, Error, number>
  export function useAddPlatform(): UseMutationResult<PlatformView, Error, { name: string; category: string }>
  ```
  Notas: `useUpdateGame`, `useDeleteGame`, `useAddPlatform` e o `AddGame` existente invalidam `libraryKeys.games` (todas as chaves com o prefixo `['library','games']`), `game` e `catalog` ao concluir (UI-042). A troca de `query` descarta as páginas (UI-036).
- Testes:
  - `library.test.tsx`:
    - "UI-036/getNextPageParam devolve loaded enquanto loaded < total e undefined depois"
    - "UI-036/mudar a query cria uma nova chave e descarta as páginas carregadas"
    - "UI-042/useUpdateGame concluído invalida games, game e catalog"
    - "UI-042/useDeleteGame e useAddPlatform também invalidam games e catalog"
    - "UI-044/a lista com erro é pedida de novo a cada 2 s"
    - "UI-009/reconexão recarrega games e a sessão atual"
- Depende de: T26

## T28 — frontend: grade virtualizada e cards
- Requisitos: UI-029, UI-031, UI-036 (parte: pré-busca), UI-043, UI-006 (alterado), UI-019 e UI-021 (alterados), UI-004 (substituído)
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `frontend/src/components/LibraryGrid.tsx` (novo; `useVirtualizer` de `@tanstack/react-virtual`)
  - `frontend/src/components/GameCard.tsx` (novo)
  - testes: `frontend/src/components/LibraryGrid.test.tsx`, `frontend/src/components/GameCard.test.tsx` (novos)
- Assinaturas públicas:
  ```ts
  export function LibraryGrid(props: {
    games: GameView[]            // páginas já carregadas, na ordem do agente
    total: number
    current: SessionView | null
    pending: PendingLaunch
    onNeedMore: () => void       // pré-busca (UI-036)
    onOpen: (id: number) => void
    onPlay: (id: number) => void
    onPlayUntracked: (id: number) => void
  }): JSX.Element
  export function GameCard(props: { game: GameView; live: 'running' | 'paused' | null; busy: 'tracked' | 'untracked' | null; onOpen; onPlay; onPlayUntracked }): JSX.Element
  ```
  `PendingLaunch` continua em `GameList.tsx` até a T34. `live` vem de `liveLabel` (UI-006/UI-043): "Em execução" para running e grace, "Pausado" para paused, nada para ended e discarded.
- Notas: pré-busca quando o último índice visível passa de `loaded - 40`, uma vez por página (flag em ref). Só o `useVirtualizer` recebe o tamanho (sem `setTimeout`). Desmontar limpa o observador.
- Testes (com `getBoundingClientRect` e `ResizeObserver` mockados; 10 000 itens sintéticos):
  - `LibraryGrid.test.tsx`:
    - "UI-029/10 000 jogos: no máximo 100 cards no DOM com altura e largura fixas"
    - "UI-029/rolar muda a janela sem chamar a rede (nenhum onNeedMore fora da pré-busca)"
    - "UI-036/pré-busca chama onNeedMore uma única vez ao passar de loaded - 40"
  - `GameCard.test.tsx`:
    - "UI-029/card mostra título, plataforma e tempo total no formato de UI-005"
    - "UI-006/card com sessão running ou grace mostra 'Em execução'; paused mostra 'Pausado'"
    - "UI-043/marcação some quando a sessão vira ended ou discarded"
    - "UI-031/card tem os botões 'Jogar' e 'Sem contar tempo'"
    - "UI-031/pendência de um jogo desabilita os dois botões e mostra 'Iniciando…' no acionado"
    - "UI-019/UI-021/botões do card seguem o bloqueio de pendência da fase 4"
- Depende de: T25, T27

## T29 — frontend: alternância grade e lista
- Requisitos: UI-030, UI-024 (alterado)
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `frontend/src/components/LibraryList.tsx` (novo; mesmo virtualizador, uma coluna, linhas)
  - `frontend/src/components/ViewToggle.tsx` (novo)
  - `frontend/src/lib/libraryView.ts` (novo: lê e grava `localStorage['arcadia.libraryView']`)
  - testes: `frontend/src/components/LibraryList.test.tsx`, `ViewToggle.test.tsx`, `frontend/src/lib/libraryView.test.ts` (novos)
- Assinaturas públicas:
  ```ts
  export type LibraryView = 'grid' | 'list'
  export function readLibraryView(storage?: Pick<Storage, 'getItem'>): LibraryView // padrão 'grid'
  export function writeLibraryView(v: LibraryView, storage?: Pick<Storage, 'setItem'>): void
  export function ViewToggle(props: { view: LibraryView; onChange: (v: LibraryView) => void }): JSX.Element
  export function LibraryList(props: Omit<Parameters<typeof LibraryGrid>[0], never>): JSX.Element
  ```
- Testes:
  - "UI-030/clicar no alternador muda de grade para lista e de volta"
  - "UI-030/lista mostra a mesma página, os mesmos dados e os mesmos botões da grade"
  - "UI-030/a escolha fica guardada em localStorage e volta a valer ao abrir de novo; sem valor salvo é grade"
  - "UI-024/linha de jogo pausado mostra 'Pausado'; jogo em execução mostra 'Em execução'"
- Depende de: T28

## T30 — frontend: painel de filtros, busca, ordenação e estados
- Requisitos: UI-032, UI-033, UI-034, UI-035, UI-044, UI-010 (substituído)
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `frontend/src/components/FilterPanel.tsx` (novo)
  - `frontend/src/components/SearchBox.tsx` (novo; usa `createDebouncer`, cancela no desmonte)
  - `frontend/src/components/SortSelect.tsx` (novo)
  - `frontend/src/components/EmptyResults.tsx` (novo)
  - `frontend/src/components/LibraryStates.tsx` (novo: "Conectando ao agente…", "Nenhum jogo ainda" com "Adicionar jogo", contagem "N jogos")
  - testes: `FilterPanel.test.tsx`, `SearchBox.test.tsx`, `SortSelect.test.tsx`, `EmptyResults.test.tsx`, `LibraryStates.test.tsx` (novos)
- Assinaturas públicas:
  ```ts
  export function FilterPanel(props: { query: LibraryQuery; catalog: CatalogView | undefined; onChange: (q: LibraryQuery) => void }): JSX.Element
  export function SearchBox(props: { text: string; onChange: (text: string) => void; timers?: Timers }): JSX.Element
  export function SortSelect(props: { sort: SortKey; onChange: (s: SortKey) => void }): JSX.Element // "Título", "Última vez jogado", "Tempo jogado", "Ano"
  export function EmptyResults(props: { onClear: () => void }): JSX.Element // "Nenhum jogo com estes filtros." e "Limpar filtros"
  export function LibraryStates(props: { loaded: boolean; total: number | undefined; filtered: boolean; onAdd: () => void }): JSX.Element | null
  ```
  Contador: "N jogos" com `total`; nenhum contador de plataformas (UI-044).
- Testes:
  - "UI-032/cada faceta tem um controle e a mudança chama onChange com um FilterSpec novo (version 1)"
  - "UI-032/listas de opções vêm do catálogo (plataforma, gênero, modo, desenvolvedora, editora)"
  - "UI-033/SearchBox envia a consulta 300 ms depois da última tecla e apagar o texto remove o filtro"
  - "UI-034/escolher ordenação recarrega com o sort correspondente e volta para o início da página"
  - "UI-035/'Limpar filtros' volta o FilterSpec para {version: 1} e a ordenação para title"
  - "UI-035/total 0 com filtro mostra 'Nenhum jogo com estes filtros.' e o botão 'Limpar filtros'"
  - "UI-044/sem resposta ainda mostra 'Conectando ao agente…'"
  - "UI-044/total 0 sem filtro mostra 'Nenhum jogo ainda' e 'Adicionar jogo'"
  - "UI-044/total > 0 mostra 'N jogos' e nenhum contador de plataformas"
- Depende de: T26, T27

## T31 — frontend: página do jogo e sessões
- Requisitos: UI-037, UI-038, UI-006 (alterado)
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `frontend/src/components/GamePage.tsx` (novo)
  - `frontend/src/components/SessionsList.tsx` (novo; usa `SessionPanel` da fase 4 para a sessão aberta)
  - `frontend/src/lib/formatDate.ts` (novo: data e hora local para primeira e última vez; data UTC para conclusão)
  - testes: `GamePage.test.tsx`, `SessionsList.test.tsx`, `formatDate.test.ts` (novos)
- Assinaturas públicas:
  ```ts
  export function GamePage(props: { id: number; current: SessionView | null; onBack: () => void; onEdit: () => void; onRemove: () => void }): JSX.Element
  export function SessionsList(props: { gameId: number; current: SessionView | null }): JSX.Element
  export function formatLocalDateTime(iso: string): string // "Nunca" quando vazio
  export function formatUtcDate(iso: string): string       // YYYY-MM-DD em UTC (decisão h)
  ```
- Notas: nenhum `<img>` nem `<video>` na página (fase 7). A sessão aberta usa o cronômetro da fase 4 com `activeMs` recebido (UI-014, UI-015, UI-022); a UI não soma nem subtrai tempo.
- Testes:
  - `GamePage.test.tsx`:
    - "UI-037/mostra título, plataforma, gêneros, modos, desenvolvedora, editora, ano, tempo total, status, favorito e descrição"
    - "UI-037/primeira e última vez jogado mostram 'Nunca' quando vazias e data local quando há"
    - "UI-037/data de conclusão sai em UTC"
    - "UI-037/a página não tem img nem video"
    - "UI-037/'Voltar' chama onBack; a marcação de sessão aparece no topo quando o jogo está em sessão"
    - "UI-006/marcação de sessão aparece no topo da página quando o jogo está em sessão"
  - `SessionsList.test.tsx`:
    - "UI-038/lista com 50 por página e 'Carregar mais' pede a próxima página"
    - "UI-038/sessão discarded mostra 'Descartada' e o tempo ativo, sem somar no total"
    - "UI-038/sessão aberta mostra 'Em andamento' e o cronômetro a partir de activeMs"
    - "UI-038/cada linha mostra início, fim, tempo ativo em UI-005 e estado"
- Depende de: T27

## T32 — frontend: edição do jogo
- Requisitos: UI-039, UI-040
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `frontend/src/components/GameEditForm.tsx` (novo)
  - `frontend/src/lib/validation.ts` (novo: título, ano, data `AAAA-MM-DD`, descrição)
  - `frontend/src/lib/gamePatch.ts` (novo: `diffPatch`)
  - testes: `GameEditForm.test.tsx`, `validation.test.ts`, `gamePatch.test.ts` (novos)
- Assinaturas públicas:
  ```ts
  export function diffPatch(original: GameDetailView, draft: EditDraft): Record<string, unknown> // só os campos que mudaram; completedAt null quando apagado
  export function validateDraft(d: EditDraft): Partial<Record<keyof EditDraft, string>> // mensagens; {} quando válido
  export function GameEditForm(props: { game: GameDetailView; onSave: (patch: Record<string, unknown>) => void; onCancel: () => void; pending: boolean; error: string | null }): JSX.Element
  ```
- Testes:
  - `gamePatch.test.ts`:
    - "UI-039/diffPatch envia só os campos alterados"
    - "UI-039/completedAt apagado vira null; campo igual não entra"
  - `validation.test.ts`:
    - "UI-040/título vazio depois de cortar, ano fora de 1900..2100, data que não é AAAA-MM-DD e descrição acima de 10 000 geram mensagem"
  - `GameEditForm.test.tsx`:
    - "UI-039/Salvar envia só o patch e fica desabilitado durante o envio"
    - "UI-039/erro mostra a mensagem e mantém o formulário com os valores digitados"
    - "UI-039/Cancelar descarta as alterações sem chamar onSave"
    - "UI-040/com erro de validação, Salvar fica desabilitado e nada é enviado"
- Depende de: T31

## T33 — frontend: remoção com confirmação
- Requisitos: UI-041
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `frontend/src/components/DeleteDialog.tsx` (novo; diálogo modal com foco preso)
  - testes: `frontend/src/components/DeleteDialog.test.tsx` (novo)
- Assinaturas públicas:
  ```ts
  export function DeleteDialog(props: { title: string; pending: boolean; error: string | null; onConfirm: () => void; onCancel: () => void }): JSX.Element
  ```
  Texto exato: "Remover «título»? As sessões e as pausas deste jogo também serão apagadas. Esta ação não pode ser desfeita." com os botões "Remover" e "Cancelar".
- Testes:
  - "UI-041/mostra o texto exato com o título entre « »"
  - "UI-041/Cancelar, Esc e fechar não chamam onConfirm"
  - "UI-041/Remover chama onConfirm uma vez; durante o envio o botão fica desabilitado"
  - "UI-041/erro (-32002, mensagem de DSK-018) aparece no diálogo com aviso, e o diálogo fecha pelo pai"
- Depende de: T31

## T34 — frontend: `App`, atualização da grade e remoção dos componentes de resumo
- Requisitos: UI-045, UI-042 (integração), UI-004 (substituído), UI-010 (substituído)
- Responsável: implementer · Testes: test-writer
- Arquivos:
  - `frontend/src/App.tsx` (alterar: grade no lugar da lista, `selectedGameId`, `GamePage`, `DeleteDialog`, `GameEditForm`; sem `LibrarySummary`/`LibraryEmpty`)
  - `frontend/src/lib/games.ts` (alterar: sai `useGames`/`listGames`/`GameView` antigos; ficam as partes de sessão)
  - remover: `frontend/src/components/LibraryEmpty.tsx`, `LibraryEmpty.test.tsx`, `frontend/src/components/GameList.tsx`, `GameList.test.tsx` (a grade substitui a lista)
  - testes: `frontend/src/App.test.tsx` (alterar), `frontend/src/lib/games.test.tsx` (alterar), `frontend/src/library-removed.test.ts` (novo: leitura de arquivos)
- Assinaturas públicas: nenhuma nova exportada.
- Testes:
  - `library-removed.test.ts`:
    - "UI-045/nenhum arquivo de frontend/src referencia LibraryEmpty, LibrarySummaryView ou useLibrarySummary"
  - `App.test.tsx`:
    - "UI-042/após UpdateGame e DeleteGame a grade e o catálogo são recarregados uma vez"
    - "UI-042/após AddGame a grade é recarregada"
    - "UI-004/a biblioteca mostra a grade de cards no lugar da lista (substitui o teste de UI-004)"
    - "UI-037/clicar no título de um card abre a página do jogo e 'Voltar' volta à grade"
- Depende de: T28, T30, T32, T33, T24

## T35 — verificação final, documentação e UI-046 manual
- Requisitos: UI-046 (manual)
- Responsável: implementer (documentação e comandos) · revisão manual pela sessão principal com o usuário
- Arquivos:
  - `docs/ARCHITECTURE.md` (alterar: §5 com `game_fts` mantido pelo agente e a grade; §4 com as leituras por IPC)
  - `CLAUDE.md` (alterar: layout e spec atual apontando para `phase-5-library`)
  - `docs/specs/phase-5-library/tasks.md` (esta planilha, com status final)
- Comandos:
  - `powershell -NoProfile -ExecutionPolicy Bypass -File scripts/spec-trace.ps1 -Spec docs/specs/phase-5-library` (sem `-SkipTests`)
  - `go build ./...`, `go test ./...`, `go vet ./...`, `GOOS=linux go vet ./...`
  - `cd frontend && npm run build && npx vitest run`
  - `cd cmd/arcadia && wails build -ldflags "-X main.version=<v>"`
- Testes: UI-046 (manual, com biblioteca real de alguns milhares de jogos): rolagem fluida; filtro por plataforma e gênero com total certo; busca por termo do título e da descrição; edição de título, ano e gêneros aparecendo na grade; página com as sessões; remoção de jogo de teste com a confirmação de UI-041 e saída da grade. `· verificação: manual`.
- Depende de: T01–T34

---

## Riscos de concorrência e recursos

- **Goroutines novas:** nenhuma. `WithoutActiveGame` roda `fn` na goroutine dona do tracker (`run`), que já existe, e o agente só usa os handlers de IPC já existentes. O frontend não cria goroutine (o TanStack Query e os temporizadores são da UI). Todo `setTimeout` novo (debounce de busca, pré-busca) tem dono e é cancelado no desmonte (`createDebouncer.cancel`).
- **Dona do tracker ocupada com `DeleteGame`:** durante `fn`, `Launch` e `LaunchUntracked` esperam na fila de `launchCh` e `ctrlCh` (buffer 16). A transação de remoção é curta, mas o `busy_timeout` de 5 s pode segurá-la. Os chamadores têm `ctx` (`launchGameTimeout` 10 s no agente, 5 s na remoção), então a fila tem limite. Ver Problema 2.
- **Ordem de shutdown:** `Close` do tracker espera a dona terminar (como na fase 4). Com `BeginShutdown`, `WithoutActiveGame` devolve `ErrShuttingDown` e `fn` não roda. A ordem de `cmd/arcadia` perde `libraryGate.Close` (T23); o `store` da UI deixa de existir, então não há handle para fechar nesse caminho.
- **Handles do Windows:** nenhum novo. A remoção, o FTS e as listagens são SQLite (`modernc.org/sqlite`, pura Go). Nada de Win32 entra em `domain` ou `store`.
- **SQLite e locks:** `_txlock=immediate` vale para toda transação, inclusive a de leitura de `ListGames` (página e `COUNT` na mesma transação). Isso segura o lock de escrita pelo tempo de duas consultas. Aceitável para uma biblioteca de milhares de jogos (a meta é ≤ 100 ms por página), mas o tracker pode esperar. Os limites de `ListGames` são 200/500 linhas. A consistência entre página e total vale durante a transação, e o design aceita o instante antigo (limitação 6 da spec).
- **Migração 5:** `INSERT … SELECT` com subconsultas correlacionadas, numa transação só. Para dezenas de milhares de jogos, o custo é linear (chaves primárias de vínculo). O `Down` só apaga o índice.
- **Remoção × sessão recuperada:** a recuperação de sessões abertas roda no início do agente (fase 3). Nenhum jogo é removido com sessão aberta por construção (`HasOpenSession` + tracker).
- **Frontend:** a grade depende de `ResizeObserver` (mockado em teste). Desmontar a grade ou a página remove o observador. Os dados da grade são páginas em memória (`useInfiniteQuery`); o limite de memória é o número de páginas carregadas, não há cache externo.

## Rastreabilidade (requisito → tarefas)

| Requisito | Tarefas |
|---|---|
| DOM-012 | T01 |
| DOM-013 | T02 |
| DOM-014 | T01 |
| DOM-015 | T01 |
| DOM-016 | T01, T20 |
| FLT-001 | T03, T16, T17, T18 |
| FLT-002 | T08 |
| FLT-003 | T08 |
| FLT-004 | T08 |
| FLT-005 | T08 |
| FLT-006 | T08 |
| FLT-007 | T08 |
| FLT-008 | T08 |
| FTS-001 | T05 |
| FTS-002 | T05 |
| FTS-003 | T12 |
| FTS-004 | T13 |
| FTS-005 | T06 |
| LIB-001 | T03, T09 |
| LIB-002 | T09, T18 |
| LIB-003 | T09, T18 |
| EDT-001 | T02, T11 |
| EDT-002 | T11 |
| EDT-003 | T02, T11 |
| EDT-004 | T12, T20 |
| EDT-005 | T11, T20 |
| EDT-006 | T20 |
| EDT-007 | T11, T12, T20 |
| EDT-008 | T14, T22 |
| EDT-009 | T20 |
| DEL-001 | T13, T21 |
| DEL-002 | T15, T21 |
| DEL-003 | T15, T21 |
| DEL-004 | T21 |
| DEL-005 | T21 |
| DEL-006 | T13 |
| STO-024 | T04 |
| STO-025 | T08, T09 |
| STO-026 | T07 |
| STO-027 | T10 |
| STO-028 | T11, T12 |
| STO-029 | T13 |
| STO-030 | T14 |
| IPC-041 | T16 |
| IPC-042 | T16, T18 |
| IPC-043 | T17, T18 |
| IPC-044 | T19 |
| IPC-045 | T20 |
| IPC-046 | T21 |
| IPC-047 | T19 |
| IPC-048 | T14, T22 |
| IPC-049 | T22 |
| IPC-050 | T20, T21, T22 |
| IPC-051 | T18, T19 |
| IPC-052 | T17, T20 |
| DSK-016 | T23 |
| DSK-017 | T24 |
| DSK-018 | T24 |
| DSK-019 | T23 |
| UI-029 | T28 |
| UI-030 | T29 |
| UI-031 | T28 |
| UI-032 | T26, T30 |
| UI-033 | T26, T30 |
| UI-034 | T26, T30 |
| UI-035 | T26, T30 |
| UI-036 | T27, T28 |
| UI-037 | T31 |
| UI-038 | T31 |
| UI-039 | T32 |
| UI-040 | T32 |
| UI-041 | T33 |
| UI-042 | T27, T34 |
| UI-043 | T28 |
| UI-044 | T27, T30 |
| UI-045 | T34 |
| UI-046 | T35 (manual) |
| **Alterados ou substituídos (fases anteriores)** | |
| STO-001 (alterado) | T04 |
| AST-001 (alterado) | T04 |
| GAM-001 (alterado, FTS-002) | T05, T07 |
| GAM-005 (alterado, vira LIB-001) | T09 |
| IPC-008 (alterado) | T18 |
| IPC-020 (alterado: `game.add` devolve o formato novo de `Game`) | T18 |
| IPC-023 (substituído por IPC-042) | T18 |
| IPC-028 (alterado) | T17 |
| IPC-032 (substituído por IPC-041) | T16 |
| UI-004 (substituído) | T28, T34 |
| UI-006 (alterado) | T28, T31 |
| UI-010 (substituído por UI-044) | T30 |
| UI-019 / UI-021 (alterados) | T28 |
| UI-024 (alterado) | T29 |
| DSK-008 (alterado; ver Problema 6) | T23 |
| P5 (remover e editar jogo) | T11–T14, T20–T24, T28–T34 |

Requisitos sem tarefa: nenhum. Todos os 77 IDs novos aparecem acima; UI-046 é a única verificação manual.
