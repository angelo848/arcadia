import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ReactNode } from 'react'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { PlatformsDialog } from './PlatformsDialog'
import { libraryGamesKey, libraryKeys } from '../lib/library'
import type { CatalogView, PlatformView } from '../lib/library'
import { platformIconSrc } from '../lib/platforms'

// The dialog is driven through the Wails bindings (window.go.main.App), like lib/library.test.tsx:
// a fake agent keeps the platform list and the test checks the calls it receives. Only the emulator
// lib is mocked (the default-emulator select reuses setPlatformEmulator from EMU-002).

type Fn = ReturnType<typeof vi.fn>
type FakeApp = {
  Catalog: Fn
  AddPlatform: Fn
  PlatformUpdate: Fn
  PlatformSetIcon: Fn
  PlatformDelete: Fn
  PickPlatformIcon: Fn
}

let app: FakeApp
let db: PlatformView[]
const setEmulatorMock = vi.fn()

const emulators = [
  { id: 1, name: 'Dolphin', exePath: 'C:\\Emu\\dolphin.exe', argsTemplate: '{rom}' },
  { id: 2, name: 'Ryujinx', exePath: 'C:\\Emu\\ryujinx.exe', argsTemplate: '{rom}' },
]

vi.mock('../lib/emulators', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../lib/emulators')>()
  return {
    ...actual,
    useEmulators: () => ({ data: emulators }),
    setPlatformEmulator: (...a: unknown[]) => setEmulatorMock(...a),
  }
})

// In the agent order (by name, case-insensitive).
function fixture(): PlatformView[] {
  return [
    { id: 4, name: 'Fliperama', category: 'arcade', emulatorId: null, seedKey: null, iconUrl: '', gameCount: 0 },
    { id: 1, name: 'Nintendo 64', category: 'console', emulatorId: null, seedKey: 'n64', iconUrl: '', gameCount: 0 },
    { id: 2, name: 'Nintendo Switch', category: 'console', emulatorId: 1, seedKey: 'switch', iconUrl: '', gameCount: 3 },
    {
      id: 3,
      name: 'Windows',
      category: 'pc',
      emulatorId: null,
      seedKey: 'windows',
      iconUrl: '/media/platforms/aa11.png',
      gameCount: 12,
    },
  ]
}

function sorted(list: PlatformView[]): PlatformView[] {
  return [...list].sort((a, b) => a.name.toLowerCase().localeCompare(b.name.toLowerCase()))
}

function catalogOf(list: PlatformView[]): CatalogView {
  return { platforms: sorted(list), genres: [], companies: [], playModes: [] }
}

function install() {
  db = fixture()
  app = {
    Catalog: vi.fn(() => Promise.resolve(catalogOf(db))),
    AddPlatform: vi.fn((name: string, category: string) => {
      const found = db.find((p) => p.name.toLowerCase() === name.trim().toLowerCase())
      if (found) {
        return Promise.resolve({ platform: found, created: false })
      }
      const created: PlatformView = {
        id: 100 + db.length,
        name: name.trim(),
        category,
        emulatorId: null,
        seedKey: null,
        iconUrl: '',
        gameCount: 0,
      }
      db = [...db, created]
      return Promise.resolve({ platform: created, created: true })
    }),
    PlatformUpdate: vi.fn((id: number, name: string, category: string) => {
      db = db.map((p) => (p.id === id ? { ...p, name, category } : p))
      return Promise.resolve(db.find((p) => p.id === id))
    }),
    PlatformSetIcon: vi.fn((id: number, path: string | null) => {
      db = db.map((p) => (p.id === id ? { ...p, iconUrl: path === null ? '' : '/media/platforms/new.png' } : p))
      return Promise.resolve(db.find((p) => p.id === id))
    }),
    PlatformDelete: vi.fn((id: number) => {
      db = db.filter((p) => p.id !== id)
      return Promise.resolve()
    }),
    PickPlatformIcon: vi.fn(() => Promise.resolve('D:\\Icones\\novo.png')),
  }
  ;(window as unknown as { go?: unknown }).go = { main: { App: app } }
  setEmulatorMock.mockReset()
  setEmulatorMock.mockImplementation((id: number, emulatorId: number | null) => {
    db = db.map((p) => (p.id === id ? { ...p, emulatorId } : p))
    return Promise.resolve(db.find((p) => p.id === id))
  })
}

beforeEach(install)

afterEach(() => {
  delete (window as unknown as { go?: unknown }).go
})

function renderDialog(onClose: () => void = vi.fn()) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const invalidate = vi.spyOn(client, 'invalidateQueries')
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  )
  render(<PlatformsDialog onClose={onClose} />, { wrapper })
  return { client, invalidate }
}

// rowOf finds the list item of a platform by its exact name; waits for the catalog to load.
async function rowOf(name: string): Promise<HTMLElement> {
  let row!: HTMLElement
  await waitFor(() => {
    const hit = screen.getAllByRole('listitem').filter((li) => within(li).queryByText(name) !== null)
    expect(hit, `row of ${name}`).toHaveLength(1)
    row = hit[0]
  })
  return row
}

async function openEdit(name: string): Promise<HTMLElement> {
  const row = await rowOf(name)
  fireEvent.click(within(row).getByRole('button', { name: 'Editar' }))
  return row
}

const blockedMessage = (n: number) =>
  `Esta plataforma tem ${n} jogo(s). Mova-os para outra plataforma ou exclua-os antes.`

describe('PlatformsDialog', () => {
  it('PLT-015/lista ícone, nome, categoria, nº de jogos e emulador em ordem de nome', async () => {
    renderDialog()
    await rowOf('Fliperama')

    const dialog = screen.getByRole('dialog', { name: 'Plataformas' })
    const rows = within(dialog).getAllByRole('listitem')
    expect(rows).toHaveLength(4)
    const names = ['Fliperama', 'Nintendo 64', 'Nintendo Switch', 'Windows']
    names.forEach((n, i) => expect(rows[i]).toHaveTextContent(n))

    const platforms = sorted(fixture())
    platforms.forEach((p, i) => {
      const img = rows[i].querySelector('img')
      expect(img, `ícone de ${p.name}`).not.toBeNull()
      expect(img?.getAttribute('src')).toBe(platformIconSrc(p))
    })

    expect(rows[0]).toHaveTextContent(/arcade/i)
    expect(rows[1]).toHaveTextContent(/console/i)
    expect(rows[3]).toHaveTextContent(/PC/)
    expect(rows[0]).toHaveTextContent('0 jogo(s)')
    expect(rows[2]).toHaveTextContent('3 jogo(s)')
    expect(rows[3]).toHaveTextContent('12 jogo(s)')

    expect(within(rows[2]).getByLabelText('Emulador padrão de Nintendo Switch')).toHaveValue('1')
    expect(within(rows[1]).getByLabelText('Emulador padrão de Nintendo 64')).toHaveValue('')
    const select = within(rows[1]).getByLabelText('Emulador padrão de Nintendo 64')
    expect(within(select).getByRole('option', { name: 'Nenhum' })).toBeInTheDocument()
    expect(within(select).getByRole('option', { name: 'Dolphin' })).toBeInTheDocument()
    expect(within(select).getByRole('option', { name: 'Ryujinx' })).toBeInTheDocument()
  })

  it('PLT-016/nova plataforma com console pré-selecionada aparece na lista', async () => {
    renderDialog()
    await rowOf('Fliperama')

    fireEvent.click(screen.getByRole('button', { name: 'Nova plataforma' }))
    expect(screen.getByLabelText('Categoria')).toHaveValue('console')
    fireEvent.change(screen.getByLabelText('Nome'), { target: { value: 'Dreamcast' } })
    fireEvent.click(screen.getByRole('button', { name: 'Salvar' }))

    await waitFor(() => expect(app.AddPlatform).toHaveBeenCalledWith('Dreamcast', 'console'))
    expect(await rowOf('Dreamcast')).toHaveTextContent(/console/i)
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('PLT-016/nome que já existe mostra "Já existe: <nome>." e não duplica a lista', async () => {
    renderDialog()
    await rowOf('Fliperama')

    fireEvent.click(screen.getByRole('button', { name: 'Nova plataforma' }))
    fireEvent.change(screen.getByLabelText('Nome'), { target: { value: 'nintendo switch' } })
    fireEvent.click(screen.getByRole('button', { name: 'Salvar' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('Já existe: Nintendo Switch.')
    expect(app.AddPlatform).toHaveBeenCalledWith('nintendo switch', 'console')
    expect(screen.getAllByRole('listitem')).toHaveLength(4)
  })

  it('PLT-018/editar o nome chama updatePlatform e recarrega o catálogo', async () => {
    renderDialog()
    await openEdit('Fliperama')
    const catalogCalls = app.Catalog.mock.calls.length

    expect(screen.getByLabelText('Nome')).toHaveValue('Fliperama')
    fireEvent.change(screen.getByLabelText('Nome'), { target: { value: 'Arcades' } })
    fireEvent.click(screen.getByRole('button', { name: 'Salvar' }))

    await waitFor(() => expect(app.PlatformUpdate).toHaveBeenCalledWith(4, 'Arcades', 'arcade'))
    expect(app.PlatformSetIcon).not.toHaveBeenCalled()
    await waitFor(() => expect(app.Catalog.mock.calls.length).toBeGreaterThan(catalogCalls))
    expect(await rowOf('Arcades')).toBeInTheDocument()
    expect(screen.queryByText('Fliperama')).not.toBeInTheDocument()
  })

  it('PLT-019/trocar a categoria chama updatePlatform', async () => {
    renderDialog()
    await openEdit('Fliperama')

    fireEvent.change(screen.getByLabelText('Categoria'), { target: { value: 'handheld' } })
    fireEvent.click(screen.getByRole('button', { name: 'Salvar' }))

    await waitFor(() => expect(app.PlatformUpdate).toHaveBeenCalledWith(4, 'Fliperama', 'handheld'))
  })

  it('PLT-018/erro do agente ao renomear aparece no alert', async () => {
    app.PlatformUpdate.mockRejectedValueOnce('Já existe uma plataforma com esse nome.')
    renderDialog()
    await openEdit('Fliperama')

    fireEvent.change(screen.getByLabelText('Nome'), { target: { value: 'Windows' } })
    fireEvent.click(screen.getByRole('button', { name: 'Salvar' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('Já existe uma plataforma com esse nome.')
  })

  it('PLT-020/excluir pede o segundo toque antes de chamar deletePlatform', async () => {
    renderDialog()
    const row = await rowOf('Fliperama')

    fireEvent.click(within(row).getByRole('button', { name: 'Excluir' }))
    expect(app.PlatformDelete).not.toHaveBeenCalled()

    fireEvent.click(within(row).getByRole('button', { name: 'Confirmar exclusão' }))
    await waitFor(() => expect(app.PlatformDelete).toHaveBeenCalledWith(4))
    await waitFor(() => expect(screen.queryByText('Fliperama')).not.toBeInTheDocument())
    expect(screen.getAllByRole('listitem')).toHaveLength(3)
  })

  it('PLT-021/com jogos o botão Excluir fica desativado com a explicação', async () => {
    renderDialog()
    const row = await rowOf('Nintendo Switch')

    const del = within(row).getByRole('button', { name: 'Excluir' })
    expect(del).toBeDisabled()
    expect(within(row).getByText(blockedMessage(3))).toBeInTheDocument()
    fireEvent.click(del)
    expect(app.PlatformDelete).not.toHaveBeenCalled()

    const windows = await rowOf('Windows')
    expect(within(windows).getByRole('button', { name: 'Excluir' })).toBeDisabled()
    expect(within(windows).getByText(blockedMessage(12))).toBeInTheDocument()

    // A platform without games does not show the explanation.
    const empty = await rowOf('Fliperama')
    expect(within(empty).getByRole('button', { name: 'Excluir' })).toBeEnabled()
    expect(within(empty).queryByText(/Mova-os para outra plataforma/)).not.toBeInTheDocument()
  })

  it('PLT-021/-32013 do agente na exclusão aparece no alert', async () => {
    app.PlatformDelete.mockRejectedValueOnce(blockedMessage(1))
    renderDialog()
    const row = await rowOf('Fliperama')

    fireEvent.click(within(row).getByRole('button', { name: 'Excluir' }))
    fireEvent.click(within(row).getByRole('button', { name: 'Confirmar exclusão' }))

    expect(await screen.findByRole('alert')).toHaveTextContent(blockedMessage(1))
  })

  it('PLT-022/escolher o emulador padrão chama setPlatformEmulator e mostra o novo valor', async () => {
    renderDialog()
    const row = await rowOf('Nintendo 64')

    fireEvent.change(within(row).getByLabelText('Emulador padrão de Nintendo 64'), { target: { value: '2' } })

    await waitFor(() => expect(setEmulatorMock).toHaveBeenCalledWith(1, 2))
    await waitFor(async () =>
      expect(within(await rowOf('Nintendo 64')).getByLabelText('Emulador padrão de Nintendo 64')).toHaveValue('2'),
    )
  })

  it('PLT-022/escolher Nenhum chama setPlatformEmulator com null', async () => {
    renderDialog()
    const row = await rowOf('Nintendo Switch')

    fireEvent.change(within(row).getByLabelText('Emulador padrão de Nintendo Switch'), { target: { value: '' } })

    await waitFor(() => expect(setEmulatorMock).toHaveBeenCalledWith(2, null))
    await waitFor(async () =>
      expect(within(await rowOf('Nintendo Switch')).getByLabelText('Emulador padrão de Nintendo Switch')).toHaveValue(''),
    )
  })

  it('PLT-023/Procurar... preenche o campo de caminho e o campo continua visível', async () => {
    renderDialog()
    await openEdit('Fliperama')

    const field = screen.getByLabelText('Caminho do ícone')
    expect(field).toBeVisible()
    expect(field).toHaveValue('')

    fireEvent.click(screen.getByRole('button', { name: 'Procurar...' }))
    await waitFor(() => expect(field).toHaveValue('D:\\Icones\\novo.png'))
    expect(app.PickPlatformIcon).toHaveBeenCalledTimes(1)
    expect(field).toBeVisible()
  })

  it('PLT-023/Procurar... cancelado não muda o caminho', async () => {
    app.PickPlatformIcon.mockResolvedValueOnce('')
    renderDialog()
    await openEdit('Fliperama')

    const field = screen.getByLabelText('Caminho do ícone')
    fireEvent.change(field, { target: { value: 'C:\\a.png' } })
    fireEvent.click(screen.getByRole('button', { name: 'Procurar...' }))

    await waitFor(() => expect(app.PickPlatformIcon).toHaveBeenCalledTimes(1))
    expect(field).toHaveValue('C:\\a.png')
  })

  it('PLT-011/salvar com um caminho de ícone chama setPlatformIcon com o caminho', async () => {
    renderDialog()
    await openEdit('Fliperama')

    fireEvent.change(screen.getByLabelText('Caminho do ícone'), { target: { value: 'D:\\Icones\\novo.png' } })
    fireEvent.click(screen.getByRole('button', { name: 'Salvar' }))

    await waitFor(() => expect(app.PlatformSetIcon).toHaveBeenCalledWith(4, 'D:\\Icones\\novo.png'))
  })

  it('PLT-012/motivo recusado pelo agente aparece no alert e o ícone não muda', async () => {
    app.PlatformSetIcon.mockRejectedValueOnce('O arquivo não é uma imagem PNG ou JPEG.')
    renderDialog()
    await openEdit('Fliperama')

    fireEvent.change(screen.getByLabelText('Caminho do ícone'), { target: { value: 'D:\\x.txt' } })
    fireEvent.click(screen.getByRole('button', { name: 'Salvar' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('O arquivo não é uma imagem PNG ou JPEG.')
  })

  it('PLT-013/Restaurar padrão chama setPlatformIcon com null', async () => {
    renderDialog()
    await openEdit('Windows')

    fireEvent.click(screen.getByRole('button', { name: 'Restaurar padrão' }))

    await waitFor(() => expect(app.PlatformSetIcon).toHaveBeenCalledWith(3, null))
  })

  it('PLT-024/depois de salvar invalida o catálogo (sem depender de notificação)', async () => {
    const { invalidate } = renderDialog()
    await openEdit('Fliperama')
    invalidate.mockClear()

    fireEvent.change(screen.getByLabelText('Nome'), { target: { value: 'Arcades' } })
    fireEvent.click(screen.getByRole('button', { name: 'Salvar' }))

    await waitFor(() =>
      expect(invalidate).toHaveBeenCalledWith(expect.objectContaining({ queryKey: libraryKeys.catalog })),
    )
    // The name shows in the game card and page, so the games lists are reloaded after a rename.
    expect(invalidate).toHaveBeenCalledWith(expect.objectContaining({ queryKey: libraryGamesKey }))
  })

  it('PLT-024/criar e excluir também invalidam o catálogo', async () => {
    const { invalidate } = renderDialog()
    const row = await rowOf('Fliperama')
    invalidate.mockClear()

    fireEvent.click(within(row).getByRole('button', { name: 'Excluir' }))
    fireEvent.click(within(row).getByRole('button', { name: 'Confirmar exclusão' }))
    await waitFor(() =>
      expect(invalidate).toHaveBeenCalledWith(expect.objectContaining({ queryKey: libraryKeys.catalog })),
    )

    invalidate.mockClear()
    fireEvent.click(screen.getByRole('button', { name: 'Nova plataforma' }))
    fireEvent.change(screen.getByLabelText('Nome'), { target: { value: 'Dreamcast' } })
    fireEvent.click(screen.getByRole('button', { name: 'Salvar' }))
    await waitFor(() =>
      expect(invalidate).toHaveBeenCalledWith(expect.objectContaining({ queryKey: libraryKeys.catalog })),
    )
  })

  it('PLT-015/Fechar chama onClose', async () => {
    const onClose = vi.fn()
    renderDialog(onClose)
    await rowOf('Fliperama')

    fireEvent.click(screen.getByRole('button', { name: 'Fechar' }))
    expect(onClose).toHaveBeenCalledTimes(1)
  })
})
