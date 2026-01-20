/*
Copyright 2025.

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

package license

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Default configuration values for activation.
const (
	// DefaultActivationServerURL is the default license activation server URL.
	DefaultActivationServerURL = "https://license.altairalabs.ai"
	// DefaultHeartbeatInterval is the default interval between heartbeats.
	DefaultHeartbeatInterval = 24 * time.Hour
	// DefaultHTTPTimeout is the default timeout for HTTP requests.
	DefaultHTTPTimeout = 30 * time.Second
	// ActivationConfigMapName is the name of the ConfigMap storing activation state.
	ActivationConfigMapName = "arena-license-activation"
)

// ActivationRequest contains the data sent when activating a license on a cluster.
type ActivationRequest struct {
	// LicenseID is the unique identifier of the license being activated.
	LicenseID string `json:"license_id"`
	// ClusterFingerprint is the unique identifier for this Kubernetes cluster.
	ClusterFingerprint string `json:"cluster_fingerprint"`
	// ClusterName is an optional user-friendly name for the cluster.
	ClusterName string `json:"cluster_name,omitempty"`
	// Version is the Omnia version running on this cluster.
	Version string `json:"version,omitempty"`
}

// ActivationResponse contains the response from the activation server.
type ActivationResponse struct {
	// Activated indicates whether the activation was successful.
	Activated bool `json:"activated"`
	// ActivationID is the unique identifier for this activation.
	ActivationID string `json:"activation_id,omitempty"`
	// Message provides additional context about the activation result.
	Message string `json:"message,omitempty"`
	// ActiveClusters lists fingerprints of all clusters with active activations for this license.
	ActiveClusters []string `json:"active_clusters,omitempty"`
	// MaxActivations is the maximum number of allowed activations for this license.
	MaxActivations int `json:"max_activations,omitempty"`
}

// HeartbeatRequest contains the data sent for periodic license validation.
type HeartbeatRequest struct {
	// ClusterFingerprint identifies the cluster sending the heartbeat.
	ClusterFingerprint string `json:"cluster_fingerprint"`
	// Version is the current Omnia version.
	Version string `json:"version,omitempty"`
	// ActiveJobs is the number of currently active ArenaJobs.
	ActiveJobs int `json:"active_jobs,omitempty"`
	// WorkerCount is the total number of worker pods.
	WorkerCount int `json:"worker_count,omitempty"`
}

// HeartbeatResponse contains the response from a heartbeat request.
type HeartbeatResponse struct {
	// Valid indicates whether the license is still valid.
	Valid bool `json:"valid"`
	// Message provides additional context.
	Message string `json:"message,omitempty"`
	// LicenseExpiry is when the license expires.
	LicenseExpiry *time.Time `json:"license_expiry,omitempty"`
}

// DeactivationResponse contains the response from a deactivation request.
type DeactivationResponse struct {
	// Deactivated indicates whether the deactivation was successful.
	Deactivated bool `json:"deactivated"`
	// Message provides additional context.
	Message string `json:"message,omitempty"`
}

// ActivationClient handles communication with the license activation server.
type ActivationClient struct {
	serverURL  string
	httpClient *http.Client
}

// ActivationClientOption configures the ActivationClient.
type ActivationClientOption func(*ActivationClient)

// WithServerURL sets a custom activation server URL.
func WithServerURL(url string) ActivationClientOption {
	return func(c *ActivationClient) {
		c.serverURL = url
	}
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(client *http.Client) ActivationClientOption {
	return func(c *ActivationClient) {
		c.httpClient = client
	}
}

// NewActivationClient creates a new activation client.
func NewActivationClient(opts ...ActivationClientOption) *ActivationClient {
	c := &ActivationClient{
		serverURL: DefaultActivationServerURL,
		httpClient: &http.Client{
			Timeout: DefaultHTTPTimeout,
		},
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// Activate registers this cluster with the license server.
// Returns an error if the activation limit has been reached or the license is invalid.
func (c *ActivationClient) Activate(ctx context.Context, req ActivationRequest) (*ActivationResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal activation request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.serverURL+"/v1/licenses/activate", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create activation request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("activation request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read activation response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(respBody, &errResp); err == nil && errResp.Error != "" {
			return nil, fmt.Errorf("activation failed: %s", errResp.Error)
		}
		return nil, fmt.Errorf("activation failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var activationResp ActivationResponse
	if err := json.Unmarshal(respBody, &activationResp); err != nil {
		return nil, fmt.Errorf("failed to parse activation response: %w", err)
	}

	return &activationResp, nil
}

// Deactivate removes this cluster's activation (for decommissioning).
// This frees up an activation slot for use on another cluster.
func (c *ActivationClient) Deactivate(
	ctx context.Context,
	licenseID, fingerprint string,
) (*DeactivationResponse, error) {
	url := fmt.Sprintf("%s/v1/licenses/%s/activations/%s", c.serverURL, licenseID, fingerprint)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create deactivation request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("deactivation request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read deactivation response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(respBody, &errResp); err == nil && errResp.Error != "" {
			return nil, fmt.Errorf("deactivation failed: %s", errResp.Error)
		}
		return nil, fmt.Errorf("deactivation failed with status %d", resp.StatusCode)
	}

	var deactivationResp DeactivationResponse
	if err := json.Unmarshal(respBody, &deactivationResp); err != nil {
		return nil, fmt.Errorf("failed to parse deactivation response: %w", err)
	}

	return &deactivationResp, nil
}

// Heartbeat sends periodic validation to the license server.
// This keeps the activation alive and allows the server to track active clusters.
func (c *ActivationClient) Heartbeat(
	ctx context.Context,
	licenseID string,
	req HeartbeatRequest,
) (*HeartbeatResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal heartbeat request: %w", err)
	}

	url := fmt.Sprintf("%s/v1/licenses/%s/heartbeat", c.serverURL, licenseID)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create heartbeat request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("heartbeat request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read heartbeat response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(respBody, &errResp); err == nil && errResp.Error != "" {
			return nil, fmt.Errorf("heartbeat failed: %s", errResp.Error)
		}
		return nil, fmt.Errorf("heartbeat failed with status %d", resp.StatusCode)
	}

	var heartbeatResp HeartbeatResponse
	if err := json.Unmarshal(respBody, &heartbeatResp); err != nil {
		return nil, fmt.Errorf("failed to parse heartbeat response: %w", err)
	}

	return &heartbeatResp, nil
}

// GetActivations retrieves the list of active clusters for a license.
func (c *ActivationClient) GetActivations(ctx context.Context, licenseID string) ([]ActivationInfo, error) {
	url := fmt.Sprintf("%s/v1/licenses/%s/activations", c.serverURL, licenseID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create get activations request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get activations request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read activations response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get activations failed with status %d", resp.StatusCode)
	}

	var activations struct {
		Activations []ActivationInfo `json:"activations"`
	}
	if err := json.Unmarshal(respBody, &activations); err != nil {
		return nil, fmt.Errorf("failed to parse activations response: %w", err)
	}

	return activations.Activations, nil
}

// ActivationInfo contains information about a single cluster activation.
type ActivationInfo struct {
	// ActivationID is the unique identifier for this activation.
	ActivationID string `json:"activation_id"`
	// ClusterFingerprint identifies the cluster.
	ClusterFingerprint string `json:"cluster_fingerprint"`
	// ClusterName is the user-friendly name for the cluster.
	ClusterName string `json:"cluster_name,omitempty"`
	// ActivatedAt is when the cluster was activated.
	ActivatedAt time.Time `json:"activated_at"`
	// LastHeartbeat is the time of the last successful heartbeat.
	LastHeartbeat time.Time `json:"last_heartbeat"`
	// Version is the Omnia version running on this cluster.
	Version string `json:"version,omitempty"`
}
