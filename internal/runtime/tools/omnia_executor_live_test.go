//go:build live
// +build live

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

package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/go-logr/logr"

	pktools "github.com/AltairaLabs/PromptKit/runtime/tools"
)

// TestOmniaExecutor_LiveHTTP calls real public APIs to verify the executor
// returns actual tool results. Run with: go test -tags live -run TestOmniaExecutor_LiveHTTP ./internal/runtime/tools/
func TestOmniaExecutor_LiveHTTP(t *testing.T) {
	executor := NewOmniaExecutor(logr.Discard(), nil)

	// Manually register handlers matching the demo tools config
	executor.handlers["calculator"] = &HandlerEntry{
		Name: "calculator",
		Type: ToolTypeHTTP,
		HTTPConfig: &HTTPCfg{
			Endpoint: "https://api.mathjs.org/v4/",
			Method:   "GET",
		},
		Tool: &ToolDefCfg{
			Name:        "calculate",
			Description: "Evaluate a math expression",
		},
	}
	executor.toolHandlers["calculate"] = "calculator"

	executor.handlers["search-places"] = &HandlerEntry{
		Name: "search-places",
		Type: ToolTypeHTTP,
		HTTPConfig: &HTTPCfg{
			Endpoint: "https://geocoding-api.open-meteo.com/v1/search",
			Method:   "GET",
		},
		Tool: &ToolDefCfg{
			Name:        "search_places",
			Description: "Search for a place",
		},
	}
	executor.toolHandlers["search_places"] = "search-places"

	ctx := context.Background()

	t.Run("calculate", func(t *testing.T) {
		args, _ := json.Marshal(map[string]any{"expr": "7 * 13"})
		desc := &pktools.ToolDescriptor{Name: "calculate"}

		result, err := executor.Execute(ctx, desc, args)
		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		t.Logf("Raw result: %s", string(result))

		// mathjs returns plain text "91" which gets JSON-marshaled
		got := strings.TrimSpace(string(result))
		if got != "91" && got != `"91"` {
			t.Errorf("expected 91, got %s", got)
		}
	})

	t.Run("search_places", func(t *testing.T) {
		args, _ := json.Marshal(map[string]any{"name": "Paris"})
		desc := &pktools.ToolDescriptor{Name: "search_places"}

		result, err := executor.Execute(ctx, desc, args)
		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		t.Logf("Raw result (first 200 bytes): %.200s", string(result))

		// Parse the response and verify it contains Paris coordinates
		var data map[string]any
		if err := json.Unmarshal(result, &data); err != nil {
			t.Fatalf("Failed to parse result: %v", err)
		}

		results, ok := data["results"].([]any)
		if !ok || len(results) == 0 {
			t.Fatalf("Expected results array, got: %v", data)
		}

		first := results[0].(map[string]any)
		name := first["name"].(string)
		lat := first["latitude"].(float64)
		lon := first["longitude"].(float64)

		if name != "Paris" {
			t.Errorf("expected name=Paris, got %s", name)
		}
		if lat < 48.0 || lat > 49.0 {
			t.Errorf("expected latitude ~48.85, got %f", lat)
		}
		if lon < 2.0 || lon > 3.0 {
			t.Errorf("expected longitude ~2.35, got %f", lon)
		}

		t.Logf("Paris: lat=%f, lon=%f", lat, lon)
	})
}
