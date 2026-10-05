import { reactive } from 'vue'

/** 文件树（侧边栏）的界面状态 */
export const sidebar = reactive({
  filter: '',
  collapsedDirs: {} as Record<string, boolean>,
  collapsedFiles: {} as Record<string, boolean>,
  activeIndex: -1, // 当前滚动到的文件，用于高亮树里的那一行
})

export function toggleDir(key: string) {
  sidebar.collapsedDirs[key] = !sidebar.collapsedDirs[key]
}

export function isFileCollapsed(path: string): boolean {
  return !!sidebar.collapsedFiles[path]
}

export function toggleFileCollapsed(path: string) {
  sidebar.collapsedFiles[path] = !sidebar.collapsedFiles[path]
}
