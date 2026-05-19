<script setup>
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

const menus = [
  { path: '/dashboard', label: '监控总览' },
  { path: '/services', label: '服务配置' },
  { path: '/routes', label: '路由配置' },
  { path: '/ratelimit', label: '限流规则' },
  { path: '/config', label: '配置版本' }
]

const current = computed(() => route.path)

function logout() {
  auth.clearToken()
  router.push('/login')
}
</script>

<template>
  <div class="shell">
    <aside class="sidebar">
      <div class="brand">
        <h1>Gateway Console</h1>
        <p>Visual Config & Monitor</p>
      </div>
      <nav>
        <router-link
          v-for="item in menus"
          :key="item.path"
          :to="item.path"
          class="nav-link"
          :class="{ active: current === item.path }"
        >
          {{ item.label }}
        </router-link>
      </nav>
      <button class="danger" @click="logout">退出登录</button>
    </aside>
    <main class="main-panel">
      <router-view />
    </main>
  </div>
</template>
