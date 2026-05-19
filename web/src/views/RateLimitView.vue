<script setup>
import { onMounted, ref } from 'vue'
import {
  deleteRateLimitRule,
  getRateLimitRules,
  upsertRateLimitRule
} from '../api/admin'

const loading = ref(false)
const saving = ref(false)
const error = ref('')
const rules = ref([])
const form = ref({
  name: '',
  type: 'memory_token_bucket',
  capacity: 100,
  refill_rate: 50
})

async function loadRules() {
  loading.value = true
  error.value = ''
  try {
    rules.value = await getRateLimitRules()
  } catch (e) {
    error.value = e.message || '加载失败'
  } finally {
    loading.value = false
  }
}

function editRule(rule) {
  form.value = {
    name: rule.name,
    type: rule.type,
    capacity: rule.tokenBucket?.capacity ?? 0,
    refill_rate: rule.tokenBucket?.refillRate ?? 0
  }
}

async function submit() {
  saving.value = true
  error.value = ''
  try {
    await upsertRateLimitRule({
      name: form.value.name,
      type: form.value.type,
      capacity: Number(form.value.capacity),
      refill_rate: Number(form.value.refill_rate)
    })
    await loadRules()
  } catch (e) {
    error.value = e.message || '保存失败'
  } finally {
    saving.value = false
  }
}

async function remove(name) {
  if (!window.confirm(`确认删除规则 ${name} ?`)) {
    return
  }
  try {
    await deleteRateLimitRule(name)
    await loadRules()
  } catch (e) {
    error.value = e.message || '删除失败'
  }
}

onMounted(loadRules)
</script>

<template>
  <section>
    <header class="section-header">
      <h2>限流规则</h2>
      <button @click="loadRules" :disabled="loading">{{ loading ? '刷新中...' : '刷新列表' }}</button>
    </header>

    <p v-if="error" class="error-text">{{ error }}</p>

    <article class="panel">
      <h3>新增 / 编辑规则</h3>
      <div class="form-grid">
        <label>name<input v-model="form.name" placeholder="default-ip-limit" /></label>
        <label>type<input v-model="form.type" /></label>
        <label>capacity<input v-model.number="form.capacity" type="number" min="1" /></label>
        <label>refill_rate<input v-model.number="form.refill_rate" type="number" min="0" /></label>
      </div>
      <button @click="submit" :disabled="saving">{{ saving ? '提交中...' : '提交 upsert' }}</button>
    </article>

    <article class="panel">
      <h3>当前规则</h3>
      <table>
        <thead>
          <tr><th>name</th><th>type</th><th>capacity</th><th>refillRate</th><th>操作</th></tr>
        </thead>
        <tbody>
          <tr v-for="rule in rules" :key="rule.name">
            <td>{{ rule.name }}</td>
            <td>{{ rule.type }}</td>
            <td>{{ rule.tokenBucket?.capacity }}</td>
            <td>{{ rule.tokenBucket?.refillRate }}</td>
            <td>
              <div class="row-actions">
                <button class="ghost" @click="editRule(rule)">编辑</button>
                <button class="danger" @click="remove(rule.name)">删除</button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </article>
  </section>
</template>
