# SkillSource + PromptPack Skills — Design Spec

**Issues:** #806 (this work) + #807 (extraction prerequisite)
**Date:** 2026-04-13
**Phasing:** Two PRs — #807 first (pure refactor), then #806 (skills feature).

## Problem

PromptKit ships AgentSkills.io support (`runtime/skills/`, `sdk.WithSkillsDir`) but Omnia has no CRD path to declare skill bundles or runtime wiring to surface them. Today's only option is to bake skills into the runtime image — no reuse, no per-workspace customisation, no version story.

Skills are a **core (non-enterprise) feature** but the existing sync infrastructure (used by `ArenaSource` and `ArenaTemplateSource`) lives in `ee/`. Phase 1 extracts the shared sync engine to `internal/sourcesync/` so SkillSource can consume it.

## Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Feature tier | **Core** (not enterprise) | Skills are baseline agent capability, like prompts and tools |
| Sync infrastructure | Extract `internal/sourcesync/` from `ee/internal/controller/arena*source_controller.go`; consume from all three CRDs | ArenaSource and ArenaTemplateSource already share schema types but duplicate the fetcher logic. SkillSource can't import ee/. One fetcher, three sources — future sources free |
| Sync mechanism | `SkillSource` CRD (core), uses extracted `internal/sourcesync/` | Same git/oci/configmap variants as ArenaSource, same content-addressable versioning, same workspace PVC target |
| Curation | `PromptPack.spec.skills` field with `include` filtering | PromptPack already serves as the shareable curated bundle; no separate `SkillRegistry` indirection |
| Inline skills | None at the CRD layer | All Omnia-managed skills go through SkillSource for uniform audit/version trail. PromptKit's pack-content payload still supports inline natively for true one-offs |
| Cross-namespace refs | `LocalObjectReference` only | Per-workspace trust + quota; replicate SkillSource into each workspace if shared |
| Runtime mount | Workspace content PVC, read-only, mirroring arena worker layout | Consistent paths everywhere a path appears (CRD, manifest, workflow scoping, runtime SDK call) |
| SDK invocation | Multiple `sdk.WithSkillsDir(...)` calls, one per resolved skill path | No symlink lifetime to manage; PromptKit's `runtime/skills/registry.go` already supports multiple dirs |
| Validation timing | Reconcile (status condition), post-filter | A SkillSource may have 1000 skills when the pack uses 3 — only validate the resolved set |
| Filter language | Globs + explicit `names:` | Covers the obvious cases; defer regex / by-tag until asked |

## Design

### Phase 1 — extract `internal/sourcesync/` (#807)

Pure refactor of the duplicated arena fetcher logic, with no behaviour change.

**Type moves** (Apache-2.0 license, no enterprise IP):
- `GitReference`, `GitSource`, `OCISource`, `ConfigMapSource`, `Artifact` move from `ee/api/v1alpha1/arenasource_types.go` to a new `api/v1alpha1/sourcesync_types.go`.
- `ee/api/v1alpha1/arenasource_types.go` and `arenatemplatesource_types.go` import from core.

**New `internal/sourcesync/` package:**
- `Fetcher` interface — given a source variant + target path, fetches content into the workspace PVC and returns an `Artifact` with revision / version / checksum.
- Implementations: `gitFetcher`, `ociFetcher`, `configMapFetcher`. Forked from the inlined logic in the two arena controllers.
- Content-addressable version computation (SHA256 over fetched tree).
- PVC writer that respects `targetPath`, atomic publish (write-and-rename), version retention.

**Refactored controllers:**
- `ee/internal/controller/arenasource_controller.go` — delegates the fetch/version/write portion to `internal/sourcesync/`. Keeps arena-specific status, version retention policy, and bundle integrity validation.
- `ee/internal/controller/arenatemplatesource_controller.go` — same delegation; keeps template discovery/parsing.

**Acceptance:** existing arena tests pass without semantic change. Bit-for-bit equivalent reconcile output.

### Phase 2 — `SkillSource` CRD (#806, namespace-scoped)

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: SkillSource
metadata:
  name: anthropic-skills
  namespace: dev-agents              # = workspace namespace
spec:
  type: git                          # git | oci | configmap
  git:
    url: https://github.com/anthropic/skills
    ref: { tag: v1.4.0 }
    path: skills                     # optional repo subpath
    secretRef: { name: github-pat, key: token }
  interval: 1h
  timeout: 5m
  targetPath: skills/anthropic       # under the workspace PVC

  filter:
    include:
      - "ai-safety/*"
      - "pdf-processing/*"
    exclude:
      - "**/draft-*"
    names:                           # by SKILL.md frontmatter name
      - "claude-code-review"

  createVersionOnSync: true

status:
  phase: Ready                       # Pending | Initializing | Ready | Fetching | Error
  observedGeneration: 1
  artifact:
    revision: "v1.4.0@sha1:abc..."
    contentPath: "skills/anthropic"
    version: "sha256:..."            # content-addressable
    checksum: "sha256:..."
    size: 524288
    lastUpdateTime: "..."
  conditions:
    - type: SourceAvailable          # upstream reachable
      status: "True"
    - type: ContentValid             # all SKILL.md frontmatter parses, no name collisions
      status: "True"
      message: "12 skills validated after filter"
  nextFetchTime: "..."
  headVersion: "sha256:..."
```

Source variants and reconciliation behaviour identical to `ArenaSource` (see `ee/api/v1alpha1/arenasource_types.go` and its controller). The new pieces:

- **`spec.filter`** — applied as a post-fetch pass before the synced tree is published. Walks `<targetPath>/...`, drops directories that don't match. Empty filter = include everything (PromptKit-style).
- **`status.conditions[ContentValid]`** — set False if any SKILL.md frontmatter fails to parse, or if duplicate names exist within the post-filter set. The artifact still publishes; consumers see the failure on the condition.

The trust boundary: SkillSource creation is RBAC-gated. Anything synced has been blessed by an operator with `skillsources/create` permission in that workspace.

### `PromptPack.spec.skills` field

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: PromptPack
metadata:
  name: support-agent
spec:
  # … existing prompts/tools/workflow …

  skills:
    - source: anthropic-skills        # LocalObjectReference; same namespace
      include: ["ai-safety", "pdf-processing"]
    - source: internal-skills
      mountAs: billing                # rename for workflow scoping
      include: ["refund-processing", "pci-compliance"]

  skillsConfig:
    maxActive: 5                      # WithMaxActiveSkillsOption
    selector: model-driven            # model-driven | tag | embedding
```

Resolution at PromptPack reconcile:

1. For each entry, look up the referenced `SkillSource` in the same namespace. Missing → `SkillsResolved=False/SourceNotFound`.
2. Apply `include` (defaults to all post-filter skills if omitted) → resolved skill set.
3. Apply `mountAs` rename (defaults to source's `targetPath` basename) → mount-path layout.
4. Detect name collisions across all entries → `SkillsValid=False/NameCollision` listing offenders.
5. Walk every resolved SKILL.md's `allowed-tools`, confirm each exists in the pack's tool set ∪ referenced `ToolRegistry` → `SkillToolsResolved=True/False`.
6. Emit a manifest into the workspace PVC at `<workspace-pvc>/manifests/<promptpack-name>.json`:

```json
{
  "version": "sha256:...",
  "skills": [
    {"mount_as": "anthropic-skills/ai-safety",  "content_path": "skills/anthropic/ai-safety"},
    {"mount_as": "anthropic-skills/pdf-processing", "content_path": "skills/anthropic/pdf-processing"},
    {"mount_as": "billing/refund-processing", "content_path": "skills/internal/refund-processing"},
    {"mount_as": "billing/pci-compliance", "content_path": "skills/internal/pci-compliance"}
  ],
  "config": {"max_active": 5, "selector": "model-driven"}
}
```

The manifest version is a SHA256 of (sorted resolved entries + `skillsConfig`). Bumps when source artifacts change OR `spec.skills` changes — feeds into PromptPack's existing version mechanism so AgentRuntimes pinned to the pack roll automatically.

### `AgentRuntime` reconcile changes

The runtime container does NOT currently mount the workspace content PVC. Add:

1. Mount the workspace content PVC read-only at the same path arena workers use (verify exact path during implementation; mirror it).
2. Pass `OMNIA_PROMPTPACK_MANIFEST_PATH=/workspace-content/<workspace>/<group>/manifests/<promptpack-name>.json` to the runtime container.

These mounts are unconditional — there's no flag to disable them. If a PromptPack has no `skills:`, the manifest just contains an empty `skills: []` array and the runtime no-ops.

### Runtime container wiring

At startup, after reading the PromptPack:

```go
manifest, err := readSkillManifest(os.Getenv("OMNIA_PROMPTPACK_MANIFEST_PATH"))
if err != nil { return fmt.Errorf("read skill manifest: %w", err) }

opts := []sdk.Option{ /* … existing … */ }
for _, s := range manifest.Skills {
    fullPath := filepath.Join(workspaceContentRoot, s.ContentPath)
    opts = append(opts, sdk.WithSkillsDir(fullPath))
}
if cfg := manifest.Config; cfg != nil {
    if cfg.MaxActive > 0 {
        opts = append(opts, sdk.WithMaxActiveSkillsOption(cfg.MaxActive))
    }
    if cfg.Selector != "" {
        opts = append(opts, selectorOption(cfg.Selector))
    }
}
```

If multiple `WithSkillsDir` calls produce surprising selector behaviour (single discovery index across all roots vs. one-per-root), file an SDK tweak request upstream rather than working around it with symlinks. Symlink tree is the documented fallback only if PromptKit can't be made to do the right thing.

### Workflow state scoping

Unchanged from PromptKit: workflow state's `skills: ./skills/billing` resolves against the runtime mount layout. `mountAs: billing` in PromptPack guarantees the path the workflow references exists.

### Filtering layers (recap)

| Layer | Where | What it controls |
|---|---|---|
| Sync time | `SkillSource.spec.filter` | What gets pulled from upstream into the workspace PVC |
| Curation time | `PromptPack.spec.skills[].include` | Which synced skills this pack exposes |
| Activation time | PromptKit selector + workflow `skills:` | What the model sees in any given turn |

## Testing strategy

### Unit tests

- **SkillSource controller**: source variants reconcile (git/oci/configmap), `filter` drops non-matching dirs, name collisions in post-filter set surface `ContentValid=False`, version hash changes only on actual content change.
- **PromptPack `skills:` reconciler**: missing source → `SourceNotFound`, name collision across entries → `NameCollision`, tool-scope mismatch → `SkillToolsResolved=False`, manifest emission idempotent, manifest version stable across no-op reconciles.
- **Runtime SDK wiring**: manifest parser, `WithSkillsDir` per entry, no-op when manifest is empty, error when manifest path missing in enterprise mode.

### Integration tests

- Full chain: create SkillSource (configmap variant for hermeticity), create PromptPack referencing it with `include` filter, verify manifest produced under PVC, verify the runtime would call `WithSkillsDir` for each resolved path.
- `mountAs` rename: PromptPack workflow state references `./skills/billing`, manifest exposes the renamed group, PromptKit selector finds it.
- Tool-scoping: SKILL.md `allowed-tools` references a tool the pack doesn't declare → `SkillToolsResolved=False` with the bad tool name in the message.
- 1000-skill source filtered to 3: validation only walks the 3, manifest only contains the 3.

### Wiring tests

- `cmd/runtime/main.go` wiring test: with `OMNIA_PROMPTPACK_MANIFEST_PATH` set, the constructed SDK opts include the expected `WithSkillsDir` calls and selector option.
- `AgentRuntime` reconciler: pod spec includes the workspace PVC mount and the manifest env var.

### E2E test (deferred)

A real-cluster test that pulls a small skill repo via SkillSource, attaches it to a PromptPack, deploys an AgentRuntime, and asserts the model can invoke `skill__activate` for one of the skills. Useful but heavy — defer to a follow-up issue.

## Files affected

### Phase 1 (#807) — extraction

**New files**
- `api/v1alpha1/sourcesync_types.go` — the moved shared types (`GitReference`, `GitSource`, `OCISource`, `ConfigMapSource`, `Artifact`)
- `internal/sourcesync/fetcher.go` — `Fetcher` interface
- `internal/sourcesync/git.go`, `oci.go`, `configmap.go` — implementations
- `internal/sourcesync/version.go` — content-addressable version computation
- `internal/sourcesync/writer.go` — PVC writer (atomic publish, retention)
- `internal/sourcesync/*_test.go`

**Modified files**
- `ee/api/v1alpha1/arenasource_types.go` — remove the moved types; keep ArenaSource-specific `Spec`/`Status` and re-export the moved ones if needed for backward import compatibility (or just update consumers to import from core).
- `ee/api/v1alpha1/arenatemplatesource_types.go` — same.
- `ee/internal/controller/arenasource_controller.go` — replace inlined fetcher with `internal/sourcesync` calls.
- `ee/internal/controller/arenatemplatesource_controller.go` — same.
- Both `*_controller_test.go` — mechanical updates only.

### Phase 2 (#806) — skills

**New files**
- `api/v1alpha1/skillsource_types.go` — SkillSource CRD types (core), embeds shared types from phase 1, adds `Filter`
- `internal/controller/skillsource_controller.go` — reconciler (uses `internal/sourcesync` + post-fetch filter)
- `internal/controller/skillsource_controller_test.go`
- `internal/webhook/skillsource_webhook.go` — minimal required-field guards
- `internal/runtime/skills/manifest.go` — manifest reader for the runtime container
- `internal/runtime/skills/manifest_test.go`

**Modified files**
- `api/v1alpha1/promptpack_types.go` — add `Skills []SkillRef` and `SkillsConfig` fields to spec; add `SkillsResolved` / `SkillsValid` / `SkillToolsResolved` condition types
- `internal/controller/promptpack_controller.go` — resolve `skills:` references, validate, emit manifest, surface conditions
- `internal/controller/promptpack_controller_test.go`
- `internal/controller/agentruntime_controller.go` — mount workspace content PVC into runtime container, set `OMNIA_PROMPTPACK_MANIFEST_PATH`
- `internal/controller/agentruntime_controller_test.go`
- `cmd/runtime/main.go` — read manifest, append `WithSkillsDir` opts and selector
- `config/crd/bases/...` + `charts/omnia/...` — regenerated CRD/RBAC YAMLs

### Docs
- `docs/src/content/docs/reference/skillsource.md`
- `docs/src/content/docs/how-to/use-skills.md`
- `cmd/runtime/SERVICE.md` — note new manifest input
- `api/CHANGELOG.md`

## Open implementation questions (resolve during plan writing)

- Exact path the workspace PVC mounts at in arena workers — runtime mount must mirror it.
- Whether the PromptPack reconciler writes the manifest directly to the PVC (needs PVC access on the operator) or to a ConfigMap that the runtime reads via projection. PVC write is consistent with arena content; ConfigMap projection avoids cross-pod PVC writes from the operator. Pick during plan writing based on which is closer to existing patterns.
- Webhook scope: just required-field validation (`type` ⇒ matching `git`/`oci`/`configmap` block), or also catch obvious filter-pattern errors? Probably just required fields; filter errors land on the status condition.

## Out of scope

- Runtime hooks (`ProviderHook` / `ToolHook` / `SessionHook`) — separate proposal; involves user code execution and a different security boundary.
- Eval hooks — already wired (`internal/runtime/evals_integration_test.go`).
- Cross-workspace shared registries — replicate SkillSource per workspace.
- Catalog / index subscription — defer until AgentSkills.io publishes a spec or someone asks.
