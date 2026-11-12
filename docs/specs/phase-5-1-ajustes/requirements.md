# Fase 5.1 — Ajustes da biblioteca: requisitos

- **Status:** aprovado pelo usuário (requisitos em 2026-11-12); implementado nas tarefas T01..T08 e testado manualmente pelo usuário (2026-11-12)
- **Base:** [phase-5-library](../phase-5-library/requirements.md) (implementada),
  [phase-3-tracker](../phase-3-tracker/requirements.md) (limitação 4: jogos que exigem administrador) e
  [single-exe](../single-exe/requirements.md) (início com o Windows, AST-002..AST-011, substituídos aqui).
- **ADRs relacionados:** 0005 (rastreamento em camadas), 0014 (executável único; a parte de início com o Windows é
  substituída pela 0024), 0015 (agente único escritor), 0016 (Job Object permite breakaway),
  [0023](../../adr/0023-execucao-como-administrador.md) e [0024](../../adr/0024-agente-sempre-como-administrador.md)
  (ambos propostos).
- **Objetivo:** o Arcadia roda sempre como administrador, para que os jogos que exigem administrador funcionem com o
  mesmo rastreio; o início automático com o Windows sai; e o campo de gêneros vira tags que sugerem os já cadastrados.

Formato: cada requisito tem ID estável (`ÁREA-NNN`), nunca reutilizado. Critérios em Dado/Quando/Então.
Áreas novas: **ADM** (execução como administrador) e **TAG** (tags de gênero). Todo ID precisa de pelo menos um teste
que o cite no nome, exceto os marcados `· verificação: manual`.

---

## Decisões de produto

1. **O Arcadia sempre sobe como administrador.** Não há distinção por jogo: o `arcadia.exe` pede elevação ao ser aberto
   (manifesto `requireAdministrator`), a UI e o agente que ela inicia rodam elevados, e todos os jogos herdam essa
   elevação. O rastreio, a pausa e os filhos continuam como estão (fases 3 e 4).
2. **O início automático com o Windows é removido.** Não há entrada `Run`, nem item na bandeja, nem a preferência
   `autostart`. Na primeira subida da nova versão, a entrada `Arcadia` já existente é apagada (ADM-004).
3. **Tags: só gêneros, com sugestões do catálogo.** Para jogo novo ou existente, os gêneros já cadastrados aparecem para
   escolher (TAG-001). Desenvolvedora e editora continuam como texto separado por vírgula, para uma fase futura.

---

## ADM — Execução como administrador

### ADM-001 — Elevação na abertura
Dado o `arcadia.exe` de release, quando o usuário o abre (UI ou `--agent`), então o Windows pede a elevação (UAC) e o
processo só sobe se for aceita. Dado que a elevação é recusada, então nada sobe e nada é gravado.
Dado um build `dev` (`go run`, `wails dev`), então o build não força elevação e não toca no registro. · verificação: manual

### ADM-002 — Agente elevado, jogos herdam
Dado o agente rodando como administrador, quando um jogo qualquer é lançado, então ele roda elevado e segue as regras de
rastreio, pausa e filhos já existentes (fases 3 e 4). Não há campo por jogo: nenhuma coluna nova, nenhuma mudança em
`game.update`, nenhuma mudança na página do jogo. Consequência aceita pelo usuário: jogos sem exigência de administrador
também rodam elevados.

### ADM-003 — Lançamento de jogo que exige administrador
Dado um jogo que falha com `ERROR_ELEVATION_REQUIRED` (740), então não há sessão (como hoje) e a mensagem não é mais
"Não foi possível iniciar o jogo", e sim "Não foi possível iniciar o jogo: o Arcadia precisa estar como administrador.
Feche-o e abra de novo com a elevação." · verificação: manual

Observação: com ADM-001 o caso não deve ocorrer; o requisito cobre o caso de um executável do jogo que pede elevação
ainda mais alta, sem alterar o rastreio.

### ADM-004 — Início automático removido
Dado uma versão nova do agente subindo, quando há a entrada `HKCU\...\Run` com o valor `Arcadia` apontando para o mesmo
executável, então a entrada é apagada, e nenhum erro aparece se ela não existir. Dado a bandeja, então não há mais o item
"Iniciar com o Windows". Dado a preferência `autostart` no banco, então ela é ignorada e não é lida nem gravada; a linha
pode ficar sem uso (sem migração nova). Erro ao apagar a entrada é registrado em log e não derruba o agente (ver AST-009,
substituído).

### ADM-005 — Crash e relançamento
Dado o agente caindo, quando a UI o relança (fase 2, AGT-001/AGT-002), então o agente novo também sobe elevado, porque
herda a elevação da UI. Dado a UI fechada e o agente rodando, então o agente continua, como antes.
· verificação: manual

### ADM-006 — Agente antigo sem elevação
Dado um agente sem elevação já em execução (versão anterior, iniciada antes da remoção do início automático), quando o
usuário abre a UI como administrador, então a UI mostra o aviso "Já há um agente rodando sem privilégio de administrador.
Saia dele pela bandeja e abra o Arcadia de novo como administrador." e não usa o agente: lista, sessão e lançamentos
ficam indisponíveis até que o agente certo esteja rodando. Dado o agente elevado, então o indicador mostra "Agente com
privilégio de administrador" (ADM-002). Dado que a checagem falha, então a UI trata o agente como não elevado. Esta
regra estava na versão anterior da spec e foi reincluída aqui; o design (IsProcessElevated) já a previa.
· verificação: manual

---

## TAG — Gêneros como tags

### TAG-001 — Entrada de tags com gêneros já cadastrados
Dado o campo de gêneros na edição de um jogo novo ou existente, quando o usuário digita, então aparecem as sugestões do
catálogo de gêneros que contêm o texto digitado, sem diferenciar maiúsculas. Quando o usuário confirma com Enter ou
vírgula, então o texto vira uma tag. Quando o usuário toca numa sugestão, então ela vira uma tag com o nome exato do
catálogo.

### TAG-002 — Criação de tag nova
Dado um texto que não existe no catálogo, quando a tag é confirmada, então ela aparece marcada como "nova" e é criada
ao salvar (EDT-004). Dado um texto que existe no catálogo sem diferença de maiúsculas, então a tag usa a grafia gravada
(EDT-004, exemplo de "Team Cherry").

### TAG-003 — Validação e duplicatas
Dado um texto vazio ou com mais de 100 caracteres (DOM-016), quando o usuário tenta confirmar, então a tag não é criada
e a mensagem de erro aparece. Dado uma tag já presente (sem diferença de maiúsculas), então ela não é adicionada de novo.

### TAG-004 — Remoção e teclado
Dado uma tag na lista, quando o usuário toca no "×" dela, então ela sai da lista e o patch leva o conjunto sem ela.
Dado o campo vazio, quando o usuário pressiona Backspace, então a última tag é removida.

### TAG-005 — Filtro e contrato inalterados
Dado a biblioteca, então o filtro por gênero continua igual (FLT-002), e o contrato `game.update` continua recebendo
`genres` como array de nomes (EDT-004). Nenhuma mudança de IPC nesta fase.
· verificação: manual (UI).

---

## Fora de escopo nesta fase

- Scrapers e busca automática de metadados (fase 7).
- Tags para desenvolvedora e editora (fase futura).
- Elevação só do jogo, com o agente sem elevação (ADR 0023, alternativas).
- Início automático com o Windows, por qualquer mecanismo (ADR 0024).
