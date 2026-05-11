<template>
  <div class="login-page">
    <div class="login-box">
      <h2>TProxy</h2>
      <div v-if="error" class="alert alert-error">{{ error }}</div>
      <div class="form-group">
        <label>用户名</label>
        <input v-model="username" class="form-control" autofocus @keyup.enter="login" />
      </div>
      <div class="form-group">
        <label>密码</label>
        <input v-model="password" type="password" class="form-control" @keyup.enter="login" />
      </div>
      <button class="btn btn-primary" style="width: 100%; margin-top: 8px;" @click="login" :disabled="loading">
        {{ loading ? '登录中...' : '登录' }}
      </button>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api'

const router = useRouter()
const username = ref('admin')
const password = ref('')
const error = ref('')
const loading = ref(false)

async function login() {
  if (!password.value) {
    error.value = '请输入密码'
    return
  }
  error.value = ''
  loading.value = true
  try {
    await api.login(username.value, password.value)
    router.push('/status')
  } catch (e) {
    error.value = e.message
  } finally {
    loading.value = false
  }
}
</script>
