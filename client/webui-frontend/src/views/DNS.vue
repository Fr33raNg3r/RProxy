<template>
  <div>
    <div class="main-header">
      <h2>DNS 规则</h2>
      <button class="btn btn-primary" @click="save" :disabled="saving">
        {{ saving ? '保存中...' : '保存全部' }}
      </button>
    </div>

    <div v-if="error" class="alert alert-error">{{ error }}</div>
    <div v-if="success" class="alert alert-success">{{ success }}</div>

    <div class="card">
      <div class="card-title">白名单 — 强制走国内 DNS（直连）</div>
      <div class="text-muted text-sm mb-2">
        每行一个域名，支持普通域名（自动匹配子域名）。例如：
        <code>example.com</code> 会匹配 <code>example.com</code>、<code>www.example.com</code> 等
      </div>
      <textarea v-model="rules.whitelist" class="form-control" placeholder="example.com&#10;intranet.local"></textarea>
    </div>

    <div class="card">
      <div class="card-title">黑名单 — 强制走 DoH（代理）</div>
      <div class="text-muted text-sm mb-2">
        每行一个域名。即使该域名属于国内列表，也会被强制走代理
      </div>
      <textarea v-model="rules.blacklist" class="form-control" placeholder="some-blocked-cn-site.com"></textarea>
    </div>

    <div class="card">
      <div class="card-title">静态 hosts — 直接返回固定 IP</div>
      <div class="text-muted text-sm mb-2">
        每行一条记录，格式：<code>域名 IP</code>。例如：
        <code>nas.local 192.168.1.10</code>
      </div>
      <textarea v-model="rules.hosts" class="form-control" placeholder="my-router.local 192.168.1.1&#10;nas.local 192.168.1.100"></textarea>
    </div>

    <div class="alert alert-info">
      保存后将自动重启 mosdns 服务以使规则生效。
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { api } from '../api'

const rules = ref({ whitelist: '', blacklist: '', hosts: '' })
const error = ref('')
const success = ref('')
const saving = ref(false)

async function load() {
  error.value = ''
  try {
    const data = await api.getDNSRules()
    rules.value = {
      whitelist: data.whitelist || '',
      blacklist: data.blacklist || '',
      hosts: data.hosts || ''
    }
  } catch (e) {
    error.value = e.message
  }
}

async function save() {
  saving.value = true
  error.value = ''
  try {
    await api.updateDNSRules(rules.value)
    success.value = '已保存，mosdns 已重启'
    setTimeout(() => success.value = '', 3000)
  } catch (e) {
    error.value = e.message
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>
