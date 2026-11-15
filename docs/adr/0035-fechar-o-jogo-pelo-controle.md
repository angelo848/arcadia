# 0035 — Fechar o jogo pelo controle (Select + B)

- **Status:** aceito (aprovado com a fase 13, [phase-13-modo-console](../specs/phase-13-modo-console/requirements.md))
- **Data:** 2026-11-15

## Contexto
No uso com controle (modo console) o usuário precisa sair do jogo ou do emulador sem teclado nem mouse. Hoje o controle
só pausa a sessão (Select + Y, [ADR 0017](0017-modelo-de-pausa.md)), e o único jeito de fechar é pela janela do jogo.

## Decisão
- O combo **Select + B**, segurado 1 s (o mesmo tempo da pausa, sem som de confirmação), encerra a sessão em curso.
  Vale em qualquer tela, dentro ou fora do Big Box, e para sessão ativa ou pausada.
- O agente primeiro **fecha educadamente**: `WM_CLOSE` às janelas dos processos vivos do jogo, esperando até 3 s
  (via `domain.Clock`) para o emulador salvar e sair; se ainda houver processos vivos, mata a árvore do Job Object.
  Em jogo adotado ([ADR 0025](0025-rastreio-por-processo-adotado.md)) mata só os PIDs que ainda casam com a regra.
- A sessão fecha com o tempo contado até o instante do gatilho e o motivo novo `closed-by-hotkey`; o tempo pausado não
  conta. A espera de até 3 s não soma tempo.
- Sem processo para fechar (URI ainda sem adoção, ou falha ao matar) o agente toca o som de recusa; no sucesso não há
  som e não há confirmação extra.
- O jogo lançado **sem contar tempo** ([ADR 0019](0019-lancamento-sem-rastrear.md)) passa a nascer num Job Object
  guardado pelo tracker, só para poder ser fechado. Isso substitui em parte o 0019; o resto dele continua valendo.
- Fechar o agente continua sem matar o jogo ([ADR 0005](0005-rastreamento-em-camadas.md)).
- A UI nunca influencia o tempo de jogo; ela só recebe `game.closedByHotkey` para retomar o foco.

## Alternativas consideradas
- **Matar direto sem `WM_CLOSE`:** o emulador perde o salvamento automático.
- **Pedir confirmação:** contraria o uso só com controle; o 1 s segurado é a proteção.
- **Contar o tempo até o fim da espera:** o tempo ficaria maior do que o usuário pediu.

## Consequências
- ✅ O jogo fecha só com o controle e o emulador tem chance de salvar.
- ✅ O tempo registrado é o do instante do pedido.
- ⚠️ O tracker ganha um estado de fechamento (sem nova sessão até terminar) e o jogo sem contar tempo ganha um Job Object.
- ⚠️ O IPC passa a v0.15.0 (só acréscimos); novo motivo de fim `closed-by-hotkey`.
