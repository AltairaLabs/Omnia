/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/internal/media"
)

// secret keys expected in a media secretRef.
const (
	mediaSecretKeyS3AccessKeyID     = "AWS_ACCESS_KEY_ID"
	mediaSecretKeyS3SecretAccessKey = "AWS_SECRET_ACCESS_KEY"
	mediaSecretKeyAzureAccountKey   = "AZURE_ACCOUNT_KEY"
)

// mediaStorageEnvVars renders spec.media.storage into the OMNIA_MEDIA_STORAGE_*
// env contract shared by the facade and runtime containers. Returns nil for a
// nil/none config (media storage disabled).
func mediaStorageEnvVars(cfg *omniav1alpha1.MediaStorageConfig) []corev1.EnvVar {
	if cfg == nil || media.BackendType(cfg.Type) == "" || media.BackendType(cfg.Type) == media.BackendTypeNone {
		return nil
	}
	env := []corev1.EnvVar{{Name: media.EnvStorageType, Value: cfg.Type}}
	env = appendMediaLimits(env, cfg)
	switch media.BackendType(cfg.Type) {
	case media.BackendTypeLocal:
		env = appendLocalEnv(env, cfg.Local)
	case media.BackendTypeS3:
		env = appendS3Env(env, cfg.S3, cfg.SecretRef)
	case media.BackendTypeGCS:
		env = appendGCSEnv(env, cfg.GCS)
	case media.BackendTypeAzure:
		env = appendAzureEnv(env, cfg.Azure, cfg.SecretRef)
	}
	return env
}

func plain(name, value string) corev1.EnvVar { return corev1.EnvVar{Name: name, Value: value} }

func secretEnv(name, secretName, key string) corev1.EnvVar {
	return corev1.EnvVar{Name: name, ValueFrom: &corev1.EnvVarSource{
		SecretKeyRef: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
			Key:                  key,
		},
	}}
}

func appendMediaLimits(env []corev1.EnvVar, cfg *omniav1alpha1.MediaStorageConfig) []corev1.EnvVar {
	if cfg.MaxFileSizeBytes != nil {
		env = append(env, plain(media.EnvMaxFileSize, fmt.Sprintf("%d", *cfg.MaxFileSizeBytes)))
	}
	if cfg.DefaultTTL != nil {
		env = append(env, plain(media.EnvDefaultTTL, cfg.DefaultTTL.Duration.String()))
	}
	return env
}

func appendLocalEnv(env []corev1.EnvVar, b *omniav1alpha1.LocalMediaBackend) []corev1.EnvVar {
	if b != nil && b.BasePath != "" {
		env = append(env, plain(media.EnvStoragePath, b.BasePath))
	}
	return env
}

func appendS3Env(env []corev1.EnvVar, b *omniav1alpha1.S3MediaBackend, sec *corev1.LocalObjectReference) []corev1.EnvVar {
	if b == nil {
		return env
	}
	env = append(env, plain(media.EnvS3Bucket, b.Bucket))
	if b.Region != "" {
		env = append(env, plain(media.EnvS3Region, b.Region))
	}
	if b.Prefix != "" {
		env = append(env, plain(media.EnvS3Prefix, b.Prefix))
	}
	if b.Endpoint != "" {
		env = append(env, plain(media.EnvS3Endpoint, b.Endpoint))
	}
	if sec != nil {
		env = append(env,
			secretEnv(mediaSecretKeyS3AccessKeyID, sec.Name, mediaSecretKeyS3AccessKeyID),
			secretEnv(mediaSecretKeyS3SecretAccessKey, sec.Name, mediaSecretKeyS3SecretAccessKey),
		)
	}
	return env
}

func appendGCSEnv(env []corev1.EnvVar, b *omniav1alpha1.GCSMediaBackend) []corev1.EnvVar {
	if b == nil {
		return env
	}
	env = append(env, plain(media.EnvGCSBucket, b.Bucket))
	if b.Prefix != "" {
		env = append(env, plain(media.EnvGCSPrefix, b.Prefix))
	}
	return env
}

func appendAzureEnv(env []corev1.EnvVar, b *omniav1alpha1.AzureMediaBackend, sec *corev1.LocalObjectReference) []corev1.EnvVar {
	if b == nil {
		return env
	}
	env = append(env, plain(media.EnvAzureAccount, b.Account), plain(media.EnvAzureContainer, b.Container))
	if b.Prefix != "" {
		env = append(env, plain(media.EnvAzurePrefix, b.Prefix))
	}
	if sec != nil {
		env = append(env, secretEnv(media.EnvAzureKey, sec.Name, mediaSecretKeyAzureAccountKey))
	}
	return env
}
