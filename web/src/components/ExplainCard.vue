<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { marked } from 'marked'
import { isReviewed, toggleReviewed } from '../review'

const props = defineProps<{
  anchor: HTMLElement | null
  hunkId: string
  header: string
  loading: boolean
  cached: boolean
  text: string
}>()

const emit = defineEmits<{ (e: 'close'): void }>()

const card = ref<HTMLElement | null>(null)
const body = ref<HTMLElement | null>(null)
const pos = ref<Record<string, string>>({ opacity: '0' })

const MAX_W = 460
const MAX_H = 460
const GAP = 10

const rendered = computed(() => (props.text ? (marked.parse(props.text) as string) : ''))
const checked = computed(() => isReviewed(props.hunkId))

/** 贴着「✨ 解释」按钮定位，下方放不下就翻到上方，并夹在视口内 */
function place() {
  const el = props.anchor
  if (!el) return
  const r = el.getBoundingClientRect()
  const w = Math.min(MAX_W, window.innerWidth - 24)
  const left = Math.min(Math.max(12, r.right - w), Math.max(12, window.innerWidth - w - 12))
  const below = window.innerHeight - r.bottom - GAP - 12
  const above = r.top - GAP - 12
  const openUp = below < 240 && above > below
  const s: Record<string, string> = {
    left: `${left}px`,
    width: `${w}px`,
    maxHeight: `${Math.min(MAX_H, Math.max(180, openUp ? above : below))}px`,
    opacity: '1',
  }
  if (openUp) s.bottom = `${window.innerHeight - r.top + GAP}px`
  else s.top = `${r.bottom + GAP}px`
  pos.value = s
}

let raf = 0
function schedule() {
  cancelAnimationFrame(raf)
  raf = requestAnimationFrame(place)
}

function onDocMouseDown(e: MouseEvent) {
  const t = e.target as Node
  if (card.value?.contains(t)) return
  if (props.anchor?.contains(t)) return
  emit('close')
}

function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape') emit('close')
}

// 流式输出时跟着滚，但只在用户本来就贴着底部时（正在往上翻就不打扰）
watch(
  () => props.text,
  () => {
    const el = body.value
    if (!el || !props.loading) return
    if (el.scrollHeight - el.scrollTop - el.clientHeight < 48) {
      nextTick(() => {
        el.scrollTop = el.scrollHeight
      })
    }
  },
)

onMounted(() => {
  place()
  window.addEventListener('scroll', schedule, { passive: true, capture: true })
  window.addEventListener('resize', schedule)
  document.addEventListener('mousedown', onDocMouseDown)
  document.addEventListener('keydown', onKey)
})

onBeforeUnmount(() => {
  cancelAnimationFrame(raf)
  window.removeEventListener('scroll', schedule, { capture: true })
  window.removeEventListener('resize', schedule)
  document.removeEventListener('mousedown', onDocMouseDown)
  document.removeEventListener('keydown', onKey)
})
</script>

<template>
  <Teleport to="body">
    <div ref="card" class="explain-card" :style="pos">
      <div class="explain-card-head">
        <div class="ec-title-row">
          <span class="ec-title">🤖 AI 解释</span>
          <a-tag v-if="cached" color="green">缓存</a-tag>
          <span class="ec-spacer" />
          <button class="ec-close" title="关闭（Esc）" @click="emit('close')">✕</button>
        </div>
        <span class="ec-hunk" :title="header">{{ header }}</span>
      </div>

      <div ref="body" class="explain-card-body">
        <!-- 内容来自本地模型 API 调用，经 marked 渲染 -->
        <div v-if="rendered" class="explain-body" v-html="rendered"></div>
        <div v-if="loading" class="explain-loading"><a-spin size="small" /> 正在生成解释…</div>
        <div v-else-if="!rendered" class="explain-empty">没有拿到解释内容</div>
      </div>

      <div class="explain-card-foot">
        <a-checkbox :checked="checked" @change="toggleReviewed(hunkId)">已 review</a-checkbox>
      </div>
    </div>
  </Teleport>
</template>
