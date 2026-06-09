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
	"encoding/json"

	pkmemory "github.com/AltairaLabs/PromptKit/runtime/memory"
	"github.com/AltairaLabs/PromptKit/runtime/tools"
	"github.com/AltairaLabs/PromptKit/sdk"

	"github.com/altairalabs/omnia/internal/memory/metakeys"
)

// PromptKit registers generic memory tools (memory__remember / recall / list /
// forget) that know nothing about Omnia's memory model: the tiered store, the
// structured-dedup `about` key, purpose tags, or the title/summary split for
// large memories. PromptKit can't know any of this — it has no view of Omnia's
// schema or config. The integration point is the tool-calling instructions:
// sdk.WithToolDescriptorOverride lets us rewrite each tool's description and,
// for remember, extend its input schema. Any extra top-level args the LLM
// supplies are merged by PromptKit into Memory.Metadata, where the memory-api
// write path already reads them (about_kind/about_key/purpose/title/summary).
//
// This keeps Omnia on PromptKit's standard memory path — no forked tools, no
// memory-api changes — while teaching the LLM how to use the store well.

// memoryToolPatch pairs a registered memory tool name with the descriptor
// patch that injects Omnia's guidance.
type memoryToolPatch struct {
	name string
	fn   sdk.ToolDescriptorPatchFn
}

// memoryToolPatches returns the Omnia descriptor patches for the PromptKit
// memory tools, in a form that is unit-testable without an SDK conversation.
func memoryToolPatches() []memoryToolPatch {
	return []memoryToolPatch{
		{pkmemory.RememberToolName, patchRememberDescriptor},
		{pkmemory.RecallToolName, patchRecallDescriptor},
		{pkmemory.ListToolName, patchListDescriptor},
		{pkmemory.ForgetToolName, patchForgetDescriptor},
	}
}

// memoryToolOverrides wraps memoryToolPatches as SDK options appended to the
// conversation alongside sdk.WithMemory. Overrides for tools that aren't
// registered (e.g. version skew) are skipped by the SDK, so this is safe to
// pass unconditionally whenever memory is enabled.
func memoryToolOverrides() []sdk.Option {
	patches := memoryToolPatches()
	opts := make([]sdk.Option, 0, len(patches))
	for _, p := range patches {
		opts = append(opts, sdk.WithToolDescriptorOverride(p.name, p.fn))
	}
	return opts
}

// Descriptions injected into the memory tool descriptors. These are the
// instructions the LLM reads when deciding whether and how to call each tool.
const (
	rememberDescription = "Store or update a durable fact in Omnia's tiered memory so it is available in " +
		"future conversations. Call this when the user asks you to remember something, states a stable " +
		"preference, or shares a fact worth recalling later.\n\n" +
		"Identify what the memory is ABOUT with a stable key:\n" +
		"  about_kind — the subject category (e.g. 'user_profile', 'project', 'preference')\n" +
		"  about_key  — a stable id within that kind (e.g. the user id, a project name, 'seat_preference')\n" +
		"Reusing the same about_kind + about_key replaces the existing memory in place. This is how you " +
		"CHANGE or CORRECT a fact: to update a preference or status, just call memory__remember again with " +
		"the new value and the same about_kind + about_key. Do NOT call memory__forget to change a fact — " +
		"forget deletes it and the correction is lost. Always set about_kind + about_key for facts that " +
		"change over time (names, preferences, statuses).\n\n" +
		"Optional fields:\n" +
		"  purpose  — why this is stored (e.g. 'personalization', 'support_context'); drives retention\n" +
		"  category — consent category for retention/redaction policy (see values)\n" +
		"  title + summary — for long content, give a short title and one-line summary; keep the full text in content."

	recallDescription = "Search Omnia memory for relevant facts before answering. Recall spans every tier " +
		"visible to the current user — workspace-wide institutional knowledge, this agent's curated " +
		"knowledge, and the user's own memories — so one query surfaces all of them. Use it proactively " +
		"whenever the user references something they expect you to know (their name, prior decisions, " +
		"preferences, project context)."

	listDescription = "List stored memories in the current scope, newest first, optionally filtered by type. " +
		"Use memory__recall for relevance-ranked search; use this to browse what exists — for example to " +
		"find a memory's id before forgetting it."

	forgetDescription = "Permanently remove a fact from memory by its id (a soft delete: it stops being " +
		"recalled). Use this ONLY to delete a fact entirely — for example when the user asks you to forget " +
		"something. Do NOT use forget to change or correct a fact: to update a value, call memory__remember " +
		"with the same about_kind + about_key, which replaces the old value in place. Get the id from a " +
		"prior memory__recall or memory__list result."
)

// patchRememberDescriptor rewrites memory__remember's instructions and extends
// its input schema with the Omnia metadata fields. The added property names
// match the MetaKeys the memory-api write path reads, so PromptKit's
// extras-passthrough routes them into Memory.Metadata where Save expects them.
func patchRememberDescriptor(d *tools.ToolDescriptor) {
	d.Description = rememberDescription
	d.InputSchema = addSchemaProperties(d.InputSchema, map[string]any{
		metakeys.AboutKind: stringProp("Stable subject category for dedup, e.g. 'user_profile', 'project', 'preference'."),
		metakeys.AboutKey:  stringProp("Stable identifier within about_kind, e.g. the user id or a project name. Reuse to update in place."),
		metakeys.Purpose:   stringProp("Why this is stored (e.g. 'personalization', 'support_context'); influences retention."),
		metakeys.Title:     stringProp("Short title for a long memory. Optional; the full text stays in content."),
		metakeys.Summary:   stringProp("One-line summary for a long memory. Optional; the full text stays in content."),
	})
}

// JSON Schema vocabulary used when extending the remember tool schema.
const (
	schemaKeyType    = "type"
	schemaTypeString = "string"
	schemaTypeObject = "object"
)

// stringProp builds a JSON-Schema string property with a description.
func stringProp(description string) map[string]any {
	return map[string]any{schemaKeyType: schemaTypeString, "description": description}
}

func patchRecallDescriptor(d *tools.ToolDescriptor) { d.Description = recallDescription }
func patchListDescriptor(d *tools.ToolDescriptor)   { d.Description = listDescription }
func patchForgetDescriptor(d *tools.ToolDescriptor) { d.Description = forgetDescription }

// addSchemaProperties returns a copy of the given JSON-Schema object with the
// supplied properties merged into its "properties" map. Existing properties
// and sibling keys (type, required, ...) are preserved. A nil/invalid input
// schema yields a fresh object schema carrying just the new properties.
func addSchemaProperties(schema json.RawMessage, props map[string]any) json.RawMessage {
	var root map[string]any
	if len(schema) > 0 {
		if err := json.Unmarshal(schema, &root); err != nil {
			root = nil
		}
	}
	if root == nil {
		root = map[string]any{schemaKeyType: schemaTypeObject}
	}

	existing, ok := root["properties"].(map[string]any)
	if !ok || existing == nil {
		existing = map[string]any{}
	}
	for k, v := range props {
		existing[k] = v
	}
	root["properties"] = existing

	out, err := json.Marshal(root)
	if err != nil {
		return schema // keep the original schema if re-encoding fails
	}
	return out
}
