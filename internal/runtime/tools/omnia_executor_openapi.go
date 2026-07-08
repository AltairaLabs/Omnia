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

	pktools "github.com/AltairaLabs/PromptKit/runtime/tools"
)

// buildOpenAPIDescriptor populates the descriptor from parsed OpenAPI operations.
func (e *OmniaExecutor) buildOpenAPIDescriptor(desc *pktools.ToolDescriptor, toolName, handlerName string) {
	ops, ok := e.openAPIOps[handlerName]
	if !ok {
		return
	}
	op, ok := ops[toolName]
	if !ok {
		return
	}
	adapter := &OpenAPIAdapter{log: e.log}
	desc.Description = adapter.buildDescription(op)
	desc.InputSchema = marshalSchema(adapter.buildInputSchema(op))
}

func (e *OmniaExecutor) initOpenAPIHandler(ctx context.Context, name string, h *HandlerEntry) error {
	if h.OpenAPIConfig == nil {
		e.log.Info("skipping OpenAPI handler without config", "handler", name)
		return nil
	}

	adapter := NewOpenAPIAdapter(OpenAPIAdapterConfig{
		Name:            name,
		SpecURL:         h.OpenAPIConfig.SpecURL,
		BaseURL:         h.OpenAPIConfig.BaseURL,
		OperationFilter: h.OpenAPIConfig.OperationFilter,
		Headers:         h.OpenAPIConfig.Headers,
		AuthType:        h.OpenAPIConfig.AuthType,
		AuthToken:       h.OpenAPIConfig.AuthToken,
		AuthCloud:       h.OpenAPIConfig.AuthCloud,
		AuthAudience:    h.OpenAPIConfig.AuthAudience,
		AuthHeader:      h.OpenAPIConfig.AuthHeader,
		TokenAcquirer:   e.tokenAcquirer,
	}, e.log)

	if err := adapter.Connect(ctx); err != nil {
		return err
	}

	e.openAPIBaseURLs[name] = adapter.baseURL
	e.openAPIOps[name] = adapter.operations
	e.openAPIHeaders[name] = adapter.config.Headers

	for opID := range adapter.operations {
		e.toolHandlers[opID] = name
		e.log.V(1).Info("registered OpenAPI tool", "tool", opID, "handler", name)
	}

	return nil
}

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
		cfg.AuthCloud = handler.OpenAPIConfig.AuthCloud
		cfg.AuthAudience = handler.OpenAPIConfig.AuthAudience
		cfg.AuthHeader = handler.OpenAPIConfig.AuthHeader
		cfg.RetryPolicy = handler.OpenAPIConfig.RetryPolicy
	}

	return e.executeHTTPCall(ctx, toolName, handlerName, handler.Timeout.Get(), cfg, args)
}
