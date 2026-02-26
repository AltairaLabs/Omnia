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

// Package httputil provides shared HTTP constants and helpers.
package httputil

import (
	"encoding/json"
	"net/http"
)

// Common HTTP header names and content types.
const (
	HeaderContentType = "Content-Type"
	ContentTypeJSON   = "application/json"
)

// WriteJSON serialises v as JSON and writes it to w with the given status code.
// The Content-Type header is set to application/json.
func WriteJSON(w http.ResponseWriter, statusCode int, v any) error {
	w.Header().Set(HeaderContentType, ContentTypeJSON)
	w.WriteHeader(statusCode)
	return json.NewEncoder(w).Encode(v)
}
