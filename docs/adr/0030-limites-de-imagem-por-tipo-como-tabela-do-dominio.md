# 0030 — Limites de imagem por tipo como tabela única do domínio

- **Status:** aceito (aprovado com a fase 10.1, [phase-10-1-ajustes](../specs/phase-10-1-ajustes/requirements.md))
- **Data:** 2026-11-14

## Contexto
O importador do LaunchBox copiava no máximo 5 imagens por tipo, com um número fixo no código e sem contar o que o
jogo já tinha. A fase 12 (metadados unificados) também precisa limitar imagens por tipo, vindas de IGDB e SteamGridDB.
Dois números diferentes em dois lugares acabariam divergindo.

## Decisão
- Os limites por tipo ficam numa tabela única em `internal/domain/media_limits.go` (`MediaKindLimit(kind)`):
  capa 2, screenshot 10, fundo 2, tela de título 2, tela de seleção 2, banner 2.
- A cópia de imagens conta as imagens que o jogo já tem (via `ListMedia`, em memória) antes de aplicar o limite.
  Sem consulta nova ao store e sem mudança de IPC.
- O mapa das pastas de imagem do LaunchBox para os tipos de mídia do Arcadia, com prioridade entre pastas, é fixo
  no importador. Tipos que exigiriam um `kind` novo (Clear Logo, Box 3D, etc.) ficam para a fase 12, junto com a
  migração do `CHECK` de `media.kind`.

## Alternativas consideradas
- **Limite por provedor, dentro de cada importador ou scraper:** os números divergem entre fontes.
- **Limite configurável pelo usuário:** adiado; a tabela já é o ponto único para isso depois.

## Consequências
- ✅ A fase 12 reaproveita a mesma tabela.
- ✅ Reimportar não passa do limite.
- ⚠️ Os limites de tela de título, tela de seleção e banner (2 cada) são uma proposta inicial, ajustável no domínio.
