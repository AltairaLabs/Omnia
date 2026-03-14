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
	"fmt"
	"testing"

	"github.com/go-logr/logr"
	"github.com/gorilla/websocket"
)

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
