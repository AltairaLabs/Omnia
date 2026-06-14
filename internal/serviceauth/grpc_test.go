/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package serviceauth

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const grpcSubject = "system:serviceaccount:omnia-system:omnia-session-api"

func ctxWithAuth(header string) context.Context {
	if header == "" {
		return context.Background()
	}
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", header))
}

func wantCode(t *testing.T, err error, code codes.Code) {
	t.Helper()
	if status.Code(err) != code {
		t.Fatalf("got code %v (err=%v), want %v", status.Code(err), err, code)
	}
}

func TestUnaryServerInterceptor(t *testing.T) {
	var gotSubject string
	var handler grpc.UnaryHandler = func(ctx context.Context, _ any) (any, error) {
		gotSubject = SubjectFromContext(ctx)
		return "ok", nil
	}
	info := &grpc.UnaryServerInfo{}

	t.Run("allowlisted -> handler, subject in ctx", func(t *testing.T) {
		gotSubject = ""
		ic := UnaryServerInterceptor(fakeReviewer{authenticated: true, username: grpcSubject}, []string{grpcSubject}, nil)
		resp, err := ic(ctxWithAuth("Bearer good"), nil, info, handler)
		if err != nil || resp != "ok" {
			t.Fatalf("got resp=%v err=%v", resp, err)
		}
		if gotSubject != grpcSubject {
			t.Fatalf("subject = %q, want %q", gotSubject, grpcSubject)
		}
	})

	t.Run("missing token -> Unauthenticated", func(t *testing.T) {
		ic := UnaryServerInterceptor(fakeReviewer{}, []string{grpcSubject}, nil)
		_, err := ic(ctxWithAuth(""), nil, info, handler)
		wantCode(t, err, codes.Unauthenticated)
	})

	t.Run("reviewer error -> Unauthenticated", func(t *testing.T) {
		ic := UnaryServerInterceptor(fakeReviewer{err: errors.New("boom")}, []string{grpcSubject}, nil)
		_, err := ic(ctxWithAuth(testBearerX), nil, info, handler)
		wantCode(t, err, codes.Unauthenticated)
	})

	t.Run("unauthenticated -> Unauthenticated", func(t *testing.T) {
		ic := UnaryServerInterceptor(fakeReviewer{authenticated: false}, []string{grpcSubject}, nil)
		_, err := ic(ctxWithAuth(testBearerX), nil, info, handler)
		wantCode(t, err, codes.Unauthenticated)
	})

	t.Run("wrong subject -> PermissionDenied", func(t *testing.T) {
		ic := UnaryServerInterceptor(fakeReviewer{authenticated: true, username: "system:serviceaccount:other:thing"}, []string{grpcSubject}, nil)
		_, err := ic(ctxWithAuth(testBearerX), nil, info, handler)
		wantCode(t, err, codes.PermissionDenied)
	})

	t.Run("namespace-allowed subject not in subjects -> handler", func(t *testing.T) {
		gotSubject = ""
		facade := "system:serviceaccount:ws-ns:foo-facade"
		ic := UnaryServerInterceptor(fakeReviewer{authenticated: true, username: facade}, nil, []string{testNS})
		resp, err := ic(ctxWithAuth(testBearerX), nil, info, handler)
		if err != nil || resp != "ok" {
			t.Fatalf("got resp=%v err=%v", resp, err)
		}
		if gotSubject != facade {
			t.Fatalf("subject = %q, want %q", gotSubject, facade)
		}
	})

	t.Run("subject in non-allowed namespace -> PermissionDenied", func(t *testing.T) {
		ic := UnaryServerInterceptor(
			fakeReviewer{authenticated: true, username: "system:serviceaccount:other-ns:foo-facade"},
			nil, []string{testNS})
		_, err := ic(ctxWithAuth(testBearerX), nil, info, handler)
		wantCode(t, err, codes.PermissionDenied)
	})

	t.Run("nil reviewer -> pass-through", func(t *testing.T) {
		ic := UnaryServerInterceptor(nil, nil, nil)
		resp, err := ic(ctxWithAuth(""), nil, info, handler)
		if err != nil || resp != "ok" {
			t.Fatalf("got resp=%v err=%v", resp, err)
		}
	})
}

// fakeServerStream is a minimal grpc.ServerStream for stream interceptor tests.
type fakeServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *fakeServerStream) Context() context.Context { return s.ctx }

func TestStreamServerInterceptor(t *testing.T) {
	var gotSubject string
	var handler grpc.StreamHandler = func(_ any, ss grpc.ServerStream) error {
		gotSubject = SubjectFromContext(ss.Context())
		return nil
	}
	info := &grpc.StreamServerInfo{}

	t.Run("allowlisted -> handler, subject in ctx", func(t *testing.T) {
		gotSubject = ""
		ic := StreamServerInterceptor(fakeReviewer{authenticated: true, username: grpcSubject}, []string{grpcSubject}, nil)
		ss := &fakeServerStream{ctx: ctxWithAuth("Bearer good")}
		if err := ic(nil, ss, info, handler); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotSubject != grpcSubject {
			t.Fatalf("subject = %q, want %q", gotSubject, grpcSubject)
		}
	})

	t.Run("missing token -> Unauthenticated", func(t *testing.T) {
		ic := StreamServerInterceptor(fakeReviewer{}, []string{grpcSubject}, nil)
		ss := &fakeServerStream{ctx: ctxWithAuth("")}
		wantCode(t, ic(nil, ss, info, handler), codes.Unauthenticated)
	})

	t.Run("wrong subject -> PermissionDenied", func(t *testing.T) {
		ic := StreamServerInterceptor(fakeReviewer{authenticated: true, username: "system:serviceaccount:other:thing"}, []string{grpcSubject}, nil)
		ss := &fakeServerStream{ctx: ctxWithAuth(testBearerX)}
		wantCode(t, ic(nil, ss, info, handler), codes.PermissionDenied)
	})

	t.Run("nil reviewer -> pass-through", func(t *testing.T) {
		ic := StreamServerInterceptor(nil, nil, nil)
		ss := &fakeServerStream{ctx: ctxWithAuth("")}
		if err := ic(nil, ss, info, handler); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}
