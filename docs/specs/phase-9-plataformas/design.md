# Fase 9 — Plataformas: design

- **Requisitos:** [requirements.md](requirements.md) (rascunho, aguardando aprovação) · **Tarefas:** `tasks.md` (do planner)
- **Contratos:** [contracts/seed-platforms.md](contracts/seed-platforms.md) (tabela do seed, ícones, migração 00008,
  resolução por nome) e [contracts/ipc-delta.openrpc.json](contracts/ipc-delta.openrpc.json) (delta do IPC v0.11.0 sobre
  a v0.10.0 da fase 8; a primeira tarefa gera o arquivo completo `contracts/ipc.openrpc.json`).
- **ADRs:** 0006 (arquivos em `data\`), 0010 (agente dono das migrações), 0012 (enums no `domain`, tolerância a valor
  desconhecido), 0015 (agente único escritor). Nenhum é contrariado; ver §11 para o que merece ADR novo.

## 0. Estado atual (conferido no código)

- `platform(id, name UNIQUE, category, icon, emulator_id)` (migração 00001). O `UNIQUE` de `name` é **binário**: só
  `AddPlatform` evita repetição por maiúsculas (busca `GetPlatformByNameNoCase` antes de inserir). `icon` existe mas
  nunca é escrito. `game.platform_id` é `ON DELETE RESTRICT`. `emulator.name` já tem índice `NOCASE` (00006).
- Queries: `EnsurePCPlatform`/`GetPlatformIDByName` (`games.sql`, usadas por `AddGame` com `pcPlatformName = "PC"`),
  `ListPlatformsByName`, `InsertPlatform`, `GetPlatformByNameNoCase` (`library.sql`), `SetPlatformEmulator` (`emulators.sql`).
- IPC: `platform.list`, `platform.add` (EDT-008), `library.catalog` e `platform.setEmulator`; não há renomear, ícone
  nem exclusão. `ipc.Platform{id, name, category, emulatorId}` com `additionalProperties: false` no contrato.
- Importação: `importers.PlatformPC = "PC"` (Steam, Epic; o LaunchBox traduz `Windows` → `PC` em `platformOf`).
  `agent.toStoreGame` usa `PlatformPC` para candidato sem plataforma e `platformCategoryFor` (LaunchBox não-PC →
  `console`, resto → `pc`). O scan usa `Store.PlatformIDByName` (NOCASE, sem criar).
- Scrapers: `igdb.go` e `steamstore.go` devolvem `Platform: "PC"`; `domain.MatchScore` compara com o nome da plataforma
  do jogo (`EqualFold`).
- **Achado:** `copyLaunchBoxImages` passa `c.Platform` a `importers.LaunchBoxImages`, que monta
  `Images\<plataforma>\...`. Para jogos `Windows` do LaunchBox isso vira `Images\PC\...`, pasta que não existe: as
  imagens desses jogos não são copiadas, contra MID-010. A fase 9 corrige com PLT-028 (§5).
- Próxima migração: 00008; `store.SchemaVersion` passa de 7 para 8.

## 1. Pacotes e responsabilidades

| Pacote | Muda | Requisitos |
|---|---|---|
| `internal/domain` | `seed.go`: tabela `SeedPlatforms` e busca por chave e por nome canônico | PLT-001, PLT-017, PLT-025, PLT-029 |
| `internal/store` | migração `00008_platforms.sql`; queries; `Platform` com `SeedKey`, `Icon`, `GameCount`; resolução, update, ícone, exclusão; `AddGame` pela chave `windows` | PLT-001..PLT-008, PLT-014, PLT-017..PLT-021, PLT-025, PLT-027 |
| `internal/media` | cópia e remoção do ícone da plataforma em `data\media\platforms\` | PLT-011..PLT-013, PLT-020 |
| `internal/agent` | handlers `platform.update`, `platform.setIcon`, `platform.delete`; `platform.add` com `created`; importação com a resolução nova | PLT-011..PLT-021, PLT-024..PLT-029 |
| `internal/ipc` | tipos e constantes da v0.11.0; código -32013 | PLT-014, PLT-016, PLT-021 |
| `internal/importers` | `PlatformWindows = "Windows"` no lugar de `PlatformPC`; LaunchBox sem tradução | PLT-026, PLT-028 |
| `internal/scrapers` | `platforms.go` (identificador por provedor); "Windows" no IGDB e na Steam Store | PLT-030, PLT-031 |
| `cmd/arcadia` | bindings novos; `iconPath` → URL `/media/...` | PLT-011, PLT-023 |
| `frontend` | `PlatformsDialog`, `PlatformIcon`, ícones SVG, botão no cabeçalho, filtro do `FilterPanel` | PLT-009, PLT-010, PLT-015, PLT-016, PLT-018..PLT-023, PLT-032 |

`internal/tracker` não muda.

## 2. Domain [PLT-001, PLT-017, PLT-025, PLT-029]

```go
// internal/domain/seed.go
type SeedPlatform struct {
    Key      string           // stable seed key, e.g. "n64"
    Name     string           // canonical name, e.g. "Nintendo 64"
    Category PlatformCategory
}

// SeedPlatforms is the seed of migration 00008, in the order of contracts/seed-platforms.md.
var SeedPlatforms = []SeedPlatform{ /* 13 rows */ }

// SeedKeyWindows is the key of the platform of executable, Steam and Epic games.
const SeedKeyWindows = "windows"

// SeedByKey returns the seed platform of key; false for an unknown key (ADR 0012: tolerated on read).
func SeedByKey(key string) (SeedPlatform, bool)

// SeedByName returns the seed platform whose canonical name equals name after trimming, without regard to case.
func SeedByName(name string) (SeedPlatform, bool)
```

Puro, sem I/O. `SeedPlatforms` é a fonte da verdade; a migração SQL repete os valores e um teste de store compara.

## 3. Banco: migração 00008 [PLT-001..PLT-008]

Arquivo `internal/store/migrations/00008_platforms.sql`, numa só transação do goose (sem `NO TRANSACTION`), seguindo
os passos da tabela do contrato (§3 de seed-platforms.md). Esboço:

```sql
-- +goose Up
ALTER TABLE platform ADD COLUMN seed_key TEXT;

CREATE TEMP TABLE seed (key TEXT PRIMARY KEY, name TEXT NOT NULL, category TEXT NOT NULL);
INSERT INTO seed VALUES ('windows','Windows','pc'), ('3ds','Nintendo 3DS','handheld'), ... ;

-- Passo 2: fusão por maiúsculas (sobrevive o menor id de cada nome).
CREATE TEMP TABLE keep AS
  SELECT p.id AS old_id, (SELECT min(q.id) FROM platform q WHERE trim(q.name) = trim(p.name) COLLATE NOCASE) AS new_id
  FROM platform p;
UPDATE platform SET emulator_id = (...primeira outra com emulador, por id...) WHERE emulator_id IS NULL AND id IN (SELECT new_id FROM keep WHERE old_id <> new_id);
UPDATE game SET platform_id = (SELECT new_id FROM keep WHERE old_id = game.platform_id) WHERE platform_id IN (SELECT old_id FROM keep WHERE old_id <> new_id);
DELETE FROM platform WHERE id IN (SELECT old_id FROM keep WHERE old_id <> new_id);

-- Passos 3 e 4: PC → Windows (fusão se as duas existem; senão renomeia).
-- Passo 5: adota as linhas com nome canônico (seed_key, nome e categoria do seed).
-- Passo 6: insere as que faltam.
DROP TABLE seed; DROP TABLE keep;

CREATE UNIQUE INDEX platform_name_nocase_uidx ON platform (name COLLATE NOCASE);
CREATE UNIQUE INDEX platform_seed_key_uidx ON platform (seed_key) WHERE seed_key IS NOT NULL;

-- +goose Down
DROP INDEX platform_seed_key_uidx;
DROP INDEX platform_name_nocase_uidx;
ALTER TABLE platform DROP COLUMN seed_key;
```

- Ordem importa: o passo 2 antes do 3 garante no máximo uma "PC" e uma "Windows"; o passo 3/4 antes do 5 faz a linha
  renomeada ser adotada como `windows`.
- `UPDATE game` não toca `updated_at`: mudar de plataforma por migração não é edição do usuário (e evita reordenar
  listas por "atualizado em"). Sessões e totais não são tocados.
- `icon` passa a ter significado (contrato §2); como nunca foi escrito, todo `icon` existente é `NULL`.
- O `UNIQUE` binário antigo de `name` continua (SQLite não remove constraint de coluna sem reconstruir a tabela, e
  `game` depende dela com `RESTRICT`, ADR 0012); o índice `NOCASE` é mais restritivo e é o que vale.
- **Down** não desfaz dados (contrato §3); o comentário do arquivo diz isso.

## 4. Store [PLT-014, PLT-017..PLT-021, PLT-025, PLT-027]

```go
// Platform ganha campos (catalog.go).
type Platform struct {
    ID         int64
    Name       string
    Category   domain.PlatformCategory
    EmulatorID *int64
    SeedKey    string // "" when not a seed platform
    Icon       string // custom icon, relative to data\media; "" = default icon
    GameCount  int64  // games of the platform, hidden ones included
}

var (
    ErrPlatformNameTaken = errors.New("store: platform name already used")      // PLT-018 → -32602
    ErrPlatformInUse     = errors.New("store: platform has games")              // PLT-021 → -32013
)

// PlatformInUseError carries the count for the -32013 data.
type PlatformInUseError struct{ PlatformID, GameCount int64 }
func (e *PlatformInUseError) Error() string
func (e *PlatformInUseError) Unwrap() error // ErrPlatformInUse

// ResolvePlatform finds a platform by name with PLT-025 steps 1 and 2; false when none. Never creates (scan).
func (s *Store) ResolvePlatform(ctx context.Context, name string) (Platform, bool, error)

// AddPlatform resolves name (PLT-025) and returns the platform found with created=false, or creates it
// (seed key and seed category when the name is canonical and the key is free, PLT-017) with created=true.
func (s *Store) AddPlatform(ctx context.Context, name string, category domain.PlatformCategory) (Platform, bool, error)

// UpdatePlatform renames and recategorizes (PLT-018, PLT-019). ErrPlatformNotFound, ErrPlatformNameTaken.
func (s *Store) UpdatePlatform(ctx context.Context, id int64, name string, category domain.PlatformCategory) (Platform, error)

// SetPlatformIcon stores icon ("" = default) and returns the platform and the previous icon.
func (s *Store) SetPlatformIcon(ctx context.Context, id int64, icon string) (p Platform, previous string, err error)

// PlatformIconInUse reports whether any platform uses icon (PLT-013).
func (s *Store) PlatformIconInUse(ctx context.Context, icon string) (bool, error)

// DeletePlatform deletes a platform without games and returns its icon ("" when none).
// *PlatformInUseError when it has games; ErrPlatformNotFound when it does not exist.
func (s *Store) DeletePlatform(ctx context.Context, id int64) (icon string, err error)
```

- `Platforms` e `Catalog` passam a ler `GameCount` com `LEFT JOIN (SELECT platform_id, COUNT(*) ... GROUP BY
  platform_id)`; a ordem continua `name COLLATE NOCASE, id` (PLT-014).
- `DeletePlatform`: numa transação, `COUNT(*)` dos jogos e `DELETE` só se zero. O `RESTRICT` do banco é a segunda
  barreira: um jogo inserido entre a contagem e o `DELETE` não existe porque o agente é o único escritor e a transação
  do SQLite serializa as escritas (ADR 0015).
- `UpdatePlatform`: a unicidade é checada com `GetPlatformByNameNoCase` (ignorando o próprio id) e garantida pelo
  índice; erro de `UNIQUE` vira `ErrPlatformNameTaken`. Renomear para um nome canônico de seed com chave livre **não**
  atribui a chave (só a criação faz isso, PLT-017), para não mudar ícone e identificador de uma plataforma do usuário
  sem ele pedir.
- `AddGame` (PLT-027): troca `EnsurePCPlatform`/`GetPlatformIDByName('PC')` por `GetPlatformBySeedKey('windows')`; se não
  houver, `InsertSeedPlatform('Windows', 'pc', 'windows')` na mesma transação. Pela resolução (PLT-017, PLT-025) uma
  plataforma chamada "Windows" sem a chave só existe por escrita fora do agente; nesse caso, em vez de inserir, a chave
  `windows` é gravada nela (o índice `NOCASE` impediria o `INSERT`). O teste cobre: renomeada, excluída, recriada à mão.
- Consequência da resolução: com a plataforma `windows` renomeada para "PC", `platform.add("Windows")` devolve a "PC"
  com `created: false`; por isso a mensagem de PLT-016 usa o nome da plataforma devolvida.
- `EnsurePlatformID` e `PlatformIDByName` passam a usar `AddPlatform`/`ResolvePlatform`.
- Queries novas (sqlc): `GetPlatformBySeedKey`, `InsertSeedPlatform`, `UpdatePlatform`, `SetPlatformIcon`,
  `CountPlatformGames`, `DeletePlatformRow`, `CountPlatformsWithIcon`, `ListPlatformsWithCount`. Saem
  `EnsurePCPlatform` e `GetPlatformIDByName`.

## 5. Importadores [PLT-026, PLT-028, PLT-029]

- `importers.PlatformPC` é renomeada para `PlatformWindows = "Windows"`; Steam (`steam.go`) e Epic (`epic.go`) usam a
  nova. `agent.toStoreGame` usa `PlatformWindows` para candidato sem plataforma (Steam/Epic legados no fio).
- LaunchBox: `platformOf` deixa de traduzir `Windows`; o candidato leva o nome do XML. Como o nome canônico do seed é
  `Windows`, a resolução cai na plataforma `windows`. `copyLaunchBoxImages` continua passando `c.Platform`, que agora é
  sempre o nome do XML: a pasta `Images\Windows\...` passa a ser lida (PLT-028). Teste com fixture em `Images\Windows`.
- `platformCategoryFor(source, name)`: se `domain.SeedByName(name)` acha, a categoria do seed; senão a regra atual
  (LaunchBox → `console`, resto → `pc`). Na prática a categoria do seed já vem da store (PLT-017); a função fica coerente
  para o caso em que a resolução encontra a chave livre.
- Limitação conhecida (sem requisito nesta fase): uma plataforma nova criada para ROMs na fonte "Pasta" ganha `pc`;
  o usuário corrige pela troca de categoria (PLT-019).

## 6. Identificador nos provedores [PLT-030]

Opções consideradas:

| Opção | Prós | Contras |
|---|---|---|
| **A. Tabela no código** (`internal/scrapers/platforms.go`: `map[seedKey]map[provider]string`) | sem migração para corrigir ou acrescentar; dado do provedor fica no pacote do provedor; testes de tabela simples | só plataformas do seed; usuário não edita |
| B. Coluna por provedor (`platform.igdb_id`) | consulta direta; vale para qualquer plataforma | uma coluna por provedor; corrigir valor exige migração; seed em dois lugares |
| C. Tabela `platform_provider_ref(platform_id, provider, external_id)` | flexível, editável, qualquer plataforma | tabela e telas novas sem uso nesta fase; seed em dois lugares |

**Recomendação: A.** O banco guarda só a chave do seed (estável). Se a fase 12 precisar de identificador para
plataformas do usuário, ela acrescenta C por cima de A (A vira o padrão; C, a sobrescrita), sem desfazer nada.

```go
// internal/scrapers/platforms.go
// PlatformID returns the id of the seed platform of seedKey in provider (e.g. "igdb"), false when unknown.
func PlatformID(seedKey, provider string) (string, bool)
```

Os valores do IGDB estão no contrato e são conferidos na API antes de as tarefas serem aprovadas (verificação manual
de PLT-030). Valor não confirmado fica fora do mapa.

## 7. Scrapers [PLT-031]

- `igdb.go`: candidato com plataforma cujo nome contém "Windows" passa a `Platform: "Windows"` (hoje `"PC"`).
- `steamstore.go`: `Platform: "Windows"`.
- `domain.MatchScore` não muda; o teste ganha o caso "Windows" × "Windows". Um jogo cuja plataforma o usuário
  renomeou (ex.: "PC Windows") perde o bônus de 0,05 até a fase 12 casar por identificador — aceitável e registrado.

## 8. Agente e IPC [PLT-011..PLT-024]

Contrato v0.11.0 (delta em `contracts/ipc-delta.openrpc.json`):

| Método | Entrada | Saída | Erros |
|---|---|---|---|
| `platform.list` | — | `{platforms}` (Platform com `seedKey`, `iconPath`, `gameCount`) | -32602, -32603 |
| `library.catalog` | — | como antes, com o Platform novo | -32602, -32603 |
| `platform.add` | `name`, `category` | `{platform, created}` | -32602 |
| `platform.update` | `id`, `name`, `category` | `{platform}` | -32602 (nome repetido, inválido), -32001 |
| `platform.setIcon` | `platformId`, `path` \| `null` | `{platform}` | -32602 (`data.reason`), -32001 |
| `platform.delete` | `id` | `{id}` | -32001, **-32013** `{platformId, gameCount}` |
| `platform.setEmulator` | sem mudança | `{platform}` com o Platform novo | sem mudança |

- Handlers em `internal/agent/platforms.go` (novo), registrados junto de `gameHandlers`/`emulatorHandlers`.
- `iconPath` no fio é absoluto: `filepath.Join(<data>\media, platform.icon)`; vazio → `null`. O agente conhece a pasta
  de mídia (fase 7).
- **`platform.setIcon` com caminho** (PLT-011, PLT-012):
  1. valida (absoluto, `os.Stat` arquivo, ≤ 2 MB, conteúdo PNG/JPEG por assinatura) → `-32602` com `reason`;
  2. `media.Store.ImportPlatformIcon(src)` grava `platforms\<sha256>.<ext>` (escrita atômica, como `save`; sem
     miniatura) e devolve o caminho relativo;
  3. `store.SetPlatformIcon`; se falhar e o arquivo novo não for usado por ninguém, remove-o;
  4. se `previous != ""`, `previous != novo` e `PlatformIconInUse(previous)` é falso, `media.RemovePlatformIcon(previous)`.
     Falha ao apagar é só log (`slog.Warn`), não erro do método.
- **`platform.setIcon` com `null`** (PLT-013): passos 3 e 4.
- **`platform.delete`** (PLT-020, PLT-021): `store.DeletePlatform` e depois a remoção do ícone como no passo 4.
- **Concorrência:** as operações de ícone (setIcon e delete) passam por um `sync.Mutex` do handler de plataformas, para
  que "conferir se o arquivo está em uso" e "apagar" não se cruzem com outra plataforma passando a usar o mesmo hash.
  Sem goroutine nova; o mutex vive na struct dos handlers, criada pelo agente.
- Nenhum desses métodos notifica (PLT-024, IPC-050).

```go
// internal/media/platform_icon.go
const MaxPlatformIconBytes = 2 << 20

// ImportPlatformIcon copies a PNG or JPEG file into platforms\<sha256>.<ext> and returns the path relative to root.
// ErrTooLarge, ErrUnsupported (existing) for refused files.
func (s *Store) ImportPlatformIcon(ctx context.Context, src string) (string, error)

// RemovePlatformIcon deletes platforms\<name>; a path outside platforms\ is refused.
func (s *Store) RemovePlatformIcon(rel string) error
```

`RemovePlatformIcon` só apaga dentro de `<media>\platforms\` (confere com `filepath.Rel`, como `mediaURL`), para que um
valor estranho no banco nunca apague arquivo fora dali.

## 9. Desktop e UI [PLT-009, PLT-010, PLT-015..PLT-023, PLT-032]

- `cmd/arcadia`: bindings `PlatformUpdate(id, name, category)`, `PlatformSetIcon(id, path *string)`,
  `PlatformDelete(id)`, `PickPlatformIcon()` (filtro "Imagens (*.png;*.jpg;*.jpeg)", ARCHITECTURE §11). A view de
  plataforma ganha `seedKey`, `iconUrl` (de `iconPath` por `mediaURL`, que já só aceita caminho dentro de
  `data\media`) e `gameCount`. `AddPlatform` devolve também `created`.
- `frontend/src/assets/platforms/*.svg` (13 + `generic.svg`, contrato §2) e `components/PlatformIcon.tsx`:
  `iconUrl` → `<img src={iconUrl}>`; senão `seedKey` conhecido → SVG embutido; senão genérico (PLT-009, PLT-010).
- `components/PlatformsDialog.tsx`, no molde do `EmulatorsDialog`: lista (ícone, nome, categoria, nº de jogos, seletor
  de emulador padrão); "Nova plataforma"; "Editar" (nome, categoria, ícone com campo de caminho + "Procurar..." +
  "Restaurar padrão"); "Excluir" com dois toques, desativado com explicação quando `gameCount > 0`. Erros do agente
  aparecem no `role="alert"`, como no diálogo de emuladores. Depois de cada escrita invalida `libraryKeys.catalog` (e a
  lista de jogos depois de renomear, porque o cartão e a página mostram o nome da plataforma).
- Cabeçalho: botão "Plataformas" ao lado de "Emuladores" (PLT-023).
- `FilterPanel`: lista as plataformas com `gameCount > 0` ou já marcadas no filtro (PLT-032).
- O seletor de plataforma do editor do jogo (EDT-002) e o da importação (IMP-004) não mudam; passam a mostrar as 13.

## 10. Testes (cada caso começa pelo ID)

- `domain`: `SeedByKey`/`SeedByName` (grafias, espaços, desconhecido); 13 chaves únicas; categorias válidas (DOM-015).
- `store`: migração 00008 a partir de bancos v7 montados no teste: vazio (PLT-001), só "PC" (PLT-002), "PC" + "Windows"
  com e sem emulador (PLT-003), "Sony PlayStation 2" (PLT-004), duplicatas por maiúsculas (PLT-005); índice `NOCASE`
  com escrita direta (PLT-006); reabrir não recria (PLT-007); banco migrado == `domain.SeedPlatforms`. Resolução
  (PLT-025), criação com chave livre (PLT-017), update, ícone, exclusão com e sem jogo oculto (PLT-018..PLT-021),
  `AddGame` com `windows` renomeada e excluída (PLT-027), `GameCount` (PLT-014).
- `media`: `ImportPlatformIcon` (PNG, JPEG, > 2 MB, texto com extensão `.png`, pasta); `RemovePlatformIcon` recusa
  caminho fora de `platforms\`.
- `agent`: handlers com store real em `t.TempDir()` e pasta de mídia temporária: setIcon/restaurar/trocar com arquivo
  compartilhado (PLT-011..PLT-013), delete -32013 com `data` (PLT-021), `created` (PLT-016), sem notificação (PLT-024),
  apply de Steam/Epic/LaunchBox em `windows` (PLT-026, PLT-028), `platformCategoryFor` (PLT-029).
- `importers`: Steam/Epic com `PlatformWindows`; LaunchBox mantém `Windows`; `LaunchBoxImages` acha `Images\Windows`.
- `scrapers`: `PlatformID` (13 chaves, desconhecida, provedor sem valor); IGDB e Steam Store devolvem "Windows" (PLT-031).
- `ipc`: teste de contrato v0.11.0 (campos novos, `created`, -32013).
- `cmd/arcadia`: `iconUrl` dentro e fora de `data\media`; bindings chamam os métodos certos.
- `frontend` (Vitest): `PlatformIcon` (3 casos), `PlatformsDialog` (lista, criar com nome repetido, editar, excluir
  bloqueado e com confirmação, emulador padrão), `FilterPanel` sem plataformas vazias.
- Manual: PLT-023 (diálogo e seletor nativo), conferência visual dos 13 ícones, ids do IGDB na API (PLT-030), e uma
  biblioteca real com "PC" e LaunchBox migrando para a v8 sem perder jogos nem tempo.

## 11. Riscos e pontos de atenção

- **Testes existentes assumem banco sem plataformas.** Depois da 00008 todo banco novo tem 13 plataformas: testes como
  `catalog_test.go` (lista exata), `games_test.go` (`len(ps) == 1`, "PC"), `library_fixture_test.go` (plataforma padrão
  "PC") e os de `agent` que criam "PC" precisam ser atualizados pelo test-writer (por exemplo, usar "Windows" do seed ou
  filtrar por nome). É mudança esperada, não regressão; o planner deve prever uma tarefa para isso.
- **Divergência seed SQL × `domain.SeedPlatforms`:** coberta pelo teste de igualdade.
- **Fusão na migração com jogos de mesmo título:** depois de PLT-003/PLT-005 podem existir dois jogos de mesmo título na
  mesma plataforma. Nada quebra (não há `UNIQUE (title, platform_id)`); IMP-006 só passa a considerar os dois.
- **Arquivos de ícone órfãos:** se o agente cair entre gravar o arquivo e gravar o banco, sobra um arquivo em
  `platforms\` sem dono. Inofensivo (nome por hash); uma limpeza fica fora de escopo.
- **Ícone do usuário fora de `data\media`:** impossível por construção (o agente sempre copia); `mediaURL` e
  `RemovePlatformIcon` recusam caminho que sai da pasta.
- **Marca registrada:** os ícones do seed não usam logotipos; os nomes das plataformas são usados só para identificá-las.

## 12. Decisões que merecem ADR (a sessão principal escreve)

- **Plataformas do seed com chave estável**: seed por migração (uma vez), `seed_key` como identidade que sobrevive a
  renomear, recuperação pelo nome canônico, e identificadores de provedor em código indexados pela chave (§6, opção A).
- **Unicidade de nome de plataforma sem diferenciar maiúsculas no banco** (índice `NOCASE` sobre o `UNIQUE` binário),
  com fusão na migração. Pequeno, mas muda uma regra de schema da migração 1.
