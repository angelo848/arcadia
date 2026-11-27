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
              const all = [
                ...hintsFor(screen, { empty, videoVisible, detailsFocus, hasGallery }, 'Plataforma'),
                ...hintsFor(screen, { empty, videoVisible, detailsFocus, hasGallery }, 'Aba'),
              ]
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

  // BBX-054 (fase 15): terceiro argumento tabsLabel; sem ele vale "Plataforma" (as chamadas de duas argumentos acima).
  it('BBX-054/hintsFor: LB/RB "Aba" com collections e "Plataforma" com platforms', () => {
    const o = { empty: false, videoVisible: false }
    expect(pairs(hintsFor('browse', o, 'Aba'))).toEqual(['A Jogar', 'X Detalhes', 'LB/RB Aba', 'Select+B Fechar o jogo', 'B Sair'])
    expect(pairs(hintsFor('browse', o, 'Plataforma'))).toEqual([
      'A Jogar', 'X Detalhes', 'LB/RB Plataforma', 'Select+B Fechar o jogo', 'B Sair',
    ])
    expect(pairs(hintsFor('browse', o))).toContain('LB/RB Plataforma')
  })

  it('BBX-054/lista vazia com "Aba": só "LB/RB Aba" e "B Sair"; Detalhes não muda', () => {
    expect(pairs(hintsFor('browse', { empty: true, videoVisible: false }, 'Aba'))).toEqual(['LB/RB Aba', 'B Sair'])
    const d = { empty: false, videoVisible: false, detailsFocus: 'play' as const, hasGallery: true }
    expect(pairs(hintsFor('details', d, 'Aba'))).toEqual(pairs(hintsFor('details', d, 'Plataforma')))
  })

  it('BBX-021/lista vazia: só "LB/RB Plataforma" e "B Sair"', () => {
    expect(pairs(hintsFor('browse', { empty: true, videoVisible: false }))).toEqual(['LB/RB Plataforma', 'B Sair'])
  })

  // fase 17 (T05): opções layout e zone; contrato bigbox-tema-desktop §8.
  const col = { layout: 'columns' as const }
  it('TDK-024/Jogos com jogos', () => {
    const o = { empty: false, videoVisible: false, ...col, zone: 'games' as const }
    expect(pairs(hintsFor('browse', o))).toEqual(['A Jogar', 'X Detalhes', 'LB/RB Plataforma', 'Select+B Fechar o jogo', 'B Sair'])
    expect(pairs(hintsFor('browse', { ...o, videoVisible: true }))).toEqual([
      'A Jogar', 'X Detalhes', 'LB/RB Plataforma', 'Select+B Fechar o jogo', 'B Sair', 'Y Som',
    ])
  })

  it('TDK-024/Jogos aba vazia', () => {
    expect(pairs(hintsFor('browse', { empty: true, videoVisible: false, ...col, zone: 'games' }))).toEqual([
      'LB/RB Plataforma', '← Plataformas', 'B Sair',
    ])
  })

  it('TDK-024/Plataformas', () => {
    expect(pairs(hintsFor('browse', { empty: false, videoVisible: false, ...col, zone: 'platforms' }))).toEqual([
      '↑/↓ Plataforma', 'A Ver jogos', 'LB/RB Plataforma', 'B Voltar', 'Select+B Fechar o jogo',
    ])
  })

  it('TDK-024/Painel com foco em Jogar e em Jogar sem contar tempo', () => {
    const o = { empty: false, videoVisible: false, hasGallery: true, ...col }
    expect(pairs(hintsFor('details', { ...o, detailsFocus: 'play' }))).toEqual([
      'A Jogar', '↑/↓ Mover', 'B Voltar', 'Select+B Fechar o jogo',
    ])
    expect(pairs(hintsFor('details', { ...o, detailsFocus: 'playUntracked' }))).toEqual([
      'A Jogar sem contar tempo', '↑/↓ Mover', 'B Voltar', 'Select+B Fechar o jogo',
    ])
  })

  it('TDK-024/Painel no visor', () => {
    expect(pairs(hintsFor('details', { empty: false, videoVisible: false, hasGallery: true, detailsFocus: 'gallery', ...col }))).toEqual([
      '←/→ Imagem', '↓ Botões', 'B Voltar', 'Select+B Fechar o jogo',
    ])
  })

  it('TDK-024/nenhuma dica de Select+A com layout columns', () => {
    for (const screen of ['browse', 'details'] as const) {
      for (const zone of ['games', 'platforms'] as const) {
        for (const empty of [false, true]) {
          for (const detailsFocus of [undefined, 'play', 'playUntracked', 'gallery'] as const) {
            for (const h of hintsFor(screen, { empty, videoVisible: true, detailsFocus, hasGallery: true, layout: 'columns', zone })) {
              expect(`${h.button} ${h.label}`).not.toMatch(/Select\s*\+\s*A\b/)
            }
          }
        }
      }
    }
  })

  it('TDK-024/layout screens devolve exatamente o de hoje (↑ Botões no visor)', () => {
    const g = { empty: false, videoVisible: false, hasGallery: true, detailsFocus: 'gallery' as const }
    expect(pairs(hintsFor('details', { ...g, layout: 'screens', zone: 'games' }))).toEqual(pairs(hintsFor('details', g)))
    expect(pairs(hintsFor('details', { ...g, layout: 'screens' }))).toContain('↑ Botões')
    const b = { empty: false, videoVisible: false }
    expect(pairs(hintsFor('browse', { ...b, layout: 'screens', zone: 'games' }))).toEqual(pairs(hintsFor('browse', b)))
  })
})
