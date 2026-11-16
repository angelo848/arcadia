# Contrato: plataformas do seed e migração 00008

Requisitos: PLT-001..PLT-008, PLT-009, PLT-017, PLT-025, PLT-029, PLT-030.

## 1. Tabela do seed

A fonte da verdade em código é `domain.SeedPlatforms` (ordem abaixo). A migração 00008 grava os mesmos valores em SQL;
um teste da store compara o banco migrado com `domain.SeedPlatforms` para que os dois não divirjam.

| Chave (`seed_key`) | Nome canônico | Categoria | Ícone embutido | Sigla no ícone | IGDB `platform.id` |
|---|---|---|---|---|---|
| `windows` | Windows | `pc` | `windows.svg` | WIN | 6 |
| `3ds` | Nintendo 3DS | `handheld` | `3ds.svg` | 3DS | 37 |
| `n64` | Nintendo 64 | `console` | `n64.svg` | N64 | 4 |
| `nes` | Nintendo Entertainment System | `console` | `nes.svg` | NES | 18 |
| `gba` | Nintendo Game Boy Advance | `handheld` | `gba.svg` | GBA | 24 |
| `gamecube` | Nintendo GameCube | `console` | `gamecube.svg` | GC | 21 |
| `switch` | Nintendo Switch | `console` | `switch.svg` | NSW | 130 |
| `switch2` | Nintendo Switch 2 | `console` | `switch2.svg` | NS2 | 508 |
| `wii` | Nintendo Wii | `console` | `wii.svg` | Wii | 5 |
| `ps1` | Sony Playstation | `console` | `ps1.svg` | PS1 | 7 |
| `ps2` | Sony Playstation 2 | `console` | `ps2.svg` | PS2 | 8 |
| `ps3` | Sony Playstation 3 | `console` | `ps3.svg` | PS3 | 9 |
| `snes` | Super Nintendo Entertainment System | `console` | `snes.svg` | SNES | 19 |

- Os nomes canônicos seguem a grafia do LaunchBox (`Sony Playstation`, não `PlayStation`), para que a importação do
  LaunchBox caia direto nas plataformas do seed.
- **IGDB:** os 13 valores foram conferidos na API (`POST /v4/platforms`, `fields id,name,slug`) em 2026-11-13 (T02),
  incluindo o do Switch 2 (508, "Nintendo Switch 2").
- A chave do seed é estável: nunca muda de significado e nunca é reutilizada (como os IDs de requisito).

## 2. Ícones

- Arquivos SVG originais do projeto em `frontend/src/assets/platforms/<chave>.svg`, mais `generic.svg` (PLT-010).
  `viewBox="0 0 64 64"`, placa de cantos arredondados com uma cor por plataforma e o desenho do aparelho ou do
  controle dela (o genérico é um manche de fliperama), sem `<text>` e sem logotipos das fabricantes. Até 2026-11-16 a
  placa trazia a sigla da tabela; a coluna "Sigla no ícone" ficou só como referência.
- Os 14 arquivos são gerados por `node scripts/gen-icons.mjs`, que também grava o desenho do ícone do aplicativo em
  `scripts/icons/arcadia.svg`. Esse desenho, exportado em PNG de 1024 px com fundo transparente, vira o ícone do
  executável e o da bandeja (o mesmo) com `go run scripts/gen-icon.go <png> internal/agent/assets/icon.ico
  cmd/arcadia/build/windows/icon.ico cmd/arcadia/build/appicon.png`.
- Entram no executável pelo bundle do Vite (os assets do Wails já são embutidos com `//go:embed`); o agente não os lê.
- Ícone do usuário: `data\media\platforms\<sha256>.<png|jpg>`; a coluna `platform.icon` guarda o caminho relativo a
  `data\media` com barra invertida do Windows, como `media.path` (ex.: `platforms\3f…a1.png`).

| `seed_key` | `icon` | Ícone mostrado |
|---|---|---|
| definida | `NULL` | embutido da chave (PLT-009) |
| `NULL` | `NULL` | genérico (PLT-010) |
| qualquer | caminho | arquivo do usuário (PLT-011) |

## 3. Migração 00008: transições da tabela `platform`

Executada numa só transação do goose (PLT-008), nesta ordem. "Mesmo nome" = `trim(name)` igual sem diferenciar
maiúsculas (`COLLATE NOCASE`).

| Passo | Estado antes | Efeito |
|---|---|---|
| 1 | — | `ALTER TABLE platform ADD COLUMN seed_key TEXT` |
| 2 | duas ou mais linhas com o mesmo nome | jogos das outras → menor id; menor id herda `emulator_id` da primeira outra (ordem de id) se o seu for `NULL`; as outras são apagadas (PLT-005) |
| 3 | existem "PC" e "Windows" | jogos de "PC" → "Windows"; "Windows" herda `emulator_id` de "PC" se o seu for `NULL`; "PC" é apagada (PLT-003) |
| 4 | existe "PC", não existe "Windows" | `name = 'Windows'` na linha de "PC" (PLT-002) |
| 5 | linha com o mesmo nome de um nome canônico | `seed_key`, nome canônico e categoria do seed gravados na linha; id, jogos, `emulator_id` e `icon` mantidos (PLT-004) |
| 6 | nome canônico sem linha | `INSERT` com nome, categoria e `seed_key` do seed (PLT-001) |
| 7 | — | `CREATE UNIQUE INDEX platform_name_nocase_uidx ON platform (name COLLATE NOCASE)` (PLT-006) |
| 8 | — | `CREATE UNIQUE INDEX platform_seed_key_uidx ON platform (seed_key) WHERE seed_key IS NOT NULL` |

- O passo 2 roda antes do 3..5 para que cada nome tenha no máximo uma linha quando o seed é aplicado.
- Depois do passo 2, os nomes com espaços nas pontas são aparados (`name = trim(name)`), para que a resolução por nome
  e o índice do passo 7 valham para eles.
- Passo 2, herança de emulador: vale a primeira outra linha, por ordem de id, que **tenha** emulador.
- Passo 6: os ids das linhas inseridas ficam acima do maior id que existia antes da migração, para que o SQLite não
  reaproveite o id de uma plataforma apagada na fusão. Em banco vazio os ids ficam 1..13 (`windows` = 1).
- `game.platform_id` é `ON DELETE RESTRICT`: toda linha apagada já teve os jogos movidos no mesmo passo.
- `game_fts` não guarda o nome da plataforma; nenhuma reindexação é necessária.
- **Down:** apaga os dois índices e a coluna `seed_key`. Renomeações, fusões e plataformas inseridas não são desfeitas
  (migração de dados sem volta; registrado no comentário do arquivo).

## 4. Resolução por nome depois da migração (PLT-025, PLT-017)

| Situação | Resultado |
|---|---|
| existe plataforma com o mesmo nome | essa plataforma |
| nome é canônico da chave K e existe plataforma com `seed_key = K` | essa plataforma (mesmo renomeada) |
| nome é canônico da chave K e não há plataforma com `seed_key = K` | cria com o nome canônico, categoria do seed e `seed_key = K` (só quando a criação é permitida) |
| nenhum dos casos | cria com nome e categoria enviados, `seed_key = NULL` (só quando a criação é permitida) |

O scan da importação nunca cria (devolve "não encontrada"). `game.add` (PLT-027) resolve direto pela chave `windows`.
