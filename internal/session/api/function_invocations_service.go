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

	"github.com/go-logr/logr"
)

// FunctionInvocationsService is the business-logic wrapper around
// FunctionInvocationsStore. Mirrors ProviderCallsService for consistency.
type FunctionInvocationsService struct {
	store FunctionInvocationsStore
	log   logr.Logger
}

// NewFunctionInvocationsService wires a service onto a store. log may
// be a zero-value logr.Logger.
func NewFunctionInvocationsService(store FunctionInvocationsStore, log logr.Logger) *FunctionInvocationsService {
	return &FunctionInvocationsService{
		store: store,
		log:   log.WithName("function-invocations-service"),
	}
}

// CreateFunctionInvocation persists a single audit row.
func (s *FunctionInvocationsService) CreateFunctionInvocation(ctx context.Context, inv *FunctionInvocation) error {
	if s.store == nil {
		return ErrMissingFunctionInvocationsStore
	}
	return s.store.CreateFunctionInvocation(ctx, inv)
}

// GetFunctionInvocation looks up a single row by (namespace, id).
func (s *FunctionInvocationsService) GetFunctionInvocation(ctx context.Context, namespace, id string) (*FunctionInvocation, error) {
	if s.store == nil {
		return nil, ErrMissingFunctionInvocationsStore
	}
	return s.store.GetFunctionInvocation(ctx, namespace, id)
}

// ListFunctionInvocations returns recent rows for a (namespace,
// function_name) pair.
func (s *FunctionInvocationsService) ListFunctionInvocations(ctx context.Context, opts FunctionInvocationListOpts) ([]*FunctionInvocation, error) {
	if s.store == nil {
		return nil, ErrMissingFunctionInvocationsStore
	}
	return s.store.ListFunctionInvocations(ctx, opts)
}
