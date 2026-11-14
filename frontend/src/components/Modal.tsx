import { useEffect, useId, useRef, type JSX, type KeyboardEvent, type ReactNode, type RefObject } from 'react'

export type ModalSize = 'large' | 'medium' | 'compact'

export type ModalProps = {
  size: ModalSize
  title: string
  onClose: () => void
  // footer stays pinned below the scrolling body.
  footer?: ReactNode
  // initialFocus is the element focused on open; the first control is used when absent.
  initialFocus?: RefObject<HTMLElement | null>
  // closeDisabled makes Esc a no-op (an operation is in flight).
  closeDisabled?: boolean
  children: ReactNode
}

const focusableSelector = [
  'button:not([disabled])',
  'input:not([disabled])',
  'select:not([disabled])',
  'textarea:not([disabled])',
  'a[href]',
  '[tabindex]:not([tabindex="-1"])',
].join(', ')

// Modal is the shared shell of the dialogs [UIX-037..039]: fixed-size panel with a pinned header and footer,
// a scrolling body, focus trapped inside, Esc to close and focus restored on unmount.
export function Modal(props: ModalProps): JSX.Element {
  const { size, title, onClose, footer, initialFocus, closeDisabled = false, children } = props
  const titleId = useId()
  const dialogRef = useRef<HTMLDivElement>(null)

  // Read during the first render, before the commit can remove the opener from the document.
  const openerRef = useRef<Element | null>(document.activeElement)

  useEffect(() => {
    const previous = openerRef.current
    const target = initialFocus?.current ?? dialogRef.current?.querySelector<HTMLElement>(focusableSelector)
    target?.focus()
    return () => {
      if (previous instanceof HTMLElement && document.contains(previous)) {
        previous.focus()
        return
      }
      document.querySelector<HTMLElement>('[role="listbox"][aria-label="Jogos"]')?.focus()
    }
    // Runs once: focus is taken on open and given back on close.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  function onKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (event.key === 'Escape') {
      event.preventDefault()
      event.stopPropagation()
      if (!closeDisabled) {
        onClose()
      }
      return
    }
    if (event.key !== 'Tab') {
      return
    }
    const dialog = dialogRef.current
    if (dialog === null) {
      return
    }
    const items = Array.from(dialog.querySelectorAll<HTMLElement>(focusableSelector))
    const first = items[0]
    const last = items[items.length - 1]
    if (first === undefined || last === undefined) {
      return
    }
    const active = document.activeElement
    const outside = active === null || !dialog.contains(active)
    if (event.shiftKey && (outside || active === first)) {
      event.preventDefault()
      last.focus()
    } else if (!event.shiftKey && (outside || active === last)) {
      event.preventDefault()
      first.focus()
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-overlay p-4">
      <div
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        data-size={size}
        onKeyDown={onKeyDown}
        className={`modal-${size} flex max-h-full flex-col overflow-hidden rounded-lg border border-line bg-surface text-fg shadow-xl`}
      >
        <div className="shrink-0 border-b border-line px-5 py-3">
          <h2 id={titleId} className="text-lg font-semibold text-fg">
            {title}
          </h2>
        </div>
        <div className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto px-5 py-4">{children}</div>
        {footer !== undefined && (
          <div className="flex shrink-0 justify-end gap-2 border-t border-line px-5 py-3">{footer}</div>
        )}
      </div>
    </div>
  )
}
