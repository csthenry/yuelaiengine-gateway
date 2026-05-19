package core

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	svc_circuitbreaker "yuelaiengine/gateway/internal/service/circuitbreaker"
)

type monitorHistoryRecord struct {
	Timestamp time.Time                                  `json:"timestamp"`
	Metrics   metricsSummary                             `json:"metrics"`
	Circuits  map[string]svc_circuitbreaker.CircuitState `json:"circuits,omitempty"`
	Health    map[string]map[string]bool                 `json:"health,omitempty"`
}

func (g *Gateway) configureMonitorPersistenceLocked() {
	if g.monitorCancel != nil {
		g.monitorCancel()
		g.monitorCancel = nil
	}

	cfg := g.config
	if cfg == nil || !cfg.Monitoring.PersistEnabled {
		g.monitorPath = ""
		return
	}

	path := strings.TrimSpace(cfg.Monitoring.PersistPath)
	if path == "" {
		path = "./logs/monitoring/monitor-history.jsonl"
	}
	interval := cfg.Monitoring.FlushInterval
	if interval <= 0 {
		interval = 5 * time.Second
	}

	g.monitorPath = path
	ctx, cancel := context.WithCancel(context.Background())
	g.monitorCancel = cancel

	go g.runMonitorPersistence(ctx, path, interval)
}

func (g *Gateway) runMonitorPersistence(ctx context.Context, path string, interval time.Duration) {
	g.persistMonitorSnapshot(path)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			g.persistMonitorSnapshot(path)
		}
	}
}

func (g *Gateway) persistMonitorSnapshot(path string) {
	record := g.buildMonitorHistoryRecord()
	if err := appendMonitorHistoryRecord(path, record); err != nil {
		g.logger.Warn(context.Background(), "监控快照持久化失败", "path", path, "error", err.Error())
	}
}

func (g *Gateway) buildMonitorHistoryRecord() monitorHistoryRecord {
	var openCount uint64
	if g.circuitBreakerSvc != nil {
		openCount = g.circuitBreakerSvc.OpenTransitionCount(context.Background())
	}

	record := monitorHistoryRecord{
		Timestamp: time.Now(),
		Metrics:   g.metrics.Snapshot(openCount),
		Health:    g.healthChecker.GetAllStatuses(),
	}
	if g.circuitBreakerSvc != nil {
		record.Circuits = g.circuitBreakerSvc.GetAllState(context.Background())
	}
	return record
}

func appendMonitorHistoryRecord(path string, record monitorHistoryRecord) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("monitor path is empty")
	}
	dir := filepath.Dir(path)
	if dir == "" || dir == "." {
		dir = "."
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()

	return json.NewEncoder(file).Encode(record)
}

func readMonitorHistory(path string, limit int) ([]monitorHistoryRecord, error) {
	if strings.TrimSpace(path) == "" || limit <= 0 {
		return []monitorHistoryRecord{}, nil
	}

	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []monitorHistoryRecord{}, nil
		}
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 2*1024*1024)

	records := make([]monitorHistoryRecord, 0, limit)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var item monitorHistoryRecord
		if err := json.Unmarshal([]byte(line), &item); err != nil {
			continue
		}
		records = append(records, item)
		if len(records) > limit {
			records = append([]monitorHistoryRecord(nil), records[len(records)-limit:]...)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return records, nil
}
