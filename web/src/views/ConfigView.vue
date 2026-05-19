<script setup>
import { onMounted, ref } from 'vue'
import {
  applyConfig,
  getConfigVersions,
  getCurrentConfig,
  rollbackConfig
} from '../api/admin'

const loading = ref(false)
const applying = ref(false)
const error = ref('')
const success = ref('')
const versionInfo = ref(null)
const versions = ref([])
const source = ref('web:config:apply')
const persistToFile = ref(false)
const persistPath = ref('')
const configText = ref('')

async function loadData() {
  loading.value = true
  error.value = ''
  success.value = ''
  try {
    const [current, versionData] = await Promise.all([getCurrentConfig(), getConfigVersions()])
    versionInfo.value = current.version || null
    configText.value = JSON.stringify(current.config || {}, null, 2)
    versions.value = versionData.history || []
  } catch (e) {
    error.value = e.message || '加载配置失败'
  } finally {
    loading.value = false
  }
}

function parseConfig() {
  try {
    return JSON.parse(configText.value)
  } catch {
    throw new Error('配置 JSON 格式错误')
  }
}

async function runDry() {
  applying.value = true
  error.value = ''
  success.value = ''
  try {
    await applyConfig(
      parseConfig(),
      true,
      source.value || 'web:dry-run',
      persistToFile.value,
      persistPath.value.trim()
    )
    success.value = 'Dry Run 校验通过'
  } catch (e) {
    error.value = e.message || 'Dry Run 失败'
  } finally {
    applying.value = false
  }
}

async function applyNow() {
  if (!window.confirm('确认应用当前配置？该操作会立即生效。')) {
    return
  }
  applying.value = true
  error.value = ''
  success.value = ''
  try {
    await applyConfig(
      parseConfig(),
      false,
      source.value || 'web:config:apply',
      persistToFile.value,
      persistPath.value.trim()
    )
    success.value = persistToFile.value ? '配置已应用并落盘' : '配置已应用（仅内存）'
    await loadData()
  } catch (e) {
    error.value = e.message || '应用失败'
  } finally {
    applying.value = false
  }
}

async function rollback(version) {
  if (!window.confirm(`确认回滚到版本 ${version} ?`)) {
    return
  }
  error.value = ''
  success.value = ''
  try {
    await rollbackConfig(version)
    success.value = `已回滚到 ${version}`
    await loadData()
  } catch (e) {
    error.value = e.message || '回滚失败'
  }
}

onMounted(loadData)
</script>

<template>
  <section>
    <header class="section-header">
      <h2>配置版本与发布</h2>
      <button @click="loadData" :disabled="loading">{{ loading ? '刷新中...' : '刷新数据' }}</button>
    </header>

    <p v-if="error" class="error-text">{{ error }}</p>
    <p v-if="success" class="ok-text">{{ success }}</p>

    <article class="panel">
      <h3>当前版本</h3>
      <p>版本号: <strong>{{ versionInfo?.version || '-' }}</strong></p>
      <p>来源: <strong>{{ versionInfo?.source || '-' }}</strong></p>
      <p>时间: <strong>{{ versionInfo?.created_at || '-' }}</strong></p>
    </article>

    <article class="panel">
      <h3>配置编辑</h3>
      <label>source<input v-model="source" placeholder="web:config:apply" /></label>
      <label class="checkbox">
        <input type="checkbox" v-model="persistToFile" />
        落盘发布（关闭时仅内存生效）
      </label>
      <label v-if="persistToFile">
        persist_path(可选)
        <input v-model="persistPath" placeholder="./config/config.yml（留空则用服务默认路径）" />
      </label>
      <label>config(JSON)<textarea v-model="configText" rows="18"></textarea></label>
      <div class="actions">
        <button @click="runDry" :disabled="applying">{{ applying ? '处理中...' : 'Dry Run 校验' }}</button>
        <button class="danger" @click="applyNow" :disabled="applying">{{ applying ? '处理中...' : '应用配置' }}</button>
      </div>
    </article>

    <article class="panel">
      <h3>版本历史</h3>
      <table>
        <thead>
          <tr><th>version</th><th>source</th><th>created_at</th><th>操作</th></tr>
        </thead>
        <tbody>
          <tr v-for="item in versions" :key="item.version">
            <td>{{ item.version }}</td>
            <td>{{ item.source }}</td>
            <td>{{ item.created_at }}</td>
            <td><button class="ghost" @click="rollback(item.version)">回滚到此版本</button></td>
          </tr>
        </tbody>
      </table>
    </article>
  </section>
</template>
