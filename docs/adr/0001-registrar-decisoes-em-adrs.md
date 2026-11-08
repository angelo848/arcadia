# 0001 — Registrar decisões de arquitetura em ADRs

- **Status:** aceito
- **Data:** 2026-11-07

## Contexto
O projeto é desenvolvido com ajuda de subagentes (planner, implementer, reviewer...) que começam sem memória.
Eles precisam de uma fonte curta e estável do "porquê" das escolhas, e o `ARCHITECTURE.md` muda com frequência.

## Decisão
Registraremos cada decisão relevante em `docs/adr/NNNN-*.md` usando o template curto (contexto, decisão,
alternativas, consequências). ADRs aceitos são imutáveis; mudanças geram um novo ADR que substitui o anterior.

## Alternativas consideradas
- **Só o ARCHITECTURE.md** — perde o histórico e o motivo quando o texto é atualizado.
- **MADR completo** — formato mais longo do que o necessário para um projeto deste tamanho.

## Consequências
- ✅ Agentes e pessoas encontram o motivo de cada decisão em um arquivo pequeno.
- ⚠️ Toda decisão nova de arquitetura exige um ADR (o planner sinaliza quando for o caso).
