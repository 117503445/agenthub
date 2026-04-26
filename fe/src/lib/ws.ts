export interface ClientMessage {
  type: string
  payload?: string
}

export interface ServerMessage {
  type: string
  payload?: Record<string, unknown>
  serverTime: string
  version: string
}

export function getWebSocketUrl() {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  return `${protocol}//${window.location.host}/ws`
}
