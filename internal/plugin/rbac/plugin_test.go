package rbac

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"yuelaiengine/gateway/internal/config"
)

func TestRBACPlugin_Allow(t *testing.T) {
	plugin := NewPlugin(nil)
	req := httptest.NewRequest(http.MethodGet, "/secure", nil)
	req.Header.Set("X-User-Role", "admin")
	w := httptest.NewRecorder()

	ok, err := plugin.Execute(w, req, config.PluginSpec{
		"name":  "rbac",
		"header": "X-User-Role",
		"roles": []interface{}{"admin", "hr"},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !ok {
		t.Fatalf("expected plugin chain continue")
	}
}

func TestRBACPlugin_RejectAndConfigError(t *testing.T) {
	plugin := NewPlugin(nil)

	t.Run("missing role", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/secure", nil)
		w := httptest.NewRecorder()
		ok, err := plugin.Execute(w, req, config.PluginSpec{"name": "rbac", "roles": []interface{}{"admin"}})
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		if ok {
			t.Fatalf("expected stop")
		}
		if w.Code != http.StatusForbidden {
			t.Fatalf("status=%d want=%d", w.Code, http.StatusForbidden)
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/secure", nil)
		req.Header.Set("X-User-Role", "guest")
		w := httptest.NewRecorder()
		ok, err := plugin.Execute(w, req, config.PluginSpec{"name": "rbac", "roles": []interface{}{"admin"}})
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		if ok {
			t.Fatalf("expected stop")
		}
		if w.Code != http.StatusForbidden {
			t.Fatalf("status=%d want=%d", w.Code, http.StatusForbidden)
		}
	})

	t.Run("config error", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/secure", nil)
		w := httptest.NewRecorder()
		ok, err := plugin.Execute(w, req, config.PluginSpec{"name": "rbac"})
		if err == nil {
			t.Fatalf("expected config error")
		}
		if ok {
			t.Fatalf("expected stop")
		}
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status=%d want=%d", w.Code, http.StatusInternalServerError)
		}
	})
}
