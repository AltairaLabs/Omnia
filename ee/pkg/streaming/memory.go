/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package streaming

import (
	"context"
	"errors"
	"sync"
	"time"
)

// durationFromMs converts milliseconds to time.Duration.
func durationFromMs(ms int) time.Duration {
	return time.Duration(ms) * time.Millisecond
}

// MemoryPublisher is an in-memory StreamingPublisher for testing.
type MemoryPublisher struct {
	mu     sync.Mutex
	events []*SessionEvent
	closed bool
}

// NewMemoryPublisher creates a new MemoryPublisher.
func NewMemoryPublisher() *MemoryPublisher {
	return &MemoryPublisher{}
}

// Publish stores an event in memory.
func (m *MemoryPublisher) Publish(_ context.Context, event *SessionEvent) error {
	if event == nil {
		return errors.New(errMsgNilEvent)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return errors.New(errMsgPublisherClosed)
	}

	m.events = append(m.events, event)

	return nil
}

// PublishBatch stores multiple events in memory.
func (m *MemoryPublisher) PublishBatch(_ context.Context, events []*SessionEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return errors.New(errMsgPublisherClosed)
	}

	for _, event := range events {
		if event != nil {
			m.events = append(m.events, event)
		}
	}

	return nil
}

// Close marks the publisher as closed.
func (m *MemoryPublisher) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.closed = true

	return nil
}

// Events returns a copy of all published events.
func (m *MemoryPublisher) Events() []*SessionEvent {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make([]*SessionEvent, len(m.events))
	copy(result, m.events)

	return result
}

// Reset clears all stored events.
func (m *MemoryPublisher) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.events = nil
}
