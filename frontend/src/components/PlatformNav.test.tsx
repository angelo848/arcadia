import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, within } from '@testing-library/react'
import { PlatformNav } from './PlatformNav'
import type { PlatformView } from '../lib/library'

function platform(id: number, name: string, gameCount: number, seedKey: string | null = null): PlatformView {
  return { id, name, category: 'console', emulatorId: null, seedKey, iconUrl: '', gameCount }
}

const catalog: PlatformView[] = [
  platform(1, 'Windows', 5, 'windows'),
  platform(2, 'Nintendo 64', 0, 'n64'),
  platform(3, 'Super Nintendo', 3, 'snes'),
]

function items(): HTMLElement[] {
  return screen.getAllByRole('button')
}

describe('PlatformNav', () => {
  it('UIX-009/"Todos" primeiro, com a soma de gameCount das plataformas listadas', () => {
    render(<PlatformNav platforms={catalog} selectedIds={undefined} onChange={vi.fn()} />)
    expect(screen.getByText('Plataformas')).toBeInTheDocument()
    const first = items()[0]
    expect(first).toHaveAccessibleName('Todos, 8 jogos')
  })

  it('UIX-009/plataforma com 0 jogos fica fora; a selecionada com 0 jogos fica', () => {
    const { unmount } = render(<PlatformNav platforms={catalog} selectedIds={undefined} onChange={vi.fn()} />)
    expect(screen.queryByRole('button', { name: /Nintendo 64/ })).toBeNull()
    unmount()

    render(<PlatformNav platforms={catalog} selectedIds={[2]} onChange={vi.fn()} />)
    expect(screen.getByRole('button', { name: 'Nintendo 64, 0 jogos' })).toBeInTheDocument()
  })

  it('UIX-009/ordem do catálogo, cada item com ícone, nome e número', () => {
    render(<PlatformNav platforms={catalog} selectedIds={undefined} onChange={vi.fn()} />)
    const list = items()
    expect(list).toHaveLength(3)
    expect(list[1]).toHaveAccessibleName('Windows, 5 jogos')
    expect(list[2]).toHaveAccessibleName('Super Nintendo, 3 jogos')
    const win = list[1]
    expect(win.querySelector('img')).not.toBeNull()
    expect(within(win).getByText('Windows')).toBeInTheDocument()
    expect(within(win).getByText('5')).toBeInTheDocument()
  })

  it('UIX-009/sem catálogo mostra só "Todos", sem número', () => {
    render(<PlatformNav platforms={undefined} selectedIds={undefined} onChange={vi.fn()} />)
    const list = items()
    expect(list).toHaveLength(1)
    expect(list[0]).toHaveTextContent('Todos')
    expect(list[0].textContent ?? '').not.toMatch(/\d/)
  })

  it('UIX-010/clicar em P chama onChange([P]); clicar em Todos chama onChange(undefined)', () => {
    const onChange = vi.fn()
    render(<PlatformNav platforms={catalog} selectedIds={[1]} onChange={onChange} />)
    fireEvent.click(screen.getByRole('button', { name: 'Super Nintendo, 3 jogos' }))
    expect(onChange).toHaveBeenCalledTimes(1)
    expect(onChange).toHaveBeenLastCalledWith([3])
    fireEvent.click(screen.getByRole('button', { name: /^Todos/ }))
    expect(onChange).toHaveBeenCalledTimes(2)
    expect(onChange).toHaveBeenLastCalledWith(undefined)
  })

  it('UIX-010/clicar no item atual não chama onChange', () => {
    const onChange = vi.fn()
    const { unmount } = render(<PlatformNav platforms={catalog} selectedIds={[3]} onChange={onChange} />)
    fireEvent.click(screen.getByRole('button', { name: 'Super Nintendo, 3 jogos' }))
    unmount()
    render(<PlatformNav platforms={catalog} selectedIds={undefined} onChange={onChange} />)
    fireEvent.click(screen.getByRole('button', { name: /^Todos/ }))
    expect(onChange).not.toHaveBeenCalled()
  })

  it('UIX-010/sem filtro "Todos" tem aria-current; com [P], P tem aria-current; com dois ids, nenhum', () => {
    const current = () => items().filter((b) => b.getAttribute('aria-current') === 'true')

    const a = render(<PlatformNav platforms={catalog} selectedIds={undefined} onChange={vi.fn()} />)
    expect(current()).toHaveLength(1)
    expect(current()[0]).toHaveAccessibleName(/^Todos/)
    a.unmount()

    const b = render(<PlatformNav platforms={catalog} selectedIds={[3]} onChange={vi.fn()} />)
    expect(current()).toHaveLength(1)
    expect(current()[0]).toHaveAccessibleName('Super Nintendo, 3 jogos')
    b.unmount()

    render(<PlatformNav platforms={catalog} selectedIds={[1, 3]} onChange={vi.fn()} />)
    expect(current()).toHaveLength(0)
  })

  it('UIX-042/nome acessível "<plataforma>, N jogos" e ícone com alt vazio', () => {
    render(<PlatformNav platforms={catalog} selectedIds={undefined} onChange={vi.fn()} />)
    expect(screen.getByRole('button', { name: 'Windows, 5 jogos' })).toBeInTheDocument()
    const imgs = document.querySelectorAll('img')
    expect(imgs.length).toBeGreaterThan(0)
    imgs.forEach((img) => expect(img.getAttribute('alt')).toBe(''))
  })

  it('UIX-043/Enter e Espaço no item escolhem a plataforma', () => {
    // Items are native buttons, so Enter and Space activate them as a click in the browser.
    const onChange = vi.fn()
    render(<PlatformNav platforms={catalog} selectedIds={undefined} onChange={onChange} />)
    const item = screen.getByRole('button', { name: 'Windows, 5 jogos' })
    expect(item.tagName).toBe('BUTTON')
    expect(item.getAttribute('tabindex')).not.toBe('-1')
    item.focus()
    expect(document.activeElement).toBe(item)
    fireEvent.click(item) // what the browser dispatches for Enter and Space
    expect(onChange).toHaveBeenCalledWith([1])
  })
})
