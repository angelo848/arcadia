// Footer hints of the Big Box [BBX-013, BBX-015, BBX-021].

export type Hint = { button: string; label: string }

export function hintsFor(
  screen: 'browse' | 'details',
  o: { empty: boolean; videoVisible: boolean; detailsFocus?: string },
): Hint[] {
  const close: Hint = { button: 'Select+B', label: 'Fechar o jogo' }
  const sound: Hint[] = o.videoVisible ? [{ button: 'Y', label: 'Som' }] : []
  if (screen === 'details') {
    const play = o.detailsFocus === 'playUntracked' ? 'Jogar sem contar tempo' : o.detailsFocus === 'description' ? 'Rolar' : 'Jogar'
    return [{ button: 'A', label: play }, { button: 'B', label: 'Voltar' }, close, ...sound]
  }
  if (o.empty) {
    return [{ button: 'LB/RB', label: 'Plataforma' }, { button: 'B', label: 'Sair' }]
  }
  return [
    { button: 'A', label: 'Jogar' },
    { button: 'X', label: 'Detalhes' },
    { button: 'LB/RB', label: 'Plataforma' },
    close,
    { button: 'B', label: 'Sair' },
    ...sound,
  ]
}
