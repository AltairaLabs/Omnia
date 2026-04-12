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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

const httpBodyMaxBytes = 10 * 1024 * 1024 // 10 MB
const truncateErrorBodyLen = 512

// doHTTPRequest performs a direct HTTP call using the provided client, returning
// the response body (as JSON), the raw call result for retry classification, and
// any error.
//
// On transport errors (network failures, DNS errors) the returned httpCallResult
// has Err set and StatusCode is 0.  On HTTP-level errors (non-2xx) Err is nil
// and StatusCode is set.
func doHTTPRequest(
	ctx context.Context,
	client *http.Client,
	cfg *HTTPCfg,
	headers map[string]string,
	args json.RawMessage,
) (json.RawMessage, httpCallResult, error) {
	req, err := buildHTTPRequest(ctx, cfg, headers, args)
	if err != nil {
		return nil, httpCallResult{Err: err}, err
	}

	// Inject OTel trace context so downstream services can participate in the trace.
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))

	resp, err := client.Do(req)
	if err != nil {
		return nil, httpCallResult{Err: err}, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, httpBodyMaxBytes))
	if err != nil {
		callResult := httpCallResult{StatusCode: resp.StatusCode, Headers: resp.Header}
		return nil, callResult, fmt.Errorf("reading response body: %w", err)
	}

	callResult := httpCallResult{StatusCode: resp.StatusCode, Headers: resp.Header}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, callResult, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncateBody(body, truncateErrorBodyLen))
	}

	if !json.Valid(body) {
		wrapped, err := json.Marshal(map[string]string{"result": string(body)})
		if err != nil {
			return nil, callResult, fmt.Errorf("wrapping non-JSON response: %w", err)
		}
		return wrapped, callResult, nil
	}

	return body, callResult, nil
}

// buildHTTPRequest constructs an *http.Request from the tool configuration and
// caller-supplied headers and arguments.
func buildHTTPRequest(ctx context.Context, cfg *HTTPCfg, headers map[string]string, args json.RawMessage) (*http.Request, error) {
	method := strings.ToUpper(cfg.Method)
	if method == "" {
		method = http.MethodPost
	}

	hasArgs := len(args) > 0 && string(args) != "null" && string(args) != "{}"

	var (
		body   io.Reader
		reqURL = cfg.Endpoint
	)

	if hasArgs {
		if isHTTPBodyMethod(method) {
			body = bytes.NewReader(args)
		} else {
			var err error
			reqURL, err = appendQueryFromJSON(cfg.Endpoint, args)
			if err != nil {
				return nil, fmt.Errorf("building query params: %w", err)
			}
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL, body)
	if err != nil {
		return nil, fmt.Errorf("creating HTTP request: %w", err)
	}

	if body != nil {
		contentType := cfg.ContentType
		if contentType == "" {
			contentType = "application/json"
		}
		req.Header.Set("Content-Type", contentType)
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	return req, nil
}

// isHTTPBodyMethod returns true for HTTP methods that carry a request body.
func isHTTPBodyMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch:
		return true
	default:
		return false
	}
}

// appendQueryFromJSON unmarshals args as a JSON object and appends each field
// as a query parameter to baseURL.
func appendQueryFromJSON(baseURL string, args json.RawMessage) (string, error) {
	var params map[string]any
	if err := json.Unmarshal(args, &params); err != nil {
		return baseURL, fmt.Errorf("unmarshalling query args: %w", err)
	}

	u, err := url.Parse(baseURL)
	if err != nil {
		return baseURL, fmt.Errorf("parsing base URL: %w", err)
	}

	q := u.Query()
	for k, v := range params {
		q.Set(k, fmt.Sprintf("%v", v))
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// truncateBody returns the body as a string, truncating to maxLen bytes with an
// ellipsis when the body exceeds maxLen.
func truncateBody(body []byte, maxLen int) string {
	if len(body) <= maxLen {
		return string(body)
	}
	return string(body[:maxLen]) + "..."
}
