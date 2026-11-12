# Fase 6b — Emuladores: design

- **Requisitos:** [requirements.md](requirements.md) (aprovados) · **Tarefas:** [tasks.md](tasks.md)
- **Banco:** `emulator(id, name, exe_path, args_template)` e `platform.emulator_id` já existem (migração 1). Jogo: novo
  campo opcional `emulator_id` (migração 6). `launch_type = emulator` é o novo tipo; `launch_target` é a ROM.
- **Contrato:** v0.8.0 (sobre a v0.7.0 da fase 6): métodos `emulator.list`, `emulator.add`, `emulator.update`,
  `emulator.delete`, `platform.setEmulator`; `game.update` ganha `emulatorId` (nulo = usar o padrão da plataforma).

## 1. Resolução do emulador
`store.ResolveEmulator(gameID)`: se o jogo tem `emulator_id`, usa esse; senão usa `platform.emulator_id`; sem nenhum,
erro `ErrNoEmulator` (EMU-005). O agente resolve antes de lançar.

## 2. Lançamento (EMU-004)
- `handleGameLaunch` aceita `launch_type = emulator`: resolve o emulador, valida que `exe_path` é absoluto e existe
  (`os.Stat`, como em `game.add`), monta os argumentos trocando `{rom}` pelo alvo e `{dir}` pela pasta da ROM.
- O resultado vira um `tracker.LaunchSpec` `KindExe` com `Exe = emulator.exe_path`, `Args` montados, `WorkDir` = pasta
  do emulador. Assim a máquina de estados, a pausa e a carência não mudam (SES-001, RUL-005 vale).
- Erros: `ErrNoEmulator` → `-32003`-like mensagem "Não há emulador configurado para esta plataforma"; caminho
  inexistente → "O emulador não foi encontrado em <caminho>" (EMU-005).

## 3. Argumentos
Tokenização do modelo com `windows.EscapeArg` para cada valor substituído; `{rom}` e `{dir}` são os únicos marcadores.
Marcador desconhecido, como `{foo}`, é recusado no cadastro (-32602) com a lista de marcadores válidos.

## 4. Importação do LaunchBox (EMU-003)
- Remove-se a exclusão de jogos com `Emulator`. Candidato: `LaunchType = emulator`, `LaunchTarget = ApplicationPath` (ROM),
  `Platform` = plataforma do jogo. A constante nova `LaunchEmulator = "emulator"` vai para `internal/importers/types.go`.
- O apply grava `launch_type = emulator`; a plataforma é criada na categoria `console` (já implementado na 6.1).
- Sem emulador configurado a página mostra o aviso (EMU-003); a associação posterior não reimporta nada.

## 5. Cadastro, plataforma e UI (EMU-001, EMU-002, EMU-006)
- Tabela `emulator`: `name` único sem diferença de maiúsculas (índice `COLLATE NOCASE`, migração 6), `exe_path`
  absoluto, `args_template` até 2000 caracteres com validação de marcadores (`domain.ValidateArgsTemplate`).
- Apagar emulador: `ON DELETE SET NULL` em `platform` e em `game.emulator_id`.
- UI: nova tela `EmulatorsDialog` (botão no cabeçalho): lista, cadastro, edição, exclusão com confirmação (como
  DEL-001), e seletor de emulador padrão por plataforma. A página do jogo ganha o seletor "Emulador deste jogo"
  (padrão = "usar o da plataforma").

## 6. Testes
- `domain`: `ValidateArgsTemplate` (marcadores válidos/inválidos, tamanho) e `BuildArgs` (troca de `{rom}`/`{dir}`, aspas).
- `store`: resolução (jogo > plataforma > erro), `ON DELETE SET NULL`, unicidade sem diferença de maiúsculas.
- `agent`: lançamento por emulador com fake de tracker (args, WorkDir, erros EMU-005).
- `importers`: jogos com emulador viram candidatos `emulator` (e o aviso some).
- `frontend`: tela de emuladores (cadastro e recusa de marcador) e seletor na página do jogo.
- Manual: lançar um jogo de SNES com um emulador real (EMU-004) e conferir o tempo.

## 7. Riscos
- Emuladores que delegam a outro processo caem no mesmo caso de RUL-005 (sem Job para o filho). Informado na página
  do jogo quando for o caso; não é detectado automaticamente nesta fase.
- Caminhos de ROM com caracteres especiais: tratados pelo `EscapeArg`.
