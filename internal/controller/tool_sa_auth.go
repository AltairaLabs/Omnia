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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

const (
	// toolSATokenVolumeName is the single projected-token volume carrying every
	// serviceAccount-auth handler's audience-bound token (one projection each,
	// under a per-handler subpath).
	toolSATokenVolumeName = "tool-sa-tokens"
	// toolSATokenMountBase is where the projected tool SA tokens are mounted in
	// the runtime container. Each handler's token lives at <base>/<handler>/token.
	toolSATokenMountBase = "/var/run/secrets/omnia/tool-sa"
	// toolSATokenFileName is the projected token file name within a handler subpath.
	toolSATokenFileName = "token"
	// toolSATokenExpirationSeconds is the projected token lifetime; the kubelet
	// rotates it before expiry.
	toolSATokenExpirationSeconds = 3600
)

// toolSAHandler pairs a handler name with the audience its projected token binds to.
type toolSAHandler struct {
	handler  string
	audience string
}

// collectToolSAHandlers returns, in handler order, every handler whose effective
// auth is a serviceAccount projected token.
func collectToolSAHandlers(tr *omniav1alpha1.ToolRegistry) []toolSAHandler {
	if tr == nil {
		return nil
	}
	var out []toolSAHandler
	for i := range tr.Spec.Handlers {
		h := &tr.Spec.Handlers[i]
		auth := h.EffectiveAuth()
		if auth == nil || auth.Type != omniav1alpha1.ToolAuthTypeServiceAccount || auth.ServiceAccount == nil {
			continue
		}
		out = append(out, toolSAHandler{handler: h.Name, audience: auth.ServiceAccount.Audience})
	}
	return out
}

// toolSATokenPath returns the runtime-container path of a handler's projected SA token.
func toolSATokenPath(handler string) string {
	return toolSATokenMountBase + "/" + handler + "/" + toolSATokenFileName
}

// toolSATokenVolume returns the single projected-token volume for all
// serviceAccount handlers, and false when there are none. Each handler gets its
// own audience-bound ServiceAccountToken projection at the subpath <handler>/token.
func toolSATokenVolume(tr *omniav1alpha1.ToolRegistry) (corev1.Volume, bool) {
	sas := collectToolSAHandlers(tr)
	if len(sas) == 0 {
		return corev1.Volume{}, false
	}
	sources := make([]corev1.VolumeProjection, 0, len(sas))
	for _, sa := range sas {
		sources = append(sources, corev1.VolumeProjection{
			ServiceAccountToken: &corev1.ServiceAccountTokenProjection{
				Path:              sa.handler + "/" + toolSATokenFileName,
				Audience:          sa.audience,
				ExpirationSeconds: ptr.To(int64(toolSATokenExpirationSeconds)),
			},
		})
	}
	return corev1.Volume{
		Name:         toolSATokenVolumeName,
		VolumeSource: corev1.VolumeSource{Projected: &corev1.ProjectedVolumeSource{Sources: sources}},
	}, true
}

// toolSATokenMount returns the read-only mount for the projected tool SA tokens.
func toolSATokenMount() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      toolSATokenVolumeName,
		MountPath: toolSATokenMountBase,
		ReadOnly:  true,
	}
}
