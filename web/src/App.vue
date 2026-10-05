<script setup lang="ts">
import { computed } from 'vue'
import { store, resetSession, activeProvider } from './store'
import HomeView from './components/HomeView.vue'
import ReviewView from './components/ReviewView.vue'
import SettingsDrawer from './components/SettingsDrawer.vue'

const providerTag = computed(() => {
  const p = activeProvider()
  return p ? `${p.name} · ${p.model || '默认模型'}` : '未配置供应商（用环境变量）'
})
</script>

<template>
  <div>
    <header class="app-header">
      <span class="logo" @click="resetSession">🔍 diff-explainer</span>
      <span class="slogan">AI 代码审查助手 · 哪里不懂点哪里</span>
      <a-tag style="cursor: pointer" color="blue" @click="store.settingsVisible = true">
        🤖 {{ providerTag }}
      </a-tag>
      <a-button size="small" @click="store.settingsVisible = true">⚙️ 设置</a-button>
    </header>

    <HomeView v-if="!store.session" />
    <ReviewView v-else />

    <SettingsDrawer />
  </div>
</template>
