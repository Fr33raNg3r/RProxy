<template>
  <div>
    <div class="page-header">
      <h2>DDNS</h2>
      <n-button type="primary" @click="addRecord">+ 添加记录</n-button>
    </div>

    <n-alert v-if="error" type="error" closable style="margin-bottom: 12px;" @close="error = ''">{{ error }}</n-alert>
    <n-alert v-if="success" type="success" closable style="margin-bottom: 12px;" @close="success = ''">{{ success }}</n-alert>

    <n-card size="medium" style="margin-bottom: 16px;">
      <div class="text-muted text-sm" style="margin-bottom: 14px;">
        每 5 分钟自动查询公网 IP（IPv4），发生变化时更新下方各条 A 记录。支持 Cloudflare、DNSPod、NameSilo。
      </div>
      <div style="display: flex; align-items: center; gap: 24px; flex-wrap: wrap;">
        <n-switch v-model:value="cfg.enabled" />
        <span>{{ cfg.enabled ? '已启用' : '已禁用' }}</span>
        <span class="text-muted">当前公网 IP：<span class="text-mono">{{ status.wan_ip || '未知' }}</span></span>
        <span v-if="status.error" style="color: #f87171;">{{ status.error }}</span>
      </div>
    </n-card>

    <n-card title="记录列表" size="medium">
      <div v-if="!cfg.records.length" class="text-muted">暂无记录，点击右上角"添加记录"。</div>
      <div v-for="(r, i) in cfg.records" :key="r.id || i" class="ddns-row">
        <n-select v-model:value="r.provider" :options="providers" style="width: 140px;" />
        <n-input v-model:value="r.sub" placeholder="子域名，根域名留空" style="width: 150px;" />
        <n-input v-model:value="r.domain" placeholder="主域名 example.com" style="width: 190px;" />
        <n-input v-if="r.provider === 'dnspod'" v-model:value="r.auth_id" placeholder="API ID" style="width: 110px;" />
        <n-input v-model:value="r.secret" type="password" show-password-on="click"
                 :placeholder="secretHint(r.provider)" style="width: 230px;" />
        <n-switch v-model:value="r.enabled" />
        <n-button text type="error" @click="removeRecord(i)">删除</n-button>
        <div v-if="recStatus(r)" class="text-sm ddns-status" :style="{ color: recStatus(r).ok ? '#4ade80' : '#f87171' }">
          {{ recStatus(r).ok ? '已更新为 ' + recStatus(r).ip : '失败：' + recStatus(r).error }}
          <span class="text-muted">（{{ fmtTime(recStatus(r).time) }}）</span>
        </div>
      </div>
      <div style="display: flex; gap: 12px; margin-top: 16px;">
        <n-button type="primary" :loading="saving" @click="save">保存</n-button>
        <n-button :loading="running" @click="runNow">立即更新</n-button>
      </div>
    </n-card>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { api } from '../api'

const providers = [
  { label: 'Cloudflare', value: 'cloudflare' },
  { label: 'DNSPod', value: 'dnspod' },
  { label: 'NameSilo', value: 'namesilo' }
]

const cfg = ref({ enabled: false, records: [] })
const status = ref({ wan_ip: '', error: '', records: {} })
const error = ref('')
const success = ref('')
const saving = ref(false)
const running = ref(false)

function secretHint(p) {
  if (p === 'cloudflare') return 'API Token（需 DNS 编辑权限）'
  if (p === 'dnspod') return 'API Token'
  return 'API Key'
}

function recStatus(r) {
  return r.id ? status.value.records?.[r.id] : null
}

function fmtTime(t) {
  return t ? new Date(t).toLocaleString() : ''
}

function addRecord() {
  cfg.value.records.push({ id: '', provider: 'cloudflare', domain: '', sub: '', auth_id: '', secret: '', enabled: true })
}

function removeRecord(i) {
  cfg.value.records.splice(i, 1)
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

async function save() {
  saving.value = true
  error.value = ''
  try {
    const r = await api.saveDDNS(cfg.value)
    cfg.value = r.config
    success.value = '已保存，正在后台更新'
    setTimeout(() => { success.value = ''; load() }, 3000)
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
    success.value = '已执行一轮更新'
    setTimeout(() => success.value = '', 3000)
  } catch (e) {
    error.value = '执行失败：' + e.message
  } finally {
    running.value = false
  }
}

onMounted(load)
</script>

<style scoped>
.ddns-row {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 10px;
  padding: 10px 0;
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
}
.ddns-status {
  width: 100%;
}
</style>
