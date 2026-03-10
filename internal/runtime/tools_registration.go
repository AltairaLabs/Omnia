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

package runtime

import (
	"context"

	"github.com/AltairaLabs/PromptKit/sdk"

	pktools "github.com/AltairaLabs/PromptKit/runtime/tools"

	"github.com/altairalabs/omnia/pkg/logctx"
)

// registerToolsWithConversation registers the OmniaExecutor with a conversation's
// tool registry. Pack-defined tools get their Mode updated to "omnia" so the
// registry dispatches them to our executor. Discovered tools (MCP, OpenAPI,
// gRPC) that are not in the pack are registered as new descriptors.
func (s *Server) registerToolsWithConversation(ctx context.Context, conv *sdk.Conversation) error {
	log := logctx.LoggerWithContext(s.log, ctx)

	registry := conv.ToolRegistry()

	// Register the OmniaExecutor so the registry can dispatch "omnia" mode tools.
	registry.RegisterExecutor(s.toolExecutor)

	toolNames := s.toolExecutor.ToolNames()

	// Build a lookup map from discovered tool descriptors for O(1) access.
	descriptorsByName := make(map[string]*pktools.ToolDescriptor, len(s.toolExecutor.ToolDescriptors()))
	for _, d := range s.toolExecutor.ToolDescriptors() {
		descriptorsByName[d.Name] = d
	}

	var updated, registered int
	for _, name := range toolNames {
		if desc := registry.Get(name); desc != nil {
			// Tool already exists in the pack — update its mode so the
			// registry dispatches it through our executor instead of the
			// default local executor.
			desc.Mode = s.toolExecutor.Name()
			updated++
			continue
		}
		// Tool discovered from backend (MCP, OpenAPI, gRPC ListTools)
		// but not declared in the pack — register it.
		d, ok := descriptorsByName[name]
		if !ok {
			continue
		}
		if err := registry.Register(d); err != nil {
			log.Error(err, "failed to register discovered tool", "tool", name)
		} else {
			registered++
		}
	}

	log.Info("tools registered with conversation",
		"updated", updated,
		"registered", registered,
		"total", len(toolNames))
	return nil
}
