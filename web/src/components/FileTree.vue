<script setup lang="ts">
import { computed, ref } from 'vue'
import type { FileDiff, FileStatus } from '../types'
import { buildTree, flatten } from '../fileTree'
import { sidebar, toggleDir } from '../sidebar'
import { fileProgress, isFileDone } from '../review'

const props = defineProps<{
  rows: { f: FileDiff; i: number }[]
  progress: { done: number; total: number }
}>()

const emit = defineEmits<{ (e: 'select', index: number): void }>()

const input = ref<HTMLInputElement | null>(null)

const tree = computed(() => buildTree(props.rows))
const visible = computed(() => flatten(tree.value, sidebar.collapsedDirs))
const byIndex = computed(() => new Map(props.rows.map(r => [r.i, r.f])))
/** 底部统计跟着当前筛选结果走（和 godiff 的 Total 一致） */
const totals = computed(() => {
  let add = 0
  let del = 0
  for (const r of props.rows) {
    add += r.f.addCount
    del += r.f.delCount
  }
  return { add, del }
})

const statusLetter: Record<FileStatus, string> = {
  added: 'A',
  deleted: 'D',
  modified: 'M',
  renamed: 'R',
}

/** 侧边栏筛选框也可以被外面的快捷键唤起 */
defineExpose({ focus: () => input.value?.focus() })

function clearFilter() {
  sidebar.filter = ''
  input.value?.focus()
}

function pathOf(f: FileDiff) {
  return f.newPath || f.oldPath
}

function progressOf(index: number) {
  const f = byIndex.value.get(index)
  return f ? fileProgress(f) : { done: 0, total: 0 }
}

function onRow(node: { dir: boolean; key: string; fileIndex: number }) {
  if (node.dir) {
    toggleDir(node.key)
    return
  }
  emit('select', node.fileIndex)
}
</script>

<template>
  <aside class="file-list">
    <div class="tree-filter">
      <svg class="ic" viewBox="0 0 16 16" width="13" height="13" aria-hidden="true">
        <path
          fill="currentColor"
          d="M10.68 11.74a6 6 0 0 1-7.922-8.982 6 6 0 0 1 8.982 7.922l3.04 3.04a.749.749 0 0 1-.326 1.275.749.749 0 0 1-.734-.215ZM11.5 7a4.499 4.499 0 1 0-8.997 0A4.499 4.499 0 0 0 11.5 7Z"
        />
      </svg>
      <input
        ref="input"
        v-model="sidebar.filter"
        type="text"
        placeholder="Filter files"
        spellcheck="false"
        @keydown.esc="clearFilter"
      />
      <button v-if="sidebar.filter" class="clear" title="清空（Esc）" @click="clearFilter">✕</button>
    </div>

    <div class="tree-scroll">
      <div v-if="!visible.length" class="tree-empty">
        {{ sidebar.filter ? '没有匹配的文件' : '没有变更文件' }}
      </div>

      <div
        v-for="row in visible"
        :key="row.node.key"
        class="tree-row"
        :class="{
          'is-dir': row.node.dir,
          active: !row.node.dir && row.node.fileIndex === sidebar.activeIndex,
          viewed: !row.node.dir && isFileDone(progressOf(row.node.fileIndex)),
        }"
        :style="{ paddingLeft: `${6 + row.depth * 14}px` }"
        :title="row.node.dir ? row.node.label : pathOf(byIndex.get(row.node.fileIndex)!)"
        @click="onRow(row.node)"
      >
        <span v-if="row.node.dir" class="chev" :class="{ closed: sidebar.collapsedDirs[row.node.key] }">
          <svg viewBox="0 0 16 16" width="12" height="12" aria-hidden="true">
            <path
              fill="currentColor"
              d="M12.78 6.22a.75.75 0 0 1 0 1.06l-4.25 4.25a.75.75 0 0 1-1.06 0L3.22 7.28a.75.75 0 0 1 1.06-1.06L8 9.94l3.72-3.72a.75.75 0 0 1 1.06 0Z"
            />
          </svg>
        </span>
        <span class="icon">
          <svg v-if="row.node.dir" viewBox="0 0 16 16" width="14" height="14" aria-hidden="true">
            <path
              fill="currentColor"
              d="M1.75 1A1.75 1.75 0 0 0 0 2.75v10.5C0 14.216.784 15 1.75 15h12.5A1.75 1.75 0 0 0 16 13.25v-8.5A1.75 1.75 0 0 0 14.25 3H7.5a.25.25 0 0 1-.2-.1l-.9-1.2C6.07 1.26 5.55 1 5 1H1.75Z"
            />
          </svg>
          <svg v-else viewBox="0 0 16 16" width="14" height="14" aria-hidden="true">
            <path
              fill="currentColor"
              d="M2 1.75C2 .784 2.784 0 3.75 0h6.586c.464 0 .909.184 1.237.513l2.914 2.914c.329.328.513.773.513 1.237v9.586A1.75 1.75 0 0 1 13.25 16h-9.5A1.75 1.75 0 0 1 2 14.25Zm1.75-.25a.25.25 0 0 0-.25.25v12.5c0 .138.112.25.25.25h9.5a.25.25 0 0 0 .25-.25V6h-2.75A1.75 1.75 0 0 1 9 4.25V1.5Zm6.75.062V4.25c0 .138.112.25.25.25h2.688l-.011-.013-2.914-2.914-.013-.011Z"
            />
          </svg>
        </span>
        <span class="label">{{ row.node.label }}</span>

        <template v-if="!row.node.dir">
          <span
            v-if="progressOf(row.node.fileIndex).done && !isFileDone(progressOf(row.node.fileIndex))"
            class="review-badge"
          >{{ progressOf(row.node.fileIndex).done }}/{{ progressOf(row.node.fileIndex).total }}</span>
          <span class="counts">
            <b class="a">+{{ byIndex.get(row.node.fileIndex)!.addCount }}</b>
            <b class="d">−{{ byIndex.get(row.node.fileIndex)!.delCount }}</b>
          </span>
          <span class="letter" :class="`letter-${byIndex.get(row.node.fileIndex)!.status}`">
            {{ statusLetter[byIndex.get(row.node.fileIndex)!.status] }}
          </span>
        </template>
      </div>
    </div>

    <div class="tree-foot">
      <span class="total" title="所有变更的增删行数">
        <b>Total:</b>
        <b class="a">+{{ totals.add }}</b>
        <b class="d">−{{ totals.del }}</b>
      </span>
      <span
        class="prog"
        :class="{ all: progress.total > 0 && progress.done === progress.total }"
        title="已勾选「已 review」的代码块 / 全部代码块"
      >✓ {{ progress.done }}/{{ progress.total }}</span>
    </div>
  </aside>
</template>
