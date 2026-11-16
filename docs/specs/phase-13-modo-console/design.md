# Fase 13 — Modo console: design

Requisitos em [requirements.md](requirements.md). Cada seção cita os IDs que atende. ADRs: 0005, 0016, 0017, 0018, 0019 (em
parte substituído, ADR proposto 0035), 0025, 0029, 0030, 0031, 0033, 0034.

## 1. Mapa da mudança

| Pacote | O que muda | Quem escreve |
|---|---|---|
| `internal/input` | `Trigger.Action`; segundo detector (`Back + B`) no poller | implementer |
| `internal/tracker` | `Job.Kill`, `Job.RequestClose` (`WM_CLOSE`); `Tracker.CloseByHotkey`; estado `Closing` na máquina (`CloseRequested`, `CloseDone`, `CloseUndone`); job do lançamento sem rastrear (`detachedJob`); `adoptedJob.Kill`/`RequestClose` | **tracker-specialist** |
| `internal/domain` | `EndClosedByHotkey`; `MediaKindLimit("video") = 1` | implementer |
| `internal/feedback`, `internal/winapi` | `EventRefused`, `CueRefused`; `Beeper` toca o sinal novo; `ForceForeground` | implementer |
| `internal/agent` | roteador de gatilhos com duas ações; notificação `game.closedByHotkey`; serviço de vídeo (`video.*`); raia de vídeo na fila; resolvedor ScreenScraper; provedores de credencial novos | implementer |
| `internal/scrapers` | cliente `ScreenScraper` (busca, informações, URL de vídeo, `redactURL`), sistemas por chave de seed, limitador | implementer |
| `internal/media` | `FetchVideo` (streaming, teto, MP4, hash) | implementer |
| `internal/store` | migração 9 (`video` no `CHECK`); consultas de jobs de vídeo | implementer |
| `internal/ipc` | tipos v0.15.0 | implementer |
| `cmd/arcadia` | `BringToFront`, `Video*`, repasse de `game.closedByHotkey`, `Content-Type`/`Range` do `.mp4` | implementer |
| `frontend` | `src/bigbox/` (shell, reducer, entrada, temas, Faixa, Detalhes, player); seletor de tema; `SessionsList`; painel ("Buscar vídeo"); janela dos provedores | implementer |
| `cmd/fake-game` | nada (os testes usam `--depth` e `--run forever` existentes) | — |

## 2. Fechar o jogo pelo controle [CON-001..CON-025, CON-030..CON-033]

### 2.1 Entrada [CON-001..CON-003]

```go
// internal/input
type Action int
const (
    ActionPause Action = iota // zero value: existing triggers keep their meaning
    ActionClose
)
type Trigger struct {
    Source domain.PauseReason // PauseHotkey | PauseGamepad
    Action Action
    At     time.Time
}
const ButtonB uint16 = 0x2000
```

- O `pad` passa a ter dois `comboDetector` (máscaras `Back|Y` e `Back|B`), lidos na mesma leitura; cada um entrega seu
  gatilho. `PadConfig.Mask` continua sendo a máscara da pausa (testes antigos inalterados); `PadConfig.CloseMask` (zero = padrão)
  é a nova. Os dois detectores são independentes (CON-002), então `Back + Y + B` dispara os dois, cada um uma vez por pressão.
- O atalho de teclado só produz `ActionPause` (CON-003).
- A fila de saída é a mesma (`triggerQueueSize = 8`); fila cheia descarta com `Warn` (PAD/HOT).

### 2.2 Roteador [CON-004, CON-012, CON-014, CON-015, CON-019, CON-021]

O `pauseRouter` vira `triggerRouter` (mesma goroutine dona, mesma fila) com duas janelas de antirrepique, uma por ação
(`lastPause`, `lastClose`). Para `ActionClose`:

```go
type closeTracker interface {
    CloseByHotkey(ctx context.Context, at time.Time) (tracker.CloseResult, error)
}
const closeTimeout = 6 * time.Second // x-console.closeHandlerTimeoutMs: 3 s of wait (CON-030) + kill + EndSession (2 s) + margin
```

O roteador espera a resposta (a goroutine dele fica ocupada até 6 s; gatilhos que chegarem nesse meio ficam na fila de 8 e são
atendidos depois, com o próprio `At`).

| Resultado do tracker | Roteador |
|---|---|
| `CloseClosed{GameID, Untracked}` | `notifier.GameClosedByHotkey(GameID)`; sem feedback |
| `CloseRefused{Reason}` (`no-game`, `not-adopted`) | `feedback.Send(EventRefused)`; `Debug` |
| `CloseRefused{closing}` | `Debug`, sem som [CON-032] |
| `CloseFailed` | `feedback.Send(EventRefused)` (o `Warn` é do tracker) |
| `ErrShuttingDown` | `Debug`, nada |
| `ctx` vencido | `Warn` "tracker não respondeu ao fechar a tempo"; sem som (o tracker ainda pode aplicar) |

A notificação sai do roteador **depois** de o tracker responder, e o tracker só responde depois de gravar o fim e chamar
`Notify` (`session.changed`), então a ordem `session.changed` → `game.closedByHotkey` vale (CON-019). O notificador é o mesmo de
`media.changed` (broadcast, fila por conexão com descarte, IPC-017).

### 2.3 Tracker (tracker-specialist) [CON-005..CON-013, CON-030..CON-033]

Contrato completo em [contracts/session-close-state-machine.md](contracts/session-close-state-machine.md).

**Decisão de tempo (resposta 2 da aprovação):** o tempo ativo é cortado em `at`, o instante do gatilho; a espera do
`WM_CLOSE` (até 3 s) e a morte não contam. A máquina congela o tempo ao entrar em `Closing` e grava `ended_at = at` ao sair.
Justificativa em [requirements, decisão b](requirements.md).

```go
// internal/tracker
type CloseKind int
const (
    CloseClosed CloseKind = iota + 1
    CloseRefused
    CloseFailed
)
type CloseResult struct {
    Kind      CloseKind
    GameID    int64
    Untracked bool   // Closed: the game had no session (CON-010)
    Reason    string // Refused: "no-game" | "not-adopted" | "closing"
}

// CloseByHotkey asks the game in progress to close (WM_CLOSE), waits up to
// closeWait for the tree to end, kills what is left, and ends its session at
// `at` with reason closed-by-hotkey [CON-005..CON-013, CON-030..CON-033].
// The reply comes when the close is over; the owner goroutine is never blocked.
func (t *Tracker) CloseByHotkey(ctx context.Context, at time.Time) (CloseResult, error)

const (
    closeWait         = 3 * time.Second        // x-console.closeWaitMs
    detachedPollEvery = 250 * time.Millisecond // x-console.closeDetachedPollMs
)

type Job interface {
    // ... existing methods ...
    // RequestClose posts WM_CLOSE to the visible top-level windows of the live
    // processes of the tree and returns how many windows got it [CON-030].
    RequestClose() (int, error)
    // Kill terminates every process of the tree, after Resume; nil when there is
    // nothing left to kill. It never closes the job.
    Kill() error
}

// windowCloser is the Win32 seam of RequestClose (fake in tests).
type windowCloser interface {
    PostClose(pids map[uint32]struct{}) (posted int, err error)
}
```

- **Fluxo no dono:** novo `ctrlClose` em `ctrlCh`, com canal de resposta de buffer 1 (o dono nunca bloqueia se o roteador
  desistiu). `handleClose(at)`:
  1. shutdown → `ErrShuttingDown`;
  2. `t.closing != nil` (fechamento em espera, de sessão ou do jogo sem contar tempo) → `CloseRefused{closing}` [CON-032];
  3. Running/Paused com `at ≥ startedAt` → `t.step(work, func() { return t.m.CloseRequested(monotonic(at)) })` (entra em
     `Closing`, tempo congelado, nada gravado); `n, err := t.job.RequestClose()`; `n == 0` ou `err` (`Debug`) → `finishClose`
     na hora; senão guarda `t.closing = &closing{at, reply, gen}` e arma `clock.After(closeWait)` com a geração;
  4. Grace → sem pedido nem espera: `Kill` (sem efeito); ok → `t.step(work, func() { return t.m.CloseRequested(monotonic(at)) })`,
     que em Grace já devolve o fim (Ended, `ended_at = graceSince`) → `CloseClosed`; erro → `CloseFailed` [CON-007, CON-013];
  5. Launching (URI sem adoção) → `CloseRefused{not-adopted}`;
  6. Idle com `t.detached != nil` → `ActiveProcesses()`; = 0 → `Close`, `nil`, `CloseRefused{no-game}`; > 0 (ou erro) →
     `RequestClose`; `n == 0` → `Kill` na hora; senão `t.closing` + `clock.After(closeWait)` + `clock.After(detachedPollEvery)`
     (rearmado a cada disparo enquanto `ActiveProcesses() > 0`);
  7. resto (inclusive `at < startedAt`) → `CloseRefused{no-game}`.
- **Fim da espera:** `TrackedEmpty` (no job, `ACTIVE_PROCESS_ZERO`; no adotado, o aviso de "nenhum adotado vivo") em `Closing`
  ou `ActiveProcesses() == 0` no detached → `finishClose` sem matar. `closeTimeout` com a geração atual → `Kill()`; ok →
  `finishClose`; erro → `Warn` "falha ao fechar o jogo", `t.m.CloseUndone()` (volta ao estado de origem; Running com
  `runningSince = at`), `CloseFailed` [CON-013]. Disparos de geração antiga são ignorados.
- **`finishClose`:** sessão → `t.step(work, func() { return t.m.CloseDone(monotonic(now)) })` (`EndSession` com
  `ended_at = at` ou `graceSince`, `EffCloseJob`, `EffNotify` com `AsOf = now`) → `CloseClosed`; detached → `Close`,
  `t.detached = nil` → `CloseClosed{Untracked}`. Responde pelo canal guardado e zera `t.closing`.
- **Lançamentos durante a espera** → `SessionActive` (sessão: a dela; detached: `sessionId = 0`, `gameId` e título guardados com
  o job) [CON-031]. **Pausa, `Suspend`, `Wake`, heartbeat** em `Closing`: ignorados; `session.pause`/`resume` devolvem a sessão
  atual; `session.current` devolve a sessão no estado de origem com o tempo de `at` [CON-032].
- **`Stop` durante a espera:** não mata; a máquina sai de `Closing` com `EndSession(ended_at = at, accumulated, agent-shutdown)`;
  o canal guardado recebe `ErrShuttingDown`; detached é fechado sem matar [CON-033].
- **Máquina:** `CloseRequested(at)`: Running → `accumulated += at − runningSince`; Paused → `accumulated` congelado; guarda
  `closeAt = at` e `closeFrom`. `CloseDone(now)`: `EffEndSession{EndedAt: closeAt, Reason: EndClosedByHotkey, State: Outcome}`
  (Paused: o `EndSession` fecha a pausa em `closeAt`, STO-021), `EffCloseJob`, `EffNotify{EndedAt: closeAt, AsOf: now}`.
  `CloseRequested(at)` em Grace vai direto a Ended (`ended_at = graceSince`, geração da carência zerada, `AsOf = at`), sem
  passar por `Closing`. `CloseUndone()`: volta a `closeFrom`
  (Running: `runningSince = closeAt`, para o tempo da espera contar, já que o jogo não fechou). Sem `EffFeedback` (CON-014).
- **`RequestClose`:** `winJob` e `detachedJob` leem os PIDs por `QueryInformationJobObject(JobObjectBasicProcessIdList)` (buffer
  que cresce se `NumberOfAssignedProcesses > NumberOfProcessIdsInList`); `adoptedJob` usa o snapshot novo e o conjunto conferido
  de CON-008. O `windowCloser` do Windows chama `EnumWindows` com **um único callback criado uma vez no pacote**
  (`windows.NewCallback` não libera callbacks; criar um por chamada esgotaria o limite do runtime) e, para cada janela de topo
  com `IsWindowVisible`, sem dono (`GetWindow(GW_OWNER) == 0`) e `GetWindowThreadProcessId` no conjunto, chama
  `PostMessageW(hwnd, WM_CLOSE, 0, 0)`. Nunca `SendMessage` (um jogo travado bloquearia o dono). Nenhum handle é aberto.
- **`winJob.Kill`:** `TerminateJobObject(job, 1)` sob `j.mu`; recusado se `closed`; permitido depois de `Resume` (o `Terminate`
  atual continua só para lançamento falho). Os eventos do job que chegarem depois são descartados pelo `Close` em seguida.
- **`adoptedJob.Kill`:** snapshot novo pelo `ProcessLister`; para cada PID de `live` que no snapshot novo **ainda casa**
  (`matchesAny`), abre com `PROCESS_TERMINATE | PROCESS_QUERY_LIMITED_INFORMATION`, compara `QueryFullProcessImageName` com o
  caminho do snapshot e chama `TerminateProcess(h, 1)`; `defer windows.CloseHandle(h)` logo depois do erro checado. O
  terminador fica atrás de uma interface (`processKiller`) para teste sem Win32. Erros juntados com `errors.Join`; qualquer erro →
  `CloseFailed`. O `ErrNotOwned` de `Terminate` continua (é o caminho de lançamento falho).
- **Jogo sem contar tempo (`detachedJob`):** `Launcher.LaunchDetached(spec) (DetachedJob, uint32, error)`:
  `CreateProcess(CREATE_SUSPENDED | CREATE_DEFAULT_ERROR_MODE)` → `CreateJobObject` com `BREAKAWAY_OK` → `AssignProcessToJobObject`
  → `ResumeThread` → fecha `hThread` e `hProcess`. Falha em qualquer passo: `TerminateProcess` do suspenso, fecha tudo,
  `*LaunchError`. `DetachedJob` = `{ ActiveProcesses() (int, error); RequestClose() (int, error); Kill() error; Close() error }`,
  dono único: o campo `t.detached` da goroutine dona. Um lançamento novo fecha o anterior antes de guardar o novo; `shutdown()`
  fecha o guardado. Nenhuma goroutine nova, nenhuma porta de conclusão. Handles: só o do job, fechado por `Close` (idempotente).
- **Concorrência:** tudo na goroutine dona; a espera é por temporizadores do `clock` no `select` do dono (como a carência), nunca
  por bloqueio. `CloseByHotkey` usa `t.control` (envio só abandonado se o dono acabou; resposta aguardada até o `ctx`). O
  roteador tem prazo de 6 s: 3 s de espera + `Kill` + `EndSession` (até `writeTimeout`, 2 s). Waiters (`BlockUntilWaiters`):
  `Closing` soma 1 (o prazo); o detached em espera soma 2 (prazo e conferência).

### 2.4 Domínio, store e feedback [CON-015..CON-018]

- `domain.EndClosedByHotkey SessionEndReason = "closed-by-hotkey"`, aceito por `ParseSessionEndReason` (sem migração).
- `feedback.EventRefused` (valor 3) → `sound.Play(CueRefused)` sem `overlay.Show` e **sem** rearmar o temporizador de esconder o
  aviso (um aviso de pausa visível continua seu prazo). `tracker.FeedbackKind` não ganha valor: quem pede a recusa é o roteador.
- `winapi.Beeper.signal(CueRefused)` = `tone(300, 100)` + `tone(200, 200)`.
- `SessionsList`: `endReason === 'closed-by-hotkey'` → sufixo "· fechado pelo controle". `SessionEndReason` (TS) ganha
  `'client-no-launch' | 'closed-by-hotkey'`.

### 2.5 Volta ao Big Box [CON-020, CON-022, CON-023]

- **Desktop:** o supervisor decodifica `game.closedByHotkey` (`{gameId}` inteiro ≥ 1) e emite `game:closedByHotkey`.
- **`BringToFront()` (binding, `cmd/arcadia/window_windows.go`):** acha a janela principal com `EnumWindows` filtrando
  `GetWindowThreadProcessId == GetCurrentProcessId()`, visível, a de maior área; `winapi.ForceForeground(hwnd)`:
  `IsIconic → ShowWindow(SW_RESTORE)`; `SetWindowPos(HWND_TOPMOST, SWP_NOMOVE|SWP_NOSIZE)` e depois `HWND_NOTOPMOST`;
  `SetForegroundWindow`; se `GetForegroundWindow() != hwnd`, `AttachThreadInput(fg, self, true)` → `BringWindowToTop` →
  `SetForegroundWindow` → `AttachThreadInput(..., false)`. Devolve `bool` só para o `Debug`. A chance boa: logo depois de o jogo
  morrer **não há processo em primeiro plano**, e o Windows aceita o pedido.
- **Thread fixa (revisão do diff, 2026-11-15; CON-023):** `AttachThreadInput` liga a fila de entrada da thread **do SO** que
  chamou `GetCurrentThreadId`; se o runtime trocar a goroutine de thread entre o anexar e o desanexar, o desanexar vai para outra
  thread e a primeira fica presa à fila do primeiro plano. `ForceForegroundWith` faz `runtime.LockOSThread()` no começo e
  `defer runtime.UnlockOSThread()` registrado **antes** do `defer` do desanexar (então desanexa e só depois solta a thread),
  cobrindo `CurrentThread`, os dois `AttachThreadInput`, `BringWindowToTop` e `SetForegroundWindow`. O par é aninhável (Go conta
  os `LockOSThread`), então chamar de uma thread já fixa (o laço da janela) é seguro. Seam para teste: variáveis de pacote
  `lockThread`/`unlockThread` (padrão `runtime.LockOSThread`/`runtime.UnlockOSThread`).
- **Frontend:** o shell do Big Box assina `game:closedByHotkey`; se o Big Box está ativo: `BringToFront()`, `enterBigBox()`,
  `dispatch({type: 'gameClosed', gameId})` (sai de Detalhes, libera a suspensão, foca o jogo se estiver na aba). Big Box inativo:
  ignora.

### 2.6 Riscos

- **PID reaproveitado** no processo adotado: coberto pela segunda conferência de caminho (CON-008).
- **Kill parcial** (processo protegido): a sessão segue e o watcher/job a encerra normalmente; o som avisa.
- **Breakaway** (ADR 0016): fora do job, não morre (Limitações 1).
- **Primeiro plano negado:** Limitações 3; verificação manual (CON-025).
- **Jogo que ignora `WM_CLOSE` ou abre "salvar?":** morto no prazo de 3 s (Limitações 2); o tempo continua cortado em `at`.
- **Callback do `EnumWindows`:** um só, criado no pacote (§2.3); um por chamada vazaria do limite de callbacks do runtime.
- **Janela de outro processo com PID reaproveitado** entre a leitura dos PIDs e o `PostMessageW`: janela de microssegundos;
  o pior caso é um `WM_CLOSE` a um programa alheio, que pode recusar. No adotado, o conjunto já é conferido pelo caminho.
- **Fechamento que demora** atrasa até 3 s a volta do Big Box e ocupa o roteador (fila de 8 gatilhos).

## 3. Modelo de dados e contrato

### 3.1 Migração 9 [VID-001]

`internal/store/migrations/00009_video.sql`, numa transação do goose (só o tipo `video`; a tabela `game` não muda):

```sql
-- +goose Up
CREATE TABLE media_new (... mesmas colunas ..., kind TEXT NOT NULL CHECK (kind IN
  ('cover','screenshot','title','banner','background','select','video')), ... UNIQUE (game_id, hash));
INSERT INTO media_new SELECT * FROM media;  DROP TABLE media;  ALTER TABLE media_new RENAME TO media;
-- igual para media_job (FK game ON DELETE CASCADE preservada)
-- +goose Down
DELETE FROM media WHERE kind = 'video'; DELETE FROM media_job WHERE kind = 'video';
-- reconstrução sem 'video'
```

Nenhuma tabela referencia `media`/`media_job`, então a reconstrução não precisa desligar `foreign_keys`. `store.SchemaVersion`
8 → 9; os testes que usam a constante passam sem mudança.

### 3.2 Consultas novas (`sqlc generate`)

`CountGameVideo(gameID)` (atuais + jobs ativos), `EnqueueVideoJobs(gameIDs)` (insere numa transação, pulando quem já tem),
`GamesMissingVideo(platformID?)`, `CancelPendingVideoJobs`, `VideoJobCounts`. `import.apply` e `GameDetail` não mudam.

### 3.3 Contrato IPC v0.15.0 [CON-017, CON-019, CON-026, CON-027, VID-005..VID-008, SSC-001]

[contracts/ipc-v0.15.0-delta.md](contracts/ipc-v0.15.0-delta.md). Só acréscimos: métodos `video.queue`, `video.queueMissing`,
`video.cancelPending`, `video.status`; notificação `game.closedByHotkey`; enums ampliados; blocos `x-console` e `x-video`.
Contrato completo: [contracts/ipc.openrpc.json](contracts/ipc.openrpc.json) (v0.15.0 = v0.14.0 + o delta).

## 4. Big Box novo [BBX-001..BBX-044]

### 4.1 Estrutura

```
frontend/src/bigbox/
  BigBox.tsx            shell: dados, eventos, reducer, entrada, tema ativo, data-bigbox-theme
  nav.ts                reducer puro (contracts/bigbox-faixa.md §3) + seletores
  useBigBoxInput.ts     gamepad (mapeamento padrão) e teclado -> ações abstratas
  viewModel.ts          monta BigBoxViewModel a partir das leituras
  themes/registry.ts    BigBoxThemeId, BIGBOX_THEMES, resolveTheme()
  themes/faixa/Faixa.tsx, FaixaStrip.tsx, FaixaInfo.tsx
  BigBoxDetails.tsx     tela de detalhes comum aos temas
  BackgroundStage.tsx   arte + vídeo (um <video> no máximo)
  Footer.tsx            dicas
```

`App.tsx` troca o `useBigBoxNav` e a classe `big-box` pela renderização condicional `bigBox ? <BigBox .../> : <três colunas>`.
`lib/bigbox.ts` mantém `enterBigBox`/`exitBigBox`; `useBigBoxNav`, `cardGrid`, `gridNext` e `nextFocus` saem com seus testes
(UIX-044 substituído). F11 continua entrando.

### 4.2 Navegação e entrada [BBX-005, BBX-006, BBX-014, BBX-016, BBX-018, BBX-019, BBX-024]

- Reducer `bigBoxNav(state, action)`; estado `{screen: 'browse'|'details', tab, focus, detailsFocus, gallery, suspended,
  awaitingFocusAfterBlur, sound}`. Ações abstratas: `prev`, `next`, `up`, `down`, `tabPrev`, `tabNext`, `accept`, `details`,
  `back`, `toggleSound`, `launched`, `sessionEnded`, `gameClosed`, `windowBlur`, `windowFocus`, `listLoaded`. O eixo do tema
  (`horizontal`: ←/→ = `prev`/`next`; `vertical`: ↑/↓) é aplicado em `useBigBoxInput`, não no reducer.
- `useBigBoxInput`: `requestAnimationFrame` lendo o primeiro gamepad; borda de subida por botão; analógico esquerdo com zona
  morta 0,6 e repetição ao segurar (400 ms, depois 120 ms); analógico direito rola a descrição. Botão 8 apertado → B e Y
  ignorados (inclusive a borda que acontecer com ele apertado) [BBX-019]. Teclado: `keydown` sem `repeat` para A/X/B/LB/RB;
  setas com `repeat`.
- Suspensão [BBX-018]: `launched` liga `suspended`; `windowBlur` com `suspended` liga `awaitingFocusAfterBlur`; `windowFocus`
  com ele ligado, `sessionEnded(gameId)` e `gameClosed` desligam. Suspenso, o hook não despacha nada.

### 4.3 Dados [BBX-004, BBX-007, BBX-009, BBX-017, BBX-027, BBX-028, BBX-029]

- Abas: `Catalog` (plataformas com `gameCount`). Lista: ~~`useInfiniteQuery(['bigbox', tabPlatformId])` sobre `ListGames` com
  `FilterSpec{platformIds}` e `sort: 'title'`, páginas de 200; próxima página quando `focus ≥ loaded − 10`~~ · alterado em
  2026-11-16: a aba é lida inteira antes de aparecer (§4.7.3, BBX-038).
- Jogo em foco: `GetGame` (`['game', id]`), `MediaList` (`['media', id]`, a mesma chave de IMG-008), `CurrentSession`.
- Eventos: `media:changed` (já invalida `['media', id]`), `session:changed` (marca e relê `['game', id]` ao fim),
  `game:closedByHotkey` (§2.5).
- MET-067: `App` trata `bigBox` como "modal aberto" para a abertura automática da confirmação [BBX-022].

### 4.4 Fundo e vídeo [BBX-010..BBX-013, VID-023]

`BackgroundStage` recebe a mídia do jogo em foco: arte por `pickBackground(media)` (ordem de BBX-010); vídeo = primeiro item
`video`. Temporizador de 2.000 ms (`setTimeout`, limpo a cada troca de foco) liga `<video muted={!sound} loop playsInline
autoPlay src>`; a troca de foco desmonta o elemento (sem `src`); `onError` marca o jogo como "vídeo falhou" no estado local da
visita. Transições por CSS (`opacity`, 300/400 ms). A janela normal filtra `kind !== 'video'` em `MediaCarousel` e
`useCardImages` [VID-023].

### 4.5 Tema Faixa e tokens [BBX-006, BBX-008, BBX-009, BBX-015, BBX-025, BBX-026]

Layout e tokens em [contracts/bigbox-faixa.md §1, §6](contracts/bigbox-faixa.md). Os tokens `--arc-bb-*` ficam em `index.css` sob
`[data-bigbox-theme="faixa"]` e são expostos ao Tailwind por `@theme inline`, como os da janela (ADR 0029). Texto sobre a arte
sempre sobre o véu `--arc-bb-scrim` (gradiente da esquerda para a direita e de baixo para cima). Capas: `CoverImage` existente.

### 4.6 Temas futuros (preparados, não implementados) [BBX-023, BBX-024]

O registro já lista os três; `mosaico` e `roda` têm `available: false` e nenhum componente. Para fazê-los basta um componente que
receba `BigBoxThemeProps` e um bloco de tokens.

- **Mosaico (estilo Switch):** fundo claro (tokens próprios, contraste conferido como BBX-026 sobre fundo claro), uma fileira
  horizontal de capas **quadradas** grandes no meio da tela, título do jogo em foco acima da fileira, ícones de ação embaixo;
  eixo `horizontal`; sem vídeo de fundo (o vídeo aparece só em Detalhes).
- **Roda (estilo temas "Unified" do LaunchBox), com a ressalva do usuário (2026-11-15):** a lista de seleção fica à direita,
  **disposta em arco como uma roda de verdade**, sobre um **fundo translúcido, menos opaco** que no mockup; os itens são
  **logos** de plataforma (na tela de plataformas) e de jogo (na lista de jogos) em curva; o vídeo ou a arte do item em foco
  aparece **por trás**. **Logos só no Roda.** Eixo `vertical`. Exige mídia `clear_logo` (tipo novo e migração, ADR 0030) e
  provavelmente vídeo por plataforma (conferência da API, decisão k): por isso fica para uma fase própria.
- **Vitrine:** fora (decisão do usuário).

### 4.7 Ajustes do teste com controle (2026-11-16) [BBX-006, BBX-015, BBX-016, BBX-024..BBX-026, BBX-032..BBX-044]

Prevalece sobre §4.2..§4.5 onde divergir. Contrato: [bigbox-faixa §1, §1.1, §2, §3, §3.1, §4..§7](contracts/bigbox-faixa.md).
Nenhuma mudança no Go, no IPC nem no banco; só `frontend/src/bigbox/`, `frontend/src/index.css` e os testes.

#### 4.7.1 Foco dos detalhes [BBX-016, BBX-032..BBX-036, BBX-015]

**Causa** (lacuna da spec, não só do código): a tabela antiga tinha `detailsFocus ∈ {play, playUntracked, description}` e
←/→ sempre na galeria; `BigBoxDetails` desenhava o contorno de foco no botão **e** na miniatura `aria-current`, a galeria era uma
fileira de miniaturas de 12em sem visor e sem limite de largura/altura. Com isso a marca visível parecia estar sempre na galeria e
a "troca" de imagem era um contorno movendo-se fora da tela. Nenhum teste passava pelo shell com o controle nos detalhes.

`nav.ts` (assinaturas públicas):

```ts
export type DetailsFocus = 'play' | 'playUntracked' | 'gallery'
export type DetailsButton = 'play' | 'playUntracked'
export type NavState = { /* ... */ detailsFocus: DetailsFocus; detailsButton: DetailsButton; gallery: number /* ... */ }
export type NavCtx = { tabs: number; total: number; galleryCount: number; focusedGameId: number | null; videoVisible: boolean } // sem loaded
export type NavAction = /* ... */ | { type: 'galleryChanged'; count: number }
export type NavEffect = { kind: 'launch' | 'launchUntracked'; gameId: number } | { kind: 'exit' } | { kind: 'bringToFront' } | { kind: 'enterBigBox' }
// sem 'fetchNextPage'; PREFETCH_BEFORE_END removido
```

Transições: tabela de [bigbox-faixa §3](contracts/bigbox-faixa.md). `galleryChanged` fica fora de `inputTypes` (vale suspenso).
O shell despacha `galleryChanged(galleryOf(media).length)` num `useEffect` dependente dessa contagem.

`useBigBoxInput`: na tela Detalhes, `PageUp`/`PageDown` viram `scrollUp`/`scrollDown` (em Navegar continuam `tabPrev`/`tabNext`);
o analógico direito continua emitindo `scrollUp`/`scrollDown`. As direções nos detalhes continuam `prev`/`next`/`up`/`down`.

`hints.ts`: `hintsFor('details', { detailsFocus, hasGallery, ... })` — botão: `A <rótulo do botão>`, `↓ Imagens` se
`hasGallery`; galeria: `←/→ Imagem`, `↑ Botões`; sempre `B Voltar`, `Select+B Fechar o jogo` e o `Y Som` de BBX-013.

`BigBoxDetails` (props): troca `detailsFocus: 'play' | 'playUntracked' | 'description'` por `DetailsFocus`; o resto igual.
Estrutura (§1.1): `section[data-bb-details]` em grid; coluna de texto `Panel`; visor `figure[data-bb-viewer]` com o item
`gallery` em `contain` (`<img>` ou o único `<video>`), contador `Panel` "`i+1` / N"; fileira `[data-bb-thumbs]`
(`data-focused` só com `detailsFocus === 'gallery'`) com `[data-gallery-item]` (`aria-current` no selecionado). A miniatura
selecionada é rolada para a vista com `scrollIntoView({ inline: 'nearest', block: 'nearest' })` quando `gallery` muda (chamada
protegida: jsdom não implementa). O visor usa `min(gallery, N − 1)` antes do `galleryChanged` chegar.

#### 4.7.2 Painéis e contraste [BBX-025, BBX-026, BBX-037]

- Token `--arc-bb-panel` em `index.css` (bloco Faixa) e `--color-bb-panel` em `@theme inline` → utilitário `bg-bb-panel`.
- `frontend/src/bigbox/Panel.tsx` (novo): `export function Panel(p: { children: ReactNode; className?: string; as?: 'div' | 'section' | 'figcaption' }): JSX.Element`
  → elemento com `data-bb-panel`, `bg-bb-panel`, cantos arredondados e recuo. Usado por `FaixaStrip` (tablist), `FaixaInfo`,
  `Faixa` (vazio/carregando), `Footer`, `BigBoxDetails` (coluna de texto e contador).
- O shell deixa de pintar a faixa do rodapé com `--arc-bb-scrim`; o rodapé é um `Panel` dentro da área segura.
- A descrição dos detalhes deixa de usar `--arc-bb-scrim` como fundo (já está no painel da coluna).

#### 4.7.3 Lista completa e faixa sem buracos [BBX-006, BBX-024, BBX-038, BBX-039]

`useBigBoxData.ts`:

```ts
export const PAGE_SIZE = 500
export type TabGames = { games: GameView[]; total: number; isPending: boolean; error: Error | null } // sem loaded/fetchNextPage
export function useTabGames(platformId: number | null): TabGames
// useQuery(['bigbox', 'games', platformId]); queryFn({ signal }) lê offset 0, 500, 1000...; para quando a página vem vazia
// ou offset >= page.total; confere signal.aborted entre páginas (troca de aba cancela o resto); total = games.length.
// staleTime: Infinity e gcTime de pelo menos a visita (aba já lida não relê); retry: false.
```

`viewModel.ts`:

```ts
export const STRIP_ANCHOR = 2, STRIP_MAX = 15, STRIP_FALLBACK_CAPACITY = 12
export function stripWindow(total: number, focus: number, capacity: number): { start: number; end: number } // §3.1
// ViewModelInput ganha capacity: number e loading: boolean; BigBoxViewModel ganha loading e stripStart (§4)
// windowAround e STRIP_WINDOW saem
```

`themes/types.ts` e `themes/registry.ts`: `BigBoxTheme.listCapacity`; `faixaCapacity(vp)` em
`themes/faixa/capacity.ts` (novo, pura, §3.1). Mosaico e Roda: `() => STRIP_FALLBACK_CAPACITY`.

Shell: `useViewport()` (novo, em `BigBox.tsx` ou `bigbox/useViewport.ts`) devolve `{width, height}` de `window.inner*` e
assina `resize` num `useEffect` com remoção no retorno (sem goroutine/timer solto). `capacity = theme.listCapacity(vp)`. O
`useCardImages` recebe os ids de `games.slice(start, end)`. `FaixaStrip` recebe `stripStart` do modelo e usa
`index = stripStart + i` (não recalcula a janela). Enquanto `tabGames.isPending`, `model.loading = true`, `focused = null`, e o
`Faixa` mostra o `Panel` "Carregando jogos…" com as abas; `listLoaded` só é despachado quando a leitura termina (BBX-027
passa a achar o jogo inicial mesmo além da primeira página).

#### 4.7.4 Fundo, capas e miniaturas inteiros [BBX-040, BBX-041]

`BackgroundStage`: cada camada da transição é um par `div[data-testid=bb-art-layer]` com `img[data-testid=bb-art-fill]`
(`object-cover`, `blur-[24px]` ou mais, `scale-110` para não mostrar borda do desfoque) + um `div` com `--arc-bb-scrim` + o
`img[data-testid=bb-art]` (`object-contain`). A opacidade da transição vai no par. `<video data-testid=bb-video>` passa a
`object-contain`, desenhado acima da camada atual. Props públicas iguais.

`FaixaStrip`: `img` da capa em `object-contain` (o cartão já tem `bg-bb-surface`). `BigBoxDetails`: miniaturas e visor em
`object-contain`.

#### 4.7.5 Área segura e layout dos detalhes [BBX-042, BBX-043, BBX-044]

- `BigBox.tsx`: dentro da raiz, depois do `BackgroundStage`, um `div[data-bb-safe]` com `padding: var(--arc-bb-safe-y)
  var(--arc-bb-safe-x)`, `flex flex-col h-full min-h-0`, contendo o tema (`flex-1 min-h-0`) e o rodapé. Nada de texto fora dele.
- `BigBoxDetails`: §1.1 do contrato. Espaçamentos por `style` (não só classe) nos três pontos medidos pelo teste
  (`data-bb-details` gap, `data-bb-facts` rowGap e lineHeight), para que o teste de componente leia valores e não nomes de
  classe.
- Sem galeria: sem coluna do visor; coluna de texto com `max-width: 60%`.

#### 4.7.6 Riscos

- **Corrida na leitura de várias páginas:** o `signal` do react-query é a única condição de saída do laço; o laço não guarda
  estado fora do `queryFn` e não agenda nada. Biblioteca de 20.000 jogos = 40 leituras de 500 em sequência (aceitável; medir em
  BBX-030).
- **`scrollIntoView` e `ResizeObserver` em jsdom:** não usamos `ResizeObserver` (capacidade vem do `resize` da janela);
  `scrollIntoView` é chamado com `?.`.
- **Um só `<video>` na tela Detalhes:** o `BackgroundStage` já fica `active = false` nos detalhes; o visor é o único dono do
  elemento e o desmonta ao trocar de item (sem `src`).
- **Mudança de contrato de tema:** `listCapacity` é obrigatório; o tema falso vertical dos testes ganha um.

## 5. Vídeo [VID-001..VID-029, SSC-001..SSC-006]

### 5.1 Quando o vídeo é buscado [VID-004..VID-007]

| Caminho | Gatilho | Observação |
|---|---|---|
| Automático | fim de `metadata.apply` e de cada jogo aplicado em `metadata.batchApply` | condições de VID-004; silencioso |
| Um jogo | "Buscar vídeo" no painel → `video.queue [id]` | VID-024 |
| Biblioteca | "Buscar vídeos dos jogos sem vídeo" na janela dos provedores → `video.queueMissing {}` | SSC-003 |
| Big Box | **nunca** | BBX-029 |

Justificativa: o download precisa de duas chamadas à API mais o arquivo; fazê-lo ao navegar gastaria cota com cada capa olhada e
deixaria o vídeo chegar segundos depois de o usuário já ter saído do jogo. Junto da aplicação de metadados o título já foi
revisado e confirmado; o pedido explícito cobre a biblioteca importada antes desta fase.

### 5.2 Fila [VID-002, VID-003, VID-009, VID-016..VID-019]

- `mediaQueue.dispatch` separa os jobs por raia: `kind == "video"` → `videoLane` (semáforo de 1); o resto → o
  semáforo de 2 atual. · alterado em 2026-11-15 (VID-009, revisão do diff): o critério era `provider == "screenscraper"` no
  despacho e `kind == "video"` no roteador do fetch; os dois passam a chamar **uma** função, `isVideoJob(job store.MediaJob) bool`
  (`job.Kind == "video"`), em `media_queue.go`, para a vaga e para o baixador nunca divergirem. A raia guarda `pause{kind: quota|credentials, until}` em memória (sob `q.mu`); pausada, seus jobs não são
  despachados e o `nextDeadline` inclui `until` (quota). `provider.setKey`/`deleteKey` de `screenscraper*` chamam
  `q.VideoCredentialsChanged()` (limpa a pausa de credencial e `Notify`).
- Erros do resolvedor classificados por tipo (`errors.As`): `*media.PermanentError` → `failed` na hora;
  `scrapers.ErrQuota` → pausa 1 h, job volta a `pending` com as mesmas tentativas; `scrapers.ErrCredentials` → pausa de
  credencial, idem; `scrapers.ErrNoCredentials` → idem, `lane = no-credentials`; o resto → regra MID-008.
- O fetcher da fila passa a receber o job: `Fetch(ctx, job store.MediaJob) (media.Stored, bool, error)`; o roteador de fetch
  manda `kind == "video"` ao `videoFetcher` e o resto ao `media.Store.Fetch` atual.
- `video.status` lê contagens do store e o estado da raia.

### 5.3 Resolvedor e cliente [VID-010..VID-015, VID-020, VID-027..VID-029, SSC-004]

```go
// internal/scrapers
type ScreenScraper struct { /* http client, base URL, *Limiter, creds func() (SSCreds, error) */ }
type SSCreds struct{ DevID, DevPassword, User, Password string }
func (s *ScreenScraper) SearchGames(ctx context.Context, systemID int, term string) ([]SSGame, SSQuota, error)
func (s *ScreenScraper) GameInfo(ctx context.Context, systemID int, gameID string) (SSGame, SSQuota, error)
func (s *ScreenScraper) OpenVideo(ctx context.Context, url string) (io.ReadCloser, int64, error) // creds appended in memory
func SystemID(seedKey string) (int, bool)        // contracts/screenscraper-rules.md §1
func RedactURL(raw string) string                // removes devid, devpassword, ssid, sspassword

// internal/agent
type videoResolver struct { /* store, *ScreenScraper, *media.Store, clock */ }
func (r *videoResolver) Fetch(ctx context.Context, job store.MediaJob) (media.Stored, bool, error)

// internal/media
func (s *Store) FetchVideo(ctx context.Context, gameID int64, r io.Reader, size int64) (Stored, bool, error)
```

Passos de `videoResolver.Fetch`: lê o jogo (título, ano, plataforma → chave de seed) → `SystemID` → `SearchGames(SearchTerm)` →
pontua ([rules §3](contracts/screenscraper-rules.md)) → `GameInfo` só se a busca não trouxe `medias` → escolhe a mídia (§4) →
confere `size` → `OpenVideo` → `FetchVideo`. Cada requisição passa pelo `Limiter` do ScreenScraper (criado uma vez em
`newProviderFactory`, como os da fase 12; intervalo ajustável para cima por `SSQuota.MaxPerMinute`). Erros de `net/http`
(`*url.Error`) são reembrulhados com `RedactURL` antes de subir; o `last_error` do job e os logs só veem a versão limpa.
`FetchVideo` escreve em `<dir>/<rand>.tmp` com `io.TeeReader` para o `sha256`, limita a 20 MiB + 1, confere `ftyp` nos primeiros
12 bytes, renomeia para `<hash>.mp4` (ou apaga o temporário se já existe) e apaga o temporário em qualquer erro.

**Correções da revisão do diff (2026-11-15) [VID-027, VID-028, VID-029, VID-020].** Ordem de `OpenVideo`:

1. `ctx.Err()`.
2. `checkMediaOrigin(base *url.URL, raw string) (*url.URL, error)` (pura, [rules §10](contracts/screenscraper-rules.md)):
   recusa → `fmt.Errorf("%w: URL de vídeo fora do ScreenScraper", ErrBadRequest)`, **sem** ler `Creds`, sem `Limiter.Wait` e
   sem requisição. O `BaseURL` é analisado uma vez em `NewScreenScraper` (campo não exportado `baseURL *url.URL`).
3. `Creds()` e o acréscimo das quatro credenciais em memória (como hoje).
4. `do` (limitador e GET) com o cliente próprio (abaixo).
5. Status ≠ 200 → `classify` (como hoje). Status 200 com `mime.ParseMediaType(Content-Type)` começando por `text/` → lê até
   4.096 bytes, fecha o corpo e devolve `classifyMediaText(body)`: o `classify` de texto quando reconhece (cota, credencial,
   API fechada, não encontrado) e, senão, o sentinela novo `scrapers.ErrNotVideo` (o pacote `scrapers` não importa `media`); o
   `apiError` do resolvedor traduz `ErrNotVideo` em `media.Permanent("formato de vídeo não suportado")` e já traduz
   `ErrNotFound` do passo "baixar" em "sem vídeo no ScreenScraper".
6. Senão devolve o corpo e o `Content-Length` (como hoje).

`NewScreenScraper` monta `s.http` como **cópia rasa** de `cfg.HTTP` (ou de `http.DefaultClient`) com
`CheckRedirect = sameHostRedirect` ([rules §11](contracts/screenscraper-rules.md)); o `*http.Client` compartilhado do agente
(usado pelas imagens e pelos outros provedores) não é tocado. `sameHostRedirect(req, via)` devolve
`errRedirectOffHost` quando `!strings.EqualFold(req.URL.Host, via[0].URL.Host)` ou `via[0].URL.Scheme == "https" &&
req.URL.Scheme != "https"`, e `http.ErrUseLastResponse` nunca (o 3xx não é tratado como sucesso). Em `netErr`,
`errors.Is(err, errRedirectOffHost)` → `fmt.Errorf("%w: redirecionamento para fora do ScreenScraper", ErrBadRequest)` antes do
caminho de `ErrUnavailable`; a mensagem não leva a URL. O `apiError` do resolvedor mantém `ErrBadRequest` → permanente.

### 5.4 Credenciais [SSC-001..SSC-003, SSC-006, VID-019, VID-020]

`scrapers.Keys` ganha `SetPair(provider, id, secret)`/`Pair(provider)` (mesmo formato `id\nsenha` do IGDB) para
`screenscraper` e `screenscraperdev`. O agente valida o enum novo em `provider.*`.
· alterado em 2026-11-15 (SSC-006, revisão do diff): `SetPair` recusa `id` ou `secret` com `\n` ou `\r` em qualquer posição
(`strings.ContainsAny(v, "\r\n")`, antes de qualquer `trim`) com o sentinela novo `scrapers.ErrInvalidKey`, sem gravar;
`scrapeService.setKey` confere o mesmo antes de chamar `SetPair` e responde `-32602` (também mapeia `ErrInvalidKey` para
`-32602`, por defesa), sem chamar `notifyVideoCreds`. `SetIGDB` não muda nesta fase. A UI (`ProviderKeysDialog`) ganha a seção
ScreenScraper e o bloco de vídeos (`VideoStatus` a cada 5 s com a janela aberta, `setInterval` limpo ao fechar).

### 5.5 Servir o vídeo [VID-022]

`serveMedia` passa a definir `Content-Type: video/mp4` para `.mp4` antes de `http.ServeFile` (que já responde `Range` com 206;
o `mime` do Go não conhece `.mp4` sem o registro do Windows). Risco: o `AssetServer` do Wails v2 repassa `Range`? Conferir na
tarefa; se não repassar, o `<video>` ainda toca do começo com 200 (o Faixa não busca posição).

### 5.6 Riscos

- **API não confirmada** (endpoints, campos, códigos, cota, vídeo por plataforma): tarefa de conferência preenche
  [contracts/screenscraper-api-notes.md](contracts/screenscraper-api-notes.md) **antes** do cliente; divergência que mude regra
  visível volta à spec.
- **URL de mídia com credenciais embutidas** na resposta: `RedactURL` em tudo que é gravado ou registrado (VID-020).
- **URL de mídia apontando para outro host** (resposta adulterada ou servidor comprometido): as credenciais só vão a
  `https` no host do `BaseURL` ou em `*.screenscraper.fr`, e só seguem redirecionamento no mesmo host (VID-027, VID-028).
  Risco residual: um subdomínio de `screenscraper.fr` fora do controle do ScreenScraper; aceito.
- **Espelho real em outro domínio:** se a conferência (T45) mostrar que `medias[].url` usa um host fora de `*.screenscraper.fr`,
  o vídeo passa a falhar com "URL de vídeo fora do ScreenScraper"; a lista de hosts volta à spec (questão em aberto).
- **`prefers-reduced-motion`** não é requisito desta fase (requirements, "Fora desta spec").
- **Fila grande consumindo a cota:** raia de 1, intervalo ≥ 1,5 s, pausa por cota sem gastar tentativa, `video.cancelPending`.
- **Disco:** ~2 MB por jogo (medição de 2026-11-15), sem limite total por decisão do usuário; o teto de 20 MB é a trava.

## 6. Invariantes [CON-024, CON-028, CON-029]

- A UI não fecha jogos nem encerra sessões: Select + B é lido e aplicado no agente; o evento só move a janela.
- Fechar o agente continua nunca matando o jogo (ADR 0005, SES-011): `detachedJob.Close` e `winJob.Close` não matam.
- Goroutines novas: nenhuma no tracker; no agente, nenhuma além das existentes (a raia de vídeo usa as goroutines por job da
  fila, esperadas pelo `wg` do `Run`); no frontend, só `requestAnimationFrame`/temporizadores limpos no desmontar.
