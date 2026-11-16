import { describe, expect, it } from 'vitest'

// Phase 13 T42 (BBX-003, BBX-025): the old Big Box left no trace outside src/bigbox/ and in index.css.
const sources = import.meta.glob(['./**/*.ts', './**/*.tsx'], { query: '?raw', import: 'default', eager: true }) as Record<string, string>
const css = import.meta.glob('./index.css', { query: '?raw', import: 'default', eager: true }) as Record<string, string>

const production = Object.entries(sources).filter(
  ([path]) => !/\.test\.tsx?$/.test(path) && !path.startsWith('./bigbox/'),
)

describe('varredura do Big Box antigo', () => {
  it('sanidade: a varredura encontra os arquivos de produção e o index.css', () => {
    expect(production.length).toBeGreaterThan(10)
    expect(production.some(([p]) => p === './App.tsx')).toBe(true)
    expect(Object.keys(css)).toEqual(['./index.css'])
  })

  it('BBX-003/nenhum arquivo de src fora de bigbox/ usa data-bigbox-item, data-bigbox-card ou data-bigbox-play (varredura)', () => {
    const hits: string[] = []
    for (const [path, src] of production) {
      src.split('\n').forEach((line, i) => {
        if (/data-bigbox-(item|card|play)\b/.test(line)) hits.push(`${path}:${i + 1}: ${line.trim()}`)
      })
    }
    expect(hits, `atributos do Big Box antigo:\n${hits.join('\n')}`).toEqual([])
  })

  it('BBX-025/index.css não tem mais a regra .big-box', () => {
    const src = css['./index.css']
    const hits = src.split('\n').flatMap((line, i) => (/\.big-box\b/.test(line) ? [`${i + 1}: ${line.trim()}`] : []))
    expect(hits, `regra .big-box:\n${hits.join('\n')}`).toEqual([])
  })
})
