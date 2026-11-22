# 0039 — Tempo para zerar do HowLongToBeat com cache e falha silenciosa

- **Status:** aceito (aprovado com a fase 15, [phase-15-big-box-v2](../specs/phase-15-big-box-v2/requirements.md))
- **Data:** 2026-11-22

## Contexto
O usuário quer ver, junto de cada jogo, quanto tempo leva para zerar (história principal, história + extras, 100%). A fonte
natural é o HowLongToBeat, que **não tem API oficial**: o endpoint e o formato mudam sem aviso e o uso automatizado pode contrariar
os termos do site. O IGDB (já usado nos metadados, [ADR 0033](0033-busca-unificada-de-metadados-com-agregador-e-confirmacao-em-lote.md))
tem `game_time_to_beats`, com menos envios de usuários.

## Decisão
- O **agente** consulta o HowLongToBeat (serviço `timeToBeat`); a UI só lê pelo IPC (ADR [0015](0015-agente-unico-escritor-do-banco.md))
  e recebe `timeToBeat.changed` ([ADR 0031](0031-avisos-de-mudanca-em-segundo-plano-como-notificacoes-de-invalidacao.md)).
- O cliente usa o caminho fixo `POST /api/search/site` com um token obtido em `GET /api/search/site/init`, guardado **só em
  memória** e renovado uma vez no 403. Nenhuma credencial, URL ou corpo vai para log, erro ou banco.
- **Uma requisição por vez** (fila única, pedido interativo à frente do lote de até 1.000), **2 s** entre requisições e **recuo** por
  fonte de 1, 5, 30 e 60 minutos nas falhas seguidas.
- **Cache por jogo** na tabela `time_to_beat` (migração 10), inclusive "não encontrado", com validade de 90 dias (HowLongToBeat
  achado) e 30 dias (IGDB, em dúvida e não achado). Título trocado vence a linha, salvo se confirmada pelo usuário.
- A correspondência automática reaproveita o `match` da fase 12; "em dúvida" fica para o usuário confirmar em "Editar jogo"
  (`timeToBeat.search` e `timeToBeat.choose`).
- **Reserva do IGDB:** quando o HowLongToBeat não acha ou falha, o serviço lê `game_time_to_beats` do IGDB (apressado = história
  principal, normal = história + extras, completo = 100%), com recuo próprio, achado válido por 30 dias e a **fonte mostrada**
  ("Fonte: IGDB"). Sem chave do IGDB, a reserva é ignorada em silêncio.
- **Falha silenciosa:** nenhuma falha vira aviso para o usuário fora da ação explícita "Atualizar"; a UI mostra "—". Um único `Warn`
  por sequência de falhas, sem detalhes.
- O serviço pode ser desligado (`timeToBeat.enabled`) e a busca depois de aplicar metadados também (`timeToBeat.afterMetadata`); as
  duas vêm ligadas.

## Alternativas consideradas
- **Só IGDB:** menos cobertura e outra base de tempos. Mantido só como reserva.
- **Consultar pela UI:** quebraria o agente como único ponto de rede com limitadores e recuo compartilhados. Descartada.
- **Raspar a página HTML:** mais frágil que o endpoint de busca. Descartada.
- **Sem cache de "não encontrado":** repetiria buscas inúteis a cada abertura e aumentaria o risco de bloqueio.

## Consequências
- ✅ O tempo para zerar aparece sem custo de rede na navegação (leitura do cache) e sem erros na tela.
- ✅ O recuo e o intervalo de 2 s protegem contra bloqueio; um lote de 1.000 jogos leva cerca de 35 min em segundo plano.
- ⚠️ O endpoint pode mudar a qualquer momento; o efeito é "—" até uma versão nova corrigir as regras ([hltb-rules](../specs/phase-15-big-box-v2/contracts/hltb-rules.md)).
- ⚠️ A correspondência ignora a plataforma: remakes com o mesmo nome ficam "em dúvida" até a confirmação.
- ⚠️ Os tempos do IGDB têm outra base; por isso a fonte aparece e o achado dura menos.
