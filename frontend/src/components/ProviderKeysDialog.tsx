import { useEffect, useId, useState, type JSX } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import {
  deleteProviderKey,
  providerKeyStatus,
  setProviderKey,
  type KeyProviderId,
} from '../lib/metadata'

const inputClass =
  'rounded border border-neutral-700 bg-neutral-950 px-3 py-1.5 text-neutral-100 focus:border-emerald-500 focus:outline-none'

type Configured = Record<KeyProviderId, boolean | null>

const unknownStatus: Configured = { igdb: null, steamgriddb: null }

function messageOf(e: unknown): string {
  return e instanceof Error ? e.message : 'Não foi possível salvar. Tente de novo.'
}

// ProviderKeysDialog registers the keys of IGDB and SteamGridDB [MID-001]. The UI shows only
// whether a key is registered, never its value; the fields are cleared after saving.
export function ProviderKeysDialog({ onClose }: { onClose: () => void }): JSX.Element {
  const qc = useQueryClient()
  const titleId = useId()
  const idPrefix = useId()
  const [configured, setConfigured] = useState<Configured>(unknownStatus)
  const [igdbId, setIgdbId] = useState('')
  const [igdbSecret, setIgdbSecret] = useState('')
  const [sgdbKey, setSgdbKey] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function refresh() {
    const [igdb, sgdb] = await Promise.all([providerKeyStatus('igdb'), providerKeyStatus('steamgriddb')])
    setConfigured({ igdb: igdb.configured, steamgriddb: sgdb.configured })
  }

  useEffect(() => {
    refresh().catch((e: unknown) => setError(messageOf(e)))
    // refresh reads the statuses once when the dialog opens.
  }, [])

  async function save(provider: KeyProviderId) {
    setBusy(true)
    setError(null)
    try {
      if (provider === 'igdb') {
        await setProviderKey('igdb', igdbId, igdbSecret)
        setIgdbId('')
        setIgdbSecret('')
      } else {
        await setProviderKey('steamgriddb', sgdbKey)
        setSgdbKey('')
      }
      await refresh()
      await qc.invalidateQueries({ queryKey: ['media'] })
    } catch (e) {
      setError(messageOf(e))
    } finally {
      setBusy(false)
    }
  }

  async function remove(provider: KeyProviderId) {
    setBusy(true)
    setError(null)
    try {
      await deleteProviderKey(provider)
      await refresh()
    } catch (e) {
      setError(messageOf(e))
    } finally {
      setBusy(false)
    }
  }

  const label = (on: boolean | null) => (on === null ? 'Verificando…' : on ? 'Cadastrada' : 'Não cadastrada')

  return (
    <div className="fixed inset-0 z-40 flex items-center justify-center bg-black/60 p-4">
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        className="flex max-h-full w-full max-w-lg flex-col gap-4 overflow-y-auto rounded-lg bg-neutral-900 p-5"
      >
        <div className="flex items-center justify-between">
          <h2 id={titleId} className="text-lg font-semibold text-neutral-100">
            Chaves dos provedores
          </h2>
          <button type="button" onClick={onClose} className="text-sm text-neutral-300 hover:text-neutral-100">
            Fechar
          </button>
        </div>
        <p className="text-sm text-neutral-400">
          As chaves ficam no Gerenciador de Credenciais do Windows, nunca no banco. Steam Store não precisa de chave.
        </p>

        {error !== null && (
          <p role="alert" className="text-sm text-red-400">
            {error}
          </p>
        )}

        <section className="flex flex-col gap-2 rounded border border-neutral-800 p-3">
          <div className="flex items-center justify-between">
            <h3 className="text-sm font-medium text-neutral-200">IGDB</h3>
            <span className="text-xs text-neutral-400">{label(configured.igdb)}</span>
          </div>
          <label htmlFor={`${idPrefix}-igdb-id`} className="flex flex-col gap-1 text-sm text-neutral-300">
            Client ID
            <input
              id={`${idPrefix}-igdb-id`}
              type="password"
              autoComplete="off"
              value={igdbId}
              onChange={(e) => setIgdbId(e.target.value)}
              className={inputClass}
            />
          </label>
          <label htmlFor={`${idPrefix}-igdb-secret`} className="flex flex-col gap-1 text-sm text-neutral-300">
            Client secret
            <input
              id={`${idPrefix}-igdb-secret`}
              type="password"
              autoComplete="off"
              value={igdbSecret}
              onChange={(e) => setIgdbSecret(e.target.value)}
              className={inputClass}
            />
          </label>
          <div className="flex justify-end gap-2">
            {configured.igdb === true && (
              <button type="button" disabled={busy} onClick={() => void remove('igdb')} className="rounded px-3 py-1.5 text-sm text-red-300 hover:bg-neutral-800 disabled:text-neutral-500">
                Remover
              </button>
            )}
            <button
              type="button"
              disabled={busy || igdbId.trim() === '' || igdbSecret.trim() === ''}
              onClick={() => void save('igdb')}
              className="rounded bg-emerald-600 px-4 py-1.5 text-sm font-medium text-white hover:bg-emerald-500 disabled:cursor-not-allowed disabled:bg-neutral-700 disabled:text-neutral-400"
            >
              Salvar
            </button>
          </div>
        </section>

        <section className="flex flex-col gap-2 rounded border border-neutral-800 p-3">
          <div className="flex items-center justify-between">
            <h3 className="text-sm font-medium text-neutral-200">SteamGridDB</h3>
            <span className="text-xs text-neutral-400">{label(configured.steamgriddb)}</span>
          </div>
          <label htmlFor={`${idPrefix}-sgdb`} className="flex flex-col gap-1 text-sm text-neutral-300">
            Chave da API
            <input
              id={`${idPrefix}-sgdb`}
              type="password"
              autoComplete="off"
              value={sgdbKey}
              onChange={(e) => setSgdbKey(e.target.value)}
              className={inputClass}
            />
          </label>
          <div className="flex justify-end gap-2">
            {configured.steamgriddb === true && (
              <button type="button" disabled={busy} onClick={() => void remove('steamgriddb')} className="rounded px-3 py-1.5 text-sm text-red-300 hover:bg-neutral-800 disabled:text-neutral-500">
                Remover
              </button>
            )}
            <button
              type="button"
              disabled={busy || sgdbKey.trim() === ''}
              onClick={() => void save('steamgriddb')}
              className="rounded bg-emerald-600 px-4 py-1.5 text-sm font-medium text-white hover:bg-emerald-500 disabled:cursor-not-allowed disabled:bg-neutral-700 disabled:text-neutral-400"
            >
              Salvar
            </button>
          </div>
        </section>
      </div>
    </div>
  )
}
