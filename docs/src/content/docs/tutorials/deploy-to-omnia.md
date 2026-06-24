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
- `api_token` — a freshly minted, **show-once** `omnia_sk_` token **scoped to
  this workspace**: it can only deploy here (not to any other workspace you can
  access), and it acts with your own role in this workspace.
- a **discovery menu** of the Providers (with their roles) and SkillSources that
  actually exist in the workspace, so `providers` / `skills` are pre-filled with
  real names instead of guessed ones.

Pick the providers you need, bind each to a role, and paste the block into your
arena deploy config:

```yaml
config:
  api_endpoint: https://omnia.example.com
  workspace: team-acme
  api_token: omnia_sk_...        # freshly minted, show-once, revocable
  # --- discovered in this workspace (pick what you need) ---
  providers:
    - { name: default,  ref: claude-sonnet, role: llm }
    - { name: embedder, ref: text-embed-3,  role: embedding }
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
