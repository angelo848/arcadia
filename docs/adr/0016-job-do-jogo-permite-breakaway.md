# 0016 — O Job Object do jogo permite breakaway

- **Status:** aceito
- **Data:** 2026-11-09

## Contexto
Alguns jogos e launchers criam processos com `CREATE_BREAKAWAY_FROM_JOB`. Se o Job não permitir, o
`CreateProcess` desses processos falha e o jogo pode quebrar. Se permitir, o processo sai do Job e não segura o
tempo da sessão (Camada 1, ADR 0005). Spec: [docs/specs/phase-3-tracker](../specs/phase-3-tracker/requirements.md).

## Decisão
O Job do jogo terá `JOB_OBJECT_LIMIT_BREAKAWAY_OK`, sem `JOB_OBJECT_LIMIT_SILENT_BREAKAWAY_OK`. Só sai do Job o
processo que pedir breakaway explicitamente. Ele roda normalmente, mas não é rastreado.

## Alternativas consideradas
- **Bloquear breakaway:** o tempo fica sempre completo, mas o jogo pode falhar ao abrir.
- **SILENT_BREAKAWAY_OK:** todo filho sairia do Job e a Camada 1 não rastrearia nada.

## Consequências
- ✅ Nenhum jogo deixa de funcionar por causa do rastreamento.
- ⚠️ Um launcher que abre o jogo com breakaway e fecha em seguida tem o tempo cortado. A Camada 2 (watcher, fase 6)
  cobre esse caso.
