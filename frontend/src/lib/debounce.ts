// Debounce for the library search box (UI-033): the query is applied only after the last keystroke.

export const SEARCH_DEBOUNCE_MS = 300

export type Timers = {
  setTimeout: (f: () => void, ms: number) => unknown
  clearTimeout: (h: unknown) => void
}

type ClearHandle = Parameters<typeof globalThis.clearTimeout>[0]

// Resolves the global timers at call time, so fake timers installed after import are honoured.
const globalTimers: Timers = {
  setTimeout: (f, ms) => globalThis.setTimeout(f, ms),
  clearTimeout: (h) => globalThis.clearTimeout(h as ClearHandle),
}

export type Debouncer<T> = {
  call(v: T): void
  cancel(): void
}

export function createDebouncer<T>(ms: number, apply: (v: T) => void, timers: Timers = globalTimers): Debouncer<T> {
  let handle: unknown = undefined
  let pending = false

  const cancel = (): void => {
    if (pending) {
      timers.clearTimeout(handle)
      pending = false
      handle = undefined
    }
  }

  return {
    call(v: T): void {
      cancel()
      pending = true
      handle = timers.setTimeout(() => {
        pending = false
        handle = undefined
        apply(v)
      }, ms)
    },
    cancel,
  }
}
