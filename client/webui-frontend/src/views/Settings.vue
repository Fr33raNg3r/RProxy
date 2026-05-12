<template>
  <div>
    <div class="main-header">
      <h2>设置</h2>
    </div>

    <div v-if="error" class="alert alert-error">{{ error }}</div>
    <div v-if="success" class="alert alert-success">{{ success }}</div>

    <!-- WireGuard 入站服务 -->
    <div class="card">
      <div class="card-title">WireGuard 入站服务</div>
      <div class="text-muted mb-3">
        启用后，可在【WireGuard】页面添加 peer，让手机/电脑通过 WireGuard 接入旁路由。<br>
        禁用时所有 peer 配置仍会保留，再次启用立即可用。
      </div>
      <div class="flex gap-3" style="align-items: center;">
        <span class="badge" :class="settings.wg_enabled ? 'badge-success' : 'badge-muted'">
          {{ settings.wg_enabled ? '已启用' : '已禁用' }}
        </span>
        <button v-if="!settings.wg_enabled"
                class="btn btn-primary"
                @click="enableWG"
                :disabled="wgToggling">
          {{ wgToggling ? '启动中...' : '启用 WireGuard 服务' }}
        </button>
        <button v-else
                class="btn btn-danger"
                @click="disableWG"
                :disabled="wgToggling">
          {{ wgToggling ? '停止中...' : '禁用 WireGuard 服务' }}
        </button>
      </div>
    </div>

    <!-- 基础设置 -->
    <div class="card">
      <div class="card-title">基础设置</div>
      <div class="form-group">
        <label>WebUI 监听端口</label>
        <input v-model.number="settings.listen_port" type="number" class="form-control input-tiny" />
        <div class="text-muted text-xs mt-2">修改后将自动重启 WebUI 服务，需用新端口重新访问</div>
      </div>
      <div class="form-group">
        <label>WireGuard 监听端口</label>
        <input v-model.number="settings.wg_listen_port" type="number" class="form-control input-tiny" />
        <div class="text-muted text-xs mt-2">修改后将自动重启 WG 服务</div>
      </div>
      <div class="flex gap-3">
        <div class="form-group" style="flex: 1;">
          <label>每日更新时间 — 时</label>
          <input v-model.number="settings.update_hour" type="number" min="0" max="23" class="form-control input-tiny" />
        </div>
        <div class="form-group" style="flex: 1;">
          <label>每日更新时间 — 分</label>
          <input v-model.number="settings.update_minute" type="number" min="0" max="59" class="form-control input-tiny" />
        </div>
      </div>
      <button class="btn btn-primary" @click="saveSettings" :disabled="saving">
        {{ saving ? '保存中...' : '保存设置' }}
      </button>
    </div>

    <!-- 修改密码 -->
    <div class="card">
      <div class="card-title">修改密码</div>
      <div class="form-group">
        <label>旧密码</label>
        <input v-model="oldPwd" type="password" class="form-control" />
      </div>
      <div class="form-group">
        <label>新密码（至少 6 位）</label>
        <input v-model="newPwd" type="password" class="form-control" />
      </div>
      <div class="form-group">
        <label>再次输入新密码</label>
        <input v-model="confirmPwd" type="password" class="form-control" />
      </div>
      <button class="btn btn-primary" @click="changePwd" :disabled="changingPwd">
        {{ changingPwd ? '修改中...' : '修改密码' }}
      </button>
    </div>

    <!-- 紧急停止 -->
    <div class="card">
      <div class="card-title text-danger">紧急停止透明代理</div>
      <div class="text-muted mb-3">
        如果分流出问题导致全网断网，可一键清空 nftables 规则恢复直连。<br>
        相当于在 SSH 中执行 <code>/opt/tproxy-gw/scripts/emergency-stop.sh</code>
      </div>
      <button class="btn btn-danger" @click="emergencyStop" :disabled="stopping">
        {{ stopping ? '执行中...' : '紧急停止' }}
      </button>
      <pre v-if="stopOutput" class="text-mono text-sm mt-3" style="background: #f9fafb; padding: 12px; border-radius: 6px;">{{ stopOutput }}</pre>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { api } from '../api'

const settings = ref({
  listen_port: 80,
  wg_enabled: false,
  wg_listen_port: 51820,
  update_hour: 4,
  update_minute: 0
})
const error = ref('')
const success = ref('')
const saving = ref(false)
const wgToggling = ref(false)

const oldPwd = ref('')
const newPwd = ref('')
const confirmPwd = ref('')
const changingPwd = ref(false)

const stopping = ref(false)
const stopOutput = ref('')

async function load() {
  try {
    const data = await api.getSettings()
    settings.value = {
      listen_port: data.listen_port,
      wg_enabled: !!data.wg_enabled,
      wg_listen_port: data.wg_listen_port,
      update_hour: data.update_hour,
      update_minute: data.update_minute
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
    settings.value.wg_enabled = true
    setTimeout(() => success.value = '', 3000)
  } catch (e) {
    error.value = '启动失败：' + e.message
  } finally {
    wgToggling.value = false
  }
}

async function disableWG() {
  if (!confirm('确定禁用 WireGuard 入站服务？所有 peer 配置会保留，再次启用立即可用。')) return
  wgToggling.value = true
  error.value = ''
  try {
    const r = await api.disableWG()
    success.value = r.message || 'WireGuard 服务已停止'
    settings.value.wg_enabled = false
    setTimeout(() => success.value = '', 3000)
  } catch (e) {
    error.value = '停止失败：' + e.message
  } finally {
    wgToggling.value = false
  }
}

async function saveSettings() {
  saving.value = true
  error.value = ''
  try {
    const r = await api.updateSettings(settings.value)
    success.value = '设置已保存'
    if (r.webui_will_restart) {
      success.value += '。WebUI 将在 1 秒后重启，请稍候并用新端口访问。'
    }
    setTimeout(() => success.value = '', 5000)
  } catch (e) {
    error.value = e.message
  } finally {
    saving.value = false
  }
}

async function changePwd() {
  if (newPwd.value !== confirmPwd.value) {
    error.value = '两次输入的新密码不一致'
    return
  }
  if (newPwd.value.length < 6) {
    error.value = '新密码至少 6 位'
    return
  }
  changingPwd.value = true
  error.value = ''
  try {
    await api.changePassword(oldPwd.value, newPwd.value)
    success.value = '密码已修改'
    oldPwd.value = ''
    newPwd.value = ''
    confirmPwd.value = ''
    setTimeout(() => success.value = '', 3000)
  } catch (e) {
    error.value = e.message
  } finally {
    changingPwd.value = false
  }
}

async function emergencyStop() {
  if (!confirm('确认要紧急停止透明代理？局域网将恢复直连模式。')) return
  stopping.value = true
  error.value = ''
  stopOutput.value = ''
  try {
    const r = await api.emergencyStop()
    stopOutput.value = r.output || '已执行'
    success.value = '透明代理已停止'
    setTimeout(() => success.value = '', 5000)
  } catch (e) {
    error.value = e.message
  } finally {
    stopping.value = false
  }
}

onMounted(load)
</script>
