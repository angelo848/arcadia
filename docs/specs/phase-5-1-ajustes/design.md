# Fase 5.1 — Ajustes da biblioteca: design

- **Requisitos:** [requirements.md](requirements.md) (aprovados em 2026-11-12) · **Tarefas:** [tasks.md](tasks.md)
- **ADRs:** [0023](../../adr/0023-execucao-como-administrador.md) (elevação) e [0024](../../adr/0024-agente-sempre-como-administrador.md)
  (fim do início automático), ambas aceitas com a aprovação desta spec.
- **Sem mudança de contrato IPC.** Nenhuma coluna, nenhuma migração, nenhum método novo.

## 1. Elevação (ADM-001, ADM-002, ADM-003, ADM-006)

**Manifesto.** `cmd/arcadia/build/windows/wails.exe.manifest` recebe `trustInfo` com
`<requestedExecutionLevel level="requireAdministrator" uiAccess="false"/>`. Só o `wails build` embute o manifesto;
`go run` e `wails dev` continuam sem elevação (ADM-001, build dev).

**Agente herda.** A UI (elevada) inicia o agente com `CreateDetached` (já existente), e o filho herda o token. Não há
`runas`. Os jogos são lançados pelo agente, como hoje: Job Object, pausa e filhos (fases 3 e 4).

**Agente antigo não elevado (ADM-006).** Um agente da versão anterior pode estar rodando sem elevação até o logout.
A UI, ao conectar, consulta o PID do agente com a própria elevação (que é alta, pois a UI é admin):

```go
// internal/winapi/elevation_windows.go
// IsProcessElevated reports whether the process pid runs with an elevated token.
// It opens the process with PROCESS_QUERY_LIMITED_INFORMATION and the token with TOKEN_QUERY,
// reads TokenElevation and closes both handles (CLAUDE.md: handle com dono claro).
func IsProcessElevated(pid uint32) (bool, error)
```

Se `IsProcessElevated(status.pid)` é falso, a UI mostra o aviso de ADM-006 e não usa o agente. Sem mudança de IPC: o PID já
vem em `agent.status`. Esta função fica em `internal/winapi` (só Windows) e a UI a chama por um método do
`cmd/arcadia` (`App.AgentElevated`), que devolve `bool` ao frontend.

## 2. Remoção do início automático (ADM-004)

Remove-se a feature inteira, e fica só a limpeza única:

- Apagar: `internal/agent/autostart.go` e `autostart_test.go`; `internal/winapi/autostart_windows.go` e
  `autostart_windows_test.go`, **exceto** o que a limpeza usa (abaixo). A preferência `autostart` no banco fica sem leitor.
- Tray (`tray_windows.go`, `tray_loop.go`, `tray_loop_test.go`): sai o item "Iniciar com o Windows" e o canal
  `autostart`/`OnToggle`. O menu fica com "Abrir Arcadia" e "Sair".
- `run_windows.go`: no lugar de `auto.Reconcile`, chama `winapi.RemoveOwnRunEntry(exe)` com `autostartTimeout`
  (2 s, já existente). Build `dev` (`cfg.Version == "dev"`) não toca no registro.

Limpeza, em `internal/winapi/run_windows.go` (um arquivo pequeno):

```go
// RemoveOwnRunEntry deletes the HKCU Run value "Arcadia" when it points to exe (ADM-004).
// A missing value is not an error; an unreadable value is kept and reported as a warning.
func RemoveOwnRunEntry(exe string) (removed bool, err error)
```

Erros viram log `Warn`/`Error` e não derrubam o agente (AST-009 substituído). A comparação de caminho usa
`filepath.Clean` e `EqualFold`, como hoje.

## 3. Tags de gênero (TAG-001..TAG-005)

**Componente.** `frontend/src/components/TagInput.tsx`: controlado (`value: string[]`, `onChange`), com `suggestions`
vindas do catálogo de gêneros (`useCatalog`, já existente). Não cria nada no servidor: a criação acontece no `game.update`
com o nome novo (EDT-004).

- Sugestões: `startsWith`/`includes` sem diferença de maiúsculas, ao digitar; excluem o que já é tag.
- Confirmar (Enter ou vírgula) ou tocar numa sugestão: adiciona. Sugestão usa o nome exato do catálogo (TAG-001).
- Duplicata sem diferença de maiúsculas é ignorada (TAG-003). Vazio ou > 100 caracteres: erro de `validateDraft`
  (DOM-016), mostrado no campo.
- Backspace com campo vazio remove a última tag; "×" remove uma tag (TAG-004).
- Marcação "nova" para nome sem correspondência no catálogo (TAG-002).

**Formulário.** `GameEditForm` troca o `<input>` de gêneros por `TagInput`. `FormValues.genres` vira `string[]` (não mais
texto com vírgula), e `toDraft` passa o array direto. `diffPatch` continua mandando só `genres` quando muda (EDT-005).

**Sem mudança de IPC** (TAG-005). `AddGameForm` não tem gêneros, então não muda.

## 4. Testes e verificação

- Go: `winapi` (`RemoveOwnRunEntry`, `IsProcessElevated`) com testes de Windows que usam uma chave de teste em
  `HKCU\Software\ArcadiaTest` (nunca a chave real); o manifesto é verificado por um teste que lê o XML embutido.
- Frontend: `TagInput.test.tsx` (TAG-001..004) e `GameEditForm.test.tsx` (TAG-005, troca do campo de texto).
- Manual: ADM-001 (UAC ao abrir; build dev sem UAC), ADM-006 (aviso com agente antigo), TAG-005 (filtro), UI
  de gêneros em edição.
- `spec-trace.ps1 -Spec docs/specs/phase-5-1-ajustes`, e a trilha de `single-exe` continua OK com AST-002..011 substituídos.

## 5. Riscos

- **Agente antigo em execução:** só some no logout ou ao sair pela bandeja. ADM-006 cobre o aviso.
- **UAC a cada abertura:** consequência aceita (ADR 0024). Não há como evitar sem tarefa agendada, descartada.
- **`IsProcessElevated` em processo de outro usuário:** o agente roda na mesma conta, então o acesso é permitido; se
  falhar, a UI trata como "não elevado" e mostra o aviso. Registrado como caso de teste.
