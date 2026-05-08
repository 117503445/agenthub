export interface ClientMessage<TPayload = unknown> {
  /** type 表示客户端消息类型。 */
  type: string
  /** payload 表示客户端消息参数。 */
  payload?: TPayload
}

export interface ServerMessage<TPayload = unknown> {
  /** type 表示服务端消息类型。 */
  type: string
  /** payload 表示服务端消息数据。 */
  payload?: TPayload
  /** serverTime 表示服务端发送时间。 */
  serverTime: string
  /** version 表示服务端构建版本。 */
  version: string
  /** buildTime 表示后端构建时间。 */
  buildTime: string
  /** hostname 表示后端机器名。 */
  hostname?: string
}

// getWebSocketUrl 根据当前页面路径和 token 参数生成 WebSocket 地址，支持子路径部署。
export function getWebSocketUrl(token = '') {
  const pageUrl = window.location.href.split('#')[0]
  const endpoint = new URL('ws', new URL('.', pageUrl))
  endpoint.protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  if (token) {
    endpoint.searchParams.set('token', token)
  }
  return endpoint.toString()
}

// sendClientMessage 使用 ws、type 和 payload 参数发送结构化 WebSocket 消息。
export function sendClientMessage<TPayload>(ws: WebSocket | null, type: string, payload?: TPayload) {
  if (!ws || ws.readyState !== WebSocket.OPEN) {
    return false
  }
  const message: ClientMessage<TPayload> = payload === undefined ? { type } : { type, payload }
  ws.send(JSON.stringify(message))
  return true
}
