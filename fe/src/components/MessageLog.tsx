import { useCallback, useLayoutEffect, useMemo, useRef } from 'react'
import { Check, Copy, Loader2, Wrench } from 'lucide-react'
import { MarkdownRenderer } from './MarkdownRenderer'
import { PlanCard } from './PlanCard'
import { formatTime, messagePartsForRender, toolCommandTitle } from '../lib/chat'
import type { Chat, ChatMessage, ChatScrollMemory, PlanApproval } from '../types'

interface MessageLogProps {
  /** chat 表示当前聊天页。 */
  chat: Chat
  /** projectRoot 表示当前 project 根目录。 */
  projectRoot?: string
  /** copiedMessageId 表示刚复制成功的消息标识。 */
  copiedMessageId: string
  /** onCopyMessage 使用 message 参数复制消息。 */
  onCopyMessage: (message: ChatMessage) => void
  /** onExecutePlan 使用 plan 参数执行已确认 plan。 */
  onExecutePlan: (plan: PlanApproval) => void
  /** onReadScrollMemory 使用 chatId 参数读取聊天页滚动位置。 */
  onReadScrollMemory: (chatId: string) => ChatScrollMemory | undefined
  /** onSaveScrollMemory 使用 chatId 和 memory 参数保存聊天页滚动位置。 */
  onSaveScrollMemory: (chatId: string, memory: ChatScrollMemory) => void
}

// buildChatScrollSignature 使用 chat 参数生成用于判断内容是否变化的签名。
function buildChatScrollSignature(chat: Chat) {
  return chat.messages
    .map((message) => {
      const toolState =
        message.toolCalls?.map((tool) => `${tool.id}:${tool.updatedAt}:${tool.status}:${tool.input?.length ?? 0}:${tool.output?.length ?? 0}`).join(',') ?? ''
      const partState =
        message.parts
          ?.map((part) => `${part.id}:${part.updatedAt}:${part.text?.length ?? 0}:${part.toolCall?.status ?? ''}:${part.toolCall?.output?.length ?? 0}`)
          .join(',') ?? ''
      return [
        message.id,
        message.role,
        message.status,
        message.updatedAt,
        message.text.length,
        message.images?.length ?? 0,
        message.toolCalls?.length ?? 0,
        toolState,
        partState,
      ].join(':')
    })
    .join('|')
}

// MessageLog 使用 props 参数渲染聊天消息列表。
export function MessageLog({
  chat,
  projectRoot,
  copiedMessageId,
  onCopyMessage,
  onExecutePlan,
  onReadScrollMemory,
  onSaveScrollMemory,
}: MessageLogProps) {
  const logRef = useRef<HTMLDivElement | null>(null)
  const scrollSignature = useMemo(() => buildChatScrollSignature(chat), [chat])
  const scrollSignatureRef = useRef(scrollSignature)

  useLayoutEffect(() => {
    scrollSignatureRef.current = scrollSignature
  }, [scrollSignature])

  // saveCurrentScroll 使用 element 参数保存当前聊天页滚动位置。
  const saveCurrentScroll = useCallback(
    (element: HTMLDivElement | null) => {
      if (!element) {
        return
      }
      onSaveScrollMemory(chat.id, {
        scrollTop: element.scrollTop,
        signature: scrollSignatureRef.current,
      })
    },
    [chat.id, onSaveScrollMemory],
  )

  useLayoutEffect(() => {
    const element = logRef.current
    if (!element) {
      return
    }
    const memory = onReadScrollMemory(chat.id)
    const restoreSavedPosition = memory?.signature === scrollSignatureRef.current
    const frame = window.requestAnimationFrame(() => {
      const nextElement = logRef.current
      if (!nextElement) {
        return
      }
      nextElement.scrollTop = restoreSavedPosition && memory ? memory.scrollTop : nextElement.scrollHeight
      saveCurrentScroll(nextElement)
    })
    return () => {
      window.cancelAnimationFrame(frame)
      saveCurrentScroll(logRef.current)
    }
  }, [chat.id, onReadScrollMemory, saveCurrentScroll])

  return (
    <div
      ref={logRef}
      className="min-h-0 flex-1 overflow-y-auto px-4 py-5"
      data-testid="message-log"
      aria-live="polite"
      onScroll={(event) => saveCurrentScroll(event.currentTarget)}
    >
      {chat.messages.length > 0 ? (
        <div className="mx-auto flex max-w-4xl flex-col gap-3">
          {chat.messages.map((message) => {
            const timePosition = message.role === 'user' ? '-top-5 right-0' : '-top-5 left-0'
            return (
              <article
                key={message.id}
                className={`message-card message-${message.role} group/message relative mt-5 rounded-md border p-4 ${
                  message.role === 'user'
                    ? 'mb-8 border-teal-200 bg-teal-50'
                    : message.role === 'system'
                      ? 'border-rose-200 bg-rose-50'
                      : 'border-transparent bg-transparent'
                }`}
              >
                <span data-testid="message-time" className={`absolute ${timePosition} inline-flex items-center gap-2 text-xs text-slate-500`}>
                  {message.status === 'streaming' ? <Loader2 className="h-3.5 w-3.5 animate-spin text-orange-500" /> : null}
                  {message.status === 'stopped' ? '已停止' : message.status === 'error' ? '失败' : formatTime(message.updatedAt)}
                </span>
                {message.role === 'assistant' ? (
                  <div className="space-y-2">
                    {chat.plan?.messageId === message.id ? (
                      <PlanCard plan={chat.plan} projectRoot={projectRoot} onExecute={onExecutePlan} />
                    ) : (
                      messagePartsForRender(message).map((part) =>
                        part.type === 'tool_call' && part.toolCall ? (
                          <details
                            key={part.id}
                            data-testid="tool-call-details"
                            className="rounded-md border border-slate-200 bg-slate-50 px-3 py-2"
                          >
                            <summary className="flex cursor-pointer list-none items-center justify-between gap-3">
                              <span className="inline-flex min-w-0 items-center gap-2 text-sm font-medium text-slate-800">
                                <Wrench className="h-4 w-4 shrink-0 text-slate-500" />
                                <span className="truncate font-mono text-xs">{toolCommandTitle(part.toolCall)}</span>
                              </span>
                              <span className="shrink-0 text-xs text-slate-500">
                                {part.toolCall.status === 'running' ? '运行中' : part.toolCall.status === 'error' ? '失败' : '完成'}
                              </span>
                            </summary>
                            {part.toolCall.input ? <pre className="mt-2 truncate font-mono text-xs text-slate-500">{part.toolCall.input}</pre> : null}
                            {part.toolCall.output ? (
                              <pre data-testid="tool-call-output" className="mt-2 whitespace-pre-wrap break-words font-mono text-xs text-slate-600">
                                {part.toolCall.output}
                              </pre>
                            ) : null}
                          </details>
                        ) : (
                          <MarkdownRenderer key={part.id} text={part.text ?? ''} projectRoot={projectRoot} />
                        ),
                      )
                    )}
                  </div>
                ) : (
                  <div className="space-y-3">
                    <pre className="whitespace-pre-wrap break-words font-sans text-base leading-7 text-slate-800">{message.text}</pre>
                    {message.images?.length ? (
                      <div className="grid grid-cols-2 gap-2 sm:grid-cols-3">
                        {message.images.map((image) => (
                          <figure key={image.id} data-testid="message-image" className="overflow-hidden rounded-md border border-slate-200 bg-white">
                            <img src={`data:${image.mimeType};base64,${image.data}`} alt={image.fileName} className="h-28 w-full object-cover" />
                            <figcaption className="truncate px-2 py-1 text-xs text-slate-500">{image.fileName}</figcaption>
                          </figure>
                        ))}
                      </div>
                    ) : null}
                  </div>
                )}
                {message.role === 'user' ? (
                  <button
                    data-testid="user-copy-button"
                    type="button"
                    onClick={() => onCopyMessage(message)}
                    className="absolute -bottom-8 right-0 inline-flex h-7 w-7 cursor-pointer items-center justify-center rounded-md text-slate-500 opacity-0 transition hover:bg-slate-100 hover:text-slate-900 focus:opacity-100 focus:outline-none focus:ring-2 focus:ring-teal-500/20 group-hover/message:opacity-100"
                    aria-label="复制消息"
                    title="复制"
                  >
                    {copiedMessageId === message.id ? <Check className="h-3.5 w-3.5" /> : <Copy className="h-3.5 w-3.5" />}
                  </button>
                ) : null}
                {message.role === 'assistant' ? (
                  <div className="mt-2 flex justify-start">
                    <button
                      data-testid="assistant-copy-button"
                      type="button"
                      onClick={() => onCopyMessage(message)}
                      className="inline-flex h-7 w-7 cursor-pointer items-center justify-center rounded-md text-slate-500 transition hover:bg-slate-100 hover:text-slate-900 focus:outline-none focus:ring-2 focus:ring-teal-500/20"
                      aria-label="复制回复"
                      title="复制"
                    >
                      {copiedMessageId === message.id ? <Check className="h-3.5 w-3.5" /> : <Copy className="h-3.5 w-3.5" />}
                    </button>
                  </div>
                ) : null}
              </article>
            )
          })}
        </div>
      ) : null}
    </div>
  )
}
