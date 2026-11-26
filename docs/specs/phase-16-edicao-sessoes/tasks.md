# Fase 16 — Edição de sessões: tarefas

- **Status:** gerado pelo planner em 2026-11-25, sobre os requisitos aprovados na mesma data (todas as propostas padrão das perguntas 1 a 9: sem nota por sessão; limite de 24 h; caixa "Já está no tempo total" mantida; sobreposição recusada entre todos os jogos; sessão descartada só pode ser excluída; exclusão irreversível com confirmação; SED-037 incluído; textos "Manual", "Importada", "Editada", "adicionados à mão"; versão 1.2.0).
- **Requisitos:** [requirements.md](requirements.md) (SED-001..SED-037, SEL-001..SEL-017) · **Design:** [design.md](design.md) · **Contratos:** [contracts/ipc-v0.17.0-delta.md](contracts/ipc-v0.17.0-delta.md), [contracts/ipc-v0.17.0-delta.openrpc.json](contracts/ipc-v0.17.0-delta.openrpc.json) e `contracts/ipc.openrpc.json` (v0.17.0, criado na T15).
- **ADRs:** 0009, 0010, 0011, 0012, 0015, 0017, 0029, 0031 (existentes); 0041 (proposto, escrito na T35).

## Resumo

Nove fatias entregáveis, cada uma com os testes antes do código (testes vermelhos entre as duas tarefas são esperados):

- **A (T01..T02):** funções puras de `internal/domain` (regras de tempo, sobreposição, teto do total, corte de pausa).
- **B (T03..T14):** migração 11, leituras do store (`source`, `editedAt`, `sessionPlaySeconds`), criar, editar e excluir em transação `BEGIN IMMEDIATE`, e a correção das estatísticas (SED-027 e SED-037).
- **C (T15..T19):** contrato v0.17.0 completo, testes de contrato, tipos do `internal/ipc` e conversões de leitura do agente.
- **D (T20..T21):** handlers `session.create`, `session.update` e `session.delete` no agente.
- **E (T22..T24):** bindings e mensagens do desktop, e regeneração dos bindings do Wails.
- **F (T25..T34):** frontend (formulário puro, hooks, modal, confirmação, lista de sessões, marca nas estatísticas).
- **G (T35..T37):** ADR 0041, documentação e versão 1.2.0.
- **H (T38):** verificação final e roteiro manual (SEL-017).

**Nada muda em `internal/tracker/` nem em `cmd/fake-game/`** (nenhuma tarefa do tracker-specialist), nem em `pause_interval` além do corte de SED-009, `tracking_rule`, `media`, `media_job` e `time_to_beat`. Nenhuma tarefa cria goroutine, handle do Windows ou notificação IPC.

## Dúvidas e lacunas da spec (nenhuma bloqueia; as marcadas "spec-writer" pedem ajuste de texto)

1. **Mensagens de campo vazio, não numérico e minutos fora de 0..59 (spec-writer, SEL-007).** SED-036 só dá texto para os `reason` do agente. A T26 usa estas propostas, a confirmar: "Preencha este campo.", "Informe um número válido." e "Os minutos vão de 0 a 59.". A regra "Tempo jogado maior que o ajuste" (SEL-007 com a caixa marcada) usa o texto de `exceeds-adjustment`.
2. **Faixa do esquema do delta × `reason` (spec-writer, SED-019, SED-031).** O delta declara `activeSeconds` com `minimum: 30` e `maximum: 86400` em `session.create`, mas SED-019 manda responder `-32602` com `reason = active-too-short` (e `active-exceeds-span`). A T21 **não** rejeita por faixa antes de `domain.ValidateSessionTimes`: só valida forma (tipo, membros, `gameId`/`sessionId` ≥ 1, instante RFC 3339 presente) e deixa o `reason` sair da regra. Os limites do esquema são só documentação. Sugestão: dizer isso no delta §1.
3. **`updated_at` do jogo na edição (spec-writer, SED-006, SED-007).** A spec só fixa `updated_at = W` para criar (SED-004) e excluir (SED-010), e "não grava" para pedido sem mudança (SED-008). A T10 segue o design §3.4 passo 9: grava `updated_at` na edição só se algum campo do jogo (`total_play_seconds`, `first_played_at`, `last_played_at`, `play_count`) mudou de valor. Sugestão: acrescentar a frase em SED-007.
4. **`ErrSessionOpen` precisa de dados (design).** SED-022 exige `SessionRef` (título do jogo). A T10 define `SessionOpenError{SessionID, GameID, Title}` que satisfaz `errors.Is(err, ErrSessionOpen)`; o design mostra só o sentinela.
5. **`SessionWrite.Changed` (design).** O agente precisa saber se algo foi gravado (SED-008, SED-029): a T08 acrescenta `Changed bool` em `SessionWrite`. O tipo de linha é `store.SessionItem` ampliado (`Source`, `EditedAt`), no lugar de um `SessionRow` novo.
6. **`ClipPause` (design).** `resumedAt *time.Time` na entrada e `r time.Time` na saída. Em sessão encerrada não há pausa aberta (STO-023 fecha ou apaga), então nil é tratado como "retomada no fim da sessão".
7. **`editedAt` no TypeScript.** O Go envia `""` para nulo (como `endedAt`); o tipo da UI é `string` (`''` = nunca editada).
8. **Sessão importada: intervalo = tempo jogado.** A importada nasce com `started_at = fim − tempo` (IMP-007), então só aumentar o tempo jogado, sem mover o início, dá `active-exceeds-span` (SED-019 vale para ela, SED-021). O formulário (SEL-006) acompanha o intervalo, então o caminho natural é mover o início. Sem mudança na spec; entra no roteiro manual (T38).
9. **Fuso fixo no Vitest (SEL-016).** Não há configuração de TZ no frontend. A T25 fixa `process.env.TZ` no próprio arquivo de teste (Node recalcula o fuso); se ficar instável no Windows, o implementer da T26 adiciona `globalSetup` em `frontend/vite.config.ts`.
10. **Árvore suja antes de começar.** `git status` mostra `frontend/src/components/StatsPage.tsx` e `docs/GUIA-DO-PROJETO.md` modificados e `.claude/agents/*` alterados. A T34 edita `StatsPage.tsx` e a T36 edita `GUIA-DO-PROJETO.md`: commitar ou descartar essas mudanças antes.
11. **Tradução do `-32002`.** `userError` traduz `CodeSessionActive` para "Já há um jogo em andamento…". Para `session.update` e `session.delete` o texto deve ser "Não é possível alterar uma sessão em andamento." (SED-036), então a T23 usa `callAgentRaw` e um `sessionEditError` próprio antes de cair em `userError` (mesmo padrão do `DeleteGame` e do `MediaAdd`).

## Premissas

- Uma tarefa por commit; a de testes vem antes da de código. Ao fim de cada tarefa Go: `go build ./...`, `go vet ./...`, `CGO_ENABLED=0 go build ./...`, `go test ./...` (com `-race` em T10, T12 e T21). Ao fim de cada tarefa de frontend: `npm test` e `npx tsc --noEmit` em `frontend/`.
- Todo caso de teste começa pelo ID (`t.Run("SED-004/...")`, `it("SEL-005 ...")`). Sem rede e sem `time.Sleep`: `domain.FakeClock`, SQLite em `t.TempDir()`, `vi.useFakeTimers`, fakes de bindings no `window.go.main.App`.
- **Dependências externas novas: nenhuma** (Go e npm). `sqlc generate` regenera `internal/store/dbgen/` e o resultado entra no commit da tarefa de código.
- Migração: só a 00011. `frontend/wailsjs/go/**` é versionado e regerado na T24 (e conferido pelo `wails build` da T38).
- Instantes: ms UTC no banco (ADR 0009); o agente trunca a ms o que recebe. `now` de toda regra é `domain.Clock.Now()` do agente (fim igual a `now` é aceito, SED-016).
- **Testes antigos que quebram de propósito** e que o test-writer atualiza com a anotação `alterado em 2026-11-25 (fase 16)` (listados em T03, T16, T18, T22, T27, T31, T33):
  - `SchemaVersion = 10` e `requireGooseVersion(..., 10)` em `internal/store/{media,migrate5,migrate8,migrate9,migrate10}_test.go`;
  - a lista de migrações de `media_test.go` e `migrate9_test.go`;
  - `m9OtherSchema` e `m10OtherSchema` (passam a ignorar `play_session`, que ganha coluna e índice);
  - `internal/agent/metadata_wire_test.go` (aceita também `00011_session_edit.sql`);
  - `contractPath` e `contractRelPath` em `internal/ipc/contract_test.go` e o `contractPath` da v0.16.0 em `contract_phase15_test.go`;
  - os `assertExactFields` de `SessionItem`, `GameDetail` e das estatísticas;
  - fixtures TypeScript de `GameDetailView`, `SessionItemView` e `StatsGame`/`StatsPlatform`/`StatsSummary`.

## Paralelismo e dependências

```
A: T01 -> T02
B: T03 -> T04 -> T05 -> T06 -> T07 -> T08 -> T09 -> T10 -> T11 -> T12   (T07..T12 dependem de T02 e T06)
   T13 -> T14   (depende de T04; independe de T07..T12)
C: T15 (sessão principal; independe)  ->  T16 -> T17   (T17 depende de T06 e T14 só para compilar os tipos de view)
   T18 -> T19   (depende de T06, T14, T17)
D: T20 -> T21   (depende de T12, T17, T19)
E: T22 -> T23 -> T24   (T23 depende de T17; T24 depende de T23)
F: T25 -> T26 -> T27 -> T28 -> T29 -> T30 -> T31 -> T32 -> T33 -> T34   (T28 em diante usa só mocks; podem começar após T24)
G: T35 (pode sair desde T21), T36 (depende de T32 e T34), T37
H: T38 (depende de tudo)
```

- **Em paralelo (arquivos disjuntos):** A com T03 e com T15; B (T13 e T14) com B (T07..T12) se uma tarefa de código terminar verde antes da outra começar; E e F depois de T17 e T21; T35 a qualquer momento.
- **Não paralelizar:** T04, T08, T10, T12 e T14 (todas rodam `sqlc generate` e mexem em `internal/store/queries/` e `dbgen/`); T26, T28, T30, T32 e T34 em `frontend/src/lib/library.ts`, `lib/stats.ts` e `components/` compartilhados.
- **Hooks:** `gotest.ps1` roda `go test ./...` no Stop do implementer e fica vermelho entre a tarefa de teste e a de código. Terminar cada par verde e commitado antes do próximo par que toque o mesmo pacote.

---

## T01 — Testes: regras puras de `internal/domain` [concluída]
- Requisitos: SED-002, SED-009, SED-013, SED-015, SED-016, SED-017, SED-018, SED-019, SED-020, SED-021. **Responsável:** test-writer. Dependência: nenhuma.
- Arquivo: `internal/domain/session_edit_test.go` (novo). Tudo table-driven; `now` = `T = 2026-11-25T20:00:00Z`.
- Testes:
  - `ParseSessionSource`: `SED-002/tracked, import e manual são aceitos`; `SED-002/vazio, "Manual", "MANUAL" e "edited" retornam erro`;
  - `ValidateSessionTimes`, um caso por linha do contrato §5 (a mesma tabela que a T25 usa no TypeScript): `SED-015/linha 3: início = fim é end-not-after-start (e fim antes do início)`; `SED-016/linha 4: fim em T+1min é in-future`; `SED-016/fim igual a T é aceito (linha 2)`; `SED-017/linha 5: início em 1979-12-31 é too-early`; `SED-017/1980-01-01T00:00:00Z é aceito`; `SED-018/linha 6: 25 h é span-too-long`; `SED-018/linha 10: exatamente 24 h é válido`; `SED-018/24 h + 1 ms é span-too-long`; `SED-021/linha 7: spanChecked=false aceita 25 h`; `SED-019/linha 8: 29 s é active-too-short`; `SED-019/30 s exatos é aceito`; `SED-019/linha 9: tempo 1h01 num intervalo de 1h é active-exceeds-span`; `SED-019/tempo igual ao intervalo é aceito`; `SED-019/sessão com 2 h e fim 1 h depois do início é active-exceeds-span`; `SED-015/ordem das checagens: too-early vence end-not-after-start, que vence in-future, que vence span-too-long, que vence active-too-short`; `SED-021/spanChecked=false não desliga end-not-after-start, in-future, too-early nem active-too-short`;
  - `Overlaps`: `SED-020/cruzar de fato`; `SED-020/encostar (fim = início) não cruza nos dois sentidos`; `SED-020/contida e contendo`; `SED-020/intervalos disjuntos`;
  - `ClampTotal`: `SED-013/negativo vira 0`; `SED-013/acima de 3599996400 vira o teto`; `SED-013/valor dentro da faixa passa igual`;
  - `ClipPause`: `SED-009/pausa dentro do intervalo não muda`; `SED-009/pausa que sai pela esquerda é cortada no início`; `SED-009/pausa que sai pela direita é cortada no fim`; `SED-009/pausa que cobre tudo vira o intervalo todo`; `SED-009/pausa totalmente fora devolve ok=false`; `SED-009/pausa com duração zero depois do corte devolve ok=false`; `SED-009/retomada nula conta como o fim da sessão`.

## T02 — `internal/domain`: regras puras da edição de sessões [concluída]
- Requisitos: SED-002, SED-009, SED-013, SED-015, SED-016, SED-017, SED-018, SED-019, SED-020, SED-021. **Responsável:** implementer. Dependência: T01.
- Arquivo: `internal/domain/session_edit.go` (novo; puro, sem Windows).
```go
type SessionSource string
const (SourceTracked SessionSource = "tracked"; SourceImport SessionSource = "import"; SourceManual SessionSource = "manual")
func ParseSessionSource(s string) (SessionSource, error)

const (
    MaxEditedSessionSpan   = 24 * time.Hour
    MinEditedSessionActive = MinCountedSession // 30 s, the DOM-008 limit
)
var EarliestSessionStart = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)

type EditReason string
const (
    ReasonEndNotAfterStart EditReason = "end-not-after-start"; ReasonInFuture EditReason = "in-future"
    ReasonTooEarly EditReason = "too-early"; ReasonSpanTooLong EditReason = "span-too-long"
    ReasonActiveTooShort EditReason = "active-too-short"; ReasonActiveExceedsSpan EditReason = "active-exceeds-span"
    ReasonDiscarded EditReason = "discarded"; ReasonExceedsAdjustment EditReason = "exceeds-adjustment"
    ReasonEmptyPatch EditReason = "empty-patch"
)
type SessionTimes struct{ Start, End time.Time; Active time.Duration }
func ValidateSessionTimes(t SessionTimes, now time.Time, spanChecked bool) EditReason // "" = valid
func Overlaps(aStart, aEnd, bStart, bEnd time.Time) bool
func ClampTotal(seconds, max int64) int64
func ClipPause(pausedAt time.Time, resumedAt *time.Time, start, end time.Time) (p, r time.Time, ok bool)
```
- Ordem das checagens de `ValidateSessionTimes`: too-early, end-not-after-start, in-future, span-too-long (só se `spanChecked`), active-too-short, active-exceeds-span. Constantes de `reason` em inglês (formato de fio). Sem testes novos (T01 cobre).

## T03 — Testes: migração 11 e ajuste dos testes de versão [concluída]
- Requisitos: SED-001. **Responsável:** test-writer. Dependência: nenhuma.
- Arquivos: `internal/store/migrate11_test.go` (novo); alterar `internal/store/media_test.go`, `migrate5_test.go`, `migrate8_test.go`, `migrate9_test.go`, `migrate10_test.go` e `internal/agent/metadata_wire_test.go`.
- Alterações nos antigos (anotar `alterado em 2026-11-25 (fase 16)`):
  - todo `SchemaVersion != 10` passa a `!= 11` e todo `requireGooseVersion(t, s, 10)` a `11` (`media_test.go:48` e `:276`, `migrate5_test.go:227`, `migrate8_test.go:223`, `migrate9_test.go:110`, `migrate10_test.go:84` e os `requireGooseVersion` de `:88` e `:104`); o `UpTo(10)`/`DownTo(9)` do teste de `Down` da `migrate10_test.go` não muda;
  - a lista de migrações de `migrate9_test.go:318` e de `media_test.go:271` passa a terminar em `00011_session_edit.sql`;
  - `m9OtherSchema` e `m10OtherSchema` passam a excluir `play_session` (`tbl_name NOT IN (..., 'play_session')`), pois a tabela ganha coluna e índice; o teste "nenhuma outra tabela muda" da 00010 continua conferindo `pause_interval`, `game` etc.;
  - `internal/agent/metadata_wire_test.go:216`: ignorar também `00011_session_edit.sql` (a lista de referência continua em 9).
- Testes (`migrate11_test.go`, com um helper `versionTenFile` análogo ao `versionNineFile`):
  - `SED-001/banco novo tem edited_at (INTEGER, nula) em play_session e o índice play_session_started_at_idx em (started_at)`;
  - `SED-001/SchemaVersion = 11 e a versão do goose é 11`;
  - `SED-001/banco na versão 10 com sessões rastreadas, importadas e descartadas migra com edited_at nulo e os outros valores iguais`;
  - `SED-001/os índices e as outras tabelas continuam iguais (sqlite_master sem play_session é igual ao da v10)`;
  - `SED-001/Down apaga o índice e a coluna e preserva as linhas e as outras colunas`.

## T04 — Migração 11 e `SchemaVersion` [concluída]
- Requisitos: SED-001. **Responsável:** implementer. Dependência: T03.
- Arquivos: `internal/store/migrations/00011_session_edit.sql` (novo, design §3.1 literal), `internal/store/store.go` (`SchemaVersion` 10 → 11 e o comentário).
- Sem testes novos (T03 cobre). `go test ./internal/store/... ./internal/agent/...` verde inclusive os antigos ajustados.

## T05 — Testes: leituras do store (`source`, `editedAt`, `sessionPlaySeconds`) [concluída]
- Requisitos: SED-002, SED-033, SED-034. **Responsável:** test-writer. Dependência: T04.
- Arquivos: `internal/store/sessions_list_test.go` (alterar), `internal/store/library_detail_test.go` (alterar).
- Testes:
  - `SED-033/GameSessions traz Source e EditedAt de cada item, na mesma ordem e paginação de antes`;
  - `SED-033/sessão nunca editada tem EditedAt zero`;
  - `SED-002/valor de source desconhecido no banco chega como está e a página não falha`;
  - `SED-034/GameDetail traz SessionPlaySeconds = soma de active_seconds das sessões ended`;
  - `SED-034/descartada e aberta não entram na soma`;
  - `SED-034/jogo sem sessões tem 0`;
  - `SED-034/total ajustado à mão (SetTotalPlaySeconds) não muda SessionPlaySeconds`;
  - `SED-034/UpdateGame e SetTotalPlaySeconds também devolvem SessionPlaySeconds`.

## T06 — Store: leituras novas [concluída]
- Requisitos: SED-002, SED-033, SED-034. **Responsável:** implementer. Dependência: T05.
- Arquivos: `internal/store/queries/sessions.sql` (`ListSessionsByGame` passa a devolver `source` e `edited_at`), `internal/store/queries/session_edit.sql` (novo; começa com `SumCountedSeconds :one`), `internal/store/sessions_list.go`, `internal/store/library.go` (`readGameDetail` lê a soma), `sqlc generate`.
```go
type SessionItem struct { // sessions_list.go; two fields added
    ID, GameID    int64
    State         domain.SessionState // tolerant reader (ADR 0012)
    Source        string              // tolerant reader: unknown values pass through (SED-002)
    StartedAt     time.Time
    EndedAt       time.Time // zero = NULL
    ActiveSeconds int64
    EndReason     string
    EditedAt      time.Time // zero = NULL
}
type GameDetail struct { /* existing fields */ SessionPlaySeconds int64 } // sum of active_seconds of state 'ended'
```
- Sem testes novos (T05 cobre).

## T07 — Testes: criar sessão manual no store [concluída]
- Requisitos: SED-003, SED-004, SED-005, SED-012, SED-013, SED-014, SED-015, SED-016, SED-017, SED-018, SED-019, SED-020, SED-024, SED-028. **Responsável:** test-writer. Dependência: T02, T06.
- Arquivo: `internal/store/session_edit_create_test.go` (novo; fixture com jogo, sessões rastreadas, importada, descartada e aberta inseridas por SQL direto ou por `StartSession`/`EndSession`). `now` = `W`.
- Testes (`CreateManualSession`):
  - `SED-003/grava game_id, started_at, ended_at, last_seen_at = ended_at, active_seconds, state ended, source manual, end_reason nulo e edited_at nulo, sem pause_interval`;
  - `SED-004/soma o tempo ao total, atualiza first, last e contagem e updated_at = W`;
  - `SED-004/jogo sem sessões: first = início, last = fim, play_count = 1`;
  - `SED-004/o ajuste manual se conserva: total 10 h (8 h de sessões e 2 h de ajuste) + sessão de 1 h = 11 h`;
  - `SED-005/includedInTotal mantém o total e atualiza first, last e contagem`;
  - `SED-005/X − Y igual ao tempo é aceito; X − Y menor é SessionEditError exceeds-adjustment e nada muda`;
  - `SED-005/includedInTotal sem ajuste positivo é exceeds-adjustment`;
  - `SED-012/first é o menor início e last é o maior fim das contadas; descartada e aberta não entram`;
  - `SED-013/total no teto 3599996400 não passa do teto`;
  - `SED-014/falha ao atualizar o jogo (trigger BEFORE UPDATE ON game com RAISE(ABORT)) desfaz a sessão inserida`;
  - `SED-015/fim igual ao início e fim antes do início são end-not-after-start`;
  - `SED-016/fim depois de now é in-future; fim igual a now é aceito`;
  - `SED-017/início antes de 1980 é too-early`;
  - `SED-018/intervalo de 25 h é span-too-long; 24 h exatas é aceito`;
  - `SED-019/29 s é active-too-short; tempo maior que o intervalo é active-exceeds-span`;
  - `SED-015/cada recusa devolve *SessionEditError com o Reason e não grava nada`;
  - `SED-020/contrato §5: encostar na rastreada é aceito e cruzar é SessionOverlapError com o id, o jogo, o título e os instantes da outra`;
  - `SED-020/cruza uma manual de outro jogo`;
  - `SED-020/cruza a sessão aberta [started_at, now): EndedAt nulo no erro; encostar no início da aberta é aceito`;
  - `SED-020/sessões importadas e descartadas não entram na conta`;
  - `SED-024/jogo inexistente devolve erro que satisfaz ErrGameNotFound`;
  - `SED-028/status e completed_at do jogo não mudam (inclusive not_played)`.

## T08 — Store: criar sessão manual [concluída]
- Requisitos: SED-003, SED-004, SED-005, SED-012, SED-013, SED-014, SED-015, SED-016, SED-017, SED-018, SED-019, SED-020, SED-024, SED-028. **Responsável:** implementer. Dependência: T07.
- Arquivos: `internal/store/session_edit.go` (novo), `internal/store/queries/session_edit.sql` (acrescenta `InsertManualSession`, `FindOverlappingSession`, `RecomputeGamePlayFacts`, `AddToTotal`), `sqlc generate`.
```go
var (
    ErrSessionNotFound = errors.New("store: session not found")
    ErrSessionOpen     = errors.New("store: session is open")
)
type SessionOpenError struct{ SessionID, GameID int64; Title string } // errors.Is(err, ErrSessionOpen)
func (e *SessionOpenError) Error() string
func (e *SessionOpenError) Is(target error) bool
type SessionEditError struct{ Reason domain.EditReason }
func (e *SessionEditError) Error() string
type SessionOverlapError struct {
    SessionID, GameID int64
    Title             string
    StartedAt         time.Time
    EndedAt           *time.Time // nil = the other session is open
}
func (e *SessionOverlapError) Error() string
type ManualSession struct {
    GameID          int64
    Start, End      time.Time
    Active          time.Duration
    IncludedInTotal bool
}
type SessionPatch struct{ Start, End *time.Time; Active *time.Duration }
type SessionWrite struct {
    Session                 SessionItem  // after the write (zero on delete)
    Game                    GameDetail
    TotalBefore, TotalAfter int64
    Previous                *SessionItem // nil on create
    Changed                 bool         // false: nothing was written (SED-008)
}
func (s *Store) CreateManualSession(ctx context.Context, m ManualSession, now time.Time) (SessionWrite, error)
```
- `CreateManualSession` segue a ordem do design §3.4 dentro de `Store.Tx`; as consultas do jogo (existência, total, `SumCountedSeconds`) e a checagem de sobreposição ficam na mesma transação. Datas truncadas a ms. `AddToTotal` usa `MIN(MAX(total + delta, 0), ipc.MaxPlaySeconds)`: o store não importa `ipc`, então o teto vem como constante local `maxTotalPlaySeconds = 999_999 * 3600` com comentário apontando para `ipc.MaxPlaySeconds`; um teste da T16 confere que os dois valores são iguais.
- `FindOverlappingSession` ignora `id = :self`, só considera `state = 'ended' AND source IN ('tracked','manual')` ou `ended_at IS NULL`, e devolve o título do jogo da outra sessão. Sem testes novos (T07 cobre).

## T09 — Testes: editar sessão no store [concluída]
- Requisitos: SED-006, SED-007, SED-008, SED-009, SED-012, SED-013, SED-014, SED-015, SED-016, SED-017, SED-018, SED-019, SED-020, SED-021, SED-022, SED-023, SED-024, SED-025, SED-028. **Responsável:** test-writer. Dependência: T08.
- Arquivo: `internal/store/session_edit_update_test.go` (novo). Fixtures com pausas em `pause_interval` e jogo com ajuste manual.
- Testes (`EditSession`):
  - `SED-006/só início, só fim, só tempo jogado e os três: apenas os campos presentes mudam; last_seen_at acompanha o fim`;
  - `SED-006/source, state e end_reason ficam como estavam, para as origens tracked, import e manual; edited_at = W`;
  - `SED-007/total passa a X + (a' − a); contagem, first e last recalculados`;
  - `SED-007/exemplo: total 10 h (8 h de sessões e 2 h de ajuste), sessão de 1 h editada para 1 h 30 = 10 h 30`;
  - `SED-007/updated_at do jogo só muda se algum campo do jogo mudou`;
  - `SED-008/pedido igual ao gravado não grava (edited_at e updated_at do jogo intactos), Changed = false e devolve a sessão e o jogo como estão`;
  - `SED-008/igualdade é comparada em ms`;
  - `SED-009/pausa que sai do novo intervalo é cortada nas bordas`;
  - `SED-009/pausa com duração zero ou toda fora é apagada`;
  - `SED-009/mudar só o tempo jogado não mexe nas pausas`;
  - `SED-009/active_seconds fica o valor gravado, não é recalculado das pausas`;
  - `SED-012/editar a primeira ou a última sessão recalcula first_played_at e last_played_at`;
  - `SED-013/delta negativo maior que o total deixa 0`;
  - `SED-013/delta que passa do teto fica no teto`;
  - `SED-014/falha ao gravar o jogo (trigger) desfaz a sessão e o corte das pausas`;
  - `SED-015/fim resultante não posterior ao início resultante (patch só do fim) é end-not-after-start`;
  - `SED-016/fim resultante depois de now é in-future`;
  - `SED-017/início resultante antes de 1980 é too-early`;
  - `SED-018/patch que muda início ou fim e dá mais de 24 h é span-too-long em sessão tracked ou manual`;
  - `SED-018/patch só do tempo jogado de uma rastreada com intervalo de 30 h (gravada antes da regra) é aceito`;
  - `SED-019/tempo menor que 30 s é active-too-short; tempo maior que o intervalo resultante é active-exceeds-span`;
  - `SED-019/sessão com 2 h jogadas e patch que só muda o fim para 1 h depois do início é active-exceeds-span`;
  - `SED-020/a própria sessão não conta como conflito`;
  - `SED-020/cruzar rastreada, manual de outro jogo ou a aberta é SessionOverlapError; encostar é aceito`;
  - `SED-020/patch só do tempo jogado não faz a checagem de sobreposição`;
  - `SED-021/importada com intervalo de 300 h e deslocada sobre uma rastreada é aceita`;
  - `SED-021/importada continua sujeita a end-not-after-start, in-future, too-early e active-too-short`;
  - `SED-022/sessão running, paused e grace devolve *SessionOpenError (errors.Is ErrSessionOpen) com id, jogo e título, e nada muda`;
  - `SED-023/descartada devolve SessionEditError discarded e nada muda`;
  - `SED-024/id inexistente devolve ErrSessionNotFound`;
  - `SED-025/com o jogo rodando: editar a sessão anterior de 1 h para 2 h sem cruzar a aberta e depois EndSession da aberta com 30 min ativos soma 1 h 30 ao total; a aberta segue sem mudança de estado, tempo ativo nem pausas; last_played_at é o fim da sessão encerrada`;
  - `SED-028/status e completed_at não mudam`.

## T10 — Store: editar sessão [concluída]
- Requisitos: SED-006, SED-007, SED-008, SED-009, SED-012, SED-013, SED-014, SED-015, SED-016, SED-017, SED-018, SED-019, SED-020, SED-021, SED-022, SED-023, SED-024, SED-025, SED-028. **Responsável:** implementer. Dependência: T09.
- Arquivos: `internal/store/session_edit.go` (acrescenta), `internal/store/queries/session_edit.sql` (acrescenta `GetSessionForEdit`, `UpdateSessionTimes`, `ListPausesOfSession`, `UpdatePauseBounds`, `DeletePause`), `sqlc generate`.
```go
func (s *Store) EditSession(ctx context.Context, id int64, p SessionPatch, now time.Time) (SessionWrite, error)
```
- Aplica o patch sobre os valores gravados; `spanChecked = source != import && (patch traz Start ou End)`; sobreposição só se a origem é `tracked`/`manual` e o patch traz início ou fim. `UpdateSessionTimes` roda com `WHERE id = ? AND ended_at IS NOT NULL` e confere `execrows`. Rodar a suíte com `go test -race ./internal/store/...`. Sem testes novos (T09 cobre).

## T11 — Testes: excluir sessão no store [concluída]
- Requisitos: SED-010, SED-011, SED-012, SED-013, SED-014, SED-022, SED-024, SED-028. **Responsável:** test-writer. Dependência: T10.
- Arquivo: `internal/store/session_edit_delete_test.go` (novo).
- Testes (`RemoveSession`):
  - `SED-010/apaga a sessão e as pausas e subtrai o tempo: total X − a, updated_at = W`;
  - `SED-010/sessão da qual restam outras: first, last e contagem recalculados; sem sessões contadas: first e last nulos e play_count = 0`;
  - `SED-010/exemplo do ajuste manual: total ajustado para 1 h e exclusão de uma sessão de 3 h deixa 0 (SED-013)`;
  - `SED-011/descartada: a linha e as pausas saem e a linha do jogo não muda (nem updated_at)`;
  - `SED-012/descartada e aberta não entram no recálculo`;
  - `SED-014/falha ao atualizar o jogo (trigger) desfaz a exclusão e as pausas`;
  - `SED-022/aberta devolve *SessionOpenError e nada muda`;
  - `SED-024/id inexistente devolve ErrSessionNotFound`;
  - `SED-028/status e completed_at não mudam`.

## T12 — Store: excluir sessão [concluída]
- Requisitos: SED-010, SED-011, SED-012, SED-013, SED-014, SED-022, SED-024, SED-028. **Responsável:** implementer. Dependência: T11.
- Arquivos: `internal/store/session_edit.go` (acrescenta), `internal/store/queries/session_edit.sql` (`DeleteClosedSession :execrows`), `sqlc generate`.
```go
func (s *Store) RemoveSession(ctx context.Context, id int64, now time.Time) (SessionWrite, error)
```
- As pausas saem por `ON DELETE CASCADE` (conferir `PRAGMA foreign_keys` ligado no `Open`, como o `DeleteGame` já depende). Descartada: delta 0 e jogo intocado. Rodar `go test -race ./internal/store/...`. Sem testes novos (T11 cobre).

## T13 — Testes: estatísticas (manualSeconds e só contadas) [concluída]
- Requisitos: SED-027, SED-037. **Responsável:** test-writer. Dependência: T04.
- Arquivo: `internal/store/stats_test.go` (alterar).
- Testes:
  - `SED-027/StatsByGame traz ManualSeconds = parte da origem manual; a rastreada editada conta como rastreada`;
  - `SED-027/StatsByPlatform traz ManualSeconds`;
  - `SED-027/as três origens entram no tempo por jogo, por plataforma e em SessionTimes`;
  - `SED-037/período com uma contada de 1 h, uma descartada de 20 s e uma aberta com 10 min gravados: StatsByGame soma 1 h e Sessions = 1`;
  - `SED-037/StatsByPlatform só soma as contadas (plataforma só com descartada não aparece)`;
  - `SED-037/SessionTimes só devolve as contadas`;
  - `SED-037/os testes antigos de EST-001..EST-003 continuam verdes` (os fixtures que usavam sessão aberta ou descartada ganham a anotação `alterado em 2026-11-25 (fase 16)`).

## T14 — Store: estatísticas [concluída]
- Requisitos: SED-027, SED-037. **Responsável:** implementer. Dependência: T13.
- Arquivos: `internal/store/queries/stats.sql` (`StatsByGame` e `StatsByPlatform` ganham `manual_seconds` e `AND s.state = 'ended'`; `SessionTimesSince` ganha `AND state = 'ended'`; atualizar o comentário do cabeçalho), `internal/store/stats.go`, `sqlc generate`.
```go
type GameStat struct { /* existing */ ManualSeconds int64 }
type PlatformStat struct { /* existing */ ManualSeconds int64 }
```
- Sem testes novos (T13 cobre).

## T15 — Contrato completo `ipc.openrpc.json` v0.17.0 [concluída]
- Requisitos: SED-030, SED-033, SED-034, SED-027. **Responsável:** sessão principal (cópia mecânica de JSON grande, por script, como nas fases 11 a 15). Dependência: nenhuma. Bloqueia a T16 verde.
- Criar `docs/specs/phase-16-edicao-sessoes/contracts/ipc.openrpc.json` como cópia de `docs/specs/phase-15-big-box-v2/contracts/ipc.openrpc.json` (v0.16.0) mais o [delta](contracts/ipc-v0.17.0-delta.openrpc.json):
  - `info.version = "0.17.0"` e `info.description` atualizada;
  - os métodos `session.create`, `session.update` e `session.delete` logo depois de `session.list`;
  - os esquemas novos `SessionSource`, `SessionEditReason`, `SessionWriteResult`, `SessionDeleteResult`, `SessionOverlapData` (os três `*Additions` do delta **não** vão para o arquivo: são mesclados);
  - `SessionItem` ganha `source` (string, descrição de `SessionSource`) e `editedAt` (`oneOf` data-hora ou null), ambos obrigatórios;
  - `GameDetail` ganha `sessionPlaySeconds` (integer ≥ 0, obrigatório);
  - `StatsGame`, `StatsPlatform` e `StatsSummaryResult` ganham `manualSeconds` (obrigatório);
  - erros novos `SessionNotFound` (-32017) e `SessionOverlap` (-32018); as descrições de `InvalidParams` e `SessionActive` recebem o texto do delta.
- Em `contracts/ipc-v0.17.0-delta.md` e `design.md` §6, trocar "(gerado por uma tarefa)" pelo arquivo criado.

## T16 — Testes: contrato IPC v0.17.0 [concluída]
- Requisitos: SED-030, SED-033, SED-034, SED-027, SED-013. **Responsável:** test-writer. Dependência: nenhuma (fica vermelho até T15 e T17).
- Arquivos: `internal/ipc/contract_phase16_test.go` (novo); alterar `internal/ipc/contract_test.go` (`contractPath` e `contractRelPath` passam para `phase-16-edicao-sessoes`; o `assertExactFields` de `GameDetail`, `SessionItem` e dos tipos de estatística passa a incluir os campos novos), `contract_phase15_test.go` (a v0.16.0 passa a ler `contractV016Path` = `docs/specs/phase-15-big-box-v2/contracts/ipc.openrpc.json`; o teste `TTB-029/a v0.16.0 é a v0.15.0 mais acréscimos` compara `contractV015Path` com `contractV016Path` e o "versão 0.16.0" também), `contract_phase13_test.go` e `contract_phase12_test.go` (onde o teste é "≥", conferir; onde compara `contractPath` com versão exata, trocar). Anotar `alterado em 2026-11-25 (fase 16)`.
- Testes:
  - `SED-030/versão 0.17.0 e o comentário do pacote aponta para phase-16-edicao-sessoes/contracts/ipc.openrpc.json`;
  - `SED-030/os três métodos session.* existem com paramStructure by-name e os nomes das constantes Go`;
  - `SED-030/params de session.create, session.update e session.delete batem com os tipos Go (assertExactFields) e os obrigatórios são os do delta`;
  - `SED-030/session.update: startedAt, endedAt e activeSeconds opcionais`;
  - `SED-030/os resultados SessionWriteResult e SessionDeleteResult batem com os tipos Go`;
  - `SED-030/o enum source é tracked, import, manual e bate com as constantes de domain.SessionSource`;
  - `SED-030/o enum reason de SessionEditReason bate com as constantes de domain.EditReason, na mesma ordem`;
  - `SED-030/os erros -32017 e -32018 batem com ipc.CodeSessionNotFound, ipc.CodeSessionOverlap e as mensagens; SessionOverlapData bate com o tipo Go`;
  - `SED-030/session.update e session.delete citam SessionActive (-32002) nos erros`;
  - `SED-033/SessionItem tem source e editedAt obrigatórios e os tipos Go batem`;
  - `SED-034/GameDetail tem sessionPlaySeconds obrigatório`;
  - `SED-027/StatsGame, StatsPlatform e StatsSummaryResult têm manualSeconds obrigatório`;
  - `SED-030/a v0.17.0 é a v0.16.0 mais acréscimos`: normaliza `info.version` e `info.description`, tira os métodos, esquemas e erros novos, os campos acrescentados a `SessionItem`, `GameDetail` e às estatísticas e o texto acrescentado a `InvalidParams` e `SessionActive`, e então `reflect.DeepEqual` com a v0.16.0; confere que nenhum método, parâmetro, resultado, erro ou esquema antigo mudou ou sumiu e que a ordem relativa dos antigos se mantém;
  - `SED-013/o teto do total do store (maxTotalPlaySeconds) é igual a ipc.MaxPlaySeconds` (arquivo `internal/store/session_edit_limits_test.go`, novo; usa uma função exportada de teste `store.MaxTotalPlaySecondsForTest` em `export_test.go`).

## T17 — `internal/ipc`: tipos e constantes v0.17.0 [concluída]
- Requisitos: SED-030, SED-031, SED-032, SED-033, SED-034, SED-027. **Responsável:** implementer. Dependência: T15, T16.
- Arquivos: `internal/ipc/session_edit.go` (novo), `internal/ipc/library.go` (campos novos), `internal/ipc/stats.go` (campos novos), `internal/ipc/protocol.go` (comentário do pacote aponta para o contrato da fase 16).
```go
const (
    MethodSessionCreate = "session.create"
    MethodSessionUpdate = "session.update"
    MethodSessionDelete = "session.delete"
)
const (CodeSessionNotFound = -32017; CodeSessionOverlap = -32018)
const (MsgSessionNotFound = "Session not found"; MsgSessionOverlap = "Session overlaps another")

// Client and server side: plain types, RFC 3339 instants (a malformed one fails decoding -> -32602).
type SessionCreateParams struct {
    GameID          int64     `json:"gameId"`
    StartedAt       time.Time `json:"startedAt"`
    EndedAt         time.Time `json:"endedAt"`
    ActiveSeconds   int64     `json:"activeSeconds"`
    IncludedInTotal bool      `json:"includedInTotal,omitempty"`
}
// Server side only: Optional tells absent from null (null is -32602).
type SessionUpdateParams struct {
    SessionID     int64                 `json:"sessionId"`
    StartedAt     Optional[time.Time]   `json:"startedAt"`
    EndedAt       Optional[time.Time]   `json:"endedAt"`
    ActiveSeconds Optional[int64]       `json:"activeSeconds"`
}
type SessionDeleteParams struct{ SessionID int64 `json:"sessionId"` }
type SessionWriteResult struct{ Session SessionItem `json:"session"`; Game GameDetail `json:"game"` }
type SessionDeleteResult struct{ SessionID int64 `json:"sessionId"`; Game GameDetail `json:"game"` }
type SessionOverlapData struct {
    SessionID int64      `json:"sessionId"`
    GameID    int64      `json:"gameId"`
    Title     string     `json:"title"`
    StartedAt time.Time  `json:"startedAt"`
    EndedAt   *time.Time `json:"endedAt"`
}
type InvalidParamsReasonData struct{ Reason string `json:"reason"` }

// library.go
type SessionItem struct { /* existing */ Source string `json:"source"`; EditedAt *time.Time `json:"editedAt"` }
type GameDetail struct { /* existing */ SessionPlaySeconds int64 `json:"sessionPlaySeconds"` }
// stats.go: ManualSeconds int64 `json:"manualSeconds"` in StatsGame, StatsPlatform and StatsSummaryResult
```
- `ipc` não importa `domain` nem `store` (os testes da T16 conferem o enum contra `domain`). Sem testes novos (T16 cobre).

## T18 — Testes: campos novos nas leituras do agente [concluída]
- Requisitos: SED-027, SED-033, SED-034. **Responsável:** test-writer. Dependência: T06, T14, T17.
- Arquivos: `internal/agent/library_test.go` (alterar), `internal/agent/stats_test.go` (alterar), `internal/agent/games_test.go` (se compara JSON de `session.list`).
- Testes:
  - `SED-033/session.list devolve source e editedAt (RFC 3339 UTC, ou null) em cada item; ordem e paginação não mudam`;
  - `SED-033/valor de source desconhecido passa como está`;
  - `SED-034/game.get, game.update e game.setPlayTime devolvem sessionPlaySeconds`;
  - `SED-027/stats.summary devolve manualSeconds por jogo e por plataforma e o total geral é a soma dos jogos`;
  - `SED-027/importedSeconds e manualSeconds convivem sem se misturar`;
  - `SED-027/stats.weekly não ganha campo`.

## T19 — Agente: conversões das leituras [concluída]
- Requisitos: SED-027, SED-033, SED-034. **Responsável:** implementer. Dependência: T18.
- Arquivos: `internal/agent/library.go` (`sessionItemToIPC` e `gameDetailToIPC`), `internal/agent/stats.go` (`summaryResult` soma `ManualSeconds` e o expõe).
- Sem testes novos (T18 cobre).

## T20 — Testes: handlers `session.*` do agente [concluída]
- Requisitos: SED-005, SED-008, SED-013, SED-015, SED-016, SED-017, SED-018, SED-019, SED-020, SED-022, SED-023, SED-024, SED-025, SED-026, SED-029, SED-031, SED-032. **Responsável:** test-writer. Dependência: T12, T17, T19.
- Arquivo: `internal/agent/session_edit_test.go` (novo), no estilo de `manual_edit_test.go` (fixture com store real em `t.TempDir()`, `domain.FakeClock`, `slog` capturado em buffer; um segundo teste de integração com servidor IPC real e dois `ipc.Client`, no estilo de `media_changed_test.go`).
- Testes:
  - `SED-031/session.create com params por nome devolve {session, game} já com a sessão nova e os valores do jogo depois da escrita`;
  - `SED-031/includedInTotal ausente vale false`;
  - `SED-031/membro desconhecido, tipo errado, instante que não é RFC 3339, startedAt ou endedAt ausente, gameId < 1 e params posicionais são -32602 sem data.reason`;
  - `SED-031/gameId inexistente é -32001`;
  - `SED-031/instantes com mais precisão que ms são truncados a ms (ADR 0009)`;
  - `SED-019/activeSeconds 29 em session.create é -32602 com data.reason active-too-short (não rejeitado por faixa antes da regra)`;
  - `SED-015/cada reason de domínio chega em error.data.reason: end-not-after-start, in-future, too-early, span-too-long, active-too-short, active-exceeds-span`;
  - `SED-016/in-future usa o relógio do agente (FakeClock), e fim igual a now é aceito`;
  - `SED-017/too-early em session.update`;
  - `SED-018/span-too-long em session.update com início ou fim; e só tempo jogado numa rastreada de 30 h é aceito`;
  - `SED-005/includedInTotal acima do ajuste é -32602 com reason exceeds-adjustment e nada muda`;
  - `SED-032/session.update: ausente não mexe, null é -32602, patch sem nenhum dos três é -32602 com reason empty-patch`;
  - `SED-032/session.update devolve {session, game} depois da escrita; session.delete devolve {sessionId, game}`;
  - `SED-008/pedido sem mudança responde sucesso com a sessão e o jogo como estão e não registra Info`;
  - `SED-013/exclusão que levaria o total abaixo de 0 devolve total 0`;
  - `SED-020/-32018 com data = SessionOverlapData (sessionId, gameId, title, startedAt, endedAt ou null para a aberta) e nada muda`;
  - `SED-022/sessão running, paused e grace em update e delete é -32002 com data SessionRef (título do jogo); nada muda; o handler não recebe tracker`;
  - `SED-023/descartada em update é -32602 com reason discarded; delete da descartada funciona`;
  - `SED-024/sessionId inexistente em update e delete é -32017 com data {sessionId}`;
  - `SED-025/com o jogo rodando, editar outra sessão do mesmo jogo funciona e a sessão aberta segue igual (estado, tempo ativo e pausas)`;
  - `SED-026/com dois clientes conectados, session.create, session.update e session.delete não enviam session.changed nem outra notificação a nenhum deles`;
  - `SED-029/criação, edição e exclusão gravam Info "sessão criada", "sessão editada" e "sessão excluída" com session_id, game_id, source, valores antigos e novos de início, fim e tempo jogado (os que existem) e o total do jogo antes e depois`;
  - `SED-029/pedido recusado não registra nada acima de Debug`;
  - `SED-029/falha interna do store é -32603 com Error no log e a mensagem genérica`.

## T21 — Agente: handlers `session.create`, `session.update`, `session.delete` [concluída]
- Requisitos: SED-005, SED-008, SED-013, SED-015, SED-016, SED-017, SED-018, SED-019, SED-020, SED-022, SED-023, SED-024, SED-025, SED-026, SED-029, SED-031, SED-032. **Responsável:** implementer. Dependência: T20.
- Arquivos: `internal/agent/session_edit.go` (novo), `internal/agent/app.go` (uma linha: `maps.Copy(handlers, sessionEditHandlers(sessionEdit{st: st, clock: clock, log: log}))`, depois de `manualEditHandlers`).
```go
type sessionEditStore interface {
    CreateManualSession(ctx context.Context, m store.ManualSession, now time.Time) (store.SessionWrite, error)
    EditSession(ctx context.Context, id int64, p store.SessionPatch, now time.Time) (store.SessionWrite, error)
    RemoveSession(ctx context.Context, id int64, now time.Time) (store.SessionWrite, error)
}
type sessionEdit struct {
    st    sessionEditStore
    clock domain.Clock
    log   *slog.Logger
}
func sessionEditHandlers(e sessionEdit) map[string]ipc.Handler // session.create, session.update, session.delete
```
- Decodifica com `decodeOrInvalid`/`ipc.DecodeParams` (IPC-028); `Optional.Null` em `update` é `-32602` sem `data`; patch vazio é `-32602` com `data.reason = empty-patch`. Traduz erros: `ErrGameNotFound` → `-32001` (`GameNotFoundData`); `ErrSessionNotFound` → `-32017` (`{sessionId}`); `*SessionOpenError` → `-32002` com `SessionRef`; `*SessionEditError` → `-32602` com `InvalidParamsReasonData`; `*SessionOverlapError` → `-32018` com `SessionOverlapData`; resto → `-32603` com `Error` no log. Log `Info` depois do commit e só se `SessionWrite.Changed`; recusa em `Debug`. Resposta via `gameDetailToIPC` e `sessionItemToIPC` (T19). **Sem notificação, sem tracker, sem goroutine.** Rodar `go test -race ./internal/agent/...`. Sem testes novos (T20 cobre).

## T22 — Testes: bindings e mensagens do desktop [concluída]
- Requisitos: SED-027, SED-035, SED-036. **Responsável:** test-writer. Dependência: T17.
- Arquivos: `cmd/arcadia/session_edit_test.go` (novo, agente falso como `manual_edit` e `library_test.go`), `cmd/arcadia/errors_test.go` (alterar), `cmd/arcadia/library_test.go` e `cmd/arcadia/stats_test.go` (alterar para os campos novos).
- Testes:
  - `SED-035/CreateSession chama session.create com gameId, startedAt, endedAt, activeSeconds e includedInTotal e prazo de 5 s, e devolve a sessão e o detalhe convertidos`;
  - `SED-035/UpdateSession repassa o patch bruto: ausente não é enviado, null é enviado como null`;
  - `SED-035/DeleteSession chama session.delete e devolve o detalhe`;
  - `SED-035/a view traz source, editedAt ("" quando nulo) e sessionPlaySeconds, sem calcular tempo`;
  - `SED-035/GetGame, GameSessions e UpdateGame também preenchem os campos novos`;
  - `SED-035/nenhum dos três grava nada nem emite evento Wails por conta própria (DSK-017)`;
  - `SED-027/StatsSummary repassa manualSeconds por jogo, por plataforma e no total`;
  - `SED-036/-32017 vira "Esta sessão não existe mais."`;
  - `SED-036/-32018 vira "O horário cruza outra sessão de «título», de «início» a «fim»." com data e hora locais (fuso fixado no teste)`;
  - `SED-036/-32018 com endedAt nulo usa "agora" no fim`;
  - `SED-036/-32002 em session.update e session.delete vira "Não é possível alterar uma sessão em andamento."; o -32002 de game.launch continua com o texto de antes`;
  - `SED-036/-32602 com reason: um caso por linha da tabela do contrato §4 (end-not-after-start, in-future, too-early, span-too-long, active-too-short, active-exceeds-span, discarded, exceeds-adjustment, empty-patch)`;
  - `SED-036/-32602 sem reason e -32001 mantêm os textos de hoje`;
  - `SED-036/desconexão e prazo viram "O agente não está respondendo. Tente de novo."`.

## T23 — Desktop: bindings, views e mensagens [concluída]
- Requisitos: SED-027, SED-035, SED-036. **Responsável:** implementer. Dependência: T22.
- Arquivos: `cmd/arcadia/session_edit.go` (novo), `cmd/arcadia/library.go` (`SessionItemView` ganha `Source string` e `EditedAt string`; `GameDetailView` ganha `SessionPlaySeconds int64`; `sessionItemView` e `gameDetailView`), `cmd/arcadia/stats.go` (`ManualSeconds` nas três views), `cmd/arcadia/errors.go` (constantes de texto do SED-036 e `sessionEditError`).
```go
type SessionCreateInput struct {
    StartedAt       string `json:"startedAt"` // RFC 3339
    EndedAt         string `json:"endedAt"`
    ActiveSeconds   int64  `json:"activeSeconds"`
    IncludedInTotal bool   `json:"includedInTotal"`
}
type SessionWriteView struct {
    Session SessionItemView `json:"session"`
    Game    GameDetailView  `json:"game"`
}
func (a *App) CreateSession(gameID int64, in SessionCreateInput) (SessionWriteView, error) // 5 s
func (a *App) UpdateSession(sessionID int64, patch json.RawMessage) (SessionWriteView, error) // 5 s, patch como UpdateGame
func (a *App) DeleteSession(sessionID int64) (GameDetailView, error) // 5 s
// sessionEditError turns the errors of the three methods into the SED-036 texts; anything else goes to userError.
func sessionEditError(err error, log *slog.Logger, loc *time.Location) error
```
- `CreateSession` converte as strings RFC 3339 para `time.Time` antes de montar `ipc.SessionCreateParams` (string inválida vira o texto "Dados inválidos."). `UpdateSession` monta os params por mapa (`sessionId` + só os membros presentes do patch, `null` preservado), como `patchParams` faz para o jogo. A mensagem do `-32018` formata `DD/MM/AAAA HH:MM` na `time.Local` do processo; usar uma variável de pacote `displayLocation = time.Local` que o teste troca. Sem testes novos (T22 cobre).

## T24 — Bindings do Wails [concluída]
- Requisitos: SED-035. **Responsável:** sessão principal (mecânico). Dependência: T23.
- Rodar `wails generate module` em `cmd/arcadia` (ou `wails build`) e commitar `frontend/wailsjs/go/main/App.d.ts`, `App.js` e `frontend/wailsjs/go/models.ts`: `CreateSession`, `UpdateSession`, `DeleteSession`, `main.SessionCreateInput`, `main.SessionWriteView`, e os campos novos de `SessionItemView`, `GameDetailView` e das views de estatística. Sem testes (a T38 reconfere com o `wails build` completo).

## T25 — Testes: `lib/sessionForm.ts` (lógica pura) [concluída]
- Requisitos: SED-019, SEL-005, SEL-006, SEL-007, SEL-008, SEL-009, SEL-016. **Responsável:** test-writer. Dependência: T24.
- Arquivo: `frontend/src/lib/sessionForm.test.ts` (novo). O arquivo fixa `process.env.TZ = 'America/Sao_Paulo'` em `beforeAll` (Dúvida 9) e usa `vi.setSystemTime`. A mesma tabela de exemplos do contrato §5 (10 linhas) é replicada aqui.
- Testes:
  - `SEL-005/criação: Fim = agora arredondado para baixo ao minuto, Início = Fim − 1 h e Tempo jogado = 1 h 00`;
  - `SEL-005/edição: os campos vêm da sessão; o tempo jogado com segundos é mostrado arredondado para baixo ao minuto`;
  - `SEL-006/com Tempo jogado igual a Fim − Início, mudar Início ou Fim atualiza o Tempo jogado`;
  - `SEL-006/depois que o usuário muda o Tempo jogado para outro valor, mudar Início ou Fim não o altera mais`;
  - `SEL-006/voltar o Tempo jogado ao valor do intervalo religa o acompanhamento` (comportamento a fixar: religa só se for exatamente igual no momento da edição do Tempo; documentar no teste);
  - `SEL-007/validateSessionForm: um caso por linha da tabela do contrato §5 devolve o reason esperado (válido, end-not-after-start, in-future, too-early, span-too-long, active-too-short, active-exceeds-span)`;
  - `SEL-007/sessão importada não tem limite de 24 h (linha 7)`;
  - `SEL-007/Tempo jogado menor que 1 min é active-too-short; maior que o intervalo é active-exceeds-span`;
  - `SEL-007/minutos fora de 0..59, campo vazio e texto não numérico viram erro do campo com a mensagem proposta da Dúvida 1`;
  - `SEL-007/as mensagens dos reasons são as de SED-036 (tabela do contrato §4)`;
  - `SEL-007/o formulário só aceita Tempo jogado ≥ 1 min enquanto o agente aceita ≥ 30 s`;
  - `SEL-009/com a caixa marcada, Tempo jogado acima do ajuste (totalPlaySeconds − sessionPlaySeconds) é exceeds-adjustment; igual ao ajuste é válido`;
  - `SEL-009/a caixa só é oferecida na criação e com ajuste positivo (showIncludedInTotal)`;
  - `SEL-009/o texto de ajuda traz o ajuste formatado (X a mais que as sessões)`;
  - `SEL-008/toCreateInput envia os quatro campos com os instantes em UTC`;
  - `SEL-008/toUpdatePatch envia só os campos alterados; Tempo jogado intocado não é enviado e os segundos da rastreada ficam intactos`;
  - `SEL-008/patch sem nenhuma alteração é vazio (Salvar não envia)`;
  - `SEL-016/hora local que existe duas vezes (fim do horário de verão) converte para o instante que o Date do navegador escolhe e o valor devolvido ao campo é o convertido`;
  - `SEL-016/hora local que não existe (início do horário de verão) é ajustada pelo Date e o campo mostra o valor ajustado`;
  - `SEL-016/ida e volta localToUtcIso e utcIsoToLocal para um instante comum`.

## T26 — Frontend: `lib/sessionForm.ts` [concluída]
- Requisitos: SED-019, SEL-005, SEL-006, SEL-007, SEL-008, SEL-009, SEL-016. **Responsável:** implementer. Dependência: T25.
- Arquivo: `frontend/src/lib/sessionForm.ts` (novo; puro, sem React nem bindings; o "agora" é parâmetro).
```ts
export type EditReason =
  | 'end-not-after-start' | 'in-future' | 'too-early' | 'span-too-long' | 'active-too-short'
  | 'active-exceeds-span' | 'discarded' | 'exceeds-adjustment' | 'empty-patch'
export type SessionFormValues = { start: string; end: string; hours: string; minutes: string; includedInTotal: boolean } // start/end: 'YYYY-MM-DDTHH:mm' local
export type SessionFormState = { values: SessionFormValues; initial: SessionFormValues; activeFollowsSpan: boolean }
export type SessionFormField = 'start' | 'end' | 'active'
export type SessionFormErrors = Partial<Record<SessionFormField, string>>
export const sessionReasonMessages: Record<EditReason, string> // SED-036 texts
export function createFormState(now: Date): SessionFormState
export function editFormState(s: SessionItemView): SessionFormState
export function changeField(st: SessionFormState, field: keyof SessionFormValues, value: string | boolean): SessionFormState
export function validateSessionForm(
  v: SessionFormValues,
  ctx: { now: Date; source: string; adjustmentSeconds: number | null } // adjustmentSeconds: only for the create form with the box ticked
): SessionFormErrors
export function showIncludedInTotal(mode: 'create' | 'edit', game: { totalPlaySeconds: number; sessionPlaySeconds: number }): boolean
export function manualAdjustmentSeconds(game: { totalPlaySeconds: number; sessionPlaySeconds: number }): number
export function toCreateInput(v: SessionFormValues): { startedAt: string; endedAt: string; activeSeconds: number; includedInTotal: boolean }
export function toUpdatePatch(initial: SessionFormValues, v: SessionFormValues, originalActiveSeconds: number): Record<string, unknown>
export function localToUtcIso(local: string): string | null
export function utcIsoToLocal(iso: string): string
```
- Usa `Date` do navegador para a conversão local ↔ UTC; usa `playTimeParts`/`playTimeSeconds` de `lib/manualEdit.ts` onde servir. Sem testes novos (T25 cobre).

## T27 — Testes: `lib/sessionEdit.ts` (chamadas e hooks) e tipos [concluída]
- Requisitos: SEL-013. **Responsável:** test-writer. Dependência: T26.
- Arquivos: `frontend/src/lib/sessionEdit.test.tsx` (novo); alterar `frontend/src/lib/library.test.tsx`, `frontend/src/lib/games.test.tsx` e as fixtures de `GameDetailView` e `SessionItemView` dos testes que as constroem (`App.test.tsx`, `App.invariants.test.tsx`, `App.bigbox-request.test.tsx`, `App.metadata.test.tsx`, `components/GameDetailsPanel.test.tsx`, `GameEditDialog.test.tsx`, `GameEditForm*.test.tsx`, `GameImagesEditor.test.tsx`, `bigbox/**/*.test.tsx` e `bigbox-scan.test.ts` que montam `GameDetailView`), acrescentando `sessionPlaySeconds`, `source` e `editedAt`. Anotar `alterado em 2026-11-25 (fase 16)`.
- Testes (`QueryClientProvider` real e `window.go.main.App` falso):
  - `SEL-013/useCreateSession chama CreateSession com o gameId e o input e devolve a resposta`;
  - `SEL-013/depois de criar, editar ou excluir, o detalhe do jogo é preenchido com o game da resposta, sem nova chamada GetGame`;
  - `SEL-013/depois de uma escrita, a lista de sessões do jogo, as listas de jogos, os recentes e as estatísticas são recarregados uma vez (invalidação de libraryKeys.sessions(gameId), libraryGamesKey e statsKey)`;
  - `SEL-013/falha do agente não invalida nada e propaga o Error com a mensagem do agente`;
  - `SEL-013/o erro "Esta sessão não existe mais." é reconhecido por isSessionNotFound`;
  - `SEL-013/sem polling: nenhuma chamada além das invalidações`.

## T28 — Frontend: `lib/sessionEdit.ts` e tipos [concluída]
- Requisitos: SEL-013. **Responsável:** implementer. Dependência: T27.
- Arquivos: `frontend/src/lib/sessionEdit.ts` (novo), `frontend/src/lib/library.ts` (`SessionItemView` ganha `source: string` e `editedAt: string`; `GameDetailView` ganha `sessionPlaySeconds: number`), `frontend/src/lib/stats.ts` (`manualSeconds: number` em `StatsGame`, `StatsPlatform` e `StatsSummary`; `importedShare` não muda).
```ts
export const sessionNotFoundMessage = 'Esta sessão não existe mais.'
export function isSessionNotFound(err: unknown): boolean
export type SessionCreateInput = { startedAt: string; endedAt: string; activeSeconds: number; includedInTotal: boolean }
export type SessionWriteResult = { session: SessionItemView; game: GameDetailView }
export function useCreateSession(gameId: number): UseMutationResult<SessionWriteResult, Error, SessionCreateInput>
export function useUpdateSession(gameId: number): UseMutationResult<SessionWriteResult, Error, { sessionId: number; patch: Record<string, unknown> }>
export function useDeleteSession(gameId: number): UseMutationResult<GameDetailView, Error, number>
```
- No sucesso: `setQueryData(libraryKeys.game(gameId), game)` e `invalidateQueries` de `libraryKeys.sessions(gameId)`, `libraryGamesKey` (cobre `recent`) e `statsKey`. Chamadas via `window.go.main.App` com o padrão `call`/`toError` de `lib/library.ts` (Wails rejeita com string). Sem testes novos (T27 cobre).

## T29 — Testes: `SessionFormModal` [concluída]
- Requisitos: SEL-005, SEL-006, SEL-007, SEL-008, SEL-009, SEL-010, SEL-011, SEL-015. **Responsável:** test-writer. Dependência: T28.
- Arquivo: `frontend/src/components/SessionFormModal.test.tsx` (novo; props `now` injetável; bindings falsos).
- Testes:
  - `SEL-005/modal compacto com título "Adicionar sessão" ou "Editar sessão" e os campos Início, Fim e Tempo jogado (horas e minutos)`;
  - `SEL-005/criação abre com os valores iniciais (Fim = agora ao minuto, Início = Fim − 1 h, 1 h 00)`;
  - `SEL-005/edição abre com os valores da sessão`;
  - `SEL-006/mudar Início atualiza o Tempo jogado enquanto ele acompanha o intervalo; depois de digitá-lo, deixa de acompanhar`;
  - `SEL-007/campo inválido mostra a mensagem do erro, Salvar fica desativado e nada é enviado`;
  - `SEL-007/um caso por mensagem: fim antes do início, no futuro, mais de 24 h, tempo menor que 1 min, maior que o intervalo, minutos 60, campo vazio`;
  - `SEL-008/na edição, Salvar chama UpdateSession só com os campos alterados`;
  - `SEL-008/na criação, Salvar envia todos os campos com os instantes em UTC`;
  - `SEL-008/sem alteração na edição, Salvar fica desativado`;
  - `SEL-009/com ajuste positivo aparece a caixa "Já está no tempo total", desmarcada, com a ajuda "O tempo total deste jogo tem X a mais que as sessões. Marque se esta sessão já faz parte desse tempo."`;
  - `SEL-009/marcada, o envio leva includedInTotal true e tempo acima do ajuste é erro`;
  - `SEL-009/sem ajuste positivo e na edição a caixa não aparece`;
  - `SEL-010/erro do agente mantém o modal aberto com os valores digitados e mostra a mensagem em role="alert"`;
  - `SEL-010/"Esta sessão não existe mais." fecha o modal e pede o recarregamento da lista`;
  - `SEL-011/durante o envio o botão fica desativado com "Salvando…" e um segundo clique não chama o agente de novo`;
  - `SEL-015/cada campo tem rótulo associado; campo com erro tem aria-invalid="true" e aria-describedby apontando para a mensagem`;
  - `SEL-015/o foco começa em Início; Enter num campo salva quando Salvar está ativo; Esc cancela sem chamar o agente`;
  - `SEL-015/ao fechar, o foco volta ao botão que abriu o modal (ou a "Adicionar sessão" quando a linha não existe mais)`.

## T30 — Frontend: `SessionFormModal` [concluída]
- Requisitos: SEL-005, SEL-006, SEL-007, SEL-008, SEL-009, SEL-010, SEL-011, SEL-015. **Responsável:** implementer. Dependência: T29.
- Arquivo: `frontend/src/components/SessionFormModal.tsx` (novo).
```ts
export type SessionFormModalProps = {
  mode: 'create' | 'edit'
  game: GameDetailView
  session?: SessionItemView // edit only
  now?: () => Date          // injectable clock of the UI (defaults to new Date)
  onClose: () => void       // cancel or successful save
  onGone: () => void        // -32017: close and reload the list
}
export function SessionFormModal(p: SessionFormModalProps): JSX.Element
```
- Usa `Modal size="compact"`, `<input type="datetime-local" step="60">`, campos numéricos de horas e minutos, `lib/sessionForm.ts` para estado e validação e `useCreateSession`/`useUpdateSession`; o erro do agente fica em `<p role="alert">`; cores só por tokens de tema (`text-muted`, `text-warning`, `text-danger`; ADR 0029). Sem testes novos (T29 cobre).

## T31 — Testes: `SessionDeleteDialog`, `SessionsList` e `GameDetailsPanel` [concluída]
- Requisitos: SEL-001, SEL-002, SEL-003, SEL-004, SEL-010, SEL-011, SEL-012, SEL-013, SEL-015. **Responsável:** test-writer. Dependência: T30.
- Arquivos: `frontend/src/components/SessionDeleteDialog.test.tsx` (novo); alterar `SessionsList.test.tsx` (a prop `game` é nova e obrigatória; anotar `alterado em 2026-11-25 (fase 16)`) e `GameDetailsPanel.test.tsx`.
- Testes (`SessionDeleteDialog`):
  - `SEL-012/texto "Excluir a sessão de «início» («tempo jogado»)? O tempo dela sai do tempo total do jogo. Esta ação não pode ser desfeita."`;
  - `SEL-012/para a descartada: "Excluir a sessão descartada de «início»? Esta ação não pode ser desfeita."`;
  - `SEL-012/botões "Excluir" e "Cancelar" e o foco inicial em "Cancelar"`;
  - `SEL-012/Cancelar, Esc e fechar não chamam o agente; Excluir chama DeleteSession uma vez`;
  - `SEL-011/durante a exclusão o botão fica desativado com "Excluindo…" e um segundo clique não chama o agente de novo`;
  - `SEL-010/erro do agente aparece em role="alert" e o diálogo continua aberto; "Esta sessão não existe mais." fecha e recarrega`.
- Testes (`SessionsList`, agente e bindings falsos):
  - `SEL-001/o botão "Adicionar sessão" aparece com sessões, com a lista vazia ("Nenhuma sessão registrada.") e com o jogo rodando`;
  - `SEL-001/o botão abre o formulário de criação`;
  - `SEL-002/sessão contada mostra "Editar" e "Excluir"; descartada mostra só "Excluir"; a aberta e o SessionPanel não mostram nenhum dos dois`;
  - `SEL-002/os botões têm nome acessível com o início da sessão (aria-label="Editar sessão de 12/11/2026 20:00")`;
  - `SEL-003/origem manual mostra "Manual", import mostra "Importada", editedAt mostra "Editada", importada e editada mostram as duas, rastreada sem edição e origem desconhecida não mostram marca`;
  - `SEL-004/depois de criar, editar ou excluir, a lista mostra o que o agente devolveu; a UI não soma nem subtrai tempo`;
  - `SEL-013/depois de uma escrita a lista é recarregada uma vez e o detalhe do jogo mostra o total do game da resposta`;
  - `SEL-015/depois de excluir a linha, o foco vai para "Adicionar sessão"`;
  - `SEL-015/depois de salvar uma edição, o foco volta ao botão "Editar" da linha`.
- Testes (`GameDetailsPanel`): `SEL-004/o painel passa o jogo (total e sessionPlaySeconds) para a lista de sessões`.

## T32 — Frontend: `SessionDeleteDialog`, `SessionsList` e `GameDetailsPanel` [concluída]
- Requisitos: SEL-001, SEL-002, SEL-003, SEL-004, SEL-010, SEL-011, SEL-012, SEL-013, SEL-015. **Responsável:** implementer. Dependência: T31.
- Arquivos: `frontend/src/components/SessionDeleteDialog.tsx` (novo), `frontend/src/components/SessionsList.tsx` (botão "Adicionar sessão", ações por linha, marcas), `frontend/src/components/GameDetailsPanel.tsx` (passa `game`).
```ts
export type SessionDeleteDialogProps = {
  session: SessionItemView
  pending: boolean
  error: string | null
  onConfirm: () => void
  onCancel: () => void
}
export function SessionDeleteDialog(p: SessionDeleteDialogProps): JSX.Element
// SessionsList: `game` is the GameDetailView of the page; the other props stay.
export function SessionsList(p: { gameId: number; current: SessionView | null; game: GameDetailView }): JSX.Element
```
- Foco inicial em "Cancelar" (mesmo padrão do `DeleteDialog`). A lista guarda no estado qual modal está aberto e qual botão o abriu para devolver o foco (SEL-015). Marcas com tokens de tema (`text-muted`, `text-warning`). Sem testes novos (T31 cobre).

## T33 — Testes: marca de manuais nas estatísticas [concluída]
- Requisitos: SEL-014, SED-027. **Responsável:** test-writer. Dependência: T28.
- Arquivo: `frontend/src/components/StatsPage.test.tsx` (alterar; os fixtures de `StatsGame`/`StatsPlatform`/`StatsSummary` ganham `manualSeconds`).
- Testes:
  - `SEL-014/total do período com manualSeconds > 0 mostra "· X adicionados à mão" ao lado da marca de importados, no mesmo formato`;
  - `SEL-014/linha de jogo com manualSeconds > 0 mostra "inclui X adicionados à mão"`;
  - `SEL-014/manualSeconds 0 não mostra a marca`;
  - `SEL-014/importados e manuais no mesmo item mostram as duas marcas`.

## T34 — Frontend: marca de manuais nas estatísticas [concluída]
- Requisitos: SEL-014. **Responsável:** implementer. Dependência: T33 (e a Dúvida 10: `StatsPage.tsx` já tem mudanças não commitadas).
- Arquivo: `frontend/src/components/StatsPage.tsx`; `frontend/src/lib/stats.ts` já tem o campo (T28).
- Sem testes novos (T33 cobre).

## T35 — ADR 0041 [concluída]
- Requisitos: SED-002, SED-007, SED-009, SED-012, SED-020. **Responsável:** sessão principal (docs). Dependência: nenhuma (pode sair a qualquer momento depois de T21).
- Arquivos: `docs/adr/0041-sessoes-editaveis-pelo-usuario.md` (novo), `docs/adr/README.md` (linha do 0041).
- Conteúdo (design §9): (1) sessões encerradas são editáveis e excluíveis só pelo agente, a aberta nunca; (2) origem `manual` e marca `edited_at` (a origem não muda ao editar); (3) o total muda pelo delta e conserva o ajuste de EDM-005, enquanto primeira e última vez e contagem são recalculadas das sessões contadas; (4) numa sessão editada `active_seconds` é a verdade e as pausas viram registro auxiliar cortado ao intervalo, ressalva à consequência "tempo recalculável a partir de sessão e pausas" do ADR 0017 (não o substitui); (5) sem sobreposição entre sessões rastreadas e manuais de jogos diferentes. Alternativas descartadas: recalcular o total da soma (apagaria o ajuste da fase 14), origem `edited` no lugar da marca, permitir sobreposição (estatísticas por semana somariam o mesmo tempo duas vezes).

## T36 — Documentação [concluída]
- Requisitos: SED-001, SED-002, SED-033, SED-034, SEL-001, SEL-014 (documentação das mudanças dessas áreas). **Responsável:** sessão principal (docs). Dependência: T32, T34, T35.
- Arquivos:
  - `docs/ARCHITECTURE.md`: §5 `play_session(..., source[tracked|import|manual], edited_at)` (hoje diz `source[tracked|manual]`, mas o código grava `import`); índice `play_session_started_at_idx`; §4.6 ("log de ajuste" = a linha de log de SED-029); a lista de IPC (métodos `session.*`, erros -32017 e -32018, v0.17.0); migração 11; linha da fase 16 junto da 412 ("Versão 1.2.0");
  - `CLAUDE.md`: linha "Spec atual" passa a apontar `phase-16-edicao-sessoes` ("Edição de sessões") sobre `phase-15-big-box-v2`; linha da versão 1.2.0 (Fase "Versão 1.2.0 (2026-11-25): criar, editar e excluir sessões de um jogo…");
  - `README.md` (versão atual e a seção de funcionalidades com a edição de sessões), `docs/ROADMAP.md` (seção "Fase 16 — Edição de sessões" e "Versão 1.2.0"), `docs/GUIA-DO-PROJETO.md` (título da versão e a seção do painel de detalhes: botões, marcas, formulário, confirmação, caixa "Já está no tempo total");
  - `docs/specs/phase-16-edicao-sessoes/requirements.md`: Status passa a "aprovado em 2026-11-25" e "Perguntas em aberto" vira "Decisões confirmadas" com as respostas padrão; as notas "· proposto" de SED-037 saem;
  - anotações `· alterado em 2026-11-25 (docs/specs/phase-16-edicao-sessoes, <ID>)` nas specs antigas: `phase-3-tracker` (STO-016, o total deixa de ser sempre a soma das sessões, SED-007), `phase-5-library` (STO-027, IPC-050 e UI-038), `phase-8-estatisticas` (EST-001, EST-002, EST-003, só sessões contadas, SED-037, e EST-005, sessões manuais, SED-027), `phase-6-importers` (IMP-007, a sessão importada pode ser editada e excluída, SED-021) e `phase-14-edicao-manual` (EDM-005, "inalterado; ver SED-007"); o `ADR 0017` ganha só uma linha de ponteiro para o 0041 no `docs/adr/README.md`, sem editar o texto do ADR.

## T37 — Versão 1.2.0 [concluída]
- Requisitos: nenhum requisito próprio (decisão de release, resposta 9). **Responsável:** sessão principal (mecânico). Dependência: T36.
- Arquivos: `cmd/arcadia/wails.json` (`productVersion`), `frontend/package.json` e `frontend/package-lock.json` (os dois campos `version` do topo; não tocar nas versões de dependências), `CLAUDE.md` (comando de build `-X main.version=1.2.0`, "versão atual: 1.2.0"), `README.md`, `docs/GUIA-DO-PROJETO.md` e `docs/ROADMAP.md` (comando de build). Conferir `cmd/arcadia/version.go` e `version_test.go`: a versão vem do `-ldflags`, então não há literal para trocar.

## T38 — Verificação final e roteiro manual
- Requisitos: SEL-017. **Responsável:** sessão principal, com o usuário (parte manual). Dependência: todas.
- Automático: `go build ./...`, `go vet ./...`, `CGO_ENABLED=0 go build ./...`, `go test ./... -race`, `npm test` e `npx tsc --noEmit` em `frontend/`, `scripts/spec-trace.ps1 -Spec docs/specs/phase-16-edicao-sessoes` e `wails build -ldflags "-X main.version=1.2.0"` (regera `frontend/wailsjs/go/**`; entra no commit). Conferir também com `git diff --stat` que `internal/tracker/` e `cmd/fake-game/` não mudaram.
- **Manual (SEL-017):** com o agente rodando e um jogo com sessões rastreadas, importada e descartada:
  1. criar uma sessão manual de ontem (sem e com "Já está no tempo total" num jogo com ajuste de tempo da fase 14);
  2. editar o fim de uma rastreada e conferir "Editada", o total, a primeira e a última vez e a contagem;
  3. editar uma importada (mover o início; ver a Dúvida 8) e excluir uma descartada (total não muda) e uma contada (total diminui);
  4. tentar cruzar outra sessão (mensagem com o título), fim no futuro e mais de 24 h;
  5. com o jogo rodando: editar outra sessão dele, tentar editar a aberta (sem botões), encerrar o jogo e conferir o total;
  6. conferir "adicionados à mão" nas estatísticas e que a descartada e a aberta não somam (SED-037);
  7. lançar o jogo depois disso e conferir que rastreia normalmente; Tab e Esc no formulário e foco de volta ao fechar.

---

## Riscos de concorrência e recursos

- **Goroutines novas:** nenhuma. Os handlers rodam na goroutine da conexão IPC (como `manual_edit.go`); a UI usa só hooks do TanStack Query (sem temporizadores novos).
- **Handles do Windows:** nenhum.
- **Locks e atomicidade:** toda escrita passa por `Store.Tx` (`BEGIN IMMEDIATE`). A leitura da sessão, a checagem de sobreposição, a escrita da sessão, o corte das pausas, o `AddToTotal` e o recálculo acontecem na mesma transação, e o tracker grava pelo mesmo `Store`, então `EndSession` (incremental, STO-014) e a edição se serializam. Se a edição terminar antes, a sessão aberta ainda soma depois; se `EndSession` terminar antes, a sessão já está encerrada e passa a ser editável. SED-025 cobre a sequência (T09).
- **Ordem de shutdown:** inalterada. O servidor IPC para de aceitar pedidos antes de fechar o store (design §5); nada novo para esperar.
- **Notificações:** nenhuma nova. A UI invalida o próprio cache (T28); outras janelas abertas só veem a mudança ao recarregar, como em qualquer escrita da biblioteca (IPC-050).

## Riscos e dúvidas de implementação

1. **Migração 11 em banco com dados:** `ALTER TABLE ... ADD COLUMN` é seguro e o índice novo é barato; o `Down` usa `DROP COLUMN` (SQLite ≥ 3.35, já usado na 00003).
2. **`sqlc generate`:** as tarefas T04..T14 regeneram `dbgen/`; conferir `git diff` do código gerado e rodar `go build ./...` antes do commit. `COALESCE(SUM(...), 0)` e `MIN`/`MAX` escalares precisam de `CAST`/`sqlc.arg` para o sqlc inferir `int64`.
3. **Teste de atomicidade (SED-014):** forçar a falha com um `TRIGGER BEFORE UPDATE ON game ... RAISE(ABORT)` criado pelo próprio teste depois da escrita da sessão; não alterar o código de produção para injetar falha.
4. **Fuso no Vitest:** ver Dúvida 9; SEL-016 só vale com TZ fixo.
5. **Testes antigos que listam colunas ou campos:** a T03 (store e agente), a T16 (contratos) e a T27 (fixtures TypeScript) concentram o ajuste; cada tarefa de código só começa com o conjunto compilando.
6. **Sobreposição e relógio:** `in-future` e a sessão aberta `[started_at, T)` usam o `Clock` do agente; o desktop não tem relógio da regra (o do formulário só antecipa o erro).
7. **Mensagens duplicadas:** os textos de SED-036 existem em Go (desktop) e em TypeScript (`sessionReasonMessages`); um teste em cada lado cita a mesma tabela do contrato §4, e a T38 confere os dois visualmente.

## Decisões novas que merecem ADR

**0041** (sessões editáveis pelo usuário: origem `manual`, total por diferença, sem sobreposição), escrito na T35. Nenhuma outra: "versão 1.2.0" é decisão de release, não de arquitetura.

---

## Rastreabilidade

Colunas: tarefa(s) de código, tarefa(s) de testes. A conferência automática é `scripts/spec-trace.ps1 -Spec docs/specs/phase-16-edicao-sessoes` (`-SkipTests` antes das tarefas de teste). Verificação manual (sem teste exigido): SEL-017 (T38).

| Requisito | Código | Testes | Arquivo principal do teste |
|---|---|---|---|
| SED-001 | T04 | T03 | `internal/store/migrate11_test.go` |
| SED-002 | T02, T06 | T01, T05 | `domain/session_edit_test.go`, `store/sessions_list_test.go` |
| SED-003 | T08 | T07 | `store/session_edit_create_test.go` |
| SED-004 | T08 | T07 | `store/session_edit_create_test.go` |
| SED-005 | T08, T21 | T07, T20 | `store/session_edit_create_test.go`, `agent/session_edit_test.go` |
| SED-006 | T10 | T09 | `store/session_edit_update_test.go` |
| SED-007 | T10 | T09 | `store/session_edit_update_test.go` |
| SED-008 | T10, T21 | T09, T20 | `store/session_edit_update_test.go` |
| SED-009 | T02, T10 | T01, T09 | `domain/session_edit_test.go`, `store/session_edit_update_test.go` |
| SED-010 | T12 | T11 | `store/session_edit_delete_test.go` |
| SED-011 | T12 | T11 | `store/session_edit_delete_test.go` |
| SED-012 | T08, T10, T12 | T07, T09, T11 | `store/session_edit_*_test.go` |
| SED-013 | T02, T08, T10, T12, T21 | T01, T07, T09, T11, T16, T20 | `domain/session_edit_test.go`, `store/session_edit_*_test.go` |
| SED-014 | T08, T10, T12 | T07, T09, T11 | `store/session_edit_*_test.go` |
| SED-015 | T02, T08, T10, T21 | T01, T07, T09, T20 | `domain/session_edit_test.go`, `agent/session_edit_test.go` |
| SED-016 | T02, T08, T10, T21 | T01, T07, T09, T20 | `domain/session_edit_test.go` |
| SED-017 | T02, T08, T10, T21 | T01, T07, T09, T20 | `domain/session_edit_test.go` |
| SED-018 | T02, T08, T10, T21 | T01, T07, T09, T20 | `domain/session_edit_test.go`, `store/session_edit_update_test.go` |
| SED-019 | T02, T08, T10, T21 | T01, T07, T09, T20, T25 | `domain/session_edit_test.go`, `lib/sessionForm.test.ts` |
| SED-020 | T02, T08, T10, T21 | T01, T07, T09, T20 | `store/session_edit_*_test.go`, `agent/session_edit_test.go` |
| SED-021 | T02, T10 | T01, T09 | `store/session_edit_update_test.go` |
| SED-022 | T10, T12, T21 | T09, T11, T20 | `agent/session_edit_test.go` |
| SED-023 | T10, T21 | T09, T20 | `agent/session_edit_test.go` |
| SED-024 | T08, T10, T12, T21 | T07, T09, T11, T20 | `agent/session_edit_test.go` |
| SED-025 | T10, T21 | T09, T20 | `store/session_edit_update_test.go` |
| SED-026 | T21 | T20 | `agent/session_edit_test.go` |
| SED-027 | T14, T19, T23, T34 | T13, T18, T22, T33 | `store/stats_test.go`, `agent/stats_test.go`, `cmd/arcadia/session_edit_test.go` |
| SED-028 | T08, T10, T12 | T07, T09, T11 | `store/session_edit_*_test.go` |
| SED-029 | T21 | T20 | `agent/session_edit_test.go` |
| SED-030 | T15, T17 | T16 | `internal/ipc/contract_phase16_test.go` |
| SED-031 | T17, T21 | T20 | `agent/session_edit_test.go` |
| SED-032 | T17, T21 | T20 | `agent/session_edit_test.go` |
| SED-033 | T06, T17, T19 | T05, T16, T18 | `store/sessions_list_test.go`, `ipc/contract_phase16_test.go` |
| SED-034 | T06, T17, T19 | T05, T16, T18 | `store/library_detail_test.go`, `ipc/contract_phase16_test.go` |
| SED-035 | T23, T24 | T22 | `cmd/arcadia/session_edit_test.go` |
| SED-036 | T23, T26 | T22, T25 | `cmd/arcadia/session_edit_test.go`, `lib/sessionForm.test.ts` |
| SED-037 | T14 | T13 | `store/stats_test.go` |
| SEL-001 | T32 | T31 | `components/SessionsList.test.tsx` |
| SEL-002 | T32 | T31 | `components/SessionsList.test.tsx` |
| SEL-003 | T32 | T31 | `components/SessionsList.test.tsx` |
| SEL-004 | T32 | T31 | `components/SessionsList.test.tsx` |
| SEL-005 | T26, T30 | T25, T29 | `lib/sessionForm.test.ts`, `components/SessionFormModal.test.tsx` |
| SEL-006 | T26, T30 | T25, T29 | `lib/sessionForm.test.ts` |
| SEL-007 | T26, T30 | T25, T29 | `lib/sessionForm.test.ts` |
| SEL-008 | T26, T30 | T25, T29 | `lib/sessionForm.test.ts` |
| SEL-009 | T26, T30 | T25, T29 | `lib/sessionForm.test.ts` |
| SEL-010 | T30, T32 | T29, T31 | `components/SessionFormModal.test.tsx`, `SessionDeleteDialog.test.tsx` |
| SEL-011 | T30, T32 | T29, T31 | `components/SessionFormModal.test.tsx`, `SessionDeleteDialog.test.tsx` |
| SEL-012 | T32 | T31 | `components/SessionDeleteDialog.test.tsx` |
| SEL-013 | T28, T32 | T27, T31 | `lib/sessionEdit.test.tsx` |
| SEL-014 | T34 | T33 | `components/StatsPage.test.tsx` |
| SEL-015 | T30, T32 | T29, T31 | `components/SessionFormModal.test.tsx`, `SessionsList.test.tsx` |
| SEL-016 | T26 | T25 | `lib/sessionForm.test.ts` |
| SEL-017 | (manual) | T38 | roteiro manual |

Nenhum requisito ficou sem tarefa. As lacunas 1, 2 e 3 pedem ajuste de texto do spec-writer, mas nenhuma bloqueia o início das tarefas.