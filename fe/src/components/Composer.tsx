import {
  ChangeEvent,
  ClipboardEvent,
  CSSProperties,
  FormEvent,
  KeyboardEvent,
  useCallback,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
} from 'react'
import { ArrowUp, Bot, Brain, ChevronDown, ClipboardList, Loader2, MessageSquare, Square, X } from 'lucide-react'
import imagePlusLogo from '../assets/image-plus-logo.svg'
import { Button } from './ui/button'
import { Select } from './ui/select'
import { Textarea } from './ui/textarea'
import type {
  AgentModelOption,
  AgentProvider,
  AgentProviderOption,
  AgentSkillOption,
  Chat,
  ComposerImageAttachment,
  ConnectionState,
} from '../types'

interface ComposerProps {
  /** selectedChat 表示当前聊天页。 */
  selectedChat: Chat | null
  /** connectionState 表示当前 WebSocket 连接状态。 */
  connectionState: ConnectionState
  /** composerValue 表示输入框内容。 */
  composerValue: string
  /** composerImages 表示输入框图片附件草稿。 */
  composerImages: ComposerImageAttachment[]
  /** planMode 表示当前聊天页是否开启 plan 模式。 */
  planMode: boolean
  /** isRunning 表示当前聊天页是否正在输出。 */
  isRunning: boolean
  /** isSending 表示当前聊天输入是否正在提交。 */
  isSending: boolean
  /** isSendAwaiting 表示当前聊天输入仍在等待服务端确认。 */
  isSendAwaiting: boolean
  /** submitErrorText 表示当前聊天输入提交失败信息。 */
  submitErrorText: string
  /** agentProviders 表示可选 agent provider。 */
  agentProviders: AgentProviderOption[]
  /** agentSkills 表示可选 skills。 */
  agentSkills: AgentSkillOption[]
  /** selectedAgentProvider 表示当前 agent provider。 */
  selectedAgentProvider: AgentProvider
  /** selectedAgentModels 表示当前 provider 下的模型。 */
  selectedAgentModels: AgentModelOption[]
  /** selectedAgentModel 表示当前模型。 */
  selectedAgentModel: string
  /** selectedAgentModelOption 表示当前模型详情。 */
  selectedAgentModelOption: AgentModelOption | null
  /** selectedAgentReasoning 表示当前推理级别。 */
  selectedAgentReasoning: string
  /** agentControlsDisabled 表示 agent 控件是否禁用。 */
  agentControlsDisabled: boolean
  /** providerLocked 表示 provider 是否锁定。 */
  providerLocked: boolean
  /** modelControlsDisabled 表示模型和推理控件是否禁用。 */
  modelControlsDisabled: boolean
  /** onComposerValueChange 使用 value 参数更新输入框内容。 */
  onComposerValueChange: (value: string) => void
  /** onComposerImagesChange 使用 images 参数更新图片附件草稿。 */
  onComposerImagesChange: (images: ComposerImageAttachment[]) => void
  /** onRefreshAgentSkills 请求后端刷新最新 skills。 */
  onRefreshAgentSkills: () => void
  /** onPlanModeChange 使用 enabled 参数切换 plan 模式。 */
  onPlanModeChange: (enabled: boolean) => void
  /** onSubmitComposer 使用 event 和 promptOverride 参数提交输入框。 */
  onSubmitComposer: (event?: FormEvent<HTMLFormElement>, promptOverride?: string) => void
  /** onChangeAgentProvider 使用 provider 参数切换 agent。 */
  onChangeAgentProvider: (provider: AgentProvider) => void
  /** onChangeAgentModel 使用 model 参数切换模型。 */
  onChangeAgentModel: (model: string) => void
  /** onChangeAgentReasoning 使用 reasoning 参数切换推理级别。 */
  onChangeAgentReasoning: (reasoning: string) => void
}

interface ComposerSuggestion {
  /** type 表示建议来源类型。 */
  type: 'skill' | 'command'
  /** trigger 表示触发建议菜单的前缀。 */
  trigger: '/' | '#'
  /** id 表示插入输入框的命令标识。 */
  id: string
  /** label 表示界面展示名称。 */
  label: string
  /** description 表示建议说明。 */
  description: string
}

const composerCommandOptions: ComposerSuggestion[] = [
  {
    type: 'command',
    trigger: '#',
    id: 'skills',
    label: 'skills',
    description: '显示当前可用 skills 列表',
  },
]

// Composer 使用 props 参数渲染聊天输入框和 agent 控件。
export function Composer({
  selectedChat,
  connectionState,
  composerValue,
  composerImages,
  planMode,
  isRunning,
  isSending,
  isSendAwaiting,
  submitErrorText,
  agentProviders,
  agentSkills,
  selectedAgentProvider,
  selectedAgentModels,
  selectedAgentModel,
  selectedAgentModelOption,
  selectedAgentReasoning,
  agentControlsDisabled,
  providerLocked,
  modelControlsDisabled,
  onComposerValueChange,
  onComposerImagesChange,
  onRefreshAgentSkills,
  onPlanModeChange,
  onSubmitComposer,
  onChangeAgentProvider,
  onChangeAgentModel,
  onChangeAgentReasoning,
}: ComposerProps) {
  const skillMenuID = useId()
  const textareaRef = useRef<HTMLTextAreaElement | null>(null)
  const fileInputRef = useRef<HTMLInputElement | null>(null)
  const [selectedSuggestionIndex, setSelectedSuggestionIndex] = useState(0)
  const filteredSuggestions = useMemo(() => buildComposerSuggestions(composerValue, agentSkills), [agentSkills, composerValue])
  const skillMenuVisible = filteredSuggestions.length > 0
  const hasComposerPayload = Boolean(composerValue.trim()) || composerImages.length > 0
  const blockSubmit = isSendAwaiting && !(isRunning && hasComposerPayload)
  const showSendingState = blockSubmit || isSending
  const showStopState = isRunning && !hasComposerPayload && !showSendingState
  const selectedProviderLabel = agentProviders.find((provider) => provider.id === selectedAgentProvider)?.label ?? selectedAgentProvider
  const selectedModelValue = selectedAgentModels.find((model) => model.id === selectedAgentModel)?.id ?? selectedAgentModel
  const selectedReasoningLabel =
    selectedAgentModelOption?.reasoningLevels?.find((level) => level.id === selectedAgentReasoning)?.label ?? selectedAgentReasoning

  // resizeTextarea 根据当前内容调整输入框高度，并限制最大高度。
  const resizeTextarea = useCallback(() => {
    const textarea = textareaRef.current
    if (!textarea) {
      return
    }
    textarea.style.height = 'auto'
    const minHeight = 32
    const maxHeight = 168
    const nextHeight = Math.min(Math.max(textarea.scrollHeight, minHeight), maxHeight)
    textarea.style.height = `${nextHeight}px`
    textarea.style.overflowY = textarea.scrollHeight > maxHeight ? 'auto' : 'hidden'
  }, [])

  useEffect(() => {
    resizeTextarea()
  }, [composerValue, resizeTextarea, selectedChat?.id])

  const activeSuggestionIndex = skillMenuVisible ? boundedSkillIndex(selectedSuggestionIndex, filteredSuggestions.length) : 0
  const activeSuggestion = filteredSuggestions[activeSuggestionIndex] ?? null

  // applySuggestion 使用 suggestion 参数把 slash skill 或本地命令插入输入框。
  const applySuggestion = useCallback(
    (suggestion: ComposerSuggestion) => {
      setSelectedSuggestionIndex(0)
      onComposerValueChange(`${suggestion.trigger}${suggestion.id} `)
      window.setTimeout(() => textareaRef.current?.focus(), 0)
    },
    [onComposerValueChange],
  )

  // appendImages 使用 files 参数把图片文件追加到附件草稿。
  const appendImages = useCallback(
    async (files: File[]) => {
      const images = await Promise.all(files.filter((file) => file.type.startsWith('image/')).map(fileToComposerImage))
      if (images.length === 0) {
        return
      }
      onComposerImagesChange([...composerImages, ...images])
    },
    [composerImages, onComposerImagesChange],
  )

  // removeImage 使用 imageId 参数移除图片附件草稿。
  const removeImage = useCallback(
    (imageId: string) => {
      onComposerImagesChange(composerImages.filter((image) => image.id !== imageId))
    },
    [composerImages, onComposerImagesChange],
  )

  // handleImageInputChange 使用 event 参数处理文件选择结果。
  const handleImageInputChange = (event: ChangeEvent<HTMLInputElement>) => {
    const files = Array.from(event.target.files ?? [])
    event.target.value = ''
    void appendImages(files)
  }

  // handleComposerValueChange 使用 value 参数更新输入内容、刷新 skills 并重置 skill 键盘选中项。
  const handleComposerValueChange = (value: string) => {
    setSelectedSuggestionIndex(0)
    if ((value === '/' || value === '#') && composerValue !== value) {
      onRefreshAgentSkills()
    }
    onComposerValueChange(value)
  }

  // handlePaste 使用 event 参数处理剪贴板图片。
  const handlePaste = (event: ClipboardEvent<HTMLTextAreaElement>) => {
    const files = Array.from(event.clipboardData.files ?? []).filter((file) => file.type.startsWith('image/'))
    if (files.length === 0) {
      return
    }
    event.preventDefault()
    void appendImages(files)
  }

  // handleKeyDown 使用 event 参数处理 Enter 发送和 skill 菜单键盘选择。
  const handleKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (skillMenuVisible) {
      if (event.key === 'ArrowDown') {
        event.preventDefault()
        setSelectedSuggestionIndex((current) => (boundedSkillIndex(current, filteredSuggestions.length) + 1) % filteredSuggestions.length)
        return
      }
      if (event.key === 'ArrowUp') {
        event.preventDefault()
        setSelectedSuggestionIndex(
          (current) => (boundedSkillIndex(current, filteredSuggestions.length) - 1 + filteredSuggestions.length) % filteredSuggestions.length,
        )
        return
      }
      if (event.key === 'Tab' || event.key === 'Enter') {
        event.preventDefault()
        applySuggestion(activeSuggestion ?? filteredSuggestions[0])
        return
      }
    }
    if (event.key !== 'Enter' || event.shiftKey || event.nativeEvent.isComposing) {
      return
    }
    event.preventDefault()
    onSubmitComposer(undefined, event.currentTarget.value)
  }

  return (
    <form data-testid="composer-taskbar" className="composer-taskbar sticky bottom-0 z-10 shrink-0 bg-[var(--agenthub-bg)] px-4 py-4" onSubmit={onSubmitComposer}>
      <div
        data-testid="composer-shell"
        className="composer-shell relative mx-auto w-full max-w-[860px] rounded-[18px] border border-[var(--agenthub-outline)] bg-[var(--agenthub-surface-1)] px-4 py-3 shadow-[var(--agenthub-elevation-2)]"
      >
        {skillMenuVisible ? (
          <div
            id={skillMenuID}
            data-testid="skill-menu"
            role="listbox"
            aria-label="选择 skill 或命令"
            className="absolute bottom-[calc(100%+8px)] left-0 right-0 z-30 mx-auto max-h-64 max-w-[860px] overflow-y-auto rounded-lg border border-[var(--agenthub-outline)] bg-[var(--agenthub-surface-0)] p-1 shadow-[var(--agenthub-elevation-2)]"
          >
            {filteredSuggestions.map((suggestion, index) => (
              <button
                key={`${suggestion.type}:${suggestion.id}`}
                id={suggestionOptionID(skillMenuID, suggestion)}
                data-testid="skill-option"
                data-skill-id={suggestion.type === 'skill' ? suggestion.id : undefined}
                data-command-id={suggestion.type === 'command' ? suggestion.id : undefined}
                data-active={index === activeSuggestionIndex ? 'true' : 'false'}
                role="option"
                aria-selected={index === activeSuggestionIndex}
                type="button"
                onMouseDown={(event) => event.preventDefault()}
                onClick={() => applySuggestion(suggestion)}
                className={`flex w-full cursor-pointer items-start gap-3 rounded-md px-3 py-2 text-left transition ${
                  index === activeSuggestionIndex ? 'bg-[var(--agenthub-primary-container)]' : 'hover:bg-[var(--agenthub-surface-2)]'
                }`}
              >
                <span className="shrink-0 font-mono text-sm text-[var(--agenthub-primary)]">
                  {suggestion.trigger}
                  {suggestion.label}
                </span>
                <span className="min-w-0 flex-1 truncate text-xs leading-5 text-[var(--agenthub-muted)]">{suggestion.description}</span>
              </button>
            ))}
          </div>
        ) : null}
        <div>
          <label htmlFor="message-input" className="sr-only">
            Prompt
          </label>
          <Textarea
            ref={textareaRef}
            id="message-input"
            data-testid="message-input"
            value={composerValue}
            onChange={(event) => handleComposerValueChange(event.target.value)}
            onKeyDown={handleKeyDown}
            onPaste={handlePaste}
            aria-controls={skillMenuVisible ? skillMenuID : undefined}
            aria-expanded={skillMenuVisible}
            aria-activedescendant={skillMenuVisible && activeSuggestion ? suggestionOptionID(skillMenuID, activeSuggestion) : undefined}
            aria-autocomplete="list"
            disabled={!selectedChat || connectionState !== 'open'}
            rows={1}
            className="max-h-[168px] min-h-8 border-0 px-0 py-0 text-base leading-6 shadow-none focus:ring-0"
            placeholder={selectedChat ? '输入消息' : '先创建聊天'}
          />
        </div>
        {composerImages.length > 0 ? (
          <div data-testid="composer-attachments" className="mt-3 flex flex-wrap gap-2">
            {composerImages.map((image) => (
              <div
                key={image.id}
                className="inline-flex max-w-52 items-center gap-2 rounded-md border border-[var(--agenthub-outline)] bg-[var(--agenthub-surface-0)] p-1 pr-2"
              >
                <img src={image.previewUrl} alt="" className="h-9 w-9 rounded object-cover" />
                <span className="min-w-0 truncate text-xs text-[var(--agenthub-muted)]">{image.fileName}</span>
                <button
                  type="button"
                  className="inline-flex h-6 w-6 shrink-0 items-center justify-center rounded text-[var(--agenthub-muted)] hover:bg-[var(--agenthub-surface-2)] hover:text-[var(--agenthub-foreground)]"
                  onClick={() => removeImage(image.id)}
                  aria-label="移除图片"
                  title="移除"
                >
                  <X className="h-3.5 w-3.5" />
                </button>
              </div>
            ))}
          </div>
        ) : null}
        <div className="mt-3 flex flex-wrap items-center gap-2" data-testid="composer-agent-config">
          <input
            ref={fileInputRef}
            data-testid="image-file-input"
            type="file"
            accept="image/*"
            multiple
            className="sr-only"
            onChange={handleImageInputChange}
          />
          <Button
            data-testid="image-add-button"
            type="button"
            variant="ghost"
            size="icon"
            className="h-8 w-8 rounded-full text-[var(--agenthub-muted)]"
            aria-label="添加图片"
            title="添加图片"
            onClick={() => fileInputRef.current?.click()}
          >
            <img data-testid="image-add-logo" src={imagePlusLogo} alt="" draggable={false} className="h-4 w-4" />
          </Button>
          <Button
            data-testid="plan-mode-toggle"
            data-active={planMode ? 'true' : 'false'}
            type="button"
            variant="ghost"
            size="icon"
            className={`h-8 w-8 rounded-full text-[var(--agenthub-muted)] ${planMode ? 'bg-[var(--agenthub-primary-container)] text-[var(--agenthub-primary)]' : ''}`}
            aria-pressed={planMode}
            aria-label="切换 plan 模式"
            title="Plan 模式"
            onClick={() => onPlanModeChange(!planMode)}
          >
            <ClipboardList className="h-4 w-4" />
          </Button>
          <div className="relative">
            <Bot className="pointer-events-none absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-[var(--agenthub-muted)]" />
            <Select
              id="agent-provider-select"
              data-testid="agent-provider-select"
              value={selectedAgentProvider}
              onChange={(event) => onChangeAgentProvider(event.target.value as AgentProvider)}
              disabled={agentControlsDisabled || providerLocked}
              aria-label="选择助理"
              style={composerSelectWidthStyle(selectedProviderLabel, 4, 32)}
              className="composer-select h-8 appearance-none rounded-full border-transparent bg-[var(--agenthub-surface-2)] pl-8 pr-8 text-xs"
            >
              {agentProviders.map((provider) => (
                <option key={provider.id} value={provider.id}>
                  {provider.label}
                </option>
              ))}
            </Select>
            <ChevronDown className="pointer-events-none absolute right-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-[var(--agenthub-muted)]" />
          </div>
          <label htmlFor="agent-model-select" className="sr-only">
            模型
          </label>
          <div className="relative">
            <MessageSquare className="pointer-events-none absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-[var(--agenthub-muted)]" />
            <Select
              id="agent-model-select"
              data-testid="agent-model-select"
              value={selectedAgentModel}
              onChange={(event) => onChangeAgentModel(event.target.value)}
              disabled={modelControlsDisabled}
              style={composerSelectWidthStyle(selectedModelValue, 4, 42)}
              className="composer-select h-8 appearance-none rounded-full border-transparent bg-[var(--agenthub-surface-2)] pl-8 pr-8 text-xs"
            >
              {selectedAgentModels.map((model) => (
                <option key={model.id} value={model.id}>
                  {model.id}
                </option>
              ))}
            </Select>
            <ChevronDown className="pointer-events-none absolute right-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-[var(--agenthub-muted)]" />
          </div>
          {selectedAgentModelOption?.reasoningLevels?.length ? (
            <div className="relative">
              <label htmlFor="agent-reasoning-select" className="sr-only">
                推理级别
              </label>
              <Brain className="pointer-events-none absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-[var(--agenthub-muted)]" />
              <Select
                id="agent-reasoning-select"
                data-testid="agent-reasoning-select"
                value={selectedAgentReasoning}
                onChange={(event) => onChangeAgentReasoning(event.target.value)}
                disabled={modelControlsDisabled}
                style={composerSelectWidthStyle(selectedReasoningLabel, 4, 24)}
                className="composer-select h-8 appearance-none rounded-full border-transparent bg-[var(--agenthub-surface-2)] pl-8 pr-8 text-xs"
              >
                {selectedAgentModelOption.reasoningLevels.map((level) => (
                  <option key={level.id} value={level.id}>
                    {level.label}
                  </option>
                ))}
              </Select>
              <ChevronDown className="pointer-events-none absolute right-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-[var(--agenthub-muted)]" />
            </div>
          ) : null}
          <span className="min-w-4 flex-1" />
          {hasComposerPayload || isRunning || isSending ? (
            <Button
              data-testid="send-button"
              type="submit"
              size="icon"
              disabled={!selectedChat || connectionState !== 'open' || blockSubmit || (!hasComposerPayload && !isRunning)}
              className={`h-9 w-9 shrink-0 rounded-full ${showStopState ? 'bg-[var(--agenthub-warning)] hover:bg-[var(--agenthub-warning)]' : ''}`}
              aria-busy={showSendingState}
              aria-label={showSendingState ? '发送中' : showStopState ? '停止' : '发送'}
            >
              {showSendingState ? (
                <Loader2 data-testid="send-loading-icon" className="h-4 w-4 animate-spin" />
              ) : showStopState ? (
                <Square className="h-4 w-4" fill="currentColor" />
              ) : (
                <ArrowUp className="h-4 w-4" />
              )}
            </Button>
          ) : null}
        </div>
        {submitErrorText ? (
          <p data-testid="composer-error" role="alert" className="mt-2 text-sm text-[var(--agenthub-danger)]">
            {submitErrorText}
          </p>
        ) : null}
      </div>
    </form>
  )
}

// fileToComposerImage 使用 file 参数转换图片文件为前端附件。
async function fileToComposerImage(file: File): Promise<ComposerImageAttachment> {
  const dataUrl = await readFileAsDataURL(file)
  const data = dataUrl.split(',')[1] ?? ''
  return {
    id: `image-${Date.now()}-${Math.random().toString(36).slice(2)}`,
    fileName: file.name || 'image.png',
    mimeType: file.type || 'image/png',
    data,
    previewUrl: dataUrl,
  }
}

// readFileAsDataURL 使用 file 参数读取 data URL。
function readFileAsDataURL(file: File) {
  return new Promise<string>((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result ?? ''))
    reader.onerror = () => reject(reader.error ?? new Error('读取图片失败'))
    reader.readAsDataURL(file)
  })
}

// composerSelectWidthStyle 使用 label、minCh 和 maxCh 参数生成选择框宽度样式。
function composerSelectWidthStyle(label: string, minCh: number, maxCh: number): CSSProperties {
  const contentWidth = Math.min(Math.max(visualTextLength(label), minCh), maxCh)
  return { maxWidth: '100%', width: `calc(${contentWidth}ch + 4.5rem)` }
}

// boundedSkillIndex 使用 index 和 count 参数返回可见 skill 列表中的有效下标。
function boundedSkillIndex(index: number, count: number) {
  if (count <= 0 || index < 0 || index >= count) {
    return 0
  }
  return index
}

// buildComposerSuggestions 使用 value 和 agentSkills 参数生成输入框辅助提示列表。
function buildComposerSuggestions(value: string, agentSkills: AgentSkillOption[]): ComposerSuggestion[] {
  const trigger: '/' | '#' | '' = value.startsWith('/') ? '/' : value.startsWith('#') ? '#' : ''
  if (!trigger || /\s/.test(value)) {
    return []
  }
  const query = value.slice(1).toLowerCase()
  const suggestions =
    trigger === '/'
      ? agentSkills.map((skill) => ({
          type: 'skill' as const,
          trigger,
          id: skill.id,
          label: skill.label,
          description: skill.description,
        }))
      : composerCommandOptions
  return suggestions
    .filter((suggestion) => {
      const haystack = `${suggestion.id} ${suggestion.label} ${suggestion.description}`.toLowerCase()
      return haystack.includes(query)
    })
    .slice(0, 8)
}

// suggestionOptionID 使用 menuID 和 suggestion 参数生成辅助提示选项 DOM ID。
function suggestionOptionID(menuID: string, suggestion: ComposerSuggestion) {
  return `${menuID}-${suggestion.type}-${encodeURIComponent(suggestion.id)}`
}

// visualTextLength 使用 text 参数估算中英文混排文本的视觉宽度。
function visualTextLength(text: string) {
  return Array.from(text || '').reduce((width, char) => width + (char.charCodeAt(0) > 255 ? 2 : 1), 0)
}
