export interface PushSubscriptionPayload {
  endpoint: string
  keys: { p256dh: string, auth: string }
  expirationTime?: number
}

const IN_APP_CLIENT_KEY = 'aoyo.inapp.client'

export function quotaSubject(providerId: string): string {
  return `provider-quota:${providerId}`
}

function readClientId(): string {
  try {
    return localStorage.getItem(IN_APP_CLIENT_KEY) ?? ''
  } catch {
    return ''
  }
}

function writeClientId(id: string): void {
  try {
    localStorage.setItem(IN_APP_CLIENT_KEY, id)
  } catch {
    return
  }
}

export function inAppEndpoint(): string {
  let id = readClientId()
  if (!id) {
    id = crypto.randomUUID()
    writeClientId(id)
  }
  return `inapp:${id}`
}

export function inAppSubscription(): PushSubscriptionPayload {
  return { endpoint: inAppEndpoint(), keys: { p256dh: '', auth: '' } }
}

export function notificationsSupported(): boolean {
  return typeof window !== 'undefined' && 'Notification' in window
}

export async function notificationsPermitted(): Promise<boolean> {
  if (!notificationsSupported()) return false
  let permission = Notification.permission
  if (permission === 'default') permission = await Notification.requestPermission()
  return permission === 'granted'
}

export function showLocalNotification(title: string, body: string, tag: string): void {
  if (!notificationsSupported() || Notification.permission !== 'granted') return
  try {
    new Notification(title, tag ? { body, tag } : { body })
  } catch {
    return
  }
}

export function pushSupported(): boolean {
  return 'serviceWorker' in navigator && 'PushManager' in window && 'Notification' in window
}

export function urlBase64ToUint8Array(base64: string): Uint8Array {
  const padded = base64 + '='.repeat((4 - (base64.length % 4)) % 4)
  const normalized = padded.replace(/-/g, '+').replace(/_/g, '/')
  const raw = atob(normalized)
  const output = new Uint8Array(raw.length)
  for (let index = 0; index < raw.length; index += 1) {
    output[index] = raw.charCodeAt(index)
  }
  return output
}

export async function registerServiceWorker(): Promise<ServiceWorkerRegistration> {
  await navigator.serviceWorker.register('/sw.js')
  return navigator.serviceWorker.ready
}

export async function currentSubscription(): Promise<PushSubscription | null> {
  if (!pushSupported()) return null
  try {
    const registration = await navigator.serviceWorker.getRegistration()
    if (!registration) return null
    return await registration.pushManager.getSubscription()
  } catch {
    return null
  }
}

function encodeKey(buffer: ArrayBuffer | null): string {
  if (!buffer) return ''
  let binary = ''
  for (const byte of new Uint8Array(buffer)) binary += String.fromCharCode(byte)
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}

export async function ensureSubscription(vapidPublicKey: string): Promise<PushSubscription> {
  if (!pushSupported()) throw new Error('This browser does not support push notifications.')
  if (!vapidPublicKey) throw new Error('Push notifications are not configured on the server.')

  let permission = Notification.permission
  if (permission === 'default') permission = await Notification.requestPermission()
  if (permission !== 'granted') throw new Error('Notifications are blocked in your browser settings.')

  const registration = await registerServiceWorker()
  const existing = await registration.pushManager.getSubscription()
  if (existing) {
    if (encodeKey(existing.options.applicationServerKey) === vapidPublicKey.replace(/=+$/, '')) return existing
    await existing.unsubscribe()
  }

  try {
    return await registration.pushManager.subscribe({
      userVisibleOnly: true,
      applicationServerKey: urlBase64ToUint8Array(vapidPublicKey) as BufferSource,
    })
  } catch (error) {
    if (error instanceof DOMException && error.name === 'AbortError') {
      throw new Error('The browser could not reach its push service. Check that it can connect to the network, then try again.')
    }
    throw error
  }
}

export function serializeSubscription(sub: PushSubscription): PushSubscriptionPayload {
  const keys = sub.toJSON().keys ?? {}
  const payload: PushSubscriptionPayload = {
    endpoint: sub.endpoint,
    keys: { p256dh: keys.p256dh ?? '', auth: keys.auth ?? '' },
  }
  if (typeof sub.expirationTime === 'number' && Number.isFinite(sub.expirationTime)) {
    payload.expirationTime = sub.expirationTime
  }
  return payload
}
