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
	"context"
	"encoding/json"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

// newTestSpan returns a no-op trace.Span for tests.
func newTestSpan() trace.Span {
	tracer := tracenoop.NewTracerProvider().Tracer("test")
	_, span := tracer.Start(context.Background(), "test")
	return span
}

// testPolicy returns a retryPolicy suitable for fast unit tests.
func testPolicy(maxAttempts int32) retryPolicy {
	return retryPolicy{
		MaxAttempts:       maxAttempts,
		InitialBackoff:    5 * time.Millisecond,
		BackoffMultiplier: 2.0,
		MaxBackoff:        50 * time.Millisecond,
	}
}

// alwaysRetryClassify always returns retryable=true with no retryAfter override.
func alwaysRetryClassify(_ error) (bool, time.Duration) {
	return true, 0
}

func TestRetryWithBackoff_SuccessFirstAttempt(t *testing.T) {
	calls := 0
	fn := func(_ context.Context) (json.RawMessage, error) {
		calls++
		return json.RawMessage(`"ok"`), nil
	}

	result, err := retryWithBackoff(
		context.Background(),
		logr.Discard(),
		newTestSpan(),
		testPolicy(3),
		0,
		alwaysRetryClassify,
		fn,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(result) != `"ok"` {
		t.Fatalf("unexpected result: %s", result)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}

func TestRetryWithBackoff_RetryThenSucceed(t *testing.T) {
	calls := 0
	fn := func(_ context.Context) (json.RawMessage, error) {
		calls++
		if calls < 3 {
			return nil, errors.New("transient error")
		}
		return json.RawMessage(`"success"`), nil
	}

	result, err := retryWithBackoff(
		context.Background(),
		logr.Discard(),
		newTestSpan(),
		testPolicy(5),
		0,
		alwaysRetryClassify,
		fn,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(result) != `"success"` {
		t.Fatalf("unexpected result: %s", result)
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}

func TestRetryWithBackoff_NonRetryableError(t *testing.T) {
	sentinel := errors.New("permanent error")
	calls := 0
	classifyCalls := 0

	fn := func(_ context.Context) (json.RawMessage, error) {
		calls++
		return nil, sentinel
	}
	classify := func(_ error) (bool, time.Duration) {
		classifyCalls++
		return false, 0
	}

	_, err := retryWithBackoff(
		context.Background(),
		logr.Discard(),
		newTestSpan(),
		testPolicy(5),
		0,
		classify,
		fn,
	)
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
	if classifyCalls != 1 {
		t.Fatalf("expected 1 classify call, got %d", classifyCalls)
	}
}

func TestRetryWithBackoff_AllAttemptsExhausted(t *testing.T) {
	sentinel := errors.New("always fails")
	calls := 0

	fn := func(_ context.Context) (json.RawMessage, error) {
		calls++
		return nil, sentinel
	}

	const maxAttempts = 4
	_, err := retryWithBackoff(
		context.Background(),
		logr.Discard(),
		newTestSpan(),
		testPolicy(maxAttempts),
		0,
		alwaysRetryClassify,
		fn,
	)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected wrapped sentinel error, got: %v", err)
	}
	if calls != maxAttempts {
		t.Fatalf("expected %d calls, got %d", maxAttempts, calls)
	}
}

func TestRetryWithBackoff_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	calls := 0
	fn := func(_ context.Context) (json.RawMessage, error) {
		calls++
		return nil, errors.New("transient")
	}

	policy := retryPolicy{
		MaxAttempts:       10,
		InitialBackoff:    200 * time.Millisecond, // long enough to be interrupted
		BackoffMultiplier: 1.0,
		MaxBackoff:        200 * time.Millisecond,
	}

	// Cancel the context shortly after the first failure triggers a sleep.
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	_, err := retryWithBackoff(
		ctx,
		logr.Discard(),
		newTestSpan(),
		policy,
		0,
		alwaysRetryClassify,
		fn,
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call before cancel, got %d", calls)
	}
}

func TestRetryWithBackoff_PerAttemptTimeout(t *testing.T) {
	calls := 0
	// fn blocks until its context is cancelled, simulating a slow server.
	// On the 3rd call it returns immediately.
	fn := func(ctx context.Context) (json.RawMessage, error) {
		calls++
		if calls < 3 {
			<-ctx.Done()
			return nil, ctx.Err()
		}
		return json.RawMessage(`"fast"`), nil
	}

	policy := retryPolicy{
		MaxAttempts:       5,
		InitialBackoff:    1 * time.Millisecond,
		BackoffMultiplier: 1.0,
		MaxBackoff:        5 * time.Millisecond,
	}

	result, err := retryWithBackoff(
		context.Background(),
		logr.Discard(),
		newTestSpan(),
		policy,
		10*time.Millisecond, // per-attempt timeout
		alwaysRetryClassify,
		fn,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(result) != `"fast"` {
		t.Fatalf("unexpected result: %s", result)
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}

func TestRetryWithBackoff_SingleAttemptFastPath(t *testing.T) {
	sentinel := errors.New("single shot error")
	calls := 0
	classifyCalls := 0

	fn := func(_ context.Context) (json.RawMessage, error) {
		calls++
		return nil, sentinel
	}
	classify := func(_ error) (bool, time.Duration) {
		classifyCalls++
		return true, 0
	}

	_, err := retryWithBackoff(
		context.Background(),
		logr.Discard(),
		newTestSpan(),
		testPolicy(1), // MaxAttempts=1 → fast path
		0,
		classify,
		fn,
	)
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
	if classifyCalls != 0 {
		t.Fatalf("classify must NOT be called in fast path, got %d calls", classifyCalls)
	}
}

func TestRetryWithBackoff_RetryAfterOverride(t *testing.T) {
	const retryAfter = 50 * time.Millisecond
	calls := 0

	fn := func(_ context.Context) (json.RawMessage, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("rate limited")
		}
		return json.RawMessage(`"ok"`), nil
	}
	classify := func(_ error) (bool, time.Duration) {
		return true, retryAfter
	}

	policy := retryPolicy{
		MaxAttempts:       3,
		InitialBackoff:    1 * time.Millisecond, // much shorter than retryAfter
		BackoffMultiplier: 1.0,
		MaxBackoff:        200 * time.Millisecond,
	}

	start := time.Now()
	result, err := retryWithBackoff(
		context.Background(),
		logr.Discard(),
		newTestSpan(),
		policy,
		0,
		classify,
		fn,
	)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(result) != `"ok"` {
		t.Fatalf("unexpected result: %s", result)
	}
	// Allow 10% under for scheduling jitter (50ms * 0.9 = 45ms).
	const minElapsed = 45 * time.Millisecond
	if elapsed < minElapsed {
		t.Fatalf("expected at least %v delay (retryAfter override), got %v", minElapsed, elapsed)
	}
}

func TestBackoffDelay_ExponentialGrowth(t *testing.T) {
	policy := retryPolicy{
		InitialBackoff:    100 * time.Millisecond,
		BackoffMultiplier: 2.0,
		MaxBackoff:        10 * time.Second,
	}

	// Compute expected base delays: 100ms, 200ms, 400ms, 800ms.
	// With ±10% jitter the actual value must be within ±15% (extra tolerance for math).
	expected := []time.Duration{
		100 * time.Millisecond,
		200 * time.Millisecond,
		400 * time.Millisecond,
		800 * time.Millisecond,
	}
	const tolerance = 0.15

	for attempt, want := range expected {
		got := backoffDelay(policy, attempt, 0)
		lo := time.Duration(float64(want) * (1 - tolerance))
		hi := time.Duration(float64(want) * (1 + tolerance))
		if got < lo || got > hi {
			t.Errorf("attempt %d: want %v ± %d%%, got %v", attempt, want, int(tolerance*100), got)
		}
	}

	// Also verify MaxBackoff is respected.
	got := backoffDelay(policy, 100, 0) // very high attempt → would overflow without cap
	if got > policy.MaxBackoff {
		t.Errorf("expected delay capped at %v, got %v", policy.MaxBackoff, got)
	}

	// Verify exponential growth — each value must be strictly larger than the previous.
	for i := 1; i < len(expected); i++ {
		d0 := float64(expected[i-1])
		d1 := float64(expected[i])
		ratio := d1 / d0
		// Ratio should be close to BackoffMultiplier (2.0), allowing 15% jitter.
		if math.Abs(ratio-policy.BackoffMultiplier) > policy.BackoffMultiplier*tolerance {
			t.Errorf("attempt %d→%d: expected ratio ~%.1f, got %.3f", i-1, i, policy.BackoffMultiplier, ratio)
		}
	}
}
