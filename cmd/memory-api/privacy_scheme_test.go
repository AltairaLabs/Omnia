/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"testing"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	eev1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

func TestNewPrivacyMiddlewareScheme_RegistersWorkspaceKind(t *testing.T) {
	// PolicyWatcher's initial sync lists Workspace CRs (omnia API
	// group). Without omniav1alpha1.AddToScheme this fails with
	// "no kind is registered for v1alpha1.WorkspaceList" and the
	// privacy cache stays empty — silently disabling workspace-
	// scoped privacy enforcement.
	scheme := newPrivacyMiddlewareScheme()

	gvk := omniav1alpha1.GroupVersion.WithKind("Workspace")
	if _, err := scheme.New(gvk); err != nil {
		t.Fatalf("Workspace kind not registered: %v", err)
	}
	gvkList := omniav1alpha1.GroupVersion.WithKind("WorkspaceList")
	if _, err := scheme.New(gvkList); err != nil {
		t.Fatalf("WorkspaceList kind not registered: %v", err)
	}
}

func TestNewPrivacyMiddlewareScheme_RegistersSessionPrivacyPolicyKind(t *testing.T) {
	// SessionPrivacyPolicy CRs (ee API group) drive the privacy
	// middleware's per-workspace policy cache.
	scheme := newPrivacyMiddlewareScheme()

	gvk := eev1alpha1.GroupVersion.WithKind("SessionPrivacyPolicy")
	if _, err := scheme.New(gvk); err != nil {
		t.Fatalf("SessionPrivacyPolicy kind not registered: %v", err)
	}
}
