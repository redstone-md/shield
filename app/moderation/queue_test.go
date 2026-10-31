package moderation

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInMemoryQueuePublishAndConsume(t *testing.T) {
	t.Parallel()

	q := NewInMemoryQueue(1)
	defer q.Close()

	event := IncomingEvent{
		EventID:       "evt-1",
		CorrelationID: "corr-1",
		Source:        "telegram.webhook",
		ReceivedAt:    time.Now().UTC(),
	}

	err := q.Publish(context.Background(), event)
	require.NoError(t, err)

	select {
	case got := <-q.Consume():
		assert.Equal(t, event, got)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestInMemoryQueueConcurrentPublishAndClose(t *testing.T) {
	q := NewInMemoryQueue(0)

	var publishers sync.WaitGroup
	errs := make(chan error, 100)
	for range 100 {
		publishers.Go(func() {
			errs <- q.Publish(context.Background(), IncomingEvent{EventID: "concurrent"})
		})
	}

	consumerDone := make(chan struct{})
	go func() {
		for range q.Consume() {
			continue
		}
		close(consumerDone)
	}()

	q.Close()
	publishers.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			require.ErrorIs(t, err, ErrQueueClosed)
		}
	}

	select {
	case <-consumerDone:
	case <-time.After(time.Second):
		t.Fatal("consumer did not stop")
	}
}

func TestInMemoryQueueCloseWakesBlockedPublisher(t *testing.T) {
	q := NewInMemoryQueue(0)

	published := make(chan error, 1)
	go func() { published <- q.Publish(context.Background(), IncomingEvent{EventID: "blocked"}) }()
	time.Sleep(10 * time.Millisecond)

	closed := make(chan struct{})
	go func() {
		q.Close()
		close(closed)
	}()

	select {
	case err := <-published:
		require.ErrorIs(t, err, ErrQueueClosed)
	case <-time.After(time.Second):
		t.Fatal("blocked publisher was not woken")
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("close did not finish")
	}
}

func TestInMemoryQueuePublishCanceledContext(t *testing.T) {
	t.Parallel()

	q := NewInMemoryQueue(0)
	defer q.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := q.Publish(ctx, IncomingEvent{EventID: "evt-2"})
	require.ErrorIs(t, err, context.Canceled)
}

func TestInMemoryQueuePublishAfterClose(t *testing.T) {
	t.Parallel()

	q := NewInMemoryQueue(1)
	q.Close()

	err := q.Publish(context.Background(), IncomingEvent{EventID: "evt-3"})
	require.ErrorIs(t, err, ErrQueueClosed)
}

func TestInMemoryQueueCloseClosesConsumerChannel(t *testing.T) {
	t.Parallel()

	q := NewInMemoryQueue(1)
	ch := q.Consume()

	q.Close()

	select {
	case _, ok := <-ch:
		assert.False(t, ok)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for queue close")
	}
}
