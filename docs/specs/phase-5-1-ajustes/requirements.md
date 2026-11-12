# Fase 5.1 — Ajustes da biblioteca: requisitos

- **Status:** rascunho, aguardando aprovação do usuário (decisões de produto registradas abaixo)
- **Base:** [phase-5-library](../phase-5-library/requirements.md) (implementada),
  [phase-3-tracker](../phase-3-tracker/requirements.md) (limitação 4: jogos que exigem administrador) e
  [single-exe](../single-exe/requirements.md) (início com o Windows).
- **ADRs relacionados:** 0005 (rastreamento em camadas), 0014 (início com o Windows, mantido), 0015 (agente único
  escritor), 0016 (Job Object permite breakaway), [0023](../../adr/0023-execucao-como-administrador.md) (proposto).
- **Objetivo:** permitir jogos que exigem administrador, sem perder a precisão do rastreio, e trocar o campo de gêneros
  por tags que sugerem os gêneros já cadastrados.

Formato: cada requisito tem ID estável (`ÁREA-NNN`), nunca reutilizado. Critérios em Dado/Quando/Então.
Áreas novas: **ADM** (execução como administrador) e **TAG** (tags de gênero). Todo ID precisa de pelo menos um teste
que o cite no nome, exceto os marcados `· verificação: manual`.

---

## Decisões de produto

1. **Como rodar jogos que exigem administrador: o agente roda elevado quando o usuário o inicia como administrador.**
   O agente deixa de ser elevado por padrão. Quem usa jogos de administrador abre o Arcadia como administrador, e a UI
   inicia o agente com a mesma elevação. O rastreio continua igual (Job Object, pausa e filhos), porque o agente é o
   único processo envolvido. O início automático com o Windows sobe o agente sem elevação; quem usa jogos de
   administrador desliga essa opção na bandeja (ADM-005).
2. **Tags:** entram nesta fase **só para gêneros**, e a condição combinada foi atendida: para um jogo novo, os gêneros
   já cadastrados aparecem como tags para escolher (TAG-001). Desenvolvedora e editora continuam como texto separado
   por vírgula e ficam para uma fase futura.

---

## ADM — Execução como administrador

### ADM-001 — Campo `requiresAdmin` no jogo
Dado um jogo novo, quando o registro é criado, então `requiresAdmin` é `false`. Dado um jogo existente depois da migração
para a versão 6, então `requiresAdmin` é `false` e nenhum outro campo muda. Dado `game.update` com `requiresAdmin`
(boolean), quando o agente grava, então o valor é salvo e `updated_at` muda; sem o campo no patch, o valor não muda
(EDT-009).

### ADM-002 — Jogo que exige administrador com agente sem elevação
Dado um jogo com `requiresAdmin = true` e um agente sem elevação, quando o usuário toca "Jogar" ou "Sem contar tempo",
então nenhum processo é lançado, nenhuma sessão é criada e o erro `-32010` chega à UI com a mensagem "Este jogo precisa
de administrador. Abra o Arcadia como administrador."

### ADM-003 — Jogo que falha por elevação sem a marca
Dado um jogo sem `requiresAdmin` que falha com `ERROR_ELEVATION_REQUIRED` (740), então a sessão não é criada (como hoje) e
a mensagem é a mesma de ADM-002, em vez de "Não foi possível iniciar o jogo".

### ADM-004 — Agente elevado: rastreio sem mudança
Dado o agente rodando como administrador, quando um jogo qualquer é lançado, então ele roda elevado e segue as regras de
rastreio, pausa e filhos já existentes (fases 3 e 4). Os jogos sem a marca também herdam a elevação do agente: isso é
consequência da decisão 1; o indicador do agente (`AgentIndicator`) mostra "Agente com privilégio de administrador" quando é o caso.

### ADM-005 — Início automático e conflito de agentes
Dado um agente sem elevação já em execução (por exemplo, iniciado com o Windows), quando o usuário abre a UI como
administrador, então a UI mostra "Já há um agente rodando sem privilégio de administrador. Saia dele pela bandeja e abra
o Arcadia de novo como administrador." e não usa o agente antigo. A mesma mensagem inclui a orientação "Para jogos de administrador,
desligue o início com o Windows na bandeja" (a opção continua na bandeja, sem mudar o padrão).
· verificação: manual.

### ADM-006 — Edição do campo
Dado a página de edição de um jogo, quando o usuário marca "Precisa de administrador" e salva, então o patch leva só
`requiresAdmin` (EDT-001). Dado a página do jogo, então o campo aparece como "Precisa de administrador: sim/não".
· verificação: manual (UI).

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
`genres` como array de nomes (EDT-004). Nenhuma mudança de IPC além de ADM-001.
· verificação: manual (UI).

---

## Fora de escopo nesta fase

- Scrapers e busca automática de metadados (fase 7).
- Tags para desenvolvedora e editora (fase futura, decisão 2).
- Rodar só o jogo elevado enquanto o agente segue sem elevação (exigiria um processo auxiliar; ADR 0023, alternativas).
- Trocar o padrão do início automático com o Windows (continua ligado, como no ADR 0014).
