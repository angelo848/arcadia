# 0040 — Contrato de tema v2 do Big Box

- **Status:** aceito (aprovado com a fase 15, [phase-15-big-box-v2](../specs/phase-15-big-box-v2/requirements.md)).
  Estende o [ADR 0036](0036-big-box-com-temas-selecionaveis.md); não o substitui.
- **Data:** 2026-11-22

## Contexto
O contrato de tema do ADR 0036 só previa uma faixa linear com plataformas e jogos. Os temas novos pedidos (Mosaico, estilo Switch, e
Estante, estilo Big Picture) precisam de grade em duas dimensões, de fundo e vídeo diferentes (vídeo na capa ou no fundo), de abas de
coleção e, no caso da Estante, de uma tela de Detalhes própria.

## Decisão
O tema continua só **desenhando** o modelo de exibição e repassando cliques; a navegação, os dados e os pedidos seguem fora dele.
O `BigBoxThemeDef` (`frontend/src/bigbox/themes/types.ts`) declara:
- `axis` (`horizontal`, `vertical` ou `grid`) e `columns`: o reducer compartilhado (`nav.ts`) navega a **grade 2D** de qualquer tema;
  `gridRows(viewport)` e `listCapacity(viewport)` dizem quantos cartões cabem (a janela fica limitada a 30 nós no DOM).
- `anchor`: posição do item em foco dentro da faixa (a Faixa usa 2, o Mosaico 1, a Estante 0).
- `backdrop` (`contain`, `cover` ou `plain`): o **fundo por tema**; `videoSlot` (`background` ou `cover`): o **vídeo** vai para o fundo
  ou para a capa do cartão em foco (um único `<video>` na tela).
- `tabs` (`platforms` ou `collections`): as abas são as plataformas ou as coleções (Todos, Favoritos, Concluídos e depois as
  plataformas) e `sort` (`title` ou `lastPlayed`) é a ordem padrão.
- `timeToBeatInBrowse`: se o tempo para zerar aparece na navegação ou só nos Detalhes.
- `Details` opcional: um tema pode trazer a própria tela de Detalhes; sem ele vale a `BigBoxDetails` comum.
- Tokens **completos por tema** (`--arc-bb-*`, [ADR 0029](0029-tema-por-variaveis-css.md)), sem depender dos de outro tema.

O registro (`themes/registry.ts`) é a única fonte da lista de temas: o seletor das configurações lê dele (nome e "(em breve)" para
indisponíveis). O Faixa foi migrado sem mudar de comportamento. O **Roda** continua descrito e indisponível.

## Alternativas consideradas
- **Um componente de navegação por tema:** duplicaria o reducer, o foco e a pré-busca. Descartada.
- **Tema como pacote externo com CSS e JS soltos:** sem benefício num app de um só dono e perde a checagem de tipos.
- **Grade só na Estante:** o Mosaico já precisa de capacidade por viewport; melhor um reducer único.

## Consequências
- ✅ Acrescentar um tema é escrever componentes, tokens e uma entrada no registro; navegação e dados não mudam.
- ✅ Os temas diferem em fundo, vídeo, abas e ordem sem ramos no código compartilhado.
- ⚠️ O contrato cresce a cada necessidade nova; mudança de campo exige atualizar todos os temas e os testes de registro.
- ⚠️ Temas que mudam de escala o cartão em foco precisam respeitar a área segura e o limite de um vídeo na tela.
