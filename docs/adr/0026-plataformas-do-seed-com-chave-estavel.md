# 0026 — Plataformas do seed com chave estável

- **Status:** aceito (aprovado com a fase 9, [phase-9-plataformas](../specs/phase-9-plataformas/requirements.md))
- **Data:** 2026-11-13

## Contexto
A biblioteca passa a nascer com 13 plataformas (Windows, Nintendo 64, Sony Playstation 2...). O usuário pode renomear
e excluir qualquer uma. Mesmo assim, o Arcadia precisa saber "esta é a plataforma Windows" (Steam, Epic e `.exe` caem
nela), qual ícone embutido mostrar e qual é o identificador dela em cada provedor de metadados (fase 12).

## Decisão
- A migração 00008 grava o seed **uma vez**. Plataforma do seed excluída não volta ao reiniciar (PLT-007).
- Cada plataforma do seed tem uma chave fixa em `platform.seed_key` (ex.: `windows`, `n64`). A chave é a identidade:
  sobrevive a renomear; o nome é só o que o usuário vê. `domain.SeedPlatforms` é a fonte da verdade e um teste compara
  o SQL da migração com ela.
- Criar uma plataforma com o nome canônico de uma chave livre recupera a chave (PLT-017). Renomear para um nome
  canônico **não** atribui chave.
- O que depende da plataforma em código (ícone embutido, plataforma de Steam/Epic/`.exe`, identificador no IGDB) é
  indexado pela chave, nunca pelo nome. Os identificadores de provedor ficam em código (`internal/scrapers/platforms.go`).

## Alternativas consideradas
- **Casar pelo nome:** quebra quando o usuário renomeia ("PC" × "Windows" já causou perda de imagens do LaunchBox).
- **Seed a cada início do agente:** desfaz a exclusão feita pelo usuário.
- **Identificador de provedor em coluna ou tabela no banco:** corrigir um valor exigiria migração, e o seed ficaria em
  dois lugares. Uma tabela editável pode entrar por cima na fase 12, sem desfazer esta decisão.

## Consequências
- ✅ Renomear uma plataforma do seed não perde ícone, importação nem match de metadados.
- ✅ Corrigir ou acrescentar identificador de provedor é mudança de código, sem migração.
- ⚠️ Plataforma criada pelo usuário não tem chave: fica com o ícone genérico e sem identificador de provedor.
- ⚠️ Acrescentar plataformas ao seed no futuro exige migração nova (e decidir o que fazer com quem excluiu as antigas).
