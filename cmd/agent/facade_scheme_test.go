/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// TestNewFacadeScheme_RegistersCoreV1 guards issue #1571: the facade's k8s
// client must register core/v1 so the auth chain can List/Get Secrets. The
// api-key store Lists *corev1.SecretList and the oidc validator Gets
// *corev1.Secret; if the scheme only carries the omnia CRD types those
// calls fail at startup with "no kind is registered for the type v1.SecretList"
// and the facade crash-loops whenever spec.externalAuth is set.
func TestNewFacadeScheme_RegistersCoreV1(t *testing.T) {
	scheme := newFacadeScheme()

	for _, obj := range []runtime.Object{
		&corev1.SecretList{}, // api-key store loadOnce List
		&corev1.Secret{},     // oidc JWKS Get
	} {
		if gvks, _, err := scheme.ObjectKinds(obj); err != nil || len(gvks) == 0 {
			t.Errorf("facade scheme does not recognize %T: err=%v gvks=%v", obj, err, gvks)
		}
	}
}

// TestNewFacadeScheme_RegistersOmniaTypes ensures the CRD types the auth chain
// reads (AgentRuntime) remain registered alongside core/v1.
func TestNewFacadeScheme_RegistersOmniaTypes(t *testing.T) {
	scheme := newFacadeScheme()
	if gvks, _, err := scheme.ObjectKinds(&omniav1alpha1.AgentRuntime{}); err != nil || len(gvks) == 0 {
		t.Fatalf("expected AgentRuntime registered, err=%v gvks=%v", err, gvks)
	}
}
