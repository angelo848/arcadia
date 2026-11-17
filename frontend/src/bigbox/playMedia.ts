// playWithSound starts a video with or without sound. When the browser refuses to play with sound (no user
// activation yet), the video plays muted and gets its sound back on the next key press or click.
// It returns the cleanup that drops the pending listeners.
export function playWithSound(v: HTMLVideoElement, sound: boolean): () => void {
  v.muted = !sound
  const attempt = v.play()
  if (!sound || attempt === undefined) {
    void attempt?.catch(() => {})
    return () => {}
  }

  let off = () => {}
  void attempt.catch(() => {
    v.muted = true
    void v.play()?.catch(() => {})
    const retry = () => {
      off()
      v.muted = false
      void v.play()?.catch(() => {
        v.muted = true
        void v.play()?.catch(() => {})
      })
    }
    window.addEventListener('keydown', retry, { once: true })
    window.addEventListener('pointerdown', retry, { once: true })
    off = () => {
      window.removeEventListener('keydown', retry)
      window.removeEventListener('pointerdown', retry)
    }
  })
  return () => off()
}
