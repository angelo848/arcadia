import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, within } from '@testing-library/react'
import { MediaCarousel } from './MediaCarousel'
import { coverPlaceholderUrl } from './CoverImage'
import { GRID_INSET } from '../lib/gridNav'
import type { MediaItemView } from '../lib/metadata'

function item(id: number, kind: MediaItemView['kind'], thumbUrl = `/thumb/${id}.jpg`): MediaItemView {
  return { id, kind, provider: 'p', path: '', thumbPath: '', url: `/media/${id}.jpg`, thumbUrl }
}

// Mixed kinds on purpose; carouselKindOrder: cover, screenshot, title, select, background, banner.
const three = [item(1, 'screenshot'), item(2, 'cover'), item(3, 'banner')]
// Display order of `three`: cover(2), screenshot(1), banner(3).

function shownSrc(container: HTMLElement): string | null {
  return container.querySelector('img')?.getAttribute('src') ?? null
}
function strip(): HTMLElement {
  return screen.getByRole('group', { name: 'Miniaturas' })
}
function thumbs(): HTMLElement[] {
  return within(strip()).getAllByRole('button')
}

const scrollSpy = vi.fn()
function mockScroll() {
  scrollSpy.mockClear()
  Element.prototype.scrollIntoView = scrollSpy as unknown as typeof Element.prototype.scrollIntoView
}

afterEach(() => {
  scrollSpy.mockClear()
})

describe('MediaCarousel', () => {
  it('AJU-009/n = 0, items undefined ou erro: placeholder, sem rótulo, sem indicador e sem faixa', () => {
    for (const items of [[], undefined]) {
      const { container, unmount } = render(<MediaCarousel items={items} resetKey={1} />)
      expect(shownSrc(container)).toBe(coverPlaceholderUrl)
      expect(screen.queryByText(/ de \d+$/)).toBeNull()
      expect(screen.queryByText('Capa')).toBeNull()
      expect(screen.queryByRole('group', { name: 'Miniaturas' })).toBeNull()
      expect(screen.queryAllByRole('button')).toHaveLength(0)
      unmount()
    }
  })

  it('AJU-009/n = 1: imagem, rótulo e "1 de 1", sem faixa', () => {
    const { container } = render(<MediaCarousel items={[item(7, 'screenshot')]} resetKey={1} />)
    expect(shownSrc(container)).toBe('/media/7.jpg')
    expect(screen.getByText('Screenshot')).toBeTruthy()
    expect(screen.getByText('1 de 1')).toBeTruthy()
    expect(screen.queryByRole('group', { name: 'Miniaturas' })).toBeNull()
    expect(screen.queryAllByRole('button')).toHaveLength(0)
  })

  it('AJU-009/n = 3 com tipos misturados: a primeira em destaque é a capa e a ordem segue carouselKindOrder', () => {
    const { container } = render(<MediaCarousel items={three} resetKey={1} />)
    expect(shownSrc(container)).toBe('/media/2.jpg')
    expect(screen.getByText('Capa')).toBeTruthy()
    expect(screen.getByText('1 de 3')).toBeTruthy()
    expect(thumbs().map((b) => b.getAttribute('aria-label'))).toEqual([
      'Capa, 1 de 3',
      'Screenshot, 2 de 3',
      'Banner, 3 de 3',
    ])
  })

  it('AJU-009/clicar na imagem em destaque não muda nada', () => {
    const { container } = render(<MediaCarousel items={three} resetKey={1} />)
    fireEvent.click(container.querySelector('img') as HTMLImageElement)
    expect(shownSrc(container)).toBe('/media/2.jpg')
    expect(screen.getByText('1 de 3')).toBeTruthy()
  })

  it('AJU-010/n ≥ 2: group "Miniaturas" com um botão por imagem, nome "<tipo>, k de n", thumbUrl (url se vazia)', () => {
    render(<MediaCarousel items={[item(1, 'cover'), item(2, 'screenshot', '')]} resetKey={1} />)
    const bs = thumbs()
    expect(bs).toHaveLength(2)
    expect(bs[0].getAttribute('aria-label')).toBe('Capa, 1 de 2')
    expect(bs[1].getAttribute('aria-label')).toBe('Screenshot, 2 de 2')
    expect(bs[0].querySelector('img')?.getAttribute('src')).toBe('/thumb/1.jpg')
    expect(bs[1].querySelector('img')?.getAttribute('src')).toBe('/media/2.jpg')
  })

  it('AJU-010/só a miniatura em destaque tem aria-current="true" e tabIndex 0', () => {
    render(<MediaCarousel items={three} resetKey={1} />)
    const bs = thumbs()
    expect(bs.map((b) => b.getAttribute('aria-current'))).toEqual(['true', null, null])
    expect(bs.map((b) => b.tabIndex)).toEqual([0, -1, -1])
    fireEvent.click(bs[2])
    const after = thumbs()
    expect(after.map((b) => b.getAttribute('aria-current'))).toEqual([null, null, 'true'])
    expect(after.map((b) => b.tabIndex)).toEqual([-1, -1, 0])
  })

  it('AJU-010/não há botões "Imagem anterior" nem "Próxima imagem"', () => {
    render(<MediaCarousel items={three} resetKey={1} />)
    expect(screen.queryByRole('button', { name: 'Imagem anterior' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Próxima imagem' })).toBeNull()
  })

  it('AJU-010/a faixa tem a margem interna GRID_INSET', () => {
    render(<MediaCarousel items={three} resetKey={1} />)
    const s = strip()
    expect(s.className).toContain('overflow-x-auto')
    expect(s.style.padding).toBe(`${GRID_INSET}px`)
  })

  it('AJU-011/clicar na miniatura k a põe em destaque, muda rótulo e indicador e dá o foco a k', () => {
    mockScroll()
    const { container } = render(<MediaCarousel items={three} resetKey={1} />)
    fireEvent.click(thumbs()[2])
    expect(shownSrc(container)).toBe('/media/3.jpg')
    expect(screen.getByText('3 de 3')).toBeTruthy()
    expect(screen.getAllByText('Banner').length).toBeGreaterThan(0)
    expect(document.activeElement).toBe(thumbs()[2])
  })

  it('AJU-011/clicar na já em destaque não muda nada', () => {
    const { container } = render(<MediaCarousel items={three} resetKey={1} />)
    fireEvent.click(thumbs()[0])
    expect(shownSrc(container)).toBe('/media/2.jpg')
    expect(screen.getByText('1 de 3')).toBeTruthy()
  })

  it('AJU-012/seta direita e esquerda na faixa movem destaque e foco; Home e End vão às pontas', () => {
    mockScroll()
    const { container } = render(<MediaCarousel items={three} resetKey={1} />)
    thumbs()[0].focus()
    fireEvent.keyDown(thumbs()[0], { key: 'ArrowRight' })
    expect(shownSrc(container)).toBe('/media/1.jpg')
    expect(document.activeElement).toBe(thumbs()[1])
    fireEvent.keyDown(thumbs()[1], { key: 'ArrowLeft' })
    expect(shownSrc(container)).toBe('/media/2.jpg')
    expect(document.activeElement).toBe(thumbs()[0])
    fireEvent.keyDown(thumbs()[0], { key: 'End' })
    expect(shownSrc(container)).toBe('/media/3.jpg')
    expect(document.activeElement).toBe(thumbs()[2])
    fireEvent.keyDown(thumbs()[2], { key: 'Home' })
    expect(shownSrc(container)).toBe('/media/2.jpg')
    expect(document.activeElement).toBe(thumbs()[0])
  })

  it('AJU-012/seta direita na última e esquerda na primeira não fazem nada e não chamam preventDefault', () => {
    mockScroll()
    const { container } = render(<MediaCarousel items={three} resetKey={1} />)
    const first = new KeyboardEvent('keydown', { key: 'ArrowLeft', bubbles: true, cancelable: true })
    thumbs()[0].dispatchEvent(first)
    expect(first.defaultPrevented).toBe(false)
    expect(shownSrc(container)).toBe('/media/2.jpg')
    fireEvent.click(thumbs()[2])
    const last = new KeyboardEvent('keydown', { key: 'ArrowRight', bubbles: true, cancelable: true })
    thumbs()[2].dispatchEvent(last)
    expect(last.defaultPrevented).toBe(false)
    expect(shownSrc(container)).toBe('/media/3.jpg')
  })

  // Six kinds, so the display order is ids 1..6 (carouselKindOrder).
  const six = [item(1, 'cover'), item(2, 'screenshot'), item(3, 'title'), item(4, 'select'), item(5, 'background'), item(6, 'banner')]

  it('AJU-012/seta direita na miniatura 4 com destaque 0 vai para a 5', () => {
    mockScroll()
    const { container } = render(<MediaCarousel items={six} resetKey={1} />)
    expect(thumbs()).toHaveLength(6)
    thumbs()[4].focus()
    expect(screen.getByText('1 de 6')).toBeTruthy()
    fireEvent.keyDown(thumbs()[4], { key: 'ArrowRight' })
    expect(shownSrc(container)).toBe('/media/6.jpg')
    expect(screen.getByText('6 de 6')).toBeTruthy()
    expect(document.activeElement).toBe(thumbs()[5])
  })

  it('AJU-012/seta esquerda na miniatura 0 focada não age', () => {
    mockScroll()
    const { container } = render(<MediaCarousel items={six} resetKey={1} />)
    fireEvent.click(thumbs()[3])
    thumbs()[0].focus()
    const ev = new KeyboardEvent('keydown', { key: 'ArrowLeft', bubbles: true, cancelable: true })
    thumbs()[0].dispatchEvent(ev)
    expect(ev.defaultPrevented).toBe(false)
    expect(shownSrc(container)).toBe('/media/4.jpg')
    expect(screen.getByText('4 de 6')).toBeTruthy()
    expect(document.activeElement).toBe(thumbs()[0])
  })

  it('AJU-012/Home e End a partir da focada', () => {
    mockScroll()
    const { container } = render(<MediaCarousel items={six} resetKey={1} />)
    thumbs()[2].focus()
    fireEvent.keyDown(thumbs()[2], { key: 'End' })
    expect(shownSrc(container)).toBe('/media/6.jpg')
    expect(document.activeElement).toBe(thumbs()[5])
    fireEvent.click(thumbs()[1])
    thumbs()[3].focus()
    fireEvent.keyDown(thumbs()[3], { key: 'Home' })
    expect(shownSrc(container)).toBe('/media/1.jpg')
    expect(document.activeElement).toBe(thumbs()[0])
  })

  it('AJU-012/a nova miniatura chama scrollIntoView com inline nearest', () => {
    mockScroll()
    render(<MediaCarousel items={three} resetKey={1} />)
    fireEvent.keyDown(thumbs()[0], { key: 'ArrowRight' })
    expect(scrollSpy).toHaveBeenCalledWith({ inline: 'nearest', block: 'nearest' })
  })

  it('AJU-012/setas com o foco fora da faixa não mudam a imagem; cima e baixo nunca são tratadas', () => {
    mockScroll()
    const { container } = render(<MediaCarousel items={three} resetKey={1} />)
    const region = screen.getByRole('region', { name: /Imagens/ })
    fireEvent.keyDown(region, { key: 'ArrowRight' })
    expect(shownSrc(container)).toBe('/media/2.jpg')
    for (const key of ['ArrowUp', 'ArrowDown']) {
      const ev = new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true })
      thumbs()[0].dispatchEvent(ev)
      expect(ev.defaultPrevented).toBe(false)
    }
    expect(shownSrc(container)).toBe('/media/2.jpg')
  })

  it('AJU-012/Tab para uma vez na faixa, na miniatura em destaque', () => {
    render(<MediaCarousel items={three} resetKey={1} />)
    const tabbable = thumbs().filter((b) => b.tabIndex >= 0)
    expect(tabbable).toHaveLength(1)
    expect(tabbable[0].getAttribute('aria-current')).toBe('true')
  })

  it('AJU-013/trocar resetKey volta à primeira imagem e a faixa ao início (scrollLeft 0)', () => {
    mockScroll()
    const { container, rerender } = render(<MediaCarousel items={three} resetKey={1} />)
    fireEvent.click(thumbs()[2])
    strip().scrollLeft = 80
    rerender(<MediaCarousel items={three} resetKey={2} />)
    expect(shownSrc(container)).toBe('/media/2.jpg')
    expect(screen.getByText('1 de 3')).toBeTruthy()
    expect(strip().scrollLeft).toBe(0)
  })

  it('IMG-009/trocar items do mesmo jogo (resetKey igual) não volta à primeira imagem', () => {
    mockScroll()
    const { container, rerender } = render(<MediaCarousel items={three} resetKey={1} />)
    fireEvent.click(thumbs()[2])
    rerender(<MediaCarousel items={[...three, item(9, 'title')]} resetKey={1} />)
    expect(shownSrc(container)).toBe('/media/3.jpg')
    expect(screen.getByText('4 de 4')).toBeTruthy()
  })

  it('IMG-009/trocar resetKey continua levando à primeira imagem, mesmo com a lista nova', () => {
    mockScroll()
    const { container, rerender } = render(<MediaCarousel items={three} resetKey={1} />)
    fireEvent.click(thumbs()[2])
    rerender(<MediaCarousel items={[item(9, 'cover'), item(8, 'screenshot')]} resetKey={2} />)
    expect(shownSrc(container)).toBe('/media/9.jpg')
    expect(screen.getByText('1 de 2')).toBeTruthy()
  })

  it('IMG-009/imagem nova depois de M: M continua em destaque, "k de n" e faixa com a lista nova', () => {
    mockScroll()
    const { container, rerender } = render(<MediaCarousel items={three} resetKey={1} />)
    fireEvent.click(thumbs()[1]) // screenshot 1
    rerender(<MediaCarousel items={[...three, item(5, 'title')]} resetKey={1} />)
    expect(shownSrc(container)).toBe('/media/1.jpg')
    expect(screen.getByText('2 de 4')).toBeTruthy()
    expect(thumbs().map((b) => b.getAttribute('aria-label'))).toEqual([
      'Capa, 1 de 4',
      'Screenshot, 2 de 4',
      'Tela de título, 3 de 4',
      'Banner, 4 de 4',
    ])
    expect(thumbs().map((b) => b.getAttribute('aria-current'))).toEqual([null, 'true', null, null])
  })

  it('IMG-009/imagem nova antes de M: M continua em destaque e k passa a ser a nova posição', () => {
    mockScroll()
    const { container, rerender } = render(<MediaCarousel items={three} resetKey={1} />)
    fireEvent.click(thumbs()[2]) // banner 3
    rerender(<MediaCarousel items={[...three, item(5, 'title')]} resetKey={1} />)
    expect(shownSrc(container)).toBe('/media/3.jpg')
    expect(screen.getByText('4 de 4')).toBeTruthy()
    expect(thumbs()[3].getAttribute('aria-current')).toBe('true')
  })

  it('IMG-009/sem clique, uma capa que chega antes da imagem mostrada não troca o destaque', () => {
    mockScroll()
    const start = [item(1, 'screenshot'), item(3, 'banner')]
    const { container, rerender } = render(<MediaCarousel items={start} resetKey={1} />)
    expect(shownSrc(container)).toBe('/media/1.jpg')
    rerender(<MediaCarousel items={[...start, item(9, 'cover')]} resetKey={1} />)
    expect(shownSrc(container)).toBe('/media/1.jpg')
    expect(screen.getByText('2 de 3')).toBeTruthy()
    expect(thumbs()[0].getAttribute('aria-label')).toBe('Capa, 1 de 3')
  })

  it('IMG-009/a miniatura de M com o foco do teclado continua com o foco depois da lista nova', () => {
    mockScroll()
    const { rerender } = render(<MediaCarousel items={three} resetKey={1} />)
    fireEvent.click(thumbs()[2]) // banner 3, focused
    expect(document.activeElement).toBe(thumbs()[2])
    const before = document.activeElement
    rerender(<MediaCarousel items={[...three, item(5, 'title')]} resetKey={1} />)
    expect(document.activeElement).toBe(before)
    expect(document.activeElement).toBe(thumbs()[3])
  })

  it('IMG-009/M mudou de posição: scrollIntoView com inline nearest, sem mover o foco', () => {
    mockScroll()
    const { rerender } = render(<MediaCarousel items={three} resetKey={1} />)
    expect(scrollSpy).not.toHaveBeenCalled()
    fireEvent.click(thumbs()[2]) // banner 3
    thumbs()[0].focus()
    scrollSpy.mockClear()
    rerender(<MediaCarousel items={[...three, item(5, 'title')]} resetKey={1} />)
    expect(scrollSpy).toHaveBeenCalledWith({ inline: 'nearest', block: 'nearest' })
    expect(document.activeElement).toBe(thumbs()[0])
  })

  it('IMG-009/lista vazia que ganha uma imagem: ela aparece em destaque', () => {
    mockScroll()
    for (const empty of [[], undefined]) {
      const { container, rerender, unmount } = render(<MediaCarousel items={empty} resetKey={1} />)
      expect(shownSrc(container)).toBe(coverPlaceholderUrl)
      rerender(<MediaCarousel items={[item(4, 'screenshot')]} resetKey={1} />)
      expect(shownSrc(container)).toBe('/media/4.jpg')
      expect(screen.getByText('1 de 1')).toBeTruthy()
      unmount()
    }
  })

  it('IMG-009/M removida da lista nova: a primeira imagem fica em destaque', () => {
    mockScroll()
    const { container, rerender } = render(<MediaCarousel items={three} resetKey={1} />)
    fireEvent.click(thumbs()[2]) // banner 3
    rerender(<MediaCarousel items={[item(2, 'cover'), item(1, 'screenshot')]} resetKey={1} />)
    expect(shownSrc(container)).toBe('/media/2.jpg')
    expect(screen.getByText('1 de 2')).toBeTruthy()
  })

  it('IMG-009/erro no destaque não volta a ser tentado só porque a lista cresceu; volta ao trocar o destaque', () => {
    mockScroll()
    const { container, rerender } = render(<MediaCarousel items={three} resetKey={1} />)
    fireEvent.error(container.querySelector('img') as HTMLImageElement)
    expect(shownSrc(container)).toBe(coverPlaceholderUrl)
    rerender(<MediaCarousel items={[...three, item(5, 'title')]} resetKey={1} />)
    expect(shownSrc(container)).toBe(coverPlaceholderUrl)
    expect(screen.getByText('1 de 4')).toBeTruthy()
    fireEvent.click(thumbs()[1])
    expect(shownSrc(container)).toBe('/media/1.jpg')
  })

  it('AJU-013/erro na imagem em destaque mostra o placeholder; rótulo, indicador e faixa continuam', () => {
    const { container } = render(<MediaCarousel items={three} resetKey={1} />)
    fireEvent.error(container.querySelector('img') as HTMLImageElement)
    expect(shownSrc(container)).toBe(coverPlaceholderUrl)
    expect(screen.getByText('Capa')).toBeTruthy()
    expect(screen.getByText('1 de 3')).toBeTruthy()
    expect(thumbs()).toHaveLength(3)
  })

  it('AJU-013/erro numa miniatura mostra o placeholder só nela', () => {
    render(<MediaCarousel items={three} resetKey={1} />)
    const bs = thumbs()
    fireEvent.error(bs[1].querySelector('img') as HTMLImageElement)
    expect(thumbs()[1].querySelector('img')?.getAttribute('src')).toBe(coverPlaceholderUrl)
    expect(thumbs()[0].querySelector('img')?.getAttribute('src')).toBe('/thumb/2.jpg')
    expect(thumbs()[2].querySelector('img')?.getAttribute('src')).toBe('/thumb/3.jpg')
  })

  it('UIX-043/o carrossel é navegável só pelo teclado', () => {
    render(<MediaCarousel items={three} resetKey={1} />)
    const buttons = screen.getAllByRole('button')
    for (const b of buttons) {
      expect(b.tagName).toBe('BUTTON')
    }
    const focusable = buttons.filter((b) => b.tabIndex >= 0)
    expect(focusable.length).toBeGreaterThan(0)
    focusable[0].focus()
    expect(document.activeElement).toBe(focusable[0])
  })
})
