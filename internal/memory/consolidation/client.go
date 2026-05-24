/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
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
)

// Client posts FunctionInput to a function-mode AgentRuntime's
// /functions/{name} endpoint and decodes the action list response.
type Client struct {
	baseURL string
	timeout time.Duration
	http    *http.Client
}

// NewClient constructs a Client. baseURL is the AgentRuntime service URL
// (without /functions/...); timeout caps a single Call.
func NewClient(baseURL string, timeout time.Duration) *Client {
	return &Client{
		baseURL: baseURL,
		timeout: timeout,
		http:    &http.Client{Timeout: timeout},
	}
}

// Call posts the input to /functions/{name} and decodes the action
// list. Returns the decoded actions or an error.
func (c *Client) Call(ctx context.Context, functionName string, in FunctionInput) ([]Action, error) {
	body, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("marshal input: %w", err)
	}
	url := c.baseURL + "/functions/" + functionName

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
