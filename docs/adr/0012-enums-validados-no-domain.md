# 0012 — Enums validados no `domain`, sem CHECK no banco

- **Status:** aceito
- **Data:** 2026-11-08
- **Complementa:** [0004](0004-sqlite-modernc-goose-sqlc.md)

## Contexto
O schema tem várias colunas de enum (`game.status`, `game.launch_type`, `media_asset.kind`, `platform.category`,
`pause_interval.reason`, `tracking_rule.kind`, `play_session.source`) que vão ganhar valores novos ao longo das fases.
No SQLite um `CHECK` só muda reconstruindo a tabela; `game` é pai de várias tabelas com `ON DELETE CASCADE`, e a
reconstrução exige `PRAGMA foreign_keys=OFF`, que não tem efeito dentro da transação em que o goose roda cada migração.

## Decisão
Colunas de enum são `TEXT` **sem `CHECK`** no banco; os valores válidos são definidos e validados no `domain` antes de
gravar. `CHECK (x IN (0,1))` em colunas booleanas é permitido (o conjunto não muda). Unicidade de chaves naturais
(`UNIQUE` em `name`) continua no banco.

## Alternativas consideradas
- **`CHECK` no banco** — protege contra escrita fora do Go, mas cada valor novo vira uma migração `NO TRANSACTION`
  que reconstrói `game` e suas filhas.
- **Tabelas de lookup com FK** — flexível, mas adiciona joins e seeds para listas pequenas e controladas pelo código.

## Consequências
- ✅ Adicionar um valor de enum é só mudar o `domain`, sem migração.
- ⚠️ Escrita direta no banco (fora do agente/UI) pode gravar valores inválidos; leitores devem tolerar valor desconhecido.
