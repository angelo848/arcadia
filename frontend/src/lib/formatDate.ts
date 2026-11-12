// Date helpers for the game page [UI-037, UI-038].

function pad2(n: number): string {
  return String(n).padStart(2, '0')
}

// formatLocalDateTime renders an ISO instant in the local zone as DD/MM/YYYY HH:MM.
// An empty value (never played) is shown as "Nunca"; an invalid value renders as "".
export function formatLocalDateTime(iso: string): string {
  if (iso === '') {
    return 'Nunca'
  }
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) {
    return ''
  }
  const day = pad2(d.getDate())
  const month = pad2(d.getMonth() + 1)
  const hours = pad2(d.getHours())
  const minutes = pad2(d.getMinutes())
  return `${day}/${month}/${d.getFullYear()} ${hours}:${minutes}`
}

// formatUtcDate renders the calendar day of an ISO value in UTC as YYYY-MM-DD.
// The completion date arrives as a plain YYYY-MM-DD stored as midnight UTC, so it must
// show the same day in every zone [UI-037]. An empty or invalid value renders as "".
export function formatUtcDate(iso: string): string {
  if (iso === '') {
    return ''
  }
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) {
    return ''
  }
  const year = String(d.getUTCFullYear()).padStart(4, '0')
  return `${year}-${pad2(d.getUTCMonth() + 1)}-${pad2(d.getUTCDate())}`
}
