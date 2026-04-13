/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package v1alpha1

// SkillRef selects content from a SkillSource for a PromptPack.
type SkillRef struct {
	// source is the name of a SkillSource in the same namespace as the
	// PromptPack. Cross-namespace references are not supported.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Source string `json:"source"`

	// include narrows the set of skills exposed from the source to those
	// whose SKILL.md frontmatter name matches one of the entries. Empty =
	// all skills the source has synced (after its own filter).
	// +optional
	Include []string `json:"include,omitempty"`

	// mountAs renames the group under which these skills are exposed to
	// the runtime. Defaults to the source's targetPath basename. Used to
	// give PromptPack workflow states a stable "skills: ./skills/<group>"
	// path that doesn't depend on the upstream directory layout.
	// +optional
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`
	MountAs string `json:"mountAs,omitempty"`
}

// SkillSelector names a PromptKit skill selector strategy.
// +kubebuilder:validation:Enum=model-driven;tag;embedding
type SkillSelector string

const (
	// SkillSelectorModelDriven is the default — the LLM decides which
	// skills to activate based on the Phase-1 discovery index.
	SkillSelectorModelDriven SkillSelector = "model-driven"
	// SkillSelectorTag pre-filters by frontmatter metadata tags.
	SkillSelectorTag SkillSelector = "tag"
	// SkillSelectorEmbedding performs RAG-based selection for large skill sets.
	SkillSelectorEmbedding SkillSelector = "embedding"
)

// SkillsConfig tunes PromptKit's skill runtime for a PromptPack.
type SkillsConfig struct {
	// maxActive caps the number of concurrently active skills.
	// Defaults to PromptKit's default (5).
	// +kubebuilder:validation:Minimum=1
	// +optional
	MaxActive *int32 `json:"maxActive,omitempty"`

	// selector picks the skill selection strategy.
	// +kubebuilder:default="model-driven"
	// +optional
	Selector SkillSelector `json:"selector,omitempty"`
}

// PromptPack skill-related condition types.
const (
	// PromptPackConditionSkillsResolved is True when every SkillRef in
	// spec.skills names an existing SkillSource in the pack's namespace.
	PromptPackConditionSkillsResolved = "SkillsResolved"
	// PromptPackConditionSkillsValid is True when the post-include skill
	// set has no name collisions across sources.
	PromptPackConditionSkillsValid = "SkillsValid"
	// PromptPackConditionSkillToolsResolved is True when every resolved
	// SKILL.md's allowed-tools set is a subset of the pack's declared
	// tools (plus any referenced ToolRegistry).
	PromptPackConditionSkillToolsResolved = "SkillToolsResolved"
)
