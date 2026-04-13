# Skills (SkillSource + PromptPack.spec.skills) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship `SkillSource` CRD (core) + `PromptPack.spec.skills` field + runtime wiring so Omnia agents can load AgentSkills.io-formatted skills declared via CRDs, synced via the existing `internal/sourcesync` infrastructure.

**Architecture:** `SkillSource` reuses `internal/sourcesync` to fetch content into the workspace PVC with a post-fetch filter for selective subpath sync. `PromptPack.spec.skills[]` references synced sources with `include` patterns and optional `mountAs` rename. PromptPack reconciler emits a JSON manifest next to the pack describing which skill paths to expose. `AgentRuntime` reconciler mounts the workspace content PVC into the runtime container and passes the manifest path as an env var. The runtime's PromptKit bootstrap reads the manifest and calls `sdk.WithSkillsDir(...)` once per resolved entry.

**Tech Stack:** Go, controller-runtime, `ee/pkg/arena` (moved to `internal/sourcesync`), PromptKit SDK (`sdk.WithSkillsDir`, `sdk.WithMaxActiveSkillsOption`, `sdk.WithSkillSelectorOption`).

**Spec:** `docs/superpowers/specs/2026-04-13-skills-source-design.md` (Phase 2).

---

## Pre-flight context

- `#807` extraction merged — `internal/sourcesync/` is available with `Fetcher` interface + Git/OCI/ConfigMap implementations, `FilesystemSyncer`, `LoadGitCredentials`/`LoadOCICredentials`, and a structural `StorageManager` interface.
- Shared schema types (`GitReference`, `GitSource`, `OCISource`, `ConfigMapSource`, `Artifact`) live in `api/v1alpha1/sourcesync_types.go`.
- Runtime binary (`cmd/runtime/main.go`) sets `serverOpts := append(serverOpts, pkruntime.WithToolsConfig(...))` — that's the hook point for adding skill-related SDK options.
- AgentRuntime controller does NOT currently mount the workspace content PVC into the runtime container. Arena workers (via Deployment spec in arena-controller) DO mount it. The same volume definition applies — mirror it.
- PromptKit skills SDK: `sdk.WithSkillsDir(dir string) sdk.Option` — accepts multiple calls, each registering a skill source directory. `sdk.WithMaxActiveSkillsOption(n int)` caps concurrent activations. `sdk.WithSkillSelectorOption(s sdk.SkillSelector)` picks the selector.
- **Pre-commit hook constraints** (learned from #801 / #807):
  - Runs `go build ./...`, `go vet ./...`, and per-file coverage ≥80% on changed Go files.
  - Every commit must leave the repo in a buildable, test-passing state.
  - CRD changes need companion regen: `make generate manifests sync-chart-crds generate-dashboard-types`.

## File Structure

### New files
| File | Responsibility |
|------|---------------|
| `api/v1alpha1/skillsource_types.go` | `SkillSource` CRD types: `SkillSourceSpec` (embeds `sourcesync_types` via same-package reference), `Filter`, `SkillSourceStatus`, `SkillSource`, `SkillSourceList` |
| `api/v1alpha1/skillsource_types_test.go` | Scheme registration smoke test |
| `api/v1alpha1/promptpack_skills_types.go` | `SkillRef` struct (`Source`, `Include`, `MountAs`) + `SkillsConfig` (`MaxActive`, `Selector`) used by `PromptPack.spec.skills` — kept in a separate file to keep `promptpack_types.go` focused |
| `internal/controller/skillsource_controller.go` | Reconciler: uses `sourcesync.Fetcher` + `FilesystemSyncer`, applies post-fetch `Filter`, validates every resolved SKILL.md, surfaces `SkillsValid` condition |
| `internal/controller/skillsource_controller_test.go` | Ginkgo + fake client: reconcile happy path, filter applies, ContentValid=False on parse error, name collisions surface |
| `internal/controller/skillsource_filter.go` | Post-fetch filter pass: walk synced tree, match `include`/`exclude`/`names` against subdirs containing SKILL.md |
| `internal/controller/skillsource_filter_test.go` | Table-driven filter tests |
| `internal/controller/skillsource_validator.go` | Parse SKILL.md frontmatter; return `(name, description, allowedTools, err)` |
| `internal/controller/skillsource_validator_test.go` | Valid + malformed + missing-name cases |
| `internal/webhook/skillsource_webhook.go` | Required-field validation: type matches exactly one of git/oci/configmap |
| `internal/webhook/skillsource_webhook_test.go` | One test per validation rule |
| `internal/runtime/skills/manifest.go` | `Manifest` struct, `Read(path string) (Manifest, error)`. Used by the runtime binary at startup |
| `internal/runtime/skills/manifest_test.go` | Golden manifest parsing, error cases |

### Modified files
| File | Change |
|------|--------|
| `api/v1alpha1/promptpack_types.go` | Add `Skills []SkillRef` and `SkillsConfig *SkillsConfig` to `PromptPackSpec`; add `SkillsResolved` / `SkillsValid` / `SkillToolsResolved` condition types |
| `internal/controller/promptpack_controller.go` | Resolve `skills:` refs; walk every post-`include` SKILL.md; validate `allowed-tools` against pack tools (+ToolRegistry union); emit manifest to `<workspace-pvc>/manifests/<pack-name>.json`; surface conditions |
| `internal/controller/promptpack_controller_test.go` | Add test cases for: missing source, name collision across entries, tool scope mismatch, manifest idempotency |
| `internal/controller/agentruntime_controller.go` | Add workspace content PVC mount (read-only) to the runtime container; set `OMNIA_PROMPTPACK_MANIFEST_PATH` env var |
| `internal/controller/agentruntime_controller_test.go` | Pod spec assertion: volume + mount + env var present |
| `cmd/main.go` | Register `SkillSourceReconciler` with the operator manager; register skillsource webhook |
| `cmd/runtime/main.go` | Read the manifest pointed to by `OMNIA_PROMPTPACK_MANIFEST_PATH` (or skip silently if unset); append `sdk.WithSkillsDir(...)` per entry and `sdk.WithMaxActiveSkillsOption` / selector options |
| `charts/omnia/templates/rbac.yaml` (via `make manifests`) | New RBAC verbs for `skillsources` |

### Generated (regenerated)
| File | |
|------|---|
| `api/v1alpha1/zz_generated.deepcopy.go` | Gains SkillSource + SkillRef + SkillsConfig + Filter deepcopy methods |
| `config/crd/bases/omnia.altairalabs.ai_skillsources.yaml` | New CRD |
| `config/crd/bases/omnia.altairalabs.ai_promptpacks.yaml` | Updated schema with `spec.skills` and `spec.skillsConfig` |
| `charts/omnia/crds/omnia.altairalabs.ai_skillsources.yaml` | Copied from config/crd/bases |
| `charts/omnia/crds/omnia.altairalabs.ai_promptpacks.yaml` | Updated |
| `dashboard/src/types/generated/skillsource.ts` | Generated TS types |
| `dashboard/src/types/generated/promptpack.ts` | Updated |

### Docs
| File | |
|------|---|
| `docs/src/content/docs/reference/skillsource.md` | CRD reference + examples |
| `docs/src/content/docs/how-to/use-skills.md` | Step-by-step walkthrough |
| `cmd/runtime/SERVICE.md` | Document new manifest input + PVC mount |
| `api/CHANGELOG.md` | New CRD + PromptPack field additions |

---

## Commit strategy

Five atomic commits to keep the build green at every step:

1. **`feat(api): SkillSource CRD + PromptPack.spec.skills types`** — all type definitions in one shot (PromptPack new fields need SkillRef which needs SkillSource), regenerate deepcopy + manifests + chart + dashboard types. Zero consumer code yet, only schemas.
2. **`feat(controller): SkillSource reconciler`** — controller + filter + validator + webhook + registration in `cmd/main.go`. Uses `internal/sourcesync`.
3. **`feat(controller): PromptPack resolves spec.skills, emits manifest`** — PromptPack reconciler extension, tool-scoping validation, manifest output.
4. **`feat(controller): AgentRuntime mounts workspace content PVC for skills`** — volume + mount + env var.
5. **`feat(runtime): wire skill manifest into PromptKit SDK options`** — manifest reader + `sdk.WithSkillsDir(...)` call site.

Each ends with `make generate manifests sync-chart-crds generate-dashboard-types`, full build + test, commit via pre-commit hook.

Docs (`feat(docs): skills reference + how-to`) ships as a 6th commit. Small, clear diff.

---

## Task 1: SkillSource + PromptPack spec type definitions (Commit 1)

**Files:**
- Create: `api/v1alpha1/skillsource_types.go`, `api/v1alpha1/skillsource_types_test.go`
- Create: `api/v1alpha1/promptpack_skills_types.go`
- Modify: `api/v1alpha1/promptpack_types.go`

### Step 1 — Create `api/v1alpha1/skillsource_types.go`

```go
/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SkillSourceType defines the type of source for skill content.
// Matches the variants in sourcesync_types.go so consumers can reuse the Fetcher.
// +kubebuilder:validation:Enum=git;oci;configmap
type SkillSourceType string

const (
	SkillSourceTypeGit       SkillSourceType = "git"
	SkillSourceTypeOCI       SkillSourceType = "oci"
	SkillSourceTypeConfigMap SkillSourceType = "configmap"
)

// SkillFilter narrows which skills from the synced tree are kept.
// A skill is a directory containing a SKILL.md file.
type SkillFilter struct {
	// include is a list of glob patterns (matched against the skill's
	// directory path relative to targetPath). An empty list means include all.
	// +optional
	Include []string `json:"include,omitempty"`

	// exclude is a list of glob patterns to drop after include is applied.
	// +optional
	Exclude []string `json:"exclude,omitempty"`

	// names pins individual skills by frontmatter `name:` field.
	// Applied after include/exclude.
	// +optional
	Names []string `json:"names,omitempty"`
}

// SkillSourceSpec defines the desired state of a SkillSource.
// +kubebuilder:validation:XValidation:rule="self.type != 'git' || has(self.git)",message="git source requires spec.git"
// +kubebuilder:validation:XValidation:rule="self.type != 'oci' || has(self.oci)",message="oci source requires spec.oci"
// +kubebuilder:validation:XValidation:rule="self.type != 'configmap' || has(self.configMap)",message="configmap source requires spec.configMap"
type SkillSourceSpec struct {
	// type selects the source variant. Exactly one of git/oci/configMap must be set.
	// +kubebuilder:validation:Required
	Type SkillSourceType `json:"type"`

	// git specifies a Git repository source.
	// +optional
	Git *GitSource `json:"git,omitempty"`

	// oci specifies an OCI registry source.
	// +optional
	OCI *OCISource `json:"oci,omitempty"`

	// configMap specifies a ConfigMap source.
	// +optional
	ConfigMap *ConfigMapSource `json:"configMap,omitempty"`

	// interval is the reconciliation poll interval (e.g. "1h").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^([0-9]+(\.[0-9]+)?(ms|s|m|h))+$`
	Interval string `json:"interval"`

	// timeout is the maximum duration for a single fetch (e.g. "5m").
	// +kubebuilder:default="60s"
	// +optional
	Timeout string `json:"timeout,omitempty"`

	// suspend prevents the source from being reconciled when set to true.
	// +kubebuilder:default=false
	// +optional
	Suspend bool `json:"suspend,omitempty"`

	// targetPath is the path under the workspace PVC where synced content lands,
	// e.g. "skills/anthropic". Defaults to "skills/{source-name}".
	// +optional
	TargetPath string `json:"targetPath,omitempty"`

	// filter narrows which skills from the synced tree are exposed.
	// +optional
	Filter *SkillFilter `json:"filter,omitempty"`

	// createVersionOnSync mirrors the ArenaSource field: when true, each sync
	// produces a content-addressable snapshot alongside the HEAD pointer.
	// +kubebuilder:default=true
	// +optional
	CreateVersionOnSync *bool `json:"createVersionOnSync,omitempty"`
}

// SkillSourcePhase reports the current lifecycle phase.
// +kubebuilder:validation:Enum=Pending;Initializing;Ready;Fetching;Error
type SkillSourcePhase string

const (
	SkillSourcePhasePending      SkillSourcePhase = "Pending"
	SkillSourcePhaseInitializing SkillSourcePhase = "Initializing"
	SkillSourcePhaseReady        SkillSourcePhase = "Ready"
	SkillSourcePhaseFetching     SkillSourcePhase = "Fetching"
	SkillSourcePhaseError        SkillSourcePhase = "Error"
)

// SkillSourceStatus defines the observed state.
type SkillSourceStatus struct {
	// phase reports the lifecycle phase.
	// +optional
	Phase SkillSourcePhase `json:"phase,omitempty"`

	// observedGeneration tracks the last spec generation the controller saw.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// artifact describes the last successfully fetched artifact.
	// +optional
	Artifact *Artifact `json:"artifact,omitempty"`

	// skillCount is the number of SKILL.md directories that pass the filter.
	// +optional
	SkillCount int32 `json:"skillCount,omitempty"`

	// conditions report detailed status. Known types:
	//   SourceAvailable — upstream reachable + fetched
	//   ContentValid    — every resolved SKILL.md frontmatter parses cleanly,
	//                     no duplicate names inside this source
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// lastFetchTime is the timestamp of the last fetch attempt.
	// +optional
	LastFetchTime *metav1.Time `json:"lastFetchTime,omitempty"`

	// nextFetchTime is the scheduled time for the next fetch.
	// +optional
	NextFetchTime *metav1.Time `json:"nextFetchTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Skills",type=integer,JSONPath=`.status.skillCount`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// SkillSource is a reusable, namespaced declaration of skill content fetched
// from an upstream source (Git, OCI, or ConfigMap) into the workspace PVC.
// Referenced from PromptPack.spec.skills[].source.
type SkillSource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SkillSourceSpec   `json:"spec"`
	Status SkillSourceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SkillSourceList contains a list of SkillSource.
type SkillSourceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SkillSource `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SkillSource{}, &SkillSourceList{})
}
```

- [ ] **Step 2 — Create `api/v1alpha1/skillsource_types_test.go`**

```go
/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package v1alpha1

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
)

func TestSkillSourceRegistration(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := AddToScheme(scheme); err != nil {
		t.Fatalf("add to scheme: %v", err)
	}
	gvk := GroupVersion.WithKind("SkillSource")
	if _, err := scheme.New(gvk); err != nil {
		t.Fatalf("SkillSource not registered: %v", err)
	}
	gvkList := GroupVersion.WithKind("SkillSourceList")
	if _, err := scheme.New(gvkList); err != nil {
		t.Fatalf("SkillSourceList not registered: %v", err)
	}
}
```

- [ ] **Step 3 — Create `api/v1alpha1/promptpack_skills_types.go`**

```go
/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package v1alpha1

// SkillRef selects content from a SkillSource for a PromptPack.
type SkillRef struct {
	// source is the name of a SkillSource in the same namespace as the PromptPack.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Source string `json:"source"`

	// include narrows the set of skills exposed from the source to those
	// whose frontmatter `name:` matches one of the entries. Empty = all
	// skills the source has synced (after its own filter).
	// +optional
	Include []string `json:"include,omitempty"`

	// mountAs renames the group under which these skills are exposed to the
	// runtime. Defaults to the source's targetPath basename. Used to give
	// PromptPack workflow states a stable `skills: ./skills/<group>` path
	// that doesn't depend on the upstream directory layout.
	// +optional
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`
	MountAs string `json:"mountAs,omitempty"`
}

// SkillSelector names a PromptKit skill selector strategy.
// +kubebuilder:validation:Enum=model-driven;tag;embedding
type SkillSelector string

const (
	// SkillSelectorModelDriven is the default — the LLM decides which skills
	// to activate based on the Phase-1 discovery index.
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
```

- [ ] **Step 4 — Modify `api/v1alpha1/promptpack_types.go`**

Grep for `type PromptPackSpec struct {` to find the location. Add the following two fields and condition type constants. If a similar `Conditions` block exists, add the new condition types next to it; otherwise they're just string constants used by the PromptPack reconciler.

Add to `PromptPackSpec`:

```go
	// skills selects content from SkillSources for the agents using this pack.
	// All entries go through a SkillSource — no inline content at this layer.
	// +optional
	Skills []SkillRef `json:"skills,omitempty"`

	// skillsConfig tunes the PromptKit skill runtime (max active, selector).
	// +optional
	SkillsConfig *SkillsConfig `json:"skillsConfig,omitempty"`
```

Add (at the bottom of the file, or alongside any existing condition-type block):

```go
// PromptPack skill-related condition types.
const (
	// PromptPackConditionSkillsResolved is set True when every SkillRef in
	// spec.skills names a SkillSource that exists in the pack's namespace.
	PromptPackConditionSkillsResolved = "SkillsResolved"
	// PromptPackConditionSkillsValid is set True when the post-include skill
	// set has no name collisions across sources.
	PromptPackConditionSkillsValid = "SkillsValid"
	// PromptPackConditionSkillToolsResolved is set True when every
	// resolved SKILL.md's allowed-tools set is a subset of the pack's
	// declared tools union with any referenced ToolRegistry.
	PromptPackConditionSkillToolsResolved = "SkillToolsResolved"
)
```

- [ ] **Step 5 — Regenerate**

```
env GOWORK=off make -C /Users/chaholl/repos/altairalabs/omnia/.worktrees/806-skills generate manifests sync-chart-crds
cd /Users/chaholl/repos/altairalabs/omnia/.worktrees/806-skills/dashboard && npm install --silent
env GOWORK=off make -C /Users/chaholl/repos/altairalabs/omnia/.worktrees/806-skills generate-dashboard-types
```

Expected:
- New `config/crd/bases/omnia.altairalabs.ai_skillsources.yaml`
- Updated `config/crd/bases/omnia.altairalabs.ai_promptpacks.yaml` (has `skills:` array + `skillsConfig`)
- `api/v1alpha1/zz_generated.deepcopy.go` updated
- New `charts/omnia/crds/omnia.altairalabs.ai_skillsources.yaml`
- `dashboard/src/types/generated/*` updated

- [ ] **Step 6 — Add the SkillSource CRD to the chart's CRD sync script**

Read `Makefile` around the `sync-chart-crds` target — it has a list of `cp config/crd/bases/* charts/omnia/crds/` lines. Add:

```
cp config/crd/bases/omnia.altairalabs.ai_skillsources.yaml charts/omnia/crds/
```

Re-run `make sync-chart-crds`. Verify `charts/omnia/crds/omnia.altairalabs.ai_skillsources.yaml` now exists.

- [ ] **Step 7 — Build + test + commit**

```
env GOWORK=off go -C /Users/chaholl/repos/altairalabs/omnia/.worktrees/806-skills build ./...
env GOWORK=off go -C /Users/chaholl/repos/altairalabs/omnia/.worktrees/806-skills test ./api/v1alpha1/... -count=1
```

Commit message:

```
cat <<'EOF' | git commit -F -
feat(api): SkillSource CRD + PromptPack.spec.skills field (#806)

Adds a new core CRD (SkillSource) that mirrors the ArenaSource sync
model for AgentSkills.io content, plus a PromptPack.spec.skills field
that selects content from SkillSources via LocalObjectReference +
optional include/mountAs.

Types only — the controller, PromptPack resolver, AgentRuntime mount,
and runtime wiring follow in separate commits.

Ref #806
EOF
```

---

## Task 2: SkillSource reconciler (Commit 2)

**Files:**
- Create: `internal/controller/skillsource_controller.go`, `_test.go`
- Create: `internal/controller/skillsource_filter.go`, `_test.go`
- Create: `internal/controller/skillsource_validator.go`, `_test.go`
- Create: `internal/webhook/skillsource_webhook.go`, `_test.go`
- Modify: `cmd/main.go` (register controller + webhook)

### Step 1 — Validator (smallest piece, standalone unit)

Create `internal/controller/skillsource_validator.go`:

```go
/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package controller

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// SkillFrontmatter is the YAML frontmatter at the top of a SKILL.md file.
// Matches the AgentSkills.io specification subset used by PromptKit.
type SkillFrontmatter struct {
	Name         string            `yaml:"name"`
	Description  string            `yaml:"description"`
	AllowedTools []string          `yaml:"allowed-tools,omitempty"`
	Metadata     map[string]string `yaml:"metadata,omitempty"`
}

// ParseSkillFile reads a SKILL.md path and returns its parsed frontmatter.
// Returns a descriptive error when the frontmatter is missing or malformed.
func ParseSkillFile(path string) (*SkillFrontmatter, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", filepath.Base(filepath.Dir(path)), err)
	}
	return parseSkillBytes(data)
}

func parseSkillBytes(data []byte) (*SkillFrontmatter, error) {
	// Frontmatter is the block between two "---" lines at the start of the file.
	const marker = "---\n"
	if !bytes.HasPrefix(data, []byte(marker)) {
		return nil, fmt.Errorf("SKILL.md missing opening '---' frontmatter marker")
	}
	rest := data[len(marker):]
	end := bytes.Index(rest, []byte("\n"+marker))
	if end < 0 {
		// Allow single-line separator at end of frontmatter.
		end = bytes.Index(rest, []byte("\n---"))
		if end < 0 {
			return nil, fmt.Errorf("SKILL.md missing closing '---' frontmatter marker")
		}
	}
	frontmatterBytes := rest[:end]

	var fm SkillFrontmatter
	if err := yaml.Unmarshal(frontmatterBytes, &fm); err != nil {
		return nil, fmt.Errorf("parse SKILL.md frontmatter: %w", err)
	}
	if fm.Name == "" {
		return nil, fmt.Errorf("SKILL.md frontmatter missing required 'name' field")
	}
	if fm.Description == "" {
		return nil, fmt.Errorf("SKILL.md frontmatter missing required 'description' field")
	}
	return &fm, nil
}
```

Create `internal/controller/skillsource_validator_test.go`:

```go
/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package controller

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSkillBytes_Valid(t *testing.T) {
	content := `---
name: ai-safety
description: Safety guardrails for AI-generated content.
allowed-tools:
  - redact
  - escalate
metadata:
  tags: "safety,compliance"
---

# AI Safety

Follow these rules when…
`
	fm, err := parseSkillBytes([]byte(content))
	require.NoError(t, err)
	assert.Equal(t, "ai-safety", fm.Name)
	assert.Equal(t, "Safety guardrails for AI-generated content.", fm.Description)
	assert.Equal(t, []string{"redact", "escalate"}, fm.AllowedTools)
	assert.Equal(t, "safety,compliance", fm.Metadata["tags"])
}

func TestParseSkillBytes_MissingOpeningMarker(t *testing.T) {
	_, err := parseSkillBytes([]byte("name: foo\ndescription: bar\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "opening")
}

func TestParseSkillBytes_MissingClosingMarker(t *testing.T) {
	_, err := parseSkillBytes([]byte("---\nname: foo\ndescription: bar\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "closing")
}

func TestParseSkillBytes_MissingName(t *testing.T) {
	_, err := parseSkillBytes([]byte(strings.Join([]string{
		"---",
		"description: No name here",
		"---",
		"body",
	}, "\n")))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name")
}

func TestParseSkillBytes_MissingDescription(t *testing.T) {
	_, err := parseSkillBytes([]byte(strings.Join([]string{
		"---",
		"name: nodesc",
		"---",
		"body",
	}, "\n")))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "description")
}
```

Run: `env GOWORK=off go test ./internal/controller/ -run TestParseSkillBytes -count=1 -v`
Expected: all 5 pass.

### Step 2 — Filter

Create `internal/controller/skillsource_filter.go`:

```go
/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package controller

import (
	"os"
	"path/filepath"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// ResolvedSkill is a skill that passed the filter: its SKILL.md was parsed
// and the containing directory is retained under the target path.
type ResolvedSkill struct {
	// Name is the frontmatter name.
	Name string
	// Description is the frontmatter description (for logging).
	Description string
	// AllowedTools is the frontmatter allowed-tools list.
	AllowedTools []string
	// RelPath is the skill's directory path relative to the synced root.
	RelPath string
}

// ResolveSkills walks syncRoot finding every SKILL.md, parses its frontmatter,
// and applies the optional filter. Returns one ResolvedSkill per retained
// skill, plus any parse errors encountered (collected — not fatal).
func ResolveSkills(syncRoot string, filter *corev1alpha1.SkillFilter) ([]ResolvedSkill, []error) {
	var resolved []ResolvedSkill
	var errs []error

	_ = filepath.Walk(syncRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			errs = append(errs, err)
			return nil
		}
		if info.IsDir() || info.Name() != "SKILL.md" {
			return nil
		}
		dir := filepath.Dir(path)
		relDir, relErr := filepath.Rel(syncRoot, dir)
		if relErr != nil {
			errs = append(errs, relErr)
			return nil
		}

		fm, parseErr := ParseSkillFile(path)
		if parseErr != nil {
			errs = append(errs, parseErr)
			return nil
		}

		if filter != nil && !matchesFilter(relDir, fm.Name, filter) {
			return nil
		}

		resolved = append(resolved, ResolvedSkill{
			Name:         fm.Name,
			Description:  fm.Description,
			AllowedTools: fm.AllowedTools,
			RelPath:      relDir,
		})
		return nil
	})

	return resolved, errs
}

func matchesFilter(relPath, name string, f *corev1alpha1.SkillFilter) bool {
	if len(f.Include) > 0 {
		matched := false
		for _, pattern := range f.Include {
			if ok, _ := filepath.Match(pattern, relPath); ok {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	for _, pattern := range f.Exclude {
		if ok, _ := filepath.Match(pattern, relPath); ok {
			return false
		}
	}
	if len(f.Names) > 0 {
		matched := false
		for _, n := range f.Names {
			if n == name {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}
```

Create `internal/controller/skillsource_filter_test.go`:

```go
/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package controller

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func writeSkill(t *testing.T, dir, name, description string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0755))
	content := "---\nname: " + name + "\ndescription: " + description + "\n---\nbody"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0644))
}

func TestResolveSkills_NoFilter(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, filepath.Join(root, "a"), "alpha", "first")
	writeSkill(t, filepath.Join(root, "b"), "beta", "second")

	resolved, errs := ResolveSkills(root, nil)
	assert.Empty(t, errs)
	assert.Len(t, resolved, 2)
}

func TestResolveSkills_IncludeGlob(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, filepath.Join(root, "safety-a"), "a", "d")
	writeSkill(t, filepath.Join(root, "other"), "b", "d")

	resolved, errs := ResolveSkills(root, &corev1alpha1.SkillFilter{
		Include: []string{"safety-*"},
	})
	assert.Empty(t, errs)
	assert.Len(t, resolved, 1)
	assert.Equal(t, "safety-a", resolved[0].RelPath)
}

func TestResolveSkills_Exclude(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, filepath.Join(root, "keep"), "keep", "d")
	writeSkill(t, filepath.Join(root, "draft-x"), "dx", "d")

	resolved, errs := ResolveSkills(root, &corev1alpha1.SkillFilter{
		Exclude: []string{"draft-*"},
	})
	assert.Empty(t, errs)
	assert.Len(t, resolved, 1)
	assert.Equal(t, "keep", resolved[0].Name)
}

func TestResolveSkills_NamesPin(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, filepath.Join(root, "a"), "only-this", "d")
	writeSkill(t, filepath.Join(root, "b"), "and-not-this", "d")

	resolved, errs := ResolveSkills(root, &corev1alpha1.SkillFilter{
		Names: []string{"only-this"},
	})
	assert.Empty(t, errs)
	assert.Len(t, resolved, 1)
	assert.Equal(t, "only-this", resolved[0].Name)
}

func TestResolveSkills_ParseErrorsReported(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "bad"), 0755))
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "bad", "SKILL.md"),
		[]byte("no frontmatter"), 0644))
	writeSkill(t, filepath.Join(root, "good"), "good", "d")

	resolved, errs := ResolveSkills(root, nil)
	assert.Len(t, resolved, 1)
	assert.NotEmpty(t, errs)
}
```

Run: `env GOWORK=off go test ./internal/controller/ -run TestResolveSkills -count=1 -v`
Expected: all pass.

### Step 3 — Reconciler

Create `internal/controller/skillsource_controller.go`:

```go
/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package controller

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/internal/sourcesync"
)

// SkillSource condition types.
const (
	SkillSourceConditionSourceAvailable = "SourceAvailable"
	SkillSourceConditionContentValid    = "ContentValid"
)

// SkillSourceReconciler reconciles SkillSource objects.
type SkillSourceReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	// WorkspaceContentPath is the base path for workspace content volumes,
	// mirroring the arena controllers. Structure:
	//   {WorkspaceContentPath}/{workspace}/{namespace}/{targetPath}/
	WorkspaceContentPath string

	// MaxVersionsPerSource is forwarded to FilesystemSyncer. Default 10.
	MaxVersionsPerSource int

	// StorageManager is optional; when set it provisions the workspace PVC
	// before writes. Matches the ee arena behaviour.
	StorageManager sourcesync.StorageManager
}

// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=skillsources,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=skillsources/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=skillsources/finalizers,verbs=update
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=workspaces,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch

// Reconcile implements the SkillSource reconcile loop.
func (r *SkillSourceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.V(1).Info("reconciling SkillSource", "name", req.Name, "namespace", req.Namespace)

	src := &corev1alpha1.SkillSource{}
	if err := r.Get(ctx, req.NamespacedName, src); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if src.Spec.Suspend {
		return ctrl.Result{}, nil
	}

	interval, err := time.ParseDuration(src.Spec.Interval)
	if err != nil {
		return r.errorStatus(ctx, src, "InvalidInterval", err)
	}

	// Build fetcher + sync.
	opts := sourcesync.DefaultOptions()
	if src.Spec.Timeout != "" {
		if to, err := time.ParseDuration(src.Spec.Timeout); err == nil {
			opts.Timeout = to
		}
	}
	fetcher, err := r.fetcherFor(ctx, src, opts)
	if err != nil {
		return r.errorStatus(ctx, src, "FetcherBuild", err)
	}

	revision, err := fetcher.LatestRevision(ctx)
	if err != nil {
		return r.errorStatus(ctx, src, "LatestRevision", err)
	}
	artifact, err := fetcher.Fetch(ctx, revision)
	if err != nil {
		return r.errorStatus(ctx, src, "Fetch", err)
	}
	defer func() { _ = os.RemoveAll(artifact.Path) }()

	targetPath := src.Spec.TargetPath
	if targetPath == "" {
		targetPath = filepath.Join("skills", src.Name)
	}

	syncer := &sourcesync.FilesystemSyncer{
		WorkspaceContentPath: r.WorkspaceContentPath,
		MaxVersionsPerSource: r.MaxVersionsPerSource,
	}
	if r.StorageManager != nil {
		syncer.StorageManager = r.StorageManager
	}

	workspaceName := GetWorkspaceForNamespace(ctx, r.Client, src.Namespace)
	contentPath, version, err := syncer.SyncToFilesystem(ctx, sourcesync.SyncParams{
		WorkspaceName: workspaceName,
		Namespace:     src.Namespace,
		TargetPath:    targetPath,
		Artifact:      artifact,
	})
	if err != nil {
		return r.errorStatus(ctx, src, "Sync", err)
	}

	// Apply the post-fetch filter + frontmatter validation.
	resolved, parseErrs := ResolveSkills(
		filepath.Join(r.WorkspaceContentPath, workspaceName, src.Namespace, contentPath),
		src.Spec.Filter)

	// Detect duplicate names within this source.
	seen := map[string]struct{}{}
	var dupes []string
	for _, sk := range resolved {
		if _, ok := seen[sk.Name]; ok {
			dupes = append(dupes, sk.Name)
		}
		seen[sk.Name] = struct{}{}
	}

	src.Status.ObservedGeneration = src.Generation
	src.Status.LastFetchTime = &metav1.Time{Time: time.Now()}
	src.Status.NextFetchTime = &metav1.Time{Time: time.Now().Add(interval)}
	src.Status.Artifact = &corev1alpha1.Artifact{
		Revision:       revision,
		ContentPath:    contentPath,
		Version:        version,
		Checksum:       artifact.Checksum,
		Size:           artifact.Size,
		LastUpdateTime: metav1.Time{Time: artifact.LastModified},
	}
	src.Status.SkillCount = int32(len(resolved))
	meta.SetStatusCondition(&src.Status.Conditions, metav1.Condition{
		Type: SkillSourceConditionSourceAvailable, Status: metav1.ConditionTrue,
		Reason: "FetchSucceeded", Message: fmt.Sprintf("revision %s", revision),
	})
	contentValid := len(parseErrs) == 0 && len(dupes) == 0
	condStatus := metav1.ConditionTrue
	condReason := "ContentValid"
	condMsg := fmt.Sprintf("%d skills validated", len(resolved))
	if !contentValid {
		condStatus = metav1.ConditionFalse
		condReason = "InvalidContent"
		condMsg = fmt.Sprintf("%d parse errors, %d duplicate names (%v)",
			len(parseErrs), len(dupes), dupes)
	}
	meta.SetStatusCondition(&src.Status.Conditions, metav1.Condition{
		Type: SkillSourceConditionContentValid, Status: condStatus,
		Reason: condReason, Message: condMsg,
	})
	src.Status.Phase = corev1alpha1.SkillSourcePhaseReady

	if err := r.Status().Update(ctx, src); err != nil {
		log.Error(err, "status update failed")
		return ctrl.Result{}, err
	}

	if r.Recorder != nil {
		r.Recorder.Event(src, "Normal", "Synced",
			fmt.Sprintf("synced %d skills at revision %s", len(resolved), revision))
	}

	return ctrl.Result{RequeueAfter: interval}, nil
}

func (r *SkillSourceReconciler) fetcherFor(ctx context.Context, src *corev1alpha1.SkillSource, opts sourcesync.Options) (sourcesync.Fetcher, error) {
	switch src.Spec.Type {
	case corev1alpha1.SkillSourceTypeGit:
		if src.Spec.Git == nil {
			return nil, fmt.Errorf("git source missing spec.git")
		}
		cfg := sourcesync.GitFetcherConfig{
			URL:     src.Spec.Git.URL,
			Path:    src.Spec.Git.Path,
			Options: opts,
		}
		if src.Spec.Git.Ref != nil {
			cfg.Ref = sourcesync.GitRef{
				Branch: src.Spec.Git.Ref.Branch,
				Tag:    src.Spec.Git.Ref.Tag,
				Commit: src.Spec.Git.Ref.Commit,
			}
		}
		if src.Spec.Git.SecretRef != nil {
			creds, err := sourcesync.LoadGitCredentials(ctx, r.Client, src.Namespace, src.Spec.Git.SecretRef.Name)
			if err != nil {
				return nil, fmt.Errorf("load git credentials: %w", err)
			}
			cfg.Credentials = creds
		}
		return sourcesync.NewGitFetcher(cfg), nil
	case corev1alpha1.SkillSourceTypeOCI:
		if src.Spec.OCI == nil {
			return nil, fmt.Errorf("oci source missing spec.oci")
		}
		cfg := sourcesync.OCIFetcherConfig{
			URL:      src.Spec.OCI.URL,
			Insecure: src.Spec.OCI.Insecure,
			Options:  opts,
		}
		if src.Spec.OCI.SecretRef != nil {
			creds, err := sourcesync.LoadOCICredentials(ctx, r.Client, src.Namespace, src.Spec.OCI.SecretRef.Name)
			if err != nil {
				return nil, fmt.Errorf("load oci credentials: %w", err)
			}
			cfg.Credentials = creds
		}
		return sourcesync.NewOCIFetcher(cfg), nil
	case corev1alpha1.SkillSourceTypeConfigMap:
		if src.Spec.ConfigMap == nil {
			return nil, fmt.Errorf("configmap source missing spec.configMap")
		}
		cfg := sourcesync.ConfigMapFetcherConfig{
			Name:      src.Spec.ConfigMap.Name,
			Namespace: src.Namespace,
			Key:       src.Spec.ConfigMap.Key,
			Options:   opts,
		}
		return sourcesync.NewConfigMapFetcher(cfg, r.Client), nil
	}
	return nil, fmt.Errorf("unknown source type %q", src.Spec.Type)
}

func (r *SkillSourceReconciler) errorStatus(ctx context.Context, src *corev1alpha1.SkillSource, reason string, cause error) (ctrl.Result, error) {
	src.Status.Phase = corev1alpha1.SkillSourcePhaseError
	meta.SetStatusCondition(&src.Status.Conditions, metav1.Condition{
		Type: SkillSourceConditionSourceAvailable, Status: metav1.ConditionFalse,
		Reason: reason, Message: cause.Error(),
	})
	if err := r.Status().Update(ctx, src); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: time.Minute}, nil
}

// SetupWithManager registers the reconciler with a controller-runtime manager.
func (r *SkillSourceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1alpha1.SkillSource{}).
		Named("skillsource").
		Complete(r)
}
```

NOTE: `GetWorkspaceForNamespace` is an existing helper in `internal/controller/` (used by other core controllers — check via `Grep "func GetWorkspaceForNamespace"`). If it's not in core but only in ee, move it to a shared location. That's a small sub-task inside this one.

- [ ] **Step 4 — Reconciler tests with fake client**

Create `internal/controller/skillsource_controller_test.go` using Ginkgo if the rest of `internal/controller/` uses Ginkgo, or stdlib testing otherwise (check via `Grep "RegisterFailHandler|RunSpecs" path=internal/controller`).

Cover:
- ConfigMap source, simple bundle → `status.phase=Ready`, `ContentValid=True`, `skillCount=N`.
- ConfigMap source with a malformed SKILL.md → `ContentValid=False`, message mentions parse error.
- ConfigMap source with duplicate names → `ContentValid=False`, message mentions duplicates.
- Missing source CR → reconcile returns nil error.
- Suspended source → reconcile returns empty result, no fetch attempted.

Use the ConfigMap variant because it's purely in-memory (no Git server / OCI registry to fake).

### Step 5 — Webhook

Create `internal/webhook/skillsource_webhook.go`:

```go
/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package webhook

import (
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// SkillSourceValidator enforces structural constraints beyond what CEL covers.
type SkillSourceValidator struct{}

var skillSourceLog = logf.Log.WithName("skillsource-webhook")

// +kubebuilder:webhook:path=/validate-omnia-altairalabs-ai-v1alpha1-skillsource,mutating=false,failurePolicy=fail,sideEffects=None,groups=omnia.altairalabs.ai,resources=skillsources,verbs=create;update,versions=v1alpha1,name=vskillsource.kb.io,admissionReviewVersions=v1

var _ admission.Validator[*corev1alpha1.SkillSource] = &SkillSourceValidator{}

func SetupSkillSourceWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &corev1alpha1.SkillSource{}).
		WithValidator(&SkillSourceValidator{}).
		Complete()
}

func (v *SkillSourceValidator) ValidateCreate(_ context.Context, src *corev1alpha1.SkillSource) (admission.Warnings, error) {
	skillSourceLog.Info("validating create", "name", src.Name, "namespace", src.Namespace)
	return nil, validateTypeMatchesVariant(src)
}

func (v *SkillSourceValidator) ValidateUpdate(_ context.Context, _, src *corev1alpha1.SkillSource) (admission.Warnings, error) {
	skillSourceLog.Info("validating update", "name", src.Name, "namespace", src.Namespace)
	return nil, validateTypeMatchesVariant(src)
}

func (v *SkillSourceValidator) ValidateDelete(_ context.Context, _ *corev1alpha1.SkillSource) (admission.Warnings, error) {
	return nil, nil
}

func validateTypeMatchesVariant(src *corev1alpha1.SkillSource) error {
	switch src.Spec.Type {
	case corev1alpha1.SkillSourceTypeGit:
		if src.Spec.Git == nil {
			return fmt.Errorf("type=git requires spec.git")
		}
	case corev1alpha1.SkillSourceTypeOCI:
		if src.Spec.OCI == nil {
			return fmt.Errorf("type=oci requires spec.oci")
		}
	case corev1alpha1.SkillSourceTypeConfigMap:
		if src.Spec.ConfigMap == nil {
			return fmt.Errorf("type=configmap requires spec.configMap")
		}
	}
	return nil
}
```

Test file: four cases (each of the three type→variant mismatches rejects + one valid case accepts).

### Step 6 — Register in `cmd/main.go`

Find the block where other controllers are registered (grep for `SetupWithManager(mgr)`) and add:

```go
if err := (&controller.SkillSourceReconciler{
    Client:   mgr.GetClient(),
    Scheme:   mgr.GetScheme(),
    Recorder: mgr.GetEventRecorderFor("skillsource-controller"),
    WorkspaceContentPath: workspaceContentPath, // whatever the flag is named
    MaxVersionsPerSource: 10,
}).SetupWithManager(mgr); err != nil {
    setupLog.Error(err, "unable to create controller", "controller", "SkillSource")
    os.Exit(1)
}

if err := webhook.SetupSkillSourceWebhookWithManager(mgr); err != nil {
    setupLog.Error(err, "unable to set up webhook", "webhook", "SkillSource")
    os.Exit(1)
}
```

Match the existing project's setup-log style.

### Step 7 — Regen + build + test + commit

```
env GOWORK=off make -C <worktree> generate manifests sync-chart-crds
env GOWORK=off go -C <worktree> build ./...
env GOWORK=off go -C <worktree> vet ./...
env GOWORK=off go -C <worktree> test ./internal/controller/... ./internal/webhook/... -count=1
```

Commit:

```
cat <<'EOF' | git commit -F -
feat(controller): SkillSource reconciler (#806)

Consumes internal/sourcesync to fetch skill content (git/oci/configmap),
applies a post-fetch filter (includes + excludes + names), parses every
SKILL.md frontmatter to surface SourceAvailable and ContentValid
conditions.

Webhook enforces the type→variant pairing. cmd/main.go registers both.

Ref #806
EOF
```

---

## Task 3: PromptPack resolves spec.skills, emits manifest (Commit 3)

**Files:**
- Modify: `internal/controller/promptpack_controller.go`
- Modify: `internal/controller/promptpack_controller_test.go`
- Create: `internal/controller/promptpack_skills.go` — manifest emission + tool-scope validation (extracted to keep the main controller file focused)
- Create: `internal/controller/promptpack_skills_test.go`

### Step 1 — Manifest struct + emission helper

Create `internal/controller/promptpack_skills.go`:

```go
/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// SkillManifestEntry is one row in the PromptPack skill manifest written to
// the workspace PVC. The runtime reads this and calls sdk.WithSkillsDir(...)
// per entry.
type SkillManifestEntry struct {
	// MountAs is the directory name the runtime exposes this skill under,
	// e.g. "billing/refund-processing".
	MountAs string `json:"mount_as"`
	// ContentPath is relative to the workspace PVC root.
	ContentPath string `json:"content_path"`
	// Name is the skill's frontmatter name (for logging).
	Name string `json:"name"`
}

// SkillManifestConfig carries the skillsConfig block through to the runtime.
type SkillManifestConfig struct {
	MaxActive int32  `json:"max_active,omitempty"`
	Selector  string `json:"selector,omitempty"`
}

// SkillManifest is serialised as JSON at
// <workspace-pvc>/manifests/<promptpack-name>.json
type SkillManifest struct {
	Version string               `json:"version"` // content hash of (sorted entries + config)
	Skills  []SkillManifestEntry `json:"skills"`
	Config  *SkillManifestConfig `json:"config,omitempty"`
}

// ResolvePromptPackSkills walks PromptPack.spec.skills, resolves each
// referenced SkillSource, applies the per-ref Include filter, and returns
// (manifest, allowed-tools union, errors).
//
// Errors here are not fatal — the reconciler uses them to set status
// conditions.
func ResolvePromptPackSkills(
	ctx context.Context,
	c client.Reader,
	pack *corev1alpha1.PromptPack,
	workspaceContentRoot string,
) (*SkillManifest, map[string][]string, []error) {
	if len(pack.Spec.Skills) == 0 {
		return &SkillManifest{}, nil, nil
	}

	var errs []error
	var entries []SkillManifestEntry
	allowedToolsBySkill := map[string][]string{}
	seenNames := map[string]string{}

	for _, ref := range pack.Spec.Skills {
		src := &corev1alpha1.SkillSource{}
		if err := c.Get(ctx, types.NamespacedName{Name: ref.Source, Namespace: pack.Namespace}, src); err != nil {
			if apierrors.IsNotFound(err) {
				errs = append(errs, fmt.Errorf("SkillSource %q not found", ref.Source))
				continue
			}
			errs = append(errs, fmt.Errorf("SkillSource %q lookup: %w", ref.Source, err))
			continue
		}
		if src.Status.Artifact == nil || src.Status.Artifact.ContentPath == "" {
			errs = append(errs, fmt.Errorf("SkillSource %q has no synced artifact yet", ref.Source))
			continue
		}

		mountGroup := ref.MountAs
		if mountGroup == "" {
			mountGroup = filepath.Base(src.Status.Artifact.ContentPath)
		}

		workspaceName := GetWorkspaceForNamespace(ctx, c, pack.Namespace)
		syncRoot := filepath.Join(workspaceContentRoot, workspaceName, pack.Namespace, src.Status.Artifact.ContentPath)
		resolved, parseErrs := ResolveSkills(syncRoot, nil) // the source already applied its own filter
		errs = append(errs, parseErrs...)

		includeSet := map[string]struct{}{}
		for _, name := range ref.Include {
			includeSet[name] = struct{}{}
		}

		for _, sk := range resolved {
			if len(includeSet) > 0 {
				if _, ok := includeSet[sk.Name]; !ok {
					continue
				}
			}
			entries = append(entries, SkillManifestEntry{
				MountAs:     filepath.Join(mountGroup, sk.Name),
				ContentPath: filepath.Join(src.Status.Artifact.ContentPath, sk.RelPath),
				Name:        sk.Name,
			})
			if existing, ok := seenNames[sk.Name]; ok {
				errs = append(errs, fmt.Errorf(
					"skill name collision: %q appears in both %s and %s",
					sk.Name, existing, ref.Source))
			}
			seenNames[sk.Name] = ref.Source
			allowedToolsBySkill[sk.Name] = sk.AllowedTools
		}
	}

	// Sort for deterministic manifest output (version hash stability).
	sort.Slice(entries, func(i, j int) bool { return entries[i].MountAs < entries[j].MountAs })

	manifest := &SkillManifest{Skills: entries}
	if pack.Spec.SkillsConfig != nil {
		cfg := &SkillManifestConfig{Selector: string(pack.Spec.SkillsConfig.Selector)}
		if pack.Spec.SkillsConfig.MaxActive != nil {
			cfg.MaxActive = *pack.Spec.SkillsConfig.MaxActive
		}
		manifest.Config = cfg
	}
	manifest.Version = hashManifest(manifest)

	return manifest, allowedToolsBySkill, errs
}

// ValidateSkillTools returns the set of (skillName, badTool) pairs where a
// skill's allowed-tools names a tool that isn't in packTools. Empty result
// means everything checks out.
func ValidateSkillTools(allowedToolsBySkill map[string][]string, packTools map[string]struct{}) []string {
	var bad []string
	for skillName, tools := range allowedToolsBySkill {
		for _, t := range tools {
			if _, ok := packTools[t]; !ok {
				bad = append(bad, fmt.Sprintf("%s:%s", skillName, t))
			}
		}
	}
	sort.Strings(bad)
	return bad
}

// WriteSkillManifest serializes the manifest to
// <root>/manifests/<name>.json atomically (write-temp-and-rename).
func WriteSkillManifest(root, name string, manifest *SkillManifest) error {
	dir := filepath.Join(root, "manifests")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("mkdir manifests: %w", err)
	}
	target := filepath.Join(dir, name+".json")
	tmp := target + ".tmp"
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("write tmp manifest: %w", err)
	}
	if err := os.Rename(tmp, target); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("publish manifest: %w", err)
	}
	return nil
}

// hashManifest returns a short SHA256 over the sorted entries + config. The
// exact algorithm is less important than being stable — downstream versioning
// just needs `same input -> same output`.
func hashManifest(m *SkillManifest) string {
	h := fnvHash()
	for _, e := range m.Skills {
		_, _ = h.Write([]byte(e.MountAs))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(e.ContentPath))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(e.Name))
		_, _ = h.Write([]byte{0})
	}
	if m.Config != nil {
		_, _ = fmt.Fprintf(h, "ma=%d;sel=%s", m.Config.MaxActive, m.Config.Selector)
	}
	return fmt.Sprintf("v%x", h.Sum32())
}
```

Add at the bottom of the file (or in a separate `hash_helper.go`):

```go
import "hash/fnv"

func fnvHash() *fnvHasher { return &fnvHasher{h: fnv.New32a()} }

type fnvHasher struct {
	h interface {
		Write(p []byte) (int, error)
		Sum32() uint32
	}
}

func (h *fnvHasher) Write(p []byte) (int, error) { return h.h.Write(p) }
func (h *fnvHasher) Sum32() uint32                { return h.h.Sum32() }
```

(Alternative: use `crypto/sha256` for a stronger hash — fnv is plenty for manifest versioning since the bar is "detect any meaningful change". Pick what feels right during implementation.)

- [ ] **Step 2 — Tests for manifest resolution**

Create `internal/controller/promptpack_skills_test.go` with table-driven cases using `sigs.k8s.io/controller-runtime/pkg/client/fake`:

- Empty `spec.skills` → empty manifest, no errors.
- One SkillSource, no filter, 3 skills resolved → 3 manifest entries.
- Include pins subset → manifest contains only pinned names.
- `mountAs: billing` → manifest entries start with "billing/".
- Two refs with same skill name → collision error returned.
- Missing SkillSource → error returned.
- SkillSource with no `status.artifact` yet → skipped with error.

- [ ] **Step 3 — Wire into PromptPack reconciler**

Modify `internal/controller/promptpack_controller.go`'s `Reconcile` to:
1. Call `ResolvePromptPackSkills`.
2. Surface `PromptPackConditionSkillsResolved` — True if no lookup errors, False with error list in Message otherwise.
3. Collect the pack's declared tools into a set.
4. Call `ValidateSkillTools` — set `PromptPackConditionSkillToolsResolved` accordingly.
5. Set `PromptPackConditionSkillsValid` based on name-collision errors.
6. Call `WriteSkillManifest` with the workspace PVC root.
7. Update status.

Study the existing Reconcile flow to determine where to insert this block — typically near the end, after other resolution work. Make the call conditional on `len(pack.Spec.Skills) > 0` so unused packs skip it entirely.

- [ ] **Step 4 — Extend the reconciler test suite**

Add cases that apply spec.skills and assert the three new conditions.

- [ ] **Step 5 — Build, test, commit**

Commit:

```
cat <<'EOF' | git commit -F -
feat(controller): PromptPack resolves spec.skills, emits manifest (#806)

PromptPack reconciler now:
- Resolves every spec.skills entry against its SkillSource.
- Applies the Include filter, deduplicates by skill name, renames via
  mountAs.
- Validates every resolved SKILL.md's allowed-tools against the pack's
  declared tool set (future: + ToolRegistry union).
- Writes a JSON manifest to <workspace-pvc>/manifests/<pack>.json that
  the runtime container will read at startup.
- Surfaces SkillsResolved, SkillsValid, SkillToolsResolved conditions.

Ref #806
EOF
```

---

## Task 4: AgentRuntime mounts workspace content PVC (Commit 4)

**Files:**
- Modify: `internal/controller/agentruntime_controller.go`
- Modify: `internal/controller/agentruntime_controller_test.go`

### Step 1 — Inspect the current Pod/Deployment shape

Grep `internal/controller/agentruntime_controller.go` for `Volumes` and `VolumeMounts`. Find where the runtime container's mounts are assembled (there may be a helper; if not, identify the structure). Mirror what arena workers do — they mount the workspace content PVC read-only at the same path.

Grep the arena controller or the Deployment it builds (`ee/internal/controller/arenajob_controller.go`) for "workspace-content" to find the canonical mount path (something like `/workspace-content`).

### Step 2 — Add the volume and mount

In the runtime container's PodSpec template, add:

```go
// Volume at pod level:
corev1.Volume{
    Name: "workspace-content",
    VolumeSource: corev1.VolumeSource{
        PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
            ClaimName: workspacePVCName(agentRuntime), // use whatever helper arena uses
            ReadOnly:  true,
        },
    },
},

// VolumeMount on the runtime container:
corev1.VolumeMount{
    Name:      "workspace-content",
    MountPath: "/workspace-content",
    ReadOnly:  true,
},

// Env var on the runtime container:
corev1.EnvVar{
    Name:  "OMNIA_PROMPTPACK_MANIFEST_PATH",
    Value: fmt.Sprintf("/workspace-content/%s/%s/manifests/%s.json",
        workspaceName, agentRuntime.Namespace, agentRuntime.Spec.PromptPackRef.Name),
},
```

The `workspacePVCName` helper and `workspaceName` resolution should already exist for how arena workers consume the PVC — reuse, don't reimplement.

### Step 3 — Test the pod spec

Extend `agentruntime_controller_test.go` with a test that creates an AgentRuntime, reconciles, and asserts:
- The generated Deployment's PodTemplateSpec has a volume named "workspace-content" backed by a PVC.
- The runtime container has a volume mount at `/workspace-content` (read-only).
- The runtime container has `OMNIA_PROMPTPACK_MANIFEST_PATH` set to the expected value.

### Step 4 — Build, test, commit

Commit:

```
cat <<'EOF' | git commit -F -
feat(controller): AgentRuntime mounts workspace content PVC for skills (#806)

Runtime container now has:
- A read-only mount of the workspace content PVC at /workspace-content,
  matching arena worker layout.
- OMNIA_PROMPTPACK_MANIFEST_PATH env var pointing at the manifest the
  PromptPack reconciler writes.

The runtime will read the manifest at startup (next commit) and pass
sdk.WithSkillsDir(...) to PromptKit for each listed skill.

Ref #806
EOF
```

---

## Task 5: Runtime reads manifest + wires PromptKit (Commit 5)

**Files:**
- Create: `internal/runtime/skills/manifest.go`, `_test.go`
- Modify: `internal/runtime/server.go` (or wherever `sdk.Option`s are assembled — confirm during implementation)

### Step 1 — Manifest reader

Create `internal/runtime/skills/manifest.go`:

```go
/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

// Package skills parses the PromptPack skill manifest emitted by the operator
// and exposes it to the runtime binary for PromptKit SDK wiring.
package skills

import (
	"encoding/json"
	"fmt"
	"os"
)

// Manifest mirrors the struct in internal/controller/promptpack_skills.go.
// Duplicated here because importing the controller package would pull in
// controller-runtime into the runtime binary.
type Manifest struct {
	Version string          `json:"version"`
	Skills  []ManifestEntry `json:"skills"`
	Config  *Config         `json:"config,omitempty"`
}

type ManifestEntry struct {
	MountAs     string `json:"mount_as"`
	ContentPath string `json:"content_path"`
	Name        string `json:"name"`
}

type Config struct {
	MaxActive int32  `json:"max_active,omitempty"`
	Selector  string `json:"selector,omitempty"`
}

// Read loads the manifest at path. Returns a zero-value manifest (not an error)
// when path is empty or the file doesn't exist — unconfigured skills are
// normal, and the runtime silently skips WithSkillsDir in that case.
func Read(path string) (*Manifest, error) {
	if path == "" {
		return &Manifest{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Manifest{}, nil
		}
		return nil, fmt.Errorf("read skill manifest %s: %w", path, err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse skill manifest %s: %w", path, err)
	}
	return &m, nil
}
```

Tests: empty path, missing file, valid JSON, malformed JSON.

### Step 2 — Add PromptKit options

Find the place in `internal/runtime/` where `s.sdkOptions` is appended to (per pre-flight, `internal/runtime/server.go` around lines 167-327). Add:

```go
import (
    // existing …
    "github.com/altairalabs/omnia/internal/runtime/skills"
)

// In the server setup function:
manifestPath := os.Getenv("OMNIA_PROMPTPACK_MANIFEST_PATH")
manifest, err := skills.Read(manifestPath)
if err != nil {
    return fmt.Errorf("read skill manifest: %w", err)
}
for _, e := range manifest.Skills {
    full := filepath.Join("/workspace-content", e.ContentPath)
    s.sdkOptions = append(s.sdkOptions, sdk.WithSkillsDir(full))
}
if manifest.Config != nil {
    if manifest.Config.MaxActive > 0 {
        s.sdkOptions = append(s.sdkOptions, sdk.WithMaxActiveSkillsOption(int(manifest.Config.MaxActive)))
    }
    if manifest.Config.Selector != "" {
        // Map "model-driven" | "tag" | "embedding" to the PromptKit selector.
        // Default (model-driven) requires no option.
        switch manifest.Config.Selector {
        case "tag":
            s.sdkOptions = append(s.sdkOptions, sdk.WithSkillSelectorOption(sdk.TagSelector()))
        case "embedding":
            // Embedding selector needs a provider — if the pack asks for it
            // without configuring one, we skip it here and leave the default.
            // A future commit can wire a Provider CRD ref for this.
        }
    }
}
```

Confirm the exact PromptKit selector function names (`sdk.TagSelector`, etc.) at implementation time — PromptKit docs listed them but SDK versions drift.

NOTE on the base path `"/workspace-content"` hardcoded here: this matches Task 4's mount path. If the arena workers use a different path prefix, use theirs — consistency matters.

### Step 3 — Wiring test

In whatever file holds the runtime server wiring test (`internal/runtime/server_test.go` or similar):
- Write a manifest JSON to a temp file.
- Set `OMNIA_PROMPTPACK_MANIFEST_PATH` env var.
- Construct the server.
- Assert `s.sdkOptions` contains a `sdk.WithSkillsDir(...)` option for each skill.

If `sdkOptions` is unexported, add a small accessor for test introspection (per pattern from #801).

### Step 4 — Build, test, commit

Commit:

```
cat <<'EOF' | git commit -F -
feat(runtime): wire PromptPack skill manifest into PromptKit SDK (#806)

Runtime reads the manifest at OMNIA_PROMPTPACK_MANIFEST_PATH on startup
and calls sdk.WithSkillsDir(...) once per skill. Empty or missing
manifest is a no-op — skills are optional.

Also honours SkillsConfig.MaxActive and SkillsConfig.Selector from the
PromptPack (model-driven is default, tag and embedding selectors
supported per PromptKit SDK).

Closes the skills pipeline end-to-end.

Ref #806
EOF
```

---

## Task 6: Docs + CHANGELOG (Commit 6)

**Files:**
- Create: `docs/src/content/docs/reference/skillsource.md`
- Create: `docs/src/content/docs/how-to/use-skills.md`
- Modify: `cmd/runtime/SERVICE.md`
- Modify: `api/CHANGELOG.md`

### Step 1 — Reference page

Document:
- SkillSource CRD schema (spec, status, conditions).
- PromptPack.spec.skills[] and skillsConfig.
- Resolution order: SkillSource filter → PromptPack include → PromptKit selector at runtime.
- Per-workspace scope (no cross-namespace).
- A complete YAML example (SkillSource + PromptPack + AgentRuntime that consumes it).

### Step 2 — How-to

Step-by-step:
1. Create a SkillSource (configmap variant for zero-dep demo, then git variant for production).
2. Verify `status.phase=Ready` + `skillCount`.
3. Add `skills:` block to PromptPack; verify conditions.
4. Redeploy the AgentRuntime (or wait for rolling update); verify the runtime pod mounts `/workspace-content` and the manifest env var is set.
5. Check the runtime logs for `sdk.WithSkillsDir` calls.
6. Test from a session: ask the model about a topic the skill addresses; confirm it calls `skill__activate`.

### Step 3 — SERVICE.md + CHANGELOG

Minimal updates matching the style of existing entries.

### Step 4 — Commit

```
cat <<'EOF' | git commit -F -
docs(skills): SkillSource + PromptPack skills reference + how-to (#806)

Reference page covers the SkillSource CRD, the PromptPack skills
field, and the end-to-end resolution path. How-to walks through
creating a source, attaching it to a pack, and verifying via the
runtime logs.

Ref #806
EOF
```

---

## Task 7: Final verification + PR

- [ ] **Step 1 — Full build + vet + test**

```
env GOWORK=off go build ./...
env GOWORK=off go vet ./...
env GOWORK=off go test ./... -count=1 -timeout 10m
```

All green.

- [ ] **Step 2 — Dashboard**

```
cd dashboard && npm run lint && npm run typecheck && npx vitest run
```

- [ ] **Step 3 — Local arena e2e (optional but recommended)**

Verify the skills pipeline doesn't regress the existing arena content flow. Per CLAUDE.md:

```
kind create cluster --name omnia-test-e2e --wait 60s
env GOWORK=off KIND_CLUSTER=omnia-test-e2e E2E_SKIP_CLEANUP=true \
  go test -tags=e2e ./test/e2e/ -v -ginkgo.v -ginkgo.label-filter='!arena' -timeout 20m
```

Plus the arena suite.

- [ ] **Step 4 — Push and PR**

```
git push -u origin feat/806-skills
gh pr create --repo AltairaLabs/Omnia --base main \
  --title "feat: SkillSource CRD + PromptPack skills wiring (#806)" \
  --body "$(cat <<'EOF'
## Summary

Closes #806. Ships AgentSkills.io-style skill loading for Omnia agents end to end.

- **New SkillSource CRD** (core, Apache-2.0) — syncs skill content into the workspace PVC via `internal/sourcesync`, with a post-fetch filter (globs + explicit names).
- **PromptPack.spec.skills** — declarative references to SkillSources with optional include and mountAs rename. PromptPack reconciler validates SKILL.md frontmatter, checks tool-scope against the pack's declared tools, emits a JSON manifest to the workspace PVC.
- **AgentRuntime PVC mount** — runtime container now mounts the workspace content PVC read-only at /workspace-content and receives the manifest path via env var.
- **Runtime wiring** — runtime reads the manifest and calls sdk.WithSkillsDir(...) per entry, honouring skillsConfig (maxActive, selector).

## Test plan

- [x] Unit tests at every layer (CRD types, validator, filter, reconciler, manifest emitter, manifest reader)
- [x] envtest controller tests for SkillSource and PromptPack reconciliation
- [x] AgentRuntime pod-spec assertion (PVC volume + mount + env var)
- [x] Runtime wiring test (manifest → sdk options)
- [x] Dashboard typecheck + tests clean
- [x] `make generate manifests sync-chart-crds generate-dashboard-types` no drift
- [ ] CI

## Out of scope

- Full envtest + real Git server e2e for SkillSource — deferred to a follow-up issue.
- Embedding selector wiring a Provider CRD ref for embeddings — the CRD accepts selector=embedding but the runtime skips it with a log. Follow-up issue if anyone actually wants it.
EOF
)"
```

---

## Self-Review

**1. Spec coverage** (`docs/superpowers/specs/2026-04-13-skills-source-design.md` Phase 2):
- SkillSource CRD with git/oci/configmap + filter → Task 2 + Task 1 types ✓
- PromptPack.spec.skills with LocalObjectReference source, include, mountAs → Task 1 types + Task 3 reconciler ✓
- SkillSource filter: globs + names → Task 2 filter ✓
- SkillsConfig (maxActive, selector) → Task 1 types + Task 5 runtime ✓
- Manifest emission under workspace PVC → Task 3 ✓
- AgentRuntime PVC mount + manifest env var → Task 4 ✓
- Runtime wiring: multiple WithSkillsDir calls, selector option → Task 5 ✓
- Tool-scoping validation → Task 3 `ValidateSkillTools` ✓
- Status conditions on PromptPack (SkillsResolved, SkillsValid, SkillToolsResolved) → Task 1 types + Task 3 reconciler ✓
- LocalObjectReference only (no cross-namespace) → Task 1 + Task 3 lookups are namespaced ✓
- Inline skills NOT in CRD → Task 1 types do not include inline ✓

**2. Placeholder scan:**
- No "TBD" or "add validation". Every commit block has its message body.
- One soft spot: Task 2 step 3 defers confirming `GetWorkspaceForNamespace` exists in core vs ee. The plan instructs the implementer to check and move if needed, which is honest.
- Task 5 step 2 notes "confirm PromptKit selector function names at implementation time". That's also honest — SDK versions drift and the exact name isn't load-bearing.

**3. Type consistency:**
- `SkillSource`, `SkillSourceSpec`, `SkillSourceStatus`, `SkillFilter`, `SkillSourceType` — consistent across Tasks 1, 2.
- `SkillRef{Source, Include, MountAs}`, `SkillsConfig{MaxActive, Selector}` — consistent Tasks 1, 3.
- `SkillManifest{Version, Skills, Config}` struct shape matches between controller emitter (Task 3) and runtime reader (Task 5). Duplicated intentionally to keep runtime free of controller-runtime imports.
- `ResolvedSkill{Name, Description, AllowedTools, RelPath}` — only used in controller package, consistent.
- `PromptPackConditionSkillsResolved` / `PromptPackConditionSkillsValid` / `PromptPackConditionSkillToolsResolved` — all three declared in Task 1 and used in Task 3.

**4. Pre-commit hook safety:**
- Every commit ends with build + test. Each commit leaves the tree buildable.
- Per-file coverage ≥80% is achievable: the new files are small and TDD-style tests cover them directly.
- CRD regen happens in Task 1 alone; subsequent tasks don't touch schema.

**5. Risks flagged inline:**
- PromptKit SDK selector function names — Task 5 step 2 notes.
- `GetWorkspaceForNamespace` existence — Task 2 step 3 notes.
- `workspacePVCName` helper location — Task 4 step 2 notes.

All three are ~30-second decisions at implementation time, documented in the plan so the implementer doesn't silently guess.
