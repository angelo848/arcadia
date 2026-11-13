# Fase 7 — Scrapers e gerenciador de mídia: design

Base: [requirements.md](requirements.md). ARCHITECTURE §5, §6 e §9. Contrato IPC: `contracts/ipc.openrpc.json`
(versão 0.9.0, a escrever na tarefa T01).

## 1. Pacotes

| Pacote | Responsabilidade | Restrições |
|---|---|---|
| `internal/scrapers` | interface `MetadataProvider` (ARCHITECTURE §6) e clientes IGDB, SteamGridDB e Steam Store | HTTP com `context`, timeout e URL base injetável (testes com `httptest`) |
| `internal/domain/match.go` | normalização de título, similaridade (Dice sobre bigramas), pontuação com plataforma e ano, limiar 0,80 | puro, sem I/O |
| `internal/winapi/credentials_windows.go` | guardar, ler e apagar chaves no Credential Manager | `//go:build windows`; atrás de interface `KeyStore` para teste |
| `internal/store` | migração `00007_media.sql`, queries sqlc de mídia e da fila, aplicação de metadados com merge | `store` não importa Windows |
| `internal/media` | download com limite de tamanho, hash SHA-256, gravação em `data\media\<id>\`, miniatura JPEG | recebe `http.Client` e `Clock` por injeção |
| `internal/agent/scrape.go`, `internal/agent/media_queue.go` | handlers IPC e fila em background | um goroutine dono da fila; sai com o contexto do agente e o `WaitGroup` espera |

Regras de sempre (CLAUDE.md): tempo via `domain.Clock`; erros com `%w`; `slog`; sem CGO; testes sem rede externa.

## 2. Banco (migração 00007)

```sql
CREATE TABLE media (
  id          INTEGER PRIMARY KEY,
  game_id     INTEGER NOT NULL REFERENCES game (id) ON DELETE CASCADE,
  kind        TEXT    NOT NULL CHECK (kind IN ('cover','screenshot','title')),
  provider    TEXT    NOT NULL,          -- 'igdb' | 'steamgriddb' | 'steamstore' | 'launchbox'
  source_url  TEXT    NOT NULL DEFAULT '',
  hash        TEXT    NOT NULL,          -- sha-256 do conteúdo
  path        TEXT    NOT NULL,          -- relativo a data\media
  thumb_path  TEXT    NOT NULL DEFAULT '',
  created_at  TEXT    NOT NULL,
  UNIQUE (game_id, hash)
);
CREATE TABLE media_job (
  id          INTEGER PRIMARY KEY,
  game_id     INTEGER NOT NULL REFERENCES game (id) ON DELETE CASCADE,
  kind        TEXT    NOT NULL,
  provider    TEXT    NOT NULL,
  source_url  TEXT    NOT NULL,
  state       TEXT    NOT NULL CHECK (state IN ('pending','running','done','failed')),
  attempts    INTEGER NOT NULL DEFAULT 0,
  last_error  TEXT    NOT NULL DEFAULT '',
  created_at  TEXT    NOT NULL
);
```

Chaves de API **não** vão ao banco (MID-001). Provedor cadastrado é consultado no Credential Manager a cada uso.
`SchemaVersion` passa de 6 para 7; os testes de migração (`migrate5_test.go` e similares) são atualizados.

## 3. Provedores

- **IGDB:** token OAuth de cliente (`client_credentials`) obtido com Client ID e Secret, renovado perto do vencimento.
  Busca por `search` e detalhes por id; screenshots por `screenshots`.
- **SteamGridDB:** cabeçalho `Authorization: Bearer <chave>`; capas e heróis por id de jogo encontrado pela busca.
- **Steam Store:** `appdetails` por appid (sem chave); descrição e screenshots. Só para jogos PC com appid conhecido
  (importado da Steam ou casado por busca em `storesearch`).

Todos respeitam um limite de requisições por segundo por provedor e tratam 429 como falha temporária (MID-008).

## 4. Matching (MID-002)

1. Normaliza: minúsculas, sem acentos, sem ™ ® ©, sem sufixos de edição ("Edition", "Edição", "GOTY", "Remastered")
   e sem marcas de região `(USA)`, `[Europe]`, etc.
2. Similaridade Dice sobre bigramas entre o título normalizado do jogo e o do candidato.
3. Pontuação final = similaridade + 0,05 se a plataforma bate + 0,05 se o ano bate (teto 1,0).
4. Melhor candidato ≥ 0,80 é aplicado sozinho; abaixo disso a UI lista os candidatos.

## 5. Merge (MID-003)

Prioridade fixa por campo, em `scrapers/priority.go`: descrição e gêneros: IGDB, depois Steam Store; capa e hero:
SteamGridDB; screenshot: IGDB, depois Steam Store. Só preenche campo vazio, salvo "substituir" explícito.

## 6. Fila (MID-005, MID-008)

- Worker único por agente (um goroutine) com concorrência 2 para downloads (semáforo).
- Ao iniciar, jobs `running` voltam a `pending` (retomada). Falha: `attempts++`, `last_error`, até 3 tentativas
  com espera de 30 s, depois `failed`. Um job falho não derruba os outros.
- Cancelamento por `media.cancel` (marca o job, o worker respeita o contexto do download).

## 7. Arquivos (MID-004)

`data\media\<gameID>\<hash>.<ext>` e `thumb_<hash>.jpg` (largura máxima 320 px, gerada com `image` da biblioteca
padrão e redução por média de blocos). Tamanho máximo do download: 20 MB por imagem. Tipos aceitos: JPEG, PNG, WebP
(WebP só se a decodificação for possível sem CGO; senão fica de fora e isso é registrado no design de tarefas).

## 8. IPC (contrato 0.9.0)

| Método | Entrada | Saída |
|---|---|---|
| `provider.keyStatus` | `provider` | `{configured: bool}` |
| `provider.setKey` | `provider`, `key` | `{configured: bool}` |
| `provider.deleteKey` | `provider` | `{configured: bool}` |
| `scrape.search` | `gameId`, `provider` | `candidates[]` com id, título, plataforma, ano, pontuação |
| `scrape.apply` | `gameId`, `provider`, `providerId`, `replace` | `game` atualizado e `queued` (número de mídias) |
| `media.list` | `gameId` | `media[]` (kind, path, thumbPath, provider) |
| `media.queue` | `gameId`, `kinds` | `queued` |
| `media.cancel` | `jobId` | `ok` |

Erros: chave ausente → -32010 "Cadastre a chave…"; provedor fora → -32011; candidato não encontrado → -32001 (como
emulador). Os códigos 32010 e 32011 entram no contrato.

## 9. Desktop e UI

- `cmd/arcadia`: bindings `ProviderKeyStatus`, `ProviderKeySet`, `ProviderKeyDelete`, `ScrapeSearch`, `ScrapeApply`,
  `MediaList`, `MediaQueue`, `MediaCancel`. Os nomes seguem o padrão das outras fases.
- Configurações de chaves: diálogo (como o de Emuladores) com um campo por provedor; mostra "cadastrada" ou "não
  cadastrada". Campo de chave é `type="password"`, limpo após salvar.
- Página do jogo: botão "Buscar mídia" abre o diálogo de candidatos; depois mostra screenshots e title screen.
- Grade: a miniatura da capa substitui o cartão sem imagem (MID-006).
- Importação LaunchBox (MID-010): o `internal/importers/launchbox.go` lê o caminho de imagem do XML e entrega a cópia
  ao `internal/media` pelo agente, depois do `import.apply`.

## 10. Testes

- `domain/match_test.go`: tabela de títulos (™, edições, regiões, acentos) e limiar.
- `scrapers/*_test.go`: respostas gravadas em `testdata/` com `httptest`; 429 e 500 viram erro temporário.
- `winapi`: teste de build; a lógica usa o `KeyStore` fake.
- `store`: migração 7 para cima e para baixo; `UNIQUE (game_id, hash)`; apagar jogo apaga mídia e jobs.
- `media`: hash, limite de tamanho, miniatura, falha de download isolada.
- `agent`: fila com relógio fake e provedor fake; retomada de job `running`.
- `ipc`: teste de contrato 0.9.0 (nomes, campos, códigos).
- Frontend: diálogo de chaves, diálogo de candidatos, miniatura na grade (Vitest).

## 11. Pontos que dependem de verificação

- O caminho de imagem exato do XML do LaunchBox (MID-010) é confirmado no arquivo real do usuário.
- Termos de uso de IGDB, SteamGridDB e Steam Store: revisão manual antes da release (MID-009).
- Dependência `github.com/zalando/go-keyring` (ARCHITECTURE §6) ou acesso direto a `advapi32` pelo `x/sys/windows`:
  decidir na T04 pelo menor código seguro.
