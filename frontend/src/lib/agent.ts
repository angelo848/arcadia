import { useEffect } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'

// AgentStatusView mirrors cmd/arcadia StatusView (JSON field names).
export type AgentStatusView = {
  connected: boolean
  version: string
  pid: number
  startedAt: string
  dataDir: string
  uptimeSeconds: number
}

// The bound methods and runtime are reached through window with local types
// (same pattern as lib/games.ts), so tests can fake window.go and window.runtime.
type WailsApp = {
  AgentStatus?: () => Promise<AgentStatusView>
}

type WailsRuntime = {
  EventsOn?: (eventName: string, callback: (...data: unknown[]) => void) => (() => void) | void
}

type WailsWindow = {
  go?: { main?: { App?: WailsApp } }
  runtime?: WailsRuntime
}

function wails(): WailsWindow {
  return window as unknown as WailsWindow
}

export const disconnectedStatus: AgentStatusView = {
  connected: false,
  version: '',
  pid: 0,
  startedAt: '',
  dataDir: '',
  uptimeSeconds: 0,
}

const agentStatusKey = ['agent', 'status'] as const

// fetchAgentStatus never rejects: missing bindings or a failed call mean "disconnected".
async function fetchAgentStatus(): Promise<AgentStatusView> {
  const fn = wails().go?.main?.App?.AgentStatus
  if (!fn) {
    return disconnectedStatus
  }
  try {
    return await fn()
  } catch {
    return disconnectedStatus
  }
}

// useAgentStatus polls the agent status every 1 s and refetches immediately on
// each "agent:heartbeat" event forwarded by the desktop app.
export function useAgentStatus() {
  const queryClient = useQueryClient()

  useEffect(() => {
    const on = wails().runtime?.EventsOn
    if (!on) {
      return undefined
    }
    const off = on('agent:heartbeat', () => {
      void queryClient.invalidateQueries({ queryKey: agentStatusKey })
    })
    return () => {
      if (typeof off === 'function') {
        off()
      }
    }
  }, [queryClient])

  return useQuery({
    queryKey: agentStatusKey,
    queryFn: fetchAgentStatus,
    refetchInterval: 1000,
    retry: false,
  })
}
