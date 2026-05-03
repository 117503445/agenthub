import { FormEvent, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Loader2, Wifi, WifiOff } from 'lucide-react'
import { getWebSocketUrl, sendClientMessage, type ServerMessage } from './lib/ws'
import { AgentSettingsPage } from './components/AgentSettingsPage'
import { AppSidebar } from './components/AppSidebar'
import { ChatWorkspace } from './components/ChatWorkspace'
import {
  chatHasStarted,
  defaultModelForProvider,
  defaultReasoningForModel,
  fallbackAgentProviders,
  normalizeChat,
} from './lib/agent'
import { chatVisualStatus, mergeProjectVisualStatus, sortByCreatedAt, upsertById } from './lib/chat'
import { parseHashRoute, updateHashRoute, updateSettingsHashRoute } from './lib/routes'
import type {
  AgentProvider,
  AgentProvidersChangedPayload,
  AgentSkillsChangedPayload,
  AgentSkillOption,
  AgentStatusPayload,
  Chat,
  ChatChangedPayload,
  ChatDeletedPayload,
  ChatMessage,
  ChatMessageDeltaPayload,
  ChatMessageDonePayload,
  ChatTerminalIndicator,
  ChatVisualStatus,
  ConnectionState,
  Project,
  ProjectChangedPayload,
  ProjectDeletedPayload,
  SnapshotPayload,
} from './types'

// App 渲染 project 和 agent 聊天主界面。
function App() {
  const wsRef = useRef<WebSocket | null>(null)
  const pendingCreatedChatProjectIdRef = useRef('')
  const chatsRef = useRef<Chat[]>([])
  const [connectionState, setConnectionState] = useState<ConnectionState>('connecting')
  const [hostname, setHostname] = useState('')
  const [projects, setProjects] = useState<Project[]>([])
  const [chats, setChats] = useState<Chat[]>([])
  const [agentProviders, setAgentProviders] = useState(fallbackAgentProviders)
  const [agentSkills, setAgentSkills] = useState<AgentSkillOption[]>([])
  const [chatIndicators, setChatIndicators] = useState<Record<string, ChatTerminalIndicator>>({})
  const [routeView, setRouteView] = useState<'chat' | 'settings'>(() => parseHashRoute().view)
  const [selectedProjectId, setSelectedProjectId] = useState(() => parseHashRoute().projectId)
  const [selectedChatId, setSelectedChatId] = useState(() => parseHashRoute().chatId)
  const [projectFormId, setProjectFormId] = useState('')
  const [projectPath, setProjectPath] = useState('')
  const [projectDialogOpen, setProjectDialogOpen] = useState(false)
  const [newClaudeModelID, setNewClaudeModelID] = useState('')
  const [newClaudeModelLabel, setNewClaudeModelLabel] = useState('')
  const [composerValues, setComposerValues] = useState<Record<string, string>>({})
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
  const selectedComposerValue = selectedChat ? (composerValues[selectedChat.id] ?? '') : ''
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

  // pruneComposerValues 使用 chatIds 参数移除已经不存在的聊天页草稿。
  const pruneComposerValues = useCallback((chatIds: Set<string>) => {
    setComposerValues((current) => {
      const next: Record<string, string> = {}
      for (const [chatId, value] of Object.entries(current)) {
        if (chatIds.has(chatId)) {
          next[chatId] = value
        }
      }
      return next
    })
  }, [])

  // removeComposerValues 使用 chatIds 参数清理指定聊天页草稿。
  const removeComposerValues = useCallback((chatIds: string[]) => {
    setComposerValues((current) => {
      let changed = false
      const next = { ...current }
      for (const chatId of chatIds) {
        if (chatId in next) {
          delete next[chatId]
          changed = true
        }
      }
      return changed ? next : current
    })
  }, [])

  // updateSelectedComposerValue 使用 value 参数更新当前聊天页草稿。
  const updateSelectedComposerValue = useCallback((value: string) => {
    const chatId = selectedChat?.id
    if (!chatId) {
      return
    }
    setComposerValues((current) => {
      if (value === '') {
        if (!(chatId in current)) {
          return current
        }
        const next = { ...current }
        delete next[chatId]
        return next
      }
      if (current[chatId] === value) {
        return current
      }
      return { ...current, [chatId]: value }
    })
  }, [selectedChat?.id])

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
    if (!hasSnapshot) {
      return
    }
    const nextProjectId =
      selectedProject && selectedProject.id !== selectedProjectId ? selectedProject.id : !selectedProject && selectedProjectId ? '' : null
    if (nextProjectId === null) {
      return
    }
    const timer = window.setTimeout(() => {
      setSelectedProjectId(nextProjectId)
    }, 0)
    return () => window.clearTimeout(timer)
  }, [hasSnapshot, selectedProject, selectedProjectId])

  useEffect(() => {
    if (!hasSnapshot) {
      return
    }
    const nextChatId = selectedChat && selectedChat.id !== selectedChatId ? selectedChat.id : !selectedChat && selectedChatId ? '' : null
    if (nextChatId === null) {
      return
    }
    const timer = window.setTimeout(() => {
      setSelectedChatId(nextChatId)
    }, 0)
    return () => window.clearTimeout(timer)
  }, [hasSnapshot, selectedChat, selectedChatId])

  useEffect(() => {
    if (!hasSnapshot || routeView !== 'chat') {
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
        setAgentSkills(payload.agentSkills ?? [])
        setProjects(nextProjects)
        setChats(nextChats)
        pruneComposerValues(nextChatIds)
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
        removeComposerValues(payload.chatIds)
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
        removeComposerValues([payload.id])
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
      if (message.type === 'agent.skills.changed') {
        const payload = message.payload as AgentSkillsChangedPayload
        setAgentSkills(payload.agentSkills ?? [])
        return
      }
      if (message.type === 'error') {
        const payload = message.payload as { message?: string }
        setErrorText(payload.message ?? '服务端错误')
      }
    },
    [clearChatIndicator, markChatIndicator, pruneComposerValues, removeComposerValues, resetProjectForm],
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
    const prompt = selectedComposerValue.trim()
    if (!prompt && isRunning) {
      sendClientMessage(wsRef.current, 'chat.stop', { chatId: selectedChat.id })
      return
    }
    if (!prompt) {
      return
    }
    sendClientMessage(wsRef.current, 'chat.send', { chatId: selectedChat.id, prompt })
    updateSelectedComposerValue('')
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
      <AppSidebar
        connectionState={connectionState}
        connectionIcon={connectionIcon}
        hostname={hostname}
        projects={projects}
        selectedProjectId={selectedProjectId}
        projectVisualStatuses={projectVisualStatuses}
        projectDialogOpen={projectDialogOpen}
        projectPath={projectPath}
        errorText={errorText}
        routeView={routeView}
        onProjectPathChange={setProjectPath}
        onProjectSave={saveProject}
        onProjectDialogOpen={openProjectDialog}
        onProjectDialogClose={closeProjectDialog}
        onProjectSelect={selectProject}
        onProjectDelete={deleteProject}
        onSettingsOpen={openAgentSettings}
      />

      <section className="flex h-full min-h-0 min-w-0 flex-col">
        {routeView === 'settings' ? (
          <AgentSettingsPage
            connectionState={connectionState}
            claudeCodeModels={claudeCodeModels}
            newClaudeModelID={newClaudeModelID}
            newClaudeModelLabel={newClaudeModelLabel}
            onBackToChat={backToChat}
            onModelIDChange={setNewClaudeModelID}
            onModelLabelChange={setNewClaudeModelLabel}
            onAddClaudeModel={addClaudeModel}
          />
        ) : (
          <ChatWorkspace
            selectedProject={selectedProject}
            selectedChat={selectedChat}
            projectChats={projectChats}
            activeProjectId={activeProjectId}
            connectionState={connectionState}
            chatIndicators={chatIndicators}
            composerValue={selectedComposerValue}
            copiedMessageId={copiedMessageId}
            isRunning={isRunning}
            agentProviders={agentProviders}
            agentSkills={agentSkills}
            selectedAgentProvider={selectedAgentProvider}
            selectedAgentModels={selectedAgentModels}
            selectedAgentModel={selectedAgentModel}
            selectedAgentModelOption={selectedAgentModelOption}
            selectedAgentReasoning={selectedAgentReasoning}
            agentControlsDisabled={agentControlsDisabled}
            providerLocked={providerLocked}
            modelControlsDisabled={modelControlsDisabled}
            onSelectChat={selectChat}
            onDeleteChat={deleteChat}
            onCreateChat={createChat}
            onClearChatIndicator={clearChatIndicator}
            onCopyMessage={(message) => void copyMessageText(message)}
            onComposerValueChange={updateSelectedComposerValue}
            onSubmitComposer={submitComposer}
            onChangeAgentProvider={changeAgentProvider}
            onChangeAgentModel={changeAgentModel}
            onChangeAgentReasoning={changeAgentReasoning}
          />
        )}
      </section>
    </main>
  )
}

export default App
