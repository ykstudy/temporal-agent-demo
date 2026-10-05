package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"agentstudio/internal/contracts"
)

func TestRuntimeCanonicalBudgetDuringActualCommands(t *testing.T) {
	s := testService(t)
	r := runtimeFixture(t, s, "runtime_budget_reads")
	mux := http.NewServeMux()
	s.Register(mux)
	var wg sync.WaitGroup
	stop := make(chan struct{})
	faults := make(chan error, 100)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			s.Store.mu.Lock()
			current := s.Store.runs[r.ID]
			e := s.Store.verifyCanonical(current)
			sequence := current.StateSequence
			s.Store.mu.Unlock()
			if e != nil {
				select {
				case faults <- fmt.Errorf("sequence %d: %w", sequence, e):
				default:
				}
			}
			time.Sleep(time.Millisecond)
		}
	}()
	for _, action := range []string{"pause", "resume", "cancel", "retry"} {
		current, e := s.Store.Get(r.ID)
		if e != nil {
			t.Fatal(e)
		}
		ref := conversationState(current).LatestSnapshot
		body, _ := json.Marshal(contracts.ControlRequest{CommandID: action + "_budget", Action: action, Target: contracts.MessageObjectRef(*ref)})
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/api/v2/runs/"+r.ID+"/actions", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		mux.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Errorf("%s: %d %s", action, w.Code, w.Body.String())
		}
	}
	close(stop)
	wg.Wait()
	close(faults)
	for err := range faults {
		t.Error(err)
	}
}
