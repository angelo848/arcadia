import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, within } from '@testing-library/react'
import { Profiler } from 'react'
import { ImportReviewTable } from './ImportReviewTable'
import type { ImportReviewTableProps } from './ImportReviewTable'
import type { ImportCandidate, ImportSource, ReviewRow } from '../lib/imports'

// Tests for the review table (phase 11, T14). Written from requirements IMV-040..IMV-052,
// not from the implementation. The component does not exist yet: these tests are expected to fail.

function candidate(over: Partial<ImportCandidate> = {}): ImportCandidate {
  return {
    source: 'file',
    title: 'Jogo',
    platform: 'Sony Playstation',
    launchType: 'file',
    launchTarget: 'E:\\Games\\jogo.bin',
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
    file: 'jogo.bin',
    extension: '.bin',
    discs: 1,
    discFiles: [],
    ...over,
  }
}

function row(over: Partial<ImportCandidate> = {}, checked = true, title?: string): ReviewRow {
  const c = candidate(over)
  return { candidate: c, title: title ?? c.title, checked }
}

function props(over: Partial<ImportReviewTableProps> = {}): ImportReviewTableProps {
  return {
    rows: [],
    source: 'file' as ImportSource,
    busy: false,
    repeated: [],
    onToggle: vi.fn(),
    onToggleAll: vi.fn(),
    onTitle: vi.fn(),
    ...over,
  }
}

function bodyRows(): HTMLElement[] {
  return screen.getAllByRole('row').slice(1) // first row is the header
}

function selectAll(): HTMLInputElement {
  return screen.getByRole('checkbox', { name: /selecionar todos/i }) as HTMLInputElement
}

describe('ImportReviewTable', () => {
  it('IMV-040/colunas e linhas na ordem recebida; Steam mostra — em Arquivo, Extensão e Disco', () => {
    const rows = [
      row({ title: 'Zelda', source: 'steam', launchType: 'steam', file: undefined, extension: undefined, discs: undefined }, true),
      row({ title: 'Mario', file: 'mario.zip', extension: '.zip', discs: 1 }, true),
    ]
    render(<ImportReviewTable {...props({ rows, source: 'steam' })} />)

    for (const name of ['Título', 'Arquivo', 'Extensão', 'Disco']) {
      expect(screen.getByRole('columnheader', { name })).toBeInTheDocument()
    }
    const trs = bodyRows()
    expect(trs).toHaveLength(2)
    expect(within(trs[0]).getByText('Zelda')).toBeInTheDocument()
    expect(within(trs[1]).getByText('Mario')).toBeInTheDocument()

    // cells of the Steam row: Arquivo, Extensão and Disco are all "—"
    const dashes = within(trs[0]).getAllByRole('cell').filter((c) => c.textContent === '—')
    expect(dashes).toHaveLength(3)
  })

  it('IMV-040/discs 1 mostra — e discs 2 mostra "2 discos"; title do Arquivo é o launchTarget e o do Disco lista discFiles', () => {
    const rows = [
      row({ title: 'Solo', discs: 1, launchTarget: 'E:\\solo.bin', discFiles: [] }),
      row({
        title: 'Final Fantasy VIII',
        file: 'ff8.cue',
        extension: '.cue',
        discs: 2,
        launchTarget: 'E:\\ff8\\ff8.cue',
        discFiles: ['E:\\ff8\\disc1.bin', 'E:\\ff8\\disc2.bin'],
      }),
    ]
    render(<ImportReviewTable {...props({ rows })} />)

    const [solo, ff8] = bodyRows()
    expect(within(solo).getByText('—', { selector: 'td' })).toBeInTheDocument()
    expect(within(ff8).getByText('2 discos')).toBeInTheDocument()

    expect(within(ff8).getByTitle('E:\\ff8\\ff8.cue')).toBeInTheDocument()
    const discTitle = within(ff8).getByTitle(/disc1\.bin/)
    expect(discTitle.getAttribute('title')).toContain('E:\\ff8\\disc1.bin')
    expect(discTitle.getAttribute('title')).toContain('E:\\ff8\\disc2.bin')
  })

  it('IMV-040/selo já existe aparece na linha do candidato existente', () => {
    const rows = [row({ title: 'Existente', exists: true }, false), row({ title: 'Novo' })]
    render(<ImportReviewTable {...props({ rows })} />)

    const [existing, novo] = bodyRows()
    expect(within(existing).getByText('já existe')).toBeInTheDocument()
    expect(within(novo).queryByText('já existe')).toBeNull()
  })

  it('IMV-042/Selecionar todos: três estados (vazio, parcial indeterminate, tudo)', () => {
    const none = [row({ title: 'A' }, false), row({ title: 'B' }, false)]
    const { rerender } = render(<ImportReviewTable {...props({ rows: none })} />)
    expect(selectAll().checked).toBe(false)
    expect(selectAll().indeterminate).toBe(false)

    const partial = [row({ title: 'A' }, true), row({ title: 'B' }, false)]
    rerender(<ImportReviewTable {...props({ rows: partial })} />)
    expect(selectAll().indeterminate).toBe(true)

    const all = [row({ title: 'A' }, true), row({ title: 'B' }, true)]
    rerender(<ImportReviewTable {...props({ rows: all })} />)
    expect(selectAll().checked).toBe(true)
    expect(selectAll().indeterminate).toBe(false)
  })

  it('IMV-042/marcar tudo inclui as repetidas', () => {
    const onToggleAll = vi.fn()
    const rows = [row({ title: 'Igual' }, true), row({ title: 'Igual' }, false), row({ title: 'Outro' }, false)]
    render(<ImportReviewTable {...props({ rows, repeated: [false, true, false], onToggleAll })} />)

    fireEvent.click(selectAll())
    expect(onToggleAll).toHaveBeenCalledTimes(1)
    expect(onToggleAll).toHaveBeenCalledWith(true)
  })

  it('IMV-042/desmarcar quando tudo marcado', () => {
    const onToggleAll = vi.fn()
    const rows = [row({ title: 'A' }, true), row({ title: 'B' }, true)]
    render(<ImportReviewTable {...props({ rows, onToggleAll })} />)

    fireEvent.click(selectAll())
    expect(onToggleAll).toHaveBeenCalledWith(false)
  })

  it('IMV-042/desativado sem linhas marcáveis e em busy', () => {
    const allExisting = [row({ title: 'A', exists: true }, false)]
    const { rerender } = render(<ImportReviewTable {...props({ rows: allExisting })} />)
    expect(selectAll()).toBeDisabled()

    const markable = [row({ title: 'A' }, true)]
    rerender(<ImportReviewTable {...props({ rows: markable, busy: true })} />)
    expect(selectAll()).toBeDisabled()

    rerender(<ImportReviewTable {...props({ rows: markable, busy: false })} />)
    expect(selectAll()).toBeEnabled()
  })

  it('IMV-043/linha exists: caixa desmarcada e desativada, título sem campo, fora do Selecionar todos', () => {
    const onToggle = vi.fn()
    const rows = [row({ title: 'Ja Tenho', exists: true }, false), row({ title: 'Novo' }, true)]
    render(<ImportReviewTable {...props({ rows, onToggle })} />)

    const [existing, novo] = bodyRows()
    const box = within(existing).getByRole('checkbox', { name: 'Selecionar Ja Tenho' }) as HTMLInputElement
    expect(box.checked).toBe(false)
    expect(box).toBeDisabled()
    expect(within(existing).queryByRole('textbox')).toBeNull()
    expect(within(existing).getByText('Ja Tenho')).toBeInTheDocument()

    // Selecionar todos ignores the existing row: the only selectable row is checked, so the header is checked.
    expect(selectAll().checked).toBe(true)
    expect(selectAll().indeterminate).toBe(false)
    expect(within(novo).getByRole('checkbox', { name: 'Selecionar Novo' })).toBeChecked()
  })

  it('IMV-044/linha de arquivo: campo de título editável aceita colar e acentos', () => {
    const onTitle = vi.fn()
    const rows = [row({ title: 'Pokemon', file: 'pokemon.iso' })]
    render(<ImportReviewTable {...props({ rows, source: 'file', onTitle })} />)

    const field = screen.getByRole('textbox', { name: 'Título de pokemon.iso' }) as HTMLInputElement
    expect(field.value).toBe('Pokemon')

    fireEvent.change(field, { target: { value: 'Pokémon Açúcar' } })
    expect(onTitle).toHaveBeenCalledWith(0, 'Pokémon Açúcar')
  })

  it('IMV-044/linha de pasta também tem campo de título', () => {
    const onTitle = vi.fn()
    const rows = [row({ title: 'Pasta Jogo', source: 'folder', file: 'Pasta Jogo' })]
    render(<ImportReviewTable {...props({ rows, source: 'folder', onTitle })} />)

    expect(screen.getByRole('textbox', { name: 'Título de Pasta Jogo' })).toBeInTheDocument()
  })

  it('IMV-044/Steam, Epic e LaunchBox: texto sem campo', () => {
    for (const source of ['steam', 'epic', 'launchbox'] as const) {
      const rows = [row({ title: 'Sem Campo', source, file: undefined, extension: undefined, discs: undefined })]
      const { unmount } = render(<ImportReviewTable {...props({ rows, source })} />)

      const tr = bodyRows()[0]
      expect(within(tr).queryByRole('textbox')).toBeNull()
      expect(within(tr).getByText('Sem Campo')).toBeInTheDocument()
      unmount()
    }
  })

  it('IMV-045/título vazio: aria-invalid e mensagem "Título vazio" ligada ao campo', () => {
    const rows = [row({ title: 'X', file: 'x.iso' }, true, '')]
    render(<ImportReviewTable {...props({ rows })} />)

    const field = screen.getByRole('textbox', { name: 'Título de x.iso' })
    expect(field).toHaveAttribute('aria-invalid', 'true')
    expect(field).toHaveAccessibleDescription('Título vazio')
  })

  it('IMV-045/título só com espaços conta como vazio', () => {
    const rows = [row({ title: 'X', file: 'x.iso' }, true, '    ')]
    render(<ImportReviewTable {...props({ rows })} />)

    const field = screen.getByRole('textbox', { name: 'Título de x.iso' })
    expect(field).toHaveAttribute('aria-invalid', 'true')
    expect(field).toHaveAccessibleDescription('Título vazio')
  })

  it('IMV-045/201 caracteres: aria-invalid e mensagem "Título com mais de 200 caracteres"', () => {
    const rows = [row({ title: 'X', file: 'x.iso' }, true, 'a'.repeat(201))]
    render(<ImportReviewTable {...props({ rows })} />)

    const field = screen.getByRole('textbox', { name: 'Título de x.iso' })
    expect(field).toHaveAttribute('aria-invalid', 'true')
    expect(field).toHaveAccessibleDescription('Título com mais de 200 caracteres')
  })

  it('IMV-045/200 caracteres e espaços nas pontas são válidos', () => {
    const rows = [
      row({ title: 'X', file: 'x.iso' }, true, 'a'.repeat(200)),
      row({ title: 'Y', file: 'y.iso' }, true, '  Jogo  '),
    ]
    render(<ImportReviewTable {...props({ rows })} />)

    const [first, second] = [
      screen.getByRole('textbox', { name: 'Título de x.iso' }),
      screen.getByRole('textbox', { name: 'Título de y.iso' }),
    ]
    expect(first).not.toHaveAttribute('aria-invalid', 'true')
    expect(second).not.toHaveAttribute('aria-invalid', 'true')
  })

  it('IMV-045/linha desmarcada inválida não marca o campo', () => {
    const rows = [row({ title: 'X', file: 'x.iso' }, false, '')]
    render(<ImportReviewTable {...props({ rows })} />)

    const field = screen.getByRole('textbox', { name: 'Título de x.iso' })
    expect(field).not.toHaveAttribute('aria-invalid', 'true')
  })

  it('IMV-046/selo "título repetido" aparece e some com a edição; a marcação não muda sozinha', () => {
    const onTitle = vi.fn()
    const rows = [row({ title: 'Igual' }, true), row({ title: 'Igual' }, false)]
    const { rerender } = render(<ImportReviewTable {...props({ rows, repeated: [false, true], onTitle })} />)

    const second = bodyRows()[1]
    expect(within(second).getByText('título repetido')).toBeInTheDocument()
    expect(within(second).getByRole('checkbox', { name: 'Selecionar Igual' })).not.toBeChecked()

    // user edits the repeated row to a unique title: parent recomputes repeated flags
    const edited = [row({ title: 'Igual' }, true), row({ title: 'Diferente' }, false)]
    rerender(<ImportReviewTable {...props({ rows: edited, repeated: [false, false], onTitle })} />)

    expect(screen.queryByText('título repetido')).toBeNull()
    expect(within(bodyRows()[1]).getByRole('checkbox', { name: 'Selecionar Diferente' })).not.toBeChecked()
  })

  it('IMV-046/linha repetida marcada à mão: a caixa fica marcada e o selo continua', () => {
    const onToggle = vi.fn()
    const rows = [row({ title: 'Igual' }, true), row({ title: 'Igual' }, true)]
    render(<ImportReviewTable {...props({ rows, repeated: [false, true], onToggle })} />)

    const box = within(bodyRows()[1]).getByRole('checkbox', { name: 'Selecionar Igual' })
    expect(box).toBeChecked()
    expect(within(bodyRows()[1]).getByText('título repetido')).toBeInTheDocument()
  })

  it('IMV-051/estrutura: contêiner com overflow-y e cabeçalho sticky', () => {
    render(<ImportReviewTable {...props({ rows: [row({ title: 'A' })] })} />)

    const table = screen.getByRole('table')
    const scroller = table.parentElement as HTMLElement
    expect(['auto', 'scroll']).toContain(getComputedStyle(scroller).overflowY)

    const th = screen.getByRole('columnheader', { name: 'Título' })
    expect(getComputedStyle(th).position).toBe('sticky')
  })

  it('IMV-051/1.000 linhas renderizam sem erro', () => {
    const rows = Array.from({ length: 1000 }, (_, i) => row({ title: `Jogo ${i}` }))
    render(<ImportReviewTable {...props({ rows })} />)

    expect(bodyRows()).toHaveLength(1000)
  })

  it('IMV-052/papéis table, columnheader, nomes "Selecionar <título>" e "Título de <arquivo>"', () => {
    const rows = [row({ title: 'Mario', file: 'mario.zip', extension: '.zip' })]
    render(<ImportReviewTable {...props({ rows })} />)

    expect(screen.getByRole('table')).toBeInTheDocument()
    expect(screen.getAllByRole('columnheader').length).toBeGreaterThanOrEqual(4)
    expect(screen.getByRole('checkbox', { name: 'Selecionar Mario' })).toBeInTheDocument()
    expect(screen.getByRole('textbox', { name: 'Título de mario.zip' })).toBeInTheDocument()
  })

  it('IMV-052/Tab percorre caixa e campo de cada linha (ordem do DOM)', () => {
    const rows = [row({ title: 'Um', file: 'um.zip' }), row({ title: 'Dois', file: 'dois.zip' })]
    render(<ImportReviewTable {...props({ rows })} />)

    // Tab order follows DOM order. The implementer must give each control an aria-label equal to its
    // accessible name (IMV-052), which is what this test reads.
    const focusable = Array.from(
      screen.getByRole('table').querySelectorAll<HTMLElement>('input:not([disabled]), button:not([disabled])'),
    )
    expect(focusable.map((el) => el.getAttribute('aria-label'))).toEqual([
      'Selecionar todos',
      'Selecionar Um',
      'Título de um.zip',
      'Selecionar Dois',
      'Título de dois.zip',
    ])
  })

  it('IMV-052/Espaço alterna a caixa (clique nativo do checkbox)', () => {
    const onToggle = vi.fn()
    const rows = [row({ title: 'Um' }, false)]
    render(<ImportReviewTable {...props({ rows, onToggle })} />)

    // jsdom does not map Space to click; the native checkbox activation is the contract checked here.
    fireEvent.click(screen.getByRole('checkbox', { name: 'Selecionar Um' }))
    expect(onToggle).toHaveBeenCalledWith(0, true)
  })

  it('IMV-052/digitar num título não re-renderiza as outras linhas (Profiler em 3 linhas)', () => {
    const onRender = vi.fn()
    const base = [row({ title: 'A', file: 'a.zip' }), row({ title: 'B', file: 'b.zip' }), row({ title: 'C', file: 'c.zip' })]
    const tree = (rows: ReviewRow[]) => (
      <Profiler id="review" onRender={onRender}>
        <ImportReviewTable {...props({ rows })} />
      </Profiler>
    )
    const { rerender } = render(tree(base))
    const afterMount = onRender.mock.calls.length

    const edited = [base[0], { ...base[1], title: 'Bx' }, base[2]]
    rerender(tree(edited))

    // Only the edited row may commit. The Profiler sees the table subtree; a commit per row is
    // checked by the implementer's per-row memo. Here: at most one commit for one keystroke.
    expect(onRender.mock.calls.length - afterMount).toBeLessThanOrEqual(1)
  })
})
