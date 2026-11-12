# Fase 6b — Emuladores: tarefas

- **Status:** rascunho, aguardando aprovação das tarefas.
- **Requisitos:** [requirements.md](requirements.md) · **Design:** [design.md](design.md)

## T01 — Domínio: marcadores e argumentos (EMU-001, EMU-004)
- **Arquivos:** `internal/domain/emulator.go`, `emulator_test.go`.
- **Testes:** `EMU-001/modelo aceita {rom} e {dir}`, `EMU-001/marcador desconhecido é recusado`,
  `EMU-004/BuildArgs troca {rom} e {dir} com escape`, `EMU-001/modelo acima de 2000 runas é recusado`.
- **Dependência:** nenhuma.

## T02 — Migração 6 e store de emuladores (EMU-001, EMU-002)
- **Arquivos:** `internal/store/migrations/00006_emulators.sql` (`game.emulator_id`, índice `NOCASE` de nome),
  `queries/emulators.sql` (sqlc), `internal/store/emulators.go`, `emulators_test.go`.
- **Testes:** `EMU-001/nome duplicado sem diferença de maiúsculas é recusado`, `EMU-002/apagar emulador zera
  platform.emulator_id e game.emulator_id sem apagar jogos`, `EMU-002/platform.setEmulator aponta para emulador existente`.
- **Dependência:** T01.

## T03 — Resolução do emulador (EMU-002, EMU-005)
- **Arquivos:** `internal/store/emulator_resolve.go`, teste.
- **Testes:** `EMU-002/jogo com emulador próprio ganha do padrão da plataforma`,
  `EMU-005/sem emulador nenhum devolve ErrNoEmulator`.
- **Dependência:** T02.

## T04 — Lançamento por emulador no agente (EMU-004, EMU-005)
- **Arquivos:** `internal/agent/games.go` (aceita `emulator`, monta a spec `KindExe`), `internal/agent/emulator_launch.go`, teste.
- **Testes:** `EMU-004/lança o emulador com os argumentos do modelo`, `EMU-005/caminho do emulador inexistente é erro
  com o caminho`, `EMU-005/sem emulador não cria sessão`.
- **Dependência:** T03.

## T05 — Importador do LaunchBox com emulador (EMU-003)
- **Arquivos:** `internal/importers/launchbox.go` (sai a exclusão; candidato `emulator`), `internal/importers/types.go`
  (`LaunchEmulator`), `launchbox_test.go` (teste antigo de exclusão é trocado), `internal/agent/imports.go` (aceita `emulator`).
- **Testes:** `EMU-003/jogo com emulador vira candidato launch emulator com a ROM como alvo`,
  `EMU-003/sem aviso de emulador na importação`.
- **Dependência:** T02.

## T06 — Contrato v0.8.0 e handlers (EMU-001, EMU-002)
- **Arquivos:** `docs/specs/phase-6b-emulators/contracts/ipc.openrpc.json` (v0.8.0), `internal/ipc/emulator.go`,
  `internal/ipc/contract_phase6b_test.go`, `internal/agent/emulators.go` (handlers), registro em `app.go`.
- **Testes:** `EMU-001/emulator.add recusa nome vazio com -32602`, `EMU-002/platform.setEmulator grava o padrão`,
  `EMU-001/contrato v0.8.0 tem os métodos novos`.
- **Dependência:** T02, T03.

## T07 — Interface de emuladores e seletor do jogo (EMU-002, EMU-006)
- **Arquivos:** `cmd/arcadia/emulators.go` (bindings), `frontend/src/lib/emulators.ts`,
  `frontend/src/components/EmulatorsDialog.tsx` (+ teste), `GameEditForm.tsx` (seletor "Emulador deste jogo").
- **Testes:** `EMU-006/cadastro recusa marcador desconhecido`, `EMU-006/apagar pede confirmação`,
  `EMU-002/seletor do jogo usa o padrão da plataforma por padrão`.
- **Dependência:** T06.

## T08 — Verificação final da 6b
- `go build`, `go vet` (Windows e Linux), `go test ./...`; `npm test`, `tsc`, `npm run build`.
- `wails build -ldflags "-X main.version=0.7.0"`, `spec-trace.ps1` para `phase-6b-emulators`.
- Manual (usuário): EMU-004 com um emulador real de SNES, e conferir o tempo e a página do jogo.
- **Dependência:** T01..T07.

## Ordem
T01 → T02 → (T03, T05 em paralelo) → T04 → T06 → T07 → T08.
