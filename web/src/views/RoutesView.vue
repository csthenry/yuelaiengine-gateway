<script setup>
import { onMounted, ref } from 'vue'
import { deleteRoute, getRoutes, upsertRoute } from '../api/admin'

const loading = ref(false)
const saving = ref(false)
const error = ref('')
const routes = ref([])
const form = ref(resetForm())

function resetForm() {
  return {
    path_prefix: '',
    path: '',
    service_name: '',
    methods: '',
    requires_auth: false,
    hash_on: '',
    ab_header: '',
    traffic_weights_json: '{}',
    ab_variants_json: '{}',
    plugins_json: '[]'
  }
}

async function loadRoutes() {
  loading.value = true
  error.value = ''
  try {
    routes.value = await getRoutes()
  } catch (e) {
    error.value = e.message || '加载路由失败'
  } finally {
    loading.value = false
  }
}

function editRoute(item) {
  form.value = {
    path_prefix: item.path_prefix || '',
    path: item.path || '',
    service_name: item.service_name || '',
    methods: (item.methods || []).join(','),
    requires_auth: Boolean(item.requires_auth),
    hash_on: item.hash_on || '',
    ab_header: item.ab_header || '',
    traffic_weights_json: JSON.stringify(item.traffic_weights || {}, null, 2),
    ab_variants_json: JSON.stringify(item.ab_variants || {}, null, 2),
    plugins_json: JSON.stringify(item.plugins || [], null, 2)
  }
}

function parseJSONMap(raw, fieldName) {
  let parsed = {}
  try {
    parsed = JSON.parse(raw || '{}')
  } catch (e) {
    throw new Error(`${fieldName} JSON 格式错误`)
  }
  if (!parsed || Array.isArray(parsed) || typeof parsed !== 'object') {
    throw new Error(`${fieldName} 必须是 JSON 对象`)
  }
  return parsed
}

function sanitizeStringMap(input, fieldName) {
  const out = {}
  for (const [k, v] of Object.entries(input)) {
    const key = String(k || '').trim()
    const val = String(v || '').trim()
    if (!key || !val) {
      throw new Error(`${fieldName} 的 key/value 不能为空`)
    }
    out[key] = val
  }
  return out
}

function sanitizeWeightMap(input) {
  const out = {}
  for (const [k, v] of Object.entries(input)) {
    const key = String(k || '').trim()
    if (!key) {
      throw new Error('traffic_weights 的 service 名不能为空')
    }
    const num = Number(v)
    if (!Number.isFinite(num) || num <= 0 || !Number.isInteger(num)) {
      throw new Error(`traffic_weights.${key} 必须是正整数`)
    }
    out[key] = num
  }
  return out
}

async function submit() {
  saving.value = true
  error.value = ''
  try {
    const methods = form.value.methods
      .split(',')
      .map((x) => x.trim())
      .filter(Boolean)

    let plugins = []
    try {
      plugins = JSON.parse(form.value.plugins_json || '[]')
      if (!Array.isArray(plugins)) {
        throw new Error('plugins 必须是 JSON 数组')
      }
    } catch (e) {
      throw new Error(e.message || 'plugins JSON 格式错误')
    }

    const trafficWeightsRaw = parseJSONMap(form.value.traffic_weights_json, 'traffic_weights')
    const abVariantsRaw = parseJSONMap(form.value.ab_variants_json, 'ab_variants')
    const trafficWeights = sanitizeWeightMap(trafficWeightsRaw)
    const abVariants = sanitizeStringMap(abVariantsRaw, 'ab_variants')

    await upsertRoute({
      path_prefix: form.value.path_prefix || undefined,
      path: form.value.path || undefined,
      service_name: form.value.service_name,
      methods,
      requires_auth: form.value.requires_auth,
      hash_on: form.value.hash_on || undefined,
      ab_header: form.value.ab_header || undefined,
      traffic_weights: Object.keys(trafficWeights).length ? trafficWeights : undefined,
      ab_variants: Object.keys(abVariants).length ? abVariants : undefined,
      plugins
    })
    form.value = resetForm()
    await loadRoutes()
  } catch (e) {
    error.value = e.message || '保存失败'
  } finally {
    saving.value = false
  }
}

async function remove(item) {
  const key = item.path || item.path_prefix
  if (!window.confirm(`确认删除路由 ${key} ?`)) {
    return
  }

  try {
    if (item.path) {
      await deleteRoute({ path: item.path })
    } else {
      await deleteRoute({ path_prefix: item.path_prefix })
    }
    await loadRoutes()
  } catch (e) {
    error.value = e.message || '删除失败'
  }
}

onMounted(loadRoutes)
</script>

<template>
  <section>
    <header class="section-header">
      <h2>路由配置</h2>
      <button @click="loadRoutes" :disabled="loading">{{ loading ? '刷新中...' : '刷新列表' }}</button>
    </header>

    <p v-if="error" class="error-text">{{ error }}</p>

    <article class="panel">
      <h3>新增 / 编辑路由</h3>
      <div class="form-grid">
        <label>path_prefix<input v-model="form.path_prefix" placeholder="/service-a" /></label>
        <label>path<input v-model="form.path" placeholder="/healthz" /></label>
        <label>service_name<input v-model="form.service_name" placeholder="service-a" /></label>
        <label>methods(逗号分隔)<input v-model="form.methods" placeholder="GET,POST" /></label>
        <label>hash_on<input v-model="form.hash_on" placeholder="ip 或 header:X-Gray-Key" /></label>
        <label>ab_header<input v-model="form.ab_header" placeholder="X-Canary" /></label>
      </div>
      <label class="checkbox"><input type="checkbox" v-model="form.requires_auth" />requires_auth</label>
      <label>traffic_weights(JSON 对象)<textarea v-model="form.traffic_weights_json" rows="6"></textarea></label>
      <label>ab_variants(JSON 对象)<textarea v-model="form.ab_variants_json" rows="6"></textarea></label>
      <label>plugins(JSON 数组)<textarea v-model="form.plugins_json" rows="8"></textarea></label>
      <div class="actions">
        <button @click="submit" :disabled="saving">{{ saving ? '提交中...' : '提交 upsert' }}</button>
        <button class="ghost" @click="form = resetForm()">清空</button>
      </div>
    </article>

    <article class="panel">
      <h3>当前路由</h3>
      <table>
        <thead>
          <tr>
            <th>path_prefix</th>
            <th>path</th>
            <th>service</th>
            <th>methods</th>
            <th>hash_on</th>
            <th>traffic_weights</th>
            <th>ab_header</th>
            <th>ab_variants</th>
            <th>requires_auth</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="item in routes" :key="`${item.path || ''}|${item.path_prefix || ''}`">
            <td>{{ item.path_prefix || '-' }}</td>
            <td>{{ item.path || '-' }}</td>
            <td>{{ item.service_name }}</td>
            <td>{{ (item.methods || []).join(',') || '-' }}</td>
            <td>{{ item.hash_on || '-' }}</td>
            <td><pre>{{ Object.keys(item.traffic_weights || {}).length ? JSON.stringify(item.traffic_weights) : '-' }}</pre></td>
            <td>{{ item.ab_header || '-' }}</td>
            <td><pre>{{ Object.keys(item.ab_variants || {}).length ? JSON.stringify(item.ab_variants) : '-' }}</pre></td>
            <td>{{ item.requires_auth ? 'true' : 'false' }}</td>
            <td>
              <div class="row-actions">
                <button class="ghost" @click="editRoute(item)">编辑</button>
                <button class="danger" @click="remove(item)">删除</button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </article>
  </section>
</template>
