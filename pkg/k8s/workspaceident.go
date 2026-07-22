package k8s

import (
	"fmt"
	"os"
)

// EnvWorkspaceName carries the Workspace CR's metadata.name, injected by the
// operator. It is the workspace NAME (e.g. "demo"), never the namespace that
// workspace owns (e.g. "omnia-demo"). The distinction is load-bearing: RBAC
// resourceNames match a cluster-scoped object's own name, so a namespace here
// fails closed and silently (#1875).
const EnvWorkspaceName = "OMNIA_WORKSPACE_NAME"

// WorkspaceNameFromEnvOrLabels resolves the workspace a pod belongs to without
// inferring it from the pod's namespace.
//
// The operator is the only component that authoritatively knows which Workspace
// owns a namespace, so it pushes the name in via EnvWorkspaceName. That is the
// primary source for every caller.
//
// The labels fallback reads the same value off an AgentRuntime's
// workspaceLabel. It only applies to callers that have an AgentRuntime in hand
// — in practice the runtime, which passes ar.Labels and gains a rolling-upgrade
// safety net for pods scheduled before the operator began injecting the env
// var. The facade, arena-eval-worker and arena-dev-console pass nil: the first
// has no AgentRuntime loaded at this point, and the latter two are not agent
// pods at all. For them EnvWorkspaceName is the only source, and its absence
// degrades the same way a service-discovery failure does.
//
// With neither source available it returns an error naming both rather than
// guessing — a wrong workspace name yields an RBAC denial that surfaces as an
// opaque startup failure.
func WorkspaceNameFromEnvOrLabels(labels map[string]string) (string, error) {
	if name := os.Getenv(EnvWorkspaceName); name != "" {
		return name, nil
	}
	if name := labels[workspaceLabel]; name != "" {
		return name, nil
	}
	return "", fmt.Errorf("workspace name unavailable: %s unset and %q label absent",
		EnvWorkspaceName, workspaceLabel)
}
