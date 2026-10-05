import { reactive, watch } from 'vue'
import type { FileDiff, SessionInfo } from './types'

const STORAGE_KEY = 'de_reviewed'

function load(): Record<string, 1> {
  try {
    const v = JSON.parse(localStorage.getItem(STORAGE_KEY) || '{}')
    if (v && typeof v === 'object' && !Array.isArray(v)) return v as Record<string, 1>
  } catch {
    // ignore
  }
  return {}
}

/**
 * 已 review 的 hunk 集合。key 用后端的 hunk 内容哈希（含文件路径），
 * 所以同一段代码重开页面、甚至换一个 session 打开，勾选状态都还在。
 */
export const reviewed = reactive<Record<string, 1>>(load())

watch(reviewed, v => localStorage.setItem(STORAGE_KEY, JSON.stringify(v)), { deep: true })

export function isReviewed(hunkId: string): boolean {
  return !!reviewed[hunkId]
}

export function toggleReviewed(hunkId: string) {
  if (reviewed[hunkId]) delete reviewed[hunkId]
  else reviewed[hunkId] = 1
}

/** 一次设置一组 hunk 的 review 状态（文件级「已 review」用） */
export function setReviewed(hunkIds: string[], value: boolean) {
  for (const id of hunkIds) {
    if (value && !reviewed[id]) reviewed[id] = 1
    else if (!value && reviewed[id]) delete reviewed[id]
  }
}

export interface Progress {
  done: number
  total: number
}

export function fileProgress(f: FileDiff): Progress {
  let done = 0
  for (const h of f.hunks) if (reviewed[h.id]) done++
  return { done, total: f.hunks.length }
}

export function sessionProgress(s: SessionInfo): Progress {
  let done = 0
  let total = 0
  for (const f of s.files) {
    const p = fileProgress(f)
    done += p.done
    total += p.total
  }
  return { done, total }
}

export function isFileDone(p: Progress): boolean {
  return p.total > 0 && p.done === p.total
}
