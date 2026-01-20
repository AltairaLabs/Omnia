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
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewActivationClient(t *testing.T) {
	t.Run("uses default values", func(t *testing.T) {
		client := NewActivationClient()
		assert.Equal(t, DefaultActivationServerURL, client.serverURL)
		assert.NotNil(t, client.httpClient)
	})

	t.Run("applies custom server URL", func(t *testing.T) {
		customURL := "https://custom.example.com"
		client := NewActivationClient(WithServerURL(customURL))
		assert.Equal(t, customURL, client.serverURL)
	})

	t.Run("applies custom HTTP client", func(t *testing.T) {
		customClient := &http.Client{Timeout: 60 * time.Second}
		client := NewActivationClient(WithHTTPClient(customClient))
		assert.Equal(t, customClient, client.httpClient)
	})
}

func TestActivationClient_Activate(t *testing.T) {
	ctx := context.Background()

	t.Run("successful activation", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/v1/licenses/activate", r.URL.Path)
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

			var req ActivationRequest
			err := json.NewDecoder(r.Body).Decode(&req)
			require.NoError(t, err)
			assert.Equal(t, "lic_123", req.LicenseID)
			assert.Equal(t, "fp_abc", req.ClusterFingerprint)

			resp := ActivationResponse{
				Activated:    true,
				ActivationID: "act_xyz",
				Message:      "Cluster activated successfully",
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := NewActivationClient(WithServerURL(server.URL))
		resp, err := client.Activate(ctx, ActivationRequest{
			LicenseID:          "lic_123",
			ClusterFingerprint: "fp_abc",
			ClusterName:        "test-cluster",
			Version:            "1.0.0",
		})

		require.NoError(t, err)
		assert.True(t, resp.Activated)
		assert.Equal(t, "act_xyz", resp.ActivationID)
	})

	t.Run("activation rejected - limit reached", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := ActivationResponse{
				Activated:      false,
				Message:        "Maximum activations reached",
				ActiveClusters: []string{"fp_1", "fp_2", "fp_3"},
				MaxActivations: 3,
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := NewActivationClient(WithServerURL(server.URL))
		resp, err := client.Activate(ctx, ActivationRequest{
			LicenseID:          "lic_123",
			ClusterFingerprint: "fp_new",
		})

		require.NoError(t, err)
		assert.False(t, resp.Activated)
		assert.Contains(t, resp.Message, "Maximum activations")
		assert.Len(t, resp.ActiveClusters, 3)
	})

	t.Run("server error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "internal error"})
		}))
		defer server.Close()

		client := NewActivationClient(WithServerURL(server.URL))
		_, err := client.Activate(ctx, ActivationRequest{
			LicenseID:          "lic_123",
			ClusterFingerprint: "fp_abc",
		})

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "internal error")
	})

	t.Run("network error", func(t *testing.T) {
		client := NewActivationClient(WithServerURL("http://localhost:99999"))
		_, err := client.Activate(ctx, ActivationRequest{
			LicenseID:          "lic_123",
			ClusterFingerprint: "fp_abc",
		})

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "activation failed")
	})
}

func TestActivationClient_Deactivate(t *testing.T) {
	ctx := context.Background()

	t.Run("successful deactivation", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/v1/licenses/lic_123/activations/fp_abc", r.URL.Path)
			assert.Equal(t, http.MethodDelete, r.Method)

			resp := DeactivationResponse{
				Deactivated: true,
				Message:     "Cluster deactivated successfully",
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := NewActivationClient(WithServerURL(server.URL))
		resp, err := client.Deactivate(ctx, "lic_123", "fp_abc")

		require.NoError(t, err)
		assert.True(t, resp.Deactivated)
	})

	t.Run("deactivation failed - not found", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "activation not found"})
		}))
		defer server.Close()

		client := NewActivationClient(WithServerURL(server.URL))
		_, err := client.Deactivate(ctx, "lic_123", "fp_unknown")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "deactivation failed")
	})
}

func TestActivationClient_Heartbeat(t *testing.T) {
	ctx := context.Background()

	t.Run("successful heartbeat", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/v1/licenses/lic_123/heartbeat", r.URL.Path)
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

			var req HeartbeatRequest
			err := json.NewDecoder(r.Body).Decode(&req)
			require.NoError(t, err)
			assert.Equal(t, "fp_abc", req.ClusterFingerprint)

			expiry := time.Now().Add(30 * 24 * time.Hour)
			resp := HeartbeatResponse{
				Valid:         true,
				Message:       "License valid",
				LicenseExpiry: &expiry,
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := NewActivationClient(WithServerURL(server.URL))
		resp, err := client.Heartbeat(ctx, "lic_123", HeartbeatRequest{
			ClusterFingerprint: "fp_abc",
			Version:            "1.0.0",
			ActiveJobs:         5,
			WorkerCount:        10,
		})

		require.NoError(t, err)
		assert.True(t, resp.Valid)
		assert.NotNil(t, resp.LicenseExpiry)
	})

	t.Run("heartbeat with invalid license", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "license expired"})
		}))
		defer server.Close()

		client := NewActivationClient(WithServerURL(server.URL))
		_, err := client.Heartbeat(ctx, "lic_123", HeartbeatRequest{
			ClusterFingerprint: "fp_abc",
		})

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "license expired")
	})
}

func TestActivationClient_GetActivations(t *testing.T) {
	ctx := context.Background()

	t.Run("successful get activations", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/v1/licenses/lic_123/activations", r.URL.Path)
			assert.Equal(t, http.MethodGet, r.Method)

			activations := []ActivationInfo{
				{
					ActivationID:       "act_1",
					ClusterFingerprint: "fp_1",
					ClusterName:        "cluster-1",
					ActivatedAt:        time.Now().Add(-24 * time.Hour),
					LastHeartbeat:      time.Now().Add(-1 * time.Hour),
					Version:            "1.0.0",
				},
				{
					ActivationID:       "act_2",
					ClusterFingerprint: "fp_2",
					ClusterName:        "cluster-2",
					ActivatedAt:        time.Now().Add(-48 * time.Hour),
					LastHeartbeat:      time.Now().Add(-2 * time.Hour),
					Version:            "1.0.1",
				},
			}

			resp := struct {
				Activations []ActivationInfo `json:"activations"`
			}{
				Activations: activations,
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := NewActivationClient(WithServerURL(server.URL))
		activations, err := client.GetActivations(ctx, "lic_123")

		require.NoError(t, err)
		assert.Len(t, activations, 2)
		assert.Equal(t, "act_1", activations[0].ActivationID)
		assert.Equal(t, "cluster-1", activations[0].ClusterName)
	})

	t.Run("license not found", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		client := NewActivationClient(WithServerURL(server.URL))
		_, err := client.GetActivations(ctx, "lic_unknown")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "status 404")
	})
}

func TestActivationState_NeedsHeartbeat(t *testing.T) {
	t.Run("needs heartbeat after interval", func(t *testing.T) {
		state := &ActivationState{
			LastHeartbeat: time.Now().Add(-25 * time.Hour),
		}
		assert.True(t, state.NeedsHeartbeat(24*time.Hour))
	})

	t.Run("does not need heartbeat before interval", func(t *testing.T) {
		state := &ActivationState{
			LastHeartbeat: time.Now().Add(-23 * time.Hour),
		}
		assert.False(t, state.NeedsHeartbeat(24*time.Hour))
	})

	t.Run("needs heartbeat exactly at interval", func(t *testing.T) {
		state := &ActivationState{
			LastHeartbeat: time.Now().Add(-24 * time.Hour),
		}
		assert.True(t, state.NeedsHeartbeat(24*time.Hour))
	})
}

func TestActivationClient_Activate_InvalidJSON(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("invalid json"))
	}))
	defer server.Close()

	client := NewActivationClient(WithServerURL(server.URL))
	_, err := client.Activate(ctx, ActivationRequest{
		LicenseID:          "lic_123",
		ClusterFingerprint: "fp_abc",
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse activation response")
}

func TestActivationClient_Deactivate_InvalidJSON(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("invalid json"))
	}))
	defer server.Close()

	client := NewActivationClient(WithServerURL(server.URL))
	_, err := client.Deactivate(ctx, "lic_123", "fp_abc")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse deactivation response")
}

func TestActivationClient_Heartbeat_InvalidJSON(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("invalid json"))
	}))
	defer server.Close()

	client := NewActivationClient(WithServerURL(server.URL))
	_, err := client.Heartbeat(ctx, "lic_123", HeartbeatRequest{
		ClusterFingerprint: "fp_abc",
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse heartbeat response")
}

func TestActivationClient_GetActivations_InvalidJSON(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("invalid json"))
	}))
	defer server.Close()

	client := NewActivationClient(WithServerURL(server.URL))
	_, err := client.GetActivations(ctx, "lic_123")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse activations response")
}

func TestActivationClient_Activate_ServerErrorNoBody(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewActivationClient(WithServerURL(server.URL))
	_, err := client.Activate(ctx, ActivationRequest{
		LicenseID:          "lic_123",
		ClusterFingerprint: "fp_abc",
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "activation failed")
	assert.Contains(t, err.Error(), "status 500")
}

func TestActivationClient_Deactivate_ServerErrorWithMessage(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "not authorized"})
	}))
	defer server.Close()

	client := NewActivationClient(WithServerURL(server.URL))
	_, err := client.Deactivate(ctx, "lic_123", "fp_abc")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not authorized")
}

func TestActivationClient_Heartbeat_ServerErrorWithMessage(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "license revoked"})
	}))
	defer server.Close()

	client := NewActivationClient(WithServerURL(server.URL))
	_, err := client.Heartbeat(ctx, "lic_123", HeartbeatRequest{
		ClusterFingerprint: "fp_abc",
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "license revoked")
}

func TestActivationState_IsInGracePeriod(t *testing.T) {
	t.Run("in grace period with no failures", func(t *testing.T) {
		state := &ActivationState{
			HeartbeatFailures: 0,
			LastHeartbeat:     time.Now().Add(-48 * time.Hour),
		}
		assert.True(t, state.IsInGracePeriod())
	})

	t.Run("in grace period with recent failure", func(t *testing.T) {
		state := &ActivationState{
			HeartbeatFailures: 1,
			LastHeartbeat:     time.Now().Add(-24 * time.Hour),
		}
		assert.True(t, state.IsInGracePeriod())
	})

	t.Run("outside grace period", func(t *testing.T) {
		state := &ActivationState{
			HeartbeatFailures: 5,
			LastHeartbeat:     time.Now().Add(-8 * 24 * time.Hour), // 8 days ago
		}
		assert.False(t, state.IsInGracePeriod())
	})

	t.Run("exactly at grace period boundary", func(t *testing.T) {
		state := &ActivationState{
			HeartbeatFailures: 1,
			LastHeartbeat:     time.Now().Add(-HeartbeatGracePeriod),
		}
		// At exactly the boundary, we should be outside grace period
		assert.False(t, state.IsInGracePeriod())
	})
}
