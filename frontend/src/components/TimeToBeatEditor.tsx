import { useState, type JSX } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import {
  formatTimeToBeat,
  timeToBeatKey,
  timeToBeatRows,
  timeToBeatSource,
  useTimeToBeat,
  type TimeToBeatCandidate,
  type TimeToBeatView,
} from '../lib/timeToBeat'
import { chooseTimeToBeat, refreshTimeToBeat, searchTimeToBeat } from '../lib/timeToBeatEdit'
import { hasTimeToBeatValue, useTimeToBeatEnabled } from './TimeToBeatBlock'

const MAX_CANDIDATES = 10
const FAILURE = 'Não foi possível consultar o HowLongToBeat'

function localDate(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) {
    return ''
  }
  const p = (n: number) => String(n).padStart(2, '0')
  return `${p(d.getDate())}/${p(d.getMonth() + 1)}/${d.getFullYear()}`
}

function stateLine(v: TimeToBeatView | undefined): string {
  if (v === undefined) {
    return 'Ainda não buscado'
  }
  switch (v.state) {
    case 'found': {
      const head = v.source === 'igdb' ? 'Encontrado no IGDB (reserva)' : 'Encontrado'
      const date = v.fetchedAt !== '' ? localDate(v.fetchedAt) : ''
      return `${head}: ${v.externalTitle}${date !== '' ? ` · buscado em ${date}` : ''}${v.confirmed ? ' · confirmado' : ''}`
    }
    case 'doubt':
      return 'Correspondência incerta'
    case 'not-found':
      return 'Não encontrado'
    default:
      return v.unavailable ? 'HowLongToBeat indisponível no momento' : 'Ainda não buscado'
  }
}

const btn = 'rounded bg-raised px-3 py-1 text-sm text-fg hover:bg-hover disabled:text-muted'

// TimeToBeatEditor is the "Tempo para zerar" section of the edit window; every action applies at once,
// outside the form [TTB-018, TTB-019, TTB-040].
export function TimeToBeatEditor({ gameId, title }: { gameId: number; title: string }): JSX.Element {
  const qc = useQueryClient()
  const enabled = useTimeToBeatEnabled()
  const q = useTimeToBeat(gameId, { fetchAfterMs: enabled === true ? 0 : null })
  const v = q.data
  const [busy, setBusy] = useState(false)
  const [failed, setFailed] = useState(false)
  const [open, setOpen] = useState(false)
  const [term, setTerm] = useState(title)
  const [found, setFound] = useState<TimeToBeatCandidate[] | null>(null)

  const candidates = (found ?? v?.candidates ?? []).slice(0, MAX_CANDIDATES)
  const source = hasTimeToBeatValue(v) ? timeToBeatSource(v) : null

  async function run(action: () => Promise<TimeToBeatView>, closeSearch: boolean) {
    setBusy(true)
    setFailed(false)
    try {
      const next = await action()
      qc.setQueryData(timeToBeatKey(gameId), next)
      if (closeSearch) {
        setOpen(false)
        setFound(null)
      }
    } catch {
      setFailed(true)
    } finally {
      setBusy(false)
    }
  }

  async function search() {
    setBusy(true)
    setFailed(false)
    try {
      setFound(await searchTimeToBeat(gameId, term))
    } catch {
      setFailed(true)
    } finally {
      setBusy(false)
    }
  }

  return (
    <section className="flex flex-col gap-2 rounded border border-line p-3">
      <h3 className="text-sm font-medium text-fg">Tempo para zerar (HowLongToBeat)</h3>
      <ul className="flex flex-col text-sm">
        {timeToBeatRows(v).map((r) => (
          <li key={r.label} className="flex justify-between gap-3">
            <span className="text-muted">{r.label}</span>
            <span>{r.value}</span>
          </li>
        ))}
      </ul>
      {source !== null && <p className="text-xs text-muted">{`Fonte: ${source}`}</p>}
      <p className="text-xs text-muted">{stateLine(v)}</p>
      {enabled === false && <p className="text-xs text-muted">Consulta ao HowLongToBeat desligada</p>}
      {failed && <p className="text-xs text-danger">{FAILURE}</p>}

      {enabled !== false && (
        <>
          <div className="flex flex-wrap gap-2">
            <button type="button" disabled={busy} onClick={() => void run(() => refreshTimeToBeat(gameId), false)} className={btn}>
              {busy ? 'Buscando…' : 'Atualizar'}
            </button>
            <button type="button" onClick={() => setOpen(true)} className={btn}>
              Escolher outro…
            </button>
          </div>
          {open && (
            <div className="flex flex-col gap-2">
              <div className="flex gap-2">
                <input
                  type="text"
                  aria-label="Termo de busca"
                  value={term}
                  onChange={(e) => setTerm(e.target.value)}
                  className="flex-1 rounded border border-line bg-raised px-3 py-1 text-sm text-fg focus:border-accent"
                />
                <button type="button" disabled={busy} onClick={() => void search()} className={btn}>
                  Buscar
                </button>
              </div>
              <ul className="flex flex-col gap-1">
                {candidates.map((c) => (
                  <li key={c.externalId}>
                    <button
                      type="button"
                      disabled={busy}
                      onClick={() => void run(() => chooseTimeToBeat(gameId, c.externalId), true)}
                      className="w-full rounded bg-raised px-3 py-1 text-left text-sm text-fg hover:bg-hover"
                    >
                      {c.title}
                      {c.year !== null ? ` (${c.year})` : ''} · {c.platforms.join(', ')} · {formatTimeToBeat(c.mainSeconds)} ·{' '}
                      {formatTimeToBeat(c.mainExtraSeconds)} · {formatTimeToBeat(c.completionistSeconds)}
                    </button>
                  </li>
                ))}
              </ul>
              <button type="button" disabled={busy} onClick={() => void run(() => chooseTimeToBeat(gameId, ''), true)} className={btn}>
                Nenhum destes
              </button>
            </div>
          )}
        </>
      )}
    </section>
  )
}
