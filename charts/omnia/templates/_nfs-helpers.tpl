{{/*
Get the NFS server address.
Uses internal server service name if internal mode is enabled,
otherwise uses the external server address.
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
Uses /nfsshare for internal server, otherwise uses external path.
*/}}
{{- define "omnia.nfsPath" -}}
{{- if and .Values.nfs.server.enabled .Values.nfs.server.internal -}}
/nfsshare
{{- else -}}
{{ .Values.nfs.external.path }}
{{- end -}}
{{- end -}}
