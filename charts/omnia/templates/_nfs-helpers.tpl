{{/*
Get the NFS server address.
Uses internal server service DNS name if internal mode is enabled,
otherwise uses the external server address.
CSI driver pods can resolve cluster DNS, so DNS names work.
*/}}
{{- define "omnia.nfsServer" -}}
{{- if and .Values.nfs.server.enabled .Values.nfs.server.internal -}}
{{ include "omnia.fullname" . }}-nfs-server.{{ .Release.Namespace }}.svc.cluster.local
{{- else -}}
{{ .Values.nfs.external.server }}
{{- end -}}
{{- end -}}

{{/*
Get the NFS export path.
Uses / for internal server (NFSv4 pseudo-root with fsid=0), otherwise uses external path.
*/}}
{{- define "omnia.nfsPath" -}}
{{- if and .Values.nfs.server.enabled .Values.nfs.server.internal -}}
/
{{- else -}}
{{ .Values.nfs.external.path }}
{{- end -}}
{{- end -}}

{{/*
Get the NFS storage class name.
Returns the configured storage class name for NFS-backed PVCs.
*/}}
{{- define "omnia.nfsStorageClass" -}}
{{ .Values.nfs.csiDriver.storageClass.name | default "omnia-nfs" }}
{{- end -}}
