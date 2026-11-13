# Fase 8 — Estatísticas e modo Big Box: requisitos

- **Status:** requisitos com as decisões respondidas pelo usuário: sessões importadas entram marcadas; controle pelo webview; período padrão 7 dias. Design e tarefas aguardam aprovação; nada é implementado antes disso.
- **Base:** [phase-5-library](../phase-5-library/requirements.md) (página do jogo, grade), [phase-4-pause](../phase-4-pause/requirements.md)
  (pausa por controle) e [phase-6-importers](../phase-6-importers/requirements.md) (sessões importadas, IMP-007).
- **ARCHITECTURE:** §9 (fase 8).
- **Objetivo:** mostrar onde o tempo de jogo foi gasto (por jogo, semana e plataforma) e oferecer uma tela cheia navegável
  por controle, para usar o Arcadia na TV.

Formato: IDs `ÁREA-NNN`, critérios Dado/Quando/Então. Áreas novas: **EST** (estatísticas) e **BIG** (Big Box).

---

## Decisões de produto (recomendação indicada)

1. **Sessões importadas:** o tempo do LaunchBox (IMP-007) conta nas estatísticas, marcado como "importado" em cada
   total. Recomendação: sim, com a marcação visível.
2. **Navegação por controle no Big Box:** pela API de gamepad do próprio webview (já recebe o controle quando a janela
   está em foco), sem um segundo leitor de controle no agente. Recomendação: sim.
3. **Gráficos:** barras simples em SVG, sem biblioteca de gráficos. Recomendação: sim.
4. **Período padrão:** últimos 7 dias na tela de estatísticas; "tudo" e 30/365 dias como opções. Recomendação: sim.

---

## Requisitos de estatísticas

### EST-001 — Tempo por jogo
Dado a página de estatísticas, quando abro, então vejo, por jogo, o tempo total no período, o número de sessões e a
última vez jogado. Quando escolho um jogo, então vou para a página do jogo.
Verificação: teste de componente com sessões de fixture; teste de consulta na store com período.

### EST-002 — Tempo por semana
Dado as últimas 12 semanas (ISO, semana começando na segunda, horário local), quando a tela carrega, então vejo um
gráfico de barras com o tempo de cada semana, e a semana atual marcada. Semanas sem jogo aparecem com barra zero.
Verificação: teste da função de agregação com relógio fake (`domain.Clock`) e fuso fixo.

### EST-003 — Tempo por plataforma
Dado o período escolhido, quando a tela carrega, então vejo o tempo de cada plataforma em ordem decrescente, com a
porcentagem do total. Plataformas sem tempo no período não aparecem.
Verificação: teste de agregação na store.

### EST-004 — Período
Dado o seletor de período (7 dias, 30 dias, 365 dias, tudo), quando escolho uma opção, então os três blocos (jogos,
semanas, plataformas) são recalculados juntos. O período padrão é 7 dias.
Verificação: teste de componente.

### EST-005 — Sessões importadas
Dado uma sessão de origem `import` (IMP-007), quando calculo as estatísticas, então ela entra no total, e a
linha ou o total mostram "importado". As sessões rastreadas não mudam por causa da importação.
Verificação: teste na store com uma sessão de cada origem.

### EST-006 — Só leitura e atualização
Dado que a tela de estatísticas só lê dados, quando uma sessão termina, então a tela recarrega com o evento
`session.changed` e não escreve nada no banco. O agente continua o único escritor (ADR 0015).
Verificação: teste de componente com evento simulado; revisão de que não há método de escrita chamado.

---

## Requisitos do Big Box

### BIG-001 — Entrar e sair da tela cheia
Dado a biblioteca, quando escolho "Big Box" (botão ou atalho), então a janela vai para tela cheia com a grade de capas
em fonte grande. Quando aperto B ou Esc na biblioteca, então saio do Big Box, na mesma posição. Dentro de uma página (jogo ou estatísticas), B ou Esc voltam à página anterior (BIG-002).
Verificação: teste de componente; teste manual na TV.

### BIG-002 — Navegação por controle
Dado o Big Box com a janela em foco, quando aperto as setas ou o direcional do controle, então o foco se move pela
grade, com a capa do foco destacada. O foco passa por todos os itens da página (capa, Jogar, Sem contar tempo, botões de volta e ação). A (ou Enter) ativa o item focado; Y (ou Shift+Enter) inicia o jogo do cartão focado. B (ou Esc) volta uma página, e na biblioteca sai do Big Box. Ao trocar de página, o foco vai para o primeiro item dela.
Verificação: teste de componente com eventos de teclado simulados; teste manual com controle real.

### BIG-003 — Sem depender do mouse
Dado o Big Box, quando não há mouse, então todas as ações (buscar, filtrar, abrir jogo, iniciar jogo, voltar) são
alcançáveis pelo controle. A busca de texto abre pelo teclado virtual do Windows.
Verificação: revisão da lista de ações; teste manual.

### BIG-004 — Pausa durante o Big Box
Dado um jogo iniciado pelo Big Box, quando o usuário pausa pelo atalho da fase 4 (`Select + Y`), então a pausa funciona
como fora da tela cheia, e o Big Box mostra "Pausado" no jogo.
Verificação: teste manual; teste de componente da marcação.

---

## Perguntas em aberto (respondidas pelo usuário antes da aprovação)

1. Tempo importado do LaunchBox entra nas estatísticas (EST-005), marcado como importado?
2. Navegação do Big Box pela API de gamepad do webview (BIG-002), em vez de ler o controle no agente?
3. Período padrão de 7 dias e as opções 30 dias, 365 dias e tudo (EST-004)?

---

## Fora de escopo

- Metas, conquistas e comparação entre usuários.
- Gráficos com bibliotecas externas.
- Controle de Big Box por XInput no agente (só se a API do webview não bastar).
