/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package tools

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sony/gobreaker/v2"
)

func TestToolCircuitBreakers_SuccessPassesThrough(t *testing.T) {
	cbs := NewToolCircuitBreakers()

	result, err := cbs.Execute("my-tool", func() ([]byte, error) {
		return []byte("hello"), nil
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if string(result) != "hello" {
		t.Fatalf("expected 'hello', got %q", string(result))
	}
}

func TestToolCircuitBreakers_ErrorPassesThrough(t *testing.T) {
	cbs := NewToolCircuitBreakers()

	_, err := cbs.Execute("my-tool", func() ([]byte, error) {
		return nil, errors.New("boom")
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected error to contain 'boom', got %q", err.Error())
	}
}

func TestToolCircuitBreakers_OpensAfterConsecutiveFailures(t *testing.T) {
	cbs := NewToolCircuitBreakers()
	toolErr := errors.New("endpoint down")

	// Trigger 5 consecutive failures to open the circuit.
	for i := 0; i < cbMaxConsecutiveFailures; i++ {
		_, _ = cbs.Execute("flaky-tool", func() ([]byte, error) {
			return nil, toolErr
		})
	}

	// The 6th call should be rejected immediately by the open circuit.
	_, err := cbs.Execute("flaky-tool", func() ([]byte, error) {
		t.Fatal("function should not be called when circuit is open")
		return nil, nil
	})
	if err == nil {
		t.Fatal("expected circuit breaker error, got nil")
	}
	if !errors.Is(err, gobreaker.ErrOpenState) {
		t.Fatalf("expected ErrOpenState in error chain, got %v", err)
	}
}

func TestToolCircuitBreakers_SuccessResetsBreaker(t *testing.T) {
	cbs := NewToolCircuitBreakers()

	// Accumulate 4 failures (one short of opening).
	for i := 0; i < cbMaxConsecutiveFailures-1; i++ {
		_, _ = cbs.Execute("reset-tool", func() ([]byte, error) {
			return nil, errors.New("fail")
		})
	}

	// A success should reset the consecutive failure counter.
	_, err := cbs.Execute("reset-tool", func() ([]byte, error) {
		return []byte("ok"), nil
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}

	// Another 4 failures should NOT open the circuit (counter was reset).
	for i := 0; i < cbMaxConsecutiveFailures-1; i++ {
		_, _ = cbs.Execute("reset-tool", func() ([]byte, error) {
			return nil, errors.New("fail again")
		})
	}

	// This call should still execute (circuit is closed).
	called := false
	_, _ = cbs.Execute("reset-tool", func() ([]byte, error) {
		called = true
		return []byte("still works"), nil
	})
	if !called {
		t.Fatal("expected function to be called, but circuit was open")
	}
}

func TestToolCircuitBreakers_AllowsRetryAfterTimeout(t *testing.T) {
	// Create breakers with a very short timeout for testing.
	cbs := &ToolCircuitBreakers{
		breakers: make(map[string]*gobreaker.CircuitBreaker[[]byte]),
		settings: gobreaker.Settings{
			MaxRequests: cbHalfOpenMaxRequests,
			Timeout:     100 * time.Millisecond,
			ReadyToTrip: consecutiveFailureTrip,
		},
	}

	// Open the circuit.
	for i := 0; i < cbMaxConsecutiveFailures; i++ {
		_, _ = cbs.Execute("timeout-tool", func() ([]byte, error) {
			return nil, errors.New("fail")
		})
	}

	// Verify it is open.
	_, err := cbs.Execute("timeout-tool", func() ([]byte, error) {
		return []byte("should not run"), nil
	})
	if !errors.Is(err, gobreaker.ErrOpenState) {
		t.Fatalf("expected ErrOpenState, got %v", err)
	}

	// Wait for the timeout to elapse so the breaker transitions to half-open.
	time.Sleep(150 * time.Millisecond)

	// The next call should be allowed (half-open state).
	result, err := cbs.Execute("timeout-tool", func() ([]byte, error) {
		return []byte("recovered"), nil
	})
	if err != nil {
		t.Fatalf("expected success after timeout, got %v", err)
	}
	if string(result) != "recovered" {
		t.Fatalf("expected 'recovered', got %q", string(result))
	}
}

func TestToolCircuitBreakers_ConcurrentDifferentTools(t *testing.T) {
	cbs := NewToolCircuitBreakers()
	const numTools = 10
	const numCalls = 20

	var wg sync.WaitGroup
	errs := make([]error, numTools*numCalls)

	for i := 0; i < numTools; i++ {
		for j := 0; j < numCalls; j++ {
			idx := i*numCalls + j
			toolName := "tool-" + string(rune('A'+i))
			wg.Add(1)
			go func(index int, name string) {
				defer wg.Done()
				_, err := cbs.Execute(name, func() ([]byte, error) {
					return []byte("ok"), nil
				})
				errs[index] = err
			}(idx, toolName)
		}
	}

	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("call %d: unexpected error: %v", i, err)
		}
	}
}

func TestToolCircuitBreakers_IsolationBetweenTools(t *testing.T) {
	cbs := NewToolCircuitBreakers()

	// Open circuit for "broken-tool".
	for i := 0; i < cbMaxConsecutiveFailures; i++ {
		_, _ = cbs.Execute("broken-tool", func() ([]byte, error) {
			return nil, errors.New("fail")
		})
	}

	// "broken-tool" should be open.
	_, err := cbs.Execute("broken-tool", func() ([]byte, error) {
		return nil, nil
	})
	if !errors.Is(err, gobreaker.ErrOpenState) {
		t.Fatalf("expected broken-tool circuit to be open, got %v", err)
	}

	// "healthy-tool" should still work fine.
	result, err := cbs.Execute("healthy-tool", func() ([]byte, error) {
		return []byte("fine"), nil
	})
	if err != nil {
		t.Fatalf("expected healthy-tool to succeed, got %v", err)
	}
	if string(result) != "fine" {
		t.Fatalf("expected 'fine', got %q", string(result))
	}
}

func TestNewToolCircuitBreakers_DefaultSettings(t *testing.T) {
	cbs := NewToolCircuitBreakers()

	if cbs.settings.MaxRequests != cbHalfOpenMaxRequests {
		t.Errorf("expected MaxRequests %d, got %d", cbHalfOpenMaxRequests, cbs.settings.MaxRequests)
	}
	if cbs.settings.Timeout != cbOpenTimeout {
		t.Errorf("expected Timeout %v, got %v", cbOpenTimeout, cbs.settings.Timeout)
	}
	if cbs.breakers == nil {
		t.Error("expected breakers map to be initialized")
	}
}

func TestConsecutiveFailureTrip(t *testing.T) {
	tests := []struct {
		name     string
		failures uint32
		want     bool
	}{
		{"below threshold", cbMaxConsecutiveFailures - 1, false},
		{"at threshold", cbMaxConsecutiveFailures, true},
		{"above threshold", cbMaxConsecutiveFailures + 1, true},
		{"zero failures", 0, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			counts := gobreaker.Counts{ConsecutiveFailures: tc.failures}
			got := consecutiveFailureTrip(counts)
			if got != tc.want {
				t.Errorf("consecutiveFailureTrip(%d) = %v, want %v", tc.failures, got, tc.want)
			}
		})
	}
}
