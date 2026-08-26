/**
 * Reactive clock for relative-time labels. Must be called during component
 * initialisation — the interval is owned by an `$effect` and is cleared when
 * the owning component is destroyed.
 */
export function createTicker(intervalMs = 30_000) {
  let now = $state(Date.now())

  $effect(() => {
    now = Date.now()
    const timer = setInterval(() => {
      now = Date.now()
    }, intervalMs)

    // A background tab throttles timers, so resync as soon as it is visible again.
    const onVisible = () => {
      if (!document.hidden) now = Date.now()
    }
    document.addEventListener('visibilitychange', onVisible)

    return () => {
      clearInterval(timer)
      document.removeEventListener('visibilitychange', onVisible)
    }
  })

  return {
    get current() {
      return now
    },
  }
}
