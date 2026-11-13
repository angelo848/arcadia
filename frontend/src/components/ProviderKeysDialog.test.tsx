import { describe, expect, it, vi, beforeEach } from 'vitest'
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
    expect((await screen.findAllByText('Não cadastrada')).length).toBe(2)
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
