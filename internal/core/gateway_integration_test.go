package core

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"yuelaiengine/gateway/internal/config"
	"yuelaiengine/gateway/internal/testutil"
)

func TestGatewayAuthChainIntegration(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK"))
		case "/hello":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("hello secure"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	authSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK"))
		case "/validate":
			authz := r.Header.Get("Authorization")
			switch authz {
			case "Bearer token-admin":
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"valid": true, "user_id": "u-admin", "role": "admin"})
			case "Bearer token-guest":
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"valid": true, "user_id": "u-guest", "role": "guest"})
			default:
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"valid": false})
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer authSrv.Close()

	cfg := &config.GatewayConfig{
		Server: config.ServerConfig{Port: ":9000"},
		HealthCheck: config.HealthCheckConfig{
			Interval: 20 * time.Millisecond,
			Timeout:  time.Second,
		},
		AuthService: config.AuthServiceConfig{ValidateURL: "http://auth-service/validate"},
		CircuitBreaker: config.CircuitBreakerConfig{
			FailureThreshold: 5,
			SuccessThreshold: 2,
			ResetTimeout:     time.Minute,
		},
		Services: map[string]config.ServiceConfig{
			"service-a": {
				Name:            "service-a",
				LoadBalancer:    "round_robin",
				HealthCheckPath: "/healthz",
				Instances: []config.InstanceConfig{{URL: upstream.URL, Weight: 1}},
			},
			"auth-service": {
				Name:            "auth-service",
				LoadBalancer:    "round_robin",
				HealthCheckPath: "/healthz",
				Instances: []config.InstanceConfig{{URL: authSrv.URL, Weight: 1}},
			},
		},
		Routes: []*config.RouteConfig{
			{
				PathPrefix:   "/secure",
				ServiceName:  "service-a",
				RequiresAuth: true,
				Plugins: []config.PluginSpec{
					{"name": "auth"},
					{"name": "apikey", "header": "X-API-Key", "keys": []interface{}{"gateway-demo-key"}},
					{"name": "rbac", "header": "X-User-Role", "roles": []interface{}{"admin"}},
				},
			},
		},
	}

	gw, err := NewGateway(cfg, &testutil.NoopLogger{})
	if err != nil {
		t.Fatalf("NewGateway() error = %v", err)
	}
	defer gw.Shutdown()

	time.Sleep(80 * time.Millisecond)
	gatewaySrv := httptest.NewServer(gw)
	defer gatewaySrv.Close()

	t.Run("success", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, gatewaySrv.URL+"/secure/hello", nil)
		req.Header.Set("Authorization", "Bearer token-admin")
		req.Header.Set("X-API-Key", "gateway-demo-key")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("http request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("status=%d body=%s", resp.StatusCode, string(body))
		}
	})

	t.Run("missing api key", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, gatewaySrv.URL+"/secure/hello", nil)
		req.Header.Set("Authorization", "Bearer token-admin")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("http request failed: %v", err)
		}
		defer resp.Body.Close()

		assertErrorCode(t, resp, http.StatusUnauthorized, "API_KEY_MISSING")
	})

	t.Run("invalid token", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, gatewaySrv.URL+"/secure/hello", nil)
		req.Header.Set("Authorization", "Bearer token-invalid")
		req.Header.Set("X-API-Key", "gateway-demo-key")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("http request failed: %v", err)
		}
		defer resp.Body.Close()

		assertErrorCode(t, resp, http.StatusUnauthorized, "AUTH_TOKEN_INVALID")
	})

	t.Run("rbac forbidden", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, gatewaySrv.URL+"/secure/hello", nil)
		req.Header.Set("Authorization", "Bearer token-guest")
		req.Header.Set("X-API-Key", "gateway-demo-key")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("http request failed: %v", err)
		}
		defer resp.Body.Close()

		assertErrorCode(t, resp, http.StatusForbidden, "RBAC_FORBIDDEN")
	})
}

func assertErrorCode(t *testing.T, resp *http.Response, expectedStatus int, expectedCode string) {
	t.Helper()
	if resp.StatusCode != expectedStatus {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d want=%d body=%s", resp.StatusCode, expectedStatus, string(body))
	}

	var payload struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode error response failed: %v", err)
	}
	if payload.Code != expectedCode {
		t.Fatalf("code=%s want=%s (message=%s)", payload.Code, expectedCode, payload.Message)
	}
}

func TestGatewayRequiresAuthAutoInject(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusOK)
		case "/hello":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	authSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusOK)
		case "/validate":
			if r.Header.Get("Authorization") == "Bearer token-admin" {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"valid": true, "user_id": "u-admin", "role": "admin"})
				return
			}
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"valid": false})
		default:
			http.NotFound(w, r)
		}
	}))
	defer authSrv.Close()

	cfg := &config.GatewayConfig{
		Server: config.ServerConfig{Port: ":9000"},
		HealthCheck: config.HealthCheckConfig{Interval: 20 * time.Millisecond, Timeout: time.Second},
		AuthService: config.AuthServiceConfig{ValidateURL: "http://auth-service/validate"},
		CircuitBreaker: config.CircuitBreakerConfig{FailureThreshold: 5, SuccessThreshold: 2, ResetTimeout: time.Minute},
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
		Routes: []*config.RouteConfig{{
			PathPrefix:   "/secure",
			ServiceName:  "service-a",
			RequiresAuth: true,
			Plugins: []config.PluginSpec{{"name": "apikey", "header": "X-API-Key", "keys": []interface{}{"gateway-demo-key"}}},
		}},
	}

	gw, err := NewGateway(cfg, &testutil.NoopLogger{})
	if err != nil {
		t.Fatalf("NewGateway() error = %v", err)
	}
	defer gw.Shutdown()
	time.Sleep(80 * time.Millisecond)

	srv := httptest.NewServer(gw)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/secure/hello", nil)
	req.Header.Set("X-API-Key", "gateway-demo-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("http request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d want=%d body=%s", resp.StatusCode, http.StatusUnauthorized, string(body))
	}

	var payload map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode error failed: %v", err)
	}
	if got := fmt.Sprintf("%v", payload["code"]); got != "AUTH_TOKEN_MISSING" {
		t.Fatalf("code=%s want=%s", got, "AUTH_TOKEN_MISSING")
	}
}
