export type ConnectionState = 'connecting' | 'open' | 'closed' | 'error'
export type ChatStatus = 'idle' | 'running' | 'error'
export type ChatTerminalIndicator = 'success' | 'error'
export type ChatVisualStatus = 'running' | ChatTerminalIndicator
export type MessageRole = 'user' | 'assistant' | 'system'
export type MessageStatus = 'complete' | 'streaming' | 'stopped' | 'error'
export type AgentProvider = string
export type AgentProfileType = 'claude_code' | 'codex'
export type AgentBuiltinProfileKind = 'claude_code' | 'codex' | 'mock_claude_code' | 'mock_codex'

export interface MessageImage {
  /** id 表示图片附件唯一标识。 */
  id: string
  /** fileName 表示图片文件名。 */
  fileName: string
  /** mimeType 表示图片 MIME 类型。 */
  mimeType: string
  /** data 表示图片 base64 内容。 */
  data: string
  /** createdAt 表示创建时间。 */
  createdAt?: string
  /** updatedAt 表示更新时间。 */
  updatedAt?: string
}

export interface ComposerImageAttachment {
  /** id 表示前端草稿中的图片附件标识。 */
  id: string
  /** fileName 表示图片文件名。 */
  fileName: string
  /** mimeType 表示图片 MIME 类型。 */
  mimeType: string
  /** data 表示图片 base64 内容。 */
  data: string
  /** previewUrl 表示浏览器可直接显示的预览地址。 */
  previewUrl: string
}

export interface PlanApproval {
  /** id 表示待确认 plan 标识。 */
  id: string
  /** messageId 表示生成该 plan 的 assistant 消息标识。 */
  messageId: string
  /** text 表示 plan 正文。 */
  text: string
  /** status 表示 plan 当前状态。 */
  status: 'pending' | 'executing' | 'done'
  /** createdAt 表示创建时间。 */
  createdAt: string
  /** updatedAt 表示更新时间。 */
  updatedAt: string
}

export interface AgentModelOption {
  /** id 表示传递给 agent CLI 的模型标识。 */
  id: string
  /** label 表示模型展示值，与 id 保持一致。 */
  label: string
  /** default 表示是否为 provider 默认模型。 */
  default?: boolean
  /** reasoningLevels 表示模型支持的推理级别。 */
  reasoningLevels?: AgentReasoningOption[]
}

export interface AgentReasoningOption {
  /** id 表示传递给 agent CLI 的推理级别标识。 */
  id: string
  /** label 表示界面展示名称。 */
  label: string
  /** description 表示推理级别说明。 */
  description: string
  /** default 表示是否为默认推理级别。 */
  default?: boolean
}

export interface AgentProviderOption {
  /** id 表示 provider 标识。 */
  id: AgentProvider
  /** label 表示界面展示名称。 */
  label: string
  /** models 表示 provider 可选模型。 */
  models: AgentModelOption[]
}

export interface AgentEnvVar {
  /** name 表示环境变量名。 */
  name: string
  /** value 表示环境变量完整值。 */
  value?: string
  /** unset 表示是否删除后端同名环境变量。 */
  unset?: boolean
}

export interface BackendEnvVar {
  /** name 表示后端启动环境变量名。 */
  name: string
  /** value 表示后端启动环境变量完整值。 */
  value: string
}

export interface AgentProfile {
  /** id 表示 Profile 唯一标识。 */
  id: string
  /** label 表示 Profile 展示名称。 */
  label: string
  /** type 表示 Profile 类型。 */
  type: AgentProfileType
  /** command 表示启动命令。 */
  command: string
  /** args 表示固定命令参数。 */
  args: string[]
  /** env 表示 Profile 环境变量配置。 */
  env: AgentEnvVar[]
  /** models 表示聊天中可切换的模型。 */
  models: AgentModelOption[]
  /** builtin 表示是否内置 Profile。 */
  builtin?: boolean
}

export interface AgentSkillOption {
  /** id 表示 skill 命令标识。 */
  id: string
  /** label 表示界面展示名称。 */
  label: string
  /** description 表示 skill 说明。 */
  description: string
  /** path 表示 SKILL.md 文件路径。 */
  path?: string
}

export interface LastAgentSelection {
  /** provider 表示最近一次选择的 agent provider。 */
  provider: AgentProvider
  /** model 表示最近一次选择的模型。 */
  model: string
  /** reasoning 表示最近一次选择的推理级别。 */
  reasoning: string
}

export interface Project {
  /** id 表示 project 唯一标识。 */
  id: string
  /** name 表示 project 展示名称。 */
  name: string
  /** path 表示后端本机工作目录。 */
  path: string
  /** git 表示 project 当前 Git 摘要。 */
  git?: ProjectGitInfo
  /** sortOrder 表示侧栏排序值，值越小越靠前。 */
  sortOrder: number
  /** createdAt 表示创建时间。 */
  createdAt: string
  /** updatedAt 表示更新时间。 */
  updatedAt: string
}

export interface ProjectGitInfo {
  /** isRepo 表示当前目录是否位于 Git 仓库中。 */
  isRepo: boolean
  /** branch 表示当前分支或 HEAD 状态。 */
  branch: string
  /** commit 表示当前短提交哈希。 */
  commit: string
  /** dirty 表示工作区是否有未提交内容。 */
  dirty: boolean
}

export interface ToolCall {
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
  /** userInputRequest 表示 agent 请求用户输入的问题。 */
  userInputRequest?: UserInputRequest
  /** createdAt 表示创建时间。 */
  createdAt: string
  /** updatedAt 表示更新时间。 */
  updatedAt: string
}

export interface UserInputOption {
  /** label 表示选项文案。 */
  label: string
  /** description 表示选项说明。 */
  description?: string
}

export interface UserInputQuestion {
  /** id 表示问题标识。 */
  id: string
  /** header 表示问题短标题。 */
  header: string
  /** question 表示问题正文。 */
  question: string
  /** options 表示可选答案列表。 */
  options?: UserInputOption[]
  /** isOther 表示是否允许用户填写其他答案。 */
  isOther?: boolean
  /** isSecret 表示答案是否应作为敏感信息处理。 */
  isSecret?: boolean
}

export interface UserInputRequest {
  /** id 表示请求标识。 */
  id: string
  /** questions 表示本次请求包含的问题。 */
  questions: UserInputQuestion[]
  /** answers 表示按问题 ID 记录的用户答案。 */
  answers?: Record<string, string[]>
  /** createdAt 表示创建时间。 */
  createdAt?: string
  /** updatedAt 表示更新时间。 */
  updatedAt?: string
}

export interface MessagePart {
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

export interface ChatMessage {
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
  /** images 表示用户消息携带的图片附件。 */
  images?: MessageImage[]
  /** createdAt 表示创建时间。 */
  createdAt: string
  /** updatedAt 表示更新时间。 */
  updatedAt: string
}

export interface AgentUsage {
  /** inputTokens 表示输入 token 数。 */
  inputTokens?: number
  /** cachedInputTokens 表示缓存命中的输入 token 数。 */
  cachedInputTokens?: number
  /** outputTokens 表示输出 token 数。 */
  outputTokens?: number
  /** contextWindowMaxTokens 表示模型上下文窗口上限。 */
  contextWindowMaxTokens?: number
  /** contextWindowUsedTokens 表示当前已使用上下文 token 数。 */
  contextWindowUsedTokens?: number
  /** contextWindowPercentRounded 表示上下文窗口使用率整数百分比。 */
  contextWindowPercentRounded?: number
}

export interface Chat {
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
  /** agentProfile 表示聊天页绑定的 Profile 快照。 */
  agentProfile?: AgentProfile
  /** usage 表示最近一次 agent 用量和上下文窗口。 */
  usage?: AgentUsage
  /** plan 表示当前待确认或执行中的 plan。 */
  plan?: PlanApproval
  /** draftText 表示输入框尚未发送的文字草稿。 */
  draftText?: string
  /** messages 表示消息列表。 */
  messages: ChatMessage[]
  /** detailLoaded 表示前端是否已经加载该聊天页详情。 */
  detailLoaded?: boolean
  /** detailLoadedAt 表示前端加载详情时对应的聊天更新时间。 */
  detailLoadedAt?: string
  /** createdAt 表示创建时间。 */
  createdAt: string
  /** updatedAt 表示更新时间。 */
  updatedAt: string
}

export interface ChatScrollMemory {
  /** scrollTop 表示消息列表的垂直滚动位置。 */
  scrollTop: number
  /** signature 表示保存滚动位置时的聊天内容签名。 */
  signature: string
}

export interface SnapshotPayload {
  /** projects 表示所有 project。 */
  projects: Project[]
  /** chats 表示所有聊天页。 */
  chats: Chat[]
  /** agentProviders 表示可选 agent 和模型。 */
  agentProviders?: AgentProviderOption[]
  /** agentProfiles 表示可编辑的 Profile 列表。 */
  agentProfiles?: AgentProfile[]
  /** backendEnv 表示后端启动环境变量。 */
  backendEnv?: BackendEnvVar[]
  /** agentSkills 表示可选 skills。 */
  agentSkills?: AgentSkillOption[]
  /** lastAgentSelection 表示后端保存的上次 agent 配置。 */
  lastAgentSelection?: LastAgentSelection
}

export interface ProjectChangedPayload {
  /** project 表示变更后的 project。 */
  project: Project
}

export interface ProjectsReorderedPayload {
  /** projects 表示重排序后的完整 project 列表。 */
  projects: Project[]
}

export interface ProjectDeletedPayload {
  /** id 表示被删除的 project 标识。 */
  id: string
  /** chatIds 表示被删除的聊天页标识。 */
  chatIds: string[]
}

export interface ChatChangedPayload {
  /** chat 表示变更后的聊天页。 */
  chat: Chat
}

export interface ChatDetailPayload {
  /** chat 表示完整聊天页详情。 */
  chat: Chat
}

export interface ChatDeletedPayload {
  /** id 表示被删除的聊天页标识。 */
  id: string
  /** projectId 表示被删除聊天页所属 project。 */
  projectId: string
}

export interface ChatMessageDeltaPayload {
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

export interface ChatMessageDonePayload {
  /** chatId 表示聊天页标识。 */
  chatId: string
  /** message 表示完成后的消息。 */
  message: ChatMessage
}

export interface AgentStatusPayload {
  /** chatId 表示聊天页标识。 */
  chatId: string
  /** status 表示聊天页运行状态。 */
  status: ChatStatus
}

export interface AgentProvidersChangedPayload {
  /** agentProviders 表示更新后的 agent 和模型选项。 */
  agentProviders: AgentProviderOption[]
}

export interface AgentProfilesChangedPayload {
  /** agentProfiles 表示更新后的 Profile 列表。 */
  agentProfiles: AgentProfile[]
  /** agentProviders 表示兼容聊天选择框的 Profile 选项。 */
  agentProviders: AgentProviderOption[]
}

export interface AgentSkillsChangedPayload {
  /** agentSkills 表示更新后的 skill 选项。 */
  agentSkills: AgentSkillOption[]
}

export interface HashRoute {
  /** view 表示当前 hash 路由视图。 */
  view: 'chat' | 'settings'
  /** projectId 表示 hash 路由中的 project 标识。 */
  projectId: string
  /** chatId 表示 hash 路由中的聊天页标识。 */
  chatId: string
}
