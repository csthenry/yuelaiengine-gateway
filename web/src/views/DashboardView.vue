<script setup>
import * as echarts from 'echarts/core'
import { BarChart, LineChart } from 'echarts/charts'
import { GridComponent, TooltipComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import {
  getCircuitStatus,
  getHealthStatus,
  getMetricsHistory,
  getMetricsSummary,
  resetCircuit
} from '../api/admin'

echarts.use([LineChart, BarChart, GridComponent, TooltipComponent, CanvasRenderer])

const loading = ref(false)
const error = ref('')
const metrics = ref(null)
const circuits = ref({})
const health = ref({})
const qpsMode = ref('qps_10s')
const qpsPoints = ref([])

const qpsChartRef = ref(null)
const latencyChartRef = ref(null)

let qpsChartInstance = null
let latencyChartInstance = null
let chartResizeObserver = null
let timer = null

const QPS_WINDOW_SIZE = 120
const QPS_SAMPLE_INTERVAL_MS = 5000

const cards = computed(() => {
  const m = metrics.value || {}
  return [
    { label: '总请求数', value: m.total_requests ?? 0 },
    { label: '最近10秒平均 QPS', value: formatFloat(m.qps_10s) },
    { label: '最近1分钟 QPS', value: formatFloat(m.qps_1m) },
    { label: 'P99 延迟(ms)', value: formatFloat(m.latency_p99_ms) },
    { label: '429 总量(限流器)', value: m.total_429 ?? 0 },
    { label: '5xx 总量', value: m.total_5xx ?? 0 },
    { label: '熔断打开次数', value: m.circuit_open_total ?? 0 }
  ]
})

const qpsSeries = computed(() => {
  if (qpsMode.value === 'qps_1m') {
    return qpsPoints.value.map((item) => item.qps1m)
  }
  return qpsPoints.value.map((item) => item.qps10s)
})

const qpsLabels = computed(() => {
  const total = qpsPoints.value.length
  if (total === 0) {
    return []
  }
  const latestTs = qpsPoints.value[qpsPoints.value.length - 1].ts
  const maxLabels = 6
  const step = Math.max(1, Math.floor((total - 1) / (maxLabels - 1)))

  return qpsPoints.value.map((item, idx) => {
    const isLast = idx === total - 1
    const isMajor = idx % step === 0
    if (!isLast && !isMajor) {
      return ''
    }
    const deltaSec = Math.max(0, Math.round((latestTs - item.ts) / 1000))
    return deltaSec === 0 ? 'now' : `-${deltaSec}s`
  })
})

const qpsStats = computed(() => {
  const values = qpsSeries.value
  if (values.length === 0) {
    return { latest: 0, max: 0 }
  }
  return {
    latest: values[values.length - 1],
    max: Math.max(...values, 0)
  }
})

const healthRows = computed(() => {
  const out = []
  for (const [service, instances] of Object.entries(health.value || {})) {
    for (const [instance, alive] of Object.entries(instances || {})) {
      out.push({ service, instance, alive })
    }
  }
  return out
})

const serviceNames = computed(() => {
  const set = new Set()
  for (const row of healthRows.value) {
    if (row.service) {
      set.add(row.service)
    }
  }
  return Array.from(set)
})

const circuitRows = computed(() => {
  const raw = circuits.value || {}
  const byService = new Map()

  for (const [serviceName, item] of Object.entries(raw)) {
    const normalized = {
      service_name: item?.service_name || item?.ServiceName || serviceName,
      state: item?.state || item?.State || 'closed',
      failure_count: item?.failure_count ?? item?.FailureCount ?? 0,
      success_count: item?.success_count ?? item?.SuccessCount ?? 0
    }
    byService.set(normalized.service_name, normalized)
  }

  for (const name of serviceNames.value) {
    if (!byService.has(name)) {
      byService.set(name, {
        service_name: name,
        state: 'closed',
        failure_count: 0,
        success_count: 0
      })
    }
  }

  return Array.from(byService.values()).sort((a, b) => a.service_name.localeCompare(b.service_name))
})

function normalizeLatencyHistogram() {
  const list = Array.isArray(metrics.value?.latency_histogram) ? metrics.value.latency_histogram : []
  const sorted = [...list]
    .map((item) => {
      const rawLe = String(item?.le ?? '')
      const isInf = rawLe === '+Inf' || rawLe.toLowerCase() === 'inf'
      const upper = isInf ? Infinity : Number(rawLe)
      return {
        rawLe,
        upper,
        cumulative: Number(item?.count ?? 0)
      }
    })
    .filter((item) => Number.isFinite(item.upper) || item.upper === Infinity)
    .sort((a, b) => a.upper - b.upper)

  let prevCum = 0
  let prevUpper = 0
  return sorted.map((item) => {
    const current = Math.max(item.cumulative, prevCum)
    const bucketCount = current - prevCum
    const label = item.upper === Infinity ? `>${prevUpper}ms` : `<=${item.upper}ms`
    prevCum = current
    prevUpper = Number.isFinite(item.upper) ? item.upper : prevUpper
    return { label, count: bucketCount }
  })
}

function initCharts() {
  if (qpsChartRef.value && !qpsChartInstance) {
    qpsChartInstance = echarts.init(qpsChartRef.value)
  }
  if (latencyChartRef.value && !latencyChartInstance) {
    latencyChartInstance = echarts.init(latencyChartRef.value)
  }
}

function renderQPSChart() {
  if (!qpsChartInstance) {
    return
  }

  const values = qpsSeries.value
  const labels = qpsLabels.value
  const seriesName = qpsMode.value === 'qps_1m' ? 'QPS(1m)' : 'QPS(10s)'

  qpsChartInstance.setOption(
    {
      animationDuration: 250,
      tooltip: { trigger: 'axis' },
      grid: { left: 56, right: 20, top: 52, bottom: 36 },
      xAxis: {
        type: 'category',
        data: labels,
        boundaryGap: false,
        axisTick: { show: false },
        axisLabel: { color: '#64748b' },
        axisLine: { lineStyle: { color: '#cbd5e1' } }
      },
      yAxis: {
        type: 'value',
        name: 'QPS',
        min: 0,
        axisLabel: { color: '#64748b' },
        splitLine: { lineStyle: { color: '#e2e8f0' } }
      },
      series: [
        {
          name: seriesName,
          type: 'line',
          smooth: true,
          symbol: 'circle',
          symbolSize: 6,
          data: values,
          lineStyle: { width: 2.5, color: '#0f766e' },
          areaStyle: { color: 'rgba(15, 118, 110, 0.16)' },
          itemStyle: { color: '#0f766e' }
        }
      ]
    },
    true
  )
}

function renderLatencyChart() {
  if (!latencyChartInstance) {
    return
  }

  const buckets = normalizeLatencyHistogram()
  latencyChartInstance.setOption(
    {
      animationDuration: 250,
      tooltip: { trigger: 'axis' },
      grid: { left: 56, right: 20, top: 52, bottom: 48 },
      xAxis: {
        type: 'category',
        data: buckets.map((b) => b.label),
        axisLabel: {
          color: '#64748b',
          rotate: buckets.length > 6 ? 20 : 0
        },
        axisLine: { lineStyle: { color: '#cbd5e1' } }
      },
      yAxis: {
        type: 'value',
        name: '请求数',
        min: 0,
        axisLabel: { color: '#64748b' },
        splitLine: { lineStyle: { color: '#e2e8f0' } }
      },
      series: [
        {
          name: '请求数',
          type: 'bar',
          barMaxWidth: 48,
          data: buckets.map((b) => b.count),
          itemStyle: {
            color: '#14b8a6',
            borderRadius: [6, 6, 0, 0]
          }
        }
      ]
    },
    true
  )
}

function renderCharts() {
  renderQPSChart()
  renderLatencyChart()
}

function resizeCharts() {
  qpsChartInstance?.resize()
  latencyChartInstance?.resize()
}

async function loadHistorySeries() {
  const history = await getMetricsHistory(QPS_WINDOW_SIZE)
  const records = Array.isArray(history?.records) ? history.records : []
  const points = records
    .map((record) => {
      const ts = Date.parse(record?.timestamp || '')
      const qps1m = Number(record?.metrics?.qps_1m ?? 0)
      const qps10s = Number(record?.metrics?.qps_10s ?? 0)
      if (!Number.isFinite(ts) || !Number.isFinite(qps1m) || !Number.isFinite(qps10s)) {
        return null
      }
      return { ts, qps1m, qps10s }
    })
    .filter(Boolean)
    .sort((a, b) => a.ts - b.ts)

  if (points.length > 0) {
    qpsPoints.value = points.slice(-QPS_WINDOW_SIZE)
  }
}

function appendQpsSample(metricData) {
  const ts = Date.parse(metricData?.timestamp || '')
  const qps1m = Number(metricData?.qps_1m ?? 0)
  const qps10s = Number(metricData?.qps_10s ?? 0)
  if (!Number.isFinite(ts) || !Number.isFinite(qps1m) || !Number.isFinite(qps10s)) {
    return
  }

  const next = [...qpsPoints.value]
  const point = { ts, qps1m, qps10s }
  if (next.length > 0 && next[next.length - 1].ts === ts) {
    next[next.length - 1] = point
  } else {
    next.push(point)
  }
  qpsPoints.value = next.slice(-QPS_WINDOW_SIZE)
}

function setQpsMode(mode) {
  if (mode !== 'qps_10s' && mode !== 'qps_1m') {
    return
  }
  if (qpsMode.value === mode) {
    return
  }
  qpsMode.value = mode
  renderQPSChart()
}

async function loadData() {
  loading.value = true
  error.value = ''
  try {
    const [metricData, healthData, circuitData] = await Promise.all([
      getMetricsSummary(),
      getHealthStatus(),
      getCircuitStatus()
    ])
    metrics.value = metricData
    health.value = healthData
    circuits.value = circuitData

    appendQpsSample(metricData)

    await nextTick()
    renderCharts()
  } catch (e) {
    error.value = e.message || '加载失败'
  } finally {
    loading.value = false
  }
}

async function handleResetCircuit(service) {
  if (!window.confirm(`确认重置 ${service} 的熔断状态？`)) {
    return
  }
  try {
    await resetCircuit(service)
    await loadData()
  } catch (e) {
    error.value = e.message || '重置失败'
  }
}

function formatFloat(v) {
  const n = Number(v || 0)
  return Number.isFinite(n) ? n.toFixed(2) : '0.00'
}

onMounted(async () => {
  await nextTick()
  initCharts()
  renderCharts()
  resizeCharts()

  if (window.ResizeObserver) {
    chartResizeObserver = new window.ResizeObserver(() => {
      resizeCharts()
    })
    if (qpsChartRef.value) {
      chartResizeObserver.observe(qpsChartRef.value)
    }
    if (latencyChartRef.value) {
      chartResizeObserver.observe(latencyChartRef.value)
    }
  }
  window.addEventListener('resize', resizeCharts)

  try {
    await loadHistorySeries()
  } catch (e) {
    console.warn('load history failed', e)
  }
  await loadData()
  timer = window.setInterval(loadData, QPS_SAMPLE_INTERVAL_MS)
})

onBeforeUnmount(() => {
  if (timer) {
    window.clearInterval(timer)
    timer = null
  }

  window.removeEventListener('resize', resizeCharts)

  if (chartResizeObserver) {
    chartResizeObserver.disconnect()
    chartResizeObserver = null
  }

  if (qpsChartInstance) {
    qpsChartInstance.dispose()
    qpsChartInstance = null
  }
  if (latencyChartInstance) {
    latencyChartInstance.dispose()
    latencyChartInstance = null
  }
})
</script>

<template>
  <section>
    <header class="section-header">
      <h2>监控总览</h2>
      <button @click="loadData" :disabled="loading">{{ loading ? '刷新中...' : '立即刷新' }}</button>
    </header>

    <p v-if="error" class="error-text">{{ error }}</p>

    <div class="card-grid">
      <article class="card" v-for="item in cards" :key="item.label">
        <p class="card-label">{{ item.label }}</p>
        <p class="card-value">{{ item.value }}</p>
      </article>
    </div>

    <div class="panel-grid">
      <article class="panel panel-chart">
        <div class="panel-head">
          <h3>QPS 趋势（5秒刷新）</h3>
          <div class="segmented">
            <button
              class="ghost"
              :class="{ active: qpsMode === 'qps_10s' }"
              @click="setQpsMode('qps_10s')"
            >
              10s
            </button>
            <button
              class="ghost"
              :class="{ active: qpsMode === 'qps_1m' }"
              @click="setQpsMode('qps_1m')"
            >
              1m
            </button>
          </div>
        </div>
        <div class="sparkline-meta">
          <span>当前: {{ formatFloat(qpsStats.latest) }}</span>
          <span>峰值: {{ formatFloat(qpsStats.max) }}</span>
        </div>
        <div ref="qpsChartRef" class="chart-canvas"></div>
      </article>

      <article class="panel panel-chart">
        <h3>延迟分布直方图</h3>
        <p class="help-text">
          显示每个延迟区间内新增的请求数（由累计桶转换而来），更直观看出延迟分布。
        </p>
        <div ref="latencyChartRef" class="chart-canvas"></div>
      </article>
    </div>

    <div class="panel-grid">
      <article class="panel">
        <h3>服务健康矩阵</h3>
        <table>
          <thead>
            <tr><th>服务</th><th>实例</th><th>状态</th></tr>
          </thead>
          <tbody>
            <tr v-for="row in healthRows" :key="`${row.service}-${row.instance}`">
              <td>{{ row.service }}</td>
              <td>{{ row.instance }}</td>
              <td><span :class="row.alive ? 'ok' : 'bad'">{{ row.alive ? 'healthy' : 'unhealthy' }}</span></td>
            </tr>
          </tbody>
        </table>
      </article>

      <article class="panel">
        <h3>熔断状态</h3>
        <table>
          <thead>
            <tr><th>服务</th><th>状态</th><th>失败</th><th>成功</th><th>操作</th></tr>
          </thead>
          <tbody>
            <tr v-for="item in circuitRows" :key="item.service_name">
              <td>{{ item.service_name }}</td>
              <td>{{ item.state }}</td>
              <td>{{ item.failure_count }}</td>
              <td>{{ item.success_count }}</td>
              <td>
                <button class="ghost" @click="handleResetCircuit(item.service_name)">重置</button>
              </td>
            </tr>
          </tbody>
        </table>
      </article>
    </div>
  </section>
</template>
