# SharePoint RAG Hero Demo — Setup & Runbook

The hero demo tells one story: **agents are production workloads, and Omnia
catches AI-specific regressions the way CI/CD catches code regressions.** A
two-stage RAG agent (institutional-memory index → SharePoint document fetch)
serves on-call SREs. You ship a "better" candidate, eval-gated canary analysis
spots the quality regression, and Omnia **auto-rolls-back** — the hero beat.

All assets live behind `sharepointHero.enabled` in the `omnia-demos` chart.
This guide covers what you provision once (Azure AD + SharePoint + secrets),
how to install, the ~5-minute demo arc, and how to validate each beat.

---

## 1. Prerequisites

### 1.1 Cluster

- Omnia core **and** enterprise (Arena, rollout analysis) installed and
  healthy. The hero demo uses ArenaJob, ArenaSource, RolloutConfig analysis,
  ToolPolicy, and SessionPrivacyPolicy — all enterprise.
- **Istio** present: the rollout splits traffic with a VirtualService /
  DestinationRule over the `omnia.altairalabs.ai/variant` pod label.
- **In-cluster Prometheus** scraping Omnia (the rollout gate runs PromQL).
  Default query endpoint: `http://prometheus-operated.monitoring:9090`
  (override via `sharepointHero.prometheusUrl`).
- A `demo` workspace (override via `sharepointHero.workspace`) with
  `memory-api` reachable at `sharepointHero.memoryApiUrl`
  (default `http://memory-api.omnia-demo:8080`).

### 1.2 Azure AD app registration (for the SharePoint Graph adapter)

The demo-only adapter (`demos/sharepoint-adapter`) reads SharePoint via
Microsoft Graph using client-secret credentials.

1. **App registration** → Azure Portal → Entra ID → App registrations → New.
2. **API permissions** → Microsoft Graph → *Application* permissions:
   - `Sites.Read.All`
   - `Files.Read.All`

   Then **Grant admin consent** (application permissions require it).
3. **Client secret** → Certificates & secrets → New client secret. Copy the
   value immediately.
4. Record: **Directory (tenant) ID**, **Application (client) ID**, the
   **secret value**.

### 1.3 SharePoint content

Create (or reuse) a SharePoint site with a few documents the agent can cite —
e.g. an incident-failover runbook and a data-handling policy. For the demo
beats you want:

- **One "allowed" site** with the runbook + policy docs. Note its **site ID**
  (`GET /sites/{hostname}:/sites/{path}` via Graph Explorer, or the adapter's
  `/list` once running).
- **One "restricted" site** whose URL contains the string `restricted`. The
  `ToolPolicy` denies any `fetch_sharepoint` call whose `body.url` contains
  `restricted` — this is the governance beat.
- At least one doc containing **synthetic PII** (a fake name/email/phone) so
  the `SessionPrivacyPolicy` redaction beat has something to redact. Use
  obviously fake values — never real personal data.

> **Real RAG, not whole-doc injection.** The seed Job POSTs each document's
> extracted text to memory-api `/api/v1/institutional/ingest`, which **chunks
> and embeds** it into the institutional-memory index (multiple
> `knowledge_reference` memories per doc, keyed by
> `about={sharepoint_doc, <url>#<index>}` so re-seeding supersedes rather than
> duplicates). At query time the agent runs **semantic hybrid retrieval**
> (`retrieval.strategy: semantic`) over those chunks, then fetches full content
> for the top hit through the adapter. The restricted site is indexed but
> blocked two ways: a CEL **deny-filter at retrieval**
> (`accessFilter.denyCEL`, fail-closed) and the `ToolPolicy` **at fetch** —
> defense in depth.
>
> **Ingestion is configurable (#1214).** Strategy (`chunk` | `summary` |
> `summaryThenChunk`) × summarizer (`extractive` no-LLM | `agent` async
> work-queue) live on `MemoryPolicy.spec.ingestion`, read live by memory-api
> (the `--ingest-strategy` / `--ingest-summarizer` / `--ingest-chunk-*` flags
> are fallback defaults). **The demo runs the defaults — `chunk` + `extractive`,
> both no-LLM** — which are the working end-to-end paths. The `agent` backend is
> contract-complete but inert until the summarizer AgentRuntime trio ships, so
> don't select it for the demo.

### 1.4 Pre-created secrets

Create these in the demo namespace **before** `helm install` (the chart
references them by name; it does not create them). Substitute real values.

```bash
NS=omnia-demo

# Azure AD client-secret creds for the Graph adapter.
kubectl -n "$NS" create secret generic sharepoint-adapter-creds \
  --from-literal=AZURE_TENANT_ID=<tenant-id> \
  --from-literal=AZURE_CLIENT_ID=<client-id> \
  --from-literal=AZURE_CLIENT_SECRET=<client-secret>

# Anthropic key for baseline + candidate + self-play providers.
# Default key name for a claude Provider is ANTHROPIC_API_KEY.
kubectl -n "$NS" create secret generic anthropic-api-key \
  --from-literal=ANTHROPIC_API_KEY=<anthropic-key>

# Embedding key for semantic memory retrieval.
# Default embedding.provider is openai → key name OPENAI_API_KEY.
kubectl -n "$NS" create secret generic embedding-api-key \
  --from-literal=OPENAI_API_KEY=<openai-key>
```

| Secret | Keys | Used by |
|--------|------|---------|
| `sharepoint-adapter-creds` | `AZURE_TENANT_ID`, `AZURE_CLIENT_ID`, `AZURE_CLIENT_SECRET` | adapter Deployment, seed Job |
| `anthropic-api-key` | `ANTHROPIC_API_KEY` | baseline / candidate / self-play Providers |
| `embedding-api-key` | `OPENAI_API_KEY` | AgentRuntime memory embedding |

Secret names are configurable in `values.yaml`
(`sharepointHero.adapter.credentialsSecret`,
`sharepointHero.providers.*.secretRef`, `sharepointHero.embedding.secretRef`,
`sharepointHero.loadtest.selfplay.secretRef`).

---

## 2. Install

Set at minimum the SharePoint site ID. Keep `shipCandidate=false` for the
initial install so the agent runs stable-only and the canary is triggered on
demand during the demo.

```bash
helm upgrade --install omnia-demos charts/omnia-demos \
  --namespace omnia-demo --create-namespace \
  --set sharepointHero.enabled=true \
  --set sharepointHero.shipCandidate=false \
  --set sharepointHero.adapter.sharepointSiteId='<allowed-site-id>'
```

What this renders (all gated on `sharepointHero.enabled`):

| Resource | Purpose |
|----------|---------|
| `sharepoint-adapter` Deployment + Service | Graph `/list` + `/fetch` for `fetch_sharepoint` |
| `rag-hero-seed` Job (post-install/upgrade hook) | indexes SharePoint docs as institutional memories |
| `rag-hero-pack` PromptPack (v1 + v2) | v1 = stable prompt, v2 = verbose candidate prompt |
| `rag-hero-baseline` / `rag-hero-candidate` Providers | stable model vs pricier candidate model |
| `rag-hero-tools` ToolRegistry + `rag-hero-tool-policy` ToolPolicy | `fetch_sharepoint` http tool + restricted-site deny |
| `rag-hero-privacy` SessionPrivacyPolicy | PII redaction |
| `rag-hero` AgentRuntime | the agent (memory + evals + rollout, rollout gated on `shipCandidate`) |
| `rag-hero-vs` / `rag-hero-dr` (Istio) | variant traffic split |
| `rag-hero-quality-check` RolloutAnalysis | PromQL gate on `omnia_eval_faithfulness{variant="candidate"}` |
| `rag-hero-arena-bundle` ConfigMap + `rag-hero-arena` ArenaSource + `rag-hero-loadtest` ArenaJob | self-play traffic generator |

> **Pre-flight (no cluster mutation):** render and server-validate first:
> ```bash
> helm template omnia-demos charts/omnia-demos \
>   --set sharepointHero.enabled=true \
>   --set sharepointHero.adapter.sharepointSiteId=stub \
>   | kubectl apply --dry-run=server -f -
> ```

---

## 3. Smoke-validate the install

Run these before demoing — each maps to a beat and fails loudly if a
prerequisite is missing.

```bash
NS=omnia-demo

# Adapter is up and can reach Graph (lists docs from the allowed site).
kubectl -n "$NS" port-forward svc/sharepoint-adapter 8080:8080 &
curl -s localhost:8080/healthz
curl -s -XPOST localhost:8080/list | jq '.[].title'

# Seed Job ingested the docs into institutional memory (chunked + embedded —
# expect total > number of docs: roughly one memory per RAG chunk).
kubectl -n "$NS" get job rag-hero-seed -o jsonpath='{.status.succeeded}'
curl -s "$MEMORY_API_URL/api/v1/institutional/memories?workspace=demo" | jq '.total'

# Agent is reconciled and serving.
kubectl -n "$NS" get agentruntime rag-hero
kubectl -n "$NS" get pods -l app.kubernetes.io/name=rag-hero

# Arena bundle synced and the loadtest is running / completed.
kubectl -n "$NS" get arenasource rag-hero-arena      # PHASE should be Ready
kubectl -n "$NS" get arenajob rag-hero-loadtest      # PHASE Running → Succeeded
```

**Confirm the variant label is emitting** (the discriminator the gate keys on).
Once the loadtest has driven traffic, query Prometheus:

```promql
omnia_eval_faithfulness{agent="rag-hero", namespace="omnia-demo"}
```

You should see series for `variant="stable"`. After you ship the candidate
(below) you'll also see `variant="candidate"`. If the candidate label never
appears, the canary will never get a gate signal — check that the eval worker's
collector declares `variant` in its instance labels (see the design notes).

---

## 4. The ~5-minute demo arc

1. **Set the scene (≈45s).** Show the dashboard: `rag-hero` agent serving live
   self-play traffic (the ArenaJob). Sessions stream in; eval metrics
   (faithfulness, answer relevance) and cost are green on `variant=stable`.
   *"This is an agent in production — runbook Q&A for on-call SREs, grounded in
   our SharePoint docs."*

2. **Show grounded retrieval (≈45s).** Open a session: the agent runs semantic
   retrieval over the **chunked** institutional-memory index, surfaces the
   relevant `knowledge_reference` chunks, then calls `fetch_sharepoint` to pull
   the full runbook content for the top hit. Two-stage RAG (real chunk-and-embed
   retrieval → on-demand fetch), live.

3. **Governance beat (≈30s).** Show restricted content blocked **twice**: the
   CEL `accessFilter.denyCEL` drops restricted chunks at **retrieval** (fail-
   closed), and `ToolPolicy` denies any `fetch_sharepoint` to the restricted
   site at **fetch** — defense in depth. Then show a response where
   `SessionPrivacyPolicy` redacted synthetic PII. *"Unsafe retrieval and data
   leaks are AI-specific failure modes — Omnia enforces them inline."*

4. **Ship the change (≈60s).** Roll out the "improved" candidate (a more
   verbose v2 prompt on a pricier model):
   ```bash
   helm upgrade omnia-demos charts/omnia-demos \
     --namespace omnia-demo --reuse-values \
     --set sharepointHero.shipCandidate=true
   ```
   The rollout starts: 20% of traffic shifts to `variant=candidate`. Show the
   VirtualService weight split and candidate sessions appearing.

5. **Eval-gated auto-rollback — the hero beat (≈90s).** The candidate's verbose
   prompt hallucinates more → `omnia_eval_faithfulness{variant="candidate"}`
   drops below the gate threshold. The `rag-hero-quality-check` RolloutAnalysis
   fails, and because `rollback.mode: automatic`, Omnia **reverts to stable**
   without a human in the loop. Show the analysis run failing and traffic
   snapping back to 100% stable.
   *"That's CI/CD for agents — a quality regression caught and rolled back by
   policy, not by a 2 a.m. page."*

6. **Land it (≈30s).** Back on stable, metrics recover. Recap: production
   agent, AI-specific guardrails, eval-gated delivery.

---

## 5. Reset / re-run

```bash
NS=omnia-demo

# Re-arm the canary: revert to stable-only.
helm upgrade omnia-demos charts/omnia-demos --namespace "$NS" \
  --reuse-values --set sharepointHero.shipCandidate=false

# Re-run the traffic generator (ArenaJob runs once per apply; delete to rerun).
kubectl -n "$NS" delete arenajob rag-hero-loadtest
helm upgrade omnia-demos charts/omnia-demos --namespace "$NS" --reuse-values

# Re-seed institutional memory (idempotent; safe to re-run).
kubectl -n "$NS" delete job rag-hero-seed --ignore-not-found
helm upgrade omnia-demos charts/omnia-demos --namespace "$NS" --reuse-values
```

To tear the demo down entirely, set `sharepointHero.enabled=false` and upgrade
(or `helm uninstall omnia-demos`). The pre-created secrets are not chart-managed
and survive — delete them manually if desired.

---

## 6. Troubleshooting

| Symptom | Likely cause | Fix |
|---------|--------------|-----|
| Adapter `/list` 401/403 | Graph admin consent not granted, or wrong client secret | Re-grant `Sites.Read.All` / `Files.Read.All`; rotate secret |
| Seed Job fails | adapter not ready, or `memory-api` unreachable | check adapter `/healthz`; verify `memoryApiUrl` |
| `ArenaSource` stuck `Pending`/`Error` | `workspaceContent.enabled=false` on the arena controller, or ConfigMap missing | install with workspace content enabled; check `rag-hero-arena-bundle` exists |
| `ArenaJob` no traffic | agent WebSocket URL not resolved (fleet mode) | confirm `rag-hero` pods are Ready; check worker logs for `no WebSocket URL for agent` |
| Canary never gated | `variant="candidate"` series absent in Prometheus | confirm eval worker collector declares the `variant` instance label; confirm candidate is receiving traffic |
| Rollback doesn't fire | analysis threshold not crossed, or `rollback.mode` not automatic | lower the gate threshold for the demo; verify `rag-hero` rollout `rollback.mode: automatic` |
