# Fase 5.1 — Ajustes da biblioteca: tarefas

- **Status:** rascunho, aguardando aprovação das tarefas (o usuário aprovou só os requisitos).
- **Requisitos:** [requirements.md](requirements.md) · **Design:** [design.md](design.md)
- **Regra:** cada tarefa vira um commit. Quem escreve o código não escreve o teste dele (CLAUDE.md).

---

## T01 — Manifesto com elevação obrigatória (ADM-001)
- **Arquivos:** `cmd/arcadia/build/windows/wails.exe.manifest` (trustInfo `requireAdministrator`).
- **Teste:** `cmd/arcadia/manifest_test.go` — `ADM-001/manifesto pede requireAdministrator` (lê o XML do arquivo).
- **Manual:** ADM-001 (UAC na abertura, recusa não sobe nada), após `wails build`.

## T02 — Remover o início automático do agente (ADM-004, AST-002..011 substituídos)
- **Remover:** `internal/agent/autostart.go`, `internal/agent/autostart_test.go`, `internal/winapi/autostart_windows.go`,
  `internal/winapi/autostart_windows_test.go`.
- **Tray:** `internal/agent/tray_windows.go`, `tray_loop.go`, `tray_loop_test.go` — sai "Iniciar com o Windows", o canal
  `autostart` e `OnToggle`; os testes do menu são ajustados.
- **Testes:** o loop não tem mais canal nem callback de início (compilação). O menu da bandeja é Windows-only e não é unitário: `· verificação: manual` (ADM-004, menu só com "Abrir Arcadia" e "Sair").
- **Dependência:** nenhuma.

## T03 — Limpeza única da entrada Run (ADM-004)
- **Arquivos:** `internal/winapi/run_windows.go` (`RemoveOwnRunEntry(exe) (removed bool, err error)`),
  `internal/agent/run_windows.go` (chama no lugar de `Reconcile`, com `autostartTimeout`, pula em `dev`).
- **Testes:** `internal/winapi/run_windows_test.go` com chave de teste `HKCU\Software\ArcadiaTest`:
  - `ADM-004/apaga a entrada quando aponta para o mesmo exe`,
  - `ADM-004/mantém entrada de outro exe`,
  - `ADM-004/ausência não é erro`,
  - `ADM-004/valor ilegível é mantido e reportado`.
- **Dependência:** T02.

## T04 — Detectar agente não elevado (ADM-006)
- **Arquivos:** `internal/winapi/elevation_windows.go` (`IsProcessElevated(pid uint32) (bool, error)`),
  `cmd/arcadia/app.go` (`App.AgentElevated() (bool, error)` usando o PID de `agent.status`).
- **Testes:** `internal/winapi/elevation_windows_test.go`:
  - `ADM-006/processo atual sob o teste reporta o estado real do token`,
  - `ADM-006/PID inexistente retorna erro`.
- **Dependência:** nenhuma.

## T05 — Aviso de agente antigo na UI (ADM-006)
- **Arquivos:** `frontend/src/lib/agent.ts` (consulta `AgentElevated`), `frontend/src/App.tsx` (mensagem de conflito e
  não usa o agente), `frontend/src/components/AgentIndicator.tsx` (texto "Agente com privilégio de administrador" quando
  elevado — ADM-002).
- **Testes:** `frontend/src/components/AgentConflict.test.tsx` — `ADM-006/mostra o aviso de agente sem privilégio de administrador`;
  `ADM-002/mostra "Agente com privilégio de administrador" quando elevado`.
- **ADM-003 (mensagem do erro 740):** `cmd/arcadia/errors.go` (`userError`, código 740 em `winError`); testes `TestElevationRequired` (`ADM-003/...`).
- **Dependência:** T04.

## T06 — Componente de tags (TAG-001..TAG-004)
- **Arquivos:** `frontend/src/components/TagInput.tsx`, `frontend/src/components/TagInput.test.tsx`.
- **Testes:** `TAG-001/sugere gêneros do catálogo sem diferenciar maiúsculas`,
  `TAG-001/Enter, vírgula ou sugestão adiciona a tag`, `TAG-002/nome novo é marcado como novo`,
  `TAG-003/duplicata sem diferença de maiúsculas não entra`, `TAG-003/vazio ou longo demais mostra erro`,
  `TAG-004/× remove a tag`, `TAG-004/Backspace com campo vazio remove a última`.
- **Dependência:** nenhuma.

## T07 — Gêneros na edição usam tags (TAG-005)
- **Arquivos:** `frontend/src/components/GameEditForm.tsx` (campo de gêneros vira `TagInput`; `FormValues.genres` é
  `string[]`), `frontend/src/components/GameEditForm.test.tsx`.
- **Testes:** `TAG-005/salvar envia genres como array e só quando mudou`, `TAG-002/nome novo entra no patch`.
- **Dependência:** T06.

## T08 — Verificação final da fase 5.1
- `go build ./...`, `go vet ./...`, `go test ./...` (Windows); `npm test`, `npx tsc --noEmit`, `npm run build`.
- `wails build -ldflags "-X main.version=0.5.1"` (sem `-clean`).
- `spec-trace.ps1` para `phase-5-1-ajustes` e `single-exe`.
- Manual: ADM-001 (inclui build dev sem UAC), ADM-005 (relançamento pela UI herda a elevação), ADM-006, TAG-005 (filtro por gênero) — fica para o usuário.
- **Dependência:** T01..T07.

---

## Ordem e paralelismo

T01, T02 (depois T03), T04 e T06 podem andar em paralelo (arquivos diferentes). T05 depende de T04. T07 depende de T06.
T08 por último.
