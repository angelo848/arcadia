# Contrato IPC v0.12.0 — delta sobre a v0.11.0

- **Base:** [phase-9-plataformas/contracts/ipc.openrpc.json](../../phase-9-plataformas/contracts/ipc.openrpc.json)
  (v0.11.0), que continua sendo o contrato completo até a tarefa abaixo.
- **Requisitos:** IMG-001..IMG-005 ([requirements.md](../requirements.md)).
- **Natureza:** só acréscimo. Nenhum método, parâmetro, resultado, esquema ou erro da v0.11.0 muda.
- **Tarefa (planner):** gravar o contrato completo `docs/specs/phase-10-2-imagens-ao-vivo/contracts/ipc.openrpc.json`
  v0.12.0 = v0.11.0 + os três pontos abaixo, e apontar o teste de contrato de `internal/ipc` para ele.

## 1. `info`

```json
"version": "0.12.0",
"description": "... Supersedes docs/specs/phase-9-plataformas/contracts/ipc.openrpc.json (v0.11.0). v0.12.0 (phase 10.2, live images): new server-to-client notification media.changed {gameId}, sent by the media queue after a download stores a new image, coalesced per game [IMG-001..IMG-004]; new x-media block. Everything else is unchanged from v0.11.0."
```

Novo bloco em `info` (constante do agente, fora do fio, como `x-tracking`):

```json
"x-media": {
  "description": "Agent constants of the media queue (not part of the wire) [IMG-003].",
  "mediaChangedWindowMs": 500
}
```

`x-limits.clientNotificationBuffer` continua 16 (IPC-017).

## 2. `methods`: notificação nova

Inserida logo depois de `session.changed`:

```json
{
  "name": "media.changed",
  "summary": "Server-to-client notification (no id, no response) sent to every connection when the media queue stores a new image of a game [IMG-001]. Coalesced per game: the first change is sent at once and opens a window of x-media.mediaChangedWindowMs; changes inside the window are sent once when it ends [IMG-003]. Not sent for failed, cancelled, interrupted or duplicate (same hash) downloads, nor by scrape.apply, media.queue, media.cancel, import.apply, game.delete or platform writes [IMG-002, IPC-050]. Delivery is best effort: a full connection queue drops it (IPC-017); clients reload media lists on reconnect [IMG-010].",
  "x-notification": true,
  "paramStructure": "by-name",
  "params": [
    { "name": "gameId", "required": true, "schema": { "type": "integer", "minimum": 1 } }
  ],
  "result": {
    "name": "none",
    "schema": { "type": "null" }
  }
}
```

Exemplo no fio (NDJSON, uma linha):

```json
{"jsonrpc":"2.0","method":"media.changed","params":{"gameId":42}}
```

## 3. Go (`internal/ipc`)

| Contrato | Go |
|---|---|
| `media.changed` | `MethodMediaChanged = "media.changed"` |
| params | `MediaChangedParams{ GameID int64 \`json:"gameId"\` }` |

## 4. Evento Wails (desktop → frontend, fora do contrato IPC) [IMG-005]

| Evento | Payload | Origem | Inválido |
|---|---|---|---|
| `media:changed` | `{ "gameId": number }` (inteiro ≥ 1) | repasse de `media.changed` pelo supervisor | params ausentes, JSON inválido, `gameId` ausente, não inteiro ou < 1 → `Warn "media.changed com parâmetros inválidos"`, nada emitido |

O frontend trata `media:changed` só como pedido de releitura de `mediaKey(gameId)` (IMG-006); o payload não carrega
dados da mídia.
