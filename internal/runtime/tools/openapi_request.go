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
)

// requestParams holds categorized parameters for building a request.
type requestParams struct {
	path    string
	query   url.Values
	headers map[string]string
	body    map[string]any
}

// buildRequest creates an HTTP request for an operation.
func (a *OpenAPIAdapter) buildRequest(ctx context.Context, op *OpenAPIOperation, args map[string]any) (*http.Request, error) {
	params := a.categorizeArgs(op, args)
	fullURL := a.buildRequestURL(params)
	reqBody, err := a.buildRequestBody(op.Method, params.body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, op.Method, fullURL, reqBody)
	if err != nil {
		return nil, err
	}

	a.setRequestHeaders(req, reqBody != nil, params.headers)
	if err := a.setAuth(req); err != nil {
		return nil, err
	}
	return req, nil
}

// categorizeArgs categorizes arguments into path, query, header, and body params.
func (a *OpenAPIAdapter) categorizeArgs(op *OpenAPIOperation, args map[string]any) requestParams {
	params := requestParams{
		path:    op.Path,
		query:   url.Values{},
		headers: make(map[string]string),
		body:    make(map[string]any),
	}

	paramNames := make(map[string]bool)
	for _, param := range op.Parameters {
		paramNames[param.Name] = true
		value, exists := args[param.Name]
		if !exists {
			continue
		}
		switch param.In {
		case "path":
			params.path = strings.ReplaceAll(params.path, "{"+param.Name+"}", fmt.Sprintf("%v", value))
		case "query":
			params.query.Set(param.Name, fmt.Sprintf("%v", value))
		case "header":
			params.headers[param.Name] = fmt.Sprintf("%v", value)
		}
	}

	// Remaining args go to request body
	for name, value := range args {
		if !paramNames[name] {
			params.body[name] = value
		}
	}
	return params
}

// buildRequestURL builds the full URL from base URL and params.
func (a *OpenAPIAdapter) buildRequestURL(params requestParams) string {
	fullURL := a.baseURL + params.path
	if len(params.query) > 0 {
		fullURL += "?" + params.query.Encode()
	}
	return fullURL
}

// buildRequestBody builds the request body for methods that support it.
func (a *OpenAPIAdapter) buildRequestBody(method string, bodyParams map[string]any) (io.Reader, error) {
	if len(bodyParams) == 0 {
		return nil, nil
	}
	if method != "POST" && method != "PUT" && method != "PATCH" {
		return nil, nil
	}
	jsonBody, err := json.Marshal(bodyParams)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal body: %w", err)
	}
	return bytes.NewReader(jsonBody), nil
}

// setRequestHeaders sets standard and custom headers on the request.
func (a *OpenAPIAdapter) setRequestHeaders(req *http.Request, hasBody bool, paramHeaders map[string]string) {
	if hasBody {
		req.Header.Set("Content-Type", contentTypeJSON)
	}
	req.Header.Set("Accept", contentTypeJSON)
	for k, v := range a.config.Headers {
		req.Header.Set(k, v)
	}
	for k, v := range paramHeaders {
		req.Header.Set(k, v)
	}
}

// setAuth sets authentication headers on a request.
func (a *OpenAPIAdapter) setAuth(req *http.Request) error {
	switch strings.ToLower(a.config.AuthType) {
	case "bearer":
		if a.config.AuthToken == "" {
			return fmt.Errorf("bearer auth requires a token")
		}
		req.Header.Set("Authorization", "Bearer "+a.config.AuthToken)
	case "basic":
		if a.config.AuthToken == "" {
			return fmt.Errorf("basic auth requires credentials")
		}
		parts := strings.SplitN(a.config.AuthToken, ":", 2)
		if len(parts) != 2 {
			return fmt.Errorf("basic auth token must be 'username:password'")
		}
		req.SetBasicAuth(parts[0], parts[1])
	case "":
		// No authentication
	default:
		return fmt.Errorf("unsupported auth type: %s", a.config.AuthType)
	}
	return nil
}
