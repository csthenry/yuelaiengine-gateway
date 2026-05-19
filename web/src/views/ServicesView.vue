<script setup>
import { onMounted, ref } from 'vue'
import { deleteService, getServices, upsertService } from '../api/admin'

const loading = ref(false)
const saving = ref(false)
const error = ref('')
const services = ref([])
const form = ref(resetForm())

function resetForm() {
  return {
    name: '',
    health_check_path: '/healthz',
    load_balancer: 'round_robin',
    instances_json: '[\n  {"url":"http://127.0.0.1:8081","weight":1}\n]'
  }
}

async function loadServices() {
  loading.value = true
  error.value = ''
  try {
    services.value = await getServices()
  } catch (e) {
    error.value = e.message || '加载服务配置失败'
  } finally {
    loading.value = false
  }
}

function editService(item) {
  form.value = {
    name: item.name || '',
    health_check_path: item.health_check_path || '/healthz',
    load_balancer: item.load_balancer || 'round_robin',
    instances_json: JSON.stringify(item.instances || [], null, 2)
  }
}

function parseInstances(raw) {
  let parsed = []
  try {
    parsed = JSON.parse(raw || '[]')
  } catch {
    throw new Error('instances JSON 格式错误')
  }
  if (!Array.isArray(parsed)) {
    throw new Error('instances 必须是 JSON 数组')
  }
  if (parsed.length === 0) {
    throw new Error('instances 不能为空')
  }

  return parsed.map((inst, idx) => {
    const url = String(inst?.url || '').trim()
    const weight = Number(inst?.weight)
    if (!url) {
      throw new Error(`instances[${idx}].url 不能为空`)
    }
    if (!Number.isFinite(weight) || weight <= 0 || !Number.isInteger(weight)) {
      throw new Error(`instances[${idx}].weight 必须是正整数`)
    }
    return { url, weight }
  })
}

async function submit() {
  saving.value = true
  error.value = ''
  try {
    const name = form.value.name.trim()
    const healthCheckPath = form.value.health_check_path.trim()
    const loadBalancer = form.value.load_balancer.trim() || 'round_robin'

    if (!name) {
      throw new Error('name 不能为空')
    }
    if (!healthCheckPath || !healthCheckPath.startsWith('/')) {
      throw new Error('health_check_path 必须以 / 开头')
    }

    const instances = parseInstances(form.value.instances_json)

    await upsertService({
      name,
      health_check_path: healthCheckPath,
      load_balancer: loadBalancer,
      instances
    })

    form.value = resetForm()
    await loadServices()
  } catch (e) {
    error.value = e.message || '保存失败'
  } finally {
    saving.value = false
  }
}

async function remove(name) {
  if (!window.confirm(`确认删除服务 ${name} ?`)) {
    return
  }
  try {
    await deleteService(name)
    await loadServices()
  } catch (e) {
    error.value = e.message || '删除失败'
  }
}

onMounted(loadServices)
</script>

<template>
  <section>
    <header class="section-header">
      <h2>服务配置</h2>
      <button @click="loadServices" :disabled="loading">{{ loading ? '刷新中...' : '刷新列表' }}</button>
    </header>

    <p v-if="error" class="error-text">{{ error }}</p>

    <article class="panel">
      <h3>新增 / 编辑服务</h3>
      <div class="form-grid">
        <label>name<input v-model="form.name" placeholder="service-a-canary" /></label>
        <label>health_check_path<input v-model="form.health_check_path" placeholder="/healthz" /></label>
        <label>load_balancer<input v-model="form.load_balancer" placeholder="round_robin" /></label>
      </div>
      <label>instances(JSON 数组)<textarea v-model="form.instances_json" rows="8"></textarea></label>
      <div class="actions">
        <button @click="submit" :disabled="saving">{{ saving ? '提交中...' : '提交 upsert' }}</button>
        <button class="ghost" @click="form = resetForm()">清空</button>
      </div>
    </article>

    <article class="panel">
      <h3>当前服务</h3>
      <table>
        <thead>
          <tr>
            <th>name</th>
            <th>health_check_path</th>
            <th>load_balancer</th>
            <th>instances</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="item in services" :key="item.name">
            <td>{{ item.name }}</td>
            <td>{{ item.health_check_path || '-' }}</td>
            <td>{{ item.load_balancer || '-' }}</td>
            <td><pre>{{ JSON.stringify(item.instances || []) }}</pre></td>
            <td>
              <div class="row-actions">
                <button class="ghost" @click="editService(item)">编辑</button>
                <button class="danger" @click="remove(item.name)">删除</button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </article>
  </section>
</template>
