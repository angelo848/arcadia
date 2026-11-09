# `cmd/fake-game` — contrato de linha de comando

Executável de teste (console, `CGO_ENABLED=0`, só Windows; `!windows` sai com 1) que simula jogos e lançadores para os
testes de integração do tracker (TRK-*). Escrito pelo tracker-specialist. Requisitos: FKG-001..FKG-008.

Não faz parte do produto: usa `time.Sleep`/tempo real (é o "jogo", não lógica testável do Arcadia). Nunca é embutido
nem distribuído com `arcadia.exe`.

## Modelo

Cada processo tem uma profundidade `d` (`--depth`). Se `d > 0`, ele cria `--fanout` filhos, cada um sendo o próprio
executável (`os.Executable()`) com **as mesmas flags**, exceto `--depth d−1`. Depois de criar os filhos (sem esperar
por eles), o processo vive pelo tempo de vida dele e sai:

- folha (`d = 0`): tempo de vida = `--run`;
- não folha (`d > 0`): tempo de vida = `--parent-run`.

Um processo sai quando acontece o primeiro destes: o tempo de vida acaba; `--until-file` existe (conferido a cada
50 ms); `--max-life` acaba. Tempo de vida `0` = sai logo depois de criar os filhos (padrão lançador).

## Flags

| Flag | Tipo | Padrão | Significado |
|---|---|---|---|
| `--depth N` | inteiro ≥ 0 | `0` | níveis abaixo deste processo |
| `--fanout K` | inteiro ≥ 1 | `1` | filhos criados por processo não folha |
| `--run D` | duração Go ≥ 0 ou `forever` | `0` | tempo de vida da folha |
| `--parent-run D` | duração Go ≥ 0 ou `forever` | `0` | tempo de vida de processo não folha |
| `--until-file P` | caminho | (nenhum) | sai assim que `P` existir (qualquer tipo de arquivo) |
| `--max-life D` | duração Go > 0 | `2m` | limite absoluto de vida, contra órfãos em testes |
| `--exit-code N` | inteiro 0..255 | `0` | código de saída de todo processo da árvore em saída normal |
| `--breakaway` | bool | `false` | cria os filhos com `CREATE_BREAKAWAY_FROM_JOB` |
| `--report P` | caminho | (nenhum) | relatório JSON Lines (abaixo) |

Argumentos posicionais depois das flags (`-- a b`) são ignorados, mas aparecem em `args` do relatório (TRK-005).
`forever` = sem limite próprio (só `--until-file`/`--max-life` encerram).

Combinações úteis:

| Cenário | Flags |
|---|---|
| jogo que roda N s e sai | `--run 5s` |
| sai na hora (sessão descartada) | `--run 0` |
| lançador que abre o jogo e sai | `--depth 1 --parent-run 0 --run forever --until-file F` |
| processo que abre filhos e continua | `--depth 1 --fanout 3 --parent-run forever --run forever --until-file F` |
| cadeia de 3 níveis | `--depth 2 --parent-run 0 --run forever --until-file F` |
| código de saída | `--run 0 --exit-code 7` |
| filho que escapa do job | `--depth 1 --breakaway --parent-run 2s --run forever --until-file F` |

## Códigos de saída

| Código | Quando |
|---|---|
| `--exit-code` | saída normal (tempo de vida, `--until-file`, `--max-life`) |
| `2` | flag desconhecida ou valor inválido (antes de criar filhos e de escrever no relatório) [FKG-007] |
| `4` | falha ao criar um filho (os já criados continuam; linha `exit` com `error`) [FKG-008] |
| `1` | outro erro (ex.: não consegue abrir o relatório) |

## Relatório (`--report P`) [FKG-006]

Cada processo acrescenta linhas a `P` (abre com `O_APPEND|O_CREATE|O_WRONLY`, uma chamada `Write` por linha, linha
< 4 KiB; processos concorrentes não intercalam linhas):

```json
{"event":"start","pid":1234,"ppid":1200,"depth":1,"cwd":"C:\\x\\y","args":["--depth","1","--run","forever"]}
{"event":"exit","pid":1234,"depth":1,"code":0}
{"event":"exit","pid":1234,"depth":1,"code":4,"error":"create child: Access is denied."}
```

- `start` é escrito antes de criar os filhos; `exit` logo antes de sair (não é escrito se o processo for morto).
- `ppid` vem de `os.Getppid()`; `cwd` de `os.Getwd()`; `args` = `os.Args[1:]`.

## Uso nos testes
- O `TestMain` dos testes de integração compila `./cmd/fake-game` uma vez para `t.TempDir()`/pasta temporária do
  pacote (`go build`), com espaço no caminho quando o teste exigir (TRK-005).
- Todo teste que usa `--run forever` cria o `--until-file` no `t.Cleanup` e usa `--max-life` curto (ex.: `30s`).
- Esperas por eventos do job usam prazo real curto no teste (ex.: 10 s) — os temporizadores do tracker continuam no
  `FakeClock`.
