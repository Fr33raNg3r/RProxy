<template>
  <div class="layout-root">
    <div v-if="mobileMenuOpen" class="sider-backdrop" @click="mobileMenuOpen = false"></div>
    <aside class="sider" :class="{ open: mobileMenuOpen }">
      <div class="brand">
        <div class="brand-title">RProxy</div>
        <div v-if="version.current" class="brand-version">v{{ version.current }}</div>
        <div class="brand-latest">
          <span class="text-muted">最新：</span>
          <span v-if="!version.latest || version.latest === '未知'" class="text-muted">检查中…</span>
          <span v-else-if="version.has_update" style="color: #fbbf24;">v{{ version.latest }}</span>
          <span v-else style="color: #4ade80;">v{{ version.latest }}（已是最新）</span>
          <n-button size="tiny" text @click="onRefreshLatest" :loading="refreshingLatest" style="margin-left: 6px;" title="重新检查">↻</n-button>
        </div>
        <div v-if="version.has_update" class="brand-update">
          <n-button size="tiny" type="warning" :loading="upgrading" @click="onUpgrade">
            立即升级
          </n-button>
        </div>
      </div>
      <n-menu
        :value="activeKey"
        :options="menuOptions"
        :indent="18"
        :root-indent="18"
        @update:value="onSelect"
      />
      <div class="sider-footer">
        <n-button size="small" block secondary @click="logout">退出登录</n-button>
      </div>
    </aside>

    <main class="app-main">
      <button class="menu-toggle" @click="mobileMenuOpen = true" aria-label="打开菜单">☰</button>
      <n-alert
        v-if="xrayAlert.active"
        type="error"
        closable
        title="Xray 自动更新已回滚"
        style="margin-bottom: 14px;"
        @close="onDismissXrayAlert"
      >
        {{ xrayAlert.message }}
        <span v-if="xrayAlert.time" class="text-muted text-sm">（{{ xrayAlert.time }}）</span>
      </n-alert>
      <router-view />
    </main>

    <n-modal
      v-model:show="upgradeModal"
      preset="card"
      :mask-closable="false"
      :closable="false"
      title="正在升级 RProxy"
      style="width: 640px;"
    >
      <div style="margin-bottom: 10px;">
        <n-tag :type="upgradeFinished ? 'success' : (upgradeServiceDown ? 'warning' : 'info')" size="small">
          {{ upgradeStatus }}
        </n-tag>
        <span class="text-muted text-sm" style="margin-left: 8px;">
          目标版本 v{{ version.latest }}
        </span>
      </div>
      <n-input
        ref="upgradeLogRef"
        type="textarea"
        :value="upgradeLog || '等待升级输出…'"
        readonly
        :autosize="{ minRows: 14, maxRows: 18 }"
        style="font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 12px;"
      />
      <template #footer>
        <div style="display: flex; gap: 8px; justify-content: flex-end;">
          <n-button v-if="upgradeFinished" type="primary" @click="onUpgradeClose">关闭</n-button>
          <n-button v-else disabled>升级进行中…</n-button>
        </div>
      </template>
    </n-modal>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useDialog, useMessage } from 'naive-ui'
import { api } from '../api'

const router = useRouter()
const route = useRoute()
const dialog = useDialog()
const message = useMessage()

const mobileMenuOpen = ref(false)
const version = ref({ current: '', latest: '', has_update: false })
const xrayAlert = ref({ active: false, time: '', message: '' })
const upgrading = ref(false)
const refreshingLatest = ref(false)

const upgradeModal = ref(false)
const upgradeLog = ref('')
const upgradeOffset = ref(0)
const upgradeFinished = ref(false)
const upgradeServiceDown = ref(false)
const upgradeStatus = ref('启动中…')
let upgradePollTimer = null
let upgradeNotifiedDone = false
let preUpgradeVersion = ''

const menuOptions = [
  { label: '系统状态', key: '/status' },
  { label: '节点管理', key: '/nodes' },
  { label: 'WireGuard', key: '/wireguard' },
  { label: 'DNS 规则', key: '/dns' },
  { label: '设置', key: '/settings' },
  { label: '日志', key: '/logs' }
]

const activeKey = computed(() => '/' + (route.path.split('/')[1] || 'status'))

function onSelect(key) {
  router.push(key)
  mobileMenuOpen.value = false
}

async function loadVersion() {
  // 先用启动时缓存即时显示（秒开）
  try {
    version.value = await api.getVersion()
  } catch {}
  // 再后台静默拉一次 GitHub 最新版本：每次刷新/新打开页面都重新检查，
  // 不依赖 webui 启动那一次缓存（否则启动后发布的新版要手动点刷新才看得到）。
  // 失败不打扰（手动刷新按钮才提示错误）。
  try {
    version.value = await api.refreshLatestVersion()
  } catch {}
}

async function onRefreshLatest() {
  refreshingLatest.value = true
  try {
    version.value = await api.refreshLatestVersion()
  } catch (e) {
    message.error('检查更新失败：' + e.message)
  } finally {
    refreshingLatest.value = false
  }
}

function onUpgrade() {
  dialog.warning({
    title: '确认升级',
    content: `确认升级到 v${version.value.latest}？升级期间 WebUI 可能短暂不可用（1-2 分钟）。`,
    positiveText: '立即升级',
    negativeText: '取消',
    onPositiveClick: async () => {
      upgrading.value = true
      try {
        await api.triggerUpgrade()
        startUpgradeProgress()
      } catch (e) {
        message.error('升级启动失败：' + e.message)
      } finally {
        upgrading.value = false
      }
    }
  })
}

function startUpgradeProgress() {
  upgradeLog.value = ''
  upgradeOffset.value = 0
  upgradeFinished.value = false
  upgradeServiceDown.value = false
  upgradeStatus.value = '启动中…'
  upgradeNotifiedDone = false
  preUpgradeVersion = version.value.current
  upgradeModal.value = true
  if (upgradePollTimer) clearInterval(upgradePollTimer)
  upgradePollTimer = setInterval(pollUpgradeLog, 1500)
  pollUpgradeLog()
}

async function pollUpgradeLog() {
  try {
    const data = await api.getUpgradeLog(upgradeOffset.value)
    if (data.content) {
      upgradeLog.value += data.content
    }
    if (typeof data.size === 'number') {
      upgradeOffset.value = data.size
    }
    if (upgradeServiceDown.value) {
      upgradeServiceDown.value = false
    }
    if (data.done) {
      onUpgradeDone()
      return
    }
    upgradeStatus.value = '升级中…'
  } catch (e) {
    // WebUI 在升级末段会重启 systemd 服务，期间请求会失败 —— 视为预期状态
    upgradeServiceDown.value = true
    upgradeStatus.value = '服务重启中，请稍候…'
    // 顺便探测一下版本：服务恢复后版本号若已变化，即视为升级完成。
    // 用「与升级前版本不同」判断，比依赖 latest（重启后可能尚未拉到）更可靠。
    try {
      const v = await api.getVersion()
      version.value = v
      if (v.current && preUpgradeVersion && v.current !== preUpgradeVersion) {
        onUpgradeDone()
      }
    } catch { /* 继续等 */ }
  }
}

function onUpgradeDone() {
  if (upgradeNotifiedDone) return
  upgradeNotifiedDone = true
  upgradeFinished.value = true
  upgradeStatus.value = '升级完毕'
  if (upgradePollTimer) {
    clearInterval(upgradePollTimer)
    upgradePollTimer = null
  }
  // 不自动刷新：之前 2 秒后自动 reload 会让模态框一闪而过、完成提示看不清。
  // 改为停在「升级完毕」状态，由用户点「关闭」按钮再刷新加载新版前端。
  message.success('升级完毕，点击「关闭」刷新以加载新版本 WebUI')
}

function onUpgradeClose() {
  upgradeModal.value = false
  // 升级已替换前端资源，关闭时刷新加载新版本 WebUI
  location.reload()
}

async function logout() {
  try { await api.logout() } catch {}
  router.push('/login')
}

async function loadXrayAlert() {
  try {
    xrayAlert.value = await api.getXrayAlert()
  } catch {}
}

async function onDismissXrayAlert() {
  xrayAlert.value = { active: false, time: '', message: '' }
  try { await api.dismissXrayAlert() } catch {}
}

onMounted(() => {
  loadVersion()
  loadXrayAlert()
})
onUnmounted(() => {
  if (upgradePollTimer) clearInterval(upgradePollTimer)
})
</script>

<style scoped>
.sider {
  width: 220px;
  flex-shrink: 0;
  height: 100vh;
  position: sticky;
  top: 0;
  background: var(--bg-sider);
  border-right: 1px solid rgba(255, 255, 255, 0.06);
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
.brand {
  padding: 20px 18px 14px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
  margin-bottom: 8px;
}
.brand-title {
  font-size: 22px;
  font-weight: 700;
  letter-spacing: 1px;
  color: #4dd0e1;
}
.brand-version {
  margin-top: 2px;
  font-size: 11px;
  color: rgba(255, 255, 255, 0.45);
  font-family: var(--font-mono);
}
.brand-latest {
  margin-top: 4px;
  font-size: 11px;
  font-family: var(--font-mono);
  display: flex;
  align-items: center;
  flex-wrap: wrap;
}
.brand-latest .text-muted {
  color: rgba(255, 255, 255, 0.45);
}
.brand-update {
  margin-top: 8px;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 4px;
}
.sider-footer {
  padding: 16px 18px;
  margin-top: auto;
}

.menu-toggle {
  display: none;
  width: 36px;
  height: 36px;
  border-radius: 8px;
  border: 1px solid rgba(255, 255, 255, 0.12);
  background: var(--bg-sider);
  color: inherit;
  font-size: 18px;
  cursor: pointer;
  margin-bottom: 14px;
}
.sider-backdrop {
  display: none;
}

@media (max-width: 860px) {
  .menu-toggle {
    display: inline-flex;
    align-items: center;
    justify-content: center;
  }
  .sider {
    position: fixed;
    left: 0;
    top: 0;
    bottom: 0;
    z-index: 100;
    transform: translateX(-100%);
    transition: transform 0.2s ease;
    box-shadow: 2px 0 24px rgba(0, 0, 0, 0.4);
  }
  .sider.open {
    transform: translateX(0);
  }
  .sider-backdrop {
    display: block;
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.5);
    z-index: 99;
  }
}
</style>
