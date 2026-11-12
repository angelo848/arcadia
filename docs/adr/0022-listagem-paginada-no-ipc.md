# 0022 — Listagem paginada no IPC

- **Status:** aceito
- **Data:** 2026-11-12

## Contexto
A biblioteca pode ter muitos jogos, e a resposta do IPC tem um limite de tamanho por mensagem de 1 MiB. A grade é
virtualizada no frontend, então só precisa das linhas visíveis mais uma margem. Spec:
[docs/specs/phase-5-library](../specs/phase-5-library/requirements.md).

## Decisão
- `game.list` é paginado por `offset` e `limit`, com ordenação e filtro explícitos no pedido (`FilterSpec` versionado, ADR 0020).
- O agente monta cada página com um orçamento de **1 MiB em bytes** (`fitGamesPage`): inclui jogos enquanto couber, e
  informa a próxima posição. Um limite de quantidade sozinho não garante o teto de tamanho, porque os textos variam.
- A lista nunca é `null`: uma página vazia vai como `[]` (`keepEmptyArray` em `dispatch.go`).
- No frontend, `useInfiniteGames` pede a próxima página quando o usuário se aproxima do fim (`PREFETCH_AHEAD = 10`),
  e a lista só habilita depois que o status do agente chega (UI-036).

## Alternativas consideradas
- **Lista inteira de uma vez:** arrisca estourar o limite do IPC com bibliotecas grandes.
- **Paginação só por quantidade:** resposta pode passar de 1 MiB com descrições longas.
- **Cursor opaco:** mais robusto a inserções durante a rolagem, mas a ordenação por campos repetidos exige chave
  composta; o offset basta para uma biblioteca local, e a ordenação é estável por id como desempate.

## Consequências
- ✅ O teto de 1 MiB vale para qualquer biblioteca; o frontend nunca recebe mensagem fora do limite.
- ⚠️ Com inserção durante a rolagem, uma página pode repetir ou pular um jogo. Aceito para uma biblioteca local; a
  próxima atualização da lista corrige.
