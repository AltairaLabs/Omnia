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
	"net/http"

	"sigs.k8s.io/controller-runtime/pkg/client"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// handleToolRegistries lists all ToolRegistries.
func (s *Server) handleToolRegistries(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, errMethodNotAllowed)
		return
	}

	namespace := r.URL.Query().Get("namespace")

	var registries omniav1alpha1.ToolRegistryList
	var err error

	if namespace != "" {
		err = s.client.List(r.Context(), &registries, client.InNamespace(namespace))
	} else {
		err = s.client.List(r.Context(), &registries)
	}

	if err != nil {
		s.log.Error(err, "failed to list toolregistries")
		s.writeError(w, http.StatusInternalServerError, "failed to list toolregistries")
		return
	}

	s.writeJSON(w, http.StatusOK, registries.Items)
}

// handleToolRegistry gets a specific ToolRegistry.
func (s *Server) handleToolRegistry(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, errMethodNotAllowed)
		return
	}

	namespace, name, ok := parseNamespaceName(r.URL.Path, "/api/v1/toolregistries")
	if !ok {
		s.writeError(w, http.StatusBadRequest, "invalid path, expected /api/v1/toolregistries/{namespace}/{name}")
		return
	}

	var registry omniav1alpha1.ToolRegistry
	if err := s.client.Get(r.Context(), client.ObjectKey{Namespace: namespace, Name: name}, &registry); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.writeError(w, http.StatusNotFound, "toolregistry not found")
			return
		}
		s.log.Error(err, "failed to get toolregistry", "namespace", namespace, "name", name)
		s.writeError(w, http.StatusInternalServerError, "failed to get toolregistry")
		return
	}

	s.writeJSON(w, http.StatusOK, registry)
}

// handleProviders lists all Providers.
func (s *Server) handleProviders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, errMethodNotAllowed)
		return
	}

	namespace := r.URL.Query().Get("namespace")

	var providers omniav1alpha1.ProviderList
	var err error

	if namespace != "" {
		err = s.client.List(r.Context(), &providers, client.InNamespace(namespace))
	} else {
		err = s.client.List(r.Context(), &providers)
	}

	if err != nil {
		s.log.Error(err, "failed to list providers")
		s.writeError(w, http.StatusInternalServerError, "failed to list providers")
		return
	}

	s.writeJSON(w, http.StatusOK, providers.Items)
}
