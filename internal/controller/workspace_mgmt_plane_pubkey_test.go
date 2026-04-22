/*
Copyright 2026.

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

package controller

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("add core scheme: %v", err)
	}
	if err := omniav1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add omnia scheme: %v", err)
	}
	return scheme
}

func newSigningSecret(name, namespace, cert string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Type:       corev1.SecretTypeTLS,
		Data: map[string][]byte{
			tlsCertSecretKey: []byte(cert),
			"tls.key":        []byte("---fake-private-key---"),
		},
	}
}

func newWorkspace(name, namespace string) *omniav1alpha1.Workspace {
	return &omniav1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: omniav1alpha1.WorkspaceSpec{
			Namespace: omniav1alpha1.NamespaceConfig{
				Name:   namespace,
				Create: true,
			},
		},
	}
}

func TestReconcileMgmtPlanePubkey_NotConfigured(t *testing.T) {
	t.Parallel()
	scheme := newScheme(t)
	fc := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &WorkspaceReconciler{Client: fc, Scheme: scheme}
	ws := newWorkspace("ws", "ws-ns")

	if err := r.reconcileMgmtPlanePubkey(context.Background(), ws); err != nil {
		t.Fatalf("err = %v, want nil when not configured", err)
	}
	cm := &corev1.ConfigMap{}
	err := fc.Get(context.Background(),
		types.NamespacedName{Namespace: "ws-ns", Name: MgmtPlanePubkeyConfigMapName}, cm)
	if !apierrors.IsNotFound(err) {
		t.Errorf("expected ConfigMap absent when not configured, got err=%v", err)
	}
}

func TestReconcileMgmtPlanePubkey_OperatorNamespaceEmpty(t *testing.T) {
	t.Parallel()
	scheme := newScheme(t)
	fc := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &WorkspaceReconciler{
		Client:                 fc,
		Scheme:                 scheme,
		MgmtPlaneSigningSecret: "some-secret", // configured but no operator ns
	}
	ws := newWorkspace("ws", "ws-ns")

	if err := r.reconcileMgmtPlanePubkey(context.Background(), ws); err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
}

func TestReconcileMgmtPlanePubkey_SourceMissing(t *testing.T) {
	t.Parallel()
	scheme := newScheme(t)
	fc := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &WorkspaceReconciler{
		Client:                 fc,
		Scheme:                 scheme,
		OperatorNamespace:      "omnia-system",
		MgmtPlaneSigningSecret: "absent",
	}
	ws := newWorkspace("ws", "ws-ns")

	// Source Secret doesn't exist — reconciler should not error.
	if err := r.reconcileMgmtPlanePubkey(context.Background(), ws); err != nil {
		t.Fatalf("err = %v, want nil when source absent", err)
	}
}

func TestReconcileMgmtPlanePubkey_SourceMissingTLSCert(t *testing.T) {
	t.Parallel()
	scheme := newScheme(t)
	badSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "broken", Namespace: "omnia-system"},
		Type:       corev1.SecretTypeTLS,
		Data:       map[string][]byte{"tls.key": []byte("only-private")},
	}
	fc := fake.NewClientBuilder().WithScheme(scheme).WithObjects(badSecret).Build()
	r := &WorkspaceReconciler{
		Client:                 fc,
		Scheme:                 scheme,
		OperatorNamespace:      "omnia-system",
		MgmtPlaneSigningSecret: "broken",
	}
	ws := newWorkspace("ws", "ws-ns")

	if err := r.reconcileMgmtPlanePubkey(context.Background(), ws); err == nil {
		t.Error("expected error when source secret missing tls.crt")
	}
}

func TestReconcileMgmtPlanePubkey_CreatesMirror(t *testing.T) {
	t.Parallel()
	scheme := newScheme(t)
	source := newSigningSecret("dashboard-keys", "omnia-system", "-----BEGIN CERTIFICATE-----\nfake\n-----END CERTIFICATE-----\n")
	fc := fake.NewClientBuilder().WithScheme(scheme).WithObjects(source).Build()
	r := &WorkspaceReconciler{
		Client:                 fc,
		Scheme:                 scheme,
		OperatorNamespace:      "omnia-system",
		MgmtPlaneSigningSecret: "dashboard-keys",
	}
	ws := newWorkspace("ws-alpha", "ws-alpha-ns")

	if err := r.reconcileMgmtPlanePubkey(context.Background(), ws); err != nil {
		t.Fatalf("reconcileMgmtPlanePubkey: %v", err)
	}

	cm := &corev1.ConfigMap{}
	err := fc.Get(context.Background(),
		types.NamespacedName{Namespace: "ws-alpha-ns", Name: MgmtPlanePubkeyConfigMapName}, cm)
	if err != nil {
		t.Fatalf("expected ConfigMap to be created: %v", err)
	}
	if got := cm.Data[MgmtPlanePubkeyDataKey]; got != string(source.Data[tlsCertSecretKey]) {
		t.Errorf("ConfigMap data[%q] = %q, want %q", MgmtPlanePubkeyDataKey, got, string(source.Data[tlsCertSecretKey]))
	}
	if got := cm.Labels[labelWorkspace]; got != "ws-alpha" {
		t.Errorf("workspace label = %q, want %q", got, "ws-alpha")
	}
	if got := cm.Labels[labelWorkspaceManaged]; got != labelValueTrue {
		t.Errorf("managed label = %q, want %q", got, labelValueTrue)
	}
}

func TestReconcileMgmtPlanePubkey_UpdatesMirrorOnSourceChange(t *testing.T) {
	t.Parallel()
	scheme := newScheme(t)
	source := newSigningSecret("dashboard-keys", "omnia-system", "ORIGINAL")
	stale := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      MgmtPlanePubkeyConfigMapName,
			Namespace: "ws-ns",
		},
		Data: map[string]string{MgmtPlanePubkeyDataKey: "STALE"},
	}
	fc := fake.NewClientBuilder().WithScheme(scheme).WithObjects(source, stale).Build()
	r := &WorkspaceReconciler{
		Client:                 fc,
		Scheme:                 scheme,
		OperatorNamespace:      "omnia-system",
		MgmtPlaneSigningSecret: "dashboard-keys",
	}
	ws := newWorkspace("ws", "ws-ns")

	if err := r.reconcileMgmtPlanePubkey(context.Background(), ws); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	cm := &corev1.ConfigMap{}
	if err := fc.Get(context.Background(),
		types.NamespacedName{Namespace: "ws-ns", Name: MgmtPlanePubkeyConfigMapName}, cm); err != nil {
		t.Fatalf("get configmap: %v", err)
	}
	if got := cm.Data[MgmtPlanePubkeyDataKey]; got != "ORIGINAL" {
		t.Errorf("ConfigMap data not updated: got %q, want %q", got, "ORIGINAL")
	}
}

func TestReconcileMgmtPlanePubkey_DeletesMirrorWhenSourceVanishes(t *testing.T) {
	t.Parallel()
	scheme := newScheme(t)
	// Mirror exists but source is missing — simulates dashboard.enabled
	// flipped to false after a previous reconcile.
	stale := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      MgmtPlanePubkeyConfigMapName,
			Namespace: "ws-ns",
		},
		Data: map[string]string{MgmtPlanePubkeyDataKey: "OLD"},
	}
	fc := fake.NewClientBuilder().WithScheme(scheme).WithObjects(stale).Build()
	r := &WorkspaceReconciler{
		Client:                 fc,
		Scheme:                 scheme,
		OperatorNamespace:      "omnia-system",
		MgmtPlaneSigningSecret: "dashboard-keys",
	}
	ws := newWorkspace("ws", "ws-ns")

	if err := r.reconcileMgmtPlanePubkey(context.Background(), ws); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	cm := &corev1.ConfigMap{}
	err := fc.Get(context.Background(),
		types.NamespacedName{Namespace: "ws-ns", Name: MgmtPlanePubkeyConfigMapName}, cm)
	if !apierrors.IsNotFound(err) {
		t.Errorf("expected ConfigMap deleted when source vanishes, got err=%v", err)
	}
}

func TestReconcileMgmtPlanePubkey_DeleteIsIdempotent(t *testing.T) {
	t.Parallel()
	scheme := newScheme(t)
	// Source absent and mirror also absent — should not error.
	fc := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &WorkspaceReconciler{
		Client:                 fc,
		Scheme:                 scheme,
		OperatorNamespace:      "omnia-system",
		MgmtPlaneSigningSecret: "dashboard-keys",
	}
	ws := newWorkspace("ws", "ws-ns")

	if err := r.reconcileMgmtPlanePubkey(context.Background(), ws); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
}

// Sanity: prove the workspace namespace lookup uses the spec value, not
// the workspace's metadata namespace (Workspace is cluster-scoped — its
// metadata.namespace is always empty).
func TestReconcileMgmtPlanePubkey_UsesSpecNamespaceNotMetaNamespace(t *testing.T) {
	t.Parallel()
	scheme := newScheme(t)
	source := newSigningSecret("dashboard-keys", "omnia-system", "CERT")
	fc := fake.NewClientBuilder().WithScheme(scheme).WithObjects(source).Build()
	r := &WorkspaceReconciler{
		Client:                 fc,
		Scheme:                 scheme,
		OperatorNamespace:      "omnia-system",
		MgmtPlaneSigningSecret: "dashboard-keys",
	}
	ws := newWorkspace("ws", "spec-defined-ns")

	if err := r.reconcileMgmtPlanePubkey(context.Background(), ws); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	// ConfigMap must end up in spec.namespace.name, not metadata.namespace.
	if err := fc.Get(context.Background(),
		client.ObjectKey{Namespace: "spec-defined-ns", Name: MgmtPlanePubkeyConfigMapName},
		&corev1.ConfigMap{}); err != nil {
		t.Errorf("ConfigMap not in spec namespace: %v", err)
	}
}
