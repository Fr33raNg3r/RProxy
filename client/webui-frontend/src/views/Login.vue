<template>
  <div class="login-page">
    <n-card class="login-card" :bordered="false">
      <div class="login-brand">RProxy</div>

      <template v-if="!forceChange">
        <n-alert v-if="error" type="error" :show-icon="false" style="margin-bottom: 14px;">{{ error }}</n-alert>
        <n-form ref="formRef" :show-feedback="false">
          <n-form-item label="用户名">
            <n-input v-model:value="username" autofocus @keyup.enter="login" />
          </n-form-item>
          <n-form-item label="密码">
            <n-input v-model:value="password" type="password" show-password-on="click" @keyup.enter="login" />
          </n-form-item>
        </n-form>
        <n-button type="primary" block :loading="loading" style="margin-top: 18px;" @click="login">登录</n-button>
      </template>

      <template v-else>
        <n-alert type="warning" style="margin-bottom: 14px;">首次登录，请修改默认密码后再继续。</n-alert>
        <n-alert v-if="error" type="error" :show-icon="false" style="margin-bottom: 14px;">{{ error }}</n-alert>
        <n-form :show-feedback="false">
          <n-form-item label="新密码（至少 6 位）">
            <n-input v-model:value="newPwd" type="password" show-password-on="click" autofocus @keyup.enter="submitChange" />
          </n-form-item>
          <n-form-item label="确认新密码">
            <n-input v-model:value="newPwd2" type="password" show-password-on="click" @keyup.enter="submitChange" />
          </n-form-item>
        </n-form>
        <n-button type="primary" block :loading="loading" style="margin-top: 18px;" @click="submitChange">
          修改并进入
        </n-button>
      </template>
    </n-card>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api'

const router = useRouter()
const username = ref('admin')
const password = ref('')
const newPwd = ref('')
const newPwd2 = ref('')
const error = ref('')
const loading = ref(false)
const forceChange = ref(false)

async function login() {
  if (!password.value) {
    error.value = '请输入密码'
    return
  }
  error.value = ''
  loading.value = true
  try {
    const r = await api.login(username.value, password.value)
    if (r.must_change_password) {
      forceChange.value = true
    } else {
      router.push('/status')
    }
  } catch (e) {
    error.value = e.message
  } finally {
    loading.value = false
  }
}

async function submitChange() {
  if (newPwd.value.length < 6) {
    error.value = '新密码至少 6 位'
    return
  }
  if (newPwd.value !== newPwd2.value) {
    error.value = '两次输入不一致'
    return
  }
  error.value = ''
  loading.value = true
  try {
    await api.changePassword(password.value, newPwd.value)
    router.push('/status')
  } catch (e) {
    error.value = e.message
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.login-page {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: radial-gradient(ellipse at top, #1e293b 0%, #0f172a 60%);
  padding: 20px;
}
.login-card {
  width: 100%;
  max-width: 380px;
  box-shadow: 0 20px 50px rgba(0, 0, 0, 0.4);
}
.login-brand {
  text-align: center;
  font-size: 26px;
  font-weight: 700;
  letter-spacing: 2px;
  color: #4dd0e1;
  margin-bottom: 24px;
}
</style>
