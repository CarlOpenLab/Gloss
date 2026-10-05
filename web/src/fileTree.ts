import type { FileDiff } from './types'

export interface TreeNode {
  key: string
  label: string
  dir: boolean
  fileIndex: number
  children: TreeNode[]
}

export interface TreeRow {
  node: TreeNode
  depth: number
}

interface DirNode {
  dirs: Map<string, DirNode>
  files: Map<string, number>
  order: string[]
}

function newDir(): DirNode {
  return { dirs: new Map(), files: new Map(), order: [] }
}

/**
 * 把文件列表按路径拼成树。和 godiff 一样：只含一个子目录的目录链会合并成一行
 * （例如 internal/server），这样深层路径不会把侧边栏撑开。
 */
export function buildTree(entries: { f: FileDiff; i: number }[]): TreeNode[] {
  const root = newDir()
  for (const { f, i } of entries) {
    const path = f.newPath || f.oldPath
    const parts = path.split('/')
    let d = root
    for (const name of parts.slice(0, -1)) {
      let next = d.dirs.get(name)
      if (!next) {
        next = newDir()
        d.dirs.set(name, next)
        d.order.push('d' + name)
      }
      d = next
    }
    const name = parts[parts.length - 1]
    d.files.set(name, i)
    d.order.push('f' + name)
  }

  const walk = (d: DirNode, prefix: string): TreeNode[] => {
    const out: TreeNode[] = []
    for (const o of d.order) {
      const name = o.slice(1)
      if (o[0] === 'f') {
        out.push({ key: `f:${d.files.get(name)}`, label: name, dir: false, fileIndex: d.files.get(name)!, children: [] })
        continue
      }
      let sub = d.dirs.get(name)!
      let label = name
      let path = prefix + name
      while (sub.order.length === 1 && sub.order[0][0] === 'd') {
        const child = sub.order[0].slice(1)
        label += '/' + child
        path += '/' + child
        sub = sub.dirs.get(child)!
      }
      out.push({ key: `d:${path}`, label, dir: true, fileIndex: -1, children: walk(sub, path + '/') })
    }
    return out
  }
  return walk(root, '')
}

/** 展开成一维行列表，跳过折叠目录里的内容 */
export function flatten(nodes: TreeNode[], collapsed: Record<string, boolean>, depth = 0, out: TreeRow[] = []): TreeRow[] {
  for (const node of nodes) {
    out.push({ node, depth })
    if (node.dir && !collapsed[node.key]) flatten(node.children, collapsed, depth + 1, out)
  }
  return out
}

/** 子序列模糊匹配，大小写不敏感（和 godiff 的 Filter files 一致） */
export function fuzzyMatch(s: string, query: string): boolean {
  if (!query) return true
  const hay = s.toLowerCase()
  const q = query.toLowerCase()
  let i = 0
  for (const ch of hay) {
    if (i < q.length && ch === q[i]) i++
  }
  return i >= q.length
}
