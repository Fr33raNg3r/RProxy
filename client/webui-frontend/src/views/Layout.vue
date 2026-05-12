<template>
  <div class="layout">
    <aside class="sidebar">
      <div class="brand">
        <h1>RProxy</h1>
        <div class="version-line" v-if="version.current">
          v{{ version.current }}
        </div>
        <div class="update-notice" v-if="version.has_update">
          <div class="update-text">⚠ 有新版本 v{{ version.latest }}</div>
          <button class="btn btn-sm btn-warning" @click="onUpgrade" :disabled="upgrading">
            {{ upgrading ? '升级中...' : '立即升级' }}
          </button>
        </div>
      </div>
      <nav>
        <router-link to="/status">状态</router-link>
        <router-link to="/nodes">节点管理</router-link>
        <router-link to="/wireguard">WireGuard</router-link>
        <router-link to="/dns">DNS 规则</router-link>
        <router-link to="/settings">设置</router-link>
        <router-link to="/logs">日志</router-link>
      </nav>
      <div style="padding: 20px; margin-top: 30px;">
        <button class="btn btn-sm" @click="logout" style="width: 100%;">退出登录</button>
      </div>
    </aside>
    <main class="main">
      <router-view />
    </main>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api'

const router = useRouter()
const version = ref({ current: '', latest: '', has_update: false })
const upgrading = ref(false)

async function loadVersion() {
  try {
    const data = await api.getVersion()
    version.value = data
  } catch (e) {
    // 静默失败 - 不影响主功能
  }
}

async function onUpgrade() {
  if (!confirm(`确认升级到 v${version.value.latest}？\n\n升级期间 WebUI 可能短暂不可用（1-2 分钟）。\n升级后请刷新页面。`)) {
    return
  }
  upgrading.value = true
  try {
    const data = await api.triggerUpgrade()
    alert(data.message || '升级已启动，1-2 分钟后刷新页面')
  } catch (e) {
    alert('升级启动失败：' + e.message)
    upgrading.value = false
  }
}

async function logout() {
  try {
    await api.logout()
  } catch {}
  router.push('/login')
}

onMounted(() => {
  loadVersion()
})
</script>
