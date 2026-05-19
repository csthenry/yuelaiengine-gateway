package apikey

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"yuelaiengine/gateway/internal/config"
)

func newTestPlugin(t *testing.T) *Plugin {
	t.Helper()
	return NewPlugin(nil)
}

func TestAPIKeyPlugin_Allow(t *testing.T) {
	plugin := newTestPlugin(t)
	req := httptest.NewRequest(http.MethodGet, "/secure", nil)
	req.Header.Set("X-API-Key", "k1")
	w := httptest.NewRecorder()

	ok, err := plugin.Execute(w, req, config.PluginSpec{
		"name": "apikey",
		"keys": []interface{}{"k1", "k2"},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !ok {
		t.Fatalf("expected plugin chain continue")
	}
}

func TestAPIKeyPlugin_RejectAndConfigError(t *testing.T) {
	plugin := newTestPlugin(t)

	t.Run("missing key", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/secure", nil)
		w := httptest.NewRecorder()
		ok, err := plugin.Execute(w, req, config.PluginSpec{"name": "apikey", "keys": []interface{}{"k1"}})
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		if ok {
			t.Fatalf("expected plugin stop")
		}
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
		}
	})

	t.Run("invalid key", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/secure", nil)
		req.Header.Set("X-API-Key", "bad")
		w := httptest.NewRecorder()
		ok, err := plugin.Execute(w, req, config.PluginSpec{"name": "apikey", "keys": []interface{}{"k1"}})
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		if ok {
			t.Fatalf("expected plugin stop")
		}
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
		}
	})

	t.Run("config error", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/secure", nil)
		w := httptest.NewRecorder()
		ok, err := plugin.Execute(w, req, config.PluginSpec{"name": "apikey"})
		if err == nil {
			t.Fatalf("expected config error")
		}
		if ok {
			t.Fatalf("expected plugin stop")
		}
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
		}
	})
}
