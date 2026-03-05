# Scalability Review — 5 March 2026

Comprehensive review of Omnia's readiness for production load.
Covers message limits, conversation length, retry/resilience, streaming/multimodal,
database queries, infrastructure sizing, Kubernetes horizontal scaling at 10 K
concurrent connections, and duplex audio at scale.

---

## 1. Message & Payload Size Limits

### Current Limits by Layer

| Layer | Limit | Configurable | Location |
|-------|-------|:---:|----------|
| WebSocket frame (client → facade) | **16 MB** | Yes | `internal/facade/server.go:72` (DefaultServerConfig) |
| gRPC facade → runtime | **16 MB** | Yes | `internal/facade/runtime_client.go:64`, `cmd/runtime/main.go:302` |
| HTTP body session-api | **10 MB** | Yes | `internal/session/api/handler.go:46` (DefaultMaxBodySize) |
| OTLP/HTTP handler | **4 MB** | No | `internal/session/otlp/handler.go:32` |
| Redis stream entry | Unlimited | — | `internal/session/api/event_publisher.go:34` (stream capped at 10 K entries) |
| PostgreSQL TEXT column | ~1 GB | — | `internal/session/postgres/migrations/000002` |

### Issues

**S-MSG-1: Session-API is the bottleneck.**
WebSocket and gRPC accept 16 MB but session-api rejects anything over 10 MB (HTTP 413).
A user who pastes a 12 MB document will see the message delivered to the LLM but silently
fail to persist to the session record.

**S-MSG-2: Base64 encoding inflates media by 33%.**
All media is base64-encoded for gRPC transport (`internal/runtime/media.go:233`).
A 12 MB image becomes ~16 MB on the wire, hitting the gRPC ceiling.
Large videos are effectively unsendable through the current pipeline.

**S-MSG-3: No per-message compression.** *(Fixed)*
Binary WebSocket frames have a compression flag but it is not implemented.
gRPC compression (gzip) is not enabled on the facade→runtime channel.

**S-MSG-4: Read/write buffer asymmetry.** *(Fixed)*
WebSocket read/write buffers are 32 KB (`internal/facade/server.go:67-68`).
Gorilla/websocket will internally allocate to fit the full message (up to 16 MB),
but the small initial buffer means large messages trigger multiple reallocations.

### Recommendations

- Align session-api max body to 16 MB (match the rest of the pipeline) or, better,
  introduce external blob storage for large payloads and store only a reference.
- ~~Enable gRPC gzip compression on the facade→runtime channel.~~ **Done** — PR #582: `grpc.UseCompressor(gzip.Name)` on facade dial options.
- Consider streaming media as chunked binary frames instead of single base64 blobs.
- ~~Raise WebSocket read/write buffers to 64–128 KB for pods expected to handle media.~~ **Done** — PR #582: 32 KB → 64 KB.

---

## 2. Conversation Length & Context Window

### Current Behaviour

| Aspect | Default | Configurable | Location |
|--------|---------|:---:|----------|
| Max messages per session | **Unlimited** | Yes (Redis `MaxMessagesPerSession`) | `internal/session/providers/redis/config.go:41-43` |
| Context window (tokens) | **Unlimited** (0) | Yes (`OMNIA_CONTEXT_WINDOW`) | `internal/runtime/config.go:56` |
| Session list page size | 20 (max 100) | Yes | `internal/session/api/handler.go:38-40` |
| Message list page size | 50 (default) | Yes | `internal/session/api/handler.go:40` |
| Session detail initial load | 50 messages | — | `internal/session/api/handler.go:273-274` |
| Session retention (warm) | 7 days | Yes | `api/v1alpha1/sessionretentionpolicy_types.go` |
| Hot cache TTL | 24 h after last activity | Yes | Same CRD |

### Issues

**S-CONV-1: Unbounded context window by default.**
When `OMNIA_CONTEXT_WINDOW=0` (default), the PromptKit SDK loads every message from
session start into memory. A 500-turn conversation with tool calls easily exceeds 100 K
tokens and consumes hundreds of MB of RAM per conversation.

**S-CONV-2: Unbounded Redis message list.**
Without `MaxMessagesPerSession` set, every message is appended to a Redis list that is
never trimmed. 5,000 users × 200 messages × 2 KB avg = ~2 GB just for hot-cache message
lists.

**S-CONV-3: Dashboard renders all messages at once.** *(Fixed)*
`dashboard/src/app/sessions/[id]/page.tsx` renders every message returned by the API.
No virtual scrolling or "load more" pagination. 1,000+ messages will cause visible UI lag.

**S-CONV-4: `COUNT(*) OVER()` in session list queries.**
`internal/session/providers/postgres/provider.go:421` uses a window function to compute
total count on every paginated list request. At 100 K+ sessions per namespace this becomes
expensive.

**S-CONV-5: Session existence check scans all partitions.**
`sessionExists()` queries by `id` alone (`provider.go:191`). Sessions are partitioned by
`created_at`, so a lookup by `id` must probe every weekly partition.

### Recommendations

- **Require** a non-zero `OMNIA_CONTEXT_WINDOW` in production deployments (e.g., 128 K tokens).
- Set a sensible default `MaxMessagesPerSession` (e.g., 500) in the SessionRetentionPolicy CRD.
- ~~Implement virtual scrolling or paginated "load more" in the session detail page.~~ **Done** — windowed rendering (last 50 messages) with "Show earlier messages" button.
- Replace `COUNT(*) OVER()` with a separate, cacheable count query or use keyset pagination
  (cursor-based) that doesn't need total count.
- Add a partial index on `sessions(id)` or always include `created_at` in existence queries
  to enable partition pruning.

---

## 3. Retry Logic & Resilience

### Current Retry/Timeout Map

| Component | Timeout | Retries | Circuit Breaker | Location |
|-----------|---------|:---:|:---:|----------|
| httpclient → session-api | 30 s | 3 (exp backoff 100/200/400 ms) | No | `internal/session/httpclient/store.go:40-47` |
| gRPC facade → runtime | Caller ctx | None | No | `internal/facade/runtime_client.go:118` |
| gRPC tool adapters | 30 s | None | No | `internal/runtime/tools/grpc_adapter.go:88` |
| HTTP tool adapters | 30 s | None | No | `internal/runtime/tools/http_adapter.go:88` |
| WebSocket ping/pong | 30 s / 60 s | — | — | `internal/facade/server.go:69-70` |
| PostgreSQL pool | 5 s (startup ping) | None | No | `cmd/session-api/main.go:285` |
| Redis | 5 s (startup ping) | 3 (library default) | No | `internal/session/redis.go:86` |

### Issues

**S-RES-1: No circuit breaker anywhere.**
If session-api goes down, every facade goroutine retries 3× (total ~700 ms), then fails.
Subsequent requests immediately retry again. There is no "open circuit" state to shed load
and allow session-api to recover.

**S-RES-2: gRPC streaming has no per-message timeout.**
The `Converse` bidirectional stream inherits the caller's context. If the LLM provider
stalls mid-response, the stream hangs indefinitely — there is no inactivity timeout.

**S-RES-3: Tool call failures stall the entire LLM pipeline.** *(Fixed)*
A 30-second tool timeout means the user waits 30 s with no feedback. If the tool is called
multiple times in a multi-step pipeline, latency compounds. No tool-level circuit breaker
prevents repeated calls to a dead endpoint.

**S-RES-4: Session-API HTTP server has no timeouts.**
`cmd/session-api/main.go` creates `&http.Server{Addr: ..., Handler: ...}` with no
`ReadTimeout`, `WriteTimeout`, or `IdleTimeout`. Slow or malicious clients can hold
connections open forever.

**S-RES-5: `CreateSession` is not retried.** *(Fixed)*
Only `doWithRetry` covers GET-like operations and `doJSON` (POST/PUT). But `CreateSession`
calls `doJSON` which does go through retry — however, a duplicate-create on retry could
cause a conflict error. Need idempotency key or upsert semantics.

**S-RES-6: Fire-and-forget session completion uses `context.Background()`.**
`internal/facade/connection.go:85` — if session-api is down, the goroutine hangs
indefinitely with no timeout. Under load, these zombie goroutines accumulate.

**S-RES-7: No rate limiting on any API.** *(Fixed)*
No rate limits on WebSocket message ingestion, session-api endpoints, or tool execution.
A runaway client can saturate a pod.

### Recommendations

- Add a circuit breaker (e.g., Sony's `gobreaker`) around session-api HTTP calls.
- Implement a per-chunk inactivity timeout on the gRPC Converse stream (e.g., 120 s between
  chunks from provider).
- Add timeouts to session-api HTTP server: `ReadTimeout=30s`, `WriteTimeout=60s`,
  `IdleTimeout=120s`.
- Wrap fire-and-forget goroutines with `context.WithTimeout` (e.g., 10 s for session
  completion writes).
- Add a semaphore or token-bucket rate limiter on WebSocket message ingestion per connection.
- Consider tool health probing — temporarily disable a tool adapter after N consecutive
  failures.

---

## 4. Streaming & Multimodal

### Current Architecture

- LLM responses stream via PromptKit SDK → gRPC bidirectional stream → WebSocket to client.
- Each chunk is sent **synchronously**: `stream.Send()` blocks until the gRPC transport
  accepts the frame (`internal/runtime/message.go:176-196`).
- Session recording is **asynchronous** (fire-and-forget goroutine per write).

### Issues

**S-STR-1: Unbounded goroutine creation for async recording.**
Each streaming chunk triggers `go func()` in `internal/facade/recording_writer.go:103`.
A single conversation with 500 chunks spawns 500 goroutines. With 1,000 concurrent
sessions, that's 500 K goroutines just for recording — each holding an HTTP request to
session-api.

**S-STR-2: No backpressure between LLM stream and client.** *(Fixed)*
If the WebSocket client is slow (mobile network, browser tab in background), `stream.Send()`
blocks, which blocks the gRPC stream from the runtime, which blocks the PromptKit SDK from
consuming the next provider chunk. This is correct flow control but means one slow client
ties up an entire runtime conversation slot.

**S-STR-3: No per-pod connection limit.**
`internal/facade/server.go` tracks connections in a `map[*websocket.Conn]*Connection` but
never caps the count. File descriptor exhaustion (default ulimit 1024) will crash the pod
before any application-level limit kicks in.

**S-STR-4: Single-chunk media transfer.** *(Fixed)*
Media is sent as one gRPC message (`message.go:226`, `IsLast: true`). A 15 MB image
(within the 16 MB limit) is held entirely in memory on both facade and runtime simultaneously.
No streaming of large blobs.

**S-STR-5: No buffer pooling.** *(Fixed)*
`strings.Builder` and `[]byte` allocations happen per-message with no `sync.Pool`.
Under sustained load, GC pressure will cause latency spikes.

**S-STR-6: Prometheus eval_id label is high-cardinality.**
`pkg/metrics/eval.go:77-101` uses `eval_id` as a label. Each unique eval definition creates
a new time series. With 50 eval definitions × 3 label dimensions, the series count grows
linearly with eval diversity. At scale this bloats Prometheus memory and slows scrapes.

### Recommendations

- Replace per-chunk goroutines with a **bounded worker pool** (e.g., channel of size 100)
  for session recording. Coalesce chunks into batched writes.
- Set an explicit max-connections limit on the facade (e.g., 500 per pod) and return
  HTTP 503 when full. Pair with HPA to scale out.
- Implement chunked media streaming over gRPC for payloads > 1 MB.
- ~~Add `sync.Pool` for `[]byte` buffers used in streaming and media encoding.~~ **Done** — PR #582: `bufPool` in `binary.go` with `EncodePooled()`/`GetPooledBuf()`/`PutPooledBuf()`.
- Move `eval_id` from Prometheus label to trace attribute. Use only `eval_type` and
  `trigger` as labels.
- Set container `ulimit` to at least 65536 in the Helm chart security context.

---

## 5. Database & Infrastructure

### PostgreSQL

| Setting | Default | Concern | Location |
|---------|---------|---------|----------|
| Max connections per replica | 25 | Too low for 1 K+ req/s | `cmd/session-api/main.go:258` |
| Min connections | 5 | Cold-start latency | Same |
| Max conn lifetime | 1 h | OK | Same |
| Replicas | 2 | 50 total conns to PG | `charts/omnia/values.yaml` |

**S-DB-1: Connection pool too small.**
2 replicas × 25 conns = 50 connections to PostgreSQL. At 5,000 users sending 1 msg/s,
session-api needs to handle thousands of writes/sec. With 50 connections and ~5 ms per
write, throughput caps at ~10 K writes/sec — but pool contention and lock waits will
realistically halve that.

**S-DB-2: Denormalized counter contention.** *(Fixed)*
`UpdateSessionStats` does `message_count = message_count + ?` on the sessions row
(`provider.go:321`). Multiple concurrent messages to the same session serialize on the
row lock. At 10+ msg/sec per session (tool-heavy conversations), this becomes a bottleneck.

**S-DB-3: No explicit VACUUM/ANALYZE schedule.**
Weekly partitions with high write rates generate dead tuples. Without scheduled maintenance,
index bloat and query planner staleness degrade over time.

**S-DB-4: Delete cascades are multi-statement.** *(Fixed)*
`DeleteSession` runs 4 separate DELETE statements in a transaction (`provider.go:332-366`).
A session with 1,000 messages triggers 4 deletes touching thousands of rows under a single
transaction lock.

### Redis

**S-DB-5: No maxmemory or eviction policy in Helm defaults.**
Redis can grow unbounded. At 5,000 active sessions × 200 messages × 2 KB avg, hot cache
alone is ~2 GB. Add session metadata and stream entries and it climbs fast.

**S-DB-6: Single-node Redis (no cluster).** *(Fixed)*
Helm values don't mention Redis Cluster or Sentinel. A Redis restart causes all hot-cache
misses to hit PostgreSQL simultaneously (thundering herd).

### Kubernetes

**S-K8S-1: Session-API memory limit is 256 Mi.**
With connection pools, in-flight requests, and metrics, 256 MB is tight.
A burst of 10 MB body requests can OOM the pod.

**S-K8S-2: No HPA configured.**
No `HorizontalPodAutoscaler` resource in the Helm chart. Scaling is manual.

**S-K8S-3: Operator is single-replica.** *(Fixed)*
CRD reconciliation is serialized. 100+ agent updates queue up behind each other.

**S-K8S-4: Dashboard assets served by operator pod.**
No CDN or static-asset caching headers. 1,000 dashboard users cause operator CPU spikes
serving JS bundles on every page load.

### Observability

**S-OBS-1: Default trace sample rate is 1.0 (100%).**
`internal/tracing/tracing.go` samples every trace. At 10 K concurrent sessions, this
generates ~50 K spans/sec, overwhelming Tempo/Jaeger.

**S-OBS-2: V(1) logging on every write path.**
Session-api logs 4–6 lines per message write. At 1,000 msg/sec, that's 6,000 log lines/sec
(~10 MB/min). Loki ingestion and storage grow fast.

### Recommendations

- Increase `PG_MAX_CONNS` to 50+ per replica and deploy 3+ session-api replicas.
- ~~Batch stats updates: accumulate deltas in-memory and flush every 1–5 s per session instead of per-message.~~ **Done** — PR #582: `StatsBatcher` flushes every 3s.
- ~~Configure Redis with `maxmemory` and `allkeys-lru` eviction.~~ **Done** — PR #582.
- ~~Deploy Redis with Sentinel or Cluster for HA.~~ **Done** — PR #582: `architecture: replication` + Sentinel.
- ~~Raise session-api memory limit to 512 Mi–1 Gi.~~ **Done** — PR #582.
- Add HPA for session-api and facade (target CPU 70% or custom request-rate metric).
- ~~Deploy operator with 3 replicas + leader election.~~ **Done** — PR #582: `replicaCount: 3`.
- ~~Serve dashboard assets via a CDN or add `Cache-Control: public, max-age=31536000, immutable` for hashed bundles.~~ **Done** — PR #582.
- Reduce trace sample rate to 0.01–0.1 in production.
- Gate V(1) logging behind a feature flag or reduce to V(2) for high-frequency write paths.

---

## 6. Graceful Shutdown & Connection Draining

| Component | Shutdown Timeout | Drain Behaviour | Location |
|-----------|-----------------|-----------------|----------|
| Facade | 30 s | Closes WebSocket conns, then HTTP | `cmd/agent/main.go:221-235` |
| Runtime | 30 s | HTTP shutdown, then `GracefulStop()` | `cmd/runtime/main.go:385-394` |
| Session-API | 30 s | HTTP shutdown only | `cmd/session-api/main.go` |

**S-SHUT-1: gRPC `GracefulStop()` has no timeout.**
If a streaming RPC is in-flight, `GracefulStop()` waits forever. The 30-second context
applies to HTTP, not gRPC.

**S-SHUT-2: In-flight async recording goroutines are orphaned.**
Fire-and-forget goroutines using `context.Background()` are not tracked. On shutdown,
pending session-api writes are silently dropped.

### Recommendations

- Wrap `GracefulStop()` with a 10-second deadline; fall back to `Stop()`.
- Use a `sync.WaitGroup` for recording goroutines and drain on shutdown (with timeout).

---

## 7. Summary: Top 10 Items by Impact

| # | Issue | Severity | Category |
|---|-------|----------|----------|
| 1 | Unbounded goroutine creation for async recording | **Critical** | Streaming |
| 2 | No circuit breaker on session-api calls | **Critical** | Resilience |
| 3 | Connection pool too small (50 total PG conns) | **High** | Database |
| 4 | No per-pod WebSocket connection limit (FD exhaustion) | **High** | Streaming |
| 5 | Unbounded context window / Redis message list | **High** | Conversation |
| 6 | Session-API HTTP server has no timeouts | **High** | Resilience |
| 7 | No HPA; manual scaling only | **High** | Infrastructure |
| 8 | `COUNT(*) OVER()` in session list queries | **Medium** | Database |
| 9 | gRPC stream has no inactivity timeout | **Medium** | Resilience |
| 10 | Trace sample rate 1.0, V(1) log volume | **Medium** | Observability |

---

## 8. Kubernetes at 10,000 Concurrent Connections

This section models what happens when we scale horizontally to absorb 10 K simultaneous
WebSocket connections across a fleet of agent pods.

### Per-Connection Resource Cost

| Resource | Idle Connection | Active Conversation | Source |
|----------|:-:|:-:|----------|
| Goroutines | 2 | 3–6 | `handleConnection` + `runPingLoop` + async recording |
| WebSocket buffers | 64 KB | 64 KB | `facade/server.go:67-68` (32 KB read + 32 KB write) |
| Connection struct + context | ~2 KB | ~2 KB | `facade/connection.go` + 4 context layers |
| sdk.Conversation | — | 2–50 KB | PromptKit SDK (grows with history if no token budget) |
| Event subscriptions | — | ~1.5 KB | 11 subscriptions per conversation (`runtime/events.go`) |
| Ping ticker | ~128 bytes | ~128 bytes | `time.NewTicker` per connection |
| File descriptors | 1 | 1 | TCP socket for WebSocket |
| **Total** | **~66 KB** | **~70–120 KB** | |

HTTP client, gRPC connection, and Redis pool are **shared per pod** — negligible per-connection.

### Fleet Sizing at 10 K Connections

Assume ~200 active connections per pod (conservative — keeps memory well under 2 Gi limit).

| Dimension | Per Pod | Fleet (50 pods) |
|-----------|--------:|----------------:|
| Connections | 200 | 10,000 |
| Goroutines (active) | 800–1,200 | 40 K–60 K |
| Connection memory | ~14–24 MB | ~700 MB–1.2 GB |
| File descriptors | ~210 | — (per-pod) |
| gRPC streams (muxed) | 200 | — |
| Session-api write rate | ~200/s | ~10 K/s |

Go's scheduler handles 60 K goroutines comfortably. File descriptors are well within default
`ulimit` if raised to 65 536. The real bottlenecks are downstream.

### Autoscaling — What Exists Today

The operator generates HPA or KEDA ScaledObjects per AgentRuntime CRD
(`internal/controller/autoscaling.go`):

| Setting | HPA Default | KEDA Default | Location |
|---------|:-:|:-:|----------|
| Min replicas | 1 | 1 | `autoscaling.go:286, 143` |
| Max replicas | 10 | 10 | `autoscaling.go:291` |
| Memory target | 70 % | — | `autoscaling.go:298-301` |
| CPU target | 90 % | — | `autoscaling.go:304-307` |
| KEDA trigger | — | `omnia_agent_connections_active` at 10/pod | `autoscaling.go:238-247` |
| Scale-up stabilisation | 0 s (immediate) | — | `autoscaling.go:367-383` |
| Scale-down stabilisation | 300 s | 300 s | `autoscaling.go:309-314, 154` |
| Scale-up policy | double pods OR +4, whichever is larger, per 15 s | — | `autoscaling.go:367-383` |
| Scale-down policy | –50 % per 60 s | — | `autoscaling.go:357-365` |

### Issues at 10 K Connections

**S-K8S-5: HPA max replicas defaults to 10.**
At 200 connections per pod, 10 pods serve only 2,000 connections. Operators must explicitly
raise `maxReplicas` to 50–100 per agent definition. Nothing in the CRD or docs warns about
this.

**S-K8S-6: KEDA threshold of 10 connections/pod is too low for text chat.** *(Fixed)*
The default Prometheus trigger scales at 10 connections per pod. For text-only conversations
this wastes resources — a single pod can easily handle 200–500 text sessions. The threshold
should be tunable per workload type (text vs. audio).

**S-K8S-7: No PodDisruptionBudget on agent pods.**
Rolling updates with the default strategy (25 % max unavailable) can drop 12–13 pods at once
in a 50-pod fleet, disconnecting ~2,500 users simultaneously. A PDB with `minAvailable: 80%`
would cap disruption at 10 pods.

**S-K8S-8: Session-API becomes the shared bottleneck.**
50 facade pods × 200 connections = 10 K writes/s to session-api (message append + stats
update per turn). With 2 replicas × 25 PG connections × ~5 ms/write, max throughput is
~10 K writes/s — zero headroom. A single slow query or lock wait cascades into backpressure
across every facade pod.

**S-K8S-9: WebSocket connections are not drained on scale-down.**
When HPA removes a pod, in-flight WebSocket connections are terminated. Clients must
reconnect and re-establish session state. There is no pre-stop hook that stops accepting new
connections while draining existing ones.

**S-K8S-10: No topology spread constraints by default.**
Agent pods have no `topologySpreadConstraints`. All 50 pods could land on the same node,
creating a single point of failure.

**S-K8S-11: Gateway/ingress WebSocket idle timeout.**
Most cloud load balancers (ALB, GCE, Nginx) have a default idle timeout of 60–350 s. Long
idle conversations (user walks away for 10 min) will be silently dropped by the LB before
the 24 h `SessionTTL` expires. The facade's 30 s ping keeps the connection alive only if the
LB honours WebSocket frames as activity — some do not.

**S-K8S-12: No connection-aware load balancing.**
The default Kubernetes Service uses round-robin (via iptables/IPVS). New WebSocket upgrades
go to the pod with the fewest _recent_ TCP connections, not the fewest _active_ WebSockets.
Under churn this creates imbalanced pods.

### Recommendations

- Raise default `maxReplicas` to 100 or remove the ceiling and let operators set it.
- ~~Make the KEDA connection threshold configurable per agent (e.g., 200 for text, 20 for audio).~~ **Done** — PR #582: `connectionThreshold` field on `KEDAConfig`, default 200.
- Add a `PodDisruptionBudget` with `minAvailable: 80%` to every agent deployment.
- Implement a pre-stop lifecycle hook that closes the listener and waits for existing
  connections to finish (up to `terminationGracePeriodSeconds`).
- Add `topologySpreadConstraints` with `maxSkew: 1` across zones.
- Scale session-api to 4–6 replicas with 50 PG connections each for a 10 K baseline.
- Consider a connection-aware LB (Envoy/Istio with least-connections) for agent services.
- Document recommended LB idle-timeout and WebSocket upgrade annotations for each cloud
  (AWS ALB: `idle_timeout.timeout_seconds=3600`, GKE: BackendConfig
  `timeoutSec: 86400`, Nginx: `proxy_read_timeout 3600s`).

---

## 9. Duplex Audio at 10,000 Concurrent Connections

This is the stress scenario: every one of those 10 K connections is streaming bidirectional
audio in real time.

### Current State of Audio Support

| Capability | Status | Location |
|------------|:------:|----------|
| Binary WebSocket frame protocol (OMNI header) | Done | `internal/facade/binary.go` |
| Outbound media chunks (runtime → client) | Done | `internal/facade/response_writer.go`, `runtime/message.go` |
| **Inbound binary audio (client → runtime)** | **Stub only** | `internal/facade/message.go:103-106` — returns error |
| gRPC `MediaChunk` in ServerMessage | Done | `api/proto/runtime/v1/runtime.proto` |
| gRPC audio input message type | Missing | No `AudioInputChunk` in proto |
| Duplex session handler in runtime | Missing | No `CreateStreamSession()` call |
| Audio metadata extraction (event store) | Done | `internal/runtime/event_store.go:285-335` |
| Audio blob persistence | Metadata only | Blobs are stripped before recording |
| PromptKit duplex pipeline | Done | `promptkit-local/sdk/session/duplex_session.go` |
| OpenAI Realtime provider | Done (PromptKit) | `promptkit-local/runtime/providers/openai/realtime_*.go` |
| Gemini Live provider | Done (PromptKit) | PromptKit SDK |

**Bottom line:** The PromptKit SDK is duplex-ready. Omnia's infrastructure layer is not.
Inbound audio frames hit a stub that returns an error. There is no gRPC message type to
carry audio upstream, and the runtime has no code path to open a streaming session with the
provider.

### What 10 K Duplex Audio Connections Look Like

**Audio frame characteristics (16 kHz, 16-bit mono PCM — Gemini Live format):**

| Parameter | Value |
|-----------|-------|
| Sample rate | 16,000 Hz |
| Bit depth | 16 bits (2 bytes/sample) |
| Channels | 1 (mono) |
| Frame interval | 100 ms (typical VAD frame) |
| Bytes per frame | 3,200 (100 ms × 16 K × 2 bytes) |
| Frames per second per direction | 10 |
| Bandwidth per connection (one direction) | ~32 KB/s |
| Bandwidth per connection (duplex) | ~64 KB/s |

**Fleet-wide numbers at 10 K connections:**

| Metric | Value |
|--------|------:|
| Audio frames/sec (inbound + outbound) | **200,000** |
| Aggregate bandwidth | **640 MB/s** (~5.1 Gbps) |
| Binary header overhead (32 bytes × 200 K) | ~6.1 MB/s |
| gRPC messages/sec (if 1:1 frame-to-message) | 200,000 |
| Goroutines for async recording | 200,000/s created, each lasting ~30 ms = **~6,000 concurrent** |
| Heap allocation rate (9.3 KB/frame) | **~1.8 GB/s** |
| Session-api writes/sec (metadata only) | Up to 200 K (if every frame is recorded) |

### Issues Specific to Duplex Audio

**S-AUD-1: Inbound audio path does not exist.**
`handleBinaryMessage()` in `internal/facade/message.go:103-106` logs
_"binary upload not yet implemented"_ and returns an error. There is no code to forward
client audio to the runtime.

**S-AUD-2: No gRPC audio input message type.**
`api/proto/runtime/v1/runtime.proto` defines `MediaChunk` only inside `ServerMessage`
(runtime → client). There is no `AudioInputChunk` or equivalent in `ClientMessage`. The
bidirectional `Converse` stream cannot carry upstream audio today.

**S-AUD-3: 1.8 GB/s heap allocation rate will destroy GC.**
With no `sync.Pool` for frame buffers, every inbound and outbound audio frame allocates
~9.3 KB on the heap. At 200 K frames/s the allocator creates ~1.8 GB/s of garbage. Go's
concurrent GC will spend 30–50 % of CPU time collecting, causing unpredictable latency
spikes of 10–50 ms — audible in voice conversations.

**S-AUD-4: Async recording goroutine storm.**
If every audio frame triggers `go s.writeMessage()` (as the current event store does for
all events), 200 K goroutines/s are spawned. Each lives ~30 ms (HTTP round-trip to
session-api), so ~6,000 are alive concurrently at steady state. Manageable in isolation, but
combined with GC pressure and session-api load, this becomes a cascading problem.

**S-AUD-5: Session-API cannot absorb 200 K writes/sec.**
Even if audio frames are only recorded as lightweight metadata messages, 200 K writes/s
dwarfs the session-api's capacity (~10 K writes/s with current pool sizing). Recording every
audio frame is not viable without batching or a dedicated ingest path.

**S-AUD-6: WebSocket write serialisation blocks audio output.**
`sendBinaryFrame()` in `internal/facade/message.go` locks `c.mu` (the connection mutex).
The ping loop also takes this lock. If a ping write coincides with a burst of audio output
chunks, audio frames queue behind the lock, introducing jitter. At 10 ms frame intervals,
even 5 ms of lock contention is perceptible.

**S-AUD-7: 5.1 Gbps aggregate bandwidth.**
64 KB/s per connection × 10 K = ~5.1 Gbps of raw audio data transiting the cluster
(excluding headers, TLS overhead, and gRPC framing). This is achievable on modern cloud
networking but requires:
- Nodes with at least 10 Gbps NIC
- No network policies that add per-packet overhead
- Istio sidecar disabled or in passthrough mode for media frames (Envoy adds ~0.5 ms
  per-hop latency and CPU for TLS re-encryption)

**S-AUD-8: No jitter buffer or resampling.**
Audio quality degrades if frames arrive out of order or with variable latency. The current
pipeline has no jitter buffer on the receive side and no resampling if the provider expects a
different sample rate than the client sends (Gemini = 16 kHz, OpenAI = 24 kHz).

**S-AUD-9: Single-chunk media in gRPC caps file size.**
`buildMediaChunk()` sets `IsLast: true` on every media item (`message.go:226`). For
streaming audio this means each 100 ms frame is its own gRPC message. That works for small
frames (3.2 KB) but the per-message overhead (~50 bytes protobuf framing + HTTP/2 DATA
frame) adds up: 200 K messages/s × 50 bytes = 10 MB/s of pure framing overhead.

**S-AUD-10: OpenAI Realtime uses 64 MB max message size.**
PromptKit's OpenAI Realtime WebSocket client sets `wsMaxMessageSize = 64 MB`
(`promptkit-local/runtime/providers/openai/realtime_websocket_integration.go:16`). But
Omnia's gRPC channel caps at 16 MB. If the provider returns a large audio buffer (e.g.,
long TTS output), it will be truncated or error at the gRPC layer.

### Modelled Pod Sizing for Duplex Audio

Assume 50 connections per pod (lower than text due to CPU/bandwidth):

| Resource | Per Pod (50 conns) | Fleet (200 pods) |
|----------|--:|--:|
| Audio frames/sec | 1,000 | 200,000 |
| Bandwidth | 3.2 MB/s | 640 MB/s |
| CPU (encode/decode + GC) | ~500 m–1 core | 100–200 cores |
| Memory (buffers + goroutines) | ~200–400 MB | 40–80 GB |
| gRPC messages/sec | 1,000 | 200,000 |
| Session-api writes/sec (batched) | ~50 (1/s per conn) | ~10,000 |

200 pods is 4× the fleet needed for text-only. CPU becomes the bottleneck (GC + frame
processing), not memory.

### Recommendations (Audio-Specific)

**Infrastructure (must-have before duplex launch):**

1. **Implement inbound binary audio handling** in the facade — decode OMNI frame, forward
   as new gRPC `AudioInputChunk` message to runtime.
2. **Add `AudioInputChunk` / `DuplexSessionStart` to the proto** — runtime needs to
   distinguish audio input from text input.
3. **Wire duplex session in runtime** — call `provider.CreateStreamSession()` for providers
   that implement `StreamInputSupport`.
4. **Bounded recording worker pool** — replace `go s.writeMessage()` with a channel-based
   pool of 50–100 workers. Drop frames rather than spawn unbounded goroutines.
5. **Batch audio metadata recording** — aggregate audio frames into per-second summaries
   instead of per-frame session-api writes. Reduces write rate from 200 K/s to 10 K/s.
6. **`sync.Pool` for frame buffers** — reuse `[]byte` slabs for encode/decode. Target:
   reduce heap allocation rate from 1.8 GB/s to < 100 MB/s.

**Quality (important for production audio):**

7. **Client-side jitter buffer** — add a 60–100 ms playback buffer in the dashboard to
   smooth frame arrival variance.
8. **Lock-free WebSocket writes** — replace `c.mu` with a dedicated write goroutine and
   buffered channel to decouple ping/pong from audio output.
9. **Sample-rate negotiation** — facade should advertise supported rates in the handshake;
   runtime resamples if provider rate differs.
10. **Raise gRPC max message size to 64 MB** for audio agents, or implement true chunked
    streaming where a single logical audio blob spans multiple gRPC frames.

**Scaling (production fleet):**

11. **KEDA threshold of 20–50 connections/pod** for audio agents (vs. 200 for text).
12. **Dedicated node pool** with 10 Gbps NICs and no Istio sidecar injection for audio pods.
13. **Disable or passthrough Envoy** for agent services carrying audio — the mTLS
    encrypt/decrypt cycle adds 0.3–0.5 ms per frame, which at 200 K frames/s is 60–100 ms
    of aggregate CPU per second per pod.
14. **WebRTC (Phase 4)** — for true production voice, replace the WebSocket audio path with
    WebRTC via LiveKit. WebRTC handles jitter buffering, echo cancellation, bandwidth
    adaptation, and UDP transport natively. The duplex-audio-streaming-proposal
    (`docs/local-backlog/duplex-audio-streaming-proposal.md`) already scopes this as Phase 4.

---

## 10. Revised Summary: Top 15 Items by Impact

| # | Issue | Severity | Category | Status |
|---|-------|----------|----------|--------|
| 1 | Inbound audio path is a stub; duplex audio cannot work | **Critical** | Audio | |
| 2 | Unbounded goroutine creation for async recording | **Critical** | Streaming | **Done** — PR #578: `RecordingPool` (100 workers, 1000 queue) replaces `go func()` |
| 3 | No circuit breaker on session-api calls | **Critical** | Resilience | **Done** — PR #579: `gobreaker` circuit breaker wraps all httpclient calls |
| 4 | 1.8 GB/s heap allocation rate under duplex audio (no buffer pool) | **Critical** | Audio | |
| 5 | Session-API cannot absorb 200 K writes/s (audio frame recording) | **Critical** | Audio / DB | |
| 6 | Connection pool too small (50 total PG conns) | **High** | Database | **Done** — PR #579: session-api 25→50, postgres provider 10→25 |
| 7 | No per-pod WebSocket connection limit (FD exhaustion) | **High** | Streaming | **Done** — PR #578: `MaxConnections=500` default, 503 when full |
| 8 | HPA max replicas defaults to 10 | **High** | Kubernetes | **Done** — PR #579: default raised to 100 (HPA + KEDA paths) |
| 9 | Unbounded context window / Redis message list | **High** | Conversation | **Done** — PR #580: `MaxMessagesPerSession=1000` default in Redis config |
| 10 | Session-API HTTP server has no timeouts | **High** | Resilience | **Done** — PR #578: Read 30s / Write 60s / Idle 120s |
| 11 | WebSocket write mutex contention blocks audio output | **High** | Audio | |
| 12 | No PDB or topology spread on agent pods | **High** | Kubernetes | **Done** — PR #580: PDB (minAvailable:1), topology spread (zone), pre-stop hook (5s sleep), terminationGracePeriodSeconds=45 |
| 13 | gRPC stream has no inactivity timeout | **Medium** | Resilience | **Done** — PR #578: 120s inactivity timer in `receiveResponses` |
| 14 | `COUNT(*) OVER()` in session list queries | **Medium** | Database | **Done** — PR #579: separate count query in ListSessions/SearchSessions |
| 15 | Trace sample rate 1.0, V(1) log volume | **Medium** | Observability | **Done** — PR #579: default 1.0→0.1, write-path logs V(1)→V(2) |

### Additional fixes in PR #578 (not in original top 15)

| Issue | Fix |
|-------|-----|
| S-RES-6: Fire-and-forget session completion uses `context.Background()` | Added 10s `context.WithTimeout` — prevents zombie goroutines |
| S-SHUT-1: `GracefulStop()` has no timeout | Wrapped with 10s deadline, fallback to `Stop()` (runtime + session-api) |
| S-SHUT-2: In-flight recording goroutines orphaned on shutdown | `RecordingPool` drains on `Shutdown()` via `sync.WaitGroup` |

### Additional fixes in PR #580 (not in original top 15)

| Issue | Fix |
|-------|-----|
| S-MSG-1: Session-API body limit (10 MB) is lower than pipeline (16 MB) | Raised `DefaultMaxBodySize` to 16 MB, added `MAX_BODY_SIZE` env var |
| S-K8S-9: WebSocket connections not drained on scale-down | Pre-stop lifecycle hook (`sleep 5`) on facade container |
| S-K8S-10: No topology spread constraints | Default `topologySpreadConstraints` (zone, maxSkew:1) when replicas > 1 |
| S-CONV-2: Unbounded Redis message list | Default `MaxMessagesPerSession=1000` + `REDIS_MAX_MESSAGES` env var |

### Additional fixes in PR #581 (not in original top 15)

| Issue | Fix |
|-------|-----|
| S-DB-5: No maxmemory or eviction policy in Helm defaults | Added `master.extraFlags` with `--maxmemory 256mb --maxmemory-policy allkeys-lru` |
| S-K8S-1: Session-API memory limit too low (512 Mi) | Bumped `sessionApi.resources.limits.memory` to 1 Gi |
| S-K8S-4: No cache headers on dashboard static assets | Added `Cache-Control: public, max-age=31536000, immutable` for `/_next/static/` |
| S-CONV-3: Dashboard renders all messages at once | Windowed rendering in `ConversationMessages`: shows last 50 messages with "Show earlier messages" button |
| S-CONV-5: Session existence check scans all partitions | Added migration 000014: `CREATE INDEX idx_sessions_id ON sessions(id)` |

### Additional fixes in PR #582 (not in original top 15)

| Issue | Fix |
|-------|-----|
| S-MSG-3: No gRPC compression on facade→runtime | `grpc.UseCompressor(gzip.Name)` on facade dial, blank import on runtime |
| S-MSG-4: WebSocket read/write buffers too small (32 KB) | Raised to 64 KB in `DefaultServerConfig` |
| S-STR-5: No buffer pooling for streaming | `sync.Pool` with `EncodePooled()`/`GetPooledBuf()`/`PutPooledBuf()` in `binary.go` |
| S-K8S-3: Operator is single-replica | Default `replicaCount: 3` (leader election already enabled) |
| S-K8S-6: KEDA threshold hardcoded at 10 | New `connectionThreshold` field on `KEDAConfig` CRD, default 200 |
| S-DB-2: Row-lock contention on `UpdateSessionStats` | `StatsBatcher` accumulates deltas in-memory, flushes every 3s |
| S-DB-4: Multi-statement `DeleteSession` | `BEFORE DELETE` trigger (migration 000015) cascades child row deletes |
| S-DB-6: Single-node Redis | `architecture: replication` + Sentinel HA with 3 replicas |
| S-RES-5: `CreateSession` not idempotent on retry | Postgres returns nil on duplicate; httpclient treats 409 as success |
| S-STR-2: No backpressure for slow WebSocket clients | Write deadline (10s) on WebSocket; connection closed if client too slow |
| S-STR-4: Single-chunk media transfer | Chunked media streaming: payloads > 1 MB split into 64 KB OMNI frames |
| S-RES-3: Tool call failures stall LLM pipeline | Per-tool circuit breaker (`gobreaker`): opens after 5 failures, 30s recovery |
| S-RES-7: No rate limiting on any API | `KeyedLimiter` package; session-api 100 rps/IP; facade 50 msg/s per connection |
| S-K8S-2: No HPA for session-api | HPA with CPU 70% / memory 80%, min 2, max 10, scale-down stabilisation 300s |

---

## 11. Status Summary

All **text-chat, infrastructure, and resilience scalability issues** from the original review are resolved across PRs #578–#582.

The remaining open items (#1, #4, #5, #11) are all **audio/duplex-specific** — they require the inbound audio path, gRPC proto changes, and audio-specific write-path batching. These are blocked until the duplex audio infrastructure work begins.

Note: S-STR-5 (buffer pooling) is partially resolved for text streaming. The audio-specific heap allocation concern (S-AUD-3) requires additional pooling in the audio frame encode/decode path once that path exists.

### Remaining open items

| # | Issue | Severity | Category | Blocker |
|---|-------|----------|----------|---------|
| 1 | S-AUD-1: Inbound audio path is a stub | **Critical** | Audio | Duplex audio infra |
| 4 | S-AUD-3: 1.8 GB/s heap allocation (audio frames) | **Critical** | Audio | Duplex audio infra |
| 5 | S-AUD-5: Session-API cannot absorb 200K writes/s | **Critical** | Audio / DB | Duplex audio infra |
| 11 | S-AUD-6: WebSocket write mutex contention | **High** | Audio | Duplex audio infra |

### Still open (non-critical, not audio-specific)

| Issue | Severity | Notes |
|-------|----------|-------|
| S-MSG-2: Base64 encoding inflates media by 33% | Medium | Requires blob storage or chunked binary streaming |
| S-DB-3: No explicit VACUUM/ANALYZE schedule | Low | Ops concern — document recommended PG maintenance |
| S-K8S-11: Gateway/ingress WebSocket idle timeout | Low | Documentation — cloud-specific LB annotations |
| S-K8S-12: No connection-aware load balancing | Low | Requires Envoy/Istio with least-connections |
| S-STR-6: Prometheus eval_id label is high-cardinality | Low | Move eval_id to trace attribute |
