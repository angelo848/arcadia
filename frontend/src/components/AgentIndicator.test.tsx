import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'
import { AgentIndicator } from './AgentIndicator'
import { formatUptime } from '../lib/format'
import type { AgentStatusView } from '../lib/agent'

const baseStatus: AgentStatusView = {
  connected: true,
  version: '0.1.0-test',
  pid: 4242,
  startedAt: '2026-11-08T10:00:00Z',
  dataDir: 'C:\\Users\\test\\AppData\\Local\\Arcadia',
  uptimeSeconds: 3723,
}

// classesInTree returns the class attribute of the root and every descendant
// (getAttribute works for both HTML and SVG elements).
function classesInTree(root: Element): string[] {
  const all = [root, ...Array.from(root.querySelectorAll('*'))]
  return all.map((el) => el.getAttribute('class') ?? '')
}

const greenRe = /green|emerald/
const redRe = /red|rose/

describe('AgentIndicator', () => {
  it('UI-001/conectado mostra Conectado em verde com versão e uptime', () => {
    render(<AgentIndicator status={baseStatus} />)

    const indicator = screen.getByRole('status')
    expect(indicator).toHaveAttribute('data-state', 'connected')
    // \b avoids matching the "conectado" inside "Desconectado".
    expect(indicator).toHaveTextContent(/\bConectado\b/)
    expect(indicator).not.toHaveTextContent(/Desconectado/)
    expect(indicator).toHaveTextContent(baseStatus.version)
    expect(indicator).toHaveTextContent(formatUptime(baseStatus.uptimeSeconds))

    const classes = classesInTree(indicator)
    expect(classes.some((c) => greenRe.test(c))).toBe(true)
    expect(classes.some((c) => redRe.test(c))).toBe(false)
  })

  it('UI-001/desconectado mostra Desconectado em vermelho sem versão nem uptime', () => {
    // Stale version/uptime are passed on purpose: the indicator must hide them
    // whenever the agent is disconnected.
    const status: AgentStatusView = { ...baseStatus, connected: false }
    render(<AgentIndicator status={status} />)

    const indicator = screen.getByRole('status')
    expect(indicator).toHaveAttribute('data-state', 'disconnected')
    expect(indicator).toHaveTextContent(/Desconectado/)
    expect(indicator).not.toHaveTextContent(/\bConectado\b/)
    expect(indicator).not.toHaveTextContent(status.version)
    expect(indicator).not.toHaveTextContent(formatUptime(status.uptimeSeconds))
    // No uptime-like text at all (e.g. "0s", "1m 00s").
    expect(indicator).not.toHaveTextContent(/\b\d+s\b/)

    const classes = classesInTree(indicator)
    expect(classes.some((c) => redRe.test(c))).toBe(true)
    expect(classes.some((c) => greenRe.test(c))).toBe(false)
  })
})
