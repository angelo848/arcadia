# Fase 6 — Importadores e rastreio fora da árvore: design

- **Requisitos:** [requirements.md](requirements.md) (aprovados) · **Tarefas:** [tasks.md](tasks.md)
- **ADRs:** 0005 (camadas), 0015 (agente escreve), 0016 (breakaway), 0019 (lançamento sem rastrear), 0023 (agente elevado),
  e a ADR 0025 proposta sobre o rastreio por processo adotado (RUL-005).
- **Contrato:** `contracts/ipc.openrpc.json` v0.7.0 (sobre a v0.6.0 da fase 5): dois métodos novos
  (`import.scan`, `import.apply`). `game.launch` aceita `launch_type = uri`; nenhum outro método muda.
- **Banco:** sem migração. `tracking_rule` (migração 1, `learned`) e `play_session.source` (texto livre, hoje `tracked`)
  já existem. Novos valores: `source = 'import'` (IMP-007) e `source = 'watched'` (RUL-002).

## 1. Importadores (`internal/importers`, sem Windows exceto onde indicado)

Interface comum:

```go
// Source is one import origin. Scan only reads; it never writes (IMP-001, IMP-009).
type Source interface {
    ID() string                                   // "steam", "epic", "folder", "launchbox"
    Scan(ctx context.Context, in ScanInput) (Result, error)
}

type Candidate struct {
    Source       string   // ID of the Source
    Title        string   // DOM-009 rules apply on apply
    Platform     string   // "" for folder: chosen at apply (IMP-004)
    LaunchType   string   // "uri" | "exe"
    LaunchTarget string   // steam://rungameid/<appid> | com.epicgames...?AppName= | exe path
    InstallDir   string   // "" when unknown
    Developers, Publishers, Genres []string
    Year         *int
    Description  string
    PlayModes    []string // DOM-014 names
    Played       *Played  // LaunchBox only (IMP-007)
}

type Played struct {
    Seconds  int64
    LastPlay time.Time // UTC
}

type Result struct {
    Candidates []Candidate
    Warnings   []string // IMP-002 (no library), IMP-003 (items without DisplayName), IMP-008 (unreadable source)
}
```

- **Steam (IMP-002):** `steamapps\libraryfolders.vdf` (parser de chave/valor da Valve, só as chaves `path`) e
  `steamapps\appmanifest_<appid>.acf` (`appid`, `name`, `installdir`). Lê registro `HKCU\Software\Valve\Steam\SteamPath`
  para achar a pasta (winapi; teste com a pasta de fixture). Fixtures em `testdata/steam/`.
- **Epic (IMP-003):** `*.item` em `ProgramData\Epic\EpicGamesLauncher\Data\Manifests` (JSON). Campos: `AppName`,
  `DisplayName`, `InstallLocation`. Pasta injetada por `ScanInput` para teste.
- **Pasta (IMP-004):** `os.ReadDir` de primeiro nível, `.exe` apenas. Título = nome sem extensão.
- **LaunchBox (IMP-005), formato confirmado no arquivo real (`DataPlatformsWindows.xml`, 76 jogos):** `<LaunchBox>` com um `<Game>` por jogo. Campos usados: `Title`, `Platform` (`Windows`), `Developer`, `Publisher`, `Genre` (separado por `; `), `ReleaseDate` (RFC 3339 com offset; o ano é o da data), `Notes` (descrição), `ApplicationPath` (URI ou `.exe`), `PlayCount`, `PlayTime` (inteiro em **segundos**, confirmado: Counter-Strike 2 tem 33060 com 3 partidas) e `LastPlayedDate` (RFC 3339 com offset). `PlayMode` vem em português: `Único jogador` → `single`; `Cooperativo` → `coop_online`; `Multiplayer` → `multi_online` (ver decisão em §9). Campos ignorados: `Source`, `AdditionalApplication`, imagens, `ID` (GUID). Fixture: `internal/importers/testdata/launchbox/Windows-sample.xml` (3 jogos reais anonimizados: Steam, Epic e `.exe`). `PlayTime` com `PlayCount = 0` é importado como sessão mesmo assim (dois casos no arquivo).

Todas as leituras aceitam o diretório base por `ScanInput` (teste sem tocar no sistema). Nenhum `time.Now()`; o relógio
vem de `domain.Clock` onde houver data.

## 2. Aplicação das importações (`internal/store`, transação por jogo) — IMP-001, IMP-006..IMP-009

`store.ApplyCandidate(ctx, c Candidate, platformID int64, now)` numa transação:

1. Duplicata (IMP-006): mesmo `launch_type` e `launch_target` (sem diferença de maiúsculas em `exe`), ou mesmo título e
   plataforma. Se existir, devolve `ErrDuplicate` e nada grava.
2. Cria o jogo (`game` com `launch_type`, `launch_target`, `install_dir`), vínculos de desenvolvedora, editora, gênero e
   modos (reusa as funções de EDT-004), e reindexa `game_fts` (ADR 0020).
3. Regra de rastreio (RUL-001): `install_dir` se houver; `exe_path` para `launch_type = exe`. `learned = 0`.
4. Tempo (IMP-007, quando `Played != nil` e `Seconds > 0`): uma linha em `play_session` com `source = 'import'`,
   `state = 'ended'`, `started_at = LastPlay - Seconds`, `ended_at = LastPlay`, `active_seconds = Seconds`,
   `last_seen_at = LastPlay`. `total_play_seconds` e `last_played_at` são recalculados pela função que já existe (EDT-005).
5. Plataforma: categoria `pc` para Steam/Epic/pasta, plataforma escolhida pelo usuário para pasta; LaunchBox usa o nome
   da plataforma do XML. Plataforma inexistente é criada com `AddPlatform` (EDT-008, categoria `pc` ou `console` escolhida
   pelo usuário quando for nova; default `pc`).

`ApplyCandidates` (agente) roda cada candidato na sua transação. Erro de um candidato é registrado e os outros seguem
(IMP-008). Resultado: `{applied: [...], skipped: [{title, reason}]}`.

## 3. IPC — contrato v0.7.0

| Método | Params | Resultado | Observação |
|---|---|---|---|
| `import.scan` | `{source, path?}` (`source` ∈ steam, epic, folder, launchbox; `path` obrigatório para folder/launchbox) | `{candidates: [...], warnings: [...]}` | Nada é gravado (IMP-001). Candidatos marcados `exists` quando a duplicata está na biblioteca (IMP-006). |
| `import.apply` | `{candidates: [{source, title, platform?, launchType, launchTarget, ...}]}` | `{applied: [{gameId, title}], skipped: [{title, reason}]}` | Uma transação por candidato (IMP-008). Lista vazia → erro `-32602`. Limite de 500 candidatos. |

- `game.launch` aceita `launch_type = uri`, cujo alvo é o URI. Sem mudança de forma.
- Erros: `-32602` para parâmetros inválidos (fonte desconhecida, caminho ausente, lista vazia). Fontes indisponíveis não
  são erro: viram `warnings`.
- Os campos de candidatos seguem o `camelCase` do contrato. O teste de contrato (IPC-0xx) cobre a forma dos dois métodos.

## 4. Lançamento por URI (RUL-003)

- `winapi.ShellOpen(uri string) error` (Windows): `ShellExecuteExW` com verbo `open`. Sem `SEE_MASK_NOCLOSEPROCESS`:
  o Arcadia não recebe o processo do jogo, só o pedido ao cliente.
- `tracker.Launcher` ganha `OpenURI(uri string) error`. A `LaunchSpec` ganha `Kind` (`exe` ou `uri`); para `uri`,
  `Exe` é o URI e não há `Prepare`.
- A máquina de estados (§4.3) reaproveita `Launching` como espera de adoção: `Begin → Prepared → Inserted → (OpenURI)`
  e fica em `Launching` sem processo. A diferença para o lançamento por exe é o prazo `AdoptTimeout = 60 s` e a ausência de Job. Evento `Adopted(pid, now)` → `Running`; evento `AdoptTimeout` → `Discarded` com `end_reason = 'client_no_launch'`
  (novo valor de texto; `end_reason` é texto, ADR 0012).
- `agent/games.go`: a checagem `g.LaunchType != launchTypeExe` passa a aceitar `uri`, e o despacho usa `tracker.Launch`
  com `Kind = uri`.

## 5. Watcher e processo adotado (RUL-002, RUL-004, RUL-005)

- **Snapshot:** `winapi.ListProcesses() ([]ProcInfo, error)` com `CreateToolhelp32Snapshot` (PID, nome e caminho via
  `QueryFullProcessImageNameW`). Uma goroutine `watcher` do tracker, dona do `context`, com `clock.After(1500 ms)` e
  parada no `Close`.
- **Matching (RUL-002):** para a sessão em `Launching` (espera de adoção), um processo casa se o caminho estiver dentro de `install_dir` ou se
  o caminho for igual a `exe_path` ou o nome de arquivo igual a uma regra `exe_name`. Processos já vistos antes do
  lançamento são ignorados (snapshot base tirado no `OpenURI`).
- **Adoção:** `adoptedJob` implementa a interface `Job` existente: `RootPID()` é o PID casado, `Events()` vem de um
  `WaitForSingleObject` sobre `OpenProcess(SYNCHRONIZE)`, `ActiveProcesses()` conta os PIDs casados ainda vivos,
  `Terminate()` retorna erro (não é nosso), `Resume()` é no-op. Assim a máquina de estados trata o processo igual, e
  pausa por teclado e controle (RUL-005) não muda.
- **Fim (RUL-004):** quando o último casado sai, a sessão entra em carência de 15 s (existente). Se o watcher vê um casado
  de novo nesse prazo, a sessão volta a `Running`.
- **Limitação (RUL-005):** sem Job Object, filhos fora das regras não entram no tempo. A sessão recebe
  `tracking = 'process'` no snapshot (campo novo de `SessionSnapshot`, só para a UI) e a página do jogo mostra
  "rastreio por processo".
- **Sinal da Steam (STM-001):** `winapi.SteamRunning(appid) (running bool, present bool, err error)` lê
  `HKCU\Software\Valve\Steam\Apps\<appid>\Running`. O watcher o consulta no mesmo ciclo: `Running = 1` ajuda a casar o
  processo (aceita casamento por nome de qualquer processo na pasta), e `Running = 0` depois de adotado encurta a
  carência para zero. Sem a chave, o sinal é ignorado.

**Risco a verificar à mão:** o agente é elevado (ADR 0023). O cliente Steam roda sem elevação. Um `steam://` aberto por
processo elevado pode ser recusado pelo cliente ou o jogo pode subir elevado. A T08 registra o comportamento real antes de
qualquer assumção; se o cliente recusar, o caminho exe por pasta (RUL-002) continua valendo.

## 6. Interface (IMP-010, RUL-005)

- `frontend/src/components/ImportDialog.tsx`: escolhe a fonte (Steam, Epic, Pasta, LaunchBox), mostra os candidatos com
  caixas, "já existe" (`exists`), avisos, e "Importar selecionados". Foco preso como o diálogo de remoção (UI-041).
  Pasta e LaunchBox pedem o diretório pelo seletor nativo (`App.PickFolder`, binding novo na UI, sem IPC).
- `lib/imports.ts`: `useImportScan`, `useImportApply` (mutations), chamadas via `window.go` como nas outras.
- `GamePage.tsx`: aviso "rastreio por processo" quando a sessão atual tem `tracking = 'process'`.

## 7. Testes

- `internal/importers`: parsers com fixtures em `testdata/` (VDF, ACF, `.item`, XML). Cada requisito IMP-002..IMP-005 tem
  um caso com o ID no nome.
- `internal/store`: `ApplyCandidate` com banco em `t.TempDir()` (IMP-006, IMP-007, IMP-009, RUL-001).
- `internal/tracker`: fakes para `Launcher.OpenURI` e para o snapshot (RUL-002, RUL-003, RUL-004); `adoptedJob` com
  processo falso. Testes de janela de tempo com `FakeClock`.
- `internal/winapi`: `SteamRunning` e `ListProcesses` com chaves e processos de teste (`HKCU\Software\ArcadiaTest-*`, o
  próprio processo de teste).
- Manual: IMP-005 com o XML real, STM-001 com a Steam instalada, IMP-010, RUL-003 com a Steam (risco acima), RUL-005.

## 8. Riscos

- **LaunchBox:** o formato real pode diferir da suposição. Mitigação: T05 é verificada com o XML do usuário antes de
  fechar, e o parser ignora campos desconhecidos.
- **Steam elevada:** ver §5. Mitigação: caminho de pasta continua funcionando.
- **Watcher com falso positivo:** um processo de outra origem dentro da pasta de instalação (ex.: instalador) pode ser
  adotado. Mitigação: só casa com processo novo depois do `OpenURI`, e a carência encerra o rastreio quando ele sai.
- **Tempo de LaunchBox fora do limite:** `LastPlay - Seconds` pode ser antes do início real. Aceito: é um dado
  importado, não medido; a UI mostra a origem `import` na página do jogo.

## 9. Decisões tomadas com o XML real (2026-11-12)

- **Plataforma:** Steam, Epic, pasta avulsa e a plataforma `Windows` do LaunchBox entram na plataforma `PC` (categoria `pc`),
  para não duplicar plataforma. Outras plataformas do LaunchBox (PS2, Switch, etc.) não são importadas nesta fase: a
  importação do LaunchBox lê só `Windows.xml`.
- **Modos de jogo:** `Único jogador` → `single`; `Cooperativo` → `coop_online`; `Multiplayer` → `multi_online`. O XML não
  diz se é local ou online; a escolha é a mais comum. Se o usuário quiser outra, a edição do jogo (EDT-004) corrige.
- **Tempo:** `PlayTime` em segundos. `PlayCount` zero com `PlayTime > 0` gera a sessão mesmo assim.
- **Lançamentos:** LaunchBox traz URIs `steam://`, `com.epicgames.launcher://` (mesmo formato de IMP-002/IMP-003), `xbox://`,
  `uplay://` e `.exe`. Todos viram `launch_type = uri` ou `exe`; os de Steam e Epic deduplicam com a importação direta
  pelo `launch_target`.
- **Fora do escopo desta fase:** `AdditionalApplication` (apps extras do LaunchBox), e imagens.

## 10. Desvios do design na implementação (registrados em 2026-11-12)

- **Saída do processo adotado:** o watcher detecta saída comparando snapshots (Toolhelp a cada 1,5 s), e não com
  `OpenProcess(SYNCHRONIZE)` por processo. Motivo: evita uma goroutine por processo e reaproveita o mesmo snapshot do
  casamento. A precisão é de um período (1,5 s), dentro da carência de 15 s.
- **Tempo antes da adoção:** a sessão conta a partir da adoção (`Adopted`), e não do clique em "Jogar". O tempo que a
  loja leva para abrir o jogo não entra no tempo jogado.
- **Motivo de término:** `client-no-launch` (com hífen, como `agent-shutdown`), em vez de `client_no_launch`.
- **Sinal da Steam:** a transição `Running 1 -> 0` depois da adoção leva a carência a zero (`EffArmGrace` com atraso 0).
  Com `Running = 0` antes de qualquer `1`, nada acontece.
- **Campo `tracking`:** o contrato v0.7.0 ganha `tracking` (obrigatório em `Session`, valores `job` e `process`).
- **Todas as plataformas (2026-11-12, correção):** o importador lê todos os `*.xml` de `Data\Platforms` (IMP-005), com o
  nome da plataforma de cada jogo. Jogos que usam emulador (`Emulator` preenchido, como em todas as plataformas de
  console do LaunchBox) ficam de fora com um aviso por plataforma: o `ApplicationPath` deles é a ROM, e o lançamento
  por emulador é a fase 6b. Plataformas que não são PC viram categoria `console`.
