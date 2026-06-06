<template>
  <div>
    <div class="page-header">
      <h2>日志</h2>
      <div class="row-actions">
        <n-select v-model:value="component" :options="componentOpts" style="width: 180px;" />
        <n-select v-model:value="lines" :options="lineOpts" style="width: 140px;" />
        <n-button :loading="loading" @click="load">刷新</n-button>
      </div>
    </div>

    <n-alert v-if="error" type="error" closable style="margin-bottom: 12px;" @close="error = ''">{{ error }}</n-alert>

    <n-card size="medium" content-style="padding: 0;">
      <pre class="log-output">{{ content || '— 暂无日志 —' }}</pre>
    </n-card>
  </div>
</template>

<script setup>
import { ref, onMounted, watch } from 'vue'
import { api } from '../api'

const componentOpts = [
  { label: 'Xray', value: 'xray' },
  { label: 'mosdns', value: 'mosdns' },
  { label: 'WebUI', value: 'webui' },
  { label: '健康检查', value: 'watchdog' },
  { label: '每日更新', value: 'update' },
  { label: 'WireGuard', value: 'wg' }
]
const lineOpts = [
  { label: '最近 100 行', value: 100 },
  { label: '最近 500 行', value: 500 },
  { label: '最近 1000 行', value: 1000 },
  { label: '最近 2000 行', value: 2000 }
]

const component = ref('xray')
const lines = ref(200)
const content = ref('')
const error = ref('')
const loading = ref(false)

async function load() {
  loading.value = true
  error.value = ''
  try {
    const data = await api.getLogs(component.value, lines.value)
    content.value = data.content || ''
  } catch (e) {
    error.value = e.message
  } finally {
    loading.value = false
  }
}

watch([component, lines], load)
onMounted(load)
</script>
