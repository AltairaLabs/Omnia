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
Dashboard service account name
*/}}
{{- define "omnia.dashboard.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "omnia.dashboard.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}
