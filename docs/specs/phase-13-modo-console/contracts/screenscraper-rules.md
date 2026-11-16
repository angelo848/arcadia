# ScreenScraper — regras do provedor de vídeo

Requisitos: VID-003, VID-009..VID-020, VID-027..VID-029, SSC-001, SSC-004, SSC-006. Os pontos marcados **(a conferir)** dependem da conferência
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
| Download | URL de `medias[].url` | (credenciais já embutidas pela API ou acrescentadas pelo cliente, **só depois** da conferência de origem do §10) | o arquivo do vídeo |

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
| 200 com `Content-Type` `text/*` | cabeçalho da resposta, antes de `FetchVideo` | classificação do texto (§6, VID-029); corpo lido até 4.096 bytes, nenhum temporário |

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
| URL de mídia fora da origem (§10) | pedido inválido (`ErrBadRequest`), **sem requisição** | permanente "URL de vídeo fora do ScreenScraper" (VID-027) |
| Redirecionamento para outro host ou de `https` para `http` (§11) | pedido inválido (`ErrBadRequest`), o outro host não é chamado | permanente "redirecionamento para fora do ScreenScraper" (VID-028) |

Credencial ausente (qualquer das duas) → `no-credentials`, nenhuma requisição.

**Texto no corpo (download do vídeo, VID-029).** Um 200 com `Content-Type` `text/*` (subtipo e parâmetros livres, sem diferença de
caixa: `text/plain; charset=utf-8`, `TEXT/HTML`) no download não é vídeo: o corpo, lido até 4.096 bytes e fechado, passa pela
mesma classificação de texto dos endpoints JSON, nesta ordem:

| Texto contém | Classe | Efeito |
|---|---|---|
| `quota de scrape` | cota (`ErrQuota`) | `paused-quota`, sem gastar tentativa (VID-016) |
| `blacklisté` | credencial (`ErrCredentials`) | `paused-credentials`, sem gastar tentativa (VID-017) |
| `API closed` ou `API totalement fermé` | temporária (`ErrUnavailable`) | MID-008 (VID-018) |
| `non trouvée`, ou começa por `Erreur : Rom` | não encontrado (`ErrNotFound`) | permanente "sem vídeo no ScreenScraper" |
| nenhum dos acima (ex.: `NOMEDIA`) | formato | permanente "formato de vídeo não suportado" (VID-014) |

Outro `Content-Type` (ou ausente) segue para `FetchVideo`, que confere `ftyp` (§5).

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
- As credenciais só são acrescentadas a uma URL que passou pelo §10, e só seguem num redirecionamento permitido pelo §11.
- `Keys.SetPair` e `provider.setKey` recusam `\n` e `\r` em qualquer posição do `id` ou do `secret` (SSC-006): `-32602` no IPC,
  nada gravado, a credencial anterior continua.

## 9. Estados da raia

| Estado | Entra quando | Sai quando |
|---|---|---|
| `active` | início; fim de pausa; credenciais completas | erro de cota, de credencial, ou credencial ausente |
| `paused-quota` | §6 (430/431 ou preventivo) | `pausedUntil` (agora + 1 h) chega |
| `paused-credentials` | §6 (403/426) | `provider.setKey`/`deleteKey` de `screenscraper*` |
| `no-credentials` | uma das duas credenciais ausente ao despachar | as duas passam a existir |

O estado vive só em memória (some ao reiniciar o agente; a primeira requisição o refaz).

Qual raia: um job é de vídeo **se e somente se** `kind = video` (VID-009). O despacho (vaga de 1 ou vagas de 2) e o roteador do
fetch (resolvedor de vídeo ou `media.Store`) usam esse único critério; `provider` não decide raia.

## 10. Origem da URL de mídia (VID-027)

A URL de `medias[].url` vem da resposta JSON e não é confiável. `OpenVideo` a confere **antes** de ler as credenciais, de
acrescentá-las e de passar pelo limitador. Com `b` = `BaseURL` analisado e `u` = a URL da mídia analisada:

1. `url.Parse` falha, `u` não é absoluta ou `u.Host` é vazio → recusa.
2. Esquema (sem diferença de caixa): `https` aceito; `http` aceito **só** quando `b.Scheme == "http"` **e** `u.Host == b.Host`
   (o `httptest` dos testes; em produção o `BaseURL` é `https` e `http` nunca passa); qualquer outro → recusa.
3. Host (sem diferença de caixa): `u.Host == b.Host` (com a porta), ou `u.Hostname()` termina em `.screenscraper.fr`
   (sufixo com o ponto) → aceito; senão → recusa.

Recusa: `ErrBadRequest` embrulhado com "URL de vídeo fora do ScreenScraper", nenhuma requisição, nenhuma reserva do limitador,
nenhuma leitura de `Creds`; o `Warn` do resolvedor traz só o passo e a mensagem (nunca a URL nem credencial).

Vetores (`BaseURL` = `https://api.screenscraper.fr/api2/`, salvo indicação):

| URL da mídia | Resultado |
|---|---|
| `https://api.screenscraper.fr/api2/mediaVideoJeu.php?systemeid=4&jeuid=1&media=video-normalized` | aceita (mesmo host) |
| `https://neoclone.screenscraper.fr/api2/mediaVideoJeu.php?...` | aceita (sufixo) |
| `HTTPS://NEOCLONE.SCREENSCRAPER.FR/v.mp4` | aceita (caixa não importa) |
| `http://neoclone.screenscraper.fr/v.mp4` | recusa (`http` com `BaseURL` `https`) |
| `https://screenscraper.fr.evil.example/v.mp4` | recusa |
| `https://evilscreenscraper.fr/v.mp4` | recusa (sem o ponto antes do sufixo) |
| `https://x.screenscraper.fr@evil.example/v.mp4` | recusa (o host é `evil.example`) |
| `ftp://neoclone.screenscraper.fr/v.mp4` | recusa (esquema) |
| `/api2/mediaVideoJeu.php?...` e `//evil.example/v.mp4` | recusa (relativa / sem esquema) |
| `%zz` (inválida) | recusa |
| `BaseURL` = `http://127.0.0.1:P/`: `http://127.0.0.1:P/v.mp4` | aceita (o `httptest`) |
| `BaseURL` = `http://127.0.0.1:P/`: `http://127.0.0.1:Q/v.mp4` (outra porta) | recusa |
| `BaseURL` = `http://127.0.0.1:P/`: `http://neoclone.screenscraper.fr/v.mp4` | recusa (`http` fora do host do `BaseURL`) |

## 11. Redirecionamentos (VID-028)

Toda requisição do cliente (busca, informações e vídeo) usa um `http.Client` próprio do `ScreenScraper`: cópia rasa do
`SSConfig.HTTP` (ou do padrão) com `CheckRedirect` próprio; o cliente compartilhado do agente nunca é alterado. O `CheckRedirect`:

- segue só se `req.URL.Host` é igual a `via[0].URL.Host` (sem diferença de caixa, com a porta) **e** o esquema não passa de
  `https` para `http`;
- mantém o teto de 10 redirecionamentos do `net/http`;
- senão devolve erro: o pedido ao outro host não é feito (nenhuma credencial é reenviada a ele) e o cliente devolve
  `ErrBadRequest` embrulhado com "redirecionamento para fora do ScreenScraper" (permanente, nunca `ErrUnavailable`), com o
  `*url.Error` passado por `RedactError`.
