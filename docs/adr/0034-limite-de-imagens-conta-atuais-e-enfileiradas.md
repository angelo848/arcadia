# 0034 — Limite de imagens por tipo conta as atuais e as enfileiradas, em todo caminho da fila

- **Status:** aceito (aprovado com a fase 12, [phase-12-metadados-unificados](../specs/phase-12-metadados-unificados/requirements.md))
- **Data:** 2026-11-15

## Contexto
O [ADR 0030](0030-limites-de-imagem-por-tipo-como-tabela-do-dominio.md) fixou os limites por tipo em
`domain.MediaKindLimit`, mas só a importação do LaunchBox os respeita. `scrape.apply` e `media.queue` enfileiram downloads
sem teto, e com a busca unificada em lote isso estouraria o limite por jogo.

## Decisão
- Todo caminho que enfileira imagens passa por um único `queueImages`, que calcula quantas ainda cabem por tipo:
  **imagens atuais do jogo + jobs ativos (pendentes ou em andamento)**, serializado por jogo.
- Nada é apagado: se o jogo já está no limite, as imagens novas simplesmente não são enfileiradas.
- A busca unificada escolhe as melhores imagens da união dos provedores até o espaço restante, pela prioridade de
  `match-rules.md` (capa: SteamGridDB primeiro no Windows, IGDB primeiro nos consoles).

## Alternativas consideradas
- **Contar só as imagens já gravadas:** dois lotes ou duas aplicações seguidas passam do limite antes de os downloads
  terminarem.
- **Apagar as mais antigas ao passar do limite:** destrói curadoria feita pelo usuário.

## Consequências
- ✅ O limite vale em qualquer caminho, inclusive aplicações repetidas e lotes concorrentes.
- ✅ Estende o 0030 sem contradizê-lo.
- ⚠️ Uma leitura nova em `internal/store` (jobs ativos por tipo), sem migração.
- ⚠️ Jogo que já tem o limite não recebe imagens novas até o usuário remover alguma.
