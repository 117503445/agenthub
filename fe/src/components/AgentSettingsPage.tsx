import { FormEvent, useEffect, useMemo, useState } from 'react'
import { Plus, Save, Trash2 } from 'lucide-react'
import { effectiveEnvForProfile } from '../lib/agent'
import { Input } from './ui/input'
import type {
  AgentBuiltinProfileKind,
  AgentEnvVar,
  AgentProfile,
  AgentProfileType,
  BackendEnvVar,
  ConnectionState,
} from '../types'

interface AgentSettingsPageProps {
  /** connectionState 表示当前 WebSocket 连接状态。 */
  connectionState: ConnectionState
  /** backendVersion 表示后端构建版本。 */
  backendVersion: string
  /** hostname 表示后端机器名。 */
  hostname: string
  /** agentProfiles 表示可编辑的 Profile 列表。 */
  agentProfiles: AgentProfile[]
  /** backendEnv 表示后端启动环境变量。 */
  backendEnv: BackendEnvVar[]
  /** selectedProfileId 表示当前选中的 Profile 标识。 */
  selectedProfileId: string
  /** newClaudeModelID 表示新增模型值。 */
  newClaudeModelID: string
  /** onBackToChat 返回聊天页。 */
  onBackToChat: () => void
  /** onProfileSelect 使用 profileId 参数选中 Profile。 */
  onProfileSelect: (profileId: string) => void
  /** onProfileCreate 新增自定义 Profile。 */
  onProfileCreate: () => void
  /** onProfileSave 使用 profile 参数保存 Profile。 */
  onProfileSave: (profile: AgentProfile) => void
  /** onProfileDelete 使用 profileId 参数删除 Profile。 */
  onProfileDelete: (profileId: string) => void
  /** onBuiltinAdd 使用 kind 参数添加内置 Profile。 */
  onBuiltinAdd: (kind: AgentBuiltinProfileKind) => void
  /** onModelIDChange 使用 value 参数更新新增模型值。 */
  onModelIDChange: (value: string) => void
  /** onAddProfileModel 使用 event 参数提交新增模型。 */
  onAddProfileModel: (event: FormEvent<HTMLFormElement>) => void
  /** onDeleteProfileModel 使用 profileId 和 modelId 参数删除模型。 */
  onDeleteProfileModel: (profileId: string, modelId: string) => void
}

// AgentSettingsPage 使用 props 参数渲染 agent Profile 设置页。
export function AgentSettingsPage({
  connectionState,
  backendVersion,
  hostname,
  agentProfiles,
  backendEnv,
  selectedProfileId,
  newClaudeModelID,
  onBackToChat,
  onProfileSelect,
  onProfileCreate,
  onProfileSave,
  onProfileDelete,
  onBuiltinAdd,
  onModelIDChange,
  onAddProfileModel,
  onDeleteProfileModel,
}: AgentSettingsPageProps) {
  const selectedProfile = agentProfiles.find((profile) => profile.id === selectedProfileId) ?? agentProfiles[0] ?? null
  const [labelDraft, setLabelDraft] = useState('')
  const [typeDraft, setTypeDraft] = useState<AgentProfileType>('claude_code')
  const [commandDraft, setCommandDraft] = useState('')
  const [argsDraft, setArgsDraft] = useState('')
  const [envDraft, setEnvDraft] = useState('')

  useEffect(() => {
    setLabelDraft(selectedProfile?.label ?? '')
    setTypeDraft(selectedProfile?.type ?? 'claude_code')
    setCommandDraft(selectedProfile?.command ?? '')
    setArgsDraft((selectedProfile?.args ?? []).join('\n'))
    setEnvDraft(envToText(selectedProfile?.env ?? []))
  }, [selectedProfile?.id, selectedProfile?.label, selectedProfile?.type, selectedProfile?.command, selectedProfile?.args, selectedProfile?.env])

  const effectiveEnv = useMemo(
    () => effectiveEnvForProfile(backendEnv, textToEnv(envDraft)),
    [backendEnv, envDraft],
  )
  const models = selectedProfile?.models ?? []

  // saveProfile 保存当前表单中的 Profile 静态配置。
  const saveProfile = () => {
    if (!selectedProfile) {
      return
    }
    onProfileSave({
      ...selectedProfile,
      label: labelDraft.trim() || selectedProfile.id,
      type: typeDraft,
      command: commandDraft.trim(),
      args: argsDraft
        .split('\n')
        .map((line) => line.trim())
        .filter(Boolean),
      env: textToEnv(envDraft),
    })
  }

  return (
    <div className="flex h-full min-h-0 min-w-0 flex-col" data-testid="agent-settings-page">
      <header className="flex min-h-16 items-center justify-between border-b border-slate-200 bg-white px-4">
        <div>
          <h2 className="text-lg font-semibold">Agent Profile</h2>
          <p className="text-xs text-slate-500">维护运行配置、模型和环境变量</p>
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
        <div className="mx-auto grid max-w-7xl gap-5 xl:grid-cols-[280px_minmax(0,1fr)_360px]">
          <section className="min-w-0">
            <div className="mb-3 flex items-center justify-between">
              <h3 className="text-base font-semibold text-slate-900">Profile</h3>
              <span className="text-xs text-slate-500">{agentProfiles.length} 个</span>
            </div>
            <div className="overflow-hidden rounded-md border border-slate-200 bg-white" data-testid="agent-profile-list">
              {agentProfiles.map((profile) => (
                <button
                  key={profile.id}
                  type="button"
                  onClick={() => onProfileSelect(profile.id)}
                  className={`block w-full cursor-pointer border-b border-slate-100 px-3 py-3 text-left last:border-b-0 ${
                    selectedProfile?.id === profile.id ? 'bg-teal-50' : 'bg-white hover:bg-slate-50'
                  }`}
                >
                  <span className="block truncate text-sm font-medium text-slate-900">{profile.label}</span>
                  <span className="mt-1 block truncate font-mono text-xs text-slate-500">{profile.id}</span>
                  <span className="mt-2 inline-flex items-center rounded-sm bg-slate-100 px-1.5 py-0.5 text-[11px] font-medium text-slate-600">
                    {profile.type}
                  </span>
                </button>
              ))}
              {!agentProfiles.length ? <div className="px-3 py-8 text-center text-sm text-slate-500">暂无 Profile</div> : null}
            </div>
            <div className="mt-3 grid grid-cols-2 gap-2">
              <button
                type="button"
                onClick={onProfileCreate}
                disabled={connectionState !== 'open'}
                className="inline-flex h-9 cursor-pointer items-center justify-center gap-2 rounded-md border border-slate-300 px-3 text-sm font-medium text-slate-700 transition hover:border-slate-500 disabled:cursor-not-allowed disabled:text-slate-300"
              >
                <Plus className="h-4 w-4" />
                新建
              </button>
              <button
                type="button"
                onClick={() => onBuiltinAdd('claude_code')}
                disabled={connectionState !== 'open'}
                className="inline-flex h-9 cursor-pointer items-center justify-center rounded-md border border-slate-300 px-3 text-sm font-medium text-slate-700 transition hover:border-slate-500 disabled:cursor-not-allowed disabled:text-slate-300"
              >
                Claude
              </button>
              <button
                type="button"
                onClick={() => onBuiltinAdd('codex')}
                disabled={connectionState !== 'open'}
                className="inline-flex h-9 cursor-pointer items-center justify-center rounded-md border border-slate-300 px-3 text-sm font-medium text-slate-700 transition hover:border-slate-500 disabled:cursor-not-allowed disabled:text-slate-300"
              >
                Codex
              </button>
              <button
                type="button"
                onClick={() => onBuiltinAdd('mock_codex')}
                disabled={connectionState !== 'open'}
                className="inline-flex h-9 cursor-pointer items-center justify-center rounded-md border border-slate-300 px-3 text-sm font-medium text-slate-700 transition hover:border-slate-500 disabled:cursor-not-allowed disabled:text-slate-300"
              >
                Mock
              </button>
            </div>
          </section>

          <section className="min-w-0">
            <div className="mb-3 flex items-center justify-between">
              <h3 className="text-base font-semibold text-slate-900">运行配置</h3>
              {selectedProfile ? (
                <button
                  type="button"
                  onClick={() => onProfileDelete(selectedProfile.id)}
                  disabled={connectionState !== 'open'}
                  className="inline-flex h-8 cursor-pointer items-center justify-center gap-2 rounded-md border border-red-200 px-2.5 text-xs font-medium text-red-700 transition hover:border-red-300 hover:bg-red-50 disabled:cursor-not-allowed disabled:text-slate-300"
                >
                  <Trash2 className="h-3.5 w-3.5" />
                  删除
                </button>
              ) : null}
            </div>
            <div className="grid gap-4 rounded-md border border-slate-200 bg-white p-4">
              <div className="grid gap-3 md:grid-cols-2">
                <label className="block text-xs font-medium text-slate-500">
                  展示名称
                  <Input
                    value={labelDraft}
                    onChange={(event) => setLabelDraft(event.target.value)}
                    className="mt-1 h-9 w-full rounded-md border border-slate-300 px-3 text-sm"
                  />
                </label>
                <label className="block text-xs font-medium text-slate-500">
                  类型
                  <select
                    value={typeDraft}
                    onChange={(event) => setTypeDraft(event.target.value as AgentProfileType)}
                    className="mt-1 h-9 w-full rounded-md border border-slate-300 bg-white px-3 text-sm"
                  >
                    <option value="claude_code">claude_code</option>
                    <option value="codex">codex</option>
                  </select>
                </label>
              </div>
              <label className="block text-xs font-medium text-slate-500">
                命令
                <Input
                  value={commandDraft}
                  onChange={(event) => setCommandDraft(event.target.value)}
                  className="mt-1 h-9 w-full rounded-md border border-slate-300 px-3 font-mono text-sm"
                />
              </label>
              <label className="block text-xs font-medium text-slate-500">
                固定参数
                <textarea
                  value={argsDraft}
                  onChange={(event) => setArgsDraft(event.target.value)}
                  className="mt-1 min-h-20 w-full resize-y rounded-md border border-slate-300 px-3 py-2 font-mono text-sm outline-none focus:border-teal-600 focus:ring-2 focus:ring-teal-100"
                  spellCheck={false}
                />
              </label>
              <label className="block text-xs font-medium text-slate-500">
                Profile 环境变量
                <textarea
                  value={envDraft}
                  onChange={(event) => setEnvDraft(event.target.value)}
                  className="mt-1 min-h-28 w-full resize-y rounded-md border border-slate-300 px-3 py-2 font-mono text-sm outline-none focus:border-teal-600 focus:ring-2 focus:ring-teal-100"
                  spellCheck={false}
                />
              </label>
              <button
                type="button"
                onClick={saveProfile}
                disabled={connectionState !== 'open' || !selectedProfile}
                className="inline-flex h-9 w-fit cursor-pointer items-center justify-center gap-2 rounded-md bg-teal-600 px-3 text-sm font-medium text-white transition hover:bg-teal-500 disabled:cursor-not-allowed disabled:bg-slate-300"
              >
                <Save className="h-4 w-4" />
                保存 Profile
              </button>
            </div>

            <div className="mt-5">
              <div className="mb-3 flex items-center justify-between">
                <h3 className="text-base font-semibold text-slate-900">模型</h3>
                <span className="text-xs text-slate-500">{models.length} 个选项</span>
              </div>
              <div className="overflow-hidden rounded-md border border-slate-200 bg-white" data-testid="agent-settings-model-list">
                {models.map((model) => (
                  <div key={model.id} className="flex items-center justify-between gap-3 border-b border-slate-100 px-3 py-3 last:border-b-0">
                    <div className="min-w-0">
                      <div className="truncate font-mono text-sm font-medium text-slate-900">{model.id}</div>
                    </div>
                    <div className="flex shrink-0 items-center gap-2">
                      {model.default ? <span className="text-xs font-medium text-teal-700">默认</span> : null}
                      <button
                        type="button"
                        data-testid="agent-model-delete-button"
                        data-model-id={model.id}
                        onClick={() => selectedProfile && onDeleteProfileModel(selectedProfile.id, model.id)}
                        disabled={connectionState !== 'open' || !selectedProfile || models.length <= 1}
                        className="inline-flex h-7 w-7 cursor-pointer items-center justify-center rounded-md text-slate-400 transition hover:bg-red-50 hover:text-red-700 disabled:cursor-not-allowed disabled:text-slate-200"
                        aria-label={`删除模型 ${model.id}`}
                        title="删除模型"
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </button>
                    </div>
                  </div>
                ))}
              </div>
              <form className="mt-3 grid gap-3 rounded-md border border-slate-200 bg-white p-4 md:grid-cols-[minmax(0,1fr)_auto]" onSubmit={onAddProfileModel}>
                <label htmlFor="agent-model-id-input" className="block text-xs font-medium text-slate-500">
                  模型
                  <Input
                    id="agent-model-id-input"
                    data-testid="agent-model-id-input"
                    value={newClaudeModelID}
                    onChange={(event) => onModelIDChange(event.target.value)}
                    className="mt-1 h-9 w-full rounded-md border border-slate-300 px-3 font-mono text-sm"
                    placeholder="claude-sonnet-4-6"
                  />
                </label>
                <button
                  data-testid="agent-model-add-button"
                  type="submit"
                  disabled={connectionState !== 'open' || !selectedProfile || !newClaudeModelID.trim()}
                  className="mt-5 inline-flex h-9 cursor-pointer items-center justify-center gap-2 rounded-md bg-teal-600 px-3 text-sm font-medium text-white transition hover:bg-teal-500 disabled:cursor-not-allowed disabled:bg-slate-300"
                >
                  <Plus className="h-4 w-4" />
                  添加
                </button>
              </form>
            </div>
          </section>

          <div className="grid min-w-0 content-start gap-4">
            <section className="rounded-md border border-slate-200 bg-white p-4" data-testid="agent-settings-backend-info">
              <h3 className="text-base font-semibold text-slate-900">后端信息</h3>
              <dl className="mt-4 grid gap-3 text-sm">
                <InfoItem label="版本" testID="backend-version-text" value={backendVersion || '-'} />
                <InfoItem label="机器名" testID="backend-hostname-text" value={hostname || '-'} />
              </dl>
            </section>
            <EnvTable title="后端启动环境变量" env={backendEnv} testID="agent-settings-backend-env" />
            <EnvTable title="Effective Env" env={effectiveEnv} testID="agent-profile-effective-env" />
          </div>
        </div>
      </div>
    </div>
  )
}

// InfoItem 使用 props 参数渲染一项后端信息。
function InfoItem({ label, value, testID }: { label: string; value: string; testID: string }) {
  return (
    <div className="min-w-0">
      <dt className="text-xs font-medium text-slate-500">{label}</dt>
      <dd className="mt-1 truncate font-mono text-slate-900" data-testid={testID} title={value}>
        {value}
      </dd>
    </div>
  )
}

// EnvTable 使用 title、env 和 testID 参数渲染环境变量表。
function EnvTable({ title, env, testID }: { title: string; env: BackendEnvVar[]; testID: string }) {
  return (
    <section className="rounded-md border border-slate-200 bg-white p-4" data-testid={testID}>
      <div className="mb-3 flex items-center justify-between">
        <h3 className="text-base font-semibold text-slate-900">{title}</h3>
        <span className="text-xs text-slate-500">{env.length} 项</span>
      </div>
      <div className="max-h-72 overflow-auto rounded-md border border-slate-100">
        {env.slice(0, 160).map((item) => (
          <div key={item.name} className="grid grid-cols-[140px_minmax(0,1fr)] gap-2 border-b border-slate-100 px-2 py-2 text-xs last:border-b-0">
            <div className="truncate font-mono font-medium text-slate-600" title={item.name}>
              {item.name}
            </div>
            <div className="truncate font-mono text-slate-900" title={item.value}>
              {item.value}
            </div>
          </div>
        ))}
        {!env.length ? <div className="px-2 py-6 text-center text-sm text-slate-500">暂无环境变量</div> : null}
      </div>
    </section>
  )
}

// envToText 使用 env 参数转换为可编辑文本。
function envToText(env: AgentEnvVar[]) {
  return env
    .map((item) => {
      if (item.unset) {
        return `unset ${item.name}`
      }
      return `${item.name}=${item.value ?? ''}`
    })
    .join('\n')
}

// textToEnv 使用 text 参数解析 Profile 环境变量配置。
function textToEnv(text: string): AgentEnvVar[] {
  return text
    .split('\n')
    .map((line) => line.trim())
    .filter(Boolean)
    .map((line) => {
      if (line.startsWith('unset ')) {
        return { name: line.slice(6).trim(), unset: true }
      }
      const separatorIndex = line.indexOf('=')
      if (separatorIndex < 0) {
        return { name: line, value: '' }
      }
      return { name: line.slice(0, separatorIndex).trim(), value: line.slice(separatorIndex + 1) }
    })
    .filter((item) => item.name)
}
