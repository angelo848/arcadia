# Fase 10.2 — Imagens que chegam com a UI aberta: design

- **Requisitos:** [requirements.md](requirements.md) (rascunho, 2026-11-14) · **Tarefas:** `tasks.md` (planner)
- **Contrato:** [contracts/ipc-v0.12.0-delta.md](contracts/ipc-v0.12.0-delta.md) (acréscimo sobre a v0.11.0; o JSON
  completo é uma tarefa).
- **ADRs:** 0014, 0015; proposto 0031 (avisos de invalidação para trabalho em segundo plano).

## 0. Estado atual (conferido no código)

| Ponto | Onde | O que acontece hoje |
|---|---|---|
| Fila | `internal/agent/media_queue.go` | `Run` é a goroutine dona do laço; cada job roda numa goroutine esperada por `wg`. `run` chama `Fetch` e `AddMedia` e grava o estado do job. Nada avisa ninguém. O laço só arma temporizador quando há nova tentativa pendente (`retryTimer`). |
| Ordem de criação | `internal/agent/app.go` | A fila é criada **antes** do `ipc.Server`; o tracker resolve isso com `sessionNotifier` (ponteiro atômico, `set(server)` depois de `NewServer`, `set(nil)` no encerramento). No `Shutdown`, `server.Close()` vem antes de `stopQueue()` e `wg.Wait()`. |
| Notificação | `internal/ipc/server.go` | `Server.Notify` codifica uma vez e enfileira em cada conexão sem bloquear; fila cheia (16) descarta com `Warn`. No-op antes de `Serve` e depois de `Close`. |
| Cliente | `internal/ipc/client.go` | `Notifications()` com buffer `NotificationBuffer` (16); cheio descarta (IPC-017). |
| Repasse | `cmd/arcadia/supervisor.go` | `handleNote`: `agent.heartbeat`, `session.changed` (→ `session:changed`), `agent.shutdown`; o resto cai em `Debug "notificação ignorada"`. |
| UI | `frontend/src/lib/metadata.ts` | `mediaKey(id) = ['media', id]`; `useMedia` (painel, `staleTime` 0) e `useCardImages` (cartões e "Jogados recentemente", `staleTime` 60 s) dividem a chave. |
| Diálogo | `MetadataDialog.tsx:62-64` | invalida `libraryKeys.game`, `['library','games']` e `mediaKey(gameId)` logo depois de `ScrapeApply`: a releitura chega antes dos downloads. |
| Reconexão | `lib/games.ts` `useReloadOnReconnect` | relê `['library','games']`, `['library','game']`, `['library','sessions']` e a sessão; **não** relê `['media']`. |
| Carrossel | `MediaCarousel.tsx` | `useEffect(() => setIndex(0), [resetKey, items])`: qualquer lista nova (referência nova) volta à primeira imagem; `failed` também zera com `items`. A faixa (`MediaThumbStrip`, `key={resetKey}`) tem botões com `key={it.id}`. |

## 1. Arquivos e responsabilidades

| Arquivo | Mudança | IDs |
|---|---|---|
| `internal/ipc/protocol.go` | `MethodMediaChanged`, `MediaChangedParams` | IMG-001, IMG-004 |
| `internal/agent/media_changes.go` (novo) | `mediaChangeCoalescer` (estado puro) e `mediaChangedWindow` | IMG-003 |
| `internal/agent/media_queue.go` | aviso depois de `AddMedia` com `added`; descarga no laço; temporizador único (nova tentativa ou aviso) | IMG-001..IMG-003 |
| `internal/agent/notifier.go` | `mediaNotifier` (ponte para o servidor, como `sessionNotifier`) | IMG-001 |
| `internal/agent/app.go` | cria o notificador, passa à fila, `set(server)` / `set(nil)` | IMG-001 |
| `cmd/arcadia/supervisor.go` (+ constante em `games.go` ou arquivo próprio) | caso `media.changed` → `media:changed` | IMG-005 |
| `frontend/src/lib/metadata.ts` | `useMediaEvents` | IMG-006, IMG-007 |
| `frontend/src/App.tsx` | monta `useMediaEvents()` ao lado de `useSessionEvents()` | IMG-007, IMG-008, IMG-011 |
| `frontend/src/lib/games.ts` | `useReloadOnReconnect` inclui `['media']` | IMG-010 |
| `frontend/src/components/MediaCarousel.tsx` | destaque por id de mídia | IMG-009 |
| `docs/specs/phase-10-2-imagens-ao-vivo/contracts/ipc.openrpc.json` (criado na T01) | contrato completo v0.12.0 | IMG-004 |

Nada em `internal/tracker`, `cmd/fake-game`, `internal/store`, migrações nem bindings (IMG-013). `MetadataDialog.tsx`
não muda (a invalidação ao aplicar fica, idempotente).

## 2. Aviso no agente [IMG-001..IMG-003]

### Tipos

```go
// internal/ipc/protocol.go
// MethodMediaChanged is the server-to-client notification that the media list of a game changed (v0.12.0) [IMG-001].
MethodMediaChanged = "media.changed"

// MediaChangedParams are the params of the media.changed notification [IMG-001, IMG-004].
type MediaChangedParams struct {
	GameID int64 `json:"gameId"`
}

// internal/agent/notifier.go
// mediaNotifier bridges the media queue to the IPC server (the queue is built before the server) [IMG-001].
// Before set, and after set(nil), MediaChanged is a no-op; it never blocks.
type mediaNotifier struct{ srv atomic.Pointer[ipc.Server] }
func (n *mediaNotifier) set(s *ipc.Server)
func (n *mediaNotifier) MediaChanged(gameID int64) // srv.Notify(ipc.MethodMediaChanged, ipc.MediaChangedParams{GameID: gameID})

// internal/agent/media_queue.go
// mediaChangeSink receives the coalesced notices of the queue [IMG-001]. nil means none (tests of other IDs).
type mediaChangeSink interface{ MediaChanged(gameID int64) }
func newMediaQueue(st mediaQueueStore, fetch mediaFetcher, clock domain.Clock, log *slog.Logger, sink mediaChangeSink) *mediaQueue
```

### Agrupador (estado puro, sem goroutine, sem relógio próprio)

```go
// internal/agent/media_changes.go
const mediaChangedWindow = 500 * time.Millisecond // x-media.mediaChangedWindowMs [IMG-003, IMG-004]

// mediaChangeCoalescer decides when media.changed is sent for each game [IMG-003]: the first change sends at
// once and opens a window; changes inside the window are sent once when it ends, which opens a new window.
// It is not safe for concurrent use: the queue calls it under q.mu with q.clock.Now().
type mediaChangeCoalescer struct {
	window  time.Duration
	until   map[int64]time.Time // game -> end of its open window
	pending map[int64]bool      // games changed inside their open window
}
func newMediaChangeCoalescer(window time.Duration) *mediaChangeCoalescer
// changed records a change of gameID at now; it reports whether the notice is sent now.
func (c *mediaChangeCoalescer) changed(gameID int64, now time.Time) (sendNow bool)
// due returns the pending games whose window ended at or before now (each opens a new window) and drops the
// ended windows with nothing pending, so the maps do not grow with idle games.
func (c *mediaChangeCoalescer) due(now time.Time) []int64
// next is the earliest window end among the pending games; false when nothing is pending.
func (c *mediaChangeCoalescer) next() (time.Time, bool)
```

Tabela de transições por jogo G (o estado é o par `until[G]`, `pending[G]`):

| Estado de G | Evento | Novo estado | Efeito |
|---|---|---|---|
| sem janela (ou janela vencida) | gravação em `now` | janela até `now+500ms`, sem pendência | envia já |
| janela aberta, sem pendência | gravação em `now < until` | janela igual, pendente | nada (o laço é acordado) |
| janela aberta, pendente | gravação em `now < until` | igual | nada |
| janela aberta, pendente | `due(now ≥ until)` | janela até `now+500ms`, sem pendência | envia (aviso final) |
| janela aberta, sem pendência | `due(now ≥ until)` | sem janela | nada |
| qualquer | fila para (`Run` retorna) | — | pendência descartada |

### Integração na fila

- Em `run`, depois de `AddMedia` sem erro e com `added = true`: sob `q.mu`, `send := q.changes.changed(job.GameID, now)`;
  fora do lock, `if send { q.sink.MediaChanged(job.GameID) } else { q.Notify() }` (o `Notify` acorda o laço para armar o
  temporizador do aviso final). Com `added = false`, erro, cancelamento ou parada: nada (IMG-002).
- O laço de `Run`, a cada volta: `q.flushChanges()` (sob `q.mu`, `ids := q.changes.due(now)`; fora do lock, um
  `MediaChanged` por id) e depois `dispatch`, como hoje. `retryTimer` vira `nextTimer`: o menor entre a próxima nova
  tentativa futura e `q.changes.next()`; `nil` quando não há nenhum dos dois (a fila ociosa continua sem esperar no
  relógio). Um prazo já vencido é tratado pela descarga da volta, não por temporizador.
- Corrida coberta: se um job marca pendência entre a descarga e o cálculo do temporizador, o `q.Notify()` dele deixa um
  sinal em `wake` (capacidade 1) e o laço dá mais uma volta.
- O envio acontece fora de `q.mu`; `Server.Notify` nunca bloqueia e é seguro de qualquer goroutine. Dois envios do mesmo
  jogo quase juntos (um imediato de um job e um final do laço) são inofensivos: a UI só invalida (idempotente).
- **Goroutines:** nenhuma nova. A decisão é do job (que já tem dono, `wg`) e a descarga é do laço (dono, `Run`).
- **Encerramento:** `server.Close()` vem antes de `stopQueue()`; avisos depois disso são no-op. Pendências somem quando
  `Run` retorna (IMG-003). `mediaNotifier.set(nil)` junto de `notifier.set(nil)`. Um aviso enviado entre `Drain` e
  `Close` chega a uma UI que já está fechando por `agent.shutdown`: inofensivo.

## 3. Contrato [IMG-004]

Só acréscimo: notificação `media.changed` e o bloco `x-media`; versão 0.12.0. Ver
[contracts/ipc-v0.12.0-delta.md](contracts/ipc-v0.12.0-delta.md). Compatibilidade: um desktop v0.11.0 trataria a
notificação como desconhecida (`Debug "notificação ignorada"`); como agente e UI saem do mesmo executável (ADR 0014),
isso não acontece na prática. O teste de contrato (`internal/ipc/contract_test.go`) ganha a constante na tabela de
métodos e a checagem de `x-media.mediaChangedWindowMs` contra a constante do agente (exposta para o teste por um helper
ou duplicada com comentário; o planner decide).

## 4. Repasse no desktop [IMG-005]

```go
// cmd/arcadia
// mediaChangedEvent is the Wails event that forwards media.changed [IMG-005].
const mediaChangedEvent = "media:changed"

// MediaChangedView is the payload of media:changed.
type MediaChangedView struct {
	GameID int64 `json:"gameId"`
}

// decodeMediaChanged validates media.changed params: present, valid JSON, gameId an integer >= 1 [IMG-005].
func decodeMediaChanged(raw json.RawMessage) (MediaChangedView, error)
```

Em `handleNote`, `case ipc.MethodMediaChanged`: decodifica; erro → `Warn("media.changed com parâmetros inválidos",
"error", ...)`; sucesso → `s.cfg.Emit(mediaChangedEvent, v)`. "`gameId` ausente" se distingue de 0 decodificando para
`*int64` (ou `json.Number`); fracionário e texto falham no `Unmarshal`.

## 5. Frontend [IMG-006..IMG-011]

```ts
// lib/metadata.ts
const mediaChangedEvent = 'media:changed'
// useMediaEvents reloads the image list of the game named by each media:changed event; it touches no other query
// and never the focused game [IMG-006, IMG-007, IMG-012].
export function useMediaEvents(): void
```

- Assinatura com `window.runtime.EventsOn` (tipo local, como em `lib/games.ts`); dado `{ gameId }` com inteiro ≥ 1 →
  `void qc.invalidateQueries({ queryKey: mediaKey(gameId), exact: true })`; senão ignora. O padrão do React Query
  (`refetchType: 'active'`) dá exatamente IMG-006: em uso relê, em cache fora da tela fica velha, sem cache não faz nada.
  A invalidação vence o `staleTime` de 60 s de `useCardImages`.
- Fica em `lib/metadata.ts` (dona de `mediaKey` e de `fetchMedia`), não em `lib/games.ts`, para não criar dependência de
  `games.ts` em `metadata.ts`; o `App` monta `useMediaEvents()` logo depois de `useSessionEvents()`.
- Cartão, "Jogados recentemente" e carrossel leem a mesma chave: um evento atualiza os três (IMG-008, IMG-011).
  `CoverImage` já zera `failed` quando `src` muda.
- **Reconexão [IMG-010]:** `useReloadOnReconnect` acrescenta `['media']` à lista de chaves invalidadas com
  `predicate: (q) => q.state.data !== undefined` (mesmo padrão das outras).
- **Carrossel [IMG-009]:** o estado passa de `index` para `highlightId: number | undefined` (id da mídia em destaque).
  `index = max(0, ordered.findIndex(m => m.id === highlightId))`; `resetKey` novo → `highlightId = undefined` (primeira
  imagem, AJU-013); lista nova do mesmo jogo não zera o destaque. `onSelect(i)` grava `ordered[i].id`. `failed` zera
  quando muda o id em destaque ou o `resetKey`, não a cada lista nova. A faixa continua com `key={resetKey}` e botões
  `key={it.id}`, então o botão focado é o mesmo elemento e mantém o foco; `tabIndex 0` acompanha o destaque. Se o destaque
  mudar de posição, a miniatura dele é rolada para a parte visível (`scrollIntoView({ inline: 'nearest' })`) sem mover o
  foco.
- **MetadataDialog:** sem mudança; a invalidação ao aplicar continua (IMG-011).

## 6. Rede de segurança: por que a reconexão

| Opção | Custo | Cobre | Decisão |
|---|---|---|---|
| Reler `['media']` com dados ao reconectar | 1 linha no hook existente; sem IPC novo | avisos perdidos com a conexão caída e agente reiniciado (o caso em que mais se perde) | **escolhida** (IMG-010) |
| Aviso de "fila terminou" | notificação nova, estado global da fila no agente | o fim, mas não as perdas no meio | não: mais contrato para o mesmo efeito do aviso final por jogo |
| Releitura periódica depois de `scrape.apply` enquanto houver jobs | precisa ler jobs (não há método) ou chutar um tempo | só o diálogo, não `media.queue` nem jobs retomados | não: pede método novo ou timer arbitrário na UI |

Com a conexão de pé, o descarte só ocorre com 16 mensagens acumuladas numa fila; o agrupamento limita cada jogo a 2
avisos por segundo e a fila baixa 2 imagens por vez, então fica como limitação 1 (o que se perde volta na próxima leitura).

## 7. Testes e riscos

- **Go:** `media_changes_test.go` (tabela de IMG-003, incluindo jogos independentes e limpeza de janelas vencidas);
  `media_queue_test.go` (notificador espião: um aviso por gravação nova, nenhum por falha, cancelamento, parada ou hash
  repetido; aviso final com relógio fake; fila ociosa sem temporizador); teste do agente com dois clientes reais
  (`media.changed` chega a ambos, depois da gravação, e `scrape.apply` em si não notifica; nenhuma sessão criada,
  IMG-012); `contract_test.go` (IMG-004); `supervisor_test.go` (IMG-005).
- **Frontend:** `metadata.test.ts(x)` (IMG-006, IMG-007), `games.test.tsx` (IMG-010), `MediaCarousel.test.tsx` (IMG-009),
  `App.test.tsx` (IMG-008, IMG-011), `App.invariants.test.tsx` (IMG-012). Os testes de AJU-013 que esperam volta à
  primeira imagem com "imagens novas" do mesmo jogo são ajustados para IMG-009.
- **Riscos:**
  - *Temporizador do laço:* juntar nova tentativa e aviso num `nextTimer` não pode deixar esperas penduradas; os testes
    da fila com relógio fake (MID-005) precisam continuar passando, incluindo "fila ociosa sem esperar no relógio".
  - *Teste com relógio fake:* o job decide o envio com `clock.Now()`; o teste avança o relógio e espera o aviso sem
    `time.Sleep` (sincroniza pelo espião).
  - *Muitos cartões do mesmo jogo:* não existe (um cartão por jogo); a invalidação é por chave exata.
  - *Big Box:* usa os mesmos cartões e o mesmo carrossel; nada específico.

## 8. ADR proposto

0031 — Notificações de mudança em segundo plano como avisos de invalidação (ver requirements.md, "ADR proposto").
