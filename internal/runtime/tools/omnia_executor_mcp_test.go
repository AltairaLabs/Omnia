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

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countingAcquirer returns a fresh, incrementing token on every call so tests
// can prove injectedHeaderTransport.RoundTrip acquires the WIF token per
// request rather than baking a stale one in at transport-build time.
type countingAcquirer struct {
	calls int
}

func (c *countingAcquirer) Token(context.Context, string) (string, error) {
	c.calls++
	return fmt.Sprintf("tok-%d", c.calls), nil
}

func newMCPTestRequest(t *testing.T) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://mcp.example.invalid", nil)
	require.NoError(t, err)
	return req
}

// TestInjectedHeaderTransport_WIF_PerRequestAcquire proves the WIF token is
// resolved fresh inside RoundTrip on every call (not once at transport-build
// time), since the transport outlives the token's ~1h lifetime.
func TestInjectedHeaderTransport_WIF_PerRequestAcquire(t *testing.T) {
	base := &recordingRoundTripper{}
	acq := &countingAcquirer{}
	rt := &injectedHeaderTransport{base: base, acquirer: acq, wifCloud: cloudAzure, wifAudience: "api://tool"}

	_, err := rt.RoundTrip(newMCPTestRequest(t))
	require.NoError(t, err)
	assert.Equal(t, "Bearer tok-1", base.lastReq.Header.Get("Authorization"))

	_, err = rt.RoundTrip(newMCPTestRequest(t))
	require.NoError(t, err)
	assert.Equal(t, "Bearer tok-2", base.lastReq.Header.Get("Authorization"),
		"a fresh token must be acquired on each RoundTrip, not cached at transport-build time")
	assert.Equal(t, 2, acq.calls)
}

// TestInjectedHeaderTransport_WIF_CustomHeader proves a configured wifHeader
// overrides the default Authorization header name.
func TestInjectedHeaderTransport_WIF_CustomHeader(t *testing.T) {
	base := &recordingRoundTripper{}
	acq := &countingAcquirer{}
	rt := &injectedHeaderTransport{base: base, acquirer: acq, wifCloud: cloudAzure, wifAudience: "api://tool", wifHeader: "X-Tool-Auth"}

	_, err := rt.RoundTrip(newMCPTestRequest(t))
	require.NoError(t, err)
	assert.Equal(t, "Bearer tok-1", base.lastReq.Header.Get("X-Tool-Auth"))
	assert.Empty(t, base.lastReq.Header.Get("Authorization"))
}

// TestInjectedHeaderTransport_StaticTokenStillWorks proves a transport
// configured with an inline bearer token (no path, no WIF) resolves to the
// expected Authorization header.
func TestInjectedHeaderTransport_StaticTokenStillWorks(t *testing.T) {
	base := &recordingRoundTripper{}
	rt := &injectedHeaderTransport{base: base, authType: "bearer", authToken: "static-tok"}

	_, err := rt.RoundTrip(newMCPTestRequest(t))
	require.NoError(t, err)
	assert.Equal(t, "Bearer static-tok", base.lastReq.Header.Get("Authorization"))
}

// TestInjectedHeaderTransport_FileToken_RereadPerRequest proves a file-backed
// tool credential (a mounted Secret or projected serviceAccount token) is
// re-read on every RoundTrip, so a rotated/refreshed token is used rather than
// a value cached when the long-lived transport was built (#1797).
func TestInjectedHeaderTransport_FileToken_RereadPerRequest(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/token"
	require.NoError(t, os.WriteFile(path, []byte("tok-A"), 0o600))

	base := &recordingRoundTripper{}
	rt := &injectedHeaderTransport{base: base, authType: "bearer", authTokenPath: path}

	_, err := rt.RoundTrip(newMCPTestRequest(t))
	require.NoError(t, err)
	assert.Equal(t, "Bearer tok-A", base.lastReq.Header.Get("Authorization"))

	require.NoError(t, os.WriteFile(path, []byte("tok-B"), 0o600))
	_, err = rt.RoundTrip(newMCPTestRequest(t))
	require.NoError(t, err)
	assert.Equal(t, "Bearer tok-B", base.lastReq.Header.Get("Authorization"),
		"the token file must be re-read per request, not cached at transport-build time")
}

// TestInjectedHeaderTransport_WIF_FailsLoudNonAzure proves an unsupported
// cloud fails the RoundTrip rather than sending an unauthenticated request.
func TestInjectedHeaderTransport_WIF_FailsLoudNonAzure(t *testing.T) {
	base := &recordingRoundTripper{}
	rt := &injectedHeaderTransport{base: base, acquirer: fakeAcquirer{tok: "x"}, wifCloud: "aws", wifAudience: "api://tool"}

	_, err := rt.RoundTrip(newMCPTestRequest(t))
	require.Error(t, err)
	assert.Nil(t, base.lastReq, "no request should reach the base transport on a WIF resolution failure")
}

// TestInjectedHeaderTransport_WIF_FailsLoudNilAcquirer proves a WIF-configured
// transport with no acquirer wired (e.g. no ambient Azure identity) fails
// loud instead of sending an unauthenticated request.
func TestInjectedHeaderTransport_WIF_FailsLoudNilAcquirer(t *testing.T) {
	base := &recordingRoundTripper{}
	rt := &injectedHeaderTransport{base: base, wifCloud: cloudAzure, wifAudience: "api://tool"}

	_, err := rt.RoundTrip(newMCPTestRequest(t))
	require.Error(t, err)
	assert.Nil(t, base.lastReq)
}

// TestBuildMCPTransport_WorkloadIdentity proves buildMCPTransport wires the
// WIF params onto the transport instead of pre-computing a static header
// (which would error on the unknown "workloadIdentity" auth type).
func TestBuildMCPTransport_WorkloadIdentity(t *testing.T) {
	e := &OmniaExecutor{tokenAcquirer: fakeAcquirer{tok: "wtok"}}
	cfg := &MCPCfg{
		Transport:    "sse",
		Endpoint:     "http://localhost:3000/mcp",
		AuthType:     authTypeWorkloadIdentity,
		AuthCloud:    cloudAzure,
		AuthAudience: "api://tool",
	}

	transport, err := e.buildMCPTransport(cfg)
	require.NoError(t, err)
	require.NotNil(t, transport)
}

func TestInjectedHeaderTransport_FileToken_MissingFileErrors(t *testing.T) {
	base := &recordingRoundTripper{}
	rt := &injectedHeaderTransport{base: base, authType: "bearer", authTokenPath: "/nonexistent/token"}
	if _, err := rt.RoundTrip(newMCPTestRequest(t)); err == nil {
		t.Fatal("expected RoundTrip to fail when the token file is unreadable")
	}
}
