import * as React from 'react'
import { cn } from '../../lib/utils'

type ButtonVariant = 'default' | 'outline' | 'ghost' | 'destructive'
type ButtonSize = 'default' | 'sm' | 'icon'

interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  /** variant 表示按钮视觉类型。 */
  variant?: ButtonVariant
  /** size 表示按钮尺寸。 */
  size?: ButtonSize
}

const variantClass: Record<ButtonVariant, string> = {
  default: 'bg-[var(--paseo-accent)] text-[#ffffff] hover:bg-[var(--paseo-accent-hover)]',
  outline:
    'border border-[var(--paseo-border)] bg-[var(--paseo-surface-0)] text-[var(--paseo-foreground)] hover:bg-[var(--paseo-sidebar-hover)]',
  ghost: 'text-[var(--paseo-muted)] hover:bg-[var(--paseo-sidebar-hover)] hover:text-[var(--paseo-foreground)]',
  destructive: 'bg-[var(--paseo-danger)] text-[#ffffff] hover:bg-[#c64f43]',
}

const sizeClass: Record<ButtonSize, string> = {
  default: 'h-9 px-3 text-sm',
  sm: 'h-8 px-2.5 text-xs',
  icon: 'h-9 w-9',
}

// Button 使用 props 参数渲染 shadcn 风格按钮。
const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant = 'default', size = 'default', type = 'button', ...props }, ref) => (
    <button
      ref={ref}
      type={type}
      className={cn(
        'inline-flex cursor-pointer items-center justify-center gap-2 rounded-md font-medium transition focus:outline-none focus:ring-2 focus:ring-[var(--paseo-accent)]/25 disabled:cursor-not-allowed disabled:opacity-50',
        variantClass[variant],
        sizeClass[size],
        className,
      )}
      {...props}
    />
  ),
)
Button.displayName = 'Button'

export { Button }
export type { ButtonProps }
