import { useId, useState, type JSX } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { libraryGamesKey, libraryKeys, useAddPlatform, useCatalog, type PlatformView } from '../lib/library'
import { setPlatformEmulator, useEmulators } from '../lib/emulators'
import { deletePlatform, pickPlatformIcon, setPlatformIcon, updatePlatform } from '../lib/platforms'
import { Modal } from './Modal'
import { PlatformIcon } from './PlatformIcon'

const inputClass =
  'rounded border border-line bg-raised px-3 py-1.5 text-fg focus:border-accent'

// The categories of a platform (DOM-015, PLT-019) and their labels.
const categories: ReadonlyArray<{ value: string; label: string }> = [
  { value: 'pc', label: 'PC' },
  { value: 'console', label: 'Console' },
  { value: 'arcade', label: 'Arcade' },
  { value: 'handheld', label: 'Portátil' },
]

function categoryLabel(value: string): string {
  return categories.find((c) => c.value === value)?.label ?? value
}

// The text of a platform that still has games and so cannot be removed (PLT-021).
function blockedText(n: number): string {
  return `Esta plataforma tem ${n} jogo(s). Mova-os para outra plataforma ou exclua-os antes.`
}

// id is null for a new platform; iconPath is the path typed or picked for a new icon ('' keeps the current one).
type Draft = { id: number | null; name: string; category: string; iconPath: string }

const newDraft: Draft = { id: null, name: '', category: 'console', iconPath: '' }

// PlatformsDialog manages the platforms: list with icon, games and default emulator, new, edit (name, category,
// icon) and delete [PLT-015..PLT-023]. Every write goes through the agent and then reloads the catalog (PLT-024).
export function PlatformsDialog({ onClose }: { onClose: () => void }): JSX.Element {
  const qc = useQueryClient()
  const catalog = useCatalog()
  const emulators = useEmulators()
  const add = useAddPlatform()
  const [draft, setDraft] = useState<Draft | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [confirming, setConfirming] = useState<number | null>(null)

  const list: PlatformView[] = catalog.data?.platforms ?? []
  const emulatorList = emulators.data ?? []

  const reloadCatalog = () => qc.invalidateQueries({ queryKey: libraryKeys.catalog })

  async function save() {
    if (draft === null) {
      return
    }
    const name = draft.name.trim()
    if (name === '') {
      setError('Informe um nome.')
      return
    }
    setError(null)
    try {
      if (draft.id === null) {
        const res = await add.mutateAsync({ name, category: draft.category })
        if (!res.created) {
          setError(`Já existe: ${res.platform.name}.`)
          return
        }
        setDraft(null)
        return
      }
      await saveEdit(draft.id, name, draft)
      setDraft(null)
    } catch (e) {
      setError(messageOf(e))
    }
  }

  // saveEdit writes the name and category when they changed, then the icon when a path was given.
  // The catalog is reloaded even when the second call fails, because the first one went through.
  async function saveEdit(id: number, name: string, d: Draft) {
    const current = list.find((p) => p.id === id)
    const renamed = current === undefined || current.name !== name
    try {
      if (renamed || current.category !== d.category) {
        await updatePlatform(id, name, d.category)
      }
      if (d.iconPath.trim() !== '') {
        await setPlatformIcon(id, d.iconPath.trim())
      }
    } finally {
      await reloadCatalog()
      if (renamed) {
        // The platform name shows in the game cards and pages.
        await qc.invalidateQueries({ queryKey: libraryGamesKey })
      }
    }
  }

  async function restoreIcon(id: number) {
    try {
      await setPlatformIcon(id, null)
      setDraft((prev) => (prev === null ? prev : { ...prev, iconPath: '' }))
      setError(null)
      await reloadCatalog()
    } catch (e) {
      setError(messageOf(e))
    }
  }

  async function remove(id: number) {
    if (confirming !== id) {
      setConfirming(id)
      return
    }
    try {
      await deletePlatform(id)
      setConfirming(null)
      await reloadCatalog()
    } catch (e) {
      setConfirming(null)
      setError(messageOf(e))
    }
  }

  async function setDefault(platformId: number, value: string) {
    try {
      await setPlatformEmulator(platformId, value === '' ? null : Number(value))
      await reloadCatalog()
    } catch (e) {
      setError(messageOf(e))
    }
  }

  // pickIcon opens the Explorer dialog and fills the icon path of the draft (PLT-023).
  async function pickIcon() {
    try {
      const p = await pickPlatformIcon()
      if (p !== '') {
        setDraft((prev) => (prev === null ? prev : { ...prev, iconPath: p }))
      }
    } catch (e) {
      setError(messageOf(e))
    }
  }

  return (
    <Modal
      size="medium"
      title="Plataformas"
      onClose={onClose}
      footer={
        <button type="button" onClick={onClose} className="rounded px-3 py-1.5 text-sm text-muted hover:bg-hover">
          Fechar
        </button>
      }
    >

        {error !== null && (
          <p role="alert" className="text-sm text-danger">
            {error}
          </p>
        )}

        <ul className="flex flex-col gap-2">
          {list.map((p) => (
            <li key={p.id} className="flex flex-col gap-2 rounded border border-line p-2">
              <div className="flex items-center justify-between gap-3">
                <div className="flex min-w-0 items-center gap-3">
                  <PlatformIcon platform={p} size={32} />
                  <div className="min-w-0">
                    <p className="truncate text-sm text-fg">{p.name}</p>
                    <p className="truncate text-xs text-muted">
                      <span>{categoryLabel(p.category)}</span>
                      {' · '}
                      <span>{`${p.gameCount} jogo(s)`}</span>
                    </p>
                  </div>
                </div>
                <div className="flex shrink-0 items-center gap-2">
                  <select
                    aria-label={`Emulador padrão de ${p.name}`}
                    value={p.emulatorId == null ? '' : String(p.emulatorId)}
                    onChange={(e) => void setDefault(p.id, e.target.value)}
                    className={`${inputClass} py-1 text-sm`}
                  >
                    <option value="">Nenhum</option>
                    {emulatorList.map((em) => (
                      <option key={em.id} value={String(em.id)}>
                        {em.name}
                      </option>
                    ))}
                  </select>
                  <button
                    type="button"
                    onClick={() => {
                      setError(null)
                      setDraft({ id: p.id, name: p.name, category: p.category, iconPath: '' })
                    }}
                    className="rounded bg-raisedpx-3 py-1 text-sm text-fg hover:bg-hover"
                  >
                    Editar
                  </button>
                  <button
                    type="button"
                    disabled={p.gameCount > 0}
                    onClick={() => void remove(p.id)}
                    className="rounded bg-raisedpx-3 py-1 text-sm text-danger hover:bg-hover disabled:cursor-not-allowed disabled:opacity-50"
                  >
                    {confirming === p.id ? 'Confirmar exclusão' : 'Excluir'}
                  </button>
                </div>
              </div>
              {p.gameCount > 0 && <p className="text-xs text-muted">{blockedText(p.gameCount)}</p>}
            </li>
          ))}
        </ul>

        {draft === null ? (
          <button
            type="button"
            onClick={() => {
              setError(null)
              setDraft(newDraft)
            }}
            className="self-start rounded bg-accent px-4 py-1.5 text-sm font-medium text-on-accent hover:bg-accent-hover"
          >
            Nova plataforma
          </button>
        ) : (
          <PlatformForm
            draft={draft}
            onChange={setDraft}
            onPick={() => void pickIcon()}
            onRestore={() => draft.id !== null && void restoreIcon(draft.id)}
            onSave={() => void save()}
            onCancel={() => {
              setDraft(null)
              setError(null)
            }}
          />
        )}
    </Modal>
  )
}

function PlatformForm({
  draft,
  onChange,
  onPick,
  onRestore,
  onSave,
  onCancel,
}: {
  draft: Draft
  onChange: (d: Draft) => void
  onPick: () => void
  onRestore: () => void
  onSave: () => void
  onCancel: () => void
}): JSX.Element {
  const idPrefix = useId()
  return (
    <form
      className="flex flex-col gap-3 rounded border border-line p-3"
      onSubmit={(e) => {
        e.preventDefault()
        onSave()
      }}
    >
      <label className="flex flex-col gap-1 text-sm text-muted">
        Nome
        <input className={inputClass} value={draft.name} onChange={(e) => onChange({ ...draft, name: e.target.value })} />
      </label>
      <label className="flex flex-col gap-1 text-sm text-muted">
        Categoria
        <select
          className={inputClass}
          value={draft.category}
          onChange={(e) => onChange({ ...draft, category: e.target.value })}
        >
          {categories.map((c) => (
            <option key={c.value} value={c.value}>
              {c.label}
            </option>
          ))}
        </select>
      </label>
      {draft.id !== null && (
        <div className="flex flex-col gap-1 text-sm text-muted">
          <label htmlFor={`${idPrefix}-icon`}>Caminho do ícone</label>
          <div className="flex gap-2">
            <input
              id={`${idPrefix}-icon`}
              className={`${inputClass} min-w-0 flex-1`}
              value={draft.iconPath}
              onChange={(e) => onChange({ ...draft, iconPath: e.target.value })}
            />
            <button
              type="button"
              onClick={onPick}
              className="shrink-0 rounded bg-raisedpx-3 py-1.5 text-sm text-fg hover:bg-hover"
            >
              Procurar...
            </button>
            <button
              type="button"
              onClick={onRestore}
              className="shrink-0 rounded bg-raisedpx-3 py-1.5 text-sm text-fg hover:bg-hover"
            >
              Restaurar padrão
            </button>
          </div>
          <p className="text-xs text-muted">Imagem PNG ou JPEG de até 2 MB. Vazio mantém o ícone atual.</p>
        </div>
      )}
      <div className="flex justify-end gap-2">
        <button type="button" onClick={onCancel} className="rounded px-4 py-1.5 text-sm text-muted hover:bg-hover">
          Cancelar
        </button>
        <button type="submit" className="rounded bg-accent px-4 py-1.5 text-sm font-medium text-on-accent hover:bg-accent-hover">
          Salvar
        </button>
      </div>
    </form>
  )
}

function messageOf(e: unknown): string {
  return e instanceof Error ? e.message : 'Não foi possível salvar. Tente de novo.'
}
