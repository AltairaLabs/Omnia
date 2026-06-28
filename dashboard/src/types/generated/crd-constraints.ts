// Auto-generated from CRD OpenAPI schemas (issue #1612).
// Do not edit manually - run 'make generate-dashboard-types' to regenerate.

import type { FieldConstraint } from "@/lib/validation/constraint-types";

export const crdConstraints: Record<string, Record<string, FieldConstraint>> =
  {
  "AgentRuntime": {
    "spec.console.allowedAttachmentTypes[]": {
      "type": "string"
    },
    "spec.console.allowedExtensions[]": {
      "type": "string"
    },
    "spec.console.maxFileSize": {
      "type": "integer",
      "minimum": 1
    },
    "spec.console.maxFiles": {
      "type": "integer",
      "minimum": 1,
      "maximum": 20
    },
    "spec.console.mediaRequirements.audio.channels": {
      "type": "integer"
    },
    "spec.console.mediaRequirements.audio.format": {
      "type": "string"
    },
    "spec.console.mediaRequirements.audio.maxDurationSeconds": {
      "type": "integer",
      "minimum": 1
    },
    "spec.console.mediaRequirements.audio.recommendedSampleRate": {
      "type": "integer",
      "minimum": 1
    },
    "spec.console.mediaRequirements.audio.supportsSegmentSelection": {
      "type": "boolean"
    },
    "spec.console.mediaRequirements.document.maxPages": {
      "type": "integer",
      "minimum": 1
    },
    "spec.console.mediaRequirements.document.supportsOCR": {
      "type": "boolean"
    },
    "spec.console.mediaRequirements.image.compressionGuidance": {
      "type": "string",
      "enum": [
        "none",
        "lossless",
        "lossy-high",
        "lossy-medium",
        "lossy-low"
      ]
    },
    "spec.console.mediaRequirements.image.maxDimensions.height": {
      "type": "integer",
      "minimum": 1,
      "required": true
    },
    "spec.console.mediaRequirements.image.maxDimensions.width": {
      "type": "integer",
      "minimum": 1,
      "required": true
    },
    "spec.console.mediaRequirements.image.maxSizeBytes": {
      "type": "integer",
      "minimum": 1
    },
    "spec.console.mediaRequirements.image.preferredFormat": {
      "type": "string"
    },
    "spec.console.mediaRequirements.image.recommendedDimensions.height": {
      "type": "integer",
      "minimum": 1,
      "required": true
    },
    "spec.console.mediaRequirements.image.recommendedDimensions.width": {
      "type": "integer",
      "minimum": 1,
      "required": true
    },
    "spec.console.mediaRequirements.image.supportedFormats[]": {
      "type": "string"
    },
    "spec.console.mediaRequirements.video.frameExtractionInterval": {
      "type": "integer",
      "minimum": 1
    },
    "spec.console.mediaRequirements.video.maxDurationSeconds": {
      "type": "integer",
      "minimum": 1
    },
    "spec.console.mediaRequirements.video.processingMode": {
      "type": "string",
      "enum": [
        "frames",
        "transcription",
        "both",
        "native"
      ]
    },
    "spec.console.mediaRequirements.video.supportsSegmentSelection": {
      "type": "boolean"
    },
    "spec.context.storeRef.name": {
      "type": "string"
    },
    "spec.context.ttl": {
      "type": "string"
    },
    "spec.context.type": {
      "type": "string",
      "enum": [
        "memory",
        "redis"
      ],
      "required": true
    },
    "spec.duplex.enabled": {
      "type": "boolean"
    },
    "spec.duplex.mode": {
      "type": "string",
      "enum": [
        "audio",
        "audiovideo"
      ]
    },
    "spec.evals.enabled": {
      "type": "boolean"
    },
    "spec.evals.inline.groups[]": {
      "type": "string"
    },
    "spec.evals.podOverrides.affinity.nodeAffinity.preferredDuringSchedulingIgnoredDuringExecution[].preference": {
      "required": true
    },
    "spec.evals.podOverrides.affinity.nodeAffinity.preferredDuringSchedulingIgnoredDuringExecution[].preference.matchExpressions[].key": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.affinity.nodeAffinity.preferredDuringSchedulingIgnoredDuringExecution[].preference.matchExpressions[].operator": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.affinity.nodeAffinity.preferredDuringSchedulingIgnoredDuringExecution[].preference.matchExpressions[].values[]": {
      "type": "string"
    },
    "spec.evals.podOverrides.affinity.nodeAffinity.preferredDuringSchedulingIgnoredDuringExecution[].preference.matchFields[].key": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.affinity.nodeAffinity.preferredDuringSchedulingIgnoredDuringExecution[].preference.matchFields[].operator": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.affinity.nodeAffinity.preferredDuringSchedulingIgnoredDuringExecution[].preference.matchFields[].values[]": {
      "type": "string"
    },
    "spec.evals.podOverrides.affinity.nodeAffinity.preferredDuringSchedulingIgnoredDuringExecution[].weight": {
      "type": "integer",
      "required": true
    },
    "spec.evals.podOverrides.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms": {
      "required": true
    },
    "spec.evals.podOverrides.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms[].matchExpressions[].key": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms[].matchExpressions[].operator": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms[].matchExpressions[].values[]": {
      "type": "string"
    },
    "spec.evals.podOverrides.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms[].matchFields[].key": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms[].matchFields[].operator": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms[].matchFields[].values[]": {
      "type": "string"
    },
    "spec.evals.podOverrides.affinity.podAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm": {
      "required": true
    },
    "spec.evals.podOverrides.affinity.podAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm.labelSelector.matchExpressions[].key": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.affinity.podAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm.labelSelector.matchExpressions[].operator": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.affinity.podAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm.labelSelector.matchExpressions[].values[]": {
      "type": "string"
    },
    "spec.evals.podOverrides.affinity.podAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm.matchLabelKeys[]": {
      "type": "string"
    },
    "spec.evals.podOverrides.affinity.podAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm.mismatchLabelKeys[]": {
      "type": "string"
    },
    "spec.evals.podOverrides.affinity.podAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm.namespaceSelector.matchExpressions[].key": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.affinity.podAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm.namespaceSelector.matchExpressions[].operator": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.affinity.podAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm.namespaceSelector.matchExpressions[].values[]": {
      "type": "string"
    },
    "spec.evals.podOverrides.affinity.podAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm.namespaces[]": {
      "type": "string"
    },
    "spec.evals.podOverrides.affinity.podAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm.topologyKey": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.affinity.podAffinity.preferredDuringSchedulingIgnoredDuringExecution[].weight": {
      "type": "integer",
      "required": true
    },
    "spec.evals.podOverrides.affinity.podAffinity.requiredDuringSchedulingIgnoredDuringExecution[].labelSelector.matchExpressions[].key": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.affinity.podAffinity.requiredDuringSchedulingIgnoredDuringExecution[].labelSelector.matchExpressions[].operator": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.affinity.podAffinity.requiredDuringSchedulingIgnoredDuringExecution[].labelSelector.matchExpressions[].values[]": {
      "type": "string"
    },
    "spec.evals.podOverrides.affinity.podAffinity.requiredDuringSchedulingIgnoredDuringExecution[].matchLabelKeys[]": {
      "type": "string"
    },
    "spec.evals.podOverrides.affinity.podAffinity.requiredDuringSchedulingIgnoredDuringExecution[].mismatchLabelKeys[]": {
      "type": "string"
    },
    "spec.evals.podOverrides.affinity.podAffinity.requiredDuringSchedulingIgnoredDuringExecution[].namespaceSelector.matchExpressions[].key": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.affinity.podAffinity.requiredDuringSchedulingIgnoredDuringExecution[].namespaceSelector.matchExpressions[].operator": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.affinity.podAffinity.requiredDuringSchedulingIgnoredDuringExecution[].namespaceSelector.matchExpressions[].values[]": {
      "type": "string"
    },
    "spec.evals.podOverrides.affinity.podAffinity.requiredDuringSchedulingIgnoredDuringExecution[].namespaces[]": {
      "type": "string"
    },
    "spec.evals.podOverrides.affinity.podAffinity.requiredDuringSchedulingIgnoredDuringExecution[].topologyKey": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm": {
      "required": true
    },
    "spec.evals.podOverrides.affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm.labelSelector.matchExpressions[].key": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm.labelSelector.matchExpressions[].operator": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm.labelSelector.matchExpressions[].values[]": {
      "type": "string"
    },
    "spec.evals.podOverrides.affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm.matchLabelKeys[]": {
      "type": "string"
    },
    "spec.evals.podOverrides.affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm.mismatchLabelKeys[]": {
      "type": "string"
    },
    "spec.evals.podOverrides.affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm.namespaceSelector.matchExpressions[].key": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm.namespaceSelector.matchExpressions[].operator": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm.namespaceSelector.matchExpressions[].values[]": {
      "type": "string"
    },
    "spec.evals.podOverrides.affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm.namespaces[]": {
      "type": "string"
    },
    "spec.evals.podOverrides.affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm.topologyKey": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution[].weight": {
      "type": "integer",
      "required": true
    },
    "spec.evals.podOverrides.affinity.podAntiAffinity.requiredDuringSchedulingIgnoredDuringExecution[].labelSelector.matchExpressions[].key": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.affinity.podAntiAffinity.requiredDuringSchedulingIgnoredDuringExecution[].labelSelector.matchExpressions[].operator": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.affinity.podAntiAffinity.requiredDuringSchedulingIgnoredDuringExecution[].labelSelector.matchExpressions[].values[]": {
      "type": "string"
    },
    "spec.evals.podOverrides.affinity.podAntiAffinity.requiredDuringSchedulingIgnoredDuringExecution[].matchLabelKeys[]": {
      "type": "string"
    },
    "spec.evals.podOverrides.affinity.podAntiAffinity.requiredDuringSchedulingIgnoredDuringExecution[].mismatchLabelKeys[]": {
      "type": "string"
    },
    "spec.evals.podOverrides.affinity.podAntiAffinity.requiredDuringSchedulingIgnoredDuringExecution[].namespaceSelector.matchExpressions[].key": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.affinity.podAntiAffinity.requiredDuringSchedulingIgnoredDuringExecution[].namespaceSelector.matchExpressions[].operator": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.affinity.podAntiAffinity.requiredDuringSchedulingIgnoredDuringExecution[].namespaceSelector.matchExpressions[].values[]": {
      "type": "string"
    },
    "spec.evals.podOverrides.affinity.podAntiAffinity.requiredDuringSchedulingIgnoredDuringExecution[].namespaces[]": {
      "type": "string"
    },
    "spec.evals.podOverrides.affinity.podAntiAffinity.requiredDuringSchedulingIgnoredDuringExecution[].topologyKey": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraEnv[].name": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraEnv[].value": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraEnv[].valueFrom.configMapKeyRef.key": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraEnv[].valueFrom.configMapKeyRef.name": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraEnv[].valueFrom.configMapKeyRef.optional": {
      "type": "boolean"
    },
    "spec.evals.podOverrides.extraEnv[].valueFrom.fieldRef.apiVersion": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraEnv[].valueFrom.fieldRef.fieldPath": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraEnv[].valueFrom.fileKeyRef.key": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraEnv[].valueFrom.fileKeyRef.optional": {
      "type": "boolean"
    },
    "spec.evals.podOverrides.extraEnv[].valueFrom.fileKeyRef.path": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraEnv[].valueFrom.fileKeyRef.volumeName": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraEnv[].valueFrom.resourceFieldRef.containerName": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraEnv[].valueFrom.resourceFieldRef.divisor": {
      "pattern": "^(\\+|-)?(([0-9]+(\\.[0-9]*)?)|(\\.[0-9]+))(([KMGTPE]i)|[numkMGTPE]|([eE](\\+|-)?(([0-9]+(\\.[0-9]*)?)|(\\.[0-9]+))))?$"
    },
    "spec.evals.podOverrides.extraEnv[].valueFrom.resourceFieldRef.resource": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraEnv[].valueFrom.secretKeyRef.key": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraEnv[].valueFrom.secretKeyRef.name": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraEnv[].valueFrom.secretKeyRef.optional": {
      "type": "boolean"
    },
    "spec.evals.podOverrides.extraEnvFrom[].configMapRef.name": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraEnvFrom[].configMapRef.optional": {
      "type": "boolean"
    },
    "spec.evals.podOverrides.extraEnvFrom[].prefix": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraEnvFrom[].secretRef.name": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraEnvFrom[].secretRef.optional": {
      "type": "boolean"
    },
    "spec.evals.podOverrides.extraVolumeMounts[].mountPath": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraVolumeMounts[].mountPropagation": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumeMounts[].name": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraVolumeMounts[].readOnly": {
      "type": "boolean"
    },
    "spec.evals.podOverrides.extraVolumeMounts[].recursiveReadOnly": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumeMounts[].subPath": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumeMounts[].subPathExpr": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].awsElasticBlockStore.fsType": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].awsElasticBlockStore.partition": {
      "type": "integer"
    },
    "spec.evals.podOverrides.extraVolumes[].awsElasticBlockStore.readOnly": {
      "type": "boolean"
    },
    "spec.evals.podOverrides.extraVolumes[].awsElasticBlockStore.volumeID": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraVolumes[].azureDisk.cachingMode": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].azureDisk.diskName": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraVolumes[].azureDisk.diskURI": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraVolumes[].azureDisk.fsType": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].azureDisk.kind": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].azureDisk.readOnly": {
      "type": "boolean"
    },
    "spec.evals.podOverrides.extraVolumes[].azureFile.readOnly": {
      "type": "boolean"
    },
    "spec.evals.podOverrides.extraVolumes[].azureFile.secretName": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraVolumes[].azureFile.shareName": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraVolumes[].cephfs.monitors": {
      "required": true
    },
    "spec.evals.podOverrides.extraVolumes[].cephfs.monitors[]": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].cephfs.path": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].cephfs.readOnly": {
      "type": "boolean"
    },
    "spec.evals.podOverrides.extraVolumes[].cephfs.secretFile": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].cephfs.secretRef.name": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].cephfs.user": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].cinder.fsType": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].cinder.readOnly": {
      "type": "boolean"
    },
    "spec.evals.podOverrides.extraVolumes[].cinder.secretRef.name": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].cinder.volumeID": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraVolumes[].configMap.defaultMode": {
      "type": "integer"
    },
    "spec.evals.podOverrides.extraVolumes[].configMap.items[].key": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraVolumes[].configMap.items[].mode": {
      "type": "integer"
    },
    "spec.evals.podOverrides.extraVolumes[].configMap.items[].path": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraVolumes[].configMap.name": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].configMap.optional": {
      "type": "boolean"
    },
    "spec.evals.podOverrides.extraVolumes[].csi.driver": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraVolumes[].csi.fsType": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].csi.nodePublishSecretRef.name": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].csi.readOnly": {
      "type": "boolean"
    },
    "spec.evals.podOverrides.extraVolumes[].downwardAPI.defaultMode": {
      "type": "integer"
    },
    "spec.evals.podOverrides.extraVolumes[].downwardAPI.items[].fieldRef.apiVersion": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].downwardAPI.items[].fieldRef.fieldPath": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraVolumes[].downwardAPI.items[].mode": {
      "type": "integer"
    },
    "spec.evals.podOverrides.extraVolumes[].downwardAPI.items[].path": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraVolumes[].downwardAPI.items[].resourceFieldRef.containerName": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].downwardAPI.items[].resourceFieldRef.divisor": {
      "pattern": "^(\\+|-)?(([0-9]+(\\.[0-9]*)?)|(\\.[0-9]+))(([KMGTPE]i)|[numkMGTPE]|([eE](\\+|-)?(([0-9]+(\\.[0-9]*)?)|(\\.[0-9]+))))?$"
    },
    "spec.evals.podOverrides.extraVolumes[].downwardAPI.items[].resourceFieldRef.resource": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraVolumes[].emptyDir.medium": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].emptyDir.sizeLimit": {
      "pattern": "^(\\+|-)?(([0-9]+(\\.[0-9]*)?)|(\\.[0-9]+))(([KMGTPE]i)|[numkMGTPE]|([eE](\\+|-)?(([0-9]+(\\.[0-9]*)?)|(\\.[0-9]+))))?$"
    },
    "spec.evals.podOverrides.extraVolumes[].ephemeral.volumeClaimTemplate.spec": {
      "required": true
    },
    "spec.evals.podOverrides.extraVolumes[].ephemeral.volumeClaimTemplate.spec.accessModes[]": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].ephemeral.volumeClaimTemplate.spec.dataSource.apiGroup": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].ephemeral.volumeClaimTemplate.spec.dataSource.kind": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraVolumes[].ephemeral.volumeClaimTemplate.spec.dataSource.name": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraVolumes[].ephemeral.volumeClaimTemplate.spec.dataSourceRef.apiGroup": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].ephemeral.volumeClaimTemplate.spec.dataSourceRef.kind": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraVolumes[].ephemeral.volumeClaimTemplate.spec.dataSourceRef.name": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraVolumes[].ephemeral.volumeClaimTemplate.spec.dataSourceRef.namespace": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].ephemeral.volumeClaimTemplate.spec.selector.matchExpressions[].key": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraVolumes[].ephemeral.volumeClaimTemplate.spec.selector.matchExpressions[].operator": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraVolumes[].ephemeral.volumeClaimTemplate.spec.selector.matchExpressions[].values[]": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].ephemeral.volumeClaimTemplate.spec.storageClassName": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].ephemeral.volumeClaimTemplate.spec.volumeAttributesClassName": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].ephemeral.volumeClaimTemplate.spec.volumeMode": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].ephemeral.volumeClaimTemplate.spec.volumeName": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].fc.fsType": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].fc.lun": {
      "type": "integer"
    },
    "spec.evals.podOverrides.extraVolumes[].fc.readOnly": {
      "type": "boolean"
    },
    "spec.evals.podOverrides.extraVolumes[].fc.targetWWNs[]": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].fc.wwids[]": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].flexVolume.driver": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraVolumes[].flexVolume.fsType": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].flexVolume.readOnly": {
      "type": "boolean"
    },
    "spec.evals.podOverrides.extraVolumes[].flexVolume.secretRef.name": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].flocker.datasetName": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].flocker.datasetUUID": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].gcePersistentDisk.fsType": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].gcePersistentDisk.partition": {
      "type": "integer"
    },
    "spec.evals.podOverrides.extraVolumes[].gcePersistentDisk.pdName": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraVolumes[].gcePersistentDisk.readOnly": {
      "type": "boolean"
    },
    "spec.evals.podOverrides.extraVolumes[].gitRepo.directory": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].gitRepo.repository": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraVolumes[].gitRepo.revision": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].glusterfs.endpoints": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraVolumes[].glusterfs.path": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraVolumes[].glusterfs.readOnly": {
      "type": "boolean"
    },
    "spec.evals.podOverrides.extraVolumes[].hostPath.path": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraVolumes[].hostPath.type": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].image.pullPolicy": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].image.reference": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].iscsi.chapAuthDiscovery": {
      "type": "boolean"
    },
    "spec.evals.podOverrides.extraVolumes[].iscsi.chapAuthSession": {
      "type": "boolean"
    },
    "spec.evals.podOverrides.extraVolumes[].iscsi.fsType": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].iscsi.initiatorName": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].iscsi.iqn": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraVolumes[].iscsi.iscsiInterface": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].iscsi.lun": {
      "type": "integer",
      "required": true
    },
    "spec.evals.podOverrides.extraVolumes[].iscsi.portals[]": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].iscsi.readOnly": {
      "type": "boolean"
    },
    "spec.evals.podOverrides.extraVolumes[].iscsi.secretRef.name": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].iscsi.targetPortal": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraVolumes[].name": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraVolumes[].nfs.path": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraVolumes[].nfs.readOnly": {
      "type": "boolean"
    },
    "spec.evals.podOverrides.extraVolumes[].nfs.server": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraVolumes[].persistentVolumeClaim.claimName": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraVolumes[].persistentVolumeClaim.readOnly": {
      "type": "boolean"
    },
    "spec.evals.podOverrides.extraVolumes[].photonPersistentDisk.fsType": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].photonPersistentDisk.pdID": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraVolumes[].portworxVolume.fsType": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].portworxVolume.readOnly": {
      "type": "boolean"
    },
    "spec.evals.podOverrides.extraVolumes[].portworxVolume.volumeID": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraVolumes[].projected.defaultMode": {
      "type": "integer"
    },
    "spec.evals.podOverrides.extraVolumes[].projected.sources[].clusterTrustBundle.labelSelector.matchExpressions[].key": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraVolumes[].projected.sources[].clusterTrustBundle.labelSelector.matchExpressions[].operator": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraVolumes[].projected.sources[].clusterTrustBundle.labelSelector.matchExpressions[].values[]": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].projected.sources[].clusterTrustBundle.name": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].projected.sources[].clusterTrustBundle.optional": {
      "type": "boolean"
    },
    "spec.evals.podOverrides.extraVolumes[].projected.sources[].clusterTrustBundle.path": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraVolumes[].projected.sources[].clusterTrustBundle.signerName": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].projected.sources[].configMap.items[].key": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraVolumes[].projected.sources[].configMap.items[].mode": {
      "type": "integer"
    },
    "spec.evals.podOverrides.extraVolumes[].projected.sources[].configMap.items[].path": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraVolumes[].projected.sources[].configMap.name": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].projected.sources[].configMap.optional": {
      "type": "boolean"
    },
    "spec.evals.podOverrides.extraVolumes[].projected.sources[].downwardAPI.items[].fieldRef.apiVersion": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].projected.sources[].downwardAPI.items[].fieldRef.fieldPath": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraVolumes[].projected.sources[].downwardAPI.items[].mode": {
      "type": "integer"
    },
    "spec.evals.podOverrides.extraVolumes[].projected.sources[].downwardAPI.items[].path": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraVolumes[].projected.sources[].downwardAPI.items[].resourceFieldRef.containerName": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].projected.sources[].downwardAPI.items[].resourceFieldRef.divisor": {
      "pattern": "^(\\+|-)?(([0-9]+(\\.[0-9]*)?)|(\\.[0-9]+))(([KMGTPE]i)|[numkMGTPE]|([eE](\\+|-)?(([0-9]+(\\.[0-9]*)?)|(\\.[0-9]+))))?$"
    },
    "spec.evals.podOverrides.extraVolumes[].projected.sources[].downwardAPI.items[].resourceFieldRef.resource": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraVolumes[].projected.sources[].podCertificate.certificateChainPath": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].projected.sources[].podCertificate.credentialBundlePath": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].projected.sources[].podCertificate.keyPath": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].projected.sources[].podCertificate.keyType": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraVolumes[].projected.sources[].podCertificate.maxExpirationSeconds": {
      "type": "integer"
    },
    "spec.evals.podOverrides.extraVolumes[].projected.sources[].podCertificate.signerName": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraVolumes[].projected.sources[].secret.items[].key": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraVolumes[].projected.sources[].secret.items[].mode": {
      "type": "integer"
    },
    "spec.evals.podOverrides.extraVolumes[].projected.sources[].secret.items[].path": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraVolumes[].projected.sources[].secret.name": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].projected.sources[].secret.optional": {
      "type": "boolean"
    },
    "spec.evals.podOverrides.extraVolumes[].projected.sources[].serviceAccountToken.audience": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].projected.sources[].serviceAccountToken.expirationSeconds": {
      "type": "integer"
    },
    "spec.evals.podOverrides.extraVolumes[].projected.sources[].serviceAccountToken.path": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraVolumes[].quobyte.group": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].quobyte.readOnly": {
      "type": "boolean"
    },
    "spec.evals.podOverrides.extraVolumes[].quobyte.registry": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraVolumes[].quobyte.tenant": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].quobyte.user": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].quobyte.volume": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraVolumes[].rbd.fsType": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].rbd.image": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraVolumes[].rbd.keyring": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].rbd.monitors": {
      "required": true
    },
    "spec.evals.podOverrides.extraVolumes[].rbd.monitors[]": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].rbd.pool": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].rbd.readOnly": {
      "type": "boolean"
    },
    "spec.evals.podOverrides.extraVolumes[].rbd.secretRef.name": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].rbd.user": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].scaleIO.fsType": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].scaleIO.gateway": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraVolumes[].scaleIO.protectionDomain": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].scaleIO.readOnly": {
      "type": "boolean"
    },
    "spec.evals.podOverrides.extraVolumes[].scaleIO.secretRef": {
      "required": true
    },
    "spec.evals.podOverrides.extraVolumes[].scaleIO.secretRef.name": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].scaleIO.sslEnabled": {
      "type": "boolean"
    },
    "spec.evals.podOverrides.extraVolumes[].scaleIO.storageMode": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].scaleIO.storagePool": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].scaleIO.system": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraVolumes[].scaleIO.volumeName": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].secret.defaultMode": {
      "type": "integer"
    },
    "spec.evals.podOverrides.extraVolumes[].secret.items[].key": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraVolumes[].secret.items[].mode": {
      "type": "integer"
    },
    "spec.evals.podOverrides.extraVolumes[].secret.items[].path": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.extraVolumes[].secret.optional": {
      "type": "boolean"
    },
    "spec.evals.podOverrides.extraVolumes[].secret.secretName": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].storageos.fsType": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].storageos.readOnly": {
      "type": "boolean"
    },
    "spec.evals.podOverrides.extraVolumes[].storageos.secretRef.name": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].storageos.volumeName": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].storageos.volumeNamespace": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].vsphereVolume.fsType": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].vsphereVolume.storagePolicyID": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].vsphereVolume.storagePolicyName": {
      "type": "string"
    },
    "spec.evals.podOverrides.extraVolumes[].vsphereVolume.volumePath": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.imagePullSecrets[].name": {
      "type": "string"
    },
    "spec.evals.podOverrides.priorityClassName": {
      "type": "string"
    },
    "spec.evals.podOverrides.serviceAccountName": {
      "type": "string"
    },
    "spec.evals.podOverrides.tolerations[].effect": {
      "type": "string"
    },
    "spec.evals.podOverrides.tolerations[].key": {
      "type": "string"
    },
    "spec.evals.podOverrides.tolerations[].operator": {
      "type": "string"
    },
    "spec.evals.podOverrides.tolerations[].tolerationSeconds": {
      "type": "integer"
    },
    "spec.evals.podOverrides.tolerations[].value": {
      "type": "string"
    },
    "spec.evals.podOverrides.topologySpreadConstraints[].labelSelector.matchExpressions[].key": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.topologySpreadConstraints[].labelSelector.matchExpressions[].operator": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.topologySpreadConstraints[].labelSelector.matchExpressions[].values[]": {
      "type": "string"
    },
    "spec.evals.podOverrides.topologySpreadConstraints[].matchLabelKeys[]": {
      "type": "string"
    },
    "spec.evals.podOverrides.topologySpreadConstraints[].maxSkew": {
      "type": "integer",
      "required": true
    },
    "spec.evals.podOverrides.topologySpreadConstraints[].minDomains": {
      "type": "integer"
    },
    "spec.evals.podOverrides.topologySpreadConstraints[].nodeAffinityPolicy": {
      "type": "string"
    },
    "spec.evals.podOverrides.topologySpreadConstraints[].nodeTaintsPolicy": {
      "type": "string"
    },
    "spec.evals.podOverrides.topologySpreadConstraints[].topologyKey": {
      "type": "string",
      "required": true
    },
    "spec.evals.podOverrides.topologySpreadConstraints[].whenUnsatisfiable": {
      "type": "string",
      "required": true
    },
    "spec.evals.rateLimit.maxConcurrentJudgeCalls": {
      "type": "integer",
      "minimum": 1
    },
    "spec.evals.rateLimit.maxEvalsPerSecond": {
      "type": "integer",
      "minimum": 1
    },
    "spec.evals.sampling.defaultRate": {
      "type": "integer",
      "minimum": 0,
      "maximum": 100
    },
    "spec.evals.sampling.extendedRate": {
      "type": "integer",
      "minimum": 0,
      "maximum": 100
    },
    "spec.evals.sessionCompletion.inactivityTimeout": {
      "type": "string"
    },
    "spec.evals.worker.groups[]": {
      "type": "string"
    },
    "spec.externalAuth.apiKeys.defaultRole": {
      "type": "string",
      "enum": [
        "viewer",
        "editor",
        "admin"
      ]
    },
    "spec.externalAuth.apiKeys.trustEndUserHeader": {
      "type": "boolean"
    },
    "spec.externalAuth.edgeTrust.headerMapping.email": {
      "type": "string"
    },
    "spec.externalAuth.edgeTrust.headerMapping.endUser": {
      "type": "string"
    },
    "spec.externalAuth.edgeTrust.headerMapping.role": {
      "type": "string"
    },
    "spec.externalAuth.edgeTrust.headerMapping.subject": {
      "type": "string"
    },
    "spec.externalAuth.oidc.audience": {
      "type": "string",
      "minLength": 1,
      "required": true
    },
    "spec.externalAuth.oidc.claimMapping.endUser": {
      "type": "string"
    },
    "spec.externalAuth.oidc.claimMapping.role": {
      "type": "string"
    },
    "spec.externalAuth.oidc.claimMapping.subject": {
      "type": "string"
    },
    "spec.externalAuth.oidc.issuer": {
      "type": "string",
      "minLength": 1,
      "required": true
    },
    "spec.externalAuth.sharedToken.secretRef": {
      "required": true
    },
    "spec.externalAuth.sharedToken.secretRef.name": {
      "type": "string"
    },
    "spec.externalAuth.sharedToken.trustEndUserHeader": {
      "type": "boolean"
    },
    "spec.facades": {
      "required": true
    },
    "spec.facades[].a2a.agentCard.capabilities.pushNotifications": {
      "type": "boolean"
    },
    "spec.facades[].a2a.agentCard.capabilities.streaming": {
      "type": "boolean"
    },
    "spec.facades[].a2a.agentCard.defaultInputModes[]": {
      "type": "string"
    },
    "spec.facades[].a2a.agentCard.defaultOutputModes[]": {
      "type": "string"
    },
    "spec.facades[].a2a.agentCard.description": {
      "type": "string"
    },
    "spec.facades[].a2a.agentCard.name": {
      "type": "string",
      "minLength": 1,
      "required": true
    },
    "spec.facades[].a2a.agentCard.organization": {
      "type": "string"
    },
    "spec.facades[].a2a.agentCard.skills[].description": {
      "type": "string"
    },
    "spec.facades[].a2a.agentCard.skills[].examples[]": {
      "type": "string"
    },
    "spec.facades[].a2a.agentCard.skills[].id": {
      "type": "string",
      "minLength": 1,
      "required": true
    },
    "spec.facades[].a2a.agentCard.skills[].name": {
      "type": "string",
      "minLength": 1,
      "required": true
    },
    "spec.facades[].a2a.agentCard.skills[].tags[]": {
      "type": "string"
    },
    "spec.facades[].a2a.agentCard.version": {
      "type": "string"
    },
    "spec.facades[].a2a.clients[].agentRuntimeRef.name": {
      "type": "string",
      "minLength": 1,
      "required": true
    },
    "spec.facades[].a2a.clients[].agentRuntimeRef.namespace": {
      "type": "string"
    },
    "spec.facades[].a2a.clients[].authentication.secretRef.name": {
      "type": "string"
    },
    "spec.facades[].a2a.clients[].exposeAsTools": {
      "type": "boolean"
    },
    "spec.facades[].a2a.clients[].name": {
      "type": "string",
      "minLength": 1,
      "required": true
    },
    "spec.facades[].a2a.clients[].url": {
      "type": "string"
    },
    "spec.facades[].a2a.conversationTTL": {
      "type": "string"
    },
    "spec.facades[].a2a.enabled": {
      "type": "boolean"
    },
    "spec.facades[].a2a.port": {
      "type": "integer"
    },
    "spec.facades[].a2a.taskStore.redisSecretRef.name": {
      "type": "string"
    },
    "spec.facades[].a2a.taskStore.redisURL": {
      "type": "string"
    },
    "spec.facades[].a2a.taskStore.type": {
      "type": "string"
    },
    "spec.facades[].a2a.taskTTL": {
      "type": "string"
    },
    "spec.facades[].clientToolTimeout": {
      "type": "string"
    },
    "spec.facades[].drainTimeout": {
      "type": "string"
    },
    "spec.facades[].expose.enabled": {
      "type": "boolean"
    },
    "spec.facades[].expose.host": {
      "type": "string"
    },
    "spec.facades[].extraEnv[].name": {
      "type": "string",
      "required": true
    },
    "spec.facades[].extraEnv[].value": {
      "type": "string"
    },
    "spec.facades[].extraEnv[].valueFrom.configMapKeyRef.key": {
      "type": "string",
      "required": true
    },
    "spec.facades[].extraEnv[].valueFrom.configMapKeyRef.name": {
      "type": "string"
    },
    "spec.facades[].extraEnv[].valueFrom.configMapKeyRef.optional": {
      "type": "boolean"
    },
    "spec.facades[].extraEnv[].valueFrom.fieldRef.apiVersion": {
      "type": "string"
    },
    "spec.facades[].extraEnv[].valueFrom.fieldRef.fieldPath": {
      "type": "string",
      "required": true
    },
    "spec.facades[].extraEnv[].valueFrom.fileKeyRef.key": {
      "type": "string",
      "required": true
    },
    "spec.facades[].extraEnv[].valueFrom.fileKeyRef.optional": {
      "type": "boolean"
    },
    "spec.facades[].extraEnv[].valueFrom.fileKeyRef.path": {
      "type": "string",
      "required": true
    },
    "spec.facades[].extraEnv[].valueFrom.fileKeyRef.volumeName": {
      "type": "string",
      "required": true
    },
    "spec.facades[].extraEnv[].valueFrom.resourceFieldRef.containerName": {
      "type": "string"
    },
    "spec.facades[].extraEnv[].valueFrom.resourceFieldRef.divisor": {
      "pattern": "^(\\+|-)?(([0-9]+(\\.[0-9]*)?)|(\\.[0-9]+))(([KMGTPE]i)|[numkMGTPE]|([eE](\\+|-)?(([0-9]+(\\.[0-9]*)?)|(\\.[0-9]+))))?$"
    },
    "spec.facades[].extraEnv[].valueFrom.resourceFieldRef.resource": {
      "type": "string",
      "required": true
    },
    "spec.facades[].extraEnv[].valueFrom.secretKeyRef.key": {
      "type": "string",
      "required": true
    },
    "spec.facades[].extraEnv[].valueFrom.secretKeyRef.name": {
      "type": "string"
    },
    "spec.facades[].extraEnv[].valueFrom.secretKeyRef.optional": {
      "type": "boolean"
    },
    "spec.facades[].handler": {
      "type": "string",
      "enum": [
        "echo",
        "demo",
        "runtime"
      ]
    },
    "spec.facades[].image": {
      "type": "string"
    },
    "spec.facades[].managementPlane": {
      "type": "boolean"
    },
    "spec.facades[].mcp.enabled": {
      "type": "boolean"
    },
    "spec.facades[].mcp.port": {
      "type": "integer",
      "minimum": 1,
      "maximum": 65535
    },
    "spec.facades[].port": {
      "type": "integer",
      "minimum": 1,
      "maximum": 65535
    },
    "spec.facades[].type": {
      "type": "string",
      "enum": [
        "websocket",
        "a2a",
        "rest",
        "mcp"
      ],
      "required": true
    },
    "spec.framework.image": {
      "type": "string"
    },
    "spec.framework.type": {
      "type": "string",
      "enum": [
        "promptkit",
        "langchain",
        "autogen",
        "custom"
      ],
      "required": true
    },
    "spec.framework.version": {
      "type": "string"
    },
    "spec.media.basePath": {
      "type": "string"
    },
    "spec.memory.enabled": {
      "type": "boolean"
    },
    "spec.memory.retrieval.accessFilter.denyCEL": {
      "type": "string"
    },
    "spec.memory.retrieval.enabled": {
      "type": "boolean"
    },
    "spec.memory.retrieval.limit": {
      "type": "integer",
      "minimum": 1,
      "maximum": 50
    },
    "spec.memory.retrieval.strategy": {
      "type": "string",
      "enum": [
        "keyword",
        "semantic",
        "composite"
      ]
    },
    "spec.memory.tools.enabled": {
      "type": "boolean"
    },
    "spec.mode": {
      "type": "string",
      "enum": [
        "agent",
        "function"
      ]
    },
    "spec.outputFormat": {
      "type": "string",
      "enum": [
        "text",
        "json",
        "json_schema"
      ]
    },
    "spec.podOverrides.affinity.nodeAffinity.preferredDuringSchedulingIgnoredDuringExecution[].preference": {
      "required": true
    },
    "spec.podOverrides.affinity.nodeAffinity.preferredDuringSchedulingIgnoredDuringExecution[].preference.matchExpressions[].key": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.affinity.nodeAffinity.preferredDuringSchedulingIgnoredDuringExecution[].preference.matchExpressions[].operator": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.affinity.nodeAffinity.preferredDuringSchedulingIgnoredDuringExecution[].preference.matchExpressions[].values[]": {
      "type": "string"
    },
    "spec.podOverrides.affinity.nodeAffinity.preferredDuringSchedulingIgnoredDuringExecution[].preference.matchFields[].key": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.affinity.nodeAffinity.preferredDuringSchedulingIgnoredDuringExecution[].preference.matchFields[].operator": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.affinity.nodeAffinity.preferredDuringSchedulingIgnoredDuringExecution[].preference.matchFields[].values[]": {
      "type": "string"
    },
    "spec.podOverrides.affinity.nodeAffinity.preferredDuringSchedulingIgnoredDuringExecution[].weight": {
      "type": "integer",
      "required": true
    },
    "spec.podOverrides.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms": {
      "required": true
    },
    "spec.podOverrides.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms[].matchExpressions[].key": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms[].matchExpressions[].operator": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms[].matchExpressions[].values[]": {
      "type": "string"
    },
    "spec.podOverrides.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms[].matchFields[].key": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms[].matchFields[].operator": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms[].matchFields[].values[]": {
      "type": "string"
    },
    "spec.podOverrides.affinity.podAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm": {
      "required": true
    },
    "spec.podOverrides.affinity.podAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm.labelSelector.matchExpressions[].key": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.affinity.podAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm.labelSelector.matchExpressions[].operator": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.affinity.podAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm.labelSelector.matchExpressions[].values[]": {
      "type": "string"
    },
    "spec.podOverrides.affinity.podAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm.matchLabelKeys[]": {
      "type": "string"
    },
    "spec.podOverrides.affinity.podAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm.mismatchLabelKeys[]": {
      "type": "string"
    },
    "spec.podOverrides.affinity.podAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm.namespaceSelector.matchExpressions[].key": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.affinity.podAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm.namespaceSelector.matchExpressions[].operator": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.affinity.podAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm.namespaceSelector.matchExpressions[].values[]": {
      "type": "string"
    },
    "spec.podOverrides.affinity.podAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm.namespaces[]": {
      "type": "string"
    },
    "spec.podOverrides.affinity.podAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm.topologyKey": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.affinity.podAffinity.preferredDuringSchedulingIgnoredDuringExecution[].weight": {
      "type": "integer",
      "required": true
    },
    "spec.podOverrides.affinity.podAffinity.requiredDuringSchedulingIgnoredDuringExecution[].labelSelector.matchExpressions[].key": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.affinity.podAffinity.requiredDuringSchedulingIgnoredDuringExecution[].labelSelector.matchExpressions[].operator": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.affinity.podAffinity.requiredDuringSchedulingIgnoredDuringExecution[].labelSelector.matchExpressions[].values[]": {
      "type": "string"
    },
    "spec.podOverrides.affinity.podAffinity.requiredDuringSchedulingIgnoredDuringExecution[].matchLabelKeys[]": {
      "type": "string"
    },
    "spec.podOverrides.affinity.podAffinity.requiredDuringSchedulingIgnoredDuringExecution[].mismatchLabelKeys[]": {
      "type": "string"
    },
    "spec.podOverrides.affinity.podAffinity.requiredDuringSchedulingIgnoredDuringExecution[].namespaceSelector.matchExpressions[].key": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.affinity.podAffinity.requiredDuringSchedulingIgnoredDuringExecution[].namespaceSelector.matchExpressions[].operator": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.affinity.podAffinity.requiredDuringSchedulingIgnoredDuringExecution[].namespaceSelector.matchExpressions[].values[]": {
      "type": "string"
    },
    "spec.podOverrides.affinity.podAffinity.requiredDuringSchedulingIgnoredDuringExecution[].namespaces[]": {
      "type": "string"
    },
    "spec.podOverrides.affinity.podAffinity.requiredDuringSchedulingIgnoredDuringExecution[].topologyKey": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm": {
      "required": true
    },
    "spec.podOverrides.affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm.labelSelector.matchExpressions[].key": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm.labelSelector.matchExpressions[].operator": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm.labelSelector.matchExpressions[].values[]": {
      "type": "string"
    },
    "spec.podOverrides.affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm.matchLabelKeys[]": {
      "type": "string"
    },
    "spec.podOverrides.affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm.mismatchLabelKeys[]": {
      "type": "string"
    },
    "spec.podOverrides.affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm.namespaceSelector.matchExpressions[].key": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm.namespaceSelector.matchExpressions[].operator": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm.namespaceSelector.matchExpressions[].values[]": {
      "type": "string"
    },
    "spec.podOverrides.affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm.namespaces[]": {
      "type": "string"
    },
    "spec.podOverrides.affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm.topologyKey": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution[].weight": {
      "type": "integer",
      "required": true
    },
    "spec.podOverrides.affinity.podAntiAffinity.requiredDuringSchedulingIgnoredDuringExecution[].labelSelector.matchExpressions[].key": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.affinity.podAntiAffinity.requiredDuringSchedulingIgnoredDuringExecution[].labelSelector.matchExpressions[].operator": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.affinity.podAntiAffinity.requiredDuringSchedulingIgnoredDuringExecution[].labelSelector.matchExpressions[].values[]": {
      "type": "string"
    },
    "spec.podOverrides.affinity.podAntiAffinity.requiredDuringSchedulingIgnoredDuringExecution[].matchLabelKeys[]": {
      "type": "string"
    },
    "spec.podOverrides.affinity.podAntiAffinity.requiredDuringSchedulingIgnoredDuringExecution[].mismatchLabelKeys[]": {
      "type": "string"
    },
    "spec.podOverrides.affinity.podAntiAffinity.requiredDuringSchedulingIgnoredDuringExecution[].namespaceSelector.matchExpressions[].key": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.affinity.podAntiAffinity.requiredDuringSchedulingIgnoredDuringExecution[].namespaceSelector.matchExpressions[].operator": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.affinity.podAntiAffinity.requiredDuringSchedulingIgnoredDuringExecution[].namespaceSelector.matchExpressions[].values[]": {
      "type": "string"
    },
    "spec.podOverrides.affinity.podAntiAffinity.requiredDuringSchedulingIgnoredDuringExecution[].namespaces[]": {
      "type": "string"
    },
    "spec.podOverrides.affinity.podAntiAffinity.requiredDuringSchedulingIgnoredDuringExecution[].topologyKey": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraEnv[].name": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraEnv[].value": {
      "type": "string"
    },
    "spec.podOverrides.extraEnv[].valueFrom.configMapKeyRef.key": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraEnv[].valueFrom.configMapKeyRef.name": {
      "type": "string"
    },
    "spec.podOverrides.extraEnv[].valueFrom.configMapKeyRef.optional": {
      "type": "boolean"
    },
    "spec.podOverrides.extraEnv[].valueFrom.fieldRef.apiVersion": {
      "type": "string"
    },
    "spec.podOverrides.extraEnv[].valueFrom.fieldRef.fieldPath": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraEnv[].valueFrom.fileKeyRef.key": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraEnv[].valueFrom.fileKeyRef.optional": {
      "type": "boolean"
    },
    "spec.podOverrides.extraEnv[].valueFrom.fileKeyRef.path": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraEnv[].valueFrom.fileKeyRef.volumeName": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraEnv[].valueFrom.resourceFieldRef.containerName": {
      "type": "string"
    },
    "spec.podOverrides.extraEnv[].valueFrom.resourceFieldRef.divisor": {
      "pattern": "^(\\+|-)?(([0-9]+(\\.[0-9]*)?)|(\\.[0-9]+))(([KMGTPE]i)|[numkMGTPE]|([eE](\\+|-)?(([0-9]+(\\.[0-9]*)?)|(\\.[0-9]+))))?$"
    },
    "spec.podOverrides.extraEnv[].valueFrom.resourceFieldRef.resource": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraEnv[].valueFrom.secretKeyRef.key": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraEnv[].valueFrom.secretKeyRef.name": {
      "type": "string"
    },
    "spec.podOverrides.extraEnv[].valueFrom.secretKeyRef.optional": {
      "type": "boolean"
    },
    "spec.podOverrides.extraEnvFrom[].configMapRef.name": {
      "type": "string"
    },
    "spec.podOverrides.extraEnvFrom[].configMapRef.optional": {
      "type": "boolean"
    },
    "spec.podOverrides.extraEnvFrom[].prefix": {
      "type": "string"
    },
    "spec.podOverrides.extraEnvFrom[].secretRef.name": {
      "type": "string"
    },
    "spec.podOverrides.extraEnvFrom[].secretRef.optional": {
      "type": "boolean"
    },
    "spec.podOverrides.extraVolumeMounts[].mountPath": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraVolumeMounts[].mountPropagation": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumeMounts[].name": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraVolumeMounts[].readOnly": {
      "type": "boolean"
    },
    "spec.podOverrides.extraVolumeMounts[].recursiveReadOnly": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumeMounts[].subPath": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumeMounts[].subPathExpr": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].awsElasticBlockStore.fsType": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].awsElasticBlockStore.partition": {
      "type": "integer"
    },
    "spec.podOverrides.extraVolumes[].awsElasticBlockStore.readOnly": {
      "type": "boolean"
    },
    "spec.podOverrides.extraVolumes[].awsElasticBlockStore.volumeID": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraVolumes[].azureDisk.cachingMode": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].azureDisk.diskName": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraVolumes[].azureDisk.diskURI": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraVolumes[].azureDisk.fsType": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].azureDisk.kind": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].azureDisk.readOnly": {
      "type": "boolean"
    },
    "spec.podOverrides.extraVolumes[].azureFile.readOnly": {
      "type": "boolean"
    },
    "spec.podOverrides.extraVolumes[].azureFile.secretName": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraVolumes[].azureFile.shareName": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraVolumes[].cephfs.monitors": {
      "required": true
    },
    "spec.podOverrides.extraVolumes[].cephfs.monitors[]": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].cephfs.path": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].cephfs.readOnly": {
      "type": "boolean"
    },
    "spec.podOverrides.extraVolumes[].cephfs.secretFile": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].cephfs.secretRef.name": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].cephfs.user": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].cinder.fsType": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].cinder.readOnly": {
      "type": "boolean"
    },
    "spec.podOverrides.extraVolumes[].cinder.secretRef.name": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].cinder.volumeID": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraVolumes[].configMap.defaultMode": {
      "type": "integer"
    },
    "spec.podOverrides.extraVolumes[].configMap.items[].key": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraVolumes[].configMap.items[].mode": {
      "type": "integer"
    },
    "spec.podOverrides.extraVolumes[].configMap.items[].path": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraVolumes[].configMap.name": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].configMap.optional": {
      "type": "boolean"
    },
    "spec.podOverrides.extraVolumes[].csi.driver": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraVolumes[].csi.fsType": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].csi.nodePublishSecretRef.name": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].csi.readOnly": {
      "type": "boolean"
    },
    "spec.podOverrides.extraVolumes[].downwardAPI.defaultMode": {
      "type": "integer"
    },
    "spec.podOverrides.extraVolumes[].downwardAPI.items[].fieldRef.apiVersion": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].downwardAPI.items[].fieldRef.fieldPath": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraVolumes[].downwardAPI.items[].mode": {
      "type": "integer"
    },
    "spec.podOverrides.extraVolumes[].downwardAPI.items[].path": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraVolumes[].downwardAPI.items[].resourceFieldRef.containerName": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].downwardAPI.items[].resourceFieldRef.divisor": {
      "pattern": "^(\\+|-)?(([0-9]+(\\.[0-9]*)?)|(\\.[0-9]+))(([KMGTPE]i)|[numkMGTPE]|([eE](\\+|-)?(([0-9]+(\\.[0-9]*)?)|(\\.[0-9]+))))?$"
    },
    "spec.podOverrides.extraVolumes[].downwardAPI.items[].resourceFieldRef.resource": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraVolumes[].emptyDir.medium": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].emptyDir.sizeLimit": {
      "pattern": "^(\\+|-)?(([0-9]+(\\.[0-9]*)?)|(\\.[0-9]+))(([KMGTPE]i)|[numkMGTPE]|([eE](\\+|-)?(([0-9]+(\\.[0-9]*)?)|(\\.[0-9]+))))?$"
    },
    "spec.podOverrides.extraVolumes[].ephemeral.volumeClaimTemplate.spec": {
      "required": true
    },
    "spec.podOverrides.extraVolumes[].ephemeral.volumeClaimTemplate.spec.accessModes[]": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].ephemeral.volumeClaimTemplate.spec.dataSource.apiGroup": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].ephemeral.volumeClaimTemplate.spec.dataSource.kind": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraVolumes[].ephemeral.volumeClaimTemplate.spec.dataSource.name": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraVolumes[].ephemeral.volumeClaimTemplate.spec.dataSourceRef.apiGroup": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].ephemeral.volumeClaimTemplate.spec.dataSourceRef.kind": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraVolumes[].ephemeral.volumeClaimTemplate.spec.dataSourceRef.name": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraVolumes[].ephemeral.volumeClaimTemplate.spec.dataSourceRef.namespace": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].ephemeral.volumeClaimTemplate.spec.selector.matchExpressions[].key": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraVolumes[].ephemeral.volumeClaimTemplate.spec.selector.matchExpressions[].operator": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraVolumes[].ephemeral.volumeClaimTemplate.spec.selector.matchExpressions[].values[]": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].ephemeral.volumeClaimTemplate.spec.storageClassName": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].ephemeral.volumeClaimTemplate.spec.volumeAttributesClassName": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].ephemeral.volumeClaimTemplate.spec.volumeMode": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].ephemeral.volumeClaimTemplate.spec.volumeName": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].fc.fsType": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].fc.lun": {
      "type": "integer"
    },
    "spec.podOverrides.extraVolumes[].fc.readOnly": {
      "type": "boolean"
    },
    "spec.podOverrides.extraVolumes[].fc.targetWWNs[]": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].fc.wwids[]": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].flexVolume.driver": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraVolumes[].flexVolume.fsType": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].flexVolume.readOnly": {
      "type": "boolean"
    },
    "spec.podOverrides.extraVolumes[].flexVolume.secretRef.name": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].flocker.datasetName": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].flocker.datasetUUID": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].gcePersistentDisk.fsType": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].gcePersistentDisk.partition": {
      "type": "integer"
    },
    "spec.podOverrides.extraVolumes[].gcePersistentDisk.pdName": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraVolumes[].gcePersistentDisk.readOnly": {
      "type": "boolean"
    },
    "spec.podOverrides.extraVolumes[].gitRepo.directory": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].gitRepo.repository": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraVolumes[].gitRepo.revision": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].glusterfs.endpoints": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraVolumes[].glusterfs.path": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraVolumes[].glusterfs.readOnly": {
      "type": "boolean"
    },
    "spec.podOverrides.extraVolumes[].hostPath.path": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraVolumes[].hostPath.type": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].image.pullPolicy": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].image.reference": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].iscsi.chapAuthDiscovery": {
      "type": "boolean"
    },
    "spec.podOverrides.extraVolumes[].iscsi.chapAuthSession": {
      "type": "boolean"
    },
    "spec.podOverrides.extraVolumes[].iscsi.fsType": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].iscsi.initiatorName": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].iscsi.iqn": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraVolumes[].iscsi.iscsiInterface": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].iscsi.lun": {
      "type": "integer",
      "required": true
    },
    "spec.podOverrides.extraVolumes[].iscsi.portals[]": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].iscsi.readOnly": {
      "type": "boolean"
    },
    "spec.podOverrides.extraVolumes[].iscsi.secretRef.name": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].iscsi.targetPortal": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraVolumes[].name": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraVolumes[].nfs.path": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraVolumes[].nfs.readOnly": {
      "type": "boolean"
    },
    "spec.podOverrides.extraVolumes[].nfs.server": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraVolumes[].persistentVolumeClaim.claimName": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraVolumes[].persistentVolumeClaim.readOnly": {
      "type": "boolean"
    },
    "spec.podOverrides.extraVolumes[].photonPersistentDisk.fsType": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].photonPersistentDisk.pdID": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraVolumes[].portworxVolume.fsType": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].portworxVolume.readOnly": {
      "type": "boolean"
    },
    "spec.podOverrides.extraVolumes[].portworxVolume.volumeID": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraVolumes[].projected.defaultMode": {
      "type": "integer"
    },
    "spec.podOverrides.extraVolumes[].projected.sources[].clusterTrustBundle.labelSelector.matchExpressions[].key": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraVolumes[].projected.sources[].clusterTrustBundle.labelSelector.matchExpressions[].operator": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraVolumes[].projected.sources[].clusterTrustBundle.labelSelector.matchExpressions[].values[]": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].projected.sources[].clusterTrustBundle.name": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].projected.sources[].clusterTrustBundle.optional": {
      "type": "boolean"
    },
    "spec.podOverrides.extraVolumes[].projected.sources[].clusterTrustBundle.path": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraVolumes[].projected.sources[].clusterTrustBundle.signerName": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].projected.sources[].configMap.items[].key": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraVolumes[].projected.sources[].configMap.items[].mode": {
      "type": "integer"
    },
    "spec.podOverrides.extraVolumes[].projected.sources[].configMap.items[].path": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraVolumes[].projected.sources[].configMap.name": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].projected.sources[].configMap.optional": {
      "type": "boolean"
    },
    "spec.podOverrides.extraVolumes[].projected.sources[].downwardAPI.items[].fieldRef.apiVersion": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].projected.sources[].downwardAPI.items[].fieldRef.fieldPath": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraVolumes[].projected.sources[].downwardAPI.items[].mode": {
      "type": "integer"
    },
    "spec.podOverrides.extraVolumes[].projected.sources[].downwardAPI.items[].path": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraVolumes[].projected.sources[].downwardAPI.items[].resourceFieldRef.containerName": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].projected.sources[].downwardAPI.items[].resourceFieldRef.divisor": {
      "pattern": "^(\\+|-)?(([0-9]+(\\.[0-9]*)?)|(\\.[0-9]+))(([KMGTPE]i)|[numkMGTPE]|([eE](\\+|-)?(([0-9]+(\\.[0-9]*)?)|(\\.[0-9]+))))?$"
    },
    "spec.podOverrides.extraVolumes[].projected.sources[].downwardAPI.items[].resourceFieldRef.resource": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraVolumes[].projected.sources[].podCertificate.certificateChainPath": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].projected.sources[].podCertificate.credentialBundlePath": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].projected.sources[].podCertificate.keyPath": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].projected.sources[].podCertificate.keyType": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraVolumes[].projected.sources[].podCertificate.maxExpirationSeconds": {
      "type": "integer"
    },
    "spec.podOverrides.extraVolumes[].projected.sources[].podCertificate.signerName": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraVolumes[].projected.sources[].secret.items[].key": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraVolumes[].projected.sources[].secret.items[].mode": {
      "type": "integer"
    },
    "spec.podOverrides.extraVolumes[].projected.sources[].secret.items[].path": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraVolumes[].projected.sources[].secret.name": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].projected.sources[].secret.optional": {
      "type": "boolean"
    },
    "spec.podOverrides.extraVolumes[].projected.sources[].serviceAccountToken.audience": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].projected.sources[].serviceAccountToken.expirationSeconds": {
      "type": "integer"
    },
    "spec.podOverrides.extraVolumes[].projected.sources[].serviceAccountToken.path": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraVolumes[].quobyte.group": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].quobyte.readOnly": {
      "type": "boolean"
    },
    "spec.podOverrides.extraVolumes[].quobyte.registry": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraVolumes[].quobyte.tenant": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].quobyte.user": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].quobyte.volume": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraVolumes[].rbd.fsType": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].rbd.image": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraVolumes[].rbd.keyring": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].rbd.monitors": {
      "required": true
    },
    "spec.podOverrides.extraVolumes[].rbd.monitors[]": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].rbd.pool": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].rbd.readOnly": {
      "type": "boolean"
    },
    "spec.podOverrides.extraVolumes[].rbd.secretRef.name": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].rbd.user": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].scaleIO.fsType": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].scaleIO.gateway": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraVolumes[].scaleIO.protectionDomain": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].scaleIO.readOnly": {
      "type": "boolean"
    },
    "spec.podOverrides.extraVolumes[].scaleIO.secretRef": {
      "required": true
    },
    "spec.podOverrides.extraVolumes[].scaleIO.secretRef.name": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].scaleIO.sslEnabled": {
      "type": "boolean"
    },
    "spec.podOverrides.extraVolumes[].scaleIO.storageMode": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].scaleIO.storagePool": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].scaleIO.system": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraVolumes[].scaleIO.volumeName": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].secret.defaultMode": {
      "type": "integer"
    },
    "spec.podOverrides.extraVolumes[].secret.items[].key": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraVolumes[].secret.items[].mode": {
      "type": "integer"
    },
    "spec.podOverrides.extraVolumes[].secret.items[].path": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.extraVolumes[].secret.optional": {
      "type": "boolean"
    },
    "spec.podOverrides.extraVolumes[].secret.secretName": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].storageos.fsType": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].storageos.readOnly": {
      "type": "boolean"
    },
    "spec.podOverrides.extraVolumes[].storageos.secretRef.name": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].storageos.volumeName": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].storageos.volumeNamespace": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].vsphereVolume.fsType": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].vsphereVolume.storagePolicyID": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].vsphereVolume.storagePolicyName": {
      "type": "string"
    },
    "spec.podOverrides.extraVolumes[].vsphereVolume.volumePath": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.imagePullSecrets[].name": {
      "type": "string"
    },
    "spec.podOverrides.priorityClassName": {
      "type": "string"
    },
    "spec.podOverrides.serviceAccountName": {
      "type": "string"
    },
    "spec.podOverrides.tolerations[].effect": {
      "type": "string"
    },
    "spec.podOverrides.tolerations[].key": {
      "type": "string"
    },
    "spec.podOverrides.tolerations[].operator": {
      "type": "string"
    },
    "spec.podOverrides.tolerations[].tolerationSeconds": {
      "type": "integer"
    },
    "spec.podOverrides.tolerations[].value": {
      "type": "string"
    },
    "spec.podOverrides.topologySpreadConstraints[].labelSelector.matchExpressions[].key": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.topologySpreadConstraints[].labelSelector.matchExpressions[].operator": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.topologySpreadConstraints[].labelSelector.matchExpressions[].values[]": {
      "type": "string"
    },
    "spec.podOverrides.topologySpreadConstraints[].matchLabelKeys[]": {
      "type": "string"
    },
    "spec.podOverrides.topologySpreadConstraints[].maxSkew": {
      "type": "integer",
      "required": true
    },
    "spec.podOverrides.topologySpreadConstraints[].minDomains": {
      "type": "integer"
    },
    "spec.podOverrides.topologySpreadConstraints[].nodeAffinityPolicy": {
      "type": "string"
    },
    "spec.podOverrides.topologySpreadConstraints[].nodeTaintsPolicy": {
      "type": "string"
    },
    "spec.podOverrides.topologySpreadConstraints[].topologyKey": {
      "type": "string",
      "required": true
    },
    "spec.podOverrides.topologySpreadConstraints[].whenUnsatisfiable": {
      "type": "string",
      "required": true
    },
    "spec.privacyPolicyRef.name": {
      "type": "string"
    },
    "spec.promptPackRef": {
      "required": true
    },
    "spec.promptPackRef.name": {
      "type": "string",
      "minLength": 1,
      "required": true
    },
    "spec.promptPackRef.version": {
      "type": "string"
    },
    "spec.providers[].name": {
      "type": "string",
      "pattern": "^[a-z0-9]([-a-z0-9]*[a-z0-9])?$",
      "minLength": 1,
      "required": true
    },
    "spec.providers[].providerRef": {
      "required": true
    },
    "spec.providers[].providerRef.name": {
      "type": "string",
      "minLength": 1,
      "required": true
    },
    "spec.providers[].providerRef.namespace": {
      "type": "string"
    },
    "spec.providers[].requiredCapabilities[]": {
      "type": "string",
      "enum": [
        "text",
        "streaming",
        "vision",
        "tools",
        "json",
        "audio",
        "video",
        "documents",
        "duplex"
      ]
    },
    "spec.providers[].role": {
      "type": "string",
      "enum": [
        "llm",
        "embedding",
        "tts",
        "stt",
        "image",
        "inference"
      ]
    },
    "spec.rollout.candidate.promptPackRef.name": {
      "type": "string",
      "minLength": 1,
      "required": true
    },
    "spec.rollout.candidate.promptPackRef.version": {
      "type": "string"
    },
    "spec.rollout.candidate.providerRefs[].name": {
      "type": "string",
      "pattern": "^[a-z0-9]([-a-z0-9]*[a-z0-9])?$",
      "minLength": 1,
      "required": true
    },
    "spec.rollout.candidate.providerRefs[].providerRef": {
      "required": true
    },
    "spec.rollout.candidate.providerRefs[].providerRef.name": {
      "type": "string",
      "minLength": 1,
      "required": true
    },
    "spec.rollout.candidate.providerRefs[].providerRef.namespace": {
      "type": "string"
    },
    "spec.rollout.candidate.providerRefs[].requiredCapabilities[]": {
      "type": "string",
      "enum": [
        "text",
        "streaming",
        "vision",
        "tools",
        "json",
        "audio",
        "video",
        "documents",
        "duplex"
      ]
    },
    "spec.rollout.candidate.providerRefs[].role": {
      "type": "string",
      "enum": [
        "llm",
        "embedding",
        "tts",
        "stt",
        "image",
        "inference"
      ]
    },
    "spec.rollout.candidate.toolRegistryRef.name": {
      "type": "string",
      "minLength": 1,
      "required": true
    },
    "spec.rollout.candidate.toolRegistryRef.namespace": {
      "type": "string"
    },
    "spec.rollout.rollback.cooldown": {
      "type": "string"
    },
    "spec.rollout.rollback.mode": {
      "type": "string",
      "enum": [
        "automatic",
        "manual",
        "disabled"
      ]
    },
    "spec.rollout.steps": {
      "required": true
    },
    "spec.rollout.steps[].analysis.args[].name": {
      "type": "string",
      "minLength": 1,
      "required": true
    },
    "spec.rollout.steps[].analysis.args[].value": {
      "type": "string",
      "required": true
    },
    "spec.rollout.steps[].analysis.templateName": {
      "type": "string",
      "minLength": 1,
      "required": true
    },
    "spec.rollout.steps[].pause.duration": {
      "type": "string"
    },
    "spec.rollout.steps[].setWeight": {
      "type": "integer",
      "minimum": 0,
      "maximum": 100
    },
    "spec.rollout.stickySession.hashOn": {
      "type": "string",
      "minLength": 1,
      "required": true
    },
    "spec.rollout.trafficRouting.istio.destinationRule": {
      "required": true
    },
    "spec.rollout.trafficRouting.istio.destinationRule.candidateSubset": {
      "type": "string"
    },
    "spec.rollout.trafficRouting.istio.destinationRule.name": {
      "type": "string",
      "minLength": 1,
      "required": true
    },
    "spec.rollout.trafficRouting.istio.destinationRule.stableSubset": {
      "type": "string"
    },
    "spec.rollout.trafficRouting.istio.virtualService": {
      "required": true
    },
    "spec.rollout.trafficRouting.istio.virtualService.name": {
      "type": "string",
      "minLength": 1,
      "required": true
    },
    "spec.rollout.trafficRouting.istio.virtualService.routes": {
      "required": true
    },
    "spec.rollout.trafficRouting.istio.virtualService.routes[]": {
      "type": "string"
    },
    "spec.rollout.trafficRouting.mesh.candidateSubset": {
      "type": "string"
    },
    "spec.rollout.trafficRouting.mesh.hosts[]": {
      "type": "string"
    },
    "spec.rollout.trafficRouting.mesh.stableSubset": {
      "type": "string"
    },
    "spec.rollout.trafficRouting.mesh.waypoint": {
      "type": "string"
    },
    "spec.rollout.trafficRouting.mode": {
      "type": "string",
      "enum": [
        "mesh",
        "replicaWeighted",
        "external"
      ]
    },
    "spec.runtime.affinity.nodeAffinity.preferredDuringSchedulingIgnoredDuringExecution[].preference": {
      "required": true
    },
    "spec.runtime.affinity.nodeAffinity.preferredDuringSchedulingIgnoredDuringExecution[].preference.matchExpressions[].key": {
      "type": "string",
      "required": true
    },
    "spec.runtime.affinity.nodeAffinity.preferredDuringSchedulingIgnoredDuringExecution[].preference.matchExpressions[].operator": {
      "type": "string",
      "required": true
    },
    "spec.runtime.affinity.nodeAffinity.preferredDuringSchedulingIgnoredDuringExecution[].preference.matchExpressions[].values[]": {
      "type": "string"
    },
    "spec.runtime.affinity.nodeAffinity.preferredDuringSchedulingIgnoredDuringExecution[].preference.matchFields[].key": {
      "type": "string",
      "required": true
    },
    "spec.runtime.affinity.nodeAffinity.preferredDuringSchedulingIgnoredDuringExecution[].preference.matchFields[].operator": {
      "type": "string",
      "required": true
    },
    "spec.runtime.affinity.nodeAffinity.preferredDuringSchedulingIgnoredDuringExecution[].preference.matchFields[].values[]": {
      "type": "string"
    },
    "spec.runtime.affinity.nodeAffinity.preferredDuringSchedulingIgnoredDuringExecution[].weight": {
      "type": "integer",
      "required": true
    },
    "spec.runtime.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms": {
      "required": true
    },
    "spec.runtime.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms[].matchExpressions[].key": {
      "type": "string",
      "required": true
    },
    "spec.runtime.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms[].matchExpressions[].operator": {
      "type": "string",
      "required": true
    },
    "spec.runtime.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms[].matchExpressions[].values[]": {
      "type": "string"
    },
    "spec.runtime.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms[].matchFields[].key": {
      "type": "string",
      "required": true
    },
    "spec.runtime.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms[].matchFields[].operator": {
      "type": "string",
      "required": true
    },
    "spec.runtime.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms[].matchFields[].values[]": {
      "type": "string"
    },
    "spec.runtime.affinity.podAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm": {
      "required": true
    },
    "spec.runtime.affinity.podAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm.labelSelector.matchExpressions[].key": {
      "type": "string",
      "required": true
    },
    "spec.runtime.affinity.podAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm.labelSelector.matchExpressions[].operator": {
      "type": "string",
      "required": true
    },
    "spec.runtime.affinity.podAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm.labelSelector.matchExpressions[].values[]": {
      "type": "string"
    },
    "spec.runtime.affinity.podAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm.matchLabelKeys[]": {
      "type": "string"
    },
    "spec.runtime.affinity.podAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm.mismatchLabelKeys[]": {
      "type": "string"
    },
    "spec.runtime.affinity.podAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm.namespaceSelector.matchExpressions[].key": {
      "type": "string",
      "required": true
    },
    "spec.runtime.affinity.podAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm.namespaceSelector.matchExpressions[].operator": {
      "type": "string",
      "required": true
    },
    "spec.runtime.affinity.podAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm.namespaceSelector.matchExpressions[].values[]": {
      "type": "string"
    },
    "spec.runtime.affinity.podAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm.namespaces[]": {
      "type": "string"
    },
    "spec.runtime.affinity.podAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm.topologyKey": {
      "type": "string",
      "required": true
    },
    "spec.runtime.affinity.podAffinity.preferredDuringSchedulingIgnoredDuringExecution[].weight": {
      "type": "integer",
      "required": true
    },
    "spec.runtime.affinity.podAffinity.requiredDuringSchedulingIgnoredDuringExecution[].labelSelector.matchExpressions[].key": {
      "type": "string",
      "required": true
    },
    "spec.runtime.affinity.podAffinity.requiredDuringSchedulingIgnoredDuringExecution[].labelSelector.matchExpressions[].operator": {
      "type": "string",
      "required": true
    },
    "spec.runtime.affinity.podAffinity.requiredDuringSchedulingIgnoredDuringExecution[].labelSelector.matchExpressions[].values[]": {
      "type": "string"
    },
    "spec.runtime.affinity.podAffinity.requiredDuringSchedulingIgnoredDuringExecution[].matchLabelKeys[]": {
      "type": "string"
    },
    "spec.runtime.affinity.podAffinity.requiredDuringSchedulingIgnoredDuringExecution[].mismatchLabelKeys[]": {
      "type": "string"
    },
    "spec.runtime.affinity.podAffinity.requiredDuringSchedulingIgnoredDuringExecution[].namespaceSelector.matchExpressions[].key": {
      "type": "string",
      "required": true
    },
    "spec.runtime.affinity.podAffinity.requiredDuringSchedulingIgnoredDuringExecution[].namespaceSelector.matchExpressions[].operator": {
      "type": "string",
      "required": true
    },
    "spec.runtime.affinity.podAffinity.requiredDuringSchedulingIgnoredDuringExecution[].namespaceSelector.matchExpressions[].values[]": {
      "type": "string"
    },
    "spec.runtime.affinity.podAffinity.requiredDuringSchedulingIgnoredDuringExecution[].namespaces[]": {
      "type": "string"
    },
    "spec.runtime.affinity.podAffinity.requiredDuringSchedulingIgnoredDuringExecution[].topologyKey": {
      "type": "string",
      "required": true
    },
    "spec.runtime.affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm": {
      "required": true
    },
    "spec.runtime.affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm.labelSelector.matchExpressions[].key": {
      "type": "string",
      "required": true
    },
    "spec.runtime.affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm.labelSelector.matchExpressions[].operator": {
      "type": "string",
      "required": true
    },
    "spec.runtime.affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm.labelSelector.matchExpressions[].values[]": {
      "type": "string"
    },
    "spec.runtime.affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm.matchLabelKeys[]": {
      "type": "string"
    },
    "spec.runtime.affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm.mismatchLabelKeys[]": {
      "type": "string"
    },
    "spec.runtime.affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm.namespaceSelector.matchExpressions[].key": {
      "type": "string",
      "required": true
    },
    "spec.runtime.affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm.namespaceSelector.matchExpressions[].operator": {
      "type": "string",
      "required": true
    },
    "spec.runtime.affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm.namespaceSelector.matchExpressions[].values[]": {
      "type": "string"
    },
    "spec.runtime.affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm.namespaces[]": {
      "type": "string"
    },
    "spec.runtime.affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution[].podAffinityTerm.topologyKey": {
      "type": "string",
      "required": true
    },
    "spec.runtime.affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution[].weight": {
      "type": "integer",
      "required": true
    },
    "spec.runtime.affinity.podAntiAffinity.requiredDuringSchedulingIgnoredDuringExecution[].labelSelector.matchExpressions[].key": {
      "type": "string",
      "required": true
    },
    "spec.runtime.affinity.podAntiAffinity.requiredDuringSchedulingIgnoredDuringExecution[].labelSelector.matchExpressions[].operator": {
      "type": "string",
      "required": true
    },
    "spec.runtime.affinity.podAntiAffinity.requiredDuringSchedulingIgnoredDuringExecution[].labelSelector.matchExpressions[].values[]": {
      "type": "string"
    },
    "spec.runtime.affinity.podAntiAffinity.requiredDuringSchedulingIgnoredDuringExecution[].matchLabelKeys[]": {
      "type": "string"
    },
    "spec.runtime.affinity.podAntiAffinity.requiredDuringSchedulingIgnoredDuringExecution[].mismatchLabelKeys[]": {
      "type": "string"
    },
    "spec.runtime.affinity.podAntiAffinity.requiredDuringSchedulingIgnoredDuringExecution[].namespaceSelector.matchExpressions[].key": {
      "type": "string",
      "required": true
    },
    "spec.runtime.affinity.podAntiAffinity.requiredDuringSchedulingIgnoredDuringExecution[].namespaceSelector.matchExpressions[].operator": {
      "type": "string",
      "required": true
    },
    "spec.runtime.affinity.podAntiAffinity.requiredDuringSchedulingIgnoredDuringExecution[].namespaceSelector.matchExpressions[].values[]": {
      "type": "string"
    },
    "spec.runtime.affinity.podAntiAffinity.requiredDuringSchedulingIgnoredDuringExecution[].namespaces[]": {
      "type": "string"
    },
    "spec.runtime.affinity.podAntiAffinity.requiredDuringSchedulingIgnoredDuringExecution[].topologyKey": {
      "type": "string",
      "required": true
    },
    "spec.runtime.autoscaling.enabled": {
      "type": "boolean"
    },
    "spec.runtime.autoscaling.keda.connectionThreshold": {
      "type": "integer",
      "minimum": 1
    },
    "spec.runtime.autoscaling.keda.cooldownPeriod": {
      "type": "integer",
      "minimum": 0
    },
    "spec.runtime.autoscaling.keda.pollingInterval": {
      "type": "integer",
      "minimum": 1
    },
    "spec.runtime.autoscaling.keda.triggers[].metadata": {
      "required": true
    },
    "spec.runtime.autoscaling.keda.triggers[].type": {
      "type": "string",
      "required": true
    },
    "spec.runtime.autoscaling.maxReplicas": {
      "type": "integer",
      "minimum": 1
    },
    "spec.runtime.autoscaling.minReplicas": {
      "type": "integer",
      "minimum": 0
    },
    "spec.runtime.autoscaling.scaleDownStabilizationSeconds": {
      "type": "integer",
      "minimum": 0,
      "maximum": 3600
    },
    "spec.runtime.autoscaling.targetCPUUtilizationPercentage": {
      "type": "integer",
      "minimum": 1,
      "maximum": 100
    },
    "spec.runtime.autoscaling.targetMemoryUtilizationPercentage": {
      "type": "integer",
      "minimum": 1,
      "maximum": 100
    },
    "spec.runtime.autoscaling.type": {
      "type": "string",
      "enum": [
        "hpa",
        "keda"
      ]
    },
    "spec.runtime.extraEnv[].name": {
      "type": "string",
      "required": true
    },
    "spec.runtime.extraEnv[].value": {
      "type": "string"
    },
    "spec.runtime.extraEnv[].valueFrom.configMapKeyRef.key": {
      "type": "string",
      "required": true
    },
    "spec.runtime.extraEnv[].valueFrom.configMapKeyRef.name": {
      "type": "string"
    },
    "spec.runtime.extraEnv[].valueFrom.configMapKeyRef.optional": {
      "type": "boolean"
    },
    "spec.runtime.extraEnv[].valueFrom.fieldRef.apiVersion": {
      "type": "string"
    },
    "spec.runtime.extraEnv[].valueFrom.fieldRef.fieldPath": {
      "type": "string",
      "required": true
    },
    "spec.runtime.extraEnv[].valueFrom.fileKeyRef.key": {
      "type": "string",
      "required": true
    },
    "spec.runtime.extraEnv[].valueFrom.fileKeyRef.optional": {
      "type": "boolean"
    },
    "spec.runtime.extraEnv[].valueFrom.fileKeyRef.path": {
      "type": "string",
      "required": true
    },
    "spec.runtime.extraEnv[].valueFrom.fileKeyRef.volumeName": {
      "type": "string",
      "required": true
    },
    "spec.runtime.extraEnv[].valueFrom.resourceFieldRef.containerName": {
      "type": "string"
    },
    "spec.runtime.extraEnv[].valueFrom.resourceFieldRef.divisor": {
      "pattern": "^(\\+|-)?(([0-9]+(\\.[0-9]*)?)|(\\.[0-9]+))(([KMGTPE]i)|[numkMGTPE]|([eE](\\+|-)?(([0-9]+(\\.[0-9]*)?)|(\\.[0-9]+))))?$"
    },
    "spec.runtime.extraEnv[].valueFrom.resourceFieldRef.resource": {
      "type": "string",
      "required": true
    },
    "spec.runtime.extraEnv[].valueFrom.secretKeyRef.key": {
      "type": "string",
      "required": true
    },
    "spec.runtime.extraEnv[].valueFrom.secretKeyRef.name": {
      "type": "string"
    },
    "spec.runtime.extraEnv[].valueFrom.secretKeyRef.optional": {
      "type": "boolean"
    },
    "spec.runtime.replicas": {
      "type": "integer",
      "minimum": 0
    },
    "spec.runtime.resources.claims[].name": {
      "type": "string",
      "required": true
    },
    "spec.runtime.resources.claims[].request": {
      "type": "string"
    },
    "spec.runtime.tolerations[].effect": {
      "type": "string"
    },
    "spec.runtime.tolerations[].key": {
      "type": "string"
    },
    "spec.runtime.tolerations[].operator": {
      "type": "string"
    },
    "spec.runtime.tolerations[].tolerationSeconds": {
      "type": "integer"
    },
    "spec.runtime.tolerations[].value": {
      "type": "string"
    },
    "spec.runtime.volumeMounts[].mountPath": {
      "type": "string",
      "required": true
    },
    "spec.runtime.volumeMounts[].mountPropagation": {
      "type": "string"
    },
    "spec.runtime.volumeMounts[].name": {
      "type": "string",
      "required": true
    },
    "spec.runtime.volumeMounts[].readOnly": {
      "type": "boolean"
    },
    "spec.runtime.volumeMounts[].recursiveReadOnly": {
      "type": "string"
    },
    "spec.runtime.volumeMounts[].subPath": {
      "type": "string"
    },
    "spec.runtime.volumeMounts[].subPathExpr": {
      "type": "string"
    },
    "spec.runtime.volumes[].awsElasticBlockStore.fsType": {
      "type": "string"
    },
    "spec.runtime.volumes[].awsElasticBlockStore.partition": {
      "type": "integer"
    },
    "spec.runtime.volumes[].awsElasticBlockStore.readOnly": {
      "type": "boolean"
    },
    "spec.runtime.volumes[].awsElasticBlockStore.volumeID": {
      "type": "string",
      "required": true
    },
    "spec.runtime.volumes[].azureDisk.cachingMode": {
      "type": "string"
    },
    "spec.runtime.volumes[].azureDisk.diskName": {
      "type": "string",
      "required": true
    },
    "spec.runtime.volumes[].azureDisk.diskURI": {
      "type": "string",
      "required": true
    },
    "spec.runtime.volumes[].azureDisk.fsType": {
      "type": "string"
    },
    "spec.runtime.volumes[].azureDisk.kind": {
      "type": "string"
    },
    "spec.runtime.volumes[].azureDisk.readOnly": {
      "type": "boolean"
    },
    "spec.runtime.volumes[].azureFile.readOnly": {
      "type": "boolean"
    },
    "spec.runtime.volumes[].azureFile.secretName": {
      "type": "string",
      "required": true
    },
    "spec.runtime.volumes[].azureFile.shareName": {
      "type": "string",
      "required": true
    },
    "spec.runtime.volumes[].cephfs.monitors": {
      "required": true
    },
    "spec.runtime.volumes[].cephfs.monitors[]": {
      "type": "string"
    },
    "spec.runtime.volumes[].cephfs.path": {
      "type": "string"
    },
    "spec.runtime.volumes[].cephfs.readOnly": {
      "type": "boolean"
    },
    "spec.runtime.volumes[].cephfs.secretFile": {
      "type": "string"
    },
    "spec.runtime.volumes[].cephfs.secretRef.name": {
      "type": "string"
    },
    "spec.runtime.volumes[].cephfs.user": {
      "type": "string"
    },
    "spec.runtime.volumes[].cinder.fsType": {
      "type": "string"
    },
    "spec.runtime.volumes[].cinder.readOnly": {
      "type": "boolean"
    },
    "spec.runtime.volumes[].cinder.secretRef.name": {
      "type": "string"
    },
    "spec.runtime.volumes[].cinder.volumeID": {
      "type": "string",
      "required": true
    },
    "spec.runtime.volumes[].configMap.defaultMode": {
      "type": "integer"
    },
    "spec.runtime.volumes[].configMap.items[].key": {
      "type": "string",
      "required": true
    },
    "spec.runtime.volumes[].configMap.items[].mode": {
      "type": "integer"
    },
    "spec.runtime.volumes[].configMap.items[].path": {
      "type": "string",
      "required": true
    },
    "spec.runtime.volumes[].configMap.name": {
      "type": "string"
    },
    "spec.runtime.volumes[].configMap.optional": {
      "type": "boolean"
    },
    "spec.runtime.volumes[].csi.driver": {
      "type": "string",
      "required": true
    },
    "spec.runtime.volumes[].csi.fsType": {
      "type": "string"
    },
    "spec.runtime.volumes[].csi.nodePublishSecretRef.name": {
      "type": "string"
    },
    "spec.runtime.volumes[].csi.readOnly": {
      "type": "boolean"
    },
    "spec.runtime.volumes[].downwardAPI.defaultMode": {
      "type": "integer"
    },
    "spec.runtime.volumes[].downwardAPI.items[].fieldRef.apiVersion": {
      "type": "string"
    },
    "spec.runtime.volumes[].downwardAPI.items[].fieldRef.fieldPath": {
      "type": "string",
      "required": true
    },
    "spec.runtime.volumes[].downwardAPI.items[].mode": {
      "type": "integer"
    },
    "spec.runtime.volumes[].downwardAPI.items[].path": {
      "type": "string",
      "required": true
    },
    "spec.runtime.volumes[].downwardAPI.items[].resourceFieldRef.containerName": {
      "type": "string"
    },
    "spec.runtime.volumes[].downwardAPI.items[].resourceFieldRef.divisor": {
      "pattern": "^(\\+|-)?(([0-9]+(\\.[0-9]*)?)|(\\.[0-9]+))(([KMGTPE]i)|[numkMGTPE]|([eE](\\+|-)?(([0-9]+(\\.[0-9]*)?)|(\\.[0-9]+))))?$"
    },
    "spec.runtime.volumes[].downwardAPI.items[].resourceFieldRef.resource": {
      "type": "string",
      "required": true
    },
    "spec.runtime.volumes[].emptyDir.medium": {
      "type": "string"
    },
    "spec.runtime.volumes[].emptyDir.sizeLimit": {
      "pattern": "^(\\+|-)?(([0-9]+(\\.[0-9]*)?)|(\\.[0-9]+))(([KMGTPE]i)|[numkMGTPE]|([eE](\\+|-)?(([0-9]+(\\.[0-9]*)?)|(\\.[0-9]+))))?$"
    },
    "spec.runtime.volumes[].ephemeral.volumeClaimTemplate.spec": {
      "required": true
    },
    "spec.runtime.volumes[].ephemeral.volumeClaimTemplate.spec.accessModes[]": {
      "type": "string"
    },
    "spec.runtime.volumes[].ephemeral.volumeClaimTemplate.spec.dataSource.apiGroup": {
      "type": "string"
    },
    "spec.runtime.volumes[].ephemeral.volumeClaimTemplate.spec.dataSource.kind": {
      "type": "string",
      "required": true
    },
    "spec.runtime.volumes[].ephemeral.volumeClaimTemplate.spec.dataSource.name": {
      "type": "string",
      "required": true
    },
    "spec.runtime.volumes[].ephemeral.volumeClaimTemplate.spec.dataSourceRef.apiGroup": {
      "type": "string"
    },
    "spec.runtime.volumes[].ephemeral.volumeClaimTemplate.spec.dataSourceRef.kind": {
      "type": "string",
      "required": true
    },
    "spec.runtime.volumes[].ephemeral.volumeClaimTemplate.spec.dataSourceRef.name": {
      "type": "string",
      "required": true
    },
    "spec.runtime.volumes[].ephemeral.volumeClaimTemplate.spec.dataSourceRef.namespace": {
      "type": "string"
    },
    "spec.runtime.volumes[].ephemeral.volumeClaimTemplate.spec.selector.matchExpressions[].key": {
      "type": "string",
      "required": true
    },
    "spec.runtime.volumes[].ephemeral.volumeClaimTemplate.spec.selector.matchExpressions[].operator": {
      "type": "string",
      "required": true
    },
    "spec.runtime.volumes[].ephemeral.volumeClaimTemplate.spec.selector.matchExpressions[].values[]": {
      "type": "string"
    },
    "spec.runtime.volumes[].ephemeral.volumeClaimTemplate.spec.storageClassName": {
      "type": "string"
    },
    "spec.runtime.volumes[].ephemeral.volumeClaimTemplate.spec.volumeAttributesClassName": {
      "type": "string"
    },
    "spec.runtime.volumes[].ephemeral.volumeClaimTemplate.spec.volumeMode": {
      "type": "string"
    },
    "spec.runtime.volumes[].ephemeral.volumeClaimTemplate.spec.volumeName": {
      "type": "string"
    },
    "spec.runtime.volumes[].fc.fsType": {
      "type": "string"
    },
    "spec.runtime.volumes[].fc.lun": {
      "type": "integer"
    },
    "spec.runtime.volumes[].fc.readOnly": {
      "type": "boolean"
    },
    "spec.runtime.volumes[].fc.targetWWNs[]": {
      "type": "string"
    },
    "spec.runtime.volumes[].fc.wwids[]": {
      "type": "string"
    },
    "spec.runtime.volumes[].flexVolume.driver": {
      "type": "string",
      "required": true
    },
    "spec.runtime.volumes[].flexVolume.fsType": {
      "type": "string"
    },
    "spec.runtime.volumes[].flexVolume.readOnly": {
      "type": "boolean"
    },
    "spec.runtime.volumes[].flexVolume.secretRef.name": {
      "type": "string"
    },
    "spec.runtime.volumes[].flocker.datasetName": {
      "type": "string"
    },
    "spec.runtime.volumes[].flocker.datasetUUID": {
      "type": "string"
    },
    "spec.runtime.volumes[].gcePersistentDisk.fsType": {
      "type": "string"
    },
    "spec.runtime.volumes[].gcePersistentDisk.partition": {
      "type": "integer"
    },
    "spec.runtime.volumes[].gcePersistentDisk.pdName": {
      "type": "string",
      "required": true
    },
    "spec.runtime.volumes[].gcePersistentDisk.readOnly": {
      "type": "boolean"
    },
    "spec.runtime.volumes[].gitRepo.directory": {
      "type": "string"
    },
    "spec.runtime.volumes[].gitRepo.repository": {
      "type": "string",
      "required": true
    },
    "spec.runtime.volumes[].gitRepo.revision": {
      "type": "string"
    },
    "spec.runtime.volumes[].glusterfs.endpoints": {
      "type": "string",
      "required": true
    },
    "spec.runtime.volumes[].glusterfs.path": {
      "type": "string",
      "required": true
    },
    "spec.runtime.volumes[].glusterfs.readOnly": {
      "type": "boolean"
    },
    "spec.runtime.volumes[].hostPath.path": {
      "type": "string",
      "required": true
    },
    "spec.runtime.volumes[].hostPath.type": {
      "type": "string"
    },
    "spec.runtime.volumes[].image.pullPolicy": {
      "type": "string"
    },
    "spec.runtime.volumes[].image.reference": {
      "type": "string"
    },
    "spec.runtime.volumes[].iscsi.chapAuthDiscovery": {
      "type": "boolean"
    },
    "spec.runtime.volumes[].iscsi.chapAuthSession": {
      "type": "boolean"
    },
    "spec.runtime.volumes[].iscsi.fsType": {
      "type": "string"
    },
    "spec.runtime.volumes[].iscsi.initiatorName": {
      "type": "string"
    },
    "spec.runtime.volumes[].iscsi.iqn": {
      "type": "string",
      "required": true
    },
    "spec.runtime.volumes[].iscsi.iscsiInterface": {
      "type": "string"
    },
    "spec.runtime.volumes[].iscsi.lun": {
      "type": "integer",
      "required": true
    },
    "spec.runtime.volumes[].iscsi.portals[]": {
      "type": "string"
    },
    "spec.runtime.volumes[].iscsi.readOnly": {
      "type": "boolean"
    },
    "spec.runtime.volumes[].iscsi.secretRef.name": {
      "type": "string"
    },
    "spec.runtime.volumes[].iscsi.targetPortal": {
      "type": "string",
      "required": true
    },
    "spec.runtime.volumes[].name": {
      "type": "string",
      "required": true
    },
    "spec.runtime.volumes[].nfs.path": {
      "type": "string",
      "required": true
    },
    "spec.runtime.volumes[].nfs.readOnly": {
      "type": "boolean"
    },
    "spec.runtime.volumes[].nfs.server": {
      "type": "string",
      "required": true
    },
    "spec.runtime.volumes[].persistentVolumeClaim.claimName": {
      "type": "string",
      "required": true
    },
    "spec.runtime.volumes[].persistentVolumeClaim.readOnly": {
      "type": "boolean"
    },
    "spec.runtime.volumes[].photonPersistentDisk.fsType": {
      "type": "string"
    },
    "spec.runtime.volumes[].photonPersistentDisk.pdID": {
      "type": "string",
      "required": true
    },
    "spec.runtime.volumes[].portworxVolume.fsType": {
      "type": "string"
    },
    "spec.runtime.volumes[].portworxVolume.readOnly": {
      "type": "boolean"
    },
    "spec.runtime.volumes[].portworxVolume.volumeID": {
      "type": "string",
      "required": true
    },
    "spec.runtime.volumes[].projected.defaultMode": {
      "type": "integer"
    },
    "spec.runtime.volumes[].projected.sources[].clusterTrustBundle.labelSelector.matchExpressions[].key": {
      "type": "string",
      "required": true
    },
    "spec.runtime.volumes[].projected.sources[].clusterTrustBundle.labelSelector.matchExpressions[].operator": {
      "type": "string",
      "required": true
    },
    "spec.runtime.volumes[].projected.sources[].clusterTrustBundle.labelSelector.matchExpressions[].values[]": {
      "type": "string"
    },
    "spec.runtime.volumes[].projected.sources[].clusterTrustBundle.name": {
      "type": "string"
    },
    "spec.runtime.volumes[].projected.sources[].clusterTrustBundle.optional": {
      "type": "boolean"
    },
    "spec.runtime.volumes[].projected.sources[].clusterTrustBundle.path": {
      "type": "string",
      "required": true
    },
    "spec.runtime.volumes[].projected.sources[].clusterTrustBundle.signerName": {
      "type": "string"
    },
    "spec.runtime.volumes[].projected.sources[].configMap.items[].key": {
      "type": "string",
      "required": true
    },
    "spec.runtime.volumes[].projected.sources[].configMap.items[].mode": {
      "type": "integer"
    },
    "spec.runtime.volumes[].projected.sources[].configMap.items[].path": {
      "type": "string",
      "required": true
    },
    "spec.runtime.volumes[].projected.sources[].configMap.name": {
      "type": "string"
    },
    "spec.runtime.volumes[].projected.sources[].configMap.optional": {
      "type": "boolean"
    },
    "spec.runtime.volumes[].projected.sources[].downwardAPI.items[].fieldRef.apiVersion": {
      "type": "string"
    },
    "spec.runtime.volumes[].projected.sources[].downwardAPI.items[].fieldRef.fieldPath": {
      "type": "string",
      "required": true
    },
    "spec.runtime.volumes[].projected.sources[].downwardAPI.items[].mode": {
      "type": "integer"
    },
    "spec.runtime.volumes[].projected.sources[].downwardAPI.items[].path": {
      "type": "string",
      "required": true
    },
    "spec.runtime.volumes[].projected.sources[].downwardAPI.items[].resourceFieldRef.containerName": {
      "type": "string"
    },
    "spec.runtime.volumes[].projected.sources[].downwardAPI.items[].resourceFieldRef.divisor": {
      "pattern": "^(\\+|-)?(([0-9]+(\\.[0-9]*)?)|(\\.[0-9]+))(([KMGTPE]i)|[numkMGTPE]|([eE](\\+|-)?(([0-9]+(\\.[0-9]*)?)|(\\.[0-9]+))))?$"
    },
    "spec.runtime.volumes[].projected.sources[].downwardAPI.items[].resourceFieldRef.resource": {
      "type": "string",
      "required": true
    },
    "spec.runtime.volumes[].projected.sources[].podCertificate.certificateChainPath": {
      "type": "string"
    },
    "spec.runtime.volumes[].projected.sources[].podCertificate.credentialBundlePath": {
      "type": "string"
    },
    "spec.runtime.volumes[].projected.sources[].podCertificate.keyPath": {
      "type": "string"
    },
    "spec.runtime.volumes[].projected.sources[].podCertificate.keyType": {
      "type": "string",
      "required": true
    },
    "spec.runtime.volumes[].projected.sources[].podCertificate.maxExpirationSeconds": {
      "type": "integer"
    },
    "spec.runtime.volumes[].projected.sources[].podCertificate.signerName": {
      "type": "string",
      "required": true
    },
    "spec.runtime.volumes[].projected.sources[].secret.items[].key": {
      "type": "string",
      "required": true
    },
    "spec.runtime.volumes[].projected.sources[].secret.items[].mode": {
      "type": "integer"
    },
    "spec.runtime.volumes[].projected.sources[].secret.items[].path": {
      "type": "string",
      "required": true
    },
    "spec.runtime.volumes[].projected.sources[].secret.name": {
      "type": "string"
    },
    "spec.runtime.volumes[].projected.sources[].secret.optional": {
      "type": "boolean"
    },
    "spec.runtime.volumes[].projected.sources[].serviceAccountToken.audience": {
      "type": "string"
    },
    "spec.runtime.volumes[].projected.sources[].serviceAccountToken.expirationSeconds": {
      "type": "integer"
    },
    "spec.runtime.volumes[].projected.sources[].serviceAccountToken.path": {
      "type": "string",
      "required": true
    },
    "spec.runtime.volumes[].quobyte.group": {
      "type": "string"
    },
    "spec.runtime.volumes[].quobyte.readOnly": {
      "type": "boolean"
    },
    "spec.runtime.volumes[].quobyte.registry": {
      "type": "string",
      "required": true
    },
    "spec.runtime.volumes[].quobyte.tenant": {
      "type": "string"
    },
    "spec.runtime.volumes[].quobyte.user": {
      "type": "string"
    },
    "spec.runtime.volumes[].quobyte.volume": {
      "type": "string",
      "required": true
    },
    "spec.runtime.volumes[].rbd.fsType": {
      "type": "string"
    },
    "spec.runtime.volumes[].rbd.image": {
      "type": "string",
      "required": true
    },
    "spec.runtime.volumes[].rbd.keyring": {
      "type": "string"
    },
    "spec.runtime.volumes[].rbd.monitors": {
      "required": true
    },
    "spec.runtime.volumes[].rbd.monitors[]": {
      "type": "string"
    },
    "spec.runtime.volumes[].rbd.pool": {
      "type": "string"
    },
    "spec.runtime.volumes[].rbd.readOnly": {
      "type": "boolean"
    },
    "spec.runtime.volumes[].rbd.secretRef.name": {
      "type": "string"
    },
    "spec.runtime.volumes[].rbd.user": {
      "type": "string"
    },
    "spec.runtime.volumes[].scaleIO.fsType": {
      "type": "string"
    },
    "spec.runtime.volumes[].scaleIO.gateway": {
      "type": "string",
      "required": true
    },
    "spec.runtime.volumes[].scaleIO.protectionDomain": {
      "type": "string"
    },
    "spec.runtime.volumes[].scaleIO.readOnly": {
      "type": "boolean"
    },
    "spec.runtime.volumes[].scaleIO.secretRef": {
      "required": true
    },
    "spec.runtime.volumes[].scaleIO.secretRef.name": {
      "type": "string"
    },
    "spec.runtime.volumes[].scaleIO.sslEnabled": {
      "type": "boolean"
    },
    "spec.runtime.volumes[].scaleIO.storageMode": {
      "type": "string"
    },
    "spec.runtime.volumes[].scaleIO.storagePool": {
      "type": "string"
    },
    "spec.runtime.volumes[].scaleIO.system": {
      "type": "string",
      "required": true
    },
    "spec.runtime.volumes[].scaleIO.volumeName": {
      "type": "string"
    },
    "spec.runtime.volumes[].secret.defaultMode": {
      "type": "integer"
    },
    "spec.runtime.volumes[].secret.items[].key": {
      "type": "string",
      "required": true
    },
    "spec.runtime.volumes[].secret.items[].mode": {
      "type": "integer"
    },
    "spec.runtime.volumes[].secret.items[].path": {
      "type": "string",
      "required": true
    },
    "spec.runtime.volumes[].secret.optional": {
      "type": "boolean"
    },
    "spec.runtime.volumes[].secret.secretName": {
      "type": "string"
    },
    "spec.runtime.volumes[].storageos.fsType": {
      "type": "string"
    },
    "spec.runtime.volumes[].storageos.readOnly": {
      "type": "boolean"
    },
    "spec.runtime.volumes[].storageos.secretRef.name": {
      "type": "string"
    },
    "spec.runtime.volumes[].storageos.volumeName": {
      "type": "string"
    },
    "spec.runtime.volumes[].storageos.volumeNamespace": {
      "type": "string"
    },
    "spec.runtime.volumes[].vsphereVolume.fsType": {
      "type": "string"
    },
    "spec.runtime.volumes[].vsphereVolume.storagePolicyID": {
      "type": "string"
    },
    "spec.runtime.volumes[].vsphereVolume.storagePolicyName": {
      "type": "string"
    },
    "spec.runtime.volumes[].vsphereVolume.volumePath": {
      "type": "string",
      "required": true
    },
    "spec.serviceGroup": {
      "type": "string",
      "pattern": "^[a-z0-9]([a-z0-9-]*[a-z0-9])?$",
      "maxLength": 63
    },
    "spec.toolRegistryRef.name": {
      "type": "string",
      "minLength": 1,
      "required": true
    },
    "spec.toolRegistryRef.namespace": {
      "type": "string"
    }
  },
  "PromptPack": {
    "spec.skills[].include[]": {
      "type": "string"
    },
    "spec.skills[].mountAs": {
      "type": "string",
      "pattern": "^[a-z0-9]([a-z0-9-]*[a-z0-9])?$"
    },
    "spec.skills[].source": {
      "type": "string",
      "minLength": 1,
      "required": true
    },
    "spec.skillsConfig.maxActive": {
      "type": "integer",
      "minimum": 1
    },
    "spec.skillsConfig.selector": {
      "type": "string",
      "enum": [
        "model-driven",
        "tag",
        "embedding"
      ]
    },
    "spec.source": {
      "required": true
    },
    "spec.source.configMapRef.name": {
      "type": "string"
    },
    "spec.source.type": {
      "type": "string",
      "enum": [
        "configmap"
      ],
      "required": true
    },
    "spec.version": {
      "type": "string",
      "pattern": "^v?(\\d+)\\.(\\d+)\\.(\\d+)(-[a-zA-Z0-9]+(\\.[a-zA-Z0-9]+)*)?(\\+[a-zA-Z0-9]+(\\.[a-zA-Z0-9]+)*)?$",
      "required": true
    }
  },
  "ToolRegistry": {
    "spec.handlers": {
      "required": true
    },
    "spec.handlers[].clientConfig.categories[]": {
      "type": "string"
    },
    "spec.handlers[].clientConfig.consentMessage": {
      "type": "string"
    },
    "spec.handlers[].grpcConfig.endpoint": {
      "type": "string",
      "required": true
    },
    "spec.handlers[].grpcConfig.retryPolicy.backoffMultiplier": {
      "type": "string",
      "pattern": "^[0-9]+(\\.[0-9]+)?$"
    },
    "spec.handlers[].grpcConfig.retryPolicy.initialBackoff": {
      "type": "string"
    },
    "spec.handlers[].grpcConfig.retryPolicy.maxAttempts": {
      "type": "integer",
      "minimum": 1,
      "maximum": 10,
      "required": true
    },
    "spec.handlers[].grpcConfig.retryPolicy.maxBackoff": {
      "type": "string"
    },
    "spec.handlers[].grpcConfig.retryPolicy.retryableStatusCodes[]": {
      "type": "string"
    },
    "spec.handlers[].grpcConfig.tls": {
      "type": "boolean"
    },
    "spec.handlers[].grpcConfig.tlsCAPath": {
      "type": "string"
    },
    "spec.handlers[].grpcConfig.tlsCertPath": {
      "type": "string"
    },
    "spec.handlers[].grpcConfig.tlsInsecureSkipVerify": {
      "type": "boolean"
    },
    "spec.handlers[].grpcConfig.tlsKeyPath": {
      "type": "string"
    },
    "spec.handlers[].httpConfig.authSecretRef.key": {
      "type": "string",
      "required": true
    },
    "spec.handlers[].httpConfig.authSecretRef.name": {
      "type": "string",
      "required": true
    },
    "spec.handlers[].httpConfig.authType": {
      "type": "string"
    },
    "spec.handlers[].httpConfig.bodyMapping": {
      "type": "string"
    },
    "spec.handlers[].httpConfig.contentType": {
      "type": "string"
    },
    "spec.handlers[].httpConfig.endpoint": {
      "type": "string",
      "required": true
    },
    "spec.handlers[].httpConfig.method": {
      "type": "string"
    },
    "spec.handlers[].httpConfig.queryParams[]": {
      "type": "string"
    },
    "spec.handlers[].httpConfig.redact[]": {
      "type": "string"
    },
    "spec.handlers[].httpConfig.responseMapping": {
      "type": "string"
    },
    "spec.handlers[].httpConfig.retryPolicy.backoffMultiplier": {
      "type": "string",
      "pattern": "^[0-9]+(\\.[0-9]+)?$"
    },
    "spec.handlers[].httpConfig.retryPolicy.initialBackoff": {
      "type": "string"
    },
    "spec.handlers[].httpConfig.retryPolicy.maxAttempts": {
      "type": "integer",
      "minimum": 1,
      "maximum": 10,
      "required": true
    },
    "spec.handlers[].httpConfig.retryPolicy.maxBackoff": {
      "type": "string"
    },
    "spec.handlers[].httpConfig.retryPolicy.respectRetryAfter": {
      "type": "boolean"
    },
    "spec.handlers[].httpConfig.retryPolicy.retryOn[]": {
      "type": "integer"
    },
    "spec.handlers[].httpConfig.retryPolicy.retryOnNetworkError": {
      "type": "boolean"
    },
    "spec.handlers[].httpConfig.urlTemplate": {
      "type": "string"
    },
    "spec.handlers[].mcpConfig.args[]": {
      "type": "string"
    },
    "spec.handlers[].mcpConfig.command": {
      "type": "string"
    },
    "spec.handlers[].mcpConfig.endpoint": {
      "type": "string"
    },
    "spec.handlers[].mcpConfig.retryPolicy.backoffMultiplier": {
      "type": "string",
      "pattern": "^[0-9]+(\\.[0-9]+)?$"
    },
    "spec.handlers[].mcpConfig.retryPolicy.initialBackoff": {
      "type": "string"
    },
    "spec.handlers[].mcpConfig.retryPolicy.maxAttempts": {
      "type": "integer",
      "minimum": 1,
      "maximum": 10,
      "required": true
    },
    "spec.handlers[].mcpConfig.retryPolicy.maxBackoff": {
      "type": "string"
    },
    "spec.handlers[].mcpConfig.toolFilter.allowlist[]": {
      "type": "string"
    },
    "spec.handlers[].mcpConfig.toolFilter.blocklist[]": {
      "type": "string"
    },
    "spec.handlers[].mcpConfig.transport": {
      "type": "string",
      "enum": [
        "sse",
        "stdio",
        "streamable-http"
      ],
      "required": true
    },
    "spec.handlers[].mcpConfig.workDir": {
      "type": "string"
    },
    "spec.handlers[].name": {
      "type": "string",
      "pattern": "^[a-z0-9]([-a-z0-9]*[a-z0-9])?$",
      "maxLength": 63,
      "required": true
    },
    "spec.handlers[].openAPIConfig.authSecretRef.key": {
      "type": "string",
      "required": true
    },
    "spec.handlers[].openAPIConfig.authSecretRef.name": {
      "type": "string",
      "required": true
    },
    "spec.handlers[].openAPIConfig.authType": {
      "type": "string"
    },
    "spec.handlers[].openAPIConfig.baseURL": {
      "type": "string"
    },
    "spec.handlers[].openAPIConfig.operationFilter[]": {
      "type": "string"
    },
    "spec.handlers[].openAPIConfig.retryPolicy.backoffMultiplier": {
      "type": "string",
      "pattern": "^[0-9]+(\\.[0-9]+)?$"
    },
    "spec.handlers[].openAPIConfig.retryPolicy.initialBackoff": {
      "type": "string"
    },
    "spec.handlers[].openAPIConfig.retryPolicy.maxAttempts": {
      "type": "integer",
      "minimum": 1,
      "maximum": 10,
      "required": true
    },
    "spec.handlers[].openAPIConfig.retryPolicy.maxBackoff": {
      "type": "string"
    },
    "spec.handlers[].openAPIConfig.retryPolicy.respectRetryAfter": {
      "type": "boolean"
    },
    "spec.handlers[].openAPIConfig.retryPolicy.retryOn[]": {
      "type": "integer"
    },
    "spec.handlers[].openAPIConfig.retryPolicy.retryOnNetworkError": {
      "type": "boolean"
    },
    "spec.handlers[].openAPIConfig.specURL": {
      "type": "string",
      "required": true
    },
    "spec.handlers[].selector.namespace": {
      "type": "string"
    },
    "spec.handlers[].selector.port": {
      "type": "string"
    },
    "spec.handlers[].timeout": {
      "type": "string"
    },
    "spec.handlers[].tool.description": {
      "type": "string",
      "required": true
    },
    "spec.handlers[].tool.inputSchema": {
      "required": true
    },
    "spec.handlers[].tool.name": {
      "type": "string",
      "pattern": "^[a-z][a-z0-9_]*$",
      "maxLength": 64,
      "required": true
    },
    "spec.handlers[].type": {
      "type": "string",
      "enum": [
        "http",
        "openapi",
        "grpc",
        "mcp",
        "client"
      ],
      "required": true
    }
  },
  "Provider": {
    "spec.auth.credentialsSecretRef.key": {
      "type": "string"
    },
    "spec.auth.credentialsSecretRef.name": {
      "type": "string",
      "minLength": 1,
      "required": true
    },
    "spec.auth.roleArn": {
      "type": "string"
    },
    "spec.auth.serviceAccountEmail": {
      "type": "string"
    },
    "spec.auth.type": {
      "type": "string",
      "enum": [
        "workloadIdentity",
        "accessKey",
        "serviceAccount",
        "servicePrincipal"
      ],
      "required": true
    },
    "spec.baseURL": {
      "type": "string"
    },
    "spec.capabilities[]": {
      "type": "string",
      "enum": [
        "text",
        "streaming",
        "vision",
        "tools",
        "json",
        "audio",
        "video",
        "documents",
        "duplex"
      ]
    },
    "spec.credential.envVar": {
      "type": "string",
      "pattern": "^[A-Za-z_][A-Za-z0-9_]*$"
    },
    "spec.credential.filePath": {
      "type": "string",
      "pattern": "^/.*"
    },
    "spec.credential.secretRef.key": {
      "type": "string"
    },
    "spec.credential.secretRef.name": {
      "type": "string",
      "minLength": 1,
      "required": true
    },
    "spec.defaults.contextWindow": {
      "type": "integer"
    },
    "spec.defaults.maxTokens": {
      "type": "integer"
    },
    "spec.defaults.requestTimeout": {
      "type": "string"
    },
    "spec.defaults.streamIdleTimeout": {
      "type": "string"
    },
    "spec.defaults.temperature": {
      "type": "string"
    },
    "spec.defaults.topP": {
      "type": "string"
    },
    "spec.defaults.truncationStrategy": {
      "type": "string",
      "enum": [
        "sliding",
        "summarize",
        "custom"
      ]
    },
    "spec.embedding.dimensions": {
      "type": "integer",
      "minimum": 1,
      "maximum": 4096
    },
    "spec.embedding.distance": {
      "type": "string",
      "enum": [
        "cosine",
        "l2",
        "dot"
      ]
    },
    "spec.model": {
      "type": "string"
    },
    "spec.platform.endpoint": {
      "type": "string"
    },
    "spec.platform.project": {
      "type": "string"
    },
    "spec.platform.region": {
      "type": "string"
    },
    "spec.platform.type": {
      "type": "string",
      "enum": [
        "bedrock",
        "vertex",
        "azure"
      ],
      "required": true
    },
    "spec.pricing.cachedCostPer1K": {
      "type": "string"
    },
    "spec.pricing.inputCostPer1K": {
      "type": "string"
    },
    "spec.pricing.outputCostPer1K": {
      "type": "string"
    },
    "spec.role": {
      "type": "string",
      "enum": [
        "llm",
        "embedding",
        "tts",
        "stt",
        "image",
        "inference"
      ]
    },
    "spec.stt.language": {
      "type": "string",
      "pattern": "^[a-z]{2}(-[A-Z]{2})?$"
    },
    "spec.stt.sampleRate": {
      "type": "integer",
      "minimum": 8000,
      "maximum": 48000
    },
    "spec.tts.audioFiles[]": {
      "type": "string"
    },
    "spec.tts.format": {
      "type": "string",
      "enum": [
        "pcm",
        "mp3",
        "opus",
        "wav",
        "flac"
      ]
    },
    "spec.tts.sampleRate": {
      "type": "integer",
      "minimum": 8000,
      "maximum": 48000
    },
    "spec.tts.voice": {
      "type": "string"
    },
    "spec.type": {
      "type": "string",
      "enum": [
        "claude",
        "openai",
        "gemini",
        "ollama",
        "mock",
        "vllm",
        "voyageai",
        "cartesia",
        "elevenlabs",
        "imagen",
        "huggingface"
      ],
      "required": true
    }
  },
  "SessionRetentionPolicy": {
    "spec.coldArchive.compactionSchedule": {
      "type": "string"
    },
    "spec.coldArchive.enabled": {
      "type": "boolean"
    },
    "spec.coldArchive.retentionDays": {
      "type": "integer",
      "minimum": 1,
      "maximum": 36500
    },
    "spec.hotCache.enabled": {
      "type": "boolean"
    },
    "spec.hotCache.maxMessagesPerSession": {
      "type": "integer",
      "minimum": 1
    },
    "spec.hotCache.maxSessions": {
      "type": "integer",
      "minimum": 1
    },
    "spec.hotCache.ttlAfterInactive": {
      "type": "string",
      "pattern": "^([0-9]+h)?([0-9]+m)?([0-9]+s)?$"
    },
    "spec.warmStore.partitionBy": {
      "type": "string",
      "enum": [
        "week"
      ]
    },
    "spec.warmStore.retentionDays": {
      "type": "integer",
      "minimum": 1,
      "maximum": 3650
    }
  },
  "SkillSource": {
    "spec.configMap.key": {
      "type": "string"
    },
    "spec.configMap.name": {
      "type": "string",
      "minLength": 1,
      "required": true
    },
    "spec.createVersionOnSync": {
      "type": "boolean"
    },
    "spec.filter.exclude[]": {
      "type": "string"
    },
    "spec.filter.include[]": {
      "type": "string"
    },
    "spec.filter.names[]": {
      "type": "string"
    },
    "spec.git.path": {
      "type": "string"
    },
    "spec.git.ref.branch": {
      "type": "string"
    },
    "spec.git.ref.commit": {
      "type": "string"
    },
    "spec.git.ref.tag": {
      "type": "string"
    },
    "spec.git.secretRef.key": {
      "type": "string"
    },
    "spec.git.secretRef.name": {
      "type": "string",
      "minLength": 1,
      "required": true
    },
    "spec.git.url": {
      "type": "string",
      "pattern": "^(https?|ssh)://.*$",
      "required": true
    },
    "spec.interval": {
      "type": "string",
      "pattern": "^([0-9]+(\\.[0-9]+)?(ms|s|m|h))+$",
      "required": true
    },
    "spec.oci.insecure": {
      "type": "boolean"
    },
    "spec.oci.secretRef.key": {
      "type": "string"
    },
    "spec.oci.secretRef.name": {
      "type": "string",
      "minLength": 1,
      "required": true
    },
    "spec.oci.url": {
      "type": "string",
      "pattern": "^oci://.*$",
      "required": true
    },
    "spec.suspend": {
      "type": "boolean"
    },
    "spec.targetPath": {
      "type": "string"
    },
    "spec.timeout": {
      "type": "string"
    },
    "spec.type": {
      "type": "string",
      "enum": [
        "git",
        "oci",
        "configmap"
      ],
      "required": true
    }
  }
};
