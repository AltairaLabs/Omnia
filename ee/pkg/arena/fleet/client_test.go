/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package fleet

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/facade"
)

// testServer creates a WebSocket test server that runs the given handler function.
func testServer(t *testing.T, handler func(*websocket.Conn)) string {
	t.Helper()
	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("upgrade failed: %v", err)
		}
		defer func() { _ = conn.Close() }()
		handler(conn)
	}))
	t.Cleanup(srv.Close)

	return "ws" + strings.TrimPrefix(srv.URL, "http")
}

func writeServerMsg(t *testing.T, conn *websocket.Conn, msg facade.ServerMessage) {
	t.Helper()
	data, err := json.Marshal(msg)
	require.NoError(t, err)
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, data))
}

func readClientMsg(t *testing.T, conn *websocket.Conn) facade.ClientMessage {
	t.Helper()
	_, data, err := conn.ReadMessage()
	require.NoError(t, err)
	var msg facade.ClientMessage
	require.NoError(t, json.Unmarshal(data, &msg))
	return msg
}
