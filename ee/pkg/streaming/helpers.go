/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package streaming

import (
	"context"
	"encoding/json"
	"fmt"
)

// Common error messages used by streaming publishers.
const (
	ErrMsgNilEvent    = "event must not be nil"
	ErrMsgMarshalFail = "failed to marshal event"
)

// marshalEvent validates and marshals a SessionEvent to JSON bytes.
// Returns an error if the event is nil or cannot be marshalled.
func marshalEvent(event *SessionEvent) ([]byte, error) {
	if event == nil {
		return nil, fmt.Errorf("%s", ErrMsgNilEvent)
	}

	data, err := json.Marshal(event)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", ErrMsgMarshalFail, err)
	}

	return data, nil
}

// publishFunc is a function that sends serialised event data to a backend.
type publishFunc func(ctx context.Context, event *SessionEvent) error

// defaultPublishBatch iterates over events and publishes each one using the
// supplied publish function. It stops on the first error.
func defaultPublishBatch(ctx context.Context, events []*SessionEvent, publish publishFunc) error {
	for i, event := range events {
		if err := publish(ctx, event); err != nil {
			return fmt.Errorf("failed to publish event at index %d: %w", i, err)
		}
	}
	return nil
}
