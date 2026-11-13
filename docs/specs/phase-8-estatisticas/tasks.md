# Fase 8 — Estatísticas e modo Big Box: tarefas

Rascunho para aprovação. Uma tarefa por commit, testes antes da implementação. Requisitos em
[requirements.md](requirements.md), design em [design.md](design.md).

## T01 — Contrato 0.10.0 e consultas de estatísticas na store
Requisitos: EST-001, EST-003, EST-004, EST-005.
Testes: `EST-003/agregação por plataforma`, `EST-004/período 7d e tudo`, `EST-005/importado entra e é marcado`.

## T02 — Agregação semanal (ISO, horário local)
Requisitos: EST-002. Testes: `EST-002/12 semanas com zeros`, `EST-002/semana atual marcada`.

## T03 — Handlers stats.summary e stats.weekly no agente
Requisitos: EST-001, EST-002, EST-006. Testes: `EST-006/nenhum método de escrita`, contrato 0.10.0.

## T04 — Tela de estatísticas e gráfico SVG
Requisitos: EST-001, EST-002, EST-003, EST-004, EST-006. Testes: `EST-004/recalcula os três blocos`, `EST-001/ir para o jogo`.

## T05 — Modo Big Box: entrada, saída e layout
Requisitos: BIG-001, BIG-003. Testes: `BIG-001/B e Esc voltam na mesma posição`, `BIG-003/ações alcançáveis por teclado`.

## T06 — Navegação por controle (Gamepad API)
Requisitos: BIG-002. Testes: `BIG-002/direcional move o foco`, `BIG-002/A abre, Y inicia`.

## T07 — Verificação
Requisitos: todos. `go test`, `vitest`, build, `spec-trace` e checagens manuais na TV (BIG-002, BIG-004).
