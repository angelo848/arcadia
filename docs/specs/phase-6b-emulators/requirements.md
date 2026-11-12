# Fase 6b — Emuladores: requisitos

- **Status:** aprovado pelo usuário (2026-11-12). Decisões: sobrescrita por jogo só na página do jogo (recomendação); modelo com {rom} e {dir}; rastreio pelo Job do emulador como um exe.
- **Base:** [phase-6-importers](../phase-6-importers/requirements.md) (importação do LaunchBox deixa de fora os jogos
  com emulador, com aviso por plataforma) e [phase-5-library](../phase-5-library/requirements.md) (EDT-010: tipo, alvo e
  argumentos editáveis).
- **ARCHITECTURE:** §5 (`emulator(id, name, exe_path, args_template)`, `platform.emulator_id`, `launch_type = emulator`).
- **Objetivo:** cadastrar emuladores, associá-los a plataformas e lançar jogos de console pelo emulador, com o mesmo
  rastreio de tempo dos jogos de PC.

Formato: IDs `ÁREA-NNN`, critérios Dado/Quando/Então. Área nova: **EMU**.

---

## Decisões de produto (recomendação indicada)

1. **Associação emulador ↔ plataforma:** cada plataforma tem no máximo um emulador padrão (`platform.emulator_id`).
   Um jogo pode sobrescrever o emulador (campo opcional no jogo). Recomendação: sobrescrita só na página do jogo.
2. **Argumentos:** modelo de texto com `{rom}` (caminho do jogo) e, opcionalmente, `{dir}` (pasta do jogo). Ex.:
   `-f "{rom}"`. Recomendação: sem outros marcadores nesta fase.
3. **Rastreio:** jogo emulado é rastreado pelo Job Object do emulador (a árvore que o agente cria), como um jogo `exe`.
   Emuladores que delegam a outro processo que escapa do Job caem na mesma limitação de RUL-005 e são informados na página.

---

## EMU — Emuladores

### EMU-001 — Cadastro de emulador
Dado um emulador com nome, caminho de executável absoluto e modelo de argumentos (até 2000 caracteres), quando o usuário
salva, então ele é gravado. Nome vazio ou duplicado (sem diferença de maiúsculas) é recusado com -32602.

### EMU-002 — Associação à plataforma
Dado uma plataforma, quando o usuário escolhe um emulador padrão, então `platform.emulator_id` é gravado. Apagar o
emulador deixa a plataforma sem emulador (ON DELETE SET NULL), e nenhum jogo é apagado.

### EMU-003 — Jogo importado do LaunchBox sem emulador
Dado um jogo de console importado do LaunchBox (ROM como alvo, sem emulador), quando não há emulador padrão para a
plataforma, então o jogo é gravado com `launch_type = emulator` e um aviso na página ("sem emulador configurado"). Ao
escolher um emulador para a plataforma, os jogos dela passam a lançar por ele, sem reimportar.

### EMU-004 — Lançamento pelo emulador
Dado um jogo `launch_type = emulator` com emulador resolvido (EMU-002 ou sobrescrita do jogo), quando o usuário toca
"Jogar", então o agente cria o processo do emulador em Job Object (como um jogo `exe`, SES-001), com os argumentos
do modelo e `{rom}` trocado pelo alvo do jogo. Sessão, pausa e carência valem como para `exe`.

### EMU-005 — Erros de emulador
Dado um emulador sem executável no caminho, ou um jogo sem emulador resolvido, então "Jogar" não cria sessão e a UI
mostra "Não há emulador configurado para esta plataforma" ou "O emulador não foi encontrado em <caminho>".

### EMU-006 — Edição de emulador e plataforma na UI
Dado a tela de emuladores (nova, acessível pelo cabeçalho), o usuário cadastra, edita e apaga emuladores, e escolhe o
emulador padrão de cada plataforma. Apagar pede confirmação (como DEL-001).
· verificação: manual (UI)

---

## Fora de escopo nesta fase

- Scrapers e capas de emuladores (fase 7).
- Modelos de argumentos com outros marcadores além de `{rom}` e `{dir}`.
- Emuladores que rodam ROM em pasta compactada (zip) sem extração.

---

## Decisões aprovadas

1. Cada plataforma tem um emulador padrão. Um jogo pode sobrescrever o emulador na página do jogo (não em lote).
2. O modelo de argumentos aceita `{rom}` e `{dir}`.
3. O rastreio fica no Job do emulador, como um `exe`.

Consequência para a fase 6: o importador do LaunchBox deixa de excluir jogos com emulador; eles entram como candidatos
`launch_type = emulator` com a ROM como alvo (EMU-003). O aviso por plataforma sai.
