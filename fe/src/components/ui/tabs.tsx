import * as React from 'react'
import { cn } from '../../lib/utils'

interface TabsContextValue {
  /** value 表示当前激活 tab。 */
  value: string
  /** onValueChange 使用 value 参数切换当前 tab。 */
  onValueChange?: (value: string) => void
}

interface TabsProps extends React.HTMLAttributes<HTMLDivElement> {
  /** value 表示当前激活 tab。 */
  value: string
  /** onValueChange 使用 value 参数切换当前 tab。 */
  onValueChange?: (value: string) => void
}

interface TabsTriggerProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  /** value 表示该触发器对应的 tab。 */
  value: string
}

interface TabsContentProps extends React.HTMLAttributes<HTMLDivElement> {
  /** value 表示该内容区对应的 tab。 */
  value: string
}

const TabsContext = React.createContext<TabsContextValue | null>(null)

// useTabsContext 读取 TabsContext，缺失时抛出可定位错误。
function useTabsContext() {
  const context = React.useContext(TabsContext)
  if (!context) {
    throw new Error('Tabs 组件必须放在 Tabs 根节点内')
  }
  return context
}

// Tabs 使用 props 参数渲染 tab 根容器。
function Tabs({ value, onValueChange, className, ...props }: TabsProps) {
  const contextValue = React.useMemo(() => ({ value, onValueChange }), [onValueChange, value])
  return (
    <TabsContext.Provider value={contextValue}>
      <div className={cn('flex min-h-0 flex-col', className)} {...props} />
    </TabsContext.Provider>
  )
}

// TabsList 使用 props 参数渲染 tab 触发器列表。
const TabsList = React.forwardRef<HTMLDivElement, React.HTMLAttributes<HTMLDivElement>>(({ className, ...props }, ref) => (
  <div
    ref={ref}
    role="tablist"
    className={cn('flex min-h-0 items-stretch gap-1 overflow-x-auto', className)}
    {...props}
  />
))
TabsList.displayName = 'TabsList'

// TabsTrigger 使用 props 参数渲染单个 tab 触发器。
const TabsTrigger = React.forwardRef<HTMLButtonElement, TabsTriggerProps>(({ value, className, onClick, ...props }, ref) => {
  const context = useTabsContext()
  const active = context.value === value
  return (
    <button
      ref={ref}
      type="button"
      role="tab"
      aria-selected={active}
      data-state={active ? 'active' : 'inactive'}
      onClick={(event) => {
        context.onValueChange?.(value)
        onClick?.(event)
      }}
      className={cn(
        'inline-flex h-full min-h-10 shrink-0 cursor-pointer items-center gap-2 border-b-2 border-transparent px-3 text-sm text-[var(--paseo-muted)] transition hover:text-[var(--paseo-foreground)] data-[state=active]:border-[var(--paseo-accent)] data-[state=active]:text-[var(--paseo-foreground)]',
        className,
      )}
      {...props}
    />
  )
})
TabsTrigger.displayName = 'TabsTrigger'

// TabsContent 使用 props 参数渲染单个 tab 内容区。
const TabsContent = React.forwardRef<HTMLDivElement, TabsContentProps>(({ value, className, ...props }, ref) => {
  const context = useTabsContext()
  const active = context.value === value
  return (
    <div
      ref={ref}
      role="tabpanel"
      hidden={!active}
      data-state={active ? 'active' : 'inactive'}
      className={cn(active ? 'flex' : 'hidden', 'min-h-0 flex-1 flex-col', className)}
      {...props}
    />
  )
})
TabsContent.displayName = 'TabsContent'

export { Tabs, TabsList, TabsTrigger, TabsContent }
