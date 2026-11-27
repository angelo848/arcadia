# ADRs — Architecture Decision Records

Registros **curtos** das decisões de arquitetura: o que decidimos, por quê e o que custa.

## Regras
- Um arquivo por decisão: `NNNN-titulo-em-kebab-case.md`, numeração sequencial de 4 dígitos.
- Copie [`0000-template.md`](0000-template.md). Meta: caber em uma tela (~20–40 linhas).
- Status: `proposto` → `aceito` → (`substituído por NNNN` | `descontinuado`).
- ADR aceito **não é reescrito**. Mudou de ideia? Crie um novo ADR e marque o antigo como `substituído por NNNN`.
- Escreva um ADR quando a decisão for difícil de reverter, afetar mais de um pacote ou for algo que alguém
  perguntaria "por que fizemos assim?" daqui a 6 meses.

## Índice

| Nº | Título | Status |
|---|---|---|
| [0001](0001-registrar-decisoes-em-adrs.md) | Registrar decisões de arquitetura em ADRs | aceito |
| [0002](0002-go-wails-react.md) | Go + Wails v2 + React como stack | aceito |
| [0003](0003-agente-separado-da-ui.md) | Agente residente separado da UI, IPC por named pipe | aceito (em parte substituído por 0014) |
| [0004](0004-sqlite-modernc-goose-sqlc.md) | SQLite (modernc, sem CGO) + goose + sqlc | aceito |
| [0005](0005-rastreamento-em-camadas.md) | Rastreamento de tempo em camadas (Job Object + watcher + sinais de loja) | aceito |
| [0006](0006-biblioteca-portatil.md) | Nome `arcadia` e biblioteca portátil | aceito |
| [0007](0007-fluxo-de-subagentes.md) | Fluxo de subagentes com separação autor/teste | aceito |
| [0008](0008-spec-driven-development.md) | Spec-Driven Development com rastreabilidade por ID | aceito |
| [0009](0009-timestamps-unix-ms.md) | Instantes como INTEGER em milissegundos Unix (UTC) | aceito |
| [0010](0010-agente-dono-das-migracoes.md) | Só o agente migra o schema | aceito |
| [0011](0011-clock-com-temporizadores.md) | `domain.Clock` com temporizadores para toda espera | aceito |
| [0012](0012-enums-validados-no-domain.md) | Enums validados no `domain`, sem CHECK no banco | aceito |
| [0013](0013-log-fecha-por-ultimo.md) | O log fecha por último no shutdown | aceito |
| [0014](0014-executavel-unico-dois-processos.md) | Executável único com dois processos | aceito |
| [0015](0015-agente-unico-escritor-do-banco.md) | O agente é o único escritor do banco; a UI escreve via IPC | aceito |
| [0016](0016-job-do-jogo-permite-breakaway.md) | O Job Object do jogo permite breakaway | aceito |
| [0017](0017-modelo-de-pausa.md) | Modelo de pausa da sessão | aceito (ressalva em [0041](0041-sessoes-editaveis-pelo-usuario.md): em sessão editada `active_seconds` é a verdade) |
| [0018](0018-entrada-global-e-feedback-nativo.md) | Entrada global e feedback nativos no agente | aceito |
| [0019](0019-lancamento-sem-rastrear.md) | Lançamento sem rastrear | aceito |
| [0020](0020-indice-de-busca-e-filterspec.md) | Índice de busca e `FilterSpec` | aceito |
| [0021](0021-remocao-de-jogo.md) | Remoção de jogo | aceito |
| [0022](0022-listagem-paginada-no-ipc.md) | Listagem paginada no IPC | aceito |
| [0023](0023-execucao-como-administrador.md) | Execução de jogos como administrador | aceito |
| [0024](0024-agente-sempre-como-administrador.md) | Remoção do início automático com o Windows | aceito |
| [0025](0025-rastreio-por-processo-adotado.md) | Rastreio por processo adotado | aceito |
| [0026](0026-plataformas-do-seed-com-chave-estavel.md) | Plataformas do seed com chave estável | aceito |
| [0027](0027-nome-de-plataforma-unico-sem-maiusculas.md) | Nome de plataforma único sem diferenciar maiúsculas | aceito |
| [0028](0028-biblioteca-em-tres-colunas-sem-pagina-do-jogo.md) | Biblioteca em três colunas e fim da página do jogo | aceito |
| [0029](0029-tema-por-variaveis-css.md) | Tema por variáveis CSS | aceito |
| [0030](0030-limites-de-imagem-por-tipo-como-tabela-do-dominio.md) | Limites de imagem por tipo como tabela do domínio | aceito |
| [0031](0031-avisos-de-mudanca-em-segundo-plano-como-notificacoes-de-invalidacao.md) | Avisos de mudança em segundo plano como notificações de invalidação | aceito |
| [0032](0032-leitura-de-pasta-por-extensoes-fechadas-e-resultado-limitado.md) | Leitura de pasta por extensões fechadas e resultado limitado ao teto do IPC | aceito |
| [0033](0033-busca-unificada-de-metadados-com-agregador-e-confirmacao-em-lote.md) | Busca unificada de metadados com agregador, pontuação com travas e confirmação em lote | aceito |
| [0034](0034-limite-de-imagens-conta-atuais-e-enfileiradas.md) | Limite de imagens por tipo conta as atuais e as enfileiradas | aceito |
| [0035](0035-fechar-o-jogo-pelo-controle.md) | Fechar o jogo pelo controle (Select + B) | aceito |
| [0036](0036-big-box-com-temas-selecionaveis.md) | Big Box com temas selecionáveis | aceito |
| [0037](0037-video-do-screenscraper-como-midia-na-fila.md) | Vídeo do ScreenScraper como mídia `video` na fila | aceito |
| [0038](0038-big-box-por-pedido-externo.md) | Big Box por pedido externo (Select + A e `--bigbox`) | aceito |
| [0039](0039-tempo-para-zerar-hltb.md) | Tempo para zerar do HowLongToBeat com cache, reserva do IGDB e falha silenciosa | aceito |
| [0040](0040-contrato-de-tema-v2.md) | Contrato de tema v2 do Big Box | aceito (estende 0036) |
| [0041](0041-sessoes-editaveis-pelo-usuario.md) | Sessões editáveis pelo usuário: origem `manual`, total por diferença e sem sobreposição | aceito (ressalva ao 0017: em sessão editada `active_seconds` é a verdade) |
| [0042](0042-layout-em-colunas-no-contrato-de-tema-do-big-box.md) | Layout em colunas no contrato de tema do Big Box | aceito (estende 0040) |
