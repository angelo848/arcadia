// Assumed markup (T15 must follow) for DesktopSidebar [TDK-003, TDK-021, TDK-023, TDK-029, BBX-065]:
//  <nav aria-label="Plataformas"> with a text title "Plataformas" and one <button> per tab (name "<label>, <count> jogos"),
//  each with an <img> icon (only when tab.iconUrl is set), the label and the count as text; the active tab has aria-current="true"
//  (fill bb-surface, text bb-text), the others text-muted; data-focused="true" on the active item only with `focused`.
import { fireEvent, render, screen, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { BigBoxTab } from '../../viewModel'
import { DesktopSidebar } from './DesktopSidebar'

const TABS: BigBoxTab[] = [
  { key: 'all', label: 'Todos', filter: { platformId: null }, iconUrl: null, count: 120 },
  { key: 'p:7', label: 'Nintendo 64', filter: { platformId: 7 }, iconUrl: '/i/n64.png', count: 40 },
  { key: 'p:8', label: 'SNES', filter: { platformId: 8 }, iconUrl: '/i/snes.png', count: 80 },
]
const look = (el: Element) => `${el.className} ${el.getAttribute('style') ?? ''}`
function setup(o: Partial<{ tab: number; focused: boolean }> = {}) {
  const onTab = vi.fn()
  const r = render(<DesktopSidebar tabs={TABS} tab={o.tab ?? 1} focused={o.focused ?? false} onTab={onTab} />)
  return { onTab, ...r }
}
const proto = Element.prototype as unknown as { scrollIntoView?: unknown }
const original = proto.scrollIntoView

afterEach(() => {
  if (original === undefined) delete proto.scrollIntoView
  else proto.scrollIntoView = original
})

describe('DesktopSidebar', () => {
  it('TDK-003/título "Plataformas", Todos com a soma e cada plataforma com ícone, nome e contagem; sem busca nem Filtros', () => {
    const { container } = setup()
    const nav = screen.getByRole('navigation', { name: 'Plataformas' })
    expect(within(nav).getByText('Plataformas')).toBeInTheDocument()
    const all = screen.getByRole('button', { name: 'Todos, 120 jogos' })
    expect(within(all).getByText('Todos')).toBeInTheDocument()
    expect(within(all).getByText('120')).toBeInTheDocument()
    for (const [name, label, count, icon] of [
      ['Nintendo 64, 40 jogos', 'Nintendo 64', '40', '/i/n64.png'],
      ['SNES, 80 jogos', 'SNES', '80', '/i/snes.png'],
    ]) {
      const item = screen.getByRole('button', { name })
      expect(within(item).getByText(label)).toBeInTheDocument()
      expect(within(item).getByText(count)).toBeInTheDocument()
      expect(item.querySelector('img')?.getAttribute('src')).toBe(icon)
    }
    expect(screen.getAllByRole('button')).toHaveLength(3)
    expect(container.querySelector('input, select, [role="searchbox"]')).toBeNull()
    expect(container.textContent).not.toMatch(/Filtros|Buscar|Pesquisar/)
  })

  it('TDK-003/aba ativa com aria-current="true", fundo bb-surface e texto bb-text; as outras em texto muted', () => {
    setup({ tab: 1 })
    const active = screen.getByRole('button', { name: 'Nintendo 64, 40 jogos' })
    expect(active).toHaveAttribute('aria-current', 'true')
    expect(look(active)).toMatch(/bb-surface/)
    expect(look(active)).toMatch(/bb-text(?![-\w])/)
    for (const name of ['Todos, 120 jogos', 'SNES, 80 jogos']) {
      const other = screen.getByRole('button', { name })
      expect(other).not.toHaveAttribute('aria-current', 'true')
      expect(look(other)).toMatch(/bb-text-muted/)
    }
  })

  it('TDK-029/<nav aria-label="Plataformas"> com nome "<plataforma>, N jogos" por item', () => {
    setup()
    expect(screen.getByRole('navigation', { name: 'Plataformas' }).tagName).toBe('NAV')
    expect(screen.getAllByRole('button').map((b) => b.getAttribute('aria-label') ?? b.textContent)).toEqual([
      'Todos, 120 jogos', 'Nintendo 64, 40 jogos', 'SNES, 80 jogos',
    ])
  })

  it('TDK-023/data-focused só no item ativo e só com a zona Plataformas', () => {
    const { container, rerender } = setup({ focused: false })
    expect(container.querySelector('[data-focused="true"]')).toBeNull()
    rerender(<DesktopSidebar tabs={TABS} tab={2} focused onTab={vi.fn()} />)
    const marks = container.querySelectorAll('[data-focused="true"]')
    expect(marks).toHaveLength(1)
    expect(marks[0]).toBe(screen.getByRole('button', { name: 'SNES, 80 jogos' }))
    expect(look(marks[0])).toMatch(/outline|bb-focus/)
  })

  it('BBX-065/TDK-025/clicar numa aba chama onTab(i)', () => {
    const { onTab } = setup({ tab: 1 })
    fireEvent.click(screen.getByRole('button', { name: 'SNES, 80 jogos' }))
    expect(onTab).toHaveBeenCalledWith(2)
    fireEvent.click(screen.getByRole('button', { name: 'Todos, 120 jogos' }))
    expect(onTab).toHaveBeenLastCalledWith(0)
  })

  it('TDK-021/scrollIntoView({ block: "nearest" }) no item ativo quando a aba muda', () => {
    const scroll = vi.fn()
    proto.scrollIntoView = scroll
    const { rerender } = setup({ tab: 0 })
    scroll.mockClear()
    rerender(<DesktopSidebar tabs={TABS} tab={2} focused={false} onTab={vi.fn()} />)
    expect(scroll).toHaveBeenCalled()
    expect(scroll.mock.calls.at(-1)?.[0]).toMatchObject({ block: 'nearest' })
    expect((scroll.mock.contexts as Element[]).at(-1)).toBe(screen.getByRole('button', { name: 'SNES, 80 jogos' }))
  })

  it('TDK-021/sem scrollIntoView (jsdom) não quebra', () => {
    delete proto.scrollIntoView
    const { rerender } = setup({ tab: 0 })
    expect(() => rerender(<DesktopSidebar tabs={TABS} tab={1} focused={false} onTab={vi.fn()} />)).not.toThrow()
  })
})
