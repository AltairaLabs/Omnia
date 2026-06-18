/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package consolidation

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	memoryv1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func TestK8sPolicyLister_ReturnsAllPolicies(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = memoryv1.AddToScheme(scheme)

	p1 := &memoryv1.MemoryPolicy{}
	p1.Name = "ws-a"
	p2 := &memoryv1.MemoryPolicy{}
	p2.Name = "ws-b"

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(p1, p2).Build()
	lister := NewK8sPolicyLister(c)

	policies, err := lister.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(policies) != 2 {
		t.Errorf("len(policies) = %d, want 2", len(policies))
	}
}

func TestK8sPolicyLister_EmptyCluster(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = memoryv1.AddToScheme(scheme)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	lister := NewK8sPolicyLister(c)
	policies, err := lister.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(policies) != 0 {
		t.Errorf("expected empty list, got %d", len(policies))
	}
}
