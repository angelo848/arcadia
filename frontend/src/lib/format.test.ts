import { describe, expect, it } from 'vitest'
import { formatUptime } from './format'

// UI-003: uptime text format. Table mirrors requirements.md exactly.
const cases: ReadonlyArray<{ name: string; seconds: number; want: string }> = [
  { name: 'UI-003/0 → 0s', seconds: 0, want: '0s' },
  { name: 'UI-003/59 → 59s', seconds: 59, want: '59s' },
  { name: 'UI-003/60 → 1m 00s', seconds: 60, want: '1m 00s' },
  { name: 'UI-003/3599 → 59m 59s', seconds: 3599, want: '59m 59s' },
  { name: 'UI-003/3600 → 1h 00m 00s', seconds: 3600, want: '1h 00m 00s' },
  { name: 'UI-003/3723 → 1h 02m 03s', seconds: 3723, want: '1h 02m 03s' },
]

describe('formatUptime', () => {
  for (const tc of cases) {
    it(tc.name, () => {
      expect(formatUptime(tc.seconds)).toBe(tc.want)
    })
  }
})
