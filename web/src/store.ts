import { reactive, watch } from 'vue'
import type { Provider, SessionInfo } from './types'

function loadProviders(): Provider[] {
  try {
    const v = JSON.parse(localStorage.getItem('de_providers') || 'null')
    if (Array.isArray(v) && v.length) return v
  } catch {
    // ignore
  }
  // 迁移旧版单 Anthropic Key 配置
  const oldKey = localStorage.getItem('de_api_key')
  if (oldKey) {
    localStorage.removeItem('de_api_key')
    return [{ id: 'default', name: 'Anthropic', type: 'anthropic', baseUrl: '', apiKey: oldKey, model: '' }]
  }
  return []
}

export const store = reactive({
  session: null as SessionInfo | null,
  ghToken: localStorage.getItem('de_gh_token') || '',
  providers: loadProviders() as Provider[],
  activeProviderId: localStorage.getItem('de_active_provider') || '',
  settingsVisible: false,
})

watch(() => store.ghToken, v => localStorage.setItem('de_gh_token', v))
watch(() => store.providers, v => localStorage.setItem('de_providers', JSON.stringify(v)), { deep: true })
watch(() => store.activeProviderId, v => localStorage.setItem('de_active_provider', v))

/** 当前激活的供应商；没配过返回 undefined（后端走环境变量回退） */
export function activeProvider(): Provider | undefined {
  return store.providers.find(p => p.id === store.activeProviderId) || store.providers[0]
}

export function resetSession() {
  store.session = null
}
