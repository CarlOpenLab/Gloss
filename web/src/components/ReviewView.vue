<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { message } from 'ant-design-vue'
import { store, resetSession } from '../store'
import DiffFile from './DiffFile.vue'
import FileTree from './FileTree.vue'
import { fileProgress, isFileDone, reviewed, sessionProgress, toggleReviewed } from '../review'
import { fuzzyMatch } from '../fileTree'
import { sidebar } from '../sidebar'

const session = computed(() => store.session!)

const progress = computed(() => sessionProgress(session.value))
const allReviewed = computed(() => isFileDone(progress.value))
const percent = computed(() => (progress.value.total ? Math.round((progress.value.done / progress.value.total) * 100) : 0))

const onlyUnreviewed = ref(false)
const tree = ref<InstanceType<typeof FileTree> | null>(null)

/** 左侧文件树和右侧内容用同一份过滤结果，保证索引一致 */
const rows = computed(() =>
  session.value.files
    .map((f, i) => ({ f, i }))
    .filter(({ f }) => !onlyUnreviewed.value || !isFileDone(fileProgress(f))),
)

/** 侧边栏筛选框同时过滤文件树和右侧正文（和 godiff 一致） */
const shownRows = computed(() =>
  rows.value.filter(({ f }) => fuzzyMatch(f.newPath || f.oldPath, sidebar.filter.trim())),
)

function scrollTo(i: number) {
  document.getElementById(`file-${i}`)?.scrollIntoView({ behavior: 'smooth', block: 'start' })
}

// 滚动时高亮文件树里当前看到的文件（godiff 的 selectTreeFile）
let raf = 0
function trackActive() {
  let cur = -1
  for (const el of document.querySelectorAll<HTMLElement>('.diff-file')) {
    if (el.getBoundingClientRect().top > 130) break
    cur = Number(el.dataset.fileIndex)
  }
  sidebar.activeIndex = cur
}
function onScroll() {
  cancelAnimationFrame(raf)
  raf = requestAnimationFrame(trackActive)
}

/** 当前正在读的块：视口顶部（顶栏下方）那一个，都没有就取第一个 */
function currentHunk(): HTMLElement | null {
  const els = [...document.querySelectorAll<HTMLElement>('.hunk')]
  if (!els.length) return null
  let cur = els[0]
  for (const el of els) {
    if (el.getBoundingClientRect().top > 100) break
    cur = el
  }
  return cur
}

function flash(el: HTMLElement) {
  el.classList.remove('flash')
  void el.offsetWidth // 重置动画
  el.classList.add('flash')
  window.setTimeout(() => el.classList.remove('flash'), 800)
}

function onKey(e: KeyboardEvent) {
  if (e.key === '/' && !e.metaKey && !e.ctrlKey && !e.altKey) {
    const t = e.target as HTMLElement | null
    if (!t || (t.tagName !== 'INPUT' && t.tagName !== 'TEXTAREA' && !t.isContentEditable)) {
      e.preventDefault()
      tree.value?.focus()
      return
    }
  }
  if (e.key !== 'r' && e.key !== 'R') return
  if (e.metaKey || e.ctrlKey || e.altKey) return
  const t = e.target as HTMLElement | null
  // 只在「正在打字」的地方让路；勾选框/按钮上按 R 仍然当作快捷键
  const typing =
    !!t &&
    (t.isContentEditable ||
      t.tagName === 'TEXTAREA' ||
      (t.tagName === 'INPUT' &&
        !['checkbox', 'radio', 'button', 'submit', 'reset', 'range', 'file', 'color'].includes(
          (t as HTMLInputElement).type,
        )))
  if (typing) return
  const el = currentHunk()
  const id = el?.dataset.hunkId
  if (!el || !id) return
  e.preventDefault()
  toggleReviewed(id)
  flash(el)
  message.success(reviewed[id] ? '已标记为已 review' : '已取消 review 标记')
}

onMounted(() => {
  window.addEventListener('keydown', onKey)
  window.addEventListener('scroll', onScroll, { passive: true })
  trackActive()
})
onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKey)
  window.removeEventListener('scroll', onScroll)
  cancelAnimationFrame(raf)
})
</script>

<template>
  <div>
    <div class="review-sub">
      <a-button size="small" @click="resetSession">← 返回</a-button>
      <span class="title">
        <a v-if="session.htmlUrl" :href="session.htmlUrl" target="_blank">{{ session.title }}</a>
        <template v-else>{{ session.title }}</template>
      </span>
      <div class="sub-right">
        <span class="meta">{{ session.files.length }} 个文件</span>
        <span class="meta">
          <b class="stat-add">+{{ session.totalAdd }}</b>
          <b class="stat-del">−{{ session.totalDel }}</b>
        </span>
        <span class="sep" />
        <span
          class="review-progress"
          :class="{ all: allReviewed }"
          title="已勾选「已 review」的代码块 / 全部代码块"
        >
          <span class="bar"><i :style="{ width: percent + '%' }" /></span>
          <span class="txt">{{ progress.done }}/{{ progress.total }}</span>
        </span>
        <a-checkbox v-model:checked="onlyUnreviewed" class="filter-check">只看未 review</a-checkbox>
        <span class="kbd-hint" title="按 / 聚焦文件搜索，按 R 标记当前正在读的块">按 <kbd>/</kbd> 搜索 · <kbd>R</kbd> 标记</span>
      </div>
    </div>

    <div class="review-body">
      <FileTree
        ref="tree"
        :rows="shownRows"
        :progress="progress"
        @select="scrollTo"
      />

      <main class="diff-files">
        <DiffFile
          v-for="row in shownRows"
          :id="`file-${row.i}`"
          :key="row.i"
          :file="row.f"
          :file-index="row.i"
          :hide-reviewed="onlyUnreviewed"
        />
        <div v-if="!rows.length" class="all-done">🎉 这个 diff 的变更块都 review 完了</div>
        <div v-else-if="!shownRows.length" class="all-done">没有匹配「{{ sidebar.filter }}」的文件</div>
      </main>
    </div>
  </div>
</template>
