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

package agent

import (
	"context"
	"fmt"

	"github.com/altairalabs/omnia/internal/facade"
)

// EchoHandler echoes back the input message.
// Useful for testing connectivity and message flow.
type EchoHandler struct{}

// NewEchoHandler creates a new EchoHandler.
func NewEchoHandler() *EchoHandler {
	return &EchoHandler{}
}

// Name returns the handler name for metrics.
func (h *EchoHandler) Name() string {
	return "echo"
}

// HandleMessage echoes back the input message.
func (h *EchoHandler) HandleMessage(
	_ context.Context,
	_ string,
	msg *facade.ClientMessage,
	writer facade.ResponseWriter,
) error {
	response := fmt.Sprintf("Echo: %s", msg.Content)
	return writer.WriteDone(response)
}
