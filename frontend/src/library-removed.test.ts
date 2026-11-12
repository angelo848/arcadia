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
