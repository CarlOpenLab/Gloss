import { activeProvider, store } from './store'
import type { Provider, SessionInfo } from './types'

function headers(): Record<string, string> {
  const h: Record<string, string> = { 'Content-Type': 'application/json' }
  if (store.ghToken) h['X-GitHub-Token'] = store.ghToken
  return h
}

async function post<T>(url: string, body: unknown): Promise<T> {
  const resp = await fetch(url, { method: 'POST', headers: headers(), body: JSON.stringify(body) })
  const data = await resp.json().catch(() => ({}))
  if (!resp.ok) throw new Error((data as { error?: string }).error || `请求失败 (${resp.status})`)
  return data as T
}

export const createPRSession = (url: string) => post<SessionInfo>('/api/session/pr', { url })
export const createDiffSession = (raw: string) => post<SessionInfo>('/api/session/diff', { raw })

export interface TestProviderResult {
  ok: boolean
  ms?: number
  reply?: string
  error?: string
}
export const testProvider = (provider: Provider) =>
  post<TestProviderResult>('/api/test-provider', { provider })

export interface ExplainHandlers {
  onDelta: (text: string) => void
  onDone: (cached: boolean) => void
  onError: (message: string) => void
}

/** 调 /api/explain 并解析 SSE 流 */
export async function explainHunk(
  sessionId: string,
  fileIndex: number,
  hunkId: string,
  h: ExplainHandlers,
): Promise<void> {
  let resp: Response
  try {
    resp = await fetch('/api/explain', {
      method: 'POST',
      headers: headers(),
      body: JSON.stringify({ sessionId, fileIndex, hunkId, provider: activeProvider() }),
    })
  } catch (e) {
    h.onError(`网络错误: ${(e as Error).message}`)
    return
  }
  if (!resp.ok || !resp.body) {
    const data = await resp.json().catch(() => ({}))
    h.onError((data as { error?: string }).error || `请求失败 (${resp.status})`)
    return
  }

  const reader = resp.body.getReader()
  const decoder = new TextDecoder()
  let buf = ''
  for (;;) {
    const { done, value } = await reader.read()
    if (done) break
    buf += decoder.decode(value, { stream: true })
    let idx: number
    while ((idx = buf.indexOf('\n\n')) >= 0) {
      const chunk = buf.slice(0, idx)
      buf = buf.slice(idx + 2)
      for (const line of chunk.split('\n')) {
        if (!line.startsWith('data: ')) continue
        try {
          const evt = JSON.parse(line.slice(6)) as {
            type: string
            text?: string
            cached?: boolean
            message?: string
          }
          if (evt.type === 'delta' && evt.text) h.onDelta(evt.text)
          else if (evt.type === 'done') h.onDone(!!evt.cached)
          else if (evt.type === 'error') h.onError(evt.message || '未知错误')
        } catch {
          // 忽略不完整的事件行
        }
      }
    }
  }
}
