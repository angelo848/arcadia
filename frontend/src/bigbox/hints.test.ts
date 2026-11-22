import { describe, expect, it } from 'vitest'
import { hintsFor } from './hints'

const pairs = (h: Array<{ button: string; label: string }>) => h.map((x) => `${x.button} ${x.label}`)

describe('hintsFor', () => {
  it('ATB-013/hintsFor em nenhuma tela nem combinação contém "Select+A"', () => {
    for (const screen of ['browse', 'details'] as const) {
      for (const empty of [false, true]) {
        for (const videoVisible of [false, true]) {
          for (const detailsFocus of [undefined, 'play', 'playUntracked', 'gallery'] as const) {
            for (const hasGallery of [false, true]) {
              const all = hintsFor(screen, { empty, videoVisible, detailsFocus, hasGallery })
              for (const h of all) {
                expect(`${h.button} ${h.label}`).not.toMatch(/Select\s*\+\s*A\b/)
              }
            }
          }
        }
      }
    }
  })

  it('BBX-015/dicas de Navegar: A Jogar, X Detalhes, LB/RB Plataforma, Select+B Fechar o jogo, B Sair', () => {
    expect(pairs(hintsFor('browse', { empty: false, videoVisible: false }))).toEqual([
      'A Jogar',
      'X Detalhes',
      'LB/RB Plataforma',
      'Select+B Fechar o jogo',
      'B Sair',
    ])
  })

  it('BBX-015/details com foco em botão: A com o rótulo do botão, ↓ Imagens só com galeria, B Voltar, Select+B Fechar o jogo', () => {
    const p = pairs(hintsFor('details', { empty: false, videoVisible: false, detailsFocus: 'play', hasGallery: true }))
    expect(p).toEqual(['A Jogar', '↓ Imagens', 'B Voltar', 'Select+B Fechar o jogo'])
    const q = pairs(hintsFor('details', { empty: false, videoVisible: false, detailsFocus: 'playUntracked', hasGallery: false }))
    expect(q).toEqual(['A Jogar sem contar tempo', 'B Voltar', 'Select+B Fechar o jogo'])
  })

  it('BBX-015/details com foco na galeria: ←/→ Imagem, ↑ Botões, B Voltar, Select+B Fechar o jogo, sem A', () => {
    const p = pairs(hintsFor('details', { empty: false, videoVisible: false, detailsFocus: 'gallery', hasGallery: true }))
    expect(p).toEqual(['←/→ Imagem', '↑ Botões', 'B Voltar', 'Select+B Fechar o jogo'])
    expect(p.some((x) => x.startsWith('A '))).toBe(false)
  })

  it('BBX-015/nenhuma dica "Rolar"', () => {
    for (const detailsFocus of ['play', 'playUntracked', 'gallery'] as const) {
      const p = pairs(hintsFor('details', { empty: false, videoVisible: true, detailsFocus, hasGallery: true }))
      expect(p.join('|')).not.toContain('Rolar')
    }
  })

  it('BBX-015/details nunca mostra "Y Som", mesmo com videoVisible', () => {
    for (const detailsFocus of ['play', 'playUntracked', 'gallery'] as const) {
      expect(pairs(hintsFor('details', { empty: false, videoVisible: true, detailsFocus, hasGallery: true }))).not.toContain('Y Som')
    }
  })

  it('BBX-013/"Y Som" só com vídeo visível', () => {
    expect(pairs(hintsFor('browse', { empty: false, videoVisible: true }))).toContain('Y Som')
    expect(pairs(hintsFor('browse', { empty: false, videoVisible: false }))).not.toContain('Y Som')
  })

  it('BBX-021/lista vazia: só "LB/RB Plataforma" e "B Sair"', () => {
    expect(pairs(hintsFor('browse', { empty: true, videoVisible: false }))).toEqual(['LB/RB Plataforma', 'B Sair'])
  })
})
