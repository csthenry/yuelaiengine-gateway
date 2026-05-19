package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"yuelaiengine/gateway/internal/config"
	"yuelaiengine/gateway/internal/core/loadbalancer"
)

func registerAuthInstance(factory *loadbalancer.LoadBalancerFactory, serviceName, instanceURL string) {
	factory.ReplaceServiceInstances(serviceName, "round_robin", []*loadbalancer.ServiceInstance{
		{URL: instanceURL, Alive: true, Weight: 1},
	})
}

func TestAuthPlugin_AllowAndInjectHeaders(t *testing.T) {
	authSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/validate" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"valid":   true,
			"user_id": "u-100",
			"role":    "admin",
		})
	}))
	defer authSrv.Close()

	factory := loadbalancer.NewLoadBalancerFactory()
	registerAuthInstance(factory, "auth-service", authSrv.URL)

	plugin, err := NewPlugin(factory, nil, "auth-service", "http://auth-service/validate", nil)
	if err != nil {
		t.Fatalf("NewPlugin() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/secure", nil)
	req.Header.Set("Authorization", "Bearer token-ok")
	w := httptest.NewRecorder()

	ok, err := plugin.Execute(w, req, config.PluginSpec{"name": "auth"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !ok {
		t.Fatalf("expected continue")
	}
	if got := req.Header.Get("X-User-ID"); got != "u-100" {
		t.Fatalf("X-User-ID = %q, want %q", got, "u-100")
	}
	if got := req.Header.Get("X-User-Role"); got != "admin" {
		t.Fatalf("X-User-Role = %q, want %q", got, "admin")
	}
}

func TestAuthPlugin_RejectAndErrorPaths(t *testing.T) {
	t.Run("token missing", func(t *testing.T) {
		factory := loadbalancer.NewLoadBalancerFactory()
		registerAuthInstance(factory, "auth-service", "http://127.0.0.1:1")
		plugin, err := NewPlugin(factory, nil, "auth-service", "http://auth-service/validate", nil)
		if err != nil {
			t.Fatalf("NewPlugin() error = %v", err)
		}
		req := httptest.NewRequest(http.MethodGet, "/secure", nil)
		w := httptest.NewRecorder()
		ok, err := plugin.Execute(w, req, config.PluginSpec{"name": "auth"})
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		if ok {
			t.Fatalf("expected stop")
		}
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status=%d want=%d", w.Code, http.StatusUnauthorized)
		}
	})

	t.Run("invalid token", func(t *testing.T) {
		authSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"valid": false})
		}))
		defer authSrv.Close()

		factory := loadbalancer.NewLoadBalancerFactory()
		registerAuthInstance(factory, "auth-service", authSrv.URL)
		plugin, err := NewPlugin(factory, nil, "auth-service", "http://auth-service/validate", nil)
		if err != nil {
			t.Fatalf("NewPlugin() error = %v", err)
		}

		req := httptest.NewRequest(http.MethodGet, "/secure", nil)
		req.Header.Set("Authorization", "Bearer bad")
		w := httptest.NewRecorder()
		ok, err := plugin.Execute(w, req, config.PluginSpec{"name": "auth"})
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		if ok {
			t.Fatalf("expected stop")
		}
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status=%d want=%d", w.Code, http.StatusUnauthorized)
		}
	})

	t.Run("validate service error", func(t *testing.T) {
		authSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer authSrv.Close()

		factory := loadbalancer.NewLoadBalancerFactory()
		registerAuthInstance(factory, "auth-service", authSrv.URL)
		plugin, err := NewPlugin(factory, nil, "auth-service", "http://auth-service/validate", nil)
		if err != nil {
			t.Fatalf("NewPlugin() error = %v", err)
		}

		req := httptest.NewRequest(http.MethodGet, "/secure", nil)
		req.Header.Set("Authorization", "Bearer bad")
		w := httptest.NewRecorder()
		ok, err := plugin.Execute(w, req, config.PluginSpec{"name": "auth"})
		if err == nil {
			t.Fatalf("expected execute error")
		}
		if ok {
			t.Fatalf("expected stop")
		}
		if w.Code != http.StatusBadGateway {
			t.Fatalf("status=%d want=%d", w.Code, http.StatusBadGateway)
		}
	})

	t.Run("auth service unavailable", func(t *testing.T) {
		factory := loadbalancer.NewLoadBalancerFactory()
		plugin, err := NewPlugin(factory, nil, "auth-service", "http://auth-service/validate", nil)
		if err != nil {
			t.Fatalf("NewPlugin() error = %v", err)
		}

		req := httptest.NewRequest(http.MethodGet, "/secure", nil)
		req.Header.Set("Authorization", "Bearer t")
		w := httptest.NewRecorder()
		ok, err := plugin.Execute(w, req, config.PluginSpec{"name": "auth"})
		if err == nil {
			t.Fatalf("expected execute error")
		}
		if ok {
			t.Fatalf("expected stop")
		}
		if w.Code != http.StatusBadGateway {
			t.Fatalf("status=%d want=%d", w.Code, http.StatusBadGateway)
		}
	})
}
