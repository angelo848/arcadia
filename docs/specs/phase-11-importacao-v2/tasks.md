# Fase 11 — Importação v2: tarefas

- **Status:** rascunho para aprovação do usuário (2026-11-15).
- **Requisitos:** [requirements.md](requirements.md) (aprovado com as 7 recomendações; IMV-001..IMV-083) · **Design:** [design.md](design.md) · **Contratos:** [contracts/ipc-v0.13.0-delta.md](contracts/ipc-v0.13.0-delta.md), [contracts/import-rules.md](contracts/import-rules.md) e `contracts/ipc.openrpc.json` (v0.13.0, criado na T01).
- **ADRs:** 0014, 0015, 0026, 0027, 0029, 0032 (o 0032 já está escrito e commitado).

## Dúvidas e achados do planejamento (nenhuma bloqueia)

1. **`domain.SeedKeyWindows` já existe** (`internal/domain/seed.go`). O design §4 propõe uma constante local `seedKeyWindows`; o plano usa a do domínio (só leitura, sem alterar `internal/domain`, IMV-082).
2. **Testes de contrato atuais quebram de propósito.** `contract_phase6_test.go` (`IMP-001/import.scan aceita fonte e caminho…` e `IMP-001/candidatos usam camelCase…`) compara as chaves JSON de `ImportScanParams{Source, Path}` e de um `ImportCandidate` mínimo com as propriedades do schema. Os campos novos são `omitempty`, então o test-writer preenche todos os campos nesses dois casos (T02). `contract_test.go` (`IPC-041/versão 0.12.0`, `contractPath`, `contractRelPath`) e o comentário do pacote em `protocol.go` passam a apontar para a fase 11.
3. **Correções do contrato v0.13.0 (aprovadas).** No JSON da v0.12.0, `ImportCandidate.launchType` fica na linha ~2561 (`enum ["exe","uri"]`) e `lastPlayed` na ~2631 (`oneOf` com `integer` mínimo 1, ou nulo). Só esses dois pontos mudam além do acréscimo. O `launchType` da linha ~2164 (jogo editado) e o da ~409 (EDT-010) **não** mudam.
4. **Comparação de `x-import` com as constantes do código.** Segue o padrão da 10.2: `importers.MaxScanDepth` e `MaxScanItems` são conferidos num teste de `internal/importers` e `maxScanResultBytes` (não exportada) num teste de `internal/agent`. Ambos leem o JSON. O teste de `internal/ipc` confere só a presença e os valores literais.
5. **`.m3u` (IMV-023).** O plano o junta ao grupo de mesmo (escopo, chave). É equivalente ao texto de `import-rules.md` §4.3, porque "mesma pasta" implica o mesmo escopo.
6. **`FirstLevelOnly` ligado** não abre subpastas e não gera o aviso de profundidade (só o de "nenhum jogo no primeiro nível", se for o caso).
7. **Texto antigo.** "Pasta: nenhum arquivo no primeiro nível" vira "Pasta: nenhum jogo no primeiro nível" (`import-rules.md` §5). Os testes antigos de `folder_test.go` e `ImportDialog.test.tsx` são reescritos.
8. **Wails.** `frontend/wailsjs/go/**` é regerado pelo `wails build` (T17). Conferir `git status`: se esses arquivos são versionados, entram no commit da T17.

## Premissas

- Uma tarefa por commit; test-writer primeiro, implementer depois. Ao fim de cada tarefa Go: `go build ./...`, `go vet ./...`, `CGO_ENABLED=0 go build ./...`, `go test ./...`; ao fim de cada tarefa de frontend: `npm test` e `tsc --noEmit`.
- Nada em `internal/tracker`, `cmd/fake-game`, `internal/store`, `internal/domain` (IMV-082). Nenhuma goroutine nova e nenhum handle do Windows.
- Os testes de `internal/importers` usam `t.TempDir()` com arquivos vazios e um `dirReader` falso/espião (sem `Open`) onde o design pede.

## T01 — Contrato completo `ipc.openrpc.json` v0.13.0
- Requisitos: IMV-070, IMV-082. **Responsável:** sessão principal (cópia mecânica de JSON grande, como a T01 da 10.2). O delta continua na pasta como histórico.
- Criar `docs/specs/phase-11-importacao-v2/contracts/ipc.openrpc.json` como cópia de `docs/specs/phase-10-2-imagens-ao-vivo/contracts/ipc.openrpc.json` (v0.12.0) com só:
  - `info.version = "0.13.0"`, `info.description` do delta §1, `info.x-import` (`maxScanDepth` 8, `maxScanItems` 20000, `maxScanResultBytes` 921600);
  - `ImportScanParams` ganha `platform` (string, `maxLength` 100), `titleFrom` (enum `file`/`folder`, default `file`) e `recursive` (boolean, default `true`), sem entrar em `required` e com `additionalProperties: false` mantido;
  - `ImportCandidate` ganha `file`, `extension`, `discs` (integer, mínimo 1) e `discFiles` (array de string), sem entrar em `required`;
  - `summary` de `import.scan` conforme o delta §4;
  - **duas correções aprovadas:** `ImportCandidate.launchType.enum` vira `["exe","uri","emulator"]`; `ImportCandidate.lastPlayed` vira `oneOf [{type: string, format: date-time}, {type: null}]`, no lugar de `integer` (mantém a nulabilidade atual).
- Em `design.md` §1 e §5 e no `ipc-v0.13.0-delta.md`, trocar "(tarefa)" pelo arquivo criado e registrar as duas correções como feitas. Dependência: nenhuma. Bloqueia T02.

## T02 — Campos novos em `internal/ipc` e testes de contrato na v0.13.0
- Requisitos: IMV-070, IMV-024, IMV-082. **implementer:** `internal/ipc/import.go` e `internal/ipc/protocol.go` (comentário do pacote aponta para o contrato da fase 11).
```go
const (
	ImportTitleFromFile   = "file"
	ImportTitleFromFolder = "folder"
)
type ImportScanParams struct {
	Source    string `json:"source"`
	Path      string `json:"path,omitempty"`
	Platform  string `json:"platform,omitempty"`
	TitleFrom string `json:"titleFrom,omitempty"`
	Recursive *bool  `json:"recursive,omitempty"` // nil = true
}
// ImportCandidate gains, all omitempty:
//   File string `json:"file,omitempty"`; Extension string `json:"extension,omitempty"`
//   Discs int `json:"discs,omitempty"`; DiscFiles []string `json:"discFiles,omitempty"`
```
- **test-writer:**
  - `contract_test.go`: `contractPath`/`contractRelPath` para a fase 11; `IPC-041/versão` vira `0.13.0`; `openRPC.Info` ganha `x-import`; reaproveitar os testes que usam `contractPath`.
  - `contract_phase6_test.go`: os dois casos de chaves preenchem todos os campos (achado 2).
  - Novo `contract_phase11_test.go` (`TestImportV2Contract`):
    - `IMV-070/ImportScanParams tem source, path, platform, titleFrom e recursive; os três novos opcionais`;
    - `IMV-070/titleFrom é enum file/folder com default file; recursive é boolean com default true; platform tem maxLength 100`;
    - `IMV-070/ImportCandidate tem file, extension, discs e discFiles; nenhum em required`;
    - `IMV-024/campos novos codificam com omitempty: candidato de Steam serializa byte a byte como na v0.12.0`;
    - `IMV-070/Recursive nil não aparece; ponteiro para false aparece como false`;
    - `IMV-070/x-import: maxScanDepth 8, maxScanItems 20000, maxScanResultBytes 921600`;
    - `IMV-070/ImportCandidate.launchType lista exe, uri e emulator; lastPlayed é string date-time ou nulo` (as duas correções);
    - `IMV-082/v0.13.0 é a v0.12.0 mais os campos novos e x-import, nada mais`: os dois JSON como `map[string]any`; sai `info.version`, `info.description`, `info.x-import`, as 3 propriedades de `ImportScanParams`, as 4 de `ImportCandidate` e o texto do `summary` de `import.scan`, e as **duas correções** (`launchType.enum` e `lastPlayed`) são normalizadas para o valor antigo antes do `reflect.DeepEqual`; o resto igual.
- Dependência: T01.

## T03 — Normalizador de título (`internal/importers/title.go`)
- Requisitos: IMV-001, IMV-002, IMV-003, IMV-004, IMV-005, IMV-006, IMV-008 (e `DiscNumber`/`titleKey` para IMV-021). **implementer:**
```go
func SplitExt(name string) (stem, ext string)            // ext in lower case without the dot [IMV-001]
func NormalizeTitle(stem string, fromFolder bool) string // pure; "" only when stem is blank [IMV-001..IMV-006, IMV-008]
func DiscNumber(stem string) int                         // N of the first disc tag, in () or []; 0 if none [IMV-021]
func titleKey(stem string) string                        // strings.ToLower(NormalizeTitle(stem, false)) [IMV-020, IMV-021]
```
Regexes e passos do design §2 e de `import-rules.md` §2 (laço de remoção de grupos com limite = comprimento do texto; ponto → espaço sobre `[]rune`; corte em `domain.MaxTitleRunes`).
- **test-writer:** `title_test.go` (tabela com os 34 vetores de §2 e os 5 de §2.1; cada linha prefixada pelo ID da regra que exercita):
  - `TestSplitExt`: `IMV-001/Mr. Driller.zip`, `IMV-001/Dr. Mario sem extensão`, `IMV-001/Game 2.0 sem extensão`, `IMV-001/extensão em maiúsculas vira minúscula`, `IMV-001/extensão com 6 caracteres não é extensão`, `IMV-001/só dígitos não é extensão`.
  - `TestNormalizeTitle`: `IMV-002/grupos múltiplos` (1, 9), `IMV-002/aninhados` (22), `IMV-002/sem par fica` (21), `IMV-002/tudo marcação mantém o nome` (19), `IMV-003/_ vira espaço` (8), `IMV-003/ponto só sem espaços` (7, 12), `IMV-003/ponto entre dígitos` (11, 12), `IMV-003/Dr. Mario e Super Mario Bros. 3 mantêm ponto` (6, 13, 26), `IMV-004/artigo no fim` (15, 16, 17), `IMV-004/antes de " - "` (2, 3), `IMV-004/antes de ': '`, `IMV-004/no meio da frase não move` (14, 27), `IMV-004/artigos estrangeiros`, `IMV-004/só o primeiro`, `IMV-005/espaços e tabulação` (23), `IMV-005/traço solto` (20), `IMV-005/250 runas com acento viram 200` (29), `IMV-005/nome vazio não vira título`, `IMV-006/prefixo de pasta` (30), `IMV-006/mesmo texto de arquivo não perde o prefixo` (31), `IMV-006/1942` (32), `IMV-006/2.5D` (33), `IMV-006/10. 4 Game` (34).
  - `TestNormalizeTitleIdempotent`: `IMV-008/vetores 1 a 29 são idempotentes` e `IMV-008/propriedade sobre conjunto de nomes com semente fixa` (math/rand com seed fixa, alfabeto com `()[]_.-, ` e letras).
  - `TestDiscNumber`: `IMV-021/(Disc 2), (Disk 2), (CD 2), (Disc 2 of 3), [Disc 2]`, `IMV-021/sem marcador é 0`, `IMV-021/N de 1 a 99; 100 não casa`.
  - `TestTitleKey`: `IMV-021/chave ignora marcador de disco e maiúsculas`.
- Dependência: nenhuma.

## T04 — Ordem natural (`internal/importers/natural.go`)
- Requisitos: IMV-010. **implementer:**
```go
// naturalLess compares relative paths segment by segment (separator \ or /), case-insensitively,
// with digit runs compared as numbers; ties fall back to the original runes [IMV-010].
func naturalLess(a, b string) bool
```
- **test-writer:** `natural_test.go` (`TestNaturalLess`): `IMV-010/Jogo 2 antes de Jogo 10`, `IMV-010/04. FF antes de 31. Spyro`, `IMV-010/sem diferenciar maiúsculas`, `IMV-010/pasta e arquivo no mesmo critério`, `IMV-010/zeros à esquerda: mais curto primeiro e determinístico`, `IMV-010/sequência de dígitos enorme não estoura`, `IMV-010/acentos comuns`, `IMV-010/irreflexiva e transitiva num conjunto`.
- Dependência: nenhuma.

## T05 — Tipos da leitura e tabelas de regras
- Requisitos: IMV-012, IMV-013, IMV-015, IMV-020, IMV-024. **implementer:**
  - `internal/importers/types.go`: `Candidate` ganha `File string`, `Extension string`, `Discs int`, `DiscFiles []string`; `ScanInput` ganha `Folder FolderOptions`.
  - `internal/importers/folder.go` (só os tipos, o resto na T08): `type TitleFrom string` (`TitleFromFile`, `TitleFromFolder`); `type FolderKind int` (`KindAny`, `KindExecutables`, `KindConsole`); `type FolderOptions struct{ FirstLevelOnly bool; TitleFrom TitleFrom; Kind FolderKind; MaxDepth, MaxItems int }`; `const MaxScanDepth = 8`, `MaxScanItems = 20000`; `type dirReader interface{ ReadDir(name string) ([]fs.DirEntry, error) }`; `osDirReader`.
  - `internal/importers/folder_rules.go` (novo; tabelas em `var` do pacote):
```go
func isGameExt(ext string, kind FolderKind) bool // ext lower case, no dot; lists of import-rules.md §1 [IMV-013]
func isExecutableExt(ext string) bool            // exe, bat, cmd [IMV-013, IMV-016]
func isNotAGame(stem string) bool                // unins*, *setup, vcredist*... [IMV-015]
func skipEntry(name string, typ fs.FileMode) bool // . and $ prefixes, named folders, symlink/irregular [IMV-012]
var companionExts map[string][]string            // cue, ccd, mds, gdi -> tracks [IMV-020]
```
- **test-writer:** `folder_rules_test.go`:
  - `TestIsGameExt`: `IMV-013/classe Windows: só exe, bat e cmd`, `IMV-013/classe console: uma extensão de cada linha da tabela`, `IMV-013/classe qualquer: as duas listas`, `IMV-013/.sub, .wav, .raw, .dll, .sav, .txt, .png fora das listas`, `IMV-013/extensão em maiúsculas` (a comparação é sobre `ext` já minúscula; o caso de maiúsculas é do T08).
  - `TestIsNotAGame`: um caso por padrão: `IMV-015/unins000`, `uninstall`, `setup`, `gamesetup`, `vcredist_x64`, `vc_redist`, `dxsetup`, `dotnet4`, `oalinst`, `UnityCrashHandler64`, `crashpad_handler`, `MyCrashReportTool`, `Redist`, `DirectX9`, `IMV-015/jogo.exe não é ignorado`, `IMV-015/Setup Hero (nome de jogo) é ignorado`: o padrão `setup*` é o da spec.
  - `TestSkipEntry`: `IMV-012/.git, $RECYCLE.BIN`, `IMV-012/System Volume Information, @eaDir, __MACOSX, node_modules sem diferenciar maiúsculas`, `IMV-012/ModeSymlink e ModeIrregular`, `IMV-012/pasta e arquivo comuns não são ignorados`.
  - `TestCompanionExts`: `IMV-020/tabela de contêineres e trilhas`.
- Dependência: nenhuma (T03 só para `titleKey` usado depois).

## T06 — Leitura recursiva (`internal/importers/folder_walk.go`)
- Requisitos: IMV-010, IMV-011, IMV-012, IMV-018, IMV-080. **implementer:**
```go
type fileEntry struct{ Dir, Name string } // Dir is relative to the chosen folder, "\" separators, "" at the root
func (e fileEntry) Rel() string           // Dir + "\" + Name
func (e fileEntry) Scope() string         // first segment of Dir, "" for root files [IMV-021]
func (e fileEntry) Stem() string          // via SplitExt
func (e fileEntry) Ext() string
type walkResult struct {
	Files      []fileEntry // every regular file seen, in natural visit order
	DepthCut   bool        // some folder below level MaxDepth was not opened [IMV-018]
	ItemsCut   bool        // stopped at MaxItems [IMV-018]
	Unreadable int         // subfolders whose ReadDir failed [IMV-012]
}
// walk visits root depth first; each directory's entries are sorted with naturalLess; entries skipped by skipEntry
// are neither read nor counted; folders and files count as items; ctx is checked on entering each folder.
// The root level is 0. FirstLevelOnly reads root files only (no depth warning). opts.MaxDepth/MaxItems 0 = defaults.
func walk(ctx context.Context, dirs dirReader, root string, opts FolderOptions) (walkResult, error)
```
- **test-writer:** `folder_walk_test.go` (`dirReader` falso em memória e árvore real em `t.TempDir()`):
  - `IMV-010/soltos e subpastas: os dois layouts do usuário`; `IMV-010/ordem natural: Jogo 2 antes de Jogo 10 e pasta/arquivo no mesmo critério`; `IMV-010/profundidade 8 é lida e 9 não`;
  - `IMV-011/FirstLevelOnly lê só os arquivos da raiz e a mesma árvore sem a opção lê tudo`;
  - `IMV-012/nomes ignorados e o que há dentro deles não são lidos nem contados`; `IMV-012/subpasta ilegível (ReadDir com erro injetado): segue nas demais e Unreadable conta`; `IMV-012/ModeSymlink e ModeIrregular por dirReader falso`; `IMV-012/link simbólico real` (`t.Skip` com motivo se o processo não puder criá-lo);
  - `IMV-018/DepthCut com MaxDepth injetado de 2`; `IMV-018/ItemsCut para na ordem natural do item MaxItems e devolve o que achou`; `IMV-018/cancelamento do ctx no meio devolve erro de contexto`; `IMV-018/a raiz com erro de ReadDir é erro`;
  - `IMV-080/dirReader espião só implementa ReadDir (nenhum Open) e a leitura completa passa`.
- Dependência: T04, T05.

## T07 — Acompanhantes, grupos de disco e `.m3u` (`internal/importers/folder_groups.go`)
- Requisitos: IMV-020, IMV-021, IMV-022, IMV-023, IMV-080. **implementer:**
```go
// dropCompanions removes the track files that sit in the same folder as a cue/ccd/mds/gdi of the same titleKey [IMV-020].
func dropCompanions(files []fileEntry) []fileEntry
// groupEntry is one candidate before titles: the main file, how many discs, and the disc files in disc order [IMV-021, IMV-024].
type groupEntry struct{ Main fileEntry; Discs int; DiscFiles []string }
// groupDiscs joins the files with DiscNumber >= 1 by (scope, titleKey), in natural path order, skipping a disc number
// already in the group (IMV-022); a .m3u of the same (scope, key) becomes the main file [IMV-023]. Result in natural order of Main.Rel().
func groupDiscs(primaries []fileEntry) []groupEntry
```
- **test-writer:** `folder_groups_test.go` (`TestDropCompanions`, `TestGroupDiscs`; fixtures são árvores A..E de `import-rules.md` §4.4):
  - `IMV-020/cue + bin: o bin some`; `IMV-020/cue com várias trilhas (Track 01, Track 02)`; `IMV-020/bin solto continua candidato`; `IMV-020/ccd com img e sub`; `IMV-020/mds com mdf`; `IMV-020/gdi com bin e raw`; `IMV-020/mesma chave em outra pasta não é acompanhante`;
  - `IMV-021/árvore A: Final Fantasy VIII e Resident Evil 2 viram um candidato cada com discs 2`; `IMV-021/discos soltos na raiz`; `IMV-021/escopos diferentes não se juntam`; `IMV-021/disco único é candidato comum com discs 1`; `IMV-021/menor número de disco é o principal e discFiles em ordem de disco`;
  - `IMV-022/árvore D: dois disco 1 mais um disco 2`; `IMV-022/cue e chd do mesmo disco`;
  - `IMV-023/árvore C: m3u + 2 cue é um candidato com extension m3u`; `IMV-023/m3u sozinho é candidato comum`;
  - `IMV-080/nenhuma função desta tarefa tem acesso a disco` (as funções recebem só `fileEntry`).
- Dependência: T03, T05, T06.

## T08 — `folderSource`: classificar, montar candidatos e avisos
- Requisitos: IMV-010, IMV-011, IMV-013, IMV-014, IMV-015, IMV-016, IMV-017, IMV-024, IMV-007, IMV-080. **implementer:** `internal/importers/folder.go` (reescrito; a validação da raiz com `os.Stat` e os avisos da fase 6 ficam).
```go
func Folder() Source                          // uses osDirReader
func newFolderSource(dirs dirReader) Source   // tests inject a spy / fake
type ignoredCounts struct{ ByExt map[string]int; Console, Exe int } // Console/Exe feed the "wrong platform" warnings [IMV-014]
// classify keeps files whose extension is in the list of kind and that pass isNotAGame; the rest is counted.
func classify(files []fileEntry, kind FolderKind) (kept []fileEntry, ign ignoredCounts)
// toCandidates builds title (IMV-017), launchType, launchTarget = Join(root, Rel), installDir = dir of the main file,
// file, extension, discs, discFiles; sorted by naturalLess(Main.Rel()).
func toCandidates(entries []groupEntry, root string, opts FolderOptions) []Candidate
// buildWarnings follows the order of import-rules.md §5: ignored by extension, wrong platform, depth, items, unreadable, none found.
func buildWarnings(ign ignoredCounts, w walkResult, found int, opts FolderOptions) []string
```
Fluxo: `walk` → `dropCompanions` → `classify` → `groupDiscs` → `toCandidates`. O título vem de `NormalizeTitle(stem, false)` ou, com `TitleFromFolder`, de `NormalizeTitle(<subpasta de primeiro nível>, true)` com queda para o título do arquivo (design §2). O agrupamento usa sempre `titleKey` do arquivo.
- **test-writer:** `folder_test.go` (reescrito; remove os casos antigos de "todo arquivo do primeiro nível"):
  - `IMV-010/layout ps1: soltos e um jogo por subpasta, em ordem natural`; `IMV-010/árvore B (n64): três candidatos com título limpo e file igual ao nome`;
  - `IMV-011/FirstLevelOnly liga e desliga na mesma árvore`;
  - `IMV-013/uma extensão de cada classe: Windows lê só .exe/.bat/.cmd, console só ROM e disco, qualquer lê as duas`; `IMV-013/extensão em maiúsculas (.ZIP, .EXE)`; `IMV-013/arquivo de outra extensão não vira candidato`;
  - `IMV-014/aviso com total e as 5 extensões mais frequentes, empate por ordem alfabética`; `IMV-014/árvore F: aviso exato com .exe ×1, .pak ×1, .txt ×1`; `IMV-014/nenhum candidato com ROMs numa leitura Windows: aviso de plataforma trocada`; `IMV-014/nenhum candidato com executáveis numa leitura de console: aviso de plataforma trocada`; `IMV-014/nenhum jogo (e no primeiro nível quando FirstLevelOnly)`;
  - `IMV-015/setup.exe ignorado na pasta e contado como .exe`;
  - `IMV-016/launchType exe para exe, bat, cmd e emulator para os demais`; `IMV-016/launchTarget e installDir de arquivo da raiz e de subpasta`;
  - `IMV-017/título pelo arquivo e pela pasta (04. Final Fantasy VIII e 07. Resident Evil 2 nas duas opções)`; `IMV-017/arquivo na raiz com titleFrom folder usa o nome do arquivo`; `IMV-017/pasta cujo título normalizado é vazio usa o arquivo`; `IMV-017/discos agrupam igual nas duas opções`;
  - `IMV-018/aviso de profundidade e de leitura interrompida com limites injetados`; `IMV-012/aviso de n subpastas ilegíveis`;
  - `IMV-024/campos file, extension, discs e discFiles; discFiles só com discs >= 2`;
  - `IMV-007/só o título muda: launchTarget e installDir são os do disco e o nome com marcações segue em file`;
  - `IMV-080/nenhum Open: spy de dirReader` e `IMV-080/o scan não grava nada na pasta (árvore igual antes e depois)`;
  - avisos da fase 6 inalterados: `IMV-010/pasta vazia, inexistente, arquivo no lugar de pasta e caminho vazio`.
- Dependência: T03, T04, T05, T06, T07.

## T09 — Fonte "Arquivo": título normalizado e campos novos
- Requisitos: IMV-032, IMV-007, IMV-024, IMV-001. **implementer:** `internal/importers/file.go`: `SplitExt` + `NormalizeTitle(stem, false)`; `File` = nome do arquivo, `Extension`, `Discs = 1`; título vazio continua o aviso "Arquivo: nome vazio"; `runnableExts` e `InstallDir` ficam; nenhuma regra de extensão, executável ignorado ou disco.
- **test-writer:** `file_test.go`: `IMV-032/Legend of Zelda, The - Majora's Mask (U) [!].z64 vira The Legend of Zelda - Majora's Mask`; `IMV-032/setup.exe é aceito (a lista de IMV-015 não vale)`; `IMV-032/arquivo sem extensão`; `IMV-032/qualquer tipo continua um candidato: exe/bat/cmd são exe e o resto emulator`; `IMV-024/file é o nome, extension minúscula, discs 1`; `IMV-007/launchTarget e installDir intactos`; `IMV-032/avisos de arquivo inexistente, pasta no lugar de arquivo e caminho vazio não mudam`; `IMV-007/Steam, Epic e LaunchBox: título do launcher sem normalização e sem os campos novos` (testes existentes de `steam_test.go`, `epic_test.go`, `launchbox_test.go` continuam verdes; o caso novo asserta `File == ""`, `Discs == 0`).
- Dependência: T03, T05.

## T10 — Agente: opções do scan, plataforma e "já existe"
- Requisitos: IMV-030, IMV-071, IMV-063, IMV-013, IMV-017, IMV-011, IMV-024, IMV-007, IMV-080. **implementer:** `internal/agent/imports.go`:
```go
type importStore interface {
	// ... os atuais ...
	ResolvePlatform(ctx context.Context, name string) (store.Platform, bool, error) // PLT-025 steps 1 and 2; carries SeedKey
}
// scanOptions derives the folder options and the platform id of the scan; no write [IMV-030].
func scanOptions(ctx context.Context, st importStore, p ipc.ImportScanParams) (importers.FolderOptions, int64, error)
func candidateExists(ctx context.Context, st importStore, c importers.Candidate, scanPlatformID int64) (bool, error)
```
`handleImportScan`: valida `titleFrom` (`""`, `file` ou `folder`) e `platform` (≤ 100 runas) com `-32602` antes de qualquer leitura; para `file`/`folder` resolve a plataforma por `ResolvePlatform` (classe: `domain.SeedKeyWindows` → `KindExecutables`; outra chave → `KindConsole`; sem chave ou não achada ou vazia → `KindAny`, id 0); `FirstLevelOnly = Recursive != nil && !*Recursive`; `candidateExists` usa `scanPlatformID` para candidato de arquivo/pasta sem plataforma; Steam/Epic/LaunchBox ignoram os parâmetros novos. `toIPCCandidate` copia `File`, `Extension`, `Discs`, `DiscFiles`.
- **test-writer:** `internal/agent/imports_test.go` (`fakeImportStore` ganha `ResolvePlatform`; os casos de store real usam `openImportRealStore` e `importPlatformByKey`):
  - `IMV-071/titleFrom diferente de file e folder: -32602 sem ler nada (fonte espiã sem chamada)`; `IMV-071/platform com 101 caracteres: -32602`; `IMV-071/titleFrom e recursive com Steam, Epic e LaunchBox são aceitos e ignorados`; `IMV-071/platform com Steam, Epic e LaunchBox é aceito e ignorado`; `IMV-071/parâmetros ausentes: título pelo arquivo, recursivo, sem plataforma`;
  - `IMV-030/Windows renomeada (chave windows) escolhe a classe de executáveis`; `IMV-030/plataforma de console do seed escolhe a classe de console`; `IMV-030/plataforma criada pelo usuário (sem chave) lê as duas listas`; `IMV-030/nome inexistente ou vazio: classe qualquer e só o alvo é conferido`; `IMV-030/título duplicado na plataforma certa é exists; em outra plataforma, não`; `IMV-030/o scan não cria plataforma`;
  - `IMV-063/plataforma renomeada é resolvida sem diferenciar maiúsculas`;
  - `IMV-011/recursive=false chega à fonte como FirstLevelOnly` e `IMV-017/titleFrom=folder chega à fonte` (fonte espiã que guarda o `ScanInput`);
  - `IMV-024/campos novos aparecem no resultado do scan de pasta e não aparecem nos de Steam, Epic e LaunchBox`;
  - `IMV-007/título de Steam, Epic e LaunchBox sai sem normalização`;
  - `IMV-080/scan com store real não cria jogo, sessão nem regra` (conta linhas de `games` e de sessões antes e depois).
- Dependência: T02, T05 (T08 só para os casos com fonte real; os com fonte espiã não dependem).

## T11 — Agente: corte por tamanho, apply com título editado e invariantes
- Requisitos: IMV-019, IMV-072, IMV-031, IMV-081, IMV-070, IMV-016. **implementer:** `internal/agent/imports.go`:
```go
const maxScanResultBytes = 900 << 10 // x-import.maxScanResultBytes [IMV-019]
// capScanResult keeps the first candidates whose JSON (len(Marshal)+1 each) fits maxScanResultBytes and returns a warning
// with the total and the shown count when it cuts (import-rules.md §6); only for the file and folder sources.
func capScanResult(cands []ipc.ImportCandidate) (kept []ipc.ImportCandidate, warning string)
```
`handleImportApply`/`toStoreGame` **não mudam de lógica**: já aparam e validam o título e ignoram os campos novos (os testes provam). A regra `install_dir` de RUL-001 já usa `c.InstallDir`; sem código novo.
- **test-writer:** `imports_test.go` (continuação) e novo `imports_cap_test.go`:
  - `TestCapScanResult`: `IMV-019/muitos candidatos com caminhos longos: o resultado fica em até 921600 bytes`; `IMV-019/aviso com o total encontrado e os mostrados`; `IMV-019/nenhum candidato é cortado ao meio e a ordem se mantém`; `IMV-019/dentro do limite: sem corte e sem aviso`; `IMV-019/Steam, Epic e LaunchBox não são cortados`;
  - `IMV-070/maxScanResultBytes é igual a x-import.maxScanResultBytes do contrato v0.13.0` (lê `../../docs/specs/phase-11-importacao-v2/contracts/ipc.openrpc.json`);
  - `IMV-070/maxScanDepth e maxScanItems do contrato são os de importers.MaxScanDepth/MaxScanItems` (no teste de `internal/importers`, `contract_import_test.go`, mesma leitura do JSON);
  - `TestImportApplyV2`: `IMV-072/título editado é gravado aparado`; `IMV-072/título vazio recusa só aquele candidato como failed`; `IMV-072/título com mais de 200 caracteres recusa como failed`; `IMV-072/file, extension, discs e discFiles são aceitos e ignorados`; `IMV-072/o resumo usa o título gravado`;
  - `IMV-031/scan, apply, scan: tudo exists, inclusive grupos de disco, e o apply recusa com duplicate`; `IMV-016/a regra de rastreio install_dir gravada aponta para a pasta do arquivo principal`;
  - `IMV-081/importar não cria sessão rastreada nem chama o tracker` (store real; mesma base de IMP-009).
- Dependência: T02, T08, T10.

## T12 — Binding do desktop (`cmd/arcadia/imports.go`)
- Requisitos: IMV-073, IMV-024, IMV-063. **implementer:**
```go
type ImportScanOptions struct {
	Platform  string `json:"platform"`
	TitleFrom string `json:"titleFrom"`
	Recursive *bool  `json:"recursive"`
}
// ImportCandidateView gains File string `json:"file"`, Extension string `json:"extension"`, Discs int `json:"discs"`,
// DiscFiles []string `json:"discFiles"`, copied both ways by importCandidateView and importCandidateWire.
func (a *App) ImportScan(source, path string, opts ImportScanOptions) (ImportScanView, error)
```
`ImportScan` monta `ipc.ImportScanParams` com as opções; só `import.scan` e `import.apply` são chamados; erros seguem os de UI-008.
- **test-writer:** novo `cmd/arcadia/imports_test.go` (usa `gamePinger`, `singleCall` e `newBindingApp` de `games_test.go`): `IMV-073/ImportScan envia platform, titleFrom e recursive nos parâmetros de import.scan`; `IMV-073/recursive ausente (nil) não vai no JSON; false vai como false`; `IMV-073/ImportCandidateView recebe file, extension, discs e discFiles do resultado`; `IMV-073/ImportApply devolve os quatro campos ao agente`; `IMV-073/só import.scan e import.apply são chamados`; `IMV-073/erro do agente sai com a mensagem de UI-008`; `IMV-073/timeout de importTimeout`.
- Dependência: T02.

## T13 — `lib/imports.ts`: tipos, `scanImport` e helpers puros
- Requisitos: IMV-041, IMV-045, IMV-046, IMV-044, IMV-050, IMV-061, IMV-063, IMV-024. **implementer:** `frontend/src/lib/imports.ts` (sem importar componentes):
```ts
export type ImportScanOptions = { platform: string; titleFrom: 'file' | 'folder'; recursive: boolean }
// ImportCandidate gains file?, extension?, discs?, discFiles? [IMV-024]
export function scanImport(source: ImportSource, path: string, opts: ImportScanOptions): Promise<ImportScanResult>
export function isTitleEditable(source: ImportSource): boolean                    // file and folder only [IMV-044]
export function titleProblem(title: string): 'empty' | 'tooLong' | null            // trimmed, runes > 200 [IMV-045]
export function repeatedFlags(rows: { title: string; exists: boolean }[]): boolean[] // 2nd+ equal title among non-exists [IMV-046]
export function defaultPlatformName(platforms: Pick<PlatformView, 'name' | 'seedKey'>[]): string | null // [IMV-061]
export type ReviewRow = { candidate: ImportCandidate; title: string; checked: boolean }
export function buildReviewRows(candidates: ImportCandidate[]): ReviewRow[]       // checked = !exists && !repeated [IMV-041]
export function selectionCounts(rows: ReviewRow[]): { checked: number; selectable: number; existing: number } // [IMV-047]
export function markImported(rows: ReviewRow[], sent: ReviewRow[], applied: { title: string }[]): ReviewRow[] // [IMV-050]
```
`buildReviewRows`, `selectionCounts` e `markImported` são auxiliares puros acrescentados pelo planner para o `ImportDialog` ficar fino (sem comportamento novo: derivam de IMV-041, IMV-047, IMV-050). `applyImportInLots` não muda.
- **test-writer:** novo `frontend/src/lib/imports.test.ts` (o `imports.lots.test.ts` e o `imports.folder.test.ts` seguem como estão):
  - `IMV-063/scanImport repassa platform, titleFrom e recursive ao binding`; `IMV-063/binding ausente falha como chamada com erro`;
  - `IMV-044/isTitleEditable só para file e folder`;
  - `IMV-045/titleProblem: vazio, só espaços, 200 runas ok, 201 runas, acentos contam como uma runa`;
  - `IMV-046/repeatedFlags: segunda em diante, sem diferenciar maiúsculas, aparado, ignora exists`;
  - `IMV-061/defaultPlatformName: chave windows com nome trocado, windows ausente → primeira, lista vazia → null`;
  - `IMV-041/buildReviewRows: tudo marcado menos exists e repetidas`;
  - `IMV-047/selectionCounts com e sem existentes`;
  - `IMV-050/markImported: títulos aplicados viram exists, cada título do resultado consome uma linha, enviadas desmarcam, não enviadas continuam marcadas`.
- Dependência: nenhuma (usa `PlatformView` de `lib/library.ts`). Paralelo ao Go.

## T14 — `ImportReviewTable` (tabela de revisão)
- Requisitos: IMV-040, IMV-042, IMV-043, IMV-044, IMV-045, IMV-046, IMV-051, IMV-052. **implementer:** novo `frontend/src/components/ImportReviewTable.tsx`.
```ts
export type ImportReviewTableProps = {
  rows: ReviewRow[]; source: ImportSource; busy: boolean; repeated: boolean[]
  onToggle: (i: number, on: boolean) => void; onToggleAll: (on: boolean) => void; onTitle: (i: number, value: string) => void
}
export function ImportReviewTable(props: ImportReviewTableProps): JSX.Element
```
`<table>` com `<th scope="col">` (caixa, Título, Arquivo, Extensão, Disco); cabeçalho `sticky`; linha em `React.memo` com callbacks estáveis; `indeterminate` por `ref`; `aria-invalid` + mensagem por `aria-describedby`; só tokens `--arc-*` (sem token novo).
- **test-writer:** `ImportReviewTable.test.tsx`:
  - `IMV-040/colunas e linhas na ordem recebida; Steam mostra — em Arquivo, Extensão e Disco`; `IMV-040/discs 1 mostra — e discs 2 mostra "2 discos"; title do Arquivo é o launchTarget e o do Disco lista discFiles`; `IMV-040/selo já existe`;
  - `IMV-042/Selecionar todos: três estados (vazio, parcial indeterminate, tudo)`; `IMV-042/marcar tudo inclui as repetidas`; `IMV-042/desmarcar quando tudo marcado`; `IMV-042/desativado sem linhas marcáveis e em busy`;
  - `IMV-043/linha exists: caixa desmarcada e desativada, título sem campo, fora do Selecionar todos`;
  - `IMV-044/linha de arquivo/pasta: campo de título editável aceita colar e acentos; Steam, Epic e LaunchBox: texto sem campo`;
  - `IMV-045/título vazio e 201 caracteres: aria-invalid e mensagem "Título vazio"/"Título com mais de 200 caracteres" ligada ao campo`; `IMV-045/200 caracteres e espaços nas pontas são válidos`; `IMV-045/linha desmarcada inválida não marca o campo` (o bloqueio do botão é do diálogo, T15);
  - `IMV-046/selo "título repetido" aparece e some com a edição; a marcação não muda sozinha`;
  - `IMV-051/estrutura: contêiner com overflow-y e cabeçalho sticky`;
  - `IMV-052/papéis table, columnheader, nomes "Selecionar <título>" e "Título de <arquivo>"`; `IMV-052/Tab percorre caixa e campo de cada linha; Espaço alterna a caixa`; `IMV-052/digitar num título não re-renderiza as outras linhas` (contador de renders por `React.Profiler` em 3 linhas).
- Dependência: T13.

## T15 — `ImportDialog`: formulário, estado das linhas, plataforma e lotes
- Requisitos: IMV-041, IMV-045, IMV-046, IMV-047, IMV-048, IMV-049, IMV-050, IMV-051, IMV-060, IMV-061, IMV-062, IMV-063, IMV-017, IMV-011, IMV-052. **implementer:** `frontend/src/components/ImportDialog.tsx` (reescrito):
  - `rows: ReviewRow[] | null`, `platform: string | null`, plataformas de `useCatalog()`; select "Plataforma do arquivo" **antes** de "Buscar", só para `file`/`folder`; mensagem "Nenhuma plataforma. Crie uma em Plataformas." e botões desativados quando não há plataforma;
  - só `folder`: select "Título a partir de" ("Nome do arquivo" | "Nome da pasta do jogo") e caixa "Ler subpastas" (marcada);
  - `resetList()` em todo `onChange` de fonte, caminho (depois do seletor), plataforma, título-de e subpastas; `scan()` monta `buildReviewRows`;
  - contador `"{k} de {n} jogos selecionados"` + `" ({j} já existem)"`; "Importar selecionados" desativado com `busy`, `k = 0`, linha marcada inválida ou `fileLike && platform === null`;
  - `apply()`: linhas marcadas, na ordem da tabela, com `title.trim()` e `platform` escolhida nas fontes `file`/`folder`; `applyImportInLots` inalterado; depois `markImported`; `onImported` depois de gravar ao menos um jogo;
  - modal `size="large"` e corpo rolável inalterados; Esc no campo de título fecha salvo em `busy` (UIX-039).
- **test-writer:** `ImportDialog.test.tsx` (reescrito: saem os casos de IMP-010 que esperam "nada marcado" e campo de texto de plataforma; `modals.test.tsx` continua verde):
  - `IMV-060/Plataforma do arquivo é um select com as plataformas em ordem de nome, antes de Buscar; sem campo de texto; ausente em Steam, Epic e LaunchBox`;
  - `IMV-061/vem a plataforma de chave windows mesmo renomeada`; `IMV-061/windows excluída: vem a primeira`; `IMV-061/a escolha se mantém ao trocar de fonte na mesma abertura`;
  - `IMV-062/sem plataformas: select desativado com a mensagem e Buscar e Importar desativados`;
  - `IMV-063/o nome da plataforma vai em ImportScan e em cada candidato dos lotes`;
  - `IMV-017/Título a partir de só na fonte Pasta e seu valor vai ao ImportScan`; `IMV-011/Ler subpastas marcada por padrão e o valor vai como recursive`;
  - `IMV-041/15 candidatos: 2 existentes e 1 repetido → 12 marcados; Buscar de novo repõe a marcação padrão`;
  - `IMV-045/título vazio ou com 201 caracteres numa linha marcada desativa Importar selecionados; linha desmarcada não bloqueia; título é enviado aparado`;
  - `IMV-046/linha repetida marcada à mão é enviada ao ImportApply`;
  - `IMV-047/contador com e sem já existem; acompanha cada marcação e cada Buscar`;
  - `IMV-048/60 linhas → lotes 25/25/10 com títulos editados; "Importando k de N…"; falha no 2º lote interrompe e mostra o aviso; botão e Esc desativados durante a operação`;
  - `IMV-049/editar um título e mudar a plataforma descarta a lista; idem fonte, pasta, título-de e subpastas; Buscar traz lista nova com marcação padrão`;
  - `IMV-050/Importados: N, recusados com o motivo, linhas gravadas viram já existe e bloqueadas, não enviadas por falha de lote continuam marcadas`;
  - `IMV-051/data-size large, título e botões fixos e só o corpo rolável`;
  - `IMV-052/Esc dentro do campo de título fecha a janela; durante a operação não`.
- Dependência: T13, T14, T12 (o contrato do binding; o frontend usa tipos locais, então não há bloqueio de build).

## T16 — Invariantes de UI e varredura de tema
- Requisitos: IMV-081, IMV-051. **test-writer:** `frontend/src/App.invariants.test.tsx` ganha `IMV-081/ler pasta, marcar, editar título, escolher plataforma e importar em lotes chamam só ImportScan, ImportApply, PickFile, PickFolder e a leitura do catálogo; nenhum LaunchGame, LaunchGameUntracked, ToggleSession, Pause nem Resume`; `theme-scan.test.ts` ganha `IMV-051/ImportReviewTable.tsx e ImportDialog.tsx sem cores literais` (se a varredura já cobre todos os `.tsx` por glob, só o caso nomeado). Sem código novo (**implementer:** nenhum, a menos que os testes revelem cor literal).
- Dependência: T15.

## T17 — Verificação, rastreio, documentação e revisão do diff
- Requisitos: IMV-070, IMV-082. **Responsável:** sessão principal (e o reviewer).
  1. `go build ./...`, `go vet ./...`, `CGO_ENABLED=0 go build ./...`, `go test ./...` (com `-race` em `internal/importers` e `internal/agent` quando o ambiente permitir).
  2. `go test ./internal/ipc/... ./internal/importers/... ./internal/agent/... ./cmd/arcadia/... -v` conferindo os casos IMV-070, IMV-082 e IMV-019.
  3. Frontend: `npm test`, `npx tsc --noEmit`, `npm run build` (em `frontend/`); depois `cd cmd/arcadia && wails build` para regerar `frontend/wailsjs/go/**` (achado 8).
  4. `powershell -NoProfile -ExecutionPolicy Bypass -File scripts/spec-trace.ps1 -Spec docs/specs/phase-11-importacao-v2`, e também nas fases 6, 9, 10 e 10.1 (requisitos anotados "alterado em 2026-11-15").
  5. `git diff --stat` da fase só com `internal/importers/*`, `internal/agent/imports.go`, `internal/ipc/{import,protocol}.go`, `cmd/arcadia/imports.go`, `frontend/src/{lib/imports.ts,components/Import*}`, testes e docs; nenhum arquivo em `internal/tracker`, `cmd/fake-game`, `internal/store`, `internal/domain`; contratos anteriores (`phase-9`, `phase-10-2`) intactos.
  6. Revisão de concorrência e recursos: nenhum `go func` novo; `walk` confere `ctx` a cada pasta; nenhum `Open` na leitura (spy do T06/T08).
  7. Documentação: `docs/ARCHITECTURE.md` e `CLAUDE.md` já apontam para a fase 11; ao concluir, marcar a fase 11 como concluída em `docs/ROADMAP.md` e ajustar o que a implementação mudou.
- Dependência: T01..T16.

## T18 — Conferência manual ponta a ponta (IMV-083)
- Requisitos: IMV-083 (`· verificação: manual`). **Responsável:** sessão principal com o usuário. Pré-requisito: T17 verde e `wails build` gerado.
- **Roteiro:**
  1. Importar jogos → "Pasta" → plataforma Sony Playstation → ler `E:\Games\emulators roms\ps1` com subpastas: títulos limpos, sem o prefixo `NN. `; `Final Fantasy VIII` e `Resident Evil 2` uma vez cada, com "2 discos"; nenhum `.bin`; avisos coerentes.
  2. Editar um título, desmarcar um jogo, importar: os jogos aparecem na biblioteca, com o título editado; "Importados: N" correto.
  3. Ler a mesma pasta de novo: tudo "já existe" e desmarcado.
  4. Trocar para "Nome da pasta do jogo": a lista some (IMV-049); Buscar mostra os títulos das pastas.
  5. Repetir com `n64` (jogos soltos, `.zip`) e com uma pasta de jogos de PC na plataforma Windows (desinstaladores ignorados e contados no aviso).
  6. Janela em 1024 × 640 e 1920 × 1080 (IMV-051): cabeçalho fixo, rolagem só no corpo, mil linhas utilizáveis se houver pasta grande; Tab, Espaço e Esc (IMV-052).
  7. Conferir `agent.log` sem `Error` do import e que nenhum jogo foi lançado nem sessão criada (IMV-081).
- Dependência: T17.

## Ordem

- **Bloco A (paralelo):** T01 (sessão principal), T03, T04, T05, T13 (frontend, sem dependência de Go).
- **Bloco B (Go):** T02 (após T01) · T06 (após T04, T05) → T07 (após T03, T05, T06) → T08 (após T03..T07) · T09 (após T03, T05) → T10 (após T02, T05) → T11 (após T02, T08, T10) · T12 (após T02).
- **Bloco C (frontend):** T14 (após T13) → T15 (após T13, T14) → T16 (após T15).
- **Depois:** T17 → T18.
- **Arquivos compartilhados (não paralelos):** `folder.go` e `folder_test.go`: T05 (só tipos) e depois T08, nessa ordem; `types.go`: só T05; `file.go` e `file_test.go`: só T09; `agent/imports.go` e `imports_test.go`: T10 e depois T11; `ipc/import.go`, `protocol.go`, `contract_test.go`, `contract_phase6_test.go`: só T02; `lib/imports.ts`: só T13; `ImportDialog.tsx` e `ImportDialog.test.tsx`: só T15; `App.invariants.test.tsx` e `theme-scan.test.ts`: só T16.

## Riscos de concorrência e recursos

- **Goroutines:** nenhuma nova. A leitura roda na goroutine do handler do IPC e respeita `ctx` (cancelamento da conexão; timeout de 60 s do desktop). No frontend, `applyImportInLots` segue sequencial e o `mounted` ref evita estado depois de desmontar.
- **Handles do Windows:** nenhum aberto por esta fase. `os.ReadDir` abre e fecha o diretório internamente; `dirReader` não tem `Open`, e o spy dos testes prova que nenhum arquivo é aberto (IMV-080).
- **Locks e shutdown:** nenhum lock novo; nada muda em `Drain` → `server.Close()` → `stopQueue()` → `wg.Wait()`.
- **Memória:** até 20.000 `fileEntry` e 900 KiB de resposta; a tabela renderiza todas as linhas (sem virtualização) com `React.memo` e callbacks estáveis.
- **Disco de rede lento:** pode estourar os 60 s do desktop (mensagem de UI-008). Aceito, já registrado no design §9 (risco 4).
- **Teto de 1 MiB também vale para `import.apply` e para o LaunchBox:** só a pasta ganha corte; o lote de 25 do apply cabe com folga (design §9, risco 3).

## Rastreabilidade

| Requisito | Tarefas |
|---|---|
| IMV-001 | T03, T09 |
| IMV-002 | T03 |
| IMV-003 | T03 |
| IMV-004 | T03 |
| IMV-005 | T03 |
| IMV-006 | T03 |
| IMV-007 | T08, T09, T10 |
| IMV-008 | T03 |
| IMV-010 | T04, T06, T08 |
| IMV-011 | T06, T08, T10, T15 |
| IMV-012 | T05, T06, T08 |
| IMV-013 | T05, T08, T10 |
| IMV-014 | T08 |
| IMV-015 | T05, T08 |
| IMV-016 | T08, T11 |
| IMV-017 | T08, T10, T15 |
| IMV-018 | T06, T08 |
| IMV-019 | T11 |
| IMV-020 | T05, T07 |
| IMV-021 | T03, T07 |
| IMV-022 | T07 |
| IMV-023 | T07 |
| IMV-024 | T02, T05, T08, T09, T10, T12, T13 |
| IMV-030 | T10 |
| IMV-031 | T11 |
| IMV-032 | T09 |
| IMV-040 | T14 |
| IMV-041 | T13, T15 |
| IMV-042 | T14 |
| IMV-043 | T14 |
| IMV-044 | T13, T14 |
| IMV-045 | T13, T14, T15 |
| IMV-046 | T13, T14, T15 |
| IMV-047 | T13, T15 |
| IMV-048 | T15 |
| IMV-049 | T15 |
| IMV-050 | T13, T15 |
| IMV-051 | T14, T15, T16 |
| IMV-052 | T14, T15 |
| IMV-060 | T15 |
| IMV-061 | T13, T15 |
| IMV-062 | T15 |
| IMV-063 | T10, T12, T13, T15 |
| IMV-070 | T01, T02, T11, T17 |
| IMV-071 | T10 |
| IMV-072 | T11 |
| IMV-073 | T12 |
| IMV-080 | T06, T07, T08, T10 |
| IMV-081 | T11, T16 |
| IMV-082 | T01, T02, T17 |
| IMV-083 | T18 (manual) |

Os 52 requisitos têm tarefa. Nenhuma lacuna da spec.

## Decisões novas que merecem ADR

Nenhuma além do **0032** (já escrito e commitado).
