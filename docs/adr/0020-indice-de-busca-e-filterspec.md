# 0020 — Índice de busca e `FilterSpec`

- **Status:** aceito
- **Data:** 2026-11-12

## Contexto
A biblioteca precisa de busca livre por título, descrição, desenvolvedora e editora, e de filtros por status, modo,
plataforma, gênero, empresa, ano e categoria. Spec: [docs/specs/phase-5-library](../specs/phase-5-library/requirements.md).
Gênero e empresa são vínculos normalizados (tabelas próprias), então o filtro precisa referenciar o id do catálogo, não o
texto exibido.

## Decisão
- Tabela virtual `game_fts` (FTS5) com `title`, `description`, `developer` e `publisher`, criada na migração 5
  ([`00005_fts_rebuild.sql`](../../internal/store/migrations/00005_fts_rebuild.sql)) e preenchida para os jogos existentes.
- O índice é mantido pelo agente na mesma transação de cada escrita de jogo, por duas funções em `internal/store/fts.go`:
  `reindexGame` (inserção, edição e troca de vínculos) e `deleteGameIndex` (remoção).
- A consulta livre passa por `ftsQuery`, que escapa cada palavra como `"palavra"*` (prefixo, sem interpretar operadores
  do FTS5 vindos do usuário).
- `FilterSpec` é versionado no IPC e filtra gênero e empresa por **id** do catálogo (`{id, name}`, ver IPC-049).

## Alternativas consideradas
- **Triggers SQL sobre as tabelas de jogo:** os textos de desenvolvedora e editora dependem de vínculos em outras
  tabelas, e um trigger em cada uma duplicaria a regra de indexação que já existe em Go.
- **Filtro por nome de gênero/empresa:** nomes podem repetir ou mudar; o id é a chave estável que o vínculo usa.

## Consequências
- ✅ Uma só regra de indexação, testada em Go; o formato do índice tem teste de contrato (STO-024).
- ✅ Busca com escape simples: entrada do usuário nunca vira sintaxe do FTS5.
- ⚠️ Quem altera um campo indexado sem passar por `reindexGame` deixa o índice desatualizado. Mitigação: toda escrita
  de jogo passa pelo store, que é o único escritor (ADR 0015).
