import { describe, expect, it } from 'vitest'
import { fireEvent, render } from '@testing-library/react'
import { CoverImage, coverPlaceholderUrl } from './CoverImage'

function imgOf(container: HTMLElement): HTMLImageElement {
  const img = container.querySelector('img')
  expect(img, 'CoverImage renders one <img>').not.toBeNull()
  return img as HTMLImageElement
}

describe('CoverImage', () => {
  it('UIX-015/com src mostra a imagem com object-fit contain na área 2:3', () => {
    const { container } = render(<CoverImage src="/media/1/c.jpg" />)
    const img = imgOf(container)
    expect(img.getAttribute('src')).toBe('/media/1/c.jpg')
    const all = [container, ...Array.from(container.querySelectorAll<HTMLElement>('*'))]
    const styled = all.map((e) => `${e.getAttribute('class') ?? ''} ${e.getAttribute('style') ?? ''}`).join(' ')
    expect(styled).toMatch(/object-contain|object-fit:\s*contain/)
    expect(styled).toMatch(/aspect-\[2\/3\]|aspect-ratio:\s*2\s*\/\s*3/)
  })

  it('UIX-015/src vazio ou ausente mostra o placeholder do mesmo tamanho', () => {
    expect(coverPlaceholderUrl).toBeTruthy()
    const withCover = render(<CoverImage src="/media/1/c.jpg" />)
    const coverBox = (withCover.container.firstElementChild as HTMLElement).className
    withCover.unmount()

    const empty = render(<CoverImage src="" />)
    expect(imgOf(empty.container).getAttribute('src')).toBe(coverPlaceholderUrl)
    expect((empty.container.firstElementChild as HTMLElement).className).toBe(coverBox)
    empty.unmount()

    const missing = render(<CoverImage />)
    expect(imgOf(missing.container).getAttribute('src')).toBe(coverPlaceholderUrl)
  })

  it('UIX-015/evento error da imagem troca pelo placeholder', () => {
    const { container } = render(<CoverImage src="/media/1/broken.jpg" />)
    fireEvent.error(imgOf(container))
    expect(imgOf(container).getAttribute('src')).toBe(coverPlaceholderUrl)
  })

  it('UIX-015/trocar src depois de um erro volta a tentar a imagem nova', () => {
    const { container, rerender } = render(<CoverImage src="/media/1/broken.jpg" />)
    fireEvent.error(imgOf(container))
    expect(imgOf(container).getAttribute('src')).toBe(coverPlaceholderUrl)
    rerender(<CoverImage src="/media/1/new.jpg" />)
    expect(imgOf(container).getAttribute('src')).toBe('/media/1/new.jpg')
  })

  it('UIX-042/imagem e placeholder têm alt vazio', () => {
    const withCover = render(<CoverImage src="/media/1/c.jpg" />)
    expect(imgOf(withCover.container).getAttribute('alt')).toBe('')
    withCover.unmount()
    const placeholder = render(<CoverImage />)
    expect(imgOf(placeholder.container).getAttribute('alt')).toBe('')
  })
})
