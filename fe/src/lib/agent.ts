import type { AgentEnvVar, AgentProvider, AgentProviderOption, BackendEnvVar, Chat } from '../types'

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
  return Boolean(chat?.agentLocked || chat?.messages.some((message) => message.role === 'user'))
}

// normalizeChat 使用 chat 参数确保聊天页数据结构完整。
export function normalizeChat(chat: Chat) {
  return {
    ...chat,
    agentProvider: chat.agentProvider ?? 'claude-code',
    agentModel: chat.agentModel ?? 'sonnet',
    agentReasoning: chat.agentReasoning ?? '',
    agentLocked: chat.agentLocked ?? (chat.messages?.length ?? 0) > 0,
    agentProfile: chat.agentProfile,
    plan: chat.plan,
    messages: chat.messages ?? [],
  }
}

// effectiveEnvForProfile 使用 backendEnv 和 profileEnv 参数计算 Profile 实际环境变量。
export function effectiveEnvForProfile(backendEnv: BackendEnvVar[], profileEnv: AgentEnvVar[]) {
  const env = new Map<string, string>()
  for (const item of backendEnv) {
    if (item.name.trim()) {
      env.set(item.name, item.value)
    }
  }
  for (const item of profileEnv) {
    const name = item.name.trim()
    if (!name) {
      continue
    }
    if (item.unset) {
      env.delete(name)
      continue
    }
    env.set(name, item.value ?? '')
  }
  return Array.from(env.entries())
    .map(([name, value]) => ({ name, value }))
    .sort((left, right) => left.name.localeCompare(right.name))
}
