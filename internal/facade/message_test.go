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

package facade

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/go-logr/logr"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
)

// mockToolRouter implements both MessageHandler and ClientToolRouter for testing.
type mockToolRouter struct {
	ackCalls    []ackCall
	resultCalls []resultCall
}

type ackCall struct {
	sessionID string
	callID    string
}

type resultCall struct {
	sessionID string
	result    *ClientToolResultInfo
}

func (m *mockToolRouter) Name() string { return "mock-tool-router" }

func (m *mockToolRouter) HandleMessage(_ context.Context, _ string, _ *ClientMessage, w ResponseWriter) error {
	return w.WriteDone("ok")
}

func (m *mockToolRouter) AckToolCall(sessionID string, callID string) {
	m.ackCalls = append(m.ackCalls, ackCall{sessionID, callID})
}

func (m *mockToolRouter) SendToolResult(sessionID string, result *ClientToolResultInfo) bool {
	m.resultCalls = append(m.resultCalls, resultCall{sessionID, result})
	return true
}

func TestLogCloseError(t *testing.T) {
	s := &Server{log: logr.Discard()}
	log := logr.Discard()

	// Expected close codes — should not panic or error
	s.logCloseError(&websocket.CloseError{Code: websocket.CloseNormalClosure}, log)
	s.logCloseError(&websocket.CloseError{Code: websocket.CloseGoingAway}, log)
	s.logCloseError(&websocket.CloseError{Code: websocket.CloseNoStatusReceived}, log)
	s.logCloseError(&websocket.CloseError{Code: websocket.CloseAbnormalClosure}, log)

	// Unexpected close error — should log
	s.logCloseError(&websocket.CloseError{Code: websocket.CloseProtocolError}, log)

	// Non-websocket error — should log
	s.logCloseError(fmt.Errorf("connection reset"), log)
}

func TestHandleClientMessage_ToolCallAckRouted(t *testing.T) {
	router := &mockToolRouter{}
	s := &Server{
		config:  DefaultServerConfig(),
		handler: router,
		metrics: &NoOpMetrics{},
		log:     logr.Discard(),
	}
	c := &Connection{sessionID: "sess-1"}

	msg := ClientMessage{
		Type: MessageTypeToolCallAck,
		ToolCallAck: &ToolCallAckInfo{
			CallID: "call-42",
		},
	}
	data, _ := json.Marshal(msg)
	s.handleClientMessage(context.Background(), c, data, logr.Discard())

	assert.Len(t, router.ackCalls, 1)
	assert.Equal(t, "sess-1", router.ackCalls[0].sessionID)
	assert.Equal(t, "call-42", router.ackCalls[0].callID)
}

func TestHandleClientMessage_ToolCallNackConvertedToResult(t *testing.T) {
	router := &mockToolRouter{}
	s := &Server{
		config:  DefaultServerConfig(),
		handler: router,
		metrics: &NoOpMetrics{},
		log:     logr.Discard(),
	}
	c := &Connection{sessionID: "sess-2"}

	msg := ClientMessage{
		Type: MessageTypeToolCallNack,
		ToolCallNack: &ToolCallNackInfo{
			CallID: "call-99",
			Reason: "tool not supported",
		},
	}
	data, _ := json.Marshal(msg)
	s.handleClientMessage(context.Background(), c, data, logr.Discard())

	// NACK should be converted to a SendToolResult call with error
	assert.Len(t, router.resultCalls, 1)
	assert.Equal(t, "sess-2", router.resultCalls[0].sessionID)
	assert.Equal(t, "call-99", router.resultCalls[0].result.CallID)
	assert.Equal(t, "tool not supported", router.resultCalls[0].result.Error)
}

func TestSendBinaryFrame_ClosedConnection(t *testing.T) {
	s := &Server{
		config:  DefaultServerConfig(),
		metrics: &NoOpMetrics{},
		log:     logr.Discard(),
	}

	// A closed connection should return nil (silently drop)
	c := &Connection{closed: true}
	frame := &BinaryFrame{
		Header: BinaryHeader{
			MessageType: BinaryMessageTypeUpload,
			PayloadLen:  4,
		},
		Payload: []byte("test"),
	}
	err := s.sendBinaryFrame(c, frame)
	if err != nil {
		t.Errorf("sendBinaryFrame on closed connection should return nil, got: %v", err)
	}
}
