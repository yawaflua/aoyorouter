self.addEventListener('install', () => {
  self.skipWaiting()
})

self.addEventListener('activate', (event) => {
  event.waitUntil(self.clients.claim())
})

self.addEventListener('push', (event) => {
  let title = 'Aoyo Router'
  let body = 'Provider quota updated.'
  let tag = ''
  let url = '/'

  try {
    const payload = event.data ? event.data.json() : {}
    if (payload && typeof payload === 'object') {
      if (typeof payload.title === 'string' && payload.title) title = payload.title
      if (typeof payload.body === 'string' && payload.body) body = payload.body
      if (typeof payload.tag === 'string') tag = payload.tag
      if (typeof payload.url === 'string' && payload.url) url = payload.url
    }
  } catch {
    title = 'Aoyo Router'
    body = 'Provider quota updated.'
  }

  event.waitUntil(
    self.registration.showNotification(title, {
      body,
      tag,
      data: { url },
      icon: '/favicon.svg',
      badge: '/favicon.svg',
      renotify: Boolean(tag),
    }),
  )
})

self.addEventListener('notificationclick', (event) => {
  event.notification.close()
  const data = event.notification.data || {}
  const url = typeof data.url === 'string' && data.url ? data.url : '/'
  const target = new URL(url, self.location.origin)

  event.waitUntil(
    (async () => {
      const windows = await self.clients.matchAll({ type: 'window', includeUncontrolled: true })
      for (const client of windows) {
        if (new URL(client.url).origin === target.origin && 'focus' in client) return client.focus()
      }
      return self.clients.openWindow(target.href)
    })(),
  )
})
