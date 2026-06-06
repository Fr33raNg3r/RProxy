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
    if (!location.pathname.endsWith('/login')) {
      location.href = '/login'
    }
    throw new Error('未登录')
  }
  const ct = resp.headers.get('content-type') || ''
  if (ct.includes('application/json')) {
    const data = await resp.json()
    // 强制改密：被中间件拦截，回到登录页
    if (resp.status === 403 && data.code === 'password_change_required') {
      if (!location.pathname.endsWith('/login')) {
        location.href = '/login'
      }
      throw new Error('请先修改默认密码')
    }
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
  refreshLatestVersion() { return request('/api/version/refresh', { method: 'POST' }) },
  triggerUpgrade() { return request('/api/upgrade', { method: 'POST' }) },
  getUpgradeLog(offset = 0) { return request('/api/upgrade/log?offset=' + offset) },
  getXrayAlert() { return request('/api/xray/alert') },
  dismissXrayAlert() { return request('/api/xray/alert/dismiss', { method: 'POST' }) },
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
  getDNSUpstreams() { return request('/api/dns/upstreams') },
  getDNSUpstreamsDefaults() { return request('/api/dns/upstreams/defaults') },
  updateDNSUpstreams(upstreams) { return request('/api/dns/upstreams', { method: 'PUT', body: upstreams }) },

  // 设置
  getSettings() { return request('/api/settings') },
  updateSettings(settings) { return request('/api/settings', { method: 'PUT', body: settings }) },
  emergencyStop() { return request('/api/emergency-stop', { method: 'POST' }) },
  emergencyResume() { return request('/api/emergency-resume', { method: 'POST' }) },

  // 配置导入/导出
  exportConfigURL() { return '/api/config/export' },
  async importConfig(yamlText) {
    const resp = await fetch('/api/config/import', {
      method: 'POST',
      credentials: 'same-origin',
      headers: { 'Content-Type': 'application/x-yaml' },
      body: yamlText,
    })
    const data = await resp.json().catch(() => ({}))
    if (resp.status === 401) {
      if (!location.pathname.endsWith('/login')) location.href = '/login'
      throw new Error('未登录')
    }
    if (!resp.ok) throw new Error(data.error || '导入失败')
    return data
  },
  restartService(name) { return request('/api/services/' + name + '/restart', { method: 'POST' }) },

  // 日志
  getLogs(component, lines = 200) {
    return request('/api/logs/' + component + '?lines=' + lines)
  }
}
