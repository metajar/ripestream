package pipeline

import "testing"

func TestIngestionMeterSnapshot(t *testing.T) {
	m := &IngestionMeter{started: 100}
	for range 4 {
		m.observeAt(100)
	}
	for range 6 {
		m.observeAt(101)
	}

	got := m.snapshotAt(102)
	if got.Total != 10 {
		t.Fatalf("total = %d, want 10", got.Total)
	}
	if got.TestsPerSecond != 5 {
		t.Fatalf("tests/sec = %v, want 5", got.TestsPerSecond)
	}
	if len(got.Samples) != ingestionWindowSeconds {
		t.Fatalf("samples = %d, want %d", len(got.Samples), ingestionWindowSeconds)
	}
	if got.Samples[len(got.Samples)-3].Tests != 4 || got.Samples[len(got.Samples)-2].Tests != 6 {
		t.Fatalf("last completed samples = %#v, want counts 4 and 6", got.Samples[len(got.Samples)-3:])
	}
}

func TestIngestionMeterOverwritesExpiredRingBucket(t *testing.T) {
	m := &IngestionMeter{started: 1}
	m.observeAt(1)
	m.observeAt(61)

	got := m.snapshotAt(61)
	if got.Samples[0].Timestamp != 2 || got.Samples[0].Tests != 0 {
		t.Fatalf("oldest sample = %#v, want empty second 2", got.Samples[0])
	}
	if got.Samples[len(got.Samples)-1].Tests != 1 {
		t.Fatalf("newest count = %d, want 1", got.Samples[len(got.Samples)-1].Tests)
	}
}
