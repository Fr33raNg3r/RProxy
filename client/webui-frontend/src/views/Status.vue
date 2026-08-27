<template>
  <div>
    <div class="page-header">
      <h2>系统状态</h2>
      <span class="text-muted text-sm">{{ statusTime }}</span>
    </div>

    <n-alert v-if="error" type="error" closable style="margin-bottom: 14px;" @close="error = ''">{{ error }}</n-alert>
    <n-alert v-if="success" type="success" closable style="margin-bottom: 14px;" @close="success = ''">{{ success }}</n-alert>

    <div class="card-grid">
      <n-card title="组件运行状态" size="medium">
        <n-table :bordered="false" :single-line="false" size="small">
          <thead>
            <tr><th>组件</th><th>状态</th><th>说明</th><th style="width: 100px;">操作</th></tr>
          </thead>
          <tbody>
            <tr v-for="row in services" :key="row.key">
              <td style="white-space: nowrap;">{{ row.label }}</td>
              <td>
                <n-tag :type="row.tagType" size="small" round>{{ row.text }}</n-tag>
              </td>
              <td class="text-muted text-sm">{{ row.desc }}</td>
              <td>
                <n-button class="table-btn" size="tiny" @click="restart(row.key)" :disabled="row.disableRestart">重启</n-button>
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
                <span v-else class="text-muted">未选择 — 请在下方节点列表选择一个节点</span>
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

    <n-card size="medium" style="margin-top: 16px;">
      <template #header>
        <div style="display: flex; align-items: center; justify-content: space-between; width: 100%;">
          <span>节点管理</span>
          <n-button size="small" type="primary" @click="openCreate">+ 添加节点</n-button>
        </div>
      </template>
      <n-empty v-if="nodes.length === 0" description="尚未添加任何节点。点击右上角「添加节点」开始。" />
      <n-table v-else :bordered="false" :single-line="false" size="small">
        <thead>
          <tr>
            <th style="width: 130px;">序号</th>
            <th>名称</th>
            <th>地址</th>
            <th>WS 路径</th>
            <th style="width: 80px;">状态</th>
            <th style="width: 320px;">操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="(n, idx) in nodes" :key="n.id" :class="{ 'is-current': n.id === currentNodeId }">
            <td>
              <div class="row-actions">
                <span class="text-mono">{{ idx + 1 }}</span>
                <n-button class="table-btn" size="tiny" @click="moveUp(idx)" :disabled="idx === 0" title="上移">↑</n-button>
                <n-button class="table-btn" size="tiny" @click="moveDown(idx)" :disabled="idx === nodes.length - 1" title="下移">↓</n-button>
              </div>
            </td>
            <td><strong>{{ n.name }}</strong></td>
            <td class="text-mono text-sm">{{ n.address }}:{{ n.port }}</td>
            <td class="text-mono text-sm">{{ n.ws_path }}</td>
            <td>
              <n-tag :type="n.enabled ? 'success' : 'default'" size="small" round>
                {{ n.enabled ? '启用' : '未启用' }}
              </n-tag>
            </td>
            <td>
              <div class="row-actions">
                <n-button class="table-btn" size="tiny" type="primary" @click="switchTo(n)" :disabled="n.id === currentNodeId">切换</n-button>
                <n-button class="table-btn" size="tiny" @click="testNode(n)" :disabled="n.id !== currentNodeId">测试</n-button>
                <n-button class="table-btn" size="tiny" @click="openEdit(n)">编辑</n-button>
                <n-button class="table-btn" size="tiny" type="error" @click="confirmDelete(n)">删除</n-button>
              </div>
            </td>
          </tr>
        </tbody>
      </n-table>
    </n-card>

    <!-- 添加/编辑节点 -->
    <n-modal v-model:show="showModal" preset="card" :title="editingId ? '编辑节点' : '添加节点'" style="width: 540px;" :mask-closable="false">
      <n-alert type="info" :show-icon="false" style="margin-bottom: 14px;">
        仅支持 <strong>VMess + WebSocket + TLS</strong> 协议。地址必须是<strong>域名</strong>（不能是 IP，因为 TLS 需要 SNI 验证）。
      </n-alert>
      <n-form label-placement="top" :show-feedback="false">
        <n-form-item label="名称 *">
          <n-input v-model:value="form.name" placeholder="如：HK-VPS" />
        </n-form-item>
        <n-grid :cols="3" :x-gap="12">
          <n-gi :span="2">
            <n-form-item label="地址（域名）*">
              <n-input v-model:value="form.address" placeholder="如：vps.example.com" />
            </n-form-item>
          </n-gi>
          <n-gi>
            <n-form-item label="端口 *">
              <n-input-number v-model:value="form.port" :min="1" :max="65535" placeholder="443" style="width: 100%;" />
            </n-form-item>
          </n-gi>
        </n-grid>
        <n-form-item label="UUID *">
          <n-input v-model:value="form.uuid" placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx" />
        </n-form-item>
        <n-grid :cols="2" :x-gap="12">
          <n-gi>
            <n-form-item label="AlterID">
              <n-input-number v-model:value="form.alter_id" :min="0" style="width: 100%;" />
            </n-form-item>
          </n-gi>
          <n-gi>
            <n-form-item label="加密方式">
              <n-select v-model:value="form.security" :options="securityOpts" />
            </n-form-item>
          </n-gi>
        </n-grid>
        <n-form-item label="WS 路径 *">
          <n-input v-model:value="form.ws_path" placeholder="/a1b2c3d4e5f60789" />
        </n-form-item>
        <div class="text-muted text-xs" style="margin: -6px 0 12px;">必须以 / 开头</div>
        <n-form-item label="SNI / Host">
          <n-input v-model:value="form.host" :placeholder="'留空则使用地址：' + (form.address || 'example.com')" />
        </n-form-item>
        <div class="text-muted text-xs" style="margin: -6px 0 0;">通常等于地址，特殊场景可独立设置</div>
      </n-form>
      <template #footer>
        <div style="display: flex; gap: 8px; justify-content: flex-end;">
          <n-button @click="closeModal">取消</n-button>
          <n-button type="primary" :loading="saving" @click="saveNode">保存</n-button>
        </div>
      </template>
    </n-modal>

    <!-- 测试结果 -->
    <n-modal v-model:show="showTest" preset="card" title="节点测试结果" style="width: 400px;">
      <n-alert v-if="testResult?.ok" type="success" :show-icon="false">
        ✓ 连接成功，延迟：<strong>{{ testResult.latency }}ms</strong>
      </n-alert>
      <n-alert v-else-if="testResult" type="error" :show-icon="false">
        ✗ 测试失败：{{ testResult.error }}
      </n-alert>
      <template #footer>
        <div style="display: flex; justify-content: flex-end;">
          <n-button type="primary" @click="showTest = false">关闭</n-button>
        </div>
      </template>
    </n-modal>
  </div>
</template>

<script setup>
import { ref, onMounted, onUnmounted, computed } from 'vue'
import { useDialog, useMessage } from 'naive-ui'
import { api } from '../api'

const dialog = useDialog()
const message = useMessage()

// ---------- 系统状态 / 健康检查 ----------
const status = ref({})
const error = ref('')
const success = ref('')
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

// ---------- 节点管理 ----------
const nodes = ref([])
const currentNodeId = ref('')
const showModal = ref(false)
const editingId = ref(null)
const saving = ref(false)
const testResult = ref(null)
const showTest = ref(false)

const securityOpts = [
  { label: 'auto（推荐）', value: 'auto' },
  { label: 'aes-128-gcm', value: 'aes-128-gcm' },
  { label: 'chacha20-poly1305', value: 'chacha20-poly1305' },
  { label: 'none', value: 'none' },
  { label: 'zero', value: 'zero' }
]

const blankForm = () => ({
  name: '',
  address: '',
  port: 443,
  uuid: '',
  alter_id: 0,
  security: 'auto',
  ws_path: '',
  host: '',
})

const form = ref(blankForm())

async function loadNodes() {
  try {
    const r = await api.listNodes()
    nodes.value = r.nodes || []
    currentNodeId.value = r.current_node_id || ''
  } catch (e) {
    error.value = '加载节点失败：' + e.message
  }
}

function openCreate() {
  editingId.value = null
  form.value = blankForm()
  error.value = ''
  showModal.value = true
}

function openEdit(n) {
  editingId.value = n.id
  form.value = {
    name: n.name,
    address: n.address,
    port: n.port,
    uuid: n.uuid,
    alter_id: n.alter_id || 0,
    security: n.security || 'auto',
    ws_path: n.ws_path || '/',
    host: n.host || '',
  }
  error.value = ''
  showModal.value = true
}

function closeModal() {
  showModal.value = false
  editingId.value = null
  form.value = blankForm()
}

async function saveNode() {
  if (!form.value.name.trim()) { error.value = '名称不能为空'; return }
  if (!form.value.address.trim()) { error.value = '地址不能为空'; return }
  if (/^\d+\.\d+\.\d+\.\d+$/.test(form.value.address) || form.value.address.includes(':')) {
    error.value = '地址必须是域名（VMess+WS+TLS 协议不能用 IP）'
    return
  }
  if (!form.value.uuid.trim()) { error.value = 'UUID 不能为空'; return }
  if (!form.value.ws_path.trim() || !form.value.ws_path.startsWith('/')) {
    error.value = 'WS 路径必须以 / 开头'
    return
  }

  saving.value = true
  error.value = ''
  try {
    if (editingId.value) {
      await api.updateNode(editingId.value, form.value)
      success.value = '节点已更新'
    } else {
      await api.createNode(form.value)
      success.value = '节点已添加'
    }
    closeModal()
    await loadNodes()
    setTimeout(() => { success.value = '' }, 3000)
  } catch (e) {
    error.value = '保存失败：' + e.message
  } finally {
    saving.value = false
  }
}

async function switchTo(n) {
  try {
    await api.switchNode(n.id)
    success.value = `已切换到节点 ${n.name}`
    await loadNodes()
    setTimeout(() => { success.value = '' }, 3000)
  } catch (e) {
    error.value = '切换失败：' + e.message
  }
}

async function testNode(n) {
  try {
    const r = await api.testNode(n.id)
    testResult.value = r
    showTest.value = true
  } catch (e) {
    error.value = '测试失败：' + e.message
  }
}

function confirmDelete(n) {
  dialog.warning({
    title: '删除节点',
    content: `确定删除节点 "${n.name}"？`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await api.deleteNode(n.id)
        success.value = '节点已删除'
        await loadNodes()
        setTimeout(() => { success.value = '' }, 3000)
      } catch (e) {
        error.value = '删除失败：' + e.message
      }
    }
  })
}

async function moveUp(idx) {
  if (idx === 0) return
  const arr = [...nodes.value]
  ;[arr[idx], arr[idx - 1]] = [arr[idx - 1], arr[idx]]
  await reorder(arr)
}

async function moveDown(idx) {
  if (idx === nodes.value.length - 1) return
  const arr = [...nodes.value]
  ;[arr[idx], arr[idx + 1]] = [arr[idx + 1], arr[idx]]
  await reorder(arr)
}

async function reorder(arr) {
  try {
    await api.reorderNodes(arr.map(n => n.id))
    await loadNodes()
  } catch (e) {
    error.value = '排序失败：' + e.message
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
  await loadNodes()
})

onUnmounted(() => {
  if (sse) sse.close()
  if (speedSse) speedSse.close()
})
</script>

<style scoped>
.is-current {
  background: rgba(77, 208, 225, 0.08);
}
</style>
