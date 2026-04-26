import { FormEvent, useEffect, useMemo, useRef, useState } from 'react'
import {
  Activity,
  ArrowRight,
  CircleAlert,
  CircleCheck,
  Clock3,
  PlugZap,
  RefreshCw,
  Send,
  Server,
} from 'lucide-react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { getWebSocketUrl, type ServerMessage } from '@/lib/ws'

type ConnectionState = 'connecting' | 'open' | 'closed' | 'error'

interface LogEntry {
  id: string
  source: 'client' | 'server'
  type: string
  text: string
  time: string
}

const stateText: Record<ConnectionState, string> = {
  connecting: '连接中',
  open: '已连接',
  closed: '已断开',
  error: '连接异常',
}

function createId() {
  return globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random()}`
}

function payloadText(message: ServerMessage) {
  const payload = message.payload ?? {}
  const echo = payload.echo
  const text = payload.message
  if (typeof echo === 'string' && echo.length > 0) {
    return `${String(text ?? message.type)}：${echo}`
  }
  return String(text ?? message.type)
}

function App() {
  const wsRef = useRef<WebSocket | null>(null)
  const [connectionState, setConnectionState] = useState<ConnectionState>('connecting')
  const [inputValue, setInputValue] = useState('来自 E2E 的消息')
  const [version, setVersion] = useState('dev')
  const [lastServerTime, setLastServerTime] = useState('')
  const [logs, setLogs] = useState<LogEntry[]>([])

  const stateView = useMemo(() => {
    if (connectionState === 'open') {
      return {
        icon: <CircleCheck className="h-4 w-4 text-emerald-600" />,
        className: 'border-emerald-200 bg-emerald-50 text-emerald-700',
      }
    }
    if (connectionState === 'error') {
      return {
        icon: <CircleAlert className="h-4 w-4 text-rose-600" />,
        className: 'border-rose-200 bg-rose-50 text-rose-700',
      }
    }
    return {
      icon: <RefreshCw className="h-4 w-4 animate-spin text-amber-600" />,
      className: 'border-amber-200 bg-amber-50 text-amber-700',
    }
  }, [connectionState])

  useEffect(() => {
    let stopped = false
    let retryTimer = 0
    let heartbeatTimer = 0

    const appendLog = (entry: Omit<LogEntry, 'id' | 'time'>) => {
      setLogs((current) => [
        {
          ...entry,
          id: createId(),
          time: new Date().toLocaleTimeString('zh-CN', { hour12: false }),
        },
        ...current,
      ].slice(0, 12))
    }

    const connect = () => {
      setConnectionState('connecting')
      const ws = new WebSocket(getWebSocketUrl())
      wsRef.current = ws

      ws.onopen = () => {
        setConnectionState('open')
        appendLog({ source: 'client', type: 'open', text: '浏览器连接已建立' })
        heartbeatTimer = window.setInterval(() => {
          if (ws.readyState === WebSocket.OPEN) {
            ws.send(JSON.stringify({ type: 'ping', payload: 'heartbeat' }))
          }
        }, 5000)
      }

      ws.onmessage = (event) => {
        const message = JSON.parse(event.data) as ServerMessage
        setVersion(message.version)
        setLastServerTime(message.serverTime)
        appendLog({
          source: 'server',
          type: message.type,
          text: payloadText(message),
        })
      }

      ws.onerror = () => {
        setConnectionState('error')
      }

      ws.onclose = () => {
        window.clearInterval(heartbeatTimer)
        if (stopped) {
          return
        }
        setConnectionState('closed')
        appendLog({ source: 'server', type: 'close', text: '连接关闭，准备重连' })
        retryTimer = window.setTimeout(connect, 1200)
      }
    }

    connect()

    return () => {
      stopped = true
      window.clearTimeout(retryTimer)
      window.clearInterval(heartbeatTimer)
      wsRef.current?.close()
    }
  }, [])

  const sendMessage = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const text = inputValue.trim()
    const ws = wsRef.current
    if (!text || !ws || ws.readyState !== WebSocket.OPEN) {
      return
    }
    ws.send(JSON.stringify({ type: 'echo', payload: text }))
    const entry: LogEntry = {
      id: createId(),
      source: 'client',
      type: 'echo',
      text,
      time: new Date().toLocaleTimeString('zh-CN', { hour12: false }),
    }
    setLogs((current) => [entry, ...current].slice(0, 12))
    setInputValue('')
  }

  return (
    <main className="min-h-screen bg-slate-50">
      <div className="mx-auto flex w-full max-w-6xl flex-col gap-5 px-4 py-5 sm:px-6 lg:px-8">
        <header className="flex flex-col gap-4 border-b border-slate-200 pb-5 md:flex-row md:items-center md:justify-between">
          <div className="flex items-center gap-3">
            <div className="flex h-11 w-11 items-center justify-center rounded-lg bg-slate-950 text-white">
              <PlugZap className="h-5 w-5" />
            </div>
            <div>
              <h1 className="text-xl font-semibold tracking-normal text-slate-950">coding WebSocket 控制台</h1>
              <p className="text-sm text-slate-500">Go 后端与 React 前端实时通信</p>
            </div>
          </div>
          <div
            data-testid="connection-state"
            className={`inline-flex w-fit items-center gap-2 rounded-lg border px-3 py-2 text-sm font-medium ${stateView.className}`}
          >
            {stateView.icon}
            {stateText[connectionState]}
          </div>
        </header>

        <section className="grid gap-5 lg:grid-cols-[320px_minmax(0,1fr)]">
          <div className="space-y-5">
            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <Server className="h-4 w-4 text-slate-500" />
                  服务状态
                </CardTitle>
              </CardHeader>
              <CardContent className="space-y-3">
                <div className="flex items-center justify-between rounded-lg border border-slate-200 bg-slate-50 px-3 py-2">
                  <span className="text-sm text-slate-500">版本</span>
                  <code className="max-w-36 truncate rounded bg-white px-2 py-1 font-mono text-xs text-slate-700">{version}</code>
                </div>
                <div className="flex items-center justify-between rounded-lg border border-slate-200 bg-slate-50 px-3 py-2">
                  <span className="flex items-center gap-2 text-sm text-slate-500">
                    <Clock3 className="h-4 w-4" />
                    服务时间
                  </span>
                  <span className="max-w-40 truncate text-sm text-slate-700">{lastServerTime || '等待消息'}</span>
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <Activity className="h-4 w-4 text-slate-500" />
                  消息发送
                </CardTitle>
              </CardHeader>
              <CardContent>
                <form className="space-y-3" onSubmit={sendMessage}>
                  <input
                    data-testid="message-input"
                    value={inputValue}
                    onChange={(event) => setInputValue(event.target.value)}
                    className="h-10 w-full rounded-lg border border-slate-300 bg-white px-3 text-sm text-slate-950 outline-none transition focus:border-slate-700 focus:ring-2 focus:ring-slate-200"
                    placeholder="输入要发送的内容"
                  />
                  <button
                    data-testid="send-button"
                    type="submit"
                    disabled={connectionState !== 'open' || inputValue.trim().length === 0}
                    className="inline-flex h-10 w-full items-center justify-center gap-2 rounded-lg bg-slate-950 px-4 text-sm font-medium text-white transition hover:bg-slate-800 disabled:cursor-not-allowed disabled:bg-slate-300"
                  >
                    <Send className="h-4 w-4" />
                    发送
                  </button>
                </form>
              </CardContent>
            </Card>
          </div>

          <Card className="min-h-[420px]">
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <ArrowRight className="h-4 w-4 text-slate-500" />
                通信记录
              </CardTitle>
            </CardHeader>
            <CardContent>
              <div data-testid="message-log" className="space-y-3">
                {logs.length === 0 ? (
                  <div className="rounded-lg border border-dashed border-slate-300 bg-slate-50 p-8 text-center text-sm text-slate-500">
                    等待 WebSocket 消息
                  </div>
                ) : (
                  logs.map((log) => (
                    <div
                      key={log.id}
                      className="grid gap-2 rounded-lg border border-slate-200 bg-white p-3 sm:grid-cols-[96px_minmax(0,1fr)_84px] sm:items-center"
                    >
                      <span className={log.source === 'server' ? 'text-sm font-medium text-blue-700' : 'text-sm font-medium text-emerald-700'}>
                        {log.source === 'server' ? '服务端' : '浏览器'}
                      </span>
                      <div className="min-w-0">
                        <span className="mr-2 rounded bg-slate-100 px-2 py-1 font-mono text-xs text-slate-600">{log.type}</span>
                        <span className="break-words text-sm text-slate-800">{log.text}</span>
                      </div>
                      <time className="font-mono text-xs text-slate-400 sm:text-right">{log.time}</time>
                    </div>
                  ))
                )}
              </div>
            </CardContent>
          </Card>
        </section>
      </div>
    </main>
  )
}

export default App
