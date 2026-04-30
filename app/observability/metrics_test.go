package observability

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetrics_Inc(t *testing.T) {
	m := NewMetrics()
	m.Inc("spam_checks")
	m.Inc("spam_checks")
	m.Inc("spam_detected")

	snap := m.Snapshot().(MetricsSnapshot)
	assert.Equal(t, int64(2), snap.Counters["spam_checks"])
	assert.Equal(t, int64(1), snap.Counters["spam_detected"])
}

func TestMetrics_Add(t *testing.T) {
	m := NewMetrics()
	m.Add("bytes_processed", 1024)
	m.Add("bytes_processed", 2048)

	snap := m.Snapshot().(MetricsSnapshot)
	assert.Equal(t, int64(3072), snap.Counters["bytes_processed"])
}

func TestMetrics_Observe(t *testing.T) {
	m := NewMetrics()
	m.Observe("gateway_latency", 50*time.Millisecond)
	m.Observe("gateway_latency", 150*time.Millisecond)
	m.Observe("gateway_latency", 2*time.Second)

	snap := m.Snapshot().(MetricsSnapshot)
	h := snap.Histograms["gateway_latency"]
	assert.Equal(t, int64(3), h.Count)
	assert.Equal(t, int64(2200), h.SumMs)
	assert.InDelta(t, 733.33, h.AvgMs, 0.01)

	assert.Equal(t, int64(1), h.Buckets["50ms"])
	assert.Equal(t, int64(2), h.Buckets["250ms"])
	assert.Equal(t, int64(3), h.Buckets["5s"])
}

func TestMetrics_SnapshotJSON(t *testing.T) {
	m := NewMetrics()
	m.Inc("test_counter")
	m.Observe("test_latency", 100*time.Millisecond)

	snap := m.Snapshot()
	data, err := json.Marshal(snap)
	require.NoError(t, err)
	assert.Contains(t, string(data), "test_counter")
	assert.Contains(t, string(data), "test_latency")
}

func TestMetrics_EmptySnapshot(t *testing.T) {
	m := NewMetrics()
	snap := m.Snapshot().(MetricsSnapshot)
	assert.Empty(t, snap.Counters)
	assert.Empty(t, snap.Histograms)
}

func TestMetrics_Concurrent(t *testing.T) {
	m := NewMetrics()
	done := make(chan struct{})

	for i := 0; i < 100; i++ {
		go func() {
			m.Inc("concurrent_counter")
			m.Observe("concurrent_latency", time.Millisecond)
			done <- struct{}{}
		}()
	}

	for i := 0; i < 100; i++ {
		<-done
	}

	snap := m.Snapshot().(MetricsSnapshot)
	assert.Equal(t, int64(100), snap.Counters["concurrent_counter"])
	assert.Equal(t, int64(100), snap.Histograms["concurrent_latency"].Count)
}
