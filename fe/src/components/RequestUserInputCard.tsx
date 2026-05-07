import { useMemo, useState } from 'react'
import { CheckCircle2, CircleHelp, Send } from 'lucide-react'
import { Button } from './ui/button'
import type { ToolCall, UserInputQuestion } from '../types'

interface RequestUserInputCardProps {
  /** toolCall 表示 request_user_input 工具调用。 */
  toolCall: ToolCall
  /** onRespond 使用 toolCall 和 answers 参数提交用户答案。 */
  onRespond: (toolCall: ToolCall, answers: Record<string, string[]>) => void
}

// firstAnswerText 使用 question 和 answers 参数返回展示用答案。
function firstAnswerText(question: UserInputQuestion, answers: Record<string, string[]> | undefined) {
  const values = answers?.[question.id] ?? []
  if (question.isSecret && values.length > 0) {
    return '已提交'
  }
  return values.join(', ')
}

// RequestUserInputCard 使用 toolCall 和 onRespond 参数渲染 agent 用户输入请求。
export function RequestUserInputCard({ toolCall, onRespond }: RequestUserInputCardProps) {
  const request = toolCall.userInputRequest
  const submitted = toolCall.status !== 'running' || Boolean(request?.answers)
  const [answers, setAnswers] = useState<Record<string, string[]>>({})
  const questions = request?.questions ?? []

  const canSubmit = useMemo(
    () => questions.length > 0 && questions.every((question) => (answers[question.id] ?? []).some((value) => value.trim())),
    [answers, questions],
  )

  // chooseOption 使用 question 和 label 参数选择单个选项。
  const chooseOption = (question: UserInputQuestion, label: string) => {
    setAnswers((current) => ({ ...current, [question.id]: [label] }))
  }

  // updateFreeform 使用 question 和 value 参数更新自由输入答案。
  const updateFreeform = (question: UserInputQuestion, value: string) => {
    setAnswers((current) => ({ ...current, [question.id]: value.trim() ? [value] : [] }))
  }

  return (
    <section
      data-testid="request-user-input-card"
      className="rounded-md border border-[var(--agenthub-outline)] bg-[var(--agenthub-surface-1)] px-3 py-3 shadow-[var(--agenthub-elevation-1)]"
    >
      <div className="mb-3 flex items-center justify-between gap-3">
        <span className="inline-flex min-w-0 items-center gap-2 text-sm font-medium text-[var(--agenthub-foreground)]">
          <CircleHelp className="h-4 w-4 shrink-0 text-[var(--agenthub-primary)]" />
          <span>需要确认</span>
          <span className="rounded bg-[var(--agenthub-surface-2)] px-1.5 py-0.5 text-xs text-[var(--agenthub-muted)]">
            {submitted ? '已提交' : '待回答'}
          </span>
        </span>
      </div>
      <div className="space-y-3">
        {questions.map((question) => {
          const submittedText = firstAnswerText(question, request?.answers)
          return (
            <div key={question.id} className="space-y-2">
              <div className="space-y-1">
                <div className="text-xs font-medium uppercase text-[var(--agenthub-muted)]">{question.header}</div>
                <div className="text-sm leading-6 text-[var(--agenthub-foreground)]">{question.question}</div>
              </div>
              {submitted ? (
                <div className="inline-flex items-center gap-2 rounded bg-[var(--agenthub-surface-2)] px-2 py-1 text-xs text-[var(--agenthub-muted)]">
                  <CheckCircle2 className="h-3.5 w-3.5 text-[var(--agenthub-success)]" />
                  {submittedText || '已提交'}
                </div>
              ) : question.options?.length ? (
                <div className="flex flex-wrap gap-2">
                  {question.options.map((option) => {
                    const selected = answers[question.id]?.includes(option.label)
                    return (
                      <button
                        key={option.label}
                        type="button"
                        data-testid="request-user-input-option"
                        data-question-id={question.id}
                        data-option-label={option.label}
                        data-selected={selected ? 'true' : 'false'}
                        onClick={() => chooseOption(question, option.label)}
                        className={`min-h-9 rounded-md border px-3 py-1.5 text-left text-sm transition ${
                          selected
                            ? 'border-[var(--agenthub-primary)] bg-[var(--agenthub-primary-container)] text-[var(--agenthub-foreground)]'
                            : 'border-[var(--agenthub-outline)] bg-[var(--agenthub-surface-0)] text-[var(--agenthub-foreground)] hover:border-[var(--agenthub-outline-strong)]'
                        }`}
                      >
                        <span className="block">{option.label}</span>
                        {option.description ? <span className="block text-xs text-[var(--agenthub-muted)]">{option.description}</span> : null}
                      </button>
                    )
                  })}
                </div>
              ) : null}
              {!submitted && (question.isOther || !question.options?.length) ? (
                <textarea
                  data-testid="request-user-input-freeform"
                  value={answers[question.id]?.[0] ?? ''}
                  onChange={(event) => updateFreeform(question, event.target.value)}
                  className="min-h-20 w-full resize-y rounded-md border border-[var(--agenthub-outline)] bg-[var(--agenthub-surface-0)] px-3 py-2 text-sm leading-6 text-[var(--agenthub-foreground)] outline-none focus:border-[var(--agenthub-primary)]"
                />
              ) : null}
            </div>
          )
        })}
      </div>
      {!submitted ? (
        <div className="mt-3 flex justify-end">
          <Button
            data-testid="request-user-input-submit"
            type="button"
            size="sm"
            disabled={!canSubmit}
            onClick={() => onRespond(toolCall, answers)}
            className="h-8 gap-1.5"
          >
            <Send className="h-3.5 w-3.5" />
            提交
          </Button>
        </div>
      ) : null}
    </section>
  )
}
