import { useEffect, useId, useState, type JSX } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { Modal } from './Modal'
import { getTimeToBeatSettings, setTimeToBeatSettings, type TimeToBeatSettings } from '../lib/timeToBeatEdit'
import {
  deleteProviderKey,
  providerKeyStatus,
  setProviderKey,
  type KeyProviderId,
} from '../lib/metadata'
import {
  queuedLabel,
  useVideoStatus,
  videoCancelPending,
  videoCredentialsKey,
  videoQueueMissing,
  videoStatusLine,
} from '../lib/video'

const inputClass =
  'rounded border border-line bg-raised px-3 py-1.5 text-fg focus:border-accent'

type Configured = Record<KeyProviderId, boolean | null>

const unknownStatus: Configured = { igdb: null, steamgriddb: null, screenscraper: null, screenscraperdev: null }

type CredGroupProps = {
  title: string
  status: string
  firstLabel: string
  secondLabel: string
  first: string
  second: string
  onFirst: (v: string) => void
  onSecond: (v: string) => void
  busy: boolean
  registered: boolean
  onSave: () => void
  onRemove: () => void
  idPrefix: string
}

// CredGroup is one pair of password fields with its own Salvar/Remover (ScreenScraper [SSC-002]).
function CredGroup(p: CredGroupProps): JSX.Element {
  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-center justify-between">
        <h4 className="text-sm text-fg">{p.title}</h4>
        <span className="text-xs text-muted">{p.status}</span>
      </div>
      <label htmlFor={`${p.idPrefix}-a`} className="flex flex-col gap-1 text-sm text-muted">
        {p.firstLabel}
        <input id={`${p.idPrefix}-a`} type="password" autoComplete="off" value={p.first} onChange={(e) => p.onFirst(e.target.value)} className={inputClass} />
      </label>
      <label htmlFor={`${p.idPrefix}-b`} className="flex flex-col gap-1 text-sm text-muted">
        {p.secondLabel}
        <input id={`${p.idPrefix}-b`} type="password" autoComplete="off" value={p.second} onChange={(e) => p.onSecond(e.target.value)} className={inputClass} />
      </label>
      <div className="flex justify-end gap-2">
        {p.registered && (
          <button type="button" disabled={p.busy} onClick={p.onRemove} className="rounded px-3 py-1.5 text-sm text-danger hover:bg-hover disabled:text-muted">
            Remover
          </button>
        )}
        <button
          type="button"
          disabled={p.busy || p.first.trim() === '' || p.second.trim() === ''}
          onClick={p.onSave}
          className="rounded bg-accent px-4 py-1.5 text-sm font-medium text-on-accent hover:bg-accent-hover disabled:cursor-not-allowed disabled:bg-raised disabled:text-muted"
        >
          Salvar
        </button>
      </div>
    </div>
  )
}

function messageOf(e: unknown): string {
  return e instanceof Error ? e.message : 'Não foi possível salvar. Tente de novo.'
}

// ProviderKeysDialog registers the keys of IGDB and SteamGridDB [MID-001]. The UI shows only
// whether a key is registered, never its value; the fields are cleared after saving.
export function ProviderKeysDialog({ onClose }: { onClose: () => void }): JSX.Element {
  const qc = useQueryClient()
  const idPrefix = useId()
  const [configured, setConfigured] = useState<Configured>(unknownStatus)
  const [igdbId, setIgdbId] = useState('')
  const [igdbSecret, setIgdbSecret] = useState('')
  const [sgdbKey, setSgdbKey] = useState('')
  const [ssUser, setSsUser] = useState('')
  const [ssPass, setSsPass] = useState('')
  const [ssDevId, setSsDevId] = useState('')
  const [ssDevPass, setSsDevPass] = useState('')
  const [videoMsg, setVideoMsg] = useState<string | null>(null)
  const [ttb, setTtb] = useState<TimeToBeatSettings>({ enabled: false, afterMetadata: false })
  const [ttbLoaded, setTtbLoaded] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const videoReady = configured.screenscraper === true && configured.screenscraperdev === true
  const vstatus = useVideoStatus(videoReady)

  async function refresh() {
    const [igdb, sgdb, ss, ssdev] = await Promise.all([
      providerKeyStatus('igdb'),
      providerKeyStatus('steamgriddb'),
      providerKeyStatus('screenscraper'),
      providerKeyStatus('screenscraperdev'),
    ])
    setConfigured({
      igdb: igdb.configured,
      steamgriddb: sgdb.configured,
      screenscraper: ss.configured,
      screenscraperdev: ssdev.configured,
    })
    void qc.invalidateQueries({ queryKey: videoCredentialsKey })
  }

  async function videoAction(run: () => Promise<string>) {
    setBusy(true)
    setError(null)
    setVideoMsg(null)
    try {
      setVideoMsg(await run())
      void vstatus.refetch()
    } catch (e) {
      setError(messageOf(e))
    } finally {
      setBusy(false)
    }
  }

  useEffect(() => {
    let alive = true
    getTimeToBeatSettings()
      .then((s) => {
        if (alive && s) {
          setTtb(s)
          setTtbLoaded(true)
        }
      })
      .catch(() => {})
    return () => {
      alive = false
    }
  }, [])

  async function changeTtb(enabled: boolean, afterMetadata: boolean) {
    setError(null)
    try {
      const saved = await setTimeToBeatSettings(enabled, afterMetadata)
      setTtb(saved ?? { enabled, afterMetadata })
    } catch (e) {
      setError(messageOf(e))
    }
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
      } else if (provider === 'screenscraper') {
        await setProviderKey('screenscraper', ssUser, ssPass)
        setSsUser('')
        setSsPass('')
      } else if (provider === 'screenscraperdev') {
        await setProviderKey('screenscraperdev', ssDevId, ssDevPass)
        setSsDevId('')
        setSsDevPass('')
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
    <Modal
      size="compact"
      title="Chaves dos provedores"
      onClose={onClose}
      footer={
        <button type="button" onClick={onClose} className="rounded px-3 py-1.5 text-sm text-muted hover:bg-hover">
          Fechar
        </button>
      }
    >
        <p className="text-sm text-muted">
          As chaves ficam no Gerenciador de Credenciais do Windows, nunca no banco. Steam Store não precisa de chave.
        </p>

        {error !== null && (
          <p role="alert" className="text-sm text-danger">
            {error}
          </p>
        )}

        <section className="flex flex-col gap-2 rounded border border-line p-3">
          <div className="flex items-center justify-between">
            <h3 className="text-sm font-medium text-fg">IGDB</h3>
            <span className="text-xs text-muted">{label(configured.igdb)}</span>
          </div>
          <label htmlFor={`${idPrefix}-igdb-id`} className="flex flex-col gap-1 text-sm text-muted">
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
          <label htmlFor={`${idPrefix}-igdb-secret`} className="flex flex-col gap-1 text-sm text-muted">
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
              <button type="button" disabled={busy} onClick={() => void remove('igdb')} className="rounded px-3 py-1.5 text-sm text-danger hover:bg-hover disabled:text-muted">
                Remover
              </button>
            )}
            <button
              type="button"
              disabled={busy || igdbId.trim() === '' || igdbSecret.trim() === ''}
              onClick={() => void save('igdb')}
              className="rounded bg-accent px-4 py-1.5 text-sm font-medium text-on-accent hover:bg-accent-hover disabled:cursor-not-allowed disabled:bg-raised disabled:text-muted"
            >
              Salvar
            </button>
          </div>
        </section>

        <section className="flex flex-col gap-2 rounded border border-line p-3">
          <div className="flex items-center justify-between">
            <h3 className="text-sm font-medium text-fg">SteamGridDB</h3>
            <span className="text-xs text-muted">{label(configured.steamgriddb)}</span>
          </div>
          <label htmlFor={`${idPrefix}-sgdb`} className="flex flex-col gap-1 text-sm text-muted">
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
              <button type="button" disabled={busy} onClick={() => void remove('steamgriddb')} className="rounded px-3 py-1.5 text-sm text-danger hover:bg-hover disabled:text-muted">
                Remover
              </button>
            )}
            <button
              type="button"
              disabled={busy || sgdbKey.trim() === ''}
              onClick={() => void save('steamgriddb')}
              className="rounded bg-accent px-4 py-1.5 text-sm font-medium text-on-accent hover:bg-accent-hover disabled:cursor-not-allowed disabled:bg-raised disabled:text-muted"
            >
              Salvar
            </button>
          </div>
        </section>

        <section className="flex flex-col gap-2 rounded border border-line p-3">
          <h3 className="text-sm font-medium text-fg">ScreenScraper (vídeos)</h3>
          <CredGroup
            title="Conta do usuário"
            status={label(configured.screenscraper)}
            firstLabel="Usuário"
            secondLabel="Senha"
            first={ssUser}
            second={ssPass}
            onFirst={setSsUser}
            onSecond={setSsPass}
            busy={busy}
            registered={configured.screenscraper === true}
            onSave={() => void save('screenscraper')}
            onRemove={() => void remove('screenscraper')}
            idPrefix={`${idPrefix}-ss`}
          />
          <CredGroup
            title="Desenvolvedor do Arcadia"
            status={label(configured.screenscraperdev)}
            firstLabel="devid"
            secondLabel="devpassword"
            first={ssDevId}
            second={ssDevPass}
            onFirst={setSsDevId}
            onSecond={setSsDevPass}
            busy={busy}
            registered={configured.screenscraperdev === true}
            onSave={() => void save('screenscraperdev')}
            onRemove={() => void remove('screenscraperdev')}
            idPrefix={`${idPrefix}-ssdev`}
          />
          {videoReady && (
            <div className="flex flex-col gap-2">
              {vstatus.data !== undefined && <p className="text-xs text-muted">{videoStatusLine(vstatus.data)}</p>}
              {videoMsg !== null && <p className="text-sm text-fg">{videoMsg}</p>}
              <div className="flex flex-wrap gap-2">
                <button
                  type="button"
                  disabled={busy}
                  onClick={() => void videoAction(async () => queuedLabel((await videoQueueMissing(0)).queued))}
                  className="rounded bg-raised px-3 py-1.5 text-sm text-fg hover:bg-hover disabled:text-muted"
                >
                  Buscar vídeos dos jogos sem vídeo
                </button>
                <button
                  type="button"
                  disabled={busy}
                  onClick={() => void videoAction(async () => `${await videoCancelPending()} buscas canceladas`)}
                  className="rounded bg-raised px-3 py-1.5 text-sm text-fg hover:bg-hover disabled:text-muted"
                >
                  Cancelar buscas pendentes
                </button>
              </div>
            </div>
          )}
        </section>

        <section className="flex flex-col gap-2 rounded border border-line p-3">
          <h3 className="text-sm font-medium text-fg">HowLongToBeat (tempo para zerar)</h3>
          <p className="text-xs text-muted">Serviço não oficial: pode parar de funcionar sem aviso</p>
          <p className="text-xs text-muted">
            Se o HowLongToBeat falhar ou não achar o jogo, o IGDB é usado como reserva (precisa da chave do IGDB acima)
          </p>
          <label className="flex items-center gap-2 text-sm text-fg">
            <input
              type="checkbox"
              checked={ttb.enabled}
              disabled={!ttbLoaded}
              onChange={() => void changeTtb(!ttb.enabled, ttb.afterMetadata)}
            />
            Consultar o HowLongToBeat
          </label>
          <label className="flex items-center gap-2 text-sm text-fg">
            <input
              type="checkbox"
              checked={ttb.afterMetadata}
              disabled={!ttbLoaded || !ttb.enabled}
              onChange={() => {
                if (ttbLoaded && ttb.enabled) {
                  void changeTtb(true, !ttb.afterMetadata)
                }
              }}
            />
            Buscar depois de aplicar metadados (também no lote após importar)
          </label>
        </section>
    </Modal>
  )
}
