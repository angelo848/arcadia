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
- Edição manual de sessões/tempo total pelo usuário (com log de ajuste).

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
play_session(id, game_id, started_at, ended_at, last_seen_at, active_seconds, state, source[tracked|manual])
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

As fases 9 a 13 (melhorias pós-POC) estão em [ROADMAP.md](ROADMAP.md).

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
