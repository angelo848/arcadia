# Fase 12 — Metadados unificados: requisitos

- **Status:** rascunho para aprovação do usuário (2026-11-15). Perguntas em aberto no fim.
- **Design:** [design.md](design.md) · **Contratos:** [contracts/ipc-v0.14.0-delta.md](contracts/ipc-v0.14.0-delta.md)
  (só o acréscimo sobre a v0.13.0 de [phase-11-importacao-v2](../phase-11-importacao-v2/contracts/ipc.openrpc.json); o JSON
  completo `ipc.openrpc.json` v0.14.0 é uma tarefa, como na 11), [contracts/match-rules.md](contracts/match-rules.md)
  (pontuação, limiares, aplicabilidade por plataforma, prioridades, intervalos e vetores de teste) e
  [contracts/batch-state-machine.md](contracts/batch-state-machine.md) (estados da busca em lote) · **Tarefas:** `tasks.md`
  (o planner gera depois da aprovação).
- **Base:** [phase-7-midia](../phase-7-midia/requirements.md) (MID-001..MID-006, MID-008, MID-010),
  [phase-9-plataformas](../phase-9-plataformas/requirements.md) (chave de seed e `PlatformID`),
  [phase-10-2-imagens-ao-vivo](../phase-10-2-imagens-ao-vivo/requirements.md) (`media.changed`, IMG-001..IMG-011),
  [phase-11-importacao-v2](../phase-11-importacao-v2/requirements.md) (janela de revisão, IMV-040..IMV-052, IMV-072) e
  [phase-10-nova-interface](../phase-10-nova-interface/requirements.md) (UIX-033, UIX-037..UIX-039, UIX-046).
- **ROADMAP:** fase 12 (pontos 6 e 8), inclusive o **pedido do usuário de 2026-11-15**: a busca em lote roda logo depois da
  janela de revisão de títulos da fase 11, sobre os jogos recém-importados e com os títulos revisados; só depois vem a janela
  de confirmação dos matches.
- **ADRs:** 0015 (o agente é o único escritor), 0030 (limites de imagem por tipo), 0031 (avisos em segundo plano) e
  0032 (resultado limitado ao teto do IPC). Nenhum é contrariado. ADRs propostos no fim.
- **Objetivo:** o usuário só pede "Obter dados". O agente consulta todos os provedores que servem à plataforma, junta o que é o
  mesmo jogo, pontua por título normalizado + plataforma + ano e devolve **uma** lista. Na importação de uma pasta, a busca roda
  sozinha depois que os jogos entram, em segundo plano, com progresso e cancelamento; o usuário então confirma os matches numa
  janela e só aí os metadados são gravados e as imagens entram na fila de mídia. A mídia passa a ser completa por provedor e
  limitada por tipo. **Nada em `internal/tracker`, em `cmd/fake-game`, nas migrações ou no armazenamento de tempo de jogo muda.**

Formato: IDs `MET-NNN` (**MET** = metadados unificados), estáveis e nunca reutilizados; critérios Dado/Quando/Então. Todo ID
ativo precisa de um teste que o cite no nome (`t.Run("MET-006/...")`, `it("MET-061 ...")`), exceto os marcados
`· verificação: manual`. Nenhum teste usa rede (provedores fake e `httptest`) nem `time.Sleep` (relógio fake, `domain.Clock`).
Os vetores de pontuação e de nome para busca estão em [contracts/match-rules.md](contracts/match-rules.md).

---

## Decisões de produto (recomendação indicada)

Resolvem os itens "A decidir" do roadmap. O usuário pode trocar qualquer uma antes da aprovação.

1. **ScreenScraper fica fora desta fase.** Ele é o melhor para ROM, mas exige conta do usuário **e** credenciais de
   desenvolvedor do aplicativo, tem limite de requisições e de threads por conta, e o jeito preciso de achar o jogo é pelo hash
   do arquivo (abrir a ROM, contra IMV-080 e IMP-009). Sem ele, o retro depende de IGDB (ids de plataforma já mapeados no seed)
   e SteamGridDB. O agregador nasce com a porta aberta: um provedor novo é uma implementação de `MetadataProvider`, uma linha na
   tabela de aplicabilidade e uma na de prioridades (design §4). Uma fase 12.1 o inclui se a cobertura de retro ficar curta.
2. **Limiares (contracts/match-rules.md §3).** "Provável" (marcado sozinho) exige pontuação ≥ **0,85**, similaridade de título
   (sem bônus) ≥ **0,80**, vantagem de pelo menos **0,05** sobre o segundo colocado e nenhuma trava (MET-007). Entre **0,60**
   e o limiar é "em dúvida". Abaixo de 0,60 é "nenhum". Alternativas valem a partir de **0,40**. O 0,80 de MID-002 era para um
   diálogo em que o usuário sempre escolhia; num lote sem ninguém olhando, 0,85 com margem e trava de número (que impede
   `Resident Evil` de casar com `Resident Evil 2` (similaridade 0,92) ou `Mega Man X` com `Mega Man` (0,88)) é o mínimo seguro.
3. **Limite de requisições por provedor:** IGDB 1 requisição a cada 334 ms (3 por segundo, abaixo dos 4 do plano gratuito),
   SteamGridDB 1 a cada 500 ms, Steam Store 1 a cada 2 s (a loja aceita cerca de 200 por 5 minutos). Um lote de 1.000 jogos
   leva da ordem de 6 a 9 minutos de busca (IGDB e SteamGridDB em paralelo), em segundo plano. A Steam Store, a mais lenta, só é
   consultada no lote quando os outros não acharam um "provável" (MET-015).
4. **O limite de imagens por tipo conta as imagens que o jogo já tem e as já enfileiradas** (mesma regra que o ADR 0030 deu à
   cópia do LaunchBox). Um jogo que já tem 1 capa não ganha capa nova; nada é apagado para dar lugar (alterado em 2026-11-15 (capa = 1); era 2).
5. **SteamGridDB não tem screenshot de gameplay.** O roadmap supõe "screenshots de gameplay" no SteamGridDB, mas a API v2
   oferece capas (`grids`), heróis (`heroes`), logos e ícones, não screenshots (conferir na API real na tarefa, como se fez com os
   ids do IGDB). Esta spec entrega do SteamGridDB a **capa** (só retrato) e o **herói como fundo** (`background`, fanart, limite
   2); os screenshots vêm do IGDB e, no Windows, da Steam Store. Logos e ícones ficam fora (exigiriam tipo novo e migração do
   `CHECK` de `media.kind`, ADR 0030).
6. **A confirmação separa o que é seguro do que não é:** linhas "provável" vêm marcadas; "em dúvida" vêm **desmarcadas** com a
   melhor opção já escolhida; "nenhum" fica desmarcada. O roadmap diz "todas marcadas por padrão"; aqui a regra é a de
   IMV-041 (o que o sistema não tem certeza não é aplicado sem o usuário marcar).
7. **O resultado do lote vive só na memória do agente.** Os jogos já estão gravados; se o agente reiniciar antes da
   confirmação, a busca se perde e o usuário usa "Buscar metadados" no jogo. Persistir resultados exigiria migração.

---

## Requisitos de fases anteriores afetados

Cada um recebeu, na spec de origem, a anotação `· alterado em 2026-11-15 (docs/specs/phase-12-metadados-unificados, MET-NNN)`
ou `· substituído em ...`. Se esta spec não for aprovada, as anotações são desfeitas.

| Requisito | Fase | Situação | Por |
|---|---|---|---|
| MID-002 — Busca e matching | 7 | **substituído**: consulta única a todos os provedores, lista única, sem escolher provedor | MET-001..MET-009 |
| MID-003 — Metadados do jogo | 7 | alterado: o merge por campo é o mesmo; a entrada é o candidato unificado (`metadata.apply`) | MET-030..MET-033 |
| MID-004 — Mídia de imagem | 7 | alterado: limite por tipo contando as imagens atuais e as enfileiradas, em todo caminho da fila | MET-021, MET-022 |
| UIX-033 — Buscar metadados | 10 | alterado: sem escolha de provedor; "Nome para busca" + "Obter dados" | MET-043 |
| IMV-081 — A UI nunca influencia o tempo de jogo | 11 | alterado: a janela de importação também chama `Metadata*` | MET-080 |
| Roadmap, fase 12: "a 12 acrescenta [à janela de revisão] a coluna de match" | — | **superado pelo pedido do usuário (2026-11-15)**: a confirmação é uma janela própria, depois da importação | MET-051, MET-060 |
| IMV-004 (nota: "` - ` virar `: ` é trabalho da fase 12") | 11 | resolvido | MET-004 |
| ADR 0030 — "Clear Logo, Box 3D... ficam para a fase 12" | — | **adiado de novo** (sem tipo novo, sem migração; decisão 5) | MET-025 |

Contrato: `scrape.search`, `scrape.apply` e `media.queue` **continuam** na v0.14.0 (só acréscimos), marcados como obsoletos; a
UI deixa de usá-los. Eles passam pelo limite de imagens (MET-022).

Inalterados e ainda válidos: MID-001 (chaves), MID-005 (a fila é a mesma), MID-006, MID-008, MID-010, IMG-001..IMG-011
(`media.changed` e a atualização ao vivo), IPC-017 (buffer de 16 com descarte), IPC-050, IMV-040..IMV-052 (a janela de revisão
de títulos não muda), IMV-072, UIX-037..UIX-039, UIX-046.

---

## MET — Busca unificada

### MET-001 — Um pedido consulta todos os provedores aplicáveis
Dado `metadata.search` com título (1 a 200 caracteres), plataforma (nome, opcional) e ano (opcional), quando o agente atende,
então consulta **em paralelo** cada provedor aplicável (MET-003) que tem chave (MID-001; a Steam Store não precisa), junta os
resultados (MET-005), pontua (MET-006, MET-007) e devolve **uma** lista (MET-008) com a confiança (MET-009). Provedor sem chave ou
não aplicável não é consultado e não conta como falha. Dado que um provedor falha temporariamente (rede, 429, 5xx, MID-008),
então a lista vem dos demais e `warnings` diz qual falhou ("IGDB indisponível no momento"). A busca inteira tem prazo de 20 s
(medido pelo `domain.Clock`); o provedor que não respondeu até lá conta como falha temporária.
· verificação: teste do agente com provedores fake (todos respondem; um sem chave; um com falha temporária; um lento com
relógio fake), conferindo as chamadas feitas a cada fake e `warnings`.

### MET-002 — Erros da busca
Dado `metadata.search` sem **nenhum** provedor aplicável com chave, então a resposta é `-32010` (ProviderNoKey) com
`data.provider` = o primeiro provedor com chave que falta (`igdb`, depois `steamgriddb`), sem nenhuma chamada de rede. Dado que
**todos** os provedores consultados falham temporariamente, então é `-32011` (ProviderUnavailable) com `data.provider` do
primeiro que falhou. Dado que ao menos um respondeu, mesmo com lista vazia, então a resposta é sucesso. Dado título vazio,
com mais de 200 caracteres, plataforma com mais de 100 ou ano fora de 1900..2100 (0 e ausente = sem ano), então `-32602`
(MET-071).
· verificação: teste do agente (um caso por regra; chaves ausentes em plataforma de console; nenhuma chamada de rede).

### MET-003 — Provedores aplicáveis e filtro de plataforma
Dado a plataforma da busca resolvida por nome, sem criar nada (PLT-025, passos 1 e 2), então a aplicabilidade é a da tabela de
[match-rules §1](contracts/match-rules.md): `igdb` e `steamgriddb` aplicam a qualquer plataforma; `steamstore` aplica só quando a
chave de seed é `windows`, ou a plataforma não tem chave e a categoria é `pc`, ou a plataforma não foi informada ou não foi
achada. Dado que a plataforma tem chave de seed com id no IGDB (`scrapers.PlatformID`), então a consulta ao IGDB filtra por esse
id; se o filtro não devolve nada, o agente repete a consulta **sem** filtro e os resultados dessa segunda consulta ficam com a
plataforma "não confirmada" (trava de MET-007). Plataforma sem chave ou sem id: sem filtro e sem bônus de plataforma.
· verificação: teste do agente com `Platform` real do store (seed, renomeada, criada pelo usuário com categoria `pc` e
`console`, inexistente); teste do IGDB com `httptest` (corpo da consulta com e sem o filtro; segunda consulta).

### MET-004 — O termo enviado ao provedor
Dado o título da busca, então o termo enviado a todo provedor é: grupos `(...)` e `[...]` removidos, ™ ® © removidos, ` - `
trocado por `: ` e espaços repetidos reduzidos (`Legend of Zelda - Majora's Mask (USA)` → `Legend of Zelda: Majora's Mask`); o
título original nunca é alterado. Se o termo fica vazio, a resposta é `-32602`.
· verificação: teste de unidade de `domain.SearchTerm` (tabela de `match-rules.md` §5) e teste de que o fake recebe o termo.

### MET-005 — Juntar o mesmo jogo
Dado candidatos de provedores diferentes, então dois candidatos são o **mesmo jogo** quando são de provedores distintos e (a
chave de comparação, `domain.MatchKey`, é igual, ou a similaridade é ≥ 0,90 e as marcas do título, MET-007, são iguais) e (um dos anos é
desconhecido ou a diferença é de até 1 ano). Cada candidato entra no grupo mais parecido que ainda não tem o seu provedor; sem
grupo possível, abre um. Dois candidatos do mesmo provedor nunca se juntam. O grupo guarda uma referência (`provider`,
`providerId`) por provedor; título, ano e plataforma exibidos são os do primeiro membro na ordem igdb, steamstore, steamgriddb
(o ano é o primeiro não zero).
· verificação: teste de unidade do agregador com tabela (mesmo jogo em dois e em três provedores; remake com ano 6 anos
distante não junta; dois resultados do IGDB não se juntam; empate de similaridade).

### MET-006 — Pontuação
Dado um grupo, então `pontuação = min(1, similaridade + 0,05 se a plataforma confere + 0,05 se o ano confere + 0,02 por
provedor além do primeiro, no máximo 0,04)`, onde `similaridade` é a maior, entre os membros, da similaridade de Dice sobre
bigramas das chaves de comparação do título buscado e do título do membro (`domain.MatchKey`: minúsculas, sem acentos, sem
marcações, sem sinais, sem palavras de edição, sem `the`, `a`, `an` e `and`, algarismos romanos `ii`..`xx` como números; 1 quando
as chaves são iguais). A plataforma "confere" quando algum membro tem a plataforma da biblioteca entre as suas; o ano confere
quando é igual e ambos são conhecidos.
· verificação: teste de unidade de `domain.UnifiedScore` com os vetores de `match-rules.md` §2.

### MET-007 — Travas: marcas diferentes e plataforma não confirmada
Dado um grupo cujo membro de maior similaridade tem **marcas** diferentes das do título buscado (marcas = números, inclusive os
romanos `ii`..`xx`, e letras soltas como `x` e `v`, em ordem; `s`, `t`, `d` e `m` de apóstrofo não contam), **ou** cuja
plataforma é "não confirmada" (MET-003, segunda consulta) ou cujos membros com informação de plataforma não incluem a da
biblioteca, então a pontuação fica limitada a **0,79**: o grupo nunca é "provável" sozinho, mas continua na lista. Um membro sem
informação de plataforma (SteamGridDB) não dispara a trava.
· verificação: teste de unidade (`Resident Evil` × `Resident Evil 2`; `Final Fantasy VII` × `VIII` × `7`; `Mega Man X` ×
`Mega Man`; `Link's` × `Links`; plataforma não confirmada; membro só do SteamGridDB).

### MET-008 — Lista única
Dado o resultado, então a lista traz até 10 grupos com pontuação ≥ 0,40, em ordem de pontuação decrescente (empate: mais
provedores, depois o título). Cada item traz `refs` (uma por provedor), `title`, `year` (0 = desconhecido), `platform` ("" =
desconhecida; o nome da plataforma da biblioteca quando confere), `score` e `thumbUrl` (MET-068, opcional). A lista vazia é um
resultado válido ("Nenhum resultado").
· verificação: teste do agente (ordem, corte em 10, piso de 0,40, empate).

### MET-009 — Confiança do resultado
Dado a lista ordenada, então `confidence` é `match` quando o primeiro item tem pontuação ≥ 0,85, similaridade ≥ 0,80, nenhuma
trava (MET-007) e vantagem ≥ 0,05 sobre o segundo (ou não há segundo); `doubt` quando o primeiro tem pontuação ≥ 0,60 e não é
`match`; `none` caso contrário (inclusive lista vazia). A mesma regra classifica cada jogo do lote (MET-053). Os limiares estão
em `domain` como constantes (`MatchAuto`, `MatchMargin`, `MatchDoubt`, `MatchAlternative`, `MatchMinSimilarity`).
· verificação: teste de unidade de `domain.ClassifyMatch` (limites exatos 0,85, 0,849, 0,60, 0,599; vantagem 0,05 e 0,049; trava).

---

## MET — Provedores e limites de requisição

### MET-010 — Intervalo mínimo entre requisições, por provedor
· alterado em 2026-11-15 (docs/specs/phase-13-modo-console, VID-015): entra o ScreenScraper (só vídeo), com 1.500 ms ou mais pelo
limite por minuto da conta; o download do vídeo também passa pelo limitador dele.
Dado qualquer chamada de rede a um provedor, de qualquer origem (busca única, lote, aplicação), então ela espera até o próximo
horário livre do provedor: IGDB 334 ms, SteamGridDB 500 ms e Steam Store 2.000 ms depois da requisição anterior daquele
provedor (o relógio é `domain.Clock`; o limitador é um só por provedor no agente). Quem espera sai na hora quando o contexto é
cancelado. As chamadas de download de imagem (fila de mídia, MID-005) não passam por este limitador: são de CDN, não de API.
· verificação: teste do limitador com relógio fake (10 chamadas simultâneas saem espaçadas; cancelamento libera; provedores
independentes não se atrasam); teste do agente com `httptest` contando requisições por provedor.

### MET-011 — Falha temporária no lote
Dado uma busca em lote (MET-053), quando um provedor responde com falha temporária (429, 5xx, rede), então o agente conta a
falha **seguida** desse provedor: a 1ª suspende as chamadas a **esse** provedor por 30 s, a 2ª seguida as suspende por 60 s
(relógio fake nos testes) e a 3ª seguida o **desativa** até o fim daquele lote (não há suspensão de 120 s). Os jogos buscados
durante uma suspensão seguem com os demais provedores. Na desativação, o `warnings` do lote registra "IGDB indisponível: a
busca continuou sem ele" uma só vez. Uma resposta bem-sucedida zera a contagem. Um jogo cuja busca falhou em todos os
provedores aplicáveis fica com `confidence = none`.
· alterado em 2026-11-15: a regra se contradizia ("30 s, depois 60 s, depois 120 s" e "3 falhas seguidas desativam"); vale 1ª = 30 s, 2ª = 60 s, 3ª = desativa, sem degrau de 120 s.
· verificação: teste do job com provedor fake que falha N vezes e relógio fake (1ª falha: suspenso por 30 s e liberado em 30 s;
2ª: 60 s; 3ª: desativado, sem retorno no lote; sucesso entre falhas zera).

### MET-012 — IGDB: texto, capa, screenshots e fundo numa só requisição
Dado uma busca ou um detalhe do IGDB, então **uma** requisição devolve os candidatos já com o que `scrape.apply` precisaria:
descrição, gêneros, modos, empresas, ano, **capa** (`cover.image_id`, tipo `cover`), screenshots (`screenshots.image_id`, tipo
`screenshot`) e artes (`artworks.image_id`, tipo `background`). Dado a aplicação de um jogo da busca em lote (MET-065), então o IGDB não recebe nenhuma
requisição nova, porque o grupo escolhido guarda o que a busca trouxe; dado `metadata.apply` avulso, que recebe só `refs`, então
o IGDB recebe **uma** requisição por id com os mesmos campos. A consulta filtra por jogo principal (sem DLC nem pacote) e, com
plataforma, por `platforms` (MET-003).
· verificação: teste do IGDB com `httptest` (corpo da consulta; candidato com mídia embutida; uma requisição por id no apply
avulso; nenhuma na aplicação do lote).

### MET-013 — SteamGridDB: capa em retrato e fundo
Dado um jogo achado no SteamGridDB, quando se pedem as imagens, então são duas requisições: capas (`grids`, só estáticas e em
dimensões de retrato, tipo `cover`) e heróis (`heroes`, tipo `background`). Não há screenshot de gameplay (decisão 5). Sem chave,
nada é pedido (MID-001).
· verificação: teste do SteamGridDB com `httptest` (URLs e parâmetros; tipos devolvidos; ausência de `screenshot`).

### MET-014 — Steam Store: uma requisição
Dado um jogo da Steam Store, então `appdetails` é pedido **uma vez** para texto e screenshots (hoje `Details` e `Media`
repetem a requisição); a miniatura da linha vem do `appid`, sem requisição.
· verificação: teste da Steam Store com `httptest` contando requisições (uma por aplicação).

### MET-015 — Steam Store escalonada no lote
Dado uma busca em lote de um jogo com a Steam Store aplicável, então a Steam Store só é consultada se, depois dos outros
provedores, a confiança do jogo não é `match`, ou se não há outro provedor com chave. Na busca única (MET-001) ela é sempre
consultada quando aplicável.
· verificação: teste do job (IGDB acha `match` → zero chamadas à Steam Store; IGDB acha `doubt` → uma; sem chave do IGDB → uma).

---

## MET — Imagens e limite por tipo

### MET-020 — Prioridade entre provedores, por tipo
Dado imagens oferecidas por mais de um provedor, então a escolha por tipo segue a ordem de [match-rules §4](contracts/match-rules.md):
`cover` — Windows: SteamGridDB, IGDB; demais plataformas: IGDB, SteamGridDB; `screenshot` — IGDB, Steam Store; `background` —
SteamGridDB, IGDB. A mesma URL oferecida duas vezes entra uma vez. Os tipos `title`, `select` e `banner` nenhum provedor oferece
(só o LaunchBox, MID-010).
· verificação: teste de unidade de `scrapers.SelectImages` (ordem por tipo e por classe de plataforma; URL repetida).

### MET-021 — O limite conta as imagens atuais e as enfileiradas
· alterado em 2026-11-15 (capa = 1): `MediaKindLimit("cover")` passou de 2 para 1; a fórmula não muda.
Dado um jogo e um tipo, então o número de imagens novas escolhidas é `max(0, domain.MediaKindLimit(tipo) − imagens atuais do tipo −
jobs pendentes ou em andamento do tipo)`. Jobs concluídos já contam nas imagens atuais; jobs falhos ou cancelados não contam.
Imagens de qualquer origem (LaunchBox, provedores) contam; nada é apagado.
· verificação: teste de unidade de `SelectImages` (jogo vazio; com 9 de 10 screenshots; com 1 capa; com jobs pendentes);
teste do agente com store real.

### MET-022 — Todo caminho da fila passa pelo limite
Dado `metadata.apply`, a aplicação do lote, `scrape.apply` e `media.queue`, então todos enfileiram só as imagens que o limite de
MET-021 permite, pela mesma função (`queueImages`). `media.queue` com tipos explícitos limita cada tipo pedido.
· verificação: teste do agente (um caso por caminho, mesmo jogo com 1 vaga de screenshot).

### MET-023 — Dois pedidos para o mesmo jogo não passam do limite
Dado dois pedidos que enfileiram imagens do mesmo jogo ao mesmo tempo (dois `metadata.apply`, ou um deles do lote), então a
contagem de MET-021 e o enfileiramento acontecem juntos, serializados por jogo, de modo que a soma enfileirada não passa do
limite. Jogos diferentes não se bloqueiam.
· verificação: teste do agente com `-race` (dois pedidos concorrentes do mesmo jogo; total ≤ limite).

### MET-024 — O resultado diz o que entrou e o que ficou de fora
Dado `metadata.apply`, então o resultado traz `queued` (imagens enfileiradas) e `skippedByLimit` (imagens oferecidas que
ficaram de fora porque o tipo estava cheio). O diálogo mostra "N imagens na fila; M não entraram porque o jogo já tem o
máximo".
· verificação: teste do agente (contadores) e de componente (textos).

### MET-025 — Só os tipos que existem
· alterado em 2026-11-15 (docs/specs/phase-13-modo-console, VID-001): vale para a fase 12; a fase 13 cria o tipo `video`
(migração 9).
Dado a fase, então nenhum tipo de mídia novo é criado: sem migração, `media.kind` continua `cover`, `screenshot`, `title`,
`banner`, `background`, `select`. Logos, ícones e caixa 3D ficam fora (decisão 5, ADR 0030).
· verificação: revisão do diff (nenhum arquivo em `internal/store/migrations`).

---

## MET — Aplicar um candidato

### MET-030 — `metadata.apply` grava texto por prioridade e enfileira as imagens
· alterado em 2026-11-15 (docs/specs/phase-13-modo-console, VID-004): depois de aplicar, o agente enfileira a busca de vídeo do
jogo quando há credenciais do ScreenScraper, sistema mapeado e vaga; o resultado não muda.
Dado `metadata.apply` com `gameId`, `refs` (uma por provedor, MET-031) e `replace`, quando o agente atende, então obtém de cada
provedor o texto e as imagens (uma requisição ao IGDB, uma à Steam Store e duas ao SteamGridDB, MET-012..MET-014), faz o merge por campo
com a prioridade de MID-003 (descrição, gêneros, modos, empresas e ano: IGDB, depois Steam Store) preenchendo só campos vazios,
salvo `replace`, e enfileira as imagens (MET-020..MET-022). O título do jogo nunca muda.
· verificação: teste do agente com store real e provedores fake (campos vazios preenchidos; campo do usuário preservado; `replace`).

### MET-031 — Referências validadas
Dado `refs` com 1 a 3 itens, então cada `provider` é `igdb`, `steamgriddb` ou `steamstore`, sem repetir provedor, e cada
`providerId` tem de 1 a 20 dígitos; qualquer outra coisa é `-32602` sem nenhuma chamada. Provedor com chave ausente é `-32010`.
Provedor com falha temporária é `-32011`.
· verificação: teste do agente (um caso por regra).

### MET-032 — Falhou antes de gravar, o jogo fica como estava
Dado `metadata.apply`, então todos os provedores são consultados antes de qualquer escrita; se um falha, o jogo não muda e nada
entra na fila (como `scrape.apply`). Dado `gameId` inexistente, então `-32001`.
· verificação: teste do agente (falha do segundo provedor; jogo e fila intactos).

### MET-033 — Sem aviso próprio
Dado `metadata.apply`, então a chamada não envia nenhuma notificação (IPC-050); as imagens chegam pelo `media.changed` da fila
(IMG-001). O diálogo invalida a leitura do jogo e da lista como o `ScrapeApply` fazia (IMG-011).
· verificação: teste do agente (nenhuma notificação antes de um job gravar).

---

## MET — Arquivo avulso e diálogo do jogo

### MET-040 — Nome para busca sugerido
Dado a fonte "Arquivo" com um arquivo, então o candidato traz `searchName`: o título normalizado do nome do arquivo
(`importers.NormalizeTitle`, IMV-001..IMV-005), sem sufixos de motor (`-Win64-Shipping`, `-x64` etc.); se o nome é genérico
(`game`, `launcher`, `start`, `play`, `run`, `main`, `app`, `client`...) ou fica vazio, o nome da pasta pai normalizado
(IMV-006); se a pasta é genérica (`bin`, `binaries`, `win64`, `release`...), sobe até 3 pastas acima do arquivo, sem passar da raiz do
disco. As listas e os vetores estão em [match-rules §5](contracts/match-rules.md). As outras fontes não trazem o campo.
· alterado em 2026-11-15: "até 2 níveis" divergia de match-rules §5.2 (até 3 pastas).
· verificação: teste do importador (tabela de `match-rules.md` §5) e teste de contrato do IPC (campo opcional).

### MET-041 — "Nome para busca" e "Obter dados" na importação de arquivo
Dado a fonte "Arquivo" com o candidato lido, então a janela mostra, abaixo da tabela, o campo "Nome para busca" (preenchido com
`searchName`, editável) e o botão "Obter dados". Ao tocar, a janela chama `metadata.search` com esse nome, a plataforma escolhida
(IMV-063) e sem ano, e mostra a lista de opções (MET-044) com a opção "Nenhuma" (marcada por padrão). A linha de confiança
`match` vem com a primeira opção pré-selecionada. Trocar o nome, o arquivo ou a plataforma descarta a lista e a escolha (como
IMV-049). O botão fica desativado durante a busca e sem plataforma.
· verificação: teste de componente (`ImportDialog` com fake de `MetadataSearch`; pré-seleção; descarte; estados do botão).

### MET-042 — Aplicar depois de importar
Dado o candidato de arquivo com uma opção escolhida (MET-041), quando o usuário importa, então a janela primeiro importa
(`import.apply`) e, com o `gameId` do resultado, chama `metadata.apply` com as `refs` da opção e `replace = false`. Se a
importação recusa o jogo, ou se `metadata.apply` falha, a janela mostra o aviso ("Jogo importado; os dados não foram aplicados:
use Buscar metadados no painel") e a importação **não** é desfeita. Sem opção escolhida, nada é chamado.
· verificação: teste de componente (ordem das chamadas; falha do apply; recusa do import).

### MET-043 — Diálogo "Buscar metadados" do jogo
Dado G em foco e o diálogo aberto (UIX-033), então ele não tem mais a escolha de provedor: tem o campo "Nome para busca"
(título de G, editável), o botão "Obter dados", a caixa "Substituir os campos já preenchidos" e a lista de opções (MET-044)
com "Aplicar" por item. "Obter dados" chama `metadata.search` com o nome digitado, a plataforma e o ano de G (se conhecidos).
"Aplicar" chama `metadata.apply` e mostra o resultado de MET-024; as invalidações de IMG-011 continuam.
· verificação: teste de componente (campo e botão; sem `select` de provedor; chamada com plataforma e ano; aplicar; `replace`).

### MET-044 — A lista de opções
Dado a lista de MET-008, então cada opção mostra a capa pequena (MET-068), o título, o ano entre parênteses se conhecido, a
plataforma ("Plataforma desconhecida" se não), os provedores que a trouxeram ("IGDB · SteamGridDB"), a correspondência em
percentual e, só no primeiro item quando a confiança é `match`, o selo "provável". Lista vazia mostra "Nenhum resultado para
este nome." Os avisos de provedor (MET-001) aparecem acima da lista.
· verificação: teste de componente (`MetadataOptions`: itens, selo, vazio, avisos).

---

## MET — Busca em lote na importação

### MET-050 — A caixa "Buscar metadados e imagens"
Dado a fonte "Pasta", então a janela de importação tem a caixa "Buscar metadados e imagens", **marcada por padrão**. A caixa
fica desativada, com a mensagem "Cadastre a chave do IGDB ou do SteamGridDB em Configurações.", quando nem o IGDB nem o
SteamGridDB têm chave (`provider.keyStatus` das duas chaves; ambos aplicam a qualquer plataforma, MET-003). A Steam Store
**não tem chave** e não conta como "provedor com chave": só ela disponível não ativa a caixa. A caixa também fica desativada,
com "Há metadados para revisar. Revise ou descarte antes de buscar de novo.", quando existe uma busca em lote em `ready` ou
`cancelled` (MET-052). Desmarcar a caixa importa como na fase 11, sem busca. As outras fontes não têm a caixa.
· alterado em 2026-11-15: esclarecido que a Steam Store (sem chave) não conta para "provedor com chave".
· verificação: teste de componente (marcada; sem chaves; busca pendente; fonte Steam).

### MET-051 — A busca começa depois da importação, sobre o que entrou
Dado "Importar selecionados" com a caixa marcada, quando os lotes de IMV-048 terminam e ao menos um jogo foi gravado, então a
janela chama `metadata.batchStart` com os `gameId` do resultado (`applied[].gameId`, em ordem), sem passo manual. A busca usa
o título **gravado** (o que o usuário revisou na janela de IMV-040..IMV-048). A importação não espera pela busca: o resumo
"Importados: N" aparece e a biblioteca recarrega (`onImported`) antes. Falha de `metadata.batchStart` (sem chave, outra busca em
andamento) aparece como aviso na janela e não desfaz nada. Lote de importação interrompido por falha: busca só dos jogos gravados.
Depois de um `batchStart` bem-sucedido feito pelo `ImportDialog`, o `App` relê o estado da busca (`MetadataBatchStatus`; o
`ImportDialog` avisa por uma prop `onBatchStarted`), de modo que o indicador (MET-067) mostre "Buscando metadados 0/n" na hora,
sem esperar pela primeira notificação.
· alterado em 2026-11-15: releitura do estado depois do `batchStart` (`onBatchStarted`).
· verificação: teste de componente (ordem: lotes, `onImported`, `batchStart` com os ids, `onBatchStarted` só no sucesso; sem
jogos gravados → sem chamada; falha do start sem `onBatchStarted`; `App` relê o estado e o indicador mostra 0/n).

### MET-052 — `metadata.batchStart`
Dado `metadata.batchStart` com 1 a 1.000 `gameIds` distintos, então o agente cria **uma** busca em lote, devolve `batchId` e
`total` na hora e começa em segundo plano (estado `searching`). Dado `gameId` inexistente, então `-32001` e nada começa. Dado
uma busca em `searching`, `ready`, `cancelled` ou `applying`, então `-32015` (BatchBusy); em `done` ou sem busca, a nova
substitui a antiga. Dado que nem o IGDB nem o SteamGridDB têm chave, então `-32010` (`ProviderNoKey`, `data.provider` = `igdb`),
mesmo que a Steam Store fosse aplicável: ela não tem chave e não conta como "provedor com chave" (como em MET-050). Mais de
1.000 ids ou ids repetidos: `-32602`.
· alterado em 2026-11-15: `ProviderNoKey` definido pela falta das duas chaves (a Steam Store não conta).
· verificação: teste do agente (um caso por regra; sem as duas chaves e jogo de Windows → `-32010`; a chamada volta antes de
qualquer requisição de rede).

### MET-053 — Execução da busca
Dado a busca em `searching`, então 3 trabalhadores (`batchWorkers`) pegam os jogos na ordem recebida; para cada jogo o agente lê
título, plataforma e ano gravados, consulta os provedores aplicáveis em paralelo (MET-001, MET-003, MET-004 e MET-015, com prazo de 15 s por jogo em vez dos 20 s da
busca única), junta, pontua e classifica (MET-005..MET-009) e guarda uma linha com a confiança, o item
selecionado (o primeiro, ou nenhum) e até 5 alternativas com pontuação ≥ 0,40. Todas as chamadas passam pelos limitadores de
MET-010. Quando todos os jogos têm linha, o estado vai a `ready`. Jogo apagado no meio da busca vira linha `none` sem
alternativas.
· verificação: teste do job com provedores fake e relógio fake (ordem, classificação por jogo, limite de chamadas, jogo
apagado, plataforma de cada jogo na consulta).

### MET-054 — Progresso por notificação
Dado a busca ou a aplicação em andamento, então o agente envia a notificação `metadata.batchChanged` com
`{batchId, state, done, total, failed}` (só inteiros e o estado, sem dados de jogo, ADR 0031): na hora a cada mudança de
estado, e, para mudanças só de contador, uma por janela de 500 ms (a primeira sai na hora e as seguintes se juntam numa no fim
da janela; sem pendência, nenhum temporizador no relógio). A chamada da própria UI (`batchStart`, `batchApply`, `batchCancel`,
`batchDiscard`) não gera notificação por si (IPC-050); as transições feitas em segundo plano (`searching → ready`,
`applying → done`) e o progresso, sim. Entrega best effort (IPC-017).
· verificação: teste do job com relógio fake e dois clientes IPC reais (tabela de cenários do agrupador, como IMG-003; nenhuma
notificação por chamada da UI; estado final sempre avisado).

### MET-055 — Estado por consulta
Dado `metadata.batchStatus`, então a resposta traz a busca atual (`batchId`, `state`, `total`, `done`, `failed`, `warnings` de
até 10 textos e, em `done`, o `result`) ou `null` quando não há. A UI chama no início, ao reconectar (UI-009) e ao receber uma
notificação de mudança de estado; é a rede de segurança para notificações descartadas.
· verificação: teste do agente (cada estado) e de componente (reconexão relê o estado).

### MET-056 — Cancelar a busca
Dado `metadata.batchCancel` com a busca em `searching`, então ela vai a `cancelled` na hora, nenhuma requisição nova começa, as
em andamento terminam sem gravar nada e os trabalhadores encerram (o agente espera por eles no desligamento); as linhas dos
jogos já buscados ficam disponíveis para revisão. Em `ready` ou `cancelled` é um no-op que devolve o estado. Em `applying` vale
MET-066.
· verificação: teste do job (cancelar no meio com relógio fake; contagem de requisições depois do cancelamento = 0; sem
goroutine restante).

### MET-057 — Resultados em páginas que cabem na mensagem
Dado `metadata.batchResults` com `offset` (≥ 0) e `limit` (1 a 200, padrão 100), então o agente devolve as linhas na ordem dos
jogos, cada uma com `gameId`, `title`, `platform`, `confidence`, `selected` (índice do item escolhido ou -1) e `candidates`, mais
`total` e `next` (próximo `offset` ou `null`). A resposta tem no máximo 900 KiB (`x-metadata.maxResultBytes`, 921.600, ADR 0032):
o agente devolve menos linhas que `limit` quando preciso, nunca uma linha cortada, e `next` aponta para a primeira que não coube.
Só vale em `ready` e `cancelled`; em outro estado `-32016` (BatchWrongState).
· verificação: teste do agente (500 linhas com títulos de 200 caracteres e 5 alternativas; cada página ≤ limite; união das
páginas = todas as linhas; estado errado).

### MET-058 — Uma busca por vez, só em memória
Dado o agente, então existe no máximo uma busca em lote; ela vive na memória (nada vai ao banco), `batchDiscard` a libera em
`ready`, `cancelled` ou `done` (em `searching`/`applying` é `-32016`) e reiniciar o agente a perde. Um `batchId` desconhecido ou
já descartado é `-32014` (BatchNotFound). O `batchId` é o horário de criação em milissegundos (`domain.Clock`), estritamente
crescente.
· verificação: teste do agente (descartar; id desconhecido; id crescente com relógio parado; nada gravado no banco).

### MET-059 — Donos das goroutines e desligamento
Dado a busca ou a aplicação em andamento, quando o agente encerra, então o contexto cancela, os trabalhadores e o coordenador
terminam, o agente espera por eles (`sync.WaitGroup`, CLAUDE.md) e nenhuma requisição nova é feita. Nenhuma goroutine nasce fora do
job.
· verificação: teste do agente com `-race` e `goleak`-equivalente (contagem de goroutines antes e depois), como os testes da fila.

---

## MET — Janela de confirmação e indicador

### MET-060 — A janela de confirmação
Dado a busca em `ready` (ou `cancelled` e o usuário pede para revisar), então a janela "Confirmar metadados" (modal grande,
UIX-037) mostra uma tabela, uma linha por jogo, com cabeçalho fixo e as colunas: caixa de seleção; **Capa** (MET-068);
**Jogo** (o título gravado e a plataforma); **Encontrado** (o item selecionado: título, ano e plataforma); **Situação**
("provável", "em dúvida" ou "nenhum"); e uma lista de **Alternativas** (select). As linhas vêm de `metadata.batchResults`,
página a página (MET-057), e a janela mostra "k de n jogos" enquanto carrega. O corpo rola e título e botões ficam fixos
(UIX-038, UIX-039); cores só pelos tokens `--arc-*` (UIX-006), sem token novo.
· verificação: teste de componente (colunas; carga em páginas; estrutura cabeçalho/corpo/rodapé); varredura de UIX-006.

### MET-061 — Marcação padrão
Dado as linhas carregadas, então `match` vem marcada, `doubt` vem desmarcada com o primeiro item já selecionado, e `none` vem
desmarcada e **sem caixa ativa** enquanto não há item selecionado. Escolher um item nas alternativas de uma linha marca a linha;
escolher "Nenhum" a desmarca e bloqueia.
· verificação: teste de componente (15 linhas: 9 provável, 4 dúvida, 2 nenhum → 9 marcadas; trocar alternativa marca; Nenhum
desmarca).

### MET-062 — Trocar a alternativa
Dado uma linha com alternativas, quando o usuário escolhe outra no select, então a coluna "Encontrado" e a capa mostram a
escolhida, a situação da linha passa a "escolhido por você" (a pontuação continua visível como porcentagem), e a escolha é a que
será aplicada. O select lista as alternativas na ordem do agente, com título, ano e plataforma, e a opção "Nenhum".
· verificação: teste de componente (troca; situação; opção Nenhum).

### MET-063 — Selecionar todos e contador
Dado a tabela, então o cabeçalho tem "Selecionar todos" (marca todas as linhas com item selecionado; indeterminado quando
parcial; desativado sem linha marcável ou durante a aplicação) e o texto "k de n jogos selecionados" (n = linhas com item
selecionado; k = marcadas), mais " (j sem correspondência)" com j > 0 linhas `none`.
· verificação: teste de componente (três estados da caixa; contador).

### MET-064 — Aplicar selecionados
Dado "Aplicar selecionados" (ativo com k ≥ 1 e fora de uma aplicação), quando o usuário confirma, então a janela chama
`metadata.batchApply` com `choices = [{gameId, candidate}]` (índice do item na lista da linha) só das linhas marcadas e fecha. A
chamada devolve na hora `accepted` e o estado vai a `applying`; o resto acontece em segundo plano (MET-065). Dado `gameId` fora
da busca ou `candidate` fora da lista da linha, então `-32602`; busca fora de `ready`/`cancelled`, `-32016`; mais de 1.000
escolhas, `-32602`. "Descartar tudo" chama `metadata.batchDiscard`.
· verificação: teste de componente (escolhas enviadas; janela fecha; descartar); teste do agente (um caso por erro).

### MET-065 — Execução da aplicação
· alterado em 2026-11-15 (docs/specs/phase-13-modo-console, VID-004): cada jogo aplicado também ganha a busca de vídeo nas
condições de VID-004.
Dado a busca em `applying`, então um único trabalhador aplica as escolhas em ordem: para cada jogo, obtém de cada provedor da
escolha o que falta (MET-012, MET-013, MET-014; sempre `replace = false`), grava o texto (MET-030) e enfileira as imagens
(MET-020..MET-023). Falha de um jogo (provedor fora, jogo apagado) incrementa `failed`, guarda o título (até 10) e segue para o
próximo. Ao fim, estado `done` com `result {applied, imagesQueued, imagesSkippedByLimit, failedTitles}`. As imagens chegam
depois pela fila (MID-005) e pelo `media.changed` (IMG-001); a aplicação não espera por elas.
· verificação: teste do job com store real, fila fake e provedores fake (um jogo falha; contadores; nenhuma espera pela fila).

### MET-066 — Cancelar a aplicação e o resumo
Dado `metadata.batchCancel` com a busca em `applying`, então nenhum jogo novo começa, o jogo em curso termina, o que já foi
aplicado fica, e o estado vai a `done` com o `result` parcial, que inclui **todo jogo já gravado** (`applied`,
`imagesQueued`, `imagesSkippedByLimit` e `failedTitles` contam o jogo em curso que terminou e os anteriores). Ao entrar em `done`, a UI mostra o resumo ("Metadados aplicados em
N jogos; M imagens na fila; K falharam") e invalida a lista de jogos e o catálogo (leituras; gêneros e descrições mudaram),
sem tocar em sessão. O resumo some com "Fechar", que chama `batchDiscard`.
· alterado em 2026-11-15: o resultado parcial inclui todo jogo já gravado.
· verificação: teste do job (cancelar no meio; resultado parcial com `applied` = jogos gravados, incluindo o em curso) e de componente (resumo; invalidações; fechar → descarta).

### MET-067 — Indicador de tarefa e abertura automática
· alterado em 2026-11-15 (docs/specs/phase-13-modo-console, BBX-022): com o Big Box ativo a janela de confirmação não abre
sozinha; o indicador pisca como com outro modal aberto.
Dado qualquer estado da busca diferente de "sem busca", então o cabeçalho da janela principal mostra o indicador
"Buscando metadados k/n" (`searching`, com "Cancelar"), "Revisar metadados (n)" (`ready` e `cancelled`, com "Descartar"),
"Aplicando metadados k/n" (`applying`, com "Cancelar") ou o resumo (`done`). Dado a transição para `ready` com nenhum modal
aberto, ou só a janela de importação aberta e sem operação em curso, então a janela de confirmação abre sozinha (a de importação
fecha antes); com outro modal aberto, só o indicador muda e recebe **um destaque único** (animação ou realce que roda uma vez e não se
repete enquanto o estado não muda). O indicador reflete o estado que a UI leu (MET-055); dado que a UI achava ter uma busca
e o agente responde `null` (reiniciou), então mostra o aviso "A busca de metadados foi interrompida. Use Buscar metadados no
jogo." e ele **some quando a UI relê o estado depois de exibi-lo** (a leitura seguinte que também devolve `null` não o mostra
de novo, pois a UI já não achava ter uma busca).
· alterado em 2026-11-15: "uma vez" e "pisca uma vez" definidos.
· verificação: teste de componente (`App` com fake de eventos e de `MetadataBatchStatus`: cada estado; abertura automática com
e sem outro modal; destaque único com outro modal; agente reiniciado: o aviso aparece e some na releitura seguinte).

### MET-068 — A imagem pequena da linha
Dado um item com `thumbUrl`, então a UI mostra a imagem pequena (`<img>`, `alt=""`, `loading="lazy"`,
`referrerPolicy="no-referrer"`) e, se ela falha ou não há `thumbUrl`, o placeholder de capa. O agente só devolve `thumbUrl` `https`
de `images.igdb.com`, `cdn.cloudflare.steamstatic.com` ou `cdn2.steamgriddb.com`; qualquer outro vira vazio. A imagem pequena é
só uma pré-visualização: nada é baixado para a biblioteca por ela, e a UI carregar uma imagem remota não influencia o jogo
(MET-080).
· verificação: teste do agente (hosts permitidos e recusados; esquema `http`) e de componente (falha → placeholder).

### MET-069 — Teclado, acessibilidade, tema e tamanho
Dado a janela de confirmação e a lista de opções, então: a tabela é uma `<table>` com `<th scope="col">`; cada caixa tem nome
acessível "Selecionar <título do jogo>"; cada select, "Alternativas de <título do jogo>"; Tab percorre caixa e select de cada
linha; Espaço alterna a caixa; a lista de opções (MET-044) é um grupo de rádios com nomes "<título> (<ano>), <provedores>"; Esc
fecha a janela de confirmação (UIX-039) sem descartar a busca, salvo durante uma chamada em curso.
· verificação: teste de componente (papéis, nomes, Tab, Espaço, Esc).

---

## MET — Contrato e binding

### MET-070 — Contrato IPC v0.14.0
Dado o contrato IPC, então ele está na versão 0.14.0 com apenas acréscimos sobre a v0.13.0: métodos `metadata.search`,
`metadata.apply`, `metadata.batchStart`, `metadata.batchStatus`, `metadata.batchResults`, `metadata.batchApply`,
`metadata.batchCancel` e `metadata.batchDiscard`; a notificação `metadata.batchChanged`; o campo opcional
`ImportCandidate.searchName`; os erros `BatchNotFound` (-32014), `BatchBusy` (-32015) e `BatchWrongState` (-32016); o bloco
`x-metadata` com as constantes do agente. `scrape.search`, `scrape.apply` e `media.queue` ganham a marca de obsoletos na
descrição, sem mudar parâmetro, resultado ou erro. Nenhum método, parâmetro obrigatório, resultado ou erro existente muda.
`internal/ipc` expõe os tipos e os nomes JSON do delta.
· verificação: teste de contrato de `internal/ipc` (versão, métodos, campos, enums, erros, constantes iguais às do agente e de
`domain`) e o teste "só acréscimo" sobre a v0.13.0.

### MET-071 — Validação dos parâmetros
Dado os métodos novos, então cada regra de [ipc-v0.14.0-delta §3](contracts/ipc-v0.14.0-delta.md) é conferida antes de qualquer
leitura ou chamada de rede, e a violação é `-32602`. Em `metadata.batchResults` e `metadata.batchApply`, a validação dos
parâmetros (`limit`, `offset`, `choices` e seus campos que não dependem da busca) vem **antes** da checagem de estado: com
`limit` inválido ou `choices` vazio em estado errado, a resposta é `-32602`, não `-32016`. `limit` **ausente** vale 100; `limit`
**0 explícito** é `-32602` (ausente e 0 são diferentes). As violações: título vazio ou > 200 runas, plataforma > 100, ano fora de 1900..2100,
`refs` fora de 1..3 ou repetindo provedor, `providerId` fora de `^[0-9]{1,20}$`, `gameIds` fora de 1..1.000 ou repetidos,
`offset` < 0, `limit` fora de 1..200, `choices` fora de 1..1.000, `candidate` < 0, `batchId` < 1.
· alterado em 2026-11-15: ordem validação × estado; `limit` ausente = 100 e 0 explícito = `-32602`.
· verificação: teste do agente (um caso por regra; `limit` 0 explícito em `ready` → `-32602`; `limit` ausente → 100;
`limit` 0 e `choices` vazio em `searching` → `-32602`, não `-32016`).

### MET-072 — Binding do desktop e evento
Dado a UI, então o binding expõe `MetadataSearch`, `MetadataApply`, `MetadataBatchStart`, `MetadataBatchStatus`,
`MetadataBatchResults`, `MetadataBatchApply`, `MetadataBatchCancel` e `MetadataBatchDiscard`, e o supervisor repassa
`metadata.batchChanged` como o evento Wails `metadata:batch` com `{batchId, state, done, total, failed}`; params ausentes, JSON
inválido, `state` desconhecido ou inteiros fora da faixa registram um `Warn` ("metadata.batchChanged com parâmetros inválidos") e
não emitem nada (como IMG-005). O repasse não chama o agente nem muda o estado do supervisor. Erros ao usuário seguem UI-008.
· verificação: teste de `cmd/arcadia` (repasse nos dois sentidos; evento válido e inválidos; fake do agente).

---

## MET — Invariantes

### MET-080 — A UI nunca influencia o tempo de jogo
Dado as interações da fase (obter dados, escolher opção, marcar a caixa, importar, acompanhar o progresso, confirmar,
cancelar, descartar), então a UI só chama `MetadataSearch`, `MetadataApply`, `MetadataBatch*`, `ImportScan`, `ImportApply`,
`PickFile`, `PickFolder`, `ProviderKeyStatus` e leituras: nenhum lançamento, `ToggleSession`, pausa, retomada ou escrita de
sessão. O agente não toca em `internal/tracker` nem em sessões nesta fase, e a busca e a aplicação continuam enquanto há jogo
rodando sem alterar a sessão. O teste de invariantes de UIX-046 (`App.invariants.test.tsx`) ganha essas interações.
· verificação: `App.invariants.test.tsx`; teste do agente (busca e aplicação com sessão ativa: sessão, `play_session` e
`session.changed` intactos).

### MET-081 — Onde a fase mexe
Dado o diff da fase, então não há mudança em `internal/tracker`, em `cmd/fake-game`, em migrações nem em tabelas de sessão ou
tempo de jogo. A única mudança em `internal/store` é uma consulta nova, sem migração, que conta os jobs de mídia ativos de um
jogo por tipo (`sqlc generate`). As mudanças em Go ficam em `internal/domain` (chave, pontuação, termo), `internal/scrapers`
(agregador, limitador, seleção de imagens, provedores), `internal/importers` (nome para busca), `internal/agent` (serviço,
busca em lote, notificador, `queueImages`), `internal/ipc` (tipos) e `cmd/arcadia` (binding, evento); o resto é `frontend`.
· verificação: revisão do diff; teste de contrato do IPC.

### MET-082 — Sem rede e sem espera real nos testes
Dado a suíte da fase, então nenhum teste faz requisição de rede externa nem usa `time.Sleep`: provedores são fakes ou
`httptest`, e limitador, prazos, janela de avisos e reposição usam `domain.Clock` com relógio fake.
· verificação: revisão do diff; `go test ./... -race` sem internet.

### MET-083 — Ponta a ponta
Dado o Arcadia com chaves do IGDB e do SteamGridDB e a pasta real `E:\Games\emulators roms`, quando importo `ps1` com subpastas
e a caixa "Buscar metadados e imagens" marcada, então: os jogos entram e aparecem na biblioteca de imediato; o indicador mostra o
progresso; ao terminar a janela de confirmação abre sozinha com os "prováveis" marcados e os outros desmarcados; troco uma
alternativa, desmarco um jogo e aplico; capas e screenshots chegam aos cartões e ao painel conforme baixam, sem passar do limite
por tipo; repito com `n64` e com uma pasta de PC na plataforma Windows. Importo um `.exe` avulso, vejo o nome sugerido, tocar
"Obter dados" lista opções, escolho uma e importo: o jogo vem com os dados. Cancelo uma busca no meio e revejo o que veio.
· verificação: manual.

---

## Fora desta spec

- **ScreenScraper** (decisão 1) e **busca por hash de arquivo**.
- **Obter dados de vários jogos já na biblioteca** (seleção múltipla na grade): o lote é só da importação de pasta. O mesmo
  agente serve a isso depois (`batchStart` recebe ids); falta a interface.
- **Caixa de busca em lote para Steam, Epic e LaunchBox**: só a fonte "Pasta" a tem; o LaunchBox já traz descrição e imagens
  (MID-010) e Steam e Epic ficam para depois (uma linha de interface, pois o lote trabalha com `gameId`).
- **Persistir o resultado do lote** (decisão 7) e retomar busca interrompida.
- **Logos, ícones, caixa 3D e vídeo**, e **tipos de mídia novos** (MET-025).
- **Configurar limites de imagem, prioridades e limiares** pelo usuário.
- **Segunda consulta com o título sem subtítulo** quando a primeira vem vazia: o usuário refaz com outro nome em "Buscar
  metadados".
- **Substituir ou apagar imagens** que o jogo já tem (decisão 4).
- **Progresso individual dos downloads** (fora desde a 10.2).

## Limitações conhecidas

1. **Retro depende de IGDB e SteamGridDB.** Sem a chave do IGDB, só chegam capas (SteamGridDB) e nenhum texto no console.
2. **Títulos em outro idioma ou muito diferentes** do nome do provedor caem em "em dúvida" ou "nenhum"; o usuário refaz por
   "Buscar metadados" com outro nome.
3. **A trava de marcas é conservadora:** `Final Fantasy V` não casa com `Final Fantasy 5` (`v` e `x` sozinhos não viram
   algarismo romano, para não confundir `Mega Man X`); cai em "em dúvida".
4. **Um lote grande demora minutos** (decisão 3); o usuário continua usando o Arcadia, e a aplicação das escolhas também roda
   em segundo plano.
5. **A miniatura da linha vem de um host de terceiro** pelo navegador embutido; sem internet ela cai no placeholder.
6. **Jogo importado por outro caminho e já com 1 capa** não ganha capa nova (decisão 4; alterado em 2026-11-15 (capa = 1); era 2).

## Perguntas ao usuário

Cada uma tem uma recomendação, já usada nos requisitos acima.

1. **ScreenScraper** fica fora da fase 12? Recomendação: sim (decisão 1); reavaliar depois de ver a cobertura de retro do
   IGDB + SteamGridDB na conferência manual (MET-083).
2. **SteamGridDB sem screenshots:** aceitar capa + herói (fundo) no lugar de "screenshots de gameplay" (decisão 5)?
   Recomendação: sim; os screenshots vêm do IGDB e da Steam Store.
3. **Limiares** de "provável" 0,85 com margem 0,05, "em dúvida" a partir de 0,60 (decisão 2)? Recomendação: sim; ajustáveis
   em `domain` (constantes).
4. **Linhas "em dúvida" desmarcadas** na janela de confirmação, contra "todas marcadas" do roadmap (decisão 6)?
   Recomendação: desmarcadas; marcar à mão é um clique, e metadados errados em 30 jogos são trabalho para desfazer.
5. **O limite de imagens conta as atuais e as enfileiradas** e nada é apagado (decisão 4)? Recomendação: sim. Alternativa:
   limitar só o que entra por busca, ignorando as atuais (pode chegar a 12 capas após buscas repetidas; alterado em 2026-11-15 (capa = 1): o limite de capa agora é 1, o exemplo de 12 permanece só como ilustração do crescimento sem controle).
6. **Caixa "Buscar metadados e imagens" marcada por padrão**, só na fonte "Pasta" (MET-050)? Recomendação: marcada (o pedido é que
   a busca "não seja um passo que o usuário precise lembrar"), e só em Pasta nesta fase.
7. **Prioridade da capa:** Windows com SteamGridDB primeiro; consoles com IGDB (arte oficial da caixa) primeiro
   (MET-020)? Recomendação: sim. Alternativa: SteamGridDB sempre primeiro, como em MID.
8. **Limites de requisição** 3/s, 2/s e 1 a cada 2 s (decisão 3)? Recomendação: sim, conservadores; um lote de 1.000 leva
   uns 6 a 9 minutos de busca.
9. **Janela de confirmação abre sozinha** quando a busca termina e nenhum outro modal está aberto (MET-067)? Recomendação: sim;
   alternativa: só o indicador no cabeçalho.
10. **Resultado do lote só em memória** (decisão 7; se o agente reiniciar, perde)? Recomendação: aceitar nesta fase.

## ADRs propostos

A sessão principal escreve (a spec só os referencia). Próximo número livre: 0033.

1. **0033 — Busca unificada de metadados com agregador por grupos, pontuação com travas e confirmação em lote:** o agente
   consulta todos os provedores aplicáveis à plataforma (pela chave de seed), junta o mesmo jogo, pontua por título normalizado +
   plataforma + ano com travas de número e de plataforma, classifica em provável/em dúvida/nenhum com limiares do domínio, e o
   lote roda como uma única tarefa em memória (busca → revisão → aplicação) com progresso por notificação de contadores
   (ADR 0031), cancelamento, limitador por provedor e resultado paginado dentro do teto do IPC (ADR 0032). Os métodos por
   provedor da fase 7 ficam obsoletos.
2. **0034 — Limite de imagens por tipo contando as atuais e as enfileiradas, aplicado em todo caminho da fila:** estende o
   ADR 0030 (que contava as atuais só na cópia do LaunchBox) a toda inclusão na fila de mídia, com seleção por prioridade entre
   provedores, serialização por jogo e uma consulta nova de jobs ativos (sem migração).
