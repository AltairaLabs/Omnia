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
	"fmt"
	"net/http"
	"strings"
)

// authorizationValue returns the Authorization header/metadata value for the
// given auth type and token, or "" when no authentication applies. It is the
// shared credential-formatting seam used by the HTTP header path and the gRPC
// metadata path.
func authorizationValue(authType, authToken string) (string, error) {
	switch strings.ToLower(authType) {
	case "bearer":
		if authToken == "" {
			return "", fmt.Errorf("bearer auth requires a token")
		}
		return "Bearer " + authToken, nil
	case "basic":
		if authToken == "" {
			return "", fmt.Errorf("basic auth requires credentials")
		}
		parts := strings.SplitN(authToken, ":", 2)
		if len(parts) != 2 {
			return "", fmt.Errorf("basic auth token must be 'username:password'")
		}
		req := &http.Request{Header: http.Header{}}
		req.SetBasicAuth(parts[0], parts[1])
		return req.Header.Get("Authorization"), nil
	case "":
		return "", nil
	default:
		return "", fmt.Errorf("unsupported auth type: %s", authType)
	}
}

// mergeAuthHeaders adds an Authorization header to the map based on auth type.
func mergeAuthHeaders(headers map[string]string, authType, authToken string) error {
	val, err := authorizationValue(authType, authToken)
	if err != nil {
		return err
	}
	if val != "" {
		headers["Authorization"] = val
	}
	return nil
}
