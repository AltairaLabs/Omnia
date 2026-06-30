/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package privacy

import (
	_ "embed"
	"net/http"
)

//go:embed openapi.yaml
var openapiSpec []byte

//go:embed docs.html
var privacyDocsHTML []byte

// RegisterDocs adds the API documentation endpoints to mux.
//
// Routes registered:
//   - GET /api/v1/openapi.yaml — raw OpenAPI spec (application/yaml)
//   - GET /docs               — Scalar API reference UI
func RegisterDocs(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/openapi.yaml", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		_, _ = w.Write(openapiSpec)
	})

	mux.HandleFunc("GET /docs", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(privacyDocsHTML)
	})
}
