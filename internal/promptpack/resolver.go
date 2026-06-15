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

// Package promptpack resolves a PromptPack's compiled content (pack.json) from
// its CRD, hiding where and how that content is stored.
//
// A PromptPack is a Kubernetes resource whose spec.source declares where its
// content lives. Today the only source type is "configmap" (the content sits in
// a ConfigMap referenced by spec.source.configMapRef), but that is an
// implementation detail. Callers MUST go through this resolver and the
// PromptPack CR — never reach for the backing ConfigMap (or any future store)
// directly, and never assume the ConfigMap is named after the pack. New source
// types (git, OCI, …) are added here, once, so callers never change.
package promptpack

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// packJSONKey is the ConfigMap data key holding the compiled pack.json. It is an
// implementation detail of the configmap source type and is never exposed to
// callers — Load returns raw bytes.
const packJSONKey = "pack.json"

// Resolver loads PromptPack content via the PromptPack CR.
type Resolver struct {
	client client.Client
}

// NewResolver returns a Resolver backed by the given Kubernetes client. The
// client must be able to get PromptPack CRs and whatever a source type needs
// (e.g. ConfigMaps for the configmap source).
func NewResolver(c client.Client) *Resolver {
	return &Resolver{client: c}
}

// Load returns the raw pack.json bytes for the named PromptPack in namespace. It
// reads the PromptPack CR and resolves spec.source; callers must not read the
// backing store directly.
func (r *Resolver) Load(ctx context.Context, namespace, name string) ([]byte, error) {
	pp := &omniav1alpha1.PromptPack{}
	if err := r.client.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, pp); err != nil {
		return nil, fmt.Errorf("get PromptPack %s/%s: %w", namespace, name, err)
	}

	switch pp.Spec.Source.Type {
	case omniav1alpha1.PromptPackSourceTypeConfigMap:
		return r.loadConfigMap(ctx, pp)
	default:
		return nil, fmt.Errorf("PromptPack %s/%s: unsupported source type %q",
			namespace, name, pp.Spec.Source.Type)
	}
}

// loadConfigMap resolves a configmap-source PromptPack: it follows
// spec.source.configMapRef (NOT the pack name) to the backing ConfigMap and
// returns its pack.json bytes.
func (r *Resolver) loadConfigMap(ctx context.Context, pp *omniav1alpha1.PromptPack) ([]byte, error) {
	if pp.Spec.Source.ConfigMapRef == nil {
		return nil, fmt.Errorf("PromptPack %s/%s: configmap source has no configMapRef",
			pp.Namespace, pp.Name)
	}

	cmName := pp.Spec.Source.ConfigMapRef.Name
	cm := &corev1.ConfigMap{}
	if err := r.client.Get(ctx, types.NamespacedName{Name: cmName, Namespace: pp.Namespace}, cm); err != nil {
		return nil, fmt.Errorf("get PromptPack %s/%s ConfigMap %s: %w", pp.Namespace, pp.Name, cmName, err)
	}

	raw, ok := cm.Data[packJSONKey]
	if !ok {
		return nil, fmt.Errorf("PromptPack %s/%s ConfigMap %s missing %q key",
			pp.Namespace, pp.Name, cmName, packJSONKey)
	}
	return []byte(raw), nil
}
