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

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newAdminHandler(rec DimensionConsentRecorder) *Handler {
	h := NewHandler(nil, logr.Discard())
	if rec != nil {
		h = h.WithDimensionConsentRecorder(rec)
	}
	return h
}

func postDimChange(h *Handler, body string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/admin/embedding-dimension-change", strings.NewReader(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.handleEmbeddingDimensionChange(w, req)
	return w
}

func TestEmbeddingDimensionChange_RecordsConsent(t *testing.T) {
	var gotDim int
	var gotBy string
	h := newAdminHandler(func(_ context.Context, dim int, by string) error {
		gotDim, gotBy = dim, by
		return nil
	})

	w := postDimChange(h, `{"target_dim":768}`, nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 768, gotDim)
	assert.Equal(t, "operator", gotBy, "no caller header => operator")

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(768), resp["target_dim"])
}

func TestEmbeddingDimensionChange_HashesCaller(t *testing.T) {
	var gotBy string
	h := newAdminHandler(func(_ context.Context, _ int, by string) error {
		gotBy = by
		return nil
	})

	w := postDimChange(h, `{"target_dim":768}`, map[string]string{"x-omnia-user-id": "alice"})
	require.Equal(t, http.StatusOK, w.Code)
	assert.NotEmpty(t, gotBy)
	assert.NotEqual(t, "operator", gotBy)
	assert.NotEqual(t, "alice", gotBy, "raw caller id must be hashed before storage")
}

func TestEmbeddingDimensionChange_Unavailable(t *testing.T) {
	w := postDimChange(newAdminHandler(nil), `{"target_dim":768}`, nil)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestEmbeddingDimensionChange_BadBody(t *testing.T) {
	called := false
	h := newAdminHandler(func(context.Context, int, string) error { called = true; return nil })
	w := postDimChange(h, `not json`, nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.False(t, called)
}

func TestEmbeddingDimensionChange_InvalidDim(t *testing.T) {
	called := false
	h := newAdminHandler(func(context.Context, int, string) error { called = true; return nil })
	for _, body := range []string{`{"target_dim":0}`, `{"target_dim":-5}`, `{"target_dim":5000}`} {
		w := postDimChange(h, body, nil)
		assert.Equal(t, http.StatusBadRequest, w.Code, "body=%s", body)
	}
	assert.False(t, called, "recorder must not be called on an invalid dimension")
}

func TestEmbeddingDimensionChange_RecorderError(t *testing.T) {
	h := newAdminHandler(func(context.Context, int, string) error { return assert.AnError })
	w := postDimChange(h, `{"target_dim":768}`, nil)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
