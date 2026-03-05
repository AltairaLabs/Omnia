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
	"fmt"
	"sync"
	"time"

	"github.com/sony/gobreaker/v2"
)

const (
	// cbMaxConsecutiveFailures is the number of consecutive failures before the circuit opens.
	cbMaxConsecutiveFailures = 5

	// cbOpenTimeout is how long the circuit stays open before transitioning to half-open.
	cbOpenTimeout = 30 * time.Second

	// cbHalfOpenMaxRequests is the number of requests allowed in the half-open state.
	cbHalfOpenMaxRequests = 1
)

// ToolCircuitBreakers manages per-tool circuit breakers to prevent repeated
// calls to failing tool endpoints.
type ToolCircuitBreakers struct {
	mu       sync.RWMutex
	breakers map[string]*gobreaker.CircuitBreaker[[]byte]
	settings gobreaker.Settings
}

// NewToolCircuitBreakers creates a new circuit breaker registry with default settings:
// open after 5 consecutive failures, half-open after 30s, close after 1 success.
func NewToolCircuitBreakers() *ToolCircuitBreakers {
	return &ToolCircuitBreakers{
		breakers: make(map[string]*gobreaker.CircuitBreaker[[]byte]),
		settings: gobreaker.Settings{
			MaxRequests: cbHalfOpenMaxRequests,
			Timeout:     cbOpenTimeout,
			ReadyToTrip: consecutiveFailureTrip,
		},
	}
}

// Execute runs fn through the circuit breaker for the given tool name.
// If the circuit is open, it returns immediately with an error instead of
// waiting for the full request timeout.
func (t *ToolCircuitBreakers) Execute(toolName string, fn func() ([]byte, error)) ([]byte, error) {
	cb := t.getOrCreate(toolName)

	result, err := cb.Execute(fn)
	if err != nil {
		return nil, fmt.Errorf("circuit breaker [%s]: %w", toolName, err)
	}
	return result, nil
}

// getOrCreate returns the circuit breaker for a tool, creating one if needed.
func (t *ToolCircuitBreakers) getOrCreate(toolName string) *gobreaker.CircuitBreaker[[]byte] {
	t.mu.RLock()
	cb, ok := t.breakers[toolName]
	t.mu.RUnlock()
	if ok {
		return cb
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	// Double-check after acquiring write lock.
	if cb, ok = t.breakers[toolName]; ok {
		return cb
	}

	settings := t.settings
	settings.Name = toolName
	cb = gobreaker.NewCircuitBreaker[[]byte](settings)
	t.breakers[toolName] = cb
	return cb
}

// consecutiveFailureTrip opens the circuit after cbMaxConsecutiveFailures consecutive failures.
func consecutiveFailureTrip(counts gobreaker.Counts) bool {
	return counts.ConsecutiveFailures >= cbMaxConsecutiveFailures
}
