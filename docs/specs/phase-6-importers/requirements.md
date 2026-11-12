# Fase 6 — Importadores e rastreio fora da árvore: requisitos

- **Status:** rascunho, aguardando aprovação do usuário (três decisões de produto abaixo)
- **Base:** [phase-5-1-ajustes](../phase-5-1-ajustes/requirements.md) (aceita), [phase-3-tracker](../phase-3-tracker/requirements.md)
  (limitação 2: jogos que saem em segundos; limitação 3: filhos com breakaway) e
  [phase-4-pause](../phase-4-pause/requirements.md).
- **ARCHITECTURE:** §4.2 camadas 2 e 3, §5 (modelo), §7 (importadores), §9 (roadmap: fase 6).
- **ADRs relacionados:** 0005 (rastreamento em camadas), 0015 (agente único escritor), 0016 (breakaway), 0019
  (lançamento sem rastrear), 0023 (agente elevado).
- **Objetivo:** trazer a biblioteca que o usuário já tem (Steam, Epic, pastas de jogos e LaunchBox, com tempo jogado),
  e rastrear jogos que o cliente da loja abre fora da árvore do Arcadia.

Formato: cada requisito tem ID estável (`ÁREA-NNN`), nunca reutilizado. Critérios em Dado/Quando/Então.
Áreas novas: **IMP** (importação), **RUL** (regras de rastreio e watcher), **STM** (sinal da Steam).
Todo ID precisa de pelo menos um teste que o cite no nome, exceto os marcados `· verificação: manual`.

---

## Decisões de produto (recomendação indicada)

1. **Fontes desta fase: Steam, Epic, pasta/executável avulso e LaunchBox.** GOG e pasta de ROMs com emulador ficam para
   a fase 6b (a arquitetura lista as duas, mas o LaunchBox já cobre a migração de quem usa emulador).
2. **Watcher: regras 1 e 2 da ARCHITECTURE §4.2 (nome/caminho e pasta de instalação) mais o sinal da Steam (STM-001).**
   A heurística 3 ("primeira janela em tela cheia → pergunta 'Este é o jogo?'") fica para depois: exige um fluxo de
   pergunta na UI e é a mais frágil.
3. **Tempo jogado importado entra como uma sessão de origem `import`** (uma por jogo, com a duração e a data da última
   partida do LaunchBox). Assim `total_play_seconds` continua sendo a soma das sessões (EDT-005) e nenhum campo
   paralelo aparece. Sessões rastreadas nunca são alteradas.

---

## IMP — Importação

### IMP-001 — Importar é em dois passos: ver, depois confirmar
Dado uma fonte escolhida, quando o usuário pede a leitura (`import.scan`), então o agente devolve os candidatos sem gravar
nada. Dado que o usuário marca candidatos e confirma (`import.apply`), então só os marcados são gravados, numa transação
por jogo. Nenhuma gravação acontece no `scan`.

### IMP-002 — Steam
Dado a pasta de instalação da Steam, quando leio `steamapps\libraryfolders.vdf` e os `appmanifest_<appid>.acf` de cada
biblioteca, então cada app instalado vira um candidato com `name` (campo `name`), `appid`, `installdir` e plataforma PC.
O candidato tem `launch_type = uri` e alvo `steam://rungameid/<appid>`. Dado uma biblioteca inexistente, então o scan
não falha: devolve zero candidatos e um aviso.

### IMP-003 — Epic
Dado `C:\ProgramData\Epic\EpicGamesLauncher\Data\Manifests\*.item`, quando leio cada arquivo (JSON), então cada item com
`AppName`, `DisplayName` e `InstallLocation` vira candidato PC com `launch_type = uri` e alvo
`com.epicgames.launcher://apps/<AppName>?action=launch&silent=true`. Itens sem `DisplayName` são ignorados e contados no
aviso.

### IMP-004 — Pasta ou executável avulso
Dado uma pasta escolhida pelo usuário, quando o scan roda, então cada `.exe` de primeiro nível vira candidato com
`launch_type = exe`, alvo = caminho do executável e título = nome do arquivo sem `.exe`. Dado que a pasta tem
subpastas, então só o primeiro nível é lido nesta fase (escopo explícito, para evitar falsos positivos como
instaladores e utilitários). Os candidatos ficam sem plataforma definida; a plataforma é escolhida na confirmação.

### IMP-005 — LaunchBox
Dado a pasta `Data\Platforms` do LaunchBox, quando leio os `*.xml` de plataforma, então cada `<Game>` vira candidato com
título, plataforma, desenvolvedora, editora, gêneros, ano, descrição, modos de jogo e o caminho do executável. Quando
houver `PlayCount` e `LastPlayedDate`, então a sessão de IMP-007 é proposta; sem eles, o jogo é importado sem tempo.
· verificação: manual (com o XML real do usuário). Os campos exatos são confirmados no design, com fixture de teste.

### IMP-006 — Duplicatas
Dado um candidato que já existe na biblioteca (mesmo `appid` ou `AppName`, mesmo caminho de executável, ou mesmo título
e plataforma sem diferença de maiúsculas), então ele aparece marcado "já existe" e não é gravado de novo. Rodar a mesma
importação duas vezes não duplica nenhum jogo.

### IMP-007 — Tempo jogado importado
Dado um candidato com tempo jogado (LaunchBox), quando o usuário confirma, então é gravada uma sessão de origem `import`
com a duração total (`active_seconds`) e `ended_at` na data da última partida. `total_play_seconds` passa a incluí-la.
Dado um jogo que já tem sessões rastreadas, então a importação de tempo não altera essas sessões.

### IMP-008 — Falha isolada
Dado uma fonte inexistente ou um arquivo ilegível, então essa fonte devolve um aviso e as outras continuam. Dado que a
gravação de um jogo falha, então só esse jogo fica de fora e o erro é listado; os demais já gravados permanecem.

### IMP-009 — Não lança nem toca no jogo
Dado qualquer importação, então nenhum jogo é lançado, nenhuma sessão rastreada é criada e nenhum arquivo do jogo é
lido além dos manifestos e do `.exe` de primeiro nível. Os caminhos importados não são alterados depois.

### IMP-010 — Tela de importação
Dado o botão "Importar", quando o usuário escolhe uma fonte, então a UI mostra os candidatos com caixas de seleção, a
marcação "já existe" (IMP-006) e o aviso de cada fonte (IMP-008). "Importar selecionados" fica desabilitado sem
nenhum marcado e sem candidatos válidos. · verificação: manual (UI).

---

## RUL — Regras de rastreio e watcher

### RUL-001 — Regra de rastreio por jogo
Dado um jogo importado com pasta de instalação (Steam, Epic, pasta avulsa), quando a importação é confirmada, então uma
regra `install_dir` com essa pasta é gravada em `tracking_rule` (`learned = false`). Dado um jogo com `exe_path`, então
uma regra `exe_path` com o caminho é gravada. Nenhuma regra é gravada para jogo sem essas informações.

### RUL-002 — Adoção de processo fora da árvore
Dado um jogo com sessão iniciada pelo Arcadia (URI ou exe) e um processo novo que casa com uma regra do jogo (RUL-001) ou
com o nome do executável, quando o watcher vê esse processo num snapshot de ~1,5 s, então o processo é adotado pela
sessão: é aberto com `SYNCHRONIZE` e aguardado. O snapshot não exige privilégio de administrador (mas o agente já
é elevado, ADR 0023).

### RUL-003 — Sessão de lançamento por URI
Dado um jogo com `launch_type = uri`, quando o usuário toca "Jogar", então a sessão entra em `Launching` (mesmo estado
da fase 3), o URI é aberto pelo Windows e o agente aguarda até 60 s por um processo adotável (RUL-002). Dado que nenhum
processo casa nesse prazo, então a sessão é `discarded` com o motivo "o cliente não abriu o jogo", sem tempo gravado.

### RUL-004 — Fim da sessão adotada
Dado uma sessão adotada, quando o processo adotado sai, então a sessão entra em carência de 15 s (como hoje). Dado que
um processo que casa com a regra aparece de novo nesse prazo, então a sessão continua. Dado o fim da carência, então a
sessão termina com o instante de saída do último processo casado.

### RUL-005 — Pausa e demais regras
Dado uma sessão adotada, então a pausa por teclado e controle (fase 4) funciona da mesma forma, e a máquina de estados
(§4.3) não muda. Dado uma sessão adotada, então o Job Object não existe: pausa e filhos são limitados ao que o watcher
enxerga, e a página do jogo mostra "rastreio por processo" nessa sessão.

### RUL-006 — Regras aprendidas ficam para depois
Dado a heurística 3 (ARCHITECTURE §4.2), então ela não é implementada nesta fase: `learned` fica sempre `false`. A
coluna existe para a fase seguinte.

---

## STM — Sinal da Steam

### STM-001 — Sinal de execução da Steam
Dado um jogo com `appid`, quando o agente lê `HKCU\Software\Valve\Steam\Apps\<appid>\Running` a cada snapshot, então
`Running = 1` confirma o início e `Running = 0` depois de adotado confirma o fim (ARCHITECTURE §4.2, camada 3). O sinal
não substitui o watcher: sem ele, RUL-002 e RUL-004 valem igual. Dado a chave ausente, então o sinal é ignorado sem erro.
· verificação: manual (Steam instalada).

---

## Fora de escopo nesta fase

- GOG Galaxy e pasta de ROMs com emulador (fase 6b).
- Heurística 3 e a pergunta "Este é o jogo?" (RUL-006).
- ETW e eventos instantâneos (upgrade futuro, ARCHITECTURE §4.2).
- Mídia do LaunchBox e de qualquer fonte (fase 7). Só metadados e tempo jogado.
- Matching por provedor de metadados (fase 7).

---

## Perguntas para o usuário

As três decisões do topo usam a recomendação. Se você quiser outra, o requisito muda antes da aprovação:

1. **Fontes desta fase:** Steam, Epic, pasta/exe avulso e LaunchBox (recomendado), ou incluir GOG e ROMs também?
2. **Watcher:** regras 1 e 2 mais o sinal da Steam (recomendado), ou incluir a heurística 3 com a pergunta "Este é o jogo?"?
3. **Tempo do LaunchBox:** sessão de origem `import` (recomendado), ou um campo separado somado ao total?
