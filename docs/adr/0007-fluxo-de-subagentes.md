# 0007 — Fluxo de subagentes com separação autor/teste

- **Status:** aceito
- **Data:** 2026-11-07

## Contexto
O código é escrito com subagentes do Claude Code. Quando o mesmo agente escreve código e testes, os testes tendem
a confirmar a implementação em vez da spec. A área de rastreamento exige conhecimento específico de Win32.

## Decisão
Cinco subagentes em `.claude/agents/`, com permissões aplicadas por hooks (`.claude/hooks/`):

| Agente | Escreve em | Shell |
|---|---|---|
| planner | nada | nenhum |
| test-writer | `*_test.go`, `*.test.ts(x)`, `testdata/` | `go test/vet/build/list/doc`, vitest |
| implementer | tudo, exceto testes, `internal/tracker/`, `cmd/fake-game/`, `.claude/`, `docs/adr/` | go, wails, sqlc, npm, git leitura |
| tracker-specialist | `internal/tracker/`, `cmd/fake-game/` (sem testes) | go, git leitura |
| reviewer | nada | git leitura, `go vet/test` |

Fluxo: spec → planner → aprovação do usuário → test-writer → implementer/tracker-specialist → reviewer.
`gofmt -w` roda após toda edição de `.go`; `go test ./...` bloqueia o fim do implementer e do tracker-specialist.

## Alternativas consideradas
- **Um único agente com revisão manual** — mais rápido, mas sem a garantia de que os testes vêm da spec.
- **`go test` no Stop do test-writer** — conflita com TDD (os testes ficam vermelhos de propósito até a implementação).

## Consequências
- ✅ Testes derivados da spec; tracker isolado; formatação e testes garantidos por hook.
- ⚠️ Mais etapas por entrega; o shell dos agentes não aceita encadeamento/pipe (`bash-guard.ps1`).
