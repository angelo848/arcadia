import { useEffect } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'

// AgentStatusView mirrors cmd/desktop StatusView (JSON field names).
export type AgentStatusView = {
  connected: boolean
  version: string
  pid: number
  startedAt: string
  dataDir: string
  uptimeSeconds: number
}

// LibrarySummaryView mirrors cmd/desktop LibrarySummaryView.
export type LibrarySummaryView = { games: number; platforms: number }

// The generated Wails bindings (frontend/wailsjs) only exist after T12, so the
// bound methods and runtime are reached through window with local types.
type WailsApp = {
  AgentStatus?: () => Promise<AgentStatusView>
  LibrarySummary?: () => Promise<LibrarySummaryView>
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
const librarySummaryKey = ['library', 'summary'] as const

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

async function fetchLibrarySummary(): Promise<LibrarySummaryView> {
  const fn = wails().go?.main?.App?.LibrarySummary
  if (!fn) {
    throw new Error('wails bindings unavailable: LibrarySummary')
  }
  try {
    return await fn()
  } catch (e) {
    // Wails v2 rejects bound calls with a plain string; normalize to Error.
    throw e instanceof Error ? e : new Error(String(e))
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

// useLibrarySummary loads the library counters. While the call fails (e.g. the
// agent is not ready yet, DSK-008) it keeps retrying every 2 s.
export function useLibrarySummary() {
  return useQuery({
    queryKey: librarySummaryKey,
    queryFn: fetchLibrarySummary,
    retry: false,
    refetchInterval: (query) => (query.state.status === 'error' ? 2000 : false),
  })
}
