# Fase 9 — Plataformas: tarefas

- **Status:** rascunho, aguardando aprovação das tarefas.
- **Requisitos:** [requirements.md](requirements.md) · **Design:** [design.md](design.md) · **Contratos:**
  [contracts/seed-platforms.md](contracts/seed-platforms.md), `contracts/ipc.openrpc.json` (v0.11.0, gerado na T01)
- **ADRs:** 0006, 0010, 0012, 0015, 0026 (seed com chave estável), 0027 (nome único sem maiúsculas).

## Resumo

A migração 00008 cadastra as 13 plataformas do seed com `seed_key`, troca "PC" por "Windows", funde as plataformas
repetidas e cria o índice `NOCASE`. A store ganha a resolução por nome (PLT-025), além de renomear, trocar o ícone e
excluir plataformas. O agente ganha `platform.update`, `platform.setIcon` e `platform.delete`, com o ícone do usuário
copiado para `data\media\platforms\`. Importadores e scrapers passam a usar "Windows". A UI ganha o diálogo
"Plataformas", ícones SVG embutidos e o filtro sem plataformas vazias. Nada desta fase toca `internal/tracker/` nem
`cmd/fake-game/`.

**Premissas:**
- Uma tarefa por commit. Em cada tarefa com testes, o **test-writer** vai primeiro (os testes novos ficam vermelhos) e o
  **implementer** depois. Ao fim de cada tarefa, `go build ./...`, `go test ./...` e `npm test` ficam verdes.
- Quando uma tarefa muda uma assinatura ou um comportamento que testes antigos supõem, o test-writer ajusta esses testes
  **na mesma tarefa** (a lista de cada caso está na tarefa). A T04 deixa robustos, antes da migração, os testes que só
  supõem "banco sem plataformas".
- Os handlers de plataforma saem de `gameHandlers` e `emulatorHandlers` e vão para um `platformHandlers` novo
  (`internal/agent/platforms.go`). Todo método que devolve `Platform` precisa da pasta de mídia para montar o
  `iconPath` absoluto (design §8).

## T01 — Contrato IPC v0.11.0 completo
- Requisitos: PLT-014, PLT-016, PLT-018, PLT-019, PLT-011, PLT-012, PLT-013, PLT-020, PLT-021, PLT-024
- **Responsável:** spec-writer (só `docs/specs/`).
- **Arquivos:**
  - criar `docs/specs/phase-9-plataformas/contracts/ipc.openrpc.json`: cópia de
    `docs/specs/phase-8-estatisticas/contracts/ipc.openrpc.json` (v0.10.0) com o delta aplicado. Itens:
    - `info.version` = `0.11.0`, descrição "Supersedes … phase-8-estatisticas … (v0.10.0)";
    - `Platform` com `seedKey`, `iconPath` e `gameCount`;
    - `platform.add` devolve `{platform, created}`;
    - métodos novos `platform.update`, `platform.setIcon` e `platform.delete`;
    - `PlatformResult` compartilhado por `platform.setEmulator`, `platform.update` e `platform.setIcon`;
    - erros `PlatformNotFound` (-32001) e `PlatformInUse` (-32013).
  - apagar `docs/specs/phase-9-plataformas/contracts/ipc-delta.openrpc.json`.
  - `design.md` §0/§8: trocar a menção ao delta pelo arquivo completo.
- As anotações "alterado em" nas specs de origem (GAM-001, IMP-002, IMP-003, EDT-008, IPC-048, IPC-049) já foram
  aplicadas na aprovação dos requisitos (2026-11-13).
- **Testes:** nenhum nesta tarefa. A troca de `contractPath` para este arquivo fica na T10, junto com os tipos Go.
  Assim a suíte continua verde.
- **Dependência:** nenhuma.

## T02 — Conferência manual dos ids do IGDB
- Requisitos: PLT-030
- **Responsável:** sessão principal com o usuário (precisa de uma chave do IGDB; nenhum arquivo de código).
- **O que fazer:** `POST https://api.igdb.com/v4/platforms` com `fields id,name,slug; where id = (6,37,4,18,24,21,130,508,5,7,8,9,19);`.
  Conferir se cada id corresponde à plataforma da tabela de `contracts/seed-platforms.md` §1 (o do Switch 2, 508, não
  está confirmado).
- **Saída:** a tabela final `seed_key → id` registrada em `contracts/seed-platforms.md` (spec-writer). Id não confirmado
  sai da tabela ("sem identificador"). A T09 usa só os valores confirmados.
- **Dependência:** nenhuma; precisa estar pronta antes da T09.

## T03 — Domínio: tabela do seed e busca por chave e por nome canônico
- Requisitos: PLT-001, PLT-017, PLT-025, PLT-029
- **Arquivos:** `internal/domain/seed.go` (implementer), `internal/domain/seed_test.go` (test-writer).
- **Assinaturas:**
  ```go
  type SeedPlatform struct {
      Key      string           // stable seed key, e.g. "n64"
      Name     string           // canonical name, e.g. "Nintendo 64"
      Category PlatformCategory
  }
  var SeedPlatforms = []SeedPlatform{ /* 13 rows, order of contracts/seed-platforms.md */ }
  const SeedKeyWindows = "windows"
  func SeedByKey(key string) (SeedPlatform, bool)
  func SeedByName(name string) (SeedPlatform, bool) // trims, case-insensitive (same rule as EDT-008)
  ```
- **Testes** (`TestSeedPlatforms`):
  - `PLT-001/13 plataformas com chave, nome e categoria do contrato, na ordem`
  - `PLT-001/chaves únicas e categorias válidas (DOM-015)`
  - `PLT-017/SeedByName ignora maiúsculas e espaços das pontas`
  - `PLT-025/SeedByName não funde nomes parecidos (Super Nintendo)`
  - `PLT-029/SeedByKey desconhecida devolve false`
  - `PLT-001/SeedKeyWindows existe com categoria pc`
- **Dependência:** nenhuma.

## T04 — Testes existentes robustos a um banco com plataformas do seed (design §11)
- Requisitos: PLT-001, PLT-014
- **Responsável:** test-writer (só arquivos de teste; nenhum código de produção muda).
- **Objetivo:** os testes abaixo passam a valer **antes e depois** da 00008. Contagens exatas viram contagens relativas
  a uma linha de base lida no início do teste, e listas exatas viram filtros pelos nomes criados no teste. O
  comportamento que de fato muda ("PC" → "Windows", assinatura de `AddPlatform`, `Platform` com campos novos) é ajustado
  nas tarefas T05, T06, T08, T09, T10, T11 e T12, cada uma no seu momento.
- **Arquivos a ajustar:**
  - `internal/store/store_test.go`: `ListPlatforms` com `len == 1` (≈ linhas 487–492, 795–800, 834–839) e
    `platformCount == writers*iterations` (≈ 680–685) passam a ser relativos à linha de base.
  - `internal/store/catalog_test.go`: lista exata de `Catalog().Platforms` (≈ linha 47) e ordem exata
    "Amiga, atari, PC, switch" (≈ 186–203) passam a filtrar pelos nomes do teste.
  - `internal/store/games_test.go`: pré-condição "nenhuma plataforma" (≈ linha 138) passa a ser a linha de base. Os
    asserts "PC" ficam para a T06.
  - `internal/agent/library_test.go`: lista exata de `platform.list` ("amiga, Atari 2600, PC, Switch", ≈ 1427–1452)
    passa a filtrar pelos nomes do teste.
  - **Conferir** (provavelmente já robustos, porque criam "PC" por SQL direto ou só leem os próprios jogos):
    `library_fixture_test.go`, `library_filter_test.go`, `library_detail_test.go`, `game_delete_test.go`,
    `timeconv_test.go`, `import_apply_test.go` (store) e `internal/agent/stats_test.go`.
- **Testes:** nenhum caso novo; os nomes dos casos existentes não mudam.
- **Dependência:** nenhuma; deve vir antes da T05.

## T05 — Migração 00008, queries sqlc e leitura de `seed_key`, `icon` e contagem
- Requisitos: PLT-001, PLT-002, PLT-003, PLT-004, PLT-005, PLT-006, PLT-007, PLT-008, PLT-014
- **Arquivos (implementer):**
  - `internal/store/migrations/00008_platforms.sql`: passos 1–8 do contrato §3, numa só transação do goose (sem
    `NO TRANSACTION`). O comentário diz que o Down não desfaz dados.
  - `internal/store/store.go`: `SchemaVersion = 8` e o comentário.
  - `internal/store/queries/library.sql`: `ListPlatformsWithCount`, `GetPlatformWithCount`, `GetPlatformBySeedKey`,
    `InsertSeedPlatform`, `UpdatePlatform`, `SetPlatformIcon`, `SetPlatformSeedKey`, `CountPlatformGames`,
    `DeletePlatformRow`, `CountPlatformsWithIcon`. Ainda **não** remover `EnsurePCPlatform`/`GetPlatformIDByName`
    (saem na T06).
  - `sqlc generate` → `internal/store/dbgen/*` (gerado).
  - `internal/store/catalog.go`: `Platform` ganha `SeedKey string`, `Icon string` e `GameCount int64`. `Platforms` e
    `Catalog` usam `ListPlatformsWithCount` (ordem `name COLLATE NOCASE, id`, mantida).
- **Arquivos (test-writer):** `internal/store/migrate8_test.go` (novo; helper `versionSevenFile` com goose `UpTo(7)`,
  como `migrate5_test.go`). Ajustes:
  - `internal/store/store_test.go`: colunas de `platform` ganham `seed_key`;
  - `internal/store/media_test.go` (≈ linha 44) e `internal/store/migrate5_test.go` (≈ linha 227): `SchemaVersion`
    passa a 8.
- **Testes** (`TestMigration8`):
  - `PLT-001/banco vazio fica com as 13 plataformas iguais a domain.SeedPlatforms`
  - `PLT-001/seed sem ícone e sem emulador padrão`
  - `PLT-002/PC vira Windows mantendo id, jogos, sessões, totais e emulador`
  - `PLT-003/PC e Windows: jogos vão para Windows, PC some, total de jogos igual, Windows mantém o emulador`
  - `PLT-003/Windows sem emulador herda o de PC`
  - `PLT-004/Sony PlayStation 2 adotada: id, jogos e emulador mantidos, nome e categoria do seed, sem duplicata`
  - `PLT-005/Sega Genesis e SEGA GENESIS: fica o menor id com os jogos de todas`
  - `PLT-005/menor id sem emulador herda o da primeira outra por id`
  - `PLT-005/updated_at dos jogos movidos não muda`
  - `PLT-006/InsertPlatform do dbgen com o mesmo nome em outra grafia é recusado`
  - `PLT-006/segundo seed_key igual é recusado pelo índice parcial`
  - `PLT-007/plataforma do seed excluída não volta ao reabrir o banco`
  - `PLT-008/00008 roda em transação (arquivo sem NO TRANSACTION)`
  - `PLT-014/GameCount conta jogos ocultos e ordem por nome sem maiúsculas` (store)
- **Dependências:** T03 (o teste compara com `domain.SeedPlatforms`), T04.

## T06 — Store: resolução, criação com chave, renomear, ícone, exclusão, `AddGame` em `windows`
- Requisitos: PLT-014, PLT-016, PLT-017, PLT-018, PLT-019, PLT-020, PLT-021, PLT-025, PLT-026, PLT-027
- **Arquivos (implementer):**
  - `internal/store/catalog.go`: resolução, criação, update, ícone, exclusão e `PlatformByID`.
  - `internal/store/import_apply.go`: `EnsurePlatformID` passa a usar `AddPlatform` e `PlatformIDByName` passa a usar
    `ResolvePlatform`; as assinaturas não mudam.
  - `internal/store/games.go`: `AddGame` resolve pela chave `windows` (design §4); sai `pcPlatformName`.
  - `internal/store/queries/games.sql`: saem `EnsurePCPlatform` e `GetPlatformIDByName`; rodar `sqlc generate`.
  - `internal/agent/library.go`: adaptação mínima de `handlePlatformAdd` ao novo retorno (ignora `created` até a T11).
- **Assinaturas:**
  ```go
  var ErrPlatformNameTaken = errors.New("store: platform name already used")
  var ErrPlatformInUse     = errors.New("store: platform has games")
  type PlatformInUseError struct{ PlatformID, GameCount int64 }
  func (e *PlatformInUseError) Error() string
  func (e *PlatformInUseError) Unwrap() error // ErrPlatformInUse

  func (s *Store) PlatformByID(ctx context.Context, id int64) (Platform, error) // ErrPlatformNotFound
  func (s *Store) ResolvePlatform(ctx context.Context, name string) (Platform, bool, error)
  func (s *Store) AddPlatform(ctx context.Context, name string, category domain.PlatformCategory) (Platform, bool, error)
  func (s *Store) UpdatePlatform(ctx context.Context, id int64, name string, category domain.PlatformCategory) (Platform, error)
  func (s *Store) SetPlatformIcon(ctx context.Context, id int64, icon string) (p Platform, previous string, err error)
  func (s *Store) PlatformIconInUse(ctx context.Context, icon string) (bool, error)
  func (s *Store) DeletePlatform(ctx context.Context, id int64) (icon string, err error)
  ```
- **Arquivos (test-writer):** `internal/store/platforms_test.go` (novo). Ajustes pela nova assinatura de `AddPlatform` e
  por "PC" → "Windows":
  - `internal/store/catalog_test.go` (≈ linhas 123, 141, 146, 190);
  - `internal/store/games_test.go` (casos GAM-001 com "PC" → plataforma de chave `windows`, sem plataforma nova);
  - `internal/store/library_detail_test.go` (≈ linhas 145–148);
  - `internal/store/stats_test.go` (≈ linha 64, "PC" → "Windows");
  - `internal/agent/library_test.go` (wrapper ≈ linha 163; chamadas ≈ 799, 1430, 1464, 1504);
  - `internal/agent/emulators_test.go` (≈ linha 23) e `internal/agent/emulator_launch_test.go` (≈ linha 25).
- **Testes** (`TestPlatforms`):
  - `PLT-025/mesmo nome em outra grafia acha a plataforma`
  - `PLT-025/nome canônico acha a plataforma do seed renomeada`
  - `PLT-025/nome canônico com chave sem plataforma não cria (ResolvePlatform)`
  - `PLT-025/nome inexistente devolve false`
  - `PLT-016/AddPlatform de nome existente devolve created=false e ignora a categoria`
  - `PLT-017/AddPlatform com nome canônico de chave livre grava seed_key e categoria do seed`
  - `PLT-018/renomear mantém jogos, seed_key, ícone e emulador`
  - `PLT-018/mudar só as maiúsculas do próprio nome é aceito`
  - `PLT-018/nome de outra plataforma devolve ErrPlatformNameTaken e nada muda`
  - `PLT-018/renomear para nome canônico de chave livre não atribui a chave`
  - `PLT-019/UpdatePlatform grava a categoria`
  - `PLT-013/SetPlatformIcon devolve o ícone anterior; PlatformIconInUse com uma e com duas plataformas`
  - `PLT-020/DeletePlatform sem jogos apaga e devolve o ícone; emuladores ficam`
  - `PLT-021/DeletePlatform com jogo oculto devolve PlatformInUseError{GameCount: N} e nada é apagado`
  - `PLT-027/AddGame vai para a plataforma de chave windows renomeada`
  - `PLT-027/AddGame recria Windows (pc, windows) quando ela foi excluída`
  - `PLT-027/AddGame grava a chave numa Windows sem chave inserida à mão`
  - `PLT-026/EnsurePlatformID("Windows") devolve a plataforma de chave windows renomeada`
  - `PLT-014/PlatformByID traz GameCount`
- **Dependência:** T05.

## T07 — Mídia: cópia e remoção do ícone da plataforma
- Requisitos: PLT-011, PLT-012, PLT-013
- **Arquivos:** `internal/media/platform_icon.go` (implementer), `internal/media/platform_icon_test.go` (test-writer).
- **Assinaturas:**
  ```go
  const MaxPlatformIconBytes = 2 << 20
  var ErrTooLarge = errors.New("media: imagem grande demais") // new: the design calls it "existing", but it is not
  func (s *Store) ImportPlatformIcon(ctx context.Context, src string) (string, error) // "platforms\<sha256>.<png|jpg>"
  func (s *Store) RemovePlatformIcon(rel string) error // refuses anything outside <root>\platforms\
  ```
  A leitura usa `io.LimitReader(MaxPlatformIconBytes+1)` e o tipo é detectado pelo conteúdo (`extOf`). A escrita é
  atômica (`writeAtomic`), sem miniatura. A pasta `platforms\` só é criada depois que o conteúdo foi aceito.
- **Testes** (`TestPlatformIcon`):
  - `PLT-011/PNG copiado para platforms\<sha256>.png e caminho relativo devolvido`
  - `PLT-011/JPEG vira .jpg`
  - `PLT-011/apagar o original não afeta a cópia`
  - `PLT-012/acima de 2 MB é ErrTooLarge e nada é criado`
  - `PLT-012/texto com extensão .png é ErrUnsupported e nada é criado`
  - `PLT-013/RemovePlatformIcon apaga dentro de platforms\`
  - `PLT-013/RemovePlatformIcon recusa ..\ e caminho absoluto fora de platforms\`
- **Dependência:** nenhuma (pode correr em paralelo com T05/T06).

## T08 — Importadores: "Windows" no lugar de "PC"
- Requisitos: PLT-026, PLT-028, PLT-029
- **Arquivos (implementer):**
  - `internal/importers/types.go`: `PlatformPC` é renomeada para `PlatformWindows = "Windows"`;
  - `internal/importers/steam.go` e `internal/importers/epic.go`;
  - `internal/importers/launchbox.go`: `platformOf` deixa de traduzir `Windows`;
  - `internal/agent/imports.go`: `toStoreGame` usa `PlatformWindows`. `platformCategoryFor` usa `domain.SeedByName`
    primeiro e depois a regra atual (LaunchBox → `console`, resto → `pc`).
- **Assinatura:** `const PlatformWindows = "Windows"` (sai `PlatformPC`).
- **Arquivos (test-writer):** ajustes em `internal/importers/steam_test.go` (≈ linha 70), `epic_test.go` (≈ 28),
  `launchbox_test.go` (≈ 59, 163), `internal/agent/platform_category_test.go` e `internal/agent/imports_test.go`
  (≈ linha 103). Casos novos em `internal/importers/launchbox_images_test.go`, `internal/agent/imports_test.go` e
  `internal/agent/import_images_test.go`.
- **Testes:**
  - `PLT-026/Steam e Epic geram candidato com plataforma Windows`
  - `PLT-026/apply de Steam vai para a plataforma de chave windows renomeada` (store real em `t.TempDir()`)
  - `PLT-028/LaunchBox mantém a plataforma Windows do XML`
  - `PLT-028/copyLaunchBoxImages lê Images\Windows\... para um jogo Windows` (fixture)
  - `PLT-029/platformCategoryFor: nome do seed usa a categoria do seed; LaunchBox fora do seed é console; outras fontes pc`
  - `PLT-029/apply do LaunchBox cria Nintendo 3DS excluída como handheld com a chave`
- **Dependência:** T06.

## T09 — Scrapers: identificador nos provedores e "Windows"
- Requisitos: PLT-030, PLT-031
- **Arquivos (implementer):**
  - `internal/scrapers/platforms.go` (novo): `map[seedKey]map[provider]string`, só com os ids confirmados na T02;
  - `internal/scrapers/igdb.go` e `internal/scrapers/steamstore.go`: `Platform: "Windows"`.
- **Assinatura:** `func PlatformID(seedKey, provider string) (string, bool)`.
- **Arquivos (test-writer):** `internal/scrapers/platforms_test.go` (novo); ajustes em
  `internal/scrapers/scrapers_test.go` (≈ linhas 68, 195) e `internal/domain/match_test.go`.
- **Testes:**
  - `PLT-030/cada chave confirmada devolve o id do IGDB da tabela`
  - `PLT-030/chave desconhecida, chave vazia ou provedor sem valor devolvem false`
  - `PLT-030/toda chave do mapa existe em domain.SeedPlatforms`
  - `PLT-031/IGDB com plataforma PC (Microsoft Windows) devolve Windows`
  - `PLT-031/Steam Store devolve Windows`
  - `PLT-031/MatchScore dá o bônus de plataforma para Windows × Windows`
- **Dependências:** T02, T03.

## T10 — Tipos IPC v0.11.0 e teste de contrato
- Requisitos: PLT-014, PLT-016, PLT-018, PLT-019, PLT-011, PLT-012, PLT-013, PLT-020, PLT-021
- **Arquivos (implementer):**
  - `internal/ipc/library.go`: `Platform` ganha `SeedKey *string \`json:"seedKey"\``,
    `IconPath *string \`json:"iconPath"\`` e `GameCount int64 \`json:"gameCount"\``. `PlatformAddResult` ganha
    `Created bool \`json:"created"\``.
  - `internal/ipc/platform.go` (novo).
  - `internal/ipc/protocol.go`: comentário com o caminho do contrato e `CodePlatformInUse`.
- **Assinaturas:**
  ```go
  const (
      MethodPlatformUpdate  = "platform.update"
      MethodPlatformSetIcon = "platform.setIcon"
      MethodPlatformDelete  = "platform.delete"
  )
  const CodePlatformInUse = -32013
  const MsgPlatformInUse  = "Platform has games"
  const (
      IconReasonNotAbsolute = "not-absolute"
      IconReasonNotFound    = "not-found"
      IconReasonNotAFile    = "not-a-file"
      IconReasonTooLarge    = "too-large"
      IconReasonUnsupported = "unsupported"
  )
  type PlatformUpdateParams  struct{ ID int64 `json:"id"`; Name string `json:"name"`; Category string `json:"category"` }
  type PlatformSetIconParams struct{ PlatformID int64 `json:"platformId"`; Path Optional[string] `json:"path"` }
  type PlatformDeleteParams  struct{ ID int64 `json:"id"` }
  type PlatformDeletedResult struct{ ID int64 `json:"id"` }
  type PlatformInUseData     struct{ PlatformID int64 `json:"platformId"`; GameCount int64 `json:"gameCount"` }
  type PlatformIconErrorData struct{ Reason string `json:"reason"` }
  ```
- **Arquivos (test-writer):**
  - `internal/ipc/contract_test.go`: `contractPath`/`contractRelPath` → phase-9, versão `0.11.0`, tabelas de métodos
    (≈ linhas 273–282, 568–605, 642) e resultados (≈ 770–771);
  - `internal/ipc/contract_phase9_test.go` (novo);
  - `internal/ipc/library_types_test.go` (≈ linha 83, wire de `Platform`).
- **Testes:**
  - `PLT-014/Platform tem exatamente os campos do schema (seedKey, iconPath, gameCount)`
  - `PLT-016/platform.add result tem platform e created`
  - `PLT-018/platform.update params id, name, category`
  - `PLT-011/platform.setIcon params platformId e path; reasons do contrato iguais às constantes`
  - `PLT-020/platform.delete params e result {id}`
  - `PLT-021/PlatformInUse tem código -32013 e data {platformId, gameCount}`
  - `PLT-014/seedKey e iconPath nulos codificam null`
- **Dependência:** T01.

## T11 — Agente: handlers de plataforma (list, add, catalog, update, setIcon, delete, setEmulator)
- Requisitos: PLT-011, PLT-012, PLT-013, PLT-014, PLT-016, PLT-017, PLT-018, PLT-019, PLT-020, PLT-021, PLT-022, PLT-024
- **Arquivos (implementer):**
  - `internal/agent/platforms.go` (novo).
  - `internal/agent/games.go`: saem `platform.list`, `platform.add` e `library.catalog`; `gameStore` perde
    `Platforms`, `AddPlatform` e `Catalog`.
  - `internal/agent/library.go`: `handlePlatformList`/`handlePlatformAdd`/`handleLibraryCatalog`/`toIPCPlatform` vão
    para `platforms.go`.
  - `internal/agent/emulators.go`: sai `platform.setEmulator`; `emulatorStore` perde `SetPlatformEmulator`.
  - `internal/agent/app.go`: `maps.Copy(handlers, platformHandlers(newPlatformService(st, media.New(mediaRoot, httpClient), mediaRoot, os.Stat, log)))`.
    `mediaRoot` é calculado antes desse registro.
- **Assinaturas (pacote `agent`, usadas pelos testes internos):**
  ```go
  type platformStore interface {
      Platforms(ctx context.Context) ([]store.Platform, error)
      Catalog(ctx context.Context) (store.Catalog, error)
      PlatformByID(ctx context.Context, id int64) (store.Platform, error)
      AddPlatform(ctx context.Context, name string, category domain.PlatformCategory) (store.Platform, bool, error)
      UpdatePlatform(ctx context.Context, id int64, name string, category domain.PlatformCategory) (store.Platform, error)
      SetPlatformIcon(ctx context.Context, id int64, icon string) (store.Platform, string, error)
      PlatformIconInUse(ctx context.Context, icon string) (bool, error)
      DeletePlatform(ctx context.Context, id int64) (string, error)
      SetPlatformEmulator(ctx context.Context, platformID int64, emulatorID *int64) error
  }
  type platformIconFiles interface {
      ImportPlatformIcon(ctx context.Context, src string) (string, error)
      RemovePlatformIcon(rel string) error
  }
  type platformService struct { /* st, icons, mediaRoot, stat, log; iconMu sync.Mutex */ }
  func newPlatformService(st platformStore, icons platformIconFiles, mediaRoot string,
      stat func(string) (fs.FileInfo, error), log *slog.Logger) *platformService
  func platformHandlers(s *platformService) map[string]ipc.Handler
  func toIPCPlatform(p store.Platform, mediaRoot string) ipc.Platform // iconPath = Join(mediaRoot, icon) or nil; seedKey "" → nil
  ```
- **Comportamento** (design §8):
  - `platform.setIcon` com caminho:
    1. valida no agente (`not-absolute`, `not-found`, `not-a-file`, e `too-large` pelo `stat`);
    2. chama `ImportPlatformIcon` (`ErrTooLarge`/`ErrUnsupported` → reason);
    3. chama `SetPlatformIcon`; se falhar, remove o arquivo novo quando não está em uso;
    4. apaga o arquivo anterior se `previous != "" && previous != novo && !PlatformIconInUse(previous)`; uma falha ao
       apagar vai para `slog.Warn`.
  - `platform.setIcon` com `null`: só os passos 3 e 4.
  - `platform.delete`: `*store.PlatformInUseError` → `ipc.NewErrorData(CodePlatformInUse, MsgPlatformInUse, PlatformInUseData{...})`,
    seguido do passo 4.
  - `platform.setEmulator`: depois de gravar, devolve o `Platform` completo (`PlatformByID`).
  - setIcon e delete seguram `iconMu`. Nenhum método notifica.
- **Arquivos (test-writer):** `internal/agent/platforms_test.go` (novo; store real em `t.TempDir()` e pasta de mídia
  temporária). Ajustes nos testes que chamavam estes métodos por `gameHandlers`/`emulatorHandlers`:
  - `internal/agent/library_test.go` (harness passa a juntar `platformHandlers`; ≈ linha 115 "PC" → "Windows");
  - `internal/agent/emulators_test.go` (setEmulator devolve o `Platform` completo);
  - `internal/agent/games_test.go` e `internal/agent/pause_test.go`: só conferir os fakes de `gameStore`.
- **Testes** (`TestPlatformHandlers`):
  - `PLT-014/platform.list e library.catalog trazem seedKey, iconPath absoluto e gameCount`
  - `PLT-016/platform.add de nome existente devolve created false com a plataforma existente`
  - `PLT-017/platform.add com nome canônico de chave livre devolve seedKey e categoria do seed`
  - `PLT-018/platform.update renomeia; nome de outra plataforma é -32602 e nada muda`
  - `PLT-018/platform.update de id inexistente é -32001`
  - `PLT-019/categoria fora da lista é -32602 e nada muda`
  - `PLT-011/setIcon copia para platforms\ e iconPath aponta para a cópia`
  - `PLT-012/setIcon recusa não absoluto, inexistente, pasta, acima de 2 MB e não imagem com data.reason; nada criado, ícone igual`
  - `PLT-013/setIcon null restaura o padrão e apaga o arquivo`
  - `PLT-013/arquivo usado por duas plataformas não é apagado ao restaurar uma`
  - `PLT-013/trocar um ícone por outro apaga o antigo`
  - `PLT-020/delete sem jogos apaga a plataforma e o ícone; emuladores ficam`
  - `PLT-021/delete com jogo oculto é -32013 com data {platformId, gameCount}`
  - `PLT-022/platform.setEmulator devolve o Platform completo com o novo emulatorId`
  - `PLT-024/nenhum método de plataforma envia notificação`
- **Dependências:** T06, T07, T10.

## T12 — Desktop: bindings e views de plataforma
- Requisitos: PLT-011, PLT-013, PLT-016, PLT-018, PLT-019, PLT-020, PLT-021, PLT-023, PLT-024
- **Arquivos (implementer):**
  - `cmd/arcadia/library.go`: `PlatformView` ganha `SeedKey *string \`json:"seedKey"\``, `IconURL string \`json:"iconUrl"\``
    e `GameCount int64 \`json:"gameCount"\``. `platformView` passa a ser `func (a *App) platformView(p ipc.Platform) PlatformView`,
    com `iconUrl` vindo de `mediaURL(a.mediaRoot(), *p.IconPath)`. `AddPlatform` devolve `PlatformAddView`.
  - `cmd/arcadia/platforms.go` (novo).
  - `cmd/arcadia/emulators.go` (`PlatformSetEmulator` usa a view nova).
- **Assinaturas:**
  ```go
  type PlatformAddView struct { Platform PlatformView `json:"platform"`; Created bool `json:"created"` }
  func (a *App) AddPlatform(name, category string) (PlatformAddView, error)
  func (a *App) PlatformUpdate(id int64, name, category string) (PlatformView, error)
  func (a *App) PlatformSetIcon(id int64, path *string) (PlatformView, error) // nil → "path": null (map params, like PlatformSetEmulator)
  func (a *App) PlatformDelete(id int64) error // -32013 → "Esta plataforma tem N jogo(s). Mova-os para outra plataforma ou exclua-os antes."
  func (a *App) PickPlatformIcon() (string, error) // filter "Imagens (*.png;*.jpg;*.jpeg)"; "" = cancelled
  ```
- **Arquivos (test-writer):** `cmd/arcadia/platforms_test.go` (novo); ajustes em `cmd/arcadia/library_test.go`
  (≈ linhas 487–499, JSON de `PlatformView`; ≈ 504 e 545, `AddPlatform`).
- **Testes:**
  - `PLT-011/iconUrl de iconPath dentro de data\media é /media/platforms/...`
  - `PLT-011/iconPath fora de data\media vira iconUrl vazio`
  - `PLT-013/PlatformSetIcon(nil) envia path null`
  - `PLT-016/AddPlatform devolve created`
  - `PLT-018/PlatformUpdate chama platform.update com id, name, category`
  - `PLT-021/PlatformDelete com -32013 devolve a mensagem com N`
  - `PLT-023/PickPlatformIcon abre o seletor com o filtro de imagens`
  - `PLT-024/bindings só chamam o agente (nenhum acesso ao banco)`
- **Dependência:** T11.

## T13 — Frontend: ícones SVG, `PlatformIcon` e camada `lib`
- Requisitos: PLT-009, PLT-010, PLT-011, PLT-016
- **Arquivos (implementer):**
  - `frontend/src/assets/platforms/{windows,3ds,n64,nes,gba,gamecube,switch,switch2,wii,ps1,ps2,ps3,snes,generic}.svg`:
    `viewBox="0 0 64 64"`, placa com cantos arredondados e sigla em `<path>` (sem `<text>`, sem logotipo).
  - `frontend/src/components/PlatformIcon.tsx`.
  - `frontend/src/lib/platforms.ts` (novo).
  - `frontend/src/lib/library.ts`: `PlatformView` ganha `seedKey: string | null`, `iconUrl: string` e
    `gameCount: number`. `addPlatform`/`useAddPlatform` devolvem `{ platform, created }`.
- **Assinaturas (TS):**
  ```ts
  export const seedIconUrls: Record<string, string>   // the 13 seed keys -> bundled SVG URL
  export const genericIconUrl: string
  export function platformIconSrc(p: Pick<PlatformView, 'seedKey' | 'iconUrl'>): string
  export function PlatformIcon(props: { platform: PlatformView; size?: number }): JSX.Element
  // lib/platforms.ts
  export type PlatformAddView = { platform: PlatformView; created: boolean }
  export function updatePlatform(id: number, name: string, category: string): Promise<PlatformView>
  export function setPlatformIcon(id: number, path: string | null): Promise<PlatformView>
  export function deletePlatform(id: number): Promise<void>
  export function pickPlatformIcon(): Promise<string>
  ```
- **Arquivos (test-writer):** `frontend/src/components/PlatformIcon.test.tsx` (novo), `frontend/src/lib/platforms.test.ts`
  (novo); ajustes em `frontend/src/lib/library.test.tsx` (mock de `AddPlatform` e fixtures de `PlatformView`).
- **Testes:**
  - `PLT-009/cada uma das 13 chaves tem ícone embutido`
  - `PLT-009/SVGs com viewBox 0 0 64 64 e sem <text>` (via `import.meta.glob(..., { query: '?raw' })`)
  - `PLT-010/sem seedKey ou com chave desconhecida mostra o genérico`
  - `PLT-011/iconUrl preenchido ganha do ícone embutido`
  - `PLT-016/useAddPlatform devolve platform e created`
- **Dependência:** T12.

## T14 — Frontend: diálogo "Plataformas" e botão no cabeçalho
- Requisitos: PLT-015, PLT-016, PLT-018, PLT-019, PLT-020, PLT-021, PLT-022, PLT-023, PLT-024
- **Arquivos (implementer):**
  - `frontend/src/components/PlatformsDialog.tsx`, no molde do `EmulatorsDialog`:
    - lista com ícone, nome, categoria, número de jogos e seletor de emulador padrão;
    - "Nova plataforma" (nome e categoria, `console` pré-selecionada);
    - "Editar" (nome, categoria, campo de caminho do ícone visível, "Procurar..." e "Restaurar padrão");
    - "Excluir" com dois toques, desativado com explicação quando `gameCount > 0`;
    - erros no `role="alert"`.
  - `frontend/src/App.tsx`: botão "Plataformas" ao lado de "Emuladores".
  - Depois de cada escrita, invalida `libraryKeys.catalog` (e a lista de jogos depois de renomear).
- **Assinatura:** `export function PlatformsDialog({ onClose }: { onClose: () => void }): JSX.Element`.
- **Arquivos (test-writer):** `frontend/src/components/PlatformsDialog.test.tsx` (novo); ajuste em
  `frontend/src/App.test.tsx` (botão novo), se ele contar os botões do cabeçalho.
- **Testes:**
  - `PLT-015/lista ícone, nome, categoria, nº de jogos e emulador em ordem de nome`
  - `PLT-016/nova plataforma com console pré-selecionada aparece na lista`
  - `PLT-016/nome que já existe mostra "Já existe: <nome>."`
  - `PLT-018/editar o nome chama updatePlatform e recarrega o catálogo`
  - `PLT-019/trocar a categoria chama updatePlatform`
  - `PLT-020/excluir pede o segundo toque antes de chamar deletePlatform`
  - `PLT-021/com jogos o botão Excluir fica desativado com a explicação`
  - `PLT-022/escolher o emulador padrão (ou Nenhum) chama setPlatformEmulator e mostra o novo valor`
  - `PLT-023/Procurar... preenche o campo de caminho e o campo continua visível`
  - `PLT-013/Restaurar padrão chama setPlatformIcon com null`
  - `PLT-024/depois de salvar invalida o catálogo (sem depender de notificação)`
- **Dependência:** T13.

## T15 — Painel de filtros sem plataformas vazias
- Requisitos: PLT-032
- **Arquivos:** `frontend/src/components/FilterPanel.tsx` (implementer); `frontend/src/components/FilterPanel.test.tsx`
  (test-writer; as fixtures existentes ganham `gameCount`).
- **Testes:**
  - `PLT-032/plataforma sem jogos não aparece`
  - `PLT-032/plataforma sem jogos já marcada continua até ser desmarcada`
- **Dependência:** T13.

## T16 — Verificação final da fase 9
- Requisitos: PLT-001, PLT-002, PLT-003, PLT-004, PLT-005, PLT-006, PLT-007, PLT-008, PLT-009, PLT-010, PLT-011,
  PLT-012, PLT-013, PLT-014, PLT-015, PLT-016, PLT-017, PLT-018, PLT-019, PLT-020, PLT-021, PLT-022, PLT-023, PLT-024,
  PLT-025, PLT-026, PLT-027, PLT-028, PLT-029, PLT-030, PLT-031, PLT-032
- **Responsável:** sessão principal.
- **Builds e testes:**
  - `go build ./...`, `go vet ./...` (Windows e `GOOS=linux`), `CGO_ENABLED=0 go build ./...`, `go test ./...`;
  - `npm test`, `tsc --noEmit`, `npm run build`; `wails build -ldflags "-X main.version=<v>"`.
- **Rastreio:** `powershell -NoProfile -ExecutionPolicy Bypass -File scripts/spec-trace.ps1 -Spec docs/specs/phase-9-plataformas`
  (antes do test-writer: `-SkipTests`).
- **Revisões e checagens manuais:**
  - revisão de PLT-008 (sem `NO TRANSACTION`);
  - PLT-023: diálogo, seletor nativo filtrado e campo visível;
  - conferência visual dos 13 ícones;
  - biblioteca real (com backup de `data\`) com "PC" e plataformas do LaunchBox migrando para a v8 sem perder jogos nem
    tempo;
  - importação do LaunchBox copiando imagens de `Images\Windows`.
- **Documentação:** `docs/ARCHITECTURE.md` §5 (`platform.seed_key`, índice `NOCASE`).
- **Dependência:** T01..T15.

## Ordem

T01 → T02 (manual, a qualquer momento antes da T09) → T03 → T04 → T05 → T06 → (T07, T08, T09 e T10 em paralelo; T09
depende da T02) → T11 → T12 → T13 → (T14, T15) → T16.

## Riscos de concorrência e recursos

- **Goroutines:** nenhuma nova. Os handlers rodam nas goroutines do `ipc.Server`, como hoje.
- **Locks:** `platformService.iconMu` (um `sync.Mutex` na struct, que vive enquanto o agente vive) serializa
  `platform.setIcon` e `platform.delete`. Assim "conferir se o arquivo está em uso" e "apagar" não se cruzam com outra
  plataforma passando a usar o mesmo hash. A ordem é sempre `iconMu` → transação do SQLite; nenhuma transação segura
  `iconMu`. Sem efeito no shutdown (o servidor para antes de a store fechar, como hoje).
- **Handles do Windows:** nenhum. Os arquivos são abertos por `os.Open`/`os.ReadFile` dentro de
  `ImportPlatformIcon`, com `defer Close`. A escrita usa `writeAtomic` (temporário + rename) da fase 7.
- **TOCTOU no tamanho:** o `stat` do agente dá a resposta rápida; o limite de verdade é o `LimitReader` de
  `ImportPlatformIcon` (o arquivo pode crescer entre os dois).
- **Órfãos:** se o agente cair entre a cópia e a gravação no banco, sobra um arquivo em `platforms\` (aceito no
  design §11).
- **`DeletePlatform`:** a contagem e o `DELETE` ficam na mesma transação; o `RESTRICT` é a segunda barreira (ADR 0015).

## Rastreabilidade

| Requisito | Tarefas |
|---|---|
| PLT-001 | T03, T04, T05, T16 |
| PLT-002 | T05, T16 |
| PLT-003 | T05, T16 |
| PLT-004 | T05, T16 |
| PLT-005 | T05, T16 |
| PLT-006 | T05, T16 |
| PLT-007 | T05, T16 |
| PLT-008 | T05, T16 (revisão) |
| PLT-009 | T13, T16 (visual) |
| PLT-010 | T13 |
| PLT-011 | T01, T07, T10, T11, T12, T13 |
| PLT-012 | T01, T07, T10, T11 |
| PLT-013 | T01, T07, T10, T11, T12, T14 |
| PLT-014 | T01, T04, T05, T06, T10, T11 |
| PLT-015 | T14 |
| PLT-016 | T01, T06, T10, T11, T12, T13, T14 |
| PLT-017 | T03, T06, T11 |
| PLT-018 | T01, T06, T10, T11, T12, T14 |
| PLT-019 | T01, T06, T10, T11, T12, T14 |
| PLT-020 | T01, T06, T10, T11, T12, T14 |
| PLT-021 | T01, T06, T10, T11, T12, T14 |
| PLT-022 | T11, T14 |
| PLT-023 | T12, T14, T16 (manual) |
| PLT-024 | T01, T11, T12, T14 |
| PLT-025 | T03, T06 |
| PLT-026 | T06, T08 |
| PLT-027 | T06 |
| PLT-028 | T08, T16 |
| PLT-029 | T03, T08 |
| PLT-030 | T02, T09 |
| PLT-031 | T09 |
| PLT-032 | T15 |

Todos os 32 requisitos têm tarefa.

## Decisões novas

- As decisões de arquitetura da fase já estão nos ADRs 0026 (seed com chave estável e ids de provedor em código) e 0027
  (nome único sem diferenciar maiúsculas). Nenhum ADR novo é necessário.
- Decisão de plano (não merece ADR, mas fica registrada): os métodos que devolvem `Platform` (`platform.list`,
  `platform.add`, `library.catalog`, `platform.setEmulator` e os três novos) passam para `platformHandlers`, porque
  todos precisam da pasta de mídia para montar o `iconPath`.

## Pontos em aberto (do planner)

1. **PLT-025 × PLT-026.** Se a plataforma de chave `windows` foi renomeada (ex.: "PC") e outra, sem chave, passou a se
   chamar "Windows", a resolução por nome manda Steam/Epic para a outra, contra PLT-026. O plano segue PLT-025; a
   alternativa é Steam/Epic resolverem pela chave, como o `game.add` (PLT-027).
2. **Textos dos motivos de PLT-012 na UI** não estão na spec (só os códigos `reason`).
3. **`platform.setEmulator`** passa a devolver o `Platform` completo (`store.PlatformByID`, fora do design §4).
4. **`media.ErrTooLarge`** não existe hoje; a T07 cria o sentinela.
5. **`COLLATE NOCASE` só iguala maiúsculas ASCII**; `domain.SeedByName` deve seguir a mesma regra.
