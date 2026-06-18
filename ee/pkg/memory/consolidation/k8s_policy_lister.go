/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package consolidation

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	memoryv1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// K8sPolicyLister implements PolicyLister by listing MemoryPolicy CRs
// cluster-wide via a controller-runtime client. Used by the production
// memory-api wiring; tests use fakePolicyLister instead.
type K8sPolicyLister struct {
	client client.Client
}

// NewK8sPolicyLister constructs a K8sPolicyLister.
func NewK8sPolicyLister(c client.Client) *K8sPolicyLister {
	return &K8sPolicyLister{client: c}
}

// List returns every MemoryPolicy CR in the cluster.
func (l *K8sPolicyLister) List(ctx context.Context) ([]memoryv1.MemoryPolicy, error) {
	var list memoryv1.MemoryPolicyList
	if err := l.client.List(ctx, &list); err != nil {
		return nil, fmt.Errorf("list MemoryPolicies: %w", err)
	}
	return list.Items, nil
}
