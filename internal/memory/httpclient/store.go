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

// Package httpclient provides a pkmemory.Store implementation backed by HTTP
// calls to the memory-api service.
package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"

	pkmemory "github.com/AltairaLabs/PromptKit/runtime/memory"
	"github.com/altairalabs/omnia/pkg/policy"
)

// Default timeout for HTTP requests to the memory-api.
const defaultHTTPTimeout = 30 * time.Second

// memoryListResponse is the JSON response for memory list/search endpoints.
type memoryListResponse struct {
	Memories []*pkmemory.Memory `json:"memories"`
	Total    int                `json:"total"`
}

// memoryResponse is the JSON response for a single memory creation.
type memoryResponse struct {
	Memory *pkmemory.Memory `json:"memory"`
}

// errorResponse is the JSON response for errors.
type errorResponse struct {
	Error string `json:"error"`
}

// Store implements pkmemory.Store by calling the memory-api over HTTP.
type Store struct {
	baseURL    string
	httpClient *http.Client
	log        logr.Logger
}

// NewStore creates a new HTTP-backed memory store.
func NewStore(baseURL string, log logr.Logger) *Store {
	return &Store{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: defaultHTTPTimeout,
			Transport: &http.Transport{
				MaxIdleConns:        20,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		log: log.WithName("memory-httpclient"),
	}
}

// Save persists a memory via POST /api/v1/memories.
func (s *Store) Save(ctx context.Context, mem *pkmemory.Memory) error {
	body, err := json.Marshal(mem)
	if err != nil {
		return fmt.Errorf("memory httpclient: save: encode: %w", err)
	}

	resp, err := s.doRequest(ctx, http.MethodPost, "/api/v1/memories", body)
	if err != nil {
		return fmt.Errorf("memory httpclient: save: %w", err)
	}
	defer drainAndClose(resp.Body)

	if resp.StatusCode != http.StatusCreated {
		return s.readError("save", resp)
	}

	var mr memoryResponse
	if err := json.NewDecoder(resp.Body).Decode(&mr); err != nil {
		return fmt.Errorf("memory httpclient: save: decode response: %w", err)
	}

	// Populate ID and CreatedAt on the input memory (same as PostgresMemoryStore).
	if mr.Memory != nil {
		mem.ID = mr.Memory.ID
		mem.CreatedAt = mr.Memory.CreatedAt
	}

	return nil
}

// Retrieve searches memories via GET /api/v1/memories/search.
func (s *Store) Retrieve(ctx context.Context, scope map[string]string, query string, opts pkmemory.RetrieveOptions) ([]*pkmemory.Memory, error) {
	params := scopeParams(scope)
	params.Set("q", query)
	if opts.Limit > 0 {
		params.Set("limit", strconv.Itoa(opts.Limit))
	}
	if opts.MinConfidence > 0 {
		params.Set("min_confidence", strconv.FormatFloat(opts.MinConfidence, 'f', -1, 64))
	}
	addTypeParams(params, opts.Types)

	resp, err := s.doRequest(ctx, http.MethodGet, "/api/v1/memories/search?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("memory httpclient: retrieve: %w", err)
	}
	defer drainAndClose(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, s.readError("retrieve", resp)
	}

	var lr memoryListResponse
	if err := json.NewDecoder(resp.Body).Decode(&lr); err != nil {
		return nil, fmt.Errorf("memory httpclient: retrieve: decode: %w", err)
	}

	if lr.Memories == nil {
		lr.Memories = []*pkmemory.Memory{}
	}
	return lr.Memories, nil
}

// List returns memories via GET /api/v1/memories.
func (s *Store) List(ctx context.Context, scope map[string]string, opts pkmemory.ListOptions) ([]*pkmemory.Memory, error) {
	params := scopeParams(scope)
	if opts.Limit > 0 {
		params.Set("limit", strconv.Itoa(opts.Limit))
	}
	if opts.Offset > 0 {
		params.Set("offset", strconv.Itoa(opts.Offset))
	}
	addTypeParams(params, opts.Types)

	resp, err := s.doRequest(ctx, http.MethodGet, "/api/v1/memories?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("memory httpclient: list: %w", err)
	}
	defer drainAndClose(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, s.readError("list", resp)
	}

	var lr memoryListResponse
	if err := json.NewDecoder(resp.Body).Decode(&lr); err != nil {
		return nil, fmt.Errorf("memory httpclient: list: decode: %w", err)
	}

	if lr.Memories == nil {
		lr.Memories = []*pkmemory.Memory{}
	}
	return lr.Memories, nil
}

// Delete soft-deletes a memory via DELETE /api/v1/memories/{id}.
func (s *Store) Delete(ctx context.Context, scope map[string]string, memoryID string) error {
	params := scopeParams(scope)
	path := fmt.Sprintf("/api/v1/memories/%s?%s", memoryID, params.Encode())

	resp, err := s.doRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return fmt.Errorf("memory httpclient: delete: %w", err)
	}
	defer drainAndClose(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return s.readError("delete", resp)
	}
	return nil
}

// DeleteAll deletes all memories for a scope via DELETE /api/v1/memories.
func (s *Store) DeleteAll(ctx context.Context, scope map[string]string) error {
	params := scopeParams(scope)
	path := "/api/v1/memories?" + params.Encode()

	resp, err := s.doRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return fmt.Errorf("memory httpclient: delete all: %w", err)
	}
	defer drainAndClose(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return s.readError("delete all", resp)
	}
	return nil
}

// --- HTTP helpers ---

func (s *Store) doRequest(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	reqURL := s.baseURL + path
	var req *http.Request
	var err error
	if body != nil {
		req, err = http.NewRequestWithContext(ctx, method, reqURL, bytes.NewReader(body))
	} else {
		req, err = http.NewRequestWithContext(ctx, method, reqURL, nil)
	}
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	// Forward consent grants if present in context.
	if grants := policy.ConsentGrantsFromContext(ctx); len(grants) > 0 {
		req.Header.Set("X-Consent-Grants", strings.Join(grants, ","))
	}

	s.log.V(2).Info("memory-api request", "method", method, "path", path)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.log.V(1).Info("memory-api request failed", "method", method, "path", path, "error", err.Error())
		return nil, err
	}

	s.log.V(2).Info("memory-api response", "method", method, "path", path, "status", resp.StatusCode)
	return resp, nil
}

func (s *Store) readError(op string, resp *http.Response) error {
	var errResp errorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		return fmt.Errorf("memory httpclient: %s: HTTP %d", op, resp.StatusCode)
	}
	return fmt.Errorf("memory httpclient: %s: HTTP %d: %s", op, resp.StatusCode, errResp.Error)
}

// scopeParams converts a scope map to URL query parameters.
func scopeParams(scope map[string]string) url.Values {
	params := url.Values{}
	if ws := scope["workspace_id"]; ws != "" {
		params.Set("workspace", ws)
	}
	if uid := scope["user_id"]; uid != "" {
		params.Set("user_id", uid)
	}
	if agent := scope["agent_id"]; agent != "" {
		params.Set("agent", agent)
	}
	return params
}

// addTypeParams appends type filter parameters.
func addTypeParams(params url.Values, types []string) {
	if len(types) > 0 {
		for _, t := range types {
			params.Add("type", t)
		}
	}
}

// drainAndClose reads remaining body bytes and closes it.
func drainAndClose(body io.ReadCloser) {
	_, _ = io.Copy(io.Discard, body)
	_ = body.Close()
}

// Interface assertion.
var _ pkmemory.Store = (*Store)(nil)
