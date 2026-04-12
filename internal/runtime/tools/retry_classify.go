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
	"errors"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	grpcStatus "google.golang.org/grpc/status"
)

// httpCallResult captures HTTP response metadata for retry classification.
type httpCallResult struct {
	StatusCode int
	Headers    http.Header
	Err        error // transport error; nil if HTTP request completed
}

// mcpToolError marks MCP application-level errors as non-retryable.
// Transport errors are represented by other error types and are retryable.
type mcpToolError struct {
	message string
}

func (e *mcpToolError) Error() string { return e.message }

// classifyHTTPResult determines whether an HTTP call result is retryable.
// Returns (retryable, retryAfter) where retryAfter is parsed from the
// Retry-After header when policy.RespectRetryAfter is true.
func classifyHTTPResult(result httpCallResult, policy *RuntimeHTTPRetryPolicy) (bool, time.Duration) {
	if result.Err != nil {
		return isNetworkError(result.Err) && policy.RetryOnNetworkError, 0
	}

	if !slices.Contains(policy.RetryOn, int32(result.StatusCode)) {
		return false, 0
	}

	var retryAfter time.Duration
	if policy.RespectRetryAfter {
		retryAfter = parseRetryAfter(result.Headers.Get("Retry-After"))
	}
	return true, retryAfter
}

// classifyGRPCError determines whether a gRPC error is retryable based on
// the configured retryable status code names (e.g. "UNAVAILABLE").
// Non-gRPC errors (e.g. from a circuit breaker) are never retried.
func classifyGRPCError(err error, retryableStatusCodes []string) (bool, time.Duration) {
	if err == nil {
		return false, 0
	}
	st, ok := grpcStatus.FromError(err)
	if !ok {
		return false, 0
	}
	codeName := strings.ToUpper(st.Code().String())
	return slices.Contains(retryableStatusCodes, codeName), 0
}

// classifyMCPError determines whether an MCP error is retryable.
// Application-level tool errors (mcpToolError) are not retried; transport
// errors are.
func classifyMCPError(err error) (bool, time.Duration) {
	if err == nil {
		return false, 0
	}
	var toolErr *mcpToolError
	if errors.As(err, &toolErr) {
		return false, 0
	}
	return true, 0
}

// isNetworkError returns true for errors that represent transient network
// conditions: context timeouts/cancellations, dial/read/write failures, and
// DNS errors.
func isNetworkError(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	if errors.Is(err, context.Canceled) {
		return true
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}
	var dnsErr *net.DNSError
	return errors.As(err, &dnsErr)
}

// parseRetryAfter parses a Retry-After header value.
// Supports both integer-seconds and HTTP-date formats.
// Returns 0 for empty or unparseable values.
func parseRetryAfter(value string) time.Duration {
	if value == "" {
		return 0
	}
	// Integer seconds form: "120"
	if secs, err := strconv.Atoi(value); err == nil {
		return time.Duration(secs) * time.Second
	}
	// HTTP-date form: "Mon, 02 Jan 2006 15:04:05 GMT"
	if t, err := http.ParseTime(value); err == nil {
		d := time.Until(t)
		if d < 0 {
			return 0
		}
		return d
	}
	return 0
}

// httpRetryParams extracts a retryPolicy from an HTTPCfg.
// Returns a single-attempt (no-retry) policy when cfg or its RetryPolicy is nil.
// Note: the classify closure for HTTP is built in the executor because it needs
// to capture the httpCallResult from the actual response.
func httpRetryParams(cfg *HTTPCfg) retryPolicy {
	if cfg == nil || cfg.RetryPolicy == nil {
		return retryPolicy{MaxAttempts: 1}
	}
	p := cfg.RetryPolicy
	return retryPolicy{
		MaxAttempts:       p.MaxAttempts,
		InitialBackoff:    p.InitialBackoff.Get(),
		BackoffMultiplier: p.BackoffMultiplier,
		MaxBackoff:        p.MaxBackoff.Get(),
	}
}

// grpcRetryParams extracts a retryPolicy and classify function from a GRPCCfg.
// Returns a single-attempt policy and a no-op classifier when cfg or its
// RetryPolicy is nil.
func grpcRetryParams(cfg *GRPCCfg) (retryPolicy, func(error) (bool, time.Duration)) {
	if cfg == nil || cfg.RetryPolicy == nil {
		return retryPolicy{MaxAttempts: 1}, func(_ error) (bool, time.Duration) { return false, 0 }
	}
	p := cfg.RetryPolicy
	codes := p.RetryableStatusCodes
	classify := func(err error) (bool, time.Duration) {
		return classifyGRPCError(err, codes)
	}
	return retryPolicy{
		MaxAttempts:       p.MaxAttempts,
		InitialBackoff:    p.InitialBackoff.Get(),
		BackoffMultiplier: p.BackoffMultiplier,
		MaxBackoff:        p.MaxBackoff.Get(),
	}, classify
}

// mcpRetryParams extracts a retryPolicy and classify function from an MCPCfg.
// Returns a single-attempt policy and the standard MCP classifier when cfg or
// its RetryPolicy is nil.
func mcpRetryParams(cfg *MCPCfg) (retryPolicy, func(error) (bool, time.Duration)) {
	classify := func(err error) (bool, time.Duration) {
		return classifyMCPError(err)
	}
	if cfg == nil || cfg.RetryPolicy == nil {
		return retryPolicy{MaxAttempts: 1}, classify
	}
	p := cfg.RetryPolicy
	return retryPolicy{
		MaxAttempts:       p.MaxAttempts,
		InitialBackoff:    p.InitialBackoff.Get(),
		BackoffMultiplier: p.BackoffMultiplier,
		MaxBackoff:        p.MaxBackoff.Get(),
	}, classify
}
