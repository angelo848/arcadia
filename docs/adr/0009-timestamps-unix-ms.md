# 0009 — Instantes como INTEGER em milissegundos Unix (UTC)

- **Status:** aceito
- **Data:** 2026-11-08

## Contexto
O schema guarda muitos instantes (`started_at`, `ended_at`, `last_seen_at`, `paused_at`, `first_played_at`...).
O tempo de jogo é calculado somando e subtraindo intervalos, e SQLite não tem tipo de data nativo.

## Decisão
Todo instante (`*_at`) é gravado como **INTEGER com milissegundos desde a época Unix, em UTC**. Datas sem hora
(ex.: `release_date`) ficam como TEXT `YYYY-MM-DD`. A conversão `time.Time` ↔ ms fica num único helper do `store`.
O fuso local só aparece na apresentação (UI).

## Alternativas consideradas
- **TEXT RFC 3339** — legível no banco, mas comparações e somas exigem parse e dependem de formatação idêntica.
- **INTEGER em segundos** — perde precisão útil para pausas curtas e ordenação de eventos próximos.

## Consequências
- ✅ Durações e somas direto em SQL (`ended_at - started_at`), índices compactos, ordenação trivial.
- ✅ Sem ambiguidade de fuso ou horário de verão.
- ⚠️ Inspecionar o banco à mão exige `datetime(x/1000, 'unixepoch')`.
