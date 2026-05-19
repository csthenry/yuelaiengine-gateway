package core

import (
	"path/filepath"
	"testing"
	"time"
)

func TestReadMonitorHistoryLimit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "monitor-history.jsonl")

	for i := 0; i < 5; i++ {
		err := appendMonitorHistoryRecord(path, monitorHistoryRecord{
			Timestamp: time.Unix(int64(i), 0),
			Metrics: metricsSummary{
				TotalRequests: uint64(i + 1),
			},
		})
		if err != nil {
			t.Fatalf("appendMonitorHistoryRecord() error = %v", err)
		}
	}

	records, err := readMonitorHistory(path, 3)
	if err != nil {
		t.Fatalf("readMonitorHistory() error = %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("len(records)=%d want=3", len(records))
	}
	if records[0].Metrics.TotalRequests != 3 || records[2].Metrics.TotalRequests != 5 {
		t.Fatalf("unexpected records range: first=%d last=%d",
			records[0].Metrics.TotalRequests, records[2].Metrics.TotalRequests)
	}
}
