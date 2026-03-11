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

package tooltest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func TestHandleTestToolMethodNotAllowed(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	server := &Server{
		log:    log,
		tester: NewTester(nil, log),
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/default/toolregistries/my-reg/test", nil)
	w := httptest.NewRecorder()

	server.handleTestTool(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleTestToolMissingHandlerName(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	server := &Server{
		log:    log,
		tester: NewTester(nil, log),
	}

	body, _ := json.Marshal(TestRequest{
		Arguments: json.RawMessage(`{}`),
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/namespaces/default/toolregistries/my-reg/test", bytes.NewReader(body))
	req.SetPathValue("namespace", "default")
	req.SetPathValue("registry", "my-reg")
	w := httptest.NewRecorder()

	server.handleTestTool(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleTestToolInvalidBody(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	server := &Server{
		log:    log,
		tester: NewTester(nil, log),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/namespaces/default/toolregistries/my-reg/test",
		bytes.NewReader([]byte(`{invalid json`)))
	req.SetPathValue("namespace", "default")
	req.SetPathValue("registry", "my-reg")
	w := httptest.NewRecorder()

	server.handleTestTool(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleTestToolMissingPathValues(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	server := &Server{
		log:    log,
		tester: NewTester(nil, log),
	}

	body, _ := json.Marshal(TestRequest{
		HandlerName: "test",
		Arguments:   json.RawMessage(`{}`),
	})

	// No path values set
	req := httptest.NewRequest(http.MethodPost, "/api/v1/namespaces//toolregistries//test", bytes.NewReader(body))
	w := httptest.NewRecorder()

	server.handleTestTool(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleHealthz(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	server := &Server{log: log}

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()

	server.handleHealthz(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if w.Body.String() != "ok" {
		t.Errorf("body = %q, want %q", w.Body.String(), "ok")
	}
}

func TestServerShutdownNilServer(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	server := &Server{log: log}

	if err := server.Shutdown(t.Context()); err != nil {
		t.Errorf("Shutdown() error = %v", err)
	}
}

func TestNewServer(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	s := testScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()

	srv := NewServer(":0", c, log)
	if srv == nil {
		t.Fatal("expected non-nil server")
	}
	if srv.addr != ":0" {
		t.Errorf("addr = %q, want %q", srv.addr, ":0")
	}
	if srv.tester == nil {
		t.Error("expected non-nil tester")
	}
}

func TestHandleTestToolRegistryNotFound(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	s := testScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()

	server := &Server{
		log:    log,
		tester: NewTester(c, log),
	}

	body, _ := json.Marshal(TestRequest{
		HandlerName: "my-handler",
		Arguments:   json.RawMessage(`{}`),
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/namespaces/default/toolregistries/nonexistent/test", bytes.NewReader(body))
	req.SetPathValue("namespace", "default")
	req.SetPathValue("registry", "nonexistent")
	w := httptest.NewRecorder()

	server.handleTestTool(w, req)

	// Should return 422 because the test fails (registry not found)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnprocessableEntity)
	}

	var resp TestResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Success {
		t.Error("expected failure response")
	}
	if resp.Error == "" {
		t.Error("expected non-empty error")
	}
}

func TestHandleTestToolWithToolNameError(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	s := testScheme()
	registry := &omniav1alpha1.ToolRegistry{
		ObjectMeta: metav1.ObjectMeta{Name: "my-reg", Namespace: "default"},
		Spec: omniav1alpha1.ToolRegistrySpec{
			Handlers: []omniav1alpha1.HandlerDefinition{
				{
					Name: "my-handler",
					Type: omniav1alpha1.HandlerTypeHTTP,
					// No Tool and no ToolName => toolName required error
				},
			},
		},
	}
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(registry).Build()

	server := &Server{
		log:    log,
		tester: NewTester(c, log),
	}

	body, _ := json.Marshal(TestRequest{
		HandlerName: "my-handler",
		Arguments:   json.RawMessage(`{}`),
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/namespaces/default/toolregistries/my-reg/test", bytes.NewReader(body))
	req.SetPathValue("namespace", "default")
	req.SetPathValue("registry", "my-reg")
	w := httptest.NewRecorder()

	server.handleTestTool(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnprocessableEntity)
	}

	var resp TestResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Success {
		t.Error("expected failure")
	}
	if resp.HandlerType != "http" {
		t.Errorf("HandlerType = %q, want %q", resp.HandlerType, "http")
	}
}

func TestServerStartAndShutdown(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	s := testScheme()
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "dummy", Namespace: "default"},
		Data:       map[string][]byte{"key": []byte("val")},
	}
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(secret).Build()

	// Find a free port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to find free port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	srv := NewServer(addr, c, log)

	// Start in background
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start(context.Background())
	}()

	// Wait for server to be ready
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		conn, dialErr := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if dialErr == nil {
			conn.Close()
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Test healthz endpoint via the running server
	resp, err := http.Get(fmt.Sprintf("http://%s/healthz", addr))
	if err != nil {
		t.Fatalf("failed to GET /healthz: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("healthz status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	// Shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() error: %v", err)
	}

	// Start should return http.ErrServerClosed
	startErr := <-errCh
	if startErr != nil && startErr != http.ErrServerClosed {
		t.Errorf("Start() returned unexpected error: %v", startErr)
	}
}
