import type { Chat, ChatMessage, ChatTerminalIndicator, ChatVisualStatus, MessagePart, Project, ToolCall } from '../types'

// formatTime 使用 value 参数格式化界面时间。
export function formatTime(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return ''
  }
  return date.toLocaleTimeString('zh-CN', { hour12: false, hour: '2-digit', minute: '2-digit', second: '2-digit' })
}

// projectDisplayName 使用 project 参数返回侧边栏展示名称。
export function projectDisplayName(project: Project) {
  const normalized = project.path.replace(/\\/g, '/').replace(/\/+$/, '')
  return normalized.split('/').filter(Boolean).pop() || project.name || project.path
}

// projectGitText 使用 project 参数返回顶部 Git 信息文案。
export function projectGitText(project: Project | null) {
  if (!project) {
    return 'git: -'
  }
  if (!project.git?.isRepo) {
    return 'git: none'
  }
  const branch = project.git.branch && project.git.branch !== 'HEAD' ? project.git.branch : 'detached'
  const state = project.git.dirty ? 'dirty' : 'clean'
  return `git: ${branch} · ${state}`
}

// chatVisualStatus 使用 chat 和 indicators 参数返回当前应该展示的状态图标。
export function chatVisualStatus(chat: Chat, indicators: Record<string, ChatTerminalIndicator>): ChatVisualStatus | null {
  const terminalStatus = indicators[chat.id]
  if (terminalStatus) {
    return terminalStatus
  }
  if (chat.status === 'running') {
    return 'running'
  }
  return null
}

// mergeProjectVisualStatus 使用 current 和 next 参数按优先级归并 project 状态。
export function mergeProjectVisualStatus(current: ChatVisualStatus | null, next: ChatVisualStatus | null) {
  if (!next) {
    return current
  }
  if (next === 'error' || current === 'error') {
    return 'error'
  }
  if (next === 'running' || current === 'running') {
    return 'running'
  }
  return 'success'
}

// compactToolInput 使用 input 参数生成工具调用标题中的命令摘要。
function compactToolInput(input = '') {
  const trimmed = input.trim()
  if (!trimmed) {
    return ''
  }
  try {
    const parsed = JSON.parse(trimmed) as Record<string, unknown>
    const command = parsed.command ?? parsed.cmd ?? parsed.url ?? parsed.file_path
    if (typeof command === 'string' && command.trim()) {
      return command.trim()
    }
  } catch {
    return trimmed
  }
  return trimmed
}

// toolCommandTitle 使用 tool 参数返回折叠状态可见的工具调用标题。
export function toolCommandTitle(tool: ToolCall) {
  if (tool.name === 'thinking') {
    return 'Reasoning'
  }
  const command = compactToolInput(tool.input)
  if (!command) {
    return tool.name
  }
  if (tool.name === 'exec_command' || tool.name === 'command_execution') {
    return command
  }
  return `${tool.name}: ${command}`
}

// messagePartsForRender 使用 message 参数返回兼容旧数据的渲染片段。
export function messagePartsForRender(message: ChatMessage): MessagePart[] {
  if (message.parts?.length) {
    return message.parts
  }
  const parts: MessagePart[] = []
  for (const toolCall of message.toolCalls ?? []) {
    parts.push({
      id: `legacy-tool-${toolCall.id}`,
      type: 'tool_call',
      toolCall,
      createdAt: toolCall.createdAt,
      updatedAt: toolCall.updatedAt,
    })
  }
  if (message.text || message.status === 'streaming') {
    parts.push({
      id: `legacy-text-${message.id}`,
      type: 'text',
      text: message.text || (message.status === 'streaming' ? '正在等待输出...' : ''),
      createdAt: message.createdAt,
      updatedAt: message.updatedAt,
    })
  }
  return parts
}

// upsertById 使用 list、item 和 getId 参数插入或更新列表项。
export function upsertById<TItem>(list: TItem[], item: TItem, getId: (value: TItem) => string) {
  const id = getId(item)
  const index = list.findIndex((value) => getId(value) === id)
  if (index === -1) {
    return [...list, item]
  }
  const next = [...list]
  next[index] = item
  return next
}

// sortByCreatedAt 使用 list 参数按创建时间稳定排序。
export function sortByCreatedAt<TItem extends { createdAt: string; id: string }>(list: TItem[]) {
  return [...list].sort((left, right) => {
    const leftTime = new Date(left.createdAt).getTime()
    const rightTime = new Date(right.createdAt).getTime()
    if (leftTime === rightTime) {
      return left.id.localeCompare(right.id)
    }
    return leftTime - rightTime
  })
}

// sortProjects 使用 projects 参数按侧栏顺序稳定排序。
export function sortProjects(projects: Project[]) {
  return [...projects].sort((left, right) => {
    if (left.sortOrder !== right.sortOrder) {
      return left.sortOrder - right.sortOrder
    }
    const leftTime = new Date(left.createdAt).getTime()
    const rightTime = new Date(right.createdAt).getTime()
    if (leftTime !== rightTime) {
      return rightTime - leftTime
    }
    return left.id.localeCompare(right.id)
  })
}
