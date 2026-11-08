# Fase 2 — Esqueleto: requisitos

- **Status:** aprovado (2026-11-08)
- **Design:** [design.md](design.md) · **Tarefas:** [tasks.md](tasks.md)
- **Objetivo:** os dois executáveis compilando, conversando por named pipe e usando o mesmo SQLite, com testes e
  build reproduzíveis. **Nenhuma** lógica de rastreamento de jogos (fase 3).
- **ADRs relacionados:** 0003 (agente separado), 0004 (SQLite), 0006 (biblioteca portátil), 0009 (timestamps em
  milissegundos UTC), 0010 (só o agente migra o schema).

Formato: cada requisito tem ID estável (`ÁREA-NNN`), nunca reutilizado. Critérios em Dado/Quando/Então.
Todo ID precisa de pelo menos um teste que o cite no nome (`t.Run("STO-003/...")`) — exceto os marcados
`verificação: manual`, conferidos pelo reviewer.

---

## Build (BLD)

### BLD-001 — Build sem Node e sem CGO
Dado um clone limpo com só o Go instalado, quando rodo `CGO_ENABLED=0 go build ./...` e `go test ./...`,
então ambos passam. · verificação: manual

## Paths (PTH)

### PTH-001 — Prioridade de resolução
Dado flag `--data-dir`, env `ARCADIA_DATA_DIR` e pasta do exe, quando resolvo os caminhos,
então a flag vence o env, e o env vence `<exe>/data`.

### PTH-002 — Caminhos absolutos
Dado um diretório relativo na flag ou no env, quando resolvo, então `Root`, `DB`, `Media` e `Logs` são absolutos.

### PTH-003 — EnsureDirs idempotente
Dado um `Root` inexistente, quando chamo `EnsureDirs()` duas vezes, então as pastas existem e a segunda chamada não falha.

## Domain (DOM)

### DOM-001 — Tempo ativo sem pausas
Dado início e fim sem pausas, então `ActiveDuration` = fim − início.

### DOM-002 — Pausas subtraídas e recortadas
Dado pausas dentro, sobrepostas entre si ou extrapolando `[início, fim]`, então cada instante pausado é
descontado uma única vez e só dentro do intervalo.
Dada uma pausa fechada com `End < Start`, então ela é ignorada (não desconta nada).
· alterado em 2026-11-08: pausa invertida passa a ser ignorada explicitamente (revisão do planner).

### DOM-003 — Pausa aberta
Dada uma pausa sem fim, então ela conta como pausada até o fim da sessão.

### DOM-004 — Intervalo inválido
Dado fim < início, então `ActiveDuration` retorna 0.

### DOM-005 — Relógio falso
Dado um `FakeClock`, quando chamo `Advance(d)`, então `Now()` avança exatamente `d`.

### DOM-006 — Temporizadores do relógio falso
Dado um `FakeClock` em `T`:
- quando chamo `After(d)` com `d > 0` e depois `Advance(d − 1ns)`, então o canal ainda não recebeu nada;
  quando avanço mais `1ns`, então o canal recebe exatamente um valor, igual ao `Now()` resultante (`T + d`).
- quando chamo `After(d)` com `d <= 0`, então o canal já tem um valor disponível sem nenhum `Advance`.
- dados waiters com prazos `T+1s`, `T+2s` e `T+5s`, quando chamo `Advance(2s)`, então os dois primeiros disparam
  e o terceiro continua pendente.
- quando ninguém lê o canal de um waiter vencido, então `Advance` não bloqueia.
- dado uma goroutine que chama `After(d > 0)`, quando chamo `BlockUntilWaiters(ctx, 1)`, então ele retorna `nil`
  depois que o waiter está registrado; waiters já disparados ou criados com `d <= 0` não contam como pendentes.
- dado nenhum waiter pendente, quando chamo `BlockUntilWaiters(ctx, 1)` e o `ctx` é cancelado, então ele retorna
  `ctx.Err()`.

## Store (STO)

### STO-001 — Migração em banco novo
Dado um arquivo inexistente, quando abro o store **com migração**, então o schema completo da arquitetura (§5)
existe, com FTS5.
· alterado em 2026-11-08: migração passa a ser opção de `Open` (ADR 0010).

### STO-002 — Reabrir é no-op
Dado um banco já migrado, quando abro de novo com migração, então nenhuma migração é reaplicada e os dados permanecem.
· alterado em 2026-11-08: explicita a opção de migração (ADR 0010).

### STO-003 — Seeds de modo de jogo
Dado um banco novo, então `play_mode` contém single, multi_local, multi_online, coop_local e coop_online.

### STO-004 — Chaves estrangeiras ativas
Quando insiro um `game` com `platform_id` inexistente, então a inserção falha.

### STO-005 — Dois acessos concorrentes no mesmo banco
Dados dois `Store` abertos no mesmo arquivo (podem estar no mesmo processo; DSN com `_txlock=immediate`), quando
um escreve e o outro lê em paralelo (transações curtas), então nenhum recebe `SQLITE_BUSY`.
· alterado em 2026-11-08: verificação com dois `Store` no mesmo processo e `_txlock=immediate`.

### STO-006 — Tipos das colunas de tempo
Dado um banco migrado, quando inspeciono o schema (`PRAGMA table_info`), então toda coluna `*_at` é `INTEGER` e
`game.release_date` é `TEXT`. (ADR 0009)

### STO-007 — Conversão de instantes
Dado um instante `t` em fuso não-UTC e com precisão abaixo de milissegundo, quando o converto para coluna e de volta,
então o valor gravado é `t.UnixMilli()` e o lido é `t` truncado em milissegundos, em UTC. Dada uma data
`YYYY-MM-DD`, quando a gravo e leio, então o texto é idêntico. (ADR 0009)

### STO-008 — Abrir sem migrar em schema compatível
Dado um banco migrado pelo binário atual, quando abro o store **sem migração**, então a abertura funciona e as
consultas respondem. (ADR 0010)

### STO-009 — Schema incompatível ao abrir sem migrar
Dado um arquivo inexistente, ou um banco cuja versão do goose difere da esperada pelo binário (menor ou maior),
quando abro **sem migração**, então recebo erro com `errors.Is(err, store.ErrSchemaMismatch)`. (ADR 0010)

### STO-010 — Abrir sem migrar não altera o banco
Dado um banco em versão anterior à esperada, quando abro **sem migração** (e recebo `ErrSchemaMismatch`), então a
versão do goose e as tabelas continuam as mesmas; dado um arquivo inexistente, então ele não é criado. (ADR 0010)

## IPC (IPC)

### IPC-001 — Ping e status
Dado um cliente conectado, quando chamo `agent.ping` e `agent.status`, então recebo `{"pong":true}` e
`version`, `pid`, `startedAt`, `dataDir`, `uptimeSeconds`.

### IPC-002 — Erros JSON-RPC padrão
Quando envio uma linha que não é JSON válido, então recebo `-32700` com `id: null`.
Quando chamo um método inexistente, então recebo `-32601`.
Quando chamo um método sem parâmetros com `params` estruturado não vazio (ex.: `[1]` ou `{"a":1}`), então recebo `-32602`.
· alterado em 2026-11-08: define o que é parâmetro inválido em método sem parâmetros.

### IPC-003 — Mensagem acima do limite
Dado dois clientes conectados, quando um envia uma mensagem com mais de 1.048.576 bytes (sem contar o `\n`), então
o servidor fecha só aquela conexão (resposta não obrigatória), o outro cliente continua recebendo respostas e uma
nova conexão é aceita.
· alterado em 2026-11-08: limite exato, sem resposta obrigatória, isolamento por conexão.

### IPC-004 — Heartbeat
Dado um cliente conectado, então ele recebe `agent.heartbeat` a cada 5 s (relógio injetado).

### IPC-005 — Reconexão
Dado que o servidor reinicia, então o cliente reconecta sozinho e volta a responder.
· alterado em 2026-11-08: a sequência de backoff passou para IPC-013.

### IPC-006 — Encerramento limpo
Quando fecho cliente e servidor, então `Close()` só retorna depois que as goroutines de cada um terminam e nenhuma
goroutine sobra.
· alterado em 2026-11-08: `Close()` aguarda as goroutines; erro pós-`Close` passou para IPC-016.

### IPC-007 — Pipe restrito ao usuário (Windows)
Dado o servidor real, então o pipe se chama `\\.\pipe\arcadia-agent-<SID>`, aceita o usuário atual e uma segunda
escuta no mesmo nome falha.

### IPC-008 — Params vazios aceitos
Quando chamo `agent.ping` com `params` ausente, `null`, `[]` ou `{}`, então recebo o resultado normal em todos os casos.

### IPC-009 — Requisição inválida
Quando envio uma requisição com `params` não estruturado (ex.: `"x"`, `1`, `true`), ou com `jsonrpc` ausente ou
diferente de `"2.0"`, então recebo `-32600`.

### IPC-010 — Batch não suportado
Quando envio um array JSON (batch, vazio ou não), então recebo uma única resposta `-32600` com `id: null` e a
conexão continua utilizável.

### IPC-011 — Notificação do cliente
Quando envio uma requisição sem `id` (para método existente ou não), então o servidor não envia resposta e a próxima
chamada com `id` na mesma conexão é respondida normalmente.

### IPC-012 — Mensagem no limite exato
Quando envio uma mensagem válida com exatamente 1.048.576 bytes (sem contar o `\n`), então ela é processada e
respondida.

### IPC-013 — Sequência de backoff
Dado o cliente sem servidor (relógio injetado), quando ele tenta conectar repetidamente, então as esperas entre
tentativas são 250 ms, 500 ms, 1 s, 2 s, 4 s, 5 s, 5 s… (sem jitter); depois de uma conexão bem-sucedida e nova
queda, a próxima espera volta a 250 ms.

### IPC-014 — Call aguarda conexão
Dado o cliente desconectado, quando chamo `Call` e o servidor sobe antes do prazo do `ctx`, então a chamada é
enviada e respondida; quando o `ctx` expira antes de conectar, então `Call` retorna erro com
`errors.Is(err, ctx.Err())`.

### IPC-015 — Chamada em voo na queda
Dada uma chamada enviada e ainda sem resposta, quando a conexão cai, então `Call` retorna erro com
`errors.Is(err, ipc.ErrConnectionLost)`.

### IPC-016 — Chamada após Close
Dado um cliente fechado com `Close()`, quando chamo `Call`, então recebo erro com `errors.Is(err, ipc.ErrClientClosed)`.

### IPC-017 — Buffer de notificações
Dado um consumidor que não lê `Notifications()`, quando chegam 20 notificações, então o canal contém as 16 primeiras,
as demais são descartadas e `Call` continua sendo respondida na mesma conexão.

## Agente (AGT)

### AGT-001 — Inicialização
Quando rodo `arcadia-agent --data-dir X`, então `X/library.db` migrado, `X/logs/agent.log` e o ícone na bandeja existem.
· verificação: manual

### AGT-002 — Instância única
Dado um agente rodando, quando inicio outro, então o segundo sai com código 0. · verificação: manual

### AGT-003 — Saída ordenada
Quando escolho "Sair" na bandeja, então o agente encerra em < 2 s, sem goroutines nem handles vazando.
· verificação: manual

### AGT-004 — Versão do agente
Dado um binário compilado com `-ldflags "-X main.version=1.2.3"`, quando chamo `agent.status`, então `version` é
`"1.2.3"`; dado um binário sem esse `-X`, então `version` é `"dev"`.

### AGT-005 — Nível de log
Quando inicio o agente com `--log-level` igual a `debug`, `info`, `warn` ou `error`, então o logger usa o nível
correspondente do `slog`; sem a flag, o nível é `info`; com outro valor, o agente sai com código ≠ 0 antes de abrir
o banco.

### AGT-006 — Encerramento com a sessão do Windows
Dado o agente rodando, quando o usuário faz logoff ou desliga o Windows (`WM_ENDSESSION`, recebido por uma janela
top-level oculta), então o agente executa o mesmo shutdown ordenado de AGT-003. · verificação: manual

### AGT-007 — Menu "Abrir Arcadia"
Dado `arcadia.exe` na mesma pasta do agente, quando escolho "Abrir Arcadia" na bandeja, então `arcadia.exe` é
iniciado desacoplado do agente. · verificação: manual

### AGT-008 — "Abrir Arcadia" sem a UI instalada
Dado que não existe `arcadia.exe` na pasta do agente, quando aciono "Abrir Arcadia", então o agente registra um erro
no log e continua rodando, sem tentar criar processo.

## Desktop (DSK)

### DSK-001 — UI inicia o agente
Dado o agente parado, quando abro a UI, então ela inicia o agente desacoplado e conecta em até 5 s.
· verificação: manual

### DSK-002 — Fechar a UI não afeta o agente
Quando fecho a UI, então o agente continua na bandeja. · verificação: manual

### DSK-003 — Reconexão visível
Dado que o agente cai, então a UI mostra "Desconectado" e reconecta quando ele volta. · verificação: manual

### DSK-004 — Comando de início do agente
Dada a UI em `P\arcadia.exe` com `Root` resolvido `R`, quando ela monta o comando de início do agente, então o
executável é `P\arcadia-agent.exe`, os argumentos são `--data-dir R` e as flags de criação são
`DETACHED_PROCESS | CREATE_NEW_PROCESS_GROUP | CREATE_BREAKAWAY_FROM_JOB`.

### DSK-005 — Fallback sem breakaway
Dado que criar o agente com `CREATE_BREAKAWAY_FROM_JOB` falha com `ERROR_ACCESS_DENIED`, quando a UI inicia o
agente, então ela tenta uma segunda vez com as mesmas flags sem `CREATE_BREAKAWAY_FROM_JOB` e registra um aviso no
log; qualquer outro erro não gera segunda tentativa.

### DSK-006 — Agente com outro dataDir
Dado um agente já rodando cujo `agent.status.dataDir` difere do `Root` da UI, quando a UI conecta, então ela registra
um aviso no log e segue conectada normalmente.

### DSK-007 — Banco só depois do agente
Dada a UI iniciando, quando ela ainda não conectou ao agente, então o banco não é aberto; ele é aberto (sem migração)
só depois da primeira conexão bem-sucedida. (ADR 0010)

### DSK-008 — Schema incompatível na UI
Dado que abrir o banco retorna `ErrSchemaMismatch`, quando chamo `LibrarySummary()`, então recebo o erro
"agente ainda não pronto" (sem encerrar a UI) e a próxima chamada tenta abrir o banco de novo. (ADR 0010)

### DSK-009 — Log da UI
Dado `Root` `R`, quando a UI inicia, então ela registra em `R/logs/desktop.log`, com a mesma rotação do agente
(10 MB × 3).

## Frontend (UI)

### UI-001 — Indicador de status
Dado o agente conectado, então o indicador mostra "Conectado" em verde, a versão e o uptime; dado desconectado, então
mostra "Desconectado" em vermelho, sem versão nem uptime.
· alterado em 2026-11-08: textos exatos.

### UI-002 — Estado vazio da biblioteca
Dada uma biblioteca com 0 jogos e M plataformas, então a tela mostra "Nenhum jogo ainda" e "0 jogos · M plataformas".
A contagem é sempre no plural, com o texto literal "N jogos · M plataformas" (ex.: M = 1 → "0 jogos · 1 plataformas").
· alterado em 2026-11-08: textos exatos.
· alterado em 2026-11-08: plural fixo confirmado pelo usuário (sem forma singular).

### UI-003 — Formato do uptime
Dado `uptimeSeconds`, então o texto é: `0` → `0s`; `59` → `59s`; `60` → `1m 00s`; `3599` → `59m 59s`;
`3600` → `1h 00m 00s`; `3723` → `1h 02m 03s`.
· alterado em 2026-11-08: formato entre 1 min e 1 h (`1m 00s` … `59m 59s`) confirmado pelo usuário.

---

## Fora de escopo
Lançar jogos e sessões (fase 3) · hotkeys/XInput/overlay (fase 4) · grade e filtros (fase 5) · importadores (fase 6)
· scrapers (fase 7) · instalador, auto-update, assinatura, CI remoto.

## Questões em aberto
- Caminho definitivo do módulo Go (depende de onde o repositório for hospedado).
- Ícone do app (placeholder nesta fase).
