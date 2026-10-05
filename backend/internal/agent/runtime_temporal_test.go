package agent

import (
	"context"
	"os"
	"testing"
	"time"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
)

// Explicit opt-in to an existing Temporal server. Only this test's client,
// random task queue and owned worker are stopped; the server is never touched.
func TestRuntimeActualTemporalHealthAndOwnedWorkerFailure(t *testing.T) {
	address := os.Getenv("AGENT_RUNTIME_TEST_TEMPORAL")
	if address == "" {
		t.Skip("actual Temporal explicitly required")
	}
	cfg := Config{Mode: "temporal", DataDir: t.TempDir(), TemporalAddress: address, TemporalNamespace: "default", TaskQueue: newID("runtime_health_")}
	s, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	c, err := client.DialContext(ctx, client.Options{HostPort: address, Namespace: "default", Identity: s.RuntimeIdentity()})
	if err != nil {
		t.Fatal(err)
	}
	s.Temporal = c
	w := worker.New(c, cfg.TaskQueue, worker.Options{Identity: s.RuntimeIdentity()})
	s.RegisterWorker(w)
	if err = w.Start(); err != nil {
		t.Fatal(err)
	}
	s.SetWorkerRunning(true)
	stopped := false
	defer func() {
		if !stopped {
			w.Stop()
		}
	}()
	deadline := time.Now().Add(20 * time.Second)
	for {
		h, e := s.RuntimeHealth(context.Background(), true)
		if e != nil {
			t.Fatal(e)
		}
		if h.Ready {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("actual worker did not become ready: %+v", h)
		}
		time.Sleep(200 * time.Millisecond)
	}
	w.Stop()
	stopped = true
	s.SetWorkerRunning(false)
	h, err := s.RuntimeHealth(context.Background(), true)
	if err != nil || h.Ready {
		t.Fatal("stopped worker advertised readiness", err)
	}
	for _, check := range h.Checks {
		if check.Name == "temporal" && check.Status != "ready" {
			t.Fatal("server was touched or unavailable")
		}
		if check.Name == "worker" && check.Status != "not_ready" {
			t.Fatal("old poller used as current worker")
		}
	}
	c.Close()
	h, err = s.RuntimeHealth(context.Background(), true)
	if err != nil || h.Ready {
		t.Fatal("closed actual connection advertised readiness", err)
	}
	for _, check := range h.Checks {
		if check.Name == "temporal" && check.Status != "not_ready" {
			t.Fatal("cached client mistaken for live connection")
		}
	}
	if len(s.Store.List()) != 0 {
		t.Fatal("probe created a business Run")
	}
}
