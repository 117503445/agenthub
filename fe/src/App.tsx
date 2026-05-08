import { FormEvent, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Loader2, LockKeyhole, Wifi, WifiOff } from 'lucide-react'
import { getWebSocketUrl, sendClientMessage, type ServerMessage } from './lib/ws'
import {
  clearStoredAgentHubToken,
  fetchAuthStatus,
  readStoredAgentHubToken,
  writeStoredAgentHubToken,
} from './lib/auth'
import { AgentSettingsPage } from './components/AgentSettingsPage'
import { AppSidebar } from './components/AppSidebar'
import { ChatWorkspace } from './components/ChatWorkspace'
import { Button } from './components/ui/button'
import { Input } from './components/ui/input'
import {
  chatHasStarted,
  defaultModelForProvider,
  defaultReasoningForModel,
  normalizeChat,
} from './lib/agent'
import { chatVisualStatus, mergeProjectVisualStatus, projectChatTimeline, sortByCreatedAt, sortProjects, upsertById } from './lib/chat'
import { parseHashRoute, updateHashRoute, updateSettingsHashRoute } from './lib/routes'
import type {
  AgentProvider,
  AgentProviderOption,
  AgentProfile,
  AgentProfilesChangedPayload,
  BackendEnvVar,
  AgentBuiltinProfileKind,
  AgentProvidersChangedPayload,
  AgentSkillsChangedPayload,
  AgentSkillOption,
  AgentStatusPayload,
  Chat,
  ChatChangedPayload,
  ChatDeletedPayload,
  ChatMessage,
  ChatScrollMemory,
  ChatTerminalIndicator,
  ChatTimelineAppendedPayload,
  ChatTimelinePayload,
  ChatTimelineRow,
  ChatVisualStatus,
  ComposerImageAttachment,
  ConnectionState,
  PlanApproval,
  Project,
  ProjectChangedPayload,
  ProjectDeletedPayload,
  ProjectsReorderedPayload,
  SnapshotPayload,
  ToolCall,
} from './types'

const minimumSendPendingMs = 800
const chatDeltaFlushDelayMs = 48
const draftSaveDebounceMs = 400

// normalizeChatSummary 使用 chat 参数生成前端摘要状态。
function normalizeChatSummary(chat: Chat) {
  return normalizeChat({ ...chat, messages: [], plan: undefined, timelineLoaded: false, timelineLoadedAt: undefined })
}

// mergeChatSummary 使用 current 和 summary 参数合并聊天摘要并保留已加载 timeline。
function mergeChatSummary(current: Chat[], summary: Chat) {
  const existing = current.find((chat) => chat.id === summary.id)
  const normalized = normalizeChatSummary(summary)
  if (!existing?.timelineLoaded) {
    return normalized
  }
  return {
    ...normalized,
    messages: existing.messages,
    plan: existing.plan,
    usage: existing.usage,
    timelineLoaded: existing.timelineLoaded,
    timelineLoadedAt: existing.timelineLoadedAt,
  }
}

// App 渲染 project 和 agent 聊天主界面。
function App() {
  const wsRef = useRef<WebSocket | null>(null)
  const pendingCreatedChatProjectIdRef = useRef('')
  const pendingSendChatIdsRef = useRef<Record<string, number>>({})
  const awaitingSendChatIdsRef = useRef<Record<string, true>>({})
  const sendPendingTimersRef = useRef<Record<string, number>>({})
  const chatsRef = useRef<Chat[]>([])
  const chatTimelineLoadingRef = useRef<Record<string, boolean>>({})
  const chatTimelineLoadedRef = useRef<Record<string, boolean>>({})
  const chatTimelineRowsRef = useRef<Record<string, ChatTimelineRow[]>>({})
  const chatTimelineCursorsRef = useRef<Record<string, { epoch: string; startSeq: number; endSeq: number }>>({})
  const chatTimelineHasOlderRef = useRef<Record<string, boolean>>({})
  const pendingChatTimelineRowsRef = useRef<Record<string, ChatTimelineRow[]>>({})
  const chatDeltaFlushTimerRef = useRef<number | null>(null)
  const pendingDraftValuesRef = useRef<Record<string, string>>({})
  const draftSaveTimersRef = useRef<Record<string, number>>({})
  const chatScrollMemoryRef = useRef<Record<string, ChatScrollMemory>>({})
  const projectSelectedChatIdsRef = useRef<Record<string, string>>({})
  const [authChecked, setAuthChecked] = useState(false)
  const [authRequired, setAuthRequired] = useState(false)
  const [authToken, setAuthToken] = useState(() => readStoredAgentHubToken())
  const [authTokenInput, setAuthTokenInput] = useState(() => readStoredAgentHubToken())
  const [authErrorText, setAuthErrorText] = useState('')
  const [connectionState, setConnectionState] = useState<ConnectionState>('connecting')
  const [hostname, setHostname] = useState('')
  const [backendVersion, setBackendVersion] = useState('')
  const [backendBuildTime, setBackendBuildTime] = useState('')
  const [projects, setProjects] = useState<Project[]>([])
  const [chats, setChats] = useState<Chat[]>([])
  const [agentProviders, setAgentProviders] = useState<AgentProviderOption[]>([])
  const [agentProfiles, setAgentProfiles] = useState<AgentProfile[]>([])
  const [backendEnv, setBackendEnv] = useState<BackendEnvVar[]>([])
  const [agentSkills, setAgentSkills] = useState<AgentSkillOption[]>([])
  const [chatTimelineHasOlder, setChatTimelineHasOlder] = useState<Record<string, boolean>>({})
  const [chatTimelineLoading, setChatTimelineLoading] = useState<Record<string, boolean>>({})
  const [chatIndicators, setChatIndicators] = useState<Record<string, ChatTerminalIndicator>>({})
  const [routeView, setRouteView] = useState<'chat' | 'settings'>(() => parseHashRoute().view)
  const [selectedProjectId, setSelectedProjectId] = useState(() => parseHashRoute().projectId)
  const [selectedChatId, setSelectedChatId] = useState(() => parseHashRoute().chatId)
  const [projectFormId, setProjectFormId] = useState('')
  const [projectPath, setProjectPath] = useState('')
  const [projectDialogOpen, setProjectDialogOpen] = useState(false)
  const [selectedSettingsProfileId, setSelectedSettingsProfileId] = useState('')
  const [newClaudeModelID, setNewClaudeModelID] = useState('')
  const [composerValues, setComposerValues] = useState<Record<string, string>>({})
  const [composerImages, setComposerImages] = useState<Record<string, ComposerImageAttachment[]>>({})
  const [planModes, setPlanModes] = useState<Record<string, boolean>>({})
  const [projectSelectedChatIds, setProjectSelectedChatIds] = useState<Record<string, string>>({})
  const [pendingSendChatIds, setPendingSendChatIds] = useState<Record<string, number>>({})
  const [awaitingSendChatIds, setAwaitingSendChatIds] = useState<Record<string, true>>({})
  const [chatSubmitErrors, setChatSubmitErrors] = useState<Record<string, string>>({})
  const [chatScrollBottomSignals, setChatScrollBottomSignals] = useState<Record<string, number>>({})
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
  const rememberedProjectChatId = activeProjectId ? (projectSelectedChatIds[activeProjectId] ?? '') : ''
  const selectedChat = useMemo(
    () =>
      projectChats.find((chat) => chat.id === selectedChatId) ??
      projectChats.find((chat) => chat.id === rememberedProjectChatId) ??
      projectChats[0] ??
      null,
    [projectChats, rememberedProjectChatId, selectedChatId],
  )
  // applyChatTimelineRows 使用 chatID 和 rows 参数投影聊天 timeline。
  const applyChatTimelineRows = useCallback((chatId: string, rows: ChatTimelineRow[], options?: { replace?: boolean }) => {
    const sortedRows = [...rows].sort((left, right) => left.seq - right.seq)
    const currentRows = options?.replace ? [] : (chatTimelineRowsRef.current[chatId] ?? [])
    const bySeq = new Map<number, ChatTimelineRow>()
    for (const row of currentRows) {
      bySeq.set(row.seq, row)
    }
    for (const row of sortedRows) {
      bySeq.set(row.seq, row)
    }
    const nextRows = Array.from(bySeq.values()).sort((left, right) => left.seq - right.seq)
    chatTimelineRowsRef.current = { ...chatTimelineRowsRef.current, [chatId]: nextRows }
    if (nextRows.length > 0) {
      chatTimelineCursorsRef.current[chatId] = {
        epoch: nextRows[0].epoch,
        startSeq: nextRows[0].seq,
        endSeq: nextRows[nextRows.length - 1].seq,
      }
    }
    setChats((current) =>
      current.map((chat) => (chat.id === chatId ? projectChatTimeline(chat, nextRows) : chat)),
    )
  }, [])

  // requestChatTimeline 使用参数向后端拉取聊天 timeline。
  const requestChatTimeline = useCallback(
    (chatId: string, direction: 'tail' | 'after' | 'before', cursor?: { epoch: string; seq: number }, limit = 200) => {
      if (!chatId) {
        return false
      }
      chatTimelineLoadingRef.current[chatId] = true
      setChatTimelineLoading((current) => ({ ...current, [chatId]: true }))
      const sent = sendClientMessage(wsRef.current, 'chat.timeline.fetch', {
        chatId,
        direction,
        cursor,
        limit,
      })
      if (!sent) {
        delete chatTimelineLoadingRef.current[chatId]
        setChatTimelineLoading((current) => {
          if (!current[chatId]) {
            return current
          }
          const next = { ...current }
          delete next[chatId]
          return next
        })
      }
      return sent
    },
    [],
  )

  // flushPendingChatTimelineRows 批量应用后端推送的 timeline 行。
  const flushPendingChatTimelineRows = useCallback(() => {
    if (chatDeltaFlushTimerRef.current !== null) {
      window.clearTimeout(chatDeltaFlushTimerRef.current)
      chatDeltaFlushTimerRef.current = null
    }
    const entries = Object.entries(pendingChatTimelineRowsRef.current)
    pendingChatTimelineRowsRef.current = {}
    if (entries.length === 0) {
      return
    }
    for (const [chatId, rows] of entries) {
      applyChatTimelineRows(chatId, rows)
    }
  }, [applyChatTimelineRows])
  // enqueueChatTimelineRow 使用 payload 参数把聊天 timeline 行放入短延迟队列。
  const enqueueChatTimelineRow = useCallback(
    (payload: ChatTimelineAppendedPayload) => {
      const cursor = chatTimelineCursorsRef.current[payload.chatId]
      const pendingRows = pendingChatTimelineRowsRef.current[payload.chatId] ?? []
      const pendingEndSeq = pendingRows.reduce((maxSeq, row) => Math.max(maxSeq, row.seq), cursor?.endSeq ?? 0)
      if (cursor) {
        if (payload.epoch !== cursor.epoch) {
          requestChatTimeline(payload.chatId, 'tail', undefined, 200)
          return
        }
        if (payload.row.seq <= cursor.endSeq || pendingRows.some((row) => row.seq === payload.row.seq)) {
          return
        }
        if (payload.row.seq > pendingEndSeq + 1) {
          requestChatTimeline(payload.chatId, 'after', { epoch: cursor.epoch, seq: cursor.endSeq }, 0)
          return
        }
      }
      pendingChatTimelineRowsRef.current[payload.chatId] = [...pendingRows, payload.row]
      if (chatDeltaFlushTimerRef.current !== null) {
        return
      }
      chatDeltaFlushTimerRef.current = window.setTimeout(flushPendingChatTimelineRows, chatDeltaFlushDelayMs)
    },
    [flushPendingChatTimelineRows, requestChatTimeline],
  )
  const selectedComposerValue = selectedChat ? (composerValues[selectedChat.id] ?? selectedChat.draftText ?? '') : ''
  const selectedComposerImages = selectedChat ? (composerImages[selectedChat.id] ?? []) : []
  const selectedPlanMode = selectedChat ? (planModes[selectedChat.id] ?? false) : false
  const selectedChatSending = selectedChat ? Boolean(pendingSendChatIds[selectedChat.id]) : false
  const selectedChatSendAwaiting = selectedChat ? Boolean(awaitingSendChatIds[selectedChat.id]) : false
  const selectedChatSubmitError = selectedChat ? (chatSubmitErrors[selectedChat.id] ?? '') : ''
  const selectedChatScrollBottomSignal = selectedChat ? (chatScrollBottomSignals[selectedChat.id] ?? 0) : 0
  const selectedChatTimelineHasOlder = selectedChat ? (chatTimelineHasOlder[selectedChat.id] ?? false) : false
  const selectedChatTimelineLoading = selectedChat ? (chatTimelineLoading[selectedChat.id] ?? false) : false
  const isRunning = selectedChat?.status === 'running'
  const selectedAgentProvider = selectedChat?.agentProvider ?? 'claude-code'
  const chatBoundAgentProvider = selectedChat?.agentProfile?.id
    ? { id: selectedChat.agentProfile.id, label: selectedChat.agentProfile.label || selectedChat.agentProfile.id, models: selectedChat.agentProfile.models }
    : null
  const visibleAgentProviders =
    chatBoundAgentProvider && !agentProviders.some((provider) => provider.id === chatBoundAgentProvider.id)
      ? [...agentProviders, chatBoundAgentProvider]
      : agentProviders
  const selectedAgentModels = visibleAgentProviders.find((provider) => provider.id === selectedAgentProvider)?.models ?? []
  const selectedAgentModel = selectedChat?.agentModel ?? defaultModelForProvider(visibleAgentProviders, selectedAgentProvider)
  const selectedAgentModelOption = selectedAgentModels.find((model) => model.id === selectedAgentModel) ?? null
  const selectedAgentReasoning =
    selectedChat?.agentReasoning || defaultReasoningForModel(visibleAgentProviders, selectedAgentProvider, selectedAgentModel)
  const providerLocked = chatHasStarted(selectedChat) || isRunning
  const agentControlsDisabled = !selectedChat || connectionState !== 'open'
  const modelControlsDisabled = agentControlsDisabled || isRunning
  const selectedSettingsProfile = agentProfiles.find((profile) => profile.id === selectedSettingsProfileId) ?? agentProfiles[0] ?? null
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

  useEffect(() => {
    return () => {
      for (const timer of Object.values(sendPendingTimersRef.current)) {
        window.clearTimeout(timer)
      }
      sendPendingTimersRef.current = {}
      for (const timer of Object.values(draftSaveTimersRef.current)) {
        window.clearTimeout(timer)
      }
      draftSaveTimersRef.current = {}
      pendingDraftValuesRef.current = {}
      if (chatDeltaFlushTimerRef.current !== null) {
        window.clearTimeout(chatDeltaFlushTimerRef.current)
        chatDeltaFlushTimerRef.current = null
      }
      pendingChatTimelineRowsRef.current = {}
    }
  }, [])

  useEffect(() => {
    let stopped = false

    // loadAuthStatus 读取服务端鉴权状态，并决定是否需要 token。
    const loadAuthStatus = async () => {
      try {
        const status = await fetchAuthStatus()
        if (stopped) {
          return
        }
        setAuthRequired(status.tokenRequired)
        setAuthChecked(true)
        if (!status.tokenRequired) {
          clearStoredAgentHubToken()
          setAuthToken('')
          setAuthTokenInput('')
        } else if (!readStoredAgentHubToken()) {
          setConnectionState('closed')
        }
      } catch {
        if (stopped) {
          return
        }
        setAuthRequired(true)
        setAuthToken('')
        setAuthErrorText('无法读取鉴权状态')
        setAuthChecked(true)
        setConnectionState('error')
      }
    }

    void loadAuthStatus()
    return () => {
      stopped = true
    }
  }, [])

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
    for (const chatId of Object.keys(pendingDraftValuesRef.current)) {
      if (chatIds.has(chatId)) {
        continue
      }
      if (draftSaveTimersRef.current[chatId]) {
        window.clearTimeout(draftSaveTimersRef.current[chatId])
        delete draftSaveTimersRef.current[chatId]
      }
      delete pendingDraftValuesRef.current[chatId]
    }
    setComposerValues((current) => {
      const next: Record<string, string> = {}
      for (const [chatId, value] of Object.entries(current)) {
        if (chatIds.has(chatId)) {
          next[chatId] = value
        }
      }
      return next
    })
    setComposerImages((current) => {
      const next: Record<string, ComposerImageAttachment[]> = {}
      for (const [chatId, images] of Object.entries(current)) {
        if (chatIds.has(chatId)) {
          next[chatId] = images
        }
      }
      return next
    })
    setPlanModes((current) => {
      const next: Record<string, boolean> = {}
      for (const [chatId, enabled] of Object.entries(current)) {
        if (chatIds.has(chatId)) {
          next[chatId] = enabled
        }
      }
      return next
    })
  }, [])

  // removeComposerValues 使用 chatIds 参数清理指定聊天页草稿。
  const removeComposerValues = useCallback((chatIds: string[]) => {
    for (const chatId of chatIds) {
      if (draftSaveTimersRef.current[chatId]) {
        window.clearTimeout(draftSaveTimersRef.current[chatId])
        delete draftSaveTimersRef.current[chatId]
      }
      delete pendingDraftValuesRef.current[chatId]
    }
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
    setComposerImages((current) => {
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
    setPlanModes((current) => {
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

  // pruneChatScrollMemory 使用 chatIds 参数清理已不存在聊天页的滚动位置。
  const pruneChatScrollMemory = useCallback((chatIds: Set<string>) => {
    const next: Record<string, ChatScrollMemory> = {}
    for (const [chatId, memory] of Object.entries(chatScrollMemoryRef.current)) {
      if (chatIds.has(chatId)) {
        next[chatId] = memory
      }
    }
    chatScrollMemoryRef.current = next
  }, [])

  // removeChatScrollMemory 使用 chatIds 参数移除指定聊天页的滚动位置。
  const removeChatScrollMemory = useCallback((chatIds: string[]) => {
    for (const chatId of chatIds) {
      delete chatScrollMemoryRef.current[chatId]
    }
  }, [])

  // removeChatTimelineState 使用 chatIds 参数移除指定聊天页 timeline 状态。
  const removeChatTimelineState = useCallback((chatIds: string[]) => {
    for (const chatId of chatIds) {
      delete chatTimelineRowsRef.current[chatId]
      delete chatTimelineCursorsRef.current[chatId]
      delete chatTimelineLoadedRef.current[chatId]
      delete chatTimelineLoadingRef.current[chatId]
      delete chatTimelineHasOlderRef.current[chatId]
    }
    setChatTimelineHasOlder((current) => {
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
    setChatTimelineLoading((current) => {
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

  // rememberProjectSelectedChat 使用 projectId 和 chatId 参数记录 project 最近选中的聊天页。
  const rememberProjectSelectedChat = useCallback((projectId: string, chatId: string) => {
    if (!projectId || !chatId) {
      return
    }
    projectSelectedChatIdsRef.current[projectId] = chatId
    setProjectSelectedChatIds((current) => {
      if (current[projectId] === chatId) {
        return current
      }
      return { ...current, [projectId]: chatId }
    })
  }, [])

  // findRememberedProjectChatId 使用 projectId 参数查找 project 应恢复的聊天页。
  const findRememberedProjectChatId = useCallback(
    (projectId: string) => {
      if (!projectId) {
        return ''
      }
      const rememberedChatId = projectSelectedChatIdsRef.current[projectId] ?? ''
      if (rememberedChatId && chats.some((chat) => chat.id === rememberedChatId && chat.projectId === projectId)) {
        return rememberedChatId
      }
      return sortByCreatedAt(chats.filter((chat) => chat.projectId === projectId))[0]?.id ?? ''
    },
    [chats],
  )

  // pruneProjectSelectedChats 使用 nextChats 参数清理已不存在的聊天页记忆。
  const pruneProjectSelectedChats = useCallback((nextChats: Chat[]) => {
    const chatProjectIds = new Map<string, string>()
    for (const chat of nextChats) {
      chatProjectIds.set(chat.id, chat.projectId)
    }
    const next: Record<string, string> = {}
    for (const [projectId, chatId] of Object.entries(projectSelectedChatIdsRef.current)) {
      if (chatProjectIds.get(chatId) === projectId) {
        next[projectId] = chatId
      }
    }
    projectSelectedChatIdsRef.current = next
    setProjectSelectedChatIds(next)
  }, [])

  // forgetProjectSelectedChat 使用 projectId 和 chatId 参数移除被删除聊天页的记忆。
  const forgetProjectSelectedChat = useCallback((projectId: string, chatId: string) => {
    if (!projectId || !chatId) {
      return
    }
    if (projectSelectedChatIdsRef.current[projectId] === chatId) {
      delete projectSelectedChatIdsRef.current[projectId]
      setProjectSelectedChatIds((current) => {
        if (current[projectId] !== chatId) {
          return current
        }
        const next = { ...current }
        delete next[projectId]
        return next
      })
    }
  }, [])

  useEffect(() => {
    if (!selectedChat) {
      return
    }
    const timer = window.setTimeout(() => {
      rememberProjectSelectedChat(selectedChat.projectId, selectedChat.id)
    }, 0)
    return () => window.clearTimeout(timer)
  }, [rememberProjectSelectedChat, selectedChat])

  // readChatScrollMemory 使用 chatId 参数读取聊天页滚动位置。
  const readChatScrollMemory = useCallback((chatId: string) => chatScrollMemoryRef.current[chatId], [])

  // saveChatScrollMemory 使用 chatId 和 memory 参数保存聊天页滚动位置。
  const saveChatScrollMemory = useCallback((chatId: string, memory: ChatScrollMemory) => {
    chatScrollMemoryRef.current[chatId] = memory
  }, [])

  // setChatSubmitError 使用 chatId 和 message 参数更新聊天输入提交错误。
  const setChatSubmitError = useCallback((chatId: string, message: string) => {
    if (!chatId) {
      return
    }
    setChatSubmitErrors((current) => {
      if (!message) {
        if (!(chatId in current)) {
          return current
        }
        const next = { ...current }
        delete next[chatId]
        return next
      }
      if (current[chatId] === message) {
        return current
      }
      return { ...current, [chatId]: message }
    })
  }, [])

  // setChatSendPending 使用 chatId 参数标记聊天输入正在提交。
  const setChatSendPending = useCallback((chatId: string) => {
    if (!chatId) {
      return
    }
    if (sendPendingTimersRef.current[chatId]) {
      window.clearTimeout(sendPendingTimersRef.current[chatId])
      delete sendPendingTimersRef.current[chatId]
    }
    const startedAt = Date.now()
    const next = { ...pendingSendChatIdsRef.current, [chatId]: startedAt }
    pendingSendChatIdsRef.current = next
    setPendingSendChatIds(next)
    const nextAwaiting = { ...awaitingSendChatIdsRef.current, [chatId]: true as const }
    awaitingSendChatIdsRef.current = nextAwaiting
    setAwaitingSendChatIds(nextAwaiting)
  }, [])

  // clearChatSendAwaiting 使用 chatId 参数清除等待服务端确认状态。
  const clearChatSendAwaiting = useCallback((chatId: string) => {
    if (!awaitingSendChatIdsRef.current[chatId]) {
      return
    }
    const next = { ...awaitingSendChatIdsRef.current }
    delete next[chatId]
    awaitingSendChatIdsRef.current = next
    setAwaitingSendChatIds(next)
  }, [])

  // clearChatSendPending 使用 chatId 和 immediate 参数清除聊天输入提交中状态。
  const clearChatSendPending = useCallback((chatId: string, immediate = false) => {
    const startedAt = pendingSendChatIdsRef.current[chatId]
    if (!startedAt) {
      return
    }
    if (sendPendingTimersRef.current[chatId]) {
      window.clearTimeout(sendPendingTimersRef.current[chatId])
      delete sendPendingTimersRef.current[chatId]
    }
    const applyClear = () => {
      delete sendPendingTimersRef.current[chatId]
      if (!pendingSendChatIdsRef.current[chatId]) {
        return
      }
      const next = { ...pendingSendChatIdsRef.current }
      delete next[chatId]
      pendingSendChatIdsRef.current = next
      setPendingSendChatIds(next)
    }
    const remainingMs = Math.max(0, minimumSendPendingMs - (Date.now() - startedAt))
    if (!immediate && remainingMs > 0) {
      sendPendingTimersRef.current[chatId] = window.setTimeout(applyClear, remainingMs)
      return
    }
    applyClear()
  }, [])

  // requestChatScrollToBottom 使用 chatId 参数请求聊天页滚动到底部。
  const requestChatScrollToBottom = useCallback((chatId: string) => {
    if (!chatId) {
      return
    }
    setChatScrollBottomSignals((current) => ({
      ...current,
      [chatId]: (current[chatId] ?? 0) + 1,
    }))
  }, [])

  // sendChatDraft 使用 chatId 和 text 参数向后端保存草稿。
  const sendChatDraft = useCallback((chatId: string, text: string) => sendClientMessage(wsRef.current, 'chat.draft.update', { chatId, text }), [])

  // cancelChatDraftSave 使用 chatId 参数取消指定聊天页待发送草稿。
  const cancelChatDraftSave = useCallback((chatId: string) => {
    if (draftSaveTimersRef.current[chatId]) {
      window.clearTimeout(draftSaveTimersRef.current[chatId])
      delete draftSaveTimersRef.current[chatId]
    }
    delete pendingDraftValuesRef.current[chatId]
  }, [])

  // flushChatDraft 使用 chatId 参数立即发送待保存草稿，未传入时发送全部待保存草稿。
  const flushChatDraft = useCallback(
    (chatId?: string) => {
      const chatIds = chatId ? [chatId] : Object.keys(pendingDraftValuesRef.current)
      for (const id of chatIds) {
        if (draftSaveTimersRef.current[id]) {
          window.clearTimeout(draftSaveTimersRef.current[id])
          delete draftSaveTimersRef.current[id]
        }
        if (!(id in pendingDraftValuesRef.current)) {
          continue
        }
        const text = pendingDraftValuesRef.current[id]
        if (sendChatDraft(id, text)) {
          delete pendingDraftValuesRef.current[id]
        }
      }
    },
    [sendChatDraft],
  )

  // scheduleChatDraftSave 使用 chatId 和 text 参数延迟保存聊天页草稿。
  const scheduleChatDraftSave = useCallback(
    (chatId: string, text: string) => {
      pendingDraftValuesRef.current[chatId] = text
      if (draftSaveTimersRef.current[chatId]) {
        window.clearTimeout(draftSaveTimersRef.current[chatId])
      }
      draftSaveTimersRef.current[chatId] = window.setTimeout(() => {
        delete draftSaveTimersRef.current[chatId]
        flushChatDraft(chatId)
      }, draftSaveDebounceMs)
    },
    [flushChatDraft],
  )

  // clearChatComposerDraft 使用 chatId 参数清空指定聊天页输入草稿。
  const clearChatComposerDraft = useCallback((chatId: string) => {
    if (!chatId) {
      return
    }
    cancelChatDraftSave(chatId)
    setComposerValues((current) => {
      if (current[chatId] === '') {
        return current
      }
      return { ...current, [chatId]: '' }
    })
    setComposerImages((current) => {
      if (!(chatId in current)) {
        return current
      }
      const next = { ...current }
      delete next[chatId]
      return next
    })
  }, [cancelChatDraftSave])

  // finishChatSendSuccess 使用 chatId 参数处理聊天输入提交成功。
  const finishChatSendSuccess = useCallback(
    (chatId: string) => {
      if (!pendingSendChatIdsRef.current[chatId] || !awaitingSendChatIdsRef.current[chatId]) {
        return
      }
      clearChatSendAwaiting(chatId)
      clearChatSendPending(chatId)
      setChatSubmitError(chatId, '')
      clearChatComposerDraft(chatId)
      requestChatScrollToBottom(chatId)
    },
    [clearChatComposerDraft, clearChatSendAwaiting, clearChatSendPending, requestChatScrollToBottom, setChatSubmitError],
  )

  // failPendingChatSends 使用 message 参数处理当前等待中的聊天输入提交失败。
  const failPendingChatSends = useCallback(
    (message: string) => {
      const pendingChatIds = Object.keys(pendingSendChatIdsRef.current)
      for (const chatId of pendingChatIds) {
        clearChatSendAwaiting(chatId)
        clearChatSendPending(chatId)
        setChatSubmitError(chatId, message)
      }
    },
    [clearChatSendAwaiting, clearChatSendPending, setChatSubmitError],
  )

  // updateSelectedComposerValue 使用 value 参数更新当前聊天页草稿。
  const updateSelectedComposerValue = useCallback((value: string) => {
    const chatId = selectedChat?.id
    if (!chatId) {
      return
    }
    setChatSubmitError(chatId, '')
    setComposerValues((current) => {
      if (current[chatId] === value) {
        return current
      }
      return { ...current, [chatId]: value }
    })
    scheduleChatDraftSave(chatId, value)
  }, [scheduleChatDraftSave, selectedChat?.id, setChatSubmitError])

  // updateSelectedComposerImages 使用 images 参数更新当前聊天页图片附件草稿。
  const updateSelectedComposerImages = useCallback(
    (images: ComposerImageAttachment[]) => {
      const chatId = selectedChat?.id
      if (!chatId) {
        return
      }
      setChatSubmitError(chatId, '')
      setComposerImages((current) => {
        if (images.length === 0) {
          if (!(chatId in current)) {
            return current
          }
          const next = { ...current }
          delete next[chatId]
          return next
        }
        return { ...current, [chatId]: images }
      })
    },
    [selectedChat?.id, setChatSubmitError],
  )

  // updateSelectedPlanMode 使用 enabled 参数更新当前聊天页 plan 模式。
  const updateSelectedPlanMode = useCallback(
    (enabled: boolean) => {
      const chatId = selectedChat?.id
      if (!chatId) {
        return
      }
      setPlanModes((current) => {
        if (!enabled) {
          if (!(chatId in current)) {
            return current
          }
          const next = { ...current }
          delete next[chatId]
          return next
        }
        return { ...current, [chatId]: true }
      })
    },
    [selectedChat?.id],
  )

  useEffect(() => {
    return () => {
      if (selectedChat?.id) {
        flushChatDraft(selectedChat.id)
      }
    }
  }, [flushChatDraft, selectedChat?.id])

  useEffect(() => {
    const flushBeforeUnload = () => flushChatDraft()
    window.addEventListener('beforeunload', flushBeforeUnload)
    return () => window.removeEventListener('beforeunload', flushBeforeUnload)
  }, [flushChatDraft])

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

  useEffect(() => {
    if (!hasSnapshot || routeView !== 'chat' || connectionState !== 'open' || !selectedChat?.id) {
      return
    }
    if (chatTimelineLoadedRef.current[selectedChat.id] || chatTimelineLoadingRef.current[selectedChat.id]) {
      return
    }
    requestChatTimeline(selectedChat.id, 'tail', undefined, 200)
  }, [connectionState, hasSnapshot, requestChatTimeline, routeView, selectedChat?.id])

  // notifyAgentCompletion 使用 message 参数发送 agent 完成通知。
  const notifyAgentCompletion = useCallback((message: ChatMessage) => {
    if (!('Notification' in window) || Notification.permission !== 'granted') {
      return
    }
    if (message.role !== 'assistant') {
      return
    }
    if (message.status === 'stopped') {
      return
    }
    const title = message.status === 'error' ? 'Agent 回复失败' : 'Agent 回复完成'
    const body = message.text.trim().replace(/\s+/g, ' ').slice(0, 120)
    new Notification(title, { body: body || '任务已结束' })
  }, [])

  // applyServerMessage 使用 message 参数把已通过 timeline 游标校验的服务端事件归并到前端状态。
  const applyServerMessage = useCallback(
    (message: ServerMessage) => {
      setHostname(message.hostname || window.location.hostname || 'unknown')
      setBackendVersion(message.version || '')
      setBackendBuildTime(message.buildTime || '')
      if (message.type === 'state.snapshot') {
        const payload = message.payload as SnapshotPayload
        const nextProjects = sortProjects(payload.projects ?? [])
        const nextChats = sortByCreatedAt((payload.chats ?? []).map(normalizeChatSummary))
        const nextChatIds = new Set(nextChats.map((chat) => chat.id))
        chatTimelineLoadingRef.current = {}
        chatTimelineLoadedRef.current = {}
        chatTimelineRowsRef.current = {}
        chatTimelineCursorsRef.current = {}
        chatTimelineHasOlderRef.current = {}
        pendingChatTimelineRowsRef.current = {}
        setChatTimelineHasOlder({})
        setChatTimelineLoading({})
        setAgentProviders(payload.agentProviders ?? [])
        setAgentProfiles(payload.agentProfiles ?? [])
        setBackendEnv(payload.backendEnv ?? [])
        setAgentSkills(payload.agentSkills ?? [])
        setProjects(nextProjects)
        setChats(nextChats)
        pruneComposerValues(nextChatIds)
        pruneChatScrollMemory(nextChatIds)
        pruneProjectSelectedChats(nextChats)
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
        const nextSelectedChatId = selectedChatId && nextChats.some((chat) => chat.id === selectedChatId) ? selectedChatId : (nextChats[0]?.id ?? '')
        if (routeView === 'chat' && nextSelectedChatId) {
          requestChatTimeline(nextSelectedChatId, 'tail', undefined, 200)
        }
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
        setProjects((current) => sortProjects(upsertById(current, payload.project, (project) => project.id)))
        setSelectedProjectId(payload.project.id)
        setProjectDialogOpen(false)
        resetProjectForm(null)
        return
      }
      if (message.type === 'projects.reordered') {
        const payload = message.payload as ProjectsReorderedPayload
        setProjects(sortProjects(payload.projects ?? []))
        return
      }
      if (message.type === 'project.deleted') {
        const payload = message.payload as ProjectDeletedPayload
        delete projectSelectedChatIdsRef.current[payload.id]
        setProjectSelectedChatIds((current) => {
          if (!(payload.id in current)) {
            return current
          }
          const next = { ...current }
          delete next[payload.id]
          return next
        })
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
        removeChatScrollMemory(payload.chatIds)
        removeChatTimelineState(payload.chatIds)
        resetProjectForm(null)
        setProjectDialogOpen(false)
        return
      }
      if (message.type === 'chat.changed') {
        const payload = message.payload as ChatChangedPayload
        const chat = normalizeChatSummary(payload.chat)
        if (chat.status === 'running') {
          finishChatSendSuccess(chat.id)
        }
        const shouldSelectCreatedChat =
          pendingCreatedChatProjectIdRef.current === payload.chat.projectId &&
          !chatsRef.current.some((existingChat) => existingChat.id === payload.chat.id)
        if (shouldSelectCreatedChat) {
          pendingCreatedChatProjectIdRef.current = ''
          rememberProjectSelectedChat(payload.chat.projectId, payload.chat.id)
        }
        setChats((current) => sortByCreatedAt(upsertById(current, mergeChatSummary(current, chat), (item) => item.id)))
        setSelectedProjectId((current) => current || payload.chat.projectId)
        setSelectedChatId((current) => {
          if (shouldSelectCreatedChat) {
            return payload.chat.id
          }
          return current || payload.chat.id
        })
        return
      }
      if (message.type === 'chat.deleted') {
        const payload = message.payload as ChatDeletedPayload
        forgetProjectSelectedChat(payload.projectId, payload.id)
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
        removeChatScrollMemory([payload.id])
        removeChatTimelineState([payload.id])
        return
      }
      if (message.type === 'chat.timeline') {
        flushPendingChatTimelineRows()
        const payload = message.payload as ChatTimelinePayload
        delete chatTimelineLoadingRef.current[payload.chatId]
        setChatTimelineLoading((current) => {
          if (!current[payload.chatId]) {
            return current
          }
          const next = { ...current }
          delete next[payload.chatId]
          return next
        })
        chatTimelineLoadedRef.current[payload.chatId] = true
        chatTimelineHasOlderRef.current[payload.chatId] = payload.hasOlder
        setChatTimelineHasOlder((current) => ({ ...current, [payload.chatId]: payload.hasOlder }))
        const existingRows = chatTimelineRowsRef.current[payload.chatId] ?? []
        const rows =
          payload.reset || payload.direction === 'tail'
            ? payload.rows
            : payload.direction === 'before'
              ? [...payload.rows, ...existingRows]
              : [...existingRows, ...payload.rows]
        applyChatTimelineRows(payload.chatId, rows, { replace: true })
        return
      }
      if (message.type === 'chat.timeline.appended') {
        const payload = message.payload as ChatTimelineAppendedPayload
        enqueueChatTimelineRow(payload)
        if (payload.row.item.type === 'message_finished') {
          flushPendingChatTimelineRows()
          const rows = chatTimelineRowsRef.current[payload.chatId] ?? []
          if (!rows.some((row) => row.seq === payload.row.seq && row.item.type === 'message_finished')) {
            return
          }
          const chat = chatsRef.current.find((item) => item.id === payload.chatId)
          const projected = chat ? projectChatTimeline(chat, rows) : null
          const finished = projected?.messages.find((item) => item.id === payload.row.item.messageId)
          if (finished?.status === 'complete') {
            markChatIndicator(payload.chatId, 'success')
          } else if (finished?.status === 'error') {
            markChatIndicator(payload.chatId, 'error')
          }
          if (finished) {
            notifyAgentCompletion(finished)
          }
        }
        return
      }
      if (message.type === 'agent.status') {
        const payload = message.payload as AgentStatusPayload
        if (payload.status === 'running') {
          finishChatSendSuccess(payload.chatId)
          clearChatIndicator(payload.chatId)
        } else if (payload.status === 'error') {
          markChatIndicator(payload.chatId, 'error')
        } else if (payload.terminalStatus) {
          markChatIndicator(payload.chatId, payload.terminalStatus)
        }
        setChats((current) =>
          current.map((chat) => (chat.id === payload.chatId ? { ...chat, status: payload.status } : chat)),
        )
        return
      }
      if (message.type === 'agent.providers.changed') {
        const payload = message.payload as AgentProvidersChangedPayload
        setAgentProviders(payload.agentProviders ?? [])
        setNewClaudeModelID('')
        return
      }
      if (message.type === 'agent.profiles.changed') {
        const payload = message.payload as AgentProfilesChangedPayload
        setAgentProfiles(payload.agentProfiles ?? [])
        setAgentProviders(payload.agentProviders ?? [])
        setNewClaudeModelID('')
        return
      }
      if (message.type === 'agent.skills.changed') {
        const payload = message.payload as AgentSkillsChangedPayload
        setAgentSkills(payload.agentSkills ?? [])
        return
      }
      if (message.type === 'error') {
        const payload = message.payload as { message?: string; type?: string }
        const nextErrorText = payload.message ?? '服务端错误'
        if (payload.type === 'chat.send') {
          failPendingChatSends(nextErrorText)
        }
        setErrorText(nextErrorText)
      }
    },
    [
      clearChatIndicator,
      applyChatTimelineRows,
      enqueueChatTimelineRow,
      failPendingChatSends,
      finishChatSendSuccess,
      flushPendingChatTimelineRows,
      markChatIndicator,
      notifyAgentCompletion,
      pruneChatScrollMemory,
      pruneProjectSelectedChats,
      pruneComposerValues,
      requestChatTimeline,
      rememberProjectSelectedChat,
      forgetProjectSelectedChat,
      removeChatScrollMemory,
      removeChatTimelineState,
      removeComposerValues,
      resetProjectForm,
      routeView,
      selectedChatId,
    ],
  )

  // handleServerMessage 使用 message 参数处理服务端事件。
  const handleServerMessage = useCallback(
    (message: ServerMessage) => {
      applyServerMessage(message)
    },
    [applyServerMessage],
  )

  useEffect(() => {
    if (!authChecked) {
      return
    }
    if (authRequired && !authToken) {
      return
    }

    let stopped = false
    let retryTimer = 0
    let heartbeatTimer = 0

    // connect 建立 WebSocket 连接，并在断开后自动重连。
    const connect = () => {
      let opened = false
      setConnectionState('connecting')
      const ws = new WebSocket(getWebSocketUrl(authRequired ? authToken : ''))
      wsRef.current = ws

      ws.onopen = () => {
        opened = true
        setConnectionState('open')
        if (authRequired) {
          writeStoredAgentHubToken(authToken)
          setAuthTokenInput(authToken)
          setAuthErrorText('')
        }
        heartbeatTimer = window.setInterval(() => {
          sendClientMessage(ws, 'ping')
        }, 8000)
        flushChatDraft()
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
        if (authRequired && authToken && !opened) {
          clearStoredAgentHubToken()
          setAuthToken('')
          setAuthErrorText('Token 不正确')
          setHasSnapshot(false)
          setConnectionState('closed')
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
  }, [authChecked, authRequired, authToken, flushChatDraft, handleServerMessage])

  // submitAgentHubToken 处理 event 参数对应的 token 提交。
  const submitAgentHubToken = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const token = authTokenInput.trim()
    if (!token) {
      setAuthErrorText('请输入 Token')
      return
    }
    clearStoredAgentHubToken()
    setAuthErrorText('')
    setAuthToken(token)
  }

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

  // reorderProjects 使用 sourceProjectId、targetProjectId 和 placement 参数调整 Project 侧栏顺序。
  const reorderProjects = (sourceProjectId: string, targetProjectId: string, placement: 'before' | 'after') => {
    if (!sourceProjectId || !targetProjectId || sourceProjectId === targetProjectId) {
      return
    }
    const currentIds = projects.map((project) => project.id)
    const sourceIndex = currentIds.indexOf(sourceProjectId)
    const targetIndex = currentIds.indexOf(targetProjectId)
    if (sourceIndex < 0 || targetIndex < 0) {
      return
    }
    const nextIds = [...currentIds]
    nextIds.splice(sourceIndex, 1)
    const nextTargetIndex = nextIds.indexOf(targetProjectId)
    if (nextTargetIndex < 0) {
      return
    }
    const insertIndex = placement === 'after' ? nextTargetIndex + 1 : nextTargetIndex
    nextIds.splice(insertIndex, 0, sourceProjectId)
    setProjects((current) => {
      const projectsByID = new Map(current.map((project) => [project.id, project]))
      const nextProjects: Project[] = []
      for (const [index, projectId] of nextIds.entries()) {
        const project = projectsByID.get(projectId)
        if (project) {
          nextProjects.push({ ...project, sortOrder: index })
        }
      }
      return nextProjects.length === current.length ? nextProjects : current
    })
    if (!sendClientMessage(wsRef.current, 'project.reorder', { projectIds: nextIds })) {
      setErrorText('WebSocket 未连接')
    }
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
    const model = defaultModelForProvider(visibleAgentProviders, provider)
    const reasoning = defaultReasoningForModel(visibleAgentProviders, provider, model)
    updateChatAgent(provider, model, reasoning)
  }

  // changeAgentModel 使用 model 参数切换当前聊天页模型。
  const changeAgentModel = (model: string) => {
    const reasoning = defaultReasoningForModel(visibleAgentProviders, selectedAgentProvider, model)
    updateChatAgent(selectedAgentProvider, model, reasoning)
  }

  // changeAgentReasoning 使用 reasoning 参数切换当前聊天页推理级别。
  const changeAgentReasoning = (reasoning: string) => {
    updateChatAgent(selectedAgentProvider, selectedAgentModel, reasoning)
  }

  // loadOlderChatTimeline 拉取当前聊天页更早的 timeline 行。
  const loadOlderChatTimeline = () => {
    if (!selectedChat) {
      return
    }
    const cursor = chatTimelineCursorsRef.current[selectedChat.id]
    requestChatTimeline(selectedChat.id, 'before', cursor ? { epoch: cursor.epoch, seq: cursor.startSeq } : undefined, 200)
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

  // createAgentProfile 新增一个自定义 Codex Profile。
  const createAgentProfile = () => {
    const suffix = Date.now().toString(36)
    sendClientMessage(wsRef.current, 'agent.profile.create', {
      id: `profile-${suffix}`,
      label: '新 Profile',
      type: 'codex',
      command: 'codex',
      args: [],
      env: [],
      models: [{ id: 'gpt-5.5', label: 'gpt-5.5', default: true }],
    })
  }

  // saveAgentProfile 使用 profile 参数保存 Profile。
  const saveAgentProfile = (profile: AgentProfile) => {
    sendClientMessage(wsRef.current, 'agent.profile.update', profile)
  }

  // deleteAgentProfile 使用 profileId 参数删除 Profile。
  const deleteAgentProfile = (profileId: string) => {
    sendClientMessage(wsRef.current, 'agent.profile.delete', { id: profileId })
  }

  // addBuiltinProfile 使用 kind 参数新增内置 Profile。
  const addBuiltinProfile = (kind: AgentBuiltinProfileKind) => {
    sendClientMessage(wsRef.current, 'agent.profile.add_builtin', { kind })
  }

  // addProfileModel 处理 event 参数对应的 Profile 模型新增提交。
  const addProfileModel = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (!selectedSettingsProfile) {
      return
    }
    sendClientMessage(wsRef.current, 'agent.profile.model.add', {
      profileId: selectedSettingsProfile.id,
      id: newClaudeModelID.trim(),
    })
  }

  // deleteProfileModel 使用 profileId 和 modelId 参数删除 Profile 模型。
  const deleteProfileModel = (profileId: string, modelId: string) => {
    sendClientMessage(wsRef.current, 'agent.profile.model.delete', {
      profileId,
      id: modelId,
    })
  }

  // setProfileDefaultModel 使用 profileId 和 modelId 参数设置 Profile 默认模型。
  const setProfileDefaultModel = (profileId: string, modelId: string) => {
    sendClientMessage(wsRef.current, 'agent.profile.model.update', {
      profileId,
      id: modelId,
      default: true,
    })
  }

  // selectProject 使用 project 参数切换当前 project 并写入 hash 路由。
  const selectProject = (project: Project) => {
    if (selectedChat?.id) {
      flushChatDraft(selectedChat.id)
    }
    const chatId = findRememberedProjectChatId(project.id)
    setRouteView('chat')
    setSelectedProjectId(project.id)
    setSelectedChatId(chatId)
    updateHashRoute(project.id, chatId, 'push')
  }

  // selectChat 使用 chat 参数切换当前聊天页并写入 hash 路由。
  const selectChat = (chat: Chat) => {
    if (selectedChat?.id && selectedChat.id !== chat.id) {
      flushChatDraft(selectedChat.id)
    }
    setRouteView('chat')
    setSelectedProjectId(chat.projectId)
    setSelectedChatId(chat.id)
    rememberProjectSelectedChat(chat.projectId, chat.id)
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

  // requestNotificationPermission 在用户交互中请求桌面通知权限。
  const requestNotificationPermission = () => {
    if (!('Notification' in window) || Notification.permission !== 'default') {
      return
    }
    void Notification.requestPermission()
  }

  // executePlan 使用 plan 参数请求后端执行已确认 plan。
  const executePlan = (plan: PlanApproval) => {
    if (!selectedChat || plan.status !== 'pending') {
      return
    }
    requestNotificationPermission()
    updateSelectedPlanMode(false)
    sendClientMessage(wsRef.current, 'chat.plan.execute', { chatId: selectedChat.id, planId: plan.id })
  }

  // respondUserInput 使用 toolCall 和 answers 参数提交 agent 用户输入请求答案。
  const respondUserInput = (toolCall: ToolCall, answers: Record<string, string[]>) => {
    if (!selectedChat) {
      return
    }
    sendClientMessage(wsRef.current, 'chat.user_input.respond', {
      chatId: selectedChat.id,
      toolCallId: toolCall.id,
      answers,
    })
  }

  // refreshAgentSkills 请求后端重新扫描最新 skills。
  const refreshAgentSkills = () => {
    sendClientMessage(wsRef.current, 'agent.skills.refresh')
  }

  // submitComposer 处理 event 参数对应的聊天输入提交，promptOverride 用于回车事件中的 DOM 当前值。
  const submitComposer = (event?: FormEvent<HTMLFormElement>, promptOverride?: string) => {
    event?.preventDefault()
    if (!selectedChat) {
      return
    }
    const prompt = (promptOverride ?? selectedComposerValue).trim()
    const images = selectedComposerImages.map(({ id, fileName, mimeType, data }) => ({ id, fileName, mimeType, data }))
    const hasPayload = Boolean(prompt) || images.length > 0
    if (!hasPayload && isRunning) {
      sendClientMessage(wsRef.current, 'chat.stop', { chatId: selectedChat.id })
      return
    }
    if (!hasPayload) {
      return
    }
    cancelChatDraftSave(selectedChat.id)
    setChatSubmitError(selectedChat.id, '')
    setChatSendPending(selectedChat.id)
    requestNotificationPermission()
    if (!sendClientMessage(wsRef.current, 'chat.send', { chatId: selectedChat.id, prompt, images, planMode: selectedPlanMode })) {
      setChatSubmitError(selectedChat.id, 'WebSocket 未连接')
      clearChatSendAwaiting(selectedChat.id)
      clearChatSendPending(selectedChat.id, true)
    }
  }

  const connectionIcon =
    connectionState === 'open' ? (
      <Wifi className="h-4 w-4 text-[var(--agenthub-success)]" />
    ) : connectionState === 'connecting' ? (
      <Loader2 className="h-4 w-4 animate-spin text-[var(--agenthub-warning)]" />
    ) : (
      <WifiOff className="h-4 w-4 text-[var(--agenthub-danger)]" />
    )

  if (!authChecked) {
    return (
      <main data-theme="material" className="theme-material flex h-[100dvh] items-center justify-center bg-[var(--agenthub-bg)] text-[var(--agenthub-foreground)]">
        <div className="flex items-center gap-2 text-sm text-[var(--agenthub-muted)]" data-testid="auth-loading">
          <Loader2 className="h-4 w-4 animate-spin text-[var(--agenthub-warning)]" />
          <span>正在连接</span>
        </div>
      </main>
    )
  }

  if (authRequired && !authToken) {
    return (
      <main data-theme="material" className="theme-material flex h-[100dvh] items-center justify-center bg-[var(--agenthub-bg)] px-4 text-[var(--agenthub-foreground)]">
        <form
          className="w-full max-w-sm rounded-lg border border-[var(--agenthub-outline)] bg-[var(--agenthub-surface-0)] p-5 shadow-[var(--agenthub-elevation-1)]"
          data-testid="token-auth-form"
          onSubmit={submitAgentHubToken}
        >
          <div className="mb-5 flex items-center gap-3">
            <div className="flex h-9 w-9 items-center justify-center rounded-md border border-[var(--agenthub-outline)] bg-[var(--agenthub-primary-container)] text-[var(--agenthub-primary)]">
              <LockKeyhole className="h-4 w-4" />
            </div>
            <h1 className="text-base font-medium text-[var(--agenthub-foreground)]">AgentHub Token</h1>
          </div>
          <label className="mb-2 block text-sm text-[var(--agenthub-muted)]" htmlFor="agenthub-token-input">
            Token
          </label>
          <Input
            autoComplete="current-password"
            autoFocus
            data-testid="agenthub-token-input"
            id="agenthub-token-input"
            onChange={(event) => setAuthTokenInput(event.target.value)}
            type="password"
            value={authTokenInput}
          />
          <p className="mt-3 min-h-5 text-sm text-[var(--agenthub-danger)]" data-testid="token-auth-error">
            {authErrorText}
          </p>
          <Button className="mt-2 w-full" data-testid="agenthub-token-submit" type="submit">
            进入
          </Button>
        </form>
      </main>
    )
  }

  return (
    <main
      data-theme="material"
      className="theme-material grid h-[100dvh] min-h-0 overflow-hidden bg-[var(--agenthub-bg)] text-[var(--agenthub-foreground)] lg:grid-cols-[304px_minmax(0,1fr)]"
    >
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
        onProjectReorder={reorderProjects}
        onSettingsOpen={openAgentSettings}
      />

      <section className="flex h-full min-h-0 min-w-0 flex-col">
        {routeView === 'settings' ? (
          <AgentSettingsPage
            connectionState={connectionState}
            backendVersion={backendVersion}
            backendBuildTime={backendBuildTime}
            hostname={hostname}
            agentProfiles={agentProfiles}
            backendEnv={backendEnv}
            selectedProfileId={selectedSettingsProfile?.id ?? ''}
            newClaudeModelID={newClaudeModelID}
            onBackToChat={backToChat}
            onProfileSelect={setSelectedSettingsProfileId}
            onProfileCreate={createAgentProfile}
            onProfileSave={saveAgentProfile}
            onProfileDelete={deleteAgentProfile}
            onBuiltinAdd={addBuiltinProfile}
            onModelIDChange={setNewClaudeModelID}
            onAddProfileModel={addProfileModel}
            onDeleteProfileModel={deleteProfileModel}
            onSetProfileDefaultModel={setProfileDefaultModel}
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
            composerImages={selectedComposerImages}
            planMode={selectedPlanMode}
            copiedMessageId={copiedMessageId}
            isRunning={isRunning}
            isSending={selectedChatSending}
            isSendAwaiting={selectedChatSendAwaiting}
            submitErrorText={selectedChatSubmitError}
            scrollToBottomSignal={selectedChatScrollBottomSignal}
            timelineHasOlder={selectedChatTimelineHasOlder}
            timelineLoading={selectedChatTimelineLoading}
            agentProviders={visibleAgentProviders}
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
            onExecutePlan={executePlan}
            onRespondUserInput={respondUserInput}
            onReadChatScrollMemory={readChatScrollMemory}
            onSaveChatScrollMemory={saveChatScrollMemory}
            onComposerValueChange={updateSelectedComposerValue}
            onComposerDraftFlush={() => selectedChat?.id && flushChatDraft(selectedChat.id)}
            onComposerImagesChange={updateSelectedComposerImages}
            onLoadOlderTimeline={loadOlderChatTimeline}
            onRefreshAgentSkills={refreshAgentSkills}
            onPlanModeChange={updateSelectedPlanMode}
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
