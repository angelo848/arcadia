import type { JSX } from 'react'

// AgentConflict tells the user that a connected agent runs without administrator
// rights, so this UI does not use it (ADM-006). The text names the way out.
export function AgentConflict(): JSX.Element {
  return (
    <div role="alert" className="mx-6 my-3 rounded border border-warning bg-surface p-4 text-sm text-warning">
      Já há um agente rodando sem privilégio de administrador. Saia dele pela bandeja e abra o Arcadia de novo como
      administrador.
    </div>
  )
}
