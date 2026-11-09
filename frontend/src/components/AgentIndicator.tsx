import type { JSX } from 'react'
import type { AgentStatusView } from '../lib/agent'

// displayVersion prefixes "v" only for numeric versions (e.g. "0.1.0" -> "v0.1.0");
// values like "dev" are shown as they are.
function displayVersion(version: string): string {
  return /^\d/.test(version) ? `v${version}` : version
}

// AgentIndicator shows the agent connection state (UI-001). The version is only
// rendered while connected, even if a stale value is present.
export function AgentIndicator({ status }: { status: AgentStatusView }): JSX.Element {
  if (!status.connected) {
    return (
      <div role="status" data-state="disconnected" className="flex items-center gap-2 text-sm">
        <span aria-hidden="true" className="h-2.5 w-2.5 rounded-full bg-red-500" />
        <span className="font-medium text-red-400">Desconectado</span>
      </div>
    )
  }

  return (
    <div role="status" data-state="connected" className="flex items-center gap-2 text-sm">
      <span aria-hidden="true" className="h-2.5 w-2.5 rounded-full bg-emerald-500" />
      {/* Explicit spaces keep the words separated in the text content. */}
      <span className="font-medium text-emerald-400">Conectado</span>{' '}
      <span className="text-neutral-400">{displayVersion(status.version)}</span>
    </div>
  )
}
