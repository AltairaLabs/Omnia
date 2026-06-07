{{/*
Expand the name of the chart.
*/}}
{{- define "omnia-demos.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "omnia-demos.fullname" -}}
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
{{- define "omnia-demos.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "omnia-demos.labels" -}}
helm.sh/chart: {{ include "omnia-demos.chart" . }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Render a SharePoint-hero chat Provider .spec body. Switches between keyless
Azure OpenAI (workload identity, no secret) and a direct vendor credential
(secretRef) on .Values.sharepointHero.azure.enabled.

Call with a dict: (dict "ctx" $ "type" <type> "model" <model> "secretRef" <name>)
*/}}
{{- define "omnia-demos.chatProviderSpec" -}}
{{- $az := .ctx.Values.sharepointHero.azure -}}
{{- if $az.enabled -}}
type: openai
role: llm
model: {{ $az.chatModel }}
platform:
  type: azure
  endpoint: {{ $az.endpoint | quote }}
  region: {{ $az.region | quote }}
auth:
  type: workloadIdentity
{{- else -}}
type: {{ .type }}
role: llm
model: {{ .model }}
credential:
  secretRef:
    name: {{ .secretRef }}
{{- end }}
{{- end -}}

{{/*
Pod overrides that bind a SharePoint-hero pod to the Azure workload-identity
ServiceAccount + opt it into the AKS webhook's token injection. Renders nothing
when azure.enabled=false. Indent at the call site.

Call with the root context ($).
*/}}
{{- define "omnia-demos.azureWorkloadIdentityPodOverrides" -}}
{{- $az := .Values.sharepointHero.azure -}}
{{- if $az.enabled -}}
serviceAccountName: {{ $az.workloadIdentity.serviceAccountName }}
labels:
  azure.workload.identity/use: "true"
{{- end }}
{{- end -}}
