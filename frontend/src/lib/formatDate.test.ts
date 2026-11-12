import { describe, expect, it } from 'vitest'
import { formatLocalDateTime, formatUtcDate } from './formatDate'

// Expected values are built from the local (or UTC) parts of the same instant, so the
// tests hold in any time zone the suite runs in.
function pad(n: number): string {
  return String(n).padStart(2, '0')
}

function localExpected(iso: string): string {
  const d = new Date(iso)
  return `${pad(d.getDate())}/${pad(d.getMonth() + 1)}/${d.getFullYear()} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

describe('formatLocalDateTime', () => {
  it("UI-037/formatLocalDateTime vazio devolve 'Nunca'", () => {
    expect(formatLocalDateTime('')).toBe('Nunca')
  })

  it('UI-037/formatLocalDateTime mostra DD/MM/AAAA HH:MM na hora local', () => {
    const cases = [
      '2026-11-01T10:00:00.000Z',
      '2026-01-05T00:00:00.000Z',
      '2026-12-31T23:59:00.000Z',
    ]
    for (const iso of cases) {
      expect(formatLocalDateTime(iso), iso).toBe(localExpected(iso))
    }
  })

  it('UI-038/formatLocalDateTime é o formato de início e fim das sessões', () => {
    const iso = '2026-11-09T10:04:00.000Z'
    const out = formatLocalDateTime(iso)
    expect(out).toMatch(/^\d{2}\/\d{2}\/\d{4} \d{2}:\d{2}$/)
    expect(out).toBe(localExpected(iso))
  })
})

describe('formatUtcDate', () => {
  it('UI-037/formatUtcDate usa o dia em UTC, não o dia local', () => {
    // 23:30 at UTC-03:00 is 02:30 of the next day in UTC.
    const iso = '2026-11-12T23:30:00-03:00'
    expect(formatUtcDate(iso)).toBe('2026-11-13')
    expect(formatUtcDate('2026-11-12T00:00:00Z')).toBe('2026-11-12')
  })

  it('UI-037/formatUtcDate mostra YYYY-MM-DD com zeros à esquerda', () => {
    expect(formatUtcDate('2026-01-05T00:00:00Z')).toBe('2026-01-05')
    expect(formatUtcDate('2026-12-31T23:59:59Z')).toBe('2026-12-31')
  })
})
