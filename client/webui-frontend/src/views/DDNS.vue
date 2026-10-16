<template>
  <div>
    <div class="page-header">
      <h2>DDNS</h2>
      <div style="display: flex; gap: 12px;">
        <n-button :loading="running" @click="runNow"><template #icon><SvgIcon name="refresh" /></template>立即更新</n-button>
        <n-button type="primary" :loading="saving" @click="save"><template #icon><SvgIcon name="save" /></template>保存</n-button>
      </div>
    </div>

    <n-alert v-if="error" type="error" closable style="margin-bottom: 12px;" @close="error = ''">{{ error }}</n-alert>
    <n-alert v-if="success" type="success" closable style="margin-bottom: 12px;" @close="success = ''">{{ success }}</n-alert>

    <n-card size="medium" style="margin-bottom: 16px;">
      <div class="text-muted text-sm" style="margin-bottom: 10px;">
        哪家填写完整（域名、密钥）并保存，哪家就生效；留空则不生效。每 5 分钟自动查询公网 IP（IPv4），变化时更新对应的 A 记录。
      </div>
      <span class="text-muted">当前公网 IP：<span class="text-mono">{{ status.wan_ip || '未知' }}</span></span>
      <span v-if="status.error" style="color: #f87171; margin-left: 16px;">{{ status.error }}</span>
    </n-card>

    <div class="card-grid">
      <n-card v-for="r in cfg.records" :key="r.id" size="medium">
        <template #header>
          <div style="display: flex; align-items: center; gap: 10px;">
            <span>{{ labels[r.provider] }}</span>
            <n-tag :type="statusOf(r).type" size="small" round>{{ statusOf(r).text }}</n-tag>
          </div>
        </template>
        <div style="display: flex; flex-direction: column; gap: 10px;">
          <n-input v-model:value="r.sub" placeholder="子域名，如 home（根域名留空）" />
          <n-input v-model:value="r.domain" placeholder="主域名，如 example.com" />
          <n-input v-if="r.provider === 'dnspod'" v-model:value="r.auth_id" placeholder="API ID" />
          <n-input v-if="r.provider === 'cloudflare'" v-model:value="r.email" placeholder="登录邮箱（仅 Global API Key 需要；用 API Token 请留空）" />
          <n-input v-model:value="r.secret" type="password" show-password-on="click" :placeholder="secretHint(r.provider)" />
          <div v-if="r.secret === '******' && r.secret_hint" class="text-muted text-sm">
            已保存，密钥末 4 位：<span class="text-mono">{{ r.secret_hint }}</span>
          </div>
        </div>
        <div v-if="statusOf(r).detail" class="text-sm"
             :style="{ marginTop: '10px', color: statusOf(r).type === 'error' ? '#f87171' : '' }">
          {{ statusOf(r).detail }}
        </div>
      </n-card>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { api } from '../api'

const labels = { cloudflare: 'Cloudflare', dnspod: 'DNSPod', namesilo: 'NameSilo' }

const cfg = ref({ records: [] })
const status = ref({ wan_ip: '', error: '', checked: '', records: {} })
const error = ref('')
const success = ref('')
const saving = ref(false)
const running = ref(false)

function secretHint(p) {
  if (p === 'cloudflare') return 'API Token（需 Zone:读取 + DNS:编辑权限），或 Global API Key'
  if (p === 'dnspod') return 'API Token'
  return 'API Key'
}

// 与后端 Complete() 一致：域名、密钥必填，DNSPod 还要 API ID
function isComplete(r) {
  return !!(r.domain?.trim() && r.secret?.trim() && (r.provider !== 'dnspod' || r.auth_id?.trim()))
}

function fmtTime(t) {
  return t ? new Date(t).toLocaleString() : ''
}

function statusOf(r) {
  if (!isComplete(r)) return { text: '未配置', type: 'default' }
  const s = status.value.records?.[r.id]
  if (!s) return { text: '等待更新', type: 'info' }
  if (s.ok) return { text: '✓ 已更新', type: 'success', detail: `${s.ip}（${fmtTime(s.time)}）` }
  return { text: '异常', type: 'error', detail: s.error }
}

async function load() {
  try {
    const r = await api.getDDNS()
    cfg.value = r.config
    status.value = r.status
  } catch (e) {
    error.value = '加载失败：' + e.message
  }
}

// 保存后后台会立刻跑一轮，这里轮询状态（最多 30 秒），状态连续两次没变化就停
async function pollStatus(checkedBefore) {
  let prev = ''
  let stable = 0
  for (let i = 0; i < 15; i++) {
    await new Promise(r => setTimeout(r, 2000))
    try {
      const r = await api.getDDNS()
      status.value = r.status
      if (r.status.checked !== checkedBefore) {
        const cur = JSON.stringify(r.status)
        stable = cur === prev ? stable + 1 : 0
        prev = cur
        if (stable >= 2) return
      }
    } catch (e) {
      return
    }
  }
}

async function save() {
  saving.value = true
  error.value = ''
  try {
    const checkedBefore = status.value.checked
    const r = await api.saveDDNS(cfg.value)
    cfg.value = r.config
    success.value = '已保存'
    setTimeout(() => success.value = '', 3000)
    if (cfg.value.records.some(isComplete)) await pollStatus(checkedBefore)
  } catch (e) {
    error.value = '保存失败：' + e.message
  } finally {
    saving.value = false
  }
}

async function runNow() {
  running.value = true
  error.value = ''
  try {
    const r = await api.runDDNS()
    status.value = r.status
  } catch (e) {
    error.value = '执行失败：' + e.message
  } finally {
    running.value = false
  }
}

onMounted(load)
</script>
