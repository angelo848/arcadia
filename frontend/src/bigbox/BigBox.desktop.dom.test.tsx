// TDK-038, TDK-002 (T12): DOM sweep of the real Desktop theme inside the shell, in every zone/state.
// Assumed (as BigBox.test.tsx): keyboard Enter = A, "d" = X, Escape = B, arrows move; "m" = Y. In the panel the focus is stacked
// (play -> play without tracking -> viewer) with ArrowDown. Themes are the real registry (red until T15 registers "desktop").
import type { ReactNode } from 'react'
import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useMediaEvents } from '../lib/metadata'
import { useTimeToBeatEvents } from '../lib/timeToBeat'
import { BigBox } from './BigBox'
import type { BigBoxProps } from './BigBox'

const ACTIONABLE = 'button, a[href], input, select, textarea, [role="button"], [role="link"], [role="menuitem"], [contenteditable]'
const FORBIDDEN = [
  'Big Box', 'Tema do Big Box', 'Importar', 'Provedores', 'Emuladores', 'Estatísticas', 'Editar', 'Buscar metadados', 'Buscar vídeo',
  'Abrir local do arquivo', 'Remover', 'Tentar de novo', 'Confirmar…', 'Escolher outro…', 'Buscar', 'Nenhum destes', 'Adicionar sessão',
  'Carregar mais', 'Ampliar', 'Limpar filtros', 'Pausar', 'Retomar',
]
const CATALOG = {
  platforms: [
    { id: 1, name: 'Windows', category: 'pc', emulatorId: null, seedKey: 'windows', iconUrl: '', gameCount: 3 },
    { id: 2, name: 'PlayStation 2', category: 'console', emulatorId: null, seedKey: 'ps2', iconUrl: '', gameCount: 1 },
  ],
  genres: [], companies: [], playModes: [],
}
const g = (id: number, title: string, platformId: number) => ({
  id, platformId, title, platform: platformId === 1 ? 'Windows' : 'PlayStation 2', path: `C:\\g\\${id}.exe`, status: 'not_played',
  favorite: false, year: null, totalPlaySeconds: 0, playCount: 0, firstPlayedAt: '', lastPlayedAt: '',
})
const ALL = [g(30, 'Celeste', 1), g(20, 'Hades', 1), g(10, 'Hollow Knight', 1), g(40, 'Gran Turismo', 2)]
const detailOf = (id: number) => ({
  ...ALL.find((x) => x.id === id)!, description: 'Descrição.', completedAt: null, developers: [], publishers: [], genres: [], playModes: [],
  launchType: 'exe', launchArgs: '', sessionPlaySeconds: 0, emulatorId: null,
})
const session = () => ({
  sessionId: 1, gameId: 30, title: 'Celeste', state: 'running', startedAt: '2026-11-15T10:00:00.000Z', endedAt: '', activeSeconds: 0,
  activeMs: 0, asOf: '2026-11-15T10:00:00.000Z',
})
const VIDEO = { id: 1, kind: 'video', provider: 'igdb', path: 'v', thumbPath: 't', url: 'http://m/v.mp4', thumbUrl: 'http://m/t.png' }

let app: Record<string, ReturnType<typeof vi.fn>>
function install() {
  app = {
    ListGames: vi.fn(({ filter, offset, limit }: { filter: { platformIds?: number[] }; offset: number; limit: number }) => {
      const rows = filter.platformIds ? ALL.filter((x) => filter.platformIds!.includes(x.platformId)) : ALL
      return Promise.resolve({ games: rows.slice(offset, offset + limit), total: rows.length, offset })
    }),
    GetGame: vi.fn((id: number) => Promise.resolve(detailOf(id))),
    Catalog: vi.fn(() => Promise.resolve(CATALOG)),
    MediaList: vi.fn(() => Promise.resolve({ media: [] })),
    CurrentSession: vi.fn(() => Promise.resolve(null)),
    LaunchGame: vi.fn(() => Promise.resolve(session())),
    LaunchGameUntracked: vi.fn(() => Promise.resolve()),
    BringToFront: vi.fn(() => Promise.resolve()),
    TimeToBeatGet: vi.fn(() => Promise.resolve(null)),
  }
  const proxy = new Proxy(app, {
    get: (t, name: string) => (typeof name === 'string' ? (...a: unknown[]) => (t[name] ? t[name](...a) : Promise.resolve(undefined)) : undefined),
  })
  const w = window as unknown as { go?: unknown; runtime?: unknown }
  w.go = { main: { App: proxy } }
  w.runtime = {
    EventsOn: vi.fn(() => () => {}), WindowIsFullscreen: vi.fn(() => Promise.resolve(false)), WindowFullscreen: vi.fn(), WindowUnfullscreen: vi.fn(),
  }
  vi.stubGlobal('ResizeObserver', class { observe() {} unobserve() {} disconnect() {} })
  vi.spyOn(HTMLMediaElement.prototype, 'play').mockResolvedValue(undefined)
  vi.spyOn(HTMLMediaElement.prototype, 'pause').mockImplementation(() => {})
}

function mount(view: BigBoxProps['libraryView'] = 'grid') {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const Events = ({ children }: { children: ReactNode }) => {
    useMediaEvents()
    useTimeToBeatEvents()
    return <>{children}</>
  }
  return render(
    <BigBox initialPlatformId={null} initialGameId={null} themeId={'desktop' as BigBoxProps['themeId']} librarySort="title"
      libraryView={view} onExit={vi.fn()} />,
    { wrapper: ({ children }) => <QueryClientProvider client={qc}><Events>{children}</Events></QueryClientProvider> },
  )
}
const key = (k: string) => fireEvent.keyDown(window, { key: k })
const settle = () => act(async () => { await new Promise((r) => setTimeout(r, 20)) })
const ready = async () => { await screen.findByRole('listbox', { name: 'Jogos' }); await settle() }
const never = () => new Promise<never>(() => {})

type Scenario = { name: string; run: () => Promise<void>; view?: BigBoxProps['libraryView'] }
const SCENARIOS: Scenario[] = [
  { name: 'Jogos em grade', run: ready },
  { name: 'Jogos em lista', run: ready, view: 'list' },
  { name: 'Plataformas', run: async () => { await ready(); key('ArrowLeft'); await settle() } },
  { name: 'Painel no botão', run: async () => { await ready(); key('d'); await settle() } },
  { name: 'Painel no visor', run: async () => { await ready(); key('d'); await settle(); key('ArrowDown'); key('ArrowDown'); await settle() } },
  { name: 'aba vazia', run: async () => { app.ListGames.mockImplementation(() => Promise.resolve({ games: [], total: 0, offset: 0 })); await settle() } },
  { name: 'primeira página carregando', run: async () => { app.ListGames.mockImplementation(never); await settle() } },
  { name: 'detalhe carregando', run: async () => { app.GetGame.mockImplementation(never); await ready() } },
  {
    name: 'aviso de erro',
    run: async () => {
      app.LaunchGame.mockImplementation(() => Promise.reject(new Error('Já há um jogo em execução.')))
      await ready(); key('Enter'); await screen.findByRole('alert')
    },
  },
  { name: 'sessão aberta', run: async () => { app.CurrentSession.mockImplementation(() => Promise.resolve(session())); await ready() } },
  {
    name: 'vídeo tocando',
    run: async () => {
      app.MediaList.mockImplementation(() => Promise.resolve({ media: [VIDEO] }))
      await ready()
      await waitFor(() => expect(document.querySelector('video')).not.toBeNull(), { timeout: 4000 })
    },
  },
]

beforeEach(install)
afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
  const w = window as unknown as { go?: unknown; runtime?: unknown }
  delete w.go
  delete w.runtime
})

describe.each(SCENARIOS)('Big Box Desktop DOM: $name', ({ name, run, view }) => {
  const open = async () => {
    const r = mount(view)
    await run()
    return r
  }

  it('TDK-038/os únicos acionáveis do DOM do tema são os itens de plataforma (um por aba) e os dois botões do painel', async () => {
    const { container } = await open()
    const nav = screen.getByRole('navigation', { name: 'Plataformas' })
    const aside = container.querySelector('aside[aria-label="Detalhes do jogo"]') as HTMLElement
    const root = container.querySelector('[data-bigbox-theme="desktop"]') as HTMLElement
    expect(root, name).not.toBeNull()
    const found = Array.from(root.querySelectorAll(ACTIONABLE))
    const inNav = found.filter((e) => nav.contains(e))
    const inAside = found.filter((e) => aside.contains(e))
    expect(inNav.length + inAside.length, `${name}: controle fora da lista permitida`).toBe(found.length)
    expect(inNav).toHaveLength(3) // Todos + Windows + PlayStation 2, one per tab (BBX-065)
    const names = inAside.map((e) => e.textContent?.trim())
    expect(names === undefined || names.length === 0 || (names.length === 2 && names[0] === 'Jogar' && names[1] === 'Jogar sem contar tempo')).toBe(true)
  })

  it('TDK-038/cartões e linhas são role="option" e o <video> não tem controls', async () => {
    const { container } = await open()
    const list = screen.queryByRole('listbox', { name: 'Jogos' })
    if (list) {
      list.querySelectorAll('[data-testid="bb-cover"], li, [role="row"]').forEach((e) => expect(e.getAttribute('role'), name).toBe('option'))
      within(list).queryAllByRole('option').forEach((o) => expect(o.matches('button, a[href]')).toBe(false))
    }
    container.querySelectorAll('video').forEach((v) => {
      expect(v.hasAttribute('controls')).toBe(false)
      expect(v.controls).toBe(false)
    })
  })

  it('TDK-038/nenhum elemento tem nome acessível ou texto igual a um dos rótulos proibidos', async () => {
    const { container } = await open()
    for (const label of FORBIDDEN) {
      expect(screen.queryByText(label), `texto "${label}" em ${name}`).toBeNull()
      expect(screen.queryByRole('button', { name: label }), `botão "${label}" em ${name}`).toBeNull()
      expect(screen.queryByRole('link', { name: label }), `link "${label}" em ${name}`).toBeNull()
      expect(screen.queryByRole('menuitem', { name: label }), `menuitem "${label}" em ${name}`).toBeNull()
      expect(container.querySelector(`[aria-label="${label}"], [title="${label}"]`), `aria-label "${label}" em ${name}`).toBeNull()
    }
    // "Plataformas" only as the text of the left column title (the nav landmark), never as a control name
    expect(screen.queryByRole('button', { name: 'Plataformas' })).toBeNull()
    expect(screen.getByRole('navigation', { name: 'Plataformas' })).toBeInTheDocument()
  })

  it('TDK-002/o cabeçalho do tema não tem acionável', async () => {
    const { container } = await open()
    const header = container.querySelector('[data-bigbox-theme="desktop"] header') as HTMLElement
    expect(header, name).not.toBeNull()
    expect(header).toHaveTextContent('Arcadia')
    expect(header.querySelector(ACTIONABLE)).toBeNull()
  })
})
