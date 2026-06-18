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

// Workspace identifies one workspace the consolidation worker should
// process under a given policy. UID is the Workspace CR's metadata.uid
// — the value memory_entities.workspace_id is populated with — and is
// the data key for every operation (pre-filter SQL, advisory lock,
// FunctionInput.WorkspaceID, RunID, audit). Name is preserved for
// logs / Prometheus labels only.
type Workspace struct {
	Name string
	UID  string
}

// WorkspaceLister returns the Workspaces opted into a MemoryPolicy via
// any of their services[*].memory.policyRef fields.
type WorkspaceLister interface {
	ForPolicy(ctx context.Context, policyName string) ([]Workspace, error)
}

// K8sWorkspaceLister implements WorkspaceLister against a
// controller-runtime client. Cluster-wide list — the assumption is
// that any Workspace CR in the cluster may opt into any MemoryPolicy.
type K8sWorkspaceLister struct {
	client client.Client
}

// NewK8sWorkspaceLister constructs a K8sWorkspaceLister.
func NewK8sWorkspaceLister(c client.Client) *K8sWorkspaceLister {
	return &K8sWorkspaceLister{client: c}
}

// ForPolicy returns every Workspace whose services[*].memory.policyRef
// equals policyName. Service groups with no memory block or no
// policyRef are skipped.
func (l *K8sWorkspaceLister) ForPolicy(ctx context.Context, policyName string) ([]Workspace, error) {
	var list memoryv1.WorkspaceList
	if err := l.client.List(ctx, &list); err != nil {
		return nil, fmt.Errorf("list Workspaces: %w", err)
	}
	out := make([]Workspace, 0, len(list.Items))
	for _, w := range list.Items {
		if !workspaceOptsInto(w, policyName) {
			continue
		}
		out = append(out, Workspace{Name: w.Name, UID: string(w.UID)})
	}
	return out, nil
}

func workspaceOptsInto(w memoryv1.Workspace, policyName string) bool {
	for _, sg := range w.Spec.Services {
		if sg.Memory == nil || sg.Memory.PolicyRef == nil {
			continue
		}
		if sg.Memory.PolicyRef.Name == policyName {
			return true
		}
	}
	return false
}
