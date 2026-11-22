import { act, fireEvent, render } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  ART_FADE_MS,
  BackgroundStage,
  VIDEO_DELAY_MS,
  VIDEO_FADE_MS,
  type BackgroundStageProps,
} from './BackgroundStage'

// Assumed markup: art layers are <img data-testid="bb-art">; the single video is <video data-testid="bb-video">;
// the no-art fallback is data-testid="bb-gradient" (inline style/class using var(--arc-bb-bg) and var(--arc-bb-bg-2)).
// alterado em 2026-11-22 (fase 15): `backdrop` é obrigatório (BBX-052); os casos antigos valem para 'contain' (Faixa).
const base: BackgroundStageProps = {
  backdrop: 'contain',
  gameId: 1,
  artUrl: 'http://media/1/bg.jpg',
  videoUrl: 'http://media/1/v.mp4',
  active: true,
  sound: false,
  failed: false,
  onVideoError: () => {},
  onVideoVisible: () => {},
}

const videos = (c: HTMLElement) => c.querySelectorAll('video')

beforeEach(() => {
  vi.useFakeTimers()
  vi.spyOn(HTMLMediaElement.prototype, 'play').mockImplementation(() => Promise.resolve())
  vi.spyOn(HTMLMediaElement.prototype, 'pause').mockImplementation(() => {})
})
afterEach(() => {
  vi.useRealTimers()
  vi.restoreAllMocks()
})

describe('BackgroundStage', () => {
  it('BBX-011/com vídeo e foco parado 2.000 ms um único video muted, loop e playsInline passa a tocar sobre a arte', () => {
    const { container } = render(<BackgroundStage {...base} />)
    expect(videos(container)).toHaveLength(0)
    act(() => { vi.advanceTimersByTime(VIDEO_DELAY_MS - 1) })
    expect(videos(container)).toHaveLength(0)
    act(() => { vi.advanceTimersByTime(1) })
    const v = container.querySelector('video') as HTMLVideoElement
    expect(v).not.toBeNull()
    expect(v.muted).toBe(true)
    expect(v.loop).toBe(true)
    expect(v.hasAttribute('playsinline') || v.playsInline).toBe(true)
    expect(v.getAttribute('src')).toBe('http://media/1/v.mp4')
    expect(container.querySelector('img[data-testid="bb-art"]')).not.toBeNull()
  })

  it('BBX-011/troca de foco antes de 2.000 ms: nada toca e a contagem recomeça no jogo novo', () => {
    const { container, rerender } = render(<BackgroundStage {...base} />)
    act(() => { vi.advanceTimersByTime(1500) })
    rerender(<BackgroundStage {...base} gameId={2} artUrl="http://media/2/bg.jpg" videoUrl="http://media/2/v.mp4" />)
    act(() => { vi.advanceTimersByTime(1500) })
    expect(videos(container)).toHaveLength(0)
    act(() => { vi.advanceTimersByTime(500) })
    expect(container.querySelector('video')?.getAttribute('src')).toBe('http://media/2/v.mp4')
  })

  it('BBX-011/troca de foco com vídeo tocando: o vídeo para e o src é removido', () => {
    const { container, rerender } = render(<BackgroundStage {...base} />)
    act(() => { vi.advanceTimersByTime(VIDEO_DELAY_MS) })
    expect(videos(container)).toHaveLength(1)
    rerender(<BackgroundStage {...base} gameId={2} videoUrl="http://media/2/v.mp4" />)
    expect(videos(container)).toHaveLength(0)
    expect(container.innerHTML).not.toContain('http://media/1/v.mp4')
  })

  it('BBX-011/active=false (Detalhes) nunca toca', () => {
    const { container } = render(<BackgroundStage {...base} active={false} />)
    act(() => { vi.advanceTimersByTime(VIDEO_DELAY_MS * 3) })
    expect(videos(container)).toHaveLength(0)
  })

  it('BBX-011/active=false com vídeo tocando o desmonta', () => {
    const { container, rerender } = render(<BackgroundStage {...base} />)
    act(() => { vi.advanceTimersByTime(VIDEO_DELAY_MS) })
    rerender(<BackgroundStage {...base} active={false} />)
    expect(videos(container)).toHaveLength(0)
  })

  it('BBX-011/jogo sem vídeo: nunca há video', () => {
    const { container } = render(<BackgroundStage {...base} videoUrl={null} />)
    act(() => { vi.advanceTimersByTime(VIDEO_DELAY_MS * 2) })
    expect(videos(container)).toHaveLength(0)
  })

  it('BBX-011/nunca há mais de um elemento video', () => {
    const { container, rerender } = render(<BackgroundStage {...base} />)
    for (let id = 1; id <= 4; id++) {
      rerender(<BackgroundStage {...base} gameId={id} videoUrl={`http://media/${id}/v.mp4`} />)
      expect(videos(container).length).toBeLessThanOrEqual(1)
      act(() => { vi.advanceTimersByTime(VIDEO_DELAY_MS) })
      expect(videos(container)).toHaveLength(1)
    }
  })

  it('BBX-011/transições de 300 ms (arte) e 400 ms (vídeo)', () => {
    expect(ART_FADE_MS).toBe(300)
    expect(VIDEO_FADE_MS).toBe(400)
    expect(VIDEO_DELAY_MS).toBe(2000)
    const { container } = render(<BackgroundStage {...base} />)
    act(() => { vi.advanceTimersByTime(VIDEO_DELAY_MS) })
    const art = container.querySelector('[data-testid="bb-art-layer"]') as HTMLElement
    const vid = container.querySelector('[data-testid="bb-video"]') as HTMLElement
    expect(art.style.transition).toContain('opacity')
    expect(art.style.transition).toContain('300ms')
    expect(vid.style.transition).toContain('opacity')
    expect(vid.style.transition).toContain('400ms')
  })

  it('BBX-012/evento error volta à arte e chama onVideoError', () => {
    const onVideoError = vi.fn()
    const { container } = render(<BackgroundStage {...base} onVideoError={onVideoError} />)
    act(() => { vi.advanceTimersByTime(VIDEO_DELAY_MS) })
    fireEvent.error(container.querySelector('video') as HTMLVideoElement)
    expect(onVideoError).toHaveBeenCalledWith(1)
    expect(container.querySelector('img[data-testid="bb-art"]')).not.toBeNull()
  })

  it('BBX-012/failed=true não tenta de novo até o foco sair do jogo', () => {
    const { container, rerender } = render(<BackgroundStage {...base} failed />)
    act(() => { vi.advanceTimersByTime(VIDEO_DELAY_MS * 3) })
    expect(videos(container)).toHaveLength(0)
    // focus moves to another game (failed cleared by the reducer): plays again
    rerender(<BackgroundStage {...base} gameId={2} videoUrl="http://media/2/v.mp4" failed={false} />)
    act(() => { vi.advanceTimersByTime(VIDEO_DELAY_MS) })
    expect(videos(container)).toHaveLength(1)
  })

  it('BBX-013/sound=false: muted; sound=true: não muted', () => {
    const { container, rerender } = render(<BackgroundStage {...base} />)
    act(() => { vi.advanceTimersByTime(VIDEO_DELAY_MS) })
    expect((container.querySelector('video') as HTMLVideoElement).muted).toBe(true)
    rerender(<BackgroundStage {...base} sound />)
    expect((container.querySelector('video') as HTMLVideoElement).muted).toBe(false)
  })

  it('BBX-013/onVideoVisible(true) só com o vídeo tocando', () => {
    const onVideoVisible = vi.fn()
    const { rerender } = render(<BackgroundStage {...base} onVideoVisible={onVideoVisible} />)
    expect(onVideoVisible).not.toHaveBeenCalledWith(true)
    act(() => { vi.advanceTimersByTime(VIDEO_DELAY_MS) })
    expect(onVideoVisible).toHaveBeenLastCalledWith(true)
    rerender(<BackgroundStage {...base} gameId={2} onVideoVisible={onVideoVisible} videoUrl="http://media/2/v.mp4" />)
    expect(onVideoVisible).toHaveBeenLastCalledWith(false)
  })

  it('BBX-010/arte trocada faz a transição de 300 ms entre as duas', () => {
    const { container, rerender } = render(<BackgroundStage {...base} videoUrl={null} />)
    rerender(<BackgroundStage {...base} gameId={2} artUrl="http://media/2/bg.jpg" videoUrl={null} />)
    const arts = Array.from(container.querySelectorAll('[data-testid="bb-art"]')) as HTMLElement[]
    expect(arts.map((a) => a.getAttribute('src'))).toContain('http://media/2/bg.jpg')
    expect(arts.length).toBeLessThanOrEqual(2)
    arts.forEach((a) => expect(a.closest('[data-testid="bb-art-layer"]')).not.toBeNull())
    act(() => { vi.advanceTimersByTime(ART_FADE_MS) })
    const after = Array.from(container.querySelectorAll('[data-testid="bb-art"]'))
    expect(after.map((a) => a.getAttribute('src'))).toEqual(['http://media/2/bg.jpg'])
  })

  it('BBX-010/a transição de 300 ms vale para o par preenchimento + arte (as duas camadas na troca, uma depois de ART_FADE_MS)', () => {
    const { container, rerender } = render(<BackgroundStage {...base} videoUrl={null} />)
    rerender(<BackgroundStage {...base} gameId={2} artUrl="http://media/2/bg.jpg" videoUrl={null} />)
    const layers = Array.from(container.querySelectorAll('[data-testid="bb-art-layer"]')) as HTMLElement[]
    expect(layers).toHaveLength(2)
    for (const l of layers) {
      expect(l.style.transition).toContain('opacity')
      expect(l.style.transition).toContain('300ms')
      expect(l.querySelector('[data-testid="bb-art-fill"]')).not.toBeNull()
      expect(l.querySelector('[data-testid="bb-art"]')).not.toBeNull()
    }
    act(() => { vi.advanceTimersByTime(ART_FADE_MS) })
    expect(container.querySelectorAll('[data-testid="bb-art-layer"]')).toHaveLength(1)
    expect(container.querySelectorAll('[data-testid="bb-art-fill"]')).toHaveLength(1)
  })

  it('BBX-040/com arte: um preenchimento (bb-art-fill) em object-cover com desfoque ≥ 24px e véu de --arc-bb-scrim, e a arte (bb-art) em object-contain acima', () => {
    const { container } = render(<BackgroundStage {...base} videoUrl={null} />)
    const fill = container.querySelector('[data-testid="bb-art-fill"]') as HTMLElement
    const art = container.querySelector('[data-testid="bb-art"]') as HTMLElement
    expect(fill.getAttribute('src')).toBe('http://media/1/bg.jpg')
    expect(fill.className).toContain('object-cover')
    const blur = /blur-\[(\d+)px\]/.exec(fill.className) ?? /blur\(\s*(\d+)px\s*\)/.exec(fill.getAttribute('style') ?? '')
    expect(blur, 'blur do preenchimento').not.toBeNull()
    expect(Number(blur![1])).toBeGreaterThanOrEqual(24)
    expect(art.className).toContain('object-contain')
    expect(art.className).not.toContain('object-cover')
    // veil with the scrim token between the fill and the art
    const layer = fill.closest('[data-testid="bb-art-layer"]') as HTMLElement
    expect(layer.innerHTML).toMatch(/--arc-bb-scrim|bb-scrim/)
    const kids = Array.from(layer.children)
    expect(kids.indexOf(art)).toBeGreaterThan(kids.indexOf(fill))
  })

  it('BBX-040/o vídeo usa object-contain sobre o preenchimento', () => {
    const { container } = render(<BackgroundStage {...base} />)
    act(() => { vi.advanceTimersByTime(VIDEO_DELAY_MS) })
    const v = container.querySelector('[data-testid="bb-video"]') as HTMLElement
    expect(v.className).toContain('object-contain')
    expect(v.className).not.toContain('object-cover')
    expect(container.querySelector('[data-testid="bb-art-fill"]')).not.toBeNull()
  })

  it('BBX-040/sem arte: só o gradiente, sem preenchimento', () => {
    const { container } = render(<BackgroundStage {...base} artUrl={null} videoUrl={null} />)
    expect(container.querySelector('[data-testid="bb-art-fill"]')).toBeNull()
    expect(container.querySelector('[data-testid="bb-art-layer"]')).toBeNull()
    expect(container.querySelector('[data-testid="bb-gradient"]')).not.toBeNull()
  })

  it('BBX-010/sem arte: gradiente de tokens', () => {
    const { container } = render(<BackgroundStage {...base} artUrl={null} videoUrl={null} />)
    expect(container.querySelector('img[data-testid="bb-art"]')).toBeNull()
    const g = container.querySelector('[data-testid="bb-gradient"]') as HTMLElement
    const css = `${g.getAttribute('style') ?? ''} ${g.className}`
    expect(css).toMatch(/--arc-bb-bg\b/)
    expect(css).toMatch(/--arc-bb-bg-2/)
  })

  it('BBX-011/o elemento é desmontado ao sair do Big Box', () => {
    const { container, unmount } = render(<BackgroundStage {...base} />)
    act(() => { vi.advanceTimersByTime(VIDEO_DELAY_MS) })
    expect(videos(container)).toHaveLength(1)
    // jsdom queues its own setImmediate when muted is set; only the component's setTimeout counts.
    const created: unknown[] = []
    const cleared: unknown[] = []
    const realSet = globalThis.setTimeout
    const realClear = globalThis.clearTimeout
    vi.spyOn(globalThis, 'setTimeout').mockImplementation(((...a: Parameters<typeof setTimeout>) => {
      const id = realSet(...a)
      created.push(id)
      return id
    }) as typeof setTimeout)
    vi.spyOn(globalThis, 'clearTimeout').mockImplementation(((id?: Parameters<typeof clearTimeout>[0]) => {
      cleared.push(id)
      return realClear(id)
    }) as typeof clearTimeout)
    unmount()
    expect(videos(container)).toHaveLength(0)
    for (const id of created) expect(cleared).toContain(id)
    act(() => { vi.advanceTimersByTime(VIDEO_DELAY_MS * 2) })
    expect(videos(container)).toHaveLength(0)
  })

  it('BBX-010/a arte é decorativa (aria-hidden) e não há som sem ação', () => {
    const { container } = render(<BackgroundStage {...base} />)
    const art = container.querySelector('[data-testid="bb-art"]') as HTMLElement
    expect(art.getAttribute('alt') ?? '').toBe('')
    expect(art.closest('[aria-hidden="true"]')).not.toBeNull()
  })
})

// ---- Fase 15 (BBX-052): fundo por tema. Marcação assumida para o degradê do `cover`:
// data-testid="bb-cover-gradient" com style.backgroundImage = linear-gradient(to bottom, transparent, var(--arc-bb-scrim), var(--arc-bb-bg)).
describe('BackgroundStage: fundo por tema (BBX-052)', () => {
  it('BBX-052/contain desenha a arte inteira sobre o preenchimento desfocado, sem o degradê do cover', () => {
    const { container } = render(<BackgroundStage {...base} backdrop="contain" videoUrl={null} />)
    const fill = container.querySelector('[data-testid="bb-art-fill"]') as HTMLElement
    const art = container.querySelector('[data-testid="bb-art"]') as HTMLElement
    expect(fill.className).toContain('object-cover')
    expect(fill.className).toMatch(/blur-\[(\d+)px\]/)
    expect(art.className).toContain('object-contain')
    expect(container.querySelector('[data-testid="bb-cover-gradient"]')).toBeNull()
  })

  it('BBX-052/cover usa object-fit cover, sem desfoque e sem preenchimento, com degradê vertical transparente → scrim → bg', () => {
    const { container } = render(<BackgroundStage {...base} backdrop="cover" videoUrl={null} />)
    const art = container.querySelector('[data-testid="bb-art"]') as HTMLElement
    expect(art.getAttribute('src')).toBe('http://media/1/bg.jpg')
    expect(art.className).toContain('object-cover')
    expect(art.className).not.toContain('object-contain')
    expect(art.className).not.toMatch(/blur/)
    expect(art.style.filter).not.toMatch(/blur/)
    expect(container.querySelector('[data-testid="bb-art-fill"]')).toBeNull()
    const g = container.querySelector('[data-testid="bb-cover-gradient"]') as HTMLElement
    expect(g).not.toBeNull()
    const css = `${g.style.backgroundImage} ${g.getAttribute('style') ?? ''}`
    expect(css).toContain('linear-gradient')
    expect(css).toMatch(/to bottom|180deg/)
    const iT = css.search(/transparent|#0000\b|rgba\(\s*0\s*,\s*0\s*,\s*0\s*,\s*0\s*\)/)
    const iS = css.indexOf('--arc-bb-scrim')
    const iB = css.indexOf('--arc-bb-bg')
    expect(iT, 'transparente no topo').toBeGreaterThanOrEqual(0)
    expect(iS, 'scrim no meio').toBeGreaterThan(iT)
    expect(iB, 'bg opaco embaixo').toBeGreaterThan(iS)
    // the gradient is drawn above the art
    const all = Array.from(container.querySelectorAll('*'))
    expect(all.indexOf(g)).toBeGreaterThan(all.indexOf(art))
  })

  it('BBX-052/plain desenha só --arc-bb-bg: sem arte, sem preenchimento, sem gradiente e sem vídeo mesmo com videoUrl', () => {
    const onVideoVisible = vi.fn()
    const { container } = render(<BackgroundStage {...base} backdrop="plain" onVideoVisible={onVideoVisible} />)
    act(() => { vi.advanceTimersByTime(VIDEO_DELAY_MS * 3) })
    expect(container.querySelector('img')).toBeNull()
    expect(container.querySelector('[data-testid="bb-art-layer"]')).toBeNull()
    expect(container.querySelector('[data-testid="bb-art-fill"]')).toBeNull()
    expect(container.querySelector('[data-testid="bb-gradient"]')).toBeNull()
    expect(container.querySelector('[data-testid="bb-cover-gradient"]')).toBeNull()
    expect(videos(container)).toHaveLength(0)
    expect(onVideoVisible).not.toHaveBeenCalledWith(true)
    expect((container.firstElementChild as HTMLElement).className).toContain('bg-bb-bg')
  })

  it('BBX-052/a transição de 300 ms vale para contain e para cover (duas camadas na troca, uma depois de ART_FADE_MS)', () => {
    for (const backdrop of ['contain', 'cover'] as const) {
      const { container, rerender, unmount } = render(<BackgroundStage {...base} backdrop={backdrop} videoUrl={null} />)
      rerender(<BackgroundStage {...base} backdrop={backdrop} gameId={2} artUrl="http://media/2/bg.jpg" videoUrl={null} />)
      const layers = Array.from(container.querySelectorAll('[data-testid="bb-art-layer"]')) as HTMLElement[]
      expect(layers, backdrop).toHaveLength(2)
      for (const l of layers) {
        expect(l.style.transition, backdrop).toContain('opacity')
        expect(l.style.transition, backdrop).toContain('300ms')
      }
      act(() => { vi.advanceTimersByTime(ART_FADE_MS) })
      expect(container.querySelectorAll('[data-testid="bb-art-layer"]'), backdrop).toHaveLength(1)
      unmount()
    }
  })

  it('BBX-052/trocar o backdrop troca o desenho sem deixar o do anterior (contain → plain remove a arte)', () => {
    const { container, rerender } = render(<BackgroundStage {...base} backdrop="contain" videoUrl={null} />)
    expect(container.querySelector('[data-testid="bb-art"]')).not.toBeNull()
    rerender(<BackgroundStage {...base} backdrop="plain" videoUrl={null} />)
    expect(container.querySelector('img')).toBeNull()
  })

  it('BBX-053/com backdrop cover e videoUrl (vídeo no fundo) o vídeo toca depois de 2.000 ms como em BBX-011', () => {
    const { container } = render(<BackgroundStage {...base} backdrop="cover" />)
    expect(videos(container)).toHaveLength(0)
    act(() => { vi.advanceTimersByTime(VIDEO_DELAY_MS) })
    expect(videos(container)).toHaveLength(1)
  })

  it('BBX-053/com videoUrl null (tema com videoSlot cover) o fundo nunca toca vídeo', () => {
    for (const backdrop of ['contain', 'cover', 'plain'] as const) {
      const { container, unmount } = render(<BackgroundStage {...base} backdrop={backdrop} videoUrl={null} />)
      act(() => { vi.advanceTimersByTime(VIDEO_DELAY_MS * 3) })
      expect(videos(container), backdrop).toHaveLength(0)
      unmount()
    }
  })
})
