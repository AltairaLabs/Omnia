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
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// TestReconcile_LabelUpdateErrorPropagates covers the error path of the
// promptpack resolution-index label reconciliation: when persisting the
// label fails, Reconcile must surface the error rather than continuing.
func TestReconcile_LabelUpdateErrorPropagates(t *testing.T) {
	scheme := newTestScheme(t)

	pack := &omniav1alpha1.PromptPack{
		ObjectMeta: metav1.ObjectMeta{Name: "pp-label-err", Namespace: "default"},
		Spec: omniav1alpha1.PromptPackSpec{
			PackName: "mypack",
			Version:  "1.0.0",
			Source: omniav1alpha1.PromptPackContentSource{
				Type:         omniav1alpha1.PromptPackSourceTypeConfigMap,
				ConfigMapRef: &corev1.LocalObjectReference{Name: "cm"},
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pack).
		WithInterceptorFuncs(interceptor.Funcs{
			Update: func(context.Context, client.WithWatch, client.Object, ...client.UpdateOption) error {
				return errors.New("update boom")
			},
		}).Build()

	r := &PromptPackReconciler{Client: c, Scheme: scheme}
	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: pack.Name, Namespace: pack.Namespace},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "update boom")
}
