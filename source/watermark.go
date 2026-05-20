package source

import (
	"encoding/json"
	"os"

	"github.com/rs/zerolog"
)

// ReadWatermark loads per-table timestamps from a JSON file.
// Returns an empty map if the file does not exist (first run = full load).
func ReadWatermark(path string, log zerolog.Logger) map[string]string {
	if path == "" {
		return map[string]string{}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
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
func WriteWatermark(path string, marks map[string]string) error {
	data, err := json.MarshalIndent(marks, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
