/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package consolidation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	memoryv1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// Client posts FunctionInput to a function-mode AgentRuntime's
// /functions/{name} endpoint and decodes the action list response.
//
// URLs are resolved per ref from the Kubernetes Service DNS:
//
//	http://{ref.name}.{ref.namespace}.svc.cluster.local:8080/functions/{ref.name}
//
// (the operator creates one Service per function-mode AgentRuntime that
// fronts the facade port 8080). This avoids the one-global-URL constraint
// the v1 shipped with, where every pack had to live in one namespace.
type Client struct {
	timeout      time.Duration
	hostOverride string // empty in production; httptest server URL in tests
	http         *http.Client
}

// NewClient constructs a Client. timeout caps a single Call.
func NewClient(timeout time.Duration) *Client {
	return &Client{
		timeout: timeout,
		http:    &http.Client{Timeout: timeout},
	}
}

// WithBaseHostOverride routes Call to a fixed base URL instead of the
// Kubernetes Service DNS. Exists for tests against httptest.Server;
// production code never calls it.
func (c *Client) WithBaseHostOverride(baseURL string) *Client {
	c.hostOverride = baseURL
	return c
}

// Call posts the input to /functions/{ref.name} and decodes the action
// list. Returns the decoded actions or an error.
func (c *Client) Call(ctx context.Context, ref memoryv1.MemoryFunctionRef, in FunctionInput) ([]Action, error) {
	if c.hostOverride == "" && ref.Namespace == "" {
		return nil, fmt.Errorf("MemoryFunctionRef.Namespace required (no global fallback)")
	}
	body, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("marshal input: %w", err)
	}
	url := c.urlFor(ref)

	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(callCtx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http call: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("function returned %d: %s", resp.StatusCode, string(respBody))
	}
	return UnmarshalActions(respBody)
}

func (c *Client) urlFor(ref memoryv1.MemoryFunctionRef) string {
	if c.hostOverride != "" {
		return c.hostOverride + "/functions/" + ref.Name
	}
	return fmt.Sprintf("http://%s.%s.svc.cluster.local:8080/functions/%s",
		ref.Name, ref.Namespace, ref.Name)
}
