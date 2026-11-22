# Fase 15 — Big Box v2: tarefas

- **Status:** gerado pelo planner em 2026-11-22, sobre os requisitos aprovados na mesma data (resposta 1: reserva do IGDB incluída).
- **Requisitos:** [requirements.md](requirements.md) (ATB-001..ATB-016, TTB-001..TTB-040, BBX-046..BBX-059, MOS-001..MOS-016, ETN-001..ETN-015) · **Design:** [design.md](design.md) · **Contratos:** [contracts/ipc-v0.16.0-delta.md](contracts/ipc-v0.16.0-delta.md), [contracts/ipc-v0.16.0-delta.openrpc.json](contracts/ipc-v0.16.0-delta.openrpc.json), [contracts/hltb-rules.md](contracts/hltb-rules.md), [contracts/bigbox-temas-v2.md](contracts/bigbox-temas-v2.md) e `contracts/ipc.openrpc.json` (v0.16.0, criado na T10).
- **ADRs:** 0014, 0018, 0023, 0024, 0029, 0031, 0033, 0035, 0036 (existentes); 0038, 0039, 0040 (propostos, escritos na T37).

## Resumo

Oito fatias entregáveis, cada uma com seus testes antes do código:

- **A (T01..T04):** migração 10, store e funções puras de `domain` do tempo para zerar.
- **B (T05..T18):** conferência do HowLongToBeat, clientes (HowLongToBeat e `game_time_to_beats` do IGDB), IPC v0.16.0 e contrato JSON, serviço `timeToBeat` do agente, bindings Wails e `lib/` TypeScript.
- **C (T19..T20):** blocos do tempo para zerar no painel do modo normal, em "Editar jogo" e na janela dos provedores.
- **D (T21..T28):** Select + A (poller, roteador, abridor), `--bigbox`, segunda instância, `TakeBigBoxRequest`, `App.tsx` e BBX-057.
- **E (T29..T32):** contrato de tema v2, grade 2D, janela e pré-busca, fundo e vídeo por tema, `TimeToBeatList`. O Faixa é migrado sem mudar de comportamento.
- **F (T33..T34):** tema Mosaico.
- **G (T35..T36):** tema Estante, seletor de temas e `uiPrefs`.
- **H (T37..T38):** documentação, ADRs, versão, verificação final e roteiro manual.

Nada muda em `internal/tracker/`, `play_session`, `pause_interval`, `tracking_rule`, `media` ou `media_job`.

## Dúvidas e lacunas da spec (nenhuma bloqueia; as marcadas "spec-writer" pedem ajuste de texto)

1. **TTB-029 e os métodos da fase 14 (spec-writer).** O texto diz que a v0.16.0 só acrescenta `timeToBeat.*`, `timeToBeat.changed`, os esquemas e `x-timetobeat`. Mas `media.add`, `media.remove` e `game.setPlayTime` não estão em nenhum JSON, porque a fase 14 não gerou contrato e o design manda gerar a v0.16.0 "a partir da v0.15.0, dos acréscimos da fase 14 e do delta". Logo, o teste "só acréscimos" da T08 aceita **também** esses três métodos (e os esquemas que a fase 14 usa) como acréscimos. Sugestão: ajustar TTB-029 para citar a fase 14. Os tipos Go de `internal/ipc/manual_edit.go` são a fonte dos esquemas.
2. **Endpoint do HowLongToBeat (risco real).** O formato de [rules §2..§4](contracts/hltb-rules.md) é só o observado por bibliotecas de terceiros. A T05 confere com rede, antes do cliente. Divergência que mude regra visível volta ao spec-writer e pausa a T06.
3. **`SecondInstanceData.Args` e o pacote `flag`.** O `flag.Parse` para no primeiro argumento que não começa com `-`; se o Wails entregar o nome do exe em `Args[0]`, `--bigbox` nunca seria lido. Para `ContinueOnError`, um flag desconhecido também aborta o parse e perde os flags seguintes (o `runUI` de hoje já tem essa falha para `--data-dir`). Por isso a T26 implementa `wantsBigBox` por varredura própria (`--bigbox`, `-bigbox`, `--bigbox=<bool>` via `strconv.ParseBool`, ignora o resto) e confere o conteúdo real de `Args` no Wails em uso. Os testes cobrem os dois formatos de `Args`.
4. **`FilterSpec` usa `statuses` (lista) e `favorite`** (`frontend/src/lib/filters.ts`), enquanto o contrato de tema fala em `filter.status?: 'completed'`. A T30 mapeia a aba "Concluídos" para `statuses: ['completed']`. Só ajuste de texto.
5. **`HLTBConfig` ganha `Timeout time.Duration`** (padrão 10 s) além do que está no design, para o teste do prazo de TTB-010 não esperar 10 s reais (`context.WithTimeout` não usa o `Clock`). O mesmo vale para o prazo de 25 s do serviço (`syncTimeout`, campo interno do serviço).
6. **Tipos `HLTBOutcome` e `IGDBOutcome`** aparecem em `TTBResolve` no design sem definição. A T04 os define (assinaturas abaixo).
7. **`ttbFallback` usa `scrapers.Query` e `scrapers.Candidate` do IGDB da fase 12.** A T14 liga o serviço a um adaptador sobre `*scrapers.IGDB` (e ao `scrapers.Keys` para saber "sem chave" sem requisição). O implementer confere se `(*IGDB).Search` já tem a assinatura de `ttbFallback` e, senão, adapta sem mudar o comportamento da busca de metadados.
8. **Leitura de título, ano e plataforma do jogo.** O `GameTitleYear` do design só entra se o store ainda não tiver leitura equivalente (a busca de metadados já lê esses campos). A T02 reaproveita o que existir; a reserva do IGDB também precisa da chave de seed e do nome da plataforma.
9. **Versão.** A spec não fixa o número. Proponho **1.1.0** (recurso novo, sem quebra), aplicado na T37. Ajustar se o usuário preferir 1.0.4.
10. **`CLAUDE.md` e o ADR 0024** (lacuna 2 da spec): o `CLAUDE.md` ainda diz que o agente "inicia com o Windows". Corrigir na T37.
11. **Arquivos compartilhados entre fatias** (conflito de merge se rodarem em paralelo): `internal/agent/app.go` (T14 e T24), `cmd/arcadia/app.go` (T16 e T26), `frontend/src/App.tsx` (T20, T28 e T36), `frontend/src/bigbox/useBigBoxInput.ts` e o teste dele (T28, T30), `frontend/src/bigbox/themes/registry.ts`, `frontend/src/index.css`, `frontend/src/theme.test.ts` e `frontend/src/lib/uiPrefs.ts` (T32, T34, T36). Ver "Paralelismo".
12. **Hooks.** `gotest.ps1` roda `go test ./...` no Stop do implementer e deixa o test-writer vermelho de propósito. Duas tarefas de código simultâneas na mesma árvore se atrapalham: usar `git worktree` por fatia, ou terminar cada tarefa verde e commitada antes da próxima.

## Premissas

- Uma tarefa por commit; a tarefa de testes vem antes da de código (testes vermelhos são esperados entre as duas). Ao fim de cada tarefa Go: `go build ./...`, `go vet ./...`, `CGO_ENABLED=0 go build ./...`, `go test ./...` (com `-race` em T12, T14, T24 e T26). Ao fim de cada tarefa de frontend: `npm test` e `npx tsc --noEmit` em `frontend/`.
- Todo caso de teste começa pelo ID (`t.Run("ATB-004/...")`, `it("MOS-004 ...")`). Sem rede e sem `time.Sleep`: `httptest`, fakes, `domain.FakeClock` e `vi.useFakeTimers`.
- **Nenhuma tarefa do tracker-specialist.** Confirmado: o roteador usa só `Tracker.Current()`, que já existe; `cmd/fake-game` não muda; a nova ação do roteador não é vista pelo tracker.
- **Dependências externas novas: nenhuma** (Go e npm). `net/http`, `net/http/httptest`, `encoding/json` e `flag` bastam.
- Migração: só a 00010. `frontend/wailsjs/go/**` é versionado e regerado pelo `wails build` (T38); a T18 e a T26 podem ajustar os `.d.ts` à mão só para o `tsc` passar antes disso.
- Testes que quebram de propósito e que o test-writer atualiza com a anotação "alterado em 2026-11-22 (fase 15)": literais `SchemaVersion = 9` (`internal/store/media_test.go`, `migrate5_test.go`, `migrate9_test.go`), `pause_test.go` e `close_router_test.go` (nova assinatura de `newTriggerRouter` e `routerTracker.Current`), `contract_test.go`, `contract_phase12_test.go` e `contract_phase13_test.go` (o `contractPath` passa para a v0.16.0 e a v0.15.0 vira `contractV015Path`), os fakes de tema e `NavCtx` dos testes de `frontend/src/bigbox/`, `bigbox-scan.test.ts` e `App.invariants.test.tsx`.

## Paralelismo e dependências

```
A: T01 -> T02          T03 -> T04            (as duas linhas são independentes entre si)
B: T05 -> T06 -> T07
   T08 -> T09 (+T10)            T08 e T09 dependem só de A para os tipos de store/domain citados
   T11 -> T12   (depende de T02, T04, T07, T09)
   T13 -> T14   (depende de T12; T10 independe)
   T15 -> T16   (depende de T09)
   T17 -> T18   (depende de T09 apenas pelo formato; pode começar com os tipos do delta)
C: T19 -> T20   (depende de T18)
D: T21 -> T22   (independente de A, B, C)
   T23 -> T24   (depende de T22 e T02? não: só de T22)
   T25 -> T26   (independente das outras Go de D; depende de T16 só pelo arquivo cmd/arcadia/app.go)
   T27 -> T28   (depende de T26 para o binding; usa mocks, então os testes podem rodar antes)
E: T29 -> T30 -> T31 -> T32   (T32 depende de T18 por useTimeToBeat e de T28 para useBigBoxInput)
F: T33 -> T34   (depende de E)
G: T35 -> T36   (depende de E; ordem final depois de F por causa do registro)
H: T37 (depende de tudo que muda comportamento; os ADRs podem sair antes) -> T38
```

- **Podem rodar em paralelo (arquivos disjuntos):** A (T01..T04) com D-Go (T21..T24) e com T05; B depois de A; D-Go (T21..T24) com B-Go; E (T29..T32) com todo o Go depois que T18 e T28 existirem. F e G são disjuntas em `themes/mosaico/` e `themes/estante/`, mas **não em** `registry.ts`, `index.css`, `theme.test.ts` e `uiPrefs`: fazer F antes de G, ou um merge cuidadoso.
- **Não paralelizar** (arquivos compartilhados): T14 com T24 (`app.go`), T16 com T26 (`cmd/arcadia/app.go`), T20, T28 e T36 (`App.tsx`), T28 com T30 (`useBigBoxInput.ts`).

---

## T01 — Testes: migração 10 e store do tempo para zerar
- Requisitos: TTB-001, TTB-002. **Responsável:** test-writer. Dependência: nenhuma.
- Arquivos: `internal/store/migrate10_test.go` (novo), `internal/store/time_to_beat_test.go` (novo); atualizar os literais de versão em `internal/store/media_test.go`, `migrate5_test.go` e `migrate9_test.go` (`SchemaVersion` passa a 10; a lista de migrações de `migrate9_test.go` termina na 00010).
- Testes (`migrate10_test.go`):
  - `TTB-001/banco novo tem a tabela time_to_beat com as colunas do design §3.1`;
  - `TTB-001/SchemaVersion = 10 e a versão do goose é 10`;
  - `TTB-001/banco na versão 9 com jogos migra sem perder dados e sem criar linhas`;
  - `TTB-001/ON DELETE CASCADE apaga a linha ao remover o jogo`;
  - `TTB-001/CHECK: source aceita hltb e igdb, e igdb só com found e confirmed 0`;
  - `TTB-001/CHECK: valores em segundos ≤ 0 são recusados e nulo é aceito`;
  - `TTB-001/Down apaga a tabela e não mexe nas outras`;
  - `TTB-001/nenhuma outra tabela muda (sqlite_master sem time_to_beat é igual ao da v9)`.
- Testes (`time_to_beat_test.go`, `store.GetTimeToBeat`/`UpsertTimeToBeat`):
  - `TTB-002/found do HowLongToBeat grava fonte, estado, id, título externo, três valores, título e termo da busca, instantes em ms UTC e confirmed`;
  - `TTB-002/found do IGDB grava source igdb com id e nome do IGDB`;
  - `TTB-002/doubt grava os candidatos (JSON, até 10) e nenhum valor`;
  - `TTB-002/not-found grava só o estado, o título, o termo e os instantes`;
  - `TTB-002/valor ausente volta como nulo e o outro valor não é afetado`;
  - `TTB-002/upsert do mesmo jogo substitui a linha (ON CONFLICT)`;
  - `TTB-002/jogo sem linha devolve ok = false`.
- Assinaturas que o teste usa (definidas na T02): ver T02.

## T02 — Migração 10, consultas sqlc e store
- Requisitos: TTB-001, TTB-002. **Responsável:** implementer. Dependência: T01.
- Arquivos: `internal/store/migrations/00010_time_to_beat.sql` (novo, design §3.1 literal), `internal/store/queries/time_to_beat.sql` (novo), `internal/store/time_to_beat.go` (novo), `internal/store/store.go` (ou onde estiver `SchemaVersion`: 9 → 10), código gerado por `sqlc generate`.
```go
// internal/store/time_to_beat.go
type TimeToBeatRow struct {
    GameID                                   int64
    Source                                   string // "hltb" | "igdb"
    State                                    string // "found" | "doubt" | "not-found"
    ExternalID, ExternalTitle                string
    MainS, MainExtraS, CompletionistS        *int64 // seconds; nil = no data
    TitleAtFetch, Term                       string
    CandidatesJSON                           string // "[]" outside doubt
    Confirmed                                bool
    FetchedAt, ExpiresAt                     time.Time // stored as Unix ms UTC
}
func (s *Store) GetTimeToBeat(ctx context.Context, gameID int64) (TimeToBeatRow, bool, error)
func (s *Store) UpsertTimeToBeat(ctx context.Context, row TimeToBeatRow) error
// Only if the store has no equivalent read (see "Dúvidas" 8): title, year (0 = unknown), platform id, platform name and seed key of a game.
func (s *Store) GameTitleYear(ctx context.Context, gameID int64) (title string, year int, err error)
```
- As duas chaves de configuração (`timeToBeat.enabled`, `timeToBeat.afterMetadata`) usam `Setting`/`SetSetting` existentes. Sem testes novos (T01 cobre).

## T03 — Testes: funções puras do tempo para zerar
- Requisitos: TTB-004, TTB-006, TTB-011, TTB-033, TTB-034, TTB-038. **Responsável:** test-writer. Dependência: nenhuma.
- Arquivo: `internal/domain/timetobeat_test.go` (novo).
- Testes:
  - `RankTimeToBeat`: um caso por linha dos vetores de [rules §5](contracts/hltb-rules.md), conferindo o **estado**, não o número: `TTB-004/Hollow Knight (2017) vs Silksong é found`; `TTB-004/Super Mario World (USA) é found e o segundo tem a marca 2`; `TTB-004/Resident Evil 2 (1998) contra 2019 e 1998 é doubt`; `TTB-004/Mega Man X contra Mega Man X2 não é match`; `TTB-004/Zelda sem ano é doubt`; `TTB-004/Final Fantasy VII contra Remake é found`; `TTB-004/sem resultados é not-found`; `TTB-004/Celeste com apelido é found`; `TTB-004/ordem por pontuação e empate pelo menor ID`; `TTB-004/a plataforma não entra na pontuação`;
  - `TTBExpiresAt`: `TTB-006/found do hltb vence em 90 dias`; `TTB-006/found do igdb vence em 30 dias`; `TTB-006/doubt e not-found vencem em 30 dias`;
  - `TTBStale`: `TTB-006/vence quando agora ≥ expires_at`; `TTB-006/título atual diferente do gravado vence na hora se não confirmada`; `TTB-006/linha confirmada ignora a troca de título`;
  - `TTBBackoff`: `TTB-011/escada 1, 5, 30 e 60 minutos; a quarta em diante fica em 60`; `TTB-038/a mesma escada serve ao IGDB`;
  - `TTBResolve` (um caso por linha de [rules §10.1](contracts/hltb-rules.md)): `TTB-033/found do hltb: grava found hltb e não consulta o IGDB`; `TTB-033/doubt do hltb: grava doubt hltb`; `TTB-033/not-found e IGDB com valores: grava found igdb`; `TTB-033/not-found e IGDB sem match ou sem valores: not-found hltb`; `TTB-033/not-found e IGDB com falha, em recuo ou sem chave: not-found hltb`; `TTB-034/falha do hltb e IGDB com valores: found igdb`; `TTB-034/falha ou recuo do hltb e IGDB sem match ou sem valores: nada gravado`; `TTB-034/falha do hltb e IGDB com falha, recuo ou sem chave: nada gravado`; `TTB-033/termo vazio: not-found hltb sem consulta`.

## T04 — `internal/domain`: funções puras do tempo para zerar
- Requisitos: TTB-004, TTB-006, TTB-011, TTB-033, TTB-034, TTB-038. **Responsável:** implementer. Dependência: T03.
- Arquivo: `internal/domain/timetobeat.go` (novo).
```go
type TTBState string
const (TTBFound TTBState = "found"; TTBDoubt TTBState = "doubt"; TTBNotFound TTBState = "not-found")
const (TTBSourceHLTB = "hltb"; TTBSourceIGDB = "igdb")
const ( TTBFoundTTL = 90 * 24 * time.Hour; TTBIGDBFoundTTL = 30 * 24 * time.Hour; TTBDoubtTTL = 30 * 24 * time.Hour; TTBNotFoundTTL = 30 * 24 * time.Hour )
type TTBResult struct { ID int64; Name string; Aliases []string; Year int }
type TTBRanked struct { Index int; Scored Scored }
func RankTimeToBeat(title string, year int, results []TTBResult) (TTBState, []TTBRanked)
func TTBExpiresAt(source string, state TTBState, fetchedAt time.Time) time.Time
func TTBStale(source string, confirmed bool, titleAtFetch, currentTitle string, expiresAt, now time.Time) bool
func TTBBackoff(failures int) time.Duration // failures >= 1: 1, 5, 30, 60 min; capped at 60
type HLTBOutcome int // HLTBFound, HLTBDoubt, HLTBNotFound, HLTBFailed (temporary failure or backoff), HLTBEmptyTerm
type IGDBOutcome int // IGDBNotAsked, IGDBValues, IGDBNoMatch, IGDBUnavailable, IGDBNoKey
func TTBResolve(h HLTBOutcome, i IGDBOutcome) (write bool, source string, state TTBState)
```

## T05 — Conferência do endpoint do HowLongToBeat (com rede)
- Requisitos: TTB-012, TTB-027, TTB-036. **Responsável:** sessão principal, com o usuário (a tarefa exige rede; nenhum teste a faz).
- Conferir os pontos da tabela de [rules §9](contracts/hltb-rules.md) (padrão do caminho no script, passo prévio, corpo e campos obrigatórios, cabeçalhos mínimos, campos `comp_*` em segundos, `game_type`, taxa de 1 requisição a cada 2 s, e `game_time_to_beats` do IGDB) e preencher as colunas "Conferido em" e "Resultado". Divergência de regra visível volta ao spec-writer e pausa a T06 e a T07; divergência só de formato é ajustada em rules §2..§4 e §10.3.
- Saída adicional: amostras **sanitizadas** (página inicial, script, resposta de busca com resultados e vazia, resposta do IGDB) entregues ao test-writer na T06 (só o test-writer cria `testdata/`). Nenhuma credencial é gravada em arquivo ou log. Bloqueia T06 e T07.

## T06 — Testes: clientes do HowLongToBeat e do IGDB
- Requisitos: TTB-003, TTB-005, TTB-010, TTB-012, TTB-027, TTB-031, TTB-036, TTB-037, TTB-038. **Responsável:** test-writer. Dependência: T05 (e T04 para as constantes).
- Arquivos: `internal/scrapers/hltb_test.go`, `internal/scrapers/igdb_ttb_test.go` (novos); `internal/scrapers/testdata/hltb/*` (página inicial, script com o padrão da busca, script sem o padrão, busca com resultados, busca vazia, HTML no lugar de JSON, `game_type: dlc`) e `testdata/igdb_time_to_beats*.json` (com os três campos, ausentes, zeros, lista vazia).
- Testes (HowLongToBeat, `httptest`):
  - `TTB-012/descobre o caminho pela página inicial e pelo script (_app-) e o reusa na segunda busca sem nova descoberta`;
  - `TTB-012/com 404 na busca descobre de novo uma única vez; se falhar de novo é falha temporária`;
  - `TTB-012/script sem o padrão em nenhum dos 5 primeiros: falha temporária (ErrUnavailable)`;
  - `TTB-005/comp_main, comp_plus e comp_100 viram segundos; zero, ausente, negativo ou não inteiro viram 0 (nulo) sem afetar os outros`;
  - `TTB-005/game_id ausente ou ≤ 0, game_name vazio e game_type diferente de game são ignorados; release_world fora de 1950..2100 vira 0; game_alias e profile_platform separados por vírgula`;
  - `TTB-005/data vazia é resposta válida (sem resultados), não falha`;
  - `TTB-003/searchTerms é o termo dividido por espaços, sem vazios, size 20`;
  - `TTB-010/cada requisição, inclusive a da descoberta, passa pelo Limiter de 2.000 ms (relógio fake) e é abandonada pelo Timeout configurado (servidor lento)`;
  - `TTB-010/ctx cancelado pelo chamador devolve ctx.Err() sem virar ErrUnavailable`;
  - `TTB-011/rede, 403, 429, 5xx, JSON inválido, data ausente e corpo maior que 2 MiB são ErrUnavailable sem URL nem corpo no erro`;
  - `TTB-027/o pedido leva só o termo e os campos fixos, os cabeçalhos genéricos e nenhum Cookie, Authorization ou id de jogo; o cliente não tem Jar`;
  - `TTB-031/os clientes usam BaseURL injetada e um transporte que recusa host não loopback`.
- Testes (`(*IGDB).TimeToBeat`, `httptest`):
  - `TTB-036/o corpo é fields game_id,hastily,normally,completely; where game_id = <id>; limit 1; com Client-ID e Bearer`;
  - `TTB-036/hastily vira História principal, normally História + extras, completely 100%`;
  - `TTB-036/campos ausentes, zero, negativo ou não inteiro viram nulo; lista vazia devolve ok = false`;
  - `TTB-037/sem chave devolve scrapers.ErrNoKey e nenhuma requisição`;
  - `TTB-038/429, 5xx e rede são ErrUnavailable e a chamada passa pelo Limiter do IGDB (o mesmo da busca de metadados)`.

## T07 — `internal/scrapers`: cliente HowLongToBeat e leitura do IGDB
- Requisitos: TTB-003, TTB-005, TTB-010, TTB-011, TTB-012, TTB-027, TTB-036, TTB-037, TTB-038. **Responsável:** implementer. Dependência: T06.
- Arquivos: `internal/scrapers/hltb.go`, `internal/scrapers/hltb_parse.go`, `internal/scrapers/igdb_ttb.go` (novos).
```go
type HLTBGame struct {
    ID                             int64
    Name                           string
    Aliases                        []string
    Year                           int // 0 = unknown
    Platforms                      []string
    Main, MainExtra, Completionist int64 // seconds; 0 = no data
}
type HLTBConfig struct {
    BaseURL string        // default https://howlongtobeat.com
    HTTP    *http.Client  // copied; Jar nil, Timeout unset
    Limiter *Limiter      // own instance, 2 s
    Timeout time.Duration // per request, default 10 s (tests shorten it)
}
type HLTB struct{ /* cfg, mu sync.Mutex, searchPath string */ }
func NewHLTB(cfg HLTBConfig) *HLTB
func (h *HLTB) Search(ctx context.Context, term string) ([]HLTBGame, error)

type IGDBTimeToBeat struct{ Hastily, Normally, Completely int64 } // seconds; 0 = no data
func (c *IGDB) TimeToBeat(ctx context.Context, igdbID string) (IGDBTimeToBeat, bool, error) // false = no record
```
- Constantes de [rules §1](contracts/hltb-rules.md) em `internal/scrapers` (`HLTBInterval = 2 * time.Second`, `HLTBRequestTimeout`, `HLTBPageSize = 20`, `HLTBMaxBody = 2 << 20`). Erros de rede sem URL (`netCause`, como os outros provedores). Sem goroutine nova.

## T08 — Testes: contrato IPC v0.16.0
- Requisitos: TTB-029, TTB-023, TTB-020, TTB-021, TTB-022, TTB-008. **Responsável:** test-writer. Dependência: nenhuma (fica vermelho até T09 e T10).
- Arquivos: `internal/ipc/contract_phase15_test.go` (novo); `internal/ipc/contract_test.go` (`contractPath` e `contractRelPath` passam para `phase-15-big-box-v2`), `contract_phase13_test.go` e `contract_phase12_test.go` (a v0.15.0 vira `contractV015Path`, e a verificação de versão usa a v0.16.0 onde for "≥").
- Testes:
  - `TTB-029/versão 0.16.0 e o comentário do pacote aponta para phase-15-big-box-v2/contracts/ipc.openrpc.json`;
  - `TTB-029/os seis métodos timeToBeat.* existem com paramStructure by-name e os nomes das constantes Go`;
  - `TTB-029/timeToBeat.changed é notificação com o único parâmetro gameId inteiro ≥ 1`;
  - `TTB-029/campos dos tipos Go TimeToBeat, TimeToBeatCandidate e TimeToBeatSettings batem com as propriedades dos esquemas (assertExactFields)`;
  - `TTB-029/o enum source tem hltb e igdb (e vazio com none) e state tem none, found, doubt e not-found`;
  - `TTB-029/x-timetobeat repete as constantes: intervalo 2000 ms, prazos, TTLs (90, 30, 30, 30), escada 1, 5, 30, 60, fila 1000, 10 candidatos, termo 200 e as chaves de configuração`;
  - `TTB-020/-32011 do refresh e do search traz data.provider hltb e reason unavailable ou disabled; -32001 e -32602 documentados`;
  - `TTB-021/timeToBeat.search: term opcional de 1 a 200 caracteres`;
  - `TTB-022/timeToBeat.choose: externalId aceita string vazia`;
  - `TTB-023/timeToBeat.setSettings: enabled e afterMetadata opcionais, pelo menos um`;
  - `TTB-008/timeToBeat.get: fetch tem padrão false e nunca devolve -32011`;
  - `TTB-029/a v0.16.0 é a v0.15.0 mais acréscimos`: normaliza na v0.16.0 `info.version`, `info.description`, `info.x-timetobeat`, os métodos e esquemas novos e a notificação nova, **mais** `media.add`, `media.remove`, `game.setPlayTime` e os esquemas da fase 14 (Dúvidas 1); depois `reflect.DeepEqual` com a v0.15.0; confere que nenhum método, parâmetro, resultado, erro ou esquema da v0.15.0 mudou ou sumiu e que a ordem relativa dos antigos se mantém.

## T09 — `internal/ipc`: tipos e constantes v0.16.0
- Requisitos: TTB-029, TTB-020, TTB-021, TTB-022, TTB-023. **Responsável:** implementer. Dependência: T08.
- Arquivos: `internal/ipc/timetobeat.go` (novo), `internal/ipc/protocol.go` (comentário do pacote aponta para o contrato da fase 15).
```go
const (
    MethodTimeToBeatGet = "timeToBeat.get"; MethodTimeToBeatRefresh = "timeToBeat.refresh"
    MethodTimeToBeatSearch = "timeToBeat.search"; MethodTimeToBeatChoose = "timeToBeat.choose"
    MethodTimeToBeatGetSettings = "timeToBeat.getSettings"; MethodTimeToBeatSetSettings = "timeToBeat.setSettings"
    MethodTimeToBeatChanged = "timeToBeat.changed"
)
type TimeToBeatGetParams struct{ GameID int64 `json:"gameId"`; Fetch bool `json:"fetch,omitempty"` }
type TimeToBeatGameParams struct{ GameID int64 `json:"gameId"` }
type TimeToBeatSearchParams struct{ GameID int64 `json:"gameId"`; Term *string `json:"term,omitempty"` }
type TimeToBeatChooseParams struct{ GameID int64 `json:"gameId"`; ExternalID string `json:"externalId"` }
type TimeToBeatSetSettingsParams struct{ Enabled *bool `json:"enabled,omitempty"`; AfterMetadata *bool `json:"afterMetadata,omitempty"` }
type TimeToBeatSettings struct{ Enabled bool `json:"enabled"`; AfterMetadata bool `json:"afterMetadata"` }
type TimeToBeatChangedParams struct{ GameID int64 `json:"gameId"` }
type TimeToBeatCandidate struct { /* delta §3 */ }
type TimeToBeat struct { /* delta §3; candidates never nil */ }
type TimeToBeatSearchResult struct{ Candidates []TimeToBeatCandidate `json:"candidates"` }
```
- Reutilizar o código `-32011` (`ProviderUnavailable`) existente com `data = {provider, reason}`; criar o `reason` se ainda não houver. Sem testes próprios (T08 cobre).

## T10 — Contrato completo `ipc.openrpc.json` v0.16.0
- Requisitos: TTB-029. **Responsável:** sessão principal (cópia mecânica de JSON grande, por script, como nas fases 11 a 13). Dependência: nenhuma. Bloqueia T08 verde.
- Criar `docs/specs/phase-15-big-box-v2/contracts/ipc.openrpc.json` como cópia de `docs/specs/phase-13-modo-console/contracts/ipc.openrpc.json` (v0.15.0) mais: `info.version = "0.16.0"`, `info.x-timetobeat`, os esquemas e métodos do [delta](contracts/ipc-v0.16.0-delta.openrpc.json), `timeToBeat.changed`, **e** `media.add`, `media.remove`, `game.setPlayTime` (tipos de `internal/ipc/manual_edit.go`).
- Em `ipc-v0.16.0-delta.md` e `design.md` §3.5, trocar "(tarefa)" pelo arquivo criado.

## T11 — Testes: serviço `timeToBeat` do agente (núcleo)
- Requisitos: TTB-002, TTB-003, TTB-004, TTB-006, TTB-009, TTB-010, TTB-011, TTB-026, TTB-028, TTB-031, TTB-033, TTB-034, TTB-035, TTB-036, TTB-037, TTB-038. **Responsável:** test-writer. Dependência: T02, T04, T07, T09.
- Arquivos: `internal/agent/timetobeat_test.go` (novo), `internal/agent/timetobeat_fakes_test.go` (novo: cliente HLTB falso controlado por canais, `ttbFallback` falso, `KeyStore` falso, notificador espião, relógio `domain.FakeClock`, store real em `t.TempDir()`).
- Testes (chamando `newTimeToBeatService` e `lookup` pela fila e por `Refresh`):
  - `TTB-003/o termo enviado é domain.SearchTerm(título); termo vazio não faz requisição e grava not-found`;
  - `TTB-003/sem resultados e termo com ": ": segunda busca com o texto antes do primeiro ": " e classificação contra o título inteiro`;
  - `TTB-004/found, doubt e not-found chegam ao store com valores e candidatos (até 10, piso 0,40 em doubt)`;
  - `TTB-002/a linha gravada guarda título e termo da busca, instante e validade (relógio fake)`;
  - `TTB-006/validade por estado e fonte; título trocado vence na hora; linha confirmada vencida relê só os valores pelo id confirmado, e se o id não volta mantém os valores e renova a validade; not-found confirmada nunca é relida sozinha`;
  - `TTB-006/linha igdb vencida recomeça pelo HowLongToBeat`;
  - `TTB-009/no máximo uma requisição por vez (worker, Refresh e Search somados)`; `TTB-009/um jogo aparece uma vez na fila; pedir de novo não duplica`; `TTB-009/pedido interativo promove o jogo da fila do lote e passa à frente do lote`; `TTB-009/cada fila segue a ordem de chegada`; `TTB-009/a fila do lote para em 1.000 e descarta o excedente com Debug`; `TTB-009/jogo removido no meio do caminho não grava nem notifica`;
  - `TTB-010/o HowLongToBeat tem Limiter próprio de 2.000 ms e o IGDB usa o do IGDB da fase 12`;
  - `TTB-011/escada de recuo 1, 5, 30 e 60 min por falha seguida; durante o recuo nenhuma requisição ao hltb`; `TTB-011/um único Warn "HowLongToBeat indisponível" por sequência, sem corpo nem URL; resposta válida zera a sequência`; `TTB-011/com as duas fontes indisponíveis a fila espera o menor pausedUntil pelo relógio fake e get fica unavailable`; `TTB-011/ctx do agente cancelado não conta falha nem grava`;
  - `TTB-033/um caso por linha da tabela de rules §10.1; o IGDB não é chamado com found nem com doubt do hltb`;
  - `TTB-034/hltb em falha e em recuo com o IGDB dando valores, sem match, sem valores e com falha`; `TTB-034/nada gravado sai da fila sem notificar`;
  - `TTB-035/o jogo do IGDB é pontuado com título, ano e chave de seed e só match segue; doubt e none contam como sem match e não guardam candidatos`;
  - `TTB-036/os valores do IGDB mapeiam hastily, normally e completely; zeros e ausentes são nulos; sem registro ou três nulos é sem valores; a linha guarda id e nome do IGDB`;
  - `TTB-037/sem chave do IGDB (ErrNoKey) o cliente IGDB nunca é chamado, o log não tem Warn e o IGDB não entra em recuo; cadastrar a chave depois faz a reserva valer`;
  - `TTB-038/a falha do IGDB (ErrUnavailable ou credencial recusada) abre o recuo do IGDB com Warn "IGDB indisponível para o tempo para zerar" uma vez; as duas escadas são independentes e o recuo do IGDB não impede o hltb`;
  - `TTB-026/jogo removido: sai das filas e de candidates; busca em curso termina sem gravar nem notificar`;
  - `TTB-028/Close cancela o ctx, abandona a requisição em curso sem gravar e espera o worker (sem goroutine viva); idempotente`; `TTB-028/depois de BeginShutdown nenhuma notificação sai e as filas são descartadas`;
  - `TTB-031/os fakes são os únicos clientes; nenhum teste do serviço abre soquete`.

## T12 — `internal/agent`: serviço `timeToBeat` (fila, trabalhador, recuo, cache)
- Requisitos: TTB-002, TTB-003, TTB-004, TTB-006, TTB-009, TTB-010, TTB-011, TTB-026, TTB-028, TTB-033, TTB-034, TTB-035, TTB-036, TTB-037, TTB-038. **Responsável:** implementer. Dependência: T11.
- Arquivos: `internal/agent/timetobeat.go`, `internal/agent/timetobeat_lookup.go` (novos).
```go
type ttbSearcher interface{ Search(ctx context.Context, term string) ([]scrapers.HLTBGame, error) }
type ttbFallback interface {
    Search(ctx context.Context, q scrapers.Query) ([]scrapers.Candidate, error)
    TimeToBeat(ctx context.Context, igdbID string) (scrapers.IGDBTimeToBeat, bool, error)
}
type ttbNotifier interface{ TimeToBeatChanged(gameID int64) }
type timeToBeatService struct { /* design §3.4 */ }
func newTimeToBeatService(st *store.Store, client ttbSearcher, igdb ttbFallback, clock domain.Clock, log *slog.Logger, note ttbNotifier) *timeToBeatService
func (s *timeToBeatService) Start()         // one worker goroutine, owned by wg
func (s *timeToBeatService) BeginShutdown() // stopped = true; queues emptied
func (s *timeToBeatService) Close()         // cancel + wg.Wait; idempotent
func (s *timeToBeatService) Get(ctx context.Context, gameID int64, fetch bool) (ipc.TimeToBeat, error)
func (s *timeToBeatService) Refresh(ctx context.Context, gameID int64) (ipc.TimeToBeat, error)
func (s *timeToBeatService) Search(ctx context.Context, gameID int64, term string) ([]ipc.TimeToBeatCandidate, error)
func (s *timeToBeatService) Choose(ctx context.Context, gameID int64, externalID string) (ipc.TimeToBeat, error)
func (s *timeToBeatService) Settings(ctx context.Context) (ipc.TimeToBeatSettings, error)
func (s *timeToBeatService) SetSettings(ctx context.Context, p ipc.TimeToBeatSetSettingsParams) (ipc.TimeToBeatSettings, error)
func (s *timeToBeatService) AfterMetadata(gameID int64) // TTB-025
func (s *timeToBeatService) GameRemoved(gameID int64)   // TTB-026
```
- Regras e ordem de operações: design §3.4 e [rules §5..§7 e §10](contracts/hltb-rules.md). `reqMu` cobre o `lookup` inteiro (as duas fontes). Espera de recuo só por `domain.Clock.After`; sem `time.Sleep`. Prazo de 25 s de `Refresh`/`Search` configurável por campo interno (testes encurtam).

## T13 — Testes: handlers IPC, configuração e ganchos do serviço
- Requisitos: TTB-007, TTB-008, TTB-020, TTB-021, TTB-022, TTB-023, TTB-025, TTB-026, TTB-028, TTB-040. **Responsável:** test-writer. Dependência: T12 (pode começar na T11).
- Arquivos: `internal/agent/timetobeat_ipc_test.go` (novo); atualizar `internal/agent/shutdown_test.go` (ordem do shutdown) e os helpers de `components_test.go` se criarem o agente completo.
- Testes:
  - `TTB-007/get com fetch false só lê o banco e a memória: sem requisição, sem fila; none sem linha; linha vencida volta com stale true; pending e unavailable refletem o serviço`;
  - `TTB-008/get com fetch true sem linha ou vencida enfileira como interativo, responde na hora com pending true e o cache; com linha válida nada é enfileirado`;
  - `TTB-008/dois clientes IPC recebem timeToBeat.changed {gameId} sem id quando a busca grava; nenhum em falha`;
  - `TTB-008/em recuo (duas fontes) nada é enfileirado e a resposta traz unavailable true`;
  - `TTB-020/refresh ignora validade e recuo, passa pelos limitadores e pela vez única, prazo de 25 s, grava e notifica; linha confirmada relê pelo id; nada gravado por falha é -32011 com provider hltb; jogo inexistente -32001; serviço desligado -32011 reason disabled sem requisição`;
  - `TTB-021/search usa term (cortado, 1 a 200) ou o termo de TTB-003, devolve até 10 pontuados em ordem decrescente, não muda a linha e guarda os candidatos; termo vazio ou maior que 200 é -32602; falha -32011; jogo inexistente -32001`;
  - `TTB-022/choose grava found confirmada com os valores do candidato sem requisição e notifica; externalId vazio grava not-found confirmada; id fora dos candidatos (inclusive depois de reiniciar sem doubt) é -32602 e nada muda; candidatos da linha em doubt valem`;
  - `TTB-040/linha igdb nunca é confirmed; choose grava found confirmado com fonte hltb; choose vazio grava not-found confirmado com fonte hltb; depois disso nunca cai na reserva sozinha`;
  - `TTB-023/getSettings lê app_setting com ausente = ligado; setSettings grava só os campos presentes e responde o estado final; nenhum campo é -32602`;
  - `TTB-023/enabled false: nenhuma requisição por nenhum caminho (nem ao IGDB), fila esvaziada, get devolve o cache com pending sempre falso`;
  - `TTB-025/metadata.apply e a aplicação do lote enfileiram a busca como lote ignorando a validade quando enabled e afterMetadata estão ligados e a linha não é confirmada; o resultado e as notificações de metadata não mudam; faltando uma condição nada é enfileirado (Debug)`;
  - `TTB-026/game.delete (store real) apaga a linha por cascata, tira o jogo das filas e uma busca em curso termina sem gravar`;
  - `TTB-028/o shutdown chama BeginShutdown do serviço junto do tracker e Close antes de fechar o store; nenhuma notificação depois`.

## T14 — `internal/agent`: handlers IPC, ligação e ganchos
- Requisitos: TTB-007, TTB-008, TTB-020, TTB-021, TTB-022, TTB-023, TTB-025, TTB-026, TTB-028, TTB-040, TTB-010. **Responsável:** implementer. Dependência: T13.
- Arquivos: `internal/agent/timetobeat_ipc.go` (novo: registra os seis métodos e valida parâmetros), `internal/agent/app.go` (cria o serviço depois do store e do servidor IPC; `Start` junto da fila de mídia; `BeginShutdown` junto do tracker; `Close` antes de fechar o store), `internal/agent/scrape_providers.go` (instância `scrapers.HLTB` com `Limiter` próprio de 2.000 ms; adaptador `ttbFallback` sobre o `*scrapers.IGDB` e o `Limiter` do IGDB já compartilhados com os metadados), `internal/agent/notifier.go` (ou `media_changes.go`: `TimeToBeatChanged` pelo mesmo broadcast de `media.changed`), `internal/agent/metadata.go` e `metadata_batch.go` (chamam `AfterMetadata(gameID)` depois de gravar com sucesso, no ponto em que o vídeo é enfileirado), `internal/agent/games.go` (`game.delete` chama `GameRemoved` antes de apagar).
- Sem assinatura pública nova além da T12. Erros: `-32001`, `-32602`, `-32011` com `data`.

## T15 — Testes: bindings e evento do desktop
- Requisitos: TTB-030. **Responsável:** test-writer. Dependência: T09.
- Arquivo: `cmd/arcadia/timetobeat_test.go` (novo), com o fake do agente existente de `cmd/arcadia`.
- Testes:
  - `TTB-030/os seis bindings existem com as assinaturas do delta §6 (reflect): TimeToBeatGet(int64, bool), TimeToBeatRefresh, TimeToBeatSearch, TimeToBeatChoose, TimeToBeatSettings, TimeToBeatSetSettings`;
  - `TTB-030/cada binding chama o método IPC certo com os parâmetros certos e o prazo do delta (2 s, 30 s, 30 s, 2 s, 2 s, 2 s); term vazio é omitido`;
  - `TTB-030/os erros ao usuário seguem UI-008; -32011 vira "Não foi possível consultar o HowLongToBeat" e, com reason disabled, "Consulta ao HowLongToBeat desligada"`;
  - `TTB-030/o supervisor repassa timeToBeat.changed como o evento Wails timeToBeat:changed {gameId}`;
  - `TTB-030/parâmetros ausentes, JSON inválido ou gameId não inteiro positivo registram Warn "timeToBeat.changed com parâmetros inválidos" e nada é emitido`;
  - `TTB-030/nenhum binding chama método de sessão (game.launch, session.*, pause) nem fecha jogo`.

## T16 — `cmd/arcadia`: bindings `TimeToBeat*` e repasse do evento
- Requisitos: TTB-030. **Responsável:** implementer. Dependência: T15.
- Arquivos: `cmd/arcadia/timetobeat.go` (novo), `cmd/arcadia/supervisor.go` (`case ipc.MethodTimeToBeatChanged`, decodificação como a de `media.changed`).
```go
func (a *App) TimeToBeatGet(gameID int64, fetch bool) (TimeToBeatView, error)
func (a *App) TimeToBeatRefresh(gameID int64) (TimeToBeatView, error)
func (a *App) TimeToBeatSearch(gameID int64, term string) ([]TimeToBeatCandidateView, error)
func (a *App) TimeToBeatChoose(gameID int64, externalID string) (TimeToBeatView, error)
func (a *App) TimeToBeatSettings() (TimeToBeatSettingsView, error)
func (a *App) TimeToBeatSetSettings(enabled, afterMetadata bool) (TimeToBeatSettingsView, error)
// views: delta §6 (TimeToBeatView, TimeToBeatCandidateView, TimeToBeatSettingsView)
```

## T17 — Testes: `lib/timeToBeat.ts` e `lib/timeToBeatEdit.ts`
- Requisitos: TTB-013, TTB-014, TTB-015, TTB-016, TTB-017, TTB-039. **Responsável:** test-writer. Dependência: nenhuma (usa mocks dos bindings).
- Arquivos: `frontend/src/lib/timeToBeat.test.tsx`, `frontend/src/lib/timeToBeatEdit.test.ts` (novos).
- Testes:
  - `TTB-014/formatTimeToBeat dos vetores: null, 0 e -5 → "—"; 100 → "5 min"; 1800 → "30 min"; 3300 → "55 min"; 3599 → "1 h"; 5400 → "1½ h"; 97200 → "27 h"; 99000 → "27½ h"; 99900 → "28 h"`;
  - `TTB-014/timeToBeatRows devolve sempre 3 linhas com os rótulos História principal, História + extras e 100%; "—" sem dado`;
  - `TTB-013/useTimeToBeat: erro do binding, binding ausente e agente desconectado viram data undefined, sem lançar (retry false)`;
  - `TTB-015/useTimeToBeat lê o cache na hora (fetch false) e pede fetch true uma vez depois de 1.000 ms com o mesmo jogo; trocar de jogo antes cancela o pedido; desmontar limpa o temporizador`;
  - `TTB-016/fetchAfterMs 0 pede fetch true na hora, uma vez; fetchAfterMs null só lê o cache`;
  - `TTB-017/timeToBeat:changed invalida só a consulta do jogo do aviso; outro jogo não relê`;
  - `TTB-039/timeToBeatSource: hltb → "HowLongToBeat", igdb → "IGDB", sem valor → null`;
  - `TTB-018/refreshTimeToBeat, searchTimeToBeat, chooseTimeToBeat e setTimeToBeatSettings chamam o binding certo e devolvem o resultado`.

## T18 — Frontend: `lib/timeToBeat.ts` e `lib/timeToBeatEdit.ts`
- Requisitos: TTB-013, TTB-014, TTB-015, TTB-016, TTB-017, TTB-039. **Responsável:** implementer. Dependência: T17 (e T16 para os `.d.ts` dos bindings).
- Arquivos: `frontend/src/lib/timeToBeat.ts`, `frontend/src/lib/timeToBeatEdit.ts` (novos); ajuste manual dos tipos de `frontend/wailsjs/go/main/App.d.ts` e `models.ts` só se o `tsc` precisar antes do `wails build`.
```ts
export type TimeToBeatView = { /* delta §3, camelCase */ }
export type TimeToBeatRow = { label: 'História principal' | 'História + extras' | '100%'; value: string }
export const TTB_FETCH_DELAY_MS = 1000
export function formatTimeToBeat(seconds: number | null): string
export function timeToBeatRows(v: TimeToBeatView | undefined): TimeToBeatRow[]
export function timeToBeatSource(v: TimeToBeatView | undefined): 'HowLongToBeat' | 'IGDB' | null
export const timeToBeatKey = (id: number) => ['timeToBeat', id] as const
export function useTimeToBeat(gameId: number | null, o: { fetchAfterMs: number | null }): { data?: TimeToBeatView }
export function useTimeToBeatEvents(): void // EventsOn('timeToBeat:changed')
// timeToBeatEdit.ts (escrita; proibido em bigbox/)
export function refreshTimeToBeat(id: number): Promise<TimeToBeatView>
export function searchTimeToBeat(id: number, term: string): Promise<TimeToBeatCandidateView[]>
export function chooseTimeToBeat(id: number, externalId: string): Promise<TimeToBeatView>
export function useTimeToBeatSettings(): UseQueryResult<TimeToBeatSettingsView>
export function setTimeToBeatSettings(enabled: boolean, afterMetadata: boolean): Promise<TimeToBeatSettingsView>
```

## T19 — Testes: tempo para zerar no painel, em "Editar jogo" e nos provedores
- Requisitos: TTB-013, TTB-015, TTB-017, TTB-018, TTB-019, TTB-024, TTB-030, TTB-039, TTB-040. **Responsável:** test-writer. Dependência: T18 (pode escrever com mocks antes).
- Arquivos: `frontend/src/components/TimeToBeatBlock.test.tsx`, `frontend/src/components/TimeToBeatEditor.test.tsx`, `frontend/src/components/GameDetailsPanel.test.tsx` (o bloco entra depois de "Tempo total"), `frontend/src/components/ProviderKeysDialog.test.tsx`, `frontend/src/App.invariants.test.tsx` (interações desta área).
- Testes:
  - `TTB-015/o painel mostra o bloco "Tempo para zerar" com os três rótulos e valores e a linha da fonte quando há valor; lê o cache na hora e pede fetch true uma vez depois de 1.000 ms (temporizadores falsos); trocar de jogo cancela`;
  - `TTB-015/em doubt mostra "Correspondência incerta" e o botão "Confirmar…", que chama onEdit({ section: 'timeToBeat' })`;
  - `TTB-015/serviço desligado e sem cache: o bloco não aparece`;
  - `TTB-013/com binding que falha, serviço desligado, recuo, agente desconectado, not-found, doubt e valor nulo, os três rótulos ficam com "—", sem role alert, sem modal e sem mensagem de erro (painel)`;
  - `TTB-017/timeToBeat:changed do jogo mostrado relê uma vez (fetch false); de outro jogo não`;
  - `TTB-039/"Fonte: HowLongToBeat" para hltb e "Fonte: IGDB" para igdb; sem valor a linha não aparece`;
  - `TTB-018/"Editar jogo" mostra a seção "Tempo para zerar (HowLongToBeat)" com os três valores e a linha de estado: Encontrado (com "· confirmado"), Encontrado no IGDB (reserva), Correspondência incerta, Não encontrado, Ainda não buscado e HowLongToBeat indisponível no momento, com a data em DD/MM/AAAA`;
  - `TTB-018/"Atualizar" chama TimeToBeatRefresh, fica desativado com "Buscando…" e mostra o resultado; a falha mostra "Não foi possível consultar o HowLongToBeat" sem mudar os valores; vale na hora, sem "Salvar"`;
  - `TTB-018/serviço desligado: só o cache e "Consulta ao HowLongToBeat desligada", sem botões`;
  - `TTB-019/"Escolher outro…" abre o campo com o termo de TTB-003 editável e "Buscar" chama TimeToBeatSearch e lista até 10 candidatos (título, ano, plataformas e os três valores); escolher chama TimeToBeatChoose(G, id); "Nenhum destes" chama TimeToBeatChoose(G, "")`;
  - `TTB-019/em doubt a lista abre com os candidatos gravados, sem nova busca`;
  - `TTB-040/linha igdb: "Escolher outro…" lista jogos do HowLongToBeat e a escolha mostra "· confirmado" com a fonte HowLongToBeat`;
  - `TTB-024/a janela dos provedores tem a seção "HowLongToBeat (tempo para zerar)" com as duas caixas, a segunda desativada com a primeira desligada, que chamam TimeToBeatSetSettings na hora; os textos "Serviço não oficial: pode parar de funcionar sem aviso" e o da reserva do IGDB; nenhum campo de chave`;
  - `TTB-030/App.invariants: abrir o painel, os provedores e a edição com o tempo para zerar não chama LaunchGame, pausa, retomada nem fechar jogo`.

## T20 — Frontend: painel, "Editar jogo" e provedores
- Requisitos: TTB-013, TTB-015, TTB-017, TTB-018, TTB-019, TTB-024, TTB-039, TTB-040. **Responsável:** implementer. Dependência: T19, T18.
- Arquivos: `frontend/src/components/TimeToBeatBlock.tsx` (novo), `frontend/src/components/TimeToBeatEditor.tsx` (novo, fora do formulário, como `GameImagesEditor`), `frontend/src/components/GameDetailsPanel.tsx` (bloco no `<dl>` depois de "Tempo total"; `onEdit({ section: 'timeToBeat' })`), `frontend/src/components/GameEditForm.tsx` (monta a seção e rola até ela), `frontend/src/components/ProviderKeysDialog.tsx` (seção do HowLongToBeat), `frontend/src/App.tsx` (monta `useTimeToBeatEvents()` uma vez; repassa a seção pedida ao abrir "Editar jogo").
- Funções públicas: `TimeToBeatBlock({ gameId }: { gameId: number })`, `TimeToBeatEditor({ gameId, title }: { gameId: number; title: string })`.

## T21 — Testes: entrada, terceiro combo `Back + A`
- Requisitos: ATB-001, ATB-002. **Responsável:** test-writer. Dependência: nenhuma.
- Arquivos: `internal/input/pad_bigbox_test.go`, `internal/input/combo_bigbox_test.go` (novos); revisar `pad_test.go` e `pad_close_test.go` só se o `PadConfig` literal quebrar.
- Testes:
  - `ATB-001/Back+A segurado dispara Trigger{ActionBigBox, gamepad} na primeira leitura com agora−T ≥ 1 s (T+992ms não, T+1008ms sim, leituras de 16 ms)`; `ATB-001/soltar antes cancela`; `ATB-001/um gatilho por pressão`; `ATB-001/controles independentes`; `ATB-001/desconexão zera a contagem`; `ATB-001/o atalho de teclado Ctrl+Alt+P continua só ActionPause`; `ATB-001/PadConfig.BigBoxMask personalizada e o padrão Back|A (0x0020|0x1000)`;
  - `ATB-002/Back+Y só entrega pause, Back+B só close e Back+A só bigbox`; `ATB-002/Back+A+B entrega um close e um bigbox, cada um uma vez por pressão`; `ATB-002/soltar A não zera a contagem de Back+B nem de Back+Y, e vice-versa`; `ATB-002/os acordes de volume Back+cima/baixo continuam como estão`; `ATB-002/fila cheia descarta com Warn`.

## T22 — `internal/input`: `ButtonA`, `ActionBigBox` e terceiro detector
- Requisitos: ATB-001, ATB-002. **Responsável:** implementer. Dependência: T21.
- Arquivos: `internal/input/input.go`, `internal/input/combo.go`, `internal/input/pad.go`.
```go
const ButtonA uint16 = 0x1000
const ActionBigBox Action = 2 // after ActionPause and ActionClose
type PadConfig struct { /* ... */ BigBoxMask uint16 } // zero = Back|A
var defaultBigBoxMask = ButtonBack | ButtonA
// pad: third comboDetector (bdet) observed in read with det and cdet, including the error path that zeros the count;
// emits Trigger{Source: domain.PauseGamepad, Action: ActionBigBox, At: now} on the same queue.
```

## T23 — Testes: roteador (ação `bigbox`) e abridor
- Requisitos: ATB-003, ATB-004, ATB-005, ATB-006, ATB-007, ATB-015, ATB-008. **Responsável:** test-writer. Dependência: T22 (pode escrever antes com a constante).
- Arquivos: `internal/agent/bigbox_router_test.go`, `internal/agent/openui_bigbox_test.go` (novos); atualizar `pause_test.go` e `close_router_test.go` (nova assinatura de `newTriggerRouter`, fake de tracker com `Current()`, abridor falso, `shuttingDown`); teste do parse de flags do agente em `internal/agent/run_windows_test.go` (ou arquivo de opções existente).
- Testes:
  - `ATB-003/dois gatilhos do Big Box com menos de 300 ms entre os At viram um só, sem som; com 300 ms ou mais viram dois`; `ATB-003/as janelas de pausa e de fechar não contam o gatilho do Big Box e vice-versa`; `ATB-003/o roteador só chama Current: nenhuma chamada a TogglePause nem CloseByHotkey (tracker falso)`;
  - `ATB-004/sem sessão o abridor é chamado uma vez; o spawn recebe o exe E, os argumentos --bigbox --data-dir R (sem --agent) e DETACHED_PROCESS | CREATE_NEW_PROCESS_GROUP; Info "Big Box pedido pelo controle"; nenhum som`;
  - `ATB-005/um caso por estado de sessão aberta (Launching, Running, Paused, Grace e fechando): spawn nunca chamado, Debug "Big Box ignorado: jogo em execução" e feedback.EventRefused`;
  - `ATB-005/jogo sem contar tempo vivo e nenhuma sessão: segue ATB-004`;
  - `ATB-006/spawn que falha: Warn "falha ao abrir o Big Box" com path e error, EventRefused, e o gatilho seguinte chama spawn de novo`;
  - `ATB-007/depois de BeginShutdown (shuttingDown true) ou com o ctx do roteador cancelado: Debug, sem processo e sem som`;
  - `ATB-015/nenhuma sessão é criada, pausada, retomada ou encerrada pelo caminho do Big Box (tracker falso só com Current)`;
  - `ATB-008/--agent --bigbox: o parse do agente tolera o flag desconhecido e segue no modo do agente`.

## T24 — `internal/agent`: roteador do Big Box e abridor
- Requisitos: ATB-003, ATB-004, ATB-005, ATB-006, ATB-007, ATB-015. **Responsável:** implementer. Dependência: T23, T22.
- Arquivos: `internal/agent/close_router.go`, `internal/agent/openui.go`, `internal/agent/app.go` (liga `uiOpener{exe, root, spawn}` e o `atomic.Bool` `shuttingDown` em `BeginShutdown`).
```go
type routerTracker interface {
    TogglePause(ctx context.Context, reason domain.PauseReason, at time.Time) (tracker.PauseResult, error)
    CloseByHotkey(ctx context.Context, at time.Time) (tracker.CloseResult, error)
    Current() (domain.SessionSnapshot, bool)
}
type bigBoxOpener interface{ OpenBigBox() error }
func newTriggerRouter(tr routerTracker, in <-chan input.Trigger, log *slog.Logger, fb feedbackSender, sink closeSink,
    opener bigBoxOpener, shuttingDown func() bool) *triggerRouter
func openBigBox(exe, root string, spawn spawnFunc) error
type uiOpener struct{ exe, root string; spawn spawnFunc }
func (o uiOpener) OpenBigBox() error
// domain.BigBoxDebounce = 300 * time.Millisecond (internal/domain/pause.go), next to CloseDebounce
```
- Nenhuma goroutine nova: o roteador segue com a sua. O `CreateDetached` já fecha `hProcess` e `hThread`. Se a constante for criada em `domain`, a T23 a cita (`ATB-003/BigBoxDebounce é 300 ms`).

## T25 — Testes: desktop `--bigbox`, segunda instância e `TakeBigBoxRequest`
- Requisitos: ATB-008, ATB-009, ATB-012, ATB-014, ATB-015. **Responsável:** test-writer. Dependência: nenhuma.
- Arquivos: `cmd/arcadia/bigbox_request_test.go` (novo); `cmd/arcadia/app_test.go` e `mode_test.go` (revisar o que depende de `secondInstance()` sem argumento).
- Testes:
  - `ATB-008/o parse da UI lê --bigbox, -bigbox e --bigbox=true como pedido e --bigbox=false como nenhum; sem o flag nada é pedido; com --agent o modo é o do agente (isAgentMode) e --bigbox é ignorado`;
  - `ATB-008/NewApp com cfg.BigBox guarda o pedido; TakeBigBoxRequest o devolve`;
  - `ATB-009/wantsBigBox lê --bigbox em Args com e sem o nome do exe na frente, com flags desconhecidos antes ou depois e com --data-dir diferente (ignorado)`;
  - `ATB-009/secondInstance(true): ordem das chamadas na plataforma espiã = restaurar a janela, bringToFront, guardar o pedido, emit ui:bigBoxRequested sem dados`;
  - `ATB-009/secondInstance(false) segue EXE-011: janela à frente, sem pedido e sem evento`;
  - `ATB-009/antes do startup (sem wailsCtx) só o pedido é guardado e nenhum emit acontece`;
  - `ATB-009/recusa do primeiro plano pelo Windows (bringToFront devolve false) não é erro`;
  - `ATB-012/TakeBigBoxRequest devolve true uma única vez e depois false até um pedido novo; 100 chamadas concorrentes com -race entregam exatamente um true`;
  - `ATB-014/TakeBigBoxRequest(): bool existe como binding (reflect) e nunca chama o agente (cliente falso sem chamadas)`; `ATB-014/o evento ui:bigBoxRequested não leva dados e não chama o agente`;
  - `ATB-015/os caminhos desta área não chamam nenhum método de sessão no agente falso`.

## T26 — `cmd/arcadia`: `--bigbox`, pedido guardado e segunda instância
- Requisitos: ATB-008, ATB-009, ATB-012, ATB-014. **Responsável:** implementer. Dependência: T25.
- Arquivos: `cmd/arcadia/bigbox_request.go` (novo), `cmd/arcadia/main_windows.go` (`fset.BoolVar(&cfg.BigBox, "bigbox", false, ...)`; `OnSecondInstanceLaunch` passa os `Args`), `cmd/arcadia/main_other.go` (stub), `cmd/arcadia/app.go` (`desktopConfig.BigBox`, `App.bigBoxReq`, `secondInstance(bigBox bool)` com `bringToFront` + `emit`).
```go
type desktopConfig struct { /* ... */ BigBox bool }
func wantsBigBox(args []string) bool // own scan; see "Dúvidas" 3
func (a *App) TakeBigBoxRequest() bool
func (a *App) onSecondInstance(d options.SecondInstanceData) { a.secondInstance(wantsBigBox(d.Args)) }
func (a *App) secondInstance(bigBox bool)
```
- Conferir no código do Wails em uso o conteúdo de `SecondInstanceData.Args` (se o nome do exe vem em `Args[0]`) e registrar o resultado num comentário. `isAgentMode` não muda.

## T27 — Testes: frontend do pedido de Big Box, dica e BBX-057
- Requisitos: ATB-008, ATB-009, ATB-010, ATB-011, ATB-012, ATB-013, ATB-015, BBX-057. **Responsável:** test-writer. Dependência: nenhuma (usa bindings falsos).
- Arquivos: `frontend/src/lib/bigbox.test.ts` (novo), `frontend/src/App.bigbox-request.test.tsx` (novo), `frontend/src/App.invariants.test.tsx`, `frontend/src/bigbox/useBigBoxInput.test.tsx`, `frontend/src/bigbox/hints.test.ts`.
- Testes:
  - `ATB-012/takeBigBoxRequest devolve o booleano do binding e, com falha ou ausência do binding, false`;
  - `ATB-008/ao montar com TakeBigBoxRequest = true o App entra no Big Box e chama BringToFront uma vez; com false abre a janela normal`;
  - `ATB-012/o pedido chega antes da montagem: atendido na montagem; evento e montagem atendendo o mesmo pedido entram uma vez só`;
  - `ATB-009/o evento ui:bigBoxRequested consome o pedido (TakeBigBoxRequest) e entra no Big Box; o listener é removido ao desmontar`;
  - `ATB-010/já no Big Box (Navegar ou Detalhes, suspenso ou não) o pedido só traz a janela à frente: tela, aba, foco, Detalhes e suspensão não mudam`;
  - `ATB-011/com uma janela modal aberta (edição, metadados, importação, emuladores, plataformas, provedores, confirmação do lote, diálogo de remoção) o Big Box não abre e o campo digitado continua; com a página de estatísticas aberta e nenhuma modal, entra e, ao sair, ela volta`;
  - `ATB-013/o botão "Big Box" tem title "Big Box (F11 ou Select + A no controle)"`; `ATB-013/hintsFor em nenhuma tela nem tema contém "Select+A"`;
  - `ATB-015/App.invariants: o pedido externo entra no Big Box sem chamar LaunchGame, pausa nem fechar jogo`;
  - `BBX-057/Select apertado: A não emite accept; B e Y continuam ignorados; ↑/↓ do direcional (botões 12 e 13) contam como soltos, inclusive a borda de subida com Select apertado, que não emite depois que ele é solto; X, LB/RB, ←/→ e o analógico valem`;
  - `BBX-057/sem Select, A, B, Y e o direcional funcionam como antes`.

## T28 — Frontend: pedido de Big Box, dica e Select segurado
- Requisitos: ATB-008, ATB-009, ATB-010, ATB-011, ATB-012, ATB-013, ATB-015, BBX-057. **Responsável:** implementer. Dependência: T27 (e T26 para o binding real).
- Arquivos: `frontend/src/lib/bigbox.ts` (novo), `frontend/src/App.tsx` (`requestBigBox`, efeito de montagem, `EventsOn('ui:bigBoxRequested')`, `title` do botão), `frontend/src/bigbox/useBigBoxInput.ts` (Select bloqueia A e ↑/↓ do direcional).
```ts
export async function takeBigBoxRequest(): Promise<boolean> // binding lido na hora da chamada; falha ou ausência → false
// App: requestBigBox(): if (bigBox) return; if (anyModalOpen) return; enterBigBoxShell(); void bringToFront()
```
- `requestBigBox` lê `bigBox` e `anyModalOpen` por ref (o evento chega fora do ciclo de render). `bringToFront` chamado depois de entrar. Em `App.tsx`, as mudanças desta tarefa e as da T20 e T36 não podem andar em paralelo.

## T29 — Testes: contrato de tema v2, grade 2D e dados (lógica pura)
- Requisitos: BBX-047, BBX-048, BBX-049, BBX-050, BBX-051, BBX-054, BBX-055, BBX-057 (continuação), BBX-056. **Responsável:** test-writer. Dependência: T27 (mesmo arquivo `useBigBoxInput.test.tsx`).
- Arquivos: `frontend/src/bigbox/nav.test.ts`, `viewModel.test.ts`, `useBigBoxData.test.tsx`, `useBigBoxInput.test.tsx`, `hints.test.ts`, `themes/registry.test.ts`, `themes/faixa/Faixa.test.tsx` (regressão do Faixa).
- Testes:
  - `BBX-048/reducer: vetores de temas-v2 §3 com C = 5 (T 23 f 2 down → 7; f 17 down → 22; f 20 down → 20; f 3 up → 3; f 9 next → 10; f 22 next → 22; T 40 L 25 f 18 down → 23; f 22 down → 22)`; `BBX-048/M é limitado por L e ←/→ ficam em [0, M]`; `BBX-048/nos eixos horizontal e vertical nada muda (up/down sem efeito em Navegar, C = 1)`; `BBX-048/na tela Detalhes as direções seguem BBX-032..BBX-034 em qualquer eixo`; `BBX-048/useBigBoxInput: eixo grid emite up e down em Navegar e prev/next para ←/→; horizontal e vertical como antes`;
  - `BBX-049/gridWindow: vetores de temas-v2 §3.2 (início, meio, fim, fileira parcial, T 4, T 0) e nunca mais que 30 cartões; recortada em L`;
  - `BBX-050/gridPageSize(5, 1) = 25 e limitado a 500; shouldPrefetchGrid pede a página quando L < min(T, (r + R + 1 + 3)·C) e não com exhausted; faixa: pageSize e shouldPrefetch inalterados`;
  - `BBX-051/stripWindow com âncora: vetores de temas-v2 §4 (40/3/12/A2 → 1..14; 40/3/4/A1 → 2..7; 40/0/4/A1 → 0..5; 40/39/4/A1 → 36..40) e o padrão continua 2`;
  - `BBX-054/buildTabs com tabs collections: Todos, Favoritos (favorite true), Concluídos (statuses completed) e depois as plataformas; abas platforms como em BBX-004; aba inicial = plataforma da janela normal ou Todos`; `BBX-054/hintsFor: LB/RB "Aba" com collections e "Plataforma" com platforms`;
  - `BBX-055/useTabGames envia o sort do tema (title ou lastPlayed) e a chave de consulta inclui o sort`;
  - `BBX-047/cada tema do registro declara id, name, available, axis, columns, anchor, listCapacity, backdrop, videoSlot, tabs, sort e timeToBeatInBrowse`;
  - `BBX-056/viewModel: modelo com tabTotal, sortLabel, videoFailed, favorite, lastPlayedRelative, screenshots, timeToBeat e timeToBeatSource; relativeDays: hoje, ontem, há N dias (2..29), há N meses (30..364; 12 meses → há 1 ano), há N anos, sem data "Nunca jogado"`.

## T30 — Frontend: contrato v2, grade e dados (lógica pura)
- Requisitos: BBX-047, BBX-048, BBX-049, BBX-050, BBX-051, BBX-054, BBX-055, BBX-057, BBX-056. **Responsável:** implementer. Dependência: T29, T28 (mesmo `useBigBoxInput.ts`).
- Arquivos: `frontend/src/bigbox/themes/types.ts`, `themes/registry.ts` (Faixa com `anchor 2`, `backdrop contain`, `videoSlot background`, `tabs platforms`, `sort title`, `timeToBeatInBrowse false`; `roda` indisponível), `nav.ts`, `viewModel.ts`, `useBigBoxData.ts`, `useBigBoxInput.ts`, `hints.ts`.
```ts
export type BigBoxThemeDef = { /* temas-v2 §1: axis, columns, anchor, listCapacity, gridRows?, backdrop, videoSlot, tabs, sort, timeToBeatInBrowse, Component?, Details? */ }
export type BigBoxThemeProps = { model; details; onPick; onPlay; onDetails; onVideoError: (gameId: number) => void }
export type NavCtx = { /* ... */ columns: number }
export const GRID_MAX_ROWS = 5, GRID_MAX_NODES = 30, PAGE_EXTRA = 3, PAGE_MAX = 500
export function stripWindow(T: number, f: number, V: number, anchor?: number): { start: number; end: number }
export function gridWindow(T: number, f: number, C: number, R: number): { start: number; end: number }
export function gridPageSize(C: number, R: number): number
export function shouldPrefetchGrid(L: number, T: number, f: number, C: number, R: number, exhausted: boolean): boolean
export function buildTabs(catalog: PlatformCatalog, initialPlatformId: number | null, kind: 'platforms' | 'collections'): { tabs: BigBoxTab[]; initial: number }
export function relativeDays(iso: string | null, now: Date): string
export function useTabGames(tab: BigBoxTab, sort: SortKey, paging: { kind: 'strip'; capacity: number } | { kind: 'grid'; columns: number; rows: number }): TabGames
export function hintsFor(/* ... */, tabsLabel: 'Plataforma' | 'Aba'): Hint[]
```
- O `registry.ts` ganha `mosaico` e `estante` nas T34 e T36; aqui só `faixa` e `roda`. Os testes do Faixa existentes (BBX-006, BBX-007, BBX-038, BBX-039) devem passar sem mudança.

## T31 — Testes: shell, fundo, vídeo na capa, Detalhes comum e varreduras
- Requisitos: BBX-047, BBX-050, BBX-052, BBX-053, BBX-055, BBX-056, BBX-058, BBX-059, BBX-003, TTB-013, TTB-016, TTB-017, TTB-039. **Responsável:** test-writer. Dependência: T30 (assinaturas) e T18.
- Arquivos: `frontend/src/bigbox/BigBox.test.tsx`, `BackgroundStage.test.tsx`, `FocusVideo.test.tsx` (novo), `useVideoDelay.test.tsx` (novo), `TimeToBeatList.test.tsx` (novo), `BigBoxDetails.test.tsx`, `bigbox-scan.test.ts`, `frontend/src/theme.test.ts` (blocos do Faixa), `Panel.test.tsx` se `Panel` mudar.
- Testes:
  - `BBX-047/shell com um tema falso para cada combinação de eixo (horizontal, vertical, grid) navega pelo mesmo reducer e o tema recebe só o modelo e as ações`;
  - `BBX-050/o shell escolhe gridPageSize e shouldPrefetchGrid no eixo grid; com ListGames controlado por promessas, andar para baixo pede cada página uma vez e a janela nunca passa de L; troca de aba cancela`;
  - `BBX-055/o shell envia o sort do tema no ListGames`;
  - `BBX-056/tema falso com Details próprio: mesmas transições de foco (BBX-032..BBX-036), uma única marca de foco e as mesmas dicas; sem Details vale BigBoxDetails`;
  - `BBX-052/BackgroundStage: contain desenha a arte inteira sobre o preenchimento desfocado; cover usa object-fit cover sem desfoque com degradê (scrim → bg); plain desenha só --arc-bb-bg, sem arte e sem vídeo; transição de 300 ms em contain e cover`;
  - `BBX-053/com videoSlot cover o fundo nunca toca vídeo e FocusVideo toca dentro da capa em foco depois de 2.000 ms parado, só em Navegar e fora da suspensão, com um único <video>, src removido ao trocar de foco, sem nova tentativa depois de error enquanto o foco não sair e Y liga e desliga o som; com background vale BBX-011 como está`;
  - `BBX-053/useVideoDelay: false antes de 2 s e depois de failed, true depois de 2 s com active; reinicia ao trocar de jogo`;
  - `BBX-059/o shell pede fetch true só ao abrir os Detalhes e, com timeToBeatInBrowse, depois de 1.000 ms com o mesmo jogo em foco; durante a suspensão nada é pedido`;
  - `TTB-016/os Detalhes do Big Box mostram "Tempo para zerar" com os três rótulos e valores e a linha da fonte, pedem fetch true uma vez ao abrir, e não têm botão de atualizar nem de confirmar`;
  - `TTB-013/com binding que falha, os três rótulos ficam com "—", sem role alert (Detalhes do Big Box)`; `TTB-017/timeToBeat:changed do jogo mostrado relê uma vez nos Detalhes do Big Box`; `TTB-039/"Fonte: HowLongToBeat" ou "Fonte: IGDB" nos Detalhes do Big Box`;
  - `BBX-003/varredura: bigbox/ não importa lib/timeToBeatEdit nem usa TimeToBeatRefresh, TimeToBeatSearch, TimeToBeatChoose ou TimeToBeatSetSettings (nem refreshTimeToBeat, searchTimeToBeat, chooseTimeToBeat, setTimeToBeatSettings)`; `BBX-059/varredura: o único acesso ao tempo para zerar em bigbox/ é TimeToBeatGet pelo módulo de lib/`;
  - `BBX-058/o bloco [data-bigbox-theme="faixa"] define todos os tokens de temas-v2 §7, expostos ao Tailwind por @theme inline, e passa nos pares de contraste do Faixa`.

## T32 — Frontend: shell v2, fundo e vídeo por tema, `TimeToBeatList`, Detalhes
- Requisitos: BBX-047, BBX-050, BBX-052, BBX-053, BBX-055, BBX-056, BBX-058, BBX-059, TTB-016, TTB-013, TTB-017, TTB-039. **Responsável:** implementer. Dependência: T31, T30, T18, T28.
- Arquivos novos: `frontend/src/bigbox/useVideoDelay.ts`, `FocusVideo.tsx`, `CoverArt.tsx` (tirado de `FaixaStrip`), `TimeToBeatList.tsx`. Alterados: `BackgroundStage.tsx` (`backdrop: 'contain' | 'cover' | 'plain'`; vídeo só com `videoSlot 'background'`; usa `useVideoDelay`), `BigBoxDetails.tsx` (tempo para zerar, fonte, `timeToBeat` e `timeToBeatSource` nas props; exporta o `Video` do visor e a rolagem da descrição para a Estante), `BigBox.tsx` (janela, pré-busca e `useTabGames` pelo eixo; `Details = theme.Details ?? BigBoxDetails`; `useTimeToBeat` com `fetchAfterMs` conforme design §4.2; `onVideoError`), `themes/faixa/*` (usa `CoverArt`, `anchor 2`), `frontend/src/index.css` (os quatro tokens novos e o bloco completo do Faixa; `@theme inline`).
```ts
export function useVideoDelay(o: { gameId: number | null; videoUrl: string | null; active: boolean; failed: boolean }): boolean
export function FocusVideo(p: { gameId: number; url: string; active: boolean; sound: boolean; failed: boolean; onError: (gameId: number) => void }): JSX.Element | null
export function CoverArt(p: { game: BigBoxGame }): JSX.Element
export function TimeToBeatList(p: { rows: TimeToBeatRow[]; source: string | null }): JSX.Element
export type BigBoxDetailsProps = { /* fase 13 */ timeToBeat: TimeToBeatRow[]; timeToBeatSource: string | null }
```
- O Faixa não muda de comportamento: os testes de BBX-006 a BBX-013, BBX-038 e BBX-039 passam sem alteração.

## T33 — Testes: tema Mosaico
- Requisitos: MOS-001, MOS-002, MOS-003, MOS-004, MOS-005, MOS-006, MOS-007, MOS-008, MOS-009, MOS-010, MOS-011, MOS-012, MOS-013, MOS-014, MOS-015, BBX-058, BBX-051, BBX-053. **Responsável:** test-writer. Dependência: T32 (assinaturas).
- Arquivos: `frontend/src/bigbox/themes/mosaico/Mosaico.test.tsx`, `capacity.test.ts` (novos); `frontend/src/theme.test.ts` (bloco do Mosaico e contraste).
- Testes:
  - `MOS-001/de cima para baixo: abas com a contagem, título, fileira de capas, painel e rodapé, dentro da área segura; sem arte de fundo (bb-art) e sem menu lateral`;
  - `MOS-002/pílulas: --arc-bb-surface com --arc-bb-text-muted; a ativa com --arc-bb-accent e --arc-bb-on-accent; LB/RB trocam de aba`;
  - `MOS-003/"120 jogos" e "1 jogo" com T = 1; nada em "Carregando jogos…" nem em aba vazia`;
  - `MOS-004/cartões quadrados com a imagem inteira (contain); a capa em foco com escala 1,09 e contorno de 5 px afastado 4 px; ←/→ sem dar a volta; janela com âncora 1 e no máximo 15 cartões no DOM; clique foca e duplo clique lança`;
  - `MOS-005/sem capa usa a substituta; sem nenhuma imagem, cartão --arc-bb-placeholder com o título`;
  - `MOS-006/título do jogo em foco acima da fileira, uma linha, --arc-bb-heading, muda a cada foco; marcação "Em execução" ou "Pausado" ao lado`;
  - `MOS-007/o vídeo toca dentro da capa em foco pelas regras de BBX-053, no máximo um <video>, dentro do cartão em foco`;
  - `MOS-008/painel com três colunas; a primeira mostra "plataforma · ano · gênero" e a descrição curta (≤ 280 caracteres) em no máximo 4 linhas`;
  - `MOS-009/segunda coluna: Tempo jogado, Última vez (data ou "Nunca jogado") e Sessões`;
  - `MOS-010/segunda coluna: os três tempos para zerar com "—" sem dado, lidos com cache na hora e busca depois de 1 s parado (temporizadores falsos)`;
  - `MOS-011/terceira coluna: até 3 miniaturas na ordem de media.list; com menos só as que existem; sem nenhuma a coluna some e as outras duas ocupam o painel`;
  - `MOS-012/mosaicoCapacity: 1280×720 → 4; 1920×1080 → 4; 3840×2160 → 5; 2560×1080 → 6; 0×0 → 12; limitada a 1..14`;
  - `MOS-013/"Carregando jogos…", aba vazia ("Nenhum jogo nesta plataforma" e "Nenhum jogo na biblioteca"), aviso de erro e marcação de sessão no cartão, dentro de painéis`;
  - `MOS-014/Detalhes: data-bigbox-theme="mosaico" na raiz e BigBoxDetails montado, com o tempo para zerar`;
  - `MOS-015/contraste do Mosaico: text, text-muted, heading e danger ≥ 4,5:1 com panel e com bg; on-accent/accent e on-chip/chip ≥ 4,5:1; focus ≥ 3:1 contra bg e panel`;
  - `BBX-058/o bloco [data-bigbox-theme="mosaico"] define todos os tokens de temas-v2 §7 e passa nos pares do Mosaico`.

## T34 — Frontend: tema Mosaico
- Requisitos: MOS-001, MOS-002, MOS-003, MOS-004, MOS-005, MOS-006, MOS-007, MOS-008, MOS-009, MOS-010, MOS-011, MOS-012, MOS-013, MOS-014, MOS-015, BBX-058, BBX-051. **Responsável:** implementer. Dependência: T33.
- Arquivos: `frontend/src/bigbox/themes/mosaico/Mosaico.tsx`, `MosaicoRow.tsx`, `MosaicoPanel.tsx`, `capacity.ts` (novos), `frontend/src/bigbox/themes/registry.ts` (entrada `mosaico`, disponível, `anchor 1`, `backdrop plain`, `videoSlot cover`, `timeToBeatInBrowse true`), `frontend/src/index.css` (bloco `[data-bigbox-theme="mosaico"]`).
```ts
export function mosaicoCapacity(vp: { width: number; height: number }): number // temas-v2 §5.2, 1..14, unknown → 12
export function Mosaico(p: BigBoxThemeProps): JSX.Element
```
- O tema só desenha o modelo e repassa cliques (BBX-003). Os componentes compartilhados (`Panel`, `FocusVideo`, `CoverArt`, `TimeToBeatList`) vêm de `bigbox/`.

## T35 — Testes: tema Estante, registro e seletor
- Requisitos: ETN-001, ETN-002, ETN-003, ETN-004, ETN-005, ETN-006, ETN-007, ETN-008, ETN-009, ETN-010, ETN-011, ETN-012, ETN-013, ETN-014, BBX-046, BBX-058, BBX-052, BBX-054, TTB-016. **Responsável:** test-writer. Dependência: T32 (assinaturas), T34 (registro).
- Arquivos: `frontend/src/bigbox/themes/estante/Estante.test.tsx`, `EstanteDetails.test.tsx`, `capacity.test.ts` (novos); `frontend/src/bigbox/themes/registry.test.ts`, `frontend/src/lib/uiPrefs.test.ts`, `frontend/src/App.test.tsx` (cabeçalho), `frontend/src/theme.test.ts` (bloco da Estante).
- Testes:
  - `ETN-001/Navegar com foco na fileira 0: destaque, cabeçalho "Sua biblioteca", grade com a fileira de baixo cortada e rodapé; fundo cover com o vídeo de fundo de BBX-011`;
  - `ETN-002/destaque: título em texto, "Última sessão há 7 dias" (relógio falso: hoje, ontem, há N dias, há N meses, há N anos) ou "Nunca jogado", botão "Iniciar jogo" age como A, "Detalhes" age como X, coração cheio com favorite e vazio sem, sem onClick; marcação de sessão ao lado do título`;
  - `ETN-003/blocos "Tempo jogado" e "Sessões" no destaque`;
  - `ETN-004/cabeçalho: "Sua biblioteca", rótulo "Jogado recentemente", selos LB e RB, aba ativa em --arc-bb-accent com o total entre parênteses`;
  - `ETN-005/5 colunas; cartão com capa 2:3 contain, título em uma linha, "Jogou por <tempo>" ou "Nunca jogado"; foco com contorno --arc-bb-focus e escala 1,05; selo de sessão na capa; clique foca e duplo clique lança`;
  - `ETN-006/shell com controle falso: ↓ da fileira 0 vai à 1; → no fim da fileira passa à seguinte; A lança, X abre Detalhes, LB/RB trocam de aba, Y liga e desliga o som, B sai`;
  - `ETN-007/destaque presente com r = 0 e ausente com r = 1; cabeçalho sempre presente; troca animada em até 300 ms`;
  - `ETN-008/estanteRows: 1280×720 → 1; 1920×1080 → 1; 3840×2160 → 1; 1920×1200 → 1; 0×0 → 2; limitada a 1..5; capacidade 5 × R`;
  - `ETN-009/"Carregando jogos…", aba vazia, aviso de erro e marcação de sessão dentro de painéis, com cabeçalho e abas sempre visíveis`;
  - `ETN-010/Detalhes próprios: título grande, descrição com rolagem, botões "Iniciar jogo" e "Jogar sem contar tempo", faixa "Jogado por <tempo> · Jogado recentemente <relativo>" (ou "Nunca jogado"), carrossel à esquerda (~62 %) e painéis à direita (~38 %), tudo dentro da área segura e só a descrição rola`;
  - `ETN-011/carrossel: visor com o item inteiro (contain), uma bolinha por item (selecionada cheia, outras vazadas, aria-current), ←/→ trocam o item, vídeo no visor, uma única marca de foco; sem galeria o carrossel some e ↓ não faz nada`;
  - `ETN-012/coluna da direita: "Estatísticas do jogo", "Tempo para zerar" (três valores, "—" sem dado, fonte) e "Sobre" com cada linha omitida quando vazia`;
  - `TTB-016/os Detalhes da Estante mostram o tempo para zerar e pedem fetch true uma vez ao abrir`;
  - `ETN-013/nenhum destes textos ou controles no DOM: logotipo, "Gerenciar", favoritar, busca, seletor de ordem, "Todos os jogos", alternância lista/grade, "Modernos", "Clássicos", conquistas, avaliação, downloads, "jogando agora", requisitos do sistema, "Ver similares", "Co-Op", "Vs" e menu lateral`;
  - `ETN-014/contraste da Estante: text, text-muted e danger ≥ 4,5:1 com panel composto sobre branco e sobre preto e com bg; on-accent/accent e on-chip/chip ≥ 4,5:1; focus ≥ 3:1 contra panel composto e bg`;
  - `BBX-058/o bloco [data-bigbox-theme="estante"] define todos os tokens de temas-v2 §7 e passa nos pares da Estante`;
  - `BBX-052/a Estante usa fundo cover (arte em tela cheia com degradê) e o Mosaico fundo plain`; `BBX-054/Estante: abas Todos, Favoritos e Concluídos e depois as plataformas, com "Nenhum jogo nesta aba" e total entre parênteses`;
  - `BBX-046/o registro lista faixa, mosaico e estante, nesta ordem, disponíveis, e roda indisponível; o seletor do cabeçalho lista os quatro pelo nome do registro com "Roda (em breve)" desativado; a escolha vai para localStorage (arcadia.bigBoxTheme); valor ausente, desconhecido ou indisponível vira faixa (uiPrefs)`.

## T36 — Frontend: tema Estante, registro final e seletor
- Requisitos: ETN-001, ETN-002, ETN-003, ETN-004, ETN-005, ETN-006, ETN-007, ETN-008, ETN-009, ETN-010, ETN-011, ETN-012, ETN-013, ETN-014, BBX-046, BBX-058, BBX-052, BBX-054, TTB-016. **Responsável:** implementer. Dependência: T35.
- Arquivos novos: `frontend/src/bigbox/themes/estante/Estante.tsx`, `EstanteHero.tsx`, `EstanteHeader.tsx`, `EstanteGrid.tsx`, `EstanteDetails.tsx`, `capacity.ts`. Alterados: `themes/registry.ts` (`estante`: disponível, grid de 5 colunas, `backdrop cover`, `videoSlot background`, `tabs collections`, `sort lastPlayed`, `Details: EstanteDetails`; exporta `BIGBOX_THEMES`), `frontend/src/index.css` (bloco da Estante), `frontend/src/lib/uiPrefs.ts` (`BigBoxThemeId` ganha `estante`), `frontend/src/App.tsx` (opções do seletor vêm de `BIGBOX_THEMES`, com `t.name` e "(em breve)" quando indisponível; some o mapeamento fixo de nomes).
```ts
export const ESTANTE_COLUMNS = 5, ESTANTE_FOCUS_SCALE = 1.05, HERO_FADE_MS = 300
export function estanteRows(vp: { width: number; height: number }): number // temas-v2 §6.2, 1..5, unknown → 2
export function Estante(p: BigBoxThemeProps): JSX.Element
export function EstanteDetails(p: BigBoxDetailsProps): JSX.Element
```
- A Estante decide `heroVisible = Math.floor(model.focus / model.columns) === 0`.

## T37 — Documentação, ADRs e versão
- Requisitos: ATB-013, TTB-024, BBX-046, BBX-058. **Responsável:** sessão principal (docs; `wails.json` e `version` são mecânicos). Dependência: T36, T28, T26, T20, T16, T14.
- Arquivos: `README.md`, `CLAUDE.md` ("Spec atual" aponta para `phase-15-big-box-v2`; comando de build e versão; corrigir "inicia com o Windows", ADR 0024, Dúvidas 10), `docs/ROADMAP.md`, `docs/GUIA-DO-PROJETO.md`, `docs/ARCHITECTURE.md` (terceiro combo, `--bigbox`, serviço `timeToBeat`, tema v2), `docs/adr/0038-big-box-por-pedido-externo.md`, `docs/adr/0039-tempo-para-zerar-hltb.md`, `docs/adr/0040-contrato-de-tema-v2.md`, `docs/adr/README.md`, `cmd/arcadia/wails.json` (versão 1.1.0, Dúvidas 9), `docs/specs/phase-15-big-box-v2/{ipc-v0.16.0-delta.md,design.md}` (trocar "(tarefa)" pelo arquivo da T10), a anotação "alterado em 2026-11-22" nos requisitos de fases anteriores listados na tabela de afetados que ainda não a tenham, e `docs/specs/phase-15-big-box-v2/contracts/hltb-rules.md` §9 já preenchido pela T05.

## T38 — Verificação final e roteiro manual
- Requisitos: ATB-016, TTB-031, TTB-032, MOS-016, ETN-015. **Responsável:** sessão principal, com o usuário (parte manual).
- Automático: `go build ./...`, `go vet ./...`, `CGO_ENABLED=0 go build ./...`, `go test ./... -race` (sem internet para TTB-031), `npm test` e `npx tsc --noEmit` em `frontend/`, `scripts/spec-trace.ps1 -Spec docs/specs/phase-15-big-box-v2`, `wails build -ldflags "-X main.version=1.1.0"` (regera `frontend/wailsjs/go/**`; entra no commit).
- Manual (os quatro requisitos "verificação: manual"):
  - **ATB-016:** Select + A por 1 s com a janela fechada, atrás de outro programa, minimizada, já no Big Box, com jogo rodando (som de recusa) e pelo Moonlight/Sunshine; toque curto não faz nada; sem aviso do UAC.
  - **TTB-032:** cinco jogos conhecidos e um obscuro no painel, Detalhes do Big Box, lote após importar, "Atualizar" e "Escolher outro…" na edição; sem internet só "—".
  - **MOS-016:** Mosaico na TV com controle (LB/RB, fileira, vídeo na capa, tempo para zerar, X e A; legível a 3 m).
  - **ETN-015:** Estante na TV (grade nas quatro direções, destaque some ao descer e volta, LB/RB entre Todos, Favoritos, Concluídos e plataformas, X com carrossel e tempo para zerar, A lança, Y liga o som).
  - Conferir o Primeiro plano negado pelo Windows (Limitações 3 da spec) e `SecondInstanceData.Args` no executável real (T26).

---

## Riscos de concorrência e recursos

- **Goroutines novas:** só o trabalhador do `timeToBeatService` (T12). Dono: o serviço. Saída: `s.ctx` (cancelado em `Close`). Espera: `wg` em `Close`, chamado no shutdown do agente **antes** de fechar o store (T14). O roteador e o poller não ganham goroutine (T22, T24). No frontend, só temporizadores e assinaturas `EventsOn`, limpos ao desmontar (T18, T28).
- **Ordem de shutdown:** `BeginShutdown` do serviço no mesmo instante do tracker (para de notificar e descarta as filas); depois, o servidor IPC encerra; `Close` do serviço (cancela a requisição em curso sem gravar); por fim, o store. `shuttingDown` do roteador é um `atomic.Bool` ligado no mesmo ponto.
- **Locks:** `reqMu` serializa toda requisição externa (HowLongToBeat e IGDB da reserva, inclusive `Refresh` e `Search`); `mu` guarda só filas, mapas e recuos e **nunca** é segurado durante I/O nem ao chamar o notificador. Ordem: `reqMu` antes de `mu`, nunca o contrário. O `Limiter` do IGDB é compartilhado com a busca de metadados (MET-010).
- **Prazos:** `Refresh`/`Search` esperam `reqMu`; o prazo de 25 s vence antes do pior caso (descoberta + busca + nova descoberta) e devolve `-32011` sem prender a goroutine do IPC.
- **Handles do Windows:** nenhum novo. O processo do `--bigbox` é criado por `winapi.CreateDetached`, que fecha `hProcess` e `hThread` dentro dele.
- **Pedido de Big Box:** `bigBoxReq` guardado sob `a.mu`; `TakeBigBoxRequest` lê e zera sob o mesmo lock (teste com `-race`, T25). O evento Wails só é emitido com `wailsCtx` definido.
- **Notificações:** `timeToBeat.changed` usa a fila por conexão com descarte; uma conexão lenta não bloqueia o worker.

## Riscos e dúvidas de implementação

1. **Endpoint do HowLongToBeat.** Todo o cliente depende de [rules §2..§4](contracts/hltb-rules.md), que ninguém verificou. A T05 vem antes da T06 por isso. Se o site exigir token ou passo prévio, é regra visível e volta à spec. Pior caso após a entrega: "—" em toda parte até uma versão nova; o recuo de até 1 h protege contra bloqueio.
2. **Termos de uso e bloqueio.** Uma requisição por vez, 2 s entre elas, 1 s de foco parado na UI, cache longo (inclusive `not-found`). Lote de 1.000 jogos ≈ 35 min em segundo plano; o `refresh` do usuário ignora o recuo, então "Atualizar" repetido conta falha por fonte.
3. **`Args` da segunda instância e `flag`.** Ver Dúvidas 3. Testar também `wails dev`, que passa flags próprios.
4. **Primeiro plano negado.** O agente pede o Big Box sem foco; a primeira instância força (`bringToFront`) e a nova pede `BringToFront` depois de entrar. Se o Windows negar, a janela fica no topo e o botão pisca (Limitações 3): não é erro e não deve gerar `Warn` ruidoso.
5. **Select + A dentro do Big Box.** O gatilho do agente chega à segunda instância e ATB-010 não muda nada; sem BBX-057 a UI lançaria o jogo em foco. T27 e T28 precisam ser entregues antes de liberar a fatia D.
6. **Grade 2D e virtualização.** No máximo 30 cartões no DOM (`GRID_MAX_NODES`); `useCardImages` recebe os ids da janela; a janela recorta em `L`. Cuidar de: `M = min(T, L) − 1` ao navegar com a página atrasada; troca de aba com leitura em voo (cancelar); o foco inicial de BBX-027 só olha a primeira página (ordem `lastPlayed` pode não ter o jogo da janela normal nela); escala do cartão em foco sem estourar a área segura; aba de coleção vazia ainda aparece.
7. **Vídeo na capa.** `FocusVideo` dentro de um cartão que muda de escala: o `<video>` usa `contain` e deve respeitar o limite de um `<video>` na tela. O `BackgroundStage` passa a usar `useVideoDelay`; os testes de BBX-011..BBX-013 do Faixa têm de continuar verdes.
8. **Abas de coleção.** `GameListResult.total` da primeira página dá `T` para "Favoritos" e "Concluídos"; o catálogo não tem `gameCount`. Mapear "Concluídos" para `statuses: ['completed']` (Dúvidas 4).
9. **Reserva do IGDB.** Dois limitadores compartilhados (metadados e tempo para zerar) se atrasam mutuamente, sem passar do limite. A falta de chave é lida do `scrapers.Keys` sem requisição. Credencial recusada conta como falha temporária (recuo próprio de 1, 5, 30, 60 min).
10. **Testes que quebram por contrato:** a nova assinatura de `newTriggerRouter`, o `routerTracker` com `Current()`, `NavCtx.columns` e `BigBoxThemeProps.onVideoError` obrigatório mexem em muitos testes existentes. Cada tarefa de teste lista os que ela reescreve; a de código só começa com o conjunto compilando.
11. **Relativo e fuso (`relativeDays`).** Diferença em dias de calendário **locais**; os testes usam `vi.setSystemTime` e um fuso fixo (`TZ`), senão ficam instáveis na virada do dia.
12. **`FakeClock` e `context.WithTimeout`.** Prazos reais de contexto não obedecem ao relógio fake; por isso os campos de prazo configuráveis (Dúvidas 5).

## Decisões novas que merecem ADR

Já propostos na spec e escritos na T37: **0038** (Big Box por pedido externo), **0039** (tempo para zerar do HowLongToBeat com reserva do IGDB) e **0040** (contrato de tema v2). Nenhuma decisão nova além dessas; "versão 1.1.0" é só decisão de release, não de arquitetura.

---

## Rastreabilidade

Colunas: tarefa(s) de código, tarefa(s) de testes, arquivo principal do teste. A conferência automática é `scripts/spec-trace.ps1 -Spec docs/specs/phase-15-big-box-v2` (`-SkipTests` antes das tarefas de teste). Verificação manual (sem teste exigido): ATB-016, TTB-032, MOS-016, ETN-015 (T38).

| Requisito | Código | Testes | Arquivo do teste |
|---|---|---|---|
| ATB-001 | T22 | T21 | `internal/input/pad_bigbox_test.go` |
| ATB-002 | T22 | T21 | `internal/input/pad_bigbox_test.go` |
| ATB-003 | T24 | T23 | `internal/agent/bigbox_router_test.go` |
| ATB-004 | T24 | T23 | `internal/agent/bigbox_router_test.go`, `openui_bigbox_test.go` |
| ATB-005 | T24 | T23 | `internal/agent/bigbox_router_test.go` |
| ATB-006 | T24 | T23 | `internal/agent/bigbox_router_test.go` |
| ATB-007 | T24 | T23 | `internal/agent/bigbox_router_test.go` |
| ATB-008 | T26, T28 | T23, T25, T27 | `cmd/arcadia/bigbox_request_test.go`, `App.bigbox-request.test.tsx` |
| ATB-009 | T26, T28 | T25, T27 | `cmd/arcadia/bigbox_request_test.go`, `App.bigbox-request.test.tsx` |
| ATB-010 | T28 | T27 | `App.bigbox-request.test.tsx` |
| ATB-011 | T28 | T27 | `App.bigbox-request.test.tsx` |
| ATB-012 | T26, T28 | T25, T27 | `cmd/arcadia/bigbox_request_test.go`, `lib/bigbox.test.ts` |
| ATB-013 | T28 | T27 | `App.bigbox-request.test.tsx`, `hints.test.ts` |
| ATB-014 | T26 | T25 | `cmd/arcadia/bigbox_request_test.go` |
| ATB-015 | T24, T28 | T23, T25, T27 | `bigbox_router_test.go`, `App.invariants.test.tsx` |
| ATB-016 | (manual) | T38 | roteiro manual |
| TTB-001 | T02 | T01 | `internal/store/migrate10_test.go` |
| TTB-002 | T02, T12 | T01, T11 | `store/time_to_beat_test.go`, `agent/timetobeat_test.go` |
| TTB-003 | T07, T12 | T06, T11 | `scrapers/hltb_test.go`, `agent/timetobeat_test.go` |
| TTB-004 | T04, T12 | T03, T11 | `domain/timetobeat_test.go` |
| TTB-005 | T07 | T06 | `scrapers/hltb_test.go` |
| TTB-006 | T04, T12 | T03, T11 | `domain/timetobeat_test.go`, `agent/timetobeat_test.go` |
| TTB-007 | T14 | T13 | `agent/timetobeat_ipc_test.go` |
| TTB-008 | T14 | T08, T13 | `agent/timetobeat_ipc_test.go` |
| TTB-009 | T12 | T11 | `agent/timetobeat_test.go` |
| TTB-010 | T07, T12, T14 | T06, T11 | `scrapers/hltb_test.go`, `agent/timetobeat_test.go` |
| TTB-011 | T04, T07, T12 | T03, T06, T11 | `domain/timetobeat_test.go`, `agent/timetobeat_test.go` |
| TTB-012 | T05, T07 | T06 | `scrapers/hltb_test.go` |
| TTB-013 | T18, T20, T32 | T17, T19, T31 | `lib/timeToBeat.test.tsx`, `TimeToBeatBlock.test.tsx`, `BigBoxDetails.test.tsx` |
| TTB-014 | T18 | T17 | `lib/timeToBeat.test.tsx` |
| TTB-015 | T18, T20 | T17, T19 | `TimeToBeatBlock.test.tsx` |
| TTB-016 | T18, T32, T36 | T17, T31, T35 | `BigBoxDetails.test.tsx`, `EstanteDetails.test.tsx` |
| TTB-017 | T18, T20, T32 | T17, T19, T31 | `lib/timeToBeat.test.tsx` |
| TTB-018 | T20 | T17, T19 | `TimeToBeatEditor.test.tsx` |
| TTB-019 | T20 | T19 | `TimeToBeatEditor.test.tsx` |
| TTB-020 | T14 | T08, T13 | `agent/timetobeat_ipc_test.go` |
| TTB-021 | T14 | T08, T13 | `agent/timetobeat_ipc_test.go` |
| TTB-022 | T14 | T08, T13 | `agent/timetobeat_ipc_test.go` |
| TTB-023 | T14 | T08, T13 | `agent/timetobeat_ipc_test.go` |
| TTB-024 | T20, T37 | T19 | `ProviderKeysDialog.test.tsx` |
| TTB-025 | T14 | T13 | `agent/timetobeat_ipc_test.go` |
| TTB-026 | T12, T14 | T11, T13 | `agent/timetobeat_test.go`, `timetobeat_ipc_test.go` |
| TTB-027 | T05, T07 | T06 | `scrapers/hltb_test.go` |
| TTB-028 | T12, T14 | T11, T13 | `agent/timetobeat_test.go`, `shutdown_test.go` |
| TTB-029 | T09, T10 | T08 | `internal/ipc/contract_phase15_test.go` |
| TTB-030 | T16 | T15, T19 | `cmd/arcadia/timetobeat_test.go`, `App.invariants.test.tsx` |
| TTB-031 | T07, T12, T38 | T06, T11 | `scrapers/hltb_test.go`, `agent/timetobeat_test.go` |
| TTB-032 | (manual) | T38 | roteiro manual |
| TTB-033 | T04, T12 | T03, T11 | `domain/timetobeat_test.go`, `agent/timetobeat_test.go` |
| TTB-034 | T04, T12 | T03, T11 | `domain/timetobeat_test.go`, `agent/timetobeat_test.go` |
| TTB-035 | T12 | T11 | `agent/timetobeat_test.go` |
| TTB-036 | T05, T07, T12 | T06, T11 | `scrapers/igdb_ttb_test.go` |
| TTB-037 | T07, T12 | T06, T11 | `scrapers/igdb_ttb_test.go`, `agent/timetobeat_test.go` |
| TTB-038 | T04, T07, T12 | T03, T06, T11 | `domain/timetobeat_test.go`, `agent/timetobeat_test.go` |
| TTB-039 | T18, T20, T32 | T17, T19, T31 | `lib/timeToBeat.test.tsx`, `TimeToBeatBlock.test.tsx` |
| TTB-040 | T14, T20 | T13, T19 | `agent/timetobeat_ipc_test.go`, `TimeToBeatEditor.test.tsx` |
| BBX-046 | T36 | T35 | `themes/registry.test.ts`, `lib/uiPrefs.test.ts` |
| BBX-047 | T30, T32 | T29, T31 | `themes/registry.test.ts`, `BigBox.test.tsx` |
| BBX-048 | T30 | T29 | `bigbox/nav.test.ts`, `useBigBoxInput.test.tsx` |
| BBX-049 | T30 | T29 | `bigbox/viewModel.test.ts` |
| BBX-050 | T30, T32 | T29, T31 | `viewModel.test.ts`, `BigBox.test.tsx` |
| BBX-051 | T30, T34 | T29, T33 | `viewModel.test.ts` |
| BBX-052 | T32, T36 | T31, T35 | `BackgroundStage.test.tsx` |
| BBX-053 | T32, T34 | T31, T33 | `FocusVideo.test.tsx`, `useVideoDelay.test.tsx` |
| BBX-054 | T30, T36 | T29, T35 | `viewModel.test.ts`, `hints.test.ts` |
| BBX-055 | T30, T32 | T29, T31 | `useBigBoxData.test.tsx`, `BigBox.test.tsx` |
| BBX-056 | T30, T32 | T29, T31 | `BigBox.test.tsx` |
| BBX-057 | T28 | T27, T29 | `useBigBoxInput.test.tsx` |
| BBX-058 | T32, T34, T36 | T31, T33, T35 | `frontend/src/theme.test.ts` |
| BBX-059 | T32 | T31 | `bigbox-scan.test.ts`, `BigBox.test.tsx` |
| MOS-001 | T34 | T33 | `themes/mosaico/Mosaico.test.tsx` |
| MOS-002 | T34 | T33 | `themes/mosaico/Mosaico.test.tsx` |
| MOS-003 | T34 | T33 | `themes/mosaico/Mosaico.test.tsx` |
| MOS-004 | T34 | T33 | `themes/mosaico/Mosaico.test.tsx` |
| MOS-005 | T34 | T33 | `themes/mosaico/Mosaico.test.tsx` |
| MOS-006 | T34 | T33 | `themes/mosaico/Mosaico.test.tsx` |
| MOS-007 | T34 | T33 | `themes/mosaico/Mosaico.test.tsx` |
| MOS-008 | T34 | T33 | `themes/mosaico/Mosaico.test.tsx` |
| MOS-009 | T34 | T33 | `themes/mosaico/Mosaico.test.tsx` |
| MOS-010 | T34 | T33 | `themes/mosaico/Mosaico.test.tsx` |
| MOS-011 | T34 | T33 | `themes/mosaico/Mosaico.test.tsx` |
| MOS-012 | T34 | T33 | `themes/mosaico/capacity.test.ts` |
| MOS-013 | T34 | T33 | `themes/mosaico/Mosaico.test.tsx` |
| MOS-014 | T34 | T33 | `themes/mosaico/Mosaico.test.tsx` |
| MOS-015 | T34 | T33 | `frontend/src/theme.test.ts` |
| MOS-016 | (manual) | T38 | roteiro manual |
| ETN-001 | T36 | T35 | `themes/estante/Estante.test.tsx` |
| ETN-002 | T36 | T35 | `themes/estante/Estante.test.tsx` |
| ETN-003 | T36 | T35 | `themes/estante/Estante.test.tsx` |
| ETN-004 | T36 | T35 | `themes/estante/Estante.test.tsx` |
| ETN-005 | T36 | T35 | `themes/estante/Estante.test.tsx` |
| ETN-006 | T36 | T35 | `themes/estante/Estante.test.tsx` |
| ETN-007 | T36 | T35 | `themes/estante/Estante.test.tsx` |
| ETN-008 | T36 | T35 | `themes/estante/capacity.test.ts` |
| ETN-009 | T36 | T35 | `themes/estante/Estante.test.tsx` |
| ETN-010 | T36 | T35 | `themes/estante/EstanteDetails.test.tsx` |
| ETN-011 | T36 | T35 | `themes/estante/EstanteDetails.test.tsx` |
| ETN-012 | T36 | T35 | `themes/estante/EstanteDetails.test.tsx` |
| ETN-013 | T36 | T35 | `themes/estante/Estante.test.tsx` |
| ETN-014 | T36 | T35 | `frontend/src/theme.test.ts` |
| ETN-015 | (manual) | T38 | roteiro manual |
