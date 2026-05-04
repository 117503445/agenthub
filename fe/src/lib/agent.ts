import type { AgentProvider, AgentProviderOption, Chat } from '../types'

export const fallbackAgentProviders: AgentProviderOption[] = [
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
]

// defaultModelForProvider 使用 providers 和 provider 参数返回默认模型。
export function defaultModelForProvider(providers: AgentProviderOption[], provider: AgentProvider) {
  const models = providers.find((item) => item.id === provider)?.models ?? []
  return models.find((model) => model.default)?.id ?? models[0]?.id ?? ''
}

// defaultReasoningForModel 使用 providers、provider 和 model 参数返回默认推理级别。
export function defaultReasoningForModel(providers: AgentProviderOption[], provider: AgentProvider, model: string) {
  const levels =
    providers.find((item) => item.id === provider)?.models.find((item) => item.id === model)?.reasoningLevels ?? []
  return levels.find((level) => level.default)?.id ?? levels[0]?.id ?? ''
}

// chatHasStarted 使用 chat 参数判断当前聊天页是否已经发送过消息。
export function chatHasStarted(chat: Chat | null) {
  return Boolean(chat?.messages.some((message) => message.role === 'user'))
}

// normalizeChat 使用 chat 参数确保聊天页数据结构完整。
export function normalizeChat(chat: Chat) {
  return {
    ...chat,
    agentProvider: chat.agentProvider ?? 'claude-code',
    agentModel: chat.agentModel ?? 'sonnet',
    agentReasoning: chat.agentReasoning ?? '',
    agentLocked: chat.agentLocked ?? (chat.messages?.length ?? 0) > 0,
    contextWindow: chat.contextWindow,
    plan: chat.plan,
    messages: chat.messages ?? [],
  }
}
