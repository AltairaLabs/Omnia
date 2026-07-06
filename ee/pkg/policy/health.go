/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package policy

import "net/http"

// Response content-type constants shared by the broker's HTTP handlers.
const (
	contentTypeJSON   = "application/json"
	headerContentType = "Content-Type"
)

// HealthHandler returns a simple health check handler, used by the
// policy-broker binary for both /healthz and /readyz.
func HealthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set(headerContentType, contentTypeJSON)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}
}
