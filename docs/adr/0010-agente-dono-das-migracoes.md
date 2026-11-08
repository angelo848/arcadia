# 0010 — Só o agente migra o schema

- **Status:** aceito
- **Data:** 2026-11-08
- **Complementa:** [0003](0003-agente-separado-da-ui.md), [0004](0004-sqlite-modernc-goose-sqlc.md)

## Contexto
Agente e UI abrem o mesmo `library.db`. O goose não tem lock entre processos no SQLite: se os dois migrarem ao mesmo
tempo (ex.: primeira execução, quando a UI inicia o agente), o segundo falha com "table already exists".

## Decisão
Só o **agente** aplica migrações (`store.Open` com migração habilitada). A UI abre o banco **sem migrar**, apenas depois
de conectar ao agente, e recebe `ErrSchemaMismatch` se a versão do schema não for a esperada pelo binário — tratado
como "agente ainda não pronto" (tenta de novo/mostra desconectado).

## Alternativas consideradas
- **Os dois migram com lock por arquivo/mutex nomeado** — mais código e mais um recurso compartilhado para errar.
- **Migração via RPC ao agente** — acopla a UI a um método extra sem ganho nesta fase.

## Consequências
- ✅ Um único escritor de DDL; sem corrida na primeira execução.
- ⚠️ A UI depende do agente para ter um banco utilizável (coerente com o ADR 0003).
- ⚠️ Agente e UI de versões diferentes detectam o descompasso pelo número da versão do schema.
