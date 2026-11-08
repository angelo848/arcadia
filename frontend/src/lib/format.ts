// pad2 renders a non-negative integer with at least two digits.
function pad2(n: number): string {
  return n.toString().padStart(2, '0')
}

// formatUptime renders an uptime in seconds as "Ns", "Mm SSs" or "Hh MMm SSs" (UI-003).
// Invalid input is handled defensively: NaN/Infinity/negative become 0 and fractions are floored.
export function formatUptime(seconds: number): string {
  const total = Number.isFinite(seconds) ? Math.max(0, Math.floor(seconds)) : 0

  if (total < 60) {
    return `${total}s`
  }

  const s = total % 60
  if (total < 3600) {
    const m = Math.floor(total / 60)
    return `${m}m ${pad2(s)}s`
  }

  const h = Math.floor(total / 3600)
  const m = Math.floor((total % 3600) / 60)
  return `${h}h ${pad2(m)}m ${pad2(s)}s`
}
