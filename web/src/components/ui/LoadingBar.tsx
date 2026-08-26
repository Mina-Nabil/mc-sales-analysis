import { useEffect, useRef, useState } from 'react'
import { onLoadingChange } from '@/lib/api'

// Global top progress bar. Subscribes to the API client's in-flight request
// counter and shows an NProgress-style bar whenever any request is pending — so
// every page load and action gives the user immediate "still working" feedback.
// A short show-delay keeps it from flickering on very fast responses.
export function LoadingBar() {
  const [progress, setProgress] = useState(0)
  const [visible, setVisible] = useState(false)
  const showing = useRef(false)
  const pending = useRef<number | undefined>(undefined) // show-delay timer
  const trickle = useRef<number | undefined>(undefined)
  const hide = useRef<number | undefined>(undefined)

  useEffect(() => {
    const startTrickle = () => {
      if (trickle.current) return
      trickle.current = window.setInterval(() => {
        setProgress((p) => (p >= 92 ? p : p + Math.max(0.4, (92 - p) * 0.06)))
      }, 220)
    }
    const stopTrickle = () => {
      if (trickle.current) { window.clearInterval(trickle.current); trickle.current = undefined }
    }

    const start = () => {
      if (hide.current) { window.clearTimeout(hide.current); hide.current = undefined }
      if (pending.current) return
      if (showing.current) { setProgress((p) => (p >= 92 ? 85 : p)); startTrickle(); return }
      pending.current = window.setTimeout(() => {
        pending.current = undefined
        showing.current = true
        setVisible(true)
        setProgress(8)
        startTrickle()
      }, 120)
    }

    const finish = () => {
      if (pending.current) { window.clearTimeout(pending.current); pending.current = undefined }
      if (!showing.current) return
      stopTrickle()
      setProgress(100)
      hide.current = window.setTimeout(() => {
        showing.current = false
        setVisible(false)
        setProgress(0)
        hide.current = undefined
      }, 320)
    }

    const off = onLoadingChange((n) => { if (n > 0) start(); else finish() })
    return () => {
      off()
      if (pending.current) window.clearTimeout(pending.current)
      if (hide.current) window.clearTimeout(hide.current)
      stopTrickle()
    }
  }, [])

  return (
    <div aria-hidden="true" role="progressbar" className="pointer-events-none fixed inset-x-0 top-0 z-[200] h-[3px]">
      <div
        className="h-full bg-acc shadow-[0_0_10px_var(--acc)] transition-[width,opacity] duration-200 ease-out"
        style={{ width: `${progress}%`, opacity: visible ? 1 : 0 }}
      />
    </div>
  )
}
