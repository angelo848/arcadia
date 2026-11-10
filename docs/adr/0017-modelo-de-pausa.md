# 0017 — Modelo de pausa da sessão

- **Status:** aceito
- **Data:** 2026-11-10

## Contexto
A fase 4 acrescenta pausa ao rastreamento (atalho, controle, botão da UI e suspensão do PC). É preciso decidir o que
a pausa significa para o jogo, para o banco e para a recuperação após falhas. Spec: [docs/specs/phase-4-pause](../specs/phase-4-pause/requirements.md).

## Decisão
- **Pausar não suspende o jogo.** Só o tempo deixa de contar; o Job Object não é tocado.
- O tempo ativo é acumulado em memória pela máquina de estados (novo estado `Paused`) e gravado em `active_seconds`.
  `pause_interval` é registro auxiliar, com **no máximo uma pausa aberta por sessão** (índice único parcial, migração 4).
- O heartbeat continua em `Paused`. `EndSession` fecha a pausa aberta na mesma transação (pausa de tamanho zero é
  removida). Job vazio em pausa vira `Grace`, e a pausa fecha nesse instante.
- Motivos: `hotkey`, `gamepad`, `sleep` e `ui`. `idle` e `minimized` ficam reservados.
- Suspensão do PC pausa com motivo `sleep`. Ao acordar, a sessão só volta a contar quando o Windows indica usuário
  presente; uma pausa do usuário feita antes de dormir continua pausada.
- `session.pause` e `session.resume` (UI) são idempotentes e passam pelo mesmo caminho dos atalhos.

## Alternativas consideradas
- **Suspender os processos do jogo:** quebra jogos online e launchers; o objetivo é só não contar o tempo.
- **Retomar sempre ao acordar:** contaria tempo de despertares sem ninguém.
- **Nunca retomar ao acordar:** o usuário perderia tempo jogado ao esquecer de retomar.

## Consequências
- ✅ Tempo sempre recalculável a partir de sessão e pausas (STO-023); recuperação trivial após queda.
- ⚠️ Em PC com Modern Standby o aviso de usuário presente pode não chegar: a sessão fica pausada até o atalho.
- ⚠️ Suspensão que o Windows não avisa conta como tempo jogado (fora de escopo detectar por lacuna de heartbeat).
