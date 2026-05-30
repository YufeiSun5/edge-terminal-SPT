package main

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestCollectorObserveSnapshotAndWriteJSON(t *testing.T) {
	collector := newCollector()
	received := time.Date(2026, 5, 29, 12, 0, 2, 0, time.UTC)
	collector.observe([]byte(`bad`), received)
	collector.observe([]byte(`{
		"PVs":{"2":"2026-05-29 12:00:00.000 +0000","3":192},
		"Objs":[
			{"N":"A_38","1":10},
			{"N":"A_38","1":9},
			{"N":"B","1":true,"3":0}
		]
	}`), received)
	collector.observe([]byte(`{
		"PVs":{"2":"2026-05-29 12:00:00.000 +0000","3":192},
		"Objs":[{"N":"A_38","1":12}]
	}`), received.Add(time.Second))

	result := collector.snapshot(received.Add(-time.Second), received.Add(12*time.Second))
	if result.Messages != 3 || result.InvalidMessages != 1 || result.Updates != 4 || result.UniqueVariables != 2 {
		t.Fatalf("unexpected summary counters: %+v", result)
	}
	if result.BadQuality != 1 || result.LatencySamples != 4 || result.StaleOver10s == 0 {
		t.Fatalf("unexpected summary quality/latency/stale: %+v", result)
	}
	if len(result.ThirtyEight) != 1 || result.ThirtyEight[0].Regressions != 1 || result.ThirtyEight[0].Gaps != 1 {
		t.Fatalf("unexpected 38 stats: %+v", result.ThirtyEight)
	}
	path := t.TempDir() + "/summary.json"
	if err := writeJSON(path, result); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var decoded summary
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Messages != result.Messages {
		t.Fatalf("unexpected decoded summary: %+v", decoded)
	}
	printProgress(result)
}

func TestPercentileAndEmptySnapshot(t *testing.T) {
	if percentile(nil, 0.95) != 0 {
		t.Fatal("empty percentile should be zero")
	}
	values := []float64{1, 2, 3, 4}
	if percentile(values, 0) != 1 || percentile(values, 0.95) != 4 || percentile(values, 2) != 4 {
		t.Fatal("percentile clamping failed")
	}
	result := newCollector().snapshot(time.Now(), time.Now())
	if result.UniqueVariables != 0 || len(result.WorstStale) != 0 {
		t.Fatalf("unexpected empty snapshot: %+v", result)
	}
}
