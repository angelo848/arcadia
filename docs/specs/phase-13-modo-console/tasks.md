# Fase 13 — Modo console: tarefas

- **Status:** gerado pelo planner em 2026-11-15; a spec foi aprovada e o usuário pediu a execução contínua das tarefas.
- **Requisitos:** [requirements.md](requirements.md) (CON-001..CON-033, BBX-001..BBX-030, VID-001..VID-026, SSC-001..SSC-005; um requisito removido) · **Design:** [design.md](design.md) · **Contratos:** [contracts/ipc-v0.15.0-delta.md](contracts/ipc-v0.15.0-delta.md), [contracts/session-close-state-machine.md](contracts/session-close-state-machine.md), [contracts/bigbox-faixa.md](contracts/bigbox-faixa.md), [contracts/screenscraper-rules.md](contracts/screenscraper-rules.md), [contracts/screenscraper-api-notes.md](contracts/screenscraper-api-notes.md) e `contracts/ipc.openrpc.json` (v0.15.0, criado na T01).
- **ADRs:** 0005, 0016, 0017, 0018, 0019 (em parte substituído), 0025, 0029, 0030, 0031, 0033, 0034, 0035, 0036, 0037 (0035 a 0037 já escritos).

## Resumo

Três trilhas independentes, mais uma base comum e o fechamento.

- **Base (T01..T05):** contrato JSON v0.15.0, conferência da API do ScreenScraper, tipos IPC, testes de contrato e o tipo de mídia `video` no frontend.
- **Trilha A, atalho Select+B (T06..T16):** `input`, `feedback`/`winapi`, `tracker` (só o tracker-specialist), roteador e ligação do agente, desktop (`game:closedByHotkey`, `BringToFront`) e `SessionsList`.
- **Trilha B, vídeo do ScreenScraper (T17..T30):** migração 9, store, `media.FetchVideo`, cliente e resolvedor, raia de vídeo na fila, `video.*`, bindings, `serveMedia` e as janelas de provedores e do painel.
- **Trilha C, Big Box novo com temas (T31..T43):** frontend puro (`frontend/src/bigbox/`), tokens, reducer, Faixa, Detalhes e `App.tsx`.
- **Fechamento (T44, T45):** verificação final e tarefa manual.

## Dúvidas, lacunas e achados do planejamento

Nenhuma bloqueia a execução. As marcadas "spec-writer" pedem ajuste de texto depois, e a sessão principal decide se faz.

1. **Roteador e shutdown (spec-writer).** O roteador espera até 6 s por `CloseByHotkey`, mas `Router.Close()` roda antes de `Tracker.Close()` no `Shutdown`, e é o `Stop` do tracker que responde `ErrShuttingDown` (CON-033). Sem tratamento, um fechamento em espera seguraria o `Shutdown` por até 6 s. A T13 resolve: o contexto de cada chamada do roteador deriva de um contexto cancelado em `Close()`. A espera é abandonada, o tracker ainda responde no buffer de 1 e nada toca. Registrar em CON-021/CON-033.
2. **`CloseUndone` precisa rearmar os temporizadores (spec-writer).** Em `Closing`, o `Heartbeat` e o `Poll` que disparam são ignorados ou não rearmam. Se a morte falhar (CON-013), a cadeia de heartbeat ficaria quebrada. A T09 faz `CloseUndone()` devolver `armHeartbeat` e `armPoll` (gerações novas). A tabela da máquina de estados não diz isso.
3. **URL gravada do vídeo (spec-writer).** A fila grava `job.SourceURL` (`screenscraper:lookup`), mas [rules §5](contracts/screenscraper-rules.md) manda gravar `RedactURL(url da mídia)` em `media.source_url`. A T19 acrescenta `media.Stored.SourceURL` (opcional) e a T24 usa esse valor quando não vazio.
4. **Teste de CON-023.** A spec diz "teste de `cmd/arcadia` com `winapi` falsa (ordem das chamadas)". O plano põe a ordem das chamadas num seam de `internal/winapi` (`ForegroundAPI`, T15) e deixa o teste de `cmd/arcadia` provar só que `BringToFront` usa a plataforma, não chama o agente e que recusa do Windows não é erro. Os dois cobrem CON-023.
5. **Testes e fakes que quebram de propósito** (o test-writer reescreve, com a anotação "alterado em 2026-11-15 (fase 13)"):
   - `Job` ganha `RequestClose` e `Kill`, e `Launcher.LaunchDetached` muda de assinatura. Quebram `internal/tracker/fakes_test.go` e os fakes de launcher dos testes do agente (`launch_uri_test.go`, `tracker_wiring_test.go`, `app_test.go`, `shutdown_test.go`, `components_test.go`).
   - `pause_test.go` (renomeia para o roteador de gatilhos).
   - `media_queue_test.go` (o `mediaFetcher` recebe o job).
   - `contract_test.go`, `contract_phase12_test.go` e `contract_phase11_test.go` (o `contractPath` passa para a v0.15.0; os da fase 12 passam a ler a v0.14.0 por `contractV014Path`).
   - `App.test.tsx` (bloco "Big Box sobre o layout novo"), `bigbox.test.tsx`, `GameCardBigBox.test.tsx` e os trechos de `GameDetailsPanel.test.tsx` que afirmam `data-bigbox-*`.
   - Qualquer teste com o literal `8` de `SchemaVersion` (conferir com `rg "SchemaVersion|\b8\b" internal/store`).
6. **`video.status.lane = no-credentials`** é calculado na hora da consulta (credencial ausente), além do que o despacho marcou. Assim a UI não depende de haver job pendente. A regra de [rules §9](contracts/screenscraper-rules.md) vale igual.
7. **`failed` de `video.status`** não conta os jobs cancelados (`last_error = 'cancelado'`), senão "N sem vídeo" mentiria depois de "Cancelar buscas pendentes". `video.cancelPending` devolve pendentes mais em andamento cancelados.
8. **Ordem das validações de `video.queue`:** parâmetros (`-32602`), jogos (`-32001`), credenciais (`-32010`), escrita. A spec não ordena `-32001` e `-32010`.
9. **Hooks e trilhas em paralelo.** `gotest.ps1` roda `go test ./...` no Stop do implementer e do tracker-specialist, e o test-writer deixa testes vermelhos de propósito. Trilhas rodando ao mesmo tempo na mesma árvore se bloqueiam. Rodar uma trilha por `git worktree`, ou intercalar com cada tarefa terminando verde e commitada. As trilhas são independentes em dependências, não necessariamente simultâneas na mesma árvore.
10. **Risco da WebView (design §5.5).** O `AssetServer` do Wails v2 pode não repassar `Range`. O teste da T27 cobre o handler; o comportamento real é conferido na T45.
11. **ScreenScraper sem credenciais.** A T02 só confere o que a documentação pública e o código de terceiros dizem; o que exige credencial fica "NÃO VERIFICÁVEL" com o roteiro para o usuário. Divergência que mude regra visível volta ao spec-writer antes da T21.

## Premissas

- Uma tarefa por commit; test-writer primeiro, implementer (ou tracker-specialist) depois. Ao fim de cada tarefa Go: `go build ./...`, `go vet ./...`, `CGO_ENABLED=0 go build ./...`, `go test ./...`. Ao fim de cada tarefa de frontend: `npm test` e `npx tsc --noEmit` (em `frontend/`).
- Todo caso de teste começa pelo ID (`t.Run("CON-005/...")`, `it("BBX-011 ...")`). Sem rede, sem `time.Sleep`: `httptest`, fakes, `domain.FakeClock` no Go e `vi.useFakeTimers` no frontend.
- **Só o tracker-specialist** escreve em `internal/tracker/` (T09..T12). `cmd/fake-game` não muda (CON-028). Os testes de integração Windows do tracker usam `--depth`, `--run forever` e `--run 0`, sempre com `--max-life 30s`.
- Migração: só a 00009 (T17). Nenhuma mudança em `game`, `play_session`, `pause_interval` ou `tracking_rule`.
- **Dependências externas novas: nenhuma** (Go e npm). `golang.org/x/sys/windows` e as DLLs `user32`/`kernel32` por `NewLazySystemDLL` bastam.
- `frontend/wailsjs/go/**` é versionado e regerado pelo `wails build` (T44); entra no commit da T44.

---

## T01 — Contrato completo `ipc.openrpc.json` v0.15.0
- Requisitos: CON-026, CON-017, CON-019, CON-031, VID-005, VID-006, VID-007, VID-008, SSC-001. **Responsável:** sessão principal (cópia mecânica de JSON grande, por script, como nas fases 11 e 12).
- Criar `docs/specs/phase-13-modo-console/contracts/ipc.openrpc.json` como cópia de `docs/specs/phase-12-metadados-unificados/contracts/ipc.openrpc.json` (v0.14.0) mais **só** o que o `ipc-v0.15.0-delta.md` descreve:
  - `info.version = "0.15.0"`, `info.description` (§1), `info.x-console` e `info.x-video` (§1);
  - enums ampliados (§2): `provider` de `provider.keyStatus`/`setKey`/`deleteKey` e de `ProviderKeyStatus.provider`; `MediaItem.kind` + `video`; `SessionItem.endReason` com os cinco valores; descrições de `provider.setKey.secret` e de `thumbPath`;
  - os `summary` de `game.launch` e `game.launchUntracked` ganham as frases do delta §2 (o de `game.launchUntracked` também troca "no Job Object");
  - esquemas novos `VideoSkip`, `VideoQueueResult`, `VideoQueueMissingResult`, `VideoLane`, `VideoStatus` (§3);
  - os 4 métodos `video.*` depois de `metadata.batchDiscard` (§4, com os `summary` e `paramStructure: by-name`) e a notificação `game.closedByHotkey` depois de `metadata.batchChanged` (§5);
  - **nada** mais muda: `media.queue.kinds` e `MetadataRef.provider` ficam como estão.
- Em `ipc-v0.15.0-delta.md` e `design.md` §3.3, trocar "(tarefa)" pelo arquivo criado. Dependência: nenhuma. Bloqueia T04.

## T02 — Conferência da API do ScreenScraper (documentação, sem credenciais)
- Requisitos: VID-010, VID-012, VID-013, VID-015, VID-016, VID-017, SSC-004 (as hipóteses de [rules](contracts/screenscraper-rules.md) e [api-notes](contracts/screenscraper-api-notes.md)). **Responsável:** sessão principal. Nenhuma credencial é usada nem pedida; nada é gravado em repositório ou log.
- Conferir os 15 pontos da tabela de `screenscraper-api-notes.md` na documentação pública da API v2 e no código aberto de clientes de terceiros, marcando cada linha CONFIRMADO (com a fonte), DIVERGENTE (valor correto) ou NÃO VERIFICÁVEL (exige credencial: vira o roteiro "Para o usuário conferir", que a T45 usa):
  - base e endpoints (`jeuRecherche.php`, `jeuInfos.php`);
  - campos de `jeux[]`, `noms`, `systeme`, `dates`, `medias`;
  - se a busca já traz `medias`;
  - o nome do tipo `video-normalized` e se existe `size`;
  - se a URL da mídia traz credenciais embutidas;
  - os campos de cota (`ssuser`);
  - os códigos HTTP e o corpo de erro em texto puro;
  - a tabela de ids de sistema de `rules §1` (e se existe sistema para o Switch 2);
  - a existência de vídeo por plataforma (decisão k: só anotar).
- Saída: `screenscraper-api-notes.md` com a coluna "Estado" preenchida e, onde houver divergência, o substituto. Divergência que mude regra visível (ex.: busca por nome inexistente, tipo de mídia com outro nome) pausa as tarefas T20 e T21 e volta ao spec-writer. Divergência só de valor (ids, códigos) é corrigida direto em `rules` pela sessão principal, antes da T20.
- Dependência: nenhuma. Bloqueia T20 e T21.

## T03 — Tipos IPC v0.15.0
- Requisitos: CON-019, VID-005, VID-006, VID-007, VID-008, SSC-001, CON-026. **implementer:** `internal/ipc/console.go` (novo), `internal/ipc/video.go` (novo), `internal/ipc/media.go` (só o comentário de `ProviderSetKeyParams.Secret`: "required for igdb, screenscraper and screenscraperdev"), `internal/ipc/protocol.go` (o comentário do pacote aponta para o contrato da fase 13).
```go
// console.go
const MethodGameClosedByHotkey = "game.closedByHotkey"
type GameClosedByHotkeyParams struct{ GameID int64 `json:"gameId"` }

// video.go
const (
	MethodVideoQueue         = "video.queue"
	MethodVideoQueueMissing  = "video.queueMissing"
	MethodVideoCancelPending = "video.cancelPending"
	MethodVideoStatus        = "video.status"
	ProviderScreenScraper    = "screenscraper"
	ProviderScreenScraperDev = "screenscraperdev"
	VideoSkipHasVideo        = "has-video"
	VideoSkipPlatformUnmapped = "platform-unmapped"
	VideoLaneActive = "active"; VideoLanePausedQuota = "paused-quota"
	VideoLanePausedCredentials = "paused-credentials"; VideoLaneNoCredentials = "no-credentials"
)
type VideoQueueParams struct{ GameIDs []int64 `json:"gameIds"` }
type VideoSkip struct{ GameID int64 `json:"gameId"`; Reason string `json:"reason"` }
type VideoQueueResult struct{ Queued int `json:"queued"`; Skipped []VideoSkip `json:"skipped"` } // list never nil
type VideoQueueMissingParams struct{ PlatformID int64 `json:"platformId,omitempty"` }
type VideoQueueMissingResult struct{ Queued int `json:"queued"`; Skipped int `json:"skipped"` }
type VideoCancelResult struct{ Cancelled int `json:"cancelled"` }
type VideoStatus struct {
	Pending, Running, Failed int // json: pending, running, failed
	Lane string `json:"lane"`
	PausedUntil *time.Time `json:"pausedUntil,omitempty"`
	RequestsToday, MaxRequestsPerDay *int `json:"requestsToday,omitempty"` // second with its own tag
}
type VideoStatusResult struct{ Status VideoStatus `json:"status"` }
```
(Todos com as tags JSON do delta §3 e §6.) Sem testes próprios: a T04 os cobre. Dependência: nenhuma. Arquivos: só esta tarefa mexe em `internal/ipc/{console.go,video.go,media.go,protocol.go}`.

## T04 — Testes de contrato da v0.15.0 e do "só acréscimo"
- Requisitos: CON-026, CON-017, CON-019, SSC-001, VID-005, VID-006, VID-007, VID-008. **test-writer:** `internal/ipc/contract_test.go` (`contractPath`/`contractRelPath` para a fase 13; versão `0.15.0`), `contract_phase12_test.go` e `contract_phase11_test.go` (passam a ler a v0.14.0 por `contractV014Path`), novo `contract_phase13_test.go`:
  - `CON-026/versão 0.15.0`; `CON-026/os 4 métodos video.* existem com paramStructure by-name e os nomes das constantes Go`; `CON-019/game.closedByHotkey é notificação com o único parâmetro gameId inteiro ≥ 1 e resultado null`;
  - `CON-026/campos de cada tipo Go batem com as propriedades do esquema (assertExactFields): VideoSkip, VideoQueueResult, VideoQueueMissingResult, VideoStatus`; `CON-026/VideoLane tem os 4 valores e as constantes Go os repetem`;
  - `SSC-001/enum provider tem os quatro valores nos quatro lugares (keyStatus, setKey, deleteKey, ProviderKeyStatus.provider)`; `CON-026/MediaItem.kind inclui video`; `CON-017/SessionItem.endReason lista exited, agent-shutdown, recovered, client-no-launch e closed-by-hotkey`;
  - `CON-026/x-console e x-video: valores literais do delta §1 e, para as constantes de domain, scrapers e media, igualdade com o código`: `domain.CloseDebounce`, `domain.MediaKindLimit("video")`, `scrapers.IntervalScreenScraper`, `scrapers.SoftName`, `scrapers.SSVideoMediaType`, `media.MaxVideoBytes` (as de `tracker`, `input`, `winapi` e `agent` são conferidas nas T07, T08, T11, T13 e T26);
  - `VID-005/video.queue.skipped serializa [] e não null; VideoStatus omite pausedUntil e as contagens de cota quando ausentes`;
  - **teste "só acréscimo"** (`CON-026/v0.15.0 é a v0.14.0 mais acréscimos`): os dois JSON como `map[string]any`; normaliza na v0.15.0 `info.version`, `info.description`, `info.x-console`, `info.x-video`, os 5 esquemas novos, os 4 métodos novos, a notificação nova, os valores novos dos 4 enums (`provider`, `MediaItem.kind`, `SessionItem.endReason`; confere que a v0.14.0 não os tem), as `description` de `provider.setKey.secret` e de `MediaItem.thumbPath`, e os `summary` de `game.launch` e `game.launchUntracked` (confere a frase nova na v0.15.0 e a ausência na v0.14.0); depois `reflect.DeepEqual` com a v0.14.0. Também confere que nenhum método, parâmetro, resultado, erro ou esquema da v0.14.0 mudou ou sumiu e que a ordem relativa dos métodos antigos se mantém.
- Dependência: T01, T03.

## T05 — Frontend: tipo de mídia `video` e a janela normal que o ignora
- Requisitos: VID-023. **implementer:** `frontend/src/lib/metadata.ts` (`MediaKind` + `'video'`; `mediaKindLabels.video = 'Vídeo'`; `orderMedia` descarta `kind === 'video'`; `cardImageOf` e `cardFallbackOrder` continuam sem vídeo), e o que mais usar a lista crua em `GameDetailsPanel.tsx`/`MediaThumbStrip.tsx` (carrossel e contagem de itens). Prerequisito das trilhas B e C (as duas leem o tipo).
```ts
export type MediaKind = 'cover' | 'screenshot' | 'title' | 'banner' | 'background' | 'select' | 'video'
export function orderMedia(items: MediaItemView[]): MediaItemView[] // no video
export function videoOf(items: MediaItemView[]): MediaItemView | undefined // first kind video; used by the Big Box
```
- **test-writer:** `metadata.test.ts`, `useCardImages.test.tsx`, `GameDetailsPanel.test.tsx`:
  - `VID-023/orderMedia descarta vídeo e mantém a ordem dos outros`; `VID-023/cardImageOf ignora vídeo mesmo sem outra imagem`;
  - `VID-023/jogo só com vídeo: o carrossel não aparece e o contador não o inclui`; `VID-023/jogo com capa e vídeo: o carrossel mostra só a capa`; `VID-023/videoOf devolve o primeiro vídeo ou undefined`.
- Dependência: nenhuma. Arquivo compartilhado: `lib/metadata.ts` e `metadata.test.ts` (T05, depois T28).

---

# Trilha A — Select+B (tracker, input, feedback, agente, desktop)

## T06 — Domínio: motivo `closed-by-hotkey` e antirrepique do fechar
- Requisitos: CON-016, CON-004. **implementer:** `internal/domain/session.go`, `internal/domain/pause.go`.
```go
const EndClosedByHotkey SessionEndReason = "closed-by-hotkey" // next to EndClientNoLaunch; accepted by ParseSessionEndReason
const CloseDebounce = 300 * time.Millisecond // next to PauseToggleDebounce; x-console.closeDebounceMs
```
Se `store.EndSession` validar o motivo, usa `ParseSessionEndReason` (nenhuma migração: `end_reason` é `TEXT` sem `CHECK`).
- **test-writer:** `internal/domain/session_test.go`, `pause_test.go`, `internal/store/sessions_test.go`:
  - `CON-016/ParseSessionEndReason aceita exited, agent-shutdown, recovered, client-no-launch e closed-by-hotkey`; `CON-016/rejeita "", closed_by_hotkey, Closed-By-Hotkey e outro valor`;
  - `CON-004/CloseDebounce é 300 ms e é independente de PauseToggleDebounce`;
  - `CON-016/o store grava e lê o motivo closed-by-hotkey (EndSession e GameSessions)`; `CON-016/nenhuma migração nova para o motivo` (a lista de migrações só ganha a 00009 na T17).
- Dependência: nenhuma.

## T07 — Entrada: segundo detector `Back + B` no poller
- Requisitos: CON-001, CON-002, CON-003. **implementer:** `internal/input/input.go`, `internal/input/combo.go`, `internal/input/pad.go`.
```go
type Action int
const ( ActionPause Action = iota; ActionClose ) // zero value keeps the meaning of existing triggers
type Trigger struct{ Source domain.PauseReason; Action Action; At time.Time }
const ButtonB uint16 = 0x2000
// combo.go: defaultCloseMask = ButtonBack | ButtonB
// pad.go: PadConfig gains CloseMask uint16 (zero = default); pad holds det (Back|Y) and closeDet (Back|B), read in the same pass;
// each one delivers Trigger{Source: domain.PauseGamepad, Action: ..., At: now}; same queue, same full-queue Warn.
```
O atalho de teclado continua só `ActionPause` (nenhuma mudança em `hotkey_windows.go`).
- **test-writer:** `pad_test.go`, `combo_test.go`, `hotkey_windows_test.go`:
  - `CON-001/Back+B segurado dispara Trigger{ActionClose, gamepad} na primeira leitura com agora−T ≥ 1 s (T+992ms não, T+1008ms sim, leituras de 16 ms)`; `CON-001/soltar antes cancela`; `CON-001/um gatilho por pressão`; `CON-001/controles independentes`; `CON-001/desconexão zera a contagem`;
  - `CON-002/Back+Y só entrega ActionPause`; `CON-002/Back+B só entrega ActionClose`; `CON-002/Back+Y+B entrega os dois, cada um uma vez`; `CON-002/soltar B não zera a contagem de Back+Y e vice-versa`; `CON-002/PadConfig.Mask e CloseMask personalizadas`; `CON-002/fila de saída cheia descarta com Warn`;
  - `CON-001/a espera padrão é 1 s (x-console.closeHoldMs)`;
  - `CON-003/RegisterHotKey é chamado uma vez, só para Ctrl+Alt+P, e o gatilho vem com ActionPause`; `CON-003/os testes antigos de PAD-002..PAD-005 passam sem mudança`.
- Dependência: nenhuma.

## T08 — Feedback: `EventRefused`, `CueRefused` e o sinal do `Beeper`
- Requisitos: CON-015, CON-014. **implementer:** `internal/feedback/feedback.go`, `internal/winapi/beep_windows.go`.
```go
const EventRefused Event = 3   // after EventResumed; tracker.FeedbackKind does not change
const CueRefused Cue = 3       // after CueResumed
// Coordinator.handle: EventRefused -> sound.Play(CueRefused), never overlay.Show; run(): only events that show a notice re-arm the hide timer.
// Beeper.signal(CueRefused): tone(300, 100) then tone(200, 200), honoring stop between the two.
```
- **test-writer:** `internal/feedback/feedback_test.go`, `internal/winapi/beep_windows_test.go`:
  - `CON-015/EventRefused toca só CueRefused e nunca chama Overlay.Show`; `CON-015/EventRefused não rearma o temporizador: um aviso de pausa visível esconde no prazo original (relógio fake)`; `CON-015/EventRefused sem overlay não falha`; `CON-014/EventPaused e EventResumed continuam como antes (FBK-002)`;
  - `CON-015/CueRefused = 300 Hz por 100 ms e 200 Hz por 200 ms (função de tom injetada)`; `CON-015/Close entre os dois tons não toca o segundo`; `CON-015/x-console.refusedCue do contrato é igual aos dois tons`; `CON-015/os sinais de pausa e retomada não mudam (SND-001)`.
- Dependência: nenhuma.

## T09 — Tracker: máquina de estados com `Closing`
- Requisitos: CON-005, CON-006, CON-007, CON-013, CON-014, CON-032, CON-033. **tracker-specialist:** `internal/tracker/machine.go`. Puro: sem relógio, I/O ou goroutine.
```go
const StateClosing State = iota // appended after StatePaused
// New Machine fields: closeAt time.Time, closeFrom State.
func (m *Machine) CloseRequested(at time.Time) []Effect
// Running: accumulated += at-runningSince; Paused: unchanged; both -> StateClosing, closeAt = at, closeFrom set, NO effects.
// Grace: goes straight to Idle: EndSession{At: graceSince, Active: accumulated, Reason: EndClosedByHotkey, State: Outcome}, CloseJob,
//        Notify{EndedAt: graceSince, AsOf: at}, graceGen = 0. Other states: nil.
func (m *Machine) CloseDone(now time.Time) []Effect
// Closing -> Idle: EndSession{At: closeAt, Active: accumulated, Reason: EndClosedByHotkey, State: Outcome (ended | discarded)}, CloseJob,
// Notify{EndedAt: closeAt, AsOf: now}. Never an EffFeedback.
func (m *Machine) CloseUndone() []Effect
// Closing -> closeFrom. Running: runningSince = closeAt. Paused: nothing changes. Returns armHeartbeat and armPoll (new generations).
func (m *Machine) ClosingAt() (at time.Time, from State, ok bool)
```
`Snapshot` em `Closing` devolve o estado de origem (`running`/`paused`) com `Active = accumulated` congelado (`activeAt` só soma em Running); `Pending` ok em `Closing`. `Stop` em `Closing`: `EndSession{At: closeAt, Active: accumulated, Reason: EndAgentShutdown}`, `CloseJob`, sem `Notify`. `TogglePause`, `Pause`, `Resume`, `Suspend`, `Wake`, `Heartbeat`, `TrackedEmpty` e `ProcessAdded` em `Closing` devolvem `nil`; `Poll` em `Closing` só rearma. `NotPausable` não muda (o coordenador trata `Closing` antes).
- **test-writer:** `internal/tracker/machine_test.go` (tabela por linha de [session-close-state-machine.md](contracts/session-close-state-machine.md)):
  - `CON-005/Running com 600 s: CloseRequested entra em Closing sem efeitos e CloseDone grava ended_at=T+600s, active 600 s, closed-by-hotkey, ended`; `CON-005/o instante de CloseDone não muda os valores gravados (T+602s e T+603s), só o AsOf`; `CON-005/20 s de tempo ativo termina discarded`; `CON-005/CloseRequested fora de Running, Paused e Grace é nil`;
  - `CON-006/Paused com 100 s: tempo congelado, EndSession em closeAt com active 100 s`;
  - `CON-007/Grace: CloseRequested devolve EndSession em graceSince, CloseJob e Notify com AsOf=at, e o GraceTimeout antigo vira obsoleto`;
  - `CON-013/CloseUndone em Running: runningSince=closeAt, o tempo da espera passa a contar, devolve armHeartbeat e armPoll`; `CON-013/CloseUndone em Paused não muda nada`;
  - `CON-014/nenhum caminho de fechamento emite EffFeedback`;
  - `CON-032/Snapshot em Closing é o estado de origem com o tempo de closeAt`; `CON-032/TogglePause, Pause, Resume, Suspend, Wake e Heartbeat em Closing não geram efeitos`; `CON-032/Poll em Closing só rearma`;
  - `CON-033/Stop em Closing: ended_at=closeAt, agent-shutdown, CloseJob e nenhum Notify`.
- Dependência: T06 (o motivo). Arquivo compartilhado: `machine.go` e `machine_test.go` só aqui.

## T10 — Tracker: `Kill`, `RequestClose` (`WM_CLOSE`) e o conjunto adotado
- Requisitos: CON-008, CON-030, CON-013, CON-028. **tracker-specialist:** `internal/tracker/launcher.go` (só a interface `Job`), `internal/tracker/job_windows.go`, `internal/tracker/job_other.go`, `internal/tracker/closer_windows.go` (novo), `internal/tracker/closer_other.go` (novo), `internal/tracker/adopted_job.go`.
```go
// launcher.go: Job gains
//   RequestClose() (int, error) // posts WM_CLOSE to the visible top-level windows of the live processes of the tree; returns how many windows got it
//   Kill() error                // terminates every process of the tree; valid after Resume; never closes the job
type windowCloser interface{ PostClose(pids map[uint32]struct{}) (posted int, err error) }
func newWindowCloser() windowCloser // windows: EnumWindows; other OS: returns 0, errors.ErrUnsupported
// closer_windows.go: a seam for tests
type closerAPI interface {
	EnumTopLevel(fn func(hwnd uintptr) bool) error
	IsVisible(hwnd uintptr) bool
	Owner(hwnd uintptr) uintptr
	ProcessOf(hwnd uintptr) uint32
	Post(hwnd uintptr, msg uint32, w, l uintptr) error
}
func newWindowCloserWith(api closerAPI) windowCloser
// winJob.Kill: TerminateJobObject(job, 1) under j.mu; refused when closed. winJob.RequestClose: pids from
//   QueryInformationJobObject(JobObjectBasicProcessIdList) (buffer grows while NumberOfAssignedProcesses > NumberOfProcessIdsInList).
type processKiller interface{ Kill(pid uint32, wantImage string) error }
// adoptedJob: setClosers(c windowCloser, k processKiller) (defaults: real ones). RequestClose: fresh snapshot; set = PIDs of the snapshot
//   that are in j.live and still matchesAny. Kill: for each one, open with PROCESS_TERMINATE|PROCESS_QUERY_LIMITED_INFORMATION,
//   compare QueryFullProcessImageName with the snapshot path (case-insensitive), TerminateProcess(h, 1); errors joined.
```
Handles e callback: `EnumWindows` usa **um único callback criado uma vez no pacote** (`sync.Once` + `windows.NewCallback`, que não libera callbacks) e um estado de varredura protegido por mutex de pacote; o callback roda na mesma thread e de forma síncrona. `PostMessageW` apenas, nunca `SendMessage`; nenhuma janela filha, sem `IsWindowVisible` ou com dono (`GW_OWNER != 0`) é tocada. O `processKiller` real abre o processo e dá `defer windows.CloseHandle(h)` logo depois do erro checado. `Terminate` continua só para lançamento falho; `adoptedJob.Terminate` continua `ErrNotOwned`.
- **test-writer:** `internal/tracker/closer_windows_test.go` (novo), `job_windows_test.go`, `adopted_job_test.go`, `fakes_test.go` (o fake `Job` ganha os dois métodos e contadores):
  - `CON-030/o fechador só posta WM_CLOSE (0x10, wParam e lParam 0) a janelas de topo, visíveis e sem dono, de PIDs do conjunto`; `CON-030/janela de PID fora do conjunto (o Arcadia, o cliente da loja) nunca recebe`; `CON-030/erro ao enumerar devolve 0 e o erro`; `CON-030/o callback do EnumWindows é criado uma vez (duas chamadas seguidas, um só NewCallback)`; `CON-030/conjunto vazio devolve 0 sem enumerar`;
  - `CON-030/winJob.RequestClose lista só os PIDs do job (fake-game --depth 2: 3 PIDs) e devolve 0 sem janela`; `CON-030/a lista de PIDs cresce o buffer (fake-game com muitos filhos)`; `CON-030/winJob.Kill mata os 3 processos (espera real limitada a 10 s)`; `CON-030/Kill depois de Resume é permitido e Terminate continua recusado`; `CON-030/Kill depois de Close devolve erro`; `CON-013/Kill de job vazio é nil`;
  - `CON-008/o conjunto a fechar é o dos adotados vivos que ainda casam com uma regra pelo caminho (snapshot novo)`; `CON-008/processo fora das regras e o cliente da loja nunca recebem WM_CLOSE nem morrem`; `CON-008/PID reaproveitado (caminho diferente do snapshot) não é morto`; `CON-008/conjunto adotado vazio: RequestClose 0 e Kill nil`; `CON-008/saída durante a espera: o aviso de nenhum adotado vivo (EventActiveZero) chega pelo canal`; `CON-008/erro de Kill de um processo não impede o dos outros e vira erro juntado`; `CON-008/processKiller real: caminho certo mata, caminho errado não, handle fechado (contagem de handles)`.
- Dependência: nenhuma (usa só os tipos existentes). Arquivo compartilhado: `launcher.go` (T10, depois T12). Os arquivos de `internal/tracker` não são editados em paralelo por duas tarefas (ver "Arquivos compartilhados").

## T11 — Tracker: `CloseByHotkey` para a sessão (Running, Paused, Grace, Launching)
- Requisitos: CON-005, CON-006, CON-007, CON-008, CON-009, CON-012, CON-013, CON-014, CON-021, CON-030, CON-031, CON-032, CON-033. **tracker-specialist:** `internal/tracker/close.go` (novo, a lógica), `internal/tracker/tracker.go` (ganchos mínimos: `ctrlClose`, `run`, `handleJobEvent`, `handlePoll`, `handleControl`, `Launch`, `shutdown`).
```go
type CloseKind int
const ( CloseClosed CloseKind = iota + 1; CloseRefused; CloseFailed )
const ( CloseReasonNoGame = "no-game"; CloseReasonNotAdopted = "not-adopted"; CloseReasonClosing = "closing" )
type CloseResult struct{ Kind CloseKind; GameID int64; Untracked bool; Reason string }
const ( closeWait = 3 * time.Second; detachedPollEvery = 250 * time.Millisecond )
// CloseByHotkey asks the game in progress to close (WM_CLOSE), waits up to closeWait on the Clock for the tree to end, kills what is left and
// ends its session at `at` with reason closed-by-hotkey. The reply comes when the close is over; the owner goroutine never blocks.
func (t *Tracker) CloseByHotkey(ctx context.Context, at time.Time) (CloseResult, error)
type closing struct{ at time.Time; reply chan ctrlResult; gen uint64; untracked bool; gameID int64; title string } // owned by the run goroutine
// ctrlResult gains close CloseResult.
```
- Fluxo no dono: `handleClose(req)` responde sozinho (o `run` não faz `req.reply <- handleControl` para `ctrlClose`), com canal de resposta de buffer 1. Passos:
  1. shutdown → `ErrShuttingDown`.
  2. `t.closing != nil` → `Refused{closing}`.
  3. Running/Paused com `at ≥ startedAt` → `m.CloseRequested(monotonic(at))`, `RequestClose`; `n == 0` ou erro (`Debug`) → `Kill` na hora e `finishClose`; senão guarda `t.closing` e arma `clock.After(closeWait)` com geração.
  4. Grace → `Kill` (sem pedido nem espera); ok → `m.CloseRequested` devolve o fim; erro → `Failed` e `Warn`.
  5. Launching URI → `Refused{not-adopted}`.
  6. resto, inclusive `at < startedAt` → `Refused{no-game}` com `Debug "fechar ignorado"`.
- Fim da espera: `EventActiveZero` (job) ou aviso de nenhum adotado vivo, ou `handlePoll` com `ActiveProcesses() == 0` em `Closing` → `finishClose` sem `Kill`; `closeTimeout` com a geração atual → `Kill`; ok → `finishClose`; erro → `Warn "falha ao fechar o jogo"` com `session_id` e `error`, `m.CloseUndone()` (reexecuta os efeitos de rearme), `Failed`. `finishClose`: `m.CloseDone(monotonic(now))` com `exec` (EndSession, CloseJob, Notify) e só depois responde `CloseClosed{GameID}`; zera `t.closing`. Disparos de geração antiga são ignorados.
- Durante `Closing`: `Launch` e `LaunchUntracked` → `SessionActiveError` da sessão (`activeSession` já cobre pelo `Snapshot`); `Pause`, `Resume` devolvem a sessão atual sem gravar; `TogglePause`, `Suspend`, `Wake`, heartbeat ignorados (`Debug`); `Current()` mostra o estado de origem com o tempo de `at`; nenhum `session.changed` antes do fim. `shutdown()` (Stop) em `Closing`: não mata, `m.Stop`, responde `ErrShuttingDown` no `closing.reply`.
- **test-writer:** `internal/tracker/tracker_close_test.go` (novo; usa `FakeClock` e `BlockUntilWaiters`; `Closing` soma 1 waiter), `fakes_test.go` (fakes de `Job` com `RequestClose`/`Kill` programáveis):
  - `CON-005/Running: a árvore sai em 1 s durante a espera, sem Kill, e a sessão termina ended com ended_at=T+600s, active 600 s, closed-by-hotkey, session.changed com endedAt e asOf, total do jogo +600 e resultado closed`; `CON-005/a árvore não sai: Kill em at+3s e os mesmos valores`; `CON-005/RequestClose devolve 0: Kill imediato e fim na hora`; `CON-005/20 s de tempo ativo termina discarded e o jogo não muda`; `CON-005/at anterior ao startedAt: refused no-game, nada gravado`; `CON-005/o dono continua atendendo a espera (evento do job e session.current durante os 3 s)`;
  - `CON-006/Paused: pausa fechada em at, active 100 s, sem feedback`; `CON-007/Grace: nenhum RequestClose, Kill chamado, ended_at=graceSince, asOf=at, a carência pendente não grava nada depois`; `CON-007/Grace com Kill que falha: failed e a carência segue`;
  - `CON-008/sessão adotada: o conjunto vazio termina na hora; o prazo mata só quem ainda casa; saída durante a espera sem Kill`;
  - `CON-009/Launching de URI sem adoção: refused not-adopted, nada gravado, o prazo de 60 s segue`; `CON-012/Idle sem detached: refused no-game com Debug`;
  - `CON-013/Kill falha na espera: failed, Warn com session_id e error, Running continua contando de at sem lacuna nem desconto, nada gravado ou notificado, o heartbeat seguinte grava`; `CON-013/Paused com Kill que falha: continua pausada com o mesmo intervalo`; `CON-013/só parte dos processos adotados morreu: a sessão segue e o watcher encerra`;
  - `CON-014/closed não chama o feedback do tracker`;
  - `CON-021/depois de BeginShutdown: ErrShuttingDown, sem RequestClose nem Kill`;
  - `CON-030/espera de 3 s medida só pelo Clock (nenhum Sleep); disparo de geração antiga ignorado`;
  - `CON-031/game.launch e launchUntracked durante a espera: SessionActive com a sessão, nada criado; depois do fim os lançamentos voltam`;
  - `CON-032/TogglePause, Suspend, Wake e heartbeat ignorados, session.pause e session.resume devolvem a sessão sem gravar, Current congelado, nenhum session.changed antes do fim, segundo CloseByHotkey refused closing`;
  - `CON-033/Close do tracker durante a espera: nada morto, ended_at=at, agent-shutdown, sem notificação e o CloseByHotkey pendente responde ErrShuttingDown`;
  - `CON-030/x-console.closeWaitMs e closeDetachedPollMs do contrato são iguais a closeWait e detachedPollEvery`.
- Integração Windows (`integration_windows_test.go`; Windows; `--max-life 30s`): `CON-005/fake-game --depth 2 --run forever (sem janela): RequestClose devolve 0, Kill imediato e os três processos terminam (WaitForSingleObject com prazo real limitado)`; `CON-008/adoção real do fake-game com regra de caminho: Kill encerra só o adotado`.
- Dependência: T09, T10. Arquivo compartilhado: `tracker.go` (T11, depois T12).

## T12 — Tracker: jogo sem contar tempo em Job Object guardado (`detachedJob`)
- Requisitos: CON-010, CON-011, CON-012, CON-013, CON-030, CON-031, CON-033. **tracker-specialist:** `internal/tracker/launcher.go` (`Launcher`, `DetachedJob`), `internal/tracker/detached_windows.go` (novo), `internal/tracker/job_windows.go` (`winLauncher.LaunchDetached` e o antigo `launchDetached`), `internal/tracker/job_other.go`, `internal/tracker/close.go` e `tracker.go` (campo `detached`, fechamento do jogo sem contar tempo, `shutdown`).
```go
type DetachedJob interface {
	ActiveProcesses() (int, error)
	RequestClose() (int, error)
	Kill() error
	Close() error // never kills; idempotent; the only owner of the job handle
}
// Launcher.LaunchDetached changes to: LaunchDetached(spec LaunchSpec) (DetachedJob, uint32, error)
// Windows: CreateProcess(CREATE_SUSPENDED|CREATE_DEFAULT_ERROR_MODE) -> CreateJobObject with JOB_OBJECT_LIMIT_BREAKAWAY_OK only
//   (no KILL_ON_JOB_CLOSE, no SILENT_BREAKAWAY_OK, ADR 0016) -> AssignProcessToJobObject -> ResumeThread -> close hThread and hProcess.
//   Any failure: TerminateProcess of the suspended process, close everything, *LaunchError (SES-034).
// Tracker: t.detached DetachedJob + gameID + title (owned by the run goroutine); a new launch closes the previous one WITHOUT killing;
//   shutdown() closes it without killing. No new goroutine, no completion port.
```
Fechamento sem sessão (Idle com `t.detached`): `ActiveProcesses()` = 0 → `Close`, `nil`, `Refused{no-game}`; > 0 ou erro → `RequestClose`; `n == 0` → `Kill` na hora e `Closed{Untracked}`; senão `t.closing` (com `untracked`), `clock.After(closeWait)` e `clock.After(detachedPollEvery)` rearmado enquanto `ActiveProcesses() > 0`; `Kill` com erro → `Warn`, `detached` continua, `Failed`. Com sessão aberta **e** `detached` vivo, o primeiro `CloseByHotkey` fecha só a sessão. Lançamento durante a espera: `SessionActive{sessionId 0, gameId, title}`. Waiters: o detached em espera soma 2.
- **test-writer:** `internal/tracker/tracker_close_detached_test.go` (novo), `fakes_test.go` (o fake `Launcher` passa à nova assinatura; atualiza `tracker_test.go`/`uri_test.go` que o usam), `job_windows_test.go`, `integration_windows_test.go`:
  - `CON-010/detached com processos: RequestClose, espera, Kill no prazo, Close do job e result closed untracked, nenhuma linha de play_session e nenhum session.changed`; `CON-010/a conferência a cada 250 ms vê ActiveProcesses=0 e fecha sem Kill`; `CON-010/ActiveProcesses=0 de início: refused no-game e o job é fechado e esquecido`; `CON-010/sessão aberta e detached vivo: o primeiro fecha só a sessão e o segundo fecha o detached`; `CON-010/Kill falha: failed e o detached continua guardado`; `CON-010/RequestClose devolve 0: Kill na hora`;
  - `CON-011/LaunchUntracked guarda só o último job: um novo lançamento fecha o anterior sem matar`; `CON-011/Tracker.Close fecha o job guardado sem matar`; `CON-011/falha ao criar ou associar o job é LaunchFailed e fecha tudo`; `CON-011/o contrato SES-032 continua: nenhuma sessão e nenhuma notificação`;
  - `CON-012/sem detached e sem sessão: refused no-game`; `CON-031/launch e launchUntracked durante o fechamento do detached: SessionActive com sessionId 0, gameId e título do jogo sendo fechado`; `CON-033/Stop durante a espera do detached: Close sem matar e o CloseByHotkey pendente responde ErrShuttingDown`; `CON-030/segundo CloseByHotkey durante a espera do detached: refused closing`; `CON-013/Kill falha no detached`;
  - Windows: `CON-011/o processo nasce dentro do job (IsProcessInJob) com BREAKAWAY_OK e sem KILL_ON_JOB_CLOSE nem SILENT_BREAKAWAY_OK (QueryInformationJobObject)`; `CON-011/o processo continua vivo depois de DetachedJob.Close e de Tracker.Close`; `CON-011/20 lançamentos de fake-game --run 0 e um Close: contagem de handles ≤ h0 + 2 (TRK-011)`; `CON-010/fake-game --depth 1 --run forever: Kill termina os dois processos`; `CON-011/Close duas vezes é idempotente`.
- Dependência: T11. Arquivos compartilhados: `launcher.go` (após T10), `tracker.go` e `close.go` (após T11), `job_windows.go` (após T10), `fakes_test.go` (T10, T11 e T12, nesta ordem).

## T13 — Agente: roteador de gatilhos com duas ações e `game.closedByHotkey`
- Requisitos: CON-004, CON-012, CON-014, CON-015, CON-019, CON-021. **implementer:** `internal/agent/pause.go` (`pauseRouter` vira `triggerRouter`; `powerBridge` e `pauseTracker` ficam), `internal/agent/notifier.go` (`gameClosedNotifier`).
```go
const closeTimeout = 6 * time.Second // x-console.closeHandlerTimeoutMs: 3 s of wait + kill + EndSession (2 s) + margin
type closeTracker interface{ CloseByHotkey(ctx context.Context, at time.Time) (tracker.CloseResult, error) }
type routerTracker interface{ pauseTracker; closeTracker }
type feedbackSender interface{ Send(feedback.Event) }
type closeSink interface{ GameClosedByHotkey(gameID int64) }
type triggerRouter struct{ /* tr, in, log, fb, sink, stop, ctx cancelled by Close, lastPause, lastClose */ }
func newTriggerRouter(tr routerTracker, in <-chan input.Trigger, log *slog.Logger, fb feedbackSender, sink closeSink) *triggerRouter
func (r *triggerRouter) Start()
func (r *triggerRouter) Close() // idempotent; cancels the ctx of a pending CloseByHotkey and waits for the goroutine
type gameClosedNotifier struct{ srv atomic.Pointer[ipc.Server] }
func (n *gameClosedNotifier) set(s *ipc.Server)
func (n *gameClosedNotifier) GameClosedByHotkey(gameID int64) // no-op when srv is nil; never blocks (IPC-017)
```
Regras: duas janelas de antirrepique (`lastPause`, `lastClose`, 300 ms medidos pelo `At`, só por ação); gatilho de pausa e de fechar no mesmo instante vão ambos, o que chegou primeiro vai primeiro. Resultado de `CloseByHotkey`: `Closed` → `sink.GameClosedByHotkey(GameID)`, sem feedback; `Refused{no-game|not-adopted}` e `Failed` → `fb.Send(feedback.EventRefused)`; `Refused{closing}` → só `Debug`; `ErrShuttingDown` → `Debug`; prazo vencido → `Warn "tracker não respondeu ao fechar a tempo"` sem som. O contexto de cada chamada é `WithTimeout(r.ctx, closeTimeout)` (achado 1).
- **test-writer:** `internal/agent/pause_test.go` (renomeado para o roteador; os casos PAU-* continuam), `internal/agent/close_router_test.go` (novo), `notifier_test.go`:
  - `CON-004/Closed chama CloseByHotkey com prazo de 6 s`; `CON-004/dois gatilhos de fechar com menos de 300 ms entre os At: o segundo é descartado sem som`; `CON-004/300 ms exatos passam`; `CON-004/as janelas são por ação: pausa e fechar no mesmo instante vão os dois, na ordem de chegada`; `CON-004/PAU-004 continua valendo entre gatilhos de pausa`;
  - `CON-012/refused no-game toca EventRefused`; `CON-015/refused not-adopted e failed enviam EventRefused; refused closing, descarte do antirrepique e ErrShuttingDown não tocam nada`; `CON-014/closed (sessão e untracked) não envia feedback`; `CON-021/ErrShuttingDown: Debug, sem som e sem notificação`; `CON-021/Close do roteador durante uma chamada pendente retorna sem esperar os 6 s e sem tocar nada`; `CON-021/prazo vencido: Warn sem som`;
  - `CON-019/a notificação sai depois da resposta do tracker, com gameId`; `CON-019/não sai para refused nem failed`; `CON-019/dois clientes IPC reais recebem game.closedByHotkey {gameId} e, com sessão, depois do session.changed` (tracker falso que dispara o `Notify` antes de responder); `CON-019/fila cheia de uma conexão descarta sem bloquear (IPC-017)`; `CON-019/gameClosedNotifier sem servidor e depois de set(nil) não faz nada`; `CON-004/x-console.closeHandlerTimeoutMs é igual a closeTimeout`.
- Dependência: T03, T06, T07, T08 e a assinatura de `tracker.CloseResult`/`CloseByHotkey` da T11 (o roteador compila contra o tipo; os testes usam um tracker falso). Arquivo compartilhado: `notifier.go` e `pause.go` só aqui.

## T14 — Agente: ligação da trilha A em `app.go` e invariantes do shutdown
- Requisitos: CON-003, CON-017, CON-019, CON-021, CON-012, CON-014, CON-015. **implementer:** `internal/agent/app.go` (só os pontos da trilha A):
  - `closeNotes := &gameClosedNotifier{}`; `closeNotes.set(server)` ao lado de `mediaNotes.set(server)`;
  - `router := newTriggerRouter(tr, triggers, log, coord, closeNotes)`;
  - `instance` guarda `closeNotifier`; no `Shutdown`, `a.closeNotifier.set(nil)` junto dos outros `set(nil)`, depois de `a.wg.Wait()`. A ordem do restante não muda: fontes e roteador fecham antes do drain, e o `Tracker.Close` (que dá o `Stop`) vem depois.
- **test-writer:** `internal/agent/app_test.go`, `order_test.go`, `tracker_wiring_test.go`, `components_test.go`:
  - `CON-019/ponta a ponta: pad falso com Back+B segurado 1 s, tracker real com launcher falso e job falso: a sessão termina com closed-by-hotkey, o cliente IPC recebe session.changed e depois game.closedByHotkey`; `CON-017/session.list devolve endReason closed-by-hotkey para a sessão fechada e client-no-launch para a de URI descartada`;
  - `CON-003/o único atalho de teclado registrado pelo agente continua sendo Ctrl+Alt+P`; `CON-012/Back+B sem jogo: o som de recusa é pedido ao coordenador de feedback (sink falso) e nada é gravado`; `CON-014/sucesso não envia feedback`;
  - `CON-021/Shutdown com fechamento em espera termina dentro do orçamento, sem goroutine restante (-race)`; `CON-021/a ordem do Shutdown continua router.Close antes do drain, tracker.close depois do server.close`; `CON-015/refused not-adopted toca a recusa`.
- Dependência: T03, T07, T08, T11, T12, T13. Arquivo compartilhado: `app.go`, `order_test.go`, `app_test.go` (T14, depois T26; nunca em paralelo).

## T15 — Desktop: repasse de `game:closedByHotkey` e `BringToFront`
- Requisitos: CON-020, CON-023, CON-027. **implementer:** `cmd/arcadia/game_events.go` (novo), `cmd/arcadia/supervisor.go` (um `case`), `cmd/arcadia/window_front.go` (novo; `App.BringToFront`), `cmd/arcadia/app.go` (campo `platform.bringToFront`), `cmd/arcadia/main_windows.go`, `cmd/arcadia/main_other.go`, `internal/winapi/foreground_windows.go`, `internal/winapi/user32_windows.go` (procs novos).
```go
// cmd/arcadia
const gameClosedEvent = "game:closedByHotkey"
type GameClosedView struct{ GameID int64 `json:"gameId"` }
func decodeGameClosed(raw json.RawMessage) (GameClosedView, error) // same validation as decodeMediaChanged
func (a *App) BringToFront() error // calls a.platform.bringToFront(); Debug with the result; never calls the agent; the Windows refusal is not an error
// platform gains: bringToFront func() (foreground bool, err error)

// internal/winapi (windows)
type ForegroundAPI interface {
	IsIconic(hwnd uintptr) bool
	ShowWindow(hwnd uintptr, cmd int)
	SetWindowPos(hwnd, insertAfter uintptr)        // HWND_TOPMOST then HWND_NOTOPMOST, SWP_NOMOVE|SWP_NOSIZE
	SetForegroundWindow(hwnd uintptr) bool
	GetForegroundWindow() uintptr
	ThreadOf(hwnd uintptr) uint32
	CurrentThread() uint32
	AttachThreadInput(from, to uint32, attach bool) bool
	BringWindowToTop(hwnd uintptr) bool
}
func ForceForegroundWith(api ForegroundAPI, hwnd uintptr) bool
func ForceForeground(hwnd uintptr) bool    // system API
func MainWindow() (uintptr, error)         // visible top-level window of this process with the largest area; one package-level EnumWindows callback
```
- **test-writer:** `cmd/arcadia/game_events_test.go`, `supervisor_test.go`, `window_front_test.go`, `internal/winapi/foreground_windows_test.go`:
  - `CON-020/evento válido é emitido com gameId`; `CON-020/params ausentes, JSON inválido, gameId ausente, não inteiro, 0 e negativo: Warn "game.closedByHotkey com parâmetros inválidos" e nada emitido`; `CON-020/o repasse não chama o agente`; `CON-020/media.changed continua repassado`;
  - `CON-023/BringToFront chama a plataforma uma vez e nunca o agente`; `CON-023/recusa do Windows (foreground false) devolve nil`; `CON-023/erro de programação (sem janela) vira erro`;
  - `CON-023/ForceForegroundWith: minimizada → ShowWindow(SW_RESTORE); depois SetWindowPos topmost e não-topmost; depois SetForegroundWindow (ordem das chamadas)`; `CON-023/foreground já é a janela: nada de AttachThreadInput`; `CON-023/foreground negado: AttachThreadInput(fg, self, true), BringWindowToTop, SetForegroundWindow, AttachThreadInput(…, false) (o desanexar roda mesmo se o passo do meio falhar)`; `CON-023/MainWindow escolhe a visível de maior área do próprio processo (API falsa)`; `CON-027/BringToFront existe como binding`.
- Dependência: T03. Arquivo compartilhado: `cmd/arcadia/app.go` (T15 só o campo; T27 não o toca), `supervisor.go` só aqui.

## T16 — Frontend: "fechado pelo controle" em `SessionsList`
- Requisitos: CON-018. **implementer:** `frontend/src/lib/library.ts` (`SessionEndReason` = `'exited' | 'agent-shutdown' | 'recovered' | 'client-no-launch' | 'closed-by-hotkey'`), `frontend/src/components/SessionsList.tsx`.
- **test-writer:** `SessionsList.test.tsx`: `CON-018/endReason closed-by-hotkey mostra a duração e "fechado pelo controle"`; `CON-018/exited, agent-shutdown, recovered e client-no-launch não ganham texto novo`; `CON-018/sem endReason (sessão aberta) não mostra o texto`.
- Dependência: nenhuma.

---

# Trilha B — vídeo do ScreenScraper

## T17 — Migração 9 (tipo `video`) e limite do domínio
- Requisitos: VID-001, VID-002. **implementer:** `internal/store/migrations/00009_video.sql` (novo), `internal/store/store.go` (`SchemaVersion` 8 → 9), `internal/domain/media_limits.go` (`case "video": return 1`).
  - `Up`: reconstrói `media` e `media_job` com o `CHECK` de `kind` aceitando `video` (cria `*_new`, copia tudo, apaga, renomeia; mantém `UNIQUE (game_id, hash)` e a FK `ON DELETE CASCADE`), na transação do goose. A tabela `game` não muda.
  - `Down`: apaga as linhas `video` e reconstrói sem o tipo.
- **test-writer:** `internal/store/migrate9_test.go` (molde de `migrate8_test.go`: banco em v8 por `goose UpTo(8)` com mídias e jobs), `internal/domain/media_limits_test.go`, e a atualização dos testes que usam o literal da versão:
  - `VID-001/banco novo vai a 9 e aceita video em media e media_job`; `VID-001/banco na v8 com mídias e jobs migra preservando todos os dados e o UNIQUE (game_id, hash)`; `VID-001/a tabela game e as de sessão não mudam (esquema igual ao da v8 fora de media e media_job)`; `VID-001/kind inválido continua recusado`; `VID-001/Down apaga as linhas video, mantém o resto e volta ao CHECK antigo`; `VID-001/a lista de migrações termina na 00009 e SchemaVersion = 9`;
  - `VID-002/MediaKindLimit("video") = 1 e os demais valores não mudam (cover 1, screenshot 10, background/title/select/banner 2, desconhecido 0)`; `VID-002/scrapers.SelectImages ignora vídeo (MET-025)`.
- Dependência: nenhuma. Arquivo compartilhado: `store.go` só aqui.

## T18 — Store: consultas dos jobs de vídeo
- Requisitos: VID-002, VID-003, VID-006, VID-007, VID-008. **implementer:** `internal/store/queries/media.sql`, `sqlc generate` (arquivos de `internal/store/dbgen/`), `internal/store/video.go` (novo).
```go
const VideoProvider = "screenscraper"
const VideoLookupURL = "screenscraper:lookup"
type VideoGameInfo struct{ Title string; Year int; SeedKey string } // SeedKey "" when the platform has none
type VideoCandidate struct{ GameID int64; SeedKey string; HasVideo bool } // HasVideo: a video row or an active (pending/running) video job
type VideoCounts struct{ Pending, Running, Failed int } // Failed does not count last_error = 'cancelado'
func (s *Store) VideoGameInfo(ctx context.Context, gameID int64) (VideoGameInfo, error)                  // ErrGameNotFound
func (s *Store) VideoCandidates(ctx context.Context, gameIDs []int64) ([]VideoCandidate, error)        // unknown ids absent from the result
func (s *Store) VideoCandidatesIn(ctx context.Context, platformID int64) ([]VideoCandidate, error)     // 0 = whole library; ErrPlatformNotFound
// EnqueueVideoJobs re-checks has-video inside the transaction and inserts one pending job per game that still has none
// (kind video, provider screenscraper, source_url screenscraper:lookup); returns how many it queued.
func (s *Store) EnqueueVideoJobs(ctx context.Context, gameIDs []int64, at time.Time) (int, error)
// CancelVideoJobs marks every pending and running video job failed with "cancelado"; returns the pending count and the ids that were running.
func (s *Store) CancelVideoJobs(ctx context.Context) (pending int, running []int64, err error)
func (s *Store) VideoJobCounts(ctx context.Context) (VideoCounts, error)
```
- **test-writer:** `internal/store/video_test.go` (store real em `t.TempDir()`):
  - `VID-002/jogo com vídeo gravado não entra`; `VID-002/jogo com job pendente ou em andamento não entra; com job falho ou cancelado entra`; `VID-002/dois EnqueueVideoJobs concorrentes do mesmo jogo criam um job só (-race)`;
  - `VID-003/o job tem kind video, provider screenscraper e source_url screenscraper:lookup, sem título nem credencial`; `VID-006/VideoCandidatesIn por plataforma e biblioteca toda; plataforma inexistente é erro`; `VID-006/todos os jogos entram numa transação (falha no meio não deixa job parcial)`;
  - `VID-007/CancelVideoJobs só toca jobs de vídeo, devolve os pendentes e os ids em andamento`; `VID-008/VideoJobCounts conta pending, running e failed, e failed não conta os cancelados`; `VID-002/VideoGameInfo devolve título, ano da data de lançamento e a chave de seed da plataforma`; `VID-002/jogo apagado leva os jobs de vídeo (cascade)`.
- Dependência: T17.

## T19 — `media`: `FetchVideo` (streaming, teto de 20 MB, MP4, hash)
- Requisitos: VID-013, VID-014, VID-025. **implementer:** `internal/media/video.go` (novo), `internal/media/media.go` (só `Stored.SourceURL string`, opcional, achado 3).
```go
const MaxVideoBytes = 20 << 20 // 20 MiB; x-video.maxVideoBytes = 20971520
type PermanentError struct{ Msg string } // the queue fails the job at once, with no new attempt
func (e *PermanentError) Error() string
func Permanent(msg string) error
// FetchVideo streams r into <root>/<gameID>/<rand>.tmp, hashing with SHA-256 on the way (io.TeeReader, never the whole body in memory), reads at most
// MaxVideoBytes+1, checks "ftyp" at bytes 4..8, renames to <sha256>.mp4 (or drops the temp when the file already exists) and removes the temp on any error.
// size is the size announced by the API or by Content-Length (<= 0 = unknown). ThumbPath is "".
func (s *Store) FetchVideo(ctx context.Context, gameID int64, r io.Reader, size int64) (Stored, bool, error)
```
Erros permanentes: `"vídeo maior que 20 MB"` (tamanho informado, ou corpo que passou do teto) e `"formato de vídeo não suportado"`. O `ctx` é respeitado na leitura.
- **test-writer:** `internal/media/video_test.go`, `remove_test.go`:
  - `VID-013/tamanho informado maior que o teto: nada é lido (leitor que falha se tocado) e nenhuma linha`; `VID-013/corpo passa do teto durante a leitura: o download para e o temporário é apagado (pasta sem resto)`; `VID-013/exatamente 20 MiB é aceito`; `VID-013/os três casos são *PermanentError com a mensagem do contrato`;
  - `VID-014/aceita só MP4 (ftyp nos bytes 4..8); outro conteúdo é permanente "formato de vídeo não suportado" e não deixa temporário`; `VID-014/arquivo gravado em <gameID>/<sha256>.mp4 com o hash calculado no caminho`; `VID-014/conteúdo igual já gravado devolve added=false e não duplica arquivo nem deixa temporário`; `VID-014/thumb_path vazio`; `VID-014/contexto cancelado interrompe e apaga o temporário`; `VID-014/leitura via httptest com corpo de 15 MiB usa memória limitada`;
  - `VID-025/RemoveGame apaga a pasta do jogo com o .mp4`.
- Dependência: nenhuma. Arquivo compartilhado: `media.go` só aqui.

## T20 — Scrapers: sistemas, redação de URL e intervalo ajustável
- Requisitos: VID-010, VID-015, VID-020, SSC-004. **implementer:** `internal/scrapers/screenscraper_systems.go` (novo), `internal/scrapers/redact.go` (novo), `internal/scrapers/limiter.go`. Os valores de `rules §1` vêm da T02.
```go
func SystemID(seedKey string) (int, bool) // table of rules §1; switch2 and unknown keys: false
const SoftName = "arcadia"               // x-video.softname
const SSVideoMediaType = "video-normalized"
const IntervalScreenScraper = 1500 * time.Millisecond
func SSInterval(maxPerMinute int) time.Duration // max(1500 ms, ceil(60000/maxPerMinute) ms); <= 0 keeps 1500 ms
func RedactURL(raw string) string       // removes devid, devpassword, ssid, sspassword (any case) from the query; invalid URL -> "<url inválida>"
func RedactError(err error) error       // rebuilds a *url.Error with RedactURL(err.URL); other errors unchanged (wrapped chain kept)
func (l *Limiter) SetInterval(d time.Duration) // applies from the next reservation; nil-safe
```
- **test-writer:** `screenscraper_systems_test.go`, `redact_test.go`, `limiter_test.go`:
  - `VID-010/SystemID devolve o id de cada uma das 13 chaves de rules §1 e false para switch2, vazio e chave desconhecida`;
  - `VID-020/RedactURL remove os 4 parâmetros em qualquer caixa e mantém os outros, a ordem e o fragmento`; `VID-020/valores com %-escape e parâmetros repetidos`; `VID-020/URL sem query não muda; URL inválida vira "<url inválida>"`; `VID-020/RedactError reescreve *url.Error (inclusive embrulhado com %w) sem credenciais e mantém errors.Is(context.Canceled)`;
  - `VID-015/SSInterval: 1500 ms por padrão, 20 por minuto = 3000 ms, 100 por minuto = 1500 ms, 0 ou negativo = 1500 ms`; `VID-015/Limiter.SetInterval vale a partir da próxima reserva e não reabre slots já reservados (relógio fake)`; `VID-015/SetInterval em Limiter nulo não faz nada`; `SSC-004/SoftName é arcadia`; `VID-015/IntervalScreenScraper = 1500 ms (x-video.requestIntervalMs)`.
- Dependência: T02 (tabela de ids). Paralelo às T17..T19.

## T21 — Scrapers: cliente `ScreenScraper`
- Requisitos: VID-010, VID-015, VID-016, VID-017, VID-020, SSC-004. **implementer:** `internal/scrapers/screenscraper.go` (novo). Os endpoints e os campos seguem `screenscraper-api-notes.md` já preenchido (T02).
```go
type SSCreds struct{ DevID, DevPassword, User, Password string }
type SSQuota struct{ Known bool; MaxThreads, MaxPerMinute, RequestsToday, MaxPerDay, KoToday, MaxKoPerDay int }
type SSName struct{ Region, Text string }
type SSMedia struct{ Type, URL, Region, Format string; Size int64 } // Size 0 = unknown
type SSGame struct{ ID string; Names []SSName; SystemID int; Years []int; Medias []SSMedia }
var (
	ErrNoCredentials = errors.New("scrapers: credenciais do ScreenScraper ausentes")
	ErrCredentials   = errors.New("scrapers: credenciais do ScreenScraper recusadas")    // 403, 426
	ErrQuota         = errors.New("scrapers: cota do ScreenScraper esgotada")            // 430, 431, or requeststoday >= maxrequestsperday
	ErrNotFound      = errors.New("scrapers: jogo não encontrado no ScreenScraper")      // 404, "non trouvée"
	ErrBadRequest    = errors.New("scrapers: pedido recusado pelo ScreenScraper")        // 400
)
type SSConfig struct {
	BaseURL string; HTTP *http.Client; Limiter *Limiter; Log *slog.Logger
	Creds func() (SSCreds, error) // read on every request; ErrNoCredentials when one is missing
}
type ScreenScraper struct{ /* cfg, mu (one request at a time), last SSQuota */ }
func NewScreenScraper(cfg SSConfig) *ScreenScraper
func (s *ScreenScraper) SearchGames(ctx context.Context, systemID int, term string) ([]SSGame, SSQuota, error)
func (s *ScreenScraper) GameInfo(ctx context.Context, systemID int, gameID string) (SSGame, SSQuota, error)
// OpenVideo opens mediaURL with the credentials appended in memory when it lacks them; the size is Content-Length (-1 unknown).
func (s *ScreenScraper) OpenVideo(ctx context.Context, mediaURL string) (io.ReadCloser, int64, error)
func (s *ScreenScraper) Quota() SSQuota
```
Toda requisição passa por `Limiter.Wait` e depois ajusta `SetInterval(SSInterval(MaxPerMinute))`; falhas temporárias (401, 423, 429, 5xx, rede, prazo) viram `ErrUnavailable`; um erro de rede é reembrulhado com `RedactError`; um log nunca recebe URL crua. Nenhum parâmetro de ROM (`crc`, `md5`, `sha1`, `romnom`, `romtaille`) é enviado. Uma requisição por vez (`mu`, `maxthreads` 1); no download, o `mu` cobre só o pedido, e a raia de 1 trabalhador garante que o corpo não se sobrepõe.
- **test-writer:** `internal/scrapers/screenscraper_test.go` (`httptest`, relógio fake, `Limiter` real com relógio fake):
  - `SSC-004/toda requisição leva softname=arcadia e output=json`; `VID-010/a busca envia systemeid e recherche e nenhum parâmetro de ROM (crc, md5, sha1, romnom, romtaille)`; `VID-010/GameInfo envia systemeid e gameid`; `VID-010/decodifica noms, systeme, dates e medias das respostas gravadas`;
  - `VID-015/busca, informações e download passam pelo Limiter (instantes crescentes, relógio fake)`; `VID-015/maxrequestspermin menor ajusta o intervalo para a próxima requisição`; `VID-015/duas chamadas simultâneas nunca estão em voo ao mesmo tempo`;
  - `VID-016/430 e 431 viram ErrQuota`; `VID-016/requeststoday ≥ maxrequestsperday na última resposta: a próxima chamada devolve ErrQuota sem requisição`; `VID-016/Quota devolve a última leitura`;
  - `VID-017/403 e 426 viram ErrCredentials`; `VID-017/credencial ausente (Creds devolve ErrNoCredentials): ErrNoCredentials sem requisição`; `VID-015/401, 423, 429, 5xx, rede e prazo viram ErrUnavailable`; `VID-015/404 e 200 com corpo não JSON "non trouvée" viram ErrNotFound`; `VID-015/400 vira ErrBadRequest e registra Warn com a URL limpa`;
  - `VID-020/valores marcados de devid, devpassword, ssid e sspassword não aparecem no log capturado, no erro devolvido nem no texto de OpenVideo`; `VID-020/o erro de rede traz a URL sem as credenciais`; `VID-020/OpenVideo acrescenta as credenciais em memória e devolve Content-Length`; `VID-010/jogo sem medias na busca: GameInfo preenche`; `SSC-004/credenciais lidas a cada requisição (trocar vale na seguinte)`.
- Dependência: T02, T20.

## T22 — Chaves do ScreenScraper e `provider.*`
- Requisitos: SSC-001, SSC-005, VID-017. **implementer:** `internal/scrapers/keys.go`, `internal/agent/scrape.go`, `internal/agent/scrape_providers.go`.
```go
// keys.go
func (k *Keys) SetPair(provider, id, secret string) error // "id\nsecret" in arcadia/<provider>; empty id or secret -> ErrNoKey
func (k *Keys) Pair(provider string) (id, secret string, err error)
// scrape_providers.go: keyProviderNames = igdb, steamgriddb, screenscraper, screenscraperdev; providerNames (metadata) does NOT change
// scrape.go: setKey requires secret (1..512 after trim) for igdb, screenscraper and screenscraperdev; a hook on scrapeService
//   videoCredsChanged func() called after a successful setKey/deleteKey of a screenscraper* provider (set by the wiring, nil = none)
```
- **test-writer:** `internal/scrapers/keys_test.go`, `internal/agent/scrape_test.go`, `scrape_providers_test.go`, `metadata_test.go`:
  - `SSC-001/provider.setKey screenscraper (usuário e senha) e screenscraperdev (devid e devpassword) gravam em arcadia/screenscraper e arcadia/screenscraperdev com valor id\nsenha`; `SSC-001/sem secret, secret só de espaços ou com 513 runas: -32602 e nada gravado`; `SSC-001/keyStatus e deleteKey aceitam os dois e nunca devolvem o valor`; `SSC-001/Pair devolve o id e a senha; entrada sem \n é ErrKeyNotFound`; `SSC-001/igdb e steamgriddb continuam como antes`;
  - `VID-017/setKey e deleteKey de screenscraper e screenscraperdev chamam o gancho de credenciais; os de igdb não`;
  - `SSC-005/validProvider("screenscraper") é falso: metadata refs, scrape.search, scrape.apply e media.queue recusam com -32602`; `SSC-005/a fábrica de provedores de metadados não oferece screenscraper`; `SSC-005/metadata.search, metadata.apply e o lote nunca chamam o fake do ScreenScraper (contador zero) e a aplicabilidade da fase 12 não muda`.
- Dependência: T03. Arquivos compartilhados: `scrape.go` e `scrape_providers.go` só aqui (a T26 liga o gancho em `app.go`).

## T23 — Agente: resolvedor de vídeo (`videoResolver`)
- Requisitos: VID-010, VID-011, VID-012, VID-013, VID-014, VID-020. **implementer:** `internal/agent/video_resolver.go` (novo).
```go
type videoGames interface{ VideoGameInfo(ctx context.Context, gameID int64) (store.VideoGameInfo, error) }
type ssAPI interface {
	SearchGames(ctx context.Context, systemID int, term string) ([]scrapers.SSGame, scrapers.SSQuota, error)
	GameInfo(ctx context.Context, systemID int, gameID string) (scrapers.SSGame, scrapers.SSQuota, error)
	OpenVideo(ctx context.Context, mediaURL string) (io.ReadCloser, int64, error)
	Quota() scrapers.SSQuota
}
type videoFiles interface{ FetchVideo(ctx context.Context, gameID int64, r io.Reader, size int64) (media.Stored, bool, error) }
type videoResolver struct{ /* games, ss, files, log */ }
func newVideoResolver(g videoGames, ss ssAPI, files videoFiles, log *slog.Logger) *videoResolver
func (r *videoResolver) Fetch(ctx context.Context, job store.MediaJob) (media.Stored, bool, error) // Stored.SourceURL = RedactURL(media url)
func (r *videoResolver) Quota() scrapers.SSQuota
// pure, tested with the vectors of rules §3:
func pickSSGame(term string, year int, seedKey string, systemID int, games []scrapers.SSGame) (best *scrapers.SSGame, c domain.Confidence)
func pickVideoMedia(g scrapers.SSGame) (scrapers.SSMedia, bool) // rules §4: type video-normalized, region order wor, us, eu, ss, jp, none, response order; format != mp4 skipped
```
Passos: jogo (`VideoGameInfo`; `ErrGameNotFound` → permanente) → `SystemID(seedKey)` (sem chave ou sem sistema: permanente `"plataforma sem vídeo no ScreenScraper"`, sem requisição) → `SearchGames(domain.SearchTerm(título))` → um grupo de `domain.Member` por jogo com um membro por nome (`PlatformKeys = [seedKey]` quando `SystemID` confere, `Year` = menor ano) → `domain.ScoreGroup` e `ClassifyMatch` (só `match` segue; `doubt`/`none` → permanente `"sem correspondência no ScreenScraper"`) → `GameInfo` só se a busca não trouxe `medias` → mídia (sem `video-normalized`: permanente `"sem vídeo no ScreenScraper"`) → confere `size` (> `MaxVideoBytes`: permanente `"vídeo maior que 20 MB"` antes de abrir) → `OpenVideo` → confere `Content-Length` → `FetchVideo`. Erros de rede passam por `scrapers.RedactError`. Nenhum arquivo do jogo é aberto, nenhum hash é enviado.
- **test-writer:** `internal/agent/video_resolver_test.go` (`httptest` via cliente real e fakes de `ssAPI`):
  - vetores de rules §3, um por linha: `VID-011/Super Mario World (USA) × Super Mario World e Yoshi's Island: match no primeiro`; `VID-011/Resident Evil × Resident Evil 2 e Resident Evil: match no certo`; `VID-011/Mega Man X × Mega Man: sem correspondência`; `VID-011/Legend of Zelda, The - Ocarina of Time × The Legend of Zelda: Ocarina of Time: match`; `VID-011/Final Fantasy VII com dois nomes regionais no mesmo jogo: match`; `VID-011/Tetris × dois Tetris homônimos: sem correspondência (margem < 0,05)`; `VID-011/lista vazia: sem correspondência`; `VID-011/doubt e none são *media.PermanentError e nunca baixam`; `VID-011/o ano do jogo entra na pontuação`; `VID-011/sistema diferente do pedido não confirma a plataforma`;
  - `VID-010/o termo é domain.SearchTerm do título e o sistema é o da chave de seed`; `VID-010/plataforma sem chave de seed ou sem sistema: permanente "plataforma sem vídeo no ScreenScraper", zero requisição`; `VID-010/nenhum arquivo do jogo é aberto (fs espião) e nenhum hash é enviado`; `VID-010/jogo apagado: permanente`;
  - `VID-012/só video-normalized; o video original nunca é baixado`; `VID-012/ordem de região wor, us, eu, ss, jp, sem região, ordem da resposta`; `VID-012/format diferente de mp4 é pulado`; `VID-012/sem video-normalized: permanente "sem vídeo no ScreenScraper"`; `VID-012/GameInfo só é chamado quando a busca não trouxe medias`;
  - `VID-013/size informado acima do teto: nada é aberto, permanente "vídeo maior que 20 MB"`; `VID-013/Content-Length acima do teto: corpo não lido`; `VID-014/o corpo vai a FetchVideo e Stored.SourceURL é a URL da mídia sem credenciais`;
  - `VID-020/valores marcados de credencial não aparecem em erros, em Stored.SourceURL nem no log capturado, em nenhum dos caminhos (sucesso, 404, 403, rede)`.
- Dependência: T05? não; T18 (`VideoGameInfo`), T19 (`PermanentError`, `FetchVideo`), T20, T21.

## T24 — Agente: raia de vídeo na fila de mídia
- Requisitos: VID-007, VID-008, VID-009, VID-016, VID-017, VID-018, VID-019, VID-021, VID-020. **implementer:** `internal/agent/media_queue.go`.
```go
// mediaFetcher now receives the job (the router picks image or video):
type mediaFetcher interface{ Fetch(ctx context.Context, job store.MediaJob) (media.Stored, bool, error) }
type imageFetcher struct{ files *media.Store }                  // Fetch -> files.Fetch(ctx, job.GameID, job.SourceURL)
type fetchRouter struct{ images, video mediaFetcher }          // kind == "video" -> video
const ( videoConcurrency = 1; videoQuotaPause = time.Hour )
type videoLane struct{ /* guarded by q.mu: kind none|quota|credentials, until time.Time, warned bool */ }
// Wiring points (set once before Run):
func (q *mediaQueue) ConfigureVideo(ready func() bool, quota func() scrapers.SSQuota) // ready = both credentials exist
func (q *mediaQueue) VideoCredentialsChanged()                                       // clears the credentials pause and Notify
func (q *mediaQueue) VideoLaneStatus() (lane string, until time.Time)               // active | paused-quota | paused-credentials | no-credentials
func (q *mediaQueue) CancelVideo(ctx context.Context) (cancelled int, err error)    // store.CancelVideoJobs + cancels the active video job contexts
```
Comportamento: `dispatch` separa por `provider == "screenscraper"` (um semáforo de 1) do resto (os 2 atuais); com a raia cheia ou pausada, **`continue`** (não `return`), então um vídeo nunca atrasa imagens (o `return` atual só vale para a raia de imagens). Classificação do erro de `Fetch` por `errors.As`/`errors.Is`: `*media.PermanentError` → `failed` na hora (tentativa gasta, mensagem no `last_error`); `ErrQuota` → pausa de `videoQuotaPause`, job de volta a `pending` **sem** gastar tentativa, `Warn "cota do ScreenScraper esgotada"` uma vez por pausa; `ErrCredentials` → pausa de credenciais até `VideoCredentialsChanged`, `Warn "credenciais do ScreenScraper recusadas"` uma vez; `ErrNoCredentials` ou `ready() == false` no despacho → `no-credentials`, sem requisição e sem gastar tentativa, a raia volta sozinha quando `VideoCredentialsChanged` chega; o resto → regra MID-008 (30 s, 3 tentativas). Cota preventiva: depois de cada job, `quota().RequestsToday ≥ MaxPerDay` (quando `Known`) pausa a raia. `nextDeadline` inclui o fim da pausa por cota. `media.changed` só no `added = true` (já é o caminho existente). O `Notify` da fila continua não bloqueante.
- **test-writer:** `internal/agent/media_queue_test.go` (o fake de `mediaFetcher` passa à assinatura nova), novo `internal/agent/video_lane_test.go` (relógio fake, fetchers bloqueáveis):
  - `VID-009/100 jobs de vídeo pendentes: um download de vídeo por vez e um job de imagem novo começa na hora`; `VID-009/as duas vagas das imagens continuam e não são ocupadas pelo vídeo`; `VID-009/raia pausada (cota, credencial, sem credencial) não atrasa as imagens`; `VID-009/o roteador manda kind video ao resolvedor e o resto ao media.Store`;
  - `VID-016/ErrQuota: job volta a pending sem gastar tentativa, Warn uma vez por pausa, lane paused-quota com pausedUntil, nenhuma requisição durante a pausa e a raia tenta de novo depois de 1 h (relógio fake)`; `VID-016/requestsToday ≥ maxRequestsPerDay pausa antes do próximo job`; `VID-017/ErrCredentials: pending sem gastar tentativa, pausa até VideoCredentialsChanged, Warn uma vez`; `VID-018/rede, 429 e 5xx: nova tentativa em 30 s, até 3 tentativas, depois failed`; `VID-019/credencial ausente: nenhuma requisição, jobs pendentes sem gastar tentativa, lane no-credentials e a raia volta sozinha quando as duas passam a existir`; `VID-018/*media.PermanentError vira failed na primeira tentativa com a mensagem`;
  - `VID-021/job de vídeo que grava (added=true) envia media.changed uma vez; falha, cancelamento e pausa não avisam`; `VID-007/CancelVideo cancela os pendentes, interrompe o job em andamento (contexto) e devolve a soma; jobs de imagem não mudam`; `VID-008/VideoLaneStatus nos 4 estados`; `VID-020/last_error e o log de um job falho com credenciais marcadas não as contêm`; `VID-009/a raia não deixa goroutine depois de Run (-race)`.
- Dependência: T18, T19, T21 (os erros), T22 (o gancho). Arquivo compartilhado: `media_queue.go` e `media_queue_test.go` só aqui.

## T25 — Agente: serviço `video.*` e busca automática depois de aplicar metadados
- Requisitos: VID-002, VID-003, VID-004, VID-005, VID-006, VID-007, VID-008, VID-025. **implementer:** `internal/agent/video.go` (novo), `internal/agent/metadata.go` (um gancho em `commitApply`), `internal/agent/app.go` não (a T26 liga).
```go
const ( maxVideoQueueGames = 1000 ) // x-video.maxQueueGames
type videoStore interface { /* VideoCandidates, VideoCandidatesIn, EnqueueVideoJobs, VideoJobCounts, VideoGameInfo */ }
type videoQueue interface { Notify(); CancelVideo(ctx context.Context) (int, error); VideoLaneStatus() (string, time.Time) }
type videoService struct{ st videoStore; keys *scrapers.Keys; q videoQueue; quota func() scrapers.SSQuota; clock domain.Clock; log *slog.Logger }
func videoHandlers(v *videoService) map[string]ipc.Handler // video.queue, video.queueMissing, video.cancelPending, video.status
func (v *videoService) missingCredential() (provider string, missing bool) // screenscraperdev first, then screenscraper
func (v *videoService) maybeQueue(ctx context.Context, gameID int64)       // VID-004: silent; Debug naming the missing condition; never an error
```
Validação e ordem: `-32602` (1..1.000 ids distintos ≥ 1; `platformId ≥ 1`) antes de qualquer leitura → jogos (`-32001`) → credenciais (`-32010` com `data.provider`) → escrita. Por jogo: sem chave de seed ou `SystemID` falso → `skipped{platform-unmapped}`; com vídeo ou job ativo → `skipped{has-video}`; senão entra. `video.queueMissing` conta `skipped` como inteiro. `maybeQueue` (chamado em `commitApply` depois do `queueImages`, só quando o resultado não mudou: não toca no `MetadataApplyResult`) exige as duas credenciais, sistema mapeado e VID-002; nada é notificado e `queue.Notify()` só se enfileirou. `video.status` junta `store.VideoJobCounts`, `VideoLaneStatus`, `no-credentials` calculado na hora e a cota do resolvedor (`requestsToday`/`maxRequestsPerDay` só quando `Known`).
- **test-writer:** `internal/agent/video_test.go` (store real, fila falsa), `metadata_apply_test.go` e `metadata_batch_apply_test.go` (os casos novos):
  - `VID-005/queue cria um job por jogo e lista has-video e platform-unmapped em skipped (lista nunca nula)`; `VID-005/id inexistente: -32001 e nada criado`; `VID-005/falta devid ou devpassword: -32010 com data.provider screenscraperdev; só falta a conta do usuário: screenscraper; nada criado`; `VID-005/lista vazia, mais de 1.000, repetida ou id < 1: -32602 antes de qualquer leitura`; `VID-005/credenciais conferidas depois de validar e antes de escrever`;
  - `VID-006/queueMissing cria jobs para todos os jogos da plataforma (ou da biblioteca) sem vídeo, com plataforma mapeada; skipped é inteiro`; `VID-006/plataforma inexistente: -32001`; `VID-006/credencial ausente: -32010`;
  - `VID-007/cancelPending cancela pendentes e o em andamento e devolve a soma; jobs de imagem não mudam`; `VID-008/status traz pending, running, failed, lane, pausedUntil só em paused-quota e a cota só quando conhecida`;
  - `VID-002/jogo com vídeo ou com job pendente ou em andamento não ganha job novo e nada é apagado`; `VID-003/o job tem source_url screenscraper:lookup, sem título nem credencial, e o jogo é lido na hora do download`;
  - `VID-004/metadata.apply com as duas credenciais, plataforma mapeada e VID-002 livre cria um job de vídeo; o resultado de metadata.apply é o mesmo de antes`; `VID-004/cada condição que falta (devid, conta, plataforma sem sistema, já tem vídeo) não cria e registra Debug`; `VID-004/a aplicação do lote cria um job por jogo aplicado`; `VID-004/nenhuma notificação nova é enviada`; `VID-025/CancelGame do jogo interrompe o job de vídeo em andamento antes de apagar a pasta (fetcher bloqueado), e <mediaRoot>/<G> some com o .mp4`; `CON-029/nenhum teste da fase usa Sleep ou rede`.
- Dependência: T03, T18, T22, T23, T24. Arquivo compartilhado: `metadata.go` e os testes de apply só aqui nesta trilha.

## T26 — Agente: ligação da trilha B em `app.go` e invariantes
- Requisitos: VID-020, VID-021, VID-025, SSC-005, CON-026. **implementer:** `internal/agent/app.go` (só os pontos da trilha B).
  - `config` ganha `ScreenScraperURL string` (testes apontam para `httptest`; vazio = URL real).
  - Cria `scrapers.NewScreenScraper(SSConfig{BaseURL, HTTP: httpClient, Limiter: scrapers.NewLimiter(clock, scrapers.IntervalScreenScraper), Creds: <lê keys.Pair("screenscraperdev") e keys.Pair("screenscraper"); ausente = ErrNoCredentials>, Log: log})`.
  - Cria `videoResolver` e `fetchRouter{images: imageFetcher{files}, video: resolver}` e passa a `newMediaQueue` no lugar do `media.New(...)` atual.
  - `queue.ConfigureVideo(ready, resolver.Quota)`.
  - Cria `videoService` e faz `maps.Copy(handlers, videoHandlers(...))`.
  - Liga `scrapeSvc.videoCredsChanged = queue.VideoCredentialsChanged` e `metadata.video = videoSvc` (o gancho de `commitApply`).
  - Shutdown: sem mudança de ordem (a raia usa as goroutines por job da fila, esperadas pelo `a.wg`).
- **test-writer:** `internal/agent/app_test.go`, `order_test.go`, novo `video_wire_test.go`:
  - `VID-021/dois clientes IPC reais recebem media.changed quando o vídeo é gravado (ScreenScraper em httptest com MP4 pequeno)`; `VID-021/falha, cancelamento e pausa não avisam`;
  - `VID-020/fluxo completo (aplicar metadados, busca, download, erro 403, cota 431, cancelamento, status) com devid, devpassword, usuário e senha de valor marcado: o valor não aparece no log capturado, nas linhas de media_job, em media.source_url, nas respostas IPC nem em video.status`; `VID-025/game.delete com job de vídeo em curso: o download é interrompido antes de apagar a pasta e o .mp4 some`;
  - `SSC-005/o agente não consulta o ScreenScraper em metadata.search, metadata.apply e no lote (servidor httptest com contador zero)`; `CON-026/o agente registra os 4 handlers video.* e continua com todos os outros`; `CON-026/x-video do contrato: maxQueueGames, laneWorkers, quotaPauseMs e requestIntervalMs iguais às constantes do agente e softname/mediaType/kindLimit/maxVideoBytes às de scrapers, domain e media`; `VID-016/Shutdown com job de vídeo em andamento não deixa goroutine (-race)`; `CON-029/a suíte da trilha não faz rede nem Sleep`.
- Dependência: T22, T23, T24, T25 e a T14 (mesmo `app.go`: sequencial, nunca em paralelo).

## T27 — Desktop: bindings `Video*` e `.mp4` servido com `Range`
- Requisitos: CON-027, VID-005, VID-006, VID-007, VID-008, VID-022. **implementer:** `cmd/arcadia/video.go` (novo), `cmd/arcadia/media_serve.go`.
```go
type VideoSkipView struct{ GameID int64 `json:"gameId"`; Reason string `json:"reason"` }
type VideoQueueView struct{ Queued int `json:"queued"`; Skipped []VideoSkipView `json:"skipped"` } // never nil
type VideoQueueMissingView struct{ Queued int `json:"queued"`; Skipped int `json:"skipped"` }
type VideoStatusView struct {
	Pending, Running, Failed int
	Lane string
	PausedUntil string // RFC 3339 or ""
	RequestsToday, MaxRequestsPerDay int // -1 when absent
} // camelCase json
func (a *App) VideoQueue(gameIDs []int64) (VideoQueueView, error)
func (a *App) VideoQueueMissing(platformID int64) (VideoQueueMissingView, error) // 0 = whole library (omitted from the wire)
func (a *App) VideoCancelPending() (int, error)
func (a *App) VideoStatus() (VideoStatusView, error)
// media_serve.go: for .mp4 (any case) set "Content-Type: video/mp4" before http.ServeFile (which already answers Range with 206).
```
Timeouts: `libraryTimeout`; erros pelo `callAgent` (UI-008). `MediaView.Kind` passa `video` sem mudança de código.
- **test-writer:** `cmd/arcadia/video_test.go`, `media_serve_test.go`, `metadata_test.go`:
  - `CON-027/cada binding Video* chama o método IPC certo com os parâmetros certos (um caso por binding; platformId 0 não vai no fio)`; `VID-005/VideoQueue converte skipped e devolve lista vazia, não nula`; `VID-008/VideoStatus converte pausedUntil para RFC 3339 e usa -1 para cota ausente`; `CON-027/erro do agente (-32010, -32602, -32001) sai com a mensagem de UI-008`; `CON-027/ProviderKeyStatus, ProviderSetKey e ProviderDeleteKey aceitam screenscraper e screenscraperdev e repassam key e secret`; `CON-027/nenhum binding de vídeo chama lançamento, sessão ou pausa`;
  - `VID-022/GET /media/<G>/<hash>.mp4 responde Content-Type video/mp4`; `VID-022/Range bytes=0-99 responde 206 com Content-Range e o corpo parcial`; `VID-022/fora da pasta, pasta e arquivo ausente continuam 404`; `VID-022/imagens continuam com o tipo de antes`; `CON-027/MediaView.Kind passa video`.
- Dependência: T03. Arquivo compartilhado: `cmd/arcadia/media_serve.go` só aqui; `cmd/arcadia/app.go` não é tocado.

## T28 — Frontend: `lib/video.ts`
- Requisitos: VID-024, SSC-003, CON-024. **implementer:** `frontend/src/lib/video.ts` (novo), `frontend/src/lib/metadata.ts` (`KeyProviderId` ganha `'screenscraper' | 'screenscraperdev'`; sem tocar no que a T05 fez).
```ts
export type VideoLane = 'active' | 'paused-quota' | 'paused-credentials' | 'no-credentials'
export type VideoStatusView = { pending: number; running: number; failed: number; lane: VideoLane; pausedUntil: string; requestsToday: number; maxRequestsPerDay: number }
export type VideoSkipReason = 'has-video' | 'platform-unmapped'
export type VideoQueueView = { queued: number; skipped: { gameId: number; reason: VideoSkipReason }[] }
export function videoQueue(gameIds: number[]): Promise<VideoQueueView>
export function videoQueueMissing(platformId: number): Promise<{ queued: number; skipped: number }>
export function videoCancelPending(): Promise<number>
export function videoStatus(): Promise<VideoStatusView>
export function queueResultLabel(v: VideoQueueView): string  // "Vídeo na fila" | "Este jogo já tem vídeo ou busca em andamento" | "A plataforma não tem vídeo no ScreenScraper"
export function queuedLabel(n: number): string                // "N vídeos na fila"
export function videoStatusLine(s: VideoStatusView): string   // "N pendentes · M sem vídeo" + "cota esgotada até HH:MM" | "credenciais recusadas" | "R de D requisições hoje"
export function useVideoCredentials(): { ready: boolean | null } // both providerKeyStatus; null while loading; key ['provider-keys']
export function useVideoStatus(open: boolean): UseQueryResult<VideoStatusView> // refetchInterval 5000 while open; media:changed invalidates; interval cleared on close
```
- **test-writer:** `frontend/src/lib/video.test.ts` (temporizadores falsos):
  - `VID-024/cada função chama o binding certo e falha sem binding`; `VID-024/queueResultLabel: queued 1 → "Vídeo na fila"; has-video; platform-unmapped`; `SSC-003/queuedLabel singular e plural`; `SSC-003/videoStatusLine: só pendentes e sem vídeo; com paused-quota mostra "cota esgotada até HH:MM"; com paused-credentials mostra "credenciais recusadas"; com cota conhecida mostra "R de D requisições hoje"; no-credentials`; `SSC-003/useVideoStatus relê a cada 5 s com a janela aberta, para ao fechar e relê a cada media:changed`; `VID-024/useVideoCredentials é true só com as duas`; `CON-024/os hooks só chamam leituras e Video*`.
- Dependência: T05. Arquivo compartilhado: `lib/metadata.ts` e `metadata.test.ts` (T05 e depois esta).

## T29 — Frontend: seção ScreenScraper em `ProviderKeysDialog`
- Requisitos: SSC-002, SSC-003. **implementer:** `frontend/src/components/ScreenScraperSection.tsx` (novo), `frontend/src/components/ProviderKeysDialog.tsx` (monta a seção; `Modal size="medium"`; `unknownStatus` ganha os dois provedores; salvar invalida `['provider-keys']`).
  - Seção "ScreenScraper (vídeos)" com "Conta do usuário" (Usuário, Senha) e "Desenvolvedor do Arcadia" (devid, devpassword): cada grupo com "Cadastrada"/"Não cadastrada", Salvar (ativo com os dois campos preenchidos) e Remover; campos `type="password"` e `autoComplete="off"`; esvaziados depois de salvar.
  - Com as duas cadastradas: "Buscar vídeos dos jogos sem vídeo" (`videoQueueMissing(0)` e `queuedLabel`), "Cancelar buscas pendentes" (`videoCancelPending`) e a linha de `videoStatusLine` (hook da T28).
- **test-writer:** `ProviderKeysDialog.test.tsx` (os casos de IGDB e SteamGridDB continuam):
  - `SSC-002/mostra a seção com os dois grupos e os rótulos Usuário, Senha, devid e devpassword`; `SSC-002/cada grupo mostra Cadastrada ou Não cadastrada`; `SSC-002/Salvar só com os dois campos preenchidos; chama ProviderSetKey com o provedor certo (screenscraper e screenscraperdev), a chave e o secret`; `SSC-002/campos type password, autocomplete off e vazios depois de salvar`; `SSC-002/Remover chama ProviderDeleteKey do grupo certo`; `SSC-002/o valor das credenciais nunca é exibido`;
  - `SSC-003/os botões de vídeo só aparecem com as duas cadastradas`; `SSC-003/"Buscar vídeos…" chama VideoQueueMissing(0) e mostra "N vídeos na fila"`; `SSC-003/"Cancelar buscas pendentes" chama VideoCancelPending`; `SSC-003/a linha de estado traz os textos de cada situação`; `SSC-003/relê o estado a cada 5 s com a janela aberta (temporizadores falsos), a cada media:changed, e limpa o intervalo ao fechar`; `SSC-003/erro do binding mostra a mensagem de UI-008`.
- Dependência: T28. Arquivo compartilhado: `ProviderKeysDialog.tsx` só aqui.

## T30 — Frontend: "Buscar vídeo" no painel
- Requisitos: VID-024. **implementer:** `frontend/src/components/GameDetailsPanel.tsx`.
  - Botão "Buscar vídeo" visível quando `useVideoCredentials().ready === true` e o jogo (`useMedia(gameId)`) não tem item `video`; chama `videoQueue([id])` e mostra `queueResultLabel`; erros seguem UI-008. Sem credenciais, ou com vídeo, o botão não aparece.
- **test-writer:** `GameDetailsPanel.test.tsx`:
  - `VID-024/com as duas credenciais e sem vídeo o botão aparece`; `VID-024/sem credenciais, ou com vídeo, não aparece`; `VID-024/chama VideoQueue([id]) uma vez e mostra "Vídeo na fila"`; `VID-024/mostra "Este jogo já tem vídeo ou busca em andamento" e "A plataforma não tem vídeo no ScreenScraper"`; `VID-024/erro do agente mostra a mensagem`; `VID-024/o botão fica desativado durante a chamada`.
- Dependência: T05, T28. Arquivo compartilhado: `GameDetailsPanel.tsx` e seu teste (T05, T30 e T42, nessa ordem).

---

# Trilha C — Big Box novo com temas (frontend)

## T31 — Registro de temas e preferência salva
- Requisitos: BBX-023, BBX-024. **implementer:** `frontend/src/bigbox/themes/registry.ts` (novo), `frontend/src/lib/uiPrefs.ts`.
```ts
export type BigBoxThemeId = 'faixa' | 'mosaico' | 'roda'
export type BigBoxThemeProps = { model: BigBoxViewModel; details: JSX.Element; onPick(i: number): void; onPlay(i: number): void; onDetails(): void } // contracts/bigbox-faixa.md §5
export type BigBoxTheme = { id: BigBoxThemeId; name: string; available: boolean; axis: 'horizontal' | 'vertical'; Component?: (p: BigBoxThemeProps) => JSX.Element }
export const BIGBOX_THEMES: BigBoxTheme[]            // faixa (available), mosaico and roda (not available, no Component)
export function resolveTheme(id: string | null | undefined): BigBoxTheme // unknown/unavailable -> faixa
// uiPrefs.ts
export function readBigBoxTheme(storage?: Pick<Storage, 'getItem'>): BigBoxThemeId  // key arcadia.bigBoxTheme
export function writeBigBoxTheme(id: BigBoxThemeId, storage?: Pick<Storage, 'setItem'>): void
```
`BigBoxViewModel` vem de `viewModel.ts` (T34); declarar o tipo em `bigbox/types.ts` (novo, só tipos de [bigbox-faixa §4](contracts/bigbox-faixa.md)) nesta tarefa para não criar dependência circular.
- **test-writer:** `uiPrefs.test.ts`, `themes/registry.test.ts`:
  - `BBX-023/o registro tem faixa, mosaico e roda; só faixa disponível; mosaico e roda sem componente`; `BBX-023/valor ausente, desconhecido ou de tema indisponível vira faixa`; `BBX-023/a escolha fica em localStorage na chave arcadia.bigBoxTheme`; `BBX-023/storage indisponível não quebra`; `BBX-024/cada tema declara o eixo (faixa horizontal)`; `BBX-024/resolveTheme devolve o tema de teste vertical quando registrado (tema falso)`.
- Dependência: nenhuma. Arquivo compartilhado: `lib/uiPrefs.ts` só aqui.

## T32 — Tokens `--arc-bb-*` e verificação de contraste
- Requisitos: BBX-025, BBX-026. **implementer:** `frontend/src/index.css` (bloco `[data-bigbox-theme="faixa"]` com os 10 tokens de [bigbox-faixa §6](contracts/bigbox-faixa.md), expostos ao Tailwind por `@theme inline` como os da janela). Os valores são escolhidos para passar nos testes abaixo. A regra `.big-box` antiga só sai na T42.
- **test-writer:** `frontend/src/theme.test.ts`, `theme-scan.test.ts`:
  - `BBX-025/cada um dos tokens (bg, bg-2, scrim, text, text-muted, accent, on-accent, surface, focus, danger) tem valor sob [data-bigbox-theme="faixa"] e utilitário em @theme inline`; `BBX-025/a varredura de UIX-006 passa a incluir frontend/src/bigbox/ (nenhuma classe de cor da paleta nem cor literal)`;
  - `BBX-026/text, text-muted e danger ≥ 4,5:1 sobre o scrim composto sobre branco e sobre preto`; `BBX-026/on-accent sobre accent ≥ 4,5:1`; `BBX-026/text sobre surface ≥ 4,5:1`; `BBX-026/focus ≥ 3:1 contra o scrim composto nos dois casos`; `BBX-026/o scrim tem alfa ≥ 0,72 na zona do texto`.
- Dependência: nenhuma (a varredura de `bigbox/` só falha se um arquivo novo violar). Arquivo compartilhado: `index.css` (T32 e depois T42).

## T33 — Reducer puro de navegação `bigBoxNav`
- Requisitos: BBX-002, BBX-005, BBX-006, BBX-007, BBX-013, BBX-014, BBX-016, BBX-018, BBX-024, BBX-027, CON-022. **implementer:** `frontend/src/bigbox/nav.ts` (novo). Tabela de [bigbox-faixa §3](contracts/bigbox-faixa.md).
```ts
export type NavState = { screen: 'browse' | 'details'; tab: number; focus: number; detailsFocus: 'play' | 'playUntracked' | 'description'; gallery: number
  suspended: boolean; awaitingFocusAfterBlur: boolean; sound: boolean; videoFailed: number | null; launchedGameId: number | null }
export type NavAction =
  | { type: 'prev' | 'next' | 'up' | 'down' | 'tabPrev' | 'tabNext' | 'accept' | 'details' | 'back' | 'toggleSound' | 'windowBlur' | 'windowFocus' }
  | { type: 'launched'; gameId: number } | { type: 'sessionEnded'; gameId: number }
  | { type: 'gameClosed'; gameId: number; indexInTab: number | null } | { type: 'listLoaded'; initialIndex: number | null }
  | { type: 'videoFailed'; gameId: number }
export type NavCtx = { tabs: number; total: number; loaded: number; galleryCount: number; focusedGameId: number | null; videoVisible: boolean }
export type NavEffect = { kind: 'launch' | 'launchUntracked'; gameId: number } | { kind: 'exit' } | { kind: 'fetchNextPage' } | { kind: 'bringToFront' }
export const PREFETCH_BEFORE_END = 10
export const initialNav: NavState
export function bigBoxNav(s: NavState, a: NavAction, c: NavCtx): { state: NavState; effects: NavEffect[] }
```
O reducer não conhece o tema (o eixo é aplicado na entrada, T35). `gameClosed` não é bloqueado pela suspensão: sai de Detalhes, solta a suspensão, foca `indexInTab` se não nulo e pede `bringToFront`.
- **test-writer:** `frontend/src/bigbox/nav.test.ts` (um caso por linha da tabela §3):
  - `BBX-006/prev e next movem o foco de um em um e não dão a volta nas pontas`; `BBX-007/next com foco ≥ carregado − 10 pede fetchNextPage; antes disso não pede`; `BBX-005/tabNext e tabPrev circulares (da última volta a Todos e vice-versa) e foco 0`; `BBX-014/accept em browse com jogo pede launch; sem jogo não pede`; `BBX-016/details abre Detalhes com detailsFocus play e galeria 0`; `BBX-016/up e down percorrem play, playUntracked e description sem dar a volta`; `BBX-016/prev e next movem a galeria nos limites`; `BBX-016/accept em play e playUntracked pede launch e launchUntracked`; `BBX-002/back em browse pede exit; em details volta a browse com o mesmo foco`; `BBX-013/toggleSound só com vídeo visível`; `BBX-018/launched liga suspended e guarda o jogo; qualquer ação de entrada é ignorada`; `BBX-018/windowBlur com suspended marca awaitingFocusAfterBlur; windowFocus só solta depois dele`; `BBX-018/windowFocus sem blur anterior não solta`; `BBX-018/sessionEnded do jogo lançado solta; de outro jogo não`; `CON-022/gameClosed solta a suspensão, sai de Detalhes, foca indexInTab (ou mantém) e pede bringToFront`; `BBX-027/listLoaded na entrada foca o índice do jogo em foco da janela normal ou 0`; `BBX-012/foco muda: videoFailed volta a null`; `BBX-024/tema falso vertical: o mesmo reducer com ações prev e next vindas de ↑/↓ funciona (o eixo é só da entrada)`; `BBX-014/accept repetido não é problema do reducer: a tecla mantida é filtrada na entrada`.
- Dependência: T31 (tipos). Paralelo ao Go.

## T34 — Modelo de exibição e funções puras
- Requisitos: BBX-004, BBX-006, BBX-008, BBX-009, BBX-010, BBX-015, BBX-017, BBX-021. **implementer:** `frontend/src/bigbox/viewModel.ts` (novo), `frontend/src/bigbox/hints.ts` (novo).
```ts
export const SHORT_DESCRIPTION_MAX = 280
export const STRIP_WINDOW = 7
export function buildTabs(catalog: CatalogView | undefined, initialPlatformId: number | null): { tabs: BigBoxTab[]; initialTab: number } // Todos + gameCount > 0, catalog order, text only
export function shortDescription(text: string): string        // <= 280, cut at the last space before the limit + "…"; "" stays ""
export function infoLine(platform: string, year: number | null, genre: string | null): string // omits empty parts with their separator
export function pickBackground(media: MediaItemView[]): string | null // background, screenshot, title, select, banner, cover
export function pickVideo(media: MediaItemView[]): string | null     // first kind video (url)
export function windowAround<T>(items: T[], focus: number): { slice: T[]; offset: number } // at most 15
export function sessionMark(session: SessionView | null, gameId: number): 'running' | 'paused' | null // running and grace -> running
export function hintsFor(screen: 'browse' | 'details', o: { empty: boolean; videoVisible: boolean; detailsFocus?: string }): Hint[] // BBX-015, BBX-021, BBX-013
export function buildViewModel(i: ViewModelInput): BigBoxViewModel
```
- **test-writer:** `viewModel.test.ts`, `hints.test.ts`:
  - `BBX-004/Todos e só as plataformas com gameCount > 0, na ordem do catálogo, só texto`; `BBX-004/a aba inicial é a plataforma da coluna esquerda ou Todos`; `BBX-006/windowAround devolve no máximo 15 itens (foco−7..foco+7) nas pontas e no meio`; `BBX-008/a capa é a mídia cover; sem capa vale a substituta de useCardImages; sem imagem, cartão com o título`; `BBX-009/a linha plataforma · ano · gênero omite partes vazias com o separador e usa o primeiro gênero`; `BBX-009/descrição de 280 caracteres passa inteira; maior é cortada no último espaço com "…"; sem descrição, nada`; `BBX-009/tempo jogado no formato da janela normal, "Nunca jogado" e "Sessões: N"`; `BBX-010/pickBackground na ordem background, screenshot, title, select, banner, cover; sem nenhuma, null (gradiente)`; `BBX-017/sessionMark: running e grace viram "Em execução", paused vira "Pausado", outro jogo nada`;
  - `BBX-015/dicas de Navegar: A Jogar, X Detalhes, LB/RB Plataforma, Select+B Fechar o jogo, B Sair`; `BBX-015/dicas de Detalhes: A sobre o botão focado, B Voltar, Select+B Fechar o jogo`; `BBX-013/"Y Som" só com vídeo visível`; `BBX-021/lista vazia: só "LB/RB Plataforma" e "B Sair"`.
- Dependência: T31, T33.

## T35 — Hook de entrada `useBigBoxInput`
- Requisitos: BBX-006, BBX-014, BBX-019, BBX-024. **implementer:** `frontend/src/bigbox/useBigBoxInput.ts` (novo).
```ts
export const STICK_DEAD_ZONE = 0.6, REPEAT_DELAY_MS = 400, REPEAT_EVERY_MS = 120
export type InputAction = NavAction['type'] | 'scrollUp' | 'scrollDown'
export function mapDirection(axis: 'horizontal' | 'vertical', dir: 'left' | 'right' | 'up' | 'down'): 'prev' | 'next' | 'up' | 'down' | null
export function useBigBoxInput(o: { enabled: boolean; axis: 'horizontal' | 'vertical'; screen: 'browse' | 'details'; onAction: (type: InputAction) => void }): void
```
`requestAnimationFrame` lendo o primeiro gamepad do mapeamento padrão; borda de subida por botão; direcional e analógico esquerdo com zona morta e repetição (400 ms, depois 120 ms); analógico direito vertical emite `scrollUp`/`scrollDown`; botão 8 apertado → B (1) e Y (3) ignorados, **inclusive a borda de subida** que ocorrer com Select apertado. Teclado: `keydown` sem `repeat` para Enter (A), D (X), PageUp/PageDown (LB/RB), Esc (B) e M (Y); setas com `repeat`. Tudo limpo no desmontar (`cancelAnimationFrame`, `removeEventListener`, temporizadores). Sem modal em Big Box (BBX-003).
- **test-writer:** `useBigBoxInput.test.tsx` (`navigator.getGamepads` falso, `requestAnimationFrame` controlado, temporizadores falsos):
  - `BBX-019/Select apertado: B e Y são ignorados`; `BBX-019/B apertado com Select já apertado não dispara nem ao soltar o Select com B ainda apertado`; `BBX-019/A, direcional, LB, RB e X continuam valendo com Select apertado`; `BBX-019/Y sem Select liga o som`;
  - `BBX-006/direcional e analógico esquerdo repetem depois de 400 ms a cada 120 ms; zona morta 0,6`; `BBX-014/Enter com repeat não emite accept de novo; A mantido emite uma vez`; `BBX-024/eixo horizontal: ←/→ viram prev/next; eixo vertical: ↑/↓ viram prev/next (tema falso)`; `BBX-014/teclas D, PageUp, PageDown, Esc e M emitem details, tabPrev, tabNext, back e toggleSound`; `BBX-016/analógico direito emite scrollUp e scrollDown na tela de detalhes`; `BBX-019/desabilitado não lê o controle; desmontar cancela o frame e o listener`.
- Dependência: T33 (tipos).

## T36 — `BackgroundStage`: arte, vídeo depois de 2 s, som
- Requisitos: BBX-010, BBX-011, BBX-012, BBX-013. **implementer:** `frontend/src/bigbox/BackgroundStage.tsx` (novo).
```ts
export const VIDEO_DELAY_MS = 2000, VIDEO_FADE_MS = 400, ART_FADE_MS = 300
export type BackgroundStageProps = { gameId: number | null; artUrl: string | null; videoUrl: string | null; active: boolean; sound: boolean
  failed: boolean; onVideoError(gameId: number): void; onVideoVisible(visible: boolean): void }
```
Um único `<video muted={!sound} loop playsInline autoPlay>` por vez; `setTimeout` de 2.000 ms limpo a cada troca de jogo; trocar de foco desmonta o elemento e remove o `src`; `active = false` (Detalhes, fora do Big Box) nunca toca; `onError` chama `onVideoError`. Transições por CSS (`opacity`, 300 e 400 ms); sem arte, gradiente de tokens `--arc-bb-bg` e `--arc-bb-bg-2`.
- **test-writer:** `BackgroundStage.test.tsx` (temporizadores falsos):
  - `BBX-011/com vídeo e foco parado 2.000 ms um único video muted, loop e playsInline passa a tocar sobre a arte`; `BBX-011/troca de foco antes de 2.000 ms: nada toca e a contagem recomeça no jogo novo`; `BBX-011/troca de foco com vídeo tocando: o vídeo para e o src é removido`; `BBX-011/active=false (Detalhes) nunca toca`; `BBX-011/nunca há mais de um elemento video`; `BBX-011/transições de 300 ms (arte) e 400 ms (vídeo)`;
  - `BBX-012/evento error volta à arte e chama onVideoError; failed=true não tenta de novo até o foco sair do jogo`; `BBX-013/sound=false: muted; sound=true: não muted`; `BBX-013/onVideoVisible(true) só com o vídeo tocando`; `BBX-010/arte trocada faz a transição de 300 ms entre as duas`; `BBX-010/sem arte: gradiente de tokens`; `BBX-011/o elemento é desmontado ao sair do Big Box`.
- Dependência: T31, T34.

## T37 — Tema Faixa e rodapé
- Requisitos: BBX-006, BBX-008, BBX-009, BBX-014, BBX-015, BBX-017, BBX-020, BBX-021. **implementer:** `frontend/src/bigbox/themes/faixa/Faixa.tsx`, `FaixaStrip.tsx`, `FaixaInfo.tsx` (novos), `frontend/src/bigbox/Footer.tsx` (novo), registro do componente em `registry.ts` (só `faixa.Component`). Layout de [bigbox-faixa §1](contracts/bigbox-faixa.md). Só tokens `--arc-bb-*` e classes dos utilitários expostos; nenhuma cor literal.
  - `Faixa` desenha `model` e repassa cliques (`onPick`, `onPlay`, `onDetails`); não lê binding nem assina evento.
  - Barra de abas só texto; faixa de até 15 capas com a do foco maior, contorno `--arc-bb-focus`, selo "Em execução"/"Pausado"; bloco de informações com título, linha de dados, descrição curta, "Tempo jogado · Última vez · Sessões" e os botões Jogar e Detalhes; `role="alert"` acima do rodapé quando `model.alert`; estados vazios ("Nenhum jogo nesta plataforma", "Nenhum jogo na biblioteca").
- **test-writer:** `Faixa.test.tsx`, `Footer.test.tsx`:
  - `BBX-006/no máximo 15 capas no DOM e a em foco marcada`; `BBX-008/capa, substituta e placeholder com o título`; `BBX-009/o bloco mostra título, linha de dados, descrição curta, tempo jogado, última vez, sessões e os botões Jogar e Detalhes`; `BBX-009/partes ausentes somem sem deixar separador`; `BBX-014/clique na capa a põe em foco, duplo clique lança, os botões Jogar e Detalhes chamam onPlay e onDetails`; `BBX-017/selo Em execução e Pausado na capa e ao lado do título`; `BBX-020/o aviso aparece com role alert acima do rodapé e some quando model.alert é null`; `BBX-021/aba vazia: "Nenhum jogo nesta plataforma" e rodapé só com LB/RB e B`; `BBX-021/biblioteca vazia: "Nenhum jogo na biblioteca"`; `BBX-015/o rodapé mostra as dicas do modelo em texto`; `BBX-006/as abas são só texto, sem ícone`.
- Dependência: T31, T32, T34.

## T38 — `BigBoxDetails`
- Requisitos: BBX-016. **implementer:** `frontend/src/bigbox/BigBoxDetails.tsx` (novo).
```ts
export type BigBoxDetailsProps = { game: GameDetailView; media: MediaItemView[]; session: 'running' | 'paused' | null
  detailsFocus: 'play' | 'playUntracked' | 'description'; gallery: number; sound: boolean; scrollSignal: number; onPlay(): void; onPlayUntracked(): void }
```
Título; descrição completa (rolagem por `scrollSignal` e por ↑/↓ quando o foco está nela); plataforma, ano, gêneros, desenvolvedores, editoras e modos de jogo; tempo jogado, última vez e sessões; galeria com o vídeo primeiro (se houver, tocando sem som ao aparecer, sem `src` ao sair) e depois os screenshots, trocada por `gallery`; botões "Jogar" e "Jogar sem contar tempo" com o foco visível. Só tokens.
- **test-writer:** `BigBoxDetails.test.tsx` (temporizadores falsos):
  - `BBX-016/mostra título, descrição completa, plataforma, ano, gêneros, desenvolvedores, editoras e modos de jogo`; `BBX-016/mostra tempo jogado, última vez e sessões`; `BBX-016/a galeria tem o vídeo primeiro e os screenshots depois; sem vídeo, só screenshots`; `BBX-016/o vídeo da galeria toca sem som ao aparecer e perde o src ao sair da galeria`; `BBX-016/gallery seleciona o item`; `BBX-016/os botões Jogar e Jogar sem contar tempo chamam onPlay e onPlayUntracked; o foco atual tem contorno`; `BBX-016/scrollSignal rola a descrição`; `BBX-016/jogo sem descrição e sem mídia não quebra`.
- Dependência: T31, T32, T34. Paralelo à T36 e à T37.

## T39 — Dados do Big Box (`useBigBoxData`)
- Requisitos: BBX-004, BBX-007, BBX-017, BBX-027, BBX-028, BBX-029. **implementer:** `frontend/src/bigbox/useBigBoxData.ts` (novo).
```ts
export const PAGE_SIZE = 200
export function useTabGames(platformId: number | null): { games: GameView[]; total: number; loaded: number; fetchNextPage(): void; isFetchingNextPage: boolean; isPending: boolean; error: Error | null }
export function useFocusedData(gameId: number | null): { detail: GameDetailView | undefined; media: MediaItemView[]; session: SessionView | null }
```
`useInfiniteQuery(['bigbox', platformId])` sobre `ListGames` (filtro só de plataforma, ordem por título, páginas de 200); jogo em foco por `GetGame` (`['game', id]`), `MediaList` (`['media', id]`, a chave de IMG-008) e `CurrentSession`; ao chegar `media:changed` (já invalida `['media', id]`) e `session:changed` com fim, relê o jogo. **Só leituras**; nenhuma chamada de download.
- **test-writer:** `useBigBoxData.test.tsx` (bindings falsos):
  - `BBX-004/o catálogo vira abas; a aba inicial segue a plataforma da janela normal`; `BBX-007/a lista usa só o filtro de plataforma, ordem por título e páginas de 200`; `BBX-007/a próxima página é pedida uma vez e sem pedido repetido em voo`; `BBX-007/o fim da faixa é o total da aba`; `BBX-027/o jogo em foco da janela normal vira o foco inicial quando está carregado; senão o primeiro`; `BBX-017/session:changed e CurrentSession marcam o jogo em Running, Grace e Paused`; `BBX-028/media:changed do jogo em foco relê a mídia (arte, capa e vídeo)`; `BBX-028/o fim da sessão relê o jogo (tempo, última vez, sessões)`; `BBX-029/só ListGames, GetGame, MediaList, Catalog e CurrentSession são chamados; nenhum VideoQueue, MediaQueue ou escrita`.
- Dependência: T05 (tipo `video`), T31, T34. Paralelo à T36..T38.

## T40 — Shell `BigBox.tsx`, eventos, lançamento e suspensão
- Requisitos: BBX-001, BBX-002, BBX-003, BBX-004, BBX-014, BBX-018, BBX-020, BBX-021, BBX-024, BBX-027, BBX-029, CON-022, CON-024. **implementer:** `frontend/src/bigbox/BigBox.tsx` (novo), `frontend/src/bigbox/useBigBoxEvents.ts` (novo), `frontend/src/lib/bigbox.ts` (acrescenta `bringToFront()`; a remoção do antigo é a T42).
```ts
export type BigBoxProps = { initialPlatformId: number | null; initialGameId: number | null; themeId: BigBoxThemeId; onExit(): void }
export function BigBox(p: BigBoxProps): JSX.Element // data-bigbox-theme on the root; reducer + input + data + theme + BackgroundStage + Footer
export function useBigBoxEvents(o: { dispatch: (a: NavAction) => void; launchedGameId: number | null; active: boolean }): void
// window blur/focus; session:changed (ended/discarded of the launched game -> sessionEnded); game:closedByHotkey -> gameClosed
export function bringToFront(): Promise<void> // window.go.main.App.BringToFront; failures ignored
```
Lançamento por `useLaunchGame`/`useLaunchGameUntracked` (um por ação, bloqueado enquanto há um pendente, UI-021); sucesso → `launched`; erro (`-32002`, `-32003`, desconectado) → `model.alert` com o texto de UI-008 até a próxima ação. Ao receber `game:closedByHotkey` com o Big Box ativo: `bringToFront()`, `enterBigBox()` e `gameClosed` (foco no jogo se está na aba); inativo: o evento nem chega (o shell só assina montado). Nenhum modal e nenhum binding de escrita além de lançamentos.
- **test-writer:** `BigBox.test.tsx`, `useBigBoxEvents.test.tsx` (runtime do Wails falso, bindings espiões):
  - `BBX-001/o Big Box monta o tema escolhido na tela Navegar com data-bigbox-theme`; `BBX-002/B em Navegar chama onExit; B em Detalhes volta a Navegar`; `BBX-004/LB e RB trocam a aba e a lista passa a ser a da aba, com o foco no primeiro`; `BBX-014/A lança uma vez (UIX-046); um lançamento pendente bloqueia outro`; `BBX-014/X abre Detalhes e A em Jogar sem contar tempo chama LaunchGameUntracked`; `BBX-018/depois do lançamento a entrada fica suspensa até session:changed ended/discarded do jogo, game:closedByHotkey, ou focus depois de blur`; `BBX-018/A e B apertados dentro do jogo não lançam nem saem`; `BBX-020/lançamento recusado com -32002, -32003 ou agente desconectado mostra o aviso com role alert e o Big Box segue navegável`; `BBX-021/biblioteca vazia e aba vazia mostram os textos e as dicas reduzidas`; `BBX-024/um tema falso de eixo vertical navega com ↑/↓ pelo mesmo reducer`; `BBX-027/o foco inicial é o jogo da janela normal quando carregado`; `BBX-029/navegar (abas, foco, Detalhes, galeria, som) só faz leituras`;
  - `CON-022/game:closedByHotkey com o Big Box ativo chama BringToFront e enterBigBox, sai de Detalhes, libera a suspensão e foca o jogo (na aba); jogo de outra aba mantém o foco`; `CON-022/com o Big Box inativo nada acontece (BringToFront não é chamado)`; `CON-022/gameId de outro jogo vale o mesmo retorno`; `CON-024/o shell não tem ação de fechar jogo nem de encerrar sessão: os bindings chamados são só leituras, LaunchGame, LaunchGameUntracked e BringToFront`; `BBX-003/nenhum modal abre`.
- Dependência: T05, T33, T34, T35, T36, T37, T38, T39 e, para o `BringToFront` real, a T15 (os testes usam binding falso). Arquivo compartilhado: `lib/bigbox.ts` (T40, depois T42).

## T41 — `App.tsx`: botão "Big Box", seletor de tema e a confirmação sob o Big Box
- Requisitos: BBX-001, BBX-002, BBX-022, BBX-023, BBX-024. **implementer:** `frontend/src/App.tsx`, `frontend/src/components/BigBoxThemeSelect.tsx` (novo).
  - Botão "Big Box" no cabeçalho e F11 entram no Big Box: `enterBigBox()` e, no lugar das três colunas, `<BigBox initialPlatformId initialGameId themeId onExit/>`; ao sair, `exitBigBox()` e a biblioteca de três colunas volta como estava (filtros, busca, jogo em foco, visualização).
  - `BigBoxThemeSelect` ("Tema do Big Box"): lista os três temas, os indisponíveis desativados com "em breve"; a escolha é gravada por `writeBigBoxTheme` e lida por `readBigBoxTheme`.
  - Sai `useBigBoxNav`, a classe `big-box`, o botão "Sair do Big Box (Esc)" e o foco nos `data-bigbox-play`. `anyModalOpen` inclui `bigBox`: com o Big Box ativo a confirmação de metadados não abre sozinha e o indicador pisca (MET-067).
- **test-writer:** `App.test.tsx` (substitui o bloco "Big Box sobre o layout novo" e os casos de `data-bigbox-*`; os de MET-067 ganham o caso novo):
  - `BBX-001/o botão Big Box e F11 mostram o tema escolhido no lugar das três colunas e põem a janela em tela cheia`; `BBX-001/filtros, busca, jogo em foco e visualização voltam iguais ao sair`; `BBX-002/sair restaura a janela (UIX-003) e a biblioteca reaparece`;
  - `BBX-023/o seletor lista Faixa, Mosaico e Roda; os dois últimos desativados com "em breve"`; `BBX-023/a escolha fica em localStorage e vale na próxima entrada; tema indisponível salvo vira Faixa`; `BBX-024/o seletor só aceita temas disponíveis`;
  - `BBX-022/com o Big Box ativo a busca em lote chega a ready e a janela de confirmação não abre sozinha; o indicador pisca; a janela abre pelo indicador depois de sair do Big Box`;
  - `BBX-001/a aba inicial e o foco inicial vêm da janela normal`.
- Dependência: T31, T40. Arquivo compartilhado: `App.tsx` e `App.test.tsx` só aqui (a T43 acrescenta ao `App.invariants.test.tsx`, arquivo diferente).

## T42 — Remoção do Big Box antigo
- Requisitos: BBX-001, BBX-003, BBX-025 (UIX-044 substituído, UIX-045 e UIX-006 alterados). **implementer:** apaga `useBigBoxNav`, `cardGrid`, `gridNext`, `nextFocus`, `pickNext`, `bigBoxItems` e os tipos relacionados de `frontend/src/lib/bigbox.ts` (fica `enterBigBox`, `exitBigBox`, `bringToFront`); remove os atributos `data-bigbox-item`, `data-bigbox-card` e `data-bigbox-play` de `FilterPanel.tsx`, `GameDetailsPanel.tsx`, `GameCard.tsx`, `LibraryList.tsx`, `MediaThumbStrip.tsx`, `PanelEmpty.tsx`, `PlatformNav.tsx`, `SearchBox.tsx` e `StatsPage.tsx`; remove `onOpenGame` com `wantPlayFocus` de `StatsPage` (a parte "no Big Box" deixa de existir); remove `.big-box` e os comentários de `index.css`; `LibraryGrid` e `LibraryList` deixam de receber `keyboard={!bigBox}` (a prop continua, sempre verdadeira).
- **test-writer:** apaga `lib/bigbox.test.tsx` (mantém só os casos de `enterBigBox`/`exitBigBox` e `bringToFront`, num `bigbox.test.ts` novo) e `GameCardBigBox.test.tsx`; ajusta `GameDetailsPanel.test.tsx`, `PlatformNav.test.tsx`, `Sidebar.test.tsx`, `FilterPanel.test.tsx`, `StatsPage.test.tsx`, `LibraryGrid.test.tsx`, `LibraryList.test.tsx` e `App.test.tsx`, removendo os que afirmam `data-bigbox-*` e a navegação antiga:
  - `BBX-003/enterBigBox e exitBigBox continuam: só trocam a tela cheia e não fazem nada fora do desktop`; `BBX-003/bringToFront chama o binding e ignora falha`; `BBX-003/nenhum arquivo de src fora de bigbox/ usa data-bigbox-item, data-bigbox-card ou data-bigbox-play (varredura)`; `BBX-025/index.css não tem mais a regra .big-box`.
- Dependência: T30 (mesmo `GameDetailsPanel.tsx`), T40, T41.

## T43 — Invariantes de UI e varredura do Big Box
- Requisitos: CON-024, CON-003, BBX-003, BBX-025, BBX-029. **test-writer:** `frontend/src/App.invariants.test.tsx`, novo `frontend/src/bigbox/bigbox-scan.test.ts`. **implementer:** só se os testes revelarem uma chamada ou uma cor indevida.
  - `CON-024/Big Box, temas, detalhes, galeria, som, "Buscar vídeo", "Buscar vídeos dos jogos sem vídeo", cancelar buscas, janela dos provedores e o evento game:closedByHotkey: os bindings chamados são só leituras, LaunchGame, LaunchGameUntracked, BringToFront, Video* e ProviderSetKey/ProviderDeleteKey; nenhum ToggleSession, Pause, Resume, nem ação de fechar jogo ou encerrar sessão`; `CON-024/LaunchGame e LaunchGameUntracked uma vez por ação, sem repetição por tecla mantida`; `CON-003/o Big Box não tem botão nem atalho de teclado para fechar o jogo (nenhuma tecla além das do mapa de entrada o aciona)`;
  - `BBX-003/varredura de frontend/src/bigbox/: nenhum import de componentes de estatísticas, emuladores, plataformas, provedores, importação, edição, remoção, metadados nem de bindings de escrita além de LaunchGame, LaunchGameUntracked e BringToFront`; `BBX-025/a varredura de UIX-006 cobre frontend/src/bigbox/ e não acha cor literal nem classe de paleta`; `BBX-029/os arquivos de bigbox/ não importam nenhum binding Video*, MediaQueue nem MetadataApply`.
- Dependência: T28, T29, T30, T40, T41, T42.

---

## T44 — Verificação, rastreio, documentação e revisão do diff
- Requisitos: CON-028, CON-029, CON-026 (e a conferência de que todo ID ativo tem teste). **Responsável:** sessão principal (e o reviewer).
  1. `go build ./...`, `go vet ./...`, `CGO_ENABLED=0 go build ./...`, `go test ./...` e `go test ./... -race` em `internal/tracker`, `internal/input`, `internal/feedback`, `internal/agent`, `internal/scrapers`, `internal/media`, `internal/store`, `internal/ipc` e `cmd/arcadia`, sem internet.
  2. `go test ./internal/ipc/... ./internal/tracker/... ./internal/agent/... ./cmd/arcadia/... -v`, conferindo os casos CON-026 (contrato e "só acréscimo"), CON-005..CON-013 e CON-030..CON-033 (inclusive os de integração Windows) e VID-001.
  3. Frontend em `frontend/`: `npm test`, `npx tsc --noEmit`, `npm run build`; depois `cd cmd/arcadia && wails build -ldflags "-X main.version=<v>"` para regerar `frontend/wailsjs/go/**` (entra no commit; confere `BringToFront`, `Video*` e o tipo de `MediaView`).
  4. `powershell -NoProfile -ExecutionPolicy Bypass -File scripts/spec-trace.ps1 -Spec docs/specs/phase-13-modo-console`, e o mesmo em `phase-4-pause`, `phase-6-importers`, `phase-7-midia`, `phase-8-estatisticas`, `phase-10-nova-interface`, `phase-10-1-ajustes` e `phase-12-metadados-unificados` (requisitos anotados "alterado/substituído em 2026-11-15").
  5. `git diff --stat` da fase (CON-028): em `internal/tracker/` só mudanças do tracker-specialist; `cmd/fake-game` **sem mudança**; só a migração 00009; nenhuma mudança em `play_session`, `pause_interval`, `tracking_rule` nem na tabela `game`; contratos das fases anteriores intactos.
  6. CON-029: `rg "time\.(Sleep)" internal cmd frontend/src` não acha nada novo em arquivos de teste da fase; `rg "time\.Now\(\)" internal/tracker internal/agent/video* internal/scrapers/screenscraper*` não acha lógica testável fora de `domain.Clock`; nenhum teste com rede externa.
  7. Documentação: `docs/ARCHITECTURE.md` (estado `Closing`, job guardado do jogo sem contar tempo, raia de vídeo, Big Box com temas), `docs/ROADMAP.md` (fase 13 concluída no código), `CLAUDE.md` (spec atual e o layout `internal/…`), `docs/adr/README.md`; `screenscraper-api-notes.md` com o que a T45 confirmou.
- Dependência: T01..T43.

## T45 — Conferência manual ponta a ponta
- Requisitos: CON-025, BBX-030, VID-026 (`· verificação: manual`). **Responsável:** sessão principal com o usuário. Pré-requisito: T44 verde, `wails build` gerado, um controle XInput, credenciais do ScreenScraper cadastradas pelo app (nunca em arquivo).
- **Roteiro:**
  1. **Select+B (CON-025).** Segurar Select+B por 1 s: num `.exe` rastreado; no RetroArch com ROM; num jogo da Steam já adotado; num jogo da Steam antes da adoção; num jogo "sem contar tempo"; com a sessão pausada; fora do Big Box e com a UI fechada; com o Big Box ativo. Conferir: o emulador fecha sozinho pelo pedido e mantém o que salva; um jogo que ignora o pedido fecha em até 5 s; a sessão aparece com o tempo até o aperto (±2 s, sem a espera) e "fechado pelo controle"; o tempo pausado não conta; antes da adoção e sem jogo toca o som de recusa e nada fecha; com o Big Box ativo a janela volta em tela cheia com o foco no jogo; nenhum som no sucesso; um toque curto não fecha.
  2. **Vídeo (VID-026).** Aplicar metadados a um jogo de SNES e pedir "Buscar vídeos dos jogos sem vídeo": os `.mp4` (≤ 20 MB) chegam e aparecem no Big Box sem reabrir; `data\media` cresce cerca de 2 MB por jogo; a cota mostrada sobe; nenhuma credencial em `data\logs`; "Cancelar buscas pendentes" funciona; sem credenciais o botão some. Anotar as linhas "NÃO VERIFICÁVEL" de `screenscraper-api-notes.md` que o teste real confirmou ou desmentiu (inclusive se o `Range` do WebView2 funciona, achado 10).
  3. **Big Box na TV (BBX-030).** Troca de plataforma com LB/RB, andar pela faixa, vídeo depois de ~2 s parado e que sai ao mexer, Y liga o som, X abre os detalhes com a galeria, A lança, Select+B fecha o jogo e devolve ao Big Box no mesmo jogo; texto legível a 3 m; trocar o tema no seletor (Mosaico e Roda desativados).
  4. Conferir que a janela normal continua igual (carrossel sem vídeo) e que nada do Big Box cria sessão ou altera tempo.
- Dependência: T44.

---

## Ordem e paralelismo

- **Base:** T01 e T02 (sessão principal) e T03 em paralelo; T04 depois de T01 e T03; T05 em qualquer momento (precede T28, T30, T39, T40).
- **Trilha A (depende só de T03 para o IPC):**
  - em paralelo: T06, T07, T08, T09, T10 (T09 só `machine.go`; T10 só jobs e `launcher.go`);
  - depois T11 (T09, T10), depois T12 (T11);
  - T13 (T03, T06, T07, T08 e os tipos da T11) e T14 (T13, T12) em seguida;
  - T15 e T16 independentes (rodam a qualquer momento depois de T03).
- **Trilha B:**
  - em paralelo: T17, T19, T20 (depois de T02) e T22 (depois de T03);
  - T18 depois de T17; T21 depois de T20 e T02;
  - T23 depois de T18, T19 e T21; T24 depois de T18, T19, T21 e T22; T25 depois de T22, T23 e T24; T26 depois de T25 (e da T14);
  - T27 independente (depois de T03);
  - T28 depois de T05; T29 e T30 depois de T28.
- **Trilha C (frontend puro, sem depender de Go):**
  - T31 e T32 primeiro; T33 (T31); T34 (T31, T33); T35 (T33); T36, T37, T38 e T39 em paralelo depois de T34 (T36 e T39 também esperam T05);
  - T40 (todas as anteriores); T41 (T40); T42 (T30 e T41); T43 (T28, T29, T30, T40, T41, T42).
- **Fechamento:** T44 depois de todas; T45 depois da T44.

## Arquivos compartilhados (nunca em paralelo)

- `internal/tracker/`: `machine.go` e `machine_test.go` só T09; `launcher.go`, `job_windows.go`, `job_other.go`, `fakes_test.go` T10, depois T12 (`fakes_test.go` T10, T11, T12); `tracker.go`, `close.go` e `tracker_close_test.go` T11, depois T12.
- `internal/agent/app.go`, `app_test.go`, `order_test.go`: T14, depois T26. `notifier.go` e `pause.go` só T13. `media_queue.go` só T24. `metadata.go` só T25. `scrape.go`, `scrape_providers.go` só T22.
- `internal/ipc/{console.go,video.go,media.go,protocol.go}` só T03; `contract_test.go`, `contract_phase11_test.go`, `contract_phase12_test.go` só T04.
- `internal/store/store.go` só T17; `queries/media.sql` e `dbgen/*` só T18; `internal/media/media.go` só T19.
- `cmd/arcadia/app.go`, `main_windows.go`, `main_other.go`, `supervisor.go` só T15; `media_serve.go` só T27.
- `frontend/src/lib/metadata.ts` e `.test.ts`: T05, depois T28. `lib/uiPrefs.ts` só T31. `lib/bigbox.ts`: T40, depois T42. `index.css`: T32, depois T42. `App.tsx` e `App.test.tsx` só T41. `GameDetailsPanel.tsx` e `.test.tsx`: T05, T30, T42 (nessa ordem). `ProviderKeysDialog.tsx` só T29. `registry.ts`: T31, depois T37 (só o `Component` do faixa).
- `App.invariants.test.tsx` e `theme-scan.test.ts`: `theme-scan` T32, `App.invariants` e o scan do Big Box só T43.

## Riscos de concorrência e recursos

- **Goroutines (dono e saída):**
  - *Nenhuma nova no tracker.* O fechamento e a espera usam temporizadores `clock.After` no `select` do dono, com geração, como a carência; o dono nunca bloqueia (`CloseByHotkey` aguarda no chamador).
  - *Roteador de gatilhos:* a mesma goroutine de hoje, dona o `triggerRouter`; sai em `Close()`, que cancela o contexto de uma chamada pendente (achado 1) e espera o `WaitGroup`.
  - *Raia de vídeo:* usa as goroutines por job da fila (`wg` do `Run`); sem goroutine nova. `Limiter` e `videoLane` não têm goroutine.
  - *Frontend:* só `requestAnimationFrame` e `setTimeout`/`setInterval`, limpos no desmontar (`useBigBoxInput`, `BackgroundStage`, `useVideoStatus`).
- **Handles do Windows:**
  - `detachedJob`: o único handle é o do Job Object, aberto por `LaunchDetached` e fechado por `Close()` (idempotente: no lançamento seguinte, no `shutdown()` do tracker e no fim de um fechamento). Os handles de processo e de thread são fechados antes de responder `{pid}`. Falha no meio: `TerminateProcess` do suspenso e fecha tudo. Teste de contagem de handles (≤ h0 + 2).
  - `processKiller` (adotado): `OpenProcess` com `defer windows.CloseHandle(h)` logo depois do erro checado.
  - `EnumWindows`/`PostMessageW`: nenhum handle aberto; **um único** callback por pacote (`windows.NewCallback` não libera), com estado de varredura sob mutex de pacote. `AttachThreadInput` sempre desfeito com `defer`, mesmo se o passo do meio falhar.
  - `TerminateJobObject` e `Kill` nunca fecham o job; o `Close` do job nunca mata (ADR 0005).
- **Locks e ordem:**
  - `t.mu` (tracker) só protege o `view`; o dono é o único a tocar `Machine`, `job`, `detached` e `closing`.
  - `mediaQueue.mu` protege `active`, `retryAt`, `changes` e a raia de vídeo, nunca durante E/S.
  - `scrapers.ScreenScraper.mu` cobre uma requisição (cabeçalhos), nunca o corpo do vídeo; o `Limiter` reserva sob mutex e espera por `clock.After`.
  - `commitApply` já roda sob `job.mu` no lote; o `maybeQueue` (T25) só faz leitura e uma escrita curta no store e nunca chama a rede.
- **Shutdown (sem mudar a ordem):** `BeginShutdown` → fecha energia e fontes → `router.Close()` (cancela e abandona o `CloseByHotkey` pendente) → drain → `server.Close()` → `stopMetadata` + `Wait` → `stopQueue` + `wg.Wait` (a raia de vídeo termina com os jobs; o job interrompido volta a `pending`) → `notifier.set(nil)` (todos, inclusive o de fechar) → `Tracker.Close` (o `Stop` em `Closing` grava `agent-shutdown` com `ended_at = at` e **não mata**; `detached` é fechado sem matar) → `coord.Close` → `store.Close`.
- **Fechar o agente nunca mata o jogo** (SES-011, TRK-004): coberto por `CON-033` e `CON-011`. **A UI nunca influencia o tempo:** só lê e lança por ação explícita (CON-024); `BringToFront` é local.
- **Credenciais:** lidas do `KeyStore` a cada job, nunca em campo exportado, log, `media_job.last_error`, `media.source_url` nem resposta IPC (VID-020); `RedactURL`/`RedactError` em todo erro e log do cliente; teste de ponta a ponta com valores marcados (T26).
- **Disco:** ~2 MB por jogo, sem limite total por decisão do usuário; o teto de 20 MB é a trava; `FetchVideo` sempre apaga o temporário.
- **Cota:** raia de 1 trabalhador, intervalo ≥ 1,5 s, pausa de 1 h por cota sem gastar tentativa, `video.cancelPending`; o Big Box nunca dispara download (BBX-029).
- **`Closing` e a UI:** `Launch` durante o fechamento recebe `SessionActive`; a UI já trata `-32002` (UI-008).

## Rastreabilidade

| Requisito | Tarefas |
|---|---|
| CON-001 | T07 |
| CON-002 | T07 |
| CON-003 | T07, T14, T43 |
| CON-004 | T06, T13 |
| CON-005 | T09, T11 |
| CON-006 | T09, T11 |
| CON-007 | T09, T11 |
| CON-008 | T10, T11 |
| CON-009 | T11 |
| CON-010 | T12 |
| CON-011 | T12 |
| CON-012 | T11, T12, T13, T14 |
| CON-013 | T09, T10, T11, T12 |
| CON-014 | T08, T09, T11, T13, T14 |
| CON-015 | T08, T13, T14 |
| CON-016 | T06 |
| CON-017 | T01, T04, T14 |
| CON-018 | T16 |
| CON-019 | T03, T04, T13, T14 |
| CON-020 | T15 |
| CON-021 | T11, T13, T14 |
| CON-022 | T33, T40 |
| CON-023 | T15 |
| CON-024 | T28, T40, T43 |
| CON-025 | T45 (manual) |
| CON-026 | T01, T03, T04, T26 |
| CON-027 | T15, T27 |
| CON-028 | T44 |
| CON-029 | T25, T26, T44 |
| CON-030 | T10, T11, T12 |
| CON-031 | T11, T12 |
| CON-032 | T09, T11 |
| CON-033 | T09, T11, T12 |
| BBX-001 | T40, T41, T42 |
| BBX-002 | T33, T40, T41 |
| BBX-003 | T40, T42, T43 |
| BBX-004 | T34, T39, T40 |
| BBX-005 | T33 |
| BBX-006 | T33, T34, T35, T37 |
| BBX-007 | T33, T39 |
| BBX-008 | T34, T37 |
| BBX-009 | T34, T37 |
| BBX-010 | T34, T36 |
| BBX-011 | T36 |
| BBX-012 | T36 |
| BBX-013 | T33, T34, T36 |
| BBX-014 | T33, T35, T37, T40 |
| BBX-015 | T34, T37 |
| BBX-016 | T33, T38 |
| BBX-017 | T34, T37, T39 |
| BBX-018 | T33, T40 |
| BBX-019 | T35 |
| BBX-020 | T37, T40 |
| BBX-021 | T34, T37, T40 |
| BBX-022 | T41 |
| BBX-023 | T31, T41 |
| BBX-024 | T31, T33, T35, T40, T41 |
| BBX-025 | T32, T42, T43 |
| BBX-026 | T32 |
| BBX-027 | T33, T39, T40 |
| BBX-028 | T39 |
| BBX-029 | T39, T40, T43 |
| BBX-030 | T45 (manual) |
| VID-001 | T17 |
| VID-002 | T17, T18, T25 |
| VID-003 | T18, T25 |
| VID-004 | T25 |
| VID-005 | T25, T27 |
| VID-006 | T18, T25, T27 |
| VID-007 | T18, T24, T25, T27 |
| VID-008 | T18, T24, T25, T27 |
| VID-009 | T24 |
| VID-010 | T02, T20, T21, T23 |
| VID-011 | T23 |
| VID-012 | T02, T23 |
| VID-013 | T02, T19, T23 |
| VID-014 | T19, T23 |
| VID-015 | T02, T20, T21 |
| VID-016 | T02, T21, T24, T26 |
| VID-017 | T02, T21, T22, T24 |
| VID-018 | T24 |
| VID-019 | T24 |
| VID-020 | T20, T21, T23, T24, T26 |
| VID-021 | T24, T26 |
| VID-022 | T27 |
| VID-023 | T05 |
| VID-024 | T28, T30 |
| VID-025 | T19, T25, T26 |
| VID-026 | T45 (manual) |
| SSC-001 | T01, T03, T22 |
| SSC-002 | T29 |
| SSC-003 | T28, T29 |
| SSC-004 | T02, T20, T21 |
| SSC-005 | T22, T26 |

Os 94 requisitos ativos têm tarefa (um requisito foi removido). Os três marcados `· verificação: manual` (CON-025, BBX-030, VID-026) ficam na T45. Lacunas para o spec-writer (nenhuma bloqueia): achados 1 a 3, 6 e 7 do topo; e, se a T02 divergir, os pontos de [rules](contracts/screenscraper-rules.md) §1, §2 e §6.

## Decisões novas que merecem ADR

Nenhuma além de **0035** (fechar o jogo pelo controle), **0036** (Big Box com temas selecionáveis) e **0037** (vídeo do ScreenScraper como mídia `video` na fila), já escritos. Se a T02 ou a T45 revelarem divergência da API que mude uma regra visível (ex.: sem busca por nome, vídeo com outro nome de tipo), registrar no 0037. O único ponto fora do que os ADRs dizem é a abertura do roteador ao cancelamento no `Close()` (achado 1), que é detalhe do 0035 e não pede ADR próprio.

---

# Correções da revisão do diff (2026-11-15)

Acrescentadas pelo spec-writer depois da revisão do diff (T44), sem renumerar nada. Requisitos novos: **VID-027, VID-028, VID-029, SSC-006**; alterado: **VID-009**; CON-023 sem mudança de texto (detalhe em [design §2.5](design.md)). `prefers-reduced-motion` não é requisito desta fase (nenhuma tarefa). Mesmas premissas: test-writer primeiro, implementer depois, uma tarefa por commit, sem rede nem `Sleep`, todo caso começa pelo ID. Ao fim: `go build ./...`, `go vet ./...`, `CGO_ENABLED=0 go build ./...`, `go test ./...` e `-race` em `internal/scrapers` e `internal/agent`.

## T46 — Scrapers: origem da URL de mídia antes das credenciais
- Requisitos: VID-027, VID-020, VID-012, SSC-004. **implementer:** `internal/scrapers/screenscraper.go` (campo `baseURL *url.URL` analisado em `NewScreenScraper`; `OpenVideo` chama a conferência antes de `Creds`, do acréscimo das credenciais e de `do`).
```go
// checkMediaOrigin applies rules §10: https on the BaseURL host or on *.screenscraper.fr; http only when the BaseURL itself is
// http and the host is the BaseURL host. Refusal: fmt.Errorf("%w: URL de vídeo fora do ScreenScraper", ErrBadRequest).
func checkMediaOrigin(base *url.URL, raw string) (*url.URL, error)
```
- **test-writer:** `internal/scrapers/screenscraper_origin_test.go` (novo; tabela com os vetores de [rules §10](contracts/screenscraper-rules.md)), `screenscraper_test.go`, `internal/agent/video_resolver_test.go`:
  - `VID-027/checkMediaOrigin: um caso por linha dos vetores de rules §10 (aceita ou recusa)`;
  - `VID-027/URL recusada: OpenVideo devolve ErrBadRequest com "URL de vídeo fora do ScreenScraper", sem requisição (dois httptest: o do BaseURL e o "alheio" contam zero), sem chamar Creds (contador) e sem reservar o Limiter (relógio fake: a próxima requisição não espera)`;
  - `VID-027/URL recusada: valores marcados de devid, devpassword, ssid e sspassword não aparecem no erro nem no log capturado`;
  - `VID-027/URL aceita (mesmo host do httptest) recebe as quatro credenciais e softname como antes`;
  - `VID-027/http aceito só com BaseURL http e no mesmo host; com BaseURL https um http de *.screenscraper.fr é recusado`;
  - `VID-027/resolvedor: mídia com URL fora vira *media.PermanentError (sem nova tentativa), FetchVideo não é chamado, last_error sem credencial`.
- Dependência: nenhuma (T21 e T23 já feitas). Arquivo compartilhado: `screenscraper.go` e `screenscraper_test.go` (T46, depois T47, depois T48).

## T47 — Scrapers: redirecionamento só no mesmo host
- Requisitos: VID-028, VID-020, VID-015. **implementer:** `internal/scrapers/screenscraper.go` (`NewScreenScraper` copia o `*http.Client` e põe `CheckRedirect`; `netErr` reconhece o sentinela).
```go
var errRedirectOffHost = errors.New("scrapers: redirecionamento para fora do ScreenScraper") // unexported
// sameHostRedirect: follows only when req.URL.Host equals via[0].URL.Host (case-insensitive, with port) and the scheme does not
// go from https to http; keeps the net/http limit of 10. Otherwise errRedirectOffHost, mapped to
// fmt.Errorf("%w: redirecionamento para fora do ScreenScraper", ErrBadRequest).
func sameHostRedirect(req *http.Request, via []*http.Request) error
```
- **test-writer:** `internal/scrapers/screenscraper_redirect_test.go` (novo; dois `httptest`):
  - `VID-028/busca, informações e vídeo redirecionados ao mesmo host são seguidos`;
  - `VID-028/redirecionamento a outro host: o segundo servidor conta zero requisições, o erro é ErrBadRequest com "redirecionamento para fora do ScreenScraper" e não é ErrUnavailable`;
  - `VID-028/redirecionamento de https para http no mesmo host é recusado (sameHostRedirect com requisições montadas, sem rede)`;
  - `VID-028/mais de 10 redirecionamentos no mesmo host continuam falhando como no net/http`;
  - `VID-028/o *http.Client passado em SSConfig.HTTP não é alterado (CheckRedirect continua nil depois de NewScreenScraper)`;
  - `VID-028/valores marcados das credenciais não aparecem no erro nem no log capturado`;
  - `VID-028/resolvedor: redirecionamento recusado no download vira *media.PermanentError sem nova tentativa`.
- Dependência: T46 (mesmo arquivo).

## T48 — Scrapers e agente: 200 `text/*` no download passa pela classificação
- Requisitos: VID-029, VID-014, VID-016, VID-017, VID-018. **implementer:** `internal/scrapers/screenscraper.go` (`OpenVideo`: `mime.ParseMediaType` do `Content-Type`; `text/*` lê até 4.096 bytes, fecha e classifica), `internal/agent/video_resolver.go` (`apiError` traduz `ErrNotVideo`).
```go
var ErrNotVideo = errors.New("scrapers: resposta do ScreenScraper não é vídeo") // text body with no known message
// classifyMediaText(body []byte) error: the text branch of classify (ErrQuota, ErrCredentials, ErrUnavailable, ErrNotFound); otherwise ErrNotVideo.
```
- **test-writer:** `internal/scrapers/screenscraper_test.go`, `internal/agent/video_resolver_test.go`, `internal/agent/video_lane_test.go`:
  - `VID-029/200 text/plain com "Votre quota de scrape est dépassée" vira ErrQuota`; `VID-029/200 text/html com "blacklisté" vira ErrCredentials`; `VID-029/200 text/plain com "API closed" vira ErrUnavailable`; `VID-029/200 text/plain com "non trouvée" vira ErrNotFound`; `VID-029/200 text/plain "NOMEDIA" vira ErrNotVideo`;
  - `VID-029/Content-Type com parâmetros e caixa variada (text/plain; charset=utf-8, TEXT/HTML) é reconhecido`; `VID-029/video/mp4 e Content-Type ausente seguem para o corpo como antes`; `VID-029/corpo de texto lido no máximo até 4.096 bytes e fechado (leitor espião)`;
  - `VID-029/resolvedor: cota no texto pausa a raia (paused-quota) sem gastar tentativa; credencial no texto dá paused-credentials; API fechada segue MID-008`; `VID-029/resolvedor: não encontrado vira permanente "sem vídeo no ScreenScraper" e ErrNotVideo vira permanente "formato de vídeo não suportado"`; `VID-029/FetchVideo nunca é chamado e a pasta de mídia fica sem temporário`.
- Dependência: T47 (mesmo arquivo).

## T49 — Chaves e agente: `id` e `secret` sem `\n` nem `\r`
- Requisitos: SSC-006, SSC-001, VID-017. **implementer:** `internal/scrapers/keys.go`, `internal/scrapers/provider.go` (só o sentinela), `internal/agent/scrape.go`.
```go
var ErrInvalidKey = errors.New("scrapers: chave com quebra de linha") // provider.go
// SetPair: strings.ContainsAny(id, "\r\n") || strings.ContainsAny(secret, "\r\n") -> ErrInvalidKey, nothing written (checked before any trim).
// scrapeService.setKey (screenscraper*): same check before SetPair -> -32602; ErrInvalidKey from SetPair also -> -32602; no notifyVideoCreds.
```
- **test-writer:** `internal/scrapers/keys_test.go`, `internal/agent/scrape_test.go`:
  - `SSC-006/SetPair recusa \n e \r no começo, no meio e no fim do id e do secret com ErrInvalidKey e o KeyStore falso não recebe escrita`;
  - `SSC-006/provider.setKey screenscraper e screenscraperdev com \n ou \r em key ou secret: -32602, nada gravado, a credencial anterior continua (Pair devolve a antiga)`;
  - `SSC-006/secret "senha\n" (só quebra no fim) é recusado, não aparado`; `SSC-006/o gancho de credenciais não é chamado na recusa`;
  - `SSC-006/igdb e steamgriddb não mudam (SSC-001)`.
- Dependência: nenhuma. Arquivo compartilhado: `scrape.go` só aqui nesta seção.

## T50 — Fila: critério único `kind == "video"` no despacho e no roteador
- Requisitos: VID-009, VID-003, VID-007, VID-021. **implementer:** `internal/agent/media_queue.go`.
```go
// isVideoJob is the only lane criterion (VID-009): used by dispatch (which slot) and by fetchRouter.Fetch (which fetcher).
func isVideoJob(job store.MediaJob) bool { return job.Kind == "video" }
```
`CancelVideo` e a contagem de vídeos em andamento (`active` da raia) usam o mesmo critério.
- **test-writer:** `internal/agent/video_lane_test.go`, `media_queue_test.go`:
  - `VID-009/job kind video com provider diferente de screenscraper ocupa a vaga de vídeo e vai ao resolvedor`;
  - `VID-009/job kind cover com provider screenscraper (montado direto no store) ocupa uma vaga de imagem e vai ao media.Store`;
  - `VID-009/com a raia de vídeo pausada, um job de imagem de qualquer provider começa na hora e nenhum job kind video é despachado`;
  - `VID-009/despacho e roteador concordam para cada combinação de kind e provider (tabela)`.
- Dependência: nenhuma.

## T51 — `winapi`: `ForceForeground` com a thread fixa
- Requisitos: CON-023, CON-022. **implementer:** `internal/winapi/foreground_windows.go`.
```go
var ( lockThread = runtime.LockOSThread; unlockThread = runtime.UnlockOSThread ) // test seam
// ForceForegroundWith: lockThread() first; defer unlockThread() registered before the defer of AttachThreadInput(..., false),
// so the detach runs on the same OS thread and the thread is released last.
```
- **test-writer:** `internal/winapi/foreground_windows_test.go`:
  - `CON-023/ForceForegroundWith fixa a thread antes de CurrentThread e a solta depois do AttachThreadInput(..., false) (seams gravando a ordem junto das chamadas da API falsa)`;
  - `CON-023/o desanexar e o soltar rodam mesmo quando BringWindowToTop ou SetForegroundWindow falham`;
  - `CON-023/foreground já é a janela: lock e unlock em par, nenhum AttachThreadInput`;
  - `CON-023/os ids de thread do SO vistos por CurrentThread, pelo anexar e pelo desanexar são iguais (API falsa que lê windows.GetCurrentThreadId)`;
  - `CON-023/chamada a partir de uma goroutine já fixa (LockOSThread no teste) continua fixa depois (contagem aninhada)`.
- Dependência: nenhuma.

## Ordem e rastreabilidade desta seção

- T46 → T47 → T48 (mesmo `screenscraper.go`); T49, T50 e T51 em paralelo entre si e com a cadeia; depois repetir os passos 1, 2 e 4 da T44 (`spec-trace.ps1 -Spec docs/specs/phase-13-modo-console`) e a revisão do diff.
- A T45 (manual) ganha um item: anotar o host real de `medias[].url` (se fora de `*.screenscraper.fr`, volta ao spec-writer, VID-027).

| Requisito | Tarefas |
|---|---|
| VID-009 (alterado) | T24, T50 |
| VID-027 | T46 |
| VID-028 | T47 |
| VID-029 | T48 |
| SSC-006 | T49 |
| CON-023 (sem mudança de texto) | T15, T51 |

Com elas, os requisitos ativos passam a 98 (94 + 4 novos).