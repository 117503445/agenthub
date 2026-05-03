import type { FormEvent, ReactNode } from 'react'
import { Folder, Monitor, Plus, Settings, Trash2, X } from 'lucide-react'
import { Button } from './ui/button'
import { Input } from './ui/input'
import { projectDisplayName } from '../lib/chat'
import { StatusDot } from './StatusDot'
import type { ChatVisualStatus, ConnectionState, Project } from '../types'

const connectionText: Record<ConnectionState, string> = {
  connecting: '连接中',
  open: '已连接',
  closed: '已断开',
  error: '连接异常',
}

interface AppSidebarProps {
  /** connectionState 表示当前 WebSocket 连接状态。 */
  connectionState: ConnectionState
  /** connectionIcon 表示连接状态图标。 */
  connectionIcon: ReactNode
  /** hostname 表示后端机器名。 */
  hostname: string
  /** projects 表示 project 列表。 */
  projects: Project[]
  /** selectedProjectId 表示当前选中 project。 */
  selectedProjectId: string
  /** projectVisualStatuses 表示 project 聚合状态。 */
  projectVisualStatuses: Map<string, ChatVisualStatus>
  /** projectDialogOpen 表示是否打开 project 表单。 */
  projectDialogOpen: boolean
  /** projectPath 表示 project 表单路径。 */
  projectPath: string
  /** errorText 表示 project 表单错误。 */
  errorText: string
  /** routeView 表示当前页面视图。 */
  routeView: 'chat' | 'settings'
  /** onProjectPathChange 使用 value 参数更新 project 路径。 */
  onProjectPathChange: (value: string) => void
  /** onProjectSave 使用 event 参数保存 project。 */
  onProjectSave: (event: FormEvent<HTMLFormElement>) => void
  /** onProjectDialogOpen 打开 project 表单。 */
  onProjectDialogOpen: () => void
  /** onProjectDialogClose 关闭 project 表单。 */
  onProjectDialogClose: () => void
  /** onProjectSelect 使用 project 参数选中 project。 */
  onProjectSelect: (project: Project) => void
  /** onProjectDelete 使用 project 参数删除 project。 */
  onProjectDelete: (project: Project) => void
  /** onSettingsOpen 打开设置页。 */
  onSettingsOpen: () => void
}

// AppSidebar 使用 props 参数渲染左侧 project 导航。
export function AppSidebar({
  connectionState,
  connectionIcon,
  hostname,
  projects,
  selectedProjectId,
  projectVisualStatuses,
  projectDialogOpen,
  projectPath,
  errorText,
  routeView,
  onProjectPathChange,
  onProjectSave,
  onProjectDialogOpen,
  onProjectDialogClose,
  onProjectSelect,
  onProjectDelete,
  onSettingsOpen,
}: AppSidebarProps) {
  return (
    <aside data-testid="sidebar" className="relative flex h-full min-h-0 flex-col border-r border-slate-800 bg-slate-950 text-slate-100">
      <div className="border-b border-slate-800 px-3 py-3">
        <div data-testid="sidebar-identity" className="flex min-w-0 items-center gap-2 text-xs text-slate-500">
          <Monitor className="h-3.5 w-3.5 shrink-0" />
          <span data-testid="machine-name" className="min-w-0 flex-1 truncate font-mono">
            {hostname || 'unknown'}
          </span>
          <span data-testid="connection-state" className="inline-flex shrink-0 items-center gap-1.5">
            {connectionIcon}
            <span>{connectionText[connectionState]}</span>
          </span>
        </div>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto px-2 py-3" data-testid="project-list">
        {projects.length === 0 ? (
          <div className="rounded-md border border-dashed border-slate-700 px-3 py-8 text-center text-sm text-slate-500">
            还没有 Project
          </div>
        ) : (
          projects.map((project) => {
            const projectStatus = projectVisualStatuses.get(project.id)
            return (
              <div
                key={project.id}
                className={`mb-1 rounded-md border px-2 py-2 transition ${
                  project.id === selectedProjectId
                    ? 'border-teal-500 bg-teal-500/10'
                    : 'border-transparent hover:border-slate-700 hover:bg-slate-900'
                }`}
              >
                <div className="flex items-center gap-2">
                  <button
                    type="button"
                    onClick={() => onProjectSelect(project)}
                    className="flex min-w-0 flex-1 cursor-pointer items-center gap-2 text-left"
                  >
                    <Folder className="h-4 w-4 shrink-0 text-teal-400" />
                    <span data-testid="project-name" className="min-w-0 flex-1 truncate text-sm font-medium text-slate-100">
                      {projectDisplayName(project)}
                    </span>
                    {projectStatus ? <StatusDot status={projectStatus} testID="project-status-dot" /> : null}
                  </button>
                  <button
                    data-testid="project-delete-button"
                    type="button"
                    onClick={() => onProjectDelete(project)}
                    className="inline-flex h-7 w-7 shrink-0 cursor-pointer items-center justify-center rounded-md text-slate-400 transition hover:bg-rose-500/15 hover:text-rose-300"
                    aria-label="删除 Project"
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </button>
                </div>
              </div>
            )
          })
        )}
      </div>

      {projectDialogOpen ? (
        <div className="absolute bottom-[4.5rem] left-3 right-3 z-20">
          <form className="rounded-lg border border-slate-200 bg-white p-3 text-slate-900 shadow-lg" onSubmit={onProjectSave}>
            <div className="mb-2 flex items-center justify-between gap-2">
              <label htmlFor="project-path-input" className="text-xs font-medium text-slate-500">
                工作目录
              </label>
              <button
                type="button"
                onClick={onProjectDialogClose}
                className="inline-flex h-7 w-7 cursor-pointer items-center justify-center rounded-md text-slate-400 transition hover:bg-slate-100 hover:text-slate-900"
                aria-label="关闭"
              >
                <X className="h-4 w-4" />
              </button>
            </div>
            <Input
              id="project-path-input"
              data-testid="project-path-input"
              value={projectPath}
              onChange={(event) => onProjectPathChange(event.target.value)}
              className="h-9 w-full rounded-md border border-slate-200 bg-white px-3 font-mono text-sm text-slate-900 outline-none transition placeholder:text-slate-400 focus:border-teal-600 focus:ring-2 focus:ring-teal-100"
              placeholder="/workspace/project/agenthub"
            />
            <button
              data-testid="project-save-button"
              type="submit"
              disabled={connectionState !== 'open' || !projectPath.trim()}
              className="mt-3 inline-flex h-9 w-full cursor-pointer items-center justify-center gap-2 rounded-md bg-teal-600 px-3 text-sm font-medium text-white transition hover:bg-teal-500 focus:outline-none focus:ring-2 focus:ring-teal-400 disabled:cursor-not-allowed disabled:bg-slate-300 disabled:text-slate-500"
            >
              <Plus className="h-4 w-4" />
              添加
            </button>
            {errorText ? <p className="mt-2 text-xs text-rose-600">{errorText}</p> : null}
          </form>
        </div>
      ) : null}

      <div data-testid="sidebar-footer" className="border-t border-slate-800 p-3">
        <div className="flex gap-2">
          <Button
            data-testid="project-add-button"
            type="button"
            variant="outline"
            onClick={onProjectDialogOpen}
            className="h-9 flex-1 px-0"
            aria-label="添加 Project"
            title="添加 Project"
          >
            <Plus className="h-4 w-4" />
          </Button>
          <Button
            data-testid="agent-settings-button"
            type="button"
            variant={routeView === 'settings' ? 'default' : 'outline'}
            onClick={onSettingsOpen}
            className="h-9 flex-1 px-0"
            aria-label="设置"
            title="设置"
          >
            <Settings className="h-4 w-4" />
          </Button>
        </div>
      </div>
    </aside>
  )
}
