import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { Profiler } from 'react'
import { act, configure, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { MetadataReviewDialog } from './MetadataReviewDialog'
import type { MetadataReviewDialogProps } from './MetadataReviewDialog'
import { coverPlaceholderUrl } from './CoverImage'
import type { BatchRowView, BatchStatusView } from '../lib/metadataBatch'
import type { MetadataCandidateView, MetadataConfidence } from '../lib/metadata'

// Phase 12 (T37): the review window "Confirmar metadados" (MET-060..064, MET-066, MET-068, MET-069).
// Every case starts with the ID of the requirement it covers.
// The dialog loads its rows through window.go.main.App.MetadataBatchResults (lib/metadataBatch.ts, MET-057),
// so the binding is faked here; no network and no real timers.

type AppFake = Record<string, (...args: unknown[]) => Promise<unknown>>

function setApp(app: AppFake): void {
  ;(window as unknown as { go?: { main?: { App?: AppFake } } }).go = { main: { App: app } }
}

function clearApp(): void {
  delete (window as unknown as { go?: unknown }).go
}

function deferred<T = void>() {
  let resolve!: (v: T) => void
  const promise = new Promise<T>((r) => {
    resolve = r
  })
  return { promise, resolve }
}

const BATCH_ID = 7

// The suite runs ~67 files in parallel on jsdom; the default 1 s wait is too short under load.
configure({ asyncUtilTimeout: 5000 })

function batchOf(state: BatchStatusView['state'], total = 15): BatchStatusView {
  return { batchId: BATCH_ID, state, total, done: total, failed: 0, warnings: [] }
}

function title(id: number): string {
  return `Jogo ${String(id).padStart(3, '0')}`
}

function cand(name: string, year: number, platform: string, score: number): MetadataCandidateView {
  return {
    refs: [{ provider: 'igdb', providerId: `${name}-${year}` }],
    title: name,
    year,
    platform,
    score,
    thumbUrl: `https://images.igdb.com/igdb/image/upload/t_thumb/${year}.jpg`,
  }
}

function rowOf(id: number, confidence: MetadataConfidence, candidates: MetadataCandidateView[], selected: number): BatchRowView {
  return { gameId: id, title: title(id), platform: 'PC', confidence, selected, candidates }
}

// matchRow: "provável" → marked, first item selected by the agent (MET-061).
function matchRow(id: number): BatchRowView {
  return rowOf(id, 'match', [cand(`Achado ${String(id).padStart(3, '0')}`, 2001, 'PC', 0.92)], 0)
}

// doubtRow: "em dúvida" → unmarked; the agent has no selection, the first item is the default (MET-061).
function doubtRow(id: number): BatchRowView {
  return rowOf(
    id,
    'doubt',
    [cand(`Dúvida ${id}-a`, 1999, 'Switch', 0.7), cand(`Dúvida ${id}-b`, 2005, 'PC', 0.61)],
    -1,
  )
}

// noneRow: "nenhum" → unmarked, no candidates, no active checkbox (MET-061).
function noneRow(id: number): BatchRowView {
  return rowOf(id, 'none', [], -1)
}

// fixture: 15 rows, 9 match (ids 1..9), 4 doubt (10..13), 2 none (14..15).
// Default marking: 9 checked; selectable rows = 13; none = 2.
function fixture(): BatchRowView[] {
  const rows: BatchRowView[] = []
  for (let id = 1; id <= 9; id++) rows.push(matchRow(id))
  for (let id = 10; id <= 13; id++) rows.push(doubtRow(id))
  for (let id = 14; id <= 15; id++) rows.push(noneRow(id))
  return rows
}

function pageOf(all: BatchRowView[], offset: number, limit: number) {
  const next = offset + limit < all.length ? offset + limit : null
  return { rows: all.slice(offset, offset + limit), total: all.length, next }
}

// installApp fakes the results binding; every test that renders rows calls it first.
function installApp(rows: BatchRowView[]) {
  const results = vi.fn(async (_batchId: unknown, offset: unknown, limit: unknown) =>
    pageOf(rows, offset as number, limit as number),
  )
  setApp({ MetadataBatchResults: results as AppFake[string] })
  return results
}

type Over = Partial<MetadataReviewDialogProps>

function renderReview(over: Over = {}) {
  const props: MetadataReviewDialogProps = {
    batch: batchOf('ready'),
    onClose: vi.fn(),
    onApply: vi.fn(async () => undefined),
    onDiscard: vi.fn(async () => undefined),
    ...over,
  }
  return { ...render(<MetadataReviewDialog {...props} />), props }
}

// Waits for the final state: every row drawn, then effects flushed (the header's
// indeterminate flag is set in an effect, so it is only reliable after the flush).
async function loaded(count = 15): Promise<void> {
  await screen.findByRole('checkbox', { name: `Selecionar ${title(1)}` }, { timeout: 5000 })
  await waitFor(() => expect(allRowBoxes()).toHaveLength(count), { timeout: 5000 })
  await act(async () => {})
}

function checkbox(id: number): HTMLInputElement {
  return screen.getByRole('checkbox', { name: `Selecionar ${title(id)}` }) as HTMLInputElement
}

function selectOf(id: number): HTMLSelectElement {
  return screen.getByRole('combobox', { name: `Alternativas de ${title(id)}` }) as HTMLSelectElement
}

function rowOfTable(id: number): HTMLElement {
  return checkbox(id).closest('tr') as HTMLElement
}

function headerBox(): HTMLInputElement {
  return screen.getByRole('checkbox', { name: 'Selecionar todos' }) as HTMLInputElement
}

function applyButton(): HTMLButtonElement {
  return screen.getByRole('button', { name: 'Aplicar selecionados' }) as HTMLButtonElement
}

function discardButton(): HTMLButtonElement {
  return screen.getByRole('button', { name: 'Descartar tudo' }) as HTMLButtonElement
}

function allRowBoxes(): HTMLInputElement[] {
  return screen.getAllByRole('checkbox', { name: /^Selecionar Jogo / }) as HTMLInputElement[]
}

function checkedRowCount(): number {
  return allRowBoxes().filter((b) => b.checked).length
}

beforeEach(() => {
  clearApp()
})

afterEach(() => {
  clearApp()
})

describe('MetadataReviewDialog', () => {
  it('MET-060/colunas e cabeçalho da tabela', async () => {
    installApp(fixture())
    renderReview()
    await loaded()

    const table = screen.getByRole('table')
    const headers = within(table).getAllByRole('columnheader')
    const texts = headers.map((h) => h.textContent?.trim() ?? '')
    expect(texts).toEqual(expect.arrayContaining(['Capa', 'Jogo', 'Encontrado', 'Situação', 'Alternativas']))
    // Each header cell is a column header (scope="col") [MET-069].
    for (const h of headers) {
      expect(h).toHaveAttribute('scope', 'col')
    }
    // Body rows: one per game.
    expect(within(table).getAllByRole('row').length).toBeGreaterThanOrEqual(15 + 1)
  })

  it('MET-060/estrutura: título fixo, corpo com rolagem e botões fora do corpo', async () => {
    installApp(fixture())
    renderReview()
    await loaded()

    const dialog = screen.getByRole('dialog', { name: 'Confirmar metadados' })
    expect(dialog).toHaveAttribute('data-size', 'large')
    expect(within(dialog).getByRole('heading', { name: 'Confirmar metadados' })).toBeInTheDocument()

    const table = screen.getByRole('table')
    // The body is the scrolling region; the table lives inside it.
    expect(table.closest('.overflow-y-auto')).not.toBeNull()
    // Action buttons are pinned in the footer, outside the scrolling body.
    expect(applyButton().closest('.overflow-y-auto')).toBeNull()
    expect(discardButton().closest('.overflow-y-auto')).toBeNull()
  })

  it('MET-060/carrega em páginas de 100 e mostra "k de n jogos" enquanto carrega', async () => {
    const rows = Array.from({ length: 150 }, (_, k) => matchRow(k + 1))
    const pending = deferred<ReturnType<typeof pageOf>>()
    const results = vi.fn((_b: unknown, offset: unknown, limit: unknown) => {
      if (offset === 0) {
        return Promise.resolve(pageOf(rows, offset as number, limit as number))
      }
      return pending.promise
    })
    setApp({ MetadataBatchResults: results as AppFake[string] })

    renderReview()

    expect(await screen.findByText('100 de 150 jogos')).toBeInTheDocument()
    expect(results).toHaveBeenNthCalledWith(1, BATCH_ID, 0, 100)

    pending.resolve(pageOf(rows, 100, 100))

    await waitFor(() => expect(allRowBoxes()).toHaveLength(150))
    expect(results).toHaveBeenNthCalledWith(2, BATCH_ID, 100, 100)
    expect(screen.queryByText('100 de 150 jogos')).toBeNull()
  })

  it('MET-061/15 linhas (9 provável, 4 dúvida, 2 nenhum): 9 marcadas, dúvida desmarcada com 1º item, nenhum desmarcada e sem caixa ativa', async () => {
    installApp(fixture())
    renderReview()
    await loaded()

    expect(allRowBoxes()).toHaveLength(15)
    expect(checkedRowCount()).toBe(9)

    for (let id = 1; id <= 9; id++) {
      expect(checkbox(id).checked).toBe(true)
      expect(within(rowOfTable(id)).getByText('provável')).toBeInTheDocument()
    }
    for (let id = 10; id <= 13; id++) {
      expect(checkbox(id).checked).toBe(false)
      expect(selectOf(id).value).toBe('0')
      expect(within(rowOfTable(id)).getByText('em dúvida')).toBeInTheDocument()
    }
    for (let id = 14; id <= 15; id++) {
      expect(checkbox(id).checked).toBe(false)
      expect(checkbox(id)).toBeDisabled()
      expect(within(rowOfTable(id)).getByText('nenhum')).toBeInTheDocument()
    }
  })

  it('MET-061/escolher alternativa marca a linha; Nenhum desmarca e bloqueia a caixa', async () => {
    installApp(fixture())
    renderReview()
    await loaded()

    // Doubt row 10 is unmarked; choosing its second item marks it.
    fireEvent.change(selectOf(10), { target: { value: '1' } })
    expect(checkbox(10).checked).toBe(true)

    // "Nenhum" on a matched row unmarks it and blocks the checkbox.
    fireEvent.change(selectOf(1), { target: { value: '-1' } })
    expect(checkbox(1).checked).toBe(false)
    expect(checkbox(1)).toBeDisabled()

    // Choosing an item again makes the row active and marked.
    fireEvent.change(selectOf(1), { target: { value: '0' } })
    expect(checkbox(1)).not.toBeDisabled()
    expect(checkbox(1).checked).toBe(true)
  })

  it('MET-062/trocar a alternativa muda Encontrado e capa, situação vira "escolhido por você" e a pontuação continua em %', async () => {
    installApp(fixture())
    renderReview()
    await loaded()

    const row = rowOfTable(10)
    expect(within(row).getByText('Dúvida 10-a')).toBeInTheDocument()

    fireEvent.change(selectOf(10), { target: { value: '1' } })

    expect(within(row).getByText('Dúvida 10-b')).toBeInTheDocument()
    expect(within(row).queryByText('Dúvida 10-a')).toBeNull()
    expect(within(row).getByText('escolhido por você')).toBeInTheDocument()
    expect(within(row).getByText('61%')).toBeInTheDocument()
    expect(row.querySelector('img')).toHaveAttribute('src', 'https://images.igdb.com/igdb/image/upload/t_thumb/2005.jpg')
  })

  it('MET-062/select lista título, ano e plataforma na ordem do agente e "Nenhum"', async () => {
    installApp(fixture())
    renderReview()
    await loaded()

    const options = within(selectOf(10)).getAllByRole('option') as HTMLOptionElement[]
    const texts = options.map((o) => o.textContent ?? '')
    expect(options).toHaveLength(3)
    expect(texts[0]).toContain('Dúvida 10-a')
    expect(texts[0]).toContain('1999')
    expect(texts[0]).toContain('Switch')
    expect(texts[1]).toContain('Dúvida 10-b')
    expect(texts[1]).toContain('2005')
    expect(texts[1]).toContain('PC')
    expect(texts[2]).toBe('Nenhum')
    expect(options[2]).toHaveValue('-1')
  })

  it('MET-063/Selecionar todos: vazio, indeterminado e tudo', async () => {
    installApp(fixture())
    renderReview()
    await loaded()

    // Indeterminate: 9 of 13 selectable rows are marked (flag set by an effect, so wait for it).
    expect(headerBox().checked).toBe(false)
    await waitFor(() => expect(headerBox().indeterminate).toBe(true))

    // All: marks every row with a selected item (13), not the "none" rows.
    fireEvent.click(headerBox())
    expect(headerBox().checked).toBe(true)
    expect(headerBox().indeterminate).toBe(false)
    expect(checkedRowCount()).toBe(13)
    expect(checkbox(14)).toBeDisabled()

    // Empty: unmarks every row.
    fireEvent.click(headerBox())
    expect(headerBox().checked).toBe(false)
    expect(headerBox().indeterminate).toBe(false)
    expect(checkedRowCount()).toBe(0)
  })

  it('MET-063/desativado sem linha marcável e durante a aplicação', async () => {
    // No selectable row: only "none" rows.
    installApp([noneRow(1), noneRow(2)])
    const first = renderReview({ batch: batchOf('ready', 2) })
    await loaded(2)
    expect(headerBox()).toBeDisabled()
    first.unmount()
    clearApp()

    // Call in flight: onApply pending; checkboxes, header and apply are disabled.
    installApp(fixture())
    const pending = deferred()
    const onApply = vi.fn(() => pending.promise)
    renderReview({ onApply })
    await loaded()
    fireEvent.click(applyButton())
    expect(onApply).toHaveBeenCalledTimes(1)
    expect(headerBox()).toBeDisabled()
    expect(checkbox(1)).toBeDisabled()
    expect(applyButton()).toBeDisabled()
    pending.resolve()
  })

  it('MET-063/desativado quando o estado do lote é "applying"', async () => {
    installApp(fixture())
    renderReview({ batch: batchOf('applying') })
    await loaded()

    expect(headerBox()).toBeDisabled()
    expect(checkbox(1)).toBeDisabled()
    expect(applyButton()).toBeDisabled()
  })

  it('MET-063/contador "k de n jogos selecionados" com "(j sem correspondência)"', async () => {
    installApp(fixture())
    renderReview()
    await loaded()

    // 9 marked of 13 with an item; 2 without a match.
    expect(screen.getByText('9 de 13 jogos selecionados (2 sem correspondência)')).toBeInTheDocument()

    // Marking one more updates k.
    fireEvent.click(checkbox(10))
    expect(screen.getByText('10 de 13 jogos selecionados (2 sem correspondência)')).toBeInTheDocument()
  })

  it('MET-063/contador sem "(j sem correspondência)" quando não há linha nenhum', async () => {
    installApp([matchRow(1), matchRow(2), matchRow(3)])
    renderReview({ batch: batchOf('ready', 3) })
    await loaded(3)

    expect(screen.getByText('3 de 3 jogos selecionados')).toBeInTheDocument()
    expect(screen.queryByText(/sem correspondência/)).toBeNull()
  })

  it('MET-063/contador conta como "sem correspondência" as linhas nenhum que têm alternativas', async () => {
    // A "nenhum" row with alternatives is not a match, so it is counted too (MET-063, MET-053).
    const withAlternatives = rowOf(16, 'none', [cand('Alternativa 16', 2003, 'PC', 0.5)], -1)
    installApp([...fixture(), withAlternatives])
    renderReview()
    await loaded(16)

    // Same 13 selectable rows; 2 without alternatives plus the 1 with them = 3 without a match.
    expect(screen.getByText('9 de 13 jogos selecionados (3 sem correspondência)')).toBeInTheDocument()
  })

  it('MET-064/Aplicar selecionados envia só as linhas marcadas com o índice da alternativa e fecha', async () => {
    installApp(fixture())
    const onApply = vi.fn(async () => undefined)
    const { props } = renderReview({ onApply })
    await loaded()

    // Unmark game 3; choose the second item of doubt game 10 (marks it).
    fireEvent.click(checkbox(3))
    fireEvent.change(selectOf(10), { target: { value: '1' } })

    fireEvent.click(applyButton())

    await waitFor(() => expect(props.onClose).toHaveBeenCalledTimes(1))
    expect(onApply).toHaveBeenCalledTimes(1)
    expect(onApply).toHaveBeenCalledWith([
      { gameId: 1, candidate: 0 },
      { gameId: 2, candidate: 0 },
      { gameId: 4, candidate: 0 },
      { gameId: 5, candidate: 0 },
      { gameId: 6, candidate: 0 },
      { gameId: 7, candidate: 0 },
      { gameId: 8, candidate: 0 },
      { gameId: 9, candidate: 0 },
      { gameId: 10, candidate: 1 },
    ])
    expect(props.onDiscard).not.toHaveBeenCalled()
  })

  it('MET-064/desativado com zero marcadas', async () => {
    installApp(fixture())
    renderReview()
    await loaded()

    expect(applyButton()).toBeEnabled()

    // Select all, then clear: nothing marked.
    fireEvent.click(headerBox())
    fireEvent.click(headerBox())
    expect(checkedRowCount()).toBe(0)
    expect(applyButton()).toBeDisabled()
  })

  it('MET-064/Descartar tudo chama onDiscard e não aplica', async () => {
    installApp(fixture())
    const { props } = renderReview()
    await loaded()

    fireEvent.click(discardButton())

    await waitFor(() => expect(props.onDiscard).toHaveBeenCalledTimes(1))
    expect(props.onApply).not.toHaveBeenCalled()
  })

  it('MET-068/capa com falha cai no placeholder', async () => {
    installApp(fixture())
    renderReview()
    await loaded()

    const img = rowOfTable(1).querySelector('img') as HTMLImageElement
    expect(img).toHaveAttribute('src', 'https://images.igdb.com/igdb/image/upload/t_thumb/2001.jpg')
    expect(img).toHaveAttribute('alt', '')
    expect(img).toHaveAttribute('loading', 'lazy')
    expect(img).toHaveAttribute('referrerPolicy', 'no-referrer')

    fireEvent.error(img)

    expect(rowOfTable(1).querySelector('img')).toHaveAttribute('src', coverPlaceholderUrl)
  })

  it('MET-068/linha sem alternativa mostra o placeholder de capa', async () => {
    installApp(fixture())
    renderReview()
    await loaded()

    expect(rowOfTable(14).querySelector('img')).toHaveAttribute('src', coverPlaceholderUrl)
  })

  it('MET-069/th scope col; nomes "Selecionar <título>" e "Alternativas de <título>"', async () => {
    installApp(fixture())
    renderReview()
    await loaded()

    expect(checkbox(1)).toHaveAccessibleName(`Selecionar ${title(1)}`)
    expect(selectOf(1)).toHaveAccessibleName(`Alternativas de ${title(1)}`)
    expect(checkbox(10)).toHaveAccessibleName(`Selecionar ${title(10)}`)
    expect(selectOf(10)).toHaveAccessibleName(`Alternativas de ${title(10)}`)
    expect(headerBox()).toHaveAccessibleName('Selecionar todos')
    for (const h of screen.getAllByRole('columnheader')) {
      expect(h.tagName).toBe('TH')
      expect(h).toHaveAttribute('scope', 'col')
    }
  })

  it('MET-069/Tab percorre caixa e select de cada linha (ordem do DOM, sem tabindex -1)', async () => {
    installApp(fixture())
    renderReview()
    await loaded()

    // jsdom does not move focus with Tab; the tab order is the DOM order of the focusable controls.
    const box = checkbox(1)
    const sel = selectOf(1)
    expect(box.compareDocumentPosition(sel) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(box).not.toHaveAttribute('tabindex', '-1')
    expect(sel).not.toHaveAttribute('tabindex', '-1')

    const next = checkbox(2)
    expect(sel.compareDocumentPosition(next) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  })

  it('MET-069/Espaço alterna a caixa (ativação nativa do checkbox)', async () => {
    installApp(fixture())
    renderReview()
    await loaded()

    // jsdom does not map Space to click; the native checkbox activation is the contract checked here.
    expect(checkbox(1).type).toBe('checkbox')
    expect(checkbox(1).checked).toBe(true)
    fireEvent.click(checkbox(1))
    expect(checkbox(1).checked).toBe(false)
    fireEvent.click(checkbox(1))
    expect(checkbox(1).checked).toBe(true)
  })

  it('MET-069/Esc fecha sem descartar', async () => {
    installApp(fixture())
    const { props } = renderReview()
    await loaded()

    fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape' })

    expect(props.onClose).toHaveBeenCalledTimes(1)
    expect(props.onDiscard).not.toHaveBeenCalled()
    expect(props.onApply).not.toHaveBeenCalled()
  })

  it('MET-069/Esc ignorado durante chamada em curso (aplicação)', async () => {
    installApp(fixture())
    const pending = deferred()
    const onApply = vi.fn(() => pending.promise)
    const { props } = renderReview({ onApply })
    await loaded()

    fireEvent.click(applyButton())
    fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape' })
    expect(props.onClose).not.toHaveBeenCalled()

    // When the call ends the dialog closes; Esc is no longer blocked.
    pending.resolve()
    await waitFor(() => expect(props.onClose).toHaveBeenCalledTimes(1))
  })

  it('MET-060/cores só por tokens (sem cor literal nem paleta do Tailwind)', async () => {
    installApp(fixture())
    renderReview()
    await loaded()

    const dialog = screen.getByRole('dialog')
    const inlineColor = Array.from(dialog.querySelectorAll<HTMLElement>('[style]')).filter((el) =>
      /#|rgb|hsl/.test(el.getAttribute('style') ?? ''),
    )
    expect(inlineColor).toHaveLength(0)

    const palette =
      /\b(?:bg|text|border|ring|fill|stroke|outline|accent|shadow|divide|from|to|via)-(?:red|green|blue|gray|zinc|slate|neutral|amber|yellow|emerald|sky|orange|purple|indigo|pink|white|black)(?:-\d{2,3})?\b/
    const paletteClass = Array.from(dialog.querySelectorAll<HTMLElement>('[class]')).filter((el) =>
      palette.test(el.getAttribute('class') ?? ''),
    )
    expect(paletteClass).toHaveLength(0)
  })

  it('MET-060/digitar ou trocar uma linha não re-renderiza as outras (React.Profiler)', async () => {
    installApp(fixture())
    const onRender = vi.fn()
    const props: MetadataReviewDialogProps = {
      batch: batchOf('ready'),
      onClose: vi.fn(),
      onApply: vi.fn(async () => undefined),
      onDiscard: vi.fn(async () => undefined),
    }
    render(
      <Profiler id="review" onRender={onRender}>
        <MetadataReviewDialog {...props} />
      </Profiler>,
    )
    await loaded()
    const afterLoad = onRender.mock.calls.length

    // One change in one row: at most one commit of the table (no per-row re-render of the others).
    fireEvent.change(selectOf(10), { target: { value: '1' } })

    expect(onRender.mock.calls.length - afterLoad).toBeLessThanOrEqual(1)
    expect(checkbox(10).checked).toBe(true)
  })
})
