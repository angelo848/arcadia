// alterado em 2026-11-15 (docs/specs/phase-12-metadados-unificados, T34): MID-002 e MID-003 (provedor escolhido,
// ScrapeSearch/ScrapeApply) saem; a substituição está em MET-043, MET-044 e MET-024 abaixo. Os casos ficam nos IDs novos.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MetadataDialog } from './MetadataDialog'
import { libraryKeys, type GameDetailView } from '../lib/library'
import type { MetadataApplyView, MetadataCandidateView, MetadataSearchView } from '../lib/metadata'

// Bindings Wails do desktop (window.go.main.App.<Nome>): o diálogo só chama MetadataSearch/MetadataApply e leituras.
type Spy = ReturnType<typeof vi.fn>

function install(over: Record<string, unknown> = {}): Record<string, Spy> {
  const app: Record<string, Spy> = {
    GetGame: vi.fn(() => Promise.resolve(game)),
    MetadataSearch: vi.fn(),
    MetadataApply: vi.fn(),
    // Ações que a UI da fase 12 nunca pode chamar (MET-080).
    LaunchGame: vi.fn(),
    LaunchGameUntracked: vi.fn(),
    PauseSession: vi.fn(),
    ResumeSession: vi.fn(),
    ...over,
  }
  ;(window as unknown as { go?: unknown }).go = { main: { App: app } }
  return app
}

// O jogo em foco: título, plataforma e ano vêm do cache de libraryKeys.game (como no App).
const game = { id: 3, title: 'Celeste', platform: 'PC', year: 2018 } as unknown as GameDetailView

const celeste: MetadataCandidateView = {
  refs: [
    { provider: 'igdb', providerId: '1942' },
    { provider: 'steamgriddb', providerId: '77' },
  ],
  title: 'Celeste',
  year: 2018,
  platform: 'PC',
  score: 0.95,
}

const outro: MetadataCandidateView = {
  refs: [{ provider: 'steamstore', providerId: '10' }],
  title: 'Outro jogo',
  year: 2001,
  platform: '',
  score: 0.4,
}

function searchResult(over: Partial<MetadataSearchView> = {}): MetadataSearchView {
  return { candidates: [celeste, outro], confidence: 'match', warnings: [], ...over }
}

function applyResult(queued: number, skippedByLimit: number): MetadataApplyView {
  return { game, queued, skippedByLimit } as unknown as MetadataApplyView
}

function renderDialog(qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })) {
  qc.setQueryData(libraryKeys.game(3), game)
  render(
    <QueryClientProvider client={qc}>
      <MetadataDialog gameId={3} onClose={vi.fn()} />
    </QueryClientProvider>,
  )
  return qc
}

async function searchAndWait() {
  fireEvent.click(screen.getByRole('button', { name: 'Obter dados' }))
  expect(await screen.findByRole('radio', { name: 'Celeste (2018), IGDB · SteamGridDB' })).toBeInTheDocument()
}

describe('MetadataDialog', () => {
  beforeEach(() => {
    install()
  })

  afterEach(() => {
    delete (window as unknown as { go?: unknown }).go
  })

  it('MET-043/campo "Nome para busca" vem com o título do jogo e é editável', async () => {
    const app = install({ MetadataSearch: vi.fn(() => Promise.resolve(searchResult())) })
    renderDialog()

    const field = await screen.findByLabelText('Nome para busca')
    expect(field).toHaveValue('Celeste')

    fireEvent.change(field, { target: { value: 'Celeste Demo' } })
    expect(field).toHaveValue('Celeste Demo')

    fireEvent.click(screen.getByRole('button', { name: 'Obter dados' }))
    await waitFor(() => expect(app.MetadataSearch).toHaveBeenCalledWith('Celeste Demo', 'PC', 2018))
  })

  it('MET-043/não há select de provedor', async () => {
    renderDialog()
    await screen.findByLabelText('Nome para busca')

    expect(screen.queryByRole('combobox')).toBeNull()
    expect(screen.queryByLabelText('Provedor')).toBeNull()
  })

  it('MET-043/Obter dados chama metadataSearch com o nome digitado, a plataforma e o ano do jogo', async () => {
    const app = install({ MetadataSearch: vi.fn(() => Promise.resolve(searchResult())) })
    renderDialog()

    const field = await screen.findByLabelText('Nome para busca')
    fireEvent.change(field, { target: { value: 'Celeste Nome Digitado' } })
    fireEvent.click(screen.getByRole('button', { name: 'Obter dados' }))

    await waitFor(() => expect(app.MetadataSearch).toHaveBeenCalledTimes(1))
    expect(app.MetadataSearch).toHaveBeenCalledWith('Celeste Nome Digitado', 'PC', 2018)
  })

  it('MET-043/jogo sem ano envia 0', async () => {
    const semAno = { ...game, year: null } as unknown as GameDetailView
    const app = install({
      GetGame: vi.fn(() => Promise.resolve(semAno)),
      MetadataSearch: vi.fn(() => Promise.resolve(searchResult())),
    })
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    qc.setQueryData(libraryKeys.game(3), semAno)
    render(
      <QueryClientProvider client={qc}>
        <MetadataDialog gameId={3} onClose={vi.fn()} />
      </QueryClientProvider>,
    )

    await screen.findByLabelText('Nome para busca')
    fireEvent.click(screen.getByRole('button', { name: 'Obter dados' }))

    await waitFor(() => expect(app.MetadataSearch).toHaveBeenCalledWith('Celeste', 'PC', 0))
  })

  it('MET-043/botão desativado durante a busca', async () => {
    install({ MetadataSearch: vi.fn(() => new Promise<MetadataSearchView>(() => undefined)) })
    renderDialog()

    const button = await screen.findByRole('button', { name: 'Obter dados' })
    expect(button).toBeEnabled()

    fireEvent.click(button)

    await waitFor(() => expect(screen.getByRole('button', { name: 'Obter dados' })).toBeDisabled())
  })

  it('MET-043/Aplicar chama metadataApply com as refs da opção e replace falso sem marcar a caixa', async () => {
    const app = install({
      MetadataSearch: vi.fn(() => Promise.resolve(searchResult())),
      MetadataApply: vi.fn(() => Promise.resolve(applyResult(2, 0))),
    })
    renderDialog()
    await searchAndWait()

    fireEvent.click(screen.getAllByRole('button', { name: 'Aplicar' })[0])

    await waitFor(() => expect(app.MetadataApply).toHaveBeenCalledWith(3, celeste.refs, false))
  })

  it('MET-043/Aplicar chama metadataApply com replace verdadeiro quando a caixa "Substituir" está marcada', async () => {
    const app = install({
      MetadataSearch: vi.fn(() => Promise.resolve(searchResult())),
      MetadataApply: vi.fn(() => Promise.resolve(applyResult(0, 0))),
    })
    renderDialog()
    await searchAndWait()

    fireEvent.click(screen.getByRole('checkbox', { name: 'Substituir os campos já preenchidos' }))
    fireEvent.click(screen.getAllByRole('button', { name: 'Aplicar' })[1])

    await waitFor(() => expect(app.MetadataApply).toHaveBeenCalledWith(3, outro.refs, true))
  })

  it('MET-043/erro de provedor mostra a mensagem do binding', async () => {
    install({ MetadataSearch: vi.fn(() => Promise.reject(new Error('IGDB indisponível: limite de requisições'))) })
    renderDialog()

    fireEvent.click(await screen.findByRole('button', { name: 'Obter dados' }))

    expect(await screen.findByText('IGDB indisponível: limite de requisições')).toBeInTheDocument()
  })

  it('MET-043/aplicar invalida jogo e lista (IMG-011)', async () => {
    install({
      MetadataSearch: vi.fn(() => Promise.resolve(searchResult())),
      MetadataApply: vi.fn(() => Promise.resolve(applyResult(2, 0))),
    })
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const invalidate = vi.spyOn(qc, 'invalidateQueries')
    renderDialog(qc)
    await searchAndWait()

    fireEvent.click(screen.getAllByRole('button', { name: 'Aplicar' })[0])

    await waitFor(() => {
      const keys = invalidate.mock.calls.map((c) => (c[0] as { queryKey?: unknown } | undefined)?.queryKey)
      expect(keys).toEqual(
        expect.arrayContaining([libraryKeys.game(3), ['library', 'games'], ['media', 3]]),
      )
    })
  })

  it('MET-044/lista de opções aparece com selo "provável" só no primeiro item e com os avisos', async () => {
    install({
      MetadataSearch: vi.fn(() =>
        Promise.resolve(searchResult({ warnings: ['SteamGridDB sem resposta nesta busca'] })),
      ),
    })
    renderDialog()
    await searchAndWait()

    expect(screen.getByText('SteamGridDB sem resposta nesta busca')).toBeInTheDocument()
    expect(screen.getByRole('radio', { name: 'Outro jogo (2001), Steam Store' })).toBeInTheDocument()
    expect(screen.getAllByText('provável')).toHaveLength(1)
  })

  it('MET-044/confiança "doubt" não mostra o selo "provável"', async () => {
    install({ MetadataSearch: vi.fn(() => Promise.resolve(searchResult({ confidence: 'doubt' }))) })
    renderDialog()
    await searchAndWait()

    expect(screen.queryByText('provável')).toBeNull()
  })

  it('MET-044/lista vazia mostra "Nenhum resultado para este nome."', async () => {
    install({ MetadataSearch: vi.fn(() => Promise.resolve(searchResult({ candidates: [], confidence: 'none' }))) })
    renderDialog()

    fireEvent.click(await screen.findByRole('button', { name: 'Obter dados' }))

    expect(await screen.findByText('Nenhum resultado para este nome.')).toBeInTheDocument()
  })

  it('MET-024/resultado "N imagens na fila; M não entraram porque o jogo já tem o máximo"', async () => {
    install({
      MetadataSearch: vi.fn(() => Promise.resolve(searchResult())),
      MetadataApply: vi.fn(() => Promise.resolve(applyResult(4, 2))),
    })
    renderDialog()
    await searchAndWait()

    fireEvent.click(screen.getAllByRole('button', { name: 'Aplicar' })[0])

    expect(
      await screen.findByText('4 imagens na fila; 2 não entraram porque o jogo já tem o máximo'),
    ).toBeInTheDocument()
  })

  it('MET-024/sem pulados mostra só a primeira parte', async () => {
    install({
      MetadataSearch: vi.fn(() => Promise.resolve(searchResult())),
      MetadataApply: vi.fn(() => Promise.resolve(applyResult(2, 0))),
    })
    renderDialog()
    await searchAndWait()

    fireEvent.click(screen.getAllByRole('button', { name: 'Aplicar' })[0])

    const texto = await screen.findByText(/2 imagens na fila/)
    expect(texto).toHaveTextContent('2 imagens na fila')
    expect(texto).not.toHaveTextContent('não entraram')
  })

  it('MET-080/nenhuma chamada de lançamento, sessão ou pausa', async () => {
    const app = install({
      MetadataSearch: vi.fn(() => Promise.resolve(searchResult())),
      MetadataApply: vi.fn(() => Promise.resolve(applyResult(2, 0))),
    })
    renderDialog()
    await searchAndWait()
    fireEvent.click(screen.getAllByRole('button', { name: 'Aplicar' })[0])
    await waitFor(() => expect(app.MetadataApply).toHaveBeenCalledTimes(1))

    expect(app.LaunchGame).not.toHaveBeenCalled()
    expect(app.LaunchGameUntracked).not.toHaveBeenCalled()
    expect(app.PauseSession).not.toHaveBeenCalled()
    expect(app.ResumeSession).not.toHaveBeenCalled()
  })
})
