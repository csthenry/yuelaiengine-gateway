package core

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"yuelaiengine/gateway/internal/config"
	"yuelaiengine/gateway/internal/testutil"
)

func newGatewayForAdminTest(t *testing.T) (*Gateway, *httptest.Server, *httptest.Server) {
	t.Helper()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK"))
		case "/hello":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("hello"))
		default:
			http.NotFound(w, r)
		}
	}))

	authSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK"))
		case "/validate":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"valid": true, "user_id": "u-admin", "role": "admin"})
		default:
			http.NotFound(w, r)
		}
	}))

	cfg := &config.GatewayConfig{
		Server: config.ServerConfig{Port: ":9000"},
		Admin:  config.AdminConfig{Token: "admin-secret"},
		HealthCheck: config.HealthCheckConfig{
			Interval: 20 * time.Millisecond,
			Timeout:  time.Second,
		},
		AuthService: config.AuthServiceConfig{ValidateURL: "http://auth-service/validate"},
		CircuitBreaker: config.CircuitBreakerConfig{
			FailureThreshold: 2,
			SuccessThreshold: 1,
			ResetTimeout:     time.Second,
		},
		RateLimiting: config.RateLimitingConfig{
			Rules: []config.RateLimiterRule{{
				Name: "default",
				Type: "memory_token_bucket",
				TokenBucket: config.TokenBucketSettings{
					Capacity:   10,
					RefillRate: 10,
				},
			}},
		},
		Services: map[string]config.ServiceConfig{
			"service-a": {
				Name:            "service-a",
				LoadBalancer:    "round_robin",
				HealthCheckPath: "/healthz",
				Instances:       []config.InstanceConfig{{URL: upstream.URL, Weight: 1}},
			},
			"auth-service": {
				Name:            "auth-service",
				LoadBalancer:    "round_robin",
				HealthCheckPath: "/healthz",
				Instances:       []config.InstanceConfig{{URL: authSrv.URL, Weight: 1}},
			},
		},
		Routes: []*config.RouteConfig{
			{PathPrefix: "/healthz", ServiceName: "all-services", HealthCheckScope: "all-services", Methods: []string{"GET"}},
			{PathPrefix: "/service-a", ServiceName: "service-a"},
		},
	}

	gw, err := NewGateway(cfg, &testutil.NoopLogger{})
	if err != nil {
		t.Fatalf("NewGateway() error = %v", err)
	}

	return gw, upstream, authSrv
}

func newAdminTestServer(gw *Gateway) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/admin/circuit/status", gw.AdminCircuitStatusHTTP)
	mux.HandleFunc("/admin/circuit/reset", gw.AdminCircuitResetHTTP)
	mux.HandleFunc("/admin/ratelimit/rules", gw.AdminRateLimitRuleListHTTP)
	mux.HandleFunc("/admin/ratelimit/rules/upsert", gw.AdminRateLimitRuleUpsertHTTP)
	mux.HandleFunc("/admin/ratelimit/rules/delete", gw.AdminRateLimitRuleDeleteHTTP)
	mux.HandleFunc("/admin/routes", gw.AdminRouteListHTTP)
	mux.HandleFunc("/admin/routes/upsert", gw.AdminRouteUpsertHTTP)
	mux.HandleFunc("/admin/routes/delete", gw.AdminRouteDeleteHTTP)
	mux.HandleFunc("/admin/services", gw.AdminServiceListHTTP)
	mux.HandleFunc("/admin/services/upsert", gw.AdminServiceUpsertHTTP)
	mux.HandleFunc("/admin/services/delete", gw.AdminServiceDeleteHTTP)
	mux.HandleFunc("/admin/config/versions", gw.AdminConfigVersionsHTTP)
	mux.HandleFunc("/admin/config/current", gw.AdminConfigCurrentHTTP)
	mux.HandleFunc("/admin/config/rollback", gw.AdminConfigRollbackHTTP)
	mux.HandleFunc("/admin/config/apply", gw.AdminConfigApplyHTTP)
	mux.HandleFunc("/admin/health/status", gw.AdminHealthStatusHTTP)
	mux.HandleFunc("/admin/metrics/summary", gw.AdminMetricsSummaryHTTP)
	mux.HandleFunc("/admin/", gw.AdminNotFoundHTTP)
	mux.HandleFunc("/metrics", gw.HandleMetricsHTTP)
	mux.HandleFunc("/web", gw.HandleWebUIHTTP)
	mux.HandleFunc("/web/", gw.HandleWebUIHTTP)
	mux.HandleFunc("/", gw.ServeHTTP)
	return httptest.NewServer(mux)
}

func TestAdminUnauthorized(t *testing.T) {
	gw, upstream, authSrv := newGatewayForAdminTest(t)
	defer gw.Shutdown()
	defer upstream.Close()
	defer authSrv.Close()

	srv := newAdminTestServer(gw)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/admin/config/versions")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d want=%d body=%s", resp.StatusCode, http.StatusUnauthorized, string(body))
	}
}

func TestAdminRouteUpsertRollbackAndMetrics(t *testing.T) {
	gw, upstream, authSrv := newGatewayForAdminTest(t)
	defer gw.Shutdown()
	defer upstream.Close()
	defer authSrv.Close()

	time.Sleep(80 * time.Millisecond)
	srv := newAdminTestServer(gw)
	defer srv.Close()

	adminPost := func(path string, payload interface{}) *http.Response {
		data, _ := json.Marshal(payload)
		req, _ := http.NewRequest(http.MethodPost, srv.URL+path, bytes.NewReader(data))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Admin-Token", "admin-secret")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("POST %s failed: %v", path, err)
		}
		return resp
	}

	adminGet := func(path string) *http.Response {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+path, nil)
		req.Header.Set("X-Admin-Token", "admin-secret")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET %s failed: %v", path, err)
		}
		return resp
	}

	// 获取初始版本
	respVer := adminGet("/admin/config/versions")
	if respVer.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(respVer.Body)
		respVer.Body.Close()
		t.Fatalf("versions status=%d body=%s", respVer.StatusCode, string(body))
	}
	var verResp struct {
		Status string `json:"status"`
		Data   struct {
			Current struct {
				Version string `json:"version"`
			} `json:"current"`
			History []struct {
				Version string `json:"version"`
			} `json:"history"`
		} `json:"data"`
	}
	if err := json.NewDecoder(respVer.Body).Decode(&verResp); err != nil {
		respVer.Body.Close()
		t.Fatalf("decode versions failed: %v", err)
	}
	respVer.Body.Close()
	if verResp.Data.Current.Version == "" {
		t.Fatalf("expected current version")
	}

	// 在线新增路由
	respUpsert := adminPost("/admin/routes/upsert", map[string]interface{}{
		"route": map[string]interface{}{
			"path_prefix":  "/dynamic",
			"service_name": "service-a",
			"plugins":      []interface{}{},
		},
	})
	if respUpsert.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(respUpsert.Body)
		respUpsert.Body.Close()
		t.Fatalf("route upsert status=%d body=%s", respUpsert.StatusCode, string(body))
	}
	respUpsert.Body.Close()
	time.Sleep(80 * time.Millisecond)

	// 新路由生效
	respDynamic, err := http.Get(srv.URL + "/dynamic/hello")
	if err != nil {
		t.Fatalf("request dynamic route failed: %v", err)
	}
	if respDynamic.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(respDynamic.Body)
		respDynamic.Body.Close()
		t.Fatalf("dynamic status=%d body=%s", respDynamic.StatusCode, string(body))
	}
	respDynamic.Body.Close()

	// 记录包含 /dynamic 路由的版本号
	respVerAfterUpsert := adminGet("/admin/config/versions")
	if respVerAfterUpsert.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(respVerAfterUpsert.Body)
		respVerAfterUpsert.Body.Close()
		t.Fatalf("versions(after upsert) status=%d body=%s", respVerAfterUpsert.StatusCode, string(body))
	}
	if err := json.NewDecoder(respVerAfterUpsert.Body).Decode(&verResp); err != nil {
		respVerAfterUpsert.Body.Close()
		t.Fatalf("decode versions(after upsert) failed: %v", err)
	}
	respVerAfterUpsert.Body.Close()
	rollbackTarget := verResp.Data.Current.Version
	if rollbackTarget == "" {
		t.Fatalf("expected rollback target version")
	}

	// 删除路由
	respDelete := adminPost("/admin/routes/delete", map[string]interface{}{"path_prefix": "/dynamic"})
	if respDelete.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(respDelete.Body)
		respDelete.Body.Close()
		t.Fatalf("route delete status=%d body=%s", respDelete.StatusCode, string(body))
	}
	respDelete.Body.Close()

	resp404, _ := http.Get(srv.URL + "/dynamic/hello")
	if resp404.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp404.Body)
		resp404.Body.Close()
		t.Fatalf("after delete status=%d body=%s", resp404.StatusCode, string(body))
	}
	resp404.Body.Close()

	// 获取版本历史，找到非初始版本用于回滚
	respVer2 := adminGet("/admin/config/versions")
	if respVer2.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(respVer2.Body)
		respVer2.Body.Close()
		t.Fatalf("versions status=%d body=%s", respVer2.StatusCode, string(body))
	}
	if err := json.NewDecoder(respVer2.Body).Decode(&verResp); err != nil {
		respVer2.Body.Close()
		t.Fatalf("decode versions failed: %v", err)
	}
	respVer2.Body.Close()

	respRollback := adminPost("/admin/config/rollback", map[string]interface{}{"version": rollbackTarget})
	if respRollback.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(respRollback.Body)
		respRollback.Body.Close()
		t.Fatalf("rollback status=%d body=%s", respRollback.StatusCode, string(body))
	}
	respRollback.Body.Close()
	time.Sleep(80 * time.Millisecond)

	respAfterRollback, _ := http.Get(srv.URL + "/dynamic/hello")
	if respAfterRollback.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(respAfterRollback.Body)
		respAfterRollback.Body.Close()
		t.Fatalf("after rollback status=%d body=%s", respAfterRollback.StatusCode, string(body))
	}
	respAfterRollback.Body.Close()

	// 验证 metrics
	respMetrics, err := http.Get(srv.URL + "/metrics")
	if err != nil {
		t.Fatalf("metrics request failed: %v", err)
	}
	body, _ := io.ReadAll(respMetrics.Body)
	respMetrics.Body.Close()
	if respMetrics.StatusCode != http.StatusOK {
		t.Fatalf("metrics status=%d body=%s", respMetrics.StatusCode, string(body))
	}
	text := string(body)
	mustContain := []string{
		"gateway_requests_total",
		"gateway_qps_1m",
		"gateway_qps_10s",
		"gateway_latency_p99_ms",
		"gateway_responses_429_total",
		"gateway_circuit_open_total",
	}
	for _, key := range mustContain {
		if !strings.Contains(text, key) {
			t.Fatalf("metrics output missing %s", key)
		}
	}

	// 验证结构化 metrics summary
	respMetricSummary := adminGet("/admin/metrics/summary")
	if respMetricSummary.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(respMetricSummary.Body)
		respMetricSummary.Body.Close()
		t.Fatalf("metrics summary status=%d body=%s", respMetricSummary.StatusCode, string(body))
	}
	var metricSummaryResp struct {
		Status string                 `json:"status"`
		Data   map[string]interface{} `json:"data"`
	}
	if err := json.NewDecoder(respMetricSummary.Body).Decode(&metricSummaryResp); err != nil {
		respMetricSummary.Body.Close()
		t.Fatalf("decode metrics summary failed: %v", err)
	}
	respMetricSummary.Body.Close()
	if metricSummaryResp.Status != "ok" {
		t.Fatalf("metrics summary status field = %s", metricSummaryResp.Status)
	}
	for _, key := range []string{"total_requests", "qps_10s", "qps_1m", "latency_p99_ms", "circuit_open_total", "latency_histogram"} {
		if _, ok := metricSummaryResp.Data[key]; !ok {
			t.Fatalf("metrics summary missing key %s", key)
		}
	}

	// 验证 current config 接口
	respCurrent := adminGet("/admin/config/current")
	if respCurrent.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(respCurrent.Body)
		respCurrent.Body.Close()
		t.Fatalf("config current status=%d body=%s", respCurrent.StatusCode, string(body))
	}
	var currentResp struct {
		Status string `json:"status"`
		Data   struct {
			Version map[string]interface{} `json:"version"`
			Config  map[string]interface{} `json:"config"`
		} `json:"data"`
	}
	if err := json.NewDecoder(respCurrent.Body).Decode(&currentResp); err != nil {
		respCurrent.Body.Close()
		t.Fatalf("decode current config failed: %v", err)
	}
	respCurrent.Body.Close()
	if currentResp.Data.Version == nil || currentResp.Data.Config == nil {
		t.Fatalf("config current response missing version or config")
	}

	// 验证 health status 接口
	respHealth := adminGet("/admin/health/status")
	if respHealth.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(respHealth.Body)
		respHealth.Body.Close()
		t.Fatalf("health status=%d body=%s", respHealth.StatusCode, string(body))
	}
	var healthResp struct {
		Status string                     `json:"status"`
		Data   map[string]map[string]bool `json:"data"`
	}
	if err := json.NewDecoder(respHealth.Body).Decode(&healthResp); err != nil {
		respHealth.Body.Close()
		t.Fatalf("decode health status failed: %v", err)
	}
	respHealth.Body.Close()
	if len(healthResp.Data) == 0 {
		t.Fatalf("health status should not be empty")
	}
}

func TestAdminRateLimitRuleUpsertValidate(t *testing.T) {
	gw, upstream, authSrv := newGatewayForAdminTest(t)
	defer gw.Shutdown()
	defer upstream.Close()
	defer authSrv.Close()

	srv := newAdminTestServer(gw)
	defer srv.Close()

	reqBody := map[string]interface{}{
		"name":        "invalid-rule",
		"type":        "memory_token_bucket",
		"capacity":    0,
		"refill_rate": 10,
	}
	data, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/admin/ratelimit/rules/upsert", bytes.NewReader(data))
	req.Header.Set("X-Admin-Token", "admin-secret")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d want=%d body=%s", resp.StatusCode, http.StatusBadRequest, string(body))
	}
}

func TestAdminServiceUpsertAndDelete(t *testing.T) {
	gw, upstream, authSrv := newGatewayForAdminTest(t)
	defer gw.Shutdown()
	defer upstream.Close()
	defer authSrv.Close()

	srv := newAdminTestServer(gw)
	defer srv.Close()

	adminPost := func(path string, payload interface{}) *http.Response {
		data, _ := json.Marshal(payload)
		req, _ := http.NewRequest(http.MethodPost, srv.URL+path, bytes.NewReader(data))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Admin-Token", "admin-secret")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("POST %s failed: %v", path, err)
		}
		return resp
	}

	adminGet := func(path string) *http.Response {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+path, nil)
		req.Header.Set("X-Admin-Token", "admin-secret")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET %s failed: %v", path, err)
		}
		return resp
	}

	respList := adminGet("/admin/services")
	if respList.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(respList.Body)
		respList.Body.Close()
		t.Fatalf("service list status=%d body=%s", respList.StatusCode, string(body))
	}
	respList.Body.Close()

	respUpsert := adminPost("/admin/services/upsert", map[string]interface{}{
		"service": map[string]interface{}{
			"name":              "service-a-canary-temp",
			"health_check_path": "/healthz",
			"load_balancer":     "round_robin",
			"instances": []map[string]interface{}{
				{"url": upstream.URL, "weight": 1},
			},
		},
	})
	if respUpsert.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(respUpsert.Body)
		respUpsert.Body.Close()
		t.Fatalf("service upsert status=%d body=%s", respUpsert.StatusCode, string(body))
	}
	respUpsert.Body.Close()

	respDelete := adminPost("/admin/services/delete", map[string]interface{}{
		"name": "service-a-canary-temp",
	})
	if respDelete.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(respDelete.Body)
		respDelete.Body.Close()
		t.Fatalf("service delete status=%d body=%s", respDelete.StatusCode, string(body))
	}
	respDelete.Body.Close()
}

func TestAdminConfigApplyPublishModes(t *testing.T) {
	gw, upstream, authSrv := newGatewayForAdminTest(t)
	defer gw.Shutdown()
	defer upstream.Close()
	defer authSrv.Close()

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yml")
	sentinel := "sentinel: keep\n"
	if err := os.WriteFile(cfgPath, []byte(sentinel), 0o644); err != nil {
		t.Fatalf("write sentinel config failed: %v", err)
	}
	gw.SetConfigPath(cfgPath)

	srv := newAdminTestServer(gw)
	defer srv.Close()
	time.Sleep(80 * time.Millisecond)

	adminPost := func(path string, payload interface{}) *http.Response {
		data, _ := json.Marshal(payload)
		req, _ := http.NewRequest(http.MethodPost, srv.URL+path, bytes.NewReader(data))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Admin-Token", "admin-secret")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("POST %s failed: %v", path, err)
		}
		return resp
	}

	adminGet := func(path string) *http.Response {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+path, nil)
		req.Header.Set("X-Admin-Token", "admin-secret")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET %s failed: %v", path, err)
		}
		return resp
	}

	// 拉取当前配置并追加一条新路由
	respCurrent := adminGet("/admin/config/current")
	if respCurrent.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(respCurrent.Body)
		respCurrent.Body.Close()
		t.Fatalf("config current status=%d body=%s", respCurrent.StatusCode, string(body))
	}
	var currentResp struct {
		Status string `json:"status"`
		Data   struct {
			Config map[string]interface{} `json:"config"`
		} `json:"data"`
	}
	if err := json.NewDecoder(respCurrent.Body).Decode(&currentResp); err != nil {
		respCurrent.Body.Close()
		t.Fatalf("decode current config failed: %v", err)
	}
	respCurrent.Body.Close()

	routes, _ := currentResp.Data.Config["routes"].([]interface{})
	routes = append(routes, map[string]interface{}{
		"path_prefix":  "/persist-check",
		"service_name": "service-a",
		"methods":      []string{"GET"},
	})
	currentResp.Data.Config["routes"] = routes

	// 仅内存发布：配置应生效，但文件不变。
	respApplyMemory := adminPost("/admin/config/apply", map[string]interface{}{
		"config":  currentResp.Data.Config,
		"dry_run": false,
		"source":  "test:memory-only",
		"persist": false,
	})
	if respApplyMemory.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(respApplyMemory.Body)
		respApplyMemory.Body.Close()
		t.Fatalf("apply(memory) status=%d body=%s", respApplyMemory.StatusCode, string(body))
	}
	respApplyMemory.Body.Close()

	dataAfterMemory, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read config after memory apply failed: %v", err)
	}
	if string(dataAfterMemory) != sentinel {
		t.Fatalf("expected config file unchanged for memory publish, got: %s", string(dataAfterMemory))
	}

	// 落盘发布：文件应更新并包含新路由。
	respApplyPersist := adminPost("/admin/config/apply", map[string]interface{}{
		"config":  currentResp.Data.Config,
		"dry_run": false,
		"source":  "test:persist",
		"persist": true,
	})
	if respApplyPersist.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(respApplyPersist.Body)
		respApplyPersist.Body.Close()
		t.Fatalf("apply(persist) status=%d body=%s", respApplyPersist.StatusCode, string(body))
	}
	respApplyPersist.Body.Close()

	dataAfterPersist, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read config after persist apply failed: %v", err)
	}
	if strings.Contains(string(dataAfterPersist), sentinel) {
		t.Fatalf("expected config file to be replaced, still has sentinel")
	}
	if !strings.Contains(string(dataAfterPersist), "/persist-check") {
		t.Fatalf("expected persisted config to include /persist-check route, got: %s", string(dataAfterPersist))
	}
}
