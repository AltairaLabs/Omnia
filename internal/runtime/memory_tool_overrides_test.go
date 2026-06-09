/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0

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

package runtime

import (
	"context"
	"encoding/json"
	"testing"

	pkmemory "github.com/AltairaLabs/PromptKit/runtime/memory"
	"github.com/AltairaLabs/PromptKit/runtime/tools"
	"github.com/AltairaLabs/PromptKit/sdk"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/memory/metakeys"
)

// baseMemoryDescriptors registers the real PromptKit memory tools and returns
// them by name, so the override tests run against the actual upstream base
// descriptors (mock-to-contract) rather than a hand-rolled approximation.
func baseMemoryDescriptors(t *testing.T) map[string]*tools.ToolDescriptor {
	t.Helper()
	reg := tools.NewRegistry()
	pkmemory.RegisterMemoryTools(reg)

	out := map[string]*tools.ToolDescriptor{}
	for _, name := range []string{
		pkmemory.RememberToolName,
		pkmemory.RecallToolName,
		pkmemory.ListToolName,
		pkmemory.ForgetToolName,
	} {
		desc := reg.Get(name)
		require.NotNil(t, desc, "PromptKit should register %s", name)
		out[name] = desc
	}
	return out
}

func schemaProps(t *testing.T, schema json.RawMessage) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal(schema, &m))
	props, ok := m["properties"].(map[string]any)
	require.True(t, ok, "schema has a properties object")
	return props
}

func TestMemoryToolPatches(t *testing.T) {
	patches := memoryToolPatches()

	names := make([]string, 0, len(patches))
	for _, p := range patches {
		names = append(names, p.name)
		assert.NotNil(t, p.fn, "patch %s has a function", p.name)
	}
	assert.ElementsMatch(t, []string{
		pkmemory.RememberToolName,
		pkmemory.RecallToolName,
		pkmemory.ListToolName,
		pkmemory.ForgetToolName,
	}, names)
}

func TestMemoryToolOverrides(t *testing.T) {
	opts := memoryToolOverrides()
	assert.Len(t, opts, 4)
	for i, o := range opts {
		assert.NotNil(t, o, "option %d is non-nil", i)
	}
}

// TestBuildConversationOptions_WiresMemoryToolOverrides verifies the overrides
// are actually appended to the conversation when memory is enabled — guarding
// against the "code exists but isn't wired" failure mode. Enabling memory
// should add WithMemory plus exactly one descriptor override per memory tool;
// everything else about the two servers is identical.
func TestBuildConversationOptions_WiresMemoryToolOverrides(t *testing.T) {
	withMem := NewServer(
		WithLogger(logr.Discard()),
		WithMemoryStore(&fakeSemanticStore{fakeStore: fakeStore{}}),
		WithWorkspaceUID("ws-1"),
	)
	withoutMem := NewServer(WithLogger(logr.Discard()))

	memOpts, err := withMem.buildConversationOptions(context.Background(), "sess-1")
	require.NoError(t, err)
	baseOpts, err := withoutMem.buildConversationOptions(context.Background(), "sess-1")
	require.NoError(t, err)

	assert.Equal(t, 1+len(memoryToolPatches()), len(memOpts)-len(baseOpts),
		"memory wiring should append WithMemory and one override per memory tool")
}

// capturingStore records the last Memory passed to Save so a test can assert
// what the memory executor produced from tool-call args.
type capturingStore struct {
	fakeStore
	saved *pkmemory.Memory
}

func (c *capturingStore) Save(_ context.Context, m *pkmemory.Memory) error {
	c.saved = m
	return nil
}

// TestRememberPassthrough_LandsExtraArgsInMetadata is the end-to-end data-path
// proof: it runs PromptKit's real memory executor on the actually-patched
// remember descriptor with the Omnia extra args the override exposes, and
// asserts they arrive in Memory.Metadata under the exact keys the memory-api
// write path reads (the metakeys constants, shared by both sides). It also
// confirms provenance is auto-set. The only link this can't cover is the LLM
// choosing to supply the args — that needs a real model (in-cluster smoke).
func TestRememberPassthrough_LandsExtraArgsInMetadata(t *testing.T) {
	desc := baseMemoryDescriptors(t)[pkmemory.RememberToolName]
	patchRememberDescriptor(desc) // run against the patched descriptor

	store := &capturingStore{}
	exec := pkmemory.NewExecutor(store, defaultScope())

	args := json.RawMessage(`{` +
		`"content":"My name is Sarah",` +
		`"about_kind":"user_profile",` +
		`"about_key":"u1",` +
		`"purpose":"personalization"}`)

	_, err := exec.Execute(context.Background(), desc, args)
	require.NoError(t, err)

	require.NotNil(t, store.saved, "executor should have called Save")
	md := store.saved.Metadata
	assert.Equal(t, "user_profile", md[metakeys.AboutKind])
	assert.Equal(t, "u1", md[metakeys.AboutKey])
	assert.Equal(t, "personalization", md[metakeys.Purpose])
	// Provenance is auto-set by the remember executor (not LLM-spoofable).
	assert.Equal(t, string(pkmemory.ProvenanceUserRequested), md[pkmemory.MetaKeyProvenance])
}

func TestPatchRememberDescriptor(t *testing.T) {
	desc := baseMemoryDescriptors(t)[pkmemory.RememberToolName]

	patchRememberDescriptor(desc)

	// Description teaches Omnia's dedup + purpose model.
	assert.Contains(t, desc.Description, metakeys.AboutKey)
	assert.Contains(t, desc.Description, metakeys.AboutKind)

	props := schemaProps(t, desc.InputSchema)

	// Base PromptKit fields are preserved.
	assert.Contains(t, props, "content")

	// Omnia metadata fields are added with names that match the MetaKeys the
	// memory-api write path reads (so the extras-passthrough lands them where
	// Save expects them).
	for _, key := range []string{
		metakeys.AboutKind,
		metakeys.AboutKey,
		metakeys.Purpose,
		metakeys.Title,
		metakeys.Summary,
	} {
		assert.Contains(t, props, key, "remember schema exposes %q", key)
	}
}

// TestUpdateGuidance_RememberNotForget guards the fix for the cluster-observed
// bug where the LLM corrected a fact via memory__remember (which supersedes in
// place by about_key) and then ALSO called memory__forget on the same entity,
// deleting the just-updated memory. The forget guidance must steer corrections
// to remember (same about_key) and must NOT tell the model to forget after
// storing a correction.
func TestUpdateGuidance_RememberNotForget(t *testing.T) {
	// forget must point the model at remember for changes/corrections...
	assert.Contains(t, forgetDescription, pkmemory.RememberToolName,
		"forget guidance should steer corrections to memory__remember")
	// ...and must not carry the destructive "store correction then forget" inducement.
	assert.NotContains(t, forgetDescription, "stored the correction",
		"forget guidance must not tell the model to forget after correcting")

	// remember must tell the model that updating in place is the way to change a
	// fact, and that forget is not part of an update.
	assert.Contains(t, rememberDescription, pkmemory.ForgetToolName,
		"remember guidance should warn against using memory__forget to update")
}

func TestPatchDescriptionOnlyTools(t *testing.T) {
	base := baseMemoryDescriptors(t)

	cases := []struct {
		name string
		fn   sdk.ToolDescriptorPatchFn
	}{
		{pkmemory.RecallToolName, patchRecallDescriptor},
		{pkmemory.ListToolName, patchListDescriptor},
		{pkmemory.ForgetToolName, patchForgetDescriptor},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			desc := base[tc.name]
			origSchema := append(json.RawMessage(nil), desc.InputSchema...)

			tc.fn(desc)

			assert.NotEmpty(t, desc.Description)
			// Description-only patches must not disturb the input schema.
			assert.JSONEq(t, string(origSchema), string(desc.InputSchema))
		})
	}
}

func TestAddSchemaProperties(t *testing.T) {
	t.Run("preserves existing and adds new", func(t *testing.T) {
		base := json.RawMessage(`{"type":"object","properties":{"content":{"type":"string"}},"required":["content"]}`)

		out := addSchemaProperties(base, map[string]any{
			"purpose": map[string]any{schemaKeyType: schemaTypeString},
		})

		var m map[string]any
		require.NoError(t, json.Unmarshal(out, &m))
		props := m["properties"].(map[string]any)
		assert.Contains(t, props, "content")
		assert.Contains(t, props, "purpose")
		// required list is untouched.
		assert.Equal(t, []any{"content"}, m["required"])
	})

	t.Run("handles nil schema", func(t *testing.T) {
		out := addSchemaProperties(nil, map[string]any{
			"purpose": map[string]any{schemaKeyType: schemaTypeString},
		})
		var m map[string]any
		require.NoError(t, json.Unmarshal(out, &m))
		assert.Equal(t, schemaTypeObject, m[schemaKeyType])
		props := m["properties"].(map[string]any)
		assert.Contains(t, props, "purpose")
	})
}
