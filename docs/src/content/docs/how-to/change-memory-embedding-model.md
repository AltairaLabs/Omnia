---
title: Change the memory embedding model
description: Swap the embedding provider backing agentic memory, and safely change the vector dimension.
---

The memory store's semantic recall is backed by an embedding **Provider**
(`spec.role: embedding`). The vector dimension is taken from that provider's
model — memory-api sizes the embedding columns to match at startup, so you can
back memory with any embedding model (Azure/OpenAI 1536, ollama
`nomic-embed-text` 768, `mxbai-embed-large` 1024, `all-minilm` 384, …) by
configuration alone.

How a swap behaves depends on whether the new model's **dimension** matches the
one already stored.

## Same dimension (non-destructive)

Switching to a different model with the **same** vector dimension (for example
two 1536-dim models) needs no special handling. Update the embedding Provider's
`model`; the re-embed worker notices the model change and re-embeds existing
rows in the background. Recall quality is briefly mixed until the backfill
completes, but no data is dropped.

## Different dimension (destructive — requires consent)

Changing to a model with a **different** dimension (for example 1536 → 768)
cannot preserve the existing vectors: every stored embedding is discarded and
re-embedded from scratch. Because that is destructive, memory-api will **refuse
to start** when the configured dimension no longer matches the stored columns,
until you record one-shot consent for the change:

```
memory: changing the embedding dimension to 768 would discard existing
embeddings and requires one-shot consent ...
```

### 1. Point the embedding Provider at the new model

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: memory-embeddings
  namespace: agents
spec:
  role: embedding
  type: ollama
  model: nomic-embed-text
  embedding:
    dimensions: 768
```

### 2. Record consent for the change

Consent is **one-shot** — it names the exact target dimension and is consumed
the moment the change is applied, so it can never be left standing to silently
permit a later swap. Record it one of three ways:

- **Dashboard (recommended):** as a workspace **owner**, go to
  **Workspace Settings → Advanced → Change Memory Embedding Dimension**, enter
  the new dimension, and confirm.
- **Admin API:**

  ```bash
  curl -X POST "$MEMORY_API/admin/embedding-dimension-change" \
    -H 'Content-Type: application/json' \
    -d '{"target_dim": 768}'
  ```

- **Raw SQL** (break-glass, if you have direct database access):

  ```sql
  INSERT INTO memory_embedding_dim_change_consent (target_dim) VALUES (768);
  ```

### 3. Restart memory-api

On the next start, memory-api consumes the consent marker, reshapes the
embedding columns to the new dimension, and the re-embed worker repopulates the
vectors from the stored observations. Semantic recall is degraded until the
backfill finishes.

## Notes and limits

- **Fresh installs need no consent.** An empty store (no embeddings yet) is
  reshaped to the configured dimension automatically — the consent gate only
  applies when real embeddings would be discarded.
- **Maximum dimension is 2000.** pgvector's HNSW index is capped at 2000
  dimensions, so models above that (e.g. OpenAI `text-embedding-3-large` at
  3072) are rejected at startup. Choose a model with ≤ 2000 dimensions.
- **Editing the Provider warns you.** Changing an embedding Provider's
  dimension surfaces an admission warning reminding you that a destructive
  re-embed and consent are required. It's advisory only — it never blocks the
  edit.
- **The dimension change is deliberate, never accidental.** Without a matching
  consent marker, memory-api stays down rather than silently destroying
  embeddings.
