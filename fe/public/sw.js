const CACHE_NAME = 'agenthub-shell-v1'
const SHELL_URLS = ['./', './manifest.webmanifest', './agenthub-icon.svg']

self.addEventListener('install', (event) => {
  event.waitUntil(
    caches.open(CACHE_NAME).then((cache) => {
      return cache.addAll(SHELL_URLS)
    }),
  )
  self.skipWaiting()
})

self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches.keys().then((keys) => {
      return Promise.all(keys.filter((key) => key !== CACHE_NAME).map((key) => caches.delete(key)))
    }),
  )
  self.clients.claim()
})

self.addEventListener('fetch', (event) => {
  const url = new URL(event.request.url)
  if (event.request.method !== 'GET' || url.pathname.endsWith('/ws') || url.pathname.includes('/auth/') || url.pathname.includes('/filesystem/')) {
    return
  }
  event.respondWith(
    fetch(event.request)
      .then((response) => {
        if (response.ok && (event.request.mode === 'navigate' || url.pathname.includes('/assets/'))) {
          const copy = response.clone()
          caches.open(CACHE_NAME).then((cache) => cache.put(event.request, copy))
        }
        return response
      })
      .catch(() => caches.match(event.request).then((cached) => cached || caches.match('./'))),
  )
})
