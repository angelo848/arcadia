import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { MetadataOptions } from './MetadataOptions'
import type { MetadataOptionsProps } from './MetadataOptions'
import { coverPlaceholderUrl } from './CoverImage'
import type { MetadataCandidateView } from '../lib/metadata'

const igdbSteam: MetadataCandidateView = {
  refs: [
    { provider: 'igdb', providerId: '1942' },
    { provider: 'steamgriddb', providerId: '77' },
  ],
  title: 'Celeste',
  year: 2018,
  platform: 'PC',
  score: 0.93,
  thumbUrl: 'https://images.igdb.com/igdb/image/upload/t_thumb/abc.jpg',
}

const second: MetadataCandidateView = {
  refs: [{ provider: 'igdb', providerId: '5' }],
  title: 'Celeste Remake',
  year: 2021,
  platform: 'Switch',
  score: 0.61,
}

function renderOptions(over: Partial<MetadataOptionsProps> = {}) {
  const props: MetadataOptionsProps = {
    candidates: [igdbSteam, second],
    confidence: 'match',
    warnings: [],
    selected: 0,
    onSelect: vi.fn(),
    ...over,
  }
  return { ...render(<MetadataOptions {...props} />), props }
}

describe('MetadataOptions', () => {
  it('MET-044/item mostra capa, título, ano entre parênteses, plataforma, provedores e percentual', () => {
    const { container } = renderOptions({ candidates: [igdbSteam] })

    expect(screen.getByText('Celeste')).toBeInTheDocument()
    expect(screen.getByText('(2018)')).toBeInTheDocument()
    expect(screen.getByText('PC')).toBeInTheDocument()
    expect(screen.getByText('IGDB · SteamGridDB')).toBeInTheDocument()
    expect(screen.getByText('93%')).toBeInTheDocument()
    expect(container.querySelector('img')).not.toBeNull()
  })

  it('MET-044/ano 0 omite os parênteses', () => {
    renderOptions({ candidates: [{ ...igdbSteam, year: 0 }] })

    expect(screen.getByText('Celeste')).toBeInTheDocument()
    expect(screen.queryByText(/\(0\)/)).toBeNull()
    expect(screen.queryByText('()')).toBeNull()
    expect(screen.queryByText(/\(\s*\)/)).toBeNull()
  })

  it('MET-044/plataforma vazia mostra "Plataforma desconhecida"', () => {
    renderOptions({ candidates: [{ ...igdbSteam, platform: '' }] })

    expect(screen.getByText('Plataforma desconhecida')).toBeInTheDocument()
  })

  it('MET-044/selo provável só no primeiro item e só com match', () => {
    const { unmount } = renderOptions({ confidence: 'match' })
    expect(screen.getAllByText('provável')).toHaveLength(1)
    unmount()

    renderOptions({ confidence: 'doubt' })
    expect(screen.queryByText('provável')).toBeNull()
  })

  it('MET-044/lista vazia mostra "Nenhum resultado para este nome."', () => {
    renderOptions({ candidates: [], confidence: 'none', selected: null })

    expect(screen.getByText('Nenhum resultado para este nome.')).toBeInTheDocument()
  })

  it('MET-044/avisos de provedor acima da lista', () => {
    renderOptions({ warnings: ['SteamGridDB fora do ar'] })

    const warning = screen.getByText('SteamGridDB fora do ar')
    const group = screen.getByRole('radiogroup')
    // DOCUMENT_POSITION_FOLLOWING (4): o grupo vem depois do aviso.
    expect(warning.compareDocumentPosition(group) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  })

  it('MET-044/renderAction aparece por item', () => {
    const renderAction = vi.fn((_c: MetadataCandidateView, i: number) => <button type="button">Aplicar {i}</button>)
    renderOptions({ renderAction })

    expect(screen.getByRole('button', { name: 'Aplicar 0' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Aplicar 1' })).toBeInTheDocument()
    expect(renderAction).toHaveBeenCalledTimes(2)
  })

  it('MET-044/allowNone adiciona "Nenhuma"', () => {
    const onSelect = vi.fn()
    renderOptions({ allowNone: true, selected: null, onSelect })

    const none = screen.getByRole('radio', { name: 'Nenhuma' })
    fireEvent.click(none)
    expect(onSelect).toHaveBeenLastCalledWith(null)
  })

  it('MET-044/sem allowNone não há "Nenhuma"', () => {
    renderOptions()

    expect(screen.queryByRole('radio', { name: 'Nenhuma' })).toBeNull()
  })

  it('MET-068/img com alt vazio, loading lazy e referrerPolicy no-referrer', () => {
    const { container } = renderOptions({ candidates: [igdbSteam] })

    const img = container.querySelector('img')
    expect(img).not.toBeNull()
    expect(img).toHaveAttribute('alt', '')
    expect(img).toHaveAttribute('loading', 'lazy')
    expect(img).toHaveAttribute('referrerPolicy', 'no-referrer')
    expect(img).toHaveAttribute('src', igdbSteam.thumbUrl)
  })

  it('MET-068/erro da imagem: placeholder', () => {
    const { container } = renderOptions({ candidates: [igdbSteam] })

    const img = container.querySelector('img') as HTMLImageElement
    fireEvent.error(img)

    expect(container.querySelector('img')).toHaveAttribute('src', coverPlaceholderUrl)
  })

  it('MET-068/thumbUrl ausente: placeholder', () => {
    const { container } = renderOptions({ candidates: [second] })

    expect(container.querySelector('img')).toHaveAttribute('src', coverPlaceholderUrl)
  })

  it('MET-069/grupo de rádios com nomes "<título> (<ano>), <provedores>"', () => {
    renderOptions()

    expect(screen.getByRole('radiogroup')).toBeInTheDocument()
    expect(screen.getByRole('radio', { name: 'Celeste (2018), IGDB · SteamGridDB' })).toBeInTheDocument()
    expect(screen.getByRole('radio', { name: 'Celeste Remake (2021), IGDB' })).toBeInTheDocument()
  })

  it('MET-069/itens são rádios de um mesmo grupo (setas e Espaço são do navegador; sem Tab por item)', () => {
    const { container } = renderOptions()

    const radios = screen.getAllByRole('radio') as HTMLInputElement[]
    expect(radios).toHaveLength(2)
    const names = new Set(radios.map((r) => r.getAttribute('name')))
    expect(names.size).toBe(1)
    expect(names.has(null)).toBe(false)
    expect(radios.every((r) => r.type === 'radio')).toBe(true)
    expect(container.querySelectorAll('input[type="radio"]')).toHaveLength(2)
  })

  it('MET-069/Espaço (clique no rádio) escolhe o item e reflete o selecionado', () => {
    const onSelect = vi.fn()
    renderOptions({ selected: 0, onSelect })

    const second_ = screen.getByRole('radio', { name: 'Celeste Remake (2021), IGDB' }) as HTMLInputElement
    expect(second_.checked).toBe(false)
    fireEvent.click(second_)

    expect(onSelect).toHaveBeenLastCalledWith(1)
  })
})
