<template>
  <div>
    <div class="page-header">
      <h2>节点管理</h2>
      <n-button type="primary" @click="openCreate">+ 添加节点</n-button>
    </div>

    <n-alert v-if="error" type="error" closable style="margin-bottom: 12px;" @close="error = ''">{{ error }}</n-alert>
    <n-alert v-if="success" type="success" closable style="margin-bottom: 12px;" @close="success = ''">{{ success }}</n-alert>

    <n-card size="medium">
      <n-empty v-if="nodes.length === 0" description="尚未添加任何节点。点击右上角「添加节点」开始。" />
      <n-table v-else :bordered="false" :single-line="false" size="small">
        <thead>
          <tr>
            <th style="width: 130px;">序号</th>
            <th>名称</th>
            <th>地址</th>
            <th>WS 路径</th>
            <th style="width: 80px;">状态</th>
            <th style="width: 320px;">操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="(n, idx) in nodes" :key="n.id" :class="{ 'is-current': n.id === currentNodeId }">
            <td>
              <div class="row-actions">
                <span class="text-mono">{{ idx + 1 }}</span>
                <n-button size="tiny" @click="moveUp(idx)" :disabled="idx === 0" title="上移">↑</n-button>
                <n-button size="tiny" @click="moveDown(idx)" :disabled="idx === nodes.length - 1" title="下移">↓</n-button>
              </div>
            </td>
            <td><strong>{{ n.name }}</strong></td>
            <td class="text-mono text-sm">{{ n.address }}:{{ n.port }}</td>
            <td class="text-mono text-sm">{{ n.ws_path }}</td>
            <td>
              <n-tag :type="n.enabled ? 'success' : 'default'" size="small" round>
                {{ n.enabled ? '启用' : '未启用' }}
              </n-tag>
            </td>
            <td>
              <div class="row-actions">
                <n-button size="tiny" type="primary" @click="switchTo(n)" :disabled="n.id === currentNodeId">切换</n-button>
                <n-button size="tiny" @click="testNode(n)" :disabled="n.id !== currentNodeId">测试</n-button>
                <n-button size="tiny" @click="openEdit(n)">编辑</n-button>
                <n-button size="tiny" type="error" @click="confirmDelete(n)">删除</n-button>
              </div>
            </td>
          </tr>
        </tbody>
      </n-table>
    </n-card>

    <!-- 添加/编辑节点 -->
    <n-modal v-model:show="showModal" preset="card" :title="editingId ? '编辑节点' : '添加节点'" style="width: 540px;" :mask-closable="false">
      <n-alert type="info" :show-icon="false" style="margin-bottom: 14px;">
        仅支持 <strong>VMess + WebSocket + TLS</strong> 协议。地址必须是<strong>域名</strong>（不能是 IP，因为 TLS 需要 SNI 验证）。
      </n-alert>
      <n-form label-placement="top" :show-feedback="false">
        <n-form-item label="名称 *">
          <n-input v-model:value="form.name" placeholder="如：HK-VPS" />
        </n-form-item>
        <n-grid :cols="3" :x-gap="12">
          <n-gi :span="2">
            <n-form-item label="地址（域名）*">
              <n-input v-model:value="form.address" placeholder="如：vps.example.com" />
            </n-form-item>
          </n-gi>
          <n-gi>
            <n-form-item label="端口 *">
              <n-input-number v-model:value="form.port" :min="1" :max="65535" placeholder="443" style="width: 100%;" />
            </n-form-item>
          </n-gi>
        </n-grid>
        <n-form-item label="UUID *">
          <n-input v-model:value="form.uuid" placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx" />
        </n-form-item>
        <n-grid :cols="2" :x-gap="12">
          <n-gi>
            <n-form-item label="AlterID">
              <n-input-number v-model:value="form.alter_id" :min="0" style="width: 100%;" />
            </n-form-item>
          </n-gi>
          <n-gi>
            <n-form-item label="加密方式">
              <n-select v-model:value="form.security" :options="securityOpts" />
            </n-form-item>
          </n-gi>
        </n-grid>
        <n-form-item label="WS 路径 *">
          <n-input v-model:value="form.ws_path" placeholder="/a1b2c3d4e5f60789" />
        </n-form-item>
        <div class="text-muted text-xs" style="margin: -6px 0 12px;">必须以 / 开头</div>
        <n-form-item label="SNI / Host">
          <n-input v-model:value="form.host" :placeholder="'留空则使用地址：' + (form.address || 'example.com')" />
        </n-form-item>
        <div class="text-muted text-xs" style="margin: -6px 0 0;">通常等于地址，特殊场景可独立设置</div>
      </n-form>
      <template #footer>
        <div style="display: flex; gap: 8px; justify-content: flex-end;">
          <n-button @click="closeModal">取消</n-button>
          <n-button type="primary" :loading="saving" @click="saveNode">保存</n-button>
        </div>
      </template>
    </n-modal>

    <!-- 测试结果 -->
    <n-modal v-model:show="showTest" preset="card" title="节点测试结果" style="width: 400px;">
      <n-alert v-if="testResult?.ok" type="success" :show-icon="false">
        ✓ 连接成功，延迟：<strong>{{ testResult.latency }}ms</strong>
      </n-alert>
      <n-alert v-else-if="testResult" type="error" :show-icon="false">
        ✗ 测试失败：{{ testResult.error }}
      </n-alert>
      <template #footer>
        <div style="display: flex; justify-content: flex-end;">
          <n-button type="primary" @click="showTest = false">关闭</n-button>
        </div>
      </template>
    </n-modal>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useDialog, useMessage } from 'naive-ui'
import { api } from '../api'

const dialog = useDialog()
const message = useMessage()

const nodes = ref([])
const currentNodeId = ref('')
const error = ref('')
const success = ref('')
const showModal = ref(false)
const editingId = ref(null)
const saving = ref(false)
const testResult = ref(null)
const showTest = ref(false)

const securityOpts = [
  { label: 'auto（推荐）', value: 'auto' },
  { label: 'aes-128-gcm', value: 'aes-128-gcm' },
  { label: 'chacha20-poly1305', value: 'chacha20-poly1305' },
  { label: 'none', value: 'none' },
  { label: 'zero', value: 'zero' }
]

const blankForm = () => ({
  name: '',
  address: '',
  port: 443,
  uuid: '',
  alter_id: 0,
  security: 'auto',
  ws_path: '',
  host: '',
})

const form = ref(blankForm())

async function load() {
  try {
    const r = await api.listNodes()
    nodes.value = r.nodes || []
    currentNodeId.value = r.current_node_id || ''
  } catch (e) {
    error.value = '加载失败：' + e.message
  }
}

function openCreate() {
  editingId.value = null
  form.value = blankForm()
  error.value = ''
  showModal.value = true
}

function openEdit(n) {
  editingId.value = n.id
  form.value = {
    name: n.name,
    address: n.address,
    port: n.port,
    uuid: n.uuid,
    alter_id: n.alter_id || 0,
    security: n.security || 'auto',
    ws_path: n.ws_path || '/',
    host: n.host || '',
  }
  error.value = ''
  showModal.value = true
}

function closeModal() {
  showModal.value = false
  editingId.value = null
  form.value = blankForm()
}

async function saveNode() {
  if (!form.value.name.trim()) { error.value = '名称不能为空'; return }
  if (!form.value.address.trim()) { error.value = '地址不能为空'; return }
  if (/^\d+\.\d+\.\d+\.\d+$/.test(form.value.address) || form.value.address.includes(':')) {
    error.value = '地址必须是域名（VMess+WS+TLS 协议不能用 IP）'
    return
  }
  if (!form.value.uuid.trim()) { error.value = 'UUID 不能为空'; return }
  if (!form.value.ws_path.trim() || !form.value.ws_path.startsWith('/')) {
    error.value = 'WS 路径必须以 / 开头'
    return
  }

  saving.value = true
  error.value = ''
  try {
    if (editingId.value) {
      await api.updateNode(editingId.value, form.value)
      success.value = '节点已更新'
    } else {
      await api.createNode(form.value)
      success.value = '节点已添加'
    }
    closeModal()
    await load()
    setTimeout(() => { success.value = '' }, 3000)
  } catch (e) {
    error.value = '保存失败：' + e.message
  } finally {
    saving.value = false
  }
}

async function switchTo(n) {
  try {
    await api.switchNode(n.id)
    success.value = `已切换到节点 ${n.name}`
    await load()
    setTimeout(() => { success.value = '' }, 3000)
  } catch (e) {
    error.value = '切换失败：' + e.message
  }
}

async function testNode(n) {
  try {
    const r = await api.testNode(n.id)
    testResult.value = r
    showTest.value = true
  } catch (e) {
    error.value = '测试失败：' + e.message
  }
}

function confirmDelete(n) {
  dialog.warning({
    title: '删除节点',
    content: `确定删除节点 "${n.name}"？`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await api.deleteNode(n.id)
        success.value = '节点已删除'
        await load()
        setTimeout(() => { success.value = '' }, 3000)
      } catch (e) {
        error.value = '删除失败：' + e.message
      }
    }
  })
}

async function moveUp(idx) {
  if (idx === 0) return
  const arr = [...nodes.value]
  ;[arr[idx], arr[idx - 1]] = [arr[idx - 1], arr[idx]]
  await reorder(arr)
}

async function moveDown(idx) {
  if (idx === nodes.value.length - 1) return
  const arr = [...nodes.value]
  ;[arr[idx], arr[idx + 1]] = [arr[idx + 1], arr[idx]]
  await reorder(arr)
}

async function reorder(arr) {
  try {
    await api.reorderNodes(arr.map(n => n.id))
    await load()
  } catch (e) {
    error.value = '排序失败：' + e.message
  }
}

onMounted(load)
</script>

<style scoped>
.is-current {
  background: rgba(77, 208, 225, 0.08);
}
</style>
