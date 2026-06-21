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
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const testMemoryEnterpriseReaderRole = "omnia-memory-enterprise-reader"
const testMemoryEnterpriseReaderCRBName = "memory-enterprise-reader-acme-ns-memory-acme-default"

func TestMemoryEnterpriseReaderBinding_NoClusterRoleNameIsNoop(t *testing.T) {
	g := NewWithT(t)
	r := newAuthReconciler(t, ServiceAuthConfig{Enabled: true})
	r.MemoryEnterpriseReaderClusterRole = ""

	g.Expect(r.reconcileMemoryEnterpriseReaderBinding(context.Background(), testAuthNS, testMemorySAName)).To(Succeed())
	assertNoMemoryEnterpriseReaderBinding(t, r.Client)
}

func TestMemoryEnterpriseReaderBinding_CreatesClusterRoleBinding(t *testing.T) {
	g := NewWithT(t)
	r := newAuthReconciler(t, ServiceAuthConfig{Enabled: false}) // independent of session auth
	r.MemoryEnterpriseReaderClusterRole = testMemoryEnterpriseReaderRole
	ctx := context.Background()

	g.Expect(r.reconcileMemoryEnterpriseReaderBinding(ctx, testAuthNS, testMemorySAName)).To(Succeed())

	crb := &rbacv1.ClusterRoleBinding{}
	g.Expect(r.Get(ctx, types.NamespacedName{Name: testMemoryEnterpriseReaderCRBName}, crb)).To(Succeed())
	g.Expect(crb.RoleRef.Name).To(Equal(testMemoryEnterpriseReaderRole))
	g.Expect(crb.RoleRef.Kind).To(Equal("ClusterRole"))
	g.Expect(crb.Subjects).To(HaveLen(1))
	g.Expect(crb.Subjects[0].Kind).To(Equal("ServiceAccount"))
	g.Expect(crb.Subjects[0].Name).To(Equal(testMemorySAName))
	g.Expect(crb.Subjects[0].Namespace).To(Equal(testAuthNS))

	// Idempotent: second call must not error (binding already exists).
	g.Expect(r.reconcileMemoryEnterpriseReaderBinding(ctx, testAuthNS, testMemorySAName)).To(Succeed())
}

func assertNoMemoryEnterpriseReaderBinding(t *testing.T, cl client.Client) {
	t.Helper()
	g := NewWithT(t)
	crb := &rbacv1.ClusterRoleBinding{}
	err := cl.Get(context.Background(), types.NamespacedName{Name: testMemoryEnterpriseReaderCRBName}, crb)
	g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
}
