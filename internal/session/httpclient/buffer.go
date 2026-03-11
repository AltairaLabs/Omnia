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

package httpclient

import (
	"sync"
	"sync/atomic"
	"time"
)

// bufferedRequest is a write request queued for retry after session-api downtime.
type bufferedRequest struct {
	method string
	path   string
	body   []byte
	queued time.Time
}

// writeBuffer is a thread-safe, fixed-capacity ring buffer for failed write requests.
// When full, the oldest item is dropped to make room for new items.
type writeBuffer struct {
	mu       sync.Mutex
	items    []bufferedRequest
	head     int
	count    int
	capacity int
	dropped  atomic.Int64
}

func newWriteBuffer(capacity int) *writeBuffer {
	return &writeBuffer{
		items:    make([]bufferedRequest, capacity),
		capacity: capacity,
	}
}

// enqueue adds an item to the buffer. If full, the oldest item is dropped.
// Returns true if an old item was dropped to make room.
func (b *writeBuffer) enqueue(req bufferedRequest) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	idx := (b.head + b.count) % b.capacity
	if b.count == b.capacity {
		// Full — overwrite oldest, advance head.
		b.items[b.head] = req
		b.head = (b.head + 1) % b.capacity
		b.dropped.Add(1)
		return true
	}
	b.items[idx] = req
	b.count++
	return false
}

// dequeue removes and returns the oldest item. Returns false if empty.
func (b *writeBuffer) dequeue() (bufferedRequest, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.count == 0 {
		return bufferedRequest{}, false
	}
	item := b.items[b.head]
	b.items[b.head] = bufferedRequest{} // clear for GC
	b.head = (b.head + 1) % b.capacity
	b.count--
	return item, true
}

// peek returns the oldest item without removing it. Returns false if empty.
func (b *writeBuffer) peek() (bufferedRequest, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.count == 0 {
		return bufferedRequest{}, false
	}
	return b.items[b.head], true
}

// len returns the number of buffered items.
func (b *writeBuffer) len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.count
}
