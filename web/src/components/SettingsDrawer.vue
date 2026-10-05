<script setup lang="ts">
import { reactive, ref } from 'vue'
import { message } from 'ant-design-vue'
import { store, activeProvider } from '../store'
import { testProvider } from '../api'
import type { Provider } from '../types'

interface Preset {
  label: string
  type: 'anthropic' | 'openai'
  baseUrl: string
  model: string
}

const PRESETS: Preset[] = [
  { label: 'Anthropic', type: 'anthropic', baseUrl: 'https://api.anthropic.com', model: 'claude-sonnet-5-5' },
  { label: 'DeepSeek', type: 'openai', baseUrl: 'https://api.deepseek.com', model: 'deepseek-chat' },
  { label: 'Kimi（Moonshot）', type: 'openai', baseUrl: 'https://api.moonshot.cn/v1', model: 'kimi-k2-0905-preview' },
  { label: '通义千问（DashScope）', type: 'openai', baseUrl: 'https://dashscope.aliyuncs.com/compatible-mode/v1', model: 'qwen-plus' },
  { label: '智谱 GLM', type: 'openai', baseUrl: 'https://open.bigmodel.cn/api/paas/v4', model: 'glm-4.5' },
  { label: 'OpenRouter', type: 'openai', baseUrl: 'https://openrouter.ai/api/v1', model: 'anthropic/claude-sonnet-4.5' },
  { label: 'Ollama（本地）', type: 'openai', baseUrl: 'http://localhost:11434/v1', model: 'qwen2.5-coder' },
  { label: '自定义 OpenAI 兼容', type: 'openai', baseUrl: '', model: '' },
]

const modalVisible = ref(false)
const editing = reactive<Provider>({ id: '', name: '', type: 'openai', baseUrl: '', apiKey: '', model: '' })
const presetKey = ref<string>('自定义 OpenAI 兼容')
const testing = ref(false)
const isNew = ref(true)

function openAdd() {
  isNew.value = true
  presetKey.value = '自定义 OpenAI 兼容'
  Object.assign(editing, { id: crypto.randomUUID(), name: '', type: 'openai', baseUrl: '', apiKey: '', model: '' })
  modalVisible.value = true
}

function openEdit(p: Provider) {
  isNew.value = false
  Object.assign(editing, { ...p })
  modalVisible.value = true
}

function applyPreset() {
  const p = PRESETS.find(x => x.label === presetKey.value)
  if (!p) return
  editing.type = p.type
  editing.baseUrl = p.baseUrl
  editing.model = p.model
  if (!editing.name || PRESETS.some(x => x.label === editing.name)) editing.name = p.label
}

function save() {
  if (!editing.name.trim()) return message.warning('请填写名称')
  if (editing.type === 'openai' && !editing.baseUrl.trim()) return message.warning('请填写 Base URL')
  if (editing.type === 'openai' && !editing.model.trim()) return message.warning('请填写模型名称')
  const p = { ...editing, name: editing.name.trim() }
  const i = store.providers.findIndex(x => x.id === p.id)
  if (i >= 0) store.providers[i] = p
  else store.providers.push(p)
  if (!store.activeProviderId) store.activeProviderId = p.id
  modalVisible.value = false
  message.success(`已保存「${p.name}」`)
}

function remove(id: string) {
  const i = store.providers.findIndex(x => x.id === id)
  if (i < 0) return
  const name = store.providers[i].name
  store.providers.splice(i, 1)
  if (store.activeProviderId === id) store.activeProviderId = store.providers[0]?.id || ''
  message.success(`已删除「${name}」`)
}

async function testConn() {
  testing.value = true
  try {
    const r = await testProvider({ ...editing })
    if (r.ok) message.success(`连接成功（${r.ms}ms）${r.reply ? `，模型回复：${r.reply}` : ''}`)
    else message.error(r.error || '连接失败')
  } catch (e) {
    message.error((e as Error).message)
  } finally {
    testing.value = false
  }
}
</script>

<template>
  <a-drawer v-model:open="store.settingsVisible" title="设置" width="440">
    <a-form layout="vertical">
      <a-form-item label="GitHub Token（访问私有仓库 / 提高限流额度）">
        <a-input-password v-model:value="store.ghToken" placeholder="ghp_... 或 github_pat_..." allow-clear />
      </a-form-item>
    </a-form>

    <a-divider style="margin: 8px 0 16px">模型供应商</a-divider>

    <div v-if="store.providers.length" class="provider-list">
      <div
        v-for="p in store.providers"
        :key="p.id"
        class="provider-item"
        :class="{ active: activeProvider()?.id === p.id }"
        @click="store.activeProviderId = p.id"
      >
        <a-radio :checked="activeProvider()?.id === p.id" />
        <div class="provider-info">
          <div class="provider-name">
            {{ p.name }}
            <a-tag :color="p.type === 'anthropic' ? 'purple' : 'blue'" style="margin-left: 4px">
              {{ p.type === 'anthropic' ? 'Anthropic' : 'OpenAI 兼容' }}
            </a-tag>
          </div>
          <div class="provider-meta">{{ p.model || '默认模型' }} · {{ p.baseUrl || '官方接口' }}</div>
        </div>
        <a-button size="small" type="text" @click.stop="openEdit(p)">编辑</a-button>
        <a-popconfirm title="确定删除该供应商？" @confirm="remove(p.id)">
          <a-button size="small" type="text" danger @click.stop>删除</a-button>
        </a-popconfirm>
      </div>
    </div>
    <a-empty v-else description="还没有供应商" :image-style="{ height: '60px' }" />

    <a-button type="dashed" block style="margin-top: 12px" @click="openAdd">＋ 添加供应商</a-button>

    <a-alert
      style="margin-top: 16px"
      type="info"
      show-icon
      message="未配置或未选中供应商时，后端回退到环境变量（ANTHROPIC_API_KEY / ANTHROPIC_BASE_URL / ANTHROPIC_MODEL）。配置保存在浏览器 localStorage，仅随请求发给本地后端。"
    />

    <a-modal v-model:open="modalVisible" :title="isNew ? '添加供应商' : '编辑供应商'" :footer="null" width="420px">
      <a-form layout="vertical" style="margin-top: 12px">
        <a-form-item v-if="isNew" label="预设（自动填充接口类型 / Base URL / 模型）">
          <a-select v-model:value="presetKey" @change="applyPreset">
            <a-select-option v-for="p in PRESETS" :key="p.label" :value="p.label">{{ p.label }}</a-select-option>
          </a-select>
        </a-form-item>
        <a-form-item label="名称">
          <a-input v-model:value="editing.name" placeholder="例如：公司 DeepSeek" />
        </a-form-item>
        <a-form-item label="接口类型">
          <a-select v-model:value="editing.type" :disabled="isNew">
            <a-select-option value="openai">OpenAI 兼容（/chat/completions）</a-select-option>
            <a-select-option value="anthropic">Anthropic（/v1/messages）</a-select-option>
          </a-select>
        </a-form-item>
        <a-form-item :label="editing.type === 'anthropic' ? 'Base URL（留空用官方，可填中转地址）' : 'Base URL'">
          <a-input v-model:value="editing.baseUrl" placeholder="https://api.example.com/v1" />
        </a-form-item>
        <a-form-item label="API Key">
          <a-input-password v-model:value="editing.apiKey" placeholder="sk-..." allow-clear />
        </a-form-item>
        <a-form-item :label="editing.type === 'anthropic' ? '模型（留空默认 claude-sonnet-5-5）' : '模型'">
          <a-input v-model:value="editing.model" placeholder="deepseek-chat / claude-sonnet-5-5 ..." />
        </a-form-item>
        <a-space style="width: 100%; justify-content: flex-end">
          <a-button :loading="testing" @click="testConn">测试连接</a-button>
          <a-button @click="modalVisible = false">取消</a-button>
          <a-button type="primary" @click="save">保存</a-button>
        </a-space>
      </a-form>
    </a-modal>
  </a-drawer>
</template>

<style scoped>
.provider-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.provider-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 12px;
  border: 1px solid #d9d9d9;
  border-radius: 8px;
  cursor: pointer;
  transition: border-color 0.2s;
}
.provider-item:hover { border-color: #4096ff; }
.provider-item.active { border-color: #1677ff; background: #e6f4ff; }
.provider-info { flex: 1; min-width: 0; }
.provider-name { font-weight: 600; font-size: 13.5px; }
.provider-meta {
  color: #8c8c8c;
  font-size: 12px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
