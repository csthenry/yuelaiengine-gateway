package circuitbreaker

import (
	"context"
	"testing"
	"time"

	"yuelaiengine/gateway/internal/testutil"
)

func TestResetUnknownServiceInitializesCircuit(t *testing.T) {
	svc := NewService(2, 1, time.Second, &testutil.NoopLogger{})
	ctx := context.Background()

	if err := svc.Reset(ctx, "service-a-canary"); err != nil {
		t.Fatalf("Reset() error = %v", err)
	}

	states := svc.GetAllState(ctx)
	state, ok := states["service-a-canary"]
	if !ok {
		t.Fatalf("expected state for reset service")
	}
	if state.State != "closed" {
		t.Fatalf("state=%s want=closed", state.State)
	}
	if state.FailureCount != 0 || state.SuccessCount != 0 {
		t.Fatalf("unexpected counters: failure=%d success=%d", state.FailureCount, state.SuccessCount)
	}
}

func TestGetAllStatePromotesOpenToHalfOpenAfterTimeout(t *testing.T) {
	svc := NewService(1, 2, 20*time.Millisecond, &testutil.NoopLogger{})
	ctx := context.Background()
	serviceName := "service-a"

	allowed, err := svc.CheckCircuit(ctx, serviceName)
	if err != nil || !allowed {
		t.Fatalf("CheckCircuit() allowed=%v err=%v", allowed, err)
	}

	svc.RecordResult(ctx, serviceName, false)

	stateOpen := svc.GetAllState(ctx)[serviceName]
	if stateOpen.State != "open" {
		t.Fatalf("state=%s want=open", stateOpen.State)
	}

	time.Sleep(30 * time.Millisecond)

	stateHalfOpen := svc.GetAllState(ctx)[serviceName]
	if stateHalfOpen.State != "half-open" {
		t.Fatalf("state=%s want=half-open", stateHalfOpen.State)
	}
}
