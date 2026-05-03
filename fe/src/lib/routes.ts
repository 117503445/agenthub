import type { HashRoute } from '../types'

// safeDecodeRoutePart 使用 value 参数安全解码 hash 路由片段。
function safeDecodeRoutePart(value: string) {
  try {
    return decodeURIComponent(value)
  } catch {
    return ''
  }
}

// parseHashRoute 使用 hash 参数解析当前 hash 路由。
export function parseHashRoute(hash = window.location.hash): HashRoute {
  const parts = hash.replace(/^#\/?/, '').split('/').filter(Boolean)
  if (parts[0] === 'settings') {
    return { view: 'settings', projectId: '', chatId: '' }
  }
  const projectIndex = parts.indexOf('projects')
  if (projectIndex === -1) {
    return { view: 'chat', projectId: '', chatId: '' }
  }
  const projectId = safeDecodeRoutePart(parts[projectIndex + 1] ?? '')
  const chatId = parts[projectIndex + 2] === 'chats' ? safeDecodeRoutePart(parts[projectIndex + 3] ?? '') : ''
  return { view: 'chat', projectId, chatId }
}

// buildHashRoute 使用 projectId 和 chatId 参数构造 hash 路由。
export function buildHashRoute(projectId: string, chatId: string) {
  if (!projectId) {
    return '#/'
  }
  const projectRoute = `#/projects/${encodeURIComponent(projectId)}`
  if (!chatId) {
    return projectRoute
  }
  return `${projectRoute}/chats/${encodeURIComponent(chatId)}`
}

// updateHashRoute 使用 projectId、chatId 和 mode 参数更新浏览器 hash 路由。
export function updateHashRoute(projectId: string, chatId: string, mode: 'push' | 'replace' = 'replace') {
  const nextHash = buildHashRoute(projectId, chatId)
  if (window.location.hash === nextHash) {
    return
  }
  if (mode === 'push') {
    window.history.pushState(null, '', nextHash)
    return
  }
  window.history.replaceState(null, '', nextHash)
}

// updateSettingsHashRoute 使用 mode 参数切换到设置页 hash 路由。
export function updateSettingsHashRoute(mode: 'push' | 'replace' = 'push') {
  if (window.location.hash === '#/settings') {
    return
  }
  if (mode === 'push') {
    window.history.pushState(null, '', '#/settings')
    return
  }
  window.history.replaceState(null, '', '#/settings')
}
