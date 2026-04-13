# SessionPrivacyPolicy Binding Redesign + Per-Request Encryption Wiring (Issue #801)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Flip the SessionPrivacyPolicy binding model — policies become reusable documents referenced by `WorkspaceServiceGroup.privacyPolicyRef` and (optionally overridden by) `AgentRuntime.spec.privacyPolicyRef`, replacing the inverted `level` + `workspaceRef`/`agentRef` model. With deterministic policy lookup in place, wire session-api to encrypt/decrypt session data per-request using a cached `Encryptor` keyed on `(kmsProvider, keyID)`.

**Architecture:** Three layers of change — (1) CRD: drop `level`/`workspaceRef`/`agentRef` from `SessionPrivacyPolicy`, add `privacyPolicyRef` to `WorkspaceServiceGroup` (each service group has its own session-api/memory-api, so each can carry its own policy) and `AgentRuntime`; (2) Resolution: `PolicyWatcher.GetEffectivePolicy(namespace, agentName)` becomes a deterministic three-step lookup (AgentRuntime override → service group of the AgentRuntime → global default) with no merge semantics; (3) Encryption: session-api builds a per-request `Encryptor` for each write/read, cached on `(kmsProvider, keyID)` — typically the cache holds one entry (the service group's policy), with extra entries when AgentRuntimes override.

**Tech Stack:** Go 1.x, kubebuilder/controller-runtime CRDs, AES-256-GCM (`ee/pkg/encryption`), KMS provider factories (`ee/pkg/encryption/providers/`), CEL admission validation, sigs.k8s.io webhook framework

**Spec:** `https://github.com/AltairaLabs/Omnia/issues/801`

**Prerequisite:** PR for #780 (branch `feat-780-privacy-recording-encryption`) MUST be merged to main first. This plan assumes the following from #780 is in tree:
- `ee/pkg/encryption/{encryptor,envelope,config_loader,store_wrapper}.go`
- `ee/pkg/privacy/{watcher,resolver}.go` with `EffectivePolicy.Encryption` populated
- `ee/pkg/privacy/middleware.go` recording flag enforcement
- `internal/session/api/handler.go` `PolicyResolver` interface + `GET /api/v1/privacy-policy` endpoint
- `internal/session/httpclient/store.go` `GetPrivacyPolicy()` method
- `internal/facade/recording_policy.go` + `recording_writer.go` recording gate
- `cmd/session-api/main.go` `PolicyResolver` wired (but NO encryption wiring — that was reverted in commit 4038339e and is re-added by this plan)

If #780 has not merged, branch from `feat-780-privacy-recording-encryption` instead of main and merge upward when the parent lands.

---

## File Structure

### Modified files (CRD + types)
| File | Changes |
|------|---------|
| `ee/api/v1alpha1/sessionprivacypolicy_types.go` | Remove `Level`, `WorkspaceRef`, `AgentRef`, the `PolicyLevel` enum + constants, and the four CEL `XValidation` rules tied to `level`. Drop the `Level` printcolumn. Make policy namespace-scoped (remove `+kubebuilder:resource:scope=Cluster`). |
| `api/v1alpha1/workspace_types.go` | Add `PrivacyPolicyRef *corev1.LocalObjectReference` field to `WorkspaceServiceGroup`. Policy is looked up in the workspace namespace (`Namespace.Name`). |
| `api/v1alpha1/agentruntime_types.go` | Add `PrivacyPolicyRef *corev1.LocalObjectReference` field to `AgentRuntimeSpec`. Policy is looked up in the AgentRuntime's own namespace. |

### Modified files (controllers + webhook)
| File | Changes |
|------|---------|
| `ee/internal/controller/sessionprivacypolicy_controller.go` | Strip inheritance/parent/orphan/ConfigMap logic. Reconcile becomes "validate spec, set Active, update status". Remove `findParentPolicy`, `buildInheritanceChain`, `storeEffectivePolicy`, `cleanupEffectivePolicy`, `findChildPolicies`, `requeueChildren`, `isChildOf`, `handleOrphanedPolicy`, `setParentFoundCondition`, `buildAgentChain`. |
| `ee/internal/controller/sessionprivacypolicy_controller_test.go` | Rewrite to cover the simplified reconciler (Active phase, Generation tracking, no parent lookup). |
| `ee/internal/webhook/sessionprivacypolicy_webhook.go` | Remove `validateInheritance`, `findParentPolicy`, `validateStricterThanParent`, `validateRetentionNotExceeded`, `findPolicyByLevel`. Replace `ValidateDelete`'s "last global" check with "no Workspace or AgentRuntime references this policy" check. |
| `ee/internal/webhook/sessionprivacypolicy_webhook_test.go` | Rewrite: drop inheritance tests, add "delete blocked when referenced" tests. |
| `internal/controller/workspace_controller.go` | Add validation: for each service group with `privacyPolicyRef` set, surface a per-group condition on the Workspace if the referenced policy doesn't exist. Status condition only — do not block reconciliation. |
| `internal/controller/agentruntime_controller.go` | Same pattern: surface a condition when `spec.privacyPolicyRef` doesn't resolve. Don't block reconciliation. |

### Modified files (resolver + watcher)
| File | Changes |
|------|---------|
| `ee/pkg/privacy/watcher.go` | Watcher now also reads `Workspace` and `AgentRuntime` objects to resolve refs. `GetEffectivePolicy(namespace, agentName)` becomes deterministic: 1) load AgentRuntime in namespace by name → if it has `PrivacyPolicyRef`, return that policy; 2) read the AgentRuntime's `spec.serviceGroup` (default `"default"`); load Workspace whose `Namespace.Name == namespace`; find the matching `WorkspaceServiceGroup` by name → if it has `PrivacyPolicyRef`, return that policy; 3) return global default `SessionPrivacyPolicy` named `default` in `omnia-system`. Remove `buildPolicyChain`, `findByLevel`, `collectPolicies`. |
| `ee/pkg/privacy/watcher_test.go` | Rewrite: tests for direct AgentRuntime → Workspace → global default lookup; tests for missing refs falling through. |
| `ee/pkg/privacy/resolver.go` | No structural change — `ResolveEffectivePolicy(namespace, agentName)` still returns JSON-marshalled facade subset. Logic now drives off the new lookup. |
| `ee/pkg/privacy/resolver_test.go` | Update to match new lookup behavior. |
| `ee/pkg/privacy/inheritance.go` (or wherever `ComputeEffectivePolicy` lives) | DELETE — no merge logic needed. |
| `ee/pkg/privacy/middleware.go` | No signature change. Verify it still calls `GetEffectivePolicy(namespace, agent)`. |

### New files (encryption wiring)
| File | Responsibility |
|------|---------------|
| `internal/session/api/encryptor.go` | `Encryptor` interface (no `ee/` import) + `EncryptorResolver` interface that returns the encryptor for a given session ID. |
| `internal/session/api/encryptor_test.go` | Unit tests for the resolver dispatch path. |
| `cmd/session-api/encryption_resolver.go` | Concrete `PerPolicyEncryptorResolver` — takes `PolicyWatcher` + a `KMSProviderFactory`, caches built encryptors keyed on `(kmsProvider, keyID)`, invalidates entries when the `EncryptionConfig` for a session's policy changes. |
| `cmd/session-api/encryption_resolver_test.go` | Cache hit/miss/invalidation tests + multi-workspace coexistence tests. |

### Modified files (encryption + handler)
| File | Changes |
|------|---------|
| `internal/session/api/handler.go` | Replace single `Encryptor` with `EncryptorResolver`. `SetEncryptor` becomes `SetEncryptorResolver`. Each write/read handler resolves the encryptor for the session, then encrypts/decrypts as before. Sessions whose resolved policy has `Encryption.Enabled == false` get the no-op encryptor. |
| `internal/session/api/encryption_test.go` | Update mocks: `MockEncryptorResolver` returning per-session encryptors; tests for "session A uses KMS-X, session B uses KMS-Y in same handler instance". |
| `cmd/session-api/main.go` | Replace `buildEncryptorFromPolicy` (reverted in 4038339e) with construction of `PerPolicyEncryptorResolver`. Pass it to `Handler.SetEncryptorResolver`. Remove the dropped startup `omnia-system` global-policy read. |
| `cmd/session-api/wiring_test.go` | Add wiring assertion: `Handler.encryptorResolver != nil` when enterprise mode is on. |
| `internal/doctor/checks/privacy.go` | Re-add `SessionEncryptionAtRest` check. Per the new model, the check writes a probe message under a workspace whose policy has encryption enabled, then reads the raw DB row and asserts `enc:v1:` prefix. |
| `internal/doctor/checks/privacy_test.go` | Tests for the revived encryption check. |

### Helm chart + RBAC + CRD manifests
| File | Changes |
|------|---------|
| `config/crd/bases/omnia.altairalabs.ai_sessionprivacypolicies.yaml` | Regenerated by `make manifests` after type changes. |
| `config/crd/bases/omnia.altairalabs.ai_workspaces.yaml` | Regenerated. |
| `config/crd/bases/omnia.altairalabs.ai_agentruntimes.yaml` | Regenerated. |
| `charts/omnia/crds/*.yaml` | `make sync-chart-crds` copies the new CRDs. |
| `config/rbac/role.yaml` | Regenerated; `PolicyWatcher` now needs `get,list,watch` on `workspaces` and `agentruntimes` (in addition to existing `sessionprivacypolicies`). Add `+kubebuilder:rbac` markers to `ee/pkg/privacy/watcher.go` or a sentinel file. |

### Docs
| File | Changes |
|------|---------|
| `docs/src/content/docs/reference/sessionprivacypolicy.md` | NEW — describes the reusable policy document shape (no `level` field), how Workspace and AgentRuntime reference it, and the resolution order. |
| `docs/src/content/docs/how-to/configure-privacy-policies.md` | NEW — walkthrough: define a policy, attach to Workspace, override on AgentRuntime, verify with `kubectl get workspace -o jsonpath`. |
| `cmd/session-api/SERVICE.md` | Document the per-request encryption resolver and `(kmsProvider, keyID)` cache. |
| `api/CHANGELOG.md` | Entry for: removed `SessionPrivacyPolicy.spec.level`/`workspaceRef`/`agentRef`; added `Workspace.spec.privacyPolicyRef`; added `AgentRuntime.spec.privacyPolicyRef`. |
| `dashboard/src/types/generated/*.ts` | Regenerated by `make generate-dashboard-types`. |

### Migration
| File | Changes |
|------|---------|
| `hack/migrations/2026-04-13-privacy-policy-redesign.md` | Migration playbook: list existing `level`-based policies (`kubectl get sessionprivacypolicies`), for each old `workspace`-level policy, edit the corresponding Workspace to add `spec.privacyPolicyRef`; same for agent-level. Note: existing deployments are dev-only per project status, so migration is documentation, not automation. |

---

## Task 1: Remove `level` from SessionPrivacyPolicy CRD types

**Files:**
- Modify: `ee/api/v1alpha1/sessionprivacypolicy_types.go`

- [ ] **Step 1: Edit the type file**

Remove these from `ee/api/v1alpha1/sessionprivacypolicy_types.go`:

```
// Lines to delete:
// - PolicyLevel type + 3 constants (PolicyLevelGlobal/Workspace/Agent)
// - SessionPrivacyPolicySpec fields: Level, WorkspaceRef, AgentRef
// - The 4 +kubebuilder:validation:XValidation rules on SessionPrivacyPolicySpec that reference `self.level`
// - The +kubebuilder:printcolumn:name="Level" line
// - +kubebuilder:resource:scope=Cluster   (policies become namespaced)
```

The new `SessionPrivacyPolicySpec` is just:

```go
type SessionPrivacyPolicySpec struct {
    // recording configures what session data is recorded.
    // +kubebuilder:validation:Required
    Recording RecordingConfig `json:"recording"`

    // retention configures privacy-specific retention overrides.
    // +optional
    Retention *PrivacyRetentionConfig `json:"retention,omitempty"`

    // userOptOut configures user opt-out and data deletion capabilities.
    // +optional
    UserOptOut *UserOptOutConfig `json:"userOptOut,omitempty"`

    // encryption configures encryption for session data at rest.
    // +optional
    Encryption *EncryptionConfig `json:"encryption,omitempty"`

    // auditLog configures audit logging for privacy-related operations.
    // +optional
    AuditLog *AuditLogConfig `json:"auditLog,omitempty"`
}
```

- [ ] **Step 2: Regenerate deepcopy + manifests**

Run:
```
make generate
make manifests
```

Expected: `zz_generated.deepcopy.go` updates; `config/crd/bases/omnia.altairalabs.ai_sessionprivacypolicies.yaml` no longer contains `level`/`workspaceRef`/`agentRef`.

- [ ] **Step 3: Verify build**

Run: `env GOWORK=off go build ./ee/...`
Expected: Build fails in `ee/internal/controller/sessionprivacypolicy_controller.go` and `ee/internal/webhook/sessionprivacypolicy_webhook.go` (referencing removed `Level`/`WorkspaceRef`/`AgentRef`). These are addressed in tasks 4 and 5.

- [ ] **Step 4: Commit**

```
git add ee/api/v1alpha1/sessionprivacypolicy_types.go ee/api/v1alpha1/zz_generated.deepcopy.go config/crd/bases/omnia.altairalabs.ai_sessionprivacypolicies.yaml
cat <<'EOF' | git commit -F -
feat(api)!: drop level/workspaceRef/agentRef from SessionPrivacyPolicy

Policies become reusable documents. Binding moves to consumers (Workspace
and AgentRuntime) via privacyPolicyRef in subsequent commits.

Ref #801
EOF
```

---

## Task 2: Add `PrivacyPolicyRef` to WorkspaceServiceGroup

**Files:**
- Modify: `api/v1alpha1/workspace_types.go`

- [ ] **Step 1: Add the field**

In `api/v1alpha1/workspace_types.go`, in `WorkspaceServiceGroup` (line ~564, the struct that already holds `memory`/`session`/`external`), add after `External`:

```go
    // privacyPolicyRef references a SessionPrivacyPolicy that applies to sessions
    // managed by this service group's session-api. The referenced policy must live
    // in the workspace's namespace (spec.namespace.name).
    // If unset, the global default policy is used.
    // +optional
    PrivacyPolicyRef *corev1.LocalObjectReference `json:"privacyPolicyRef,omitempty"`
```

- [ ] **Step 2: Regenerate**

Run: `make generate && make manifests`

- [ ] **Step 3: Sanity-check the CRD**

Run: `Grep` for `privacyPolicyRef` in `config/crd/bases/omnia.altairalabs.ai_workspaces.yaml`.
Expected: field present under `services[].properties` with `LocalObjectReference` schema.

- [ ] **Step 4: Commit**

```
git add api/v1alpha1/workspace_types.go api/v1alpha1/zz_generated.deepcopy.go config/crd/bases/omnia.altairalabs.ai_workspaces.yaml
cat <<'EOF' | git commit -F -
feat(api): add WorkspaceServiceGroup.privacyPolicyRef

Each service group references a reusable SessionPrivacyPolicy. Each
service group runs its own session-api, so each can carry its own
privacy and encryption configuration. Resolved by PolicyWatcher in a
later commit.

Ref #801
EOF
```

---

## Task 3: Add `PrivacyPolicyRef` to AgentRuntime

**Files:**
- Modify: `api/v1alpha1/agentruntime_types.go`

- [ ] **Step 1: Add the field**

In `AgentRuntimeSpec` (line 1057), add after `Rollout`:

```go
    // privacyPolicyRef references a SessionPrivacyPolicy that overrides the
    // workspace's policy for this specific agent. Looked up in the AgentRuntime's
    // own namespace.
    // +optional
    PrivacyPolicyRef *corev1.LocalObjectReference `json:"privacyPolicyRef,omitempty"`
```

- [ ] **Step 2: Regenerate**

Run: `make generate && make manifests`

- [ ] **Step 3: Commit**

```
git add api/v1alpha1/agentruntime_types.go api/v1alpha1/zz_generated.deepcopy.go config/crd/bases/omnia.altairalabs.ai_agentruntimes.yaml
cat <<'EOF' | git commit -F -
feat(api): add AgentRuntime.spec.privacyPolicyRef

Per-agent override that takes precedence over the workspace's policy.

Ref #801
EOF
```

---

## Task 4: Simplify SessionPrivacyPolicy controller

**Files:**
- Modify: `ee/internal/controller/sessionprivacypolicy_controller.go`
- Modify: `ee/internal/controller/sessionprivacypolicy_controller_test.go`

- [ ] **Step 1: Write the new failing test**

Replace the bulk of `sessionprivacypolicy_controller_test.go` with a focused suite. Add at the top:

```go
var _ = Describe("SessionPrivacyPolicyReconciler (simplified)", func() {
    Context("when reconciling a valid policy", func() {
        It("sets phase=Active and observedGeneration", func() {
            ctx := context.Background()
            policy := &omniav1alpha1.SessionPrivacyPolicy{
                ObjectMeta: metav1.ObjectMeta{Name: "test-policy", Namespace: "default"},
                Spec: omniav1alpha1.SessionPrivacyPolicySpec{
                    Recording: omniav1alpha1.RecordingConfig{Enabled: true, RichData: true},
                },
            }
            Expect(k8sClient.Create(ctx, policy)).To(Succeed())
            DeferCleanup(func() { _ = k8sClient.Delete(ctx, policy) })

            reconciler := &SessionPrivacyPolicyReconciler{
                Client: k8sClient, Scheme: scheme.Scheme, Recorder: record.NewFakeRecorder(10),
            }
            _, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "test-policy", Namespace: "default"}})
            Expect(err).NotTo(HaveOccurred())

            updated := &omniav1alpha1.SessionPrivacyPolicy{}
            Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "test-policy", Namespace: "default"}, updated)).To(Succeed())
            Expect(updated.Status.Phase).To(Equal(omniav1alpha1.SessionPrivacyPolicyPhaseActive))
            Expect(updated.Status.ObservedGeneration).To(Equal(updated.Generation))
        })
    })
})
```

- [ ] **Step 2: Run test to verify failure**

Run: `env GOWORK=off go test ./ee/internal/controller/ -run TestSessionPrivacyPolicy -count=1 -v`
Expected: compile failure or failure ("Level field undefined"/"phase not set").

- [ ] **Step 3: Rewrite the controller**

Replace `ee/internal/controller/sessionprivacypolicy_controller.go` body with:

```go
package controller

import (
    "context"

    apierrors "k8s.io/apimachinery/pkg/api/errors"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/client-go/tools/record"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    logf "sigs.k8s.io/controller-runtime/pkg/log"

    omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
    "github.com/altairalabs/omnia/ee/pkg/metrics"
)

const (
    ConditionTypeReady          = "Ready"
    EventReasonPolicyValidated  = "PolicyValidated"
)

type SessionPrivacyPolicyReconciler struct {
    client.Client
    Scheme   *runtime.Scheme
    Recorder record.EventRecorder
    Metrics  *metrics.PrivacyPolicyMetrics
}

// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=sessionprivacypolicies,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=sessionprivacypolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

func (r *SessionPrivacyPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := logf.FromContext(ctx)
    policy := &omniav1alpha1.SessionPrivacyPolicy{}
    if err := r.Get(ctx, req.NamespacedName, policy); err != nil {
        if apierrors.IsNotFound(err) {
            return ctrl.Result{}, nil
        }
        return ctrl.Result{}, err
    }

    policy.Status.ObservedGeneration = policy.Generation
    SetCondition(&policy.Status.Conditions, policy.Generation, ConditionTypeReady,
        metav1.ConditionTrue, EventReasonPolicyValidated, "policy is active")
    policy.Status.Phase = omniav1alpha1.SessionPrivacyPolicyPhaseActive

    if r.Recorder != nil {
        r.Recorder.Event(policy, "Normal", EventReasonPolicyValidated, "Policy validated and active")
    }
    if r.Metrics != nil {
        r.Metrics.RecordEffectivePolicyComputation(policy.Name)
    }

    if err := r.Status().Update(ctx, policy); err != nil {
        log.Error(err, "failed to update status")
        return ctrl.Result{}, err
    }
    return ctrl.Result{}, nil
}

func (r *SessionPrivacyPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&omniav1alpha1.SessionPrivacyPolicy{}).
        Named("sessionprivacypolicy").
        Complete(r)
}
```

The old metric helpers `RecordReconcileError`, `RecordConfigMapSyncError`, `SetInheritanceDepth`, `SetActivePolicies` may now be unreferenced. Leave them in `ee/pkg/metrics/` for now — they'll be cleaned up in Task 14 alongside the dashboard scrape config.

- [ ] **Step 4: Run tests**

Run: `env GOWORK=off go test ./ee/internal/controller/ -count=1 -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```
git add ee/internal/controller/sessionprivacypolicy_controller.go ee/internal/controller/sessionprivacypolicy_controller_test.go
cat <<'EOF' | git commit -F -
refactor(controller): simplify SessionPrivacyPolicy reconciler

No more inheritance chain, parent lookup, ConfigMap distribution, or
orphaned-policy handling. Reconcile validates spec and sets Active.
Resolution moves to PolicyWatcher (separate commit).

Ref #801
EOF
```

---

## Task 5: Replace inheritance webhook with reference-existence webhook

**Files:**
- Modify: `ee/internal/webhook/sessionprivacypolicy_webhook.go`
- Modify: `ee/internal/webhook/sessionprivacypolicy_webhook_test.go`

- [ ] **Step 1: Write failing tests**

Replace the existing test suite in `sessionprivacypolicy_webhook_test.go` with:

```go
var _ = Describe("SessionPrivacyPolicyValidator", func() {
    var validator *SessionPrivacyPolicyValidator
    ctx := context.Background()

    BeforeEach(func() {
        validator = &SessionPrivacyPolicyValidator{Client: k8sClient}
    })

    Context("ValidateCreate", func() {
        It("accepts any well-formed policy", func() {
            policy := &omniav1alpha1.SessionPrivacyPolicy{
                ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default"},
                Spec: omniav1alpha1.SessionPrivacyPolicySpec{
                    Recording: omniav1alpha1.RecordingConfig{Enabled: true},
                },
            }
            warnings, err := validator.ValidateCreate(ctx, policy)
            Expect(err).NotTo(HaveOccurred())
            Expect(warnings).To(BeEmpty())
        })
    })

    Context("ValidateDelete", func() {
        It("blocks deletion when a Workspace service group references the policy", func() {
            policy := &omniav1alpha1.SessionPrivacyPolicy{
                ObjectMeta: metav1.ObjectMeta{Name: "in-use", Namespace: "default"},
                Spec:       omniav1alpha1.SessionPrivacyPolicySpec{Recording: omniav1alpha1.RecordingConfig{Enabled: true}},
            }
            Expect(k8sClient.Create(ctx, policy)).To(Succeed())
            DeferCleanup(func() { _ = k8sClient.Delete(ctx, policy) })

            ws := &corev1alpha1.Workspace{
                ObjectMeta: metav1.ObjectMeta{Name: "ws-1"},
                Spec: corev1alpha1.WorkspaceSpec{
                    DisplayName: "ws",
                    Namespace:   corev1alpha1.NamespaceConfig{Name: "default"},
                    Services: []corev1alpha1.WorkspaceServiceGroup{{
                        Name:             "default",
                        Mode:             corev1alpha1.ServiceModeManaged,
                        Memory:           &corev1alpha1.MemoryServiceConfig{ /* minimal valid */ },
                        Session:          &corev1alpha1.SessionServiceConfig{ /* minimal valid */ },
                        PrivacyPolicyRef: &corev1.LocalObjectReference{Name: "in-use"},
                    }},
                },
            }
            Expect(k8sClient.Create(ctx, ws)).To(Succeed())
            DeferCleanup(func() { _ = k8sClient.Delete(ctx, ws) })

            _, err := validator.ValidateDelete(ctx, policy)
            Expect(err).To(MatchError(ContainSubstring(`service group "default"`)))
        })

        It("allows deletion when no consumer references the policy", func() {
            policy := &omniav1alpha1.SessionPrivacyPolicy{
                ObjectMeta: metav1.ObjectMeta{Name: "free", Namespace: "default"},
                Spec:       omniav1alpha1.SessionPrivacyPolicySpec{Recording: omniav1alpha1.RecordingConfig{Enabled: true}},
            }
            Expect(k8sClient.Create(ctx, policy)).To(Succeed())
            DeferCleanup(func() { _ = k8sClient.Delete(ctx, policy) })

            warnings, err := validator.ValidateDelete(ctx, policy)
            Expect(err).NotTo(HaveOccurred())
            Expect(warnings).To(BeEmpty())
        })
    })
})
```

- [ ] **Step 2: Run to confirm failure**

Run: `env GOWORK=off go test ./ee/internal/webhook/ -count=1 -v`
Expected: compile failure (`PrivacyPolicyRef` undefined on Workspace) or failures referencing missing helpers.

- [ ] **Step 3: Rewrite the webhook**

Replace `ee/internal/webhook/sessionprivacypolicy_webhook.go` with:

```go
package webhook

import (
    "context"
    "fmt"

    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    logf "sigs.k8s.io/controller-runtime/pkg/log"
    "sigs.k8s.io/controller-runtime/pkg/webhook/admission"

    corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
    omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

type SessionPrivacyPolicyValidator struct {
    Client client.Reader
}

var privacypolicylog = logf.Log.WithName("sessionprivacypolicy-webhook")

func SetupSessionPrivacyPolicyWebhookWithManager(mgr ctrl.Manager) error {
    return ctrl.NewWebhookManagedBy(mgr, &omniav1alpha1.SessionPrivacyPolicy{}).
        WithValidator(&SessionPrivacyPolicyValidator{Client: mgr.GetClient()}).
        Complete()
}

// +kubebuilder:webhook:path=/validate-omnia-altairalabs-ai-v1alpha1-sessionprivacypolicy,mutating=false,failurePolicy=fail,sideEffects=None,groups=omnia.altairalabs.ai,resources=sessionprivacypolicies,verbs=create;update;delete,versions=v1alpha1,name=vsessionprivacypolicy.kb.io,admissionReviewVersions=v1

var _ admission.Validator[*omniav1alpha1.SessionPrivacyPolicy] = &SessionPrivacyPolicyValidator{}

func (v *SessionPrivacyPolicyValidator) ValidateCreate(_ context.Context, policy *omniav1alpha1.SessionPrivacyPolicy) (admission.Warnings, error) {
    privacypolicylog.Info("validating create", "name", policy.Name)
    return nil, nil
}

func (v *SessionPrivacyPolicyValidator) ValidateUpdate(_ context.Context, _, policy *omniav1alpha1.SessionPrivacyPolicy) (admission.Warnings, error) {
    privacypolicylog.Info("validating update", "name", policy.Name)
    return nil, nil
}

func (v *SessionPrivacyPolicyValidator) ValidateDelete(ctx context.Context, policy *omniav1alpha1.SessionPrivacyPolicy) (admission.Warnings, error) {
    privacypolicylog.Info("validating delete", "name", policy.Name)

    var workspaces corev1alpha1.WorkspaceList
    if err := v.Client.List(ctx, &workspaces); err != nil {
        return nil, fmt.Errorf("listing workspaces: %w", err)
    }
    for i := range workspaces.Items {
        ws := &workspaces.Items[i]
        // Only service groups whose namespace matches the policy's namespace can reference it.
        if ws.Spec.Namespace.Name != policy.Namespace {
            continue
        }
        for j := range ws.Spec.Services {
            sg := &ws.Spec.Services[j]
            if sg.PrivacyPolicyRef != nil && sg.PrivacyPolicyRef.Name == policy.Name {
                return nil, fmt.Errorf("policy %q is referenced by Workspace %s service group %q and cannot be deleted",
                    policy.Name, ws.Name, sg.Name)
            }
        }
    }

    var agents corev1alpha1.AgentRuntimeList
    if err := v.Client.List(ctx, &agents, client.InNamespace(policy.Namespace)); err != nil {
        return nil, fmt.Errorf("listing agentruntimes: %w", err)
    }
    for i := range agents.Items {
        a := &agents.Items[i]
        if a.Spec.PrivacyPolicyRef != nil && a.Spec.PrivacyPolicyRef.Name == policy.Name {
            return nil, fmt.Errorf("policy %q is referenced by AgentRuntime %s/%s and cannot be deleted", policy.Name, a.Namespace, a.Name)
        }
    }

    return nil, nil
}
```

- [ ] **Step 4: Run tests**

Run: `env GOWORK=off go test ./ee/internal/webhook/ -count=1 -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```
git add ee/internal/webhook/sessionprivacypolicy_webhook.go ee/internal/webhook/sessionprivacypolicy_webhook_test.go
cat <<'EOF' | git commit -F -
refactor(webhook): replace inheritance check with reference-existence check

ValidateDelete now blocks removal when a Workspace or AgentRuntime
references the policy. ValidateCreate/Update are pass-through —
schema-level CEL handles structural validation.

Ref #801
EOF
```

---

## Task 6: Rewrite PolicyWatcher with deterministic lookup

**Files:**
- Modify: `ee/pkg/privacy/watcher.go`
- Modify: `ee/pkg/privacy/watcher_test.go`
- Delete (or empty): `ee/pkg/privacy/inheritance.go` (only if it exists with `ComputeEffectivePolicy`)

- [ ] **Step 1: Write failing tests**

Append to `ee/pkg/privacy/watcher_test.go`:

```go
func TestGetEffectivePolicy_AgentRefWins(t *testing.T) {
    cl := fake.NewClientBuilder().WithScheme(scheme).
        WithObjects(
            policyFixture("global-default", "omnia-system"),
            policyFixture("group-policy", "ws-ns"),
            policyFixture("agent-policy", "ws-ns"),
            workspaceFixture("ws-1", "ws-ns", map[string]string{"default": "group-policy"}),
            agentRuntimeFixture("agent-1", "ws-ns", "default", "agent-policy"),
        ).Build()
    w := NewPolicyWatcher(cl, logr.Discard())
    require.NoError(t, w.loadAll(context.Background()))

    eff := w.GetEffectivePolicy("ws-ns", "agent-1")
    require.NotNil(t, eff)
    assert.Equal(t, "agent-policy", w.lastResolvedName) // helper for assertions
}

func TestGetEffectivePolicy_ServiceGroupFallback(t *testing.T) {
    cl := fake.NewClientBuilder().WithScheme(scheme).
        WithObjects(
            policyFixture("group-policy", "ws-ns"),
            workspaceFixture("ws-1", "ws-ns", map[string]string{"default": "group-policy"}),
            agentRuntimeFixture("agent-1", "ws-ns", "default", ""), // no override
        ).Build()
    w := NewPolicyWatcher(cl, logr.Discard())
    require.NoError(t, w.loadAll(context.Background()))

    eff := w.GetEffectivePolicy("ws-ns", "agent-1")
    require.NotNil(t, eff)
    assert.Equal(t, "group-policy", w.lastResolvedName)
}

func TestGetEffectivePolicy_NamedServiceGroup(t *testing.T) {
    // Two service groups in one workspace, each with its own policy.
    // Agent uses serviceGroup="prod"; expect "prod-policy".
    cl := fake.NewClientBuilder().WithScheme(scheme).
        WithObjects(
            policyFixture("dev-policy", "ws-ns"),
            policyFixture("prod-policy", "ws-ns"),
            workspaceFixture("ws-1", "ws-ns", map[string]string{
                "default": "dev-policy",
                "prod":    "prod-policy",
            }),
            agentRuntimeFixture("agent-prod", "ws-ns", "prod", ""),
        ).Build()
    w := NewPolicyWatcher(cl, logr.Discard())
    require.NoError(t, w.loadAll(context.Background()))

    eff := w.GetEffectivePolicy("ws-ns", "agent-prod")
    require.NotNil(t, eff)
    assert.Equal(t, "prod-policy", w.lastResolvedName)
}

func TestGetEffectivePolicy_GlobalDefault(t *testing.T) {
    cl := fake.NewClientBuilder().WithScheme(scheme).
        WithObjects(
            policyFixture("default", "omnia-system"),
            workspaceFixture("ws-1", "ws-ns", map[string]string{"default": ""}),
            agentRuntimeFixture("agent-1", "ws-ns", "default", ""),
        ).Build()
    w := NewPolicyWatcher(cl, logr.Discard())
    require.NoError(t, w.loadAll(context.Background()))

    eff := w.GetEffectivePolicy("ws-ns", "agent-1")
    require.NotNil(t, eff)
    assert.Equal(t, "default", w.lastResolvedName)
}

func TestGetEffectivePolicy_NoPolicy(t *testing.T) {
    cl := fake.NewClientBuilder().WithScheme(scheme).Build()
    w := NewPolicyWatcher(cl, logr.Discard())
    require.NoError(t, w.loadAll(context.Background()))

    assert.Nil(t, w.GetEffectivePolicy("ws-ns", "agent-1"))
}
```

Add fixture helpers:

```go
func policyFixture(name, ns string) *omniav1alpha1.SessionPrivacyPolicy {
    return &omniav1alpha1.SessionPrivacyPolicy{
        ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
        Spec: omniav1alpha1.SessionPrivacyPolicySpec{
            Recording: omniav1alpha1.RecordingConfig{Enabled: true, RichData: true},
        },
    }
}

// workspaceFixture creates a workspace with one service group per entry in
// groupPolicies (group name → policy name; "" means no policy ref on that group).
func workspaceFixture(name, ns string, groupPolicies map[string]string) *corev1alpha1.Workspace {
    ws := &corev1alpha1.Workspace{
        ObjectMeta: metav1.ObjectMeta{Name: name},
        Spec: corev1alpha1.WorkspaceSpec{
            DisplayName: name,
            Namespace:   corev1alpha1.NamespaceConfig{Name: ns},
        },
    }
    for groupName, polName := range groupPolicies {
        sg := corev1alpha1.WorkspaceServiceGroup{
            Name: groupName,
            Mode: corev1alpha1.ServiceModeManaged,
        }
        if polName != "" {
            sg.PrivacyPolicyRef = &corev1.LocalObjectReference{Name: polName}
        }
        ws.Spec.Services = append(ws.Spec.Services, sg)
    }
    return ws
}

func agentRuntimeFixture(name, ns, serviceGroup, policyName string) *corev1alpha1.AgentRuntime {
    a := &corev1alpha1.AgentRuntime{
        ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
        Spec:       corev1alpha1.AgentRuntimeSpec{ServiceGroup: serviceGroup},
    }
    if policyName != "" {
        a.Spec.PrivacyPolicyRef = &corev1.LocalObjectReference{Name: policyName}
    }
    return a
}
```

- [ ] **Step 2: Run tests to confirm failure**

Run: `env GOWORK=off go test ./ee/pkg/privacy/ -run TestGetEffectivePolicy -count=1 -v`
Expected: compile or assertion failures.

- [ ] **Step 3: Rewrite watcher.go**

Replace `ee/pkg/privacy/watcher.go` with:

```go
package privacy

import (
    "context"
    "fmt"
    "sync"
    "time"

    "github.com/go-logr/logr"
    apierrors "k8s.io/apimachinery/pkg/api/errors"
    "k8s.io/apimachinery/pkg/types"
    "sigs.k8s.io/controller-runtime/pkg/client"

    corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
    omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

const (
    // GlobalDefaultPolicyName is the well-known name for the global default policy.
    GlobalDefaultPolicyName = "default"
    // GlobalDefaultPolicyNamespace is the namespace of the global default policy.
    GlobalDefaultPolicyNamespace = "omnia-system"
)

type EffectivePolicy struct {
    Recording  omniav1alpha1.RecordingConfig
    UserOptOut *omniav1alpha1.UserOptOutConfig
    Encryption omniav1alpha1.EncryptionConfig
}

type PolicyWatcher struct {
    client       client.Client
    policies     sync.Map // key: "namespace/name" -> *SessionPrivacyPolicy
    workspaces   sync.Map // key: namespace string -> *Workspace
    agents       sync.Map // key: "namespace/name" -> *AgentRuntime
    pollInterval time.Duration
    log          logr.Logger
    lastResolvedName string // exported for tests via accessor; remove for prod if unwanted
}

// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=workspaces,verbs=get;list;watch
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=agentruntimes,verbs=get;list;watch

func NewPolicyWatcher(k8sClient client.Client, log logr.Logger) *PolicyWatcher {
    return &PolicyWatcher{
        client:       k8sClient,
        pollInterval: 30 * time.Second,
        log:          log.WithName("policy-watcher"),
    }
}

func (w *PolicyWatcher) SetPollInterval(d time.Duration) { w.pollInterval = d }

func (w *PolicyWatcher) Start(ctx context.Context) error {
    if err := w.loadAll(ctx); err != nil {
        return fmt.Errorf("initial load: %w", err)
    }
    w.log.Info("policy watcher synced")
    ticker := time.NewTicker(w.pollInterval)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return nil
        case <-ticker.C:
            if err := w.loadAll(ctx); err != nil {
                w.log.Error(err, "reload failed")
            }
        }
    }
}

func (w *PolicyWatcher) loadAll(ctx context.Context) error {
    if err := w.loadPolicies(ctx); err != nil {
        return err
    }
    if err := w.loadWorkspaces(ctx); err != nil {
        return err
    }
    return w.loadAgentRuntimes(ctx)
}

func (w *PolicyWatcher) loadPolicies(ctx context.Context) error {
    var list omniav1alpha1.SessionPrivacyPolicyList
    if err := w.client.List(ctx, &list); err != nil {
        return fmt.Errorf("list policies: %w", err)
    }
    seen := map[string]bool{}
    for i := range list.Items {
        p := &list.Items[i]
        key := p.Namespace + "/" + p.Name
        seen[key] = true
        w.policies.Store(key, p.DeepCopy())
    }
    w.policies.Range(func(k, _ any) bool {
        if !seen[k.(string)] {
            w.policies.Delete(k)
        }
        return true
    })
    return nil
}

func (w *PolicyWatcher) loadWorkspaces(ctx context.Context) error {
    var list corev1alpha1.WorkspaceList
    if err := w.client.List(ctx, &list); err != nil {
        return fmt.Errorf("list workspaces: %w", err)
    }
    seen := map[string]bool{}
    for i := range list.Items {
        ws := &list.Items[i]
        key := ws.Spec.Namespace.Name
        seen[key] = true
        w.workspaces.Store(key, ws.DeepCopy())
    }
    w.workspaces.Range(func(k, _ any) bool {
        if !seen[k.(string)] {
            w.workspaces.Delete(k)
        }
        return true
    })
    return nil
}

func (w *PolicyWatcher) loadAgentRuntimes(ctx context.Context) error {
    var list corev1alpha1.AgentRuntimeList
    if err := w.client.List(ctx, &list); err != nil {
        return fmt.Errorf("list agentruntimes: %w", err)
    }
    seen := map[string]bool{}
    for i := range list.Items {
        a := &list.Items[i]
        key := a.Namespace + "/" + a.Name
        seen[key] = true
        w.agents.Store(key, a.DeepCopy())
    }
    w.agents.Range(func(k, _ any) bool {
        if !seen[k.(string)] {
            w.agents.Delete(k)
        }
        return true
    })
    return nil
}

// GetEffectivePolicy returns the effective policy for a session running under
// (namespace, agentName), using deterministic lookup:
//   1. AgentRuntime.spec.privacyPolicyRef (if set)
//   2. WorkspaceServiceGroup.privacyPolicyRef for the agent's serviceGroup
//      (workspace identified by its Namespace.Name; group looked up by name)
//   3. global default at omnia-system/default
// Returns nil when no policy is found.
func (w *PolicyWatcher) GetEffectivePolicy(namespace, agentName string) *EffectivePolicy {
    agent := w.lookupAgent(namespace, agentName)

    if agent != nil && agent.Spec.PrivacyPolicyRef != nil {
        if p := w.lookupPolicy(namespace, agent.Spec.PrivacyPolicyRef.Name); p != nil {
            return policyToEffective(p, agent.Spec.PrivacyPolicyRef.Name, w)
        }
    }

    serviceGroup := defaultServiceGroup
    if agent != nil && agent.Spec.ServiceGroup != "" {
        serviceGroup = agent.Spec.ServiceGroup
    }
    if name := w.serviceGroupPolicyRef(namespace, serviceGroup); name != "" {
        if p := w.lookupPolicy(namespace, name); p != nil {
            return policyToEffective(p, name, w)
        }
    }

    if p := w.lookupPolicy(GlobalDefaultPolicyNamespace, GlobalDefaultPolicyName); p != nil {
        return policyToEffective(p, GlobalDefaultPolicyName, w)
    }
    // Live fallback: global default might not be in cache yet.
    p := &omniav1alpha1.SessionPrivacyPolicy{}
    err := w.client.Get(context.Background(), types.NamespacedName{
        Name: GlobalDefaultPolicyName, Namespace: GlobalDefaultPolicyNamespace,
    }, p)
    if err == nil {
        return policyToEffective(p, GlobalDefaultPolicyName, w)
    }
    if !apierrors.IsNotFound(err) {
        w.log.V(1).Info("global default lookup failed", "err", err)
    }
    return nil
}

const defaultServiceGroup = "default"

func (w *PolicyWatcher) lookupAgent(namespace, agentName string) *corev1alpha1.AgentRuntime {
    v, ok := w.agents.Load(namespace + "/" + agentName)
    if !ok {
        return nil
    }
    a, _ := v.(*corev1alpha1.AgentRuntime)
    return a
}

// serviceGroupPolicyRef returns the policy name referenced by the named service
// group of the workspace whose namespace matches the given namespace.
func (w *PolicyWatcher) serviceGroupPolicyRef(namespace, groupName string) string {
    v, ok := w.workspaces.Load(namespace)
    if !ok {
        return ""
    }
    ws, _ := v.(*corev1alpha1.Workspace)
    if ws == nil {
        return ""
    }
    for i := range ws.Spec.Services {
        sg := &ws.Spec.Services[i]
        if sg.Name == groupName && sg.PrivacyPolicyRef != nil {
            return sg.PrivacyPolicyRef.Name
        }
    }
    return ""
}

func (w *PolicyWatcher) lookupPolicy(namespace, name string) *omniav1alpha1.SessionPrivacyPolicy {
    v, ok := w.policies.Load(namespace + "/" + name)
    if !ok {
        return nil
    }
    p, _ := v.(*omniav1alpha1.SessionPrivacyPolicy)
    return p
}

func policyToEffective(p *omniav1alpha1.SessionPrivacyPolicy, resolvedName string, w *PolicyWatcher) *EffectivePolicy {
    w.lastResolvedName = resolvedName
    eff := &EffectivePolicy{
        Recording:  p.Spec.Recording,
        UserOptOut: p.Spec.UserOptOut,
    }
    if p.Spec.Encryption != nil {
        eff.Encryption = *p.Spec.Encryption
    }
    return eff
}
```

(`lastResolvedName` is a test seam; if you prefer no test-only state, drop it and assert via `EffectivePolicy` content instead.)

- [ ] **Step 4: Delete obsolete files**

If `ee/pkg/privacy/inheritance.go` exists with `ComputeEffectivePolicy` and is no longer used, delete it after confirming no other callers:

```
Grep "ComputeEffectivePolicy" path=ee/
```
If only resolver/watcher referenced it (and they've been rewritten), delete the file.

- [ ] **Step 5: Run all privacy tests**

Run: `env GOWORK=off go test ./ee/pkg/privacy/ -count=1 -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```
git add ee/pkg/privacy/
cat <<'EOF' | git commit -F -
refactor(privacy): deterministic policy lookup by Workspace/AgentRuntime ref

GetEffectivePolicy now resolves agent → workspace → global default with
no merge semantics. Watcher tracks Workspaces and AgentRuntimes alongside
SessionPrivacyPolicy. Drops ComputeEffectivePolicy and the inheritance
chain machinery.

Ref #801
EOF
```

---

## Task 7: Update Workspace + AgentRuntime controllers to surface ref-resolution conditions

**Files:**
- Modify: `internal/controller/workspace_controller.go`
- Modify: `internal/controller/workspace_controller_test.go`
- Modify: `internal/controller/agentruntime_controller.go`
- Modify: `internal/controller/agentruntime_controller_test.go`

- [ ] **Step 1: Identify reconcile entry points**

Run:
```
Grep "func .* Reconcile.*Workspace" path=internal/controller -n
Grep "func .* Reconcile.*AgentRuntime" path=internal/controller -n
```
Note the line numbers — you'll insert validation calls near the end of each `Reconcile` (just before status update).

- [ ] **Step 2: Add validation helper to Workspace controller**

In `internal/controller/workspace_controller.go`, add:

```go
// validatePrivacyPolicyRefs returns one Condition summarising privacyPolicyRef
// resolution across all service groups. Type is "PrivacyPolicyResolved";
// Status is False if ANY referenced policy is missing, with a Message listing
// every unresolved (groupName, policyName) pair.
func (r *WorkspaceReconciler) validatePrivacyPolicyRefs(ctx context.Context, ws *corev1alpha1.Workspace) metav1.Condition {
    var missing []string
    var resolved []string
    for i := range ws.Spec.Services {
        sg := &ws.Spec.Services[i]
        if sg.PrivacyPolicyRef == nil {
            continue
        }
        p := &omniav1alpha1.SessionPrivacyPolicy{}
        err := r.Get(ctx, types.NamespacedName{
            Name: sg.PrivacyPolicyRef.Name, Namespace: ws.Spec.Namespace.Name,
        }, p)
        if err != nil {
            missing = append(missing, fmt.Sprintf("services[%s] -> %s (%v)", sg.Name, sg.PrivacyPolicyRef.Name, err))
            continue
        }
        resolved = append(resolved, fmt.Sprintf("services[%s] -> %s", sg.Name, sg.PrivacyPolicyRef.Name))
    }
    if len(missing) > 0 {
        return metav1.Condition{
            Type: "PrivacyPolicyResolved", Status: metav1.ConditionFalse,
            Reason:  "PolicyNotFound",
            Message: "unresolved privacyPolicyRef(s): " + strings.Join(missing, "; "),
        }
    }
    if len(resolved) == 0 {
        return metav1.Condition{
            Type: "PrivacyPolicyResolved", Status: metav1.ConditionTrue,
            Reason:  "DefaultPolicy",
            Message: "no service group sets privacyPolicyRef; sessions use global default",
        }
    }
    return metav1.Condition{
        Type: "PrivacyPolicyResolved", Status: metav1.ConditionTrue,
        Reason:  "PolicyResolved",
        Message: strings.Join(resolved, "; "),
    }
}
```

Call it from `Reconcile` before `Status().Update`:

```go
cond := r.validatePrivacyPolicyRefs(ctx, ws)
meta.SetStatusCondition(&ws.Status.Conditions, cond)
```

- [ ] **Step 3: Same for AgentRuntime controller**

In `internal/controller/agentruntime_controller.go`:

```go
func (r *AgentRuntimeReconciler) validatePrivacyPolicyRef(ctx context.Context, ar *corev1alpha1.AgentRuntime) metav1.Condition {
    if ar.Spec.PrivacyPolicyRef == nil {
        return metav1.Condition{
            Type: "PrivacyPolicyResolved", Status: metav1.ConditionTrue,
            Reason: "WorkspaceDefault",
            Message: "no privacyPolicyRef set; using workspace or global default",
        }
    }
    p := &omniav1alpha1.SessionPrivacyPolicy{}
    err := r.Get(ctx, types.NamespacedName{
        Name: ar.Spec.PrivacyPolicyRef.Name, Namespace: ar.Namespace,
    }, p)
    if err != nil {
        return metav1.Condition{
            Type: "PrivacyPolicyResolved", Status: metav1.ConditionFalse,
            Reason: "PolicyNotFound",
            Message: fmt.Sprintf("privacyPolicyRef %q not found: %v",
                ar.Spec.PrivacyPolicyRef.Name, err),
        }
    }
    return metav1.Condition{
        Type: "PrivacyPolicyResolved", Status: metav1.ConditionTrue,
        Reason: "PolicyResolved",
        Message: fmt.Sprintf("using SessionPrivacyPolicy %q", ar.Spec.PrivacyPolicyRef.Name),
    }
}
```

Wire it into `Reconcile` before status update.

- [ ] **Step 4: Tests for both controllers**

In `workspace_controller_test.go`, add a Context covering: (a) no service group sets a ref → `PrivacyPolicyResolved=True/DefaultPolicy`, (b) one group references a non-existent policy → `PrivacyPolicyResolved=False/PolicyNotFound` with the group name in the message, (c) one group references an existing policy → `PrivacyPolicyResolved=True/PolicyResolved`, (d) two groups, one missing one resolved → `PrivacyPolicyResolved=False` listing the missing one. For `agentruntime_controller_test.go`, three cases: (a) no ref → `PrivacyPolicyResolved=True/WorkspaceDefault`, (b) ref to non-existent policy → `PrivacyPolicyResolved=False/PolicyNotFound`, (c) ref to existing policy → `PrivacyPolicyResolved=True/PolicyResolved`.

- [ ] **Step 5: Run controller tests**

Run: `env GOWORK=off go test ./internal/controller/ -run "Workspace|AgentRuntime" -count=1 -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```
git add internal/controller/{workspace,agentruntime}_controller.go internal/controller/{workspace,agentruntime}_controller_test.go
cat <<'EOF' | git commit -F -
feat(controller): surface privacyPolicyRef resolution status

Workspace and AgentRuntime each get a PrivacyPolicyResolved condition.
Missing refs do not block reconciliation but are visible in kubectl
describe output.

Ref #801
EOF
```

---

## Task 8: Define EncryptorResolver interface in handler

**Files:**
- Create: `internal/session/api/encryptor.go`
- Create: `internal/session/api/encryptor_test.go`

- [ ] **Step 1: Write the interface and tests**

Create `internal/session/api/encryptor.go`:

```go
package api

// Encryptor encrypts/decrypts opaque byte slices.
// Implementations live outside this package (ee/pkg/encryption).
type Encryptor interface {
    Encrypt(plaintext []byte) ([]byte, error)
    Decrypt(ciphertext []byte) ([]byte, error)
}

// EncryptorResolver returns the Encryptor that should be used for the given session.
// Returns (nil, false) when no encryption applies (plaintext passthrough).
type EncryptorResolver interface {
    EncryptorForSession(sessionID string) (Encryptor, bool)
}

// EncryptorResolverFunc adapts a function to the EncryptorResolver interface.
type EncryptorResolverFunc func(sessionID string) (Encryptor, bool)

func (f EncryptorResolverFunc) EncryptorForSession(sessionID string) (Encryptor, bool) {
    return f(sessionID)
}
```

Create `internal/session/api/encryptor_test.go`:

```go
package api

import (
    "testing"

    "github.com/stretchr/testify/assert"
)

type stubEnc struct{}

func (stubEnc) Encrypt(p []byte) ([]byte, error) { return append([]byte("E:"), p...), nil }
func (stubEnc) Decrypt(c []byte) ([]byte, error) { return c[2:], nil }

func TestEncryptorResolverFunc(t *testing.T) {
    var resolver EncryptorResolver = EncryptorResolverFunc(func(id string) (Encryptor, bool) {
        if id == "encrypted-session" {
            return stubEnc{}, true
        }
        return nil, false
    })

    enc, ok := resolver.EncryptorForSession("encrypted-session")
    assert.True(t, ok)
    out, err := enc.Encrypt([]byte("hi"))
    assert.NoError(t, err)
    assert.Equal(t, []byte("E:hi"), out)

    _, ok = resolver.EncryptorForSession("plaintext-session")
    assert.False(t, ok)
}
```

- [ ] **Step 2: Run tests**

Run: `env GOWORK=off go test ./internal/session/api/ -run TestEncryptorResolver -count=1 -v`
Expected: PASS.

- [ ] **Step 3: Commit**

```
git add internal/session/api/encryptor.go internal/session/api/encryptor_test.go
cat <<'EOF' | git commit -F -
feat(api): add EncryptorResolver interface for per-session encryption

Replaces the single-Encryptor model that was reverted in 4038339e.
Concrete implementation lives in cmd/session-api.

Ref #801
EOF
```

---

## Task 9: Wire EncryptorResolver into Handler write/read paths

**Files:**
- Modify: `internal/session/api/handler.go`
- Modify: `internal/session/api/handler_test.go` (or add `encryption_test.go`)
- Modify: any encryption helpers in `internal/session/api/` that previously took a single `Encryptor`

- [ ] **Step 1: Add resolver field + setter**

In `internal/session/api/handler.go`:

```go
type Handler struct {
    service           *SessionService
    evalService       *EvalService
    policyResolver    PolicyResolver
    encryptorResolver EncryptorResolver
    log               logr.Logger
    maxBodySize       int64
}

// SetEncryptorResolver configures per-session encryption. When nil (default),
// all sessions are stored in plaintext.
func (h *Handler) SetEncryptorResolver(r EncryptorResolver) {
    h.encryptorResolver = r
}
```

Add a helper:

```go
// encryptorFor returns the encryptor for the given session, or nil if none applies.
func (h *Handler) encryptorFor(sessionID string) Encryptor {
    if h.encryptorResolver == nil {
        return nil
    }
    enc, ok := h.encryptorResolver.EncryptorForSession(sessionID)
    if !ok {
        return nil
    }
    return enc
}
```

- [ ] **Step 2: Update write/read handlers**

For each of `handleAppendMessage`, `handleRecordToolCall`, `handleRecordRuntimeEvent`, `handleGetMessages`, `handleGetToolCalls`, `handleGetRuntimeEvents`:

1. Extract `sessionID` (already done early in each handler).
2. Call `enc := h.encryptorFor(sessionID)`.
3. If `enc != nil`, encrypt before persisting (write paths) or decrypt after fetching (read paths).

Reuse the encrypt/decrypt helpers already added by #780 (look in `internal/session/api/encryption_*.go` from the worktree; they took a single `Encryptor` parameter — they still work, just resolved per-session now).

The only new logic is the per-session resolution. Refer to the wiring that was reverted in commit `4038339e` (file `internal/session/api/handler.go` at the prior commit `ddad4574`) for the exact encrypt/decrypt call sites — copy those, but pass the per-session encryptor.

- [ ] **Step 3: Write tests**

In `internal/session/api/encryption_test.go` (or recreate it from the reverted version):

```go
func TestHandler_PerSessionEncryption(t *testing.T) {
    // session-A → encryptor A; session-B → encryptor B; verify each
    // session round-trips with its own encryptor and not the other's.
    encA := stubEnc{tag: "A"}
    encB := stubEnc{tag: "B"}
    resolver := EncryptorResolverFunc(func(id string) (Encryptor, bool) {
        switch id {
        case "session-A":
            return encA, true
        case "session-B":
            return encB, true
        }
        return nil, false
    })

    h := newTestHandler(t)
    h.SetEncryptorResolver(resolver)

    // POST a message under session-A; assert raw stored bytes start with "EA:"
    // POST a message under session-B; assert raw stored bytes start with "EB:"
    // GET both; assert plaintext returned matches what was POSTed
    // POST under session-C (no encryptor); assert raw bytes are plaintext
}
```

- [ ] **Step 4: Run**

Run: `env GOWORK=off go test ./internal/session/api/ -count=1 -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```
git add internal/session/api/handler.go internal/session/api/encryption_test.go
cat <<'EOF' | git commit -F -
feat(api): per-session encryption via EncryptorResolver

Each write/read in the handler resolves the encryptor for the specific
session. Sessions in workspaces with different KMS providers can now
coexist in one session-api instance.

Ref #801
EOF
```

---

## Task 10: Implement PerPolicyEncryptorResolver in cmd/session-api

**Files:**
- Create: `cmd/session-api/encryption_resolver.go`
- Create: `cmd/session-api/encryption_resolver_test.go`

- [ ] **Step 1: Write tests first**

Create `cmd/session-api/encryption_resolver_test.go`:

```go
package main

import (
    "testing"

    "github.com/go-logr/logr"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"

    omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

type fakePolicySource struct {
    bySession map[string]*privacyEffective // helper struct local to test
}

// privacyEffective is a minimal mock of privacy.EffectivePolicy
type privacyEffective struct {
    encryption omniav1alpha1.EncryptionConfig
}

func TestPerPolicyEncryptorResolver_NoEncryption(t *testing.T) {
    src := func(sessionID string) (*omniav1alpha1.EncryptionConfig, bool) {
        return &omniav1alpha1.EncryptionConfig{Enabled: false}, true
    }
    resolver := newPerPolicyEncryptorResolver(src, stubFactory{}, logr.Discard())
    enc, ok := resolver.EncryptorForSession("any-session")
    assert.Nil(t, enc)
    assert.False(t, ok)
}

func TestPerPolicyEncryptorResolver_CachesByKMSAndKey(t *testing.T) {
    factory := &countingFactory{}
    src := func(sessionID string) (*omniav1alpha1.EncryptionConfig, bool) {
        return &omniav1alpha1.EncryptionConfig{
            Enabled: true, KMSProvider: "aws-kms", KeyID: "key1",
        }, true
    }
    resolver := newPerPolicyEncryptorResolver(src, factory, logr.Discard())

    _, ok := resolver.EncryptorForSession("s1")
    require.True(t, ok)
    _, ok = resolver.EncryptorForSession("s2") // same KMS+key → cache hit
    require.True(t, ok)

    assert.Equal(t, 1, factory.builds, "expected one build for one (kmsProvider,keyID) pair")
}

func TestPerPolicyEncryptorResolver_DifferentKeysAreSeparate(t *testing.T) {
    factory := &countingFactory{}
    cfgs := map[string]*omniav1alpha1.EncryptionConfig{
        "s-aws":   {Enabled: true, KMSProvider: "aws-kms", KeyID: "k1"},
        "s-azure": {Enabled: true, KMSProvider: "azure-keyvault", KeyID: "k1"},
    }
    src := func(sid string) (*omniav1alpha1.EncryptionConfig, bool) { return cfgs[sid], true }
    resolver := newPerPolicyEncryptorResolver(src, factory, logr.Discard())

    _, _ = resolver.EncryptorForSession("s-aws")
    _, _ = resolver.EncryptorForSession("s-azure")
    assert.Equal(t, 2, factory.builds)
}
```

(Define `stubFactory`/`countingFactory` returning trivial `Encryptor` impls.)

- [ ] **Step 2: Implement the resolver**

Create `cmd/session-api/encryption_resolver.go`:

```go
package main

import (
    "fmt"
    "sync"

    "github.com/go-logr/logr"

    omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
    "github.com/altairalabs/omnia/ee/pkg/encryption"
    "github.com/altairalabs/omnia/internal/session/api"
)

// EncryptionConfigForSession returns the EncryptionConfig that applies to the
// session, or (nil, false) when no policy applies.
type EncryptionConfigForSession func(sessionID string) (*omniav1alpha1.EncryptionConfig, bool)

// EncryptorFactory builds an api.Encryptor from a fully specified EncryptionConfig.
// Implemented by the KMS factory in cmd/session-api/main.go.
type EncryptorFactory interface {
    Build(cfg omniav1alpha1.EncryptionConfig) (api.Encryptor, error)
}

type cacheKey struct{ provider, keyID string }

type perPolicyEncryptorResolver struct {
    source  EncryptionConfigForSession
    factory EncryptorFactory
    cache   sync.Map // cacheKey -> api.Encryptor
    log     logr.Logger
}

func newPerPolicyEncryptorResolver(src EncryptionConfigForSession, f EncryptorFactory, log logr.Logger) *perPolicyEncryptorResolver {
    return &perPolicyEncryptorResolver{source: src, factory: f, log: log.WithName("encryption-resolver")}
}

func (r *perPolicyEncryptorResolver) EncryptorForSession(sessionID string) (api.Encryptor, bool) {
    cfg, ok := r.source(sessionID)
    if !ok || cfg == nil || !cfg.Enabled {
        return nil, false
    }
    key := cacheKey{provider: string(cfg.KMSProvider), keyID: cfg.KeyID}
    if v, hit := r.cache.Load(key); hit {
        return v.(api.Encryptor), true
    }
    enc, err := r.factory.Build(*cfg)
    if err != nil {
        r.log.Error(err, "encryptor build failed", "kmsProvider", cfg.KMSProvider, "keyID", cfg.KeyID)
        return nil, false
    }
    actual, _ := r.cache.LoadOrStore(key, enc)
    return actual.(api.Encryptor), true
}

// Invalidate drops a cached entry. Called when policy changes are detected.
func (r *perPolicyEncryptorResolver) Invalidate(provider, keyID string) {
    r.cache.Delete(cacheKey{provider: provider, keyID: keyID})
}

// helper: cache key for logging
func (k cacheKey) String() string { return fmt.Sprintf("%s/%s", k.provider, k.keyID) }
```

- [ ] **Step 3: Run tests**

Run: `env GOWORK=off go test ./cmd/session-api/ -run TestPerPolicy -count=1 -v`
Expected: PASS.

- [ ] **Step 4: Commit**

```
git add cmd/session-api/encryption_resolver.go cmd/session-api/encryption_resolver_test.go
cat <<'EOF' | git commit -F -
feat(session-api): per-policy encryptor resolver with (kmsProvider,keyID) cache

Concrete EncryptorResolver for the API handler. Caches built encryptors
to amortize KMS init cost. Invalidate() will be called from the policy
watcher loop in a follow-up commit.

Ref #801
EOF
```

---

## Task 11: Wire PerPolicyEncryptorResolver into cmd/session-api/main.go

**Files:**
- Modify: `cmd/session-api/main.go`
- Modify: `cmd/session-api/wiring_test.go`

- [ ] **Step 1: Write the wiring test**

Add to `cmd/session-api/wiring_test.go`:

```go
func TestEncryptorResolverWired_Enterprise(t *testing.T) {
    f := &flags{enterprise: true /* + minimal valid fields */}
    h, err := buildHandler(f, testDeps()) // refactor main.go to expose buildHandler if needed
    require.NoError(t, err)
    // Resolver must be installed in enterprise mode
    require.NotNil(t, getEncryptorResolverFromHandler(h))
}

func TestEncryptorResolverNotWired_OSS(t *testing.T) {
    f := &flags{enterprise: false}
    h, err := buildHandler(f, testDeps())
    require.NoError(t, err)
    require.Nil(t, getEncryptorResolverFromHandler(h))
}
```

(You may need to expose a small accessor on `*api.Handler` for testing — `EncryptorResolver()` returning the field.)

- [ ] **Step 2: Wire it in main.go**

In `cmd/session-api/main.go` enterprise setup block (where `PolicyWatcher` is constructed), after the watcher is created:

```go
// Per-session encryption resolver.
// Note: this session-api instance serves one service group (f.workspace + f.serviceGroup).
// All sessions belong to the same workspace+group, so most of them resolve to the
// same EncryptionConfig — but AgentRuntime overrides can introduce additional
// (kmsProvider, keyID) pairs, hence the cache.
configForSession := func(sessionID string) (*omniav1alpha1.EncryptionConfig, bool) {
    sess, err := store.GetSessionMetadata(ctx, sessionID)
    if err != nil || sess == nil {
        return nil, false
    }
    eff := policyWatcher.GetEffectivePolicy(sess.Namespace, sess.AgentName)
    if eff == nil {
        return nil, false
    }
    return &eff.Encryption, true
}

factory := &kmsEncryptorFactory{
    log:     log,
    secrets: kubeClient,                    // for SecretRef resolution via ProviderConfigFromEncryptionSpec
    builder: encryption.NewProviderFromConfig, // existing helper
}

resolver := newPerPolicyEncryptorResolver(configForSession, factory, log)
handler.SetEncryptorResolver(resolver)
```

You'll need an `EncryptorFactory` impl (`kmsEncryptorFactory`) that:
1. Calls `encryption.ProviderConfigFromEncryptionSpec(cfg, secretGetter)` (already merged from #780).
2. Calls the KMS provider factory to get an `encryption.Provider`.
3. Wraps in `encryption.NewEncryptor(provider)`.
4. Adapts to `api.Encryptor` (drop the `[]EncryptionEvent` return — same adapter that lived in `internal/session/api/encryption_adapter.go` in the reverted commit).

Refer to `cmd/session-api/main.go` at commit `ddad4574` (`buildEncryptorFromPolicy`) for the factory shape. Adapt it: take an `EncryptionConfig` arg instead of reading the global policy.

- [ ] **Step 3: Add policy-change cache invalidation**

In the `PolicyWatcher.loadPolicies` reload tick, after an updated policy is stored, if its `Encryption.KMSProvider`/`KeyID` changed, call `resolver.Invalidate(...)`. Wire this via a callback on `PolicyWatcher`:

```go
// In ee/pkg/privacy/watcher.go
type PolicyChangeCallback func(old, new *omniav1alpha1.SessionPrivacyPolicy)
func (w *PolicyWatcher) OnPolicyChange(cb PolicyChangeCallback) { w.onChange = cb }
```

Then in main.go:
```go
policyWatcher.OnPolicyChange(func(old, new *omniav1alpha1.SessionPrivacyPolicy) {
    if old != nil && old.Spec.Encryption != nil {
        resolver.Invalidate(string(old.Spec.Encryption.KMSProvider), old.Spec.Encryption.KeyID)
    }
})
```

- [ ] **Step 4: Run wiring tests + build**

```
env GOWORK=off go build ./cmd/session-api/...
env GOWORK=off go test ./cmd/session-api/ -count=1 -v
```
Expected: PASS.

- [ ] **Step 5: Commit**

```
git add cmd/session-api/main.go cmd/session-api/wiring_test.go ee/pkg/privacy/watcher.go
cat <<'EOF' | git commit -F -
feat(session-api): wire per-policy encryptor resolver

In enterprise mode, session-api resolves an encryptor per-session via
PolicyWatcher. Cache invalidation triggers on policy change. Sessions
without an enabled encryption policy stay plaintext.

Closes the gap left by the 4038339e revert.

Ref #801
EOF
```

---

## Task 12: Re-add encryption health check in doctor

**Files:**
- Modify: `internal/doctor/checks/privacy.go`
- Modify: `internal/doctor/checks/privacy_test.go`

- [ ] **Step 1: Look at the reverted check**

Run:
```
git show 4038339e:internal/doctor/checks/privacy.go > /tmp/old-privacy.go
```
Read `/tmp/old-privacy.go` for the original `SessionEncryptionAtRest` check shape.

- [ ] **Step 2: Adapt the check**

The new model: we can't tell from a global config whether encryption is on. Instead, the check:
1. Lists all `Workspace`s and their `PrivacyPolicyRef`.
2. For each workspace whose policy has `Encryption.Enabled`, write a probe message to a session in that workspace.
3. Read the raw DB row; assert it carries the `enc:v1:` prefix.

Rewrite `internal/doctor/checks/privacy.go`'s `SessionEncryptionAtRest` along these lines.

- [ ] **Step 3: Tests**

Add tests covering: (a) no encryption-enabled policy → check skipped with "no workspaces with encryption" status; (b) workspace with encryption enabled → write probe → assert ciphertext.

- [ ] **Step 4: Run**

```
env GOWORK=off go test ./internal/doctor/checks/ -count=1 -v
```
Expected: PASS.

- [ ] **Step 5: Commit**

```
git add internal/doctor/checks/privacy.go internal/doctor/checks/privacy_test.go
cat <<'EOF' | git commit -F -
feat(doctor): re-add SessionEncryptionAtRest check

Walks workspaces with encryption-enabled policies and verifies the
session-api stores ciphertext (enc:v1: prefix) in Postgres.

Ref #801
EOF
```

---

## Task 13: Sync chart, RBAC, dashboard types

**Files:**
- Modify: `charts/omnia/crds/*.yaml` (auto)
- Modify: `config/rbac/role.yaml` (auto)
- Modify: `dashboard/src/types/generated/*.ts` (auto)

- [ ] **Step 1: Run the regen toolchain**

```
make manifests
make sync-chart-crds
make generate-dashboard-types
```

- [ ] **Step 2: Inspect for surprises**

```
git status
git diff config/rbac/role.yaml | head -50
```
Expected diff: RBAC adds verbs on `workspaces` and `agentruntimes` for whichever ServiceAccount runs `cmd/session-api`.

- [ ] **Step 3: Hand-update Helm values examples (if any reference `.spec.level`)**

```
Grep -r "spec.*level" path=charts/omnia
```
Update any sample CRs.

- [ ] **Step 4: Commit**

```
git add charts/ config/ dashboard/src/types/generated/
cat <<'EOF' | git commit -F -
chore: regenerate CRDs, RBAC, and dashboard types after privacy redesign

PolicyWatcher now needs read access to Workspaces and AgentRuntimes.
Helm chart CRDs synced from config/crd/bases.

Ref #801
EOF
```

---

## Task 14: End-to-end integration tests

**Files:**
- Create: `ee/pkg/privacy/integration_redesign_test.go`
- Modify: `test/e2e/e2e_test.go` (only if existing tests depend on `level` field)

- [ ] **Step 1: Write end-to-end test**

Cover: one workspace with two service groups (`dev` + `prod`) using different policies, plus an AgentRuntime that overrides → three distinct `(kmsProvider, keyID)` pairs in one session-api instance, all round-tripping correctly.

```go
func TestServiceGroupEncryption_EndToEnd(t *testing.T) {
    // 1. envtest setup with API + Workspace + AgentRuntime + SessionPrivacyPolicy CRDs
    // 2. Create three policies in ws-ns:
    //    - dev-policy   (KMS=local, key=key-dev)
    //    - prod-policy  (KMS=local, key=key-prod)
    //    - audit-policy (KMS=local, key=key-audit)  -- per-agent override
    // 3. Create one Workspace ws-1 with two service groups:
    //    services[default]: privacyPolicyRef=dev-policy
    //    services[prod]:    privacyPolicyRef=prod-policy
    // 4. Create AgentRuntimes in ws-ns:
    //    agent-dev    (serviceGroup=default, no override)         -> resolves dev-policy
    //    agent-prod   (serviceGroup=prod,    no override)         -> resolves prod-policy
    //    agent-audit  (serviceGroup=prod,    PrivacyPolicyRef=audit-policy) -> resolves audit-policy
    // 5. Spin up handler with PerPolicyEncryptorResolver
    // 6. POST messages to a session in each agent's namespace
    // 7. Read raw Postgres rows; assert three distinct ciphertexts (different keys)
    // 8. GET via handler; assert plaintext returned matches what was POSTed
}
```

- [ ] **Step 2: Update any e2e tests referencing removed `level` field**

```
Grep -r "Spec.Level|spec.level|PolicyLevelGlobal|PolicyLevelWorkspace|PolicyLevelAgent" path=test
```
For each match: replace with the new `privacyPolicyRef` model or delete if obsolete.

- [ ] **Step 3: Run**

```
env GOWORK=off go test ./ee/pkg/privacy/ -run TestMultiWorkspace -count=1 -v
```

Local e2e (per CLAUDE.md):
```
kind create cluster --name omnia-test-e2e --wait 60s
env GOWORK=off KIND_CLUSTER=omnia-test-e2e E2E_SKIP_CLEANUP=true \
  go test -tags=e2e ./test/e2e/ -v -ginkgo.v -ginkgo.label-filter='!arena' -timeout 20m
```

- [ ] **Step 4: Commit**

```
git add ee/pkg/privacy/integration_redesign_test.go test/e2e/
cat <<'EOF' | git commit -F -
test: end-to-end per-service-group encryption with redesigned policy binding

Two service groups in one workspace plus an AgentRuntime override exercise
three distinct (kmsProvider, keyID) pairs in one session-api instance.

Ref #801
EOF
```

---

## Task 15: Public docs

**Files:**
- Create: `docs/src/content/docs/reference/sessionprivacypolicy.md`
- Create: `docs/src/content/docs/how-to/configure-privacy-policies.md`
- Modify: `cmd/session-api/SERVICE.md`
- Modify: `api/CHANGELOG.md`

- [ ] **Step 1: Reference doc**

Look at the reverted versions for inspiration:
```
git show 4038339e:docs/src/content/docs/reference/sessionprivacypolicy.md > /tmp/old-ref.md
git show 4038339e:docs/src/content/docs/how-to/configure-privacy-policies.md > /tmp/old-howto.md
```

Rewrite both for the new model:
- Reference: schema (no `level`), namespacing rules, lifecycle, status conditions, resolution order (AgentRuntime override → service group → global default).
- How-to: step-by-step `kubectl apply` of a policy in the workspace namespace + `kubectl edit workspace ws-1` to add `privacyPolicyRef` to one of `spec.services[]` + (optional) per-agent override on `AgentRuntime.spec.privacyPolicyRef` + verify with `kubectl get workspace ws-1 -o jsonpath='{.status.conditions[?(@.type=="PrivacyPolicyResolved")]}'`.

- [ ] **Step 2: SERVICE.md**

Add to `cmd/session-api/SERVICE.md` under inputs/dependencies:
- Watches: `SessionPrivacyPolicy`, `Workspace`, `AgentRuntime` (read).
- Per-request encryption resolver with `(kmsProvider, keyID)` cache.

- [ ] **Step 3: CHANGELOG**

Append to `api/CHANGELOG.md`:

```
## Unreleased

### Breaking
- `SessionPrivacyPolicy.spec.level`, `spec.workspaceRef`, and `spec.agentRef` removed. Policies are now reusable namespaced documents bound by consumers.
- `SessionPrivacyPolicy` is now namespace-scoped (was cluster-scoped).

### Added
- `Workspace.spec.services[].privacyPolicyRef` (LocalObjectReference) — selects the policy applied to all sessions managed by that service group's session-api.
- `AgentRuntime.spec.privacyPolicyRef` (LocalObjectReference) — per-agent override.
```

- [ ] **Step 4: Migration playbook**

Create `hack/migrations/2026-04-13-privacy-policy-redesign.md` with step-by-step kubectl commands for upgrading existing deployments.

- [ ] **Step 5: Commit**

```
git add docs/src/content/docs/ cmd/session-api/SERVICE.md api/CHANGELOG.md hack/migrations/2026-04-13-privacy-policy-redesign.md
cat <<'EOF' | git commit -F -
docs: SessionPrivacyPolicy redesign reference + how-to + migration

Reusable policy documents referenced by Workspace and AgentRuntime,
with deterministic agent → workspace → global default lookup.

Ref #801
EOF
```

---

## Task 16: Pre-PR verification

- [ ] **Step 1: Lint**

```
env GOWORK=off golangci-lint run ./...
```
Fix any new findings.

- [ ] **Step 2: Full test pass**

```
env GOWORK=off go test ./... -count=1
```

- [ ] **Step 3: Dashboard**

```
cd dashboard && npm run lint && npm run typecheck && npx vitest run --coverage
```

- [ ] **Step 4: Local e2e (non-arena)**

```
env GOWORK=off KIND_CLUSTER=omnia-test-e2e E2E_SKIP_CLEANUP=true \
  go test -tags=e2e ./test/e2e/ -v -ginkgo.v -ginkgo.label-filter='!arena' -timeout 20m
```

- [ ] **Step 5: Open PR**

```
git push -u origin feat/801-privacy-policy-redesign
gh pr create --title "feat(privacy): reusable SessionPrivacyPolicy + per-request encryption (#801)" --body "$(cat <<'EOF'
## Summary
- Removes `level`/`workspaceRef`/`agentRef` from `SessionPrivacyPolicy`; policies become reusable namespaced documents.
- Adds `Workspace.spec.services[].privacyPolicyRef` (each service group has its own session-api, so each can carry its own policy) and `AgentRuntime.spec.privacyPolicyRef` (per-agent override).
- `PolicyWatcher.GetEffectivePolicy` becomes a deterministic agent → service group → global-default lookup.
- Wires session-api per-request encryption with a `(kmsProvider, keyID)` cache. AgentRuntime overrides within a service group now work.
- Re-adds the doctor encryption check; restores public docs in revised form.

Closes #801. Builds on #780 (which shipped recording control + the encryption primitives but reverted the broken global-policy wiring).

## Test plan
- [ ] `go test ./...` green
- [ ] `golangci-lint run ./...` clean
- [ ] e2e: `go test -tags=e2e ./test/e2e/ -v -ginkgo.label-filter='!arena'` green
- [ ] Manual: two workspaces with different KMS keys; verify per-session ciphertext differs in raw DB and round-trips via API.
EOF
)"
```

---

## Self-Review

**Spec coverage:**
- "Remove `spec.level`" → Task 1 ✓
- "WorkspaceServiceGroup gains `privacyPolicyRef`" (per-service-group, not per-workspace, per follow-up clarification) → Task 2 ✓
- "AgentRuntime gains `spec.privacyPolicyRef`" → Task 3 ✓
- "Resolution: agent → service group → global default" → Task 6 ✓
- "Per-request encryptor resolution with `(kmsProvider, keyID)` cache" → Tasks 8–11 ✓
- "PolicyWatcher invalidates cache on policy change" → Task 11 ✓
- "Drop stricter-than-parent webhook validation; add ref existence" → Task 5 ✓ (note: ref-existence is enforced on delete in webhook; on create/update it surfaces as a status condition rather than blocking — matches "validate refs resolve" in spec by giving fast feedback without breaking the bootstrapping order between policies and consumers)
- "Session-api handler: per-request encryption" → Task 9 ✓
- "Migration guide" → Task 15 ✓
- "Public docs (reference + how-to)" → Task 15 ✓
- "Tests: controller, watcher, middleware, wiring, end-to-end integration" → Tasks 4, 6, 9, 11, 14 ✓
- "`SessionPrivacyPolicyReconciler` simplified — drop inheritance ConfigMap distribution" → Task 4 ✓

**Type consistency:**
- `EncryptorResolver.EncryptorForSession(sessionID string) (Encryptor, bool)` — used identically in Tasks 8, 9, 10.
- `EncryptionConfigForSession func(sessionID string) (*omniav1alpha1.EncryptionConfig, bool)` — Task 10 only.
- `cacheKey{provider, keyID string}` — Task 10 only.
- `policyWatcher.GetEffectivePolicy(namespace, agentName) *EffectivePolicy` — same signature in Tasks 6 and 11.
- `WorkspaceServiceGroup.PrivacyPolicyRef *corev1.LocalObjectReference` — same in Tasks 2, 5, 6, 7.
- `AgentRuntime.Spec.PrivacyPolicyRef *corev1.LocalObjectReference` — same in Tasks 3, 5, 6, 7.

**Placeholder scan:** No "TBD"/"add validation"/"similar to". Test code blocks present where steps are TDD steps. Where exact reverted-commit code is reused (Tasks 11, 12), the commit hash + path are provided so the engineer can read the canonical source.

**One known soft spot:** Task 11's `getEncryptorResolverFromHandler(h)` test seam needs an accessor on `*api.Handler` — flagged inline in step 1.
