import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { renderHook, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { mediaKey, useCardImages } from './metadata'
import type { MediaItemView } from './metadata'

const item = (id: number, kind: MediaItemView['kind']): MediaItemView => ({
  id,
  kind,
  provider: 'p',
  path: '',
  thumbPath: '',
  url: `/m/${id}`,
  thumbUrl: `/t/${id}`,
})

afterEach(() => {
  delete (window as unknown as { go?: unknown }).go
})

describe('useCardImages', () => {
  it('AJU-015/useCardImages lê MediaList uma vez por jogo, compartilha mediaKey com o painel e omite jogos com erro ou sem imagem', async () => {
    const lists: Record<number, MediaItemView[] | Error> = {
      1: [item(10, 'cover')],
      2: [item(20, 'screenshot'), item(21, 'title')],
      3: [],
      4: new Error('boom'),
    }
    const MediaList = vi.fn(async (id: number) => {
      const v = lists[id]
      if (v instanceof Error) throw v
      return { media: v }
    })
    ;(window as unknown as { go: unknown }).go = { main: { App: { MediaList } } }
    const client = new QueryClient()
    const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={client}>{children}</QueryClientProvider>

    const { result, rerender } = renderHook(() => useCardImages([1, 2, 3, 4]), { wrapper })
    await waitFor(() => expect(MediaList).toHaveBeenCalledTimes(4))
    await waitFor(() => expect(Object.keys(result.current)).toHaveLength(2))
    expect(result.current).toEqual({ 1: { url: '/t/10', fit: 'contain' }, 2: { url: '/t/21', fit: 'cover' } })
    expect(client.getQueryData(mediaKey(1))).toEqual(lists[1])

    rerender()
    expect(MediaList).toHaveBeenCalledTimes(4)
  })
})
