<script setup lang="ts">
import { computed } from 'vue'
import type { FileDiff, FileStatus } from '../types'
import { langOf } from '../hl'
import DiffHunk from './DiffHunk.vue'
import { fileProgress, isFileDone, reviewed, setReviewed } from '../review'
import { isFileCollapsed, sidebar, toggleFileCollapsed } from '../sidebar'

const props = defineProps<{ file: FileDiff; fileIndex: number; hideReviewed?: boolean }>()

/** 文件在新旧两侧的展示路径（重命名时两侧都给出） */
const path = computed(() => props.file.newPath || props.file.oldPath)

const displayPath = computed(() => {
  const f = props.file
  if (f.status === 'renamed') return `${f.oldPath} → ${f.newPath}`
  return path.value
})

const statusText: Record<FileStatus, string> = {
  added: '新增',
  deleted: '删除',
  modified: '修改',
  renamed: '重命名',
}

const lang = computed(() => langOf(path.value))

const progress = computed(() => fileProgress(props.file))
const allReviewed = computed(() => isFileDone(progress.value))
const collapsed = computed(() => isFileCollapsed(path.value))

// 「只看未 review」模式下，已经勾掉的整块直接不渲染
const visibleHunks = computed(() =>
  props.hideReviewed ? props.file.hunks.filter(h => !reviewed[h.id]) : props.file.hunks,
)

/** 文件级「已 review」：一次勾掉整个文件，像 godiff 的 Viewed 一样顺手折叠 */
function toggleViewed() {
  const mark = !allReviewed.value
  setReviewed(props.file.hunks.map(h => h.id), mark)
  sidebar.collapsedFiles[path.value] = mark
}
</script>

<template>
  <div class="diff-file" :class="{ collapsed }" :data-file-index="fileIndex">
    <div class="file-header">
      <button
        class="fh-chev"
        :class="{ closed: collapsed }"
        :title="collapsed ? '展开这个文件' : '折叠这个文件'"
        @click="toggleFileCollapsed(path)"
      >
        <svg viewBox="0 0 16 16" width="12" height="12" aria-hidden="true">
          <path
            fill="currentColor"
            d="M12.78 6.22a.75.75 0 0 1 0 1.06l-4.25 4.25a.75.75 0 0 1-1.06 0L3.22 7.28a.75.75 0 0 1 1.06-1.06L8 9.94l3.72-3.72a.75.75 0 0 1 1.06 0Z"
          />
        </svg>
      </button>
      <span class="fh-status" :class="`fh-${file.status}`">{{ statusText[file.status] }}</span>
      <span class="path">{{ displayPath }}</span>
      <span class="a">+{{ file.addCount }}</span>
      <span class="d">−{{ file.delCount }}</span>
      <button
        class="viewed-pill"
        :class="{ on: allReviewed }"
        :title="allReviewed ? '取消整个文件的 review 标记' : '把这个文件的所有块标记为已 review'"
        @click="toggleViewed"
      >
        <span class="box">{{ allReviewed ? '✓' : '' }}</span>
        <span>已 review</span>
        <span v-if="progress.total > 1" class="pill-count">{{ progress.done }}/{{ progress.total }}</span>
      </button>
    </div>
    <div v-show="!collapsed" class="hunks">
      <DiffHunk
        v-for="h in visibleHunks"
        :key="h.id"
        :hunk="h"
        :file-index="fileIndex"
        :lang="lang"
      />
    </div>
    <div v-if="!file.hunks.length" class="no-hunks">
      该文件没有文本变更（可能是二进制文件或仅文件模式变更）
    </div>
  </div>
</template>
