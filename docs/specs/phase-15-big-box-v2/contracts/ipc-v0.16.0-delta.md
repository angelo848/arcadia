# IPC v0.16.0 — acréscimo sobre a v0.15.0 (fases 13 e 14)

Requisitos: TTB-007, TTB-008, TTB-020..TTB-023, TTB-029, TTB-030, TTB-033..TTB-040, ATB-014. · alterado em 2026-11-22 (aprovação,
resposta 1): `source` ganha `igdb`; `x-timetobeat` ganha a reserva. Formal (OpenRPC, só o acréscimo):
[ipc-v0.16.0-delta.openrpc.json](ipc-v0.16.0-delta.openrpc.json). O JSON completo `contracts/ipc.openrpc.json` v0.16.0 é gerado por
uma tarefa a partir da v0.15.0 de [phase-13](../../phase-13-modo-console/contracts/ipc.openrpc.json), dos acréscimos de
[phase-14](../../phase-14-edicao-manual/requirements.md) (`media.add`, `media.remove`, `game.setPlayTime`) e deste delta. **Só
acréscimos**: nenhum método, campo ou valor existente muda.

## 1. Métodos novos

| Método | Parâmetros | Resultado | Bloqueia? |
|---|---|---|---|
| `timeToBeat.get` | `gameId` (int ≥ 1), `fetch` (bool, padrão `false`) | `TimeToBeat` | não: responde do cache; a busca, se houver, vai para a fila |
| `timeToBeat.refresh` | `gameId` | `TimeToBeat` | sim, até `x-timetobeat.syncTimeoutMs` |
| `timeToBeat.search` | `gameId`, `term` (string opcional, 1..200 depois do corte de espaços) | `{ "candidates": TimeToBeatCandidate[] }` | sim, até `syncTimeoutMs` |
| `timeToBeat.choose` | `gameId`, `externalId` (string; `""` = "nenhum destes") | `TimeToBeat` | não (sem rede) |
| `timeToBeat.getSettings` | `{}` | `TimeToBeatSettings` | não |
| `timeToBeat.setSettings` | `enabled` (bool, opcional), `afterMetadata` (bool, opcional); pelo menos um | `TimeToBeatSettings` | não |

## 2. Notificação nova

| Notificação | `params` | Quando |
|---|---|---|
| `timeToBeat.changed` | `{ "gameId": int }` | depois de gravar uma busca (fila, `refresh`) ou uma escolha (`choose`) do jogo; nunca em falha, nunca depois de `BeginShutdown`; fila de conexão cheia descarta (IPC-017) |

## 3. Esquemas

```ts
type TimeToBeat = {
  gameId: number
  state: 'none' | 'found' | 'doubt' | 'not-found'   // 'none' = nada gravado
  source: 'hltb' | 'igdb' | ''                      // '' com state 'none'; 'igdb' só com state 'found' (reserva, TTB-033)
  externalId: string                                // id na fonte; '' fora de 'found'
  externalTitle: string                             // nome na fonte; '' fora de 'found'
  mainSeconds: number | null                        // História principal
  mainExtraSeconds: number | null                   // História + extras
  completionistSeconds: number | null               // 100%
  fetchedAt: string | null                          // RFC 3339 UTC; null com 'none'
  confirmed: boolean                                // escolhido pelo usuário (timeToBeat.choose); sempre false com 'igdb'
  stale: boolean                                    // vencido (TTB-006); valores ainda devolvidos
  pending: boolean                                  // há busca deste jogo na fila ou em curso
  unavailable: boolean                              // as duas fontes indisponíveis (HowLongToBeat em recuo e IGDB em recuo ou sem chave, TTB-011)
  candidates: TimeToBeatCandidate[]                 // só com state 'doubt'; [] nos outros
}
type TimeToBeatCandidate = {                        // sempre do HowLongToBeat (TTB-035, TTB-040)
  externalId: string
  title: string
  year: number | null
  platforms: string[]
  mainSeconds: number | null
  mainExtraSeconds: number | null
  completionistSeconds: number | null
  score: number                                     // 0..1000 (milésimos, domain.ScoreGroup)
}
type TimeToBeatSettings = { enabled: boolean; afterMetadata: boolean }
```

Valores em segundos são inteiros ≥ 1 ou `null` (nunca 0).

## 4. Erros

| Código | Quando | `data` |
|---|---|---|
| `-32602` | `gameId` < 1 ou não inteiro; `term` vazio depois do corte ou > 200; `externalId` fora dos candidatos guardados; `setSettings` sem campos; tipos errados | — |
| `-32001` | jogo inexistente (`get`, `refresh`, `search`, `choose`) | — |
| `-32011` | `refresh`: nada gravado porque o HowLongToBeat falhou e a reserva do IGDB não achou, falhou ou não tem chave ([rules §10.1](hltb-rules.md)); `search`: HowLongToBeat falhou; os dois: serviço desligado | `{ "provider": "hltb", "reason": "unavailable" \| "disabled" }` |

`timeToBeat.get` **nunca** devolve `-32011`: falha e recuo aparecem como `unavailable: true` (TTB-013).

## 5. Bloco `x-timetobeat`

```json
{
  "x-timetobeat": {
    "source": "hltb",
    "fallbackSource": "igdb",
    "fallbackMapping": { "mainSeconds": "hastily", "mainExtraSeconds": "normally", "completionistSeconds": "completely" },
    "requestIntervalMs": 2000,
    "requestTimeoutMs": 10000,
    "syncTimeoutMs": 25000,
    "foundTtlDays": 90,
    "fallbackFoundTtlDays": 30,
    "doubtTtlDays": 30,
    "notFoundTtlDays": 30,
    "backoffMinutes": [1, 5, 30, 60],
    "batchQueueMax": 1000,
    "maxCandidates": 10,
    "termMaxLength": 200,
    "settingKeys": { "enabled": "timeToBeat.enabled", "afterMetadata": "timeToBeat.afterMetadata" }
  }
}
```

## 6. Desktop (bindings Wails e eventos)

```go
// cmd/arcadia
type TimeToBeatView struct {
    GameID               int64                     `json:"gameId"`
    State                string                    `json:"state"`
    Source               string                    `json:"source"`
    ExternalID           string                    `json:"externalId"`
    ExternalTitle        string                    `json:"externalTitle"`
    MainSeconds          *int64                    `json:"mainSeconds"`
    MainExtraSeconds     *int64                    `json:"mainExtraSeconds"`
    CompletionistSeconds *int64                    `json:"completionistSeconds"`
    FetchedAt            string                    `json:"fetchedAt"` // "" when none
    Confirmed            bool                      `json:"confirmed"`
    Stale                bool                      `json:"stale"`
    Pending              bool                      `json:"pending"`
    Unavailable          bool                      `json:"unavailable"`
    Candidates           []TimeToBeatCandidateView `json:"candidates"`
}
type TimeToBeatCandidateView struct { /* the fields of TimeToBeatCandidate */ }
type TimeToBeatSettingsView struct {
    Enabled       bool `json:"enabled"`
    AfterMetadata bool `json:"afterMetadata"`
}

func (a *App) TimeToBeatGet(gameID int64, fetch bool) (TimeToBeatView, error)            // 2 s
func (a *App) TimeToBeatRefresh(gameID int64) (TimeToBeatView, error)                    // 30 s
func (a *App) TimeToBeatSearch(gameID int64, term string) ([]TimeToBeatCandidateView, error) // 30 s; "" = term of TTB-003
func (a *App) TimeToBeatChoose(gameID int64, externalID string) (TimeToBeatView, error)  // 2 s
func (a *App) TimeToBeatSettings() (TimeToBeatSettingsView, error)                       // 2 s
func (a *App) TimeToBeatSetSettings(enabled, afterMetadata bool) (TimeToBeatSettingsView, error) // 2 s
func (a *App) TakeBigBoxRequest() bool // ATB-012; never calls the agent
```

Os prazos são os do `ctx` de cada chamada ao agente (os de `refresh`/`search` ficam acima de `syncTimeoutMs`). Erros ao usuário
seguem UI-008; `-32011` vira "Não foi possível consultar o HowLongToBeat" (ou "Consulta ao HowLongToBeat desligada" com
`reason = disabled`). A UI mostra "Fonte: HowLongToBeat" ou "Fonte: IGDB" conforme `source` (TTB-039).

| Evento Wails | Dados | Origem |
|---|---|---|
| `timeToBeat:changed` | `{ "gameId": int }` | repasse de `timeToBeat.changed` pelo supervisor (mesma validação de `media.changed`) |
| `ui:bigBoxRequested` | nenhum | segunda instância com `--bigbox` (ATB-009) |
