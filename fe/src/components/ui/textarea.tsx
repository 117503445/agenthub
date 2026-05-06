import * as React from 'react'
import { cn } from '../../lib/utils'

// Textarea 使用 props 参数渲染 shadcn 风格多行输入框。
const Textarea = React.forwardRef<HTMLTextAreaElement, React.TextareaHTMLAttributes<HTMLTextAreaElement>>(({ className, ...props }, ref) => (
  <textarea
    ref={ref}
    className={cn(
      'w-full resize-none rounded-md border border-[var(--agenthub-outline-strong)] bg-[var(--agenthub-surface-0)] px-3 py-3 text-sm leading-5 text-[var(--agenthub-foreground)] outline-none transition placeholder:text-[var(--agenthub-muted)] focus:border-[var(--agenthub-primary)] focus:ring-2 focus:ring-[var(--agenthub-primary)]/10 disabled:cursor-not-allowed disabled:bg-[var(--agenthub-surface-2)] disabled:text-[var(--agenthub-muted)]',
      className,
    )}
    {...props}
  />
))
Textarea.displayName = 'Textarea'

export { Textarea }
