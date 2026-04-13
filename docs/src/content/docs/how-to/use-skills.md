---
title: "Use Skills"
description: "Sync AgentSkills.io content and attach it to a PromptPack"
sidebar:
  order: 25
---

Skills are demand-loaded knowledge bundles that PromptKit activates per turn based on the conversation. Omnia syncs skill content from Git, OCI, or ConfigMaps into the workspace PVC and exposes it to agents via the `PromptPack` CRD.

This guide walks through:

1. Create a `SkillSource` to sync content.
2. Attach the source to a `PromptPack` via `spec.skills`.
3. Verify the manifest, mount, and runtime wiring.

## Prerequisites

- A running Omnia operator with the `--workspace-content-path` flag set (defaults to `/workspace-content`).
- A workspace with shared content storage configured (the workspace's content PVC must exist).
- A `PromptPack` already deployed.

## Step 1: Create a SkillSource

Save as `skills.yaml`:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: SkillSource
metadata:
  name: support-skills
  namespace: dev-agents
spec:
  type: git
  git:
    url: https://github.com/anthropic/skills
    ref: { tag: v1.4.0 }
  interval: 1h
  targetPath: skills/anthropic
  filter:
    include: ["ai-safety", "pdf-processing"]
```

Apply and wait for the controller to fetch:

```bash
kubectl apply -f skills.yaml

kubectl get skillsource support-skills -n dev-agents -w
```

When `phase=Ready` and `skillCount` reflects the filtered set, the source is synced.

```bash
kubectl get skillsource support-skills -n dev-agents \
  -o jsonpath='{.status.conditions[?(@.type=="ContentValid")]}'
```

`status: "True"` means every retained `SKILL.md` parsed cleanly.

## Step 2: Attach to a PromptPack

Patch the existing pack:

```bash
kubectl patch promptpack support-pack -n dev-agents --type=merge --patch '
spec:
  skills:
    - source: support-skills
      include: ["ai-safety"]
      mountAs: compliance
  skillsConfig:
    maxActive: 5
    selector: model-driven
'
```

The PromptPack reconciler resolves the references, writes a JSON manifest into the workspace PVC, and surfaces three conditions:

```bash
kubectl get promptpack support-pack -n dev-agents \
  -o jsonpath='{range .status.conditions[?(@.type=="SkillsResolved")]}{.status} {.reason}{"\n"}{end}'

# Expected: "True AllSkillsResolved"
```

Other useful condition queries:

```bash
# All skills from this pack collide with no other source's skill names.
kubectl get promptpack support-pack -n dev-agents \
  -o jsonpath='{.status.conditions[?(@.type=="SkillsValid")]}'

# Every skill's allowed-tools is declared in the pack.
kubectl get promptpack support-pack -n dev-agents \
  -o jsonpath='{.status.conditions[?(@.type=="SkillToolsResolved")]}'
```

If `SkillToolsResolved=False`, a skill references a tool the pack doesn't declare — PromptKit would reject the activation at runtime, so add the tool to the pack or remove it from the skill's `allowed-tools`.

## Step 3: Verify the runtime sees the manifest

The next reconcile of any `AgentRuntime` referencing this pack will recreate its runtime pod with:

- the workspace content PVC mounted at `/workspace-content` (read-only);
- env var `OMNIA_PROMPTPACK_MANIFEST_PATH=/workspace-content/manifests/support-pack.json`.

Inspect the running pod:

```bash
kubectl exec -it <agent-pod> -c runtime -- env | grep OMNIA_PROMPTPACK
kubectl exec -it <agent-pod> -c runtime -- cat $OMNIA_PROMPTPACK_MANIFEST_PATH
```

Check the runtime logs for skill registration:

```bash
kubectl logs <agent-pod> -c runtime | grep -i skill
```

## Step 4: Test from a session

Open a session and ask the agent something the skill addresses (e.g. for `ai-safety`, ask about handling sensitive content). PromptKit's model-driven selector hands the LLM a Phase-1 index of available skills; the LLM calls the `skill__activate` tool to load the relevant one before answering.

You can confirm activation in the session-api `tool-calls` table:

```bash
curl -s http://session-api/api/v1/sessions/{id}/tool-calls | jq '.[] | select(.tool=="skill__activate")'
```

## Troubleshooting

| Symptom | Likely cause | Fix |
|---|---|---|
| `SkillsResolved=False / LookupFailed` | Referenced `SkillSource` doesn't exist in the same namespace | Check `kubectl get skillsource -n <pack-ns>`; per-workspace, no cross-namespace lookup |
| `SkillsResolved=False / LookupFailed: ... no synced artifact yet` | The `SkillSource` hasn't completed its first fetch | Wait for `SkillSource.status.phase=Ready` |
| `SkillsValid=False / NameCollision` | Two referenced sources publish skills with the same `name` | Use `include` on one of the refs to disambiguate |
| `SkillToolsResolved=False / UnknownTool` | A SKILL.md `allowed-tools` references a tool not in the pack | Either declare the tool in the pack or drop it from the skill |
| Runtime pod never picks up skills | `--workspace-content-path` flag unset on the operator | Set the flag in the operator deployment; default `/workspace-content` |
| Runtime can't read the manifest | PVC isn't mounted (volume `workspace-content` missing on the pod) | Confirm `WorkspaceContentPath` on `AgentRuntimeReconciler` and that the per-namespace PVC `workspace-{ns}-content` exists |

## Related

- [`SkillSource` CRD reference](/reference/skillsource/)
- [`PromptPack` CRD reference](/reference/promptpack/)
- [PromptKit Skills concepts](https://promptkit.altairalabs.ai/concepts/skills/)
