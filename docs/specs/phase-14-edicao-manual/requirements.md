# Fase 14 — Edição manual de imagens e do tempo jogado: requisitos

- **Status:** pedido direto do usuário em 2026-11-16, feito em versão enxuta (sem design, sem tarefas e sem ADR, a
  pedido dele). É a última mudança antes da versão 1.0.
- **Base:** [phase-7-midia](../phase-7-midia/requirements.md) (MID-004, MID-006),
  [phase-10-1-ajustes](../phase-10-1-ajustes/requirements.md) (AJU-020, limites por tipo),
  [phase-10-2-imagens-ao-vivo](../phase-10-2-imagens-ao-vivo/requirements.md) (aviso `media.changed`) e
  [phase-5-library](../phase-5-library/requirements.md) (edição do jogo).
- **Objetivo:** na janela "Editar jogo", trocar, adicionar e remover a capa, as screenshots e os fundos do jogo, e
  corrigir o tempo total jogado. **Sem mudança no tracker.**

Área nova: **EDM** (edição manual).

## Requisitos

### EDM-001 — Adicionar imagem de um arquivo
Dado um jogo e um arquivo PNG ou JPEG de até 20 MB, quando o usuário adiciona o arquivo como capa, screenshot ou fundo,
então a imagem é copiada para a mídia do jogo e aparece na lista dele, com provedor `user`.

### EDM-002 — Limites por tipo
Dado um jogo que já tem capa, quando o usuário adiciona outra capa, então a nova substitui a antiga (a antiga é
removida). Dado um jogo no limite de screenshots (10) ou de fundos (2), quando o usuário tenta adicionar mais uma, então
o pedido é recusado e nada muda. Arquivo que não é PNG nem JPEG também é recusado.

### EDM-003 — Remover imagem
Dado um jogo com uma imagem, quando o usuário a remove, então o registro e os arquivos dela (imagem e miniatura) são
apagados.

### EDM-004 — Imagens na janela de edição
Dado a janela "Editar jogo", então ela mostra a capa, as screenshots e os fundos do jogo, cada imagem com "Remover", e
cada tipo com um botão que abre o seletor de arquivos do Explorer ("Adicionar...", ou "Trocar..." para a capa
existente). As mudanças de imagem valem na hora, sem esperar "Salvar".

### EDM-005 — Editar o tempo total jogado
· alterado em 2026-11-25 (docs/specs/phase-16-edicao-sessoes, SED-007): inalterado; criar, editar e excluir sessões movem o total pela diferença e conservam este ajuste.
Dado a janela "Editar jogo", então ela mostra o tempo total em horas e minutos. Quando o usuário altera o valor e
salva, então o total do jogo passa a ser o valor digitado. As sessões registradas não mudam, por isso as estatísticas
por período continuam vindo delas. Valor inválido (minutos acima de 59, texto que não é número) desativa "Salvar".

## Contrato (acréscimos ao IPC v0.15.0)

| Método | Parâmetros | Resultado |
|---|---|---|
| `media.add` | `gameId`, `kind` (`cover`, `screenshot` ou `background`), `path` (absoluto) | `{ "added": bool }` |
| `media.remove` | `mediaId` | `{ "removed": true }` |
| `game.setPlayTime` | `gameId`, `totalPlaySeconds` (0 a 3 599 996 400) | o mesmo de `game.get` |

Erros: `-32602` para parâmetro inválido, arquivo recusado ou limite atingido; `-32001` para jogo inexistente.
`media.add` e `media.remove` emitem `media.changed` do jogo.
