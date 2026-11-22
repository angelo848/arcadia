import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { TimeToBeatList } from './TimeToBeatList'

// Assumed (contrato temas-v2 §1): TimeToBeatList({ rows, source }) draws the three label/value pairs and, when `source` is set,
// the line "Fonte: <source>" (TTB-039). It never draws buttons nor role="alert" (TTB-013, TTB-016).
const rows = [
  { label: 'História principal' as const, value: '20 h' },
  { label: 'História + extras' as const, value: '31½ h' },
  { label: '100%' as const, value: '41 h' },
]
const empty = rows.map((r) => ({ ...r, value: '—' }))

describe('TimeToBeatList', () => {
  it('TTB-016/mostra os três rótulos e os três valores, na ordem', () => {
    const { container } = render(<TimeToBeatList rows={rows} source="HowLongToBeat" />)
    const text = container.textContent ?? ''
    for (const s of ['História principal', '20 h', 'História + extras', '31½ h', '100%', '41 h']) {
      expect(text).toContain(s)
    }
    const order = ['História principal', 'História + extras', '100%'].map((l) => text.indexOf(l))
    expect(order).toEqual([...order].sort((a, b) => a - b))
    expect(text.indexOf('20 h')).toBeLessThan(text.indexOf('História + extras'))
  })

  it('TTB-039/fonte HowLongToBeat: a linha "Fonte: HowLongToBeat"', () => {
    render(<TimeToBeatList rows={rows} source="HowLongToBeat" />)
    expect(screen.getByText('Fonte: HowLongToBeat')).toBeInTheDocument()
  })

  it('TTB-039/fonte IGDB: a linha "Fonte: IGDB"', () => {
    render(<TimeToBeatList rows={rows} source="IGDB" />)
    expect(screen.getByText('Fonte: IGDB')).toBeInTheDocument()
    expect(screen.queryByText(/HowLongToBeat/)).toBeNull()
  })

  it('TTB-039/sem fonte (sem valor) a linha da fonte não aparece', () => {
    const { container } = render(<TimeToBeatList rows={empty} source={null} />)
    expect(container.textContent).not.toContain('Fonte')
  })

  it('TTB-013/sem dado os três rótulos continuam com "—", sem alerta, sem botão e sem mensagem de erro', () => {
    const { container } = render(<TimeToBeatList rows={empty} source={null} />)
    const text = container.textContent ?? ''
    for (const l of ['História principal', 'História + extras', '100%']) expect(text).toContain(l)
    expect(text.match(/—/g)).toHaveLength(3)
    expect(screen.queryByRole('alert')).toBeNull()
    expect(screen.queryByRole('button')).toBeNull()
    expect(text).not.toMatch(/erro|falha|indispon/i)
  })

  it('TTB-016/não há botão de atualizar nem de confirmar', () => {
    render(<TimeToBeatList rows={rows} source="HowLongToBeat" />)
    expect(screen.queryByRole('button', { name: /atualizar|confirmar|escolher/i })).toBeNull()
    expect(screen.queryByRole('button')).toBeNull()
  })

  it('TTB-016/mostra o que recebe: valores mistos (um dado, dois "—") sem mexer nos rótulos', () => {
    const mixed = [rows[0], { ...rows[1], value: '—' }, { ...rows[2], value: '—' }]
    const { container } = render(<TimeToBeatList rows={mixed} source="IGDB" />)
    expect(container.textContent).toContain('20 h')
    expect(container.textContent?.match(/—/g)).toHaveLength(2)
  })
})
