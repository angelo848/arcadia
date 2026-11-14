# Fase 5 — Biblioteca: requisitos

- **Status:** aprovado pelo usuário (2026-11-12)
- **Design:** [design.md](design.md) · **Contratos:** [contracts/ipc.openrpc.json](contracts/ipc.openrpc.json) (v0.6.0,
  substitui a v0.5.0 de [phase-4-pause](../phase-4-pause/contracts/ipc.openrpc.json) como fonte da verdade) ·
  **Tarefas:** [tasks.md](tasks.md) (placeholder; o planner gera depois da aprovação)
- **Base:** fases 2, 3 e 4 implementadas ([phase-2-skeleton](../phase-2-skeleton/requirements.md),
  [phase-3-tracker](../phase-3-tracker/requirements.md), [phase-4-pause](../phase-4-pause/requirements.md)) e executável
  único ([single-exe](../single-exe/requirements.md)).
- **Objetivo:** transformar a lista mínima em uma biblioteca: grade virtualizada (ou lista), filtros e busca por texto
  com o mesmo `FilterSpec` que servirá às playlists inteligentes, página do jogo com as sessões, edição de metadados e
  remoção de jogo com confirmação. Toda escrita passa pelo agente por IPC; a UI não abre o banco para escrever.
- **ADRs relacionados:** 0009 (instantes em ms UTC), 0010 (só o agente migra), 0011 (Clock), 0012 (enums validados no
  `domain`, sem `CHECK`), 0015 (agente único escritor), 0019 (lançamento sem rastrear). **ADRs a escrever pela sessão
  principal:** ver [ADRs propostos](#adrs-propostos).

Formato: cada requisito tem ID estável (`ÁREA-NNN`), nunca reutilizado. Critérios em Dado/Quando/Então.
Todo ID precisa de pelo menos um teste que o cite no nome (`t.Run("FLT-003/...")`, `it("UI-033 ...")`), exceto os
marcados `· verificação: manual`. Áreas novas: **FLT** (filtros), **FTS** (índice de busca), **LIB** (listagem e
ordenação), **EDT** (edição), **DEL** (remoção). Áreas existentes continuam: **DOM**, **STO**, **IPC**, **DSK**, **UI**.

---

## Perguntas para o usuário

Três itens de comportamento de produto que não foram respondidos. A spec já usa a recomendação indicada; se o usuário
discordar, o requisito muda antes da aprovação.

1. **Remover jogo apaga as sessões e as pausas? (recomendação: sim).** Remover apaga o jogo e, em cascata, suas sessões e
   pausas (DEL-001). A confirmação da UI diz isso com todas as letras (UI-041). Manter sessões órfãs não tem uso (nenhuma
   tela as mostraria) e um histórico separado ("lixeira") fica para depois. *Afeta:* DEL-001, UI-041, ADR 0021.
2. **O usuário pode criar plataformas novas? (recomendação: sim).** Hoje só existe "PC". Sem criação, trocar a plataforma
   de um jogo não serve para consoles e emuladores. A criação recebe nome e categoria (`pc`, `console`, `arcade`,
   `handheld`); nesta fase plataforma não pode ser renomeada nem apagada. *Afeta:* EDT-002, EDT-008, IPC-048.
3. **A página do jogo mostra sessões descartadas (< 30 s)? (recomendação: sim, marcadas).** Mostrar ajuda a explicar por
   que o total não subiu depois de um lançamento curto. Elas aparecem com o rótulo "Descartada" e o tempo ativo, mas
   nunca somam no total. *Afeta:* UI-038, STO-027.

---

## Decisões do usuário (2026-11-12, não reabrir)

1. **Página do jogo:** título, plataforma, gêneros, modos de jogo, desenvolvedora, editora, ano, tempo total jogado,
   primeira e última vez jogado, e a lista de sessões com duração (tempo ativo) e início/fim. Sem capa nem mídia (fase 7).
   *Afeta:* UI-037, UI-038, STO-026, STO-027, IPC-044, IPC-047.
2. **Biblioteca:** grade virtualizada de cards (título, plataforma, tempo total) com alternativa em lista (alternância
   na tela). Virtualização para bibliotecas grandes. *Afeta:* UI-029..UI-031, UI-036.
3. **Filtros e busca:** todos os de [ARCHITECTURE §5](../../ARCHITECTURE.md#5-modelo-de-dados-resumo): plataforma, gênero,
   modo de jogo (single/multi/coop/todos), desenvolvedora, editora, ano, status, favorito, jogado/não jogado, faixa de
   tempo jogado e texto livre por FTS5 sobre título, descrição, desenvolvedora e editora. A tabela `game_fts` é mantida
   em sincronia e a migração 5 reconstrói o índice dos jogos existentes. `FilterSpec` é serializável (o mesmo objeto
   servirá às playlists inteligentes; as playlists em si ficam fora desta fase). *Afeta:* FLT-*, FTS-*, LIB-*, STO-024,
   UI-032..UI-035.
4. **Edição e remoção:** editar título, plataforma, status, favorito, data de conclusão (`completed_at`), descrição, ano,
   desenvolvedora, editora, gêneros e modos de jogo. Remover com confirmação explícita. Toda escrita passa pelo agente
   por IPC (ADR 0015); leituras da grade e dos filtros também (`game.list` com `FilterSpec`, `game.get`, `game.update`,
   `game.delete`, `session.list`). *Afeta:* EDT-*, DEL-*, IPC-042..IPC-047, UI-039..UI-041.
5. **Pendências da fase 3 resolvidas aqui:** remover e editar jogo (P5 da fase 3) entram nesta fase; a grade substitui a
   lista mínima (UI-004 e UI-010 são substituídos); os componentes e o binding de resumo da biblioteca
   (`LibrarySummary`, `LibraryEmpty`) saem (decisão do spec-writer, item *m* abaixo). *Afeta:* UI-029, UI-044, UI-045,
   DSK-019.

## Decisões do spec-writer (a confirmar na aprovação)

Detalhes que não são produto puro; ficam como estão se ninguém objetar.

- **a.** `game.list` deixa de ser sem parâmetros e passa a ser paginado (padrão 200, máximo 500), porque uma biblioteca de
  milhares de jogos não cabe no limite de 1 MiB por mensagem do IPC (IPC-051). Sem paginação, a grade não funciona com
  bibliotecas grandes.
- **b.** O ano é guardado em `game.release_date` (já existe, `TEXT`). Aceita `YYYY`, `YYYY-MM` ou `YYYY-MM-DD`. Editar só
  o ano grava `YYYY`, a menos que a data completa já tenha esse mesmo ano (DOM-013).
- **c.** Filtros: dentro de cada faceta (plataforma, gênero, desenvolvedora, editora, status) vale **qualquer um** (OR);
  entre facetas vale **todas** (AND). Ex.: gênero "RPG" ou "Ação" **e** plataforma "PC" (FLT-002).
- **d.** "Jogado" é `play_count > 0`, ou seja, tem pelo menos uma sessão contada. Uma sessão em andamento não conta até
  encerrar (regra já existente, STO-014).
- **e.** A busca por texto usa a ordenação escolhida; não há ranking por relevância nesta fase (LIB-001).
- **f.** Não há notificação de mudança da biblioteca (IPC-050). A UI recarrega com as próprias mutações e na reconexão
  (UI-009, UI-042). Só existe uma UI por agente nesta fase.
- **g.** Escritas concorrentes: a última vence (sem versão nem bloqueio otimista). Só uma UI escreve por vez na prática.
- **h.** `completed_at` (instante, ADR 0009) é gravado como meia-noite UTC do dia informado, e a UI mostra a data em UTC
  (EDT-001). A UI nunca usa fuso local para isso.
- **i.** Gêneros, empresas e plataformas são casados sem diferenciar maiúsculas; o nome já existente prevalece sobre o
  digitado (EDT-004). Entradas de catálogo sem uso não são apagadas (coleta de lixo fora de escopo).
- **j.** Remover um jogo com um lançamento sem contar tempo ainda aberto é permitido (DEL-005): o processo continua vivo,
  fora de qualquer rastreamento, como já é hoje.
- **k.** Remoção e lançamento disputam o mesmo estado do tracker: a remoção só acontece dentro da goroutine dona do
  tracker, quando não há sessão nem lançamento pendentes do jogo (DEL-002, design).
- **l.** Os filtros e a ordenação vão para o agente, não para a UI, para que a grade e o cronômetro nunca disputem o
  mesmo cálculo; a UI nunca soma nem filtra tempo localmente (UI-044).
- **m.** `LibrarySummary` (store, binding e `DSK-008`) e os componentes `LibraryEmpty` e `LibrarySummaryView` são
  removidos (DSK-019, UI-045). O contador "N jogos · M plataformas" some; a contagem agora vem de `game.list.total`.

---

## Requisitos de fases anteriores afetados

| ID | Situação | Por |
|---|---|---|
| STO-001 — Migração em banco novo (fase 2) | alterado (schema na versão 5) | STO-024 |
| AST-001 — Tabela de preferências (single-exe) | alterado (`store.SchemaVersion` passa de 4 para 5; resto mantido) | STO-024 |
| GAM-001 — Adicionar jogo grava a linha (fase 3) | alterado (também grava a linha de `game_fts`, na mesma transação) | FTS-002 |
| GAM-005 — Ordem da lista (fase 3) | alterado (vira `sort = title` de LIB-001; a regra de ordenação não muda) | LIB-001 |
| IPC-008 — Params vazios (fase 2) | continua valendo para os métodos sem params; `game.list` deixa de ser um deles | IPC-042 |
| IPC-023 — `game.list` (fase 3) | substituído por IPC-042 (filtro, ordem e página; itens com `platform`, `status`, `favorite`, `year`) | IPC-042 |
| IPC-028 — Params inválidos (fase 3) | alterado (`game.list` passa a aceitar params e segue a mesma regra de tipo) | IPC-043 |
| IPC-032 — Contrato v0.5.0 é a fonte da verdade (fase 4) | substituído | IPC-041 |
| UI-004 — Lista de jogos (fase 3, com UI-019 da fase 4) | substituído (a grade com cards e a lista alternativa) | UI-029, UI-030 |
| UI-010 — Estado vazio e carregamento (fase 3) | substituído (mesmos textos, dentro da grade) | UI-044 |
| UI-006 — Jogo em execução (fase 3) | alterado (a marcação também aparece no card e na página do jogo) | UI-029, UI-038 |
| UI-019 / UI-021 — "Sem contar tempo" e pendência (fase 4) | alterados (os dois botões ficam no card e na linha da grade) | UI-031 |
| UI-024 — Linha do jogo pausado (fase 4) | alterado (vale para o card e para a linha da lista) | UI-029 |
| DSK-008 — Schema incompatível na UI (fase 2) | alterado (o `LibrarySummary` some; a regra vale para as leituras novas) | DSK-019 |
| P5 — Remover/editar jogo (fase 3, decisões) | entra nesta fase | EDT-*, DEL-* |
| Fora de escopo das fases 3 e 4: grade, filtros, mídia e remover/editar jogo | os itens de grade, filtros e edição entram nesta fase; mídia continua na fase 7 | — |
| ARCHITECTURE §5 — Modelo de dados e filtros | sem mudança de tabela; `game_fts` passa a ser mantido pelo agente | FTS-001 |

Inalterados e ainda válidos: DOM-001..DOM-011, GAM-002, GAM-003, STO-002..STO-023, SES-*, TRK-*, AGT-*, PAU-*, HOT-*,
PAD-*, PWR-*, OVL-*, SND-*, FBK-*, IPC-001..IPC-022, IPC-024..IPC-027, IPC-029, IPC-031, IPC-033..IPC-040, DSK-001..DSK-007,
DSK-009..DSK-018, UI-001..UI-003, UI-005, UI-007..UI-009, UI-011..UI-018, UI-020, UI-022..UI-023, UI-025..UI-028.
(UI-005 — formato do tempo total — é reaproveitado pelos cards e pela página; o teste continua o mesmo.)

---

## Domínio (DOM)

### DOM-012 — Status do jogo validado
Dado o texto de uma coluna ou de um campo de edição, quando chamo `domain.ParseGameStatus`, então `not_played`, `playing`,
`completed`, `abandoned` e `backlog` são aceitos e qualquer outro valor (inclusive `""` e `Playing`) retorna erro. O
padrão de um jogo novo é `not_played`. (ADR 0012)

### DOM-013 — Ano de lançamento sobre `release_date`
Dado um `release_date` `R` (`TEXT`, vazio ou nulo permitido), quando chamo `domain.ReleaseYear(R)`, então `YYYY`,
`YYYY-MM` e `YYYY-MM-DD` dão o ano; qualquer outro texto (ex.: `"abc"`, `"98"`) não dá ano. Dado um ano `y` entre 1900 e
2100 e a data `R`, quando chamo `domain.WithYear(R, y)`, então: se `R` já tem o ano `y`, devolve `R` sem alteração; senão
devolve `"y"` (com quatro dígitos). Exemplos: `1998-06-12` com `1998` → `1998-06-12`; com `1999` → `1999`; `""` com `2001` →
`2001`. Dado `y` fora de 1900..2100, então erro.

### DOM-014 — Modos de jogo e grupos do filtro
Dado um nome de modo, quando chamo `domain.ParsePlayMode`, então `single`, `multi_local`, `multi_online`, `coop_local` e
`coop_online` são aceitos e os demais retornam erro. Dado um grupo, quando chamo `domain.ParsePlayModeGroup`, então
`all`, `single`, `multi` e `coop` são aceitos; `multi` abrange `multi_local` e `multi_online`; `coop` abrange
`coop_local` e `coop_online`; `single` abrange só `single`; `all` abrange qualquer modo (inclusive jogo sem modo).

### DOM-015 — Categoria de plataforma validada
Dado o texto de uma categoria, quando chamo `domain.ParsePlatformCategory`, então `pc`, `console`, `arcade` e `handheld`
são aceitos e qualquer outro valor retorna erro. Dado um nome de plataforma, então ele precisa ficar, depois de cortar
espaços, entre 1 e 100 caracteres (runas); fora disso retorna erro. (ADR 0012)

### DOM-016 — Nomes de catálogo validados
Dado um nome de gênero ou de empresa, quando chamo `domain.NormalizeName`, então o resultado é o nome sem espaços nas
pontas; vazio ou com mais de 100 caracteres (runas) depois do corte retorna erro. Não há outra transformação (o nome
digitado é guardado com a capitalização dada, salvo EDT-004).

---

## Filtros (FLT)

Um `FilterSpec` é um objeto JSON com os membros da tabela de [design.md](design.md#filterspec). Todos os membros são
opcionais, exceto `version`. É o mesmo objeto para a grade, para o contador e, no futuro, para as playlists.

### FLT-001 — FilterSpec serializável e validado
Dado um `FilterSpec` com `version = 1` e os membros da tabela, quando é serializado e lido de volta, então o resultado é
igual ao original. Dado `version` ausente ou diferente de `1`, dado um membro desconhecido, dado um tipo errado, dado um
id não positivo, dado mais de 50 ids numa faceta, dado `yearFrom > yearTo` ou `playSecondsMin > playSecondsMax`, então
é recusado como parâmetro inválido (`-32602`), sem consulta. Dado `text` com mais de 200 caracteres (runas) depois de
cortar espaços, então também é recusado.

### FLT-002 — Plataforma, gênero, desenvolvedora e editora: qualquer um dentro da faceta, todas as facetas juntas
Dada a biblioteca com o jogo `A` (plataforma 1, gênero 5) e o jogo `B` (plataforma 2, gênero 5) e o jogo `C` (plataforma 1,
gênero 6), quando filtro `platformIds = [1, 2]` e `genreIds = [5]`, então o resultado é `[A, B]`. Quando filtro
`platformIds = [1]` e `genreIds = [5, 6]`, então o resultado é `[A, C]`. As mesmas regras valem para `developerIds` e
`publisherIds`.

### FLT-003 — Modo de jogo por grupo
Dado o jogo `M` com o modo `multi_local` e o jogo `S` com o modo `single` e o jogo `N` sem modo, quando filtro
`playMode = "multi"`, então o resultado é `[M]`; com `"single"`, `[S]`; com `"coop"`, `[]`; com `"all"` (ou sem o membro),
`[M, S, N]`.

### FLT-004 — Ano em intervalo fechado
Dados os jogos com ano 1998, 2005 e sem data, quando filtro `yearFrom = 2000`, então o resultado é `[2005]`; com
`yearFrom = 1998, yearTo = 2005`, então `[1998, 2005]`. Com qualquer limite informado, jogo sem ano fica fora; sem nenhum
limite (ou com ambos nulos), a faceta não filtra e o jogo sem ano entra.

### FLT-005 — Status e favorito
Dado `statuses = ["playing", "backlog"]`, então entram só os jogos com um desses status (lista vazia não filtra). Dado
`favorite = true`, então só favoritos; `favorite = false`, só não favoritos; `null` ou ausente, não filtra.

### FLT-006 — Jogado e faixa de tempo
Dado `played = "played"`, então entram só os jogos com `play_count > 0`; `"notPlayed"`, os com `play_count = 0`; `"any"`
ou ausente, não filtra (decisão d). Dados `playSecondsMin = 3600` e `playSecondsMax = 7200`, então entram os jogos com
`total_play_seconds` entre 3600 e 7200 inclusive; um jogo com 0 s não entra com `playSecondsMin = 1`.

### FLT-007 — Texto livre combina com os outros filtros
Dado `text` não vazio depois de cortar espaços, então entram só os jogos que casam com a busca de FTS-005 **e** passam em
todos os outros filtros. `text` vazio ou só com espaços não filtra.

### FLT-008 — Filtro vazio é a biblioteca toda
Dado `{"version": 1}`, então o resultado é a biblioteca inteira e `total` é o número de jogos. Dada a biblioteca com 3 jogos
e 1 favorito, quando filtro `{"version": 1, "favorite": true}`, então `total = 1`, e a página e o total batem.

---

## Busca textual (FTS)

Tabela `game_fts` (FTS5, já existe desde a migração 1): `rowid` é `game.id`; colunas `title`, `description`, `developer` e
`publisher` (nomes das empresas, separados por espaço). Detalhes em [design.md](design.md#índice-de-busca-fts5).

### FTS-001 — Conteúdo do índice
Dado um jogo `G`, quando o índice é lido, então existe uma linha com `rowid = G.id` cujas colunas são o título, a
descrição, os nomes das desenvolvedoras e os nomes das editoras de `G` (vazios quando não há). O índice não contém
plataforma, gênero, ano nem modos.

### FTS-002 — Adicionar jogo indexa na mesma transação
Dado um executável válido, quando adiciono o jogo (GAM-001), então a linha de `game_fts` existe com o título do jogo
depois do commit; dado que a inserção no índice falha, então o jogo também não é gravado.

### FTS-003 — Editar jogo reindexa na mesma transação
Dado o jogo `G` com descrição `"X"`, quando edito a descrição para `"Metroidvania"` (EDT-001) e troco a desenvolvedora,
então uma consulta por `metroidvania` encontra `G`, uma consulta pelo texto antigo não encontra `G`, e a desenvolvedora
nova é encontrada. Dado que a reindexação falha, então a edição inteira é desfeita (EDT-007).

### FTS-004 — Remover jogo tira o jogo do índice e não vaza para o próximo
Dado o jogo `G` removido (DEL-001), quando consulto o índice pelo título de `G`, então `G` não aparece. Dado que um
jogo novo `H` recebe o mesmo `id` (SQLite pode reaproveitar o maior `rowid`), quando consulto pelo texto de `G`, então
`H` não aparece, porque a linha antiga foi apagada e a de `H` tem só o conteúdo de `H`.

### FTS-005 — Consulta livre sem erro de sintaxe
Dado o texto do usuário, quando monto a consulta de FTS, então cada palavra vira um termo entre aspas com prefixo
(`"palavra"*`), e os termos são combinados com AND. Caracteres de sintaxe do FTS5 (`"`, `*`, `:`, `(`, `)`, `-`, `^`,
`AND`, `OR`, `NOT` digitados como palavra) não causam erro e não mudam o significado. Busca sem diferenciar maiúsculas
nem acentos: `pokemon` encontra `Pokémon`. Texto que não gera nenhum termo (só pontuação) não filtra.

---

## Biblioteca: listagem e ordenação (LIB)

### LIB-001 — Ordenação
Dado o `sort` escolhido, então a ordem é: `title` (padrão; título sem diferenciar maiúsculas, GAM-005), `lastPlayed`
(`last_played_at` decrescente, jogos nunca jogados por último), `totalPlayed` (`total_play_seconds` decrescente), `year`
(ano crescente, sem ano por último). Em todos, empate por título sem diferenciar maiúsculas e, depois, por `id`. Dados
jogos `b`, `A`, `a`, `C` com ids 1..4, então `title` dá `A`(2), `a`(3), `b`(1), `C`(4). Busca por texto (FLT-007) usa o
mesmo `sort`, sem ranking.

### LIB-002 — Paginação estável
Dado `offset` (inteiro ≥ 0, padrão 0) e `limit` (1 a 500, padrão 200), quando listo, então recebo no máximo `limit` jogos
a partir de `offset`, e `total` é a contagem de todos os jogos que passam pelo filtro (não só da página). Dada a
biblioteca com 450 jogos e `limit = 200`, as páginas `offset = 0, 200, 400` dão 200, 200 e 50 jogos, sem repetição nem
falta, porque a ordem tem desempate por `id`. `offset` além do fim dá `games: []` e `total` correto. `limit` ou `offset`
fora da faixa retorna `-32602`.

### LIB-003 — Lista vazia nunca é nula
Dado um filtro sem resultado ou uma biblioteca vazia, quando listo, então `games` é `[]` (nunca `null`) e `total = 0`.

---

## Edição (EDT)

Edição é um patch: membro ausente não muda o campo; membro presente com `null` apaga só os campos que aceitam `null`
(`completedAt`, `year`). Tudo numa transação (EDT-007). Métodos e erros na seção IPC e no contrato v0.6.0 (IPC-045).

### EDT-001 — Campos simples
Dado um jogo `G`, quando edito `title`, `status`, `favorite`, `description` ou `completedAt`:
- `title`: vale a regra de GAM-003 sem o padrão: depois de cortar espaços, vazio ou mais de 200 caracteres → `-32602`;
- `status`: vale DOM-012;
- `favorite`: booleano;
- `description`: texto de até 10 000 caracteres (runas), pode ser vazio;
- `completedAt`: `YYYY-MM-DD` ou `null`; gravado como meia-noite UTC desse dia (ADR 0009; decisão h). Outro formato → `-32602`.
Exemplo: `completedAt = "2026-11-12"` grava `2026-11-12T00:00:00Z`; `null` apaga.

### EDT-002 — Trocar a plataforma
Dado um `platformId` existente, quando edito `G`, então `G` aponta para a plataforma nova e mantém sessões, pausas, total e
contagem. Dado um `platformId` inexistente, então `-32602` e nada muda.

### EDT-003 — Ano
Dado `year` (inteiro de 1900 a 2100), quando edito `G`, então `release_date` vem de DOM-013 (`WithYear`). Dado `year = null`,
então `release_date` vira nulo. Dado `year` fora da faixa, então `-32602`. Dado `year` igual ao ano já gravado, então a data
completa é preservada.

### EDT-004 — Desenvolvedoras, editoras, gêneros e modos
Dado um array de nomes (desenvolvedoras, editoras, gêneros) ou de nomes de modo (`playModes`), quando edito `G`, então o
conjunto de `G` passa a ser exatamente o informado. Cada nome passa por DOM-016; duplicados sem diferenciar maiúsculas são
removidos; um nome que já existe no catálogo (sem diferenciar maiúsculas) é reutilizado com a grafia já gravada; senão o
nome é criado. Modos são nomes exatos de DOM-014 (senão `-32602`). Array vazio limpa o conjunto. Exemplo:
`developers = ["Team Cherry", "team cherry"]` com "Team Cherry" já cadastrada → uma só empresa, "Team Cherry".

### EDT-005 — Edição não mexe no tempo
Dado o jogo `G` com total `T`, contagem `N`, primeira e última vez jogado, sessões e pausas, quando edito qualquer campo
de EDT-001..EDT-004, então `total_play_seconds`, `play_count`, `first_played_at`, `last_played_at`, as sessões e as pausas
são iguais antes e depois. `updated_at` muda só quando algum campo mudou de fato (patch vazio ou igual ao atual não
altera `updated_at`).

### EDT-006 — Editar com o jogo em execução
Dado uma sessão de `G` em Running, Grace ou Paused, quando edito `G`, então a edição é aceita e a sessão não muda
(`session.current` igual antes e depois, sem `session.changed`).

### EDT-007 — Erro não grava nada
Dado um patch com um membro válido e outro inválido (ex.: `title` vazio com `developers` novos), quando envio o patch,
então `-32602` é devolvido e nenhum campo, nenhum nome de catálogo novo e nenhuma linha de `game_fts` mudam.

### EDT-008 — Criar plataforma
· alterado em 2026-11-13 (docs/specs/phase-9-plataformas, PLT-016, PLT-018, PLT-020): `platform.add` também devolve
`created`, e plataformas passam a poder ser renomeadas e excluídas.
Dado `name` e `category` (DOM-015), quando chamo `platform.add`, então: se já existe plataforma com o mesmo nome sem
diferenciar maiúsculas, devolve a existente (a categoria enviada é ignorada); senão cria e devolve a nova. Nome ou
categoria inválidos → `-32602`. Plataformas não são renomeadas nem apagadas nesta fase (pergunta 2).

### EDT-009 — Patch vazio e membros desconhecidos
Dado `game.update` só com `gameId`, quando chamo, então recebo o jogo sem alteração (sucesso). Dado um membro
desconhecido no patch, então `-32602` (IPC-052).

---

## Remoção (DEL)

### DEL-001 — Remover apaga o jogo, suas sessões e pausas
Dado o jogo `G` sem sessão ativa, quando removo `G` (`game.delete`), então `G` some de `game.list` e de `game.get`; as
sessões de `G` e as pausas dessas sessões somem (cascata, pergunta 1); as linhas de vínculo (gêneros, empresas, modos),
`additional_app`, `tracking_rule` e `media_asset` de `G` somem; a linha de `game_fts` de `G` é apagada (FTS-004). Os
outros jogos, a plataforma, os gêneros, as empresas e os modos continuam como estavam. Resultado: `{gameId: G}`.

### DEL-002 — Jogo com sessão ativa ou lançamento pendente não é removido
Dado o jogo `G` em Running, Grace ou Paused, ou um lançamento rastreado de `G` ainda em andamento, quando removo `G`, então
recebo `-32002` com `data.sessionId` (0 quando é só o lançamento), `data.gameId = G` e `data.title`, e nada muda.

### DEL-003 — Sessão ativa de outro jogo não impede a remoção
Dado a sessão ativa do jogo `A` e o jogo `B` parado, quando removo `B`, então `B` é removido e a sessão de `A` não muda.

### DEL-004 — Jogo inexistente
Dado um `gameId` que não existe, quando removo, então `-32001` e nada muda.

### DEL-005 — Processo do lançamento sem contar tempo continua
Dado um jogo `G` com um processo lançado por "Sem contar tempo" ainda aberto, quando removo `G`, então a remoção é aceita
(o agente não tem estado sobre esse processo, ADR 0019), o processo continua vivo, nada é registrado sobre ele e
`session.current` continua `null`.

### DEL-006 — Remoção atômica
Dado que apagar as linhas de `G` falha no meio (teste com trigger que aborta, como em STO-021), quando removo `G`, então
o jogo, as sessões, as pausas e a linha de `game_fts` continuam todos como estavam.

---

## Store (STO)

### STO-024 — Migração 5: índice de busca reconstruído
Dado um banco novo ou na versão 4 com dados, quando o agente abre o store com migração, então `store.SchemaVersion = 5`, a
versão do goose é 5, os dados permanecem, e `game_fts` tem uma linha por jogo existente com título, descrição e nomes das
empresas (FTS-001). Dado um banco na versão 4 com 3 jogos, quando migra, então uma consulta pelo título de cada um
encontra o jogo. O `Down` apaga só as linhas de `game_fts`.

### STO-025 — Listar com filtro, ordem e página
Dado um `FilterSpec` válido, um `sort` e `offset`/`limit`, quando chamo `Store.ListGames`, então recebo a página (`Games`,
com `platform`, `status`, `favorite` e `year`) e `Total` (LIB-002), obedecendo FLT-001..FLT-008 e LIB-001. A consulta é
montada só com parâmetros; nenhum texto do usuário entra no SQL como texto (nem o `sort`, que é um valor de um conjunto
fechado). Lista nunca nula (LIB-003).

### STO-026 — Detalhe do jogo
Dado o jogo `G`, quando chamo `Store.GameDetail(G)`, então recebo os campos de `Game` mais `description`, `completedAt`,
`developers`, `publishers`, `genres` e `playModes` (nomes, em ordem alfabética sem diferenciar maiúsculas), com as listas
vazias quando não há nada (nunca nulas). Dado um id inexistente, então erro que embrulha `ErrGameNotFound`.

### STO-027 — Sessões de um jogo
Dado o jogo `G` com sessões `ended`, `discarded` e uma aberta, quando chamo `Store.GameSessions(G, offset, limit)`, então
recebo as sessões ordenadas por `started_at` decrescente e `id` decrescente, cada uma com `state`, `startedAt`, `endedAt`
(nulo se aberta), `activeSeconds` e `endReason` (nulo quando não há). Dado `G` inexistente, então `ErrGameNotFound`.
Lista nunca nula; `total` é a contagem de todas as sessões de `G`.

### STO-028 — Editar jogo numa transação
Dado um patch válido para `G` (EDT-001..EDT-004), quando chamo `Store.UpdateGame(G, patch, W)`, então numa única transação
os campos mudam, os vínculos são substituídos, o catálogo recebe os nomes novos (EDT-004), `game_fts` é reindexado
(FTS-003) e `updated_at = W` só se algum campo mudou (EDT-005). Dado que qualquer passo falha, então nada muda.

### STO-029 — Remover jogo numa transação
Dado o jogo `G`, quando chamo `Store.DeleteGame(G)`, então numa única transação apaga a linha de `game_fts` e a linha de
`game` (as demais por `ON DELETE CASCADE`, com `foreign_keys = 1`). Dado que `G` tem sessão aberta (`ended_at` nulo), então
erro `ErrGameInUse` (que o agente traduz para DEL-002) e nada muda. Dado `G` inexistente, então `ErrGameNotFound`.

### STO-030 — Catálogo e criação de plataforma
Dado o banco, quando chamo `Store.Catalog`, então recebo as plataformas (`id`, `name`, `category`), os nomes de gênero e de
empresa em ordem alfabética sem diferenciar maiúsculas, e os modos de DOM-014; nenhum campo é nulo. Quando chamo
`Store.AddPlatform(name, category)`, então segue EDT-008.

---

## IPC (IPC)

Contrato: [contracts/ipc.openrpc.json](contracts/ipc.openrpc.json) v0.6.0. Os códigos de erro são os da v0.5.0; esta fase
não cria código novo (violações de regra viram `-32602`, e `-32002` cobre a remoção com sessão ativa).

### IPC-041 — Contrato v0.6.0 é a fonte da verdade
Dado `docs/specs/phase-5-library/contracts/ipc.openrpc.json`, quando o teste de contrato roda, então a versão é `0.6.0`,
todos os métodos e notificações existem no servidor/`internal/ipc` com os mesmos nomes, os campos obrigatórios dos
resultados batem com os tipos Go, os enums (`sort`, `playMode`, `status`, `played`, `category`) batem com o `domain`, os
limites de `x-limits` batem com as constantes, e os códigos de erro batem com as constantes. O comentário do pacote `ipc`
aponta para este arquivo. (Substitui IPC-032.)

### IPC-042 — `game.list` com filtro, ordem e página
Dado a biblioteca, quando chamo `game.list` com `{filter?, sort?, offset?, limit?}`, então recebo `{games, total, offset}`
com `games` no formato `Game` (inclui `platform`, `status`, `favorite`, `year`), na ordem de LIB-001 e na página de
LIB-002. Sem params (ou `{}`), recebo a primeira página (200) com o filtro vazio e ordem `title`. (Substitui IPC-023, que
continua como histórico.)

### IPC-043 — Params inválidos em `game.list`
Dado `sort` desconhecido, `limit` fora de 1..500, `offset` negativo ou não inteiro, `filter` com membro desconhecido ou
`version` diferente de 1 (FLT-001), array posicional, ou membro de tipo errado, quando chamo `game.list`, então recebo
`-32602`.

### IPC-044 — `game.get`
Dado um `gameId` existente, quando chamo `game.get`, então recebo `{game: GameDetail}` (STO-026). Dado `gameId`
inexistente, então `-32001`. Params como em IPC-028 (`gameId` obrigatório, inteiro ≥ 1).

### IPC-045 — `game.update`
Dado um `gameId` existente e um patch válido (EDT-001..EDT-004), quando chamo `game.update`, então recebo
`{game: GameDetail}` já com a alteração, e nenhum outro cliente recebe notificação (IPC-050). Dado patch inválido, então
`-32602` e nada muda (EDT-007). Dado `gameId` inexistente, então `-32001`. Sem sessão ativa exigida (EDT-006).

### IPC-046 — `game.delete`
Dado um `gameId` existente sem sessão ativa, quando chamo `game.delete`, então recebo `{gameId}` e o jogo sai do banco
(DEL-001). Dado `gameId` com sessão ou lançamento pendente (DEL-002), então `-32002` com `data` no formato `SessionRef`
(`sessionId` 0 quando só há lançamento pendente), e nada muda. Dado `gameId` inexistente, `-32001`.

### IPC-047 — `session.list`
Dado um `gameId` existente, quando chamo `session.list` com `{gameId, offset?, limit?}` (`limit` padrão 50, máximo 200),
então recebo `{sessions, total, offset}` na ordem de STO-027. Dado `gameId` inexistente, `-32001`. Paginação fora da faixa,
`-32602`.

### IPC-048 — `platform.list` e `platform.add`
· alterado em 2026-11-13 (docs/specs/phase-9-plataformas, PLT-014): `Platform` ganha `seedKey`, `iconPath` e `gameCount`.
Dado o banco, quando chamo `platform.list`, então recebo `{platforms: [{id, name, category}]}` em ordem de nome. Quando
chamo `platform.add` com `{name, category}` válidos, então recebo `{platform}` (existente ou nova, EDT-008). Nome ou
categoria inválidos, `-32602`.

### IPC-049 — `library.catalog`
· alterado em 2026-11-13 (docs/specs/phase-9-plataformas, PLT-014): `Platform` ganha `seedKey`, `iconPath` e `gameCount`.
Dado o banco, quando chamo `library.catalog`, então recebo `{platforms, genres, companies, playModes}` (STO-030); listas
nunca nulas; `playModes` tem os cinco nomes de DOM-014.

### IPC-050 — Sem notificação de biblioteca
Dado dois clientes conectados, quando um deles chama `game.add`, `game.update`, `game.delete` ou `platform.add`, então
nenhum `session.changed` nem outra notificação é enviado por causa disso. `session.changed` segue IPC-027/IPC-037.
(Decisão f.)

### IPC-051 — Página cabe no limite de mensagem
Dada uma página de 500 jogos com títulos de 200 caracteres, caminhos de 260 caracteres e plataforma de 100 caracteres,
quando `game.list` responde, então a resposta JSON (sem o `\n`) tem no máximo 1 048 576 bytes (`x-limits.maxMessageBytes`).
Dada a página de `game.get` com descrição de 10 000 caracteres e 50 nomes de vínculo, então também cabe.

### IPC-052 — Patch e params por nome
Dado `game.update` com array posicional, com membro desconhecido, com tipo errado em um campo (ex.: `favorite` texto) ou com
item de tipo errado em um array (ex.: `genres: [1]`), quando chamo, então recebo `-32602` (IPC-028). Ausente ≠ `null`:
`completedAt: null` apaga, `completedAt` ausente não mexe.

---

## Desktop (DSK)

### DSK-016 — Bindings de leitura da biblioteca
Quando chamo `ListGames(query)`, `GetGame(id)` ou `GameSessions(id, offset, limit)`, então a UI chama o método IPC
correspondente (prazo de 5 s), converte o resultado para o formato da UI sem calcular tempo (DSK-010 vale para a sessão)
e devolve o erro conforme DSK-011.

### DSK-017 — Bindings de escrita da biblioteca
Quando chamo `UpdateGame(id, patch)`, `DeleteGame(id)`, `Catalog()` ou `AddPlatform(name, category)`, então a UI chama
`game.update`, `game.delete`, `library.catalog` ou `platform.add` (prazo de 5 s); a UI não grava nada no banco e não
emite evento por conta própria. `UpdateGame` devolve a página de detalhe; `DeleteGame` devolve sucesso sem valor.

### DSK-018 — Mensagem de remoção com sessão ativa
Dado `DeleteGame` com `-32002`, então a mensagem é "Não é possível remover: o jogo está em uma sessão ativa. Feche a
sessão antes de remover." (sem o título, para não repetir o que a tela já mostra). Os demais códigos seguem DSK-011.

### DSK-019 — `LibrarySummary` removido
Dado o binding da fase 2/3, quando o código é conferido, então `LibrarySummary` não existe em `cmd/arcadia`, no `store`
nem em `frontend/wailsjs/`, e DSK-008 passa a valer para as leituras de DSK-016 (decisão m).

---

## Frontend (UI)

### UI-029 — Grade virtualizada de cards
Dada uma biblioteca com 10 000 jogos carregada em páginas, quando a grade é mostrada, então ela renderiza só os cards da
janela visível (no máximo 100 no DOM, em teste com altura e largura fixas), e rolar muda a janela sem recarregar a
página já carregada. Cada card mostra título, plataforma, tempo total (UI-005) e a marcação de sessão (UI-006: "Em
execução" ou "Pausado"). Os botões do card são UI-031.

### UI-030 — Alternar grade e lista
Dado o botão de alternância, quando clico, então a biblioteca passa de grade para lista (e de volta); a lista mostra a
mesma página, os mesmos dados e os mesmos botões, em linhas virtualizadas. A escolha fica guardada na própria UI
(preferência local, não no agente) e volta a valer ao abrir de novo; o padrão é grade.

### UI-031 — "Jogar" e "Sem contar tempo" no card e na linha
Dado um card ou uma linha, então há os botões "Jogar" e "Sem contar tempo" (UI-019 e UI-020, da fase 4), com o mesmo
bloqueio de UI-021: pendência de um jogo desabilita os dois botões dele e mostra "Iniciando…" no acionado.

### UI-032 — Painel de filtros
Dado o painel de filtros, então ele tem um controle para cada faceta de FLT-002..FLT-006 (plataforma, gênero, modo,
desenvolvedora, editora, ano de/até, status, favorito, jogado, faixa de tempo em horas). Cada mudança chama `ListGames`
com o `FilterSpec` novo (FLT-001) e a primeira página; o contador "N jogos" mostra `total` da resposta. Os valores das
listas de opções vêm de `Catalog()`.

### UI-033 — Busca com espera
Dado o campo de busca, quando digito, então a consulta só é enviada 300 ms (medidos pelo relógio da UI, injetável) depois
da última tecla; apagar o texto remove o filtro `text` (FLT-007). Uma busca por tecla não é enviada.

### UI-034 — Ordenação
Dado o seletor de ordenação com as opções "Título", "Última vez jogado", "Tempo jogado" e "Ano", quando escolho uma, então a
lista é recarregada com `sort` correspondente (LIB-001) e a página volta para o início.

### UI-035 — Limpar filtros e resultado vazio
Dado algum filtro ou busca ativo, quando clico "Limpar filtros", então o `FilterSpec` volta a `{"version": 1}` e a
ordenação ao padrão. Dado um filtro sem resultado, então a grade mostra "Nenhum jogo com estes filtros." e o botão
"Limpar filtros". Dada a biblioteca vazia sem filtro, então vale UI-044.

### UI-036 — Paginação sob demanda
Dada a grade com a primeira página carregada, quando a janela visível chega perto do último item carregado e ainda há
jogos (`loaded < total`), então a próxima página (`offset = loaded`) é pedida uma única vez; a página nova é anexada sem
repetir nem pular jogos. Mudar filtro, texto ou ordenação descarta as páginas carregadas.

### UI-037 — Página do jogo
Dado um card (clique no título ou botão "Detalhes"), quando abro, então a página mostra título, plataforma, gêneros,
modos de jogo, desenvolvedora, editora, ano, tempo total (UI-005), primeira e última vez jogado (data e hora locais, ou
"Nunca"), status, favorito, data de conclusão (em UTC, decisão h), descrição e o botão "Voltar". Não há imagem nem
vídeo (fase 7). A marcação de sessão (UI-006) aparece no topo da página quando o jogo está em sessão.

### UI-038 — Sessões na página do jogo
Dada a página de um jogo, então a lista de sessões (`GameSessions`, 50 por página, "Carregar mais") mostra, por linha:
início, fim (ou "Em andamento" quando aberta), tempo ativo em UI-005 e o estado. Sessão `discarded` mostra "Descartada"
e seu tempo ativo, sem somar no total (pergunta 3). A sessão aberta do jogo mostra o cronômetro de UI-014, UI-015 e
UI-022, a partir do `activeMs` recebido. A UI não soma nem subtrai tempo para montar a lista.

### UI-039 — Editar jogo
Dada a página de um jogo, quando clico "Editar", então um formulário mostra os campos de EDT-001..EDT-004 com os valores
atuais. "Salvar" envia só os campos alterados (patch, IPC-052) e fica desabilitado durante o envio. Sucesso: a página
mostra o detalhe devolvido e a grade e o catálogo são atualizados (UI-042). Erro: aviso com a mensagem de DSK-011 e o
formulário continua aberto com os valores digitados. "Cancelar" descarta as alterações sem chamar o agente.

### UI-040 — Validação antes do envio
Dado o formulário de edição, quando o título fica vazio (depois de cortar espaços), o ano fica fora de 1900..2100, a data
de conclusão não é `AAAA-MM-DD` ou a descrição passa de 10 000 caracteres, então o campo mostra o erro, "Salvar" fica
desabilitado e nada é enviado ao agente.

### UI-041 — Remover jogo com confirmação
Dada a página de um jogo, quando clico "Remover", então abre uma confirmação com o texto "Remover «título»? As sessões e as
pausas deste jogo também serão apagadas. Esta ação não pode ser desfeita." e os botões "Remover" e "Cancelar". "Cancelar"
ou fechar não chama o agente. "Remover" chama `DeleteGame` uma vez; sucesso volta para a grade sem o jogo e atualiza as
listas (UI-042); `-32002` mostra a mensagem de DSK-018 e fecha a confirmação com aviso no topo (UI-008).

### UI-042 — Atualização depois de escrita
Dado um `UpdateGame`, `DeleteGame`, `AddGame` ou `AddPlatform` concluído, então as listas de jogos, o detalhe afetado e o
catálogo são recarregados uma vez. Não há polling; a reconexão recarrega como em UI-009.

### UI-043 — Cards de jogo em sessão
Dado um jogo com sessão em Running, Grace ou Paused, então o card e a linha dele mostram a marcação de UI-006, e a
marcação some quando chega `session:changed` com `ended` ou `discarded` e a grade é recarregada (UI-006). Esta regra é
a mesma do painel e da lista da fase 4 (UI-024), agora para a grade.

### UI-044 — Carregamento, biblioteca vazia e contagem
Dado a lista ainda não carregada ou a chamada falhando, então a tela mostra "Conectando ao agente…" e tenta de novo a
cada 2 s (UI-010). Dado `game.list` com `total = 0` sem filtro, então mostra "Nenhum jogo ainda" e o botão "Adicionar
jogo". Dado `total > 0`, então o contador é "N jogos" (plural fixo, como em UI-002 da fase 2). Nenhum contador de
plataformas.

### UI-045 — Componentes de resumo removidos
Dado o código do frontend, quando a suíte roda, então nenhum arquivo em `frontend/src` referencia `LibraryEmpty`,
`LibrarySummaryView` ou `useLibrarySummary`, e os testes desses componentes foram removidos (decisão m).

### UI-046 — Ponta a ponta da biblioteca
Dado o Arcadia com uma biblioteca real de alguns milhares de jogos, quando rolo a grade, filtro por plataforma e gênero,
busco por um termo do título e da descrição, edito um jogo (título, ano, gêneros), abro a página e removo um jogo de
teste, então a rolagem é fluida, o filtro e a busca mostram o total certo, a edição aparece na grade, a página lista as
sessões do jogo e a remoção pede a confirmação de UI-041 e tira o jogo da grade.
· verificação: manual

---

## Fora de escopo

Capas, screenshots, vídeos e qualquer mídia (fase 7) · playlists (salvas, inteligentes ou manuais; o `FilterSpec` já é o
seu formato, mas não há tela nem tabela de uso) · importadores (fase 6) · edição ou exclusão de sessões e de pausas ·
edição de `first_played_at`, `last_played_at`, `total_play_seconds` e `play_count` (são derivados das sessões) ·
renomear ou apagar plataforma · coleta de gêneros, empresas e plataformas sem uso · ranking de relevância na busca ·
atualização da biblioteca em tempo real entre janelas (IPC-050) · favorito e status alterados automaticamente
(`status` continua manual, como na fase 3) · estatísticas (fase 8) · HLTB, nota (`rating`) e ocultar jogo (`hidden`) ·
tela de configurações.

## Limitações conhecidas

A confirmar na aprovação; não geram requisito nem teste.

1. **Busca com prefixo e sem ranking:** termos curtos (1 ou 2 letras) geram muitos resultados e a ordem é a escolhida, não
   a relevância.
2. **Catálogo sem coleta:** gêneros e empresas sem jogo continuam na lista de sugestões até uma limpeza futura.
3. **Sem desfazer:** a remoção não tem lixeira; o caminho é restaurar um backup do `library.db`.
4. **Última escrita vence:** duas janelas editando o mesmo jogo sobrescrevem campos uma da outra (decisão g).
5. **Ano aproximado:** quem tem só o ano guarda `YYYY`; a ordenação por ano usa o ano, não a data (decisão b).
6. **Concorrência entre leitura e escrita:** a grade pode mostrar, por um instante, um total antigo até a recarga de
   UI-042 (a leitura é uma consulta; não há cache de servidor).
7. **Limite de página:** a página de 500 jogos com títulos muito longos fica perto do limite de 1 MiB (IPC-051); os
   limites de texto da tabela de IPC evitam passar disso.
8. **Filtros de texto com acento:** a comparação de acentos depende do tokenizador `unicode61` do SQLite; letras fora do
   alfabeto latino podem não casar sem acento.

## ADRs propostos

A sessão principal escreve (a spec só os referencia). Numeração a partir de 0020.

1. **0020 — Índice de busca e `FilterSpec`:** `game_fts` é mantido pelo agente na mesma transação de cada escrita de
   jogo (SQL direto, fora do sqlc, por não ser tabela que o sqlc entenda); a migração 5 reconstrói o índice; o
   `FilterSpec` é o contrato de filtro (JSON versionado) compartilhado pela grade e pelas playlists futuras; a
   consulta é montada só com parâmetros.
2. **0021 — Remoção de jogo:** apaga em cascata (sessões e pausas), recusada com sessão ativa ou lançamento pendente do
   próprio jogo, serializada na goroutine dona do tracker; sem lixeira; confirmação obrigatória na UI.
3. **0022 — Listagem paginada no IPC:** `game.list` pagina (padrão 200, máximo 500) porque o limite de 1 MiB por mensagem
   não cabe a biblioteca inteira; `total` vem na mesma resposta. Ver IPC-051.

### EDT-010 — Tipo, alvo e argumentos de lançamento (incluído na fase 6.1, 2026-11-12)
Dado um jogo, quando edito `launchType`, `launchTarget` e `launchArgs` com `game.update`, então os valores vão para
`launch_type`, `launch_target` e `launch_args` no mesmo patch (EDT-001). `launchType` aceita só `exe` e `uri`; qualquer
outro valor é -32602. `launchTarget` não pode ser vazio, tem até 2000 caracteres e, para `exe`, é um caminho absoluto.
`launchArgs` tem até 2000 caracteres e pode ser vazio. Editar esses campos não toca sessões nem tempo (EDT-005, EDT-006).
O lançamento seguinte usa os valores novos; uma sessão já aberta continua com o alvo que tinha.
· verificação: manual (UI)
