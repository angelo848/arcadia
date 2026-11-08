# 0004 — SQLite (modernc, sem CGO) + goose + sqlc

- **Status:** aceito
- **Data:** 2026-11-07

## Contexto
A biblioteca precisa ser um arquivo local portátil, com filtros combinados rápidos e busca textual, acessado
pelo agente e pela UI ao mesmo tempo. Queremos manter o build sem toolchain C.

## Decisão
Usaremos **SQLite** via `modernc.org/sqlite` (Go puro), em modo **WAL** com `busy_timeout=5000` e `foreign_keys=ON`.
Migrações com **goose** embutidas no binário; queries tipadas geradas por **sqlc** (código gerado versionado);
busca com **FTS5**. O agente é dono de `play_session`/`pause_interval`; a UI é dona das tabelas de biblioteca.

## Alternativas consideradas
- **mattn/go-sqlite3** — mais rápido, mas exige CGO/gcc no Windows.
- **ORM (GORM/ent)** — mágica demais para um schema pequeno e estável.
- **Arquivos XML/JSON (como o LaunchBox)** — lentos para filtrar e sujeitos a corrupção em escrita concorrente.

## Consequências
- ✅ `CGO_ENABLED=0` compila tudo; banco único fácil de copiar/backup.
- ⚠️ modernc é mais lento que a versão C (irrelevante na escala de dezenas de milhares de jogos).
- ⚠️ Transações curtas obrigatórias para os dois processos não se bloquearem.
