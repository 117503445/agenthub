import { FormEvent, KeyboardEvent, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  ArrowUp,
  Clock3,
  Folder,
  Loader2,
  MessageSquare,
  Pencil,
  Plus,
  Save,
  Square,
  Trash2,
  Wifi,
  WifiOff,
  X,
} from 'lucide-react'
import { getWebSocketUrl, sendClientMessage, type ServerMessage } from './lib/ws'

type ConnectionState = 'connecting' | 'open' | 'closed' | 'error'
type ChatStatus = 'idle' | 'running' | 'error'
type MessageRole = 'user' | 'assistant' | 'system'
type MessageStatus = 'complete' | 'streaming' | 'stopped' | 'error'

interface Project {
  /** id 表示 project 唯一标识。 */
  id: string
  /** name 表示 project 展示名称。 */
  name: string
  /** path 表示后端本机工作目录。 */
  path: string
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
  /** agentSessionId 表示 Claude 会话标识。 */
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

interface ChatMessageDeltaPayload {
  /** chatId 表示聊天页标识。 */
  chatId: string
  /** messageId 表示消息标识。 */
  messageId: string
  /** delta 表示增量文本。 */
  delta: string
  /** text 表示服务端当前完整文本。 */
  text: string
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

interface HashRoute {
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
  const projectIndex = parts.indexOf('projects')
  if (projectIndex === -1) {
    return { projectId: '', chatId: '' }
  }
  const projectId = safeDecodeRoutePart(parts[projectIndex + 1] ?? '')
  const chatId = parts[projectIndex + 2] === 'chats' ? safeDecodeRoutePart(parts[projectIndex + 3] ?? '') : ''
  return { projectId, chatId }
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

// formatTime 使用 value 参数格式化界面时间。
function formatTime(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return ''
  }
  return date.toLocaleTimeString('zh-CN', { hour12: false, hour: '2-digit', minute: '2-digit' })
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
    messages: chat.messages ?? [],
  }
}

// App 渲染 project 和 agent 聊天主界面。
function App() {
  const wsRef = useRef<WebSocket | null>(null)
  const [connectionState, setConnectionState] = useState<ConnectionState>('connecting')
  const [version, setVersion] = useState('dev')
  const [projects, setProjects] = useState<Project[]>([])
  const [chats, setChats] = useState<Chat[]>([])
  const [selectedProjectId, setSelectedProjectId] = useState(() => parseHashRoute().projectId)
  const [selectedChatId, setSelectedChatId] = useState(() => parseHashRoute().chatId)
  const [projectFormId, setProjectFormId] = useState('')
  const [projectName, setProjectName] = useState('')
  const [projectPath, setProjectPath] = useState('')
  const [composerValue, setComposerValue] = useState('')
  const [errorText, setErrorText] = useState('')
  const [hasSnapshot, setHasSnapshot] = useState(false)

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

  // resetProjectForm 使用 project 参数重置 project 表单。
  const resetProjectForm = useCallback((project?: Project | null) => {
    setProjectFormId(project?.id ?? '')
    setProjectName(project?.name ?? '')
    setProjectPath(project?.path ?? '')
    setErrorText('')
  }, [])

  useEffect(() => {
    // syncFromHash 从当前 hash 路由恢复选中的 project 和聊天页。
    const syncFromHash = () => {
      const route = parseHashRoute()
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
      setProjectName(selectedProject.name)
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
    updateHashRoute(selectedProject?.id ?? '', selectedChat?.id ?? '')
  }, [hasSnapshot, selectedProject?.id, selectedChat?.id])

  // handleServerMessage 使用 message 参数把服务端事件归并到前端状态。
  const handleServerMessage = useCallback(
    (message: ServerMessage) => {
      setVersion(message.version)
      if (message.type === 'state.snapshot') {
        const payload = message.payload as SnapshotPayload
        const nextProjects = sortByCreatedAt(payload.projects ?? [])
        const nextChats = sortByCreatedAt((payload.chats ?? []).map(normalizeChat))
        setProjects(nextProjects)
        setChats(nextChats)
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
        resetProjectForm(payload.project)
        return
      }
      if (message.type === 'project.deleted') {
        const payload = message.payload as ProjectDeletedPayload
        setProjects((current) => current.filter((project) => project.id !== payload.id))
        setChats((current) => current.filter((chat) => !payload.chatIds.includes(chat.id)))
        setSelectedProjectId((current) => (current === payload.id ? '' : current))
        setSelectedChatId((current) => (payload.chatIds.includes(current) ? '' : current))
        resetProjectForm(null)
        return
      }
      if (message.type === 'chat.changed') {
        const payload = message.payload as ChatChangedPayload
        const chat = normalizeChat(payload.chat)
        setChats((current) => sortByCreatedAt(upsertById(current, chat, (item) => item.id)))
        setSelectedProjectId((current) => current || payload.chat.projectId)
        setSelectedChatId((current) => current || payload.chat.id)
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
                item.id === payload.messageId ? { ...item, text: payload.text, status: 'streaming' } : item,
              ),
            }
          }),
        )
        return
      }
      if (message.type === 'chat.message.done') {
        const payload = message.payload as ChatMessageDonePayload
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
        setChats((current) =>
          current.map((chat) => (chat.id === payload.chatId ? { ...chat, status: payload.status } : chat)),
        )
        return
      }
      if (message.type === 'error') {
        const payload = message.payload as { message?: string }
        setErrorText(payload.message ?? '服务端错误')
      }
    },
    [resetProjectForm],
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
    const payload = projectFormId
      ? { id: projectFormId, name: projectName.trim(), path: projectPath.trim() }
      : { name: projectName.trim(), path: projectPath.trim() }
    if (!sendClientMessage(wsRef.current, type, payload)) {
      setErrorText('WebSocket 未连接')
    }
  }

  // deleteProject 使用 project 参数删除 project。
  const deleteProject = (project: Project) => {
    sendClientMessage(wsRef.current, 'project.delete', { id: project.id })
  }

  // createChat 为当前 project 创建聊天页。
  const createChat = () => {
    if (!activeProjectId) {
      return
    }
    sendClientMessage(wsRef.current, 'chat.create', { projectId: activeProjectId })
  }

  // selectProject 使用 project 参数切换当前 project 并写入 hash 路由。
  const selectProject = (project: Project) => {
    setSelectedProjectId(project.id)
    setSelectedChatId('')
    resetProjectForm(project)
    updateHashRoute(project.id, '', 'push')
  }

  // selectChat 使用 chat 参数切换当前聊天页并写入 hash 路由。
  const selectChat = (chat: Chat) => {
    setSelectedProjectId(chat.projectId)
    setSelectedChatId(chat.id)
    updateHashRoute(chat.projectId, chat.id, 'push')
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
    <main className="grid min-h-screen bg-slate-100 text-slate-950 lg:grid-cols-[320px_minmax(0,1fr)]">
      <aside className="flex min-h-[280px] flex-col border-r border-slate-800 bg-slate-950 text-slate-100">
        <div className="flex items-center justify-between border-b border-slate-800 px-4 py-4">
          <div className="min-w-0">
            <h1 className="truncate text-base font-semibold">Coding Agent</h1>
            <div data-testid="connection-state" className="mt-1 flex items-center gap-2 text-xs text-slate-400">
              {connectionIcon}
              <span>{connectionText[connectionState]}</span>
              <span className="truncate font-mono">{version}</span>
            </div>
          </div>
          <button
            type="button"
            onClick={() => resetProjectForm(null)}
            className="inline-flex h-9 w-9 cursor-pointer items-center justify-center rounded-md border border-slate-700 text-slate-300 transition hover:border-teal-500 hover:text-white focus:outline-none focus:ring-2 focus:ring-teal-500"
            aria-label="新建 Project"
          >
            <Plus className="h-4 w-4" />
          </button>
        </div>

        <form className="border-b border-slate-800 p-4" onSubmit={saveProject}>
          <div className="mb-3 flex items-center justify-between">
            <span className="text-sm font-medium text-slate-200">{projectFormId ? '编辑 Project' : '新建 Project'}</span>
            {projectFormId ? (
              <button
                type="button"
                onClick={() => resetProjectForm(null)}
                className="inline-flex h-7 w-7 cursor-pointer items-center justify-center rounded-md text-slate-400 transition hover:bg-slate-800 hover:text-white"
                aria-label="取消编辑"
              >
                <X className="h-4 w-4" />
              </button>
            ) : null}
          </div>
          <label htmlFor="project-name-input" className="mb-1 block text-xs font-medium text-slate-400">
            名称
          </label>
          <input
            id="project-name-input"
            data-testid="project-name-input"
            value={projectName}
            onChange={(event) => setProjectName(event.target.value)}
            className="mb-3 h-9 w-full rounded-md border border-slate-700 bg-slate-900 px-3 text-sm text-white outline-none transition placeholder:text-slate-500 focus:border-teal-500 focus:ring-2 focus:ring-teal-500/30"
            placeholder="Project 名称"
          />
          <label htmlFor="project-path-input" className="mb-1 block text-xs font-medium text-slate-400">
            工作目录
          </label>
          <input
            id="project-path-input"
            data-testid="project-path-input"
            value={projectPath}
            onChange={(event) => setProjectPath(event.target.value)}
            className="h-9 w-full rounded-md border border-slate-700 bg-slate-900 px-3 font-mono text-sm text-white outline-none transition placeholder:text-slate-500 focus:border-teal-500 focus:ring-2 focus:ring-teal-500/30"
            placeholder="/workspace/project/coding"
          />
          <button
            data-testid="project-save-button"
            type="submit"
            disabled={connectionState !== 'open' || !projectName.trim() || !projectPath.trim()}
            className="mt-3 inline-flex h-9 w-full cursor-pointer items-center justify-center gap-2 rounded-md bg-teal-600 px-3 text-sm font-medium text-white transition hover:bg-teal-500 focus:outline-none focus:ring-2 focus:ring-teal-400 disabled:cursor-not-allowed disabled:bg-slate-700 disabled:text-slate-400"
          >
            <Save className="h-4 w-4" />
            保存
          </button>
          {errorText ? <p className="mt-3 text-sm text-rose-300">{errorText}</p> : null}
        </form>

        <div className="min-h-0 flex-1 overflow-y-auto px-2 py-3" data-testid="project-list">
          {projects.length === 0 ? (
            <div className="rounded-md border border-dashed border-slate-700 px-3 py-8 text-center text-sm text-slate-500">
              还没有 Project
            </div>
          ) : (
            projects.map((project) => (
              <div
                key={project.id}
                className={`mb-1 rounded-md border px-2 py-2 transition ${
                  project.id === selectedProjectId
                    ? 'border-teal-500 bg-teal-500/10'
                    : 'border-transparent hover:border-slate-700 hover:bg-slate-900'
                }`}
              >
                <button
                  type="button"
                  onClick={() => selectProject(project)}
                  className="flex w-full cursor-pointer items-start gap-2 text-left"
                >
                  <Folder className="mt-0.5 h-4 w-4 shrink-0 text-teal-400" />
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-sm font-medium text-slate-100">{project.name}</span>
                    <span className="block truncate font-mono text-xs text-slate-500">{project.path}</span>
                  </span>
                </button>
                <div className="mt-2 flex justify-end gap-1">
                  <button
                    type="button"
                    onClick={() => resetProjectForm(project)}
                    className="inline-flex h-7 w-7 cursor-pointer items-center justify-center rounded-md text-slate-400 transition hover:bg-slate-800 hover:text-white"
                    aria-label="编辑 Project"
                  >
                    <Pencil className="h-3.5 w-3.5" />
                  </button>
                  <button
                    type="button"
                    onClick={() => deleteProject(project)}
                    className="inline-flex h-7 w-7 cursor-pointer items-center justify-center rounded-md text-slate-400 transition hover:bg-rose-500/15 hover:text-rose-300"
                    aria-label="删除 Project"
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </button>
                </div>
              </div>
            ))
          )}
        </div>
      </aside>

      <section className="flex min-h-screen min-w-0 flex-col">
        <header className="flex min-h-16 items-center justify-between border-b border-slate-200 bg-white px-4">
          <div className="min-w-0">
            <h2 className="truncate text-lg font-semibold">{selectedProject?.name ?? '选择或创建 Project'}</h2>
            <p className="truncate font-mono text-xs text-slate-500">{selectedProject?.path ?? 'Project 会作为 Claude 的工作目录'}</p>
          </div>
          <button
            data-testid="chat-new-button"
            type="button"
            onClick={createChat}
            disabled={!activeProjectId || connectionState !== 'open'}
            className="inline-flex h-9 cursor-pointer items-center justify-center gap-2 rounded-md bg-slate-950 px-3 text-sm font-medium text-white transition hover:bg-slate-800 focus:outline-none focus:ring-2 focus:ring-slate-400 disabled:cursor-not-allowed disabled:bg-slate-300"
          >
            <Plus className="h-4 w-4" />
            新建聊天
          </button>
        </header>

        <div className="flex min-h-12 items-center gap-2 overflow-x-auto border-b border-slate-200 bg-slate-50 px-3" data-testid="chat-tabs">
          {projectChats.length === 0 ? (
            <span className="text-sm text-slate-500">当前 Project 还没有聊天页</span>
          ) : (
            projectChats.map((chat) => (
              <button
                key={chat.id}
                type="button"
                onClick={() => selectChat(chat)}
                className={`inline-flex h-9 shrink-0 cursor-pointer items-center gap-2 rounded-md border px-3 text-sm transition ${
                  chat.id === selectedChat?.id
                    ? 'border-slate-950 bg-white text-slate-950'
                    : 'border-slate-200 bg-transparent text-slate-600 hover:border-slate-400 hover:text-slate-950'
                }`}
              >
                <MessageSquare className="h-4 w-4" />
                <span>{chat.title}</span>
                {chat.status === 'running' ? <span className="h-2 w-2 rounded-full bg-orange-500" /> : null}
              </button>
            ))
          )}
        </div>

        <div className="min-h-0 flex-1 overflow-y-auto px-4 py-5" data-testid="message-log" aria-live="polite">
          {!selectedChat ? (
            <div className="mx-auto mt-20 max-w-md rounded-md border border-dashed border-slate-300 bg-white px-5 py-10 text-center text-sm text-slate-500">
              创建聊天页后开始输入 prompt
            </div>
          ) : selectedChat.messages.length === 0 ? (
            <div className="mx-auto mt-20 max-w-md rounded-md border border-dashed border-slate-300 bg-white px-5 py-10 text-center text-sm text-slate-500">
              这个聊天页还没有消息
            </div>
          ) : (
            <div className="mx-auto flex max-w-4xl flex-col gap-4">
              {selectedChat.messages.map((message) => (
                <article
                  key={message.id}
                  className={`rounded-md border p-4 ${
                    message.role === 'user'
                      ? 'border-teal-200 bg-teal-50'
                      : message.role === 'system'
                        ? 'border-rose-200 bg-rose-50'
                        : 'border-slate-200 bg-white'
                  }`}
                >
                  <div className="mb-2 flex items-center justify-between gap-3">
                    <span className="text-sm font-semibold text-slate-800">
                      {message.role === 'user' ? '你' : message.role === 'assistant' ? 'Claude' : '系统'}
                    </span>
                    <span className="inline-flex items-center gap-2 text-xs text-slate-500">
                      {message.status === 'streaming' ? <Loader2 className="h-3.5 w-3.5 animate-spin text-orange-500" /> : null}
                      {message.status === 'stopped' ? '已停止' : message.status === 'error' ? '失败' : formatTime(message.updatedAt)}
                    </span>
                  </div>
                  <pre className="whitespace-pre-wrap break-words font-sans text-sm leading-6 text-slate-800">
                    {message.text || (message.status === 'streaming' ? '正在等待输出...' : '')}
                  </pre>
                </article>
              ))}
            </div>
          )}
        </div>

        <form className="border-t border-slate-200 bg-white px-4 py-4" onSubmit={submitComposer}>
          <div className="mx-auto flex max-w-4xl items-end gap-3">
            <div className="min-w-0 flex-1">
              <label htmlFor="message-input" className="sr-only">
                Prompt
              </label>
              <textarea
                id="message-input"
                data-testid="message-input"
                value={composerValue}
                onChange={(event) => setComposerValue(event.target.value)}
                onKeyDown={handleComposerKeyDown}
                disabled={!selectedChat || connectionState !== 'open'}
                rows={1}
                className="max-h-40 min-h-12 w-full resize-none rounded-md border border-slate-300 bg-white px-3 py-3 text-sm leading-5 text-slate-950 outline-none transition placeholder:text-slate-400 focus:border-teal-600 focus:ring-2 focus:ring-teal-100 disabled:cursor-not-allowed disabled:bg-slate-100"
                placeholder={selectedChat ? '输入 prompt，Enter 发送，Shift+Enter 换行' : '先创建聊天页'}
              />
            </div>
            <button
              data-testid="send-button"
              type="submit"
              disabled={!selectedChat || connectionState !== 'open' || (!composerValue.trim() && !isRunning)}
              className={`inline-flex h-12 min-w-24 cursor-pointer items-center justify-center gap-2 rounded-md px-4 text-sm font-semibold text-white transition focus:outline-none focus:ring-2 focus:ring-offset-2 disabled:cursor-not-allowed disabled:bg-slate-300 ${
                isRunning ? 'bg-orange-600 hover:bg-orange-500 focus:ring-orange-300' : 'bg-teal-600 hover:bg-teal-500 focus:ring-teal-300'
              }`}
            >
              {isRunning ? <Square className="h-4 w-4" fill="currentColor" /> : <ArrowUp className="h-4 w-4" />}
              {isRunning ? '停止' : '发送'}
            </button>
          </div>
          {selectedChat?.agentSessionId ? (
            <div className="mx-auto mt-2 flex max-w-4xl items-center gap-2 text-xs text-slate-500">
              <Clock3 className="h-3.5 w-3.5" />
              <span className="truncate font-mono">session {selectedChat.agentSessionId}</span>
            </div>
          ) : null}
        </form>
      </section>
    </main>
  )
}

export default App
