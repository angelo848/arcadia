# ScreenScraper — regras do provedor de vídeo

Requisitos: VID-003, VID-009..VID-020, SSC-001, SSC-004. Os pontos marcados **(a conferir)** dependem da conferência
([screenscraper-api-notes.md](screenscraper-api-notes.md)); uma divergência que mude regra visível volta à spec antes do código.

## 1. Sistemas por chave de seed (a conferir)

`scrapers.SystemID(seedKey) (int, bool)`; chave fora da tabela → sem vídeo (`platform-unmapped`).

| Chave de seed | Plataforma | `systemeid` esperado |
|---|---|---|
| `windows` | Windows | 138 |
| `3ds` | Nintendo 3DS | 17 |
| `n64` | Nintendo 64 | 14 |
| `nes` | Nintendo Entertainment System | 3 |
| `gba` | Nintendo Game Boy Advance | 12 |
| `gamecube` | Nintendo GameCube | 13 |
| `switch` | Nintendo Switch | 225 |
| `switch2` | Nintendo Switch 2 | — (sem sistema conhecido; fica fora até a conferência achar) |
| `wii` | Nintendo Wii | 16 |
| `ps1` | Sony Playstation | 57 |
| `ps2` | Sony Playstation 2 | 58 |
| `ps3` | Sony Playstation 3 | 59 |
| `snes` | Super Nintendo Entertainment System | 4 |

## 2. Requisições (a conferir)

Base `https://api.screenscraper.fr/api2/`. Parâmetros comuns de toda requisição: `devid`, `devpassword` (credencial
`screenscraperdev`), `softname=arcadia`, `ssid`, `sspassword` (credencial `screenscraper`), `output=json`.

| Passo | Endpoint | Parâmetros próprios | Uso |
|---|---|---|---|
| Busca | `jeuRecherche.php` | `systemeid`, `recherche` = `domain.SearchTerm(título)` | lista de jogos (até ~30), cada um com `id`, `noms[] {region, text}`, `systeme.id`, `dates[]`, talvez `medias[]` |
| Informações | `jeuInfos.php` | `systemeid`, `gameid` | só quando a busca não trouxe `medias[]` |
| Download | URL de `medias[].url` | (credenciais já embutidas pela API ou acrescentadas pelo cliente) | o arquivo do vídeo |

Toda resposta JSON pode trazer `response.ssuser` com `maxthreads`, `maxrequestspermin`, `requeststoday`, `maxrequestsperday`,
`requestskotoday`, `maxrequestskoperday`, guardados em `SSQuota` (última leitura vale).

Nada da ROM é enviado: sem `crc`, `md5`, `sha1`, `romnom`, `romtaille` (decisão do usuário, IMV-080).

## 3. Escolha do jogo

- `domain.Query{Title: SearchTerm(título), Year: ano do jogo (0 = sem), LibraryKey: chave de seed}`.
- Cada jogo da resposta vira um grupo de `domain.Member`, um por nome em `noms[]` (todas as regiões), com `Provider =
  "screenscraper"`, `PlatformKeys = [chave de seed]` quando `systeme.id` é o sistema pedido (senão vazio), `Year` = o menor ano
  de `dates[]` (0 se nenhum).
- `domain.ScoreGroup` por grupo; ordena por pontuação (empate: ordem da resposta); `domain.ClassifyMatch(primeiro, segundo)`.
- Só `ConfidenceMatch` segue. `doubt`/`none` → `media.PermanentError("sem correspondência no ScreenScraper")`.

Vetores (todos com o sistema pedido e sem ano, salvo indicação):

| Título do jogo | Nomes devolvidos (um jogo por linha `;`) | Resultado |
|---|---|---|
| `Super Mario World (USA)` | `Super Mario World` ; `Super Mario World 2: Yoshi's Island` | match no primeiro |
| `Resident Evil` | `Resident Evil 2` ; `Resident Evil` | match em `Resident Evil` |
| `Mega Man X` | `Mega Man` | sem correspondência (marca `x` difere, trava) |
| `Legend of Zelda, The - Ocarina of Time` | `The Legend of Zelda: Ocarina of Time` | match |
| `Final Fantasy VII` | `Final Fantasy VII` (EU) e `Final Fantasy VII International` (JP) no mesmo jogo | match (o melhor nome do grupo conta) |
| `Tetris` | `Tetris` ; `Tetris` (dois jogos homônimos no mesmo sistema, ambos com 1,00) | sem correspondência (margem < 0,05) |
| `Chrono Trigger` | (lista vazia) | sem correspondência |

## 4. Escolha da mídia

- Só `medias[].type == "video-normalized"`; nunca `video`.
- Ordem de região: `wor`, `us`, `eu`, `ss`, `jp`, sem região, depois a ordem da resposta.
- `format` diferente de `mp4` (quando presente) é pulado.
- Nenhuma → `media.PermanentError("sem vídeo no ScreenScraper")`.

## 5. Tamanho e formato

| Verificação | Momento | Falha |
|---|---|---|
| `medias[].size` > 20.971.520 | antes do download | permanente "vídeo maior que 20 MB" |
| `Content-Length` > 20.971.520 | cabeçalho da resposta | idem, corpo não lido |
| corpo > 20.971.520 bytes | durante a leitura (`LimitReader` de 20 MiB + 1) | idem, temporário apagado |
| bytes 4..8 ≠ `ftyp` | primeiros 12 bytes | permanente "formato de vídeo não suportado" |

Gravação: `<mediaRoot>/<gameId>/<sha256>.mp4`, `thumb_path = ''`, `kind = video`, `provider = screenscraper`,
`source_url = RedactURL(url da mídia)`.

## 6. Respostas e erros (a conferir)

| Resposta | Classe | Efeito na raia / no job |
|---|---|---|
| 200 com JSON válido | ok | segue |
| 200 com corpo não JSON contendo "non trouvée" / 404 | não encontrado | permanente "sem correspondência no ScreenScraper" |
| 400 | pedido inválido | permanente (registra `Warn` com a URL limpa) |
| 401, 423 | API fechada (servidor sobrecarregado ou fechado para não membros) | temporária (MID-008) |
| 403, 426 | credencial de desenvolvedor recusada / software bloqueado | raia `paused-credentials`; job volta a pendente sem gastar tentativa |
| 429 | limite de threads ou de requisições por minuto | temporária (MID-008) |
| 430, 431 | cota diária (ou de não encontrados) esgotada | raia `paused-quota` por 1 h; job volta a pendente sem gastar tentativa |
| 5xx, rede, prazo | temporária | MID-008 |
| `requeststoday ≥ maxrequestsperday` na última resposta | cota esgotada (preventivo) | `paused-quota` antes da próxima requisição |

Credencial ausente (qualquer das duas) → `no-credentials`, nenhuma requisição.

## 7. Limites

- Uma requisição por vez (raia de 1 trabalhador, `maxthreads` = 1 assumido mesmo se a conta permitir mais).
- Intervalo mínimo 1.500 ms (`scrapers.Limiter`); se `maxrequestspermin` é informado, intervalo = `max(1500, ceil(60000 /
  maxrequestspermin))` ms.
- O download do vídeo passa pelo mesmo limitador (a API conta mídia como requisição — a conferir).

## 8. Credenciais e redação

- Lidas do `KeyStore` a cada job (trocas valem no job seguinte); nunca guardadas em campo de struct exportado nem em log.
- `RedactURL(raw)`: remove os parâmetros `devid`, `devpassword`, `ssid` e `sspassword` (qualquer caixa) da query; URL inválida
  vira `"<url inválida>"`. Usada em: `media.source_url`, `media_job.last_error`, todo log, todo erro que sobe.
- Erros de `net/http` (`*url.Error`) são reconstruídos com `URL = RedactURL(...)` antes de embrulhar.

## 9. Estados da raia

| Estado | Entra quando | Sai quando |
|---|---|---|
| `active` | início; fim de pausa; credenciais completas | erro de cota, de credencial, ou credencial ausente |
| `paused-quota` | §6 (430/431 ou preventivo) | `pausedUntil` (agora + 1 h) chega |
| `paused-credentials` | §6 (403/426) | `provider.setKey`/`deleteKey` de `screenscraper*` |
| `no-credentials` | uma das duas credenciais ausente ao despachar | as duas passam a existir |

O estado vive só em memória (some ao reiniciar o agente; a primeira requisição o refaz).
