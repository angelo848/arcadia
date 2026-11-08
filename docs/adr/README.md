# ADRs — Architecture Decision Records

Registros **curtos** das decisões de arquitetura: o que decidimos, por quê e o que custa.

## Regras
- Um arquivo por decisão: `NNNN-titulo-em-kebab-case.md`, numeração sequencial de 4 dígitos.
- Copie [`0000-template.md`](0000-template.md). Meta: caber em uma tela (~20–40 linhas).
- Status: `proposto` → `aceito` → (`substituído por NNNN` | `descontinuado`).
- ADR aceito **não é reescrito**. Mudou de ideia? Crie um novo ADR e marque o antigo como `substituído por NNNN`.
- Escreva um ADR quando a decisão for difícil de reverter, afetar mais de um pacote ou for algo que alguém
  perguntaria "por que fizemos assim?" daqui a 6 meses.

## Índice

| Nº | Título | Status |
|---|---|---|
| [0001](0001-registrar-decisoes-em-adrs.md) | Registrar decisões de arquitetura em ADRs | aceito |
| [0002](0002-go-wails-react.md) | Go + Wails v2 + React como stack | aceito |
| [0003](0003-agente-separado-da-ui.md) | Agente residente separado da UI, IPC por named pipe | aceito |
| [0004](0004-sqlite-modernc-goose-sqlc.md) | SQLite (modernc, sem CGO) + goose + sqlc | aceito |
| [0005](0005-rastreamento-em-camadas.md) | Rastreamento de tempo em camadas (Job Object + watcher + sinais de loja) | aceito |
| [0006](0006-biblioteca-portatil.md) | Nome `arcadia` e biblioteca portátil | aceito |
| [0007](0007-fluxo-de-subagentes.md) | Fluxo de subagentes com separação autor/teste | aceito |
| [0008](0008-spec-driven-development.md) | Spec-Driven Development com rastreabilidade por ID | aceito |
| [0009](0009-timestamps-unix-ms.md) | Instantes como INTEGER em milissegundos Unix (UTC) | aceito |
| [0010](0010-agente-dono-das-migracoes.md) | Só o agente migra o schema | aceito |
| [0011](0011-clock-com-temporizadores.md) | `domain.Clock` com temporizadores para toda espera | aceito |
| [0012](0012-enums-validados-no-domain.md) | Enums validados no `domain`, sem CHECK no banco | aceito |
| [0013](0013-log-fecha-por-ultimo.md) | O log fecha por último no shutdown | aceito |
