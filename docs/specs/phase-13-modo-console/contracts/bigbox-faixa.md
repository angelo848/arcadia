# Big Box — contrato do shell, da navegação e do tema Faixa

Requisitos: BBX-001..BBX-030, CON-022. Fonte da verdade para `frontend/src/bigbox/`.

## 1. Layout do Faixa (tela Navegar, 16:9; medidas relativas à janela)

```
┌───────────────────────────────────────────────────────────────────────────────┐
│ [Todos]  Nintendo 64  Super Nintendo ...            ← barra de abas (topo, 6vh)│
│ ▢ ▢ ▢ [▣] ▢ ▢ ▢ ▢ ...                                ← faixa de capas (18vh)    │
│                                                                               │
│ TÍTULO GRANDE (clamp 2.4rem..4rem)                                            │
│ Plataforma · 1996 · Ação                                                      │
│ Descrição curta (até 280 caracteres, 3 linhas)          ARTE / VÍDEO DE FUNDO  │
│ Tempo jogado 12 h 5 min · Última vez 10/11/2026 · Sessões 14   (tela cheia)   │
│ [ Jogar ]  [ Detalhes ]                                                       │
│                                                                               │
│ (aviso de erro, role=alert)                                                   │
│ A Jogar   X Detalhes   LB/RB Plataforma   Select+B Fechar o jogo   B Sair  [Y Som]│
└───────────────────────────────────────────────────────────────────────────────┘
```

- Fundo: arte em `object-fit: cover`, véu `--arc-bb-scrim` da esquerda (forte) para a direita (fraco) e do rodapé para cima.
- Faixa: capas em 2:3; a em foco escala 1,25 com contorno `--arc-bb-focus`; janela de no máximo 15 capas (foco − 7 .. foco + 7).
- Bloco de informações: coluna esquerda, até 40 % da largura. Marcação "Em execução"/"Pausado" ao lado do título e como selo na
  capa (BBX-017).
- Texto sempre: nomes de plataforma e de jogo nunca são logos no Faixa.

## 2. Mapa de entrada

Mapeamento padrão da Gamepad API (`navigator.getGamepads()`, primeiro controle conectado): 0 A, 1 B, 2 X, 3 Y, 4 LB, 5 RB,
8 Select/Back, 9 Start, 12..15 direcional; eixos 0/1 analógico esquerdo, 2/3 analógico direito. Borda de subida; zona morta 0,6.

| Entrada | Teclado | Navegar | Detalhes |
|---|---|---|---|
| ←/→ (direcional, analógico esq.) | setas | jogo anterior/seguinte (sem volta nas pontas) | item anterior/seguinte da galeria |
| ↑/↓ | setas | nada | foco entre Jogar, Jogar sem contar tempo e descrição |
| analógico direito ↕ | — | nada | rola a descrição |
| A | Enter | `LaunchGame` do jogo em foco | aciona o botão em foco |
| X | D | abre Detalhes | nada |
| LB / RB | PageUp / PageDown | aba anterior/seguinte (circular) | nada |
| Y | M | liga/desliga o som (com vídeo visível) | idem |
| B | Esc | sai do Big Box | volta a Navegar (foco no mesmo jogo) |
| Select segurado + B ou Y | — | **ignorado na UI** (combos do agente) | idem |
| Start | — | nada nesta fase | nada |
| clique / duplo clique na capa | — | foca / lança | — |

Tecla mantida (`repeat`) nunca relança nem reabre (UIX-046). Setas e analógico repetem ao segurar (400 ms, depois 120 ms).

## 3. Estados da navegação (reducer puro `bigBoxNav`)

Estado: `screen ∈ {browse, details}`, `tab` (índice; 0 = Todos), `focus` (índice na lista da aba), `detailsFocus ∈ {play,
playUntracked, description}`, `gallery` (índice), `suspended` (bool), `awaitingFocusAfterBlur` (bool), `sound` (bool, começa
`false`), `videoFailed` (id do jogo ou `null`).

| Estado | Ação | Próximo | Efeito pedido ao shell |
|---|---|---|---|
| qualquer, `suspended` | qualquer ação de entrada | (mesmo) | nada [BBX-018] |
| browse | `prev` / `next` | `focus ± 1` limitado a `[0, total − 1]`; `videoFailed = null` | pedir próxima página se `focus ≥ loaded − 10` [BBX-006, BBX-007] |
| browse | `tabPrev` / `tabNext` | `tab ∓/± 1` circular; `focus = 0` | trocar a consulta da lista [BBX-005] |
| browse | `accept` (com jogo) | browse | `LaunchGame(id)` [BBX-014] |
| browse | `details` (com jogo) | details; `detailsFocus = play`; `gallery = 0` | — [BBX-016] |
| browse | `back` | — | `exitBigBox()` e desmontar [BBX-002] |
| details | `prev` / `next` | `gallery ± 1` limitado | — |
| details | `up` / `down` | `detailsFocus` anterior/seguinte (play → playUntracked → description) | — |
| details | `accept` em play / playUntracked | details | `LaunchGame` / `LaunchGameUntracked` |
| details | `back` | browse (mesmo `focus`) | — |
| qualquer | `toggleSound` com vídeo visível | `sound = !sound` | — [BBX-013] |
| qualquer | `launched(id)` (sucesso) | `suspended = true` | — [BBX-018] |
| qualquer, `suspended` | `windowBlur` | `awaitingFocusAfterBlur = true` | — |
| qualquer, `suspended`, `awaitingFocusAfterBlur` | `windowFocus` | `suspended = false`, `awaitingFocusAfterBlur = false` | — |
| qualquer, `suspended` | `sessionEnded(id)` do jogo lançado | `suspended = false` | — |
| qualquer | `gameClosed(id)` | browse; `suspended = false`; `focus` = índice de `id` se carregado na aba, senão o mesmo | `BringToFront()`, `enterBigBox()` [CON-022] |
| browse | `listLoaded(items)` na entrada | `focus` = índice do jogo em foco da janela normal, se presente; senão 0 | — [BBX-027] |

## 4. Modelo de exibição

```ts
type BigBoxThemeId = 'faixa' | 'mosaico' | 'roda'
type BigBoxTab = { platformId: number | null; label: string } // null = Todos
type BigBoxGame = { id: number; title: string; coverUrl: string | null; session: 'running' | 'paused' | null }
type BigBoxFocused = {
  game: BigBoxGame
  platform: string; year: number | null; genre: string | null
  shortDescription: string; totalPlaySeconds: number; lastPlayedAt: string | null; playCount: number
  backgroundUrl: string | null; videoUrl: string | null
}
type BigBoxViewModel = {
  screen: 'browse' | 'details'
  tabs: BigBoxTab[]; tab: number
  games: BigBoxGame[]; total: number; focus: number   // games = janela de até 15 em torno do foco
  focused: BigBoxFocused | null
  videoVisible: boolean; sound: boolean
  hints: Array<{ button: string; label: string }>
  alert: string | null
}
```

## 5. Contrato de tema

```ts
type BigBoxThemeProps = {
  model: BigBoxViewModel
  details: JSX.Element            // BigBoxDetails comum, mostrado quando screen = 'details'
  onPick: (index: number) => void  // clique numa capa
  onPlay: (index: number) => void  // duplo clique / botão Jogar
  onDetails: () => void
}
type BigBoxTheme = {
  id: BigBoxThemeId
  name: string                     // "Faixa", "Mosaico", "Roda"
  available: boolean               // só faixa nesta fase
  axis: 'horizontal' | 'vertical'
  Component?: (p: BigBoxThemeProps) => JSX.Element
}
```

O tema não lê bindings, não assina eventos e não guarda estado de navegação; só desenha o modelo e repassa cliques.

## 6. Tokens do Faixa (`index.css`, `[data-bigbox-theme="faixa"]`)

| Token | Uso |
|---|---|
| `--arc-bb-bg` | fundo sem arte (base do gradiente) |
| `--arc-bb-bg-2` | segunda cor do gradiente sem arte |
| `--arc-bb-scrim` | véu sobre a arte (cor com alfa; ≥ 0,72 de opacidade na zona do texto) |
| `--arc-bb-text` | título, linhas de informação, abas ativas |
| `--arc-bb-text-muted` | descrição curta, abas inativas, rodapé |
| `--arc-bb-accent` | botão Jogar (fundo) |
| `--arc-bb-on-accent` | texto do botão Jogar |
| `--arc-bb-surface` | botão Detalhes, selo de sessão, faixa de aviso |
| `--arc-bb-focus` | contorno de foco (capa, botões) |
| `--arc-bb-danger` | texto do aviso de erro |

Pares conferidos por BBX-026 (texto sobre `scrim` composto sobre `#fff` e sobre `#000`, ≥ 4,5:1): `text`, `text-muted`,
`danger`; `on-accent` sobre `accent` (≥ 4,5:1); `text` sobre `surface` (≥ 4,5:1); `focus` contra `scrim` composto (≥ 3:1).

## 7. Tempos e limites

| Constante | Valor | Requisito |
|---|---|---|
| `VIDEO_DELAY_MS` | 2000 | BBX-011 |
| `VIDEO_FADE_MS` | 400 | BBX-011 |
| `ART_FADE_MS` | 300 | BBX-010 |
| `STRIP_WINDOW` | 7 de cada lado (15 no DOM) | BBX-006 |
| `PREFETCH_BEFORE_END` | 10 | BBX-007 |
| `PAGE_SIZE` | 200 | BBX-007 |
| `SHORT_DESCRIPTION_MAX` | 280 | BBX-009 |
| `REPEAT_DELAY_MS` / `REPEAT_EVERY_MS` | 400 / 120 | §2 |
| `STICK_DEAD_ZONE` | 0,6 | §2 |
