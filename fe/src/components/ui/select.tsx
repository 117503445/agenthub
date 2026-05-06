import * as React from 'react'
import { cn } from '../../lib/utils'

// Select 使用 props 参数渲染原生 select 的 shadcn 风格外观。
const Select = React.forwardRef<HTMLSelectElement, React.SelectHTMLAttributes<HTMLSelectElement>>(
  ({ className, ...props }, ref) => (
    <select
      ref={ref}
      className={cn(
        'h-8 rounded-md border border-[var(--agenthub-outline-strong)] bg-[var(--agenthub-surface-0)] px-2 text-xs text-[var(--agenthub-foreground)] outline-none transition focus:border-[var(--agenthub-primary)] focus:ring-2 focus:ring-[var(--agenthub-primary)]/10 disabled:cursor-not-allowed disabled:bg-[var(--agenthub-surface-2)] disabled:text-[var(--agenthub-muted)]',
        className,
      )}
      {...props}
    />
  ),
)
Select.displayName = 'Select'

export { Select }
