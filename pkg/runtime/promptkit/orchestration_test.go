/*
Copyright 2026 Altaira Labs.

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

package promptkit

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/PromptKit/sdk"

	pkruntime "github.com/altairalabs/omnia/internal/runtime"
)

// freePort returns an OS-assigned free TCP port. A tiny race window exists
// between close and rebind, acceptable for these tests.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := l.Addr().(*net.TCPAddr).Port
	require.NoError(t, l.Close())
	return port
}

// TestNew_DefaultLoggerAndSDKOptions covers the no-caller-logger path (New
// builds its own Zap logger via ensureLogger) and the opaque SDK-option
// passthrough (WithSDKOptions → buildServerOpts appends pkruntime.WithSDKOptions).
func TestNew_DefaultLoggerAndSDKOptions(t *testing.T) {
	t.Setenv("OMNIA_MEDIA_STORAGE_TYPE", "")

	rt, err := New(mockConfig(t), WithSDKOptions(sdk.WithAPIKey("test-key")))
	require.NoError(t, err)
	t.Cleanup(func() { _ = rt.Close() })
	require.NotNil(t, rt.server)
	require.NotNil(t, rt.log.GetSink(), "New must construct a default logger when none supplied")
}

// TestNew_RichConfigWiresOptionalBranches drives the optional buildServerOpts
// branches (session recording, tracing provider, promptpack version, skill
// manifest) plus memoryServerOpts and newTracingProvider's enabled path in one
// construction.
func TestNew_RichConfigWiresOptionalBranches(t *testing.T) {
	t.Setenv("OMNIA_MEDIA_STORAGE_TYPE", "")
	manifest := filepath.Join(t.TempDir(), "manifest.yaml")
	require.NoError(t, os.WriteFile(manifest, []byte("sources: []\n"), 0o600))
	t.Setenv("OMNIA_PROMPTPACK_MANIFEST_PATH", manifest)

	cfg := mockConfig(t)
	cfg.PromptPackVersion = "1.2.3"
	cfg.SessionAPIURL = "http://session-api.svc:8080"
	cfg.MemoryEnabled = true
	cfg.MemoryAPIURL = "http://memory-api.svc:8080"
	cfg.WorkspaceUID = "ws-uid-1"
	cfg.TracingEnabled = true
	cfg.TracingInsecure = true
	cfg.TracingEndpoint = "localhost:4317"

	rt, err := New(cfg, WithLogger(logr.Discard()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = rt.Close() })
	require.NotNil(t, rt.tracing, "tracing provider must be constructed when TracingEnabled")
}

// TestNew_StateStoreError proves a bad context store fails construction: a redis
// context type with an unparseable URL surfaces as a New error (not a silent
// nil store).
func TestNew_StateStoreError(t *testing.T) {
	t.Setenv("OMNIA_MEDIA_STORAGE_TYPE", "")
	cfg := mockConfig(t)
	cfg.ContextType = pkruntime.ContextTypeRedis
	cfg.ContextURL = "://not-a-url"

	_, err := New(cfg, WithLogger(logr.Discard()))
	require.Error(t, err)
}

func TestNewStateStore(t *testing.T) {
	log := logr.Discard()

	t.Run("memory type yields a store", func(t *testing.T) {
		store, err := newStateStore(&pkruntime.Config{ContextType: pkruntime.ContextTypeMemory}, log)
		require.NoError(t, err)
		require.NotNil(t, store)
	})

	t.Run("redis type connects to a live server", func(t *testing.T) {
		mr := miniredis.RunT(t)
		store, err := newStateStore(&pkruntime.Config{
			ContextType: pkruntime.ContextTypeRedis,
			ContextURL:  "redis://" + mr.Addr(),
		}, log)
		require.NoError(t, err)
		require.NotNil(t, store)
	})

	t.Run("redis type with an unreachable server errors", func(t *testing.T) {
		_, err := newStateStore(&pkruntime.Config{
			ContextType: pkruntime.ContextTypeRedis,
			ContextURL:  "redis://127.0.0.1:1", // nothing listens on port 1
		}, log)
		require.Error(t, err)
	})

	t.Run("unset type yields a nil store", func(t *testing.T) {
		store, err := newStateStore(&pkruntime.Config{}, log)
		require.NoError(t, err)
		require.Nil(t, store)
	})
}

func TestLoadEvalDefs(t *testing.T) {
	log := logr.Discard()

	t.Run("disabled yields nil", func(t *testing.T) {
		require.Nil(t, loadEvalDefs(&pkruntime.Config{EvalEnabled: false}, log))
	})

	t.Run("enabled with a valid pack loads without error", func(t *testing.T) {
		defs := loadEvalDefs(&pkruntime.Config{EvalEnabled: true, PromptPackPath: writePack(t)}, log)
		// The conformance pack declares no evals; the point is the enabled branch
		// runs without error and returns a (possibly empty) slice.
		require.Empty(t, defs)
	})

	t.Run("enabled with an unreadable pack logs and returns nil", func(t *testing.T) {
		require.Nil(t, loadEvalDefs(&pkruntime.Config{EvalEnabled: true, PromptPackPath: "/no/such/pack"}, log))
	})
}

func TestMemoryServerOpts(t *testing.T) {
	log := logr.Discard()

	t.Run("disabled yields no options", func(t *testing.T) {
		require.Empty(t, memoryServerOpts(&pkruntime.Config{MemoryEnabled: false}, log))
	})

	t.Run("enabled without an API URL yields no options", func(t *testing.T) {
		require.Empty(t, memoryServerOpts(&pkruntime.Config{MemoryEnabled: true}, log))
	})

	t.Run("enabled with an API URL and workspace UID wires the store", func(t *testing.T) {
		opts := memoryServerOpts(&pkruntime.Config{
			MemoryEnabled: true,
			MemoryAPIURL:  "http://memory-api.svc:8080",
			WorkspaceUID:  "ws-1",
		}, log)
		require.NotEmpty(t, opts)
	})

	t.Run("enabled with an API URL but no workspace UID still wires the store", func(t *testing.T) {
		opts := memoryServerOpts(&pkruntime.Config{
			MemoryEnabled: true,
			MemoryAPIURL:  "http://memory-api.svc:8080",
		}, log)
		require.NotEmpty(t, opts)
	})
}

func TestInitTools(t *testing.T) {
	log := logr.Discard()

	t.Run("no config path is a no-op", func(t *testing.T) {
		srv := pkruntime.NewServer(pkruntime.WithLogger(log))
		t.Cleanup(func() { _ = srv.Close() })
		initTools(&pkruntime.Config{}, srv, log) // must not panic
	})

	t.Run("valid config initializes tools and enriches registry", func(t *testing.T) {
		dir := t.TempDir()
		toolsPath := filepath.Join(dir, "tools.yaml")
		require.NoError(t, os.WriteFile(toolsPath, []byte("handlers: []\n"), 0o600))

		cfg := &pkruntime.Config{ToolsConfigPath: toolsPath, ToolRegistryName: "orders", ToolRegistryNamespace: "ns"}
		srv := pkruntime.NewServer(pkruntime.WithLogger(log), pkruntime.WithToolsConfig(toolsPath))
		t.Cleanup(func() { _ = srv.Close() })

		initTools(cfg, srv, log)

		name, ns := pkruntime.ServerToolRegistryInfo(srv)
		require.Equal(t, "orders", name)
		require.Equal(t, "ns", ns)
	})

	t.Run("initialize failure is logged and swallowed", func(t *testing.T) {
		cfg := &pkruntime.Config{ToolsConfigPath: filepath.Join(t.TempDir(), "missing.yaml")}
		srv := pkruntime.NewServer(pkruntime.WithLogger(log), pkruntime.WithToolsConfig(cfg.ToolsConfigPath))
		t.Cleanup(func() { _ = srv.Close() })
		initTools(cfg, srv, log) // must not panic despite the load error
	})
}

func TestNewTracingProvider(t *testing.T) {
	t.Run("disabled yields nil", func(t *testing.T) {
		require.Nil(t, newTracingProvider(&pkruntime.Config{TracingEnabled: false}, logr.Discard()))
	})

	t.Run("enabled yields a provider", func(t *testing.T) {
		p := newTracingProvider(&pkruntime.Config{
			TracingEnabled:  true,
			TracingInsecure: true,
			TracingEndpoint: "localhost:4317",
			AgentName:       "a",
		}, logr.Discard())
		require.NotNil(t, p)
	})
}

// TestReportStartup_NoAgentIdentitySkipsK8s covers reportStartup's guard: with
// no agent name/namespace it validates the pack and returns without touching
// Kubernetes.
func TestReportStartup_NoAgentIdentitySkipsK8s(t *testing.T) {
	t.Setenv("OMNIA_MEDIA_STORAGE_TYPE", "")
	cfg := mockConfig(t)
	cfg.AgentName = ""
	cfg.Namespace = ""

	rt, err := New(cfg, WithLogger(logr.Discard()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = rt.Close() })

	rt.reportStartup(context.Background()) // must return without a k8s client
}

// TestServe_StartsServesAndShutsDown is the end-to-end wiring proof for the
// serving path: Serve brings up the gRPC and HTTP health servers, /healthz and
// /metrics answer, and cancelling the context returns Serve after a clean
// shutdown.
func TestServe_StartsServesAndShutsDown(t *testing.T) {
	t.Setenv("OMNIA_MEDIA_STORAGE_TYPE", "")
	cfg := mockConfig(t)
	cfg.GRPCPort = freePort(t)
	cfg.HealthPort = freePort(t)

	rt, err := New(cfg, WithLogger(logr.Discard()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = rt.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan error, 1)
	go func() { served <- rt.Serve(ctx) }()

	base := fmt.Sprintf("http://127.0.0.1:%d", cfg.HealthPort)
	requireStatus(t, base+"/healthz", http.StatusOK)
	requireStatus(t, base+"/readyz", http.StatusOK)
	requireStatus(t, base+"/metrics", http.StatusOK)

	cancel()
	select {
	case err := <-served:
		require.NoError(t, err)
	case <-time.After(15 * time.Second):
		t.Fatal("Serve did not return after context cancellation")
	}
}

// TestServe_ReadyzFailsForInvalidPack proves the readiness probe drops the pod
// when the mounted pack becomes unreadable, independent of liveness.
func TestServe_ReadyzFailsForInvalidPack(t *testing.T) {
	t.Setenv("OMNIA_MEDIA_STORAGE_TYPE", "")
	cfg := mockConfig(t)
	cfg.GRPCPort = freePort(t)
	cfg.HealthPort = freePort(t)
	cfg.PromptPackPath = filepath.Join(t.TempDir(), "gone.promptpack") // never created

	rt, err := New(cfg, WithLogger(logr.Discard()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = rt.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	served := make(chan error, 1)
	go func() { served <- rt.Serve(ctx) }()

	base := fmt.Sprintf("http://127.0.0.1:%d", cfg.HealthPort)
	requireStatus(t, base+"/healthz", http.StatusOK)
	requireStatus(t, base+"/readyz", http.StatusServiceUnavailable)

	cancel()
	<-served
}

// requireStatus polls url until it answers with wantStatus or the deadline
// elapses, tolerating the brief window before the servers bind.
func requireStatus(t *testing.T, url string, wantStatus int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var lastErr error
	var lastStatus int
	for time.Now().Before(deadline) {
		resp, err := http.Get(url) //nolint:noctx // short-lived test probe
		if err != nil {
			lastErr = err
			time.Sleep(25 * time.Millisecond)
			continue
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		lastStatus = resp.StatusCode
		if resp.StatusCode == wantStatus {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("GET %s: never returned %d (last status %d, last err %v)", url, wantStatus, lastStatus, lastErr)
}
