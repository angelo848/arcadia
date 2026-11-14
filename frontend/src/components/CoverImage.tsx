import { useEffect, useState } from 'react'
import type { JSX } from 'react'
import placeholder from '../assets/cover-placeholder.svg'

export const coverPlaceholderUrl: string = placeholder

// CoverImage shows a cover inside a 2:3 area with object-fit contain [UIX-015]. With no
// src, or when the image fails to load, it shows the placeholder in the same box.
export function CoverImage({ src }: { src?: string }): JSX.Element {
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    setFailed(false)
  }, [src])

  const shown = src && !failed ? src : coverPlaceholderUrl

  return (
    <div className="aspect-[2/3] w-full overflow-hidden rounded bg-raised">
      <img
        src={shown}
        alt=""
        loading="lazy"
        onError={() => setFailed(true)}
        className="h-full w-full object-contain"
      />
    </div>
  )
}
