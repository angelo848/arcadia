import { useId, useState, type JSX } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { libraryKeys, useCatalog } from '../lib/library'
import { Modal } from './Modal'
import {
  addEmulator,
  deleteEmulator,
  emulatorKey,
  emulatorMarkers,
  appendMarker,
  commandSample,
  isAbsoluteWindowsPath,
  pickEmulatorExe,
  setPlatformEmulator,
  unknownMarkers,
  updateEmulator,
  useEmulators,
  type EmulatorView,
} from '../lib/emulators'

const inputClass =
  'rounded border border-line bg-raised px-3 py-1.5 text-fg focus:border-accent focus:outline-none'

type Draft = { id: number | null; name: string; exePath: string; argsTemplate: string }

const emptyDraft: Draft = { id: null, name: '', exePath: '', argsTemplate: '' }

// validateEmulator returns the first problem of an emulator draft, or '' (EMU-001).
export function validateEmulator(d: Draft): string {
  if (d.name.trim() === '') {
    return 'Informe um nome.'
  }
  if (!isAbsoluteWindowsPath(d.exePath.trim())) {
    return 'Use o caminho completo do executável (com a unidade, como C:\\Emu\\emulador.exe).'
  }
  const bad = unknownMarkers(d.argsTemplate)
  if (bad.length > 0) {
    return `Marcador desconhecido: ${bad.join(', ')}. Use ${emulatorMarkers.join(' e ')}.`
  }
  return ''
}

// EmulatorsDialog manages the emulators and the default emulator of each platform
// [EMU-001, EMU-002, EMU-006]. Deleting asks twice; the games and platforms that used an
// emulator keep their rows without it.
export function EmulatorsDialog({ onClose }: { onClose: () => void }): JSX.Element {
  const qc = useQueryClient()
  const emulators = useEmulators()
  const catalog = useCatalog()
  const [draft, setDraft] = useState<Draft | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [confirming, setConfirming] = useState<number | null>(null)

  const list: EmulatorView[] = emulators.data ?? []

  async function save() {
    if (draft === null) {
      return
    }
    const problem = validateEmulator(draft)
    if (problem !== '') {
      setError(problem)
      return
    }
    try {
      if (draft.id === null) {
        await addEmulator(draft.name.trim(), draft.exePath.trim(), draft.argsTemplate)
      } else {
        await updateEmulator(draft.id, draft.name.trim(), draft.exePath.trim(), draft.argsTemplate)
      }
      setDraft(null)
      setError(null)
      await qc.invalidateQueries({ queryKey: emulatorKey })
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
      await deleteEmulator(id)
      setConfirming(null)
      await qc.invalidateQueries({ queryKey: emulatorKey })
      await qc.invalidateQueries({ queryKey: libraryKeys.catalog })
    } catch (e) {
      setError(messageOf(e))
    }
  }

  async function setDefault(platformId: number, value: string) {
    try {
      await setPlatformEmulator(platformId, value === '' ? null : Number(value))
      await qc.invalidateQueries({ queryKey: libraryKeys.catalog })
    } catch (e) {
      setError(messageOf(e))
    }
  }

  // pickExe opens the Explorer dialog and fills the executable of the draft (EMU-006).
  async function pickExe() {
    try {
      const p = await pickEmulatorExe()
      if (p !== '') {
        setDraft((prev) => (prev === null ? prev : { ...prev, exePath: p }))
      }
    } catch (e) {
      setError(messageOf(e))
    }
  }

  return (
    <Modal
      size="medium"
      title="Emuladores"
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
          {list.map((em) => (
            <li key={em.id} className="flex items-center justify-between gap-3 rounded border border-line p-2">
              <div className="min-w-0">
                <p className="truncate text-sm text-fg">{em.name}</p>
                <p className="truncate text-xs text-muted">{em.exePath}</p>
              </div>
              <div className="flex shrink-0 gap-2">
                <button
                  type="button"
                  onClick={() => {
                    setError(null)
                    setDraft({ id: em.id, name: em.name, exePath: em.exePath, argsTemplate: em.argsTemplate })
                  }}
                  className="rounded bg-raisedpx-3 py-1 text-sm text-fg hover:bg-hover"
                >
                  Editar
                </button>
                <button
                  type="button"
                  onClick={() => void remove(em.id)}
                  className="rounded bg-raisedpx-3 py-1 text-sm text-danger hover:bg-hover"
                >
                  {confirming === em.id ? 'Confirmar exclusão' : 'Excluir'}
                </button>
              </div>
            </li>
          ))}
        </ul>

        {draft === null ? (
          <button
            type="button"
            onClick={() => {
              setError(null)
              setDraft(emptyDraft)
            }}
            className="self-start rounded bg-accent px-4 py-1.5 text-sm font-medium text-on-accent hover:bg-accent-hover"
          >
            Novo emulador
          </button>
        ) : (
          <EmulatorForm
            draft={draft}
            onChange={setDraft}
            onPick={() => void pickExe()}
            onSave={() => void save()}
            onCancel={() => setDraft(null)}
          />
        )}

        <section className="flex flex-col gap-2 border-t border-line pt-4">
          <h3 className="text-sm font-medium text-muted">Emulador padrão de cada plataforma</h3>
          {(catalog.data?.platforms ?? []).map((p) => (
            <label key={p.id} className="flex items-center justify-between gap-3 text-sm text-fg">
              {p.name}
              <select
                value={p.emulatorId === null ? '' : String(p.emulatorId)}
                onChange={(e) => void setDefault(p.id, e.target.value)}
                className={inputClass}
              >
                <option value="">Nenhum</option>
                {list.map((em) => (
                  <option key={em.id} value={String(em.id)}>
                    {em.name}
                  </option>
                ))}
              </select>
            </label>
          ))}
        </section>
    </Modal>
  )
}

function EmulatorForm({
  draft,
  onChange,
  onPick,
  onSave,
  onCancel,
}: {
  draft: Draft
  onChange: (d: Draft) => void
  onPick: () => void
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
      <div className="flex flex-col gap-1 text-sm text-muted">
        <label htmlFor={`${idPrefix}-exe`}>Caminho do aplicativo (executável do emulador)</label>
        <div className="flex gap-2">
          <input
            id={`${idPrefix}-exe`}
            className={`${inputClass} min-w-0 flex-1`}
            value={draft.exePath}
            onChange={(e) => onChange({ ...draft, exePath: e.target.value })}
          />
          <button
            type="button"
            onClick={onPick}
            className="shrink-0 rounded bg-raisedpx-3 py-1.5 text-sm text-fg hover:bg-hover"
          >
            Procurar...
          </button>
        </div>
      </div>
      <div className="flex flex-col gap-1 text-sm text-muted">
        <label htmlFor={`${idPrefix}-args`}>Parâmetros de linha de comando padrão</label>
        <input
          id={`${idPrefix}-args`}
          className={inputClass}
          value={draft.argsTemplate}
          onChange={(e) => onChange({ ...draft, argsTemplate: e.target.value })}
        />
        <div className="flex flex-wrap items-center gap-2 pt-1">
          <span className="text-xs text-muted">Adicionar:</span>
          {emulatorMarkers.map((m) => (
            <button
              key={m}
              type="button"
              onClick={() => onChange({ ...draft, argsTemplate: appendMarker(draft.argsTemplate, m) })}
              className="rounded bg-raisedpx-2 py-1 text-xs text-fg hover:bg-hover"
            >
              {m === '{rom}' ? 'Jogo ({rom})' : 'Pasta do jogo ({dir})'}
            </button>
          ))}
        </div>
      </div>
      <div className="flex flex-col gap-1 text-sm text-muted">
        <span>Amostra de comando</span>
        <code className="break-all rounded bg-raised px-3 py-1.5 text-fg">
          {commandSample(draft.exePath, draft.argsTemplate)}
        </code>
      </div>
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
