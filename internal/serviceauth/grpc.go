/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package serviceauth

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

// metadataAuthKey is the lowercased gRPC metadata key carrying the bearer token.
const metadataAuthKey = "authorization"

// bearerFromMetadata extracts the bearer token from the incoming gRPC
// "authorization" metadata of ctx, stripping the "Bearer " scheme prefix.
func bearerFromMetadata(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	vals := md.Get(metadataAuthKey)
	if len(vals) == 0 {
		return ""
	}
	if after, ok := strings.CutPrefix(vals[0], bearerPrefix); ok {
		return strings.TrimSpace(after)
	}
	return ""
}

// authenticateGRPC validates the bearer token in ctx's incoming metadata
// against reviewer and authz. On success it returns a context carrying the
// verified subject. Failures map to gRPC status errors: Unauthenticated for a
// missing/invalid token, PermissionDenied for an unauthorized subject.
func authenticateGRPC(ctx context.Context, reviewer TokenReviewer, authz authorizer) (context.Context, error) {
	token := bearerFromMetadata(ctx)
	if token == "" {
		return nil, status.Error(codes.Unauthenticated, "unauthenticated")
	}
	authenticated, subject, err := reviewer.ReviewToken(ctx, token)
	if err != nil {
		ctrllog.FromContext(ctx).Error(err, "serviceauth: token review failed")
		return nil, status.Error(codes.Unauthenticated, "unauthenticated")
	}
	if !authenticated {
		return nil, status.Error(codes.Unauthenticated, "unauthenticated")
	}
	if !authz.allowed(subject) {
		ctrllog.FromContext(ctx).Info("serviceauth: subject not allowed", "subject", subject)
		return nil, status.Error(codes.PermissionDenied, "permission denied")
	}
	return WithSubject(ctx, subject), nil
}

// UnaryServerInterceptor returns a gRPC unary interceptor that requires a Bearer
// token whose TokenReview subject is authorized: an exact match in
// allowedSubjects OR a ServiceAccount whose namespace is in allowedNamespaces. A
// nil reviewer disables auth (pass-through). On success the verified subject is
// injected into the handler context via WithSubject.
func UnaryServerInterceptor(reviewer TokenReviewer, allowedSubjects, allowedNamespaces []string) grpc.UnaryServerInterceptor {
	authz := newAuthorizer(allowedSubjects, allowedNamespaces)
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if reviewer == nil {
			return handler(ctx, req)
		}
		authCtx, err := authenticateGRPC(ctx, reviewer, authz)
		if err != nil {
			return nil, err
		}
		return handler(authCtx, req)
	}
}

// StreamServerInterceptor returns a gRPC stream interceptor that requires a
// Bearer token whose TokenReview subject is authorized: an exact match in
// allowedSubjects OR a ServiceAccount whose namespace is in allowedNamespaces. A
// nil reviewer disables auth (pass-through). On success the verified subject is
// injected into the stream context via WithSubject.
func StreamServerInterceptor(reviewer TokenReviewer, allowedSubjects, allowedNamespaces []string) grpc.StreamServerInterceptor {
	authz := newAuthorizer(allowedSubjects, allowedNamespaces)
	return func(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if reviewer == nil {
			return handler(srv, ss)
		}
		authCtx, err := authenticateGRPC(ss.Context(), reviewer, authz)
		if err != nil {
			return err
		}
		return handler(srv, &subjectServerStream{ServerStream: ss, ctx: authCtx})
	}
}

// subjectServerStream wraps a grpc.ServerStream to override its context with one
// carrying the verified subject.
type subjectServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *subjectServerStream) Context() context.Context { return s.ctx }
