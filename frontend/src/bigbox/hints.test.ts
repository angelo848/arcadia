import { describe, expect, it } from 'vitest'
import { hintsFor } from './hints'

const pairs = (h: Array<{ button: string; label: string }>) => h.map((x) => `${x.button} ${x.label}`)

describe('hintsFor', () => {
  it('BBX-015/dicas de Navegar: A Jogar, X Detalhes, LB/RB Plataforma, Select+B Fechar o jogo, B Sair', () => {
    expect(pairs(hintsFor('browse', { empty: false, videoVisible: false }))).toEqual([
      'A Jogar',
      'X Detalhes',
      'LB/RB Plataforma',
      'Select+B Fechar o jogo',
      'B Sair',
    ])
  })

  it('BBX-015/dicas de Detalhes: A sobre o botão focado, B Voltar, Select+B Fechar o jogo', () => {
    const p = pairs(hintsFor('details', { empty: false, videoVisible: false, detailsFocus: 'play' }))
    expect(p).toHaveLength(3)
    expect(p[0]).toMatch(/^A /)
    expect(p.slice(1)).toEqual(['B Voltar', 'Select+B Fechar o jogo'])
    const q = pairs(hintsFor('details', { empty: false, videoVisible: false, detailsFocus: 'playUntracked' }))
    expect(q[0]).not.toBe(p[0])
  })

  it('BBX-013/"Y Som" só com vídeo visível', () => {
    expect(pairs(hintsFor('browse', { empty: false, videoVisible: true }))).toContain('Y Som')
    expect(pairs(hintsFor('browse', { empty: false, videoVisible: false }))).not.toContain('Y Som')
  })

  it('BBX-021/lista vazia: só "LB/RB Plataforma" e "B Sair"', () => {
    expect(pairs(hintsFor('browse', { empty: true, videoVisible: false }))).toEqual(['LB/RB Plataforma', 'B Sair'])
  })
})
