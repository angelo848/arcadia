import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { Panel } from './Panel'

describe('Panel', () => {
  it('BBX-037/Panel tem data-bb-panel e a classe do token bg-bb-panel', () => {
    render(<Panel>texto</Panel>)
    const el = screen.getByText('texto')
    expect(el).toHaveAttribute('data-bb-panel')
    expect(el.className).toContain('bg-bb-panel')
  })

  it('BBX-037/className extra é somada e "as" troca o elemento', () => {
    const { container } = render(<Panel as="section" className="extra">x</Panel>)
    const el = container.firstElementChild as HTMLElement
    expect(el.tagName).toBe('SECTION')
    expect(el.className).toContain('extra')
    expect(el.className).toContain('bg-bb-panel')
    expect(el).toHaveAttribute('data-bb-panel')
  })

  it('BBX-037/o painel nunca usa o scrim como fundo', () => {
    render(<Panel>y</Panel>)
    expect(screen.getByText('y').className).not.toContain('scrim')
  })
})
