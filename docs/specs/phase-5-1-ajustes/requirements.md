# Fase 5.1 — Ajustes da biblioteca: requisitos

- **Status:** rascunho, aguardando aprovação do usuário
- **Base:** [phase-5-library](../phase-5-library/requirements.md) (implementada) e
  [phase-3-tracker](../phase-3-tracker/requirements.md) (limitação 4: jogos que exigem administrador).
- **ADRs relacionados:** 0015 (agente único escritor), 0016 (Job Object permite breakaway), 0005 (rastreamento em camadas),
  [0023](../../adr/0023-execucao-como-administrador.md) (proposto, decisão do item ADM).
- **Objetivo:** permitir jogos que exigem administrador e transformar os gêneros em tags na edição.

Formato: cada requisito tem ID estável (`ÁREA-NNN`), nunca reutilizado. Critérios em Dado/Quando/Então.
Áreas novas: **ADM** (execução como administrador) e **TAG** (tags de gênero). Todo ID precisa de pelo menos um teste
que o cite no nome, exceto os marcados `· verificação: manual`.

---

## Perguntas para o usuário

Duas decisões de produto. A spec usa a recomendação indicada; se o usuário discordar, o requisito muda antes da aprovação.

1. **Como rastrear um jogo que roda como administrador? (recomendação: opção A).**
   - **A (recomendada):** o agente lança o jogo com elevação (UAC) e mede o tempo pelo ciclo de vida do processo
     principal, sem Job Object. Não há pausa por teclado/controle nesse modo, e o tempo dos processos filhos não é contado.
   - **B:** rodar o agente inteiro como administrador. Descartada: o agente inicia com o Windows e um serviço elevado
     amplia a superfície de ataque.
   - **C:** não rastrear esses jogos. Descartada: é a limitação atual, e o usuário pediu suporte.
2. **Gêneros como tags, e desenvolvedora/editora também? (recomendação: só gêneros nesta fase).** Desenvolvedora e editora
   continuam como texto separado por vírgula. Estender para elas é trocar o componente de TAG-001, sem mudar o contrato.

---

## ADM — Execução como administrador

### ADM-001 — Campo `runAsAdmin` no jogo
Dado um jogo novo, quando o registro é criado, então `runAsAdmin` é `false`. Dado um jogo existente depois da migração
para a versão 6, então `runAsAdmin` é `false` e nenhum outro campo muda. Dado `game.update` com `runAsAdmin` (boolean),
quando o agente grava, então o valor é salvo e `updated_at` muda; sem o campo no patch, o valor não muda (EDT-009).

### ADM-002 — Elevação pelo Windows
Dado um jogo com `runAsAdmin = true`, quando o usuário toca "Jogar" ou "Sem contar tempo", então o agente lança o
executável pelo verbo `runas` (UAC), e o Windows mostra a confirmação de elevação. Dado que o usuário recusa a
confirmação, então nenhuma sessão é criada e o erro `-32010` ("elevação recusada") chega à UI com a mensagem
"Permissão de administrador recusada".

### ADM-003 — Rastreio de jogo elevado (opção A)
Dado um jogo elevado iniciado com sucesso, quando o processo principal encerra, então a sessão termina com o instante de
saída (mesma regra de SES-005). Dado que o agente não consegue abrir o processo elevado com `SYNCHRONIZE`, então a sessão
não é criada e o erro `-32011` ("não foi possível acompanhar o jogo elevado") é mostrado, sem contar tempo.

### ADM-004 — Pausa e filhos em jogo elevado
Dado uma sessão elevada em execução, quando o usuário pressiona a combinação de pausa, então nada acontece e a UI mostra
"Pausa indisponível para jogos com administrador". Dado que o jogo abre um processo filho, então o filho não entra no
tempo; a limitação aparece na página do jogo.

### ADM-005 — Edição do campo
Dado a página de edição de um jogo, quando o usuário marca "Executar como administrador" e salva, então o patch leva só
`runAsAdmin` (EDT-001). Dado a página do jogo, então o campo aparece como "Executa como administrador: sim/não".
· verificação: manual (UI).

---

## TAG — Gêneros como tags

### TAG-001 — Entrada de tags
Dado o campo de gêneros da edição, quando o usuário digita, então aparecem as sugestões do catálogo de gêneros que
contêm o texto digitado, sem diferenciar maiúsculas. Quando o usuário confirma com Enter ou vírgula, então o texto vira
uma tag. Quando o usuário toca numa sugestão, então ela vira uma tag com o nome exato do catálogo.

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

- Scrapers e busca automática de metadados (fase 7). A escolha de candidato entra lá.
- Elevação do agente inteiro (opção B da pergunta 1).
- Pausa e rastreio de filhos em jogo elevado (ADM-004): só com a fase 6 (Camada 2).
