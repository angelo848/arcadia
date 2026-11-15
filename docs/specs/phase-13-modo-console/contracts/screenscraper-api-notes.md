# Notas de API do ScreenScraper (conferência)

**Estado: NÃO VERIFICADO.** Este arquivo é preenchido pela tarefa de conferência, **antes** do cliente
(`internal/scrapers/screenscraper.go`), como o [provider-api-notes.md](../../phase-12-metadados-unificados/contracts/provider-api-notes.md)
da fase 12. As hipóteses abaixo vêm de memória e de documentação de terceiros; nenhuma foi conferida com a API real.

Legenda: CONFIRMADO (fonte citada), DIVERGENTE (valor correto), NÃO VERIFICÁVEL (conferir com credenciais, à mão).
Credenciais: o usuário cadastra pelo app (SSC-002); **nada** de credencial no repositório, na conversa ou em log. Para conferir
por linha de comando, as credenciais vão em variáveis de ambiente da sessão, nunca em arquivo versionado.

| # | Ponto | Hipótese usada na spec | Estado | Se divergir |
|---|---|---|---|---|
| 1 | Base e versão | `https://api.screenscraper.fr/api2/` | NÃO VERIFICADO | trocar a constante |
| 2 | Busca por nome | `jeuRecherche.php?systemeid=&recherche=` devolve até ~30 jogos ordenados por probabilidade | NÃO VERIFICADO | se não existir, `jeuInfos.php?systemeid=&romnom=<SearchTerm>` (sem `romtaille`/hash) e um só candidato |
| 3 | Campos do jogo | `response.jeux[]` com `id`, `noms[] {region, text}`, `systeme {id}`, `dates[] {region, text}`, `medias[]` | NÃO VERIFICADO | ajustar o decodificador; regra de §3 inalterada |
| 4 | Mídias na busca | `jeuRecherche` já traz `medias[]` | NÃO VERIFICADO | se não, `jeuInfos.php?gameid=` (uma requisição a mais por jogo) |
| 5 | Mídia de vídeo | `medias[] {type: "video-normalized", url, region?, format: "mp4", size?}` | NÃO VERIFICADO | se `size` ausente, valem `Content-Length` e o limite de leitura |
| 6 | URL da mídia | `mediaVideoJeu.php?...&media=video-normalized` com as credenciais **embutidas** | NÃO VERIFICADO | se sem credenciais, o cliente as acrescenta em memória |
| 7 | Formato | MP4 H.264 (tocável na WebView2) | NÃO VERIFICADO | se outro codec, BBX-012 cai na arte |
| 8 | Quota | `response.ssuser {maxthreads, maxrequestspermin, requeststoday, maxrequestsperday, requestskotoday, maxrequestskoperday}`; endpoint `ssuserInfos.php` | NÃO VERIFICADO | sem esses campos, só os códigos de §6 pausam |
| 9 | Cota típica de conta grátis | ~1 thread, ~20.000 requisições/dia | NÃO VERIFICADO | a raia já é de 1 e o intervalo de 1,5 s dá ~57.600/dia no máximo: a pausa por cota cobre |
| 10 | Download conta na cota | sim | NÃO VERIFICADO | se não, o download sai do limitador (mais rápido) |
| 11 | Códigos HTTP | 400, 401, 403, 404, 423, 426, 429, 430, 431 com os sentidos de [rules §6](screenscraper-rules.md) | NÃO VERIFICADO | ajustar a tabela de §6 |
| 12 | Corpo de erro | texto puro (não JSON) em alguns erros, inclusive com status 200 | NÃO VERIFICADO | o decodificador trata corpo não JSON |
| 13 | Ids de sistema | tabela de [rules §1](screenscraper-rules.md) (`systemesListe.php`) | NÃO VERIFICADO | corrigir a tabela; `switch2` entra se existir |
| 14 | Vídeo por plataforma | `systemesListe.php` traz `medias[]` do sistema com algum vídeo | NÃO VERIFICADO | só anotar (decisão k: entra com o tema Roda) |
| 15 | `softname` | texto livre identificando o software (`arcadia`) | NÃO VERIFICADO | ajustar a constante |

## Para o usuário conferir (com credenciais, nada gravado no repositório)

1. `jeuRecherche.php` com `systemeid=4` e `recherche=Super Mario World`: 200, JSON, `jeux[]` com `noms` e, se vier, `medias`.
2. `jeuInfos.php` com `systemeid=4` e o `gameid` do passo 1: `medias[]` com `type = video-normalized`, `format`, `size`, `url`.
3. Baixar a URL do passo 2 (sem salvar no repositório): tamanho ≈ 2 MB, primeiros bytes com `ftyp`, toca no Edge.
4. `ssuserInfos.php`: os campos de cota do item 8.
5. `systemesListe.php`: conferir os ids da tabela §1 e procurar `medias` de vídeo do sistema (item 14).
6. Um erro de propósito (senha errada) para ver o código e o corpo (itens 11 e 12).
