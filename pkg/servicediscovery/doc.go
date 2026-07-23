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

// Package servicediscovery resolves per-workspace service endpoints from the
// Workspace CRD.
//
// # Identity is pushed; service config is resolved
//
// The operator injects OMNIA_WORKSPACE_NAME and OMNIA_SERVICE_GROUP because
// only it knows which Workspace owns a namespace — a pod must never infer that.
// The workspace name ("demo") is not the namespace it owns ("omnia-demo"), and
// RBAC resourceNames match the workspace name, so a conflation fails closed and
// silently (#1875).
//
// Everything else — every service endpoint — is read from the Workspace by the
// pod that needs it, taking only the services it requires.
//
// # There is no env fallback for service URLs
//
// An earlier resolveFromEnv silently bypassed the Workspace whenever
// SESSION_API_URL and MEMORY_API_URL were both set. That gave endpoints two
// sources of truth, where the env one could not express what a service group
// carries (redis, retention, the privacy policy ref). It was also unsatisfiable
// in the cases that mattered: it required both URLs, but the facade needs only
// session and a group may legitimately have no memory-api.
//
// PRIVACY_API_URL and MEMORY_API_URLS are gone for the same reason. privacy-api
// is per-workspace like everything else, so the workspace already knows its
// endpoint.
//
// # The boundary: env injection survives only where there is no cluster-scoped read
//
// A pod resolves for itself when it holds a client that can read cluster-scoped
// resources. A namespaced client is not sufficient — Workspace is cluster-scoped,
// so a Role cannot grant it.
//
// Two consumers sit on the other side of that boundary and are injected by
// their controller instead:
//
//   - The demos chart's sharepoint-hero-seed adapter, a third-party image that
//     will never hold Omnia's resolver. Takes MEMORY_API_URL.
//   - Arena job pods (ee/cmd/arena-worker). They build a client, but only for
//     namespaced CRD reads; ee/internal/controller/arena_worker_rbac.go grants a
//     namespaced Role and nothing on workspaces. Self-discovery would mean
//     minting a per-job ClusterRoleBinding to give ephemeral, user-supplied
//     load-test workloads a cluster-scoped read. Takes SESSION_API_URL.
//
// If you are adding an env var for a service endpoint to a pod that can read
// its own Workspace, resolve it here instead.
//
// # Resolve-before-ready
//
// Workspace.status.services is populated after the Workspace itself exists, so
// a consumer resolving at startup must retry rather than fail. Injecting a URL
// from a controller does not avoid this — it moves the resolution to
// pod-creation time, which is strictly earlier and therefore worse. That is how
// the Arena dev console came to crash-loop (#1897).
package servicediscovery
