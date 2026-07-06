/*
Copyright 2026.

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

import "context"

// injectedHeadersKey is a private type for the injected-headers context key,
// avoiding collisions with other packages' context keys.
type injectedHeadersKey struct{}

// WithInjectedHeaders returns a context carrying headers the ToolPolicy
// broker decided to inject into the outbound tool call
// (DecisionResponse.InjectedHeaders). dispatch stashes these on ctx before
// routing to the type-specific executor, since header/metadata assembly
// (buildHTTPHeaders, gRPC PolicyGRPCMetadata) is per-executor. Does nothing
// when headers is empty, so callers can pass a possibly-empty map through
// unconditionally.
func WithInjectedHeaders(ctx context.Context, headers map[string]string) context.Context {
	if len(headers) == 0 {
		return ctx
	}
	return context.WithValue(ctx, injectedHeadersKey{}, headers)
}

// InjectedHeadersFromContext extracts broker-injected headers from ctx, or
// nil when none were stashed. Executor header/metadata builders merge these
// in last, so a broker-injected value wins over static/auth/policy headers
// on key collision.
func InjectedHeadersFromContext(ctx context.Context) map[string]string {
	if v := ctx.Value(injectedHeadersKey{}); v != nil {
		if m, ok := v.(map[string]string); ok {
			return m
		}
	}
	return nil
}
