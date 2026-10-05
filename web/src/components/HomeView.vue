<script setup lang="ts">
import { ref } from 'vue'
import { message } from 'ant-design-vue'
import { createPRSession, createDiffSession } from '../api'
import { store } from '../store'

const tab = ref<'pr' | 'diff'>('pr')
const prUrl = ref('')
const rawDiff = ref('')
const loading = ref(false)

async function loadPR() {
  if (!prUrl.value.trim()) {
    message.warning('请先粘贴 PR 链接')
    return
  }
  loading.value = true
  try {
    store.session = await createPRSession(prUrl.value.trim())
  } catch (e) {
    message.error((e as Error).message)
  } finally {
    loading.value = false
  }
}

async function loadDiff() {
  if (!rawDiff.value.trim()) {
    message.warning('请先粘贴 git diff 的输出')
    return
  }
  loading.value = true
  try {
    store.session = await createDiffSession(rawDiff.value)
  } catch (e) {
    message.error((e as Error).message)
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="home">
    <div class="hero">
      <h1>🔍 diff-explainer</h1>
      <p>GitHub 风格 diff 视图 + 每个代码块的 AI 解释按钮，review 不再逐行啃代码</p>
    </div>

    <a-card>
      <a-tabs v-model:activeKey="tab">
        <a-tab-pane key="pr" tab="GitHub PR">
          <a-space direction="vertical" style="width: 100%" :size="12">
            <a-input
              v-model:value="prUrl"
              size="large"
              placeholder="https://github.com/owner/repo/pull/123"
              @press-enter="loadPR"
            />
            <a-button type="primary" size="large" block :loading="loading" @click="loadPR">
              加载 PR
            </a-button>
            <a-typography-text type="secondary" style="font-size: 12px">
              公开仓库直接可用；私有仓库请在右上角「设置」里配置 GitHub Token
            </a-typography-text>
          </a-space>
        </a-tab-pane>

        <a-tab-pane key="diff" tab="本地 Diff">
          <a-space direction="vertical" style="width: 100%" :size="12">
            <a-textarea
              v-model:value="rawDiff"
              :rows="10"
              placeholder="把 git diff 的输出粘贴到这里，例如：git diff HEAD~1 、git diff main...feature 、git show"
              style="font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 12px"
            />
            <a-button type="primary" size="large" block :loading="loading" @click="loadDiff">
              解析 Diff
            </a-button>
            <a-typography-text type="secondary" style="font-size: 12px">
              小技巧：git diff | pbcopy 一键复制，适合提交前自查 AI 刚写的代码
            </a-typography-text>
          </a-space>
        </a-tab-pane>
      </a-tabs>
    </a-card>

    <div class="feature-row">
      <div class="feature-card"><b>📄 GitHub 风格 diff</b>熟悉的红增绿删 + 行号 + 语法高亮</div>
      <div class="feature-card"><b>✨ 逐块 AI 解释</b>每个 hunk 一个按钮，人话讲清增删意图</div>
      <div class="feature-card"><b>⚡ 流式 + 缓存</b>解释秒出，同一代码块不重复花钱</div>
    </div>
  </div>
</template>
