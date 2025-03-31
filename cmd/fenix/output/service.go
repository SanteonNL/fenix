package output

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"
)

// OutputManager handles centralized output management
type OutputManager struct {
	baseDir   string
	timestamp string
	log       zerolog.Logger
}

// archiveExistingOutputs moves existing output directories to an archive folder
func ArchiveExistingOutputs(baseDir string) error {
	// Create archive directory if it doesn't exist
	archiveDir := filepath.Join(baseDir, "archive")
	if err := os.MkdirAll(archiveDir, os.ModePerm); err != nil {
		return fmt.Errorf("failed to create archive directory: %w", err)
	}

	// Read directory contents
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return fmt.Errorf("failed to read base directory: %w", err)
	}

	// Move each timestamped directory to archive
	for _, entry := range entries {
		// Skip if it's not a directory or if it's the archive directory
		if !entry.IsDir() || entry.Name() == "archive" {
			continue
		}

		// Check if the directory name matches timestamp format (YYYYMMDD_HHMMSS)
		if len(entry.Name()) != 15 { // Length of "YYYYMMDD_HHMMSS"
			continue
		}

		oldPath := filepath.Join(baseDir, entry.Name())
		newPath := filepath.Join(archiveDir, entry.Name())

		// Move the directory
		if err := os.Rename(oldPath, newPath); err != nil {
			return fmt.Errorf("failed to move directory %s to archive: %w", entry.Name(), err)
		}
	}

	return nil
}

// NewOutputManager creates a new OutputManager with the given base directory
func NewOutputManager(baseDir string, log zerolog.Logger) (*OutputManager, error) {
	timestamp := time.Now().Format("20060102_150405")

	// Create the base output directory with timestamp
	outputPath := filepath.Join(baseDir, timestamp)
	if err := os.MkdirAll(outputPath, os.ModePerm); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	// Create logs directory
	logsDir := filepath.Join(outputPath, "logs")
	if err := os.MkdirAll(logsDir, os.ModePerm); err != nil {
		return nil, fmt.Errorf("failed to create logs directory: %w", err)
	}

	// Set up log file
	logFile, err := os.Create(filepath.Join(logsDir, "app.log"))
	if err != nil {
		return nil, fmt.Errorf("failed to create log file: %w", err)
	}

	// Create new logger that writes to both console and file
	consoleWriter := zerolog.NewConsoleWriter(func(w *zerolog.ConsoleWriter) {
		w.Out = os.Stdout
	})
	multiWriter := zerolog.MultiLevelWriter(consoleWriter, logFile)

	// Create new logger with same settings as input logger
	combinedLogger := zerolog.New(multiWriter).
		With().
		Timestamp().
		Caller().
		Logger()

	return &OutputManager{
		baseDir:   outputPath,
		timestamp: timestamp,
		log:       combinedLogger,
	}, nil
}

// WriteToJSON writes data to a JSON file in the output directory
func (om *OutputManager) WriteToJSON(data interface{}, prefix string) error {
	// Generate filename
	filename := fmt.Sprintf("%s_%s.json", prefix, om.timestamp)
	outputPath := filepath.Join(om.baseDir, filename)

	// Create the file
	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer file.Close()

	// Create an encoder with indentation for readable output
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")

	// Write the data
	if err := encoder.Encode(data); err != nil {
		return fmt.Errorf("failed to encode data to JSON: %w", err)
	}

	om.log.Debug().
		Str("file", outputPath).
		Str("prefix", prefix).
		Msg("Wrote data to JSON file")

	return nil
}

// GetLogger returns the configured logger
func (om *OutputManager) GetLogger() zerolog.Logger {
	return om.log
}

// GetOutputPath returns the full path for a given filename
func (om *OutputManager) GetOutputPath(filename string) string {
	return filepath.Join(om.baseDir, filename)
}

// GetTimestamp returns the timestamp being used
func (om *OutputManager) GetTimestamp() string {
	return om.timestamp
}

// GetBaseDir returns the base output directory
func (om *OutputManager) GetBaseDir() string {
	return om.baseDir
}
