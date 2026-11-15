# Contrato IPC v0.13.0 — delta sobre a v0.12.0

- **Base:** [phase-10-2-imagens-ao-vivo/contracts/ipc.openrpc.json](../../phase-10-2-imagens-ao-vivo/contracts/ipc.openrpc.json)
  (v0.12.0), que continua sendo o contrato completo até a tarefa abaixo.
- **Requisitos:** IMV-024, IMV-030, IMV-070..IMV-073 ([requirements.md](../requirements.md)).
- **Natureza:** só acréscimo. Nenhum método, parâmetro obrigatório, resultado, esquema existente ou erro da v0.12.0 muda
  (a pergunta 7 dos requisitos propõe corrigir duas divergências antigas à parte; sem aprovação dela, ficam como estão).
- **Tarefa (planner):** gravar o contrato completo `docs/specs/phase-11-importacao-v2/contracts/ipc.openrpc.json` v0.13.0 =
  v0.12.0 + os pontos abaixo, e apontar o teste de contrato de `internal/ipc` (`contract_test.go`) para ele; o teste que compara
  com a versão anterior (`contract_phase10_2_test.go`) ganha um irmão da fase 11 que confere "só acréscimo".

## 1. `info`

```json
"version": "0.13.0",
"description": "... Supersedes docs/specs/phase-10-2-imagens-ao-vivo/contracts/ipc.openrpc.json (v0.12.0). v0.13.0 (phase 11, import v2): import.scan takes the optional params platform, titleFrom and recursive [IMV-030, IMV-017, IMV-011]; ImportCandidate gains the optional fields file, extension, discs and discFiles [IMV-024]; new x-import block. import.apply and everything else are unchanged from v0.12.0."
```

Novo bloco em `info` (constantes do agente, fora do fio, como `x-media` e `x-tracking`):

```json
"x-import": {
  "description": "Agent constants of the folder import (not part of the wire) [IMV-018, IMV-019].",
  "maxScanDepth": 8,
  "maxScanItems": 20000,
  "maxScanResultBytes": 921600
}
```

`x-limits.maxMessageBytes` continua 1048576; `maxScanResultBytes` deixa 127 KiB de folga para os avisos e o envelope (IMV-019).

## 2. `components.schemas.ImportScanParams`

Acrescenta três propriedades (a v0.12.0 tem `source` e `path`; `additionalProperties` continua `false`):

```json
"platform": {
  "type": "string",
  "maxLength": 100,
  "description": "Platform name chosen for the file and folder sources. Resolved without creating anything (PLT-025 steps 1 and 2): its seed key picks the extensions read (windows = executables, other seed key = console games, none or not found = both) and the exists mark also checks title + platform [IMV-013, IMV-030]. Ignored by steam, epic and launchbox."
},
"titleFrom": {
  "type": "string",
  "enum": ["file", "folder"],
  "default": "file",
  "description": "Folder source only: the proposed title comes from the file name (default) or from the first-level subfolder that holds the file [IMV-017]. Ignored by the other sources."
},
"recursive": {
  "type": "boolean",
  "default": true,
  "description": "Folder source only: read the subfolders (up to x-import.maxScanDepth). false reads the first level only [IMV-010, IMV-011]. Absent means true. Ignored by the other sources."
}
```

Erros: `-32602` quando `titleFrom` não é `file` nem `folder` ou `platform` passa de 100 caracteres, sem ler nada (IMV-071). Sem
código novo.

## 3. `components.schemas.ImportCandidate`

Acrescenta quatro propriedades opcionais (nenhuma entra em `required`; o objeto continua com `additionalProperties: false`):

```json
"file": {
  "type": "string",
  "maxLength": 2000,
  "description": "File and folder sources: path of the main file relative to the chosen folder, with backslashes (the file name for the file source) [IMV-024]."
},
"extension": {
  "type": "string",
  "maxLength": 16,
  "description": "Lowercase extension of the main file without the dot, \"\" when it has none [IMV-024]."
},
"discs": {
  "type": "integer",
  "minimum": 1,
  "description": "Number of discs of the game (1 when single) [IMV-021, IMV-024]."
},
"discFiles": {
  "type": "array",
  "items": { "type": "string", "maxLength": 2000 },
  "description": "Relative paths of the disc files in disc order; present only when discs >= 2 [IMV-021, IMV-024]."
}
```

`import.scan` preenche os quatro para as fontes `file` e `folder` (`discFiles` só com `discs >= 2`) e nenhum para `steam`,
`epic` e `launchbox`. `import.apply` aceita e **ignora** os quatro (IMV-072); o `title` recebido é gravado depois de validado.

Exemplo de candidato de pasta (resultado de `import.scan`, uma linha NDJSON compactada):

```json
{"source":"folder","title":"Final Fantasy VIII","platform":"","launchType":"emulator","launchTarget":"E:\\Games\\emulators roms\\ps1\\04. Final Fantasy VIII (USA)\\Final Fantasy VIII (USA) (Disc 1)\\Final Fantasy VIII (USA) (Disc 1).cue","installDir":"E:\\Games\\emulators roms\\ps1\\04. Final Fantasy VIII (USA)\\Final Fantasy VIII (USA) (Disc 1)","developers":[],"publishers":[],"genres":[],"year":null,"description":"","playModes":[],"playedSeconds":null,"lastPlayed":null,"exists":false,"file":"04. Final Fantasy VIII (USA)\\Final Fantasy VIII (USA) (Disc 1)\\Final Fantasy VIII (USA) (Disc 1).cue","extension":"cue","discs":2,"discFiles":["04. Final Fantasy VIII (USA)\\Final Fantasy VIII (USA) (Disc 1)\\Final Fantasy VIII (USA) (Disc 1).cue","04. Final Fantasy VIII (USA)\\Final Fantasy VIII (USA) (Disc 2)\\Final Fantasy VIII (USA) (Disc 2).cue"]}
```

`platform` do candidato de pasta continua vazio no scan (a UI o preenche na confirmação, IMV-063); o parâmetro `platform` do
scan só decide extensões e "já existe".

## 4. `methods`

`import.scan`: o `summary` acrescenta "[IMV-010..IMV-024, IMV-030]. For the folder source it reads subfolders, proposes
normalized titles, joins the discs of a game into one candidate and returns at most x-import.maxScanResultBytes of candidates,
with a warning when it cuts the list [IMV-019]". Mesmos `params` (um objeto `ImportScanParams`), mesmo `result`, mesmos erros.
`import.apply`: sem mudança (só a descrição do esquema do candidato cresce).

## 5. Go

| Contrato | Go |
|---|---|
| `ImportScanParams.platform` | `Platform string \`json:"platform,omitempty"\`` |
| `ImportScanParams.titleFrom` | `TitleFrom string \`json:"titleFrom,omitempty"\`` (constantes `ImportTitleFromFile = "file"`, `ImportTitleFromFolder = "folder"`) |
| `ImportScanParams.recursive` | `Recursive *bool \`json:"recursive,omitempty"\`` (`nil` = `true`; ponteiro para distinguir ausente de `false`) |
| `ImportCandidate.file` | `File string \`json:"file,omitempty"\`` |
| `ImportCandidate.extension` | `Extension string \`json:"extension,omitempty"\`` |
| `ImportCandidate.discs` | `Discs int \`json:"discs,omitempty"\`` |
| `ImportCandidate.discFiles` | `DiscFiles []string \`json:"discFiles,omitempty"\`` |
| `x-import` | constantes do agente (`maxScanResultBytes`) e de `internal/importers` (`MaxScanDepth`, `MaxScanItems`); o teste de contrato confere as três contra o JSON |

Os campos novos de `ImportCandidate` usam `omitempty` (diferente dos antigos, que sempre aparecem): o JSON de Steam, Epic e
LaunchBox fica byte a byte igual ao da v0.12.0. `extension = ""` num candidato de pasta sem extensão some do JSON; a UI trata
ausente como `""`.

## 6. Binding do Wails (desktop → frontend, fora do contrato IPC) [IMV-073]

| Binding | Antes | Depois |
|---|---|---|
| `ImportScan` | `ImportScan(source, path string)` | `ImportScan(source, path string, opts ImportScanOptions)` com `ImportScanOptions{ Platform string; TitleFrom string; Recursive *bool }` (JSON `platform`, `titleFrom`, `recursive`). `Recursive` é ponteiro, como no IPC: ausente vale `true` (um `bool` simples tornaria "ausente" em `false`). O frontend sempre envia o valor da caixa "Ler subpastas". |
| `ImportCandidateView` | os 15 campos da v0.7.0 | + `File string`, `Extension string`, `Discs int`, `DiscFiles []string` (JSON `file`, `extension`, `discs`, `discFiles`), nos dois sentidos |
| `ImportApply`, `PickFile`, `PickFolder` | — | sem mudança |

Os arquivos gerados em `frontend/wailsjs/go/` são regerados pelo build do Wails; nenhum teste do frontend depende deles (o
`lib/imports.ts` acessa `window.go` com tipos locais).
