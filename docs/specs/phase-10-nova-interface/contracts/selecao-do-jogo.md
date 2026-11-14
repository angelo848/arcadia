# Contrato: jogo em foco e estado do painel

Máquina de estados do **jogo em foco** e do **painel da direita** (fase 10). É estado só da UI: nada disto vai ao
agente nem ao banco, e nenhuma transição chama lançamento, pausa ou escrita (UIX-046). Implementada como um reducer puro
em `frontend/src/lib/selection.ts` (design §5), testável sem DOM.

## Estados

| Estado | Jogo em foco | O painel mostra |
|---|---|---|
| `Vazio` | nenhum | o resumo (UIX-024) |
| `Carregando(G)` | G | "Carregando jogo…" (UIX-035) |
| `Mostrando(G)` | G | o detalhe de G (UIX-025..UIX-033) |
| `Erro(G, msg)` | G | a mensagem com "Tentar de novo" (UIX-035) |

Estado inicial: `Vazio` (decisão h: o jogo em foco não é salvo entre execuções).

## Eventos

| Evento | Origem |
|---|---|
| `selecionar(H)` | clique no cartão ou na linha, seta/Home/End na grade, A no Big Box, clique em "Jogados recentemente", escolha nas estatísticas (UIX-017, UIX-019, UIX-020, UIX-024, UIX-044, UIX-045) |
| `lancar(H)` | duplo clique, Enter na grade, Y no Big Box (UIX-018, UIX-019, UIX-044) |
| `leituraOk(G)` | `GetGame(G)` respondeu |
| `leituraInexistente(G)` | `GetGame(G)` respondeu `-32001` |
| `leituraErro(G, msg)` | `GetGame(G)` falhou com outro erro |
| `tentarDeNovo` | botão "Tentar de novo" |
| `filtroMudou` | busca, faceta, plataforma, ordenação ou "Limpar filtros" (UIX-021) |
| `trocaDeTela` | grade ↔ lista, abrir/fechar estatísticas ou modal, entrar/sair do Big Box (UIX-022) |
| `reconectou` | o agente voltou a responder (UI-009, UIX-022) |
| `sessaoTerminou(J)` | `session:changed` com `ended` ou `discarded` do jogo J (UIX-031) |
| `removido(G)` | `DeleteGame(G)` com sucesso (UIX-034) |
| `remocaoRecusada(G)` | `DeleteGame(G)` com erro (ex.: `-32002`, DSK-018) (UIX-034) |

## Transições

Linhas não listadas: o estado não muda e não há efeito.

| Estado | Evento | Novo estado | Efeito |
|---|---|---|---|
| `Vazio` | `selecionar(H)` | `Carregando(H)` | lê `GetGame(H)`, `MediaList(H)`, `GameSessions(H)` (do cache, se houver) |
| `Vazio` | `lancar(H)` | `Carregando(H)` | como `selecionar(H)`; e `LaunchGame(H)` uma vez, se não há lançamento pendente (UIX-018) |
| `Carregando(G)` / `Mostrando(G)` / `Erro(G)` | `selecionar(G)` | igual | nenhum (mesmo jogo) |
| `Carregando(G)` / `Mostrando(G)` / `Erro(G)` | `selecionar(H)`, H ≠ G | `Carregando(H)` | leituras de H; painel volta ao topo (UIX-036); carrossel na primeira imagem (UIX-026) |
| qualquer | `lancar(H)` | como `selecionar(H)` | e `LaunchGame(H)` uma vez, se não há lançamento pendente |
| `Carregando(G)` | `leituraOk(G)` | `Mostrando(G)` | — |
| `Carregando(G)` / `Mostrando(G)` | `leituraInexistente(G)` | `Vazio` | recarrega a grade uma vez (UIX-023) |
| `Carregando(G)` | `leituraErro(G, msg)` | `Erro(G, msg)` | — |
| `Erro(G, msg)` | `tentarDeNovo` | `Carregando(G)` | lê `GetGame(G)` de novo |
| `Carregando(G)` / `Mostrando(G)` / `Erro(G)` | `leituraOk(J)` ou erro de J, J ≠ G | igual | resposta atrasada de outro jogo é ignorada |
| qualquer | `filtroMudou` | igual | nenhum; a grade marca e rola até G só se G estiver no resultado carregado (UIX-021) |
| qualquer | `trocaDeTela` | igual | nenhum (UIX-022) |
| `Mostrando(G)` / `Erro(G)` | `reconectou` | `Mostrando(G)` ou `Carregando(G)` | relê G (UI-009); sem passar por `Vazio` |
| `Erro(G, msg)` | `leituraOk(G)` | `Mostrando(G)` | releitura da reconexão deu certo (acrescentado no plano da fase) |
| `Mostrando(G)` | `leituraErro(G, msg)` | `Mostrando(G)` | releitura (fim de sessão ou reconexão) falhou; os dados antigos continuam na tela (acrescentado no plano da fase) |
| `Mostrando(G)` | `sessaoTerminou(G)` | `Mostrando(G)` | relê `GetGame(G)` e `GameSessions(G)` uma vez (UIX-031) |
| `Mostrando(G)` | `sessaoTerminou(J)`, J ≠ G | `Mostrando(G)` | nada no painel (a grade recarrega como em UI-006) |
| `Mostrando(G)` | `removido(G)` | `Vazio` | grade e catálogo recarregados (UI-042) |
| `Mostrando(G)` | `remocaoRecusada(G)` | `Mostrando(G)` | aviso no topo (DSK-018) |

## Invariantes

1. No máximo um jogo em foco.
2. Nenhuma transição chama `LaunchGameUntracked`, `ToggleSession` nem escrita. Só `lancar` chama `LaunchGame`, e no
   máximo uma vez por evento.
3. `filtroMudou` e `trocaDeTela` nunca levam a `Vazio`.
4. O painel nunca mostra dados de um jogo diferente do jogo em foco (respostas atrasadas são descartadas pelo id).
