import { Gauge } from 'lucide-react'
import type { ContextWindowUsage } from '../types'

interface ContextWindowMeterProps {
  /** usage 表示上下文窗口使用情况。 */
  usage?: ContextWindowUsage
}

// compactTokenCount 使用 value 参数格式化 token 数。
function compactTokenCount(value: number) {
  if (value >= 1_000_000) {
    return `${Math.round(value / 100_000) / 10}M`
  }
  if (value >= 1_000) {
    return `${Math.round(value / 1_000)}k`
  }
  return `${value}`
}

// ContextWindowMeter 使用 usage 参数渲染 context window 使用图标。
export function ContextWindowMeter({ usage }: ContextWindowMeterProps) {
  const maxTokens = Math.max(usage?.maxTokens ?? 0, 0)
  const usedTokens = Math.max(usage?.usedTokens ?? 0, 0)
  const percent = maxTokens > 0 ? Math.min(100, Math.round((usedTokens / maxTokens) * 100)) : 0
  const tone = percent >= 90 ? '#b04138' : percent >= 70 ? '#d97706' : '#20744a'

  return (
    <div
      data-testid="context-window-meter"
      data-percent={percent}
      className="context-window-meter group relative inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-full text-slate-500"
      style={{ background: `conic-gradient(${tone} ${percent * 3.6}deg, #e4e4e7 0deg)` }}
      aria-label={`Context window ${percent}% used`}
      title={`Context window ${percent}% used`}
      tabIndex={0}
    >
      <span className="absolute inset-[3px] rounded-full bg-white" />
      <Gauge className="relative h-4 w-4" />
      <span className="sr-only">Context window</span>
      <span className="pointer-events-none absolute bottom-[calc(100%+10px)] right-0 z-40 hidden w-40 rounded-md border border-slate-200 bg-white p-3 text-left text-xs leading-5 text-slate-600 shadow-lg group-hover:block group-focus:block">
        <span className="block font-medium text-slate-900">Context window</span>
        <span className="block">{percent}% used</span>
        <span className="block font-mono">
          {compactTokenCount(usedTokens)} / {compactTokenCount(maxTokens)} tokens
        </span>
      </span>
    </div>
  )
}
