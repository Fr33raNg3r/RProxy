<template>
  <div>
    <div class="main-header">
      <h2>日志</h2>
      <div class="flex gap-2">
        <select v-model="component" class="form-control" style="width: 180px;">
          <option value="xray">Xray</option>
          <option value="mosdns">mosdns</option>
          <option value="webui">WebUI</option>
          <option value="watchdog">健康检查</option>
          <option value="update">每日更新</option>
          <option value="wg">WireGuard</option>
        </select>
        <select v-model.number="lines" class="form-control" style="width: 120px;">
          <option :value="100">最近 100 行</option>
          <option :value="500">最近 500 行</option>
          <option :value="1000">最近 1000 行</option>
          <option :value="2000">最近 2000 行</option>
        </select>
        <button class="btn" @click="load" :disabled="loading">
          {{ loading ? '加载中...' : '刷新' }}
        </button>
      </div>
    </div>

    <div v-if="error" class="alert alert-error">{{ error }}</div>

    <div class="card">
      <pre class="log-output">{{ content || '— 暂无日志 —' }}</pre>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, watch } from 'vue'
import { api } from '../api'

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

<style scoped>
.log-output {
  background: #1f2937;
  color: #e5e7eb;
  padding: 16px;
  border-radius: 6px;
  max-height: calc(100vh - 240px);
  overflow: auto;
  font-family: ui-monospace, "SF Mono", Menlo, Consolas, monospace;
  font-size: 12px;
  white-space: pre;
  margin: 0;
}
</style>
