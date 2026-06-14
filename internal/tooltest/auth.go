/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package tooltest

import (
	"k8s.io/client-go/rest"

	"github.com/altairalabs/omnia/internal/serviceauth"
)

// TokenReviewer authenticates a bearer token and returns whether it is valid
// and the authenticated username (e.g. "system:serviceaccount:omnia-system:omnia-dashboard").
// It is an alias of the shared serviceauth.TokenReviewer so there is a single
// implementation across the codebase.
type TokenReviewer = serviceauth.TokenReviewer

// NewK8sTokenReviewer builds a TokenReviewer from a rest.Config using the
// shared serviceauth implementation with default (unbound) audiences. The
// operator ServiceAccount needs `authentication.k8s.io/tokenreviews: create`.
func NewK8sTokenReviewer(cfg *rest.Config) (TokenReviewer, error) {
	return serviceauth.NewK8sTokenReviewer(cfg, nil)
}
