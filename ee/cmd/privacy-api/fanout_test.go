/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// TestMemoryFanoutFromWorkspace_ReturnsWorkspaceUID guards the consent fan-out
// contract. memory-api's consent-events endpoint scopes its prune by
// memory_entities.workspace_id, which is the Workspace CRD UID (the runtime
// writes string(ws.UID) there). privacy-api must therefore send the UID as the
// ?workspace= scope — sending the workspace NAME makes the prune fail with
// "invalid input syntax for type uuid" and consent revocation never enforces.
func TestMemoryFanoutFromWorkspace_ReturnsWorkspaceUID(t *testing.T) {
	scheme := k8sruntime.NewScheme()
	require.NoError(t, omniav1alpha1.AddToScheme(scheme))

	const wsUID = "0d7ed66a-5187-449a-8730-e2823a62cef5"
	ws := &omniav1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "demo", UID: types.UID(wsUID)},
		Status: omniav1alpha1.WorkspaceStatus{
			Services: []omniav1alpha1.ServiceGroupStatus{
				{MemoryURL: "http://memory-demo-default.omnia-demo:8080"},
			},
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ws).Build()

	urls, uid := memoryFanoutFromWorkspace(context.Background(), c, "demo")

	require.Equal(t, []string{"http://memory-demo-default.omnia-demo:8080"}, urls)
	require.Equal(t, wsUID, uid, "fan-out must send the Workspace UID, not the name")
	require.NotEqual(t, "demo", uid)
}

// TestMemoryFanoutFromWorkspace_MissingWorkspace returns empty values rather
// than erroring when the Workspace CRD is absent.
func TestMemoryFanoutFromWorkspace_MissingWorkspace(t *testing.T) {
	scheme := k8sruntime.NewScheme()
	require.NoError(t, omniav1alpha1.AddToScheme(scheme))
	c := fake.NewClientBuilder().WithScheme(scheme).Build()

	urls, uid := memoryFanoutFromWorkspace(context.Background(), c, "demo")

	require.Empty(t, urls)
	require.Empty(t, uid)
}

// TestGroupTargetsFromWorkspace_CollectsPerGroupURLsAndUID verifies DSAR fan-out
// target resolution: one GroupTarget per service-group with a session or memory
// URL, plus the Workspace UID (the memory-scope key). Groups with neither URL are
// skipped.
func TestGroupTargetsFromWorkspace_CollectsPerGroupURLsAndUID(t *testing.T) {
	scheme := k8sruntime.NewScheme()
	require.NoError(t, omniav1alpha1.AddToScheme(scheme))

	const wsUID = "0d7ed66a-5187-449a-8730-e2823a62cef5"
	ws := &omniav1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "demo", UID: types.UID(wsUID)},
		Status: omniav1alpha1.WorkspaceStatus{
			Services: []omniav1alpha1.ServiceGroupStatus{
				{Name: "default", SessionURL: "http://session-default:8080", MemoryURL: "http://memory-default:8080"},
				{Name: "grp2", SessionURL: "http://session-grp2:8080"},
				{Name: "empty"},
			},
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ws).Build()

	targets, uid := groupTargetsFromWorkspace(context.Background(), c, "demo")

	require.Equal(t, wsUID, uid)
	require.Len(t, targets, 2, "the URL-less group must be skipped")
	require.Equal(t, "default", targets[0].Name)
	require.Equal(t, "http://session-default:8080", targets[0].SessionURL)
	require.Equal(t, "http://memory-default:8080", targets[0].MemoryURL)
	require.Equal(t, "grp2", targets[1].Name)
	require.Equal(t, "http://session-grp2:8080", targets[1].SessionURL)
	require.Empty(t, targets[1].MemoryURL)
}

// TestGroupTargetsFromWorkspace_MissingWorkspace returns empty values when the
// Workspace CRD is absent.
func TestGroupTargetsFromWorkspace_MissingWorkspace(t *testing.T) {
	scheme := k8sruntime.NewScheme()
	require.NoError(t, omniav1alpha1.AddToScheme(scheme))
	c := fake.NewClientBuilder().WithScheme(scheme).Build()

	targets, uid := groupTargetsFromWorkspace(context.Background(), c, "demo")

	require.Empty(t, targets)
	require.Empty(t, uid)
}
