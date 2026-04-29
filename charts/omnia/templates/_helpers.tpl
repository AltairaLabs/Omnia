{{/*
Auth-mode guardrails — emits nothing but fails `helm template|install|upgrade`
when the deployment would silently run unauthenticated.

Rules:
  - dashboard.auth.mode MUST be explicitly set when dashboard.enabled=true.
    (The chart no longer has a safe default — you have to choose.)
  - Only oauth / builtin / proxy / anonymous are accepted.
  - mode=anonymous additionally requires dashboard.auth.allowAnonymous=true
    as an explicit acknowledgement. Anonymous mode disables authentication
    entirely and is intended for isolated development only.

Include this from every template that renders when dashboard.enabled=true
so the render aborts early on misconfiguration.
*/}}
{{- define "omnia.validateAuth" -}}
{{- if .Values.dashboard.enabled -}}
{{- $mode := required "dashboard.auth.mode must be set explicitly to one of: oauth, builtin, proxy, anonymous" .Values.dashboard.auth.mode -}}
{{- $validModes := list "oauth" "builtin" "proxy" "anonymous" -}}
{{- if not (has $mode $validModes) -}}
{{- fail (printf "dashboard.auth.mode=%q is not valid. Must be one of: oauth, builtin, proxy, anonymous" $mode) -}}
{{- end -}}
{{- if eq $mode "anonymous" -}}
{{- if not .Values.dashboard.auth.allowAnonymous -}}
{{- fail "dashboard.auth.mode=\"anonymous\" disables authentication entirely. Set dashboard.auth.allowAnonymous=true to acknowledge this is intentional. Anonymous mode is intended for isolated development only." -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Expand the name of the chart.
*/}}
{{- define "omnia.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "omnia.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "omnia.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "omnia.labels" -}}
helm.sh/chart: {{ include "omnia.chart" . }}
{{ include "omnia.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "omnia.selectorLabels" -}}
app.kubernetes.io/name: {{ include "omnia.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
control-plane: controller-manager
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "omnia.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "omnia.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Manager image
*/}}
{{- define "omnia.image" -}}
{{- $tag := default .Chart.AppVersion .Values.image.tag }}
{{- printf "%s:%s" .Values.image.repository $tag }}
{{- end }}

{{/*
Dashboard fullname
*/}}
{{- define "omnia.dashboard.fullname" -}}
{{- printf "%s-dashboard" (include "omnia.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Dashboard labels
*/}}
{{- define "omnia.dashboard.labels" -}}
helm.sh/chart: {{ include "omnia.chart" . }}
{{ include "omnia.dashboard.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Dashboard selector labels
*/}}
{{- define "omnia.dashboard.selectorLabels" -}}
app.kubernetes.io/name: {{ include "omnia.name" . }}-dashboard
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: dashboard
{{- end }}

{{/*
Dashboard image
*/}}
{{- define "omnia.dashboard.image" -}}
{{- $tag := default .Chart.AppVersion .Values.dashboard.image.tag }}
{{- printf "%s:%s" .Values.dashboard.image.repository $tag }}
{{- end }}

{{/*
Dashboard service account name.

The dashboard has its own ServiceAccount bound to a narrower ClusterRole
(omnia-dashboard-role, see templates/dashboard/clusterrole.yaml) so that a
dashboard compromise cannot escalate via the operator's manager ClusterRole.

Lookup order:
  1. .Values.dashboard.serviceAccount.name (explicit override)
  2. Computed: "<release>-dashboard"
  3. "default" when dashboard.serviceAccount.create=false and no name set.
*/}}
{{- define "omnia.dashboard.serviceAccountName" -}}
{{- if .Values.dashboard.serviceAccount.create }}
{{- default (include "omnia.dashboard.fullname" .) .Values.dashboard.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.dashboard.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Compaction image
*/}}
{{- define "omnia.compaction.image" -}}
{{- $tag := default .Chart.AppVersion .Values.sessionRetention.compaction.image.tag }}
{{- printf "%s:%s" .Values.sessionRetention.compaction.image.repository $tag }}
{{- end }}

{{/*
Session API fullname
*/}}
{{- define "omnia.sessionApi.fullname" -}}
{{- printf "%s-session-api" (include "omnia.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Session API labels
*/}}
{{- define "omnia.sessionApi.labels" -}}
helm.sh/chart: {{ include "omnia.chart" . }}
{{ include "omnia.sessionApi.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Session API selector labels
*/}}
{{- define "omnia.sessionApi.selectorLabels" -}}
app.kubernetes.io/name: {{ include "omnia.name" . }}-session-api
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: session-api
{{- end }}

{{/*
Session API image
*/}}
{{- define "omnia.sessionApi.image" -}}
{{- $tag := default .Chart.AppVersion .Values.workspaceServices.sessionApi.image.tag }}
{{- printf "%s:%s" .Values.workspaceServices.sessionApi.image.repository $tag }}
{{- end }}

{{/*
Memory API fullname
*/}}
{{- define "omnia.memoryApi.fullname" -}}
{{- printf "%s-memory-api" (include "omnia.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Memory API labels
*/}}
{{- define "omnia.memoryApi.labels" -}}
helm.sh/chart: {{ include "omnia.chart" . }}
{{ include "omnia.memoryApi.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Memory API selector labels
*/}}
{{- define "omnia.memoryApi.selectorLabels" -}}
app.kubernetes.io/name: {{ include "omnia.name" . }}-memory-api
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: memory-api
{{- end }}

{{/*
Memory API image
*/}}
{{- define "omnia.memoryApi.image" -}}
{{- $tag := default .Chart.AppVersion .Values.workspaceServices.memoryApi.image.tag }}
{{- printf "%s:%s" .Values.workspaceServices.memoryApi.image.repository $tag }}
{{- end }}

{{/*
Doctor fullname
*/}}
{{- define "omnia.doctor.fullname" -}}
{{- printf "%s-doctor" (include "omnia.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Doctor labels
*/}}
{{- define "omnia.doctor.labels" -}}
helm.sh/chart: {{ include "omnia.chart" . }}
{{ include "omnia.doctor.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Doctor selector labels
*/}}
{{- define "omnia.doctor.selectorLabels" -}}
app.kubernetes.io/name: {{ include "omnia.name" . }}-doctor
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: doctor
{{- end }}

{{/*
Doctor image
*/}}
{{- define "omnia.doctor.image" -}}
{{- $tag := default .Chart.AppVersion .Values.doctor.image.tag }}
{{- printf "%s:%s" .Values.doctor.image.repository $tag }}
{{- end }}

{{/*
Eval Worker fullname
*/}}
{{- define "omnia.evalWorker.fullname" -}}
{{- printf "%s-eval-worker" (include "omnia.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Eval Worker labels
*/}}
{{- define "omnia.evalWorker.labels" -}}
helm.sh/chart: {{ include "omnia.chart" . }}
{{ include "omnia.evalWorker.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Eval Worker selector labels
*/}}
{{- define "omnia.evalWorker.selectorLabels" -}}
app.kubernetes.io/name: {{ include "omnia.name" . }}-eval-worker
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: eval-worker
{{- end }}

{{/*
Eval Worker image
*/}}
{{- define "omnia.evalWorker.image" -}}
{{- $tag := default .Chart.AppVersion .Values.enterprise.evalWorker.image.tag }}
{{- printf "%s:%s" .Values.enterprise.evalWorker.image.repository $tag }}
{{- end }}

{{/*
Walk a dotted path into .Values and return the resolved sub-value, or
an empty dict if any segment is missing. Internal helper.
*/}}
{{- define "omnia.redis._lookupConsumer" -}}
{{- $ctx := .ctx -}}
{{- $path := .consumer -}}
{{- $node := $ctx.Values -}}
{{- if $path -}}
  {{- range $part := splitList "." $path -}}
    {{- if and (kindIs "map" $node) (hasKey $node $part) -}}
      {{- $node = index $node $part -}}
    {{- else -}}
      {{- $node = dict -}}
    {{- end -}}
  {{- end -}}
{{- end -}}
{{- toYaml $node -}}
{{- end }}

{{/*
Resolve the existingSecret reference for a given consumer's Redis URL.

Args (dict):
  ctx      - the root context ($)
  consumer - dotted path to the per-consumer Redis block.

Returns the resolved {name, key} dict from consumer.existingSecret
(preferred) or redis.default.existingSecret (fallback). Returns an
empty dict when neither has one. Internal helper for the public
omnia.redis.* family.
*/}}
{{- define "omnia.redis._existingSecret" -}}
{{- $consumer := fromYaml (include "omnia.redis._lookupConsumer" .) -}}
{{- $default := .ctx.Values.redis.default | default dict -}}
{{- $secret := dict -}}
{{- if and (kindIs "map" $consumer) (kindIs "map" $consumer.existingSecret) (default "" $consumer.existingSecret.name) (default "" $consumer.existingSecret.key) -}}
  {{- $secret = $consumer.existingSecret -}}
{{- else if and (kindIs "map" $default.existingSecret) (default "" $default.existingSecret.name) (default "" $default.existingSecret.key) -}}
  {{- $secret = $default.existingSecret -}}
{{- end -}}
{{- toYaml $secret -}}
{{- end }}

{{/*
Resolve the Redis URL for a given consumer.

Args (dict):
  ctx      - the root context ($)
  consumer - dotted path to the per-consumer Redis block, e.g.
             "dashboard.session.redis"
             "workspaceServices.memoryApi.cache.redis"
             "enterprise.arena.queue.redis"

Resolution order — first non-empty wins:
  1. <consumer>.existingSecret    → returns ""; caller uses
     omnia.redis.urlEnv to emit a secretKeyRef-sourced env entry.
  2. <consumer>.url               → literal URL.
  3. <consumer>.host              → decomposed; synthesise plaintext URL.
  4. redis.default.existingSecret → returns "" (same as 1).
  5. redis.default.url            → literal URL.
  6. redis.default.host           → decomposed; synthesise plaintext URL.
  7. redis.enabled=true           → Bitnami subchart in-cluster service.
  8. ""                           → consumer disabled.

`omnia.redis.url` returns the literal URL string for forms 2, 3, 5, 6, 7
or empty string for forms 1, 4, 8. To detect the existingSecret form
and emit the correct env entry, use `omnia.redis.hasSecret` and
`omnia.redis.urlEnv` (paired helper).
*/}}
{{- define "omnia.redis.url" -}}
{{- $secret := fromYaml (include "omnia.redis._existingSecret" .) -}}
{{- if and (kindIs "map" $secret) $secret.name -}}
{{- /* secret form — caller renders env via omnia.redis.urlEnv */ -}}
{{- else -}}
{{- $consumer := fromYaml (include "omnia.redis._lookupConsumer" .) -}}
{{- $default := .ctx.Values.redis.default | default dict -}}
{{- if and (kindIs "map" $consumer) $consumer.url -}}
{{- $consumer.url -}}
{{- else if and (kindIs "map" $consumer) $consumer.host -}}
{{- include "omnia.redis.synthesise" $consumer -}}
{{- else if $default.url -}}
{{- $default.url -}}
{{- else if $default.host -}}
{{- include "omnia.redis.synthesise" $default -}}
{{- else if .ctx.Values.redis.enabled -}}
{{- printf "redis://%s-redis-master.%s.svc.cluster.local:6379/0" (include "omnia.fullname" .ctx) .ctx.Release.Namespace -}}
{{- end -}}
{{- end -}}
{{- end }}

{{/*
Returns "true" when the consumer's resolved URL comes from an
existingSecret (rather than a literal URL or decomposed form);
otherwise returns "". Use to branch in templates between literal-value
and secretKeyRef-sourced EnvVar emission.
*/}}
{{- define "omnia.redis.hasSecret" -}}
{{- $secret := fromYaml (include "omnia.redis._existingSecret" .) -}}
{{- if and (kindIs "map" $secret) $secret.name -}}
true
{{- end -}}
{{- end }}

{{/*
Emit a single EnvVar entry carrying the resolved Redis URL.

Args (dict):
  ctx      - the root context ($)
  consumer - dotted path to the per-consumer Redis block.
  envName  - name of the env var to emit, e.g. "OMNIA_SESSION_REDIS_URL".

Produces nothing when the consumer's URL resolves empty (consumer
disabled). Use the alongside `omnia.validate*` helpers to fail render
when an empty URL would silently break the consumer.

Output forms:
  Literal/decomposed/subchart →
    - name: <envName>
      value: "<resolved URL>"

  existingSecret →
    - name: <envName>
      valueFrom:
        secretKeyRef:
          name: "<secret name>"
          key:  "<secret key>"

Pod templates use this in their env: list:

  env:
    {{- include "omnia.redis.urlEnv" (dict "ctx" . "consumer" "..." "envName" "...") | nindent 12 }}
*/}}
{{- define "omnia.redis.urlEnv" -}}
{{- $secret := fromYaml (include "omnia.redis._existingSecret" .) -}}
{{- if and (kindIs "map" $secret) $secret.name -}}
- name: {{ .envName }}
  valueFrom:
    secretKeyRef:
      name: {{ $secret.name | quote }}
      key: {{ $secret.key | quote }}
{{- else -}}
{{- $url := include "omnia.redis.url" . -}}
{{- if $url -}}
- name: {{ .envName }}
  value: {{ $url | quote }}
{{- end -}}
{{- end -}}
{{- end }}

{{/*
Synthesise a plaintext Redis URL from a decomposed config block.

Block shape:
  host: required
  port: default 6379
  db:   default 0
  user: default "" (Redis ACL default user)
  tls.caExistingSecret: ignored here (CA is mounted as a file by
    consumer templates; the URL just uses redis:// vs rediss://
    based on the scheme implied by tls).

Output: "redis://[user@]host:port/db".

The decomposed form intentionally cannot synthesise auth (no password
field). Callers wanting auth use the `existingSecret` form instead.
*/}}
{{- define "omnia.redis.synthesise" -}}
{{- $port := default 6379 .port -}}
{{- $db := default 0 .db -}}
{{- $user := default "" .user -}}
{{- if $user -}}
{{- printf "redis://%s@%s:%v/%v" $user .host $port $db -}}
{{- else -}}
{{- printf "redis://%s:%v/%v" .host $port $db -}}
{{- end -}}
{{- end }}

{{/*
Render-time guard: dashboard.replicaCount > 1 with no resolved session
Redis means per-pod in-memory sessions. Users get logged out the moment
their request hits a different replica. Fail render rather than ship a
silently broken multi-replica deployment.

Include this from every template that renders when dashboard.enabled=true.
*/}}
{{- define "omnia.validateSession" -}}
{{- if .Values.dashboard.enabled -}}
{{- $replicas := int (default 1 .Values.dashboard.replicaCount) -}}
{{- if gt $replicas 1 -}}
{{- $args := dict "ctx" . "consumer" "dashboard.session.redis" -}}
{{- $sessionURL := include "omnia.redis.url" $args -}}
{{- $hasSecret := include "omnia.redis.hasSecret" $args -}}
{{- if and (not $sessionURL) (not $hasSecret) -}}
{{- fail "dashboard.replicaCount > 1 requires a resolvable Redis session store. Set redis.default, dashboard.session.redis, redis.enabled=true, or scale to dashboard.replicaCount=1." -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- end }}

{{/*
Render-time guard: enterprise Arena queue requires durable Redis when
type=redis (which is the default). The in-memory queue mode is only
useful in dev / E2E; production Arena needs jobs to survive controller
restarts.
*/}}
{{- define "omnia.validateArenaQueue" -}}
{{- if and .Values.enterprise.enabled (eq (default "redis" .Values.enterprise.arena.queue.type) "redis") -}}
{{- $args := dict "ctx" . "consumer" "enterprise.arena.queue.redis" -}}
{{- $arenaURL := include "omnia.redis.url" $args -}}
{{- $hasSecret := include "omnia.redis.hasSecret" $args -}}
{{- if and (not $arenaURL) (not $hasSecret) -}}
{{- fail "enterprise.arena.queue.type=redis requires a resolvable Redis URL. Set redis.default, enterprise.arena.queue.redis, redis.enabled=true, or set enterprise.arena.queue.type=memory (dev only)." -}}
{{- end -}}
{{- end -}}
{{- end }}

{{/*
Resolve OTLP tracing endpoint for runtime/facade containers.
Priority:
1. .Values.tracing.endpoint (explicit override)
2. Alloy service (when alloy.enabled=true)
3. Tempo service (when tempo.enabled=true)
Returns empty string when no endpoint can be resolved.
*/}}
{{- define "omnia.tracing.endpoint" -}}
{{- if .Values.tracing.endpoint -}}
{{- .Values.tracing.endpoint -}}
{{- else if .Values.alloy.enabled -}}
{{- printf "%s-alloy:4317" .Release.Name -}}
{{- else if .Values.tempo.enabled -}}
{{- printf "%s-tempo:4317" .Release.Name -}}
{{- else -}}
{{- "" -}}
{{- end -}}
{{- end }}
