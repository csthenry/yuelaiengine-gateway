package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"yuelaiengine/gateway/internal/config"
	svc_ratelimit "yuelaiengine/gateway/internal/service/ratelimit"
	"yuelaiengine/gateway/internal/testutil"
)

func newRateLimitPlugin(t *testing.T, rules []config.RateLimiterRule) *Plugin {
	t.Helper()
	svc, err := svc_ratelimit.NewService(config.RateLimitingConfig{Rules: rules}, &testutil.NoopLogger{})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return NewPlugin(svc, &testutil.NoopLogger{})
}

func TestRateLimitPlugin_RejectPath(t *testing.T) {
	plugin := newRateLimitPlugin(t, []config.RateLimiterRule{
		{
			Name: "burst",
			Type: "memory_token_bucket",
			TokenBucket: config.TokenBucketSettings{Capacity: 1, RefillRate: 0},
		},
	})

	cfg := config.PluginSpec{"name": "ratelimit", "rule": "burst", "strategy": "global"}

	w1 := httptest.NewRecorder()
	req1 := httptest.NewRequest(http.MethodGet, "/x", nil)
	ok, err := plugin.Execute(w1, req1, cfg)
	if err != nil {
		t.Fatalf("Execute(1) error = %v", err)
	}
	if !ok {
		t.Fatalf("first request should pass")
	}

	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/x", nil)
	ok, err = plugin.Execute(w2, req2, cfg)
	if err != nil {
		t.Fatalf("Execute(2) error = %v", err)
	}
	if ok {
		t.Fatalf("second request should be blocked")
	}
	if w2.Code != http.StatusTooManyRequests {
		t.Fatalf("status=%d want=%d", w2.Code, http.StatusTooManyRequests)
	}
}

func TestRateLimitPlugin_ErrorPaths(t *testing.T) {
	plugin := newRateLimitPlugin(t, []config.RateLimiterRule{
		{
			Name: "burst",
			Type: "memory_token_bucket",
			TokenBucket: config.TokenBucketSettings{Capacity: 1, RefillRate: 1},
		},
	})

	t.Run("invalid strategy", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		ok, err := plugin.Execute(w, req, config.PluginSpec{"name": "ratelimit", "rule": "burst", "strategy": "unknown"})
		if err == nil {
			t.Fatalf("expected error")
		}
		if ok {
			t.Fatalf("expected stop")
		}
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status=%d want=%d", w.Code, http.StatusInternalServerError)
		}
	})

	t.Run("undefined rule", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		ok, err := plugin.Execute(w, req, config.PluginSpec{"name": "ratelimit", "rule": "not-exist", "strategy": "global"})
		if err == nil {
			t.Fatalf("expected error")
		}
		if ok {
			t.Fatalf("expected stop")
		}
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status=%d want=%d", w.Code, http.StatusInternalServerError)
		}
	})
}
