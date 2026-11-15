# 0031 — Avisos de mudança em segundo plano como notificações de invalidação

- **Status:** aceito (aprovado com a fase 10.2, [phase-10-2-imagens-ao-vivo](../specs/phase-10-2-imagens-ao-vivo/requirements.md))
- **Data:** 2026-11-14

## Contexto
Trabalho em segundo plano no agente (hoje, os downloads da fila de mídia) muda dados que a UI já carregou. A UI só
via a mudança quando uma consulta era refeita por outro motivo (trocar de jogo, por exemplo). O canal agente→UI existia
só para sessões (`session.changed`), e as chamadas feitas pela própria UI não geram notificação (IPC-050).

## Decisão
- Mudanças causadas por trabalho em segundo plano do agente são avisadas por notificações IPC que carregam **só o id**
  da entidade (`media.changed {gameId}`), sem os dados.
- O agente agrupa os avisos por entidade numa janela (500 ms, via `domain.Clock`, sem goroutine nova): o primeiro sai na
  hora e os seguintes viram um só no fim da janela.
- A UI responde invalidando a consulta afetada (`['media', id]`), não aplicando dados do aviso. Reconectar recarrega as
  consultas já carregadas como rede de segurança, porque o canal de notificações descarta mensagens quando cheio.
- Mudanças causadas por uma chamada da própria UI continuam sem notificação (IPC-050 e a decisão f da fase 5 valem).

## Alternativas consideradas
- **Polling na UI depois de `scrape.apply`:** imprecisa (não se sabe quando a fila termina) e gasta chamadas.
- **Aviso de "fila terminou":** acrescenta contrato sem superar o aviso agrupado por jogo.
- **Levar os dados da imagem no aviso:** duplica a lógica de leitura e aumenta o risco de divergir do banco.

## Consequências
- ✅ Capa do cartão e carrossel atualizam sozinhos quando o download termina, sem mexer no foco do jogo.
- ✅ O padrão serve a outras mudanças em segundo plano (fase 12).
- ⚠️ O IPC passa a v0.12.0 (só acréscimo).
- ⚠️ Com a fila de 16 cheia um aviso ainda pode se perder; a próxima leitura corrige.
