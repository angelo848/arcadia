// Assumed markup (T15 must follow) for DesktopGrid and DesktopList [TDK-005, TDK-006, TDK-019, TDK-023, TDK-025, TDK-029]:
//  both render a fragment/wrapper of role="option" items (the listbox belongs to the parent; the tests supply it).
//  Card: aria-label = title, aria-selected on game index `focus`; cover area aspect 3/4 with <img> (object-contain, or object-cover when
//  coverFit = 'cover'; the placeholder image coverPlaceholderUrl when coverUrl is null); title element with title="<full title>"
//  and line-clamp-2; "Jogado <lastPlayedText>" only when the game was played (lastPlayedText not "Nunca"/empty); session badge text.
//  Row (list): fixed 64 px height (inline style height or h-16), text title, platform, playTimeText, lastPlayedText ("Nunca" when
//  never) and the badge; no buttons. Focus: data-focused="true" + outline 2px var(--arc-bb-focus) only with zoneFocused; otherwise
//  the selected item has bb-surface fill and a text-muted ring, no data-focused.
import { fireEvent, render, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { coverPlaceholderUrl, LIST_ROW_HEIGHT } from '../../desktopLayout'
import type { BigBoxGame } from '../../viewModel'
import { DesktopGrid } from './DesktopGrid'
import { DesktopList } from './DesktopList'

const game = (id: number, o: Partial<BigBoxGame> = {}): BigBoxGame => ({
  id, title: `Jogo ${id}`, coverUrl: `http://m/${id}.jpg`, session: null, coverFit: 'contain', platform: 'Super Nintendo',
  playTimeText: '2 h 10 min', lastPlayedText: '12/10/2026 10:05', ...o,
})
const look = (el: Element) => `${el.className} ${el.getAttribute('style') ?? ''}`
type Items = typeof DesktopGrid
function setup(Comp: Items, games: BigBoxGame[], o: Partial<{ start: number; focus: number; zoneFocused: boolean; columns: number }> = {}) {
  const cb = { onPick: vi.fn(), onPlay: vi.fn() }
  const props = { games, start: o.start ?? 0, focus: o.focus ?? 0, columns: o.columns ?? 6, zoneFocused: o.zoneFocused ?? true, ...cb }
  const r = render(
    <div role="listbox" aria-label="Jogos">
      <Comp {...props} />
    </div>,
  )
  return { ...cb, ...r }
}
const games = (n: number, from = 1) => Array.from({ length: n }, (_, i) => game(from + i))

describe('DesktopGrid', () => {
  it('TDK-005/cartão com capa 3:4 inteira, título em duas linhas com title completo, "Jogado DD/MM/AAAA HH:MM" e selo de sessão', () => {
    const long = 'Um título bem comprido que não cabe em duas linhas do cartão de jogo'
    setup(DesktopGrid, [game(1, { title: long, session: 'running' }), game(2, { session: 'paused' }), game(3)])
    const [a, b, c] = screen.getAllByRole('option')
    const cover = a.querySelector('img')!
    expect(cover.getAttribute('src')).toBe('http://m/1.jpg')
    expect(cover.className).toContain('object-contain')
    expect(look(cover.parentElement!) + look(cover.parentElement!.parentElement!)).toMatch(/aspect-\[3\/4\]|aspect-ratio:\s*3\s*\/\s*4/)
    const title = a.querySelector(`[title="${long}"]`)!
    expect(title).toHaveTextContent(long)
    expect(look(title)).toMatch(/line-clamp-2|line-clamp:\s*2/)
    expect(within(a).getByText('Jogado 12/10/2026 10:05')).toBeInTheDocument()
    expect(within(a).getByText('Em execução')).toBeInTheDocument()
    expect(within(b).getByText('Pausado')).toBeInTheDocument()
    expect(within(c).queryByText(/Em execução|Pausado/)).toBeNull()
  })

  it('TDK-005/nunca jogado não mostra "Jogado"; capa em modo cover usa object-cover; placeholder sem imagem', () => {
    setup(DesktopGrid, [game(1, { lastPlayedText: 'Nunca' }), game(2, { coverFit: 'cover' }), game(3, { coverUrl: null })])
    const [a, b, c] = screen.getAllByRole('option')
    expect(a.textContent).not.toMatch(/Jogado/)
    expect(b.querySelector('img')!.className).toContain('object-cover')
    expect(c.querySelector('img')?.getAttribute('src')).toBe(coverPlaceholderUrl)
  })

  it('TDK-025/clique foca (onPick com o índice absoluto stripStart + i) e duplo clique lança (onPlay)', () => {
    const { onPick, onPlay } = setup(DesktopGrid, games(8, 13), { start: 12, focus: 12 })
    const options = screen.getAllByRole('option')
    fireEvent.click(options[3])
    expect(onPick).toHaveBeenCalledWith(15)
    fireEvent.doubleClick(options[3])
    expect(onPlay).toHaveBeenCalledWith(15)
  })

  it('TDK-019/desenha só os cartões da janela (um nó por jogo recebido), sem barra de rolagem', () => {
    const { container } = setup(DesktopGrid, games(18))
    expect(screen.getAllByRole('option')).toHaveLength(18)
    expect(container.innerHTML).not.toMatch(/overflow(-y)?:\s*(auto|scroll)|overflow-y-(auto|scroll)|overflow-(auto|scroll)/)
  })

  it('TDK-029/aria-selected só no jogo em foco, com o aria-label do título', () => {
    setup(DesktopGrid, games(8, 13), { start: 12, focus: 14 })
    const options = screen.getAllByRole('option')
    expect(options.map((o) => o.getAttribute('aria-selected'))).toEqual(['false', 'false', 'true', 'false', 'false', 'false', 'false', 'false'])
    expect(options[2]).toHaveAttribute('aria-label', 'Jogo 15')
  })

  it('TDK-023/zona games: contorno de foco (outline 2px --arc-bb-focus) e data-focused só no jogo em foco', () => {
    const { container } = setup(DesktopGrid, games(6), { focus: 2, zoneFocused: true })
    const marks = container.querySelectorAll('[data-focused="true"]')
    expect(marks).toHaveLength(1)
    expect(marks[0]).toBe(screen.getAllByRole('option')[2])
    expect(look(marks[0])).toMatch(/outline[^;]*2px[^;]*(--arc-bb-focus|bb-focus)|outline-2/)
  })

  it('TDK-023/fora da zona games o selecionado mantém o destaque (bb-surface) sem data-focused nem contorno', () => {
    const { container } = setup(DesktopGrid, games(6), { focus: 2, zoneFocused: false })
    expect(container.querySelector('[data-focused="true"]')).toBeNull()
    const sel = screen.getAllByRole('option')[2]
    expect(sel).toHaveAttribute('aria-selected', 'true')
    expect(look(sel)).toMatch(/bb-surface/)
    expect(look(sel)).not.toMatch(/outline:\s*2px solid/)
  })
})

describe('DesktopList', () => {
  it('TDK-006/linha de 64 px com título, plataforma, tempo, última vez ("Nunca") e marcação, sem botões', () => {
    const { container } = setup(DesktopList, [game(1, { session: 'running' }), game(2, { lastPlayedText: 'Nunca', playTimeText: '0 min' })], { columns: 1 })
    const [a, b] = screen.getAllByRole('option')
    expect(LIST_ROW_HEIGHT).toBe(64)
    expect(look(a)).toMatch(/height:\s*64px|\bh-16\b|\bh-\[64px\]/)
    for (const t of ['Jogo 1', 'Super Nintendo', '2 h 10 min', '12/10/2026 10:05', 'Em execução']) {
      expect(within(a).getByText(t), t).toBeInTheDocument()
    }
    expect(within(b).getByText('Nunca')).toBeInTheDocument()
    expect(container.querySelector('button, [role="button"], a[href], input')).toBeNull()
  })

  it('TDK-006/TDK-023/TDK-029/destaque do jogo em foco: contorno só com a zona games; aria-selected e aria-label', () => {
    const { container, rerender } = setup(DesktopList, games(5), { columns: 1, focus: 1, zoneFocused: true })
    expect(container.querySelectorAll('[data-focused="true"]')).toHaveLength(1)
    const opts = screen.getAllByRole('option')
    expect(opts[1]).toHaveAttribute('data-focused', 'true')
    expect(opts[1]).toHaveAttribute('aria-selected', 'true')
    expect(opts[1]).toHaveAttribute('aria-label', 'Jogo 2')
    rerender(
      <div role="listbox" aria-label="Jogos">
        <DesktopList games={games(5)} start={0} focus={1} columns={1} zoneFocused={false} onPick={vi.fn()} onPlay={vi.fn()} />
      </div>,
    )
    expect(container.querySelector('[data-focused="true"]')).toBeNull()
    expect(look(screen.getAllByRole('option')[1])).toMatch(/bb-surface/)
  })

  it('TDK-025/clique foca (índice absoluto) e duplo clique lança', () => {
    const { onPick, onPlay } = setup(DesktopList, games(5, 21), { columns: 1, start: 20, focus: 20 })
    fireEvent.click(screen.getAllByRole('option')[2])
    expect(onPick).toHaveBeenCalledWith(22)
    fireEvent.doubleClick(screen.getAllByRole('option')[2])
    expect(onPlay).toHaveBeenCalledWith(22)
  })
})
