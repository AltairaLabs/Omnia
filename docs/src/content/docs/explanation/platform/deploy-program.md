---
title: "The PromptPack Deploy Program"
description: "How the promptarena-deploy-omnia adapter turns a workspace into a deploy profile and a pack into a running AgentRuntime"
sidebar:
  order: 11
---

Omnia is a Kubernetes-native runtime, but pack authors work in
[PromptArena](https://promptkit.altairalabs.ai/) — not `kubectl`. The **deploy
program** bridges the two: it lets a PromptPack author push a pack into an Omnia
workspace without hand-writing CRDs, using the `promptarena-deploy-omnia` deploy
adapter.

This page explains the moving parts — the deploy-profile export, the adapter
config schema, and how a deployed pack becomes a running
[AgentRuntime](/reference/core/agentruntime/). For the step-by-step walkthrough, see
[Deploy a PromptPack to Omnia](/tutorials/deploy-to-omnia/).

## The problem it solves

A pack on its own is inert. To run it in Omnia you need:

- an **API endpoint** and **workspace** to deploy into,
- the target workspace configured with a ready **[service group](/how-to/workspaces/configure-service-groups/)** (the session-api / memory-api backend agents persist to),
- an **API credential** scoped to that workspace,
- one or more **Providers** to back the LLM calls, and
- optionally **SkillSources** for retrieval/tools.

Getting each of these names exactly right by hand is error-prone — a typo in a
Provider name fails the deploy at plan time, and referencing a Provider that
isn't `Ready` fails it at runtime. The deploy-profile export exists to hand the
author a pre-filled, validated config block instead.

:::caution[The workspace needs a service group, or sessions silently vanish]
The deploy program creates a PromptPack and an AgentRuntime, but it does **not**
configure the workspace's session/memory backend. If the target workspace has no
ready service group, the deployed agent starts and chats but **silently falls back
to an in-memory session store** — conversations, token usage, and cost are **not
persisted**, dashboard session views stay empty, and memory is unavailable.
Configure at least one service group (conventionally `default`) on the workspace
**before** deploying — see
[Configure workspace service groups](/how-to/workspaces/configure-service-groups/).
:::

## The deploy profile

The **deploy profile** is a discovery payload for a single workspace. The
dashboard serves it at `GET /api/workspaces/{name}/deploy-profile`. It is
**discovery only** — the endpoint never returns a secret. It contains:

- `api_endpoint` — the dashboard's external ingress URL, derived from the
  forwarded host (falling back to `OMNIA_DASHBOARD_EXTERNAL_URL`).
- `workspace` — the target workspace name.
- `providers` — the discovery menu of Providers.
- `skills` — the discovery menu of SkillSources.

Two filters shape that menu:

**Ready-only.** Only Providers and SkillSources whose `status.phase` is `Ready`
are listed. Non-Ready resources (Unavailable, Error, still syncing) are excluded
because a deployment that references one fails.

**LLM-only providers.** Only `llm`-role Providers are exported. Embedding, TTS,
STT, and image Providers are workspace-level services — consumed by memory-api
and similar subsystems — not per-agent extras. Bundling them into an agent's
`spec.providers` breaks the pack at its first request, so the export drops them
(#1596). A Provider CRD with no explicit `spec.role` defaults to `llm`.

### Default-LLM picker and the token

The raw discovery payload lists real Provider names. Two things happen on top of
it when an author exports:

- **Default-LLM picker.** The runtime requires exactly one primary LLM bound
  under the alias `default`. The export UI asks the author to pick one Provider
  and marks it `name: default` in the adapter config, with `ref` pointing at the
  real Provider CRD.
- **Workspace-scoped token.** The profile itself carries no secret, so the
  export mints an `omnia_sk_` API token **scoped to this workspace** — it can
  deploy only here, and it acts with the author's own role in the workspace. On
  re-export the dashboard offers to **reuse** the saved token or **regenerate**
  it (revoking the old key), so repeated exports don't pile up duplicate keys.
  If the deployment uses a read-only API-key store, the token is left as a
  placeholder for the author to mint by hand.

### Browser-login autoconfigure

When the dashboard runs in OAuth mode, the CLI can mint the whole config through
a browser-login flow (the same shape as `gcloud auth login`) instead of
copy-paste. The Omnia-side contract is four endpoints:

1. `GET /api/cli/authorize` — the CLI's loopback `callback` + `state` are
   stashed; the browser is routed through OIDC login (if needed) to a workspace
   picker at `/cli/select`.
2. `POST /api/cli/grant` — re-checks the author's **editor** access, mints a
   **one-time exchange code**, and 303-redirects the browser back to the loopback
   callback. No token crosses the browser — only the code.
3. `POST /api/cli/token` — the CLI exchanges the code on a back channel. This is
   the only place a token is issued: it mints a **workspace-scoped, short-lived**
   token (default TTL 1 hour, `OMNIA_AUTH_CLI_TOKEN_TTL_SECONDS`) and returns it
   **together with the deploy profile**.

These entry points are public so an unauthenticated CLI can start the flow
(#1593); token creation must be enabled for the exchange to succeed. The precise
CLI command that drives this lives in the `promptarena` deploy tooling, not in
Omnia.

## The adapter config schema

The export produces the `config:` block the `promptarena-deploy-omnia` adapter
consumes:

```yaml
config:
  api_endpoint: https://omnia.example.com   # dashboard external URL
  workspace: team-acme                      # target workspace
  api_token: omnia_sk_...                   # workspace-scoped, show-once
  providers:
    - { name: default, ref: claude-sonnet, role: llm }
  skills:
    - docs-search
```

| Key | Meaning |
|-----|---------|
| `api_endpoint` | Dashboard ingress the adapter POSTs to |
| `workspace` | Workspace name; the server resolves the namespace from the Workspace CR |
| `api_token` | `omnia_sk_` bearer credential, workspace-scoped |
| `providers[]` | Provider bindings: `name` is the in-pack alias, `ref` is the real Provider CRD, `role` is the Provider role. Exactly one must be aliased `default` |
| `skills[]` | SkillSource names to attach |

The `providers[]` and `skills[]` names must match resources that actually exist
and are `Ready` in the workspace — which is exactly what the discovery menu
guarantees when you export rather than hand-write.

## From profile to AgentRuntime

The adapter uses the token to call the workspace REST API and create the
Kubernetes resources a running agent needs:

- a **PromptPack** (and its backing ConfigMap) holding the compiled pack — see
  [the PromptPack reference](/reference/core/promptpack/);
- an **AgentRuntime** that references the PromptPack, binds the selected
  Providers under `spec.providers` (with the `default`-aliased LLM as primary),
  and attaches the chosen skills.

The AgentRuntime exposes the runtime through
[`spec.facades[]`](/reference/core/agentruntime/) — a list of facade entries, each
serving one protocol. A pack deployed for browser chat lands as a single
`websocket` facade on port 8080 (the default); an agent can expose more
surfaces (for example an additional `a2a` entry) by listing multiple facade
entries. The operator then reconciles the AgentRuntime into a Deployment +
Service, and the pack is live.

The exact translation from adapter config to CRD fields lives in the external
`promptarena-deploy-omnia` adapter; Omnia's side is the discovery profile it
exports and the workspace REST API the adapter writes through.

That workspace REST API — served by the dashboard at
`POST /api/workspaces/{name}/agents` (and the matching PromptPack routes) — is a
**verbatim passthrough**. It takes the AgentRuntime body the adapter sends and
applies it to the Kubernetes API unchanged: no field translation, no
schema-version adaptation, no server-side defaulting of the spec. The only
validation is the AgentRuntime CRD's own OpenAPI/CEL schema, enforced by the
apiserver. This keeps the platform decoupled from any one pack format — but it
means **the adapter, not Omnia, owns the AgentRuntime schema it emits.**

## Schema-version contract and upgrades

Because the deploy path is a passthrough and the adapter lives in a separate repo
on its own release train, **no single component owns the AgentRuntime schema
contract at this boundary.** The adapter must emit the schema that matches the
AgentRuntime **CRD + operator** version installed in the target cluster. When
they drift, the failure mode is asymmetric and easy to misread:

- **Adapter behind the cluster** (adapter emits an older shape the installed CRD
  still accepts, but a newer operator can't reconcile): the deploy *succeeds*
  (`201 Created`) but the agent **never comes up** — the AgentRuntime sits with an
  empty `status` and no pods, and the only evidence is a repeating
  `Reconciler error` in the operator log. Nothing fails at deploy time, so this
  looks like a pack or cluster problem when it is really a version skew.
- **Cluster ahead of the adapter** (CRD already upgraded, adapter still old): the
  deploy **fails immediately** with the apiserver's validation error (e.g.
  `422 spec.facades: Required value`), surfaced straight back to the adapter. This
  is the loud, attributable failure — much easier to diagnose than the first case.

Two rules follow:

1. **Ship breaking CRD changes in lockstep.** A breaking AgentRuntime field change
   (for example the `spec.facade` → `spec.facades[]` cutover) must land in the
   in-app deploy wizard **and** the external `promptarena-deploy-omnia` adapter
   together with the operator/CRD release — otherwise every adapter-deployed agent
   silently stops reconciling on upgrade.
2. **Upgrade the CRDs explicitly.** `helm upgrade` does **not** upgrade CRDs — Helm
   installs the contents of `charts/omnia/crds/` only on first install and never
   touches them again. After upgrading the operator you must apply the new CRDs
   yourself (server-side, because they exceed the client-side apply annotation
   limit):

   ```bash
   kubectl apply --server-side --force-conflicts -f charts/omnia/crds/
   ```

   Skipping this leaves a new operator binary reconciling objects against an old
   CRD schema — the exact "created but never reconciles" trap above.

## Related

- [Deploy a PromptPack to Omnia](/tutorials/deploy-to-omnia/) — the walkthrough.
- [PromptPack reference](/reference/core/promptpack/) — the resource the deploy creates.
- [AgentRuntime reference](/reference/core/agentruntime/) — `spec.facades[]` and provider binding.
