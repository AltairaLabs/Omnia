/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package k8s

import (
	"os"
	"strings"
)

// podNamespaceFile is the in-cluster service-account namespace file, present in
// every pod regardless of downward-API wiring. Overridable in tests.
var podNamespaceFile = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"

// OperatorNamespace returns the namespace the current process runs in.
//
// Resolution order:
//  1. POD_NAMESPACE env (downward API, when the deployment injects it),
//  2. the in-cluster service-account namespace file (always present in a pod),
//  3. the provided fallback (preserves legacy behaviour for kustomize/test
//     paths where neither is available — historically "omnia-system").
//
// This lets Helm installs into any release namespace (e.g. "omnia") resolve
// the operator's own namespace instead of assuming "omnia-system".
func OperatorNamespace(fallback string) string {
	if ns := strings.TrimSpace(os.Getenv("POD_NAMESPACE")); ns != "" {
		return ns
	}
	if data, err := os.ReadFile(podNamespaceFile); err == nil {
		if ns := strings.TrimSpace(string(data)); ns != "" {
			return ns
		}
	}
	return fallback
}
