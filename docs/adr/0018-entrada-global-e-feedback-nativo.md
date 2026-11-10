# 0018 — Entrada global e feedback nativos no agente

- **Status:** aceito
- **Data:** 2026-11-10

## Contexto
A pausa precisa funcionar com o jogo em primeiro plano, sem a UI aberta. O agente não usa Wails, então atalho,
controle e aviso têm de ser nativos. Spec: [docs/specs/phase-4-pause](../specs/phase-4-pause/requirements.md).

## Decisão
- Atalho global fixo **Ctrl+Alt+P** por `RegisterHotKey`; controle por polling **XInput**, combo **Select (Back) + Y**
  segurado por 1 s, um disparo por pressão. Valores são constantes nesta fase (sem tela de configurações).
- Aviso "Tempo pausado/retomado" como janela Win32 nativa, sempre no topo, sem ativar e sem clique, por 2,5 s, mais um
  som curto (`Beep`, dois pares de tons). Sem dependência nova.
- Cada componente (atalho, poller, energia, aviso, som, coordenador de feedback) tem uma goroutine dona, com a mesma
  regra do CLAUDE.md (condição de saída, espera no shutdown). O `wndProc` das janelas escondidas é compartilhado por um
  mapa `hwnd → handler`.
- O feedback nunca atrasa o tracker; uma falha do aviso ou do som não impede a pausa.

## Alternativas consideradas
- **Overlay em Wails/WebView:** exigiria a UI ou um WebView residente, contra o agente minúsculo (ADR 0003).
- **Hook de teclado de baixo nível:** mais invasivo e mais frágil que `RegisterHotKey`.
- **Só som:** sem sinal visual em jogos em janela.

## Consequências
- ✅ Funciona sem a UI e com o jogo em foco. Em tela cheia exclusiva o aviso pode não aparecer; o som garante o feedback.
- ⚠️ Atalhos fixos até existir uma tela de configurações. Se o registro do atalho falhar, só o log avisa (o botão da UI cobre).
