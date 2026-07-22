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
	pkmemory "github.com/AltairaLabs/PromptKit/runtime/memory"
	"github.com/AltairaLabs/PromptKit/sdk"
	v1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// WithMemoryStore sets the memory store for cross-session memory.
// The store is typically an HTTP client backed by memory-api.
func WithMemoryStore(store pkmemory.Store) ServerOption {
	return func(s *Server) {
		s.memoryStore = store
	}
}

// WithWorkspaceUID sets the workspace UID for memory scope.
func WithWorkspaceUID(uid string) ServerOption {
	return func(s *Server) {
		s.workspaceUID = uid
	}
}

// WithMemoryRetrieval configures the retrieval strategy, access deny-filter,
// and episodic limit (from spec.memory.retrieval). When strategy is "semantic"
// and the memory store supports it, per-turn retrieval uses semantic hybrid
// search with the deny-filter; otherwise keyword FTS. limit 0 falls back to
// defaultEpisodicLimit (10).
func WithMemoryRetrieval(strategy, denyCEL string, limit int) ServerOption {
	return func(s *Server) {
		s.memoryStrategy = strategy
		s.memoryDenyCEL = denyCEL
		s.memoryLimit = limit
	}
}

// WithMemoryModes sets the two independent memory axes from
// spec.memory.retrieval.enabled and spec.memory.tools.enabled. retrievalEnabled
// gates ambient RAG auto-injection; toolsEnabled gates the memory__remember /
// memory__recall tools. Both default true (see NewServer) when not set.
func WithMemoryModes(retrievalEnabled, toolsEnabled bool) ServerOption {
	return func(s *Server) {
		s.memoryRetrievalEnabled = retrievalEnabled
		s.memoryToolsEnabled = toolsEnabled
	}
}

// WithMediaBasePath sets the base path for resolving mock:// URLs.
func WithMediaBasePath(path string) ServerOption {
	return func(s *Server) {
		if path != "" {
			s.mediaResolver = NewMediaResolver(path)
		}
	}
}

// HasMediaResolver reports whether a media resolver has been wired into the
// server via WithMediaBasePath. Used by wiring tests in cmd/runtime to assert
// that cmd/runtime/main.go forwards cfg.MediaBasePath to the server (without
// which mock:// and file:// URL resolution in media chunks silently fails).
func (s *Server) HasMediaResolver() bool {
	return s.mediaResolver != nil
}

// WithContextWindow sets the token budget for conversation context.
// When set, PromptKit automatically truncates older messages when the budget is exceeded.
func WithContextWindow(tokens int) ServerOption {
	return func(s *Server) {
		if tokens > 0 {
			s.sdkOptions = append(s.sdkOptions, sdk.WithTokenBudget(tokens))
		}
	}
}

// WithTruncationStrategy sets the strategy for handling context overflow.
// Valid values: "sliding" (remove oldest), "summarize" (summarize before
// removing), "custom" (the runtime implements truncation itself — no SDK
// truncation is configured). "custom" is intended for custom runtimes
// (spec.framework.type: custom); on this PromptKit runtime it means no
// truncation is applied at all, which cmd/runtime warns about at startup.
func WithTruncationStrategy(strategy string) ServerOption {
	return func(s *Server) {
		// "custom" means the custom runtime handles it - don't set SDK truncation
		if strategy != "" && strategy != string(v1alpha1.TruncationStrategyCustom) {
			s.sdkOptions = append(s.sdkOptions, sdk.WithTruncation(strategy))
		}
	}
}
