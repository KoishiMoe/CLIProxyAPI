package usage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

const (
	// DefaultStatisticsFilename is the on-disk file name used for persisted usage statistics.
	DefaultStatisticsFilename = "usage-statistics.json"
	// DefaultStatisticsDirectory is the subdirectory used to keep persisted usage
	// statistics out of the watched auth-file root.
	DefaultStatisticsDirectory = "usage-statistics"

	statisticsPersistenceVersion = 1
)

type statisticsPersistencePayload struct {
	Version int                `json:"version"`
	SavedAt time.Time          `json:"saved_at"`
	Usage   StatisticsSnapshot `json:"usage"`
}

var statisticsPersistence = struct {
	mu     sync.Mutex
	path   string
	loaded bool
}{}

// ConfigureStatisticsPersistence enables disk persistence for the shared usage statistics store.
func ConfigureStatisticsPersistence(path string) error {
	path = strings.TrimSpace(path)

	statisticsPersistence.mu.Lock()
	if path == "" {
		statisticsPersistence.path = ""
		statisticsPersistence.loaded = false
		statisticsPersistence.mu.Unlock()
		return nil
	}
	if statisticsPersistence.loaded && statisticsPersistence.path == path {
		statisticsPersistence.mu.Unlock()
		return nil
	}
	statisticsPersistence.mu.Unlock()

	payload, err := readStatisticsPersistence(path)
	if err != nil {
		return err
	}
	if payload != nil {
		result := defaultRequestStatistics.MergeSnapshot(payload.Usage)
		if result.Added > 0 {
			log.WithFields(log.Fields{
				"path":    path,
				"added":   result.Added,
				"skipped": result.Skipped,
			}).Info("loaded persisted usage statistics")
		}
	}

	statisticsPersistence.mu.Lock()
	statisticsPersistence.path = path
	statisticsPersistence.loaded = true
	statisticsPersistence.mu.Unlock()
	return nil
}

// FlushStatisticsPersistence writes the current shared usage statistics snapshot to disk.
func FlushStatisticsPersistence() error {
	statisticsPersistence.mu.Lock()
	path := statisticsPersistence.path
	statisticsPersistence.mu.Unlock()
	if strings.TrimSpace(path) == "" {
		return nil
	}

	payload := statisticsPersistencePayload{
		Version: statisticsPersistenceVersion,
		SavedAt: time.Now().UTC(),
		Usage:   defaultRequestStatistics.Snapshot(),
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal usage statistics: %w", err)
	}
	data = append(data, '\n')
	if err = writeStatisticsPersistence(path, data); err != nil {
		return err
	}
	return nil
}

func persistStatisticsAfterUpdate() {
	if err := FlushStatisticsPersistence(); err != nil {
		log.WithError(err).Warn("failed to persist usage statistics")
	}
}

func readStatisticsPersistence(path string) (*statisticsPersistencePayload, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read usage statistics: %w", err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, nil
	}

	var payload statisticsPersistencePayload
	if err = json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("parse usage statistics: %w", err)
	}
	if payload.Version != 0 && payload.Version != statisticsPersistenceVersion {
		return nil, fmt.Errorf("unsupported usage statistics version: %d", payload.Version)
	}
	return &payload, nil
}

func writeStatisticsPersistence(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create usage statistics directory: %w", err)
	}

	tmpFile, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create usage statistics temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	removeTmp := true
	defer func() {
		if removeTmp {
			_ = os.Remove(tmpPath)
		}
	}()

	if err = tmpFile.Chmod(0o600); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("chmod usage statistics temp file: %w", err)
	}
	if _, err = tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("write usage statistics temp file: %w", err)
	}
	if err = tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("sync usage statistics temp file: %w", err)
	}
	if err = tmpFile.Close(); err != nil {
		return fmt.Errorf("close usage statistics temp file: %w", err)
	}
	if err = os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace usage statistics file: %w", err)
	}
	removeTmp = false
	return nil
}
