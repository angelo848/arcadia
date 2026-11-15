# 0036 — Big Box com temas selecionáveis

- **Status:** aceito (aprovado com a fase 13, [phase-13-modo-console](../specs/phase-13-modo-console/requirements.md))
- **Data:** 2026-11-15

## Contexto
O Big Box atual é a biblioteca normal em tela cheia, com estatísticas, emuladores e provedores. O usuário quer um launcher
de console: só navegar por plataformas e jogos, ver imagens e vídeos, detalhes e tempo jogado, tudo pelo controle, e poder
trocar o visual entre temas ao longo do tempo.

## Decisão
- O Big Box passa a mostrar só: plataformas, jogos de cada plataforma, imagens e vídeo, detalhes, tempo total e a
  navegação por controle. Estatísticas, emuladores, plataformas, provedores e importação saem dele.
- O visual é um **tema selecionável**. A escolha é persistida em `uiPrefs`, e cada tema é um componente que segue um
  contrato de tema (mesmos dados e a mesma navegação, só muda a apresentação) com tokens `--arc-bb-*`
  ([ADR 0029](0029-tema-por-variaveis-css.md)).
- Só o **Faixa** (estilo PS5) é implementado nesta fase. **Mosaico** (estilo Switch) e **Roda** (lista em arco com logos,
  estilo "Unified" do LaunchBox) ficam descritos como temas futuros; o Vitrine foi descartado.
- Isto substitui a parte de Big Box do [ADR 0028](0028-biblioteca-em-tres-colunas-sem-pagina-do-jogo.md) (UIX-044).

## Alternativas consideradas
- **Um único visual fixo:** impede evoluir sem refazer a navegação.
- **Temas do LaunchBox importados:** dependem de XAML e do Big Box dele.

## Consequências
- ✅ Trocar ou acrescentar um tema não mexe na navegação nem nos dados.
- ⚠️ O contrato de tema precisa ser mantido; temas futuros devem cumpri-lo.
- ⚠️ Quem usava estatísticas e configurações dentro do Big Box passa a usar a janela normal.
