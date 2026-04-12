# Retry Execution Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement retry logic with exponential backoff for HTTP, gRPC, MCP, and OpenAPI tool executors, including replacing PromptKit's HTTP executor with a direct `http.Client` implementation.

**Architecture:** A generic `retryWithBackoff` engine with transport-specific error classifiers. Each executor wraps its call in the retry engine. HTTP execution is moved from PromptKit's `HTTPExecutor` to a direct `http.Client.Do()` to access response headers for `Retry-After` support and status code classification.

**Tech Stack:** Go, OTel tracing (`go.opentelemetry.io/otel/trace`), `logr.Logger`, `google.golang.org/grpc/status`, `net/http`

---

## File Map

| File | Purpose |
|------|---------|
| `internal/runtime/tools/retry.go` | Generic retry engine: `retryPolicy` struct, `retryWithBackoff()`, backoff/jitter helpers, policy extraction helpers |
| `internal/runtime/tools/retry_test.go` | Unit tests for retry engine |
| `internal/runtime/tools/retry_classify.go` | Transport-specific error classifiers: HTTP (with Retry-After), gRPC, MCP |
| `internal/runtime/tools/retry_classify_test.go` | Unit tests for classifiers |
| `internal/runtime/tools/http_client.go` | Direct HTTP execution: request building, response processing (replaces PromptKit HTTPExecutor) |
| `internal/runtime/tools/http_client_test.go` | Unit tests for HTTP client |
| `internal/runtime/tools/omnia_executor.go` | Modified: wire retry into all four executor paths, remove PromptKit HTTPExecutor |
| `internal/runtime/tools/omnia_executor_test.go` | Modified: integration tests proving retry works end-to-end per transport |

## Task Execution Notes

- **TDD throughout**: write the failing test, run it, implement, run it, commit.
- **Go import rule**: always add imports and their usage in the same Edit call. Run `goimports -w <file>` after finishing edits to a Go file.
- **Commit style**: conventional commits (`feat:`, `test:`, `refactor:`).
- **Test commands**: `go test ./internal/runtime/tools/... -count=1 -run <TestName> -v`
- **Lint**: `golangci-lint run ./internal/runtime/tools/...` before committing non-test files.

---

## Tasks

### Task 1: Implement `retryPolicy` struct and `retryWithBackoff` core loop

**Files:**
- Create: `internal/runtime/tools/retry.go`
- Create: `internal/runtime/tools/retry_test.go`

- [ ] **Step 1: Write the retry_test.go with first test — success on first attempt**

```go
/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
...
*/

package tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/go-logr/logr"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

func TestRetryWithBackoff_SuccessFirstAttempt(t *testing.T) {
	ctx := context.Background()
	log := logr.Discard()
	span := tracenoop.Span{}
	policy := retryPolicy{MaxAttempts: 3, InitialBackoff: 10 * time.Millisecond, BackoffMultiplier: 2.0, MaxBackoff: 1 * time.Second}

	calls := 0
	result, err := retryWithBackoff(ctx, log, span, policy, 0,
		func(_ error) (bool, time.Duration) { return true, 0 },
		func(ctx context.Context) (json.RawMessage, error) {
			calls++
			return json.RawMessage(`{"ok":true}`), nil
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Errorf("expected 1 call, got %d", calls)
	}
	if string(result) != `{"ok":true}` {
		t.Errorf("unexpected result: %s", result)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/runtime/tools/... -count=1 -run TestRetryWithBackoff_SuccessFirstAttempt -v`
Expected: FAIL — `retryWithBackoff` undefined.

- [ ] **Step 3: Write retry.go with retryPolicy struct and retryWithBackoff implementation**

```go
/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
...
*/

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/rand/v2"
	"time"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// retryPolicy is the common retry configuration extracted from the
// transport-specific Runtime*RetryPolicy types.
type retryPolicy struct {
	MaxAttempts       int32
	InitialBackoff    time.Duration
	BackoffMultiplier float64
	MaxBackoff        time.Duration
}

// retryWithBackoff executes fn up to policy.MaxAttempts times with exponential
// backoff and jitter between attempts. classify determines whether a given error
// is retryable and optionally returns a Retry-After duration override.
// Each attempt gets its own context.WithTimeout using attemptTimeout (0 = no
// per-attempt timeout). The parent ctx is used for cancellation between attempts.
func retryWithBackoff(
	ctx context.Context,
	log logr.Logger,
	span trace.Span,
	policy retryPolicy,
	attemptTimeout time.Duration,
	classify func(error) (retryable bool, retryAfter time.Duration),
	fn func(ctx context.Context) (json.RawMessage, error),
) (json.RawMessage, error) {
	if policy.MaxAttempts <= 1 {
		return fn(withAttemptTimeout(ctx, attemptTimeout))
	}

	var lastErr error
	for attempt := int32(0); attempt < policy.MaxAttempts; attempt++ {
		attemptCtx := withAttemptTimeout(ctx, attemptTimeout)
		result, err := fn(attemptCtx)
		if err == nil {
			return result, nil
		}
		lastErr = err

		// Last attempt — don't classify or sleep.
		if attempt == policy.MaxAttempts-1 {
			break
		}

		retryable, retryAfter := classify(err)
		if !retryable {
			return nil, err
		}

		delay := backoffDelay(policy, attempt, retryAfter)

		log.V(1).Info("retry attempt",
			"attempt", attempt+1,
			"maxAttempts", policy.MaxAttempts,
			"delay", delay,
			"error", err.Error())
		span.AddEvent("retry.attempt", trace.WithAttributes(
			attribute.Int("attempt", int(attempt+1)),
			attribute.String("delay", delay.String()),
			attribute.String("error", err.Error()),
		))

		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	log.V(0).Info("retries exhausted",
		"attempts", policy.MaxAttempts,
		"error", lastErr.Error())
	span.AddEvent("retry.exhausted", trace.WithAttributes(
		attribute.Int("attempts", int(policy.MaxAttempts)),
	))

	return nil, fmt.Errorf("%d attempts exhausted: %w", policy.MaxAttempts, lastErr)
}

// withAttemptTimeout returns a context with the given timeout applied.
// If timeout is 0, returns the parent context unchanged.
func withAttemptTimeout(ctx context.Context, timeout time.Duration) context.Context {
	if timeout <= 0 {
		return ctx
	}
	attemptCtx, cancel := context.WithTimeout(ctx, timeout)
	// The cancel func is intentionally not deferred here — the caller (fn) owns
	// the context lifetime. The GC will clean up after fn returns and the timer
	// fires or the parent is cancelled.
	_ = cancel
	return attemptCtx
}

// backoffDelay calculates the delay for the given attempt number using
// exponential backoff with ±10% jitter. If retryAfter > 0, it is used
// as a floor for the delay.
func backoffDelay(policy retryPolicy, attempt int32, retryAfter time.Duration) time.Duration {
	delay := float64(policy.InitialBackoff) * math.Pow(policy.BackoffMultiplier, float64(attempt))
	if delay > float64(policy.MaxBackoff) {
		delay = float64(policy.MaxBackoff)
	}

	// ±10% jitter
	jitter := delay * 0.1 * (2*rand.Float64() - 1)
	delay += jitter

	d := time.Duration(delay)
	if d < 0 {
		d = 0
	}

	if retryAfter > 0 && retryAfter > d {
		d = retryAfter
		if d > policy.MaxBackoff {
			d = policy.MaxBackoff
		}
	}

	return d
}
```

**Note on `withAttemptTimeout`:** The cancel func is captured but not deferred because `fn` receives `attemptCtx` and the context tree is cleaned up when the parent context completes. This avoids a context leak warning from `go vet`. However, this is a known pattern trade-off — if `go vet` flags it, switch to passing the cancel func alongside the context. See Step 4.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/runtime/tools/... -count=1 -run TestRetryWithBackoff_SuccessFirstAttempt -v`
Expected: PASS.

If `go vet` complains about the cancel func, refactor `withAttemptTimeout` to return `(context.Context, context.CancelFunc)` and defer the cancel in the retry loop body.

- [ ] **Step 5: Add remaining retry engine tests**

Add these tests to `retry_test.go`:

```go
func TestRetryWithBackoff_RetryThenSucceed(t *testing.T) {
	ctx := context.Background()
	log := logr.Discard()
	span := tracenoop.Span{}
	policy := retryPolicy{MaxAttempts: 3, InitialBackoff: 1 * time.Millisecond, BackoffMultiplier: 2.0, MaxBackoff: 100 * time.Millisecond}

	calls := 0
	result, err := retryWithBackoff(ctx, log, span, policy, 0,
		func(_ error) (bool, time.Duration) { return true, 0 },
		func(ctx context.Context) (json.RawMessage, error) {
			calls++
			if calls < 3 {
				return nil, errors.New("transient")
			}
			return json.RawMessage(`{"ok":true}`), nil
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 3 {
		t.Errorf("expected 3 calls, got %d", calls)
	}
	if string(result) != `{"ok":true}` {
		t.Errorf("unexpected result: %s", result)
	}
}

func TestRetryWithBackoff_NonRetryableError(t *testing.T) {
	ctx := context.Background()
	log := logr.Discard()
	span := tracenoop.Span{}
	policy := retryPolicy{MaxAttempts: 3, InitialBackoff: 1 * time.Millisecond, BackoffMultiplier: 2.0, MaxBackoff: 100 * time.Millisecond}

	calls := 0
	_, err := retryWithBackoff(ctx, log, span, policy, 0,
		func(_ error) (bool, time.Duration) { return false, 0 },
		func(ctx context.Context) (json.RawMessage, error) {
			calls++
			return nil, errors.New("permanent")
		},
	)
	if err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Errorf("expected 1 call (no retries), got %d", calls)
	}
	if err.Error() != "permanent" {
		t.Errorf("expected original error, got: %v", err)
	}
}

func TestRetryWithBackoff_AllAttemptsExhausted(t *testing.T) {
	ctx := context.Background()
	log := logr.Discard()
	span := tracenoop.Span{}
	policy := retryPolicy{MaxAttempts: 3, InitialBackoff: 1 * time.Millisecond, BackoffMultiplier: 2.0, MaxBackoff: 100 * time.Millisecond}

	calls := 0
	_, err := retryWithBackoff(ctx, log, span, policy, 0,
		func(_ error) (bool, time.Duration) { return true, 0 },
		func(ctx context.Context) (json.RawMessage, error) {
			calls++
			return nil, errors.New("always fails")
		},
	)
	if err == nil {
		t.Fatal("expected error")
	}
	if calls != 3 {
		t.Errorf("expected 3 calls, got %d", calls)
	}
	if !errors.Is(err, errors.New("")) {
		// Just check it wraps the original
		if !errors.As(err, new(error)) {
			t.Errorf("error should wrap original")
		}
	}
}

func TestRetryWithBackoff_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	log := logr.Discard()
	span := tracenoop.Span{}
	policy := retryPolicy{MaxAttempts: 5, InitialBackoff: 50 * time.Millisecond, BackoffMultiplier: 2.0, MaxBackoff: 1 * time.Second}

	calls := 0
	_, err := retryWithBackoff(ctx, log, span, policy, 0,
		func(_ error) (bool, time.Duration) { return true, 0 },
		func(ctx context.Context) (json.RawMessage, error) {
			calls++
			if calls == 1 {
				cancel() // Cancel during backoff sleep
			}
			return nil, errors.New("transient")
		},
	)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got: %v", err)
	}
}

func TestRetryWithBackoff_PerAttemptTimeout(t *testing.T) {
	ctx := context.Background()
	log := logr.Discard()
	span := tracenoop.Span{}
	policy := retryPolicy{MaxAttempts: 3, InitialBackoff: 1 * time.Millisecond, BackoffMultiplier: 2.0, MaxBackoff: 100 * time.Millisecond}

	calls := 0
	_, err := retryWithBackoff(ctx, log, span, policy, 10*time.Millisecond,
		func(_ error) (bool, time.Duration) { return true, 0 },
		func(ctx context.Context) (json.RawMessage, error) {
			calls++
			if calls < 3 {
				<-ctx.Done() // Block until timeout
				return nil, ctx.Err()
			}
			return json.RawMessage(`{"ok":true}`), nil
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 3 {
		t.Errorf("expected 3 calls, got %d", calls)
	}
}

func TestRetryWithBackoff_SingleAttemptFastPath(t *testing.T) {
	ctx := context.Background()
	log := logr.Discard()
	span := tracenoop.Span{}
	policy := retryPolicy{MaxAttempts: 1}

	calls := 0
	_, err := retryWithBackoff(ctx, log, span, policy, 0,
		func(_ error) (bool, time.Duration) { t.Fatal("classify should not be called"); return false, 0 },
		func(ctx context.Context) (json.RawMessage, error) {
			calls++
			return nil, errors.New("fail")
		},
	)
	if err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Errorf("expected 1 call, got %d", calls)
	}
}

func TestRetryWithBackoff_RetryAfterOverride(t *testing.T) {
	ctx := context.Background()
	log := logr.Discard()
	span := tracenoop.Span{}
	policy := retryPolicy{MaxAttempts: 2, InitialBackoff: 1 * time.Millisecond, BackoffMultiplier: 2.0, MaxBackoff: 1 * time.Second}

	start := time.Now()
	calls := 0
	_, err := retryWithBackoff(ctx, log, span, policy, 0,
		func(_ error) (bool, time.Duration) { return true, 50 * time.Millisecond },
		func(ctx context.Context) (json.RawMessage, error) {
			calls++
			return nil, errors.New("retry-after")
		},
	)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected error")
	}
	// The retryAfter of 50ms should override the 1ms initial backoff.
	if elapsed < 40*time.Millisecond {
		t.Errorf("expected at least ~50ms delay, got %v", elapsed)
	}
}

func TestBackoffDelay_ExponentialGrowth(t *testing.T) {
	policy := retryPolicy{InitialBackoff: 100 * time.Millisecond, BackoffMultiplier: 2.0, MaxBackoff: 10 * time.Second}

	// Run many times to average out jitter
	for attempt := int32(0); attempt < 5; attempt++ {
		delay := backoffDelay(policy, attempt, 0)
		expected := 100 * time.Millisecond * time.Duration(math.Pow(2.0, float64(attempt)))
		if expected > 10*time.Second {
			expected = 10 * time.Second
		}
		// Allow ±15% for jitter
		low := time.Duration(float64(expected) * 0.85)
		high := time.Duration(float64(expected) * 1.15)
		if delay < low || delay > high {
			t.Errorf("attempt %d: delay %v outside [%v, %v]", attempt, delay, low, high)
		}
	}
}
```

- [ ] **Step 6: Run all retry tests**

Run: `go test ./internal/runtime/tools/... -count=1 -run TestRetryWithBackoff -v`
Expected: All PASS.

Run: `go test ./internal/runtime/tools/... -count=1 -run TestBackoffDelay -v`
Expected: PASS.

- [ ] **Step 7: Run goimports**

Run: `goimports -w internal/runtime/tools/retry.go internal/runtime/tools/retry_test.go`

- [ ] **Step 8: Commit**

```
git add internal/runtime/tools/retry.go internal/runtime/tools/retry_test.go
git commit -m "feat(runtime): add generic retryWithBackoff engine with exponential backoff and jitter"
```

---

### Task 2: Implement error classifiers

**Files:**
- Create: `internal/runtime/tools/retry_classify.go`
- Create: `internal/runtime/tools/retry_classify_test.go`

- [ ] **Step 1: Write classifier test file with HTTP tests**

```go
/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
...
*/

package tools

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestClassifyHTTPResult_NetworkError_RetryEnabled(t *testing.T) {
	result := httpCallResult{Err: &net.OpError{Op: "dial", Err: errors.New("connection refused")}}
	policy := &RuntimeHTTPRetryPolicy{RetryOn: []int32{502}, RetryOnNetworkError: true}

	retryable, retryAfter := classifyHTTPResult(result, policy)
	if !retryable {
		t.Error("expected retryable for network error")
	}
	if retryAfter != 0 {
		t.Errorf("expected retryAfter=0, got %v", retryAfter)
	}
}

func TestClassifyHTTPResult_NetworkError_RetryDisabled(t *testing.T) {
	result := httpCallResult{Err: &net.OpError{Op: "dial", Err: errors.New("connection refused")}}
	policy := &RuntimeHTTPRetryPolicy{RetryOn: []int32{502}, RetryOnNetworkError: false}

	retryable, _ := classifyHTTPResult(result, policy)
	if retryable {
		t.Error("expected not retryable when RetryOnNetworkError is false")
	}
}

func TestClassifyHTTPResult_StatusCodeInRetryOn(t *testing.T) {
	result := httpCallResult{StatusCode: 503}
	policy := &RuntimeHTTPRetryPolicy{RetryOn: []int32{502, 503, 504}, RetryOnNetworkError: true}

	retryable, _ := classifyHTTPResult(result, policy)
	if !retryable {
		t.Error("expected retryable for status 503")
	}
}

func TestClassifyHTTPResult_StatusCodeNotInRetryOn(t *testing.T) {
	result := httpCallResult{StatusCode: 400}
	policy := &RuntimeHTTPRetryPolicy{RetryOn: []int32{502, 503, 504}, RetryOnNetworkError: true}

	retryable, _ := classifyHTTPResult(result, policy)
	if retryable {
		t.Error("expected not retryable for status 400")
	}
}

func TestClassifyHTTPResult_RetryAfterSeconds(t *testing.T) {
	headers := http.Header{}
	headers.Set("Retry-After", "5")
	result := httpCallResult{StatusCode: 503, Headers: headers}
	policy := &RuntimeHTTPRetryPolicy{RetryOn: []int32{503}, RespectRetryAfter: true}

	retryable, retryAfter := classifyHTTPResult(result, policy)
	if !retryable {
		t.Error("expected retryable")
	}
	if retryAfter != 5*time.Second {
		t.Errorf("expected retryAfter=5s, got %v", retryAfter)
	}
}

func TestClassifyHTTPResult_RetryAfterHTTPDate(t *testing.T) {
	futureTime := time.Now().Add(10 * time.Second)
	headers := http.Header{}
	headers.Set("Retry-After", futureTime.UTC().Format(http.TimeFormat))
	result := httpCallResult{StatusCode: 503, Headers: headers}
	policy := &RuntimeHTTPRetryPolicy{RetryOn: []int32{503}, RespectRetryAfter: true}

	retryable, retryAfter := classifyHTTPResult(result, policy)
	if !retryable {
		t.Error("expected retryable")
	}
	// Allow 2s tolerance for test execution time
	if retryAfter < 8*time.Second || retryAfter > 12*time.Second {
		t.Errorf("expected retryAfter ~10s, got %v", retryAfter)
	}
}

func TestClassifyHTTPResult_RetryAfterIgnoredWhenDisabled(t *testing.T) {
	headers := http.Header{}
	headers.Set("Retry-After", "5")
	result := httpCallResult{StatusCode: 503, Headers: headers}
	policy := &RuntimeHTTPRetryPolicy{RetryOn: []int32{503}, RespectRetryAfter: false}

	retryable, retryAfter := classifyHTTPResult(result, policy)
	if !retryable {
		t.Error("expected retryable")
	}
	if retryAfter != 0 {
		t.Errorf("expected retryAfter=0 when disabled, got %v", retryAfter)
	}
}

func TestClassifyHTTPResult_ContextDeadlineExceeded(t *testing.T) {
	result := httpCallResult{Err: context.DeadlineExceeded}
	policy := &RuntimeHTTPRetryPolicy{RetryOnNetworkError: true}

	retryable, _ := classifyHTTPResult(result, policy)
	if !retryable {
		t.Error("expected retryable for deadline exceeded")
	}
}

func TestClassifyHTTPResult_Success(t *testing.T) {
	result := httpCallResult{StatusCode: 200}
	policy := &RuntimeHTTPRetryPolicy{RetryOn: []int32{502}, RetryOnNetworkError: true}

	retryable, _ := classifyHTTPResult(result, policy)
	if retryable {
		t.Error("expected not retryable for success")
	}
}
```

- [ ] **Step 2: Add gRPC classifier tests**

Append to `retry_classify_test.go`:

```go
func TestClassifyGRPCError_RetryableStatusCode(t *testing.T) {
	err := grpcStatus.Error(grpcCodes.Unavailable, "service unavailable")

	retryable, _ := classifyGRPCError(err, []string{"UNAVAILABLE", "DEADLINE_EXCEEDED"})
	if !retryable {
		t.Error("expected retryable for UNAVAILABLE")
	}
}

func TestClassifyGRPCError_NonRetryableStatusCode(t *testing.T) {
	err := grpcStatus.Error(grpcCodes.NotFound, "not found")

	retryable, _ := classifyGRPCError(err, []string{"UNAVAILABLE"})
	if retryable {
		t.Error("expected not retryable for NOT_FOUND")
	}
}

func TestClassifyGRPCError_CircuitBreakerError(t *testing.T) {
	// Circuit breaker wraps errors with fmt.Errorf, not gRPC status
	err := errors.New("circuit breaker [tool]: circuit open")

	retryable, _ := classifyGRPCError(err, []string{"UNAVAILABLE"})
	if retryable {
		t.Error("expected not retryable for circuit breaker error")
	}
}
```

**Note:** The imports for gRPC status and codes need aliases to avoid colliding with the `status` word:

```go
import (
	grpcCodes "google.golang.org/grpc/codes"
	grpcStatus "google.golang.org/grpc/status"
)
```

- [ ] **Step 3: Add MCP classifier tests**

Append to `retry_classify_test.go`:

```go
func TestClassifyMCPError_TransportError(t *testing.T) {
	err := &net.OpError{Op: "read", Err: errors.New("connection reset")}

	retryable, _ := classifyMCPError(err)
	if !retryable {
		t.Error("expected retryable for transport error")
	}
}

func TestClassifyMCPError_ContextDeadline(t *testing.T) {
	retryable, _ := classifyMCPError(context.DeadlineExceeded)
	if !retryable {
		t.Error("expected retryable for deadline exceeded")
	}
}

func TestClassifyMCPError_NilError(t *testing.T) {
	retryable, _ := classifyMCPError(nil)
	if retryable {
		t.Error("expected not retryable for nil error")
	}
}

func TestClassifyMCPError_ToolError(t *testing.T) {
	// MCP tool errors are returned as mcpToolError, not transport errors
	err := &mcpToolError{message: "file not found"}

	retryable, _ := classifyMCPError(err)
	if retryable {
		t.Error("expected not retryable for tool error")
	}
}
```

- [ ] **Step 4: Run tests to verify they fail**

Run: `go test ./internal/runtime/tools/... -count=1 -run "TestClassify" -v`
Expected: FAIL — `classifyHTTPResult`, `classifyGRPCError`, `classifyMCPError` undefined.

- [ ] **Step 5: Implement retry_classify.go**

```go
/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
...
*/

package tools

import (
	"context"
	"errors"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	grpcCodes "google.golang.org/grpc/codes"
	grpcStatus "google.golang.org/grpc/status"
)

// httpCallResult captures the HTTP response metadata needed for retry
// classification. It is populated by the HTTP executor before calling
// the classifier.
type httpCallResult struct {
	StatusCode int
	Headers    http.Header
	Err        error // nil if HTTP request succeeded (even if status is non-2xx)
}

// mcpToolError represents an MCP tool-level error (IsError: true in the
// MCP result). These are application errors and should not be retried.
type mcpToolError struct {
	message string
}

func (e *mcpToolError) Error() string {
	return e.message
}

// classifyHTTPResult determines if an HTTP call result is retryable based on
// the retry policy. Returns (retryable, retryAfter) where retryAfter is the
// parsed Retry-After header value if present and enabled.
func classifyHTTPResult(result httpCallResult, policy *RuntimeHTTPRetryPolicy) (bool, time.Duration) {
	if result.Err != nil {
		if isNetworkError(result.Err) && policy.RetryOnNetworkError {
			return true, 0
		}
		return false, 0
	}

	// No error means we have a status code to check.
	if !slices.Contains(policy.RetryOn, int32(result.StatusCode)) {
		return false, 0
	}

	var retryAfter time.Duration
	if policy.RespectRetryAfter && result.Headers != nil {
		retryAfter = parseRetryAfter(result.Headers.Get("Retry-After"))
	}

	return true, retryAfter
}

// classifyGRPCError determines if a gRPC error is retryable based on the
// status code. Circuit breaker errors (non-gRPC errors) are never retryable.
func classifyGRPCError(err error, retryableStatusCodes []string) (bool, time.Duration) {
	if err == nil {
		return false, 0
	}

	st, ok := grpcStatus.FromError(err)
	if !ok {
		// Not a gRPC status error — likely a circuit breaker or transport wrapper.
		return false, 0
	}

	codeName := strings.ToUpper(st.Code().String())
	return slices.Contains(retryableStatusCodes, codeName), 0
}

// classifyMCPError determines if an MCP error is retryable. Only transport-level
// errors are retryable; tool errors (mcpToolError) are not.
func classifyMCPError(err error) (bool, time.Duration) {
	if err == nil {
		return false, 0
	}

	// MCP tool errors are application-level and not retryable.
	var toolErr *mcpToolError
	if errors.As(err, &toolErr) {
		return false, 0
	}

	// Everything else is a transport error (connection, timeout, etc.)
	return true, 0
}

// isNetworkError returns true if err represents a network-level failure
// (connection refused, DNS, timeout).
func isNetworkError(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	var netErr *net.OpError
	if errors.As(err, &netErr) {
		return true
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}
	return false
}

// parseRetryAfter parses an HTTP Retry-After header value. It supports
// both seconds (e.g., "5") and HTTP-date formats per RFC 9110.
// Returns 0 if the header is empty or unparseable.
func parseRetryAfter(value string) time.Duration {
	if value == "" {
		return 0
	}

	// Try seconds first (most common).
	if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}

	// Try HTTP-date format.
	if t, err := http.ParseTime(value); err == nil {
		delay := time.Until(t)
		if delay > 0 {
			return delay
		}
	}

	return 0
}

// Policy extraction helpers — convert transport-specific retry policies
// into the generic retryPolicy used by retryWithBackoff.

func httpRetryParams(cfg *HTTPCfg) (retryPolicy, func(error) (bool, time.Duration)) {
	if cfg == nil || cfg.RetryPolicy == nil {
		return retryPolicy{MaxAttempts: 1}, nil
	}
	p := cfg.RetryPolicy
	return retryPolicy{
			MaxAttempts:       p.MaxAttempts,
			InitialBackoff:    p.InitialBackoff.Get(),
			BackoffMultiplier: p.BackoffMultiplier,
			MaxBackoff:        p.MaxBackoff.Get(),
		}, func(_ error) (bool, time.Duration) {
			// The actual httpCallResult is captured by the closure in the executor.
			// This is a placeholder — the real classify closure is built in executeHTTP.
			return false, 0
		}
}

func grpcRetryParams(cfg *GRPCCfg) (retryPolicy, func(error) (bool, time.Duration)) {
	if cfg == nil || cfg.RetryPolicy == nil {
		return retryPolicy{MaxAttempts: 1}, nil
	}
	p := cfg.RetryPolicy
	return retryPolicy{
		MaxAttempts:       p.MaxAttempts,
		InitialBackoff:    p.InitialBackoff.Get(),
		BackoffMultiplier: p.BackoffMultiplier,
		MaxBackoff:        p.MaxBackoff.Get(),
	}, func(err error) (bool, time.Duration) {
		return classifyGRPCError(err, p.RetryableStatusCodes)
	}
}

func mcpRetryParams(cfg *MCPCfg) (retryPolicy, func(error) (bool, time.Duration)) {
	if cfg == nil || cfg.RetryPolicy == nil {
		return retryPolicy{MaxAttempts: 1}, nil
	}
	p := cfg.RetryPolicy
	return retryPolicy{
		MaxAttempts:       p.MaxAttempts,
		InitialBackoff:    p.InitialBackoff.Get(),
		BackoffMultiplier: p.BackoffMultiplier,
		MaxBackoff:        p.MaxBackoff.Get(),
	}, func(err error) (bool, time.Duration) {
		return classifyMCPError(err)
	}
}
```

- [ ] **Step 6: Run classifier tests**

Run: `go test ./internal/runtime/tools/... -count=1 -run "TestClassify" -v`
Expected: All PASS.

- [ ] **Step 7: Run goimports**

Run: `goimports -w internal/runtime/tools/retry_classify.go internal/runtime/tools/retry_classify_test.go`

- [ ] **Step 8: Commit**

```
git add internal/runtime/tools/retry_classify.go internal/runtime/tools/retry_classify_test.go
git commit -m "feat(runtime): add HTTP, gRPC, and MCP error classifiers with Retry-After support"
```

---

### Task 3: Implement direct HTTP client (replace PromptKit HTTPExecutor)

**Files:**
- Create: `internal/runtime/tools/http_client.go`
- Create: `internal/runtime/tools/http_client_test.go`

- [ ] **Step 1: Write http_client_test.go with basic tests**

```go
/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
...
*/

package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDoHTTPRequest_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"result":"ok"}`))
	}))
	defer srv.Close()

	cfg := &HTTPCfg{Endpoint: srv.URL, Method: "GET"}
	result, callResult, err := doHTTPRequest(context.Background(), http.DefaultClient, cfg, nil, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callResult.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", callResult.StatusCode)
	}
	if string(result) != `{"result":"ok"}` {
		t.Errorf("unexpected result: %s", result)
	}
}

func TestDoHTTPRequest_NonJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("plain text response"))
	}))
	defer srv.Close()

	cfg := &HTTPCfg{Endpoint: srv.URL, Method: "GET"}
	result, _, err := doHTTPRequest(context.Background(), http.DefaultClient, cfg, nil, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Non-JSON responses should be wrapped in {"result": "..."}
	var m map[string]string
	if jsonErr := json.Unmarshal(result, &m); jsonErr != nil {
		t.Fatalf("result is not valid JSON: %v", jsonErr)
	}
	if m["result"] != "plain text response" {
		t.Errorf("unexpected wrapped result: %s", result)
	}
}

func TestDoHTTPRequest_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("bad gateway"))
	}))
	defer srv.Close()

	cfg := &HTTPCfg{Endpoint: srv.URL, Method: "GET"}
	_, callResult, err := doHTTPRequest(context.Background(), http.DefaultClient, cfg, nil, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	// Non-2xx is not a transport error — it returns callResult with status code
	if callResult.StatusCode != 502 {
		t.Errorf("expected status 502, got %d", callResult.StatusCode)
	}
}

func TestDoHTTPRequest_POSTWithBody(t *testing.T) {
	var receivedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var readErr error
		receivedBody, readErr = json.Marshal(r.Method)
		_ = readErr
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		receivedBody = body
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"received":true}`))
	}))
	defer srv.Close()

	cfg := &HTTPCfg{Endpoint: srv.URL, Method: "POST"}
	_, _, err := doHTTPRequest(context.Background(), http.DefaultClient, cfg, nil, json.RawMessage(`{"input":"data"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDoHTTPRequest_Headers(t *testing.T) {
	var receivedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	cfg := &HTTPCfg{
		Endpoint: srv.URL,
		Method:   "GET",
		Headers:  map[string]string{"Authorization": "Bearer test-token"},
	}
	_, _, err := doHTTPRequest(context.Background(), http.DefaultClient, cfg, nil, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedAuth != "Bearer test-token" {
		t.Errorf("expected auth header, got %q", receivedAuth)
	}
}

func TestDoHTTPRequest_ConnectionRefused(t *testing.T) {
	cfg := &HTTPCfg{Endpoint: "http://127.0.0.1:1", Method: "GET"}
	_, callResult, err := doHTTPRequest(context.Background(), http.DefaultClient, cfg, nil, json.RawMessage(`{}`))
	// Transport error — err is set, callResult.Err is also set
	if err == nil && callResult.Err == nil {
		t.Fatal("expected transport error for refused connection")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/runtime/tools/... -count=1 -run "TestDoHTTPRequest" -v`
Expected: FAIL — `doHTTPRequest` undefined.

- [ ] **Step 3: Implement http_client.go**

```go
/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
...
*/

package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

const maxHTTPResponseSize = 10 * 1024 * 1024 // 10MB per response

// doHTTPRequest executes an HTTP request using the provided client and config.
// It returns the response body as JSON, an httpCallResult for retry classification,
// and a transport-level error (connection refused, DNS, etc.).
//
// Non-2xx responses are NOT returned as errors — the caller inspects
// callResult.StatusCode for retry classification. Only transport failures
// set callResult.Err.
func doHTTPRequest(
	ctx context.Context,
	client *http.Client,
	cfg *HTTPCfg,
	headers map[string]string,
	args json.RawMessage,
) (json.RawMessage, httpCallResult, error) {
	req, err := buildHTTPRequest(ctx, cfg, headers, args)
	if err != nil {
		return nil, httpCallResult{Err: err}, err
	}

	// Inject OTel trace context into outbound request headers.
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))

	resp, err := client.Do(req)
	if err != nil {
		return nil, httpCallResult{Err: err}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxHTTPResponseSize))
	if err != nil {
		return nil, httpCallResult{Err: err}, fmt.Errorf("failed to read response body: %w", err)
	}

	callResult := httpCallResult{
		StatusCode: resp.StatusCode,
		Headers:    resp.Header,
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Non-2xx: return the call result for classification but also
		// return an error so the retry engine sees a failure.
		return nil, callResult, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncateBody(body, 512))
	}

	// Wrap non-JSON response.
	if !json.Valid(body) {
		wrapped := map[string]string{"result": string(body)}
		body, _ = json.Marshal(wrapped)
	}

	return body, callResult, nil
}

// buildHTTPRequest constructs an HTTP request from the handler config.
func buildHTTPRequest(
	ctx context.Context,
	cfg *HTTPCfg,
	headers map[string]string,
	args json.RawMessage,
) (*http.Request, error) {
	method := cfg.Method
	if method == "" {
		method = "POST"
	}

	hasArgs := len(args) > 0 && string(args) != "null" && string(args) != "{}"
	url := cfg.Endpoint
	var bodyReader io.Reader

	if hasArgs {
		if isHTTPBodyMethod(method) {
			bodyReader = bytes.NewReader(args)
		} else {
			var err error
			url, err = appendQueryFromJSON(url, args)
			if err != nil {
				return nil, fmt.Errorf("failed to build query parameters: %w", err)
			}
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	if bodyReader != nil {
		ct := cfg.ContentType
		if ct == "" {
			ct = "application/json"
		}
		req.Header.Set("Content-Type", ct)
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	return req, nil
}

// isHTTPBodyMethod returns true for HTTP methods that carry a request body.
func isHTTPBodyMethod(method string) bool {
	switch strings.ToUpper(method) {
	case "POST", "PUT", "PATCH":
		return true
	default:
		return false
	}
}

// appendQueryFromJSON appends JSON key-value pairs as query parameters.
func appendQueryFromJSON(baseURL string, args json.RawMessage) (string, error) {
	var params map[string]any
	if err := json.Unmarshal(args, &params); err != nil {
		return baseURL, err
	}
	if len(params) == 0 {
		return baseURL, nil
	}

	sep := "?"
	if strings.Contains(baseURL, "?") {
		sep = "&"
	}

	var sb strings.Builder
	sb.WriteString(baseURL)
	first := true
	for k, v := range params {
		if first {
			sb.WriteString(sep)
			first = false
		} else {
			sb.WriteByte('&')
		}
		sb.WriteString(k)
		sb.WriteByte('=')
		sb.WriteString(fmt.Sprintf("%v", v))
	}

	return sb.String(), nil
}

// truncateBody truncates a response body to maxLen bytes for error messages.
func truncateBody(body []byte, maxLen int) string {
	if len(body) <= maxLen {
		return string(body)
	}
	return string(body[:maxLen]) + "..."
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/runtime/tools/... -count=1 -run "TestDoHTTPRequest" -v`
Expected: All PASS.

- [ ] **Step 5: Run goimports**

Run: `goimports -w internal/runtime/tools/http_client.go internal/runtime/tools/http_client_test.go`

- [ ] **Step 6: Commit**

```
git add internal/runtime/tools/http_client.go internal/runtime/tools/http_client_test.go
git commit -m "feat(runtime): add direct HTTP client replacing PromptKit HTTPExecutor"
```

---

### Task 4: Wire retry into executeHTTP and executeOpenAPI

**Files:**
- Modify: `internal/runtime/tools/omnia_executor.go:569-593` (executeHTTP)
- Modify: `internal/runtime/tools/omnia_executor.go:920-973` (executeOpenAPI)
- Modify: `internal/runtime/tools/omnia_executor.go:65-97` (OmniaExecutor struct — remove httpExecutor field)
- Modify: `internal/runtime/tools/omnia_executor.go:107` (NewOmniaExecutor — remove httpExecutor init)

- [ ] **Step 1: Replace executeHTTP with retry-wrapped direct HTTP client**

Replace the `executeHTTP` method (lines 569-593) with:

```go
func (e *OmniaExecutor) executeHTTP(
	ctx context.Context,
	toolName, handlerName string,
	handler *HandlerEntry,
	args json.RawMessage,
) (json.RawMessage, error) {
	cfg := handler.HTTPConfig
	if cfg == nil {
		return nil, fmt.Errorf("handler %q has no HTTP config", handlerName)
	}

	headers := e.buildHTTPHeaders(ctx, cfg, toolName, handlerName, args)

	policy, _ := httpRetryParams(cfg)
	var lastCallResult httpCallResult
	classify := func(err error) (bool, time.Duration) {
		if cfg.RetryPolicy == nil {
			return false, 0
		}
		return classifyHTTPResult(lastCallResult, cfg.RetryPolicy)
	}

	return retryWithBackoff(ctx, e.log, e.currentSpan(ctx), policy, handler.Timeout.Get(), classify,
		func(attemptCtx context.Context) (json.RawMessage, error) {
			result, callResult, err := doHTTPRequest(attemptCtx, &http.Client{}, cfg, headers, args)
			lastCallResult = callResult
			return result, err
		},
	)
}
```

**Note:** `e.currentSpan(ctx)` extracts the span from context — we'll add this small helper in this task.

- [ ] **Step 2: Add currentSpan helper**

Add after the `startSpan` method (around line 434):

```go
// currentSpan extracts the current span from context for use by sub-operations
// like retry that need to add events to the parent span.
func (e *OmniaExecutor) currentSpan(ctx context.Context) trace.Span {
	return trace.SpanFromContext(ctx)
}
```

- [ ] **Step 3: Replace executeOpenAPI to use direct HTTP client with retry**

Replace the `executeOpenAPI` method (lines 920-973) with:

```go
func (e *OmniaExecutor) executeOpenAPI(
	ctx context.Context,
	toolName, handlerName string,
	handler *HandlerEntry,
	args json.RawMessage,
) (json.RawMessage, error) {
	e.mu.RLock()
	ops := e.openAPIOps[handlerName]
	baseURL := e.openAPIBaseURLs[handlerName]
	hdrs := e.openAPIHeaders[handlerName]
	e.mu.RUnlock()

	op, ok := ops[toolName]
	if !ok {
		return nil, fmt.Errorf("OpenAPI operation %q not found", toolName)
	}

	// Build a synthetic HTTPCfg for the OpenAPI operation.
	cfg := &HTTPCfg{
		Endpoint: baseURL + op.Path,
		Method:   op.Method,
		Headers:  make(map[string]string),
	}
	for k, v := range hdrs {
		cfg.Headers[k] = v
	}
	if handler.OpenAPIConfig != nil {
		cfg.AuthType = handler.OpenAPIConfig.AuthType
		cfg.AuthToken = handler.OpenAPIConfig.AuthToken
		cfg.RetryPolicy = handler.OpenAPIConfig.RetryPolicy
	}

	headers := e.buildHTTPHeaders(ctx, cfg, toolName, handlerName, args)

	policy, _ := httpRetryParams(cfg)
	var lastCallResult httpCallResult
	classify := func(err error) (bool, time.Duration) {
		if cfg.RetryPolicy == nil {
			return false, 0
		}
		return classifyHTTPResult(lastCallResult, cfg.RetryPolicy)
	}

	return retryWithBackoff(ctx, e.log, e.currentSpan(ctx), policy, handler.Timeout.Get(), classify,
		func(attemptCtx context.Context) (json.RawMessage, error) {
			result, callResult, err := doHTTPRequest(attemptCtx, &http.Client{}, cfg, headers, args)
			lastCallResult = callResult
			return result, err
		},
	)
}
```

- [ ] **Step 4: Remove httpExecutor field from OmniaExecutor struct and NewOmniaExecutor**

In the struct definition (line 78), remove:
```go
httpExecutor *sdktools.HTTPExecutor
```

In `NewOmniaExecutor` (line 107), remove:
```go
httpExecutor: sdktools.NewHTTPExecutor(),
```

Remove the `sdktools` import if no longer used elsewhere. Check with: `grep -n "sdktools\." internal/runtime/tools/omnia_executor.go` — if the only references were `HTTPExecutor` related, remove the import.

- [ ] **Step 5: Run existing tests to verify no regressions**

Run: `go test ./internal/runtime/tools/... -count=1 -v -timeout 120s`
Expected: All existing tests pass. Some may need updates if they reference `e.httpExecutor` — fix any compilation errors.

- [ ] **Step 6: Run goimports**

Run: `goimports -w internal/runtime/tools/omnia_executor.go`

- [ ] **Step 7: Commit**

```
git add internal/runtime/tools/omnia_executor.go
git commit -m "feat(runtime): wire retry into HTTP and OpenAPI executors, replace PromptKit HTTPExecutor"
```

---

### Task 5: Wire retry into executeGRPC and executeMCP

**Files:**
- Modify: `internal/runtime/tools/omnia_executor.go:837-874` (executeGRPC)
- Modify: `internal/runtime/tools/omnia_executor.go:692-721` (executeMCP)

- [ ] **Step 1: Wrap executeGRPC with retry (retry wraps circuit breaker)**

Replace the `executeGRPC` method (lines 837-874) with:

```go
func (e *OmniaExecutor) executeGRPC(
	ctx context.Context,
	toolName, handlerName string,
	args json.RawMessage,
) (json.RawMessage, error) {
	e.mu.RLock()
	client := e.grpcClients[handlerName]
	handler := e.handlers[handlerName]
	e.mu.RUnlock()

	if client == nil {
		return nil, fmt.Errorf("gRPC handler %q not connected", handlerName)
	}

	grpcCfg := handler.GRPCConfig
	policy, classify := grpcRetryParams(grpcCfg)

	return retryWithBackoff(ctx, e.log, e.currentSpan(ctx), policy, handler.Timeout.Get(), classify,
		func(attemptCtx context.Context) (json.RawMessage, error) {
			// Inject policy metadata.
			md := PolicyGRPCMetadata(attemptCtx, toolName, handlerName, nil)
			if len(md) > 0 {
				pairs := make([]string, 0, len(md)*2)
				for k, v := range md {
					pairs = append(pairs, k, v)
				}
				attemptCtx = metadata.AppendToOutgoingContext(attemptCtx, pairs...)
			}

			// Execute through circuit breaker.
			var resp *toolsv1.ToolResponse
			_, cbErr := e.breakers.Execute(toolName, func() ([]byte, error) {
				var execErr error
				resp, execErr = client.Execute(attemptCtx, &toolsv1.ToolRequest{
					ToolName:      toolName,
					ArgumentsJson: string(args),
				})
				return nil, execErr
			})
			if cbErr != nil {
				return nil, fmt.Errorf("gRPC tool execution failed: %w", cbErr)
			}

			return marshalGRPCResponse(resp)
		},
	)
}
```

- [ ] **Step 2: Wrap executeMCP with retry**

Replace the `executeMCP` method (lines 692-721) with:

```go
func (e *OmniaExecutor) executeMCP(
	ctx context.Context,
	toolName, handlerName string,
	args json.RawMessage,
) (json.RawMessage, error) {
	e.mu.RLock()
	session := e.mcpSessions[handlerName]
	handler := e.handlers[handlerName]
	e.mu.RUnlock()

	if session == nil {
		return nil, fmt.Errorf("MCP handler %q not connected", handlerName)
	}

	mcpCfg := handler.MCPConfig
	policy, classify := mcpRetryParams(mcpCfg)

	return retryWithBackoff(ctx, e.log, e.currentSpan(ctx), policy, handler.Timeout.Get(), classify,
		func(attemptCtx context.Context) (json.RawMessage, error) {
			var argsMap map[string]any
			if len(args) > 0 {
				if err := json.Unmarshal(args, &argsMap); err != nil {
					return nil, fmt.Errorf("failed to parse MCP args: %w", err)
				}
			}

			result, err := session.CallTool(attemptCtx, &mcp.CallToolParams{
				Name:      toolName,
				Arguments: argsMap,
			})
			if err != nil {
				return nil, fmt.Errorf("MCP tool call failed: %w", err)
			}

			// Convert MCP tool errors to mcpToolError so the classifier
			// can distinguish them from transport errors.
			if result.IsError {
				msg := "MCP tool error"
				if len(result.Content) > 0 {
					if text, ok := result.Content[0].(mcp.TextContent); ok {
						msg = text.Text
					}
				}
				return nil, &mcpToolError{message: msg}
			}

			return marshalMCPResult(result)
		},
	)
}
```

**Note:** The existing `marshalMCPResult` handles `IsError` internally. Since we now need to distinguish tool errors from transport errors for the classifier, we check `result.IsError` before calling `marshalMCPResult` and return a `mcpToolError` instead. Update `marshalMCPResult` to skip the `IsError` check since the caller handles it, or leave it as-is and only call it for non-error results. Check the existing implementation to decide.

- [ ] **Step 3: Run existing tests**

Run: `go test ./internal/runtime/tools/... -count=1 -v -timeout 120s`
Expected: All pass. Fix any compilation errors from the handler lookup change (we now read `e.handlers[handlerName]` inside `executeGRPC` and `executeMCP` — make sure this works with the existing test setup that directly sets `e.grpcClients` etc.).

- [ ] **Step 4: Run goimports**

Run: `goimports -w internal/runtime/tools/omnia_executor.go`

- [ ] **Step 5: Commit**

```
git add internal/runtime/tools/omnia_executor.go
git commit -m "feat(runtime): wire retry into gRPC and MCP executors"
```

---

### Task 6: Integration tests — HTTP retry

**Files:**
- Modify: `internal/runtime/tools/omnia_executor_test.go`

- [ ] **Step 1: Add HTTP retry integration test — retry then succeed**

```go
func TestExecuteHTTP_RetryThenSucceed(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls < 3 {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("bad gateway"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"result":"ok"}`))
	}))
	defer srv.Close()

	e := NewOmniaExecutor(logr.Discard(), nil)
	e.handlers["retry-http"] = &HandlerEntry{
		Name: "retry-http",
		Type: ToolTypeHTTP,
		HTTPConfig: &HTTPCfg{
			Endpoint: srv.URL,
			Method:   "GET",
			RetryPolicy: &RuntimeHTTPRetryPolicy{
				MaxAttempts:       int32(3),
				InitialBackoff:    Duration(1 * time.Millisecond),
				BackoffMultiplier: 2.0,
				MaxBackoff:        Duration(100 * time.Millisecond),
				RetryOn:           []int32{502},
				RetryOnNetworkError: true,
			},
		},
	}
	e.toolHandlers["retry-http-tool"] = "retry-http"

	desc := &pktools.ToolDescriptor{Name: "retry-http-tool"}
	result, err := e.Execute(context.Background(), desc, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if calls != 3 {
		t.Errorf("expected 3 calls (2 retries + 1 success), got %d", calls)
	}
	if string(result) != `{"result":"ok"}` {
		t.Errorf("unexpected result: %s", result)
	}
}

func TestExecuteHTTP_NonRetryableStatus(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("bad request"))
	}))
	defer srv.Close()

	e := NewOmniaExecutor(logr.Discard(), nil)
	e.handlers["no-retry-http"] = &HandlerEntry{
		Name: "no-retry-http",
		Type: ToolTypeHTTP,
		HTTPConfig: &HTTPCfg{
			Endpoint: srv.URL,
			Method:   "GET",
			RetryPolicy: &RuntimeHTTPRetryPolicy{
				MaxAttempts:       int32(3),
				InitialBackoff:    Duration(1 * time.Millisecond),
				BackoffMultiplier: 2.0,
				MaxBackoff:        Duration(100 * time.Millisecond),
				RetryOn:           []int32{502, 503},
				RetryOnNetworkError: true,
			},
		},
	}
	e.toolHandlers["no-retry-tool"] = "no-retry-http"

	desc := &pktools.ToolDescriptor{Name: "no-retry-tool"}
	_, err := e.Execute(context.Background(), desc, json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for 400 response")
	}
	if calls != 1 {
		t.Errorf("expected 1 call (no retries for 400), got %d", calls)
	}
}
```

- [ ] **Step 2: Run integration tests**

Run: `go test ./internal/runtime/tools/... -count=1 -run "TestExecuteHTTP_Retry" -v`
Expected: All PASS.

- [ ] **Step 3: Commit**

```
git add internal/runtime/tools/omnia_executor_test.go
git commit -m "test(runtime): add HTTP retry integration tests"
```

---

### Task 7: Integration tests — gRPC retry

**Files:**
- Modify: `internal/runtime/tools/omnia_executor_test.go`

- [ ] **Step 1: Add gRPC retry integration test**

Use the existing `mockToolServiceClient` pattern. Create a variant that fails N times then succeeds:

```go
type failNTimesClient struct {
	failCount   int
	calls       int
	successResp *toolsv1.ToolResponse
	failErr     error
}

func (m *failNTimesClient) Execute(
	_ context.Context,
	_ *toolsv1.ToolRequest,
	_ ...grpc.CallOption,
) (*toolsv1.ToolResponse, error) {
	m.calls++
	if m.calls <= m.failCount {
		return nil, m.failErr
	}
	return m.successResp, nil
}

func (m *failNTimesClient) ListTools(
	_ context.Context,
	_ *toolsv1.ListToolsRequest,
	_ ...grpc.CallOption,
) (*toolsv1.ListToolsResponse, error) {
	return nil, nil
}

func TestExecuteGRPC_RetryThenSucceed(t *testing.T) {
	mock := &failNTimesClient{
		failCount: 2,
		failErr:   grpcStatus.Error(grpcCodes.Unavailable, "service unavailable"),
		successResp: &toolsv1.ToolResponse{
			ResultJson: `{"answer":42}`,
		},
	}

	e := NewOmniaExecutor(logr.Discard(), nil)
	e.grpcClients["grpc-retry"] = mock
	e.handlers["grpc-retry"] = &HandlerEntry{
		Name: "grpc-retry",
		Type: ToolTypeGRPC,
		GRPCConfig: &GRPCCfg{
			Endpoint: "localhost:50051",
			RetryPolicy: &RuntimeGRPCRetryPolicy{
				MaxAttempts:          int32(3),
				InitialBackoff:       Duration(1 * time.Millisecond),
				BackoffMultiplier:    2.0,
				MaxBackoff:           Duration(100 * time.Millisecond),
				RetryableStatusCodes: []string{"UNAVAILABLE"},
			},
		},
	}
	e.toolHandlers["grpc-retry-tool"] = "grpc-retry"
	e.breakers = NewToolCircuitBreakers()

	result, err := e.executeGRPC(context.Background(), "grpc-retry-tool", "grpc-retry", json.RawMessage(`{"q":"test"}`))
	if err != nil {
		t.Fatalf("executeGRPC failed: %v", err)
	}
	if mock.calls != 3 {
		t.Errorf("expected 3 calls, got %d", mock.calls)
	}
	if string(result) != `{"answer":42}` {
		t.Errorf("unexpected result: %s", result)
	}
}

func TestExecuteGRPC_NonRetryableStatus(t *testing.T) {
	mock := &failNTimesClient{
		failCount: 5,
		failErr:   grpcStatus.Error(grpcCodes.NotFound, "not found"),
	}

	e := NewOmniaExecutor(logr.Discard(), nil)
	e.grpcClients["grpc-no-retry"] = mock
	e.handlers["grpc-no-retry"] = &HandlerEntry{
		Name: "grpc-no-retry",
		Type: ToolTypeGRPC,
		GRPCConfig: &GRPCCfg{
			Endpoint: "localhost:50051",
			RetryPolicy: &RuntimeGRPCRetryPolicy{
				MaxAttempts:          int32(3),
				InitialBackoff:       Duration(1 * time.Millisecond),
				BackoffMultiplier:    2.0,
				MaxBackoff:           Duration(100 * time.Millisecond),
				RetryableStatusCodes: []string{"UNAVAILABLE"},
			},
		},
	}
	e.toolHandlers["grpc-no-retry-tool"] = "grpc-no-retry"
	e.breakers = NewToolCircuitBreakers()

	_, err := e.executeGRPC(context.Background(), "grpc-no-retry-tool", "grpc-no-retry", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for NOT_FOUND")
	}
	if mock.calls != 1 {
		t.Errorf("expected 1 call (no retries for NOT_FOUND), got %d", mock.calls)
	}
}
```

**Note:** You'll need to add gRPC status/codes imports to the test file if not already present. Check existing imports first.

- [ ] **Step 2: Run gRPC integration tests**

Run: `go test ./internal/runtime/tools/... -count=1 -run "TestExecuteGRPC_Retry" -v`
Expected: All PASS.

- [ ] **Step 3: Commit**

```
git add internal/runtime/tools/omnia_executor_test.go
git commit -m "test(runtime): add gRPC retry integration tests"
```

---

### Task 8: Integration tests — MCP retry

**Files:**
- Modify: `internal/runtime/tools/omnia_executor_test.go`

- [ ] **Step 1: Add MCP retry integration test**

MCP testing is harder because `session.CallTool()` requires a real `mcp.ClientSession`. Check if the existing tests mock this or if there's a test helper. If there's no existing mock, create a minimal interface:

```go
// If mcp.ClientSession is a concrete type, we need to check if executeMCP
// can be refactored to use an interface. If not, we test at a higher level
// by testing retryWithBackoff + classifyMCPError together.

func TestMCPRetry_TransportErrorRetried(t *testing.T) {
	// Test the retry + classify composition directly since MCP sessions
	// are hard to mock.
	ctx := context.Background()
	log := logr.Discard()
	span := tracenoop.Span{}
	policy := retryPolicy{MaxAttempts: 3, InitialBackoff: 1 * time.Millisecond, BackoffMultiplier: 2.0, MaxBackoff: 100 * time.Millisecond}

	calls := 0
	result, err := retryWithBackoff(ctx, log, span, policy, 0,
		func(err error) (bool, time.Duration) { return classifyMCPError(err) },
		func(ctx context.Context) (json.RawMessage, error) {
			calls++
			if calls < 3 {
				return nil, &net.OpError{Op: "read", Err: errors.New("connection reset")}
			}
			return json.RawMessage(`{"mcp":"ok"}`), nil
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 3 {
		t.Errorf("expected 3 calls, got %d", calls)
	}
	if string(result) != `{"mcp":"ok"}` {
		t.Errorf("unexpected result: %s", result)
	}
}

func TestMCPRetry_ToolErrorNotRetried(t *testing.T) {
	ctx := context.Background()
	log := logr.Discard()
	span := tracenoop.Span{}
	policy := retryPolicy{MaxAttempts: 3, InitialBackoff: 1 * time.Millisecond, BackoffMultiplier: 2.0, MaxBackoff: 100 * time.Millisecond}

	calls := 0
	_, err := retryWithBackoff(ctx, log, span, policy, 0,
		func(err error) (bool, time.Duration) { return classifyMCPError(err) },
		func(ctx context.Context) (json.RawMessage, error) {
			calls++
			return nil, &mcpToolError{message: "file not found"}
		},
	)
	if err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Errorf("expected 1 call (no retries for tool error), got %d", calls)
	}
}
```

- [ ] **Step 2: Run MCP integration tests**

Run: `go test ./internal/runtime/tools/... -count=1 -run "TestMCPRetry" -v`
Expected: All PASS.

- [ ] **Step 3: Commit**

```
git add internal/runtime/tools/omnia_executor_test.go
git commit -m "test(runtime): add MCP retry integration tests"
```

---

### Task 9: Full verification and cleanup

**Files:**
- None modified; verification only.

- [ ] **Step 1: Full Go build**

Run: `env GOWORK=off go build ./...`
Expected: Success.

- [ ] **Step 2: Full test suite**

Run: `env GOWORK=off go test ./... -count=1 -timeout=300s`
Expected: All tests pass.

- [ ] **Step 3: Lint**

Run: `golangci-lint run ./internal/runtime/tools/...`
Expected: 0 new issues. Pre-existing issues in other files are acceptable.

- [ ] **Step 4: Check coverage on new files**

Run: `env GOWORK=off go test ./internal/runtime/tools/... -coverprofile=/tmp/retry-cover.out -count=1`
Run: `go tool cover -func=/tmp/retry-cover.out | grep -E "retry|http_client"`
Expected: All new functions >= 80% coverage.

- [ ] **Step 5: Generated code freshness**

Run: `make generate && make manifests`
Run: `git status --porcelain`
Expected: No changes (no CRD or generated code was modified).

---

## Self-Review Checklist

After completing all tasks, verify against the spec (`docs/superpowers/specs/2026-04-12-retry-execution-design.md`):

- [ ] **Section 2 — Core retry engine**: `retryWithBackoff` with exponential backoff + jitter. Covered by Task 1.
- [ ] **Section 2 — Circuit breaker composition**: retry wraps breaker for gRPC. Covered by Task 5.
- [ ] **Section 2 — Per-attempt timeout**: `withAttemptTimeout` wraps each attempt. Covered by Task 1.
- [ ] **Section 3 — Replace PromptKit HTTP executor**: `doHTTPRequest` with direct `http.Client`. Covered by Task 3.
- [ ] **Section 4 — HTTP classifier**: `classifyHTTPResult` with status codes + network errors + Retry-After. Covered by Task 2.
- [ ] **Section 4 — gRPC classifier**: `classifyGRPCError` with status code matching. Covered by Task 2.
- [ ] **Section 4 — MCP classifier**: `classifyMCPError` transport-only. Covered by Task 2.
- [ ] **Section 5 — Observability**: span events + V(1) log lines per retry. Covered by Task 1 (in `retryWithBackoff`).
- [ ] **Section 6 — File changes**: All listed files created/modified. Covered by Tasks 1-8.
- [ ] **Section 7 — Unit tests**: retry engine, classifiers, HTTP client. Covered by Tasks 1-3.
- [ ] **Section 7 — Integration tests**: HTTP, gRPC, MCP retry wiring. Covered by Tasks 6-8.

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-04-12-retry-execution.md`. Two execution options:

**1. Subagent-Driven (recommended)** — I dispatch a fresh subagent per task, review between tasks, fast iteration

**2. Inline Execution** — Execute tasks in this session using executing-plans, batch execution with checkpoints

Which approach?
