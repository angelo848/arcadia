# Arcadia — launcher de jogos para PC (Windows primeiro)

Inspirado no LaunchBox, com foco em **rastreamento de tempo de jogo robusto** (árvore de processos via Job Object,
watcher de processos, pausa por teclado/controle, "iniciar sem contar tempo").

- Arquitetura completa: [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)
- Decisões registradas: [docs/adr/](docs/adr/README.md)
- Roadmap das fases 9 a 13: [docs/ROADMAP.md](docs/ROADMAP.md)
- **Versão 1.0.0** (2026-11-16): primeira versão definitiva, com as fases 1 a 14 no código.
- **Versão 1.0.1** (2026-11-16): correções e ajustes da grade, da tela cheia e do foco do jogo (ver ROADMAP).
- **Versão 1.0.2** (2026-11-16): o jogo em primeiro plano não fica mais preso acima das outras janelas.
- **Versão 1.0.3** (2026-11-17): importar por URI, carrossel ampliado, abrir local do arquivo, importador da Epic novo e janelas que fecham após a ação.
- **Versão 1.1.0** (2026-11-22): Select + A abre o Big Box (e `--bigbox`), tempo para zerar do HowLongToBeat (reserva do IGDB), temas Mosaico e Estante do Big Box.
- **Versão 1.2.0** (2026-11-25): criar, editar e excluir sessões de um jogo na lista de sessões do painel de detalhes (origem Manual, marca Editada, total por diferença, sem sobreposição) e estatísticas só com sessões contadas.
- **Versão 1.3.0** (2026-11-27): tema **Desktop** do Big Box (Plataformas, jogos e painel de detalhes em colunas, igual à janela normal, navegável pelo controle; só "Jogar" e "Jogar sem contar tempo"), sessões em páginas de 10 e criação de sessão só com início e tempo jogado.
- Spec atual: [docs/specs/phase-17-tema-desktop/](docs/specs/phase-17-tema-desktop/requirements.md) — **Tema Desktop do Big Box** (layout em colunas, zonas no controle, ADR 0042) (sobre [phase-16-edicao-sessoes](docs/specs/phase-16-edicao-sessoes/requirements.md) — **Edição de sessões** (criar, editar e excluir sessões, `session.create/update/delete`, migração 11) (sobre [phase-15-big-box-v2](docs/specs/phase-15-big-box-v2/requirements.md) — **Big Box v2** (Select + A, tempo para zerar, temas Mosaico e Estante, contrato de tema v2) (sobre [phase-14-edicao-manual](docs/specs/phase-14-edicao-manual/requirements.md) — **Edição manual** (imagens e tempo total jogado na janela de edição do jogo) (sobre [phase-13-modo-console](docs/specs/phase-13-modo-console/requirements.md) — **Modo console** (atalho Select + B, Big Box novo com tema Faixa e vídeo do ScreenScraper) (sobre [phase-12-metadados-unificados](docs/specs/phase-12-metadados-unificados/requirements.md) — **Metadados unificados** (busca única entre provedores, lote logo após a importação, janela de confirmação dos matches) (sobre [phase-11-importacao-v2](docs/specs/phase-11-importacao-v2/requirements.md) — **Importação v2** (normalização de títulos, subpastas, discos agrupados, janela de revisão), sobre [phase-10-2-imagens-ao-vivo](docs/specs/phase-10-2-imagens-ao-vivo/requirements.md) — **Imagens ao vivo** (aviso `media.changed` do agente para a UI) (sobre [phase-10-1-ajustes](docs/specs/phase-10-1-ajustes/requirements.md) — **Ajustes da nova interface e imagens do LaunchBox**, sobre [phase-10-nova-interface](docs/specs/phase-10-nova-interface/requirements.md) — **Nova interface** (três colunas, jogo em foco, tema escuro por tokens, modais proporcionais, janela maximizada) (sobre [phase-9-plataformas](docs/specs/phase-9-plataformas/requirements.md), [phase-8-estatisticas](docs/specs/phase-8-estatisticas/requirements.md), [phase-7-midia](docs/specs/phase-7-midia/requirements.md), [phase-6b-emulators](docs/specs/phase-6b-emulators/requirements.md), [phase-6-importers](docs/specs/phase-6-importers/requirements.md), [phase-5-1-ajustes](docs/specs/phase-5-1-ajustes/requirements.md), [phase-5-library](docs/specs/phase-5-library/requirements.md), [phase-4-pause](docs/specs/phase-4-pause/requirements.md),
  [phase-3-tracker](docs/specs/phase-3-tracker/requirements.md), [single-exe](docs/specs/single-exe/requirements.md) e a fase 2, [phase-2-skeleton](docs/specs/phase-2-skeleton/requirements.md)))
- Processo: **Spec-Driven Development** ([ADR 0008](docs/adr/0008-spec-driven-development.md)) — a spec é a fonte da verdade.

## Stack

Go 1.27 (`golang.org/x/sys/windows`) · Wails v2 + React/TS/Vite · SQLite via `modernc.org/sqlite` (sem CGO),
migrações `goose`, queries `sqlc` · named pipe (`go-winio`) + JSON-RPC 2.0 entre agente e UI · `log/slog`.

Um executável, dois processos ([ADR 0014](docs/adr/0014-executavel-unico-dois-processos.md)): `arcadia.exe --agent` (bandeja; lança jogos, rastreia tempo,
atalhos, único escritor do banco e dono das migrações; não inicia com o Windows, [ADR 0024](docs/adr/0024-agente-sempre-como-administrador.md): fica na bandeja enquanto o Arcadia está em uso, e o Select + A só funciona com ele rodando) e `arcadia.exe` (UI Wails, instância
única; pede tudo ao agente por IPC, [ADR 0015](docs/adr/0015-agente-unico-escritor-do-banco.md); sobe/relança o agente).
"Sair" na bandeja fecha os dois. **A UI nunca influencia o tempo de jogo.**
`cmd/fake-game` é o jogo falso dos testes de rastreamento ([contrato](docs/specs/phase-3-tracker/contracts/fake-game.md)).

## Layout

```
cmd/arcadia  cmd/fake-game
internal/{agent,domain,store,tracker,input,feedback,ipc,scrapers,importers,winapi,paths,logging}
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
cd cmd/arcadia && wails build -ldflags "-X main.version=1.3.0"   # gera build/bin/arcadia.exe (versão atual: 1.3.0, também em wails.json)
sqlc generate                      # depois de alterar internal/store/queries/*.sql
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/spec-trace.ps1 -Spec docs/specs/phase-4-pause
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
