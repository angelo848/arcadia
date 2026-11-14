# 0029 — Tema por variáveis CSS

- **Status:** aceito (aprovado com a fase 10, [phase-10-nova-interface](../specs/phase-10-nova-interface/requirements.md))
- **Data:** 2026-11-14

## Contexto
A nova interface adota um tema escuro próximo do LaunchBox. Cores espalhadas pelos componentes tornariam um segundo
tema (ou ajustes de contraste) uma mudança em todo o frontend.

## Decisão
- Toda cor vem de tokens `--arc-*` definidos sob `[data-theme="dark"]` e expostos ao Tailwind por `@theme inline`
  (UIX-006).
- Componentes não usam classes de paleta do Tailwind nem cores literais; um teste varre os componentes e falha se
  encontrar (UIX-006). Outro teste confere o contraste mínimo WCAG dos pares de tokens (UIX-007).
- Só o tema escuro existe agora; um segundo tema é novo bloco de tokens, sem tocar componentes.

## Alternativas consideradas
- **Classes `dark:` do Tailwind por componente:** espalha a decisão de cor e não permite checagem central.

## Consequências
- ✅ Trocar ou ajustar o tema é editar um arquivo.
- ⚠️ Os componentes existentes precisam ser migrados para os tokens na fase 10.
