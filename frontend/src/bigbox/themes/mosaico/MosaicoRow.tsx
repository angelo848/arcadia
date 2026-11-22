import { CoverArt } from '../../widgets'
import { FocusVideo } from '../../FocusVideo'
import type { BigBoxGame, BigBoxViewModel } from '../../viewModel'

type Props = {
  model: BigBoxViewModel
  onPick: (index: number) => void
  onPlay: (index: number) => void
  onVideoError: (gameId: number) => void
}

export const sessionText = { running: 'Em execução', paused: 'Pausado' } as const

const focusStyle = { transform: 'scale(1.09)', outline: '5px solid var(--arc-bb-focus)', outlineOffset: '4px' }

// Row of square covers; the focused one is larger, outlined and plays the video [MOS-004, MOS-005, MOS-007].
export function MosaicoRow({ model, onPick, onPlay, onVideoError }: Props) {
  const videoUrl = model.focused?.videoUrl ?? null
  return (
    <div className="flex items-center gap-4 overflow-hidden px-3 py-4">
      {model.games.map((g: BigBoxGame, i) => {
        const index = model.stripStart + i
        const focused = index === model.focus
        return (
          <div
            key={g.id}
            data-testid="bb-cover"
            data-focused={focused ? 'true' : 'false'}
            role="button"
            aria-label={g.title}
            onClick={() => onPick(index)}
            onDoubleClick={() => onPlay(index)}
            style={{ width: '30vh', height: '30vh', ...(focused ? focusStyle : {}) }}
            className={`relative flex shrink-0 items-center justify-center overflow-hidden rounded-xl text-center ${
              g.coverUrl ? 'bg-bb-surface' : 'bg-bb-placeholder'
            } text-bb-text ${focused ? 'z-10' : ''}`}
          >
            <CoverArt game={g} />
            {focused && videoUrl && (
              <FocusVideo
                gameId={g.id}
                url={videoUrl}
                active={model.videoVisible}
                sound={model.sound}
                failed={model.videoFailed}
                onError={onVideoError}
              />
            )}
            {g.session && (
              <span className="absolute bottom-0 left-0 right-0 bg-bb-accent px-1 text-xs text-bb-on-accent">{sessionText[g.session]}</span>
            )}
          </div>
        )
      })}
    </div>
  )
}
