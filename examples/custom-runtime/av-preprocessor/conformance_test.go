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

package main

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	"github.com/altairalabs/omnia/pkg/runtime/conformance"
	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"
)

// TestExampleRuntime_IsConformant dogfoods the conformance suite against this
// example: a second, independent runtime the suite must pass. Protocol-only ⇒
// no ffmpeg is needed at test time.
func TestExampleRuntime_IsConformant(t *testing.T) {
	lis := bufconn.Listen(1024 * 1024)
	gs := grpc.NewServer()
	runtimev1.RegisterRuntimeServiceServer(gs, server{})
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
	require.True(t, res.Passed, "example runtime must pass conformance")
}

// TestBuildVideoPreprocessStage constructs the real published video-to-frames
// stage — proving the A/V seam compiles and wires against the SDK.
func TestBuildVideoPreprocessStage(t *testing.T) {
	require.NotNil(t, buildVideoPreprocessStage())
}
