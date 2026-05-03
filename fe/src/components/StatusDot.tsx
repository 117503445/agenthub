import type { ChatVisualStatus } from '../types'

const chatStatusLabel: Record<ChatVisualStatus, string> = {
  running: '工作中',
  success: '已完成',
  error: '失败',
}

const chatStatusClass: Record<ChatVisualStatus, string> = {
  running: 'bg-amber-400',
  success: 'bg-emerald-500',
  error: 'bg-red-500',
}

interface StatusDotProps {
  /** status 表示需要展示的状态。 */
  status: ChatVisualStatus
  /** testID 表示测试定位标识。 */
  testID: string
}

// StatusDot 使用 status 和 testID 参数渲染紧凑状态圆点。
export function StatusDot({ status, testID }: StatusDotProps) {
  return (
    <span
      data-testid={testID}
      data-status={status}
      className={`inline-block h-2.5 w-2.5 shrink-0 rounded-full ${chatStatusClass[status]}`}
      title={chatStatusLabel[status]}
      aria-label={chatStatusLabel[status]}
    />
  )
}
