import { act, fireEvent, render } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { FocusVideo } from './FocusVideo'

// Assumed (design §4.3): FocusVideo({ gameId, url, active, sound, failed, onError }) renders null until useVideoDelay is true,
// then a single <video data-testid="bb-video"> with object-contain, loop and playsInline; src removed on unmount.
type P = Parameters<typeof FocusVideo>[0]
const DELAY = 2000
const base = (): P => ({ gameId: 1, url: 'http://m/1/v.mp4', active: true, sound: false, failed: false, onError: vi.fn() })

beforeEach(() => {
  vi.useFakeTimers()
  vi.spyOn(HTMLMediaElement.prototype, 'play').mockImplementation(() => Promise.resolve())
  vi.spyOn(HTMLMediaElement.prototype, 'pause').mockImplementation(() => {})
})
afterEach(() => {
  vi.useRealTimers()
  vi.restoreAllMocks()
})

const videos = (c: HTMLElement) => c.querySelectorAll('video')
const advance = (ms: number) => act(() => { vi.advanceTimersByTime(ms) })

describe('FocusVideo', () => {
  it('BBX-053/nada antes de 2.000 ms; depois um único video muted, loop, playsInline e object-contain com o src', () => {
    const { container } = render(<FocusVideo {...base()} />)
    expect(videos(container)).toHaveLength(0)
    advance(DELAY - 1)
    expect(videos(container)).toHaveLength(0)
    advance(1)
    expect(videos(container)).toHaveLength(1)
    const v = videos(container)[0] as HTMLVideoElement
    expect(v.getAttribute('src')).toBe('http://m/1/v.mp4')
    expect(v.muted).toBe(true)
    expect(v.loop).toBe(true)
    expect(v.hasAttribute('playsinline') || v.playsInline).toBe(true)
    expect(v.className).toContain('object-contain')
    expect(v.className).not.toContain('object-cover')
    expect(HTMLMediaElement.prototype.play).toHaveBeenCalled()
  })

  it('BBX-053/active false (fora de Navegar) nunca toca e desmonta o que tocava', () => {
    const { container, rerender } = render(<FocusVideo {...base()} active={false} />)
    advance(DELAY * 3)
    expect(videos(container)).toHaveLength(0)
    rerender(<FocusVideo {...base()} active />)
    advance(DELAY)
    expect(videos(container)).toHaveLength(1)
    rerender(<FocusVideo {...base()} active={false} />)
    expect(videos(container)).toHaveLength(0)
  })

  it('BBX-053/trocar de foco: o vídeo some, o src antigo não fica no DOM e o novo só toca depois de 2.000 ms', () => {
    const { container, rerender } = render(<FocusVideo {...base()} />)
    advance(DELAY)
    expect(videos(container)).toHaveLength(1)
    rerender(<FocusVideo {...base()} gameId={2} url="http://m/2/v.mp4" />)
    expect(videos(container)).toHaveLength(0)
    expect(container.innerHTML).not.toContain('http://m/1/v.mp4')
    advance(DELAY - 1)
    expect(videos(container)).toHaveLength(0)
    advance(1)
    expect(videos(container)[0]?.getAttribute('src')).toBe('http://m/2/v.mp4')
  })

  it('BBX-053/troca de foco antes de 2.000 ms: a contagem recomeça no jogo novo', () => {
    const { container, rerender } = render(<FocusVideo {...base()} />)
    advance(1500)
    rerender(<FocusVideo {...base()} gameId={2} url="http://m/2/v.mp4" />)
    advance(1500)
    expect(videos(container)).toHaveLength(0)
    advance(500)
    expect(videos(container)).toHaveLength(1)
  })

  it('BBX-053/evento error chama onError(gameId) uma vez e o vídeo some; com failed true não tenta de novo', () => {
    const p = base()
    const { container, rerender } = render(<FocusVideo {...p} />)
    advance(DELAY)
    fireEvent.error(videos(container)[0])
    expect(p.onError).toHaveBeenCalledTimes(1)
    expect(p.onError).toHaveBeenCalledWith(1)
    rerender(<FocusVideo {...p} failed />)
    expect(videos(container)).toHaveLength(0)
    advance(DELAY * 3)
    expect(videos(container)).toHaveLength(0)
    expect(p.onError).toHaveBeenCalledTimes(1)
  })

  it('BBX-053/failed true desde o início nunca toca; sair do jogo e voltar (failed limpo) toca de novo', () => {
    const { container, rerender } = render(<FocusVideo {...base()} failed />)
    advance(DELAY * 3)
    expect(videos(container)).toHaveLength(0)
    rerender(<FocusVideo {...base()} gameId={2} url="http://m/2/v.mp4" failed={false} />)
    advance(DELAY)
    expect(videos(container)).toHaveLength(1)
  })

  it('BBX-053/sound liga e desliga o som sem recriar o elemento (Y)', () => {
    const { container, rerender } = render(<FocusVideo {...base()} sound={false} />)
    advance(DELAY)
    const v = videos(container)[0] as HTMLVideoElement
    expect(v.muted).toBe(true)
    rerender(<FocusVideo {...base()} sound />)
    expect(videos(container)[0]).toBe(v)
    expect(v.muted).toBe(false)
    rerender(<FocusVideo {...base()} sound={false} />)
    expect(v.muted).toBe(true)
  })

  it('BBX-053/sem som desde o início o video nasce com sound true e não muted', () => {
    const { container } = render(<FocusVideo {...base()} sound />)
    advance(DELAY)
    expect((videos(container)[0] as HTMLVideoElement).muted).toBe(false)
  })

  it('BBX-053/desmontar remove o elemento, tira o src e não deixa temporizador', () => {
    const { container, unmount } = render(<FocusVideo {...base()} />)
    advance(DELAY)
    const v = videos(container)[0] as HTMLVideoElement
    unmount()
    expect(videos(container)).toHaveLength(0)
    expect(v.hasAttribute('src')).toBe(false)
    advance(DELAY * 2)
    expect(videos(container)).toHaveLength(0)
  })

  it('BBX-053/nunca há mais de um video, mesmo trocando de jogo várias vezes', () => {
    const { container, rerender } = render(<FocusVideo {...base()} />)
    for (let id = 1; id <= 4; id++) {
      rerender(<FocusVideo {...base()} gameId={id} url={`http://m/${id}/v.mp4`} />)
      expect(videos(container).length).toBeLessThanOrEqual(1)
      advance(DELAY)
      expect(videos(container)).toHaveLength(1)
    }
  })

  it('BBX-053/o vídeo é decorativo: sem controles', () => {
    const { container } = render(<FocusVideo {...base()} />)
    advance(DELAY)
    expect((videos(container)[0] as HTMLVideoElement).controls).toBe(false)
  })
})
