package observability

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"
)

type Metrics struct {
	counters  sync.Map
	histograms sync.Map
}

type counter struct {
	total atomic.Int64
}

type timingHistogram struct {
	mu    sync.Mutex
	buckets []bucket
	count atomic.Int64
	sum   atomic.Int64
}

type bucket struct {
	label string
	le    time.Duration
	count atomic.Int64
}

func NewMetrics() *Metrics {
	return &Metrics{}
}

func (m *Metrics) Inc(name string) {
	v, _ := m.counters.LoadOrStore(name, &counter{})
	v.(*counter).total.Add(1)
}

func (m *Metrics) Add(name string, delta int64) {
	v, _ := m.counters.LoadOrStore(name, &counter{})
	v.(*counter).total.Add(delta)
}

func (m *Metrics) Observe(name string, duration time.Duration) {
	h := m.getOrCreateHistogram(name)
	h.count.Add(1)
	h.sum.Add(duration.Milliseconds())
	h.record(duration)
}

func (m *Metrics) Snapshot() any {
	snap := MetricsSnapshot{
		Counters:   make(map[string]int64),
		Histograms: make(map[string]HistogramSnapshot),
	}

	m.counters.Range(func(key, value any) bool {
		snap.Counters[key.(string)] = value.(*counter).total.Load()
		return true
	})

	m.histograms.Range(func(key, value any) bool {
		h := value.(*timingHistogram)
		hs := HistogramSnapshot{
			Count: h.count.Load(),
			SumMs: h.sum.Load(),
		}
		if hs.Count > 0 {
			hs.AvgMs = float64(hs.SumMs) / float64(hs.Count)
		}
		hs.Buckets = make(map[string]int64)
		for _, b := range h.buckets {
			hs.Buckets[b.label] = b.count.Load()
		}
		snap.Histograms[key.(string)] = hs
		return true
	})

	return snap
}

func (m *Metrics) getOrCreateHistogram(name string) *timingHistogram {
	v, _ := m.histograms.LoadOrStore(name, newTimingHistogram())
	return v.(*timingHistogram)
}

func newTimingHistogram() *timingHistogram {
	thresholds := []struct {
		label string
		le    time.Duration
	}{
		{"10ms", 10 * time.Millisecond},
		{"50ms", 50 * time.Millisecond},
		{"100ms", 100 * time.Millisecond},
		{"250ms", 250 * time.Millisecond},
		{"500ms", 500 * time.Millisecond},
		{"1s", time.Second},
		{"5s", 5 * time.Second},
		{"+Inf", 1<<62 - 1},
	}
	h := &timingHistogram{}
	for _, t := range thresholds {
		h.buckets = append(h.buckets, bucket{label: t.label, le: t.le})
	}
	return h
}

func (h *timingHistogram) record(d time.Duration) {
	for i := range h.buckets {
		if d <= h.buckets[i].le {
			h.buckets[i].count.Add(1)
		}
	}
}

type MetricsSnapshot struct {
	Counters   map[string]int64          `json:"counters"`
	Histograms map[string]HistogramSnapshot `json:"histograms"`
}

type HistogramSnapshot struct {
	Count   int64             `json:"count"`
	SumMs   int64             `json:"sum_ms"`
	AvgMs   float64           `json:"avg_ms"`
	Buckets map[string]int64  `json:"buckets"`
}

func (s MetricsSnapshot) MarshalJSON() ([]byte, error) {
	type alias MetricsSnapshot
	return json.Marshal(alias(s))
}
