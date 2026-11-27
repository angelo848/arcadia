// Footer hints of the Big Box [BBX-013, BBX-015, BBX-021, TDK-024].

import type { DetailsFocus, NavZone } from './nav'

export type Hint = { button: string; label: string }

type HintOptions = {
  empty: boolean
  videoVisible: boolean
  detailsFocus?: DetailsFocus
  hasGallery?: boolean
  layout?: 'screens' | 'columns'
  zone?: NavZone
}

export function hintsFor(screen: 'browse' | 'details', o: HintOptions, tabsLabel: 'Plataforma' | 'Aba' = 'Plataforma'): Hint[] {
  const close: Hint = { button: 'Select+B', label: 'Fechar o jogo' }
  const sound: Hint[] = o.videoVisible ? [{ button: 'Y', label: 'Som' }] : []
  const columns = o.layout === 'columns'
  if (screen === 'details') {
    const back: Hint = { button: 'B', label: 'Voltar' }
    if (o.detailsFocus === 'gallery') {
      return [{ button: '←/→', label: 'Imagem' }, { button: columns ? '↓' : '↑', label: 'Botões' }, back, close]
    }
    const play = o.detailsFocus === 'playUntracked' ? 'Jogar sem contar tempo' : 'Jogar'
    if (columns) return [{ button: 'A', label: play }, { button: '↑/↓', label: 'Mover' }, back, close]
    const images: Hint[] = o.hasGallery ? [{ button: '↓', label: 'Imagens' }] : []
    return [{ button: 'A', label: play }, ...images, back, close]
  }
  if (columns && o.zone === 'platforms') {
    return [
      { button: '↑/↓', label: 'Plataforma' },
      { button: 'A', label: 'Ver jogos' },
      { button: 'LB/RB', label: 'Plataforma' },
      { button: 'B', label: 'Voltar' },
      close,
    ]
  }
  if (o.empty) {
    const left: Hint[] = columns ? [{ button: '←', label: 'Plataformas' }] : []
    return [{ button: 'LB/RB', label: tabsLabel }, ...left, { button: 'B', label: 'Sair' }]
  }
  return [
    { button: 'A', label: 'Jogar' },
    { button: 'X', label: 'Detalhes' },
    { button: 'LB/RB', label: tabsLabel },
    close,
    { button: 'B', label: 'Sair' },
    ...sound,
  ]
}
