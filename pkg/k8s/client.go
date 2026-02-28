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

// Package k8s provides a reusable Kubernetes client with Omnia CRD scheme registration.
// It is used by facade, eval worker, and other in-cluster components.
package k8s

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// Scheme returns a runtime.Scheme with corev1 and Omnia CRDs registered.
func Scheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = corev1.AddToScheme(s)
	_ = omniav1alpha1.AddToScheme(s)
	return s
}

// NewClient creates a controller-runtime client with the Omnia CRD scheme registered.
// Uses in-cluster config (service account token).
func NewClient() (client.Client, error) {
	cfg, err := ctrl.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("get k8s config: %w", err)
	}
	return NewClientWithConfig(cfg)
}

// NewClientWithConfig creates a client from an explicit rest.Config.
func NewClientWithConfig(cfg *rest.Config) (client.Client, error) {
	c, err := client.New(cfg, client.Options{Scheme: Scheme()})
	if err != nil {
		return nil, fmt.Errorf("create k8s client: %w", err)
	}
	return c, nil
}
