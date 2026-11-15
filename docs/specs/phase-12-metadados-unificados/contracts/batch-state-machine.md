# Máquina de estados da busca em lote — fase 12

- **Requisitos:** MET-052..MET-059, MET-064..MET-067 ([requirements.md](../requirements.md)).
- **Natureza:** tabela de transições (estado × evento → estado + efeito) que o test-writer usa como fonte. Se uma linha daqui e o
  requisito divergirem, vale o requisito e a divergência é bug desta spec.
- **Onde vira código:** `internal/agent/metadata_batch.go` (o job) e `metadata_notify.go` (notificações). Há no máximo **uma**
  busca por agente, só em memória (MET-058).

## 1. Estados

| Estado | Significado | Goroutines vivas |
|---|---|---|
| (sem busca) | nada em memória; `metadata.batchStatus` devolve `null` | nenhuma |
| `searching` | os trabalhadores buscam os jogos | coordenador + até 3 trabalhadores (+ as de provedor de cada jogo, curtas) |
| `ready` | todos os jogos têm linha; esperando confirmação | nenhuma |
| `cancelled` | busca cancelada; as linhas dos jogos já buscados continuam disponíveis | nenhuma depois que os trabalhadores terminam |
| `applying` | um trabalhador aplica as escolhas | coordenador + 1 trabalhador |
| `done` | aplicação terminada (ou cancelada); traz o `result` | nenhuma |

`done` e (sem busca) são os únicos estados em que `metadata.batchStart` cria uma busca nova (em `done` a antiga é descartada).

## 2. Transições

"→ X" = novo estado. "notif." = notificação `metadata.batchChanged` (MET-054). Efeitos de chamada da UI não geram notificação
(IPC-050); só as transições feitas em segundo plano e o progresso geram.

| Estado | Evento | Novo estado | Efeito |
|---|---|---|---|
| (sem busca) | `batchStart` válido | `searching` | cria a busca com `batchId` = horário em ms (crescente), `total` = ids; inicia os trabalhadores; devolve `batchId`, `total` |
| (sem busca) | `batchStatus` | (sem busca) | devolve `batch: null` |
| (sem busca) | qualquer outro método de lote | (sem busca) | `-32014` BatchNotFound |
| `searching` | trabalhador termina um jogo | `searching` | `done`+1 (ou `failed`+1 se todos os provedores falharam); notif. de contador (agrupada em 500 ms) |
| `searching` | todos os jogos têm linha | `ready` | notif. imediata (estado mudou) |
| `searching` | `batchCancel` | `cancelled` | contexto do job cancelado; nenhuma requisição nova; as em andamento terminam e **não** gravam linha; devolve o estado |
| `searching` | `batchStart` | `searching` | `-32015` BatchBusy |
| `searching` | `batchResults`, `batchApply`, `batchDiscard` | `searching` | `-32016` BatchWrongState (`data.state`) |
| `searching` | `batchStatus` | `searching` | devolve o estado com contadores |
| `searching` | desligamento do agente | (sem busca) | contexto cancelado; o agente espera os trabalhadores (WaitGroup) |
| `ready` | `batchResults` | `ready` | devolve uma página (MET-057) |
| `ready` | `batchApply` válido | `applying` | `total` = nº de escolhas, `done` = 0, `failed` = 0; inicia o trabalhador; devolve `accepted` |
| `ready` | `batchApply` inválido (id fora da busca, índice fora da lista, > 1.000) | `ready` | `-32602` |
| `ready` | `batchCancel` | `ready` | no-op; devolve o estado |
| `ready` | `batchDiscard` | (sem busca) | libera as linhas; devolve `discarded: true` |
| `ready` | `batchStart` | `ready` | `-32015` BatchBusy (revisar ou descartar antes) |
| `cancelled` | `batchResults` | `cancelled` | devolve uma página das linhas que existem |
| `cancelled` | `batchApply` válido | `applying` | como em `ready`, só com jogos que têm linha |
| `cancelled` | `batchCancel` | `cancelled` | no-op |
| `cancelled` | `batchDiscard` | (sem busca) | libera; `discarded: true` |
| `cancelled` | `batchStart` | `cancelled` | `-32015` BatchBusy |
| `applying` | trabalhador termina uma escolha | `applying` | `done`+1 (`failed`+1 e título guardado, até 10, se falhou); notif. de contador agrupada |
| `applying` | todas as escolhas processadas | `done` | monta o `result`; notif. imediata |
| `applying` | `batchCancel` | `done` | nenhum jogo novo começa; o em curso termina; `result` parcial; devolve o estado |
| `applying` | `batchStart`, `batchApply` | `applying` | `-32015` / `-32016` |
| `applying` | `batchResults`, `batchDiscard` | `applying` | `-32016` |
| `applying` | desligamento do agente | (sem busca) | contexto cancelado; o agente espera o trabalhador; o que foi gravado fica |
| `done` | `batchStatus` | `done` | devolve o estado com o `result` |
| `done` | `batchDiscard` | (sem busca) | `discarded: true` |
| `done` | `batchStart` válido | `searching` | descarta a antiga e cria a nova |
| `done` | `batchResults`, `batchApply`, `batchCancel` | `done` | `-32016` (`batchCancel` devolve o estado, no-op) |

- **Ordem das checagens** (MET-071, alterado em 2026-11-15): em `batchResults` e `batchApply`, primeiro os parâmetros
  (`limit` ausente = 100, `0` explícito = `-32602`; `choices`), depois `batchId` e o estado. Por isso as linhas "`-32016`" da
  tabela valem só com parâmetros válidos.
- **Falha temporária de provedor durante `searching`** (MET-011): 1ª falha seguida suspende o provedor 30 s, 2ª 60 s, 3ª o
  desativa até o fim do lote; sem degrau de 120 s. Não muda o estado do job.
- **`applying` → `batchCancel`** (MET-066): o `result` parcial inclui todo jogo já gravado, inclusive o que estava em curso.
- **`batchId` desconhecido** (diferente do atual) em qualquer método com `batchId`: `-32014` BatchNotFound, em qualquer estado.
- **`batchStatus` não tem `batchId`** e nunca falha por estado.
- **Reiniciar o agente** leva a (sem busca) (MET-058); a UI percebe quando `batchStatus` devolve `null` e a tinha em memória
  (MET-067).

## 3. Notificação

`metadata.batchChanged {batchId, state, done, total, failed}`.

| Causa | Quando | Agrupamento |
|---|---|---|
| mudança de `state` feita em segundo plano (`searching → ready`, `applying → done`) | na hora | nunca agrupada; reabre a janela de 500 ms |
| mudança só de `done`/`failed` | primeira na hora; as seguintes numa só, no fim da janela de 500 ms | como `media.changed` (IMG-003), medida por `domain.Clock`, sem goroutine nova (o coordenador do job usa o relógio) |
| chamadas da UI (`batchStart`, `batchApply`, `batchCancel`, `batchDiscard`) | nunca | — |

Sem pendência, o coordenador não mantém temporizador no relógio. Notificação pendente quando o job termina por cancelamento ou
desligamento é descartada (a UI relê com `batchStatus`, MET-055).

## 4. Invariantes

1. Nunca há mais de um job; `batchStart` só cria em (sem busca) ou `done`.
2. Nenhum estado grava no banco: só `applying` escreve (metadados e fila de mídia), jogo a jogo, cada escrita é uma transação do
   store.
3. `searching` e `applying` nunca coexistem.
4. Depois de um cancelamento o estado muda na hora e as linhas ficam congeladas: um trabalhador que ainda tinha requisição em
   andamento termina sozinho (seu contexto já foi cancelado) e **não grava** nada (a escrita confere o estado sob o mesmo
   mutex). `batchApply` e `batchDiscard` não esperam por ele; o desligamento do agente espera por todos (WaitGroup) e nenhuma
   goroutine sobrevive ao contexto do agente.
5. Nenhum evento toca sessão, tracker ou `play_session` (MET-080).
