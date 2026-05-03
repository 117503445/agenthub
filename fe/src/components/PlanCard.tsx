import { CheckCircle2, ClipboardList, Play } from 'lucide-react'
import { MarkdownRenderer } from './MarkdownRenderer'
import { Button } from './ui/button'
import type { PlanApproval } from '../types'

interface PlanCardProps {
  /** plan 表示待确认或执行中的 plan。 */
  plan: PlanApproval
  /** onExecute 使用 plan 参数开始执行已确认 plan。 */
  onExecute: (plan: PlanApproval) => void
}

// planStatusText 使用 status 参数返回 plan 状态文案。
function planStatusText(status: PlanApproval['status']) {
  if (status === 'executing') {
    return '执行中'
  }
  if (status === 'done') {
    return '已完成'
  }
  return '待确认'
}

// PlanCard 使用 plan 参数渲染 plan 模式确认卡片。
export function PlanCard({ plan, onExecute }: PlanCardProps) {
  const executing = plan.status !== 'pending'

  return (
    <section data-testid="plan-card" className="rounded-md border border-slate-200 bg-white p-3">
      <div className="mb-3 flex items-center justify-between gap-3">
        <span className="inline-flex min-w-0 items-center gap-2 text-sm font-medium text-slate-900">
          <ClipboardList className="h-4 w-4 shrink-0 text-slate-500" />
          <span>Plan</span>
          <span className="rounded bg-slate-100 px-1.5 py-0.5 text-xs text-slate-500">{planStatusText(plan.status)}</span>
        </span>
        <Button
          data-testid="plan-execute-button"
          type="button"
          size="sm"
          disabled={executing}
          onClick={() => onExecute(plan)}
          className="h-8 gap-1.5"
        >
          {executing ? <CheckCircle2 className="h-3.5 w-3.5" /> : <Play className="h-3.5 w-3.5" />}
          开始执行
        </Button>
      </div>
      <MarkdownRenderer text={plan.text} />
    </section>
  )
}
