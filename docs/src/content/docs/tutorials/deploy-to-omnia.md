---
title: "Deploy a PromptPack to Omnia"
description: "Bootstrap the promptarena-deploy-omnia adapter config from a workspace and deploy a pack"
sidebar:
  order: 7
---

This tutorial shows how to deploy a PromptPack to an Omnia workspace using the
`promptarena-deploy-omnia` deploy adapter. The fastest path is to **export a
deploy profile** from the dashboard, which bootstraps the adapter's `config:`
block for you.

## Bootstrap your deploy config (recommended)

In the dashboard, open your workspace → **Settings → Deploy → Export deploy
profile**.

This generates a ready-to-paste `config:` block containing:

- `api_endpoint` — your dashboard's external URL.
- `workspace` — the target workspace name.
- `api_token` — a **show-once** `omnia_sk_` token **scoped to this workspace**:
  it can only deploy here (not to any other workspace you can access), and it
  acts with your own role in this workspace. The first export mints the
  `deploy-<workspace>` key. On a later export, if that key still exists the
  dialog asks whether to **regenerate** it (revokes the old key and mints a
  fresh token — any previously downloaded profile for this workspace stops
  working) or **reuse** the token you saved, so re-exporting doesn't pile up
  duplicate keys.
- a **configure step** listing the **Ready** Providers and SkillSources in the
  workspace. Only `llm`-role Providers are exported — embedding, TTS, STT and
  image Providers are workspace-level services (consumed by memory-api and the
  like), not per-agent extras, and bundling them into a deployment breaks the
  pack at first request. Non-Ready resources (Unavailable, Error, still syncing)
  are also excluded, since a deployment that references one fails. You check
  which to include and pick **one LLM as the `default`** provider; the runtime
  requires a provider bound under `default` as its primary, so the export marks
  your choice as `name: default` (with `ref` pointing at the real Provider).

Paste the generated block into your arena deploy config:

```yaml
config:
  api_endpoint: https://omnia.example.com
  workspace: team-acme
  api_token: omnia_sk_...        # freshly minted, show-once, revocable
  # --- discovered in this workspace (pick what you need) ---
  providers:
    - { name: default, ref: claude-sonnet, role: llm }
  skills:
    - docs-search
```

:::caution[The token is a real credential]
The minted `api_token` is shown once. Store it securely and revoke it from
**Settings → API keys** when you no longer need it. It is confined to this
workspace, but anyone holding it can act with your role here.
:::

If your deployment uses a read-only API-key store, the dashboard still exports
the discovery menu but leaves the `api_token` as a placeholder — mint a token
manually under **Settings → API keys** and paste it in.

## Autoconfigure via browser login

When the dashboard runs in OAuth mode, the deploy CLI can skip the manual
copy-paste and mint everything through a browser-login flow, the same shape as
`gcloud auth login` or `gh auth login`. This is the fastest path for a local
workstation:

1. The CLI opens the dashboard's browser-login entry point
   (`GET /api/cli/authorize`), passing a loopback `callback` URL and a random
   `state`.
2. Your browser goes through the normal OIDC login (if you aren't already
   signed in), then lands on a **workspace picker** (`/cli/select`). You choose
   the target workspace.
3. Granting (`POST /api/cli/grant`) re-checks your **editor** access to that
   workspace, mints a **one-time exchange code**, and redirects the browser back
   to the CLI's loopback callback with `?code=&state=`. No token crosses the
   browser — only the short-lived code.
4. The CLI exchanges the code on a back channel (`POST /api/cli/token`). That
   endpoint mints a **workspace-scoped, short-lived** `omnia_sk_` token (default
   TTL 1 hour, `OMNIA_AUTH_CLI_TOKEN_TTL_SECONDS`) and returns it **together with
   the deploy profile** — the same `api_endpoint` / `workspace` / `providers` /
   `skills` discovery menu the dashboard export produces. The CLI writes the
   ready-to-use `config:` block for you.

The exact CLI command that drives this lives in the `promptarena` deploy
tooling; the endpoints above are the Omnia-side contract. Browser login requires
the dashboard to be in OAuth mode and API-key creation to be enabled — otherwise
fall back to the dashboard export or manual setup.

## Manual setup (fallback)

If you prefer to assemble the config by hand:

1. **Mint an API key** — go to **Settings → API keys**, create a key, and copy
   the `omnia_sk_` value (shown once).
2. **Copy the connection details** — your dashboard URL is the `api_endpoint`,
   and the workspace name is the `workspace`.
3. **Reference your resources** — list the Providers and SkillSources in the
   workspace (**Settings → Services**) and fill in `providers` / `skills` with
   their exact names. A typo in a Provider name fails the deploy at plan time.

## Deploy

With the `config:` block in place, run the deploy adapter as usual:

```bash
promptarena deploy --provider omnia
```

The adapter validates the config (`validate_config`), plans the changes, and
applies them to your workspace.
