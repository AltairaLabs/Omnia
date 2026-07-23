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
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	pkruntime "github.com/altairalabs/omnia/internal/runtime"
	"github.com/altairalabs/omnia/pkg/runtime/conformance"
)

// conformancePack is a minimal, valid PromptPack for protocol-only conformance:
// one default prompt, no tools, no evals.
const conformancePack = `{
	"id": "conformance-facade",
	"name": "conformance-facade",
	"version": "1.0.0",
	"template_engine": {"version": "v1", "syntax": "{{variable}}"},
	"prompts": {
		"default": {"id": "default", "name": "default", "version": "1.0.0", "system_template": "You are a test assistant."}
	}
}`

// writePack writes a conformance pack to a temp file and returns its path.
func writePack(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pack.promptpack")
	require.NoError(t, os.WriteFile(path, []byte(conformancePack), 0o600))
	return path
}

// mockConfig returns a minimal mock-provider Config sufficient to build a
// conformant runtime with no external dependencies.
func mockConfig(t *testing.T) *pkruntime.Config {
	t.Helper()
	return &pkruntime.Config{
		AgentName:      "conformance",
		Namespace:      "conformance",
		PromptName:     "default",
		ProviderType:   "mock",
		Model:          "mock-model",
		MockProvider:   true,
		PromptPackPath: writePack(t),
	}
}

// TestFacade_IsConformant is the regression gate for the public facade: a
// Runtime built through New (the same construction path FromEnv and downstream
// callers use) must expose a gRPC server that passes the protocol-only
// conformance suite. A change that breaks contract compliance — dropping the
// hello, mis-advertising a capability, crashing on empty input — fails here.
func TestFacade_IsConformant(t *testing.T) {
	// Media storage must be off so New builds no external backend.
	t.Setenv("OMNIA_MEDIA_STORAGE_TYPE", "")

	rt, err := New(mockConfig(t), WithLogger(logr.Discard()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = rt.Close() })

	lis := bufconn.Listen(1024 * 1024)
	gs := rt.newGRPCServer()
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	res := conformance.Run(context.Background(), conformance.Config{Conn: conn})
	for _, c := range res.Checks {
		t.Logf("%-26s %-5s %s", c.Name, c.Status, c.Detail)
	}
	require.True(t, res.Passed, "facade-built runtime must pass conformance")
}
