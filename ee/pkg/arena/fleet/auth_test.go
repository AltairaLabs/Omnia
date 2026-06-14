/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package fleet

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/facade"
)

// stubTokenSource records the (agent, workspace) it was asked for and returns a
// canned token/error.
type stubTokenSource struct {
	token    string
	err      error
	gotAgent string
	gotWS    string
}

type fakeDialer struct {
	gotURL     string
	gotHeaders http.Header
}

func (d *fakeDialer) DialContext(_ context.Context, urlStr string, headers http.Header) (Conn, error) {
	d.gotURL = urlStr
	d.gotHeaders = headers.Clone()
	return &fakeConn{}, nil
}

type fakeConn struct{}

func (c *fakeConn) ReadMessage() (int, []byte, error) {
	msg := facade.ServerMessage{Type: facade.MessageTypeConnected, SessionID: "sess-fake", Timestamp: time.Now()}
	data, err := json.Marshal(msg)
	if err != nil {
		return 0, nil, err
	}
	return websocket.TextMessage, data, nil
}

func (c *fakeConn) WriteMessage(_ int, _ []byte) error { return nil }

func (c *fakeConn) SetReadDeadline(_ time.Time) error { return nil }

func (c *fakeConn) Close() error { return nil }

func (s *stubTokenSource) Token(agent, workspace string) (string, error) {
	s.gotAgent = agent
	s.gotWS = workspace
	return s.token, s.err
}

// connectedServer returns a ws URL whose handler completes the facade
// connected-handshake then idles, capturing the upgrade request headers.
func connectedServer(t *testing.T, captured *http.Header) string {
	return testServerWithHeaders(t, captured, func(conn *websocket.Conn) {
		writeServerMsg(t, conn, facade.ServerMessage{
			Type:      facade.MessageTypeConnected,
			SessionID: "sess-auth",
			Timestamp: time.Now(),
		})
		time.Sleep(time.Second)
	})
}

func TestSetAuth_AttachesBearerOnDial(t *testing.T) {
	dialer := &fakeDialer{}
	ts := &stubTokenSource{token: "test-jwt"}
	p := NewProvider("agent-rag-hero", "wss://agent.example/ws?agent=rag-hero&namespace=demo", dialer)
	p.SetAuth(ts, "rag-hero", "demo")

	require.NoError(t, p.Connect(context.Background()))

	assert.Equal(t, "Bearer test-jwt", dialer.gotHeaders.Get("Authorization"),
		"dial should attach the mgmt-plane Bearer token")
	assert.Equal(t, "rag-hero", ts.gotAgent, "token requested for the agent")
	assert.Equal(t, "demo", ts.gotWS, "token requested for the workspace")
}

func TestSetAuth_NilSourceLeavesDialUnauthenticated(t *testing.T) {
	var captured http.Header
	wsURL := connectedServer(t, &captured)

	// No SetAuth call — and an explicit nil source — both leave the header off.
	p := NewProvider("agent-x", wsURL, nil)
	p.SetAuth(nil, "x", "ws")

	require.NoError(t, p.Connect(context.Background()))
	defer func() { _ = p.Close() }()

	assert.Empty(t, captured.Get("Authorization"),
		"no token source should mean no Authorization header")
}

func TestSetAuth_RequiresWSSForBearerToken(t *testing.T) {
	dialer := &fakeDialer{}
	ts := &stubTokenSource{token: "test-jwt"}
	p := NewProvider("agent-x", "ws://agent.example/ws?agent=x&namespace=demo", dialer)
	p.SetAuth(ts, "x", "demo")

	err := p.Connect(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "use wss://")
	assert.Empty(t, ts.gotAgent)
	assert.Empty(t, ts.gotWS)
	assert.Empty(t, dialer.gotURL)
	assert.Empty(t, dialer.gotHeaders.Get("Authorization"))
}

func TestSetAuth_AllowsWSSAndAttachesBearerToken(t *testing.T) {
	dialer := &fakeDialer{}
	ts := &stubTokenSource{token: "test-jwt"}
	p := NewProvider("agent-rag-hero", "wss://agent.example/ws?agent=rag-hero&namespace=demo", dialer)
	p.SetAuth(ts, "rag-hero", "demo")

	require.NoError(t, p.Connect(context.Background()))

	assert.Equal(t, "wss://agent.example/ws?agent=rag-hero&namespace=demo", dialer.gotURL)
	assert.Equal(t, "Bearer test-jwt", dialer.gotHeaders.Get("Authorization"))
	assert.Equal(t, "rag-hero", ts.gotAgent)
	assert.Equal(t, "demo", ts.gotWS)
}

func TestSetAuth_TokenErrorFailsDial(t *testing.T) {
	dialer := &fakeDialer{}
	ts := &stubTokenSource{err: errors.New("dashboard 403: not allowlisted")}
	p := NewProvider("agent-y", "wss://agent.example/ws?agent=y&namespace=ws", dialer)
	p.SetAuth(ts, "y", "ws")

	err := p.Connect(context.Background())
	require.Error(t, err, "dial must fail when the token cannot be obtained")
	assert.Contains(t, err.Error(), "mgmt-plane token")
	assert.Empty(t, dialer.gotURL)
}
