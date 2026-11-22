# 0038 — Big Box por pedido externo (Select + A e `--bigbox`)

- **Status:** aceito (aprovado com a fase 15, [phase-15-big-box-v2](../specs/phase-15-big-box-v2/requirements.md))
- **Data:** 2026-11-22

## Contexto
O Big Box (ADR [0036](0036-big-box-com-temas-selecionaveis.md)) só abria pela janela do Arcadia (`F11`). Quem usa o PC pela TV, só com
o controle, precisa abri-lo sem tocar no teclado, com a janela fechada ou atrás de outro programa. Quem lê o controle é o agente
(ADR [0018](0018-entrada-global-e-feedback-nativo.md)), que não tem janela; a UI é outro processo (ADR [0014](0014-executavel-unico-dois-processos.md)).

## Decisão
- `internal/input` ganha um terceiro acorde, **Select + A segurado por 1 s** (`BigBoxMask`, padrão Back+A), ao lado dos de pausa e de
  fechar o jogo ([ADR 0035](0035-fechar-o-jogo-pelo-controle.md)). O `triggerRouter` do agente despacha a ação `bigbox`, com
  antirrepique de 300 ms.
- **Sem sessão de jogo aberta**, o agente executa o próprio executável com `--bigbox` (`uiOpener`, processo destacado). **Com jogo
  rodando** (`Tracker.Current()`), o pedido é recusado com o som de recusa e nada abre.
- A UI lê `--bigbox` na primeira instância e, na **segunda instância** (Wails `OnSecondInstanceLaunch`), por varredura própria dos
  argumentos. Isto muda a regra do single-exe de ignorar os argumentos da segunda instância. O pedido fica guardado no `App` e o
  frontend o toma uma vez (`TakeBigBoxRequest`, e evento Wails na instância já aberta). Já dentro do Big Box, o pedido não faz nada.
- O pedido é um fato de entrada: o agente nunca guarda estado do Big Box e a UI continua sem influenciar o tempo de jogo.

## Alternativas consideradas
- **IPC do agente para a UI ("abra o Big Box"):** exigiria a UI já aberta; com a janela fechada não há quem receba. Descartada.
- **Agente criar a janela do Big Box:** duplicaria a UI no agente (que não tem Wails). Descartada.
- **Atalho do Windows ou comando externo no Sunshine/Steam:** obriga configuração manual e não resolve o controle local. Mantida
  como extra (`arcadia.exe --bigbox` funciona de qualquer lugar).
- **Bloquear com "jogo sem contar tempo" também:** o roteador só vê sessões do tracker; mudar isso exigiria mexer em
  `internal/tracker/`. Fica fora (Limitações da spec).

## Consequências
- ✅ O Big Box abre pelo controle em qualquer situação sem jogo; o tracker não muda.
- ✅ `--bigbox` serve também a atalhos e ao Moonlight/Sunshine.
- ⚠️ Depende do agente na bandeja: desde o [ADR 0024](0024-agente-sempre-como-administrador.md) ele não sobe com o Windows.
- ⚠️ O Windows pode negar o primeiro plano a uma janela aberta pelo agente; a UI força (`ForceForeground`) e, se negar, a janela fica
  no topo e o botão pisca na barra de tarefas.
- ⚠️ Um jogo iniciado sem contar tempo não bloqueia o pedido.
