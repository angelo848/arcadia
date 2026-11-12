# 0021 — Remoção de jogo

- **Status:** aceito
- **Data:** 2026-11-12

## Contexto
O usuário pode remover um jogo da biblioteca. O jogo tem sessões, pausas e vínculos (plataforma, gênero, empresa, modo),
e pode haver uma sessão ativa no momento do pedido. Spec: [docs/specs/phase-5-library](../specs/phase-5-library/requirements.md).

## Decisão
- `game.delete` é atendido pelo agente (ADR 0015). Apaga o jogo e, em cascata, suas sessões, pausas e vínculos, e remove
  o jogo do índice de busca (ADR 0020), tudo na mesma transação.
- Se houver sessão ativa do jogo, a remoção é recusada com `ErrGameInUse` e a UI mostra a mensagem de DSK-018.
- A verificação de sessão ativa e a remoção rodam dentro de `tracker.WithoutActiveGame`, com prazo de 5 s. Se o prazo
  vencer antes de a remoção começar, ela não roda. Se já começou, o pedido aguarda o resultado e responde sucesso ou erro
  real, nunca "sucesso" sem a remoção ter ocorrido.
- A UI exige confirmação antes do envio (diálogo com foco preso, DSK-018 e UI-041).

## Alternativas consideradas
- **Remoção lógica (flag `deleted`):** mantém sessões órfãs visíveis e complica a consulta de tempo; o usuário pediu
  remoção de fato.
- **Responder sucesso ao vencer o prazo:** deixaria a UI mostrando um jogo removido que ainda existe.

## Consequências
- ✅ Nenhuma sessão ativa é apagada no meio da contagem de tempo; a regra "um jogo por vez" vale também para remoção.
- ⚠️ A remoção de um jogo com muitas sessões pode levar mais que o prazo; nesse caso o pedido ainda aguarda o resultado.
