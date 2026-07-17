package pipeline

import (
	"sync"
	"time"
)

const ingestionWindowSeconds = 60

// IngestionSample is one per-second count of RIPE Atlas results read from the
// stream. Timestamp is a Unix second.
type IngestionSample struct {
	Timestamp int64  `json:"timestamp"`
	Tests     uint64 `json:"tests"`
}

// IngestionSnapshot is the lightweight, in-memory view exposed to the UI.
// TestsPerSecond is averaged over up to five completed seconds so the headline
// rate does not jump around with the exact instant the API is polled.
type IngestionSnapshot struct {
	Total          uint64            `json:"total"`
	TestsPerSecond float64           `json:"tests_per_second"`
	WindowSeconds  int               `json:"window_seconds"`
	Samples        []IngestionSample `json:"samples"`
}

type ingestionBucket struct {
	second int64
	count  uint64
}

// IngestionMeter records stream reads in a fixed-size per-second ring. It adds
// no storage dependency and retains only the last minute of rate history.
type IngestionMeter struct {
	mu      sync.Mutex
	started int64
	total   uint64
	buckets [ingestionWindowSeconds]ingestionBucket
}

func NewIngestionMeter() *IngestionMeter {
	return &IngestionMeter{started: time.Now().Unix()}
}

// Observe records one successfully parsed result entering the ingestion
// pipeline.
func (m *IngestionMeter) Observe() {
	m.observeAt(time.Now().Unix())
}

func (m *IngestionMeter) observeAt(second int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.started == 0 || second < m.started {
		m.started = second
	}
	idx := int(second % ingestionWindowSeconds)
	if idx < 0 {
		idx += ingestionWindowSeconds
	}
	b := &m.buckets[idx]
	if b.second != second {
		*b = ingestionBucket{second: second}
	}
	b.count++
	m.total++
}

// Snapshot returns a continuous 60-second series, including zero-value gaps.
func (m *IngestionMeter) Snapshot() IngestionSnapshot {
	return m.snapshotAt(time.Now().Unix())
}

func (m *IngestionMeter) snapshotAt(now int64) IngestionSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()

	samples := make([]IngestionSample, 0, ingestionWindowSeconds)
	for second := now - ingestionWindowSeconds + 1; second <= now; second++ {
		count := m.countAt(second)
		samples = append(samples, IngestionSample{Timestamp: second, Tests: count})
	}

	// Average completed seconds only; the current bucket is still being filled.
	firstRateSecond := now - 5
	if firstRateSecond < m.started {
		firstRateSecond = m.started
	}
	var rateTotal uint64
	var rateSeconds int64
	for second := firstRateSecond; second < now; second++ {
		rateTotal += m.countAt(second)
		rateSeconds++
	}
	var rate float64
	if rateSeconds > 0 {
		rate = float64(rateTotal) / float64(rateSeconds)
	}

	return IngestionSnapshot{
		Total:          m.total,
		TestsPerSecond: rate,
		WindowSeconds:  ingestionWindowSeconds,
		Samples:        samples,
	}
}

func (m *IngestionMeter) countAt(second int64) uint64 {
	idx := int(second % ingestionWindowSeconds)
	if idx < 0 {
		idx += ingestionWindowSeconds
	}
	b := m.buckets[idx]
	if b.second != second {
		return 0
	}
	return b.count
}
