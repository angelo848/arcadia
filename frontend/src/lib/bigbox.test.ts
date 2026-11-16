import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

// Phase 13 T42: only the window helpers stay in lib/bigbox. The runtime is mocked so the full-screen calls can be seen.
const rt = vi.hoisted(() => ({
  fullscreen: false,
  WindowIsFullscreen: vi.fn(),
  WindowFullscreen: vi.fn(),
  WindowUnfullscreen: vi.fn(),
}))
vi.mock('../../wailsjs/runtime/runtime', () => ({
  WindowIsFullscreen: rt.WindowIsFullscreen,
  WindowFullscreen: rt.WindowFullscreen,
  WindowUnfullscreen: rt.WindowUnfullscreen,
}))

import { bringToFront, enterBigBox, exitBigBox } from './bigbox'

const w = window as unknown as { go?: unknown }

beforeEach(() => {
  rt.WindowIsFullscreen.mockReset().mockResolvedValue(false)
  rt.WindowFullscreen.mockReset()
  rt.WindowUnfullscreen.mockReset()
})
afterEach(() => {
  delete w.go
})

describe('lib/bigbox (alterado em 2026-11-15, fase 13)', () => {
  it('BBX-003/enterBigBox e exitBigBox continuam: só trocam a tela cheia e não fazem nada fora do desktop', async () => {
    // Windowed: enter goes to full screen, exit does nothing.
    await enterBigBox()
    expect(rt.WindowFullscreen).toHaveBeenCalledTimes(1)
    await exitBigBox()
    expect(rt.WindowUnfullscreen).not.toHaveBeenCalled()

    // Full screen: enter does not toggle back, exit leaves it.
    rt.WindowFullscreen.mockClear()
    rt.WindowIsFullscreen.mockResolvedValue(true)
    await enterBigBox()
    expect(rt.WindowFullscreen).not.toHaveBeenCalled()
    await exitBigBox()
    expect(rt.WindowUnfullscreen).toHaveBeenCalledTimes(1)

    // Outside the desktop shell the runtime throws: nothing happens and nothing fails.
    rt.WindowIsFullscreen.mockRejectedValue(new Error('no runtime'))
    await expect(enterBigBox()).resolves.toBeUndefined()
    await expect(exitBigBox()).resolves.toBeUndefined()
  })

  it('BBX-003/bringToFront chama o binding e ignora falha', async () => {
    const BringToFront = vi.fn(() => Promise.resolve())
    w.go = { main: { App: { BringToFront } } }
    await bringToFront()
    expect(BringToFront).toHaveBeenCalledTimes(1)

    BringToFront.mockRejectedValueOnce('falhou')
    await expect(bringToFront()).resolves.toBeUndefined()
    expect(BringToFront).toHaveBeenCalledTimes(2)

    delete w.go
    await expect(bringToFront()).resolves.toBeUndefined()
  })
})
