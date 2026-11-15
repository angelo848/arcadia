# Fase 10.2 — Imagens que chegam com a UI aberta: tarefas

- **Status:** rascunho para aprovação do usuário (2026-11-14).
- **Requisitos:** [requirements.md](requirements.md) (aprovado; IMG-001..IMG-014; o carrossel mantém a imagem em destaque) · **Design:** [design.md](design.md) · **Contrato:** [contracts/ipc-v0.12.0-delta.md](contracts/ipc-v0.12.0-delta.md) e `contracts/ipc.openrpc.json` (v0.12.0, criado na T01).
- **ADRs:** 0014, 0015, 0031.

## Decisões do plano

1. **IMG-009 × design §5:** segue o requisito. Na primeira lista não vazia depois de um `resetKey` novo (ou quando a imagem M em destaque some), `highlightId` recebe o id de `ordered[0]`; a partir daí, imagens novas não mudam o destaque.
2. A janela do contrato (500 ms) é comparada com a constante do agente por um teste de `internal/agent` que lê `x-media.mediaChangedWindowMs` do JSON; `mediaChangedWindow` não é exportada.
3. O evento e o decodificador no desktop ficam em arquivo novo, `cmd/arcadia/media_events.go`.
4. **Prazo de aviso já vencido** entre a descarga e o cálculo do temporizador não pode ser ignorado: `nextDeadline` inclui janelas pendentes já vencidas e `clock.After(d ≤ 0)` dispara na hora. As novas tentativas seguem como hoje.
5. **T01 fica com a sessão principal** (copiar a v0.11.0 e acrescentar o delta por script), não com o spec-writer: é uma cópia mecânica de um JSON grande. O delta continua na pasta como histórico.

## Premissas

- Uma tarefa por commit; test-writer primeiro, implementer depois. Ao fim de cada tarefa: `go build ./...`, `go vet ./...`, `CGO_ENABLED=0 go build ./...`, `go test ./...`, `npm test` e `tsc --noEmit` verdes.
- Nada em `internal/tracker`, `cmd/fake-game`, `internal/store`, `internal/domain` nem nos bindings de `cmd/arcadia` (IMG-013); `main_windows.go` não muda (o `EventsEmit` já é genérico). Nenhuma goroutine nova.

## T01 — Contrato completo `ipc.openrpc.json` v0.12.0
- Requisitos: IMG-004, IMG-013. **Responsável:** sessão principal. Criar `contracts/ipc.openrpc.json` como cópia de `docs/specs/phase-9-plataformas/contracts/ipc.openrpc.json` (v0.11.0) com só: `info.version = "0.12.0"` e `info.description` do delta §1; `info.x-media` com `mediaChangedWindowMs: 500`; método `media.changed` logo depois de `session.changed` (delta §2: `x-notification: true`, `paramStructure: by-name`, `gameId` obrigatório `{type: integer, minimum: 1}`, resultado `none`/`null`). Trocar no `design.md` §1 e §3 a menção "(tarefa)" pelo arquivo criado. Dependência: nenhuma. Bloqueia T02 e o caso de contrato da T03.

## T02 — `media.changed` em `internal/ipc` e teste de contrato na v0.12.0
- Requisitos: IMG-004, IMG-013, IMG-001. **implementer:** `internal/ipc/protocol.go`: comentário do pacote aponta para o contrato da fase 10.2; `MethodMediaChanged = "media.changed"`; `type MediaChangedParams struct{ GameID int64 \`json:"gameId"\` }`.
- **test-writer:** `internal/ipc/contract_test.go` (`contractPath`/`contractRelPath` para a fase 10.2; `openRPC.Info` ganha `x-media`; schema de parâmetro ganha `minimum`; `IPC-041/versão 0.11.0` vira `0.12.0`; `phase6Methods` ganha `ipc.MethodMediaChanged`) e novo `contract_phase10_2_test.go` (`TestMediaChangedContract`): `IMG-004/media.changed é notificação com o nome literal "media.changed"`; `IMG-004/params: só gameId, obrigatório, integer com minimum 1; resultado null`; `IMG-004/MediaChangedParams codifica {"gameId":42} e tem só o campo gameId`; `IMG-004/x-media.mediaChangedWindowMs = 500`; `IMG-004/media.changed vem logo depois de session.changed`; `IMG-004/nenhum handler do agente para media.changed`; `IMG-013/v0.12.0 é a v0.11.0 mais media.changed e x-media, nada mais` (os dois JSON como `map[string]any`; saem `info.version`, `info.description`, `info.x-media` e o método; o resto `reflect.DeepEqual`).
- Dependência: T01.

## T03 — Agrupador de avisos por jogo
- Requisitos: IMG-003, IMG-004. **implementer:** novo `internal/agent/media_changes.go` (estado puro, sem goroutine nem relógio próprio):
```go
const mediaChangedWindow = 500 * time.Millisecond // x-media.mediaChangedWindowMs [IMG-003, IMG-004]
type mediaChangeCoalescer struct{ window time.Duration; until map[int64]time.Time; pending map[int64]bool } // not safe for concurrent use: the queue calls it under q.mu
func newMediaChangeCoalescer(window time.Duration) *mediaChangeCoalescer
func (c *mediaChangeCoalescer) changed(gameID int64, now time.Time) (sendNow bool)
func (c *mediaChangeCoalescer) due(now time.Time) []int64 // sorted by gameID; reopens windows of the sent ones; drops ended idle windows
func (c *mediaChangeCoalescer) next() (time.Time, bool)    // earliest until among pending games; false when none
```
- **test-writer:** `internal/agent/media_changes_test.go` (`TestMediaChangeCoalescer`, instantes relativos a t0): `IMG-003/primeira gravação envia na hora e abre janela de 500 ms`; `IMG-003/gravações em 0, 100 e 200 ms: avisos em 0 e 500 ms, nada em 1000 ms`; `IMG-003/gravação em 1200 ms depois da janela vazia envia na hora`; `IMG-003/G e H em t = 0: um aviso imediato de cada`; `IMG-003/janelas de G e H independentes`; `IMG-003/due antes do fim da janela não envia; due exatamente no fim envia`; `IMG-003/aviso final abre nova janela: gravação 100 ms depois fica pendente`; `IMG-003/next é false sem pendência e é o menor fim entre os pendentes`; `IMG-003/janelas vencidas sem pendência saem dos mapas`; `IMG-004/mediaChangedWindow é igual a x-media.mediaChangedWindowMs do contrato v0.12.0` (lê `../../docs/specs/phase-10-2-imagens-ao-vivo/contracts/ipc.openrpc.json`).
- Dependência: nenhuma para os casos IMG-003; T01 para o IMG-004.

## T04 — A fila avisa depois de gravar, com um só temporizador
- Requisitos: IMG-001, IMG-002, IMG-003. **implementer:** `internal/agent/media_queue.go`:
```go
type mediaChangeSink interface{ MediaChanged(gameID int64) } // nil means none [IMG-001]
func newMediaQueue(st mediaQueueStore, fetch mediaFetcher, clock domain.Clock, log *slog.Logger, sink mediaChangeSink) *mediaQueue
func (q *mediaQueue) flushChanges()                   // under mu: ids := changes.due(now); outside mu: sink.MediaChanged per id
func (q *mediaQueue) nextDeadline() (time.Time, bool) // min(earliest future retry, changes.next() even if already due)
func (q *mediaQueue) nextTimer() <-chan time.Time     // replaces retryTimer: nil when !ok, else clock.After(deadline.Sub(now))
```
Em `run`: com `err == nil && added`, sob `q.mu` `send := q.changes.changed(...)`; fora do lock, `send` chama `sink.MediaChanged` e o caso contrário `q.Notify()`. Sem aviso em `added = false`, erro, cancelamento ou parada. Em `Run`, cada volta faz `flushChanges()`, `dispatch(...)` e `select` com `ctx.Done`, `wake` e `nextTimer()`; pendências somem quando `Run` retorna. `app.go` passa `nil` por enquanto.
- **test-writer:** `media_queue_test.go` (`newTestQueue` passa o sink; `fakeQueueStore.AddMedia` ganha `dup map[string]bool`; `fakeFetcher` ganha portão por URL; novo `spySink`), `scrape_test.go` (só a chamada de `newMediaQueue`). `TestMediaQueueNotices`: `IMG-001/gravação nova envia media.changed(G) uma vez, depois de AddMedia`; `IMG-001/job pendente retomado na subida também avisa`; `IMG-002/download que falha e terá nova tentativa não avisa`; `IMG-002/falha definitiva depois de 3 tentativas não avisa`; `IMG-002/job cancelado durante o download não avisa`; `IMG-002/job interrompido pelo fim do ctx não avisa`; `IMG-002/hash repetido (added = false) não avisa`; `IMG-003/duas gravações de G dentro da janela: aviso imediato e um aviso final depois de Advance(500ms)`; `IMG-003/gravações de G e H juntas: um aviso imediato de cada`; `IMG-003/fila ociosa sem aviso pendente: nextDeadline false`; `IMG-003/aviso pendente quando Run retorna é descartado`; `IMG-003/nova tentativa e aviso pendentes: o temporizador segue o prazo mais cedo e os dois acontecem`. Sincronizar pelo espião e por `BlockUntilWaiters`, sem `time.Sleep`. MID-005 e MID-008 continuam verdes.
- Dependência: T03.

## T05 — Notificador de mídia e ligação no agente
- Requisitos: IMG-001, IMG-002, IMG-012. **implementer:** `internal/agent/notifier.go`: `type mediaNotifier struct{ srv atomic.Pointer[ipc.Server] }`; `set(s *ipc.Server)`; `MediaChanged(gameID int64)` chama `srv.Notify(ipc.MethodMediaChanged, ipc.MediaChangedParams{GameID: gameID})`; no-op antes de `set` e depois de `set(nil)`; nunca bloqueia. `internal/agent/app.go`: `mediaNotes := &mediaNotifier{}` antes da fila e `newMediaQueue(..., mediaNotes)`; `mediaNotes.set(server)` logo depois de `notifier.set(server)`; `instance` ganha o campo; no `Shutdown`, `mediaNotifier.set(nil)` junto de `notifier.set(nil)`, depois de `wg.Wait()` (a ordem `server.Close` → `stopQueue` → `wg.Wait` não muda).
- **test-writer:** `notifier_test.go`: `IMG-001/mediaNotifier sem servidor e depois de set(nil) não faz nada e não bloqueia`. Novo `media_changed_test.go`: com agente real (`startTestAgent` + `tcpListen`, banco com jogo e job `EnqueueMedia` apontando para um `httptest.Server` com PNG e portão, dois clientes): `IMG-001/job retomado na subida grava e os dois clientes recebem media.changed {gameId: G}`; `IMG-001/media.list de G feito ao receber o aviso já traz a imagem`; `IMG-012/gravar mídia não cria sessão, não envia session.changed e session.current continua null`; e com montagem em teste (store real, `scrapeService` com `fakeProvider`, `mediaQueue` com `mediaNotifier`, `ipc.NewServer` com `scrapeHandlers`): `IMG-001/scrape.apply: cada imagem gravada chega como media.changed aos dois clientes`; `IMG-001/media.queue também avisa quando o job grava`; `IMG-002/scrape.apply, media.queue e media.cancel não enviam media.changed antes de um job gravar`; `IMG-002/job cancelado não envia media.changed`. Prazos com `testTimeout`; o `httptest.Server` fecha em `t.Cleanup` e o portão é liberado antes do `Shutdown`.
- Dependências: T02, T04.

## T06 — Repasse `media.changed` → `media:changed` no supervisor
- Requisitos: IMG-005. **implementer:** novo `cmd/arcadia/media_events.go` (`const mediaChangedEvent = "media:changed"`; `type MediaChangedView struct{ GameID int64 \`json:"gameId"\` }`; `decodeMediaChanged(raw json.RawMessage) (MediaChangedView, error)`: ausente/nulo, fração, texto e < 1 falham); `supervisor.go`: `handleNote` ganha `case ipc.MethodMediaChanged` (erro: um `Warn` "media.changed com parâmetros inválidos" e nada emitido; sucesso: `s.cfg.Emit(mediaChangedEvent, v)`), sem mudar estado, sem chamar o agente e sem `Quit`.
- **test-writer:** `supervisor_test.go` (`TestSupervisorMediaChanged`, harness de DSK-010): `IMG-005/media.changed {gameId: 42} emite media:changed com MediaChangedView{GameID: 42}`; `IMG-005/params ausentes, JSON inválido, {}, gameId null, 0, -3, 1.5 e "7": um Warn cada e nada emitido`; `IMG-005/o repasse não chama nenhum método do agente, não chama Quit e o supervisor continua conectado`; `IMG-005/mediaChangedEvent é "media:changed"`. Novo `media_events_test.go`: `IMG-005/decodeMediaChanged aceita inteiro ≥ 1 e recusa o resto` (tabela).
- Dependência: T02.

## T07 — `useMediaEvents`: o aviso relê só a lista de mídia do jogo
- Requisitos: IMG-006, IMG-007, IMG-012. **implementer:** `frontend/src/lib/metadata.ts` (tipo local `WailsRuntime` com `EventsOn`, como em `lib/games.ts`): `const mediaChangedEvent = 'media:changed'`; `export function useMediaEvents(): void`; handler com `data[0] = { gameId }`, `Number.isInteger(gameId) && gameId >= 1` → `qc.invalidateQueries({ queryKey: mediaKey(gameId), exact: true })`, senão ignora.
- **test-writer:** novo `frontend/src/lib/mediaEvents.test.tsx` (`EventsOn` falso; espiões nos bindings): `IMG-006/consulta em uso: media:changed(G) relê MediaList(G) na hora, mesmo com staleTime 60 s`; `IMG-006/em cache fora da tela: fica velha e não é lida até voltar à tela`; `IMG-006/sem cache de G: nada é lido`; `IMG-006/só mediaKey(G) é invalidada; mediaKey(H) e as outras chaves não mudam`; `IMG-006/dado inválido (sem gameId, 0, -1, 1.5, "3", null) é ignorado`; `IMG-007/uma assinatura de media:changed; cancelada ao desmontar`; `IMG-012/o aviso só chama MediaList`.
- Dependência: nenhuma.

## T08 — Reconexão relê as listas de mídia já carregadas
- Requisitos: IMG-010. **implementer:** `frontend/src/lib/games.ts`: em `useReloadOnReconnect`, a lista de chaves invalidadas ganha `['media']`, com `predicate: (q) => q.state.data !== undefined`; sem importar `metadata.ts`.
- **test-writer:** `frontend/src/lib/games.test.tsx` (bloco `useReloadOnReconnect`): `IMG-010/Desconectado → Conectado: ['media', id] com dados é invalidada; em uso é relida`; `IMG-010/consulta de mídia ainda sem resposta não é relida de novo`; `IMG-010/consulta de mídia com dados fora da tela fica velha, sem leitura`; ajustar casos de UI-009 que listem exatamente as chaves invalidadas.
- Dependência: nenhuma.

## T09 — Carrossel com destaque por id de mídia
- Requisitos: IMG-009. **implementer:** `MediaCarousel.tsx`: o estado `index` vira `highlightId: number | undefined`; `index = max(0, ordered.findIndex(m => m.id === highlightId))`; `resetKey` novo leva a `undefined` (AJU-013) e a primeira lista não vazia (ou lista sem M) fixa `highlightId = ordered[0].id` (decisão 1); `onSelect(i)` grava `ordered[i].id`; `failed` zera quando mudam o id em destaque ou o `resetKey`, não a cada lista nova; faixa com `key={resetKey}`. `MediaThumbStrip.tsx`: um ref guarda `{id, index}` do destaque; quando o id não muda e a posição muda, a miniatura em destaque recebe `scrollIntoView({ inline: 'nearest', block: 'nearest' })` sem `focus()` e não na montagem. Props públicas sem mudança.
- **test-writer:** `MediaCarousel.test.tsx`: o caso `AJU-013/trocar items volta à primeira imagem` é reescrito como IMG-009; o de `resetKey` continua. Novos: `IMG-009/imagem nova depois de M: M continua em destaque, "k de n" e faixa com a lista nova`; `IMG-009/imagem nova antes de M: M continua em destaque e k passa a ser a nova posição`; `IMG-009/sem clique, uma capa que chega antes da imagem mostrada não troca o destaque`; `IMG-009/a miniatura de M com o foco do teclado continua com o foco depois da lista nova`; `IMG-009/M mudou de posição: scrollIntoView com inline nearest, sem mover o foco`; `IMG-009/lista vazia que ganha uma imagem: ela aparece em destaque`; `IMG-009/M removida da lista nova: a primeira imagem fica em destaque`; `IMG-009/trocar resetKey continua levando à primeira imagem`; `IMG-009/erro no destaque não volta a ser tentado só porque a lista cresceu; volta ao trocar o destaque`. `lib/bigbox.test.tsx` continua verde.
- Dependência: nenhuma.

## T10 — `App` assina os avisos: cartão, carrossel e cenário do bug
- Requisitos: IMG-007, IMG-008, IMG-011, IMG-012. **implementer:** `frontend/src/App.tsx`: `useMediaEvents()` logo depois de `useSessionEvents()`; `MetadataDialog.tsx` não muda.
- **test-writer:** `App.test.tsx`: `IMG-007/App montado tem exatamente um handler de media:changed; desmontar o remove`; `IMG-008/grade com G em foco e H visível sem imagem: media:changed(H) põe a capa no cartão de H; foco, marcação, activeElement e scrollTop iguais`; `IMG-008/cartão de H com substituta passa para a capa quando ela chega`; `IMG-008/media:changed(H) não relê ListGames nem MediaList de outras linhas`; `IMG-008/H = G: o cartão em foco troca a imagem e continua em foco`; `IMG-008/lista e Big Box: o mesmo cartão atualiza sem mudar o foco`; `IMG-011/MetadataDialog aberto, ScrapeApply responde queued = 3, chegam três media:changed(G) com a lista crescendo: carrossel e cartão de G mostram as imagens a cada evento, G em foco, diálogo aberto`; `IMG-011/aplicar continua invalidando mediaKey(G)`. `App.invariants.test.tsx`: `IMG-012/eventos media:changed e reconexão chamam só leituras; nenhum LaunchGame, LaunchGameUntracked, Pause/Resume nem escrita; jogo em foco e sessão exibida não mudam`.
- Dependências: T07, T08, T09.

## T11 — Verificação do contrato, rastreio e revisão do diff
- Requisitos: IMG-004, IMG-013. **Responsável:** sessão principal (e o reviewer). `go test ./internal/ipc/... ./internal/agent/... ./cmd/arcadia/... -v` com os casos IMG-004 e IMG-013; `spec-trace.ps1` nas fases 10.2, 3, 10 e 10.1; `git diff --stat` da fase só com `internal/ipc/protocol.go`, `internal/agent/{media_changes,media_queue,notifier,app}.go`, `cmd/arcadia/{media_events,supervisor}.go` e testes, e `docs/specs/phase-9-plataformas/contracts/ipc.openrpc.json` intacto; revisão de concorrência (nenhum `go func` novo; `sink.MediaChanged` sempre fora de `q.mu`; `nextTimer` nil com a fila ociosa).
- Dependência: T01..T10.

## T12 — Conferência manual ponta a ponta e documentação
- Requisitos: IMG-014. **Responsável:** sessão principal com o usuário. Builds: `go build ./...`, `go vet ./...`, `CGO_ENABLED=0 go build ./...`, `go test ./...`, `npm test`, `tsc --noEmit`, `npm run build`, `wails build`.
- **Roteiro (IMG-014):** (1) jogo sem capa em foco, "Buscar metadados" no IGDB e aplicar: sem sair do jogo, capa no cartão e imagens no carrossel conforme cada download termina, jogo em foco; (2) o mesmo com SteamGridDB; (3) durante os downloads, navegar pelas miniaturas: o carrossel não pula para a primeira; (4) fechar o diálogo no meio dos downloads: as imagens continuam chegando; (5) sair na bandeja durante os downloads e reabrir: a reconexão mostra o que foi gravado; (6) `agent.log` sem `Warn` de `media.changed`.
- **Documentação:** `docs/ARCHITECTURE.md` (IPC v0.12.0, janela de 500 ms, evento `media:changed`, `useMediaEvents`, releitura na reconexão, ADR 0031), `docs/ROADMAP.md` (10.2 concluída; limitação 2 da fase 10 resolvida), `CLAUDE.md` (spec atual), `docs/adr/README.md` (0031).
- Dependência: T11.

## Ordem

- **Bloco A (paralelo):** T01 (sessão principal), T03, T07, T08, T09.
- **Bloco B:** Go: T02 (após T01) → T04 (após T03) → T05 (após T02 e T04); T06 (após T02) em paralelo com T04 e T05. Frontend: T10 (após T07, T08 e T09). As duas cadeias rodam em paralelo.
- **Depois:** T11 → T12.
- **Arquivos compartilhados (não paralelos):** `media_queue.go`, `media_queue_test.go`, `scrape_test.go`: só T04; `app.go` e `notifier.go`: só T05 (a T04 mexe em `app.go` só na chamada de `newMediaQueue` com `nil`); `contract_test.go` e `protocol.go`: só T02; `supervisor.go` e `supervisor_test.go`: só T06; `lib/metadata.ts`: só T07; `lib/games.ts` e `games.test.tsx`: só T08; `MediaCarousel*` e `MediaThumbStrip.tsx`: só T09; `App.tsx`, `App.test.tsx` e `App.invariants.test.tsx`: só T10.

## Riscos de concorrência e recursos

- **Goroutines:** nenhuma nova. A decisão "envia já" é do job (já no `wg` de `Run`); a descarga dos avisos finais é do laço de `Run`, esperado por `instance.wg`. O `httptest.Server` dos testes fecha em `t.Cleanup` e o portão é liberado antes do `Shutdown`.
- **Locks:** `changes` só é lido e escrito sob `q.mu`; `sink.MediaChanged` e `q.Notify()` sempre fora de `q.mu`; `Server.Notify` nunca bloqueia e descarta com a fila cheia (IPC-017); dois avisos de G quase juntos são inofensivos (a UI só invalida).
- **Temporizador único:** `nextTimer` é nil sem nova tentativa futura nem aviso pendente; prazo de aviso vencido dispara na hora (decisão 4); um job que marca pendência entre a descarga e o `select` deixa sinal em `wake` (capacidade 1). Nos testes, esperas antigas ficam no FakeClock: conferir `nextDeadline` quando usar `BlockUntilWaiters`.
- **Shutdown:** `Drain` → `server.Close()` → `stopQueue()` → `wg.Wait()` → `notifier.set(nil)` e `mediaNotifier.set(nil)` → tracker → store. Avisos depois do `Close` são no-op.
- **UI:** `useMediaEvents` tem uma assinatura por montagem do `App`, cancelada no cleanup; a invalidação é por chave exata e só relê o que está na tela.

## Rastreabilidade

| Requisito | Tarefas |
|---|---|
| IMG-001 | T02, T04, T05 |
| IMG-002 | T04, T05 |
| IMG-003 | T03, T04 |
| IMG-004 | T01, T02, T03, T11 |
| IMG-005 | T06 |
| IMG-006 | T07 |
| IMG-007 | T07, T10 |
| IMG-008 | T10 |
| IMG-009 | T09 |
| IMG-010 | T08 |
| IMG-011 | T10 |
| IMG-012 | T05, T07, T10 |
| IMG-013 | T01, T02, T11 |
| IMG-014 | T12 (manual) |

Os 14 requisitos têm tarefa. Nenhum ADR novo além do 0031.
