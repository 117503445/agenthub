import * as React from 'react'
import { cn } from '../../lib/utils'

// Input 使用 props 参数渲染 shadcn 风格输入框。
const Input = React.forwardRef<HTMLInputElement, React.InputHTMLAttributes<HTMLInputElement>>(({ className, ...props }, ref) => (
  <input
    ref={ref}
    className={cn(
      'h-9 w-full rounded-md border border-[var(--paseo-border-accent)] bg-[var(--paseo-surface-0)] px-3 text-sm text-[var(--paseo-foreground)] outline-none transition placeholder:text-[var(--paseo-muted)] focus:border-[var(--paseo-accent)] focus:ring-2 focus:ring-[var(--paseo-accent)]/10 disabled:cursor-not-allowed disabled:bg-[var(--paseo-surface-2)] disabled:text-[var(--paseo-muted)]',
      className,
    )}
    {...props}
  />
))
Input.displayName = 'Input'

export { Input }
