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

package agent

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newMetricsWithRegistry creates metrics registered with a custom registry (for testing).
// This avoids conflicts with the global prometheus registry.
//
//nolint:unparam // agentName/namespace are parameterized for consistency with NewMetrics
func newMetricsWithRegistry(agentName, namespace string, reg prometheus.Registerer) *Metrics {
	labels := prometheus.Labels{
		"agent":     agentName,
		"namespace": namespace,
	}

	connectionsActive := prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        "omnia_agent_connections_active",
		Help:        "Current number of active WebSocket connections",
		ConstLabels: labels,
	})
	reg.MustRegister(connectionsActive)

	connectionsTotal := prometheus.NewCounter(prometheus.CounterOpts{
		Name:        "omnia_agent_connections_total",
		Help:        "Total number of WebSocket connections since startup",
		ConstLabels: labels,
	})
	reg.MustRegister(connectionsTotal)

	sessionsActive := prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        "omnia_agent_sessions_active",
		Help:        "Current number of active sessions",
		ConstLabels: labels,
	})
	reg.MustRegister(sessionsActive)

	requestsInflight := prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        "omnia_agent_requests_inflight",
		Help:        "Current number of requests being processed",
		ConstLabels: labels,
	})
	reg.MustRegister(requestsInflight)

	requestsTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name:        "omnia_agent_requests_total",
		Help:        "Total number of requests processed",
		ConstLabels: labels,
	}, []string{"status"})
	reg.MustRegister(requestsTotal)

	requestDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:        "omnia_agent_request_duration_seconds",
		Help:        "Request processing duration in seconds",
		ConstLabels: labels,
		Buckets:     []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60, 120},
	}, []string{"handler"})
	reg.MustRegister(requestDuration)

	messagesReceived := prometheus.NewCounter(prometheus.CounterOpts{
		Name:        "omnia_agent_messages_received_total",
		Help:        "Total number of WebSocket messages received",
		ConstLabels: labels,
	})
	reg.MustRegister(messagesReceived)

	messagesSent := prometheus.NewCounter(prometheus.CounterOpts{
		Name:        "omnia_agent_messages_sent_total",
		Help:        "Total number of WebSocket messages sent",
		ConstLabels: labels,
	})
	reg.MustRegister(messagesSent)

	// Media metrics
	uploadsTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name:        "omnia_facade_uploads_total",
		Help:        "Total number of media upload attempts",
		ConstLabels: labels,
	}, []string{"status"})
	reg.MustRegister(uploadsTotal)

	uploadBytesTotal := prometheus.NewCounter(prometheus.CounterOpts{
		Name:        "omnia_facade_upload_bytes_total",
		Help:        "Total bytes uploaded",
		ConstLabels: labels,
	})
	reg.MustRegister(uploadBytesTotal)

	uploadDuration := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:        "omnia_facade_upload_duration_seconds",
		Help:        "Upload duration in seconds",
		ConstLabels: labels,
		Buckets:     []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60, 120},
	})
	reg.MustRegister(uploadDuration)

	downloadsTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name:        "omnia_facade_downloads_total",
		Help:        "Total number of media download attempts",
		ConstLabels: labels,
	}, []string{"status"})
	reg.MustRegister(downloadsTotal)

	downloadBytesTotal := prometheus.NewCounter(prometheus.CounterOpts{
		Name:        "omnia_facade_download_bytes_total",
		Help:        "Total bytes downloaded",
		ConstLabels: labels,
	})
	reg.MustRegister(downloadBytesTotal)

	mediaChunksTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name:        "omnia_facade_media_chunks_total",
		Help:        "Total number of media chunks sent",
		ConstLabels: labels,
	}, []string{"type"})
	reg.MustRegister(mediaChunksTotal)

	mediaChunkBytesTotal := prometheus.NewCounter(prometheus.CounterOpts{
		Name:        "omnia_facade_media_chunk_bytes_total",
		Help:        "Total bytes sent as media chunks",
		ConstLabels: labels,
	})
	reg.MustRegister(mediaChunkBytesTotal)

	return &Metrics{
		ConnectionsActive:    connectionsActive,
		ConnectionsTotal:     connectionsTotal,
		SessionsActive:       sessionsActive,
		RequestsInflight:     requestsInflight,
		RequestsTotal:        requestsTotal,
		RequestDuration:      requestDuration,
		MessagesReceived:     messagesReceived,
		MessagesSent:         messagesSent,
		UploadsTotal:         uploadsTotal,
		UploadBytesTotal:     uploadBytesTotal,
		UploadDuration:       uploadDuration,
		DownloadsTotal:       downloadsTotal,
		DownloadBytesTotal:   downloadBytesTotal,
		MediaChunksTotal:     mediaChunksTotal,
		MediaChunkBytesTotal: mediaChunkBytesTotal,
	}
}

func TestNewMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := newMetricsWithRegistry("test-agent", "test-namespace", reg)

	require.NotNil(t, m)
	assert.NotNil(t, m.ConnectionsActive)
	assert.NotNil(t, m.ConnectionsTotal)
	assert.NotNil(t, m.SessionsActive)
	assert.NotNil(t, m.RequestsInflight)
	assert.NotNil(t, m.RequestsTotal)
	assert.NotNil(t, m.RequestDuration)
	assert.NotNil(t, m.MessagesReceived)
	assert.NotNil(t, m.MessagesSent)
	// Media metrics
	assert.NotNil(t, m.UploadsTotal)
	assert.NotNil(t, m.UploadBytesTotal)
	assert.NotNil(t, m.UploadDuration)
	assert.NotNil(t, m.DownloadsTotal)
	assert.NotNil(t, m.DownloadBytesTotal)
	assert.NotNil(t, m.MediaChunksTotal)
	assert.NotNil(t, m.MediaChunkBytesTotal)
}

func TestMetricsConnectionTracking(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := newMetricsWithRegistry("test-agent", "test-namespace", reg)

	// Test connection opened
	m.ConnectionOpened()
	assert.Equal(t, float64(1), getGaugeValue(t, m.ConnectionsActive))

	// Open another connection
	m.ConnectionOpened()
	assert.Equal(t, float64(2), getGaugeValue(t, m.ConnectionsActive))

	// Close a connection
	m.ConnectionClosed()
	assert.Equal(t, float64(1), getGaugeValue(t, m.ConnectionsActive))

	// Verify total connections
	assert.Equal(t, float64(2), getCounterValue(t, m.ConnectionsTotal))
}

func TestMetricsSessionTracking(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := newMetricsWithRegistry("test-agent", "test-namespace", reg)

	// Test session created
	m.SessionCreated()
	assert.Equal(t, float64(1), getGaugeValue(t, m.SessionsActive))

	// Create another session
	m.SessionCreated()
	assert.Equal(t, float64(2), getGaugeValue(t, m.SessionsActive))

	// Close a session
	m.SessionClosed()
	assert.Equal(t, float64(1), getGaugeValue(t, m.SessionsActive))
}

func TestMetricsRequestTracking(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := newMetricsWithRegistry("test-agent", "test-namespace", reg)

	// Start a request
	m.RequestStarted()
	assert.Equal(t, float64(1), getGaugeValue(t, m.RequestsInflight))

	// Complete the request
	m.RequestCompleted("success", 1.5, "demo")
	assert.Equal(t, float64(0), getGaugeValue(t, m.RequestsInflight))

	// Start and complete another request with error
	m.RequestStarted()
	assert.Equal(t, float64(1), getGaugeValue(t, m.RequestsInflight))
	m.RequestCompleted("error", 0.5, "demo")
	assert.Equal(t, float64(0), getGaugeValue(t, m.RequestsInflight))
}

func TestMetricsMessageTracking(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := newMetricsWithRegistry("test-agent", "test-namespace", reg)

	// Track messages
	m.MessageReceived()
	m.MessageReceived()
	m.MessageSent()

	assert.Equal(t, float64(2), getCounterValue(t, m.MessagesReceived))
	assert.Equal(t, float64(1), getCounterValue(t, m.MessagesSent))
}

// Helper functions to extract metric values for testing

func getGaugeValue(t *testing.T, g prometheus.Gauge) float64 {
	t.Helper()
	ch := make(chan prometheus.Metric, 1)
	g.Collect(ch)
	close(ch)

	m := <-ch
	metric := &dto.Metric{}
	err := m.Write(metric)
	require.NoError(t, err)
	return metric.GetGauge().GetValue()
}

func getCounterValue(t *testing.T, c prometheus.Counter) float64 {
	t.Helper()
	ch := make(chan prometheus.Metric, 1)
	c.Collect(ch)
	close(ch)

	m := <-ch
	metric := &dto.Metric{}
	err := m.Write(metric)
	require.NoError(t, err)
	return metric.GetCounter().GetValue()
}

func TestMetricsUploadTracking(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := newMetricsWithRegistry("test-agent", "test-namespace", reg)

	// UploadStarted is a no-op (we track in completed/failed)
	m.UploadStarted()

	// Test successful upload
	m.UploadCompleted(1024, 1.5)
	assert.Equal(t, float64(1024), getCounterValue(t, m.UploadBytesTotal))

	// Test failed upload
	m.UploadFailed()

	// Verify counter vec values by collecting
	ch := make(chan prometheus.Metric, 10)
	m.UploadsTotal.Collect(ch)
	close(ch)

	successCount := 0.0
	failedCount := 0.0
	for metric := range ch {
		d := &dto.Metric{}
		require.NoError(t, metric.Write(d))
		for _, label := range d.GetLabel() {
			if label.GetName() == "status" {
				if label.GetValue() == "success" {
					successCount = d.GetCounter().GetValue()
				} else if label.GetValue() == "failed" {
					failedCount = d.GetCounter().GetValue()
				}
			}
		}
	}
	assert.Equal(t, 1.0, successCount)
	assert.Equal(t, 1.0, failedCount)
}

func TestMetricsDownloadTracking(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := newMetricsWithRegistry("test-agent", "test-namespace", reg)

	// DownloadStarted is a no-op (we track in completed/failed)
	m.DownloadStarted()

	// Test successful download
	m.DownloadCompleted(2048)
	assert.Equal(t, float64(2048), getCounterValue(t, m.DownloadBytesTotal))

	// Test another download
	m.DownloadCompleted(1024)
	assert.Equal(t, float64(3072), getCounterValue(t, m.DownloadBytesTotal))

	// Test failed download
	m.DownloadFailed()

	// Verify counter vec values
	ch := make(chan prometheus.Metric, 10)
	m.DownloadsTotal.Collect(ch)
	close(ch)

	successCount := 0.0
	failedCount := 0.0
	for metric := range ch {
		d := &dto.Metric{}
		require.NoError(t, metric.Write(d))
		for _, label := range d.GetLabel() {
			if label.GetName() == "status" {
				if label.GetValue() == "success" {
					successCount = d.GetCounter().GetValue()
				} else if label.GetValue() == "failed" {
					failedCount = d.GetCounter().GetValue()
				}
			}
		}
	}
	assert.Equal(t, 2.0, successCount)
	assert.Equal(t, 1.0, failedCount)
}

func TestMetricsMediaChunkTracking(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := newMetricsWithRegistry("test-agent", "test-namespace", reg)

	// Test JSON chunk
	m.MediaChunkSent(false, 512)
	assert.Equal(t, float64(512), getCounterValue(t, m.MediaChunkBytesTotal))

	// Test binary chunk
	m.MediaChunkSent(true, 1024)
	assert.Equal(t, float64(1536), getCounterValue(t, m.MediaChunkBytesTotal))

	// Verify counter vec values by type
	ch := make(chan prometheus.Metric, 10)
	m.MediaChunksTotal.Collect(ch)
	close(ch)

	jsonCount := 0.0
	binaryCount := 0.0
	for metric := range ch {
		d := &dto.Metric{}
		require.NoError(t, metric.Write(d))
		for _, label := range d.GetLabel() {
			if label.GetName() == "type" {
				if label.GetValue() == "json" {
					jsonCount = d.GetCounter().GetValue()
				} else if label.GetValue() == "binary" {
					binaryCount = d.GetCounter().GetValue()
				}
			}
		}
	}
	assert.Equal(t, 1.0, jsonCount)
	assert.Equal(t, 1.0, binaryCount)
}
