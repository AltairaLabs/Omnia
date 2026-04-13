# Extract `internal/sourcesync` from Arena Controllers (#807)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move the existing `ee/pkg/arena/fetcher/` package, the `FilesystemSyncer` (`ee/internal/controller/fssync.go`), the credential resolver (`ee/internal/controller/credentials.go`), and the shared schema types (`GitSource`, `OCISource`, `ConfigMapSource`, `GitReference`, `Artifact`) into a new core `internal/sourcesync/` package and `api/v1alpha1/sourcesync_types.go`. Refactor `ArenaSource` and `ArenaTemplateSource` controllers to consume the moved code. Pure refactor, no behaviour change.

**Architecture:** All current logic stays — just relicensed (Apache-2.0) and relocated to core so the upcoming `SkillSource` (#806) can consume the same fetcher and syncer without importing `ee/`. Each task is an atomic move-plus-import-update commit so the build stays green at every step (per `hack/pre-commit` running `go build ./...` and `go vet ./...` on the whole repo).

**Tech Stack:** Go, controller-runtime, kubebuilder. No new dependencies.

**Spec:** `docs/superpowers/specs/2026-04-13-skills-source-design.md` (Phase 1 section).

---

## Pre-flight context (read once)

- `ee/pkg/arena/fetcher/` is already a clean, encapsulated package (~1.4k LOC + ~2.6k test LOC) with `Fetcher` interface, `Artifact` struct, `Options`, and three implementations (Git, OCI, ConfigMap). Currently FSL-licensed.
- `ee/internal/controller/fssync.go` defines `FilesystemSyncer` (~310 LOC) — content-addressable storage, HEAD pointer, version GC. Depends on `ee/pkg/workspace.StorageManager` for lazy PVC creation (an interface dependency we'll bridge).
- `ee/internal/controller/credentials.go` (~74 LOC) — resolves Secret-backed credentials for fetchers.
- `ee/api/v1alpha1/arenasource_types.go` defines `GitReference`, `GitSource`, `OCISource`, `ConfigMapSource`, `Artifact`. `arenatemplatesource_types.go` reuses them via plain Go references in the same package.
- Both arena controllers consume `ee/pkg/arena/fetcher` and `FilesystemSyncer`.
- `arenasource_controller.go` also depends on `ee/pkg/license.Validator` (enterprise license check) and `ee/pkg/workspace.StorageManager` — these stay in ee.
- All eight files importing `ee/pkg/arena/fetcher` per `Grep`:
  - `ee/internal/controller/arenasource_controller.go`
  - `ee/internal/controller/arenasource_controller_test.go`
  - `ee/internal/controller/arenatemplatesource_controller.go`
  - `ee/internal/controller/arenatemplatesource_controller_test.go`
  - `ee/internal/controller/fssync.go`
  - `ee/internal/controller/fssync_test.go`
  - `ee/internal/controller/credentials.go`
  - `.golangci.yml` (lint exclusions for some fetcher_test patterns)

---

## File Structure

### New files
| File | Responsibility |
|------|---------------|
| `api/v1alpha1/sourcesync_types.go` | Shared schema types: `GitReference`, `GitSource`, `OCISource`, `ConfigMapSource`, `Artifact` (Apache-2.0) |
| `internal/sourcesync/fetcher.go` | `Fetcher` interface, `Artifact` runtime type (distinct from CRD type), `Options`, `DefaultOptions` |
| `internal/sourcesync/git.go` | `GitFetcher` — moved from `ee/pkg/arena/fetcher/git.go` |
| `internal/sourcesync/oci.go` | `OCIFetcher` — moved from `ee/pkg/arena/fetcher/oci.go` |
| `internal/sourcesync/configmap.go` | `ConfigMapFetcher` — moved from `ee/pkg/arena/fetcher/configmap.go` |
| `internal/sourcesync/dir.go` | Directory utilities — moved from `ee/pkg/arena/fetcher/dir.go` |
| `internal/sourcesync/hash.go` | `CalculateDirectoryHash` — moved from `ee/pkg/arena/fetcher/hash.go` |
| `internal/sourcesync/syncer.go` | `FilesystemSyncer` — moved from `ee/internal/controller/fssync.go`. `StorageManager` interface defined here (ee provides the implementation) |
| `internal/sourcesync/credentials.go` | Credential resolver — moved from `ee/internal/controller/credentials.go` |
| `internal/sourcesync/*_test.go` | Tests moved alongside source files |

### Deleted files (after moves)
- `ee/pkg/arena/fetcher/*.go` — entire package
- `ee/internal/controller/fssync.go` + test
- `ee/internal/controller/credentials.go`

### Modified files
- `ee/api/v1alpha1/arenasource_types.go` — remove the moved type definitions; leave only `ArenaSourceType`, `ArenaSourceSpec`, `ArenaSourceStatus`, `ArenaSourcePhase`, `ArenaSource`, `ArenaSourceList`. Re-import the shared types via type aliases for backward source compat (`type GitSource = corev1alpha1.GitSource`) so callers within ee don't need updates beyond import paths.
- `ee/api/v1alpha1/arenatemplatesource_types.go` — same
- `ee/internal/controller/arenasource_controller.go` — replace `fetcher.X` with `sourcesync.X`, replace `FilesystemSyncer` reference (was in same package, now imported from `internal/sourcesync`)
- `ee/internal/controller/arenasource_controller_test.go` — same
- `ee/internal/controller/arenatemplatesource_controller.go` — same
- `ee/internal/controller/arenatemplatesource_controller_test.go` — same
- `.golangci.yml` — update path patterns from `ee/pkg/arena/fetcher` to `internal/sourcesync`

### Generated (regenerated by `make generate manifests sync-chart-crds`)
- `api/v1alpha1/zz_generated.deepcopy.go` — gains DeepCopy methods for moved types
- `ee/api/v1alpha1/zz_generated.deepcopy.go` — loses methods for the same types
- `config/crd/bases/...` — should be no diff (the CRDs reference the same fields, just defined in a different Go package)
- `charts/omnia/...` — should be no diff

---

## Task 1: Move the fetcher package + update consumers

**Files:**
- Create: `internal/sourcesync/fetcher.go`, `git.go`, `oci.go`, `configmap.go`, `dir.go`, `hash.go` (and matching `_test.go`)
- Delete: `ee/pkg/arena/fetcher/*.go` (after consumers migrated)
- Modify: 8 consumer files (controllers + tests + fssync + credentials + .golangci.yml)

The Artifact struct in `ee/pkg/arena/fetcher/fetcher.go` is the **runtime** Artifact (with a `Path string` field). It's distinct from the CRD `Artifact` type in `ee/api/v1alpha1/arenasource_types.go` (which has `Revision`, `URL`, `ContentPath`, `Version`, `Checksum`, `Size`, `LastUpdateTime`). Keep them distinct — they're not the same struct.

- [ ] **Step 1: Create the new package directory and copy files**

```bash
mkdir -p internal/sourcesync
for f in fetcher git oci configmap dir hash; do
  cp ee/pkg/arena/fetcher/${f}.go internal/sourcesync/${f}.go
  cp ee/pkg/arena/fetcher/${f}_test.go internal/sourcesync/${f}_test.go 2>/dev/null || true
done
```

(Note: `fetcher.go` has no `_test.go`; `dir.go`, `hash.go`, `git.go`, `oci.go`, `configmap.go` all do.)

- [ ] **Step 2: Update license headers and package name in every moved file**

Replace the file header in every `internal/sourcesync/*.go`:

```go
/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package fetcher
```

with:

```go
/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package sourcesync
```

Use `Edit` with `replace_all=false` per file (each file's header is identical so it's a single Edit per file).

- [ ] **Step 3: Update package doc comment in `fetcher.go`**

The original starts with:

```go
// Package fetcher provides interfaces and implementations for fetching
// PromptKit bundles from various sources (Git, OCI, ConfigMap).
package fetcher
```

Change to:

```go
// Package sourcesync provides interfaces and implementations for fetching
// content (PromptKit bundles, skills, templates) from various sources
// (Git, OCI, ConfigMap), with content-addressable versioning and
// filesystem synchronisation.
package sourcesync
```

- [ ] **Step 4: Update consumer imports**

In each of these files, replace the import:
- `"github.com/altairalabs/omnia/ee/pkg/arena/fetcher"` → `"github.com/altairalabs/omnia/internal/sourcesync"`

And the alias used in code (sed-style — but use Edit per file):
- `fetcher.Artifact` → `sourcesync.Artifact`
- `fetcher.Fetcher` → `sourcesync.Fetcher`
- `fetcher.Options` → `sourcesync.Options`
- `fetcher.DefaultOptions` → `sourcesync.DefaultOptions`
- `fetcher.GitFetcherConfig` → `sourcesync.GitFetcherConfig`
- `fetcher.GitRef` → `sourcesync.GitRef`
- `fetcher.NewGitFetcher` → `sourcesync.NewGitFetcher`
- `fetcher.OCIFetcherConfig` → `sourcesync.OCIFetcherConfig`
- `fetcher.NewOCIFetcher` → `sourcesync.NewOCIFetcher`
- `fetcher.ConfigMapFetcherConfig` → `sourcesync.ConfigMapFetcherConfig`
- `fetcher.NewConfigMapFetcher` → `sourcesync.NewConfigMapFetcher`
- `fetcher.CalculateDirectoryHash` (if exposed) → `sourcesync.CalculateDirectoryHash`

Files to update:
- `ee/internal/controller/arenasource_controller.go`
- `ee/internal/controller/arenasource_controller_test.go`
- `ee/internal/controller/arenatemplatesource_controller.go`
- `ee/internal/controller/arenatemplatesource_controller_test.go`
- `ee/internal/controller/fssync.go`
- `ee/internal/controller/fssync_test.go`
- `ee/internal/controller/credentials.go`

For each file: read it, find every `fetcher.` reference, replace, then save.

- [ ] **Step 5: Delete the original fetcher package**

```bash
rm -rf ee/pkg/arena/fetcher
```

- [ ] **Step 6: Update `.golangci.yml`**

Read `.golangci.yml` and replace any `ee/pkg/arena/fetcher` patterns with `internal/sourcesync`. Likely a path-prefix pattern in `exclusions` or `issues`.

- [ ] **Step 7: Build and test**

```
env GOWORK=off go -C /Users/chaholl/repos/altairalabs/omnia build ./...
env GOWORK=off go -C /Users/chaholl/repos/altairalabs/omnia vet ./...
env GOWORK=off go -C /Users/chaholl/repos/altairalabs/omnia test ./internal/sourcesync/... ./ee/internal/controller/... -count=1
```

Expected: build clean, all tests pass. Coverage on `internal/sourcesync/*.go` should be the same as the original `ee/pkg/arena/fetcher/*.go` (same code, same tests).

If a test fails referencing FSL license headers in test fixtures, update the fixture string. (Unlikely — tests don't typically inspect license headers.)

- [ ] **Step 8: Commit**

```
git add internal/sourcesync/ ee/pkg/arena/fetcher/ ee/internal/controller/ .golangci.yml
cat <<'EOF' | git commit -F -
refactor: move ee/pkg/arena/fetcher to internal/sourcesync (#807)

Pure mechanical move + relicense (FSL -> Apache-2.0). The Fetcher
interface and Git/OCI/ConfigMap implementations were already a
cleanly-encapsulated package; this just relocates them to core so
non-enterprise controllers (next: SkillSource for #806) can consume
the same fetcher.

Consumers updated:
- ee/internal/controller/{arenasource,arenatemplatesource}_controller{,_test}.go
- ee/internal/controller/fssync{,_test}.go
- ee/internal/controller/credentials.go

No behaviour change. Existing arena tests pass without modification.

Ref #807
EOF
```

---

## Task 2: Move shared CRD schema types from ee to core

**Files:**
- Create: `api/v1alpha1/sourcesync_types.go`
- Modify: `ee/api/v1alpha1/arenasource_types.go`
- Modify: `ee/api/v1alpha1/arenatemplatesource_types.go`
- Modify: any consumer that referenced these by name

Backward-compat strategy: keep type aliases in `ee/api/v1alpha1/` so existing ee code continues to use the short names without import-path churn. The CRD definitions in `ArenaSourceSpec` and `ArenaTemplateSourceSpec` continue to embed the moved types via the alias.

- [ ] **Step 1: Create `api/v1alpha1/sourcesync_types.go`**

Copy the type definitions for `GitReference`, `GitSource`, `OCISource`, `ConfigMapSource`, `Artifact` from `ee/api/v1alpha1/arenasource_types.go` (lines ~32-193). Use Apache-2.0 license header. Keep all `+kubebuilder` markers exactly. Imports needed:

```go
package v1alpha1

import (
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)
```

Note: the `corev1alpha1.SecretKeyRef` reference in `GitSource.SecretRef` and `OCISource.SecretRef` becomes a self-reference (since `SecretKeyRef` lives in this same `api/v1alpha1` package). Drop the `corev1alpha1` qualifier:

Before (in ee):
```go
SecretRef *corev1alpha1.SecretKeyRef `json:"secretRef,omitempty"`
```

After (in core):
```go
SecretRef *SecretKeyRef `json:"secretRef,omitempty"`
```

Verify `SecretKeyRef` exists in `api/v1alpha1/` first:
```
Grep "type SecretKeyRef" path=api/v1alpha1
```
If yes, drop the qualifier. If no (it's only in ee), STOP and report — needs a separate decision about whether to move SecretKeyRef too.

- [ ] **Step 2: Add type aliases in `ee/api/v1alpha1/arenasource_types.go`**

After removing the original type definitions (lines ~32-193 covering the five types listed above), add at the top of the file (after the existing imports):

```go
// Type aliases for backward compatibility — the canonical definitions
// live in api/v1alpha1.
type (
    GitReference    = corev1alpha1.GitReference
    GitSource       = corev1alpha1.GitSource
    OCISource       = corev1alpha1.OCISource
    ConfigMapSource = corev1alpha1.ConfigMapSource
    Artifact        = corev1alpha1.Artifact
)
```

Ensure `corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"` is in the import block.

- [ ] **Step 3: Verify `arenatemplatesource_types.go` still compiles**

It already references the bare `GitSource`, `OCISource`, `ConfigMapSource` names — the aliases keep this working.

```
env GOWORK=off go -C /Users/chaholl/repos/altairalabs/omnia build ./ee/api/...
```

If any field references break, the alias didn't catch them — STOP and report.

- [ ] **Step 4: Regenerate deepcopy methods**

```
env GOWORK=off make -C /Users/chaholl/repos/altairalabs/omnia generate manifests sync-chart-crds
```

Expected:
- `api/v1alpha1/zz_generated.deepcopy.go` — gains DeepCopy methods for the moved types
- `ee/api/v1alpha1/zz_generated.deepcopy.go` — loses those methods (controller-gen sees the aliases and skips them)
- CRD YAMLs in `config/crd/bases/...` and `charts/omnia/...` — diff should be empty (same fields, same schema)

Verify the CRD YAMLs are unchanged:
```
git -C /Users/chaholl/repos/altairalabs/omnia diff config/crd/bases/ charts/omnia/ | head -40
```

If there are field-level differences (e.g. ordering changes in the YAML), that's likely cosmetic from controller-gen and OK. If actual schema fields appear/disappear, STOP and investigate.

- [ ] **Step 5: Build + test**

```
env GOWORK=off go -C /Users/chaholl/repos/altairalabs/omnia build ./...
env GOWORK=off go -C /Users/chaholl/repos/altairalabs/omnia vet ./...
env GOWORK=off go -C /Users/chaholl/repos/altairalabs/omnia test ./api/v1alpha1/... ./ee/api/v1alpha1/... ./ee/internal/controller/... -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```
git add api/v1alpha1/ ee/api/v1alpha1/ config/crd/bases/ charts/omnia/
cat <<'EOF' | git commit -F -
refactor(api): move shared source-sync schema types to core (#807)

GitReference, GitSource, OCISource, ConfigMapSource, and Artifact move
from ee/api/v1alpha1/arenasource_types.go to api/v1alpha1/sourcesync_types.go
under Apache-2.0 license. ArenaSource and ArenaTemplateSource keep
backward compatibility via type aliases — no consumer changes required.

CRD YAMLs unchanged (same fields, same schema; controller-gen sees the
moved types via the aliases).

Ref #807
EOF
```

---

## Task 3: Move FilesystemSyncer to internal/sourcesync

**Files:**
- Create: `internal/sourcesync/syncer.go`
- Create: `internal/sourcesync/syncer_test.go`
- Delete: `ee/internal/controller/fssync.go`
- Delete: `ee/internal/controller/fssync_test.go`
- Modify: `ee/internal/controller/arenasource_controller.go` (one import + one type reference)
- Modify: `ee/internal/controller/arenatemplatesource_controller.go` (same)

The tricky bit: `FilesystemSyncer` depends on `*workspace.StorageManager` from `ee/pkg/workspace`. We can't import that into core. Solution: define a minimal interface in `internal/sourcesync/syncer.go` that the ee `StorageManager` already satisfies (or trivially can):

```go
// StorageManager is the minimal interface FilesystemSyncer needs to ensure a
// workspace PVC exists before writing artifacts. The ee implementation in
// ee/pkg/workspace provides this.
type StorageManager interface {
    EnsureWorkspaceStorage(ctx context.Context, workspaceName, namespace string) error
}
```

Verify the actual method signature on `*ee/pkg/workspace.StorageManager` matches before defining the interface — the ee type has to satisfy it without modification.

- [ ] **Step 1: Inspect the existing StorageManager interface**

```
Grep "func.*StorageManager.*Ensure|func.*StorageManager" path=ee/pkg/workspace -n
```

Read the matching method's signature. Document it inline below.

- [ ] **Step 2: Copy `fssync.go` → `internal/sourcesync/syncer.go`**

```bash
cp ee/internal/controller/fssync.go internal/sourcesync/syncer.go
cp ee/internal/controller/fssync_test.go internal/sourcesync/syncer_test.go
```

Update license header and package name (FSL → Apache-2.0; `package controller` → `package sourcesync`).

- [ ] **Step 3: Define StorageManager interface in syncer.go**

At the top of `internal/sourcesync/syncer.go` (after imports), add:

```go
// StorageManager is the minimal interface FilesystemSyncer needs to ensure a
// workspace PVC exists before writing artifacts. The ee/pkg/workspace
// StorageManager type satisfies this interface.
//
// May be nil — when nil, the syncer skips lazy storage provisioning and
// assumes the PVC is already mounted.
type StorageManager interface {
    EnsureWorkspaceStorage(ctx context.Context, workspaceName, namespace string) error
}
```

Replace the `*workspace.StorageManager` field type on `FilesystemSyncer`:

Before:
```go
StorageManager *workspace.StorageManager
```

After:
```go
StorageManager StorageManager
```

Drop the `"github.com/altairalabs/omnia/ee/pkg/workspace"` import.

Drop the `"github.com/altairalabs/omnia/ee/pkg/arena/fetcher"` import; replace `fetcher.Artifact` references with `Artifact` (same package now).

- [ ] **Step 4: Update arena controllers**

In `ee/internal/controller/arenasource_controller.go`:
- Remove the inline `FilesystemSyncer` reference (it was a type in the same package); add import `"github.com/altairalabs/omnia/internal/sourcesync"` and use `sourcesync.FilesystemSyncer`.
- The `*workspace.StorageManager` field on `ArenaSourceReconciler` keeps its concrete type (ee uses ee). When constructing `FilesystemSyncer`, pass the StorageManager — Go's structural interface satisfaction handles the rest, no cast needed.

Same in `arenatemplatesource_controller.go`.

If the syncer is constructed inline in one of these controllers (e.g. `&FilesystemSyncer{...}`), update to `&sourcesync.FilesystemSyncer{...}`.

- [ ] **Step 5: Delete the originals**

```bash
rm ee/internal/controller/fssync.go ee/internal/controller/fssync_test.go
```

- [ ] **Step 6: Build + test**

```
env GOWORK=off go -C /Users/chaholl/repos/altairalabs/omnia build ./...
env GOWORK=off go -C /Users/chaholl/repos/altairalabs/omnia vet ./...
env GOWORK=off go -C /Users/chaholl/repos/altairalabs/omnia test ./internal/sourcesync/... ./ee/internal/controller/... -count=1
```

Expected: PASS. Coverage on `internal/sourcesync/syncer.go` should equal what `fssync.go` had.

If `EnsureWorkspaceStorage` doesn't exist on `*workspace.StorageManager` with the exact signature in the interface, STOP and report — the interface needs adjustment.

- [ ] **Step 7: Commit**

```
git add internal/sourcesync/syncer.go internal/sourcesync/syncer_test.go ee/internal/controller/
cat <<'EOF' | git commit -F -
refactor: move FilesystemSyncer to internal/sourcesync (#807)

The arena controllers' shared content-addressable storage pipeline
(version write, HEAD pointer, GC) moves to core. Lazy PVC provisioning
becomes a structural interface (StorageManager) so core doesn't need
to import ee/pkg/workspace.

Both arena controllers updated to reference sourcesync.FilesystemSyncer.
No behaviour change — same code, same tests.

Ref #807
EOF
```

---

## Task 4: Move credentials.go to internal/sourcesync

**Files:**
- Create: `internal/sourcesync/credentials.go`
- Delete: `ee/internal/controller/credentials.go`
- Modify: `ee/internal/controller/arenasource_controller.go` and `arenatemplatesource_controller.go` — update references

This is the smallest task. `credentials.go` is 74 lines; resolves Secret-backed credentials for fetchers.

- [ ] **Step 1: Inspect the file's surface**

```
Read path=/Users/chaholl/repos/altairalabs/omnia/ee/internal/controller/credentials.go
```

Note the exported function names (likely `resolveCredentials` or similar, plus possibly types). Document for the next step.

- [ ] **Step 2: Move the file**

```bash
cp ee/internal/controller/credentials.go internal/sourcesync/credentials.go
```

Update license header (FSL → Apache-2.0). Update package (`controller` → `sourcesync`).

If the file has package-private function names (lowercase first letter), they need to become exported (uppercase) to be callable from `ee/internal/controller`. Document the renames in the commit message.

- [ ] **Step 3: Update consumers**

Search for callers in the ee package:

```
Grep "resolveCredentials\\|credentials\\." path=ee/internal/controller -n
```

For each call site, prefix with `sourcesync.` and update the function names if they were renamed in step 2.

- [ ] **Step 4: Delete the original**

```bash
rm ee/internal/controller/credentials.go
```

- [ ] **Step 5: Build + test**

```
env GOWORK=off go -C /Users/chaholl/repos/altairalabs/omnia build ./...
env GOWORK=off go -C /Users/chaholl/repos/altairalabs/omnia vet ./...
env GOWORK=off go -C /Users/chaholl/repos/altairalabs/omnia test ./internal/sourcesync/... ./ee/internal/controller/... -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```
git add internal/sourcesync/credentials.go ee/internal/controller/
cat <<'EOF' | git commit -F -
refactor: move credentials resolver to internal/sourcesync (#807)

Final piece of the source-sync extraction. Arena controllers now
consume the resolver from internal/sourcesync.

Ref #807
EOF
```

---

## Task 5: Final verification + PR

- [ ] **Step 1: Full repo build + vet**

```
env GOWORK=off go -C /Users/chaholl/repos/altairalabs/omnia build ./...
env GOWORK=off go -C /Users/chaholl/repos/altairalabs/omnia vet ./...
```

Both must be clean.

- [ ] **Step 2: Full test suite**

```
env GOWORK=off go -C /Users/chaholl/repos/altairalabs/omnia test ./... -count=1 -timeout 10m
```

Every package green. Specifically check:
- `internal/sourcesync/` — coverage parity with the old `ee/pkg/arena/fetcher/` (should be identical since tests moved verbatim)
- `ee/internal/controller/` — both arena suites pass

- [ ] **Step 3: Lint**

```
env GOWORK=off golangci-lint run ./...
```

Expected: 0 new findings. Update `.golangci.yml` if any rules referencing the old path slipped through.

- [ ] **Step 4: Regen check**

```
env GOWORK=off make -C /Users/chaholl/repos/altairalabs/omnia generate manifests sync-chart-crds generate-dashboard-types
git -C /Users/chaholl/repos/altairalabs/omnia status --short
```

Expected: empty (everything regenerated cleanly during prior tasks). If anything new shows up, run `git add` and amend the most relevant prior commit.

- [ ] **Step 5: Local arena e2e (optional but recommended)**

If kind cluster is available:

```
kind create cluster --name omnia-test-e2e --wait 60s
env GOWORK=off KIND_CLUSTER=omnia-test-e2e E2E_SKIP_CLEANUP=true \
  go test -tags=e2e ./test/e2e/ -v -ginkgo.v -ginkgo.label-filter='!arena' -timeout 20m
```

Then for arena specifically:

```
env GOWORK=off ./scripts/setup-arena-e2e.sh
kubectl config use-context kind-omnia-arena-e2e
env GOWORK=off E2E_SKIP_SETUP=true E2E_PREDEPLOYED=true ENABLE_ARENA_E2E=true \
  E2E_SKIP_CLEANUP=true go test -tags=e2e ./test/e2e/ -v -ginkgo.v \
  -ginkgo.label-filter=arena -timeout 30m
```

Expected: PASS — pure refactor should be invisible to e2e.

If e2e infrastructure isn't immediately available, skip and rely on CI to catch it.

- [ ] **Step 6: Push and open PR**

```
git -C /Users/chaholl/repos/altairalabs/omnia push -u origin <branch-name>
gh pr create --repo AltairaLabs/Omnia --title "refactor: extract internal/sourcesync from arena controllers (#807)" --body "$(cat <<'EOF'
## Summary

Pure refactor — no behaviour change. Closes #807. Prerequisite for #806.

The shared sync infrastructure used by ArenaSource and ArenaTemplateSource moves out of ee/ so the upcoming SkillSource (#806, core) can consume it.

### What moved

| Was | Now | License |
|---|---|---|
| ee/pkg/arena/fetcher/ (entire package) | internal/sourcesync/ | FSL → Apache-2.0 |
| ee/internal/controller/fssync.go | internal/sourcesync/syncer.go | FSL → Apache-2.0 |
| ee/internal/controller/credentials.go | internal/sourcesync/credentials.go | FSL → Apache-2.0 |
| ee/api/v1alpha1/arenasource_types.go (5 shared types) | api/v1alpha1/sourcesync_types.go | FSL → Apache-2.0 |

ArenaSource and ArenaTemplateSource controllers now import from core. Type aliases in ee/api/v1alpha1 keep ee call sites compiling without churn. CRD YAMLs unchanged (same schema, same fields).

A new structural \`StorageManager\` interface in \`internal/sourcesync\` lets \`FilesystemSyncer\` depend on lazy PVC provisioning without importing ee/pkg/workspace — the existing \`*workspace.StorageManager\` satisfies it.

## Test plan

- [x] go build ./... + go vet ./... clean
- [x] Full test suite passes (coverage parity on moved files)
- [x] CRD YAML diff empty (controller-gen sees moved types via aliases)
- [ ] CI green
- [ ] Arena e2e green (local)

## Out of scope

Adding new sources or changing reconciliation semantics — that's #806.
EOF
)"
```

---

## Self-Review

**1. Spec coverage:**
- "Move schema types to core" → Task 2 ✓
- "Extract fetcher to core" → Task 1 ✓
- "Refactor both arena controllers to delegate" → Tasks 1, 3, 4 ✓
- "FilesystemSyncer (versioning, HEAD pointer, GC)" → Task 3 ✓
- "Credentials helper" → Task 4 ✓
- "StorageManager interface" → Task 3 ✓
- "Existing tests pass without modification" → verified at every task step + Task 5
- "CRD YAMLs unchanged" → Task 2 step 4 + Task 5 step 4

**2. Placeholder scan:**
- No "TBD"/"add validation"/"similar to". Task 4 step 2 has a STOP-and-report path if the file has package-private functions — that's a real branch, not a placeholder.
- Code blocks present where steps require code.

**3. Type consistency:**
- `Fetcher`, `Artifact`, `Options`, `GitFetcherConfig`, `OCIFetcherConfig`, `ConfigMapFetcherConfig`, `GitRef`, `NewGitFetcher`, `NewOCIFetcher`, `NewConfigMapFetcher`, `CalculateDirectoryHash` — same names through Tasks 1, 3, 4.
- `FilesystemSyncer`, `StorageManager` — Task 3 only.
- Schema types (`GitReference`, `GitSource`, `OCISource`, `ConfigMapSource`, `Artifact`-CRD) — Task 2 only. Note: the runtime `Artifact` in `internal/sourcesync` and the CRD `Artifact` in `api/v1alpha1` are DIFFERENT structs (one has `Path string`, the other has `URL`/`ContentPath`/`Version`). Pre-flight notes flag this.

**4. Pre-commit hook safety:**
- Every task ends with a build + test verification before commit. Each commit is atomic and self-consistent.
- The known #801 lesson — that the pre-commit hook runs `go build ./...` and `go vet ./...` on the whole repo — is honoured: every task moves files **and** updates consumers in the same commit.

**One known soft spot:** Task 2 step 1 has a STOP path if `SecretKeyRef` doesn't exist in `api/v1alpha1`. If that fires, it's a small inline decision — either move SecretKeyRef too (most likely) or keep `GitSource.SecretRef` typed as a forward-declared local struct. The plan can accommodate either; the implementer will know in 30 seconds.
