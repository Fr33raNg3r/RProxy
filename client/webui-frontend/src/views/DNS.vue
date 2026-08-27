<template>
  <div>
    <div class="page-header">
      <h2>DNS 配置</h2>
    </div>

    <n-alert v-if="error" type="error" closable style="margin-bottom: 12px;" @close="error = ''">{{ error }}</n-alert>
    <n-alert v-if="success" type="success" closable style="margin-bottom: 12px;" @close="success = ''">{{ success }}</n-alert>
    <n-alert v-if="warning" type="warning" closable style="margin-bottom: 12px;" @close="warning = ''">{{ warning }}</n-alert>

    <div class="card-stack">
      <n-card title="DNS 上游" size="medium">
        <div class="split-row">
          <div>
            <div class="text-muted text-sm" style="margin-bottom: 8px;">
              国内 DNS — 每行一个 mosdns 上游地址。格式：<code>udp://IP</code> /
              <code>tcp://IP</code>。<strong>这里的 IP 会自动写入 nftables cn_ips（强制直连）。</strong>
            </div>
            <n-input
              v-model:value="upstreamsLocalText"
              type="textarea"
              :autosize="{ minRows: 6, maxRows: 12 }"
              placeholder="udp://223.5.5.5&#10;udp://119.29.29.29"
            />
          </div>
          <div>
            <div class="text-muted text-sm" style="margin-bottom: 8px;">
              国外 DoH — 每行一个 URL。<strong>解析后的 IP 会写入 nftables force_proxy_ips（强制走代理）。</strong>
              域名形式（如 <code>https://dns.google/dns-query</code>）也支持，保存时会自动解析。
            </div>
            <n-input
              v-model:value="upstreamsRemoteText"
              type="textarea"
              :autosize="{ minRows: 6, maxRows: 12 }"
              placeholder="https://1.1.1.1/dns-query&#10;https://8.8.8.8/dns-query"
            />
          </div>
        </div>
        <div class="row-actions" style="margin-top: 14px;">
          <n-button size="small" @click="restoreDefaults" :disabled="savingUpstreams">恢复默认</n-button>
          <n-button size="small" type="primary" :loading="savingUpstreams" @click="saveUpstreams">保存上游</n-button>
        </div>
      </n-card>

      <h3 style="margin: 6px 0 0;">DNS 规则</h3>

      <div class="card-grid">
        <n-card title="白名单 — 强制直连" size="medium">
          <div class="text-muted text-sm" style="margin-bottom: 8px;">
            每行一个<strong>域名或 IP/CIDR</strong>。域名走国内 DNS 直连（自动匹配子域名，如
            <code>example.com</code> 含 <code>www.example.com</code>）；填 IP/CIDR（如
            <code>1.2.3.4</code>、<code>1.2.3.0/24</code>）则直接写入 nftables 强制直连（绕过代理）。
          </div>
          <n-input
            v-model:value="rules.whitelist"
            type="textarea"
            :autosize="{ minRows: 6, maxRows: 12 }"
            placeholder="example.com&#10;intranet.local&#10;1.2.3.4"
          />
          <div class="row-actions" style="margin-top: 14px;">
            <n-button size="small" type="primary" :loading="savingRules" @click="saveRules">保存规则</n-button>
          </div>
        </n-card>

        <n-card title="黑名单 — 强制走代理" size="medium">
          <div class="text-muted text-sm" style="margin-bottom: 8px;">
            每行一个<strong>域名或 IP/CIDR</strong>。即使属于国内列表也强制走代理：域名走 DoH 解析后写入，
            填 IP/CIDR（如 <code>1.2.3.4</code>、<code>1.2.3.0/24</code>）则直接写入 nftables 强制代理。
          </div>
          <n-input
            v-model:value="rules.blacklist"
            type="textarea"
            :autosize="{ minRows: 6, maxRows: 12 }"
            placeholder="some-blocked-cn-site.com&#10;1.2.3.4"
          />
          <div class="row-actions" style="margin-top: 14px;">
            <n-button size="small" type="primary" :loading="savingRules" @click="saveRules">保存规则</n-button>
          </div>
        </n-card>

        <n-card title="静态 hosts — 直接返回固定 IP" size="medium">
          <div class="text-muted text-sm" style="margin-bottom: 8px;">
            每行一条记录，格式：<code>域名 IP</code>。例如：
            <code>nas.local 192.168.1.10</code>
          </div>
          <n-input
            v-model:value="rules.hosts"
            type="textarea"
            :autosize="{ minRows: 6, maxRows: 12 }"
            placeholder="my-router.local 192.168.1.1&#10;nas.local 192.168.1.100"
          />
          <div class="row-actions" style="margin-top: 14px;">
            <n-button size="small" type="primary" :loading="savingRules" @click="saveRules">保存规则</n-button>
          </div>
        </n-card>
      </div>

      <n-alert type="info" :show-icon="false">
        保存上游或规则后会自动重启 mosdns 服务以生效。
      </n-alert>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { api } from '../api'

const rules = ref({ whitelist: '', blacklist: '', hosts: '' })
const savingRules = ref(false)

const upstreamsLocalText = ref('')
const upstreamsRemoteText = ref('')
const savingUpstreams = ref(false)

const error = ref('')
const success = ref('')
const warning = ref('')

function parseLines(text) {
  return text.split('\n').map(s => s.trim()).filter(Boolean)
}

function flashSuccess(msg) {
  success.value = msg
  setTimeout(() => { if (success.value === msg) success.value = '' }, 3000)
}
function flashWarning(msg) {
  warning.value = msg
  setTimeout(() => { if (warning.value === msg) warning.value = '' }, 5000)
}

async function loadRules() {
  try {
    const data = await api.getDNSRules()
    rules.value = {
      whitelist: data.whitelist || '',
      blacklist: data.blacklist || '',
      hosts: data.hosts || ''
    }
  } catch (e) { error.value = e.message }
}

async function loadUpstreams() {
  try {
    const u = await api.getDNSUpstreams()
    upstreamsLocalText.value = (u.local || []).join('\n')
    upstreamsRemoteText.value = (u.remote || []).join('\n')
  } catch (e) { error.value = e.message }
}

async function saveRules() {
  savingRules.value = true
  error.value = ''
  try {
    await api.updateDNSRules(rules.value)
    flashSuccess('DNS 规则已保存')
  } catch (e) {
    error.value = e.message
  } finally {
    savingRules.value = false
  }
}

async function saveUpstreams() {
  savingUpstreams.value = true
  error.value = ''
  warning.value = ''
  try {
    const resp = await api.updateDNSUpstreams({
      local: parseLines(upstreamsLocalText.value),
      remote: parseLines(upstreamsRemoteText.value)
    })
    if (resp.warning) {
      flashWarning(resp.warning)
    } else {
      flashSuccess('DNS 上游已保存，mosdns 已重启')
    }
  } catch (e) {
    error.value = e.message
  } finally {
    savingUpstreams.value = false
  }
}

async function restoreDefaults() {
  try {
    const u = await api.getDNSUpstreamsDefaults()
    upstreamsLocalText.value = (u.local || []).join('\n')
    upstreamsRemoteText.value = (u.remote || []).join('\n')
    flashSuccess('已填入默认值，点击「保存上游」生效')
  } catch (e) { error.value = e.message }
}

onMounted(() => {
  loadRules()
  loadUpstreams()
})
</script>
