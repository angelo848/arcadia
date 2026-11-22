import { afterEach, describe, expect, it, vi, beforeEach } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ProviderKeysDialog } from './ProviderKeysDialog'

const statusMock = vi.fn()
const setMock = vi.fn()
const deleteMock = vi.fn()

vi.mock('../lib/metadata', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../lib/metadata')>()
  return {
    ...actual,
    providerKeyStatus: (...a: unknown[]) => statusMock(...a),
    setProviderKey: (...a: unknown[]) => setMock(...a),
    deleteProviderKey: (...a: unknown[]) => deleteMock(...a),
  }
})

function renderDialog() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <ProviderKeysDialog onClose={vi.fn()} />
    </QueryClientProvider>,
  )
}

describe('ProviderKeysDialog', () => {
  beforeEach(() => {
    statusMock.mockReset()
    setMock.mockReset()
    deleteMock.mockReset()
    statusMock.mockImplementation((p: string) => Promise.resolve({ provider: p, configured: false }))
  })

  it('MID-001/as chaves são campos de senha e mostram só cadastrada ou não cadastrada', async () => {
    renderDialog()
    // alterado em 2026-11-15 (fase 13): o ScreenScraper soma dois grupos (SSC-002), 2 + 2.
    expect((await screen.findAllByText('Não cadastrada')).length).toBe(4)
    expect(screen.getByLabelText('Client ID')).toHaveAttribute('type', 'password')
    expect(screen.getByLabelText('Client secret')).toHaveAttribute('type', 'password')
    expect(screen.getByLabelText('Chave da API')).toHaveAttribute('type', 'password')
  })

  it('MID-001/salvar manda a chave e o segredo do IGDB e limpa os campos', async () => {
    setMock.mockResolvedValue({ provider: 'igdb', configured: true })
    renderDialog()
    await screen.findAllByText('Não cadastrada')
    fireEvent.change(screen.getByLabelText('Client ID'), { target: { value: 'cid' } })
    fireEvent.change(screen.getByLabelText('Client secret'), { target: { value: 'sec' } })
    fireEvent.click(screen.getAllByRole('button', { name: 'Salvar' })[0])

    await waitFor(() => expect(setMock).toHaveBeenCalledWith('igdb', 'cid', 'sec'))
    await waitFor(() => expect(screen.getByLabelText('Client ID')).toHaveValue(''))
  })
})

// ---------------------------------------------------------------------------
// ScreenScraper (phase 13, T29). Premissas: a seção tem rótulos "Usuário", "Senha", "devid" e "devpassword";
// os grupos aparecem na ordem Conta do usuário, Desenvolvedor do Arcadia; "Salvar" dos ScreenScraper são os
// dois últimos; "Remover" só aparece num grupo cadastrado. Os bindings Video* vêm de window.go.main.App.
// ---------------------------------------------------------------------------

type Handler = (...d: unknown[]) => void
let app: Record<string, ReturnType<typeof vi.fn>>
let handlers: Map<string, Set<Handler>>

function configured(set: string[]) {
  statusMock.mockImplementation((p: string) => Promise.resolve({ provider: p, configured: set.includes(p) }))
}

function installVideo(over: Record<string, ReturnType<typeof vi.fn>> = {}) {
  handlers = new Map()
  app = {
    VideoStatus: vi.fn(() =>
      Promise.resolve({ pending: 2, running: 0, failed: 1, lane: 'active', pausedUntil: '', requestsToday: 0, maxRequestsPerDay: 0 }),
    ),
    VideoQueueMissing: vi.fn(() => Promise.resolve({ queued: 3, skipped: 0 })),
    VideoCancelPending: vi.fn(() => Promise.resolve(2)),
    ...over,
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

describe('ProviderKeysDialog: ScreenScraper', () => {
  beforeEach(() => {
    statusMock.mockReset()
    setMock.mockReset()
    deleteMock.mockReset()
    configured([])
    installVideo()
  })
  afterEach(() => {
    vi.useRealTimers()
    delete (window as unknown as { go?: unknown }).go
    delete (window as unknown as { runtime?: unknown }).runtime
  })

  const ssSave = () => screen.getAllByRole('button', { name: 'Salvar' }).slice(-2)

  it('SSC-002/mostra a seção com os dois grupos e os rótulos Usuário, Senha, devid e devpassword', async () => {
    renderDialog()
    expect(await screen.findByText(/ScreenScraper/)).toBeInTheDocument()
    expect(screen.getByText('Conta do usuário')).toBeInTheDocument()
    expect(screen.getByText('Desenvolvedor do Arcadia')).toBeInTheDocument()
    for (const l of ['Usuário', 'Senha', 'devid', 'devpassword']) expect(screen.getByLabelText(l)).toBeInTheDocument()
  })

  it('SSC-002/cada grupo mostra Cadastrada ou Não cadastrada', async () => {
    configured(['screenscraper'])
    renderDialog()
    await screen.findAllByText('Cadastrada')
    expect(screen.getAllByText('Cadastrada')).toHaveLength(1)
    expect(screen.getAllByText('Não cadastrada')).toHaveLength(3)
  })

  it('SSC-002/Salvar só com os dois campos preenchidos; chama ProviderSetKey com o provedor certo (screenscraper e screenscraperdev), a chave e o secret', async () => {
    setMock.mockResolvedValue({ provider: 'x', configured: true })
    renderDialog()
    await screen.findAllByText('Não cadastrada')
    const [userSave, devSave] = ssSave()
    expect(userSave).toBeDisabled()
    expect(devSave).toBeDisabled()

    fireEvent.change(screen.getByLabelText('Usuário'), { target: { value: 'ana' } })
    expect(userSave).toBeDisabled()
    fireEvent.change(screen.getByLabelText('Senha'), { target: { value: 'pw' } })
    expect(userSave).toBeEnabled()
    fireEvent.click(userSave)
    await waitFor(() => expect(setMock).toHaveBeenCalledWith('screenscraper', 'ana', 'pw'))

    fireEvent.change(screen.getByLabelText('devid'), { target: { value: 'did' } })
    fireEvent.change(screen.getByLabelText('devpassword'), { target: { value: 'dpw' } })
    fireEvent.click(ssSave()[1])
    await waitFor(() => expect(setMock).toHaveBeenCalledWith('screenscraperdev', 'did', 'dpw'))
  })

  it('SSC-002/campos type password, autocomplete off e vazios depois de salvar', async () => {
    setMock.mockResolvedValue({ provider: 'screenscraper', configured: true })
    renderDialog()
    await screen.findAllByText('Não cadastrada')
    for (const l of ['Usuário', 'Senha', 'devid', 'devpassword']) {
      const el = screen.getByLabelText(l)
      expect(el, l).toHaveAttribute('type', 'password')
      expect(el, l).toHaveAttribute('autocomplete', 'off')
    }
    fireEvent.change(screen.getByLabelText('Usuário'), { target: { value: 'ana' } })
    fireEvent.change(screen.getByLabelText('Senha'), { target: { value: 'pw' } })
    fireEvent.click(ssSave()[0])
    await waitFor(() => expect(screen.getByLabelText('Usuário')).toHaveValue(''))
    expect(screen.getByLabelText('Senha')).toHaveValue('')
  })

  it('SSC-002/Remover chama ProviderDeleteKey do grupo certo', async () => {
    configured(['screenscraper', 'screenscraperdev'])
    deleteMock.mockResolvedValue({ provider: 'x', configured: false })
    renderDialog()
    await screen.findAllByText('Cadastrada')
    const removes = screen.getAllByRole('button', { name: 'Remover' }).slice(-2)
    fireEvent.click(removes[1])
    await waitFor(() => expect(deleteMock).toHaveBeenCalledWith('screenscraperdev'))
    fireEvent.click(removes[0])
    await waitFor(() => expect(deleteMock).toHaveBeenCalledWith('screenscraper'))
  })

  it('SSC-002/o valor das credenciais nunca é exibido', async () => {
    configured(['screenscraper', 'screenscraperdev'])
    statusMock.mockImplementation((p: string) =>
      Promise.resolve({ provider: p, configured: true, key: 'segredo-ana', secret: 'segredo-pw' }),
    )
    renderDialog()
    await screen.findAllByText('Cadastrada')
    expect(document.body.textContent).not.toContain('segredo-')
    for (const l of ['Usuário', 'Senha', 'devid', 'devpassword']) expect(screen.getByLabelText(l)).toHaveValue('')
  })

  it('SSC-003/os botões de vídeo só aparecem com as duas cadastradas', async () => {
    configured(['screenscraper'])
    const { unmount } = renderDialog()
    await screen.findAllByText('Cadastrada')
    expect(screen.queryByRole('button', { name: 'Buscar vídeos dos jogos sem vídeo' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Cancelar buscas pendentes' })).toBeNull()
    unmount()

    configured(['screenscraper', 'screenscraperdev'])
    renderDialog()
    expect(await screen.findByRole('button', { name: 'Buscar vídeos dos jogos sem vídeo' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Cancelar buscas pendentes' })).toBeInTheDocument()
  })

  it('SSC-003/"Buscar vídeos…" chama VideoQueueMissing(0) e mostra "N vídeos na fila"', async () => {
    configured(['screenscraper', 'screenscraperdev'])
    renderDialog()
    fireEvent.click(await screen.findByRole('button', { name: 'Buscar vídeos dos jogos sem vídeo' }))
    await waitFor(() => expect(app.VideoQueueMissing).toHaveBeenCalledWith(0))
    expect(await screen.findByText('3 vídeos na fila')).toBeInTheDocument()
  })

  it('SSC-003/"Cancelar buscas pendentes" chama VideoCancelPending', async () => {
    configured(['screenscraper', 'screenscraperdev'])
    renderDialog()
    fireEvent.click(await screen.findByRole('button', { name: 'Cancelar buscas pendentes' }))
    await waitFor(() => expect(app.VideoCancelPending).toHaveBeenCalledTimes(1))
  })

  it('SSC-003/a linha de estado traz os textos de cada situação', async () => {
    configured(['screenscraper', 'screenscraperdev'])
    app.VideoStatus.mockResolvedValue({
      pending: 2, running: 0, failed: 1, lane: 'paused-credentials', pausedUntil: '', requestsToday: 0, maxRequestsPerDay: 0,
    })
    renderDialog()
    expect(await screen.findByText(/2 pendentes/)).toBeInTheDocument()
    expect(screen.getByText(/1 sem vídeo/)).toBeInTheDocument()
    expect(screen.getByText(/credenciais recusadas/)).toBeInTheDocument()
  })

  it('SSC-003/relê o estado a cada 5 s com a janela aberta (temporizadores falsos), a cada media:changed, e limpa o intervalo ao fechar', async () => {
    configured(['screenscraper', 'screenscraperdev'])
    vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval'] })
    const { unmount } = renderDialog()
    await vi.waitFor(() => expect(app.VideoStatus).toHaveBeenCalled())
    const n = app.VideoStatus.mock.calls.length
    await vi.advanceTimersByTimeAsync(5000)
    expect(app.VideoStatus.mock.calls.length).toBeGreaterThan(n)

    const m = app.VideoStatus.mock.calls.length
    await vi.waitFor(() => expect(handlers.get('media:changed')?.size ?? 0).toBeGreaterThan(0))
    handlers.get('media:changed')!.forEach((h) => h({ gameId: 1 }))
    await vi.waitFor(() => expect(app.VideoStatus.mock.calls.length).toBeGreaterThan(m))

    unmount()
    const k = app.VideoStatus.mock.calls.length
    await vi.advanceTimersByTimeAsync(20000)
    expect(app.VideoStatus.mock.calls.length).toBe(k)
  })

  it('SSC-003/erro do binding mostra a mensagem de UI-008', async () => {
    configured(['screenscraper', 'screenscraperdev'])
    app.VideoQueueMissing.mockRejectedValue(new Error('O agente não está respondendo. Tente de novo.'))
    renderDialog()
    fireEvent.click(await screen.findByRole('button', { name: 'Buscar vídeos dos jogos sem vídeo' }))
    expect(await screen.findByText('O agente não está respondendo. Tente de novo.')).toBeInTheDocument()
  })
})

// alterado em 2026-11-22 (fase 15): seção "HowLongToBeat (tempo para zerar)" (TTB-024). Premissas: as duas caixas são
// checkboxes com esses nomes exatos; a seção não tem campo de chave nem botão "Salvar" (grava na hora); os bindings
// vêm de window.go.main.App (TimeToBeatSettings, TimeToBeatSetSettings).
describe('ProviderKeysDialog: HowLongToBeat', () => {
  const FIRST = 'Consultar o HowLongToBeat'
  const SECOND = 'Buscar depois de aplicar metadados (também no lote após importar)'
  let ttb: { TimeToBeatSettings: ReturnType<typeof vi.fn>; TimeToBeatSetSettings: ReturnType<typeof vi.fn> }

  function installTtb(enabled = true, afterMetadata = true) {
    ttb = {
      TimeToBeatSettings: vi.fn(() => Promise.resolve({ enabled, afterMetadata })),
      TimeToBeatSetSettings: vi.fn((e: boolean, a: boolean) => Promise.resolve({ enabled: e, afterMetadata: a })),
    }
    ;(window as unknown as { go?: unknown }).go = { main: { App: { ...ttb, VideoStatus: vi.fn(() => Promise.resolve({ pending: 0, running: 0, failed: 0, lane: 'active', pausedUntil: '', requestsToday: 0, maxRequestsPerDay: 0 })) } } }
    ;(window as unknown as { runtime?: unknown }).runtime = { EventsOn: () => () => {} }
  }

  beforeEach(() => {
    statusMock.mockReset()
    setMock.mockReset()
    deleteMock.mockReset()
    statusMock.mockImplementation((p: string) => Promise.resolve({ provider: p, configured: false }))
  })
  afterEach(() => {
    delete (window as unknown as { go?: unknown }).go
    delete (window as unknown as { runtime?: unknown }).runtime
  })

  it('TTB-024/a seção "HowLongToBeat (tempo para zerar)" tem as duas caixas com os valores lidos', async () => {
    installTtb(true, false)
    renderDialog()
    expect(await screen.findByText('HowLongToBeat (tempo para zerar)')).toBeInTheDocument()
    await waitFor(() => expect(screen.getByRole('checkbox', { name: FIRST })).toBeChecked())
    expect(screen.getByRole('checkbox', { name: SECOND })).not.toBeChecked()
  })

  it('TTB-024/os textos do serviço não oficial e da reserva do IGDB aparecem', async () => {
    installTtb()
    renderDialog()
    expect(await screen.findByText('Serviço não oficial: pode parar de funcionar sem aviso')).toBeInTheDocument()
    expect(
      screen.getByText('Se o HowLongToBeat falhar ou não achar o jogo, o IGDB é usado como reserva (precisa da chave do IGDB acima)'),
    ).toBeInTheDocument()
  })

  it('TTB-024/nenhum campo de chave na seção do HowLongToBeat', async () => {
    installTtb()
    renderDialog()
    await screen.findByText('HowLongToBeat (tempo para zerar)')
    expect(screen.queryByLabelText(/HowLongToBeat/i, { selector: 'input[type="password"], input[type="text"]' })).toBeNull()
    // A seção não tem "Salvar": os ScreenScraper continuam sendo os dois últimos (SSC-002).
    expect(screen.getAllByRole('button', { name: 'Salvar' })).toHaveLength(4)
  })

  it('TTB-024/desligar a primeira chama TimeToBeatSetSettings(false, afterMetadata) na hora e desativa a segunda', async () => {
    installTtb(true, true)
    renderDialog()
    const first = await screen.findByRole('checkbox', { name: FIRST })
    await waitFor(() => expect(first).toBeChecked())
    expect(screen.getByRole('checkbox', { name: SECOND })).toBeEnabled()
    fireEvent.click(first)
    await waitFor(() => expect(ttb.TimeToBeatSetSettings).toHaveBeenCalledTimes(1))
    expect(ttb.TimeToBeatSetSettings).toHaveBeenCalledWith(false, true)
    await waitFor(() => expect(screen.getByRole('checkbox', { name: SECOND })).toBeDisabled())
    expect(screen.getByRole('checkbox', { name: FIRST })).not.toBeChecked()
  })

  it('TTB-024/com a primeira desligada a segunda já abre desativada e não grava ao ser clicada', async () => {
    installTtb(false, true)
    renderDialog()
    const second = await screen.findByRole('checkbox', { name: SECOND })
    await waitFor(() => expect(second).toBeDisabled())
    fireEvent.click(second)
    expect(ttb.TimeToBeatSetSettings).not.toHaveBeenCalled()
  })

  it('TTB-024/desmarcar a segunda chama TimeToBeatSetSettings(true, false) na hora', async () => {
    installTtb(true, true)
    renderDialog()
    const second = await screen.findByRole('checkbox', { name: SECOND })
    await waitFor(() => expect(second).toBeChecked())
    fireEvent.click(second)
    await waitFor(() => expect(ttb.TimeToBeatSetSettings).toHaveBeenCalledWith(true, false))
    expect(ttb.TimeToBeatSetSettings).toHaveBeenCalledTimes(1)
  })

  it('TTB-024/religar a primeira chama TimeToBeatSetSettings(true, afterMetadata)', async () => {
    installTtb(false, true)
    renderDialog()
    const first = await screen.findByRole('checkbox', { name: FIRST })
    await waitFor(() => expect(first).not.toBeChecked())
    fireEvent.click(first)
    await waitFor(() => expect(ttb.TimeToBeatSetSettings).toHaveBeenCalledWith(true, true))
  })
})
