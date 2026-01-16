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
	"time"
)

// sendMessage sends a server message to a connection.
func (s *Server) sendMessage(c *Connection, msg *ServerMessage) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}

	if err := c.conn.SetWriteDeadline(time.Now().Add(s.config.WriteTimeout)); err != nil {
		return err
	}

	if err := c.conn.WriteJSON(msg); err != nil {
		return err
	}

	// Record message sent
	s.metrics.MessageSent()
	return nil
}

// sendError sends an error message to a connection.
func (s *Server) sendError(c *Connection, sessionID, code, message string) {
	if err := s.sendMessage(c, NewErrorMessage(sessionID, code, message)); err != nil {
		s.log.Error(err, "failed to send error message")
	}
}

// sendConnected sends a connected message to a connection.
func (s *Server) sendConnected(c *Connection, sessionID string) error {
	// Always send capabilities so clients know the max payload size
	// for deciding when to use the upload mechanism
	return s.sendMessage(c, NewConnectedMessageWithCapabilities(sessionID, &ConnectionCapabilities{
		BinaryFrames:    c.binaryCapable,
		MaxPayloadSize:  int(s.config.MaxMessageSize),
		ProtocolVersion: BinaryVersion,
	}))
}
