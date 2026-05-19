#!/usr/bin/env bash

set -euo pipefail

GATEWAY_ADDR="${GATEWAY_ADDR:-http://127.0.0.1:9000}"
UPSTREAM_ADDR="${UPSTREAM_ADDR:-http://127.0.0.1:8081}"
ADMIN_TOKEN="${ADMIN_TOKEN:-admin-secret}"
OUT_DIR="${OUT_DIR:-./logs/benchmarks/$(date +%Y%m%d-%H%M%S)}"

mkdir -p "${OUT_DIR}"

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing command: $1" >&2
    exit 1
  fi
}

require_cmd curl
require_cmd ab

retry_http_200() {
  local url="$1"
  local max_try="${2:-30}"
  local sleep_s="${3:-1}"
  local code

  for _ in $(seq 1 "${max_try}"); do
    code="$(curl -s -o /dev/null -w '%{http_code}' "${url}" || true)"
    if [[ "${code}" == "200" ]]; then
      return 0
    fi
    sleep "${sleep_s}"
  done

  echo "endpoint not ready: ${url}" >&2
  return 1
}

echo "[bench] output dir: ${OUT_DIR}"

retry_http_200 "${GATEWAY_ADDR}/healthz"
curl -sS "${GATEWAY_ADDR}/healthz" >"${OUT_DIR}/gateway-health.json"

# Create a no-plugin route so we can benchmark gateway forwarding overhead
# without auth/ratelimit/circuitbreaker noise.
curl -fsS -X POST "${GATEWAY_ADDR}/admin/routes/upsert" \
  -H "X-Admin-Token: ${ADMIN_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{"route":{"path_prefix":"/dynamic","service_name":"service-a","plugins":[],"methods":["GET"]}}' \
  >"${OUT_DIR}/admin-upsert-dynamic.json"

for _ in $(seq 1 30); do
  curl -s "${GATEWAY_ADDR}/dynamic/perf" >/dev/null || true
done

retry_http_200 "${GATEWAY_ADDR}/dynamic/perf"

ab -n 1000 -c 100 "${UPSTREAM_ADDR}/perf" >"${OUT_DIR}/ab-upstream-1000-100.txt"
ab -n 1000 -c 100 "${GATEWAY_ADDR}/dynamic/perf" >"${OUT_DIR}/ab-gateway-dynamic-1000-100.txt"
ab -n 1000 -c 100 "${GATEWAY_ADDR}/service-a/perf" >"${OUT_DIR}/ab-gateway-governed-1000-100.txt"

ab -n 5000 -c 200 "${UPSTREAM_ADDR}/perf" >"${OUT_DIR}/ab-upstream-5000-200.txt"
ab -n 5000 -c 200 "${GATEWAY_ADDR}/dynamic/perf" >"${OUT_DIR}/ab-gateway-dynamic-5000-200.txt"
ab -n 5000 -c 200 "${GATEWAY_ADDR}/service-a/perf" >"${OUT_DIR}/ab-gateway-governed-5000-200.txt"

{
  echo "system: $(uname -a)"
  if command -v sysctl >/dev/null 2>&1; then
    echo "cpu: $(sysctl -n machdep.cpu.brand_string 2>/dev/null || true)"
    echo "physical_cpu: $(sysctl -n hw.physicalcpu 2>/dev/null || true)"
    echo "logical_cpu: $(sysctl -n hw.logicalcpu 2>/dev/null || true)"
  fi
  if command -v sw_vers >/dev/null 2>&1; then
    echo "os:"
    sw_vers
  fi
  echo "go: $(go version 2>/dev/null || true)"
  echo "date: $(date '+%Y-%m-%d %H:%M:%S %z')"
} >"${OUT_DIR}/env.txt"

echo
echo "[bench] key metrics:"
for f in "${OUT_DIR}"/ab-*.txt; do
  echo "===== $(basename "${f}") ====="
  grep -E "Failed requests:|Non-2xx responses:|Requests per second:|Time per request:|  95%|  99%" "${f}" || true
  echo
done

echo "[bench] done"
