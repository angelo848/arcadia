import type { Hint } from './hints'

// Footer renders the model's hints as text [BBX-015].
export function Footer({ hints }: { hints: Hint[] }) {
  return (
    <ul className="flex flex-wrap items-center gap-x-6 gap-y-1 text-bb-text-muted">
      {hints.map((h) => (
        <li key={`${h.button}-${h.label}`} className="flex items-center gap-2">
          <span className="rounded bg-bb-surface px-2 py-0.5 font-semibold text-bb-text">{h.button}</span>
          <span>{h.label}</span>
        </li>
      ))}
    </ul>
  )
}
