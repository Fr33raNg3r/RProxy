<template>
  <div>
    <div class="main-header">
      <h2>系统状态</h2>
      <span class="text-muted text-sm">{{ statusTime }}</span>
    </div>

    <div v-if="error" class="alert alert-error">{{ error }}</div>

    <!-- 组件状态 -->
    <div class="card">
      <div class="card-title">组件运行状态</div>
      <table class="table">
        <thead>
          <tr><th>组件</th><th>状态</th><th>说明</th><th>操作</th></tr>
        </thead>
        <tbody>
          <tr>
            <td>Xray</td>
            <td>
              <span class="badge" :class="status.services?.xray ? 'badge-success' : 'badge-danger'">
                {{ status.services?.xray ? '运行中' : '已停止' }}
              </span>
            </td>
            <td class="text-muted text-sm">透明代理核心</td>
            <td><button class="btn btn-sm" @click="restart('xray')">重启</button></td>
          </tr>
          <tr>
            <td>mosdns</td>
            <td>
              <span class="badge" :class="status.services?.mosdns ? 'badge-success' : 'badge-danger'">
                {{ status.services?.mosdns ? '运行中' : '已停止' }}
              </span>
            </td>
            <td class="text-muted text-sm">DNS 分流</td>
            <td><button class="btn btn-sm" @click="restart('mosdns')">重启</button></td>
          </tr>
          <tr>
            <td>WireGuard</td>
            <td>
              <span class="badge" :class="status.services?.wg ? 'badge-success' : 'badge-muted'">
                {{ status.services?.wg ? '运行中' : '未启用' }}
              </span>
            </td>
            <td class="text-muted text-sm">远程接入入站</td>
            <td><button class="btn btn-sm" @click="restart('wg')" :disabled="!status.services?.wg">重启</button></td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- 透明代理健康 -->
    <div class="card">
      <div class="card-title">透明代理健康检查</div>
      <table class="table">
        <tbody>
          <tr>
            <td style="width: 200px;">代理连通性</td>
            <td>
              <span v-if="status.health?.proxy_ok === 1" class="badge badge-success">正常</span>
              <span v-else-if="status.current_node_id" class="badge badge-danger">异常</span>
              <span v-else class="badge badge-muted">无活动节点</span>
            </td>
          </tr>
          <tr>
            <td>当前活动节点</td>
            <td>
              <span v-if="status.current_node_name">
                <span class="text-mono">{{ status.current_node_name }}</span>
                <span class="text-muted text-xs"> ({{ status.current_node_id }})</span>
              </span>
              <span v-else class="text-muted">未选择 — 请到「节点管理」选择一个节点</span>
            </td>
          </tr>
          <tr>
            <td>最近检查时间</td>
            <td class="text-muted">{{ status.health?.last_check || '尚未检查' }}</td>
          </tr>
          <tr v-if="status.health?.last_action">
            <td>最近自动操作</td>
            <td><span class="badge badge-info">{{ formatAction(status.health.last_action) }}</span></td>
          </tr>
          <tr>
            <td>失败计数 / 重启计数</td>
            <td class="text-mono">{{ status.health?.fail_count || 0 }} / {{ status.health?.restart_count || 0 }}</td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- 节点摘要 -->
    <div class="card">
      <div class="card-title">节点</div>
      <p>共配置 <strong>{{ status.node_count || 0 }}</strong> 个节点</p>
      <router-link to="/nodes" class="btn btn-sm">前往节点管理 →</router-link>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, onUnmounted, computed } from 'vue'
import { api } from '../api'

const status = ref({})
const error = ref('')
let sse = null

const statusTime = computed(() => {
  if (!status.value.timestamp) return ''
  return new Date(status.value.timestamp).toLocaleTimeString()
})

function formatAction(a) {
  return { restart_xray: '重启 Xray', switch_node: '切换节点' }[a] || a
}

async function restart(name) {
  try {
    await api.restartService(name)
  } catch (e) {
    error.value = e.message
  }
}

function startSSE() {
  sse = new EventSource('/api/status/stream', { withCredentials: true })
  sse.onmessage = (ev) => {
    try {
      status.value = JSON.parse(ev.data)
    } catch {}
  }
  sse.onerror = () => {
    if (sse) {
      sse.close()
      sse = null
      // 5 秒后重连
      setTimeout(startSSE, 5000)
    }
  }
}

onMounted(async () => {
  try {
    status.value = await api.getStatus()
  } catch (e) {
    error.value = e.message
  }
  startSSE()
})

onUnmounted(() => {
  if (sse) sse.close()
})
</script>
