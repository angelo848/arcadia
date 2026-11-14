# Fase 9 — Plataformas: requisitos

- **Status:** aprovado pelo usuário (2026-11-13), com todas as recomendações das decisões de produto (ver "Decisões
  aprovadas").
- **Base:** [phase-6b-emulators](../phase-6b-emulators/requirements.md) (EMU-002, EMU-006: emulador padrão por
  plataforma, tela de emuladores), [phase-5-library](../phase-5-library/requirements.md) (DOM-015, EDT-008, IPC-048,
  IPC-049, IPC-050), [phase-6-importers](../phase-6-importers/requirements.md) (IMP-002..IMP-006),
  [phase-7-midia](../phase-7-midia/requirements.md) (MID-002, MID-004, MID-010) e
  [phase-3-tracker](../phase-3-tracker/requirements.md) (GAM-001).
- **ROADMAP:** fase 9 (pontos 5 e 10). **ARCHITECTURE:** §5 (`platform`), §11 (todo campo de arquivo tem "Procurar...").
- **ADRs:** 0006 (biblioteca portátil: arquivos ficam em `data\`), 0012 (enums validados no `domain`), 0015 (o agente é o
  único escritor do banco).
- **Objetivo:** gerenciar plataformas na UI como os emuladores; já trazer as plataformas principais cadastradas, cada uma
  com ícone; trocar "PC" por "Windows"; e deixar pronto o identificador de cada plataforma nos provedores de metadados
  para o casamento por título e plataforma da fase 12.

Formato: IDs `ÁREA-NNN`, critérios Dado/Quando/Então. Área nova: **PLT** (plataformas). A barra lateral agrupada por
plataforma é da fase 10; esta fase só garante os dados (nome, ícone e contagem de jogos).

Termos usados abaixo:
- **Plataforma do seed:** uma das 13 plataformas da tabela de [contracts/seed-platforms.md](contracts/seed-platforms.md).
  Cada uma tem uma **chave do seed** estável (`windows`, `n64`, `ps2`...), que não muda quando o usuário renomeia a
  plataforma.
- **Nome canônico:** o nome da plataforma na tabela do seed (ex.: `Sony Playstation 2`).
- **Mesmo nome:** igual depois de cortar os espaços das pontas, sem diferenciar maiúsculas (como EDT-008).

---

## Decisões de produto (recomendação indicada)

1. **Ícones do seed:** desenhos próprios do projeto (SVG simples: placa colorida com a sigla da plataforma, como "N64",
   "PS2", "GBA"), embutidos no executável junto com o frontend. Não usam logotipos das fabricantes (marcas registradas,
   sem licença livre de verdade). Recomendação: sim. Alternativa: o conjunto Simple Icons (CC0), que tem logotipos, mas
   cobre só parte das 13 plataformas, muda de versão para versão e não resolve a questão de marca.
2. **Ícone do usuário:** arquivo PNG ou JPEG de até 2 MB, escolhido por "Procurar...". O agente copia o arquivo para
   `data\media\platforms\` (biblioteca portátil, ADR 0006), com nome pelo hash do conteúdo, como as mídias da fase 7.
   O arquivo original pode ser apagado depois. "Restaurar padrão" volta ao ícone do seed (ou ao genérico). Recomendação: sim.
3. **Nome igual ao do seed:** a migração adota a plataforma existente com o mesmo nome (sem diferenciar maiúsculas),
   mantendo id, jogos e emulador, e passa a usar a grafia e a categoria do seed. Só maiúsculas e espaços das pontas
   contam; nomes "parecidos" de outra forma ("Super Nintendo" × "Super Nintendo Entertainment System") não são fundidos.
   Recomendação: sim.
4. **"PC" e "Windows" ao mesmo tempo:** os jogos de "PC" passam para "Windows", que fica com o seu emulador padrão (ou
   herda o de "PC" se não tiver), e "PC" é apagada. Recomendação: sim.
5. **Excluir e renomear plataforma do seed:** permitido, com as mesmas regras de qualquer plataforma (excluir só sem
   jogos). Renomear mantém a chave do seed (ícone e identificador nos provedores). O seed roda uma vez, na migração; uma
   plataforma do seed excluída não volta sozinha, mas criar uma plataforma com o nome canônico dela recupera a chave
   (PLT-016). Recomendação: sim (sem "ocultar").
6. **Categoria:** as do seed vêm da tabela (Windows `pc`; Nintendo 3DS e Game Boy Advance `handheld`; as demais,
   inclusive Switch e Switch 2, `console`). Na criação pelo usuário a categoria é escolhida no formulário, com `console`
   pré-selecionada, e pode ser trocada depois. Recomendação: sim.
7. **Identificador nos provedores:** fica numa tabela no código (`internal/scrapers`), indexada pela chave do seed. O
   banco só guarda a chave do seed. Os identificadores são fatos do provedor, não dados do usuário: uma correção chega
   com a versão nova, sem migração. Plataformas fora do seed ficam sem identificador nesta fase. Recomendação: sim
   (detalhes e alternativas no design §6).
8. **Emulador padrão:** pode ser escolhido no diálogo de plataformas; a seção equivalente do diálogo de emuladores
   (EMU-006) continua. Recomendação: manter os dois, pelo mesmo método.
9. **Painel de filtros atual:** com 13 plataformas cadastradas, as que não têm jogos deixam de aparecer no painel de
   filtros (até a fase 10 refazer a barra lateral). Recomendação: sim.

---

## PLT — Seed e migração (00008)

### PLT-001 — Seed na biblioteca nova
Dado um banco sem nenhuma plataforma, quando o agente aplica a migração 00008, então existem exatamente as 13
plataformas de [contracts/seed-platforms.md](contracts/seed-platforms.md), cada uma com o nome canônico, a categoria
e a chave do seed da tabela, sem ícone personalizado e sem emulador padrão.
· verificação: teste de migração na store (SQLite em `t.TempDir()`), comparando com a tabela do seed do `domain`.

### PLT-002 — "PC" passa a "Windows"
Dado um banco com a plataforma "PC" (id P) com jogos, sessões e emulador padrão, e sem plataforma "Windows", quando a
migração 00008 roda, então a plataforma de id P passa a se chamar "Windows", com categoria `pc` e chave `windows`; os
jogos continuam em `platform_id = P`, e as sessões, os totais e o emulador padrão não mudam.
· verificação: teste de migração na store a partir de um banco na versão 7.

### PLT-003 — "PC" e "Windows" ao mesmo tempo
Dado um banco com as plataformas "PC" (id P) e "Windows" (id W), cada uma com jogos, quando a migração 00008 roda,
então todos os jogos de P passam para W, P deixa de existir, W fica com a chave `windows` e o número total de jogos não
muda. W mantém o seu emulador padrão; se W não tinha emulador padrão, fica com o de P.
· verificação: teste de migração na store (com e sem emulador em W).

### PLT-004 — Adoção de plataforma com o nome do seed
Dado um banco com uma plataforma que tem o mesmo nome de uma plataforma do seed, em outra grafia (ex.: "Sony
PlayStation 2" importada do LaunchBox, categoria `console`), quando a migração 00008 roda, então essa plataforma
mantém o id, os jogos e o emulador padrão, ganha a chave do seed e passa a ter o nome canônico e a categoria do seed;
nenhuma outra plataforma com esse nome é criada.
· verificação: teste de migração na store.

### PLT-005 — Fusão de plataformas repetidas por maiúsculas
Dado um banco com duas ou mais plataformas de mesmo nome que só diferem em maiúsculas (ex.: "Sega Genesis" e "SEGA
GENESIS"), quando a migração 00008 roda, então fica só a de menor id, com os jogos de todas; ela mantém o seu emulador
padrão ou, se não tinha, fica com o da primeira das outras que tinha (em ordem de id).
· verificação: teste de migração na store.

### PLT-006 — Nome de plataforma único sem diferenciar maiúsculas
Dado o banco depois da migração 00008, quando uma escrita tenta gravar uma plataforma com o mesmo nome de outra (em
qualquer grafia), então o banco recusa a escrita (índice único `COLLATE NOCASE`) e nada é gravado.
· verificação: teste na store com escrita direta pelo `dbgen`.

### PLT-007 — Seed só uma vez
Dado uma plataforma do seed excluída pelo usuário (PLT-020), quando o agente reinicia, então ela não é criada de novo.
· verificação: teste na store (migrar, excluir, abrir o banco de novo).

### PLT-008 — Migração atômica
Dado uma falha no meio da migração 00008, quando o agente sobe, então nenhuma parte dela fica gravada (as plataformas e
os jogos ficam como na versão 7) e o agente registra o erro como em qualquer migração (ADR 0010).
· verificação: revisão (a migração roda numa transação do goose; não usa `NO TRANSACTION`).

---

## PLT — Ícones

### PLT-009 — Ícone embutido das plataformas do seed
Dado uma plataforma com chave do seed e sem ícone personalizado, quando a UI mostra a plataforma (diálogo de
plataformas), então aparece o ícone embutido daquela chave, sem acesso à rede nem a arquivo de `data\`.
· verificação: teste de componente (cada uma das 13 chaves tem ícone); conferência visual manual.

### PLT-010 — Ícone genérico
Dado uma plataforma sem chave do seed e sem ícone personalizado, quando a UI mostra a plataforma, então aparece o
ícone genérico de plataforma.
· verificação: teste de componente.

### PLT-011 — Trocar o ícone por um arquivo
Dado uma plataforma e um arquivo PNG ou JPEG de até 2 MB escolhido por "Procurar..." (ARCHITECTURE §11), quando o
usuário salva, então o agente copia o arquivo para `data\media\platforms\<hash>.<ext>`, grava esse caminho (relativo a
`data\media`) na plataforma e a UI passa a mostrar o novo ícone. Apagar ou mover o arquivo original depois não muda o
ícone.
· verificação: teste no agente com pasta temporária; teste de componente do seletor.

### PLT-012 — Arquivo de ícone recusado
Dado um caminho que não é absoluto, não existe, é uma pasta, passa de 2 MB ou não é PNG nem JPEG (pelo conteúdo, não
pela extensão), quando o usuário pede para usá-lo como ícone, então recebe `-32602` com o motivo, nenhum arquivo é
criado em `data\media\platforms\` e o ícone da plataforma não muda.
· verificação: teste no agente (tabela de casos).

### PLT-013 — Restaurar o ícone padrão
Dado uma plataforma com ícone personalizado, quando o usuário escolhe "Restaurar padrão", então a plataforma fica sem
ícone personalizado (volta ao ícone do seed ou ao genérico) e o arquivo antigo é apagado de `data\media\platforms\`, a
não ser que outra plataforma use o mesmo arquivo. A troca de um ícone personalizado por outro também apaga o antigo
nessa condição.
· verificação: teste no agente (arquivo usado por uma e por duas plataformas).

---

## PLT — Gerenciamento

### PLT-014 — Dados da plataforma no IPC
Dado o banco, quando chamo `platform.list` ou `library.catalog`, então cada plataforma traz, além de `id`, `name`,
`category` e `emulatorId`: `seedKey` (chave do seed ou `null`), `iconPath` (caminho absoluto do ícone personalizado ou
`null`) e `gameCount` (número de jogos da plataforma, contando os ocultos). A ordem continua por nome sem diferenciar
maiúsculas.
· verificação: teste de contrato no `ipc` (v0.11.0) e teste na store da contagem.

### PLT-015 — Lista no diálogo de plataformas
Dado o diálogo "Plataformas", quando ele abre, então lista todas as plataformas com ícone, nome, categoria, número de
jogos e emulador padrão, em ordem de nome.
· verificação: teste de componente com catálogo de fixture.

### PLT-016 — Criar plataforma
Dado o formulário de nova plataforma com nome (DOM-015) e categoria (`console` pré-selecionada), quando o usuário
salva, então a plataforma é criada e aparece na lista. Dado um nome que já resolve para uma plataforma existente
(PLT-025), então nada é criado e o formulário mostra "Já existe: <nome da plataforma existente>." (`platform.add`
devolve essa plataforma com `created: false`, EDT-008).
· verificação: teste de componente; teste no agente de `created`.

### PLT-017 — Nome do seed recupera a chave
Dado uma chave do seed sem plataforma (a plataforma foi excluída), quando uma plataforma é criada com o nome canônico
dessa chave (pelo diálogo, por `platform.add` ou por uma importação), então ela ganha a chave do seed (e com ela o
ícone embutido e o identificador nos provedores) e a categoria do seed, em vez da categoria enviada.
· verificação: teste na store.

### PLT-018 — Renomear plataforma
Dado uma plataforma, quando o usuário salva um nome novo válido (DOM-015), então o nome muda, os jogos continuam nela
pelo `platform_id` e a chave do seed, o ícone e o emulador padrão não mudam. Mudar só as maiúsculas do próprio nome é
aceito. Um nome igual ao de outra plataforma é recusado com `-32602` e nada muda.
· verificação: teste no agente e na store.

### PLT-019 — Trocar a categoria
Dado uma plataforma, quando o usuário escolhe outra categoria (`pc`, `console`, `arcade` ou `handheld`, DOM-015), então
a categoria é gravada. Categoria fora da lista é recusada com `-32602` e nada muda.
· verificação: teste no agente.

### PLT-020 — Excluir plataforma sem jogos
Dado uma plataforma sem nenhum jogo, quando o usuário toca "Excluir" e confirma (dois toques, como DEL-001 e EMU-006),
então a plataforma é apagada, some da lista e o arquivo do seu ícone personalizado é apagado (com a ressalva de
PLT-013). Nenhum emulador é apagado.
· verificação: teste no agente; teste de componente da confirmação.

### PLT-021 — Exclusão bloqueada com jogos
Dado uma plataforma com N jogos (N ≥ 1, inclusive ocultos), quando a exclusão é pedida, então o agente recusa com
`-32013` (`data: {platformId, gameCount: N}`), nada é apagado, e a UI mostra "Esta plataforma tem N jogo(s). Mova-os
para outra plataforma ou exclua-os antes." No diálogo, o botão "Excluir" dessa plataforma fica desativado com a mesma
explicação.
· verificação: teste no agente; teste de componente.

### PLT-022 — Emulador padrão no diálogo de plataformas
Dado o diálogo "Plataformas", quando o usuário escolhe o emulador padrão de uma plataforma (ou "Nenhum"), então
`platform.setEmulator` é chamado (EMU-002) e a lista mostra o novo valor. A seção equivalente do diálogo de emuladores
continua funcionando.
· verificação: teste de componente.

### PLT-023 — Diálogo "Plataformas" no cabeçalho
Dado a biblioteca, quando o usuário toca "Plataformas" no cabeçalho (ao lado de "Emuladores"), então o diálogo abre; o
campo de ícone tem o botão "Procurar..." que abre o seletor nativo filtrado para imagens (`*.png;*.jpg;*.jpeg`), e o
campo de texto do caminho continua visível (ARCHITECTURE §11).
· verificação: manual (UI).

### PLT-024 — Escritas só pelo agente, sem notificação
Dado a UI, quando ela cria, renomeia, troca categoria, troca ícone ou exclui uma plataforma, então a escrita é feita
pelo agente por IPC (ADR 0015) e nenhuma notificação é enviada por causa disso (como IPC-050); a UI recarrega o
catálogo por conta própria.
· verificação: teste no agente (nenhuma notificação); revisão de que a UI não abre o banco.

---

## PLT — Importação, adição e metadados

### PLT-025 — Resolução de plataforma por nome
Dado um nome de plataforma vindo de uma importação ou de `platform.add`, quando o agente procura a plataforma, então
usa, nesta ordem: (1) a plataforma com o mesmo nome; (2) se o nome é o nome canônico de uma chave do seed, a plataforma
com essa chave, mesmo que tenha sido renomeada; (3) senão cria uma plataforma nova (com PLT-017). O scan da importação
usa os passos 1 e 2 sem criar nada (IMP-001, IMP-006).
· verificação: teste na store (plataforma renomeada, excluída e inexistente).

### PLT-026 — Steam, Epic e arquivo `.exe` em Windows
Dado um candidato da Steam (IMP-002) ou da Epic (IMP-003), quando o scan roda, então a plataforma do candidato é
"Windows". Dado a confirmação, então o jogo vai para a plataforma de chave `windows` (PLT-025), mesmo que o usuário a
tenha renomeado.
· verificação: testes dos importadores e do apply no agente.

### PLT-027 — Jogo adicionado por executável em Windows
Dado um executável adicionado por `game.add` (GAM-001), quando o jogo é gravado, então ele vai para a plataforma de
chave `windows`, mesmo que renomeada. Se ela não existe (foi excluída), então é criada como "Windows", categoria `pc`,
com a chave `windows`, na mesma transação.
· verificação: teste na store.

### PLT-028 — LaunchBox: plataforma "Windows" e suas imagens
Dado um jogo do LaunchBox na plataforma `Windows` do XML, quando o scan roda, então o candidato tem a plataforma
"Windows" (sem tradução para outro nome), e a cópia de imagens (MID-010) procura em
`<LaunchBox>\Images\Windows\...`. Para qualquer plataforma, a pasta de imagens procurada é a do nome da plataforma no
XML.
· verificação: teste do importador com fixture em `Images\Windows`.

### PLT-029 — Categoria da plataforma criada pela importação
Dado um candidato cuja plataforma é resolvida para uma chave do seed (PLT-025 passo 2 ou PLT-017), quando a plataforma
é criada, então ela usa a categoria do seed. Para as demais plataformas a regra de hoje não muda (LaunchBox: `console`;
outras fontes: `pc`).
· verificação: teste no agente (`platformCategoryFor` e apply).

### PLT-030 — Identificador da plataforma nos provedores de metadados
Dado uma chave do seed e um provedor (nesta fase, só `igdb`), quando o código pede o identificador da plataforma nesse
provedor, então recebe o valor da tabela de [contracts/seed-platforms.md](contracts/seed-platforms.md). Dado uma
plataforma sem chave do seed, ou um provedor sem identificador, então a resposta é "sem identificador" (nunca um
valor inventado). Nada usa o identificador nesta fase; o uso é da fase 12.
· verificação: teste de tabela em `internal/scrapers`; os 13 valores do IGDB são conferidos manualmente na API
  (`/platforms`) antes da aprovação das tarefas.

### PLT-031 — Scrapers chamam a plataforma Windows de "Windows"
Dado um candidato do IGDB com uma plataforma Windows, ou um candidato da Steam Store, quando o scraper o devolve, então a
plataforma do candidato é "Windows". Assim um jogo na plataforma "Windows" continua recebendo o bônus de plataforma do
casamento (MID-002).
· verificação: testes dos scrapers com respostas gravadas e teste de `MatchScore` com "Windows".

---

## PLT — Biblioteca

### PLT-032 — Painel de filtros sem plataformas vazias
Dado o painel de filtros atual, quando ele lista as plataformas, então mostra só as que têm pelo menos um jogo; uma
plataforma sem jogos que já está marcada no filtro continua aparecendo até ser desmarcada.
· verificação: teste de componente.

---

## Alterações em requisitos de outras fases (aplicadas em 2026-11-13)

| Requisito | Fase | Mudança |
|---|---|---|
| GAM-001 | 3 | a plataforma do jogo adicionado por executável passa de `PC` para a de chave `windows` (PLT-027) |
| IMP-002, IMP-003 | 6 | "plataforma PC" passa a "plataforma Windows" (PLT-026) |
| EDT-008 | 5 | `platform.add` também devolve `created`; plataformas passam a poder ser renomeadas e excluídas (PLT-016, PLT-018, PLT-020) |
| IPC-048, IPC-049 | 5 | `Platform` ganha `seedKey`, `iconPath` e `gameCount` (PLT-014) |

Cada um recebeu a anotação `· alterado em 2026-11-13 (docs/specs/phase-9-plataformas, PLT-NNN)` na spec de origem.
O design da fase 6 (§9) recebeu a nota de que a plataforma `Windows` do LaunchBox, Steam e Epic passam a entrar em
"Windows" (PLT-002, PLT-026, PLT-028).

---

## Fora de escopo nesta fase

- Barra lateral com plataformas, ícones e contagem (fase 10); esta fase só entrega os dados.
- Casamento de metadados por título e plataforma usando o identificador do provedor (fase 12).
- Identificador nos provedores para plataformas fora do seed, e edição desse identificador pelo usuário.
- Plataformas além das 13 do seed (o usuário cria as outras pelo diálogo ou pela importação).
- Ícones animados, SVG enviado pelo usuário, ícone por região, e cópia dos ícones de plataforma do LaunchBox.
- "Restaurar plataformas padrão" (recriar de uma vez as plataformas do seed excluídas).
- Mover jogos em lote de uma plataforma para outra (o caminho continua sendo editar o jogo, EDT-002).
- Fusão de nomes parecidos além de maiúsculas e espaços das pontas (apelidos, abreviações).

---

## Decisões aprovadas

Aprovadas pelo usuário em 2026-11-13, todas pela recomendação.

1. Ícones do seed: desenhos próprios (placa com a sigla, sem logotipo das fabricantes). (PLT-009)
2. Ícone do usuário: só PNG e JPEG, até 2 MB, copiado para `data\media\platforms\`; `.ico` não é aceito. (PLT-011,
   PLT-012)
3. Plataforma do seed já existente em outra grafia: adota a grafia e a categoria do seed. (PLT-004)
4. "PC" e "Windows" ao mesmo tempo: tudo vai para "Windows" e "PC" é apagada. (PLT-003)
5. Plataforma do seed pode ser renomeada e excluída como as outras, sem "ocultar"; renomear mantém a chave, e recriar
   pelo nome canônico a recupera. (PLT-017, PLT-018, PLT-020)
6. Categorias do seed: Nintendo 3DS e Game Boy Advance `handheld`; Switch e Switch 2 `console`. (PLT-001)
7. Identificador nos provedores: tabela no código, indexada pela chave do seed. (PLT-030)
8. Emulador padrão: escolhido nos dois diálogos (plataformas e emuladores), pelo mesmo método. (PLT-022, EMU-006)
9. Painel de filtros: esconde plataformas sem jogos até a fase 10. (PLT-032)
