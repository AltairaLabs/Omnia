/*
Copyright 2026 Altaira Labs.

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

	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

const (
	testPrivacyDefaultRole  = "omnia-privacy-default-reader"
	testConsolidationRole   = "omnia-memory-consolidation-reader"
	testSessionSAForPrivacy = "session-acme-default"
)

// TestPrivacyDefaultBinding_NoClusterRoleNameIsNoop verifies the binding is
// skipped on OSS installs where no ClusterRole name is configured.
func TestPrivacyDefaultBinding_NoClusterRoleNameIsNoop(t *testing.T) {
	g := NewWithT(t)
	r := newAuthReconciler(t, ServiceAuthConfig{Enabled: true})
	r.PrivacyDefaultReaderClusterRole = ""
	ctx := context.Background()

	g.Expect(r.reconcilePrivacyDefaultBinding(ctx, testAuthNS, testMemorySAName)).To(Succeed())

	crb := &rbacv1.ClusterRoleBinding{}
	err := r.Get(ctx, types.NamespacedName{Name: privacyDefaultBindingName(testAuthNS, testMemorySAName)}, crb)
	g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
}

// TestPrivacyDefaultBinding_CreatesClusterRoleBinding verifies the CRB is
// created against the configured ClusterRole and target SA, and is idempotent.
func TestPrivacyDefaultBinding_CreatesClusterRoleBinding(t *testing.T) {
	g := NewWithT(t)
	r := newAuthReconciler(t, ServiceAuthConfig{Enabled: false})
	r.PrivacyDefaultReaderClusterRole = testPrivacyDefaultRole
	ctx := context.Background()

	g.Expect(r.reconcilePrivacyDefaultBinding(ctx, testAuthNS, testMemorySAName)).To(Succeed())

	crb := &rbacv1.ClusterRoleBinding{}
	name := privacyDefaultBindingName(testAuthNS, testMemorySAName)
	g.Expect(r.Get(ctx, types.NamespacedName{Name: name}, crb)).To(Succeed())
	g.Expect(crb.RoleRef.Name).To(Equal(testPrivacyDefaultRole))
	g.Expect(crb.RoleRef.Kind).To(Equal(kindClusterRole))
	g.Expect(crb.Subjects).To(HaveLen(1))
	g.Expect(crb.Subjects[0].Kind).To(Equal(kindServiceAccount))
	g.Expect(crb.Subjects[0].Name).To(Equal(testMemorySAName))
	g.Expect(crb.Subjects[0].Namespace).To(Equal(testAuthNS))

	// Idempotent: second call must not error.
	g.Expect(r.reconcilePrivacyDefaultBinding(ctx, testAuthNS, testMemorySAName)).To(Succeed())
}

// TestPrivacyDefaultBinding_RepointsStaleRoleRef verifies that a pre-existing
// binding pointing at a different (legacy) ClusterRole is deleted + recreated,
// since roleRef is immutable.
func TestPrivacyDefaultBinding_RepointsStaleRoleRef(t *testing.T) {
	g := NewWithT(t)
	r := newAuthReconciler(t, ServiceAuthConfig{Enabled: false})
	ctx := context.Background()

	name := privacyDefaultBindingName(testAuthNS, testMemorySAName)
	stale := &rbacv1.ClusterRoleBinding{}
	stale.Name = name
	stale.RoleRef = rbacv1.RoleRef{APIGroup: rbacAPIGroup, Kind: kindClusterRole, Name: "omnia-legacy-reader"}
	stale.Subjects = []rbacv1.Subject{{Kind: kindServiceAccount, Name: testMemorySAName, Namespace: testAuthNS}}
	g.Expect(r.Create(ctx, stale)).To(Succeed())

	r.PrivacyDefaultReaderClusterRole = testPrivacyDefaultRole
	g.Expect(r.reconcilePrivacyDefaultBinding(ctx, testAuthNS, testMemorySAName)).To(Succeed())

	crb := &rbacv1.ClusterRoleBinding{}
	g.Expect(r.Get(ctx, types.NamespacedName{Name: name}, crb)).To(Succeed())
	g.Expect(crb.RoleRef.Name).To(Equal(testPrivacyDefaultRole))
}

// TestMemoryConsolidationBinding_NoClusterRoleNameIsNoop verifies the binding is
// skipped on OSS installs.
func TestMemoryConsolidationBinding_NoClusterRoleNameIsNoop(t *testing.T) {
	g := NewWithT(t)
	r := newAuthReconciler(t, ServiceAuthConfig{Enabled: true})
	r.MemoryConsolidationReaderClusterRole = ""
	ctx := context.Background()

	g.Expect(r.reconcileMemoryConsolidationBinding(ctx, testAuthNS, testMemorySAName)).To(Succeed())

	crb := &rbacv1.ClusterRoleBinding{}
	err := r.Get(ctx, types.NamespacedName{Name: memoryConsolidationBindingName(testAuthNS, testMemorySAName)}, crb)
	g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
}

// TestMemoryConsolidationBinding_CreatesClusterRoleBinding verifies the CRB is
// created against the configured ClusterRole and memory-api SA.
func TestMemoryConsolidationBinding_CreatesClusterRoleBinding(t *testing.T) {
	g := NewWithT(t)
	r := newAuthReconciler(t, ServiceAuthConfig{Enabled: false})
	r.MemoryConsolidationReaderClusterRole = testConsolidationRole
	ctx := context.Background()

	g.Expect(r.reconcileMemoryConsolidationBinding(ctx, testAuthNS, testMemorySAName)).To(Succeed())

	crb := &rbacv1.ClusterRoleBinding{}
	name := memoryConsolidationBindingName(testAuthNS, testMemorySAName)
	g.Expect(r.Get(ctx, types.NamespacedName{Name: name}, crb)).To(Succeed())
	g.Expect(crb.RoleRef.Name).To(Equal(testConsolidationRole))
	g.Expect(crb.Subjects).To(HaveLen(1))
	g.Expect(crb.Subjects[0].Name).To(Equal(testMemorySAName))
	g.Expect(crb.Subjects[0].Namespace).To(Equal(testAuthNS))

	// Idempotent.
	g.Expect(r.reconcileMemoryConsolidationBinding(ctx, testAuthNS, testMemorySAName)).To(Succeed())
}

// TestPrivacyReaderRole_CreatesNamespacedRole verifies the namespaced Role
// grants list;watch on sessionprivacypolicies + agentruntimes, owner-referenced
// to the Workspace.
func TestPrivacyReaderRole_CreatesNamespacedRole(t *testing.T) {
	g := NewWithT(t)
	r := newAuthReconciler(t, ServiceAuthConfig{Enabled: false})
	ws := newTestWorkspace("acme", testAuthNS, nil)
	ctx := context.Background()

	g.Expect(r.reconcilePrivacyReaderRole(ctx, ws, testAuthNS)).To(Succeed())

	role := &rbacv1.Role{}
	g.Expect(r.Get(ctx, types.NamespacedName{Name: privacyReaderRoleName(testAuthNS), Namespace: testAuthNS}, role)).To(Succeed())
	g.Expect(role.Rules).To(HaveLen(1))
	g.Expect(role.Rules[0].APIGroups).To(ConsistOf(omniaAPIGroup))
	g.Expect(role.Rules[0].Resources).To(ConsistOf(resSessionPrivacyPolicies, resAgentRuntimes))
	g.Expect(role.Rules[0].Verbs).To(ConsistOf(verbList, verbWatch))
	// Owner-referenced to the Workspace so it is GC'd with it.
	g.Expect(role.OwnerReferences).To(HaveLen(1))
	g.Expect(role.OwnerReferences[0].Name).To(Equal("acme"))

	// Idempotent: a second call (e.g. the next service group) re-ensures it.
	g.Expect(r.reconcilePrivacyReaderRole(ctx, ws, testAuthNS)).To(Succeed())
}

// TestPrivacyReaderBinding_CreatesNamespacedRoleBinding verifies the RoleBinding
// ties the SA to the namespaced privacy-reader Role.
func TestPrivacyReaderBinding_CreatesNamespacedRoleBinding(t *testing.T) {
	g := NewWithT(t)
	r := newAuthReconciler(t, ServiceAuthConfig{Enabled: false})
	ws := newTestWorkspace("acme", testAuthNS, nil)
	ctx := context.Background()

	g.Expect(r.reconcilePrivacyReaderBinding(ctx, ws, testAuthNS, testSessionSAForPrivacy)).To(Succeed())

	rb := &rbacv1.RoleBinding{}
	name := privacyReaderBindingName(testSessionSAForPrivacy)
	g.Expect(r.Get(ctx, types.NamespacedName{Name: name, Namespace: testAuthNS}, rb)).To(Succeed())
	g.Expect(rb.RoleRef.Kind).To(Equal(kindRole))
	g.Expect(rb.RoleRef.Name).To(Equal(privacyReaderRoleName(testAuthNS)))
	g.Expect(rb.Subjects).To(HaveLen(1))
	g.Expect(rb.Subjects[0].Name).To(Equal(testSessionSAForPrivacy))
	g.Expect(rb.Subjects[0].Namespace).To(Equal(testAuthNS))

	// Idempotent.
	g.Expect(r.reconcilePrivacyReaderBinding(ctx, ws, testAuthNS, testSessionSAForPrivacy)).To(Succeed())
}
