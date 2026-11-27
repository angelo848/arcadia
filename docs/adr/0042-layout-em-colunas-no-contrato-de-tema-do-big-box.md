# 0042 — Layout em colunas no contrato de tema do Big Box

- **Status:** aceito (aprovado com a fase 17, [phase-17-tema-desktop](../specs/phase-17-tema-desktop/requirements.md)).
  Estende o [ADR 0040](0040-contrato-de-tema-v2.md); não o substitui.
- **Data:** 2026-11-27

## Contexto
O usuário quer navegar por controle uma interface igual à do modo desktop. Dar navegação por controle à janela normal exigiria
foco espacial em todas as telas e modais. Os temas do Big Box (Faixa, Mosaico, Estante) são carrosséis ou grades de tela cheia,
com Detalhes como tela separada. A fase 17 adiciona o tema `desktop`: Plataformas, jogos e painel de detalhes em três colunas.

## Decisão
1. **Zonas no reducer compartilhado:** o foco passa a ter zonas (Plataformas, grade, Painel) em `nav.ts`; os outros temas
   continuam com uma zona só e não mudam de comportamento.
2. **Detalhes empilhados:** o painel fica sempre montado, também em Navegar; X (ou → na borda da grade) leva o foco a ele.
3. **Colunas e fileiras calculadas** pela janela e pela visualização (grade ou lista, herdada da janela normal), com limite de
   cartões no DOM por tema e rolagem por fileiras inteiras pelas bordas.
4. **Ordem e visualização herdadas** da janela normal; o tema não tem busca, filtros, ordenação própria, edição, importação nem
   sessões. O painel só tem "Jogar" e "Jogar sem contar tempo".
5. **Visual próprio, sem importar `components/`:** a varredura de BBX-003 proíbe isso. O tema redesenha a marcação com as mesmas
   constantes de layout e 15 tokens `--arc-bb-*` copiados dos `--arc-*` do tema escuro, com um teste que confere a igualdade.
6. **Entrada:** o controle continua lido pela Gamepad API da UI (`useBigBoxInput`); o agente, o IPC, o banco e o tracker não mudam.

## Alternativas consideradas
- **Navegação por controle na janela normal:** foco espacial em todas as telas e modais; custo e risco altos. Descartada.
- **Reaproveitar os componentes do desktop dentro do tema:** têm botões de escrita, foco do DOM e virtualizador próprios. Descartada.

## Consequências
- ✅ O controle navega uma interface familiar sem tocar no desktop.
- ✅ Os outros temas não mudam.
- ⚠️ Há dois lugares com o mesmo visual (desktop e tema); as cores são mantidas iguais por teste.
- ⚠️ Sem área de segurança: uma TV com overscan pode cortar as bordas.
