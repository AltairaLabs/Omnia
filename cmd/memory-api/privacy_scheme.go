/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	eev1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

// newPrivacyMiddlewareScheme constructs the controller-runtime scheme
// the enterprise privacy middleware's k8s client uses. PolicyWatcher
// lists SessionPrivacyPolicy CRs (ee API group) AND Workspace CRs
// (core API group); both must be registered or the initial cache
// sync fails with
//
//	"no kind is registered for the type v1alpha1.WorkspaceList"
//
// and workspace-scoped privacy enforcement is silently disabled.
//
// Extracted to a standalone testable function so we can verify both
// kinds are registered without spinning up an in-cluster client.
func newPrivacyMiddlewareScheme() *k8sruntime.Scheme {
	scheme := k8sruntime.NewScheme()
	utilruntime.Must(eev1alpha1.AddToScheme(scheme))
	utilruntime.Must(omniav1alpha1.AddToScheme(scheme))
	return scheme
}
