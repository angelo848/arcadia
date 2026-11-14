# 0027 — Nome de plataforma único sem diferenciar maiúsculas

- **Status:** aceito (aprovado com a fase 9, [phase-9-plataformas](../specs/phase-9-plataformas/requirements.md))
- **Data:** 2026-11-13

## Contexto
`platform.name` é `UNIQUE` desde a migração 1, mas a comparação é binária: "Sony Playstation 2" e "Sony PlayStation 2"
podiam coexistir. Só o código de `AddPlatform` evitava a repetição. Com o seed e a importação do LaunchBox, grafias
diferentes do mesmo nome passam a ser comuns.

## Decisão
- A migração 00008 cria `platform_name_nocase_uidx` (`UNIQUE` em `name COLLATE NOCASE`). O banco passa a recusar nome
  repetido sem diferenciar maiúsculas (PLT-006), como já faz com `emulator.name`.
- Antes de criar o índice, a migração funde as plataformas repetidas na de menor id: os jogos mudam de `platform_id`,
  o emulador padrão é herdado se a sobrevivente não tiver um, e as demais são apagadas (PLT-005). "PC" e "Windows"
  juntas viram "Windows" (PLT-003).
- O `UNIQUE` binário antigo continua na coluna; o índice novo é mais restritivo e é o que vale.

## Alternativas consideradas
- **Manter a regra só no código:** uma escrita fora de `AddPlatform` (importação, renomear) reabre o problema.
- **Reconstruir a tabela `platform` para trocar a constraint:** `game` depende dela com `RESTRICT`; risco alto para
  remover uma constraint que o índice novo já torna redundante.

## Consequências
- ✅ Renomear e criar plataforma têm a unicidade garantida pelo banco; o erro vira `-32602` no IPC.
- ⚠️ A fusão na migração não é desfeita no `Down`.
- ⚠️ Depois da fusão podem existir dois jogos de mesmo título na mesma plataforma; nada quebra, e IMP-006 passa a
  considerar os dois.
