import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ImportDialog } from './ImportDialog'
import type { ImportCandidate, ImportSource } from '../lib/imports'
import type { PlatformView } from '../lib/library'

// Phase 11 (importação v2): the review window of ImportDialog (IMV-040..IMV-063).
// Every case starts with the ID of the requirement it covers.

const mocks = vi.hoisted(() => ({
  scan: vi.fn(),
  apply: vi.fn(),
  pickFile: vi.fn(),
  pickFolder: vi.fn(),
  platforms: [] as PlatformView[],
}))

vi.mock('../lib/imports', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../lib/imports')>()
  return {
    ...actual,
    scanImport: (...args: unknown[]) => mocks.scan(...args),
    pickFile: () => mocks.pickFile(),
    pickFolder: () => mocks.pickFolder(),
  }
})

vi.mock('../lib/library', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../lib/library')>()
  return {
    ...actual,
    useCatalog: () => ({ data: { platforms: mocks.platforms, genres: [], companies: [], playModes: [] } }),
  }
})

function makePlatform(id: number, name: string, seedKey: string | null): PlatformView {
  return { id, name, category: 'pc', emulatorId: null, seedKey, iconUrl: '', gameCount: 0, }
}

// Catalog fixtures in name order (the dialog must still sort them, IMV-060).
const N64 = makePlatform(3, 'Nintendo 64', 'n64')
const SONY = makePlatform(2, 'Sony Playstation', 'ps1')
const WINDOWS = makePlatform(1, 'Windows', 'windows')

function steamCandidate(title: string, over: Partial<ImportCandidate> = {}): ImportCandidate {
  return {
    source: 'steam',
    title,
    platform: 'PC',
    launchType: 'uri',
    launchTarget: `steam://rungameid/${title}`,
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
    ...over,
  }
}

// fileCandidate is a candidate of the "Arquivo" source: title normalized by the agent, file and extension filled (IMV-024).
function fileCandidate(file: string, over: Partial<ImportCandidate> = {}): ImportCandidate {
  const dot = file.lastIndexOf('.')
  return {
    ...steamCandidate(file.slice(0, dot)),
    source: 'file',
    platform: '',
    launchType: 'emulator',
    launchTarget: `C:\\Games\\${file}`,
    installDir: 'C:\\Games',
    file,
    extension: file.slice(dot + 1),
    discs: 1,
    ...over,
  }
}

// folderCandidate is a candidate of the "Pasta" source, placed in a subfolder (IMV-016).
function folderCandidate(file: string, over: Partial<ImportCandidate> = {}): ImportCandidate {
  return { ...fileCandidate(file), source: 'folder', launchTarget: `E:\\Games\\ps1\\${file}`, installDir: 'E:\\Games\\ps1', ...over }
}

function renderDialog() {
  const onImported = vi.fn()
  const onClose = vi.fn()
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={client}>
      <ImportDialog onImported={onImported} onClose={onClose} />
    </QueryClientProvider>,
  )
  return { onImported, onClose }
}

function chooseSource(source: ImportSource) {
  fireEvent.change(screen.getByLabelText('Fonte'), { target: { value: source } })
}

// chooseOption selects an option by its visible text, not by its value.
function chooseOption(select: HTMLSelectElement, text: string) {
  const opt = Array.from(select.options).find((o) => o.textContent === text)
  if (!opt) {
    throw new Error(`opção "${text}" ausente`)
  }
  fireEvent.change(select, { target: { value: opt.value } })
}

function platformSelect(): HTMLSelectElement {
  return screen.getByRole('combobox', { name: 'Plataforma do arquivo' }) as HTMLSelectElement
}

function titleFromSelect(): HTMLSelectElement {
  return screen.getByRole('combobox', { name: 'Título a partir de' }) as HTMLSelectElement
}

function buscar(): HTMLElement {
  return screen.getByRole('button', { name: 'Buscar' })
}

function importar(): HTMLElement {
  return screen.getByRole('button', { name: 'Importar selecionados' })
}

// pickPath clicks the chooser button, waits for the path field and for Buscar to enable.
async function pickPath(button: string, label: string, value: string) {
  fireEvent.click(screen.getByRole('button', { name: button }))
  await waitFor(() => expect(screen.getByRole('textbox', { name: label })).toHaveValue(value))
  await waitFor(() => expect(buscar()).toBeEnabled())
}

async function scanAndWait() {
  fireEvent.click(buscar())
  await screen.findByRole('table')
}

// rowBoxes returns the row checkboxes in table order (not the "Selecionar todos" one).
function rowBoxes(): HTMLInputElement[] {
  return screen.getAllByRole('checkbox').filter((b) => {
    const name = b.getAttribute('aria-label') ?? ''
    return name.startsWith('Selecionar ') && name !== 'Selecionar todos'
  }) as HTMLInputElement[]
}

function titleBox(file: string): HTMLInputElement {
  return screen.getByRole('textbox', { name: `Título de ${file}` }) as HTMLInputElement
}

describe('ImportDialog', () => {
  beforeEach(() => {
    mocks.scan.mockReset()
    mocks.apply.mockReset()
    mocks.pickFile.mockReset()
    mocks.pickFolder.mockReset()
    mocks.platforms = [N64, SONY, WINDOWS]
    // applyImportInLots reaches the binding through window, not through the mocked export.
    ;(window as unknown as { go?: unknown }).go = {
      main: { App: { ImportApply: (c: unknown, p: unknown) => mocks.apply(c, p) } },
    }
  })

  it('IMV-060/Plataforma do arquivo é um select em ordem de nome, antes de Buscar, sem campo de texto', () => {
    mocks.platforms = [WINDOWS, SONY, N64]
    renderDialog()
    chooseSource('file')

    const select = platformSelect()
    expect(select.tagName).toBe('SELECT')
    expect(Array.from(select.options).map((o) => o.textContent)).toEqual([
      'Nintendo 64',
      'Sony Playstation',
      'Windows',
    ])
    expect(select.compareDocumentPosition(buscar()) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(screen.queryByRole('textbox', { name: /Plataforma/ })).toBeNull()
  })

  it('IMV-060/o campo de plataforma some em Steam, Epic e LaunchBox', () => {
    renderDialog()
    for (const source of ['steam', 'epic', 'launchbox'] as const) {
      chooseSource(source)
      expect(screen.queryByRole('combobox', { name: 'Plataforma do arquivo' }), source).toBeNull()
    }
  })

  it('IMV-061/vem a plataforma de chave windows mesmo renomeada', () => {
    mocks.platforms = [makePlatform(1, 'PC de mesa', 'windows'), SONY]
    renderDialog()
    chooseSource('file')
    expect(platformSelect()).toHaveDisplayValue('PC de mesa')
  })

  it('IMV-061/windows excluída: vem a primeira plataforma da lista', () => {
    mocks.platforms = [N64, SONY]
    renderDialog()
    chooseSource('folder')
    expect(platformSelect()).toHaveDisplayValue('Nintendo 64')
  })

  it('IMV-061/a escolha do usuário se mantém ao trocar de fonte na mesma abertura', () => {
    renderDialog()
    chooseSource('file')
    chooseOption(platformSelect(), 'Sony Playstation')
    chooseSource('folder')
    expect(platformSelect()).toHaveDisplayValue('Sony Playstation')
    chooseSource('file')
    expect(platformSelect()).toHaveDisplayValue('Sony Playstation')
  })

  it('IMV-062/sem plataformas: select desativado com a mensagem; Buscar e Importar desativados em Arquivo e Pasta', async () => {
    mocks.platforms = []
    mocks.pickFile.mockResolvedValue('C:\\Games\\a.z64')
    mocks.pickFolder.mockResolvedValue('E:\\Games')
    renderDialog()

    chooseSource('file')
    expect(platformSelect()).toBeDisabled()
    expect(screen.getByText('Nenhuma plataforma. Crie uma em Plataformas.')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Escolher arquivo' }))
    await waitFor(() => expect(screen.getByRole('textbox', { name: 'Arquivo' })).toHaveValue('C:\\Games\\a.z64'))
    expect(buscar()).toBeDisabled()
    expect(importar()).toBeDisabled()

    chooseSource('folder')
    expect(platformSelect()).toBeDisabled()
    fireEvent.click(screen.getByRole('button', { name: 'Escolher pasta' }))
    await waitFor(() => expect(screen.getByRole('textbox', { name: 'Pasta' })).toHaveValue('E:\\Games'))
    expect(buscar()).toBeDisabled()
    expect(importar()).toBeDisabled()
  })

  it('IMV-063/o nome da plataforma vai em ImportScan e em cada candidato dos lotes', async () => {
    mocks.scan.mockResolvedValue({
      candidates: [folderCandidate('Spyro.cue'), folderCandidate('Crash.cue')],
      warnings: [],
    })
    mocks.apply.mockResolvedValue({ applied: [], skipped: [] })
    mocks.pickFolder.mockResolvedValue('E:\\Games\\ps1')
    renderDialog()

    chooseSource('folder')
    chooseOption(platformSelect(), 'Sony Playstation')
    await pickPath('Escolher pasta', 'Pasta', 'E:\\Games\\ps1')
    await scanAndWait()
    expect(mocks.scan.mock.calls[0][2]).toEqual(expect.objectContaining({ platform: 'Sony Playstation' }))

    fireEvent.click(importar())
    await waitFor(() => expect(mocks.apply).toHaveBeenCalledTimes(1))
    const sent = mocks.apply.mock.calls[0][0] as ImportCandidate[]
    expect(sent).toHaveLength(2)
    expect(sent.map((c) => c.platform)).toEqual(['Sony Playstation', 'Sony Playstation'])
  })

  it('IMV-017/Título a partir de só existe na fonte Pasta; o valor vai ao ImportScan', async () => {
    mocks.scan.mockResolvedValue({ candidates: [folderCandidate('Spyro.cue')], warnings: [] })
    mocks.pickFolder.mockResolvedValue('E:\\Games\\ps1')
    renderDialog()

    chooseSource('file')
    expect(screen.queryByRole('combobox', { name: 'Título a partir de' })).toBeNull()

    chooseSource('folder')
    const titleFrom = titleFromSelect()
    expect(Array.from(titleFrom.options).map((o) => o.textContent)).toEqual([
      'Nome do arquivo',
      'Nome da pasta do jogo',
    ])
    expect(titleFrom).toHaveDisplayValue('Nome do arquivo')
    chooseOption(titleFrom, 'Nome da pasta do jogo')
    await pickPath('Escolher pasta', 'Pasta', 'E:\\Games\\ps1')
    await scanAndWait()

    expect(mocks.scan.mock.calls[0][0]).toBe('folder')
    expect(mocks.scan.mock.calls[0][2]).toEqual(expect.objectContaining({ titleFrom: 'folder' }))
  })

  it('IMV-011/Ler subpastas marcada por padrão, só na Pasta, e o valor vai como recursive', async () => {
    mocks.scan.mockResolvedValue({ candidates: [folderCandidate('Spyro.cue')], warnings: [] })
    mocks.pickFolder.mockResolvedValue('E:\\Games\\ps1')
    renderDialog()

    chooseSource('file')
    expect(screen.queryByRole('checkbox', { name: 'Ler subpastas' })).toBeNull()

    chooseSource('folder')
    const subfolders = screen.getByRole('checkbox', { name: 'Ler subpastas' })
    expect(subfolders).toBeChecked()
    await pickPath('Escolher pasta', 'Pasta', 'E:\\Games\\ps1')
    await scanAndWait()
    expect(mocks.scan.mock.calls[0][2]).toEqual(expect.objectContaining({ recursive: true }))

    fireEvent.click(screen.getByRole('checkbox', { name: 'Ler subpastas' }))
    fireEvent.click(buscar())
    await waitFor(() => expect(mocks.scan).toHaveBeenCalledTimes(2))
    expect(mocks.scan.mock.calls[1][2]).toEqual(expect.objectContaining({ recursive: false }))
  })

  it('IMV-041/15 candidatos com 2 existentes e 1 repetido: 12 marcados; Buscar de novo repõe a marcação padrão', async () => {
    const candidates = Array.from({ length: 15 }, (_, i) =>
      steamCandidate(`Jogo ${String(i + 1).padStart(2, '0')}`, { exists: i < 2 }),
    )
    candidates[5] = { ...candidates[5], title: candidates[4].title }
    mocks.scan.mockResolvedValue({ candidates, warnings: [] })
    renderDialog()

    await scanAndWait()
    expect(rowBoxes().filter((b) => b.checked)).toHaveLength(12)
    expect(screen.getByText('12 de 13 jogos selecionados (2 já existem)')).toBeInTheDocument()

    fireEvent.click(rowBoxes()[2])
    expect(screen.getByText('11 de 13 jogos selecionados (2 já existem)')).toBeInTheDocument()

    fireEvent.click(buscar())
    await waitFor(() => expect(screen.getByText('12 de 13 jogos selecionados (2 já existem)')).toBeInTheDocument())
    expect(rowBoxes().filter((b) => b.checked)).toHaveLength(12)
  })

  it('IMV-045/título vazio ou com 201 caracteres numa linha marcada desativa Importar; desmarcada não bloqueia; 200 é válido', async () => {
    mocks.scan.mockResolvedValue({
      candidates: [fileCandidate('a.z64', { title: 'Jogo A' }), fileCandidate('b.z64', { title: 'Jogo B' })],
      warnings: [],
    })
    mocks.pickFile.mockResolvedValue('C:\\Games\\a.z64')
    renderDialog()
    chooseSource('file')
    await pickPath('Escolher arquivo', 'Arquivo', 'C:\\Games\\a.z64')
    await scanAndWait()

    fireEvent.change(titleBox('a.z64'), { target: { value: '' } })
    expect(titleBox('a.z64')).toHaveAttribute('aria-invalid', 'true')
    expect(titleBox('a.z64')).toHaveAccessibleDescription('Título vazio')
    expect(importar()).toBeDisabled()

    fireEvent.change(titleBox('a.z64'), { target: { value: 'x'.repeat(201) } })
    expect(titleBox('a.z64')).toHaveAccessibleDescription('Título com mais de 200 caracteres')
    expect(importar()).toBeDisabled()

    fireEvent.click(rowBoxes()[0])
    expect(importar()).toBeEnabled()

    fireEvent.click(rowBoxes()[0])
    fireEvent.change(titleBox('a.z64'), { target: { value: 'y'.repeat(200) } })
    expect(titleBox('a.z64')).not.toHaveAttribute('aria-invalid', 'true')
    expect(importar()).toBeEnabled()
  })

  it('IMV-045/o título enviado vai aparado (espaços nas pontas saem)', async () => {
    mocks.scan.mockResolvedValue({
      candidates: [fileCandidate('a.z64', { title: 'Jogo A' }), fileCandidate('b.z64', { title: 'Jogo B' })],
      warnings: [],
    })
    mocks.apply.mockResolvedValue({ applied: [], skipped: [] })
    mocks.pickFile.mockResolvedValue('C:\\Games\\a.z64')
    renderDialog()
    chooseSource('file')
    await pickPath('Escolher arquivo', 'Arquivo', 'C:\\Games\\a.z64')
    await scanAndWait()

    fireEvent.change(titleBox('b.z64'), { target: { value: '  Jogo B  ' } })
    fireEvent.click(importar())
    await waitFor(() => expect(mocks.apply).toHaveBeenCalledTimes(1))
    const sent = mocks.apply.mock.calls[0][0] as ImportCandidate[]
    expect(sent.map((c) => c.title)).toEqual(['Jogo A', 'Jogo B'])
  })

  it('IMV-046/linha repetida marcada à mão é enviada ao ImportApply', async () => {
    mocks.scan.mockResolvedValue({
      candidates: [
        steamCandidate('Jogo X', { launchTarget: 'steam://rungameid/1' }),
        steamCandidate('Jogo X', { launchTarget: 'steam://rungameid/2' }),
      ],
      warnings: [],
    })
    mocks.apply.mockResolvedValue({ applied: [], skipped: [] })
    renderDialog()
    await scanAndWait()

    expect(screen.getByText('título repetido')).toBeInTheDocument()
    expect(rowBoxes()[1]).not.toBeChecked()
    fireEvent.click(rowBoxes()[1])
    fireEvent.click(importar())
    await waitFor(() => expect(mocks.apply).toHaveBeenCalledTimes(1))
    expect(mocks.apply.mock.calls[0][0]).toHaveLength(2)
  })

  it('IMV-047/contador sem já existem: "k de n jogos selecionados" acompanha marcação e Buscar', async () => {
    mocks.scan.mockResolvedValue({
      candidates: [steamCandidate('Um'), steamCandidate('Dois'), steamCandidate('Tres')],
      warnings: [],
    })
    renderDialog()
    await scanAndWait()

    expect(screen.getByText('3 de 3 jogos selecionados')).toBeInTheDocument()
    fireEvent.click(rowBoxes()[0])
    expect(screen.getByText('2 de 3 jogos selecionados')).toBeInTheDocument()
    fireEvent.click(buscar())
    await waitFor(() => expect(screen.getByText('3 de 3 jogos selecionados')).toBeInTheDocument())
  })

  it('IMV-047/contador com já existem: acrescenta "(j já existem)" e não conta as existentes', async () => {
    mocks.scan.mockResolvedValue({
      candidates: [steamCandidate('Um'), steamCandidate('Dois'), steamCandidate('Tres'), steamCandidate('Quatro', { exists: true })],
      warnings: [],
    })
    renderDialog()
    await scanAndWait()

    expect(screen.getByText('3 de 3 jogos selecionados (1 já existem)')).toBeInTheDocument()
    fireEvent.click(rowBoxes()[0])
    expect(screen.getByText('2 de 3 jogos selecionados (1 já existem)')).toBeInTheDocument()
    fireEvent.click(buscar())
    await waitFor(() => expect(screen.getByText('3 de 3 jogos selecionados (1 já existem)')).toBeInTheDocument())
  })

  it('IMV-048/60 linhas em lotes de 25, 25 e 10, com o título editado no lote certo', async () => {
    mocks.scan.mockResolvedValue({
      candidates: Array.from({ length: 60 }, (_, i) => fileCandidate(`arq${i}.z64`, { title: `Jogo ${i}` })),
      warnings: [],
    })
    mocks.apply.mockResolvedValue({ applied: [], skipped: [] })
    mocks.pickFile.mockResolvedValue('C:\\Games\\lote.z64')
    renderDialog()
    chooseSource('file')
    await pickPath('Escolher arquivo', 'Arquivo', 'C:\\Games\\lote.z64')
    await scanAndWait()

    fireEvent.change(titleBox('arq30.z64'), { target: { value: 'Editado 30' } })
    fireEvent.click(importar())
    await waitFor(() => expect(mocks.apply).toHaveBeenCalledTimes(3))

    const lots = mocks.apply.mock.calls.map((call) => call[0] as ImportCandidate[])
    expect(lots.map((l) => l.length)).toEqual([25, 25, 10])
    expect(lots[1][5].file).toBe('arq30.z64')
    expect(lots[1][5].title).toBe('Editado 30')
    expect(lots[2][0].file).toBe('arq50.z64')
  })

  it('IMV-048/"Importando k de N…" e Importar, Fechar e Esc desativados durante a operação', async () => {
    mocks.scan.mockResolvedValue({
      candidates: Array.from({ length: 60 }, (_, i) => steamCandidate(`Jogo ${i}`)),
      warnings: [],
    })
    let release: (v: unknown) => void = () => {}
    mocks.apply
      .mockImplementationOnce(() => Promise.resolve({ applied: [], skipped: [] }))
      .mockImplementationOnce(
        () =>
          new Promise((r) => {
            release = r
          }),
      )
      .mockImplementationOnce(() => Promise.resolve({ applied: [], skipped: [] }))
    const { onClose } = renderDialog()
    await scanAndWait()
    fireEvent.click(importar())

    expect(await screen.findByText('Importando 25 de 60…')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Fechar' })).toBeDisabled()
    expect(importar()).toBeDisabled()
    fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape' })
    expect(onClose).not.toHaveBeenCalled()

    release({ applied: [], skipped: [] })
    await waitFor(() => expect(screen.queryByText(/Importando \d+ de 60…/)).toBeNull())
    expect(screen.getByRole('button', { name: 'Fechar' })).toBeEnabled()
  })

  it('IMV-048/falha no 2º lote: mostra o resumo do 1º e o aviso com quantos não foram enviados; onImported roda', async () => {
    mocks.scan.mockResolvedValue({
      candidates: Array.from({ length: 60 }, (_, i) => steamCandidate(`Jogo ${i}`)),
      warnings: [],
    })
    mocks.apply
      .mockResolvedValueOnce({
        applied: Array.from({ length: 25 }, (_, i) => ({ gameId: i, title: `Jogo ${i}` })),
        skipped: [],
      })
      .mockRejectedValueOnce(new Error('O agente não está respondendo.'))
    const { onImported } = renderDialog()
    await scanAndWait()
    fireEvent.click(importar())

    expect(await screen.findByText('Importados: 25')).toBeInTheDocument()
    expect(screen.getByRole('alert')).toHaveTextContent('35 jogos não foram enviados')
    expect(mocks.apply).toHaveBeenCalledTimes(2)
    expect(onImported).toHaveBeenCalledTimes(1)
  })

  it('IMV-048/só as linhas marcadas são enviadas, na ordem da tabela', async () => {
    mocks.scan.mockResolvedValue({
      candidates: [steamCandidate('Um'), steamCandidate('Dois'), steamCandidate('Tres')],
      warnings: [],
    })
    mocks.apply.mockResolvedValue({ applied: [], skipped: [] })
    renderDialog()
    await scanAndWait()

    fireEvent.click(rowBoxes()[1])
    fireEvent.click(importar())
    await waitFor(() => expect(mocks.apply).toHaveBeenCalledTimes(1))
    expect((mocks.apply.mock.calls[0][0] as ImportCandidate[]).map((c) => c.title)).toEqual(['Um', 'Tres'])
  })

  it('IMV-049/editar um título e mudar a plataforma descarta a lista; Buscar traz a lista nova com marcação padrão', async () => {
    mocks.scan.mockResolvedValue({
      candidates: [fileCandidate('a.z64', { title: 'A' }), fileCandidate('b.z64', { title: 'B' })],
      warnings: [],
    })
    mocks.pickFile.mockResolvedValue('C:\\Games\\a.z64')
    renderDialog()
    chooseSource('file')
    await pickPath('Escolher arquivo', 'Arquivo', 'C:\\Games\\a.z64')
    await scanAndWait()

    fireEvent.change(titleBox('a.z64'), { target: { value: 'Editado' } })
    chooseOption(platformSelect(), 'Sony Playstation')
    expect(screen.queryByRole('table')).toBeNull()
    expect(screen.queryAllByRole('checkbox')).toHaveLength(0)

    await scanAndWait()
    expect(titleBox('a.z64')).toHaveValue('A')
    expect(rowBoxes().every((b) => b.checked)).toBe(true)
  })

  it('IMV-049/mudar fonte, pasta, "Título a partir de" ou "Ler subpastas" descarta a lista', async () => {
    mocks.scan.mockResolvedValue({
      candidates: [fileCandidate('a.z64', { title: 'A' }), fileCandidate('b.z64', { title: 'B' })],
      warnings: [],
    })
    mocks.pickFile.mockResolvedValue('C:\\Games\\a.z64')
    mocks.pickFolder.mockResolvedValue('E:\\Games\\ps1')
    renderDialog()

    chooseSource('file')
    await pickPath('Escolher arquivo', 'Arquivo', 'C:\\Games\\a.z64')
    await scanAndWait()
    chooseSource('steam')
    expect(screen.queryByRole('table')).toBeNull()

    chooseSource('folder')
    await pickPath('Escolher pasta', 'Pasta', 'E:\\Games\\ps1')
    await scanAndWait()
    chooseOption(titleFromSelect(), 'Nome da pasta do jogo')
    expect(screen.queryByRole('table')).toBeNull()

    await scanAndWait()
    mocks.pickFolder.mockResolvedValueOnce('E:\\Outra')
    fireEvent.click(screen.getByRole('button', { name: 'Escolher pasta' }))
    await waitFor(() => expect(screen.queryByRole('table')).toBeNull())

    await scanAndWait()
    fireEvent.click(screen.getByRole('checkbox', { name: 'Ler subpastas' }))
    expect(screen.queryByRole('table')).toBeNull()
  })

  it('IMV-050/Importados: N, recusados com o motivo, e a linha gravada vira "já existe" e fica bloqueada', async () => {
    mocks.scan.mockResolvedValue({ candidates: [steamCandidate('Um'), steamCandidate('Dois')], warnings: [] })
    mocks.apply.mockResolvedValue({
      applied: [{ gameId: 1, title: 'Um' }],
      skipped: [{ title: 'Dois', reason: 'duplicate' }],
    })
    renderDialog()
    await scanAndWait()

    fireEvent.click(importar())
    expect(await screen.findByText('Importados: 1')).toBeInTheDocument()
    expect(screen.getByText('Dois: já estava na biblioteca')).toBeInTheDocument()
    expect(screen.getByText('já existe')).toBeInTheDocument()
    expect(screen.getByRole('checkbox', { name: 'Selecionar Um' })).toBeDisabled()
    expect(screen.getByRole('checkbox', { name: 'Selecionar Dois' })).not.toBeChecked()
  })

  it('IMV-050/linhas não enviadas por falha de lote continuam marcadas; as gravadas viram "já existe"', async () => {
    mocks.scan.mockResolvedValue({
      candidates: Array.from({ length: 60 }, (_, i) => steamCandidate(`Jogo ${i}`)),
      warnings: [],
    })
    mocks.apply
      .mockResolvedValueOnce({
        applied: Array.from({ length: 25 }, (_, i) => ({ gameId: i, title: `Jogo ${i}` })),
        skipped: [],
      })
      .mockRejectedValueOnce(new Error('O agente não está respondendo.'))
    renderDialog()
    await scanAndWait()
    fireEvent.click(importar())

    expect(await screen.findByText('Importados: 25')).toBeInTheDocument()
    expect(screen.getAllByText('já existe')).toHaveLength(25)
    expect(screen.getByRole('checkbox', { name: 'Selecionar Jogo 0' })).toBeDisabled()
    expect(rowBoxes()[30]).toBeChecked()
    expect(rowBoxes()[59]).toBeChecked()
  })

  it('IMV-043/candidato que já existe: selo, caixa desmarcada e desativada, título sem campo', async () => {
    mocks.scan.mockResolvedValue({
      candidates: [fileCandidate('hades.z64', { title: 'Hades', exists: true }), fileCandidate('celeste.z64', { title: 'Celeste' })],
      warnings: [],
    })
    mocks.pickFile.mockResolvedValue('C:\\Games\\hades.z64')
    renderDialog()
    chooseSource('file')
    await pickPath('Escolher arquivo', 'Arquivo', 'C:\\Games\\hades.z64')
    await scanAndWait()

    expect(screen.getByText('já existe')).toBeInTheDocument()
    const box = screen.getByRole('checkbox', { name: 'Selecionar Hades' })
    expect(box).not.toBeChecked()
    expect(box).toBeDisabled()
    expect(screen.queryByRole('textbox', { name: 'Título de hades.z64' })).toBeNull()
    expect(titleBox('celeste.z64')).toBeInTheDocument()
  })

  it('IMV-014/os avisos do scan aparecem na janela', async () => {
    mocks.scan.mockResolvedValue({
      candidates: [steamCandidate('Hades')],
      warnings: ['Steam: appmanifest_570.acf ilegível'],
    })
    renderDialog()
    await scanAndWait()
    expect(screen.getByText('Steam: appmanifest_570.acf ilegível')).toBeInTheDocument()
  })

  it('IMV-052/Esc dentro do campo de título fecha a janela; durante a operação não fecha', async () => {
    mocks.scan.mockResolvedValue({ candidates: [fileCandidate('a.z64', { title: 'A' })], warnings: [] })
    mocks.apply.mockReturnValue(new Promise(() => {}))
    mocks.pickFile.mockResolvedValue('C:\\Games\\a.z64')
    const { onClose } = renderDialog()
    chooseSource('file')
    await pickPath('Escolher arquivo', 'Arquivo', 'C:\\Games\\a.z64')
    await scanAndWait()

    fireEvent.keyDown(titleBox('a.z64'), { key: 'Escape' })
    expect(onClose).toHaveBeenCalledTimes(1)

    fireEvent.click(importar())
    await waitFor(() => expect(mocks.apply).toHaveBeenCalledTimes(1))
    fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape' })
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('IMV-051/janela grande; cabeçalho e botões ficam fora do corpo que rola com a tabela', async () => {
    mocks.scan.mockResolvedValue({ candidates: [steamCandidate('Um'), steamCandidate('Dois')], warnings: [] })
    renderDialog()
    await scanAndWait()

    expect(screen.getByRole('dialog')).toHaveAttribute('data-size', 'large')
    const scroller = screen.getByRole('table').parentElement as HTMLElement
    expect(scroller.contains(importar())).toBe(false)
    expect(scroller.contains(buscar())).toBe(false)
    expect(scroller.contains(screen.getByRole('button', { name: 'Fechar' }))).toBe(false)
  })

  it('UIX-039/foco preso no diálogo: Tab no último elemento volta ao primeiro', () => {
    renderDialog()
    const first = screen.getByLabelText('Fonte')
    const close = screen.getByRole('button', { name: 'Fechar' })
    close.focus()
    fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Tab' })
    expect(document.activeElement).toBe(first)
  })
})
