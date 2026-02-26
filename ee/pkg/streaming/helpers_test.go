/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package streaming

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestMarshalEvent_NilEvent(t *testing.T) {
	data, err := marshalEvent(nil)
	if err == nil {
		t.Fatal("expected error for nil event")
	}
	if data != nil {
		t.Errorf("expected nil data, got %v", data)
	}
	if !strings.Contains(err.Error(), ErrMsgNilEvent) {
		t.Errorf("expected error to contain %q, got %q", ErrMsgNilEvent, err.Error())
	}
}

func TestMarshalEvent_Success(t *testing.T) {
	event := newTestEvent("evt-1", "sess-1")
	data, err := marshalEvent(event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty data")
	}
	if !strings.Contains(string(data), "evt-1") {
		t.Errorf("expected data to contain event ID, got %s", string(data))
	}
}

func TestDefaultPublishBatch_Success(t *testing.T) {
	var published []*SessionEvent
	pub := func(_ context.Context, event *SessionEvent) error {
		published = append(published, event)
		return nil
	}

	events := []*SessionEvent{
		newTestEvent("evt-1", "sess-1"),
		newTestEvent("evt-2", "sess-2"),
	}

	err := defaultPublishBatch(context.Background(), events, pub)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(published) != 2 {
		t.Errorf("expected 2 published events, got %d", len(published))
	}
}

func TestDefaultPublishBatch_Empty(t *testing.T) {
	callCount := 0
	pub := func(_ context.Context, _ *SessionEvent) error {
		callCount++
		return nil
	}

	err := defaultPublishBatch(context.Background(), nil, pub)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount != 0 {
		t.Errorf("expected no calls, got %d", callCount)
	}
}

func TestDefaultPublishBatch_ErrorMidBatch(t *testing.T) {
	callCount := 0
	pub := func(_ context.Context, _ *SessionEvent) error {
		callCount++
		if callCount == 2 {
			return fmt.Errorf("publish error")
		}
		return nil
	}

	events := []*SessionEvent{
		newTestEvent("evt-1", "sess-1"),
		newTestEvent("evt-2", "sess-2"),
		newTestEvent("evt-3", "sess-3"),
	}

	err := defaultPublishBatch(context.Background(), events, pub)
	if err == nil {
		t.Fatal("expected error from batch publish")
	}
	if !strings.Contains(err.Error(), "index 1") {
		t.Errorf("expected error to reference index 1, got %q", err.Error())
	}
}

func TestDefaultPublishBatch_NilEventPropagatesError(t *testing.T) {
	pub := func(_ context.Context, event *SessionEvent) error {
		if event == nil {
			return fmt.Errorf("nil event")
		}
		return nil
	}

	events := []*SessionEvent{newTestEvent("evt-1", "sess-1"), nil}
	err := defaultPublishBatch(context.Background(), events, pub)
	if err == nil {
		t.Fatal("expected error for nil event")
	}
}
