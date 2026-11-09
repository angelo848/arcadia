# 0015 — O agente é o único escritor do banco; a UI escreve via IPC

- **Status:** aceito
- **Data:** 2026-11-09

## Contexto
A ARCHITECTURE §2 dividia a escrita: o agente era dono das sessões, a UI da biblioteca. Na fase 3, o fim de uma
sessão atualiza `game.total_play_seconds`, `play_count`, `last_played_at` e `first_played_at` na mesma transação
que fecha `play_session`. Com dois escritores, o agente escreveria numa tabela alheia, ou o total seria gravado em
dois passos por dois processos. Spec: [docs/specs/phase-3-tracker](../specs/phase-3-tracker/requirements.md).

## Decisão
O agente será o único processo que escreve em `library.db`. A UI pede as escritas por IPC (`game.add`,
`game.launch`) e lê a lista da tela por IPC (`game.list`), sem abrir o banco para isso.
A fase 5 pode mover leituras pesadas (grade, filtros, FTS) para leitura direta da UI. Nesse caso a UI só lê e
nunca migra (ADR 0010).

## Alternativas consideradas
- **UI dona da biblioteca (divisão antiga):** dois escritores, `SQLITE_BUSY` entre processos e totais atualizados
  fora da transação da sessão.
- **Agente grava só os totais em `game`:** a dona da tabela deixa de ser clara, e as duas escritas continuam
  disputando a mesma tabela.

## Consequências
- ✅ Totais sempre consistentes com as sessões (uma transação) e nenhuma disputa de escrita entre processos.
- ✅ "A UI nunca influencia o tempo" fica estrutural: a UI não tem caminho de escrita.
- ⚠️ Toda escrita nova da UI (editar e remover jogos, mídia) vira um método IPC no agente.
- ⚠️ Com o agente fora do ar, a UI não consegue adicionar jogos.
