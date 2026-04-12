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

	// Apply field redaction if configured.
	if len(cfg.Redact) > 0 {
		body = redactResponseFields(body, cfg.Redact)
	}

	return body, callResult, nil
}

// httpRequestParams holds the intermediate state built up while processing
// HTTPCfg fields, before the final *http.Request is created.
type httpRequestParams struct {
	method       string
	url          string
	body         io.Reader
	extraHeaders map[string]string
}

// buildHTTPRequest constructs an *http.Request from the tool configuration and
// caller-supplied headers and arguments.
func buildHTTPRequest(ctx context.Context, cfg *HTTPCfg, headers map[string]string, args json.RawMessage) (*http.Request, error) {
	method := strings.ToUpper(cfg.Method)
	if method == "" {
		method = http.MethodPost
	}

	hasArgs := len(args) > 0 && string(args) != "null" && string(args) != "{}"
	needsParsedArgs := hasArgs && hasAdvancedHTTPConfig(cfg)

	params, err := resolveHTTPParams(cfg, method, hasArgs, needsParsedArgs, args)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, method, params.url, params.body)
	if err != nil {
		return nil, fmt.Errorf("creating HTTP request: %w", err)
	}

	if params.body != nil {
		contentType := cfg.ContentType
		if contentType == "" {
			contentType = "application/json"
		}
		req.Header.Set("Content-Type", contentType)
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}
	for k, v := range params.extraHeaders {
		req.Header.Set(k, v)
	}

	return req, nil
}

// hasAdvancedHTTPConfig returns true if cfg uses any fields that require parsed args.
func hasAdvancedHTTPConfig(cfg *HTTPCfg) bool {
	return cfg.URLTemplate != "" || len(cfg.HeaderParams) > 0 ||
		len(cfg.QueryParams) > 0 || cfg.StaticBody != nil
}

// resolveHTTPParams processes all HTTPCfg fields (URL template, header params,
// query params, static query, static body) and returns the resolved request params.
func resolveHTTPParams(cfg *HTTPCfg, method string, hasArgs, needsParsedArgs bool, args json.RawMessage) (*httpRequestParams, error) {
	p := &httpRequestParams{method: method, url: cfg.Endpoint}

	var argsMap map[string]any
	if needsParsedArgs {
		if err := json.Unmarshal(args, &argsMap); err != nil {
			return nil, fmt.Errorf("unmarshalling args for advanced config: %w", err)
		}
	}

	// 1. URL template resolution.
	if cfg.URLTemplate != "" {
		p.url = cfg.URLTemplate
		if argsMap != nil {
			p.url, argsMap = resolveURLTemplate(p.url, argsMap)
		}
	}

	// 2. Header params extraction.
	if len(cfg.HeaderParams) > 0 && argsMap != nil {
		p.extraHeaders, argsMap = extractHeaderParams(cfg.HeaderParams, argsMap)
	}

	// 3. Static query params.
	if len(cfg.StaticQuery) > 0 {
		resolved, err := applyStaticQuery(p.url, cfg.StaticQuery)
		if err != nil {
			return nil, err
		}
		p.url = resolved
	}

	// 4. Body and query params based on method.
	var err error
	if needsParsedArgs {
		p.url, p.body, err = buildAdvancedBodyAndQuery(cfg, method, p.url, argsMap)
	} else {
		p.url, p.body, err = buildSimpleBodyAndQuery(cfg, method, p.url, hasArgs, args)
	}
	if err != nil {
		return nil, err
	}

	return p, nil
}

// applyStaticQuery appends static query params to a URL.
func applyStaticQuery(rawURL string, staticQuery map[string]string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL, fmt.Errorf("parsing URL for static query: %w", err)
	}
	q := u.Query()
	for k, v := range staticQuery {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// buildAdvancedBodyAndQuery builds body and query params when the args map has
// been parsed for advanced features (URLTemplate, HeaderParams, QueryParams, StaticBody).
func buildAdvancedBodyAndQuery(cfg *HTTPCfg, method, reqURL string, argsMap map[string]any) (string, io.Reader, error) {
	if isHTTPBodyMethod(method) {
		return buildAdvancedBodyMethod(cfg, reqURL, argsMap)
	}
	return buildAdvancedNonBodyMethod(cfg, reqURL, argsMap)
}

// buildAdvancedBodyMethod handles POST/PUT/PATCH with parsed args.
func buildAdvancedBodyMethod(cfg *HTTPCfg, reqURL string, argsMap map[string]any) (string, io.Reader, error) {
	if len(cfg.QueryParams) > 0 {
		var queryFields map[string]any
		queryFields, argsMap = extractQueryParams(cfg.QueryParams, argsMap)
		if len(queryFields) > 0 {
			var err error
			reqURL, err = appendQueryFromMap(reqURL, queryFields)
			if err != nil {
				return reqURL, nil, fmt.Errorf("building query params: %w", err)
			}
		}
	}
	merged := mergeStaticBody(cfg.StaticBody, argsMap)
	bodyBytes, err := json.Marshal(merged)
	if err != nil {
		return reqURL, nil, fmt.Errorf("marshalling body: %w", err)
	}
	return reqURL, bytes.NewReader(bodyBytes), nil
}

// buildAdvancedNonBodyMethod handles GET/DELETE/etc with parsed args.
func buildAdvancedNonBodyMethod(cfg *HTTPCfg, reqURL string, argsMap map[string]any) (string, io.Reader, error) {
	var queryMap map[string]any
	if len(cfg.QueryParams) > 0 {
		queryMap, _ = extractQueryParams(cfg.QueryParams, argsMap)
	} else {
		queryMap = argsMap
	}
	if len(queryMap) > 0 {
		var err error
		reqURL, err = appendQueryFromMap(reqURL, queryMap)
		if err != nil {
			return reqURL, nil, fmt.Errorf("building query params: %w", err)
		}
	}
	return reqURL, nil, nil
}

// buildSimpleBodyAndQuery handles the simple path (no advanced features).
func buildSimpleBodyAndQuery(cfg *HTTPCfg, method, reqURL string, hasArgs bool, args json.RawMessage) (string, io.Reader, error) {
	if hasArgs {
		if isHTTPBodyMethod(method) {
			return reqURL, bytes.NewReader(args), nil
		}
		resolved, err := appendQueryFromJSON(reqURL, args)
		if err != nil {
			return reqURL, nil, fmt.Errorf("building query params: %w", err)
		}
		return resolved, nil, nil
	}
	if isHTTPBodyMethod(method) && cfg.StaticBody != nil {
		bodyBytes, err := json.Marshal(cfg.StaticBody)
		if err != nil {
			return reqURL, nil, fmt.Errorf("marshalling static body: %w", err)
		}
		return reqURL, bytes.NewReader(bodyBytes), nil
	}
	return reqURL, nil, nil
}

// resolveURLTemplate replaces {param} placeholders in the template with values
// from args, returning the resolved URL and the remaining args (with consumed
// keys removed).
func resolveURLTemplate(tmpl string, args map[string]any) (string, map[string]any) {
	remaining := make(map[string]any, len(args))
	for k, v := range args {
		remaining[k] = v
	}
	for key, val := range args {
		placeholder := "{" + key + "}"
		if strings.Contains(tmpl, placeholder) {
			tmpl = strings.ReplaceAll(tmpl, placeholder, fmt.Sprintf("%v", val))
			delete(remaining, key)
		}
	}
	return tmpl, remaining
}

// extractHeaderParams extracts arg fields listed in headerParams, returning the
// header map (argField→headerName→value) and the remaining args.
func extractHeaderParams(headerParams map[string]string, args map[string]any) (map[string]string, map[string]any) {
	remaining := make(map[string]any, len(args))
	for k, v := range args {
		remaining[k] = v
	}
	extracted := make(map[string]string, len(headerParams))
	for argField, headerName := range headerParams {
		if val, ok := remaining[argField]; ok {
			extracted[headerName] = fmt.Sprintf("%v", val)
			delete(remaining, argField)
		}
	}
	return extracted, remaining
}

// extractQueryParams extracts the named fields from args, returning the extracted
// fields and the remaining args.
func extractQueryParams(paramNames []string, args map[string]any) (map[string]any, map[string]any) {
	remaining := make(map[string]any, len(args))
	for k, v := range args {
		remaining[k] = v
	}
	extracted := make(map[string]any, len(paramNames))
	for _, name := range paramNames {
		if val, ok := remaining[name]; ok {
			extracted[name] = val
			delete(remaining, name)
		}
	}
	return extracted, remaining
}

// mergeStaticBody merges a static body with args, where args take precedence.
// If staticBody is nil, returns args. Both are expected to be map[string]any or
// convertible to one.
func mergeStaticBody(staticBody interface{}, args map[string]any) map[string]any {
	if staticBody == nil {
		return args
	}
	// Convert staticBody to map[string]any.
	var base map[string]any
	switch sb := staticBody.(type) {
	case map[string]any:
		base = make(map[string]any, len(sb))
		for k, v := range sb {
			base[k] = v
		}
	default:
		// Try JSON round-trip for other types.
		data, err := json.Marshal(staticBody)
		if err != nil {
			return args
		}
		if err := json.Unmarshal(data, &base); err != nil {
			return args
		}
	}
	// Args override static body.
	for k, v := range args {
		base[k] = v
	}
	return base
}

// appendQueryFromMap appends key-value pairs from a map as query params to a URL.
func appendQueryFromMap(baseURL string, params map[string]any) (string, error) {
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

// redactResponseFields replaces the values of the named fields with "[REDACTED]"
// in a JSON response body. If the body is not a JSON object, it is returned as-is.
func redactResponseFields(body json.RawMessage, fields []string) json.RawMessage {
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		return body
	}
	for _, field := range fields {
		if _, ok := obj[field]; ok {
			obj[field] = "[REDACTED]"
		}
	}
	result, err := json.Marshal(obj)
	if err != nil {
		return body
	}
	return result
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
