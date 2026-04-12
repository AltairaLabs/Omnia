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
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	grpcCodes "google.golang.org/grpc/codes"
	grpcStatus "google.golang.org/grpc/status"
)

// --- httpCallResult / classifyHTTPResult ---

func TestClassifyHTTPResult_NetworkError_RetryOnNetworkErrorTrue(t *testing.T) {
	netErr := &net.OpError{Op: "dial", Err: errors.New("connection refused")}
	result := httpCallResult{Err: netErr}
	policy := &RuntimeHTTPRetryPolicy{RetryOnNetworkError: true}

	retryable, retryAfter := classifyHTTPResult(result, policy)

	if !retryable {
		t.Error("expected retryable=true for network error with RetryOnNetworkError=true")
	}
	if retryAfter != 0 {
		t.Errorf("expected retryAfter=0, got %v", retryAfter)
	}
}

func TestClassifyHTTPResult_NetworkError_RetryOnNetworkErrorFalse(t *testing.T) {
	netErr := &net.OpError{Op: "dial", Err: errors.New("connection refused")}
	result := httpCallResult{Err: netErr}
	policy := &RuntimeHTTPRetryPolicy{RetryOnNetworkError: false}

	retryable, _ := classifyHTTPResult(result, policy)

	if retryable {
		t.Error("expected retryable=false for network error with RetryOnNetworkError=false")
	}
}

func TestClassifyHTTPResult_Status503InRetryOn(t *testing.T) {
	result := httpCallResult{StatusCode: 503, Headers: make(http.Header)}
	policy := &RuntimeHTTPRetryPolicy{RetryOn: []int32{502, 503, 504}}

	retryable, retryAfter := classifyHTTPResult(result, policy)

	if !retryable {
		t.Error("expected retryable=true for 503 in RetryOn list")
	}
	if retryAfter != 0 {
		t.Errorf("expected retryAfter=0, got %v", retryAfter)
	}
}

func TestClassifyHTTPResult_Status400NotInRetryOn(t *testing.T) {
	result := httpCallResult{StatusCode: 400, Headers: make(http.Header)}
	policy := &RuntimeHTTPRetryPolicy{RetryOn: []int32{502, 503, 504}}

	retryable, _ := classifyHTTPResult(result, policy)

	if retryable {
		t.Error("expected retryable=false for 400 not in RetryOn list")
	}
}

func TestClassifyHTTPResult_RetryAfterSeconds_RespectRetryAfterTrue(t *testing.T) {
	headers := make(http.Header)
	headers.Set("Retry-After", "5")
	result := httpCallResult{StatusCode: 503, Headers: headers}
	policy := &RuntimeHTTPRetryPolicy{
		RetryOn:           []int32{503},
		RespectRetryAfter: true,
	}

	retryable, retryAfter := classifyHTTPResult(result, policy)

	if !retryable {
		t.Error("expected retryable=true")
	}
	if retryAfter != 5*time.Second {
		t.Errorf("expected retryAfter=5s, got %v", retryAfter)
	}
}

func TestClassifyHTTPResult_RetryAfterHTTPDate(t *testing.T) {
	// Set a date ~10 seconds in the future.
	future := time.Now().Add(10 * time.Second)
	headers := make(http.Header)
	headers.Set("Retry-After", future.UTC().Format(http.TimeFormat))
	result := httpCallResult{StatusCode: 429, Headers: headers}
	policy := &RuntimeHTTPRetryPolicy{
		RetryOn:           []int32{429},
		RespectRetryAfter: true,
	}

	retryable, retryAfter := classifyHTTPResult(result, policy)

	if !retryable {
		t.Error("expected retryable=true")
	}
	// Allow generous window: 8s–12s to account for test execution time.
	if retryAfter < 8*time.Second || retryAfter > 12*time.Second {
		t.Errorf("expected retryAfter ~10s, got %v", retryAfter)
	}
}

func TestClassifyHTTPResult_RetryAfterIgnored_RespectRetryAfterFalse(t *testing.T) {
	headers := make(http.Header)
	headers.Set("Retry-After", "60")
	result := httpCallResult{StatusCode: 503, Headers: headers}
	policy := &RuntimeHTTPRetryPolicy{
		RetryOn:           []int32{503},
		RespectRetryAfter: false,
	}

	retryable, retryAfter := classifyHTTPResult(result, policy)

	if !retryable {
		t.Error("expected retryable=true")
	}
	if retryAfter != 0 {
		t.Errorf("expected retryAfter=0 when RespectRetryAfter=false, got %v", retryAfter)
	}
}

func TestClassifyHTTPResult_DeadlineExceeded_RetryOnNetworkErrorTrue(t *testing.T) {
	result := httpCallResult{Err: context.DeadlineExceeded}
	policy := &RuntimeHTTPRetryPolicy{RetryOnNetworkError: true}

	retryable, _ := classifyHTTPResult(result, policy)

	if !retryable {
		t.Error("expected retryable=true for DeadlineExceeded with RetryOnNetworkError=true")
	}
}

func TestClassifyHTTPResult_Status200NotRetryable(t *testing.T) {
	result := httpCallResult{StatusCode: 200, Headers: make(http.Header)}
	policy := &RuntimeHTTPRetryPolicy{RetryOn: []int32{503}}

	retryable, _ := classifyHTTPResult(result, policy)

	if retryable {
		t.Error("expected retryable=false for 200 success")
	}
}

// --- classifyGRPCError ---

func TestClassifyGRPCError_UnavailableInList(t *testing.T) {
	err := grpcStatus.Error(grpcCodes.Unavailable, "service unavailable")
	retryable, retryAfter := classifyGRPCError(err, []string{"UNAVAILABLE", "RESOURCE_EXHAUSTED"})

	if !retryable {
		t.Error("expected retryable=true for UNAVAILABLE in list")
	}
	if retryAfter != 0 {
		t.Errorf("expected retryAfter=0 for gRPC, got %v", retryAfter)
	}
}

func TestClassifyGRPCError_NotFoundNotInList(t *testing.T) {
	err := grpcStatus.Error(grpcCodes.NotFound, "not found")
	retryable, _ := classifyGRPCError(err, []string{"UNAVAILABLE"})

	if retryable {
		t.Error("expected retryable=false for NOT_FOUND not in list")
	}
}

func TestClassifyGRPCError_NonGRPCError(t *testing.T) {
	// Plain error (e.g. from circuit breaker) — not a gRPC status.
	err := errors.New("circuit breaker open")
	retryable, _ := classifyGRPCError(err, []string{"UNAVAILABLE"})

	if retryable {
		t.Error("expected retryable=false for non-gRPC error")
	}
}

func TestClassifyGRPCError_NilError(t *testing.T) {
	retryable, _ := classifyGRPCError(nil, []string{"UNAVAILABLE"})
	if retryable {
		t.Error("expected retryable=false for nil error")
	}
}

// --- classifyMCPError ---

func TestClassifyMCPError_TransportNetOpError(t *testing.T) {
	err := &net.OpError{Op: "read", Err: errors.New("connection reset")}
	retryable, retryAfter := classifyMCPError(err)

	if !retryable {
		t.Error("expected retryable=true for transport net.OpError")
	}
	if retryAfter != 0 {
		t.Errorf("expected retryAfter=0, got %v", retryAfter)
	}
}

func TestClassifyMCPError_DeadlineExceeded(t *testing.T) {
	retryable, _ := classifyMCPError(context.DeadlineExceeded)
	if !retryable {
		t.Error("expected retryable=true for DeadlineExceeded")
	}
}

func TestClassifyMCPError_NilError(t *testing.T) {
	retryable, _ := classifyMCPError(nil)
	if retryable {
		t.Error("expected retryable=false for nil error")
	}
}

func TestClassifyMCPError_MCPToolError(t *testing.T) {
	err := &mcpToolError{message: "tool returned error: invalid input"}
	retryable, _ := classifyMCPError(err)
	if retryable {
		t.Error("expected retryable=false for mcpToolError (application-level error)")
	}
}

func TestClassifyMCPError_WrappedMCPToolError(t *testing.T) {
	inner := &mcpToolError{message: "tool failed"}
	wrapped := fmt.Errorf("executor: %w", inner)
	retryable, _ := classifyMCPError(wrapped)
	if retryable {
		t.Error("expected retryable=false for wrapped mcpToolError")
	}
}

// --- parseRetryAfter ---

func TestParseRetryAfter_Empty(t *testing.T) {
	if d := parseRetryAfter(""); d != 0 {
		t.Errorf("expected 0 for empty string, got %v", d)
	}
}

func TestParseRetryAfter_Seconds(t *testing.T) {
	if d := parseRetryAfter("30"); d != 30*time.Second {
		t.Errorf("expected 30s, got %v", d)
	}
}

func TestParseRetryAfter_Unparseable(t *testing.T) {
	if d := parseRetryAfter("not-a-date-or-number"); d != 0 {
		t.Errorf("expected 0 for unparseable value, got %v", d)
	}
}

// --- policy extraction helpers ---

func TestHTTPRetryParams_NilCfg(t *testing.T) {
	p := httpRetryParams(nil)
	if p.MaxAttempts != 1 {
		t.Errorf("expected MaxAttempts=1 for nil cfg, got %d", p.MaxAttempts)
	}
}

func TestHTTPRetryParams_NilPolicy(t *testing.T) {
	p := httpRetryParams(&HTTPCfg{})
	if p.MaxAttempts != 1 {
		t.Errorf("expected MaxAttempts=1 for nil policy, got %d", p.MaxAttempts)
	}
}

func TestHTTPRetryParams_WithPolicy(t *testing.T) {
	cfg := &HTTPCfg{
		RetryPolicy: &RuntimeHTTPRetryPolicy{
			MaxAttempts:       3,
			InitialBackoff:    Duration(100 * time.Millisecond),
			BackoffMultiplier: 2.0,
			MaxBackoff:        Duration(5 * time.Second),
		},
	}
	p := httpRetryParams(cfg)
	if p.MaxAttempts != 3 {
		t.Errorf("expected MaxAttempts=3, got %d", p.MaxAttempts)
	}
	if p.InitialBackoff != 100*time.Millisecond {
		t.Errorf("expected InitialBackoff=100ms, got %v", p.InitialBackoff)
	}
	if p.BackoffMultiplier != 2.0 {
		t.Errorf("expected BackoffMultiplier=2.0, got %v", p.BackoffMultiplier)
	}
	if p.MaxBackoff != 5*time.Second {
		t.Errorf("expected MaxBackoff=5s, got %v", p.MaxBackoff)
	}
}

func TestGRPCRetryParams_NilCfg(t *testing.T) {
	p, classify := grpcRetryParams(nil)
	if p.MaxAttempts != 1 {
		t.Errorf("expected MaxAttempts=1 for nil cfg, got %d", p.MaxAttempts)
	}
	retryable, _ := classify(errors.New("some error"))
	if retryable {
		t.Error("expected classify to return false for nil policy")
	}
}

func TestGRPCRetryParams_WithPolicy(t *testing.T) {
	cfg := &GRPCCfg{
		RetryPolicy: &RuntimeGRPCRetryPolicy{
			MaxAttempts:          4,
			InitialBackoff:       Duration(50 * time.Millisecond),
			BackoffMultiplier:    1.5,
			MaxBackoff:           Duration(2 * time.Second),
			RetryableStatusCodes: []string{"UNAVAILABLE"},
		},
	}
	p, classify := grpcRetryParams(cfg)
	if p.MaxAttempts != 4 {
		t.Errorf("expected MaxAttempts=4, got %d", p.MaxAttempts)
	}
	// Verify classifier uses the policy's status codes.
	unavailableErr := grpcStatus.Error(grpcCodes.Unavailable, "down")
	retryable, _ := classify(unavailableErr)
	if !retryable {
		t.Error("expected UNAVAILABLE to be retryable")
	}
}

func TestMCPRetryParams_NilCfg(t *testing.T) {
	p, classify := mcpRetryParams(nil)
	if p.MaxAttempts != 1 {
		t.Errorf("expected MaxAttempts=1 for nil cfg, got %d", p.MaxAttempts)
	}
	retryable, _ := classify(errors.New("transport error"))
	if !retryable {
		t.Error("expected classify to return true for transport error with nil policy")
	}
}

func TestMCPRetryParams_WithPolicy(t *testing.T) {
	cfg := &MCPCfg{
		RetryPolicy: &RuntimeMCPRetryPolicy{
			MaxAttempts:       5,
			InitialBackoff:    Duration(200 * time.Millisecond),
			BackoffMultiplier: 2.0,
			MaxBackoff:        Duration(10 * time.Second),
		},
	}
	p, classify := mcpRetryParams(cfg)
	if p.MaxAttempts != 5 {
		t.Errorf("expected MaxAttempts=5, got %d", p.MaxAttempts)
	}
	toolErr := &mcpToolError{message: "tool error"}
	retryable, _ := classify(toolErr)
	if retryable {
		t.Error("expected mcpToolError to be non-retryable")
	}
}
