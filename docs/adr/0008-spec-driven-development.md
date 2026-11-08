# 0008 — Spec-Driven Development (SDD) com rastreabilidade por ID

- **Status:** aceito
- **Data:** 2026-11-07
- **Complementa:** [0007](0007-fluxo-de-subagentes.md)

## Contexto
Os subagentes começam sem memória, e o test-writer precisa testar "contra a spec", não contra o código.
Uma spec em prosa corrida não diz o que exatamente testar e se desatualiza sem que ninguém perceba.

## Decisão
Cada feature/fase tem `docs/specs/<feature>/` com:
- `requirements.md` — requisitos com **ID estável** (`ÁREA-NNN`) e critério **Dado/Quando/Então**;
- `design.md` — como implementar, citando os IDs;
- `contracts/` — contratos formais (IPC em OpenRPC, máquina de estados como tabela de transições);
- `tasks.md` — gerado pelo planner, cada tarefa citando os IDs que cumpre.

Fluxo: **spec-writer** → aprovação dos requisitos → planner (`tasks.md`) → aprovação → test-writer
(cada caso de teste começa pelo ID) → implementer/tracker-specialist → reviewer.
`scripts/spec-trace.ps1` garante requisito ↔ tarefa ↔ teste. **Mudança de comportamento e bug começam na spec.**

## Alternativas consideradas
- **GitHub Spec Kit / Kiro como ferramenta** — o fluxo é o mesmo; dispensamos a dependência e mantemos tudo em Markdown + script.
- **BDD com Gherkin executável (godog)** — mais uma camada de ferramenta; os IDs nos nomes de teste dão a mesma rastreabilidade.

## Consequências
- ✅ Testes derivados de critérios explícitos; escopo extra e requisito esquecido viram achados objetivos.
- ✅ Agentes recebem contexto pequeno e preciso.
- ⚠️ Uma aprovação a mais por feature (requisitos). Rigor completo no tracker/IPC; spec leve para UI.
- ⚠️ IDs nunca são reutilizados; a spec é mantida junto com o código.
