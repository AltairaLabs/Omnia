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

package runtime

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/altairalabs/omnia/pkg/policy"
)

// PolicyUnaryServerInterceptor returns a gRPC unary server interceptor that
// extracts policy propagation fields from incoming gRPC metadata and populates
// the Go context for downstream use by tool adapters.
func PolicyUnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		_ *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		ctx = extractPolicyFromMetadata(ctx)
		return handler(ctx, req)
	}
}

// PolicyStreamServerInterceptor returns a gRPC stream server interceptor that
// extracts policy propagation fields from incoming gRPC metadata and populates
// the Go context for downstream use by tool adapters.
func PolicyStreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(
		srv any,
		ss grpc.ServerStream,
		_ *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		ctx := extractPolicyFromMetadata(ss.Context())
		return handler(srv, &policyServerStream{ServerStream: ss, ctx: ctx})
	}
}

// policyServerStream wraps grpc.ServerStream to override the context.
type policyServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

// Context returns the wrapped context with policy fields.
func (s *policyServerStream) Context() context.Context {
	return s.ctx
}

// extractPolicyFromMetadata reads incoming gRPC metadata and populates the
// context with policy propagation fields.
func extractPolicyFromMetadata(ctx context.Context) context.Context {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ctx
	}
	fields := &policy.PropagationFields{
		AgentName:     firstValue(md, policy.HeaderAgentName),
		Namespace:     firstValue(md, policy.HeaderNamespace),
		SessionID:     firstValue(md, policy.HeaderSessionID),
		RequestID:     firstValue(md, policy.HeaderRequestID),
		UserID:        firstValue(md, policy.HeaderUserID),
		UserRoles:     firstValue(md, policy.HeaderUserRoles),
		UserEmail:     firstValue(md, policy.HeaderUserEmail),
		Authorization: firstValue(md, policy.HeaderAuthorization),
		Provider:      firstValue(md, policy.HeaderProvider),
		Model:         firstValue(md, policy.HeaderModel),
		Claims:        extractClaims(md),
	}
	return policy.WithPropagationFields(ctx, fields)
}

// firstValue returns the first value for the given metadata key, or empty string.
func firstValue(md metadata.MD, key string) string {
	vals := md.Get(key)
	if len(vals) > 0 {
		return vals[0]
	}
	return ""
}

// extractClaims extracts all claim headers from metadata.
func extractClaims(md metadata.MD) map[string]string {
	claims := make(map[string]string)
	for key, vals := range md {
		if len(key) > len(policy.HeaderClaimPrefix) &&
			key[:len(policy.HeaderClaimPrefix)] == policy.HeaderClaimPrefix &&
			len(vals) > 0 {
			claimName := key[len(policy.HeaderClaimPrefix):]
			claims[claimName] = vals[0]
		}
	}
	if len(claims) == 0 {
		return nil
	}
	return claims
}
