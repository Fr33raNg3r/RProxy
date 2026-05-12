// 统一的 API 客户端
// 所有请求都带 cookie（HttpOnly session），401 → 跳转登录

const BASE = ''  // 同域，相对路径

async function request(url, options = {}) {
  const opts = {
    credentials: 'same-origin',
    headers: { 'Content-Type': 'application/json' },
    ...options
  }
  if (opts.body && typeof opts.body !== 'string') {
    opts.body = JSON.stringify(opts.body)
  }
  const resp = await fetch(BASE + url, opts)
  if (resp.status === 401) {
    // 跳转到登录页
    if (!location.pathname.endsWith('/login')) {
      location.href = '/login'
    }
    throw new Error('未登录')
  }
  const ct = resp.headers.get('content-type') || ''
  if (ct.includes('application/json')) {
    const data = await resp.json()
    if (!resp.ok) throw new Error(data.error || '请求失败')
    return data
  }
  if (!resp.ok) throw new Error(await resp.text())
  return resp
}

export const api = {
  // 认证
  login(username, password) {
    return request('/api/login', { method: 'POST', body: { username, password } })
  },
  logout() {
    return request('/api/logout', { method: 'POST' })
  },
  changePassword(oldPwd, newPwd) {
    return request('/api/settings/password', {
      method: 'POST',
      body: { old_password: oldPwd, new_password: newPwd }
    })
  },

  // 状态
  getStatus() { return request('/api/status') },

  // 节点
  listNodes() { return request('/api/nodes') },
  createNode(node) { return request('/api/nodes', { method: 'POST', body: node }) },
  updateNode(id, node) { return request('/api/nodes/' + id, { method: 'PUT', body: node }) },
  deleteNode(id) { return request('/api/nodes/' + id, { method: 'DELETE' }) },
  switchNode(id) { return request('/api/nodes/' + id + '/switch', { method: 'POST' }) },
  testNode(id) { return request('/api/nodes/' + id + '/test', { method: 'POST' }) },
  reorderNodes(ids) { return request('/api/nodes/reorder', { method: 'POST', body: { ids } }) },

  // WireGuard
  listPeers() { return request('/api/wireguard/peers') },
  createPeer(name) { return request('/api/wireguard/peers', { method: 'POST', body: { name } }) },
  deletePeer(id) { return request('/api/wireguard/peers/' + id, { method: 'DELETE' }) },
  enableWG() { return request('/api/wireguard/enable', { method: 'POST' }) },
  disableWG() { return request('/api/wireguard/disable', { method: 'POST' }) },
  setEndpoint(endpoint) {
    return request('/api/wireguard/endpoint', { method: 'POST', body: { endpoint } })
  },

  // 版本检查与升级
  getVersion() { return request('/api/version') },
  triggerUpgrade() { return request('/api/upgrade', { method: 'POST' }) },
  peerQRCodeURL(id, endpoint) {
    const q = endpoint ? '?endpoint=' + encodeURIComponent(endpoint) : ''
    return '/api/wireguard/peers/' + id + '/qrcode' + q
  },
  peerConfigURL(id, endpoint) {
    const q = endpoint ? '?endpoint=' + encodeURIComponent(endpoint) : ''
    return '/api/wireguard/peers/' + id + '/config' + q
  },

  // DNS
  getDNSRules() { return request('/api/dns/rules') },
  updateDNSRules(rules) { return request('/api/dns/rules', { method: 'PUT', body: rules }) },

  // 设置
  getSettings() { return request('/api/settings') },
  updateSettings(settings) { return request('/api/settings', { method: 'PUT', body: settings }) },
  emergencyStop() { return request('/api/emergency-stop', { method: 'POST' }) },
  restartService(name) { return request('/api/services/' + name + '/restart', { method: 'POST' }) },

  // 日志
  getLogs(component, lines = 200) {
    return request('/api/logs/' + component + '?lines=' + lines)
  }
}
