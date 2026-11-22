import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  chooseTimeToBeat,
  refreshTimeToBeat,
  searchTimeToBeat,
  setTimeToBeatSettings,
} from './timeToBeatEdit'

// Bindings (window.go.main.App): TimeToBeatRefresh(id), TimeToBeatSearch(id, term),
// TimeToBeatChoose(id, externalId), TimeToBeatSetSettings(enabled, afterMetadata), TimeToBeatSettings().

type Fn = ReturnType<typeof vi.fn>
let app: Record<string, Fn>

beforeEach(() => {
  app = {
    TimeToBeatRefresh: vi.fn(() => Promise.resolve({ gameId: 3, state: 'found' })),
    TimeToBeatSearch: vi.fn(() => Promise.resolve([{ externalId: 'a', title: 'A' }])),
    TimeToBeatChoose: vi.fn(() => Promise.resolve({ gameId: 3, state: 'found', confirmed: true })),
    TimeToBeatSetSettings: vi.fn(() => Promise.resolve({ enabled: false, afterMetadata: true })),
    TimeToBeatSettings: vi.fn(() => Promise.resolve({ enabled: true, afterMetadata: true })),
  }
  ;(window as unknown as { go?: unknown }).go = { main: { App: app } }
})

afterEach(() => {
  delete (window as unknown as { go?: unknown }).go
})

describe('timeToBeatEdit', () => {
  it('TTB-018/refreshTimeToBeat chama TimeToBeatRefresh(id) e devolve a view', async () => {
    const v = await refreshTimeToBeat(3)
    expect(app.TimeToBeatRefresh).toHaveBeenCalledWith(3)
    expect(v).toMatchObject({ gameId: 3, state: 'found' })
  })

  it('TTB-018/searchTimeToBeat chama TimeToBeatSearch(id, term) e devolve os candidatos', async () => {
    const c = await searchTimeToBeat(3, 'zelda')
    expect(app.TimeToBeatSearch).toHaveBeenCalledWith(3, 'zelda')
    expect(c).toEqual([{ externalId: 'a', title: 'A' }])
  })

  it('TTB-018/chooseTimeToBeat chama TimeToBeatChoose(id, externalId); "" = nenhum destes', async () => {
    const v = await chooseTimeToBeat(3, 'abc')
    expect(app.TimeToBeatChoose).toHaveBeenCalledWith(3, 'abc')
    expect(v).toMatchObject({ confirmed: true })
    await chooseTimeToBeat(3, '')
    expect(app.TimeToBeatChoose).toHaveBeenLastCalledWith(3, '')
  })

  it('TTB-018/setTimeToBeatSettings chama TimeToBeatSetSettings(enabled, afterMetadata)', async () => {
    const s = await setTimeToBeatSettings(false, true)
    expect(app.TimeToBeatSetSettings).toHaveBeenCalledWith(false, true)
    expect(s).toEqual({ enabled: false, afterMetadata: true })
  })

  it('TTB-018/erro do binding propaga (a janela de edição mostra o estado)', async () => {
    app.TimeToBeatRefresh.mockImplementation(() => Promise.reject(new Error('unavailable')))
    await expect(refreshTimeToBeat(3)).rejects.toThrow('unavailable')
  })
})
