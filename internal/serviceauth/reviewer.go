/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

// Package serviceauth provides the reusable foundation for Kubernetes
// ServiceAccount-token-based service-to-service authentication in Omnia.
//
// Callers (in-cluster pods) present their projected ServiceAccount token as an
// "Authorization: Bearer <jwt>" header (HTTP) or "authorization" metadata
// (gRPC). The server validates the token via the Kubernetes TokenReview API and
// authorizes the resulting ServiceAccount subject
// (system:serviceaccount:<ns>:<name>) against an allowlist.
package serviceauth

import (
	"context"
	"fmt"

	authnv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// TokenReviewer authenticates a bearer token and returns whether it is valid
// and the authenticated username (e.g.
// "system:serviceaccount:omnia-system:omnia-dashboard"). Implementations call
// the Kubernetes TokenReview API.
type TokenReviewer interface {
	ReviewToken(ctx context.Context, token string) (authenticated bool, username string, err error)
}

// k8sTokenReviewer authenticates tokens via the Kubernetes TokenReview API.
type k8sTokenReviewer struct {
	client    kubernetes.Interface
	audiences []string
}

// NewK8sTokenReviewer builds a TokenReviewer from a rest.Config. The server
// ServiceAccount needs `authentication.k8s.io/tokenreviews: create`.
//
// When audiences is non-empty it is set on the TokenReviewSpec, which supports
// audience-bound projected tokens (the token must have been minted for one of
// these audiences). An empty/nil audiences slice reviews against the API
// server's default audiences.
func NewK8sTokenReviewer(cfg *rest.Config, audiences []string) (TokenReviewer, error) {
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("build kubernetes client for token review: %w", err)
	}
	return &k8sTokenReviewer{client: cs, audiences: audiences}, nil
}

func (k *k8sTokenReviewer) ReviewToken(ctx context.Context, token string) (bool, string, error) {
	spec := authnv1.TokenReviewSpec{Token: token}
	if len(k.audiences) > 0 {
		spec.Audiences = k.audiences
	}
	review := &authnv1.TokenReview{Spec: spec}
	res, err := k.client.AuthenticationV1().TokenReviews().Create(ctx, review, metav1.CreateOptions{})
	if err != nil {
		return false, "", fmt.Errorf("token review request: %w", err)
	}
	if res.Status.Error != "" {
		return false, "", fmt.Errorf("token review error: %s", res.Status.Error)
	}
	return res.Status.Authenticated, res.Status.User.Username, nil
}
