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

// ServerMetrics defines the interface for server metrics.
// This allows the metrics implementation to be optional and testable.
type ServerMetrics interface {
	// ConnectionOpened records a new connection.
	ConnectionOpened()
	// ConnectionClosed records a closed connection.
	ConnectionClosed()
	// SessionCreated records a new session.
	SessionCreated()
	// SessionClosed records a closed session.
	SessionClosed()
	// RequestStarted records the start of a request.
	RequestStarted()
	// RequestCompleted records the completion of a request.
	RequestCompleted(status string, durationSeconds float64, handler string)
	// MessageReceived records a received message.
	MessageReceived()
	// MessageSent records a sent message.
	MessageSent()
}

// NoOpMetrics is a no-op implementation of ServerMetrics for when metrics are disabled.
// All methods are intentionally empty as this is a null object pattern implementation.
type NoOpMetrics struct{}

// ConnectionOpened is a no-op - metrics are disabled.
func (n *NoOpMetrics) ConnectionOpened() {}

// ConnectionClosed is a no-op - metrics are disabled.
func (n *NoOpMetrics) ConnectionClosed() {}

// SessionCreated is a no-op - metrics are disabled.
func (n *NoOpMetrics) SessionCreated() {}

// SessionClosed is a no-op - metrics are disabled.
func (n *NoOpMetrics) SessionClosed() {}

// RequestStarted is a no-op - metrics are disabled.
func (n *NoOpMetrics) RequestStarted() {}

// RequestCompleted is a no-op - metrics are disabled.
func (n *NoOpMetrics) RequestCompleted(string, float64, string) {}

// MessageReceived is a no-op - metrics are disabled.
func (n *NoOpMetrics) MessageReceived() {}

// MessageSent is a no-op - metrics are disabled.
func (n *NoOpMetrics) MessageSent() {}
