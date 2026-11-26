# Fase 16 — Edição de sessões: requisitos

- **Status:** aprovado em 2026-11-25 (todas as respostas padrão das perguntas 1 a 9 foram confirmadas; ver "Decisões confirmadas" no
  fim).
- **Pedido:** "adicione opções para editar, deletar e criar sessões de um game".
- **Design:** [design.md](design.md) · **Contratos:** [contracts/ipc-v0.17.0-delta.md](contracts/ipc-v0.17.0-delta.md)
  ([OpenRPC do acréscimo](contracts/ipc-v0.17.0-delta.openrpc.json)) · **Tarefas:** [tasks.md](tasks.md) (do planner).
- **Base:** [phase-3-tracker](../phase-3-tracker/requirements.md) (STO-012..STO-017, DOM-008, SES-002),
  [phase-4-pause](../phase-4-pause/requirements.md) (pausas, ADR 0017), [phase-5-library](../phase-5-library/requirements.md)
  (STO-027, IPC-047, IPC-050, UI-038, UI-042), [phase-6-importers](../phase-6-importers/requirements.md) (sessão importada,
  IMP-007), [phase-8-estatisticas](../phase-8-estatisticas/requirements.md) (EST-001..EST-006) e
  [phase-14-edicao-manual](../phase-14-edicao-manual/requirements.md) (EDM-005, `game.setPlayTime`).
- **ADRs:** 0009 (instantes em ms UTC), 0011 (Clock), 0012 (enums validados no `domain`), 0015 (o agente é o único
  escritor; a UI pede por IPC), 0017 (modelo de pausa), 0031 (avisos de mudança). Decisão nova que merece ADR: ver design §9.
- **Objetivo:** na lista de sessões do painel de detalhes do jogo, criar uma sessão à mão, editar início, fim e tempo
  jogado de uma sessão encerrada e excluir uma sessão encerrada, mantendo coerentes o tempo total, a primeira e a última
  vez jogado, a contagem de sessões e as estatísticas. **Sem mudança no tracker** (`internal/tracker/` e `cmd/fake-game/`
  não mudam) e **sem influência no tempo da sessão em andamento**.

Formato: IDs `ÁREA-NNN`, estáveis e nunca reutilizados; critérios Dado/Quando/Então. Todo ID precisa de pelo menos um teste
que o cite no nome (`t.Run("SED-004/...")`, `it("SEL-005 ...")`), exceto os marcados `· verificação: manual`. `T` é o
`Now()` de um `FakeClock` do agente. Áreas novas: **SED** (regras, store, agente, IPC e desktop da edição de sessões) e
**SEL** (lista de sessões e formulário na UI).

Vocabulário desta spec:
- **Sessão encerrada:** `ended_at` não nulo (`state` `ended` ou `discarded`). **Sessão aberta** (em andamento): `ended_at`
  nulo (`running`, `paused` ou `grace`).
- **Sessão contada:** `state = 'ended'`. Só ela soma no jogo (STO-014, STO-015).
- **Origem** (`play_session.source`): `tracked` (rastreada pelo agente), `import` (LaunchBox, IMP-007) ou `manual` (criada
  nesta fase).
- **Intervalo** de uma sessão: `fim − início`. **Tempo jogado:** `active_seconds`.
- **Tempo das sessões** do jogo: soma de `active_seconds` das suas sessões contadas. **Ajuste manual:** `total_play_seconds`
  menos o tempo das sessões (vem de EDM-005; pode ser negativo).

---

## Decisões (confirmadas em 2026-11-25; ver "Decisões confirmadas" no fim)

1. **Campos editáveis:** início, fim e tempo jogado. Sem nota/comentário nesta fase (pergunta 1).
2. **Origem visível:** sessão criada à mão tem origem `manual`; sessão rastreada ou importada que o usuário edita mantém a
   origem e ganha a marca "editada" (`edited_at`). A lista mostra "Manual", "Importada" e "Editada".
3. **Total por diferença:** criar, editar e excluir mudam `total_play_seconds` pela diferença do tempo jogado, preservando
   o ajuste manual de EDM-005. Primeira e última vez jogado e a contagem de sessões são recalculadas das sessões contadas.
4. **Contagem dupla com o ajuste manual:** ao criar, a caixa "Já está no tempo total" (só aparece quando há ajuste manual
   positivo) cria a sessão sem mudar o total (pergunta 3).
5. **Sobreposição:** recusada contra qualquer sessão rastreada ou manual, de qualquer jogo, inclusive a aberta; sessões
   importadas e descartadas não entram na conta (pergunta 4).
6. **Limites:** intervalo de até 24 h (pergunta 2), tempo jogado de 30 s até o intervalo, fim nunca no futuro.
7. **Sessão aberta:** nunca é editada nem excluída. Com o jogo rodando, as outras sessões dele continuam editáveis.
8. **Sessão descartada:** só pode ser excluída (pergunta 5).
9. **Exclusão:** pede confirmação e é irreversível (sem lixeira nem desfazer, pergunta 6).
10. **Estatísticas:** sessões manuais contam como tempo jogado, marcadas "adicionadas à mão", como as importadas (EST-005).
11. **Sem aviso IPC novo:** a própria UI recarrega o que mudou (IPC-050, UI-042).

---

## Requisitos de fases anteriores afetados

| ID | Situação | Por |
|---|---|---|
| STO-016 — Total recalculável (fase 3) | alterado: o total deixa de ser sempre igual à soma das sessões contadas; a diferença é o ajuste manual (EDM-005 já quebrava a igualdade) | SED-007, SED-010, SED-013 |
| STO-027 — Sessões de um jogo (fase 5) | alterado: cada item ganha `source` e `editedAt` | SED-033 |
| IPC-050 — Sem notificação de biblioteca (fase 5) | alterado: vale também para `session.create`, `session.update` e `session.delete` | SED-026 |
| UI-038 — Sessões na página do jogo (fase 5) | alterado: botões de criar, editar e excluir e marcas de origem | SEL-001..SEL-004 |
| EST-005 — Sessões importadas (fase 8) | alterado: as sessões manuais entram do mesmo jeito, com marca própria | SED-027, SEL-014 |
| EDM-005 — Editar o tempo total jogado (fase 14) | inalterado (continua substituindo o total; as sessões não mudam) | SED-034, SEL-009 |
| ARCHITECTURE §5 (`source[tracked\|manual]`) | corrigido na documentação: `source[tracked\|import\|manual]`, mais `edited_at` | SED-002, SED-001 |

As anotações "· alterado em" nos arquivos das fases antigas são feitas depois da aprovação deste arquivo.

---

## Domínio e store (SED)

### SED-001 — Migração 11: marca de edição
Dado um banco novo ou na versão 10 com dados, quando o agente abre o store com migração, então `play_session` ganha a coluna
`edited_at INTEGER` (nula), existe o índice `play_session_started_at_idx` em `(started_at)`, `store.SchemaVersion = 11`, a
versão do goose é 11 e as sessões existentes ficam com `edited_at` nulo e os outros valores iguais. O `Down` apaga o índice e
a coluna.

### SED-002 — Origem da sessão validada no domínio
Dado o texto de `play_session.source`, quando chamo `domain.ParseSessionSource`, então `tracked`, `import` e `manual` são
aceitos e qualquer outro valor (inclusive `""` e `Manual`) retorna erro (ADR 0012). A leitura da lista é tolerante: valor
desconhecido no banco chega à UI como está e não derruba a página.

### SED-003 — Criar sessão manual grava a linha
Dado o jogo `G` e início `S`, fim `E` e tempo jogado `a` válidos (SED-015..SED-020), quando o agente cria a sessão no instante
`W`, então existe uma linha em `play_session` com `game_id = G`, `started_at = S`, `ended_at = E`, `last_seen_at = E`,
`active_seconds = a`, `state = 'ended'`, `source = 'manual'`, `end_reason` nulo e `edited_at` nulo, sem `pause_interval`.

### SED-004 — Criar soma o tempo ao total do jogo
Dado o jogo `G` com total `X`, quando uma sessão manual com tempo jogado `a` é criada sem "Já está no tempo total", então
`total_play_seconds` passa a `X + a` (com o teto de SED-013), a primeira e a última vez jogado e a contagem seguem SED-012, e
`updated_at = W`.

### SED-005 — "Já está no tempo total" não muda o total
Dado o jogo `G` com total `X` e tempo das sessões `Y`, com `X − Y ≥ a`, quando uma sessão manual com tempo jogado `a` é criada
com `includedInTotal = true`, então `total_play_seconds` continua `X` e a primeira e a última vez jogado e a contagem seguem
SED-012. Dado `X − Y < a`, então o pedido é recusado com `-32602` (`reason = exceeds-adjustment`) e nada muda.

### SED-006 — Editar sessão grava os campos pedidos
Dada uma sessão encerrada contada (`state = 'ended'`), de qualquer origem, quando o agente aplica um pedido com qualquer
subconjunto não vazio de início, fim e tempo jogado no instante `W`, então só os campos presentes mudam (`last_seen_at`
acompanha o fim), `source`, `state` e `end_reason` ficam como estavam e `edited_at = W`.

### SED-007 — Editar ajusta o total pela diferença
Dada uma sessão contada do jogo `G` com tempo jogado `a`, e `G` com total `X`, quando o tempo jogado passa a `a'`, então
`total_play_seconds` passa a `X + (a' − a)` (com os limites de SED-013), e a primeira e a última vez jogado e a contagem seguem
SED-012. Exemplo: `X = 10 h` (8 h de sessões e 2 h de ajuste), sessão de 1 h editada para 1 h 30 → `X = 10 h 30`.

### SED-008 — Pedido de edição sem mudança não grava
Dado um pedido de edição em que todos os campos presentes são iguais aos gravados, quando o agente o recebe, então nada é
gravado (nem `edited_at`, nem `updated_at` do jogo) e a resposta traz a sessão e o jogo como estão.

### SED-009 — Editar início ou fim corta as pausas
Dada uma sessão rastreada com intervalos de pausa, quando o início ou o fim mudam, então cada pausa que sai do novo intervalo
`[início, fim]` é cortada nas bordas dele, e a pausa que fica com duração zero (ou totalmente fora) é apagada. Mudar só o
tempo jogado não mexe nas pausas. As pausas não recalculam o tempo jogado: numa sessão editada, `active_seconds` é o valor
gravado (ADR 0017 continua valendo para as sessões não editadas).

### SED-010 — Excluir sessão contada subtrai do total
Dada uma sessão contada do jogo `G` com tempo jogado `a`, e `G` com total `X`, quando o agente a exclui, então a linha e suas
pausas saem do banco, `total_play_seconds` passa a `X − a` (nunca abaixo de 0, SED-013), a primeira e a última vez jogado e
a contagem seguem SED-012 e `updated_at = W`.

### SED-011 — Excluir sessão descartada não muda o jogo
Dada uma sessão descartada do jogo `G`, quando o agente a exclui, então a linha e suas pausas saem do banco e a linha de `G`
não muda (nem `updated_at`).

### SED-012 — Primeira vez, última vez e contagem vêm das sessões contadas
Dado o jogo `G` depois de criar, editar ou excluir uma sessão dele, então, na mesma transação, `first_played_at` é o menor
`started_at`, `last_played_at` é o maior `ended_at` e `play_count` é a quantidade das sessões contadas de `G`. Dado `G` sem
nenhuma sessão contada, então `first_played_at` e `last_played_at` ficam nulos e `play_count = 0`. A sessão aberta não entra
nesse cálculo (ela soma quando termina, STO-014).

### SED-013 — Limites do total
Dada uma operação que levaria `total_play_seconds` abaixo de 0, então ele fica 0; acima de `ipc.MaxPlaySeconds`
(3 599 996 400), então fica no máximo. Exemplo: total ajustado à mão para 1 h (EDM-005) e exclusão de uma sessão de 3 h →
total 0.

### SED-014 — Tudo numa transação
Dado qualquer passo de criar, editar ou excluir que falha (validação dentro da transação, sessão, pausas ou jogo), então nada
muda no banco: nem a sessão, nem as pausas, nem o jogo.

### SED-015 — Fim depois do início
Dado um pedido de criação ou edição em que o fim resultante não é posterior ao início resultante, quando o agente valida, então
o pedido é recusado com `-32602` (`reason = end-not-after-start`) e nada muda.

### SED-016 — Nada no futuro
Dado um pedido de criação ou edição em que o fim resultante é posterior a `T` (relógio do agente), então o pedido é recusado com
`-32602` (`reason = in-future`). Fim igual a `T` é aceito.

### SED-017 — Início mínimo
Dado um pedido de criação ou edição com início resultante antes de 1980-01-01T00:00:00Z, então o pedido é recusado com `-32602`
(`reason = too-early`).

### SED-018 — Intervalo de até 24 h
Dado um pedido de criação, ou de edição que traz início ou fim, de uma sessão rastreada ou manual cujo intervalo resultante
passa de 24 h, então o pedido é recusado com `-32602` (`reason = span-too-long`). Um pedido de edição só do tempo jogado de
uma sessão rastreada com intervalo maior que 24 h (gravada antes desta regra) é aceito.

### SED-019 — Tempo jogado entre 30 s e o intervalo
Dado um pedido de criação ou edição cujo tempo jogado resultante é menor que 30 s (DOM-008), então ele é recusado com `-32602`
(`reason = active-too-short`); maior que o intervalo resultante, então `reason = active-exceeds-span`. Tempo igual ao intervalo
é aceito. Exemplo: sessão com 2 h jogadas e pedido que só muda o fim para 1 h depois do início → `active-exceeds-span`.

### SED-020 — Sem sobreposição com sessões rastreadas ou manuais
Dado um pedido de criação, ou de edição que traz início ou fim, de uma sessão rastreada ou manual, quando o intervalo resultante
`[início, fim)` cruza o de outra sessão contada de origem `tracked` ou `manual` de qualquer jogo, ou o da sessão aberta
(`[started_at, T)`), então o pedido é recusado com `-32018` e `data` identificando a sessão em conflito (`sessionId`, `gameId`,
`title`, `startedAt`, `endedAt` ou nulo), e nada muda. Encostar não é cruzar (fim de uma igual ao início da outra é aceito). A
própria sessão editada, as sessões importadas e as descartadas não entram na conta.

### SED-021 — Sessão importada não tem limite de intervalo nem checagem de sobreposição
Dada uma sessão de origem `import` (que resume todo o tempo do LaunchBox num só intervalo, IMP-007), quando ela é editada, então
SED-018 e SED-020 não se aplicam a ela; SED-015, SED-016, SED-017 e SED-019 se aplicam.

### SED-022 — Sessão aberta não é editada nem excluída
Dada uma sessão aberta (`running`, `paused` ou `grace`), quando chega um pedido de edição ou exclusão dela, então o pedido é
recusado com `-32002` (`data` no formato `SessionRef` da sessão), nada muda e o tracker não é chamado.

### SED-023 — Sessão descartada não é editada
Dada uma sessão descartada, quando chega um pedido de edição dela, então o pedido é recusado com `-32602`
(`reason = discarded`) e nada muda. A exclusão segue SED-011.

### SED-024 — Sessão inexistente
Dado um `sessionId` que não existe, quando chega um pedido de edição ou exclusão, então o pedido é recusado com `-32017`
(`data = {sessionId}`). Dado `gameId` inexistente em `session.create`, então `-32001`.

### SED-025 — Jogo rodando não bloqueia as outras sessões
Dado o jogo `G` com uma sessão aberta desde `S0` e uma sessão contada anterior de 1 h, quando essa sessão anterior é editada para
2 h (sem cruzar `[S0, T)`) e depois a sessão aberta termina com 30 min ativos, então o total de `G` aumentou 1 h 30 no fim, a
sessão aberta seguiu sem mudança de estado, de tempo ativo nem de pausas, e `last_played_at` é o fim da sessão que terminou.

### SED-026 — Sem notificação por escrita de sessão
Dado dois clientes conectados, quando um deles chama `session.create`, `session.update` ou `session.delete`, então nenhum
`session.changed` nem outra notificação é enviado por causa disso (extensão de IPC-050). `session.changed` continua só nas
transições do tracker.

### SED-027 — Estatísticas contam e marcam as sessões manuais
Dada uma sessão de cada origem no período, quando calculo as estatísticas, então as três entram no tempo por jogo, por
plataforma, por semana e no total; e o total por jogo, o por plataforma e o geral de `stats.summary` trazem, além de
`importedSeconds`, `manualSeconds` com a parte de origem `manual` (as semanas de `stats.weekly` não ganham campo). A sessão
rastreada editada conta como rastreada.

### SED-028 — Status do jogo não muda
Dado o jogo `G` com qualquer status (inclusive `not_played`), quando uma sessão dele é criada, editada ou excluída, então
`status` e `completed_at` não mudam (o status é sempre escolha do usuário, DOM-012).

### SED-029 — Registro do ajuste no log
Dado uma criação, edição ou exclusão gravada, então o agente registra `Info` "sessão criada", "sessão editada" ou "sessão
excluída" com `session_id`, `game_id`, `source` e os valores antigos e novos de início, fim e tempo jogado (os que existem),
e o total do jogo antes e depois. Pedido recusado não registra nada acima de `Debug`.

## IPC e desktop (SED)

### SED-030 — Contrato v0.17.0 é a fonte da verdade
Dado `docs/specs/phase-16-edicao-sessoes/contracts/ipc.openrpc.json` (gerado por uma tarefa a partir da v0.16.0 e do
[delta](contracts/ipc-v0.17.0-delta.openrpc.json)), quando o teste de contrato roda, então a versão é `0.17.0`, os métodos
novos existem com os mesmos nomes, os campos obrigatórios batem com os tipos Go, o enum `source` bate com o `domain`, os
códigos `-32017` e `-32018` batem com as constantes e o enum `reason` bate com as constantes do agente. Nada que existia muda,
exceto os campos acrescentados a resultados (SED-033, SED-034).

### SED-031 — `session.create`
Dado um `gameId` existente e `startedAt`, `endedAt` (RFC 3339), `activeSeconds` e `includedInTotal` (opcional, padrão `false`)
válidos, quando chamo `session.create`, então recebo `{session: SessionItem, game: GameDetail}` já com a sessão nova e os
valores do jogo depois da escrita. Params por nome; membro desconhecido, tipo errado ou instante que não é RFC 3339 → `-32602`
(IPC-028).

### SED-032 — `session.update` e `session.delete`
Dado um `sessionId` existente e um patch com pelo menos um de `startedAt`, `endedAt` e `activeSeconds` (ausente não mexe;
`null` é `-32602`), quando chamo `session.update`, então recebo `{session, game}` depois da escrita; patch vazio → `-32602`
(`reason = empty-patch`). Quando chamo `session.delete` com `{sessionId}`, então recebo `{sessionId, game}` depois da escrita.

### SED-033 — Item da lista com origem e marca de edição
Dado `session.list`, então cada `SessionItem` traz também `source` (`tracked`, `import` ou `manual`) e `editedAt` (RFC 3339 ou
`null`). A ordem e a paginação de STO-027 e IPC-047 não mudam.

### SED-034 — Tempo das sessões no detalhe do jogo
Dado `game.get` (e todo resultado que traz `GameDetail`), então ele traz também `sessionPlaySeconds`: a soma de `active_seconds`
das sessões contadas do jogo. `totalPlaySeconds − sessionPlaySeconds` é o ajuste manual que a UI usa em SEL-009.

### SED-035 — Bindings do desktop
Quando a UI chama `CreateSession(gameId, input)`, `UpdateSession(sessionId, patch)` ou `DeleteSession(sessionId)`, então o
desktop chama `session.create`, `session.update` ou `session.delete` (prazo de 5 s), devolve a sessão e o detalhe convertidos
para o formato da UI (com `source`, `editedAt` e `sessionPlaySeconds`) sem calcular tempo, e não grava nada nem emite evento
por conta própria (DSK-017).

### SED-036 — Mensagens em português dos erros novos
Dado um erro dos métodos novos, então a mensagem ao usuário (DSK-011) é: `-32017` "Esta sessão não existe mais."; `-32018`
"O horário cruza outra sessão de «título», de «início» a «fim»." (instantes em data e hora locais; "agora" quando a outra está
aberta); `-32002` "Não é possível alterar uma sessão em andamento."; `-32602` com `reason` conforme a tabela do
[contrato](contracts/ipc-v0.17.0-delta.md#4-erros); `-32001` como hoje.

### SED-037 — Estatísticas somam só sessões contadas
Lacuna encontrada ao escrever esta spec (pergunta 7), incluída nesta fase.
Dado um período com uma sessão contada de 1 h, uma descartada de 20 s e uma aberta com 10 min gravados, quando calculo as
estatísticas, então o tempo é 1 h e a contagem de sessões é 1. Hoje `StatsByGame`, `StatsByPlatform` e `SessionTimesSince`
somam qualquer `state`, contra UI-038 ("descartada nunca soma em lugar nenhum").

---

## Interface (SEL)

### SEL-001 — Botão "Adicionar sessão"
Dada a seção "Sessões" do painel de detalhes de um jogo, então ela tem o botão "Adicionar sessão", que abre o formulário de
criação (SEL-005). O botão aparece também com a lista vazia ("Nenhuma sessão registrada.") e com o jogo rodando.

### SEL-002 — Ações por linha
Dada a lista de sessões, então uma sessão contada mostra "Editar" e "Excluir"; uma descartada mostra só "Excluir"; a aberta
(e o `SessionPanel` acima da lista) não mostra nenhum dos dois. Os botões são de texto ou ícone com nome acessível que inclui o
início da sessão (ex.: `aria-label="Editar sessão de 12/11/2026 20:00"`).

### SEL-003 — Marca de origem
Dada uma linha da lista, então a sessão de origem `manual` mostra "Manual", a de `import` mostra "Importada", e a que tem
`editedAt` mostra "Editada" (as duas marcas juntas quando for importada e editada). A sessão rastreada não editada não mostra
marca. Origem desconhecida não mostra marca.

### SEL-004 — A lista não calcula tempo
Dada a lista depois de qualquer criação, edição ou exclusão, então os valores mostrados (linha, tempo total, primeira e última
vez) são os devolvidos pelo agente; a UI não soma nem subtrai tempo (UI-038).

### SEL-005 — Formulário e valores iniciais
Dado o formulário (modal `compact`, título "Adicionar sessão" ou "Editar sessão"), então ele tem "Início" e "Fim" (data e hora
locais, precisão de minuto) e "Tempo jogado" (horas 0..24 e minutos 0..59). Na criação, Fim = agora (relógio da UI, injetável)
arredondado para baixo ao minuto, Início = Fim − 1 h e Tempo jogado = 1 h 00. Na edição, os campos vêm da sessão (o tempo
jogado mostrado é arredondado para baixo ao minuto).

### SEL-006 — Tempo jogado acompanha o intervalo
> **Alterado em 2026-11-26 (criação):** o formulário de criação não pede o Fim: mostra apenas Início e Tempo jogado e exibe "Fim (calculado)" = Início + Tempo jogado, somente leitura. Mudar o Início não altera o Tempo jogado. As validações de Fim (futuro, mais de 24 h) valem sobre o Fim calculado. A edição segue como abaixo.

Dado o formulário com Tempo jogado igual a Fim − Início, quando o usuário muda Início ou Fim, então Tempo jogado passa a ser o
novo Fim − Início. Dado que o usuário mudou Tempo jogado para outro valor, então mudar Início ou Fim não o altera mais.

### SEL-007 — Validação antes do envio
Dado o formulário, quando Fim não é posterior a Início, Fim está no futuro, o intervalo passa de 24 h (sessão rastreada ou
manual), Tempo jogado é menor que 1 min, maior que o intervalo ou tem minutos fora de 0..59, ou um campo está vazio ou não é
número, então o campo mostra a mensagem do erro, "Salvar" fica desativado e nada é enviado. As mensagens são as de SED-036
para o `reason` correspondente.

### SEL-008 — Envio só do que mudou
Dado o formulário de edição, quando o usuário salva, então a UI chama `UpdateSession` só com os campos alterados; um campo que
o usuário não tocou não é enviado (o tempo jogado com segundos de uma sessão rastreada fica intacto). Na criação, todos os
campos são enviados, com os instantes convertidos de hora local para UTC.

### SEL-009 — "Já está no tempo total"
Dado o formulário de criação de um jogo com `totalPlaySeconds − sessionPlaySeconds > 0`, então aparece, desmarcada, a caixa
"Já está no tempo total", com a ajuda "O tempo total deste jogo tem X a mais que as sessões. Marque se esta sessão já faz parte
desse tempo." Marcada, o envio leva `includedInTotal: true` e o tempo jogado acima do ajuste é erro de SEL-007. Sem ajuste
positivo, a caixa não aparece. Na edição, nunca aparece.

### SEL-010 — Erro do agente no formulário
Dado um "Salvar" que o agente recusa, então o modal continua aberto com os valores digitados e mostra, em `role="alert"`, a
mensagem de SED-036; `-32017` fecha o modal e recarrega a lista (a sessão sumiu).

### SEL-011 — Envio em andamento
Dado um "Salvar" ou "Excluir" enviado, então o botão fica desativado e mostra "Salvando…" ou "Excluindo…" até a resposta; um
segundo clique não chama o agente de novo.

### SEL-012 — Exclusão com confirmação
Dada uma linha, quando clico "Excluir", então abre a confirmação "Excluir a sessão de «início» («tempo jogado»)? O tempo dela
sai do tempo total do jogo. Esta ação não pode ser desfeita." (para a descartada: "Excluir a sessão descartada de «início»?
Esta ação não pode ser desfeita."), com "Excluir" e "Cancelar" e o foco inicial em "Cancelar". "Cancelar", Esc ou fechar não
chamam o agente. "Excluir" chama `DeleteSession` uma vez.

### SEL-013 — Atualização depois de escrita
Dado um `CreateSession`, `UpdateSession` ou `DeleteSession` concluído, então a lista de sessões do jogo, o detalhe do jogo, as
listas de jogos (ordem por última vez e por tempo jogado), os jogos recentes e as estatísticas são recarregados uma vez (UI-042).
O detalhe pode ser preenchido direto com o `game` da resposta. Sem polling.

### SEL-014 — Marca de manuais nas estatísticas
Dada a tela de estatísticas com `manualSeconds > 0` num total ou numa linha de jogo, então aparece "inclui X adicionados à
mão" ao lado da marca de importados (EST-005), no mesmo formato.

### SEL-015 — Acessibilidade do formulário
Dado o formulário, então cada campo tem rótulo associado; o campo com erro tem `aria-invalid="true"` e `aria-describedby`
apontando para a mensagem; o foco começa em "Início"; Enter num campo salva quando "Salvar" está ativo; Esc cancela; ao fechar,
o foco volta ao botão que abriu o modal (ou a "Adicionar sessão", quando a linha não existe mais).

### SEL-016 — Hora local e horário de verão
Dado um fuso com horário de verão, quando o usuário digita uma hora local que existe duas vezes ou não existe no dia da troca,
então a conversão para UTC é a do `Date` do navegador para aquela hora local, e o formulário mostra de volta o valor convertido
antes do envio. Testado com fuso fixo.

### SEL-017 — Ponta a ponta
· verificação: manual
Dado o Arcadia com o agente rodando e um jogo com sessões, quando crio uma sessão manual de ontem, edito o fim de uma rastreada
e excluo uma descartada, então o painel mostra as marcas, o tempo total, a primeira e a última vez corretos, as estatísticas
mostram o tempo manual marcado, e lançar o jogo depois disso rastreia normalmente.

---

## Fora de escopo

- Nota ou comentário por sessão (pergunta 1).
- Desfazer, lixeira ou histórico de versões de uma sessão (pergunta 6); o "log de ajuste" da ARCHITECTURE §4.6 é a linha de
  log de SED-029.
- Editar pausas individualmente, dividir ou juntar sessões, mover uma sessão para outro jogo.
- Edição em lote e importação de sessões de arquivo.
- Edição de sessões no Big Box (ele só navega, mostra e lança, fase 13).
- Conquistas e metas (fora desde a fase 8). Registro para quando vierem: conquistas por tempo devem poder ignorar as origens
  `manual` e `import` (a marca de SED-002 existe para isso).
- Mudar o status do jogo automaticamente.

---

## Decisões confirmadas (2026-11-25)

O usuário aprovou os requisitos com as respostas padrão das nove perguntas que estavam em aberto:

1. **Nota por sessão:** não nesta fase (fora de escopo).
2. **Limite do intervalo:** 24 h (SED-018).
3. **Contagem dupla com o ajuste manual de EDM-005:** a caixa "Já está no tempo total" fica na criação (SED-005, SEL-009).
4. **Sobreposição:** recusada contra as sessões rastreadas e manuais de todos os jogos, inclusive a aberta (SED-020).
5. **Sessão descartada:** só pode ser excluída (SED-023).
6. **Exclusão:** irreversível, com confirmação e sem desfazer (SEL-012).
7. **Estatísticas só com sessões contadas:** SED-037 está incluído nesta fase.
8. **Textos das marcas:** "Manual", "Importada", "Editada" e "adicionados à mão" confirmados (SEL-003, SEL-014).
9. **Versão:** esta fase sai como 1.2.0.
