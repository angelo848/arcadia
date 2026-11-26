import { useState, type ReactNode } from 'react'
import { formatLocalDateTime } from '../lib/formatDate'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { SessionFormModal } from './SessionFormModal'
import type { GameDetailView, SessionItemView } from '../lib/library'

// Phase 16 (edicao-sessoes), SEL-005..SEL-011 and SEL-015. Local-time values are built from
// local Date parts, so the tests do not depend on the time zone of the machine.

type Fn = ReturnType<typeof vi.fn>
type FakeApp = { CreateSession: Fn; UpdateSession: Fn }

const GAME_ID = 10
let app: FakeApp

// "Now" of the UI: 25/11/2026 20:30:45 local (the form floors it to the minute).
const NOW = new Date(2026, 10, 25, 20, 30, 45)
const now = () => NOW
const iso = (y: number, mo: number, d: number, h: number, mi: number) => new Date(y, mo - 1, d, h, mi).toISOString()

function game(over: Partial<GameDetailView> = {}): GameDetailView {
  return {
    id: GAME_ID,
    platformId: 1,
    title: 'Hollow Knight',
    platform: 'PC',
    path: 'C:\\Games\\HK\\hk.exe',
    status: 'playing',
    favorite: false,
    year: 2017,
    totalPlaySeconds: 3600,
    playCount: 1,
    firstPlayedAt: '',
    lastPlayedAt: '',
    description: '',
    completedAt: null,
    developers: [],
    publishers: [],
    genres: [],
    playModes: [],
    launchType: 'exe',
    launchArgs: '',
    emulatorId: null,
    sessionPlaySeconds: 3600,
    ...over,
  }
}

// An ended session of 24/11 from 18:00 to 19:00 (span 60 min) with 50 min of active time.
function session(over: Partial<SessionItemView> = {}): SessionItemView {
  return {
    sessionId: 5,
    gameId: GAME_ID,
    state: 'ended',
    startedAt: iso(2026, 11, 24, 18, 0),
    endedAt: iso(2026, 11, 24, 19, 0),
    activeSeconds: 3000,
    endReason: 'exited',
    source: 'tracked',
    editedAt: '',
    ...over,
  }
}

function result(g = game()) {
  return Promise.resolve({ session: session(), game: g })
}

beforeEach(() => {
  app = {
    CreateSession: vi.fn(() => result()),
    UpdateSession: vi.fn(() => result()),
  }
  ;(window as unknown as { go?: unknown }).go = { main: { App: app } }
})

afterEach(() => {
  delete (window as unknown as { go?: unknown }).go
})

function wrapper({ children }: { children: ReactNode }) {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
}

type Over = Partial<Parameters<typeof SessionFormModal>[0]>

function renderForm(over: Over = {}) {
  const props = {
    mode: 'create' as const,
    game: game(),
    now,
    onClose: vi.fn(),
    onGone: vi.fn(),
    ...over,
  }
  render(<SessionFormModal {...props} />, { wrapper })
  return props
}

const start = () => screen.getByLabelText('Início') as HTMLInputElement
const end = () => screen.getByLabelText('Fim') as HTMLInputElement
const hours = () => screen.getByLabelText(/horas/i) as HTMLInputElement
const minutes = () => screen.getByLabelText(/minutos/i) as HTMLInputElement
const endText = (local: string) => formatLocalDateTime(new Date(local).toISOString())
const save = () => screen.getByRole('button', { name: 'Salvar' })

function type(el: HTMLElement, value: string) {
  fireEvent.change(el, { target: { value } })
}

describe('SessionFormModal', () => {
  it('SEL-005/modal compacto com título e os campos Início, Fim e Tempo jogado (horas e minutos)', () => {
    renderForm()
    const dialog = screen.getByRole('dialog', { name: 'Adicionar sessão' })
    expect(dialog).toHaveAttribute('data-size', 'compact')
    expect(start()).toHaveAttribute('type', 'datetime-local')
    expect(screen.queryByLabelText('Fim')).not.toBeInTheDocument()
    expect(screen.getByText('Fim (calculado)')).toBeInTheDocument()
    expect(hours()).toBeInTheDocument()
    expect(minutes()).toBeInTheDocument()
    expect(screen.getByText('Tempo jogado')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Cancelar' })).toBeInTheDocument()
  })

  it('SEL-005/título "Editar sessão" no modo edição', () => {
    renderForm({ mode: 'edit', session: session() })
    expect(screen.getByRole('dialog', { name: 'Editar sessão' })).toBeInTheDocument()
  })

  it('SEL-005/criação abre com Início = agora - 1 h, 1 h 00 e Fim calculado = agora', () => {
    renderForm()
    expect(screen.getByText(endText('2026-11-25T20:30'))).toBeInTheDocument()
    expect(start().value).toBe('2026-11-25T19:30')
    expect(hours().value).toBe('1')
    expect(minutes().value).toBe('0')
  })

  it('SEL-005/edição abre com os valores da sessão (tempo arredondado para baixo ao minuto)', () => {
    renderForm({ mode: 'edit', session: session({ activeSeconds: 3030 }) })
    expect(start().value).toBe('2026-11-24T18:00')
    expect(end().value).toBe('2026-11-24T19:00')
    expect(hours().value).toBe('0')
    expect(minutes().value).toBe('50')
  })

  it('SEL-006/na criação o Fim é calculado: Início + tempo jogado; mudar o Início não altera o tempo jogado', () => {
    renderForm()
    type(start(), '2026-11-25T19:00')
    expect(hours().value).toBe('1')
    expect(minutes().value).toBe('0')
    expect(screen.getByText(endText('2026-11-25T20:00'))).toBeInTheDocument()
    type(minutes(), '15')
    expect(screen.getByText(endText('2026-11-25T20:15'))).toBeInTheDocument()
  })

  it('SEL-007/campo inválido mostra a mensagem, Salvar fica desativado e nada é enviado', () => {
    renderForm()
    type(hours(), '3')
    expect(screen.getByText('O fim não pode estar no futuro.')).toBeInTheDocument()
    expect(save()).toBeDisabled()
    fireEvent.click(save())
    expect(app.CreateSession).not.toHaveBeenCalled()
  })

  it.each([
    ['fim no futuro', () => type(start(), '2026-11-25T20:00'), 'O fim não pode estar no futuro.'],
    [
      'mais de 24 h',
      () => {
        type(start(), '2026-11-24T10:00')
        type(hours(), '25')
      },
      'Uma sessão pode ter no máximo 24 horas.',
    ],
    [
      'tempo menor que 1 min',
      () => {
        type(hours(), '0')
        type(minutes(), '0')
      },
      'O tempo jogado precisa ter pelo menos 1 minuto.',
    ],
    [
      'minutos 60',
      () => {
        type(hours(), '0')
        type(minutes(), '60')
      },
      'Os minutos vão de 0 a 59.',
    ],
    ['campo vazio', () => type(minutes(), ''), 'Preencha este campo.'],
  ])('SEL-007/um caso por mensagem: %s', (_name, act, message) => {
    renderForm()
    act()
    expect(screen.getByText(message)).toBeInTheDocument()
    expect(save()).toBeDisabled()
  })

  it('SEL-008/na edição, Salvar chama UpdateSession só com os campos alterados', async () => {
    const props = renderForm({ mode: 'edit', session: session() })
    type(end(), '2026-11-24T19:30')
    // active time (50 min) did not follow the span, so it stays untouched.
    fireEvent.click(save())
    await waitFor(() => expect(app.UpdateSession).toHaveBeenCalledTimes(1))
    expect(app.UpdateSession).toHaveBeenCalledWith(5, {
      endedAt: iso(2026, 11, 24, 19, 30),
    })
    await waitFor(() => expect(props.onClose).toHaveBeenCalledTimes(1))
  })

  it('SEL-008/na edição, o tempo jogado com segundos de sessão rastreada não é enviado se não foi tocado', async () => {
    renderForm({ mode: 'edit', session: session({ activeSeconds: 3030 }) })
    type(start(), '2026-11-24T17:50')
    // 3030 s shows as 50 min, which does not follow the 70 min span: typed time stays.
    fireEvent.click(save())
    await waitFor(() => expect(app.UpdateSession).toHaveBeenCalledTimes(1))
    const patch = app.UpdateSession.mock.calls[0][1] as Record<string, unknown>
    expect(Object.keys(patch)).toEqual(['startedAt'])
  })

  it('SEL-008/na criação, Salvar envia todos os campos com os instantes em UTC', async () => {
    const props = renderForm()
    fireEvent.click(save())
    await waitFor(() => expect(app.CreateSession).toHaveBeenCalledTimes(1))
    expect(app.CreateSession).toHaveBeenCalledWith(GAME_ID, {
      startedAt: iso(2026, 11, 25, 19, 30),
      endedAt: iso(2026, 11, 25, 20, 30),
      activeSeconds: 3600,
      includedInTotal: false,
    })
    await waitFor(() => expect(props.onClose).toHaveBeenCalledTimes(1))
  })

  it('SEL-008/sem alteração na edição, Salvar fica desativado', () => {
    renderForm({ mode: 'edit', session: session() })
    expect(save()).toBeDisabled()
    fireEvent.click(save())
    expect(app.UpdateSession).not.toHaveBeenCalled()
  })

  describe('SEL-009 já está no tempo total', () => {
    // total 5400 s, sessions 1800 s: adjustment of 3600 s (1 h 00 min).
    const withAdjustment = game({
      totalPlaySeconds: 5400,
      sessionPlaySeconds: 1800,
    })

    it('SEL-009/com ajuste positivo aparece a caixa desmarcada com a ajuda', () => {
      renderForm({ game: withAdjustment })
      const box = screen.getByLabelText('Já está no tempo total') as HTMLInputElement
      expect(box.type).toBe('checkbox')
      expect(box.checked).toBe(false)
      expect(
        screen.getByText(
          'O tempo total deste jogo tem 1 h 00 min a mais que as sessões. Marque se esta sessão já faz parte desse tempo.',
        ),
      ).toBeInTheDocument()
    })

    it('SEL-009/marcada, o envio leva includedInTotal true', async () => {
      renderForm({ game: withAdjustment })
      fireEvent.click(screen.getByLabelText('Já está no tempo total'))
      fireEvent.click(save())
      await waitFor(() => expect(app.CreateSession).toHaveBeenCalledTimes(1))
      expect(app.CreateSession.mock.calls[0][1]).toMatchObject({
        includedInTotal: true,
        activeSeconds: 3600,
      })
    })

    it('SEL-009/marcada, tempo acima do ajuste é erro; desmarcada, não', () => {
      // adjustment of 40 min, default active time of 1 h.
      renderForm({
        game: game({ totalPlaySeconds: 4200, sessionPlaySeconds: 1800 }),
      })
      expect(save()).toBeEnabled()
      fireEvent.click(screen.getByLabelText('Já está no tempo total'))
      expect(
        screen.getByText('O tempo jogado é maior que a parte do tempo total que não vem de sessões.'),
      ).toBeInTheDocument()
      expect(save()).toBeDisabled()
      fireEvent.click(screen.getByLabelText('Já está no tempo total'))
      expect(save()).toBeEnabled()
    })

    it('SEL-009/sem ajuste positivo a caixa não aparece', () => {
      renderForm({
        game: game({ totalPlaySeconds: 1800, sessionPlaySeconds: 1800 }),
      })
      expect(screen.queryByLabelText('Já está no tempo total')).toBeNull()
    })

    it('SEL-009/na edição a caixa nunca aparece', () => {
      renderForm({ mode: 'edit', session: session(), game: withAdjustment })
      expect(screen.queryByLabelText('Já está no tempo total')).toBeNull()
    })
  })

  it('SEL-010/erro do agente mantém o modal aberto com os valores digitados e mostra em role="alert"', async () => {
    app.CreateSession.mockRejectedValueOnce('O horário cruza outra sessão de «Celeste», de 10:00 a 11:00.')
    const props = renderForm()
    type(hours(), '0') // alterado em 2026-11-25 (fase 16): tempo 1 h 30 excedia o intervalo de 1 h
    type(minutes(), '30')
    fireEvent.click(save())
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'O horário cruza outra sessão de «Celeste», de 10:00 a 11:00.',
    )
    expect(screen.getByRole('dialog')).toBeInTheDocument()
    expect(props.onClose).not.toHaveBeenCalled()
    expect(props.onGone).not.toHaveBeenCalled()
    expect(minutes().value).toBe('30')
    expect(start().value).toBe('2026-11-25T19:30')
  })

  it('SEL-010/"Esta sessão não existe mais." pede o recarregamento da lista', async () => {
    app.UpdateSession.mockRejectedValueOnce('Esta sessão não existe mais.')
    const props = renderForm({ mode: 'edit', session: session() })
    type(end(), '2026-11-24T19:30')
    fireEvent.click(save())
    await waitFor(() => expect(props.onGone).toHaveBeenCalledTimes(1))
  })

  it('SEL-011/durante o envio o botão fica desativado com "Salvando…" e um segundo clique não chama o agente de novo', async () => {
    let release: (v: unknown) => void = () => {}
    app.CreateSession.mockImplementationOnce(() => new Promise((resolve) => (release = resolve)))
    const props = renderForm()
    fireEvent.click(save())
    const busy = await screen.findByRole('button', { name: 'Salvando…' })
    expect(busy).toBeDisabled()
    fireEvent.click(busy)
    expect(app.CreateSession).toHaveBeenCalledTimes(1)
    release({ session: session(), game: game() })
    await waitFor(() => expect(props.onClose).toHaveBeenCalledTimes(1))
    expect(app.CreateSession).toHaveBeenCalledTimes(1)
  })

  // Added 2026-11-25 (fase 16 review).
  it('SEL-011/dois cliques em Salvar no mesmo tick chamam o binding uma única vez', async () => {
    const props = renderForm()
    const btn = save()
    act(() => {
      fireEvent.click(btn)
      fireEvent.click(btn)
    })
    await waitFor(() => expect(props.onClose).toHaveBeenCalledTimes(1))
    expect(app.CreateSession).toHaveBeenCalledTimes(1)
  })

  it('SEL-011/durante o envio o botão Cancelar fica desativado', async () => {
    app.CreateSession.mockImplementationOnce(() => new Promise(() => {}))
    renderForm()
    fireEvent.click(save())
    await screen.findByRole('button', { name: 'Salvando…' })
    expect(screen.getByRole('button', { name: 'Cancelar' })).toBeDisabled()
  })

  it('SEL-015/cada campo tem rótulo associado; campo com erro tem aria-invalid e aria-describedby para a mensagem', () => {
    renderForm()
    expect(start()).not.toHaveAttribute('aria-invalid', 'true')
    type(start(), '')
    expect(start()).toHaveAttribute('aria-invalid', 'true')
    const id = start().getAttribute('aria-describedby')
    expect(id).toBeTruthy()
    expect(document.getElementById(id as string)).toHaveTextContent('Preencha este campo.')
  })

  it('SEL-015/aria-invalid também no tempo jogado', () => {
    renderForm()
    type(minutes(), '60')
    expect(minutes()).toHaveAttribute('aria-invalid', 'true')
    const id = minutes().getAttribute('aria-describedby')
    expect(document.getElementById(id as string)).toHaveTextContent('Os minutos vão de 0 a 59.')
  })

  it('SEL-015/o foco começa em Início', () => {
    renderForm()
    expect(start()).toHaveFocus()
  })

  it('SEL-015/Enter num campo salva quando Salvar está ativo e não salva quando está desativado', async () => {
    renderForm()
    type(minutes(), '60')
    fireEvent.keyDown(start(), { key: 'Enter' })
    fireEvent.submit(start().closest('form') ?? start())
    expect(app.CreateSession).not.toHaveBeenCalled()
    type(minutes(), '0')
    fireEvent.keyDown(start(), { key: 'Enter' })
    await waitFor(() => expect(app.CreateSession).toHaveBeenCalledTimes(1))
  })

  it('SEL-015/Esc cancela sem chamar o agente', () => {
    const props = renderForm()
    fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape' })
    expect(props.onClose).toHaveBeenCalledTimes(1)
    expect(app.CreateSession).not.toHaveBeenCalled()
    expect(app.UpdateSession).not.toHaveBeenCalled()
  })

  it('SEL-015/ao fechar, o foco volta ao botão que abriu o modal', () => {
    function Harness() {
      const [open, setOpen] = useState(false)
      return (
        <>
          <button type="button" onClick={() => setOpen(true)}>
            Abrir
          </button>
          {open && (
            <SessionFormModal mode="create" game={game()} now={now} onClose={() => setOpen(false)} onGone={() => {}} />
          )}
        </>
      )
    }
    render(<Harness />, { wrapper })
    const opener = screen.getByRole('button', { name: 'Abrir' })
    opener.focus()
    fireEvent.click(opener)
    expect(screen.getByRole('dialog')).toBeInTheDocument()
    fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape' })
    expect(screen.queryByRole('dialog')).toBeNull()
    expect(opener).toHaveFocus()
  })
})
