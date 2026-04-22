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
	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// projectLegacyA2AAuth is a thin wrapper around v1alpha1.ProjectLegacyA2AAuth.
// The canonical implementation now lives in api/v1alpha1 so cmd/agent's
// startup chain builder can share the same logic (otherwise legacy CRs
// without spec.externalAuth would silently 401 at the facade). See the
// shared helper for precedence rules and rationale.
func projectLegacyA2AAuth(ar *omniav1alpha1.AgentRuntime) {
	omniav1alpha1.ProjectLegacyA2AAuth(ar)
}
