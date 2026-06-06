<template>
  <div>
    <div class="page-header">
      <h2>设置</h2>
    </div>

    <n-alert v-if="error" type="error" closable style="margin-bottom: 12px;" @close="error = ''">{{ error }}</n-alert>
    <n-alert v-if="success" type="success" closable style="margin-bottom: 12px;" @close="success = ''">{{ success }}</n-alert>

    <div class="card-stack">
      <n-card title="WireGuard 入站服务" size="medium">
        <div class="text-muted" style="margin-bottom: 14px;">
          启用后，可在【WireGuard】页面添加 peer，让手机/电脑通过 WireGuard 接入旁路由。<br>
          禁用时所有 peer 配置仍会保留，再次启用立即可用。
        </div>
        <div style="display: flex; align-items: center; gap: 12px;">
          <n-tag :type="settings.wg_enabled ? 'success' : 'default'" size="medium" round>
            {{ settings.wg_enabled ? '已启用' : '已禁用' }}
          </n-tag>
          <n-button v-if="!settings.wg_enabled" type="primary" :loading="wgToggling" @click="enableWG">
            启用 WireGuard 服务
          </n-button>
          <n-button v-else type="error" :loading="wgToggling" @click="disableWG">
            禁用 WireGuard 服务
          </n-button>
        </div>
      </n-card>

      <n-card title="基础设置" size="medium">
        <n-form label-placement="top" :show-feedback="false">
          <n-form-item label="WebUI 监听端口">
            <n-input-number v-model:value="settings.listen_port" :min="1" :max="65535" style="width: 180px;" />
          </n-form-item>
          <div class="text-muted text-xs" style="margin: -4px 0 12px;">修改后将自动重启 WebUI 服务，需用新端口重新访问</div>

          <n-form-item label="WireGuard 监听端口">
            <n-input-number v-model:value="settings.wg_listen_port" :min="1" :max="65535" style="width: 180px;" />
          </n-form-item>
          <div class="text-muted text-xs" style="margin: -4px 0 12px;">修改后将自动重启 WG 服务</div>

          <n-form-item label="每日更新时间（北京时间）">
            <div style="display: flex; align-items: center; gap: 6px;">
              <n-input-number v-model:value="settings.update_hour" :min="0" :max="23" :show-button="false" style="width: 56px;" />
              <span>时</span>
              <n-input-number v-model:value="settings.update_minute" :min="0" :max="59" :show-button="false" style="width: 56px;" />
              <span>分</span>
            </div>
          </n-form-item>
        </n-form>
        <n-button type="primary" :loading="saving" style="margin-top: 16px;" @click="saveSettings">保存设置</n-button>
      </n-card>

      <n-card title="本机网络（IP / 网关）" size="medium">
        <div class="text-muted" style="margin-bottom: 14px;">
          旁路由本机的静态 IP 和网关，写入网卡配置 <code>/etc/network/interfaces</code>。<br>
          IP 必须为<strong>内网地址</strong>且使用 <strong>CIDR</strong> 格式，例如 <code>192.168.1.2/24</code>。<br>
          <span style="color:#f87171;">保存后会立即重启网络，若 IP 有变更，当前页面会断开，请用新 IP 重新访问 WebUI。</span>
        </div>
        <n-form label-placement="top" :show-feedback="false">
          <n-form-item label="网卡接口">
            <n-input v-model:value="lan.lan_iface" placeholder="如 eth0 / ens18" style="width: 240px;" />
          </n-form-item>
          <n-form-item label="本机 IP 地址（CIDR）">
            <n-input v-model:value="lan.lan_ip_cidr" placeholder="如 192.168.1.2/24" style="width: 240px;" />
          </n-form-item>
          <n-form-item label="默认网关">
            <n-input v-model:value="lan.lan_gateway" placeholder="如 192.168.1.1" style="width: 240px;" />
          </n-form-item>
        </n-form>
        <n-button type="warning" :loading="savingLan" style="margin-top: 16px;" @click="saveLan">保存并应用网络配置</n-button>
      </n-card>

      <n-card title="修改密码" size="medium">
        <n-form label-placement="top" :show-feedback="false">
          <n-form-item label="旧密码">
            <n-input v-model:value="oldPwd" type="password" show-password-on="click" />
          </n-form-item>
          <n-form-item label="新密码（至少 6 位）">
            <n-input v-model:value="newPwd" type="password" show-password-on="click" />
          </n-form-item>
          <n-form-item label="再次输入新密码">
            <n-input v-model:value="confirmPwd" type="password" show-password-on="click" />
          </n-form-item>
        </n-form>
        <n-button type="primary" :loading="changingPwd" style="margin-top: 12px;" @click="changePwd">修改密码</n-button>
      </n-card>

      <n-card title="配置导入/导出" size="medium">
        <div class="text-muted" style="margin-bottom: 14px;">
          导出节点池、DNS 上游、基础设置为 YAML 文件。<br>
          导入会替换上述配置并重启 Xray/mosdns。<strong>不包含密码、session 等敏感字段。</strong>
        </div>
        <div class="row-actions">
          <n-button type="primary" @click="exportConfig">导出 YAML</n-button>
          <n-button :loading="importing" @click="$refs.cfgFile.click()">导入 YAML</n-button>
          <input ref="cfgFile" type="file" accept=".yaml,.yml,text/yaml" style="display:none" @change="onImportFile" />
        </div>
        <n-alert v-if="importResult" type="success" :show-icon="false" style="margin-top: 14px;">{{ importResult }}</n-alert>
      </n-card>

      <n-card size="medium">
        <template #header>
          <span style="color: #f87171;">紧急停止 / 恢复透明代理</span>
        </template>
        <div class="text-muted" style="margin-bottom: 14px;">
          分流故障时点「紧急停止」清空规则恢复直连；恢复后点「恢复透明代理」重新启用。<br>
          对应脚本：<code>/opt/tproxy-gw/scripts/emergency-stop.sh</code> / <code>emergency-resume.sh</code>
        </div>
        <div class="row-actions">
          <n-button type="error" :loading="stopping" :disabled="resuming" @click="emergencyStop">紧急停止</n-button>
          <n-button type="primary" :loading="resuming" :disabled="stopping" @click="emergencyResume">恢复透明代理</n-button>
        </div>
        <pre v-if="stopOutput" class="log-output" style="margin-top: 14px;">{{ stopOutput }}</pre>
      </n-card>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useDialog } from 'naive-ui'
import { api } from '../api'

const dialog = useDialog()

const settings = ref({
  listen_port: 80,
  wg_enabled: false,
  wg_listen_port: 51820,
  update_hour: 4,
  update_minute: 0
})
const lan = ref({
  lan_iface: '',
  lan_ip_cidr: '',
  lan_gateway: ''
})
const error = ref('')
const success = ref('')
const saving = ref(false)
const savingLan = ref(false)
const wgToggling = ref(false)

const oldPwd = ref('')
const newPwd = ref('')
const confirmPwd = ref('')
const changingPwd = ref(false)

const stopping = ref(false)
const resuming = ref(false)
const stopOutput = ref('')

const importing = ref(false)
const importResult = ref('')

function exportConfig() {
  window.location.href = api.exportConfigURL()
}

async function onImportFile(e) {
  const file = e.target.files[0]
  if (!file) return
  e.target.value = ''
  await new Promise((resolve) => {
    dialog.warning({
      title: '导入配置',
      content: `确认导入 ${file.name}？将替换节点池、DNS 上游、基础设置并重启 Xray/mosdns。`,
      positiveText: '导入',
      negativeText: '取消',
      onPositiveClick: async () => {
        importing.value = true
        error.value = ''
        importResult.value = ''
        try {
          const text = await file.text()
          const r = await api.importConfig(text)
          importResult.value = `导入完成：节点 ${r.node_count} 个${r.dns ? '，DNS 配置已替换' : ''}`
          success.value = '配置已导入'
          setTimeout(() => success.value = '', 5000)
        } catch (e) {
          error.value = '导入失败：' + e.message
        } finally {
          importing.value = false
          resolve()
        }
      },
      onNegativeClick: resolve,
      onClose: resolve,
      onMaskClick: resolve
    })
  })
}

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
    lan.value = {
      lan_iface: data.lan_iface || '',
      lan_ip_cidr: data.lan_ip_cidr || '',
      lan_gateway: data.lan_gateway || ''
    }
  } catch (e) {
    error.value = e.message
  }
}

function saveLan() {
  dialog.warning({
    title: '应用网络配置',
    content: `确认把本机 IP 设为 ${lan.value.lan_ip_cidr}、网关 ${lan.value.lan_gateway}（接口 ${lan.value.lan_iface}）？保存后会立即重启网络，若 IP 有变更当前页面会断开，请用新 IP 重新访问。`,
    positiveText: '保存并应用',
    negativeText: '取消',
    onPositiveClick: async () => {
      savingLan.value = true
      error.value = ''
      try {
        const r = await api.updateSettings({
          lan_iface: lan.value.lan_iface,
          lan_ip_cidr: lan.value.lan_ip_cidr,
          lan_gateway: lan.value.lan_gateway
        })
        success.value = '网络配置已保存'
        if (r.network_will_restart) {
          success.value += '。网络即将重启，若 IP 有变更请用新地址重新访问 WebUI。'
        }
        setTimeout(() => success.value = '', 8000)
      } catch (e) {
        error.value = e.message
      } finally {
        savingLan.value = false
      }
    }
  })
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
        settings.value.wg_enabled = false
        setTimeout(() => success.value = '', 3000)
      } catch (e) {
        error.value = '停止失败：' + e.message
      } finally {
        wgToggling.value = false
      }
    }
  })
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

function emergencyStop() {
  dialog.error({
    title: '紧急停止',
    content: '确认要紧急停止透明代理？局域网将恢复直连模式。',
    positiveText: '紧急停止',
    negativeText: '取消',
    onPositiveClick: async () => {
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
  })
}

function emergencyResume() {
  dialog.warning({
    title: '恢复透明代理',
    content: '确认要恢复透明代理？',
    positiveText: '恢复',
    negativeText: '取消',
    onPositiveClick: async () => {
      resuming.value = true
      error.value = ''
      stopOutput.value = ''
      try {
        const r = await api.emergencyResume()
        stopOutput.value = r.output || '已执行'
        success.value = '透明代理已恢复'
        setTimeout(() => success.value = '', 5000)
      } catch (e) {
        error.value = e.message
      } finally {
        resuming.value = false
      }
    }
  })
}

onMounted(load)
</script>
