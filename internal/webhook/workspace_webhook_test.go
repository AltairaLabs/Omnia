/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package webhook

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// workspace returns a cluster-scoped Workspace named wsName targeting nsName.
func workspace(wsName, nsName string) *corev1alpha1.Workspace {
	return &corev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: wsName},
		Spec: corev1alpha1.WorkspaceSpec{
			Namespace: corev1alpha1.NamespaceConfig{Name: nsName},
		},
	}
}

// TestWorkspaceValidator_RejectsCollidingCreate: a second workspace pointing at a
// namespace an existing workspace already claims is rejected at admission time.
func TestWorkspaceValidator_RejectsCollidingCreate(t *testing.T) {
	scheme := newWebhookScheme(t)
	existing := workspace("my-team", "my-team")
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()
	v := &WorkspaceValidator{Client: cl}

	warns, err := v.ValidateCreate(context.Background(), workspace("customer-support", "my-team"))
	require.Error(t, err)
	assert.Empty(t, warns)
	assert.Contains(t, err.Error(), "my-team")
	assert.Contains(t, err.Error(), "already claimed by workspace")
}

// TestWorkspaceValidator_AllowsNonCollidingCreate: distinct namespaces are fine.
func TestWorkspaceValidator_AllowsNonCollidingCreate(t *testing.T) {
	scheme := newWebhookScheme(t)
	existing := workspace("my-team", "my-team")
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()
	v := &WorkspaceValidator{Client: cl}

	warns, err := v.ValidateCreate(context.Background(), workspace("customer-support", "customer-support"))
	require.NoError(t, err)
	assert.Empty(t, warns)
}

// TestWorkspaceValidator_AllowsSelfUpdate: re-applying the same workspace (matched
// by name) never collides with its own namespace claim.
func TestWorkspaceValidator_AllowsSelfUpdate(t *testing.T) {
	scheme := newWebhookScheme(t)
	ws := workspace("my-team", "my-team")
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ws).Build()
	v := &WorkspaceValidator{Client: cl}

	warns, err := v.ValidateUpdate(context.Background(), ws, ws)
	require.NoError(t, err)
	assert.Empty(t, warns)
}

// TestWorkspaceValidator_RejectsCollidingUpdate: repointing a workspace at another
// workspace's namespace is rejected on update.
func TestWorkspaceValidator_RejectsCollidingUpdate(t *testing.T) {
	scheme := newWebhookScheme(t)
	other := workspace("my-team", "my-team")
	moving := workspace("customer-support", "customer-support")
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(other, moving).Build()
	v := &WorkspaceValidator{Client: cl}

	repointed := workspace("customer-support", "my-team")
	_, err := v.ValidateUpdate(context.Background(), moving, repointed)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "my-team")
}

// TestWorkspaceValidator_AllowsDelete: deletions are always permitted.
func TestWorkspaceValidator_AllowsDelete(t *testing.T) {
	v := &WorkspaceValidator{}
	warns, err := v.ValidateDelete(context.Background(), workspace("my-team", "my-team"))
	require.NoError(t, err)
	assert.Empty(t, warns)
}

// TestWorkspaceValidator_NilClientIsNoop: a nil client stays advisory (never blocks).
func TestWorkspaceValidator_NilClientIsNoop(t *testing.T) {
	v := &WorkspaceValidator{Client: nil}
	warns, err := v.ValidateCreate(context.Background(), workspace("customer-support", "my-team"))
	require.NoError(t, err)
	assert.Empty(t, warns)
}

// TestWorkspaceValidator_EmptyNamespaceNameIsNoop: a workspace with no namespace
// name declared can't collide (CEL requires it in practice; guard anyway).
func TestWorkspaceValidator_EmptyNamespaceNameIsNoop(t *testing.T) {
	scheme := newWebhookScheme(t)
	existing := workspace("my-team", "my-team")
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()
	v := &WorkspaceValidator{Client: cl}

	warns, err := v.ValidateCreate(context.Background(), workspace("no-ns", ""))
	require.NoError(t, err)
	assert.Empty(t, warns)
}
