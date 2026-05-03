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
import { ArrowUp, Bot, Brain, ChevronDown, ClipboardList, MessageSquare, Square, X } from 'lucide-react'
import imagePlusLogo from '../assets/image-plus-logo.svg'
import { ContextWindowMeter } from './ContextWindowMeter'
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
  /** onPlanModeChange 使用 enabled 参数切换 plan 模式。 */
  onPlanModeChange: (enabled: boolean) => void
  /** onSubmitComposer 使用 event 参数提交输入框。 */
  onSubmitComposer: (event?: FormEvent<HTMLFormElement>) => void
  /** onChangeAgentProvider 使用 provider 参数切换 agent。 */
  onChangeAgentProvider: (provider: AgentProvider) => void
  /** onChangeAgentModel 使用 model 参数切换模型。 */
  onChangeAgentModel: (model: string) => void
  /** onChangeAgentReasoning 使用 reasoning 参数切换推理级别。 */
  onChangeAgentReasoning: (reasoning: string) => void
}

// Composer 使用 props 参数渲染聊天输入框和 agent 控件。
export function Composer({
  selectedChat,
  connectionState,
  composerValue,
  composerImages,
  planMode,
  isRunning,
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
  onPlanModeChange,
  onSubmitComposer,
  onChangeAgentProvider,
  onChangeAgentModel,
  onChangeAgentReasoning,
}: ComposerProps) {
  const skillMenuID = useId()
  const textareaRef = useRef<HTMLTextAreaElement | null>(null)
  const fileInputRef = useRef<HTMLInputElement | null>(null)
  const [selectedSkillIndex, setSelectedSkillIndex] = useState(0)
  const skillQuery = composerValue.startsWith('/') && !composerValue.includes(' ') ? composerValue.slice(1).toLowerCase() : ''
  const filteredSkills = useMemo(() => {
    if (!composerValue.startsWith('/') || composerValue.includes(' ')) {
      return []
    }
    return agentSkills
      .filter((skill) => {
        const haystack = `${skill.id} ${skill.label} ${skill.description}`.toLowerCase()
        return haystack.includes(skillQuery)
      })
      .slice(0, 8)
  }, [agentSkills, composerValue, skillQuery])
  const skillMenuVisible = filteredSkills.length > 0
  const hasComposerPayload = composerValue.trim() || composerImages.length > 0
  const selectedProviderLabel = agentProviders.find((provider) => provider.id === selectedAgentProvider)?.label ?? selectedAgentProvider
  const selectedModelLabel = selectedAgentModels.find((model) => model.id === selectedAgentModel)?.label ?? selectedAgentModel
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

  const activeSkillIndex = skillMenuVisible ? boundedSkillIndex(selectedSkillIndex, filteredSkills.length) : 0
  const activeSkill = filteredSkills[activeSkillIndex] ?? null

  // applySkill 使用 skill 参数把 slash skill 插入输入框。
  const applySkill = useCallback(
    (skill: AgentSkillOption) => {
      setSelectedSkillIndex(0)
      onComposerValueChange(`/${skill.id} `)
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

  // handleComposerValueChange 使用 value 参数更新输入内容并重置 skill 键盘选中项。
  const handleComposerValueChange = (value: string) => {
    setSelectedSkillIndex(0)
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
        setSelectedSkillIndex((current) => (boundedSkillIndex(current, filteredSkills.length) + 1) % filteredSkills.length)
        return
      }
      if (event.key === 'ArrowUp') {
        event.preventDefault()
        setSelectedSkillIndex((current) => (boundedSkillIndex(current, filteredSkills.length) - 1 + filteredSkills.length) % filteredSkills.length)
        return
      }
      if (event.key === 'Tab' || event.key === 'Enter') {
        event.preventDefault()
        applySkill(activeSkill ?? filteredSkills[0])
        return
      }
    }
    if (event.key !== 'Enter' || event.shiftKey || event.nativeEvent.isComposing) {
      return
    }
    event.preventDefault()
    onSubmitComposer()
  }

  return (
    <form data-testid="composer-taskbar" className="composer-taskbar sticky bottom-0 z-10 shrink-0 bg-white px-4 py-4" onSubmit={onSubmitComposer}>
      <div data-testid="composer-shell" className="composer-shell relative mx-auto w-full max-w-[860px] rounded-[20px] border border-slate-200 bg-white px-4 py-3 shadow-sm">
        {skillMenuVisible ? (
          <div
            id={skillMenuID}
            data-testid="skill-menu"
            role="listbox"
            aria-label="选择 skill"
            className="absolute bottom-[calc(100%+8px)] left-0 right-0 z-30 mx-auto max-h-64 max-w-[860px] overflow-y-auto rounded-md border border-slate-200 bg-white p-1 shadow-lg"
          >
            {filteredSkills.map((skill, index) => (
              <button
                key={skill.id}
                id={skillOptionID(skillMenuID, skill.id)}
                data-testid="skill-option"
                data-skill-id={skill.id}
                data-active={index === activeSkillIndex ? 'true' : 'false'}
                role="option"
                aria-selected={index === activeSkillIndex}
                type="button"
                onMouseDown={(event) => event.preventDefault()}
                onClick={() => applySkill(skill)}
                className={`flex w-full cursor-pointer items-start gap-3 rounded-md px-3 py-2 text-left transition ${
                  index === activeSkillIndex ? 'bg-slate-100' : 'hover:bg-slate-50'
                }`}
              >
                <span className="shrink-0 font-mono text-sm text-teal-700">/{skill.label}</span>
                <span className="min-w-0 flex-1 truncate text-xs leading-5 text-slate-500">{skill.description}</span>
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
            aria-activedescendant={skillMenuVisible && activeSkill ? skillOptionID(skillMenuID, activeSkill.id) : undefined}
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
              <div key={image.id} className="inline-flex max-w-52 items-center gap-2 rounded-md border border-slate-200 bg-white p-1 pr-2">
                <img src={image.previewUrl} alt="" className="h-9 w-9 rounded object-cover" />
                <span className="min-w-0 truncate text-xs text-slate-600">{image.fileName}</span>
                <button
                  type="button"
                  className="inline-flex h-6 w-6 shrink-0 items-center justify-center rounded text-slate-500 hover:bg-slate-100 hover:text-slate-900"
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
            className="h-8 w-8 rounded-full text-slate-500"
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
            className={`h-8 w-8 rounded-full text-slate-500 ${planMode ? 'bg-slate-200 text-slate-900' : ''}`}
            aria-pressed={planMode}
            aria-label="切换 plan 模式"
            title="Plan 模式"
            onClick={() => onPlanModeChange(!planMode)}
          >
            <ClipboardList className="h-4 w-4" />
          </Button>
          <div className="relative">
            <Bot className="pointer-events-none absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-slate-500" />
            <Select
              id="agent-provider-select"
              data-testid="agent-provider-select"
              value={selectedAgentProvider}
              onChange={(event) => onChangeAgentProvider(event.target.value as AgentProvider)}
              disabled={agentControlsDisabled || providerLocked}
              aria-label="选择助理"
              style={composerSelectWidthStyle(selectedProviderLabel, 4, 32)}
              className="composer-select h-8 appearance-none rounded-full border-transparent bg-slate-50 pl-8 pr-8 text-xs"
            >
              {agentProviders.map((provider) => (
                <option key={provider.id} value={provider.id}>
                  {provider.label}
                </option>
              ))}
            </Select>
            <ChevronDown className="pointer-events-none absolute right-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-slate-500" />
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
              onChange={(event) => onChangeAgentModel(event.target.value)}
              disabled={modelControlsDisabled}
              style={composerSelectWidthStyle(selectedModelLabel, 4, 42)}
              className="composer-select h-8 appearance-none rounded-full border-transparent bg-slate-50 pl-8 pr-8 text-xs"
            >
              {selectedAgentModels.map((model) => (
                <option key={model.id} value={model.id}>
                  {model.label}
                </option>
              ))}
            </Select>
            <ChevronDown className="pointer-events-none absolute right-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-slate-500" />
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
                onChange={(event) => onChangeAgentReasoning(event.target.value)}
                disabled={modelControlsDisabled}
                style={composerSelectWidthStyle(selectedReasoningLabel, 4, 24)}
                className="composer-select h-8 appearance-none rounded-full border-transparent bg-slate-50 pl-8 pr-8 text-xs"
              >
                {selectedAgentModelOption.reasoningLevels.map((level) => (
                  <option key={level.id} value={level.id}>
                    {level.label}
                  </option>
                ))}
              </Select>
              <ChevronDown className="pointer-events-none absolute right-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-slate-500" />
            </div>
          ) : null}
          <span className="min-w-4 flex-1" />
          <ContextWindowMeter usage={selectedChat?.contextWindow} />
          {hasComposerPayload || isRunning ? (
            <Button
              data-testid="send-button"
              type="submit"
              size="icon"
              disabled={!selectedChat || connectionState !== 'open' || (!hasComposerPayload && !isRunning)}
              className={`h-9 w-9 shrink-0 rounded-full ${isRunning && !hasComposerPayload ? 'bg-orange-600 hover:bg-orange-500' : ''}`}
              aria-label={isRunning && !hasComposerPayload ? '停止' : '发送'}
            >
              {isRunning && !hasComposerPayload ? <Square className="h-4 w-4" fill="currentColor" /> : <ArrowUp className="h-4 w-4" />}
            </Button>
          ) : null}
        </div>
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

// skillOptionID 使用 menuID 和 skillID 参数生成 skill 选项 DOM ID。
function skillOptionID(menuID: string, skillID: string) {
  return `${menuID}-skill-${encodeURIComponent(skillID)}`
}

// visualTextLength 使用 text 参数估算中英文混排文本的视觉宽度。
function visualTextLength(text: string) {
  return Array.from(text || '').reduce((width, char) => width + (char.charCodeAt(0) > 255 ? 2 : 1), 0)
}
