import { describe, expect, it } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { MediaCarousel } from './MediaCarousel'
import { coverPlaceholderUrl } from './CoverImage'
import type { MediaItemView } from '../lib/metadata'

function item(id: number, kind: MediaItemView['kind']): MediaItemView {
  return { id, kind, provider: 'p', path: '', thumbPath: '', url: `/media/${id}.jpg`, thumbUrl: `/thumb/${id}.jpg` }
}

const three = [item(1, 'screenshot'), item(2, 'cover'), item(3, 'banner')]

function shownSrc(container: HTMLElement): string | null {
  return container.querySelector('img')?.getAttribute('src') ?? null
}

describe('MediaCarousel', () => {
  it('UIX-026/n = 0 ou items undefined mostra o placeholder, sem botões', () => {
    for (const items of [[], undefined]) {
      const { container, unmount } = render(<MediaCarousel items={items} resetKey={1} />)
      expect(shownSrc(container)).toBe(coverPlaceholderUrl)
      expect(screen.queryAllByRole('button')).toHaveLength(0)
      unmount()
    }
  })

  it('UIX-026/n = 1 mostra a imagem, o rótulo e "1 de 1", sem botões', () => {
    const { container } = render(<MediaCarousel items={[item(7, 'screenshot')]} resetKey={1} />)
    expect(shownSrc(container)).toBe('/media/7.jpg')
    expect(screen.getByText('Screenshot')).toBeTruthy()
    expect(screen.getByText('1 de 1')).toBeTruthy()
    expect(screen.queryAllByRole('button')).toHaveLength(0)
  })

  it('UIX-026/n = 3: a capa vem primeiro e Próxima imagem/Imagem anterior dão a volta nas pontas', () => {
    const { container } = render(<MediaCarousel items={three} resetKey={1} />)
    expect(shownSrc(container)).toBe('/media/2.jpg')
    expect(screen.getByText('Capa')).toBeTruthy()
    expect(screen.getByText('1 de 3')).toBeTruthy()
    const next = screen.getByRole('button', { name: 'Próxima imagem' })
    const prev = screen.getByRole('button', { name: 'Imagem anterior' })
    fireEvent.click(next)
    expect(shownSrc(container)).toBe('/media/1.jpg')
    expect(screen.getByText('2 de 3')).toBeTruthy()
    fireEvent.click(next)
    fireEvent.click(next)
    expect(shownSrc(container)).toBe('/media/2.jpg')
    fireEvent.click(prev)
    expect(shownSrc(container)).toBe('/media/3.jpg')
    expect(screen.getByText('3 de 3')).toBeTruthy()
  })

  it('UIX-026/setas esquerda e direita com o foco no carrossel trocam a imagem', () => {
    const { container } = render(<MediaCarousel items={three} resetKey={1} />)
    const region = screen.getByRole('region', { name: /Imagens/ })
    fireEvent.keyDown(region, { key: 'ArrowRight' })
    expect(shownSrc(container)).toBe('/media/1.jpg')
    fireEvent.keyDown(region, { key: 'ArrowLeft' })
    expect(shownSrc(container)).toBe('/media/2.jpg')
    fireEvent.keyDown(region, { key: 'ArrowLeft' })
    expect(shownSrc(container)).toBe('/media/3.jpg')
  })

  it('UIX-026/mudar resetKey volta à primeira imagem', () => {
    const { container, rerender } = render(<MediaCarousel items={three} resetKey={1} />)
    fireEvent.click(screen.getByRole('button', { name: 'Próxima imagem' }))
    expect(shownSrc(container)).toBe('/media/1.jpg')
    rerender(<MediaCarousel items={three} resetKey={2} />)
    expect(shownSrc(container)).toBe('/media/2.jpg')
    expect(screen.getByText('1 de 3')).toBeTruthy()
  })

  it('UIX-026/imagem que falha mostra o placeholder no lugar dela', () => {
    const { container } = render(<MediaCarousel items={three} resetKey={1} />)
    fireEvent.error(container.querySelector('img') as HTMLImageElement)
    expect(shownSrc(container)).toBe(coverPlaceholderUrl)
  })

  it('UIX-042/botões com os nomes "Imagem anterior" e "Próxima imagem"', () => {
    render(<MediaCarousel items={three} resetKey={1} />)
    expect(screen.getByRole('button', { name: 'Imagem anterior' })).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Próxima imagem' })).toBeTruthy()
    const region = screen.getByRole('region', { name: /Imagens/ })
    expect(region.getAttribute('aria-roledescription')).toBe('carrossel')
    expect(region.querySelector('[aria-live="polite"]')?.textContent).toContain('1 de 3')
  })

  it('UIX-043/o carrossel é navegável só pelo teclado', () => {
    render(<MediaCarousel items={three} resetKey={1} />)
    const buttons = screen.getAllByRole('button')
    for (const b of buttons) {
      expect(b.tagName).toBe('BUTTON')
      expect(b.getAttribute('tabindex')).not.toBe('-1')
    }
    buttons[0].focus()
    expect(document.activeElement).toBe(buttons[0])
  })
})
