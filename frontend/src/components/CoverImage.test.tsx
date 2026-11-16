import { describe, expect, it } from 'vitest'
import { fireEvent, render } from '@testing-library/react'
import { CoverImage, coverPlaceholderUrl } from './CoverImage'

function imgOf(container: HTMLElement): HTMLImageElement {
  const img = container.querySelector('img')
  expect(img, 'CoverImage renders one <img>').not.toBeNull()
  return img as HTMLImageElement
}

describe('CoverImage', () => {
  it('UIX-015/com src mostra a imagem com object-fit contain na área 3:4', () => {
    const { container } = render(<CoverImage src="/media/1/c.jpg" />)
    const img = imgOf(container)
    expect(img.getAttribute('src')).toBe('/media/1/c.jpg')
    const all = [container, ...Array.from(container.querySelectorAll<HTMLElement>('*'))]
    const styled = all.map((e) => `${e.getAttribute('class') ?? ''} ${e.getAttribute('style') ?? ''}`).join(' ')
    expect(styled).toMatch(/object-contain|object-fit:\s*contain/)
    expect(styled).toMatch(/aspect-\[3\/4\]|aspect-ratio:\s*3\s*\/\s*4/)
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

  it('AJU-016/fit contain: data-fit="contain" e object-contain', () => {
    const { container } = render(<CoverImage src="/m/c.jpg" fit="contain" />)
    const img = imgOf(container)
    expect(img).toHaveAttribute('data-fit', 'contain')
    expect(img.className).toContain('object-contain')
    const dflt = render(<CoverImage src="/m/c.jpg" />)
    expect(imgOf(dflt.container)).toHaveAttribute('data-fit', 'contain')
  })

  it('AJU-016/fit cover: data-fit="cover" e object-cover object-center', () => {
    const { container } = render(<CoverImage src="/m/t.jpg" fit="cover" />)
    const img = imgOf(container)
    expect(img).toHaveAttribute('data-fit', 'cover')
    expect(img.className).toContain('object-cover')
    expect(img.className).toContain('object-center')
    expect(img.className).not.toContain('object-contain')
  })

  it('AJU-016/substituta que falha mostra o placeholder, sem data-fit (não tenta a próxima)', () => {
    const { container } = render(<CoverImage src="/m/t.jpg" fit="cover" />)
    fireEvent.error(imgOf(container))
    const img = imgOf(container)
    expect(img.getAttribute('src')).toBe(coverPlaceholderUrl)
    expect(img).not.toHaveAttribute('data-fit')
    const none = render(<CoverImage fit="cover" />)
    expect(imgOf(none.container)).not.toHaveAttribute('data-fit')
  })

  it('UIX-042/imagem e placeholder têm alt vazio', () => {
    const withCover = render(<CoverImage src="/media/1/c.jpg" />)
    expect(imgOf(withCover.container).getAttribute('alt')).toBe('')
    withCover.unmount()
    const placeholder = render(<CoverImage />)
    expect(imgOf(placeholder.container).getAttribute('alt')).toBe('')
  })
})
