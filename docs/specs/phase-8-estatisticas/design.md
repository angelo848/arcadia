# Fase 8 — Estatísticas e modo Big Box: design

Base: [requirements.md](requirements.md). Contrato IPC: nova versão 0.10.0 (a escrever na T01).

## 1. Estatísticas (EST)

- **Leitura no agente:** dois métodos de leitura novos, `stats.summary(period)` (por jogo, plataforma e total) e
  `stats.weekly(weeks)` (semanas ISO). Ambos só leem `play_session` e `game`.
- **Período:** `7d | 30d | 365d | all`, convertido em instantes UTC pelo `domain.Clock` e pelo fuso local do agente.
- **Semanas:** agregadas em Go (segunda-feira, horário local), 12 semanas, com zeros preenchidos.
- **Origem:** a coluna `origin` das sessões (`import` ou rastreada) entra nos totais; a resposta traz o tempo
  importado à parte para a marcação (EST-005).
- **Sem escrita:** nenhum método novo grava. A tela recarrega com `session.changed` (EST-006).
- **Gráficos:** SVG simples (barras), componente próprio em `frontend/src/components/StatsChart.tsx`.

## 2. Big Box (BIG)

- **Tela:** modo do `App` (`bigBox` em estado), que troca a barra superior por um cabeçalho mínimo e aumenta a fonte
  da grade. Entrada por botão e pelo atalho `F11`; saída com B ou Esc (BIG-001).
- **Foco:** `useGamepadNav` no frontend, lê `navigator.getGamepads()` em `requestAnimationFrame` e converte o direcional
  e os botões A, B, Y em ações (BIG-002). O teclado usa as mesmas ações (setas, Enter, Esc).
- **Pausa:** o atalho da fase 4 continua no agente; o Big Box só marca o jogo (BIG-004).
- **Sem agente novo:** nenhum leitor de controle no Go (decisão do usuário).

## 3. Testes

- `internal/store`: agregação por período, por plataforma, importado e rastreado (`STA/...`).
- `internal/agent`: handlers `stats.*` com relógio fake e contrato 0.10.0.
- `internal/ipc`: teste de contrato (chaves dos tipos novos).
- Frontend (Vitest): `StatsChart`, tela de estatísticas, `useGamepadNav` com `getGamepads` simulado, teclas de foco.
- Manual: Big Box com controle real na TV (BIG-002, BIG-004).
