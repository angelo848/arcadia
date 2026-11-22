// Footer hints of the Big Box [BBX-013, BBX-015, BBX-021].

import type { DetailsFocus } from './nav'

export type Hint = { button: string; label: string }

export function hintsFor(
  screen: 'browse' | 'details',
  o: { empty: boolean; videoVisible: boolean; detailsFocus?: DetailsFocus; hasGallery?: boolean },
  tabsLabel: 'Plataforma' | 'Aba' = 'Plataforma',
): Hint[] {
  const close: Hint = { button: 'Select+B', label: 'Fechar o jogo' }
  const sound: Hint[] = o.videoVisible ? [{ button: 'Y', label: 'Som' }] : []
  if (screen === 'details') {
    const back: Hint = { button: 'B', label: 'Voltar' }
    if (o.detailsFocus === 'gallery') {
      return [{ button: '←/→', label: 'Imagem' }, { button: '↑', label: 'Botões' }, back, close]
    }
    const play = o.detailsFocus === 'playUntracked' ? 'Jogar sem contar tempo' : 'Jogar'
    const images: Hint[] = o.hasGallery ? [{ button: '↓', label: 'Imagens' }] : []
    return [{ button: 'A', label: play }, ...images, back, close]
  }
  if (o.empty) {
    return [{ button: 'LB/RB', label: tabsLabel }, { button: 'B', label: 'Sair' }]
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
