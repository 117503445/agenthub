import type { FormEvent } from 'react'
import { Plus } from 'lucide-react'
import { Input } from './ui/input'
import type { AgentModelOption, ConnectionState } from '../types'

interface AgentSettingsPageProps {
  /** connectionState 表示当前 WebSocket 连接状态。 */
  connectionState: ConnectionState
  /** claudeCodeModels 表示 Claude Code 模型列表。 */
  claudeCodeModels: AgentModelOption[]
  /** newClaudeModelID 表示新增模型标识。 */
  newClaudeModelID: string
  /** newClaudeModelLabel 表示新增模型展示名称。 */
  newClaudeModelLabel: string
  /** onBackToChat 返回聊天页。 */
  onBackToChat: () => void
  /** onModelIDChange 使用 value 参数更新新增模型标识。 */
  onModelIDChange: (value: string) => void
  /** onModelLabelChange 使用 value 参数更新新增模型展示名称。 */
  onModelLabelChange: (value: string) => void
  /** onAddClaudeModel 使用 event 参数提交新增模型。 */
  onAddClaudeModel: (event: FormEvent<HTMLFormElement>) => void
}

// AgentSettingsPage 使用 props 参数渲染 agent 设置页。
export function AgentSettingsPage({
  connectionState,
  claudeCodeModels,
  newClaudeModelID,
  newClaudeModelLabel,
  onBackToChat,
  onModelIDChange,
  onModelLabelChange,
  onAddClaudeModel,
}: AgentSettingsPageProps) {
  return (
    <div className="flex h-full min-h-0 min-w-0 flex-col" data-testid="agent-settings-page">
      <header className="flex min-h-16 items-center justify-between border-b border-slate-200 bg-white px-4">
        <div>
          <h2 className="text-lg font-semibold">Agent 设置</h2>
          <p className="text-xs text-slate-500">维护 Claude Code 可选模型</p>
        </div>
        <button
          data-testid="back-to-chat-button"
          type="button"
          onClick={onBackToChat}
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
          <form className="rounded-md border border-slate-200 bg-white p-4" onSubmit={onAddClaudeModel}>
            <h3 className="text-base font-semibold text-slate-900">新增模型</h3>
            <label htmlFor="agent-model-id-input" className="mt-4 block text-xs font-medium text-slate-500">
              模型标识
            </label>
            <Input
              id="agent-model-id-input"
              data-testid="agent-model-id-input"
              value={newClaudeModelID}
              onChange={(event) => onModelIDChange(event.target.value)}
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
              onChange={(event) => onModelLabelChange(event.target.value)}
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
  )
}
