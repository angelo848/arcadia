# 0037 — Vídeo do ScreenScraper como mídia `video` na fila

- **Status:** aceito (aprovado com a fase 13, [phase-13-modo-console](../specs/phase-13-modo-console/requirements.md))
- **Data:** 2026-11-15

## Contexto
O Big Box novo mostra o vídeo do jogo em foco. Entre as fontes pesquisadas, o ScreenScraper tem vídeos curtos por jogo
(a versão "normalizada" ficou entre 0,6 e 4,8 MB em 47 jogos de 11 plataformas). IGDB só dá ids do YouTube e as demais
fontes são pagas ou sem API aberta.

## Decisão
- O vídeo é um tipo de mídia novo, `video`, na mesma tabela e na mesma fila de downloads (MID-005): migração 9 só
  amplia o `CHECK` do tipo. O limite é **1 por jogo** ([ADR 0030](0030-limites-de-imagem-por-tipo-como-tabela-do-dominio.md),
  [0034](0034-limite-de-imagens-conta-atuais-e-enfileiradas.md)).
- Baixa-se só o `video-normalized`, em MP4, com **teto de 20 MB por vídeo** (acima disso não baixa) e sem limite total.
- O jogo é identificado no ScreenScraper por **nome + sistema**, com a pontuação da fase 12; só uma correspondência
  "match" baixa vídeo. Não se abre a ROM (sem hash).
- A busca tem uma raia própria de 1 trabalhador, separada das imagens, respeita o `scrapers.Limiter` e pausa por cota ou
  credencial inválida sem gastar tentativa. As credenciais (desenvolvedor do Arcadia e conta do usuário) entram pela
  janela de provedores e nunca vão para o repositório, o banco nem o log.
- O vídeo é buscado sozinho depois de aplicar metadados e por pedido explícito na janela normal, nunca pelo Big Box.
- É servido ao WebView pelo mesmo caminho das imagens, com `video/mp4` e `Range`.

## Alternativas consideradas
- **IGDB (YouTube):** sem arquivo local; depende de rede e de player externo.
- **Vídeo do LaunchBox local:** arquivos de 13 a 96 MB, grandes demais.
- **Hash da ROM no ScreenScraper:** exigiria abrir a ROM (contra IMV-080).

## Consequências
- ✅ Um vídeo curto por jogo, em cerca de 2 MB (mil jogos ≈ 2 GB), com a infraestrutura de fila existente.
- ⚠️ Depende de conta e credenciais de desenvolvedor no ScreenScraper e da conferência da API real.
- ⚠️ O IPC passa a v0.15.0 (só acréscimos).
