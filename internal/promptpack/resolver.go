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
	"github.com/altairalabs/omnia/internal/promptpack/packselect"
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

// Load returns the raw pack.json bytes for packName in namespace, selecting
// the version by exact match (version non-empty) or by stable-channel-max
// (version empty). It resolves the PromptPack CR by the packName label — NOT
// by object name, since metadata.name is a deterministic pp-<hash> that never
// equals packName — and then resolves spec.source; callers must not read the
// backing store directly.
func (r *Resolver) Load(ctx context.Context, namespace, packName, version string) ([]byte, error) {
	var list omniav1alpha1.PromptPackList
	if err := r.client.List(ctx, &list, client.InNamespace(namespace), client.MatchingLabels{packselect.Label: packName}); err != nil {
		return nil, fmt.Errorf("list PromptPacks %s/%s: %w", namespace, packName, err)
	}

	pp, err := selectPromptPack(list.Items, packName, version)
	if err != nil {
		return nil, err
	}

	switch pp.Spec.Source.Type {
	case omniav1alpha1.PromptPackSourceTypeConfigMap:
		return r.loadConfigMap(ctx, pp)
	default:
		return nil, fmt.Errorf("PromptPack %s/%s: unsupported source type %q",
			namespace, pp.Name, pp.Spec.Source.Type)
	}
}

// selectPromptPack picks one PromptPack from candidates (all sharing packName,
// already filtered by the packselect.Label List call) by exact version match,
// or — when version is empty — the highest stable (non-prerelease) semver via
// packselect.ChannelMax. This applies even when there is exactly one candidate:
// a lone prerelease-only pack must NOT be returned as the implicit default,
// consistent with the operator's stable-channel default (#1837 review). It wraps
// packselect with resolver-specific error wording that callers/tests rely on.
func selectPromptPack(candidates []omniav1alpha1.PromptPack, packName, version string) (*omniav1alpha1.PromptPack, error) {
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no PromptPack found for packName %q", packName)
	}
	if version != "" {
		for i := range candidates {
			if packselect.VersionsEqual(candidates[i].Spec.Version, version) {
				return &candidates[i], nil
			}
		}
		return nil, fmt.Errorf("no PromptPack matches packName %q version %q", packName, version)
	}
	pp, err := packselect.ChannelMax(candidates, packselect.TrackStable)
	if err != nil {
		return nil, fmt.Errorf("no stable PromptPack version found for packName %q", packName)
	}
	return pp, nil
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
