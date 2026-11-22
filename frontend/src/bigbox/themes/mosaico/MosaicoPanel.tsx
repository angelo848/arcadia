import { Panel } from '../../Panel'
import { TimeToBeatList } from '../../widgets'
import type { BigBoxFocused } from '../../viewModel'

function Pair({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex flex-col">
      <span className="text-sm text-bb-text-muted">{label}</span>
      <span className="text-lg font-semibold text-bb-text">{value}</span>
    </div>
  )
}

// Information panel under the row: info, times and screenshots [MOS-008..MOS-011].
export function MosaicoPanel({ focused }: { focused: BigBoxFocused }) {
  const meta = [focused.platform, focused.year ? String(focused.year) : '', focused.genre ?? ''].filter((s) => s).join(' · ')
  const shots = focused.screenshots.slice(0, 3)
  return (
    <Panel className="flex min-h-0 flex-1 gap-8 overflow-hidden !px-6 !py-4 text-bb-text" >
      <div data-testid="mos-panel" className="contents">
        <div data-testid="mos-col-info" className="flex min-w-0 flex-1 flex-col gap-2">
          {meta && <p className="text-bb-text-muted">{meta}</p>}
          {focused.shortDescription && (
            <p data-testid="mos-description" className="line-clamp-4 text-bb-text">
              {focused.shortDescription}
            </p>
          )}
        </div>
        <div data-testid="mos-col-times" className="flex min-w-0 flex-1 flex-col gap-2">
          <Pair label="Tempo jogado" value={focused.playTimeLabel} />
          <Pair label="Última vez" value={focused.lastPlayedLabel ?? 'Nunca jogado'} />
          <Pair label="Sessões" value={String(focused.playCount)} />
          <TimeToBeatList rows={focused.timeToBeat} source={focused.timeToBeatSource} />
        </div>
        {shots.length > 0 && (
          <div data-testid="mos-col-shots" className="flex min-w-0 flex-1 gap-2">
            {shots.map((s) => (
              <img key={s} src={s} alt="" className="h-full min-w-0 flex-1 rounded object-contain" />
            ))}
          </div>
        )}
      </div>
    </Panel>
  )
}
