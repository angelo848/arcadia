# Executável único (`arcadia.exe`, dois processos) — requisitos

- **Status:** aprovado (2026-11-08) — EXE-001..EXE-027, AST-001..AST-011
- **Design:** [design.md](design.md) · **Contrato IPC:** [contracts/ipc.openrpc.json](contracts/ipc.openrpc.json) (v0.3.0)
  · **Tarefas:** [tasks.md](tasks.md)
- **Base:** fase 2 implementada ([phase-2-skeleton](../phase-2-skeleton/requirements.md)).
- **Objetivo:** um só executável `arcadia.exe` que roda em dois modos — UI (padrão) e agente (`--agent`) — em dois
  processos separados; "Sair" na bandeja fecha agente e UI; a UI relança o agente se ele morrer; o agente inicia com
  o Windows (ligado por padrão, desligável na bandeja).
- **ADRs relacionados:** 0003 (agente separado — **substituído em parte** pelo ADR novo de executável único, a
  escrever pela sessão principal), 0006 (biblioteca portátil), 0010 (só o agente migra), 0011 (Clock), 0013 (log fecha
  por último).

Formato: cada requisito tem ID estável (`ÁREA-NNN`), nunca reutilizado. Critérios em Dado/Quando/Então.
Todo ID precisa de pelo menos um teste que o cite no nome (`t.Run("EXE-013/...")`) — exceto os marcados
`verificação: manual`, conferidos pelo reviewer.

Invariantes que **continuam valendo** sem mudança: o agente é o único dono do tempo de jogo e das migrações
(ADR 0010); a UI nunca influencia o tempo; fechar a UI não encerra o agente (DSK-002); instância única do agente
(AGT-002); anti-squatting do pipe (IPC-019); `CGO_ENABLED=0 go build ./...` sem Node (BLD-001).

---

## Requisitos da fase 2 afetados

| ID (fase 2) | Situação | Por |
|---|---|---|
| AGT-001 — Inicialização | substituído | EXE-002 |
| AGT-002 — Instância única | alterado (vale para `arcadia.exe --agent`; critério mantido) | EXE-002 |
| AGT-003 — Saída ordenada | alterado (shutdown notifica os clientes; < 2 s mantido; também disparado por falha do IPC) | EXE-013, EXE-014, EXE-018, EXE-026 |
| AGT-007 — Menu "Abrir Arcadia" | substituído | EXE-009 |
| AGT-008 — "Abrir Arcadia" sem a UI instalada | substituído | EXE-010 |
| DSK-001 — UI inicia o agente | substituído | EXE-008 |
| DSK-003 — Reconexão visível | alterado (a UI também relança o agente) | EXE-019..EXE-025 |
| DSK-004 — Comando de início do agente | substituído | EXE-007 |
| DSK-005 — Fallback sem breakaway | alterado (vale para o comando de EXE-007; regra mantida) | EXE-007 |
| STO-001 — Migração em banco novo | alterado (schema inclui `app_setting`) | AST-001 |

Inalterados e ainda válidos: BLD-001, PTH-*, DOM-*, STO-002..010, IPC-001..019, AGT-004..006, DSK-002, DSK-006..009,
UI-001..002 (UI-001 alterado e UI-003 removido em 2026-11-09: o indicador não mostra mais o uptime).

---

## Modo e build (EXE)

### EXE-001 — Seleção do modo pela linha de comando
Dados os argumentos do processo (sem o nome do exe), quando escolho o modo, então é **agente** se algum argumento for
exatamente `--agent` ou `-agent`, e **UI** caso contrário. Exemplos: `[]` → UI; `["--data-dir","X"]` → UI;
`["--agent"]` → agente; `["--data-dir","X","--agent"]` → agente; `["--agent=false"]` → UI; `["--agents"]` → UI.

### EXE-002 — Modo agente roda só o agente
Quando rodo `arcadia.exe --agent --data-dir X`, então `X/library.db` migrado, `X/logs/agent.log` e o ícone na bandeja
existem, nenhuma janela é aberta, nenhum processo `msedgewebview2.exe` é criado por ele e `X/logs/desktop.log` não é
criado nem escrito. AGT-002 (segundo agente sai com código 0) vale para `arcadia.exe --agent`.
· verificação: manual

### EXE-003 — Flag desconhecida no modo agente
Dado o modo agente, quando os argumentos contêm uma flag que o agente não define (ex.: `--agent --foo`), então o
processo sai com código 2 antes de criar o mutex, a pasta de dados ou abrir o banco. (`--log-level` segue AGT-005.)

### EXE-004 — Modo UI não segura recursos do agente
Dada a UI aberta e nenhum agente rodando, quando um agente sobe (pela UI ou à mão), então ele obtém o mutex
`Local\arcadia-agent` e o pipe normalmente — o processo da UI nunca cria esse mutex nem escuta o pipe.
· verificação: manual

### EXE-005 — Build gera um só executável
Dado um clone com Node e Wails CLI, quando rodo `wails build` em `cmd/arcadia`, então é gerado
`cmd/arcadia/build/bin/arcadia.exe` e nenhum `arcadia-agent.exe`; o repositório não tem mais `cmd/agent` nem
`cmd/desktop`. · verificação: manual

### EXE-006 — Modo agente sem Node
Dado um clone limpo só com Go, quando rodo `CGO_ENABLED=0 go build -o arcadia.exe ./cmd/arcadia` e depois
`arcadia.exe --agent --data-dir X`, então o agente sobe como em EXE-002 (o modo UI desse binário pode falhar por falta
dos assets e das build tags do Wails; isso é aceito). · verificação: manual

## Início do agente pela UI (EXE)

### EXE-007 — Comando de início do agente
Dada a UI rodando a partir do executável `E` (caminho de `os.Executable()`, qualquer nome de arquivo) com `Root`
resolvido `R`, quando ela monta o comando de início do agente, então o executável é `E`, os argumentos são
`--agent --data-dir R` e as flags são `DETACHED_PROCESS | CREATE_NEW_PROCESS_GROUP | CREATE_BREAKAWAY_FROM_JOB`.
O fallback de DSK-005 (segunda tentativa sem `CREATE_BREAKAWAY_FROM_JOB` só em `ERROR_ACCESS_DENIED`, com aviso no log)
vale para este comando.

### EXE-008 — UI inicia o agente
Dado o agente parado, quando abro `arcadia.exe` (sem flags), então um processo `arcadia.exe --agent --data-dir R` é
criado desacoplado da UI e a UI mostra "Conectado" em até 5 s. · verificação: manual

## "Abrir Arcadia" e instância única da UI (EXE)

### EXE-009 — "Abrir Arcadia" lança o próprio executável
Dado o agente rodando a partir do executável `E` com `Root` `R`, quando escolho "Abrir Arcadia" na bandeja, então o
agente cria um processo com executável `E`, argumentos `--data-dir R` (sem `--agent`) e flags
`DETACHED_PROCESS | CREATE_NEW_PROCESS_GROUP`.

### EXE-010 — Falha ao abrir a UI
Dado que criar o processo de EXE-009 falha, quando aciono "Abrir Arcadia", então o agente registra `Error`
"falha ao abrir o Arcadia" com `path` e `error` e continua rodando.

### EXE-011 — Instância única da UI
Dada uma UI aberta (visível, minimizada ou atrás de outras janelas), quando inicio outro `arcadia.exe` sem `--agent`
(pelo Explorer, por atalho ou por "Abrir Arcadia" na bandeja), então o segundo processo sai com código 0 sem abrir
janela, sem iniciar agente e sem escrever em `desktop.log`, e a janela existente é restaurada e trazida à frente (se o
Windows bloquear a troca de foco, o botão dela pisca na barra de tarefas). Em nenhum momento há mais de uma UI do
Arcadia aberta no mesmo usuário. · verificação: manual
· alterado em 2026-11-08: explicitado, na aprovação, que vale também para "Abrir Arcadia" e que nunca há duas UIs

### EXE-012 — Segunda instância retoma o relançamento
Dada a UI que desistiu de relançar o agente (EXE-022), quando ela recebe o aviso de segunda instância, então o
contador de falhas volta a zero, uma nova tentativa (EXE-020) começa sem espera e é registrado `Info`
"segunda instância da UI; trazendo a janela à frente". Dada a UI em qualquer outro estado, então o aviso só gera o log.

## Encerramento ordenado: "Sair" fecha agente e UI (EXE)

### EXE-013 — Notificação `agent.shutdown`
Dado um servidor IPC com clientes conectados, quando o agente inicia o shutdown ordenado, então cada cliente recebe a
notificação `agent.shutdown` (sem `id`) com `params.reason` antes de a conexão dele ser fechada pelo servidor.
(Contrato: [contracts/ipc.openrpc.json](contracts/ipc.openrpc.json).)

### EXE-014 — Drain espera os clientes, com limite
Dado o servidor enviando `agent.shutdown` (relógio injetado), quando todos os clientes conectados fecham as conexões,
então a espera termina com sucesso sem aguardar o limite; quando algum cliente não fecha em 500 ms, então a espera
termina com erro `errors.Is(err, context.DeadlineExceeded)`, o agente registra `Warn` "clientes não fecharam a conexão
a tempo" com `pending` e o shutdown segue (`Server.Close` fecha as conexões restantes).

### EXE-015 — Conexão aceita durante o drain
Dado o servidor em drain, quando um novo cliente conecta, então a primeira mensagem que ele recebe é
`agent.shutdown` com o mesmo `reason`.

### EXE-016 — Motivo do encerramento
Dado o agente rodando, quando o shutdown é disparado por "Sair", por `WM_ENDSESSION`, por Ctrl+C/SIGTERM, pelo fim
do loop da bandeja por conta própria ou pelo fim do `Serve` com erro (EXE-026), então o `reason` do shutdown é,
respectivamente, `quit`, `session-end`, `signal`, `other` e `other`.
· alterado em 2026-11-08: inclui o gatilho de EXE-026 (aprovação)

### EXE-017 — UI fecha ao receber `agent.shutdown`
Dada a UI conectada, quando recebe `agent.shutdown`, então ela registra `Info` "agente encerrado; fechando a UI" com
`reason`, pede o fechamento da janela uma única vez e, mesmo que a conexão caia em seguida, não inicia o agente.

### EXE-018 — "Sair" encerra agente e UI
Dados o agente e a UI conectada abertos (janela visível, minimizada ou em segundo plano), quando escolho "Sair" na
bandeja, então a janela da UI fecha sem pedir confirmação, os dois processos terminam e o agente termina em < 2 s
(AGT-003), sem nenhum agente novo ser criado. "Sair" é sempre o encerramento do Arcadia inteiro, nunca só do agente.
· verificação: manual
· alterado em 2026-11-08: explicitado, na aprovação, que "Sair" fecha também a UI em qualquer estado da janela

### EXE-026 — Agente sem IPC encerra
Dado o agente rodando (antes ou depois de a bandeja inicializar), quando `Server.Serve` retorna erro que não vem de
`Close` nem do fim do `ctx` (IPC-018: 10 falhas seguidas de `Accept`, ou `net.ErrClosed` sem `Close`), então o agente
registra `Error` "IPC indisponível; encerrando o agente" com `error`, encerra a bandeja e faz o shutdown ordenado com
`reason = other` (EXE-016) na ordem e no prazo de AGT-003, e o processo sai com código 1 mesmo que o shutdown não
tenha outro erro. Consequência na UI (sem requisito novo): como `Serve` já fechou todas as conexões antes de retornar,
nenhum cliente recebe `agent.shutdown`; a UI vê uma queda sem aviso e aplica EXE-020..EXE-023.

### EXE-027 — Drain com `Serve` já encerrado
Dado um `ipc.Server` cujo `Serve` já retornou (por erro, `Close` ou fim do `ctx`), quando chamo `Drain`, então ele
retorna `nil` imediatamente, sem esperar o `ctx` nem o limite de 500 ms e sem enviar nada (não há conexões abertas nem
novas conexões aceitas).

## Queda do agente sem aviso: a UI relança (EXE)

### EXE-019 — Estado de conexão do cliente IPC
Dado um `ipc.Client`, quando a conexão sobe ou cai, então `Connected()` reflete o estado novo e `StateChanged()`
recebe um sinal (canal com buffer 1; sinais seguidos se acumulam em um, sem bloquear o cliente). Dada uma notificação
recebida antes de a conexão cair, quando o sinal da queda é observado, então essa notificação já está disponível em
`Notifications()`.

### EXE-020 — Queda sem aviso relança o agente
Dada a UI conectada há ≥ 60 s (relógio injetado), quando a conexão cai sem `agent.shutdown`, então a UI registra
`Warn` "agente desconectado sem aviso de encerramento", espera 1 s e, se continuar desconectada, faz uma tentativa:
registra `Info` "relançando o agente" (`attempt`, `max` = 3) e executa a sequência de EXE-008 (sonda o pipe; sem
resposta, inicia o agente com o comando de EXE-007 e espera até 5 s).

### EXE-021 — Volta espontânea não relança
Dada a UI esperando para relançar (EXE-020, EXE-022), quando a conexão volta antes do fim da espera, ou quando a
sondagem da tentativa recebe resposta, então o agente não é iniciado e é registrado `Info` "agente reconectado".

### EXE-022 — Limite e espera entre tentativas
Dada a UI relançando o agente (relógio injetado), quando as tentativas falham seguidamente, então as esperas antes das
tentativas são 1 s, 2 s e 4 s (indexadas pelo número de falhas seguidas: 0, 1, 2); cada falha registra `Error`
"falha ao relançar o agente" (`attempt`, `error`); na 3ª falha seguida a UI registra `Error` "desistindo de relançar o
agente", não faz mais tentativas e continua mostrando "Desconectado" (o `ipc.Client` continua reconectando sozinho, e um
agente iniciado por fora é detectado como em EXE-021).

### EXE-023 — Conexão curta conta como falha
Dada uma conexão com o agente que durou menos de 60 s, quando ela cai sem `agent.shutdown`, então isso conta como uma
falha seguida para EXE-022 e é registrado `Warn` "agente caiu logo após subir"; dada uma conexão que durou ≥ 60 s,
então o contador de falhas volta a zero ao cair.

### EXE-024 — Falha no início da UI conta como tentativa
Dada a UI iniciando, quando a sequência de EXE-008 falha (sem resposta em 5 s ou erro ao criar o processo), então isso
conta como a 1ª falha seguida e a próxima tentativa segue EXE-022 (espera de 2 s).

### EXE-025 — Fechar a UI cancela o relançamento
Dada a UI esperando uma tentativa ou no meio dela, quando a UI é fechada, então nenhum agente é iniciado depois do
início do `OnShutdown` e o `OnShutdown` só retorna depois que a goroutine de supervisão terminou.

---

## Início com o Windows (AST)

### AST-001 — Tabela de preferências
Dado um banco novo ou na versão 1, quando o agente abre o store com migração, então existe a tabela
`app_setting(key TEXT PRIMARY KEY, value TEXT NOT NULL, updated_at INTEGER NOT NULL)`, a versão do goose é 2
(`store.SchemaVersion = 2`) e os dados existentes permanecem. Dada uma chave inexistente, quando a leio, então recebo
`errors.Is(err, store.ErrSettingNotFound)`; quando gravo uma chave duas vezes, então a leitura devolve o último valor.
· alterado em 2026-11-09: `store.SchemaVersion` passa de 2 para 3 (a tabela `app_setting` e o resto se mantêm) — ver
STO-011 (docs/specs/phase-3-tracker).

### AST-002 — Comando gravado na entrada Run
Dado o executável `E` na pasta `D` e `Root` `R`, quando monto o valor da entrada Run, então ele é `E` sempre entre
aspas, seguido de ` --agent`; se `R` ≠ `D\data` (comparação com `filepath.Clean`, sem diferenciar maiúsculas), segue
` --data-dir ` e `R` escapado pelas regras de `windows.EscapeArg`. Exemplos:
`C:\Jogos\Arcadia\arcadia.exe`, `R = C:\Jogos\Arcadia\data` → `"C:\Jogos\Arcadia\arcadia.exe" --agent`;
`R = E:\Minha Biblioteca` → `"C:\Jogos\Arcadia\arcadia.exe" --agent --data-dir "E:\Minha Biblioteca"`;
`R = D:\` → `... --agent --data-dir D:\`.

### AST-003 — Primeira execução liga o início com o Windows
Dado um banco sem a chave `autostart` (build de release), quando o agente sobe, então grava `autostart = "true"` e
escreve em `HKCU\Software\Microsoft\Windows\CurrentVersion\Run` o valor `Arcadia` (REG_SZ) com o comando de AST-002.

### AST-004 — Ligado: subida corrige a entrada
Dado `autostart = "true"`, quando o agente sobe e o valor `Arcadia` não existe ou difere do comando de AST-002, então o
valor é regravado com o comando atual (ex.: exe movido junto com a biblioteca); quando é igual, então nada é escrito no
registro.

### AST-005 — Desligado: subida só remove a entrada própria
Dado `autostart = "false"`, quando o agente sobe e o valor `Arcadia` aponta para o mesmo executável (primeiro token da
linha de comando igual a `E`, sem diferenciar maiúsculas), então o valor é removido; quando aponta para outro
executável ou não existe, então nada é alterado no registro.

### AST-006 — Item da bandeja reflete a preferência
Dado o agente rodando, quando abro o menu da bandeja, então há o item marcável "Iniciar com o Windows" entre
"Abrir Arcadia" e o separador de "Sair", marcado se e só se `autostart = "true"`. · verificação: manual

### AST-007 — Desmarcar desliga
Dado `autostart = "true"`, quando aciono "Iniciar com o Windows", então `autostart = "false"` é gravado, o valor
`Arcadia` é removido segundo a regra de AST-005 e o item fica desmarcado.

### AST-008 — Marcar liga
Dado `autostart = "false"`, quando aciono "Iniciar com o Windows", então `autostart = "true"` é gravado, o valor
`Arcadia` é escrito com o comando de AST-002 e o item fica marcado.

### AST-009 — Falha de registro ou banco não derruba o agente
Dado que ler/escrever o registro ou a chave `autostart` falha (na subida ou no clique), quando a operação ocorre, então
o agente registra `Error` com `op` e `error` e continua rodando. Se a gravação da preferência falhou no clique, o item
mantém o estado anterior e o registro não é tocado; se só o registro falhou, a preferência gravada vale e o item mostra
o estado novo (a próxima subida tenta de novo por AST-004/AST-005).

### AST-010 — Build de desenvolvimento não toca no registro
Dado um binário sem `-X main.version` (`version == "dev"`), quando o agente sobe ou o item é acionado, então nada é
lido nem escrito no registro nem na chave `autostart`, o item "Iniciar com o Windows" aparece desabilitado e desmarcado
e é registrado `Info` "início com o Windows desativado em build de desenvolvimento" uma vez na subida.

### AST-011 — Login abre só o agente
Dado `autostart = "true"` e um build de release, quando faço logon no Windows, então só o agente sobe (ícone na bandeja,
nenhuma janela da UI), usando o `Root` do comando gravado. · verificação: manual

---

## Fora de escopo
Instalador e atualização do exe · ler/gravar `HKCU\...\Explorer\StartupApproved\Run` (desativação pelo Gerenciador de
Tarefas/Configurações) · tela de configurações na UI · migração automática de entradas antigas que apontem para
`arcadia-agent.exe` (ver limitações).

## Limitações conhecidas
Aceitas pelo usuário na aprovação (2026-11-08); não geram requisito nem teste.

1. **Duas cópias do Arcadia no mesmo usuário** (pastas diferentes) disputam o valor `Arcadia` da entrada Run (vence a
   última cópia ligada que subiu; AST-005 impede que uma cópia desligada apague a entrada da outra) e compartilham a
   instância única da UI (abrir a cópia B traz a janela da A, EXE-011).
2. **Entrada Run antiga apontando para `arcadia-agent.exe`:** nada é feito. A fase 2 não gravava a entrada Run, então
   ela só existiria se criada à mão; o usuário a remove pelo Gerenciador de Tarefas/Configurações.
3. **"Sair" com a UI sem conexão de cliente:** se, no momento do "Sair", a UI estiver travada ou ainda na sondagem
   inicial (EXE-008, antes de o `ipc.Client` conectar), ela pode não ler `agent.shutdown` em 500 ms (EXE-014), tratar
   o fim do agente como queda e relançá-lo (EXE-020). Janela pequena; um novo "Sair" resolve (design.md, Riscos).
4. **Relançamento logo após EXE-026:** a primeira tentativa da UI (1 s depois da queda) pode encontrar o agente antigo
   ainda segurando o mutex durante o shutdown; o agente novo sai com 0 (AGT-002), a tentativa conta como falha
   (EXE-022) e a seguinte (2 s depois) sobe normalmente. Se o IPC falhar de novo de forma persistente, a UI desiste
   após 3 falhas seguidas e mostra "Desconectado".
