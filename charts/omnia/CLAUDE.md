# charts/omnia — Claude Code Instructions

Rules specific to the Omnia Helm chart. Root `CLAUDE.md` applies too.

History: the conventions below came out of the chart overhaul shipped 2026-04-18 (tracked in issue #895). Post-mortem at `docs/local-backlog/implemented/2026-04-18-helm-chart-overhaul.md`.

## Golden rules

### 1. No hardcoded tunables in templates

Every probe timing, resource request/limit, port, image tag, and grace period MUST be values-driven. When editing or adding a template:

```bash
# Self-check: run inside charts/omnia/
grep -rn "initialDelaySeconds:\|periodSeconds:\|timeoutSeconds:\|cpu:\|memory:\|containerPort:\|image: [^{]" templates/
```

Every match should be either a `{{ .Values.* }}` reference or have an inline `# keep-hardcoded: <reason>` comment. Literal numbers/strings without that justification are a regression.

Exception: structural ports that the binary itself hardcodes (e.g. the Postgres StatefulSet's container port 5432) can stay literal — they're wire-protocol values, not tunables.

### 2. `podOverrides` is the canonical shape for pod customization

All six chart-owned Deployments (operator, dashboard, arena-controller, eval-worker, promptkit-lsp, doctor) expose a `<component>.podOverrides` block. Shape matches the CRD `PodOverrides` struct in `api/v1alpha1/shared_types.go` — 13 fields:

```
serviceAccountName  labels  annotations
nodeSelector  tolerations  affinity  priorityClassName  topologySpreadConstraints
imagePullSecrets
extraEnv  extraEnvFrom  extraVolumes  extraVolumeMounts
```

When adding a new chart-owned Deployment:
- Add `<component>.podOverrides: {}` to `values.yaml`
- Wire it in the template using the pattern in `templates/deployment.yaml` (the operator Deployment is the reference):
  - `{{- $po := .Values.<component>.podOverrides | default dict }}` at the top
  - Use the same merge semantics everywhere else uses them (see below).

**Merge semantics** (documented in commits for PR #906 / #908 and in `_helpers.tpl`):

| Field | Semantics |
|---|---|
| `serviceAccountName`, `affinity`, `priorityClassName` | User REPLACES chart default when set |
| `labels` | Chart operator-set labels WIN on collision (Service selectors depend on them) |
| `annotations` | User WINS on collision |
| `nodeSelector` | Merged per-key via `mergeOverwrite` (user wins) |
| `tolerations`, `imagePullSecrets`, `topologySpreadConstraints` | APPEND to chart-wide |
| `extraEnv`, `extraEnvFrom`, `extraVolumes`, `extraVolumeMounts` | APPEND to legacy `<component>.extra*` values |

Legacy chart values (`<component>.extraEnv`, `extraEnvFrom`, `extraVolumes`, `extraVolumeMounts`) from PR #842 are preserved — `podOverrides` is strictly additive.

### 3. Schema moves with values.yaml

`values.schema.json` is a safety net, not decoration. When you add a top-level key to `values.yaml`:

- Add a matching schema entry.
- Use shared `$defs` — `port` (1..65535), `quantity` (pattern-matched k8s resource strings), `probe`, `image`, `resources`.
- Constrain with `enum` where there's a fixed set of legal values.
- `required:` at the schema level is for things that break installs WITHOUT `.enabled: false` gates. For conditional requirements, use render-time `_helpers.tpl` checks (see rule 4).

Avoid `additionalProperties: false` at the top level — too many legitimate extensions would be flagged.

### 4. `dashboard.auth.mode` is required at RENDER time, not schema time

The chart intentionally has no default for `dashboard.auth.mode` (prevents accidental unauthenticated deploys). But `dashboard.enabled=false` installs (Arena E2E, some test configs) never set it.

Resolution: schema allows `""` in the enum; `_helpers.tpl` `omnia.validateAuth` fails render-time only when `dashboard.enabled=true` AND mode is empty. Don't add `required: ["mode"]` to the schema — it breaks dashboard-disabled installs.

### 5. Visible default changes need a Chart.yaml version bump

If a values default changes in a way that affects `helm template` output for an existing installation, bump `Chart.yaml` `version:` (semver minor for additive/default-flip changes). Recent example: `enterprise.communityTemplates.enabled: true → false` shipped as 0.9.0-beta.6 → 0.9.0-beta.7.

Document the change in the commit + PR body so the CHANGELOG generator picks it up.

### 6. CRDs are too big for client-side `kubectl apply`

The CRD OpenAPI embeds `corev1.Volume`/`Affinity`/`Toleration` (via the `PodOverrides` struct). After PR #865, `agentruntimes` and `workspaces` CRDs exceed the 262144-byte `kubectl.kubernetes.io/last-applied-configuration` annotation limit.

`make install` / `make deploy` / `make deploy-ee` all use `kubectl apply --server-side --force-conflicts`. Don't switch back to client-side apply without planning for this.

### 7. Pre-commit + CI gates

Before committing chart changes:
1. `bash hack/validate-helm.sh` — lint + render (default + enterprise).
2. `helm unittest charts/omnia` — all 26+ assertions pass.
3. If you edited any `examples/*.yaml` or the template matrix that they exercise, confirm `helm template omnia charts/omnia -f charts/omnia/examples/<file>.yaml` still renders. Overlays (Azure KV, IRSA, GKE WLI, observability, Istio ambient) must combine with `values-prod-oauth.yaml` per their headers.

CI gates enforce these (`test-helm-e2e.yml`). E2E workflows **do NOT** fire on `charts/**/values*.yaml` or `charts/**/*.md` changes (dropped in #899). Trigger manually via `gh workflow run test-e2e.yml --ref <branch>` for values-only PRs.

### 8. Helm lint needs `dashboard.auth.mode` passed explicitly

`helm lint charts/omnia` fails on a fresh clone unless you pass `-f charts/omnia/values-chart-tests.yaml` (or `--set dashboard.auth.mode=oauth`). The test values file supplies a valid `anonymous + allowAnonymous: true` combo so lint doesn't trip on the render-time auth check.

## Where things live

| Path | Purpose |
|---|---|
| `README.md` | User-facing install guide: minimum install, three profiles (dev/prod/enterprise), gotchas. **First thing a new operator reads.** |
| `values.yaml` | Canonical value reference with helm-docs-style `# --` comments on every field. 1,900+ lines. |
| `values.schema.json` | JSON Schema validation. 45 top-level properties. |
| `values-*.yaml` | Profile overlays for local dev + demos. NOT cloud-deploy examples — those are in `examples/`. |
| `examples/` | 8 worked values files for cloud deployments + observability + Istio ambient. See `examples/README.md`. |
| `templates/_helpers.tpl` | Shared macros. `omnia.validateAuth` + name helpers live here. |
| `templates/<component>/deployment.yaml` | Per-component Deployments. All wire `podOverrides`. |
| `crds/` | Non-enterprise CRDs. Synced from `config/crd/bases/` via `make sync-chart-crds`. |
| `templates/enterprise/*.yaml` | Enterprise CRDs gated by `enterprise.enabled`, synced by the same Makefile target. |
| `tests/*_test.yaml` | `helm unittest` suites. Suite-level `values: [../values-chart-tests.yaml]` satisfies auth guardrails. |
| `NOTES.txt` | Post-install guidance — includes a commented `podOverrides:` example on the sample AgentRuntime. |

## When to bump `Chart.yaml`

- Add a new `.Values.*` key → no bump needed (additive).
- Change a default in a way that alters rendered output → minor bump.
- Remove or rename a `.Values.*` key → major bump + migration note in `NOTES.txt`.
- Chart appVersion tracks app releases; bump both together when cutting a release.

## See also

- **Post-mortem**: `docs/local-backlog/implemented/2026-04-18-helm-chart-overhaul.md` — what shipped, the full PR list, why decisions were made.
- **PodOverrides user docs**: `https://omnia.altairalabs.ai/docs/how-to/configure-pod-overrides` (source at `docs/src/content/docs/how-to/configure-pod-overrides.md`).
- **Umbrella issue**: [#895](https://github.com/AltairaLabs/Omnia/issues/895).
