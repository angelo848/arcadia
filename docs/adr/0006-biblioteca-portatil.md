# 0006 — Nome `arcadia` e biblioteca portátil

- **Status:** aceito
- **Data:** 2026-11-07

## Contexto
Precisamos de um nome para módulo/executáveis e de um local padrão para `library.db`, mídias e logs. Usuários de
LaunchBox costumam manter a biblioteca (com GBs de mídia) num disco de jogos e movê-la entre PCs.

## Decisão
O projeto se chama **arcadia** (`arcadia.exe`, `arcadia-agent.exe`). Os dados ficam numa pasta **`data/` ao lado do
executável** (portátil). Ordem de resolução: flag `--data-dir` > env `ARCADIA_DATA_DIR` > `<exe>/data`.

## Alternativas consideradas
- **`%APPDATA%\arcadia`** — padrão do Windows, mas enche o disco do sistema com mídia e dificulta mover a biblioteca.
- **Perguntar na primeira execução** — pode vir depois como opção, em cima da mesma ordem de resolução.

## Consequências
- ✅ Copiar a pasta leva tudo junto; backup trivial.
- ⚠️ Instalar em `Program Files` não funciona sem override (sem permissão de escrita) — o instalador futuro deve
  usar outra pasta ou gravar `ARCADIA_DATA_DIR`.
- ⚠️ Em `wails dev` o exe fica em `build/bin`; usar `--data-dir`/env no desenvolvimento.
