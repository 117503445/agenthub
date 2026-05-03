export interface AuthStatus {
  /** tokenRequired 表示服务端是否要求前端输入 AGENTHUB_TOKEN。 */
  tokenRequired: boolean
}

export const agentHubTokenStorageKey = 'AGENTHUB_TOKEN'

// getAuthStatusUrl 根据当前页面路径生成鉴权状态接口地址，支持子路径部署。
export function getAuthStatusUrl() {
  const pageUrl = window.location.href.split('#')[0]
  return new URL('auth/status', new URL('.', pageUrl)).toString()
}

// fetchAuthStatus 从服务端读取前端鉴权状态。
export async function fetchAuthStatus() {
  const response = await fetch(getAuthStatusUrl(), {
    headers: {
      Accept: 'application/json',
    },
  })
  if (!response.ok) {
    throw new Error(`读取鉴权状态失败: ${response.status}`)
  }
  return (await response.json()) as AuthStatus
}

// readStoredAgentHubToken 从浏览器持久化状态读取 AGENTHUB_TOKEN。
export function readStoredAgentHubToken() {
  try {
    return window.localStorage.getItem(agentHubTokenStorageKey) ?? ''
  } catch {
    return ''
  }
}

// writeStoredAgentHubToken 使用 token 参数写入 AGENTHUB_TOKEN。
export function writeStoredAgentHubToken(token: string) {
  try {
    window.localStorage.setItem(agentHubTokenStorageKey, token)
  } catch {
    // localStorage 不可用时只保留内存 token。
  }
}

// clearStoredAgentHubToken 清理浏览器持久化状态中的 AGENTHUB_TOKEN。
export function clearStoredAgentHubToken() {
  try {
    window.localStorage.removeItem(agentHubTokenStorageKey)
  } catch {
    // localStorage 不可用时无需额外处理。
  }
}
