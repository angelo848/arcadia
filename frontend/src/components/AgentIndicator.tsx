import type { JSX } from 'react'
import type { AgentStatusView } from '../lib/agent'

// displayVersion prefixes "v" only for numeric versions (e.g. "0.1.0" -> "v0.1.0");
// values like "dev" are shown as they are.
function displayVersion(version: string): string {
  return /^\d/.test(version) ? `v${version}` : version
}

// AgentIndicator shows the agent connection state (UI-001). The version is only
// rendered while connected, even if a stale value is present.
// elevated is true when the connected agent runs as administrator (ADM-002); it is
// not shown while disconnected or while the check is still pending.
export function AgentIndicator({
  status,
  elevated,
}: {
  status: AgentStatusView
  elevated?: boolean
}): JSX.Element {
  if (!status.connected) {
    return (
      <div role="status" data-state="disconnected" className="flex items-center gap-2 text-sm">
        <span aria-hidden="true" className="h-2.5 w-2.5 rounded-full bg-danger" />
        <span className="font-medium text-danger">Desconectado</span>
      </div>
    )
  }

  return (
    <div role="status" data-state="connected" className="flex items-center gap-2 text-sm">
      <span aria-hidden="true" className="h-2.5 w-2.5 rounded-full bg-running" />
      {/* Explicit spaces keep the words separated in the text content. */}
      <span className="font-medium text-running">Conectado</span>{' '}
      <span className="text-muted">{displayVersion(status.version)}</span>
      {elevated === true && <span className="text-muted">· Agente com privilégio de administrador</span>}
    </div>
  )
}
