package moderation

import (
	"context"
	"errors"
	"sync"
)

// Queue defines the initial async seam between ingestion and moderation work.
type Queue interface {
	Publish(ctx context.Context, event IncomingEvent) error
	Consume() <-chan IncomingEvent
	Close()
}

// ErrQueueClosed is returned when publishing to a closed queue.
var ErrQueueClosed = errors.New("moderation queue closed")

// InMemoryQueue is a single-stream channel-backed queue for tracer-bullet flows.
type InMemoryQueue struct {
	ch     chan IncomingEvent
	done   chan struct{}
	closed bool
	mu     sync.RWMutex
	once   sync.Once
}

// NewInMemoryQueue creates an in-memory queue with the provided buffer size.
func NewInMemoryQueue(buffer int) *InMemoryQueue {
	if buffer < 0 {
		buffer = 0
	}

	return &InMemoryQueue{
		ch:   make(chan IncomingEvent, buffer),
		done: make(chan struct{}),
	}
}

// Publish enqueues an event or returns when the context or queue closes.
func (q *InMemoryQueue) Publish(ctx context.Context, event IncomingEvent) error {
	q.mu.RLock()
	closed := q.closed
	done := q.done
	q.mu.RUnlock()

	if closed {
		return ErrQueueClosed
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return ErrQueueClosed
	case q.ch <- event:
		return nil
	}
}

// Consume returns the queue stream used by a worker.
func (q *InMemoryQueue) Consume() <-chan IncomingEvent {
	return q.ch
}

// Close stops the queue and closes the consumer stream.
func (q *InMemoryQueue) Close() {
	q.once.Do(func() {
		q.mu.Lock()
		q.closed = true
		close(q.done)
		close(q.ch)
		q.mu.Unlock()
	})
}
