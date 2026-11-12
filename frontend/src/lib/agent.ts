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
  AgentElevated?: () => Promise<boolean>
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

// fetchAgentElevated never rejects. A missing binding is "unknown" (null) and raises no
// warning. A failed check counts as "not elevated", so the UI shows the warning instead of
// using that agent (ADM-006).
async function fetchAgentElevated(): Promise<boolean | null> {
  const fn = wails().go?.main?.App?.AgentElevated
  if (!fn) {
    return null
  }
  try {
    return await fn()
  } catch {
    return false
  }
}

// useAgentElevated asks, once per agent process, whether the connected agent runs
// with administrator rights (ADM-006). It is disabled while no agent is connected.
export function useAgentElevated(agent: AgentStatusView) {
  return useQuery({
    queryKey: ['agent', 'elevated', agent.pid],
    queryFn: fetchAgentElevated,
    enabled: agent.connected,
    staleTime: Infinity,
    retry: false,
  })
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
