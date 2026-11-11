# Fase 4 — Pausa e "sem contar tempo": progresso

Documento de passagem entre agentes. Estado do repositório em 2026-11-11, branch `main`, último commit
`6a77201` (T20). Não há alterações não commitadas.

## Onde parei

- **Concluídas e commitadas:** T01 a T20 (de 25 tarefas).
- **Próxima:** T21 — UI Go: bindings, mensagens de erro e repasse de sessão pausada.
  - Começar pelo test-writer (`cmd/arcadia/*_test.go`), depois implementer, depois reviewer.
  - Lembrar de rodar `cd cmd/arcadia && wails generate module` na T21 para atualizar `frontend/wailsjs/`.
- **Pendentes depois da T21:** T22 (lib do frontend), T23 (componentes e `App`),
  T24 (documentação), T25 (verificação final e checagens manuais).

## Como trabalhar (processo do projeto)

- Spec: `docs/specs/phase-4-pause/` (requirements, design, contracts, tasks). Status: **aprovada** (2026-11-10).
- Plano: `docs/specs/phase-4-pause/tasks.md` — **aprovado** (2026-11-10). Não implementar nada fora dele.
- Ciclo por tarefa: test-writer escreve os testes (vermelhos) → implementer ou tracker-specialist escreve o código
  → reviewer revisa → correções → `go vet`, `GOOS=linux go vet`, `go test ./...` → commit próprio em português.
- Regras (CLAUDE.md): quem escreve o código não escreve o teste; `internal/tracker/` e `cmd/fake-game/` só o
  tracker-specialist; sem `time.Now`/`time.Sleep` em lógica testável; toda goroutine com dono e saída; todo handle
  do Windows fechado logo após o erro checado.
- Commits: mensagem em português, terminando com `Co-Authored-By: ...`.
- Comandos úteis:
  - `go vet ./...` e `GOOS=linux go vet ./...`
  - `go test ./...`
  - `powershell -NoProfile -ExecutionPolicy Bypass -File scripts/spec-trace.ps1 -Spec docs/specs/phase-4-pause`
    (`-SkipTests` antes da etapa de testes)
  - `sqlc generate` (já feito em T02/T03/T04; sem mudanças pendentes)
  - `cd cmd/arcadia && wails generate module` (necessário na T21, atualiza `frontend/wailsjs/`)
  - Frontend: `cd frontend && npx vitest run && npx tsc --noEmit -p . && npm run build` (Node fica em
    `C:\Program Files\nodejs`; ajustar o PATH no shell)
- Se `go` não estiver no PATH: `"C:\Program Files\Go\bin\go.exe"`.

## Tarefas concluídas (commits)

| Tarefa | Conteúdo | Commit |
|---|---|---|
| T01 | domain: estado `paused`, motivos de pausa, antirrepique | 375e9fd |
| T02 | store: migração 4 (índice único de pausa aberta), `SchemaVersion = 4` | aa6c862 |
| T03 | store: abrir, fechar e listar pausas | 9c2cbf5 |
| T04 | store: fim de sessão fecha a pausa; STO-023; recuperação | bb836c3 |
| T05 | ipc: protocolo v0.5.0 e teste de contrato | 530b765 |
| T06 | tracker: máquina pura com `Paused` (+ revisão) | 21459f0 |
| T07 | tracker: coordenador com pausa, suspensão e retomada | 4bc8476 |
| T08 | tracker: lançamento sem rastrear (+ correção de teste de handles) | 8e7ba9a, f1ada10 |
| T09 | input: combo Select+Y e poller do controle | 62d12a9 |
| T10 | input: atalho global RegisterHotKey (+ revisão: Close concorrente) | ee01452, 16e659c |
| T11 | input: leitura do controle por XInput | 5649d3c |
| T12 | winapi: wndProc compartilhado por mapa de handlers | 46bf2fc |
| T13 | winapi: PowerWatcher (suspensão e retomada do PC) | 3d99eec |
| T14 | feedback: coordenador portátil | 7ba7b50 |
| T15 | winapi: geometria do aviso | 7ba7b50 |
| T16 | winapi: janela do aviso (Overlay) | fa9bd63 |
| T17 | winapi: som (Beeper) | fa9bd63 |
| T18 | agente: handlers `game.launchUntracked`, `session.pause`, `session.resume` | 234b475 |
| T19 | agente: roteador de gatilhos e ponte de energia | 3ba5a4c |
| T20 | agente: subida e shutdown com as fontes, o feedback e a energia | 8352715 |

## Tarefas pendentes (resumo do tasks.md)

- **T21 — UI Go:** bindings `PauseSession`, `ResumeSession`, `LaunchGameUntracked`; mensagem de `-32005`;
  repasse de `paused`; `wails generate module`. Requisitos: DSK-013..015, DSK-010/011 (alterados).
- **T22 — frontend lib:** `lib/games.ts` e `lib/sessionClock.ts` com o estado `paused` (cronômetro congelado).
- **T23 — frontend componentes:** botão "Sem contar tempo" por jogo, botão Pausar/Retomar no painel, "Pausado"
  na lista, lançamento pendente bloqueia os dois botões do jogo.
- **T24 — documentação:** ARCHITECTURE §2/§4, CLAUDE.md (spec atual já aponta para a fase 4), índice de ADRs
  (0017–0019 já listados), anotações das specs anteriores.
- **T25 — verificação final:** `CGO_ENABLED=0 go build`, testes Go e frontend, `wails build`, migração real de
  banco v3→v4, e as checagens manuais (UI-026, PAD-009, HOT-005, OVL-007, PWR-007, SND-003, PAU-007, TRK-002
  manual se aplicável). Ver a tabela da T25 em `tasks.md`.

## Pontos de atenção herdados

- **Testes de handles do Windows** são sensíveis a threads novas do runtime (≈6 handles por thread). Já há
  tolerância por thread em `internal/tracker/main_windows_test.go` e `internal/winapi/power_windows_test.go`. Usar o
  mesmo padrão em qualquer teste novo de recursos.
- **`TestHandleCount` e o teste de recursos do aviso** medem por blocos até platô; não relaxar além disso sem
  revisar o motivo.
- **Shell:** o hook do Bash de subagentes bloqueia prefixos `env` e encadeamento complexo; rodar `GOOS=linux go vet`
  diretamente pelo agente principal quando um subagente não puder.
- **Limitações conhecidas da fase 4** (requirements.md, seção "Limitações conhecidas"):
  - Modern Standby pode não enviar "usuário presente": a sessão fica pausada até o atalho.
  - Suspensão que o Windows não anuncia conta como tempo jogado.
  - Lançamento sem rastrear com outro jogo já em sessão é recusado; o inverso não é detectável.
- **Pendências de observação já registradas em revisões (não bloqueantes):**
  - `internal/tracker/tracker.go`: `control` ignora `ctx` no envio; documentar no godoc.
  - `internal/tracker/job_windows.go`: erro do `CloseHandle` em `defer` é descartado (aceito).
  - `internal/winapi/overlay_windows.go`: fallback de `GetMonitorInfoW` falho; `GetDpiForMonitor` sem `Find()`.
  - `internal/feedback/feedback.go`: `Start` chamado duas vezes cria duas goroutines (sem proteção).
  - `internal/agent/pause.go`: `OnSuspend` loga Warn "falha ao registrar suspensão" em erros que não sejam
    `ErrShuttingDown`; confirmar o comportamento na revisão da T20.
  - Verificação de Linux (`GOOS=linux go vet`) do arquivo `internal/agent/pause.go` ainda não rodada pelo agente
    que o escreveu — rodar antes da T20.

## Arquivos de referência

- Spec: `docs/specs/phase-4-pause/requirements.md`, `design.md`, `tasks.md`, `contracts/`
- ADRs: `docs/adr/0017-modelo-de-pausa.md`, `0018-entrada-global-e-feedback-nativo.md`, `0019-lancamento-sem-rastrear.md`
- Arquitetura: `docs/ARCHITECTURE.md` (§4.3 e §4.4 já atualizados para a fase 4)
- Projeto: `CLAUDE.md`
