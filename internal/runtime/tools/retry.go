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
	"fmt"
	"math"
	randv2 "math/rand/v2"
	"time"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// retryPolicy describes exponential-backoff retry behaviour for a tool call.
type retryPolicy struct {
	MaxAttempts       int32
	InitialBackoff    time.Duration
	BackoffMultiplier float64
	MaxBackoff        time.Duration
}

// retryWithBackoff executes fn, retrying on retryable errors according to policy.
//
// If MaxAttempts <= 1 the function is called once and returns immediately (fast
// path; classify is never invoked).
//
// For each attempt the function is called with a child context that has
// attemptTimeout applied when > 0.  On failure the classifier decides whether
// to retry.  If the classifier returns a positive retryAfter hint, the sleep
// delay is at least that value (still capped at MaxBackoff).  Context
// cancellation during the sleep is honoured immediately.
func retryWithBackoff(
	ctx context.Context,
	log logr.Logger,
	span trace.Span,
	policy retryPolicy,
	attemptTimeout time.Duration,
	classify func(error) (retryable bool, retryAfter time.Duration),
	fn func(ctx context.Context) (json.RawMessage, error),
) (json.RawMessage, error) {
	// Fast path: single attempt, no retry machinery.
	if policy.MaxAttempts <= 1 {
		attemptCtx, cancel := withAttemptTimeout(ctx, attemptTimeout)
		defer cancel()
		return fn(attemptCtx)
	}

	var lastErr error
	for attempt := int32(0); attempt < policy.MaxAttempts; attempt++ {
		attemptCtx, cancel := withAttemptTimeout(ctx, attemptTimeout)
		result, err := fn(attemptCtx)
		cancel()

		if err == nil {
			return result, nil
		}
		lastErr = err

		// Do not classify or sleep after the last attempt.
		if attempt == policy.MaxAttempts-1 {
			break
		}

		retryable, retryAfter := classify(err)
		if !retryable {
			return nil, err
		}

		delay := backoffDelay(policy, int(attempt), retryAfter)

		log.V(1).Info("retry attempt",
			"attempt", attempt+1,
			"maxAttempts", policy.MaxAttempts,
			"delay", delay,
			"error", err,
		)
		span.AddEvent("retry.attempt",
			trace.WithAttributes(
				attribute.Int("attempt", int(attempt+1)),
				attribute.String("delay", delay.String()),
				attribute.String("error", err.Error()),
			),
		)

		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	log.Info("retries exhausted",
		"attempts", policy.MaxAttempts,
		"error", lastErr,
	)
	span.AddEvent("retry.exhausted",
		trace.WithAttributes(
			attribute.Int("attempts", int(policy.MaxAttempts)),
			attribute.String("error", lastErr.Error()),
		),
	)
	return nil, fmt.Errorf("%d attempts exhausted: %w", policy.MaxAttempts, lastErr)
}

// withAttemptTimeout returns a derived context with timeout applied when
// timeout > 0.  The returned cancel function must always be called by the
// caller to avoid context leaks.
func withAttemptTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		// No timeout requested; return a no-op cancel so callers can always
		// `defer cancel()` without a nil check.
		return ctx, func() { /* no-op cancel — no derived context to cancel */ }
	}
	return context.WithTimeout(ctx, timeout)
}

// backoffDelay computes the sleep duration for a given attempt using
// exponential back-off with ±10% jitter.  If retryAfter > 0 the result is
// at least retryAfter (still capped at policy.MaxBackoff).
func backoffDelay(policy retryPolicy, attempt int, retryAfter time.Duration) time.Duration {
	base := float64(policy.InitialBackoff) * math.Pow(policy.BackoffMultiplier, float64(attempt))
	if max := float64(policy.MaxBackoff); base > max {
		base = max
	}

	// ±10% jitter: multiply by a random factor in [0.9, 1.1].
	jitter := 0.9 + randv2.Float64()*0.2
	delay := time.Duration(base * jitter)

	delay = max(delay, retryAfter)
	if delay > policy.MaxBackoff {
		delay = policy.MaxBackoff
	}
	return delay
}
