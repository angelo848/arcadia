import type { CSSProperties, ReactNode } from 'react'

type Props = {
  children: ReactNode
  className?: string
  style?: CSSProperties
  as?: 'div' | 'section' | 'figcaption'
}

// Panel is the translucent plate behind every block of text of the Big Box [BBX-037, BBX-026].
export function Panel({ children, className, style, as }: Props) {
  const Tag = as ?? 'div'
  return (
    <Tag data-bb-panel="" className={`rounded-lg bg-bb-panel px-4 py-2${className ? ` ${className}` : ''}`} style={style}>
      {children}
    </Tag>
  )
}
