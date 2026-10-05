import hljs from 'highlight.js/lib/common'

const extMap: Record<string, string> = {
  ts: 'typescript', tsx: 'typescript', js: 'javascript', jsx: 'javascript', mjs: 'javascript',
  vue: 'xml', go: 'go', py: 'python', java: 'java', rs: 'rust', rb: 'ruby', php: 'php',
  c: 'c', h: 'c', cpp: 'cpp', hpp: 'cpp', cc: 'cpp', cs: 'csharp',
  css: 'css', scss: 'scss', less: 'less', html: 'xml', xml: 'xml', svg: 'xml',
  json: 'json', md: 'markdown', yml: 'yaml', yaml: 'yaml', toml: 'ini', ini: 'ini',
  sh: 'bash', bash: 'bash', zsh: 'bash', sql: 'sql', swift: 'swift', kt: 'kotlin',
  dockerfile: 'dockerfile', makefile: 'makefile', gradle: 'gradle', proto: 'protobuf',
}

export function langOf(path: string): string | null {
  const base = path.split('/').pop()?.toLowerCase() || ''
  if (base === 'dockerfile') return 'dockerfile'
  if (base === 'makefile') return 'makefile'
  const m = /\.([a-z0-9]+)$/.exec(base)
  return m ? extMap[m[1]] || null : null
}

function escapeHtml(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}

/** 高亮单行代码；失败时回退为转义纯文本 */
export function highlightLine(content: string, lang: string | null): string {
  if (!content) return ''
  if (lang) {
    try {
      return hljs.highlight(content, { language: lang, ignoreIllegals: true }).value
    } catch {
      // fallthrough
    }
  }
  return escapeHtml(content)
}
