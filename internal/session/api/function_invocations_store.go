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

package api

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// ErrFunctionInvocationNotFound is returned by GetFunctionInvocation
// when the (namespace, id) tuple does not resolve to a row. Cross-tenant
// reads (a namespace that doesn't match the row's actual namespace)
// also surface as this error to avoid leaking existence across tenants.
var ErrFunctionInvocationNotFound = errors.New("function invocation not found")

// ErrMissingFunctionInvocationsStore is returned by the service when no
// store has been wired into the Handler — typically because session-api
// is running without database access.
var ErrMissingFunctionInvocationsStore = errors.New("function invocations store is not configured")

// FunctionInvocationStatus values. Mirrors the CHECK constraint on the
// function_invocations table.
const (
	FunctionInvocationStatusSuccess       = "success"
	FunctionInvocationStatusInputInvalid  = "input_invalid"
	FunctionInvocationStatusOutputInvalid = "output_invalid"
	FunctionInvocationStatusRuntimeError  = "runtime_error"
)

// FunctionInvocation is the persistence shape for one Function call.
// Mirrors the function_invocations table 1:1; downstream consumers
// (dashboard, audit exports) should read via this struct.
type FunctionInvocation struct {
	ID           string          `json:"id"`
	Namespace    string          `json:"namespace"`
	FunctionName string          `json:"functionName"`
	InputHash    string          `json:"inputHash"`
	OutputJSON   json.RawMessage `json:"outputJson,omitempty"`
	Status       string          `json:"status"`
	DurationMs   int32           `json:"durationMs"`
	CostUSD      float64         `json:"costUsd"`
	TraceID      string          `json:"traceId,omitempty"`
	CreatedAt    time.Time       `json:"createdAt"`
}

// Pagination defaults / ceilings for the list endpoint. Match
// provider_calls limits so dashboards have consistent paging behaviour.
const (
	DefaultFunctionInvocationListLimit = 100
	MaxFunctionInvocationListLimit     = 1000
)

// FunctionInvocationListOpts configures GetFunctionInvocations. Namespace
// is required to keep reads scoped to the caller's tenant. FunctionName,
// From, To are optional filters.
type FunctionInvocationListOpts struct {
	Namespace    string
	FunctionName string
	From         time.Time
	To           time.Time
	Limit        int
}

// FunctionInvocationsStore is the persistence interface for the
// function_invocations table. Write path is the facade (when
// spec.invocationRecording.state == "enabled"); read path is the
// dashboard / audit-export tooling.
type FunctionInvocationsStore interface {
	// CreateFunctionInvocation persists a single invocation record.
	// Called by the facade after a successful or failed Function call
	// when recording is enabled.
	CreateFunctionInvocation(ctx context.Context, inv *FunctionInvocation) error

	// GetFunctionInvocation returns a single invocation by id. The
	// namespace must match (cross-tenant reads return ErrNotFound).
	GetFunctionInvocation(ctx context.Context, namespace, id string) (*FunctionInvocation, error)

	// ListFunctionInvocations returns recent invocations for a
	// namespace+function pair, ordered by created_at DESC.
	ListFunctionInvocations(ctx context.Context, opts FunctionInvocationListOpts) ([]*FunctionInvocation, error)
}
