import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { TagInput } from './TagInput'

const catalog = ['Ação', 'RPG', 'rpg tático', 'Plataforma']

function renderTags(value: string[] = [], onChange = vi.fn()) {
  const utils = render(
    <>
      <label htmlFor="genres">Gêneros</label>
      <TagInput id="genres" value={value} suggestions={catalog} onChange={onChange} />
    </>,
  )
  return { ...utils, onChange, input: screen.getByLabelText('Gêneros') as HTMLInputElement }
}

describe('TagInput', () => {
  it('TAG-001/sugere gêneros do catálogo sem diferenciar maiúsculas', () => {
    const { input } = renderTags()

    fireEvent.change(input, { target: { value: 'rpg' } })

    expect(screen.getByRole('option', { name: 'RPG' })).toBeInTheDocument()
    expect(screen.getByRole('option', { name: 'rpg tático' })).toBeInTheDocument()
    expect(screen.queryByRole('option', { name: 'Ação' })).not.toBeInTheDocument()
  })

  it('TAG-001/Enter, vírgula ou sugestão adiciona a tag', () => {
    const { input, onChange } = renderTags()

    fireEvent.change(input, { target: { value: 'Plataforma' } })
    fireEvent.keyDown(input, { key: 'Enter' })
    expect(onChange).toHaveBeenLastCalledWith(['Plataforma'])

    fireEvent.change(input, { target: { value: 'Ação' } })
    fireEvent.keyDown(input, { key: ',' })
    expect(onChange).toHaveBeenLastCalledWith(['Ação'])

    fireEvent.change(input, { target: { value: 'rpg' } })
    fireEvent.click(screen.getByRole('option', { name: 'RPG' }))
    expect(onChange).toHaveBeenLastCalledWith(['RPG'])
  })

  it('TAG-001/nome digitado em outra caixa usa a grafia do catálogo', () => {
    const { input, onChange } = renderTags()

    fireEvent.change(input, { target: { value: 'AÇÃO' } })
    fireEvent.keyDown(input, { key: 'Enter' })

    expect(onChange).toHaveBeenLastCalledWith(['Ação'])
  })

  it('TAG-002/nome que não existe no catálogo é marcado como nova', () => {
    const { input, onChange, rerender } = renderTags()

    fireEvent.change(input, { target: { value: 'Terror' } })
    fireEvent.keyDown(input, { key: 'Enter' })
    expect(onChange).toHaveBeenLastCalledWith(['Terror'])

    rerender(
      <>
        <label htmlFor="genres">Gêneros</label>
        <TagInput id="genres" value={['Terror']} suggestions={catalog} onChange={onChange} />
      </>,
    )
    expect(screen.getByText('nova')).toBeInTheDocument()
  })

  it('TAG-003/tag já presente sem diferença de maiúsculas não é adicionada de novo', () => {
    const { input, onChange } = renderTags(['Ação'])

    fireEvent.change(input, { target: { value: 'AÇÃO' } })
    fireEvent.keyDown(input, { key: 'Enter' })

    expect(onChange).not.toHaveBeenCalled()
  })

  it('TAG-003/texto vazio mostra erro e não cria tag', () => {
    const { input, onChange } = renderTags()

    fireEvent.change(input, { target: { value: '   ' } })
    fireEvent.keyDown(input, { key: 'Enter' })

    expect(onChange).not.toHaveBeenCalled()
    expect(screen.getByText('Digite um gênero.')).toBeInTheDocument()
  })

  it('TAG-003/texto com mais de 100 caracteres mostra erro e não cria tag', () => {
    const { input, onChange } = renderTags()

    fireEvent.change(input, { target: { value: 'a'.repeat(101) } })
    fireEvent.keyDown(input, { key: 'Enter' })

    expect(onChange).not.toHaveBeenCalled()
    expect(screen.getByText('Use no máximo 100 caracteres.')).toBeInTheDocument()
  })

  it('TAG-004/× remove a tag e envia o conjunto sem ela', () => {
    const { onChange } = renderTags(['Ação', 'RPG'])

    fireEvent.click(screen.getByRole('button', { name: 'Remover Ação' }))

    expect(onChange).toHaveBeenLastCalledWith(['RPG'])
  })

  it('TAG-004/Backspace com campo vazio remove a última tag', () => {
    const { input, onChange } = renderTags(['Ação', 'RPG'])

    fireEvent.keyDown(input, { key: 'Backspace' })

    expect(onChange).toHaveBeenLastCalledWith(['Ação'])
  })
})
