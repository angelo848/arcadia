import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { GameDetailsPanel } from './GameDetailsPanel'
import type { SessionView } from '../lib/games'
import { gameNotFoundMessage } from '../lib/library'
import type { CatalogView, GameDetailView, PlatformView, SessionItemView, SessionListView } from '../lib/library'
import type { MediaItemView } from '../lib/metadata'
import { platformIconSrc } from '../lib/platforms'

type Props = Parameters<typeof GameDetailsPanel>[0]

const platformWin: PlatformView = {
  id: 1,
  name: 'Windows',
  category: 'pc',
  emulatorId: null,
  seedKey: 'windows',
  iconUrl: '',
  gameCount: 3,
}

type Fakes = {
  GetGame: ReturnType<typeof vi.fn>
  GameSessions: ReturnType<typeof vi.fn>
  Catalog: ReturnType<typeof vi.fn>
  MediaList: ReturnType<typeof vi.fn>
}

let app: Fakes

type InstallOpts = {
  detail?: GameDetailView | ((id: number) => Promise<GameDetailView>)
  platforms?: PlatformView[]
  media?: MediaItemView[]
  sessions?: SessionItemView[]
}

function install(opts: InstallOpts = {}): Fakes {
  const d = opts.detail ?? detail()
  const all = opts.sessions ?? []
  const catalog: CatalogView = { platforms: opts.platforms ?? [platformWin], genres: [], companies: [], playModes: [] }
  app = {
    GetGame: vi.fn((id: number) => (typeof d === 'function' ? d(id) : Promise.resolve(d))),
    GameSessions: vi.fn(
      (_id: number, offset: number, limit: number): Promise<SessionListView> =>
        Promise.resolve({ sessions: all.slice(offset, offset + limit), total: all.length, offset }),
    ),
    Catalog: vi.fn(() => Promise.resolve(catalog)),
    MediaList: vi.fn(() => Promise.resolve({ media: opts.media ?? [] })),
  }
  ;(window as unknown as { go?: unknown }).go = { main: { App: app } }
  return app
}

function detail(over: Partial<GameDetailView> = {}): GameDetailView {
  return {
    id: 10,
    platformId: 1,
    title: 'Hollow Knight',
    platform: 'Windows',
    path: 'C:\\Games\\HK\\hollow_knight.exe',
    status: 'playing',
    favorite: true,
    year: 2017,
    totalPlaySeconds: 3900,
    playCount: 4,
    firstPlayedAt: '',
    lastPlayedAt: '',
    description: 'Um metroidvania difícil.',
    completedAt: null,
    developers: ['Team Cherry'],
    publishers: ['Team17'],
    genres: ['Metroidvania', 'Plataforma'],
    playModes: ['single'],
    launchType: 'exe',
    launchArgs: '',
    sessionPlaySeconds: 0, // alterado em 2026-11-25 (fase 16)
    emulatorId: null,
    ...over,
  }
}

function sessionView(over: Partial<SessionView> = {}): SessionView {
  return {
    sessionId: 1,
    gameId: 10,
    title: 'Hollow Knight',
    state: 'running',
    startedAt: '2026-11-09T10:00:00.000Z',
    endedAt: '',
    activeSeconds: 252,
    activeMs: 252400,
    asOf: '2026-11-09T10:04:12.400Z',
    ...over,
  }
}

function sessionItem(id: number): SessionItemView {
  return {
    sessionId: id,
    gameId: 10,
    state: 'ended',
    startedAt: '2026-11-09T10:00:00.000Z',
    endedAt: '2026-11-09T11:00:00.000Z',
    activeSeconds: 3600,
    endReason: 'exited',
    source: 'tracked', // alterado em 2026-11-25 (fase 16)
    editedAt: '', // alterado em 2026-11-25 (fase 16)
  }
}

function media(id: number, kind: MediaItemView['kind']): MediaItemView {
  return {
    id,
    kind,
    provider: 'igdb',
    path: `p${id}`,
    thumbPath: `t${id}`,
    url: `/media/${id}.jpg`,
    thumbUrl: `/media/${id}-t.jpg`,
  }
}

function newClient(): QueryClient {
  return new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
}

function wrapperFor(qc: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  }
}

function makeProps(over: Partial<Props> = {}): Props {
  return {
    gameId: 10,
    current: null,
    pending: null,
    onPlay: vi.fn(),
    onPlayUntracked: vi.fn(),
    onEdit: vi.fn(),
    onSearchMetadata: vi.fn(),
    onRemove: vi.fn(),
    onSelectionEvent: vi.fn(),
    ...over,
  }
}

function renderPanel(over: Partial<Props> = {}) {
  const props = makeProps(over)
  const Wrapper = wrapperFor(newClient())
  const utils = render(<GameDetailsPanel {...props} />, { wrapper: Wrapper })
  return { ...utils, props }
}

async function loaded(): Promise<void> {
  await screen.findByRole('heading', { level: 2, name: 'Hollow Knight' })
}

function expectLocalDate(text: string, iso: string) {
  const d = new Date(iso)
  expect(text).toContain(String(d.getFullYear()))
  expect(text).toMatch(new RegExp(`(^|\\D)0?${d.getDate()}(\\D|$)`))
}

function precedes(a: Node, b: Node): boolean {
  return (a.compareDocumentPosition(b) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0
}

async function settle() {
  await act(async () => {
    await Promise.resolve()
  })
}

afterEach(() => {
  delete (window as unknown as { go?: unknown }).go
})

describe('GameDetailsPanel', () => {
  it('UIX-025/cabeçalho com título, ícone e nome da plataforma', async () => {
    install()
    renderPanel()
    const title = await screen.findByRole('heading', { level: 2, name: 'Hollow Knight' })

    expect(await screen.findByText('Windows')).toBeInTheDocument()
    const icon = Array.from(document.querySelectorAll('img')).find((i) => i.getAttribute('src') === platformIconSrc(platformWin))
    expect(icon).toBeDefined()
    expect(icon).toHaveAttribute('alt', '')
    expect(precedes(title, icon as Element) || precedes(icon as Element, title)).toBe(true)
  })

  it('UIX-025/sessão running ou grace do jogo mostra "Em execução"; paused mostra "Pausado"; sessão de outro jogo, nada', async () => {
    const cases: Array<[SessionView['state'], string]> = [
      ['running', 'Em execução'],
      ['grace', 'Em execução'],
      ['paused', 'Pausado'],
    ]
    for (const [state, label] of cases) {
      install()
      const { unmount } = renderPanel({ current: sessionView({ state }) })
      await loaded()
      expect(screen.getByText(label), state).toBeInTheDocument()
      unmount()
    }

    install()
    renderPanel({ current: sessionView({ gameId: 99, state: 'running' }) })
    await loaded()
    expect(screen.queryByText('Em execução')).toBeNull()
    expect(screen.queryByText('Pausado')).toBeNull()
  })

  it('UIX-025/sessão por processo adotado mostra o aviso de rastreio por processo', async () => {
    install()
    renderPanel({ current: sessionView({ tracking: 'process' }) })
    expect(await screen.findByText(/Rastreio por processo/)).toBeInTheDocument()
  })

  it('UIX-025/jogo de emulador sem emulador próprio nem padrão mostra o aviso de EMU-003; com padrão, não', async () => {
    install({ detail: detail({ launchType: 'emulator', emulatorId: null }) })
    const { unmount } = renderPanel()
    expect(await screen.findByText(/Sem emulador configurado/)).toBeInTheDocument()
    unmount()

    install({
      detail: detail({ launchType: 'emulator', emulatorId: null }),
      platforms: [{ ...platformWin, emulatorId: 5 }],
    })
    renderPanel()
    await loaded()
    await waitFor(() => expect(app.Catalog).toHaveBeenCalled())
    await settle()
    expect(screen.queryByText(/Sem emulador configurado/)).toBeNull()
  })

  it('UIX-026/carrossel com as imagens de MediaList do jogo', async () => {
    install({ media: [media(1, 'screenshot'), media(2, 'cover')] })
    renderPanel()
    await loaded()

    const region = await screen.findByRole('region', { name: 'Imagens' })
    await waitFor(() => expect(app.MediaList).toHaveBeenCalledWith(10))
    // The cover comes first in the carousel.
    await waitFor(() => expect(region.querySelector('img')).toHaveAttribute('src', '/media/2.jpg'))
    expect(within(region).getByText(/1 de 2/)).toBeInTheDocument()
  })

  // alterado em 2026-11-17 (1.0.3): o vídeo aparece no carrossel da janela normal (VID-023)
  it('VID-023/jogo só com vídeo: o carrossel mostra o vídeo e o conta', async () => {
    install({ media: [media(1, 'video')] })
    renderPanel()
    await loaded()
    const region = await screen.findByRole('region', { name: 'Imagens' })
    await waitFor(() => expect(region.querySelector('video')).toHaveAttribute('src', '/media/1.jpg'))
    expect(within(region).getByText(/1 de 1/)).toBeInTheDocument()
  })

  it('VID-023/jogo com capa e vídeo: a capa vem primeiro e o vídeo é o segundo item', async () => {
    install({ media: [media(1, 'video'), media(2, 'cover')] })
    renderPanel()
    await loaded()
    const region = await screen.findByRole('region', { name: 'Imagens' })
    await waitFor(() => expect(region.querySelector('img')).toHaveAttribute('src', '/media/2.jpg'))
    expect(within(region).getByText(/1 de 2/)).toBeInTheDocument()
    expect(region.querySelector('video')).toBeNull()
    fireEvent.click(within(region).getByRole('button', { name: /Vídeo, 2 de 2/ }))
    await waitFor(() => expect(region.querySelector('video')).toHaveAttribute('src', '/media/1.jpg'))
  })

  it('UIX-027/Jogar chama onPlay(G) uma vez e Jogar sem contar tempo chama onPlayUntracked(G) uma vez', async () => {
    install()
    const { props } = renderPanel()
    await loaded()

    fireEvent.click(screen.getByRole('button', { name: 'Jogar' }))
    expect(props.onPlay).toHaveBeenCalledTimes(1)
    expect(props.onPlay).toHaveBeenCalledWith(10)
    expect(props.onPlayUntracked).not.toHaveBeenCalled()

    fireEvent.click(screen.getByRole('button', { name: 'Jogar sem contar tempo' }))
    expect(props.onPlayUntracked).toHaveBeenCalledTimes(1)
    expect(props.onPlayUntracked).toHaveBeenCalledWith(10)
    expect(props.onPlay).toHaveBeenCalledTimes(1)
  })

  async function buttonsAfterCarousel(): Promise<HTMLButtonElement[]> {
    await loaded()
    const region = await screen.findByRole('region', { name: 'Imagens' })
    return Array.from(document.querySelectorAll('button')).filter(
      (b) => !region.contains(b) && precedes(region, b),
    )
  }

  const classesOf = (el: Element) => Array.from(el.classList)

  it('AJU-006/"Jogar" é o primeiro botão depois do carrossel, com w-full, min-h-12, text-lg, font-bold, bg-play, hover:bg-play-hover e text-on-accent', async () => {
    install({ media: [media(1, 'cover')] })
    renderPanel()
    const buttons = await buttonsAfterCarousel()

    expect(buttons[0]).toHaveTextContent('Jogar')
    expect(buttons[0]).toHaveAccessibleName('Jogar')
    expect(classesOf(buttons[0])).toEqual(
      expect.arrayContaining(['w-full', 'min-h-12', 'text-lg', 'font-bold', 'bg-play', 'hover:bg-play-hover', 'text-on-accent']),
    )
    // alterado em 2026-11-15 (fase 13): sem data-bigbox-play / data-bigbox-item (BBX-003)
  })

  it('AJU-006/com pendência continua "Iniciando…" e desativado', async () => {
    install({ media: [media(1, 'cover')] })
    renderPanel({ pending: { id: 10, mode: 'tracked' } })
    const buttons = await buttonsAfterCarousel()

    expect(buttons[0]).toHaveTextContent('Iniciando…')
    expect(buttons[0]).toBeDisabled()
  })

  it('AJU-007/"Jogar sem contar tempo" vem logo depois, com w-full, min-h-10, text-base, bg-raised, hover:bg-hover, border border-play e text-fg', async () => {
    install({ media: [media(1, 'cover')] })
    renderPanel()
    const buttons = await buttonsAfterCarousel()

    expect(buttons[1]).toHaveTextContent('Jogar sem contar tempo')
    expect(classesOf(buttons[1])).toEqual(
      expect.arrayContaining(['w-full', 'min-h-10', 'text-base', 'bg-raised', 'hover:bg-hover', 'border', 'border-play', 'text-fg']),
    )
  })

  it('AJU-007/Editar, Buscar metadados e Remover vêm depois, numa linha, sem w-full', async () => {
    install({ media: [media(1, 'cover')] })
    renderPanel()
    const buttons = await buttonsAfterCarousel()

    const names = ['Editar', 'Buscar metadados', 'Remover']
    const rest = names.map((n) => screen.getByRole('button', { name: n }))
    for (const b of rest) {
      expect(precedes(buttons[1], b)).toBe(true)
      expect(classesOf(b)).not.toContain('w-full')
    }
    expect(rest[0].parentElement).toBe(rest[1].parentElement)
    expect(rest[1].parentElement).toBe(rest[2].parentElement)
    expect(rest[0].parentElement).not.toBe(buttons[1].parentElement)
  })

  it('UIX-027/pendência de G desativa os dois e o acionado mostra "Iniciando…"', async () => {
    install()
    const { props } = renderPanel({ pending: { id: 10, mode: 'untracked' } })
    await loaded()

    const untracked = screen.getByText('Iniciando…').closest('button') as HTMLButtonElement
    expect(untracked).toHaveTextContent('Iniciando…')
    expect(untracked).toBeDisabled()
    const play = screen.getByRole('button', { name: 'Jogar' })
    expect(play).toBeDisabled()
    expect(screen.getAllByText('Iniciando…')).toHaveLength(1)
    fireEvent.click(play)
    fireEvent.click(untracked)
    expect(props.onPlay).not.toHaveBeenCalled()
    expect(props.onPlayUntracked).not.toHaveBeenCalled()
  })

  it('UIX-027/pendência de outro jogo deixa os botões de G ativos', async () => {
    install()
    renderPanel({ pending: { id: 99, mode: 'tracked' } })
    await loaded()

    expect(screen.getByRole('button', { name: 'Jogar' })).toBeEnabled()
    expect(screen.getByRole('button', { name: 'Jogar sem contar tempo' })).toBeEnabled()
    expect(screen.queryByText('Iniciando…')).toBeNull()
  })

  it('UIX-028/tempo total, primeira e última vez (ou "Nunca") e "Concluído em" em UTC, como vêm de GetGame', async () => {
    install()
    const { unmount } = renderPanel()
    await loaded()
    expect(screen.getByText('1 h 05 min')).toBeInTheDocument()
    expect(screen.getAllByText('Nunca')).toHaveLength(2)
    unmount()

    const first = '2026-11-01T10:00:00.000Z'
    const last = '2026-11-09T10:00:00.000Z'
    install({ detail: detail({ firstPlayedAt: first, lastPlayedAt: last, completedAt: '2026-11-12' }) })
    renderPanel()
    await loaded()
    expect(screen.queryByText('Nunca')).toBeNull()
    const text = document.body.textContent ?? ''
    expectLocalDate(text, first)
    expectLocalDate(text, last)
    expect(screen.getByText(/2026-11-12/)).toBeInTheDocument()
  })

  it('UIX-029/status, favorito, gêneros, modos, desenvolvedora, editora, ano e descrição inteira; "—" para vazios', async () => {
    const long = 'Descrição longa. '.repeat(60).trim()
    install({ detail: detail({ description: long }) })
    const { unmount } = renderPanel()
    await loaded()

    expect(screen.getByText('Jogando')).toBeInTheDocument()
    expect(screen.getByText(/favorito/i)).toBeInTheDocument()
    expect(screen.getByText('Metroidvania')).toBeInTheDocument()
    expect(screen.getByText('Plataforma')).toBeInTheDocument()
    expect(screen.getByText('Um jogador')).toBeInTheDocument()
    expect(screen.getByText('Team Cherry')).toBeInTheDocument()
    expect(screen.getByText('Team17')).toBeInTheDocument()
    expect(screen.getByText('2017')).toBeInTheDocument()
    expect(screen.getByText(long)).toBeInTheDocument()
    unmount()

    install({
      detail: detail({ genres: [], playModes: [], developers: [], publishers: [], year: null, description: '' }),
    })
    renderPanel()
    await loaded()
    expect(screen.getAllByText('—').length).toBeGreaterThanOrEqual(5)
  })

  it('UIX-030/seção Sessões com as sessões do jogo e "Carregar mais"', async () => {
    install({ sessions: Array.from({ length: 60 }, (_, i) => sessionItem(i + 1)) })
    renderPanel()
    await loaded()

    expect(screen.getByRole('heading', { name: 'Sessões' })).toBeInTheDocument()
    await screen.findByRole('button', { name: 'Carregar mais' })
    expect(app.GameSessions).toHaveBeenNthCalledWith(1, 10, 0, 50)
    fireEvent.click(screen.getByRole('button', { name: 'Carregar mais' }))
    await waitFor(() => expect(app.GameSessions).toHaveBeenCalledTimes(2))
  })

  // alterado em 2026-11-25 (fase 16): a lista de sessões recebe o jogo da página.
  it('SEL-004/o painel passa o jogo (total e sessionPlaySeconds) para a lista de sessões', async () => {
    // total 3900 s, sessions 0 s: adjustment of 1 h 05 min offered in the create form (SEL-009).
    install({ sessions: [] })
    const { unmount } = renderPanel()
    await loaded()
    fireEvent.click(await screen.findByRole('button', { name: 'Adicionar sessão' }))
    expect(await screen.findByLabelText('Já está no tempo total')).toBeInTheDocument()
    expect(screen.getByText(/1 h 05 min a mais que as sessões/)).toBeInTheDocument()
    unmount()

    // total equal to the sessions: no adjustment, no checkbox.
    install({ detail: detail({ sessionPlaySeconds: 3900 }), sessions: [] })
    renderPanel()
    await loaded()
    fireEvent.click(await screen.findByRole('button', { name: 'Adicionar sessão' }))
    await screen.findByRole('dialog', { name: 'Adicionar sessão' })
    expect(screen.queryByLabelText('Já está no tempo total')).toBeNull()
  })

  it('UIX-035/sem resposta mostra "Carregando jogo…" e nada do jogo anterior', async () => {
    install({
      detail: (id) =>
        id === 10 ? Promise.resolve(detail()) : new Promise<GameDetailView>(() => {}),
    })
    const props = makeProps()
    const Wrapper = wrapperFor(newClient())
    const { rerender } = render(<GameDetailsPanel {...props} />, { wrapper: Wrapper })
    await loaded()

    rerender(<GameDetailsPanel {...props} gameId={11} />)
    expect(await screen.findByText('Carregando jogo…')).toBeInTheDocument()
    expect(screen.queryByText('Hollow Knight')).toBeNull()
    expect(screen.queryByRole('button', { name: 'Jogar' })).toBeNull()
  })

  it('UIX-035/erro diferente de -32001 mostra role=alert e "Tentar de novo", que relê GetGame e emite retry', async () => {
    const message = 'O agente não está respondendo. Tente de novo.'
    install({ detail: () => Promise.reject(new Error(message)) })
    const { props } = renderPanel()

    const alert = await screen.findByRole('alert')
    expect(alert).toHaveTextContent(message)
    expect(props.onSelectionEvent).toHaveBeenCalledWith(expect.objectContaining({ type: 'readError', gameId: 10 }))
    expect(props.onSelectionEvent).not.toHaveBeenCalledWith({ type: 'readMissing', gameId: 10 })
    const calls = app.GetGame.mock.calls.length

    fireEvent.click(screen.getByRole('button', { name: 'Tentar de novo' }))
    expect(props.onSelectionEvent).toHaveBeenCalledWith({ type: 'retry' })
    await waitFor(() => expect(app.GetGame.mock.calls.length).toBeGreaterThan(calls))
  })

  it('UIX-023/-32001 emite readMissing(G)', async () => {
    install({ detail: () => Promise.reject(gameNotFoundMessage) })
    const { props } = renderPanel()

    await waitFor(() => expect(props.onSelectionEvent).toHaveBeenCalledWith({ type: 'readMissing', gameId: 10 }))
    expect(props.onSelectionEvent).not.toHaveBeenCalledWith(expect.objectContaining({ type: 'readError' }))
  })

  it('UIX-035/leitura ok emite readOk(G)', async () => {
    install()
    const { props } = renderPanel()
    await loaded()

    expect(props.onSelectionEvent).toHaveBeenCalledWith({ type: 'readOk', gameId: 10 })
  })

  it('UIX-036/trocar gameId volta o scrollTop do painel a 0', async () => {
    install({
      detail: (id) => Promise.resolve(detail({ id, title: id === 10 ? 'Hollow Knight' : 'Celeste' })),
    })
    const props = makeProps()
    const Wrapper = wrapperFor(newClient())
    const { container, rerender } = render(<GameDetailsPanel {...props} />, { wrapper: Wrapper })
    await loaded()

    // The root is the element that scrolls; jsdom does not lay out, so scrollTop is faked.
    const root = container.firstElementChild as HTMLElement
    let top = 120
    Object.defineProperty(root, 'scrollTop', {
      configurable: true,
      get: () => top,
      set: (v: number) => {
        top = v
      },
    })
    expect(root.className).toContain('overflow-y-auto')

    rerender(<GameDetailsPanel {...props} gameId={11} />)
    expect(container.firstElementChild).toBe(root)
    expect(top).toBe(0)
  })

  // T30. Premissa: o botão se chama "Buscar vídeo"; credenciais vêm de ProviderKeyStatus (screenscraper e
  // screenscraperdev); o resultado e o erro aparecem como texto no painel.
  function installVideo(
    creds: [boolean, boolean],
    videoQueue: ReturnType<typeof vi.fn>,
    items: MediaItemView[] = [],
  ): Fakes & { ProviderKeyStatus: ReturnType<typeof vi.fn> } {
    const a = install({ media: items })
    const extra = {
      ProviderKeyStatus: vi.fn((p: string) =>
        Promise.resolve({ provider: p, configured: p === 'screenscraper' ? creds[0] : creds[1] }),
      ),
      VideoQueue: videoQueue,
    }
    Object.assign(a, extra)
    return Object.assign(a, { ProviderKeyStatus: extra.ProviderKeyStatus })
  }
  const videoButton = () => screen.queryByRole('button', { name: 'Buscar vídeo' })

  it('VID-024/com as duas credenciais e sem vídeo o botão aparece', async () => {
    installVideo([true, true], vi.fn())
    renderPanel()
    await loaded()
    expect(await screen.findByRole('button', { name: 'Buscar vídeo' })).toBeEnabled()
  })

  it('VID-024/sem credenciais, ou com vídeo, não aparece', async () => {
    for (const [creds, items] of [
      [[true, false], []],
      [[false, true], []],
      [[true, true], [media(1, 'video')]],
    ] as Array<[[boolean, boolean], MediaItemView[]]>) {
      const a = installVideo(creds, vi.fn(), items)
      const { unmount } = renderPanel()
      await loaded()
      await waitFor(() => expect(a.MediaList).toHaveBeenCalled())
      await waitFor(() => expect(a.ProviderKeyStatus).toHaveBeenCalled())
      await settle()
      expect(videoButton()).toBeNull()
      unmount()
    }
  })

  it('VID-024/chama VideoQueue([id]) uma vez e mostra "Vídeo na fila"', async () => {
    const q = vi.fn(() => Promise.resolve({ queued: 1, skipped: [] }))
    installVideo([true, true], q)
    renderPanel()
    await loaded()
    fireEvent.click(await screen.findByRole('button', { name: 'Buscar vídeo' }))
    expect(await screen.findByText('Vídeo na fila')).toBeInTheDocument()
    expect(q).toHaveBeenCalledTimes(1)
    expect(q).toHaveBeenCalledWith([10])
  })

  it('VID-024/mostra "Este jogo já tem vídeo ou busca em andamento" e "A plataforma não tem vídeo no ScreenScraper"', async () => {
    const cases: Array<['has-video' | 'platform-unmapped', string]> = [
      ['has-video', 'Este jogo já tem vídeo ou busca em andamento'],
      ['platform-unmapped', 'A plataforma não tem vídeo no ScreenScraper'],
    ]
    for (const [reason, text] of cases) {
      installVideo([true, true], vi.fn(() => Promise.resolve({ queued: 0, skipped: [{ gameId: 10, reason }] })))
      const { unmount } = renderPanel()
      await loaded()
      fireEvent.click(await screen.findByRole('button', { name: 'Buscar vídeo' }))
      expect(await screen.findByText(text)).toBeInTheDocument()
      unmount()
    }
  })

  it('VID-024/erro do agente mostra a mensagem', async () => {
    const message = 'O agente não está respondendo. Tente de novo.'
    installVideo([true, true], vi.fn(() => Promise.reject(new Error(message))))
    renderPanel()
    await loaded()
    fireEvent.click(await screen.findByRole('button', { name: 'Buscar vídeo' }))
    expect(await screen.findByText(message)).toBeInTheDocument()
  })

  it('VID-024/o botão fica desativado durante a chamada', async () => {
    let done: (v: { queued: number; skipped: [] }) => void = () => {}
    const q = vi.fn(() => new Promise<{ queued: number; skipped: [] }>((r) => (done = r)))
    installVideo([true, true], q)
    renderPanel()
    await loaded()
    const btn = await screen.findByRole('button', { name: 'Buscar vídeo' })
    fireEvent.click(btn)
    await waitFor(() => expect(btn).toBeDisabled())
    fireEvent.click(btn)
    expect(q).toHaveBeenCalledTimes(1)
    await act(async () => {
      done({ queued: 1, skipped: [] })
    })
    await screen.findByText('Vídeo na fila')
  })

  it('1.0.3/"Abrir local do arquivo" fica entre Buscar metadados e Remover e abre o local do jogo', async () => {
    const a = install()
    const OpenGameLocation = vi.fn(() => Promise.resolve())
    Object.assign(a, { OpenGameLocation })
    renderPanel()
    await loaded()

    const names = Array.from(screen.getByRole('button', { name: 'Remover' }).parentElement?.querySelectorAll('button') ?? []).map(
      (b) => b.textContent,
    )
    expect(names.indexOf('Abrir local do arquivo')).toBe(names.indexOf('Remover') - 1)
    expect(names.indexOf('Buscar metadados')).toBeLessThan(names.indexOf('Abrir local do arquivo'))
    fireEvent.click(screen.getByRole('button', { name: 'Abrir local do arquivo' }))
    await waitFor(() => expect(OpenGameLocation).toHaveBeenCalledWith(10))
  })

  it('1.0.3/sem pasta conhecida o aviso do agente aparece no painel', async () => {
    const a = install()
    Object.assign(a, { OpenGameLocation: vi.fn(() => Promise.reject(new Error('Este jogo não tem uma pasta conhecida.'))) })
    renderPanel()
    await loaded()

    fireEvent.click(screen.getByRole('button', { name: 'Abrir local do arquivo' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('Este jogo não tem uma pasta conhecida.')
  })

  it('UIX-025/Editar, Buscar metadados e Remover chamam os callbacks uma vez', async () => {
    install()
    const { props } = renderPanel()
    await loaded()

    fireEvent.click(screen.getByRole('button', { name: 'Editar' }))
    expect(props.onEdit).toHaveBeenCalledTimes(1)
    fireEvent.click(screen.getByRole('button', { name: 'Buscar metadados' }))
    expect(props.onSearchMetadata).toHaveBeenCalledTimes(1)
    fireEvent.click(screen.getByRole('button', { name: 'Remover' }))
    expect(props.onRemove).toHaveBeenCalledTimes(1)
    expect(props.onEdit).toHaveBeenCalledTimes(1)
    expect(props.onPlay).not.toHaveBeenCalled()
  })
})

// alterado em 2026-11-22 (fase 15): acréscimos do bloco "Tempo para zerar" (TTB-013, TTB-015, TTB-039).
// Premissa: o painel passa o seu onEdit ao bloco, que o chama com { section: 'timeToBeat' } em "Confirmar…"
// (o botão "Editar" continua chamando onEdit() sem argumentos).
describe('GameDetailsPanel: tempo para zerar', () => {
  const nulls = { mainSeconds: null, mainExtraSeconds: null, completionistSeconds: null }
  function ttbView(over: Record<string, unknown> = {}) {
    return {
      gameId: 10,
      state: 'found',
      source: 'hltb',
      externalId: '1',
      externalTitle: 'Hollow Knight',
      mainSeconds: 97200,
      mainExtraSeconds: 5400,
      completionistSeconds: 99000,
      fetchedAt: '2026-11-20T12:00:00.000Z',
      confirmed: false,
      stale: false,
      pending: false,
      unavailable: false,
      candidates: [],
      ...over,
    }
  }
  function installTtb(get: (id: number, fetch: boolean) => Promise<unknown>, enabled = true) {
    const a = install()
    Object.assign(a, {
      TimeToBeatGet: vi.fn(get),
      TimeToBeatSettings: vi.fn(() => Promise.resolve({ enabled, afterMetadata: true })),
    })
    return a as Fakes & { TimeToBeatGet: ReturnType<typeof vi.fn> }
  }

  it('TTB-015/o bloco "Tempo para zerar" vem depois de "Tempo total", com os três rótulos, valores e a fonte', async () => {
    installTtb(() => Promise.resolve(ttbView()))
    renderPanel()
    await loaded()
    const block = await screen.findByText('Tempo para zerar')
    expect(precedes(screen.getByText('Tempo total'), block)).toBe(true)
    for (const l of ['História principal', 'História + extras', '100%']) expect(screen.getByText(l)).toBeInTheDocument()
    expect(await screen.findByText('27 h')).toBeInTheDocument()
    expect(screen.getByText('Fonte: HowLongToBeat')).toBeInTheDocument()
  })

  it('TTB-015/ao mostrar o jogo lê o cache (fetch false) com o id do jogo', async () => {
    const a = installTtb(() => Promise.resolve(ttbView()))
    renderPanel()
    await loaded()
    await waitFor(() => expect(a.TimeToBeatGet).toHaveBeenCalledWith(10, false))
  })

  it('TTB-015/em doubt "Confirmar…" chama onEdit({ section: "timeToBeat" }) e "Editar" segue chamando onEdit()', async () => {
    installTtb(() => Promise.resolve(ttbView({ state: 'doubt', ...nulls })))
    const { props } = renderPanel()
    await loaded()
    expect(await screen.findByText('Correspondência incerta')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Confirmar…' }))
    expect(props.onEdit).toHaveBeenCalledTimes(1)
    expect(props.onEdit).toHaveBeenCalledWith({ section: 'timeToBeat' })
    fireEvent.click(screen.getByRole('button', { name: 'Editar' }))
    expect(props.onEdit).toHaveBeenCalledTimes(2)
    expect(props.onEdit).toHaveBeenLastCalledWith()
  })

  it('TTB-015/serviço desligado e sem cache: o painel não mostra o bloco', async () => {
    installTtb(() => Promise.resolve(ttbView({ state: 'none', source: '', ...nulls })), false)
    renderPanel()
    await loaded()
    await waitFor(() => expect(app.MediaList).toHaveBeenCalled())
    await settle()
    expect(screen.queryByText('Tempo para zerar')).toBeNull()
  })

  it('TTB-013/binding que rejeita: o painel segue inteiro, com os rótulos em "—", sem alert nem modal', async () => {
    installTtb(() => Promise.reject(new Error('boom')))
    renderPanel()
    await loaded()
    await settle()
    for (const l of ['História principal', 'História + extras', '100%']) expect(screen.getByText(l)).toBeInTheDocument()
    expect(screen.queryByRole('alert')).toBeNull()
    expect(screen.queryByRole('dialog')).toBeNull()
    expect(screen.getByRole('button', { name: 'Jogar' })).toBeEnabled()
  })

  it('TTB-039/"Fonte: IGDB" para igdb', async () => {
    installTtb(() => Promise.resolve(ttbView({ source: 'igdb' })))
    renderPanel()
    await loaded()
    expect(await screen.findByText('Fonte: IGDB')).toBeInTheDocument()
    expect(screen.queryByText('Fonte: HowLongToBeat')).toBeNull()
  })
})
