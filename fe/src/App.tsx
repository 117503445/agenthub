import { FormEvent, KeyboardEvent, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  ArrowUp,
  Bot,
  Brain,
  Check,
  Copy,
  Folder,
  GitBranch,
  GitCommit,
  Loader2,
  MessageSquare,
  Monitor,
  Plus,
  Settings,
  Square,
  Trash2,
  Wrench,
  Wifi,
  WifiOff,
  X,
} from 'lucide-react'
import { getWebSocketUrl, sendClientMessage, type ServerMessage } from './lib/ws'
import { Button } from './components/ui/button'
import { Input } from './components/ui/input'
import { Select } from './components/ui/select'
import { Tabs, TabsContent, TabsList, TabsTrigger } from './components/ui/tabs'
import { Textarea } from './components/ui/textarea'
import { MarkdownRenderer } from './components/MarkdownRenderer'

type ConnectionState = 'connecting' | 'open' | 'closed' | 'error'
type ChatStatus = 'idle' | 'running' | 'error'
type ChatTerminalIndicator = 'success' | 'error'
type ChatVisualStatus = 'running' | ChatTerminalIndicator
type MessageRole = 'user' | 'assistant' | 'system'
type MessageStatus = 'complete' | 'streaming' | 'stopped' | 'error'
type AgentProvider = 'claude-code' | 'codex' | 'mock-claude-code' | 'mock-codex'

interface AgentModelOption {
  /** id 表示传递给 agent CLI 的模型标识。 */
  id: string
  /** label 表示界面展示名称。 */
  label: string
  /** default 表示是否为 provider 默认模型。 */
  default?: boolean
  /** reasoningLevels 表示模型支持的推理级别。 */
  reasoningLevels?: AgentReasoningOption[]
}

interface AgentReasoningOption {
  /** id 表示传递给 agent CLI 的推理级别标识。 */
  id: string
  /** label 表示界面展示名称。 */
  label: string
  /** description 表示推理级别说明。 */
  description: string
  /** default 表示是否为默认推理级别。 */
  default?: boolean
}

interface AgentProviderOption {
  /** id 表示 provider 标识。 */
  id: AgentProvider
  /** label 表示界面展示名称。 */
  label: string
  /** models 表示 provider 可选模型。 */
  models: AgentModelOption[]
}

interface Project {
  /** id 表示 project 唯一标识。 */
  id: string
  /** name 表示 project 展示名称。 */
  name: string
  /** path 表示后端本机工作目录。 */
  path: string
  /** git 表示 project 当前 Git 摘要。 */
  git?: ProjectGitInfo
  /** createdAt 表示创建时间。 */
  createdAt: string
  /** updatedAt 表示更新时间。 */
  updatedAt: string
}

interface ProjectGitInfo {
  /** isRepo 表示当前目录是否位于 Git 仓库中。 */
  isRepo: boolean
  /** branch 表示当前分支或 HEAD 状态。 */
  branch: string
  /** commit 表示当前短提交哈希。 */
  commit: string
  /** dirty 表示工作区是否有未提交内容。 */
  dirty: boolean
}

interface ToolCall {
  /** id 表示工具调用唯一标识。 */
  id: string
  /** name 表示工具名称。 */
  name: string
  /** status 表示工具调用状态。 */
  status: 'running' | 'complete' | 'error'
  /** input 表示工具入参摘要。 */
  input?: string
  /** output 表示工具输出摘要。 */
  output?: string
  /** createdAt 表示创建时间。 */
  createdAt: string
  /** updatedAt 表示更新时间。 */
  updatedAt: string
}

interface MessagePart {
  /** id 表示片段唯一标识。 */
  id: string
  /** type 表示片段类型。 */
  type: 'text' | 'tool_call'
  /** text 表示文本片段内容。 */
  text?: string
  /** toolCall 表示工具调用片段。 */
  toolCall?: ToolCall
  /** createdAt 表示创建时间。 */
  createdAt: string
  /** updatedAt 表示更新时间。 */
  updatedAt: string
}

interface ChatMessage {
  /** id 表示消息唯一标识。 */
  id: string
  /** chatId 表示消息所属聊天页。 */
  chatId: string
  /** role 表示消息角色。 */
  role: MessageRole
  /** text 表示消息文本。 */
  text: string
  /** status 表示消息状态。 */
  status: MessageStatus
  /** toolCalls 表示消息中的工具调用。 */
  toolCalls?: ToolCall[]
  /** parts 表示 assistant 内容与工具调用的顺序片段。 */
  parts?: MessagePart[]
  /** createdAt 表示创建时间。 */
  createdAt: string
  /** updatedAt 表示更新时间。 */
  updatedAt: string
}

interface Chat {
  /** id 表示聊天页唯一标识。 */
  id: string
  /** projectId 表示所属 project。 */
  projectId: string
  /** title 表示聊天页标题。 */
  title: string
  /** status 表示聊天页运行状态。 */
  status: ChatStatus
  /** agentProvider 表示聊天页使用的 agent 类型。 */
  agentProvider: AgentProvider
  /** agentModel 表示聊天页使用的模型。 */
  agentModel: string
  /** agentReasoning 表示聊天页使用的推理级别。 */
  agentReasoning?: string
  /** agentLocked 表示会话开始后 agent 配置是否锁定。 */
  agentLocked: boolean
  /** agentSessionId 表示 agent 会话标识。 */
  agentSessionId?: string
  /** messages 表示消息列表。 */
  messages: ChatMessage[]
  /** createdAt 表示创建时间。 */
  createdAt: string
  /** updatedAt 表示更新时间。 */
  updatedAt: string
}

interface SnapshotPayload {
  /** projects 表示所有 project。 */
  projects: Project[]
  /** chats 表示所有聊天页。 */
  chats: Chat[]
  /** agentProviders 表示可选 agent 和模型。 */
  agentProviders?: AgentProviderOption[]
}

interface ProjectChangedPayload {
  /** project 表示变更后的 project。 */
  project: Project
}

interface ProjectDeletedPayload {
  /** id 表示被删除的 project 标识。 */
  id: string
  /** chatIds 表示被删除的聊天页标识。 */
  chatIds: string[]
}

interface ChatChangedPayload {
  /** chat 表示变更后的聊天页。 */
  chat: Chat
}

interface ChatDeletedPayload {
  /** id 表示被删除的聊天页标识。 */
  id: string
  /** projectId 表示被删除聊天页所属 project。 */
  projectId: string
}

interface ChatMessageDeltaPayload {
  /** chatId 表示聊天页标识。 */
  chatId: string
  /** messageId 表示消息标识。 */
  messageId: string
  /** delta 表示增量文本。 */
  delta: string
  /** text 表示服务端当前完整文本。 */
  text: string
  /** message 表示服务端当前完整消息。 */
  message?: ChatMessage
}

interface ChatMessageDonePayload {
  /** chatId 表示聊天页标识。 */
  chatId: string
  /** message 表示完成后的消息。 */
  message: ChatMessage
}

interface AgentStatusPayload {
  /** chatId 表示聊天页标识。 */
  chatId: string
  /** status 表示聊天页运行状态。 */
  status: ChatStatus
}

interface AgentProvidersChangedPayload {
  /** agentProviders 表示更新后的 agent 和模型选项。 */
  agentProviders: AgentProviderOption[]
}

interface HashRoute {
  /** view 表示当前 hash 路由视图。 */
  view: 'chat' | 'settings'
  /** projectId 表示 hash 路由中的 project 标识。 */
  projectId: string
  /** chatId 表示 hash 路由中的聊天页标识。 */
  chatId: string
}

const connectionText: Record<ConnectionState, string> = {
  connecting: '连接中',
  open: '已连接',
  closed: '已断开',
  error: '连接异常',
}

const chatStatusLabel: Record<ChatVisualStatus, string> = {
  running: '工作中',
  success: '已完成',
  error: '失败',
}

const chatStatusClass: Record<ChatVisualStatus, string> = {
  running: 'bg-amber-400',
  success: 'bg-emerald-500',
  error: 'bg-red-500',
}

const fallbackAgentProviders: AgentProviderOption[] = [
  {
    id: 'claude-code',
    label: 'Claude Code',
    models: [
      { id: 'sonnet', label: 'Sonnet', default: true },
      { id: 'opus', label: 'Opus' },
      { id: 'haiku', label: 'Haiku' },
    ],
  },
  {
    id: 'codex',
    label: 'Codex',
    models: [
      {
        id: 'gpt-5.5',
        label: 'GPT-5.5',
        default: true,
        reasoningLevels: [
          { id: 'low', label: 'Low', description: '快速响应，使用较轻推理。' },
          { id: 'medium', label: 'Medium', description: '默认级别，平衡速度和推理深度。' },
          { id: 'high', label: 'High', description: '更深入推理，适合复杂问题。' },
          { id: 'xhigh', label: 'Extra high', description: '最深推理，适合复杂实现和排障。', default: true },
        ],
      },
      { id: 'gpt-5.4-mini', label: 'GPT-5.4 Mini' },
      { id: 'gpt-5.4', label: 'GPT-5.4' },
      { id: 'gpt-5.3-codex', label: 'GPT-5.3 Codex' },
    ],
  },
  {
    id: 'mock-claude-code',
    label: 'Mock Claude Code',
    models: [
      { id: 'mock-claude-sonnet', label: 'Mock Claude Sonnet', default: true },
      { id: 'mock-claude-opus', label: 'Mock Claude Opus' },
    ],
  },
  {
    id: 'mock-codex',
    label: 'Mock Codex',
    models: [
      {
        id: 'mock-codex-gpt-5.5',
        label: 'Mock Codex GPT-5.5',
        default: true,
        reasoningLevels: [
          { id: 'low', label: 'Low', description: '快速响应，使用较轻推理。' },
          { id: 'medium', label: 'Medium', description: '默认级别，平衡速度和推理深度。' },
          { id: 'high', label: 'High', description: '更深入推理，适合复杂问题。' },
          { id: 'xhigh', label: 'Extra high', description: '最深推理，适合复杂实现和排障。', default: true },
        ],
      },
      { id: 'mock-codex-fast', label: 'Mock Codex Fast' },
    ],
  },
]

// safeDecodeRoutePart 使用 value 参数安全解码 hash 路由片段。
function safeDecodeRoutePart(value: string) {
  try {
    return decodeURIComponent(value)
  } catch {
    return ''
  }
}

// parseHashRoute 使用 hash 参数解析当前 hash 路由。
function parseHashRoute(hash = window.location.hash): HashRoute {
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
function buildHashRoute(projectId: string, chatId: string) {
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
function updateHashRoute(projectId: string, chatId: string, mode: 'push' | 'replace' = 'replace') {
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
function updateSettingsHashRoute(mode: 'push' | 'replace' = 'push') {
  if (window.location.hash === '#/settings') {
    return
  }
  if (mode === 'push') {
    window.history.pushState(null, '', '#/settings')
    return
  }
  window.history.replaceState(null, '', '#/settings')
}

// formatTime 使用 value 参数格式化界面时间。
function formatTime(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return ''
  }
  return date.toLocaleTimeString('zh-CN', { hour12: false, hour: '2-digit', minute: '2-digit' })
}

// projectDisplayName 使用 project 参数返回侧边栏展示名称。
function projectDisplayName(project: Project) {
  const normalized = project.path.replace(/\\/g, '/').replace(/\/+$/, '')
  return normalized.split('/').filter(Boolean).pop() || project.name || project.path
}

// projectGitText 使用 project 参数返回顶部 Git 信息文案。
function projectGitText(project: Project | null) {
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
function chatVisualStatus(chat: Chat, indicators: Record<string, ChatTerminalIndicator>): ChatVisualStatus | null {
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
function mergeProjectVisualStatus(current: ChatVisualStatus | null, next: ChatVisualStatus | null) {
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

interface StatusDotProps {
  /** status 表示需要展示的状态。 */
  status: ChatVisualStatus
  /** testID 表示测试定位标识。 */
  testID: string
}

// StatusDot 使用 status 和 testID 参数渲染紧凑状态圆点。
function StatusDot({ status, testID }: StatusDotProps) {
  return (
    <span
      data-testid={testID}
      data-status={status}
      className={`inline-block h-2.5 w-2.5 shrink-0 rounded-full ${chatStatusClass[status]}`}
      title={chatStatusLabel[status]}
      aria-label={chatStatusLabel[status]}
    />
  )
}

// chatHasStarted 使用 chat 参数判断当前聊天页是否已经发送过消息。
function chatHasStarted(chat: Chat | null) {
  return Boolean(chat?.messages.some((message) => message.role === 'user'))
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
function toolCommandTitle(tool: ToolCall) {
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
function messagePartsForRender(message: ChatMessage): MessagePart[] {
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

// defaultModelForProvider 使用 providers 和 provider 参数返回默认模型。
function defaultModelForProvider(providers: AgentProviderOption[], provider: AgentProvider) {
  const models = providers.find((item) => item.id === provider)?.models ?? []
  return models.find((model) => model.default)?.id ?? models[0]?.id ?? ''
}

// defaultReasoningForModel 使用 providers、provider 和 model 参数返回默认推理级别。
function defaultReasoningForModel(providers: AgentProviderOption[], provider: AgentProvider, model: string) {
  const levels =
    providers.find((item) => item.id === provider)?.models.find((item) => item.id === model)?.reasoningLevels ?? []
  return levels.find((level) => level.default)?.id ?? levels[0]?.id ?? ''
}

// upsertById 使用 list、item 和 getId 参数插入或更新列表项。
function upsertById<TItem>(list: TItem[], item: TItem, getId: (value: TItem) => string) {
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
function sortByCreatedAt<TItem extends { createdAt: string; id: string }>(list: TItem[]) {
  return [...list].sort((left, right) => {
    const leftTime = new Date(left.createdAt).getTime()
    const rightTime = new Date(right.createdAt).getTime()
    if (leftTime === rightTime) {
      return left.id.localeCompare(right.id)
    }
    return leftTime - rightTime
  })
}

// normalizeChat 使用 chat 参数确保聊天页数据结构完整。
function normalizeChat(chat: Chat) {
  return {
    ...chat,
    agentProvider: chat.agentProvider ?? 'mock-claude-code',
    agentModel: chat.agentModel ?? 'mock-claude-sonnet',
    agentReasoning: chat.agentReasoning ?? '',
    agentLocked: chat.agentLocked ?? (chat.messages?.length ?? 0) > 0,
    messages: chat.messages ?? [],
  }
}

// App 渲染 project 和 agent 聊天主界面。
function App() {
  const wsRef = useRef<WebSocket | null>(null)
  const pendingCreatedChatProjectIdRef = useRef('')
  const chatsRef = useRef<Chat[]>([])
  const [connectionState, setConnectionState] = useState<ConnectionState>('connecting')
  const [hostname, setHostname] = useState('')
  const [projects, setProjects] = useState<Project[]>([])
  const [chats, setChats] = useState<Chat[]>([])
  const [agentProviders, setAgentProviders] = useState<AgentProviderOption[]>(fallbackAgentProviders)
  const [chatIndicators, setChatIndicators] = useState<Record<string, ChatTerminalIndicator>>({})
  const [routeView, setRouteView] = useState<'chat' | 'settings'>(() => parseHashRoute().view)
  const [selectedProjectId, setSelectedProjectId] = useState(() => parseHashRoute().projectId)
  const [selectedChatId, setSelectedChatId] = useState(() => parseHashRoute().chatId)
  const [projectFormId, setProjectFormId] = useState('')
  const [projectPath, setProjectPath] = useState('')
  const [projectDialogOpen, setProjectDialogOpen] = useState(false)
  const [newClaudeModelID, setNewClaudeModelID] = useState('')
  const [newClaudeModelLabel, setNewClaudeModelLabel] = useState('')
  const [composerValue, setComposerValue] = useState('')
  const [errorText, setErrorText] = useState('')
  const [hasSnapshot, setHasSnapshot] = useState(false)
  const [copiedMessageId, setCopiedMessageId] = useState('')

  const selectedProject = useMemo(
    () => projects.find((project) => project.id === selectedProjectId) ?? projects[0] ?? null,
    [projects, selectedProjectId],
  )
  const activeProjectId = selectedProject?.id ?? ''
  const projectChats = useMemo(
    () => sortByCreatedAt(chats.filter((chat) => chat.projectId === activeProjectId)),
    [activeProjectId, chats],
  )
  const selectedChat = useMemo(
    () => projectChats.find((chat) => chat.id === selectedChatId) ?? projectChats[0] ?? null,
    [projectChats, selectedChatId],
  )
  const isRunning = selectedChat?.status === 'running'
  const selectedAgentProvider = selectedChat?.agentProvider ?? 'mock-claude-code'
  const selectedAgentModels = agentProviders.find((provider) => provider.id === selectedAgentProvider)?.models ?? []
  const selectedAgentModel = selectedChat?.agentModel ?? defaultModelForProvider(agentProviders, selectedAgentProvider)
  const selectedAgentModelOption = selectedAgentModels.find((model) => model.id === selectedAgentModel) ?? null
  const selectedAgentReasoning =
    selectedChat?.agentReasoning || defaultReasoningForModel(agentProviders, selectedAgentProvider, selectedAgentModel)
  const providerLocked = chatHasStarted(selectedChat) || isRunning
  const agentControlsDisabled = !selectedChat || connectionState !== 'open'
  const modelControlsDisabled = agentControlsDisabled || isRunning
  const selectedAgentLabel = agentProviders.find((provider) => provider.id === selectedAgentProvider)?.label ?? 'Agent'
  const claudeCodeModels = agentProviders.find((provider) => provider.id === 'claude-code')?.models ?? []
  const projectVisualStatuses = useMemo(() => {
    const statuses = new Map<string, ChatVisualStatus>()
    for (const chat of chats) {
      const status = chatVisualStatus(chat, chatIndicators)
      const mergedStatus = mergeProjectVisualStatus(statuses.get(chat.projectId) ?? null, status)
      if (mergedStatus) {
        statuses.set(chat.projectId, mergedStatus)
      }
    }
    return statuses
  }, [chatIndicators, chats])

  useEffect(() => {
    chatsRef.current = chats
  }, [chats])

  // resetProjectForm 使用 project 参数重置 project 表单。
  const resetProjectForm = useCallback((project?: Project | null) => {
    setProjectFormId(project?.id ?? '')
    setProjectPath(project?.path ?? '')
    setErrorText('')
  }, [])

  // clearChatIndicator 使用 chatId 参数清除聊天页的成功或失败提示。
  const clearChatIndicator = useCallback((chatId: string) => {
    if (!chatId) {
      return
    }
    setChatIndicators((current) => {
      if (!current[chatId]) {
        return current
      }
      const next = { ...current }
      delete next[chatId]
      return next
    })
  }, [])

  // markChatIndicator 使用 chatId 和 status 参数记录聊天页终态提示。
  const markChatIndicator = useCallback((chatId: string, status: ChatTerminalIndicator) => {
    if (!chatId) {
      return
    }
    setChatIndicators((current) => ({ ...current, [chatId]: status }))
  }, [])

  useEffect(() => {
    // syncFromHash 从当前 hash 路由恢复选中的 project 和聊天页。
    const syncFromHash = () => {
      const route = parseHashRoute()
      setRouteView(route.view)
      setSelectedProjectId(route.projectId)
      setSelectedChatId(route.chatId)
    }
    window.addEventListener('hashchange', syncFromHash)
    window.addEventListener('popstate', syncFromHash)
    return () => {
      window.removeEventListener('hashchange', syncFromHash)
      window.removeEventListener('popstate', syncFromHash)
    }
  }, [])

  useEffect(() => {
    if (!selectedProject) {
      return
    }
    if (!projectFormId) {
      return
    }
    if (projectFormId === selectedProject.id) {
      setProjectPath(selectedProject.path)
    }
  }, [projectFormId, selectedProject])

  useEffect(() => {
    if (!hasSnapshot) {
      return
    }
    if (selectedProject && selectedProject.id !== selectedProjectId) {
      setSelectedProjectId(selectedProject.id)
    } else if (!selectedProject && selectedProjectId) {
      setSelectedProjectId('')
    }
  }, [hasSnapshot, selectedProject, selectedProjectId])

  useEffect(() => {
    if (!hasSnapshot) {
      return
    }
    if (selectedChat && selectedChat.id !== selectedChatId) {
      setSelectedChatId(selectedChat.id)
    } else if (!selectedChat && selectedChatId) {
      setSelectedChatId('')
    }
  }, [hasSnapshot, selectedChat, selectedChatId])

  useEffect(() => {
    if (!hasSnapshot) {
      return
    }
    if (routeView !== 'chat') {
      return
    }
    updateHashRoute(selectedProject?.id ?? '', selectedChat?.id ?? '')
  }, [hasSnapshot, routeView, selectedProject?.id, selectedChat?.id])

  // handleServerMessage 使用 message 参数把服务端事件归并到前端状态。
  const handleServerMessage = useCallback(
    (message: ServerMessage) => {
      setHostname(message.hostname || window.location.hostname || 'unknown')
      if (message.type === 'state.snapshot') {
        const payload = message.payload as SnapshotPayload
        const nextProjects = sortByCreatedAt(payload.projects ?? [])
        const nextChats = sortByCreatedAt((payload.chats ?? []).map(normalizeChat))
        const nextChatIds = new Set(nextChats.map((chat) => chat.id))
        setAgentProviders(payload.agentProviders?.length ? payload.agentProviders : fallbackAgentProviders)
        setProjects(nextProjects)
        setChats(nextChats)
        setChatIndicators((current) => {
          const next: Record<string, ChatTerminalIndicator> = {}
          for (const [chatId, status] of Object.entries(current)) {
            if (nextChatIds.has(chatId)) {
              next[chatId] = status
            }
          }
          return next
        })
        setHasSnapshot(true)
        setSelectedProjectId((current) => {
          if (current && nextProjects.some((project) => project.id === current)) {
            return current
          }
          return nextProjects[0]?.id ?? ''
        })
        setSelectedChatId((current) => {
          if (current && nextChats.some((chat) => chat.id === current)) {
            return current
          }
          return nextChats[0]?.id ?? ''
        })
        return
      }
      if (message.type === 'project.changed') {
        const payload = message.payload as ProjectChangedPayload
        setProjects((current) => sortByCreatedAt(upsertById(current, payload.project, (project) => project.id)))
        setSelectedProjectId(payload.project.id)
        setProjectDialogOpen(false)
        resetProjectForm(null)
        return
      }
      if (message.type === 'project.deleted') {
        const payload = message.payload as ProjectDeletedPayload
        setProjects((current) => current.filter((project) => project.id !== payload.id))
        setChats((current) => current.filter((chat) => !payload.chatIds.includes(chat.id)))
        setChatIndicators((current) => {
          const next = { ...current }
          for (const chatId of payload.chatIds) {
            delete next[chatId]
          }
          return next
        })
        setSelectedProjectId((current) => (current === payload.id ? '' : current))
        setSelectedChatId((current) => (payload.chatIds.includes(current) ? '' : current))
        resetProjectForm(null)
        setProjectDialogOpen(false)
        return
      }
      if (message.type === 'chat.changed') {
        const payload = message.payload as ChatChangedPayload
        const chat = normalizeChat(payload.chat)
        setChats((current) => sortByCreatedAt(upsertById(current, chat, (item) => item.id)))
        setSelectedProjectId((current) => current || payload.chat.projectId)
        setSelectedChatId((current) => {
          if (pendingCreatedChatProjectIdRef.current === payload.chat.projectId) {
            pendingCreatedChatProjectIdRef.current = ''
            return payload.chat.id
          }
          return current || payload.chat.id
        })
        return
      }
      if (message.type === 'chat.deleted') {
        const payload = message.payload as ChatDeletedPayload
        setChats((current) => current.filter((chat) => chat.id !== payload.id))
        setChatIndicators((current) => {
          if (!current[payload.id]) {
            return current
          }
          const next = { ...current }
          delete next[payload.id]
          return next
        })
        setSelectedChatId((current) => (current === payload.id ? '' : current))
        return
      }
      if (message.type === 'chat.message.delta') {
        const payload = message.payload as ChatMessageDeltaPayload
        setChats((current) =>
          current.map((chat) => {
            if (chat.id !== payload.chatId) {
              return chat
            }
            return {
              ...chat,
              messages: chat.messages.map((item) =>
                item.id === payload.messageId ? { ...(payload.message ?? item), text: payload.text, status: 'streaming' } : item,
              ),
            }
          }),
        )
        return
      }
      if (message.type === 'chat.message.done') {
        const payload = message.payload as ChatMessageDonePayload
        if (payload.message.status === 'complete') {
          markChatIndicator(payload.chatId, 'success')
        } else if (payload.message.status === 'error') {
          markChatIndicator(payload.chatId, 'error')
        }
        setChats((current) =>
          current.map((chat) => {
            if (chat.id !== payload.chatId) {
              return chat
            }
            return {
              ...chat,
              messages: chat.messages.map((item) => (item.id === payload.message.id ? payload.message : item)),
            }
          }),
        )
        return
      }
      if (message.type === 'agent.status') {
        const payload = message.payload as AgentStatusPayload
        if (payload.status === 'running') {
          clearChatIndicator(payload.chatId)
        } else if (payload.status === 'error') {
          markChatIndicator(payload.chatId, 'error')
        }
        setChats((current) =>
          current.map((chat) => (chat.id === payload.chatId ? { ...chat, status: payload.status } : chat)),
        )
        return
      }
      if (message.type === 'agent.providers.changed') {
        const payload = message.payload as AgentProvidersChangedPayload
        setAgentProviders(payload.agentProviders?.length ? payload.agentProviders : fallbackAgentProviders)
        setNewClaudeModelID('')
        setNewClaudeModelLabel('')
        return
      }
      if (message.type === 'error') {
        const payload = message.payload as { message?: string }
        setErrorText(payload.message ?? '服务端错误')
      }
    },
    [clearChatIndicator, markChatIndicator, resetProjectForm],
  )

  useEffect(() => {
    let stopped = false
    let retryTimer = 0
    let heartbeatTimer = 0

    // connect 建立 WebSocket 连接，并在断开后自动重连。
    const connect = () => {
      setConnectionState('connecting')
      const ws = new WebSocket(getWebSocketUrl())
      wsRef.current = ws

      ws.onopen = () => {
        setConnectionState('open')
        heartbeatTimer = window.setInterval(() => {
          sendClientMessage(ws, 'ping')
        }, 8000)
      }

      ws.onmessage = (event) => {
        const message = JSON.parse(event.data) as ServerMessage
        handleServerMessage(message)
      }

      ws.onerror = () => {
        setConnectionState('error')
        setChatIndicators((current) => {
          const next = { ...current }
          for (const chat of chatsRef.current) {
            if (chat.status === 'running') {
              next[chat.id] = 'error'
            }
          }
          return next
        })
      }

      ws.onclose = () => {
        window.clearInterval(heartbeatTimer)
        if (stopped) {
          return
        }
        setConnectionState('closed')
        retryTimer = window.setTimeout(connect, 1000)
      }
    }

    connect()
    return () => {
      stopped = true
      window.clearInterval(heartbeatTimer)
      window.clearTimeout(retryTimer)
      wsRef.current?.close()
    }
  }, [handleServerMessage])

  // saveProject 处理 event 参数对应的 project 表单提交。
  const saveProject = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const type = projectFormId ? 'project.update' : 'project.create'
    const payload = projectFormId ? { id: projectFormId, path: projectPath.trim() } : { path: projectPath.trim() }
    if (!sendClientMessage(wsRef.current, type, payload)) {
      setErrorText('WebSocket 未连接')
    }
  }

  // openProjectDialog 打开新建 project 工作目录输入框。
  const openProjectDialog = () => {
    resetProjectForm(null)
    setProjectDialogOpen(true)
  }

  // closeProjectDialog 关闭新建 project 工作目录输入框。
  const closeProjectDialog = () => {
    setProjectDialogOpen(false)
    resetProjectForm(null)
  }

  // deleteProject 使用 project 参数删除 project。
  const deleteProject = (project: Project) => {
    sendClientMessage(wsRef.current, 'project.delete', { id: project.id })
  }

  // deleteChat 使用 chat 参数关闭并删除聊天页。
  const deleteChat = (chat: Chat) => {
    sendClientMessage(wsRef.current, 'chat.delete', { id: chat.id })
  }

  // createChat 为当前 project 创建聊天页。
  const createChat = () => {
    if (!activeProjectId) {
      return
    }
    pendingCreatedChatProjectIdRef.current = activeProjectId
    sendClientMessage(wsRef.current, 'chat.create', { projectId: activeProjectId })
  }

  // updateChatAgent 使用 provider、model 和 reasoning 参数更新当前聊天页 agent 配置。
  const updateChatAgent = (provider: AgentProvider, model: string, reasoning: string) => {
    if (!selectedChat) {
      return
    }
    sendClientMessage(wsRef.current, 'chat.agent.update', {
      chatId: selectedChat.id,
      provider,
      model,
      reasoning,
    })
  }

  // changeAgentProvider 使用 provider 参数切换当前聊天页 agent。
  const changeAgentProvider = (provider: AgentProvider) => {
    const model = defaultModelForProvider(agentProviders, provider)
    const reasoning = defaultReasoningForModel(agentProviders, provider, model)
    updateChatAgent(provider, model, reasoning)
  }

  // changeAgentModel 使用 model 参数切换当前聊天页模型。
  const changeAgentModel = (model: string) => {
    const reasoning = defaultReasoningForModel(agentProviders, selectedAgentProvider, model)
    updateChatAgent(selectedAgentProvider, model, reasoning)
  }

  // changeAgentReasoning 使用 reasoning 参数切换当前聊天页推理级别。
  const changeAgentReasoning = (reasoning: string) => {
    updateChatAgent(selectedAgentProvider, selectedAgentModel, reasoning)
  }

  // openAgentSettings 打开 agent 设置页。
  const openAgentSettings = () => {
    setRouteView('settings')
    updateSettingsHashRoute('push')
  }

  // backToChat 从设置页回到聊天页。
  const backToChat = () => {
    setRouteView('chat')
    updateHashRoute(selectedProject?.id ?? '', selectedChat?.id ?? '', 'push')
  }

  // addClaudeModel 处理 event 参数对应的 Claude Code 模型新增提交。
  const addClaudeModel = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    sendClientMessage(wsRef.current, 'agent.model.add', {
      provider: 'claude-code',
      id: newClaudeModelID.trim(),
      label: newClaudeModelLabel.trim(),
    })
  }

  // selectProject 使用 project 参数切换当前 project 并写入 hash 路由。
  const selectProject = (project: Project) => {
    setRouteView('chat')
    setSelectedProjectId(project.id)
    setSelectedChatId('')
    updateHashRoute(project.id, '', 'push')
  }

  // selectChat 使用 chat 参数切换当前聊天页并写入 hash 路由。
  const selectChat = (chat: Chat) => {
    setRouteView('chat')
    setSelectedProjectId(chat.projectId)
    setSelectedChatId(chat.id)
    clearChatIndicator(chat.id)
    updateHashRoute(chat.projectId, chat.id, 'push')
  }

  // copyMessageText 使用 message 参数复制聊天消息文本。
  const copyMessageText = async (message: ChatMessage) => {
    const text = message.text.trim()
    if (!text) {
      return
    }
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text)
    }
    setCopiedMessageId(message.id)
    window.setTimeout(() => {
      setCopiedMessageId((current) => (current === message.id ? '' : current))
    }, 1200)
  }

  // submitComposer 处理 event 参数对应的聊天输入提交。
  const submitComposer = (event?: FormEvent<HTMLFormElement>) => {
    event?.preventDefault()
    if (!selectedChat) {
      return
    }
    const prompt = composerValue.trim()
    if (!prompt && isRunning) {
      sendClientMessage(wsRef.current, 'chat.stop', { chatId: selectedChat.id })
      return
    }
    if (!prompt) {
      return
    }
    sendClientMessage(wsRef.current, 'chat.send', { chatId: selectedChat.id, prompt })
    setComposerValue('')
  }

  // handleComposerKeyDown 使用 event 参数处理 Enter 发送。
  const handleComposerKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key !== 'Enter' || event.shiftKey || event.nativeEvent.isComposing) {
      return
    }
    event.preventDefault()
    submitComposer()
  }

  const connectionIcon =
    connectionState === 'open' ? (
      <Wifi className="h-4 w-4 text-teal-500" />
    ) : connectionState === 'connecting' ? (
      <Loader2 className="h-4 w-4 animate-spin text-amber-500" />
    ) : (
      <WifiOff className="h-4 w-4 text-rose-500" />
    )

  return (
    <main className="theme-paseo grid h-[100dvh] min-h-0 overflow-hidden bg-slate-100 text-slate-950 lg:grid-cols-[320px_minmax(0,1fr)]">
      <aside data-testid="sidebar" className="relative flex h-full min-h-0 flex-col border-r border-slate-800 bg-slate-950 text-slate-100">
        <div className="border-b border-slate-800 px-3 py-3">
          <div data-testid="sidebar-identity" className="flex min-w-0 items-center gap-2 text-xs text-slate-500">
            <Monitor className="h-3.5 w-3.5 shrink-0" />
            <span data-testid="machine-name" className="min-w-0 flex-1 truncate font-mono">
              {hostname || 'unknown'}
            </span>
            <span data-testid="connection-state" className="inline-flex shrink-0 items-center gap-1.5">
              {connectionIcon}
              <span>{connectionText[connectionState]}</span>
            </span>
          </div>
        </div>

        <div className="min-h-0 flex-1 overflow-y-auto px-2 py-3" data-testid="project-list">
          {projects.length === 0 ? (
            <div className="rounded-md border border-dashed border-slate-700 px-3 py-8 text-center text-sm text-slate-500">
              还没有 Project
            </div>
          ) : (
            projects.map((project) => {
              const projectStatus = projectVisualStatuses.get(project.id)
              return (
                <div
                  key={project.id}
                  className={`mb-1 rounded-md border px-2 py-2 transition ${
                    project.id === selectedProjectId
                      ? 'border-teal-500 bg-teal-500/10'
                      : 'border-transparent hover:border-slate-700 hover:bg-slate-900'
                  }`}
                >
                  <div className="flex items-center gap-2">
                    <button
                      type="button"
                      onClick={() => selectProject(project)}
                      className="flex min-w-0 flex-1 cursor-pointer items-center gap-2 text-left"
                    >
                      <Folder className="h-4 w-4 shrink-0 text-teal-400" />
                      <span data-testid="project-name" className="min-w-0 flex-1 truncate text-sm font-medium text-slate-100">
                        {projectDisplayName(project)}
                      </span>
                      {projectStatus ? <StatusDot status={projectStatus} testID="project-status-dot" /> : null}
                    </button>
                    <button
                      data-testid="project-delete-button"
                      type="button"
                      onClick={() => deleteProject(project)}
                      className="inline-flex h-7 w-7 shrink-0 cursor-pointer items-center justify-center rounded-md text-slate-400 transition hover:bg-rose-500/15 hover:text-rose-300"
                      aria-label="删除 Project"
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                    </button>
                  </div>
                </div>
              )
            })
          )}
        </div>

        {projectDialogOpen ? (
          <div className="absolute bottom-[4.5rem] left-3 right-3 z-20">
            <form className="rounded-lg border border-slate-200 bg-white p-3 text-slate-900 shadow-lg" onSubmit={saveProject}>
              <div className="mb-2 flex items-center justify-between gap-2">
                <label htmlFor="project-path-input" className="text-xs font-medium text-slate-500">
                  工作目录
                </label>
                <button
                  type="button"
                  onClick={closeProjectDialog}
                  className="inline-flex h-7 w-7 cursor-pointer items-center justify-center rounded-md text-slate-400 transition hover:bg-slate-100 hover:text-slate-900"
                  aria-label="关闭"
                >
                  <X className="h-4 w-4" />
                </button>
              </div>
              <Input
                id="project-path-input"
                data-testid="project-path-input"
                value={projectPath}
                onChange={(event) => setProjectPath(event.target.value)}
                className="h-9 w-full rounded-md border border-slate-200 bg-white px-3 font-mono text-sm text-slate-900 outline-none transition placeholder:text-slate-400 focus:border-teal-600 focus:ring-2 focus:ring-teal-100"
                placeholder="/workspace/project/coding"
              />
              <button
                data-testid="project-save-button"
                type="submit"
                disabled={connectionState !== 'open' || !projectPath.trim()}
                className="mt-3 inline-flex h-9 w-full cursor-pointer items-center justify-center gap-2 rounded-md bg-teal-600 px-3 text-sm font-medium text-white transition hover:bg-teal-500 focus:outline-none focus:ring-2 focus:ring-teal-400 disabled:cursor-not-allowed disabled:bg-slate-300 disabled:text-slate-500"
              >
                <Plus className="h-4 w-4" />
                添加
              </button>
              {errorText ? <p className="mt-2 text-xs text-rose-600">{errorText}</p> : null}
            </form>
          </div>
        ) : null}

        <div data-testid="sidebar-footer" className="border-t border-slate-800 p-3">
          <div className="flex gap-2">
            <Button
              data-testid="project-add-button"
              type="button"
              variant="outline"
              onClick={openProjectDialog}
              className="h-9 flex-1 px-0"
              aria-label="添加 Project"
              title="添加 Project"
            >
              <Plus className="h-4 w-4" />
            </Button>
            <Button
              data-testid="agent-settings-button"
              type="button"
              variant={routeView === 'settings' ? 'default' : 'outline'}
              onClick={openAgentSettings}
              className="h-9 flex-1 px-0"
              aria-label="设置"
              title="设置"
            >
              <Settings className="h-4 w-4" />
            </Button>
          </div>
        </div>
      </aside>

      <section className="flex h-full min-h-0 min-w-0 flex-col">
        {routeView === 'settings' ? (
          <div className="flex h-full min-h-0 min-w-0 flex-col" data-testid="agent-settings-page">
            <header className="flex min-h-16 items-center justify-between border-b border-slate-200 bg-white px-4">
              <div>
                <h2 className="text-lg font-semibold">Agent 设置</h2>
                <p className="text-xs text-slate-500">维护 Claude Code 可选模型</p>
              </div>
              <button
                data-testid="back-to-chat-button"
                type="button"
                onClick={backToChat}
                className="inline-flex h-9 cursor-pointer items-center justify-center rounded-md border border-slate-300 px-3 text-sm font-medium text-slate-700 transition hover:border-slate-500 hover:text-slate-950"
              >
                返回聊天
              </button>
            </header>
            <div className="min-h-0 flex-1 overflow-y-auto px-4 py-5">
              <div className="mx-auto grid max-w-4xl gap-6 lg:grid-cols-[minmax(0,1fr)_320px]">
                <section className="min-w-0">
                  <div className="mb-3 flex items-center justify-between">
                    <h3 className="text-base font-semibold text-slate-900">Claude Code 模型</h3>
                    <span className="text-xs text-slate-500">{claudeCodeModels.length} 个选项</span>
                  </div>
                  <div className="overflow-hidden rounded-md border border-slate-200 bg-white" data-testid="agent-settings-model-list">
                    {claudeCodeModels.map((model) => (
                      <div key={model.id} className="flex items-center justify-between gap-3 border-b border-slate-100 px-3 py-3 last:border-b-0">
                        <div className="min-w-0">
                          <div className="truncate text-sm font-medium text-slate-900">{model.label}</div>
                          <div className="truncate font-mono text-xs text-slate-500">{model.id}</div>
                        </div>
                        {model.default ? <span className="shrink-0 text-xs font-medium text-teal-700">默认</span> : null}
                      </div>
                    ))}
                  </div>
                </section>
                <form className="rounded-md border border-slate-200 bg-white p-4" onSubmit={addClaudeModel}>
                  <h3 className="text-base font-semibold text-slate-900">新增模型</h3>
                  <label htmlFor="agent-model-id-input" className="mt-4 block text-xs font-medium text-slate-500">
                    模型标识
                  </label>
                  <Input
                    id="agent-model-id-input"
                    data-testid="agent-model-id-input"
                    value={newClaudeModelID}
                    onChange={(event) => setNewClaudeModelID(event.target.value)}
                    className="mt-1 h-9 w-full rounded-md border border-slate-300 px-3 font-mono text-sm outline-none transition focus:border-teal-600 focus:ring-2 focus:ring-teal-100"
                    placeholder="claude-sonnet-4-6"
                  />
                  <label htmlFor="agent-model-label-input" className="mt-3 block text-xs font-medium text-slate-500">
                    展示名称
                  </label>
                  <Input
                    id="agent-model-label-input"
                    data-testid="agent-model-label-input"
                    value={newClaudeModelLabel}
                    onChange={(event) => setNewClaudeModelLabel(event.target.value)}
                    className="mt-1 h-9 w-full rounded-md border border-slate-300 px-3 text-sm outline-none transition focus:border-teal-600 focus:ring-2 focus:ring-teal-100"
                    placeholder="Claude Sonnet 4.6"
                  />
                  <button
                    data-testid="agent-model-add-button"
                    type="submit"
                    disabled={connectionState !== 'open' || !newClaudeModelID.trim()}
                    className="mt-4 inline-flex h-9 w-full cursor-pointer items-center justify-center gap-2 rounded-md bg-teal-600 px-3 text-sm font-medium text-white transition hover:bg-teal-500 disabled:cursor-not-allowed disabled:bg-slate-300"
                  >
                    <Plus className="h-4 w-4" />
                    添加
                  </button>
                </form>
              </div>
            </div>
          </div>
        ) : (
          <>
            <header className="flex min-h-14 items-center border-b border-slate-200 bg-white px-4" data-testid="project-meta">
              <div className="flex min-w-0 flex-1 items-center gap-3 text-xs text-slate-500">
                <span data-testid="project-path-text" className="min-w-0 truncate font-mono text-sm font-medium text-slate-900">
                  {selectedProject?.path ?? '选择或创建 Project'}
                </span>
                <span data-testid="project-git-info" className="inline-flex min-w-0 shrink-0 items-center gap-2">
                  <span className="inline-flex min-w-0 items-center gap-1">
                    <GitBranch className="h-3.5 w-3.5 shrink-0" />
                    <span className="truncate">{projectGitText(selectedProject)}</span>
                  </span>
                  {selectedProject?.git?.isRepo ? (
                    <span className="inline-flex items-center gap-1 font-mono">
                      <GitCommit className="h-3.5 w-3.5" />
                      <span data-testid="project-commit-text">{selectedProject.git.commit || '-'}</span>
                    </span>
                  ) : null}
                </span>
              </div>
            </header>

            <Tabs
              value={selectedChat?.id ?? ''}
              onValueChange={(chatId) => {
                const chat = projectChats.find((item) => item.id === chatId)
                if (chat) {
                  selectChat(chat)
                }
              }}
              className="min-h-0 flex-1 overflow-hidden"
            >
              <div className="flex min-h-8 items-stretch border-b border-slate-200 bg-slate-50 px-2" data-testid="chat-tabs">
                <TabsList className="h-full flex-1">
                  {projectChats.length === 0 ? (
                    <span className="flex items-center text-xs text-slate-500">没有聊天页</span>
                  ) : (
                    projectChats.map((chat) => {
                      const status = chatVisualStatus(chat, chatIndicators)
                      return (
                        <TabsTrigger key={chat.id} value={chat.id} data-testid="chat-tab" className="group/tab max-w-60 pr-1">
                          <MessageSquare className="h-3.5 w-3.5 shrink-0" />
                          <span className="truncate">{chat.title}</span>
                          {status ? <StatusDot status={status} testID="chat-status-dot" /> : null}
                          <span
                            data-testid="chat-tab-close-button"
                            role="button"
                            tabIndex={0}
                            onClick={(event) => {
                              event.stopPropagation()
                              deleteChat(chat)
                            }}
                            onKeyDown={(event) => {
                              if (event.key !== 'Enter' && event.key !== ' ') {
                                return
                              }
                              event.preventDefault()
                              event.stopPropagation()
                              deleteChat(chat)
                            }}
                            className="inline-flex h-5 w-5 shrink-0 cursor-pointer items-center justify-center rounded text-slate-400 opacity-80 transition hover:bg-slate-200 hover:text-slate-900 group-hover/tab:opacity-100"
                            aria-label="关闭聊天页"
                            title="关闭"
                          >
                            <X className="h-3.5 w-3.5" />
                          </span>
                        </TabsTrigger>
                      )
                    })
                  )}
                  <Button
                    data-testid="chat-tab-add-button"
                    variant="ghost"
                    size="icon"
                    onClick={createChat}
                    disabled={!activeProjectId || connectionState !== 'open'}
                    className="my-0.5 h-7 w-7 shrink-0"
                    aria-label="新建聊天"
                  >
                    <Plus className="h-3.5 w-3.5" />
                  </Button>
                </TabsList>
              </div>

              {selectedChat ? (
                <TabsContent value={selectedChat.id}>
                  <div className="min-h-0 flex-1 overflow-y-auto px-4 py-5" data-testid="message-log" aria-live="polite">
                    {selectedChat.messages.length > 0 ? (
                      <div className="mx-auto flex max-w-4xl flex-col gap-3">
                        {selectedChat.messages.map((message) => (
                          <article
                            key={message.id}
                            className={`message-card message-${message.role} group/message relative rounded-md border p-4 ${
                              message.role === 'user'
                                ? 'border-teal-200 bg-teal-50'
                                : message.role === 'system'
                                  ? 'border-rose-200 bg-rose-50'
                                  : 'border-transparent bg-transparent'
                            }`}
                          >
                            <div className="mb-2 flex items-center justify-between gap-3">
                              <span className="text-sm font-medium text-slate-800">
                                {message.role === 'user' ? '你' : message.role === 'assistant' ? selectedAgentLabel : '系统'}
                              </span>
                              <span className="inline-flex items-center gap-2 text-xs text-slate-500">
                                {message.status === 'streaming' ? <Loader2 className="h-3.5 w-3.5 animate-spin text-orange-500" /> : null}
                                {message.status === 'stopped' ? '已停止' : message.status === 'error' ? '失败' : formatTime(message.updatedAt)}
                              </span>
                            </div>
                            {message.role === 'assistant' ? (
                              <div className="space-y-2">
                                {messagePartsForRender(message).map((part) =>
                                  part.type === 'tool_call' && part.toolCall ? (
                                    <details
                                      key={part.id}
                                      data-testid="tool-call-details"
                                      className="rounded-md border border-slate-200 bg-slate-50 px-3 py-2"
                                    >
                                      <summary className="flex cursor-pointer list-none items-center justify-between gap-3">
                                        <span className="inline-flex min-w-0 items-center gap-2 text-sm font-medium text-slate-800">
                                          <Wrench className="h-4 w-4 shrink-0 text-slate-500" />
                                          <span className="truncate font-mono text-xs">{toolCommandTitle(part.toolCall)}</span>
                                        </span>
                                        <span className="shrink-0 text-xs text-slate-500">
                                          {part.toolCall.status === 'running' ? '运行中' : part.toolCall.status === 'error' ? '失败' : '完成'}
                                        </span>
                                      </summary>
                                      {part.toolCall.input ? <pre className="mt-2 truncate font-mono text-xs text-slate-500">{part.toolCall.input}</pre> : null}
                                      {part.toolCall.output ? (
                                        <pre data-testid="tool-call-output" className="mt-2 whitespace-pre-wrap break-words font-mono text-xs text-slate-600">
                                          {part.toolCall.output}
                                        </pre>
                                      ) : null}
                                    </details>
                                  ) : (
                                    <MarkdownRenderer key={part.id} text={part.text ?? ''} />
                                  ),
                                )}
                              </div>
                            ) : (
                              <pre className="whitespace-pre-wrap break-words font-sans text-sm leading-6 text-slate-800">{message.text}</pre>
                            )}
                            {message.role === 'user' ? (
                              <button
                                data-testid="user-copy-button"
                                type="button"
                                onClick={() => void copyMessageText(message)}
                                className="absolute bottom-2 right-2 inline-flex h-7 w-7 cursor-pointer items-center justify-center rounded-md text-slate-500 opacity-0 transition hover:bg-white hover:text-slate-900 focus:opacity-100 focus:outline-none focus:ring-2 focus:ring-teal-500/20 group-hover/message:opacity-100"
                                aria-label="复制消息"
                                title="复制"
                              >
                                {copiedMessageId === message.id ? <Check className="h-3.5 w-3.5" /> : <Copy className="h-3.5 w-3.5" />}
                              </button>
                            ) : null}
                            {message.role === 'assistant' ? (
                              <div className="mt-2 flex justify-start">
                                <button
                                  data-testid="assistant-copy-button"
                                  type="button"
                                  onClick={() => void copyMessageText(message)}
                                  className="inline-flex h-7 w-7 cursor-pointer items-center justify-center rounded-md text-slate-500 transition hover:bg-slate-100 hover:text-slate-900 focus:outline-none focus:ring-2 focus:ring-teal-500/20"
                                  aria-label="复制回复"
                                  title="复制"
                                >
                                  {copiedMessageId === message.id ? <Check className="h-3.5 w-3.5" /> : <Copy className="h-3.5 w-3.5" />}
                                </button>
                              </div>
                            ) : null}
                          </article>
                        ))}
                      </div>
                    ) : null}
                  </div>

                  <form
                    data-testid="composer-taskbar"
                    className="sticky bottom-0 z-10 shrink-0 bg-white px-4 py-4"
                    onSubmit={submitComposer}
                  >
                    <div
                      data-testid="composer-shell"
                      className="composer-shell mx-auto w-full max-w-[860px] rounded-[20px] border border-slate-200 bg-white px-4 py-3 shadow-sm"
                    >
                      <div>
                        <label htmlFor="message-input" className="sr-only">
                          Prompt
                        </label>
                        <Textarea
                          id="message-input"
                          data-testid="message-input"
                          value={composerValue}
                          onChange={(event) => setComposerValue(event.target.value)}
                          onKeyDown={handleComposerKeyDown}
                          disabled={!selectedChat || connectionState !== 'open'}
                          rows={2}
                          className="max-h-40 min-h-24 border-0 px-0 py-0 text-[15px] leading-6 shadow-none focus:ring-0"
                          placeholder={selectedChat ? '输入消息' : '先创建聊天'}
                        />
                      </div>
                      <div className="mt-3 flex flex-wrap items-center gap-2" data-testid="composer-agent-config">
                        <Button type="button" variant="ghost" size="icon" className="h-8 w-8 rounded-full text-slate-500" aria-label="添加上下文">
                          <Plus className="h-4 w-4" />
                        </Button>
                        <div className="relative">
                          <Bot className="pointer-events-none absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-slate-500" />
                          <Select
                            id="agent-provider-select"
                            data-testid="agent-provider-select"
                            value={selectedAgentProvider}
                            onChange={(event) => changeAgentProvider(event.target.value as AgentProvider)}
                            disabled={agentControlsDisabled || providerLocked}
                            aria-label="选择助理"
                            className="composer-select h-8 min-w-36 max-w-48 rounded-full border-transparent bg-slate-50 pl-8 pr-3 text-xs"
                          >
                            {agentProviders.map((provider) => (
                              <option key={provider.id} value={provider.id}>
                                {provider.label}
                              </option>
                            ))}
                          </Select>
                        </div>
                        <label htmlFor="agent-model-select" className="sr-only">
                          模型
                        </label>
                        <div className="relative">
                          <MessageSquare className="pointer-events-none absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-slate-500" />
                          <Select
                            id="agent-model-select"
                            data-testid="agent-model-select"
                            value={selectedAgentModel}
                            onChange={(event) => changeAgentModel(event.target.value)}
                            disabled={modelControlsDisabled}
                            className="composer-select h-8 min-w-44 max-w-56 rounded-full border-transparent bg-slate-50 pl-8 pr-3 text-xs"
                          >
                            {selectedAgentModels.map((model) => (
                              <option key={model.id} value={model.id}>
                                {model.label}
                              </option>
                            ))}
                          </Select>
                        </div>
                        {selectedAgentModelOption?.reasoningLevels?.length ? (
                          <div className="relative">
                            <label htmlFor="agent-reasoning-select" className="sr-only">
                              推理级别
                            </label>
                            <Brain className="pointer-events-none absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-slate-500" />
                            <Select
                              id="agent-reasoning-select"
                              data-testid="agent-reasoning-select"
                              value={selectedAgentReasoning}
                              onChange={(event) => changeAgentReasoning(event.target.value)}
                              disabled={modelControlsDisabled}
                              className="composer-select h-8 min-w-28 max-w-36 rounded-full border-transparent bg-slate-50 pl-8 pr-3 text-xs"
                            >
                              {selectedAgentModelOption.reasoningLevels.map((level) => (
                                <option key={level.id} value={level.id}>
                                  {level.label}
                                </option>
                              ))}
                            </Select>
                          </div>
                        ) : null}
                        <span className="min-w-4 flex-1" />
                        {composerValue.trim() || isRunning ? (
                          <Button
                            data-testid="send-button"
                            type="submit"
                            size="icon"
                            disabled={!selectedChat || connectionState !== 'open' || (!composerValue.trim() && !isRunning)}
                            className={`h-9 w-9 shrink-0 rounded-full ${
                              isRunning && !composerValue.trim() ? 'bg-orange-600 hover:bg-orange-500' : ''
                            }`}
                            aria-label={isRunning && !composerValue.trim() ? '停止' : '发送'}
                          >
                            {isRunning && !composerValue.trim() ? <Square className="h-4 w-4" fill="currentColor" /> : <ArrowUp className="h-4 w-4" />}
                          </Button>
                        ) : null}
                      </div>
                    </div>
                  </form>
                </TabsContent>
              ) : (
                <div className="min-h-0 flex-1 overflow-y-auto px-4 py-5" data-testid="message-log" aria-live="polite" />
              )}
            </Tabs>
          </>
        )}
      </section>
    </main>
  )
}

export default App
