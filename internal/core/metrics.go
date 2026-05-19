package core

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

type latencyBucket struct {
	le    float64
	count uint64
}

type metricsSummary struct {
	Timestamp        time.Time             `json:"timestamp"`
	TotalRequests    uint64                `json:"total_requests"`
	Total4xx         uint64                `json:"total_4xx"`
	Total5xx         uint64                `json:"total_5xx"`
	Total429         uint64                `json:"total_429"`
	QPS10s           float64               `json:"qps_10s"`
	QPS1m            float64               `json:"qps_1m"`
	LatencyP99Ms     float64               `json:"latency_p99_ms"`
	LatencyCount     uint64                `json:"latency_count"`
	LatencySumMs     float64               `json:"latency_sum_ms"`
	LatencyHistogram []latencyBucketMetric `json:"latency_histogram"`
	CircuitOpenTotal uint64                `json:"circuit_open_total"`
	UptimeSeconds    float64               `json:"uptime_seconds"`
}

type latencyBucketMetric struct {
	Le    string `json:"le"`
	Count uint64 `json:"count"`
}

type metricsCollector struct {
	mu sync.Mutex

	startTime time.Time

	totalRequests uint64
	total4xx      uint64
	total5xx      uint64
	total429      uint64

	latencyBuckets []latencyBucket
	latencyCount   uint64
	latencySum     float64

	secBucketSize int
	secCounts     []uint64
	secEpoch      []int64
}

func newMetricsCollector() *metricsCollector {
	bounds := []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 2000, 5000}
	buckets := make([]latencyBucket, 0, len(bounds)+1)
	for _, b := range bounds {
		buckets = append(buckets, latencyBucket{le: b})
	}
	buckets = append(buckets, latencyBucket{le: -1}) // +Inf

	now := time.Now()
	return &metricsCollector{
		startTime:      now,
		latencyBuckets: buckets,
		secBucketSize:  60,
		secCounts:      make([]uint64, 60),
		secEpoch:       make([]int64, 60),
	}
}

func (m *metricsCollector) Observe(statusCode int, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.totalRequests++
	if statusCode >= 400 && statusCode < 500 {
		m.total4xx++
	}
	if statusCode >= 500 {
		m.total5xx++
	}
	if statusCode == 429 {
		m.total429++
	}

	latencyMs := float64(duration.Milliseconds())
	if latencyMs < 0 {
		latencyMs = 0
	}
	m.latencyCount++
	m.latencySum += latencyMs

	for i := range m.latencyBuckets {
		if m.latencyBuckets[i].le < 0 || latencyMs <= m.latencyBuckets[i].le {
			m.latencyBuckets[i].count++
		}
	}

	nowSec := time.Now().Unix()
	idx := int(nowSec % int64(m.secBucketSize))
	if m.secEpoch[idx] != nowSec {
		m.secEpoch[idx] = nowSec
		m.secCounts[idx] = 0
	}
	m.secCounts[idx]++
}

func (m *metricsCollector) oneMinuteQPS(now time.Time) float64 {
	return m.windowQPS(now, 60)
}

func (m *metricsCollector) tenSecondQPS(now time.Time) float64 {
	return m.windowQPS(now, 10)
}

func (m *metricsCollector) windowQPS(now time.Time, windowSec int) float64 {
	if windowSec <= 0 {
		return 0
	}
	if windowSec > m.secBucketSize {
		windowSec = m.secBucketSize
	}
	nowSec := now.Unix()
	windowStart := nowSec - int64(windowSec) + 1
	var total uint64
	for i := 0; i < m.secBucketSize; i++ {
		epoch := m.secEpoch[i]
		if epoch >= windowStart && epoch <= nowSec {
			total += m.secCounts[i]
		}
	}
	return float64(total) / float64(windowSec)
}

func (m *metricsCollector) p99LatencyMs() float64 {
	if m.latencyCount == 0 {
		return 0
	}
	target := float64(m.latencyCount) * 0.99
	for _, b := range m.latencyBuckets {
		if float64(b.count) >= target {
			if b.le < 0 {
				return 5000
			}
			return b.le
		}
	}
	return 0
}

func (m *metricsCollector) RenderPrometheus(circuitOpenCount uint64) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	summary := m.snapshotLocked(circuitOpenCount)

	var sb strings.Builder
	writeHelpType := func(name, help, typ string) {
		sb.WriteString("# HELP ")
		sb.WriteString(name)
		sb.WriteString(" ")
		sb.WriteString(help)
		sb.WriteString("\n# TYPE ")
		sb.WriteString(name)
		sb.WriteString(" ")
		sb.WriteString(typ)
		sb.WriteString("\n")
	}

	writeHelpType("gateway_requests_total", "Total requests handled by gateway", "counter")
	sb.WriteString(fmt.Sprintf("gateway_requests_total %d\n", summary.TotalRequests))

	writeHelpType("gateway_responses_4xx_total", "Total 4xx responses", "counter")
	sb.WriteString(fmt.Sprintf("gateway_responses_4xx_total %d\n", summary.Total4xx))

	writeHelpType("gateway_responses_5xx_total", "Total 5xx responses", "counter")
	sb.WriteString(fmt.Sprintf("gateway_responses_5xx_total %d\n", summary.Total5xx))

	writeHelpType("gateway_responses_429_total", "Total 429 responses", "counter")
	sb.WriteString(fmt.Sprintf("gateway_responses_429_total %d\n", summary.Total429))

	writeHelpType("gateway_latency_ms", "Gateway request latency in milliseconds", "histogram")
	for _, b := range summary.LatencyHistogram {
		sb.WriteString(fmt.Sprintf("gateway_latency_ms_bucket{le=\"%s\"} %d\n", b.Le, b.Count))
	}
	sb.WriteString(fmt.Sprintf("gateway_latency_ms_sum %s\n", trimFloat(summary.LatencySumMs)))
	sb.WriteString(fmt.Sprintf("gateway_latency_ms_count %d\n", summary.LatencyCount))

	writeHelpType("gateway_qps_1m", "Average QPS in last 1 minute", "gauge")
	sb.WriteString(fmt.Sprintf("gateway_qps_1m %s\n", trimFloat(summary.QPS1m)))
	writeHelpType("gateway_qps_10s", "Average QPS in last 10 seconds", "gauge")
	sb.WriteString(fmt.Sprintf("gateway_qps_10s %s\n", trimFloat(summary.QPS10s)))

	writeHelpType("gateway_latency_p99_ms", "Approximate P99 request latency in milliseconds", "gauge")
	sb.WriteString(fmt.Sprintf("gateway_latency_p99_ms %s\n", trimFloat(summary.LatencyP99Ms)))

	writeHelpType("gateway_circuit_open_total", "Total circuit breaker open transitions", "counter")
	sb.WriteString(fmt.Sprintf("gateway_circuit_open_total %d\n", summary.CircuitOpenTotal))

	writeHelpType("gateway_uptime_seconds", "Gateway process uptime in seconds", "gauge")
	sb.WriteString(fmt.Sprintf("gateway_uptime_seconds %s\n", trimFloat(summary.UptimeSeconds)))

	return sb.String()
}

func (m *metricsCollector) Snapshot(circuitOpenCount uint64) metricsSummary {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.snapshotLocked(circuitOpenCount)
}

func (m *metricsCollector) snapshotLocked(circuitOpenCount uint64) metricsSummary {
	now := time.Now()
	buckets := m.sortedBuckets()
	hist := make([]latencyBucketMetric, 0, len(buckets))
	for _, b := range buckets {
		le := "+Inf"
		if b.le >= 0 {
			le = trimFloat(b.le)
		}
		hist = append(hist, latencyBucketMetric{
			Le:    le,
			Count: b.count,
		})
	}

	return metricsSummary{
		Timestamp:        now,
		TotalRequests:    m.totalRequests,
		Total4xx:         m.total4xx,
		Total5xx:         m.total5xx,
		Total429:         m.total429,
		QPS10s:           m.tenSecondQPS(now),
		QPS1m:            m.oneMinuteQPS(now),
		LatencyP99Ms:     m.p99LatencyMs(),
		LatencyCount:     m.latencyCount,
		LatencySumMs:     m.latencySum,
		LatencyHistogram: hist,
		CircuitOpenTotal: circuitOpenCount,
		UptimeSeconds:    now.Sub(m.startTime).Seconds(),
	}
}

func (m *metricsCollector) sortedBuckets() []latencyBucket {
	out := make([]latencyBucket, len(m.latencyBuckets))
	copy(out, m.latencyBuckets)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].le < 0 {
			return false
		}
		if out[j].le < 0 {
			return true
		}
		return out[i].le < out[j].le
	})
	return out
}

func trimFloat(v float64) string {
	s := fmt.Sprintf("%.6f", v)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	if s == "" || s == "-" {
		return "0"
	}
	return s
}
