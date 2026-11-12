# Fase 6 — Importadores e rastreio fora da árvore: tarefas

- **Status:** rascunho, aguardando aprovação das tarefas (o usuário aprovou só os requisitos).
- **Requisitos:** [requirements.md](requirements.md) · **Design:** [design.md](design.md)
- **Regra:** cada tarefa vira um commit. Quem escreve o código não escreve o teste dele (CLAUDE.md). Mudança de
  comportamento de tracker passa pelo tracker-specialist.

---

## T00 — ADR 0025 e contrato v0.7.0 (IPC-050..IPC-053)
- **Arquivos:** `docs/adr/0025-rastreio-por-processo-adotado.md` (proposto); `docs/specs/phase-6-importers/contracts/ipc.openrpc.json`
  (v0.7.0: `import.scan`, `import.apply`, forma de `CandidateView` e `ApplyResult`, `game.launch` com `uri`).
- **Testes:** `internal/ipc/contract_phase6_test.go` — `IPC-050/import.scan aceita fonte e caminho`,
  `IPC-051/import.apply rejeita lista vazia com -32602`, `IPC-052/candidatos usam camelCase`,
  `IPC-053/game.launch aceita launchType uri`.
- **Dependência:** nenhuma.

## T01 — Parser da Steam (IMP-002)
- **Arquivos:** `internal/importers/steam.go`, `internal/importers/vdf.go`, `testdata/steam/` (libraryfolders.vdf, appmanifest_*.acf).
- **Testes:** `IMP-002/lê appmanifest e devolve candidato com appid, nome, pasta e uri`,
  `IMP-002/biblioteca inexistente devolve zero candidatos e aviso`, `IMP-002/appmanifest ilegível vira aviso`.
- **Dependência:** nenhuma.

## T02 — Parser da Epic (IMP-003)
- **Arquivos:** `internal/importers/epic.go`, `testdata/epic/*.item`.
- **Testes:** `IMP-003/item vira candidato com uri de lançamento silencioso`,
  `IMP-003/item sem DisplayName é ignorado e contado no aviso`.
- **Dependência:** nenhuma.

## T03 — Pasta e executável avulso (IMP-004)
- **Arquivos:** `internal/importers/folder.go`.
- **Testes:** `IMP-004/só o primeiro nível, só .exe, título sem extensão`,
  `IMP-004/subpastas não são lidas`.
- **Dependência:** nenhuma.

## T04 — LaunchBox (IMP-005, IMP-007 dados)
- **Arquivos:** `internal/importers/launchbox.go`, `testdata/launchbox/platform.xml` (fixture anonimizada).
- **Testes:** `IMP-005/Game vira candidato com título, plataforma, empresas, gêneros, ano e caminho`,
  `IMP-005/PlayCount e LastPlayedDate viram tempo jogado; sem eles, sem tempo`,
  `IMP-005/campos desconhecidos são ignorados`.
- **Verificação manual:** IMP-005 com o XML real do usuário antes de fechar a tarefa. Se os campos diferirem da fixture,
  a tarefa volta para ajuste.
- **Dependência:** nenhuma.

## T05 — Gravação de candidato (IMP-001, IMP-006, IMP-007, IMP-009, RUL-001)
- **Arquivos:** `internal/store/import_apply.go` (`ApplyCandidate`, `ErrDuplicate`), `internal/store/import_apply_test.go`.
- **Testes:** `IMP-006/mesmo launch_target não duplica`, `IMP-006/mesmo título e plataforma não duplica`,
  `IMP-007/tempo vira sessão source=import com ended_at na última partida`,
  `IMP-007/sessões rastreadas existentes não mudam`, `IMP-009/apply não cria sessão rastreada`,
  `RUL-001/install_dir gera regra install_dir com learned=0`, `RUL-001/exe gera regra exe_path`,
  `IMP-001/o scan não grava nada` (no teste de agente, T06).
- **Dependência:** T00.

## T06 — Handlers do agente (IMP-001, IMP-008)
- **Arquivos:** `internal/agent/imports.go` (`handleImportScan`, `handleImportApply`), registro em `games.go`/`dispatch`,
  `internal/agent/imports_test.go`.
- **Testes:** `IMP-001/scan não grava nada`, `IMP-001/apply grava só os selecionados`,
  `IMP-008/uma fonte com erro não derruba as outras`, `IMP-008/um candidato que falha não impede os demais`,
  `IMP-006/scan marca exists para candidatos já na biblioteca`.
- **Dependência:** T01..T05.

## T07 — Lançamento por URI no agente e tracker (RUL-003)
- **Arquivos:** `internal/winapi/shell_windows.go` (`ShellOpen(uri)`), `internal/tracker/launcher.go` (`LaunchSpec.Kind`,
  `Launcher.OpenURI`), `internal/tracker/launcher_windows.go`, `internal/agent/games.go` (aceita `uri`).
- **Testes:** `RUL-003/ShellOpen abre um URI de teste sem processo filho` (winapi, com esquema de teste),
  `RUL-003/launch_type uri não é mais recusado` (agente, fake).
- **Dependência:** T00.
- **Revisão:** tracker-specialist (diretório `internal/tracker/`).

## T08 — Estado de espera de adoção na máquina (RUL-003, RUL-004) — tracker-specialist
- **Arquivos:** `internal/tracker/machine.go` (evento `Adopted`, evento `AdoptTimeout`, `end_reason = client_no_launch`),
  `internal/tracker/tracker.go` (prazo de 60 s com o clock).
- **Testes:** `RUL-003/AdoptTimeout sem processo vira discarded com client_no_launch`,
  `RUL-003/Adopted leva Launching a Running`, `RUL-004/carência continua valendo para sessão adotada`.
- **Dependência:** T07.

## T09 — Processo adotado e watcher (RUL-002, RUL-004, RUL-005) — tracker-specialist
- **Arquivos:** `internal/winapi/process_windows.go` (`ListProcesses`), `internal/tracker/adopted_job.go` (implementa
  `Job` com `SYNCHRONIZE`), `internal/tracker/watch.go` (goroutine dona, `Close` espera), `internal/tracker/watch_test.go`
  com processos falsos.
- **Testes:** `RUL-002/processo novo que casa com install_dir é adotado`,
  `RUL-002/processo que existia antes do OpenURI é ignorado`, `RUL-004/volta a casar na carência e a sessão continua`,
  `RUL-005/sessão adotada tem tracking=process e Job vazio`, `RUL-005/Terminate de processo adotado retorna erro`.
- **Dependência:** T08.

## T10 — Sinal da Steam (STM-001) e campo `tracking` (RUL-005)
- **Arquivos:** `internal/winapi/steam_windows.go` (`SteamRunning`), `internal/tracker/watch.go` (consulta a cada ciclo),
  `internal/domain` (`SessionSnapshot.Tracking`), `internal/ipc` (campo `tracking` no `SessionView`, opcional).
- **Testes:** `STM-001/Running 1 confirma o casamento`, `STM-001/Running 0 depois de adotado encerra sem carência`,
  `STM-001/chave ausente é ignorada sem erro` (winapi com chave de teste).
- **Dependência:** T09.

## T11 — Interface de importação (IMP-010)
- **Arquivos:** `frontend/src/lib/imports.ts`, `frontend/src/components/ImportDialog.tsx`, `ImportDialog.test.tsx`,
  `cmd/arcadia/library.go` (binding `ImportScan`, `ImportApply`, `PickFolder`), `frontend/src/App.tsx` (botão "Importar").
- **Testes:** `IMP-010/Importar selecionados fica desabilitado sem marcados`, `IMP-010/mostra já existe e avisos`,
  `IMP-010/foco preso no diálogo`.
- **Dependência:** T06.

## T12 — Aviso de rastreio por processo na página do jogo (RUL-005)
- **Arquivos:** `frontend/src/components/GamePage.tsx`, `SessionPanel.tsx` (texto "rastreio por processo").
- **Testes:** `RUL-005/página mostra rastreio por processo quando a sessão é adotada`.
- **Dependência:** T10, T11.

## T13 — Verificação final da fase 6
- `go build ./...`, `go vet ./...` (Windows e Linux), `go test ./...`; `npm test`, `npx tsc --noEmit`, `npm run build`.
- `wails build -ldflags "-X main.version=0.6.0"` (sem `-clean`).
- `spec-trace.ps1` para `phase-6-importers`.
- Manual (fica para o usuário): IMP-005 com o XML real; IMP-010; STM-001 com a Steam; RUL-003 com a Steam (risco de
  elevação, design §5); RUL-005.
- **Dependência:** T00..T12.

---

## Ordem e paralelismo

T00 primeiro. T01, T02, T03, T04 podem andar em paralelo (arquivos diferentes). T05 depende de T00. T06 depende de
T01..T05. T07 depende de T00. T08 → T09 → T10 em sequência (mesmo pacote do tracker). T11 depende de T06. T12 depende de
T10 e T11. T13 por último.
