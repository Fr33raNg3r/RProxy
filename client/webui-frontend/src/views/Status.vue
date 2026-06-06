<template>
  <div>
    <div class="page-header">
      <h2>系统状态</h2>
      <span class="text-muted text-sm">{{ statusTime }}</span>
    </div>

    <n-alert v-if="error" type="error" closable style="margin-bottom: 14px;" @close="error = ''">{{ error }}</n-alert>

    <div class="card-stack">
      <n-card title="组件运行状态" size="medium">
        <n-table :bordered="false" :single-line="false" size="small">
          <thead>
            <tr><th>组件</th><th>状态</th><th>说明</th><th style="width: 100px;">操作</th></tr>
          </thead>
          <tbody>
            <tr v-for="row in services" :key="row.key">
              <td>{{ row.label }}</td>
              <td>
                <n-tag :type="row.tagType" size="small" round>{{ row.text }}</n-tag>
              </td>
              <td class="text-muted text-sm">{{ row.desc }}</td>
              <td>
                <n-button size="tiny" @click="restart(row.key)" :disabled="row.disableRestart">重启</n-button>
              </td>
            </tr>
          </tbody>
        </n-table>
      </n-card>

      <n-card title="透明代理健康检查" size="medium">
        <table class="kv-table">
          <tbody>
            <tr>
              <td>代理连通性</td>
              <td>
                <n-tag v-if="status.health?.proxy_ok === 1" type="success" size="small" round>正常</n-tag>
                <n-tag v-else-if="status.current_node_id" type="error" size="small" round>异常</n-tag>
                <n-tag v-else size="small" round>无活动节点</n-tag>
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
              <td><n-tag type="info" size="small" round>{{ formatAction(status.health.last_action) }}</n-tag></td>
            </tr>
            <tr>
              <td>失败计数 / 重启计数</td>
              <td class="text-mono">{{ status.health?.fail_count || 0 }} / {{ status.health?.restart_count || 0 }}</td>
            </tr>
          </tbody>
        </table>
      </n-card>

      <n-card title="节点" size="medium">
        <p>共配置 <strong>{{ status.node_count || 0 }}</strong> 个节点</p>
        <n-button size="small" @click="$router.push('/nodes')">前往节点管理 →</n-button>
      </n-card>

      <n-card v-if="peers.length" size="medium">
        <template #header>
          <div style="display: flex; align-items: center; justify-content: space-between; width: 100%;">
            <span>WireGuard 实时网速</span>
            <span class="text-muted text-sm">单位 KB/s</span>
          </div>
        </template>
        <n-table :bordered="false" :single-line="false" size="small">
          <thead>
            <tr>
              <th>Peer</th>
              <th>分配 IP</th>
              <th style="width: 120px;">↑ 上传</th>
              <th style="width: 120px;">↓ 下载</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="p in peers" :key="p.id">
              <td><strong>{{ p.name }}</strong></td>
              <td class="text-mono text-sm">{{ p.address }}</td>
              <td class="text-mono">{{ getSpeed(p.public_key, 'tx') }} <span class="text-muted text-xs">KB/s</span></td>
              <td class="text-mono">{{ getSpeed(p.public_key, 'rx') }} <span class="text-muted text-xs">KB/s</span></td>
            </tr>
          </tbody>
        </n-table>
      </n-card>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, onUnmounted, computed } from 'vue'
import { useMessage } from 'naive-ui'
import { api } from '../api'

const status = ref({})
const error = ref('')
const message = useMessage()
let sse = null

const peers = ref([])
const speeds = ref({})
let speedSse = null

// 与 WireGuard 页一致：服务端 rx = 客户端上传，服务端 tx = 客户端下载
function getSpeed(pubKey, dir) {
  const s = speeds.value[pubKey]
  if (!s) return '0'
  return dir === 'rx' ? (s.tx_kbps || 0) : (s.rx_kbps || 0)
}

function startSpeedSSE() {
  speedSse = new EventSource('/api/wireguard/speed/stream', { withCredentials: true })
  speedSse.onmessage = (ev) => {
    try {
      const data = JSON.parse(ev.data)
      const m = {}
      for (const s of (data.speeds || [])) m[s.public_key] = s
      speeds.value = m
    } catch {}
  }
  speedSse.onerror = () => {
    if (speedSse) {
      speedSse.close()
      speedSse = null
      setTimeout(startSpeedSSE, 5000)
    }
  }
}

const statusTime = computed(() => {
  if (!status.value.timestamp) return ''
  return new Date(status.value.timestamp).toLocaleTimeString()
})

const services = computed(() => ([
  {
    key: 'xray',
    label: 'Xray',
    desc: '透明代理核心',
    text: status.value.services?.xray ? '运行中' : '已停止',
    tagType: status.value.services?.xray ? 'success' : 'error',
    disableRestart: false
  },
  {
    key: 'mosdns',
    label: 'mosdns',
    desc: 'DNS 分流',
    text: status.value.services?.mosdns ? '运行中' : '已停止',
    tagType: status.value.services?.mosdns ? 'success' : 'error',
    disableRestart: false
  },
  {
    key: 'wg',
    label: 'WireGuard',
    desc: '远程接入入站',
    text: status.value.services?.wg ? '运行中' : '未启用',
    tagType: status.value.services?.wg ? 'success' : 'default',
    disableRestart: !status.value.services?.wg
  }
]))

function formatAction(a) {
  return { restart_xray: '重启 Xray', switch_node: '切换节点' }[a] || a
}

async function restart(name) {
  try {
    await api.restartService(name)
    message.success(`${name} 重启已发起`)
  } catch (e) {
    error.value = e.message
  }
}

function startSSE() {
  sse = new EventSource('/api/status/stream', { withCredentials: true })
  sse.onmessage = (ev) => {
    try { status.value = JSON.parse(ev.data) } catch {}
  }
  sse.onerror = () => {
    if (sse) {
      sse.close()
      sse = null
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
  try {
    const data = await api.listPeers()
    peers.value = data.peers || []
  } catch {}
  startSSE()
  if (peers.value.length) startSpeedSSE()
})

onUnmounted(() => {
  if (sse) sse.close()
  if (speedSse) speedSse.close()
})
</script>
