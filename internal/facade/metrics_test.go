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

package facade

import (
	"context"
	"testing"
)

func TestNoOpMetrics_ConnectionOpened(t *testing.T) {
	m := &NoOpMetrics{}
	m.ConnectionOpened() // Should not panic
}

func TestNoOpMetrics_ConnectionClosed(t *testing.T) {
	m := &NoOpMetrics{}
	m.ConnectionClosed() // Should not panic
}

func TestNoOpMetrics_SessionCreated(t *testing.T) {
	m := &NoOpMetrics{}
	m.SessionCreated() // Should not panic
}

func TestNoOpMetrics_SessionClosed(t *testing.T) {
	m := &NoOpMetrics{}
	m.SessionClosed() // Should not panic
}

func TestNoOpMetrics_RequestStarted(t *testing.T) {
	m := &NoOpMetrics{}
	m.RequestStarted() // Should not panic
}

func TestNoOpMetrics_RequestCompleted(t *testing.T) {
	m := &NoOpMetrics{}
	m.RequestCompleted(context.Background(), "success", 1.5, "demo") // Should not panic
}

func TestNoOpMetrics_MessageReceived(t *testing.T) {
	m := &NoOpMetrics{}
	m.MessageReceived() // Should not panic
}

func TestNoOpMetrics_MessageSent(t *testing.T) {
	m := &NoOpMetrics{}
	m.MessageSent() // Should not panic
}

func TestNoOpMetrics_RecordingDropped(t *testing.T) {
	m := &NoOpMetrics{}
	m.RecordingDropped() // Should not panic
}

func TestServerMetricsInterface(t *testing.T) {
	// Verify that NoOpMetrics can be used as ServerMetrics
	var metrics ServerMetrics = &NoOpMetrics{}

	// All operations should work without panic
	metrics.ConnectionOpened()
	metrics.ConnectionClosed()
	metrics.SessionCreated()
	metrics.SessionClosed()
	metrics.RequestStarted()
	metrics.RequestCompleted(context.Background(), "error", 0.5, "echo")
	metrics.MessageReceived()
	metrics.MessageSent()
	metrics.RecordingDropped()
}

func TestNoOpMetrics_AudioSessionStarted(t *testing.T) {
	m := &NoOpMetrics{}
	m.AudioSessionStarted() // Should not panic
}

func TestNoOpMetrics_AudioSessionEnded(t *testing.T) {
	m := &NoOpMetrics{}
	m.AudioSessionEnded() // Should not panic
}

func TestNoOpMetrics_AudioIngestLatency(t *testing.T) {
	m := &NoOpMetrics{}
	m.AudioIngestLatency(0.001) // Should not panic
}

func TestNoOpMetrics_AudioMethods_ViaInterface(t *testing.T) {
	// Verify audio methods satisfy the ServerMetrics interface.
	var metrics ServerMetrics = &NoOpMetrics{}
	metrics.AudioSessionStarted()
	metrics.AudioSessionEnded()
	metrics.AudioIngestLatency(0.001)
}

func TestNoOpMetrics_RealtimeHooks(t *testing.T) {
	var m ServerMetrics = &NoOpMetrics{}
	m.RealtimeSessionParked()
	m.RealtimeSessionReattached()
	m.RealtimeSessionParkExpired()
}

func TestNoOpMetrics_DrainHooks(t *testing.T) {
	var m ServerMetrics = &NoOpMetrics{}
	m.RealtimeDrainStarted()
	m.RealtimeDrainCompleted("all_drained", 1.2, 3, 0)
	m.RealtimeDrainCompleted("deadline", 30.0, 0, 2)
	m.RealtimeDrainCompleted("ctx_canceled", 5.5, 1, 1)
}
