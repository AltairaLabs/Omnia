package k8s

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Throughout: the workspace is named "demo" and owns the namespace
// "omnia-demo". Nothing here may return the namespace — conflating the two is
// the bug class this helper exists to prevent (#1875).

func TestWorkspaceNameFromEnvOrLabels_PrefersEnv(t *testing.T) {
	t.Setenv(EnvWorkspaceName, "demo")

	got, err := WorkspaceNameFromEnvOrLabels(map[string]string{workspaceLabel: "other"})

	require.NoError(t, err)
	assert.Equal(t, "demo", got)
}

func TestWorkspaceNameFromEnvOrLabels_FallsBackToLabel(t *testing.T) {
	t.Setenv(EnvWorkspaceName, "")

	got, err := WorkspaceNameFromEnvOrLabels(map[string]string{workspaceLabel: "demo"})

	require.NoError(t, err)
	assert.Equal(t, "demo", got)
}

func TestWorkspaceNameFromEnvOrLabels_ErrorsWhenNeitherSet(t *testing.T) {
	t.Setenv(EnvWorkspaceName, "")

	_, err := WorkspaceNameFromEnvOrLabels(nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), EnvWorkspaceName)
	assert.Contains(t, err.Error(), workspaceLabel)
}

// The namespace is never a valid source for the workspace name. This asserts
// the helper has no namespace-shaped fallback: a pod in "omnia-demo" with no
// env var and no label gets an error, not "omnia-demo".
func TestWorkspaceNameFromEnvOrLabels_NeverInfersFromNamespace(t *testing.T) {
	t.Setenv(EnvWorkspaceName, "")

	got, err := WorkspaceNameFromEnvOrLabels(map[string]string{"kubernetes.io/metadata.name": "omnia-demo"})

	require.Error(t, err)
	assert.Empty(t, got)
}
