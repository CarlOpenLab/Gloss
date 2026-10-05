export type LineType = 'add' | 'del' | 'context'

export interface DiffLine {
  type: LineType
  content: string
  oldNum?: number
  newNum?: number
}

export interface Hunk {
  id: string
  header: string
  section?: string
  oldStart: number
  oldLines: number
  newStart: number
  newLines: number
  lines: DiffLine[]
}

export type FileStatus = 'added' | 'deleted' | 'modified' | 'renamed'

export interface FileDiff {
  oldPath: string
  newPath: string
  status: FileStatus
  hunks: Hunk[]
  addCount: number
  delCount: number
}

export interface SessionInfo {
  sessionId: string
  source: 'pr' | 'diff'
  title: string
  htmlUrl?: string
  files: FileDiff[]
  totalAdd: number
  totalDel: number
}

export interface Provider {
  id: string
  name: string
  type: 'anthropic' | 'openai' // openai = OpenAI 兼容接口（DeepSeek/Kimi/通义/智谱/OpenRouter/Ollama…）
  baseUrl: string
  apiKey: string
  model: string
}
