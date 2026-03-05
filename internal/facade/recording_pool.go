/*
Copyright 2026.

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
	"sync"

	"github.com/go-logr/logr"
)

const (
	// DefaultRecordingPoolSize is the number of worker goroutines.
	DefaultRecordingPoolSize = 100
	// DefaultRecordingQueueSize is the channel buffer size.
	DefaultRecordingQueueSize = 1000
)

// RecordingPool is a bounded worker pool for asynchronous session recording.
// It replaces unbounded goroutine creation with a fixed set of workers
// and a buffered channel. If the queue is full, new tasks are dropped
// with a warning log rather than blocking the caller.
type RecordingPool struct {
	queue chan func()
	wg    sync.WaitGroup
	log   logr.Logger
}

// NewRecordingPool creates and starts a recording pool with the given worker
// count and queue capacity.
func NewRecordingPool(workers, queueSize int, log logr.Logger) *RecordingPool {
	if workers <= 0 {
		workers = DefaultRecordingPoolSize
	}
	if queueSize <= 0 {
		queueSize = DefaultRecordingQueueSize
	}

	p := &RecordingPool{
		queue: make(chan func(), queueSize),
		log:   log.WithName("recording-pool"),
	}

	p.wg.Add(workers)
	for range workers {
		go p.worker()
	}

	p.log.Info("recording pool started", "workers", workers, "queueSize", queueSize)
	return p
}

// Submit enqueues a recording task. If the queue is full, the task is dropped.
func (p *RecordingPool) Submit(task func()) {
	select {
	case p.queue <- task:
	default:
		p.log.V(1).Info("recording queue full, dropping task")
	}
}

// Close drains the queue and waits for all workers to finish.
func (p *RecordingPool) Close() {
	close(p.queue)
	p.wg.Wait()
	p.log.Info("recording pool stopped")
}

func (p *RecordingPool) worker() {
	defer p.wg.Done()
	for task := range p.queue {
		task()
	}
}
