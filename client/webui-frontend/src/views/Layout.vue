<template>
  <div class="layout">
    <aside class="sidebar">
      <h1>TProxy</h1>
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
import { useRouter } from 'vue-router'
import { api } from '../api'

const router = useRouter()

async function logout() {
  try {
    await api.logout()
  } catch {}
  router.push('/login')
}
</script>
