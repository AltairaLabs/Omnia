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

package cold

import (
	"context"
	"sort"
	"strings"
	"sync"
)

// MemoryBlobStore is a thread-safe in-memory BlobStore for unit testing.
type MemoryBlobStore struct {
	mu     sync.RWMutex
	data   map[string][]byte
	closed bool
}

// NewMemoryBlobStore creates a new in-memory BlobStore.
func NewMemoryBlobStore() *MemoryBlobStore {
	return &MemoryBlobStore{
		data: make(map[string][]byte),
	}
}

func (m *MemoryBlobStore) Put(_ context.Context, key string, data []byte, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]byte, len(data))
	copy(cp, data)
	m.data[key] = cp
	return nil
}

func (m *MemoryBlobStore) Get(_ context.Context, key string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	d, ok := m.data[key]
	if !ok {
		return nil, ErrObjectNotFound
	}
	cp := make([]byte, len(d))
	copy(cp, d)
	return cp, nil
}

func (m *MemoryBlobStore) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.data[key]; !ok {
		return ErrObjectNotFound
	}
	delete(m.data, key)
	return nil
}

func (m *MemoryBlobStore) List(_ context.Context, prefix string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var keys []string
	for k := range m.data {
		if strings.HasPrefix(k, prefix) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys, nil
}

func (m *MemoryBlobStore) Exists(_ context.Context, key string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.data[key]
	return ok, nil
}

func (m *MemoryBlobStore) Ping(_ context.Context) error {
	return nil
}

func (m *MemoryBlobStore) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

// Ensure MemoryBlobStore implements BlobStore.
var _ BlobStore = (*MemoryBlobStore)(nil)
