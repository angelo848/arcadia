# Arcadia — launcher de jogos para PC (Windows primeiro)

Inspirado no LaunchBox, com foco em **rastreamento de tempo de jogo robusto** (árvore de processos via Job Object,
watcher de processos, pausa por teclado/controle, "iniciar sem contar tempo").

- Arquitetura completa: [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)
- Decisões registradas: [docs/adr/](docs/adr/README.md)
- Spec atual: [docs/specs/phase-3-tracker/](docs/specs/phase-3-tracker/requirements.md) — **Tracker MVP** (sobre
  [single-exe](docs/specs/single-exe/requirements.md) e a fase 2, [phase-2-skeleton](docs/specs/phase-2-skeleton/requirements.md))
- Processo: **Spec-Driven Development** ([ADR 0008](docs/adr/0008-spec-driven-development.md)) — a spec é a fonte da verdade.

## Stack

Go 1.27 (`golang.org/x/sys/windows`) · Wails v2 + React/TS/Vite · SQLite via `modernc.org/sqlite` (sem CGO),
migrações `goose`, queries `sqlc` · named pipe (`go-winio`) + JSON-RPC 2.0 entre agente e UI · `log/slog`.

Um executável, dois processos ([ADR 0014](docs/adr/0014-executavel-unico-dois-processos.md)): `arcadia.exe --agent` (bandeja; lança jogos, rastreia tempo,
atalhos, único escritor do banco e dono das migrações; inicia com o Windows) e `arcadia.exe` (UI Wails, instância
única; pede tudo ao agente por IPC, [ADR 0015](docs/adr/0015-agente-unico-escritor-do-banco.md); sobe/relança o agente).
"Sair" na bandeja fecha os dois. **A UI nunca influencia o tempo de jogo.**
`cmd/fake-game` é o jogo falso dos testes de rastreamento ([contrato](docs/specs/phase-3-tracker/contracts/fake-game.md)).

## Layout

```
cmd/arcadia  cmd/fake-game
internal/{agent,domain,store,tracker,input,ipc,scrapers,importers,winapi,paths,logging}
frontend/  scripts/  docs/{adr,specs}
docs/specs/<feature>/{requirements.md,design.md,tasks.md,contracts/}
```

## Comandos

```bash
go build ./...
go test ./...
go vet ./...
go run ./cmd/arcadia --agent --data-dir ./.devdata   # só o agente (build "dev" não mexe no registro)
cd cmd/arcadia && wails dev        # UI em modo dev
cd cmd/arcadia && wails build -ldflags "-X main.version=<v>"   # gera build/bin/arcadia.exe
sqlc generate                      # depois de alterar internal/store/queries/*.sql
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/spec-trace.ps1 -Spec docs/specs/phase-3-tracker
```

`spec-trace.ps1` confere requisito ↔ tarefa ↔ teste (`-SkipTests` antes da etapa de testes).

Se `go` não estiver no PATH da sessão, use `"C:\Program Files\Go\bin\go.exe"` (reinicie o terminal/app após instalar).

## Convenções de código

- **Código, identificadores e comentários em inglês; docs, specs, ADRs e mensagens de commit em português.**
- Sem CGO (`CGO_ENABLED=0` deve compilar). Código Win32 só em arquivos `//go:build windows` ou `*_windows.go`;
  `domain` e `store` não importam nada específico de Windows.
- Todo `windows.Handle` aberto tem dono claro e é fechado (`defer windows.CloseHandle(h)` logo após o erro checado).
  Nada de handle "vazando" para fora de uma struct sem método `Close()`.
- Toda goroutine tem dono e condição de saída (`context.Context` ou canal de stop); quem a inicia espera ela terminar
  no shutdown (`sync.WaitGroup`/`errgroup`). Nada de `go func()` solto.
- Tempo sempre via interface `domain.Clock` — proibido `time.Now()`/`time.Sleep()` em lógica testável.
- Erros: `fmt.Errorf("contexto: %w", err)`; sem `panic` fora de `main`. Log com `slog`, sem `fmt.Println`.
- Testes table-driven, `t.TempDir()` para SQLite, sem dependência de rede.
- Mudou uma decisão de arquitetura? Escreva um ADR novo (não edite o antigo; marque-o como substituído).

## Fluxo de trabalho com subagentes

Definidos em [.claude/agents/](.claude/agents/). Ordem padrão para cada feature (SDD):

0. **spec-writer** (só `docs/specs/`) escreve `requirements.md` (IDs `ÁREA-NNN` + Dado/Quando/Então), `design.md`
   e `contracts/`. **O usuário aprova os requisitos.**
1. **planner** (só leitura) devolve o `tasks.md`: tarefas citando IDs, arquivos, assinaturas públicas e testes.
   A sessão principal grava em `docs/specs/<feature>/tasks.md` — **nada é implementado sem tarefas aprovadas pelo usuário.**
2. **test-writer** escreve os testes (só arquivos de teste); **cada caso começa pelo ID** (`t.Run("IPC-002/...")`).
   Testes vermelhos nesta etapa são esperados.
3. **implementer** escreve o código até os testes passarem. Não edita `*_test.go` nem `internal/tracker/`.
4. **tracker-specialist** é o único que escreve em `internal/tracker/` (e `cmd/fake-game/`).
5. **reviewer** (só leitura) revisa o diff: bugs, goroutines sem dono, handles do Windows não fechados, aderência à spec.
   Achados voltam ao implementer/tracker-specialist.

Regras de ouro: **quem escreve o código não escreve o teste dele**, e **mudança de comportamento (ou bug) começa
na spec** — implementer e tracker-specialist propõem a mudança em vez de desviar no código.

## Hooks (`.claude/hooks/`, PowerShell)

- `gofmt.ps1` — PostToolUse em Edit/Write: roda `gofmt -w` em todo `.go` editado (qualquer agente ou sessão).
- `gotest.ps1` — Stop do implementer e do tracker-specialist: roda `go test ./...` e impede o agente de terminar
  enquanto falhar (até 3 tentativas; depois libera e avisa).
- `path-guard.ps1` / `bash-guard.ps1` — aplicam as permissões de escrita e de shell de cada agente.
