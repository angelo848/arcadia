import type { JSX } from 'react'

export type ChartBar = { label: string; value: number; current?: boolean }

// StatsChart draws simple vertical bars in SVG, without a chart library (EST-002). The current bar is
// marked with a stronger colour; every bar has a text title with its value.
export function StatsChart({ bars, label, format }: { bars: ChartBar[]; label: string; format: (seconds: number) => string }): JSX.Element {
  const width = 640
  const height = 180
  const pad = 24
  const max = Math.max(1, ...bars.map((b) => b.value))
  const slot = bars.length > 0 ? (width - pad * 2) / bars.length : 0
  const barWidth = Math.max(4, slot * 0.6)

  return (
    <svg role="img" aria-label={label} viewBox={`0 0 ${width} ${height}`} className="w-full" preserveAspectRatio="none">
      {bars.map((b, i) => {
        const h = Math.round((b.value / max) * (height - pad * 2))
        const x = pad + i * slot + (slot - barWidth) / 2
        const y = height - pad - h
        return (
          <g key={`${b.label}-${i}`}>
            <title>{`${b.label}: ${format(b.value)}`}</title>
            <rect x={x} y={y} width={barWidth} height={Math.max(h, 1)} rx={2} className={b.current ? 'fill-emerald-400' : 'fill-neutral-600'} />
            <text x={x + barWidth / 2} y={height - 8} textAnchor="middle" className="fill-neutral-400" fontSize="10">
              {b.label}
            </text>
          </g>
        )
      })}
    </svg>
  )
}
