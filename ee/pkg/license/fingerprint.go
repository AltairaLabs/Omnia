/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package license

import (
	"context"
	"crypto/sha256"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ClusterFingerprint generates a stable, unique identifier for the Kubernetes cluster.
// The fingerprint is a SHA-256 hash of the kube-system and omnia-system namespace UIDs.
// This survives most cluster operations (node replacements, etcd restores) but changes
// if the cluster is completely destroyed and recreated.
func ClusterFingerprint(ctx context.Context, c client.Client) (string, error) {
	// Get kube-system namespace UID (stable across cluster lifetime)
	kubeSystem := &corev1.Namespace{}
	if err := c.Get(ctx, types.NamespacedName{Name: "kube-system"}, kubeSystem); err != nil {
		return "", fmt.Errorf("failed to get kube-system namespace: %w", err)
	}

	// Get omnia-system namespace UID (created during installation)
	omniaSystem := &corev1.Namespace{}
	if err := c.Get(ctx, types.NamespacedName{Name: LicenseSecretNamespace}, omniaSystem); err != nil {
		return "", fmt.Errorf("failed to get %s namespace: %w", LicenseSecretNamespace, err)
	}

	// Combine both UIDs for cluster fingerprint
	fingerprint := fmt.Sprintf("%s:%s", kubeSystem.UID, omniaSystem.UID)

	// Hash for privacy (don't send raw UIDs)
	hash := sha256.Sum256([]byte(fingerprint))
	return fmt.Sprintf("%x", hash[:16]), nil // 128-bit hash (32 hex chars)
}

// ValidateFingerprint checks if the given fingerprint matches the current cluster.
func ValidateFingerprint(ctx context.Context, c client.Client, expectedFingerprint string) (bool, error) {
	currentFingerprint, err := ClusterFingerprint(ctx, c)
	if err != nil {
		return false, err
	}
	return currentFingerprint == expectedFingerprint, nil
}
