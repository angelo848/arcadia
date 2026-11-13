# Fase 7 — Scrapers e gerenciador de mídia: tarefas

Rascunho para aprovação. Ordem de execução: uma tarefa por commit, testes da tarefa antes da implementação
(test-writer primeiro, quando a tarefa tiver testes novos). Requisitos em [requirements.md](requirements.md),
design em [design.md](design.md).

| Tarefa | Entrega | Requisitos | Depende de |
|---|---|---|---|
| T01 | Contrato 0.9.0, migração 00007, queries sqlc de mídia e fila | MID-003, MID-004, MID-005 | — |
| T02 | `domain/match.go`: normalização, similaridade e pontuação | MID-002 | — |
| T03 | `internal/scrapers`: interface, IGDB, SteamGridDB, Steam Store com fixtures | MID-002, MID-003, MID-008, MID-009 | T02 |
| T04 | Chaves no Credential Manager: `KeyStore`, implementação Windows e fake | MID-001 | — |
| T05 | Aplicação de metadados com merge por prioridade | MID-003 | T01, T03 |
| T06 | `internal/media`: download limitado, hash, gravação, miniatura | MID-004 | T01 |
| T07 | Fila em background: worker, retomada, cancelamento, retry | MID-005, MID-008 | T01, T06 |
| T08 | Handlers IPC e contrato 0.9.0 (provider, scrape, media) | MID-001 a MID-005 | T04, T05, T07 |
| T09 | Importação LaunchBox com imagens | MID-010 | T06, T08 |
| T10 | Bindings Wails e `lib` do frontend | MID-001 a MID-006 | T08 |
| T11 | UI: diálogo de chaves, candidatos, mídia na página e miniatura na grade | MID-001, MID-002, MID-006 | T10 |
| T12 | Verificação: `go test`, `vitest`, build, `spec-trace`, checagens manuais | todos | T01 a T11 |

## Testes por tarefa (IDs das requisições)

- **T01:** `MID-004/migração 7 sobe e desce`, `MID-004/hash único por jogo`, `MID-005/apagar jogo apaga mídia e jobs`.
- **T02:** `MID-002/normaliza ™, ®, edição e região`, `MID-002/similaridade e limiar 0,80`, `MID-002/plataforma e ano somam`.
- **T03:** `MID-002/IGDB busca e detalhes`, `MID-002/SteamGridDB com cabeçalho Bearer`, `MID-002/Steam Store sem chave`,
  `MID-008/429 é erro temporário`, `MID-009/nenhum dado do provedor é embutido` (teste de que `testdata` é só de teste).
- **T04:** `MID-001/chave gravada e lida pelo KeyStore fake`, `MID-001/status mostra só cadastrada ou não`,
  `MID-001/chave ausente não chama o provedor`, `MID-001/chave não aparece em log`.
- **T05:** `MID-003/campo vazio é preenchido`, `MID-003/campo do usuário não muda`, `MID-003/substituir troca tudo`.
- **T06:** `MID-004/grava em data\media\<id> com hash`, `MID-004/duplicata não grava de novo`,
  `MID-004/limite de 20 MB`, `MID-004/falha isola a mídia`, `MID-004/miniatura de 320 px`.
- **T07:** `MID-005/retomada de job running`, `MID-005/limite de concorrência 2`, `MID-005/cancelamento`,
  `MID-008/três tentativas e depois failed`.
- **T08:** `IPC-MID/contrato 0.9.0 nomes e campos`, `MID-001/setKey sem valor de volta`,
  `MID-002/candidatos ordenados`, `MID-005/media.queue enfileira`.
- **T09:** `MID-010/copia imagem do LaunchBox`, `MID-010/imagem ausente vira aviso`.
- **T10:** `MID-001/bindings chamam o agente`, testes de `lib/`.
- **T11:** `MID-001/campo de chave é password e limpa`, `MID-002/lista candidatos abaixo do limiar`,
  `MID-006/miniatura substitui o cartão`, `MID-006/sem mídia a grade igual`.

## Riscos e pontos de atenção

- **WebP:** só entra se a decodificação for possível sem CGO. Se não for, fica para fase posterior (design §7).
- **Limites de API:** cada provedor tem regras próprias; o limite de taxa é configurável por provedor.
- **Caminho LaunchBox (T09):** depende do XML real; se o campo não existir no arquivo do usuário, T09 é adiada.
- **Manual (T12):** chaves reais de IGDB e SteamGridDB, busca em jogo real, queda de rede no meio do download.
