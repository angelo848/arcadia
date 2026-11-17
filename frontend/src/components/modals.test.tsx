import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ReactElement } from 'react'
import { fireEvent, render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { DeleteDialog } from './DeleteDialog'
import { EmulatorsDialog } from './EmulatorsDialog'
import { ImportDialog } from './ImportDialog'
import { MetadataDialog } from './MetadataDialog'
import { PlatformsDialog } from './PlatformsDialog'
import { ProviderKeysDialog } from './ProviderKeysDialog'
import type { ImportCandidate } from '../lib/imports'

const scanMock = vi.fn()
const applyMock = vi.fn()

vi.mock('../lib/imports', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../lib/imports')>()
  return {
    ...actual,
    scanImport: (...a: unknown[]) => scanMock(...a),
    applyImport: (...a: unknown[]) => applyMock(...a),
  }
})

vi.mock('../lib/emulators', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../lib/emulators')>()
  return { ...actual, useEmulators: () => ({ data: [] }) }
})

vi.mock('../lib/library', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../lib/library')>()
  return {
    ...actual,
    useCatalog: () => ({ data: { platforms: [], genres: [], companies: [], playModes: [] } }),
  }
})

vi.mock('../lib/metadata', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../lib/metadata')>()
  return { ...actual, providerKeyStatus: () => Promise.resolve({ configured: false }) }
})

function candidate(): ImportCandidate {
  return {
    source: 'steam',
    title: 'Counter-Strike 2',
    platform: 'PC',
    launchType: 'uri',
    launchTarget: 'steam://rungameid/730',
    installDir: '',
    developers: [],
    publishers: [],
    genres: [],
    year: null,
    description: '',
    playModes: [],
    playedSeconds: null,
    lastPlayed: null,
    exists: false,
  }
}

function wrap(node: ReactElement) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={client}>{node}</QueryClientProvider>)
}

type Row = {
  name: string
  title: string
  size: 'large' | 'medium' | 'compact'
  // element(close) builds the dialog; close is the handler that Esc must trigger once.
  element: (close: () => void) => ReactElement
}

const rows: Row[] = [
  {
    name: 'Importar',
    title: 'Importar jogos',
    size: 'large',
    element: (close) => <ImportDialog onImported={vi.fn()} onClose={close} />,
  },
  { name: 'Emuladores', title: 'Emuladores', size: 'medium', element: (close) => <EmulatorsDialog onClose={close} /> },
  { name: 'Plataformas', title: 'Plataformas', size: 'medium', element: (close) => <PlatformsDialog onClose={close} /> },
  {
    name: 'Buscar metadados',
    title: 'Buscar metadados e mídia',
    size: 'medium',
    element: (close) => <MetadataDialog gameId={3} onClose={close} />,
  },
  {
    name: 'Provedores',
    title: 'Chaves dos provedores',
    size: 'compact',
    element: (close) => <ProviderKeysDialog onClose={close} />,
  },
  {
    name: 'Remover jogo',
    title: 'Remover jogo',
    size: 'compact',
    element: (close) => (
      <DeleteDialog title="Celeste" pending={false} error={null} onConfirm={vi.fn()} onCancel={close} />
    ),
  },
]

describe('modais existentes sobre o Modal', () => {
  beforeEach(() => {
    scanMock.mockReset()
    applyMock.mockReset()
    // applyImportInLots reaches the binding through window, not through the mocked export.
    ;(window as unknown as { go?: unknown }).go = {
      main: { App: { ImportApply: (c: unknown, p: unknown) => applyMock(c, p) } },
    }
  })

  it('UIX-037/Importar é large; Emuladores, Plataformas e Buscar metadados são medium; Provedores e Remover jogo são compact', () => {
    for (const row of rows) {
      const { unmount } = wrap(row.element(vi.fn()))
      const dialog = screen.getByRole('dialog')
      expect(dialog, row.name).toHaveAttribute('data-size', row.size)
      expect(dialog, row.name).toHaveClass(`modal-${row.size}`)
      unmount()
    }
  })

  it('UIX-038/Importar: a tabela de revisão fica no corpo; o contêiner rolável é o pai direto, sem max-h fixo', async () => {
    scanMock.mockResolvedValue({ candidates: [candidate()], warnings: [] })
    wrap(<ImportDialog onImported={vi.fn()} onClose={vi.fn()} />)
    fireEvent.change(screen.getByLabelText('Fonte'), { target: { value: 'steam' } })
    fireEvent.click(screen.getByRole('button', { name: 'Buscar' }))
    await screen.findByRole('checkbox', { name: /Counter-Strike 2/ })
    const table = screen.getByRole('table')
    const scroller = table.parentElement as HTMLElement
    expect(scroller.className).not.toMatch(/max-h-/)
    expect(scroller.getAttribute('style') ?? '').not.toMatch(/max-height/)
    expect(scroller.contains(screen.getByRole('button', { name: 'Importar selecionados' }))).toBe(false)
  })

  it('UIX-039/cada diálogo tem role dialog, aria-modal e nome pelo título', () => {
    for (const row of rows) {
      const { unmount } = wrap(row.element(vi.fn()))
      const dialog = screen.getByRole('dialog', { name: row.title })
      expect(dialog, row.name).toHaveAttribute('aria-modal', 'true')
      unmount()
    }
  })

  it('UIX-039/Esc em cada diálogo faz o mesmo que fechar ou Cancelar', () => {
    for (const row of rows) {
      const close = vi.fn()
      const { unmount } = wrap(row.element(close))
      fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape' })
      expect(close, row.name).toHaveBeenCalledTimes(1)
      unmount()
    }
  })

  it('UIX-039/Remover jogo abre com o foco em Cancelar', () => {
    wrap(<DeleteDialog title="Celeste" pending={false} error={null} onConfirm={vi.fn()} onCancel={vi.fn()} />)
    expect(screen.getByRole('button', { name: 'Cancelar' })).toHaveFocus()
  })

  it('UIX-039/Importar em andamento: Esc não fecha', async () => {
    scanMock.mockResolvedValue({ candidates: [candidate()], warnings: [] })
    applyMock.mockReturnValue(new Promise(() => {}))
    const onClose = vi.fn()
    wrap(<ImportDialog onImported={vi.fn()} onClose={onClose} />)
    fireEvent.change(screen.getByLabelText('Fonte'), { target: { value: 'steam' } })
    fireEvent.click(screen.getByRole('button', { name: 'Buscar' }))
    // IMV-041: the row comes already checked, so no click on it.
    await screen.findByRole('checkbox', { name: /Counter-Strike 2/ })
    fireEvent.click(screen.getByRole('button', { name: 'Importar selecionados' }))
    expect(applyMock).toHaveBeenCalledTimes(1)

    fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape' })
    expect(onClose).not.toHaveBeenCalled()
  })
})
