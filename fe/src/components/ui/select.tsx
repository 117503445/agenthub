import * as React from 'react'
import { cn } from '../../lib/utils'

// Select 使用 props 参数渲染原生 select 的 shadcn 风格外观。
const Select = React.forwardRef<HTMLSelectElement, React.SelectHTMLAttributes<HTMLSelectElement>>(
  ({ className, ...props }, ref) => (
    <select
      ref={ref}
      className={cn(
        'h-8 rounded-md border border-[var(--paseo-border-accent)] bg-[var(--paseo-surface-0)] px-2 text-xs text-[var(--paseo-foreground)] outline-none transition focus:border-[var(--paseo-accent)] focus:ring-2 focus:ring-[var(--paseo-accent)]/10 disabled:cursor-not-allowed disabled:bg-[var(--paseo-surface-2)] disabled:text-[var(--paseo-muted)]',
        className,
      )}
      {...props}
    />
  ),
)
Select.displayName = 'Select'

export { Select }
