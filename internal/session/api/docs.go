/*
Copyright 2026.

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
	_ "embed"
	"net/http"
)

//go:embed openapi.yaml
var openapiSpec []byte

// registerDocsRoutes adds the API documentation endpoints.
func (h *Handler) registerDocsRoutes(mux *http.ServeMux) {
	// Serve the raw OpenAPI spec.
	mux.HandleFunc("GET /api/v1/openapi.yaml", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		_, _ = w.Write(openapiSpec)
	})

	// Serve the branded API docs UI.
	mux.HandleFunc("GET /docs", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(docsHTML))
	})
}

const docsHTML = `<!DOCTYPE html>
<html>
<head>
  <title>Omnia Session API</title>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <style>
    body { margin: 0; }
    .custom-header {
      background: linear-gradient(135deg, #3B82F6, #8B5CF6);
      color: white;
      padding: 16px 24px;
      font-family: system-ui, -apple-system, sans-serif;
      display: flex;
      align-items: center;
      gap: 12px;
    }
    .custom-header h1 {
      margin: 0;
      font-size: 18px;
      font-weight: 600;
      letter-spacing: -0.01em;
    }
    .custom-header .subtitle {
      font-size: 13px;
      opacity: 0.8;
      font-weight: 400;
    }
    .custom-header .logo {
      width: 28px;
      height: 28px;
      background: rgba(255,255,255,0.2);
      border-radius: 6px;
      display: flex;
      align-items: center;
      justify-content: center;
      font-weight: 700;
      font-size: 14px;
    }
  </style>
</head>
<body>
  <div class="custom-header">
    <div class="logo">O</div>
    <div>
      <h1>Omnia Session API</h1>
      <div class="subtitle">AltairaLabs</div>
    </div>
  </div>
  <script id="api-reference" data-url="/api/v1/openapi.yaml" data-configuration='{"theme":"default","layout":"modern","hideDarkModeToggle":false,"customCss":".darklight-reference-promo{display:none} .sidebar{border-color:#E5E7EB} .dark .sidebar{border-color:#374151}","metaData":{"title":"Omnia Session API"}}'></script>
  <script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"></script>
</body>
</html>`
