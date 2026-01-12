{{/*
Check for existing KEDA installation when keda.enabled is true.
This prevents CRD ownership conflicts when KEDA is already installed.
*/}}
{{- define "omnia.kedaCheck" -}}
{{- if .Values.keda.enabled -}}
{{- $existingKeda := lookup "apiextensions.k8s.io/v1" "CustomResourceDefinition" "" "scaledobjects.keda.sh" -}}
{{- if $existingKeda -}}
{{- fail `
================================================================================
ERROR: KEDA is already installed in this cluster!
================================================================================

The KEDA CRD 'scaledobjects.keda.sh' already exists, which means KEDA is
installed separately (e.g., via 'helm install keda kedacore/keda').

Enabling the KEDA subchart (keda.enabled=true) would cause a CRD ownership
conflict and fail the installation.

SOLUTION: Set 'keda.enabled=false' in your values file or command line:

  helm install omnia ./charts/omnia --set keda.enabled=false

Your existing KEDA installation will work with Omnia's ScaledObject resources.

================================================================================
` -}}
{{- end -}}
{{- end -}}
{{- end -}}
