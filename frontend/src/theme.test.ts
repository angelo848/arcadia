// Tests for the theme tokens and base CSS (UIX-005, UIX-006, UIX-007, UIX-037, UIX-040).
import { describe, expect, it } from 'vitest'
import css from './index.css?raw'
import html from '../index.html?raw'

// token (CSS variable) -> utility name, from design §3
const tokens: Record<string, string> = {
  '--arc-bg': 'app',
  '--arc-surface': 'surface',
  '--arc-surface-2': 'raised',
  '--arc-surface-3': 'hover',
  '--arc-border': 'line',
  '--arc-text': 'fg',
  '--arc-text-muted': 'muted',
  '--arc-accent': 'accent',
  '--arc-accent-hover': 'accent-hover',
  '--arc-on-accent': 'on-accent',
  '--arc-play': 'play',
  '--arc-play-hover': 'play-hover',
  '--arc-running': 'running',
  '--arc-paused': 'paused',
  '--arc-warning': 'warning',
  '--arc-danger': 'danger',
  '--arc-danger-strong': 'danger-strong',
  '--arc-focus': 'focus',
  '--arc-overlay': 'overlay',
  '--arc-placeholder': 'placeholder',
}

// Returns the body of the first block whose selector text matches `header` (braces balanced).
function block(source: string, header: RegExp): string {
  const m = header.exec(source)
  if (!m) return ''
  let i = source.indexOf('{', m.index + m[0].length - 1)
  if (i < 0) return ''
  const start = i + 1
  let depth = 0
  for (; i < source.length; i++) {
    if (source[i] === '{') depth++
    else if (source[i] === '}' && --depth === 0) return source.slice(start, i)
  }
  return ''
}

const darkBlock = block(css, /\[data-theme=["']dark["']\]\s*\{/)
const themeInline = block(css, /@theme\s+inline\s*\{/)

function tokenValue(name: string): string {
  const m = new RegExp(`${name}\\s*:\\s*([^;]+);`).exec(darkBlock)
  return m ? m[1].trim() : ''
}

function rgb(hex: string): [number, number, number] {
  let h = hex.replace('#', '')
  if (h.length === 3) h = [...h].map((c) => c + c).join('')
  return [0, 2, 4].map((i) => parseInt(h.slice(i, i + 2), 16)) as [number, number, number]
}

function luminance(hex: string): number {
  const [r, g, b] = rgb(hex).map((v) => {
    const c = v / 255
    return c <= 0.03928 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4)
  })
  return 0.2126 * r + 0.7152 * g + 0.0722 * b
}

function contrast(a: string, b: string): number {
  const la = luminance(a)
  const lb = luminance(b)
  return (Math.max(la, lb) + 0.05) / (Math.min(la, lb) + 0.05)
}

const byUtility = (u: string): string => {
  const name = Object.keys(tokens).find((k) => tokens[k] === u) as string
  return tokenValue(name)
}

function expectPairs(fgs: string[], bgs: string[], min: number) {
  for (const f of fgs) {
    for (const b of bgs) {
      const ratio = contrast(byUtility(f), byUtility(b))
      expect(ratio, `${f} sobre ${b}: ${ratio.toFixed(2)}`).toBeGreaterThanOrEqual(min)
    }
  }
}

describe('theme', () => {
  it('UIX-006/cada token da tabela tem valor sob [data-theme="dark"] e utilitário em @theme inline', () => {
    expect(darkBlock).not.toBe('')
    expect(themeInline).not.toBe('')
    for (const [name, util] of Object.entries(tokens)) {
      expect(tokenValue(name), `valor de ${name}`).not.toBe('')
      const re = new RegExp(`--color-${util}\\s*:\\s*var\\(${name}\\)\\s*;`)
      expect(re.test(themeInline), `--color-${util}: var(${name})`).toBe(true)
    }
  })

  it('UIX-006/index.html tem data-theme="dark" no elemento raiz', () => {
    const m = /<html\b[^>]*>/i.exec(html)
    expect(m).not.toBeNull()
    expect(m![0]).toMatch(/data-theme=["']dark["']/)
  })

  it('UIX-007/fg e muted ≥ 4,5:1 sobre app, surface, raised e hover', () => {
    expectPairs(['fg', 'muted'], ['app', 'surface', 'raised', 'hover'], 4.5)
  })

  it('UIX-007/on-accent ≥ 4,5:1 sobre accent, play e danger-strong', () => {
    expectPairs(['on-accent'], ['accent', 'play', 'danger-strong'], 4.5)
  })

  it('UIX-007/running, paused, warning e danger ≥ 4,5:1 sobre surface e raised', () => {
    expectPairs(['running', 'paused', 'warning', 'danger'], ['surface', 'raised'], 4.5)
  })

  it('UIX-007/focus ≥ 3:1 sobre app, surface e raised', () => {
    expectPairs(['focus'], ['app', 'surface', 'raised'], 3)
  })

  it('UIX-040/regra global :focus-visible usa var(--arc-focus)', () => {
    const body = block(css, /(^|[}\s]):focus-visible\s*\{/m)
    expect(body).toMatch(/outline\s*:\s*2px\s+solid\s+var\(--arc-focus\)/)
    expect(body).toMatch(/outline-offset\s*:\s*2px/)
  })

  it('UIX-005/--col-left e --col-right com os clamp da spec', () => {
    expect(css).toMatch(/--col-left\s*:\s*clamp\(\s*220px\s*,\s*16vw\s*,\s*300px\s*\)/)
    expect(css).toMatch(/--col-right\s*:\s*clamp\(\s*320px\s*,\s*26vw\s*,\s*480px\s*\)/)
  })

  it('UIX-037/.modal-large, .modal-medium e .modal-compact com largura e altura da tabela', () => {
    const norm = (s: string) => s.replace(/\s+/g, ' ')
    const large = norm(block(css, /\.modal-large\s*\{/))
    expect(large).toMatch(/width\s*:\s*min\(\s*92vw\s*,\s*1600px\s*\)/)
    expect(large).toMatch(/height\s*:\s*88vh/)
    const medium = norm(block(css, /\.modal-medium\s*\{/))
    expect(medium).toMatch(/width\s*:\s*min\(\s*75vw\s*,\s*1100px\s*\)/)
    expect(medium).toMatch(/height\s*:\s*80vh/)
    const compact = norm(block(css, /\.modal-compact\s*\{/))
    expect(compact).toMatch(/width\s*:\s*clamp\(\s*420px\s*,\s*40vw\s*,\s*560px\s*\)/)
    expect(compact).toMatch(/max-height\s*:\s*80vh/)
  })
})
