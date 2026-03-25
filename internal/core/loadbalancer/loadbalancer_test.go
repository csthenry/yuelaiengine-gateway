package loadbalancer

import (
	"sync"
	"testing"
	"time"
)

func newInstance(url string, alive bool, weight int, connections int) *ServiceInstance {
	return &ServiceInstance{
		URL:         url,
		Alive:       alive,
		Weight:      weight,
		Connections: connections,
	}
}

func TestRoundRobinBalancer_RotationAndHealthUpdate(t *testing.T) {
	lb := NewRoundRobinBalancer("svc")
	a := newInstance("http://a", true, 0, 0)
	b := newInstance("http://b", true, 0, 0)
	c := newInstance("http://c", true, 0, 0)

	lb.RegisterInstance("svc", a)
	lb.RegisterInstance("svc", b)
	lb.RegisterInstance("svc", c)

	want := []string{"http://a", "http://b", "http://c", "http://a"}
	for i := 0; i < len(want); i++ {
		inst, err := lb.GetNextInstance("svc")
		if err != nil {
			t.Fatalf("GetNextInstance() error = %v", err)
		}
		if inst.URL != want[i] {
			t.Fatalf("round robin order mismatch at %d: got %s want %s", i, inst.URL, want[i])
		}
	}

	if err := lb.SetInstanceAlive("svc", "http://b", false); err != nil {
		t.Fatalf("SetInstanceAlive() error = %v", err)
	}

	inst1, err := lb.GetNextInstance("svc")
	if err != nil {
		t.Fatalf("GetNextInstance() error = %v", err)
	}
	inst2, err := lb.GetNextInstance("svc")
	if err != nil {
		t.Fatalf("GetNextInstance() error = %v", err)
	}
	if inst1.URL != "http://a" || inst2.URL != "http://c" {
		t.Fatalf("unexpected order after health update: got [%s,%s] want [http://a,http://c]", inst1.URL, inst2.URL)
	}

	if err := lb.SetInstanceAlive("svc", "http://not-exist", true); err == nil {
		t.Fatalf("expected error when setting unknown instance alive")
	}
	if err := lb.ReleaseConnection("svc", "http://a"); err != nil {
		t.Fatalf("ReleaseConnection() for RR should be nil, got %v", err)
	}
}

func TestWeightedRoundRobinBalancer_WeightedDistribution(t *testing.T) {
	lb := NewWeightedRoundRobinBalancer("svc")
	lb.RegisterInstance("svc", newInstance("http://a", true, 2, 0))
	lb.RegisterInstance("svc", newInstance("http://b", true, 1, 0))

	want := []string{"http://a", "http://a", "http://b", "http://a", "http://a", "http://b"}
	for i := 0; i < len(want); i++ {
		inst, err := lb.GetNextInstance("svc")
		if err != nil {
			t.Fatalf("GetNextInstance() error = %v", err)
		}
		if inst.URL != want[i] {
			t.Fatalf("weighted order mismatch at %d: got %s want %s", i, inst.URL, want[i])
		}
	}
}

func TestWeightedRoundRobinBalancer_FallbackRRForNonPositiveWeight(t *testing.T) {
	lb := NewWeightedRoundRobinBalancer("svc")
	lb.RegisterInstance("svc", newInstance("http://a", true, 0, 0))
	lb.RegisterInstance("svc", newInstance("http://b", true, -3, 0))
	lb.RegisterInstance("svc", newInstance("http://c", true, 0, 0))

	want := []string{"http://a", "http://b", "http://c", "http://a"}
	for i := 0; i < len(want); i++ {
		inst, err := lb.GetNextInstance("svc")
		if err != nil {
			t.Fatalf("GetNextInstance() error = %v", err)
		}
		if inst.URL != want[i] {
			t.Fatalf("fallback RR mismatch at %d: got %s want %s", i, inst.URL, want[i])
		}
	}
}

func TestLeastConnectionsBalancer_SelectAndRelease(t *testing.T) {
	lb := NewLeastConnectionsBalancer("svc")
	a := newInstance("http://a", true, 0, 0)
	b := newInstance("http://b", true, 0, 0)
	lb.RegisterInstance("svc", a)
	lb.RegisterInstance("svc", b)

	inst1, err := lb.GetNextInstance("svc")
	if err != nil {
		t.Fatalf("GetNextInstance() error = %v", err)
	}
	inst2, err := lb.GetNextInstance("svc")
	if err != nil {
		t.Fatalf("GetNextInstance() error = %v", err)
	}
	inst3, err := lb.GetNextInstance("svc")
	if err != nil {
		t.Fatalf("GetNextInstance() error = %v", err)
	}

	if inst1.URL != "http://a" || inst2.URL != "http://b" || inst3.URL != "http://a" {
		t.Fatalf("least-connections order mismatch: got [%s,%s,%s]", inst1.URL, inst2.URL, inst3.URL)
	}
	if a.Connections != 2 || b.Connections != 1 {
		t.Fatalf("unexpected connection counters: a=%d b=%d", a.Connections, b.Connections)
	}

	if err := lb.ReleaseConnection("svc", "http://a"); err != nil {
		t.Fatalf("ReleaseConnection() error = %v", err)
	}
	if a.Connections != 1 {
		t.Fatalf("expected a connections to be 1 after release, got %d", a.Connections)
	}

	if err := lb.SetInstanceAlive("svc", "http://a", false); err != nil {
		t.Fatalf("SetInstanceAlive(false) error = %v", err)
	}
	if err := lb.SetInstanceAlive("svc", "http://a", true); err != nil {
		t.Fatalf("SetInstanceAlive(true) error = %v", err)
	}
	if a.Connections != 0 {
		t.Fatalf("expected connections reset to 0 on recovery, got %d", a.Connections)
	}

	if err := lb.ReleaseConnection("svc", "http://not-exist"); err == nil {
		t.Fatalf("expected error when releasing unknown instance")
	}
}

func TestConsistentHashBalancer_StableAndHealthAware(t *testing.T) {
	lb := NewConsistentHashBalancer("svc")
	lb.RegisterInstance("svc", newInstance("http://a", true, 0, 0))
	lb.RegisterInstance("svc", newInstance("http://b", true, 0, 0))

	first, err := lb.GetInstanceByKey("svc", "user-42")
	if err != nil {
		t.Fatalf("GetInstanceByKey() error = %v", err)
	}

	for i := 0; i < 5; i++ {
		next, err := lb.GetInstanceByKey("svc", "user-42")
		if err != nil {
			t.Fatalf("GetInstanceByKey() error = %v", err)
		}
		if next.URL != first.URL {
			t.Fatalf("consistent hash is not stable: first=%s next=%s", first.URL, next.URL)
		}
	}

	if err := lb.SetInstanceAlive("svc", first.URL, false); err != nil {
		t.Fatalf("SetInstanceAlive() error = %v", err)
	}

	after, err := lb.GetInstanceByKey("svc", "user-42")
	if err != nil {
		t.Fatalf("GetInstanceByKey() after health update error = %v", err)
	}
	if after.URL == first.URL {
		t.Fatalf("expected key remap after instance unhealthy, still got %s", after.URL)
	}

	if err := lb.SetInstanceAlive("svc", "http://a", false); err != nil {
		t.Fatalf("SetInstanceAlive(http://a,false) error = %v", err)
	}
	if err := lb.SetInstanceAlive("svc", "http://b", false); err != nil {
		t.Fatalf("SetInstanceAlive(http://b,false) error = %v", err)
	}
	if _, err := lb.GetInstanceByKey("svc", "user-42"); err == nil {
		t.Fatalf("expected error when all instances are unhealthy")
	}

	if err := lb.SetInstanceAlive("svc", "http://not-exist", true); err == nil {
		t.Fatalf("expected error when setting unknown instance alive")
	}
}

func TestLoadBalancerFactory_UpdateAndRelease(t *testing.T) {
	f := NewLoadBalancerFactory()
	lb := f.GetOrCreateLoadBalancer("svc", "least_connections")
	a := newInstance("http://a", true, 0, 0)
	b := newInstance("http://b", true, 0, 0)
	lb.RegisterInstance("svc", a)
	lb.RegisterInstance("svc", b)

	inst, err := lb.GetNextInstance("svc")
	if err != nil {
		t.Fatalf("GetNextInstance() error = %v", err)
	}
	if err := f.ReleaseConnection("svc", inst.URL); err != nil {
		t.Fatalf("factory ReleaseConnection() error = %v", err)
	}

	if err := f.UpdateInstanceAlive("svc", "http://not-exist", false); err == nil {
		t.Fatalf("expected factory UpdateInstanceAlive() to return error for unknown instance")
	}
	if err := f.UpdateInstanceAlive("svc", "http://a", false); err != nil {
		t.Fatalf("factory UpdateInstanceAlive() error = %v", err)
	}

	healthy := lb.GetAllInstances("svc")
	if len(healthy) != 1 || healthy[0].URL != "http://b" {
		t.Fatalf("unexpected healthy instances after update: %+v", healthy)
	}

	if err := f.ReleaseConnection("not-exist", "http://a"); err == nil {
		t.Fatalf("expected error for unknown service in ReleaseConnection")
	}
	if _, err := f.GetInstanceByKey("svc", "k"); err == nil {
		t.Fatalf("expected GetInstanceByKey to fail for non-hash balancer")
	}
}

func TestLoadBalancerFactory_GetOrCreateConcurrent(t *testing.T) {
	f := NewLoadBalancerFactory()
	const n = 64

	results := make(chan LoadBalancer, n)
	var wg sync.WaitGroup

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			lb := f.GetOrCreateLoadBalancer("svc", "round_robin")
			results <- lb
		}()
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(results)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("concurrent GetOrCreate timed out")
	}

	var first LoadBalancer
	for lb := range results {
		if first == nil {
			first = lb
			continue
		}
		if lb != first {
			t.Fatalf("expected same balancer instance, got different pointers")
		}
	}
}
