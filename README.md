# Arcadia

Launcher e organizador de jogos para Windows, inspirado no LaunchBox, com foco em **rastreamento de tempo de jogo
robusto**: o tempo é contado pela árvore de processos do jogo, não pela janela em foco.

Versão atual: **1.0.0**.

## O que ele faz

- **Biblioteca** em três colunas: plataformas e filtros à esquerda, jogos no meio, detalhes do jogo à direita.
- **Tempo de jogo** contado pelo agente, com pausa por teclado (`Ctrl+Alt+P`) ou controle (`Select + Y` por 1 s) e a
  opção de iniciar um jogo sem contar tempo.
- **Importação** de executáveis, pastas de ROMs (com subpastas e discos agrupados) e da biblioteca do LaunchBox, com
  uma janela de revisão antes de gravar.
- **Emuladores** por plataforma ou por jogo.
- **Metadados e imagens** buscados em IGDB, SteamGridDB e Steam, e vídeo do ScreenScraper. As chaves de cada provedor
  são informadas dentro do aplicativo.
- **Modo console** (Big Box): tela cheia navegável pelo controle, aberta com `F11`. `Select + B` por 1 s fecha o jogo
  em execução.
- **Edição manual** dos dados do jogo, das imagens (capa, screenshots, fundo) e do tempo total jogado.
- **Estatísticas** de tempo de jogo por período.

## Como usar

1. Baixe o `arcadia.exe` da [página de releases](https://github.com/angelo848/arcadia/releases).
2. Execute. O Windows pede permissão de administrador: o Arcadia roda elevado para conseguir iniciar e acompanhar
   jogos que também exigem elevação.
3. Um ícone aparece na bandeja. É o agente, que continua contando o tempo mesmo com a janela fechada. "Sair" no menu
   da bandeja fecha tudo.

Requisitos: Windows 10 ou 11, com o WebView2 (já vem no Windows 11).

A biblioteca é **portátil**: banco, imagens e logs ficam na pasta `data` ao lado do executável. Para usar outra pasta,
passe `--data-dir <pasta>` ou defina a variável `ARCADIA_DATA_DIR`.

## Como funciona

Um executável, dois processos:

- `arcadia.exe --agent` fica na bandeja. Lança os jogos, rastreia o tempo, lê os atalhos e é o único que escreve no
  banco.
- `arcadia.exe` é a interface. Pede tudo ao agente por um named pipe local (JSON-RPC 2.0) e nunca influencia o tempo de
  jogo.

Stack: Go 1.27, Wails v2 com React, TypeScript e Vite, SQLite via `modernc.org/sqlite` (sem CGO).

Mais detalhes em [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md), nas decisões registradas em [docs/adr/](docs/adr/README.md)
e no [roadmap](docs/ROADMAP.md).

## Desenvolvimento

Pré-requisitos: Go 1.27, Node.js e a CLI do [Wails v2](https://wails.io) (`go install github.com/wailsapp/wails/v2/cmd/wails@latest`).

```bash
go build ./...
go test ./...
go vet ./...
```

Só o agente, com os dados numa pasta de desenvolvimento:

```bash
go run ./cmd/arcadia --agent --data-dir ./.devdata
```

Interface em modo de desenvolvimento (a partir de `cmd/arcadia`):

```bash
wails dev
```

Gerar o executável em `cmd/arcadia/build/bin/arcadia.exe` (a partir de `cmd/arcadia`):

```bash
wails build -ldflags "-X main.version=1.0.0"
```

Testes da interface (a partir de `frontend`):

```bash
npx vitest run
```

Depois de alterar as queries em `internal/store/queries/`, rode `sqlc generate`.

### Estrutura

```
cmd/arcadia      executável (interface e agente)
cmd/fake-game    jogo falso usado nos testes de rastreamento
internal/        agente, domínio, banco, rastreador, entrada, IPC, provedores, importadores
frontend/        interface em React
scripts/         geração de ícones e conferência das specs
docs/            arquitetura, ADRs, roadmap e specs de cada fase
```

### Processo

O projeto segue Spec-Driven Development ([ADR 0008](docs/adr/0008-spec-driven-development.md)): cada fase tem uma spec
em `docs/specs/<fase>/` com requisitos numerados, e cada teste cita o requisito que cobre. As convenções de código e o
fluxo de trabalho estão em [CLAUDE.md](CLAUDE.md).

Os ícones são desenhos próprios, sem logotipos das fabricantes. Para refazê-los, veja `scripts/gen-icons.mjs` e
`scripts/gen-icon.go`.
