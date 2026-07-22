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

package k8s

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func TestNewFileBackedClient_GetsSeededObjects(t *testing.T) {
	dir := t.TempDir()
	// One file, two docs: an AgentRuntime and its Secret.
	manifest := `apiVersion: omnia.altairalabs.ai/v1alpha1
kind: AgentRuntime
metadata:
  name: demo
  namespace: dev
spec:
  promptPackRef:
    name: demo-pack
---
apiVersion: v1
kind: Secret
metadata:
  name: openai-secret
  namespace: dev
data:
  OPENAI_API_KEY: c2stZGV2
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "agent.yaml"), []byte(manifest), 0o600))

	c, err := newFileBackedClient(dir)
	require.NoError(t, err)

	ar := &omniav1alpha1.AgentRuntime{}
	require.NoError(t, c.Get(context.Background(),
		client.ObjectKey{Name: "demo", Namespace: "dev"}, ar))
	assert.Equal(t, "demo-pack", ar.Spec.PromptPackRef.Name)
}

func TestNewFileBackedClient_BadYAMLErrors(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bad.yaml"),
		[]byte("this: is: not: a: k8s: object\n"), 0o600))
	_, err := newFileBackedClient(dir)
	require.Error(t, err)
}

func TestNewFileBackedClient_FailsFastOnBadPath(t *testing.T) {
	// A path that does not exist.
	_, err := newFileBackedClient(filepath.Join(t.TempDir(), "does-not-exist"))
	require.Error(t, err)

	// A path that is a file, not a directory.
	f := filepath.Join(t.TempDir(), "a-file.yaml")
	require.NoError(t, os.WriteFile(f, []byte("{}"), 0o600))
	_, err = newFileBackedClient(f)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a directory")
}

func TestNewFileBackedClient_EmptyDirIsEmptyClient(t *testing.T) {
	c, err := newFileBackedClient(t.TempDir())
	require.NoError(t, err)
	var list omniav1alpha1.AgentRuntimeList
	require.NoError(t, c.List(context.Background(), &list))
	assert.Empty(t, list.Items)
}

// TestNewFileBackedClient_LoadsExampleDevroot guards the shipped example: the
// manifests in examples/custom-runtime/devroot must stay valid against the
// scheme and seed a usable client.
func TestNewFileBackedClient_LoadsExampleDevroot(t *testing.T) {
	c, err := newFileBackedClient("../../examples/custom-runtime/devroot")
	require.NoError(t, err)

	ar := &omniav1alpha1.AgentRuntime{}
	require.NoError(t, c.Get(context.Background(),
		client.ObjectKey{Name: "demo", Namespace: "dev"}, ar))
	require.Len(t, ar.Spec.Providers, 1)
	assert.Equal(t, "mock-provider", ar.Spec.Providers[0].ProviderRef.Name)
}

func TestNewClient_FileBackedWhenConfigDirSet(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ar.yaml"), []byte(
		`apiVersion: omnia.altairalabs.ai/v1alpha1
kind: AgentRuntime
metadata: {name: local, namespace: dev}
spec: {promptPackRef: {name: p}}
`), 0o600))
	t.Setenv(envConfigDir, dir)

	c, err := NewClient()
	require.NoError(t, err)

	ar := &omniav1alpha1.AgentRuntime{}
	require.NoError(t, c.Get(context.Background(),
		client.ObjectKey{Name: "local", Namespace: "dev"}, ar))
	assert.Equal(t, "p", ar.Spec.PromptPackRef.Name)
}
