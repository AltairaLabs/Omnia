/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"

	coreomniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

// TestNewPrivacyWatcherScheme_RecognizesWatchedKinds is the #1567 regression: the
// privacy PolicyWatcher lists SessionPrivacyPolicy (EE api) plus Workspace and
// AgentRuntime (core api) cluster-wide. Registering only the EE api left the core
// list kinds unknown, so the watcher failed to start with "no kind is registered
// for the type v1alpha1.WorkspaceList" even after its RBAC was granted (#1594).
func TestNewPrivacyWatcherScheme_RecognizesWatchedKinds(t *testing.T) {
	scheme := newPrivacyWatcherScheme()
	objs := []runtime.Object{
		&coreomniav1alpha1.WorkspaceList{},
		&coreomniav1alpha1.Workspace{},
		&coreomniav1alpha1.AgentRuntimeList{},
		&coreomniav1alpha1.AgentRuntime{},
		&omniav1alpha1.SessionPrivacyPolicyList{},
		&omniav1alpha1.SessionPrivacyPolicy{},
		&corev1.Secret{}, // the KMS encryptor factory reads Secrets via this client
	}
	for _, obj := range objs {
		gvks, _, err := scheme.ObjectKinds(obj)
		if err != nil || len(gvks) == 0 {
			t.Errorf("scheme does not recognize %T: err=%v gvks=%v", obj, err, gvks)
		}
	}
}
