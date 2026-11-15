# Regras de busca, pontuação e imagens — fase 12

- **Requisitos:** MET-003..MET-009, MET-010..MET-015, MET-020, MET-021, MET-040, MET-068 ([requirements.md](../requirements.md)).
- **Natureza:** tabelas e vetores de teste que o test-writer usa como fonte. Se uma linha daqui e o requisito divergirem,
  vale o requisito e a divergência é bug desta spec.
- **Onde vira código:** `internal/domain` (`match_unified.go`: `MatchKey`, `SearchTerm`, `UnifiedScore`, `ClassifyMatch` e as
  constantes), `internal/scrapers` (`applicability.go`, `aggregate.go`, `images.go`, `limiter.go`, `platforms.go`) e
  `internal/importers` (`search_name.go`). As tabelas são `var` do pacote, nunca espalhadas em `if`.
- **Dois "NormalizeTitle", dois papéis:** `importers.NormalizeTitle` (fase 11) produz o **título proposto** de um arquivo e já
  entrega o título revisado que o lote usa; `domain.NormalizeTitle` (fase 7) produz a base da **chave de comparação**. Esta fase
  não muda nenhum dos dois.

## 1. Aplicabilidade por plataforma (MET-003)

A plataforma da busca é resolvida por nome, sem criar nada (PLT-025, passos 1 e 2). "Sem plataforma" = não informada ou não achada.

| Provedor | Precisa de chave | Aplica quando | Filtro de plataforma na consulta |
|---|---|---|---|
| `igdb` | sim (Client ID + Secret) | sempre | `where platforms = (<id>)` se a plataforma tem chave de seed com id em `scrapers.PlatformID(chave, "igdb")`; senão nenhum |
| `steamgriddb` | sim | sempre | nenhum (a API não tem plataforma) |
| `steamstore` | não | chave de seed `windows`; ou plataforma sem chave de seed com categoria `pc`; ou sem plataforma | nenhum (só tem jogos de PC) |

- **Segunda consulta do IGDB:** filtro ligado e resposta vazia → repete sem filtro. Os candidatos da segunda consulta ficam com
  `platformConfirmed = false` (trava de MET-007) e não ganham o bônus de plataforma.
- **Plataforma dos candidatos:** IGDB devolve `platforms.id`; o agente converte para chaves de seed pelo inverso de
  `igdbPlatformIDs` (ids fora da tabela são ignorados). `Candidate.PlatformKeys` guarda as chaves achadas. O nome mostrado é o
  da plataforma da biblioteca quando a chave dela está em `PlatformKeys`; senão o nome canônico da primeira chave conhecida
  (`domain.SeedPlatforms`); senão `""`. Steam Store: `PlatformKeys = ["windows"]`. SteamGridDB: vazio (sem informação).
- **Confere** (bônus de plataforma): a chave de seed da plataforma da biblioteca está em `PlatformKeys` de algum membro e a
  consulta não é a segunda do IGDB. Plataforma sem chave de seed: nunca confere e nunca trava.
- **Trava de plataforma** (MET-007): algum membro tem `PlatformKeys` não vazio, nenhum membro confere e a biblioteca tem
  plataforma com chave de seed; ou o grupo veio da segunda consulta do IGDB.

## 2. Chave de comparação e pontuação (MET-005..MET-007)

### 2.1 `domain.MatchKey(title) string`

1. `domain.NormalizeTitle` (minúsculas, sem grupos `(...)` e `[...]`, ™ ® © e sinais viram espaço, acentos comuns trocados, palavras
   de edição removidas, espaços únicos).
2. Descarta os termos `the`, `a`, `an` e `and` (o `&` já virou espaço no passo 1; `Mario & Luigi` e `Mario and Luigi` têm a
   mesma chave).
3. Troca os termos de algarismo romano **de duas letras ou mais** pelo número: `ii`=2, `iii`=3, `iv`=4, `vi`=6, `vii`=7, `viii`=8,
   `ix`=9, `xi`=11, `xii`=12, `xiii`=13, `xiv`=14, `xv`=15, `xvi`=16, `xvii`=17, `xviii`=18, `xix`=19, `xx`=20. `i`, `v` e `x`
   sozinhos **não** são convertidos (`Mega Man X`).
4. Junta com um espaço.

`Marks(key)` = lista, na ordem em que aparecem, dos termos da chave que **distinguem** um jogo de outro da mesma série: os
formados só por dígitos e as letras soltas (`x`, `v`, `z`...), **exceto** `s`, `t`, `d` e `m`, que são restos de apóstrofo
(`Link's` → `link s`, `Don't` → `don t`) e não distinguem nada. `Marks("mega man x") = [x]`;
`Marks("final fantasy 7") = [7]`; `Marks("link s awakening") = []`.

### 2.2 `domain.UnifiedScore`

```
sim       = 1 se as chaves são iguais; senão Dice sobre bigramas de runas das duas chaves (0 se alguma tem < 2 runas)
score     = min(1, sim + 0,05·[plataforma confere] + 0,05·[ano confere] + 0,02·min(provedores extras, 2))
se Marks(chave do título buscado) != Marks(chave do membro de maior sim): score = min(score, 0,79)   (MET-007)
se trava de plataforma:                                                        score = min(score, 0,79)   (MET-007)
```

- `provedores extras` = provedores do grupo além do primeiro. `ano confere` = ambos conhecidos e iguais.
- **Comparação em milésimos:** todo limiar é comparado como inteiro em milésimos (`int(math.Round(x·1000))`), nunca como
  `float64`, para que `0,95 − 0,90 ≥ 0,05` seja verdadeiro.
- O membro de maior `sim` de um grupo é o que define `sim` e as marcas a comparar; empate: o primeiro na ordem igdb, steamstore,
  steamgriddb.

### 2.3 Vetores

`plat` = bônus de plataforma, `ano` = bônus de ano, `ext` = bônus por provedor extra. Dice calculado à mão, 2 casas.

| Buscado | Candidato | Chaves (buscado / candidato) | Dice | Bônus | Trava | Pontuação |
|---|---|---|---|---|---|---|
| `Super Mario World` | `Super Mario World` | `super mario world` / igual | 1,00 (chaves iguais) | — | — | 1,00 |
| `The Legend of Zelda - Majora's Mask` | `The Legend of Zelda: Majora's Mask` | `legend of zelda majora s mask` / igual | 1,00 | — | — | 1,00 |
| `Mario & Luigi` | `Mario and Luigi` | `mario luigi` / igual | 1,00 | — | — | 1,00 |
| `Final Fantasy VII` | `Final Fantasy 7` | `final fantasy 7` / igual | 1,00 | — | — | 1,00 |
| `Link's Awakening` | `Links Awakening` | `link s awakening` / `links awakening` | 26/29 = 0,90 | — | marcas `[]` × `[]` (o `s` do apóstrofo não conta) | 0,90 |
| `Resident Evil` | `Resident Evil 2` | `resident evil` / `resident evil 2` | 24/26 = 0,92 | — | marcas `[]` × `[2]` | **0,79** |
| `Final Fantasy VII` | `Final Fantasy VIII` | `final fantasy 7` / `final fantasy 8` | 26/28 = 0,93 | — | marcas `[7]` × `[8]` | **0,79** |
| `Mega Man X` | `Mega Man` | `mega man x` / `mega man` | 14/16 = 0,88 | — | marcas `[x]` × `[]` | **0,79** |
| `Final Fantasy V` | `Final Fantasy 5` | `final fantasy v` / `final fantasy 5` | 26/28 = 0,93 | — | marcas `[v]` × `[5]` (`v` sozinho não vira número) | **0,79** |
| `Pokemon Red` | `Pokemon Red Version` | `pokemon red` / `pokemon red version` | 20/28 = 0,71 | — | — | 0,71 |
| `Pokemon Red` | `Pokemon Red Version` (2 provedores, ano e plataforma conferem) | idem | 0,71 | +0,05 plat +0,05 ano +0,02 ext | — | 0,83 |
| `Sonic` | `Sonic the Hedgehog` | `sonic` / `sonic hedgehog` | 8/17 = 0,47 | — | — | 0,47 |
| `Doom` | `DOOM` (IGDB) + `DOOM` (SteamGridDB) | `doom` / igual | 1,00 | +0,02 ext | — | 1,00 (teto) |

- A segunda linha de `Pokemon Red` mostra que os bônus não levam um título parecido a "provável": 0,83 < 0,85 e `sim` 0,71 < 0,80.
- Os números de Dice acima são de bigramas com espaço; o teste usa tolerância de 0,005 neles e confere exatamente a pontuação
  final das linhas travadas (0,79) e das de chaves iguais (1,00).

### 2.4 Juntar o mesmo jogo (MET-005)

Ordem de entrada: IGDB, Steam Store, SteamGridDB; dentro do provedor, a ordem devolvida. Um candidato `c` entra no grupo `g` quando:
`g` não tem o provedor de `c`; **e** (`MatchKey` igual **ou** Dice ≥ 0,90 com `Marks` iguais); **e** (ano de `c` ou do
primeiro membro de `g` desconhecido, ou diferença ≤ 1). Entre os grupos possíveis vale o de maior Dice (empate: o mais antigo).

| Caso | Resultado |
|---|---|
| IGDB `Hades` 2020 + SteamGridDB `Hades` 2020 | 1 grupo, 2 provedores |
| IGDB `Resident Evil` 1996 + IGDB `Resident Evil` 2002 | 2 grupos (mesmo provedor) |
| IGDB `Resident Evil` 1996 e 2002 + SteamGridDB `Resident Evil` sem ano | entra no grupo mais antigo (empate de Dice, o primeiro) |
| IGDB `Resident Evil` 2002 + SteamGridDB `Resident Evil` 1996 | 2 grupos (diferença de 6 anos) |
| IGDB `Sonic` + Steam `Sonic Mania` | 2 grupos (Dice 0,57 < 0,90) |

## 3. Limiares e confiança (MET-009)

Constantes em `internal/domain` (valores em milésimos entre parênteses):

| Constante | Valor | Uso |
|---|---|---|
| `MatchAuto` | 0,85 (850) | pontuação mínima de `match` |
| `MatchMinSimilarity` | 0,80 (800) | `sim` mínima de `match` (bônus não compensam título fraco) |
| `MatchMargin` | 0,05 (50) | vantagem mínima do primeiro sobre o segundo |
| `MatchDoubt` | 0,60 (600) | pontuação mínima de `doubt` |
| `MatchAlternative` | 0,40 (400) | piso de qualquer item da lista |
| `MatchCap` | 0,79 (790) | teto das travas |
| `MatchThreshold` (fase 7) | 0,80 | **inalterado**; só `scrape.search` (obsoleto) o usa |

`ClassifyMatch(first, second *Scored) Confidence`:

| `first` | Resultado |
|---|---|
| ausente | `none` |
| pontuação ≥ 850 **e** `sim` ≥ 800 **e** sem trava **e** (`second` ausente **ou** pontuação do primeiro − do segundo ≥ 50) | `match` |
| pontuação ≥ 600 (e não é `match`) | `doubt` |
| senão | `none` |

Vetores (pontuação e `sim` em milésimos; "trava" = limitado a 790):

| Primeiro | `sim` | Trava | Segundo | Confiança |
|---|---|---|---|---|
| 850 | 850 | não | ausente | `match` |
| 849 | 849 | não | ausente | `doubt` |
| 900 | 780 | não | ausente | `doubt` (título fraco, bônus) |
| 950 | 950 | não | 900 | `match` (vantagem exatamente 50) |
| 950 | 950 | não | 901 | `doubt` (vantagem 49) |
| 790 | 920 | sim | ausente | `doubt` |
| 600 | 600 | não | ausente | `doubt` |
| 599 | 599 | não | ausente | `none` |
| (lista vazia) | — | — | — | `none` |

## 4. Imagens (MET-012..MET-014, MET-020, MET-021)

### 4.1 O que cada provedor oferece

| Provedor | `cover` | `screenshot` | `background` | Requisições |
|---|---|---|---|---|
| IGDB | `cover.image_id` → `.../t_cover_big/<id>.jpg` | `screenshots.image_id` → `.../t_screenshot_big/<id>.jpg` | `artworks.image_id` → `.../t_1080p/<id>.jpg` | 0 depois da busca (vem embutido, MET-012) |
| SteamGridDB | `/grids/game/<id>` (estáticas, dimensões de retrato) | — | `/heroes/game/<id>` | 2 por jogo (MET-013) |
| Steam Store | — | `screenshots[].path_full` (só `https`) | — | 1 por jogo (MET-014) |

Miniatura da linha (`thumbUrl`, MET-068): IGDB `.../t_cover_small/<id>.jpg`; Steam
`https://cdn.cloudflare.steamstatic.com/steam/apps/<appid>/capsule_231x87.jpg`; SteamGridDB só se o candidato já trouxe uma
(não há na busca). A ordem é a do primeiro membro com miniatura. Hosts permitidos: `images.igdb.com`,
`cdn.cloudflare.steamstatic.com`, `cdn2.steamgriddb.com` (esquema `https`).

### 4.2 Prioridade por tipo e classe de plataforma (MET-020)

| Tipo | Plataforma de chave `windows` | Outras plataformas e sem plataforma |
|---|---|---|
| `cover` | SteamGridDB, IGDB | IGDB, SteamGridDB |
| `screenshot` | IGDB, Steam Store | IGDB, Steam Store |
| `background` | SteamGridDB, IGDB | SteamGridDB, IGDB |

`SelectImages(offers, have, active, classWindows)`: para cada tipo de `cover`, `screenshot`, `background`:
`vagas = max(0, MediaKindLimit(tipo) − have[tipo] − active[tipo])`; percorre os provedores na ordem da tabela, mantém a ordem
de cada um, descarta URL já vista e escolhe até `vagas`. O que sobrou das ofertas (depois de descartar URL repetida) conta em
`skippedByLimit`.

| Caso | `have` | `active` | Ofertas | Escolhidas | `skippedByLimit` |
|---|---|---|---|---|---|
| jogo vazio, Windows | — | — | SGDB 3 capas; IGDB 1 capa | 2 capas SGDB | 2 |
| jogo vazio, console | — | — | IGDB 1 capa; SGDB 3 capas | IGDB, 1ª SGDB | 2 |
| 9 screenshots | `screenshot: 9` | — | IGDB 5 | 1 | 4 |
| 2 capas | `cover: 2` | — | SGDB 2 | 0 | 2 |
| 8 screenshots + 2 jobs pendentes | `screenshot: 8` | `screenshot: 2` | IGDB 5 | 0 | 5 |
| mesma URL em IGDB e Steam | — | — | IGDB `u1`, Steam `u1` e `u2` | `u1`, `u2` | 0 |
| tipo desconhecido (`title`) | — | — | oferta `title` | 0 (tipo fora de MET-025) | — (não conta) |

## 5. Termo de busca e nome para busca (MET-004, MET-040)

### 5.1 `domain.SearchTerm(title) string`

Remove grupos `(...)` e `[...]`, ™ ® ©; reduz espaços; troca a **primeira** ` - ` por `: `; apara. Vazio é erro.

| Título | Termo |
|---|---|
| `Legend of Zelda - Majora's Mask (USA)` | `Legend of Zelda: Majora's Mask` |
| `Final Fantasy VII™` | `Final Fantasy VII` |
| `Spider-Man` | `Spider-Man` |
| `Game [!] (Disc 1)` | `Game` |
| `Foo  -  Bar` | `Foo: Bar` |
| `A - B - C` | `A: B - C` |
| `(Demo)` | erro (vazio) |

### 5.2 `importers.SearchName(path) string` (fonte "Arquivo")

1. `stem` = nome sem extensão (`SplitExt`, IMV-001). Remove do fim, repetidamente, sufixos de motor: separadores
   (`-`, `_`, ` `, `.`) seguidos de `win32`, `win64`, `x86`, `x64`, `32bit`, `64bit` ou `shipping`, sem diferenciar maiúsculas.
2. `nome` = `NormalizeTitle(stem, false)`.
3. Se `nome` não é vazio e **não** é genérico, é o resultado.
4. Senão examina, de baixo para cima, até 3 pastas acima do arquivo (sem incluir a raiz do disco): a primeira que não é genérica
   dá `NormalizeTitle(<pasta>, true)` (IMV-006); se esse resultado é vazio, passa para a seguinte.
5. Se nenhuma serve, o resultado é `nome` mesmo genérico (ou o `stem` cru se `nome` é vazio).

Genérico (nome de arquivo, comparado sem separadores nem maiúsculas): `game`, `games`, `launcher`, `launch`, `start`, `startgame`,
`play`, `run`, `main`, `app`, `application`, `client`, `bin`, `program`, `exe`.
Genérico (nome de pasta, sem diferenciar maiúsculas): `bin`, `bin32`, `bin64`, `binaries`, `win32`, `win64`, `x86`, `x64`,
`windows`, `release`, `debug`, `retail`, `shipping`, `build`, `game`, `games`, `jogos`, `app`, `program`, `program files`,
`program files (x86)`, `common`, `steamapps`, `data`, `roms`, `downloads`, `desktop`.

| Caminho | `searchName` |
|---|---|
| `E:\Games\Hades\Hades.exe` | `Hades` |
| `E:\Games\Hollow Knight\hollow_knight.exe` | `hollow knight` |
| `E:\Games\MyGame\MyGame-Win64-Shipping.exe` | `MyGame` |
| `E:\Games\Some Game\game.exe` | `Some Game` |
| `D:\Games\Hades II\Binaries\Win64\Launcher.exe` | `Hades II` (Win64 e Binaries são genéricas) |
| `E:\Games\Cuphead\Cuphead (v1.3.4).exe` | `Cuphead` |
| `E:\Games\04. Final Fantasy VIII (USA)\launcher.exe` | `Final Fantasy VIII` (prefixo `04. ` e `(USA)` saem, IMV-006) |
| `F:\game.exe` | `game` (sem pasta útil: nome genérico mesmo) |
| `E:\Roms\snes\Super Metroid (U).smc` | `Super Metroid` |

## 6. Intervalos e prazos (MET-001, MET-010, MET-011, MET-053)

| Constante | Valor |
|---|---|
| intervalo IGDB / SteamGridDB / Steam Store | 334 ms / 500 ms / 2.000 ms |
| prazo da busca única | 20 s |
| prazo da busca de um jogo no lote | 15 s |
| suspensão depois de falha temporária (1ª, 2ª, 3ª) | 30 s, 60 s, 120 s; na 3ª seguida o provedor é desativado no lote |
| trabalhadores da busca em lote | 3 |
| itens por busca única / alternativas por linha do lote | 10 / 5 (a linha tem até 5 itens, o primeiro é o escolhido) |
| janela de agrupamento do progresso | 500 ms |
| teto de uma página de resultados | 921.600 bytes; `limit` 1..200 (padrão 100) |
