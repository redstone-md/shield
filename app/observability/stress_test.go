package observability

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetrics_StressCounters(t *testing.T) {
	m := NewMetrics()
	const goroutines = 500
	const opsPer = 200

	var wg sync.WaitGroup
	for range goroutines {
		wg.Go(func() {
			for range opsPer {
				m.Inc("requests")
				m.Add("bytes", 1)
			}
		})
	}
	wg.Wait()

	snap := m.Snapshot().(MetricsSnapshot)
	assert.Equal(t, int64(goroutines*opsPer), snap.Counters["requests"])
	assert.Equal(t, int64(goroutines*opsPer), snap.Counters["bytes"])
}

func TestMetrics_StressHistograms(t *testing.T) {
	m := NewMetrics()
	const goroutines = 300
	const opsPer = 100

	var wg sync.WaitGroup
	for i := range goroutines {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := range opsPer {
				d := time.Duration(id+j%10+1) * time.Millisecond
				m.Observe("latency", d)
			}
		}(i)
	}
	wg.Wait()

	snap := m.Snapshot().(MetricsSnapshot)
	h := snap.Histograms["latency"]
	expected := int64(goroutines * opsPer)
	assert.Equal(t, expected, h.Count)
	assert.Positive(t, h.SumMs)
	assert.Positive(t, h.AvgMs)
}

func TestMetrics_StressMixedOps(t *testing.T) {
	m := NewMetrics()
	const workers = 200
	const opsPer = 50
	var wg sync.WaitGroup

	for range workers {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for range opsPer {
				m.Inc("counter_a")
			}
		}()
		go func() {
			defer wg.Done()
			for j := range opsPer {
				m.Observe("hist_x", time.Duration(j+1)*time.Millisecond)
			}
		}()
	}
	wg.Wait()

	snap := m.Snapshot().(MetricsSnapshot)
	assert.Equal(t, int64(workers*opsPer), snap.Counters["counter_a"])
	assert.Equal(t, int64(workers*opsPer), snap.Histograms["hist_x"].Count)
}

func TestMetrics_StressSnapshotConsistency(t *testing.T) {
	m := NewMetrics()
	const writers = 100
	const opsPer = 200

	var wg sync.WaitGroup
	wg.Add(writers)
	for i := range writers {
		go func(id int) {
			defer wg.Done()
			for range opsPer {
				m.Inc("snap_ctr")
				m.Inc("snap_bytes")
			}
		}(i)
	}
	wg.Wait()

	snap := m.Snapshot().(MetricsSnapshot)
	assert.Equal(t, int64(writers*opsPer), snap.Counters["snap_ctr"])
	assert.Equal(t, int64(writers*opsPer), snap.Counters["snap_bytes"])
}

func TestMetrics_StressManyNames(t *testing.T) {
	m := NewMetrics()
	const nameCount = 100
	const opsPerName = 50

	var wg sync.WaitGroup
	for i := range nameCount {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			name := fmt.Sprintf("metric_%d", idx)
			for range opsPerName {
				m.Inc(name)
			}
		}(i)
	}
	wg.Wait()

	snap := m.Snapshot().(MetricsSnapshot)
	assert.Len(t, snap.Counters, nameCount)
	for i := range nameCount {
		name := fmt.Sprintf("metric_%d", i)
		assert.Equal(t, int64(opsPerName), snap.Counters[name], "counter %s mismatch", name)
	}
}

func TestMetrics_StressObserveAccuracy(t *testing.T) {
	m := NewMetrics()
	const count = 1000
	dur := 25 * time.Millisecond

	var wg sync.WaitGroup
	for range count {
		wg.Go(func() {
			m.Observe("test_lat", dur)
		})
	}
	wg.Wait()

	snap := m.Snapshot().(MetricsSnapshot)
	h := snap.Histograms["test_lat"]
	require.Equal(t, int64(count), h.Count)
	assert.Equal(t, int64(count*25), h.SumMs)
	assert.Equal(t, int64(count), h.Buckets["50ms"])
	assert.Equal(t, int64(count), h.Buckets["100ms"])
	assert.Equal(t, int64(count), h.Buckets["+Inf"])
}

func BenchmarkMetrics_Inc(b *testing.B) {
	m := NewMetrics()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			m.Inc("bench_counter")
		}
	})
}

func BenchmarkMetrics_Observe(b *testing.B) {
	m := NewMetrics()
	b.ReportAllocs()
	d := 10 * time.Millisecond
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			m.Observe("bench_lat", d)
		}
	})
}

func BenchmarkMetrics_Snapshot(b *testing.B) {
	m := NewMetrics()
	for i := range 50 {
		m.Inc(fmt.Sprintf("counter_%d", i))
		m.Observe(fmt.Sprintf("hist_%d", i), time.Duration(i+1)*time.Millisecond)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.Snapshot()
	}
}
