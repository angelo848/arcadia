# 0023 — Execução de jogos que exigem administrador

- **Status:** proposto (aguarda aprovação do usuário junto com a spec [phase-5-1-ajustes](../specs/phase-5-1-ajustes/requirements.md))
- **Data:** 2026-11-12

## Contexto
Alguns jogos têm `requireAdministrator` no manifesto. Lançados sem elevação, falham com `ERROR_ELEVATION_REQUIRED`
(740). A limitação 4 da fase 3 registra o problema: elevar só o jogo quebra a árvore do Job Object, porque o agente não
elevado não consegue controlar um processo de integridade alta. O rastreio (Job Object, pausa e filhos) precisa
continuar igual.

## Decisão
- O agente roda elevado quando o usuário o inicia como administrador (a UI, ao ser aberta como administrador, inicia o
  agente com a mesma elevação). Nenhum processo é elevado por conta própria pelo Arcadia.
- Jogos sem a marca também herdam a elevação do agente. Isso é aceito pelo usuário como consequência.
- Campo `requiresAdmin` por jogo (padrão `false`), usado para avisar antes do lançamento quando o agente não está elevado
  (ADM-002) e para o erro 740 (ADM-003). Não muda a forma de lançar.
- O início automático com o Windows (ADR 0014) continua como está. Ele sobe o agente sem elevação, então quem usa jogos
  de administrador desliga essa opção. Quando há conflito, a UI não usa o agente antigo (ADM-005).

## Alternativas consideradas
- **Elevar só o jogo (`runas` por processo):** a elevação quebra o Job Object e a pausa; o agente não elevado não controla
  o processo. Descartada.
- **Processo auxiliar elevado só para jogos de administrador:** cumpriria o rastreio sem elevar o agente, mas adiciona
  um terceiro processo, um canal privilegiado e instalação com privilégio. Custo alto para um caso de borda; fica como
  possibilidade futura.

## Consequências
- ✅ Rastreio, pausa e filhos continuam como estão para todos os jogos.
- ✅ Nenhum código novo de elevação no agente; o Windows decide a elevação de quem inicia.
- ⚠️ Com o agente elevado, todos os jogos rodam com privilégio de administrador. Quem não quer isso abre o Arcadia sem
  elevação.
- ⚠️ O início automático com o Windows conflita com a abertura elevada; por isso a orientação e o aviso da UI (ADM-005).
