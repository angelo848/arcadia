import { useEffect } from 'react'
import { useQueries, useQuery, useQueryClient } from '@tanstack/react-query'
import type { GameDetailView } from './library'

// Providers of the metadata search (phase 7, MID-002). Only IGDB and SteamGridDB need a key (MID-001).
export type ProviderId = 'igdb' | 'steamgriddb' | 'steamstore'
export type KeyProviderId = 'igdb' | 'steamgriddb'

// The labels the UI shows for each provider.
export const providerLabels: Record<ProviderId, string> = {
  igdb: 'IGDB',
  steamgriddb: 'SteamGridDB',
  steamstore: 'Steam Store',
}

// The kinds of image a provider can offer (MID-004).
export type MediaKind = 'cover' | 'screenshot' | 'title' | 'banner' | 'background' | 'select'

// ProviderKeyView says whether a key is registered; the key itself is never shown (MID-001).
export type ProviderKeyView = { provider: KeyProviderId; configured: boolean }

// ScrapeCandidateView is a game a provider found, with its match score (MID-002).
export type ScrapeCandidateView = {
  providerId: string
  title: string
  platform: string
  year: number
  score: number
  accepted: boolean
}

// ScrapeApplyView is the game after the metadata was applied and the number of images queued.
export type ScrapeApplyView = { game: GameDetailView; queued: number }

// MediaItemView is one image of a game; url and thumbUrl are what the page reads (MID-006).
export type MediaItemView = {
  id: number
  kind: MediaKind
  provider: string
  path: string
  thumbPath: string
  url: string
  thumbUrl: string
}

export const mediaKindLabels: Record<MediaKind, string> = {
  cover: 'Capa',
  screenshot: 'Screenshot',
  title: 'Tela de título',
  banner: 'Banner',
  background: 'Fundo',
  select: 'Tela de seleção',
}

// The order the details carousel shows the kinds in [AJU-009].
export const carouselKindOrder: MediaKind[] = ['cover', 'screenshot', 'title', 'select', 'background', 'banner']

// orderMedia sorts by carouselKindOrder, stable within a kind; it does not mutate.
export function orderMedia(items: MediaItemView[]): MediaItemView[] {
  const rank = (k: MediaKind) => {
    const i = carouselKindOrder.indexOf(k)
    return i < 0 ? carouselKindOrder.length : i
  }
  return items.map((m, i) => ({ m, i })).sort((a, b) => rank(a.m.kind) - rank(b.m.kind) || a.i - b.i).map((x) => x.m)
}

// The kinds a card falls back to when the game has no cover [AJU-015].
export const cardFallbackOrder: MediaKind[] = ['title', 'select', 'screenshot', 'banner', 'background']

export type CardImage = { url: string; fit: 'contain' | 'cover' }

// cardImageOf picks the card image: the first cover (contain), else the first fallback kind (cover).
export function cardImageOf(items: MediaItemView[]): CardImage | undefined {
  const cover = items.find((m) => m.kind === 'cover')
  if (cover) {
    return { url: cover.thumbUrl || cover.url, fit: 'contain' }
  }
  for (const kind of cardFallbackOrder) {
    const m = items.find((x) => x.kind === kind)
    if (m) {
      return { url: m.thumbUrl || m.url, fit: 'cover' }
    }
  }
  return undefined
}

// The matching threshold of MID-002: a candidate at or above it is applied without asking.
export const acceptedScore = 0.8

// scoreLabel formats a match score as a percentage, for the candidate list.
export function scoreLabel(score: number): string {
  return `${Math.round(score * 100)}%`
}

type WailsApp = {
  ProviderKeyStatus?: (provider: string) => Promise<ProviderKeyView>
  ProviderSetKey?: (provider: string, key: string, secret: string) => Promise<ProviderKeyView>
  ProviderDeleteKey?: (provider: string) => Promise<ProviderKeyView>
  ScrapeSearch?: (gameId: number, provider: string) => Promise<{ candidates: ScrapeCandidateView[] }>
  ScrapeApply?: (gameId: number, provider: string, providerId: string, replace: boolean) => Promise<ScrapeApplyView>
  MediaList?: (gameId: number) => Promise<{ media: MediaItemView[] }>
  MediaQueue?: (gameId: number, provider: string, providerId: string, kinds: string[]) => Promise<number>
  MediaCancel?: (jobId: number) => Promise<void>
}

type WailsWindow = { go?: { main?: { App?: WailsApp } } }

function app(): WailsApp {
  return ((window as unknown as WailsWindow).go?.main?.App ?? {}) as WailsApp
}

async function call<T>(name: keyof WailsApp, args: unknown[]): Promise<T> {
  const fn = app()[name] as ((...a: unknown[]) => Promise<T>) | undefined
  if (!fn) {
    throw new Error(`${name} indisponível`)
  }
  return fn(...args)
}

export const mediaKey = (gameId: number) => ['media', gameId] as const

type WailsRuntime = {
  EventsOn?: (eventName: string, callback: (...data: unknown[]) => void) => (() => void) | void
}

const mediaChangedEvent = 'media:changed'

// useMediaEvents invalidates a game's media cache on each media:changed event [IMG-006/007].
export function useMediaEvents(): void {
  const qc = useQueryClient()

  useEffect(() => {
    const on = (window as unknown as { runtime?: WailsRuntime }).runtime?.EventsOn
    if (!on) {
      return undefined
    }
    const off = on(mediaChangedEvent, (...data: unknown[]) => {
      const p = data[0]
      if (typeof p !== 'object' || p === null) {
        return
      }
      const gameId = (p as { gameId?: unknown }).gameId
      if (typeof gameId !== 'number' || !Number.isInteger(gameId) || gameId < 1) {
        return
      }
      void qc.invalidateQueries({ queryKey: mediaKey(gameId), exact: true })
    })
    return () => {
      if (typeof off === 'function') {
        off()
      }
    }
  }, [qc])
}

// providerKeyStatus tells whether a key is registered (MID-001).
export function providerKeyStatus(provider: KeyProviderId): Promise<ProviderKeyView> {
  return call<ProviderKeyView>('ProviderKeyStatus', [provider])
}

// setProviderKey stores a key; IGDB also takes its secret (MID-001).
export function setProviderKey(provider: KeyProviderId, key: string, secret = ''): Promise<ProviderKeyView> {
  return call<ProviderKeyView>('ProviderSetKey', [provider, key, secret])
}

// deleteProviderKey removes a key (MID-001).
export function deleteProviderKey(provider: KeyProviderId): Promise<ProviderKeyView> {
  return call<ProviderKeyView>('ProviderDeleteKey', [provider])
}

// scrapeSearch asks a provider for the game; the candidates come best first (MID-002).
export async function scrapeSearch(gameId: number, provider: ProviderId): Promise<ScrapeCandidateView[]> {
  const res = await call<{ candidates: ScrapeCandidateView[] }>('ScrapeSearch', [gameId, provider])
  return res.candidates
}

// scrapeApply applies a candidate's metadata and queues its images (MID-003, MID-004).
export function scrapeApply(gameId: number, provider: ProviderId, providerId: string, replace: boolean): Promise<ScrapeApplyView> {
  return call<ScrapeApplyView>('ScrapeApply', [gameId, provider, providerId, replace])
}

// fetchMedia lists the images of a game (MID-006).
export async function fetchMedia(gameId: number): Promise<MediaItemView[]> {
  return (await call<{ media: MediaItemView[] }>('MediaList', [gameId])).media
}

// useMedia is the image list of a game, cached per game.
export function useMedia(gameId: number) {
  return useQuery({ queryKey: mediaKey(gameId), queryFn: () => fetchMedia(gameId), retry: false })
}

// useCardImages maps each game to its card image; games with errors or no image are omitted [AJU-015].
export function useCardImages(gameIds: number[]): Record<number, CardImage> {
  const results = useQueries({
    queries: gameIds.map((id) => ({ queryKey: mediaKey(id), queryFn: () => fetchMedia(id), retry: false, staleTime: 60_000 })),
  })
  const images: Record<number, CardImage> = {}
  results.forEach((r, i) => {
    const img = r.data ? cardImageOf(r.data) : undefined
    if (img) {
      images[gameIds[i]] = img
    }
  })
  return images
}

// mediaQueue queues the chosen kinds of a provider's images (MID-005).
export function mediaQueue(gameId: number, provider: ProviderId, providerId: string, kinds: MediaKind[]): Promise<number> {
  return call<number>('MediaQueue', [gameId, provider, providerId, kinds])
}

// mediaCancel stops a download (MID-005).
export function mediaCancel(jobId: number): Promise<void> {
  return call<void>('MediaCancel', [jobId])
}
