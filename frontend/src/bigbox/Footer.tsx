import type { Hint } from './hints'
import { Panel } from './Panel'

// Footer renders the model's hints as text, on a panel [BBX-015, BBX-037].
export function Footer({ hints }: { hints: Hint[] }) {
  return (
    <Panel>
      <ul className="flex flex-wrap items-center gap-x-6 gap-y-1 text-bb-text-muted">
        {hints.map((h) => (
          <li key={`${h.button}-${h.label}`} className="flex items-center gap-2">
            <span className="rounded bg-bb-surface px-2 py-0.5 font-semibold text-bb-text">{h.button}</span>
            <span>{h.label}</span>
          </li>
        ))}
      </ul>
    </Panel>
  )
}
