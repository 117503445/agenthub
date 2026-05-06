import * as React from 'react'
import { cn } from '../../lib/utils'

// Input 使用 props 参数渲染 shadcn 风格输入框。
const Input = React.forwardRef<HTMLInputElement, React.InputHTMLAttributes<HTMLInputElement>>(({ className, ...props }, ref) => (
  <input
    ref={ref}
    className={cn(
      'h-9 w-full rounded-md border border-[var(--agenthub-outline-strong)] bg-[var(--agenthub-surface-0)] px-3 text-sm text-[var(--agenthub-foreground)] outline-none transition placeholder:text-[var(--agenthub-muted)] focus:border-[var(--agenthub-primary)] focus:ring-2 focus:ring-[var(--agenthub-primary)]/10 disabled:cursor-not-allowed disabled:bg-[var(--agenthub-surface-2)] disabled:text-[var(--agenthub-muted)]',
      className,
    )}
    {...props}
  />
))
Input.displayName = 'Input'

export { Input }
