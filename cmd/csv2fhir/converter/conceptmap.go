package converter

import (
	"encoding/csv"
	"fmt"
	"os"
	"strings"

	"github.com/rs/zerolog"
)

// ConceptMapEntry represents one row in a flat concept map CSV.
type ConceptMapEntry struct {
	SourceCode    string
	TargetCode    string
	TargetDisplay string
}

// valuesetMap holds all entries for one valueset and the set of valid target codes.
type valuesetMap struct {
	entries    []ConceptMapEntry
	validCodes map[string]bool // all unique code_target values — already-valid codes skip mapping
}

// ConceptMapService loads flat CSV concept maps indexed by target_valueset_uri.
// If the source code is already a valid target code for a valueset it is passed
// through unchanged — no identity rows needed in the CSV.
type ConceptMapService struct {
	byValueset map[string]*valuesetMap // target_valueset_uri (no version) → map
	logger     zerolog.Logger
}

// NewConceptMapService creates an empty service.
func NewConceptMapService(logger zerolog.Logger) *ConceptMapService {
	return &ConceptMapService{
		byValueset: make(map[string]*valuesetMap),
		logger:     logger,
	}
}

// LoadCSV loads a flat semicolon-delimited concept map CSV.
// The valueset key is taken from the target_valueset_uri column (version suffix stripped).
// Required header columns: code_source, code_target, target_valueset_uri.
func (s *ConceptMapService) LoadCSV(filePath string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open %s: %w", filePath, err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.Comma = ';'
	r.TrimLeadingSpace = true
	r.FieldsPerRecord = -1

	records, err := r.ReadAll()
	if err != nil {
		return fmt.Errorf("read %s: %w", filePath, err)
	}
	if len(records) < 2 {
		return fmt.Errorf("CSV %s has no data rows", filePath)
	}

	header := records[0]
	srcIdx := columnIndex(header, "code_source")
	tgtCodeIdx := columnIndex(header, "code_target")
	tgtDisplayIdx := columnIndex(header, "display_target")
	valuesetIdx := columnIndex(header, "target_valueset_uri")

	if srcIdx < 0 || tgtCodeIdx < 0 || valuesetIdx < 0 {
		return fmt.Errorf("CSV %s missing required columns (code_source, code_target, target_valueset_uri)", filePath)
	}

	type rowGroup struct {
		entries    []ConceptMapEntry
		validCodes map[string]bool
	}
	groups := map[string]*rowGroup{}

	for _, row := range records[1:] {
		vsURI := stripVersion(safeCol(row, valuesetIdx))
		if vsURI == "" {
			continue
		}
		g, ok := groups[vsURI]
		if !ok {
			g = &rowGroup{validCodes: map[string]bool{}}
			groups[vsURI] = g
		}
		tgt := safeCol(row, tgtCodeIdx)
		g.entries = append(g.entries, ConceptMapEntry{
			SourceCode:    safeCol(row, srcIdx),
			TargetCode:    tgt,
			TargetDisplay: safeCol(row, tgtDisplayIdx),
		})
		if tgt != "" {
			g.validCodes[tgt] = true
		}
	}

	for vsURI, g := range groups {
		s.byValueset[vsURI] = &valuesetMap{entries: g.entries, validCodes: g.validCodes}
		s.logger.Info().
			Str("valueset", vsURI).
			Str("file", filePath).
			Int("entries", len(g.entries)).
			Msg("Loaded concept map")
	}
	return nil
}

// LoadDir loads all .csv files from a directory.
func (s *ConceptMapService) LoadDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read dir %s: %w", dir, err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".csv") {
			continue
		}
		if err := s.LoadCSV(dir + "/" + e.Name()); err != nil {
			s.logger.Warn().Err(err).Str("file", e.Name()).Msg("Skipping concept map")
		}
	}
	return nil
}

// Translate maps sourceCode using the concept map for the given valueset URI.
// If the code is already a valid target code for this valueset it is returned unchanged.
// Falls back to the wildcard entry ("*") for unknown codes.
// Returns the original code when no concept map is loaded for the valueset.
func (s *ConceptMapService) Translate(valuesetURI, sourceCode string) (string, bool) {
	vm, ok := s.byValueset[stripVersion(valuesetURI)]
	if !ok {
		return sourceCode, false
	}

	// Already a valid FHIR target code — no mapping needed
	if vm.validCodes[sourceCode] {
		return sourceCode, false
	}

	var wildcard string
	wildcardFound := false
	for _, e := range vm.entries {
		if e.SourceCode == sourceCode {
			s.logger.Debug().
				Str("valueset", valuesetURI).
				Str("from", sourceCode).
				Str("to", e.TargetCode).
				Msg("Concept mapped (exact)")
			return e.TargetCode, true
		}
		if e.SourceCode == "*" {
			wildcard = e.TargetCode
			wildcardFound = true
		}
	}
	if wildcardFound {
		s.logger.Debug().
			Str("valueset", valuesetURI).
			Str("from", sourceCode).
			Str("to", wildcard).
			Msg("Concept mapped (wildcard)")
		return wildcard, true
	}
	return sourceCode, false
}

// stripVersion removes a version suffix like "|4.0.1" from a valueset URI.
func stripVersion(uri string) string {
	if idx := strings.Index(uri, "|"); idx >= 0 {
		return uri[:idx]
	}
	return uri
}

func columnIndex(header []string, name string) int {
	for i, h := range header {
		if strings.TrimSpace(h) == name {
			return i
		}
	}
	return -1
}

func safeCol(row []string, idx int) string {
	if idx < 0 || idx >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[idx])
}
