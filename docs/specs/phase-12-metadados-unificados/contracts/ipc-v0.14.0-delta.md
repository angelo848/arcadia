# Contrato IPC v0.14.0 — delta sobre a v0.13.0

- **Base:** [phase-11-importacao-v2/contracts/ipc.openrpc.json](../../phase-11-importacao-v2/contracts/ipc.openrpc.json)
  (v0.13.0). O contrato completo v0.14.0 já está em [ipc.openrpc.json](ipc.openrpc.json) (criado na T01).
- **Requisitos:** MET-040, MET-052..MET-058, MET-064, MET-070..MET-072 ([requirements.md](../requirements.md)).
- **Natureza:** só acréscimo. Nenhum método, parâmetro obrigatório, resultado, esquema existente ou erro da v0.13.0 muda;
  `scrape.search`, `scrape.apply` e `media.queue` só ganham a palavra "obsoleto" na descrição.
- **Tarefa (T01, feita):** o contrato completo `docs/specs/phase-12-metadados-unificados/contracts/ipc.openrpc.json`
  v0.14.0 = v0.13.0 + os pontos abaixo foi gravado; resta (T11) apontar o teste de contrato de `internal/ipc` (`contract_test.go`) para ele; o teste
  que compara com a versão anterior ganha um irmão da fase 12 que confere "só acréscimo" (mais as palavras de obsolescência).

## 1. `info`

```json
"version": "0.14.0",
"description": "... Supersedes docs/specs/phase-11-importacao-v2/contracts/ipc.openrpc.json (v0.13.0). v0.14.0 (phase 12, unified metadata): new methods metadata.search, metadata.apply, metadata.batchStart, metadata.batchStatus, metadata.batchResults, metadata.batchApply, metadata.batchCancel and metadata.batchDiscard [MET-001, MET-030, MET-052..MET-058, MET-064]; new server-to-client notification metadata.batchChanged {batchId, state, done, total, failed} [MET-054]; ImportCandidate gains the optional field searchName [MET-040]; new errors BatchNotFound (-32014), BatchBusy (-32015) and BatchWrongState (-32016); new x-metadata block. scrape.search, scrape.apply and media.queue are obsolete (still served, same behaviour; the image limit of MET-022 now applies to them). Everything else is unchanged from v0.13.0."
```

Novo bloco em `info` (constantes do agente e de `domain`, fora do fio, como `x-import` e `x-media`):

```json
"x-metadata": {
  "description": "Agent and domain constants of the unified metadata search (not part of the wire) [MET-006..MET-011, MET-053..MET-057].",
  "maxSearchCandidates": 10,
  "maxRowCandidates": 5,
  "matchAuto": 0.85,
  "matchMinSimilarity": 0.80,
  "matchMargin": 0.05,
  "matchDoubt": 0.60,
  "matchAlternative": 0.40,
  "matchCap": 0.79,
  "searchTimeoutMs": 20000,
  "gameSearchTimeoutMs": 15000,
  "requestIntervalMs": { "igdb": 334, "steamgriddb": 500, "steamstore": 2000 },
  "batchWorkers": 3,
  "maxBatchGames": 1000,
  "maxBatchChoices": 1000,
  "defaultResultsLimit": 100,
  "maxResultsLimit": 200,
  "maxResultBytes": 921600,
  "progressWindowMs": 500,
  "providerCooldownMs": [30000, 60000, 120000],
  "providerMaxFailures": 3,
  "maxStatusWarnings": 10,
  "maxFailedTitles": 10
}
```

`x-limits.maxMessageBytes` continua 1048576; `maxResultBytes` deixa 127 KiB de folga para o envelope (como `x-import`).
`x-media.mediaChangedWindowMs` continua 500 e `x-metadata.progressWindowMs` usa o mesmo valor por decisão (a UI reage igual).

## 2. Esquemas novos (`components.schemas`)

```json
"MetadataRef": {
  "type": "object",
  "required": ["provider", "providerId"],
  "additionalProperties": false,
  "properties": {
    "provider": { "type": "string", "enum": ["igdb", "steamgriddb", "steamstore"] },
    "providerId": { "type": "string", "pattern": "^[0-9]{1,20}$" }
  }
},
"MetadataCandidate": {
  "type": "object",
  "required": ["refs", "title", "year", "platform", "score"],
  "additionalProperties": false,
  "properties": {
    "refs": { "type": "array", "minItems": 1, "maxItems": 3, "items": { "$ref": "#/components/schemas/MetadataRef" } },
    "title": { "type": "string", "maxLength": 200 },
    "year": { "type": "integer", "minimum": 0, "description": "0 = unknown." },
    "platform": { "type": "string", "maxLength": 100, "description": "Name of the library platform when it matches; otherwise the canonical name of the first known platform of the candidate; \"\" when unknown [MET-008]." },
    "score": { "type": "number", "minimum": 0, "maximum": 1 },
    "thumbUrl": { "type": "string", "maxLength": 300, "description": "https URL of a small cover on images.igdb.com, cdn.cloudflare.steamstatic.com or cdn2.steamgriddb.com; absent otherwise [MET-068]." }
  }
},
"MetadataConfidence": { "type": "string", "enum": ["match", "doubt", "none"] },
"MetadataSearchResult": {
  "type": "object",
  "required": ["candidates", "confidence", "warnings"],
  "additionalProperties": false,
  "properties": {
    "candidates": { "type": "array", "maxItems": 10, "items": { "$ref": "#/components/schemas/MetadataCandidate" } },
    "confidence": { "$ref": "#/components/schemas/MetadataConfidence" },
    "warnings": { "type": "array", "items": { "type": "string", "maxLength": 200 } }
  }
},
"MetadataApplyResult": {
  "type": "object",
  "required": ["detail", "queued", "skippedByLimit"],
  "additionalProperties": false,
  "properties": {
    "detail": { "$ref": "#/components/schemas/GameDetail" },
    "queued": { "type": "integer", "minimum": 0 },
    "skippedByLimit": { "type": "integer", "minimum": 0 }
  }
},
"BatchState": { "type": "string", "enum": ["searching", "ready", "cancelled", "applying", "done"] },
"BatchResult": {
  "type": "object",
  "required": ["applied", "imagesQueued", "imagesSkippedByLimit", "failedTitles"],
  "additionalProperties": false,
  "properties": {
    "applied": { "type": "integer", "minimum": 0 },
    "imagesQueued": { "type": "integer", "minimum": 0 },
    "imagesSkippedByLimit": { "type": "integer", "minimum": 0 },
    "failedTitles": { "type": "array", "maxItems": 10, "items": { "type": "string", "maxLength": 200 } }
  }
},
"BatchStatus": {
  "type": "object",
  "required": ["batchId", "state", "total", "done", "failed", "warnings"],
  "additionalProperties": false,
  "properties": {
    "batchId": { "type": "integer", "minimum": 1 },
    "state": { "$ref": "#/components/schemas/BatchState" },
    "total": { "type": "integer", "minimum": 0 },
    "done": { "type": "integer", "minimum": 0 },
    "failed": { "type": "integer", "minimum": 0 },
    "warnings": { "type": "array", "maxItems": 10, "items": { "type": "string", "maxLength": 200 } },
    "result": { "$ref": "#/components/schemas/BatchResult", "description": "Only in state done [MET-065, MET-066]." }
  }
},
"BatchRow": {
  "type": "object",
  "required": ["gameId", "title", "platform", "confidence", "selected", "candidates"],
  "additionalProperties": false,
  "properties": {
    "gameId": { "type": "integer", "minimum": 1 },
    "title": { "type": "string", "maxLength": 200 },
    "platform": { "type": "string", "maxLength": 100 },
    "confidence": { "$ref": "#/components/schemas/MetadataConfidence" },
    "selected": { "type": "integer", "minimum": -1, "description": "Index in candidates of the chosen item; -1 when confidence is none [MET-061]." },
    "candidates": { "type": "array", "maxItems": 5, "items": { "$ref": "#/components/schemas/MetadataCandidate" } }
  }
},
"BatchChoice": {
  "type": "object",
  "required": ["gameId", "candidate"],
  "additionalProperties": false,
  "properties": {
    "gameId": { "type": "integer", "minimum": 1 },
    "candidate": { "type": "integer", "minimum": 0, "description": "Index in the candidates of that game's row [MET-064]." }
  }
}
```

`ImportCandidate` (esquema da v0.13.0) ganha uma propriedade opcional (fora de `required`; `additionalProperties` continua `false`):

```json
"searchName": {
  "type": "string",
  "maxLength": 200,
  "description": "File source only: the name proposed for the metadata search, from the file name or the nearest non-generic parent folder [MET-040, contracts/match-rules.md §5.2]. Absent for the other sources. import.apply accepts and ignores it."
}
```

## 3. `methods` novos

Inseridos depois de `scrape.apply`. Todos por nome (`paramStructure: by-name`).

| Método | Parâmetros | Resultado | Erros |
|---|---|---|---|
| `metadata.search` | `title` (string, 1..200 runas, obrigatório), `platform` (string ≤ 100, opcional), `year` (integer, 0 ou 1900..2100, opcional) | `MetadataSearchResult` | `InvalidParams`, `ProviderNoKey` (-32010), `ProviderUnavailable` (-32011), `InternalError` |
| `metadata.apply` | `gameId` (integer ≥ 1), `refs` (`MetadataRef[]`, 1..3, sem repetir `provider`), `replace` (boolean, opcional, padrão `false`) | `MetadataApplyResult` | `InvalidParams`, `GameNotFound` (-32001), `ProviderNoKey`, `ProviderUnavailable`, `InternalError` |
| `metadata.batchStart` | `gameIds` (integer[] ≥ 1, 1..1.000, distintos) | `{ "batchId": integer ≥ 1, "total": integer ≥ 1 }` | `InvalidParams`, `GameNotFound`, `ProviderNoKey`, `BatchBusy` (-32015), `InternalError` |
| `metadata.batchStatus` | — | `{ "batch": BatchStatus \| null }` | `InternalError` |
| `metadata.batchResults` | `batchId` (integer ≥ 1), `offset` (integer ≥ 0, padrão 0), `limit` (integer 1..200, padrão 100) | `{ "rows": BatchRow[], "total": integer, "next": integer \| null }` | `InvalidParams`, `BatchNotFound` (-32014), `BatchWrongState` (-32016) |
| `metadata.batchApply` | `batchId`, `choices` (`BatchChoice[]`, 1..1.000, `gameId` distintos) | `{ "accepted": integer ≥ 1 }` | `InvalidParams`, `BatchNotFound`, `BatchWrongState` |
| `metadata.batchCancel` | `batchId` | `{ "state": BatchState }` | `InvalidParams`, `BatchNotFound` |
| `metadata.batchDiscard` | `batchId` | `{ "discarded": true }` | `InvalidParams`, `BatchNotFound`, `BatchWrongState` |

Resumos (`summary`):

- `metadata.search`: "Searches every applicable provider at once and returns one scored list with the confidence of the best item [MET-001..MET-009]."
- `metadata.apply`: "Applies the text (priority igdb, steamstore; only empty fields unless replace) and queues the images of one unified candidate, within the per-kind image limit [MET-030..MET-033, MET-021]. No notification of its own; images arrive by media.changed [IMG-001, IPC-050]."
- `metadata.batchStart`: "Starts the background search over games already written (the titles stored are the ones searched). Returns at once; progress comes by metadata.batchChanged [MET-052, MET-053]."
- `metadata.batchStatus`: "The current batch or null. Safety net for dropped notifications and reconnects [MET-055]."
- `metadata.batchResults`: "One page of the rows of a ready or cancelled batch; fewer than limit rows when the page would pass maxResultBytes [MET-057]."
- `metadata.batchApply`: "Accepts the user's choices and applies them in the background; returns at once [MET-064, MET-065]."
- `metadata.batchCancel`: "Cancels a search (keeps the rows already found) or an application (keeps what was applied); no-op in the other states [MET-056, MET-066]."
- `metadata.batchDiscard`: "Frees a ready, cancelled or done batch [MET-058]."

Regras de validação (todas `-32602`, antes de qualquer leitura ou rede, MET-071):

| Parâmetro | Regra |
|---|---|
| `title` | aparado, 1..200 runas, e o termo de busca (`domain.SearchTerm`) não vazio |
| `platform` | ≤ 100 runas; vazio = sem plataforma |
| `year` | 0 ou 1900..2100 |
| `refs` | 1..3 itens, `provider` único entre eles, `providerId` `^[0-9]{1,20}$` |
| `gameIds` | 1..1.000, todos ≥ 1, sem repetição |
| `batchId` | ≥ 1 |
| `offset` / `limit` | ≥ 0 / 1..200 |
| `choices` | 1..1.000, `gameId` ≥ 1 sem repetição, `candidate` ≥ 0 e dentro da lista da linha, `gameId` presente na busca |

## 4. Notificação nova

Inserida depois de `media.changed`:

```json
{
  "name": "metadata.batchChanged",
  "summary": "Server-to-client notification (no id, no response) sent to every connection while a metadata batch runs in the background: at once when its state changes by itself (searching -> ready, applying -> done), and, for counter-only changes, at most once per x-metadata.progressWindowMs (the first at once, the following folded into one at the end of the window) [MET-054]. Carries only the state and counters, never game data (ADR 0031). Not sent because of metadata.batchStart, batchApply, batchCancel or batchDiscard [IPC-050]. Delivery is best effort: a full connection queue drops it (IPC-017); clients read metadata.batchStatus on reconnect [MET-055].",
  "x-notification": true,
  "paramStructure": "by-name",
  "params": [
    { "name": "batchId", "required": true, "schema": { "type": "integer", "minimum": 1 } },
    { "name": "state", "required": true, "schema": { "$ref": "#/components/schemas/BatchState" } },
    { "name": "done", "required": true, "schema": { "type": "integer", "minimum": 0 } },
    { "name": "total", "required": true, "schema": { "type": "integer", "minimum": 0 } },
    { "name": "failed", "required": true, "schema": { "type": "integer", "minimum": 0 } }
  ],
  "result": { "name": "none", "schema": { "type": "null" } }
}
```

Exemplo no fio (NDJSON, uma linha):

```json
{"jsonrpc":"2.0","method":"metadata.batchChanged","params":{"batchId":1763217600000,"state":"searching","done":12,"total":80,"failed":1}}
```

## 5. Erros novos (`components.errors`)

```json
"BatchNotFound": { "code": -32014, "message": "Metadata batch not found",
  "description": "No batch, or the batchId is not the current one (discarded, replaced or lost with an agent restart) [MET-058]." },
"BatchBusy": { "code": -32015, "message": "Metadata batch in progress",
  "data": { "type": "object", "required": ["state"], "additionalProperties": false,
            "properties": { "state": { "$ref": "#/components/schemas/BatchState" } } },
  "description": "metadata.batchStart while a batch is searching, ready, cancelled or applying [MET-052]." },
"BatchWrongState": { "code": -32016, "message": "Wrong batch state",
  "data": { "type": "object", "required": ["state"], "additionalProperties": false,
            "properties": { "state": { "$ref": "#/components/schemas/BatchState" } } },
  "description": "A batch method that does not apply to the current state (batch-state-machine.md §2)." }
```

`ProviderNoKey` (`-32010`) e `ProviderUnavailable` (`-32011`) continuam como estão, com `data.provider`. `metadata.search` e
`metadata.batchStart` os usam como em MET-002 e MET-052.

## 6. Go (`internal/ipc`)

| Contrato | Go |
|---|---|
| métodos | `MethodMetadataSearch = "metadata.search"`, `MethodMetadataApply = "metadata.apply"`, `MethodMetadataBatchStart = "metadata.batchStart"`, `MethodMetadataBatchStatus = "metadata.batchStatus"`, `MethodMetadataBatchResults = "metadata.batchResults"`, `MethodMetadataBatchApply = "metadata.batchApply"`, `MethodMetadataBatchCancel = "metadata.batchCancel"`, `MethodMetadataBatchDiscard = "metadata.batchDiscard"` |
| notificação | `MethodMetadataBatchChanged = "metadata.batchChanged"`, `MetadataBatchChangedParams{ BatchID int64 \`json:"batchId"\`; State string \`json:"state"\`; Done, Total, Failed int \`json:"done"...\` }` |
| esquemas | `MetadataRef{Provider, ProviderID string}`, `MetadataCandidate{Refs []MetadataRef; Title string; Year int; Platform string; Score float64; ThumbURL string \`json:"thumbUrl,omitempty"\`}`, `MetadataSearchParams{Title, Platform string \`json:"platform,omitempty"\`; Year int \`json:"year,omitempty"\`}`, `MetadataSearchResult{Candidates []MetadataCandidate; Confidence string; Warnings []string}`, `MetadataApplyParams{GameID int64; Refs []MetadataRef; Replace bool \`json:"replace,omitempty"\`}`, `MetadataApplyResult{Detail GameDetail; Queued, SkippedByLimit int}`, `BatchStartParams{GameIDs []int64}`, `BatchStartResult{BatchID int64; Total int}`, `BatchStatus{...; Result *BatchResult \`json:"result,omitempty"\`}`, `BatchStatusResult{Batch *BatchStatus \`json:"batch"\`}` (ponteiro nulo serializa `null`), `BatchResultsParams{BatchID int64; Offset, Limit int}`, `BatchRow`, `BatchResultsResult{Rows []BatchRow; Total int; Next *int \`json:"next"\`}`, `BatchChoice`, `BatchApplyParams`, `BatchApplyResult{Accepted int}`, `BatchIDParams{BatchID int64}`, `BatchCancelResult{State string}`, `BatchDiscardResult{Discarded bool}` |
| `ImportCandidate.searchName` | `SearchName string \`json:"searchName,omitempty"\`` (omitido nas outras fontes: o JSON de Steam, Epic, LaunchBox e do candidato de pasta fica byte a byte igual ao da v0.13.0) |
| erros | `CodeBatchNotFound = -32014`, `CodeBatchBusy = -32015`, `CodeBatchWrongState = -32016` com as mensagens do contrato |
| `x-metadata` | constantes de `domain` (`MatchAuto`, `MatchMinSimilarity`, `MatchMargin`, `MatchDoubt`, `MatchAlternative`, `MatchCap`) e do agente; o teste de contrato confere todas contra o JSON |

`MetadataCandidate.Year` usa `0` para desconhecido (sem `omitempty`; o campo é obrigatório no esquema).

## 7. Binding do Wails e evento (desktop → frontend, fora do contrato IPC) [MET-072]

| Binding | Assinatura | Chama |
|---|---|---|
| `MetadataSearch` | `(title, platform string, year int) (MetadataSearchView, error)` | `metadata.search` |
| `MetadataApply` | `(gameID int64, refs []MetadataRefView, replace bool) (MetadataApplyView, error)` | `metadata.apply` |
| `MetadataBatchStart` | `(gameIDs []int64) (MetadataBatchStartView, error)` | `metadata.batchStart` |
| `MetadataBatchStatus` | `() (MetadataBatchStatusView, error)`; `Batch *MetadataBatchView` (nulo = sem busca) | `metadata.batchStatus` |
| `MetadataBatchResults` | `(batchID int64, offset, limit int) (MetadataBatchResultsView, error)` | `metadata.batchResults` |
| `MetadataBatchApply` | `(batchID int64, choices []MetadataChoiceView) (int, error)` | `metadata.batchApply` |
| `MetadataBatchCancel` | `(batchID int64) (string, error)` | `metadata.batchCancel` |
| `MetadataBatchDiscard` | `(batchID int64) error` | `metadata.batchDiscard` |
| `ImportCandidateView` | + `SearchName string` (JSON `searchName`), nos dois sentidos | — |

| Evento | Payload | Origem | Inválido |
|---|---|---|---|
| `metadata:batch` | `{ "batchId": number, "state": string, "done": number, "total": number, "failed": number }` (inteiros ≥ 0, `batchId` ≥ 1, `state` um dos cinco) | repasse de `metadata.batchChanged` pelo supervisor | params ausentes, JSON inválido, campo ausente, não inteiro, fora da faixa ou `state` desconhecido → `Warn "metadata.batchChanged com parâmetros inválidos"`, nada emitido |

O frontend trata `metadata:batch` como atualização dos contadores do indicador e, quando o `state` muda, como pedido de
releitura de `MetadataBatchStatus` (MET-055); o evento não carrega dados de jogo.
