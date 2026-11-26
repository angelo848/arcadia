# 0041 — Sessões editáveis pelo usuário: origem `manual`, total por diferença e sem sobreposição

- **Status:** aceito (aprovado com a fase 16, [phase-16-edicao-sessoes](../specs/phase-16-edicao-sessoes/requirements.md)).
  Faz uma ressalva ao [ADR 0017](0017-modelo-de-pausa.md) (item 4); não o substitui.
- **Data:** 2026-11-25

## Contexto
Até a fase 15 a sessão era um fato gravado só pelo agente: nascia no lançamento, era fechada pelo tracker e o usuário não podia
corrigi-la. Jogos fora do Arcadia, esquecimentos e erros de rastreamento deixavam o histórico errado. A fase 16 permite criar,
editar e excluir sessões de um jogo, sem mexer no tracker e sem influenciar a sessão em andamento. Isso toca o total jogado (que
desde a fase 14 já pode ter um ajuste manual), a primeira e a última vez jogado, a contagem de sessões e as estatísticas.

## Decisão
1. **Quem escreve:** sessões **encerradas** (`ended_at` não nulo) são criadas, editadas e excluídas só pelo agente, por IPC
   (`session.create`, `session.update`, `session.delete`, [ADR 0015](0015-agente-unico-escritor-do-banco.md)), em transação
   `BEGIN IMMEDIATE`. A sessão **aberta** nunca é editada nem excluída; com o jogo rodando, as outras sessões dele continuam editáveis.
2. **Origem e marca:** a sessão criada à mão tem `source = 'manual'`. Editar não muda a origem: a sessão ganha só a marca
   `edited_at` (migração 11). A lista mostra "Manual", "Importada" e "Editada". A marca existe também para que conquistas por tempo,
   quando vierem, possam ignorar `manual` e `import`.
3. **Total por diferença:** criar, editar e excluir mudam `total_play_seconds` pela diferença do tempo jogado e conservam o ajuste
   manual de EDM-005 (fase 14). Primeira e última vez jogado e a contagem são **recalculadas** das sessões contadas (`state = 'ended'`).
   O total é limitado a 0 e ao teto de `ipc.MaxPlaySeconds`.
4. **`active_seconds` é a verdade numa sessão editada:** o tempo jogado gravado vale, e as pausas viram registro auxiliar, cortado ao
   novo intervalo (as que saem são cortadas nas bordas; as que ficam sem duração são apagadas). É uma **ressalva** à consequência do
   ADR 0017 de que o tempo é sempre recalculável a partir da sessão e das pausas: ela continua valendo para as sessões não editadas.
5. **Sem sobreposição:** a linha do tempo de sessões rastreadas e manuais não tem sobreposição entre **todos** os jogos (coerente com
   "um jogo por vez"), inclusive contra a sessão aberta. Encostar (fim igual ao início) é aceito. Importadas e descartadas não entram
   na conta (a importada tem intervalo sintético, IMP-007).

## Alternativas consideradas
- **Recalcular o total como soma das sessões:** apagaria o ajuste manual da fase 14 (EDM-005). Descartada.
- **Origem `edited` no lugar da marca:** perderia a origem original (rastreada ou importada). Descartada.
- **Permitir sobreposição:** as estatísticas por semana somariam o mesmo tempo duas vezes. Descartada.
- **Desfazer ou lixeira para a exclusão:** complexidade sem pedido; a exclusão é irreversível e pede confirmação.

## Consequências
- ✅ O histórico de tempo pode ser corrigido sem tocar no tracker nem na sessão em andamento.
- ✅ O ajuste manual da fase 14 sobrevive a criar, editar e excluir sessões.
- ✅ A origem original da sessão se preserva e a marca "editada" a distingue.
- ⚠️ O total deixa de ser sempre a soma das sessões contadas (STO-016, fase 3): a diferença é o ajuste manual.
- ⚠️ Em sessão editada, o tempo jogado não se recalcula das pausas; quem lê pausas deve tratá-las como registro auxiliar.
- ⚠️ A sobreposição entre jogos é recusada também ao editar; sessões antigas que já se cruzavam não são corrigidas.
