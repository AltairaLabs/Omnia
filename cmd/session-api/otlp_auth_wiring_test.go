/*
Copyright 2026 Altaira Labs.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-logr/logr"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/otlp"

	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
)

// noopWriter is an otlp.SessionWriter that does nothing — sufficient for the
// auth-wiring tests, which only care whether requests are gated before reaching
// the transformer.
type noopWriter struct{}

func (noopWriter) GetSession(context.Context, string) (*session.Session, error) {
	return nil, nil
}
func (noopWriter) CreateSession(context.Context, *session.Session) error { return nil }
func (noopWriter) AppendMessage(context.Context, string, *session.Message) error {
	return nil
}
func (noopWriter) UpdateSessionStatus(context.Context, string, session.SessionStatusUpdate) error {
	return nil
}

const otlpAllowedSubject = "system:serviceaccount:ns:allowed"

// TestOTLPGRPC_AuthInterceptorWired stands up the real OTLP gRPC server (built
// with otlpGRPCServerOptions + a fake reviewer) over an in-memory listener and
// verifies Export is rejected without/with-bad tokens and accepted for an
// allowlisted subject. This is the OTLP analogue of the JSON API wiring test.
func TestOTLPGRPC_AuthInterceptorWired(t *testing.T) {
	reviewer := fakeReviewer{authenticated: true, username: otlpAllowedSubject}

	srv := grpc.NewServer(otlpGRPCServerOptions(reviewer, []string{otlpAllowedSubject}, nil)...)
	receiver := otlp.NewReceiver(otlp.NewTransformer(noopWriter{}, logr.Discard()), logr.Discard())
	coltracepb.RegisterTraceServiceServer(srv, receiver)

	lis := bufconn.Listen(1024 * 1024)
	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()

	dial := func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }
	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(dial),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()
	client := coltracepb.NewTraceServiceClient(conn)
	req := &coltracepb.ExportTraceServiceRequest{}

	t.Run("no token is Unauthenticated", func(t *testing.T) {
		_, err := client.Export(context.Background(), req)
		if status.Code(err) != codes.Unauthenticated {
			t.Fatalf("expected Unauthenticated, got %v", err)
		}
	})

	t.Run("allowlisted subject is accepted", func(t *testing.T) {
		ctx := metadata.AppendToOutgoingContext(context.Background(),
			"authorization", "Bearer good-token")
		_, err := client.Export(ctx, req)
		if err != nil {
			t.Fatalf("allowlisted subject should pass, got %v", err)
		}
	})
}

// TestOTLPGRPC_NonAllowlistedSubjectIsPermissionDenied verifies a caller that
// authenticates but is not on the allowlist is rejected with PermissionDenied.
func TestOTLPGRPC_NonAllowlistedSubjectIsPermissionDenied(t *testing.T) {
	reviewer := fakeReviewer{authenticated: true, username: "system:serviceaccount:ns:other"}

	srv := grpc.NewServer(otlpGRPCServerOptions(reviewer, []string{otlpAllowedSubject}, nil)...)
	receiver := otlp.NewReceiver(otlp.NewTransformer(noopWriter{}, logr.Discard()), logr.Discard())
	coltracepb.RegisterTraceServiceServer(srv, receiver)

	lis := bufconn.Listen(1024 * 1024)
	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()

	dial := func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }
	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(dial),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()
	client := coltracepb.NewTraceServiceClient(conn)

	ctx := metadata.AppendToOutgoingContext(context.Background(),
		"authorization", "Bearer good-token")
	_, err = client.Export(ctx, &coltracepb.ExportTraceServiceRequest{})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected PermissionDenied, got %v", err)
	}
}

// TestOTLPGRPC_NilReviewerNoAuth verifies that when the reviewer is nil (auth
// disabled), the OTLP gRPC server does not gate Export — a request with no token
// succeeds.
func TestOTLPGRPC_NilReviewerNoAuth(t *testing.T) {
	srv := grpc.NewServer(otlpGRPCServerOptions(nil, nil, nil)...)
	receiver := otlp.NewReceiver(otlp.NewTransformer(noopWriter{}, logr.Discard()), logr.Discard())
	coltracepb.RegisterTraceServiceServer(srv, receiver)

	lis := bufconn.Listen(1024 * 1024)
	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()

	dial := func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }
	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(dial),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()
	client := coltracepb.NewTraceServiceClient(conn)

	if _, err := client.Export(context.Background(), &coltracepb.ExportTraceServiceRequest{}); err != nil {
		t.Fatalf("nil reviewer must not gate Export, got %v", err)
	}
}

// TestBuildOTLPHTTPHandler_AuthWired verifies the OTLP/HTTP handler assembled by
// buildOTLPHTTPHandler (with a fake reviewer) rejects unauthenticated POSTs with
// 401 and lets an allowlisted subject through to the OTLP handler (which then
// returns a non-auth status for the empty/invalid body).
func TestBuildOTLPHTTPHandler_AuthWired(t *testing.T) {
	reviewer := fakeReviewer{authenticated: true, username: otlpAllowedSubject}
	h := buildOTLPHTTPHandler(
		otlp.NewTransformer(noopWriter{}, logr.Discard()),
		logr.Discard(), reviewer, []string{otlpAllowedSubject}, nil,
	)

	t.Run("no token is 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/v1/traces", nil)
		req.Header.Set("Content-Type", "application/x-protobuf")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 with no token, got %d body=%q", rr.Code, rr.Body.String())
		}
	})

	t.Run("allowlisted subject passes auth", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/v1/traces", nil)
		req.Header.Set("Content-Type", "application/x-protobuf")
		req.Header.Set("Authorization", "Bearer good-token")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code == http.StatusUnauthorized || rr.Code == http.StatusForbidden {
			t.Fatalf("allowlisted subject should pass auth, got %d body=%q", rr.Code, rr.Body.String())
		}
	})
}

// TestBuildOTLPHTTPHandler_NonAllowlistedIs403 verifies an authenticated but
// non-allowlisted subject is rejected with 403.
func TestBuildOTLPHTTPHandler_NonAllowlistedIs403(t *testing.T) {
	reviewer := fakeReviewer{authenticated: true, username: "system:serviceaccount:ns:other"}
	h := buildOTLPHTTPHandler(
		otlp.NewTransformer(noopWriter{}, logr.Discard()),
		logr.Discard(), reviewer, []string{otlpAllowedSubject}, nil,
	)

	req := httptest.NewRequest(http.MethodPost, "/v1/traces", nil)
	req.Header.Set("Content-Type", "application/x-protobuf")
	req.Header.Set("Authorization", "Bearer good-token")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("non-allowlisted subject should be 403, got %d body=%q", rr.Code, rr.Body.String())
	}
}

// TestBuildOTLPHTTPHandler_NilReviewerNoAuth verifies a nil reviewer leaves the
// OTLP/HTTP handler ungated.
func TestBuildOTLPHTTPHandler_NilReviewerNoAuth(t *testing.T) {
	h := buildOTLPHTTPHandler(
		otlp.NewTransformer(noopWriter{}, logr.Discard()),
		logr.Discard(), nil, nil, nil,
	)

	req := httptest.NewRequest(http.MethodPost, "/v1/traces", nil)
	req.Header.Set("Content-Type", "application/x-protobuf")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code == http.StatusUnauthorized || rr.Code == http.StatusForbidden {
		t.Fatalf("nil reviewer must not gate OTLP/HTTP, got %d", rr.Code)
	}
}
