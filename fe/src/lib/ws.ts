export interface ClientMessage {
  /** type 表示要发送的消息类型。 */
  type: string
  /** payload 表示要发送的消息正文。 */
  payload?: string
}

export interface ServerMessage {
  /** type 表示服务端返回的消息类型。 */
  type: string
  /** payload 表示服务端返回的数据。 */
  payload?: Record<string, unknown>
  /** serverTime 表示服务端发送时间。 */
  serverTime: string
  /** version 表示服务端构建版本。 */
  version: string
}

// getWebSocketUrl 根据当前页面协议生成 WebSocket 地址。
export function getWebSocketUrl() {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  return `${protocol}//${window.location.host}/ws`
}
