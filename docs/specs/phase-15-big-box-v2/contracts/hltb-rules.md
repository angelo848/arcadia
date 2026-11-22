# HowLongToBeat (e reserva do IGDB) — regras do cliente e do serviço

Requisitos: TTB-002..TTB-014, TTB-020..TTB-022, TTB-027, TTB-033..TTB-040. Fonte da verdade para `internal/scrapers/hltb*.go`,
`internal/scrapers/igdb_ttb.go` e `internal/agent/timetobeat*.go`. · alterado em 2026-11-22 (aprovação, resposta 1): reserva do IGDB
(§10), validade do achado do IGDB (§1, §6) e recuo por fonte (§7). · alterado em 2026-11-22 (conferência T05 com requisições
reais): a descoberta do caminho por script não existe mais; o caminho é fixo e a busca exige um token obtido antes (§2, §3), com o
corpo e a resposta de §3 e §4. Nada visível ao usuário muda (estados, valores, prazos, cache e recuo iguais).

> **O HowLongToBeat não tem API oficial.** §2..§4 descrevem o formato **conferido em 2026-11-22** com requisições reais (§9). O site
> muda sem aviso: divergência que mude regra visível (estados, valores, prazos) volta à spec; divergência só de formato (caminho,
> nome de campo, cabeçalho) é ajustada aqui e no cliente, com teste novo em `testdata`.

## 1. Constantes

| Constante | Valor | Requisito |
|---|---|---|
| `hltbBaseURL` | `https://howlongtobeat.com` (configurável no cliente, `httptest` nos testes) | TTB-012 |
| `hltbInterval` | 2.000 ms entre requisições (`scrapers.Limiter` próprio) | TTB-010 |
| `hltbRequestTimeout` | 10 s por requisição | TTB-010 |
| `hltbInitPath` | `/api/search/site/init` (com `?t=<epoch ms>`) | TTB-012 |
| `hltbSearchPath` | `/api/search/site` | TTB-012 |
| `hltbSyncTimeout` | 25 s por `refresh` ou `search` (token + busca + token novo + busca repetida) | TTB-020, TTB-021 |
| `hltbPageSize` | 20 resultados pedidos por busca | TTB-004 |
| `hltbMaxCandidates` | 10 candidatos guardados ou devolvidos | TTB-002, TTB-021 |
| `hltbMaxBody` | 2 MiB lidos de qualquer resposta (o resto é descartado e conta como formato inválido) | TTB-011 |
| `ttbFoundTTL` | 90 dias (`found` do HowLongToBeat) | TTB-006 |
| `ttbIGDBFoundTTL` | 30 dias (`found` do IGDB, reserva) | TTB-006, TTB-033 |
| `ttbDoubtTTL`, `ttbNotFoundTTL` | 30 dias | TTB-006 |
| `ttbBackoff` | 1 min, 5 min, 30 min, 60 min (a quarta em diante), uma escada por fonte | TTB-011, TTB-038 |
| `igdbRequestTimeout` | 10 s por requisição da reserva (limitador do IGDB da fase 12) | TTB-038 |
| `ttbBatchQueueMax` | 1.000 jogos na fila do lote | TTB-009 |
| `ttbTermMax` | 200 caracteres | TTB-021 |
| `TTB_FETCH_DELAY_MS` (frontend) | 1.000 ms com o mesmo jogo em foco | TTB-015, BBX-059 |

## 2. Token da busca [TTB-012]

· alterado em 2026-11-22 (T05): substitui a descoberta do caminho por script (`_app-` + `/api/<segmento>/<chave>`). O site passou a
usar Turbopack (chunks `/_next/static/chunks/<hash>.js`, sem `_app-`) e o caminho da busca é **fixo**; o que muda entre sessões é um
token.

1. **Obter o token:** `GET {base}/api/search/site/init?t=<epoch ms>` (o instante vem do `domain.Clock` do cliente). Resposta aceita:
   `200` com o objeto JSON `{"token": "<string>"}` e token não vazio. Qualquer outra coisa (rede, status ≠ 200, JSON inválido, sem
   `token` ou vazio, corpo > `hltbMaxBody`): **falha temporária** "token do HowLongToBeat indisponível" (TTB-011).
2. **Guardar:** o token fica só em memória, num campo do cliente sob mutex, e é reusado em toda busca enquanto valer. Nunca vai para
   disco, banco, erro devolvido ou log (só `Debug` "token do HowLongToBeat renovado", sem o valor). Sem token guardado, a busca
   obtém um antes.
3. **Renovar:** uma busca que responde **403** (token expirado, pelo código do site) apaga o token guardado, obtém um novo e repete a
   busca **uma única vez**; um segundo 403 (ou falha ao obter o token novo) é falha temporária.
4. Um **404** na busca **não** dispara nada especial: é falha temporária (o site respondeu 404 a busca sem token e corpo mínimo).

Cada requisição (o `init` e cada busca, inclusive a repetida) passa pelo limitador de 2 s e tem o prazo de `hltbRequestTimeout`. O
pior caso de uma busca é 4 requisições (token, busca 403, token novo, busca), dentro de `hltbSyncTimeout`.

## 3. Pedido de busca [TTB-003, TTB-027]

· alterado em 2026-11-22 (T05): caminho fixo, cabeçalho `x-auth-token`, `year` no lugar de `rangeYear` e `gameplay` sem
`difficulty`.

`POST {base}/api/search/site` com o cabeçalho `x-auth-token: <token>` (§2) e o corpo JSON:

```json
{
  "searchType": "games",
  "searchTerms": ["hollow", "knight"],
  "searchPage": 1,
  "size": 20,
  "searchOptions": {
    "games": { "userId": 0, "platform": "", "sortCategory": "popular", "rangeCategory": "main",
               "rangeTime": { "min": null, "max": null },
               "gameplay": { "perspective": "", "flow": "", "genre": "" },
               "year": { "min": "", "max": "" }, "modifier": "" },
    "users": { "sortCategory": "postcount" }, "lists": { "sortCategory": "follows" },
    "filter": "", "sort": 0, "randomizer": 0
  },
  "useCache": true
}
```

- `searchTerms`: o termo (TTB-003) dividido por espaços, sem vazios.
- Cabeçalhos fixos, os mesmos em toda requisição (`init` e busca): `Accept: */*`, `Origin: {base}`, `Referer: {base}/` e
  `User-Agent` de navegador genérico fixo no código; na busca, também `Content-Type: application/json` e `x-auth-token`. Nenhum
  `Cookie` é enviado nem guardado entre requisições (cliente sem `Jar`). O único valor variável além do termo é o token (que veio do
  próprio site) e o `t` do `init`. Nada que identifique o usuário, a máquina ou o jogo no Arcadia é enviado.
- Respostas: `200` com JSON → §4. `403` → §2 passo 3 (renova e repete uma vez). `429` (limite de taxa; o site mostra "Too many
  searches, please wait a minute"), `404`, `5xx` e qualquer outro status → falha temporária (não há "erro permanente": o serviço pode
  voltar).

## 4. Leitura da resposta [TTB-005]

· alterado em 2026-11-22 (T05): a resposta real é um objeto com `color`, `title`, `category`, `count`, `pageCurrent`, `pageTotal`,
`pageSize`, `data`, `userData` e `displayModifier`; só `data` é lido. Cada item traz, além dos campos abaixo, outros que são
**ignorados** (`game_name_date`, `comp_all`, contagens etc.).

Exemplo real (sanitizado, item de "hollow knight"):

```json
{ "count": 8, "pageCurrent": 1, "pageTotal": 1, "pageSize": 20, "data": [
  { "game_id": 26286, "game_name": "Hollow Knight", "game_alias": "Hollow Knight: Voidheart Edition", "game_type": "game",
    "release_world": 2017, "profile_platform": "Nintendo Switch, PC, PlayStation 4, Xbox One",
    "comp_main": 97159, "comp_plus": 149918, "comp_100": 236144 } ] }
```

Sem resultado: `200` com `"count": 0` e `"data": []`.

| Campo | Uso | Regra |
|---|---|---|
| `data` | lista de resultados | ausente ou não lista → formato inválido (falha temporária) |
| `game_id` | `externalId` (texto do inteiro) | ausente ou ≤ 0 → o item é ignorado |
| `game_name` | título do candidato | vazio → o item é ignorado |
| `game_alias` | apelidos, separados por vírgula | cada parte não vazia vira mais um membro do grupo (§5) |
| `game_type` | — | itens com valor diferente de `game` (ex.: `dlc`, `mod`) são ignorados quando o campo existe |
| `release_world` | ano | inteiro entre 1950 e 2100; fora disso, 0 (desconhecido) |
| `profile_platform` | plataformas (texto, só para mostrar) | dividido por vírgula, sem vazios |
| `comp_main` | `mainSeconds` | inteiro > 0, senão nulo |
| `comp_plus` | `mainExtraSeconds` | inteiro > 0, senão nulo |
| `comp_100` | `completionistSeconds` | inteiro > 0, senão nulo |

Uma resposta `200` com `data` vazia é **válida** (sem resultados), não falha.

## 5. Correspondência [TTB-004, TTB-002, TTB-021]

Para o jogo `G` (título `t`, ano `y`, 0 se desconhecido):

1. Cada resultado vira um grupo `[]domain.Member`: `{Provider: "hltb", Title: game_name, Year: release_world}` mais um membro por
   apelido com o mesmo ano. `PlatformKeys` vazio, `PlatformConfirmed` falso.
2. `domain.ScoreGroup(domain.Query{Title: t, Year: y, LibraryKey: ""}, grupo)` — sem chave de plataforma, então sem bônus nem trava de
   plataforma; a trava de marcas (MET-007) vale.
3. Ordena por `Score` decrescente (empate: menor `game_id` primeiro, para ser determinístico).
4. `domain.ClassifyMatch(&primeiro, &segundo)` (segundo nulo com um só resultado): `match` → `found` com o primeiro; `doubt` → `doubt`;
   `none` → `not-found`.
5. Candidatos guardados (em `doubt` na linha; em `search` na memória do serviço, por jogo, até 64 jogos, o mais antigo sai): os até
   10 primeiros com `Score ≥ domain.MatchAlternative` (0,40) em `doubt`; em `search`, os 10 primeiros sem piso.

Vetores (título e ano de `G` → resultados → estado):

| `G` | Resultados (nome, ano) | Esperado | Por quê |
|---|---|---|---|
| "Hollow Knight" (2017) | ("Hollow Knight", 2017), ("Hollow Knight: Silksong", 2025) | `found` (26286) | 1,00 contra 0,6x; vantagem |
| "Super Mario World (USA)" (1990) | ("Super Mario World", 1990), ("Super Mario World 2: Yoshi's Island", 1995) | `found` | grupos e marcas; o segundo tem a marca "2" |
| "Resident Evil 2" (1998) | ("Resident Evil 2", 2019), ("Resident Evil 2", 1998) | `doubt` | as duas pontuações chegam a 1,00 (teto); vantagem 0 |
| "Mega Man X" (1993) | ("Mega Man X2", 1994) | não `match` | marcas diferentes ("x" contra "x2") → teto 0,79 |
| "Zelda" (0) | ("The Legend of Zelda", 1986), ("Zelda II", 1987) | `doubt` | o mais parecido ("Zelda II") tem marca diferente (teto 0,79); o outro tem similaridade < 0,80 |
| "Final Fantasy VII" (1997) | ("Final Fantasy VII", 1997), ("Final Fantasy VII Remake", 2020) | `found` | romanos viram números; vantagem ≥ 0,05 |
| "Jogo Que Não Existe" (0) | (nenhum) | `not-found` | sem resultados |
| "Celeste" (2018) | ("Celeste", 2018, alias "Celeste Classic") | `found` | o apelido não atrapalha (melhor membro) |

Os valores exatos de pontuação saem de `domain.ScoreGroup`; o teste confere o **estado**, não o número.

## 6. Validade e linha confirmada [TTB-006, TTB-020, TTB-022]

- `expires_at = fetched_at + TTL` do estado e da fonte (§1). Vencida quando `agora ≥ expires_at`. Vencida uma linha `igdb`, a
  busca nova recomeça pelo HowLongToBeat (§10).
- Também vencida, na leitura, quando `confirmed = 0` e `title_at_fetch ≠ título atual de G` (comparação exata do texto gravado).
- `confirmed = 1` (escolha em `choose`): a identidade (`external_id` ou "nenhum") nunca muda sozinha. Vencida uma linha `found`
  confirmada, a releitura busca com o termo `external_title` e procura o resultado com o mesmo `game_id`: achou → grava os valores
  novos e renova a validade; não achou → mantém os valores e renova a validade. Uma linha `not-found` confirmada nunca é relida
  sozinha (só `refresh` explícito, que então segue as regras de uma busca nova e desfaz a confirmação).

## 7. Falhas e recuo [TTB-011]

| Situação | Classe | Efeito |
|---|---|---|
| erro de rede, prazo vencido, `ctx` do pedido vencido | temporária | recuo |
| status 404, 429, 5xx, outro não 2xx da busca | temporária | recuo |
| 403 da busca | renova o token e repete uma vez (§2); o segundo 403 é temporária | recuo só se a repetição falhar |
| JSON inválido, `data` ausente, corpo > `hltbMaxBody` | temporária ("formato do HowLongToBeat mudou") | recuo |
| token não obtido (`init` com rede, status ≠ 200, sem `token`) | temporária | recuo |
| `200` com `data` vazia | resposta válida | `not-found` gravado |
| `ctx` do agente cancelado (encerramento) | — | nada gravado, sem recuo, sem log além de `Debug` |

Recuo, **um por fonte** (`hltb` e `igdb`): `failures` conta falhas seguidas da fonte (memória do serviço); depois da falha `n`,
`pausedUntil = agora + ttbBackoff[min(n, 4) − 1]`. Com o HowLongToBeat em recuo, a fila continua pela reserva (§10) enquanto o IGDB
estiver disponível (chave cadastrada e fora do recuo); com as duas indisponíveis, a fila não é atendida (o trabalhador espera o menor
`pausedUntil` pelo `domain.Clock`), `get` responde `unavailable: true` e não enfileira. `refresh` tenta as duas assim mesmo (pedido
explícito do usuário); `search` só o HowLongToBeat; cada falha conta `n + 1` na sua fonte. Falta de chave do IGDB (`ErrNoKey`) não é
falha e não entra no recuo (TTB-037). O `Warn`
"HowLongToBeat indisponível" (`cause` sem corpo nem URL com chave) sai quando `failures` passa de 0 para 1; os seguintes vão em
`Debug`. Uma resposta válida zera `failures` e `pausedUntil`.

## 8. Formato das horas (frontend) [TTB-014]

```
formatTimeToBeat(s):
  s nulo ou s ≤ 0          → "—"
  m = round(s / 300) × 5   (minutos em passos de 5)
  m < 60                   → max(m, 5) + " min"
  n = round(s / 1800)      (meias horas)
  → floor(n / 2) + (n ímpar ? "½" : "") + " h"
```

| s | texto |
|---|---|
| null, 0, −5 | — |
| 100 | 5 min |
| 1800 | 30 min |
| 3300 | 55 min |
| 3599 | 1 h |
| 5400 | 1½ h |
| 97200 | 27 h |
| 99000 | 27½ h |
| 99900 | 28 h |

`round` é o arredondamento para o inteiro mais próximo com meio para cima (`Math.round`).

## 10. Reserva do IGDB [TTB-033..TTB-040]

### 10.1 Tabela de fontes

`H` = resultado do HowLongToBeat; `I` = resultado do IGDB ("—" = não consultado). Vale para o trabalhador, para TTB-025 e para
`refresh` (que ignora os recuos). Linha confirmada (TTB-022) nunca consulta o IGDB.

| `H` | `I` | Gravado | Validade |
|---|---|---|---|
| `found` | — | `found`, `hltb` | 90 d |
| `doubt` | — | `doubt`, `hltb` (candidatos) | 30 d |
| `not-found` | valores | `found`, `igdb` | 30 d |
| `not-found` | sem `match` / sem valores | `not-found`, `hltb` | 30 d |
| `not-found` | falha, recuo ou sem chave | `not-found`, `hltb` | 30 d |
| falha ou recuo | valores | `found`, `igdb` | 30 d |
| falha ou recuo | sem `match` / sem valores | nada (sai da fila; `refresh` → `-32011`) | — |
| falha ou recuo | falha, recuo ou sem chave | nada (sai da fila; `refresh` → `-32011`) | — |
| termo vazio (TTB-003) | — | `not-found`, `hltb` | 30 d |

### 10.2 Identificar o jogo no IGDB

1. `scrapers.IGDB.Search(ctx, scrapers.Query{Title: título de G, Platform, Year, SeedKey})`, a mesma da busca de metadados (filtro
   por plataforma e segunda consulta sem filtro, MET-003), pelo limitador do IGDB.
2. Cada candidato vira um grupo de um membro (`Provider: "igdb"`, `PlatformKeys`, `PlatformConfirmed`); `domain.ScoreGroup` com
   `Query{Title, Year, LibraryKey: chave de seed}`; ordena; `domain.ClassifyMatch`. Só `match` segue.
3. `ErrNoKey` → reserva ignorada (TTB-037). `ErrUnavailable` ou credencial recusada → falha do IGDB (recuo da fonte `igdb`).

### 10.3 Ler `game_time_to_beats`

`POST https://api.igdb.com/v4/game_time_to_beats` (base configurável; `httptest` nos testes), com os cabeçalhos do cliente IGDB
existente (`Client-ID`, `Authorization: Bearer`) e o corpo:

```
fields game_id,hastily,normally,completely; where game_id = <id>; limit 1;
```

| Campo | Destino | Regra |
|---|---|---|
| `hastily` | `mainSeconds` (História principal) | inteiro > 0, senão nulo |
| `normally` | `mainExtraSeconds` (História + extras) | inteiro > 0, senão nulo |
| `completely` | `completionistSeconds` (100%) | inteiro > 0, senão nulo |

Lista vazia ou os três nulos → "sem valores". A linha gravada usa `external_id` = id do jogo no IGDB e `external_title` = nome do
candidato. Erros seguem o cliente IGDB (429/5xx/rede → `ErrUnavailable`).

### 10.4 Vetores

| Situação | `H` | `I` | Esperado |
|---|---|---|---|
| jogo popular | `found` | (não chamado) | `hltb`, 90 d; nenhuma chamada ao IGDB |
| remake ambíguo | `doubt` | (não chamado) | `doubt`, `hltb` |
| jogo só no IGDB | `not-found` | `match`, `hastily = 36000` | `found`, `igdb`, `mainSeconds = 36000`, 30 d |
| obscuro | `not-found` | `doubt` | `not-found`, `hltb` |
| HowLongToBeat fora | falha | `match` com valores | `found`, `igdb`; recuo do `hltb` = 1 min |
| HowLongToBeat fora, sem chave do IGDB | falha | `ErrNoKey` | nada gravado; sem chamada HTTP ao IGDB; sem `Warn` do IGDB |
| HowLongToBeat em recuo, IGDB em recuo | recuo | recuo | fila espera o menor `pausedUntil`; `get` com `unavailable: true` |
| IGDB com os três zeros | `not-found` | `match`, valores 0 | `not-found`, `hltb` |

## 9. Pontos a conferir (preenchido pela tarefa de conferência)

| Ponto | Esperado nesta spec | Conferido em | Resultado |
|---|---|---|---|
| Padrão do caminho de busca no script (`/api/<segmento>/` + partes) | §2 (versão anterior) | 2026-11-22 | **Divergente.** Não existe mais: o site usa Turbopack (chunks `/_next/static/chunks/<hash>.js`, nenhum `_app-`) e o caminho é fixo, `POST /api/search/site`. §2 reescrito. |
| Se a busca exige um passo prévio (token/"init") além do caminho | não (versão anterior) | 2026-11-22 | **Divergente.** Exige: `GET /api/search/site/init?t=<epoch ms>` → `200 {"token": "<string>"}` (só essa chave); a busca leva `x-auth-token`. Pelo código do site, 403 = token expirado (renovar e repetir uma vez), 429 = limite de taxa. Sem token e com corpo mínimo, a busca respondeu 404. §2, §3 e §7 ajustados. |
| Corpo aceito (§3) e campos obrigatórios | §3 | 2026-11-22 | **Divergente só no formato:** `year` no lugar de `rangeYear`, `gameplay` sem `difficulty`; o resto igual. "hollow knight" → 200 com 8 resultados. §3 ajustado. |
| Cabeçalhos mínimos aceitos (User-Agent, Referer, Origin) | §3 | 2026-11-22 | A busca respondeu 200 com `Content-Type: application/json` e `x-auth-token`; os cabeçalhos genéricos de navegador continuam sendo enviados (não se conferiu se são exigidos um a um). Nenhum `Cookie`. |
| Nomes e unidades de `comp_main`, `comp_plus`, `comp_100` (segundos) | §4 | 2026-11-22 | **Confirmado** (segundos). Ex.: Hollow Knight (26286) 97159 / 149918 / 236144, `release_world` 2017, alias "Hollow Knight: Voidheart Edition". A resposta é um objeto com `color, title, category, count, pageCurrent, pageTotal, pageSize, data, userData, displayModifier`; itens têm campos extras (ignorados). Vazio: `count: 0`, `data: []`. |
| `game_type` e valores | §4 | 2026-11-22 | **Confirmado:** `"game"` nos resultados de jogo. |
| Limite de taxa observado (429 com 1 requisição a cada 2 s?) | não | 2026-11-22 | Nenhum 429 nas poucas requisições da conferência; o 429 existe no código do site ("Too many searches, please wait a minute") e é falha temporária (§7). Intervalo de 2 s mantido. |
| IGDB `game_time_to_beats`: endpoint, campos `hastily`/`normally`/`completely` em segundos | §10.3 | 2026-11-22 | **Conferido apenas pela documentação** (credenciais nunca vão para o chat); confirmar com a chave do usuário no roteiro manual T38. |

Amostras reais sanitizadas da conferência (para o test-writer copiar para `internal/scrapers/testdata/hltb/` na T06): `init.json`,
`search-results.json` e `search-empty.json`, entregues pela sessão principal.
