import { describe, expect, it } from 'vitest'

// UI-045: the summary components and their binding are gone from the frontend source.
// The sources are read as text with Vite (no Node types in this project). This file is
// excluded from the scan because it names the removed symbols.

const sources = import.meta.glob<string>(['./**/*.ts', './**/*.tsx'], {
  query: '?raw',
  import: 'default',
  eager: true,
})

const selfPath = './library-removed.test.ts'

const forbidden = ['LibraryEmpty', 'LibrarySummaryView', 'useLibrarySummary'] as const

// Components and tests that the grid replaces (tasks.md T34): they must not exist any more.
const removedFiles = [
  './components/LibraryEmpty.tsx',
  './components/LibraryEmpty.test.tsx',
  './components/GameList.tsx',
  './components/GameList.test.tsx',
] as const

describe('UI-045 componentes de resumo removidos', () => {
  it('UI-045/nenhum arquivo de frontend/src referencia LibraryEmpty, LibrarySummaryView ou useLibrarySummary', () => {
    const offenders: string[] = []
    for (const [path, text] of Object.entries(sources)) {
      if (path === selfPath) {
        continue
      }
      for (const name of forbidden) {
        if (text.includes(name)) {
          offenders.push(`${path} referencia ${name}`)
        }
      }
    }
    expect(offenders).toEqual([])
  })

  it('UI-045/os arquivos de LibraryEmpty e GameList e os respectivos testes não existem mais', () => {
    const present = removedFiles.filter((f) => f in sources)
    expect(present).toEqual([])
  })
})

// UIX-048: the game page is gone; the details panel replaced it (phase 10).
const forbiddenPage = ['GamePage', 'GameScreen'] as const
const removedPageFiles = ['./components/GamePage.tsx', './components/GamePage.test.tsx'] as const

describe('UIX-048 página do jogo removida', () => {
  it('UIX-048/nenhum arquivo de frontend/src referencia GamePage nem GameScreen', () => {
    const offenders: string[] = []
    for (const [path, text] of Object.entries(sources)) {
      if (path === selfPath) {
        continue
      }
      for (const name of forbiddenPage) {
        if (text.includes(name)) {
          offenders.push(`${path} referencia ${name}`)
        }
      }
    }
    expect(offenders).toEqual([])
  })

  it('UIX-048/GamePage.tsx e GamePage.test.tsx não existem', () => {
    const present = removedPageFiles.filter((f) => f in sources)
    expect(present).toEqual([])
  })

  it('UIX-048/não há botão "Voltar" fora de StatsPage nem botão "Detalhes"', () => {
    const offenders: string[] = []
    for (const [path, text] of Object.entries(sources)) {
      // Tests may name the buttons they look for; only the production sources are scanned.
      if (path === selfPath || /\.test\.tsx?$/.test(path)) {
        continue
      }
      // alterado em 2026-11-15 (fase 13): the new Big Box (src/bigbox/) legitimately has the "Detalhes"
      // button (BBX-009) and the "Voltar" hint (hints.ts); the ban holds everywhere else.
      if (path.startsWith('./bigbox/')) {
        continue
      }
      if (path !== './components/StatsPage.tsx' && /\bVoltar\b/.test(text)) {
        offenders.push(`${path} tem o botão Voltar`)
      }
      // A button labelled "Detalhes" is JSX text or an accessible name on its own; the
      // aria-label "Detalhes do jogo" of the panel is a different string.
      if (/>\s*Detalhes\s*</.test(text) || /^\s*Detalhes\s*$/m.test(text) || /['"`]Detalhes['"`]/.test(text)) {
        offenders.push(`${path} tem o botão Detalhes`)
      }
    }
    expect(offenders).toEqual([])
  })
})
