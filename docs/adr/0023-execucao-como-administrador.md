# 0023 — Execução como administrador

- **Status:** aceito (aprovado com a spec [phase-5-1-ajustes](../specs/phase-5-1-ajustes/requirements.md), testado em 2026-11-12)
- **Data:** 2026-11-12

## Contexto
Alguns jogos têm `requireAdministrator` no manifesto. Lançados sem elevação, falham com `ERROR_ELEVATION_REQUIRED`
(740). A limitação 4 da fase 3 registra o problema: elevar só o jogo quebra a árvore do Job Object, porque o agente não
elevado não controla um processo de integridade alta. O rastreio (Job Object, pausa e filhos) precisa continuar igual.

## Decisão
- O `arcadia.exe` declara `requestedExecutionLevel="requireAdministrator"` no manifesto. Assim, a UI e o agente sobem
  sempre elevados; o Windows pede a confirmação (UAC) a cada abertura.
- O agente que a UI inicia herda a elevação. Todos os jogos que ele lança rodam elevados, com o mesmo rastreio das fases
  3 e 4.
- Não há campo por jogo nem lançamento com `runas`. Um jogo que exige administrador roda sem nenhum tratamento especial.
- O início automático com o Windows é removido ([ADR 0024](0024-agente-sempre-como-administrador.md)).

## Alternativas consideradas
- **Elevar só o jogo (`runas` por processo):** quebra o Job Object e a pausa; o agente não elevado não controla o processo.
  Descartada.
- **Campo `requiresAdmin` por jogo, só para avisar:** exige um campo e uma tela que a decisão de produto dispensou.
  Descartada.
- **Processo auxiliar elevado só para jogos de administrador:** cumpriria o rastreio sem elevar o agente, mas adiciona um
  terceiro processo, um canal privilegiado e instalação com privilégio. Custo alto para um caso de borda; fica como
  possibilidade futura.

## Consequências
- ✅ Rastreio, pausa e filhos continuam como estão para todos os jogos, sem código novo de elevação.
- ✅ Nenhuma migração e nenhum campo novo; `game.update` não muda.
- ⚠️ Todo jogo roda com privilégio de administrador, inclusive os que não precisam. Aceito pelo usuário.
- ⚠️ O UAC aparece a cada abertura do Arcadia. Se a elevação for recusada, nada sobe.
- ⚠️ Build `dev` não tem manifesto elevado: o `go run` e o `wails dev` continuam sem elevação, o que é útil para desenvolver.
