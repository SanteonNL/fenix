package source

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/rs/zerolog"
)

// ReadWatermark loads per-table timestamps from a JSON file.
// Returns an empty map if the file does not exist (first run = full load).
func ReadWatermark(path string, log zerolog.Logger) map[string]string {
	if path == "" {
		log.Warn().Msg("watermark: no path configured — incremental loading disabled")
		return map[string]string{}
	}
	log.Info().Str("path", path).Msg("watermark: reading")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			log.Info().Str("path", path).Msg("watermark: no file yet — first run will be full load")
		} else {
			log.Warn().Err(err).Str("path", path).Msg("watermark: failed to read")
		}
		return map[string]string{}
	}
	var marks map[string]string
	if err := json.Unmarshal(data, &marks); err != nil {
		log.Warn().Err(err).Str("path", path).Msg("watermark: failed to parse")
		return map[string]string{}
	}
	log.Info().Str("path", path).Int("tables", len(marks)).Msg("watermark loaded — incremental mode")
	return marks
}

// WriteWatermark persists per-table timestamps to a JSON file.
// The parent directory is created automatically if it does not exist.
func WriteWatermark(path string, marks map[string]string) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	data, err := json.MarshalIndent(marks, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
