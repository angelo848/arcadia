import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, fireEvent, render, screen, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { TimeToBeatEditor } from './TimeToBeatEditor'
import { useTimeToBeatEvents } from '../lib/timeToBeat'
import type { TimeToBeatCandidate, TimeToBeatView } from '../lib/timeToBeat'

// T19: seção "Tempo para zerar (HowLongToBeat)" de "Editar jogo" (TTB-018, TTB-019, TTB-040, TTB-017, TTB-039).
// Premissas (design 3.7): TimeToBeatEditor({ gameId, title }); lê por TimeToBeatGet (fetch true na hora, sem espera);
// o estado "serviço desligado" vem de TimeToBeatSettings().enabled === false; o campo de busca é um textbox
// "Termo de busca" iniciado com `title`; cada candidato é um botão cujo texto traz título, ano, plataformas e
// os três valores; fora do formulário: nenhum botão "Salvar" na seção.

type Handler = (...d: unknown[]) => void
let handlers: Map<string, Set<Handler>>
let app: Record<string, ReturnType<typeof vi.fn>>

const FETCHED = '2026-11-20T12:00:00.000Z'

function localDate(iso: string): string {
  const d = new Date(iso)
  const p = (n: number) => String(n).padStart(2, '0')
  return `${p(d.getDate())}/${p(d.getMonth() + 1)}/${d.getFullYear()}`
}

function view(over: Partial<TimeToBeatView> = {}): TimeToBeatView {
  return {
    gameId: 10,
    state: 'found',
    source: 'hltb',
    externalId: '1',
    externalTitle: 'Hollow Knight',
    mainSeconds: 97200,
    mainExtraSeconds: 5400,
    completionistSeconds: 99000,
    fetchedAt: FETCHED,
    confirmed: false,
    stale: false,
    pending: false,
    unavailable: false,
    candidates: [],
    ...over,
  }
}

function cand(i: number, over: Partial<TimeToBeatCandidate> = {}): TimeToBeatCandidate {
  return {
    externalId: `c${i}`,
    title: `Jogo ${i}`,
    year: 2000 + i,
    platforms: ['PC', 'Switch'],
    mainSeconds: 3600 * i,
    mainExtraSeconds: 5400,
    completionistSeconds: 7200,
    score: 0.7,
    ...over,
  }
}

type Opts = { get?: TimeToBeatView; enabled?: boolean }

function install(o: Opts = {}) {
  handlers = new Map()
  app = {
    TimeToBeatGet: vi.fn(() => Promise.resolve(o.get ?? view())),
    TimeToBeatRefresh: vi.fn(() => Promise.resolve(view())),
    TimeToBeatSearch: vi.fn(() => Promise.resolve([cand(1), cand(2)])),
    TimeToBeatChoose: vi.fn(() => Promise.resolve(view({ confirmed: true }))),
    TimeToBeatSettings: vi.fn(() => Promise.resolve({ enabled: o.enabled ?? true, afterMetadata: true })),
    TimeToBeatSetSettings: vi.fn(),
  }
  ;(window as unknown as { go?: unknown }).go = { main: { App: app } }
  ;(window as unknown as { runtime?: unknown }).runtime = {
    EventsOn: (name: string, cb: Handler) => {
      if (!handlers.has(name)) handlers.set(name, new Set())
      handlers.get(name)!.add(cb)
      return () => handlers.get(name)!.delete(cb)
    },
  }
}

function emit(name: string, ...data: unknown[]) {
  act(() => {
    for (const cb of handlers.get(name) ?? []) cb(...data)
  })
}

function Events({ children }: { children: ReactNode }) {
  useTimeToBeatEvents()
  return <>{children}</>
}

function renderEditor(title = 'Hollow Knight') {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <Events>
        <TimeToBeatEditor gameId={10} title={title} />
      </Events>
    </QueryClientProvider>,
  )
}

async function flush(ms = 0) {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms)
  })
}

// Text of the whole section, normalized, for state lines that may be split in elements.
const text = () => (document.body.textContent ?? '').replace(/\s+/g, ' ')

beforeEach(() => {
  vi.useFakeTimers()
  install()
})

afterEach(() => {
  vi.useRealTimers()
  const w = window as unknown as { go?: unknown; runtime?: unknown }
  delete w.go
  delete w.runtime
})

describe('TimeToBeatEditor: seção e estados', () => {
  it('TTB-018/mostra a seção "Tempo para zerar (HowLongToBeat)" com os três rótulos e valores', async () => {
    renderEditor()
    await flush()
    expect(screen.getByText('Tempo para zerar (HowLongToBeat)')).toBeInTheDocument()
    for (const l of ['História principal', 'História + extras', '100%']) expect(screen.getByText(l)).toBeInTheDocument()
    expect(screen.getByText('27 h')).toBeInTheDocument()
    expect(screen.getByText('1½ h')).toBeInTheDocument()
    expect(screen.getByText('27½ h')).toBeInTheDocument()
  })

  it('TTB-018/ao abrir pede fetch true uma vez, sem espera', async () => {
    renderEditor()
    await flush()
    expect(app.TimeToBeatGet.mock.calls.filter((c) => c[1] === true)).toHaveLength(1)
    await flush(5000)
    expect(app.TimeToBeatGet.mock.calls.filter((c) => c[1] === true)).toHaveLength(1)
  })

  it('TTB-018/Encontrado: título no HowLongToBeat · buscado em DD/MM/AAAA', async () => {
    renderEditor()
    await flush()
    expect(text()).toContain(`Encontrado: Hollow Knight · buscado em ${localDate(FETCHED)}`)
    expect(text()).not.toContain('· confirmado')
  })

  it('TTB-018/Encontrado confirmado acrescenta "· confirmado"', async () => {
    install({ get: view({ confirmed: true }) })
    renderEditor()
    await flush()
    expect(text()).toContain(`Encontrado: Hollow Knight · buscado em ${localDate(FETCHED)} · confirmado`)
  })

  it('TTB-018/Encontrado no IGDB (reserva): título no IGDB · buscado em DD/MM/AAAA', async () => {
    install({ get: view({ source: 'igdb', externalTitle: 'Hollow Knight (IGDB)' }) })
    renderEditor()
    await flush()
    expect(text()).toContain(`Encontrado no IGDB (reserva): Hollow Knight (IGDB) · buscado em ${localDate(FETCHED)}`)
  })

  const nulls = { mainSeconds: null, mainExtraSeconds: null, completionistSeconds: null }
  const stateCases: Array<[string, Partial<TimeToBeatView>, string]> = [
    ['Correspondência incerta', { state: 'doubt', ...nulls }, 'Correspondência incerta'],
    ['Não encontrado', { state: 'not-found', ...nulls }, 'Não encontrado'],
    ['Ainda não buscado', { state: 'none', source: '', externalId: '', externalTitle: '', fetchedAt: '', ...nulls }, 'Ainda não buscado'],
    [
      'HowLongToBeat indisponível no momento',
      { state: 'none', source: '', externalId: '', externalTitle: '', fetchedAt: '', unavailable: true, ...nulls },
      'HowLongToBeat indisponível no momento',
    ],
  ]
  for (const [name, over, want] of stateCases) {
    it(`TTB-018/linha de estado: ${name}`, async () => {
      install({ get: view(over) })
      renderEditor()
      await flush()
      expect(text()).toContain(want)
      expect(screen.getAllByText('—')).toHaveLength(3)
    })
  }

  it('TTB-039/linha da fonte: "Fonte: HowLongToBeat" para hltb, "Fonte: IGDB" para igdb, ausente sem valor', async () => {
    const { unmount } = renderEditor()
    await flush()
    expect(screen.getByText('Fonte: HowLongToBeat')).toBeInTheDocument()
    unmount()

    install({ get: view({ source: 'igdb' }) })
    const second = renderEditor()
    await flush()
    expect(screen.getByText('Fonte: IGDB')).toBeInTheDocument()
    second.unmount()

    install({ get: view({ state: 'not-found', mainSeconds: null, mainExtraSeconds: null, completionistSeconds: null }) })
    renderEditor()
    await flush()
    expect(screen.queryByText(/^Fonte:/)).toBeNull()
  })

  it('TTB-017/timeToBeat:changed do jogo relê uma vez (fetch false); de outro jogo não', async () => {
    renderEditor()
    await flush()
    const reads = () => app.TimeToBeatGet.mock.calls.filter((c) => c[1] === false).length
    const before = reads()
    emit('timeToBeat:changed', { gameId: 99 })
    await flush()
    expect(reads()).toBe(before)
    emit('timeToBeat:changed', { gameId: 10 })
    await flush()
    expect(reads()).toBe(before + 1)
  })
})

describe('TimeToBeatEditor: Atualizar', () => {
  it('TTB-018/"Atualizar" chama TimeToBeatRefresh(G), fica desativado com "Buscando…" e mostra o resultado', async () => {
    let done: (v: TimeToBeatView) => void = () => {}
    app.TimeToBeatRefresh.mockImplementation(() => new Promise<TimeToBeatView>((r) => (done = r)))
    renderEditor()
    await flush()
    fireEvent.click(screen.getByRole('button', { name: 'Atualizar' }))
    await flush()
    expect(app.TimeToBeatRefresh).toHaveBeenCalledTimes(1)
    expect(app.TimeToBeatRefresh).toHaveBeenCalledWith(10)
    const busy = screen.getByRole('button', { name: 'Buscando…' })
    expect(busy).toBeDisabled()
    fireEvent.click(busy)
    expect(app.TimeToBeatRefresh).toHaveBeenCalledTimes(1)

    await act(async () => {
      done(view({ mainSeconds: 1800 }))
    })
    await flush()
    expect(screen.getByText('30 min')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Atualizar' })).toBeEnabled()
    expect(screen.queryByText('Buscando…')).toBeNull()
  })

  it('TTB-018/a falha mostra "Não foi possível consultar o HowLongToBeat" sem mudar os valores', async () => {
    app.TimeToBeatRefresh.mockImplementation(() => Promise.reject(new Error('indisponível')))
    renderEditor()
    await flush()
    fireEvent.click(screen.getByRole('button', { name: 'Atualizar' }))
    await flush()
    expect(screen.getByText('Não foi possível consultar o HowLongToBeat')).toBeInTheDocument()
    expect(screen.getByText('27 h')).toBeInTheDocument()
    expect(screen.getByText('1½ h')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Atualizar' })).toBeEnabled()
  })

  it('TTB-018/vale na hora: não há botão "Salvar" na seção e nada além de TimeToBeat* é chamado', async () => {
    renderEditor()
    await flush()
    fireEvent.click(screen.getByRole('button', { name: 'Atualizar' }))
    await flush()
    expect(screen.queryByRole('button', { name: 'Salvar' })).toBeNull()
    const called = Object.entries(app)
      .filter(([, f]) => f.mock.calls.length > 0)
      .map(([n]) => n)
    expect(called.filter((n) => !n.startsWith('TimeToBeat'))).toEqual([])
  })

  it('TTB-018/serviço desligado: só o cache e "Consulta ao HowLongToBeat desligada", sem botões', async () => {
    install({ enabled: false })
    renderEditor()
    await flush()
    expect(text()).toContain('Consulta ao HowLongToBeat desligada')
    expect(screen.getByText('27 h')).toBeInTheDocument()
    expect(screen.queryByRole('button')).toBeNull()
    expect(app.TimeToBeatRefresh).not.toHaveBeenCalled()
  })

  it('TTB-018/serviço desligado: não pede fetch true', async () => {
    install({ enabled: false })
    renderEditor()
    await flush(2000)
    expect(app.TimeToBeatGet.mock.calls.filter((c) => c[1] === true)).toHaveLength(0)
  })
})

describe('TimeToBeatEditor: Escolher outro…', () => {
  async function openSearch() {
    fireEvent.click(screen.getByRole('button', { name: 'Escolher outro…' }))
    await flush()
  }

  it('TTB-019/"Escolher outro…" abre o campo com o termo editável e "Buscar", sem buscar sozinho', async () => {
    renderEditor('Hollow Knight')
    await flush()
    await openSearch()
    const field = screen.getByRole('textbox', { name: 'Termo de busca' })
    expect(field).toHaveValue('Hollow Knight')
    expect(screen.getByRole('button', { name: 'Buscar' })).toBeInTheDocument()
    expect(app.TimeToBeatSearch).not.toHaveBeenCalled()
  })

  it('TTB-019/"Buscar" chama TimeToBeatSearch(G, termo editado) e lista título, ano, plataformas e os três valores', async () => {
    renderEditor('Hollow Knight')
    await flush()
    await openSearch()
    fireEvent.change(screen.getByRole('textbox', { name: 'Termo de busca' }), { target: { value: 'Hollow' } })
    fireEvent.click(screen.getByRole('button', { name: 'Buscar' }))
    await flush()
    expect(app.TimeToBeatSearch).toHaveBeenCalledTimes(1)
    expect(app.TimeToBeatSearch).toHaveBeenCalledWith(10, 'Hollow')
    const b = screen.getByRole('button', { name: /Jogo 1/ })
    expect(b).toHaveTextContent('Jogo 1')
    expect(b).toHaveTextContent('2001')
    expect(b).toHaveTextContent('PC')
    expect(b).toHaveTextContent('Switch')
    expect(b).toHaveTextContent('1 h')
    expect(b).toHaveTextContent('1½ h')
    expect(b).toHaveTextContent('2 h')
  })

  it('TTB-019/a lista mostra no máximo 10 candidatos', async () => {
    app.TimeToBeatSearch.mockImplementation(() => Promise.resolve(Array.from({ length: 12 }, (_, i) => cand(i + 1))))
    renderEditor()
    await flush()
    await openSearch()
    fireEvent.click(screen.getByRole('button', { name: 'Buscar' }))
    await flush()
    expect(screen.getAllByRole('button', { name: /Jogo \d+/ })).toHaveLength(10)
  })

  it('TTB-019/escolher um candidato chama TimeToBeatChoose(G, id) e mostra "· confirmado"', async () => {
    renderEditor()
    await flush()
    await openSearch()
    fireEvent.click(screen.getByRole('button', { name: 'Buscar' }))
    await flush()
    fireEvent.click(screen.getByRole('button', { name: /Jogo 2/ }))
    await flush()
    expect(app.TimeToBeatChoose).toHaveBeenCalledTimes(1)
    expect(app.TimeToBeatChoose).toHaveBeenCalledWith(10, 'c2')
    expect(text()).toContain('· confirmado')
  })

  it('TTB-019/"Nenhum destes" chama TimeToBeatChoose(G, "")', async () => {
    app.TimeToBeatChoose.mockImplementation(() =>
      Promise.resolve(view({ state: 'not-found', confirmed: true, mainSeconds: null, mainExtraSeconds: null, completionistSeconds: null })),
    )
    renderEditor()
    await flush()
    await openSearch()
    fireEvent.click(screen.getByRole('button', { name: 'Buscar' }))
    await flush()
    fireEvent.click(screen.getByRole('button', { name: 'Nenhum destes' }))
    await flush()
    expect(app.TimeToBeatChoose).toHaveBeenCalledTimes(1)
    expect(app.TimeToBeatChoose).toHaveBeenCalledWith(10, '')
    expect(text()).toContain('Não encontrado')
  })

  it('TTB-019/em doubt a lista abre com os candidatos gravados, sem nova busca', async () => {
    install({ get: view({ state: 'doubt', mainSeconds: null, mainExtraSeconds: null, completionistSeconds: null, candidates: [cand(1), cand(2), cand(3)] }) })
    renderEditor()
    await flush()
    await openSearch()
    expect(screen.getAllByRole('button', { name: /Jogo \d+/ })).toHaveLength(3)
    expect(app.TimeToBeatSearch).not.toHaveBeenCalled()
  })

  it('TTB-040/linha igdb: "Escolher outro…" lista jogos do HowLongToBeat e a escolha mostra "· confirmado" com a fonte HowLongToBeat', async () => {
    install({ get: view({ source: 'igdb', confirmed: false, externalTitle: 'Hollow Knight (IGDB)' }) })
    app.TimeToBeatChoose.mockImplementation(() => Promise.resolve(view({ source: 'hltb', confirmed: true })))
    renderEditor()
    await flush()
    expect(screen.getByText('Fonte: IGDB')).toBeInTheDocument()
    expect(text()).not.toContain('· confirmado')
    await openSearch()
    fireEvent.click(screen.getByRole('button', { name: 'Buscar' }))
    await flush()
    expect(app.TimeToBeatSearch).toHaveBeenCalledWith(10, expect.any(String))
    fireEvent.click(screen.getByRole('button', { name: /Jogo 1/ }))
    await flush()
    expect(app.TimeToBeatChoose).toHaveBeenCalledWith(10, 'c1')
    expect(text()).toContain('· confirmado')
    expect(screen.getByText('Fonte: HowLongToBeat')).toBeInTheDocument()
    expect(screen.queryByText('Fonte: IGDB')).toBeNull()
  })

  it('TTB-013/falha da busca ou da escolha não lança nem abre modal', async () => {
    app.TimeToBeatSearch.mockImplementation(() => Promise.reject(new Error('x')))
    renderEditor()
    await flush()
    await openSearch()
    fireEvent.click(screen.getByRole('button', { name: 'Buscar' }))
    await flush()
    expect(screen.queryByRole('dialog')).toBeNull()
    expect(within(document.body).getByText('27 h')).toBeInTheDocument()
  })
})
