/*
Copyright 2025.

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
	"fmt"
	"testing"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func TestCleanupHPA_DeleteError(t *testing.T) {
	scheme := newTestScheme(t)
	_ = autoscalingv2.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Delete: func(_ context.Context, _ client.WithWatch, _ client.Object, _ ...client.DeleteOption) error {
				return fmt.Errorf("connection refused")
			},
		}).
		Build()

	r := &AgentRuntimeReconciler{Client: fakeClient}

	agentRuntime := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
	}

	err := r.cleanupHPA(t.Context(), agentRuntime)
	if err == nil {
		t.Fatal("expected error from cleanupHPA")
	}
}

func TestCleanupKEDA_DeleteError(t *testing.T) {
	scheme := newTestScheme(t)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Delete: func(_ context.Context, _ client.WithWatch, _ client.Object, _ ...client.DeleteOption) error {
				return fmt.Errorf("connection refused")
			},
		}).
		Build()

	r := &AgentRuntimeReconciler{Client: fakeClient}

	agentRuntime := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
	}

	err := r.cleanupKEDA(t.Context(), agentRuntime)
	if err == nil {
		t.Fatal("expected error from cleanupKEDA")
	}
}

func TestCleanupKEDA_DeleteSuccess(t *testing.T) {
	scheme := newTestScheme(t)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Delete: func(_ context.Context, _ client.WithWatch, _ client.Object, _ ...client.DeleteOption) error {
				return nil // simulate successful deletion
			},
		}).
		Build()

	r := &AgentRuntimeReconciler{Client: fakeClient}

	agentRuntime := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
	}

	err := r.cleanupKEDA(t.Context(), agentRuntime)
	if err != nil {
		t.Fatalf("unexpected error from cleanupKEDA: %v", err)
	}
}
