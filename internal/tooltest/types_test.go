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

package tooltest

import (
	"encoding/json"
	"testing"
)

func TestTestRequestSerialization(t *testing.T) {
	req := TestRequest{
		HandlerName: "my-handler",
		ToolName:    "my-tool",
		Arguments:   json.RawMessage(`{"query":"hello"}`),
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal TestRequest: %v", err)
	}

	var decoded TestRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal TestRequest: %v", err)
	}

	if decoded.HandlerName != "my-handler" {
		t.Errorf("HandlerName = %q, want %q", decoded.HandlerName, "my-handler")
	}
	if decoded.ToolName != "my-tool" {
		t.Errorf("ToolName = %q, want %q", decoded.ToolName, "my-tool")
	}
	if string(decoded.Arguments) != `{"query":"hello"}` {
		t.Errorf("Arguments = %q, want %q", string(decoded.Arguments), `{"query":"hello"}`)
	}
}

func TestTestRequestWithoutOptionalFields(t *testing.T) {
	input := `{"handlerName":"h1","arguments":{}}`
	var req TestRequest
	if err := json.Unmarshal([]byte(input), &req); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if req.HandlerName != "h1" {
		t.Errorf("HandlerName = %q, want %q", req.HandlerName, "h1")
	}
	if req.ToolName != "" {
		t.Errorf("ToolName = %q, want empty", req.ToolName)
	}
}

func TestTestResponseSerialization(t *testing.T) {
	resp := TestResponse{
		Success:     true,
		Result:      json.RawMessage(`{"data":"ok"}`),
		DurationMs:  42,
		HandlerType: "http",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal TestResponse: %v", err)
	}

	var decoded TestResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal TestResponse: %v", err)
	}

	if !decoded.Success {
		t.Error("Success = false, want true")
	}
	if decoded.DurationMs != 42 {
		t.Errorf("DurationMs = %d, want 42", decoded.DurationMs)
	}
	if decoded.HandlerType != "http" {
		t.Errorf("HandlerType = %q, want %q", decoded.HandlerType, "http")
	}
}

func TestTestResponseError(t *testing.T) {
	resp := TestResponse{
		Success:     false,
		Error:       "connection refused",
		DurationMs:  5,
		HandlerType: "grpc",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded TestResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.Success {
		t.Error("Success = true, want false")
	}
	if decoded.Error != "connection refused" {
		t.Errorf("Error = %q, want %q", decoded.Error, "connection refused")
	}
	if decoded.Result != nil {
		t.Errorf("Result = %s, want nil", string(decoded.Result))
	}
}
