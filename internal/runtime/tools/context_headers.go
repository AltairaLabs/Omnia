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
	"net/http"

	"github.com/altairalabs/omnia/pkg/policy"
)

// SetPolicyHeaders sets all policy propagation headers on an outbound HTTP request.
// This includes context headers (agent, namespace, session, request), user headers
// (user-id, roles, authorization), provider headers, and claim headers.
func SetPolicyHeaders(ctx context.Context, req *http.Request) {
	for k, v := range policy.ToOutboundHeaders(ctx) {
		req.Header.Set(k, v)
	}
}

// SetToolHeaders sets tool-specific headers on an outbound HTTP request.
func SetToolHeaders(req *http.Request, toolName, registryName string) {
	if toolName != "" {
		req.Header.Set(policy.HeaderToolName, toolName)
	}
	if registryName != "" {
		req.Header.Set(policy.HeaderToolRegistry, registryName)
	}
}

// SetParamHeaders promotes scalar tool arguments to HTTP headers.
func SetParamHeaders(req *http.Request, args map[string]any) {
	for k, v := range policy.PromoteScalarParams(args) {
		req.Header.Set(k, v)
	}
}

// SetAllOutboundHeaders sets policy, tool, and parameter headers on an HTTP request.
// This is the primary function tool adapters should call.
func SetAllOutboundHeaders(ctx context.Context, req *http.Request, toolName, registryName string, args map[string]any) {
	SetPolicyHeaders(ctx, req)
	SetToolHeaders(req, toolName, registryName)
	SetParamHeaders(req, args)
}

// PolicyGRPCMetadata returns all policy propagation fields as a map suitable for
// gRPC outgoing metadata, including tool-specific and parameter headers.
func PolicyGRPCMetadata(ctx context.Context, toolName, registryName string, args map[string]any) map[string]string {
	md := policy.ToGRPCMetadata(ctx)
	if toolName != "" {
		md[policy.HeaderToolName] = toolName
	}
	if registryName != "" {
		md[policy.HeaderToolRegistry] = registryName
	}
	for k, v := range policy.PromoteScalarParams(args) {
		md[k] = v
	}
	return md
}
