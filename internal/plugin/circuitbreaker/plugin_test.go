package circuitbreaker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"yuelaiengine/gateway/internal/config"
	svc_cb "yuelaiengine/gateway/internal/service/circuitbreaker"
	"yuelaiengine/gateway/internal/testutil"
)

func TestCircuitBreakerPlugin_RejectAndConfigError(t *testing.T) {
	svc := svc_cb.NewService(1, 1, time.Minute, &testutil.NoopLogger{})
	plugin := NewPlugin(svc, &testutil.NoopLogger{})

	// 初始化并制造熔断打开状态
	_, _ = svc.CheckCircuit(context.Background(), "svc-a")
	svc.RecordResult(context.Background(), "svc-a", false)

	req := httptest.NewRequest(http.MethodGet, "/service-a/x", nil)
	w := httptest.NewRecorder()
	ok, err := plugin.Execute(w, req, config.PluginSpec{"name": "circuitbreaker", "service": "svc-a"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if ok {
		t.Fatalf("expected plugin stop when circuit open")
	}
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d want=%d", w.Code, http.StatusServiceUnavailable)
	}

	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/service-a/x", nil)
	ok, err = plugin.Execute(w2, req2, config.PluginSpec{"name": "circuitbreaker"})
	if err == nil {
		t.Fatalf("expected config error")
	}
	if ok {
		t.Fatalf("expected plugin stop")
	}
	if w2.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want=%d", w2.Code, http.StatusInternalServerError)
	}
}
