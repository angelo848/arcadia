# Contrato IPC v0.15.0 — delta sobre a v0.14.0

- **Base:** [phase-12-metadados-unificados/contracts/ipc.openrpc.json](../../phase-12-metadados-unificados/contracts/ipc.openrpc.json)
  (v0.14.0). O contrato completo `docs/specs/phase-13-modo-console/contracts/ipc.openrpc.json` v0.15.0 = v0.14.0 + os pontos abaixo
  está gravado em [ipc.openrpc.json](ipc.openrpc.json) (tarefa T01, como nas fases 11 e 12); o teste de contrato de `internal/ipc` passa a apontar para ele e um teste irmão
  confere "só acréscimo" em relação à v0.14.0 (CON-026).
- **Requisitos:** CON-017, CON-019, CON-026, CON-027, CON-031, VID-005..VID-008, SSC-001.
- **Natureza:** só acréscimo. Nenhum método, parâmetro obrigatório, resultado, erro ou esquema existente perde nada: enums ganham
  valores, entram métodos, uma notificação e dois blocos `x-*`; dois `summary` ganham uma frase. Nenhum código de erro novo.
  `GameDetail` não muda (sem número de discos, resposta 7 da aprovação).

## 1. `info`

```json
"version": "0.15.0",
"description": "... Supersedes docs/specs/phase-12-metadados-unificados/contracts/ipc.openrpc.json (v0.14.0). v0.15.0 (phase 13, console mode): new server-to-client notification game.closedByHotkey {gameId}, sent after the gamepad combo Select+B closed a game [CON-019]; new methods video.queue, video.queueMissing, video.cancelPending and video.status [VID-005..VID-008]; provider enums gain screenscraper and screenscraperdev [SSC-001]; MediaItem.kind gains video [VID-001]; SessionItem.endReason lists client-no-launch (correction) and closed-by-hotkey [CON-017]; game.launch and game.launchUntracked are refused with SessionActive while a close by the combo is in progress [CON-031]; new x-console and x-video blocks. Everything else is unchanged from v0.14.0."
```

Blocos novos em `info` (constantes do agente, fora do fio, como `x-metadata`):

```json
"x-console": {
  "description": "Agent constants of the close-game gamepad combo (not part of the wire) [CON-001, CON-004, CON-010, CON-015, CON-030].",
  "closeCombo": "Back+B",
  "closeHoldMs": 1000,
  "closeDebounceMs": 300,
  "closeWaitMs": 3000,
  "closeDetachedPollMs": 250,
  "closeHandlerTimeoutMs": 6000,
  "refusedCue": [[300, 100], [200, 200]]
},
"x-video": {
  "description": "Agent constants of the ScreenScraper video lane (not part of the wire) [VID-002, VID-005, VID-009, VID-012..VID-016].",
  "kindLimit": 1,
  "mediaType": "video-normalized",
  "maxVideoBytes": 20971520,
  "laneWorkers": 1,
  "requestIntervalMs": 1500,
  "quotaPauseMs": 3600000,
  "maxQueueGames": 1000,
  "softname": "arcadia"
}
```

`x-tracking`, `x-media` e `x-metadata` não mudam (`x-metadata.requestIntervalMs` continua sem `screenscraper`: o ScreenScraper
não entra na busca de metadados, SSC-005).

## 2. Esquemas alterados (só acréscimo)

| Esquema / lugar | Acréscimo |
|---|---|
| `provider` de `provider.keyStatus`, `provider.setKey`, `provider.deleteKey` e `ProviderKeyStatus.provider` | enum `["igdb", "steamgriddb", "screenscraper", "screenscraperdev"]` |
| `provider.setKey.secret` | descrição: "required (1..512) for igdb, screenscraper and screenscraperdev" (o esquema continua opcional; a regra é de validação, `-32602`) |
| `MediaItem.kind` | enum + `"video"`; descrição de `thumbPath`: "empty for video" |
| `SessionItem.endReason` | enum `["exited", "agent-shutdown", "recovered", "client-no-launch", "closed-by-hotkey"]` (ou `null`) |
| `summary` de `game.launch` e de `game.launchUntracked` | acrescenta: "Also refused with SessionActive while a close by the gamepad combo Select+B is in progress (up to closeWaitMs plus the kill); for an untracked game being closed, sessionId is 0 [CON-031]." (`SessionActive` já está nos `errors` dos dois). No de `game.launchUntracked`, "no Job Object" vira "in a Job Object kept by the agent only so the gamepad combo can close it; closing the agent never kills it [CON-011]" |

`media.queue.kinds` **não** ganha `video` (obsoleto desde a v0.14.0; vídeo só por `video.*`). `MetadataRef.provider` não muda.

## 3. Esquemas novos (`components.schemas`)

```json
"VideoSkip": {
  "type": "object",
  "required": ["gameId", "reason"],
  "additionalProperties": false,
  "properties": {
    "gameId": { "type": "integer", "minimum": 1 },
    "reason": { "type": "string", "enum": ["has-video", "platform-unmapped"] }
  }
},
"VideoQueueResult": {
  "type": "object",
  "required": ["queued", "skipped"],
  "additionalProperties": false,
  "properties": {
    "queued": { "type": "integer", "minimum": 0 },
    "skipped": { "type": "array", "maxItems": 1000, "items": { "$ref": "#/components/schemas/VideoSkip" } }
  }
},
"VideoQueueMissingResult": {
  "type": "object",
  "required": ["queued", "skipped"],
  "additionalProperties": false,
  "properties": {
    "queued": { "type": "integer", "minimum": 0 },
    "skipped": { "type": "integer", "minimum": 0, "description": "Games of the scope left out: platform without a ScreenScraper system, or video already present or queued." }
  }
},
"VideoLane": { "type": "string", "enum": ["active", "paused-quota", "paused-credentials", "no-credentials"] },
"VideoStatus": {
  "type": "object",
  "required": ["pending", "running", "failed", "lane"],
  "additionalProperties": false,
  "properties": {
    "pending": { "type": "integer", "minimum": 0 },
    "running": { "type": "integer", "minimum": 0, "maximum": 1 },
    "failed": { "type": "integer", "minimum": 0 },
    "lane": { "$ref": "#/components/schemas/VideoLane" },
    "pausedUntil": { "type": "string", "format": "date-time", "description": "Only in paused-quota." },
    "requestsToday": { "type": "integer", "minimum": 0, "description": "Last value reported by ScreenScraper; absent before the first answer." },
    "maxRequestsPerDay": { "type": "integer", "minimum": 0 }
  }
}
```

## 4. `methods` novos

Inseridos depois de `metadata.batchDiscard`. Todos por nome (`paramStructure: by-name`).

| Método | Parâmetros | Resultado | Erros |
|---|---|---|---|
| `video.queue` | `gameIds` (integer[] ≥ 1, 1..1.000, distintos) | `VideoQueueResult` | `InvalidParams`, `GameNotFound` (-32001), `ProviderNoKey` (-32010, `data.provider` = `screenscraperdev` ou `screenscraper`), `InternalError` |
| `video.queueMissing` | `platformId` (integer ≥ 1, opcional; ausente = biblioteca toda) | `VideoQueueMissingResult` | `InvalidParams`, `PlatformNotFound` (-32001), `ProviderNoKey`, `InternalError` |
| `video.cancelPending` | — | `{ "cancelled": integer ≥ 0 }` | `InternalError` |
| `video.status` | — | `{ "status": VideoStatus }` | `InternalError` |

Resumos (`summary`):

- `video.queue`: "Queues one ScreenScraper video lookup per game (media_job kind video, source screenscraper:lookup), within the limit of one video per game; games already with a video or a queued lookup, or whose platform has no ScreenScraper system, are listed in skipped [VID-002, VID-003, VID-005]. No notification; the video arrives by media.changed."
- `video.queueMissing`: "Same as video.queue for every game of the platform (or the library) without a video [VID-006]."
- `video.cancelPending`: "Cancels every pending video lookup and interrupts the running one; image jobs are untouched [VID-007]."
- `video.status`: "Counters and lane state of the video queue, with the last quota reported by ScreenScraper [VID-008]."

Validação (todas `-32602`, antes de qualquer leitura): `gameIds` 1..1.000, todos ≥ 1, sem repetição; `platformId` ≥ 1 quando
presente. Credenciais são conferidas depois da validação e antes de qualquer escrita (`-32010`).

## 5. Notificação nova

Inserida depois de `metadata.batchChanged`:

```json
{
  "name": "game.closedByHotkey",
  "summary": "Server-to-client notification (no id, no response) sent to every connection after the gamepad combo Select+B closed a game: for a tracked session, after its end was written and session.changed was queued; for a game launched untracked, on its own (there is no session) [CON-019]. Carries only the game id (ADR 0031). Not sent when nothing was closed. Delivery is best effort: a full connection queue drops it (IPC-017).",
  "x-notification": true,
  "paramStructure": "by-name",
  "params": [
    { "name": "gameId", "required": true, "schema": { "type": "integer", "minimum": 1 } }
  ],
  "result": { "name": "none", "schema": { "type": "null" } }
}
```

Exemplo no fio (NDJSON, uma linha):

```json
{"jsonrpc":"2.0","method":"game.closedByHotkey","params":{"gameId":42}}
```

## 6. Go (`internal/ipc`)

| Contrato | Go |
|---|---|
| métodos | `MethodVideoQueue = "video.queue"`, `MethodVideoQueueMissing = "video.queueMissing"`, `MethodVideoCancelPending = "video.cancelPending"`, `MethodVideoStatus = "video.status"` |
| notificação | `MethodGameClosedByHotkey = "game.closedByHotkey"`, `GameClosedByHotkeyParams{ GameID int64 \`json:"gameId"\` }` |
| esquemas | `VideoQueueParams{GameIDs []int64 \`json:"gameIds"\`}`, `VideoSkip{GameID int64; Reason string}`, `VideoQueueResult{Queued int; Skipped []VideoSkip}` (lista nunca nula), `VideoQueueMissingParams{PlatformID int64 \`json:"platformId,omitempty"\`}`, `VideoQueueMissingResult{Queued, Skipped int}`, `VideoCancelResult{Cancelled int}`, `VideoStatus{Pending, Running, Failed int; Lane string; PausedUntil *time.Time \`json:"pausedUntil,omitempty"\`; RequestsToday, MaxRequestsPerDay *int \`json:",omitempty"\`}`, `VideoStatusResult{Status VideoStatus}` |
| provedores | constantes `ProviderScreenScraper = "screenscraper"`, `ProviderScreenScraperDev = "screenscraperdev"` |
| `x-console`, `x-video` | constantes do agente/`domain`/`scrapers`; o teste de contrato confere todas contra o JSON |

## 7. Bindings do Wails e eventos (desktop → frontend, fora do contrato IPC) [CON-020, CON-023, CON-027]

| Binding | Assinatura | Chama |
|---|---|---|
| `BringToFront` | `() error` (só erro de programação; recusa do Windows não é erro) | — (local, `winapi.ForceForeground`) |
| `VideoQueue` | `(gameIDs []int64) (VideoQueueView, error)`; `VideoQueueView{Queued int; Skipped []VideoSkipView}` | `video.queue` |
| `VideoQueueMissing` | `(platformID int64) (VideoQueueMissingView, error)` (`0` = biblioteca toda) | `video.queueMissing` |
| `VideoCancelPending` | `() (int, error)` | `video.cancelPending` |
| `VideoStatus` | `() (VideoStatusView, error)` (`PausedUntil` RFC 3339 ou `""`; contagens de cota `-1` quando ausentes) | `video.status` |
| `ProviderKeyStatus`, `ProviderSetKey`, `ProviderDeleteKey` | inalterados; aceitam `screenscraper` e `screenscraperdev` | `provider.*` |
| `MediaView.Kind` | pode ser `video` | — |

| Evento | Payload | Origem | Inválido |
|---|---|---|---|
| `game:closedByHotkey` | `{ "gameId": number }` (inteiro ≥ 1) | repasse de `game.closedByHotkey` pelo supervisor | params ausentes, JSON inválido, campo ausente, não inteiro ou < 1 → `Warn "game.closedByHotkey com parâmetros inválidos"`, nada emitido |

O frontend trata `game:closedByHotkey` só quando o Big Box está ativo (CON-022); a janela normal o ignora.
