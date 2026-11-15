# Fase 10.2 — Imagens que chegam com a UI aberta: requisitos

- **Status:** rascunho para aprovação do usuário (2026-11-14).
- **Design:** [design.md](design.md) · **Contratos:** [contracts/ipc-v0.12.0-delta.md](contracts/ipc-v0.12.0-delta.md)
  (só o acréscimo sobre a v0.11.0 de [phase-9-plataformas](../phase-9-plataformas/contracts/ipc.openrpc.json); o JSON
  completo `ipc.openrpc.json` v0.12.0 é uma tarefa) · **Tarefas:** `tasks.md` (o planner gera depois da aprovação).
- **Base:** [phase-7-midia](../phase-7-midia/requirements.md) (MID-004, MID-005), [phase-10-nova-interface](../phase-10-nova-interface/requirements.md)
  (UIX-015, UIX-033, UIX-046, UIX-047), [phase-10-1-ajustes](../phase-10-1-ajustes/requirements.md) (AJU-013, AJU-015,
  AJU-028), [phase-3-tracker](../phase-3-tracker/requirements.md) (IPC-027, DSK-010, UI-009) e
  [phase-2-skeleton](../phase-2-skeleton/requirements.md) (IPC-017).
- **ADRs:** 0014 (dois processos), 0015 (o agente é o único escritor do banco e dos arquivos de mídia). Nenhum é
  contrariado. Fica parcialmente superada a decisão f da fase 5 ("não há notificação de mudança da biblioteca"): ver ADR
  proposto no fim.
- **Objetivo:** corrigir o bug relatado pelo usuário: depois de "Buscar metadados" (MetadataDialog → `scrape.apply`), os
  dados textuais do jogo atualizam, mas a capa do cartão na grade e o carrossel do painel só mostram as imagens novas
  quando o usuário navega ou troca o jogo em foco. Toda imagem gravada pela fila de mídia passa a aparecer na hora, sem
  mexer no jogo em foco. **Sem mudança no tracker.**

Formato: IDs `ÁREA-NNN`, estáveis e nunca reutilizados; critérios Dado/Quando/Então. Área nova: **IMG** (imagens ao
vivo). Todo ID ativo precisa de um teste que o cite no nome (`t.Run("IMG-003/...")`, `it("IMG-009 ...")`), exceto os
marcados `· verificação: manual`.

---

## Causa (conferida no código)

1. `scrape.apply` (e `media.queue`) só **enfileira** os downloads (`internal/agent/media_queue.go`, MID-005) e responde
   com `queued`.
2. O MetadataDialog invalida `mediaKey(gameId)` logo depois da resposta (`MetadataDialog.tsx`, linhas 62-64), quando
   nenhuma imagem chegou ainda: a releitura devolve a lista antiga.
3. Quando cada download termina (`mediaQueue.run` → `AddMedia`), nada avisa a UI. A única notificação de dados é
   `session.changed` (IPC-027); a fase 5 decidiu não notificar mudanças da biblioteca (decisão f, IPC-050), porque a UI
   recarregava com as próprias mutações. A fila escreve **depois** da mutação, em segundo plano: a regra não cobre esse
   caso.
4. `mediaKey(id)` é a chave compartilhada por `useMedia` (carrossel do painel) e `useCardImages` (cartões e "Jogados
   recentemente", `staleTime` 60 s). Por isso o cartão só muda quando sai e volta à tela depois de 60 s, e o carrossel só
   quando o jogo volta ao foco.
5. Lacuna da spec: a limitação 2 da fase 10 dizia que as imagens novas apareciam "na próxima leitura (trocar de jogo em
   foco, reconexão ou nova busca)", mas a reconexão (UI-009, `useReloadOnReconnect`) não relê a lista de mídia.

**Bug sem requisito correspondente:** nenhum critério exigia que imagens gravadas em segundo plano chegassem à UI aberta
(UIX-033 só pedia "o carrossel lê a lista de mídia de novo" depois de aplicar). Registrado como lacuna; IMG-001..IMG-011
a fecham.

---

## Requisitos de fases anteriores afetados

Cada um recebeu, na spec de origem, a anotação `· alterado em 2026-11-14 (docs/specs/phase-10-2-imagens-ao-vivo,
IMG-NNN)`. Se esta spec não for aprovada, as anotações são desfeitas.

| Requisito | Fase | Situação | Por |
|---|---|---|---|
| UI-009 — Recarga ao reconectar | 3 | alterado: também relê as listas de mídia já carregadas | IMG-010 |
| UIX-033 — Buscar metadados | 10 | alterado: as imagens baixadas depois de aplicar aparecem quando chegam, não só na releitura imediata | IMG-011 |
| UIX-047 — Sem mudança no agente, no tracker e no IPC | 10 | alterado: na fase 10.2 o IPC ganha `media.changed` (v0.12.0) | IMG-013 |
| AJU-013 — Troca de jogo e imagens que falham | 10.1 | alterado: imagens novas do mesmo jogo não tiram a imagem em destaque | IMG-009 |
| AJU-028 — Onde a fase mexe | 10.1 | alterado: o "IPC continua v0.11.0" vale para o diff da 10.1; a 10.2 acrescenta `media.changed` | IMG-013 |
| Limitação 2 da fase 10 — Imagens que chegam depois | 10 | resolvida | IMG-001..IMG-011 |

Inalterados e ainda válidos: MID-004, MID-005 (a fila continua igual; só passa a avisar), IPC-017 (buffer de 16 com
descarte), IPC-027 e DSK-010 (`session.changed`), **IPC-050** (`game.add`, `game.update`, `game.delete` e `platform.add`
continuam sem notificação: `media.changed` vem da fila, não dessas chamadas), UIX-015, UIX-046, AJU-015, AJU-016.

---

## IMG — Aviso do agente

### IMG-001 — `media.changed` quando a fila grava uma imagem
Dado um ou mais clientes conectados e um job da fila de mídia (MID-005) do jogo G, de qualquer origem (`scrape.apply`,
`media.queue` ou job pendente retomado na subida do agente), quando o download termina e `AddMedia` grava uma imagem nova
(`added = true`), então cada cliente recebe a notificação `media.changed` (sem `id`) com `params = {"gameId": G}`,
respeitando o agrupamento de IMG-003. O aviso é enviado depois da gravação, de modo que um `media.list` de G feito ao
recebê-lo já devolve a imagem.
· verificação: teste do agente com fila, provedor fake (`httptest`), relógio fake e dois clientes IPC reais; teste da
fila com notificador espião (ordem: gravação antes do aviso).

### IMG-002 — Sem aviso quando a lista de mídia não muda
Dado um job da fila de G, quando o download falha (com ou sem nova tentativa, MID-008), é cancelado (`media.cancel`), é
interrompido pelo encerramento do agente ou grava uma imagem já existente pelo hash (`AddMedia` com `added = false`,
MID-004), então nenhum `media.changed` é enviado por esse job. As chamadas `scrape.apply`, `media.queue`, `media.cancel`,
`import.apply`, `game.delete` e as escritas de plataforma não enviam `media.changed` por si (IPC-050 continua valendo).
· verificação: teste da fila com notificador espião (um caso por desfecho); teste do agente que chama os métodos e
confere que nenhum `media.changed` chega antes de um job gravar.

### IMG-003 — Avisos agrupados por jogo
Dado a janela de agrupamento `mediaChangedWindow` = 500 ms, medida pelo `domain.Clock` do agente, então, para cada jogo:
- uma gravação de G sem janela aberta para G envia `media.changed(G)` na hora e abre a janela de G por 500 ms;
- gravações de G com a janela aberta não enviam nada na hora; ao fim da janela é enviado **um único**
  `media.changed(G)`, que abre uma nova janela de 500 ms;
- uma janela que termina sem gravação de G dentro dela não envia nada, e a próxima gravação de G volta a enviar na hora;
- jogos diferentes têm janelas independentes.
Assim, G recebe no máximo um aviso a cada 500 ms e o último estado de G sempre é avisado. Sem aviso pendente, a fila não
mantém nenhum temporizador no relógio. Um aviso pendente quando a fila para (encerramento do agente) é descartado
(IMG-010 cobre a volta).
Exemplo com relógio fake: gravações de G em t = 0, 100 ms e 200 ms → avisos de G em t = 0 e t = 500 ms; nada em
t = 1000 ms; gravação de G em t = 1200 ms → aviso em t = 1200 ms. Gravações de G e H em t = 0 → um aviso de cada em
t = 0.
· verificação: teste de unidade do agrupador (tabela de cenários) e teste da fila com relógio fake (sem `time.Sleep`;
nenhum temporizador pendente com a fila ociosa).

### IMG-004 — Contrato IPC v0.12.0
Dado o contrato IPC, então ele está na versão 0.12.0, com a notificação `media.changed` (`x-notification: true`,
parâmetro por nome `gameId` inteiro ≥ 1, sem resposta), a constante do agente `x-media.mediaChangedWindowMs = 500` e
nenhuma outra mudança sobre a v0.11.0 (nenhum método, parâmetro, resultado ou erro existente muda). `internal/ipc` expõe
`MethodMediaChanged = "media.changed"` e `MediaChangedParams{GameID}` com o nome JSON `gameId`.
· verificação: teste de contrato de `internal/ipc` (método listado, constante, nome do campo, versão, janela igual à
constante do agente).

---

## IMG — Repasse no desktop

### IMG-005 — `media.changed` vira o evento `media:changed`
Dado a UI conectada, quando recebe `media.changed` com `gameId` inteiro ≥ 1, então emite o evento Wails `media:changed`
com `{"gameId": G}`. Dado `media.changed` com params ausentes, JSON inválido, `gameId` ausente, não inteiro ou < 1,
então registra um único `Warn` "media.changed com parâmetros inválidos" e não emite nada. O repasse não chama nenhum
método do agente e não muda o estado do supervisor (reconexão, relançamento, `agent.shutdown`).
· verificação: teste do supervisor (`supervisor_test.go`, mesmo formato dos de DSK-010).

---

## IMG — Atualização na UI

### IMG-006 — O aviso relê só a lista de mídia do jogo
Dado a UI com a lista de mídia de G em cache, quando chega `media:changed` com `gameId = G`, então a consulta
`mediaKey(G)` é invalidada: se está em uso na tela (cartão visível, carrossel do painel, "Jogados recentemente"), é
relida na hora com `MediaList(G)`, ignorando o `staleTime` de 60 s; se está em cache mas fora da tela, fica marcada como
velha e é relida quando voltar à tela. Dado G sem lista de mídia em cache, então nada é lido. Em nenhum caso o aviso
chama outro binding (`ListGames`, `GetGame`, `CurrentSession`, sessões, estatísticas) nem invalida outra consulta. Um
evento com dado inválido (sem `gameId` inteiro ≥ 1) é ignorado.
· verificação: teste de `useMediaEvents` com `EventsOn` fake e espiões nos bindings (em uso, em cache fora da tela, sem
cache, dado inválido).

### IMG-007 — Uma assinatura, cancelada ao desmontar
Dado o `App` montado, então existe exatamente uma assinatura de `media:changed`; quando o `App` desmonta, a assinatura é
cancelada.
· verificação: teste de `useMediaEvents` (contagem de handlers, como em UI-006).

### IMG-008 — O cartão ganha a imagem sem mexer no foco
Dado a grade (ou a lista, ou o Big Box) com o jogo G em foco e o cartão de H visível, quando chega `media:changed(H)` e a
releitura traz uma imagem nova, então o cartão de H passa a seguir AJU-015/AJU-016 com a lista nova: do placeholder para
a capa ou para a imagem substituta, e da substituta para a capa quando a primeira capa chega. O jogo em foco continua G
(painel e marcação de UIX-017), o foco do teclado e a posição de rolagem da grade não mudam, e nenhuma
outra linha da grade é relida. O mesmo vale com H = G (o cartão em foco troca a imagem e continua em foco).
· verificação: teste de componente (`App`) com espiões; cartão sem imagem → capa; substituta → capa; foco e rolagem
inalterados.

### IMG-009 — O carrossel recebe imagens novas sem perder o destaque
Dado G em foco com a imagem de id M em destaque no carrossel (AJU-009), quando a lista de mídia de G é relida com imagens
novas (por IMG-006), então as imagens novas entram na ordem de AJU-009, o indicador "k de n" e a faixa de miniaturas
(AJU-010) refletem a lista nova, e M continua em destaque (k passa a ser a posição de M na lista nova). Se a miniatura de
M tinha o foco do teclado, ela continua com o foco. Dado o carrossel sem imagens (placeholder), quando chega a primeira
imagem, então ela aparece em destaque. Dado M ausente da lista nova, então a primeira imagem fica em destaque. Trocar o
jogo em foco continua levando à primeira imagem (AJU-013).
· verificação: teste de componente de `MediaCarousel` (lista que cresce antes e depois de M; foco na miniatura; lista
vazia que ganha uma imagem; M removida; troca de `resetKey`).

### IMG-010 — Rede de segurança: reler as listas de mídia ao reconectar
Dado que o indicador passa de "Desconectado" para "Conectado" (UI-009), então toda consulta `['media', id]` que já tem
dados é invalidada (as que estão em uso são relidas; as demais ficam velhas). Consultas de mídia ainda sem resposta não
são relidas de novo.
· verificação: teste de `useReloadOnReconnect` (consultas com e sem dados).

### IMG-011 — O cenário do bug: buscar metadados com o jogo em foco
Dado G em foco, sem capa, com o painel aberto e o cartão de G visível, quando o usuário aplica um candidato no
MetadataDialog (`ScrapeApply` responde `queued = 3`) e, depois da resposta, chegam `media:changed(G)` com a lista de mídia
crescendo a cada um, então o carrossel e o cartão de G mostram as imagens novas a cada evento, sem o usuário navegar,
fechar o diálogo nem trocar o foco; G continua em foco. O MetadataDialog continua invalidando `mediaKey(G)` ao aplicar
(idempotente: com nada novo, a releitura não muda a tela).
· verificação: teste de componente (`App` com MetadataDialog aberto e eventos emitidos depois da resposta).

---

## IMG — Invariantes

### IMG-012 — A UI nunca influencia o tempo de jogo
Dado os eventos `media:changed` e a recarga da reconexão, então a UI só chama `MediaList`: nenhum lançamento, nenhum
`ToggleSession`, pausa, retomada ou escrita; o aviso não mexe no jogo em foco nem na sessão em exibição. O agente envia
`media.changed` só a partir da fila de mídia, que não toca no tracker nem nas sessões. As regras de UIX-046 não mudam.
· verificação: o teste de invariantes de UIX-046 (`App.invariants.test.tsx`) ganha eventos `media:changed`; teste do
agente (gravar mídia não cria sessão nem envia `session.changed`).

### IMG-013 — Onde a fase mexe
Dado o diff da fase, então não há mudança em `internal/tracker`, em `cmd/fake-game`, em `internal/store` (nem query, nem
migração, nem `sqlc generate`) nem nos bindings de `cmd/arcadia` (nenhum binding novo); as mudanças em Go ficam em
`internal/ipc` (constante e params de `media.changed`), `internal/agent` (agrupador, aviso da fila, notificador) e
`cmd/arcadia` (repasse do supervisor). O contrato passa a v0.12.0 só com o acréscimo de IMG-004.
· verificação: revisão do diff; teste de contrato do IPC.

### IMG-014 — Ponta a ponta
Dado o Arcadia com um jogo sem capa em foco, quando busco metadados no IGDB (ou SteamGridDB) e aplico, então, sem sair do
jogo, a capa aparece no cartão e as imagens aparecem no carrossel conforme cada download termina; o jogo continua em foco
e o carrossel não pula para a primeira imagem enquanto navego pelas miniaturas.
· verificação: manual.

---

## Fora desta spec

- **Progresso da fila na UI** ("baixando k de n", lista de jobs, erros por imagem): exigiria leitura de jobs no IPC.
- **Aviso para a cópia síncrona do LaunchBox** (`import.apply`): a UI já relê a grade com a resposta da chamada, e os
  jogos gravados são novos (sem lista de mídia em cache); um candidato "já existe" não recebe imagens (AJU-018).
- **Aviso de remoção de mídia e de mudança de ícone de plataforma**: não há remoção de mídia hoje; o ícone vem da
  resposta de `platform.setIcon` (IPC-050).
- **Notificação de biblioteca para várias UIs** (decisão f da fase 5): continua uma UI por agente.
- **Capas em lote no IPC** (limitação 3 da fase 10): cada cartão continua com seu `media.list`.
- **Garantia de entrega**: o canal continua com descarte (IPC-017); ver limitação 1.

## Limitações conhecidas

1. **Aviso descartado com a conexão de pé:** se a fila da conexão no agente (16) ou o canal do cliente (16) estiverem
   cheios, um `media.changed` se perde (IPC-017). Com o agrupamento (no máximo 2 avisos por segundo por jogo, e a fila
   com 2 downloads simultâneos), isso exige um pico improvável. O que se perde volta na próxima leitura da lista: trocar o
   foco, o cartão voltar à tela depois de 60 s, nova busca de metadados ou reconexão (IMG-010).
2. **Carrossel com a mesma imagem por outra posição:** quando entram imagens antes da imagem em destaque, o "k de n"
   muda (a imagem é a mesma, IMG-009).

## Perguntas ao usuário

1. **Imagem em destaque quando chegam imagens novas do jogo em foco.** Hoje (AJU-013) qualquer mudança na lista volta o
   carrossel para a primeira imagem; com a atualização ao vivo, isso faria o carrossel pular para o início a cada
   download enquanto você navega pelas miniaturas. **Recomendação: manter a imagem em destaque (IMG-009, como está
   escrito)**; a capa nova aparece de qualquer jeito no cartão e na primeira miniatura. Alternativa: voltar para a
   primeira imagem a cada chegada (mostra a capa nova no destaque na hora, mas interrompe a navegação).

## ADR proposto

A sessão principal escreve (a spec só o referencia). Próximo número livre: 0031.

1. **0031 — Notificações de mudança em segundo plano como avisos de invalidação:** o agente só notifica a UI de mudanças
   que ela não causou (trabalho em segundo plano, como a fila de mídia); a notificação leva só o identificador do que
   mudou (sem dados), é agrupada por entidade no agente com `domain.Clock` (envio imediato e um aviso final por janela),
   e a UI responde relendo a consulta afetada, com releitura na reconexão como rede de segurança para avisos perdidos.
   Mudanças causadas por uma chamada da UI continuam sem notificação (decisão f da fase 5, IPC-050). Padrão para avisos
   futuros (por exemplo, a busca unificada da fase 12).
