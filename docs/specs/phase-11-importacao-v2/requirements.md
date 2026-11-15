# Fase 11 — Importação v2: requisitos

- **Status:** rascunho para aprovação do usuário (2026-11-15). Perguntas em aberto no fim.
- **Design:** [design.md](design.md) · **Contratos:** [contracts/ipc-v0.13.0-delta.md](contracts/ipc-v0.13.0-delta.md)
  (só o acréscimo sobre a v0.12.0 de [phase-10-2-imagens-ao-vivo](../phase-10-2-imagens-ao-vivo/contracts/ipc.openrpc.json);
  o JSON completo `ipc.openrpc.json` v0.13.0 é uma tarefa, como na 10.2) e
  [contracts/import-rules.md](contracts/import-rules.md) (tabelas de extensões, vetores do normalizador, grupos de disco e
  textos de aviso) · **Tarefas:** `tasks.md` (o planner gera depois da aprovação).
- **Base:** [phase-6-importers](../phase-6-importers/requirements.md) (IMP-001, IMP-004, IMP-006, IMP-008..IMP-010),
  [phase-9-plataformas](../phase-9-plataformas/requirements.md) (PLT-025, PLT-026),
  [phase-10-nova-interface](../phase-10-nova-interface/requirements.md) (UIX-006, UIX-007, UIX-037..UIX-039, UIX-046) e
  [phase-10-1-ajustes](../phase-10-1-ajustes/requirements.md) (AJU-025, importação em lotes de 25).
- **ROADMAP:** fase 11 (pontos 1 e 7 e a janela de revisão); o campo "plataforma do arquivo" vem do achado da conferência
  da fase 9.
- **ADRs:** 0014, 0015 (o agente é o único escritor do banco), 0026 e 0027 (plataformas por chave e nome sem diferenciar
  maiúsculas), 0029 (tema por variáveis CSS). Nenhum é contrariado. ADR proposto no fim.
- **Objetivo:** a fonte "Pasta" passa a ler subpastas e a propor títulos limpos (`Legend of Zelda, The - Majora's Mask
  (U) [!]` vira `The Legend of Zelda - Majora's Mask`); discos do mesmo jogo viram um candidato só; a lista de candidatos
  vira uma janela de revisão (marcar, editar o título, ver arquivo, extensão e disco, contador); e a plataforma do arquivo
  vira um select, com Windows por padrão. **Nada em `internal/tracker`, em `cmd/fake-game` ou no armazenamento de tempo de
  jogo muda.**

Formato: IDs `IMV-NNN` (**IMV** = importação v2), estáveis e nunca reutilizados; critérios Dado/Quando/Então. Todo ID ativo
precisa de um teste que o cite no nome (`t.Run("IMV-013/...")`, `it("IMV-041 ...")`), exceto os marcados
`· verificação: manual`. Os vetores de teste do normalizador e das extensões estão em
[contracts/import-rules.md](contracts/import-rules.md); os testes de leitura de pasta usam `t.TempDir()` com arquivos vazios
(a leitura nunca abre arquivo, IMV-080).

---

## Conferência no LaunchBox real (somente leitura, 2026-11-15)

A instalação do usuário (`C:\Users\angel\LaunchBox`) foi consultada só para conferir comportamento; nada foi alterado.

1. **Título vem do nome do arquivo, não da pasta.** `ps1\07. Resident Evil 2 (USA)\Resident Evil 2 - Dual Shock Ver. (USA)
   (Disc 1)\Resident Evil 2 - Dual Shock Ver. (USA) (Disc 1).cue` virou `Resident Evil 2: Dual Shock Ver.`; o nome da pasta
   (`Resident Evil 2`) não aparece no título. O prefixo `07. ` da pasta nunca chega ao título.
2. **As marcações saem do título e vão para o campo `Version`** (`(USA) (Disc 1)`). O Arcadia não tem esse campo: as
   marcações são descartadas do título e continuam no nome do arquivo (IMV-007).
3. **Artigo final é movido:** `Legend of Zelda, The - Majora's Mask (U) [!].zip` virou `The Legend of Zelda: Majora's Mask`
   (o `:` vem do banco de dados do LaunchBox; o Arcadia mantém ` - `, IMV-004). `_redux_patched` virou ` redux patched`.
4. **Discos:** um jogo de vários discos é **um** jogo cujo caminho é o disco 1; os demais ficam como "aplicações
   adicionais". `Settings.xml` traz `ImportDefaultCombineRoms = true` (combinar ROMs é o padrão do LaunchBox). O Arcadia não
   tem "aplicação adicional": grava o disco 1 e informa os demais na janela (limitação 1).
5. **Layout real do usuário:** `E:\Games\emulators roms\ps1\<NN. Jogo (USA)>\<Jogo (USA)>.cue` (+ `.bin`), com discos em
   subpastas `<Jogo (Disc N)>`; `n64\*.zip` direto na pasta. Os dois layouts precisam funcionar (IMV-010, IMV-021).

---

## Decisões de produto (recomendação indicada)

Resolvem os itens "A decidir" do roadmap. O usuário pode trocar qualquer uma antes da aprovação.

1. **Título: pelo nome do arquivo (padrão), com a opção "nome da pasta do jogo".** Conferido no LaunchBox (achado 1). A
   opção existe para quando cada jogo fica na própria pasta e o arquivo tem nome genérico (`game.exe`, `disc.cue`). "Pasta
   do jogo" é a subpasta de primeiro nível dentro da pasta escolhida que contém o arquivo (funciona para
   `Jogos\Jogo X\bin\x.exe` e para o layout de discos acima); arquivo solto na raiz usa o nome do arquivo. (IMV-017)
2. **Extensões por lista de conhecidas, e dependentes da plataforma.** Sem filtro, uma leitura recursiva trazia `.dll`,
   `.sav`, `.png` e desinstaladores. A lista de extensões de jogo é fechada (tabela em `import-rules.md`): plataforma
   Windows lê só `.exe`, `.bat`, `.cmd`; plataforma de console do seed lê só ROM e imagem de disco; plataforma criada pelo
   usuário (sem chave) lê as duas listas. O que não entra é contado no aviso, e a fonte "Arquivo" continua aceitando
   qualquer tipo (é a saída para uma extensão fora da lista). (IMV-013, IMV-014)
3. **Limites:** no máximo **8 níveis** de subpasta abaixo da escolhida, **20.000 itens** visitados e um resultado de até
   **900 KiB** (cerca de mil jogos), por causa do teto de 1 MiB por mensagem do IPC. Passou do limite: a leitura mostra o
   que coube e avisa. Sem paginação nesta fase. (IMV-018, IMV-019)
4. **Marcação padrão: todas marcadas**, menos "já existe" (bloqueada) e os títulos repetidos dentro da própria lista (que
   o `import.apply` recusaria como duplicata). (IMV-041)
5. **Só as fontes "Arquivo" e "Pasta" têm título normalizado e editável.** Steam, Epic e LaunchBox trazem o título do
   próprio launcher (e o LaunchBox usa o título para achar as imagens, MID-010); para eles a tabela mostra o título sem
   edição. (IMV-007, IMV-044)

---

## Requisitos de fases anteriores afetados

Cada um recebeu, na spec de origem, a anotação `· alterado em 2026-11-15 (docs/specs/phase-11-importacao-v2, IMV-NNN)`.
Se esta spec não for aprovada, as anotações são desfeitas.

| Requisito | Fase | Situação | Por |
|---|---|---|---|
| IMP-004 — Arquivo avulso e pasta de jogos | 6 | **parte da pasta substituída** (primeiro nível, sem filtro, título = nome sem extensão); a parte do arquivo (qualquer tipo, `exe` × `emulator`, aviso) continua, com título normalizado | IMV-010..IMV-019, IMV-032 |
| IMP-010 — Tela de importação | 6 | **substituído** pela janela de revisão | IMV-040..IMV-052 |
| IMP-006 — Duplicatas | 6 | ampliado: o scan de arquivo e pasta recebe a plataforma e confere título + plataforma já na leitura (hoje só o alvo); a regra em si não muda | IMV-030 |
| PLT-026 — Steam, Epic e `.exe` em Windows | 9 | inalterado (candidato sem plataforma vai para a chave `windows`); a UI passa a sempre enviar a plataforma escolhida | IMV-063 |

Inalterados e ainda válidos: IMP-001 (dois passos, o scan não grava), IMP-002, IMP-003, IMP-005, IMP-007, IMP-008, **IMP-009**
(a importação não lança nem toca no jogo; o scan de pasta nem abre arquivo, IMV-080), AJU-025 (lotes de 25; a janela só
muda o que entra nos lotes), UIX-037..UIX-039 (o modal continua o grande, com corpo rolável e Esc desativado durante a
operação), UIX-046 (a UI nunca influencia o tempo de jogo), MID-010 e AJU-018..AJU-022 (imagens do LaunchBox).

---

## IMV — Normalização do título

### IMV-001 — O título proposto vem do nome sem a extensão
Dado um arquivo candidato das fontes "Arquivo" ou "Pasta", quando o agente monta o título proposto, então parte do nome
do arquivo sem a extensão, e só trata como extensão o trecho depois do último ponto quando ele tem de 1 a 5 caracteres
`[A-Za-z0-9]` com pelo menos uma letra (assim `Dr. Mario` e `Game 2.0` sem extensão mantêm o nome inteiro). O título proposto é o resultado das regras IMV-002 a
IMV-006 nessa ordem.
· verificação: teste de unidade do normalizador (tabela de `contracts/import-rules.md`, seção 2).

### IMV-002 — Marcações entre parênteses e colchetes saem
Dado um nome com grupos `( ... )` ou `[ ... ]` (ex.: `(U)`, `(USA)`, `(Rev 1)`, `(v1.1)`, `(Disc 2)`, `[!]`, `[b1]`,
`[S][!]`), quando o título é normalizado, então todos os grupos fechados são removidos, também os aninhados (o mais interno
primeiro, até não haver nenhum). Parêntese ou colchete sem par permanece como está. Dado um nome em que a remoção deixaria
o título vazio (ex.: `(Disc 1)`), então o título é o nome com os espaços normalizados, sem remover nada (IMV-005).
· verificação: teste de unidade (grupos múltiplos, aninhados, sem par, tudo marcação).

### IMV-003 — `_` e `.` viram espaço
Dado um nome, quando o título é normalizado, então todo `_` vira espaço. O `.` vira espaço **só** quando o nome (depois
das regras IMV-002 e do `_`, **já aparado e cortado como em IMV-005**) não tem nenhum espaço, e nunca entre dois dígitos (`Super.Mario.World.USA` →
`Super Mario World USA`; `Dr. Mario`, `Super Mario Bros. 3` e `Game v1.2` mantêm o ponto).
· verificação: teste de unidade (nome com espaços e pontos; nome só com pontos; ponto entre dígitos).

### IMV-004 — Artigo final vai para o começo
Dado um título com `, <Artigo>` onde o artigo é `The`, `A`, `An`, `Le`, `La`, `Les`, `El`, `Los`, `Las`, `Il`, `Der`, `Die` ou
`Das` (sem diferenciar maiúsculas), e o artigo está **no fim do título** ou **imediatamente antes de ` - ` ou de `: `**,
quando o título é normalizado, então o artigo passa para o começo, como foi escrito e com a primeira letra maiúscula, e a
vírgula some (`Legend of Zelda, The` → `The Legend of Zelda`; `Legend of Zelda, The - Majora's Mask` → `The Legend of Zelda -
Majora's Mask`). Só a primeira ocorrência é movida. `Hello, A World` não muda. Se o título **já começa com um artigo**
(`The`, `A`, `An`, `Le`, `La`, `Les`, `El`, `Los`, `Las`, `Il`, `Der`, `Die` ou `Das` + espaço, sem diferenciar maiúsculas),
nada é movido (`Foo, A - Bar, The` → `A Foo - Bar, The`, e normalizar de novo não move o `, The`).
· alterado em 2026-11-15: a regra "já começa com artigo" torna a normalização idempotente (IMV-008). O ` - ` **não** vira `: ` (isso é
trabalho da busca de metadados, fase 12).
· verificação: teste de unidade (fim, antes de ` - `, antes de `: `, no meio da frase, artigos estrangeiros, só a primeira).

### IMV-005 — Espaços, separadores soltos, tamanho e título nunca vazio
Dado o resultado das regras anteriores, então: sequências de espaços (inclusive tabulação) viram um espaço; as pontas são
aparadas de espaços e de `-` ou `,` sobrando (`Game - (USA)` → `Game`); um título com mais de 200 caracteres é cortado em
200 (limite de `domain.MaxTitleRunes`, contado em runas) e reaparado; o aparo e o corte acontecem **antes** de decidir se o
`.` vira espaço (IMV-003) e de mover o artigo (IMV-004), e o texto é aparado de novo depois de cada um (alterado em
2026-11-15: ordem necessária para a idempotência, IMV-008). Dado que o resultado ainda ficaria vazio, então o
título é o nome do arquivo sem a extensão com os espaços normalizados; dado que ele também é vazio, então o arquivo não vira
candidato.
· verificação: teste de unidade (espaços repetidos, traço solto, 250 caracteres com acento, nome só de marcações, nome vazio).

### IMV-006 — Prefixo de ordem em nome de pasta
Dado um título tirado do nome de uma pasta (IMV-017), quando o título é normalizado, então um prefixo `^\d{1,3}\.\s+`
seguido de um caractere que não é dígito é removido antes das demais regras (`31. Spyro - Year of the Dragon (USA)` →
`Spyro - Year of the Dragon`). Para títulos tirados de nome de arquivo e da fonte "Arquivo" essa regra não vale
(`1942`, `1. FC Köln` ficam como estão). `2.5D Game` (sem espaço depois do ponto) não é prefixo.
· verificação: teste de unidade (prefixo de pasta; mesmo texto vindo de arquivo; `1942`; `2.5D`).

### IMV-007 — Só o título proposto muda
Dado um candidato das fontes "Arquivo" ou "Pasta", então a normalização muda apenas `title`: o nome do arquivo, o
`launchTarget` e a pasta de instalação são os do disco, intactos, e as marcações removidas continuam só no nome do arquivo.
Dado um candidato da Steam, da Epic ou do LaunchBox, então o título é o do launcher, sem normalização.
· verificação: teste do agente e do importador (alvo igual ao caminho do disco; título normalizado; títulos de Steam, Epic e
LaunchBox inalterados).

### IMV-008 — O normalizador é determinístico e idempotente
Dado qualquer nome, então a normalização depende só dele (sem relógio, disco ou rede) e normalizar o resultado de novo não o
muda (`NormalizeTitle(NormalizeTitle(x)) == NormalizeTitle(x)`, para título de arquivo).
· verificação: teste de unidade com a tabela de vetores e um teste de propriedade sobre um conjunto de nomes (semente fixa).

---

## IMV — Leitura da pasta

### IMV-010 — Leitura recursiva em ordem natural
Dado a fonte "Pasta" com "Ler subpastas" ligado (o padrão), quando o scan roda, então o agente lê a pasta escolhida e todas
as subpastas até 8 níveis abaixo dela, e cada arquivo de jogo (IMV-013) vira um candidato. A ordem dos candidatos é a ordem
natural do caminho relativo: sem diferenciar maiúsculas, com sequências de dígitos comparadas como número (`Jogo 2` antes
de `Jogo 10`), pasta e arquivo no mesmo critério. Os dois layouts do usuário funcionam: jogos soltos na pasta
(`n64\Banjo-Kazooie (U) [!].zip`) e um jogo por subpasta (`ps1\31. Spyro (USA)\Spyro (USA).cue`).
· verificação: teste do importador com árvore em `t.TempDir()` (soltos, subpastas, ordem natural, profundidade 8 e 9).

### IMV-011 — "Ler subpastas" desligado lê só o primeiro nível
Dado "Ler subpastas" desligado, quando o scan roda, então só os arquivos diretamente dentro da pasta escolhida são lidos
(o comportamento da fase 6), com as regras de extensão, título e disco desta spec.
· verificação: teste do importador (mesma árvore, `FirstLevelOnly` ligado e desligado).

### IMV-012 — Itens que a leitura não segue
Dado um item cujo nome começa com `.` ou `$`, ou que se chama `System Volume Information`, `@eaDir`, `__MACOSX` ou
`node_modules`, ou que é link simbólico, junção ou outro ponto de reanálise, então nem ele nem o que há dentro dele é lido
nem contado. Dado uma subpasta que não pode ser lida (permissão), então a leitura continua nas demais e o aviso diz quantas
subpastas ficaram ilegíveis (IMP-008).
· verificação: teste do importador (nomes ignorados; pasta ilegível simulada por injeção de `ReadDir`; link simbólico quando
o teste puder criá-lo, senão `t.Skip` com motivo).

### IMV-013 — Extensões de jogo conhecidas, por plataforma
Dado o scan com uma plataforma (IMV-030), então a lista de extensões lidas é: plataforma de chave `windows` → só executáveis
(`.exe`, `.bat`, `.cmd`); plataforma com outra chave do seed → só ROM e imagem de disco; plataforma sem chave ou não
encontrada, ou scan sem plataforma → as duas listas juntas. A comparação da extensão não diferencia maiúsculas. As listas
estão na seção 1 de `contracts/import-rules.md`. Arquivo de outra extensão não vira candidato.
· verificação: teste do importador (um arquivo de cada extensão da tabela por classe; extensão em maiúsculas; classe
`Windows`, `console`, `qualquer`).

### IMV-014 — O que foi ignorado aparece no aviso
Dado arquivos ignorados por extensão (IMV-013), quando o scan termina, então o resultado traz um aviso com o total e as 5
extensões mais frequentes (`Pasta: 41 arquivos ignorados pela extensão (.png ×20, .txt ×12, .sav ×5, .nfo ×3, .dll ×1)`).
Dado que nenhum candidato foi encontrado e há arquivos da outra lista (executáveis numa leitura de console, ou ROMs numa
leitura de Windows), então o aviso diz que a plataforma escolhida não lê aquele tipo. Textos exatos em
`contracts/import-rules.md`, seção 5.
· verificação: teste do importador (contagem e ordem das extensões, empate por ordem alfabética; aviso de plataforma
trocada).

### IMV-015 — Executáveis que não são jogos
Dado uma leitura que inclui executáveis (IMV-013), então um arquivo cujo nome sem extensão, sem diferenciar maiúsculas,
casa com `unins*`, `uninstall*`, `setup*`, `*setup`, `vcredist*`, `vc_redist*`, `dxsetup`, `dotnet*`, `oalinst`,
`UnityCrashHandler*`, `crashpad_handler`, `*crashreport*`, `*redist*` ou `DirectX*` não vira candidato e conta como
ignorado no aviso de IMV-014. A fonte "Arquivo" não aplica essa lista (o usuário escolheu o arquivo).
· verificação: teste do importador (cada padrão; `setup.exe` ignorado na pasta e aceito na fonte "Arquivo").

### IMV-016 — Tipo de lançamento, alvo e pasta de instalação
Dado um candidato de pasta, então `launchType = exe` para `.exe`, `.bat` e `.cmd` e `launchType = emulator` para os demais
(IMP-004, EMU-003); `launchTarget` é o caminho absoluto do arquivo principal (`<pasta escolhida>\<caminho relativo>`);
`installDir` é a pasta do arquivo principal (como na fonte "Arquivo"), e não mais a pasta escolhida, que numa leitura
recursiva abrangeria todos os jogos. A regra de rastreio `install_dir` que a importação grava (RUL-001) passa a apontar
para essa pasta.
· verificação: teste do importador (alvo e `installDir` num arquivo da raiz e num de subpasta; `launchType` por extensão).

### IMV-017 — Título pelo nome do arquivo (padrão) ou pela pasta do jogo
Dado a fonte "Pasta" com "Título a partir de" = "Nome do arquivo" (padrão), então o título é o do nome do arquivo
(IMV-001..IMV-005). Dado "Nome da pasta do jogo", então o título é o do nome da subpasta de primeiro nível da pasta
escolhida que contém o arquivo, com IMV-006 aplicada; arquivo direto na raiz, ou pasta cujo nome normalizado é vazio, usa o
nome do arquivo. O agrupamento de discos (IMV-021) é o mesmo nas duas opções.
· verificação: teste do importador (`ps1\04. Final Fantasy VIII (USA)\...` e `07. Resident Evil 2 (USA)\Resident Evil 2 -
Dual Shock Ver. (USA) (Disc 1)\...` nas duas opções; arquivo na raiz).

### IMV-018 — Limites da leitura
Dado uma pasta com mais de 8 níveis de subpasta, então os níveis além do 8º não são lidos e o aviso diz isso. Dado mais de
20.000 itens (arquivos e pastas) visitados, então a leitura para na ordem natural do 20.000º item, devolve o que achou até
ali e o aviso diz "leitura interrompida". A leitura confere o cancelamento da requisição (`ctx`) a cada pasta.
· verificação: teste do importador com limites injetados (profundidade e itens pequenos nos testes; cancelamento no meio).

### IMV-019 — Limite do tamanho do resultado
Dado um scan cujo JSON dos candidatos passaria de 900 KiB (921.600 bytes; o IPC recusa mensagens maiores que 1 MiB,
IPC-003; a lista de jogos já resolve o mesmo problema com a página que cabe no limite, IPC-051), então o agente devolve os primeiros candidatos, na ordem do resultado, que cabem no limite e um aviso com o total
encontrado e quantos foram mostrados. Nenhum candidato é cortado ao meio.
· verificação: teste do agente (muitos candidatos com caminhos longos; resultado ≤ limite; aviso com os números).

---

## IMV — Discos e arquivos acompanhantes

### IMV-020 — Arquivos de trilha não viram candidato
Dado um `.cue` e, na mesma pasta, arquivos `.bin`, `.img`, `.sub`, `.wav` cujo nome, sem extensão e depois das regras
IMV-002 e IMV-005 (a chave do nome), é igual ao do `.cue` (`Jogo (USA).cue` e `Jogo (USA) (Track 01).bin`, ambos `jogo`), então
esses arquivos acompanham o `.cue` e não viram candidatos. O mesmo vale para `.ccd` (com `.img`, `.sub`), `.mds` (com
`.mdf`) e `.gdi` (com `.bin`, `.raw`). Um `.bin` sem `.cue` correspondente continua um candidato (ROM de Mega Drive). Nenhum
arquivo é aberto para isso.
· verificação: teste do importador (cue + bin; cue + várias trilhas; bin solto; ccd/img/sub; mds/mdf).

### IMV-021 — Discos do mesmo jogo viram um candidato
Dado arquivos principais cujo nome tem um marcador de disco (`(Disc N)`, `(Disk N)`, `(CD N)`, `(Disc N of M)`, também entre
colchetes, N de 1 a 99), quando eles têm a mesma chave de nome sem o marcador (sem diferenciar maiúsculas) e o mesmo "escopo"
(a mesma subpasta de primeiro nível da pasta escolhida; arquivos soltos na raiz formam o escopo raiz), então formam **um**
candidato: o arquivo do menor número de disco é o principal (`file`, `launchTarget`), `discs` é a quantidade de discos e
`discFiles` lista os caminhos relativos em ordem de disco. O título perde o marcador (IMV-002). Só o arquivo principal é
gravado como alvo do jogo (limitação 1). Um arquivo com marcador de disco sem par é um candidato comum, com `discs = 1`.
· verificação: teste do importador (layout `04. Final Fantasy VIII (USA)\<Jogo (Disc N)>\<Jogo (Disc N)>.cue`; discos soltos
na raiz; discos em escopos diferentes não se juntam; disco único).

### IMV-022 — Mesmo número de disco não se junta
Dado dois arquivos com a mesma chave de nome e o mesmo número de disco no mesmo escopo (`Jogo (USA) (Disc 1).cue` e
`Jogo (Europe) (Disc 1).cue`, ou `.cue` e `.chd` do mesmo disco), então o segundo continua um candidato separado, e a
repetição do título é sinalizada na janela (IMV-046). A ordem de decisão é a ordem natural do caminho.
· verificação: teste do importador (dois disco 1; dois discos 1 e 2 mais um disco 1 extra).

### IMV-023 — `.m3u` é o arquivo principal do grupo
Dado um `.m3u` na mesma pasta de arquivos de disco cuja chave de nome sem marcador é igual à dele, então o `.m3u` é o arquivo
principal do candidato (`file`, `launchTarget`, `extension = m3u`), `discs` conta os discos e `discFiles` os lista. Sem
arquivos de disco correspondentes, o `.m3u` é um candidato comum. Nenhum arquivo é aberto (a ligação é só pelo nome).
· verificação: teste do importador (m3u + 2 cue; m3u sozinho).

### IMV-024 — Campos do candidato para a janela
Dado um candidato das fontes "Arquivo" ou "Pasta", então ele traz `file` (caminho do arquivo principal relativo à pasta
escolhida, com `\`; na fonte "Arquivo", o nome do arquivo), `extension` (minúscula, sem ponto, `""` se não tem) e `discs`
(≥ 1); `discFiles` só aparece com `discs ≥ 2`. Candidatos de Steam, Epic e LaunchBox não trazem esses campos.
· verificação: teste do importador e teste de contrato (campos opcionais, nomes JSON).

---

## IMV — Plataforma e duplicatas na leitura

### IMV-030 — O scan recebe a plataforma
Dado `import.scan` das fontes "Arquivo" ou "Pasta" com `platform` (nome), quando o agente resolve a plataforma sem criar
nada (PLT-025, passos 1 e 2), então (a) a classe de extensões vem da chave dela (IMV-013) e (b) o "já existe" confere, além
do alvo, título + plataforma, com o título **proposto** (IMP-006). Dado `platform` ausente, vazio ou não encontrado, então a
classe é "qualquer" e só o alvo é conferido, como hoje. O scan continua sem gravar nada (IMP-001).
· verificação: teste do agente com store real (plataforma do seed renomeada; nome inexistente; título duplicado na
plataforma certa e em outra).

### IMV-031 — Rodar a mesma importação de novo não duplica
Dado uma pasta já importada (inclusive grupos de disco), quando o usuário lê a mesma pasta de novo, então todos os candidatos
voltam como "já existe" (pelo alvo do arquivo principal) e nenhum jogo é gravado de novo (IMP-006).
· verificação: teste do agente (scan, apply, scan: tudo `exists`; apply recusa com `duplicate`).

### IMV-032 — Fonte "Arquivo": um candidato, título normalizado
Dado a fonte "Arquivo" com um arquivo de qualquer tipo (IMP-004), então o candidato continua um só, com `launchType` por
extensão, e passa a ter título normalizado (IMV-001..IMV-005) e os campos de IMV-024 (`file` = nome do arquivo, `discs = 1`).
Os avisos de arquivo inexistente, pasta no lugar de arquivo e caminho vazio não mudam. As regras de extensão, de
executáveis ignorados e de disco não se aplicam.
· verificação: teste do importador (`Legend of Zelda, The - Majora's Mask (U) [!].z64`; `setup.exe`; arquivo sem extensão).

---

## IMV — Janela de revisão

### IMV-040 — A lista é uma tabela de revisão
Dado um scan com candidatos de qualquer fonte, quando ele termina, então a janela "Importar jogos" mostra uma tabela com as
colunas: caixa de seleção, **Título**, **Arquivo**, **Extensão**, **Disco**, uma linha por candidato, na ordem recebida, com
cabeçalho fixo. Candidatos sem os campos de IMV-024 (Steam, Epic, LaunchBox) mostram `—` em Arquivo, Extensão e Disco. A
coluna Disco mostra `—` com `discs = 1` e "N discos" com `discs ≥ 2`; a dica (atributo `title`) do Arquivo mostra o caminho
completo do alvo, e a do Disco, os `discFiles`. "Já existe" aparece como selo na linha (IMV-043).
· verificação: teste de componente (`ImportDialog`/`ImportReviewTable`) com candidatos de arquivo, de grupo de disco e de Steam.

### IMV-041 — Marcação padrão
Dado um scan recém-terminado, então toda linha está marcada, exceto as de "já existe" (IMV-043) e as de título repetido
(IMV-046). Ler de novo (Buscar) repõe a marcação padrão sobre a lista nova.
· verificação: teste de componente (15 candidatos: 2 existentes, 1 repetido → 12 marcados).

### IMV-042 — Marcar e desmarcar todos
Dado a tabela com linhas marcáveis, então o cabeçalho tem uma caixa "Selecionar todos" que marca todas as linhas marcáveis
(inclusive as de título repetido) quando desmarcada ou parcial, e desmarca todas quando está marcada; ela mostra o estado
parcial (`indeterminate`) quando só algumas estão marcadas, e fica desativada sem linhas marcáveis ou durante a importação.
· verificação: teste de componente (três estados; marcar tudo inclui repetidas; desativada em `busy`).

### IMV-043 — "Já existe" desmarcado e bloqueado
Dado um candidato com `exists = true` (IMP-006), então a linha mostra o selo "já existe", a caixa fica desmarcada e
desativada, o título não é editável e o "Selecionar todos" não a marca.
· verificação: teste de componente.

### IMV-044 — Título editável
Dado uma linha das fontes "Arquivo" ou "Pasta", quando o usuário edita o título no campo da linha, então o valor digitado
é o que será gravado, sem alterar mais nada da linha. Dado uma linha de Steam, Epic ou LaunchBox, então o título aparece
como texto, sem campo. O campo mostra o título proposto, aceita colar e acentos e não é cortado enquanto o usuário digita
(a validação é IMV-045).
· verificação: teste de componente (editar, importar, conferir o título enviado; linha de Steam sem campo).

### IMV-045 — Título editado inválido bloqueia a importação
Dado uma linha **marcada** cujo título, aparado, está vazio ou tem mais de 200 caracteres, então o campo fica marcado como
inválido (`aria-invalid`), com a mensagem "Título vazio" ou "Título com mais de 200 caracteres" ligada ao campo, e
"Importar selecionados" fica desativado enquanto existir uma linha marcada inválida. Linha desmarcada inválida não bloqueia.
Dado o título com espaços nas pontas, então o valor enviado é o aparado.
· verificação: teste de componente (vazio, 201 caracteres, 200 caracteres, espaços; linha desmarcada).

### IMV-046 — Título repetido na própria lista
Dado duas ou mais linhas não bloqueadas cujos títulos atuais, aparados e sem diferenciar maiúsculas, são iguais, então a
segunda em diante mostram o selo "título repetido" e, na leitura recém-feita, começam desmarcadas (IMV-041). O selo é
recalculado a cada edição de título; mudar um título para um valor único tira o selo, e a marcação da linha não muda sozinha.
A linha que o usuário marcar mesmo assim vai ao `import.apply`, que a recusa como `duplicate` (IMP-006) e o resultado a
lista.
· verificação: teste de componente (selo aparece e some com a edição; linha marcada à mão é enviada).

### IMV-047 — Contador
Dado a tabela, então um texto de status mostra "k de n jogos selecionados", com n = linhas marcáveis (as de "já existe" não
contam) e k = marcadas; com j > 0 linhas "já existe", acrescenta " (j já existem)". O texto acompanha cada marcação e cada
Buscar.
· verificação: teste de componente (contagens com e sem existentes).

### IMV-048 — Importar envia os títulos editados, em lotes
Dado "Importar selecionados" (ativo com k ≥ 1 marcadas válidas e fora de uma operação), quando o usuário confirma, então a UI
envia só as linhas marcadas, na ordem da tabela, cada uma com o título atual aparado e, nas fontes "Arquivo" e "Pasta", a
plataforma escolhida (IMV-063), em lotes de 25 como em AJU-025 (progresso "Importando k de N…", o lote que falha interrompe os
seguintes, o diálogo não fecha durante a operação). O agente grava o título recebido (revalidado, IMV-072) e o resumo e a
lista de recusados usam o título gravado. Depois de gravar ao menos um jogo, `onImported` roda como hoje.
· verificação: teste de componente (60 linhas → 25/25/10; títulos editados nos lotes; falha no 2º lote; botão desativado
durante a operação).

### IMV-049 — Mudar a entrada da leitura descarta a lista
Dado uma lista lida, quando o usuário muda a fonte, a pasta ou o arquivo escolhido, a plataforma, "Título a partir de" ou
"Ler subpastas", então a lista, a marcação e os títulos editados são descartados e a janela volta ao estado "sem leitura"
(é preciso Buscar de novo), porque a classe de extensões (IMV-013), os títulos propostos (IMV-017) e o "já existe"
(IMV-030) dependem desses valores. Ler de novo substitui a lista e as edições sem pedir confirmação.
· verificação: teste de componente (editar um título, mudar a plataforma → lista some; Buscar → lista nova com marcação padrão).

### IMV-050 — Resultado da importação
Dado o fim da importação, então a janela mostra "Importados: N" e a lista dos recusados com o motivo (`duplicate`, `failed`,
rótulos da fase 6), mantém a tabela com as linhas enviadas (gravadas ou recusadas) desmarcadas e as não enviadas, por
falha de um lote, ainda marcadas, e mostra o aviso de IMV-048 quando um lote falhou. Linhas gravadas passam a "já existe" na
tabela (casadas pelo título gravado, uma linha por título do resultado).
· verificação: teste de componente (resumo, recusados, linhas gravadas viram "já existe" e ficam bloqueadas).

### IMV-051 — Tema, tamanho e rolagem
Dado a janela, então ela continua do tamanho "grande" (UIX-037), com título e botões fixos e só o corpo rolando (UIX-038); a
tabela usa toda a altura livre do corpo e rola por dentro, com o cabeçalho fixo; todas as cores vêm dos tokens `--arc-*`
(UIX-006, UIX-007), sem token novo. Com 1.000 linhas a janela continua utilizável (sem virtualização nesta fase).
· verificação: teste de componente (`data-size`, estrutura, corpo rolável); a varredura de UIX-006 cobre os `.tsx` novos;
conferência manual em 1024 × 640 e 1920 × 1080 (junto de IMV-083).

### IMV-052 — Teclado e acessibilidade
Dado a tabela, então ela é uma `<table>` com `<th scope="col">`; cada caixa de seleção tem nome acessível "Selecionar
<título atual>"; cada campo de título tem nome acessível "Título de <arquivo ou título>"; Tab percorre caixa e campo de cada
linha; Espaço alterna a caixa; Esc dentro do campo de título fecha a janela (UIX-039), salvo durante a operação.
· verificação: teste de componente (papéis, nomes, Tab, Espaço).

---

## IMV — Plataforma do arquivo

### IMV-060 — Um select no lugar do texto livre
Dado as fontes "Arquivo" e "Pasta", então o campo "Plataforma do arquivo" é um `<select>` com as plataformas existentes
(`platform.list`, a mesma lista do catálogo da biblioteca), em ordem de nome, e fica **antes** do botão "Buscar" (a
plataforma entra na leitura, IMV-030). Não há como digitar um nome novo na janela; plataformas novas se criam no diálogo
Plataformas. As outras fontes não mostram o campo.
· verificação: teste de componente (opções; ausência de campo de texto; ausência nas fontes Steam, Epic, LaunchBox).

### IMV-061 — Windows por padrão
Dado a janela aberta numa fonte "Arquivo" ou "Pasta", então o select vem com a plataforma de chave `windows` (pela chave, não
pelo nome: mesmo renomeada, é a escolhida). Dado que ela foi excluída, então vem a primeira plataforma da lista. A escolha do
usuário se mantém ao trocar de fonte dentro da mesma abertura da janela.
· verificação: teste de componente (Windows renomeada; Windows ausente; troca de fonte).

### IMV-062 — Sem plataforma disponível
Dado a biblioteca sem nenhuma plataforma, quando o usuário escolhe "Arquivo" ou "Pasta", então o select fica desativado com a
mensagem "Nenhuma plataforma. Crie uma em Plataformas.", e "Buscar" e "Importar selecionados" ficam desativados para essas
fontes.
· verificação: teste de componente.

### IMV-063 — A plataforma segue com a leitura e com a confirmação
Dado a plataforma escolhida, então o nome dela vai em `import.scan` (IMV-030) e em cada candidato de `import.apply` (campo
`platform`, como hoje); o agente a resolve por nome sem diferenciar maiúsculas (PLT-025). Candidato sem plataforma continua
indo para a chave `windows` (PLT-026), mas a janela nunca o envia assim nas fontes "Arquivo" e "Pasta".
· verificação: teste de componente (nome enviado no scan e nos lotes); teste do agente (plataforma renomeada resolvida).

---

## IMV — Contrato e binding

### IMV-070 — Contrato IPC v0.13.0
Dado o contrato IPC, então ele está na versão 0.13.0 com apenas estes acréscimos sobre a v0.12.0: `import.scan` ganha os
parâmetros opcionais `platform`, `titleFrom` (`file` | `folder`) e `recursive` (ausente = `true`); `ImportCandidate` ganha os
campos opcionais `file`, `extension`, `discs` e `discFiles`; novo bloco `x-import` com as constantes do agente
(`maxScanDepth` 8, `maxScanItems` 20000, `maxScanResultBytes` 921600). Nenhum método, parâmetro obrigatório, resultado ou erro
existente muda. `internal/ipc` expõe os campos novos com os nomes JSON acima.
· verificação: teste de contrato de `internal/ipc` (versão, campos, enum, constantes iguais às do agente).

### IMV-071 — Validação dos parâmetros novos
Dado `import.scan` com `titleFrom` diferente de `file` e `folder`, ou `platform` com mais de 100 caracteres, então a resposta é
`-32602` sem ler nada. Dado `titleFrom` ou `recursive` com uma fonte que não é "Pasta", então são aceitos e ignorados; `platform`
com Steam, Epic ou LaunchBox é aceito e ignorado. Os parâmetros ausentes dão o comportamento padrão (título pelo arquivo,
leitura recursiva, sem plataforma).
· verificação: teste do agente (um caso por regra).

### IMV-072 — `import.apply` grava o título recebido
Dado `import.apply` com `title` diferente do proposto, então o agente valida o título como hoje (aparado, 1 a 200
caracteres, `domain.ValidateTitle`) e grava esse valor; título inválido recusa só aquele candidato (`failed`, IMP-008). Os campos
`file`, `extension`, `discs` e `discFiles` do candidato são aceitos e ignorados (nada novo é gravado). O contrato de
`import.apply` não muda.
· verificação: teste do agente com store real (título editado gravado; título vazio recusado; campos novos ignorados).

### IMV-073 — Binding do desktop
Dado a UI, então `ImportScan` do binding do Wails recebe as opções novas (`platform`, `titleFrom`, `recursive`) e
`ImportCandidateView` carrega os campos novos nos dois sentidos, sem chamar nenhum método além de `import.scan` e
`import.apply`. Mensagens de erro continuam as de UI-008.
· verificação: teste de `cmd/arcadia` (repasse das opções e dos campos nos dois sentidos; fake do agente).

---

## IMV — Invariantes

### IMV-080 — A leitura só olha nomes
Dado qualquer scan de arquivo ou pasta, então nenhum arquivo é aberto ou lido (só entradas de diretório e o tipo de cada
uma); nenhum jogo é lançado, nenhuma sessão rastreada é criada e nada é gravado antes do `import.apply` (IMP-001, IMP-009). A
ligação de trilhas e discos (IMV-020..IMV-023) é só por nome.
· verificação: teste do importador com um `fs` espião/contador (nenhum `Open`) e teste do agente (o scan não cria sessão nem
linha).

### IMV-081 — A UI nunca influencia o tempo de jogo
· alterado em 2026-11-15 (docs/specs/phase-12-metadados-unificados, MET-080): com a caixa "Buscar metadados e imagens"
(Pasta) e o "Obter dados" (Arquivo), a janela de importação também chama os bindings `Metadata*`; continua sem nenhum
lançamento, pausa, retomada ou escrita de sessão.
Dado as interações da fase (ler pasta, marcar, editar título, escolher plataforma, importar em lotes), então a UI só chama
`ImportScan`, `ImportApply`, `PickFile`, `PickFolder` e a leitura do catálogo: nenhum lançamento, `ToggleSession`, pausa,
retomada ou escrita de sessão. O teste de invariantes de UIX-046 (`App.invariants.test.tsx`) ganha essas interações.
· verificação: `App.invariants.test.tsx`; teste do agente (importar não cria sessão rastreada).

### IMV-082 — Onde a fase mexe
Dado o diff da fase, então não há mudança em `internal/tracker`, em `cmd/fake-game`, em `internal/store` (nem query, nem
migração, nem `sqlc generate`), em tabelas de sessão ou em tempo de jogo; as mudanças em Go ficam em `internal/importers`
(normalizador, leitura de pasta, grupos), `internal/agent/imports.go` (opções do scan, plataforma, limite do resultado),
`internal/ipc/import.go` (campos) e `cmd/arcadia/imports.go` (binding); o resto é `frontend` (`ImportDialog`, tabela nova,
`lib/imports.ts`). O contrato passa a v0.13.0 só com o acréscimo de IMV-070.
· verificação: revisão do diff; teste de contrato do IPC.

### IMV-083 — Ponta a ponta
Dado o Arcadia com a pasta real `E:\Games\emulators roms` (ou a cópia de teste do usuário), quando escolho "Pasta", a plataforma
Sony Playstation e leio `ps1` com subpastas, então: os jogos aparecem com títulos limpos e sem o prefixo `NN. `;
`Final Fantasy VIII` e `Resident Evil 2` aparecem uma vez cada, com "N discos"; os `.bin` não aparecem; edito um título,
desmarco um jogo, importo e vejo os jogos na biblioteca com o título editado; ler a mesma pasta de novo marca tudo como "já
existe"; trocar para "Nome da pasta do jogo" mostra os títulos das pastas. Repito com `n64` (jogos soltos, `.zip`) e com uma
pasta de jogos de PC na plataforma Windows (desinstaladores ignorados).
· verificação: manual.

---

## Fora desta spec

- **Busca de metadados e imagens na importação**, coluna de match e limiar de dúvida: fase 12 (a janela de revisão desta
  fase é a base).
- **Gravar os demais discos** (aplicações adicionais do LaunchBox): o modelo não tem. Hoje só o disco 1 (ou o `.m3u`) é o
  alvo; o usuário troca de disco no emulador.
- **Guardar região, revisão e versão** removidas do título (o LaunchBox usa o campo `Version`): o modelo não tem o campo.
- **Converter ` - ` em `: `** e traduzir títulos (busca de metadados, fase 12).
- **Ler o conteúdo de `.cue`, `.m3u` ou `.gdi`** para achar trilhas e discos: a ligação é por nome (IMV-080).
- **Paginação do `import.scan`**, cancelamento da leitura pela UI e **virtualização da tabela**: um resultado cabe em 900 KiB
  e a tabela renderiza todas as linhas.
- **Extensões configuráveis** por usuário: a fonte "Arquivo" cobre extensões fora da lista.
- **Recalcular "já existe" ao editar o título ou a plataforma:** a plataforma descarta a lista (IMV-049); o título editado é
  conferido pelo `import.apply`.
- **Correção de divergências antigas do contrato** (ver pergunta 7).

## Limitações conhecidas

1. **Só o disco 1 é gravado.** Um jogo de vários discos entra com o alvo do disco 1 (ou do `.m3u`); os outros discos aparecem
   na janela e ficam no disco, mas o Arcadia não os guarda. Com `.m3u` o emulador troca de disco sozinho.
2. **Título editado não refaz "já existe".** Se o título editado colide com um jogo da biblioteca, o `import.apply` o recusa
   como `duplicate` e o resumo mostra.
3. **Um jogo por arquivo principal.** Uma pasta com várias ROMs sem marcador de disco (`Pokemon Red.gb`, `Pokemon Blue.gb`) dá
   um candidato por ROM; com "Nome da pasta do jogo" elas ficam com o mesmo título e a segunda vem como "título repetido".
4. **Regras de extensão são fechadas.** Uma ROM de extensão fora da lista (ver `import-rules.md`) é ignorada na pasta e
   contada no aviso; entra pela fonte "Arquivo".
5. **A leitura não é cancelável pela UI** e o diálogo não fecha durante ela (`closeDisabled`, UIX-039); o teto de 20.000 itens
   mantém a leitura em segundos.
6. **O normalizador é só texto:** `Final Fantasy VII (USA) (Disc 1) (v1.1)` fica `Final Fantasy VII`, mas um título que
   tem parênteses de verdade (`Mario & Luigi (Superstar Saga)`) perde o trecho; o usuário corrige na janela de revisão.

## Perguntas ao usuário

**Respondidas:** o usuário aprovou a spec com as recomendações das perguntas 1 a 7 (inclusive corrigir as duas divergências
do contrato na v0.13.0). Cada uma tem uma recomendação, já usada nos requisitos acima.

1. **Título: nome do arquivo (padrão) ou da pasta?** Recomendação: arquivo, com a opção "Nome da pasta do jogo" (decisão 1).
   Conferido no seu LaunchBox: ele usa o nome do arquivo.
2. **Extensões por plataforma** (Windows lê só `.exe/.bat/.cmd`; consoles só ROM e disco; plataforma sua, sem chave, lê os
   dois). Recomendação: sim. Alternativa: aceitar qualquer extensão menos uma lista de ignoradas (`.txt`, `.nfo`, `.sav`,
   imagens...), que na leitura recursiva traz muito lixo (DLLs, saves, capas).
3. **Linhas de título repetido começam desmarcadas** (o roadmap diz "todas marcadas"). Recomendação: desmarcadas, porque o
   `import.apply` as recusaria como duplicata de qualquer jeito; você pode marcar à mão.
4. **Só o disco 1 é gravado** (limitação 1). Recomendação: aceitar nesta fase. Alternativa: guardar os outros discos num campo
   novo (migração; é o "aplicação adicional" do LaunchBox).
5. **Prefixo numérico de pasta** (`31. Spyro...` → `Spyro...`) só quando o título vem da pasta (IMV-006). Recomendação: sim; seu
   layout de ROMs usa esse prefixo.
6. **Limites:** 8 níveis, 20.000 itens, cerca de mil jogos por leitura. Recomendação: sim. Se você tem uma pasta de ROMs com
   mais de mil jogos de uma vez, a leitura mostra os primeiros e pede para importar por subpasta; paginar é uma fase à parte.
7. **Divergências antigas no contrato** `ipc.openrpc.json` v0.12.0 (confira no design §8): `ImportCandidate.launchType` só
   lista `exe` e `uri` (o código aceita `emulator` desde a fase 6b) e `lastPlayed` é `integer` no contrato e texto RFC 3339 no
   fio. Recomendação: corrigir as duas na v0.13.0 junto com o acréscimo (é correção, não quebra ninguém); se você prefere
   "só acréscimos", ficam como dívida registrada.

## ADR proposto

A sessão principal escreve (a spec só o referencia). Próximo número livre: 0032.

1. **0032 — Leitura de pasta por lista fechada de extensões, grupos por pasta do jogo e resultado limitado ao teto do IPC:**
   a fonte "Pasta" lê recursivamente só extensões de jogo conhecidas, escolhidas pela chave da plataforma (Windows =
   executáveis; console = ROM e disco); liga trilhas e discos só por nome (nenhum arquivo é aberto); agrupa discos dentro da
   subpasta de primeiro nível; o título proposto é normalizado no agente (determinístico, sem tocar no nome do arquivo) e o
   usuário o edita na janela de revisão; o `import.scan` devolve no máximo 900 KiB por causa do teto de 1 MiB por mensagem,
   sem paginação. Padrão para a coluna de match da fase 12.
