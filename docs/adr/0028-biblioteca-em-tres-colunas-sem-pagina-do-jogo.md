# 0028 — Biblioteca em três colunas e fim da página do jogo

- **Status:** aceito (aprovado com a fase 10, [phase-10-nova-interface](../specs/phase-10-nova-interface/requirements.md))
- **Data:** 2026-11-14

## Contexto
A biblioteca tinha filtros no topo, grade de cartões com botões e uma página separada para cada jogo. Ir e voltar
entre as duas telas perdia o contexto (busca, rolagem, seleção) e o cartão carregava informação demais.

## Decisão
- A janela principal passa a ter três colunas: filtros e plataformas à esquerda, grade ao centro e detalhes do jogo em
  foco à direita (UIX-004, UIX-008..036).
- A página do jogo deixa de existir (UIX-048). Editar abre num modal; o resto vive no painel direito.
- O jogo em foco é estado da UI (reducer puro em `lib/selection.ts`), sobrevive a filtros e telas e não é gravado
  entre execuções. Um clique foca, duplo clique ou Enter lança.
- Sem painel em foco, o painel mostra um resumo ("Jogados recentemente") e mantém a largura, para a grade não
  deslocar cartões durante um duplo clique.
- Nenhuma mudança no agente, no tracker ou no IPC (UIX-047); a UI continua sem influenciar o tempo de jogo.

## Alternativas consideradas
- **Manter a página do jogo e só reorganizar a grade:** mantém a perda de contexto.
- **Painel direito recolhido sem foco:** a grade muda de colunas ao focar um jogo.

## Consequências
- ✅ Navegação sem trocar de tela; mais próxima do LaunchBox.
- ⚠️ Vários requisitos de UI das fases 3, 4, 5, 6b, 7, 8 e 9 foram alterados ou substituídos (anotados nas specs).
- ⚠️ O Big Box precisou ser adaptado, pois dependia da página do jogo.
