import type { GameStatus } from './filters'
import type { EditDraft } from './gamePatch'

// Messages keyed by the draft field they refer to; an empty object means the draft is valid.
export type DraftErrors = Partial<Record<keyof EditDraft, string>>

// Limits of UI-040. Lengths count runes (code points), as the server does (EDT-001).
const titleMaxRunes = 200
// Longest launch target and arguments, in runes (EDT-010).
const launchMaxRunes = 2000

// isAbsolutePath accepts a Windows drive path or a UNC path.
function isAbsolutePath(p: string): boolean {
  const drive = /^[A-Za-z]:/.test(p) && (p.charAt(2) === '\\' || p.charAt(2) === '/')
  const unc = p.startsWith('\\\\')
  return drive || unc
}
const descriptionMaxRunes = 10000
const nameMaxRunes = 100
const yearMin = 1900
const yearMax = 2100

const statuses: readonly GameStatus[] = ['not_played', 'playing', 'completed', 'abandoned', 'backlog']

function runeCount(s: string): number {
  return Array.from(s).length
}

// isCalendarDay accepts AAAA-MM-DD that names a real day (no 2026-02-30).
function isCalendarDay(s: string): boolean {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(s)) {
    return false
  }
  const [y, m, d] = s.split('-').map(Number) as [number, number, number]
  const dt = new Date(Date.UTC(y, m - 1, d))
  return dt.getUTCFullYear() === y && dt.getUTCMonth() === m - 1 && dt.getUTCDate() === d
}

function listError(names: readonly string[], label: string): string | undefined {
  if (names.some((n) => n.trim() === '')) {
    return `${label}: remova os nomes em branco.`
  }
  if (names.some((n) => runeCount(n.trim()) > nameMaxRunes)) {
    return `${label}: cada nome tem até ${nameMaxRunes} caracteres.`
  }
  return undefined
}

// validateDraft checks the edit form before it is sent (UI-040). The agent validates again (EDT-001).
export function validateDraft(d: EditDraft): DraftErrors {
  const errors: DraftErrors = {}

  const title = d.title.trim()
  if (title === '') {
    errors.title = 'Informe um título.'
  } else if (runeCount(title) > titleMaxRunes) {
    errors.title = `O título tem no máximo ${titleMaxRunes} caracteres.`
  }

  if (runeCount(d.description) > descriptionMaxRunes) {
    errors.description = `A descrição tem no máximo ${descriptionMaxRunes} caracteres.`
  }

  const year = d.year.trim()
  if (year !== '' && !(/^\d{4}$/.test(year) && Number(year) >= yearMin && Number(year) <= yearMax)) {
    errors.year = `O ano deve estar entre ${yearMin} e ${yearMax}.`
  }

  const completed = d.completedAt.trim()
  if (completed !== '' && !isCalendarDay(completed)) {
    errors.completedAt = 'Use a data no formato AAAA-MM-DD.'
  }

  if (!statuses.includes(d.status)) {
    errors.status = 'Escolha um status da lista.'
  }

  const developers = listError(d.developers, 'Desenvolvedora')
  if (developers !== undefined) {
    errors.developers = developers
  }
  const publishers = listError(d.publishers, 'Editora')
  if (publishers !== undefined) {
    errors.publishers = publishers
  }
  const genres = listError(d.genres, 'Gêneros')
  if (genres !== undefined) {
    errors.genres = genres
  }

  // Launch fields (EDT-010): the server checks them again; these messages come first.
  const target = d.launchTarget.trim()
  if (target === "") {
    errors.launchTarget = "Informe o executável ou a URI."
  } else if (runeCount(target) > launchMaxRunes) {
    errors.launchTarget = `Use no máximo ${launchMaxRunes} caracteres.`
  } else if ((d.launchType === "exe" || d.launchType === "emulator") && !isAbsolutePath(target)) {
    errors.launchTarget = "Use o caminho completo do executável (com a unidade, como C:\\Jogos\\jogo.exe)."
  }
  if (runeCount(d.launchArgs) > launchMaxRunes) {
    errors.launchArgs = `Use no máximo ${launchMaxRunes} caracteres.`
  }

  return errors
}
