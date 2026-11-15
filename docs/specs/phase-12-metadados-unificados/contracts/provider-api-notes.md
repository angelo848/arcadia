# Notas de API dos provedores (T02)

Conferência feita **só pela documentação pública**, sem chaves. As páginas oficiais da IGDB (`api-docs.igdb.com`) responderam 403 ao
buscador e a da SteamGridDB (`steamgriddb.com/api/v2`) é renderizada por JavaScript (sem conteúdo extraível). Por isso a maior parte
fica **NÃO VERIFICÁVEL sem chave / sem doc oficial**; o usuário confere na conferência manual (seção final). Nenhuma divergência
confirmada; nenhuma regra visível da spec foi contradita.

Legenda: CONFIRMADO (fonte citada), DIVERGENTE (valor correto), NÃO VERIFICÁVEL (conferir à mão). Fontes marcadas "secundária" são
bibliotecas de terceiros, não a doc oficial.

## 1. IGDB

| Ponto | Estado | Detalhe |
|---|---|---|
| (a) `category` x `game_type` | CONFIRMADO (secundária) | `category` está marcado como obsoleto em favor de `game_type` (pacote `igdb-api-types`: https://github.com/DmitryScaletta/igdb-api-types; protobuf em https://git.nite07.com/nite/go-igdb/src/tag/v0.0.14/proto/igdbapi.pb.go). Usar `game_type`. |
| Valores do enum | CONFIRMADO (secundária) | 0 main_game, 1 dlc_addon, 2 expansion, 3 bundle, 4 standalone_expansion, 5 mod, 6 episode, 7 season, 8 remake, 9 remaster, 10 expanded_game, 11 port, 12 fork, 13 pack, 14 update. Fonte: o proto acima (numeração herdada do `category`; a numeração de `game_type` é a mesma). Exemplo da T13 `game_type = (0,8,9,...)` é coerente: principal, remake, remaster (+ 10 expanded_game, 11 port se quiser). Excluídos: 1, 2, 3, 5, 6, 7, 13, 14. Decisão de 4 (standalone_expansion) e 12 (fork) fica para o usuário. |
| `where game_type = (...)` junto com `search` | NÃO VERIFICÁVEL | A doc oficial diz (de memória, não confirmado hoje) que `search` aceita `where`; testar `search "x"; where game_type = (0,8,9,10,11); fields ...; limit 10;`. Se der erro, substituto: filtrar `game_type` no cliente (já vem em `fields`). |
| (b) `where platforms = (<id>)` com `search` | NÃO VERIFICÁVEL | Sintaxe esperada: `where platforms = (6);`. Conferir junto com (a) na mesma consulta (`where game_type = (...) & platforms = (6);`). |
| (c) `cover.image_id`, `artworks.image_id`, `screenshots.image_id` na mesma consulta | NÃO VERIFICÁVEL | Esperado: `fields name, cover.image_id, screenshots.image_id, artworks.image_id, ...;` (expansão com ponto é a sintaxe documentada da IGDB). Conferir. |
| (d) tamanhos | PARCIAL (secundária) | URL: `https://images.igdb.com/igdb/image/upload/t_<tamanho>/<image_id>.jpg` (ou `.png`) (https://stackoverflow.com, resposta citada na busca; fórum LaunchBox lista `cover_big`, `cover_small`, `720p`, `1080p`). `t_screenshot_big` e `t_cover_small` / `t_cover_big` / `t_1080p` não confirmados em fonte oficial; são nomes que a doc da IGDB lista de memória. Conferir que as 4 URLs devolvem imagem. |
| (e) ids de `igdbPlatformIDs` | NÃO VERIFICÁVEL | Ver tabela abaixo. A doc de plataformas (`/platforms`) exige consulta. |

Ids em uso em `internal/scrapers/platforms.go` (para o usuário comparar com `POST /v4/platforms` `fields id,name; where id = (6,37,4,18,24,21,130,508,5,7,8,9,19); limit 50;`):

| chave | id | nome esperado |
|---|---|---|
| windows | 6 | PC (Microsoft Windows) |
| 3ds | 37 | Nintendo 3DS |
| n64 | 4 | Nintendo 64 |
| nes | 18 | Nintendo Entertainment System |
| gba | 24 | Game Boy Advance |
| gamecube | 21 | Nintendo GameCube |
| switch | 130 | Nintendo Switch |
| switch2 | 508 | Nintendo Switch 2 |
| wii | 5 | Wii |
| ps1 | 7 | PlayStation |
| ps2 | 8 | PlayStation 2 |
| ps3 | 9 | PlayStation 3 |
| snes | 19 | Super Nintendo Entertainment System |

## 2. SteamGridDB

| Ponto | Estado | Detalhe |
|---|---|---|
| (a) `GET /grids/game/<id>` com `dimensions=` e `types=static` | CONFIRMADO (secundária) | Biblioteca oficial `node-steamgriddb` (https://github.com/SteamGridDB/node-steamgriddb, `src/index.ts`): caminho `{base}/grids/{game\|steam}/{id}`, base `https://www.steamgriddb.com/api/v2`; parâmetros de consulta `styles`, `dimensions`, `mimes`, `types`, `platformdata` (listas separadas por vírgula), `nsfw`, `humor`, `epilepsy`, `oneoftag`, `page`. `types=animated,static` aparece em exemplos de wrappers. Valores aceitos de `dimensions` **não** listados na fonte: a doc oficial enumera dimensões válidas de grid (as de retrato `600x900`, `342x482`, `660x930` são as esperadas); dimensão inválida costuma dar erro 400. Conferir com chave. |
| (b) `GET /heroes/game/<id>` | CONFIRMADO (secundária) | Mesmo código: `{base}/heroes/{game\|steam}/{id}` com os mesmos filtros (`styles`, `dimensions`, `mimes`, `types`, ...). Dimensões válidas de hero (ex.: `1920x620`, `3840x1240`) não confirmadas; **recomendação: não filtrar `dimensions` em heroes** e pegar o primeiro. |
| (c) sem screenshot de gameplay | CONFIRMADO (indireto) | A API tem só grids, heroes, logos e icons (a lib oficial expõe `getGrids`, `getHeroes`, `getLogos`, `getIcons`); nenhum endpoint de screenshots. Decisão 5 mantida. |
| (d) formato da resposta | CONFIRMADO (secundária) | Objetos de grid têm `id`, `score`, `style`, `url`, `thumb`, `tags`, `author{name,steam64,avatar}`; envelope `{"success":true,"data":[...]}`. Usar `data[].url`. Em erro, `success:false` com `errors`. |

## 3. Steam Store

| Ponto | Estado | Detalhe |
|---|---|---|
| host do `capsule_231x87.jpg` | NÃO VERIFICÁVEL (doc oficial inexistente) | Busca pública só mostra o nome do arquivo (SteamDB, ex.: https://steamdb.info/app/892970). O padrão `https://cdn.cloudflare.steamstatic.com/steam/apps/<appid>/capsule_231x87.jpg` precisa de um `curl -I` manual. Observação: a Steam também serve `shared.fastly.steamstatic.com` / `cdn.akamai.steamstatic.com`; se o host divergir, ajustar MET-068 e a lista de hosts permitidos. |
| `screenshots[].path_full` em `https` | NÃO VERIFICÁVEL | `appdetails` não é documentado oficialmente. Conferir com `curl "https://store.steampowered.com/api/appdetails?appids=<id>"` e olhar o prefixo de `path_full`. O descarte de `http` (MET-014) cobre os dois casos. |

## 4. Para o usuário conferir (com chaves, nada gravado no repositório)

1. IGDB: `search "hades"; where game_type = (0,8,9,10,11) & platforms = (6); fields name,game_type,platforms.id,first_release_date,cover.image_id,screenshots.image_id,artworks.image_id; limit 10;` -- deve responder 200 sem erro e trazer os campos expandidos.
2. IGDB: abrir no navegador as 4 URLs `https://images.igdb.com/igdb/image/upload/t_{cover_big,screenshot_big,1080p,cover_small}/<image_id>.jpg`.
3. IGDB: consulta de `/platforms` acima e comparar nomes com a tabela.
4. SteamGridDB: `GET /grids/game/<id>?dimensions=600x900,342x482,660x930&types=static` (200 com `data[].url`) e `GET /heroes/game/<id>` sem filtro.
5. Steam: `curl -I https://cdn.cloudflare.steamstatic.com/steam/apps/<appid>/capsule_231x87.jpg` (200) e prefixo `https` de `path_full`.

Se (1) falhar por `game_type` + `search`, o substituto é filtrar no cliente; se (4) rejeitar alguma dimensão, remover só essa dimensão da lista. Nenhum dos dois muda regra visível.
