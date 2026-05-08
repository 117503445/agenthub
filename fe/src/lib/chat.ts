import type { AgentUsage, Chat, ChatMessage, ChatTerminalIndicator, ChatTimelineRow, ChatVisualStatus, MessagePart, PlanApproval, Project, ToolCall } from '../types'

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

// messagePartsForRender 使用 message 参数返回可渲染片段。
export function messagePartsForRender(message: ChatMessage): MessagePart[] {
  return message.parts ?? []
}

// projectChatTimeline 使用 chat 和 rows 参数投影聊天正文。
export function projectChatTimeline(chat: Chat, rows: ChatTimelineRow[]): Chat {
  const messages: ChatMessage[] = []
  const indexes = new Map<string, number>()
  let plan: PlanApproval | undefined
  let usage: AgentUsage | undefined = chat.usage
  for (const row of [...rows].sort((left, right) => left.seq - right.seq)) {
    const item = row.item
    if (item.type === 'message_started' || item.type === 'system_message') {
      const message: ChatMessage = {
        id: item.messageId ?? row.id,
        chatId: row.chatId,
        role: item.type === 'system_message' ? 'system' : (item.role ?? 'assistant'),
        text: item.text ?? '',
        status: (item.status as ChatMessage['status']) ?? 'complete',
        images: item.images ?? [],
        toolCalls: [],
        parts: [],
        createdAt: row.timestamp,
        updatedAt: row.timestamp,
      }
      indexes.set(message.id, messages.length)
      messages.push(message)
      continue
    }
    if (item.type === 'assistant_delta' && item.messageId) {
      const index = indexes.get(item.messageId)
      if (index === undefined) continue
      const message = messages[index]
      const delta = item.delta ?? ''
      message.text += delta
      message.status = 'streaming'
      message.updatedAt = row.timestamp
      appendTextPart(message, delta, row)
      continue
    }
    if (item.type === 'tool_call' && item.messageId && item.toolCall) {
      const index = indexes.get(item.messageId)
      if (index === undefined) continue
      const message = messages[index]
      upsertToolCall(message, item.toolCall, row.timestamp)
      message.updatedAt = row.timestamp
      continue
    }
    if (item.type === 'usage_updated' && item.usage) {
      usage = item.usage
      continue
    }
    if (item.type === 'message_finished' && item.messageId) {
      const index = indexes.get(item.messageId)
      if (index === undefined) continue
      messages[index] = { ...messages[index], status: (item.status as ChatMessage['status']) ?? 'complete', updatedAt: row.timestamp }
      continue
    }
    if (item.type === 'plan_set' && item.plan) {
      plan = item.plan
      continue
    }
    if (item.type === 'plan_status_changed' && plan) {
      plan = { ...plan, status: (item.status as PlanApproval['status']) ?? plan.status, updatedAt: row.timestamp }
    }
  }
  return { ...chat, messages, plan, usage, timelineLoaded: true, timelineLoadedAt: chat.updatedAt }
}

// appendTextPart 使用 message、delta 和 row 参数追加文本片段。
function appendTextPart(message: ChatMessage, delta: string, row: ChatTimelineRow) {
  if (!delta) return
  const parts = message.parts ?? []
  const last = parts.at(-1)
  if (last?.type === 'text') {
    last.text = `${last.text ?? ''}${delta}`
    last.updatedAt = row.timestamp
    message.parts = parts
    return
  }
  message.parts = [
    ...parts,
    {
      id: `part-${row.id}`,
      type: 'text',
      text: delta,
      createdAt: row.timestamp,
      updatedAt: row.timestamp,
    },
  ]
}

// upsertToolCall 使用 message、tool 和 timestamp 参数归并工具调用。
function upsertToolCall(message: ChatMessage, tool: ToolCall, timestamp: string) {
  const tools = [...(message.toolCalls ?? [])]
  const index = tools.findIndex((item) => item.id === tool.id)
  const nextTool = index >= 0 ? mergeToolCall(tools[index], tool, timestamp) : { ...tool, createdAt: timestamp, updatedAt: timestamp }
  if (index >= 0) {
    tools[index] = nextTool
  } else {
    tools.push(nextTool)
  }
  message.toolCalls = tools
  const parts = [...(message.parts ?? [])]
  const partIndex = parts.findIndex((part) => part.type === 'tool_call' && part.toolCall?.id === nextTool.id)
  const part: MessagePart = {
    id: partIndex >= 0 ? parts[partIndex].id : `part-${nextTool.id}`,
    type: 'tool_call',
    toolCall: nextTool,
    createdAt: partIndex >= 0 ? parts[partIndex].createdAt : timestamp,
    updatedAt: timestamp,
  }
  if (partIndex >= 0) {
    parts[partIndex] = part
  } else {
    parts.push(part)
  }
  message.parts = parts
}

// mergeToolCall 使用 existing、incoming 和 timestamp 参数归并工具调用。
function mergeToolCall(existing: ToolCall, incoming: ToolCall, timestamp: string): ToolCall {
  const output =
    incoming.output && existing.output && incoming.status === 'running' && existing.status === 'running' && !existing.output.includes(incoming.output)
      ? `${existing.output}${incoming.output}`
      : incoming.output || existing.output
  return {
    ...existing,
    ...incoming,
    name: incoming.name || existing.name,
    input: incoming.input || existing.input,
    output,
    userInputRequest: incoming.userInputRequest ?? existing.userInputRequest,
    createdAt: existing.createdAt,
    updatedAt: timestamp,
  }
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
