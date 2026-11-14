import { describe, expect, it } from 'vitest'

// Scans every production .tsx of the frontend (UIX-006, UIX-040, UIX-041).
// Legitimate exceptions: none. (cover-placeholder.svg is not a .tsx and is not scanned.)
const all = import.meta.glob('./**/*.tsx', { query: '?raw', import: 'default', eager: true }) as Record<string, string>
const files = Object.entries(all).filter(([path]) => !path.endsWith('.test.tsx'))

// Design §3: Tailwind palette utilities are forbidden; only theme tokens are allowed.
const palette =
  /\b(bg|text|border|ring|outline|from|via|to|fill|stroke|divide|placeholder|accent|caret|decoration|shadow)-(slate|gray|zinc|neutral|stone|red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose|black|white)(-\d+)?\b/g
const literalColor = /#[0-9a-fA-F]{3,8}\b|\brgba?\(|\bhsla?\(/g

function lineOf(src: string, index: number): number {
  return src.slice(0, index).split('\n').length
}

// scan returns "file:line: match" for each hit of the pattern.
function scan(pattern: RegExp): string[] {
  const hits: string[] = []
  for (const [path, src] of files) {
    for (const m of src.matchAll(new RegExp(pattern.source, pattern.flags))) {
      hits.push(`${path}:${lineOf(src, m.index ?? 0)}: ${m[0]}`)
    }
  }
  return hits
}

// Each className="..." / className={`...`} / className='...' literal, with its start index.
function classNames(src: string): { text: string; index: number }[] {
  const out: { text: string; index: number }[] = []
  const re = /className\s*=\s*(?:"([^"]*)"|'([^']*)'|\{`([^`]*)`\}|\{\s*"([^"]*)"\s*\}|\{\s*'([^']*)'\s*\})/g
  for (const m of src.matchAll(re)) {
    out.push({ text: m[1] ?? m[2] ?? m[3] ?? m[4] ?? m[5] ?? '', index: m.index ?? 0 })
  }
  return out
}

describe('varredura do tema', () => {
  it('sanidade: a varredura encontra os .tsx de produção', () => {
    expect(files.length).toBeGreaterThan(10)
    expect(files.some(([p]) => p.endsWith('/App.tsx'))).toBe(true)
    expect(files.some(([p]) => p.endsWith('.test.tsx'))).toBe(false)
  })

  it('AJU-027/MediaThumbStrip.tsx e os .tsx alterados na fase 10.1 estão no conjunto varrido', () => {
    const changed = [
      'App',
      'CoverImage',
      'GameCard',
      'GameDetailsPanel',
      'ImportDialog',
      'LibraryGrid',
      'LibraryList',
      'MediaCarousel',
      'MediaThumbStrip',
      'PanelEmpty',
    ]
    const missing = changed.filter((n) => !files.some(([p]) => p.endsWith(`/${n}.tsx`)))
    expect(missing, `.tsx fora da varredura: ${missing.join(', ')}`).toEqual([])
  })

  it('UIX-006/nenhum .tsx usa classe de paleta do Tailwind (padrão de design §3)', () => {
    const hits = scan(palette)
    expect(hits, `classes de paleta:\n${hits.join('\n')}`).toEqual([])
  })

  it('UIX-006/nenhum .tsx usa cor literal (#hex, rgb(, hsl() em className ou style', () => {
    const hits: string[] = []
    for (const [path, src] of files) {
      const regions: { text: string; index: number }[] = [...classNames(src)]
      for (const m of src.matchAll(/style\s*=\s*\{\{[\s\S]*?\}\}/g)) {
        regions.push({ text: m[0], index: m.index ?? 0 })
      }
      for (const r of regions) {
        for (const c of r.text.matchAll(literalColor)) {
          hits.push(`${path}:${lineOf(src, r.index)}: ${c[0]}`)
        }
      }
    }
    expect(hits, `cores literais:\n${hits.join('\n')}`).toEqual([])
  })

  it('UIX-040/nenhum className com outline-none sem focus-visible: correspondente', () => {
    const hits: string[] = []
    for (const [path, src] of files) {
      // Any string literal (className attributes and class constants alike).
      for (const m of src.matchAll(/"([^"\n]*)"|'([^'\n]*)'|`([^`]*)`/g)) {
        const text = m[1] ?? m[2] ?? m[3] ?? ''
        if (/(^|[\s:])outline-none(\s|$)/.test(text) && !/focus-visible:/.test(text)) {
          hits.push(`${path}:${lineOf(src, m.index ?? 0)}: ${text.trim()}`)
        }
      }
    }
    expect(hits, `outline-none sem focus-visible:\n${hits.join('\n')}`).toEqual([])
  })

  it('UIX-041/nenhum tabIndex maior que 0', () => {
    const hits = scan(/tabIndex\s*=\s*\{?\s*["']?[1-9]\d*/g)
    expect(hits, `tabIndex positivo:\n${hits.join('\n')}`).toEqual([])
  })
})
