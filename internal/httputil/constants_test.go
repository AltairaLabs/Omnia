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

package httputil

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestConstants(t *testing.T) {
	if HeaderContentType != "Content-Type" {
		t.Errorf("expected HeaderContentType to be %q, got %q", "Content-Type", HeaderContentType)
	}
	if ContentTypeJSON != "application/json" {
		t.Errorf("expected ContentTypeJSON to be %q, got %q", "application/json", ContentTypeJSON)
	}
}

func TestWriteJSON_Success(t *testing.T) {
	w := httptest.NewRecorder()

	payload := map[string]string{"key": "value"}
	err := WriteJSON(w, http.StatusOK, payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	ct := w.Header().Get("Content-Type")
	if ct != ContentTypeJSON {
		t.Errorf("expected Content-Type %q, got %q", ContentTypeJSON, ct)
	}

	var result map[string]string
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if result["key"] != "value" {
		t.Errorf("expected key=value, got key=%s", result["key"])
	}
}

func TestWriteJSON_CustomStatusCode(t *testing.T) {
	w := httptest.NewRecorder()

	payload := map[string]string{"error": "not found"}
	err := WriteJSON(w, http.StatusNotFound, payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestWriteJSON_NilPayload(t *testing.T) {
	w := httptest.NewRecorder()

	err := WriteJSON(w, http.StatusNoContent, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if w.Code != http.StatusNoContent {
		t.Errorf("expected status %d, got %d", http.StatusNoContent, w.Code)
	}
}

func TestWriteJSON_Struct(t *testing.T) {
	type response struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}

	w := httptest.NewRecorder()

	err := WriteJSON(w, http.StatusCreated, response{Name: "test", Count: 42})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if w.Code != http.StatusCreated {
		t.Errorf("expected status %d, got %d", http.StatusCreated, w.Code)
	}

	var result response
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if result.Name != "test" || result.Count != 42 {
		t.Errorf("expected {test, 42}, got {%s, %d}", result.Name, result.Count)
	}
}

func TestWriteJSON_UnmarshalableValue(t *testing.T) {
	w := httptest.NewRecorder()

	// channels cannot be marshalled to JSON
	err := WriteJSON(w, http.StatusOK, make(chan int))
	if err == nil {
		t.Fatal("expected error for unmarshalable value")
	}
}
