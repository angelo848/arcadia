# Fase 10.1 — Ajustes da nova interface: requisitos

- **Status:** rascunho para aprovação do usuário (2026-11-14). As perguntas da primeira versão foram respondidas pelo
  usuário em 2026-11-14; as respostas estão em "Decisões" e o restante foi aprovado como estava.
- **Design:** [design.md](design.md) · **Contratos:** nenhum novo; o IPC continua a v0.11.0 de
  [phase-9-plataformas](../phase-9-plataformas/contracts/ipc.openrpc.json), sem mudança · **Tarefas:** `tasks.md` (o
  planner gera depois da aprovação).
- **Base:** [phase-10-nova-interface](../phase-10-nova-interface/requirements.md) (implementada; conferida pelo usuário
  em 2026-11-14, com prints), [phase-7-midia](../phase-7-midia/requirements.md) (MID-004, MID-006, MID-010) e
  [phase-6-importers](../phase-6-importers/requirements.md) (IMP-005, IMP-006, IMP-008).
- **ROADMAP:** ajuste da fase 10; antecipa da fase 12 a tabela de limites de imagem por tipo (só para o LaunchBox).
- **ADRs:** 0015 (o agente é o único escritor do banco e dos arquivos de mídia), 0028 (três colunas), 0029 (tema por
  variáveis CSS). Nenhum é contrariado. ADR proposto no fim.
- **Objetivo:** corrigir o contorno de seleção cortado na grade, dar ao carrossel o modelo do LaunchBox (destaque e faixa
  de miniaturas), destacar "Jogar", mostrar o cursor de mão nos controles clicáveis, usar outra imagem do jogo quando não
  há capa, padronizar a barra de rolagem e fazer a importação do LaunchBox trazer as imagens que hoje ficam para trás.
  **Sem mudança no tracker nem no IPC.**
- **Contexto (decisão do usuário, 2026-11-14):** não haverá "Completar imagens" de jogos já importados. Depois desta fase,
  o usuário vai limpar a base e reimportar tudo do LaunchBox; as imagens novas (AJU-018..AJU-021) valem para essa
  reimportação. Por isso a correção da importação só precisa valer para jogos gravados pela importação.

Formato: IDs `ÁREA-NNN`, estáveis e nunca reutilizados; critérios Dado/Quando/Então. Área nova: **AJU** (ajustes da
fase 10). Todo ID ativo precisa de um teste que o cite no nome (`it("AJU-009 ...")`, `t.Run("AJU-018/...")`), exceto os
marcados `· verificação: manual`. IDs removidos (AJU-023, AJU-024) ficam no documento, riscados, e não são reutilizados.

---

## Avaliação dos pontos levantados na conferência

| # | Ponto | Decisão | Por quê |
|---|---|---|---|
| Bug | Contorno do cartão cortado na 1ª linha e na 1ª coluna | **Entra** (AJU-001..AJU-003) | Erro visual da fase 10, só no frontend, de correção local. |
| 1 | Carrossel do LaunchBox (destaque + faixa de miniaturas) | **Entra** (AJU-009..AJU-014) | Só frontend: `media.list` já devolve todas as imagens com miniatura. Substitui UIX-026. |
| 2 | "Jogar" maior, "Jogar sem contar tempo" visível, cursor de mão | **Entra** (AJU-006..AJU-008) | Só CSS e marcação. A causa do cursor é do Tailwind 4 (o preflight põe `cursor: default` em `button`), então a correção é global. |
| 3 | Jogo sem capa usa outra imagem dele | **Entra** (AJU-015..AJU-017) | Só frontend: a grade já lê `media.list` por jogo (todas as imagens, não só a capa). **Sem mudança de IPC nem de agente.** |
| 4 | Importação do LaunchBox não traz todas as imagens | **Entra** (AJU-018..AJU-022, AJU-025) | É bug: `Box - Front` e as pastas de loja não são lidas (ver achados no design §7). O mapa de pastas é só no agente. Jogos já importados **não** são completados (o usuário reimporta, ver "Contexto"). |
| 4a | Limites por tipo (screenshot ≤ 10, capa ≤ 2, fanart ≤ 2) | **Entra só para o LaunchBox** (AJU-020) | O mapa novo traz mais capas e screenshots; sem o limite, o número de imagens por jogo cresce sem controle. A tabela fica no domínio para a fase 12 reusar nos provedores. |
| 4b | Clear Logo, caixa 3D, verso, cartucho, disco, flyers, arcade | **Fica para a fase 12** | Pede tipos de mídia novos (migração da restrição `CHECK` de `media.kind`, rótulos, uso na UI); casa com os logos do SteamGridDB da fase 12. |
| 5 | Barra de rolagem fina, no tema, em todas as áreas | **Entra** (AJU-004, AJU-005) | Só CSS global com tokens novos. |

---

## Decisões

### Respondidas pelo usuário (2026-11-14)

1. **Sem "Completar imagens" de jogos já importados.** AJU-023 e AJU-024 foram removidos; IMP-006 não muda; o IPC fica na
   v0.11.0, sem campo novo em `import.apply`. O usuário vai limpar a base e reimportar tudo do LaunchBox.
2. **Ordem da imagem do cartão sem capa:** tela de título → tela de seleção → screenshot (gameplay) → banner → fundo,
   preenchendo a área 2:3 com corte das bordas (`object-fit: cover`). (AJU-015, AJU-016)
3. **Limites por tipo já nesta fase, só para o LaunchBox, contando as imagens que o jogo já tem:** capa 2, screenshot 10,
   fundo 2, tela de título 2, tela de seleção 2, banner 2. (AJU-020; ADR 0030 proposto)
4. **Nomes com GUID existem:** o usuário confirmou que há arquivos `.jpg` com o ID do jogo no nome na pasta do LaunchBox.
   AJU-021 é requisito firme; o formato exato do nome é conferido contra a pasta real antes de implementar.
5. **Carrossel sem os botões "anterior/próximo" e sem dar a volta nas pontas** (decisões a e b abaixo).

### Do spec-writer (aprovadas com o restante)

- **a. A faixa de miniaturas não dá a volta nas pontas.** Como a grade (UIX-019): na primeira e na última miniatura a
  seta não faz nada. Assim, no Big Box, a seta na ponta da faixa leva o foco para fora dela. (AJU-012, AJU-014)
- **b. Os botões "Imagem anterior" e "Próxima imagem" saem.** A faixa substitui os dois, como no LaunchBox. (AJU-010)
- **c. Ordem fixa das imagens no carrossel:** capa, screenshots, tela de título, tela de seleção, fundo, banner; dentro do
  tipo, a ordem recebida. Hoje, depois da capa, vem a ordem alfabética do tipo no banco (fundo e banner antes dos
  screenshots). (AJU-009)
- **d. Imagem substituta que falha ao carregar mostra o placeholder** (não tenta a próxima), como a capa (UIX-015).
  (AJU-016)
- **e. Cursor de mão em todo controle clicável**, não só nos botões de lançamento: botões, itens de plataforma, cartões,
  linhas da lista, miniaturas, alternâncias, caixas de seleção. Desativado mostra "não permitido". (AJU-008)
- **f. A importação de qualquer fonte passa a ser gravada em lotes** de até 25 candidatos por chamada, com progresso.
  Hoje uma chamada com mais de 500 candidatos é recusada (`-32602`) e uma importação grande do LaunchBox, com as imagens
  a mais, pode passar dos 60 s da chamada. Com a reimportação completa prevista pelo usuário, este é o caso comum.
  (AJU-025)
- **g. Dentro de um tipo, as pastas são lidas em ordem de prioridade** (tabela de AJU-018); em cada pasta, primeiro os
  arquivos da raiz e depois as subpastas de região em ordem alfabética; em cada lugar, por nome (`-01` antes de `-02`).
  Não há região preferida configurável nesta fase. (AJU-019)

---

## Requisitos de fases anteriores afetados

Cada um recebeu, na spec de origem, a anotação `· alterado em 2026-11-14 (docs/specs/phase-10-1-ajustes, AJU-NNN)`.
Se esta spec não for aprovada, as anotações são desfeitas.

| Requisito | Fase | Situação | Por |
|---|---|---|---|
| UIX-014 — Cartão com capa e título | 10 | alterado: a área da capa pode mostrar a imagem substituta; o contorno de seleção aparece inteiro | AJU-001, AJU-015 |
| UIX-015 — Capa ou placeholder, sempre | 10 | alterado: capa → imagem substituta → placeholder | AJU-015, AJU-016 |
| UIX-020 — Visualização em lista | 10 | alterado: o contorno de foco da linha aparece inteiro | AJU-002 |
| UIX-024 — Painel sem jogo em foco | 10 | alterado: a capa pequena de "Jogados recentemente" segue a regra da substituta | AJU-017 |
| UIX-026 — Carrossel de imagens | 10 | substituído: destaque e faixa de miniaturas | AJU-009..AJU-014 |
| UIX-027 — "Jogar" e "Jogar sem contar tempo" | 10 | alterado: hierarquia visual dos dois botões; regras de chamada inalteradas | AJU-006, AJU-007 |
| UIX-042 — Nomes e estados acessíveis | 10 | alterado: os botões do carrossel dão lugar às miniaturas com nome; o critério "uma tecla, uma ação" passa a valer para a faixa | AJU-010, AJU-014 |
| UIX-043 — Tudo alcançável pelo teclado | 10 | alterado: "navegar no carrossel" é pela faixa de miniaturas | AJU-012 |
| UIX-047 — Sem mudança no agente, no tracker e no IPC | 10 | alterado para esta fase: agente, domínio e importadores mudam; tracker e IPC não | AJU-028 |
| MID-010 — Mídias do LaunchBox | 7 | alterado: pastas lidas, ordem, limite por tipo e nome com ID | AJU-018..AJU-021 |

IMP-006 (fase 6) **não muda**: a anotação condicional da primeira versão desta spec foi revertida (decisão 1).

Inalterados e ainda válidos: UIX-005 (colunas por largura; AJU-003 mantém os mínimos), UIX-006, UIX-007, UIX-019,
UIX-040, UIX-041, UIX-046 e os demais da fase 10; MID-004 (hash, miniatura, deduplicação), MID-006 e IMP-006.

**Limitação resolvida:** a limitação conhecida 1 da fase 10 ("Jogos do LaunchBox sem capa", porque `Box - Front` não era
lido) é resolvida por AJU-018 para jogos importados a partir desta fase; os atuais ganham as imagens na reimportação que
o usuário vai fazer (ver "Contexto").

**Lacunas registradas (bugs sem requisito):** o contorno cortado (AJU-001), o cursor sem mão (AJU-008), as pastas do
LaunchBox não lidas (AJU-018) e a recusa de importações com mais de 500 candidatos (AJU-025) não tinham critério que os
pegasse.

---

## AJU — Contorno de seleção e de foco na grade e na lista

### AJU-001 — Contorno do cartão inteiro em todas as bordas da grade
Dado a grade com o jogo em foco num cartão da primeira linha, da primeira coluna, da última coluna ou da última linha
(rolada até o fim), quando o cartão é mostrado marcado (UIX-017) ou recebe o foco do teclado (UIX-040), então o contorno
de seleção (anel `--arc-accent`) e o contorno de foco (`--arc-focus`, com o afastamento) aparecem inteiros nos quatro
lados: nenhuma parte fica escondida pela borda do contêiner de rolagem, pela barra do centro (UIX-013), pela coluna
esquerda ou pela barra de rolagem.
· verificação: teste de componente (o conteúdo rolável tem a margem interna `GRID_INSET` nos quatro lados, e a margem é
pelo menos a largura do anel somada à largura e ao afastamento do contorno de foco); teste de que rolar até um cartão
(UIX-019) leva em conta a margem; conferência manual nos quatro cantos.

### AJU-002 — Contorno da linha inteiro na lista
Dado a visualização em lista (UIX-020), quando a primeira ou a última linha recebe o foco do teclado, então o contorno de
foco aparece inteiro nos quatro lados, sem ser cortado pelo contêiner de rolagem nem pela barra de rolagem.
· verificação: teste de componente (margem interna da lista); conferência manual.

### AJU-003 — Número de colunas descontando a margem e a barra
Dado a grade com largura de caixa L, então o número de colunas e a largura do cartão são calculados sobre a largura útil
`L − 2 × GRID_INSET − SCROLLBAR_SIZE` (a barra de rolagem tem espaço reservado sempre, AJU-004), de modo que a coluna da
direita não fica sob a barra e o número de colunas não muda quando a barra aparece. Os mínimos de UIX-005 continuam:
pelo menos 2 cartões por linha com caixa de 484 px e pelo menos 5 com caixa de 1132 px.
· verificação: teste de unidade da largura útil e de `columnsFor` com 484 e 1132 px.

---

## AJU — Barra de rolagem

### AJU-004 — Barra fina e no tema em toda área rolável
Dado qualquer área rolável da UI (coluna esquerda, grade, lista, painel, faixa de miniaturas, corpo dos modais, listas
dentro dos modais, estatísticas e sessões), quando o conteúdo é maior que a área, então a barra de rolagem (vertical ou
horizontal) tem `SCROLLBAR_SIZE` (8 px) de espessura, sem setas nas pontas, com o trilho na cor `--arc-scrollbar-track`
e o polegar arredondado na cor `--arc-scrollbar-thumb`, que passa a `--arc-scrollbar-thumb-hover` com o mouse em cima ou
arrastando. A regra é global (vale para áreas novas sem código extra). A grade e a lista reservam o espaço da barra
(`scrollbar-gutter: stable`).
· verificação: teste que lê `index.css` (regras globais de barra presentes, com a espessura e os três tokens);
conferência manual em cada área.

### AJU-005 — Tokens da barra de rolagem
Dado o tema escuro, então `--arc-scrollbar-track`, `--arc-scrollbar-thumb` e `--arc-scrollbar-thumb-hover` existem no
bloco `[data-theme="dark"]` de `index.css`; as regras da barra usam só `var(--arc-*)`; fora do bloco do tema, `index.css`
não tem cor literal (`#rgb`, `rgb(...)`, `hsl(...)`); e o polegar em hover tem contraste de pelo menos 3:1 com
`--arc-surface` (componente de interface, como o contorno de foco em UIX-007).
· verificação: teste que lê os tokens e calcula o contraste (mesma fórmula de UIX-007).

---

## AJU — Botões de lançamento e cursor

### AJU-006 — "Jogar" em destaque
Dado G em foco no painel, então "Jogar" é o primeiro botão abaixo do carrossel, ocupa toda a largura do conteúdo do
painel, tem pelo menos 48 px de altura, texto de pelo menos 1,125 rem em negrito, fundo `--arc-play` (com
`--arc-play-hover` ao passar o mouse) e texto `--arc-on-accent`. As regras de chamada e de pendência de UIX-027 não mudam
("Iniciando…", desativado, um lançamento por vez).
· verificação: teste de componente (ordem dos botões, classes de tamanho do design §4); conferência manual.

### AJU-007 — "Jogar sem contar tempo" visível, com destaque menor
Dado G em foco, então "Jogar sem contar tempo" fica logo abaixo de "Jogar", ocupa toda a largura do conteúdo do painel,
tem pelo menos 40 px de altura, texto de 1 rem, fundo `--arc-surface-2` com borda de 1 px `--arc-play` e texto
`--arc-text` (hover `--arc-surface-3`): menor que "Jogar" e maior que "Editar", "Buscar metadados" e "Remover", que ficam
numa linha abaixo dele com o tamanho de hoje. As regras de UIX-027 e UI-020 não mudam.
· verificação: teste de componente (ordem e classes do design §4); conferência manual.

### AJU-008 — Cursor de mão em todo controle clicável
Dado qualquer controle clicável e ativo (botão, item de plataforma, cartão da grade, linha da lista, miniatura do
carrossel, alternância grade/lista, cabeçalho recolhível, caixa de seleção, link), quando o mouse passa sobre ele, então
o cursor é a mão (`pointer`). Dado o controle desativado (por exemplo "Jogar" com "Iniciando…"), então o cursor é "não
permitido" (`not-allowed`). A regra é global, em `index.css`, e vale para os modais e o Big Box.
· verificação: teste que lê `index.css` (seletores e valores do design §4); conferência manual.

---

## AJU — Carrossel com destaque e faixa de miniaturas

### AJU-009 — Imagem em destaque
Dado G em foco com n ≥ 1 imagens (`media.list`), então abaixo do cabeçalho do painel há a imagem em destaque, inteira
dentro de uma área 16:9 da largura do conteúdo do painel, com o rótulo do tipo ("Capa", "Screenshot", "Tela de título",
"Tela de seleção", "Fundo", "Banner") e o indicador "k de n" logo abaixo. As imagens seguem a ordem da decisão c (capa,
screenshots, tela de título, tela de seleção, fundo, banner; dentro do tipo, a ordem recebida). Ao pôr G em foco, a
primeira imagem está em destaque. Dado n = 0 ou a leitura com erro, então a área mostra o placeholder padrão, sem rótulo,
sem indicador e sem faixa. Clicar na imagem em destaque não faz nada. O carrossel não avança sozinho.
· verificação: teste de componente (n = 0, 1, 3; ordem com tipos misturados).

### AJU-010 — Faixa de miniaturas
Dado n ≥ 2, então abaixo da imagem em destaque há uma faixa horizontal (`role="group"`, nome "Miniaturas") com uma
miniatura por imagem, na mesma ordem, cada uma um botão com a miniatura da imagem (`thumbUrl`, ou `url` se vazia) e o
nome acessível "<tipo>, k de n"; a miniatura em destaque tem `aria-current="true"` e anel `--arc-accent` visível inteiro
(sem corte pela borda da faixa, como AJU-001). Quando as miniaturas não cabem na largura, a faixa rola na horizontal
(barra de AJU-004). Dado n = 1, então não há faixa. Não há botões "Imagem anterior" nem "Próxima imagem" (decisão b,
confirmada pelo usuário).
· verificação: teste de componente por papel e nome (`getByRole('group', { name: 'Miniaturas' })`, botões).

### AJU-011 — Clique numa miniatura
Dado a faixa, quando o usuário clica na miniatura k, então a imagem k passa ao destaque (o rótulo e "k de n" mudam), a
miniatura k fica com `aria-current="true"` e recebe o foco do teclado, e nenhum binding além de leituras é chamado.
Clicar na miniatura que já está em destaque não muda nada.
· verificação: teste de componente com espiões nos bindings.

### AJU-012 — Setas só com o foco na faixa
Dado o foco do teclado numa miniatura (depois de um clique ou de Tab, que para uma única vez na faixa, na miniatura em
destaque), quando o usuário aperta seta direita ou esquerda, então a vizinha passa ao destaque e recebe o foco; Home e End
vão à primeira e à última. Na primeira (esquerda) e na última (direita) miniatura, a seta não faz nada (decisão a,
confirmada pelo usuário). A miniatura em destaque rola para a parte visível da faixa. Dado o foco fora da faixa (na
grade, no painel, em outro botão), então as setas não mudam a imagem em destaque. Setas para cima e para baixo nunca são
tratadas pela faixa.
A seta, Home e End agem a partir da miniatura que **tem o foco**, não da que está em destaque (as duas costumam coincidir,
mas no Big Box o foco pode chegar a uma miniatura não destacada vindo de baixo ou de cima). As pontas também são as da
miniatura focada. Dado o Big Box com a miniatura 0 em destaque e o foco na miniatura 4 (de n ≥ 6), quando o usuário
aperta seta direita, então a miniatura 5 passa ao destaque e recebe o foco. Dado o foco na miniatura 0 com outra em
destaque, quando o usuário aperta seta esquerda, então a faixa não trata a tecla (o destaque não muda e não há
`preventDefault`). Dado o foco na miniatura k com outra em destaque, quando o usuário aperta Home ou End, então a
primeira ou a última passa ao destaque e recebe o foco.
· alterado em 2026-11-14 (acrescentado após a revisão): a navegação parte da miniatura focada, não da destacada.
· verificação: teste de componente com eventos de teclado (foco fora da faixa não muda a imagem; pontas; Home/End;
foco numa miniatura diferente da destacada).

### AJU-013 — Troca de jogo e imagens que falham
· alterado em 2026-11-14 (docs/specs/phase-10-2-imagens-ao-vivo, IMG-009): "ou G recebe imagens novas" deixa de voltar à
primeira imagem; imagens novas do mesmo jogo mantêm a imagem em destaque (IMG-009). A troca de jogo continua levando à
primeira.
Dado o carrossel de G com a imagem k em destaque, quando o jogo em foco passa a H (ou G recebe imagens novas, UIX-033),
então a primeira imagem de H fica em destaque e a faixa volta ao início. Dado uma imagem em destaque que falha ao
carregar, então o placeholder aparece no lugar dela; dado uma miniatura que falha, então o placeholder aparece só naquela
miniatura; o rótulo, o indicador e a navegação continuam.
· verificação: teste de componente (troca de `resetKey`; evento `error` na imagem e na miniatura).

### AJU-014 — Faixa de miniaturas no Big Box
Dado o Big Box ativo, então cada miniatura é um item navegável (`data-bigbox-item`); A (ou Enter) numa miniatura a põe em
destaque. Com o foco numa miniatura, seta esquerda ou direita que a faixa trata (AJU-012) faz uma única ação: a imagem
troca e o Big Box não move o foco pela mesma tecla (evento marcado como tratado, `defaultPrevented`, regra de UIX-042 e
UIX-044). Na ponta da faixa, a faixa não trata a seta e o Big Box move o foco para o item vizinho, como hoje; para cima e
para baixo são sempre do Big Box.
· verificação: teste de componente com `useBigBoxNav` e eventos simulados.

---

## AJU — Imagem do cartão quando o jogo não tem capa

### AJU-015 — Imagem substituta da capa
Dado um jogo com pelo menos uma imagem de tipo `cover`, então o cartão mostra a primeira capa, como hoje (UIX-015). Dado
um jogo sem capa e com imagem de outro tipo, então o cartão mostra a miniatura da primeira imagem na ordem
**tela de título → tela de seleção → screenshot (gameplay) → banner → fundo** (decisão 2 do usuário). Dado um jogo sem
nenhuma imagem, com a lista de mídia sem resposta ou com erro, então aparece o placeholder padrão. A área continua 2:3 e
do mesmo tamanho em todos os casos (a grade não muda de altura). Vale para a grade e para o Big Box (mesmo cartão).
· verificação: teste de unidade da escolha (cada combinação de tipos, incluindo só banner e fundo: vence o banner) e
teste de componente do cartão.

### AJU-016 — Enquadramento da substituta
Dado o cartão com a capa, então ela aparece inteira dentro da área (`object-fit: contain`, como hoje). Dado o cartão com
uma imagem substituta (de qualquer tipo da ordem de AJU-015), então ela preenche a área 2:3, centralizada, com as bordas
cortadas (`object-fit: cover`, decisão 2 do usuário). Dado uma substituta que falha ao carregar, então aparece o
placeholder (decisão d).
· verificação: teste de componente (atributo `data-fit` e classe de enquadramento; evento `error`).

### AJU-017 — A mesma regra em "Jogados recentemente"
Dado o painel sem jogo em foco com a lista "Jogados recentemente" (UIX-024), então a capa pequena de cada jogo segue
AJU-015 e AJU-016.
· verificação: teste de componente de `PanelEmpty`.

---

## AJU — Imagens importadas do LaunchBox

### AJU-018 — Pastas lidas e tipo gravado
Dado a importação de um candidato do LaunchBox com a pasta do LaunchBox conhecida (MID-010), então as imagens são lidas
de `<LaunchBox>\Images\<plataforma>\<pasta>\[<região>\]` para as pastas abaixo, gravadas com o tipo indicado, nesta ordem
de prioridade dentro do tipo:

| Tipo no Arcadia | Pastas do LaunchBox, em ordem de prioridade |
|---|---|
| `cover` (Capa) | `Box - Front`, `Box - Front - Reconstructed`, `Fanart - Box - Front`, `Steam Poster`, `Epic Games Poster`, `GOG Poster`, `Origin Poster`, `Uplay Thumbnail`, `Amazon Poster` |
| `screenshot` (Screenshot) | `Screenshot - Gameplay`, `Steam Screenshot`, `Epic Games Screenshot`, `GOG Screenshot`, `Origin Screenshot`, `Amazon Screenshot`, `Screenshot - High Scores`, `Screenshot - Game Over` |
| `title` (Tela de título) | `Screenshot - Game Title` |
| `select` (Tela de seleção) | `Screenshot - Game Select` |
| `background` (Fundo) | `Fanart - Background`, `Epic Games Background`, `Origin Background`, `Uplay Background`, `Amazon Background` |
| `banner` (Banner) | `Banner`, `Steam Banner` |

Os nomes de pasta são comparados sem diferenciar maiúsculas. Qualquer outra pasta (por exemplo `Clear Logo`, `Box - 3D`,
`Box - Back`, `Cart - Front`, `Disc`, `Advertisement Flyer - Front`, `Arcade - Marquee`) não é lida (ver "Fora desta
spec"). Pasta ausente ou sem arquivo do jogo não é erro (MID-010). Só candidatos gravados pela importação recebem imagens;
um candidato "já existe" (IMP-006) não recebe nada, como hoje (decisão 1).
· verificação: teste com árvore temporária (uma pasta de cada linha, mais uma pasta não mapeada); verificação manual
contra a pasta real (`ARCADIA_LAUNCHBOX_ROOT`, teste real que registra a contagem por pasta).

### AJU-019 — Ordem de escolha dentro do tipo
Dado várias imagens do mesmo tipo, então elas são consideradas na ordem: pasta pela prioridade de AJU-018; dentro da
pasta, primeiro os arquivos da raiz e depois as subpastas de região em ordem alfabética sem diferenciar maiúsculas; em
cada lugar, os arquivos por nome (decisão g). A primeira imagem gravada de cada tipo é a que a UI usa primeiro (a capa
do cartão, AJU-015).
· verificação: teste com árvore temporária (`Box - Front\Jogo-01.jpg`, `Box - Front\North America\Jogo-01.jpg` e
`Steam Poster\Jogo-01.jpg`: a primeira capa é a da raiz de `Box - Front`).

### AJU-020 — Limite de imagens por tipo
Dado a cópia das imagens do LaunchBox de um jogo, então são gravadas no máximo: capa 2, screenshot 10, fundo 2, tela de
título 2, tela de seleção 2, banner 2 (decisão 3 do usuário). O limite conta as imagens que o jogo já tem daquele tipo no
início da cópia, de qualquer provedor, e só as efetivamente gravadas (uma imagem repetida pelo hash, MID-004, não conta e
não ocupa vaga). Imagens que o jogo já tinha além do limite não são apagadas. A tabela de limites fica no domínio, para a
fase 12 usar nos provedores. Substitui o limite de 5 por tipo de MID-010.
· verificação: teste com árvore temporária (12 screenshots → 10; 3 capas iguais pelo conteúdo → 1; jogo com 2 capas já
gravadas antes da cópia → nenhuma capa nova).

### AJU-021 — Nome do arquivo com o ID do jogo
Dado o título T de um jogo (com os caracteres trocados por `_` como em MID-010), então são aceitos os arquivos
`T-NN.<ext>` e também `T.<GUID>-NN.<ext>`, onde `<GUID>` tem a forma `xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx` (hexadecimal),
forma que o LaunchBox usa para diferenciar jogos de mesmo título (o usuário confirmou que esses arquivos existem, em
`.jpg`). Extensões aceitas continuam `.jpg`, `.jpeg` e `.png`. Um arquivo de outro título que comece com T (por exemplo
`T 2-01.jpg`) não é aceito. **Antes de implementar, o formato exato do nome (separador entre título e GUID, maiúsculas
do GUID, posição do `-NN`) é conferido contra a pasta real do usuário** (`ARCADIA_LAUNCHBOX_ROOT`); se divergir do
descrito aqui, este critério é corrigido antes do código (com `· alterado em`).
· verificação: teste com árvore temporária (as duas formas; título prefixo de outro); teste real que conta os arquivos
com GUID na pasta do usuário.

### AJU-022 — Pastas não lidas registradas no log
Dado uma chamada de `import.apply` que copia imagens do LaunchBox, então, para cada plataforma dos candidatos, o agente
registra uma única vez, em `slog` nível Info, a lista das subpastas de `Images\<plataforma>` que não estão na tabela de
AJU-018 (nome da plataforma e nomes das pastas). Sem pasta não mapeada, nada é registrado. O registro não interrompe a
importação.
O registro é sobre a pasta de imagens da plataforma, não sobre a cópia: todo candidato do LaunchBox da chamada com a
pasta do LaunchBox conhecida conta, inclusive os "já existe" (IMP-006), que não recebem imagens (AJU-018). A unicidade
vale **por chamada** de `import.apply`: como a UI envia lotes de até 25 (AJU-025), uma plataforma pode aparecer uma vez
por lote numa mesma importação; esse comportamento é aceito (as entradas repetidas são idênticas e o agente não guarda
estado entre chamadas).
· alterado em 2026-11-14 (acrescentado após a revisão): explicitados a unicidade por chamada (lotes) e a contagem dos
candidatos "já existe".
· verificação: teste com logger de captura (uma entrada por plataforma; nenhuma sem pasta extra; candidato "já existe"
também gera a entrada).

### ~~AJU-023 — Completar imagens de jogos que já existem~~
· status: removido em 2026-11-14 (decisão 1 do usuário: não haverá "Completar imagens" de jogos já importados; o usuário
limpa a base e reimporta). Sem teste. O IPC continua v0.11.0 e IMP-006 não muda.

### ~~AJU-024 — Opção "Completar imagens" na importação~~
· status: removido em 2026-11-14 (decisão 1 do usuário, junto com AJU-023). Sem teste.

### AJU-025 — Importação gravada em lotes
Dado a confirmação de uma importação de qualquer fonte com N candidatos a enviar, então a UI chama `import.apply` em lotes
de até 25 candidatos, um lote por vez, na ordem da lista, e mostra "Importando k de N…" (k = candidatos já enviados); o
resumo final soma os lotes. Uma importação com mais de 500 candidatos deixa de ser recusada. Dado um lote que falha por
inteiro (erro do agente ou tempo esgotado), então os lotes seguintes não são enviados, o resumo mostra o que já foi
gravado e a mensagem de erro (UI-008) diz quantos candidatos ficaram sem enviar. Durante a importação, o diálogo não fecha
(UIX-039, botão de fechar desativado). O contrato de `import.apply` não muda (cada lote respeita o limite de 500 da
v0.11.0).
· verificação: teste de componente (60 candidatos → 3 chamadas de 25, 25, 10; falha no 2º lote → o 3º não é enviado;
progresso).

---

## AJU — Invariantes

### AJU-026 — A UI nunca influencia o tempo de jogo
Dado as interações desta fase (clicar e navegar nas miniaturas, rolar qualquer área, passar o mouse, importar em lotes),
então a UI só chama leituras e `import.apply`, e a importação continua só gravando jogos e imagens (IMP-009): nenhum
lançamento, nenhuma sessão rastreada, nenhum `ToggleSession`. As regras de lançamento de UIX-046 não mudam.
· verificação: o teste de invariantes de UIX-046 ganha as interações novas; teste do agente (a cópia com limite não cria
sessão).

### AJU-027 — Varreduras de tema, foco e ordem continuam valendo
Dado o código novo e alterado desta fase, então os testes de UIX-006 (sem classe de paleta nem cor literal nos `.tsx`),
UIX-040 (sem `outline-none` sem `focus-visible:`) e UIX-041 (sem `tabIndex` positivo) continuam passando sem exceção nova,
e toda cor nova é um token `--arc-*` (AJU-005).
· verificação: os testes de varredura existentes, sem alteração da lista de exceções.

### AJU-028 — Onde a fase mexe
· alterado em 2026-11-14: sem a parte de IPC e de completar existentes (decisão 1 do usuário).
· alterado em 2026-11-14 (docs/specs/phase-10-2-imagens-ao-vivo, IMG-013): o critério vale para o diff da fase 10.1; a
fase 10.2 acrescenta a notificação `media.changed` ao IPC (v0.12.0) e muda `internal/ipc`, `internal/agent` e o repasse
de `cmd/arcadia` (sem binding novo).
Dado o diff da fase, então não há mudança em `internal/tracker`, em `cmd/fake-game`, em `internal/ipc`, nos bindings de
`cmd/arcadia`, no contrato IPC (continua a v0.11.0) nem em migração ou query do banco (`internal/store/queries`); as
mudanças em Go ficam em `internal/importers` (pastas e nomes do LaunchBox), `internal/domain` (tabela de limites) e
`internal/agent` (cópia com limite e log das pastas não lidas).
· verificação: revisão do diff; o teste de contrato do IPC continua passando sem alteração.

### AJU-029 — Ponta a ponta dos ajustes
· alterado em 2026-11-14: a checagem das imagens do LaunchBox é pela reimportação com a base limpa (decisão 1).
Dado o Arcadia com a base limpa e a biblioteca real reimportada do LaunchBox, em 1366 × 768 e 1920 × 1080, quando ponho
em foco cartões nos quatro cantos da grade, rolo a grade, a lista, o painel e o modal de importação, uso o carrossel pelas
miniaturas (mouse, teclado e Big Box), passo o mouse nos botões e vejo os jogos que antes estavam sem capa, então os
contornos aparecem inteiros, as barras são finas e no tema, o cursor é a mão nos controles, "Jogar" é o botão mais
visível, a importação inteira termina (em lotes) e os jogos do LaunchBox passam a ter capa (ou a imagem substituta).
· verificação: manual.

---

## Fora desta spec

- **Completar as imagens de jogos já importados** (decisão 1 do usuário): não haverá; o usuário reimporta com a base
  limpa.
- **Tipos de mídia novos (fase 12):** `Clear Logo` (tipo `logo`), `Box - 3D`, `Box - Back`, `Box - Back - Reconstructed`,
  `Box - Full`, `Box - Spine`, `Fanart - Box - Back`, `Cart - Front/Back/3D`, `Fanart - Cart - *`, `Disc`, `Fanart - Disc`,
  `Advertisement Flyer - *`, `Arcade - *`. Exigem migração da restrição `CHECK` de `media.kind`, rótulos e um lugar na UI;
  o logo casa com os logos do SteamGridDB que a fase 12 vai trazer.
- **Limites para os provedores (fase 12):** a tabela de AJU-020 é reusada pela busca unificada; a fase 12 decide se o
  limite dos provedores também conta as imagens atuais (recomendação: sim, como aqui).
- **Título editável na revisão da importação (fase 11):** quando o título do candidato puder ser editado, a busca das
  imagens do LaunchBox precisa continuar usando o título original do XML. Registrar na spec da fase 11.
- **Região preferida configurável** para escolher as capas do LaunchBox (decisão g).
- **Escolher a capa manualmente** ("Definir como capa" no carrossel) e apagar imagens de um jogo.
- **Ampliar a imagem em destaque em tela cheia** e vídeos no carrossel (MID-007), como na fase 10.
- **Capas em lote no IPC** (limitação 3 da fase 10): a escolha da substituta (AJU-015) usa a mesma leitura por cartão de
  hoje; um método em lote continua para depois.
- **Aviso de imagens novas** quando a fila de mídia termina (limitação 2 da fase 10).
  · alterado em 2026-11-14: tratado na fase 10.2 ([phase-10-2-imagens-ao-vivo](../phase-10-2-imagens-ao-vivo/requirements.md)),
  com um aviso por imagem gravada, agrupado por jogo.

## Limitações conhecidas

1. **Nomes de pasta das lojas no LaunchBox:** os nomes da tabela de AJU-018 seguem o padrão do LaunchBox; se a pasta real
   do usuário tiver outro nome para alguma delas, AJU-022 a mostra no log e a tabela é corrigida antes da aprovação das
   tarefas.
2. **Jogos importados antes desta fase:** não ganham as imagens novas por si; o usuário vai limpar a base e reimportar
   tudo do LaunchBox (decisão 1). Limpar a base apaga as sessões rastreadas pelo Arcadia; o tempo do LaunchBox volta pela
   sessão importada (IMP-007).
3. **Imagem substituta recortada:** com `object-fit: cover`, um banner muito largo e um fundo aparecem só no meio; por
   isso as telas do jogo vêm antes deles na ordem de AJU-015.

## ADR proposto

A sessão principal escreve (a spec só o referencia). Numeração a partir de 0030.

1. **0030 — Limites de imagem por tipo como tabela única do domínio:** `internal/domain` guarda o máximo de imagens por
   tipo de mídia (capa 2, screenshot 10, fundo 2, demais 2), contado sobre o que o jogo já tem e só sobre o que é gravado
   de fato; toda fonte de imagens (LaunchBox agora, provedores na fase 12) passa por ela. Registra também que o
   LaunchBox é lido por um mapa fixo pasta → tipo, com prioridade.
