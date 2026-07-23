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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
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

// K8sWorkspaceLister implements WorkspaceLister against a controller-runtime
// client, scoped to the memory-api's OWN workspace. memory-api is deployed
// per workspace, so it Gets its single Workspace by name rather than listing
// every Workspace in the cluster (#1899).
type K8sWorkspaceLister struct {
	client       client.Client
	ownWorkspace string
}

// NewK8sWorkspaceLister constructs a lister scoped to ownWorkspace (the
// Workspace CR's metadata.name, e.g. "demo" — not the namespace it owns).
func NewK8sWorkspaceLister(c client.Client, ownWorkspace string) *K8sWorkspaceLister {
	return &K8sWorkspaceLister{client: c, ownWorkspace: ownWorkspace}
}

// ForPolicy returns the memory-api's own Workspace when it opts into policyName
// via any services[*].memory.policyRef, else an empty slice. A missing own
// Workspace is non-fatal (transient at boot / mid-reconcile): logged upstream
// by returning empty, matching #1875's tolerance.
func (l *K8sWorkspaceLister) ForPolicy(ctx context.Context, policyName string) ([]Workspace, error) {
	if l.ownWorkspace == "" {
		return nil, nil
	}
	var w memoryv1.Workspace
	if err := l.client.Get(ctx, client.ObjectKey{Name: l.ownWorkspace}, &w); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("get own Workspace %q: %w", l.ownWorkspace, err)
	}
	if !workspaceOptsInto(w, policyName) {
		return nil, nil
	}
	return []Workspace{{Name: w.Name, UID: string(w.UID)}}, nil
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
