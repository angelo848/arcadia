# Regras de leitura de pasta e de título — fase 11

- **Requisitos:** IMV-001..IMV-008, IMV-010..IMV-024, IMV-030, IMV-032 ([requirements.md](../requirements.md)).
- **Natureza:** tabelas e vetores de teste que o test-writer usa como fonte. Se uma linha daqui e o requisito divergirem,
  vale o requisito e a divergência é bug desta spec.
- **Onde vira código:** `internal/importers` (`title.go`, `folder.go`, `folder_groups.go`); as tabelas ficam em `var` do
  pacote, nunca espalhadas em `if`.

## 1. Extensões lidas (IMV-013)

A comparação não diferencia maiúsculas. A classe vem da plataforma do scan: chave `windows` → **executáveis**; outra chave do
seed → **jogos de console**; sem chave, não encontrada ou sem plataforma → **qualquer** (as duas listas).

| Lista | Extensões | `launchType` |
|---|---|---|
| Executáveis | `.exe` `.bat` `.cmd` | `exe` |
| Nintendo | `.nes` `.unf` `.unif` `.fds` `.sfc` `.smc` `.fig` `.swc` `.gb` `.gbc` `.gba` `.n64` `.z64` `.v64` `.ndd` `.nds` `.dsi` `.3ds` `.cia` `.cci` `.cxi` `.3dsx` `.gcm` `.gcz` `.rvz` `.wbfs` `.wad` `.wia` `.nsp` `.xci` `.nsz` `.xcz` `.vb` `.min` | `emulator` |
| Sega | `.sms` `.gg` `.md` `.gen` `.smd` `.32x` `.sg` `.cdi` `.gdi` | `emulator` |
| Disco (Sony e outros) | `.iso` `.cue` `.bin` `.img` `.chd` `.pbp` `.cso` `.mdf` `.mds` `.ccd` `.m3u` | `emulator` |
| Atari | `.a26` `.a52` `.a78` `.lnx` `.j64` `.jag` `.st` `.stx` `.xex` | `emulator` |
| NEC, SNK, Bandai e outros | `.pce` `.sgx` `.tg16` `.ngp` `.ngc` `.ws` `.wsc` `.col` `.int` `.vec` `.rom` | `emulator` |
| Computadores | `.adf` `.dsk` `.d64` `.t64` `.tap` `.crt` `.tzx` | `emulator` |
| Compactados | `.zip` `.7z` | `emulator` |

Fora das listas (ignorados e contados no aviso): `.sub`, `.wav`, `.raw` (só valem como trilha de um contêiner, seção 4),
`.dll`, `.sav`, `.srm`, `.txt`, `.nfo`, imagens, vídeos etc. Qualquer outra extensão entra pela fonte "Arquivo".

A fonte "Arquivo" (IMV-032) **não** usa essas listas: qualquer tipo vira candidato; `.exe`/`.bat`/`.cmd` são `exe`, o resto
`emulator` (IMP-004).

### 1.1 Executáveis que não são jogos (IMV-015)

Só na classe executáveis (e em "qualquer", para os três tipos de executável). Casa o nome sem extensão, em minúsculas;
`*` = qualquer sequência:

`unins*` · `uninstall*` · `setup*` · `*setup` · `vcredist*` · `vc_redist*` · `dxsetup` · `dotnet*` · `oalinst` ·
`unitycrashhandler*` · `crashpad_handler` · `*crashreport*` · `*redist*` · `directx*`

## 2. Normalizador de título (IMV-001..IMV-008)

`NormalizeTitle(stem string, fromFolder bool) string`, onde `stem` é o nome sem a extensão (ou o nome da pasta). Passos, nesta
ordem:

1. Se `fromFolder`, remove o prefixo `^\d{1,3}\.\s+` quando o caractere seguinte não é dígito (IMV-006).
2. Remove grupos `\([^()]*\)` e `\[[^\[\]]*\]` até não restar nenhum (IMV-002).
3. `_` vira espaço (IMV-003).
4. **Limpa** (`clean`): espaços repetidos (inclusive tabulação) viram um; apara espaços, `-` e `,` das pontas (IMV-005).
   Depois corta em 200 runas e **limpa de novo** (IMV-005).
5. Se o texto (já limpo e cortado) não tem espaço, cada `.` que não está entre dois dígitos vira espaço; **limpa** de novo
   (IMV-003).
6. Move o artigo (IMV-004): primeira ocorrência de `, <Artigo>` seguida de fim, ` - ` ou `: `, **a menos que o texto já
   comece com um artigo** (`the|an|a|les|le|la|el|los|las|il|der|die|das` + espaço, sem diferenciar maiúsculas); **limpa** de
   novo (a remoção da vírgula pode deixar espaço sobrando).
7. Se o resultado é vazio, usa `stem` com os espaços normalizados (`clean` sobre o `stem` cru); se ainda vazio, não há título.

Por que esta ordem (IMV-008): toda decisão que depende do texto ("tem espaço?", "começa com artigo?") é tomada sobre o texto
já limpo e já cortado, e os passos seguintes só encurtam o texto ou preservam os espaços internos; assim a segunda passada
vê o mesmo texto e não muda nada.

### Vetores (`fromFolder = false`, salvo indicação)

| # | Entrada (`stem`) | Saída |
|---|---|---|
| 1 | `Super Mario World (U) [!]` | `Super Mario World` |
| 2 | `Legend of Zelda, The - Majora's Mask (U) [!]` | `The Legend of Zelda - Majora's Mask` |
| 3 | `Legend of Zelda, The - Ocarina of Time (USA)_redux_patched` | `The Legend of Zelda - Ocarina of Time redux patched` |
| 4 | `Final Fantasy VIII (USA) (Disc 1)` | `Final Fantasy VIII` |
| 5 | `Resident Evil 2 - Dual Shock Ver. (USA) (Disc 1)` | `Resident Evil 2 - Dual Shock Ver.` |
| 6 | `Dr. Mario (World)` | `Dr. Mario` |
| 7 | `Super.Mario.World.USA` | `Super Mario World USA` |
| 8 | `Sonic_The_Hedgehog_2_(W)_(REV01)` | `Sonic The Hedgehog 2` |
| 9 | `Pokemon - Red Version (UE) [S][!]` | `Pokemon - Red Version` |
| 10 | `Game [b1]` | `Game` |
| 11 | `Game v1.2 (USA)` | `Game v1.2` |
| 12 | `Game.v1.2.(USA)` | `Game v1.2` |
| 13 | `Super Mario Bros. 3 (U)` | `Super Mario Bros. 3` |
| 14 | `Hello, A World` | `Hello, A World` |
| 15 | `Zelda, The` | `The Zelda` |
| 16 | `Legend of Dragoon, The (USA) (Disc 1)` | `The Legend of Dragoon` |
| 17 | `Bug's Life, A (USA)` | `A Bug's Life` |
| 18 | `Astérix & Obélix (F)` | `Astérix & Obélix` |
| 19 | `(Disc 1)` | `(Disc 1)` |
| 20 | `Game - (USA)` | `Game` |
| 21 | `Game (USA` | `Game (USA` |
| 22 | `Game ((USA))` | `Game` |
| 23 | `A  B` + tabulação + `C` | `A B C` |
| 24 | `Final Fantasy VII (USA) (Disc 1) (v1.1)` | `Final Fantasy VII` |
| 25 | `007 - The World is Not Enough (U) [!]` | `007 - The World is Not Enough` |
| 26 | `Mr. Driller` | `Mr. Driller` |
| 27 | `Smith, Mr.` | `Smith, Mr.` |
| 28 | `The Legend of Zelda` | `The Legend of Zelda` |
| 29 | 250 letras `é` | 200 letras `é` (runas, não bytes) |
| 30 | `31. Spyro - Year of the Dragon (USA)` (`fromFolder = true`) | `Spyro - Year of the Dragon` |
| 31 | `31. Spyro - Year of the Dragon (USA)` (`fromFolder = false`) | `31. Spyro - Year of the Dragon` |
| 32 | `1942` (`fromFolder = true`) | `1942` |
| 33 | `2.5D Game` (`fromFolder = true`) | `2.5D Game` |
| 34 | `10. 4 Game` (`fromFolder = true`) | `10. 4 Game` |
| 35 | `roeteekhwuIT,.uD9k_` | `roeteekhwuIT, uD9k` (o `_` final some na aparagem antes de decidir "tem espaço"; o ponto vira espaço) |
| 36 | `Foo, A - Bar, The` | `A Foo - Bar, The` (só a primeira ocorrência move; a 2ª passada não move porque já começa com artigo) |
| 37 | `The Foo, A` | `The Foo, A` (já começa com artigo: nada é movido) |
| 38 | `Foo , The - Bar` | `The Foo - Bar` (o espaço antes da vírgula não sobra) |
| 39 | `Super.Mario.World (USA)` | `Super Mario World` (depois de tirar o grupo e aparar, não há espaço: pontos viram espaço) |
| 40 | `x.y` + 200 letras `a` + ` z` | `x y` + 197 letras `a` (o corte em 200 runas vem **antes** de decidir "tem espaço": o texto cortado não tem espaço, então o ponto vira espaço) |

Idempotência (IMV-008): `NormalizeTitle(NormalizeTitle(x, false), false) == NormalizeTitle(x, false)` para todos os vetores de
1 a 29 e 35 a 40.

### 2.1 Extensão do arquivo (IMV-001)

`stem` e `extension` de um nome de arquivo: o trecho depois do último `.` é a extensão quando tem de 1 a 5 caracteres
`[A-Za-z0-9]` e pelo menos uma letra; senão o nome inteiro é o `stem` e `extension = ""`.

| Nome | `stem` | `extension` |
|---|---|---|
| `Mr. Driller.zip` | `Mr. Driller` | `zip` |
| `Dr. Mario` | `Dr. Mario` | `` |
| `Game 2.0` | `Game 2.0` | `` |
| `Banjo-Kazooie (U) [!].Z64` | `Banjo-Kazooie (U) [!]` | `z64` |
| `Jogo.cue` | `Jogo` | `cue` |

## 3. Ordem natural e leitura (IMV-010, IMV-012, IMV-018)

- **Ordem natural do caminho relativo:** compara por segmento (separados por `\`), sem diferenciar maiúsculas (`strings.ToLower`
  e depois ordem de runas); dentro do segmento, sequências de dígitos são comparadas como número (e, se iguais, a mais curta
  vem primeiro, para ser determinístico). `Jogo 2` < `Jogo 10`; `04. FF` < `31. Spyro`.
- **Profundidade:** a pasta escolhida é o nível 0. Um arquivo é lido quando a pasta dele está no máximo no nível 8; as
  subpastas do nível 8 (nível 9) não são abertas, e o aviso de profundidade aparece uma vez.
- **Contagem de itens (20.000):** cada entrada de diretório (arquivo ou pasta) lida conta um; entradas ignoradas por IMV-012 não
  contam. Ao chegar a 20.000, a leitura para (a ordem de visita é a ordem natural, em profundidade).
- **Ignorados (IMV-012):** nome que começa com `.` ou `$`; `System Volume Information`, `@eaDir`, `__MACOSX`, `node_modules`
  (sem diferenciar maiúsculas); entradas cujo tipo não é arquivo nem pasta comum (link simbólico, junção, dispositivo).

## 4. Contêineres, trilhas e discos (IMV-020..IMV-023)

**Chave do nome** de um arquivo = `strings.ToLower(NormalizeTitle(stem, false))`: já sem marcações, inclusive `(Disc N)` e
`(Track NN)`.

**Número de disco** de um arquivo = N do primeiro marcador que casa `(?i)[\(\[]\s*(?:disc|disk|cd)\s*(\d{1,2})(?:\s*(?:of|de|/)\s*\d{1,2})?\s*[\)\]]`
no `stem` cru; 0 se não há marcador.

**Escopo** de um arquivo = a subpasta de primeiro nível da pasta escolhida que o contém; arquivo direto na pasta escolhida
tem o escopo raiz.

### 4.1 Trilhas acompanhantes (IMV-020)

Um arquivo é acompanhante, e não vira candidato, quando existe na **mesma pasta** um contêiner com a mesma chave:

| Contêiner | Acompanhantes |
|---|---|
| `.cue` | `.bin` `.img` `.sub` `.wav` |
| `.ccd` | `.img` `.sub` |
| `.mds` | `.mdf` |
| `.gdi` | `.bin` `.raw` |

Os contêineres seguem candidatos. Sem contêiner correspondente, `.bin`/`.img`/`.mdf` são candidatos comuns; `.sub`, `.wav` e
`.raw` soltos são ignorados por extensão (seção 1).

### 4.2 Grupos de disco (IMV-021, IMV-022)

1. Entre os arquivos principais (não acompanhantes) que têm número de disco ≥ 1, agrupa por (escopo, chave).
2. Dentro de cada grupo, em ordem natural do caminho, cada arquivo entra no grupo se o número de disco dele ainda não está
   no grupo; senão fica de fora, como candidato comum (IMV-022).
3. Um grupo com 2 ou mais discos vira **um** candidato: principal = menor número de disco; `discs` = tamanho do grupo;
   `discFiles` = caminhos relativos em ordem de número de disco. Grupo de 1 arquivo é candidato comum (`discs = 1`, sem
   `discFiles`).
4. Arquivos sem número de disco nunca entram em grupo.

### 4.3 `.m3u` (IMV-023)

Um `.m3u` cuja chave é igual à de um grupo de disco (mesma pasta que o arquivo principal do grupo, ou o escopo inteiro se os
discos estão em subpastas do mesmo escopo) torna-se o arquivo principal desse candidato; `discs` e `discFiles` continuam
os dos discos. Um `.m3u` sem grupo correspondente é candidato comum.

### 4.4 Exemplos

**Árvore A — o layout do usuário (`ps1`), título pelo arquivo:**

```
04. Final Fantasy VIII (USA)\Final Fantasy VIII (USA) (Disc 1)\Final Fantasy VIII (USA) (Disc 1).cue
04. Final Fantasy VIII (USA)\Final Fantasy VIII (USA) (Disc 1)\Final Fantasy VIII (USA) (Disc 1).bin
04. Final Fantasy VIII (USA)\Final Fantasy VIII (USA) (Disc 2)\Final Fantasy VIII (USA) (Disc 2).cue
04. Final Fantasy VIII (USA)\Final Fantasy VIII (USA) (Disc 2)\Final Fantasy VIII (USA) (Disc 2).bin
07. Resident Evil 2 (USA)\Resident Evil 2 - Dual Shock Ver. (USA) (Disc 1)\Resident Evil 2 - Dual Shock Ver. (USA) (Disc 1).cue
07. Resident Evil 2 (USA)\Resident Evil 2 - Dual Shock Ver. (USA) (Disc 2)\Resident Evil 2 - Dual Shock Ver. (USA) (Disc 2).cue
31. Spyro - Year of the Dragon (USA)\Spyro - Year of the Dragon (USA).cue
31. Spyro - Year of the Dragon (USA)\Spyro - Year of the Dragon (USA).bin
```

| `title` | `file` (principal) | `extension` | `discs` |
|---|---|---|---|
| `Final Fantasy VIII` | `04. ...\Final Fantasy VIII (USA) (Disc 1)\Final Fantasy VIII (USA) (Disc 1).cue` | `cue` | 2 |
| `Resident Evil 2 - Dual Shock Ver.` | `07. ...\...(Disc 1)\...(Disc 1).cue` | `cue` | 2 |
| `Spyro - Year of the Dragon` | `31. ...\Spyro - Year of the Dragon (USA).cue` | `cue` | 1 |

Com `titleFrom = folder`: `Final Fantasy VIII`, `Resident Evil 2`, `Spyro - Year of the Dragon` (as pastas de primeiro nível sem
o prefixo e sem as marcações). Os quatro `.bin` não aparecem. O agrupamento é o mesmo nas duas opções.

**Árvore B — jogos soltos (`n64`):** `Toy Story 2 (U) [!].zip`, `Legend of Zelda, The - Majora's Mask (U) [!].zip`,
`Bomberman 64 - Arcade Edition (J).zip` → três candidatos, títulos `Toy Story 2`,
`The Legend of Zelda - Majora's Mask`, `Bomberman 64 - Arcade Edition`; `launchType = emulator`; `file` = o nome do arquivo.

**Árvore C — `.m3u`:** `Jogo.m3u`, `Jogo (Disc 1).cue`, `Jogo (Disc 1).bin`, `Jogo (Disc 2).cue`, `Jogo (Disc 2).bin` na mesma
pasta → um candidato, `file = Jogo.m3u`, `extension = m3u`, `discs = 2`, `discFiles = [Jogo (Disc 1).cue, Jogo (Disc 2).cue]`.

**Árvore D — número de disco repetido (IMV-022):** `Jogo (USA) (Disc 1).cue`, `Jogo (Europe) (Disc 1).cue`,
`Jogo (USA) (Disc 2).cue` na mesma pasta. A decisão segue a ordem natural do caminho: `Jogo (Europe) (Disc 1).cue` entra
primeiro no grupo (disco 1); `Jogo (USA) (Disc 1).cue` fica de fora (o disco 1 já está no grupo); `Jogo (USA) (Disc 2).cue`
entra (disco 2). Resultado: candidato 1 = `Jogo (Europe) (Disc 1).cue` com `discs = 2` e
`discFiles = [Jogo (Europe) (Disc 1).cue, Jogo (USA) (Disc 2).cue]`; candidato 2 = `Jogo (USA) (Disc 1).cue` com `discs = 1`.
Os dois têm o título `Jogo` (a janela sinaliza "título repetido", IMV-046). O caso é raro; o objetivo é só ser determinístico.

**Árvore E — trilhas:** `Jogo (USA).cue`, `Jogo (USA) (Track 01).bin`, `Jogo (USA) (Track 02).bin`, `Outro.bin` → candidatos
`Jogo` (cue) e `Outro` (bin); as duas trilhas não aparecem.

**Árvore F — Windows:** `Jogo X\bin\jogo.exe`, `Jogo X\unins000.exe`, `Jogo X\readme.txt`, `Jogo X\data.pak`, `Jogo Y.exe` com
plataforma Windows → dois candidatos: `jogo` (título `jogo`, ou `Jogo X` com `titleFrom = folder`) e `Jogo Y`. `unins000.exe`
é ignorado por IMV-015, `readme.txt` e `data.pak` pela extensão; o aviso é
`Pasta: 3 arquivos ignorados pela extensão (.exe ×1, .pak ×1, .txt ×1)` (o executável ignorado por IMV-015 conta como `.exe`).

## 5. Textos de aviso (IMV-014, IMV-018, IMV-019, IMP-008)

Os avisos vão em `ImportScanResult.warnings` (máximo 500 caracteres cada). `{n}` etc. são números inteiros sem separador.

| Situação | Texto |
|---|---|
| nenhum jogo na leitura | `Pasta: nenhum jogo encontrado` (com "Ler subpastas" desligado: `Pasta: nenhum jogo no primeiro nível`) |
| ignorados por extensão ou por IMV-015 | `Pasta: {n} arquivos ignorados pela extensão ({ext} ×{k}, ...)` — até 5 extensões, da mais frequente para a menos; empate por ordem alfabética; um executável ignorado por IMV-015 conta como `.exe` |
| nenhum candidato e há arquivos de console numa leitura de Windows | `Pasta: a plataforma escolhida lê só executáveis; {n} arquivos de jogo de console foram ignorados` |
| nenhum candidato e há executáveis numa leitura de console | `Pasta: a plataforma escolhida lê só arquivos de jogo de console; {n} executáveis foram ignorados` |
| profundidade | `Pasta: subpastas além do nível 8 não foram lidas` |
| itens | `Pasta: leitura interrompida em 20000 itens; escolha uma subpasta` |
| pasta ilegível | `Pasta: {n} subpastas ilegíveis` |
| resultado cortado (IMV-019) | `Pasta: {total} jogos encontrados; mostrando {shown} (limite do tamanho da lista). Importe os demais por subpasta` |
| já existentes, pasta vazia, não encontrada, ilegível, "escolha uma pasta" | os da fase 6 (`Pasta: nenhuma pasta escolhida`, `Pasta: não encontrada`, `Pasta: ilegível`, `Pasta: escolha uma pasta, não um arquivo`) |

## 6. Resultado limitado (IMV-019)

`maxScanResultBytes = 921600`. O agente soma, na ordem do resultado, `len(json.Marshal(candidate)) + 1` de cada candidato e para
no primeiro que ultrapassaria o limite. O aviso de corte entra no fim de `warnings`. A soma não inclui `warnings` (até alguns KB,
folga dos 127 KiB restantes até 1 MiB).
