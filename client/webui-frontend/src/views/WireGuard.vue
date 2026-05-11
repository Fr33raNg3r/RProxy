<template>
  <div>
    <div class="main-header">
      <h2>WireGuard 入站</h2>
      <button class="btn btn-primary"
              @click="openCreate"
              :disabled="!wgEnabled"
              :title="wgEnabled ? '' : '请先到【设置】启用 WireGuard 服务'">
        + 添加 Peer
      </button>
    </div>

    <div v-if="error" class="alert alert-error">{{ error }}</div>
    <div v-if="success" class="alert alert-success">{{ success }}</div>

    <div v-if="!wgEnabled" class="card" style="border: 1px solid rgba(251, 191, 36, 0.4); background: rgba(251, 191, 36, 0.06);">
      <div class="card-title">⚠️ WireGuard 入站服务未启用</div>
      <div class="text-muted mb-3">
        要使用 WireGuard 让手机/电脑接入旁路由，请先到【设置】页启用 WireGuard 入站服务。<br>
        启用后才能在此页面添加 peer。
      </div>
      <router-link to="/settings" class="btn btn-primary">前往设置启用</router-link>
    </div>

    <div class="card">
      <div class="card-title">
        <span>服务端信息</span>
        <span class="badge" :class="wgActive ? 'badge-success' : 'badge-muted'">
          {{ wgActive ? '运行中' : '未启用' }}
        </span>
      </div>
      <table class="table">
        <tbody>
          <tr><td style="width: 200px;">监听端口</td><td class="text-mono">{{ listenPort }}</td></tr>
          <tr><td>子网</td><td class="text-mono">{{ subnet }}</td></tr>
          <tr>
            <td>服务端公钥</td>
            <td class="text-mono text-sm" style="word-break: break-all;">{{ serverPublicKey }}</td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- 客户端连接 Endpoint 设置 -->
    <div class="card">
      <div class="card-title">客户端连接 Endpoint</div>
      <div class="text-muted text-sm mb-3">
        生成 peer 配置文件时，Endpoint 字段使用此地址。<br>
        填写你的<strong>公网 IP 或 DDNS 域名</strong>（不要带端口号），手机/电脑将通过此地址连接到旁路由。<br>
        若留空，每次生成配置会自动探测公网 IP。
      </div>
      <div class="flex gap-2" style="align-items: center;">
        <input v-model="endpoint"
               class="form-control"
               placeholder="如 myhome.ddns.net 或 1.2.3.4"
               style="flex: 1;" />
        <button class="btn btn-primary"
                @click="saveEndpoint"
                :disabled="savingEndpoint || !wgEnabled">
          {{ savingEndpoint ? '保存中...' : '保存' }}
        </button>
      </div>
      <div v-if="endpointMsg" class="text-success text-sm mt-2">{{ endpointMsg }}</div>
    </div>

    <div class="card">
      <div class="card-title">
        <span>Peer 列表</span>
        <span class="text-muted text-sm">实时网速（KB/s）</span>
      </div>
      <div v-if="peers.length === 0" class="text-muted" style="text-align:center; padding: 30px 0;">
        尚未添加任何 peer
      </div>
      <table v-else class="table">
        <thead>
          <tr>
            <th>名称</th>
            <th>分配 IP</th>
            <th style="width: 120px;">↑ 上传</th>
            <th style="width: 120px;">↓ 下载</th>
            <th style="width: 220px;">操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="p in peers" :key="p.id">
            <td><strong>{{ p.name }}</strong></td>
            <td class="text-mono text-sm">{{ p.address }}</td>
            <td class="text-mono">{{ getSpeed(p.public_key, 'tx') }} <span class="text-muted text-xs">KB/s</span></td>
            <td class="text-mono">{{ getSpeed(p.public_key, 'rx') }} <span class="text-muted text-xs">KB/s</span></td>
            <td>
              <div class="flex gap-2">
                <button class="btn btn-sm" @click="showQR(p)">配置 / 二维码</button>
                <button class="btn btn-sm btn-danger" @click="confirmDelete(p)">删除</button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- 添加 Peer 模态框 -->
    <div v-if="showCreate" class="modal-mask" @click.self="showCreate = false">
      <div class="modal-box">
        <h3>添加 WireGuard Peer</h3>
        <div class="form-group">
          <label>名称 *</label>
          <input v-model="newName" class="form-control" placeholder="如：我的手机" autofocus />
        </div>
        <div class="alert alert-info text-sm">
          密钥对将由服务端自动生成。客户端配置文件可在创建后通过列表中的「配置 / 二维码」按钮获取。
        </div>
        <div class="modal-actions">
          <button class="btn" @click="showCreate = false">取消</button>
          <button class="btn btn-primary" @click="createPeer" :disabled="creating">
            {{ creating ? '创建中...' : '创建' }}
          </button>
        </div>
      </div>
    </div>

    <!-- 二维码模态框 -->
    <div v-if="qrPeer" class="modal-mask" @click.self="closeQR">
      <div class="modal-box">
        <h3>{{ qrPeer.name }} - 客户端配置</h3>
        <div class="text-muted text-sm mb-3">
          Endpoint: <code>{{ endpoint || '(自动探测)' }}</code>
          <span v-if="!endpoint" class="text-warning">— 建议先在主页面设置一个稳定的 Endpoint</span>
        </div>
        <div style="text-align: center; margin: 16px 0;">
          <img :src="qrUrl" alt="QR Code" style="max-width: 320px; border: 1px solid #e5e7eb; border-radius: 6px;" />
        </div>
        <div class="text-muted text-sm" style="text-align: center;">
          扫码导入到 WireGuard 客户端
        </div>
        <div class="modal-actions">
          <a class="btn" :href="cfgUrl" download>下载配置文件</a>
          <button class="btn btn-primary" @click="closeQR">关闭</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, onUnmounted, computed } from 'vue'
import { api } from '../api'

const peers = ref([])
const wgEnabled = ref(false)
const wgActive = ref(false)
const listenPort = ref('')
const subnet = ref('')
const serverPublicKey = ref('')
const error = ref('')
const success = ref('')

const showCreate = ref(false)
const newName = ref('')
const creating = ref(false)

const qrPeer = ref(null)
const endpoint = ref('')
const endpointMsg = ref('')
const savingEndpoint = ref(false)

const speeds = ref({})  // { public_key: { rx_kbps, tx_kbps } }
let sse = null

const qrUrl = computed(() => qrPeer.value ? api.peerQRCodeURL(qrPeer.value.id, endpoint.value) : '')
const cfgUrl = computed(() => qrPeer.value ? api.peerConfigURL(qrPeer.value.id, endpoint.value) : '')

function getSpeed(pubKey, dir) {
  const s = speeds.value[pubKey]
  if (!s) return '0'
  // 注意：这里 rx 是服务端接收（即客户端上传），tx 是服务端发送（即客户端下载）
  // 用户界面显示的 ↑ 上传是 客户端→服务端 = 服务端 rx
  // ↓ 下载是 服务端→客户端 = 服务端 tx
  // 实际上 wg show transfer 输出的两个值是： received tx
  // 第一个数是 receive (rx)、第二个是 sent (tx)
  // 故 上传(client → server) = rx；下载(server → client) = tx
  return dir === 'rx' ? (s.tx_kbps || 0) : (s.rx_kbps || 0)
}

async function load() {
  error.value = ''
  try {
    const data = await api.listPeers()
    peers.value = data.peers || []
    wgEnabled.value = !!data.wg_enabled
    wgActive.value = data.wg_active
    listenPort.value = data.wg_listen_port
    subnet.value = data.wg_subnet
    serverPublicKey.value = data.server_public_key
    // 已保存的 endpoint，没有就保持当前值
    if (data.wg_endpoint !== undefined && data.wg_endpoint !== null) {
      endpoint.value = data.wg_endpoint
    }
  } catch (e) {
    error.value = e.message
  }
}

async function saveEndpoint() {
  savingEndpoint.value = true
  endpointMsg.value = ''
  error.value = ''
  try {
    await api.setEndpoint(endpoint.value.trim())
    endpointMsg.value = '已保存。新生成的 peer 配置会使用此 Endpoint。'
    setTimeout(() => endpointMsg.value = '', 4000)
  } catch (e) {
    error.value = e.message
  } finally {
    savingEndpoint.value = false
  }
}

function openCreate() {
  newName.value = ''
  showCreate.value = true
}

async function createPeer() {
  if (!newName.value.trim()) {
    error.value = '请填写名称'
    return
  }
  creating.value = true
  error.value = ''
  try {
    await api.createPeer(newName.value.trim())
    showCreate.value = false
    success.value = '已添加'
    setTimeout(() => success.value = '', 2500)
    await load()
  } catch (e) {
    error.value = e.message
  } finally {
    creating.value = false
  }
}

async function confirmDelete(p) {
  if (!confirm(`确定删除 peer "${p.name}"？`)) return
  try {
    await api.deletePeer(p.id)
    success.value = '已删除'
    setTimeout(() => success.value = '', 2500)
    await load()
  } catch (e) {
    error.value = e.message
  }
}

function showQR(p) {
  qrPeer.value = p
  // 注意：endpoint 已在加载时从后端取（如果用户保存过）
  // 如果还是空，URL 不带 endpoint 参数，后端会自动探测公网 IP
}

function closeQR() {
  qrPeer.value = null
}

function startSpeedSSE() {
  sse = new EventSource('/api/wireguard/speed/stream', { withCredentials: true })
  sse.onmessage = (ev) => {
    try {
      const data = JSON.parse(ev.data)
      const m = {}
      for (const s of (data.speeds || [])) {
        m[s.public_key] = s
      }
      speeds.value = m
    } catch {}
  }
  sse.onerror = () => {
    if (sse) {
      sse.close()
      sse = null
      setTimeout(startSpeedSSE, 5000)
    }
  }
}

onMounted(async () => {
  await load()
  startSpeedSSE()
})

onUnmounted(() => {
  if (sse) sse.close()
})
</script>
