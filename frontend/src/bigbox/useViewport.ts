// Window size for the theme capacity [BBX-024, BBX-039]. The resize listener is removed on unmount.
import { useEffect, useState } from 'react'

export type Viewport = { width: number; height: number }

const read = (): Viewport => ({ width: window.innerWidth, height: window.innerHeight })

export function useViewport(): Viewport {
  const [vp, setVp] = useState<Viewport>(read)
  useEffect(() => {
    const onResize = () => {
      const next = read()
      setVp((prev) => (prev.width === next.width && prev.height === next.height ? prev : next))
    }
    window.addEventListener('resize', onResize)
    return () => window.removeEventListener('resize', onResize)
  }, [])
  return vp
}
