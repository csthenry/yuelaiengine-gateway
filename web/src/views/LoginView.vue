<script setup>
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth'

const token = ref('')
const error = ref('')
const router = useRouter()
const auth = useAuthStore()

function submit() {
  const next = token.value.trim()
  if (!next) {
    error.value = '请输入 admin token'
    return
  }
  auth.setToken(next)
  router.push('/dashboard')
}
</script>

<template>
  <div class="login-wrap">
    <div class="login-shell">
      <section class="login-intro">
        <p class="login-kicker">Gateway Management</p>
        <h1>网关控制台</h1>
        <p class="login-desc">
          统一管理路由、限流、灰度与配置发布。登录后可在同一界面完成可视化配置与实时监控。
        </p>
        <ul class="login-points">
          <li>路由/服务/限流规则在线变更</li>
          <li>配置 Dry Run、版本回滚、落盘发布</li>
          <li>QPS、延迟与熔断状态实时观测</li>
        </ul>
      </section>

      <form class="login-card" @submit.prevent="submit">
        <div class="login-card-head">
          <h2>管理员登录</h2>
          <p>请输入配置文件中的 <code>admin.token</code></p>
        </div>
        <label for="token">Admin Token</label>
        <input
          id="token"
          v-model="token"
          type="password"
          autocomplete="current-password"
          placeholder="admin-secret"
        />
        <p v-if="error" class="error-text">{{ error }}</p>
        <button type="submit">进入控制台</button>
      </form>
    </div>
  </div>
</template>
