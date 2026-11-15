# Roadmap pós-POC (fases 9 a 13)

As fases 1 a 8 estão em [ARCHITECTURE.md §9](ARCHITECTURE.md). Este documento agrupa os 10 pontos de melhoria
levantados depois da primeira versão funcional. Cada fase segue o fluxo SDD ([ADR 0008](adr/0008-spec-driven-development.md)):
spec → tarefas aprovadas → testes → implementação → revisão.

## Pontos de origem

| # | Ponto | Fase |
|---|---|---|
| 1 | Normalização de títulos de ROM (`(U)`, `[!]`, etc.) | 11 |
| 2 | Nova interface: barra de filtros à esquerda, detalhes à direita, sem página do jogo | 10 |
| 3 | Janelas (emuladores, importar, etc.) maiores | 10 |
| 4 | UI abre maximizada (sem ser Big Box) | 10 |
| 5 | Gerenciamento de plataformas (criar, ícone, emulador padrão) e agrupamento na barra lateral | 9 |
| 6 | Buscar metadados e imagens já na importação, com janela de confirmação | 12 |
| 7 | Importação por pasta lendo subpastas | 11 |
| 8 | Busca unificada de metadados (sem escolher provedor; match por título + plataforma) | 12 |
| 9 | Atalho de controle Select + B para fechar o jogo e voltar ao Big Box | 13 |
| 10 | Seed das plataformas principais; "PC" vira "Windows" | 9 |

## Ordem e dependências

```
9 Plataformas ──► 10 Nova interface ──► 11 Importação v2 ──► 12 Metadados unificados
13 Modo console (independente; pode entrar em qualquer ponto)
```

- A **9** vem primeiro porque a barra lateral da 10 lista plataformas com ícone, e o match automático da 12 depende
  de plataformas conhecidas (com o identificador de cada uma nos provedores).
- A **10** vem antes da 11 e da 12 para que as telas novas de importação e de confirmação já nasçam no layout e nos
  tamanhos de janela novos, em vez de serem refeitas.
- A **11** entrega a janela de revisão; a **12** acrescenta a ela a coluna de match dos provedores.

---

## Fase 9 — Plataformas (pontos 5 e 10)

**Entrega**
- Tela "Plataformas", nos moldes da de emuladores: listar, criar, renomear, trocar ícone, definir emulador padrão,
  excluir (bloqueado enquanto houver jogo na plataforma).
- Migração com o seed: Windows, Nintendo 3DS, Nintendo 64, Nintendo Entertainment System, Nintendo Game Boy Advance,
  Nintendo GameCube, Nintendo Switch, Nintendo Switch 2, Nintendo Wii, Sony Playstation, Sony Playstation 2,
  Sony Playstation 3, Super Nintendo Entertainment System. Cada uma com ícone embutido no executável.
- "PC" passa a "Windows": a migração renomeia a linha existente (os jogos acompanham pelo `platform_id`) e os
  importadores de Steam, Epic e arquivo `.exe` passam a usar "Windows".
- Cada plataforma do seed guarda o identificador dela nos provedores de metadados (usado na fase 12).

**Áreas**: `internal/store` (migração, queries), `internal/domain`, `internal/agent` + `internal/ipc` (métodos de
plataforma), `internal/importers` (constante `PlatformPC`), `frontend` (diálogo de plataformas).

**A decidir na spec**
- Origem dos ícones (conjunto livre embutido; o usuário pode trocar por um arquivo dele).
- Plataformas importadas do LaunchBox com nome parecido com o do seed (ex.: "Sony Playstation" × "Sony PlayStation"):
  fundir sem diferenciar maiúsculas.
- Excluir uma plataforma do seed: permitir ou só ocultar.

## Fase 10 — Nova interface (pontos 2, 3 e 4) — implementada (ajustes das fases 10.1 e 10.2 também; falta a conferência manual, UIX-049, AJU-029 e IMG-014)

**Entrega**
- Layout em três colunas, como o LaunchBox:
  - **Esquerda**: busca, "Todos", plataformas com ícone e contagem (agrupamento padrão), demais filtros (status,
    gênero, favoritos, etc.) abaixo.
  - **Centro**: grade de cartões, sempre com a capa (ou um placeholder padrão) e o título embaixo.
  - **Direita**: detalhes do jogo em foco (um clique): título, carrossel de imagens, **Jogar** e **Jogar sem contar
    tempo**, metadados, tempo jogado, sessões, editar, buscar metadados, excluir.
- A página do jogo deixa de existir; tudo o que ela tinha vai para o painel da direita. Duplo clique no cartão lança.
- Janelas modais em tamanho proporcional à tela (importar a maior delas), com rolagem interna.
- A janela principal abre maximizada (não é tela cheia nem Big Box).

**Áreas**: `frontend` quase todo (`App`, `FilterPanel`, `GameCard`, `LibraryGrid`, `GamePage` → painel),
`cmd/arcadia/main_windows.go` (`WindowStartState`). Sem mudança no agente nem no tracker.

**A decidir na spec**
- Tema: escuro próximo do LaunchBox (recomendado) com as cores em variáveis, para um segundo tema depois.
- Visualização em lista: manter ao lado da grade ou remover.
- Painel direito sem jogo em foco: recolhido ou com resumo da biblioteca.
- O Big Box fica como está nesta fase.

## Fase 11 — Importação v2 (pontos 1 e 7, e a janela de revisão)

> **Status:** concluída no código (conferência manual IMV-083 pendente). Spec em [phase-11-importacao-v2](specs/phase-11-importacao-v2/requirements.md) (51 requisitos,
> IMV-*; [ADR 0032](adr/0032-leitura-de-pasta-por-extensoes-fechadas-e-resultado-limitado.md)). Os itens "A decidir"
> abaixo foram resolvidos lá.

**Entrega**
- **Normalização de título**: remove marcações de ROM entre parênteses e colchetes (`(U)`, `(USA)`, `(Rev 1)`,
  `[!]`, `[b1]`, `(Disc 2)`), troca `_` e `.` por espaço, move o artigo final (`Legend of Zelda, The` →
  `The Legend of Zelda`). O nome do arquivo original não muda; só o título proposto.
- **Subpastas**: a fonte "Pasta" lê recursivamente. Arquivos de discos do mesmo jogo (`(Disc 1)`, `(Disc 2)`, ou
  `.cue` + `.bin`) viram um candidato só.
- **Janela de revisão antes de importar** (como o "Pronto para importar" do LaunchBox): tabela com caixa de seleção
  (todas marcadas por padrão), título editável, arquivo, extensão e disco; contador de jogos; "já existe" continua
  desmarcado e bloqueado (IMP-006).
- Substitui IMP-004 e IMP-010 da fase 6 (que hoje dizem "só o primeiro nível").
- **Campo "plataforma do arquivo"** (achado na conferência da fase 9): hoje vem pré-preenchido com "PC" e é texto
  livre. Passa a vir com "Windows" quando a fonte é arquivo e a ser um **select** com as plataformas existentes
  (`platform.list`), sem digitar nome novo na janela (plataformas novas ficam no diálogo Plataformas).

**Áreas**: `internal/importers` (scan de pasta, normalizador), `internal/agent` + `internal/ipc` (título editado na
confirmação), `frontend` (`ImportDialog`).

**A decidir na spec**
- Subpasta: título pelo nome do arquivo (padrão do LaunchBox) ou pelo nome da pasta (opção dele, útil quando cada
  jogo fica na própria pasta). O comportamento exato do LaunchBox é conferido na spec, com a instalação real.
- Extensões ignoradas no scan recursivo (`.txt`, `.nfo`, `.sav`, imagens) — hoje não há filtro nenhum.
- Limite de profundidade e de quantidade de arquivos.

## Fase 12 — Metadados unificados (pontos 6 e 8)

> **Status:** concluída no código (conferência manual MET-083 pendente). Spec em [phase-12-metadados-unificados](specs/phase-12-metadados-unificados/requirements.md)
> (57 requisitos, MET-*; [ADR 0033](adr/0033-busca-unificada-de-metadados-com-agregador-e-confirmacao-em-lote.md) e
> [0034](adr/0034-limite-de-imagens-conta-atuais-e-enfileiradas.md)). O ScreenScraper ficou fora; o SteamGridDB traz
> capa e herói (a API v2 não tem screenshots de gameplay); a janela de confirmação dos matches é separada da janela de
> revisão de títulos e abre depois da importação.

**Entrega**
- **Busca unificada**: o usuário só pede "Obter dados". O agente consulta todos os provedores com chave, junta os
  resultados que são o mesmo jogo, pontua por título normalizado + plataforma + ano e devolve uma lista única.
  O usuário não escolhe mais o provedor.
- **Arquivo avulso**: campo "Nome para busca" (preenchido com o nome do executável ou da pasta pai) e botão
  "Obter dados"; as opções aparecem para o usuário escolher.
- **Pasta (lote)**: o usuário informa a pasta e a plataforma, marca "Buscar metadados e imagens", e o sistema escolhe
  sozinho o melhor match de cada jogo por título + plataforma.
  **Pedido do usuário (2026-11-15):** a busca acontece **logo na importação**, depois da janela de revisão dos títulos
  da fase 11: o usuário confirma/edita os títulos, clica em importar, e a busca de metadados e imagens roda em seguida
  sobre os jogos recém-importados (a janela de confirmação dos matches vem depois dela). Os títulos revisados na fase 11
  são os usados na busca. Não é um passo à parte que o usuário precise lembrar de disparar depois.
- **Janela de confirmação dos matches**: uma linha por jogo com o que foi encontrado (capa pequena, título, ano,
  plataforma). Match abaixo do limiar vem marcado como "em dúvida", com as alternativas numa lista para trocar, ou
  "nenhum". Caixa de seleção por linha, todas marcadas por padrão.
- A busca roda em background com progresso e cancelamento; a importação não espera por ela (jogo entra, mídia chega
  depois, pela fila de MID-005).
- Substitui MID-002 e a escolha de provedor do `MetadataDialog`.
- **Mídia completa por provedor** (achados na conferência da fase 9):
  - IGDB hoje traz só screenshots; passa a trazer também a **capa** (campo `cover` do jogo, endpoint `/covers`).
  - SteamGridDB hoje traz só capas; passa a trazer também **screenshots de gameplay**.
  - **Limite de imagens por tipo** ao importar: screenshot de gameplay no máximo 10, capa no máximo 1, fanart no
    máximo 2. Vale para a união dos provedores (a busca unificada escolhe as melhores até o limite) e para o que já
    existe no jogo (decidir na spec se o limite conta as imagens atuais).

**Áreas**: `internal/scrapers` (agregador, pontuação, mapa de plataformas), `internal/agent` + `internal/ipc`,
`frontend` (`ImportDialog`, `MetadataDialog`).

**A decidir na spec**
- Incluir o ScreenScraper (adiado na fase 7): é o melhor para ROMs, mas exige conta e tem limite por usuário.
  Sem ele, o retro depende de IGDB + SteamGridDB.
- Limiar de "em dúvida" e limite de requisições por provedor num lote grande.

## Fase 13 — Modo console (ponto 9)

**Entrega**
- Atalho só de controle **Select + B** (segurado, como o Select + Y da pausa): encerra a sessão em curso matando a
  árvore de processos do jogo ou do emulador (Job Object). A sessão fecha normalmente, com o tempo contado até ali
  e um motivo de fim próprio.
- Depois de fechar, se o Big Box estava ativo, a janela volta ao primeiro plano, em tela cheia e com o foco no
  cartão do jogo que estava rodando.
- Vale também para sessão "sem contar tempo" e para processo adotado (RUL-002).

**Áreas**: `internal/input` (combinação nova), `internal/tracker` (encerrar a árvore — tracker-specialist),
`internal/agent` (evento para a UI), `frontend` (Big Box retoma o foco), migração se houver motivo de fim novo.

**Decidido pelo usuário (2026-11-15), antes da spec**
1. Tempo de segurar igual ao da pausa (1 s), **sem** som de confirmação.
2. Jogo lançado por URI (Steam/Epic): tenta fechar o processo que o tracker adotou, se houver; sem processo adotado,
   ignora com o aviso sonoro de recusa.
3. O atalho funciona fora do Big Box também.
4. Sem confirmação extra contra acidente: só o 1 s segurado.
5. Motivo de fim novo: `closed_by_hotkey`.
6. Com a sessão pausada, encerra normalmente e o tempo pausado não conta.
7. Só existe uma sessão por vez; não há caso de várias.

**Escopo ampliado (pedido do usuário, 2026-11-15): Big Box novo**
- Visual refeito, mais parecido com launchers de console e temas do LaunchBox. O tema é escolhido pelo usuário entre os
  mockups (Faixa, Mosaico, Roda, Vitrine) antes da spec.
- O Big Box fica só com: navegar por plataformas e pelos jogos de cada uma, ver imagens e vídeos, detalhes do jogo,
  tempo de jogo total, e navegação pelo gamepad. Saem dele estatísticas, configuração de emuladores, plataformas e
  provedores (continuam na janela normal).
- **Vídeo:** entra nesta fase. Procurar uma fonte que forneça vídeo ao menos por plataforma e, se possível, vídeos
  curtos dos principais jogos, que ocupem pouco espaço em disco.

**Decidido pelo usuário (2026-11-15), depois dos mockups**
- **Tema inicial: Faixa** (estilo PS5). O Big Box nasce com **temas selecionáveis**: a estrutura permite trocar de tema, e
  Mosaico e Roda ficam preparados para mais tarde (Roda com ressalvas, a detalhar). Vitrine fica de fora.
- **Fonte dos vídeos: ScreenScraper.** Teto de **15 MB por vídeo**.
- **Medição feita em 2026-11-15** (47 jogos, 11 plataformas, pela página pública do ScreenScraper): o vídeo
  `video-normalized` ficou entre 0,6 e 4,8 MB (média perto de 2 MB); o `video` original ficou entre 0,9 e 21,2 MB, com
  dois acima de 15 MB (God of War: Chains of Olympus, PSP, 21,2 MB; Aero the Acro-Bat, GBA, 15,1 MB). Exemplos: Super
  Mario World 2,0 MB, Ocarina of Time 1,7 MB, Final Fantasy VII 0,7 MB, God of War 2 2,4 MB (todos normalizados).
  Conclusão: baixar sempre o `video-normalized`; o teto de 15 MB fica como trava de segurança e nunca foi atingido por
  ele. Mil jogos com vídeo ocupam cerca de 2 GB.

**A decidir na spec**
- Conta e credenciais do ScreenScraper (conta do usuário e credenciais de desenvolvedor do Arcadia), cota diária e nomes
  exatos dos campos da API; se o ScreenScraper fornece vídeo por plataforma (não confirmado).
- Limite total de espaço dos vídeos na biblioteca.
- As ressalvas do tema Roda.
