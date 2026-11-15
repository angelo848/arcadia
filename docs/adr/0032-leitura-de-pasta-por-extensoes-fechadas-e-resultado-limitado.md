# 0032 — Leitura de pasta por lista fechada de extensões, grupos por pasta do jogo e resultado limitado ao teto do IPC

- **Status:** aceito (aprovado com a fase 11, [phase-11-importacao-v2](../specs/phase-11-importacao-v2/requirements.md))
- **Data:** 2026-11-15

## Contexto
A fonte "Pasta" lia só o primeiro nível, sem filtro de extensão. Ler subpastas de ROMs traz DLLs, saves, capas e trilhas
`.bin`; jogos de vários discos viram vários candidatos; e uma pasta grande gera um resultado maior que o teto de 1 MiB por
mensagem do IPC.

## Decisão
- A leitura é recursiva (até 8 níveis e 20.000 itens) e aceita só extensões de jogo, escolhidas pela chave da plataforma:
  Windows = executáveis; console do seed = ROM e imagem de disco; plataforma sem chave = as duas listas. A fonte "Arquivo"
  continua aceitando qualquer tipo.
- Trilhas e discos se ligam só por nome; nenhum arquivo é aberto. Discos do mesmo jogo, dentro da mesma subpasta de
  primeiro nível, viram um candidato (disco 1 ou `.m3u` como alvo).
- O título proposto é normalizado no agente (determinístico, sem tocar no nome do arquivo); o usuário o edita na janela de
  revisão e o `import.apply` grava o título recebido.
- O `import.scan` devolve no máximo 900 KiB, com aviso, sem paginação.

## Alternativas consideradas
- **Aceitar qualquer extensão menos uma lista de ignoradas:** traz muito lixo na leitura recursiva.
- **Paginar o `import.scan`:** acrescenta estado e contrato; o teto de mil jogos por leitura basta, e importa-se por subpasta.
- **Abrir `.cue`/`.m3u` para achar trilhas e discos:** custo e risco de leitura de arquivos arbitrários; o nome basta.

## Consequências
- ✅ Subpastas de ROMs viram candidatos limpos, com discos agrupados e títulos normalizados.
- ✅ O padrão (lista fechada, ligação por nome, teto de resultado) serve à coluna de match da fase 12.
- ⚠️ ROM de extensão fora da lista é ignorada na pasta (entra pela fonte "Arquivo") e contada no aviso.
- ⚠️ Só o disco 1 (ou o `.m3u`) é gravado; o modelo não tem "aplicação adicional".
- ⚠️ O IPC passa a v0.13.0.
