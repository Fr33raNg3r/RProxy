<template>
  <div>
    <div class="main-header">
      <h2>节点管理</h2>
      <button class="btn btn-primary" @click="openCreate">+ 添加节点</button>
    </div>

    <div v-if="error" class="alert alert-error">{{ error }}</div>
    <div v-if="success" class="alert alert-success">{{ success }}</div>

    <div class="card">
      <div v-if="nodes.length === 0" class="text-muted" style="text-align:center; padding: 40px 0;">
        尚未添加任何节点。点击右上角「添加节点」开始。
      </div>
      <table v-else class="table">
        <thead>
          <tr>
            <th style="width: 50px;">序号</th>
            <th>名称</th>
            <th>地址</th>
            <th>WS 路径</th>
            <th>状态</th>
            <th style="width: 280px;">操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="(n, idx) in nodes" :key="n.id" :class="{ 'is-current': n.id === currentNodeId }">
            <td>
              <div class="flex gap-2">
                <span class="text-mono">{{ idx + 1 }}</span>
                <button class="btn btn-sm" @click="moveUp(idx)" :disabled="idx === 0" title="上移">↑</button>
                <button class="btn btn-sm" @click="moveDown(idx)" :disabled="idx === nodes.length - 1" title="下移">↓</button>
              </div>
            </td>
            <td>
              <strong>{{ n.name }}</strong>
              <span v-if="n.id === currentNodeId" class="badge badge-info" style="margin-left: 6px;">当前</span>
            </td>
            <td class="text-mono text-sm">{{ n.address }}:{{ n.port }}</td>
            <td class="text-mono text-sm">{{ n.ws_path }}</td>
            <td>
              <span class="badge" :class="n.enabled ? 'badge-success' : 'badge-muted'">
                {{ n.enabled ? '启用' : '禁用' }}
              </span>
            </td>
            <td>
              <div class="flex gap-2">
                <button class="btn btn-sm btn-primary" @click="switchTo(n)" :disabled="!n.enabled || n.id === currentNodeId">
                  切换
                </button>
                <button class="btn btn-sm" @click="testNode(n)" :disabled="n.id !== currentNodeId">测试</button>
                <button class="btn btn-sm" @click="openEdit(n)">编辑</button>
                <button class="btn btn-sm btn-danger" @click="confirmDelete(n)">删除</button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- 添加/编辑节点模态框 -->
    <div v-if="showModal" class="modal-mask" @click.self="closeModal">
      <div class="modal-box">
        <h3>{{ editingId ? '编辑节点' : '添加节点' }}</h3>
        <div class="alert alert-info text-sm mb-3">
          仅支持 <strong>VMess + WebSocket + TLS</strong> 协议。<br>
          地址必须是<strong>域名</strong>（不能是 IP，因为 TLS 需要 SNI 验证）。
        </div>

        <div class="form-group">
          <label>名称 *</label>
          <input v-model="form.name" class="form-control" placeholder="如：HK-VPS" />
        </div>

        <div class="flex gap-3">
          <div class="form-group" style="flex: 2;">
            <label>地址（域名）*</label>
            <input v-model="form.address" class="form-control" placeholder="如：vps.example.com" />
          </div>
          <div class="form-group" style="flex: 1;">
            <label>端口 *</label>
            <input v-model.number="form.port" type="number" class="form-control input-tiny" placeholder="443" />
          </div>
        </div>

        <div class="form-group">
          <label>UUID *</label>
          <input v-model="form.uuid" class="form-control input-long" placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx" />
        </div>

        <div class="flex gap-3">
          <div class="form-group" style="flex: 1;">
            <label>AlterID</label>
            <input v-model.number="form.alter_id" type="number" class="form-control input-tiny" placeholder="0" />
          </div>
          <div class="form-group" style="flex: 1;">
            <label>加密方式</label>
            <select v-model="form.security" class="form-control input-short">
              <option value="auto">auto（推荐）</option>
              <option value="aes-128-gcm">aes-128-gcm</option>
              <option value="chacha20-poly1305">chacha20-poly1305</option>
              <option value="none">none</option>
              <option value="zero">zero</option>
            </select>
          </div>
        </div>

        <div class="form-group">
          <label>WS 路径 *</label>
          <input v-model="form.ws_path" class="form-control input-long" placeholder="/a1b2c3d4e5f60789" />
          <div class="text-muted text-xs">必须以 / 开头</div>
        </div>

        <div class="form-group">
          <label>SNI / Host</label>
          <input v-model="form.host" class="form-control" :placeholder="'留空则使用地址：' + (form.address || 'example.com')" />
          <div class="text-muted text-xs">通常等于地址，特殊场景可独立设置</div>
        </div>

        <div class="form-group">
          <label class="checkbox-label">
            <input type="checkbox" v-model="form.enabled" />
            启用此节点
          </label>
        </div>

        <div class="flex gap-2" style="justify-content: flex-end; margin-top: 20px;">
          <button class="btn" @click="closeModal">取消</button>
          <button class="btn btn-primary" @click="saveNode" :disabled="saving">
            {{ saving ? '保存中...' : '保存' }}
          </button>
        </div>
      </div>
    </div>

    <!-- 测试结果对话框 -->
    <div v-if="testResult" class="modal-mask" @click.self="testResult = null">
      <div class="modal-box" style="max-width: 400px;">
        <h3>节点测试结果</h3>
        <div v-if="testResult.ok" class="alert alert-success">
          ✓ 连接成功，延迟：<strong>{{ testResult.latency }}ms</strong>
        </div>
        <div v-else class="alert alert-error">
          ✗ 测试失败：{{ testResult.error }}
        </div>
        <div class="flex gap-2" style="justify-content: flex-end; margin-top: 16px;">
          <button class="btn btn-primary" @click="testResult = null">关闭</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { api } from '../api'

const nodes = ref([])
const currentNodeId = ref('')
const error = ref('')
const success = ref('')
const showModal = ref(false)
const editingId = ref(null)
const saving = ref(false)
const testResult = ref(null)

const blankForm = () => ({
  name: '',
  address: '',
  port: 443,
  uuid: '',
  alter_id: 0,
  security: 'auto',
  ws_path: '',
  host: '',
  enabled: true,
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
    enabled: n.enabled,
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
  // 前端基础校验
  if (!form.value.name.trim()) { error.value = '名称不能为空'; return }
  if (!form.value.address.trim()) { error.value = '地址不能为空'; return }
  // IP 格式检测
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
  } catch (e) {
    error.value = '测试失败：' + e.message
  }
}

function confirmDelete(n) {
  if (!confirm(`确定删除节点 "${n.name}"？`)) return
  api.deleteNode(n.id)
    .then(async () => {
      success.value = '节点已删除'
      await load()
      setTimeout(() => { success.value = '' }, 3000)
    })
    .catch(e => { error.value = '删除失败：' + e.message })
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
  background: rgba(77, 208, 225, 0.06);
}
.checkbox-label {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
}
.text-xs { font-size: 11px; }
.mb-3 { margin-bottom: 12px; }
</style>
