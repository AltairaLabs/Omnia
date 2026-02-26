/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package controller

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/altairalabs/omnia/ee/pkg/arena/fetcher"
)

// LoadGitCredentials loads Git credentials from a Kubernetes Secret.
// It supports both HTTPS (username/password) and SSH (identity/known_hosts) credentials.
func LoadGitCredentials(ctx context.Context, c client.Reader, namespace, secretName string) (*fetcher.GitCredentials, error) {
	secret := &corev1.Secret{}
	if err := c.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, secret); err != nil {
		return nil, err
	}

	creds := &fetcher.GitCredentials{}

	// HTTPS credentials
	if username, ok := secret.Data["username"]; ok {
		creds.Username = string(username)
	}
	if password, ok := secret.Data["password"]; ok {
		creds.Password = string(password)
	}

	// SSH credentials
	if identity, ok := secret.Data["identity"]; ok {
		creds.PrivateKey = identity
	}
	if knownHosts, ok := secret.Data["known_hosts"]; ok {
		creds.KnownHosts = knownHosts
	}

	return creds, nil
}

// LoadOCICredentials loads OCI registry credentials from a Kubernetes Secret.
// It supports basic auth (username/password) and Docker config (.dockerconfigjson) credentials.
func LoadOCICredentials(ctx context.Context, c client.Reader, namespace, secretName string) (*fetcher.OCICredentials, error) {
	secret := &corev1.Secret{}
	if err := c.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, secret); err != nil {
		return nil, err
	}

	creds := &fetcher.OCICredentials{}

	// Basic auth credentials
	if username, ok := secret.Data["username"]; ok {
		creds.Username = string(username)
	}
	if password, ok := secret.Data["password"]; ok {
		creds.Password = string(password)
	}

	// Docker config
	if dockerConfig, ok := secret.Data[".dockerconfigjson"]; ok {
		creds.DockerConfig = dockerConfig
	}

	return creds, nil
}
