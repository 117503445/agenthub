import { GitBranch, GitCommit, MessageSquare, Plus, X } from 'lucide-react'
import { Button } from './ui/button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from './ui/tabs'
import { Composer } from './Composer'
import { MessageLog } from './MessageLog'
import { StatusDot } from './StatusDot'
import { chatVisualStatus, projectGitText } from '../lib/chat'
import type {
  AgentModelOption,
  AgentProvider,
  AgentProviderOption,
  AgentSkillOption,
  Chat,
  ChatMessage,
  ChatTerminalIndicator,
  ConnectionState,
  Project,
} from '../types'
import type { FormEvent } from 'react'

interface ChatWorkspaceProps {
  /** selectedProject 表示当前 project。 */
  selectedProject: Project | null
  /** selectedChat 表示当前聊天页。 */
  selectedChat: Chat | null
  /** projectChats 表示当前 project 下的聊天页。 */
  projectChats: Chat[]
  /** activeProjectId 表示当前 project 标识。 */
  activeProjectId: string
  /** connectionState 表示当前 WebSocket 连接状态。 */
  connectionState: ConnectionState
  /** chatIndicators 表示聊天页终态提示。 */
  chatIndicators: Record<string, ChatTerminalIndicator>
  /** composerValue 表示输入框内容。 */
  composerValue: string
  /** copiedMessageId 表示刚复制成功的消息标识。 */
  copiedMessageId: string
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
  /** onSelectChat 使用 chat 参数切换聊天页。 */
  onSelectChat: (chat: Chat) => void
  /** onDeleteChat 使用 chat 参数删除聊天页。 */
  onDeleteChat: (chat: Chat) => void
  /** onCreateChat 创建聊天页。 */
  onCreateChat: () => void
  /** onClearChatIndicator 使用 chatId 参数清除终态提示。 */
  onClearChatIndicator: (chatId: string) => void
  /** onCopyMessage 使用 message 参数复制消息。 */
  onCopyMessage: (message: ChatMessage) => void
  /** onComposerValueChange 使用 value 参数更新输入框内容。 */
  onComposerValueChange: (value: string) => void
  /** onSubmitComposer 使用 event 参数提交输入框。 */
  onSubmitComposer: (event?: FormEvent<HTMLFormElement>) => void
  /** onChangeAgentProvider 使用 provider 参数切换 agent。 */
  onChangeAgentProvider: (provider: AgentProvider) => void
  /** onChangeAgentModel 使用 model 参数切换模型。 */
  onChangeAgentModel: (model: string) => void
  /** onChangeAgentReasoning 使用 reasoning 参数切换推理级别。 */
  onChangeAgentReasoning: (reasoning: string) => void
}

// ChatWorkspace 使用 props 参数渲染聊天主工作区。
export function ChatWorkspace({
  selectedProject,
  selectedChat,
  projectChats,
  activeProjectId,
  connectionState,
  chatIndicators,
  composerValue,
  copiedMessageId,
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
  onSelectChat,
  onDeleteChat,
  onCreateChat,
  onClearChatIndicator,
  onCopyMessage,
  onComposerValueChange,
  onSubmitComposer,
  onChangeAgentProvider,
  onChangeAgentModel,
  onChangeAgentReasoning,
}: ChatWorkspaceProps) {
  return (
    <>
      <header className="flex min-h-14 items-center border-b border-slate-200 bg-white px-4" data-testid="project-meta">
        <div className="flex min-w-0 flex-1 items-center gap-3 text-xs text-slate-500">
          <span data-testid="project-path-text" className="min-w-0 truncate font-mono text-sm font-medium text-slate-900">
            {selectedProject?.path ?? '选择或创建 Project'}
          </span>
          <span data-testid="project-git-info" className="inline-flex min-w-0 shrink-0 items-center gap-2">
            <span className="inline-flex min-w-0 items-center gap-1">
              <GitBranch className="h-3.5 w-3.5 shrink-0" />
              <span className="truncate">{projectGitText(selectedProject)}</span>
            </span>
            {selectedProject?.git?.isRepo ? (
              <span className="inline-flex items-center gap-1 font-mono">
                <GitCommit className="h-3.5 w-3.5" />
                <span data-testid="project-commit-text">{selectedProject.git.commit || '-'}</span>
              </span>
            ) : null}
          </span>
        </div>
      </header>

      <Tabs
        value={selectedChat?.id ?? ''}
        onValueChange={(chatId) => {
          const chat = projectChats.find((item) => item.id === chatId)
          if (chat) {
            onSelectChat(chat)
          }
        }}
        className="min-h-0 flex-1 overflow-hidden"
      >
        <div className="flex min-h-8 items-stretch border-b border-slate-200 bg-slate-50 px-2" data-testid="chat-tabs">
          <TabsList className="h-full flex-1">
            {projectChats.length === 0 ? (
              <span className="flex items-center text-xs text-slate-500">没有聊天页</span>
            ) : (
              projectChats.map((chat) => {
                const status = chatVisualStatus(chat, chatIndicators)
                return (
                  <TabsTrigger key={chat.id} value={chat.id} data-testid="chat-tab" className="group/tab max-w-60 pr-1">
                    <MessageSquare className="h-3.5 w-3.5 shrink-0" />
                    <span className="truncate">{chat.title}</span>
                    {status ? <StatusDot status={status} testID="chat-status-dot" /> : null}
                    <span
                      data-testid="chat-tab-close-button"
                      role="button"
                      tabIndex={0}
                      onClick={(event) => {
                        event.stopPropagation()
                        onDeleteChat(chat)
                      }}
                      onKeyDown={(event) => {
                        if (event.key !== 'Enter' && event.key !== ' ') {
                          return
                        }
                        event.preventDefault()
                        event.stopPropagation()
                        onDeleteChat(chat)
                      }}
                      className="inline-flex h-5 w-5 shrink-0 cursor-pointer items-center justify-center rounded text-slate-400 opacity-80 transition hover:bg-slate-200 hover:text-slate-900 group-hover/tab:opacity-100"
                      aria-label="关闭聊天页"
                      title="关闭"
                    >
                      <X className="h-3.5 w-3.5" />
                    </span>
                  </TabsTrigger>
                )
              })
            )}
            <Button
              data-testid="chat-tab-add-button"
              variant="ghost"
              size="icon"
              onClick={onCreateChat}
              disabled={!activeProjectId || connectionState !== 'open'}
              className="my-0.5 h-7 w-7 shrink-0"
              aria-label="新建聊天"
            >
              <Plus className="h-3.5 w-3.5" />
            </Button>
          </TabsList>
        </div>

        {selectedChat ? (
          <TabsContent value={selectedChat.id} onClickCapture={() => onClearChatIndicator(selectedChat.id)}>
            <MessageLog chat={selectedChat} copiedMessageId={copiedMessageId} onCopyMessage={onCopyMessage} />
            <Composer
              selectedChat={selectedChat}
              connectionState={connectionState}
              composerValue={composerValue}
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
              onComposerValueChange={onComposerValueChange}
              onSubmitComposer={onSubmitComposer}
              onChangeAgentProvider={onChangeAgentProvider}
              onChangeAgentModel={onChangeAgentModel}
              onChangeAgentReasoning={onChangeAgentReasoning}
            />
          </TabsContent>
        ) : (
          <div className="min-h-0 flex-1 overflow-y-auto px-4 py-5" data-testid="message-log" aria-live="polite" />
        )}
      </Tabs>
    </>
  )
}
