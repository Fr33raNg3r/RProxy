import { createRouter, createWebHistory } from 'vue-router'

const routes = [
  { path: '/login', component: () => import('./views/Login.vue') },
  {
    path: '/',
    component: () => import('./views/Layout.vue'),
    children: [
      { path: '', redirect: '/status' },
      { path: 'status',    component: () => import('./views/Status.vue') },
      { path: 'nodes',     component: () => import('./views/Nodes.vue') },
      { path: 'wireguard', component: () => import('./views/WireGuard.vue') },
      { path: 'dns',       component: () => import('./views/DNS.vue') },
      { path: 'settings',  component: () => import('./views/Settings.vue') },
      { path: 'logs',      component: () => import('./views/Logs.vue') }
    ]
  }
]

const router = createRouter({
  history: createWebHistory(),
  routes
})

export default router
