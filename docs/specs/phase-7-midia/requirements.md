# Fase 7 — Scrapers e gerenciador de mídia: requisitos

- **Status:** requisitos com as decisões respondidas pelo usuário (2026-11-12): provedores IGDB + SteamGridDB + Steam Store;
  vídeo adiado; mídias do LaunchBox entram nesta fase. Design e tarefas aguardam aprovação; nada é implementado antes disso.
- **Base:** [phase-5-library](../phase-5-library/requirements.md) (grade, página do jogo, edição) e
  [phase-6-importers](../phase-6-importers/requirements.md) (importação e LaunchBox).
- **ARCHITECTURE:** §5 (tabela de mídia), §6 (provedores, matching, merge, fila) e §9 (fase 7).
- **Objetivo:** buscar metadados e mídia (capa, screenshots, title screen e, se aprovado, vídeo) nos provedores,
  guardar as mídias na biblioteca e mostrá-las na grade e na página do jogo.

Formato: IDs `ÁREA-NNN`, critérios Dado/Quando/Então. Área nova: **MID**.

---

## Decisões de produto (recomendação indicada)

1. **Provedores nesta fase:** IGDB (metadados, screenshots), SteamGridDB (capas, heróis, logos) e Steam Store API
   (jogos PC, sem chave). Recomendação: ScreenScraper (retro, title screen, vídeo) fica para depois, por ter limites
   por usuário e exigir conta. LaunchBox Games DB e HowLongToBeat ficam fora (termos de uso e API não oficial).
2. **Vídeo:** não nesta fase (recomendação). Se aprovado, baixa só sob ação explícita do usuário, nunca em lote.
3. **Chaves de API:** guardadas no Windows Credential Manager, nunca em texto puro, nunca em log nem em
   `library.db`. A UI mostra só se a chave está cadastrada, nunca o valor.
4. **Armazenamento:** mídias em `data\media\<id do jogo>\` ao lado do `library.db` (ADR 0006), com
   deduplicação por hash. Miniatura gerada para a grade.
5. **Merge:** prioridade fixa por campo nesta fase (descrição: IGDB; capa: SteamGridDB; screenshot: IGDB; título
   de tela: ScreenScraper quando houver). Configuração por campo fica para uma fase posterior.

---

## Requisitos

### MID-001 — Chaves de API
Dado que o usuário informa a chave de um provedor, quando salvo, então ela é guardada no Credential Manager sob um
nome por provedor, e a UI mostra só "cadastrada" ou "não cadastrada". Dado que a chave não existe, então o provedor
fica indisponível com a mensagem "Cadastre a chave em Configurações" e nenhuma chamada é feita.
Verificação: teste com o Credential Manager trocado por fake; teste de que a chave não aparece em log.

### MID-002 — Busca e matching
Dado um jogo com título, plataforma e ano, quando busco no provedor, então recebo candidatos normalizados (sem ™, ®,
edição e região no título) ordenados por similaridade, plataforma e ano. Dado que a melhor similaridade fica abaixo do
limiar, então a UI lista os candidatos e o usuário escolhe, ou ignora a busca.
Verificação: tabela de casos de título com fixture; sem rede (provedor fake).

### MID-003 — Metadados do jogo
Dado um candidato escolhido, quando aplico, então os campos vazios do jogo (descrição, gêneros, modos, desenvolvedora,
editora, ano) são preenchidos pelo provedor de maior prioridade para o campo. Campos já preenchidos pelo usuário não
mudam, salvo se o usuário pedir "substituir".
Verificação: teste de merge por campo com dois provedores fake.

### MID-004 — Mídia de imagem
Dado um jogo com mídia escolhida (capa, screenshot, title screen), quando baixo, então o arquivo é gravado em
`data\media\<id>\` com nome pelo hash do conteúdo, deduplicado, e uma miniatura é gerada. O registro da mídia guarda
tipo, provedor, hash e caminho. Dado que o download falha, então só essa mídia fica de fora e o erro é listado (como
IMP-008).
Verificação: teste com HTTP local (`httptest`), sem rede externa.

### MID-005 — Fila em background
Dado uma ou mais mídias para baixar, quando a fila roda, então os downloads acontecem fora da UI, com limite de
concorrência, retomada após reinício do agente (itens não concluídos continuam pendentes) e cancelamento. O agente é o
único que escreve no banco e nos arquivos de mídia (ADR 0015).
Verificação: teste da fila com relógio fake (`domain.Clock`) e provedor fake.

### MID-006 — Exibição na grade e na página
Dado um jogo com capa baixada, quando a grade é mostrada, então a miniatura da capa aparece no lugar do cartão
sem imagem. Dado um jogo com title screen ou screenshots, quando abro a página do jogo, então essas imagens aparecem
com o tamanho de tela do aplicativo. Sem mídia, a grade e a página funcionam como hoje.
Verificação: teste de componente (Vitest) com imagens de exemplo; teste manual na grade real.

### MID-007 — Vídeo (adiado para fase posterior)
Substituído por uma fase posterior: o vídeo ficou fora desta fase por decisão do usuário (2026-11-12).
Dado que o usuário pede o vídeo de um jogo, quando confirma, então o arquivo é baixado para `data\media\<id>\` com aviso
de tamanho antes do download. Nenhum vídeo toca sozinho.
Verificação: teste de confirmação na UI; teste de download com HTTP local.

### MID-008 — Falha de rede
Dado que a rede está indisponível ou o provedor responde com erro, quando a busca ou o download roda, então a UI mostra
o aviso e o restante da biblioteca continua funcionando. Nenhum teste depende de rede externa.
Verificação: testes com provedores que falham; `go test` e `vitest` sem internet.

### MID-009 — Termos de uso
Dado que um provedor tem termos de uso que proíbem redistribuição, então o app baixa para o computador do usuário e
não embute dados desse provedor no repositório. A documentação lista os termos de cada provedor usado.
Verificação: revisão de documentação (manual).

---

## Perguntas em aberto (respondidas pelo usuário antes da aprovação)

1. Confirma os provedores desta fase (IGDB, SteamGridDB, Steam Store), ou quer incluir ScreenScraper já?
2. Vídeo entra nesta fase (MID-007) ou fica para depois?
3. Quer que as mídias importadas do LaunchBox (ARCHITECTURE §7, item 6) também entrem nesta fase?

---

### MID-010 — Mídias do LaunchBox
Estrutura real (confirmada na pasta do usuário): `<LaunchBox>\Images\<plataforma>\<tipo>\[<região>\]<título>-NN.<ext>`,
onde `<plataforma>` é o nome da plataforma do XML (`Windows` para PC). Tipos lidos e tipo de mídia gravado:
`Banner` → `banner`; `Fanart - Background` → `background`; `Screenshot - Game Title` → `title`;
`Screenshot - Gameplay` → `screenshot`; `Screenshot - Game Select` → `select`. As subpastas de região
(`North America`, `United States`, `World`, ...) são lidas como um só tipo, sem separar por região.
Nome do arquivo: o título com `: ` trocado por `_ ` e `'` trocado por `_` (ex.: `Black Myth: Wukong` →
`Black Myth_ Wukong-01.jpg`; `Assassin's Creed Mirage` → `Assassin_s Creed Mirage-01.jpg`). Aceitos: `.jpg`, `.jpeg`, `.png`.
Dado a importação de um candidato do LaunchBox, quando a pasta do LaunchBox é conhecida, então até 5 imagens por tipo são
copiadas para `data\media\<id>\` (com hash e miniatura, como MID-004), com `provider = launchbox`, sem consultar provedor.
Dado que a pasta de imagens não existe ou não há arquivo do jogo, então o candidato segue sem mídia, sem erro.
Verificação: teste com árvore temporária (incluindo subpasta de região e título com `:` e `'`); verificação manual
contra a pasta real do usuário.

## Fora de escopo

- Configuração de prioridade de merge pelo usuário (fase posterior).
- HowLongToBeat e LaunchBox Games DB.
- Modo tela cheia "Big Box" (fase 8).
