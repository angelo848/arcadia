import { describe, expect, it, vi } from 'vitest'
import { playWithSound } from './playMedia'

function fakeVideo(playImpl: () => Promise<void> | undefined) {
  return { muted: true, play: vi.fn(playImpl) } as unknown as HTMLVideoElement & { play: ReturnType<typeof vi.fn> }
}

describe('playWithSound', () => {
  it('BBX-013/com som liberado toca sem mutar', async () => {
    const v = fakeVideo(() => Promise.resolve())
    playWithSound(v, true)
    await Promise.resolve()
    expect(v.muted).toBe(false)
    expect(v.play).toHaveBeenCalledTimes(1)
  })

  it('BBX-013/sound=false toca mudo', () => {
    const v = fakeVideo(() => Promise.resolve())
    playWithSound(v, false)
    expect(v.muted).toBe(true)
  })

  it('BBX-013/recusa do navegador: toca mudo e ganha o som na próxima tecla', async () => {
    let blocked = true
    const v = fakeVideo(() => (blocked ? Promise.reject(new Error('NotAllowedError')) : Promise.resolve()))
    const stop = playWithSound(v, true)
    await new Promise((r) => setTimeout(r, 0))
    expect(v.muted).toBe(true)
    blocked = false
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'a' }))
    await Promise.resolve()
    expect(v.muted).toBe(false)
    stop()
  })
})
