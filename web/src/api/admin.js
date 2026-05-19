import http from './http'

function unwrap(res) {
  if (res?.data?.data !== undefined) {
    return res.data.data
  }
  return res.data
}

export async function getConfigVersions() {
  return unwrap(await http.get('/admin/config/versions'))
}

export async function getCurrentConfig() {
  return unwrap(await http.get('/admin/config/current'))
}

export async function getRoutes() {
  const raw = unwrap(await http.get('/admin/routes'))
  const list = Array.isArray(raw) ? raw : []
  return list
    .filter(Boolean)
    .map((item) => ({
      path_prefix: item.path_prefix ?? item.PathPrefix ?? '',
      path: item.path ?? item.Path ?? '',
      service_name: item.service_name ?? item.ServiceName ?? '',
      plugins: item.plugins ?? item.Plugins ?? [],
      methods: item.methods ?? item.Methods ?? [],
      requires_auth: item.requires_auth ?? item.RequiresAuth ?? false,
      hash_on: item.hash_on ?? item.hashOn ?? item.HashOn ?? '',
      ab_header: item.ab_header ?? item.abHeader ?? item.ABHeader ?? '',
      ab_variants: item.ab_variants ?? item.abVariants ?? item.ABVariants ?? {},
      traffic_weights: item.traffic_weights ?? item.trafficWeights ?? item.TrafficWeights ?? {}
    }))
}

export async function upsertRoute(route) {
  return unwrap(await http.post('/admin/routes/upsert', { route }))
}

export async function deleteRoute(payload) {
  return unwrap(await http.post('/admin/routes/delete', payload))
}

export async function getServices() {
  const raw = unwrap(await http.get('/admin/services'))
  const list = Array.isArray(raw) ? raw : []
  return list
    .filter(Boolean)
    .map((item) => ({
      name: item.name ?? item.Name ?? '',
      health_check_path: item.health_check_path ?? item.healthCheckPath ?? item.HealthCheckPath ?? '',
      load_balancer: item.load_balancer ?? item.loadBalancer ?? item.LoadBalancer ?? '',
      instances: item.instances ?? item.Instances ?? []
    }))
}

export async function upsertService(service) {
  return unwrap(await http.post('/admin/services/upsert', { service }))
}

export async function deleteService(name) {
  return unwrap(await http.post('/admin/services/delete', { name }))
}

export async function getRateLimitRules() {
  const raw = unwrap(await http.get('/admin/ratelimit/rules'))
  const list = Array.isArray(raw) ? raw : []
  return list
    .filter(Boolean)
    .map((item) => {
      const tokenBucket = item.token_bucket ?? item.tokenBucket ?? item.TokenBucket ?? {}
      return {
        name: item.name ?? item.Name ?? '',
        type: item.type ?? item.Type ?? '',
        tokenBucket: {
          capacity: tokenBucket.capacity ?? tokenBucket.Capacity ?? 0,
          refillRate: tokenBucket.refillRate ?? tokenBucket.RefillRate ?? 0
        }
      }
    })
}

export async function upsertRateLimitRule(rule) {
  return unwrap(await http.post('/admin/ratelimit/rules/upsert', rule))
}

export async function deleteRateLimitRule(name) {
  return unwrap(await http.post('/admin/ratelimit/rules/delete', { name }))
}

export async function applyConfig(
  config,
  dryRun = false,
  source = 'web:config:apply',
  persist = false,
  persistPath = ''
) {
  const payload = { config, dry_run: dryRun, source, persist }
  if (persistPath) {
    payload.persist_path = persistPath
  }
  return unwrap(await http.post('/admin/config/apply', payload))
}

export async function rollbackConfig(version) {
  return unwrap(await http.post('/admin/config/rollback', { version }))
}

export async function getHealthStatus() {
  return unwrap(await http.get('/admin/health/status'))
}

export async function getMetricsSummary() {
  return unwrap(await http.get('/admin/metrics/summary'))
}

export async function getMetricsHistory(limit = 100) {
  return unwrap(await http.get('/admin/metrics/history', { params: { limit } }))
}

export async function getCircuitStatus() {
  const res = await http.get('/admin/circuit/status')
  return res.data?.circuits || res.data?.data?.circuits || {}
}

export async function resetCircuit(service) {
  return unwrap(await http.post(`/admin/circuit/reset?service=${encodeURIComponent(service)}`))
}
