# Fase 16 — Edição de sessões: design

Requisitos: [requirements.md](requirements.md). Contrato: [contracts/ipc-v0.17.0-delta.md](contracts/ipc-v0.17.0-delta.md).
Nada aqui contradiz ADRs aceitos; o §9 lista o que pede ADR novo.

## 1. Visão geral

```
SessionsList ──(Wails)── App.CreateSession/UpdateSession/DeleteSession ──(IPC)── agent.sessionEdit
                                                                                   │ valida (domain)
                                                                                   ▼
                                                                    store.CreateManualSession / EditSession / RemoveSession
                                                                    (uma transação BEGIN IMMEDIATE: sessão, pausas, jogo)
```

- Escreve só o agente (ADR 0015). A UI pede, recebe `{session, game}` e invalida o próprio cache.
- O tracker não muda nem é chamado: a única interação é a regra "sessão aberta não se toca" (SED-022), lida do banco
  (`ended_at IS NULL`). Como o tracker grava a sessão antes de retomar o processo (SES-001), a linha aberta existe durante
  todo o tempo em que o tracker tem sessão.
- Pacotes tocados: `internal/domain`, `internal/store` (migração 11, queries, funções novas), `internal/ipc` (tipos e
  constantes), `internal/agent` (handlers), `cmd/arcadia` (bindings e mensagens), `frontend/` (lista, formulário,
  confirmação, estatísticas). **Não tocados:** `internal/tracker/`, `cmd/fake-game/`, `internal/input`, `internal/feedback`.

## 2. Domínio `[SED-002, SED-015..SED-021, SED-023]`

`internal/domain/session_edit.go` (puro, sem Windows):

```go
type SessionSource string

const (
    SourceTracked SessionSource = "tracked"
    SourceImport  SessionSource = "import"
    SourceManual  SessionSource = "manual"
)

func ParseSessionSource(s string) (SessionSource, error) // [SED-002]

const (
    MaxEditedSessionSpan  = 24 * time.Hour // [SED-018]
    MinEditedSessionActive = 30 * time.Second // = limite de DOM-008 [SED-019]
)

var EarliestSessionStart = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC) // [SED-017]

// EditReason is the closed set of data.reason values of -32602 (contract §4).
type EditReason string

const (
    ReasonEndNotAfterStart  EditReason = "end-not-after-start"
    ReasonInFuture          EditReason = "in-future"
    ReasonTooEarly          EditReason = "too-early"
    ReasonSpanTooLong       EditReason = "span-too-long"
    ReasonActiveTooShort    EditReason = "active-too-short"
    ReasonActiveExceedsSpan EditReason = "active-exceeds-span"
    ReasonDiscarded         EditReason = "discarded"
    ReasonExceedsAdjustment EditReason = "exceeds-adjustment"
    ReasonEmptyPatch        EditReason = "empty-patch"
)

// SessionTimes are the resulting values of a create or edit, after merging the patch.
type SessionTimes struct {
    Start, End time.Time
    Active     time.Duration
}

// ValidateSessionTimes checks one resulting session against now. spanChecked is false for an
// imported session and for an edit that touches neither start nor end [SED-018, SED-021].
// It returns "" when valid. Order of checks: too-early, end-not-after-start, in-future,
// span-too-long, active-too-short, active-exceeds-span.
func ValidateSessionTimes(t SessionTimes, now time.Time, spanChecked bool) EditReason

// Overlaps reports whether [a.Start, a.End) and [b.Start, b.End) cross; touching is not crossing [SED-020].
func Overlaps(aStart, aEnd, bStart, bEnd time.Time) bool

// ClampTotal keeps a game total inside 0..max [SED-013].
func ClampTotal(seconds, max int64) int64

// ClipPause cuts a pause to [start, end]; ok is false when nothing is left [SED-009].
func ClipPause(pausedAt time.Time, resumedAt *time.Time, start, end time.Time) (p, r time.Time, ok bool)
```

`ValidateSessionTimes` é a única fonte das regras; o frontend replica as mesmas regras em `lib/sessionForm.ts` (SEL-007), e os
testes dos dois lados usam a mesma tabela de exemplos (contrato §5).

## 3. Store `[SED-001, SED-003..SED-014, SED-020, SED-025, SED-027, SED-034, SED-037]`

### 3.1 Migração 11 `[SED-001]`

`internal/store/migrations/00011_session_edit.sql`:

```sql
-- +goose Up
ALTER TABLE play_session ADD COLUMN edited_at INTEGER;
CREATE INDEX play_session_started_at_idx ON play_session (started_at);
-- +goose Down
DROP INDEX play_session_started_at_idx;
ALTER TABLE play_session DROP COLUMN edited_at;
```

`store.SchemaVersion` passa de 10 para 11. ⚠️ Vários testes conferem `SchemaVersion = 10` (`media_test.go`, `migrate5_test.go`,
`migrate9_test.go`, `migrate10_test.go`): a tarefa de testes precisa ajustá-los (só o test-writer edita `*_test.go`). O índice
atende a checagem de sobreposição entre jogos e as consultas de estatística por período.

### 3.2 Queries novas (`internal/store/queries/session_edit.sql`, `sqlc generate`)

| Query | Uso |
|---|---|
| `GetSessionForEdit :one` — `id, game_id, state, source, started_at, ended_at, active_seconds, edited_at` por id | ler a sessão alvo |
| `InsertManualSession :one` — `state='ended'`, `source='manual'`, `last_seen_at = ended_at` | SED-003 |
| `UpdateSessionTimes :execrows` — `started_at, ended_at, last_seen_at, active_seconds, edited_at` `WHERE id = ? AND ended_at IS NOT NULL` | SED-006 |
| `DeleteClosedSession :execrows` — `WHERE id = ? AND ended_at IS NOT NULL` | SED-010, SED-011 (pausas por `ON DELETE CASCADE`) |
| `FindOverlappingSession :one` — primeira sessão com `id <> :self` e ((`state='ended' AND source IN ('tracked','manual') AND started_at < :end AND ended_at > :start`) ou (`ended_at IS NULL AND started_at < :end`)), com o título do jogo | SED-020 |
| `SumCountedSeconds :one` — `SUM(active_seconds)` das contadas do jogo | SED-005, SED-034 |
| `RecomputeGamePlayFacts :execrows` — `first_played_at`, `last_played_at`, `play_count` por subconsultas das contadas | SED-012 |
| `AddToTotal :execrows` — `total_play_seconds = MIN(MAX(total_play_seconds + :delta, 0), :max)`, `updated_at` | SED-004, SED-007, SED-010, SED-013 |
| `ListPausesOfSession` / `UpdatePauseBounds` / `DeletePause` | SED-009 |

A sessão aberta entra na sobreposição como `[started_at, T)`: como o fim pedido nunca passa de `T` (SED-016), a condição
`started_at < :end` basta.

Estatísticas (`stats.sql`): `StatsByGame` e `StatsByPlatform` ganham `manual_seconds` (`CASE WHEN s.source = 'manual'`) e o
total geral soma os dois `[SED-027]`; se a pergunta 7 for aceita, as três consultas (com `SessionTimesSince`) ganham
`AND s.state = 'ended'` `[SED-037]`.

`ListSessionsByGame` passa a devolver `source` e `edited_at` `[SED-033]`; `GameDetail` lê `sessionPlaySeconds` com
`SumCountedSeconds` `[SED-034]`.

### 3.3 Funções públicas (`internal/store/session_edit.go`)

```go
var (
    ErrSessionNotFound = errors.New("store: session not found")   // -> -32017
    ErrSessionOpen     = errors.New("store: session is open")      // -> -32002
)

// SessionEditError carries a domain.EditReason (-> -32602 with data.reason).
type SessionEditError struct{ Reason domain.EditReason }

// SessionOverlapError carries the conflicting session (-> -32018).
type SessionOverlapError struct {
    SessionID, GameID int64
    Title             string
    StartedAt         time.Time
    EndedAt           *time.Time
}

type ManualSession struct {
    GameID          int64
    Start, End      time.Time
    Active          time.Duration
    IncludedInTotal bool
}

type SessionPatch struct {
    Start, End *time.Time
    Active     *time.Duration
}

// SessionWrite is what the three operations return; Before/After totals feed the log [SED-029].
type SessionWrite struct {
    Session                 SessionRow  // the row after the write (zero on delete)
    Game                    GameDetail
    TotalBefore, TotalAfter int64
    Previous                *SessionRow // nil on create
}

func (s *Store) CreateManualSession(ctx context.Context, m ManualSession, now time.Time) (SessionWrite, error)
func (s *Store) EditSession(ctx context.Context, id int64, p SessionPatch, now time.Time) (SessionWrite, error)
func (s *Store) RemoveSession(ctx context.Context, id int64, now time.Time) (SessionWrite, error)
```

`now` é o `Clock.Now()` do agente, usado para `in-future`, para o `edited_at` e para `updated_at` (STO-014 usa o mesmo
padrão de instante de escrita).

### 3.4 Ordem dentro da transação `[SED-014]`

Tudo dentro de `Store.Tx` (BEGIN IMMEDIATE, `_txlock=immediate`), por isso a checagem de sobreposição e a escrita são atômicas
em relação ao tracker, que grava pelo mesmo `Store` no mesmo processo:

1. Ler a sessão (`ErrSessionNotFound`) e o jogo (`ErrGameNotFound` na criação).
2. Sessão aberta → `ErrSessionOpen` (SED-022). Edição de descartada → `discarded` (SED-023).
3. Montar os valores resultantes (patch sobre o gravado); igual ao gravado → devolve sem escrever (SED-008).
4. `domain.ValidateSessionTimes` (SED-015..SED-019, SED-021).
5. Criação com `IncludedInTotal`: `total − SumCountedSeconds ≥ a`, senão `exceeds-adjustment` (SED-005).
6. Sobreposição (só origens `tracked`/`manual` e só quando início ou fim estão no pedido) → `SessionOverlapError` (SED-020).
7. Escrever a sessão; cortar pausas se início ou fim mudaram (SED-009).
8. `AddToTotal` com o delta (0 em descartada e com `IncludedInTotal`; não roda quando delta = 0 e nada mais muda no jogo).
9. `RecomputeGamePlayFacts` e `updated_at = now` só se algum campo do jogo mudou (SED-011, SED-012).
10. Ler `GameDetail` para a resposta.

## 4. Total, primeira e última vez `[SED-004, SED-007, SED-010, SED-012, SED-013, SED-025]`

- O total é um contador com dois componentes: o tempo das sessões contadas e o ajuste manual de EDM-005. As operações desta fase
  mudam só o primeiro componente (delta); o ajuste se conserva. Por isso não se recalcula o total da soma (isso apagaria o que
  o usuário digitou na fase 14).
- `first_played_at`, `last_played_at` e `play_count` são recalculados das sessões contadas: são função pura das sessões e nada
  no produto os edita à mão (`game.update` não tem esses campos). Isso também corrige valores quando a primeira ou a última
  sessão é editada ou excluída, o que um delta não faria.
- Com o jogo rodando, a sessão aberta não entra no recálculo; quando ela termina, `AddPlayToGame` (STO-014) soma o tempo dela,
  faz `last_played_at = E` e `first_played_at = COALESCE(atual, S)`: o resultado coincide com o recálculo, porque `E` é posterior
  a todo fim gravado (SED-016) e `S` só vale quando não há contada. SED-025 testa essa sequência no store real.
- O status do jogo não é tocado (SED-028); o tracker também nunca muda o status.

## 5. Agente `[SED-022..SED-024, SED-026, SED-029, SED-031, SED-032]`

`internal/agent/session_edit.go`, no estilo de `manual_edit.go`:

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

- Decodifica por nome (IPC-028), distingue ausente de `null` no patch (`ipc.Optional`, como `game.update`), converte RFC 3339 e
  trunca a ms (ADR 0009).
- Traduz erros: `ErrGameNotFound` → `-32001`; `ErrSessionNotFound` → `-32017`; `ErrSessionOpen` → `-32002` com `SessionRef`
  (título do jogo da sessão); `SessionEditError` → `-32602` com `data.reason`; `SessionOverlapError` → `-32018`; resto → `-32603`
  com `Error` no log.
- Log `Info` de SED-029 depois do commit, a partir de `SessionWrite`. Recusa em `Debug`.
- Não envia notificação (SED-026). Não chama o tracker. Sem goroutine nova: cada handler roda na goroutine da conexão IPC.
- Durante o shutdown, os métodos seguem a regra geral do servidor IPC (o agente para de aceitar pedidos antes de fechar o store).

## 6. IPC `[SED-030..SED-034]`

Contrato v0.17.0, só acréscimos (detalhe em [contracts/ipc-v0.17.0-delta.md](contracts/ipc-v0.17.0-delta.md)):

- Métodos `session.create`, `session.update`, `session.delete`.
- Erros `SessionNotFound` (`-32017`) e `SessionOverlap` (`-32018`); `InvalidParams` ganha `data.reason` para os três métodos.
- `SessionItem` ganha `source` e `editedAt`; `GameDetail` ganha `sessionPlaySeconds`; os resultados de estatística ganham
  `manualSeconds`.
- `internal/ipc/session_edit.go`: `MethodSessionCreate`, `MethodSessionUpdate`, `MethodSessionDelete`, `CodeSessionNotFound`,
  `CodeSessionOverlap`, `MsgSessionNotFound`, `MsgSessionOverlap`, `SessionCreateParams`, `SessionUpdateParams`,
  `SessionDeleteParams`, `SessionWriteResult`, `SessionDeleteResult`, `SessionOverlapData`, `InvalidParamsReasonData`.
- O JSON completo `contracts/ipc.openrpc.json` v0.17.0 é gerado por uma tarefa (v0.16.0 de phase-15 + delta), como na fase 15.
  ⚠️ Os testes de contrato que conferem os campos de `SessionItem` e `GameDetail` contra os tipos Go precisam do campo novo.

## 7. Desktop `[SED-035, SED-036]`

`cmd/arcadia/session_edit.go`:

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
func (a *App) UpdateSession(sessionID int64, patch json.RawMessage) (SessionWriteView, error) // 5 s; patch como UpdateGame
func (a *App) DeleteSession(sessionID int64) (GameDetailView, error)                         // 5 s
```

`SessionItemView` ganha `Source string` e `EditedAt string` (`""` quando nulo); `GameDetailView` ganha
`SessionPlaySeconds int64`. As mensagens de SED-036 entram no mapa de DSK-011; a de `-32018` usa o `data` do erro e formata os
instantes em hora local do processo do desktop. Bindings do Wails regenerados.

## 8. Frontend `[SEL-001..SEL-016]`

- `lib/sessionEdit.ts`: chamadas `createSession`, `updateSession`, `deleteSession` e os hooks `useCreateSession`,
  `useUpdateSession`, `useDeleteSession` (TanStack Query). No sucesso: `setQueryData` do detalhe com o `game` da resposta e
  invalidação de `libraryKeys.sessions(gameId)`, listas de jogos, recentes e chaves de estatística (SEL-013).
- `lib/sessionForm.ts` (puro, testável): estado do formulário, valores iniciais (SEL-005), regra de acompanhar o intervalo
  (SEL-006), validação com os mesmos limites do domínio (SEL-007), patch só do que mudou (SEL-008), conversão hora local ↔ UTC
  (SEL-016). O "agora" vem de um relógio injetável.
- `components/SessionFormModal.tsx`: `Modal` `compact`, campos `<input type="datetime-local" step="60">`, horas e minutos
  numéricos, caixa de SEL-009, erro do agente em `role="alert"` (SEL-010), pendência (SEL-011), acessibilidade (SEL-015).
- `components/SessionDeleteDialog.tsx`: confirmação no padrão de `DeleteDialog` (SEL-012).
- `components/SessionsList.tsx`: botão "Adicionar sessão", ações por linha, marcas (SEL-001..SEL-003). Precisa do `GameDetail`
  (para `totalPlaySeconds` e `sessionPlaySeconds`): `GameDetailsPanel` passa o jogo.
- `components/StatsPage.tsx`: marca de SEL-014 ao lado da de importados.
- Tokens de tema para as marcas (`text-muted`, `text-warning`), sem cor fixa (ADR 0029).

## 9. Decisões que pedem ADR (a sessão principal escreve)

**ADR 0041 — Sessões editáveis pelo usuário: origem `manual`, total por diferença e sem sobreposição.** Conteúdo:
(1) sessões encerradas passam a ser editáveis e excluíveis só pelo agente; a aberta nunca; (2) origem `manual` e marca
`edited_at` (a origem não muda ao editar); (3) o total muda pelo delta e preserva o ajuste manual de EDM-005, enquanto primeira e
última vez e contagem são recalculadas das sessões contadas; (4) numa sessão editada, `active_seconds` é a verdade e as pausas
viram registro auxiliar cortado ao intervalo, uma ressalva à consequência "tempo sempre recalculável a partir de sessão e pausas"
do ADR 0017 (que continua valendo para sessões não editadas; o ADR 0017 não é substituído); (5) a linha do tempo de sessões
rastreadas e manuais não tem sobreposição entre jogos. Alternativas: recalcular o total da soma (apaga o ajuste da fase 14);
origem `edited` no lugar da marca (perde a origem original); permitir sobreposição (estatísticas por semana somariam o mesmo
tempo duas vezes).

## 10. Riscos

| Risco | Mitigação |
|---|---|
| Corrida entre a edição e o fim da sessão aberta | Mesmo `Store`, transações `BEGIN IMMEDIATE` serializadas; `AddPlayToGame` é incremental; SED-025 testa a sequência |
| Sobreposição checada fora da transação | A checagem e a escrita estão na mesma transação (§3.4) |
| Sessão aberta aparece entre a validação da UI e o pedido | O agente revalida tudo; a sessão nova do tracker começa em `T` ≥ todo fim aceito |
| Hora local ambígua no horário de verão | Conversão pelo `Date` do navegador e eco do valor convertido (SEL-016) |
| Testes antigos presos a `SchemaVersion = 10` e aos campos de `SessionItem` | Tarefas de teste explícitas (§3.1, §6) |
| Total do jogo menor que uma sessão excluída (ajuste manual negativo) | Piso 0 (SED-013), registrado no log (SED-029) |
| Goroutines e handles | Nenhuma goroutine nova, nenhum handle do Windows |

## 11. Rastreabilidade (área → onde é testado)

| IDs | Testes esperados |
|---|---|
| SED-002, SED-015..SED-019, SED-021 (regras puras) | `internal/domain/session_edit_test.go`, table-driven, com a tabela do contrato §5 |
| SED-001 | `internal/store/migrate11_test.go` |
| SED-003..SED-014, SED-020, SED-025, SED-027, SED-028, SED-034, SED-037 | `internal/store/session_edit_test.go`, `stats_test.go` (SQLite em `t.TempDir()`) |
| SED-022..SED-024, SED-026, SED-029, SED-031, SED-032 | `internal/agent/session_edit_test.go` (store falso e real, `FakeClock`, servidor IPC de teste para SED-026) |
| SED-030, SED-033 | `internal/ipc/contract_phase16_test.go` |
| SED-035, SED-036 | `cmd/arcadia/session_edit_test.go` (agente falso) |
| SEL-001..SEL-016 | Vitest: `SessionsList.test.tsx`, `SessionFormModal.test.tsx`, `SessionDeleteDialog.test.tsx`, `sessionForm.test.ts`, `StatsPage.test.tsx` |
| SEL-017 | manual |
