# Arquitetura — Fase 1 (tecnologias e desenho)

> Projeto: launcher/organizador de jogos para PC (Windows primeiro), inspirado no LaunchBox,
> com foco em **rastreamento de tempo de jogo robusto**.
> Nome provisório no código: `arcadia` (troque quando quiser).

---

## 1. Decisões de tecnologia

| Camada | Escolha | Por quê |
|---|---|---|
| Núcleo / backend | **Go** (1.27) com `golang.org/x/sys/windows` | Job Objects, IOCP, CreateProcess, Toolhelp, Registry já vêm embrulhados no pacote; DLLs extras (user32, xinput) via `windows.NewLazyDLL`. Compila para exe nativo único, sem runtime externo, ~8–15 MB de RAM no agente. |
| Shell desktop | **Wails v2** (WebView2) — migrar para v3 quando estável | O "Tauri do Go": app nativo leve, bindings Go↔TypeScript gerados automaticamente. WebView2 já vem no Windows 11. |
| Interface | **React + TypeScript + Vite** | Grade de capas com virtualização, vídeo HTML5 nativo (mp4/webm), animações e temas por CSS. |
| UI libs | TanStack Virtual (grade de milhares de jogos), TanStack Query (cache/IPC), Zustand (estado), Tailwind (estilo) | Padrões maduros, pouca mágica. |
| Banco | **SQLite** (WAL) via `modernc.org/sqlite` (Go puro, sem CGO) + migrações `goose`; queries tipadas com `sqlc`; busca com **FTS5** | Arquivo único, portátil; sem CGO o build continua trivial; WAL permite UI e agente juntos. |
| Imagens | `golang.org/x/image` + `github.com/chai2010/webp` (ou `disintegration/imaging` p/ resize) → miniaturas **WebP** | Grade fluida mesmo com 10 mil capas. |
| HTTP / scraping | `net/http` + goroutines, rate-limit por provedor (`golang.org/x/time/rate`) | Concorrência simples, respeita limites das APIs. |
| Controle (gamepad) | XInput direto via `xinput1_4.dll` (incl. botão Guide via ordinal 100 `XInputGetStateEx`); DirectInput/HID depois, se necessário | XInput funciona **em segundo plano**, sem foco — essencial para o atalho de pausa durante o jogo. |
| Bandeja (agente) | `fyne.io/systray` (ou `getlantern/systray`) | Ícone e menu de bandeja sem framework de UI. |
| IPC | Named pipe via `github.com/Microsoft/go-winio` + JSON-RPC 2.0 | Pipe local com ACL do usuário, sem porta de rede aberta. |
| Logs | `log/slog` + `lumberjack` (rotação) | Diagnóstico do rastreador de processos. |

**Alternativas consideradas:**
- **Rust + Tauri:** mais enxuto ainda, mas curva de aprendizado alta.
- **Java 25 + GraalVM native-image:** familiar, porém FFM trabalhosa para Win32, native-image exige configuração
  de reflection e não há equivalente maduro ao Wails para UI web.
- **C#/.NET 10:** ótimo no Windows, mas Go entrega o mesmo com binários menores e build mais simples.

---

## 2. Visão geral dos processos

```
┌──────────────────────────┐        Named Pipe (JSON-RPC)        ┌───────────────────────────┐
│  arcadia.exe  (Wails UI) │ ◄─────────────────────────────────► │  arcadia.exe --agent      │
│  - biblioteca / filtros  │    eventos: sessão iniciou, pausou, │  (residente, bandeja)     │
│  - edição de metadata    │    tempo atual, jogo encerrou...    │  - lança jogos            │
│  - scraping de mídia     │                                     │  - rastreia processos     │
│  - pode ser aberto e     │                                     │  - atalhos teclado/controle│
│    fechado à vontade     │                                     │  - grava sessões          │
└────────────┬─────────────┘                                     └─────────────┬─────────────┘
             │                     SQLite (WAL)  library.db                    │
             └──────────────────────────────►  ◄───────────────────────────────┘
                                   media/  (arquivos de imagens e vídeos)
```

**Por que dois processos?** O rastreamento de tempo nunca depende da janela principal. Um único executável roda nos dois papéis ([ADR 0014](adr/0014-executavel-unico-dois-processos.md)): sem flag abre a UI (instância única) e garante o agente vivo; com `--agent` é o agente.
Abrir, fechar, atualizar ou até travar a UI **não afeta o tempo de jogo**. O agente é minúsculo,
sobe sempre como administrador (ADR 0023), não inicia com o Windows (ADR 0024) e fica na bandeja. "Sair" na bandeja encerra o agente e fecha a UI; se o agente cair, a UI o relança. Se a UI for aberta durante um jogo, ela apenas
se conecta ao pipe e mostra o cronômetro ao vivo.

**Escrita no banco:** o agente é o único escritor de `library.db`, incluindo a biblioteca. A UI pede as escritas
por IPC (`game.add`, `game.launch`) e lê a lista por IPC ([ADR 0015](adr/0015-agente-unico-escritor-do-banco.md)).

---

## 3. Estrutura do repositório (um módulo Go + front)

```
arcadia/
├─ go.mod
├─ cmd/
│  ├─ arcadia/       # main único (arcadia.exe): UI Wails + wails.json; `--agent` delega a internal/agent
│  └─ fake-game/     # executável de teste que simula launchers (spawn filho e sai, relança, etc.)
├─ internal/
│  ├─ agent/         # modo agente: bandeja, IPC, store (Run(Options) int)
│  ├─ domain/        # entidades, FilterSpec, cálculo de tempo (sem Win32, 100% testável)
│  ├─ store/         # SQLite: migrações (goose), queries (sqlc), FTS
│  ├─ tracker/       # Job Objects, watcher de processos, heurísticas, máquina de estados da sessão
│  ├─ input/         # hotkeys globais (RegisterHotKey) + polling XInput
│  ├─ ipc/           # JSON-RPC sobre named pipe (tipos compartilhados agente↔UI)
│  ├─ scrapers/      # interface MetadataProvider + IGDB, SteamGridDB, ScreenScraper...
│  ├─ importers/     # Steam, Epic, GOG, pastas de ROM, import do LaunchBox (XML)
│  └─ winapi/        # wrappers finos para o que não está em x/sys/windows (user32, xinput)
├─ frontend/         # React + TS + Vite (consumido pelo Wails)
└─ docs/
```

Código Win32 fica atrás de build tags (`//go:build windows`), mantendo `domain` e `store` portáveis
para uma futura versão Linux/macOS.

---

## 4. Rastreamento de tempo de jogo (o diferencial)

### 4.1 Problema
Muitos jogos usam um executável "lançador" que abre o jogo real e **se fecha** (ex.: `launcher.exe` → `game.exe`),
ou entregam o lançamento para um cliente já aberto (Steam, Epic, EA app, Ubisoft Connect).
Rastrear só o PID inicial — como fazem launchers simples — conta segundos em vez de horas.

### 4.2 Estratégia em camadas

**Camada 1 — Job Object (árvore de processos)**
- O agente cria o processo com `CreateProcessW(CREATE_SUSPENDED)`, associa a um **Job Object** e só então
  retoma a thread (sem janela de corrida).
- O Job é ligado a uma *I/O Completion Port* (`JOBOBJECT_ASSOCIATE_COMPLETION_PORT`), recebendo
  `JOB_OBJECT_MSG_NEW_PROCESS`, `EXIT_PROCESS` e `ACTIVE_PROCESS_ZERO`.
- Todo filho/neto herda o Job automaticamente → **se o exe inicial fecha e o filho continua, o tempo continua**.
- Sessão termina somente quando o Job fica vazio (`ACTIVE_PROCESS_ZERO`) + período de carência.
- Não usamos `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE` — fechar o agente nunca mata o jogo.
- O Job tem `JOB_OBJECT_LIMIT_BREAKAWAY_OK` (sem `SILENT`): um processo que pede breakaway sai do Job e não é
  rastreado ([ADR 0016](adr/0016-job-do-jogo-permite-breakaway.md)).

**Camada 2 — Watcher de processos (jogos fora da árvore)**
Para lançamentos via Steam/Epic/URI, o jogo nasce como filho do cliente, não nosso.
- Snapshot `CreateToolhelp32Snapshot` a cada ~1,5 s (sem admin, custo desprezível), comparando com o anterior.
- Um processo novo é "adotado" pela sessão se casar com uma **regra de rastreamento** do jogo:
  1. nome/caminho de exe configurado pelo usuário ou aprendido anteriormente;
  2. exe dentro da pasta de instalação do jogo;
  3. (heurística de descoberta) primeiro processo novo, nos primeiros N segundos, que cria janela visível
     em primeiro plano/tela cheia — a UI pergunta "Este é o jogo?" e **salva a regra** para a próxima vez.
- Processos adotados são abertos com `OpenProcess(SYNCHRONIZE)` e aguardados com `WaitForSingleObject`
  (uma goroutine por processo — barato em Go).
- Upgrade futuro: ETW (`Microsoft-Windows-Kernel-Process`) para eventos instantâneos quando rodar como admin.

**Camada 3 — Sinais específicos de loja**
- Steam: `HKCU\Software\Valve\Steam\Apps\<appid>\Running = 1` e `RunningAppID` (confirma início/fim com precisão).
- Epic/GOG: regras de pasta de instalação lidas dos manifestos importados.

### 4.3 Máquina de estados da sessão

```
          launch()                primeiro processo do jogo detectado
 Idle ───────────► Launching ───────────────────────────────────► Running ◄──┐
                       │ falha do lançamento                          │ │     │ retomada (atalho, controle, UI,
                       ▼                                              │ ▼     │ ou acordar com usuário presente)
                    Failed                  pausa (atalho, controle, UI, │ Paused┘
                                            ou suspensão do PC)          │
                                     conjunto rastreado vazio            ▼
                                   ┌──────────────────────────── Grace (15 s)
                                   │ novo processo casou → volta Running
                                   ▼
                                 Ended  → grava sessão, atualiza total e "primeira vez jogado"
```

Com só a Camada 1 (fase 3), não existe timeout de Launching: o primeiro processo nasce no Job no próprio
lançamento, e Launching → Failed só acontece quando o lançamento falha. O timeout de 90 s volta com a Camada 2.
Paused existe desde a fase 4: pausar não suspende o jogo, só para de contar o tempo. A tabela completa de transições está em
[contracts/session-state-machine.md](specs/phase-4-pause/contracts/session-state-machine.md).

O período de **Grace** cobre jogos que fecham e relançam (atualizadores, troca de DirectX/Vulkan,
"restart to apply settings").

### 4.4 Pausa
- **Teclado:** `RegisterHotKey` global, numa goroutine com `runtime.LockOSThread()` rodando o loop de mensagens (fixo `Ctrl+Alt+P` na fase 4; configurável depois). Funciona com o jogo em foco.
- **Controle:** thread no agente faz polling XInput a ~60 Hz (custo ~0%); combo fixo `Select (Back) + Y`
  segurado por 1 s, um disparo por pressão (evita acionamento acidental). Suporte a até 4 controles.
- **Feedback:** toast/overlay *topmost* ("⏸ Tempo pausado") + som curto. Em jogos em tela cheia exclusiva o overlay
  pode não aparecer — o som garante feedback.
- **Pausas automáticas opcionais:** suspensão/hibernação do PC (`WM_POWERBROADCAST` — tempo dormindo nunca conta),
  jogo minimizado, inatividade de entrada por X minutos.
- No banco, pausa vira um `pause_interval`; tempo efetivo = `fim − início − Σ pausas`. Modelo da fase 4 em [ADR 0017](adr/0017-modelo-de-pausa.md), entrada e feedback nativos em [ADR 0018](adr/0018-entrada-global-e-feedback-nativo.md). Na fase 4 só a suspensão do PC pausa sozinha; jogo minimizado e inatividade ficam para depois.

### 4.5 Abrir sem contar tempo
Duas situações cobertas:
1. **Abrir o próprio programa (UI)** nunca influencia o tempo — ela é separada do agente (seção 2).
2. **Abrir um jogo/app sem rastrear:** ação "Iniciar sem contar tempo" no menu do jogo, e flag
   `track_time = false` em **apps adicionais** (configurador, mod manager, editor de mapas, trainer).
   O agente lança mas não abre sessão.

### 4.6 Robustez
- *Heartbeat* a cada 30 s grava `last_seen_at` da sessão → se o PC desligar/travar, perde-se no máximo 30 s
  e a sessão é fechada na próxima inicialização do agente.
- Sessões com duração < 30 s são marcadas como `discarded` (configurável), não somam.
- Edição manual de sessões/tempo total pelo usuário (com log de ajuste: a linha de log `Info` "sessão criada/editada/excluída"
  do agente, com valores antigos e novos, SED-029; fase 16, [ADR 0041](adr/0041-sessoes-editaveis-pelo-usuario.md)).

---

## 5. Modelo de dados (resumo)

```
platform(id, name [único sem diferenciar maiúsculas, ADR 0027], category[pc|console|arcade|handheld], icon,
         emulator_id?, seed_key? [chave estável das plataformas do seed, ADR 0026])
game(id, platform_id, title, sort_title, release_date, description, install_dir,
     launch_type[exe|uri|emulator], launch_target, launch_args, working_dir,
     rating, favorite, hidden,
     status[not_played|playing|completed|abandoned|backlog],
     first_played_at,          -- automático (1ª sessão), editável
     completed_at,             -- marcado pelo usuário
     total_play_seconds,       -- cache recalculável a partir das sessões
     last_played_at, play_count,
     hltb_main_hours, hltb_extra_hours, hltb_complete_hours,  -- "tempo para zerar" estimado
     created_at, updated_at)
company(id, name);  game_developer(game_id, company_id);  game_publisher(game_id, company_id)
genre(id, name);    game_genre(game_id, genre_id)
play_mode(id, name) -- single, multi_local, multi_online, coop_local, coop_online
game_play_mode(game_id, play_mode_id)
media_asset(id, game_id, kind, path, thumb_path, source, width, height, region, is_primary)
    kind ∈ box_front, box_back, box_3d, clear_logo, banner, fanart_background,
           screenshot_gameplay, screenshot_title, screenshot_gameover, video_gameplay, video_trailer, icon
additional_app(id, game_id, name, target, args, track_time)
tracking_rule(id, game_id, kind[exe_name|exe_path|install_dir], value, learned)
emulator(id, name, exe_path, args_template)  -- ex.: "-f {rom}"
play_session(id, game_id, started_at, ended_at, last_seen_at, active_seconds, state, source[tracked|import|manual], edited_at)
    -- índice play_session_started_at_idx (started_at); edited_at é a marca de "sessão editada" (migração 11, fase 16)
pause_interval(id, session_id, paused_at, resumed_at, reason[hotkey|gamepad|sleep|idle|minimized])
playlist(id, name, filter_json?)   -- listas manuais ou "inteligentes"
game_fts (FTS5: title, description, developer, publisher)
```

**Filtros:** plataforma, gênero, modo de jogo (single/multi/coop/**todos**), desenvolvedor, editora, ano,
status, favorito, jogado/não jogado, faixa de tempo jogado, texto livre. O `core` monta a query a partir
de um `FilterSpec` serializável — o mesmo objeto alimenta playlists inteligentes.

---

## 6. Mídia e metadata — provedores

Interface comum no pacote `scrapers`:

```go
type MetadataProvider interface {
    ID() string
    Search(ctx context.Context, q GameQuery) ([]Candidate, error)
    Details(ctx context.Context, id ProviderID) (*GameMetadata, error)
    Media(ctx context.Context, id ProviderID) ([]RemoteMedia, error)
}
```

| Provedor | Uso principal | Observação |
|---|---|---|
| **IGDB** (API Twitch) | metadata geral, gêneros, modos de jogo, datas, empresas, screenshots | grátis, requer Client ID/Secret |
| **SteamGridDB** | capas, heróis/backgrounds, logos, ícones | grátis com API key |
| **ScreenScraper** | retro: title screen, gameplay, **vídeos**, box art por região | conta grátis, limites por usuário |
| **Steam Store API** | jogos PC: descrição, trailers (vídeo), screenshots | sem chave |
| **LaunchBox Games DB** | metadata (dump XML público) | verificar termos de uso antes de embutir |
| **HowLongToBeat** | tempo estimado para zerar | API não oficial — opcional e isolada |

- **Matching:** normaliza título (remove ™, ®, edição, região), compara por similaridade + plataforma + ano;
  abaixo de um limiar a UI mostra candidatos para o usuário escolher.
- **Merge por prioridade configurável** por campo (ex.: descrição do IGDB, capa do SteamGridDB, vídeo do ScreenScraper).
- Fila de downloads em background com retomada, deduplicação por hash e geração de miniatura.
- Chaves de API do usuário guardadas com **Windows Credential Manager** (`github.com/zalando/go-keyring`), nunca em texto puro.

---

## 7. Importadores

1. Steam (`libraryfolders.vdf` + `appmanifest_*.acf`)
2. Epic (`C:\ProgramData\Epic\EpicGamesLauncher\Data\Manifests\*.item`)
3. GOG Galaxy (registro / banco local)
4. Pasta de ROMs por plataforma + emulador configurado
5. Executáveis avulsos (arrastar e soltar)
6. **Importar biblioteca do LaunchBox** (`Data\Platforms\*.xml`) incluindo tempo jogado e mídias — migração sem dor.

**Importação v2 (fase 11, [spec](specs/phase-11-importacao-v2/requirements.md), [ADR 0032](adr/0032-leitura-de-pasta-por-extensoes-fechadas-e-resultado-limitado.md)):**
a fonte "Pasta" lê subpastas (até 8 níveis e 20.000 itens) só com extensões de jogo escolhidas pela chave da plataforma;
agrupa discos do mesmo jogo num candidato; propõe títulos normalizados (sem `(USA)`, `[!]`, `_`, artigo final movido para o
começo) sem alterar o nome do arquivo; e o usuário revisa e edita tudo numa janela com caixas de seleção antes de importar
(o `import.apply` grava o título recebido). O `import.scan` devolve até 900 KiB por causa do teto de 1 MiB por mensagem do
IPC (v0.13.0, só acréscimos). A plataforma do arquivo é um select, com "Windows" por padrão.

---

## 8. Estratégia de testes

- `domain` e `store`: `go test` puro, table-driven (cálculo de tempo com pausas, filtros, migrações em SQLite temporário).
- Relógio injetável (interface `Clock`) para testar pausas/carência sem `time.Sleep`.
- `tracker`: testes de integração com `cmd/fake-game` simulando:
  pai que abre filho e sai · relançamento em < 10 s · cadeia de 3 níveis · processo fora da árvore (via regra) ·
  jogo que nunca abre · suspensão do PC (simulada).
- UI: Vitest + Playwright (WebView) para fluxos principais.

---

## 9. Roadmap proposto

| Fase | Entrega |
|---|---|
| **1** | Este documento: tecnologias e arquitetura ✅ |
| **2** | Esqueleto: módulo Go, Wails + React, SQLite com migrações, IPC agente↔UI |
| **3** | **Tracker MVP**: lançar exe, Job Object, sessões, heartbeat, carência; `fake-game` + testes |
| **4** | Pausa por teclado e controle, overlay/som, pausa em suspensão, "iniciar sem contar tempo" |
| **5** | Biblioteca: plataformas, gêneros, modos, filtros, busca, grade virtualizada, página do jogo |
| **6** | Importadores (Steam, Epic, pastas, LaunchBox) + watcher/regras aprendidas para lojas |
| **7** | Scrapers e gerenciador de mídia (capas, screenshots, title screen, vídeo) |
| **8** | Estatísticas (tempo por jogo/semana/plataforma), modo tela cheia navegável por controle ("Big Box") |

As fases 9 a 16 (melhorias pós-POC) estão em [ROADMAP.md](ROADMAP.md).

---

## 10. Pontos em aberto para decidir

1. ~~Stack~~ → decidido: **Go + Wails + React + SQLite** ([ADR 0002](adr/0002-go-wails-react.md)).
2. ~~Nome do projeto~~ → `arcadia` ([ADR 0006](adr/0006-biblioteca-portatil.md)).
3. ~~Local da biblioteca~~ → portátil, `data/` ao lado do exe, com override ([ADR 0006](adr/0006-biblioteca-portatil.md)).
4. ~~Atalhos padrão de pausa~~ → `Ctrl+Alt+P` no teclado e `Select + Y` (1 s) no controle, fixos na fase 4 ([ADR 0018](adr/0018-entrada-global-e-feedback-nativo.md)).
5. ~~Agente iniciando com o Windows~~ → removido; o agente sobe sempre como administrador ([ADR 0023](adr/0023-execucao-como-administrador.md), [ADR 0024](adr/0024-agente-sempre-como-administrador.md)).

---

## 11. Convenções de interface

- **Caminhos de arquivo e de pasta sempre têm seletor do Explorer.** Todo campo que recebe um arquivo ou uma pasta
  (executável do jogo, executável do emulador, pasta do LaunchBox, arquivo avulso na importação) oferece o botão
  "Procurar..." que abre o diálogo nativo (`OpenFileDialog` / `OpenDirectoryDialog`). O campo de texto continua
  visível para colar ou corrigir o caminho, mas não é a única forma de preencher.
- O seletor só preenche o caminho; quem grava é o formulário, pelo caminho normal de salvar (validações e
  permissões do agente valem igual).
- Um campo que não é arquivo nem pasta (por exemplo, uma URI de Steam ou Epic) não tem seletor.
- O diálogo nativo é um binding do desktop, com filtro adequado ao campo (`*.exe` para executáveis, todos os arquivos
  para arquivos avulsos e ROMs). Cancelar devolve vazio e não muda o campo.

### Biblioteca em três colunas (fase 10)

- A janela principal abre **maximizada** (não é tela cheia nem Big Box; restaurada, 1280×800, mínimo 1024×640).
  Layout: filtros e plataformas à esquerda, grade ou lista no centro, detalhes do jogo em foco à direita
  ([ADR 0028](adr/0028-biblioteca-em-tres-colunas-sem-pagina-do-jogo.md)). A página do jogo não existe mais.
- O **jogo em foco** é estado da UI (reducer puro em `frontend/src/lib/selection.ts`, contrato em
  `docs/specs/phase-10-nova-interface/contracts/selecao-do-jogo.md`): um clique foca, duplo clique ou Enter lança.
  Não é gravado entre execuções e sobrevive a filtros e telas.
- **Tema**: toda cor vem de tokens `--arc-*` em `index.css` (`[data-theme="dark"]`, expostos ao Tailwind por
  `@theme inline`); testes varrem os componentes contra cor fixa e conferem o contraste
  ([ADR 0029](adr/0029-tema-por-variaveis-css.md)).
- **Modais** usam o componente `Modal` (tamanhos `large`, `medium`, `compact`; cabeçalho e rodapé fixos, corpo com
  rolagem, foco preso). Com um modal aberto o Big Box fica suspenso.
- A UI continua sem influenciar o tempo de jogo; a fase não alterou agente, tracker nem IPC.

### Ajustes da nova interface e imagens do LaunchBox (fase 10.1)

- **Grade e lista** têm margem interna (`GRID_INSET`) e reservam o espaço da barra de rolagem (`scrollbar-gutter`);
  o contorno de foco e de seleção não é mais cortado. A barra de rolagem é fina e usa tokens `--arc-scrollbar-*`;
  controles clicáveis ganham cursor de mão por regra global em `index.css`.
- **Carrossel** do painel: imagem em destaque e faixa de miniaturas, com as setas agindo a partir da miniatura
  focada. **Cartão sem capa** usa outra imagem do jogo (tela de título, tela de seleção, screenshot, banner,
  fundo), com `useCardImages`; sem mudança de IPC.
- **Limites de imagem por tipo** ficam numa tabela do domínio (`domain.MediaKindLimit`, [ADR 0030](adr/0030-limites-de-imagem-por-tipo-como-tabela-do-dominio.md)):
  capa 1, screenshot 10, fundo 2, tela de título 2, tela de seleção 2, banner 2 (a capa passou de 2 para 1 em 2026-11-15). O importador do LaunchBox
  (`internal/importers/launchbox_images.go`) lê as pastas de imagem pelo mapa fixo da spec, com prioridade e
  nomes `Título-NN` ou `Título.<GUID>-NN`; o agente copia respeitando o limite e contando o que o jogo já tem, e
  registra no log as pastas de imagem não lidas. Tipos que exigiriam um `kind` novo ficam para a fase 12.
- **Importação em lotes** de 25 candidatos por chamada de `import.apply`, com progresso na janela de importação.

### Imagens que chegam com a UI aberta (fase 10.2)

- A fila de mídia do agente avisa a UI quando um download grava uma imagem nova: notificação IPC `media.changed
  {gameId}` (contrato v0.12.0, só acréscimo), repassada pelo supervisor como o evento do Wails `media:changed`
  ([ADR 0031](adr/0031-avisos-de-mudanca-em-segundo-plano-como-notificacoes-de-invalidacao.md)).
- Os avisos são agrupados por jogo numa janela de 500 ms (`domain.Clock`; o primeiro sai na hora, os seguintes viram
  um só no fim da janela). Não há aviso para falha, cancelamento, interrupção nem duplicata, nem para chamadas da
  própria UI (IPC-050). A fila arma um só temporizador e só o rearma quando o prazo muda.
- No frontend, `useMediaEvents` invalida só `['media', gameId]` (cartão e carrossel leem a mesma chave); a
  reconexão também recarrega as listas de mídia já carregadas. O carrossel mantém a imagem em destaque por id
  quando chegam imagens novas.

### Metadados unificados (fase 12)

Spec em [phase-12-metadados-unificados](specs/phase-12-metadados-unificados/requirements.md); decisões nos
[ADR 0033](adr/0033-busca-unificada-de-metadados-com-agregador-e-confirmacao-em-lote.md) e
[0034](adr/0034-limite-de-imagens-conta-atuais-e-enfileiradas.md); IPC v0.14.0 (só acréscimos).

- **Busca unificada:** `metadata.search` consulta em paralelo os provedores aplicáveis à plataforma (IGDB, SteamGridDB,
  Steam Store; `scrapers.Aggregator` com prazo de 20 s), junta o mesmo jogo em grupos (`domain.SameGame`), pontua
  (`domain.ScoreGroup`, em milésimos, com travas de marca e de plataforma) e devolve até 10 opções com confiança
  (`match`, `doubt`, `none`). O usuário não escolhe provedor. `metadata.apply` busca texto e imagens dos provedores
  escolhidos e grava por campo (IGDB primeiro; campo do usuário preservado, salvo "substituir").
- **Limites de requisição:** um `scrapers.Limiter` por provedor, criado uma vez na fábrica (`newProviderFactory`) e
  compartilhado por busca única, lote e aplicação; o IGDB também compartilha o token.
- **Imagens:** todo caminho que enfileira passa por `queueImages`, que conta as imagens atuais mais os jobs ativos
  (`Store.ActiveMediaJobCounts`) e nunca apaga; a escolha é de `scrapers.SelectImages` (SteamGridDB traz capa e
  herói; a API v2 não tem screenshots de gameplay).
- **Lote após a importação:** com a caixa "Buscar metadados e imagens" (fonte Pasta), depois de `import.apply` a UI
  chama `metadata.batchStart` com os `gameId` gravados (o título usado é o revisado, já no banco). O job fica só em
  memória no agente (`metadataService`, um por vez, 3 trabalhadores, saúde por provedor: suspende 30 s, 60 s e
  desativa na 3ª falha seguida), avisa progresso por `metadata.batchChanged` (só contadores, agrupado numa janela de
  500 ms, ADR 0031) e é cancelado e esperado no desligamento antes da fila de mídia e do banco.
- **Confirmação:** o indicador do cabeçalho (`MetadataBatchChip`) mostra o estado; ao chegar em `ready`, a janela
  "Confirmar metadados" abre sozinha se nenhum outro modal estiver aberto (linhas em dúvida vêm desmarcadas;
  `metadata.batchResults` em páginas de até 900 KiB). `metadata.batchApply` responde na hora e aplica em segundo
  plano; as imagens chegam pela fila de mídia e por `media.changed`.
- **Arquivo avulso:** `searchName` (do executável ou da pasta pai) preenche o campo "Nome para busca".
- **Limitação conhecida:** o IPC atende uma requisição por vez por conexão; um `metadata.search` ou
  `metadata.apply` em curso pode atrasar `batchCancel`/`batchStatus` na mesma conexão da UI.
- A UI nunca influencia o tempo de jogo: nada em `internal/tracker`, `play_session` ou `tracking_rule`.

### Modo console (fase 13)

Spec em [phase-13-modo-console](specs/phase-13-modo-console/requirements.md); decisões nos
[ADR 0035](adr/0035-fechar-o-jogo-pelo-controle.md), [0036](adr/0036-big-box-com-temas-selecionaveis.md) e
[0037](adr/0037-video-do-screenscraper-como-midia-na-fila.md); IPC v0.15.0 (só acréscimos); migração 9.

- **Fechar pelo controle (Select + B, 1 s):** `internal/input` tem um segundo detector (`CloseMask`) ao lado do da
  pausa; o `triggerRouter` do agente despacha `ActionPause` e `ActionClose` (antirrepique de 300 ms). O tracker ganha
  o estado `Closing` (`CloseByHotkey`): envia `WM_CLOSE` às janelas dos processos do jogo, espera até 3 s pelo
  `domain.Clock` e, se ainda houver processos vivos, mata a árvore do Job Object (jogo adotado: só os PIDs que ainda
  casam com a regra). A sessão fecha com o tempo até o instante do gatilho e o motivo `closed-by-hotkey`; o tempo
  pausado não conta. Sem processo para fechar, toca o som de recusa. O agente publica `game.closedByHotkey` e a UI
  só usa isso para trazer a janela à frente (`BringToFront`).
- **Jogo sem contar tempo:** nasce num Job Object guardado pelo tracker (`DetachedJob`), apenas para poder ser
  fechado; um jogo por vez (lançar outro durante o job vivo é recusado).
- **Big Box novo:** `frontend/src/bigbox/` (reducer de navegação, modelo de exibição, entrada por Gamepad API, palco
  de fundo com vídeo após 2 s, rodapé de dicas, detalhes) e temas selecionáveis por `themes/registry.ts` com tokens
  `--arc-bb-*`. Só o tema **Faixa** existe; Mosaico e Roda estão reservados. A escolha fica em `uiPrefs`
  (`arcadia.bigBoxTheme`). O Big Box só navega, mostra e lança jogo (`LaunchGame`, `LaunchGameUntracked`).
- **Vídeo (ScreenScraper):** tipo de mídia `video` (limite 1 por jogo, MP4 `video-normalized`, teto de 20 MB). O
  cliente `scrapers.ScreenScraper` identifica o jogo por nome + sistema (sem hash) com a pontuação da fase 12; só
  "match" baixa. A fila de mídia ganhou uma raia própria de 1 trabalhador (`fetchRouter` separa imagem e vídeo),
  que pausa por cota (1 h) ou credencial inválida (até o usuário regravar a chave) sem gastar tentativa. O
  `videoService` expõe `video.queue`, `video.queueMissing`, `video.cancelPending` e `video.status`, e busca o vídeo
  sozinho depois de aplicar metadados quando as duas credenciais existem. As credenciais (desenvolvedor e usuário)
  ficam no Credential Manager, entram pela janela de provedores e nunca vão para log, banco ou URL registrada. O
  `.mp4` é servido ao WebView pelo mesmo servidor de mídia, com `Range`.
- A UI nunca influencia o tempo de jogo; `cmd/fake-game`, `play_session`, `pause_interval`, `tracking_rule` e a
  tabela `game` não mudaram.

### Big Box v2 (fase 15)

Spec em [phase-15-big-box-v2](specs/phase-15-big-box-v2/requirements.md); decisões nos
[ADR 0038](adr/0038-big-box-por-pedido-externo.md), [0039](adr/0039-tempo-para-zerar-hltb.md) e
[0040](adr/0040-contrato-de-tema-v2.md); IPC v0.16.0 (só acréscimos); migração 10. Versão 1.1.0.

- **Terceiro acorde do controle (Select + A, 1 s):** `internal/input` ganha o detector `BigBoxMask` (padrão Back+A) ao lado
  dos de pausa e fechar; o `triggerRouter` despacha também `ActionBigBox` (antirrepique `domain.BigBoxDebounce`, 300 ms). Sem
  sessão aberta (`Tracker.Current()` vazio), o `uiOpener` executa `arcadia.exe --bigbox --data-dir <raiz>` destacado do agente
  (`winapi.CreateDetached`); com jogo rodando, o roteador só toca o som de recusa. O tracker não vê a ação nova.
- **`--bigbox` e segunda instância:** o desktop lê `--bigbox` na primeira instância (`desktopConfig.BigBox`) e, na segunda
  (`OnSecondInstanceLaunch`), por varredura própria dos argumentos (`wantsBigBox`; não usa `flag`). O pedido fica no `App`
  (`bigBoxRequest`) e o frontend o toma uma vez com `TakeBigBoxRequest` (e pelo evento Wails na instância já aberta). Dentro do Big
  Box, o pedido não faz nada (BBX-057: o Select + A não lança o jogo em foco). Isto muda a regra do single-exe de ignorar os
  argumentos da segunda instância.
- **Tempo para zerar:** serviço `timeToBeat` do agente (`internal/agent/timetobeat*.go`), com uma goroutine trabalhadora (dona:
  o serviço; encerra no `Close`, antes de fechar o store). Duas filas (interativa e lote de até 1.000), uma requisição externa por
  vez, `Limiter` de 2 s no HowLongToBeat (`scrapers.HLTB`, token de `/api/search/site/init` só em memória, renovado no 403) e
  recuo por fonte (1, 5, 30 e 60 min). `domain.RankTimeToBeat` pontua com as regras da fase 12; `domain.TTBResolve` decide o que
  gravar. Sem achado no HowLongToBeat, ou com falha dele, o serviço tenta a reserva do IGDB (`(*IGDB).TimeToBeat`,
  `game_time_to_beats`), ignorada em silêncio sem chave. O cache fica na tabela `time_to_beat` (migração 10, uma linha por jogo,
  `ON DELETE CASCADE`), com validade de 90 dias (HowLongToBeat achado), 30 dias (IGDB, em dúvida e não achado) e linha confirmada
  pelo usuário preservada. A UI lê pelos métodos `timeToBeat.get/refresh/search/choose/getSettings/setSettings` e recebe
  `timeToBeat.changed {gameId}` (ADR 0031). `metadata.apply` e o lote enfileiram a busca depois de gravar. Falhas nunca chegam ao
  usuário: aparece "—".
- **Contrato de tema v2:** `BigBoxThemeDef` ganha `axis` (`horizontal`, `vertical` ou `grid`), `columns`, `anchor`, `gridRows`,
  `backdrop` (`contain`, `cover` ou `plain`), `videoSlot` (`background` ou `cover`), `tabs` (`platforms` ou `collections`), `sort`,
  `timeToBeatInBrowse` e `Details` opcional. O reducer de navegação (`nav.ts`) passou a ter grade 2D, a janela de itens fica
  limitada a 30 nós, e o modelo (`viewModel.ts`) cobre as abas de coleção (Todos, Favoritos, Concluídos e plataformas).
- **Temas:** **Faixa** (estilo PS5, igual à fase 13), **Mosaico** (estilo Switch: fileira horizontal, vídeo na capa, painel com tempo
  para zerar) e **Estante** (estilo Big Picture: grade de 5 colunas, destaque no alto que some ao descer, abas de coleção, Detalhes
  próprios). O **Roda** continua indisponível. O registro é `themes/registry.ts`; o seletor lê dele.
- A UI nunca influencia o tempo de jogo; nada mudou em `internal/tracker`, `play_session`, `pause_interval`, `tracking_rule`,
  `media` ou `media_job`.

### Tema Desktop do Big Box (fase 17)

O contrato de tema ganhou o layout `columns` ([ADR 0042](adr/0042-layout-em-colunas-no-contrato-de-tema-do-big-box.md)): o reducer
(`nav.ts`) tem zonas (Plataformas, jogos e painel), o painel de detalhes fica montado também em Navegar, colunas e fileiras vêm da
janela e da visualização herdada da janela normal, e a rolagem é por fileiras inteiras. O tema `desktop`
(`themes/desktop/`) desenha a marcação com as constantes de layout do desktop e tokens `--arc-bb-*` copiados dos `--arc-*` do tema
escuro, e só tem os botões "Jogar" e "Jogar sem contar tempo". Só frontend: agente, IPC, banco e tracker não mudam.

### Edição de sessões (fase 16)

Spec em [phase-16-edicao-sessoes](specs/phase-16-edicao-sessoes/requirements.md); decisão no
[ADR 0041](adr/0041-sessoes-editaveis-pelo-usuario.md); IPC v0.17.0 (só acréscimos); migração 11. Versão 1.2.0.

- **Migração 11:** `play_session.edited_at` (INTEGER, nula = nunca editada) e o índice `play_session_started_at_idx` em
  `(started_at)`, usado pela checagem de sobreposição. A origem (`source`) passa a ser validada no `domain` (`tracked`, `import` ou
  `manual`).
- **Regras puras em `internal/domain/session_edit.go`:** `ValidateSessionTimes` (fim depois do início, nada no futuro, início a
  partir de 1980, intervalo de até 24 h, tempo jogado de 30 s até o intervalo), `Overlaps`, `ClampTotal` e `ClipPause`. As razões
  de recusa (`EditReason`) viajam em `error.data.reason`.
- **Store (`internal/store/session_edit.go`):** `CreateManualSession`, `EditSession` e `RemoveSession`, cada um numa transação
  `BEGIN IMMEDIATE` que também lê a sessão, checa a sobreposição (contra sessões rastreadas e manuais de qualquer jogo, inclusive
  a aberta), corta as pausas, move o total pela diferença (conserva o ajuste do EDM-005) e recalcula primeira e última vez e a
  contagem das sessões contadas. Como o `EndSession` do tracker grava pelo mesmo `Store`, as duas escritas se serializam.
- **Agente:** os handlers `session.create`, `session.update` e `session.delete` (`internal/agent/session_edit.go`) não recebem o
  tracker, não criam goroutine e **não emitem notificação** (a UI invalida o próprio cache). Erros novos: `-32017` (sessão não
  encontrada) e `-32018` (cruza outra sessão); `-32002` para sessão em andamento e `-32602` com `reason` para as demais recusas.
  Cada escrita grava uma linha `Info` com os valores antigos e novos (SED-029).
- **Leituras:** `session.list` traz `source` e `editedAt`; o detalhe do jogo traz `sessionPlaySeconds`; as estatísticas só somam
  sessões contadas (SED-037) e trazem `manualSeconds`.
- **UI:** a lista de sessões do painel de detalhes ganha "Adicionar sessão", editar e excluir por linha (só encerradas), as marcas
  "Manual", "Importada" e "Editada", um formulário modal e uma confirmação de exclusão (irreversível); as estatísticas mostram
  "adicionados à mão".
- A UI nunca influencia o tempo de jogo; nada mudou em `internal/tracker`, `cmd/fake-game`, `tracking_rule`, `media`, `media_job`
  e `time_to_beat`. `pause_interval` só muda pelo corte de pausas da edição.
