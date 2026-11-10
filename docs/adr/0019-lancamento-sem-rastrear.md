# 0019 — Lançamento sem rastrear

- **Status:** aceito
- **Data:** 2026-11-10

## Contexto
O usuário quer abrir um jogo sem contar tempo ("Sem contar tempo"). Diferente de `game.launch`, não há sessão a
devolver, e o processo não deve ficar preso ao agente. Spec: [docs/specs/phase-4-pause](../specs/phase-4-pause/requirements.md).

## Decisão
- Método separado `game.launchUntracked`, com resultado `{pid}` (não uma `Session`).
- O processo é lançado desacoplado: sem Job Object, sem `play_session`, sem `session.changed`; a UI não o mostra
  como "Em execução". Os handles do lançamento são fechados (TRK-011).
- É recusado com `-32002` quando há qualquer sessão ativa (Running, Grace ou Paused) ou durante o shutdown, mantendo
  "um jogo por vez" nos dois botões.

## Alternativas consideradas
- **Flag `track: false` em `game.launch`:** o resultado e os erros têm formas diferentes; um método à parte é mais claro.
- **Permitir com sessão ativa:** o jogo A contaria enquanto o usuário joga o B sem contar.

## Consequências
- ✅ Contrato simples e sem estado novo no tracker.
- ⚠️ Abrir "Jogar" com um jogo sem contar já aberto não é detectável: as duas coisas rodam ao mesmo tempo.
