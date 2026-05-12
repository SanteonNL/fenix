package output

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/rs/zerolog"
)

// Manager handles timestamped output and archiving of previous runs
type Manager struct {
	baseDir        string
	archiveDir     string
	retentionCount int
	log            *zerolog.Logger
	currentRunDir  string
}

// NewManager creates a new output manager
// baseDir: base output directory (will contain timestamped run folders)
// retentionCount: how many previous runs to keep in archive (0 = keep all)
func NewManager(baseDir string, retentionCount int, log *zerolog.Logger) (*Manager, error) {
	// Create base output directory
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	archiveDir := filepath.Join(baseDir, "archive")
	if err := os.MkdirAll(archiveDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create archive directory: %w", err)
	}

	m := &Manager{
		baseDir:        baseDir,
		archiveDir:     archiveDir,
		retentionCount: retentionCount,
		log:            log,
	}

	// Initialize new timestamped run directory and archive old ones
	if err := m.initializeRun(); err != nil {
		return nil, err
	}

	return m, nil
}

// initializeRun creates a new timestamped directory and archives old runs
func (m *Manager) initializeRun() error {
	// Create timestamped directory for current run
	timestamp := time.Now().Format("20060102_150405")
	m.currentRunDir = filepath.Join(m.baseDir, timestamp)

	if err := os.MkdirAll(m.currentRunDir, 0755); err != nil {
		return fmt.Errorf("failed to create run directory: %w", err)
	}

	m.log.Info().Str("runDir", m.currentRunDir).Msg("Created new timestamped output directory")

	// Archive previous runs
	if err := m.archiveOldRuns(); err != nil {
		m.log.Warn().Err(err).Msg("Failed to archive old runs")
		// Don't fail the whole process if archiving fails
	}

	return nil
}

// archiveOldRuns moves old timestamped directories to archive and cleans up based on retention policy
func (m *Manager) archiveOldRuns() error {
	entries, err := os.ReadDir(m.baseDir)
	if err != nil {
		return fmt.Errorf("failed to read output directory: %w", err)
	}

	// Find all timestamped directories (excluding archive folder)
	var runDirs []string
	for _, entry := range entries {
		if entry.IsDir() && entry.Name() != "archive" {
			runDirs = append(runDirs, entry.Name())
		}
	}

	currentName := filepath.Base(m.currentRunDir)
	var runsToArchive []string
	for _, dirName := range runDirs {
		if dirName != currentName {
			runsToArchive = append(runsToArchive, dirName)
		}
	}

	for _, dirName := range runsToArchive {
		srcPath := filepath.Join(m.baseDir, dirName)
		dstPath := filepath.Join(m.archiveDir, dirName)

		// If destination exists, remove it first
		if _, err := os.Stat(dstPath); err == nil {
			if err := os.RemoveAll(dstPath); err != nil {
				m.log.Warn().Err(err).Str("path", dstPath).Msg("Failed to remove existing archive entry")
				continue
			}
		}

		// Move to archive
		if err := os.Rename(srcPath, dstPath); err != nil {
			m.log.Warn().Err(err).Str("src", srcPath).Str("dst", dstPath).Msg("Failed to move to archive")
			continue
		}
		m.log.Debug().Str("runDir", dirName).Msg("Archived run directory")
	}

	// Clean up old archived runs based on retention policy
	if m.retentionCount > 0 {
		if err := m.pruneArchive(); err != nil {
			m.log.Warn().Err(err).Msg("Failed to prune archive")
		}
	}

	return nil
}

// pruneArchive removes old archived runs exceeding retention count
func (m *Manager) pruneArchive() error {
	entries, err := os.ReadDir(m.archiveDir)
	if err != nil {
		return fmt.Errorf("failed to read archive directory: %w", err)
	}

	// Get all timestamped directories and sort by name (descending = newest first)
	var archived []string
	for _, entry := range entries {
		if entry.IsDir() {
			archived = append(archived, entry.Name())
		}
	}

	sort.Slice(archived, func(i, j int) bool {
		return archived[i] > archived[j]
	})

	// Remove runs exceeding retention count
	if len(archived) > m.retentionCount {
		toDelete := archived[m.retentionCount:]
		for _, dirName := range toDelete {
			path := filepath.Join(m.archiveDir, dirName)
			if err := os.RemoveAll(path); err != nil {
				m.log.Warn().Err(err).Str("path", path).Msg("Failed to delete archived run")
				continue
			}
			m.log.Debug().Str("runDir", dirName).Msg("Deleted old archived run")
		}
	}

	return nil
}

// WriteFile writes data to a file in the current run directory
func (m *Manager) WriteFile(filename string, data []byte) (string, error) {
	filePath := filepath.Join(m.currentRunDir, filename)

	// Create subdirectories if needed
	if dir := filepath.Dir(filePath); dir != m.currentRunDir {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", fmt.Errorf("failed to create subdirectory: %w", err)
		}
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	return filePath, nil
}

// CurrentRunDir returns the path to the current run directory
func (m *Manager) CurrentRunDir() string {
	return m.currentRunDir
}

// ArchiveDir returns the path to the archive directory
func (m *Manager) ArchiveDir() string {
	return m.archiveDir
}
