# Retry Execution Design Spec

**Date:** 2026-04-12
**Related PR:** #790 (CRD shape + config plumbing, merged)
**Related issue:** #779 (forward parsed tool config fields to executors)

## 1. Goal

Implement retry logic for tool execution in the runtime. PR #790 plumbed transport-specific retry policies (`HTTPRetryPolicy`, `GRPCRetryPolicy`, `MCPRetryPolicy`) from CRD through the operator into the runtime's `HandlerEntry` config. The policies are loaded but ignored at execution time. This PR makes them operational.

## 2. Architecture

### Core retry engine

A generic `retryWithBackoff` function in `internal/runtime/tools/retry.go`:

```go
type retryPolicy struct {
    MaxAttempts       int32
    InitialBackoff    time.Duration
    BackoffMultiplier float64
    MaxBackoff        time.Duration
}

func retryWithBackoff(
    ctx context.Context,
    log logr.Logger,
    span trace.Span,
    policy retryPolicy,
    attemptTimeout time.Duration,
    classify func(error) (retryable bool, retryAfter time.Duration),
    fn func(ctx context.Context) (json.RawMessage, error),
) (json.RawMessage, error)
```

**HTTP variant:** The HTTP classifier needs access to the response (status code, headers), not just the error. For HTTP, the `fn` closure captures the `*http.Response` in its scope and the `classify` closure reads it. Concretely:

```go
var lastResp *http.Response
result, err := retryWithBackoff(ctx, log, span, policy, timeout,
    func(err error) (bool, time.Duration) {
        return classifyHTTPResult(httpCallResult{
            StatusCode: lastResp.StatusCode,
            Headers:    lastResp.Header,
            Err:        err,
        }, retryPolicy)
    },
    func(attemptCtx context.Context) (json.RawMessage, error) {
        resp, err := client.Do(req.WithContext(attemptCtx))
        lastResp = resp
        if err != nil { return nil, err }
        defer resp.Body.Close()
        return processHTTPResponse(resp)
    },
)
```

This keeps `retryWithBackoff` generic (it only sees `func(error)`) while giving the HTTP classifier access to the full response via closure capture.

The loop:
1. Create `attemptCtx` with per-attempt timeout from `HandlerEntry.Timeout` (0 means no timeout).
2. Call `fn(attemptCtx)`.
3. On success, return.
4. On error, call `classify(err)`. If not retryable, return immediately.
5. If retryable, compute delay: `min(initialBackoff * multiplier^attempt, maxBackoff)` with ~10% jitter. If classifier returned a `retryAfter > 0`, use `max(computed, retryAfter)` capped at `maxBackoff`.
6. Log at `V(1)` and add a span event (see section 5).
7. Sleep for the delay, respecting context cancellation.
8. If all attempts exhausted, return `fmt.Errorf("tool %s: %d attempts exhausted: %w", toolName, maxAttempts, lastErr)`.

**No-policy fast path:** When no retry policy is configured, the extraction helpers return `retryPolicy{MaxAttempts: 1}`. With `MaxAttempts == 1`, `retryWithBackoff` calls `fn` once and returns — no backoff logic, no overhead.

### Composition with circuit breaker

gRPC execution already uses a circuit breaker (`e.breakers.Execute()`). Retry wraps the circuit breaker: each retry attempt independently goes through the breaker. If the breaker is open, the attempt fails fast, consuming one retry attempt. This is the standard pattern (Polly, resilience4j).

```
retry loop
  └─ circuit breaker
       └─ gRPC call
```

### Timeout model

Per-attempt timeout. Each retry attempt gets its own `context.WithTimeout` derived from `HandlerEntry.Timeout`. A handler with `timeout: 5s` and `maxAttempts: 3` could take up to ~15s plus backoff delays. Users who want to cap total duration reduce `maxAttempts` or the timeout.

## 3. HTTP execution: replace PromptKit executor

The current `executeHTTP` delegates to PromptKit's `HTTPExecutor.Execute()`, which closes the `*http.Response` internally. This prevents the retry classifier from inspecting response headers (needed for status code classification and `Retry-After`).

**Change:** `executeHTTP` builds and executes HTTP requests directly using a standard `http.Client`, replacing the PromptKit dependency for HTTP tool execution. This gives us:
- Direct access to `*http.Response` for status code and header inspection
- `Retry-After` header parsing for backoff floor
- Cleaner error classification without string-parsing PromptKit error messages

The request building logic (URL construction, header injection, body mapping, OTel propagation) stays in Omnia — it's already partly there for policy headers. The PromptKit `HTTPExecutor` import is removed.

`executeOpenAPI` delegates to the same HTTP path and gets the same benefit.

## 4. Error classifiers

### HTTP classifier

```go
type httpCallResult struct {
    StatusCode int
    Headers    http.Header
    Err        error
}

func classifyHTTPResult(result httpCallResult, policy *RuntimeHTTPRetryPolicy) (retryable bool, retryAfter time.Duration)
```

- **Network errors** (connection refused, DNS, timeout): retryable if `policy.RetryOnNetworkError` is true. Detected via `errors.Is(err, context.DeadlineExceeded)`, `net.Error` interface, or unwrapping to connection errors.
- **Status codes**: retryable if `result.StatusCode` is in `policy.RetryOn` (e.g., `[502, 503, 504]`).
- **`Retry-After` header**: when `policy.RespectRetryAfter` is true and the response has a `Retry-After` header, parse it (supports both seconds and HTTP-date formats per RFC 9110) and return it as `retryAfter`. Capped at `policy.MaxBackoff` in the retry loop.
- **All other errors**: not retryable.

### gRPC classifier

```go
func classifyGRPCError(err error, retryableStatusCodes []string) (retryable bool, retryAfter time.Duration)
```

- Extract gRPC status via `status.FromError(err)`.
- Retryable if the status code name (e.g., `"UNAVAILABLE"`, `"DEADLINE_EXCEEDED"`) is in `retryableStatusCodes`.
- Circuit breaker errors (wrapped by `e.breakers.Execute()`) are not retryable — detected by checking if the error is not a gRPC status error.
- `retryAfter` is always 0 (gRPC has no standard retry-after mechanism).

### MCP classifier

```go
func classifyMCPError(err error) (retryable bool, retryAfter time.Duration)
```

- **Transport errors** (Go `error` from `session.CallTool()`): retryable. These are connection drops, timeouts, stdio process crashes.
- **Tool errors** (MCP result with `IsError: true`): not retryable. These are application-level failures (file not found, permission denied) and retrying won't help.
- `retryAfter` is always 0.

## 5. Observability

Both span events and structured log lines, per the project's conventions.

**Per retry attempt:**

```go
e.log.V(1).Info("retry attempt",
    "tool", toolName,
    "handler", handlerName,
    "attempt", n,
    "maxAttempts", policy.MaxAttempts,
    "delay", delay,
    "error", err.Error())

span.AddEvent("retry.attempt", trace.WithAttributes(
    attribute.Int("attempt", n),
    attribute.String("delay", delay.String()),
    attribute.String("error", err.Error()),
))
```

**All attempts exhausted:**

```go
e.log.V(0).Info("retries exhausted",
    "tool", toolName,
    "handler", handlerName,
    "attempts", policy.MaxAttempts,
    "error", lastErr.Error())

span.AddEvent("retry.exhausted", trace.WithAttributes(
    attribute.Int("attempts", int(policy.MaxAttempts)),
))
```

Success after retry logs nothing extra — the existing span captures the successful result. Only retries and final failure add output.

## 6. File changes

| File | Change |
|------|--------|
| `internal/runtime/tools/retry.go` | **New.** `retryPolicy` struct, `retryWithBackoff()`, backoff/jitter calculation, policy extraction helpers (`httpRetryParams`, `grpcRetryParams`, `mcpRetryParams`) |
| `internal/runtime/tools/retry_classify.go` | **New.** `classifyHTTPResult()`, `classifyGRPCError()`, `classifyMCPError()`, `httpCallResult` type, `Retry-After` header parser |
| `internal/runtime/tools/retry_test.go` | **New.** Unit tests for retry engine |
| `internal/runtime/tools/retry_classify_test.go` | **New.** Unit tests for all three classifiers |
| `internal/runtime/tools/omnia_executor.go` | **Modified.** `executeHTTP` replaced: drops PromptKit HTTPExecutor, uses direct `http.Client.Do()`. `executeGRPC`, `executeMCP`, `executeOpenAPI` wrapped with `retryWithBackoff`. |
| `internal/runtime/tools/omnia_executor_test.go` | **Modified.** Integration tests for retry wiring per transport |

**Not changed:** No CRD, controller, config, or dashboard changes. All plumbing was completed in PR #790.

## 7. Testing strategy

### Unit tests (`retry_test.go`)

- `fn` succeeds on first attempt — no retries, no backoff
- `fn` fails twice then succeeds — verify 3 attempts, correct result returned
- Non-retryable error — returns immediately, single attempt
- All attempts exhausted — returns wrapped error with attempt count
- Backoff timing — delays increase exponentially, within ±10% jitter tolerance
- Context cancellation — parent cancelled mid-retry, no further attempts
- Per-attempt timeout — `fn` hangs, each attempt killed by timeout
- No-policy fast path — `MaxAttempts=1` calls `fn` once
- Span events emitted per retry (via test span exporter)
- Log lines emitted per retry (via log buffer)

### Classifier tests (`retry_classify_test.go`)

**HTTP:**
- Network error + `retryOnNetworkError: true` → retryable
- Network error + `retryOnNetworkError: false` → not retryable
- Status 502 in `retryOn` → retryable
- Status 400 not in `retryOn` → not retryable
- `Retry-After: 5` header + `respectRetryAfter: true` → retryable, `retryAfter = 5s`
- `Retry-After` header + `respectRetryAfter: false` → retryable, `retryAfter = 0`
- `Retry-After` with HTTP-date format → parsed correctly
- Success (2xx) → not retryable

**gRPC:**
- `UNAVAILABLE` in `retryableStatusCodes` → retryable
- `NOT_FOUND` not in list → not retryable
- Non-gRPC error (circuit breaker) → not retryable

**MCP:**
- Transport error (Go error) → retryable
- Context deadline exceeded → retryable
- Non-error (nil) → not retryable

### Executor integration tests (in `omnia_executor_test.go`)

One test per transport type:
- Configure retry policy, mock backend fails once then succeeds → tool call succeeds, retry happened (verified via span events)
- Configure retry policy, mock returns non-retryable error → no retry, single attempt

## 8. Scope boundaries

**In scope:**
- Retry loop with exponential backoff + jitter
- Per-attempt timeout from `HandlerEntry.Timeout`
- HTTP/gRPC/MCP/OpenAPI error classification
- `Retry-After` header respect for HTTP
- Retry-wraps-breaker composition for gRPC
- Replace PromptKit HTTP executor with direct `http.Client`
- OTel span events + structured debug logs

**Out of scope:**
- MCP session reconnection on transport failure (separate work — the MCP SDK may handle this)
- Circuit breaker for HTTP/MCP (currently only gRPC has one; adding to other transports is independent work)
- Rate limiting / concurrency control on retries
- Per-tool retry policy overrides (policies are per-handler, not per-tool)
