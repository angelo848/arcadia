# IPC v0.17.0 — acréscimo sobre a v0.16.0 (fase 15)

Requisitos: SED-020, SED-022..SED-024, SED-026, SED-027, SED-030..SED-036. Formal (OpenRPC, só o acréscimo):
[ipc-v0.17.0-delta.openrpc.json](ipc-v0.17.0-delta.openrpc.json). O JSON completo `ipc.openrpc.json` v0.17.0 é gerado por uma
tarefa a partir da v0.16.0 de [phase-15](../../phase-15-big-box-v2/contracts/ipc.openrpc.json) e deste delta.
**Só acréscimos:** nenhum método, valor ou campo existente muda de sentido; quatro resultados ganham campos (§3), o que
clientes antigos ignoram.

## 1. Métodos novos

| Método | Parâmetros | Resultado |
|---|---|---|
| `session.create` | `gameId` (int ≥ 1), `startedAt`, `endedAt` (RFC 3339), `activeSeconds` (int 30..86 400), `includedInTotal` (bool, opcional, padrão `false`) | `{ "session": SessionItem, "game": GameDetail }` |
| `session.update` | `sessionId` (int ≥ 1) e pelo menos um de `startedAt`, `endedAt`, `activeSeconds` (int ≥ 30); ausente não mexe, `null` é inválido | `{ "session": SessionItem, "game": GameDetail }` |
| `session.delete` | `sessionId` (int ≥ 1) | `{ "sessionId": int, "game": GameDetail }` |

O limite de `activeSeconds` no esquema é o de forma (tipo e faixa larga); as regras que dependem dos outros valores (intervalo,
futuro, sobreposição) são do agente e voltam como `-32602` com `data.reason` ou `-32018`. Uma sessão importada pode ter
`activeSeconds` acima de 86 400 em `session.update` (SED-021); por isso o esquema de `update` não tem máximo.

Instantes recebidos com mais precisão que milissegundo são truncados a ms (ADR 0009). Todos os instantes devolvidos são UTC.

## 2. Notificações

Nenhuma nova. Os três métodos **não** enviam `session.changed` nem outra notificação (SED-026, extensão de IPC-050).

## 3. Esquemas

```ts
// SessionItem (acréscimos)
type SessionItem = {
  /* sessionId, gameId, state, startedAt, endedAt, activeSeconds, endReason: como na v0.16.0 */
  source: 'tracked' | 'import' | 'manual' | string   // valor desconhecido passa como está (leitor tolerante, SED-002)
  editedAt: string | null                            // RFC 3339 UTC da última edição; null se nunca editada
}

// GameDetail (acréscimo)
type GameDetail = {
  /* campos da v0.16.0 */
  sessionPlaySeconds: number   // soma de activeSeconds das sessões 'ended' do jogo (SED-034)
}

// StatsGame, StatsPlatform, StatsSummaryResult (acréscimo)
type ManualPart = { manualSeconds: number }   // parte de activeSeconds/totalSeconds com source 'manual' (SED-027)

type SessionWriteResult = { session: SessionItem; game: GameDetail }
type SessionDeleteResult = { sessionId: number; game: GameDetail }
type SessionOverlapData = {
  sessionId: number
  gameId: number
  title: string
  startedAt: string
  endedAt: string | null        // null: a outra sessão está aberta
}
type InvalidParamsReasonData = { reason: EditReason }
```

## 4. Erros

Ordem das checagens no agente (a primeira que falha responde): forma dos params → sessão/jogo existe → sessão aberta → sessão
descartada (só `update`) → patch vazio → sem mudança (responde sucesso sem escrever, SED-008) → regras de tempo na ordem de
`domain.ValidateSessionTimes` → ajuste insuficiente (`includedInTotal`) → sobreposição.

| Código | Quando | `data` | Mensagem ao usuário (SED-036) |
|---|---|---|---|
| `-32602` | forma inválida (IPC-028) | — | a de hoje (DSK-011) |
| `-32602` | `reason = end-not-after-start` | `{reason}` | "O fim precisa ser depois do início." |
| `-32602` | `reason = in-future` | `{reason}` | "O fim não pode estar no futuro." |
| `-32602` | `reason = too-early` | `{reason}` | "O início não pode ser antes de 1980." |
| `-32602` | `reason = span-too-long` | `{reason}` | "Uma sessão pode ter no máximo 24 horas." |
| `-32602` | `reason = active-too-short` | `{reason}` | "O tempo jogado precisa ter pelo menos 1 minuto." |
| `-32602` | `reason = active-exceeds-span` | `{reason}` | "O tempo jogado não pode passar do intervalo entre início e fim." |
| `-32602` | `reason = discarded` | `{reason}` | "Sessões descartadas não podem ser editadas, só excluídas." |
| `-32602` | `reason = exceeds-adjustment` | `{reason}` | "O tempo jogado é maior que a parte do tempo total que não vem de sessões." |
| `-32602` | `reason = empty-patch` | `{reason}` | "Nada para salvar." |
| `-32001` | `session.create` com jogo inexistente | `{gameId}` | a de hoje |
| `-32002` | sessão aberta em `update`/`delete` (SED-022) | `SessionRef` | "Não é possível alterar uma sessão em andamento." |
| `-32017` | `SessionNotFound`: `sessionId` inexistente | `{sessionId}` | "Esta sessão não existe mais." |
| `-32018` | `SessionOverlap`: cruza outra sessão (SED-020) | `SessionOverlapData` | "O horário cruza outra sessão de «title», de «startedAt» a «endedAt»." (hora local; "agora" com `endedAt` nulo) |
| `-32603` | falha interna | — | a de hoje |

A mensagem de `active-too-short` fala em 1 minuto porque a UI trabalha em minutos (SEL-007); o agente aceita a partir de 30 s.

## 5. Tabela de exemplos (domínio e UI usam a mesma)

`T = 2026-11-25T20:00:00Z`. Sessão rastreada ou manual, salvo indicação.

| # | Início | Fim | Tempo jogado | Resultado |
|---|---|---|---|---|
| 1 | T−2h | T−1h | 1h | válido |
| 2 | T−2h | T | 2h | válido (fim = agora; tempo = intervalo) |
| 3 | T−1h | T−1h | 30s | `end-not-after-start` |
| 4 | T−1h | T+1min | 30min | `in-future` |
| 5 | 1979-12-31T23:59:00Z | 1980-01-01T01:00:00Z | 30min | `too-early` |
| 6 | T−25h | T | 2h | `span-too-long` |
| 7 | T−25h | T | 2h, importada | válido (SED-021) |
| 8 | T−1h | T | 29s | `active-too-short` |
| 9 | T−1h | T | 1h01 | `active-exceeds-span` |
| 10 | T−24h | T | 24h | válido (limite incluso) |

Sobreposição (outra sessão rastreada contada `[T−3h, T−2h)`): `[T−2h, T−1h)` válido (encosta); `[T−2h30, T−1h)` → `-32018`. Sessão
aberta desde `T−10min`: `[T−30min, T−10min)` válido; `[T−30min, T−5min)` → `-32018` com `endedAt` nulo. Sessão importada
`[T−300h, T)` e descartada `[T−1h, T−59min40s)`: ignoradas.

## 6. Desktop (bindings Wails)

```go
func (a *App) CreateSession(gameID int64, in SessionCreateInput) (SessionWriteView, error) // 5 s
func (a *App) UpdateSession(sessionID int64, patch json.RawMessage) (SessionWriteView, error) // 5 s
func (a *App) DeleteSession(sessionID int64) (GameDetailView, error)                         // 5 s
```

`SessionItemView` ganha `source` e `editedAt` (`""` quando nulo); `GameDetailView` ganha `sessionPlaySeconds`; as views de
estatística ganham `manualSeconds`. Nenhum evento Wails novo.
