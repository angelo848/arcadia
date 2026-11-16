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

// ---- Phase 13 (BBX-025, BBX-026): tokens do Big Box, tema Faixa ----
const bbTokens: Record<string, string> = {
  '--arc-bb-bg': 'bb-bg',
  '--arc-bb-bg-2': 'bb-bg-2',
  '--arc-bb-scrim': 'bb-scrim',
  '--arc-bb-panel': 'bb-panel',
  '--arc-bb-text': 'bb-text',
  '--arc-bb-text-muted': 'bb-text-muted',
  '--arc-bb-accent': 'bb-accent',
  '--arc-bb-on-accent': 'bb-on-accent',
  '--arc-bb-surface': 'bb-surface',
  '--arc-bb-focus': 'bb-focus',
  '--arc-bb-danger': 'bb-danger',
}
const faixaBlock = block(css, /\[data-bigbox-theme=["']faixa["']\]\s*\{/)
const bb = (name: string): string => {
  const m = new RegExp(`${name}\\s*:\\s*([^;]+);`).exec(faixaBlock)
  return m ? m[1].trim() : ''
}

type RGBA = [number, number, number, number]
function parseColor(v: string): RGBA {
  const hex = /^#([0-9a-fA-F]{3,8})$/.exec(v)
  if (hex) {
    let h = hex[1]
    if (h.length <= 4) h = [...h].map((c) => c + c).join('')
    const n = [0, 2, 4].map((i) => parseInt(h.slice(i, i + 2), 16))
    const a = h.length === 8 ? parseInt(h.slice(6, 8), 16) / 255 : 1
    return [n[0], n[1], n[2], a]
  }
  const f = /^rgba?\(([^)]+)\)$/.exec(v)
  if (f) {
    const p = f[1].split(/[\s,/]+/).filter(Boolean).map((x) => (x.endsWith('%') ? parseFloat(x) / 100 : parseFloat(x)))
    return [p[0], p[1], p[2], p[3] ?? 1]
  }
  throw new Error(`cor não reconhecida: ${v}`)
}
const toHex = (c: number[]) => '#' + c.slice(0, 3).map((x) => Math.round(x).toString(16).padStart(2, '0')).join('')
function over(top: RGBA, base: string): string {
  const b = parseColor(base)
  return toHex([0, 1, 2].map((i) => top[i] * top[3] + b[i] * (1 - top[3])))
}
const solid = (name: string) => toHex(parseColor(bb(name)))

describe('theme: tokens do Big Box (fase 13)', () => {
  it('BBX-025/cada um dos tokens (bg, bg-2, scrim, panel, text, text-muted, accent, on-accent, surface, focus, danger) tem valor sob [data-bigbox-theme="faixa"] e utilitário em @theme inline', () => {
    expect(faixaBlock).not.toBe('')
    for (const [name, util] of Object.entries(bbTokens)) {
      expect(bb(name), `valor de ${name}`).not.toBe('')
      const re = new RegExp(`--color-${util}\\s*:\\s*var\\(${name}\\)\\s*;`)
      expect(re.test(themeInline), `--color-${util}: var(${name})`).toBe(true)
    }
  })

  it('BBX-026/text, text-muted e danger ≥ 4,5:1 sobre o panel composto sobre branco e sobre preto', () => {
    const panel = parseColor(bb('--arc-bb-panel'))
    for (const base of ['#ffffff', '#000000']) {
      const bg = over(panel, base)
      for (const t of ['--arc-bb-text', '--arc-bb-text-muted', '--arc-bb-danger']) {
        const r = contrast(solid(t), bg)
        expect(r, `${t} sobre panel/${base}: ${r.toFixed(2)}`).toBeGreaterThanOrEqual(4.5)
      }
    }
  })

  it('BBX-026/on-accent sobre accent ≥ 4,5:1', () => {
    expect(contrast(solid('--arc-bb-on-accent'), solid('--arc-bb-accent'))).toBeGreaterThanOrEqual(4.5)
  })

  it('BBX-026/text sobre surface ≥ 4,5:1', () => {
    expect(contrast(solid('--arc-bb-text'), solid('--arc-bb-surface'))).toBeGreaterThanOrEqual(4.5)
  })

  it('BBX-026/focus ≥ 3:1 contra o panel composto nos dois casos', () => {
    const panel = parseColor(bb('--arc-bb-panel'))
    for (const base of ['#ffffff', '#000000']) {
      const r = contrast(solid('--arc-bb-focus'), over(panel, base))
      expect(r, `focus sobre panel/${base}: ${r.toFixed(2)}`).toBeGreaterThanOrEqual(3)
    }
  })

  it('BBX-042/--arc-bb-safe-y é 5vh e --arc-bb-safe-x é 5vw no bloco do Faixa', () => {
    expect(bb('--arc-bb-safe-y')).toBe('5vh')
    expect(bb('--arc-bb-safe-x')).toBe('5vw')
  })
})

// ---- Phase 10.1 (AJU-004, AJU-005, AJU-008) ----
const plainCss = css.replace(/\/\*[\s\S]*?\*\//g, '')

// Innermost rules (selector list, body) of the stylesheet, comments stripped.
const rules: { selectors: string[]; body: string }[] = []
for (const m of plainCss.matchAll(/([^{}]+)\{([^{}]*)\}/g)) {
  rules.push({
    selectors: m[1].split(',').map((s) => s.replace(/\s+/g, ' ').trim()),
    body: m[2],
  })
}

// Bodies of every rule whose selector list has a selector satisfying `match`.
function bodiesFor(match: (sel: string) => boolean): string[] {
  return rules.filter((r) => r.selectors.some(match)).map((r) => r.body)
}

function bodyOf(selector: string): string {
  return bodiesFor((s) => s === selector).join(';')
}

describe('theme: scrollbar and cursor (fase 10.1)', () => {
  it('AJU-004/::-webkit-scrollbar com width e height 8px e ::-webkit-scrollbar-button com display none', () => {
    const bar = bodyOf('::-webkit-scrollbar')
    expect(bar).toMatch(/width\s*:\s*8px/)
    expect(bar).toMatch(/height\s*:\s*8px/)
    expect(bodyOf('::-webkit-scrollbar-button')).toMatch(/display\s*:\s*none/)
  })

  it('AJU-004/track e corner usam var(--arc-scrollbar-track); thumb usa var(--arc-scrollbar-thumb) com border-radius; :hover e :active usam var(--arc-scrollbar-thumb-hover)', () => {
    expect(bodyOf('::-webkit-scrollbar-track')).toMatch(/var\(--arc-scrollbar-track\)/)
    expect(bodyOf('::-webkit-scrollbar-corner')).toMatch(/var\(--arc-scrollbar-track\)/)
    const thumb = bodyOf('::-webkit-scrollbar-thumb')
    expect(thumb).toMatch(/var\(--arc-scrollbar-thumb\)/)
    expect(thumb).toMatch(/border-radius\s*:\s*4px/)
    expect(bodyOf('::-webkit-scrollbar-thumb:hover')).toMatch(/var\(--arc-scrollbar-thumb-hover\)/)
    expect(bodyOf('::-webkit-scrollbar-thumb:active')).toMatch(/var\(--arc-scrollbar-thumb-hover\)/)
  })

  it('AJU-004/index.css não usa scrollbar-width nem scrollbar-color', () => {
    expect(plainCss).not.toMatch(/scrollbar-width\s*:/)
    expect(plainCss).not.toMatch(/scrollbar-color\s*:/)
  })

  it('AJU-005/os três tokens existem no bloco do tema', () => {
    expect(tokenValue('--arc-scrollbar-track')).toBe('transparent')
    expect(tokenValue('--arc-scrollbar-thumb')).toBe('#3a404d')
    expect(tokenValue('--arc-scrollbar-thumb-hover')).toBe('#6b7587')
  })

  it('AJU-005/as regras de barra usam só var(--arc-*)', () => {
    const barRules = rules.filter((r) => r.selectors.some((s) => s.includes('::-webkit-scrollbar')))
    expect(barRules.length).toBeGreaterThan(0)
    for (const r of barRules) {
      for (const v of r.body.matchAll(/var\(([^)]*)\)/g)) {
        expect(v[1].trim()).toMatch(/^--arc-/)
      }
      expect(r.body).not.toMatch(/#[0-9a-fA-F]{3,8}\b|rgba?\(|hsla?\(/)
    }
  })

  it('AJU-005/fora do bloco do tema não há #hex, rgb( nem hsl(', () => {
    const outside = plainCss.replace(darkBlock, '')
    expect(outside).not.toMatch(/#[0-9a-fA-F]{3,8}\b/)
    expect(outside).not.toMatch(/\brgba?\(/)
    expect(outside).not.toMatch(/\bhsla?\(/)
  })

  it('AJU-005/thumb-hover tem contraste ≥ 3:1 com --arc-surface', () => {
    const ratio = contrast(tokenValue('--arc-scrollbar-thumb-hover'), byUtility('surface'))
    expect(ratio).toBeGreaterThanOrEqual(3)
  })

  it('AJU-008/button:not(:disabled), [role="button"], [role="option"], [role="tab"], a[href], summary, select, checkbox, radio e label[for] com cursor: pointer', () => {
    const pointer = (match: (s: string) => boolean) =>
      bodiesFor(match).some((b) => /cursor\s*:\s*pointer/.test(b))
    const wanted: [string, (s: string) => boolean][] = [
      ['button:not(:disabled)', (s) => s.includes('button:not(:disabled)')],
      ['[role="button"]', (s) => /\[role=["']button["']\]/.test(s)],
      ['[role="option"]', (s) => /\[role=["']option["']\]/.test(s)],
      ['[role="tab"]', (s) => /\[role=["']tab["']\]/.test(s)],
      ['a[href]', (s) => s.includes('a[href]')],
      ['summary', (s) => /(^|\s)summary\b/.test(s)],
      ['select', (s) => /(^|\s)select\b/.test(s) && !s.includes(':disabled')],
      ['checkbox', (s) => s.includes('checkbox') && !s.includes(':disabled')],
      ['radio', (s) => s.includes('radio') && !s.includes(':disabled')],
      ['label[for]', (s) => s.includes('label[for]')],
    ]
    for (const [name, match] of wanted) {
      expect(pointer(match), `cursor: pointer em ${name}`).toBe(true)
    }
  })

  it('AJU-008/button:disabled, select:disabled, input:disabled e [aria-disabled="true"] com cursor: not-allowed', () => {
    const notAllowed = (match: (s: string) => boolean) =>
      bodiesFor(match).some((b) => /cursor\s*:\s*not-allowed/.test(b))
    const wanted: [string, (s: string) => boolean][] = [
      ['button:disabled', (s) => s.includes('button:disabled')],
      ['select:disabled', (s) => s.includes('select:disabled')],
      ['input:disabled', (s) => s.includes('input:disabled')],
      ['[aria-disabled="true"]', (s) => /\[aria-disabled=["']true["']\]/.test(s)],
    ]
    for (const [name, match] of wanted) {
      expect(notAllowed(match), `cursor: not-allowed em ${name}`).toBe(true)
    }
  })
})
