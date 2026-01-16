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

package api

import (
	"encoding/json"
	"net/http"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// handlePromptPacks lists all PromptPacks.
func (s *Server) handlePromptPacks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, errMethodNotAllowed)
		return
	}

	namespace := r.URL.Query().Get("namespace")

	var packs omniav1alpha1.PromptPackList
	var err error

	if namespace != "" {
		err = s.client.List(r.Context(), &packs, client.InNamespace(namespace))
	} else {
		err = s.client.List(r.Context(), &packs)
	}

	if err != nil {
		s.log.Error(err, "failed to list promptpacks")
		s.writeError(w, http.StatusInternalServerError, "failed to list promptpacks")
		return
	}

	s.writeJSON(w, http.StatusOK, packs.Items)
}

// handlePromptPack gets a specific PromptPack or its content.
func (s *Server) handlePromptPack(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, errMethodNotAllowed)
		return
	}

	// Check if this is a content request: /api/v1/promptpacks/{namespace}/{name}/content
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/promptpacks/")
	parts := strings.Split(path, "/")

	if len(parts) == 3 && parts[2] == "content" {
		s.handlePromptPackContent(w, r, parts[0], parts[1])
		return
	}

	namespace, name, ok := parseNamespaceName(r.URL.Path, "/api/v1/promptpacks")
	if !ok {
		s.writeError(w, http.StatusBadRequest, "invalid path, expected /api/v1/promptpacks/{namespace}/{name}")
		return
	}

	var pack omniav1alpha1.PromptPack
	if err := s.client.Get(r.Context(), client.ObjectKey{Namespace: namespace, Name: name}, &pack); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.writeError(w, http.StatusNotFound, "promptpack not found")
			return
		}
		s.log.Error(err, errFailedGetPromptPack, "namespace", namespace, "name", name)
		s.writeError(w, http.StatusInternalServerError, errFailedGetPromptPack)
		return
	}

	s.writeJSON(w, http.StatusOK, pack)
}

// handlePromptPackContent gets the resolved content (pack.json) of a PromptPack.
func (s *Server) handlePromptPackContent(w http.ResponseWriter, r *http.Request, namespace, name string) {
	// Get the PromptPack to find the source ConfigMap
	var pack omniav1alpha1.PromptPack
	if err := s.client.Get(r.Context(), client.ObjectKey{Namespace: namespace, Name: name}, &pack); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.writeError(w, http.StatusNotFound, "promptpack not found")
			return
		}
		s.log.Error(err, errFailedGetPromptPack, "namespace", namespace, "name", name)
		s.writeError(w, http.StatusInternalServerError, errFailedGetPromptPack)
		return
	}

	// Check if source is a ConfigMap
	if pack.Spec.Source.Type != "configmap" || pack.Spec.Source.ConfigMapRef == nil {
		s.writeError(w, http.StatusBadRequest, "promptpack source is not a configmap")
		return
	}

	// Get the ConfigMap
	var cm corev1.ConfigMap
	cmName := pack.Spec.Source.ConfigMapRef.Name
	if err := s.client.Get(r.Context(), client.ObjectKey{Namespace: namespace, Name: cmName}, &cm); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.writeError(w, http.StatusNotFound, "configmap not found")
			return
		}
		s.log.Error(err, "failed to get configmap", "namespace", namespace, "name", cmName)
		s.writeError(w, http.StatusInternalServerError, "failed to get configmap")
		return
	}

	// Get the pack.json content (default key)
	content, ok := cm.Data["pack.json"]
	if !ok {
		s.writeError(w, http.StatusNotFound, "pack.json not found in configmap")
		return
	}

	// Parse and return the JSON content
	var packContent map[string]interface{}
	if err := json.Unmarshal([]byte(content), &packContent); err != nil {
		s.log.Error(err, "failed to parse pack.json")
		s.writeError(w, http.StatusInternalServerError, "failed to parse pack.json")
		return
	}

	s.writeJSON(w, http.StatusOK, packContent)
}
