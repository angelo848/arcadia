# Fase 11 — Importação v2: design

- **Requisitos:** [requirements.md](requirements.md) (rascunho, 2026-11-15) · **Tarefas:** `tasks.md` (planner)
- **Contratos:** [contracts/ipc-v0.13.0-delta.md](contracts/ipc-v0.13.0-delta.md) (acréscimo sobre a v0.12.0; o JSON completo é
  uma tarefa) e [contracts/import-rules.md](contracts/import-rules.md) (extensões, vetores, grupos de disco, avisos).
- **ADRs:** 0014, 0015, 0026, 0027, 0029; proposto 0032 (leitura de pasta por lista fechada de extensões).

## 0. Estado atual (conferido no código)

| Ponto | Onde | O que acontece hoje |
|---|---|---|
| Pasta | `internal/importers/folder.go` | `os.ReadDir` do primeiro nível; todo arquivo vira candidato (sem filtro); título = nome sem `filepath.Ext`; `InstallDir` = a pasta escolhida (vira a regra `install_dir` de RUL-001 para **todo** jogo da pasta). |
| Arquivo | `internal/importers/file.go` | Um candidato; título = nome sem extensão; `InstallDir` = pasta do arquivo; `runnableExts` decide `exe` × `emulator`. |
| Tipos | `internal/importers/types.go` | `Candidate` sem campos de arquivo; `ScanInput{Path}`. |
| Scan | `internal/agent/imports.go` `handleImportScan` | Valida caminho absoluto; chama `Source.Scan`; `candidateExists` usa a plataforma do candidato (vazia em arquivo e pasta, então só o alvo é conferido). Sem limite de tamanho do resultado. |
| Apply | `handleImportApply` / `toStoreGame` | Valida o título (`domain.ValidateTitle`), resolve plataforma por nome (`EnsurePlatformID`) ou pela chave `windows`; uma transação por jogo; 1..500 candidatos. |
| Binding | `cmd/arcadia/imports.go` | `ImportScan(source, path)`, `ImportApply(candidates, launchboxPath)`, `ImportCandidateView` com os 15 campos da v0.7.0. |
| Janela | `frontend/src/components/ImportDialog.tsx` | `Modal size="large"`; lista de `<li>` com caixa + título; **nenhuma** linha marcada depois do scan; campo de texto "Plataforma do arquivo" com `'PC'` (resto da fase 9), mostrado **depois** da lista; lotes de 25 por `applyImportInLots`. |
| Mensagem do IPC | `internal/ipc/protocol.go` | `MaxMessageSize = 1 << 20`: um `import.scan` com mais de ~1.000 candidatos de pasta não caberia. |
| Contrato | `…/phase-10-2…/ipc.openrpc.json` v0.12.0 | `ImportCandidate` sem os campos novos; `ImportScanParams` com `source` e `path`. |

Duas dívidas achadas na leitura e tratadas aqui: o `InstallDir` amplo demais (IMV-016) e o teto de 1 MiB (IMV-019).

## 1. Arquivos e responsabilidades

| Arquivo | Mudança | IDs |
|---|---|---|
| `internal/importers/title.go` (novo) | `SplitExt`, `NormalizeTitle`, `DiscNumber`, `titleKey` | IMV-001..IMV-008, IMV-021 |
| `internal/importers/folder_rules.go` (novo) | tabelas: extensões por lista, contêineres, executáveis ignorados, nomes de pasta ignorados | IMV-012, IMV-013, IMV-015, IMV-020 |
| `internal/importers/folder_walk.go` (novo) | leitura recursiva, ordem natural, limites, contadores | IMV-010..IMV-012, IMV-018 |
| `internal/importers/folder_groups.go` (novo) | acompanhantes, grupos de disco, `.m3u` | IMV-020..IMV-023 |
| `internal/importers/folder.go` | `folderSource` monta candidatos a partir do resultado da leitura; avisos | IMV-010, IMV-014, IMV-016, IMV-017, IMV-024 |
| `internal/importers/file.go` | título normalizado e campos de IMV-024 | IMV-032 |
| `internal/importers/types.go` | `Candidate` ganha `File`, `Extension`, `Discs`, `DiscFiles`; `ScanInput` ganha `Folder FolderOptions` | IMV-024 |
| `internal/ipc/import.go` | campos novos de `ImportCandidate` e `ImportScanParams`; constantes `ImportTitleFrom*` | IMV-070 |
| `internal/agent/imports.go` | opções do scan, plataforma → classe, `candidateExists` com plataforma, corte por tamanho | IMV-019, IMV-030, IMV-071, IMV-072 |
| `cmd/arcadia/imports.go` | `ImportScanOptions`, `ImportCandidateView` com os campos novos | IMV-073 |
| `frontend/src/lib/imports.ts` | tipos, `scanImport(source, path, opts)`, helpers puros | IMV-041..IMV-049, IMV-061 |
| `frontend/src/components/ImportReviewTable.tsx` (novo) | tabela de revisão | IMV-040..IMV-047, IMV-051, IMV-052 |
| `frontend/src/components/ImportDialog.tsx` | formulário (fonte, caminho, título, subpastas, plataforma), estado das linhas, lotes | IMV-048..IMV-050, IMV-060..IMV-063 |
| `docs/specs/phase-11-importacao-v2/contracts/ipc.openrpc.json` (criado na T01) | contrato completo v0.13.0 | IMV-070 |

Nada em `internal/tracker`, `cmd/fake-game`, `internal/store` (nem migração nem `sqlc generate`), `internal/domain` (a chave
`windows` vira constante local do agente) nem nos testes de sessão (IMV-082). A decisão de classe usa
`store.ResolvePlatform`, que já existe e já devolve `SeedKey`.

## 2. Normalizador de título [IMV-001..IMV-008, IMV-021]

```go
// internal/importers/title.go

// SplitExt splits a file name into stem and extension [IMV-001]. The extension is the text after the last dot
// when it has 1..5 [A-Za-z0-9] characters with at least one letter; it is returned in lower case without the dot.
func SplitExt(name string) (stem, ext string)

// NormalizeTitle returns the proposed title of a file stem or folder name [IMV-001..IMV-006, IMV-008].
// fromFolder enables the removal of the "NN. " ordering prefix [IMV-006]. It is pure: no clock, no disk.
// It returns "" only when stem is blank after trimming [IMV-005].
func NormalizeTitle(stem string, fromFolder bool) string

// DiscNumber returns N of the first "(Disc N)", "(Disk N)", "(CD N)", "(Disc N of M)" tag, in () or [], or 0 [IMV-021].
func DiscNumber(stem string) int

// titleKey is the grouping key of a file: the normalized title in lower case [IMV-020, IMV-021].
func titleKey(stem string) string
```

Implementação (regras e vetores em `import-rules.md` §2):

- Regexes compiladas no pacote: `reGroupParen = \([^()]*\)`, `reGroupBracket = \[[^\[\]]*\]`, `reOrderPrefix = ^\d{1,3}\.\s+(\D)`
  (o grupo `(\D)` é recolocado), `reDisc` de `import-rules.md` §4. A remoção de grupos é um laço "até não mudar" (cada volta
  remove os grupos mais internos). Limite de voltas = comprimento do texto (nunca infinito).
- O ponto → espaço roda sobre `[]rune`, com a exceção "entre dois dígitos" olhando o vizinho anterior e o seguinte.
- O artigo usa uma regex única: `(?i)^(.+?),\s+(the|an|a|les|le|la|el|los|las|il|der|die|das)(\s+-\s+|:\s+|\s*$)`; o artigo é
  recomposto como `Artigo + " " + principal + resto`, com a primeira letra do artigo em maiúscula. A ordem na alternância
  coloca os nomes mais longos antes dos curtos (`an` antes de `a`, `las`/`les`/`los` antes de `la`/`le`).
- Corte em 200 runas: `[]rune(s)[:domain.MaxTitleRunes]` e nova aparagem. `internal/importers` pode importar `internal/domain`
  (não é Windows-específico).
- `NormalizeTitle` não usa `strings.Title` nem `unicode.ToTitle` fora da primeira letra do artigo.

**Fonte do título** na leitura de pasta (IMV-017): `titleFrom = file` → `NormalizeTitle(stem, false)`; `folder` →
`NormalizeTitle(<nome da subpasta de primeiro nível>, true)`, caindo para o título do arquivo quando o resultado é vazio ou o
arquivo está na raiz. O **agrupamento** usa sempre `titleKey(stem do arquivo)`, nunca o título de pasta.

## 3. Leitura de pasta [IMV-010..IMV-024]

### Tipos

```go
// internal/importers/types.go
type Candidate struct {
	// ... campos atuais ...
	// File is the main file relative to the chosen folder (the file name for the file source); Extension is its
	// lower-case extension without the dot; Discs is the number of discs (>=1 for file and folder candidates, 0 for the
	// other sources); DiscFiles lists the disc files in disc order when Discs >= 2 [IMV-024].
	File      string
	Extension string
	Discs     int
	DiscFiles []string
}

// internal/importers/folder.go
type TitleFrom string

const (
	TitleFromFile   TitleFrom = "file"
	TitleFromFolder TitleFrom = "folder"
)

// FolderKind is the class of extensions a folder scan reads [IMV-013].
type FolderKind int

const (
	KindAny         FolderKind = iota // both lists (no platform, unknown platform, user-created platform)
	KindExecutables                   // platform of seed key windows
	KindConsole                       // platform of any other seed key
)

// FolderOptions are the scan options of the folder source. The zero value is the default of the contract:
// recursive, title from the file, both lists. MaxDepth and MaxItems are 0 for the defaults (MaxScanDepth,
// MaxScanItems); tests inject small values [IMV-018].
type FolderOptions struct {
	FirstLevelOnly bool // zero value = read the subfolders [IMV-010, IMV-011]
	TitleFrom      TitleFrom
	Kind           FolderKind
	MaxDepth       int
	MaxItems       int
}

const (
	MaxScanDepth = 8     // x-import.maxScanDepth [IMV-018]
	MaxScanItems = 20000 // x-import.maxScanItems [IMV-018]
)

// ScanInput gains Folder; the other sources ignore it.
type ScanInput struct {
	Path   string
	Folder FolderOptions
}

// dirReader is all the folder scan needs from the disk: directory entries and nothing else. It has no Open, so a
// spy that implements only ReadDir proves that no file is opened [IMV-080].
type dirReader interface {
	ReadDir(name string) ([]fs.DirEntry, error)
}
```

`Folder()` usa `osDirReader{}` (`os.ReadDir`); `folderSource{dirs dirReader}` e um construtor interno para os testes. A raiz
continua validada com `os.Stat` (avisos da fase 6 inalterados).

### Fluxo (uma função por etapa, todas puras exceto a primeira)

1. **`walk(ctx, dirs, root, opts) walkResult`** — percorre em profundidade, em ordem natural de cada diretório (ordena as
   entradas de cada pasta com `naturalLess`, não só o `ReadDir` que ordena por nome binário). Para cada entrada: pula as de
   IMV-012; conta o item; se passou de `MaxItems`, marca `itemsCut` e para; pasta além de `MaxDepth` marca `depthCut`;
   `ReadDir` com erro em subpasta incrementa `unreadable` e segue; confere `ctx.Err()` ao entrar em cada pasta. Devolve
   `files []fileEntry{dirRel, name, scope}` (todos os arquivos vistos, em ordem de visita) e os contadores.
2. **`companions(files)`** — por pasta, acha contêineres (`cue`, `ccd`, `mds`, `gdi`) e marca como acompanhantes os arquivos
   da tabela §4.1 de mesma `titleKey` (IMV-020). Acompanhantes saem da lista e **não** contam como ignorados.
3. **`classify(files, kind)`** — mantém os que têm extensão da lista da classe (IMV-013) e passam pela lista negra de
   executáveis (IMV-015); o resto soma em `ignoredByExt[ext]` (e `ignoredConsole`/`ignoredExe` para os avisos de IMV-014).
4. **`groupDiscs(primaries)`** — IMV-021..IMV-023: agrupa por `(scope, titleKey)` os que têm `DiscNumber ≥ 1`, na ordem natural,
   com a regra do "número de disco já presente"; junta o `.m3u` correspondente. Devolve `[]entry{main, discs}`.
5. **`toCandidates(entries, root, opts)`** — título (§2), `launchType` (`runnableExts`), `launchTarget` =
   `filepath.Join(root, main.rel)`, `installDir` = pasta do arquivo principal (IMV-016), `file`, `extension`, `discs`,
   `discFiles`. Ordena por `naturalLess(main.rel)` e devolve.
6. **`warnings(...)`** — monta os avisos de `import-rules.md` §5, nessa ordem: ignorados por extensão, plataforma trocada,
   profundidade, itens, ilegíveis, "nenhum jogo".

`naturalLess(a, b string) bool` compara segmento a segmento (separador `\` ou `/`), em minúsculas, com blocos de dígitos
comparados como inteiro (sem estourar: compara por tamanho depois de tirar zeros à esquerda, depois lexicograficamente; empate
total → ordem de runas do original). Fica em `internal/importers/natural.go` com teste próprio de tabela.

- **Concorrência e handles:** nenhuma goroutine nova e nenhum handle do Windows. `os.ReadDir` abre e fecha o diretório
  internamente; a leitura roda na goroutine do handler do IPC e respeita `ctx` (cancelamento da conexão ou timeout de 60 s do
  desktop).
- **Memória:** no máximo 20.000 `fileEntry` (nome + caminho relativo), poucos MB.
- **Reparse points:** `fs.DirEntry.Type()` de junção no Windows vem como `fs.ModeIrregular` em Go recentes; o teste de IMV-012
  cobre `ModeSymlink` e `ModeIrregular` por um `dirReader` falso (criar junção no `t.TempDir()` não é portátil), e o link
  simbólico real só quando o processo tem privilégio (senão `t.Skip`).
- **`file.go`:** `SplitExt` (extensão só quando válida) + `NormalizeTitle(stem, false)`; `File` = nome, `Discs` = 1. Mantém `InstallDir`
  e `runnableExts`.

## 4. Agente [IMV-019, IMV-030, IMV-071, IMV-072]

```go
// internal/agent/imports.go
type importStore interface {
	// ... os atuais ...
	// ResolvePlatform finds a platform by name without creating it (PLT-025 steps 1 and 2); it carries SeedKey.
	ResolvePlatform(ctx context.Context, name string) (store.Platform, bool, error)
}

const seedKeyWindows = "windows" // domain.SeedPlatforms key; resolved by key, never by name (ADR 0026)

// maxScanResultBytes is x-import.maxScanResultBytes: 900 KiB of the 1 MiB message limit [IMV-019].
const maxScanResultBytes = 900 << 10
```

`handleImportScan` passa a:

1. Decodificar `ImportScanParams` (campos novos) e validar `titleFrom` e `platform` (`-32602` antes de qualquer leitura, IMV-071).
2. Para `file` e `folder`: se `platform != ""`, `ResolvePlatform`; achada → `scanPlatformID = p.ID` e
   `kind` = `KindExecutables` (`SeedKey == seedKeyWindows`), `KindConsole` (outra chave) ou `KindAny` (sem chave); não achada
   ou vazia → `scanPlatformID = 0`, `KindAny`. O `ScanInput.Folder` leva `FirstLevelOnly = (Recursive != nil && !*Recursive)`,
   `TitleFrom` (padrão `file`) e `Kind`.
3. `candidateExists(ctx, st, c, scanPlatformID)`: para candidato sem plataforma de fonte `file`/`folder`, usa `scanPlatformID` no
   lugar de 0; o resto da função não muda (PLT-026, PLT-028).
4. **Corte por tamanho (IMV-019):** depois de converter os candidatos, soma `len(json.Marshal(c)) + 1` na ordem; no primeiro
   que passa de `maxScanResultBytes` descarta o resto e acrescenta o aviso de `import-rules.md` §6. Só vale para as fontes
   `file` e `folder` (Steam, Epic e LaunchBox não mudam nesta fase; o LaunchBox com milhares de jogos tem o mesmo teto, ver
   risco 3).
5. `toIPCCandidate` copia os campos novos (`File`, `Extension`, `Discs`, `DiscFiles`) com `omitempty`.

`handleImportApply` **não muda** de lógica (IMV-072): `toStoreGame` já apara e valida o título recebido e ignora os campos
novos. A fase só acrescenta testes (título editado gravado; título vazio recusado como `failed`).

## 5. Contrato e binding [IMV-070, IMV-073]

Ver [contracts/ipc-v0.13.0-delta.md](contracts/ipc-v0.13.0-delta.md). O teste de contrato (`internal/ipc/contract_test.go`)
passa a apontar para o JSON v0.13.0, confere os campos novos pelos nomes JSON (`assertExactFields` de `ImportCandidate` e de
`ImportScanParams`), a versão, e que `x-import` bate com `importers.MaxScanDepth`, `importers.MaxScanItems` e a constante do
agente (exposta ao teste do mesmo modo que a janela de `media.changed` na 10.2: helper exportado em `internal/agent` ou valor
duplicado com comentário, o planner decide).

`cmd/arcadia/imports.go`: `ImportScanOptions{Platform, TitleFrom string; Recursive *bool}` e `ImportCandidateView` com os campos
novos; `ImportScan(source, path string, opts ImportScanOptions)` monta `ipc.ImportScanParams`. Os arquivos
`frontend/wailsjs/go/**` são regerados pelo `wails build`.

## 6. Frontend [IMV-040..IMV-063]

### Estado e helpers puros (`lib/imports.ts`)

```ts
export type ImportScanOptions = { platform: string; titleFrom: 'file' | 'folder'; recursive: boolean }

// ImportCandidate gains file?, extension?, discs?, discFiles? (absent for steam, epic, launchbox) [IMV-024].
export function scanImport(source: ImportSource, path: string, opts: ImportScanOptions): Promise<ImportScanResult>

// isTitleEditable: only the file and folder sources have an editable title [IMV-044].
export function isTitleEditable(source: ImportSource): boolean
// titleProblem returns 'empty' | 'tooLong' for a trimmed title (rune count > 200), else null [IMV-045].
export function titleProblem(title: string): 'empty' | 'tooLong' | null
// repeatedFlags marks the 2nd and later non-existing rows with the same trimmed, case-insensitive title [IMV-046].
export function repeatedFlags(rows: { title: string; exists: boolean }[]): boolean[]
// defaultPlatformName picks the platform of seed key "windows", else the first one, else null [IMV-061].
export function defaultPlatformName(platforms: Pick<PlatformView, 'name' | 'seedKey'>[]): string | null
```

`ImportDialog` guarda `rows: ReviewRow[] | null` com `ReviewRow = { candidate; title; checked }` no lugar de `candidates` +
`checked: Set<number>`, e `platform: string | null` (nome). Plataformas vêm de `useCatalog()` (a mesma consulta do painel
esquerdo; as mutações de plataforma já a invalidam, então uma plataforma criada no diálogo Plataformas aparece aqui sem
recarregar).

- **Leitura:** `scan()` chama `scanImport(source, path, { platform, titleFrom, recursive })` e monta as linhas:
  `title = candidate.title`; `repeated = repeatedFlags(...)`; `checked = !exists && !repeated[i]` (IMV-041).
- **Mudança de entrada (IMV-049):** os `onChange` de fonte, caminho (depois do seletor), plataforma, "Título a partir de" e
  "Ler subpastas" chamam `resetList()` (`rows = null`, `result = null`, `warnings = []`).
- **Edição:** `onTitle(i, v)` atualiza só `rows[i].title`; os selos "título repetido" e os problemas são derivados (`useMemo` de
  `repeatedFlags(rows)` e de `titleProblem`), nunca guardados.
- **Confirmar:** `chosen = rows.filter(r => r.checked && !r.candidate.exists).map(r => ({ ...r.candidate, title: r.title.trim(),
  platform: fileLike ? platform : r.candidate.platform }))` → `applyImportInLots` (inalterado). Desativa o botão com
  `busy || selected === 0 || hasInvalidChecked || (fileLike && platform === null)`.
- **Depois do lote:** enviadas → desmarcadas; as de `result.applied` viram `exists = true` casando o título aparado com
  `applied[].title` (cada título do resultado consome uma linha, em ordem); não enviadas (falha de lote) continuam marcadas.
- **Contador:** `k = rows marcadas`, `n = rows não-exists`, `j = rows exists`: `"{k} de {n} jogos selecionados"` + `" ({j} já existem)"`.
- **Cabeçalho:** `ref` na caixa "Selecionar todos" para `indeterminate` (propriedade do DOM, não atributo).

### `ImportReviewTable` (apresentacional)

Props: `rows`, `source`, `busy`, `repeated: boolean[]`, `onToggle(i, on)`, `onToggleAll(on)`, `onTitle(i, value)`. Uma `<table>` com
`<thead>` fixo (`sticky top-0 bg-surface`) e `<tbody>`; linha em `React.memo` com callbacks estáveis (`useCallback` + índice), para
que digitar num título não re-renderize as outras 999 linhas. Colunas: caixa (largura fixa), Título (flex, `<input>` ou texto),
Arquivo (`truncate`, `title={launchTarget}`), Extensão, Disco. Selos (`já existe`, `título repetido`) em `text-xs text-muted` /
`text-warning`; `aria-invalid` e a mensagem de erro por `aria-describedby`. Sem cores literais e sem token novo (IMV-051).

Nomes acessíveis (IMV-052): caixa `aria-label="Selecionar {título atual}"`; campo `aria-label="Título de {arquivo || título proposto}"`.

### Formulário

Primeira linha (como hoje): Fonte · [Arquivo/Pasta + botão Escolher] · [só `folder`: "Título a partir de" (select: "Nome do
arquivo" | "Nome da pasta do jogo"), caixa "Ler subpastas" (marcada)] · [`file`/`folder`: "Plataforma" (select)] · Buscar. O
`<select>` de plataforma lista `catalog.platforms` (já ordenadas por nome). A mensagem de IMV-062 aparece no lugar do select.
Os avisos e o resultado continuam abaixo da tabela, como hoje. O rótulo "Plataforma do arquivo" é mantido (agora de um select),
para os testes e a leitura de tela continuarem achando o campo pelo mesmo nome.

### Tamanho e rolagem [IMV-051]

`Modal size="large"` não muda. A tabela fica num contêiner `min-h-0 flex-1 overflow-y-auto` (como a lista de hoje), com o
cabeçalho `sticky`. Mil linhas em DOM simples (sem virtualização): cada linha tem 1 caixa, 1 `<input>` e 3 células de texto, na
casa de 5 mil nós; aceitável. Se a conferência manual mostrar lentidão, virtualizar é uma melhoria separada (fora desta spec).

## 7. Alternativas consideradas

| Decisão | Escolhida | Alternativa | Por que não |
|---|---|---|---|
| Título da pasta | arquivo por padrão + opção de pasta | só pasta; só arquivo | O LaunchBox usa o arquivo (conferido); só arquivo não serve para `game.exe` por pasta; só pasta quebra pasta com várias ROMs. |
| Pasta do jogo | subpasta de **primeiro nível** da pasta escolhida | pasta imediata do arquivo | A imediata dá `bin`, `Disc 1`, `Release`; a de primeiro nível dá o nome do jogo nos dois layouts do usuário e agrupa os discos. |
| Filtro de extensão | lista fechada por classe de plataforma | lista de ignoradas; sem filtro | Lista de ignoradas nunca acaba (DLL, `.pak`, `.sav`); sem filtro era a causa de lixo na leitura recursiva. |
| Acompanhantes e discos | por nome | ler `.cue`/`.m3u`/`.gdi` | Ler o conteúdo exige parser, codificações e caminhos relativos; o nome resolve os casos reais e preserva "nenhum arquivo é aberto". |
| Resultado grande | corte por bytes com aviso | paginar `import.scan` com cursor | Paginação pede estado no agente (cursor/sessão) e mais contrato; mil jogos por leitura cobre o uso real. |
| Marcação inicial | tudo marcado (menos exists/repetido) | nada marcado (hoje) | Pedido do roadmap; a revisão serve para desmarcar o que não quer. |
| Título repetido | selo + desmarcado | recusar na UI | O `import.apply` já recusa; o selo só antecipa o aviso sem bloquear quem quer corrigir o título. |
| Plataforma no scan | `platform` opcional no `import.scan` | novo método `import.check` para reconferir "já existe" | Um método a mais por um caso (título editado) que o apply já cobre; a plataforma entra uma vez, antes da leitura. |
| `installDir` | pasta do arquivo (como a fonte Arquivo) | vazio para ROM | Mantém o comportamento da fonte "Arquivo"; a regra `install_dir` de uma pasta de ROM não casa processo de emulador (risco 5). |

## 8. Divergências antigas do contrato v0.12.0 (correção aprovada pelo usuário)

> **Decisão (aprovada com a spec):** as duas divergências abaixo são corrigidas na v0.13.0, junto com os acréscimos
> (`launchType` passa a listar `emulator`; `lastPlayed` vira `string` RFC 3339). Para o planner: o teste "só acréscimo" da
> fase 11 ignora essas duas correções.

Conferidas ao ler `ipc.openrpc.json` da fase 10.2 contra `internal/ipc/import.go`:

1. `ImportCandidate.launchType` tem `enum: ["exe","uri"]`; o código aceita também `"emulator"` (EMU-003, fase 6b) e o scan de
   pasta e arquivo devolve `emulator` para ROM.
2. `ImportCandidate.lastPlayed` é `integer` no contrato; o código usa texto RFC 3339 (`*string`) e o frontend também.

Nenhum teste de contrato as pega hoje (`assertExactFields` confere nomes de campo, não tipos). A pergunta 7 dos requisitos
propõe corrigi-las na v0.13.0; sem aprovação, o JSON v0.13.0 as mantém iguais e esta seção vira dívida registrada.

## 9. Testes e riscos

- **Go (`internal/importers`):** `title_test.go` (vetores de `import-rules.md` §2 e §2.1, idempotência, propriedade com semente
  fixa); `natural_test.go`; `folder_test.go` (árvores A..F de §4.4, limites injetados, ordem, ignorados, classes de extensão,
  `titleFrom`, `FirstLevelOnly`, cancelamento, `dirReader` espião sem `Open`); `file_test.go` (IMV-032). Fixtures são
  arquivos vazios em `t.TempDir()`.
- **Go (`internal/agent`):** `imports_test.go` (plataforma resolvida: renomeada, inexistente, sem chave; `exists` por título +
  plataforma; corte por bytes; `titleFrom` e `platform` inválidos; título editado gravado e inválido recusado; scan e apply não
  criam sessão); teste de contrato em `internal/ipc` (IMV-070).
- **`cmd/arcadia`:** repasse de opções e campos (IMV-073).
- **Frontend:** `imports.test.ts` (helpers puros), `ImportReviewTable.test.tsx`, `ImportDialog.test.tsx` (reescrito: os testes
  de IMP-010 que esperam "nada marcado" e campo de texto de plataforma são trocados pelos de IMV-040..IMV-063),
  `imports.lots.test.ts` (inalterado), `App.invariants.test.tsx` (IMV-081), `theme-scan.test.ts` (cobre os `.tsx` novos).
- **Riscos:**
  1. *Normalizador agressivo:* remover tudo entre parênteses pode apagar parte legítima do título (limitação 6). Mitigação: a
     revisão deixa corrigir; o nome do arquivo e o alvo nunca mudam.
  2. *Ordem natural e agrupamento dependem de `ToLower`:* nomes com maiúsculas especiais (`İ`) comparam por runas
     minúsculas do Go; o teste fixa o comportamento para ASCII e acentos comuns.
  3. *Teto de 1 MiB também vale para o LaunchBox e para `import.apply`:* o scan do LaunchBox não tem corte nesta fase (fora do
     escopo); uma biblioteca do LaunchBox com mais de ~1.000 jogos teria o mesmo problema que esta fase resolve só para
     pasta. Fica registrado para uma fase própria.
  4. *Leitura lenta em disco de rede:* o teto de 20.000 itens e o `ctx` limitam, mas 20.000 `ReadDir` remotos podem passar
     de 60 s e dar `timeout` no desktop; a mensagem é a de UI-008. Aceito.
  5. *`install_dir` de pasta de ROM:* a regra de rastreio que RUL-001 grava para um jogo `emulator` aponta para a pasta da ROM,
     como já acontece na fonte "Arquivo"; o watcher só adota processos que casam, e um emulador não roda de dentro da pasta da
     ROM. Se aparecer um caso real, a correção é do planner/tracker em fase própria (a spec desta fase não mexe no tracker).
  6. *Re-renderização da tabela:* 1.000 linhas com `<input>` controlado; mitigada com `React.memo` e callbacks estáveis; o teste
     de componente mede só correção, não tempo.
  7. *Plataforma apagada entre a leitura e a confirmação:* o `import.apply` a recria por nome (PLT-025/PLT-017); mesmo
     comportamento de hoje.
- **Cobertura da UI de IMV-083:** manual (a janela real, com a pasta real do usuário).

## 10. ADR proposto

0032 — Leitura de pasta por lista fechada de extensões, grupos por pasta do jogo e resultado limitado ao teto do IPC (ver
requirements.md, "ADR proposto").
