# 0033 — Busca unificada de metadados com agregador por grupos, pontuação com travas e confirmação em lote

- **Status:** aceito (aprovado com a fase 12, [phase-12-metadados-unificados](../specs/phase-12-metadados-unificados/requirements.md))
- **Data:** 2026-11-15

## Contexto
Hoje o usuário escolhe o provedor (IGDB, SteamGridDB, Steam Store) e cada busca devolve resultados separados. Na
importação em lote isso não escala: é preciso escolher o melhor match de cada jogo sozinho, sem o usuário abrir um
diálogo por jogo, e deixar claro o que ficou em dúvida.

## Decisão
- O agente consulta em paralelo todos os provedores aplicáveis (com chave e compatíveis com a plataforma), junta os
  resultados que são o mesmo jogo em **grupos**, pontua cada grupo (título normalizado + plataforma + ano) e devolve uma
  lista única. O usuário não escolhe provedor.
- A pontuação tem **travas**: títulos que diferem só por número ou letra solta (`Resident Evil` × `Resident Evil 2`)
  ficam limitados a 0,79 e nunca são "provável". Limiares: provável ≥ 0,85 com vantagem ≥ 0,05; em dúvida ≥ 0,60;
  alternativas ≥ 0,40.
- Na importação em lote, depois da janela de revisão de títulos (fase 11), a UI inicia `metadata.batchStart` com os
  jogos gravados. A busca roda em segundo plano (3 trabalhadores, uma por vez, só em memória), com progresso por
  notificação só com contadores (ADR 0031) e cancelamento.
- Os resultados vão para uma **janela de confirmação** separada (páginas de até 900 KiB, ADR 0032): "em dúvida" vem
  desmarcada, "nenhum" bloqueada; `batchApply` responde na hora e aplica em segundo plano, e as imagens chegam pela fila
  de mídia e por `media.changed`.
- Cada provedor tem intervalo mínimo entre requisições; falha temporária suspende o provedor (30/60/120 s) e a terceira
  seguida o desativa naquele lote.

## Alternativas consideradas
- **Manter a escolha de provedor:** inviável em lote.
- **Aplicar sempre o melhor match sem confirmação:** erra jogos com nomes parecidos e não deixa o usuário corrigir.
- **Persistir o resultado do lote:** exigiria migração; o valor não compensa (perde-se só se o agente reiniciar).
- **ScreenScraper:** adiado (conta de desenvolvedor, limite por conta e modo preciso por hash, que abre a ROM).

## Consequências
- ✅ "Obter dados" não pede provedor; o lote traz capa, metadados e imagens logo após importar.
- ✅ Casos ambíguos aparecem para o usuário em vez de gravar o jogo errado.
- ⚠️ O IPC passa a v0.14.0 (só acréscimo; `scrape.search`, `scrape.apply` e `media.queue` ficam obsoletos).
- ⚠️ Um lote de mil jogos leva alguns minutos de busca por causa dos limites de requisição.
- ⚠️ O resultado do lote só existe em memória.
