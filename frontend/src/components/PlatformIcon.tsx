import type { JSX } from 'react'
import type { PlatformView } from '../lib/library'
import { platformIconSrc } from '../lib/platforms'

// PlatformIcon shows the icon of a platform: the custom image, the bundled icon of its seed key, or the generic
// one (PLT-009..PLT-011). It is decorative: the name is always shown beside it.
export function PlatformIcon({ platform, size = 32 }: { platform: PlatformView; size?: number }): JSX.Element {
  return (
    <img
      src={platformIconSrc(platform)}
      alt=""
      width={size}
      height={size}
      className="shrink-0 rounded"
      draggable={false}
    />
  )
}
