<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import type { Hunk, LineType } from '../types'
import { highlightLine } from '../hl'
import { explainHunk } from '../api'
import { store, activeProvider } from '../store'
import { reviewed, toggleReviewed } from '../review'
import { openExplainCard } from '../explainCard'
import ExplainCard from './ExplainCard.vue'

const props = defineProps<{ hunk: Hunk; fileIndex: number; lang: string | null }>()

// 预渲染每行的高亮 HTML，避免解释文本更新时重复高亮
const renderedLines = props.hunk.lines.map(l => ({
  ...l,
  html: highlightLine(l.content, props.lang),
}))

const signs: Record<LineType, string> = { add: '+', del: '−', context: ' ' }

const state = reactive({
  loading: false,
  done: false,
  cached: false,
  text: '',
  providerId: '', // 生成当前解释时用的供应商，切换供应商后重新生成
})

const checked = computed(() => !!reviewed[props.hunk.id])
const open = computed(() => openExplainCard.value === props.hunk.id)
const anchor = ref<HTMLElement | null>(null)

/** @@ -60,9 +62,11 @@ 这一段，和后面的函数名分开排版 */
const range = computed(() => props.hunk.header.match(/^@@[^@]*@@/)?.[0] ?? props.hunk.header)
const section = computed(() => props.hunk.section || '')

function onToggle() {
  if (open.value) {
    openExplainCard.value = null
    return
  }
  openExplainCard.value = props.hunk.id
  void load()
}

async function load() {
  if (!store.session) return
  const pid = activeProvider()?.id || 'env'
  if (state.loading || (state.done && state.providerId === pid)) return
  state.providerId = pid
  state.loading = true
  state.text = ''
  state.cached = false
  await explainHunk(store.session.sessionId, props.fileIndex, props.hunk.id, {
    onDelta: t => {
      state.text += t
    },
    onDone: c => {
      state.loading = false
      state.done = true
      state.cached = c
    },
    onError: m => {
      state.loading = false
      state.done = true
      state.text += (state.text ? '\n\n' : '') + `> ⚠️ ${m}`
    },
  })
}
</script>

<template>
  <div class="hunk" :data-hunk-id="hunk.id">
    <div class="hunk-header" :class="{ 'is-reviewed': checked }">
      <a-checkbox
        class="review-check"
        :checked="checked"
        title="勾选表示这个代码块已经 review 过（状态存在本地，重开还在）"
        @change="toggleReviewed(hunk.id)"
      />
      <span class="hunk-range">{{ range }}</span>
      <span v-if="section" class="hunk-section" :title="section">{{ section }}</span>
      <span class="spacer" />
      <span v-if="checked" class="reviewed-tag">✓ 已 review</span>
      <span ref="anchor" class="explain-anchor">
        <a-button class="explain-btn" size="small" :loading="state.loading" @click="onToggle">
          {{ open ? '收起解释' : '✨ 解释' }}
        </a-button>
      </span>
    </div>

    <ExplainCard
      v-if="open"
      :anchor="anchor"
      :hunk-id="hunk.id"
      :header="hunk.header"
      :loading="state.loading"
      :cached="state.cached"
      :text="state.text"
      @close="openExplainCard = null"
    />

    <table class="diff-table">
      <tbody>
        <tr v-for="(l, i) in renderedLines" :key="i" :class="`line-${l.type}`">
          <td class="gutter">{{ l.oldNum || '' }}</td>
          <td class="gutter">{{ l.newNum || '' }}</td>
          <td class="code"><span class="sign">{{ signs[l.type] }}</span><span v-html="l.html"></span></td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
