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

package runtime

import (
	"context"
	"net"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	"github.com/altairalabs/omnia/pkg/runtime/conformance"
	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"
)

// TestReferenceRuntime_IsConformant is the regression gate: the real runtime,
// started with a minimal mock-provider config, must pass the protocol-only
// conformance suite. A future change that breaks contract compliance (dropping
// the hello, mis-advertising a capability, crashing on empty input) fails here.
func TestReferenceRuntime_IsConformant(t *testing.T) {
	packPath := t.TempDir() + "/pack.promptpack"
	packContent := `{
		"id": "conformance-ref",
		"name": "conformance-ref",
		"version": "1.0.0",
		"template_engine": {"version": "v1", "syntax": "{{variable}}"},
		"prompts": {
			"default": {"id": "default", "name": "default", "version": "1.0.0", "system_template": "You are a test assistant."}
		}
	}`
	require.NoError(t, writeTestFile(t, packPath, packContent))

	srv := NewServer(
		WithLogger(logr.Discard()),
		WithPackPath(packPath),
		WithPromptName("default"),
		WithMockProvider(true),
		WithProviderInfo("mock", "mock-model"),
	)

	lis := bufconn.Listen(1024 * 1024)
	gs := grpc.NewServer()
	runtimev1.RegisterRuntimeServiceServer(gs, srv)
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
	require.True(t, res.Passed, "reference runtime must pass conformance")
}
