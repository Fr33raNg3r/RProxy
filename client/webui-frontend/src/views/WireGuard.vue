<template>
  <div>
    <div class="page-header">
      <h2>WireGuard 入站</h2>
      <n-button type="primary"
                :disabled="!wgEnabled"
                :title="wgEnabled ? '' : '请先在下方启用 WireGuard 服务'"
                @click="openCreate">
        + 添加 Peer
      </n-button>
    </div>

    <n-alert v-if="error" type="error" closable style="margin-bottom: 12px;" @close="error = ''">{{ error }}</n-alert>
    <n-alert v-if="success" type="success" closable style="margin-bottom: 12px;" @close="success = ''">{{ success }}</n-alert>

    <div class="card-grid">
      <n-card size="medium">
        <template #header>
          <div style="display: flex; align-items: center; gap: 10px;">
            <span>WireGuard 入站服务</span>
            <n-tag :type="wgEnabled ? 'success' : 'default'" size="small" round>
              {{ wgEnabled ? '已启用' : '已禁用' }}
            </n-tag>
          </div>
        </template>
        <div class="text-muted text-sm" style="margin-bottom: 14px;">
          启用后可在下方添加 peer，让手机/电脑通过 WireGuard 接入旁路由。禁用时所有 peer 配置仍会保留，再次启用立即可用。
        </div>
        <div style="display: flex; align-items: flex-end; gap: 24px; flex-wrap: wrap;">
          <n-button v-if="!wgEnabled" type="primary" :loading="wgToggling" @click="enableWG">
            启用 WireGuard 服务
          </n-button>
          <n-button v-else type="error" :loading="wgToggling" @click="disableWG">
            禁用 WireGuard 服务
          </n-button>
          <div>
            <div class="text-muted text-xs" style="margin-bottom: 6px;">监听端口</div>
            <n-input-group>
              <n-input-number v-model:value="wgPort" :min="1" :max="65535" style="width: 140px;" />
              <n-button :loading="savingPort" @click="savePort">保存</n-button>
            </n-input-group>
          </div>
        </div>
      </n-card>

      <n-card size="medium">
        <template #header>
          <div style="display: flex; align-items: center; gap: 10px;">
            <span>服务端信息</span>
            <n-tag :type="wgActive ? 'success' : 'default'" size="small" round>
              {{ wgActive ? '运行中' : '未启用' }}
            </n-tag>
          </div>
        </template>
        <table class="kv-table">
          <tbody>
            <tr><td>子网</td><td class="text-mono">{{ subnet }}</td></tr>
            <tr><td>服务端公钥</td><td class="text-mono text-sm break-all">{{ serverPublicKey }}</td></tr>
          </tbody>
        </table>
      </n-card>

      <n-card title="客户端连接 Endpoint" size="medium">
        <div class="text-muted text-sm" style="margin-bottom: 12px;">
          生成 peer 配置文件时，Endpoint 字段使用此地址。<br>
          填写你的<strong>公网 IP 或 DDNS 域名</strong>（不要带端口号），手机/电脑将通过此地址连接到旁路由。<br>
          若留空，每次生成配置会自动探测公网 IP。
        </div>
        <n-input-group>
          <n-input v-model:value="endpoint" placeholder="如 myhome.ddns.net 或 1.2.3.4" />
          <n-button type="primary" :loading="savingEndpoint" :disabled="!wgEnabled" @click="saveEndpoint">保存</n-button>
        </n-input-group>
        <div v-if="endpointMsg" class="text-sm" style="margin-top: 8px; color: #4ade80;">{{ endpointMsg }}</div>
      </n-card>

      <n-card size="medium" class="card-full">
        <template #header>
          <div style="display: flex; align-items: center; justify-content: space-between; width: 100%;">
            <span>Peer 列表</span>
            <span class="text-muted text-sm">实时网速（KB/s）</span>
          </div>
        </template>
        <n-empty v-if="peers.length === 0" description="尚未添加任何 peer" />
        <n-table v-else :bordered="false" :single-line="false" size="small">
          <thead>
            <tr>
              <th>名称</th>
              <th>分配 IP</th>
              <th style="width: 120px;">↑ 上传</th>
              <th style="width: 120px;">↓ 下载</th>
              <th style="width: 240px;">操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="p in peers" :key="p.id">
              <td><strong>{{ p.name }}</strong></td>
              <td class="text-mono text-sm">{{ p.address }}</td>
              <td class="text-mono">{{ getSpeed(p.public_key, 'tx') }} <span class="text-muted text-xs">KB/s</span></td>
              <td class="text-mono">{{ getSpeed(p.public_key, 'rx') }} <span class="text-muted text-xs">KB/s</span></td>
              <td>
                <div class="row-actions">
                  <n-button class="table-btn" size="tiny" @click="showQR(p)">配置 / 二维码</n-button>
                  <n-button class="table-btn" size="tiny" type="error" @click="confirmDelete(p)">删除</n-button>
                </div>
              </td>
            </tr>
          </tbody>
        </n-table>
      </n-card>
    </div>

    <!-- 添加 Peer -->
    <n-modal v-model:show="showCreate" preset="card" title="添加 WireGuard Peer" style="width: 460px;">
      <n-form label-placement="top" :show-feedback="false">
        <n-form-item label="名称 *">
          <n-input v-model:value="newName" placeholder="如：我的手机" autofocus />
        </n-form-item>
      </n-form>
      <n-alert type="info" :show-icon="false" style="margin-top: 12px;">
        密钥对将由服务端自动生成。客户端配置文件可在创建后通过列表中的「配置 / 二维码」按钮获取。
      </n-alert>
      <template #footer>
        <div style="display: flex; gap: 8px; justify-content: flex-end;">
          <n-button @click="showCreate = false">取消</n-button>
          <n-button type="primary" :loading="creating" @click="createPeer">创建</n-button>
        </div>
      </template>
    </n-modal>

    <!-- QR -->
    <n-modal v-model:show="qrVisible" preset="card" :title="(qrPeer?.name || '') + ' - 客户端配置'" style="width: 520px;" @after-leave="onQrClose">
      <div class="text-muted text-sm" style="margin-bottom: 12px;">
        Endpoint: <code>{{ endpoint || '(自动探测)' }}</code>
        <span v-if="!endpoint" style="color: #fbbf24;">— 建议先在主页面设置一个稳定的 Endpoint</span>
      </div>
      <div style="text-align: center; margin: 16px 0;">
        <img :src="qrUrl" alt="QR Code" style="max-width: 320px; border-radius: 6px; background: white; padding: 12px;" />
      </div>
      <div class="text-muted text-sm" style="text-align: center; margin-bottom: 12px;">扫码导入到 WireGuard 客户端</div>
      <div class="text-muted text-sm" style="margin-bottom: 6px;">配置文件内容：</div>
      <n-input
        type="textarea"
        :value="cfgText"
        readonly
        :autosize="{ minRows: 8, maxRows: 14 }"
        :placeholder="cfgLoading ? '加载中…' : (cfgError || '')"
        style="font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 12px;"
      />
      <template #footer>
        <div style="display: flex; gap: 8px; justify-content: flex-end;">
          <n-button :disabled="!cfgText" @click="copyConfig">复制配置文件内容</n-button>
          <n-button type="primary" @click="qrVisible = false">关闭</n-button>
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

const peers = ref([])
const wgEnabled = ref(false)
const wgActive = ref(false)
const wgToggling = ref(false)
const wgPort = ref(51820)
const savingPort = ref(false)
const subnet = ref('')
const serverPublicKey = ref('')
const error = ref('')
const success = ref('')

const showCreate = ref(false)
const newName = ref('')
const creating = ref(false)

const qrPeer = ref(null)
const qrVisible = ref(false)
const endpoint = ref('')
const endpointMsg = ref('')
const savingEndpoint = ref(false)

const speeds = ref({})
let sse = null

const qrUrl = computed(() => qrPeer.value ? api.peerQRCodeURL(qrPeer.value.id, endpoint.value) : '')
const cfgText = ref('')
const cfgLoading = ref(false)
const cfgError = ref('')

async function loadConfigText(peer) {
  cfgText.value = ''
  cfgError.value = ''
  cfgLoading.value = true
  try {
    const url = api.peerConfigURL(peer.id, endpoint.value)
    const resp = await fetch(url, { credentials: 'same-origin' })
    if (resp.status === 401) {
      location.href = '/login'
      return
    }
    if (!resp.ok) throw new Error(await resp.text() || '加载失败')
    cfgText.value = await resp.text()
  } catch (e) {
    cfgError.value = e.message || '加载失败'
  } finally {
    cfgLoading.value = false
  }
}

async function copyConfig() {
  if (!cfgText.value) return
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(cfgText.value)
    } else {
      const ta = document.createElement('textarea')
      ta.value = cfgText.value
      ta.style.position = 'fixed'
      ta.style.opacity = '0'
      document.body.appendChild(ta)
      ta.select()
      document.execCommand('copy')
      document.body.removeChild(ta)
    }
    message.success('已复制到剪贴板')
  } catch (e) {
    message.error('复制失败：' + (e.message || e))
  }
}

function onQrClose() {
  qrPeer.value = null
  cfgText.value = ''
  cfgError.value = ''
}

function getSpeed(pubKey, dir) {
  const s = speeds.value[pubKey]
  if (!s) return '0'
  return dir === 'rx' ? (s.tx_kbps || 0) : (s.rx_kbps || 0)
}

async function load() {
  error.value = ''
  try {
    const data = await api.listPeers()
    peers.value = data.peers || []
    wgEnabled.value = !!data.wg_enabled
    wgActive.value = data.wg_active
    wgPort.value = data.wg_listen_port
    subnet.value = data.wg_subnet
    serverPublicKey.value = data.server_public_key
    if (data.wg_endpoint !== undefined && data.wg_endpoint !== null) {
      endpoint.value = data.wg_endpoint
    }
  } catch (e) {
    error.value = e.message
  }
}

async function enableWG() {
  wgToggling.value = true
  error.value = ''
  try {
    const r = await api.enableWG()
    success.value = r.message || 'WireGuard 服务已启动'
    wgEnabled.value = true
    setTimeout(() => success.value = '', 3000)
    await load()
  } catch (e) {
    error.value = '启动失败：' + e.message
  } finally {
    wgToggling.value = false
  }
}

function disableWG() {
  dialog.warning({
    title: '禁用 WireGuard',
    content: '确定禁用 WireGuard 入站服务？所有 peer 配置会保留，再次启用立即可用。',
    positiveText: '禁用',
    negativeText: '取消',
    onPositiveClick: async () => {
      wgToggling.value = true
      error.value = ''
      try {
        const r = await api.disableWG()
        success.value = r.message || 'WireGuard 服务已停止'
        wgEnabled.value = false
        setTimeout(() => success.value = '', 3000)
      } catch (e) {
        error.value = '停止失败：' + e.message
      } finally {
        wgToggling.value = false
      }
    }
  })
}

async function savePort() {
  savingPort.value = true
  error.value = ''
  try {
    await api.updateSettings({ wg_listen_port: wgPort.value })
    success.value = '监听端口已保存'
    setTimeout(() => success.value = '', 3000)
  } catch (e) {
    error.value = e.message
  } finally {
    savingPort.value = false
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

function confirmDelete(p) {
  dialog.warning({
    title: '删除 Peer',
    content: `确定删除 peer "${p.name}"？`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await api.deletePeer(p.id)
        success.value = '已删除'
        setTimeout(() => success.value = '', 2500)
        await load()
      } catch (e) {
        error.value = e.message
      }
    }
  })
}

function showQR(p) {
  qrPeer.value = p
  qrVisible.value = true
  loadConfigText(p)
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
