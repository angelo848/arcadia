import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, fireEvent, render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { TimeToBeatBlock } from './TimeToBeatBlock'
import { useTimeToBeatEvents } from '../lib/timeToBeat'
import type { TimeToBeatView } from '../lib/timeToBeat'

// T19: bloco "Tempo para zerar" do painel do modo normal (TTB-013, TTB-015, TTB-017, TTB-039).
// Premissas (design 3.7): o bloco recebe { gameId } e, opcionalmente, onEdit (o painel passa o seu);
// o serviço desligado vem de TimeToBeatSettings().enabled === false; os bindings vêm de window.go.main.App;
// o evento vem de window.runtime.EventsOn (montado fora do bloco, por useTimeToBeatEvents no App).

type Handler = (...d: unknown[]) => void
let handlers: Map<string, Set<Handler>>
let app: {
  TimeToBeatGet: ReturnType<typeof vi.fn>
  TimeToBeatSettings: ReturnType<typeof vi.fn>
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
    fetchedAt: '2026-11-20T10:00:00.000Z',
    confirmed: false,
    stale: false,
    pending: false,
    unavailable: false,
    candidates: [],
    ...over,
  }
}

function install(get?: (id: number, fetch: boolean) => Promise<TimeToBeatView | null>, enabled = true) {
  handlers = new Map()
  app = {
    TimeToBeatGet: vi.fn(get ?? ((id: number) => Promise.resolve(view({ gameId: id })))),
    TimeToBeatSettings: vi.fn(() => Promise.resolve({ enabled, afterMetadata: true })),
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

function renderBlock(gameId = 10, onEdit?: (o?: { section?: string }) => void) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const ui = (id: number) => (
    <QueryClientProvider client={qc}>
      <Events>
        <TimeToBeatBlock gameId={id} {...(onEdit ? { onEdit } : {})} />
      </Events>
    </QueryClientProvider>
  )
  const utils = render(ui(gameId))
  return { ...utils, rerenderWith: (id: number) => utils.rerender(ui(id)) }
}

async function flush(ms = 0) {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms)
  })
}

const getCalls = (fetch: boolean, id?: number) =>
  app.TimeToBeatGet.mock.calls.filter((c) => c[1] === fetch && (id === undefined || c[0] === id))

const labels = ['História principal', 'História + extras', '100%']

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

describe('TimeToBeatBlock', () => {
  it('TTB-015/mostra "Tempo para zerar" com os três rótulos, os valores e a linha da fonte', async () => {
    renderBlock()
    await flush()
    expect(screen.getByText('Tempo para zerar')).toBeInTheDocument()
    for (const l of labels) expect(screen.getByText(l)).toBeInTheDocument()
    expect(screen.getByText('27 h')).toBeInTheDocument()
    expect(screen.getByText('1½ h')).toBeInTheDocument()
    expect(screen.getByText('27½ h')).toBeInTheDocument()
    expect(screen.getByText('Fonte: HowLongToBeat')).toBeInTheDocument()
  })

  it('TTB-015/lê o cache na hora (fetch false) e pede fetch true uma vez após 1.000 ms', async () => {
    renderBlock()
    await flush()
    expect(getCalls(false, 10)).toHaveLength(1)
    expect(getCalls(true)).toHaveLength(0)
    await flush(999)
    expect(getCalls(true)).toHaveLength(0)
    await flush(1)
    expect(getCalls(true, 10)).toHaveLength(1)
    await flush(5000)
    expect(getCalls(true)).toHaveLength(1)
  })

  it('TTB-015/trocar de jogo antes de 1.000 ms cancela o pedido do anterior', async () => {
    const { rerenderWith } = renderBlock(10)
    await flush(500)
    rerenderWith(20)
    await flush(600)
    expect(getCalls(true, 10)).toHaveLength(0)
    expect(getCalls(true, 20)).toHaveLength(0)
    await flush(400)
    expect(getCalls(true, 20)).toHaveLength(1)
    expect(getCalls(true, 10)).toHaveLength(0)
  })

  it('TTB-015/em doubt mostra "Correspondência incerta" e "Confirmar…", que chama onEdit({ section: "timeToBeat" })', async () => {
    install(() =>
      Promise.resolve(
        view({ state: 'doubt', source: 'hltb', mainSeconds: null, mainExtraSeconds: null, completionistSeconds: null }),
      ),
    )
    const onEdit = vi.fn()
    renderBlock(10, onEdit)
    await flush()
    expect(screen.getByText('Correspondência incerta')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Confirmar…' }))
    expect(onEdit).toHaveBeenCalledTimes(1)
    expect(onEdit).toHaveBeenCalledWith({ section: 'timeToBeat' })
  })

  it('TTB-015/fora de doubt não há "Confirmar…" nem "Correspondência incerta"', async () => {
    renderBlock()
    await flush()
    expect(screen.queryByRole('button', { name: 'Confirmar…' })).toBeNull()
    expect(screen.queryByText('Correspondência incerta')).toBeNull()
  })

  it('TTB-015/serviço desligado e sem cache: o bloco não aparece', async () => {
    install(() => Promise.resolve(view({ state: 'none', source: '', mainSeconds: null, mainExtraSeconds: null, completionistSeconds: null })), false)
    renderBlock()
    await flush(1500)
    expect(screen.queryByText('Tempo para zerar')).toBeNull()
    for (const l of labels) expect(screen.queryByText(l)).toBeNull()
  })

  it('TTB-015/serviço desligado com cache: o bloco aparece com os valores do cache', async () => {
    install(undefined, false)
    renderBlock()
    await flush()
    expect(screen.getByText('Tempo para zerar')).toBeInTheDocument()
    expect(screen.getByText('27 h')).toBeInTheDocument()
  })

  describe('TTB-013/falhas deixam os três rótulos com "—", sem alert, sem modal e sem mensagem de erro', () => {
    const cases: Array<[string, () => void]> = [
      ['binding que rejeita (erro do IPC)', () => install(() => Promise.reject(new Error('boom')))],
      ['agente desconectado', () => install(() => Promise.reject('agent disconnected'))],
      [
        'recuo (unavailable)',
        () =>
          install(() =>
            Promise.resolve(view({ state: 'none', source: '', unavailable: true, mainSeconds: null, mainExtraSeconds: null, completionistSeconds: null })),
          ),
      ],
      [
        'not-found',
        () =>
          install(() =>
            Promise.resolve(view({ state: 'not-found', source: 'hltb', mainSeconds: null, mainExtraSeconds: null, completionistSeconds: null })),
          ),
      ],
      [
        'doubt',
        () =>
          install(() =>
            Promise.resolve(view({ state: 'doubt', mainSeconds: null, mainExtraSeconds: null, completionistSeconds: null })),
          ),
      ],
      [
        'valores nulos',
        () => install(() => Promise.resolve(view({ mainSeconds: null, mainExtraSeconds: null, completionistSeconds: null }))),
      ],
      ['resposta nula', () => install(() => Promise.resolve(null))],
    ]
    for (const [name, setup] of cases) {
      it(`TTB-013/${name}`, async () => {
        setup()
        renderBlock()
        await flush(1500)
        for (const l of labels) expect(screen.getByText(l), l).toBeInTheDocument()
        expect(screen.getAllByText('—')).toHaveLength(3)
        expect(screen.queryByRole('alert')).toBeNull()
        expect(screen.queryByRole('dialog')).toBeNull()
        expect(screen.queryByText(/erro|falha|não foi possível/i)).toBeNull()
      })
    }

    it('TTB-013/serviço desligado (settings) com cache vazio e binding ausente não lança e não mostra alert', async () => {
      install(() => Promise.resolve(null), false)
      renderBlock()
      await flush(1500)
      expect(screen.queryByRole('alert')).toBeNull()
      expect(screen.queryByRole('dialog')).toBeNull()
    })
  })

  it('TTB-017/timeToBeat:changed do jogo mostrado relê uma vez (fetch false); de outro jogo não', async () => {
    renderBlock(10)
    await flush(1100)
    const before = getCalls(false).length
    emit('timeToBeat:changed', { gameId: 99 })
    await flush()
    expect(getCalls(false)).toHaveLength(before)
    emit('timeToBeat:changed', { gameId: 10 })
    await flush()
    expect(getCalls(false)).toHaveLength(before + 1)
    expect(getCalls(false).at(-1)?.[0]).toBe(10)
  })

  it('TTB-017/depois do aviso o bloco mostra os valores novos', async () => {
    let n = 0
    install(() => {
      n += 1
      return Promise.resolve(n === 1 ? view({ mainSeconds: 1800 }) : view({ mainSeconds: 5400 }))
    })
    renderBlock()
    await flush()
    expect(screen.getByText('30 min')).toBeInTheDocument()
    emit('timeToBeat:changed', { gameId: 10 })
    await flush()
    expect(screen.queryByText('30 min')).toBeNull()
    expect(screen.getAllByText('1½ h').length).toBeGreaterThan(0)
  })

  it('TTB-039/"Fonte: HowLongToBeat" para hltb', async () => {
    install(() => Promise.resolve(view({ source: 'hltb' })))
    renderBlock()
    await flush()
    expect(screen.getByText('Fonte: HowLongToBeat')).toBeInTheDocument()
    expect(screen.queryByText('Fonte: IGDB')).toBeNull()
  })

  it('TTB-039/"Fonte: IGDB" para igdb', async () => {
    install(() => Promise.resolve(view({ source: 'igdb' })))
    renderBlock()
    await flush()
    expect(screen.getByText('Fonte: IGDB')).toBeInTheDocument()
    expect(screen.queryByText('Fonte: HowLongToBeat')).toBeNull()
  })

  it('TTB-039/sem valor a linha da fonte não aparece', async () => {
    install(() =>
      Promise.resolve(view({ state: 'not-found', source: 'hltb', mainSeconds: null, mainExtraSeconds: null, completionistSeconds: null })),
    )
    renderBlock()
    await flush(1500)
    expect(screen.queryByText(/^Fonte:/)).toBeNull()
  })
})
