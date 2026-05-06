import { useState, type DragEvent, type FormEvent, type ReactNode } from 'react'
import { Folder, GripVertical, Monitor, Plus, Settings, Trash2, X } from 'lucide-react'
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
  /** onProjectReorder 使用 sourceProjectId、targetProjectId 和 placement 参数调整 project 顺序。 */
  onProjectReorder: (sourceProjectId: string, targetProjectId: string, placement: 'before' | 'after') => void
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
  onProjectReorder,
  onSettingsOpen,
}: AppSidebarProps) {
  const [draggedProjectId, setDraggedProjectId] = useState('')
  const [dragOverProject, setDragOverProject] = useState<{ id: string; placement: 'before' | 'after' } | null>(null)

  // projectDragPlacement 使用 event 参数判断拖拽目标应插入到项目之前还是之后。
  const projectDragPlacement = (event: DragEvent<HTMLDivElement>) => {
    const rect = event.currentTarget.getBoundingClientRect()
    return event.clientY > rect.top + rect.height / 2 ? 'after' : 'before'
  }

  // handleProjectDragStart 使用 event 和 projectId 参数开始拖拽 Project。
  const handleProjectDragStart = (event: DragEvent<HTMLDivElement>, projectId: string) => {
    setDraggedProjectId(projectId)
    event.dataTransfer.effectAllowed = 'move'
    event.dataTransfer.setData('text/plain', projectId)
  }

  // handleProjectDragOver 使用 event 和 projectId 参数更新当前拖拽落点。
  const handleProjectDragOver = (event: DragEvent<HTMLDivElement>, projectId: string) => {
    event.preventDefault()
    event.dataTransfer.dropEffect = 'move'
    const placement = projectDragPlacement(event)
    setDragOverProject((current) => {
      if (current?.id === projectId && current.placement === placement) {
        return current
      }
      return { id: projectId, placement }
    })
  }

  // handleProjectDragLeave 使用 event 和 projectId 参数清理离开的拖拽落点。
  const handleProjectDragLeave = (event: DragEvent<HTMLDivElement>, projectId: string) => {
    const nextTarget = event.relatedTarget
    if (nextTarget instanceof Node && event.currentTarget.contains(nextTarget)) {
      return
    }
    setDragOverProject((current) => (current?.id === projectId ? null : current))
  }

  // handleProjectDrop 使用 event 和 targetProjectId 参数提交 Project 排序。
  const handleProjectDrop = (event: DragEvent<HTMLDivElement>, targetProjectId: string) => {
    event.preventDefault()
    const sourceProjectId = event.dataTransfer.getData('text/plain') || draggedProjectId
    const placement = dragOverProject?.id === targetProjectId ? dragOverProject.placement : projectDragPlacement(event)
    setDraggedProjectId('')
    setDragOverProject(null)
    if (sourceProjectId && sourceProjectId !== targetProjectId) {
      onProjectReorder(sourceProjectId, targetProjectId, placement)
    }
  }

  // clearProjectDragState 清理当前 Project 拖拽状态。
  const clearProjectDragState = () => {
    setDraggedProjectId('')
    setDragOverProject(null)
  }

  return (
    <aside
      data-testid="sidebar"
      className="relative flex h-full min-h-0 flex-col border-r border-[var(--agenthub-outline)] bg-[var(--agenthub-sidebar)] text-[var(--agenthub-foreground)]"
    >
      <div className="border-b border-[var(--agenthub-outline)] px-3 py-3">
        <div
          data-testid="sidebar-identity"
          className="flex min-w-0 items-center gap-2 rounded-md border border-[var(--agenthub-outline)] bg-[var(--agenthub-surface-0)] px-2.5 py-2 text-xs text-[var(--agenthub-muted)] shadow-[var(--agenthub-elevation-1)]"
        >
          <Monitor className="h-3.5 w-3.5 shrink-0 text-[var(--agenthub-primary)]" />
          <span data-testid="machine-name" className="min-w-0 flex-1 truncate font-mono text-[var(--agenthub-foreground)]">
            {hostname || 'unknown'}
          </span>
          <span data-testid="connection-state" className="inline-flex shrink-0 items-center gap-1.5 rounded-full bg-[var(--agenthub-surface-2)] px-2 py-1">
            {connectionIcon}
            <span>{connectionText[connectionState]}</span>
          </span>
        </div>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto px-2 py-3" data-testid="project-list">
        {projects.length === 0 ? (
          <div className="rounded-md border border-dashed border-[var(--agenthub-outline-strong)] px-3 py-8 text-center text-sm text-[var(--agenthub-muted)]">
            还没有 Project
          </div>
        ) : (
          projects.map((project) => {
            const projectStatus = projectVisualStatuses.get(project.id)
            const dragPlacement = dragOverProject?.id === project.id ? dragOverProject.placement : null
            return (
              <div
                key={project.id}
                data-testid="project-item"
                draggable
                onDragStart={(event) => handleProjectDragStart(event, project.id)}
                onDragOver={(event) => handleProjectDragOver(event, project.id)}
                onDragLeave={(event) => handleProjectDragLeave(event, project.id)}
                onDrop={(event) => handleProjectDrop(event, project.id)}
                onDragEnd={clearProjectDragState}
                className={`mb-1 rounded-md border px-2 py-2 transition ${
                  project.id === selectedProjectId
                    ? 'border-[var(--agenthub-secondary)] bg-[var(--agenthub-primary-container)] shadow-[var(--agenthub-elevation-1)]'
                    : 'border-transparent hover:border-[var(--agenthub-outline)] hover:bg-[var(--agenthub-sidebar-hover)]'
                } ${draggedProjectId === project.id ? 'opacity-55' : ''} ${
                  dragPlacement === 'before'
                    ? 'shadow-[inset_0_2px_0_var(--agenthub-primary)]'
                    : dragPlacement === 'after'
                      ? 'shadow-[inset_0_-2px_0_var(--agenthub-primary)]'
                      : ''
                }`}
              >
                <div className="flex items-center gap-2">
                  <GripVertical className="h-4 w-4 shrink-0 cursor-grab text-[var(--agenthub-muted)] active:cursor-grabbing" />
                  <button
                    type="button"
                    onClick={() => onProjectSelect(project)}
                    className="flex min-w-0 flex-1 cursor-pointer items-center gap-2 text-left"
                  >
                    <Folder className="h-4 w-4 shrink-0 text-[var(--agenthub-primary)]" />
                    <span data-testid="project-name" className="min-w-0 flex-1 truncate text-sm font-medium text-[var(--agenthub-foreground)]">
                      {projectDisplayName(project)}
                    </span>
                    {projectStatus ? <StatusDot status={projectStatus} testID="project-status-dot" /> : null}
                  </button>
                  <button
                    data-testid="project-delete-button"
                    type="button"
                    onClick={() => onProjectDelete(project)}
                    className="inline-flex h-7 w-7 shrink-0 cursor-pointer items-center justify-center rounded-md text-[var(--agenthub-muted)] transition hover:bg-[var(--agenthub-danger-muted)] hover:text-[var(--agenthub-danger)]"
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
          <form
            className="rounded-lg border border-[var(--agenthub-outline)] bg-[var(--agenthub-surface-0)] p-3 text-[var(--agenthub-foreground)] shadow-[var(--agenthub-elevation-2)]"
            onSubmit={onProjectSave}
          >
            <div className="mb-2 flex items-center justify-between gap-2">
              <label htmlFor="project-path-input" className="text-xs font-medium text-[var(--agenthub-muted)]">
                工作目录
              </label>
              <button
                type="button"
                onClick={onProjectDialogClose}
                className="inline-flex h-7 w-7 cursor-pointer items-center justify-center rounded-md text-[var(--agenthub-muted)] transition hover:bg-[var(--agenthub-surface-2)] hover:text-[var(--agenthub-foreground)]"
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
              className="h-9 w-full rounded-md border border-[var(--agenthub-outline)] bg-[var(--agenthub-surface-0)] px-3 font-mono text-sm text-[var(--agenthub-foreground)] outline-none transition placeholder:text-[var(--agenthub-muted)] focus:border-[var(--agenthub-primary)] focus:ring-2 focus:ring-[var(--agenthub-primary)]/10"
              placeholder="/workspace/project/agenthub"
            />
            <button
              data-testid="project-save-button"
              type="submit"
              disabled={connectionState !== 'open' || !projectPath.trim()}
              className="mt-3 inline-flex h-9 w-full cursor-pointer items-center justify-center gap-2 rounded-md bg-[var(--agenthub-primary)] px-3 text-sm font-medium text-white transition hover:bg-[var(--agenthub-primary-hover)] focus:outline-none focus:ring-2 focus:ring-[var(--agenthub-primary)]/20 disabled:cursor-not-allowed disabled:bg-[var(--agenthub-surface-3)] disabled:text-[var(--agenthub-muted)]"
            >
              <Plus className="h-4 w-4" />
              添加
            </button>
            {errorText ? <p className="mt-2 text-xs text-[var(--agenthub-danger)]">{errorText}</p> : null}
          </form>
        </div>
      ) : null}

      <div data-testid="sidebar-footer" className="border-t border-[var(--agenthub-outline)] p-3">
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
