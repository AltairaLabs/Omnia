/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0

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

package httpclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetrieveSemantic_PostsBodyAndDecodes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/v1/memories/retrieve/semantic", r.URL.Path)
		b, _ := io.ReadAll(r.Body)
		assert.Contains(t, string(b), `"deny_cel"`)
		assert.Contains(t, string(b), `"workspace_id":"ws-1"`)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"memories": []map[string]any{{"id": "a", "content": "allowed"}},
			"total":    1,
		})
	}))
	defer srv.Close()

	s := NewStore(srv.URL, logr.Discard())
	got, err := s.RetrieveSemantic(context.Background(), "ws-1", "failover",
		`metadata.url.contains("restricted")`, 5)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "a", got[0].ID)
}

func TestRetrieveSemantic_Non200Errors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"boom"}`))
	}))
	defer srv.Close()

	s := NewStore(srv.URL, logr.Discard())
	_, err := s.RetrieveSemantic(context.Background(), "ws-1", "q", "", 5)
	require.Error(t, err)
}
