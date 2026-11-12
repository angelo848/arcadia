# 0023 — Execução de jogos como administrador

- **Status:** proposto (aguarda aprovação do usuário junto com a spec [phase-5-1-ajustes](../specs/phase-5-1-ajustes/requirements.md))
- **Data:** 2026-11-12

## Contexto
Alguns jogos têm `requireAdministrator` no manifesto. Lançados sem elevação, falham com `ERROR_ELEVATION_REQUIRED`
(740). A limitação 4 da fase 3 registra o problema: elevar o processo quebra a árvore do Job Object, porque o agente não
elevado não consegue colocar nem abrir com acesso total um processo de integridade alta.

## Decisão
- Campo `runAsAdmin` por jogo (padrão `false`), editável na página de edição.
- Quando `true`, o agente lança pelo verbo `runas` do `ShellExecuteEx`, com `SEE_MASK_NOCLOSEPROCESS`, e o Windows
  mostra o UAC.
- O tempo é medido pelo ciclo de vida do processo principal: o agente espera o handle com `SYNCHRONIZE`, que é permitido
  de processo não elevado para processo elevado. Sem Job Object, sem pausa por entrada e sem contagem de filhos nesse modo.
- A limitação aparece na UI (ADM-004), e não é escondida.

## Alternativas consideradas
- **Agente elevado (sempre):** o agente inicia com o Windows; elevar tudo amplia a superfície de ataque. Descartada.
- **Serviço elevado auxiliar:** cumpriria o rastreio completo, mas adiciona um terceiro processo, um canal privilegiado e
  instalação com privilégio. Custo alto para um caso de borda; fica como possibilidade futura.
- **Não rastrear:** é o comportamento atual; o usuário pediu suporte.

## Consequências
- ✅ Jogos elevados passam a ser lançáveis, sem alterar o agente nem o instalador.
- ⚠️ Nenhum filho e nenhuma pausa nesse modo: o tempo medido é o do processo principal, o que pode subcontar jogos
  que delegam a um lançador filho.
- ⚠️ Recusa do UAC é tratada como erro de lançamento, sem sessão.
