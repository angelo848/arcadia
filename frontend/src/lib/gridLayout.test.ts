import { describe, expect, it } from 'vitest'
import { DIVIDER_HEIGHT, buildLayout, lastPlayedGroup, moveInLayout } from './gridLayout'

const years = ['2026', '2026', '2026', '2026', '2026', '2025', '2025', 'Nunca jogado']

describe('gridLayout', () => {
  it('sem grupo: linhas cheias de cols cards', () => {
    const l = buildLayout([1, 2, 3, 4, 5], 2, 100)
    expect(l.rows).toEqual([
      { kind: 'cards', first: 0, count: 2 },
      { kind: 'cards', first: 2, count: 2 },
      { kind: 'cards', first: 4, count: 1 },
    ])
    expect(l.tops).toEqual([0, 100, 200])
    expect(l.height).toBe(300)
  })

  it('com grupo: uma divisória abre cada ano e o ano novo começa linha nova', () => {
    const l = buildLayout(years, 4, 100, (y) => y)
    expect(l.rows.map((r) => (r.kind === 'divider' ? r.label : `${r.first}+${r.count}`))).toEqual([
      '2026',
      '0+4',
      '4+1',
      '2025',
      '5+2',
      'Nunca jogado',
      '7+1',
    ])
    expect(l.tops[3]).toBe(DIVIDER_HEIGHT + 200)
    expect(l.rowOf).toEqual([1, 1, 1, 1, 2, 4, 4, 6])
  })

  it('setas seguem as linhas visuais, presas ao tamanho da linha', () => {
    const l = buildLayout(years, 4, 100, (y) => y)
    expect(moveInLayout(l, 3, 'ArrowDown')).toBe(4) // column 3 clamped to the single card of the next row
    expect(moveInLayout(l, 4, 'ArrowDown')).toBe(5) // skips the divider
    expect(moveInLayout(l, 6, 'ArrowUp')).toBe(4)
    expect(moveInLayout(l, 2, 'ArrowUp')).toBe(2) // first row
    expect(moveInLayout(l, 7, 'ArrowDown')).toBe(7) // last row
    expect(moveInLayout(l, 4, 'ArrowRight')).toBe(4)
    expect(moveInLayout(l, 5, 'ArrowLeft')).toBe(5)
    expect(moveInLayout(l, 5, 'End')).toBe(7)
    expect(moveInLayout(l, 5, 'Home')).toBe(0)
  })

  it('o grupo é o ano local da última vez jogado, ou "Nunca jogado"', () => {
    expect(lastPlayedGroup('')).toBe('Nunca jogado')
    expect(lastPlayedGroup('2025-06-15T12:00:00Z')).toBe('2025')
    expect(lastPlayedGroup('lixo')).toBe('Nunca jogado')
  })
})
